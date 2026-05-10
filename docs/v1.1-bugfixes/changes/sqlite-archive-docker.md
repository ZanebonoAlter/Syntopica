# SQLite Archive Docker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a root `docker-compose.yml` plus frontend/backend Dockerfiles so this SQLite branch can boot from containers with the database file persisted in `./data`.

**Architecture:** Keep SQLite as the only database, mount `./data` into the backend container, and make both services build production artifacts up front. Use backend env overrides for runtime settings and Nuxt runtime config for browser/server API routing.

**Tech Stack:** Docker Compose, Go, Gin, GORM SQLite, Nuxt 4, Node.js, pnpm, Vitest

---

### Task 1: Lock runtime config behavior with tests

**Files:**
- Create: `backend-go/internal/platform/config/config_test.go`
- Create: `front/app/utils/api.test.ts`

**Step 1:** Write a backend test that sets `SERVER_PORT`, `DATABASE_DSN`, and `CORS_ORIGINS`, then asserts `LoadConfig` produces overridden values.

**Step 2:** Run `go test ./internal/platform/config -v` and confirm it fails because env override handling does not exist yet.

**Step 3:** Write a frontend test for `resolveApiBaseUrlFromConfig` and `resolveApiOriginFromConfig`, covering client/server resolution.

**Step 4:** Run `pnpm test:unit -- app/utils/api.test.ts` and confirm it fails because the helper does not exist yet.

### Task 2: Add backend env override support

**Files:**
- Modify: `backend-go/internal/platform/config/config.go`

**Step 1:** Add minimal env override logic for `SERVER_PORT`, `SERVER_MODE`, `DATABASE_DRIVER`, `DATABASE_DSN`, and `CORS_ORIGINS`.

**Step 2:** Re-run `go test ./internal/platform/config -v` and confirm it passes.

### Task 3: Add frontend runtime API helpers

**Files:**
- Create: `front/app/utils/api.ts`
- Modify: `front/app/api/client.ts`
- Modify: `front/app/api/scheduler.ts`
- Modify: `front/app/composables/useAI.ts`
- Modify: `front/app/features/summaries/composables/useSummaryWebSocket.ts`
- Modify: `front/nuxt.config.ts`

**Step 1:** Add a small helper that resolves public API origin, public API base, and server-side internal API base from runtime config.

**Step 2:** Replace hard-coded `localhost:5000` usage with the helper.

**Step 3:** Re-run `pnpm test:unit -- app/utils/api.test.ts` and confirm it passes.

### Task 4: Add container build files

**Files:**
- Create: `backend-go/Dockerfile`
- Create: `front/Dockerfile`

**Step 1:** Add a backend multi-stage Dockerfile that builds the Go server and runs it with `/app/configs` plus `/app/data`.

**Step 2:** Add a frontend multi-stage Dockerfile that builds Nuxt and serves `.output/server/index.mjs` on `0.0.0.0:3000`.

### Task 5: Add root compose and env defaults

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`
- Modify: `.gitignore`
- Create: `data/.gitkeep`

**Step 1:** Define `backend` and `front` services, bind-mount `./data`, and expose configurable host ports.

**Step 2:** Add `.env.example` with `FRONT_PORT`, `BACKEND_PORT`, and `SQLITE_DB_FILE`.

**Step 3:** Ignore runtime database files under `data/` while keeping the directory itself tracked.

### Task 6: Document and verify

**Files:**
- Modify: `README.md`
- Modify: `docs/operations/development.md`

**Step 1:** Add short archive startup instructions using `docker compose up --build`.

**Step 2:** Run:
- `go test ./internal/platform/config -v`
- `pnpm test:unit -- app/utils/api.test.ts`
- `docker compose config`

**Step 3:** Summarize any unverified areas if the local environment cannot complete container builds.
