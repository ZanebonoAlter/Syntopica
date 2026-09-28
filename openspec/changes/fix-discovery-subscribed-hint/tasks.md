## 1. 后端订阅状态判定 helper（design D1/D2/D3）

- [x] 1.1  新建 `backend-go/internal/discovery/service/subscription_state.go`：纯函数 `routeFixedPrefix(namespace, path)`（截到首个 `:` 参数段之前、去尾 `/`）、`routeSubscriptionPrefix(baseURL, namespace, path)`（`TrimRight(base,"/")+"/"+ns+fixedPath`）、`feedURLMatchesRoutePrefix(feedURL, prefix)`（相等 / `prefix+"/"` / `prefix+"?"` 三种命中，**禁止裸 HasPrefix**）；只读索引 `LoadSubscriptionIndex(ctx, db)`（一次 `SELECT url FROM feeds`）与 `ExactMatches(url)` / `MatchRouteSubscriptions(prefixes []string) map[string][]string`（**最长前缀优先**消歧，结果按 URL 字典序去重）
- [x] 1.2  单测 `backend-go/internal/discovery/service/subscription_state_test.go`（SQLite，覆盖边界）：无参路径（`/newsflash`）、尾部可选参（`/event/:category?`、`/znews/:section?`）、必填参（`/news/:category/:locale`）、regex 约束段（`/fggz/:category{.+}?`）、裸 `/:id?`（前缀 = `base/ns`）；段边界反例（`/zaobao/znews` MUST NOT 命中 `/zaobao/znews2`）；query 形态（`prefix?x=1`）与尾斜杠；最长前缀消歧（`/gov` vs `/gov/ndrc/fggz`）；原生精确匹配不误伤同 path 的其它 host。断言方式：表驱动 + `require.Equal`/`require.ElementsMatch`

## 2. 后端三处响应补订阅状态（design D4）

- [x] 2.1  `CandidateCatalogService`：`CandidateView` 增 `SubscribedURLs []string \`json:"subscribed_urls"\``；`buildViews` 改用订阅索引——`kind=rsshub` 且有 `RouteID` → `MatchRouteSubscriptions` 取路由级命中地址（同时修正 `Subscribed`），`kind=rss` → `ExactMatches(feed_url)`；空集合序列化为 `[]`；单测扩展 `candidate_catalog_service_test.go`（rsshub 路由级命中：订 `/znews/world` 而 example 是 `/znews/china` → `subscribed=true` 且 `subscribed_urls` 含 world；未订阅 rsshub 保持 false；原生精确仍为 true/false）
- [x] 2.2  `RecommendationService.GetRecommendations`：`RecommendationCard` 增 `Subscribed bool` / `SubscribedURLs []string`；按卡片 `CandidateID` 批量取 `feed_candidates`（`kind`/`route_id`/`feed_url`，禁 N+1，legacy 无候选行保持 false/空），rsshub 用路由级前缀、rss 用精确匹配；单测 `recommendation_service_test.go`（同类「已订阅但 example 不同」断言 + 无候选 legacy 行不崩）
- [x] 2.3  `DiscoveryRunService.GetRun`：`DiscoveryRunItemView` 增 `Subscribed bool` / `SubscribedURLs []string`；SQL 在既有 join 之外补取候选侧路由（`feed_candidates.route_id → rsshub_routes` 的 namespace/path，不改现有展示字段来源），逐条按 kind 判定；单测 `discovery_run_service_test.go`（rsshub 路由级命中、原生候选精确命中、无订阅为空）

## 3. 前端契约与四入口展示（design D5）

- [x] 3.1  契约层：`front/app/types/discovery.ts` 给 `DiscoveryRecommendation` / `DiscoveryRunItem` 增 `subscribed: boolean`、`subscribedUrls: string[]`（`DiscoveryCandidate` 同步补 `subscribedUrls`）；`front/app/api/discovery.ts` normalizer 归一（`Boolean(p.subscribed)`、`Array.isArray(p.subscribed_urls) ? … : []`），保持 snake→camel 惯例
- [x] 3.2  `DiscoveryCard.vue`：头部徽标行新增「已订阅」徽标（`title` 拼已有订阅地址），一键订阅/填参表单原样保留；扩展 `DiscoveryCard.test.ts`（已订阅显示徽标且按钮仍在；未订阅不显示）
- [x] 3.3  `DiscoveryRunCard.vue`：新增「已订阅」徽标，订阅按钮**保留**（去掉「subscribed 时按钮换文本」的旧分支，会话内标记 `store.subscribedRunIds` 与后端字段取或）；同步 `DiscoveryPanel.vue` 传参与 `DiscoveryRunCard.test.ts`（会话标记 / 后端字段各自触发徽标，按钮均在）
- [x] 3.4  `CandidateLibrary.vue`：状态位走修正后的 `subscribed`（RSSHub 条目不再恒「未订阅」）；按钮规则改为 `kind === 'rss' && subscribed` 才隐藏，RSSHub 条目保留入口；扩展 `CandidateLibrary.test.ts`（rsshub 已订阅 → 徽标「已订阅」且按钮可见；原生已订阅 → 无按钮）
- [x] 3.5  `CandidateSubscribeDialog.vue`：新增只读「已有订阅」信息块（列出 `subscribedUrls`）；`kind === 'rsshub'` 时用实时 `finalUrl ∈ subscribedUrls` 锁定提交并说明「该地址已在订阅列表」，未命中但存在其它订阅时不锁定；`kind === 'rss'` 详情已订阅仍走「已订阅」终态（同现状）；扩展 `CandidateSubscribeDialog.test.ts`（同址锁定 / 同路由它参放行 + 信息块可见 / 原生终态不回退）
- [x] 3.6  前端走查前自检：确认四个入口徽标文案统一为「已订阅」、无新增基础组件、无硬编码色值（只用既有 CSS 变量）

