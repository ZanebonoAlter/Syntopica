## MODIFIED Requirements

### Requirement: DailyReportSection 强制归属 PersistentTopic

`daily_report_sections` SHALL 包含 `persistent_topic_id`（外键到 board_persistent_topics）、`topic_match_distance`、`topic_match_confidence`、`lane_tier` 四列。`topic_match_confidence` SHALL 取值 anchor_hit / auto_new / unmatched / manual。`lane_tier` SHALL 取值 l1_direct / l2_llm / l3_new，标识该 section 的分桶来源。

归属算法 SHALL 保证每个新生成的 section 归属到恰好 1 个 PersistentTopic，采用三层分桶：

- **L1 直挂（lane_tier=l1_direct）**：当天 tag 经 embedding 质心匹配，到某 **active** topic 质心距离 < `lane_l1_threshold`（默认 0.18）且该 topic 非 vacuum 时，section 直接归属该 topic，topic_match_confidence=anchor_hit，不调用 LLM。**candidate topic 不享有直挂资格**：即使距离 < `lane_l1_threshold`，最近话题为 candidate 时该 tag SHALL 降级进入 L2 band（观察期门禁，`persistent_topic_candidate_l1_gate_enabled` 默认启用；开关关闭时回退为 active/candidate 均可直挂的旧行为）。
- **L2 LLM 裁决（lane_tier=l2_llm）**：质心距离在 [`lane_l1_threshold`, `lane_l2_threshold`]（默认 [0.18, 0.30]）的 tag，或最近话题为 candidate 的近距离 tag，系统 SHALL 将其与 embedding 预筛的 top-K 候选 topic（默认 K=5）一并交 LLM 做「留/换/新」三选一；LLM 选「留」归属候选 topic（confidence=anchor_hit），选「换」归属 LLM 指定 topic（须校验在候选集内，confidence=anchor_hit），选「新」转 L3。**keep 解析尊重显式 target**：LLM 输出「留」但显式携带候选集内另一 target_topic_id 时（小模型常态混用 keep/switch），SHALL 尊重该指定归属而非静默改写回最近候选——否则 keep 会把 tag 无条件吸附回 embedding 最近处（含僵尸 candidate）；target 为空或不在候选集内时归属最近候选（安全网，不降级 new）。
- **约束准入（topic-lane-constraint）**：话题若配置了 `required` 辅助标签约束，lane group 的 section 标签集合 SHALL 先过约束覆盖判定（语义见 `topic-lane-constraint`）。未覆盖任一 required 约束时，该 lane group SHALL NOT 以 `l1_direct` 直挂该话题，SHALL 降级进入 L2 band 由 LLM 重新裁决（`lane_tier` 最终仍为 `l2_llm`）。**无约束话题 SHALL 逐字保持既有行为，不产生额外 LLM 调用。**
- **约束违例的终态**：L2 裁决若仍选中该话题、而其 required 约束依旧未被覆盖，系统 SHALL 沿用既有「switch 到候选集外 → 降级 new」范式，转 L3 新建 candidate，SHALL NOT 锚定到违例话题。
- **L3 新建（lane_tier=l3_new）**：质心距离 > `lane_l2_threshold` 或 L2 LLM 选「新」的 tag，系统 SHALL 新建 candidate topic 归属，topic_match_confidence=auto_new。

embedding 为空时 confidence=unmatched、lane_tier 允许为空、persistent_topic_id 允许为 NULL（仅此例外）。`persistent_topic_id` 列 SHALL NOT 加 NOT NULL 约束。`topic_match_confidence=manual` 表示用户手动改写（非算法判定），lane_tier 保持改写前的值，`topic_match_distance` 为该 section embedding 到所属话题质心的距离。

