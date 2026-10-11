
## 发现页「已订阅」判定现状与三个洞

判定现状（2026-10 实测本地库 syntopica，rsshub_base_url=http://47.110.71.194:1200，feeds 25 条）：

1) 候选库：`CandidateCatalogService.buildViews`（backend-go/internal/discovery/service/candidate_catalog_service.go:442-525）只按 `c.FeedURL` 与 `feeds.url` 精确匹配判 `Subscribed`；而 `feed_candidates.feed_url` 对 `kind='rsshub'` 恒为 NULL（本地 3097/3097 全 NULL，实测 `SELECT count(*) FILTER (WHERE kind='rsshub' AND feed_url IS NULL)`），所以 RSSHub 候选永远 `subscribed=false`。`CandidateView.Subscribed` 定义在同文件 :343-344；`GetCandidate`（:420）走 `buildView`→`buildViews`。
2) 推荐卡：`RecommendationService.GetRecommendations`（recommendation_service.go:~130-175）返回 `RecommendationCard`（:115），无任何订阅字段。召回期去重只在 `discovery_recall.go:searchCandidates` SQL（~:519-570，baseURL+example 精确匹配 / accepted route_id 排除）生效，且**只在建卡那一刻**：残留 pending 卡不会被清。实测 oschina route 3003（example=/oschina/event）已订阅 feed 36（http://47.110.71.194:1200/oschina/event，15:59 建），rec 220 于 15:27 建卡仍 pending；zaobao route 2457（example=/zaobao/znews/china）已订阅 feed 34（/zaobao/znews/world）→ example 不匹配，路由永远可再推。
3) 查询 run 结果：`DiscoveryRunItemView`（discovery_run_service.go:984-996）与 `GetRun`（:1008-1093）无 subscribed 字段；前端 run 卡只有会话内 `store.subscribedRunIds` 标记（front/app/stores/discovery.ts:markSubscribed）。
4) 订阅弹窗：`CandidateSubscribeDialog.vue` load() 里 `if (detail.value.subscribed) done.value = true`（:100）——依赖详情 subscribed，rsshub 也判不出。

复用点：`resolveRSSHubBaseURL(db)`（rsshub_config.go，读 ai_settings.rsshub_base_url，缺省常量 DefaultRSSHubBaseURL="https://rsshub.app"）；`buildFeedURL(route, params, baseURL)`（recommendation_service.go:346+）已实现模板填参/可选段 strip（stripOptionalParams/stripBraceConstraints）。

前端落点：front/app/types/discovery.ts（DiscoveryRecommendation :29 / DiscoveryRunItem :283 / DiscoveryCandidate :93）、front/app/api/discovery.ts（normalizeCard :74 / normalizeRunItem :397 / normalizeCandidate :230 / normalizeCandidateDetail :512）、components：DiscoveryCard.vue（徽标行 head，已有 问答/需填参数/一键订阅 徽标）、DiscoveryRunCard.vue（run-card__actions 用 subscribed prop 决定按钮 vs 已订阅文本）、CandidateLibrary.vue（:287-311 状态列 + `v-if="!c.subscribed"` 隐藏订阅按钮）、CandidateSubscribeDialog.vue。

**引用**：backend-go/internal/discovery/service/candidate_catalog_service.go:buildViews、backend-go/internal/discovery/service/recommendation_service.go:GetRecommendations、backend-go/internal/discovery/service/discovery_run_service.go:GetRun、backend-go/internal/discovery/service/discovery_recall.go:searchCandidates、front/app/features/discovery/components/DiscoveryCard.vue、front/app/features/discovery/components/DiscoveryRunCard.vue、front/app/features/discovery/components/CandidateLibrary.vue、front/app/features/discovery/components/CandidateSubscribeDialog.vue

<!-- pinned 2026-09-28T08:47:20Z -->
