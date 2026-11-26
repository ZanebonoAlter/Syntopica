<!-- domain: semantic-board, daily-report, topic-graph -->

# lane-ops Specification

## Purpose

泳道这条线怎么被改、被看、被量：①**改**——人工重设首义、节点归属变更（移除与加回）、派生值刷新时机、修正留痕与可追溯；②**看**——版块「板块内容」首屏的泳道动态视图（滚动 14 天态势 + 逐日时间线）、日报阅读视图里的泳道趋势区块（14 天/月/年三档、逐日事件就地展开）；③**量**——泳道当天热度 = 线索数按话题状态加权，版块关注度 = 各泳道热度之和，是雷达分档、泳道行序与矩阵排序的**唯一口径出处**。本族只管泳道维度；版块态势页骨架见 `workshop-board-console`，画布见 `workshop-lane-compose` / `lane-correction-surface`。

## Requirements

### Requirement: 人工重设泳道首义

<!-- from: lane-manual-correction (2026-11-16 lane-manual-correction-loop) -->

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

<!-- from: lane-manual-correction (2026-11-16 lane-manual-correction-loop) -->

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

<!-- from: lane-manual-correction (2026-11-16 lane-manual-correction-loop) -->

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

<!-- from: lane-manual-correction (2026-11-16 lane-manual-correction-loop) -->

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

<!-- from: lane-manual-correction (2026-11-16 lane-manual-correction-loop) -->

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

### Requirement: 泳道动态视图入口

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

版块态势页（`/tags/:boardId`）SHALL 以「泳道 × 日期」矩阵作为版块选中后的默认首屏概览，取代原「板块内容」tab 内的泳道卡片区。矩阵的**日格** SHALL 保持只读展示（格内不提供任何写操作；治理右轨/抽屉的操作 SHALL NOT 触发矩阵以外的副作用）。矩阵的**行头** SHALL 提供泳道操作入口（见下），写操作 SHALL 统一由泳道编辑面板承载，SHALL NOT 在矩阵内就地改变话题生命周期状态。演进带作为矩阵的选中泳道明细区，与矩阵共用同一聚合响应。

矩阵行头 SHALL 提供「⋯」操作入口，至少包含「编辑泳道」项（打开泳道修正工作面，见 lane-correction-surface 能力）。行头操作 SHALL NOT 触发行选中、选中日变更或泳道选中副作用。

#### Scenario: 默认展示泳道动态区

- **WHEN** 用户选中某版块并进入版块态势页
- **THEN** 页面 SHALL 渲染泳道 × 日期矩阵（泳道动态视图），并在其下方渲染当前选中泳道的演进带

#### Scenario: 只读语义

- **WHEN** 用户点击矩阵的某个日格
- **THEN** 系统 SHALL 只改变当前选中日与选中格，SHALL NOT 在格内提供归档/转正/合并等生命周期写操作；日格 SHALL 保持只读，写操作统一由泳道编辑面板承载

#### Scenario: 行头提供编辑入口

- **WHEN** 用户查看矩阵某行的行头
- **THEN** 行头 SHALL 提供「⋯」操作入口，其中包含「编辑泳道」项，点击后 SHALL 打开泳道编辑面板

#### Scenario: 行头操作不误触发行选中

- **WHEN** 用户点击行头的「⋯」按钮
- **THEN** 系统 SHALL NOT 选中该行、SHALL NOT 改变当前选中日，SHALL NOT 触发泳道选中副作用

#### Scenario: 矩阵内不就地改生命周期

- **WHEN** 用户在矩阵区域操作
- **THEN** 系统 SHALL NOT 在矩阵内直接执行归档/转正/合并等话题生命周期写操作，此类写操作 SHALL 统一在泳道编辑面板内完成

### Requirement: 泳道卡片构成

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

原「每条泳道渲染为一张卡片」的形态 SHALL 退役，改由矩阵行与演进带共同承载：矩阵行 SHALL 展示泳道名、追踪标识与断档；矩阵格内数字 SHALL 为该泳道当日的线索合计（各主题桶线索数之和，事件线退役后不再有事件线量纲，也不再有桶峰值口径），行尾 MUST NOT 再渲染重复总数列。矩阵格 SHALL NOT 渲染颜色通道，格值与 title/aria SHALL 承载全部信息。选中泳道的演进带 SHALL 展示按日期排列的节点（节点数字与点径 = 当日各主题桶线索数之和，与矩阵日格同一量纲，线索密集日显示标签）、滚动态势句入口（最新结算快照逐字，标注 as_of 与「非本视窗总结」）、以及当日线索工作面。事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排。

