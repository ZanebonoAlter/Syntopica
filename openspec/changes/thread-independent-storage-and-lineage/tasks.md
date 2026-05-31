## 1. Database Migration

- [x] 1.1 Add migration `20260529_0001` in `postgres_migrations.go` to create `daily_report_threads` table with all columns (id, report_id, section_id, title, summary, status, tag_ids, confidence, prev_thread_id, created_at) and indexes (report_id, section_id, prev_thread_id partial WHERE NOT NULL)
- [x] 1.2 Add migration `20260529_0002` to extract existing thread data from `daily_report_sections.threads` JSONB into `daily_report_threads` rows. SQL SHALL map JSON field names correctly: `related_tag_ids` → `tag_ids`, skip `related_article_ids` and `parent_thread_id`. `prev_thread_id` left NULL for historical data
- [x] 1.3 Add migration `20260529_0003` to drop `threads` column from `daily_report_sections` table

## 2. Backend Model & Repository

- [x] 2.1 Add `DailyReportThread` GORM model struct in `models.go` with TableName() method, mapping to `daily_report_threads` table. Include all fields: ID, ReportID, SectionID, Title, Summary, Status, TagIDs (JSONB, json tag `"tag_ids"`), Confidence, PrevThreadID (*uint, json tag `"prev_thread_id"`), CreatedAt. Use `json:"tag_ids"` (not `related_tag_ids`)
- [x] 2.2 In `models.go`: Remove `Threads JSON` field (`gorm:"type:jsonb" json:"threads"`) from `DailyReportSection`. Replace with GORM association field `Threads []DailyReportThread \`gorm:"foreignKey:SectionID" json:"threads,omitempty"\``
- [x] 2.3 Add thread repository functions in `repository.go`: `SaveThreads(reportID, sectionID uint, threads []DailyReportThread) error`, `GetThreadsBySection(sectionID uint)`, `GetThreadsByReport(reportID uint)`, `GetThreadByID(id uint)`, `DeleteThreadsByReport(reportID uint) error` (for upsert cleanup)
- [x] 2.4 Add `GetThreadLineage(threadID uint)` in `repository.go` using recursive CTE to fetch full chain (ancestors + descendants) with period_date from joined report
- [x] 2.5 Add `GetBoardThreadTimeline(boardID uint, days int)` in `repository.go` to fetch all threads for a board within date range, joining period_date from reports and cluster_label from sections
- [x] 2.6 Update `GetReportByID()` to preload threads for each section via nested Preload `"Sections.Threads"` (replaces JSON unmarshaling)
- [x] 2.7 Update `SaveReport()` to handle threads: (a) Before deleting old sections, set `prev_thread_id=NULL` on any downstream threads that reference this report's threads; (b) After deleting old sections, delete old threads; (c) After inserting new sections, insert new threads with correct report_id and section_id

## 3. Backend Generator & Handler

- [x] 3.1 Update `matchPreviousThreads()` in `generator.go` to accept previous threads as `[]DailyReportThread` (with DB IDs) and assign `PrevThreadID` using the best-match thread's DB ID
- [x] 3.2 Update `getPrevThreadSummaries()` in `generator.go` to return `[]DailyReportThread` instead of `[]string`, reading from `prevReport.Sections[*].Threads` (now GORM-loaded, not JSON)
- [x] 3.3 Update `findPreviousReport()` to Preload `"Sections.Threads"` so matchPreviousThreads/getPrevThreadSummaries have thread DB IDs available
- [x] 3.4 Update `GenerateDailyReport()` return signature to `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)`. After LLM thread generation and matching, convert `[]Thread` to `[]DailyReportThread` for each cluster. Section building no longer sets `Threads` JSON field. Update `generateSingleBoard` caller to handle new signature and pass threads to SaveReport
- [x] 3.5 Add `GET /api/daily-reports/threads/:id/lineage` handler in `handler.go` calling `GetThreadLineage`
- [x] 3.6 Add `GET /api/semantic-boards/:id/thread-timeline` handler in `handler.go` calling `GetBoardThreadTimeline`
- [x] 3.7 Update report detail API response: `GetReportByID` with nested Preload `"Sections.Threads"` ensures each section includes a `threads` array of `DailyReportThread` objects (with id, prev_thread_id, report_id, section_id, etc.)

## 4. Frontend API Types

- [x] 4.1 Update `DailyReportThread` interface in `dailyReports.ts`: add `id: number`, `prev_thread_id: number | null`, `report_id: number`, `section_id: number`; rename `related_tag_ids` to `tag_ids`; rename `parent_thread_id` (string) to remove (replaced by `prev_thread_id`); keep `related_article_ids`
- [x] 4.2 Add `getThreadLineage(threadId: number)` API function in `dailyReports.ts`
- [x] 4.3 Add `getBoardThreadTimeline(boardId: number, days?: number)` API function in `dailyReports.ts`

