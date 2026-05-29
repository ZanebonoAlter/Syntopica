## ADDED Requirements

### Requirement: Scheduled log table cleanup
The system SHALL run a `LogCleanupScheduler` that executes every 24 hours and deletes rows older than 7 days from both `ai_call_logs` and `otel_spans` tables, in that order.

#### Scenario: Cron cleanup removes expired rows from both tables
- **WHEN** the scheduler ticker fires (every 24h)
- **THEN** the system deletes all rows from `ai_call_logs` where `created_at < NOW() - 7 days`, followed by all rows from `otel_spans` where `start_time_unix_nano < (NOW() - 7 days).UnixNano()`

#### Scenario: Startup delay before first cleanup
- **WHEN** the scheduler starts
- **THEN** the first execution SHALL occur after a 5-minute delay, not immediately

#### Scenario: No rows to clean
- **WHEN** the cleanup runs and no rows are older than 7 days
- **THEN** the scheduler logs "no rows to clean" and reports zero rows affected

### Requirement: Manual trigger support
The `LogCleanupScheduler` SHALL support manual triggering via the scheduler API (`POST /api/schedulers/log_cleanup/trigger`).

#### Scenario: Manual trigger from API
- **WHEN** a POST request is sent to `/api/schedulers/log_cleanup/trigger`
- **THEN** the cleanup executes immediately and returns the number of rows deleted per table

#### Scenario: Manual trigger while already running
- **WHEN** a manual trigger is requested while a cleanup cycle is executing
- **THEN** the request returns 409 with `accepted: false` and reason `already_running`

### Requirement: ai_call_logs created_at index
The `ai_call_logs` table SHALL have a btree index on the `created_at` column to support efficient range deletes.

#### Scenario: Index exists for cleanup query
- **WHEN** the cleanup DELETE executes on `ai_call_logs`
- **THEN** the query uses the `created_at` index (no sequential scan)

### Requirement: Scheduler status observable via API
The `LogCleanupScheduler` SHALL report its status through `GetStatus()` following the existing scheduler interface, including: status, check_interval, is_executing, last execution time, rows cleaned per table, and any errors.

#### Scenario: Status response includes cleanup metrics
- **WHEN** `GET /api/schedulers/log_cleanup` is called
- **THEN** the response includes `last_ai_call_logs_deleted` and `last_otel_spans_deleted` counts
