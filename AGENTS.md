# AGENTS.md

Agent guide for coding assistants working in `D:\project\my-robot`.

## Rule Sources
- Primary source of truth: this file, `README.md`, and docs under `docs/`.
- Subdirectory guides: `front/AGENTS.md`, `backend-go/AGENTS.md` for domain-specific conventions.
- Checked for Cursor rules: no `.cursorrules` and no `.cursor/rules/` directory found.
- Checked for Copilot rules: no `.github/copilot-instructions.md` found.
- If new rule files appear later, merge their guidance here before making broad changes.
- 完成任务后，参考`llm-wiki.md`的指导更新维护本地知识库 `./docs`
- 和用户沟通使用中文
- 用户开发环境 windows 

## Project Snapshot
- RSS Reader app with a Nuxt 4 frontend and a Go backend.
- Main features: feed subscriptions, article reading, AI summaries, Firecrawl enrichment, digest export, schedulers, topic graph analysis with vector search.
- Personal/single-user deployment only; there is no auth system.
- Frontend API: `http://localhost:5000/api`; WebSocket: `ws://localhost:5000/ws`.
- **Main branch**: Backend persistence uses PostgreSQL with pgvector extension for vector search.
- **SQLite support**: Only available in the `sqlite` archive branch, no longer supported in main.
- Optional Redis for persistent job queues (defaults to in-memory if not configured).
- Crawl service (default: `http://localhost:11235`) for content completion and full-text scraping.
- AI configuration (LLM, Firecrawl, Digest) is stored in the database and managed via web UI, no config files needed.

## Repo Layout
- `front/`: Nuxt 4, Vue 3, TypeScript, Pinia, Tailwind CSS v4.
- `backend-go/`: Gin, GORM, SQLite, schedulers, digest jobs.
- `docs/`: architecture and workflow docs.
- `tests/workflow/`: Python integration tests for scheduler and workflow behavior.
- `tests/firecrawl/`: Python integration check for Firecrawl flow.

## First Files To Read
- `README.md` for product scope.
- `front/app/app.vue` for frontend entry.
- `front/app/api/client.ts` for HTTP conventions.
- `front/app/stores/api.ts` for data mapping and main store usage.
- `backend-go/cmd/server/main.go` for backend entry.
- `backend-go/internal/app/router.go` for route layout.
- `backend-go/internal/app/runtime.go` for scheduler/runtime wiring.

## Build, Test, And Verify

### Frontend
Run from `front/`:
```bash
pnpm install
pnpm dev
pnpm build
pnpm generate
pnpm preview
pnpm exec nuxi typecheck
pnpm test:unit
pnpm test:e2e
```
- Single test file: `pnpm test:unit -- app/utils/articleContentSource.test.ts`
- Single test by name: `pnpm test:unit -- app/utils/articleContentSource.test.ts -t "prefers firecrawl"`
- E2E tests (Playwright): `pnpm test:e2e` (runs all E2E tests), `pnpm test:e2e:ui` for interactive UI mode
- No dedicated lint/formatting script is configured in `front/package.json`. Match existing code style (no semicolons, UTF-8 encoding).
- Main quality gates: `pnpm exec nuxi typecheck` and `pnpm build`.

### Backend Go
Run from `backend-go/`:
```bash
go mod tidy
go run cmd/server/main.go
go build ./...
go test ./...
go run cmd/migrate-digest/main.go
go run cmd/test-digest/main.go
go run cmd/migrate-tags/main.go
go run cmd/migrate-db/main.go
```
- Single package: `go test ./internal/domain/feeds -v`
- Single test: `go test ./internal/domain/feeds -run TestBuildArticleFromEntryTracksOnlyRunnableStates -v`
- Prefer targeted package tests first, then `go test ./...` for broader coverage.
- Local development requires PostgreSQL with pgvector extension, run via Docker:
  ```bash
  docker run -d --name rss-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=rss_reader pgvector/pgvector:pg18-trixie
  ```