## 5. Frontend Thread Detail Panel (View A)

- [x] 5.1 Create `ThreadLineagePanel.vue` component: side panel within newspaper modal, fetches lineage via `getThreadLineage(threadId)`, renders vertical timeline with date nodes, status badges, title, summary; highlights current thread; provides close button
- [x] 5.2 Update `BoardDailyReportTimeline.vue`: (a) Split thread click area — clicking thread title/body opens ThreadLineagePanel; clicking the article icon retains the existing article popup; (b) Pass clicked thread's `id` to the panel; (c) Adjust modal layout to accommodate side panel (split view: newspaper + lineage side panel)

## 6. Frontend Board Thread Browser (View B)

- [x] 6.1 Create `BoardThreadBrowser.vue` component: fetches data via `getBoardThreadTimeline(boardId, days)`, builds lineage chains client-side from flat thread list using prev_thread_id, renders Gantt-chart grid (columns=dates, rows=lineage chains, nodes=colored status dots with connecting lines)
- [x] 6.2 Add node click interaction in BoardThreadBrowser: clicking a node shows thread detail popup (title, summary, status, date)
- [x] 6.3 Add days range selector (7/14/30/60 toggle buttons) in BoardThreadBrowser
- [x] 6.4 Add "线程总览" toggle button in `BoardDailyReportTimeline.vue` to switch between the report list view and the BoardThreadBrowser view

## 7. Verification

- [x] 7.1 Backend: `go build ./...` and `go vet ./...` pass
- [x] 7.2 Backend: targeted `go test ./internal/domain/daily_report/...` passes
- [x] 7.3 Frontend: `pnpm lint` passes
- [x] 7.4 Frontend: `pnpm exec nuxi typecheck` passes (via Windows cmd)
- [x] 7.5 Frontend: `pnpm build` passes (via Windows cmd)

## 8. Section Embedding 语义匹配

- [ ] 8.1 添加数据库迁移：`daily_report_sections` 表增加 `embedding vector(2560)` 列。添加 HNSW 索引 (`embedding vector_cosine_ops`)。
- [ ] 8.2 更新 `DailyReportSection` GORM 模型：增加 `Embedding` 字段（`gorm:"type:vector(2560)"`）。
- [ ] 8.3 在 `generator.go` 的 `GenerateDailyReport()` 中，section 生成完毕后批量调用 `Router.Embed()` 为所有 section 的 `cluster_label` 生成 embedding 向量（一次批量 API 调用）。将 embedding 设置到 section 的 `Embedding` 字段。
- [ ] 8.4 更新 `SaveReport()`：存储 section 时同时写入 embedding 字段。在同一事务内，对每个新 section 使用 pgvector `<=>` 查询同板块内（排除当前 report）的最近邻 section，如果余弦距离 < 0.3 则设置 `prev_section_id` 和 `status='continuing'`。
- [ ] 8.5 新增 repository 函数 `MatchSectionsByEmbedding(boardID uint, reportID uint, sectionEmbeddings map[uint]string)`：在 DB 侧为当前 report 的每个 section 找最近邻。或者直接嵌入 `SaveReport()` 流程中作为一个步骤。
- [ ] 8.6 替换 `matchPreviousSections()` 调用：在 `GenerateDailyReport()` 中移除对 `findPreviousSections()` + `matchPreviousSections()` 的调用，改为在 `SaveReport()` 事务内通过 pgvector 完成匹配。
- [ ] 8.7 清理废弃代码：移除 `findPreviousSections()` 函数和 `matchPreviousSections()` 函数（Go 侧 tag Jaccard 匹配逻辑）。

## 9. 历史数据 Embedding 回填

- [ ] 9.1 新增回填函数 `BackfillSectionEmbeddings()`：查询所有 `embedding IS NULL` 的 `daily_report_sections`，按批量为每条的 `cluster_label` 调用 Embedding API，更新 `embedding` 列。
- [ ] 9.2 回填完成后，对所有已有 embedding 的 section，按板块分组执行 pgvector 匹配，更新 `prev_section_id`（仅在 `prev_section_id IS NULL` 的记录上执行，避免覆盖已有值）。
- [ ] 9.3 添加 CLI/API 触发入口（例如通过 handler 或命令行），允许手动触发回填。

## 10. 验证

- [ ] 10.1 Backend: `go build ./...` and `go vet ./...` pass
- [ ] 10.2 Backend: targeted `go test ./internal/domain/daily_report/...` passes
- [ ] 10.3 验证新数据：生成一次日报后，检查 section 的 `embedding` 和 `prev_section_id` 是否正确填充
- [ ] 10.4 验证回填：运行回填后，检查历史 section 的 embedding 和 prev_section_id
