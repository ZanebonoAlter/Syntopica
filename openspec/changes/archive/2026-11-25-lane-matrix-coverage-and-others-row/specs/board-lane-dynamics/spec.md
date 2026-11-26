## MODIFIED Requirements

### Requirement: 泳道动态聚合端点

系统 SHALL 提供 `GET /semantic-boards/:id/lane-dynamics?days=14` 端点，单次返回该板块全部泳道行数据：泳道元信息（含 watch 关联标识）、泳道当天热度（`lane_heat`，新口径）、全窗线索合计、滚动态势（`summary` 单版演进叙事态势句 + `as_of` 与快照存在性；`detail` 仅存量历史值透出，新结算恒空、omitempty 不出现）、逐日发展时间线（日期 → 该日线索合计与事件 thread 标题列表）、候选清单。days 参数 SHALL 支持指定时间线窗口（默认 14）。

逐日条目 SHALL 给出该泳道该日的线索合计（thread 数），SHALL NOT 再下发事件线数、最热事件线身份或未归组事件数。候选清单 SHALL 附带候选泳道当天（锚点日）的线索合计，使版块关注度能够对候选泳道按候选权重计入；SHALL NOT 给候选泳道编造时间线。

响应 SHALL 另带 `other_topics` 逐日数组：窗口内**每一份 completed 日报日**一条（含无未入泳道分区的真实 0），字段为日期、该日未入泳道分区数、该日未入泳道分区的**非综合稿线索数**。「未入泳道」SHALL 指 `persistent_topic_id` 为空、或不属于该板块当前**非归档**的 active 与 watch 关联话题集合的 section（归档话题即使仍被 watch 指向也不在泳道集合里，其分区 SHALL 计入）；MUST NOT 计入未完成日报，MUST NOT 把综合稿计入线索数。该字段只描述既有 section/thread 归属，MUST NOT 改动泳道行、候选清单或热度的任何计算。

#### Scenario: 单请求聚合

- **WHEN** 前端请求某板块的 lane-dynamics
- **THEN** 响应 SHALL 包含主区全部数据与候选栏数据，前端无需再发 N 次逐泳道请求

#### Scenario: 存量长版透出

- **WHEN** 某泳道快照含存量历史长版（其泳道尚未经历新结算）
- **THEN** 响应中该泳道态势对象 SHALL 原样透出存量 `detail` 值（读旧值无害）；新结算后该字段恒空、不再出现，不存在「长版缺失」二级降级语义

#### Scenario: 无态势快照降级

- **WHEN** 某泳道无滚动态势快照（如新泳道或首个日报日前）
- **THEN** 响应 SHALL 明示快照缺失，前端 SHALL 展示「待结算」占位且发展时间线照常渲染

#### Scenario: 逐日负载为线索合计

- **WHEN** 前端读取某泳道某日的时间线条目
- **THEN** 该条目 SHALL 带该日线索合计
- **AND** SHALL NOT 带事件线数、最热事件线标题或未归组事件数

#### Scenario: 候选泳道带当天线索数

- **WHEN** 候选栏列出某 candidate 泳道
- **THEN** 该条目 SHALL 带其锚点日的线索合计
- **AND** 版块关注度 SHALL 能据此按候选权重计入该泳道

#### Scenario: 未入泳道逐日读数含真实零

- **WHEN** 窗口内某日报日的全部分区都已归属 active/watch 泳道
- **THEN** 该日 SHALL 出现在 `other_topics` 中，分区数与线索数均为 0（真实零，MUST NOT 省略成缺失）

#### Scenario: 未入泳道排除口径与矩阵其他组一致

- **WHEN** 某日报日的分区指向 candidate、archived 或没有 persistent topic
- **THEN** 该分区 SHALL 计入 `other_topics`（与前端「当日其他话题」的 A 名单排除一致）；归属 active 或 watch 关联话题的分区 MUST NOT 计入

#### Scenario: 未完成日报不参与

- **WHEN** 窗口内存在 status 非 completed 的日报
- **THEN** 该日的 sections/threads MUST NOT 进入 `other_topics`（与泳道时间线同一边界）
