
## discovery v2 迁移收尾语义与哨兵机制（2.2/2.3）

迁移落点（2.2/2.3 已交付）：
1. 入口与哨兵：PG-only 迁移收尾在 migrator.go:runDiscoveryV2Backfill，由 RunAutoMigrate 尾部调用（每次启动重跑、幂等；sqlite 跳过）。步骤 a（旧 pending 去重）以 pg_indexes 中 idx_feed_recommendations_hash_pending 是否存在为「已迁移」哨兵——存在即整步跳过，保证新系统上线后写入的 pending 永不被转 legacy（后续服务层切片不要在此再引入 cutoff 逻辑）。
2. 步骤 a 语义（任务书措辞有歧义，最终取舍）：同 hash 多条 pending 保留 id 最大者继续 pending，其余（重复组非最大 id + 全部独立 hash 的旧 pending）转 status='legacy' 且 expires_at=now()。cutoff=当时 max(id) 仅覆盖首迁时刻行。
3. 索引：旧全表唯一 idx_feed_recommendations_hash 由步骤 b DROP；新约束=部分唯一索引 idx_feed_recommendations_hash_pending ON feed_recommendations(recommendation_hash) WHERE status='pending'。模型 tag 只留普通查询索引 idx_feed_recommendations_hash_lookup。服务层写同 hash pending 必须「更新不插入」（D2），历史行同 hash 多条合法。
4. 模型：FeedRecommendation 新增 CandidateID *uint（索引）/LastSelectedAt/ExpiresAt；DiscoveryInterestEntry.RunID 改 *uint（legacy 迁移行 NULL，普通唯一索引允许多 NULL）。
5. 旧 seed 迁移：discovery_interest_entries 写 status='legacy_inactive'、run_id=NULL、query_text 固定「legacy seed（无原始查询）」、legacy_ref='preference_vectors:'||旧id（NOT EXISTS 挡重）；behavior 源不迁移。
6. 冷却回填：candidate_preferences 仅 30 天内 dismissed（snoozed_until=dismissed_at+30d，note='legacy dismissed'，ON CONFLICT DO NOTHING 不覆盖服务层写的行）。
7. 测试：internal/platform/database/discovery_backfill_test.go（PG testcontainer，含幂等重跑+新 pending 不受影响断言）；sqlite 模型单测在 discovery_run_models_test.go（含 NULL run_id 多行合法）。
8. 踩坑：RSSHubRoute.Parameters 是映射 jsonb 列的 string 字段，GORM Create 时必须给合法 JSON（如 "{}"），空串会被 PG 以 invalid input syntax for type json 拒绝（fixture/服务层同坑）。

<!-- pinned 2026-09-11T17:34:53Z -->

## 4.4 生命周期落点与前端契约

切片 4.4（推荐生命周期）实现落点：
- 前端已上线契约：GET /api/discovery/recommendations?scope=history（HistoryPayload={id,name,status,snoozed_until,last_selected_at,llm_reason}，status∈accepted|expired|snoozed|excluded）；POST /api/discovery/recommendations/:id/dismiss（现为「不感兴趣（30天内不再推荐）」→ 升级为暂时不看）；POST /api/discovery/recommendations/:id/restore（恢复资格）。前端无常驻「长期排除」调用 → 新增 POST /:id/exclude。
- 冷却权威 = candidate_preferences（迁移 f 已把 30 天内 dismissed_at 回填为 snoozed_until = dismissed_at+30d，note='legacy dismissed'）。旧 route-level dismissed hash 池退役点：discovery_recall.go:526-538（searchCandidates SQL NOT IN dismissed）、discovery_run_service.go:580-592（publishRun cooldown recheck）、常量 DismissCooldownDaysDefault（discovery_helpers.go:31）。
- 召回已按 candidate_preferences 过滤（excluded_at 非空 OR snoozed_until>now），跨 qa/refresh 生效。
- publishRun 位于 discovery_run_service.go:566；expires_at=now+RecommendationTTLDaysDefault(Hardcoded 14)。upsertPendingRecommendation 按 hash+pending 更新 last_selected_at/expires_at。
- FeedRecommendation 有预载 Route/Board；CandidateID 可空。

**引用**：backend-go/internal/admin/service/discovery_run_service.go:566、backend-go/internal/admin/service/discovery_recall.go:499、front/app/api/discovery.ts:321、front/app/types/discovery.ts:289

<!-- pinned 2026-09-13T14:46:05Z -->

## 4.5 共享建源服务边界与依赖方向

