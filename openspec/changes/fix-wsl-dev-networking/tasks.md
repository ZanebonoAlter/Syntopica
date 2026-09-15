## 1. 后端端口默认值

- [x] 1.1 `backend-go/internal/platform/config/config.go` `viper.SetDefault("server.port", "5000")` → `"5100"`；`docker-compose.yml` `${PORT:-5000}:5000` → `${PORT:-5100}:5000`；`.env.example` `PORT=5000` → `PORT=5100`。验证：`grep -n "5100" backend-go/internal/platform/config/config.go docker-compose.yml .env.example` 三处命中且 `cd backend-go && go build ./...` 绿
- [x] 1.2 排查后端代码/测试硬编码 5000 残留（`rg -n "localhost:5000|:5000" backend-go --type go` 逐条判定改/留）。验证：残留清单逐条给出归属判定，`go test -short ./internal/platform/config/...` 绿

## 2. 前端 devProxy 与 apiBase 默认值

- [x] 2.1 `front/nuxt.config.ts`：`apiBase` 默认 `http://localhost:5100/api`（绝对直连）；新增 `devServer.host: '0.0.0.0'`（修 v6-only 监听，WSL mirrored v4 可达）。验证：`cd front && pnpm lint` 绿
- [x] 2.2 `front/app/utils/api.ts`：删除 `isDev()` 端口嗅探回退；规则简化为"绝对 `http(s)` 原样 / 相对一律同源"（`getApiBaseUrl`/`getApiOrigin` 同构）。验证：`pnpm lint` 绿，文件内无 `5000` 残留

## 3. 前端单测同步

- [x] 3.1 改写 `front/app/utils/api.test.ts`：默认相对 `/api` 同源、绝对 `NUXT_PUBLIC_API_BASE` 原样、相对 base 下 origin=页面 origin 三组断言；删除锁定 localhost:5000 回退的旧断言。验证：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"` 中 utils 相关用例全绿

## 4. 文档

<!-- doc-impact: api, architecture, configuration, deployment -->

- [x] 4.1 `AGENTS.md`（Project Snapshot 的前端 API/WS 地址、开发环境节 no_proxy 约定）、`docs/reference/development.md`、`docs/reference/configuration.md`（端口默认值、`NUXT_PUBLIC_API_BASE`/`SYNTOPICA_DEV_BACKEND`/no_proxy 条目）、`docs/reference/deployment.md`（宿主端口默认 5100、访问口径）。验证：`rg -n "localhost:5000" AGENTS.md docs/reference/` 仅剩兼容/历史说明语境
- [x] 4.2 `docs/reference/api/_conventions.md`、`docs/reference/architecture/overview.md`、`docs/reference/architecture/frontend.md` 的 API base 示例与网络描述；`.agents/skills/ui-verify/references/network-and-navigation.md` 实测表改写为新拓扑（同源 `/api`+`/ws`、后端 5100、powershell 中转等旧土办法标注退役）。验证：`rg -n ":5000" docs/reference .agents/skills/ui-verify` 残留逐条确认归属（兼容说明/历史归档除外）

## 5. 测试

- [x] 5.1 后端影响包：`cd backend-go && go test -short ./internal/platform/config/...` → 期望 0 失败
- [x] 5.2 前端单测全量：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"` → 期望 0 失败（api.test.ts 改写后无连带回归）

## 6. 验证

- [x] 6.1 起新后端实例（WSL 侧 `DEMO_READ_ONLY=1 go run cmd/server/main.go`，默认 5100，不与旧 5000 实例冲突）→ `curl --noproxy '*' -s -o /dev/null -w '%{http_code}' http://localhost:5100/api/health` → 期望 200；输出留证 `evidence/`
- [x] 6.2 重启前端 dev server（新配置 devServer.host 0.0.0.0）→ WSL `curl --noproxy '*' http://127.0.0.1:3000/` → 期望 200（v4 可达，非 v6-only）；Node `WebSocket('ws://127.0.0.1:5100/ws')` open 事件 → 期望连接成功（WS 直连后端）。留证 `evidence/`
- [x] 6.3 兼容路径：`NUXT_PUBLIC_API_BASE` 绝对值行为由 3.1 单测覆盖（custom absolute base 用例）；配置文件/环境变量端口覆盖由 5.1 的 config_test 覆盖（5500 覆盖用例）。验证：单测绿即视为覆盖
- [x] 6.4 `cd backend-go && golangci-lint run ./internal/platform/config/... && go vet ./internal/platform/config/...` → 期望 0 报告；`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck && pnpm build"` → 期望全绿
- [x] 6.5 完工汇报（本报告）

| Scenario | 测试文件 |
|---|---|
| 开发直跑默认端口 | 人工：evidence/（后端 5100 启动 + /health 探测 200） |
| Docker 默认宿主映射 | 人工：docker-compose.yml 端口行 grep（5100:5000） |
| 显式端口配置兼容 | backend-go/internal/platform/config/config_test.go |
| 前端默认直连（API/WS/icons 同 origin 5100） | front/app/utils/api.test.ts + front/app/components/feed/FeedIcon.test.ts + 人工：evidence/（ws://:5100/ws open） |
| 显式地址覆盖 | front/app/utils/api.test.ts（custom absolute base 用例） |
| 相对 base 同源解析 | front/app/utils/api.test.ts（relative base 用例） |
| WSL 可达前端 dev server | 人工：evidence/（WSL v4 探测 :3000 200） |
| 文档口径一致 | 人工：验证节 grep 检查 |
