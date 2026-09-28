# 审查 brief：fix-discovery-subscribed-hint（全量增量，2026-09-28）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令（允许 read/grep/find/ls 与只读验证如 `go vet`/`gofmt -l`/`go build`；不要求跑测试；优先 grep 与精读）。

## 一、审查对象（本轮增量，未 commit，全在工作树）

**背景**：发现页四个入口对「该来源已订阅」判定不一致（详见 `proposal.md` Why）：
- 候选库 RSSHub 候选订阅状态只看 `feed_candidates.feed_url`，而该字段对 rsshub 行恒 NULL → 永远「未订阅」；
- 推荐卡 / 查询结果条目响应无订阅字段；召回期精确 URL 去重只在建卡那刻生效 + 参数不同不匹配；
- 订阅弹窗依赖候选详情判定，RSSHub 路径判不出。

**用户决策（不可当问题报）**：路由级命中**只提示、不禁用订阅入口**；原生 RSS 精确已订阅才不给重复入口。

**本轮改动清单**：

| 任务 | 辖区代码 |
| --- | --- |
| 1.1–1.2 判定 helper | `backend-go/internal/discovery/service/subscription_state.go`（新增：`routeFixedPrefix`/`routeSubscriptionPrefix`/`feedURLMatchesRoutePrefix`/`SubscriptionIndex`/`LoadSubscriptionIndex`/`ExactMatches`/`MatchRouteSubscriptions`/`resolveCandidateSubscriptions`/`uniqueSorted`）+ `subscription_state_test.go` |
| 2.1 候选库 | `candidate_catalog_service.go`：`CandidateView` 增 `SubscribedURLs`；`buildViews` 改用订阅索引（rsshub 路由级 / rss 精确）+ `candidate_catalog_service_test.go` |
| 2.2 推荐卡 | `recommendation_service.go`：`RecommendationCard` 增 `Subscribed`/`SubscribedURLs`；新增 `attachSubscriptionState` + `recommendation_service_test.go` |
| 2.3 run 条目 | `discovery_run_service.go`：`DiscoveryRunItemView` 增两字段；`GetRun` SQL 补 `fc.kind`/`fc.route_id → cr`（展示字段来源不变）+ `discovery_run_service_test.go` |
| 3.x 前端 | `front/app/types/discovery.ts`、`front/app/api/discovery.ts`（normalizer）；`DiscoveryCard.vue`（徽标+title）、`DiscoveryRunCard.vue`（徽标+按钮保留）、`CandidateLibrary.vue`（按钮规则 `kind==='rss' && subscribed` 才隐藏）、`CandidateSubscribeDialog.vue`（已有订阅信息块 + `finalUrl ∈ subscribedUrls` 锁定）；组件测试与 fixture 同步 |
| 4.1–4.2 文档 | `docs/reference/flow/discovery.md`（业务约束 25 + 链路设计一句 + 代码入口一行）、`docs/reference/api/discovery.md`（三处响应字段说明 + JSON 示例） |

设计权威源：`openspec/changes/fix-discovery-subscribed-hint/design.md`（D1–D6、Risks、Non-Goals）。
验收契约：`specs/feed-candidate-catalog/spec.md`、`specs/feed-discovery/spec.md`。
Scenario 映射：`tasks.md` §6「验证」节。

## 二、审查维度（按开发执行规范 §0.6 步骤 4）