## 4. 聚焦 review 修复（2026-09-28）

- [x] 4.1 **High（本 change 引入的死控件）**：`DiscoveryPanel.openRunSubscribe` 去掉「会话内已订阅即早退」——卡片去掉 `v-if` 后该早退让按钮点击无反应，与「路由级命中保留订阅入口」决策相悖；新增面板级回归测试（会话标记后点按钮仍打开弹窗）
- [x] 4.2 **Medium（前后端 URL 编码分叉）**：`front/app/utils/routeParams.ts` 新增与 Go `url.PathEscape` 对齐的 `pathEscape`（保留 `A-Za-z0-9-_.~$&+:=@`，转义 `!'()*,;/?` 等）替换 `encodeURIComponent`——分叉会让同一逻辑地址落成两条 feed，并让「最终地址锁」漏锁；补 `routeParams.test.ts` 逐类断言（实测 Go 输出对齐）
- [x] 4.3 **连带修复（M1 暴露）**：两侧「仍有未填参数」判定由「整串含 `:`」改为「段首 `:` 残留」（后端 `hasUnfilledParamSegment` / 前端 `split('/').some(startsWith(':'))`）——参数值含 `:`（如 `10:00`）此前会被误判为未填必填、地址拼不出来；补前后端各一条用例
- [x] 4.4 **Low（review 建议）**：`store.markSubscribed(candidateId, url?)` 同步候选行 `subscribedUrls`，订阅成功后候选库「已有订阅」提示即时准确（不必等下次列表刷新）

## 5. 文档

<!-- doc-impact: flow, api -->

- [x] 5.1 `docs/reference/flow/discovery.md`「业务约束与不变量」节补一条约束（编号顺延）：**来源订阅状态的展示判定与召回期资格过滤是两套口径**——展示层 RSSHub 按路由级前缀（段边界 + 最长前缀优先）、原生 RSS 按精确地址，只读不建关联；召回期仍按精确 `feeds.url` 过滤（本 change 不改）；路由级命中只提示、不禁用订阅入口，重复保护落在最终地址（弹出/提交前比对）。同时更新「链路设计」中候选库/推荐卡的订阅状态描述（若与现状不符）
- [x] 5.2 `docs/reference/api/discovery.md`：候选列表/详情、推荐卡片、查询结果条目三处响应说明补 `subscribed`（RSSHub 路由级 / 原生精确）与 `subscribed_urls` 字段语义与空值形态
- [ ] 5.3 **归档后**（§12 强制）：在 `flow/discovery.md`「变更溯源」表补本 change 行（`<date>-fix-discovery-subscribed-hint` + 一句话说明）

## 6. 测试

测试分层与落点（为什么这么分）：

- **前缀匹配算法**：纯函数、零 IO，SQLite 即可穷尽边界（段边界、可选参、regex 约束、最长前缀），放 `subscription_state_test.go`。
- **三处视图接线**：SQLite（`testutil.SetupTestDB`）够用——判定只读 `feeds`/`feed_candidates`/`rsshub_routes`，不涉及 pgvector 或 PostgreSQL 特有语义，不需要 testcontainer。
- **前端组件**：Vitest + happy-dom 覆盖四个入口的徽标/入口保留/弹窗锁定三件事；`DiscoveryPanel.test.ts` 既有用例需随 `DiscoveryRunCard` 行为变化同步（原「已订阅 → 按钮换成文本」断言改为「徽标 + 按钮并存」）。
- 回归关注点：候选库既有「入库 ≠ 订阅」断言（新建候选 `subscribed=false`）、原生 RSS 精确匹配用例、推荐/run 既有字段不回归（`candidate_catalog_transfer_test.go` 导出禁区「不含 subscribed」不受影响——导出条目结构未变）。

## 7. 验证

| Scenario | 测试文件 |
| --- | --- |
| RSSHub 候选的订阅状态按路由判定 | backend-go/internal/discovery/service/candidate_catalog_service_test.go |
| 原生 RSS 地址已订阅不再给重复入口 | backend-go/internal/discovery/service/candidate_catalog_service_test.go front/app/features/discovery/components/CandidateLibrary.test.ts |
| 已订阅来源的提示（推荐卡） | backend-go/internal/discovery/service/recommendation_service_test.go front/app/features/discovery/components/DiscoveryCard.test.ts |
| 已订阅来源的提示（查询结果） | backend-go/internal/discovery/service/discovery_run_service_test.go front/app/features/discovery/components/DiscoveryRunCard.test.ts front/app/features/discovery/components/DiscoveryPanel.test.ts |
| 原生与参数订阅（弹窗按最终地址锁定，行为不回退） | front/app/features/discovery/components/CandidateSubscribeDialog.test.ts |
| 前缀/段边界/最长前缀/精确匹配规则 | backend-go/internal/discovery/service/subscription_state_test.go |
| 三处响应字段契约（含空值形态 `[]`） | backend-go/internal/discovery/service/{candidate_catalog,recommendation,discovery_run}_service_test.go |
| 文档口径与字段说明落实 | `grep -n "subscribed_urls" docs/reference/api/discovery.md`；`grep -n "路由级" docs/reference/flow/discovery.md` |
