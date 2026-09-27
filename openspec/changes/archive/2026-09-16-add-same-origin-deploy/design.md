## Context

现状事实（2026-09-15 实测）：

- **故障现场**：Pi（`10.11.12.55`）上前端 dev server 跑在 `:3000`、后端 `:5100`、PG `:5432`。PC 浏览器打开 `http://10.11.12.55:3000` 后，页面里的 JS 请求 `http://localhost:5100/api/categories` → 浏览器把 `localhost` 解析为 **PC 自己** → `ERR_CONNECTION_REFUSED`。后端本身健康：直连 `http://10.11.12.55:5100/api/categories` 返回 200 且有数据。
- **第二堵墙（实测）**：后端 CORS 只回白名单内的 origin——`Origin: http://localhost:3000` 命中（有 ACAO），`Origin: http://10.11.12.55:3000` / `http://10.11.12.111:3000` 均**无** ACAO 响应头。白名单来源 `backend-go/configs/config.yaml:19-22`（`localhost:3000`、`localhost:3001`）与代码默认值（`config.go:125`）。
- **已有的同源储备**：`front/app/utils/api.ts` 的相对 base 分支（`getApiOrigin()` 返回页面 origin）已实现并被 `front/app/utils/api.test.ts` 覆盖；`fix-wsl-dev-networking/design.md` D2 把同源反代划归部署侧 Non-Goals。
- **devProxy 已实测否决**（同 change `evidence/dev-verification.md`）：「Nitro devProxy → h3 剥离挂载点前缀，WS upgrade 不被处理；Vite server.proxy `ws:true` → Nuxt middlewareMode 下不接 upgrade」——**这是选择独立反代的直接动因，不是重复发明**。
- **后端非 `/api` 路由**（`backend-go/internal/app/router.go`）：`/icons/*filepath`（feed 图标，前端用 `getApiOrigin()` 拼 URL，`FeedIcon.vue:31`）、`/ws`（`useEventStream.ts:76`、`useTagWebSocket`）、`/health`。**这三条必须一起反代**，否则同源下图标 404、事件流断。
- **前端形态**：`ssr: false` 纯 SPA；唯一 server route `front/server/api/fetch-feed.post.ts` 经全仓 grep（`front/app`、`front/server`、`front/tests`，排除构建产物）确认**零调用点**，仅 `front/app/types/feed.ts:6` 注释提及。故静态化无功能损失。
- **仓库已有同源代码路径（实现前发现，改变了方案形状）**：`backend-go/internal/app/static.go` 在进程工作目录下托管 `frontend/` 静态产物 + SPA 兜底（跳过 `/api/`、`/icons`、`/ws`、`/health`），`cmd/server/main.go:106` 已接线；`Dockerfile` 也把 `pnpm generate` 产物 `COPY` 进镜像的 `/app/frontend/`。但 `Dockerfile:8` 的 `RUN pnpm generate` **没带 `NUXT_PUBLIC_API_BASE`** → 构建期把绝对 `http://localhost:5100/api` 烘进产物，镜像跨机访问症状与用户问题同源。实测 Pi 的 `:5100` 根路径返回 404（无 `frontend/` 目录）→ 这条既有路径在该 Pi 上从未走通。
- **文档指向幽灵制品**：`docs/reference/deployment.md` 描述的前端服务是已删除的 `front` compose 服务（`docker-compose.yml` 实际只有 `postgres` + `syntopica` 两服务）与不存在的 `front/Dockerfile`，并仍列着已废弃的 `NUXT_PUBLIC_API_ORIGIN`。

## Goals / Non-Goals

**Goals:**

- 多机部署零跨域配置：浏览器只面对一个 origin，CORS 白名单与 `NUXT_PUBLIC_API_BASE` 主机地址都不再是必配项。
- 制品即插即用：从零到浏览器可用只需「PC 构建 → 拷贝产物 → `docker compose up -d`」，不依赖仓库其余部分在 Pi 上可用（Pi 上不需要 Node 工具链）。
- 低内存友好：Pi 上不再常驻 dev server（HMR/未压缩），改为静态产物 + 轻量反代。

