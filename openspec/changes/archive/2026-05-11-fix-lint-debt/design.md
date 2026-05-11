## Context

代码库长期累积静态分析债务：后端 golangci-lint 161 个告警（unused 49 / staticcheck 36 / errcheck 31 / gosec 17 / gocritic 18 / ineffassign 5 / gofmt 5），前端 eslint 33 个 warning。全部是现有工具链已配置的检查项，但因历史原因从未统一清零。

当前 `.golangci.yml` 已启用 staticcheck/revive/gosec/errcheck/gocritic/unused/govet/ineffassign + gofmt formatter。前端 `eslint.config.js` 已配置 typescript-eslint recommended。

## Goals / Non-Goals

**Goals:**

- `golangci-lint run ./...` 输出 0 issues
- `pnpm lint` 输出 0 problems
- 所有改动为机械性修复，不引入新逻辑、不改变行为
- 按 phase 分批执行，每个 phase 完成后验证门禁

**Non-Goals:**

- 不收紧 lint 规则（如将 `no-explicit-any` 从 warn 升为 error）
- 不新增 linter（如 prettier、golangci-lint 新增 linter）
- 不处理 `go vet` 和 `go test` 现有问题（另行处理）
- 不重构代码架构或改进设计

## Decisions

### D1: 执行顺序 — 先机械后人工

Phase 1 gofmt → Phase 2 unused → Phase 3 gosec/errcheck/staticcheck/gocritic/ineffassign → Phase 4 前端。

理由：gofmt 和 unused 删除是零判断操作，能快速减少 ~100 条告警，让后续人工判断的信号更清晰。

### D2: unused 处理 — 全删

49 个 unused 项全部删除，不加 `//nolint` 保留。包括 `extractor_enhanced.go` 中 12 个未接入的 AI 消歧函数（`aiJudgment`、`buildResolutionSystemPrompt` 等）。这些代码可通过 git 历史恢复。

替代方案：加 `//nolint:unused` 保留。否决理由：死代码增加维护负担，lint 输出失去信号。

### D3: gosec 按类别处理

| 规则 | # | 处理 |
|------|---|------|
| G118 (goroutine context) | 4 | 传入 request context 替代 `context.Background()` |
| G115 (整数溢出) | 4 | 添加范围检查或使用 `int64` |
| G304 (文件路径变量) | 3 | 仅测试文件，加 `//nolint:gosec` 注释 |
| G306 (文件权限) | 2 | 仅测试文件，改为 `0600` |
| G501/G401 (md5) | 2 | 仅用于 slug 生成，无安全风险，加 `//nolint:gosec` 注释 |

### D4: errcheck 处理模式

- `.Close()` / `.Rows.Close()` → `defer` 并忽略（标准模式）
- `json.Unmarshal` → 补充错误检查
- `logging.go` 的 `Output()` → 加 `//nolint:errcheck`（日志失败无意义重试）
- Handler 中的错误返回 → 补充 `c.JSON(500, ...)` 错误响应

### D5: staticcheck 按代码编号处理

- SA1012 (nil context) → 改为 `context.TODO()`
- SA1019 (deprecated) → 替换为推荐 API
- SA9003 (empty branch) → 删除空分支或补充注释
- SA4006 (值未使用) → 修正赋值
- QF* (quick fix) → 应用建议（Sprintf 简化、switch 替换等）
- S10xx (simplify) → 应用简化建议

### D6: 前端 any → proper type

28 个 `no-explicit-any` 全部替换为具体类型。API 响应类型优先使用 `front/app/types/api.ts` 中已有定义，缺失的按实际 API 响应补充。

### D7: TDD 豁免

本变更为纯静态分析修复，不涉及新逻辑或行为变更。按照执行规范 §2.3「纯配置豁免」条款，免除 TDD 要求。验证方式为门禁命令通过。

## Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| 删除 unused 代码可能误删预留功能 | 所有删除可通过 git 历史恢复；unused 意味着当前无调用者 |
| G118 context 传递可能改变 goroutine 生命周期 | 只在有 request context 可用时传入；Background 仅在测试保留 |
| 前端类型补充可能与实际 API 不一致 | 对照 `backend-go` handler 响应结构验证 |
| 大批量改动难以 code review | 按 phase 分批提交，每批有独立验证 |
