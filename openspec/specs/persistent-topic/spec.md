## Purpose

PersistentTopic（持久叙事话题）：在 SemanticBoard（板块）和 DailyReportSection（当天聚类）之间引入持久叙事话题层。强制 section 1:N 归属、人工确认生命周期、关系叠加身份边，解决板块下话题因命名漂移而散乱、每日聚类随机的根因问题。

## Requirements

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

### Requirement: ClusterTags 注入历史叙事框架

`ClusterTags` SHALL 仅处理 L2 弱区 tag 与 L3 新叙事 tag，不再对全部当天 tag 做自由聚类。

- 对 **L2 tag**，`ClusterTags` SHALL 查询该 board 下所有 active 与 candidate 状态 PersistentTopic 的质心，embedding 预筛 top-K 候选 topic（连同其近期 section 摘要）注入 LLM prompt，指示 LLM 在候选集内做「留/换/新」三选一。**近期 section 摘要 SHALL 以该 section 当天的实际 tag 标签（事实指纹）为内容**，SHALL NOT 以 topic label 或被 topic label 覆盖的 section 标题作为"近期内容"信号（该信号为零信息量复读）；**注入范围 SHALL 覆盖 active 与 candidate 两类话题**（candidate 流经 L2 裁决，需要内容供判断）；**注入窗口 SHALL 排除当日 sections**（`period_date < today`）——同日重跑时当日早前运行挂错的 tag 会作为"近期内容"证据洗白错挂（自证回路），次日运行昨日 section 才作为证据。briefs 查询失败时 SHALL 降级为仅注入 label 与命中元数据，不阻塞聚类。
- 对 **L3 tag**，`ClusterTags` SHALL 指示 LLM 为无法归属的 tag 起新叙事标题（开新组）。

LLM 输出 schema：L2 group 含 `decision`(keep/switch/new) + `target_topic_id`(keep/switch 时必填)；L3 group 含 `group_name`。`ClusterTags` SHALL 校验 `target_topic_id` 存在于传入候选集，非法值降级为 new。

#### Scenario: L2 候选预筛注入

- **GIVEN** L2 弱区 tag「OpenAI 模型入侵 HuggingFace」，board 有 active topic「大模型监管与安全」
- **WHEN** 系统构建 L2 LLM prompt
- **THEN** prompt SHALL 含 top-K 候选 topic（含「大模型监管与安全」）及其近期 section 的实际 tag 标签，要求 LLM 输出 keep/switch/new

#### Scenario: candidate 话题近期内容注入

- **GIVEN** candidate topic「伊朗政局」近 3 天各有 section 归属，section 携带 tag 标签「伊朗代防长强硬言论」「中伊海湾形势会谈」
- **WHEN** 某 tag 因观察期门禁降级进入 L2 裁决且该 candidate 在其候选集内
- **THEN** prompt 中该 candidate 的近期内容 SHALL 呈现上述实际 tag 标签而非 candidate label 复读

#### Scenario: briefs 排除当日 section（同日重跑防自证）

- **GIVEN** 话题 T 今日早前运行（或同日重跑前）被误挂了 section（period_date=今日，携带 tag「今日误挂」），昨日正常归属 section（period_date=昨日，携带 tag「昨日事实」）
- **WHEN** 当日再次生成日报时加载 T 的近期 briefs
- **THEN** 注入内容 SHALL 仅含昨日 section 的 tag 标签，SHALL NOT 含今日 section——防止当日误挂自我印证为"有内容支撑"

#### Scenario: briefs 查询失败降级 label-only

- **WHEN** 近期 section 摘要查询失败（DB 异常）
- **THEN** 系统 SHALL 仅注入候选 topic 的 label 与命中元数据（状态/最近命中/累计天数/距离）继续 L2 裁决，SHALL NOT 使当日日报生成失败

#### Scenario: L3 新叙事起标题

- **GIVEN** L3 tag 集合无法归属任何现有 topic
- **WHEN** 系统执行 ClusterTags L3 分支
- **THEN** LLM SHALL 为其起新叙事标题，系统据此新建 candidate topic

### Requirement: PersistentTopic 自动升级生命周期

系统 SHALL 在每日报生成并归属后，执行 topic 状态机更新：