### Python Integration Tests
Run from `tests/workflow/`:
```bash
uv venv
.venv\Scripts\activate
uv pip install -r requirements.txt
pytest test_*.py -v
```
- Single file: `pytest test_schedulers.py -v`
- Single test: `pytest test_schedulers.py::TestAutoRefreshScheduler::test_name -v`
- Coverage: `pytest --cov=. --cov-report=html`
- These tests expect the Go backend on `localhost:5000`.

### Firecrawl Check
- Start backend from `backend-go/` with `go run cmd/server/main.go`.
- Then run `python test_firecrawl_integration.py` from `tests/firecrawl/`.

## Frontend Conventions
- Use Vue 3 Composition API with `<script setup lang="ts">` for new Vue files.
- Use TypeScript across frontend code.
- Keep route pages thin in `front/app/pages/` (only for component mounting, no business logic).
- Move business logic into `front/app/features/` or composables.
- Put network calls in `front/app/api/`, not directly in components.
- `useApiStore` is the primary data source; other stores should be derived UI state.
- Keep shared types in `front/app/types/`.
- Convert backend numeric IDs to frontend strings at the API/store boundary.
- Keep `snake_case → camelCase` mapping in API/store code, never in templates.
- Reuse `ApiResponse<T>` for request results.

### Frontend Imports, Formatting, Naming
- Preferred import order: Vue/Nuxt, third-party, internal modules, then type-only imports.
- Use `import type` for type-only dependencies.
- Use `~` alias imports for app-root paths.
- Follow existing file-local formatting; do not reformat unrelated lines.
- Most frontend files omit semicolons; preserve surrounding style.
- Frontend files must remain UTF-8; never rewrite them as ANSI, GBK, or UTF-16.
- Components: PascalCase, e.g. `ArticleContentView.vue`.
- Composables and stores: camelCase with `use` prefix, e.g. `useSummaryWebSocket`.
- Utility files: descriptive camelCase names.
- Props interfaces are commonly named `Props`; emits should be typed with `defineEmits<...>()`.

### Frontend Error Handling
- Wrap HTTP access behind `ApiClient`.
- Return `{ success, data, error, message }` shaped results instead of throwing into the UI.
- In components, show friendly messages and keep `console.error` for debugging context.
- Prefer defensive null checks around API data.

## Backend Go Conventions
- Keep HTTP routes in `internal/app/router.go`; keep business logic in `internal/domain/*`.
- Use focused domain packages such as `feeds`, `digest`, `summaries`, `contentprocessing`, `topicanalysis`, `topicgraph`, and `preferences`.
- Use PascalCase for exported symbols and lowerCamelCase for private helpers.
- Keep JSON fields snake_case via struct tags.
- Use `fmt.Errorf(... %w ...)` when wrapping lower-level errors.
- Prefer early returns for validation failures and DB errors.
- Handlers should return `gin.H{"success": bool, "data"|"error"|"message": ...}`.
- Validate params and request bodies before touching the database.
- Keep GORM models in `internal/domain/models` and shared infrastructure in `internal/platform/*`.
- Business logic belongs in domain packages, not in HTTP handlers or job processors.

### Backend Imports, Formatting, Tests
- Let `gofmt` format Go files.
- Group imports as stdlib, blank line, third-party, blank line, local packages.
- Keep package names short; alias only when collision or readability requires it.
- Use `testing` directly; `testify` is acceptable if a file already uses it.
- Prefer table tests when many cases share behavior.
- Keep tests close to code as `*_test.go` files.
- 使用"my-robot-backend/internal/platform/logging"日志分流

## UI And Content Direction
- Preserve the repo's editorial / magazine feel.
- Avoid generic SaaS layouts, especially centered hero-plus-cards pages.
- Do not introduce purple/indigo default SaaS palettes.
- Prefer textured, layered, or gradient backgrounds over flat fills.
- Use Iconify for icons.
- Keep copy short, concrete, and conversational; short Chinese UI text is common here.