1. **精读高风险文件**（至少覆盖）：

   - `subscription_state.go`：
     - `routeFixedPrefix` 的「段首 `:` 才算参数段」判定与 `ParseRouteParameters`/`buildFeedURL`（`recommendation_service.go` 的 `stripBraceConstraints`/`stripOptionalParams`）是否同口径；`/foo:bar`（段内冒号）、`/:id?`（空前缀）、尾斜杠 path 的边界是否正确。
     - `feedURLMatchesRoutePrefix` 的三种命中形态是否覆盖真实 `feeds.url` 形态，**是否存在漏判**（如带 query 的 `prefix?a=b`、带尾斜杠 `prefix/`、`prefix/` 后接 query）；段边界是否真的挡住 `/znews2` 类误命中。
     - `MatchRouteSubscriptions` 的最长前缀消歧：`len(p)` 按字节比较是否安全（URL 全 ASCII？中文 path 会不会比错）；空 prefixes / 空前缀 / 同一地址命中多个前缀时的归属；返回 map 的键是否必为传入前缀（调用方按 prefix 取值会不会 miss）。
     - `LoadSubscriptionIndex` 一次全表 `Pluck` 的开销与**一致性**（同一请求内多次调用是否重复查库、`buildViews`/`GetRun`/`attachSubscriptionState` 各调一次是否可接受）。
     - `resolveCandidateSubscriptions`：rsshub 但 `RouteID==nil`/路由缺失 → 未订阅（是否合理）；rss 且 `FeedURL==nil`；返回空切片非 nil 的保证是否覆盖所有分支。
   - `candidate_catalog_service.go`：`buildViews` 里订阅索引失败 → **整个列表 500**（原先只有精确 IN 查询）；`resolveRSSHubBaseURL(s.db)` 经 `aisettings.LoadRSSHubConfig()` 解引用全局 `database.DB` ——生产路径是否必然已初始化（`GetCandidate`/`CreateCandidate`/`ListCandidates` 三入口）、测试侧 `setupCandidateCatalogTestDB` 里 `database.DB = db` 是否会污染其它测试（全局变量赋值后有无恢复）。
   - `recommendation_service.go`：`attachSubscriptionState` 的候选批量查询只 `Select("id","kind","route_id","feed_url")`；`routes` map 用 `cards[i].RouteID` 键（注意 `FeedRecommendation.RouteID` 是 uint 非指针，native rss 候选 route_id=0 时 map 会不会写入 `0` 键并被误用）；legacy 无 `CandidateID` 行保持 false 的语义是否与 design D4 一致；重复订阅查询失败返回 error 是否让「推荐列表」从可用变不可用（权衡是否该降级）。
   - `discovery_run_service.go` `GetRun`：新增的 `LEFT JOIN rsshub_routes cr ON cr.id = fc.route_id` 是否可能放大行数（一候选一行？`fc.route_id` 唯一？）；`candidateInputs` 用 `r.CandidateID` 为键，若同一 run 有重复 candidate 行（联合唯一约束应挡住）会怎样；展示字段（`r.name`/`r.description`/availability）来源未被改动这一不变量是否成立（对照 `git diff`）。
   - 前端 `CandidateSubscribeDialog.vue`：`finalUrlSubscribed` 用**字符串精确相等**（`includes`）与后端 `subscribed_urls`（同口径精确）是否一致；`kind==='rss' && subscribed` 直接终态是否让「原生 RSS 已订阅但用户想改分类/其它」无路可走（同现状，但确认是不是回归）；`canSubmit` 加 `!finalUrlSubscribed` 后按钮 disabled 的提示文案链路是否完整（用户能不能看出为什么点不了）。
   - 前端 `CandidateLibrary.vue`：`canOfferSubscribe` 与 `isUpstreamGone(c)` 的 `disabled` 组合是否让「RSSHub 已订阅 + gone」出现「可见但禁用」的合理终态；`openSubscribe` 的早退与模板 `v-if` 是否同口径（避免点了没反应）。

2. **grep 全包验证关键不变量**（至少）：

   - `grep -rn "subscribed" backend-go/internal/discovery/ --include=*.go | grep -v _test` → 字段口径一致（rsshub 路由级 / rss 精确），无第二套判定实现。
   - `grep -rn "LoadSubscriptionIndex\|resolveCandidateSubscriptions" backend-go/internal/discovery/` → 只有三处响应接线 + helper 自身，无遗漏入口（如 `CreateCandidate` 返回视图、`GetCandidate` 详情、历史视图）。
   - `git diff -- backend-go/internal/discovery/service/discovery_recall.go` → **空**（召回 SQL 不在范围）。
   - `grep -rn "subscribed_urls" front/app` → 类型/normalizer/四入口齐全，命名一致（snake→camel）。
   - `grep -rn "color-success-subtle\|--color-" front/app/features/discovery/components/DiscoveryCard.vue front/app/features/discovery/components/DiscoveryRunCard.vue` → 只用既有主题变量，无硬编码色值。
   - `grep -rn "tasks.md\|openspec/" backend-go/internal/discovery front/app --include=*.go --include=*.ts --include=*.vue` → 代码不引用 change 制品路径（除注释）。

3. **分级**：High（bug/安全/清晰错误，必修）/ Medium（合理但依赖上下文，选修）/ Low（误报，静默丢弃）。

## 三、输出要求（三段式，中文）

1. **总评**：改动是否达成 design/spec 意图、可否进入门禁收口（一句话结论 + 理由）。
2. **问题清单**：按 High/Medium/Low 分级，每条给 `file:line` + 复现/依据 + 建议修法；没有就明确写「无」。
3. **核实记录**：实际跑过的只读命令与退出码、读过的关键文件清单（防幻觉）。

## 已知且已裁决、**不要**当问题报的事项

- 路由级命中**不禁用**订阅入口（用户明确决策）；原生 RSS 精确已订阅不给重复入口（同现状）。
- 召回期资格过滤 SQL 与历史 pending 卡**刻意不动**（design Non-Goals）。
- 跨 RSSHub 基址变更后的历史订阅不识别（与召回期 SQL 同口径，design Risks 已登记）。
- 最长前缀消歧只在单响应内生效（design D2/Risks 已登记）。
- legacy 无 `CandidateID` 的推荐行恒 `subscribed=false`（design D4 已登记）。
- 订阅索引装载失败向上返回错误、不静默降级（design 取舍：不得伪造订阅状态）。
- `setupCandidateCatalogTestDB` 增 `database.DB = db` 沿用仓库既有测试约定（`setupAcceptTestDB` 同款）。
- 工作树含其他 active change 脏文件（`backend-go/internal/reader/repository/**`、`backend-go/internal/platform/database/**`、`docs/reference/flow/reading.md`、`docs/reference/database/**`、`front/app/features/articles/components/ArticleStatusMenu.vue`、`.pi/agents/*.md`、`openspec/changes/ai-provider-model-overhaul/**`、`openspec/changes/fix-feed-delete-and-sequence-drift/**`）——超出本 change 辖区，一律不评。
