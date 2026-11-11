# 用例 — deconflict-radar-warming-and-new-stories

> 测试单元 = 本 change 的 Requirement「今日该读与升温提示」（MODIFIED）。下面主链路表把 6 个 Scenario 串成一条完整故事。
> 层选择：分档互斥与计数 = 后端 repository 单测（隔离 PostgreSQL）；归一三态 = normalizer 单测；文案与结构 = Vitest 组件层；完整交互 = agent-browser（静态 :5100）。

## 1 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 造 6 条无基线事件线，热度递减，全部当日有证据 | 今日新起与该读互斥 | `top_stories` 5 条（热度前 5）；`new_stories` 只含第 6 条；`new_count == 1`；`new_total_count == 6`（去 Top 前全量）；两者 `story_id` 无交集 | repository | `board_radar_stories_test.go:TestRadarStoryFeedNewStoriesExcludeTopStories` |
| 2 | 造 4 条无基线事件线，Top 上限覆盖全部 | 新起全被该读覆盖时说明已列入该读 | `new_total_count == 4 > 0`；`new_count == 0`；`new_stories` 为空；`top_stories` 4 条（三联组合钉住覆盖态） | repository | `...:TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered` + `TestRadarStoryFeedNewTotalCount` |
| 3 | 造 8 条无基线线（Top 5 覆盖 5 条） | 新起超出列出上限时附剩余计数摘要 | `new_total_count == 8`；`new_count == 3`；`len(new_stories) == 3`（截断前完整数不受上限影响；截断态三联 10/5/3 由 `TestRadarStoryFeedNewTotalCount` 钉住） | repository | `...:TestRadarStoryFeedNewCountNotAffectedByTrim` + `TestRadarStoryFeedNewTotalCount` |
| 4 | 造 2 条「昨日+今日都有证据」的跨天线 + 1 条「仅昨日证据」的旧线 + 1 条纯当日线 | 无跨天延续事件线时的空态 / 有跨天延续但未升温时用既有措辞 | `cross_day_story_count == 2`（昨日已结束的旧线**不得**计入）；另造全无基线场景断言 `== 0` | repository | `...:TestRadarStoryFeedCrossDayStoryCount` |
| 5 | 造混合来源数（2 来源 1 条 + 1 来源 4 条） | 该读不按来源数硬过滤 | `top_stories` 仍 5 条，SHALL NOT 因来源数 1 剔除 | repository | `...:TestRadarStoryFeedTopStoriesIgnoreParticipantFloor` |
| 6 | 响应含 `cross_day_story_count: 0` | 无跨天延续事件线时的空态 | 派生 `warmingEmptyKind == 'no-cross-day'`；rail 渲染「当前没有跨天延续的事件线。」 | composable + 组件 | `useBoardRadarOverview.test.ts` / `BoardRadarView.test.ts` |
| 7 | 响应含 `cross_day_story_count: 2` 且 `warming` 为空 | 有跨天延续但未升温时用既有措辞 | 派生 `'not-triggered'`；渲染「暂未触发升温条件。」；SHALL NOT 出现「没有跨天延续」 | composable + 组件 | 同上 |
| 8 | 响应缺 `cross_day_story_count` | 旧响应缺跨天线计数时不冒充无跨天线 | 归一为 `null`；派 `'unknown'`；渲染「暂未触发升温条件。」；SHALL NOT 断言无跨天线 | normalizer + 组件 | `boardRadar.test.ts` / `BoardRadarView.test.ts` |
| 9 | 真接口实况（真库） | 全 6 个 Scenario 的效果核对 | `top ∩ new = ∅`；`new_count` 与真库去 Top 后新起数一致；`cross_day_story_count == 0`（当前跨天线 0 条） | 人工：curl + 断言脚本 | 见 §5 效果核对 |

## 2 ⓪ 继承与调整（契约 M/R，逐行处置）

