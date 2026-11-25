<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, topic-graph -->

# align-lane-correction-workface

## Why

「编辑泳道」（泳道修正工作面）与「新建泳道」（编排态画布）是同一个全屏弹层的两种模式，但能力不对齐：修正画布**没有任何节点拖拽**（只能点开节点卡按按钮），候选「相关度」只映射到 3.6–4.6px 半径、视觉上等同常量；而首义（anchor）在界面里恒显示「自动」——用户看不出哪条节点是首义来源，也看不出哪 30 条节点在参与质心滚动计算。同时新建画布的高度只按内容算（地线 460px），单泳道时下方空掉约 30% 视口。

这三处都指向同一件事：**用户在修正工作面做判断时缺少可操作与可核对的事实**。拖拽让归属变更从「三次点击」变成「一次拖动」；相关度分档让候选之间的差别看得见；首义与滚动窗口可见，才能解释「为什么这条新闻被这条泳道吸走」。

## What Changes

**A. 编辑画布对齐新建画布的拖拽与状态显示**

- 修正画布新增**节点拖拽改归属**：拖入成员区 = 加入/加回（立即落库），成员拖出成员区 = 移出（立即落库）；只读参照区与「更早」折叠带节点不可拖。松手即发请求，沿用现有 `add/remove/restore` 写路径与 `pendingIds` 防重入，**不引入「应用/取消」草稿态**（与既有「归属变更立即生效」口径一致）。
- 复用新建画布的 ghost 跟随、落点区高亮、4px 拖拽阈值、拖后 250ms 吞 click、Esc 取消等手势纪律（`laneComposeLogic` 的 `isDragBeyond/contentPointOf/applyDrop/resetGesture` + `.lcc-drag-ghost*`/`.lcc-canvas__dropzone` 样式）。
- 画布补齐**新建画布独有的落点视觉**：新建画布 `.is-drop-other`（拖到非目标行）目前无任何 CSS（已知缺陷），本 change 在修正画布实现时一并给出「不可落点」视觉，并回填到新建画布。

**B. 相关度分档看得见（编辑画布）**

- 候选节点尺寸/填色从「绝对阈值换算」改为**批内相对归一**（复用 `relativeProximities` + `recommendRange`/`strengthRange`/`proximityOverride`），并同步调整成员基准半径与依赖它的一整条布局链（`NODE_BLOCK_H/nodeRowGap/membersBottom/zones`）。
- 图例从「单一固定圆」改为**尺寸图例（小圆—大圆）**，与新版视觉一致。
- **修正图例语义错位**：「虚线 = 首义」目前只画未提交的勾选集，**已生效的首义成员在画布上没有标记**——改为已生效首义与待提交勾选各一种标记。
- **红线不变**：成员接近度（快照 `1-topic_match_distance`）与候选接近度（估算 `1-质心距离`）MUST 保持不同名不同形，相对归一**只作用于候选桶**。

**C. 首义来源可见**

- 读接口 `GET /api/daily-reports/topics/:id/anchor` **扩展返回**（同 struct 加字段，不新增往返）：
  - `anchor_origin`：首义来源节点（`section_id`/`title`/`period_date`/`resolved`）；`auto` 且不可考时显式 `resolved=false` + `null`，**不虚构**。
  - `centroid`：滚动窗口口径（`window_size`/`anchor_weight`/`section_ids`/`members[]`/`degraded`）。
- 抽出只读 `ListCentroidWindowSections(topicID, window)`，让 `ComputeTopicCentroid` 的内联 SQL 与读接口**共用一份口径**（防两份实现漂移）。
- 前端在左栏「首义范围」面板展示：首义来源节点（或「创建时写入 · 来源不可考」）+ 折叠组「参与滚动计算的节点（N/30）」；新增的「滚动窗口 N」与既有「窗口内 N · 全部 M」**分开命名**（后者是成员总数，不是同一把尺子）。
- 读接口**零写副作用**：不写表、不动 `centroid`、不触发重算、不回填快照。

**D. 手动建泳道时选中的节点即首义集合（写侧留痕）**

- 手动建泳道（`CreateManualTopic`）在创建事务内**追加一行 `topic_anchor_overrides`**（`source=selected_sections`、`input_summary` = 选中的 section id 列表），使首义来源可读为「人工（选定的 N 个节点）」。
- 留痕口径与人工重设首义**完全一致**（同一张表、同一字段语义），不新增来源枚举。
- **历史手动建的泳道不回填**：当时未留痕，事后推断即编造，继续显示「创建时写入 · 来源不可考」；用户想补齐可走既有「用选中的 N 个节点定首义」重新定一次。
- **非目标**：算法自动建的候选话题（`auto_new`）不在此列（其首义是首条命中节点向量，是否留痕另行决定）。

**E. 新建画布高度贴合容器**

- `canvasHeight` 从「只按内容算 + 460/560 地线」改为**先贴合容器再按需撑高**（照编辑画布 `baseCanvasHeight/needed` 的现成写法），**多余高度全部给底部散料区**，本次泳道行与候选链行位置不变。
- 复核副作用：横向滚动条吃掉 clientHeight 的反馈环、`overflowing` 判据变化后第 0 行吸顶是否仍符合语义。

**F. 修正画布三处可用性补丁（2026-11-24 用户实机反馈追加）**

