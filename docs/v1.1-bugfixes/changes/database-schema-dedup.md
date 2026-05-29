# Database Schema Dedup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rename article/feed schema fields to match current Firecrawl and article-summary behavior, remove redundant article fields from active application use, and switch backend plus frontend to the new contract in one coordinated cutover.

**Architecture:** Keep SQLite migration safety by adding and backfilling new columns before application code switches over, but do not keep API compatibility aliases. Fresh databases should create only the new schema. Existing databases may retain old physical columns as migration leftovers, but runtime reads, writes, queries, and frontend payload mapping must use only the new names.

**Tech Stack:** Go, Gin, GORM, SQLite, Nuxt 4, Vue 3, TypeScript, Pinia, GitNexus

---

### Task 1: Re-check Impact Scope Before Editing

**Files:**
- Reference: `backend-go/internal/platform/database/db.go`
- Reference: `backend-go/internal/domain/models/article.go`
- Reference: `backend-go/internal/domain/models/feed.go`
- Reference: `backend-go/internal/domain/contentprocessing/content_completion_service.go`
- Reference: `backend-go/internal/domain/contentprocessing/content_completion_handler.go`
- Reference: `backend-go/internal/domain/contentprocessing/firecrawl_handler.go`
- Reference: `backend-go/internal/jobs/content_completion.go`
- Reference: `backend-go/internal/jobs/firecrawl.go`
- Reference: `backend-go/internal/domain/feeds/handler.go`
- Reference: `front/app/stores/api.ts`

- [ ] **Step 1: Check GitNexus index freshness**

Run:

```bash
npx gitnexus status
```

Expected: index exists; if stale, re-run analyze before continuing.

- [ ] **Step 2: Refresh index if needed**

Run:

```bash
npx gitnexus analyze
```

Expected: repo indexed successfully.

- [ ] **Step 3: Run impact analysis for the highest-risk symbols**

Run these first if MCP is healthy:

```text
gitnexus_impact({ target: "EnsureTables", direction: "upstream", repo: "my-robot" })
gitnexus_impact({ target: "runMigrations", direction: "upstream", repo: "my-robot" })
gitnexus_impact({ target: "ProcessCompletedFirecrawlJobs", direction: "upstream", repo: "my-robot" })
gitnexus_impact({ target: "ProcessPendingContentCompletionJobs", direction: "upstream", repo: "my-robot" })
gitnexus_impact({ target: "UpdateFeed", direction: "upstream", repo: "my-robot" })
```

Expected: identify d=1 callers and affected flows before code changes.

- [ ] **Step 4: Use grep fallback if MCP still fails**

Run:

```bash
rg "content_completion_enabled|content_status|content_fetched_at|full_content|firecrawl_enabled" backend-go front docs
```

Expected: concrete edit list for direct-cutover work.

### Task 2: Write Regression Tests First

**Files:**
- Modify: `backend-go/internal/domain/feeds/service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_handler_test.go`
- Modify: `backend-go/internal/jobs/content_completion_test.go`
- Modify: `backend-go/internal/jobs/auto_summary_test.go`
- Modify: `front/app/utils/articleContentSource.test.ts`

- [ ] **Step 1: Add feed-level field rename tests**

Write tests that assert article-summary scheduling uses `article_summary_enabled`, not `content_completion_enabled`.

- [ ] **Step 2: Add article-level field rename tests**

Write tests that assert article pipeline state uses `summary_status` and `summary_generated_at`.

- [ ] **Step 3: Add removal coverage for `full_content`**

Update the frontend content-source test so content fallback still prefers the intended order after `full_content` disappears.

- [ ] **Step 4: Run targeted tests and confirm RED**

Run:

```bash
go test ./internal/domain/feeds ./internal/domain/contentprocessing ./internal/jobs -v
pnpm test:unit -- app/utils/articleContentSource.test.ts
```

Expected: failures point at old field names or stale mappings.

### Task 3: Cut Over Models And SQLite Migrations

**Files:**
- Modify: `backend-go/internal/domain/models/article.go`
- Modify: `backend-go/internal/domain/models/feed.go`
- Modify: `backend-go/internal/platform/database/db.go`

- [ ] **Step 1: Rename the active model fields**

Use this target shape:

```go
// Feed
ArticleSummaryEnabled bool `gorm:"default:false" json:"article_summary_enabled"`

// Article
SummaryStatus      string     `gorm:"size:20;default:complete" json:"summary_status"`
SummaryGeneratedAt *time.Time `json:"summary_generated_at"`
```

- [ ] **Step 2: Remove deprecated article fields from active models**

Delete active model usage of:

```go
FullContent
FirecrawlEnabled
```

- [ ] **Step 3: Update fresh-database DDL**

Edit `EnsureTables` so new databases create only:

```text
feeds.article_summary_enabled
articles.summary_status
articles.summary_generated_at
```

and do not recreate `articles.full_content` or `articles.firecrawl_enabled`.

- [ ] **Step 4: Add additive runtime migrations**

Implement add-and-backfill steps for:

```text
feeds.article_summary_enabled <- feeds.content_completion_enabled
articles.summary_status <- articles.content_status
articles.summary_generated_at <- articles.content_fetched_at
```

- [ ] **Step 5: Remove deprecated columns from legacy migration lists**

Ensure the bootstrap path and `runMigrations()` no longer add old columns for fresh installs.

- [ ] **Step 6: Run targeted backend tests again**

Run:

```bash
go test ./internal/domain/feeds ./internal/domain/contentprocessing ./internal/jobs -v
```

Expected: model and migration layer compiles; downstream query failures may remain.

