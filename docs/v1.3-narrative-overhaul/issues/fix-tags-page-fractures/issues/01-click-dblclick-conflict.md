Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

修复 `/tags` 页面 TagHierarchy 组件中 click/dblclick 事件冲突。当前单击 label 会触发 `select-tag` 事件导致页面异常导航，双击重命名因单击先触发而不可用。

修复方案：给 TagHierarchy 新增 `selectable` prop（/tags 页面传 false 禁止 select），在 TagHierarchyRow 中用 250ms click timer 区分单击和双击。TagsPage 传入 `:selectable="false"`。

修复后：/tags 页面中单击只展开/折叠节点，双击进入行内编辑模式。

## Acceptance criteria

- [ ] TagHierarchy 新增 `selectable` prop，默认 true
- [ ] TagHierarchyRow label 按钮使用 click timer 区分单击/双击
- [ ] TagsPage 传入 `:selectable="false"`
- [ ] /tags 页面单击节点不再触发导航
- [ ] /tags 页面双击节点可进入行内编辑模式
- [ ] /topics 页面（TopicGraphPage 中的 TagHierarchy）单击行为不受影响
- [ ] `pnpm lint` + `pnpm exec nuxi typecheck` 通过

## Blocked by

None - can start immediately

## Reference

- Plan: `docs/plans/2026-05-17-fix-tags-page-interaction.md` Task 1
- `TagHierarchyRow.vue:134-142`, `TagHierarchy.vue:316-318`, `TagsPage.vue:245`
