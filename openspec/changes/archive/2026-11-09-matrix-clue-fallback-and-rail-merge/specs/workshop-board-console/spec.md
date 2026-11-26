# workshop-board-console Delta（matrix-clue-fallback-and-rail-merge）

## MODIFIED Requirements

### Requirement: 泳道热度口径（2026-10-27 矩阵阶段修订）

泳道热度与计数 SHALL 分三个口径并各自命名，MUST NOT 混称：**矩阵日格** = 该泳道该日的事件线数，同日多个主题桶 SHALL 合并计数而非取峰值；**矩阵格颜色** = 该泳道该日最热事件线的独立参与者数（`participant_count`），分档 SHALL 有图例；**演进节点 / 泳道趋势** = 该泳道该日的事件线数；**行序** = 基准响应全窗 `lane_heat` 降序 → 全窗事件数降序 → topic id 升序。

矩阵日格的**展示降级兼容**（matrix-clue-fallback-and-rail-merge）：格值与顶部时间视窗曲线共用同一条降级规则——该日事件线数 > 0 用事件线数；事件线数为 0（已回填版块的历史未回填日，线索仍在）而当日线索合计 > 0 时按**当日线索合计**（各桶 `thread_count ?? events+folded` 之和）兜底显示，且格子 title/aria SHALL 明示「0 条事件线，按线索合计兜底显示」，MUST NOT 静默冒充事件线数；事件线数未知（旧响应缺 `story_count`）维持现状——已知线索合计则回退线索合计（现状行为），全未知显示 `?`，不造 0；事件线数为 0 而**线索全未知**时同样归未知（`?`，无法区分真零与未回填空白），不得造 0。降级只影响格值与曲线展示，MUST NOT 参与热度、行序、休眠或分档计算。

矩阵行 MUST NOT 再渲染行尾重复总数列；行头与格子的可读文本（title/aria）SHALL 同时给出事件线数、最热事件线参与者数、未归组事件数与主题桶数，避免口径不可见。同日主题分桶 MUST NOT 冒充热度（分桶多 ≠ 更热）；`事件` 条数 MUST NOT 直接充当事件数；`section` 数 MUST NOT 参与任何热度或排序计算。

事件身份未回填（`story_id` 全为 NULL）的版块 SHALL 按「每个非综合稿事件记 1 条事件线」保守计算，并在矩阵 footnote 明示「事件身份未回填」，MUST NOT 显示 0 或空态冒充无事件。

#### Scenario: 同日分桶不冒充分区热度

- **WHEN** 某泳道同日两个主题桶各含 1 个事件线，另一泳道当日 1 个桶含 2 个事件线
- **THEN** 前者日格 SHALL 显示 2（两桶事件线数之和，非桶峰值 1），后者 SHALL 也显示 2；行序 SHALL 由 `lane_heat` 决定，SHALL NOT 由 section 数或桶峰值决定

#### Scenario: 矩阵格值是桶峰值

- **WHEN** 用户查看矩阵格内数字
- **THEN** 该数字 SHALL 为该泳道当日去重后的事件线数；事件线未知（未回填）时 SHALL 按非综合稿事件数作上界并给出降级说明，MUST NOT 用主题桶数量或事件总数冒充
- **AND** 事件线数为 0 而当日线索合计 > 0 时，格值 SHALL 按线索合计兜底显示，title/aria SHALL 明示降级（0 条事件线，按线索合计兜底），MUST NOT 显示空白冒充无事件

#### Scenario: 历史未回填日按线索合计兜底

- **WHEN** 某泳道某日事件线数为 0（事件线回填未覆盖的历史日）而该日各主题桶线索合计为 132
- **THEN** 该格 SHALL 显示 132，title/aria SHALL 标注「0 条事件线，按线索合计 132 兜底显示」
- **AND** 该兜底值 SHALL NOT 参与 `lane_heat`、行序（story 合计排序键用原始事件线数）或休眠判定

#### Scenario: 真零事件不兜底成非零

