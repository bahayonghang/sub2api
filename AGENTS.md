<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

# 本仓库合并约束

本机部署使用 `dev`，只供本机使用。每次上游更新都按 [上游更新合并到dev.md](上游更新合并到dev.md) 执行：拉取 `ranxi2001/sub2api` 的 `production`，先更新本仓库 `production`，再合并进 `dev`，然后重建本机容器。本仓库 `production` 保持与上游 `production` 尖端一致。

## Excel / BPS 本地图片

`dev` 在上游 `ranxi2001/sub2api` 的 `production` 之外保留提交 `10b5660dc`（`fix(bps): 支持无公网地址的本地图片`，2026-09-26）。这是本 fork 的图片本地传输优化。把本仓库 `production` 合并进 `dev` 时保留下面的行为。契约在 `.trellis/spec/backend/excel-bps-images.md`，操作说明在 `docs/excel-bps.md` 的「本地图片」。

管理后台的「立即更新」会下载 `ranxi2001/sub2api` 的 Release，并替换当前进程里的二进制。该 Release 不含 `10b5660dc`。本机运行镜像是 `sub2api:local`。版本更新按 [上游更新合并到dev.md](上游更新合并到dev.md) 执行。容器和数据目录的细节见 [本地部署指南.md](本地部署指南.md)。

### 必须保留的传输

- 默认不要求公网域名、图片托管、隧道或 `excel_bps_image_relay_enabled`。
- 客户端继续发送 `input_image.image_url` 的 data URL。服务器不读取客户端文件路径，并拒绝客户端自带的 `file_id`。
- 顺序固定：用原始历史做校验和 `Prepare`，再上传用户图片，再写最终上游请求，最后在成功、失败或取消后清理已知附件。`task_id` 和 `turn_id` 按原始历史计算。
- 用户消息中的图片经当前账号调用 `POST https://bps.openai.com/basispoints/api/attachments`。只在最终上游正文里把 `image_url` 换成返回的 `openai_file_id`。
- 函数工具和自定义工具结果中的图片保持校验后的 data URL。
- 已经是 HTTPS 的图片保持原 URL，不触发附件上传。
- `detail` 保留 `auto`、`low`、`high`、`original`。
- 继续限制为 PNG、JPEG、GIF、WebP，单张最多 20 MiB，每个请求最多 20 张，解码后合计不超过 32 MiB，单张最多 64×1024×1024 像素。
- `excel_bps_image_body_limit_mib`、`excel_bps_image_budget_mib`、`excel_bps_image_max_requests` 在中转关闭时仍然生效。
- 公网 HTTPS 图片中转保持为可选项，默认关闭，配置继续存在数据库里。

### 合并时按本仓库语义处理的文件

这些文件承载上述行为。出现冲突时，先保留本仓库的图片传输语义，再把上游里与图片传输无关的改动接回去：

- `backend/internal/service/basispoints/images.go`
- `backend/internal/service/basispoints/image_relay.go`
- `backend/internal/service/basispoints/content.go`
- `backend/internal/service/openai_excel_bps.go`
- `backend/internal/service/openai_excel_bps_attachments.go`
- `backend/internal/server/middleware/excel_bps_image_admission.go`
- `docs/excel-bps.md`
- `frontend/src/views/admin/SettingsView.vue`
- `frontend/src/i18n/locales/zh/admin/settings.ts`
- `frontend/src/i18n/locales/en/admin/settings.ts`

2026-09-26 已对上游 `production` `1c5151cc` 做过 `git merge-tree` 干跑。该提交包含 tag `v2.8.15`（`7fd73c1c`），以及其后的 Grok 媒体资格合并。与本地 `dev` 的共同祖先是 `f671a8d30`。干跑没有冲突。结果里 `images.go`、`images_test.go`、`image_relay.go` 和附件上传文件与当时的 `dev` 相同。`content.go` 只增加 `encrypted_content` 诊断。`openai_excel_bps.go` 在 `attachments.prepare` 之后把 `bridge.Stream` 换成 `StreamWithToolRepair`。纠错请求使用已经准备好的上游正文，不重新上传，也不把 data URL 写回。工具历史回放和封装纠错可以一并保留。
