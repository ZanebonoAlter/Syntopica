## ADDED Requirements

### Requirement: 降级不得呈现为业务结论

在「版块总览聚合端点」之外，端点与前端 SHALL 满足下列降级诚实性契约：**「没取到」SHALL NOT 被渲染成「就是 0」**。

端点 SHALL 为每个版块行给出「本次是否取到」的判定：当端点因超时未完成该版块、或该版块泳道动态计算失败时，该行 SHALL 标 `data_unavailable=true`，且其计数类字段（`attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count`）SHALL 输出**未知**（`null`）而非 0；该行 SHALL NOT 参与关注度分档、行序与「N 个板块值得关注」计数。取数正常但确实没有数据的版块 SHALL 仍返回零值行且 SHALL NOT 标 `data_unavailable`。

前端 SHALL 把未取到的版块与真实零值区别呈现：报头标题 SHALL NOT 出现「0 个板块值得关注」这类由未取到数据推出的结论；当日线索合计与热点数在存在未取到版块时 SHALL 显示未知（—），SHALL NOT 把未取到的计数合计为 0 或据此断言「没有热点」；未取到的版块行 SHALL NOT 显示「关注强度 0.0」「0 篇文章 · 0 个事件」「无日报日期」等由未取到数据推出的呈现，SHALL 以「数据未取到」表达未知态，并 SHALL 保留进入版块与重试的可达路径。

#### Scenario: 全部版块未取到时不报「0 个板块值得关注」

- **WHEN** 端点因超时未取到任何版块的数据（全部行 `data_unavailable=true`）
- **THEN** 报头标题 SHALL 显示「本次数据未取全」类表述
- **AND** SHALL NOT 显示「0 个板块值得关注」
- **AND** 页面 SHALL NOT 渲染「今天没有需要看的版块」这一业务空态（未取到 ≠ 真的没有）
- **AND** SHALL 提供重试入口

#### Scenario: 部分版块未取到时不污染统计

- **WHEN** 部分版块 `data_unavailable=true`，其余版块取数正常
- **THEN** 报头 SHALL 标注本次数据不完整
- **AND** 当日线索合计与热点数 SHALL 显示未知（—），SHALL NOT 把未取到的版块计为 0
- **AND** 未取到的版块 SHALL NOT 参与关注度分档与行序，SHALL NOT 被计入「N 个板块值得关注」
- **AND** 行内 SHALL 以「数据未取到」表达，SHALL NOT 显示「关注强度 0.0」

#### Scenario: 未取到与真实零值可区分

- **WHEN** 端点未取到某版块的数据
- **THEN** 该行 SHALL 标 `data_unavailable=true`，且 `attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count` SHALL 为未知
- **AND** SHALL NOT 输出 0 冒充该版块的业务结论

#### Scenario: 真实零值仍照旧

- **WHEN** 某活跃版块近 14 天没有日报或泳道，取数本身成功
- **THEN** 端点 SHALL 返回该版块的零值行且 SHALL NOT 标 `data_unavailable`
- **AND** 前端 SHALL 仍将其归入「常温」，SHALL NOT 标为「数据未取到」

#### Scenario: 计算失败与超时同标

- **WHEN** 某版块的泳道动态计算失败（而非端点超时）
- **THEN** 该行 SHALL 与超时未取到的版块一样标 `data_unavailable=true`
- **AND** SHALL NOT 只对超时路径标未取到、让计算失败路径继续以零值冒充业务结论

### Requirement: 滞后版块不占前排

主列表的行序 SHALL 把「锚点日落后于全局锚点日」的版块（滞后版块）从关注度排序里拆出来：滞后版块 SHALL NOT 参与关注度分档（热点 / 关注 / 常温）、SHALL NOT 凭旧数据的关注度占据前排名次；SHALL 逐行显示在主列表最底部，SHALL 保留自己锚点日的「数据至 MM/DD」中性标注，SHALL NOT 冒充全局锚点日。

「没取到」的版块（`data_unavailable=true`）SHALL 排在滞后版块之下，行序序号 SHALL 连续不重号。

报头的「今日线索」合计 SHALL 只计锚点日与全局锚点日一致的版块：滞后版块的线索数是它自己那个更早的锚点日的数，SHALL NOT 计入今日合计（旧数据不污染今日口径）。

#### Scenario: 滞后版块的旧日线索不进今日合计

- **WHEN** 某版块锚点日为 2026-11-07、当日线索 3 条，全局锚点日为 2026-11-18
- **THEN** 报头「今日线索」合计 SHALL NOT 包含这 3 条
- **AND** 该版块自己的行内线索数 SHALL 照旧如实显示

#### Scenario: 滞后版块不占前排名次

- **WHEN** 某版块锚点日为 2026-11-07、而全局锚点日为 2026-11-18，且其关注度高于其它若干版块
- **THEN** 该版块 SHALL NOT 出现在按关注度排列的前排名次
- **AND** SHALL NOT 挂「热点 / 关注 / 常温」分档徽标
- **AND** SHALL 逐行显示在主列表末尾，并标「数据至 11/07」
- **AND** 该版块 SHALL NOT 被折叠隐藏（仍可进版块）

#### Scenario: 未取到版块排在最底

- **WHEN** 某版块 `data_unavailable=true`
- **THEN** 该版块 SHALL 排在滞后版块之后（列表最底）
- **AND** 行序序号 SHALL 与其它行连续、不重号