- 当天有 section 归属到某 topic：consecutive_hits += 1，hit_count += 1，last_seen_date = 当天；若 status=candidate 且 consecutive_hits ≥ upgrade_threshold（默认 3），SHALL 获得人工确认资格，但 SHALL NOT 自动转为 active。
- 当天无 section 归属：consecutive_hits 归 0；若 status=active 且 today - last_seen_date > decay_window（默认 30 天），SHALL 自动转为 archived。

upgrade_threshold、decay_window SHALL 可通过 ai_settings 配置。

#### Scenario: 候选连续命中后等待人工确认

- **GIVEN** topic #12 status=candidate，consecutive_hits=2
- **WHEN** 第 3 天有 section 归属到 #12
- **THEN** #12 SHALL 保持 candidate，consecutive_hits=3，hit_count+1，并允许用户确认启用

#### Scenario: 未达多天门禁时拒绝确认

- **GIVEN** topic #12 status=candidate，consecutive_hits=2，upgrade_threshold=3
- **WHEN** 用户尝试将 #12 更新为 active
- **THEN** 系统 SHALL 拒绝，并返回连续出现天数不足的错误

#### Scenario: 人工确认后进入持久泳道

- **GIVEN** topic #12 status=candidate，consecutive_hits=3
- **WHEN** 用户确认启用 #12
- **THEN** #12 SHALL 转为 active；只有 active topic SHALL 形成持久话题泳道

#### Scenario: 连续命中中断重置

- **GIVEN** topic #12 consecutive_hits=2
- **WHEN** 第 3 天无 section 归属到 #12
- **THEN** #12 consecutive_hits SHALL 归 0，status 保持 candidate

#### Scenario: 正式话题超期归档

- **GIVEN** topic #18 status=active，last_seen_date=2026-05-15，当前日期 2026-06-19（间隔 35 天 > decay_window 30）
- **WHEN** 执行状态机更新
- **THEN** #18 SHALL 转为 archived

#### Scenario: 正式话题窗口内保留

- **GIVEN** topic #18 status=active，last_seen_date=2026-06-05，当前日期 2026-06-19（间隔 14 天 < decay_window 30）
- **WHEN** 执行状态机更新
- **THEN** #18 status SHALL 保持 active

### Requirement: Section 关系叠加身份边

`daily_report_section_relations` SHALL 新增 `relation_type` 列（identity/similarity，默认 similarity）。匈牙利二分匹配产出的关系 SHALL 标记为 similarity（算法实现不动）。

section 的 emerging/continuing/split/merge/ending 状态 SHALL 只由 similarity 关系推导，identity 关系 SHALL NOT 改变时间线状态。

关系重建 SHALL 额外写入身份边：对每个 PersistentTopic 下相邻天（按 period_date）的两个 section，SHALL 写入或覆盖一条 relation_type=identity 的关系。身份边 SHALL NOT 受匈牙利 penalty（0.28）限制——即使两 section embedding distance > penalty，只要归属同一 topic 即写边。

当同一 (from_id, to_id) 同时存在 similarity 与 identity 关系时，SHALL 以 identity 为准（覆盖）。

#### Scenario: 命名漂移不断链

- **GIVEN** Day1 section 归属 topic #12，Day2 section 归属 topic #12，但两者 embedding distance=0.32 > penalty 0.28
- **WHEN** 关系重建
- **THEN** 系统 SHALL 写入 (Day1, Day2, distance=0.32, relation_type=identity)，DAG 上两节点连通

#### Scenario: 不同话题不写身份边

- **GIVEN** Day1 section 归属 topic #12，Day2 section 归属 topic #18
- **WHEN** 关系重建
- **THEN** 系统 SHALL NOT 写入 identity 关系（仅由匈牙利 similarity 边决定是否连通）

### Requirement: 话题生命线 API

`GET /api/daily-reports/topics/:id/lifeline` SHALL 返回该 PersistentTopic 下的全部 section（不限天数，按 `persistent_topic_id` 聚合）及内部关系。

响应 SHALL 包含 topic 元信息（label/status/first_seen_date/last_seen_date/hit_count）和 sections 列表（含 period_date/cluster_label/status/article_count/thread_count）。relations SHALL 只包含两端 section 都属于该 topic 的关系。

相比 `getSectionLifecycle`，本 API SHALL NOT 依赖 embedding 连通性，直接按 persistent_topic_id 聚合。

