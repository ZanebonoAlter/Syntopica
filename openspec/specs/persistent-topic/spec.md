## Purpose

PersistentTopic（持久叙事话题）：在 SemanticBoard（板块）和 DailyReportSection（当天聚类）之间引入持久叙事话题层。强制 section 1:N 归属、人工确认生命周期、关系叠加身份边，解决板块下话题因命名漂移而散乱、每日聚类随机的根因问题。

## Requirements

### Requirement: PersistentTopic 持久化数据模型

系统 SHALL 维护 `board_persistent_topics` 表，持久化每个 board 内的长期叙事话题。每行 SHALL 包含：`semantic_board_id`（所属板块）、`label`（叙事框架标题）、`description`、`embedding`（与 `semantic_labels.embedding` 同维度）、`status`（candidate/active/archived）、`source`（auto/manual）、`first_seen_date`、`last_seen_date`、`hit_count`、`consecutive_hits`。

`status` SHALL 受 CHECK 约束为 candidate/active/archived 三态。`source` SHALL 受 CHECK 约束为 auto/manual 二态，默认 auto（历史数据与自动涌现的 topic 均为 auto）。表 SHALL 在 `(semantic_board_id, status)` 上建 BTree 索引、在 `embedding` 上建 HNSW 索引。

建表 SHALL 通过显式数据库迁移完成（按开发执行规范 §10），不依赖 gorm AutoMigrate。

#### Scenario: 候选话题创建

- **WHEN** 归属算法判定某 section 无法归属到已有 topic，需新建 PersistentTopic
- **THEN** 系统 SHALL 创建一行，status=candidate，source=auto，first_seen_date=last_seen_date=当天，hit_count=1，consecutive_hits=1

#### Scenario: 话题状态约束

- **WHEN** 尝试写入 status='pending'
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: 来源约束

- **WHEN** 尝试写入 source='imported'
- **THEN** 系统 SHALL 因 CHECK 约束拒绝

#### Scenario: 历史 topic 默认来源

- **WHEN** 迁移执行后，历史已存在的 topic 行
- **THEN** 其 source SHALL 为 auto（迁移默认值），不报错

### Requirement: DailyReportSection 强制归属 PersistentTopic

`daily_report_sections` SHALL 包含 `persistent_topic_id`（外键到 board_persistent_topics）、`topic_match_distance`、`topic_match_confidence`、`lane_tier` 四列。`topic_match_confidence` SHALL 取值 anchor_hit / auto_new / unmatched / manual。`lane_tier` SHALL 取值 l1_direct / l2_llm / l3_new，标识该 section 的分桶来源。

归属算法 SHALL 保证每个新生成的 section 归属到恰好 1 个 PersistentTopic，采用三层分桶：

- **L1 直挂（lane_tier=l1_direct）**：当天 tag 经 embedding 质心匹配，到某 **active** topic 质心距离 < `lane_l1_threshold`（默认 0.18）且该 topic 非 vacuum 时，section 直接归属该 topic，topic_match_confidence=anchor_hit，不调用 LLM。**candidate topic 不享有直挂资格**：即使距离 < `lane_l1_threshold`，最近话题为 candidate 时该 tag SHALL 降级进入 L2 band（观察期门禁，`persistent_topic_candidate_l1_gate_enabled` 默认启用；开关关闭时回退为 active/candidate 均可直挂的旧行为）。
- **L2 LLM 裁决（lane_tier=l2_llm）**：质心距离在 [`lane_l1_threshold`, `lane_l2_threshold`]（默认 [0.18, 0.30]）的 tag，或最近话题为 candidate 的近距离 tag，系统 SHALL 将其与 embedding 预筛的 top-K 候选 topic（默认 K=5）一并交 LLM 做「留/换/新」三选一；LLM 选「留」归属候选 topic（confidence=anchor_hit），选「换」归属 LLM 指定 topic（须校验在候选集内，confidence=anchor_hit），选「新」转 L3。**keep 解析尊重显式 target**：LLM 输出「留」但显式携带候选集内另一 target_topic_id 时（小模型常态混用 keep/switch），SHALL 尊重该指定归属而非静默改写回最近候选——否则 keep 会把 tag 无条件吸附回 embedding 最近处（含僵尸 candidate）；target 为空或不在候选集内时归属最近候选（安全网，不降级 new）。
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

