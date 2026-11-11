# 设计 — deconflict-radar-warming-and-new-stories

## D1 分档互斥：`new_stories` 排除 `top_stories`

**结论**：`newStories` 的筛选条件从 `当日有证据 ∧ 无基线` 收紧为 `当日有证据 ∧ 无基线 ∧ story_id ∉ top_ids`，其中 `top_ids` 由**已截断**的 `feed.TopStories` 生成。

**落点**：`backend-go/internal/topicgraph/repository/board_radar_repository.go` 的 `finalizeRadarStories`，在 `for _, item := range items` 前先建 `topIDs` 集合。

**取舍**：
- 排除依据是截断后的 Top N（5 条），不是全部有基线者——「今日该读」实际展示的就是这 5 条，去重必须对齐**实际展示集**，否则仍会重复。
- 顺序依赖：必须先填 `feed.TopStories`，再遍历算 `warming` / `newStories`。这是既有代码的顺序，无需调整。
- `warming` 不参与互斥：升温档要求 `HasBaseline == true`，而新起档要求 `HasBaseline == false`，两者天然不相交；升温条目是否同时在「今日该读」里**允许重叠**——它们是同一个事件的两种视角（今日最热 vs 跨天变热），语义不同，重复出现不构成冗余。这一点与「新起重复该读」性质不同：后者是同一视角的重复。

## D2 新起计数两口径：`new_count`（去 Top 后）与 `new_total_count`（去 Top 前）

**结论**：`new_count` = 「当日有证据 ∧ 无基线 ∧ 不在 Top N」的事件线**完整计数**（截断前）；`new_total_count` = 「当日有证据 ∧ 无基线」的**全部**条数（去 Top 之前、截断之前）。前端新起档按三态渲染：

- `new_count > len(new_stories)`（截断发生）→ 列出条目 + 「另有 N 条今日新起事件线（未在今日该读中列出）」（N = `new_count` − 列出数）；
- `new_total_count > 0` 而 `new_count == 0`（全被该读覆盖）→ 「N 条今日新起事件线均已列入「今日该读」」（N = `new_total_count`）；
- `new_total_count == 0` → 「今天没有新起的事件线。」

没有 `new_total_count`，前端无法区分后两种状态：同一函数内 `new_count == len(去 Top 后新起)` 在截断前赋值，截断上限 3 ≥ 1 ⇒ `len(new_stories)==0 ⟺ new_count==0` 恒成立，旧摘要条件 `!newStories.length && newCount > 0` 对任何真实载荷永假（review R-1 判定的死代码）。

**落点**：`feed.NewCount = len(newStories)` 与 `feed.NewTotalCount = newTotal` 都在截断之前赋值；`newTotal` 在 `!HasBaseline` 分支里、排除 `topIDs` **之前**自增。

**取舍**：
- 现注释写 `WarmingCount / NewCount are the FULL counts (not the trimmed ones)`。本 change 后该注释仍成立，但语义从「新起全量」收窄为「**去 Top 后的**新起全量」。注释须同步改写，否则后来者按旧义理解会算错摘要数字。
- 当前真库：138 条新起 − 5 条已进 Top = **133**（`new_count`），其中列出前 3 条（5052/5074/5110），摘要行显示「另有 **130** 条」。（更正 review R-2：首版把展示态写成「一行摘要显示 133 + 0 条目」——摘要分支是死代码、实测是 3 条目且无摘要；本版按实现与实测更正为「3 条目 + 摘要行」。）
- 备选方案（保留 `new_count`=138 并在前端减 5）被否：前端减 5 会把「Top 里有几条是新起」这条本属于后端分档逻辑的规则泄漏到前端，且 Top 上限一旦调整就出错。
- 备选方案（前端用 `len(new_stories)==0 && new_count>0` 反推「全被覆盖」）被否：见上，该条件永假。

## D3 空态判定：新增 `cross_day_story_count`，由服务端判

**结论**：`stories` 段新增 `cross_day_story_count`（int）= **`HasBaseline && HasTodayEvidence` 的事件线条数**（不受 Top N 截断影响，统计全部候选）。`0` ⇒ 不存在跨天延续事件线 ⇒ 空态用「当前没有跨天延续的事件线」；`> 0` 且 `warming_count == 0` ⇒ 空态用「暂未触发升温条件」。

**落点**：`RadarStoryFeed` 新增字段；`finalizeRadarStories` 在 `items` 排序前统计 `HasBaseline` 数量。

