# persistent-topic Delta

## MODIFIED Requirements

### Requirement: 手动建持久化话题

系统 SHALL 提供手动创建持久化话题的能力：用户提交 `label` 与一组 `section_ids`，系统在单个事务内创建新 PersistentTopic 并将选中 section 归属改写到该话题。

创建流程 SHALL：
1. 读取选中 section 的 embedding（维度缺失的 section 跳过并返回提示）。
2. 聚合为 mean pooling 平均向量作为新话题 embedding。
3. `CreateTopic`：status=active、source=manual、embedding=聚合向量、first_seen_date=min(选中 section period_date)、last_seen_date=max(...)、hit_count=选中数、consecutive_hits=选中数。
4. 对每个选中 section 调用归属改写（persistent_topic_id=新话题、confidence=manual、distance=该 section 到聚合锚点距离）。
5. **在同一事务内追加一行首义留痕**：`topic_anchor_overrides` 写入 `source=selected_sections`、`input_summary` = **实际参与聚合的 section id 列表**（跳过无向量的 section 不计入），使该话题的首义来源可被读端点反解为「人工（建话题时选定的 N 个节点）」。留痕口径与人工重设首义**完全一致**（同一张表、同一字段语义），SHALL **NOT 新增来源枚举**。
6. 事务末尾触发 `RebuildBoardRelations`（幂等重建该 board 关系，重算 identity 边，保证血统一致）。

手动话题 SHALL 跳过 `upgrade_threshold` 连续命中门禁（自动路径的 candidate→active 门禁对本路径不生效），因用户主权声明且已有选中内容支撑。

手动话题 SHALL NOT 调用任何 LLM（embedding 聚合为纯向量运算）。

**留痕 SHALL 与建话题同生共死**：首义留痕写入失败 SHALL 回滚整个建话题事务（连同话题创建与归属改写一并回滚），SHALL NOT 留下「有话题、无留痕」的半成品——首义不可考的话题会一直显示「来源不可考」，那正是本条要消除的状态。

**历史手动建的泳道 SHALL NOT 回填**：系统 SHALL NOT 对本条生效前创建的手动话题事后推断其首义来源（当时未留痕，事后推断即编造），SHALL 继续按「创建时写入 · 来源不可考」读取；用户想补齐 SHALL 走既有「用选中的 N 个节点定首义」重新定一次（该路径本身留痕）。

**非目标**：算法自动创建的候选话题（`auto_new`）的首义留痕不在本条范围内（其首义是首条命中节点向量，是否留痕另行决定）。

#### Scenario: 手动串联建泳道

