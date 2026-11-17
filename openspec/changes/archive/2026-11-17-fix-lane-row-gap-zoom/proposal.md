# Change: 泳道修正画布成员区间距跟随半径（滚轮放大后仍重叠）

<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

编辑泳道的成员区（画布顶部实心节点带）行距写死 `slot * 16`，而**单个节点的纵向总占高是 34.5px**：

| 组成 | 值 |
| --- | --- |
| 成员圆直径（`MEMBER_BASE_RADIUS` 5.5 固定，**不随列宽变**） | 11 |
| 标题起笔间距（`LABEL_TOP_GAP`） | 5 |
| 标题最多两行（`LABEL_MAX_LINES` 2 × `LABEL_LINE_H` 12） | 24 |
| **合计（圆顶 → 标签底，行距基准）** | **40** |

**圆本身不重叠（16 > 11），是标题标签压在下一个节点的圆上。** 同一天 2 个成员就必然压盖。

滚轮缩放是等比拉伸（`:width="vbWidth * zoom"` 配固定 `viewBox`），放大只把重叠一起放大，**比值不变**，所以放大不缓解拥挤。用户的直接反馈是「放大后间隔应该也顺应放大，但现在的间隔卡死的」。

> 排查校正（2026-11-17）：本 change 立项时曾按 `pointAppearance` 的默认路径推算成员半径可达 13.6px，据此判断"圆重叠"。核实后发现本画布 `appearanceOf` 传了 `memberRadius: MEMBER_BASE_RADIUS`（5.5）覆盖，**真实半径恒为 5.5**。实现与 spec 已按"标签占高"这一真实成因改写；按半径推导行距会得出 16px 并保留缺陷。

这是既有实现的缺陷，不是新增需求；`unify-lane-workbench` 归档后才在真机上暴露。

## What Changes

- **三个区统一行距，按节点纵向占高**：成员区 `slot * 16`、候选/已移出区 `slot * 22`、只读参照区 `slot * 12` → 统一为 `slot * max(16, 圆直径 + 标签起笔间距 + 标签最多行数 × 行高 + 4)` = **44px**，保证上一个节点的两行标签不碰到下一个节点的圆。公式只用常量（`MEMBER_BASE_RADIUS` / `LABEL_TOP_GAP` / `LABEL_MAX_LINES` / `LABEL_LINE_H`），**不读列宽**——半径固定，读列宽只会得出 16px 并保留缺陷。
- **候选与已移出共用堆叠池**：原先两者各自从 0 计数、起点都是 `zone1Top`，同一天的候选与已移出节点会**重合在同一坐标**；改为共用同一计数池顺序排布。
- **每个区高度按自身堆叠需求**：三个区不再固定 60px，而是按「该区同日最大堆叠数 × 行距」计算，画布总高随之增长（滚动承接）。
- **成员区高度驱动分区位置**：成员区底部随「同一天最多成员数 × 行距」下移，`zones.zone1Top` 不再硬编码 `156`；`canvasHeight` 至少容纳成员区 + 两个分区的最小高度，超出走容器滚动。
- **等比缩放的语义不变**：滚轮仍等比拉伸（实测 `width 900→990`、`viewBox` 不变），本次不改变缩放机制，只修正「行距 / 直径」这个被写死的比值。
- **不改节点半径**：`MEMBER_BASE_RADIUS = 5.5` 保持不变（它是刻意的固定小尺寸）。本次只放开行距。
- **不改新建泳道画布**：`LaneComposeCanvas` 的 `rowGap ≥ 36`，其成员标签布局与行距策略不同且未报重叠，不纳入本次改动。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `lane-correction-surface`：画布成员区**同日节点不得重叠**成为可验收约束（新增一条 Requirement + Scenario），并把「滚轮缩放只影响呈现、不改变布局比值」写清楚——缩放的语义是放大镜，不是解拥挤的手段，拥挤必须由布局本身保证。

## Impact

- **前端**：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue`（行距、成员区高度、分区位置）、`front/app/features/tags/composables/laneComposeLogic.ts`（新增 `defaultMemberRadius` 纯函数）、对应两个测试文件。
- **后端**：无。无接口、无数据模型、无迁移改动。
- **数据**：无。仅呈现层布局计算。
- **风险**：成员区变高后，同一天成员很多时画布整体变高（纵向滚动）。这是刻意取舍——宁可滚动，不可重叠。已在 `ui-design.md` 的 Layout Contract 记录。
