## MODIFIED Requirements

### Requirement: AICallLog records trace context
The `AICallLog` model SHALL include a `TraceID` field of type `string` (NULLABLE), and when writing a log entry, the system SHALL populate it from the current span context's trace ID. The `AICallLog` model SHALL also have a btree index on `created_at` for efficient retention-based cleanup.

#### Scenario: Successful LLM call records trace_id
- **WHEN** `Router.Chat` successfully calls an LLM provider within an active trace
- **THEN** the `AICallLog` row written has `trace_id` set to the current span's trace ID (32-char hex string)

#### Scenario: Failed LLM call still records trace_id
- **WHEN** `Router.Chat` fails on all provider attempts within an active trace
- **THEN** the `AICallLog` row for each failed attempt has `trace_id` set

#### Scenario: created_at index supports cleanup
- **WHEN** a DELETE query filters on `ai_call_logs.created_at`
- **THEN** the database uses the btree index on `created_at` (no sequential scan)

## REMOVED Requirements

### Requirement: Exporter-internal cleanup loop
**Reason**: Cleanup responsibility moved to `LogCleanupScheduler` for unified management and API observability.
**Migration**: The `SQLiteSpanExporter` no longer starts a background cleanup goroutine. Cleanup is handled by the scheduler system.
