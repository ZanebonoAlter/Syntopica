# 移除 Auto Summary + Digest 功能实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 移除 auto summary（feed 级 AI 总结）和 digest（日报/周报导出）功能，叙事（narrative）功能已完全覆盖其价值。

**Architecture:** 后端移除 `internal/domain/summaries/`、`internal/domain/digest/`、`internal/jobs/auto_summary.go`；解耦 `topicgraph` 和 `topicanalysis` 对 `AISummary` 模型的依赖改为直接使用文章+标签数据；前端移除 summaries 页面、digest 页面及相关组件和 API。

**Tech Stack:** Go (Gin, GORM), Vue 3 (Nuxt 4), TypeScript

---

## Phase 1: 后端核心移除（无依赖冲突的删除）

### Task 1: 删除 summaries 和 auto_summary 后端代码

**Files:**
- Delete: `backend-go/internal/domain/summaries/` (整个目录)
- Delete: `backend-go/internal/jobs/auto_summary.go`
- Delete: `backend-go/internal/jobs/auto_summary_test.go`

**Step 1:** 删除以上文件

**Step 2:** 移除 `backend-go/internal/app/runtime.go` 中对 AutoSummary 的引用：
- 删除 `AutoSummary *jobs.AutoSummaryScheduler` 字段（line 22）
- 删除 import `"my-robot-backend/internal/domain/summaries"` 如果存在
- 删除 scheduler 创建和启动代码
- 删除 shutdown 时的 Stop 调用

**Step 3:** 移除 `backend-go/internal/app/router.go` 中 summaries 相关路由：
- 删除 `summariesdomain` import
- 删除所有 `/summaries`、`/auto-summary/status`、`/auto-summary/config` 路由注册
- 删除 `/ai/summarize`、`/ai/test`、`/ai/settings` 路由（如果仅服务于 auto summary）

**Step 4:** 移除 `backend-go/internal/app/runtimeinfo/schedulers.go` 中 `AutoSummarySchedulerInterface`

**Step 5:** 移除 `backend-go/internal/jobs/handler.go` 中 auto_summary 相关代码：
- 删除 summaries import
- 删除 `auto_summary` scheduler 注册
- 删除 `summaries.GetSummaryQueue()` 调用
- 删除 auto-summary 状态检查

**Step 6:** 移除 `backend-go/internal/jobs/auto_refresh.go` 中 `triggerAutoSummaryAfterRefreshes` 方法和调用

**Step 7:** 运行 `go build ./...` 确认编译通过（此时会有其他文件的编译错误，预期内的，后续 task 修复）

**Verify:** `go build ./...` 编译错误只来自尚未修改的引用文件

---

### Task 2: 删除 digest 后端代码

**Files:**
- Delete: `backend-go/internal/domain/digest/` (整个目录，含所有测试)
- Delete: `backend-go/cmd/migrate-digest/` (整个目录)
- Delete: `backend-go/cmd/test-digest/` (整个目录)

**Step 1:** 删除以上目录

**Step 2:** 移除 `backend-go/internal/app/runtime.go` 中 digest 引用：
- 删除 `Digest *digest.DigestScheduler` 字段
- 删除 `digest` import
- 删除 digest scheduler 创建、启动、停止代码

**Step 3:** 移除 `backend-go/internal/app/router.go` 中 digest 路由：
- 删除 `digestdomain` import
- 删除所有 `/digest` 路由注册

**Step 4:** 移除 `backend-go/cmd/server/main.go` 中 `digest.Migrate()` 调用和 import

**Step 5:** 移除 `backend-go/cmd/migrate-db/main.go` 中 digest 相关 import 和调用

**Step 6:** 移除 `backend-go/internal/jobs/handler.go` 中 digest scheduler 注册

**Step 7:** 移除 `backend-go/internal/app/runtimeinfo/schedulers.go` 中 `DigestSchedulerInterface`

**Step 8:** 移除 `backend-go/internal/platform/opennotebook/client.go` 中 `SummarizeDigest` 方法及其测试中的引用

**Step 9:** 运行 `go build ./...` 确认编译通过

**Verify:** `go build ./...` 编译通过

---

### Task 3: 清理后端数据模型和迁移

