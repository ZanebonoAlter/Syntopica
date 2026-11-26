
## 迁移模式与版本号（add-article-search 迁移 A/B）

迁移模式（add-article-search 迁移 A/B 落点）：
- Migration struct：{Version, Description, Up func(db *gorm.DB) error, RunOutsideTx bool, Down}。版本号格式 YYYYMMDD_NNNN，当前最新 20261012_0001（keyEventMarkersMigration），新迁移用 20261013_0001（A：pg_trgm 扩展+GIN 索引）与 20261013_0002（B：DROP search_vector）。
- 列表尾部追加：postgresMigrations() 末尾 `return append(migrations, boardSignalMigration(), ...)` 链——新迁移函数照此模式追加（独立函数 + append）。
- 破坏性守卫：migrator.go IsDestructiveAllowed()（env MIGRATIONS_ALLOW_DESTRUCTIVE=1，默认 false）。DROP COLUMN search_vector 属破坏性——先例不一：多数 DROP 用守卫（postgres_migrations.go:1275/1312/1378/1606/2012），dropProfileTablesMigration(20260926_0001) 未守卫（零数据表）。迁移 B 推荐用守卫模式：!IsDestructiveAllowed() 时跳过 DROP 并记日志（列数据可再生，见 design D7 回滚策略）。注意：testcontainer 集成测试跑迁移时若 B 带守卫会跳过 DROP——迁移 B 的 testcontainer 断言（列/触发器不存在）需在测试侧开 destructive 或迁移内对测试库放行；先查 testutil 是否设置该 env（未确认）。
- 旧 search_vector 创建点：postgres_migrations.go:399-413（列+GIN 索引+触发器 articles_search_vector_trigger+函数 articles_search_vector_update）；:1969 有 DROP INDEX IF EXISTS idx_articles_search_vector 先例。
- 迁移测试先例：backend-go/internal/platform/database/*_migration_test.go，用 testutil.SetupTestDB(t)（testcontainer pgvector，-short 跳过，Docker 需在跑——本机 Docker 已确认 healthy）。

**引用**：backend-go/internal/platform/database/postgres_migrations.go、backend-go/internal/platform/database/migrator.go

<!-- pinned 2026-10-13T15:07:23Z -->

## article_handler tsquery 分支与 feed_service search_vector 残留

tsquery 消费点（add-article-search 2.8 切换目标）：
- article_handler.go GetArticles：主查询 :199 `query.Where("articles.search_vector @@ plainto_tsquery('simple', ?)", search)`（postgres 分支）；watched-tags count 查询 :250 同款。非 postgres（sqlite 测试库）分支走 `title LIKE ? OR description LIKE ?`。切换时保留 sqlite LIKE 分支（handler 测试用内存 sqlite，见 article_handler_test.go setupArticlesHandlerTestDB），postgres 分支换成 search 服务 D2 管线（L0/L1/L2/L3 OR 组合）。
- 归档默认排除：列表查询 `Where("articles.archived = ?", archived)`（契约保持）。
- feed_service.go:296-301 归档 updates：`updates["search_vector"] = nil` 有 Migrator().HasColumn 守卫（sqlite 无此列）——迁移 B DROP 列后 HasColumn 恒 false 自动短路，但按 tasks 1.2 要求整段移除该分支（含注释），updates 只留 {"archived": true}；feed_service_cleanup_test.go 有既有测试。
- 路由注册：internal/reader/routes.go RegisterRoutes——articles 组 GET ""（GetArticles）。新 search 端点：rg.GET("/search", ...) 平级新增，handler 放 internal/reader/search 包（design D6）。

**引用**：backend-go/internal/reader/handler/article_handler.go、backend-go/internal/reader/service/feed_service.go、backend-go/internal/reader/routes.go

<!-- pinned 2026-10-13T15:07:23Z -->

## 语义路径依赖：airouter Embed + pgvector cosine + 配置读取先例

语义路径与配置读取（add-article-search 2.4/2.7 依赖）：
- embedding 调用：airouter.Router.Embed(ctx, EmbeddingRequest{Operation 必填, Input, ...}, capability) (*EmbeddingResult, error)（router.go:479）；内建 ai_embedding_cache 缓存（embeddingCacheable(Operation) 白名单控制，新 operation 需确认是否进缓存白名单——查 embeddingCacheOperations）。EmbeddingRequest/Result 定义在 airouter/embedding.go。
- TopicTagEmbedding 模型（tagmanagement/models/tag_models.go:20）：表 topic_tag_embeddings，EmbeddingVec 字段 gorm:"type:vector;column:embedding"，EmbeddingType（identity/semantic/event_keyword），Dimension，TextHash。查询用原始列名 embedding。
- pgvector cosine SQL 先例（tagmanagement/service/core/embedding.go:180-187）：`SELECT t.id, e.embedding <=> ?::vector AS distance FROM ... WHERE e.embedding_type='identity' ORDER BY e.embedding <=> ?::vector LIMIT n`——cosine 相似度 = 1 - distance。
- LIKE 转义先例：discovery/service/candidate_catalog_service.go:546 escapeLikePattern（%/_/反斜杠 + ESCAPE '\' 子句）——search 包复制该模式（discovery 包私有，不可直接 import）。
- ai_settings 配置读取先例：internal/platform/aisettings/config_store.go（key=JSON blob 模式）与 discovery/service/seed_policy_config.go（loadXxxConfig：键缺失/空值→默认，解析失败→错误）。search_trigram_threshold 等三键按 spec 是标量键（非 JSON blob），实现可单键读 ai_settings（Value 字符串），非法值回退默认+warn（spec 要求）。

**引用**：backend-go/internal/platform/airouter/router.go、backend-go/internal/platform/airouter/embedding.go、backend-go/internal/tagmanagement/models/tag_models.go、backend-go/internal/tagmanagement/service/core/embedding.go、backend-go/internal/discovery/service/candidate_catalog_service.go、backend-go/internal/platform/aisettings/config_store.go

<!-- pinned 2026-10-13T15:07:41Z -->

## 前端复用点：卡片/预览弹窗/侧边栏/图标子集

前端复用点（add-article-search 3.x 任务）：
- ArticleCardView.vue（features/articles/components/）：props={article, compact?, selected?, showFeedTitle?}，emit click/favorite；新增可选 hitReasons/archived props 缺省行为不变。样式走 ~/components/article/ArticleCard.css + 语义 token。
- ArticlePreviewModal.vue（features/tags/components/）：props={visible, selectedPreviewArticle, previewArticles, loadingPreviewArticle}，emit close/navigate/favorite/article-update；AppDialog 90vw + close-on-overlay/escape。新增可选命中词高亮 prop（如 highlightTerms?: string[]），tags 域现有用法零改动。
- AppSidebarView.vue：「发现订阅源」入口在 :176-178（mdi:compass-outline，@click navigateTo('/discovery')），新「文章搜索」入口加其后（分隔线之前），图标 mdi:magnify。
- 图标红线：新增 mdi:magnify 后必须跑 `pnpm generate:icons`（front/scripts/generate-icon-subset.mjs 扫 app/ 源码重新生成 app/assets/iconify-subset.json）并提交产物。
- 组合式函数先例：features/discovery/composables/useDiscovery.ts（含 export interface 状态分组）；组件测试先例见 front/app 各 *.test.ts（Vitest + happy-dom）。
- 页面壳：pages/discovery.vue 等 use AppPageShell；search 页用 mode="contained"（≤1120px 居中）。

**引用**：front/app/features/articles/components/ArticleCardView.vue、front/app/features/tags/components/ArticlePreviewModal.vue、front/app/features/shell/components/AppSidebarView.vue、front/scripts/generate-icon-subset.mjs、front/app/features/discovery/composables/useDiscovery.ts

<!-- pinned 2026-10-13T15:07:41Z -->

## search 包接线契约（tasks 2.1-2.5 → 2.6-2.8）

internal/reader/search 包（tasks 2.1-2.5 已落地）对 2.6-2.8 的接线契约：

**公开 API**
- 词典：NewDictCache(db) / (*DictCache).EnsureLoaded(ctx)（懒加载、幂等、失败不冻结）/ InvalidateDict()（标签写入方接线点，tasks 2.8）/ Segment(query)；纯函数 SegmentForwardMaxMatch(words, query)。词典 = topic_tags.label 3~8 rune 无空白去重全集。
- 预处理：NewQueryPreprocessor(db, dict).Prepare(ctx, q) → (*PreparedQuery, error)。空/纯空白 → errors.Is(err, ErrEmptyQuery) → handler 转 400 且零 SQL。PreparedQuery{Raw, L0Terms/L1Words []MatchTerm{Text 原文, Escaped LIKE 转义}, TrigramThreshold}。L0Terms len==1 时 repository 自动跳过 L0（与 L2 重合）。
- 关键词路径：NewSearchRepository(db).SearchKeyword(ctx, pq, archived) → []ArticleHit{Article ArticleSearchRow, Hits []HitDetail{Level, Field, Text, TrigramScore}}。归档口径常量 ArchiveInclude（默认，不过滤）/ ArchiveActiveOnly。LIKE 层失败=硬错误；L3 trigram 失败=warn 降级（红线 11）。
- 语义路径：NewSemanticSearcher(db, embedder).Search(ctx, q, archived) → (hits []SemanticHit{Article, Cosine, Tag, MatchedTags}, matchedTags []MatchedTag, err)。Embedder 接口 {EmbedQuery(ctx, input) ([]float64, error)}；生产适配 AirouterEmbedder{Router *airouter.Router}（Operation="article_search_query"）。embedding 失败/超时 → ErrSemanticSkipped 哨兵（Search 内部 3s ctx.WithTimeout，白盒可改 s.timeout）；SQL 失败返回普通错误——**2.6 编排层对语义路任何 error 都应降级为 semantic.skipped=true + warn，不得让 /api/search 整体报错**。零命中 = (空,空,nil) ≠ skipped。
- 融合：KeywordRankEntries([]ArticleHit)/SemanticRankEntries([]SemanticHit) → 已排序 []RankEntry；Fuse(FuseInput{Keyword, Semantic, Expand}) → []FusedResult{ArticleID, Score(RRF), KeywordScore, SemanticScore, HitReasons 已聚合去重}。HitReason{Type: keyword|semantic|expand, Text, Score} 直接对应响应契约 hit_reasons。

**关键决策**
- L0 多词归 LevelDictMulti 系数 0.85（与词典多词匹配语义相同：全词出现、位置可分散）；层级档 whole 1.0 / dict-multi 0.85 / dict-single 0.6 / trigram 原始分；字段权重 title 1.0 / description 0.7。
- 命中明细：分层查询 + Go 聚合（单条大 SQL 的动态列拼装可读性差）；字段归属用行内 title 全量 + substr(description,1,2000) 有界源做 strings.Contains 判定，词落 2000 界外时回退 description。
- ArticleSearchRow = articleListColumns 窄投影镜像 + excerpt 源三字段（ExcerptDescription/ExcerptContent/NoFirecrawlBody，substr 2000 有界）；**excerpt ≤200 纯文本构建在 handler 层**（buildExcerpt 在 reader/handler，search 不得 import 避免环）——2.6 对最终分页行调 buildExcerpt(row.ExcerptDescription, row.ExcerptContent, row.NoFirecrawlBody==1)。
- L3 SQL：事务内 SELECT set_config('pg_trgm.similarity_threshold', ?, true)（事务作用域）+ `title % ?`（走 GIN 索引）+ similarity() 显式分 + Go 侧按阈值防御性复验。sqlite 分支（db.Name()!="postgres"）无 L3。
- 配置键：search_trigram_threshold(0.10)/search_semantic_min_cosine(0.55)/search_semantic_top_n(8)，LoadTrigramThreshold/LoadSemanticConfig(ctx, db)，键缺失静默回默认、非法值回默认+Warnf。

**遗留（2.6-2.8 或主线程）**
1. airouter embeddingCacheOperations 白名单不含 "article_search_query"（airouter/router.go:446）→ 查询 embedding 无缓存；加白名单是 airouter 包一行改动（design D3 期望缓存命中）。
2. InvalidateDict 未接线到 tagmanagement 标签写入方。
3. expand 榜（FuseInput.Expand + ReasonExpand="expand"）已预留，LLM 扩展实现属 2.7。

<!-- pinned 2026-10-13T16:05:13Z -->
