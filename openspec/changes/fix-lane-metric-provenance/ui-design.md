<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约 · fix-lane-metric-provenance

> 判定依据：本 change 只改**数字口径与文案**，不新增页面、不改布局模式、不改交互流程、不新增组件位。属 minor（复用既有组件与状态契约），故不制作可视原型。

## 入口与入口变更

**入口全部不变**（无新增、无移除、无禁用）：

| 入口 | 位置 | 触发元素与当前状态 | 本 change 的变化 |
| --- | --- | --- | --- |
| 矩阵格 tooltip | `console/ObservationMatrix*.vue` | 悬停/聚焦矩阵格 → 显示格值 + 伴生说明 | **文案与口径变**：伴生说明里的线索条数改与格值同口径，并补「另有 N 条综合稿未计入」 |
| 矩阵区口径说明 | `console/ObservationMatrix*.vue` | 常驻一行（新增只读文本行，不是入口） | **新增一行常驻说明**（数字 = 当日线索合计 / 综合稿不计入） |
| 当日线索工作面 | `console/ObservationMatrix*.vue` → 当日工作面 | 点日格进入；条目为只读 | **条目加综合稿标记**（只读徽标，非可点） |
| 当日其他话题组 | `console/dayOtherTopics.ts` | 矩阵下方只读分组 | **数字口径变**（线索合计改非综合稿）；无新增操作 |
| 泳道工作面左栏 | `console/LaneMemberList.vue` / `LaneAnchorScope.vue` | 成员行勾选框（破坏性：取消勾选即移除）、关联推荐行「＋加入」、首义范围勾选 | **计数与标签变**：「全部 M」同定义；接近度加「估算/快照」区分；**勾选框的允许性与触发条件完全不变** |
| 话题生命线 | API `GET /api/daily-reports/topics/:id/lifeline` | 只读 | **响应字段变化**（`topic_match_distance` 不再省略 0 值），非 UI 入口 |

操作允许性盘点（AGENTS.md §0.6 要求）：本 change **不改任何动作的允许性**（谁可勾选、谁可加入、什么条件下禁用均不变），因此无需改动绑定属性。

## 受影响状态

| 状态 | 是否受影响 | 说明 |
| --- | --- | --- |
| loading | 否 | 骨架与加载态不变；新增的「桶级 composite_count」随同一响应返回，不引入二次加载态 |
| empty | 否 | 空态文案与触发条件不变（无分区日仍为断档 `—`、无成员仍为空态提示） |
| error | **轻微** | 「当日其他话题」组若线索合计口径无法判定（各桶线索均未知），SHALL 显示 `?` 并标注口径未知，MUST NOT 造 0；错误重试路径不变 |
| success | **是** | tooltip 伴生数字、常驻口径说明、当日工作面综合稿标记、成员计数与接近度标注 |

## 复用组件与布局模式

- 布局模式：**不变**（矩阵页 `contained`/`workspace` 既有模式；泳道工作面 `workspace` 两栏 380px + 中央画布）。
- 复用既有组件：**不新增任何基础组件**。新增的「综合稿」标记为既有徽标样式（同泳道工作面「首义」徽标那一档），SHALL NOT 另造浮层或新档位。
- 常驻口径说明行复用既有 muted 小字样式（与图例说明同档），SHALL NOT 用强调色，SHALL NOT 做成可交互元素。

## 验收映射

| 制品要求 | 验证方式 | 测试文件（仓库根相对路径） |
| --- | --- | --- |
| 矩阵格 tooltip 伴生数字同口径 | 组件测试：断言 tooltip 不含含综合稿的线索条数、含差额说明 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 常驻口径说明行 | 组件测试：断言矩阵区渲染口径说明文本 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 当日工作面综合稿标记 | 组件测试：断言综合稿条目带标记 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 「当日其他话题」线索合计口径 | 单元测试：断言合计排除综合稿 | `front/app/features/tags/components/console/dayOtherTopics.test.ts` || 前端回退路径不冒充线索数 | 单元测试：构造缺 `clue_count` 的响应，断言回退值为非综合稿合计 | `front/app/features/tags/composables/laneMatrixModel.test.ts`（**本 change 新建**） |
| 两个「全部 M」同定义 | 单元测试：构造含窗口内外成员的夹具，断言两处计数一致 | `front/app/features/tags/composables/useLaneCorrection.test.ts` |
| 接近度区分快照与估算 | 单元测试 + 组件测试：断言两类节点标签不同 | `front/app/features/tags/composables/useLaneCorrection.test.ts`、`front/app/features/tags/components/console/LaneMemberList.test.ts` |
| 生命线 distance 0 值不丢 | 后端测试：断言 `topic_match_distance: 0` 出现在响应 JSON 中 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 桶级 composite_count | 后端测试：断言响应含该字段且值正确 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 两个窗口末日一致 | 后端测试：构造未完成日报，断言两处窗口末日相同 | `backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go` |
