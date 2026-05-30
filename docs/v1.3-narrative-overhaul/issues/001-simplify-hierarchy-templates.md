# #1 - 简化层级模板：删除 keyword 子类型，统一为单个 keyword 模板

## What to build

当前硬编码层级模板有 5 个：`event`、`person`、`keyword:technology`、`keyword:company_business`、`keyword:concept`。但实际标签生成时 category 只有 `event | person | keyword` 三个值，且 `sub_type` 字段因 bug 均为 NULL，导致 `GetTemplate("keyword", "")` 返回 nil，413 个 keyword 标签全部无法进入层级树。

将 keyword 的三个子模板合并为一个裸 `keyword` 模板（3 层），与 event 模板结构对称。删除三个子模板，使 `GetTemplate(category, "")` 对所有 category 都能命中。

## Acceptance criteria

- [ ] `BuildAllDefaultTemplates()` 只返回 3 个模板：`event`、`person`、`keyword`
- [ ] `keyword` 模板有 3 层（类似 event），层级命名合理
- [ ] `GetTemplate("keyword", "")` 返回非 nil 模板
- [ ] `GetTemplate("keyword", "technology")` 等旧 key 返回 nil（已删除）
- [ ] `go build ./...` 通过，`go test ./internal/domain/tagging/...` 通过
- [ ] 前端 HierarchyConfigPage 正常显示 3 个模板

## Blocked by

None - can start immediately.
