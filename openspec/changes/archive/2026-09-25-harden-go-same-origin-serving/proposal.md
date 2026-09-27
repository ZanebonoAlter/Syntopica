<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: scheduler -->

## Why

公网访问（`http://zanebono.top/` → 阿里云 47.110.71.194 → Go 后端）实测有两个"纯损耗"，与业务无关、全是可修的：

1. **全程零压缩**：`Accept-Encoding: gzip, br` 下所有响应仍是 `identity`。实测（浏览器 HAR，冷启动 40 请求 / 2.06 MB / 6.7s）：`entry.css` **640.8 KB**（gzip 后应 286.8 KB）、`entry.js` **295.9 KB**（应 121.1 KB）、`/api/articles` 单次 **1125 KB**（应 312 KB）。原因是公网路径直达 Go 进程（`Server` 头为空、`/api/tasks/status` 返回 Go 侧 500），而 Go 侧**没有任何压缩中间件**——局域网那条 nginx 会压，公网这条裸奔。
2. **静态资源无缓存头**：HAR 40/40 响应都没有 `Cache-Control`；内容哈希命名的 `/_nuxt/*` 每次访问都要重新协商/重下，`favicon.png` 更是 **321.6 KB** + `no-cache`（每次开页面重下 322 KB）。

另有两个"小病灶"：

3. **轮询开销**：三处常驻轮询（`/api/schedulers/status` 8/15/30s 自适应、`/api/tag-queue/status` 60s、`/api/notifications/unread-count` 60s）合计占全部请求 53%（18783 + 15535 + 9806 = 44,124 次）；单客户端实测约 **10 次/分钟**，而且**没有任何 `visibilitychange` 处理**——标签页在后台也照轮询。版块分析期还有 3s/5s 的快轮询。
4. **demo 只读实例 `/api/schedulers/status` 稳定 500**（空 body，连测 3 次复现）：只读模式下调度器未注册，handler 直接报错；而前端**每几秒就轮询它**，500 白占连接配额、也不产生可用状态。

再叠加部署形态的混乱：同一份代码有三个静态副本、两个版本（局域网 nginx 的 `/srv/www` 是 9-19 构建，`backend-go/frontend` 与公网实例是 9-23 构建），"改了看不见"随时可能发生。

用户决策（2026-09-24）：**以后只维护 Go `:5100` 同域单进程形态，nginx + `/srv/www` 那条不再维护。**

## What Changes

- **Go 侧新增响应压缩**：文本类响应（`text/*`、`application/json`、`application/javascript`、`image/svg+xml`）按 `Accept-Encoding` 协商 gzip；已压缩类型（`woff2`/`png`/`jpg`/`webp`）不重复压缩；带 `Vary: Accept-Encoding`；小于阈值（默认 1 KB）不压；压缩独立于任何反代（`/api/*`、`/icons/*`、静态资源一律覆盖）。
- **静态资源缓存契约**：内容哈希资源（`/_nuxt/*`）返回 `Cache-Control: public, max-age=31536000, immutable`；`index.html` 与 SPA 兜底返回 `no-cache`；图标资源拆为「标签图标 64×64 ≈4.5 KB + 空态插画 720×720 ≈29 KB（仅空态加载）」两个文件，合计 322 KB → 约 33 KB（实测：`/favicon.png` 现在是唯一文件同时充当标签图标 / 顶栏 logo / 360px 空态插画）。
- **轮询预算与合并**：新增单一批量对账端点承载"常驻状态类"数据（调度器状态 + 标签队列计数 + 未读计数），前端三处轮询合并为一次；常驻轮询间隔下限 15s、空闲退避、**页面隐藏时暂停、恢复可见时立即刷新一次**。
- **demo 只读模式 `/api/schedulers/status` 返回 200 + 空集合**（不再 500）；`/api/tasks/status` 同源问题一并核查。
- **部署形态收口**：`deploy/same-origin/` 的 Caddy/nginx 反代制品与 `/srv/www` 静态托管标注**弃用**（不再维护、不再是受支持路径），`docs/reference/deployment.md` 与 `deploy/same-origin/README.md` 同步；受支持形态 = Go 单进程同域（`:5100` 托管前端静态产物 + API + WS + 图标）。

## Capabilities

### New Capabilities
- `http-response-compression`: 文本响应压缩协商契约 —— 静态资源与 API 一律按 `Accept-Encoding` 协商压缩，且不依赖反代层。
- `client-poll-budget`: 前端常驻轮询预算契约 —— 合并为单一对账入口、间隔下限与空闲退避、页面隐藏暂停/恢复即刷。

### Modified Capabilities
- `same-origin-deployment`: 新增「静态资源体积与缓存契约」；「同源部署路径与边界文档化」收敛为只维护 Go 单进程同域形态（反代制品弃用）。
- `scheduler-observability`: 新增「只读/降级模式下状态端点 MUST 返回 200 + 空集合」，不得以 5xx 表达"无调度器"。

## Impact

- **后端**：`backend-go/internal/platform/middleware/`（新增压缩中间件）、`internal/app/router.go`（挂载中间件）、`internal/app/static.go`（静态响应头 + favicon）、`internal/admin/handler/`（只读模式状态端点 + 新增批量对账端点）、`cmd/server`/`internal/app/runtime.go`（装配）
- **前端**：`front/app/composables/useSchedulerStatus.ts`、`useTagQueueProgress.ts`、`useNotifications.ts`（合并轮询 + 可见性暂停）、`front/app/api/*`（批量端点客户端）、`front/public/favicon.png` 与新增 `front/public/brand-mark.png`（图标拆分）、`front/app/features/articles/components/ArticleContentView.vue`（空态插画引用改指向）
- **部署/文档**：`docs/reference/deployment.md`、`deploy/same-origin/README.md`（弃用标注）、`docs/reference/configuration.md`（若涉压缩开关）
- **依赖**：无新增第三方依赖（压缩用标准库 `compress/gzip`；brotli 非必需，若需另行评估）
- **兼容性**：压缩与缓存头为增量响应头，不改变响应体语义；批量对账端点为新增，旧轮询端点保留（避免已打开标签页 404）
