# Topic Graph Homepage Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a dedicated topic graph page that turns digest-window AI summaries into an interactive 3D topic network with clickable history and related summary details.

**Architecture:** Add a lightweight backend `topicgraph` domain that reuses digest-style daily/weekly time windows, extracts topic labels from existing AI summaries, and returns graph-ready nodes, edges, and topic detail payloads. Add a dedicated frontend `/topics` page with a 3D force graph canvas, editorial side panels, and a sidebar entry so the feature feels like a first-class surface instead of a hidden digest add-on.

**Tech Stack:** Go 1.21, Gin, GORM, SQLite, Nuxt 4, Vue 3, TypeScript, Vitest, Three.js, 3d-force-graph

---

### Task 1: Define backend topic graph contract and extractor tests

**Files:**
- Create: `backend-go/internal/domain/topicgraph/types.go`
- Create: `backend-go/internal/domain/topicgraph/extractor.go`
- Create: `backend-go/internal/domain/topicgraph/extractor_test.go`

**Step 1: Write the failing test**

Add extractor tests for:

- AI topic dictionary tags like `AI Agent` and `OpenAI`
- entity-like uppercase/product tokens like `GPT-5`
- dedupe and canonical label normalization
- fallback when summary text is sparse

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/topicgraph -run Extract -v`

Expected: FAIL because package does not exist.

**Step 3: Write minimal implementation**

Implement a hybrid extractor that combines:

- curated AI topic dictionary
- regex extraction for entity/product names
- canonical label + slug normalization

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/topicgraph -run Extract -v`

Expected: PASS

---

### Task 2: Add graph builder and API handler tests

**Files:**
- Create: `backend-go/internal/domain/topicgraph/service.go`
- Create: `backend-go/internal/domain/topicgraph/handler.go`
- Create: `backend-go/internal/domain/topicgraph/handler_test.go`
- Modify: `backend-go/internal/app/router.go`

**Step 1: Write the failing test**

Add handler tests for:

- `GET /api/topic-graph/daily?date=2026-03-11`
- `GET /api/topic-graph/topic/ai-agent?type=daily&date=2026-03-11`
- correct graph node/edge counts and topic detail history ordering

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/topicgraph -run Graph -v`

Expected: FAIL because handlers and routes do not exist.

**Step 3: Write minimal implementation**

Implement:

- daily/weekly summary window query
- graph node/edge aggregation
- topic detail history from recent summaries
- Gin handlers that return `{ success, data }`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/topicgraph -run Graph -v`

Expected: PASS

---

### Task 3: Add frontend API layer and graph view-model tests

**Files:**
- Create: `front/app/api/topicGraph.ts`
- Create: `front/app/features/topic-graph/utils/buildTopicGraphViewModel.ts`
- Create: `front/app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts`
- Modify: `front/app/api/index.ts`

**Step 1: Write the failing test**

Test the view-model builder for:

- stable node sizing from heat/confidence
- edge filtering for weak relations
- spotlight topic card defaults

**Step 2: Run test to verify it fails**

Run: `pnpm test:unit -- app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts`

Expected: FAIL because files do not exist.

**Step 3: Write minimal implementation**

Add typed API client methods and a pure view-model helper that prepares API data for the 3D scene and side panels.

**Step 4: Run test to verify it passes**

Run: `pnpm test:unit -- app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts`

Expected: PASS

---

### Task 4: Build the dedicated topic graph page

**Files:**
- Create: `front/app/pages/topics.vue`
- Create: `front/app/features/topic-graph/components/TopicGraphPage.vue`
- Create: `front/app/features/topic-graph/components/TopicGraphCanvas.client.vue`
- Create: `front/app/features/topic-graph/components/TopicGraphSidebar.vue`
- Create: `front/app/features/topic-graph/components/TopicGraphHeader.vue`
- Modify: `front/app/features/shell/components/AppSidebarView.vue`
- Modify: `front/app/features/shell/components/FeedLayoutShell.vue`
- Modify: `front/package.json`

**Step 1: Write the failing test**

Reuse the Task 3 view-model test as the guardrail before UI code.

**Step 2: Run test to verify it fails for missing UI dependencies if needed**

Run: `pnpm test:unit -- app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts`

Expected: PASS before UI work begins.

**Step 3: Write minimal implementation**

Build a magazine-like graph page with:

- date + daily/weekly controls
- 3D topic graph canvas
- click-to-load topic details
- summary/history/search action side panel
- sidebar entry `主题图谱`

**Step 4: Verify app integration**

Run: `pnpm exec nuxi typecheck`

Expected: PASS

---

### Task 5: Verify end-to-end behavior and update progress log

**Files:**
- Modify: `docs/plans/2026-03-11-topic-graph-homepage-progress.md`
- Optional: `docs/architecture/frontend.md`
- Optional: `docs/architecture/backend-go.md`

**Step 1: Run targeted backend tests**

Run: `go test ./internal/domain/topicgraph -v`

**Step 2: Run targeted frontend test and typecheck**

Run: `pnpm test:unit -- app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts`

Run: `pnpm exec nuxi typecheck`

**Step 3: Update local progress record**

Record completed work, remaining gaps, and next implementation slice.