- **节点卡标题完整可读**：节点卡标题由「单行 + 省略号」改为**完整展示、按需换行**（卡高随内容实测，位置钳制口径不变）——卡的职责是让人认出「这是哪条」，截断即失职。
- **去掉话题带上方的强调色横条**：成员行上方那条强调色圆角横条（`.lcc-band-bar`）及其垂到成员节点的刻度线（`.lcc-band-tick`）不承载任何判断（成员归属已由圆点 + 勾 + 分区标签表达），整体移除；「本泳道话题带 / 泳道名」标签与中性基线保留，与候选区 / 只读参照区「一线 + 左标签」的形态一致。
- **点空白画布取消聚焦**：聚焦某节点后，**轻点画布空白处即取消聚焦**（节点卡收起、相关高亮 / 弱化恢复默认），与编排画布节点卡「×」同义；平移松手、落在节点上的轻点、落在节点卡 / 缩放条 / 折叠带勾选框上的操作**不算**取消聚焦，且取消聚焦 SHALL NOT 发任何写请求。

## Capabilities

### New Capabilities

（无——本 change 全部是对既有行为契约的修订与补齐）

### Modified Capabilities

- `lane-correction-surface`：修正画布新增拖拽改归属、相关度分档与尺寸图例、已生效首义标记、首义来源与滚动窗口展示、画布高度贴合容器；节点卡标题完整换行、话题带强调色横条移除、点空白画布取消聚焦（F 节）。
- `lane-manual-correction`：首义读接口返回来源与滚动窗口口径；「自动」不再等于「无从知晓」。
- `workshop-lane-compose`：画布高度贴合容器（余量给散料区）；补「不可落点」视觉。
- `persistent-topic`：手动建持久化话题 SHALL 写首义留痕（选中节点即首义集合），历史不回填。

## Impact

**后端（`backend-go/`）**
- `internal/topicgraph/repository/daily_report_anchor_read.go`：扩展 `TopicAnchorState` 结构 + 来源解析
- `internal/topicgraph/repository/daily_report_topic_repository.go`：抽 `ListCentroidWindowSections` 并让 `ComputeTopicCentroid` 复用
- `internal/topicgraph/repository/daily_report_manual_topic.go`：建话题事务内写 `topic_anchor_overrides`
- `internal/topicgraph/handler/daily_report_anchor_read_handler.go`：响应扩展
- 迁移：无新表（复用 `topic_anchor_overrides`）

**前端（`front/`）**
- `features/tags/components/console/LaneCorrectionCanvas.vue`：拖拽手势、落点视觉、相对归一、图例、首义标记
- `features/tags/composables/useLaneCorrection.ts`：`anchor_origin`/`centroid` 状态与派生
- `features/tags/components/console/LaneAnchorScope.vue`：首义来源节点 + 滚动窗口折叠组
- `features/tags/components/compose/LaneComposeCanvas.vue`：高度贴合容器、`.is-drop-other` 视觉
- `features/tags/composables/laneComposeLogic.ts`：抽公共手势/归一能力（不改变现有导出语义）
- `api/dailyReports.ts`：`TopicAnchorState` 类型扩展
- `features/tags/components/console/BoardComposeOverlay.vue`：接线

**API**
- `GET /api/daily-reports/topics/:id/anchor` 响应**向后兼容扩展**（新增字段，既有字段不变）

**文档**
- `docs/reference/api/daily-reports.md`（anchor 响应字段）
- `docs/reference/flow/topic-graph.md`（手动建泳道首义留痕）
- `docs/reference/database/tables/daily-report-watch.md`（`topic_anchor_overrides` 写入来源补 `manual_create`）

## 非目标

- 不改变质心公式、窗口默认值（30）、α 默认值（0.5）与任何匹配阈值。
- 不引入编辑画布的「草稿 / 应用 / 撤销」模型（归属变更仍即时生效）。
- 不回填任何历史数据（首义来源、快照、质心）。
- 不改动左栏「辅助标签」面板——那块归活跃 change `topic-lane-auxiliary-label-constraint`，本 change 只在其之后追加只读展示。
- 不做「拖拽改日期」或「拖拽排序」：修正画布的 X 轴是日期事实，不可拖动。

## 部署后影响 + 需要的操作

**(a) 用户可见行为变化**
- 编辑泳道的候选圆点大小/深浅差别会明显变大（以前肉眼几乎一致）；图例改成尺寸图例。
- 已生效的首义成员在画布上会出现标记（以前只有待提交的勾选有）。
- 候选节点可以直接拖进成员区/拖出，松手即生效（失败会在节点上标错并回滚显示）。
- 左栏「首义范围」多出两行信息：首义来源节点（或「来源不可考」）、参与滚动计算的节点数。
- 新建泳道画布会填满窗口高度，多出的空间给底部散料区——行位置不变，只是下方不再空一块。
- 编辑泳道：点节点弹出的卡片标题不再截断（长标题换行显示全文）；成员行上方那条强调色横条没了；选中节点后轻点旁边空白画布即可取消选中（节点卡收起）。

**(b) 需要用户手动执行的操作**
- 无。无需迁移、无需重新生成日报。
- 若希望某条历史手动建的泳道显示真实首义来源，用左栏「用选中的 N 个节点定首义」重新定一次即可（会留痕）。

**(c) 旧数据如何降级**
- 历史手动建的泳道：首义来源显示「创建时写入 · 来源不可考」（`resolved=false`），不阻断任何操作；滚动窗口节点照常可读。
- 算法自动建的话题：同上一行；滚动窗口节点照常可读（推算来自当前成员，不是历史快照）。
- 无数据库结构变更，回滚只需回退代码。