响应 SHALL 向后兼容地提供可选已确认关键节点列表，包含确认快照、来源日报日期、依据和来源可用状态。节点 SHALL 不占用真实 section ID、不改变 identity/similarity 关系；无真实 section 但有已确认快照时 SHALL 仍可读取节点。无节点时返回空集合，不自动启用/归档话题。

#### Scenario: 查询话题完整生命线
- **WHEN** topic #12 下有 4 天的 section（#40/#50/#60/#70）
- **THEN** 响应 SHALL 返回这 4 个 section 及其内部 identity/similarity 关系

#### Scenario: 话题跨命名漂移聚合
- **GIVEN** topic #12 下 section 标签依次为 "AI 编程竞争"/"开发者生态重构"/"AI 工具内卷"
- **WHEN** 查询 lifeline
- **THEN** 响应 SHALL 返回全部 3 个 section（即使 embedding 两两 distance 部分超过 penalty）

#### Scenario: 无 section 仍能查确认快照
- **WHEN** 原 section 被替换且该泳道保有确认快照
- **THEN** 生命周期响应返回快照，不伪造 section 或关系，不自动改变话题状态

### Requirement: 现有 API 向后兼容扩展

`getBoardSectionTimeline` 和 `getSectionLifecycle` 响应 SHALL 增加可选字段：每个 section 嵌套 `persistent_topic`（id/label/status/color）、每条 relation 增加 `relation_type`。新字段 SHALL 全部 optional，老前端忽略不报错。

`persistent_topic.color` SHALL 由后端按 topic_id 哈希分配稳定色，避免前端刷新跳色。

#### Scenario: 老客户端兼容

- **WHEN** 老前端调用 getBoardSectionTimeline，不读取新字段
- **THEN** 系统 SHALL 正常返回响应，前端 SHALL 正常渲染（忽略新字段）

#### Scenario: 主题色稳定

- **WHEN** 同一 topic #12 连续两次查询
- **THEN** 两次返回的 persistent_topic.color SHALL 一致

### Requirement: 历史数据回刷

系统 SHALL 提供 `BackfillPersistentTopics(boardID)`，从该 board 历史 section 用 average_link 聚类反推 PersistentTopic（默认 cluster_threshold=0.30）。回刷产出的 topic SHALL 直接给 active 状态（因已有历史命中支撑）。

回刷 SHALL 回填所有历史 section 的 persistent_topic_id。

回刷 SHALL 在 SaveReport 之后、RebuildBoardRelations 之前执行（关系重建需要 topic_id 写身份边）。

#### Scenario: 回刷历史 section 归属

- **GIVEN** board 有 30 天历史 section，无任何 PersistentTopic
- **WHEN** 执行 BackfillPersistentTopics
- **THEN** 系统 SHALL 产出 active topic 集合（目标单 board 5-15 个），全部历史 section 的 persistent_topic_id 被回填

#### Scenario: 回刷后关系含身份边

- **WHEN** 回刷后执行 RebuildBoardRelations
- **THEN** 同 topic 相邻天 section SHALL 写入 identity 关系

### Requirement: 配置项

系统 SHALL 在 ai_settings 新增/维护以下配置项（含默认值；α 为线性混合权重，匹配走 cosine 角度，实际锚定效果因滚动均值模长收缩而略强于标称比例）：
- `persistent_topic_match_threshold`：0.30（section 命中 topic 的 embedding distance 上限）
- `persistent_topic_upgrade_threshold`：3（candidate 连续命中天数转 active）
- `persistent_topic_decay_window`：30（active 无命中转 archived 的天数；迁移种子遗留 key，代码实际读 `persistent_topic_candidate_decay_window`，仅作 prompt 候选过滤）
- `persistent_topic_cluster_threshold`：0.30（回刷时历史 section 聚类成 topic 的阈值）
- `persistent_topic_centroid_anchor_weight`：0.5（质心混合的首义权重 α；0=纯滚动旧行为，1=纯首义冻结；非法值回退默认并告警）

#### Scenario: 读取默认配置

- **WHEN** ai_settings 未设置上述 key
- **THEN** 系统 SHALL 使用默认值

#### Scenario: 自定义升级阈值

- **WHEN** ai_settings 设置 persistent_topic_upgrade_threshold=5
- **THEN** candidate 连续命中 5 天才转 active

#### Scenario: 自定义锚定权重

- **WHEN** ai_settings 设置 persistent_topic_centroid_anchor_weight=0.3
- **THEN** 质心混合 SHALL 采用 α=0.3（首义 30% + 滚动均值 70%）

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