**取舍**：
- **为什么必须加上 `HasTodayEvidence`（真库实证修正）**：只用 `HasBaseline` 时真库报 **126 条**「跨天线」，但 `board_stories` 里跨天跨度（`first_seen_date <> last_seen_date`）为 **0**。原因是 388 条事件线「锚点日之前有证据、锚点日当天已无证据」——它们是锚点日前就结束、只剩 48h 残热的旧线，并没有延续到今天。改成 `HasBaseline && HasTodayEvidence` 后真库报 **0**，与「跨天跨度 0」一致。这个错误如果只在单测里验证就会被放过（测试造的数据天然同时具备两侧证据），是「效果核对」这一步抓出来的。
- **不选**「前端用 `warming.length == 0 && new_stories.length == 0` 反推」：`new_stories` 有 3 条截断上限，且现在被去 Top 后可能为空，反推会得出错误结论。
- **不选**「前端检查每条 item 的 `has_baseline`」：条目已被 Top N 与各档上限截断，看不到全量候选，可能全部被截掉而误判为「没有跨天线」。
- 服务端判定的额外好处：口径变化只在后端一处，前端不需要知道 `has_baseline` 的语义。
- **已知口径边界（基线窗口差，review R-3 记录）**：「跨天延续」按 `HasBaseline && HasTodayEvidence` 判定，而 `HasBaseline` 只看锚点日之前 **3 日**基线窗口——一条 5 天前有证据、今天复现的线（中间断档 >3 日）`HasBaseline=false`，既不计入 `cross_day_story_count`、又落进「今日新起」。这是 3 日基线窗的必然结果，**不是缺陷**，本 change 不改窗口（改它属另一个 change）。`enable-story-cross-day-continuation` 的层二延续判据（日期窗口 ≤1 天）落地后，接上的跨天线在锚点日前 1 日必有证据 → 落在 3 日窗内 → `HasBaseline=true` 且当日有挂载证据 → 计入 `cross_day_story_count`、空态自动切到 `not-triggered`；断档 >3 天的线在该 change 下会被判成**新线**（不接续），「客观跨天线被漏计」在身份层已被消除——本边界收窄为措辞层。spec 已把「跨天延续的事件线」定义为判据本身（既有锚点日之前 3 日基线窗口内证据、又有锚点日当天证据），断档线按定义不是跨天延续线、而是今日新起，空态文案「当前没有跨天延续的事件线。」在该定义下始终为真。

## D4 该读不过滤来源数

**结论**：`top_stories` 保持现有排序（heat desc → participants desc → story_id asc），不新增 `participants >= 2` 门槛。

**落点**：无代码改动，仅 spec 显式写明，避免后来者把它当遗漏。

**取舍**：用户原话「今日该读是多篇文章报道事件」容易读成「必须 ≥2 篇」。但热度公式本身已按参与者数加权，多来源者自然靠前（当前真库 Top 1 就是 2 个来源）；硬卡 ≥2 会让当日 Top 5 只剩 1 条，右栏几乎全空。真库实测：`participants` 分布中 1 来源占绝大多数，硬门槛不可接受。

## D5 视觉：新起从嵌套子块提升为同级分档

**结论**：`今日新起` 从 `.rd-warm-block`（今天可能升温）内部移出，成为独立 `.rd-warm-block`。

**落点**：`BoardRadarReadingRail.vue` 模板结构调整，复用既有 `.rd-warm-block` / `.rd-warm-sub` / `.rd-warm-why` / `.rd-read-meta` 类与 `--color-text-muted` token，不新增色值/断点/组件。

**取舍**：这是本 change 唯一的视觉改动，属 `ui-impact: minor`。它解决的是「同一张卡里两个标题、读者分不清」的直接成因——即使数据修好了（Change C 落地后有升温条目），嵌套结构仍会让人误以为新起是升温的子集。

## D6 兼容与降级

**结论**：前端在响应缺少 `cross_day_story_count`（旧后端）时，升温空态回退为「暂未触发升温条件」，**不**断言「没有跨天延续的事件线」。

**落点**：`front/app/api/normalizers/boardRadar.ts` 归一为 `null`（区分「缺失」与「0」），`useBoardRadarOverview.ts` 透出三态，rail 组件按三态渲染。

**取舍**：`0` 与「缺失」必须能区分——把缺失归一成 `0` 会导致新前端 + 旧后端时显示「当前没有跨天延续的事件线」，那是一句无依据的断言。用 `null` / 可选字段表达「无法判定」，前端回退到旧文案（不断言成因），符合红线 30 的诚实降级纪律。`new_total_count` 同理归一为 `null`：旧后端的 `new_count` 是未去 Top 的全量，与摘要口径对不上，缺失时前端只列条目、不出「另有 N 条」摘要（不冒充去重口径）；空档仍渲染「今天没有新起的事件线。」——旧后端 `len(new_stories)==0 ⟺ new_count==0` 恒成立，该句在降级态为真。

## 风险

| 风险 | 影响 | 处置 |
| --- | --- | --- |
| 摘要数字与列表不一致（口径错位） | 读者看到「另有 133 条」但列表 0 条，或反之 | `new_count` 与 `new_stories` 必须在同一过滤条件下产生（D2）；用例断言 `new_total_count == 138 ∧ new_count == 133 ∧ len(new_stories) == 3`（截断态）与 `new_total_count == 4 ∧ new_count == 0 ∧ len == 0`（覆盖态） |
| 新起档退化为纯数字后信息量下降 | 右栏只剩 5 条事件线可看 | 用户已知情并选择此方案；若嫌少应调 `radarTopStoryLimit` 而非恢复重复 |
| 旧后端 + 新前端断言错误 | 显示无依据的「没有跨天线」 | D6 的 `null` 三态 |
| `HasBaseline` 语义被后来者改动 | 空态判定失准 | D3 在 spec 里把判定输入写死为「既有锚点日之前 3 日基线窗口内的证据、又有锚点日当天证据的事件线数」（= 判据本身，spec 实文） |
| 与 `enable-story-cross-day-continuation` 的交互 | 该 change 落地后跨天线出现，本 change 的空态自动切到「暂未触发」分支 | 两 change 改动文件不重叠（本 change 改分档逻辑与展示，那个 change 改归属判据）；D3 的判定不依赖跨天线是否存在，天然兼容 |

## 开放问题

无。三项口径均已由用户拍板（升温诚实空态 / 新起只列该读之外 / 该读不硬卡来源数）。