**Files:**
- Modify: `backend-go/internal/domain/models/ai_models.go`
- Modify: `backend-go/internal/domain/models/article.go`
- Modify: `backend-go/internal/domain/models/feed.go`
- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Modify: `backend-go/internal/platform/database/migrator.go`
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Modify: `backend-go/internal/platform/database/bootstrap_postgres.go`
- Modify: `backend-go/internal/platform/database/datamigrate/types.go`

**Step 1:** 从 `ai_models.go` 删除：
- `AISummary` struct
- `AISummaryFeed` struct
- `AISummary.ToDict()` 方法
- `AISummaryTopic` struct（在 topic_graph.go 中）

**Step 2:** 从 `article.go` 删除：
- `FeedSummaryID` 字段
- `FeedSummaryGeneratedAt` 字段
- ToDict 中的对应条目

**Step 3:** 从 `feed.go` 删除：
- `AISummaryEnabled` 字段
- ToDict 中的对应条目

**Step 4:** 从 `migrator.go` 删除 AutoMigrate 中的：
- `&models.AISummary{}`
- `&models.AISummaryTopic{}`
- `&models.AISummaryFeed{}`

**Step 5:** 从 `postgres_migrations.go` 删除 migration `20260414_0003`（feed_summary_id 列迁移）

**Step 6:** 从 `bootstrap_postgres.go` 删除 `ai_summaries` 索引创建

**Step 7:** 从 `datamigrate/types.go` 删除 `ai_summaries` 条目

**Verify:** `go build ./...` 编译通过

---

## Phase 2: 后端核心重写（topicgraph + topicanalysis 解耦 AISummary）

### Task 4: 重写 topicgraph service 解耦 AISummary

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/service.go`
- Modify: `backend-go/internal/domain/topicgraph/handler.go`
- Modify: `backend-go/internal/domain/topicgraph/handler_test.go`
- Modify: `backend-go/internal/domain/topicgraph/hotspot_digests.go`
- Modify: `backend-go/internal/domain/topictypes/types.go`
- Modify: `backend-go/internal/domain/topictypes/services.go`

**关键变更说明：**

topicgraph 当前依赖 `AISummary` 做两件事：
1. **构建图谱节点/边**：`buildGraphPayload()` 遍历 summaries 提取 feed 节点、topic 边
2. **构建 TopicDetail 的 summaries 列表**：`BuildTopicDetail()` 返回匹配的 summary cards
3. **热点标签反查 digests**：`hotspot_digests.go` 通过 `ai_summaries` 反查

改为直接从 `article_topic_tags` + `articles` + `feeds` 构建相同数据。

**Step 1:** 在 `service.go` 中：
- 删除 `fetchSummaries()` 函数，改为已有 `fetchArticleTagsData()` 和 `buildGraphPayloadFromArticles()` 作为唯一构建路径（当前 `BuildTopicGraph` 已经走这条路径了）
- 删除所有 `models.AISummary` 参数的函数：`buildGraphPayload`, `summaryTopics`, `mapSummaryCard`, `feedNodeID`, `feedLabel`, `feedColor`, `feedIcon`, `categoryLabel`
- 删除 `BuildTopicDetail` 中的 summaries 相关逻辑（line 88-125 附近的 `matchedSourceSummaries`）
- 从 TopicDetail 返回值中移除 Summaries 字段

**Step 2:** 在 `topictypes/types.go` 中：
- 删除 `TopicSummaryCard` struct
- 删除 `TopicDetail.Summaries` 字段
- 删除 `TopicTagExtra.SummaryID` 字段

**Step 3:** 在 `topictypes/services.go` 中：
- 删除 `FetchArticlesForSummaries` 函数
- 删除 `resultFromLegacySummaryArticles` 函数

**Step 4:** 在 `hotspot_digests.go` 中：
- 删除 `models.AISummary` 引用
- 删除或重写 `GetDigestsByArticleTag()` 改为直接查 articles
- 删除 `getMatchedArticlesFromSummary()` 函数
- 重写为通过 `article_topic_tags` 直接反查文章列表

**Step 5:** 更新 handler 和 handler_test：
- 移除 `models.AISummary` 和 `models.AISummaryTopic` 的 AutoMigrate
- 移除 summary 测试 fixtures
- 重写受影响的测试

**Verify:** `go build ./...` 编译通过，`go test ./internal/domain/topicgraph/... -v` 通过

---

### Task 5: 重写 topicanalysis analysis_service 解耦 AISummary

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/analysis_service.go`
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Modify 相关测试文件

