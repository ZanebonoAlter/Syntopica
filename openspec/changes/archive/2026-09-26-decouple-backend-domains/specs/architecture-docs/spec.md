# architecture-docs Specification Delta

## MODIFIED Requirements

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
