# Design — slim-article-list-payload

## Context

见 proposal.md 「Why」（实测数据）。补充实现相关的现状约束：

- `GetArticles`（`internal/reader/handler/article_handler.go:65`）在 4 个分支（无筛选 / relevance 排序 / concept/aux 筛选 / DISTINCT 路径）各自 `Select("articles.*, ...")`，其中 `articles.*` 含 `content`/`description`/`firecrawl_content`/`ai_content_summary`；列表行消费的字段只有 title/link/author/read/favorite（`ArticleCardView.vue` 实测只引用 `article.title/link/author/read/favorite`）。
- 阅读页正文与导语目前**首帧来自列表行**：`FeedLayoutShell.vue:204 hydrateSelectedArticle()` 先把列表项赋给 `selectedArticle`，再调 `GET /api/articles/:id` 用返回的完整记录覆盖（`normalizeArticle`）。即「两段式」流程已存在，列表带正文只是首帧填充。
- `ArticleContentPreviewPanel` 的导语是 `v-html="article?.description"`（`ArticleContentPreviewPanel.vue:117`），由 `ArticleContentView` 传入 `previewProps`。
- **`articles.tag_count` 列不可用**：只读实测 24873 行里 `tag_count IS NULL` 有 24259 行（97.5%），与 `article_topic_tags` 实际计数 mismatch 328 行；只有 `article_tagger.go` 与一处迁移在写它。因此列表的 `tag_count` 不能改为直接读该列。
- `article_topic_tags(article_id)` 有两个等价索引（`idx_article_topic_tag_article`、`idx_article_topic_tags_article_id`），现有逐行子查询代价可控。

## Goals / Non-Goals

**Goals**
- 列表响应体积：834 KB → 目标 < 100 KB（20 条，未压缩），且不因筛选/排序分支回退。
- 消除 `per_page=10000` 全量重拉与同分钟 24 连发；刷新后列表更新代价与"当前页"成正比。
- 阅读页正文/导语行为不变（仍由详情提供），只有首帧导语来源变化。

**Non-Goals**
- 不改 `GET /api/articles/:id` 契约；不改阅读页版式/交互结构。
- 不动同族列表接口（`/api/semantic-boards/:id/articles`、话题/日报侧的文章列表）——本次只覆盖实测占 91% 流量的 `/api/articles`；契约在 `article-list-projection` 中定义，后续可按需复用。
- 不做游标分页/增量 diff 协议（投影后当前页代价已足够小，见 D5）。
- 不修 `articles.tag_count` 列的 NULL 回填（独立后续项，见 D8）。
- HTTP 压缩、静态资源缓存头、轮询降频、部署收口 → 见 change `harden-go-same-origin-serving`。

## Decisions

### D1 列表窄投影用后端显式列投影，而非请求参数或仅靠压缩

`GetArticles` 各分支统一改为显式列清单（白名单），正文类字段不进列表 SELECT；`excerpt` 作为派生列在后端生成。

- 备选 A：`?fields=` 请求参数让前端挑字段 —— 否决：默认值仍会是"全量"，危险默认值留在接口上，且前端每个调用点都要重写参数。
- 备选 B：只加 HTTP 压缩（gzip/brotli）不动字段 —— 否决：压缩实测只到 2.5×（1125 KB → 312 KB），投影是 20~50×；两者互补而非替代（压缩在姊妹 change 做）。
- 备选 C：保留 `description`（只砍 content/firecrawl_content）—— 否决：实测 `description` 与 `content` 是同一份 HTML，占 46%，砍了 content 留 description 等于没瘦身。

### D2 `excerpt` 由后端生成（去标签 + 折叠空白 + 截断 200 字符）

- 备选：前端截断 —— 否决：要截断就必须先把完整 HTML 传下来（=继续搬运），且 `v-html` 面更大。
- 备选：不给 excerpt，导语完全等详情 —— 会导致首帧导语空白与布局跳动（阅读页导语段是版式的一部分），实测现有两段式流程能在首帧就给出导语，保留该体验成本仅 ~4 KB/20 条。
- 实现要点：去标签用简单文本抽取（不引入 HTML 解析依赖），实体折叠与空白折叠一并做；源为 `description`，空则回退 `content`。

### D3 正文与完整导语的唯一来源是详情接口（复用既有两段式）

`hydrateSelectedArticle` 已做「列表行首帧 + 详情覆盖」，本 change 只把首帧导语的取值改为「详情 `description` →（未就绪）`excerpt`」。阅读页渲染结构、去重 guard、`displayContent` 取值链不改。

### D4 `per_page` 超限：clamp + WARN，不返回 400

