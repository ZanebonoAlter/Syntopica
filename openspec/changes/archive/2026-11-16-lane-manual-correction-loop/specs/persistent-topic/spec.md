# persistent-topic — 首义可人工改写 + 手动解除归属扩展 delta

## MODIFIED Requirements

### Requirement: PersistentTopic 持久化数据模型

系统 SHALL 维护 `board_persistent_topics` 表，持久化每个 board 内的长期叙事话题。每行 SHALL 包含：`semantic_board_id`（所属板块）、`label`（叙事框架标题）、`description`、`embedding`（与 `semantic_labels.embedding` 同维度）、`status`（candidate/active/archived）、`source`（auto/manual）、`first_seen_date`、`last_seen_date`、`hit_count`、`consecutive_hits`。

`status` SHALL 受 CHECK 约束为 candidate/active/archived 三态。`source` SHALL 受 CHECK 约束为 auto/manual 二态，默认 auto（历史数据与自动涌现的 topic 均为 auto）。表 SHALL 在 `(semantic_board_id, status)` 上建 BTree 索引、在 `embedding` 上建 HNSW 索引。

`embedding`（首义向量）SHALL NOT 被算法改写——质心滚动刷新只写 `centroid` 列，SHALL NOT 触碰 `embedding` 列。首义 SHALL 允许**人工显式改写**（用户主权声明），改写 SHALL 留痕于 `topic_anchor_overrides` 追加表（`topic_id`、改写前向量、改写后向量、输入模式、输入摘要、操作者、时间），SHALL NOT 被后续改写覆盖或删除。

表 SHALL 新增 `centroid_pending_recompute`（boolean，非空，默认 false）列，标记该话题的质心需在下一次日报生成时强制重算。标记 SHALL 在该话题质心重算成功后清空，重算失败 SHALL 保留标记以待下次重试。

建表 SHALL 通过显式数据库迁移完成（按开发执行规范 §10），不依赖 gorm AutoMigrate。

#### Scenario: 候选话题创建

- **WHEN** L3 自动新建 candidate topic
- **THEN** 系统 SHALL 写入首义 embedding（首条命中 section 的内容 embedding）、status=candidate、source=auto、hit_count=1、consecutive_hits=1，且 `centroid_pending_recompute` SHALL 为 false

#### Scenario: 话题状态约束

- **WHEN** 尝试写入 status='pending'
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: 来源约束

- **WHEN** 尝试写入 source='imported'
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: 历史 topic 默认来源

- **WHEN** 迁移执行后，历史已存在的 topic 行
- **THEN** 其 source SHALL 为 auto（迁移默认值），不报错

#### Scenario: 待重算标记默认值与迁移

- **WHEN** 迁移新增 `centroid_pending_recompute` 列
- **THEN** 历史 topic 行该列 SHALL 为 false（无需回填，不触发额外重算）

#### Scenario: 首义改写留痕追加

- **GIVEN** topic #7 首义为 E0
- **WHEN** 用户人工改写首义为 E1
- **THEN** `topic_anchor_overrides` SHALL 追加一行（改前 E0、改后 E1、时间、操作者），历史留痕行 SHALL 保持不变

### Requirement: PersistentTopic 质心表示

PersistentTopic 的 embedding 匹配锚点 SHALL 为「首义向量与滚动质心的加权混合」：`centroid = α·首义 embedding + (1-α)·最近 centroid_window 条命中 section embedding 等权均值`。α = `persistent_topic_centroid_anchor_weight`（默认 0.5，ai_settings 可配，见配置项）。manual 话题的首义为创建时所选 section 的均值向量，auto 话题的首义为首条命中 section 的内容 embedding——两者均持久化于 `board_persistent_topics.embedding`。

**首义 SHALL NOT 被算法改写**（算法只写 `centroid` 列）；首义 SHALL 允许**人工显式改写并留痕**（见「PersistentTopic 持久化数据模型」）。质心 SHALL 物化存储于 `board_persistent_topics.centroid` 列（与 embedding 同维度），section 新增或归属变更时增量更新。漂移 SHALL 有界：质心随话题下 section 的实际内容自适应移动（保留内容漂移打破标题回声闭环的能力），但首义以 α 权重持续锚定，滑出窗口的历史 section 不再使锚点脱离话题创建定义——防止宽口径话题质心漂移为「泛新闻磁铁」。

质心刷新范围 SHALL 为「当日 touched 话题」∪「`centroid_pending_recompute=true` 的话题」。后者保证人工修正（重设首义、移除节点、加回节点）在该话题**当日无新内容命中**时同样生效。

首义 embedding 与滚动均值维度不一致、或首义为空/不可解析时，质心 SHALL 回退为纯滚动均值（记警告日志，不阻塞刷新）。历史 section 数 <2 的 topic，质心退化为首义向量（等价于现有 embedding 列语义）。

系统 SHALL 提供存量质心重算入口（`POST /api/daily-reports/recompute-topic-centroids`，可选 board_id 过滤，异步执行、幂等可重跑），按当前公式与配置重算既有话题质心；用于公式升级后的一次性存量纠正。该入口 SHALL 是唯一的质心重算触发方式——系统 SHALL NOT 提供「立即重算某话题质心」的人工触发端点。

section embedding SHALL 基于 section 实际聚合内容生成（见 section-content-embedding 能力）。

#### Scenario: 质心按近期 section 计算