**Non-Goals:**

- 不改 `apiBase` 默认值、不改后端 CORS 默认值（单机直连拓扑仍是默认，见 D5）。
- 不做 TLS / 域名 / 公网暴露方案（局域网 IP 场景；换域名只是 Caddyfile 一行）。
- 不做 CI/CD、镜像构建流水线、systemd 单元（保持手工三步）。
- 不删 `front/server/api/fetch-feed.post.ts`（零调用点但属他人历史资产，删它要独立判据）。
- 不改 `fix-wsl-dev-networking` 的 dev 拓扑结论（dev 仍是绝对直连；本 change 只补部署形态）。

## Decisions

### D0: 先修既有单镜像路径，反代作为补充而非替代

- **选择**：① 修 `Dockerfile` 的构建期 base 注入（一行，让既有的单镜像同源路径真正可用）；② 另提供反代制品服务「后端已在跑、不重建镜像」的场景；③ 纠正文档。三者共存，不是二选一。
- **理由**：单镜像路径是仓库**已有且已接线**的设计（`static.go` + `Dockerfile`），绕开它去新造一套反代等于在仓库里维护两套不认识彼此的同源实现；但它要求重建镜像、改变后端运行方式，对「后端已裸跑着、正在迁库」的现场太重。两条路径的适用条件互斥且都有真实场景，故都保留并在文档里标明边界。
- **为什么不只修 Dockerfile**：用户现场的 Pi 跑的是裸后端（`:5100` 根路径 404 已证），走单镜像要改后端运行方式；先上反代可以不动后端地恢复可用性。
- **为什么不只做反代**：那会把仓库既有的单镜像路径永久留在「带 bug 的幽灵状态」，下一个用 `docker compose up` 部署到非本机的人会踩同一个坑。

### D1: 用 Caddy，不用 nginx

- **选择**：`caddy:2-alpine`，单文件 Caddyfile。
- **理由**：① WebSocket **透明转发**——Caddy 的 `reverse_proxy` 自动处理 `Connection: Upgrade`，nginx 需要 `proxy_http_version 1.1` + `proxy_set_header Upgrade/Connection` 三行样板，漏一行就是静默断流（正是本次故障的同类陷阱）；② `try_files {path} /index.html` 对应的 SPA 兜底在 Caddy 是一行；③ 未来上域名自动签证书，无需另配。
- **备选**：nginx（样板多、易漏 WS 头，无收益）。

### D2: 前端静态 SPA（`nuxt generate`），不跑 Nitro node server

- **选择**：产物取 `.output/public`，容器只挂静态目录，Pi 上不跑 Node。
- **理由**：应用是 `ssr: false` 纯 SPA，静态产物功能等价；唯一 server route 零调用点（Context 已核）；Pi 上省一个常驻 Node 进程（~120MB RSS）与一次 `pnpm install`。
- **备选（否决）**：Nitro node server（`node .output/server/index.mjs`）——多一个进程、多一份内存，唯一收益是被保留的 `fetch-feed` 路由，而它没人调。

### D3: `network_mode: host`

- **选择**：Caddy 容器走 host 网络，`reverse_proxy 127.0.0.1:5100`，Caddyfile 直接绑 `:80`。
- **理由**：Pi 上的后端可能是裸二进制/`go run`，也可能是容器并映射了宿主端口——host 网络下两种情况都是 `127.0.0.1:5100`，制品不需要知道后端怎么起的；同时免掉 `ports:` 映射，也免掉 Linux Docker 无 `host.docker.internal` 别名的坑（Linux 上要写 `extra_hosts: host-gateway`）。
- **代价**：Linux-only（Pi 就是 Linux，无损失）；compose 中**不得**同时写 `ports:`（host 网络下 compose 直接报错，README 标注）。
- **备选（否决）**：bridge + `ports: "80:80"` 映射 → 容器内 `127.0.0.1` 不是宿主，需 `host.docker.internal`，Linux 下还要 `extra_hosts` 兜底。

