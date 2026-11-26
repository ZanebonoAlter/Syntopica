<!-- domain: semantic-board, topic-graph -->

# lane-manual-correction Specification

## Purpose
TBD - created by archiving change lane-manual-correction-loop. Update Purpose after archive.

## Requirements

### Requirement: 人工重设泳道首义

系统 SHALL 提供人工重设 PersistentTopic 首义向量（`board_persistent_topics.embedding`）的能力。首义是话题的「定义面」，是质心公式 `centroid = α·首义 + (1-α)·滚动均值` 中承载话题身份的那一半。

首义 SHALL 通过**手动选择话题节点范围**来设定：用户选定一组 section（节点），系统 SHALL 按 mean pooling 聚合其 embedding 作为新首义（与既有手动建话题的聚合口径一致）。首义 SHALL NOT 通过自然语言描述设定——用户对「这条泳道是什么」的描述，其可操作的形态就是「哪些节点定义它」，画布上 SHALL 看得见当时选了哪些节点。

写入端点 SHALL 为 `PATCH /api/daily-reports/topics/:id/anchor`，请求体 SHALL 携带 `section_ids`（选定节点的 id 列表）。响应 SHALL 返回 `topic_id`、新首义来源（`selected_sections`）、该话题的人工改写次数、以及生效时机标记 `next_report`。

配套读端点 SHALL 为 `GET /api/daily-reports/topics/:id/anchor`（纯读，不产生任何写入）。响应 SHALL 为**向后兼容扩展**：既有字段（`topic_id` / `source` / `section_ids` / `override_count` / `effective_from`）SHALL 全部保留，新增字段 SHALL 只增不改。


`anchor_origin.resolved` SHALL 为布尔且语义唯一：`true` 表示来源节点**可考**，SHALL 同时返回该节点的 `section_id` / `title` / `period_date`；`false` 表示来源**不可考**，节点字段 SHALL 为 `null`，SHALL NOT 用「最早成员」等推测值冒充（沿用「自动首义无节点集不得虚构」的既有契约）。`resolved=false` 是**最终形态而非占位**。

`centroid` 段 SHALL 描述**当前质心的口径与输入**：窗口大小 `window_size`（配置默认值 30）、首义权重 `anchor_weight`（α）、参与滚动计算的 section 集合（`section_ids` 与只读明细 `members[]`）、以及退化标记 `degraded`。`degraded` 为真表示当前质心**不是正常混合口径**：可解析向量不足 2 条时退回首义向量，首义不可用时退化为纯滚动均值——两种分支的落点不同，因此 `degraded` SHALL 是单一布尔退化标记，前端文案 SHALL NOT 把它断言为「退回首义向量」。`degraded` 为真时前端 SHALL 显示退化结论而非空白列表。

注：watch 物化话题在库中 `source=manual` 且无 override 行，读端点按 `auto` 返回（建题语义：系统写入的初始定义）；watch 话题被人工重设后转 `manual`，检索句锚被 override 覆盖。

**读端点 SHALL 零写副作用**：SHALL NOT 写任何表、SHALL NOT 修改 `centroid` / `embedding` 列、SHALL NOT 置 `centroid_pending_recompute`、SHALL NOT 触发质心重算、SHALL NOT 回填任何历史快照（读首义不得改变话题状态）。

被选定的 section SHALL 满足：属于同一 board、当前归属该话题、embedding 存在且维度一致。任一条不满足 SHALL 拒绝该次写入（整体不落库）并给出可读原因。

**算法 SHALL NOT 改写首义**——首义列只允许人工显式写入；质心滚动刷新 SHALL 只写 `centroid` 列，SHALL NOT 触碰 `embedding` 列。

重设首义 SHALL NOT 在写入事务内立即重算质心。写入后系统 SHALL 将该话题标记为待重算（`centroid_pending_recompute=true`），由下一次日报生成统一重算——修正只影响往后，SHALL NOT 改写任何已有输出。

