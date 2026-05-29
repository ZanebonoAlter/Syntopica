# AI Router Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a unified AI routing layer with provider failover, capability-based routing, and decoupled configuration storage across all AI-powered backend flows.

**Architecture:** Replace the monolithic `summary_config` JSON with normalized AI provider and route tables, then route all AI-capable modules through a shared router service. Keep Firecrawl and other non-LLM service settings separate, and preserve backward compatibility through a migration layer during rollout.

**Tech Stack:** Go, Gin, GORM, SQLite, Nuxt 4, Vue 3, TypeScript

---

### Task 1: Add AI routing models and database migration

**Files:**
- Modify: `backend-go/internal/domain/models/ai_models.go`
- Modify: `backend-go/internal/platform/database/db.go`
- Test: `backend-go/internal/platform/database/db_test.go`

**Step 1: Write the failing test**

Add a database test that boots SQLite and asserts the new tables are auto-migrated:

```go
func TestInitDBAutoMigratesAIRouterTables(t *testing.T) {
    // assert ai_providers, ai_routes, ai_route_providers exist
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/database -run TestInitDBAutoMigratesAIRouterTables -v`
Expected: FAIL because models/tables do not exist.

**Step 3: Write minimal implementation**

- Add `AIProvider`, `AIRoute`, `AIRouteProvider`, optional `AICallLog` structs.
- Register them in database auto-migration.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/database -run TestInitDBAutoMigratesAIRouterTables -v`
Expected: PASS.

**Step 5: Verify formatting**

Run: `go test ./internal/platform/database -v`
Expected: PASS.

### Task 2: Create AI provider and route store layer

**Files:**
- Create: `backend-go/internal/platform/airouter/store.go`
- Create: `backend-go/internal/platform/airouter/store_test.go`
- Modify: `backend-go/internal/platform/aisettings/config_store.go`

**Step 1: Write the failing tests**

Add tests for:

- loading enabled route by capability
- returning providers ordered by priority
- rejecting missing enabled routes

```go
func TestStoreLoadRouteWithProvidersOrdersByPriority(t *testing.T) {}
func TestStoreLoadRouteWithProvidersReturnsErrorWhenMissing(t *testing.T) {}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/airouter -run TestStore -v`
Expected: FAIL because store package is missing.

**Step 3: Write minimal implementation**

- Add store methods to query routes/providers.
- Keep `aisettings` only for legacy config migration helpers.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/airouter -run TestStore -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/platform/airouter -v`
Expected: PASS.

### Task 3: Build the openai-compatible provider adapter

**Files:**
- Create: `backend-go/internal/platform/airouter/openai_compatible.go`
- Create: `backend-go/internal/platform/airouter/openai_compatible_test.go`
- Reference: `backend-go/internal/platform/ai/service.go`

**Step 1: Write the failing tests**

Cover:

- builds request correctly
- parses valid response
- returns typed retryable error on 429/5xx
- returns terminal config error on 401/invalid request

```go
func TestOpenAICompatibleClientChatSuccess(t *testing.T) {}
func TestOpenAICompatibleClientChatMarksRetryableErrors(t *testing.T) {}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/airouter -run TestOpenAICompatible -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Extract request/response code from duplicated call sites into adapter.
- Define typed errors for fallback decisions.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/airouter -run TestOpenAICompatible -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/platform/airouter -v`
Expected: PASS.

### Task 4: Build the AI router with ordered failover

**Files:**
- Create: `backend-go/internal/platform/airouter/router.go`
- Create: `backend-go/internal/platform/airouter/router_test.go`
- Modify: `backend-go/internal/platform/airouter/store.go`

**Step 1: Write the failing tests**

Cover:

- uses first provider on success
- falls back to second provider on retryable error
- stops on terminal config error
- returns aggregate error if all providers fail

```go
func TestRouterUsesPrimaryProviderWhenSuccessful(t *testing.T) {}
func TestRouterFallsBackOnRetryableProviderError(t *testing.T) {}
func TestRouterStopsOnTerminalError(t *testing.T) {}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/airouter -run TestRouter -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add `Capability`, `ChatRequest`, `ChatResult`, router service.
- Implement `ordered_failover`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/airouter -run TestRouter -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/platform/airouter -v`
Expected: PASS.

### Task 5: Add migration from legacy `summary_config`

**Files:**
- Create: `backend-go/internal/platform/airouter/migration.go`
- Create: `backend-go/internal/platform/airouter/migration_test.go`
- Modify: `backend-go/cmd/server/main.go`
- Modify: `backend-go/internal/platform/aisettings/config_store.go`

**Step 1: Write the failing tests**

Cover:

- imports a legacy summary config into one default provider and default routes
- does not duplicate providers when rerun
- extracts Firecrawl legacy section separately

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/airouter -run TestMigrate -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Parse legacy `summary_config`.
- Create `default-primary` provider.
- Create default routes for `summary`, `article_completion`, `topic_tagging`, `digest_polish`.
- Keep migration idempotent.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/airouter -run TestMigrate -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/platform/airouter -v`
Expected: PASS.

### Task 6: Switch summary generation flows to capability routing

**Files:**
- Modify: `backend-go/internal/domain/summaries/ai_handler.go`
- Modify: `backend-go/internal/domain/summaries/handler.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue.go`
- Modify: `backend-go/internal/jobs/auto_summary.go`
- Test: `backend-go/internal/domain/summaries/summary_queue_test.go`
- Test: `backend-go/internal/jobs/auto_summary_test.go`

**Step 1: Write the failing tests**

Cover:

- queue summary no longer needs explicit provider credentials
- auto summary reads route config rather than `summary_config`
- fallback provider can rescue a failed summary request

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/summaries ./internal/jobs -run "Test.*(Queue|AutoSummary)" -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Inject router into summary services.
- Remove raw HTTP calls from summary modules.
- Use capability `summary`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/summaries ./internal/jobs -run "Test.*(Queue|AutoSummary)" -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/domain/summaries ./internal/jobs -v`
Expected: PASS.

