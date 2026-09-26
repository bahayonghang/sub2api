# 本地提交计划

状态：用户于2026-09-26确认提交和归档，按以下44个文件执行。运行中的本地服务不更新。

## 工作提交

拟提交标题：fix(bps): 支持无公网地址的本地图片

拟提交说明：启用 Excel / BPS 后，本地图片被统一 URL 校验拒绝。按已验证的内容位置协议传输图片，保持原始历史标识，并在请求结束后清理上传附件。

本次修改构成一个行为变更，合为一个工作提交。以下文件均属于本任务；未发现未识别的并行修改。提交采用本地 git，不推送，不部署。

## 文件清单

### 产品、测试与用户文档

- backend/internal/server/middleware/excel_bps_image_admission.go
- backend/internal/server/middleware/excel_bps_image_admission_test.go
- backend/internal/server/routes/excel_bps_image_admission_test.go
- backend/internal/service/basispoints/content.go
- backend/internal/service/basispoints/deep_audit_test.go
- backend/internal/service/basispoints/image_relay.go
- backend/internal/service/basispoints/image_relay_test.go
- backend/internal/service/basispoints/images.go
- backend/internal/service/basispoints/images_test.go
- backend/internal/service/basispoints/inline_images_test.go
- backend/internal/service/basispoints/plan_test.go
- backend/internal/service/basispoints/request.go
- backend/internal/service/basispoints/route.go
- backend/internal/service/basispoints/tools.go
- backend/internal/service/openai_excel_bps.go
- backend/internal/service/openai_excel_bps_attachment_test.go
- backend/internal/service/openai_excel_bps_attachments.go
- docs/excel-bps.md
- frontend/src/i18n/locales/en/admin/settings.ts
- frontend/src/i18n/locales/zh/admin/settings.ts
- frontend/src/views/admin/SettingsView.vue
- frontend/src/views/admin/__tests__/SettingsView.spec.ts

### 协议规范

- .trellis/spec/backend/excel-bps-images.md
- .trellis/spec/backend/index.md

### Trellis 任务与验证证据

- .trellis/tasks/09-25-excel-bps-local-images/check.jsonl
- .trellis/tasks/09-25-excel-bps-local-images/commit-plan.md
- .trellis/tasks/09-25-excel-bps-local-images/design.md
- .trellis/tasks/09-25-excel-bps-local-images/implement.jsonl
- .trellis/tasks/09-25-excel-bps-local-images/implement.md
- .trellis/tasks/09-25-excel-bps-local-images/prd.md
- .trellis/tasks/09-25-excel-bps-local-images/research/adapter-probe-results.json
- .trellis/tasks/09-25-excel-bps-local-images/research/adapter-probe.go.txt
- .trellis/tasks/09-25-excel-bps-local-images/research/bps-upload-protocol.md
- .trellis/tasks/09-25-excel-bps-local-images/research/local-image-analysis.md
- .trellis/tasks/09-25-excel-bps-local-images/research/probe_bps_adapter.py
- .trellis/tasks/09-25-excel-bps-local-images/research/probe_bps_images.py
- .trellis/tasks/09-25-excel-bps-local-images/research/probe_bps_tool_images.py
- .trellis/tasks/09-25-excel-bps-local-images/research/probe_bps_upload.py
- .trellis/tasks/09-25-excel-bps-local-images/research/protocol-probe-results.json
- .trellis/tasks/09-25-excel-bps-local-images/research/protocol-verification.md
- .trellis/tasks/09-25-excel-bps-local-images/research/tool-image-probe-results.json
- .trellis/tasks/09-25-excel-bps-local-images/research/upload-probe-results.json
- .trellis/tasks/09-25-excel-bps-local-images/task.json
- .trellis/tasks/09-25-excel-bps-local-images/verification.md

共 44 个候选文件。任务目录包含协议研究、合成图片探测源、脱敏结果和验收记录；不包含凭据或用户原图。

## 后续 Trellis 记录

工作提交后，按仓库流程归档当前任务并记录会话。归档与日志脚本会各自产生记录提交。该流程不涉及其他任务，不推送，不更新运行中的容器。

## 验证依据

详见 verification.md。P1–P6 已覆盖；真实适配器四个场景通过。完整 golangci-lint 未运行，已通过相关 Go 包的 go vet、格式检查及定向测试。
