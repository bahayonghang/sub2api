# 技术设计：Excel / BPS 本地图片

## 决策与授权

用户于 2026-09-26 在方案摘要后要求“开始实施”。已完成已批准计划中的 G1 协议研究：用户消息 data URL 返回 422，官方附件上传后的 file_id 可被同账户同模型识别；工具结果 data URL 和历史重放可被直接识别。详见 research/protocol-verification.md。

采用按内容位置传输的官方契约。用户无需新增账户、API Key、模型或公网图片地址。属于既定 A/B 条件式计划的技术收敛；不采用原生通道回退。

## 数据流与所有权

1. 客户端或工具读取授权本地文件，形成标准 input_image 图片字节。服务器不读取客户端路径。
2. HTTP 入口对 OpenAI/Composite 的已有图片准入路由执行请求体和并发预算。预算不再依赖公网中转开关。
3. 若用户已经启用并配置 HTTPS 中转，继续使用现有中转路径。
4. basispoints.Prepare 校验并转换完整客户端历史，保留图片、文本、call_id 及基于原始历史计算的 task_id/turn_id。
5. 未中转的工具 output 图片保留 data URL。用户消息 content 内的 data URL 在 Prepare 之后由 service 上传至官方 attachments，再将最终上游副本中的 image_url 替换为 file_id。
6. 最终请求沿现有 BPS HTTP/SSE 通道发送。完整处理结束后清理本请求上传的附件，包括成功、上游失败、取消和上传部分失败路径。

上传必须在 Prepare 之后，避免每轮新生成的 file_id 改变历史 fingerprint。Prepare 的产物在发往 BPS 前必须完成用户消息附件转换；纯工具图片和 HTTPS 图片不触发上传。

## 适配器契约

- 接受并验证 PNG、JPEG、GIF、WebP 的有效 Base64 data URL；媒体声明与实际格式一致。
- 复用现有上限：单图 20 MiB，单请求最多 20 张内联图片，解码后合计 32 MiB，单图不超过 64×1024×1024 像素。
- content 与函数/自定义工具 output 使用一致的格式和容量规则；仅字段位置决定发送方式。
- 接受已实测的 auto、low、high、original detail；不静默降低 detail。上游模型自身的限制继续由上游明确返回。
- HTTPS URL 原样保留。明确拒绝客户端 file_id、file://、本地路径、混合 URL/file_id 和非法格式。服务端生成的 file_id 仅写入最终上传完成后的请求副本。
- 校验错误保留 input 索引和 content/output 索引，不回显图片数据、URL token 或凭据。
- 数据地址的首尾空白处理在一个共享解析入口统一，不对任意文本字段做替换。

## 官方附件传输

固定目标为 https://bps.openai.com/basispoints/api/attachments。使用 POST multipart/form-data，字段 file 包含合成文件名、MIME 和字节；读取 JSON 的非空 openai_file_id。删除目标为同一官方 origin 的 /basispoints/api/attachments/delete/{URL-encoded file_id}。

复用所选账户的认证头、代理、HTTPUpstreamProfile 与禁止重定向的传输策略。用户图片不发送到第三方图床。不新增凭据配置，不在上传失败后自动切换模型、账户或通道。

先完成本地图片验证再上传。请求内相同消息图片可去重；不建立跨请求缓存或新的数据库记录。每个成功上传返回的 ID 都必须注册清理。清理在上游完全消费图片后执行，使用不受客户端取消影响的短超时上下文；清理失败不覆盖已完成响应，仅记录不含内容或引用的错误分类。不得无界后台重试。

客户端重放原始历史时重新上传用户消息图片，因此无需依赖未知的服务端附件 TTL。若上传已成功但响应中缺少 ID，只能报告协议错误；无法凭猜测构造删除对象。

## 设置与兼容性

默认无需公网中转即可使用图片。现有 relay_enabled 与 base_url 仍仅控制可选 HTTPS 中转，已配置部署的行为保留。

现有请求体、共享资源预算及在途数量字段复用，不增加用户必填项。管理员可在未启用公网中转时查看和调整这些限制。说明预算会覆盖现有 OpenAI/Composite HTTP 准入路由，包括纯文本；其他平台及 WebSocket 继续沿用已有规则。

HTTPS 中转的文件生命周期与访问接口不重构。仅调整本任务所需的校验、service 上传阶段、准入启用条件和对应设置说明。

## 变更边界与验证

- basispoints 图片校验和相关 Prepare/历史测试：消除错误的全局 data URL 禁令，并验证原始 input[260].output[2] 场景。
- openai_excel_bps service、附件助手及测试：官方上传、最终引用替换、认证、部分失败和清理。
- ExcelBPSImageAdmission 及对应路由测试：本地默认方式的读体前预算与释放。
- SettingsView、对应中英文文案、设置测试及 docs/excel-bps.md：解释默认本地支持和可选中转。

保留原有模型映射、账号调度、工具重放和错误终态。协议验收使用合成图片；不上传用户原图。运行变更相关定向测试，再由 Trellis check 代理审查实际差异。回滚代码可恢复旧行为，不自动建立公网隧道。