### Requirement: 手动话题参与自动归属

`source=manual` 且 `status=active` 的话题 SHALL 与 `source=auto` 的 active 话题在后续日报生成中一视同仁地参与 embedding AND-gate 自动归属：`ListAnchorableTopicsByBoard` SHALL 包含手动 active 话题，其聚合 embedding 作为最近邻锚点。

手动话题 SHALL NOT 因 source=manual 而被排除出归属候选集，也 SHALL NOT 获得任何特殊阈值放宽/收紧（与自动 active 话题共用同一 match_threshold）。

#### Scenario: 手动话题次期被自动命中

- **GIVEN** 手动建 topic #20（source=manual, active, embedding=V），次日新 section #300 embedding 到 V 距离=0.22 ≤ match_threshold
- **WHEN** 日报生成执行 planTopicAssignments
- **THEN** #300 SHALL 被 AND-gate 判定归属到 #20（若 LLM 也认同），confidence=anchor_hit

#### Scenario: 手动话题不享特殊阈值

- **GIVEN** 手动 topic #20，match_threshold=0.30
- **WHEN** 某 section 到 #20 距离=0.35
- **THEN** 系统 SHALL 按统一阈值判定不归属（SHALL NOT 因 #20 是手动而放宽）

### Requirement: 回刷重建持久化话题（单成员不建）

`BackfillPersistentTopics` 为历史未归属 section 重建持久化话题时，SHALL 采用 complete-link 聚类（一个 section 仅当与某 cluster 的**每个**成员距离 ≤ ClusterThreshold 时才加入，否则 seed 新 cluster），并对每个达到规模门槛的 cluster 直接创建 ACTIVE 话题。

**规模门槛**：只有成员数 ≥ 2 的 cluster SHALL 建话题。成员数为 1 的孤立 cluster（一条 section 跟谁都不挨着）SHALL NOT 建话题，其 section SHALL 保持 `persistent_topic_id IS NULL`（unassigned），留给日报生成路径按连续命中观察期（candidate→active）自然涌现。理由：单次出现的孤立 section 不构成「持久叙事」证据，直接建 ACTIVE 话题会绕过连续多天观察期、产生噪音泳道。

达到门槛的 cluster 创建话题时，`status=active`、`source=auto`、`hit_count=成员数`、`consecutive_hits=成员数`（历史成员即历史证据）。

回刷 SHALL NOT 调用任何 LLM（纯聚类 + 向量运算）。回刷事务末尾 SHALL 触发 `RebuildBoardRelations`（失败仅告警、不回滚，与既有行为一致）。

#### Scenario: 单成员 cluster 不建话题

- **GIVEN** board 有两条正交 section（彼此距离 > ClusterThreshold），各自无法与对方聚成 cluster
- **WHEN** 执行 BackfillPersistentTopics
- **THEN** 系统 SHALL 创建 0 个话题，两条 section SHALL 保持 persistent_topic_id IS NULL

#### Scenario: 多成员 cluster 直接建 active

- **GIVEN** board 有 3 条相互距离 ≤ ClusterThreshold 的 section（同一叙事）
- **WHEN** 执行 BackfillPersistentTopics
- **THEN** 系统 SHALL 创建 1 个 status=active、source=auto、hit_count=3、consecutive_hits=3 的话题，3 条 section 归属到该话题

#### Scenario: 孤立 section 留给日报路径涌现

- **GIVEN** 回刷后 section #9 保持 unassigned（孤立单成员）
- **WHEN** 后续日报生成执行 planTopicAssignments 且 #9 仍无法归属已有话题
- **THEN** 系统 SHALL 按日报路径为 #9 开 candidate（hit_count=1），需累计命中 upgrade_threshold 次才能转 active

### Requirement: 候选激活门禁（累计命中口径）

candidate→active 的激活门禁 SHALL 基于**累计命中次数**（`hit_count`），而非连续命中天数（`consecutive_hits`）。`hit_count` 每次归属命中 +1、只增不减；`consecutive_hits` 仍记录"最近连续命中期数"（miss 时 reset 0）但不再作为门禁。

