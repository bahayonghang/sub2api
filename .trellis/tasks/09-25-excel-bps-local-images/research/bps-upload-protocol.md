# Research: BPS attachment upload protocol

- Query: Can a local Sub2API deployment send image bytes to BPS without a public image host?
- Scope: mixed; official public client assets and local adapter code.
- Date: 2026-09-26.

## Findings

The official Excel client contains a concrete upload path: multipart upload to BPS, followed by an `input_image` with the returned `file_id`. The client also contains an attachment delete operation. These findings provide a source-backed candidate for design B. The research agent made no authenticated requests. Account support and pixel recognition still require the main session's protocol probe.

### Official sources and build identifiers

All assets below were fetched without cookies or credentials. The requests used `User-Agent: Mozilla/5.0`. Each request returned HTTP 200. Only assets referenced by the extension page or its imported modules were followed.

| ID | Source | Evidence |
| --- | --- | --- |
| O1 | `https://bps.openai.com/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/` | HTML declares `bps-build-version=1790362980` and `bps-tools-version-id=tools-excel-core-2026-06-16-3af59f22`. |
| O2 | O1 + `assets/index-u0FoMu0V.js` | Entry module imports the legacy startup module. |
| O3 | O1 + `assets/legacy-office-warning-B9Y3S_PL.js` | Startup imports `x-square-DUrhLSGN.js` and calls its `renderApp`. |
| O4 | O1 + `assets/x-square-DUrhLSGN.js` | Main application bundle with upload, image reference, authentication, and delete code. |
| O5 | O1 + `assets/client-info-BZSGgJF-.js` | Client information header builder, exported as `n` and imported as `kee` by O4. |
| O6 | `https://help.openai.com/en/articles/20001063-chatgpt-for-excel` | Official product documentation confirms attachment processing. The page does not define the HTTP upload contract. |

O4 SHA-256: `7dbf4d37099e9040b07d063cf9554d8b1ada166bf0874aeda1e194f3bb9d851b`. O4 has 6,849,208 decoded characters. O5 SHA-256: `581ca316d8bf4a841b93c2cb36cabdcc5bb80691011cd63939830d9bcf3ccb2a`.

The offsets below are zero-based character offsets after UTF-8 decoding with Python, not byte offsets or line numbers. The assets are minified; the symbol and hash provide additional anchors.

### Upload and inference contract

| Step | Confirmed client behavior | O4 anchor |
| --- | --- | --- |
| Upload | `POST https://bps.openai.com/basispoints/api/attachments`; multipart field `file`, with file name, MIME type, and bytes. The browser supplies the multipart boundary. No `purpose` field is added. | `dOt`, offset 3962256 |
| Parse result | Require a nonempty `openai_file_id`. Read optional `filename`, `content_type`, `size`, and `input_tokens`. | `fOt`, near offset 3963080 |
| Reference image | Build the image content item shown below. Both branches of the attachment conversion retain image references. | `Nme`, offset 4266363 |
| Read attachment | `GET https://bps.openai.com/basispoints/api/attachments/{encoded_file_id}/content`, with authentication. | `wre`, near offset 1913946 |
| Delete attachment | `DELETE https://bps.openai.com/basispoints/api/attachments/delete/{openai_file_id}`, with authentication. | `Ojn`, near offset 6433265 |

The image content shape is:

```json
{
  "type": "input_image",
  "file_id": "<openai_file_id from upload>",
  "detail": "auto"
}
```

`Fee` and `FL` resolve the `chatgpt` mode base URL to `https://bps.openai.com/basispoints/api`. Thus the full URLs above follow the client resolver; they are not guessed endpoint names. `FL` starts at offset 102761.

### Authentication and lifecycle

O4 `BA`, `H6`, `kTe`, and `nQt` add `Authorization: Bearer <token>` and `X-Basispoints-Auth-Mode: chatgpt`. When the token provides account claims, the client adds `X-OpenAI-Account-Id`, `ChatGPT-Account-ID`, and `X-OpenAI-Account-User-Id`. Upload requests also request the client information headers from O5. The source does not establish which optional headers the server requires. Authentication handling starts near O4 offset 103786; `BA` starts at 104255.

The upload UI requires ChatGPT sign-in. An API key upload flow is not established by this research. `dOt` has a five-minute client timeout. That timeout is not a server retention limit.

`Ojn`, `Pjn`, and `Tjn` delete a completed attachment when the user removes the pending attachment, or when an upload finishes after cancellation. The source does not establish automatic deletion after inference, file TTL, retention after failed requests, or cross-account access rules. An implementation must keep uploaded references valid while the server consumes the request and must define cleanup.

### Tool image evidence requires a separate probe

The official `read_range_image` executor `Jgr` creates an `input_image` with a PNG data URL. `Eqr`, near O4 offset 4347439, preserves data URLs in image tool content and supplies the default detail. Static client code does not prove that the current server accepts every content position.

The main session's [protocol verification](protocol-verification.md) records HTTP 422 for a user-message data URL and HTTP 200 for a public HTTPS control, using the same account and model. That result establishes a user-message restriction. The result does not independently establish the contract for function tool output, custom tool output, or historical replay. Test the uploaded `file_id` at all required positions before implementing design B.

### Local files and code patterns

| File | Relevant evidence |
| --- | --- |
| `backend/internal/service/basispoints/NOTICE.md:3` | Attributes the adapter to `hloolx/codex2api`; lists imported commits and the secondary CPA review reference. Official client evidence now supplies the upload contract directly. |
| `backend/internal/service/basispoints/request.go:15` | Defines the existing BPS Responses endpoint. |
| `backend/internal/service/basispoints/images.go:9` | States that the current bridge does not upload image bytes. |
| `backend/internal/service/basispoints/images.go:11` | Requires `image_url`; rejects data URLs and `file_id`. This validation will need a verified upload-reference path. |
| `backend/internal/service/openai_images.go:1592` | Resolves existing `file-service://` or `sediment://` pointers through authenticated download requests. The helper does not implement the BPS upload operation. |
| `.trellis/tasks/09-25-excel-bps-local-images/design.md` | Design B requires evidence for upload, account scope, lifecycle, and replay. |

### Related specs

- `.trellis/spec/guides/cross-layer-thinking-guide.md`: assign one owner for transport payload decoding and preserve data across layer boundaries.
- `.trellis/spec/backend/index.md`: backend guide index.
- `.trellis/spec/backend/error-handling.md`: currently a template; it defines no concrete BPS error contract.
- `.trellis/workflow.md`: persist research and verify design premises before implementation.

## Caveats / Not Found

- This report confirms official client behavior at the identified build. A private endpoint has no stability guarantee from the public help article.
- No credentials, user files, or authenticated endpoints were accessed by the research agent. Public asset requests did not upload files.
- Real upload success, same-account vision, `file_id` tool outputs, history replay, deletion response shape, and server retention remain for the main session to verify.
- The client UI uses 20 files, 200 MiB per file, and 500 MiB total near O4 offset 1908494. Those generic UI limits do not establish server image limits. Keep the existing narrower local image limits until protocol evidence justifies a change.
- Root and nested `CONTEXT.md` files were not found in the inspected repository scope. No product code, specs, or Git state were changed.
