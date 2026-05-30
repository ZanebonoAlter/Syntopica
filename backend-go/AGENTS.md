# Backend Agent Guide

遵循根 `AGENTS.md` 的所有规则。以下为后端特有差异。

## Backend-Specific Conventions
- Routes in `internal/app/router.go`. Business logic in `internal/domain/*`.
- PostgreSQL + pgvector for persistence. Use Docker: `pgvector/pgvector:pg18-trixie`.
- Handler response: `gin.H{"success": bool, "data"|"error"|"message": ...}`.
- JSON struct tags: `snake_case`. Wrap errors with `fmt.Errorf(... %w ...)`.
- Imports: stdlib → blank line → third-party → blank line → local.
- Naming: PascalCase exported, lowerCamelCase private. Short package names.
- Validate params before touching DB. Early returns for errors.

## Anti-Patterns
- No business logic in `router.go`. No direct DB access from handlers. No `panic` for errors.

## Commands
```bash
go mod tidy  &&  go run cmd/server/main.go
golangci-lint run ./...  &&  go vet ./...
go test ./...  &&  go build ./...
# Single: go test ./internal/domain/feed -run TestName -v
```
