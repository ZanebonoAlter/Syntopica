## 1. Database Migration

- [x] 1.1 Add `created_at` btree index to `AICallLog` model in `backend-go/internal/domain/models/ai_models.go`

## 2. LogCleanupScheduler Core

- [x] 2.1 Create `backend-go/internal/jobs/log_cleanup.go` with `LogCleanupScheduler` struct following `BlockedArticleRecoveryScheduler` pattern (Start/Stop/TriggerNow/GetStatus/ResetStats/UpdateInterval)
- [x] 2.2 Implement cleanup logic: delete `ai_call_logs` rows where `created_at < NOW() - 7 days` (uses `created_at` index), then delete `otel_spans` rows where `start_time_unix_nano < (NOW() - 7 days).UnixNano()` (uses existing `idx_otel_spans_start_time` index). Log affected counts per table.
- [x] 2.3 Add 5-minute startup delay before first execution

## 3. Registration

- [x] 3.1 Add `LogCleanupSchedulerInterface` to `backend-go/internal/app/runtimeinfo/schedulers.go`
- [x] 3.2 Register scheduler in `backend-go/internal/app/runtime.go` StartRuntime and SetupGracefulShutdown
- [x] 3.3 Add `log_cleanup` entry to `schedulerDescriptors()` in `backend-go/internal/jobs/handler.go`

## 4. Remove Old Cleanup

- [x] 4.1 Remove `cleanupLoop` and `cleanExpiredSpans` from `backend-go/internal/platform/tracing/exporter.go`
- [x] 4.2 Remove `stopCh` field and `go exporter.cleanupLoop()` from `NewSQLiteSpanExporter`

## 5. Verification

- [x] 5.1 Run `go build ./...` and `go vet ./...` in backend-go
- [ ] 5.2 Start server, verify `GET /api/schedulers` includes `log_cleanup`
- [ ] 5.3 Trigger cleanup via `POST /api/schedulers/log_cleanup/trigger`, verify rows deleted