**关键变更说明：**

`analysis_service.go` 中所有 analysis payload 构建都通过 `[]models.AISummary` 作为输入。需要改为直接从 `articles` + `article_topic_tags` 构建相同数据。

**Step 1:** 重写 `fetchSummariesByTag` → 改为 `fetchArticlesByTag`，直接从 `article_topic_tags` JOIN `articles` 获取文章数据

**Step 2:** 重写 `buildPayloadJSON`、`buildPayload`、`buildEventPayload`、`buildPersonPayload`、`buildKeywordPayload` 参数从 `[]models.AISummary` 改为 `[]models.Article` 或新定义的中间结构

**Step 3:** 重写 `mapSummaryInfos` → 改为 `mapArticleInfos`，从文章提取 title/summary/date/feed 信息

**Step 4:** 删除 `EnqueueForSummary`、`EnqueueTopicAnalysisForSummary`、`fetchTagIDsBySummaryID`

**Step 5:** 重写 `maxSummaryID` → `maxArticleID`、`updateCursor` 中 `LastSummaryID` → `LastArticleID`、`buildTrendData`

**Step 6:** 删除 `embedding.go` 中 `models.AISummaryTopic` 引用和 `ai_summaries` JOIN

**Step 7:** 更新所有相关测试

**Verify:** `go build ./...` 编译通过，`go test ./internal/domain/topicanalysis/... -v` 通过

---

## Phase 3: 后端零散清理

### Task 6: 清理后端剩余引用

**Files:**
- Modify: `backend-go/internal/domain/feeds/handler.go` — 移除 AISummaryEnabled
- Modify: `backend-go/internal/domain/feeds/service.go` — 移除 summary_status 相关逻辑
- Modify: `backend-go/internal/domain/feeds/service_test.go` — 移除 summary_status 测试
- Modify: `backend-go/internal/domain/articles/handler.go` — 移除 feed_summary_id/feed_summary_generated_at
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — 删除 TagSummary 和 primaryArticleIDForSummary
- Modify: `backend-go/internal/domain/topicextraction/metadata_test.go` — 移除 AISummary/AISummaryTopic AutoMigrate 和相关测试
- Modify: `backend-go/internal/platform/aisettings/config_store.go` — 删除 LoadAutoSummaryConfig/SaveAutoSummaryConfig
- Modify: `backend-go/internal/platform/airouter/migration.go` — 删除 AutoSummaryConfig 调用
- Modify: `backend-go/internal/platform/airouter/migration_test.go` — 删除相关测试
- Modify: `backend-go/internal/platform/ws/hub.go` — 删除 SummaryProgressMessage 和 BroadcastProgress
- Modify: `backend-go/internal/jobs/handler_test.go` — 移除 auto_summary 测试断言
- 修改 `runtimeinfo/schedulers.go` 中 `AISummarySchedulerInterface` 重命名为 `ContentCompletionSchedulerInterface`
- 更新所有引用 `AISummarySchedulerInterface` 的文件

**Step 1:** 逐一修改以上文件

**Step 2:** 运行 `go build ./...` 确认编译通过

**Step 3:** 运行 `go test ./...` 确认测试通过

**Verify:** `go build ./...` 和 `go test ./...` 全部通过

---

## Phase 4: 前端移除

### Task 7: 删除前端 summaries 和 digest 页面及 API

**Files:**
- Delete: `front/app/features/summaries/` (整个目录)
- Delete: `front/app/features/digest/` (整个目录)
- Delete: `front/app/pages/digest/` (整个目录)
- Delete: `front/app/api/summaries.ts`
- Delete: `front/app/api/digest.ts`
- Delete: `front/app/components/ai/AISummariesList.css`

**Step 1:** 删除以上文件和目录

**Step 2:** 修改 `front/app/api/index.ts` — 移除 `useSummariesApi` 和 `useDigestApi` 的 export

**Step 3:** 修改 `front/app/types/ai.ts` — 移除 AISummary 相关类型定义

**Step 4:** 修改 `front/app/stores/api.ts` — 移除 summariesApi 和 digestApi 相关代码

**Step 5:** 修改 `front/app/stores/api.test.ts` — 移除 summaries mock

**Verify:** `pnpm exec nuxi typecheck` 确认类型检查通过（可能有残留引用，后续 task 修复）

