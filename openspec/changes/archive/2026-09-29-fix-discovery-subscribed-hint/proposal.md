<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: discovery -->

## Why

发现页对「该来源已经订阅过」的判定既不统一也不完整，用户会在四个入口看到已订阅来源仍挂着「订阅 / 一键订阅」入口，担心重复订阅。实测本地实例：

- **候选源库**：RSSHub 候选的订阅状态只看 `feed_candidates.feed_url`，而该字段对 rsshub 候选恒为 NULL（本地库 3097/3097 全 NULL）→ RSSHub 条目**永远显示「未订阅」**。
- **为你推荐 / 查询结果**：卡片响应里根本没有订阅状态字段；召回期的去重只在「新建推荐那一刻 + 完全同址」生效——实测残留 pending 卡（订阅发生在建卡之后：`oschina /event/:category?` 15:27 建卡、15:59 才订阅）与「同路由不同参数」订阅（`zaobao /znews/:section?` 订了 world，路由 example 是 china）都会继续出现，界面无任何提示。
- **订阅弹窗**：同样依赖候选详情的订阅判定，RSSHub 路径判不出，用户会走到提交才被 409 拦下。

## What Changes

- 后端新增统一的「来源是否已订阅」判定 helper（纯读，不写库、不建关联）：RSSHub 按**路由级**——同一 `namespace + path` 模板下任意已填参数的订阅都算该来源已订阅；原生 RSS 按**精确地址**。
- 三个响应补/修订阅状态字段：候选库列表与详情（修正 rsshub 取值，并附 `subscribed_urls` 已有订阅地址）、推荐卡片 `GET /api/discovery/recommendations`、查询结果条目 `GET /api/discovery/runs/:id`。
- 发现页四个入口显示「已订阅」提示：为你推荐卡、查询结果卡、候选源库行（已有位置）、订阅弹窗（已有位置）；提示附已有订阅地址供对照。
- **订阅入口保留**（用户决策）：路由级命中只提示、不禁用——同一 RSSHub 路由可能还要订别的参数；仅在订阅后不可能产生新地址时（原生 RSS 地址已订阅，行为同现状）才不提供重复入口。
- 订阅弹窗对「当前填参拼出的最终地址已在订阅列表」就地锁定提交并说明（区分「恰好同址」与「同路由已有其它订阅」）。
- **不改**召回期去重 SQL（仍按精确 `feeds.url` 做资格过滤）与数据库结构；不给推荐池新增过滤，也不清理历史 pending 卡。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `feed-candidate-catalog`: 候选条目的独立订阅状态判定改为「RSSHub 按路由级、原生 RSS 按精确地址」，修正 RSSHub 候选恒为未订阅；明确路由级命中不阻止再订阅其它参数。
- `feed-discovery`: 推荐卡片与查询结果条目 SHALL 携带订阅状态并显示「已订阅」提示；路由级命中 MUST NOT 隐藏订阅入口，是否阻止重复提交由订阅动作按最终地址判定。

## Impact

- 后端：`backend-go/internal/discovery/service/` 新增订阅状态 helper；`candidate_catalog_service.go`（`CandidateView` / `buildViews` / 详情）、`recommendation_service.go`（`RecommendationCard` / `GetRecommendations`）、`discovery_run_service.go`（`DiscoveryRunItemView` / `GetRun`）。
- API 响应新增/修正字段：`GET /api/discovery/candidates`、`GET /api/discovery/candidates/:id`（补 `subscribed_urls`）、`GET /api/discovery/recommendations`、`GET /api/discovery/runs/:id`（`subscribed` 为新增，向后兼容；无 BREAKING）。
- 前端：`front/app/types/discovery.ts`、`front/app/api/discovery.ts`（normalizer）、`front/app/features/discovery/components/{DiscoveryCard,DiscoveryRunCard,CandidateLibrary,CandidateSubscribeDialog}.vue`。
- 数据库：无 schema 变更、无迁移、无存量数据改写。
