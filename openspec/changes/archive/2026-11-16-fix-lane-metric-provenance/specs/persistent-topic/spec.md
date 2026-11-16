# persistent-topic — 泳道成员计数与接近度单一口径 delta

## ADDED Requirements

### Requirement: 泳道成员计数与接近度的单一口径

泳道工作面 SHALL 用**同一把尺子**量所有成员相关数字：

- **窗口内成员数** = 取数窗口内、归属本话题的成员 section 数；
- **全部成员数** = 本话题的成员 section 总数（窗口内 + 窗口外），SHALL NOT 用「窗口内 + 窗口外」的加法与「去重并集」两套算法各算一个「全部」，同屏出现两个不同的「全部 M」；
- **窗口内成员集合与窗口外成员集合 SHALL 用同一过滤判据**（向量可用性判据一致），MUST NOT 一侧要求向量、另一侧不要求，导致「窗口 N」与「全部 M」不是同一集合的子集关系。

**接近度** SHALL 保留两把尺子但 MUST 可区分：成员接近度 = 日报生成时的归属快照距离换算（`1 − topic_match_distance`，入库即冻结）；候选/推荐接近度 = 对当前成员质心的实时估算（`1 − 余弦距离`）。实时估算值 SHALL 在展示上带「估算」标注，MUST NOT 与快照值同名同形。

话题生命线响应 SHALL NOT 因 `topic_match_distance` 恰为 0 而省略该字段（值为 0 是合法结果，不是缺数据）。

矩阵窗口与泳道工作面窗口的**末日锚点 SHALL 一致**（同为「最新 completed 日报日」），SHALL NOT 一个取不筛状态的 `MAX(period_date)`、另一个取 `status='completed'`，导致有未完成日报时两个窗口末日差一天。

#### Scenario: 同屏只有一个全部 M

- **GIVEN** 某话题窗口内成员 11、窗口外成员 97
- **WHEN** 用户同时查看泳道工作面左栏「本泳道成员」计数与「首义范围」面板计数
- **THEN** 两处的「全部」SHALL 同为 108，MUST NOT 一个显示 108、另一个显示 99（去重并集口径）

#### Scenario: 窗口内外集合对称

- **GIVEN** 某话题窗口内有 11 个成员、窗口外有 97 个成员，其中若干成员无可用向量
- **WHEN** 系统统计成员数
- **THEN** 窗口内计数与窗口外计数 SHALL 用同一向量可用性判据过滤
- **AND** 「窗口 N」SHALL 恒为「全部 M」的真子集，MUST NOT 出现 N > M 或 N 与更早成员数之和 ≠ M

#### Scenario: 接近度区分快照与估算

- **WHEN** 用户查看泳道工作面左栏的成员行与关联推荐行
- **THEN** 成员行的接近度 SHALL 标注为归属快照口径
- **AND** 关联推荐行的接近度 SHALL 标注为估算口径（对当前成员质心实时计算）
- **AND** 两处 MUST NOT 出现同名同形、无法区分来源的数字

#### Scenario: 距离为零不被丢弃

- **GIVEN** 某成员与其话题质心的归属距离恰为 0
- **WHEN** 客户端读取话题生命线
- **THEN** 响应 SHALL 含 `topic_match_distance: 0`，MUST NOT 省略该字段
- **AND** 客户端 SHALL 渲染接近度 1.00，MUST NOT 渲染为空/未知

#### Scenario: 两个窗口末日一致

- **GIVEN** 某版块存在一份未完成日报，其 `period_date` 比最新 completed 日报晚一天
- **WHEN** 用户对比矩阵观察窗末日与泳道工作面窗口末日
- **THEN** 两处 SHALL 取同一末日（最新 completed 日报日），MUST NOT 相差一天
