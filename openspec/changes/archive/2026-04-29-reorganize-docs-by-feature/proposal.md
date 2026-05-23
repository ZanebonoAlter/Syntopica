## Why

The `docs/` directory has grown organically without a clear organizational strategy. Files are scattered across generic categories (`api/`, `architecture/`, `guides/`) with no consistent structure. Finding documentation requires guessing which folder contains the relevant information, and related documents (e.g., a feature's design, implementation, and API docs) are often separated. This creates friction for developers navigating the codebase and increases the risk of documentation becoming stale or duplicated.

## What Changes

- **Restructure `docs/` by functional domain** instead of document type. Move from a flat/type-based hierarchy to a feature-based hierarchy where all documentation related to a specific capability lives together.
- **Create clear top-level domains** that map to the system's functional areas: feeds, articles, AI/summaries, topic graph, schedulers, operations, and development.
- **Consolidate scattered plans** into a single `plans/` directory with consistent naming.
- **Preserve all existing content** - no documentation will be deleted, only reorganized and cross-linked.
- **Add a new `docs/README.md`** at the root that serves as a navigation index for the reorganized structure.
- **Update `AGENTS.md`** references if any hardcoded doc paths are affected.

## Capabilities

### New Capabilities
<!-- This is a documentation reorganization with no new functional capabilities. No new specs required. -->
_(No new functional capabilities - this is a documentation infrastructure change.)_

### Modified Capabilities
<!-- No existing spec requirements are changing. -->
_(No existing capabilities modified.)_

## Impact

- **Docs**: Entire `docs/` directory structure will be reorganized.
- **Developer workflow**: Bookmarked doc URLs may break (mitigated by root README index).
- **No code changes**: This change is documentation-only; no application code, APIs, or behavior is modified.