改写 SHALL 留痕：系统 SHALL 在 `topic_anchor_overrides` 表追加一行，记录 `topic_id`、改写前向量、改写后向量、输入模式（`selected_sections`）、输入摘要（选定的 section id 列表）、操作者、时间。留痕 SHALL 只追加，SHALL NOT 被后续改写覆盖或删除。

#### Scenario: 选定节点范围重设首义

- **GIVEN** topic #7「Claude Code 工具链」当前归属 12 条 section，用户选定其中 3 条（#101/#105/#110），三者 embedding 聚合得向量 V
- **WHEN** 用户提交重设首义（`section_ids=[101,105,110]`）
- **THEN** 系统 SHALL 将 `board_persistent_topics.embedding` 写为 V，SHALL 在 `topic_anchor_overrides` 追加一行留痕（记录改前值、改后值 V、section id 列表、操作者、时间），SHALL 将该话题标记 `centroid_pending_recompute=true`，SHALL NOT 在本事务内改写 `centroid` 列

#### Scenario: 首义不接受自然语言输入

- **WHEN** 用户提交的重设首义请求体携带的是自然语言描述而非 `section_ids`
- **THEN** 系统 SHALL 拒绝并返回 4xx（首义只能由选定节点范围定义），SHALL NOT 将明文字符串向量化后写入向量列

#### Scenario: 选定节点跨话题时拒绝

- **GIVEN** topic #7 归属 section #101（board 1980），用户提交的 `section_ids` 含 #777（归属 board 1999 的另一话题）
- **WHEN** 系统执行重设首义
- **THEN** 系统 SHALL 拒绝并返回 4xx（选中节点必须属本话题），SHALL NOT 写入任何部分结果

#### Scenario: 选定节点无向量时拒绝

- **GIVEN** 用户提交的 `section_ids` 含一条 embedding 为空的 section
- **WHEN** 系统执行重设首义
- **THEN** 系统 SHALL 拒绝并返回 4xx 并指明该节点，SHALL NOT 静默跳过（静默跳过会让用户以为选中的 N 个节点全部生效）

#### Scenario: 写入后质心次日生效

- **GIVEN** topic #7 的 `centroid` 列当前为 C_old（由旧首义算出），且 #7 已被标记 `centroid_pending_recompute=true`
- **WHEN** 下一次日报生成完成、post-commit 质心刷新轮处理到 #7
- **THEN** #7 的 `centroid` SHALL 按新首义与当时的滚动窗口均值重算为 C_new，SHALL 清空 `centroid_pending_recompute` 标记；C_old 在此之前 SHALL 保持不变

#### Scenario: 无新内容命中也仍会重算

- **GIVEN** topic #7 被标记 `centroid_pending_recompute=true`，但次日日报没有任何 section 命中 #7（#7 不进入当日 touched 集合）
- **WHEN** 下一次日报生成完成
- **THEN** #7 SHALL 仍被纳入本次质心刷新范围并按新首义重算（待重算标记 SHALL 独立于 touched 集合触发纳入），SHALL NOT 停留在旧质心上

#### Scenario: 重算失败保留待重算标记

- **GIVEN** topic #7 被标记 `centroid_pending_recompute=true`
- **WHEN** 质心刷新轮处理 #7 时计算失败
- **THEN** 系统 SHALL 保留 #7 的 `centroid_pending_recompute=true`（下次继续重试），SHALL NOT 静默清除标记，SHALL NOT 中断其余话题的刷新

#### Scenario: 首义列不被算法改写

- **GIVEN** topic #7 的 `embedding` 列值为 E
- **WHEN** 系统执行质心滚动刷新
- **THEN** 系统 SHALL 只更新 `centroid` 列，SHALL NOT 修改 `embedding` 列的值

#### Scenario: 读端点返回来源与滚动窗口

