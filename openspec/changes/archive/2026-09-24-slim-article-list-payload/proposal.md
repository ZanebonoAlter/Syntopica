<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: reading, content-enrichment -->

## Why

远程访问卡顿的主因是**搬运量**，不是算得慢：实测 `GET /api/articles?per_page=20` 响应体 **834 KB**，字段合计 748 KB 里 `content` 349.5 KB + `description` 345.7 KB + `firecrawl_content` 48.2 KB = **743 KB（占字段 99%）**，而列表行只消费 title/link/author/read/favorite；`description` 实测与 `content` 是同一份 HTML（同一条 89.3 KB 逐字节相同）。服务端生成这份响应只要 ~20ms，代价全在传输。

前端又把这份大响应放大了 24 倍/小时：

- `front/app/features/feeds/composables/useAutoRefresh.ts:36` 调 `fetchArticles({ per_page: 10000 })`，被服务端 `maxPerPage=100` **静默截断**成 100 条（1.7 MB 原始 / 312 KB gzip）；
- 同文件 `initialize()` 给**每个 feed 建一个独立 `setInterval`**（实测 DB：23 个 feed @60min + 1 个 @15min），页面加载时这些定时器在毫秒内创建完毕 → **同一分钟内 24 个请求同时触发**。

生产日志实测（单客户端 24h）：

| 观测 | 数值 |
|---|---|
| `/api/articles` 请求数 / 占总流量 | 3475 次 / **91%**（2903 MB of 3197 MB，gzip 口径） |
| 峰值单分钟 | 20–41 次并发、22–44 MB |
| 该接口 p50 / max | **12–31 s** / 59 s（同期 1 KB 小接口仍是 **2 ms**） |
| 8 并发实测 | 每个 1125 KB → **20–21 s**，聚合吞吐仅 ~430 KB/s（单发 ~300 KB/s，管道是硬上限） |

结论：会话 91% 的字节都在搬同一个列表的正文，且每次都是全量重拉同一页。列表投影 + 刷新编排修正的收益是**数量级**（834 KB → 数十 KB），不是零头。

## What Changes

- **BREAKING（内部 API 契约）**：`GET /api/articles` 列表响应改为**窄投影** —— 不再返回 `content`、`firecrawl_content` 与完整 HTML `description`；新增 `excerpt`（≤200 字符纯文本，替换列表层导语用途）。正文与抓取正文由 `GET /api/articles/:id` 提供（**该接口契约不变**；阅读页既有的「先放列表行做首帧、再取详情补全」两段式流程不变，只把首帧导语换成 `excerpt`）。
- `per_page` 超上限不再静默截断：按上限返回 **并记 WARN 日志**（含请求值与路径），让「客户端要 10000 条」在日志里可见。
- 前端消除 `fetchArticles({ per_page: 10000 })`：刷新后只重取**当前视图的当前页**（投影后 ≤ 数十 KB），不做全量重拉、不做增量 diff（见 design D5）。
- 前端把「每个 feed 一个定时器」收敛为**单一刷新调度器**：同相位批量定时器改为按 due 时间依次触发，同一时刻至多一个刷新在跑。
- 前端 normalizer 对大字段缺失容错（`content`/`description`/`firecrawl_content` 转可选 + 空值兜底），使已打开标签页在部署切换期间不崩。

## Capabilities

### New Capabilities
- `article-list-projection`: 文章列表接口的字段投影与分页契约 —— 列表只承载「扫描-选择」所需字段，正文类大字段仅由详情接口提供；`per_page` 上限语义与超限可观测性。

### Modified Capabilities
- `refresh-parallelization`: 新增「刷新后列表更新 MUST NOT 全量重拉」「定时刷新 MUST 单调度器错峰」两条约束（原有两波并行加载行为不变）。
- `reading-article-pane`: 「简介导语段按实质内容呈现」的数据来源改为「详情接口 `description` 权威 + 列表 `excerpt` 过渡」，渲染结构与 guard 条件不变。

## Impact

- **后端**：`backend-go/internal/reader/handler/article_handler.go`（`GetArticles` 及各筛选/排序分支的 `Select` 投影）、`internal/reader/repository/repository.go`、`internal/models/article.go`（序列化面）
- **前端**：`front/app/stores/articles.ts`、`front/app/api/normalizers/article.ts`、`front/app/types`、`front/app/features/feeds/composables/useAutoRefresh.ts`、`useRefreshPolling.ts`、`front/app/features/shell/components/FeedLayoutShell.vue`（`loadArticles`/`hydrateSelectedArticle`）、`front/app/features/articles/components/ArticleContentPreviewPanel.vue`（导语取值）
- **契约**：`/api/articles` 响应字段收敛（单用户应用，唯一消费者是同源 SPA）；`/api/articles/:id` 不变
- **依赖**：无新增依赖；无 DB 迁移
- **部署**：前后端须同批上线（同源单进程形态，见 change `harden-go-same-origin-serving`）