## Docs And Architecture Notes
- Update docs when APIs, runtime flows, or major UI structure change.
- Relevant docs usually include `docs/architecture/frontend.md`, `docs/architecture/backend-go.md`, and `docs/operations/development.md`.
- If scheduler flow, digest flow, or data mapping changes, document it.

## GitNexus Workflow
- Repo is indexed in GitNexus as `my-robot`.
- Before editing a function, method, or class, run `gitnexus_impact` on that symbol.
- If impact risk is HIGH or CRITICAL, warn the user before proceeding.
- Use `gitnexus_query` to understand unfamiliar execution flows.
- Use `gitnexus_context` when you need callers, callees, and process participation.
- Before committing, run `gitnexus_detect_changes()` and confirm the affected scope matches intent.

## Agent Expectations
- Do not assume there is a Python backend; the product backend is Go.
- Do not add new linters, formatters, or tooling unless the user asks.
- Ignore unrelated dirty-worktree changes.
- Verify the smallest relevant command after edits, then broaden if needed.
- Frontend-only edits: prefer `pnpm exec nuxi typecheck`, `pnpm test:unit`, or `pnpm build`.
- Backend-only edits: prefer targeted `go test` first, then `go test ./...` or `go build ./...`.
- Docs-only edits usually only need consistency checks unless the docs describe changed behavior.
- Recommended pre-push verification sequence:
  ```bash
  cd backend-go && go test ./... && go build ./...
  cd front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build
  ```

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **my-robot** (12689 symbols, 22191 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/my-robot/context` | Codebase overview, check index freshness |
| `gitnexus://repo/my-robot/clusters` | All functional areas |
| `gitnexus://repo/my-robot/processes` | All execution flows |
| `gitnexus://repo/my-robot/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

## Browser Automation

Use `agent-browser` for web automation. Run `agent-browser --help` for all commands.

Core workflow:

1. `agent-browser open <url>` - Navigate to page
2. `agent-browser snapshot -i` - Get interactive elements with refs (@e1, @e2)
3. `agent-browser click @e1` / `fill @e2 "text"` - Interact using refs
4. Re-snapshot after page changes

# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

<!-- code-review-graph MCP tools -->
## MCP Tools: code-review-graph

**IMPORTANT: This project has a knowledge graph. ALWAYS use the
code-review-graph MCP tools BEFORE using Grep/Glob/Read to explore
the codebase.** The graph is faster, cheaper (fewer tokens), and gives
you structural context (callers, dependents, test coverage) that file
scanning cannot.

### When to use graph tools FIRST

- **Exploring code**: `semantic_search_nodes` or `query_graph` instead of Grep
- **Understanding impact**: `get_impact_radius` instead of manually tracing imports
- **Code review**: `detect_changes` + `get_review_context` instead of reading entire files
- **Finding relationships**: `query_graph` with callers_of/callees_of/imports_of/tests_for
- **Architecture questions**: `get_architecture_overview` + `list_communities`

Fall back to Grep/Glob/Read **only** when the graph doesn't cover what you need.

### Key Tools

| Tool | Use when |
|------|----------|
| `detect_changes` | Reviewing code changes — gives risk-scored analysis |
| `get_review_context` | Need source snippets for review — token-efficient |
| `get_impact_radius` | Understanding blast radius of a change |
| `get_affected_flows` | Finding which execution paths are impacted |
| `query_graph` | Tracing callers, callees, imports, tests, dependencies |
| `semantic_search_nodes` | Finding functions/classes by name or keyword |
| `get_architecture_overview` | Understanding high-level codebase structure |
| `refactor_tool` | Planning renames, finding dead code |

### Workflow

1. The graph auto-updates on file changes (via hooks).
2. Use `detect_changes` for code review.
3. Use `get_affected_flows` to understand impact.
4. Use `query_graph` pattern="tests_for" to check coverage.

## Agent skills

### Issue tracker

Issues and PRDs live as markdown files in `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context repo — `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.
