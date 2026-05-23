## Context

The current `docs/` directory uses a type-based organization (api/, architecture/, guides/, operations/, etc.). As the project has grown to include feeds, articles, AI summaries, topic graphs, schedulers, and multiple processing pipelines, this structure forces related documentation to be scattered. For example, information about article content processing exists in both `guides/content-processing.md` and `api/content-completion.md`, with architecture notes in `architecture/data-flow.md`. A developer working on the article feature must hunt across three separate directories.

The codebase itself is organized by functional domain (`backend-go/internal/domain/feeds`, `internal/domain/summaries`, `internal/domain/topicgraph`, etc.), making the documentation structure misaligned with the code structure.

## Goals / Non-Goals

**Goals:**
- Reorganize all documentation under functional domains that mirror the codebase structure
- Co-locate API docs, guides, and architecture notes for each feature
- Add a root `README.md` that serves as a navigation index
- Ensure no content is lost during reorganization
- Update `AGENTS.md` if any hardcoded documentation paths exist

**Non-Goals:**
- Rewriting or updating outdated documentation content (reorganization only)
- Changing any application code, APIs, or behavior
- Introducing new documentation tools or formats
- Deleting any existing files (only moving/renaming)

## Decisions

### Decision: Organize by functional domain instead of document type
**Rationale**: The codebase is already organized by domain (`feeds/`, `articles/`, `summaries/`, `topicgraph/`, `schedulers/`). Aligning docs with code reduces cognitive overhead - a developer can navigate to the docs folder that matches the package they're working in. This follows the "principle of proximity" - related concepts should live together.

**Alternative considered**: Keep type-based structure but add cross-reference indices. Rejected because it doesn't solve the scattering problem, only masks it.

### Decision: Create a flat feature-directory structure at the top level
**Rationale**: A two-level hierarchy (feature/ + doc-type/) would recreate the old problem within each feature. Instead, each feature directory contains all its docs with descriptive filenames (e.g., `feeds/api.md`, `feeds/guides.md`, `feeds/architecture.md`).

### Decision: Keep `plans/` and `releases/` as top-level directories
**Rationale**: Plans and releases are cross-cutting concerns that span multiple features. They don't belong to a single functional domain. However, add a `plans/README.md` index for discoverability.

### Decision: Consolidate experience notes into `development/`
**Rationale**: `experience/ENCODING_SAFETY.md` and `experience/LESSONS_LEARNED.md` are developer-facing operational knowledge. Moving them to `development/` keeps all contributor-facing docs in one place.

## Risks / Trade-offs

- **[Risk] Broken bookmarks/mental models** → Existing team members have bookmarks and mental maps of the old structure. **Mitigation**: Root README.md provides a clear index with "moved to..." notes for common lookups.
- **[Risk] Git history fragmentation** → Moving files breaks `git blame` continuity for those files. **Mitigation**: Acceptable trade-off for a one-time reorganization; use `git log --follow` if needed.
- **[Risk] Partial reorganization** → If stopped midway, docs become even more scattered. **Mitigation**: Treat as a single atomic change; complete all moves in one commit.

## Migration Plan

1. Create new directory structure alongside existing directories (don't delete yet)
2. Move files to new locations with `git mv` to preserve history
3. Create new `docs/README.md` with navigation index
4. Create `plans/README.md` with chronological index
5. Update `AGENTS.md` references if any paths are hardcoded
6. Delete old empty directories
7. Single commit with clear message: "docs: reorganize documentation by functional domain"

## Open Questions

- Should we add redirects/placeholder files at old locations pointing to new locations? (Probably not necessary for internal docs)
- Should `architecture/overview.md` become `system/architecture.md` or stay as a top-level `architecture.md`?
