<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

树莓派常驻部署（Pi 5 跑后端 + 采集，浏览器在 PC 上）第一次暴露了仓库的**部署面缺口**：

- 前端 `apiBase` 默认 `http://localhost:5100/api`（`front/nuxt.config.ts:18`）
- 后端 CORS 白名单默认只认 `http://localhost:3000`（`backend-go/internal/platform/config/config.go:125`）

**两个 `localhost` 都指「浏览器所在主机」，不是 Pi。** 浏览器打开 Pi 上的前端时 API 全废（`ERR_CONNECTION_REFUSED`）；把 `apiBase` 改成 Pi 地址后立刻撞第二堵墙——实测 `Origin: http://10.11.12.55:3000` 的响应**不含** `Access-Control-Allow-Origin`。于是每次多机部署都要现场配两个环境变量，主机地址写死、随访问地址漂移（换网段/换设备就再废一次）。

`fix-wsl-dev-networking` 的 design.md 已把同源反代明确划归部署侧（Non-Goals：「生产容器拓扑由部署侧反代/端口映射解决」），其 spec `dev-api-networking` 也预留了「相对 base 同源解析」场景且 `front/app/utils/api.ts:26-30` 已实现——**但部署侧那一步从来没做**：仓库里没有任何反代制品，所以那条预留路径至今无人走通。

同时 Pi 上目前用 `pnpm dev` 当常驻前端（实测页面含 `@vite/client`）：HMR + 无压缩 + 常驻 Node 进程，对低内存设备不合适。

**实现前的额外发现（改变了方案形状）**：仓库**本来就有同源部署的代码路径**，只是从未被用起来、且带着同一个 bug：

- `backend-go/internal/app/static.go` 在进程工作目录下托管 `frontend/` 静态产物（含 SPA 兜底），`cmd/server/main.go:106` 已接线；`Dockerfile` 也确实把 `pnpm generate` 的产物 `COPY` 进了镜像的 `/app/frontend/`——**单镜像同源是仓库既有的设计路径**。
- 但 `Dockerfile:8` 的 `RUN pnpm generate` **没带 `NUXT_PUBLIC_API_BASE`**，于是在构建期把绝对地址 `http://localhost:5100/api` 烘进了产物——镜像部署到非本机访问时症状与用户遇到的一模一样。**这是同一个根因的另一出口。**
- 实测 Pi 的 `:5100` 根路径返回 404 → 该 Pi 上后端以裸二进制方式运行、`frontend/` 目录不存在，这条既有路径根本未被走通。
- `docs/reference/deployment.md` 描述的前端服务形态**指向幽灵制品**：不存在的 `front` compose 服务（`docker-compose.yml` 只有 `postgres` + `syntopica` 两服务）、不存在的 `front/Dockerfile`、已废弃的 `NUXT_PUBLIC_API_ORIGIN` 环境变量。

所以本 change 不是「新增一套反代」，而是**把既有同源路径修通 + 为「后端已在跑、不想重建镜像」的场景补一条反代路径 + 纠正文档**。

## What Changes

- **修单镜像同源路径**（一行 bug）：`Dockerfile` 前端构建阶段加 `ARG NUXT_PUBLIC_API_BASE=/api` + `ENV` 后 `pnpm generate`——镜像内产物改走相对 base，浏览器从任意主机访问 `:5100` 都同源；保留 `--build-arg` 覆盖能力。
- 新增 `deploy/same-origin/` 同源反代部署制品（Caddy，非 nginx）：`Caddyfile` + `docker-compose.yml` + `README.md`。单入口提供前端静态产物，`/api/*`、`/ws`、`/icons/*`、`/health` 反代到后端 `127.0.0.1:5100`；适用于后端已在既有方式下运行、不重建镜像的场景。
- 前端以静态 SPA 形态部署（`pnpm generate`），**构建期**注入 `NUXT_PUBLIC_API_BASE=/api`：浏览器只看到一个 origin，跨域请求与 CORS 白名单彻底不参与。
- `docs/reference/deployment.md`：修正前端服务描述（删幽灵 `front` 容器 / `front/Dockerfile` / 架构图旧拓扑，改为实际形态）、新增「前端服务的三种形态」与「同源反代部署（Caddy）」小节；`docs/reference/configuration.md` 的 `NUXT_PUBLIC_API_BASE` 与 `CORS_ORIGINS` 条目补语义差异、Docker 变量表去掉失效的 `FRONT_PORT`/`BACKEND_PORT`。**注**：「多机 / 远程访问（浏览器与后端不同机）」小节由 `fix-wsl-dev-networking` 交付（其 spec 有对应 Requirement），本 change 只引用不重复声明。

## Impact

- **一行代码/bug 修复**：`Dockerfile`（前端构建阶段的 base 注入）——修的是既有生产路径的跨机失效
- **新增制品**：`deploy/same-origin/{Caddyfile,docker-compose.yml,README.md}`
- **文档**：`docs/reference/deployment.md`（含纠正三处幽灵制品描述）、`docs/reference/configuration.md`
- **其余代码零改动**：`front/`、`backend-go/` 不改一行；`apiBase` 默认值与后端 CORS 默认值保持原样（同源形态下二者都不参与）
- **兼容**：单机直连拓扑（浏览器与后端同机，含 Windows 本地开发）完全不受影响——`/api` 相对 base 在本机访问下同样成立，且 dev 模式仍走绝对直连（`fix-wsl-dev-networking` 结论不变）。镜像构建只多一个可覆盖的 ARG
- **不引入**：TLS / 域名（局域网 IP 场景 `auto_https off`；要公网时换域名，Caddy 自动签证书，属后续独立决策）
