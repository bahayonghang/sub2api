package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func excelBPSAttachmentImage(t *testing.T, width int) (string, []byte) {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, 3))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes()), data.Bytes()
}

func excelBPSAttachmentResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

const excelBPSAttachmentCompleted = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_attachment\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"output\":[]}}\n\n"

func TestExcelBPSLocalImageUploadsAfterPrepare(t *testing.T) {
	dataURL, pixels := excelBPSAttachmentImage(t, 2)
	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", path, stream), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					excelBPSAttachmentResponse(200, `{"openai_file_id":"file-local/private?image"}`),
					excelBPSAttachmentResponse(200, excelBPSAttachmentCompleted),
					excelBPSAttachmentResponse(200, `{}`),
				}}
				svc := openAIClientToolsTestService(upstream)
				account := excelAccount()
				account.Proxy = &Proxy{Protocol: "http", Host: "proxy.example", Port: 3128}
				body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%v,"input":[{"role":"user","content":[{"type":"input_text","text":"describe"},{"type":"input_image","image_url":%q,"detail":"original"},{"type":"input_image","image_url":%q}]}]}`, stream, dataURL, dataURL))
				original := append([]byte(nil), body...)
				expected, _, err := basispoints.Prepare(body, "account:300/key:0/thread:", nil)
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, original, body)
				require.Len(t, upstream.requests, 3)
				upload, response, cleanup := upstream.requests[0], upstream.requests[1], upstream.requests[2]
				require.Equal(t, "https://bps.openai.com/basispoints/api/attachments", upload.URL.String())
				require.Equal(t, http.MethodPost, upload.Method)
				multipart, err := upload.MultipartReader()
				require.NoError(t, err)
				part, err := multipart.NextPart()
				require.NoError(t, err)
				require.Equal(t, "file", part.FormName())
				require.NotEmpty(t, part.FileName())
				require.Equal(t, "image/png", part.Header.Get("Content-Type"))
				actual, err := io.ReadAll(part)
				require.NoError(t, err)
				require.Equal(t, pixels, actual)
				_, err = multipart.NextPart()
				require.ErrorIs(t, err, io.EOF)
				require.Equal(t, basispoints.ResponsesURL, response.URL.String())
				forwarded := upstream.bodies[1]
				require.Equal(t, gjson.GetBytes(expected, "metadata.task_id").String(), gjson.GetBytes(forwarded, "metadata.task_id").String())
				require.Equal(t, gjson.GetBytes(expected, "metadata.turn_id").String(), gjson.GetBytes(forwarded, "metadata.turn_id").String())
				require.NotContains(t, string(forwarded), dataURL)
				require.Contains(t, string(forwarded), `"file_id":"file-local/private?image"`)
				require.Contains(t, string(forwarded), `"detail":"original"`)
				require.Contains(t, string(forwarded), `"text":"describe"`)
				require.Equal(t, http.MethodDelete, cleanup.Method)
				require.Equal(t, "https://bps.openai.com/basispoints/api/attachments/delete/file-local%2Fprivate%3Fimage", cleanup.URL.String())
				for _, req := range upstream.requests {
					require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
					require.Equal(t, "test-account", req.Header.Get("ChatGPT-Account-ID"))
					require.Equal(t, "test-account", req.Header.Get("X-OpenAI-Account-ID"))
					require.Equal(t, "chatgpt", req.Header.Get("X-Basispoints-Auth-Mode"))
					require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
					require.Equal(t, HTTPUpstreamProfileLongStream, HTTPUpstreamProfileFromContext(req.Context()))
				}
				require.Equal(t, account.Proxy.URL(), upstream.lastProxyURL)
			})
		}
	}
}

func TestExcelBPSAttachmentPartialFailureCleansKnownIDs(t *testing.T) {
	first, _ := excelBPSAttachmentImage(t, 2)
	second, _ := excelBPSAttachmentImage(t, 4)
	for _, failure := range []struct {
		name, body string
		status     int
	}{
		{"HTTP failure", `PRIVATE_ERROR`, 403},
		{"redirect", `PRIVATE_ERROR`, 302},
		{"missing ID", `{"filename":"PRIVATE_ERROR"}`, 200},
		{"invalid JSON", `{PRIVATE_ERROR`, 200},
	} {
		t.Run(failure.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				excelBPSAttachmentResponse(200, `{"openai_file_id":"file-first"}`),
				excelBPSAttachmentResponse(failure.status, failure.body),
				excelBPSAttachmentResponse(200, `{}`),
			}}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"role":"user","content":[{"type":"input_image","image_url":%q},{"type":"input_image","image_url":%q}]}]}`, first, second))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			_, err := svc.Forward(context.Background(), c, excelAccount(), body)
			require.Error(t, err)
			require.Equal(t, 502, rec.Code)
			require.Len(t, upstream.requests, 3)
			require.Equal(t, http.MethodDelete, upstream.requests[2].Method)
			require.Equal(t, "/basispoints/api/attachments/delete/file-first", upstream.requests[2].URL.Path)
			require.NotContains(t, rec.Body.String(), "PRIVATE_ERROR")
			require.NotContains(t, rec.Body.String(), "file-first")
			require.NotContains(t, rec.Body.String(), first)
		})
	}
}