#### Scenario: 发展时间线结构
- **WHEN** 某泳道观察范围内多个日期有锚定 section
- **THEN** 演进带 SHALL 按日期顺序排列节点，密集日（≥3 条线索）显示事件标签
- **AND** 事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排

#### Scenario: 一句话态势展示
- **WHEN** 某泳道存在有效的滚动态势快照
- **THEN** 演进带区域 SHALL 提供该态势句并标注汇总截止日（as_of）与「非本视窗总结」，长句 SHALL 以折叠方式呈现而非直接铺满版面

#### Scenario: 单日多事件
- **WHEN** 某泳道同一天有 2 个锚定 section、各含多条 thread
- **THEN** 当日工作面 SHALL 列出该日全部事件的 thread 标题；聚合响应每 section 截前 N 条标题时，完整标题 SHALL 经日报详情按稳定 `section_id` 读取（截断提示仅作读取不可用时的降级）

#### Scenario: 矩阵格与演进节点同量纲
- **WHEN** 某泳道某日两个主题桶分别含 3 条与 4 条线索
- **THEN** 矩阵日格 SHALL 显示 7，演进带该日节点数字 SHALL 同为 7
- **AND** SHALL NOT 一个取桶峰值 4、另一个取合计 7

### Requirement: 展示范围与排序

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

泳道动态区 SHALL 展示「关注（active）」与「我在追踪（watch 关联）」的泳道，排序 SHALL 为泳道当天热度降序 → 全窗线索合计降序 → topic id 升序（热度口径与权重见本族「泳道当天热度口径」）；watch 关联泳道 SHALL 带追踪标识。近 14 天无锚定 section 的 active 泳道（沉寂）SHALL NOT 展示。candidate 泳道 SHALL NOT 出现在主卡片区。

排序口径 SHALL NOT 使用 section 条数：泳道内主题分桶会把同一话题同一天的内容拆成多条 section，按条数排序会让"当天拆得多的泳道"虚高。排序 SHALL NOT 使用事件线数或事件数（事件线退役后无此输入）。

#### Scenario: 活跃排序

- **GIVEN** 泳道 A 近 14 天有 6 天出现锚定 section（其中某天 3 条），泳道 B 有 3 天（每天 1 条），A 的当天热度高于 B
- **WHEN** 渲染泳道动态区
- **THEN** A SHALL 排在 B 前（泳道当天热度降序），SHALL NOT 因为 A 的 section 条数被拆分放大而进一步加权

#### Scenario: 同日多条只计一天

- **GIVEN** 泳道 A 仅有 1 天出现锚定 section，但该天被拆成 4 条 section；泳道 B 有 2 天、每天 1 条
- **WHEN** 渲染泳道动态区
- **THEN** 行序 SHALL 由泳道当天热度决定，A 被拆成 4 条 section 本身 SHALL NOT 抬高其热度

#### Scenario: watch 标识

- **WHEN** 某泳道被用户 watch（追踪）关联
- **THEN** 泳道行 SHALL 带追踪标识（与普通 active 行视觉区分）

#### Scenario: 沉寂不展示

- **WHEN** 某 active 泳道近 14 天无任何锚定 section
- **THEN** 该泳道 SHALL NOT 渲染

#### Scenario: 候选不进主区

- **WHEN** 某 candidate 泳道近 14 天有锚定 section
- **THEN** 该泳道 SHALL NOT 出现在主区（仅可能出现在候选栏）

### Requirement: 候选栏

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

泳道动态区底部 SHALL 提供候选栏，列出最近达可见门槛的 candidate 泳道（名字 + 最近动向摘要），作为「值得注意」的只读提示。候选栏 SHALL NOT 提供转正等生命周期操作（转正走既有话题管理入口）。

#### Scenario: 候选列出
- **WHEN** 某 candidate 泳道 hit_count 达可见门槛
- **THEN** 候选栏 SHALL 列出其名称与最近动向