切片 4.5 落点：
- 共享服务 new file backend-go/internal/reader/service/feed_create_service.go：FeedCreateService{db} + NewFeedCreateService(db)；
  VerifySubscriptionURL(ctx,rawURL)->(normalized,err)（Normalize（≤500 rune）→ 可替换包级 fetch（默认 safefetch.Fetch，SetSubscriptionFetcher 测试注入）→ 2xx + ParseFeedBody 可解析 RSS/Atom）；
  CreateOrReuseFeed(tx,normalized,opts)->(feed,created,err)（纯 DB，按 url 查重，不自行开事务）；
  CreateFeedWithVerification(ctx,rawURL,opts)=Verify + db.Transaction{CreateOrReuseFeed}。
- 依赖方向：admin/service → reader/service（reader 不得反依赖 admin，否则循环）。因此 reader/service **不能** import admin/service 的 NormalizeRSSURL（candidate_identity.go 属只读成果）；共享服务在新文件内实现同规则 NormalizeSubscriptionURL。
- rss_parser.go 新增纯入口 ParseFeedBody([]byte)，ParseFeedURL 复用，语义不变。
- accept 事务：admin RecommendationService.AcceptRecommendation → resolveRecommendationCandidate（回写 candidate_id 后须同步内存 rec.CandidateID 防 Save 清空）→ kind=rss 用 candidate.FeedURL；kind=rsshub 用 buildFeedURL + resolveRSSHubBaseURL → Verify（网络，事务外）→ 短事务 { CreateOrReuseFeed + 标记 accepted+accepted_feed_id }；已 accepted 走幂等短路返回既有 feed。
- 普通 reader handler CreateFeed 迁移到 CreateFeedWithVerification，created=false → 409，ErrInvalidFeedURL/ErrFeedVerification → 400。

**引用**：backend-go/internal/reader/service/feed_create_service.go、backend-go/internal/reader/service/rss_parser.go:ParseFeedBody、backend-go/internal/admin/service/recommendation_service.go:AcceptRecommendation

<!-- pinned 2026-09-13T15:12:40Z -->

## 3.4 目录同步语义（fetch 失败/gone/候选隔离/向量遗留）

目录同步（`internal/admin/service/catalog_sync_service.go:SyncAll`）3.4 后的行为契约：

1. **fetch 失败 = 错误**：`SyncAll` 返回 `(nil, error)`（wrap `fetch /api/namespace`），不再返回空成功 summary；handler `SyncCatalog` → 500 + error 文本，scheduler `RSSHubCatalogSyncJob` → job failed。绝不写库。
2. **解析不完整 = 整轮失败（fail-closed）**：`flattenNamespace` 现签名 `([]routeRecord, error)`——任一 namespace 体不是对象、任一 route detail 无法反序列化或缺失 `path`，都在写库前中止整轮。原因：拿不到完整目录时按「未出现在本轮列表」推导 gone 会把没取得的路由误判上游删除（spec C2）。
3. **content_hash = sha256(namespace|path|name|url|description|example|parametersJSON)[:32]**；旧 hash 行必然不等 → 下一轮走 Update 幂等 upsert，一次重同步收敛，无迁移脚本。
4. **候选联动**：`SyncAll` 每轮对**所有**本轮路由（含无变化者，一次 `stable_key IN (...)` 预载禁 N+1）调用 `syncRouteCandidate`——不存在则建（kind=rsshub / canonical_key 空 / manual {} / enabled true / access public / revision 1，`OnConflict DoNothing` 兜底并发）；已存在只 `Update("route_id", ...)`，**唯二候选 UPDATE 列就是 route_id**（绝不碰 manual_metadata / recommendation_enabled / access_scope / revision）——人工停用不复活、人工说明不被冲掉，测试 `TestCatalogSyncLinksExistingCandidateWithoutTouchingManualFields` 逐字段钉住。覆盖导入条目（RouteID=NULL）在内容未变时也能绑上本地上游（design D8）。
5. **gone 语义**：只有本轮成功且缺席才 `rsshub_routes.status='gone'`（保留行）；候选保留不删、订阅照旧。视图透出：`CandidateView.Route.Status`（CandidateView/GetCandidate/ListCandidates 共用 buildViews）已是 `route.status`，测试 `TestCatalogSyncMarkGoneKeepsCandidateAndViewExposesStatus` 钉住；**前端 `front/app/types/discovery.ts` 的 `CandidateRouteInfo` 未声明 status、列表 normalizer 直接丢 route** → UI 目前显示不了「上游已下架」，属 front 侧改动（本切片未碰 front/）。
6. **EmbedPendingRoutes 保持原状（只补注释）**：`route_embeddings` 自 4.3 起无消费方（召回走 `candidate_embeddings × feed_candidates`，见 `discovery_recall.go:32`），故不做无消费方的 hash 变化重嵌；内容变化的重嵌由 `CandidateEmbeddingService.DirtyCandidateEmbeddings` 按候选有效介绍指纹节流完成（`catalog_extras.go` 顶部与函数注释已写明）。
7. **summary 新增字段** `CandidatesCreated` / `CandidatesLinked`（无 json tag，与既有 PascalCase 一致）。

