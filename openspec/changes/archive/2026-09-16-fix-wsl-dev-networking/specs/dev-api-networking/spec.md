# dev-api-networking

## Purpose

定义开发期前后端网络访问契约：后端默认端口 5100、前端绝对 apiBase 直连 + CORS 白名单、dev server 全接口绑定与 WSL 侧探测卫生约定，消除 Windows 5000 端口保留段冲突（svchost 抢占导致 v4 半开）与 v6-only 监听（WSL mirrored v4 够不着）两类连通性根因。

## ADDED Requirements

### Requirement: 后端默认端口为 5100

后端开发直跑（`go run`/`go build`）与配置文件（`configs/config.yaml`）的默认监听端口 SHALL 为 5100。Docker Compose 的宿主端口映射默认值 SHALL 为 `${PORT:-5100}:5000`（容器内 5000 不变）。用户显式配置的端口（`SERVER_PORT`/`server.port`/`.env` 的 `PORT`）SHALL 继续原样生效。

#### Scenario: 开发直跑默认端口
- **WHEN** 开发者在 backend-go 目录以默认配置启动后端（未设置任何端口环境变量）
- **THEN** 后端监听 `:5100`，`http://localhost:5100/health` 返回 200

#### Scenario: Docker 默认宿主映射
- **WHEN** 用户无 `.env` 执行 `docker compose up`
- **THEN** 宿主机 `5100` 端口映射到 backend 容器内 `5000`，`http://localhost:5100/api/...` 可访问

#### Scenario: 显式端口配置兼容
- **WHEN** 用户设置了 `PORT=6000`（`.env`）或 `SERVER_PORT=6000` 启动后端
- **THEN** 后端按显式配置端口监听，默认值不覆盖用户配置

### Requirement: 前端绝对 apiBase 直连后端

前端 `apiBase` 默认值 SHALL 为绝对地址 `http://localhost:5100/api`：API 请求、WebSocket（`ws://localhost:5100/ws`）与后端静态资源（`/icons/*`）均直连后端 origin，由后端 CORS 白名单放行前端 origin（既有已验证拓扑）。`NUXT_PUBLIC_API_BASE` 显式配置时 SHALL 原样生效（跨域/远程后端）；相对 base（如 `/api`）SHALL 按页面 origin 同源解析（保留给未来同源反代部署）。

#### Scenario: 默认直连
- **WHEN** dev 模式浏览器加载前端（默认配置，未设置 `NUXT_PUBLIC_API_BASE`）
- **THEN** API 请求指向 `http://localhost:5100/api/...`，WebSocket 连 `ws://localhost:5100/ws`，图标等静态资源从 `http://localhost:5100/icons/...` 加载

#### Scenario: 显式地址覆盖
- **WHEN** `NUXT_PUBLIC_API_BASE` 设为 `http://my-backend:5000/api`
- **THEN** API 请求与 WebSocket 均指向 `my-backend:5000`，不做改写

#### Scenario: 相对 base 同源解析
- **WHEN** `NUXT_PUBLIC_API_BASE` 设为相对路径 `/api`
- **THEN** API 请求按页面 origin 同源解析（同源反代部署形态）

### Requirement: 前端 dev server 绑定全部接口

前端 dev server SHALL 显式绑定 `0.0.0.0`：默认 `localhost` 绑定在 Windows 上解析为 `::1` 仅监听 IPv6 环回，WSL2 mirrored 模式的 IPv4 loopback（`127.0.0.1`）无法到达。

#### Scenario: WSL 可达前端 dev server
- **WHEN** dev server 启动后从 WSL 侧访问 `http://127.0.0.1:3000/`
- **THEN** 请求成功（IPv4 全接口监听，非 v6-only）

### Requirement: 开发网络访问口径文档化

开发文档 SHALL 记录：后端默认 5100（浏览器/工具直连 + CORS 白名单）、WSL 侧 `no_proxy=localhost,127.0.0.1,::1` 探测卫生约定（防止系统代理劫持 localhost 探测产生假象）。

#### Scenario: 文档口径一致
- **WHEN** 开发者查阅 AGENTS.md / docs/reference/development.md / configuration.md / deployment.md
- **THEN** dev 模式的口径为「后端默认 5100、前端 dev server 绑 0.0.0.0、绝对 base 直连后端 origin」，无残留「dev 走同源代理 / 5000 端口」的过时指引（同源反代是独立的**部署**形态，见 `deploy/same-origin/`，不属 dev 口径）

### Requirement: 多机访问口径文档化

`docs/reference/deployment.md` SHALL 记录「浏览器与后端不同机」时的必配项与失效症状：`NUXT_PUBLIC_API_BASE` 必须为**浏览器**可达的地址（默认 `http://localhost:5100/api` 里的 `localhost` 指浏览器所在主机，不是后端主机），`CORS_ORIGINS` 必须逐个包含浏览器地址栏中的 origin（精确匹配，无通配回退）；文档 SHALL 给出该形态的两类失败症状（`ERR_CONNECTION_REFUSED` / 响应缺 `Access-Control-Allow-Origin`）并指向同源反代部署作为免配置形态。文档 SHALL NOT 残留代码中已不存在的环境变量名。

#### Scenario: 多机访问口径与症状齐备
- **WHEN** 开发者查阅 `docs/reference/deployment.md`
- **THEN** 存在多机访问小节，含两个必配项、`localhost` 根因说明、两类失败症状与同源反代指引

#### Scenario: 幽灵环境变量零残留
- **WHEN** 对 `docs/reference/` 执行 `grep -rn 'NUXT_PUBLIC_API_ORIGIN'`
- **THEN** 零命中（该变量在代码中已不存在，仅历史归档文档保留）
