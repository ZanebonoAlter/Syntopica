Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

修复前端 LLM 板块确认 API 数据结构不匹配。后端 `confirmRegenerateSectors` 期望 `{ category, diff: { keep, add, merge, split } }` 但前端发送 `{ category, ...diff }`（展开），导致后端收到空 `diff` 结构，确认操作无效果。

修复：前端 `boardConcepts.ts:confirmRegenerateSectors` 中将 `...diff` 改为 `diff`，嵌套传递。

## Acceptance criteria

- [ ] `boardConcepts.ts:confirmRegenerateSectors` 请求体改为 `{ category, diff }`
- [ ] LLM 重新生成板块 → 调整 keep/add/merge/split → 点击确认 → 后端正确接收 diff 数据
- [ ] `pnpm exec nuxi typecheck` 通过（无类型变更）

## Blocked by

None - can start immediately

## Reference

- Plan: `docs/plans/2026-05-17-fix-phase14-16-bugs.md` Task 1
- `front/app/api/boardConcepts.ts:101-106`
