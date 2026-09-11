# lint-zero-debt Specification

## Purpose

门禁零债务契约：后端 golangci-lint 与前端 pnpm lint 保持 0 issues，禁止以"存量豁免"名义累积 lint 债。

## Requirements

### Requirement: Quality gate zero-debt

后端 `golangci-lint run ./...` 和前端 `pnpm lint` SHALL 输出 0 issues。

#### Scenario: Backend lint clean

- **WHEN** 在 `backend-go/` 下执行 `golangci-lint run ./...`
- **THEN** 命令退出码为 0，无任何输出

#### Scenario: Frontend lint clean

- **WHEN** 在 `front/` 下执行 `pnpm lint`
- **THEN** 命令退出码为 0，无任何 warning 或 error
