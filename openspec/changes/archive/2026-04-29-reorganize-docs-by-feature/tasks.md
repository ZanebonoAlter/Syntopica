## 1. Preparation

- [x] 1.1 Create new directory structure under `docs/`:
  - `docs/feeds/`
  - `docs/articles/`
  - `docs/ai-features/`
  - `docs/topic-graph/`
  - `docs/schedulers/`
  - `docs/system/`
  - `docs/development/`
  - `docs/plans/` (already exists, ensure it stays)
- [x] 1.2 Audit all current files in `docs/` and map each to its target location
- [x] 1.3 Check `AGENTS.md` and other repo files for hardcoded `docs/` paths that need updating

## 2. Core Migration - API Documentation

- [x] 2.1 Move `docs/api/feeds.md` → `docs/feeds/api.md`
- [x] 2.2 Move `docs/api/articles.md`, `docs/api/reading.md`, `docs/api/content-completion.md` → `docs/articles/api.md` (merge or separate as needed)
- [x] 2.3 Move `docs/api/summaries.md`, `docs/api/ai-admin.md`, `docs/api/firecrawl.md` → `docs/ai-features/api.md`
- [x] 2.4 Move `docs/api/topic-graph.md` → `docs/topic-graph/api.md`
- [x] 2.5 Move `docs/api/schedulers.md`, `docs/api/traces.md`, `docs/api/system.md` → `docs/schedulers/api.md`
- [x] 2.6 Move `docs/api/opml.md`, `docs/api/categories.md` → determine appropriate feature directory
- [x] 2.7 Move `docs/api/_conventions.md`, `docs/api/_index.md` → `docs/system/api-conventions.md`

## 3. Core Migration - Guides & Architecture

- [x] 3.1 Move `docs/guides/getting-started.md` → `docs/getting-started.md` (top level)
- [x] 3.2 Move `docs/guides/content-processing.md`, `docs/guides/reading-preferences.md` → `docs/articles/`
- [x] 3.3 Move `docs/guides/tagging-flow.md`, `docs/guides/topic-graph.md` → split to `docs/ai-features/` and `docs/topic-graph/`
- [x] 3.4 Move `docs/guides/frontend-features.md` → `docs/development/frontend-features.md`
- [x] 3.5 Move `docs/guides/configuration.md`, `docs/guides/deployment.md`, `docs/guides/testing.md` → `docs/system/` or `docs/development/`
- [x] 3.6 Move `docs/architecture/frontend.md`, `docs/architecture/frontend-components.md` → `docs/development/frontend-architecture.md`
- [x] 3.7 Move `docs/architecture/backend-go.md`, `docs/architecture/backend-runtime.md`, `docs/architecture/data-flow.md`, `docs/architecture/overview.md` → `docs/system/architecture/`
- [x] 3.8 Move `docs/architecture/tracing.md` → `docs/system/tracing.md`
- [x] 3.9 Move `docs/architecture/tag-cleanup-redesign.md` → `docs/topic-graph/tag-cleanup-design.md`

## 4. Core Migration - Operations & Experience

- [x] 4.1 Move `docs/operations/database.md`, `docs/operations/postgres-migration.md` → `docs/system/database-operations.md`
- [x] 4.2 Move `docs/operations/development.md` → `docs/development/operations.md`
- [x] 4.3 Move `docs/operations/troubleshooting.md` → `docs/system/troubleshooting.md`
- [x] 4.4 Move `docs/experience/ENCODING_SAFETY.md` → `docs/development/encoding-safety.md`
- [x] 4.5 Move `docs/experience/LESSONS_LEARNED.md` → `docs/development/lessons-learned.md`
- [x] 4.6 Move `docs/database/DATABASE_FIELDS.md` → `docs/system/database-schema.md`

## 5. Index & Cross-Reference Creation

- [x] 5.1 Create `docs/README.md` with:
  - Overview of new organization
  - Directory listing with descriptions
  - "Moved from..." mapping table for common lookups
- [x] 5.2 Create `docs/plans/README.md` with chronological index of all plan documents
- [x] 5.3 Add per-feature index files if a feature has 3+ documents (optional)

## 6. Cleanup & Verification

- [x] 6.1 Delete old empty directories (`docs/api/`, `docs/architecture/`, `docs/database/`, `docs/experience/`, `docs/guides/`, `docs/operations/`)
- [x] 6.2 Update any hardcoded paths in `AGENTS.md`, `README.md`, or other repo files
- [x] 6.3 Verify no broken internal links within moved documentation (spot check)
- [x] 6.4 Run `git status` to confirm all moves are tracked as renames
- [ ] 6.5 Commit with message: `docs: reorganize documentation by functional domain`
