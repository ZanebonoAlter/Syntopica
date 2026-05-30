# #4 - Feed/Category CRUD 事件处理器 no-op 问题

## What to build

`FeedLayoutShell.vue` 中多个 dialog 的事件处理器为 `() => {}` 空函数（`@added`, `@updated`, `@deleted`）。当前依赖 store 内部 re-fetch 刷新数据，但若 store 刷新静默失败，UI 不会更新。

统一改为：dialog 事件触发时，由父组件显式调用 `fetchFeeds()` / `fetchCategories()` 确保数据一致性。

## 受影响位置

- `FeedLayoutShell.vue` line ~514: `<DialogAddFeedDialog @added="() => {}" />`
- `FeedLayoutShell.vue` line ~521: `<DialogAddCategoryDialog @added="() => {}" />`
- `FeedLayoutShell.vue` line ~528: `<DialogEditCategoryDialog @updated="() => {}" />`
- `FeedLayoutShell.vue` line ~535: `<DialogEditFeedDialog @updated="() => {}" @deleted="() => {}" />`

## Fix

每个 no-op handler 改为调用对应刷新函数：
- `@added` → `fetchFeeds` + `fetchCategories`
- `@updated` → 对应的 `fetchFeeds` 或 `fetchCategories`
- `@deleted` → `fetchFeeds` + `fetchCategories`

## Acceptance criteria

- [ ] 所有 `() => {}` handler 替换为实际的刷新调用
- [ ] Feed/Category CRUD 操作后 sidebar 立即刷新
- [ ] `pnpm lint && pnpm exec nuxi typecheck` 通过

## Blocked by

None - can start immediately.
