# AGENTS.md

Agent guide for coding assistants working in `D:\project\my-robot`.

## Project Snapshot
- RSS Reader: Nuxt 4 frontend + Go backend (Gin/GORM), single-user, no auth.
- Frontend API: `http://localhost:5000/api`; WebSocket: `ws://localhost:5000/ws`.
- PostgreSQL + pgvector for persistence; Redis optional for job queues.
- Crawl service: `http://localhost:11235`. AI config managed via web UI, no config files.
- 和用户沟通使用中文，开发环境 Windows。

## Reference Docs (authoritative source)
- **Architecture**: `docs/reference/architecture/`
- **API**: `docs/reference/api/`
- **Database**: `docs/reference/database/`
- **Development**: `docs/reference/development.md`
- **Configuration**: `docs/reference/configuration.md`
- **Deployment**: `docs/reference/deployment.md`
- **Testing**: `docs/reference/testing.md`
- Subdirectory guides: `front/AGENTS.md`, `backend-go/AGENTS.md`.

## Repo Layout
- `front/`: Nuxt 4, Vue 3, TypeScript, Pinia, Tailwind CSS v4.
- `backend-go/`: Gin, GORM, PostgreSQL + pgvector.
- `docs/`: reference/ (活文档) + v1.x/ (里程碑) + experience/.
- `tests/workflow/`, `tests/firecrawl/`: Python integration tests.

## Key Entry Points
- `README.md`, `front/app/app.vue`, `front/app/api/client.ts`, `front/app/stores/api.ts`
- `backend-go/cmd/server/main.go`, `backend-go/internal/app/router.go`, `backend-go/internal/app/runtime.go`

## Build & Verify

**Frontend** (`front/`): `pnpm install` / `pnpm dev` / `pnpm build` / `pnpm lint` / `pnpm exec nuxi typecheck` / `pnpm test:unit` / `pnpm test:e2e`

**Backend** (`backend-go/`): `go mod tidy` / `go run cmd/server/main.go` / `golangci-lint run ./...` / `go vet ./...` / `go test ./...` / `go build ./...`

**Pre-push check**: `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...` && `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`

## AI Behavior Rules
- Do not add linters, formatters, or tooling unless asked.
- Do not assume Python backend; the product backend is Go.
- Ignore unrelated dirty-worktree changes. Verify smallest relevant command after edits.
- Frontend edits → `pnpm lint` / `pnpm exec nuxi typecheck` / `pnpm test:unit` / `pnpm build`.
- Backend edits → `golangci-lint run ./...` / targeted `go test` first, then `go test ./...` / `go build ./...`.
- Docs-only edits: consistency check unless behavior changed.
- Keep code changes minimal and scoped. Match existing code style.
- 完成任务后更新维护 `./docs` 知识库。

## GitNexus Workflow
- Repo indexed as `my-robot`. Before editing any function/method/class, run `gitnexus_impact`.
- Warn user if HIGH or CRITICAL risk. Use `gitnexus_query` for unfamiliar flows.
- Before committing, run `gitnexus_detect_changes()`. Never skip impact analysis.
- See `.claude/skills/gitnexus/` for detailed workflow docs.

## Browser Automation
Use `agent-browser`: `open <url>` → `snapshot -i` → `click @eX` / `fill @eX "text"` → re-snapshot.
