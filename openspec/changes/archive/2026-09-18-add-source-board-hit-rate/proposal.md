<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading, semantic-board -->

## Why

用户每天要吃进 700–800 篇新闻（2026-09-17 实测入库 794 篇、9-16 928 篇），但绝大多数不落在自己关心的板块里——真正能用的只有挂上板块的那一部分。系统目前只有正向链路（`feeds → articles → article_topic_tags → topic_tag_board_labels → semantic_labels(board)`），没有任何**反视角聚合**：谁能回答「这个订阅源到底给我喂了多少有用的东西」？

现在只能靠人工翻列表感受，看不到全貌也看不到比例。实测已经证明这个信号一算就出来、而且直接指向可处置的源头（近 7 天）：

| 源 | 近 7 天 | 入板块率 |
| --- | --- | --- |
| 华尔街见闻 - 最热文章 | 67 | 83.6% |
| 掘金人工智能本周最热 | 22 | 81.8% |
| V2EX - 技术 | 163 | 43.6% |
| 资讯_凤凰网 | 492 | 32.5% |
| 华尔街见闻 - 实时快讯 - 要闻 | **1662** | **26.3%** |
| HackerNews | 14 | 7.1% |

一个源一周灌 1662 篇、只有四分之一进板块——这就是典型的杂音源，值得降频或退订；而「最热文章」这种 80%+ 命中的源值得留。现在没有任何界面能让用户看到这件事。

## What Changes

- **新增只读聚合「源 × 板块命中统计」**：按窗口（默认近 7 天，可切 30/90 天）聚合每个订阅源的文章三分解——**入板块** / **有标签但没板块** / **未打标**（未打标再分「打标排队中」与「已处理无标签」），外加该源的板块分布（落到哪些板块、各多少篇）。命中口径 = 文章至少有 1 个标签经 `topic_tag_board_labels` 挂到 `label_type='board' AND status='active'` 的板块。
- **新增板块视角聚合**：某个板块近 N 天的来源构成（哪些源在供血、各占该板块多少篇、占比多少）。
- **设置 → 订阅源两处展示**：列表每项显示「入板块率 + 窗口内篇数」，支持**按入板块率升序 / 按篇数降序 / 默认（分类内标题）**排序与「只看低命中源」筛选（低命中阈值可配，默认 30%）；右栏详情新增「来源质量」块——窗口切换（7/30/90 天）、三分解条、板块分布 chips。
- **`/tags` 板块页「板块内容」tab 底部新增「来源构成」面板**（不新增 tab）：本板块的来源构成——供血源列表（篇数 / 占本板块比例 / 该源自身入板块率），附本板块窗口内文章总数与来源数汇总。
- **明确只读**：本 change 不提供一键降噪动作（不加「调低 max_articles / 关打标 / 退订」按钮），处置仍走现有订阅源编辑入口。
- **口径必须与实现一致的三条硬约束**（写在 spec 里，防回归）：
  1. **分母按文章去重**——一篇文章多标签多板块，直接 `count(*)` 会翻倍虚高（实测把 1662 篇算成 5193 篇）；
  2. **窗口内必须包含已归档文章**——高频源超过 `max_articles` 的部分被 `CleanupOldArticles` 归档（华尔街见闻快讯近 7 天 1671 篇里 1574 篇已归档，其 437 篇命中**全部**在归档区），排除归档会得出完全相反的结论（0% 命中）；
  3. **必须限窗口，不看全量**——老文章打标覆盖只有 15%（全库 20035 篇仅 3074 篇有标签，9-10 之后才接近 100%），全量会把所有老源冤枉成杂音源。

## Capabilities

### New Capabilities
- `source-board-hit-rate`: 源 × 板块命中统计——口径定义（命中判定 / 窗口 / 去重 / 归档计入 / 未打标两分）、两个只读聚合端点（按源、按板块）、设置页与板块页两处展示契约（列表指标 + 排序筛选 + 详情三分解；板块内容 tab 内来源构成面板）。

### Modified Capabilities
- `feed-settings-ui`: 订阅源列表与卡片新增「来源质量」信息面——列表项显示入板块率与窗口篇数、排序与低命中筛选；详情新增窗口切换、三分解与板块分布。既有 toggle / 最大文章数 / 管理入口契约不变。

## Impact

- **后端（新增只读端点，无迁移、无写路径）**：
  - `backend-go/internal/reader/handler/feed_handler.go` —— 新增按源聚合端点（沿用 `GetFeeds` 已有的批量 stats 注入范式，不逐源 N+1 查询）
  - `backend-go/internal/tagmanagement/handler/` + `routes.go` —— 新增按板块聚合端点（与既有 `GET /api/semantic-boards/:id/articles` 同组，复用其板块归属判定）
  - 聚合走原生 SQL（GORM `Raw` + 参数绑定）；只读、无新表、无迁移
- **前端**：
  - `front/app/features/settings/components/FeedMasterList.vue`（列表项指标 + 排序/筛选控件）、`FeedDetailEditor.vue`（来源质量块）、`SettingsSectionFeeds.vue`（数据装配，已 `per_page: 10000` 全量拉取，排序筛选可在前端做）
  - `front/app/features/tags/components/TagsPage.vue`（「板块内容」tab 内、既有 `BoardCompositionPanel` 之后挂载新面板组件；tab 栏不动）
  - `front/app/types/feed.ts`、`front/app/api/feeds.ts`、`front/app/api/`（板块侧）
- **数据/性能**：只读聚合，命中走 `articles(feed_id, pub_date)`、`article_topic_tags(article_id)`、`topic_tag_board_labels(topic_tag_id)` 既有索引；窗口参数白名单（7/30/90），不接受任意天数
- **不做（明确出圈）**：一键降噪动作、自动建议清单、按来源过滤阅读列表、把命中率反馈进订阅源发现推荐（后续独立 change）
