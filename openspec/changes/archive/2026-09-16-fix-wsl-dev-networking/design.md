## Context

现状事实（探索结论，2026-09-14 实测）：

- 前后端都跑 Windows：Nuxt dev = node `:3000`（从 WSL mirrored loopback 稳定可达，14ms 200）；Go 后端 `go run` 临时 exe，`:5000` 被 svchost（WSD，PID 4576）抢占 v4，仅 `[::]:5000` 有效——WSL 两个栈都连不通，浏览器靠 `localhost`→`::1` 解析侥幸可用。
- 前端纯 SPA（`ssr: false`），所有后端请求从浏览器发出：`front/nuxt.config.ts` runtimeConfig `apiBase` 默认 `http://localhost:5000/api`（`NUXT_PUBLIC_API_BASE` 可覆盖）；`front/app/utils/api.ts` 的 `isDev()` 分支在相对 base 时强制回退 `http://localhost:5000/api`（历史补丁，现已无意义）；WS 由 `getApiOrigin().replace(/^http/,'ws') + '/ws'` 构建（`useEventStream.ts` 单例 + `useTagWebSocket.ts`）。
- 后端端口单一来源：`backend-go/internal/platform/config/config.go:117` `viper.SetDefault("server.port", "5000")`。
- Docker 拓扑：`docker-compose.yml` `${PORT:-5000}:5000`，`.env.example` `PORT=5000`；容器内 Linux 无 svchost 问题，内网仍用 `backend:5000`。
- WSL shell 的 curl 走 Clash（127.0.0.1:7897，经 mirrored 进来），localhost 探测被污染。
- 生产/部署文档（deployment.md）宣称"front 容器代理 API"但**无代码实现**（front/server 只有 fetch-feed.post.ts）。

## Goals / Non- Goals

**Goals:**

- 所有客户端（浏览器/WSL 工具/集成测试）经前端 origin `:3000` 单通道访问 API 与 WS，摆脱对后端端口双栈行为的地狱级依赖。
- 后端挪出 Windows 5000 保留段（WSD 重灾区），Windows 侧自愈（重启不再随机 v4 半开）。
- 显式配置（`PORT`/`SERVER_PORT`/`NUXT_PUBLIC_API_BASE` 绝对值）全部向后兼容。

**Non-Goals:**

- 不治 cmd.exe interop vsock 间歇故障（进程互操作层，升级 WSL / `wsl --shutdown` 缓解，另行处理）。
- 不实现生产 Nitro routeRules 反代（生产容器拓扑由部署侧反代/端口映射解决，本 change 只保证相对 apiBase 在同源部署下可用；Docker 内部 `backend:5000` 与浏览器 `localhost:5100` 直连路径不变）。
- 不改后端 CORS 行为、API 路由、任何业务逻辑。
- 不迁移历史文档（docs/archive、docs/research 旧文里的 5000 表述不动）。

## Decisions

### D1: 端口选 5100，而非继续 5000 + netsh 保留

- **选择**：默认端口改 5100（config.go viper 默认值 + compose 宿主映射默认 + `.env.example`）。
- **理由**：5000 是 WSD/UPnP 固定占用段，svchost 抢占在 Windows 更新/重启后随机复现；`netsh excludedportrange` 持久保留需要管理员操作且动态排除段会漂移，治标。5100 处于常用开发端口段且无系统服务固定占用。
- **备选**：保留 5000 + 每次重启后 netsh 保留（操作性差、易漂移）；换 8787 等高位段（无必要，5100 已避开）。
- **兼容**：容器内端口 5000 不变（Linux 无此问题）；`.env` 已显式设 `PORT` 的用户零影响。

### D2: 前端直连后端（绝对 apiBase），不做同源代理层

- **选择**：apiBase 默认 `http://localhost:5100/api`（绝对直连，后端 CORS 白名单放行前端 origin），API/WS/icons 全走后端 origin。
- **理由**：这是项目已验证数月的既有拓扑，唯一病根是 5000 被 svchost 抢占（D1 已根治）。实测中放弃了同源代理方案：① Nitro `devProxy` 的 h3 语义会剥离挂载点前缀（需 target 拼回，HTTP 可修）；② `proxyRequest` 不处理 WebSocket upgrade（Vite middlewareMode 也不转发），WS 无解；③ 生产容器同样无反代实现，相对 apiBase 到 prod 会 404。代理层三处墙，直连零墙。
- **备选（已实测否决）**：Nitro devProxy（WS 断）、Vite server.proxy（middlewareMode 不接 upgrade）、独立 ws 转发器（多进程多端口）。

