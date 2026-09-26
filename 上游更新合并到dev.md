# 上游更新：先更新 production，再合并到 dev

本机部署使用仓库的 `dev` 分支，只供本机使用。每次跟上上游按这个顺序执行：

1. 拉取 `ranxi2001/sub2api` 的 `production`。
2. 用该尖端更新本仓库的 `production`，使本仓库 `production` 与上游 `production` 一致。
3. 把本仓库 `production` 合并进 `dev`。
4. 按 [本地部署指南.md](本地部署指南.md) 重建本机容器。

图片本地传输的保留条件在 [AGENTS.md](AGENTS.md) 的「Excel / BPS 本地图片」。重复构建使用 [redeploy-local.ps1](redeploy-local.ps1)。

2026-09-26 的代码合并提交是 `13c876c4`，内容来自上游 `1c5151cc`（含 tag `v2.8.15`，`7fd73c1c`）。本仓库 `production` 随后快进到同一个 `1c5151cc`。`dev` 已包含该提交。之后每次都先移动 `production`，再合并到 `dev`。

---

## 1. 固定对象

| 项目 | 值 |
|------|-----|
| 源码目录 | `D:\Documents\Code\Agents\sub2api` |
| 部署分支 | `dev` |
| 本仓库集成分支 | `production` |
| 本机 `origin` | `https://github.com/bahayonghang/sub2api.git` |
| 上游仓库 | `https://github.com/ranxi2001/sub2api.git` |
| 上游分支 | `production` |
| 本地上游引用 | `refs/remotes/ranxi2001/production` |
| 运行目录 | `D:\Documents\Code\Agents\sub2api\local-deploy` |
| 运行镜像 | `sub2api:local` |
| 宿主机端口 | `8081`，容器内 `8080` |

管理后台的「立即更新」会下载 `ranxi2001/sub2api` 的 Release，并替换当前进程里的二进制。该 Release 不含提交 `10b5660dc`。版本更新使用本文件的分支更新和重新构建。

本流程更新本地 `production` 与 `dev`，并重建本机容器。推送到 `origin` 另作决定。

---

## 2. 拉取上游并更新本仓库 production

在源码目录执行。工作区需能完成合并；已跟踪文件有未提交改动时，先提交或暂存。当前分支保持 `dev`。

```powershell
Set-Location D:\Documents\Code\Agents\sub2api
git status -sb
git branch --show-current
git fetch https://github.com/ranxi2001/sub2api.git production:refs/remotes/ranxi2001/production
git rev-parse production
git rev-parse refs/remotes/ranxi2001/production
git log --oneline production..refs/remotes/ranxi2001/production
```

`git branch --show-current` 的结果应为 `dev`。两个 `rev-parse` 相同，且上一条日志为空时，本仓库 `production` 已经是上游尖端，进入第 3 节。

本仓库 `production` 是上游尖端的祖先时，快进：

```powershell
git push . refs/remotes/ranxi2001/production:production
git rev-parse production
git rev-parse refs/remotes/ranxi2001/production
```

两条提交号必须相同。快进被拒绝时，说明本仓库 `production` 上有上游没有的提交。切到 `production`，把上游合并进来，再回到 `dev`：

```powershell
git switch production
git merge --no-ff refs/remotes/ranxi2001/production -m "merge: 更新 production 到上游 <提交号>"
git switch dev
```

上游 `production` 的尖端可以比最新 Release tag 多几个提交。本仓库 `production` 对齐的是这个分支尖端。记录其完整提交号，以及 `backend/cmd/server/VERSION`。

---

## 3. 检查 production 相对 dev 的差异

```powershell
git log --oneline HEAD..production
git diff --stat HEAD...production
```

`HEAD..production` 为空时，`dev` 已包含本仓库 `production`，跳过第 4 节和第 5 节的合并，直接做第 6 节的状态检查。

`AGENTS.md` 列出的图片传输文件若出现在差异里，先做无检出合并：

```powershell
git merge-tree --write-tree --name-only --messages HEAD production
```

命令退出码为 0 且输出没有冲突文件名时，可以合并。出现冲突时，先保留 `AGENTS.md` 中的图片传输语义，再把 `production` 里与图片传输无关的改动接回去。合入之后仍须满足：