- **WHEN** 某泳道某日有分区但事件线数为 0 且线索合计也为 0（全部事件为综合稿或无事件）
- **THEN** 该格 SHALL 显示 0 并可进入当日工作面（零事件是合法结果），SHALL NOT 附降级说明
- **AND** 该格 SHALL NOT 与断档格（`—`）混淆

#### Scenario: 颜色表达参与者数

- **WHEN** 某日格内最热事件线有 6 个独立 feed 报道
- **THEN** 该格 SHALL 按 6 个参与者对应的档位渲染颜色，并在 title/aria 中给出「2 条事件线；最热：事件线标题（6 个来源）」，MUST NOT 只用颜色表达

颜色所指事件线 SHALL 由 timeline 的 `hottest_story_id` 与 `hottest_story_title` 给出，二者 SHALL 来自生成 `max_participant_count` 的同一条存活事件线；同分 SHALL 按存活 id 升序选择。title 与 aria SHALL 同时点名，不改计数、颜色与点击口径。

#### Scenario: 多线格点名获胜线
- **WHEN** 同一日格有 A（2 来源）与 B（4 来源）两条线
- **THEN** 格值 SHALL 为 2，颜色 SHALL 对应 4，title/aria SHALL 点名 B 并显示 4 个来源，hottest_story_id SHALL 为 B 的存活 id

#### Scenario: 来源同分稳定点名
- **WHEN** 两条线来源数均为 4 且存活 id 分别为 10、20
- **THEN** SHALL 选择 10 的标题与 id，且两字段 SHALL NOT 来自不同记录

#### Scenario: 无身份不伪造标题
- **WHEN** 未回填或旧响应没有最热线身份
- **THEN** SHALL 保留原格值/颜色及降级说明，显示「最热事件线暂不可识别」，SHALL NOT 用泳道或 section 标题代替

#### Scenario: 空标题与零来源
- **WHEN** 获胜线 id=10、标题为空且来源数为 0
- **THEN** SHALL 显示「最热：事件线 #10（0 个来源）」，SHALL NOT 当作断档

### Requirement: 观察窗与自由日期显示（2026-10-27 用户裁决 A 路线）

矩阵/追踪 SHALL 采用「观察窗：名单稳定」：`sourceDays` 仅由明确选择独立7/14/30/365日档按钮或 URL `days` 变更；A（该档响应）SHALL 为名单、前端原排序、休眠与候选的唯一基准。行序 SHALL 以 A 的 source 全轴计算 `clueTotal` 降序→`sectionTotal` 降序→topic ID 升序，MUST NOT 换为后端返回序或按观察轴/B 合并后的 timeline 重排。B（真实365日池）SHALL 只补 A 同 ID 泳道的历史日内容，MUST NOT 将 B 独有成员或候选提升为基准。拖选、双端调整、平移缩放、键盘逐日/逐周、精确 `YYYY-MM-DD` 输入 SHALL 仅改变这批泳道的显示区间；即使观察范围恰7/14/30/365日或等于某档自身区间，也 MUST NOT 改 sourceDays、base、名单、排序、休眠或候选，MUST NOT 写话题状态。界面 SHALL 常态标「名单/排序/休眠按 N 日档」，超档窗历史区段另标「历史内容由365日池补充（不改名单）」。观察日期含起止日、按 Asia/Shanghai 日历日；非法输入（缺项/格式错/起始晚于截止/池外日期）SHALL 给出明确文案且原视窗保持不变，MUST NOT 静默截期或补0。

