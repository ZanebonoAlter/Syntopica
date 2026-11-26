<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档结构重构**：

- 只改 `openspec/specs/` 的文件边界（4 个 spec → 2 族 + 1 保留），不产生界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- Requirement 逐字搬运，用户可见行为零变化
- `radar-pipeline-status` 本身就是前端契约，本次**保留不动**，不前移也不改字段
