# Design — 让重点与矩阵颜色可追溯

## 1. 既有事实与范围

上游见 `../archive/2026-11-06-add-event-story-heat/design.md` 及其 specs。事件线=board_stories；线程=daily_report_threads；来源第一版按 feed 去重。保留 `heat-v1-48h-hl24h`：story 48h 窗/24h 半衰期，lane 14d/72h，雷达阈值 3.3/0.7 不变。

实读代码发现：`daily_report_orchestrator.go` 的 GenerateHighlights 目前与线程生成并行；`daily_report_watch.go` 在 SaveReport 后才 SpawnAttributeStoriesForReport，归属与去重异步执行。仅加 prompt 字段读不到本期身份。本设计的时序前移已获主线程批准；不改变归属判定算法、watch 匹配规则或数据库身份模型。

## 2. 矩阵：点名颜色真正对应的那一条

`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go` 的 LaneTimelineDay 增加：

| JSON 字段 | Go / TS 类型 | 含义 |
|---|---|---|
| hottest_story_id | *uint / number 或 null（旧响应可缺省） | 当日日格在已有聚合集合内，来源数最多的存活事件线 |
| hottest_story_title | string / string（旧响应可缺省） | 同一获胜事件线标题；无标题为空串 |

不增加表列。沿用该格现有 `max_participant_count` 的统计范围（当前使用事件线聚合 participant_count），不在本 change 将颜色重定义为 48h 热度或另一个来源窗口。「当天」限定格内候选事件线集合，不暗示来源数另按当天截窗。查询标题与 id 时必须与产生 max 的同一行绑定，禁止分别 MAX(id)/MAX(title)。同分按存活 story_id 升序，不借 title 字典序破同分；先解析 merged_into 再去重，标题取存活记录。

- 正常：`2 条事件线；最热：美伊博弈升级（4 个来源）；…`，title 和 aria 同源，并保留未归组数、主题桶数等既有可读项。
- 来源数为 0 但有真实事件线：仍按 id 选一条，显示其标题与「0 个来源」，不把它当断档。
- 无事件线/未回填 fallback：id=null、title=""，沿用既有格值、颜色、未回填说明，追加「最热事件线暂不可识别」，不拿 section/话题标题冒充。
- 有 id 但标题空白：显示「最热：事件线 #ID（N 个来源）」；旧后端缺字段同无可识别身份降级。
- 图例固定为「数字 = 当日事件线数；颜色 = 当天被报道最多的那条事件线的来源数」。不改格子点击行为。

## 3. 时序变更与降级

```text
生成各聚类 threads（C×K 保持并行）/分桶/物化/embedding
  → SaveReport（sections/threads 落库，highlights=[]、临时兜底标题）
  → 归属（有界，尽力，失败不阻断）
  → 跨期去重 display_tier
  → 热度候选
  → highlights（带 story_id，或标签回退）
  → title/summary（与 highlights 同一短事务提交）
  → 返回生成结果/既有终态通知
```

- 归属单份报告总预算 **60s**，超时或任一归属错误即停止本轮同步归属并走标签回退。不能只在外层 race timeout 后留下继续写库的 goroutine；ctx 必须传到 DB、embedding 与分类器，循环每次检查 ctx，取消后不得再提交归属写入。当前 subject embedder 自建 background ctx，实施时须让此在线路径继承预算。backfill 的逐条继续策略不在此次修改范围，在线同步适配器单独反馈失败状态。
- 归属与 highlights 的 LLM 各自独立，**不得包在数据库事务中**；数据库事务只包对应写入。归属部分成功的数据保留，不为生成重点回滚已写 signals。
- 归属失败/超时、去重失败、候选查询失败、当日没有可用事件线（全 composite/未归属）、或没有可定位正文候选：本轮 **只调用现有标签聚类 highlights 路径**，不捏造 story_id；先选择模式再调用，禁止先调用故事模式再常规补一次标签模式。合法空响应与同参重试仍按原健壮性契约执行。
- 回退不是保证任何输入都非空：有事实材料且 fake LLM 合法返回时应生成非空重点；无事实或两次 LLM 失败仍允许 `[]`，优先遵守事实锚⑪及现有健壮性要求。标题总有既有 `日报 YYYY-MM-DD` 兜底，summary 可为空，禁止为了「有重点」拼造内容。
- 移除当前生成入口的重复 detached 归属派发，归属只做一次；watch 命中、结算与推荐仍保持既有独立失败边界，不纳入 highlights 成功条件。生成终态在重点最终写入后通知，不新增异步刷新重点/title 的第二轮任务。中间读取仍可见兜底标题，不宣称跨外部调用的整份原子可见。
- 重跑幂等：沿用同板同日生成闸与 SaveReport 覆盖语义；先清空旧 highlights/标题摘要缓存，按本轮线程重新归属、去重、筛选，不累加旧候选/重点、不带回上一轮线程 story_id。事件线身份/人工保护与 signals upsert 保持上游幂等。最终写只更新本轮保存的 report（带 SaveReport 返回的 updated_at 修订条件），不允许迟到结果覆盖更新一轮报告；零行更新记告警而非覆盖。相同证据、截止时刻、模型响应下重复两次结果一致，不承诺 LLM 自由文本逐字确定。