- 用户消息图片先用原始历史做校验和 `Prepare`，再经当前账号上传，只在最终上游正文里把 `image_url` 换成 `file_id`。
- 函数工具和自定义工具结果中的图片保持校验后的 data URL。
- `detail` 保留 `auto`、`low`、`high`、`original`。
- 公网 HTTPS 图片中转保持为可选项，默认关闭。
- 请求体、预算和在途请求限制在中转关闭时仍然生效。

---

## 4. 把 production 合并到 dev

提交说明里写上本仓库 `production` 的提交号。版本号按该提交的 `backend/cmd/server/VERSION` 填写。

```powershell
git merge --no-ff production -m "merge: 合并 production v<版本号> 到 dev" -m "保留 10b5660dc 的 Excel/BPS 本地图片传输。production 为 <提交号>。"
```

合并完成后核对：

```powershell
git merge-base --is-ancestor production HEAD
git status -sb
Select-String -Path backend\internal\service\openai_excel_bps.go -Pattern "attachments.prepare","StreamWithToolRepair"
Select-String -Path backend\internal\service\basispoints\images.go -Pattern "isInlineImage","auto, low, high or original"
Get-Content backend\cmd\server\VERSION
git diff --name-only --diff-filter=U
```

`git merge-base --is-ancestor` 的退出码为 0，表示 `dev` 已包含本仓库 `production`。`openai_excel_bps.go` 中 `attachments.prepare` 位于 `StreamWithToolRepair` 之前。`git diff --name-only --diff-filter=U` 无输出。工作区里不留下 `<<<<<<<` 冲突标记。

---

## 5. 重建本机容器

`dev` 的代码相对正在运行的镜像有变化时才重建。

```powershell
Set-Location D:\Documents\Code\Agents\sub2api
powershell -NoProfile -ExecutionPolicy Bypass -File .\redeploy-local.ps1
```

脚本构建 `sub2api:local`，只重建应用容器。PostgreSQL 与 Redis 保持运行。`local-deploy` 里的 `.env`、`data`、`postgres_data`、`redis_data` 保持原文件。缺少 `local-deploy\data\config.yaml` 或 `local-deploy\data\.installed` 时脚本停止，不执行首次安装。

脚本可能在打印 `sub2api running healthy` 之后以退出码 1 结束。已发生过的原因是启动日志把警告写到标准错误，PowerShell 在收集 `docker logs` 时把它当成失败。容器已经健康时，用第 6 节补做脚本未完成的检查。

---

## 6. 核对运行结果

```powershell
docker inspect sub2api --format "{{.Config.Image}} {{.State.Status}} {{.State.Health.Status}}"
docker port sub2api
curl.exe -fsS http://127.0.0.1:8081/health
curl.exe -fsS http://127.0.0.1:8081/setup/status
docker exec sub2api /app/sub2api -version
docker logs sub2api *> $env:TEMP\sub2api-redeploy.log
Select-String -Path $env:TEMP\sub2api-redeploy.log -Pattern "Admin user created" -Quiet
```

最后一条的结果应为 `False`。数据库用户名和库名取 `local-deploy\.env` 的 `POSTGRES_USER`、`POSTGRES_DB`，缺省都是 `sub2api`。

```powershell
docker exec sub2api-postgres psql -U sub2api -d sub2api -tAc "SELECT 'users=' || COUNT(*) FROM users;"
docker exec sub2api-postgres psql -U sub2api -d sub2api -tAc "SELECT 'accounts=' || COUNT(*) FROM accounts;"
docker exec sub2api-postgres psql -U sub2api -d sub2api -tAc "SELECT 'api_keys=' || COUNT(*) FROM api_keys;"
```

| 检查 | 期望 |
|------|------|
| 本仓库 `production` | 与 `refs/remotes/ranxi2001/production` 相同 |
| `dev` | 包含本仓库 `production` |
| 镜像与状态 | `sub2api:local running healthy` |
| 端口 | `8080/tcp -> 0.0.0.0:8081` |
| `/health` | `{"status":"ok"}` |
| `/setup/status` | `needs_setup` 为 `false` |
| 进程版本 | 与合并后的 `backend/cmd/server/VERSION` 一致 |
| 启动日志 | 没有 `Admin user created` |
| 数据库 | `users` 大于 0 |

后台地址是 http://localhost:8081。侧栏版本号需要登录后查看；进程版本以 `sub2api -version` 为准。
