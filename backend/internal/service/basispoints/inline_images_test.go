package basispoints

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInlineImagesPreserveUserContentAndOriginalHistoryIdentity(t *testing.T) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	for _, detail := range []string{"auto", "low", "high", "original"} {
		t.Run(detail, func(t *testing.T) {
			source := testSource()
			delete(source, "prompt_cache_key")
			item := object{"role": "user", "content": []any{
				object{"type": "input_text", "text": "Inspect the pixels"},
				object{"type": "input_image", "image_url": " \t" + dataURL + "\r\n", "detail": detail},
			}}
			source["input"] = []any{item}
			wire, _ := mustPrepare(t, source, "scope", nil)
			input := mustTestValue[[]any](t, wire["input"])
			parts := mustTestValue[[]any](t, mustTestValue[object](t, input[len(input)-1])["content"])
			require.Equal(t, "Inspect the pixels", mustTestValue[object](t, parts[0])["text"])
			require.Equal(t, dataURL, mustTestValue[object](t, parts[1])["image_url"])
			require.Equal(t, detail, mustTestValue[object](t, parts[1])["detail"])
			metadata := mustTestValue[object](t, wire["metadata"])
			require.Equal(t, fingerprint([]any{"scope", fingerprint(item)}), metadata["task_id"])
			require.Equal(t, fingerprint([]any{"scope", source["input"]}), metadata["turn_id"])
		})
	}
}

func TestInlineImagesToolOutputLongHistoryAndTextContinuation(t *testing.T) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
		t.Run(kind, func(t *testing.T) {
			source := testSource()
			input := make([]any, 261)
			for i := range input {
				input[i] = message("user", "History")
			}
			input[259] = object{"type": "function_call", "call_id": "call_image", "name": "view_image", "arguments": "{}"}
			parts := []any{object{"type": "input_text", "text": "Before"}, object{"type": "input_text", "text": "After"}, object{"type": "input_image", "image_url": dataURL, "detail": "original"}}
			input[260] = object{"type": kind, "call_id": "call_image", "output": parts}
			source["input"] = input
			for _, continuation := range []bool{false, true} {
				if continuation {
					source["input"] = append(input, message("user", "Describe the image again"))
				}
				wire, _ := mustPrepare(t, source, "scope", nil)
				translated := mustTestValue[[]any](t, wire["input"])
				resultIndex := len(translated) - 1
				if continuation {
					resultIndex--
				}
				result := mustTestValue[object](t, translated[resultIndex])
				require.Equal(t, "call_image", result["call_id"])
				require.Equal(t, parts, result["output"])
			}
			mustTestValue[object](t, parts[2])["image_url"] = "data:image/png;base64,PRIVATE_INVALID_IMAGE"
			raw, err := json.Marshal(source)
			require.NoError(t, err)
			_, _, err = Prepare(raw, "scope", nil)
			require.ErrorContains(t, err, "path=input[260].output[2]")
			require.NotContains(t, err.Error(), "PRIVATE_INVALID_IMAGE")
		})
	}
}

func TestInlineImagesCountLimitAcrossMessagesAndTools(t *testing.T) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	source := testSource()
	parts := make([]any, imageRelayMaxRequestImages)
	for i := range parts {
		parts[i] = object{"type": "input_image", "image_url": dataURL}
	}
	source["input"] = []any{object{"role": "user", "content": parts}, object{"type": "function_call", "call_id": "call_image", "name": "view_image", "arguments": "{}"}, object{"type": "function_call_output", "call_id": "call_image", "output": []any{parts[0]}}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	_, _, err = Prepare(raw, "scope", nil)
	require.ErrorContains(t, err, "at most 20")
	require.ErrorContains(t, err, "path=input[2].output[0]")
	require.False(t, strings.Contains(err.Error(), dataURL))
}

func TestInlineImagesAggregateByteLimitAcrossMessagesAndTools(t *testing.T) {
	data := make([]byte, imageRelayMaxRequestBytes/2+1)
	copy(data, relayTestPNG(t))
	part := object{"type": "input_image", "image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}
	source := testSource()
	source["input"] = []any{object{"role": "user", "content": []any{part}}, object{"type": "function_call", "call_id": "call_image", "name": "view_image", "arguments": "{}"}, object{"type": "function_call_output", "call_id": "call_image", "output": []any{part}}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	_, _, err = Prepare(raw, "scope", nil)
	require.ErrorContains(t, err, "32 MiB")
	require.ErrorContains(t, err, "path=input[2].output[0]")
}

func TestInlineImagesRejectClientFileIDs(t *testing.T) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	for _, fileID := range []any{"file-private", "", nil, 12} {
		source := testSource()
		source["input"] = []any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": dataURL, "file_id": fileID}}}}
		raw, err := json.Marshal(source)
		require.NoError(t, err)
		_, _, err = Prepare(raw, "scope", nil)
		require.ErrorContains(t, err, "client file_id")
		require.ErrorContains(t, err, "path=input[0].content[0]")
		require.NotContains(t, err.Error(), "file-private")
	}
}

func TestInlineImagesNormalizeMIMEAndWhitespace(t *testing.T) {
	data := relayTestPNG(t)
	dataURL := " \tDATA:IMAGE/PNG;BASE64," + base64.StdEncoding.EncodeToString(data) + "\r\n"
	decoded, mime, err := DecodeInlineImage(dataURL)
	require.NoError(t, err)
	require.Equal(t, data, decoded)
	require.Equal(t, "image/png", mime)
	source := testSource()
	source["input"] = []any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": dataURL, "detail": "original"}}}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	wire, _, err := Prepare(raw, "scope", nil)
	require.NoError(t, err)
	require.Contains(t, string(wire), "data:image/png;base64,")
	relay, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	rewritten, err := relay.Rewrite(raw, "scope")
	require.NoError(t, err)
	require.NotContains(t, string(rewritten), "BASE64")
	require.Len(t, relay.entries, 1)
}

func TestDecodeInlineImageByteLimit(t *testing.T) {
	for _, size := range []int{imageRelayMaxImageBytes, imageRelayMaxImageBytes + 1} {
		data := make([]byte, size)
		copy(data, relayTestPNG(t))
		decoded, mime, err := DecodeInlineImage("data:image/png;base64," + base64.StdEncoding.EncodeToString(data))
		if size > imageRelayMaxImageBytes {
			require.ErrorContains(t, err, "20 MiB")
			require.Nil(t, decoded)
			require.Empty(t, mime)
		} else {
			require.NoError(t, err)
			require.Equal(t, data, decoded)
			require.Equal(t, "image/png", mime)
		}
	}
}