### 为什么不选异步刷新

异步归属后再补重点会使首次打开无重点/标题先兜底后变化，影响已读链接，还可能多做一次 highlights 调用。选有界同步方案，代价是夜间生成时延上升，收益是生成完成时选题、正文身份与报告标题一致。实施须记录归属耗时/超时，确认失败短路及 60s 边界，而不是扩大整条流水线超时或改定时窗口。

## 4. 热度候选与 LLM schema

### 4.1 候选集合及公式

只取本版块、本 report 已归属的 single 事件线，按 merged_into 一跳解析存活 id 去重。先复用既有跨期去重，再要求至少有一条详情可返回的 body thread，以保证重点有正文落点。此条件仅决定重点展示资格，**不得传给热度函数**；flash/hidden 仍贡献 signals/热度。人工指派的 single/body 线程可成为候选，但不改其归属；人工拆分 story_id=null 不造线。

热度锚点 `t` 固定为该 period_date 的北京时间 23:59:59（与现有 storyHeatAsOf 日锚一致），读取 signals 时只取 `[t-48h,t]`，未来信源时间不计。每个 participant_key 取窗口内最后 observed_at：

```text
H(s,t) = Σp 0.5 ^ ((t - max(observed_at_p))/24h)
P(s,t) = 窗内 participant_key 去重数
排序 = H 降序（原始精度）→ P 降序 → 存活 story_id 升序
候选 = 前 min(8, 可用事件线数) 条
```

复用 `story_heat.go`/仓储既有可见证据过滤和 feed 粒度，不使用 board_stories 全历史 participant_count 代替 P，不用 lane_heat/文章数/section 数近似 H。不加来源数门槛：热度 0 但有正文仍可入选，并真实显示 0；排序前不得 round。展示 heat 保留一位小数。

每条候选附 `{story_id, story_title, participant_count, story_heat, tag_ids, facts}`；facts 为本期对应线程合法 tag 的 label/description/代表文章，不以历史故事标题、线程摘要或热度数字代替事实锚。候选 facts 不足时模型可不选。最多选 3 个不同故事，正常期望 2–3 个，只有 1 个候选可输出 1 个，空集不调故事模式。

### 4.2 LLM 与持久化边界

故事模式输出 schema（必填字段，无额外字段）：

```json
{"highlights":[{"story_id":123,"title":"事实标题","reason":"基于候选事实的理由","tag_ids":[456]}]}
```

`story_id` 为正整数，必须属于当次 top 8 白名单且同版块；tag_ids 仅保留该 story 本期事实白名单内 id，剔除后为空的条目拒收。重复 story 保留首条，最多 3 条；title/reason trim 后必须非空。集外 id/空 tag/类型错误属非法语义响应，沿用同参重试一次；合法 `[]` 不重试。LLM 不输出来源数、heat 或进展布尔，服务端补齐可信字段。

落库 highlights 继续为 JSON 数组，每项新增可空 `story_id`，以及故事模式的 `participant_count`、`story_heat`、`heat_rule_version`、`heat_as_of`、`heat_incomplete` 快照。存量/标签回退这些字段缺省，不填 0 冒充测得 0。JSON 无新列/FK，不迁移旧报告。读取将 story_id 解析为同版块存活 id（不回写历史 JSON），无法解析置空；快照数字保持生成时值，不在阅读旧报时用今日热度替换。模型原始 tag_ids 与客户端合法类型校验继续保留。

最终 title=第一条有效 highlight.title，summary=其 reason；无条目用既有兜底，前端仍取前三条，不另造重要度等级或首条加权规则。promptVersion 从当前 6.0 升级未使用版本，保留 `daily_report.highlights` operation/日志审计与既有 SessionID。

## 5. 「进展」是昨日重点延续，不是事件阶段推断

详情读取按同 board、`period_date - 1` **北京时间日历日**精确查已完成报告，取 highlights 中非空合法 story_id 的集合；不能用最近一份日报或无日期校验的 prev_report_id 代替昨日。昨日无报告、无重点、null 数组或仅旧 tag_ids，则集合为空。

今天与昨日两边 id 均经 merged_into 一跳归一到存活线再比较（A→B，昨日 A/今日 B 算同线）；破损引用、跨 board、循环或多跳异常保守不标，不补做合并，不改人工线程字段。详情返回每条派生 `is_progress: boolean`，不由 LLM 决定、不写回线程 `is_development`。读取时计算可反映归并与昨日重建；本文将这一点明确为「按当前身份读取时比较」，不是永久快照。徽标 title/aria 解释「昨日重点再次入选」，不宣称一定有新的事实进展。