#### Scenario: 无候选
- **WHEN** 无达门槛的 candidate
- **THEN** 候选栏 SHALL NOT 渲染（或明确提示无候选，二选一于 ui-design 定稿）

### Requirement: 卡片跳转联动

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

点击泳道卡片（含候选栏条目）SHALL 跳转「话题总览」tab 并聚焦该话题（focus 视图），沿用既有联动交互模式。

#### Scenario: 点卡片聚焦
- **WHEN** 用户点击泳道动态区某张泳道卡
- **THEN** 界面 SHALL 切换到「话题总览」tab 并进入该话题的 focus 视图

### Requirement: 空态处理

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

板块无日报时，泳道动态区 SHALL 展示空态引导用户生成日报（沿用既有生成入口与进度反馈模式）；生成完成后 SHALL 自动刷新泳道动态。

#### Scenario: 空态引导
- **WHEN** 板块无任何日报
- **THEN** 泳道动态区 SHALL 展示「生成日报」引导而非空白

#### Scenario: 生成后刷新
- **WHEN** 空态触发的日报生成完成（进度反馈结束）
- **THEN** 泳道动态区 SHALL 自动加载新数据

### Requirement: 矩阵观察窗读取边界

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

矩阵/追踪的自由观察日期 SHALL 仅消费既有 `GET /semantic-boards/:id/lane-dynamics?days=N` 端点的7/14/30/365四档响应：观察窗 SHALL NOT 新增 `start/end` 聚合参数、新端点或新的统计模式。sourceDays 仅由独立档按钮或明确 URL days 变更；A（source档）SHALL 是名单、source全轴前端 clueTotal→sectionTotal→ID 排序、休眠与候选的唯一基准。B（365池）SHALL 仅补 A 同 ID 历史内容；观察日期无论是否恰7/14/30/365日均 MUST NOT 换基准，B独有ID不得加入名单/候选。A/B失败 SHALL 分层：A成功B失败保留A可读，历史未取到不补0、不换名单或强改视窗；A首次失败B成功显示A错误态，A刷新失败保留旧A并提示stale，MUST NOT 用B冒档成功。sourceDays=365 SHALL 只发一次请求、档/池共用。当前板块source成功缓存 SHALL 限四档（365共用），报告列表索引仅1份、报告详情最多14份、文章详情最多20份；SHALL 用简单Map FIFO/既有工具，失败不入成功缓存、不逐出成功项，切板块失效、卸载释放。当前展开阅读对象 SHALL 不因逐出消失或回顶。完整线索标题、摘要与相关文章 SHALL 复用既有日报列表/详情与文章详情端点按稳定 ID（report `period_date` → `section_id` → thread `id` / `related_article_ids`）读取，MUST NOT 以线索标题文本匹配作为长期关联。报日索引 SHALL 复用既有板块日报列表端点（前端每板块至多 1 份成功缓存），MUST NOT 为日期轴新增聚合参数或端点；0 正式泳道版块的日期轴与「当日其他话题」SHALL 复用同一索引与既有单日报告详情端点（按稳定 `section_id` 读取全量 threads），MUST NOT 因窗内无泳道而禁止读取真实已存在的日报。报告优先读取 SHALL 与既有编年共用同一有界缓存与板块作用域（索引 1 份、报告 ≤14、文章 ≤20）并各持独立 epoch，同板块切换/卸载统一释放；MUST NOT 引入第二套读取器或重复 GET。报日与报告优先读取同样 SHALL 零写入。产品实现 MUST NOT 复制原型离线样本、原型常数或把样本切片冒充后端取数；本能力全部交互 SHALL 零写入（不改话题状态、不触发生成/回填）。

#### Scenario: 自由日期不触发新聚合
- **WHEN** 用户把source14的观察范围拖到任意合法区间（含恰7/30/365日）
- **THEN** 前端 SHALL NOT 发出带起止日期参数的lane-dynamics请求或换source档；A14仍是唯一基准，B仅补其同ID历史，已缓存其他档不得因日期命中被提升为base

#### Scenario: 365档池请求去重
- **WHEN** sourceDays明确选择365，或该板块365请求已在途
- **THEN** 档/池 SHALL 共用同一次365读取及成功缓存，不增加第五档缓存

