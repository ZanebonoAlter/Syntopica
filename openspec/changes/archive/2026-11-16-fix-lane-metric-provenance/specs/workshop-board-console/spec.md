# workshop-board-console — 矩阵/观察窗数字口径收敛 delta

## REMOVED Requirements

### Requirement: 泳道热度口径（2026-10-27 矩阵阶段修订）
**Reason**: 该 requirement 的主体口径建立在**事件线（story）**身份之上——「矩阵日格 = 该日事件线数」「格色深 = 最热事件线的独立参与者数（`participant_count`）」「最热事件线标题点名（`hottest_story_id` / `hottest_story_title`）」「事件身份未回填（`story_id` 全 NULL）footnote」——而事件线身份与量纲已随 `retire-story-adopt-event-metrics`（2026-11-11 归档）整体退役：后端 `lane-dynamics` 响应已不再下发 `story_count` / `participant_count` / `hottest_story_*`，前端亦无对应消费点（`grep participant_count|hottest_story|story_count` 在 `lane_snapshot_repository.go` 与 `front/app/features/tags` 下均 0 命中）。该 requirement 的多个 Scenario（「矩阵格值是桶峰值」「历史未回填日按线索合计兜底」「真零事件不兜底成非零」「颜色表达参与者数」「多线格点名获胜线」「来源同分稳定点名」「无身份不伪造标题」「空标题与零来源」）描述的是已不存在的字段与量纲，继续保留会误导实现。
**Migration**: 现行口径由 ADDED 的「泳道热度口径（线索数单一口径）」承接：矩阵日格 = 当日**非综合稿线索数合计**；格色深 = 行内相对强度的连续刻度（不再是事件线参与者数分档）；行序 = `lane_heat` → 全窗线索合计 → topic id。已落库的 `daily_report_threads.scope`（综合稿判据）与 `daily_report_sections.topic_match_distance` 不回填、不迁移，读路径兼容不变；历史未回填日的降级兜底逻辑随事件线退役一并消失（现口径下「线索合计」本身就是唯一口径，无需降级）。

## MODIFIED Requirements

### Requirement: 观察窗与自由日期显示（2026-10-27 用户裁决 A 路线）

矩阵/追踪 SHALL 采用「观察窗：名单稳定」：`sourceDays` 仅由明确选择独立7/14/30/365日档按钮或 URL `days` 变更；A（该档响应）SHALL 为名单、前端原排序、休眠与候选的唯一基准。行序 SHALL 以 A 的 source 全轴计算 `lane_heat` 降序 → 全窗线索合计（`clueTotal`）降序 → topic ID 升序，MUST NOT 换为后端返回序或按观察轴/B 合并后的 timeline 重排。B（真实365日池）SHALL 只补 A 同 ID 泳道的历史日内容，MUST NOT 将 B 独有成员或候选提升为基准。拖选、双端调整、平移缩放、键盘逐日/逐周、精确 `YYYY-MM-DD` 输入 SHALL 仅改变这批泳道的显示区间；即使观察范围恰7/14/30/365日或等于某档自身区间，也 MUST NOT 改 sourceDays、base、名单、排序、休眠或候选，MUST NOT 写话题状态。界面 SHALL 常态标「名单/排序/休眠按 N 日档」，超档窗历史区段另标「历史内容由365日池补充（不改名单）」。观察日期含起止日、按 Asia/Shanghai 日历日；非法输入（缺项/格式错/起始晚于截止/池外日期）SHALL 给出明确文案且原视窗保持不变，MUST NOT 静默截期或补0。

顶部时间视窗图 SHALL 与矩阵日格用**同一条口径**：逐日值 = A 名单同 ID 泳道各日**非综合稿线索数合计**之和（非文章数、非全库热度、非矩阵桶峰值），并带日期轴、单位与刻度。事件线（story）量纲已随 `retire-story-adopt-event-metrics` 退役，曲线 SHALL NOT 再使用「事件线数 > 0 用事件线数，否则线索合计兜底」的降级规则，曲线单位文案 SHALL 直接表达「线索数（非文章数）」，MUST NOT 出现已退役量纲的字样。该图 SHALL 在**首屏即取到 365 池**（A 数据到位且锚点已知即预取），MUST NOT 只画当前档窗那几天、拖动选区后才补历史——「按需拉池」只适用于历史日**内容**解析，不适用于常驻背景曲线。365 池未取到或加载失败时 SHALL 给出提示并提供单独重试入口（A 保持可读、名单/排序不动、历史日显示 `?` 不补 0），MUST NOT 静默只留半截曲线；365 档即池（档/池共用同一响应，无独立池可失败）时该提示不适用，切到 365 档 MUST 清掉残留的池失败/陈旧态（不得误述「历史池未取到」且重试键变死）。`days/start/end` SHALL 随 URL query 恢复（非法项忽略并提示）；切板块 SHALL 将观察范围、选中日与展开态复位。

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
- **THEN** 系统 SHALL 已请求 365 池，顶部时间视窗图 SHALL 画出 365 池内已返回泳道的逐日线索数合计（含档窗外的历史日），MUST NOT 等待用户拖动选区才补历史曲线

#### Scenario: 曲线与矩阵格同口径

- **WHEN** 某日 A 名单各泳道的非综合稿线索合计之和为 132
- **THEN** 该日曲线值 SHALL 为 132（与矩阵格值同一口径），曲线单位文案 SHALL 表达「线索数（非文章数）」
- **AND** 曲线值 SHALL NOT 与含综合稿的 thread 总数混加，SHALL NOT 出现「事件线」「兜底」等已退役量纲字样

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

## ADDED Requirements