- **GIVEN** topic「Claude Code 工具链崛起」有历史 section 40 条，首义 embedding 为 E0，最近 30 条 section 均值为 M，α=0.5
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL = 0.5·E0 + 0.5·M（按元素加权），centroid_window 之外的 section 不参与 M

#### Scenario: 质心随内容漂移

- **GIVEN** topic「美军机从以色列境内基地起飞对伊朗实施打击」质心原锚定首义方向，连续两日被归因了「阿联酋与伊朗贸易」相关 section（内容 embedding 与首义语义距离约 0.24），α=0.5
- **WHEN** 质心重算
- **THEN** 新质心 SHALL 向阿联酋贸易内容方向移动（内容漂移仍生效、标题回声闭环仍被打破），但移动幅度 SHALL 不超过纯滚动均值位移的 (1-α)，质心到首义的距离 SHALL 保持有界

#### Scenario: section 不足退化首义向量

- **GIVEN** topic 只有 1 条历史 section
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL 等于该 section 的内容 embedding（首义向量语义）

#### Scenario: 质心增量更新

- **GIVEN** 新 section 归属 topic「X」
- **WHEN** section 持久化
- **THEN** topic X 的 centroid SHALL 增量重算（新公式混合，纳入新 section，窗口溢出的旧 section 淘汰）

#### Scenario: 首义不可用时回退纯滚动

- **GIVEN** topic 的 embedding 列为 NULL 或与滚动均值维度不一致，有 5 条可解析历史 section
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL 等于滚动窗口均值（无首义混合），并 SHALL 记录警告日志

#### Scenario: 锚定权重配置边界

- **WHEN** ai_settings 设置 persistent_topic_centroid_anchor_weight=0（或 1）
- **THEN** 质心 SHALL 分别退化为纯滚动均值（旧行为）/ 纯首义（话题冻结）

#### Scenario: 存量质心重算

- **GIVEN** 部署混合公式后，存量话题质心仍为旧公式（纯滚动）产物
- **WHEN** 调用 POST /api/daily-reports/recompute-topic-centroids
- **THEN** 全部（或指定 board）话题质心 SHALL 按新公式与当前配置重算；重复调用 SHALL 幂等（同输入同输出）；单个话题重算失败 SHALL 不中断其余话题

#### Scenario: 待重算标记强制纳入刷新范围

- **GIVEN** topic #7 的 `centroid_pending_recompute=true`，但当日无任何 section 命中 #7
- **WHEN** 当日日报生成后的质心刷新轮执行
- **THEN** #7 SHALL 被纳入刷新范围并按当前首义与滚动窗口重算，SHALL NOT 因不在 touched 集合而漏算

#### Scenario: 重算成功后清标记

- **GIVEN** topic #7 的 `centroid_pending_recompute=true`
- **WHEN** #7 质心重算成功
- **THEN** `centroid_pending_recompute` SHALL 被置为 false

#### Scenario: 重算失败保留标记

- **GIVEN** topic #7 的 `centroid_pending_recompute=true`
- **WHEN** #7 质心重算失败
- **THEN** `centroid_pending_recompute` SHALL 保持 true，SHALL NOT 静默清除，SHALL NOT 中断其余话题的刷新

#### Scenario: 首义不被算法改写

- **GIVEN** topic #7 的 `embedding` 列为 E
- **WHEN** 质心刷新轮执行
- **THEN** 系统 SHALL 只更新 `centroid` 列，SHALL NOT 修改 `embedding` 列

### Requirement: DailyReportSection 强制归属 PersistentTopic

`daily_report_sections` SHALL 包含 `persistent_topic_id`（外键到 board_persistent_topics）、`topic_match_distance`、`topic_match_confidence`、`lane_tier` 四列。`topic_match_confidence` SHALL 取值 anchor_hit / auto_new / unmatched / manual。`lane_tier` SHALL 取值 l1_direct / l2_llm / l3_new，标识该 section 的分桶来源。

归属算法 SHALL 保证每个新生成的 section 归属到恰好 1 个 PersistentTopic，采用三层分桶：

- **L1 直挂（lane_tier=l1_direct）**：当天 tag 经 embedding 质心匹配，到某 **active** topic 质心距离 < `lane_l1_threshold`（默认 0.18）且该 topic 非 vacuum 时，section 直接归属该 topic，topic_match_confidence=anchor_hit，不调用 LLM。**candidate topic 不享有直挂资格**：即使距离 < `lane_l1_threshold`，最近话题为 candidate 时该 tag SHALL 降级进入 L2 band（观察期门禁，`persistent_topic_candidate_l1_gate_enabled` 默认启用；开关关闭时回退为 active/candidate 均可直挂的旧行为）。
- **L2 LLM 裁决（lane_tier=l2_llm）**：质心距离在 [`lane_l1_threshold`, `lane_l2_threshold`]（默认 [0.18, 0.30]）的 tag，或最近话题为 candidate 的近距离 tag，系统 SHALL 将其与 embedding 预筛的 top-K 候选 topic（默认 K=5）一并交 LLM 做「留/换/新」三选一；LLM 选「留」归属候选 topic（confidence=anchor_hit），选「换」归属 LLM 指定 topic（须校验在候选集内，confidence=anchor_hit），选「新」转 L3。**keep 解析尊重显式 target**：LLM 输出「留」但显式携带候选集内另一 target_topic_id 时（小模型常态混用 keep/switch），SHALL 尊重该指定归属而非静默改写回最近候选——否则 keep 会把 tag 无条件吸附回 embedding 最近处（含僵尸 candidate）；target 为空或不在候选集内时归属最近候选（安全网，不降级 new）。
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