#### Scenario: 有界缓存与当前阅读稳定
- **WHEN** 第15份报告或第21篇文章成功读取，且最先入缓存项正在展开阅读
- **THEN** SHALL FIFO逐出到14份/20篇上限，但当前内容、展开态、选日、焦点与滚动保持；切板块/卸载释放缓存，失败不计成功项

#### Scenario: 稳定 ID 关联
- **WHEN** 当日工作面读取某主题桶的线索与摘要
- **THEN** 关联 SHALL 经 `section_id`/thread id 命中日报详情数据；稳定ID缺失或读取失败时 SHALL 降级注明，MUST NOT 借用相似标题；标题文字不同不影响ID关联

#### Scenario: 零写边界
- **WHEN** 用户拖选观察日期、切换档位、展开摘要或相关文章
- **THEN** 全程 SHALL 只有只读 GET 请求，无任何写请求或状态变更

#### Scenario: 零泳道仍按报日读分区
- **WHEN** 版块 0 正式泳道但有日报，用户在日期轴选择任一报日并展开「当日其他话题」
- **THEN** 系统 SHALL 经既有报日索引与日报详情端点列出该日报中不属于正式/休眠泳道的分区与全量 threads；MUST NOT 新增端点/参数，MUST NOT 把候选池或管理数据当分区列出

#### Scenario: 报日索引不改聚合口径
- **WHEN** 前端为日期轴加载报日索引与所选日报告详情
- **THEN** SHALL 复用既有日报列表/详情端点与共享缓存；MUST NOT 改变 lane-dynamics 的名单/排序/休眠/候选口径，MUST NOT 产生写请求

### Requirement: 日报后泳道态势演进结算

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

每日日报生成完成（该板块报告保存成功）后，系统 SHALL 为该板块的活跃泳道异步结算滚动 14 天态势：输入为该泳道近 14 天锚定 section 的「日期｜节标题｜thread 标题」清单，并附带**量级信号段**（系统计算的每日线索量与近 7 天 / 上个 7 天合计对比，非 LLM 产出、随素材注入）；输出为**单版演进叙事态势句**——一段 150-200 字的中文纯文本，按「阶段划分（带大致日期锚）→ 每阶段驱动/转折（事件仅作转折标记，不展开）→ 当前落点与局势判断 → 量级佐证」的演进叙事组织，随每日结算覆盖更新（每泳道保留一份当前快照，含 as_of 汇总截止日）。态势句 SHALL NOT 逐日罗列事件、SHALL NOT 预测未来走向（只描述已发生的变化）、SHALL NOT 编造清单与量级信号之外的数字。快照的 `rolling_detail` 字段 SHALL 固定写入空值（不再产长版）；存量快照的历史长版数据 SHALL 保留原值不清理不回填，下个日报日结算自然覆盖。单次结算调用的输出 token 预算 SHALL 覆盖单版上限。结算 SHALL NOT 阻塞或失败中断日报主流程（松耦合：失败仅记日志，下个日报日自愈）。

#### Scenario: 日报完成后结算

- **WHEN** 某板块当日日报生成成功
- **THEN** 系统 SHALL 异步为该板块活跃泳道逐个刷新滚动 14 天态势快照（单版演进叙事态势句，写入 `rolling_summary`；`rolling_detail` 置空）

#### Scenario: 量级信号随素材注入

- **WHEN** 结算调用构造 prompt 素材
- **THEN** 系统 SHALL 在事件清单之外附带量级信号段：窗口内每日线索量与近 7 天 / 上个 7 天合计对比，由系统按素材统计计算，SHALL NOT 依赖模型自行计数

#### Scenario: 演进叙事定位约束

- **WHEN** 结算调用构造 system 指令
- **THEN** 指令 SHALL 要求按阶段组织叙述（事件仅作转折标记）、SHALL 包含对逐日罗列与未来预测的显式禁止

#### Scenario: 机械截断保护

- **WHEN** 模型输出的态势句超过 200 字
- **THEN** 系统 SHALL 在 200 字窗口内最后一个句末标点（`。`/`！`/`？`/`…`）处收口后入库，SHALL NOT 从字中间切断；窗口内无可用句末标点（或标点过早，低于预算 60%）时 SHALL 退回 rune 安全截断

#### Scenario: 空输出守卫

- **WHEN** 模型输出为空或纯空白
- **THEN** 该泳道本次结算 SHALL 记为失败并跳过，快照保持旧值，下个日报日重试

