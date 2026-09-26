# 验证记录

日期：2026-09-26。

## 实施范围

- 默认本地图片：用户 content 上传至当前账户的官方 BPS 附件接口；工具 output 直接传输有效 data URL。
- 附件在 Prepare 之后生成，仅改最终上游副本。原始历史标识和客户端图片字节保持不变。
- 复用已有图片限制和可选 HTTPS 中转。HTTP 准入预算与中转开关解耦。
- 设置界面可在关闭中转时修改资源限制，并保留原有 API 字段。

## 已通过的检查

| 范围 | 命令或入口 | 结果 |
| --- | --- | --- |
| 前端 | pnpm exec vitest run src/views/admin/__tests__/SettingsView.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts | 2 个文件、64 项测试通过。 |
| 前端类型 | pnpm typecheck | 通过。 |
| 前端 lint | pnpm exec eslint，4 个修改文件 | 通过。 |
| service | go test ./internal/service -run 'TestExcelBPS\|TestHTTPSImages\|TestUnsupportedImage' -count=1 | 通过，6.253 秒。 |
| 图片包 | 官方 golang:1.27.1-alpine 临时容器内 go test ./internal/service/basispoints -count=1 | 完整测试通过，0.697 秒，包含 POSIX 权限断言。 |
| HTTP 准入 | go test ./internal/server/middleware ./internal/server/routes -run 'TestExcelBPS' -count=1 | 通过。另覆盖关闭中转时 gzip/chunked 预算释放。 |
| 格式 | gofmt、修改范围 git diff --check | 通过。 |
| 真实适配器 | probe_bps_adapter.py --run，经 svc.Forward | 4 个合成图片场景通过，详见下一节。 |

失败回归先于实现运行，复现了原始工具图片拒绝及中转关闭时缺少 HTTP 413/503 准入限制。图片和 service 单元测试覆盖格式、MIME、像素、容量、索引、调用关联、历史标识、multipart、清理时序、部分失败、取消及错误脱敏。

## 真实上游结果

当前账户和模型保持不变：账户 1，gpt-6-astra。公网中转关闭，未配置图片 BaseURL。仅使用合成的红蓝双色 PNG。

- user_image：HTTP 200，response.completed；准确识别颜色；upload/delete 均为 200。
- tool_image_stream：HTTP 200，response.completed；准确识别颜色；无上传。
- user_history：HTTP 200，response.completed；准确识别颜色；upload/delete 均为 200。
- tool_history：HTTP 200，response.completed；准确识别颜色；无上传。

视觉请求合计 20.95 秒，Go 测试命令总耗时 26.440 秒。详情见 research/adapter-probe-results.json。临时测试源已删除，凭据未写入仓库。

## 环境记录

- Windows 上既有 POSIX 0600 权限测试得到 0666。未改测试断言；完整图片包已在 Linux 容器通过。
- Go 依赖下载首次出现 unexpected EOF，重试后恢复。Linux 检查使用仓库 Dockerfile 已指定的 goproxy.cn。未修改 go.mod、go.sum 或全局 Go 设置。
- pnpm 首次运行自动安装依赖并触发构建脚本限制，重试已有依赖后检查通过。自身产生的锁文件变更已撤销，包清单未修改。

## 审查与交付

Trellis check 已审查全部实际差异，P1–P6 均已覆盖，无未解决的产品或设计问题。审查修正了新测试中一处未检查的类型断言，以及图片设置文档中的旧标签和容量数字。修改后的长历史测试通过；service/basispoints、service、server/middleware、server/routes 四个 Go 包的定向 go vet 通过。

完整 golangci-lint 未运行：本机 PATH 中没有该工具。使用了定向 go vet、gofmt 和现有前端 ESLint；未安装或修改全局工具配置。

用户于2026-09-26确认本地提交和 Trellis 归档。运行中的本地服务仍使用原镜像。提交与归档结果由 task.json 和开发者日志记录。

附件服务端保留期未确认。实现按请求清理，并在历史重放时重传用户图片。删除失败或超时只记录固定错误分类，不覆盖已完成响应，不进行后台重试。
