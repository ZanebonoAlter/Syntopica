<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, topic-graph, daily-report -->

# Proposal: 泳道/矩阵数字口径修正——线索合计口径收敛 + 伴生数字对齐 + 计数去歧义

## Why

矩阵与泳道工作面上一份响应里并存两套数，用户看到的是互相打架的数字：

```
矩阵格大字：   3
tooltip：     "3 条线索 · 1 个主题桶（最多一天桶 5 条）"   ← 同一句里 3 和 5
点开当日：    5 条线索条目（综合稿无任何标记）
```

根因不是矩阵算错，而是**伴生数字没跟着收敛**：

- `lane-dynamics` 响应里 `day.clue_count` = 当日线索合计（**排除**综合稿）→ 矩阵格大字 / 演进节点 / 池曲线 / lane_heat 输入；
- 同一响应的 `sections[].thread_count` = 该桶 thread 总数（**包含**综合稿）→ 被 tooltip 当成「一天桶 N 条」露出来。

矩阵口径本身是对的（`docs/reference/flow/semantic-board.md` 红线：section 数、桶数、thread 条数、事件数 MUST NOT 参与任何热度或排序计算）。缺的是两件事：① tooltip 把另一口径的数当「辅助说明」露出来；② 「综合稿不计入」的说明随事件线退役后，界面上**再没有任何一行**告诉用户矩阵数排除了综合稿。

除这条主线外，本 change 一并修掉同一轮溯源确认的其余真缺陷：两个「全部 M」不是同一把尺子量出来的、窗口内外成员集合不对称、「最新日报」两个锚点差一天、距离恰为 0 时字段被 JSON 省略、「接近度」两套算法同名同标签。

## What Changes

### 口径主线（伴生数字收敛 + 差额可见）

1. **后端**：`lane-dynamics` 响应每个主题桶补 `composite_count`（该桶综合稿 thread 数；`listLaneSectionThreads` 已在 select `scope`，几乎零成本）。
2. **tooltip**：矩阵格 tooltip 的「最多一天桶 N 条」SHALL NOT 再把含综合稿的 thread 总数当线索数露出；改为与矩阵格同口径的线索数，并显式补「另有 N 条综合稿未计入」。
3. **「当日其他话题」组**：「线索合计」改用非综合稿口径（现取 `section.threads.length`，含综合稿），并修正其注释里「与矩阵主行格值同口径」的失实陈述。
4. **前端回退路径**：旧后端缺 `clue_count` 时的回退（`laneMatrixModel.ts` 与 `dayCellMetrics.total`）口径统一，不得再用含综合稿的 `thread_count` 冒充线索数。
5. **池曲线口径与文案**：顶部时间视窗曲线的降级规则（「事件线数 > 0 用事件线数，否则线索合计兜底」）随事件线退役失效，SHALL 收敛为「非综合稿线索数合计」单一口径；曲线单位文案 SHALL 直接表达「线索数（非文章数）」，MUST NOT 再出现「事件线」「兜底」等已退役量纲字样。
5. **常驻口径说明**：矩阵区常驻一行说明——数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度）。

### 同轮真缺陷修复

6. **两个「全部 M」同尺**：左栏「窗口 N · 全部 M」与首义范围面板「窗口内 N · 全部 M」SHALL 用同一定义（本话题成员总数），不再一个用加法一个用去重并集。
7. **窗口内外成员集合对称**：compose-candidates 的 `sections` 要求 `embedding IS NOT NULL`、`early_members` 不要求 → 「窗口 N」与「全部 M」口径不对称。两侧 SHALL 用同一过滤判据。
8. **「最新日报」锚点统一**：lane-dynamics 取 `MAX(period_date)` 不筛 status、compose-candidates 取 `status='completed'` → 有未完成日报时两个窗口末日差一天。SHALL 统一为 completed 口径。
9. **距离 0 不丢**：话题生命线的 `topic_match_distance` 是 `float64 + omitempty` → 距离恰为 0 时字段被省略，前端接近度变 null。SHALL 不再省略 0 值。
10. **「接近度」标签自洽**：成员接近度 = 日报生成时的归属快照距离换算；候选/推荐接近度 = 对当前成员质心的实时估算。SHALL 保留两把尺子（数据来源不同），但标签 SHALL 能区分。**本 change 的范围只限泳道工作面（编辑泳道）**；新建泳道编排侧（画布节点卡 / 右栏「距离 x.xxx」）的同向统一归 `unify-lane-workbench`（该侧画布正在重做，避免同文件并行写）。
11. **lane_heat 的 NULL scope 判据**：`COALESCE(t.scope,'') <> 'composite'` 让历史 NULL scope 按非综合稿计入，SHALL 显式确认并在代码注释与文档中写死该判据。
12. **文档口径错**：`docs/reference/flow/topic-graph.md` 写候选门槛是 `consecutive_hits`，代码实际用 `hit_count >= upgrade_threshold`（代码两处自洽）→ 修正文档。

## Capabilities

### New Capabilities

（无 —— 本 change 不引入新能力，只收敛既有能力的口径。）

### Modified Capabilities

- `board-lane-dynamics`: 矩阵格值、tooltip 伴生数字、「当日其他话题」线索合计、前端回退路径、常驻口径说明、池曲线与矩阵格的同口径承诺。
- `persistent-topic`: 话题成员集合口径（窗口内/更早成员对称、两个「全部 M」同定义）、成员接近度快照语义、候选接近度「估算」标注、话题生命线 `topic_match_distance` 的 0 值不得省略。

## Impact

**后端**（`backend-go/internal/topicgraph/`）
- `repository/lane_snapshot_repository.go`：桶级 `composite_count` 字段、report 锚点补 completed 过滤。
- `repository/daily_report_manual_topic.go`：`early_members` 与 `sections` 的 embedding 过滤对称。
- `repository/daily_report_models.go` / `daily_report_repository.go`：`topic_match_distance` 去 `omitempty`。
- `repository/lane_heat_repository.go`：NULL scope 判据注释化。
- `handler/lane_dynamics_handler.go`：响应 DTO 补字段。

**前端**（`front/app/features/tags/`）
- `components/console/ObservationMatrix*.vue`、`composables/useConsoleSelection.ts`、`api/normalizers/laneDynamicsWindow.ts`：tooltip 与矩阵格口径。
- `components/console/dayOtherTopics.ts`：「当日其他话题」线索合计与注释。
- `composables/laneMatrixModel.ts`：前端回退路径。
- `composables/useLaneCorrection.ts`、`components/console/LaneMemberList.vue`、`components/console/LaneAnchorScope.vue`：「全部 M」同定义。

**文档**
- `docs/reference/api/daily-reports.md`、`docs/reference/api/semantic-boards.md`：响应字段变化。
- `docs/reference/flow/topic-graph.md`：候选门槛判据修正。
- `docs/reference/flow/semantic-board.md`：矩阵格口径与综合稿说明（§12 变更溯源）。

**并发提示**：本 change 与 `unify-lane-workbench`（画布统一 + 时间视窗）在 `ObservationRangeControls.vue` / `LaneCorrectionCanvas.vue` 上有潜在重叠。本 change 只做 `ObservationRangeControls.vue` 的**曲线口径与单位文案**修正（不碰该组件的结构与交互）；`LaneCorrectionCanvas.vue` 的画布图例与「大小=接近度」/成员半径不符的修正归 `unify-lane-workbench`。两条 change **串行**执行（先本 change 并 commit，再起下一条）——串行后同文件无写冲突。
