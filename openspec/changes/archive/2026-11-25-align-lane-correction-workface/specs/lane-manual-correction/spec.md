# lane-manual-correction Delta

## MODIFIED Requirements

### Requirement: 人工重设泳道首义

系统 SHALL 提供人工重设 PersistentTopic 首义向量（`board_persistent_topics.embedding`）的能力。首义是话题的「定义面」，是质心公式 `centroid = α·首义 + (1-α)·滚动均值` 中承载话题身份的那一半。

首义 SHALL 通过**手动选择话题节点范围**来设定：用户选定一组 section（节点），系统 SHALL 按 mean pooling 聚合其 embedding 作为新首义（与既有手动建话题的聚合口径一致）。首义 SHALL NOT 通过自然语言描述设定——用户对「这条泳道是什么」的描述，其可操作的形态就是「哪些节点定义它」，画布上 SHALL 看得见当时选了哪些节点。

写入端点 SHALL 为 `PATCH /api/daily-reports/topics/:id/anchor`，请求体 SHALL 携带 `section_ids`（选定节点的 id 列表）。响应 SHALL 返回 `topic_id`、新首义来源（`selected_sections`）、该话题的人工改写次数、以及生效时机标记 `next_report`。

配套读端点 SHALL 为 `GET /api/daily-reports/topics/:id/anchor`（纯读，不产生任何写入）。响应 SHALL 为**向后兼容扩展**：既有字段（`topic_id` / `source` / `section_ids` / `override_count` / `effective_from`）SHALL 全部保留，新增字段 SHALL 只增不改。

| 字段 | 含义 |
|---|---|
| `source` | `auto` / `manual` 两态：`manual` 表示存在**人工首义留痕**（人工改写，或手动建泳道时写入的建话题留痕）；`auto` 表示无人工留痕、首义为创建时写入 |
| `section_ids` | 当前首义节点集（取 `topic_anchor_overrides` 最新行反解）；`auto` 且无节点集时 SHALL 为**空集**，前端 SHALL NOT 虚构列表 |
| `override_count` | 该话题的**首义留痕条数**（`topic_anchor_overrides` 行数）；手动建泳道时写入的建话题留痕计入其中 |
| `effective_from` | 存在人工留痕时为 `next_report`（质心混合同次日日报生效）；无留痕时为空串 |
| `anchor_origin` | 首义来源节点：`section_id` / `title` / `period_date` / `resolved` |
| `centroid` | 滚动窗口口径：`window_size` / `anchor_weight` / `section_ids` / `members[]` / `degraded` |

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

## ADDED Requirements

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
