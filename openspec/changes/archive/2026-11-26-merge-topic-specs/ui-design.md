<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档结构重构**，零 UI 变更：

- 只改 `openspec/specs/` 的文件边界（4 → 2，两族均就地扩容），不产生任何界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- 虽然搬运的条款里含用户可见交互（泳道节点标记、日报物化栏位），但**契约文本一字不改**——字段、交互、文案、降级行为全部原样搬迁，无行为变化
- 之所以不是 `minor`：minor 档要求「复用契约 + 挂验收证据」，而本次没有任何新的 UI 行为需要验收；把契约从文件 A 搬到文件 B 不构成 UI 变更