### Requirement: 泳道热度口径（线索数单一口径）

泳道热度与计数 SHALL 分三个口径并各自命名，MUST NOT 混称：**矩阵日格** = 该泳道该日各主题桶**非综合稿线索数之和**（当日线索合计）；**矩阵格色深** = 该格值在同行内的相对强度（行内峰值 = 1 的连续刻度，不是离散分档）；**演进节点 / 泳道趋势** = 该泳道该日各主题桶线索数之和；**行序** = 基准响应全窗 `lane_heat` 降序 → 全窗线索合计降序 → topic id 升序。

事件线（story）身份与量纲已随 `retire-story-adopt-event-metrics` 退役：矩阵日格、演进节点、顶部时间视窗曲线、格色深、行序 SHALL NOT 再引入事件线数、事件线参与者数、事件线来源数作为计数或分档依据；旧响应缺当日线索合计时 SHALL 回退到**非综合稿**线索数合计，全未知显示 `?`，MUST NOT 造 0，MUST NOT 用含综合稿的 thread 总数冒充线索数。

「综合稿」判据 SHALL 为：日报的一个 thread（线索条目）其 `tag_ids` 中 `category='event'` 的标签数 ≥ 2。综合稿 MAY 作为「提及」出现在展示层，但 SHALL NOT 参与线索计数、热度、行序与色深计算。

矩阵行 MUST NOT 再渲染行尾重复总数列；行头与格子的可读文本（title/aria）SHALL 给出当日线索合计、主题桶数与各桶的非综合稿线索数，避免口径不可见。同日主题分桶 MUST NOT 冒充热度（分桶多 ≠ 更热）；`section` 数 MUST NOT 参与任何热度或排序计算。

#### Scenario: 同日分桶不冒充分区热度

- **WHEN** 某泳道同日两个主题桶各含 1 条非综合稿线索，另一泳道当日 1 个桶含 2 条
- **THEN** 前者日格 SHALL 显示 2（两桶线索数之和，非桶峰值 1），后者 SHALL 也显示 2；行序 SHALL 由 `lane_heat` 决定，SHALL NOT 由 section 数或桶峰值决定

#### Scenario: 矩阵格值是当日线索合计

- **WHEN** 用户查看矩阵格内数字
- **THEN** 该数字 SHALL 为该泳道当日的非综合稿线索合计；当日线索合计未知（响应缺该字段且各桶线索均未知）时 SHALL 显示 `?` 并标注口径未知，MUST NOT 用主题桶数量、thread 总数或空白冒充
- **AND** 当日线索合计为 0（该日全部线索均为综合稿或该日无分区）时 SHALL 显示 0 并可进入当日工作面（零线索是合法结果），SHALL NOT 与断档格（`—`）混淆

#### Scenario: 综合稿不计入线索数

- **GIVEN** 某泳道某日 1 个主题桶含 5 条线索，其中 2 条为综合稿（thread 的 event 标签数 ≥ 2）
- **THEN** 该日格 SHALL 显示 3（非综合稿线索数之和）
- **AND** 该 2 条综合稿 SHALL NOT 影响 `lane_heat`、行序、休眠判定与色深分档

#### Scenario: 前端回退不冒充线索数

- **WHEN** 响应缺少当日线索合计字段（旧后端），但各主题桶的线索可数
- **THEN** 格值 SHALL 回退为**非综合稿**线索数合计，MUST NOT 回退为含综合稿的 thread 总数
- **AND** 各桶线索均不可数时 SHALL 显示 `?`，MUST NOT 造 0

#### Scenario: 色深表达行内相对强度

- **WHEN** 某泳道行内有多个非零日格
- **THEN** 各格色深 SHALL 按该行内格值相对行内峰值（= 1）线性插值表达，MUST NOT 用事件线参与者数等已退役量纲分档
- **AND** 色深 SHALL NOT 单独承载信息，title/aria SHALL 同时给出当日线索合计与主题桶数

### Requirement: 矩阵格伴生数字与常驻口径说明

矩阵格 SHALL NOT 把与该格值口径不同的数字（尤其含综合稿的 thread 总数）作为「辅助说明」露出：格子的 title/aria 中出现的线索条数 SHALL 与格值同口径（非综合稿线索数）。各主题桶的 thread 总数与格值口径不同时，系统 SHALL 以差额形式显式说明（如「另有 N 条综合稿未计入」），MUST NOT 让同一句说明里同时出现两个不同口径的线索数。

矩阵区 SHALL 常驻一行口径说明：数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度）。

「当日其他话题」组的线索合计 SHALL 与矩阵格同口径（非综合稿线索数合计），MUST NOT 使用含综合稿的 thread 总数；其注释 SHALL 与实际口径一致。

#### Scenario: 同屏不出现两个口径的线索数

- **GIVEN** 某日格值为 3，其所在桶 thread 总数为 5（其中 2 条综合稿）
- **WHEN** 用户悬停该格
- **THEN** 标题 SHALL 给出 3（非综合稿线索数）与主题桶数，并显式说明「另有 2 条综合稿未计入」
- **AND** SHALL NOT 在同一句里把 5 表述为「线索条数」

#### Scenario: 当日工作面标出综合稿

- **WHEN** 用户点开某日工作面，条目中含综合稿
- **THEN** 综合稿条目 SHALL 带可辨识标记，MUST NOT 与普通线索条目混同
- **AND** 标记 SHALL NOT 暗示综合稿被计入线索合计

#### Scenario: 常驻口径说明可见

- **WHEN** 用户查看矩阵区
- **THEN** 矩阵区 SHALL 常驻显示口径说明行（数字 = 当日线索合计 / 综合稿不计入）