### D4: 构建期注入 `NUXT_PUBLIC_API_BASE=/api`

- **选择**：PC 上构建时设 `NUXT_PUBLIC_API_BASE=/api`。
- **理由**：静态 SPA 下 `runtimeConfig.public` 在**构建时内联**进客户端产物——运行期给容器设同名环境变量**不会生效**（与 dev 模式不同，dev 是启动时读）。这是本形态最容易踩的坑，README 与 configuration.md 双处标注，并有可执行判据（构建产物内零 `localhost:5100` 命中）。
- **备选（否决）**：构建时用绝对地址——等于把访问主机名烘死进产物，换设备/换网段就废，正是本次故障的形态。

### D5: 默认值不动，同源是叠加形态

- **选择**：`apiBase` 默认仍为 `http://localhost:5100/api`，后端 CORS 默认值不变。
- **理由**：默认值服务的是「浏览器与后端同机」这个主场景（Windows 本地开发即此），改成相对 `/api` 会让本地 dev 的 WS 直连失效（design 已实测：devProxy 路径 WS 过不去）——**修一个部署场景不能回退主场景**。
- **连带**：`docs/reference/deployment.md` 需讲清两种形态的适用边界（本 change 与 `fix-wsl-dev-networking` 的多机口径小节互相引用）。

## Risks / Trade-offs

- [两条同源路径并存导致选择困难] → 文檔单一入口（deployment.md 同源小节）给决策表：镜像已建/愿重建 → 单镜像；后端已在裸跑 → 反代；仅本地开发 → dev 直连。
- [Dockerfile 改成相对 base 后，想“前端与 API 不同 origin”的用户] → 保留 `ARG NUXT_PUBLIC_API_BASE`，`--build-arg` 可覆盖，且默认值仍然是相对（当前文档已不再提供任何依赖绝对 base 的镜像用法）。
- [静态产物与后端版本漂移] → 产物只含前端资产，后端升级不需重新构建前端；但 `/api` 产物**不能**直接用于「前端与 API 不同 origin」形态（反之亦然）——README 用「一份产物绑一种形态」表述。
- [`/icons/*` 漏配 → 图标全裂] → Caddyfile 显式列出四条后端路径（`/api/*`、`/ws`、`/icons/*`、`/health`）；验证节给 `curl` 判据（图标返回 `image/*` 而非 HTML）。
- [host 网络 + `ports:` 混写] → compose 报错；README 显式警告，验证节含 YAML 合法性检查。
- [Pi 上 80 端口被占] → Caddyfile 单点改 `:80` 为其他端口即可；README 给排查命令（`ss -ltnp | grep :80`）。
- [手工三步易漏] → README 把「构建期注入」写成第一条并在验收节给 grep 判据，漏配会当场暴露（产物里搜得到 `localhost:5100`）。

## Migration Plan

1. 修 `Dockerfile` 后重新构建镜像即生效（镜像内前端改走相对 base）；本次不重建也能回滚（单行 ARG）。
2. PC 构建前端静态产物：`cd front && NUXT_PUBLIC_API_BASE=/api pnpm generate`（Windows 走 cmd/pwsh，见 `standard/frontend/testing.md` 跨平台约定）。
3. 打包拷贝 `.output/public` → Pi 的 `deploy/same-origin/www/`。
4. Pi 起容器：`docker compose -f deploy/same-origin/docker-compose.yml up -d`。
5. 浏览器访问 `http://<pi-ip>/`（不带 `:3000`）；旧形态（`:3000` dev server）可并行保留，确认后停掉释放内存。
6. 回滚：`docker compose ... down` + 回到 `pnpm dev` 形态（前端代码零改动，回滚只是不起 Caddy）。

## Open Questions

（无——入口选型、前端形态、网络模式、注入语义均已在 D1–D5 定案。）