**引用**：backend-go/internal/admin/service/catalog_sync_service.go:SyncAll、backend-go/internal/admin/service/catalog_sync_service.go:syncRouteCandidate、backend-go/internal/admin/service/catalog_sync_service.go:contentHash、backend-go/internal/admin/service/catalog_sync_service.go:flattenNamespace、backend-go/internal/admin/service/catalog_extras.go:EmbedPendingRoutes、backend-go/internal/admin/service/catalog_sync_service_test.go

<!-- pinned 2026-09-13T16:58:11Z -->

## WSL↔前后端连不通的根因与转发方案评估

## 拓扑与根因（2026-09-14 实测）

- 运行拓扑：前后端都跑 **Windows**（Nuxt dev = node.exe :3000；Go 后端 = go run 临时 main.exe :5000）；pi agent 与工具（curl/ctx/uv/opencli）在 WSL2（mirrored 模式，内核 5.15.167.4 偏老）。
- **根因①端口 5000 被 svchost（WSD 系统服务，PID 4576）占 0.0.0.0:5000 v4**：Go 后端实际有效监听只剩 [::]:5000。WSL→127.0.0.1:5000 命中 svchost 死 socket → 超时（半开）；WSL→[::1]:5000 → ECONNREFUSED（v6 镜像不通）。Windows 浏览器 localhost 解析 ::1 所以用户无感。ui-verify/references/network-and-navigation.md 2026-09-03 已记录同现象（"后端重启后仅监听 IPv6、v4 半开"，当时约定 powershell.exe 中转 / opencli 同源 fetch）。
- **根因②WSL shell 代理污染**：curl 走 http://127.0.0.1:7897（Clash，Windows 侧经 mirrored 进来），对 localhost:5000 返回超时/502，探测结论被污染；.bashrc/.profile 未见 proxy 导出（来源待查，可能是 /etc/environment 或 Windows 注入）。
- **根因③（另一层）cmd.exe interop vsock 间歇故障**：events.db 2026-09-12 两条 policy.decision interop-down（fail-open）、2026-09-14 gate.check `go build` diag 含 `UtilAcceptVsock:251: accept4 failed 110`。进程互操作层问题，TCP 代理救不了；缓解 = 升级 WSL / `wsl --shutdown` 重启 VM。
- 实测通道稳定性：WSL→Windows **:3000 稳定可达**（Node fetch 14ms 200）；:5000 两个栈都不通。

## 前端→后端连接链路事实

- `front/nuxt.config.ts`：ssr:false（纯 SPA，后端请求全从浏览器发出，Node 进程本身不调后端）；runtimeConfig.public.apiBase 默认 `http://localhost:5000/api`（`NUXT_PUBLIC_API_BASE` 可覆盖）；**无 devProxy / routeRules**。
- `front/app/utils/api.ts`：`getApiBaseUrl()`——base 以 http 开头→原样用；否则 **isDev()（浏览器端口 3000）强制回退 `http://localhost:5000/api`**；非 dev 用相对路径（同源生产形态已预留）。`getApiOrigin()` 同构逻辑，供 WS。
- WS：`front/app/composables/useEventStream.ts` 单例，`getApiOrigin().replace(/^http/,'ws') + '/ws'` → 浏览器直连 `ws://localhost:5000/ws`；另有 features/articles/composables/useTagWebSocket.ts。
- 后端：`cmd/server/main.go` `r.Run(":5000")`（config.Server.Port），CORS 白名单可配（CORS_ORIGINS）。生产 Docker：compose 映射 `${PORT:-5000}:5000`，**deployment.md "front 内部代理 API" 说法无代码实现**（front/server 只有 fetch-feed.post.ts，非反代）。
- e2e：playwright baseURL localhost:3000。

## 方案评估