### Task 7: Switch content completion to capability routing

**Files:**
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_handler.go`
- Modify: `backend-go/internal/domain/contentprocessing/service` 
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_config.go`
- Test: `backend-go/internal/jobs/content_completion_test.go`

**Step 1: Write the failing tests**

Cover:

- content completion no longer loads AI credentials from `summary_config`
- service requests `article_completion` capability

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/contentprocessing ./internal/jobs -run Test.*Completion -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Replace `loadCompletionAISettings` dependency with router-backed config.
- Keep Firecrawl independent.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/contentprocessing ./internal/jobs -run Test.*Completion -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/domain/contentprocessing ./internal/jobs -v`
Expected: PASS.

### Task 8: Switch topic tagging to capability routing with heuristic fallback

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/tagger.go`
- Modify: `backend-go/internal/domain/topicgraph/tagger_test.go`

**Step 1: Write the failing tests**

Cover:

- topic tagging uses `topic_tagging` capability
- when all provider attempts fail, heuristic extraction still runs

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/topicgraph -run TestTagger -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Inject router dependency.
- Preserve current heuristic fallback path.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/topicgraph -run TestTagger -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/domain/topicgraph -v`
Expected: PASS.

### Task 9: Add provider and route management APIs

**Files:**
- Create: `backend-go/internal/domain/aiadmin/handler.go`
- Create: `backend-go/internal/domain/aiadmin/handler_test.go`
- Modify: `backend-go/internal/app/router.go`

**Step 1: Write the failing tests**

Cover:

- create/list/update provider
- update route provider order
- test route health

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/aiadmin -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add CRUD for providers.
- Add capability route update API.
- Add health/test endpoint.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/aiadmin -v`
Expected: PASS.

**Step 5: Verify router wiring**

Run: `go test ./internal/app -v`
Expected: PASS.

### Task 10: Refactor frontend settings to provider + route management

**Files:**
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue`
- Create: `front/app/api/aiAdmin.ts`
- Modify: `front/app/types/ai.ts`
- Modify: `front/app/composables/useAI.ts`
- Modify: `front/app/stores/api.ts`

**Step 1: Write the failing frontend tests or type assertions**

Add component/composable tests for:

- loading provider list from API
- saving route provider order
- removing dependency on localStorage as source of truth

**Step 2: Run test/typecheck to verify it fails**

Run: `pnpm exec nuxi typecheck`
Expected: FAIL because types and component bindings still use the old model.

**Step 3: Write minimal implementation**

- Replace single-model settings form with provider cards + route editor.
- Keep sensitive values server-backed.
- Remove business requests that push `base_url/api_key/model`.

**Step 4: Run test/typecheck to verify it passes**

Run: `pnpm exec nuxi typecheck`
Expected: PASS.

**Step 5: Run focused frontend verification**

Run: `pnpm test:unit -- app`
Expected: PASS for affected frontend tests.

### Task 11: Remove explicit AI config fields from business APIs

**Files:**
- Modify: `front/app/api/summaries.ts`
- Modify: `front/app/features/summaries/components/AISummariesListView.vue`
- Modify: `front/app/types/ai.ts`
- Modify: `backend-go/internal/domain/summaries/handler.go`
- Modify: `backend-go/internal/domain/summaries/ai_handler.go`

**Step 1: Write the failing tests**

Cover:

- summary requests only send business inputs
- backend rejects no longer depend on missing `api_key`

**Step 2: Run test/typecheck to verify it fails**

Run: `go test ./internal/domain/summaries -v && pnpm exec nuxi typecheck`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Remove `base_url/api_key/model` from request payloads and request structs.
- Resolve providers only in backend.

**Step 4: Run test/typecheck to verify it passes**

Run: `go test ./internal/domain/summaries -v && pnpm exec nuxi typecheck`
Expected: PASS.

**Step 5: Run broader verification**

Run: `go test ./internal/domain/summaries ./internal/jobs ./internal/domain/topicgraph -v`
Expected: PASS.

### Task 12: Split Firecrawl from AI config completely

**Files:**
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_config.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_handler.go`
- Modify: `backend-go/internal/platform/aisettings/config_store.go`
- Test: `backend-go/internal/domain/contentprocessing/firecrawl_handler_test.go`

**Step 1: Write the failing tests**

Cover:

- Firecrawl config persists independently from AI routes/providers
- updating Firecrawl no longer touches AI provider config

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/contentprocessing -run TestFirecrawl -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Create a dedicated Firecrawl config storage path.
- Remove reads/writes to `summary_config.firecrawl`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/contentprocessing -run TestFirecrawl -v`
Expected: PASS.

**Step 5: Run package tests**

Run: `go test ./internal/domain/contentprocessing -v`
Expected: PASS.

### Task 13: Add docs and regression verification

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/backend-go.md`
- Modify: `docs/operations/development.md`
- Reference: `docs/plans/2026-03-12-ai-router-design.md`

**Step 1: Update docs**

- document provider/route concepts
- document migration behavior
- document new APIs and verification flow

**Step 2: Run backend verification**

Run: `go test ./...`
Expected: PASS.

**Step 3: Run frontend verification**

Run: `pnpm exec nuxi typecheck && pnpm build`
Expected: PASS.

**Step 4: Run final scope verification**

Run: `git status --short`
Expected: only intended files changed.

**Step 5: Commit**

```bash
git add backend-go front README.md docs
git commit -m "refactor: add capability-based AI routing with provider failover"
```

Expected: commit created after all tests pass.
