<!-- doc-impact: api -->

## 1. 复现测试先行（bug 修复不可豁免）

- [x] 1.1 handler 复现测试：`PUT /api/articles/bulk-update` `{read:true}` 无 scope → 400 "Must specify a scope"（现状红 → 修复后该用例仍应 400，语义保持）；`{read:true, all:true}` → 修复前 400（复现 bug）、修复后 200 且全站 read 置位，验证：handler 测试红转绿留痕
- [x] 1.2 互斥用例：`{all:true, feed_id:1}` → 400；`{all:true}` 无更新字段 → 400 "At least one field"，验证：handler 测试绿

## 2. 修复

- [x] 2.1 后端 `article_handler.go` bulk 分支：新增 `all` scope（与 ids/feed_id/category_id/uncategorized 互斥校验），验证：`go test ./internal/reader/...` 绿
- [x] 2.2 前端 `stores/articles.ts` 无 options 分支传 `all: true` + types `BulkUpdateArticlesData.all?`，验证：`stores/api.test.ts` 补断言（无 options → body 含 all:true）+ `pnpm exec vitest run app/stores/api.test.ts` 绿
- [x] 2.3 影响包测试：`go test -short ./internal/reader/...` 全绿

## 3. 测试

- [x] T.1 test-cases.md：主链路（全部文章视图点全部标为已读 → all 请求 → 200 → unread 归零）+ 变体走查（五组：无 scope/all+scope 互斥/仅 scope 无字段/空库 all/重复提交幂等），验证：文件存在
- [x] T.2 后端影响包：`go test -short ./internal/reader/...`，期望全绿

## 4. 文档

- [ ] D.1 `docs/reference/api/`（articles API 文档所在文件）补 bulk-update 的 all scope 说明，验证：grep 命中

## 5. 验证

- [x] V.1 `cd backend-go && golangci-lint run ./internal/reader/... && go build ./...`，期望零报错
- [x] V.2 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm exec vitest run app/stores/api.test.ts`，期望全绿
- [x] V.3 人工（2026-09-18 实测）：dev 后端重启为新二进制后，①API 级 `{read:true,all:true}` → 200（全站 19410 篇置已读，未读 4361→0，DB 复核）；②浏览器全部文章视图点「全部标为已读」→ 无 400 报错弹层，DB 未读 0。人工验证通过

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 无 scope 无 all 返回 400 | backend-go/internal/reader/handler/bulk_update_test.go |
| 显式 all 全站标已读 | backend-go/internal/reader/handler/bulk_update_test.go |
| all 与其他 scope 互斥 | backend-go/internal/reader/handler/bulk_update_test.go |
| 仅有 scope 无更新字段返回 400 | backend-go/internal/reader/handler/bulk_update_test.go |