#### Scenario: 结算失败不阻塞

- **WHEN** 某泳道结算 LLM 调用失败
- **THEN** 日报主流程 SHALL 不受影响，失败记日志；该泳道快照保持旧值（或缺失态），下个日报日重试

#### Scenario: 存量快照兼容

- **WHEN** 某泳道快照为存量数据（`rolling_detail` 含历史长版）
- **THEN** 系统 SHALL 保持该值原样入库不清理，读取方 SHALL 以 `rolling_summary` 为唯一态势内容源，下个日报日结算覆盖时 `rolling_detail` 自然置空

#### Scenario: 态势句与时间线同窗

- **THEN** 态势句的素材窗口与时间线窗口 SHALL 同为滚动 14 天（以最近一份已完成日报为期）

### Requirement: 泳道动态聚合端点

<!-- from: board-lane-dynamics (2026-09-11 overview-lane-dynamics) -->

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

### Requirement: 泳道趋势区挂载

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

日报阅读视图的 active 话题泳道展开体顶部 SHALL 渲染「泳道趋势」区块；泳道未展开或泳道无话题 id 时 SHALL NOT 渲染。趋势区 SHALL 置于当日 section 明细与 7 天连续性节点图之上，既有内容与交互 SHALL 保持不变。

#### Scenario: 展开泳道见趋势区

- **WHEN** 用户在日报阅读视图展开某条 active 话题泳道
- **THEN** 泳道展开体顶部 SHALL 出现「泳道趋势」区块，其下依次为既有节点图与当日明细

#### Scenario: 非话题分组不渲染

- **WHEN** 某泳道分组无话题 id（未锚定到持久话题）
- **THEN** 该分组展开体 SHALL NOT 渲染趋势区，其余内容不受影响

### Requirement: 周期三档切换

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

趋势区 SHALL 提供「近 14 天 / 月 / 年」三档分段切换，默认选中「近 14 天」。当前选中档 SHALL 展示该周期的概要全文与汇总截止信息；概要文本 SHALL 完整展示，SHALL NOT 因长度截断。切换 SHALL 即时生效且各档选中态在泳道展开期间保持。

#### Scenario: 默认 14 天档

- **WHEN** 用户展开泳道且趋势区数据就绪
- **THEN** 「近 14 天」档 SHALL 为选中态并展示 14 天长版态势全文与汇总截止日

#### Scenario: 切换月档

- **WHEN** 用户从「近 14 天」切换到「月」
- **THEN** 概要区 SHALL 展示该话题最新月度归档摘要全文与该归档的汇总截止日

#### Scenario: 概要不截断

- **WHEN** 某档概要文本长度超过单屏可容纳范围
- **THEN** 该档 SHALL 完整渲染全文（区块内自然滚动），SHALL NOT 出现省略号或硬截断

### Requirement: 月/年档内容与降级

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

「月」「年」档 SHALL 展示该话题对应粒度最新归档周期的摘要全文（来源于既有话题周期归档上下文，按周期取最新一条）；该粒度无归档时 SHALL 展示明确的空归档占位，SHALL NOT 以其它粒度或其它话题的数据冒充。

#### Scenario: 月档展示最新归档

- **WHEN** 该话题存在多条月度归档摘要
- **THEN** 月档 SHALL 展示周期最新一条的全文与该条汇总截止日

#### Scenario: 年档空归档

- **WHEN** 该话题无年度归档摘要
- **THEN** 年档 SHALL 展示「该周期暂无归档摘要」占位，SHALL NOT 展示相邻粒度数据

### Requirement: 逐日事件就地展开

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

「近 14 天」档 SHALL 提供逐日事件时间线的就地展开开关（默认收起）。展开后 SHALL 按「日期 → 当日事件列表」结构展示该泳道近 14 天逐日事件脉络，事件为日报 thread 级标题；事件数量被截断时 SHALL 如实标注被折叠数。

#### Scenario: 默认收起

- **WHEN** 14 天档概要就绪
- **THEN** 逐日事件列表 SHALL 处于收起态，仅展示展开开关

#### Scenario: 展开见逐日事件

- **WHEN** 用户展开逐日事件列表
- **THEN** SHALL 按日期组织展示近 14 天逐日事件（同日事件归组、按日期倒序），每条事件为 thread 级标题

