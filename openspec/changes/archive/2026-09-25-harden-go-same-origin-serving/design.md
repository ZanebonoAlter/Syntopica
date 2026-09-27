# Design — harden-go-same-origin-serving

## Context

见 proposal.md「Why」（实测数据）。实现相关现状：

- 静态托管在 `backend-go/internal/app/static.go`：`spaFallback`（`http.FileServer` 包一层，`/api/`、`/icons`、`/ws`、`/health` 直通）+ `r.Static("/_nuxt", ...)` + `r.StaticFile("/favicon.png", ...)`；**没有任何响应头设置**（无 `Cache-Control`，仅 `http.FileServer` 自带 `Last-Modified`/条件请求）。
- `internal/app/router.go` 只挂了 `middleware.ReadOnly()`，**无压缩相关中间件**（`grep -n 'gzip' backend-go/internal/app/router.go` 零命中）。
- 公网路径直达 Go（无 nginx 参与），因此压缩/缓存头必须在应用内实现，不能靠反代。
- 轮询现状（实测）：`useSchedulerStatus` 自适应 8s/15s/30s（`useSchedulerStatus.ts:56-62`）、`useTagQueueProgress` 60s、`useNotifications` 60s；**全仓无 `document.hidden`/`visibilityState` 处理**（`grep -rn 'visibilityState|document.hidden' front/app` 零命中）。
- 只读 demo 模式下调度器未注册：实测公网实例 `/api/schedulers/status` 与 `/api/tasks/status` 均返回 **500 空 body**，而本地（正常模式）为 200。
- 弃用形态的制品：`deploy/same-origin/`（Caddy/nginx + `install-nginx.sh`）、`/srv/www`（9-19 旧构建）、`docs/reference/deployment.md` 的同源反代小节。

## Goals / Non-Goals

**Goals**
- 公网路径的文本响应字节数下降 ≥ 2.5×（实测基线：`entry.css` 640.8 KB、`entry.js` 295.9 KB、`/api/articles` 1125 KB）。
- 内容哈希静态资源二次访问零网络请求；`favicon` ≥ 5× 瘦身。
- 常驻轮询请求量下降 ≥ 3×（合并 + 下限），后台标签页零轮询。
- 只读模式状态端点不再 5xx。
- 部署形态在文档上收敛为单一受支持路径。

**Non-Goals**
- 不做 brotli（工具链与收益权衡，见 D7）、不做 HTTP/2/TLS 终止层（需证书与独立入口，另开）。
- 不改 `/api/articles` 列表字段（见 change `slim-article-list-payload`）。
- 不删弃用制品本身（保留历史参考，只做标注与文档收口）。
- 不动 WS 事件流协议本身（仅确保压缩不干扰）。

## Decisions

### D1 压缩用自写中间件（标准库 `compress/gzip`），不引入新依赖

覆盖路径：静态资源 + `/api/*` + `/icons/*` + `/health`。

- 备选 A：`gin-contrib/gzip` —— 否决：多一个依赖，且对流式端点与已压缩类型的控制粒度不足（AGENTS.md：除非要求不新增工具/依赖）。
- 备选 B：只在静态资源上压 —— 否决：实测 `/api/articles` 1125 KB 是最大单笔（列表瘦身在姊妹 change，但压缩对两者互补）。
- 实现要点（必须落实，否则出问题）：
  1. **跳过 WebSocket 升级请求**（`Upgrade: websocket`）与非 HTTP 正文响应；
  2. **跳过流式端点**（AI 流式输出/SSE 类，路径白名单），响应缓冲策略 MUST NOT 破坏"逐块到达"语义；
  3. **跳过已压缩 MIME**（`woff2/png/jpg/webp/gif/zip/gz`）与已带 `Content-Encoding` 的响应；
  4. **阈值**：小于 1 KB 不压（对静态文件可按 `Content-Length` 预判；对动态响应按缓冲后大小判断）；
  5. **`Vary: Accept-Encoding` 必加**（含未压缩分支，避免缓存污染）；
  6. 压缩后 MUST NOT 保留原 `Content-Length`（改为分块或重算）。

