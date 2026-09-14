
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