- **GIVEN** topic #7 的首义为人工改写（来源节点 #101，报告期 2026-11-10），当前滚动窗口内有 12 条可解析 section
- **WHEN** 用户读取 `GET /api/daily-reports/topics/7/anchor`
- **THEN** 响应 SHALL 含 `anchor_origin`（`resolved=true`、`section_id=101`、标题与报告期齐备）
- **AND** 响应 SHALL 含 `centroid` 段：窗口大小、首义权重、12 条参与滚动计算的 section 集合与 `degraded=false`

#### Scenario: 自动首义不可考时显式不可考

- **GIVEN** topic #9 的首义为创建时写入、无任何人工留痕
- **WHEN** 用户读取该话题的 anchor
- **THEN** 响应 SHALL 含 `anchor_origin.resolved=false` 且来源节点字段为 `null`
- **AND** 系统 SHALL NOT 以「最早成员」等推测节点冒充来源

#### Scenario: 读端点零写副作用

- **GIVEN** topic #7 的 `centroid` 列为 C_old、`centroid_pending_recompute=false`
- **WHEN** 用户读取该话题的 anchor（含来源与滚动窗口）
- **THEN** 系统 SHALL NOT 修改 `centroid` / `embedding` 列、SHALL NOT 置 `centroid_pending_recompute`、SHALL NOT 触发重算、SHALL NOT 写任何表

#### Scenario: 手动建泳道的留痕使读端点报人工来源

- **GIVEN** topic #20 由手动建泳道创建，建话题时留痕了选中的 3 个 section
- **WHEN** 用户读取该话题的 anchor
- **THEN** 响应 SHALL 报 `source=manual`、`section_ids` 为那 3 个 section、`override_count` 为该话题的首义留痕条数

### Requirement: 泳道节点归属变更（移除与加回）

系统 SHALL 提供把 section 在「不归属任何泳道」与「归属某条泳道」之间切换的人工能力，两个方向共用一个端点：

- **移除**（解除归属）：section 的 `persistent_topic_id` 置空，**不再归属任何泳道**。移除 SHALL NOT 要求用户指定换到哪条泳道——「移除」就是「不再归属任何泳道」。
- **加回**（挂回泳道）：把一个当前**不归属任何泳道**的 section 挂到指定泳道——包括本次会话中刚被移除的节点。

内容本身 SHALL NOT 被删除——无论移除还是加回，section 行、其 embedding、正文与线索全部保留，后续仍可再参与候选。

端点 SHALL 为 `PATCH /api/daily-reports/sections/:id/assignment`，请求体 SHALL 携带 `topic_id`：`null` 表示移除（解除归属），数值表示加回到该泳道。批量移除端点 SHALL 为 `POST /api/daily-reports/topics/:id/detach-date`，请求体 SHALL 携带 `period_date`，用于一次移除某一天归属该话题的全部节点。

校验 SHALL 区分两个方向：


加回 SHALL 用于把**无归属**的 section 挂回泳道；**已归属**的 section 改挂到 B 泳道 SHALL 仅在**来源泳道为 candidate（动态）**时允许 —— 改挂语义 SHALL NOT 从 active（已转正）泳道取走节点（用户表达的是「它不该在这儿」而非「它该在那儿」；已转正泳道的素材不因编辑而流动）。改挂 SHALL 追加归属留痕（记录来源与目标泳道）并 SHALL 把**来源与目标两条泳道**均置待重算。

归属变更 SHALL 在单个事务内完成，写入 `daily_report_sections` 的归属快照列：


系统 SHALL 记录人工归属变更留痕（变更时间、变更前的话题 id、动作 `detach`/`attach`），SHALL 使「这条内容为什么被移出 / 为什么被加回」可追溯。

事务末尾 SHALL 触发 `RebuildBoardRelations`（整板重建，非增量）——identity 边由 `persistent_topic_id` 派生，归属变更必然改边。重建失败 SHALL 回滚整个事务。

