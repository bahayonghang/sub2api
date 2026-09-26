package basispoints

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/url"
	"strings"

	_ "golang.org/x/image/webp"
)

// Client file IDs are unsupported. The service creates upload references only
// after Prepare validates the request and calculates its history identity.
func validateImage(part object) error {
	if _, exists := part["file_id"]; exists {
		return fmt.Errorf("basispoints input_image does not support client file_id; provide an image_url")
	}
	raw, ok := part["image_url"].(string)
	if !ok || raw == "" {
		return fmt.Errorf("basispoints input_image requires an HTTPS or base64 image_url")
	}
	if !isInlineImage(raw) {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || strings.TrimSpace(raw) != raw {
			return fmt.Errorf("basispoints input_image requires an absolute HTTPS image URL without embedded credentials, or a base64 image data URL")
		}
	}
	if detail, exists := part["detail"]; exists && detail != nil {
		switch text(detail) {
		case "auto", "low", "high", "original":
		default:
			return fmt.Errorf("basispoints image detail must be auto, low, high or original")
		}
	}
	return nil
}

func isInlineImage(raw string) bool {
	raw = strings.TrimSpace(raw)
	return len(raw) >= len("data:") && strings.EqualFold(raw[:len("data:")], "data:")
}

func inlineImagePayload(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if !isInlineImage(raw) {
		return "", "", fmt.Errorf("basispoints inline image requires a base64 image data URL")
	}
	header, payload, ok := strings.Cut(raw[len("data:"):], ",")
	if !ok || !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return "", "", fmt.Errorf("basispoints inline image requires a base64 image data URL")
	}
	declared, params, err := mime.ParseMediaType(header[:len(header)-len(";base64")])
	if err != nil || len(params) != 0 {
		return "", "", fmt.Errorf("basispoints inline image has an invalid media type")
	}
	switch declared {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", "", fmt.Errorf("basispoints inline images must be PNG, JPEG, GIF or WebP")
	}
	if len(payload) > base64.StdEncoding.EncodedLen(imageRelayMaxImageBytes) {
		return "", "", fmt.Errorf("basispoints inline image exceeds the 20 MiB limit")
	}
	if payload == "" {
		return "", "", fmt.Errorf("basispoints inline image contains invalid base64 data")
	}
	return declared, payload, nil
}

func validateInlineImage(reader io.Reader, declared string) error {
	dimensions, format, err := image.DecodeConfig(reader)
	if err != nil || dimensions.Width <= 0 || dimensions.Height <= 0 || int64(dimensions.Width)*int64(dimensions.Height) > imageRelayMaxPixels {
		return fmt.Errorf("basispoints inline image is invalid or exceeds 64 megapixels")
	}
	if "image/"+format != declared {
		return fmt.Errorf("basispoints inline image media type does not match its contents")
	}
	return nil
}

// DecodeInlineImage validates a data URL and returns its bytes and normalized
// MIME type. Both direct attachments and the HTTPS relay use these rules.
func DecodeInlineImage(raw string) ([]byte, string, error) {
	declared, payload, err := inlineImagePayload(raw)
	if err != nil {
		return nil, "", err
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf("basispoints inline image contains invalid base64 data")
	}
	if len(data) > imageRelayMaxImageBytes {
		return nil, "", fmt.Errorf("basispoints inline image exceeds the 20 MiB limit")
	}
	if err := validateInlineImage(bytes.NewReader(data), declared); err != nil {
		return nil, "", err
	}
	return data, declared, nil
}

type requestImages struct {
	count, bytes int
	normalized   []struct {
		part object
		url  string
	}
}

func (images *requestImages) validate(part object) error {
	if err := validateImage(part); err != nil {
		return err
	}
	raw := text(part["image_url"])
	if !isInlineImage(raw) {
		return nil
	}
	if images.count >= imageRelayMaxRequestImages {
		return fmt.Errorf("basispoints accepts at most 20 inline images per request")
	}
	data, declared, err := DecodeInlineImage(raw)
	if err != nil {
		return err
	}
	images.count++
	images.bytes += len(data)
	if images.bytes > imageRelayMaxRequestBytes {
		return fmt.Errorf("basispoints inline images exceed the 32 MiB request limit")
	}
	_, payload, _ := strings.Cut(strings.TrimSpace(raw), ",")
	images.normalized = append(images.normalized, struct {
		part object
		url  string
	}{part: part, url: "data:" + declared + ";base64," + payload})
	return nil
}

func (images *requestImages) normalize() {
	for _, image := range images.normalized {
		image.part["image_url"] = image.url
	}
}
