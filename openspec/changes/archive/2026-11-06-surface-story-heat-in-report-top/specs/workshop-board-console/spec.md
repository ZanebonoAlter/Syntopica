## MODIFIED Requirements

### Requirement: 泳道热度口径（2026-10-27 矩阵阶段修订）

泳道热度与计数 SHALL 分三个口径并各自命名，MUST NOT 混称：**矩阵日格** = 该泳道该日的事件线数，同日多个主题桶 SHALL 合并计数而非取峰值；**矩阵格颜色** = 该泳道该日最热事件线的独立参与者数（`participant_count`），分档 SHALL 有图例；**演进节点 / 泳道趋势** = 该泳道该日的事件线数；**行序** = 基准响应全窗 `lane_heat` 降序 → 全窗事件数降序 → topic id 升序。

矩阵行 MUST NOT 再渲染行尾重复总数列；行头与格子的可读文本（title/aria）SHALL 同时给出事件线数、最热事件线参与者数、未归组事件数与主题桶数，避免口径不可见。同日主题分桶 MUST NOT 冒充热度（分桶多 ≠ 更热）；`事件` 条数 MUST NOT 直接充当事件数；`section` 数 MUST NOT 参与任何热度或排序计算。

事件身份未回填（`story_id` 全为 NULL）的版块 SHALL 按「每个非综合稿事件记 1 条事件线」保守计算，并在矩阵 footnote 明示「事件身份未回填」，MUST NOT 显示 0 或空态冒充无事件。

#### Scenario: 同日分桶不冒充分区热度

- **WHEN** 某泳道同日两个主题桶各含 1 个事件线，另一泳道当日 1 个桶含 2 个事件线
- **THEN** 前者日格 SHALL 显示 2（两桶事件线数之和，非桶峰值 1），后者 SHALL 也显示 2；行序 SHALL 由 `lane_heat` 决定，SHALL NOT 由 section 数或桶峰值决定

#### Scenario: 矩阵格值是桶峰值

- **WHEN** 用户查看矩阵格内数字
- **THEN** 该数字 SHALL 为该泳道当日去重后的事件线数；事件线未知（未回填）时 SHALL 按非综合稿事件数作上界并给出降级说明，MUST NOT 用主题桶数量或事件总数冒充

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


### Requirement: 矩阵视图（泳道 × 日期）

版块态势页默认视图 SHALL 为矩阵：行为泳道（可为 0 行）、列为观察范围内的日期（见「观察窗与自由日期显示」）、格内数字为该泳道当日的事件线数。无分区的格子 SHALL 显示断档标记（如「—」）且不可点击；有分区但零事件（全部事件为综合稿或未归组且无事件线）的格子 SHALL 显示 0 且可进入当日工作面（零事件是合法结果）。日期列头 SHALL 对真实报日可选并写入与日格/演进节点同一个选中日（见「日报日驱动的日期轴」）；版块无正式泳道但有日报时 MUST NOT 以空态替换网格与日期轴。矩阵 SHALL 只读展示，MUST NOT 在格内提供归档/合并/生命周期写操作。可见日数超过 31 时矩阵 SHALL 在舞台容器内横向滚动（列宽随列数），MUST NOT 产生页面级横向滚动。

矩阵 SHALL 在表头或 footnote 提供一条图例，说明数字为事件线数、颜色为来源数分档；颜色 SHALL NOT 是唯一信息载体。

#### Scenario: 渲染矩阵

- **WHEN** 版块观察范围内有 6 条泳道、网格覆盖 14 天
- **THEN** 矩阵 SHALL 渲染 6 行 × 14 列，行尾无重复总数列，并在今天所在列落于范围内时高亮今天列、以独立提示表达数据截止日与今天的区别

#### Scenario: 图例可见

- **WHEN** 用户首次打开矩阵
- **THEN** 表头或 footnote SHALL 显示「数字 = 当日事件线数；颜色 = 当天被报道最多的那条事件线的来源数」图例，且每个格子的 title/aria SHALL 带具体数字

#### Scenario: 断档可读

- **WHEN** 某泳道在 10-17 没有 section
- **THEN** 该格 SHALL 为断档标记（非 0）、不可点击，视觉与可点格可区分，可读文本 SHALL 注明无分区

#### Scenario: 零线索可开

- **WHEN** 某泳道某日有分区但全部主题桶无事件（无事件线且无非综合稿事件）
- **THEN** 该格 SHALL 显示 0 并可进入当日工作面，MUST NOT 与断档格混淆
