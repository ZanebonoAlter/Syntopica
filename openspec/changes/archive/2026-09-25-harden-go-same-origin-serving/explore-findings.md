
## 公网路径实测：零压缩/无缓存头/吞吐上限/只读 500/轮询清单

**公网部署形态（实测 2026-09-24）**
- `http://zanebono.top/` → DNS 解析 **47.110.71.194（阿里云杭州）**，仅 80 端口、无 TLS/HTTP2/CDN（`Server` 头为空 → 直连 Go 进程；`https://` 443 拒绝连接）。
- 该实例**不是本机树莓派**：同请求 `/api/articles?per_page=20` 公网 259.7 KB vs 本机 525 KB；`/api/tasks/status` 公网 500 / 本机 200 → 另一实例 + 另一份库（疑 demo 脱敏 seed）。
- **零压缩**：带 `Accept-Encoding: gzip, br` 时所有响应仍 `identity`。浏览器 HAR（冷启动）：40 请求 / **2.06 MB** / 6.7s；`/_nuxt/entry.DBijpxhp.css` 640.8 KB、`/_nuxt/B2RVEoLI.js` 295.9 KB、`/api/articles?per_page=20` 259.7 KB、`/favicon.png` 321.6 KB。
- **无缓存头**：HAR 40/40 响应无 `Cache-Control`。
- **吞吐**：单发 1125 KB/3.7s（≈300 KB/s）；8 并发每个 20–21s、聚合 ≈430 KB/s（**~3.5 Mbps 硬上限，并发只会摊薄**）；24 并发 290s 未跑完。
- `/api/schedulers/status` 与 `/api/tasks/status` 公网稳定 **500 空 body**（连测 3 次）——只读/无调度器模式下 500；该端点前端每几秒轮询（日志 18,783 次）。

**服务端代码位置**
- 静态托管：`backend-go/internal/app/static.go`（`spaFallback` + `r.Static("/_nuxt")` + `r.StaticFile("/favicon.png")`，**无任何 Cache-Control**；Go `http.FileServer` 仅提供 `Last-Modified`/条件请求）。
- 无压缩中间件：`grep -n 'gzip' backend-go/internal/app/router.go` 零命中；router 只挂 `middleware.ReadOnly()`。
- 局域网 nginx 那条路**会** gzip（`deploy/same-origin/nginx.static.conf`），但用户已决定弃用该形态 → 压缩必须在应用内实现。

**轮询清单（前端）**
| 轮询点 | 间隔 | 备注 |
|---|---|---|
| `useSchedulerStatus.ts:56-62` | 8s/15s/30s 自适应 | 顶栏常驻 |
| `useTagQueueProgress.ts:15` | 60s | 芯片 + WS 事件驱动 |
| `useNotifications.ts:23` | 60s | 角标 + WS 事件 |
| `useRefreshPolling`（constants.ts:9 `REFRESH_POLLING_INTERVAL`） | 2s | 仅刷新进行中 |
| `useBoardEnrichment.ts:490/996` | 3s / 5s | 版块分析中 |
| `useHealthReprobe.ts:13` | 2s | 重探期间 |
- **全仓无 `document.hidden`/`visibilityState` 处理**（grep 零命中）→ 后台标签页照常轮询。
- 三项常驻轮询合计占全部请求 53%（18783 + 15535 + 9806 = 44,124 次）；单客户端实测 ≈10 次/分钟。

**部署副本现状**：`/srv/www`（nginx 静态根，**9-19 构建**，entry 哈希 `HsEHUZ54.css`/`BlO3utiI.js`）与 `backend-go/frontend`（**9-23 构建**，`DBijpxhp.css`/`B2RVEoLI.js`，公网实例同款）不一致 → 两个版本三份副本；`/srv/www/_nuxt` 137 个 js/css 共 14.87 MB，`backend-go/frontend` 47 MB 全量。

**引用**：backend-go/internal/app/static.go、backend-go/internal/app/router.go、front/app/composables/useSchedulerStatus.ts、front/app/composables/useTagQueueProgress.ts、front/app/composables/useNotifications.ts、deploy/same-origin/nginx.static.conf

<!-- pinned 2026-09-24T08:19:36Z -->

## 补充确认：zanebono.top = 脱敏 demo 实例（非本机 Pi）；favicon 实为三用拆两文件

**公网实例身份确认（用户 2026-09-24 答复）**：`http://zanebono.top/` 是**脱敏 demo 实例**（非本机树莓派）→ 前一条 finding 里「疑 demo 脱敏 seed」已坐实。推论：
- 该实例 `/api/articles` payload 只有本机一半（259.7 KB vs 525 KB/20 条）是脱敏 seed 数据的结果，不是代码版本差异（两者入口哈希相同，同为 9-23 构建）。
- 实测 196–362 KB/s（8 并发聚合 ≈430 KB/s）是**那台 demo 宿主（阿里云 47.110.71.194）的带宽档位**，与树莓派无关；树莓派自身日常走 ZeroTier/局域网（局域网路径有 nginx gzip，公网 demo 路径没有）。
- demo 模式（`DEMO_READ_ONLY=1`）下调度器未注册 → `/api/schedulers/status`、`/api/tasks/status` 返回 500，是本 change 要修的契约。

**`/favicon.png` 实为三用（决定了图标方案）**
- 源文件：`front/public/favicon.png` = **1254×1254 RGBA / 321.6 KB**（git 已跟踪；`backend-go/frontend/` 是 .gitignore 的构建产物）。
- 三处引用：① 标签图标（`front/app/composables/useAnalysisPauseFavicon.ts:32` 回退 href `/favicon.png`）；② 顶栏 logo `AppHeaderView.vue:201`（`width=32 height=32`）；③ **阅读页空态插画 `ArticleContentView.vue:130`（`width=360 height=360`，未选中文章时渲染）**。
- 因此只缩到 32×32 会把空态插画（含 2x）弄糊 → 拆两文件（PIL 实测体积）：`favicon.png` 64×64 = 4.5 KB（可用 128 色量化到 1.8 KB）；新增 `brand-mark.png` 720×720 量化 128 色 = **29.1 KB**（全彩 163 KB）。合计 322 KB → ≈33 KB。
- 工具链事实：本机**无** ImageMagick/pngquant/cwebp，但**有 `python3 + PIL 11.1.0`**（实测可用）；`uv` 在 `~/.pi/agent/bin/uv`。生成是一次性动作、产物入库；构建链路（`pnpm generate` → `.output/public/` → 镜像 `COPY` / `deploy-frontend.sh` 拷贝）**不调用任何图像工具**，故独立部署与 uv/ImageMagick 是否存在无关。

**引用**：front/public/favicon.png、front/app/composables/useAnalysisPauseFavicon.ts:32、front/app/features/shell/components/AppHeaderView.vue:201、front/app/features/articles/components/ArticleContentView.vue:130、.gitignore:74

<!-- pinned 2026-09-24T08:39:03Z -->
