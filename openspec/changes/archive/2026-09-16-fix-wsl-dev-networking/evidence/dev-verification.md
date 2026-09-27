# dev 实测证据（2026-09-15）

## 后端 5100 直连（6.1）

- 后端实例：Windows 侧 `go run cmd/server/main.go`（config.yaml 默认 5100），Gin 日志见运行进程
- `GET http://127.0.0.1:5100/health` → **200** `{"database":"connected","status":"healthy"}`（WSL 侧 Node fetch，v4 直连）
- `GET http://127.0.0.1:5100/api/tasks/status` → **200** `{"data":{"active_tasks":0,...},"success":true}`
- `GET http://127.0.0.1:5100/icons/feeds/__probe__.png` → **404 text（后端 FileServer 应答签名，证明请求到达后端静态服务）**

## 前端 dev server（6.2）

- `devServer.host: '0.0.0.0'` 生效：WSL `127.0.0.1:3000` → **200**（修复前 Nuxt 默认 localhost 只绑 `[::1]:3000`，WSL v4 全部 ECONNREFUSED，netstat 实证见排查记录）
- WS 直连：Node `WebSocket('ws://127.0.0.1:5100/ws')` → **OPEN ✓**

## 排查过程中实测否决的方案（记录留档）

- Nitro `devProxy`：h3 挂载点前缀剥离（`/api/health` → 后端 `/health` 返回了 200 healthy JSON 实锤）；target 拼回前缀后 HTTP 200 可用，但 **WS upgrade 不被处理**（`proxyRequest` 只管 HTTP）→ 否决
- Vite `server.proxy` ws:true：Nuxt middlewareMode 下不接 upgrade 事件，WS 仍 ERROR → 否决
- 结论：直连拓扑（绝对 apiBase 5100 + 后端 CORS 白名单）零代理层，API/WS/icons 全通

## 自动化验证

- 后端：`go build ./...` ✓；`go test -short ./internal/platform/config/...` ✓（含新增 TestServerPortDefaultsTo5100）；`golangci-lint run ./internal/platform/config/...` 0 issues；`go vet` ✓
- 前端：`pnpm lint` 0 errors（5 warning 为既有历史）；`pnpm exec nuxi typecheck` ✓；`pnpm test:unit` **968/968** ✓；`pnpm build` ✓

## 文档口径

- `rg "localhost:5000" AGENTS.md docs/reference/` → 仅剩历史归档/兼容说明语境
- `.agents/skills/ui-verify/references/network-and-navigation.md` 实测表已更新为直连 5100 拓扑
