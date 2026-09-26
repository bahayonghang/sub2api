# Excel / BPS 本地图片分析

## 结论与证据边界

已确认本地拒绝链路。当前代码将未转换的 data: 图片拒绝在向 BPS 发起 HTTP 请求之前。已有图片中转把图片保存到本地磁盘，再生成由 BPS 回源读取的 HTTPS 链接；该方式要求部署地址对上游可达。

尚未确认 BPS 服务本身是否接受内联图片，或是否提供适用于当前认证方式的图片上传接口。不能仅凭本地错误文案，把上游协议能力判定为不支持。推荐先验证同账户、同模型的 BPS 内联图片协议，再确定产品实现。

分析日期：2026-09-25（工作区本地日期）。代码基线：0f2ed3d62，分支 dev。未读取用户的数据库、凭据或实际服务配置。用户报告的故障作为事实输入；本轮没有重新运行故障复现或产品测试。

## 错误如何产生

| 环节 | 代码证据 | 可确认事实 |
| --- | --- | --- |
| 路由 | backend/internal/service/openai_gateway_forward.go:81 | 开启 BPS 的适用模型进入 forwardExcelBPS。 |
| 图片留在 BPS | backend/internal/service/basispoints/route.go:43 | 内联图片不会触发原生通道回退，代码明确依赖中转改写。 |
| 读取设置 | backend/internal/service/openai_excel_bps.go:52 | 图片中转未启用时返回 nil relay。 |
| 默认值 | backend/internal/service/setting_parse.go:277 | 中转默认关闭，公网地址默认为空。 |
| 无中转行为 | backend/internal/service/basispoints/image_relay.go:176 | nil relay 原样返回请求。 |
| 调用顺序 | backend/internal/service/openai_excel_bps.go:166 | Rewrite 在 Prepare 之前执行；Prepare 失败后不会创建上游图片请求。 |
| 工具结果校验 | backend/internal/service/basispoints/tools.go:330 | function_call_output 和 custom_tool_call_output 的 output 数组进入图片校验。 |
| 错误位置 | backend/internal/service/basispoints/content.go:5 | 校验为错误添加 input 索引、字段名和内容索引。 |
| 拒绝源 | backend/internal/service/basispoints/images.go:15 | data: 输入直接返回用户看到的错误。 |

path=input[260].output[2] 使用零基索引，指向第 261 个输入项的第 3 个 output 内容块。对应工具结果数组中的图片，不能只修复用户消息的 content 数组。

BPS 按展开后的完整历史转换请求。backend/internal/service/basispoints/request.go:84 拒绝 previous_response_id；backend/internal/service/basispoints/tools.go:298 遍历全部输入。因此，即使下一轮只输入文字，历史工具图片仍会再次触发错误。

代码可以确认图片没有在当前校验前转换成 HTTPS 引用。用户部署的具体设置值和运行版本未核对。默认中转关闭是符合当前现象的执行路径。另有同类边界：image_relay.go:217 从字符串首字符判断 data:，images.go:15 则先去掉首尾空格；带前置空格的数据地址可能跳过中转后被拒绝。该边界需要进入未来规范化测试。

## 现有中转为何不满足本地部署

1. image_relay.go:76 校验 HTTPS origin；setting_excel_bps_image.go:33 在启用中转时要求地址合法。
2. image_relay.go:204 遍历 content 和工具 output；image_relay.go:232 把图片改写为 baseURL + /api/bps-images/ + token。
3. openai_excel_bps.go:65 把临时图片放在 DATA_DIR/bps-images 下。server/routes/gateway.go:33 注册供上游读取的 GET/HEAD 路由。
4. frontend/src/i18n/locales/zh/admin/settings.ts:22 明确要求公网 HTTPS 地址；SettingsView.vue:7374 在启用中转时显示地址配置。

图片已经能保存到本地。缺失的能力是向远端 BPS 传递图片内容且不要求入站回源。把 URL 改为 localhost、127.0.0.1、局域网地址或 file:// 不能让远端服务读取用户机器。HTTPS 字符串校验也没有验证实际网络可达性。

## 现有测试能证明什么