- **WHEN** 用户提交 label="美伊博弈"、section_ids=[#101,#105,#110]（均属其他话题），三者 embedding 聚合得向量 V
- **THEN** 系统 SHALL 创建 topic（label=美伊博弈, status=active, source=manual, embedding=V），并将 #101/#105/#110 的 persistent_topic_id 改写为该 topic、confidence=manual

#### Scenario: 绕过升级门禁

- **GIVEN** upgrade_threshold=3
- **WHEN** 手动创建话题（选中 5 条 section）
- **THEN** 新话题 SHALL 直接 active，SHALL NOT 先进 candidate 状态，SHALL NOT 等待连续命中天数

#### Scenario: 选中 section 含无 embedding

- **GIVEN** 选中 section #200 的 embedding 为空
- **WHEN** 执行手动建泳道
- **THEN** 系统 SHALL 跳过 #200（不计入聚合、不改写其归属），SHALL 在响应中提示"1 条 section 因无向量被跳过"，其余正常建泳道

#### Scenario: 关系重建保证血统一致

- **WHEN** 手动建泳道事务提交
- **THEN** 系统 SHALL 执行 RebuildBoardRelations，新话题下相邻天的 section SHALL 写入 identity 关系边，被移出原话题的 section 与原话题的 identity 边 SHALL 不再保留

#### Scenario: 手动建泳道非致命于日报生成

- **WHEN** 手动建泳道事务中 RebuildBoardRelations 失败
- **THEN** 系统 SHALL 回滚整个事务（含话题创建与归属改写），SHALL NOT 留下半成品状态

#### Scenario: 建话题时写入首义留痕

- **GIVEN** 用户手动创建泳道，选中 section #101/#105/#110（均有向量）
- **WHEN** 建话题事务提交
- **THEN** 系统 SHALL 在同一事务内向 `topic_anchor_overrides` 追加一行（`source=selected_sections`、`input_summary` = [#101,#105,#110]）
- **AND** 读该话题的 anchor 时 SHALL 返回 `source=manual` 与这 3 个 section 作为首义节点集

#### Scenario: 留痕只记参与聚合的节点

- **GIVEN** 用户手动创建泳道，选中 4 条 section，其中 1 条无 embedding 被跳过
- **WHEN** 建话题事务提交
- **THEN** 留痕的 `input_summary` SHALL 只含实际参与聚合的 3 条 section id
- **AND** 被跳过的 section SHALL NOT 出现在首义节点集里

#### Scenario: 留痕写入失败整体回滚

- **WHEN** 手动建泳道事务中首义留痕写入失败
- **THEN** 系统 SHALL 回滚整个事务（话题创建与归属改写一并回滚）
- **AND** SHALL NOT 留下「有话题、无首义留痕」的半成品

#### Scenario: 历史手动泳道不回填

- **GIVEN** 一条在本条生效之前手动创建的泳道（当时未写首义留痕）
- **WHEN** 用户读取它的 anchor
- **THEN** 系统 SHALL 报来源不可考，SHALL NOT 事后推断出首义节点集
- **AND** 该话题 SHALL 仍可正常使用（不阻断任何操作）

### Requirement: 泳道成员计数与接近度的单一口径

泳道工作面 SHALL 用**同一把尺子**量所有成员相关数字：

- **窗口内成员数** = 取数窗口内、归属本话题的成员 section 数；
- **全部成员数** = 本话题的成员 section 总数（窗口内 + 窗口外），SHALL NOT 用「窗口内 + 窗口外」的加法与「去重并集」两套算法各算一个「全部」，同屏出现两个不同的「全部 M」；
- **窗口内成员集合与窗口外成员集合 SHALL 用同一过滤判据**（向量可用性判据一致），MUST NOT 一侧要求向量、另一侧不要求，导致「窗口 N」与「全部 M」不是同一集合的子集关系。

**接近度** SHALL 保留两把尺子但 MUST 可区分：成员接近度 = 日报生成时的归属快照距离换算（`1 − topic_match_distance`，入库即冻结）；候选/推荐接近度 = 对当前成员质心的实时估算（`1 − 余弦距离`）。实时估算值 SHALL 在展示上带「估算」标注，MUST NOT 与快照值同名同形。

**展示幅度 SHALL 与来源口径分开**：候选/推荐接近度的**输入**恒为上述实时估算，其**显示幅度** MAY 为同一渲染批次内的相对归一（最近 = 最大最深、最远 = 最小最浅）；成员接近度 SHALL 恒为**归属快照原值**，SHALL NOT 参与任何批内归一、SHALL NOT 因同批次内容变化而改变数值或尺寸。两者 SHALL 同名（「接近度」）但**不同形**：成员尺寸固定（不编码接近度），候选尺寸编码批内相对位置，且各自 SHALL 带来源标注（快照 / 估算），SHALL NOT 出现同名同形、无法区分来源的数字。

**「窗口」一词 SHALL 只有一个含义**：成员计数口径的「窗口内 N」是**报告期观察窗**；质心滚动口径的「滚动窗口 N」是**参与滚动计算的 section 条数上限**（默认 30）。两者 SHALL 分开命名，SHALL NOT 在同一界面用同一个「窗口 N」指代两把尺子。

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

#### Scenario: 批内归一不污染成员快照

- **GIVEN** 某话题有成员行与候选行同屏展示
- **WHEN** 候选批内的相对远近变化
- **THEN** 候选行的尺寸与填色 SHALL 按批内相对位置重新分布
- **AND** 成员行的接近度数值与尺寸 SHALL 保持不变（成员尺寸固定、不编码接近度）

#### Scenario: 两个窗口口径不同名

- **GIVEN** 某话题窗口内成员 11、全部成员 108、参与滚动计算的 section 30（窗口上限 30）
- **WHEN** 用户在同一界面查看这些计数
- **THEN** 成员计数 SHALL 显示为「窗口内 11 · 全部 108」，质心滚动口径 SHALL 显示为另一名称（如「滚动窗口 30」/「参与滚动计算的节点（30/30）」）
- **AND** SHALL NOT 出现两个同名「窗口 N」分别指代成员观察窗与滚动计算窗