- A 根治=后端默认端口挪出 Windows 保留段（5000 是 WSD 重灾区；换 5100 等）——最便宜且同时治浏览器/dev/prod。
- B 用户提议的"前端 Node 转发"推荐落成 **Nitro devProxy**（nuxt.config.ts `devProxy: {'/api': {target,changeOrigin}, '/ws': {target, ws:true}}`）+ apiBase 改相对 `/api`：需同步改 utils/api.ts isDev 强制回退分支；浏览器与 WSL 工具全部收敛到 :3000 单通道（已证稳定），dev 与"front 为入口"的目标拓扑对齐，CORS 在 dev 消失；后续可用 nitro routeRules 平移到生产。
- C 卫生：WSL shell 设 no_proxy=localhost,127.0.0.1,::1。
- 顺序建议 A+B 一起开 change（同触 front/nuxt.config.ts、utils/api.ts、后端 config 默认值、docs/configuration.md、deployment.md、AGENTS.md 端口口径）。

<!-- pinned 2026-09-15T13:50:55Z -->

## 发现页双交互 bug 根因与修复（出口清洗 + refresh 异步轮询）

## 2026-09-18 用户双 bug 修复（6.2 验证发现，已实现并测试通过）

### Bug1：候选库/查询结果描述一大坨原始 URL
- 链路：RSSHub 目录 API 的 description 是 VitePress 文档页 markdown 源码（表格/链接/`<details>`/`::: tip`）→ `catalog_sync_service.go` 原文入 `rsshub_routes.description` → 出口 `EffectiveMetadata`（candidate_identity.go）原样下发 → 前端 `{{ c.description }}` 文本插值。
- 修复：`EffectiveMetadata` 出口统一对 name/description 调 `SanitizeEffectiveText`（candidate_embedding_service.go 已有函数，对已清洗文本幂等）；一处改动覆盖候选列表/详情（buildViews）、run 详情（GetRun items）、embedding 文本三个出口。存储原文保留（搜索 LIKE/指纹/重嵌不受影响）。
- 前端配套：`normalizeRoute` 透传 route.description 原文、normalizeCandidate 透传 manualMetadata；CandidateEditDialog 回填优先级改为 人工原文 → 上游原文 → 出口值（防止"打开编辑框就保存"把清洗值固化成人工覆盖、阻断上游更新）；三处展示位（library__desc / run-card__desc / discovery-card__desc）加 line-clamp 3。

### Bug2：点刷新必弹"暂无可推荐的内容：先同步目录"且无处同步
- 根因：后端 refresh 已异步化（E2E async-run-fix），handler 秒回 `{run_id, status, candidates:0, inserted:0, ...}`（注释明言"前端待按 run_id 轮询，不在本次范围"——遗留断裂）；前端 `refresh()` 仍读全 0 计数 → `candidates===0` 恒成立 → 必弹该 toast；且 `loadRecommendations()` 在 run 跑完前执行，拉到的还是旧列表。DB 实证：run 6 refresh succeeded、35 个 run items，用户却被告知"无可推荐"。
- 次生问题：toast 引导"同步目录"但同步按钮只在 `catalogEmpty`（total===0）空态出现；且目录有 3097 条时同步根本不解决召回为空（召回四路：全局行为批/版块批/版块行为批/seed 批，全依赖 preference_vectors、semantic_labels 向量、discovery_interest_entries）。
- 修复：stores/discovery.ts 抽共享 `pollRun(runId)`（2s×60 次与 submitQuery 同节奏，submitQuery 已重构复用）；`refresh()` 改为受理→轮询→按 run 终态与 items 数提示（succeeded+items>0 →「换了一批新推荐（本轮选出 N 条）」+重拉；succeeded+0 → 诚实文案"推荐依据还不够"，不提同步目录；failed → error 且不重拉；超时 → warn 可稍后再看）。CandidateLibrary 工具栏加常驻「同步目录」按钮（library-sync-catalog-btn，调已有 syncCatalog）。
- 类型契约：RefreshSummary 改为 `{runId, status}`（后端占位计数字段前端不再消费）。

### 验证记录
- 后端：`go test -short ./internal/admin/service ./internal/admin/handler` 绿；golangci-lint 0 issues；go vet / go build 绿。新用例：TestEffectiveMetadata/上游脏markdown出口清洗、人工脏markdown同样清洗（首版断言把 ::: tip 标记与正文写同行被整行删除——标记行整行去除是 SanitizeEffectiveText 预期行为）。
- 前端：discovery 相关 10 测试文件 116 用例绿；lint 0 errors；nuxi typecheck 干净（新增 CandidateRouteInfo.description 必填字段补齐 6 处测试字面量）。