| 测试 | 已覆盖契约 | 不能证明的事项 |
| --- | --- | --- |
| basispoints/images_test.go:27 | 本地拒绝 data:、file_id、file://、HTTP 和不支持的 detail。 | BPS 是否实际拒绝相同输入。 |
| openai_excel_bps_image_test.go:71 | 中转关闭或无效图片会在本地失败，upstream.requests 为空。 | 真实账户和模型的视觉能力。 |
| openai_excel_bps_image_test.go:22 | TLS 测试服务器能取回改写后的图片；包含 Responses/compact 和流式/非流式。 | 测试以固定 image received 文本模拟上游，没有验证模型读取像素。 |
| basispoints/image_relay_test.go:111 | 函数和自定义工具结果图片的中转、URL 和文本保留。 | 无中转的直接传输。 |
| basispoints/history_recovery_test.go:162 | HTTPS 图片与历史工具调用的恢复。 | 原生通道与 BPS 跨协议切换时的完整兼容性。 |

本轮只读取上述测试，没有执行。未来验收需包含真实 BPS 的无敏感内容合成图片，不能用固定模拟响应代替视觉验收。

## 方案比较

| 方案 | 用户配置公网地址 | BPS 与当前模型 | 判断 |
| --- | --- | --- | --- |
| A：内联 data URL 随 BPS 请求发送 | 不需要 | 保持 | 优先验证；协议支持尚未确认。 |
| B：服务端主动上传到 BPS 兼容的受支持文件服务，再引用结果 | 不需要用户托管 | 取决于上传接口与引用契约 | 次选研究方向；接口、鉴权、隔离和生命周期均未知。 |
| C：图片请求走原生 Codex 通道 | 不需要 | 通道发生变化；当前模型权限未必相同 | 已有回退路由可参考，不能自动作为默认方案。 |
| D：公网 HTTPS 中转或隧道 | 需要公网回源基础设施 | 保持 | 现有部署兼容路径，不满足本次目标。 |
| E：只放宽 HTTPS 校验、换成 localhost、删除图片或仅 OCR | 表面不需要 | 无法保证视觉输入 | 不采用。 |

A 的最低实现成本最小，前提是 BPS 接受该格式。B 不能假设普通 OpenAI Files API 的 file_id 能用于 BPS；当前适配器会拒绝 file_id。C 需单独决定是否接受协议切换，并验证同模型可用性、认证、调用历史和计费；不静默替换模型。

## 上游能力验证门槛 G1

后续获准实施时，先使用合成图片进行独立协议验证。沿用当前选中账户、模型和 newExcelBPSRequest 的认证头；不保存或显示凭据。不用用户原图。

- 构造小型有效 PNG，内容包含仅从像素可知的颜色和位置。提示词不给出预期答案。
- 分别检查用户 content、function_call_output.output、custom_tool_call_output.output 的请求转换结果；保持 call_id 配对。
- 对同一模型比较无图片控制请求和有效 data URL 请求。若已有可用 HTTPS 控制图片，可补充；不得为本任务建立公网托管。
- 记录经过脱敏的请求形状、状态码、错误代码、最终可视回答、流式行为和日期。HTTP 200 或正常 token usage 不足以认定图片已被读取。
- 图片输入、工具续接和历史重放均通过后，才能选定 A。若只支持部分内容位置，记录边界，重新设计合法转换，不能直接删除图片或改为文本。
- 若 A 被真实上游拒绝，研究有证据支持的上传契约 B。不得猜测上传地址，不得尝试绕过权限。
- 若 A 和 B 都不可用，报告当前同通道约束下无法满足目标，再讨论 C。不要把未验证的 B 当成已存在能力。

## 外部资料与限制

官方 OpenAI Images and vision 文档列出普通 Responses API 的 URL、Base64 data URL 和文件引用输入。该文档仅作为协议候选依据，不证明 BPS 私有入口具有相同契约。

来源：https://developers.openai.com/api/docs/guides/images-vision 。本轮查阅了该页面的 Passing a Base64 encoded image 部分。

本轮对公开入口 https://bps.openai.com 发起无认证 GET，返回 HTTP 404。该结果不能判断 POST /basispoints/api/responses 或登录后上传接口的能力。没有找到足以确认 BPS 图片上传接口的公开资料；未发起带账户认证的上游实验。
