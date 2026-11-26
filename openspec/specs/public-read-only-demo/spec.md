<!-- domain: dev -->

# Spec — public-read-only-demo

## Purpose

Provide a self-contained, one-command Docker Compose demo that lets any visitor browse the full Syntopica application in read-only mode without external credentials, RSS sources, pre-existing data, or local Go/Frontend toolchains. The demo builds and seeds itself from scratch, enforces strict read-only semantics, and zeros out all production-side effects when inactive.

## Requirements

### Requirement: One-command read-only demo launch

The system SHALL provide a self-contained Docker Compose configuration that launches a fully browsable, read-only demo instance with a single command, requiring no external AI credentials, no RSS sources, and no pre-existing data.

#### Scenario: First-time visitor launches the demo

- **WHEN** a user runs `docker compose -f demo/docker-compose.demo.yml up -d --build` on a machine with only Docker installed
- **THEN** the PostgreSQL (pgvector) and syntopica-demo containers start, the schema is auto-created via AutoMigrate + migrations, the sanitized seed data is imported, and `http://localhost:5000` serves the full application within the startup window
- **AND** all four primary pages (home `/`, topic graph `/topics`, tag management `/tags` with detective wall, settings `/settings`) render real-looking content without errors

#### Scenario: Read-only enforcement blocks all mutations

- **WHEN** the demo is launched with `DEMO_READ_ONLY=1`
- **THEN** all non-GET requests to `/api/*` return HTTP 405 with `{"error":"read-only demo"}`
- **AND** the two side-effecting GET SSE endpoints (`/api/topic-tags/merge-preview/scan/stream`, `/api/topic-tags/merge-preview/evaluate/stream`) return 405
- **AND** no background schedulers run (no RSS refresh, no LLM daily-report generation, no firecrawl crawling)
- **AND** the `/ws` WebSocket endpoint is not registered (frontend degrades silently)

### Requirement: Sanitized seed data export tool

The system SHALL provide a Go CLI tool (`backend-go/cmd/dump-sanitizer`) that connects to a real running database, exports recent business data (configurable window, default 30 days), applies conservative sanitization, and emits a portable `seed.sql` reproducible on a fresh empty database.

#### Scenario: Export produces a sanitized, import-safe seed file

- **WHEN** the operator runs `cd backend-go && go run ./cmd/dump-sanitizer` against a populated database
- **THEN** `demo/seed/seed.sql` is generated containing `INSERT` statements in topological order (respecting the `topic_tags_merged_into_id_fkey` self-reference by splitting `topic_tags` into two batches)
- **AND** the file contains no `api_providers.api_key` values, no `ai_call_logs` rows, no `schema_migrations` rows, and all `reading_behaviors.session_id` values are SHA-256 hashed
- **AND** all pgvector embedding columns (`topic_tag_embeddings.embedding`, `semantic_labels.embedding`, `semantic_labels.merge_embedding`, `daily_report_sections.embedding`) are emitted as NULL
- **AND** each exported table is followed by a `SELECT setval(...)` statement resetting its `id` sequence to `MAX(id)+1`
- **AND** the file is idempotent when re-imported into a fresh database (re-running `docker compose down && up` yields consistent data)

#### Scenario: Sanitization closes token leakage and unique-key collapse

- **WHEN** a sensitive token literal (e.g. `api_key`, `API_KEY`) appears inside `articles.content` or `articles.ai_content_summary`, or a configuration value is stored in `ai_settings.value`
- **THEN** the exporter replaces those token literals with `[redacted-token]` before capping the text to 2000 characters, and clears every `ai_settings.value` to `{}`
- **AND** for tables whose sanitization can collapse distinct values onto the same unique key (e.g. `feeds.url` after query-string stripping), the emitted `INSERT` carries the configured `ConflictClause` (e.g. `ON CONFLICT (url) DO NOTHING`) so re-import never fails on a unique violation

#### Scenario: Seed import clears bootstrap data before inserting

- **WHEN** the demo container starts against a fresh database
- **THEN** the entrypoint starts the backend, waits for `/health`, and runs `TRUNCATE TABLE <all demo tables> RESTART IDENTITY CASCADE` before importing `seed.sql`
- **AND** this avoids duplicate-key conflicts with the default rows (ai_settings, embedding_config, etc.) that migrations seed at backend boot

### Requirement: Self-contained demo image with same-origin frontend

The system SHALL build a demo Docker image that compiles both frontend and backend from source (no pre-built local binary), serving the frontend static files from the backend on a single port with same-origin API access.

#### Scenario: Image builds without local prerequisites

- **WHEN** `docker compose -f demo/docker-compose.demo.yml build` runs on a clean checkout
- **THEN** the frontend is built with `NUXT_PUBLIC_API_BASE=/api` injected at generate time (same-origin relative path, no absolute localhost URL baked in)
- **AND** the Go backend is compiled inside the image (multi-stage, no host `go build` required)
- **AND** the runtime stage includes `curl` and `postgresql-client` for the entrypoint health-check and seed import

#### Scenario: Build tolerates flaky upstream networks and a pre-existing lockfile mismatch

- **WHEN** the Go module proxy or Docker registry is intermittent during build
- **THEN** the Dockerfile declares an overridable `ARG GOPROXY` (defaulting to a CN mirror) and wraps `go mod download` in a short retry loop, and omits the `# syntax=docker/dockerfile:1` directive so no extra frontend image is pulled
- **AND** when the repo `pnpm-lock.yaml` declares a `patchedDependencies` entry absent from `package.json`, the frontend stage installs with `--no-frozen-lockfile` (still honouring the lockfile for resolution) rather than failing the frozen check

#### Scenario: Entrypoint imports seed deterministically on a fresh database

- **WHEN** the demo container starts
- **THEN** the entrypoint launches the backend, waits for `/health`, truncates all demo tables (RESTART IDENTITY CASCADE), imports `seed.sql`, and then waits on the backend process
- **AND** the import succeeds without duplicate-key errors despite migrations seeding default rows at backend boot

### Requirement: Zero impact on production paths when demo mode is inactive

The read-only middleware and runtime-skip logic MUST be no-ops when `DEMO_READ_ONLY` is unset or not `"1"`, ensuring production deployments are unaffected.

#### Scenario: Production deployment behaves identically without the flag

- **WHEN** the server starts without `DEMO_READ_ONLY` set
- **THEN** the read-only middleware passes all requests through unconditionally
- **AND** `StartRuntime()` and graceful-shutdown wiring execute exactly as before
- **AND** all write endpoints and schedulers function normally