## 6. 点击定位与降级

在 `DailyReportTopicSection.vue` 两条正文渲染分支的 thread 容器加稳定 `id="report-thread-{thread.id}"` 和存活 story 标识。阅读容器按**当前正文实际 DOM 顺序**找第一条对应线 thread（不是最小 thread_id，也不是话题/section 标题），范围限当前 report；必要时先展开该正文 group/section、nextTick 后定位线程。前后端 thread DTO 明确传 `story_id`（当前前端类型需补），解析存活 id 但不改库内人工归属。

刊头使用键盘可达的文字按钮发出定位意图给阅读容器，不导航新页、不打开弹层。scrollIntoView + focus（thread tabindex=-1，不抢初始焦点）+ **2s** 单目标高亮；重复点击清理上次高亮/计时器，切 report/卸载清理。prefers-reduced-motion 关闭平滑滚动，颜色之外保留焦点轮廓。选择文案不误触跳转，保留现有批注 selection guard。

- 旧 highlights 无 story_id：标题理由仍显示，不渲染假按钮，不猜 tag→story。
- story_id 有效但因数据变更/过滤/错误找不到正文：不跳 section 近似位置、不触发重建、不请求不存在详情页；保留重点，按钮禁用并显示「本期正文暂无对应事件」。若点击时目标才消失，留在当前位置并以现有轻提示反馈同文案。
- 热度/来源快照缺失则省略数字，非缺失 0 显示「0 个来源」。正常重点优先显示「N 个来源」，title/aria 附「48h 热度 X.X，截止…」与不完整信号提示（复用热度查询 incomplete 标记）；不新增永久次要说明区。

## 7. 与业务红线兼容

| 约束 | 保持方式 |
|---|---|
| ⑪ 重点事实锚 | story 标题/热度仅帮助选题；写文案必须有候选标签事实；禁止编造数字、情绪、因果；不足允许 [] |
| ㊷ composite 不挂线 | 不改 scope 推导；故事候选排除 composite，标签回退 story_id 为空；未归属线程照常进正文 |
| ㊸ 人工不可覆盖 | 复用归属人工跳过；候选/徽标/存活 id 只读解析，不改人工 story_id，人工拆分不自动重挂 |
| ㊹ display_tier 仅展示 | 只筛可定位候选/正文，身份/signals/热度不读 display_tier，不删 flash/hidden |
| ㊺ 跨期去重 | 保持 7 天窗口、0.6、新 feed/is_development/body 判据；先归属再去重；昨日重点徽标独立，绝不反向恢复 flash |

## 8. 风险与实施前检查

- 有界同步需要打通当前 background embedding 与无 ctx 的 DB seam；实施先以 fake 时钟/阻塞依赖验证 60s 取消和不迟到写，再接真实调用。只针对在线入口，不重写归属算法。
- 保留生成闸和现有 watch 独立执行边界；若实施发现预算取消无法落实或 watch 时序硬依赖异步归属，应停止并上报，不能静默退回异步刷新。
- 独立复杂逻辑可丢弃原型验证列入 tasks（只在 change 下临时 mock，不碰产品实现/真库），本次制品阶段不写原型代码。
- 本设计不回填历史重点，不增事件详情页，不重标雷达，不改分桶/归属规则；后续文档影响列在 tasks，本阶段不编辑 docs/reference。

## 9. 后续路线（不在本 change 范围，2026-11-06 与用户对齐）

事件线的目标是**逐步成为读者关注的展示单位**，线索（thread）退回证据层。分四阶段，本 change 只做阶段 1：

| 阶段 | 内容 | 变化程度 | 状态 |
|---|---|---|---|
| **1（本 change）** | 矩阵格子点名最热事件线；日报「今日重点」吃热度、带 `story_id`、标「进展」、显示来源数、可点击定位 | minor | 进行中 |
| 2 | 话题页 / 看板加「近期事件线」列表（按热度排序），线索仍按天展示 | minor~medium | 待立项 |
| 3 | 事件线详情：时间轴（每天线索）+ 来源数 + 热度曲线 | major（需原型审批） | 待立项 |
| 4 | 读者默认按事件线浏览，线索折叠为「展开看细节」 | major（需用户反馈验证） | 远期 |

**分层职责（不得混淆）**：
- **话题** `board_persistent_topics`：长期主题，承载泳道与订阅（跨月）
- **事件线** `board_stories`：跨天同一事件，承载「读者跟踪的单位」（跨天）
- **线索** `daily_report_threads`：当天某一角度的叙述，承载正文文字与证据（当日）

**关键约束**：事件线**不得替代正文段落**。一条事件线可跨多天，正文仍按天写；若把多天叙述揉成一段，产出就不再是日报而是综述。阶段 4 的「替代」指**浏览单位**上移，不是把线索删除。