func TestExcelBPSAttachmentRejectsClientFileIDBeforeUpload(t *testing.T) {
	dataURL, _ := excelBPSAttachmentImage(t, 2)
	body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"role":"user","content":[{"type":"input_image","image_url":%q},{"type":"input_image","file_id":"file-private"}]}]}`, dataURL))
	upstream := &httpUpstreamRecorder{}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Empty(t, upstream.requests)
	require.Contains(t, rec.Body.String(), "input[0].content[1]")
}

func TestExcelBPSAttachmentErrorRedaction(t *testing.T) {
	dataURL, _ := excelBPSAttachmentImage(t, 2)
	body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"role":"user","content":[{"type":"input_image","image_url":%q}]}]}`, dataURL))
	errorBody, err := json.Marshal(map[string]any{"error": map[string]any{"message": "Failed file-private and " + dataURL}})
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		excelBPSAttachmentResponse(200, `{"openai_file_id":"file-private"}`),
		excelBPSAttachmentResponse(400, string(errorBody)),
		excelBPSAttachmentResponse(200, `{}`),
	}}
	svc := openAIClientToolsTestService(upstream)
	svc.cfg.Gateway.LogUpstreamErrorBody = true
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err = svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Len(t, upstream.requests, 3)
	for _, output := range []string{rec.Body.String(), c.GetString(OpsUpstreamErrorDetailKey), c.GetString(OpsUpstreamErrorMessageKey)} {
		require.NotContains(t, output, "file-private")
		require.NotContains(t, output, strings.TrimPrefix(dataURL, "data:image/png;base64,"))
	}
}

type excelBPSAttachmentUpstream struct {
	HTTPUpstream
	do func(*http.Request, string, int64, int) (*http.Response, error)
}

func (u *excelBPSAttachmentUpstream) Do(req *http.Request, proxy string, accountID int64, concurrency int) (*http.Response, error) {
	return u.do(req, proxy, accountID, concurrency)
}

type excelBPSAttachmentBody struct {
	read   func([]byte) (int, error)
	closed atomic.Bool
}

func (b *excelBPSAttachmentBody) Read(p []byte) (int, error) { return b.read(p) }
func (b *excelBPSAttachmentBody) Close() error               { b.closed.Store(true); return nil }