反查命令：`bash scripts/harness/test-assets.sh workshop-radar`（5 Requirements / 21 Scenarios）。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 无升温 | **期望翻转**（拆分） | `BoardRadarView.test.ts` 中含「暂未触发升温条件」的断言 | 保留原断言为「有跨天线但未升温」分支（`cross_day_story_count > 0`）；新增「无跨天线」分支断言新文案；新增「旧响应」分支断言回退 |
| 今日新起单列且不显示倍率 | **语义收窄** | `BoardRadarView.test.ts` 新起档断言、`board_radar_stories_test.go:TestRadarOverviewStoryFeedMergesBoardsAndSorts` | 旧断言「新起显示来源数与热度、不显示倍率」保留不变；**新增**互斥断言（新起 ∉ Top）——旧用例构造的数据若 Top 覆盖全部新起，需补第 6 条低热度线使其仍有新起条目 |
| 今日该读按事件线跨版块混排 | 继承不变 | `TestRadarOverviewStoryFeedMergesBoardsAndSorts` | 无动作；本 change 不改排序与混排 |
| 排序只按热度与来源数 | 继承不变 | 同上 | 无动作 |
| 今日按各版块自己的锚点日 / 锚点日取最新日报日 / 滞后版块显示自己的日期 | 继承不变 | `TestRadarOverviewStoryFeedLaggingBoardKeepsOwnAnchorDate` / `MeasuresAtBoardOwnReportDate` | 无动作 |
| 升温判定 / 分区多但事件少不升温 | 继承不变 | `board_radar_stories_test.go` 升温相关断言 | 无动作；`cross_day_story_count` 新增不影响既有判定 |
| 未回填版块不冒充无事件 / 部分版块延迟 / 单版块计算失败 | 继承不变 | `TestRadarOverviewStoryFeedBoardLevelIdentityState` / `PartialOnDegradedBoard` / `PartialOnStoryComputationFailure` | 无动作 |
| 其余 Requirement（工坊首屏 / 版块行 / 聚合端点 / 报头操作区） | 不涉及 | — | 无动作 |

## 3 展示字段盘点

本 change 改响应结构（新增字段 + 收窄语义），所有用户可见字段的来源语义如下：

| 用户可见位置 | 字段 | 来源语义 | spec 锚 |
| --- | --- | --- | --- |
| 今日该读条目 · 版块名 | `board_label` | 事件线所属版块标签 | 「今日该读」SHALL 每条带版块名 |
| 今日该读条目 · 来源数 | `participants` | 当日窗口内该事件线的参与者数（`story_signals` 派生） | 「SHALL 带上来源数」 |
| 今日该读条目 · 热度 | `heat` | 锚点日 CST 日终 `story_heat`，展示保留 1 位小数、排序不取整 | 「条目 SHALL 来自 `story_signals` 派生的 `story_heat`」 |
| 今日该读条目 · 日期 | `anchor_date` | **该版块自己的**最新日报日 | 「条目 SHALL 显式携带其所属版块的锚点日」 |
| 升温条目 · 倍率 | `ratio` | `heat / baseline_heat`，仅 `has_baseline` 时有意义 | 「SHALL 标出该事件线的倍率」「SHALL NOT 以 0/∞ 硬凑」 |
| 升温空态 · 成因 | **`cross_day_story_count`（新增）** | 既有锚点日之前 3 日基线窗口内证据、又有锚点日当天证据的事件线数（全量候选，不受截断） | 「跨天延续事件线是否存在 SHALL 由服务端以『既有锚点日之前 3 日基线窗口内的证据、又有锚点日当天证据的事件线数』判定」 |
| 新起摘要 · 计数 | **`new_count`（语义收窄）** | 当日有证据 ∧ 无基线 ∧ **未进 Top N** 的事件线完整数 | 「另有 N 条今日新起事件线（未在今日该读中列出）」（N = `new_count` 减去列出条数） |
| 新起档 · 全被覆盖说明 | **`new_total_count`（新增）** | 当日有证据 ∧ 无基线的全部事件线条数（去 Top 之前、截断之前） | 「N 条今日新起事件线均已列入「今日该读」」（N = `new_total_count`） |
| 新起条目 · meta | `participants` / `heat` / `anchor_date` | 同今日该读 | Scenario「今日新起单列且不显示倍率」（meta 沿用条目通用的来源数/热度口径，既有不变） |

无隐式契约：`cross_day_story_count` 是新字段，spec 已先补 Requirement 句再实现；`new_count` 语义收窄已在 spec 写明「完整计数」并与 design D2 对齐。

## 4 变体走查

### 4.1 输入

| 变体 | 答案 |
| --- | --- |
| 空串 / 纯空白 | 不适用——输入是结构化数值与 story 记录，无自由文本入参。**划除留痕** |
| 纯分隔符 | 不适用，同上。**划除留痕** |
| 单 token | 覆盖：仅 1 条事件线时 `top_stories` 1 条、新起 0 条、`cross_day_story_count` 按其 `has_baseline` 取 0/1 |
| 大小写 | 不适用（无文本比较）。**划除留痕** |
| 特殊字符 | 标题含 emoji / 全角标点时原样展示，不参与分档判定（分档只看数值与 id）。组件层已有超长标题用例 |
| 超长 | 超长标题在摘要行与条目中均走既有 `.rd-read-title` 换行规则；390×844 断言不溢出（§5） |

### 4.2 前置