### D2b: dev server 显式绑 0.0.0.0

- **选择**：`devServer.host: '0.0.0.0'`。
- **理由**：Nuxt 默认 host `localhost` 在 Windows 解析为 `::1` 只绑 v6 环回——WSL mirrored v4 够不着（实测：多次实例仅 `[::1]:3000` 监听，WSL 127.0.0.1 全 ECONNREFUSED，Windows 浏览器因 Chrome 优先 ::1 无感知）。这是独立于 svchost 的第二个连通性根因。

### D3: api.ts 解析规则保留绝对/相对双分支，删 isDev 嗅探

- **选择**：解析规则简化为"绝对 `http(s)://` 原样 / 相对按页面 origin 同源"；删除 `isDev()` 端口嗅探回退。相对分支保留给未来同源反代部署，默认值是绝对直连（D2）。
- **理由**：旧 isDev 补丁（"相对 base + dev 端口 3000 → 强制回 localhost:5000"）随着端口稳定在 5100 失去意义，留着是第二条隐蔽改写路径。
- **连带**：`vitest.setup.ts` 的 useRuntimeConfig stub 同步为绝对默认值；`FeedIcon.test.ts` 期望恢复后端 origin。

### D4: no_proxy 卫生约定文档化，不写脚本

- **选择**：在 AGENTS.md 开发环境节 + docs/reference/development.md 记录 `no_proxy=localhost,127.0.0.1,::1`（或临时 `curl --noproxy '*'`）；同时更新 ui-verify skill 的网络实测表（新拓扑下 powershell 中转等土办法退役）。
- **理由**：Clash 注入来源不在仓库可控面（shell 全局/系统代理），自动化脚本改环境变量副作用大；约定 + 实测表更新即可消除主要误报源。
- **备选**：往 `~/.bashrc` 写 export（动用户全局环境，越权）。

## Risks / Trade-offs

- [用户已有 `.env` 设 `PORT=5000` 或 `NUXT_PUBLIC_API_BASE=http://localhost:5000/api`] → 兼容不破坏：显式值继续生效；但 svchost 问题依旧——完工汇报给出"建议改 5100 或删掉显式值"的操作指引。
- [dev 代理目标后端未启动] → 现象从"浏览器 CORS/连接失败"变为"3000 返回 502/504"——报错位置一致且更明确；ui-verify 已有"以 opencli open 实测为准"的纪律，不新增风险。
- [5100 也落入 Windows 动态排除段（低概率）] → 文档给一条自检命令（`netsh interface ipv4 show excludedportrange protocol=tcp`）与持久保留逃生口；且 D2 单通道下 WSL 工具不再依赖 5100 的 WSL 可达性，受影响面只剩 Windows 本机浏览器与 Nitro 转发（同机 v4/v6 至少一个可用）。
- [WS 代理在 devProxy 下的成熟度] → http-proxy `ws: true` 是久经考验路径；tasks 里安排 dev 实测（opencli/浏览器 console 验证事件流连通）作为验收步骤；若个别事件类型异常，`SYNTOPICA_DEV_BACKEND` 兜底直连路径仍在（绝对 apiBase 配置）。
- [前端单测锁定旧行为] → `utils/api.test.ts` 同步改写为断言新解析规则（绝对/相对/生产同源三分支）。

## Migration Plan

1. 合并后重启后端（`go run`）→ 监听 5100；重启前端 dev server → devProxy 生效。
2. 浏览器验证 `http://localhost:3000`（Network 面板确认请求走 `:3000/api` + WS `:3000/ws`）。
3. 回滚 = git revert 单提交；若用户本地 `.env` 有显式 `PORT=5000`，回滚后原样可用。

## Open Questions

（无——端口值、代理机制、解析规则均已定；5100 落排除段属运行时自检项，不改变设计。）
