<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

WSL2 侧（pi agent / 工具链 / 集成测试）访问本地后端长期不稳定。2026-09-14 实测定位三层根因：

1. **Windows 5000 端口被 svchost（WSD 系统服务）抢占 v4**：`0.0.0.0:5000` 上同时挂着 svchost（PID 4576）与 Go 后端（仅 `[::]:5000` 有效）。WSL→`127.0.0.1:5000` 命中死 socket 超时（半开），`[::1]:5000` 被拒；浏览器因 Windows `localhost`→`::1` 解析侥幸可用。`ui-verify/references/network-and-navigation.md` 2026-09-03 已记录同现象（"后端重启后仅监听 IPv6、v4 半开"），一直靠 powershell.exe 中转等土办法绕行。
2. **WSL shell 的 curl 被 Clash 代理（127.0.0.1:7897）劫持**，localhost 探测结论被污染（超时/502 假象）。
3. cmd.exe interop vsock 间歇故障（`UtilAcceptVsock`，events.db 2026-09-12 两条 interop-down）——进程互操作层问题，本 change 不治但不再加重依赖。

实测 WSL→Windows `:3000`（Nuxt dev）14ms 稳定 200。方案：后端挪出 5000 保留段 + 前端 dev 统一同源代理，把所有客户端（浏览器/WSL 工具/集成测试）收敛到 `:3000` 单通道。

## What Changes

- **后端默认端口 5000 → 5100**（`config.go` viper 默认值、`docker-compose.yml` 宿主映射默认值、`.env.example`）。容器内端口不变（Linux 无 svchost 问题）。**BREAKING**（开发环境）：后端 API 地址从 `localhost:5000` 变为 `localhost:5100`，需重启后端进程。
- **前端 dev 同源代理**：`nuxt.config.ts` 增加 Nitro `devProxy`——`/api` 与 `/ws`（含 WebSocket 升级）转发到后端（目标可用 `SYNTOPICA_DEV_BACKEND` 环境变量覆盖，默认 `http://localhost:5100`）。
- **apiBase 默认改相对路径 `/api`**：`runtimeConfig.public.apiBase` 默认值从 `http://localhost:5000/api` 改为 `/api`；`utils/api.ts` 删除 `isDev()` 强制回退 `localhost:5000` 的补丁逻辑（相对 base 一律同源解析，dev 经 devProxy、生产经同源反代/Docker 拓扑）。WS（`useEventStream`）随 `getApiOrigin()` 同源化自动走 `ws://<前端origin>/ws`。
- **代理目标可配置**：WSL/自定义拓扑下 `SYNTOPICA_DEV_BACKEND` 指向实际后端。
- **文档与口径更新**：AGENTS.md、docs/reference/（configuration/deployment/development/api 约定/architecture）、`.agents/skills/ui-verify/references/network-and-navigation.md` 的端口与访问方式表述；新增 WSL 侧 `no_proxy=localhost,127.0.0.1,::1` 卫生约定（防 Clash 污染 localhost 探测）。
- 前端单测 `utils/api.test.ts` 同步改写（旧断言锁定 localhost:5000 回退行为）。

## Capabilities

### New Capabilities
- `dev-api-networking`: 开发期前后端网络拓扑契约——前端同源 `/api`+`/ws` 代理、后端默认端口 5100、代理目标环境变量覆盖、WSL 探测卫生约定。

### Modified Capabilities

（无——现有 specs 无一覆盖 dev 网络拓扑；`configuration-docs` 只管文档准确性，其 delta 由 doc 更新任务覆盖，不改其 requirement 集合。）

## Impact

- **后端**：`backend-go/internal/platform/config/config.go`（默认端口）；无 API 路由/行为变化。
- **前端**：`front/nuxt.config.ts`（devProxy + apiBase 默认值）、`front/app/utils/api.ts`（同源解析简化）、`front/app/utils/api.test.ts`；`useEventStream`/`useTagWebSocket` 无需改码（随 origin 同源化）。
- **部署**：`docker-compose.yml` 宿主映射默认 `${PORT:-5100}:5000`、`.env.example` `PORT=5100`；容器内 5000 不变，已有 `.env` 用户不受影响（显式 PORT 继续生效）。
- **文档**：AGENTS.md、docs/reference/configuration.md、deployment.md、development.md、api/_conventions.md、architecture/overview.md、architecture/frontend.md、ui-verify skill references。
- **风险**：旧 `NUXT_PUBLIC_API_BASE` 绝对值（若用户 .env 设过）继续按绝对值生效，不破坏；dev 中浏览器不再直连后端端口，CORS 在 dev 语境消失（后端 CORS 配置不动）。
