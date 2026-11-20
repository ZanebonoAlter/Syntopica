## MODIFIED Requirements

### Requirement: DailyReportSection 强制归属 PersistentTopic

`daily_report_sections` SHALL 包含 `persistent_topic_id`（外键到 board_persistent_topics）、`topic_match_distance`、`topic_match_confidence`、`lane_tier` 四列。`topic_match_confidence` SHALL 取值 anchor_hit / auto_new / unmatched / manual。`lane_tier` SHALL 取值 l1_direct / l2_llm / l3_new，标识该 section 的分桶来源。

归属算法 SHALL 保证每个新生成的 section 归属到恰好 1 个 PersistentTopic，采用三层分桶：

- **L1 直挂（lane_tier=l1_direct）**：当天 tag 经 embedding 质心匹配，到某 **active** topic 质心距离 < `lane_l1_threshold`（默认 0.18）且该 topic 非 vacuum 时，section 直接归属该 topic，topic_match_confidence=anchor_hit，不调用 LLM。**candidate topic 不享有直挂资格**：即使距离 < `lane_l1_threshold`，最近话题为 candidate 时该 tag SHALL 降级进入 L2 band（观察期门禁，`persistent_topic_candidate_l1_gate_enabled` 默认启用；开关关闭时回退为 active/candidate 均可直挂的旧行为）。

- **约束准入（话题辅助标签约束）**：话题存在 required 约束时，section 的标签集合 SHALL 覆盖全部 required 约束（判定语义见 `topic-lane-constraint`）方可锚定到该话题；未覆盖时该 tag SHALL NOT 直挂，改走 L2 band。该话题无任何约束行时本条 SHALL 为直通，归属行为与引入约束前逐字一致，SHALL NOT 产生额外 LLM 调用。
- **L2 LLM 裁决（lane_tier=l2_llm）**：质心距离在 [`lane_l1_threshold`, `lane_l2_threshold`]（默认 [0.18, 0.30]）的 tag，或最近话题为 candidate 的近距离 tag，系统 SHALL 将其与 embedding 预筛的 top-K 候选 topic（默认 K=5）一并交 LLM 做「留/换/新」三选一；LLM 选「留」归属候选 topic（confidence=anchor_hit），选「换」归属 LLM 指定 topic（须校验在候选集内，confidence=anchor_hit），选「新」转 L3。**keep 解析尊重显式 target**：LLM 输出「留」但显式携带候选集内另一 target_topic_id 时（小模型常态混用 keep/switch），SHALL 尊重该指定归属而非静默改写回最近候选——否则 keep 会把 tag 无条件吸附回 embedding 最近处（含僵尸 candidate）；target 为空或不在候选集内时归属最近候选（安全网，不降级 new）。

**约束违例**：L2 裁决选定的话题若其 required 约束未被覆盖，系统 SHALL 沿用「switch 到候选集外 → 降级为 new」的既有范式处理，SHALL NOT 锚定到约束不满足的话题。
- **L3 新建（lane_tier=l3_new）**：质心距离 > `lane_l2_threshold` 或 L2 LLM 选「新」的 tag，系统 SHALL 新建 candidate topic 归属，topic_match_confidence=auto_new。

embedding 为空时 confidence=unmatched、lane_tier 允许为空、persistent_topic_id 允许为 NULL（仅此例外）。`persistent_topic_id` 列 SHALL NOT 加 NOT NULL 约束。`topic_match_confidence=manual` 表示用户手动改写（非算法判定），lane_tier 保持改写前的值，`topic_match_distance` 为该 section embedding 到所属话题质心的距离。

归属判定 SHALL 在 section 生成阶段完成（section 天生挂 topic），不再有事后 section 标题↔topic 匹配环节。

**一篇日报内同一 topic MAY 拥有多条 section**（泳道内主题分桶拆分、手动编排、watch 物化）。此类兄弟 section SHALL 各自独立持有归属列与各自的 `topic_match_distance`；话题命中计数 SHALL 按「该篇日报是否命中」去重（同一日报内多条只算一次），SHALL NOT 按 section 条数累加；质心刷新 SHALL 按 section 逐条计入窗口，SHALL NOT 对同日同话题的多条 section 做特殊去重。

#### Scenario: L1 直挂命中（质心强匹配）

- **GIVEN** board 有 active topic「Claude Code 工具链崛起」，当天 tag「Claude Code 模型选型」到该 topic 质心距离 0.12
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag 所属 section SHALL 归属该 topic，lane_tier=l1_direct，topic_match_confidence=anchor_hit，不调用 LLM

#### Scenario: candidate 近距离降级 L2（观察期门禁）

- **GIVEN** board 有 candidate topic「伊朗议长透露凌晨应急决断」（一次性新闻标题），当天 tag「伊朗代防长发表强硬言论」到该 topic 质心距离 0.08，门禁开关启用
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag SHALL 进入 L2 band 交 LLM 裁决，SHALL NOT 直挂；LLM 判「新」时转 L3，当日该 candidate 无其他命中时不刷新 last_seen_date

#### Scenario: 门禁开关关闭回退旧行为

- **GIVEN** 同上场景但 `persistent_topic_candidate_l1_gate_enabled=false`
- **WHEN** 系统执行三层分桶
- **THEN** candidate topic 近距离 tag SHALL 按 L1 直挂（旧行为）

