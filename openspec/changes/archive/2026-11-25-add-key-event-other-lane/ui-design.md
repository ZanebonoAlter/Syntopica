<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约：关键事件时间视野「其他」兜底泳道

## 入口与变更

- **入口不变**：版块态势页（`/tags/:boardId`）→ 主舞台视图切换「关键事件」，视图工具栏、时间视窗、右轨全部沿用现状。
- **唯一新增**：时间视野行区末尾（休眠泳道组之后）多一行「其他」；无此类事件时整行不渲染。
- **不变**：矩阵视图、「当日其他话题」右栏组、视图头控件、选中日下拉、休眠组标题与折叠行为均不改。

## 受影响状态

| 状态 | 现状 | 加「其他」行后 |
| --- | --- | --- |
| success（有兜底事件） | 这些事件不可见 | 「其他」行渲染：行头 `其他` + `N 事件 · 最近 MM/DD` + 当日条数；行内节点/标签/卡片列与正式泳道同一套渲染 |
| success（无兜底事件） | — | 不渲染该行，不造空行；其余行与当前完全一致 |
| loading（事件层加载中） | 轴照画 + 轻提示 | 不变；「其他」行随事件数据到位后出现，MUST NOT 先出空行再填充 |
| empty（窗口内 0 条 confirmed） | 说明行「这段时间没有确认的关键事件」 | 不变；「其他」不出现 |
| error（事件读取失败） | 首错块 + 重试 | 不变；MUST NOT 用「其他」行冒充读取失败 |
| stale（刷新失败保留旧内容） | stale 提示 + 旧内容 | 不变；「其他」行沿用旧内容，不冒称 0 条 |
| no-lanes（版块无持久化话题） | 「该版块暂无持久化话题」 | 若「其他」行有事件，SHALL 渲染该行并保留说明行；MUST NOT 因无正式行而吞掉兜底事件 |

## 复用组件与布局模式

- **布局模式**：沿用版块态势页既有的 `workspace` 主舞台（行头列 + 横向滚动区两列结构），**不新增布局模式、不改栏宽、不改横向滚动策略**。
- **复用**：行头（`.kev-row-head` / `.kev-row-name` / `.kev-row-meta` / `.kev-row-day`）、生命线 + 节点 + 标签、日聚焦卡片列（`KeyEventLabel` / 卡片列既有样式）、休眠组标题盒（仅作为位置参照，不参与折叠）。
- **新增样式**：仅「其他」行名与说明（可复用 `.kev-row-name` + 一个弱化副标题），不新增颜色 token、不新增基础组件。
- **交互**：行头与节点/标签沿用本视图内部选中（留在本视图）；「其他」行不写入壳层 `selectedLane`（它不是任何持久化话题），点该行头 MUST NOT 触发视图切换或死选中。事件卡沿用「所属话题」快照名（`topic_label_snapshot`）表达多话题混装。
- **视口**：桌面 1440×900 与 1920×1080 下均在现有舞台内，无新增横向溢出；窄屏沿用现有窄屏摘要，不在本 change 扩展。

## Data Provenance

- 「其他」行事件：`GET /semantic-boards/:id/key-events?days=N` 响应中 `persistent_topic_id` 不在当前行集合（含 `null`）的 `confirmed` 记录；字段 `title_snapshot` / `topic_label_snapshot` / `source_period_date` / `article_ids_snapshot`（确认时冻结）。
- 行头计数与「最近 MM/DD」：同响应派生（按 `source_period_date` 在轴内计数）。
- 行名「其他」：常量，不读任何接口。

## 验收映射

- 组件测试（真实路径）：`front/app/features/tags/components/console/KeyEventTimelineView.test.ts`（「其他」行渲染/位置/常显/读数/空态/不可写选中）、`front/app/features/tags/composables/useBoardKeyEvents.test.ts`（`topicId=null` 保留与未归入分组纯函数）。
- 实机验证：`ui-verification/report.md` 至少 1 条判定条目（版块「开发者工具与开源生态」14 天窗：11-18 / 11-24 的 candidate 事件出现在「其他」行；与右栏「当日其他话题」交叉核对）。