| 变体 | 答案 |
| --- | --- |
| 空集（无任何事件线） | `top_stories` 空、`new_stories` 空、`new_count == 0`、`new_total_count == 0`、`cross_day_story_count == 0`；rail 三档各自空态；新起档渲染「今天没有新起的事件线。」（不静默消失） |
| 单元素 | 见 4.1「单 token」 |
| 重复（同一 story_id 在多板出现） | `story_id` 是全局主键，跨版块同一 id 只应出现一次（上游 `board_stories` 归属唯一）。互斥集合按 `story_id` 判定，重复 id 不会造成一档内重复 |
| 越界引用（`new_stories` 指向不存在的事件线） | 分档在内存中对同一 `items` 切片操作，不存在跨查询引用；无此变体。**划除留痕** |
| 部分满足（部分新起被 Top 覆盖） | **核心变体**：Top 5 覆盖 5 条新起、另有 3 条未覆盖 → `new_total_count == 8`、`new_count == 3`、`new_stories` 3 条（无摘要）；另有 6 条未覆盖 → `new_count == 6`、`new_stories` 3 条 + 摘要「另有 3 条」；全部被覆盖 → `new_total_count > 0` 而 `new_count == 0` → 「N 条均已列入今日该读」。三种组合后端均可产生（旧伪造组合 `len == 0 ∧ new_count > 0` 不可达，已删） |

### 4.3 时间窗口

| 变体 | 答案 |
| --- | --- |
| 边界两端（当天算不算基线） | 既有口径不变：基线取锚点日**之前** 3 天（锚点日不计入自身基线）。本 change 不改窗口 |
| 空窗口（近 3 日无证据） | `has_baseline == false` → 该线进新起候选；若全部候选如此则 `cross_day_story_count == 0` → 升温空态用「没有跨天延续」 |
| 跨窗口（候选线证据日早于 3 天窗口） | `has_baseline == false`（窗口内无证据）→ 同样落新起分支。与「从未存在过」在**展示上不可区分**，这是既有口径；本 change 不放宽也不加严 |
| 归一化（CST 日终） | 既有 `radarDayEnd` 取 CST 23:59:59，本 change 不改 |

### 4.4 幂等

| 变体 | 答案 |
| --- | --- |
| 重复执行（同一请求多次） | 纯读、无副作用；同一数据下三档与计数完全一致（排序稳定：`sort.SliceStable` + 确定性排序键） |
| 部分失败重试 | 单版块计算失败 → `partial=true` + 该板进 `degraded_board_ids`（既有行为，本 change 不改）；此时 `cross_day_story_count` 只统计成功版块，界面已有「排序不完整」提示兜底 |
| 并发 | 不声称线程安全（repository 层每次请求独立构造切片，无共享可变状态）；**不写此用例** |

### 4.5 可用性（UI）

| 变体 | 答案 |
| --- | --- |
| 误输入反馈 | 不适用——本档无用户输入。**划除留痕** |
| 空态 | 三种空态必须可区分：①无跨天线 ②有跨天线未升温 ③旧响应无法判定。组件层逐条断言文案互不相同 |
| 错误态 | 端点失败由既有错误态处理（本 change 不改）；`partial` 提示保留 |
| 加载态 | 既有加载骨架不变（本 change 不改） |
| 超长文本 | 见 4.1 |
| 重复提交 | 不适用——无提交动作，纯展示。**划除留痕** |

## 5 效果核对（真库量化）

**触发原因**：三项改动的正确性依赖真实数据的分布（Top 5 与新起的实际重叠度、跨天线是否为 0），单测只能验证逻辑，不能验证口径在真库上的表现。

**方法**：`curl -s 'http://127.0.0.1:5100/api/semantic-boards/overview'` 取 `stories` 段，用脚本断言四项。

**改前基线（实测，2026-11-07）**：`top_stories` = 5 条（5017 / 5026 / 5056 / 5015 / 5014）；`new_stories` = 3 条（5017 / 5026 / 5056）；`new ⊆ top` 为 **true**；交集 3；新起独有 **0**；`new_count` = 138；`cross_day_story_count` 字段不存在；`warming_count` = 0。

**改后期望**：
1. `top_stories` 与 `new_stories` 的 `story_id` 交集为空
2. `new_count == 133`（138 − 5，去 Top 后的完整数）、`new_total_count == 138`（去 Top 之前全量）；界面新起档 = 3 条目 + 摘要「另有 130 条」（R-1 修复后口径）
3. `cross_day_story_count == 0`（真库跨天跨度为 0；注意只数 `HasBaseline` 会错报 126/388，必须同时要求锚点日当天有证据）
4. `warming_count == 0` 且界面显示「当前没有跨天延续的事件线」