- 备选：400 —— 否决：`/api/feeds` 等端点存在合法的 `per_page=10000`（全量订阅源，响应仅 14.6 KB），统一 400 会连带打破既有流程；本次以「可观测」替代「拒绝」，前端调用点的消除由前端任务负责（spec `refresh-parallelization` 负向节拍兜住）。

### D5 刷新后更新策略：重取当前视图当前页

投影后单页（`per_page=20`）目标 < 100 KB，重取当前页的代价与实现复杂度都远低于增量 diff；同时天然保持选中行与滚动位置（不整表替换）。

- 备选：增量 diff / 游标 —— 否决：需要新增协议与服务端支持，收益在投影后不明显（Denial by measurement：现状 834 KB/页，投影后 < 100 KB）。

### D6 定时器收敛为单调度器串行 + due 时间

把 `initialize()` 的「每 feed 一个 `setInterval`」改为：维护 `{feedId, intervalMinutes, nextDueAt}` 列表，用单个 `setTimeout` 排下一个到期项，刷新完成后重排；同时刻至多一个刷新在跑。

- 备选 A：保留每 feed 定时器 + 随机抖动（jitter）—— 否决：能打散相位但仍是 N 个并发对象，推理与测试成本高，且"同一分钟最多 1 个"无法保证。
- 备选 B：完全交给后端调度（前端不刷新）—— 否决：前端定时刷新是既有产品行为（feed 级 `refreshInterval` 由用户配置），本次只改编排不改语义。
- 注意：后端 refresh 并发限流 semaphore=3（`refresh-parallelization` 既有要求）与前端串行不冲突。

### D7 不变量：列表行渲染与阅读页正文

- 列表行字段集不变（title/author/时间/已读/收藏/状态图标）。
- 阅读页：`displayContent` 在详情返回后 MUST 非空（这是本 change 最容易静默回归的点，进 test-cases 主链路节拍）。

### D8 记录不作为本次范围的技术债

- `articles.tag_count` 97.5% NULL：修它需要回填 + 补齐所有写路径，属于独立 change；本 change 保留逐行子查询（有索引，代价可控），并在 design 留痕避免后续误用该列。

## Risks / Trade-offs

- [风险] 前端仍有消费点依赖"列表带来的 `content`/`description`"，瘦身后静默空白 → **缓解**：实现前先全仓反查消费点（`grep -rn "\.content\b\|\.description\b" front/app`），normalizer 对缺失字段给空值兜底，`ArticleContentPreviewPanel` 加测试断言「详情未就绪 → 渲染 excerpt；详情返回 → 渲染详情 description」。
- [风险] 已打开标签页跑旧 bundle + 新后端（部署切换窗口）→ **缓解**：normalizer 容错（旧 bundle 读不到大字段时渲染空内容而非抛错）；`index.html` 为 `no-cache`，刷新即得新 bundle；单用户同源形态下窗口极短。
- [风险] `excerpt` 去标签实现引入 HTML 解析依赖 → **缓解**：用最小正则/扫描实现（HTML 标签剥离 + 实体折叠），不新增依赖；边界用例见 test-cases 白盒附加。
- [权衡] 列表不再自包含（前端必须依赖详情接口拿正文）→ 换来 10× 以上体积下降；阅读页本就在选中时请求详情（既有无变化）。
- [权衡] 单调度器串行刷新会让最坏情况下"最后一个 feed 的刷新"晚于原来 → 由于原来 24 个并发在慢链路上互相拖到 20–60 s，串行总时长不会更差（实测聚合吞吐 ~430 KB/s 是硬上限）。

## Migration Plan

1. **后端先行**：投影 + WARN 日志上线（`/api/articles` 字段收敛；同时把 `excerpt` 加进响应）。
2. **前端同批**：normalizer 容错 + 导语回退 + 刷新编排改造 + 消除 10000 重拉。
3. **部署形态**：同源单进程（Go `:5100` 托管静态 + API），构建前端静态产物 → 铺 `backend-go/frontend/` → 重启后端（`bash scripts/dev/deploy-frontend.sh`）。
4. **回滚**：回退二进制 + 静态产物即可（无 DB 迁移、无数据写入变化）；回滚后前端旧 bundle 与新后端不兼容的点同样由 normalizer 容错兜住。

## Open Questions

- 同族列表接口（版块/话题/日报侧）是否在后续 change 沿用 `article-list-projection` 的字段集与 `excerpt` 契约？——留待后续评估（本次仅定义契约）。
- `excerpt` 与前端既有的「摘要」概念（`ai_content_summary` 整理稿）在 UI 上是否需要区分文案？——实现阶段按现有导语段文案不变处理，若需要区分另开。