凡"候选是否达标"的判定 SHALL 统一用 `hit_count >= upgrade_threshold`：
- `can_activate`（话题列表响应）= `status=candidate && hit_count >= upgrade_threshold`。
- `FilterVisibleTopics`（管理 UI 可见性）：候选仅当 `hit_count >= upgrade_threshold` 时可见，其余 observing 候选隐藏。
- `UpdateTopic` candidate→active 校验：`hit_count < upgrade_threshold` 拒绝并提示"需累计命中至少 N 次"。
- `PruneUnderqualifiedCandidates`（一次性清理迁移）：删除 `hit_count < upgrade_threshold` 的候选。

理由：话题天然有间隔（今日涌现、隔日中断、后日重现），严格"连续每期命中"在真实数据中极难达到（实测最多连续 1 期），导致候选永远无法达标转正。改用累计命中后，断断续续出现的话题也能在累计够次数后转正。

`PersistentTopicBrief`（timeline 节点的话题摘要）SHALL 含 `hit_count` 字段，供前端展示"累计命中 N 次"。

#### Scenario: 累计命中达标可激活

- **GIVEN** upgrade_threshold=3，候选 #5 历史命中过 3 期（hit_count=3）但最近一期 miss（consecutive_hits=0）
- **THEN** #5 SHALL can_activate=true（累计达标），SHALL 可被人工确认转 active

#### Scenario: 连续命中高但累计不足不可激活

- **GIVEN** 候选 #6 consecutive_hits=2（最近连续 2 期）但 hit_count=2（< 3）
- **THEN** #6 SHALL can_activate=false（累计不足），SHALL NOT 可激活

#### Scenario: 清理迁移按累计口径

- **GIVEN** 一次性清理迁移执行，候选 #7 hit_count=1（< 3）但 consecutive_hits=1
- **THEN** #7 SHALL 被 PruneUnderqualifiedCandidates 删除（累计不足）

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

### Requirement: 吸尘器 topic 检测

系统 SHALL 对每个 active/candidate topic 维护「吸引统计」：近 `vacuum_window`（默认 7 天）内以该 topic 为质心最近邻的当天 tag 数（attracted），其中质心距离 < `lane_l1_threshold` 的计为 strong、落在 [`lane_l1_threshold`, `lane_l2_threshold`] 的计为 mid。`strong/(strong+mid) < vacuum_ratio`（默认 0.20）的 topic 标记 `is_vacuum=true`。

挂到 `is_vacuum=true` topic 的 tag SHALL 降级到 L2（即使质心距离 < `lane_l1_threshold`），交 LLM 裁决，避免过宽质心误吸。

#### Scenario: 吸尘器 topic 识别

- **GIVEN** topic「中国中央银行相关新闻」近 7 天 attracted=17、strong=0、mid=11
- **WHEN** 系统计算 vacuum 标记
- **THEN** strong/(strong+mid)=0 < 0.20，系统 SHALL 标记 is_vacuum=true

#### Scenario: 吸尘器 topic 的强挂 tag 降级

- **GIVEN** topic「中国央行新闻」is_vacuum=true，当天 tag「WAIC 主旨讲话」到其质心距离 0.15（本应 L1 直挂）
- **WHEN** 系统分桶
- **THEN** 该 tag SHALL 降级到 L2 交 LLM 裁决，不直接 L1 直挂

### Requirement: 当前泳道关键节点高亮

既有泳道 SHALL 在当前选中版块范围内，以来源日报日期从左到右定位节点，普通动态作背景、人工确认节点醒目高亮；未确认AI建议只在日报展示，总览不得出现建议徽标、理由或挑选动作，建议所对应的真实section仍是普通新闻背景。总览可展示本版块多条泳道，进入既有focus后 SHALL 只展示选中泳道，不能将所有版块的话题混到同一时间轴。系统 SHALL 提供突出关键节点模式，弱化普通背景但不删除普通节点、移动时间位置或把内容改成关键事件列表。确认注记 SHALL 贴合对应来源节点的生命线基线，不排成独立“已标记”轴。同一section有多条已确认thread时 MAY 以节点旁的星标数量入口紧凑展示，数量 SHALL 来自该来源的独立confirmed marker，展开后 SHALL 逐条回看/取消，不把整章标为关键或合并标记身份；无真实来源的快照 SHALL 用同轴独立快照标识而非伪造section。同日节点顺序 SHALL 稳定。日期范围复用现有选项；当前窗口无确认节点 SHALL 提示并允许关闭强调，背景仍可查看。高亮 SHALL 不更改真实动态计数、图关系或人工选择；切换泳道 SHALL 重置局部强调，迟到响应 SHALL 不串入新泳道。

