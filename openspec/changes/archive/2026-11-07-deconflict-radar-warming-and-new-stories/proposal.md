<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->
<!-- doc-impact: api -->

# 拆开「今日该读」「今天可能升温」「今日新起」三档的口径

## Why

用户看真实界面后提问：「今日该读和今天可能升温怎么一样的，我的理解是：今日该读是多篇文章报道事件，今日可能升温是跨天事件延续」。

这个理解**是对的**，实现也确实是按这个意图写的，但落到当前数据上产生了两处**结构性**问题（真库实测，非推测）：

1. **「今日新起」纯冗余**：`GET /api/semantic-boards/overview` 的 `stories` 段实测 `top_stories` = 5 条（story 5017 / 5026 / 5056 / 5015 / 5014），`new_stories` = **3 条（5017 / 5026 / 5056）**；`new ⊆ top` 为 **true**，交集 3、新起独有 **0**。根因是两档吃的是同一个 `items` 数组（同一份热度排序），`TopStories` 取前 5、`newStories` 取全部无基线者，前者恒为后者的前缀。视觉上「今日新起」又嵌在「今天可能升温」卡片内部（`.rd-warm-block` 内嵌 `.rd-warm-sub`），于是读者看到的就是同一张卡里的同一批条目。
2. **「今天可能升温」结构性恒空，且空态不诚实**：该档要求事件线有跨天基线才有倍率，但当前 910 条活跃 `board_stories` **全部 `first_seen_date = last_seen_date`**（跨天事件线 0 条，由 change `enable-story-cross-day-continuation` 单独修复），因此 100% 的候选都落进 `!HasBaseline` 的「新起」分支。界面此时显示「暂未触发升温条件。」——这句话**暗示条件没达到**，真实原因是「这类数据还不存在」。降级必须诚实（红线 30 同一纪律）。

三档本应各自回答不同问题：**今日该读**＝今天哪些事件被报道得最多（不论是否跨天）；**今天可能升温**＝跨天延续的事件线里哪条今天突然变热；**今日新起**＝今天才出现、没有历史可比的。当前实现让三者混成一件事。

## What Changes

- **三档互斥**：`new_stories` SHALL NOT 与 `top_stories` 重叠；全部被 `top_stories` 覆盖时 SHALL 只给一行摘要计数 + 说明，SHALL NOT 列重复条目。
- **升温空态分两种且文案不同**：不存在跨天延续事件线时 SHALL 明示「当前没有跨天延续的事件线」；存在跨天线但今天都没到 2 倍时才 SHALL 说「暂未触发升温条件」。SHALL NOT 用后者掩盖前者。
- **「今日该读」不按来源数过滤**：保持按热度排序（热度公式本身含来源权重），SHALL NOT 新增 `participants ≥ 2` 硬门槛。
- 新增 `stories.cross_day_story_count`（既有锚点日之前 3 日基线窗口内证据、又有锚点日当天证据的事件线数），作为空态二选一的判定输入；新增 `stories.new_total_count`（去 Top 之前的当日新起全量，区分「没有新起」与「新起全被该读覆盖」）；`new_count` 口径改为「未进 `top_stories` 的新起事件线完整数」。
- 视觉上把「今日新起」从「今天可能升温」卡片内部提出，成为同级独立分档。

## Impact

- 受影响能力：`workshop-radar`（MODIFIED「今日该读与升温提示」Requirement 及其 3 个 Scenario）。
- 受影响代码：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（分档互斥 + 新字段）、`front/app/features/tags/components/console/BoardRadarReadingRail.vue`（两档文案与结构）、`front/app/api/normalizers/boardRadar.ts`、`front/app/features/tags/composables/useBoardRadarOverview.ts`。
- 不受影响：热度公式与窗口（`heat-v1-48h-hl24h`）、排序键、`top_stories` 上限 5、事件线身份/归属/去重算法、`story_heat` 计算。
- API 契约：`stories` 段新增 2 个字段（`cross_day_story_count`、`new_total_count`），`new_count` 语义收窄（口径见 design D2）。

## 部署后影响与需要的操作

**用户可见行为变化**（部署后立即生效，无需任何操作）：

1. 「今日新起」不再重复「今日该读」的条目。当前真库数据下它会**列出 3 条**非 Top 新起条目（5052 / 5074 / 5110），并附一行摘要**「另有 130 条今日新起事件线（未在今日该读中列出）」**（138 条新起 − 5 条进 Top 5 = 133 条去 Top 后完整数，再 − 3 条列出 = 130）。右栏实际展示的条目数是 5 条（该读）+ 3 条（新起条目）+ 0 条（升温）——**没有重复**，且其余 130 条不再完全不可见（首版本节把展示态写成「一行摘要 + 0 条目」，与实测矛盾，review R-2 已更正；全被该读覆盖时则显示「N 条今日新起事件线均已列入「今日该读」」，不说「今天没有新起」）。
2. 「今天可能升温」的空态文案变为**「当前没有跨天延续的事件线」**（+ 一句「跨天延续能力尚在建设中」）。这会**明确暴露**一个当前的能力缺口——这是有意为之：原本文案在骗人。
3. 「今日新起」从「今天可能升温」卡片内**提出来**，成为右栏同级分档，不再是嵌套卡片。

**需要的操作**：无。无数据迁移、无 DDL、无需重跑回填、无需清缓存（接口字段只增不改）。

**旧数据/旧响应如何降级**：
- 旧前端 + 新后端：多出 `cross_day_story_count` / `new_total_count` 字段被忽略，`new_count` 变小只影响它自己的显示逻辑，无破坏。
- 新前端 + 旧后端（响应无 `cross_day_story_count` / `new_total_count`）：升温空态按「无法判定」处理——回退到旧的单一文案「暂未触发升温条件」，`new_stories` 照旧渲染（可能仍与 top 重叠；也不出「另有 N 条」摘要——旧 `new_count` 未去 Top，口径对不上，不冒充）。这是保守降级，不会显示错误的「没有跨天线」断言。
- 历史日报与事件线数据**一律不改写**。本 change 不动任何持久化状态。
