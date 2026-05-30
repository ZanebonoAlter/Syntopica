# #8 - 移除"整理标签"按钮

## What to build

`TagHierarchy.vue` 的 `organizeUnclassified()` 调用 `POST /topic-tags/organize`，这是旧 topic-graph 时代的 AI 批量归类接口，通过 `useOrganizeWebSocket` 监听 `organize_progress` 事件。该功能与 `/tags` 页面的新层级闭环体系（Sector → PlaceTagInHierarchy → PendingChange）完全不同，会造成用户混淆。

移除"整理标签"按钮及相关代码：`organizeUnclassified` 函数、`useOrganizeWebSocket` 引用、`organizing`/`organizeResult` 状态、`.th-organize-btn` 样式。保留 composable 文件本身（其他页面可能仍在使用），只移除 `TagHierarchy.vue` 中的调用。

## Acceptance criteria

- [ ] `TagHierarchy.vue` 中"整理标签"按钮及相关 UI 不再渲染
- [ ] `useOrganizeWebSocket` composable 文件保留不动
- [ ] 其他引用 `useOrganizeWebSocket` 的组件不受影响
- [ ] 层级 header 区域布局正常（标签层级标题 + 标签数量）

## Blocked by

None - can start immediately.
