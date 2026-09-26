# Excel / BPS 本地图片免公网地址支持

## Goal

本地部署启用 Excel / BPS 后，用户图片和工具截图可被当前账户、当前模型读取，无需用户配置公网图片地址。用户于 2026-09-26 批准开始实施；保留现有 HTTPS 中转兼容性。

## Background

用户报告：basispoints does not accept data:image/base64 image input; provide an HTTPS image URL, or disable Basispoints and start a new conversation to send this image (path=input[260].output[2])。

本地拒绝源为 backend/internal/service/basispoints/images.go:16；错误位置由 backend/internal/service/basispoints/content.go:5 附加。初始分析见 research/local-image-analysis.md。

真实 G1 已收敛：用户消息使用官方附件上传后的 file_id；工具结果使用 data URL。两条路径均在账户 1、gpt-6-astra 上识别了合成图片；工具历史重放与附件删除通过。证据见 research/protocol-verification.md，官方契约见 research/bps-upload-protocol.md。

## Requirements

- R1：支持用户消息、函数工具、自定义工具及历史中的有效图片，保留相邻文本与调用关联。
- R2：默认本地配置不要求公网域名、图片托管或隧道；不丢弃图片，不用文本摘要替代视觉输入。
- R3：保持当前账户、模型、BPS 路由、认证和调度；兼容原有 HTTPS 图片及中转部署。
- R4：统一图片格式与容量校验；默认方式具有请求体和并发预算；错误不暴露内容或凭据。
- R5：用户附件保留至上游消费完毕，请求结束或失败后有界清理；客户端历史保留原始图片字节。

## Acceptance Criteria

- [x] P1 → R2/R3：relay=false、BaseURL 为空时，用户图片与工具图片采用各自已验证的上游形状，模型保持不变。
- [x] P2 → R1：覆盖 input[260].output[2]、函数及自定义工具续接、后续文字轮次和 compact；图片与 call_id 保留。
- [x] P3 → R3：HTTPS 原样保留，已启用中转行为不变；无图请求不上传、不改变路由。
- [x] P4 → R4：非法格式、MIME、尺寸、单图/总量/数量及请求体超限被有界拒绝；错误保留位置且不泄露内容。
- [x] P5 → R5：file_id 仅写入最终上游副本，task_id/turn_id 仍基于原始历史；成功、部分上传失败、上游失败和取消均清理已知附件。
- [x] P6 → R2/R4：设置说明默认本地支持；仅可选中转要求公网地址，关闭中转时容量字段仍可配置。

## Decisions and Limits

采用既定条件式计划内的官方上传与工具直接传输。服务端附件 TTL 未确认，按请求清理并在下一轮重传，避免依赖未知 TTL。不采用原生通道备选。实施顺序见 implement.md，技术边界见 design.md。

## Out of Scope

不读取任意客户端路径，不引入第三方图床或隧道，不切换模型或账户，不部署运行中的服务，不上传用户原图，不增加跨请求附件缓存，不重构原有中转存储。