### D2 静态缓存头在应用内设置（按路径分档）

- `/_nuxt/*`（内容哈希）→ `Cache-Control: public, max-age=31536000, immutable`；
- `index.html` 与 SPA 兜底 → `Cache-Control: no-cache`；
- 其他静态文件（`textures/`、`robots.txt` 等）→ `public, max-age=86400`（保守），并保留 `Last-Modified` 条件请求；
- `favicon` → 见 D3，同样带缓存头。

### D3 `favicon.png` 拆成「标签图标 + 版式插画」两个文件（实测定档）

实测发现 `/favicon.png` 现在**身兼三职**：标签图标（`useAnalysisPauseFavicon` 回退 href）、顶栏 logo（`AppHeaderView.vue:201`，`width=32`）、**阅读页空态插画（`ArticleContentView.vue:130`，`width=360`）**。只缩成 32×32 会把空态插画（含 retina 2x）弄糊，因此拆开：

| 文件 | 尺寸 | 实测体积 | 用途 | 引用变更 |
|---|---|---|---|---|
| `front/public/favicon.png`（原地替换） | 64×64 | **4.5 KB**（量化 128 色可到 1.8 KB） | 标签图标 + 顶栏 logo（32 CSS px @2x） | 无（`index.html` 的 `data-hid="app-favicon"` 与 `useAnalysisPauseFavicon` 回退 href 均不变） |
| `front/public/brand-mark.png`（新增） | 720×720（量化 128 色） | **29.1 KB** | 空态插画（360 CSS px，retina 安全） | 只改 `ArticleContentView.vue:130` 一处 `<img src>` |

合计 322 KB → **约 33 KB（↓ ~10×）**，且插画只在空态（未选中文章）渲染时才请求，常见路径零下载。

- 备选 A：保留大图 + 只加缓存头 —— 否决：首屏仍多 300 KB。
- 备选 B：单文件缩到 360×360 —— 否决：360 CSS px 在高分屏下要 720 物理像素，会糊；且标签图标仍要多下一个 20~60 KB 的文件。
- 实现手段：**本机已具备 `python3 + PIL 11.1.0`**（实测 `import PIL` 通过），一次性生成后产物入库；`uv`（`~/.pi/agent/bin/uv`）仅作「某台机器连 PIL 都没有」时的可选兜底，**不是部署期要求**。
- 部署链路零图像工具：`front/public/*` 被 `pnpm generate` 原样拷进 `.output/public/`，镜像构建 `COPY --from=front-build /app/.output/public/ /app/frontend/`，`backend-go/frontend/` 是 gitignore 的构建产物 —— 独立部署（demo 镜像 / rsync 推送）**只拷贝文件**。
- 量化色数按目视验收定档（128 色 ≈ 29 KB 为起点）。

### D4 新增单一批量对账端点 `GET /api/poll`

一次返回三类常驻状态：调度器状态（复用既有查询）、标签队列计数、未读计数；响应结构：
`{ success, data: { schedulers: {...既有结构...}, tag_queue: {pending, processing, completed, failed, total}, notifications: {unread} }, server_time }`。

- 备选：把三类合成"前端只看 WS 推送" —— 否决：WS 是事件流（推送不保证覆盖冷启动与丢失补偿），对账语义需要一次拉取。
- 备选：路径名 `/api/status/overview` —— 采用 `/api/poll`（短、语义=一次对账）；路径名不进入 spec 的行为契约（spec 只约定"单一入口 + 三个分项端点保留"）。
- 保留三个旧端点，避免已打开旧标签页请求 404（spec 负向节拍）。

### D5 前端轮询合并为单例调度器 + 可见性暂停

