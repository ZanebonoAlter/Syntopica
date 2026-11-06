<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是 **harness 注入机制 + 文档标签维护**，不产生任何用户可见界面：

- 改动面在 `.pi/extensions/constraint-injection.ts`（注入 gate）、`docs/reference/flow/*.md`（头部元数据标签）、`scripts/harness/harness-retro.sh`（探针 SQL）
- 唯一"用户可见"效果是**模型上下文里多/少几段约束文字**，与浏览器渲染、布局、交互无关
- 全部验收走扩展 smoke 用例 + 事件库记账查询，无前端路径、无 API 契约

`ui-impact: none` 与实际改动面一致；不创建 `ui-prototype/`，不需要 UI 审批。
