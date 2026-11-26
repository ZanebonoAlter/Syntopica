<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档结构重构**：

- 只改 `openspec/specs/` 的文件边界（6 → 2），不产生界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- `board-editing-ui` 描述的对话框与 direction_mismatch 展示契约**原样搬运**，字段与交互零变化
- Requirement 逐字搬运，用户可见行为零变化