系统 SHALL 在 ai_settings 新增以下配置项（含默认值）：
- `persistent_topic_match_threshold`：0.30（section 命中 topic 的 embedding distance 上限）
- `persistent_topic_upgrade_threshold`：3（candidate 连续命中天数转 active）
- `persistent_topic_decay_window`：30（active 无命中转 archived 的天数）
- `persistent_topic_cluster_threshold`：0.30（回刷时历史 section 聚类成 topic 的阈值）

#### Scenario: 读取默认配置

- **WHEN** ai_settings 未设置上述 key
- **THEN** 系统 SHALL 使用默认值

#### Scenario: 自定义升级阈值

- **WHEN** ai_settings 设置 persistent_topic_upgrade_threshold=5
- **THEN** candidate 连续命中 5 天才转 active

### Requirement: 手动建持久化话题

系统 SHALL 提供手动创建持久化话题的能力：用户提交 `label` 与一组 `section_ids`，系统在单个事务内创建新 PersistentTopic 并将选中 section 归属改写到该话题。

创建流程 SHALL：
1. 读取选中 section 的 embedding（维度缺失的 section 跳过并返回提示）。
2. 聚合为 mean pooling 平均向量作为新话题 embedding。
3. `CreateTopic`：status=active、source=manual、embedding=聚合向量、first_seen_date=min(选中 section period_date)、last_seen_date=max(...)、hit_count=选中数、consecutive_hits=选中数。
4. 对每个选中 section 调用归属改写（persistent_topic_id=新话题、confidence=manual、distance=该 section 到聚合锚点距离）。
5. 事务末尾触发 `RebuildBoardRelations`（幂等重建该 board 关系，重算 identity 边，保证血统一致）。

手动话题 SHALL 跳过 `upgrade_threshold` 连续命中门禁（自动路径的 candidate→active 门禁对本路径不生效），因用户主权声明且已有选中内容支撑。

手动话题 SHALL NOT 调用任何 LLM（embedding 聚合为纯向量运算）。

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

PersistentTopic 的 embedding 匹配锚点 SHALL 为「历史 section embedding 质心」，而非首条 section 标题继承的首义向量。质心 SHALL 按 `centroid_window`（默认最近 30 条 section，ai_settings 可配）加权计算，section 新增或归属变更时增量更新。质心 SHALL 物化存储于 `board_persistent_topics.centroid` 列（与 embedding 同维度），首次部署按历史 section embedding 离线构建一次。

section embedding SHALL 基于 section 实际聚合内容生成（见 section-content-embedding 能力），因此质心 SHALL 随话题下 section 的实际内容漂移：当被归因的 section 内容偏离话题标题语义时，质心 SHALL 被拉向实际内容方向，使后续与该内容无关的 tag 匹配距离增大，脱离 L1/L2 匹配带。

新建 candidate 话题的首义 embedding（`board_persistent_topics.embedding`）SHALL 取该 section 的内容化 embedding（L3 归因时 section embedding 即内容向量），不再继承标题文本向量。

历史 section 数 < 2 的 topic，质心退化为首义向量（等价于现有 embedding 列语义）。

#### Scenario: 质心按近期 section 计算

- **GIVEN** topic「Claude Code 工具链崛起」有历史 section 40 条
- **WHEN** 系统计算其质心
- **THEN** 系统 SHALL 取最近 30 条 section embedding 加权平均作为 centroid，centroid_window 之外的 section 不参与

#### Scenario: 质心随内容漂移

- **GIVEN** topic「美军机从以色列境内基地起飞对伊朗实施打击」质心原锚定标题语义，连续两日被归因了「阿联酋与伊朗贸易」相关 section（内容 embedding 与标题语义距离约 0.24）
- **WHEN** 质心重算
- **THEN** 新质心 SHALL 向阿联酋贸易内容方向移动，后续「美军机打击伊朗」类 tag 到该质心的距离 SHALL 大于原值（标题回声闭环被打破）

#### Scenario: section 不足退化首义向量

- **GIVEN** topic 只有 1 条历史 section
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL 等于该 section 的内容 embedding（首义向量语义）

#### Scenario: 质心增量更新

- **GIVEN** 新 section 归属 topic「X」
- **WHEN** section 持久化
- **THEN** topic X 的 centroid SHALL 增量重算（纳入新 section，窗口溢出的旧 section 淘汰）

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