### 遗留
- 候选 embedding 回补 20/批每小时：3099 候选仅 1483 有向量，全量补齐需数天（无阻塞，召回用已有向量子集）。
- e2e-log 的小缺口清单仍在：409 existing_id 透前端 dupHint、?tab=candidates、PATCH 不发 revision、dismiss 丢 snoozed_until。
- dev-process-guard 报 12 个窗口外 chromium/agent-browser 泄漏进程待用户人工确认处置。

<!-- pinned 2026-09-18T16:00:22Z -->

## 候选库分页补全 + markdown 不渲染的决策依据

## 候选库分页补全（2026-09-19 用户报告，已实现）

- 现状：后端 ListCandidates 一直支持 page/page_size（默认 30、上限 100）并返回 {items,total}，前端 api 层 getCandidates 也支持 page/perPage 并回传 pagination；但 store.loadCandidates 从不传 page（恒第 1 页）、CandidateLibrary 无分页控件 → 3099 条候选只能看前 30 条。
- 修复：stores/discovery.ts 增 candidatesPage/candidatesPages 状态 + goToCandidatesPage（越界/同页/加载中防抖）+ 翻页间数据变少的越界回退（当前页空且 pages 变小 → 自动回拉末页）；setCandidateFilters/clearCandidateFilters 重置页码 1。CandidateLibrary 列表底部加分页条（上一页/「第 x / y 页 · 共 N 条」/下一页，data-testid=library-pager*，单页不渲染）。
- 测试：store 2 新用例（翻页带页码、越界回退）+ 2 处既有断言适配 page:1；组件 3 新用例（多页分页条/单页隐藏/翻页+筛选重置）。

## markdown 不渲染只清洗截断的理由记录（用户质疑时的决策依据）
- 数据：1283 条有描述的 RSSHub 路由里 1085 条（85%）含文档噪音（表格/details/:::/标题），最长 67629 字——本质是开发者路由文档不是源介绍。
- 参数取值信息已有结构化通道（param_options 字典 → 订阅弹窗下拉框），不靠渲染文档获得。
- 渲染需配 XSS 消毒（上游内容含裸 HTML）+ markdown 库，列表 30 条/页渲染 6 万字文档不现实；design D6 契约即「先清洗格式再截取有效说明」。
- 截断（line-clamp 3）只是清洗后仍超长的兜底，绝大多数清洗后为一两句话。详情场景如需完整文档可后续在弹窗加「查看上游文档」链接。

<!-- pinned 2026-09-18T16:09:56Z -->

## 订阅填参被无视（usableDirectly 短路）修复

## 订阅填参被无视 bug 修复（2026-09-19 用户实测发现，已实现）

- 现象：订阅候选 `zaobao/realtime/:section?`（usable_directly + example=/zaobao/realtime/china），填 section 后最终地址仍是 example 的 china，用户填参被无视。
- 根因（前后端对称两处）：`buildFeedURL`（backend recommendation_service.go）/ `buildRSSHubFeedUrl`（front utils/routeParams.ts）的 usableDirectly 分支**无条件短路返回 example**，parameters 整个被忽略。既有测试只覆盖"空参 → example"与"必填路由填参"，恰好漏掉"usableDirectly + 填参"组合。
- 次生 bug 一并修：`:name?` 替换裸 `:name` 后值尾残留 `?`（/singapore?），裸 `?` 可致 RSSHub 实例 404；`{regex}` 约束在替换后剥离会把约束混进值尾。
- 修复规则（两端一致）：① 用户填了任一非空参数值 → 一律走模板替换（example 只是未填参时的缺省形态）；② 替换前先剥 `{regex}` 约束（后端提取为 stripBraceConstraints）；③ 先替换 `:name?` 再替换 `:name`（可选标记跟随参数名一起替换）；④ 空白值视作未提供（后端 trim 判定），必填段残留报错、可选段 strip。
- 影响面：前端 CandidateSubscribeDialog finalUrl（候选/run 条目订阅弹窗共用）；后端 accept 推荐填参（recommendation_service.go:315）与可用性检查（candidate_check_service.go:256，params=nil 行为不变）。
- 回归测试：前端 routeParams.test.ts +4（zaobao 替换/未填保持缺省/? 残留/{regex}）；后端 recommendation_service_test.go +3（UsableDirectlyWithParams 三态/NoQuestionMarkResidue/BraceConstraint）。后端 admin/service 全包绿、golangci-lint 0 issues；前端 routeParams+CandidateSubscribeDialog 52 用例绿、lint/typecheck 干净。

<!-- pinned 2026-09-18T16:25:42Z -->
