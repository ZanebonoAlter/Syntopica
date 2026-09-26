# architecture-docs

## Purpose

TBD: This spec governs the accuracy and maintenance of architecture documentation files (`docs/reference/architecture/*.md`) relative to the actual backend codebase.

## Requirements

### Requirement: backend.md SHALL accurately describe the current directory structure

The `architecture/backend.md` document SHALL reflect the actual `backend-go/` directory structure as it exists in the codebase.

#### Scenario: cmd directory matches code
- **WHEN** a developer reads the directory tree in `architecture/backend.md`
- **THEN** the `cmd/` tree SHALL list only `server/`
- **AND** SHALL NOT list `migrate-db/`, `migrate-tags/`, `migrate-embedding-queue/`, `migrate-digest/`, `test-digest/`, or `test-embedding/` (none exist in `backend-go/cmd/`)

#### Scenario: internal directory matches code
- **WHEN** a developer reads the directory tree in `architecture/backend.md`
- **THEN** the `internal/` tree SHALL match `backend-go/internal/`（含 `admin/`、`discovery/`、`reader/`、`tagmanagement/`、`topicgraph/`、`dataenrichment/`、`datasources/`、`platform/`、`models/`、`app/`；各业务域含自己的 `models/` 子包）
- **AND** SHALL NOT reference deleted directories (`internal/domain/`, `internal/jobs/`, `internal/app/runtimeinfo/`)

#### Scenario: platform subpackages match code
- **WHEN** a developer reads the `internal/platform/` section
- **THEN** the listed subpackages SHALL match `ls backend-go/internal/platform/`（decouple-backend-domains 后含 `scheduler/` 调度器框架子包，完整清单以 `ls` 实际输出为准）
- **AND** SHALL NOT list `ai/` or `opennotebook/` (neither exists)

### Requirement: backend.md SHALL use correct tech stack

The `architecture/backend.md` document SHALL describe the actual technology stack.