#### Scenario: 截断如实标注

- **WHEN** 某日事件数超过单日展示上限
- **THEN** 该日 SHALL 截断展示并标注被折叠的事件数

### Requirement: 加载与错误隔离

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

趋势区数据加载状态 SHALL 明确可见；任一档数据请求失败 SHALL 在趋势区内展示内联错误与重试入口，SHALL NOT 影响泳道展开体的既有内容渲染。

#### Scenario: 月档请求失败

- **WHEN** 用户切换到月档且归档摘要请求失败
- **THEN** 趋势区 SHALL 展示内联错误条与重试入口，下方既有节点图与当日明细 SHALL 照常渲染

#### Scenario: 加载态可见

- **WHEN** 某档数据请求进行中
- **THEN** 该档 SHALL 展示加载提示，SHALL NOT 以空白或旧档内容冒充

### Requirement: 14 天档内容

<!-- from: lane-trend-overview (2026-09-22 lane-trend-overview) -->

「近 14 天」档 SHALL 展示该泳道滚动 14 天态势的**单版演进叙事态势句**（快照 `rolling_summary`）全文，SHALL NOT 消费 `rolling_detail`（含存量历史长版）；态势快照整体缺失时 SHALL 展示待结算占位且三档切换仍可用。

#### Scenario: 态势句就绪

- **WHEN** 该泳道快照含态势句
- **THEN** 14 天档 SHALL 展示态势句全文与汇总截止日

#### Scenario: 存量长版不消费

- **WHEN** 该泳道快照的 `rolling_detail` 含存量历史长版
- **THEN** 14 天档 SHALL 仅渲染 `rolling_summary`，SHALL NOT 渲染长版，也不展示任何「长版待生成」类提示

#### Scenario: 快照缺失

- **WHEN** 该泳道无态势快照
- **THEN** 14 天档 SHALL 展示待结算占位，用户仍可切换月/年档

### Requirement: 泳道当天热度口径

<!-- from: lane-heat-metric (2026-11-20 retire-story-adopt-event-metrics) -->

系统 SHALL 以「该泳道当天线索数」为唯一输入计算泳道当天热度：活跃（active）泳道的当天线索数按权重 `1.0` 计，候选（candidate）泳道的当天线索数按权重 `lane_heat_candidate_weight` 计（默认 `0.5`）。

当天 SHALL 为 Asia/Shanghai（UTC+8）日历日，与日报 `period_date` 同一日界。线索数 SHALL 为该泳道在该日锚定 section 下的 thread 数；综合稿（`scope=composite`）的 thread SHALL NOT 计入线索数。

系统 SHALL NOT 把事件线数、事件数、section 数、主题桶数、参与者数、半衰期衰减或任何 LLM 输出作为热度输入。热度 SHALL 为确定性函数：同一份日报数据重复计算 SHALL 得到同一结果；任意历史日 SHALL 可回算（热度不落历史表）。

候选权重 SHALL 经 `ai_settings` 在线可调，有效值为 `[0,1]` 内有限数；缺项 SHALL 回退默认 `0.5`，坏存量值 SHALL 告警并回退默认，非法提交 SHALL 整体拒绝且不覆盖原值。候选权重 SHALL NOT 参与话题生命周期判定：candidate 连续命中与转正资格仍 SHALL 只由命中计数与转正门槛决定。

#### Scenario: 活跃与候选按不同权重计

- **WHEN** 某活跃泳道当天有 6 条线索，某候选泳道当天有 4 条线索，候选权重为 0.5
- **THEN** 活跃泳道当天热度 SHALL 为 6.0
- **AND** 候选泳道当天热度 SHALL 为 2.0

#### Scenario: 候选权重在线调整后立即生效

- **WHEN** 把候选权重从 0.5 调到 0.2 并重新读取同一泳道热度
- **THEN** 该候选泳道热度 SHALL 按 0.2 重算为 0.8
- **AND** 活跃泳道热度 SHALL 保持 6.0 不变

#### Scenario: 热度不因分桶或事件标签数量变化而变

- **WHEN** 同一泳道同一天的线索数不变，但打标模型把当天内容拆成不同数量的主题桶或产出不同数量的事件标签
- **THEN** 该泳道当天热度 SHALL 保持不变

