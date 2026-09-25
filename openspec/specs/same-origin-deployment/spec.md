# same-origin-deployment Specification

## Purpose
定义 Syntopica 的**同源部署形态**：前端产物与后端 API / WebSocket / 图标由同一个入口对浏览器提供服务，浏览器只面对一个 origin。同源消除跨主机部署下「`localhost` 默认值 + CORS 白名单」的双重必配项，适用于浏览器与后端不同机的常驻部署（树莓派 / 任意 Linux 主机）。

受支持形态自 2026-09-24 起收敛为单一：**Go 单进程同域**（后端 `internal/app/static.go` 在进程工作目录下托管 `frontend/` 静态产物，前端静态副本唯一来源 `backend-go/frontend/`）。历史反代形态（`deploy/same-origin/` 的 Caddy/nginx 制品与 `/srv/www` 静态根）已**弃用（不再维护、不再是受支持路径）**，仅作历史参考保留。

## Requirements

### Requirement: 同源构建期注入相对 base

前端静态产物在**构建期** SHALL 注入 `NUXT_PUBLIC_API_BASE=/api`，且 SHALL 支持构建期覆盖（`--build-arg`）。因静态 SPA 的 `runtimeConfig.public` 在构建时内联，产物 SHALL NOT 依赖运行期环境变量确定 API 地址；默认的绝对地址 `http://localhost:5100/api` 会把访问主机名烘死进产物，非本机浏览器打开即失败。

#### Scenario: 镜像内产物不含宿主地址

- **WHEN** 以默认参数构建 `deploy/docker/Dockerfile` 的镜像
- **THEN** 产物中不出现 `http://localhost:5100`，浏览器从任意主机访问该镜像暴露的端口时，请求发往同一 origin 的 `/api/...`

#### Scenario: 构建期可覆盖

- **WHEN** 以 `--build-arg NUXT_PUBLIC_API_BASE=http://api.example.com/api` 构建
- **THEN** 产物内 API 地址为该绝对地址（覆盖生效，仍不读运行期环境变量）

#### Scenario: 手工静态产物可判据

- **WHEN** 在 `front/` 以 `NUXT_PUBLIC_API_BASE=/api` 执行静态产物生成
- **THEN** 产物目录内 `localhost:5100` 零命中，且 `index.html` 存在

### Requirement: 同源部署路径与边界文档化

`docs/reference/deployment.md` SHALL 收录同源部署的**受支持形态**（Go 单进程静态托管：后端进程同时提供前端静态产物、API、WebSocket 与图标）及适用条件，SHALL 说明部署中前端服务的真实形态，并 SHALL NOT 残留已不存在的制品描述（幽灵容器、幽灵 Dockerfile、幽灵环境变量）。基于反代制品的形态（`deploy/same-origin/` 的 Caddy/nginx 路径与 `/srv/www` 静态托管）SHALL 被显式标注为**弃用且不再维护**（保留制品供历史参考，但 MUST NOT 作为推荐路径或被文档引导使用）。`docs/reference/configuration.md` 的 `NUXT_PUBLIC_API_BASE` 条目 SHALL 标明「dev 启动时读取 / 静态构建期内联」两种语义差异。前端静态副本 SHALL 只有一个受维护来源（`backend-go/frontend/`）。

#### Scenario: 部署步骤可复现

- **WHEN** 开发者按 `docs/reference/deployment.md` 的同源小节逐步操作（构建静态产物 → 铺到 `backend-go/frontend/` → 重启后端）
- **THEN** 每一步都有可直接执行的命令与期望结果，最终浏览器访问该端口可见列表页数据

#### Scenario: 文档与实际部署形态一致

- **WHEN** 对 `docs/reference/deployment.md` 检索前端服务描述
- **THEN** 描述与仓库实际制品一致（受支持路径只有 Go 单进程同域；`/srv/www` 与反代制品明确标注弃用），不存在指向幽灵制品或引导使用弃用路径的指引

#### Scenario: 弃用形态可判据

- **WHEN** 在仓库检索 `/srv/www` 与 `deploy/same-origin/`
- **THEN** 命中的文档站点 SHALL 带有「弃用/不再维护」标注，且 `docs/reference/deployment.md` 的推荐路径段落 SHALL NOT 把反代形态列为可选推荐

### Requirement: 静态资源体积与缓存契约

应用自身（Go 单进程静态托管）SHALL 为静态资源返回明确的缓存语义：内容哈希命名资源（`/_nuxt/*`）SHALL 返回 `Cache-Control: public, max-age=31536000, immutable`；`index.html` 与 SPA 兜底响应 SHALL 返回 `Cache-Control: no-cache`（允许协商复用，禁止长缓存）；其余静态文件（纹理/图标/`favicon`）SHALL 至少带可缓存语义与 `Last-Modified`。`favicon` 资源 SHALL ≤ 64 KB，且 MUST NOT 以每次页面加载重下数 MB 级别资源的形态存在。

#### Scenario: 哈希资源长缓存

- **WHEN** 浏览器请求 `/_nuxt/<hash>.js` 或 `/_nuxt/<hash>.css`
- **THEN** 响应 SHALL 含 `Cache-Control: public, max-age=31536000, immutable`，二次访问 SHALL 直接命中浏览器缓存（不产生网络请求）

#### Scenario: HTML 不长缓存

- **WHEN** 浏览器请求 `/`、`/tags` 等 SPA 路由或兜底 HTML
- **THEN** 响应 SHALL 含 `Cache-Control: no-cache`，MUST NOT 含 `max-age` 长缓存语义（保证前端新版本可即时生效）

#### Scenario: favicon 体积与缓存

- **WHEN** 浏览器请求 `/favicon.png`
- **THEN** 响应体积 SHALL ≤ 64 KB，且响应 SHALL 带缓存语义（MUST NOT 为 `no-cache` 且无校验字段）
