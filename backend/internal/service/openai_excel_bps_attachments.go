package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/tidwall/gjson"
)

const (
	excelBPSAttachmentsURL           = "https://bps.openai.com/basispoints/api/attachments"
	excelBPSAttachmentUploadTimeout  = 5 * time.Minute
	excelBPSAttachmentCleanupTimeout = 10 * time.Second
	excelBPSAttachmentResponseLimit  = 64 << 10
)

// References belong to one forwarding request and remain valid until its
// response body closes. Client history retains the original image bytes.
type excelBPSAttachments struct {
	service                    *OpenAIGatewayService
	account                    *Account
	token, accountID, proxyURL string
	ids                        []string
}

// prepare runs only after Prepare has validated the full client history and
// calculated task/turn identities. Tool output images stay as data URLs.
func (a *excelBPSAttachments) prepare(ctx context.Context, body []byte) ([]byte, error) {
	hasInlineContent := false
	for _, item := range gjson.GetBytes(body, "input").Array() {
		for _, part := range item.Get("content").Array() {
			if part.Get("type").String() == "input_image" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.Get("image_url").String())), "data:") {
				hasInlineContent = true
			}
		}
	}
	if !hasInlineContent {
		return body, nil
	}
	var request map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		return nil, errors.New("basispoints attachment request is invalid")
	}
	uploaded := make(map[[32]byte]string)
	input, _ := request["input"].([]any)
	for _, rawItem := range input {
		item, _ := rawItem.(map[string]any)
		content, _ := item["content"].([]any)
		for _, rawPart := range content {
			part, _ := rawPart.(map[string]any)
			raw, _ := part["image_url"].(string)
			if part["type"] != "input_image" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:") {
				continue
			}
			data, contentType, err := basispoints.DecodeInlineImage(raw)
			if err != nil {
				return nil, err
			}
			key := sha256.Sum256(data)
			fileID := uploaded[key]
			if fileID == "" {
				fileID, err = a.upload(ctx, data, contentType)
				if err != nil {
					return nil, err
				}
				a.ids = append(a.ids, fileID)
				uploaded[key] = fileID
			}
			delete(part, "image_url")
			part["file_id"] = fileID
		}
	}
	return json.Marshal(request)
}

func (a *excelBPSAttachments) request(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	ctx = WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileLongStream))
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, errors.New("basispoints attachment request is invalid")
	}
	req.Header = excelBPSHeaders(a.token, a.accountID)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (a *excelBPSAttachments) upload(ctx context.Context, data []byte, contentType string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}[contentType]
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="image.` + extension + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		return "", errors.New("basispoints attachment encoding failed")
	}
	if _, err = part.Write(data); err != nil {
		return "", errors.New("basispoints attachment encoding failed")
	}
	if err = writer.Close(); err != nil {
		return "", errors.New("basispoints attachment encoding failed")
	}
	uploadCtx, cancel := context.WithTimeout(ctx, excelBPSAttachmentUploadTimeout)
	defer cancel()
	req, err := a.request(uploadCtx, http.MethodPost, excelBPSAttachmentsURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := a.service.httpUpstream.Do(req, a.proxyURL, a.account.ID, a.account.Concurrency)
	if err != nil {
		return "", errors.New("basispoints attachment connection failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("basispoints attachment upload rejected")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, excelBPSAttachmentResponseLimit+1))
	if err != nil || len(raw) > excelBPSAttachmentResponseLimit {
		return "", errors.New("basispoints attachment response is invalid")
	}
	var result struct {
		FileID string `json:"openai_file_id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || strings.TrimSpace(result.FileID) == "" {
		return "", errors.New("basispoints attachment response is invalid")
	}
	return result.FileID, nil
}

func (a *excelBPSAttachments) cleanup(ctx context.Context) {
	if len(a.ids) == 0 {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), excelBPSAttachmentCleanupTimeout)
	defer cancel()
	for _, fileID := range a.ids {
		if cleanupCtx.Err() != nil {
			logger.LegacyPrintf("service.openai_excel_bps", "attachment cleanup failed: timeout")
			return
		}
		req, err := a.request(cleanupCtx, http.MethodDelete, excelBPSAttachmentsURL+"/delete/"+url.PathEscape(fileID), nil)
		if err != nil {
			logger.LegacyPrintf("service.openai_excel_bps", "attachment cleanup failed: request")
			continue
		}
		resp, err := a.service.httpUpstream.Do(req, a.proxyURL, a.account.ID, a.account.Concurrency)
		if err != nil {
			logger.LegacyPrintf("service.openai_excel_bps", "attachment cleanup failed: transport")
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, excelBPSAttachmentResponseLimit))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.LegacyPrintf("service.openai_excel_bps", "attachment cleanup failed: rejected")
		}
	}
}