#### Scenario: 高亮保留背景但不混入他泳道
- **WHEN** 已聚焦的当前泳道窗口有确认节点、未确认建议以及另一泳道节点
- **THEN** 高亮只来自本泳道人工确认，普通及建议所对应的来源新闻仍为可达背景，但不展示建议状态/理由/挑选入口，他泳道不混入，时间位置和真实动态计数不变

#### Scenario: 取消最后节点仍保留时间背景
- **WHEN** 在突出模式中成功取消最后一个节点
- **THEN** 提示当前窗口无确认节点，原新闻与日期仍可查看并能关闭强调；失效快照不伪装成真实动态

#### Scenario: 切换泳道不串状态
- **WHEN** 前一泳道请求未返回而用户切到新泳道
- **THEN** 新泳道强调重置，迟到响应不覆盖新泳道状态

### Requirement: 泳道节点可读性与就地详情

泳道 SHALL 保持可读日期与节点标题，不将长跨度压成无法辨识的小点。同日多个条目 SHALL 分别可选并避开标题/命中区遮挡；常规标题换行，超长标题以省略号呈现且完整文本在可访问名称、title或就地详情可达。点击/键盘选择 SHALL 展示对应条目原位详情而非直接确认；横向滚动 SHALL 限于轴容器，不造成页面级横向溢出，窄屏入口 SHALL 可触摸。

#### Scenario: 点击节点读详情但不标记
- **WHEN** 用户点击或键盘选择未确认的节点
- **THEN** 原位详情显示匹配的完整标题、依据和有效来源日报导航；总览不提供新确认/不采纳，已有确认可取消维护，状态不因选择改变、不自动研究、不新开弹窗

#### Scenario: 同日标题错开且长文本可读
- **WHEN** 同日有多个事件，含一条超长标题
- **THEN** 每条入口与标题分别可达且不遮挡；超长内容可从就地详情读取，不能只硬裁标签

#### Scenario: 窄屏长轴局部滚动
- **WHEN** 375px宽视口查看长时间窗口并选择节点
- **THEN** 轴局部横向滚动、页面不横向溢出，详情单栏，节点可触摸并可键盘选择

### Requirement: 版块内泳道边界与既有主题

优化 SHALL 复用叙事工坊的版块选择、话题总览的lanes/focus入口；总览的泳道、section和关键注记 SHALL 限于selectedBoardId。当前版块身份 SHALL 在上下文与详情可辨；切换版块 SHALL 清理局部选择/强调，前版块迟到结果 SHALL NOT 覆盖新版块。全部入口 SHALL 保持文章池语义，不能新增跨版块全局泳道。

editorial/dark SHALL 复用main.css既有语义Token，不另定义绿色品牌配色。关键注记 SHALL 使用独立外环/星标，不覆盖原statusFill的emerging/continuing/split/merge/ending颜色或回写状态。普通节点标题 SHALL 继续来自section.cluster_label；关键文字来自thread/确认快照并作为独立注记，不能回写原section标题。真实section ID、关系和动态计数 SHALL 保留；失效快照 SHALL 不伪造section。版块侧栏标签数 SHALL 继续来自tag_count，不能被关键数量替代。

#### Scenario: 总览只展示选中版块泳道
- **WHEN** 用户选择AI版块，系统另有住房版块
- **THEN** 总览只含AI版块的泳道/动态/注记，详情标明该版块与泳道；点泳道进入原focus，返回仍是AI版块

#### Scenario: 切换版块迟到结果不串入
- **WHEN** 用户在AI版块的请求未返回时切至住房版块
- **THEN** 新版块清理局部选择/强调，迟到结果不改变住房内容或提示，AI版块合法人工选择仍按原owner保存

#### Scenario: 双主题高亮不覆盖原状态语义
- **WHEN** 用户切换editorial/dark并确认或取消关键注记
- **THEN** 配色沿既有主题Token，原section状态/状态色/标题/身份与真实动态计数不变，侧栏tag_count不变，只有人工注记层更新；注记与原节点同轴，同章多条以星标数量展开且各自可取消

#### Scenario: 全部入口不汇总所有泳道
- **WHEN** 用户选择侧栏全部而未选定版块
- **THEN** 保持文章池语义，不展示全局泳道或让跨版块注记进入同一时间轴

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