#### Scenario: vacuum active 不直挂（既有规则保持）

- **GIVEN** active topic X 被标记 is_vacuum，某 tag 到 X 质心距离 0.10
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag SHALL 进入 L2 band（vacuum 降级规则不变）

#### Scenario: L2 弱区 LLM 裁决留/换/新

- **GIVEN** 当天 tag「特斯拉工业扩张」到最近 topic 质心距离 0.25（落在 L2 弱区）
- **WHEN** 系统将 tag 与 top-K 候选 topic 交 LLM
- **THEN** LLM SHALL 输出留/换/新三选一；选「留」归属候选 topic（anchor_hit），选「换」归属指定 topic（须在候选集内），选「新」转 L3

#### Scenario: L2 裁决中观察中话题从严判断

- **GIVEN** L2 裁决 prompt 含 candidate topic「伊朗议长透露凌晨应急决断」（状态标注"观察中"）及其近期 section 的实际 tag 标签（与当日 tag 语义无关）
- **WHEN** LLM 裁决当日 tag 的归属
- **THEN** prompt SHALL 已指示 LLM 依据候选话题近期实际内容从严判断；无近期内容支撑的一次性标题话题不应仅凭域相近 keep

#### Scenario: L2 keep 显式指定候选集内话题尊重归属

- **GIVEN** 某 tag 最近候选为 candidate topic #1032（距离 0.10），候选集含 active topic #1151（距离 0.12）
- **WHEN** LLM 输出 `{"decision":"keep","target_topic_id":1151}`
- **THEN** 系统 SHALL 将该 tag 归属 #1151（尊重 LLM 显式指定），SHALL NOT 改写回最近候选 #1032

#### Scenario: L2 keep 集外/空 target 维持最近候选

- **WHEN** LLM 输出 keep 但 target_topic_id 为空或指向候选集外话题
- **THEN** 系统 SHALL 归属最近候选（安全网），SHALL NOT 降级 new（不同于集外 switch 的降级规则）

#### Scenario: required 约束未覆盖时不得直挂

- **GIVEN** active topic「以黎冲突升级：停火破裂与军事打击」有 required 约束「以色列」与「黎巴嫩」，某 tag「以色列议会选举计票完成」到该 topic 质心距离 0.08（< L1 阈值 0.18），但该 section 的标签集合无任何标签满足「黎巴嫩」约束的覆盖条件
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag SHALL NOT 直挂到该话题，SHALL 进入 L2 band 交 LLM 裁决

#### Scenario: 约束满足时 L1 直挂不变

- **GIVEN** 同上话题，某 tag「以军空袭黎巴嫩真主党据点」到该 topic 质心距离 0.10，且该 section 的标签集合覆盖全部 required 约束
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag 所属 section SHALL 仍以 lane_tier=l1_direct 归属该话题，SHALL NOT 额外调用 LLM

#### Scenario: 无约束话题的 L1 直挂不变

- **GIVEN** active topic X 无任何辅助标签约束，某 tag 到 X 质心距离 0.10
- **WHEN** 系统执行三层分桶
- **THEN** 该 tag 所属 section SHALL 仍以 lane_tier=l1_direct 归属 X，归属行为与引入约束前逐字一致

#### Scenario: L3 新建候选（质心挂不上）

- **GIVEN** 当天 tag「商务部出口管制清单」到所有 topic 质心距离均 > 0.30
- **WHEN** 系统执行三层分桶
- **THEN** 系统 SHALL 新建 candidate topic 归属，lane_tier=l3_new，topic_match_confidence=auto_new

#### Scenario: L2 LLM 指定 topic 超出候选集降级

- **WHEN** L2 裁决中 LLM 输出「换」且目标 topic 不在预筛候选集
- **THEN** 系统 SHALL 将其视为「新」，转 L3 新建 candidate，并在 section 元数据标注 `llm_target_off_shortlist=true`

#### Scenario: embedding 为空标记未匹配

- **WHEN** section embedding 为空
- **THEN** 系统 SHALL 设置 topic_match_confidence=unmatched，lane_tier 为空，persistent_topic_id 保持 NULL

#### Scenario: 手动改写归属

- **GIVEN** section #123 的 persistent_topic_id=8（lane_tier=l1_direct, confidence=anchor_hit）
- **WHEN** 用户在编排态将 section #123 串联进手动新建的 topic #20
- **THEN** 系统 SHALL 将 persistent_topic_id 改写为 20，topic_match_confidence=manual，lane_tier 保持 l1_direct，topic_match_distance=该 section embedding 到 #20 质心的距离

#### Scenario: 同日多条 section 只记一次命中

- **GIVEN** topic #502 在某篇日报内被拆成 3 条 section（bucket_index 0/1/2），当日该话题再无其他 section
- **WHEN** 系统执行生命周期更新
- **THEN** topic #502 的 `hit_count` SHALL 只 +1，`consecutive_hits` SHALL 只 +1，`last_seen_date` SHALL = 该日报日期

#### Scenario: 质心按 section 逐条计入

- **GIVEN** topic #502 在某日报内有 3 条 section（各自 embedding 不同）
- **WHEN** 系统重算该话题质心
- **THEN** 3 条 section 的 embedding SHALL 各自计入 `centroid_window` 窗口，SHALL NOT 按当日合并为一条