#### Scenario: 只表达当天不跨天累积

- **WHEN** 某泳道前一天有 20 条线索、当天 0 条线索
- **THEN** 该泳道当天热度 SHALL 为 0
- **AND** SHALL NOT 包含前一天的任何衰减残留或半衰期分量

#### Scenario: 坏权重存量回退默认

- **WHEN** `lane_heat_candidate_weight` 的存量值为越界数、非有限数或空串
- **THEN** 系统 SHALL 回退默认 0.5 并留下告警
- **AND** SHALL NOT 因该坏值拒绝渲染泳道或版块行

#### Scenario: 候选权重不影响话题状态机

- **WHEN** 把候选权重调到 0
- **THEN** 候选话题的命中计数、连续命中资格与转正门槛 SHALL 保持原判定
- **AND** 候选话题 SHALL NOT 因热度为 0 被隐藏、归档或转正

### Requirement: 版块关注度与分档

<!-- from: lane-heat-metric (2026-11-20 retire-story-adopt-event-metrics) -->

版块关注度 SHALL 为该版块各泳道当天热度之和，保留一位小数。版块行 SHALL 按关注度降序排列；关注度低于关注阈值的版块 SHALL 折叠为一行「常温」摘要。

分档阈值 SHALL 为可配置项，且 SHALL 按新关注度量纲下的真实分布标定，标定依据 SHALL 记录在 design 或实现文档中。阈值 MUST NOT 直接沿用事件线热度量纲下的 `3.3` / `0.7`，也 MUST NOT 沿用更早的 section 计数量纲下的 `50` / `15`。

分档信息 SHALL 以可读形式表达（行内徽标或文字 + hover 提示），MUST NOT 只以颜色表达。关注度 SHALL NOT 被用于话题状态判定，也 SHALL NOT 因候选泳道权重调高而被解释为话题活跃度。

#### Scenario: 关注度为泳道热度之和

- **WHEN** 某版块有两个活跃泳道当天热度分别为 6.0 与 2.0
- **THEN** 该版块关注度 SHALL 为 8.0 并按一位小数呈现

#### Scenario: 分档阈值按新分布标定

- **WHEN** 新关注度量纲下真实分布的分位点与旧 3.3 / 0.7 不一致
- **THEN** 分档阈值 SHALL 以新分布重新标定并留痕标定依据（样本、日期、分位点）
- **AND** SHALL NOT 沿用旧数值，也 SHALL NOT 沿用更早的 50 / 15

#### Scenario: 分档可读不靠颜色

- **WHEN** 某版块落入「关注」档
- **THEN** 界面 SHALL 以文字徽标或行内提示表达该档位
- **AND** SHALL NOT 仅以颜色区分档位

### Requirement: 线索计数的内容身份去重口径

<!-- from: lane-heat-metric (2026-11-20 retire-story-adopt-event-metrics) -->

对外呈现的「来源数 / 文章数 / 参与篇数」SHALL 按**内容身份**去重：同一篇内容（规范化链接相同）被多个 feed 各抓一份 SHALL 只计 1 篇。

去重键 SHALL 为 `COALESCE(NULLIF(content_key,''), participant_key)`，其中 `content_key` = 规范化文章链接（去 `#fragment`、去 `?query`、scheme/host 小写、path 原样），`participant_key` 只作 feed 粒度兜底。内容身份不可判定（链接为空）时 SHALL 退化为 feed 粒度，SHALL NOT 静默按 0 计。

该口径 SHALL 是所有对外计数的唯一实现点，SHALL NOT 在不同消费面各写一份等价式。

#### Scenario: 同一内容多 feed 只算一篇

- **WHEN** 同一篇文章被 3 个 feed 各抓一份、规范化链接相同
- **THEN** 该簇的来源篇数 SHALL 为 1 而不是 3

#### Scenario: 内容身份不可判定时退化为 feed 粒度

- **WHEN** 某文章链接为空、内容身份不可判定
- **THEN** 该篇 SHALL 按 feed 粒度单独计 1 篇
- **AND** SHALL NOT 被丢弃或与其他文章合并

#### Scenario: 不同消费面口径一致

- **WHEN** 同一份数据分别被矩阵来源数、聚簇文章数与候选排序消费
- **THEN** 三处 SHALL 得出同一去重口径下的同一计数
