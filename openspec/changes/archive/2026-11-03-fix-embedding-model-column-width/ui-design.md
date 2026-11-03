<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为**纯后端 schema 修复**：把 5 张 embedding 表的 `model` 列由 `VARCHAR(50)` 加宽到 `VARCHAR(100)`，同步 gorm tag，补迁移与回归测试。

- 不新增/修改任何前端页面、面板、弹窗、导航、多步流程；
- 不改变任何用户可见文案、字段、交互；
- `frontend/` 目录零改动（`git diff --name-only -- front/` 应为空）。

UI 验收：**N/A**（`ui-verification-trace.sh` 判定 none 档要求 N/A 一致性——本 change 不产出 `ui-verification/report.md`，因为没有任何 UI 变更可验收）。
