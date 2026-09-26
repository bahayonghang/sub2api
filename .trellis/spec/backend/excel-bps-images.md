# Excel BPS image contract

## 1. Scope / Trigger

Use this contract when changing BPS image validation, history translation, attachment transport, or image admission settings. The original failure rejected a valid tool image at input[260].output[2]. The local validator applied one URL restriction to all content positions. Real BPS requests on 2026-09-26 established separate contracts for user messages and tool results.

## 2. Signatures

- Prepare(raw []byte, scope string, replay *ReplayCache) ([]byte, *Bridge, error) in internal/service/basispoints/request.go validates and translates client history.
- DecodeInlineImage(raw string) ([]byte, string, error) returns bounded decoded bytes and the verified MIME type for local images.
- OpenAIGatewayService.forwardExcelBPS owns selected-account authentication and network requests.
- POST https://bps.openai.com/basispoints/api/attachments accepts multipart/form-data with one file field.
- A successful upload returns a nonempty JSON openai_file_id.
- DELETE https://bps.openai.com/basispoints/api/attachments/delete/{encoded_file_id} releases an uploaded attachment.
- ExcelBPSImageAdmission(settings excelBPSImageSettingsReader, configuredMax int64) gin.HandlerFunc owns HTTP body and concurrent request admission.

## 3. Contracts

A user content item starts as {"type":"input_image","image_url":"data:image/png;base64,...","detail":"original"}. Validate the complete client request and call Prepare before uploading. Replace image_url with the returned file_id only in the final upstream body. Keep the original bytes for task_id and turn_id calculation and for client history.

A function or custom tool output image keeps its validated data URL. Preserve adjacent text, call_id, item order, and detail. HTTPS image URLs retain their existing path. Do not accept a client-supplied file_id.

Use the selected account's token, account ID, proxy, and HTTPUpstream transport. Disable redirects and use the fixed BPS origin. Keep each uploaded ID until the upstream response is consumed. Clean up known IDs after success, failure, or cancellation with a bounded context independent of client cancellation. Do not cache file IDs across requests or log image bytes, credentials, or attachment references.

The existing excel_bps_image_relay_enabled and excel_bps_image_base_url settings control the optional HTTPS relay. Local image support needs neither setting. Preserve the configured relay path. The existing excel_bps_image_body_limit_mib, excel_bps_image_budget_mib, and excel_bps_image_max_requests settings apply without the relay. Preserve the existing OpenAI/Composite HTTP route scope, which also includes text requests. Preserve other platform and WebSocket exclusions.

## 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Valid PNG, JPEG, GIF, or WebP data URL | Check decoded content and MIME agreement. |
| auto, low, high, original detail | Retain the value; upstream model restrictions still apply. |
| Invalid Base64, unsupported MIME, mismatched bytes, or invalid dimensions | Reject before uploading; include the content path without echoing payload. |
| Single image >20 MiB, total decoded images >32 MiB, >20 images, or image >64*1024*1024 pixels | Reject with bounded work. |
| Client file_id, mixed file_id/image_url, local path, or file:// URL | Reject with a path-bearing validation error. |
| Upload fails after earlier successful uploads | Release every known attachment and return a fixed error. |
| Cleanup fails | Report a fixed error classification without image or reference data; preserve the completed response. |
| Body or shared admission limit reached | Reject through the existing admission error contract and release reserved resources. |

## 5. Good / Base / Bad Cases

- Good: relay=false and an empty base URL; a user image uploads, BPS receives file_id, and cleanup runs after the response.
- Good: input[260].output[2] contains a valid tool image; BPS receives the same image bytes and call_id.
- Base: text-only or HTTPS-only requests make no attachment upload requests.
- Bad: upload user images before Prepare. Each new file ID changes the history fingerprint.
- Bad: require the HTTPS relay switch to protect local data images with admission limits.

## 6. Tests Required

Adapter tests must verify function/custom outputs, long history, later text turns, compact requests, image bytes, text, and call IDs. Validate MIME, format, pixel, byte, total, count, and path errors.

Service tests must inspect multipart bytes, authentication, proxy and redirect policy, final file_id replacement, unchanged original history identifiers, and zero uploads for tool-only or text-only requests. Cover cleanup after success, partial upload failure, upstream failure, and cancellation.

Admission and UI tests must cover limits while the relay is disabled. Preserve the enabled relay regressions and locale keys. An opt-in test may send only synthetic pixels through the real adapter; do not modify the running deployment or save credentials.

## 7. Invalid and Required Sequences

Invalid sequence: client history -> upload -> Prepare -> responses. Fresh attachment references alter identity calculation.

Required sequence: client history -> validation and Prepare -> user content upload -> final upstream copy -> responses -> bounded cleanup. Tool output data URLs remain in the upstream copy.

Official client source and authenticated protocol evidence are recorded in the excel-bps-local-images Trellis task. The public client build was tools-excel-core-2026-06-16-3af59f22. The endpoint retention period is unknown; request cleanup and historical re-upload avoid depending on a retention period.