顶部时间视窗图 SHALL 与矩阵日格用**同一条降级规则**（matrix-clue-fallback-and-rail-merge）：逐日值 = A 名单同 ID 泳道各日「事件线数 > 0 用事件线数，否则当日线索合计兜底」之和（非文章数、非全库热度、非矩阵桶峰值），并带日期轴、单位与刻度；曲线标注与 Y 轴单位文案 SHALL 与该口径一致（不再称「线索合计」）。该图 SHALL 在**首屏即取到 365 池**（A 数据到位且锚点已知即预取），MUST NOT 只画当前档窗那几天、拖动选区后才补历史——「按需拉池」只适用于历史日**内容**解析，不适用于常驻背景曲线。365 池未取到或加载失败时 SHALL 给出提示并提供单独重试入口（A 保持可读、名单/排序不动、历史日显示 `?` 不补 0），MUST NOT 静默只留半截曲线；365 档即池（档/池共用同一响应，无独立池可失败）时该提示不适用，切到 365 档 MUST 清掉残留的池失败/陈旧态（不得误述「历史池未取到」且重试键变死）。`days/start/end` SHALL 随 URL query 恢复（非法项忽略并提示）；切板块 SHALL 将观察范围、选中日与展开态复位。

#### Scenario: 拖选不重排

- **WHEN** 用户在 14 档下把观察范围拖到历史 10 天
- **THEN** sourceDays SHALL 仍为14，名单、source全轴行序、休眠分组与候选 SHALL 与拖选前逐项一致；B 仅补 A 同 ID 历史，界面标明14日名单基准与历史补充区段

#### Scenario: 精确档回切

- **WHEN** sourceDays=14，用户拖至恰7/30/365日、其他档自身区间或拖回14档自身区间
- **THEN** base SHALL 始终为 A14，名单/行序/休眠/候选不变，不因精确日期命中切档或把 B 提为名单；此 Scenario 名保留既有回切义务，但日期自动换基准规则已被 D11 覆盖

#### Scenario: 明确切独立档

- **WHEN** 用户明确点击7日独立档或将 URL days 改为7
- **THEN** sourceDays SHALL 改为7，A7成功后按其source全轴重算名单/前端原排序/休眠/候选并重置默认观察范围；仅观察日期变化不得触发此分支

#### Scenario: 档池分层失败

- **WHEN** A成功B失败，或A首次/刷新失败但B成功
- **THEN** 前者 SHALL 保持A可读、历史未取到不补0、不换名单或强改视窗；后者 SHALL 分别显示A错误态或staleNotice保留旧A，MUST NOT 用B冒充该档成功

#### Scenario: 首屏曲线即含历史

- **WHEN** 用户首次打开版块态势矩阵页（A 档数据到位、锚点已知，观察范围仍是当前档默认区间）
- **THEN** 系统 SHALL 已请求 365 池，顶部时间视窗图 SHALL 画出 365 池内已返回泳道的逐日降级口径值（事件线数 > 0 用事件线数，否则线索合计兜底；含档窗外的历史日），MUST NOT 等待用户拖动选区才补历史曲线

#### Scenario: 曲线与矩阵格同口径

- **WHEN** 某日各泳道事件线数为 0 而线索合计为 132（历史未回填日）
- **THEN** 该日曲线值 SHALL 为 132（与矩阵格值同一降级规则），曲线标注 SHALL 表达「无事件线日按线索合计兜底」
- **AND** 事件线数已知的日子 SHALL 用事件线数，MUST NOT 与线索合计混加

#### Scenario: 池未取到有提示可重试

- **WHEN** 365 池加载失败（或刷新失败保留旧池）
- **THEN** 界面 SHALL 给出池未取到/陈旧提示与「重试历史池」入口，A 侧名单、排序与观察范围 SHALL 保持不变，MUST NOT 因观察范围尚未跨出档窗而静默隐藏该提示

#### Scenario: 池失败后切 365 档不残留误述

- **WHEN** 365 池加载失败后，用户切到「近 1 年」档（365 档即池）
- **THEN** 池失败/陈旧提示 SHALL 消失（该档没有独立的历史池），重试入口 MUST NOT 变成无效按钮；切回其它档时池按需重新取

#### Scenario: 非法日期反馈

- **WHEN** 用户输入起始日晚于截止日，或池外日期
- **THEN** 界面 SHALL 给出对应明确文案，原观察范围与数据保持不变

#### Scenario: 深链恢复

- **WHEN** 用户带 `?days=30&start=…&end=…` 刷新
- **THEN** 档与观察范围 SHALL 恢复；任一参数非法时 SHALL 忽略该项并提示，其余合法项照常生效