归属变更 SHALL NOT 变更任何话题的 `hit_count` / `consecutive_hits` / `first_seen_date` / `last_seen_date`（命中计数属算法所有），SHALL NOT 变更任何话题的 `status`（归档只能由用户显式操作触发）。

归属变更 SHALL NOT 在事务内立即重算质心。归属变更 SHALL 将**受影响的话题**（移除时为被移除时其所属的话题；加回时为目标话题）标记为 `centroid_pending_recompute=true`，由下一次日报生成统一重算——质心滚动窗口随 section 移出/移入自然收缩或扩张。

#### Scenario: 单条 section 解除归属

- **GIVEN** section #501 的 `persistent_topic_id=8`（lane_tier=l1_direct，confidence=anchor_hit）
- **WHEN** 用户移除 #501
- **THEN** 系统 SHALL 将 `persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `topic_status_at_report` 写为 NULL，`lane_tier` 保持 `l1_direct`，并记录留痕（动作 `detach`、移除前话题 id 8）

#### Scenario: 移除不需要指定去处

- **WHEN** 用户执行移除
- **THEN** 请求体 SHALL NOT 需要目标话题参数，系统 SHALL NOT 校验或列举其它泳道，SHALL NOT 弹出目标话题选择

加回的边界口径：section 已属目标话题时 SHALL 幂等成功（200，不追加留痕、不重复置标记）；加回时 section 无可用 embedding SHALL 允许，`topic_match_distance` 写 NULL（不虚构 0 值）；校验顺序为「话题存在 → 同 board → 非 archived → 无归属/幂等」。

#### Scenario: 加回本次被移除的节点

- **GIVEN** section #501 在本次会话中被从 topic #8 移除（当前 `persistent_topic_id` 为 NULL，留痕动作 detach）
- **WHEN** 用户把 #501 加回 topic #8
- **THEN** 系统 SHALL 将 `persistent_topic_id` 写为 8、`topic_match_confidence` 写为 `manual`、`topic_match_distance` 写为 #501 embedding 到 #8 锚点的实算距离、`topic_status_at_report` 写为 NULL、`lane_tier` 保持原值，并记录留痕（动作 `attach`、变更前话题 id 为 NULL）

#### Scenario: 加回后质心窗口重新纳入

- **GIVEN** topic #8 因移除 section #501 而被标记 `centroid_pending_recompute=true`，其滚动窗口已不含 #501
- **WHEN** 用户把 #501 加回 #8
- **THEN** #8 SHALL 仍被标记为待重算，且下一次日报生成时其质心 SHALL 按**重新包含 #501** 的滚动窗口重算

#### Scenario: 加回目标校验

- **GIVEN** section #501 当前不归属任何泳道
- **WHEN** 用户尝试加回到 board 1999 的 topic #30（跨版块）
- **THEN** 系统 SHALL 拒绝并返回 4xx（目标必须同版块），SHALL NOT 写入任何列

#### Scenario: 加回不接受从 active 泳道改挂

- **GIVEN** section #501 当前归属 topic #8（status=active）
- **WHEN** 用户尝试加回到 topic #20
- **THEN** 系统 SHALL 拒绝并返回 4xx（已转正泳道的节点只能移除，不能改挂），SHALL NOT 写入任何列

#### Scenario: 从 candidate（动态）泳道改挂

- **GIVEN** section #502 当前归属 topic #9（status=candidate，尚未转正）
- **WHEN** 用户把该节点加入本泳道 topic #20
- **THEN** 系统 SHALL 允许改挂：节点归属改为 topic #20，SHALL 追加归属留痕（来源 topic #9 → 目标 topic #20），SHALL 把 topic #9 与 topic #20 **两条泳道**均置待重算（centroid_pending_recompute=true）

#### Scenario: 归属变更触发关系重建

- **WHEN** 归属变更事务提交
- **THEN** 系统 SHALL 执行 `RebuildBoardRelations`，identity 边按新归属重算，SHALL NOT 保留旧归属的边

#### Scenario: 关系重建失败整体回滚

- **WHEN** 归属变更事务中 `RebuildBoardRelations` 失败
- **THEN** 系统 SHALL 回滚整个事务（归属变更与留痕一并回滚），SHALL NOT 留下中间态

#### Scenario: 批量移除某一天

- **GIVEN** topic #8 在 2026-11-10 归属 3 条 section（#501/#502/#503）
- **WHEN** 用户对 #8 执行 `detach-date`（period_date=2026-11-10）
- **THEN** 系统 SHALL 在单事务内把该日归属 #8 的全部 3 条 section 解除归属（逐条按单条移除口径写列并留痕），并触发一次 `RebuildBoardRelations`

#### Scenario: 被移除内容仍可再参与候选

- **GIVEN** section #501 已从 #8 解除归属
- **WHEN** 后续日报生成时对应内容再次触发归属判定
- **THEN** 系统 SHALL 仍以该内容参与判定（内容未被删除），SHALL NOT 因曾被人工移除而被永久排除出候选集

#### Scenario: 归属变更不动命中计数与状态

- **GIVEN** topic #8 的 hit_count=5、consecutive_hits=2、status=active
- **WHEN** 用户移除或加回某条 section
- **THEN** #8 的 hit_count SHALL 仍为 5、consecutive_hits SHALL 仍为 2、status SHALL 仍为 active

#### Scenario: 归属变更不立即重算质心

- **GIVEN** topic #8 的 `centroid` 列为 C_old
- **WHEN** 归属变更事务提交
- **THEN** #8 的 `centroid` 列 SHALL 保持 C_old 不变（不立即重算），但 #8 SHALL 被标记 `centroid_pending_recompute=true`

### Requirement: 人工修正的派生值刷新时机

系统 SHALL 区分「人工可写的持久事实」与「算法每日重算的派生缓存」，人工修正 SHALL 只落在前者。

质心（`centroid`）、吸尘器标记（`is_vacuum` / `vacuum_strong` / `vacuum_mid`）、泳道快照（`topic_lane_snapshots`）SHALL 为派生缓存。人工修正 SHALL NOT 直接写入这些列，SHALL NOT 提供「立即重算」的人工触发端点。

修正生效 SHALL 依赖下一次日报生成的既有刷新机制：

- 质心刷新 SHALL 覆盖「当日 touched 话题」∪「`centroid_pending_recompute=true` 的话题」；
- 真空统计 SHALL 在日报生成后按整板重算（既有机制）；
- 泳道快照 SHALL 在日报生成后按整板结算（既有机制）。

派生值刷新失败 SHALL NOT 中断其余话题的刷新（既有逐话题隔离语义）。

#### Scenario: 无立即重算端点

- **WHEN** 检查对外 API
- **THEN** 系统 SHALL NOT 暴露「立即重算某话题质心」的人工触发端点；质心重算只有日报生成后的自动路径与既有的存量一次性重算入口（`POST /api/daily-reports/recompute-topic-centroids`）

#### Scenario: 派生缓存不被人工直写

- **GIVEN** topic #7 的 `is_vacuum=true`、`centroid` 为某值
- **WHEN** 用户执行重设首义或剔除节点
- **THEN** 系统 SHALL NOT 写 `is_vacuum` / `vacuum_strong` / `vacuum_mid` / `topic_lane_snapshots` 各列，SHALL 只写人工事实列与待重算标记

#### Scenario: 修正后受影响话题次日重算

- **GIVEN** topic #8 与 #20 均被标记 `centroid_pending_recompute=true`
- **WHEN** 次日日报生成完成
- **THEN** #8 与 #20 的质心 SHALL 均按新的滚动窗口重算（#8 的窗口不再含被剔除的 section，#20 的窗口含新移入的 section），SHALL NOT 因当日无新内容命中而漏算

### Requirement: 人工修正留痕与可追溯

系统 SHALL 记录每一次人工修正的来源与时间，使用户能回答「这条内容为什么被挪走」「这条泳道的定义面什么时候被谁改过」。

首义改写 SHALL 落 `topic_anchor_overrides` 追加表（只追加）；section 归属变更 SHALL 落 `section_assignment_overrides` 追加表（只追加）。两者 SHALL 携带时间戳与操作者标识。

`daily_report_sections` 上的归属变更留痕三列（`manual_assignment_at` / `manual_assignment_from_topic_id` / `manual_assignment_action`）SHALL 保留，其语义为「**最近一次**人工归属变更的快照」——读路径只看这三列；**完整历史**在 `section_assignment_overrides` 追加表里查询（「移除 → 加回」两步各留一行，先前的 detach 痕迹不被覆盖）。两个只读查询端点 SHALL 为 `GET /api/daily-reports/topics/:id/anchor/history` 与 `GET /api/daily-reports/sections/:id/assignment/history`，两者均按时间升序返回。

留痕 SHALL NOT 影响归属判定、质心计算或任何派生值。

#### Scenario: 首义改写历史可查

- **GIVEN** topic #7 的首义已被人工改写 2 次（均为选定节点范围）
- **WHEN** 查询该话题的首义改写历史
- **THEN** 系统 SHALL 返回 2 条留痕记录（每次的改前值、改后值、输入模式、选定的 section id 列表、时间），SHALL 按时间升序

#### Scenario: 移除留痕可查

- **GIVEN** section #501 曾被从 topic #8 移除、之后又被加回
- **WHEN** 查询该 section 的归属变更留痕
- **THEN** 系统 SHALL 返回变更时间、变更前的话题 id 与动作（`detach` / `attach`）

### Requirement: 滚动窗口成员取数的单点实现

系统 SHALL 为「某话题当前参与滚动计算的 section 集合」提供**单一取数实现**（只读），**质心计算**与**首义读端点** SHALL 共用该实现，SHALL NOT 各自维护一份查询——两份实现会随窗口口径演进而漂移（一个改了排序、另一个没改，读端点就会显示与实际质心不同的成员集）。

取数口径 SHALL 与既有滚动窗口口径**完全等价**：

- **谓词**：该话题归属的 section，且 `embedding IS NOT NULL`；
- **排序**：报告期日期倒序，同期按 section id 倒序；
- **上限**：窗口大小（配置项 `persistent_topic_centroid_window`，默认 30）。

该实现 SHALL 承载两处调用：质心计算（取窗口内向量做滚动均值）与首义读端点（取窗口内 section 供只读展示）。取数 SHALL 为**只读**：SHALL NOT 写表、SHALL NOT 触碰质心或向量列、SHALL NOT 触发重算。

等价性 SHALL 可验证：质心计算与读端点 SHALL 调用**同一实现**取数，SHALL NOT 在读端点另写一条「看起来等价」的查询；任何口径调整 SHALL 一次改、两处生效。

#### Scenario: 质心计算与读端点取数同源

- **GIVEN** topic #7 有 40 条可解析 section，窗口大小 30
- **WHEN** 系统分别执行质心计算与读取该话题的 anchor
- **THEN** 两者 SHALL 取到**同一批** section（同一谓词、同一排序、同一上限）
- **AND** 读端点展示的参与滚动计算节点 SHALL 与质心实际使用的窗口成员一致

#### Scenario: 窗口取数口径等价

- **WHEN** 检查滚动窗口取数实现
- **THEN** 取数 SHALL 为「归属本话题 + embedding 非空 → 按报告期倒序、同期按 id 倒序 → 取前 N 条」
- **AND** 窗口大小 SHALL 取自既有配置项（默认 30），SHALL NOT 硬编码为另一数值

#### Scenario: 取数只读

- **WHEN** 首义读端点取滚动窗口成员
- **THEN** 系统 SHALL NOT 写任何表、SHALL NOT 修改质心或向量列、SHALL NOT 置待重算标记
