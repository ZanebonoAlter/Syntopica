<!-- stage: implement -->
<!-- constraint-domains: semantic-board -->

# unify-lane-canvas-shell — 阶段载体

## 当前进度

| 制品 | 状态 | 备注 |
|---|---|---|
| proposal | ✅ 完成 | `complexity: simple` / `ui-impact: minor` / `ui-approval: not-required` / `ui-prototype: none` |
| ui-design | ✅ 完成 | minor 复用契约：入口零变更 + 受影响状态子集 + Interaction Decisions D2/D3 + Acceptance 指名到测试文件 |
| specs | ✅ 完成 | 2 个 delta：`lane-correction-surface`（3 条 ADDED：横向吸顶 / 纵向吸顶 / 手势同源）、`workshop-lane-compose`（1 条 ADDED：吸顶列是共用单元） |
| design | ✅ 完成 | D1–D8 三行齐备，落点全部可解析 |
| tasks | ✅ 完成 | 8 节含测试/文档/验证三固定尾节；`doc-impact: flow` |

## 探索阶段已核实的事实（实现时不必重探）

1. **两支画布共享纯函数层但不共享外壳**：`laneComposeLogic.ts` 已被两侧 import；缩放/平移/测量/滚轮/Esc 各写一份。
2. **三处已发生的漂移**：编辑侧拖拽期滚轮仍缩放；编辑侧 `Esc` 只取消拖拽不取消平移；两侧吸顶能力不对称。
3. **编辑侧几何**：`LEFT_PAD=150`、`FOLD_W=68`、`FOLD_GAP=8` → `axisX0` 在 150 / 226 之间；`MIN_COLUMN_WIDTH=64`；86 天窗口 `vbWidth≈5682px` vs 容器 ≈1900px。
4. **新建侧吸顶参数**：`LABEL_W=132`；`label-bg → --color-bg-base`；`sticky-tint → --color-bg-elevated`；6 级阴影 `width:1`、`x = LABEL_W + band - 1`、`opacity = 0.28 - (band-1)*0.045`；全部 `overflowingX` 门控；文件内**内联 3 份**。
5. **新建侧已有吸顶测试**（`LaneComposeCanvas.test.ts` 的「本次泳道行吸顶（P-1）」组），断言含「不溢出时 `row-label-bg` 为 0」「`row-label-shadow` 数 === `row-label-group` 数」→ 抽单元时四个 `data-role` 钩子必须原样保留。
6. **编辑侧零吸顶测试**（`LaneCorrectionCanvas.test.ts` 无 `吸顶`/`scrollLeft` 断言）。

## 用户已拍板的决策（2026-11-25）

1. **本轮范围 = L1 外壳 + 吸顶（①②）**：抽外壳 composable + 抽吸顶单元并让编辑侧复用；列宽口径（③）留后续 change。
2. **编辑侧分区标签修法 = 像新建那样钉一列**（不采用「顶部位置条」「分区名挂顶部」两个备选）。
3. **固定列宽度取 `axisX0`（动态）**，让折叠带与其首义勾选框常驻；接受折叠带出现时宽度跳变一次。

## 明确不在这轮（已写入 proposal 非目标）

- 列宽下限 64 vs 124 的统一（后续 change）
- 纵向语法「分区 vs 行」的合并（建议永不统一）
- 节点标签策略「两行换行+跳过」vs「单行截断+整档关闭」（L2 议题）
- 任何数据口径改动