---

### Task 8: 清理前端组件中对 summaries/digest 的引用

**Files:**
- Modify: `front/app/features/shell/components/FeedLayoutShell.vue` — 移除 AISummariesList/AISummaryDetail 引用
- Modify: `front/app/features/shell/components/AppSidebarView.vue` — 移除 ai-summaries sidebar 按钮
- Modify: `front/app/features/feeds/composables/useRefreshPolling.ts` — 移除 ai-summaries 排除检查
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue` — 移除 auto_summary scheduler 显示
- Modify: `front/app/features/ai/components/AIRouterSettingsPanel.vue` — 移除 updateAutoSummaryConfig 调用
- Modify: `front/app/utils/schedulerMeta.ts` — 移除 auto_summary 和 digest 条目
- Modify: `front/app/features/topic-graph/components/TopicGraphPage.vue` — 移除 summaries 引用
- Modify: `front/app/api/topicGraph.ts` — 移除 summaries 相关类型和方法
- Modify: `front/app/components/ai/AISummary.vue` — 移除（这是文章级 AI 总结组件，非 feed 级，确认是否保留）

**Step 1:** 逐一修改以上文件

**Step 2:** 运行 `pnpm exec nuxi typecheck` 确认类型检查通过

**Step 3:** 运行 `pnpm build` 确认构建通过

**Verify:** `pnpm exec nuxi typecheck` 和 `pnpm build` 通过

---

## Phase 5: 文档和测试清理

### Task 9: 清理文档和集成测试

**Files:**
- Delete: `docs/api/digest.md`
- Delete: `docs/guides/digest.md`
- Delete: `docs/guides/digest-setup-guide.md`
- Delete: `docs/plans/2026-03-10-open-notebook-digest-integration.md`
- Delete: `docs/plans/2026-03-10-open-notebook-digest-integration-design.md`
- Delete: `docs/plans/2026-03-22-digest-topic-aggregation.md`
- Delete: `docs/plans/2026-03-22-digest-topic-aggregation-design.md`
- Delete: `docs/plans/2026-03-04-ai-summary-phase1-implementation.md`
- Delete: `docs/plans/2026-03-04-ai-summary-enhancement-design.md`
- Delete: `docs/plans/260414-summary-article-markers.md`
- Delete: `docs/plans/2026-04-15-narrative-summary.md`（旧叙事计划，已实施）
- Modify: `docs/architecture/backend-go.md` — 移除 auto_summary 和 digest 引用
- Modify: `docs/architecture/backend-runtime.md` — 移除 AutoSummary 和 Digest 引用
- Modify: `docs/architecture/overview.md` — 移除 AutoSummary 和 Digest 引用
- Modify: `docs/architecture/tracing.md` — 移除相关引用
- Modify: `docs/api/schedulers.md` — 移除 auto_summary 和 digest 条目
- Modify: `docs/guides/content-processing.md` — 移除 auto_summary 引用
- Modify: `docs/guides/tagging-flow.md` — 移除 summary 阶段的 tagging 兜底描述
- Modify: `docs/guides/topic-graph.md` — 更新为反映叙事替代总结后的新架构
- Modify: `docs/database/DATABASE_FIELDS.md` — 移除 ai_summaries 相关字段
- Modify: `docs/operations/database.md` — 移除 ai_summaries 表
- Modify: `tests/workflow/utils/database.py` — 移除 ai_summaries DELETE
- Modify: `tests/workflow/config.py` — 移除 auto_summary 配置

**Step 1:** 删除文档文件

**Step 2:** 更新剩余文档中的引用

**Verify:** 文档内部一致性

---

## Phase 6: 最终验证

### Task 10: 全量验证

**Step 1:** `cd backend-go && go build ./...`
**Step 2:** `cd backend-go && go test ./...`
**Step 3:** `cd front && pnpm exec nuxi typecheck`
**Step 4:** `cd front && pnpm test:unit`
**Step 5:** `cd front && pnpm build`
**Step 6:** 确认无残留引用：`grep -r "AISummary\|auto_summary\|auto-summary\|ai_summaries\|SummaryQueue\|DigestScheduler\|DigestGenerator" backend-go/ front/ --include="*.go" --include="*.ts" --include="*.vue"` 返回空

**Verify:** 所有命令通过
