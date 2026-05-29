Status: ready-for-agent

## Parent

fix-tags-page-fractures

## What to build

在 `/tags` 页面的标签层级树底部添加"未归属"折叠区域，展示没有父级抽象标签的标签。

当前问题：后端 `/topic-tags/hierarchy` API 可能不返回未归属标签数据。需要先确认 API 返回结构，如果缺少则需后端补充返回 `unplaced` 字段。

前端在 TagHierarchy 中添加 `unplacedTags` ref 和 `showUnplaced` toggle，在树底部渲染折叠区域，复用 TagHierarchyRow 组件展示。

## Acceptance criteria

- [ ] 确认后端 `/topic-tags/hierarchy` 是否返回未归属标签数据
- [ ] 如果后端不返回，补充后端 API 返回 `unplaced` 字段
- [ ] TagHierarchy 新增 `unplacedTags` state 和 `showUnplaced` toggle
- [ ] 树底部渲染"未归属"折叠区域，展示未归属标签数量
- [ ] 折叠区域中复用 TagHierarchyRow 组件
- [ ] 后端改动: `go test` 通过；前端改动: `pnpm lint` + `pnpm exec nuxi typecheck` 通过

## Blocked by

None - can start immediately (但需先确认后端 API 返回结构)

## Reference

- Plan: `docs/plans/2026-05-17-fix-tags-page-interaction.md` Task 2
- `TagHierarchy.vue:369-541`, `abstractTags.ts:fetchHierarchy`
