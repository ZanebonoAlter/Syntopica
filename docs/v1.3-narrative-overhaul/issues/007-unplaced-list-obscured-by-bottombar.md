# #7 - 未归属列表被底部栏遮挡

## What to build

`TagsPage.vue` 的 bottombar 使用 `position: fixed`（不占文档流空间），而 `.tags-content` 的可滚动区域高度为 `100vh - topbar高度`，没有为底部栏预留空间。展开未归属标签列表时，最后几行被底部栏遮挡，看起来像"渲染出界"。

给 `.tags-content` 添加 `padding-bottom`（约 56px，等于 bottombar 高度），确保内容不被遮挡。或者将 bottombar 从 `position: fixed` 改为参与 flex 布局（如 sticky）。

## Acceptance criteria

- [ ] 展开未归属标签列表时，所有标签行可见，不被底部栏遮挡
- [ ] 树内容较长时滚动到底部，最后的内容不被遮挡
- [ ] 底部栏的 pending change/rebuild 功能不受影响
- [ ] 不引入新的布局抖动或闪烁

## Blocked by

None - can start immediately.