### Task 4: Switch Backend Runtime Logic In One Pass

**Files:**
- Modify: `backend-go/internal/domain/feeds/service.go`
- Modify: `backend-go/internal/domain/feeds/handler.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_handler.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_handler.go`
- Modify: `backend-go/internal/jobs/firecrawl.go`
- Modify: `backend-go/internal/jobs/content_completion.go`
- Modify: `backend-go/internal/jobs/auto_summary.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue.go`
- Modify: `backend-go/internal/domain/topicgraph/article_tagger.go`

- [ ] **Step 1: Replace all struct field reads and writes**

Change code such as:

```go
article.ContentStatus
article.ContentFetchedAt
feed.ContentCompletionEnabled
```

to:

```go
article.SummaryStatus
article.SummaryGeneratedAt
feed.ArticleSummaryEnabled
```

- [ ] **Step 2: Replace all SQL predicates and updates**

Change queries such as:

```go
Where("articles.content_status = ?", "incomplete")
Where("feeds.content_completion_enabled = ?", true)
```

to:

```go
Where("articles.summary_status = ?", "incomplete")
Where("feeds.article_summary_enabled = ?", true)
```

- [ ] **Step 3: Remove request/response compatibility branches**

Do not accept old request keys or return old response aliases. `article_summary_enabled`, `summary_status`, and `summary_generated_at` become the only active API names.

- [ ] **Step 4: Remove stale payload fields from handlers**

Stop serializing `full_content` and article-level `firecrawl_enabled` from backend handlers and helper maps.

- [ ] **Step 5: Run targeted backend tests to GREEN**

Run:

```bash
go test ./internal/domain/feeds ./internal/domain/contentprocessing ./internal/jobs -v
```

Expected: targeted packages pass.

### Task 5: Switch Frontend Types And Mapping To New Contract

**Files:**
- Modify: `front/app/types/feed.ts`
- Modify: `front/app/types/article.ts`
- Modify: `front/app/stores/api.ts`
- Modify: `front/app/utils/articleContentSource.ts`
- Modify: `front/app/features/articles/composables/useArticleProcessingStatus.ts`
- Modify: `front/app/features/articles/composables/useContentCompletion.ts`
- Modify: `front/app/features/articles/components/ArticleContentView.vue`
- Modify: `front/app/features/articles/components/ContentCompletionView.vue`
- Modify: `front/app/features/articles/components/ArticleCardView.vue`
- Modify: `front/app/features/topic-graph/components/TopicGraphPage.vue`
- Modify: `front/app/features/shell/components/ArticleListPanelView.vue`
- Modify: `front/app/components/dialog/EditFeedDialog.vue`
- Modify: `front/app/api/firecrawl.ts`

- [ ] **Step 1: Rename frontend payload types**

Switch API-facing interfaces to:

```ts
article_summary_enabled
summary_status
summary_generated_at
```

- [ ] **Step 2: Update store mapping**

Edit `front/app/stores/api.ts` to map only the new backend keys and stop reading old names.

- [ ] **Step 3: Remove deleted fields from UI models**

Delete active `fullContent` and article-level `firecrawlEnabled` usage where they only mirror removed backend fields.

- [ ] **Step 4: Re-check content source and status UI**

Update article processing and content display utilities so they still show the correct source and article-summary state after the field removals.

- [ ] **Step 5: Run frontend verification**

Run:

```bash
pnpm exec nuxi typecheck
pnpm test:unit -- app/utils/articleContentSource.test.ts
```

Expected: typecheck and targeted unit test pass.

### Task 6: Rewrite Docs To The New Contract

**Files:**
- Modify: `docs/database/DATABASE_FIELDS.md`
- Modify: `docs/architecture/backend-go.md`
- Modify: `docs/architecture/data-flow.md`
- Modify: `docs/architecture/frontend.md`
- Modify: `docs/operations/development.md`

- [ ] **Step 1: Search docs for stale primary terminology**

Run:

```bash
rg "content_completion_enabled|content_status|content_fetched_at|full_content|articles.firecrawl_enabled" docs
```

Expected: old terms still appear and need replacement.

- [ ] **Step 2: Rewrite docs to the new schema**

Make the new contract primary and move old names, if mentioned at all, into migration notes only.

- [ ] **Step 3: Re-run the doc search**

Run the same command and verify old names remain only in historical or migration context.

### Task 7: Final Verification And Scope Check

**Files:**
- Modify: remaining references found by search

- [ ] **Step 1: Run whole-repo stale-name search**

Run:

```bash
rg "content_completion_enabled|content_status|content_fetched_at|full_content|firecrawl_enabled" backend-go front docs
```

Expected: only feed-level `firecrawl_enabled`, migration backfill code, or intentional historical doc references remain.

- [ ] **Step 2: Run focused verification commands**

Run:

```bash
go test ./internal/domain/feeds ./internal/domain/contentprocessing ./internal/jobs -v
pnpm exec nuxi typecheck
pnpm test:unit -- app/utils/articleContentSource.test.ts
```

- [ ] **Step 3: Run broader backend verification**

Run:

```bash
go test ./...
```

Expected: full backend suite passes or only known unrelated failures remain and are documented.

- [ ] **Step 4: Check final change scope with GitNexus**

If MCP is healthy, run:

```text
gitnexus_detect_changes({ scope: "all", repo: "my-robot" })
```

If MCP is still unhealthy, use:

```bash
git diff -- backend-go front docs
```

and compare the changed files against this plan.

- [ ] **Step 5: Commit**

```bash
git add backend-go front docs
git commit -m "refactor: align article summary schema"
```