**人工改写归属 SHALL 同时支持「新建话题」与「归属变更（解除归属 / 加回）」两种路径**（后者见 lane-manual-correction 能力）。解除归属时，`persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `topic_status_at_report` SHALL 写为 NULL（不再归属任何泳道）；加回时 `persistent_topic_id` 写为目标话题、`topic_match_confidence` 写为 `manual`、`topic_match_distance` 为到目标话题锚点的实算距离、`topic_status_at_report` 写为 NULL。两种方向的 `lane_tier` SHALL 保持原值。归属变更 SHALL NOT 变更任何话题的命中计数与状态，SHALL 在事务末尾触发 `RebuildBoardRelations`，并 SHALL 将受影响的话题标记为待重算质心。

归属判定 SHALL 在 section 生成阶段完成（section 天生挂 topic），不再有事后 section 标题↔topic 匹配环节。

**一篇日报内同一 topic MAY 拥有多条 section**（泳道内主题分桶拆分、手动编排、watch 物化）。此类兄弟 section SHALL 各自独立持有归属列与各自的 `topic_match_distance`；话题命中计数 SHALL 按「该篇日报是否命中」去重（同一日报内多条只算一次），SHALL NOT 按 section 条数累加；质心刷新 SHALL 按 section 逐条计入窗口，SHALL NOT 对同日同话题的多条 section 做特殊去重。

#### Scenario: L1 直挂命中（质心强匹配）

- **GIVEN** board 有 active topic「Claude Code 工具链崛起」，当天 tag「Claude Code 模型选型」到该 topic 质心距离 0.12
- **WHEN** 系统做泳道分桶
- **THEN** 该 section SHALL 归属该 topic，topic_match_confidence=anchor_hit，lane_tier=l1_direct

#### Scenario: candidate 近距离降级 L2（观察期门禁）

- **GIVEN** board 有 candidate topic「伊朗议长透露凌晨应急决断」（一次性新闻标题），当天 tag「伊朗代防长发表强硬言论」到该 topic 质心距离 0.08，门禁开关启用
- **WHEN** 系统做泳道分桶
- **THEN** 该 tag SHALL 进入 L2 band，SHALL NOT 直挂该 candidate topic

#### Scenario: 门禁开关关闭回退旧行为

- **GIVEN** `persistent_topic_candidate_l1_gate_enabled=false`
- **WHEN** 某 tag 到 candidate topic 质心距离小于 L1 阈值
- **THEN** 系统 SHALL 允许直挂该 candidate topic（回退旧行为）

#### Scenario: vacuum active 不直挂（既有规则保持）

- **GIVEN** active topic X 被标记 is_vacuum，某 tag 到 X 质心距离 0.10
- **WHEN** 系统做泳道分桶
- **THEN** 该 tag SHALL NOT 直挂 X，SHALL 降级进入 L2 band

#### Scenario: L2 弱区 LLM 裁决留/换/新

- **GIVEN** 当天 tag「特斯拉工业扩张」到最近 topic 质心距离 0.25（落在 L2 弱区）
- **WHEN** 系统交 LLM 裁决
- **THEN** 系统 SHALL 依据 LLM 的留/换/新输出归属，confidence=anchor_hit

#### Scenario: L2 裁决中观察中话题从严判断

- **GIVEN** 候选集中含 candidate 话题
- **WHEN** LLM 裁决
- **THEN** 系统 SHALL 对观察中话题从严判断

#### Scenario: L2 keep 显式指定候选集内话题尊重归属

- **GIVEN** LLM 输出 keep 且显式给出候选集内另一 target_topic_id
- **WHEN** 系统解析裁决结果
- **THEN** 系统 SHALL 归属该显式指定话题，SHALL NOT 无条件吸附回最近候选

#### Scenario: L2 keep 集外/空 target 维持最近候选

- **GIVEN** LLM 输出 keep 但 target 为空或不在候选集内
- **WHEN** 系统解析裁决结果
- **THEN** 系统 SHALL 归属最近候选话题，SHALL NOT 降级为新建

#### Scenario: L3 新建候选（质心挂不上）

- **GIVEN** 当天 tag「商务部出口管制清单」到所有 topic 质心距离均 > 0.30
- **WHEN** 系统做泳道分桶
- **THEN** 系统 SHALL 新建 candidate topic 归属，topic_match_confidence=auto_new

#### Scenario: L2 LLM 指定 topic 超出候选集降级

- **GIVEN** LLM 输出的 target_topic_id 不在候选集内
- **WHEN** 系统解析裁决结果
- **THEN** 系统 SHALL 降级处理，SHALL NOT 归属集外话题

#### Scenario: embedding 为空标记未匹配

- **WHEN** section embedding 为空
- **THEN** topic_match_confidence=unmatched，lane_tier 为空，persistent_topic_id 为 NULL

#### Scenario: 手动改写归属

- **GIVEN** section #123 的 persistent_topic_id=8（lane_tier=l1_direct, confidence=anchor_hit）
- **WHEN** 用户在编排态将 section #123 串联进手动新建的 topic #20
- **THEN** 系统 SHALL 将 persistent_topic_id 改写为 20，topic_match_confidence=manual，lane_tier 保持 l1_direct，topic_match_distance=该 section embedding 到 #20 质心的距离

#### Scenario: 手动解除归属

- **GIVEN** section #123 的 persistent_topic_id=8（lane_tier=l1_direct, confidence=anchor_hit）
- **WHEN** 用户将 section #123 从 #8 移除
- **THEN** 系统 SHALL 将 persistent_topic_id / topic_match_distance / topic_match_confidence / topic_status_at_report 写为 NULL，lane_tier 保持 l1_direct，SHALL 记录归属变更留痕，SHALL 在事务末尾触发 RebuildBoardRelations，SHALL 将 #8 标记为待重算质心

#### Scenario: 手动加回被移除的节点

- **GIVEN** section #123 当前不归属任何泳道（曾被从 #8 移除）
- **WHEN** 用户把 #123 加回 topic #8
- **THEN** 系统 SHALL 将 persistent_topic_id 写为 8、topic_match_confidence 写为 manual、topic_match_distance 写为 #123 embedding 到 #8 锚点的实算距离、topic_status_at_report 写为 NULL，lane_tier 保持原值，SHALL 记录归属变更留痕（动作 attach），SHALL 将 #8 标记为待重算质心

#### Scenario: 同日多条 section 只记一次命中

- **WHEN** 同一日报内同一 topic 命中多条 section
- **THEN** 命中计数 SHALL 只按该日报计一次，SHALL NOT 按 section 条数累加

#### Scenario: 质心按 section 逐条计入

- **GIVEN** topic #502 在某日报内有 3 条 section（各自 embedding 不同）
- **WHEN** 系统重算该话题质心
- **THEN** 3 条 section 的 embedding SHALL 各自计入 `centroid_window` 窗口，SHALL NOT 按当日合并为一条

#### Scenario: required 约束未覆盖时不得直挂

- **GIVEN** 话题 #2 绑定了 `required` 约束「以色列」与「黎巴嫩」，某 lane group 的 section 标签集合只覆盖「以色列」侧
- **WHEN** 该 lane group 到话题 #2 质心的距离小于 `persistent_topic_lane_l1_threshold`
- **THEN** 系统 SHALL NOT 以 `l1_direct` 直挂话题 #2，SHALL 降级进入 L2 band

#### Scenario: 约束满足时 L1 直挂不变

- **GIVEN** 同上话题，某 lane group 的标签集合分别覆盖「以色列」与「黎巴嫩」两条 required 约束
- **WHEN** 该 lane group 到话题 #2 质心的距离小于 `persistent_topic_lane_l1_threshold`
- **THEN** 系统 SHALL 按既有规则直接归属话题 #2，`lane_tier` 为 `l1_direct`、`topic_match_confidence` 为 `anchor_hit`，且 SHALL NOT 调用 LLM

#### Scenario: 无约束话题的 L1 直挂不变

- **GIVEN** 话题 #3 未配置任何辅助标签约束
- **WHEN** 某 lane group 到话题 #3 质心的距离小于 `persistent_topic_lane_l1_threshold`
- **THEN** 系统 SHALL 按既有规则直接归属，行为与引入约束机制前逐字一致，且 SHALL NOT 产生额外 LLM 调用
