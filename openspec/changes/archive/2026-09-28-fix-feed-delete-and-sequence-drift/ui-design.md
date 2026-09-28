<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只改后端删除路径与启动期序列自愈，不涉及任何界面元素、文案、布局或交互变化：

- **删源确认弹窗**（设置页 `useGlobalSettings.deleteFeed` → `window.confirm`）与**删分类确认弹窗**（`FeedLayoutShell.vue` → `confirm`）的文案本就声明「连带删除其下订阅源及其文章，且不可撤销」。本次是让后端行为兑现既有文案，弹窗结构、文案、确认方式均不改。
- **删除成功后的反馈**不变：删源走 `fetchFeeds({ per_page: 10000 })` 刷新列表 + 2 秒成功提示；删分类刷新分类列表；失败时既有错误提示位（`error` / `alert`）已能显示后端错误，本次不再失败即无需新增提示。
- **无新增/修改组件**：不涉及 `AppPageShell` / `AppDialog` / `AppButton` 等复用契约，不涉及状态矩阵（loading/empty/error/success）与布局模式、dialog 尺寸档、新增图标。
- **窄部署/生产残留表场景**同样零界面差异：删除结果一致（成功或失败），只是失败原因消失。
