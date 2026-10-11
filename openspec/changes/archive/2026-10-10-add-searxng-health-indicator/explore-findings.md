
## 页边注联网链路与健康探测实现基线

QA 联网链路现状（add-searxng-health-indicator 实现基线）：

1. 搜索降级树入口 `searchWebForMarginNoteWithQueryGen(ctx, quotedText, question, threadTitle, threadSummary) []searxng.Result`（topicgraph/service/margin_notes_websearch.go）：端点预检 `marginNoteSearxngEndpointFn()`（默认=marginNoteSearxngEndpoint，DB(aisettings.LoadSearxngConfig) > env/yaml(config.AppConfig.Searxng.Endpoint)，每次现读）→ query 生成 `marginNoteQueryGenFn` → 并行搜索 `marginNoteWebSearchFn`（默认 searxng.Search，8s 客户端超时）→ 合并截 5。降级树分支：预检不过 return nil；生成失败回退锚文本 searchWebForMarginNote（margin_notes_qa.go:352 单 query）；并行全失败/空 return nil（不二跳）。

2. `AskMarginNote`（margin_notes_qa.go）：webResults := searchWebForMarginNoteWithQueryGen(...) → prompt → 单次 marginNoteChatFn → 白名单过滤 → `repository.AnnotationQA` InsertAnnotationQA 落库 → MarginNoteQAResult{Answer,CitedArticleIDs,CitedWebSources,Terms,NewTerms,PureModelKnowledge,Provider,QA}。handler askAnnotationQuestion（topicgraph/handler/margin_notes_handler.go:151+）手工组 gin.H 响应 qa 条目。

3. AnnotationQA 模型（repository/margin_notes_repository.go:44）：jsonb 数组契约列 + operation/provider/model 审计冗余；读路径 Preload QAs 直接 JSON 序列化（GET 响应=模型 tag）。旧行 NULL 语义靠指针字段实现（TermNote.Embedding 先例）。

4. 健康探测端点放 discovery 域：settings 组路由在 discovery/routes.go:60-61（GET/POST /api/settings/searxng → handler.GetSearxngSettings/SaveSearxngSettings）；探测函数须从 topicgraph/service 导出（复用 marginNoteSearxngEndpoint，discovery→topicgraph/service 无循环依赖；topicgraph 不 import discovery）。探测直接 searxng.Search（不走 airouter/不落 ai_call_logs）。

5. 迁移：postgres_migrations.go 注册尾（marginNotesWebSourcesMigration=20260924_0001 → vendorModelMigration → boardSignal 系列 20261005/20261006_0001）；新迁移版本取 20261007_0001。迁移测试范式：database_test 包 findXxxMigration by Version + testutil.SetupTestDB（testcontainer PG）+ 幂等重跑断言。

6. 测试钩子范式（margin_notes_qa_test.go）：swapMarginNoteChat（默认同时 stub 搜索=禁用/端点=false/queryGen=err，联网用例最后显式 swapMarginNoteEndpoint+swapMarginNoteQueryGen+swapMarginNoteSearch 覆盖）；handler 测试只覆盖校验/404/502（LLM 成功路径在 service 层 stub）；日志断言用 logging.SetWriters(&buf,&buf)+defer ResetWriters。

7. 前端：api/searxng.ts useSearxngApi（getConfig/saveSettings）；marginNotes.ts normalizeQa spread payload（新字段自动透传，MarginNoteQa 接口加可选字段即可）；MarginNoteCard mn-qa__webs/mn-web-chip 中性色系为标记样式同族；SearxngConfigPanel 现有 loading/success/error 范式 + AppToggle。

8. 版本排序：migrator 按注册顺序还是版本排序未确认——实现时看 migrator.go（版本号唯一性必须保证）。

**引用**：backend-go/internal/topicgraph/service/margin_notes_websearch.go:searchWebForMarginNoteWithQueryGen、backend-go/internal/topicgraph/service/margin_notes_qa.go:AskMarginNote、backend-go/internal/topicgraph/repository/margin_notes_repository.go:AnnotationQA、backend-go/internal/discovery/handler/discovery_handler.go:GetSearxngSettings、backend-go/internal/discovery/routes.go:RegisterRoutes、backend-go/internal/platform/database/postgres_migrations.go

<!-- pinned 2026-10-03T15:33:59Z -->
