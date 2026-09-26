# 实施计划

## 状态与证据

用户已批准开始实施。G1 已验证：工具结果 data URL 与历史重放成功；用户消息通过官方附件上传 + file_id 成功；附件删除成功。按 design.md 实现，不要求用户新增部署配置。

## 1. 协议与规划

- [x] 保存官方客户端上传、引用、删除和认证来源。
- [x] 用当前账户与模型验证合成图片及 auto/high/original detail。
- [x] 根据真实结果收敛 PRD、设计和上下文。

## 2. 后端实施

- [x] 图片校验允许有效 data URL，复用现有容量和格式规则，保留客户端 file_id 拒绝及路径错误。
- [x] 工具图片直接发送，覆盖函数/自定义工具/历史/input[260].output[2]。
- [x] Prepare 后上传用户 content 图片，只替换最终请求副本，保持原始历史 fingerprint。
- [x] 固定官方 BPS endpoint，复用账户、代理和禁止重定向策略；注册并有界清理本请求附件。
- [x] 将预算与公网中转开关解耦，保持平台和路由边界、读体前拒绝与资源释放。
- [x] 保留现有中转回归，更新相关文档。

## 3. 界面实施

- [x] SettingsView 说明默认本地支持与可选中转。
- [x] 中转关闭时仍显示请求体、预算和在途数量字段；地址仅中转开启时必填。
- [x] 更新中英文文案和对应设置测试，不调整无关设置。

## 4. 验证

后端新增回归应先对应当前缺失行为失败，再由实现修复。覆盖字节/文本/调用关系、multipart 与 file_id 形状、原始历史标识、无图不上传、原始 file_id 拒绝、部分失败和取消清理、预算与脱敏。

    go test ./internal/service/basispoints
    go test ./internal/service -run 'TestExcelBPS|TestHTTPSImages|TestUnsupportedImage' -count=1
    go test ./internal/server/middleware ./internal/server/routes -run 'TestExcelBPS' -count=1

前端：

    pnpm exec vitest run src/views/admin/__tests__/SettingsView.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
    pnpm typecheck

前端结果：两个定向测试文件共 64 项测试通过，typecheck 与四个修改文件的 ESLint 通过。首次 pnpm 执行产生的 lockfile 改动已撤销，包清单未修改。

- [x] 实施代理完成本范围测试，Trellis check 代理审查实际差异。
- [x] 使用隔离入口和合成图片验证修改后的真实适配器，不替换运行中的容器。
- [x] 对照 PRD 记录通过范围和限制，详见 verification.md。
- [x] 用户确认第3.4阶段提交清单，完成本地工作提交10b5660dc。归档状态与会话记录见task.json和开发者日志。

## 回滚与边界

验证失败不部署。回滚代码，不改变当前账户、模型或用户的公网设置。清理失败记录固定错误分类，不覆盖完成响应，不无界重试。日志与报告不包含凭据、图片内容或附件引用。