新增单例（模块级，`usePollBundle`）：唯一 timer + 自适应间隔（空闲 ≥ 30s / 近期反馈 ≥ 15s / 热态 ≥ 5s）+ 失败退避；`visibilitychange` → hidden 暂停、visible 立即对账一次；结果分发到既有三处 state，消费组件（顶栏、芯片、角标）不改 props/DOM。

- 备选：分别保留三个 composable 的 timer，只把 URL 换成合并端点 —— 否决：仍会有三套相位，请求数虽降但不满足"单页 ≤ 4 次/分钟"的可判定性。

### D6 只读模式状态端点返回 200 + 空集合

在调度器未注册的模式下，`/api/schedulers/status` 与 `/api/tasks/status` 返回 HTTP 200 与结构合法的空集合（`data: []` / `{active_tasks:0, queue_size:0, tasks:[]}`）。实现时先定位 500 的确切成因（registry 为空时的类型断言或 nil 解引用），修在 handler 层而非"仅 demo 特判"，使"无调度器"成为合法状态。

### D7 明确不做与理由

- **brotli**：需引入第三方库或外部工具链，相对 gzip 的额外收益（约 15–20%）不足以支撑本次范围；记录为后续可选。
- **HTTP/2**：nginx 弃用后唯一入口是明文 HTTP/1.1（无 TLS），HTTP/2 需要 TLS 终止或 h2c 支持；属独立基础设施议题（并需要证书运维），另开。

## Risks / Trade-offs

- [风险] 压缩缓冲破坏流式响应（AI 流式摘要逐字到达）→ **缓解**：流式路径白名单跳过压缩 + 回归测试断言分块到达 + WS 直接跳过升级请求。
- [风险] 漏加 `Vary` 造成中间缓存把 gzip 版本发给不支持压缩的客户端 → **缓解**：中间件在压缩与未压缩两个分支统一写 `Vary`，测试断言。
- [风险] 合并端点成为单点：一次失败影响三类显示 → **缓解**：spec 要求失败保留旧值 + 退避；旧端点保留。
- [风险] favicon 拆文件后新插画文件漏改引用 → **缓解**：tasks 3.2/3.3 明确只改 `ArticleContentView.vue:130` 一处，并以组件测试/人工空态截图验证；`grep -rn '/favicon.png' front/app` 判据兜底（应只剩顶栏与 favicon composable）
- [权衡] 单一批量端点让"调度器状态"与"未读计数"耦合在一次请求里 → 换来 3× 请求量下降与统一节奏；两者都是只读计数，耦合代价低。
- [权衡] 后台暂停轮询会让角标在回到前台前滞后 → 恰好符合"回来即准"（恢复可见立即对账），且避免无意义流量。

## Migration Plan

1. 后端：压缩中间件 + 静态缓存头 + favicon 替换 + 只读模式状态端点修复 + `/api/poll` 端点；`golangci-lint`/影响包测试通过。
2. 前端：合并轮询（单例 + 可见性）→ 指向 `/api/poll`，三处消费点不变。
3. 部署：静态产物铺 `backend-go/frontend/` → 重启后端（`bash scripts/dev/deploy-frontend.sh`）；验证 `curl -H 'Accept-Encoding: gzip' -D -` 与二次加载命中缓存。
4. 文档：`docs/reference/deployment.md` 收口（弃用标注 + 受支持形态），`deploy/same-origin/README.md` 顶部加弃用横幅。
5. **回滚**：回退二进制 + 静态产物（含复原 favicon 原图）；无 DB 迁移、无数据写入变化。

## Open Questions

- `favicon` 缩放后是否需要同时提供 `.ico`（旧浏览器）？—— 当前 `index.html` 只引用 `favicon.png`，实现时以"不改引用路径"为准，如需 `.ico` 另开。
- `/api/poll` 是否也承载"分析暂停状态/健康态"（顶栏目前从 `/schedulers/status` 顶层取）？—— 采用：承接既有顶层语义（`analysis_paused`/`ai_healthy`），保持消费点零改动。