func TestExcelBPSAttachmentCleanupAfterResponseOrCancellation(t *testing.T) {
	dataURL, _ := excelBPSAttachmentImage(t, 2)
	body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"role":"user","content":[{"type":"input_image","image_url":%q}]}]}`, dataURL))
	for _, mode := range []string{"success", "HTTP failure", "transport failure", "client cancellation", "cleanup failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := strings.NewReader(excelBPSAttachmentCompleted)
			responseBody := &excelBPSAttachmentBody{read: reader.Read}
			if mode == "client cancellation" {
				responseBody.read = func([]byte) (int, error) { cancel(); return 0, ctx.Err() }
			}
			calls := 0
			upstream := &excelBPSAttachmentUpstream{do: func(req *http.Request, proxy string, accountID int64, concurrency int) (*http.Response, error) {
				calls++
				require.Equal(t, int64(300), accountID)
				require.Equal(t, 10, concurrency)
				switch calls {
				case 1:
					require.Equal(t, http.MethodPost, req.Method)
					return excelBPSAttachmentResponse(200, `{"openai_file_id":"file-lifecycle"}`), nil
				case 2:
					if mode == "transport failure" {
						return nil, errors.New("PRIVATE_TRANSPORT")
					}
					status := 200
					if mode == "HTTP failure" {
						status = 403
					}
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: responseBody}, nil
				case 3:
					require.Equal(t, http.MethodDelete, req.Method)
					if mode != "transport failure" {
						require.True(t, responseBody.closed.Load(), "cleanup ran before closing the upstream body")
					}
					require.NoError(t, req.Context().Err(), "cleanup inherited client cancellation")
					deadline, ok := req.Context().Deadline()
					require.True(t, ok)
					require.InDelta(t, 10, time.Until(deadline).Seconds(), 1)
					require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
					if mode == "cleanup failure" {
						return nil, errors.New("PRIVATE_CLEANUP file-lifecycle")
					}
					return excelBPSAttachmentResponse(200, `{}`), nil
				default:
					t.Fatalf("unexpected request %d", calls)
					return nil, errors.New("unexpected request")
				}
			}}
			svc := openAIClientToolsTestService(nil)
			svc.httpUpstream = upstream
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
			_, err := svc.Forward(ctx, c, excelAccount(), body)
			if mode == "success" || mode == "cleanup failure" {
				require.NoError(t, err)
				require.Equal(t, 200, rec.Code)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, 3, calls)
			require.NotContains(t, rec.Body.String(), "PRIVATE_")
		})
	}
}

func TestExcelBPSAttachmentNoUploadForOtherContent(t *testing.T) {
	dataURL, _ := excelBPSAttachmentImage(t, 2)
	for _, input := range []string{
		`"text only"`,
		`[{"role":"user","content":[{"type":"input_image","image_url":"https://images.example/photo.png"}]}]`,
		fmt.Sprintf(`[{"role":"user","content":"read image"},{"type":"function_call","name":"inspect","call_id":"call_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_text","text":"screenshot"},{"type":"input_image","image_url":%q}]}]`, dataURL),
	} {
		upstream := &httpUpstreamRecorder{resp: excelBPSAttachmentResponse(200, excelBPSAttachmentCompleted)}
		svc := openAIClientToolsTestService(upstream)
		body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}],"input":%s}`, input))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		_, err := svc.Forward(context.Background(), c, excelAccount(), body)
		require.NoError(t, err)
		require.Len(t, upstream.requests, 1)
		require.Equal(t, basispoints.ResponsesURL, upstream.requests[0].URL.String())
		if strings.Contains(input, "data:image") {
			require.Contains(t, string(upstream.lastBody), dataURL)
		}
	}
}

func TestExcelBPSAttachmentStreamErrorRedaction(t *testing.T) {
	dataURL, _ := excelBPSAttachmentImage(t, 2)
	body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":true,"input":[{"role":"user","content":[{"type":"input_image","image_url":%q}]}]}`, dataURL))
	for _, kind := range []string{"error", "response.failed", "response.incomplete"} {
		t.Run(kind, func(t *testing.T) {
			errorFields := map[string]any{"code": "invalid_image", "message": "Failed file-stream-private: \"" + dataURL + "\" test-token"}
			event := map[string]any{"type": kind}
			if kind == "error" {
				for k, v := range errorFields {
					event[k] = v
				}
			} else {
				event["response"] = map[string]any{"id": "resp_error", "status": "failed", "error": errorFields}
			}
			data, err := json.Marshal(event)
			require.NoError(t, err)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				excelBPSAttachmentResponse(200, `{"openai_file_id":"file-stream-private"}`),
				excelBPSAttachmentResponse(200, "event: "+kind+"\ndata: "+string(data)+"\n\n"),
				excelBPSAttachmentResponse(200, `{}`),
			}}
			svc := openAIClientToolsTestService(upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			_, err = svc.Forward(context.Background(), c, excelAccount(), body)
			require.Error(t, err)
			require.Contains(t, rec.Body.String(), kind)
			require.Contains(t, rec.Body.String(), "invalid_image")
			require.NotContains(t, rec.Body.String(), "file-stream-private")
			require.NotContains(t, rec.Body.String(), "test-token")
			require.NotContains(t, rec.Body.String(), strings.TrimPrefix(dataURL, "data:image/png;base64,"))
			require.Len(t, upstream.requests, 3)
		})
	}
}
