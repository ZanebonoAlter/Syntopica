# Backend Documentation Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite the backend documentation so `docs/` describes the real Go backend today, while clearly separating current architecture from the future directory reorganization.

**Architecture:** Treat `docs/` as the source of truth and stop relying on stale markdown under `backend-go/` for detailed architecture claims. Document the live startup flow, route surface, schedulers, data model drift, and the target `app/platform/domain/jobs` structure as a future migration direction rather than a current fact.

**Tech Stack:** Markdown, Go, Gin, GORM, SQLite

---

### Task 1: Audit stale backend docs

**Files:**
- Check: `backend-go/ARCHITECTURE.md`
- Check: `backend-go/DATABASE.md`
- Check: `backend-go/README.md`
- Check: `docs/architecture/backend-go.md`
- Check: `docs/operations/database.md`
- Check: `docs/operations/development.md`

**Step 1: Write the failing check**

List the stale claims that no longer match the codebase.

**Step 2: Run check to verify it fails**

Run: `rg "cmd/migrate|cmd/create-behavior-tables|/summaries/generate|/summaries/auto-generate|100% API 兼容|功能完全对等|Go 1.23\+|共8张" backend-go docs -g "*.md"`

Expected: matches found in current docs.

**Step 3: Write minimal implementation**

Replace or de-emphasize stale backend claims so maintained docs only describe real files, real routes, and real commands.

**Step 4: Run check to verify it passes**

Run: `rg "cmd/migrate|cmd/create-behavior-tables|/summaries/generate|/summaries/auto-generate|100% API 兼容|功能完全对等|Go 1.23\+|共8张" docs backend-go/README.md -g "*.md"`

Expected: no stale matches remain in maintained docs.

### Task 2: Rewrite backend architecture docs around current reality

**Files:**
- Modify: `docs/architecture/backend-go.md`
- Create: `docs/architecture/backend-runtime.md`

**Step 1: Write the failing check**

Capture missing documentation topics around runtime, route groups, WebSocket, schedulers, and future reorganization status.

**Step 2: Run check to verify it fails**

Run: `rg "runtime|WebSocket|content-completion|firecrawl|digest|queue|未来结构|目标结构" docs/architecture/backend-go.md -n`

Expected: missing or incomplete coverage in the current backend architecture doc.

**Step 3: Write minimal implementation**

Rewrite `docs/architecture/backend-go.md` to show current structure, current functional domains, current debt, and target structure. Add `docs/architecture/backend-runtime.md` for startup flow, route groups, schedulers, and runtime responsibilities.

**Step 4: Run check to verify it passes**

Run: `rg "runtime|WebSocket|content-completion|firecrawl|digest|queue|未来结构|目标结构" docs/architecture/backend-go.md docs/architecture/backend-runtime.md -n`

Expected: these topics are covered in maintained docs.

### Task 3: Refresh backend operation docs

**Files:**
- Modify: `docs/operations/development.md`
- Modify: `docs/operations/database.md`
- Modify: `backend-go/README.md`

**Step 1: Write the failing check**

Capture backend commands and schema notes that are currently under-documented.

**Step 2: Run check to verify it fails**

Run: `rg "migrate-digest|test-digest|ai_summary_queue|digest_configs|internal/app|/ws" docs/operations backend-go/README.md -g "*.md"`

Expected: incomplete or missing matches before the rewrite.

**Step 3: Write minimal implementation**

Update backend development and database docs to include current commands, runtime entrypoints, additional tables, and the fact that `docs/` is now the maintained source of truth. Reduce `backend-go/README.md` to a concise local entrypoint that points to `docs/`.

**Step 4: Run check to verify it passes**

Run: `rg "migrate-digest|test-digest|ai_summary_queue|digest_configs|internal/app|/ws" docs/operations backend-go/README.md -g "*.md"`

Expected: maintained docs now cover these current backend details.

### Task 4: Rebuild docs navigation

**Files:**
- Modify: `docs/README.md`

**Step 1: Write the failing check**

Identify missing links from the docs index to the refreshed backend docs.

**Step 2: Run check to verify it fails**

Run: `rg "backend-runtime|后端运行|后端启动|后端任务" docs/README.md -n`

Expected: no direct navigation exists yet.

**Step 3: Write minimal implementation**

Add backend architecture and runtime/doc navigation so readers know where to start depending on whether they care about structure, runtime, or operations.

**Step 4: Run check to verify it passes**

Run: `rg "backend-runtime|后端运行|后端启动|后端任务" docs/README.md -n`

Expected: the docs index links readers to the new backend docs.

### Task 5: Final verification

**Files:**
- Check: `docs/architecture/backend-go.md`
- Check: `docs/architecture/backend-runtime.md`
- Check: `docs/operations/development.md`
- Check: `docs/operations/database.md`
- Check: `docs/README.md`
- Check: `backend-go/README.md`

**Step 1: Run stale-claim search**

Run: `rg "cmd/migrate|cmd/create-behavior-tables|/summaries/generate|/summaries/auto-generate|100% API 兼容|功能完全对等|Go 1.23\+|共8张" docs backend-go/README.md -g "*.md"`

**Step 2: Run coverage search**

Run: `rg "internal/app|content-completion|firecrawl|digest|queue|/ws|migrate-digest|test-digest|ai_summary_queue|digest_configs" docs backend-go/README.md -g "*.md"`

**Step 3: Run final status check**

Run: `git status --short`

Expected: only intended backend documentation changes are staged or modified.
