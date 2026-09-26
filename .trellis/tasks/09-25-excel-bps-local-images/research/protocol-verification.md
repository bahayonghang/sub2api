# G1：真实 BPS 图片协议验证

日期：2026-09-26 UTC。用户已在上一轮方案摘要后明确要求“开始实施”。本记录覆盖获准计划中的协议验证步骤。

## 固定条件

- 当前 Codex 客户端使用本地 Sub2API，模型为 gpt-6-astra。
- 通过当前客户端 API Key 对应的最近 BPS 用量记录定位账户 1；账户为 active OAuth，未配置账户代理。
- 验证沿用该账户的 BPS Bearer 和 ChatGPT account ID；不更换账户或模型，不修改数据库配置。
- 请求发送至既有 BPS Responses 地址，认证头与 openai_excel_bps.go 的 newExcelBPSRequest 保持一致。
- 使用临时合成的 128×128 PNG：左半红色、右半蓝色。提示词仅要求识别左右颜色，不包含预期答案。
- 凭据只存在于进程内；报告不包含 access token、API Key、账户邮箱或 ChatGPT account ID。

## 结果

| 请求 | UTC 时间 | HTTP | 结果 |
| --- | --- | --- | --- |
| 文字控制 | 05:19:54 | 200 | response.completed，完整返回 BPS_IMAGE_CONTROL_OK。 |
| 用户 content 内 data:image/png;base64 | 05:19:57 | 422 | Invalid request body；未开始视觉回答。 |
| Wikimedia HTTPS 图片控制 | 05:21:05 | 400 | 图片下载返回 400；属于外部图片抓取失败。 |
| 当前官方文档示例 HTTPS 图片 | 05:21:39 | 200 | response.completed，返回画作中人物、服装、椅子和花束的视觉描述。 |

可复核摘要见 protocol-probe-results.json。探测脚本为 probe_bps_images.py，必须显式使用 --run；脚本不写入业务配置。

## 结论

相同账户和模型的 HTTPS 图片请求能进入视觉处理，用户消息的内联 data URL 请求被真实上游以 422 拒绝。全位置直接传输方案所需的用户图片前提不成立；不能通过只移除本地校验实现全部目标。

后续按官方客户端证据分别验证工具结果和用户附件，结果见下节。原生通道、其他模型和其他账户没有作为替代进行测试。

## G1 收敛结果：按内容位置传输

官方客户端协议证据见 bps-upload-protocol.md。已验证以下出站上传与直接工具图片路径；详细结果分别保存在 upload-probe-results.json 和 tool-image-probe-results.json。

| 请求 | HTTP | 视觉或生命周期结果 |
| --- | --- | --- |
| POST /basispoints/api/attachments，multipart 字段 file | 200 | 返回 openai_file_id，MIME 为 image/png，大小 399 字节。 |
| 用户 input_image.file_id，detail=auto/high/original | 各 200 | 均准确返回 Red, blue。 |
| 工具 output 文字数组控制 | 200 | 完整返回 BPS_TOOL_CONTROL_OK。 |
| 工具 output 图片 data URL，detail=auto/high/original | 各 200 | 均准确返回 Red, blue。 |
| 工具图片保留在完整历史中，再次提问 | 200 | 完成响应并返回 Blue。 |
| DELETE /basispoints/api/attachments/delete/{id} | 200 | 合成测试附件已清理。 |

上传的 file_id 放入初次工具结果样本时返回 422；该样本与后续成功工具控制的 item.id 结构不同，因此不据此作一般化结论。实现采用已验证的工具 data URL 路径，无需依赖工具 file_id。

实施据此收敛为：工具结果保留有效 data URL；用户消息上传后仅在最终上游请求中改为 file_id。两条路径无需用户提供公网地址，保持选定账户和模型，符合用户已批准的条件式实施范围。

客户端历史仍保存原始图片字节。用户消息附件按请求创建，在完整响应结束或失败后执行有界清理；后续请求根据原始历史重新上传。不把临时 file_id 写回客户端历史，不引入跨请求附件缓存。

## 修改后适配器验证

2026-09-26，使用当前 Go service 的 svc.Forward、所选账户 1 和 gpt-6-astra，通过隔离测试入口发送合成图片。未启用公网中转，未更换运行中的容器。

- user_image：HTTP 200，response.completed，Red, Blue；附件上传和删除各返回 200。
- tool_image_stream：HTTP 200，response.completed，Red, blue；未调用上传。
- user_history：HTTP 200，response.completed，Red, blue；重传历史用户图片，附件上传和删除各返回 200。
- tool_history：HTTP 200，response.completed，Red, blue；未调用上传。

四个场景通过，视觉请求耗时合计 20.95 秒。复核命令：python -X utf8 .trellis/tasks/09-25-excel-bps-local-images/research/probe_bps_adapter.py --run。脚本按需安装临时测试源，凭据仅传入子进程环境，执行后删除临时测试源。结果见 adapter-probe-results.json，源见 adapter-probe.go.txt。

## 外部依据

HTTPS 成功控制图片取自当日官方 Images and vision 文档：https://developers.openai.com/api/docs/guides/images-vision 。图片地址为该页面当前示例中的 NGA HTTPS 地址。

官方 BPS extension 页面由公开检索发现。研究代理已记录页面真实引用的上传、图片引用和删除代码；来源、构建标识和资源哈希见 bps-upload-protocol.md。