**实测记录（2026-11-07，真库）**：`top` = 5014/5015/5017/5026/5056，`new` = 5052/5074/5110，交集 **空**；`cross_day_story_count` = **0**；`new_count` = **133**；`warming_count` = **0**。**结论：通过。**R-1 修复（同日）：新增 `new_total_count` 字段（预期 138）与新起档三态渲染（3 条目 + 「另有 130 条」摘要）；归档前需 UI 复测确认摘要行实际渲染。

**结论**：见 `ui-verification/report.md` 的实测记录（归档前填写）。

## 6 白盒附加（simple 档，附加件非主角）

### 6.1 分支表

| 分支 | 条件 | 期望 |
| --- | --- | --- |
| B1 | `HasTodayEvidence == false` | 不进任何档（新起与升温都要求当日证据） |
| B2 | `HasBaseline == true ∧ Ratio >= 2 ∧ HasTodayEvidence` | 进升温档 |
| B3 | `HasBaseline == true ∧ Ratio < 2` | 不进升温档，也不进新起档 |
| B4 | `HasBaseline == false ∧ HasTodayEvidence ∧ story_id ∈ topIDs` | **不进新起档**（互斥，本 change 核心） |
| B5 | `HasBaseline == false ∧ HasTodayEvidence ∧ story_id ∉ topIDs` | 进新起档 |
| B6 | `HasBaseline == false ∧ HasTodayEvidence` 的条数 > 3 | `new_count` 为完整数（截断前），`new_total_count ≥ new_count` 同为全量，`new_stories` 截断为 3，界面附「另有 N 条」摘要 |
| B7 | 全部候选 `HasBaseline == false` | `cross_day_story_count == 0` |
| B7b | 有候选 `HasBaseline == true` 但 `HasTodayEvidence == false`（旧线只剩残热） | **不计入** `cross_day_story_count`（真库实测该情形 388 条，若计入会把「能力还没有」误报成「有跨天线未升温」） |
| B8 | 存在 `HasBaseline && HasTodayEvidence` | `cross_day_story_count == 该条数`（不受 Top 截断影响） |
| B9 | 响应缺 `cross_day_story_count` | 归一 `null` → `warmingEmptyKind == 'unknown'` → 文案回退 |
| B10 | `new_count > len(new_stories)`（截断发生，如 133 > 3） | 摘要行「另有 N − len 条…」（后端可产生；旧组合 `len == 0 ∧ new_count > 0` 不可达，伪造用例已删，review R-1） |
| B11 | `new_total_count > 0` ∧ `new_count == 0` ∧ len == 0（全被该读覆盖） | 「N 条今日新起事件线均已列入「今日该读」。」（N = new_total_count） |
| B11b | `new_total_count == 0`（当日没有任何新起） | 「今天没有新起的事件线。」（不静默消失） |
| B11c | 响应缺 `new_total_count`（旧后端） | 归一 null → 只列条目不出「另有 N 条」摘要（旧 new_count 未去 Top，口径对不上不冒充）；空档仍渲染「今天没有新起的事件线。」 |
| B12 | `partial == true` | 既有「排序不完整」提示保留；本 change 不改 |

### 6.2 边界值

| 边界 | 值 | 期望 |
| --- | --- | --- |
| Top 上限 | 恰好 5 条候选 | 全部进 Top，新起为空、`new_count == 0` |
| Top 上限 + 1 | 6 条候选 | Top 5，新起 1 条、`new_count == 1` |
| 新起截断上限 | 恰好 3 条去 Top 后新起 | `new_count == 3`、`len(new_stories) == 3` |
| 新起截断上限 + 1 | 4 条 | `new_count == 4`、`len(new_stories) == 3` |
| 升温倍率阈值 | `Ratio == 2.0` 恰好 | 进升温档（`>= 2`，含端点） |
| 升温倍率阈值下沿 | `Ratio == 1.999` | 不进升温档 |
| 基线为零 | `BaselineHeat == 0` | `HasBaseline == false` → 新起分支，SHALL NOT 显示倍率或 ∞ |
| 跨天线计数 | `0` vs 缺失 | `0` → `no-cross-day`；缺失 → `unknown`（**两者必须不同**） |
| 跨天线计数 | 仅有基线（昨日结束）vs 有基线且今天有证据 | 前者**不计入**；后者计入 |

### 6.3 不适用划除

- 输入类变体（空串 / 分隔符 / 大小写）：无自由文本入参，**划除**。
- 并发：不声称线程安全，**划除**。
- 重复提交：无提交动作，**划除**。
- 越界引用：分档在内存对同一切片操作，无跨查询引用，**划除**。