#### Scenario: Go version is correct
- **WHEN** a developer reads the "技术栈" section
- **THEN** the Go version SHALL be `1.25` (matching `go.mod`'s `go 1.25.0`)

#### Scenario: No reference to removed cron dependency
- **WHEN** a developer reads the "技术栈" section
- **THEN** the document SHALL NOT list `robfig/cron` (not present in `go.mod` or source imports)
- **AND** SHALL describe the scheduler framework (BaseScheduler factory + JobFunc + Interval) as located in `internal/platform/scheduler`, with domain job definitions (`job_*.go`) remaining in `internal/admin/scheduler`

### Requirement: backend.md SHALL list all registered schedulers

The `architecture/backend.md` document SHALL list the schedulers registered in `runtime.go`.

#### Scenario: Scheduler list is complete and accurate
- **WHEN** a developer reads the scheduler section
- **THEN** the list SHALL include exactly these 9 schedulers (matching `runtime.go`): `auto_refresh`, `preference_update`, `content_completion`, `firecrawl`, `blocked_article_recovery`, `daily_report`, `tag_quality_score`, `log_cleanup`, `aux_label_cleanup`
- **AND** SHALL NOT include `auto_tag_merge` or `narrative_summary` (not registered in `runtime.go`)

### Requirement: backend.md SHALL NOT reference removed runtimeinfo or AutoTagMerge

The `architecture/backend.md` document SHALL NOT describe the removed `runtimeinfo` global Interface sharing mechanism or the removed `AutoTagMerge` scheduler.

#### Scenario: No runtimeinfo references
- **WHEN** a developer reads `architecture/backend.md`
- **THEN** the document SHALL NOT reference `internal/app/runtimeinfo/schedulers.go`
- **AND** SHALL describe scheduler sharing via `SchedulerRegistry` (`registry.Register`)

#### Scenario: No AutoTagMerge scheduler references
- **WHEN** a developer reads the "Tag 合并" section
- **THEN** the document SHALL NOT claim an `AutoTagMerge` scheduler auto-triggers on pgvector cosine similarity > 0.97 (no such scheduler is registered in `runtime.go`)

### Requirement: overview.md SHALL use correct backend paths

The `architecture/overview.md` document SHALL reference the correct backend package paths.

#### Scenario: No references to deleted packages
- **WHEN** a developer reads `architecture/overview.md`
- **THEN** the document SHALL NOT reference `backend-go/internal/domain/feed/`, `internal/domain/article/`, `internal/domain/content/`, `internal/domain/preferences/`, or `internal/domain/narrative/`
- **AND** SHALL NOT reference `NarrativeSummaryScheduler`
- **AND** SHALL use `internal/reader/`, `internal/admin/`, `internal/tagmanagement/`, `internal/topicgraph/` instead

#### Scenario: cmd directory is correct
- **WHEN** a developer reads the directory tree in `architecture/overview.md`
- **THEN** the `cmd/` annotation SHALL list only `server/`

#### Scenario: No reference to removed cron dependency or phantom packages
- **WHEN** a developer reads the tech stack table or platform annotation
- **THEN** the "定时任务" row SHALL NOT reference `robfig/cron`
- **AND** the platform annotation SHALL NOT list `ai` or `opennotebook`

### Requirement: overview.md SHALL describe scheduler count correctly

The `architecture/overview.md` document SHALL state the correct number of registered schedulers.

#### Scenario: Scheduler count matches runtime.go
- **WHEN** a developer reads the "后台调度器一览" or "统一调度器管理" sections
- **THEN** the document SHALL NOT state "8 类" (there are 9 schedulers registered in `runtime.go`)
- **AND** SHALL NOT list `AutoTagMerge` as a scheduler
- **AND** SHALL NOT reference a `jobs/` directory (no such directory exists)

### Requirement: runtime.md SHALL describe the SchedulerRegistry pattern

The `architecture/runtime.md` document SHALL describe runtime state sharing via the `SchedulerRegistry`——框架（registry/base/persistence）位于 `internal/platform/scheduler`（decouple-backend-domains 迁自 admin/scheduler），注册发生在 `internal/app/runtime.go` 的 `StartRuntime()`——not the removed `runtimeinfo` global Interface pattern.

#### Scenario: No references to removed runtimeinfo interfaces
- **WHEN** a developer reads `architecture/runtime.md`
- **THEN** the document SHALL NOT reference `internal/app/runtimeinfo/schedulers.go`
- **AND** SHALL NOT reference the removed global Interface variables (`AutoRefreshSchedulerInterface`, `PreferenceUpdateSchedulerInterface`, `AISummarySchedulerInterface`, `FirecrawlSchedulerInterface`, `AutoTagMergeSchedulerInterface`, `TagQualityScoreSchedulerInterface`, `NarrativeSummarySchedulerInterface`)
- **AND** SHALL describe that schedulers are registered via `registry.Register(name, scheduler.New(...))` in `StartRuntime()`

#### Scenario: No references to removed worker package functions
- **WHEN** a developer reads the worker startup description
- **THEN** the document SHALL NOT reference `topicextraction.GetTagQueue().Start()`, `topicanalysis.StartEmbeddingQueueWorker()`, or `topicanalysis.StartMergeReembeddingQueueWorker()` (these packages are deleted)
- **AND** SHALL describe worker startup via `tagging.StartAllWorkers()` (the unified entry in `internal/tagmanagement`)

#### Scenario: Scheduler list matches runtime.go
- **WHEN** a developer reads the scheduler section
- **THEN** the list SHALL match the 9 schedulers registered in `backend-go/internal/app/runtime.go`
- **AND** SHALL NOT reference `internal/jobs/handler.go`, `internal/jobs/content_completion.go`, `internal/jobs/narrative_summary.go`, or `internal/jobs/tag_hierarchy_cleanup.go` (the `jobs/` package is deleted)

### Requirement: tracing.md SHALL reference correct file paths

The `architecture/tracing.md` document SHALL reference file paths that actually exist in the codebase.

#### Scenario: Auto-injection table uses real paths
- **WHEN** a developer reads the "已落地的自动注入方法" table
- **THEN** the file column SHALL reference `internal/reader/service/feed_service.go`, `internal/reader/service/firecrawl_service.go`, and `internal/reader/service/content_completion_service.go`
- **AND** SHALL NOT reference `internal/domain/feeds/service.go` or `internal/domain/contentprocessing/*` (deleted)

#### Scenario: No phantom scheduler references
- **WHEN** a developer reads `architecture/tracing.md`
- **THEN** the document SHALL NOT reference `narrative_summary` as a scheduler

### Requirement: data-flow.md SHALL NOT reference phantom schedulers

The `architecture/data-flow.md` document SHALL NOT describe data flows triggered by schedulers that do not exist.

#### Scenario: No NarrativeSummaryScheduler references
- **WHEN** a developer reads the narrative data flow section
- **THEN** the document SHALL NOT claim flows are triggered by `NarrativeSummaryScheduler` (not registered in `runtime.go`)
- **AND** SHALL align trigger descriptions with the actual schedulers registered in `runtime.go`
