# Repo Reorganization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize the repository so docs, frontend structure, backend structure, and top-level entrypoints all reflect the real project and are easier to read and extend.

**Architecture:** First repair the repository's information architecture so readers land on accurate entrypoints. Then migrate frontend files into feature-oriented directories and backend files into domain-oriented directories while preserving behavior, commands, routes, and API contracts.

**Tech Stack:** Markdown, Nuxt 4, Vue 3, TypeScript, Pinia, Go, Gin, GORM

---

### Task 1: Rebuild documentation entrypoints

**Files:**
- Create: `docs/README.md`
- Create: `docs/architecture/overview.md`
- Create: `docs/architecture/frontend.md`
- Create: `docs/architecture/backend-go.md`
- Create: `docs/architecture/data-flow.md`
- Create: `docs/guides/reading-preferences.md`
- Create: `docs/guides/content-processing.md`
- Create: `docs/guides/digest.md`
- Create: `docs/operations/development.md`
- Create: `docs/operations/database.md`
- Create: `docs/operations/encoding-safety.md`
- Modify: `README.md`
- Modify: `front/README.md`
- Modify: `backend-go/README.md`

**Step 1: Write the failing checks**

Search for invalid references and stale structure claims in root and service docs.

**Step 2: Run checks to verify they fail**

Run: `rg "crawl-service|docs/QUICKSTART.md|docs/CONTENT_COMPLETION.md|docs/READING_PREFERENCES.md|CLAUDE.md" README.md PROJECT_STRUCTURE.md backend-go/README.md front/README.md docs -g "*.md"`

Expected: matches found in current docs.

**Step 3: Write minimal implementation**

Create the new docs tree, move surviving content into the new hierarchy, and rewrite entrypoint docs so they only reference real files and real runtime components.

**Step 4: Run checks to verify they pass**

Run: `rg "crawl-service|docs/QUICKSTART.md|docs/CONTENT_COMPLETION.md|docs/READING_PREFERENCES.md|CLAUDE.md" README.md backend-go/README.md front/README.md docs -g "*.md"`

Expected: no matches in maintained docs.

### Task 2: Restructure frontend into features and shared layers

**Files:**
- Create: `front/app/api/*`
- Create: `front/app/features/**/*`
- Create: `front/app/shared/**/*`
- Modify: `front/app/app.vue`
- Modify: `front/app/pages/index.vue`
- Modify: `front/app/pages/digest/index.vue`
- Modify: `front/app/pages/digest/[id].vue`
- Modify: `front/app/stores/api.ts`
- Modify: `front/app/stores/feeds.ts`
- Modify: `front/app/stores/articles.ts`
- Modify: imports in affected Vue and TypeScript files

**Step 1: Write the failing check**

Capture the current state by identifying old imports and sync-based coupling.

**Step 2: Run check to verify it fails**

Run: `rg "syncToLocalStores|~/components|~/composables/api|app/services" front/app front/server -g "*.{ts,vue}"`

Expected: matches found in current frontend.

**Step 3: Write minimal implementation**

Move API modules to `app/api`, regroup components/composables by feature, move shared utilities into `shared`, and update imports. Keep route files thin. Remove or fold thin service wrappers where possible.

**Step 4: Run checks to verify it passes**

Run: `pnpm exec nuxi typecheck`

Expected: typecheck passes, or any pre-existing unrelated errors are isolated and reported.

### Task 3: Restructure backend-go into app/platform/domain/jobs

**Files:**
- Create: `backend-go/internal/app/*`
- Create: `backend-go/internal/platform/**/*`
- Create: `backend-go/internal/domain/**/*`
- Create: `backend-go/internal/jobs/*`
- Modify: `backend-go/cmd/server/main.go`
- Modify: imports in affected Go files

**Step 1: Write the failing check**

Capture current technical-layer coupling by searching for old package imports.

**Step 2: Run check to verify it fails**

Run: `rg "internal/(handlers|services|models|middleware|config|schedulers|ws)|pkg/database" backend-go -g "*.go"`

Expected: matches found across backend-go.

**Step 3: Write minimal implementation**

Extract server bootstrap into `internal/app`, move infrastructure packages into `platform`, regroup feature code into domain packages, and keep public behavior unchanged.

**Step 4: Run checks to verify it passes**

Run: `go test ./...`

Expected: tests pass, or remaining failures are documented with exact output.

### Task 4: Clean repository noise and stale top-level files

**Files:**
- Delete or archive: `PROJECT_STRUCTURE.md`
- Delete or archive: `READING_PREFERENCES_GUIDE.md`
- Delete or archive: `WORKFLOW.md`
- Modify: `.gitignore`

**Step 1: Write the failing check**

Identify generated artifacts and obsolete top-level docs still shaping the repo.

**Step 2: Run check to verify it fails**

Run: `rg "PROJECT_STRUCTURE.md|READING_PREFERENCES_GUIDE.md|WORKFLOW.md" README.md docs -g "*.md"`

Expected: references found before cleanup.

**Step 3: Write minimal implementation**

Replace stale top-level docs with maintained docs under `docs/`, update ignore rules for generated artifacts, and stop advertising deleted structure.

**Step 4: Run checks to verify it passes**

Run: `git status --short`

Expected: only intended source/documentation changes remain tracked.

### Task 5: Full verification

**Files:**
- Check: `README.md`
- Check: `docs/README.md`
- Check: `front/app/**/*`
- Check: `backend-go/**/*`

**Step 1: Run docs consistency checks**

Run: `rg "crawl-service|docs/QUICKSTART.md|docs/CONTENT_COMPLETION.md|docs/READING_PREFERENCES.md|CLAUDE.md" README.md backend-go/README.md front/README.md docs -g "*.md"`

**Step 2: Run frontend verification**

Run: `pnpm exec nuxi typecheck && pnpm build`

**Step 3: Run backend verification**

Run: `go test ./...`

**Step 4: Run final status check**

Run: `git status --short`

Expected: repository is reorganized, references are accurate, and verification results are captured before any completion claim.
