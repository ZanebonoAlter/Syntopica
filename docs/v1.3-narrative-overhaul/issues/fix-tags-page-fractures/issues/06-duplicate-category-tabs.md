Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

修复 /tags 页面中 TagsPage 顶部分类标签页和 TagHierarchy 内部分类标签页双重显示冲突。

修复：给 TagHierarchy 新增 `hideCategoryTabs` prop，/tags 页面传 true 隐藏内部标签页，由 TagsPage 顶栏统一控制分类切换。同时强化 category prop 的 watch 同步逻辑——当 tabs 隐藏时始终从父组件同步分类。

## Acceptance criteria

- [ ] TagHierarchy 新增 `hideCategoryTabs` prop
- [ ] 当 `hideCategoryTabs=true` 时隐藏内部分类按钮
- [ ] TagsPage 传入 `:hide-category-tabs="true"`
- [ ] TagsPage 顶栏切换分类 → TagHierarchy 正确同步过滤
- [ ] /topics 页面中 TagHierarchy 分类标签页不受影响
- [ ] `pnpm lint` + `pnpm exec nuxi typecheck` 通过

## Blocked by

- `01-click-dblclick-conflict` — TagsPage 对 TagHierarchy 的 prop 修改在同一次改动中

## Reference

- Plan: `docs/plans/2026-05-17-fix-phase14-16-bugs.md` Task 3
- `TagHierarchy.vue:11-18, 387-392, 439-478`
