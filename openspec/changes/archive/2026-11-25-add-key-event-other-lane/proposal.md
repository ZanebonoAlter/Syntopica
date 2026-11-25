# Proposal: 关键事件时间视野补「其他」兜底泳道

<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

版块态势页「关键事件」视图（`?view=events`）的行集合来自后端 `lane-dynamics`，口径是 **status=active ∪ 被关注（watch）的话题，且窗口内有日报分区**。其结果是：**待观察（candidate）且未关注的话题、已归档话题、日报重建后分区消失的话题，以及 `persistent_topic_id` 已置空的历史注记**——这些话题的 confirmed 关键事件虽然随 `GET /semantic-boards/:id/key-events` 一起返回，却在视图层被静默丢弃（`KeyEventTimelineView.vue` 的 `rowTopicIds` 过滤）。

用户实看（版块「开发者工具与开源生态」，14 天窗）：11-18 的两条 OpenAI GPT-6.2 / 腾讯混元事件、11-24 的 GPT-6.2 Sol 事件都确认过，时间轴上一条都没有——因为这三个话题是未关注的 candidate。右栏「当日其他话题」能看到它们的线索，时间轴却看不到它们的关键事件，两处读数自相矛盾。

## What Changes

1. **新增「其他」兜底行（唯一一行）**：窗口内所有无法归属到任一持久化话题行的事件收进这一行——未关注的 candidate、archived、话题已删除（`topicId=null`）、窗口内分区消失的话题。candidate **仍不单独占行**，行序与矩阵口径不变。
2. **位置固定在最末**：正式泳道 → 休眠泳道组 → 「其他」；**独立常显、不参与休眠折叠**；无事件时整行不渲染（不造空行）。
3. **读数口径统一**：视图头「MM/DD · N 条」、行头「N 事件 · 最近 MM/DD」、当日卡片列、整窗空态判断全部把「其他」行算进去，MUST NOT 出现「行里有节点、头部读 0 条」的自相矛盾。
4. **事件可读且有话题上下文**：「其他」行混装多话题，事件卡沿用既有的「所属话题」快照名（`topic_label_snapshot`）；标签仍显示 `title_snapshot`。
5. **补齐 spec**：`workshop-board-console` 现写的「候选话题 MUST NOT 占行，其确认事件 MUST NOT 在本视图呈现」改为「不单独占行，但 MUST 收进『其他』行」，对应 Scenario 同步改写并新增兜底行场景。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `workshop-board-console`：关键事件视图的「行集合」契约从「只含持久化话题、丢弃其余事件」改为「持久化话题行 + 一个『其他』兜底行」，并补兜底行的位置 / 折叠 / 读数 / 空态 / 点击行为要求。

## Impact

- **前端（唯一改动面）**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`（伪行 + 读数同步）、`front/app/features/tags/composables/useBoardKeyEvents.ts`（保留 `topicId=null` 事件，新增「未归入行集合的事件」分组纯函数）、对应单测。
- **后端零改动**：`ListBoardKeyEventMarkers` 已返回全量 confirmed（不按话题状态过滤），无需接口、DTO、表结构变更。
- **用户可见**：时间视野里会多出「其他」这一行（仅当有这类事件），历史上「确认了却看不到」的事件开始显形。
- **不影响**：矩阵视图行序与休眠分组、右栏「当日其他话题」（那是当日线索，与本行不是同一数据面）、`lane-dynamics` 口径、事件写入口（本视图仍只读）。
