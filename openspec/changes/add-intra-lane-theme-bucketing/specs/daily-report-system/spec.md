## MODIFIED Requirements

### Requirement: 日报数据模型

系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。线程数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, created_at。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

日报记录的落库 SHALL 以 (semantic_board_id, period_date) 为冲突键执行原子 upsert：同 board 同日重复落库（无论顺序还是并发）SHALL 收敛为一条记录，SHALL NOT 抛出唯一约束冲突错误。

**同一 report 内 `cluster_index` MAY 重复**：一个聚类/话题的当日内容被拆成多条 section 时（泳道内主题分桶、手动编排、watch 物化），兄弟 section SHALL 共享同一 `cluster_index`，以 `bucket_index`（0 起、组内唯一）区分先后；(report_id, cluster_index) SHALL NOT 建唯一约束。未分桶的 section SHALL 取 `bucket_index=0`、`bucket_title=NULL`。

#### Scenario: 创建日报记录
- **WHEN** 为 SemanticBoard #5 在 2026-05-25 生成日报
- **THEN** 系统 SHALL 在 `board_daily_reports` 表创建一条记录，status="generating"，period_date="2026-05-25"，semantic_board_id=5

#### Scenario: 日报记录唯一性
- **WHEN** SemanticBoard #5 在 2026-05-25 已有一条 status="done" 的日报
- **THEN** 系统 SHALL NOT 创建重复记录，而是更新已有记录

#### Scenario: 并发落库同板同日不炸唯一约束
- **WHEN** 两条写路径（正常情况下被生成闸互斥；异常绕过闸时）先后或并发为 SemanticBoard #5 落库 2026-05-25 的日报
- **THEN** 系统 SHALL 通过原子 upsert 收敛为一条记录（后写覆盖先写），SHALL NOT 出现 duplicate key 唯一约束错误，SHALL NOT 产生部分写入的残缺日报（sections 与主记录同事务落库）

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事线程 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

#### Scenario: 兄弟桶共享聚类序号

- **GIVEN** 某聚类被泳道内主题分桶拆成 3 个桶
- **WHEN** 日报落库
- **THEN** 3 条 section SHALL 共享同一 `cluster_index`，`bucket_index` 分别为 0/1/2，`bucket_title` 各自为主题方向名

#### Scenario: 未分桶 section 取默认桶序号

- **GIVEN** 某聚类未达到分桶阈值（threads < 6 且 tags < 10）
- **WHEN** 日报落库
- **THEN** 该 section SHALL 为 `bucket_index=0`、`bucket_title=NULL`，且其 `cluster_index` 在该 report 内仍然唯一

### Requirement: 日报生成编排流水线

系统 SHALL 提供 `GenerateDailyReport(ctx, boardID, date)` 编排函数，按顺序执行：收集板内事件标签 → 质量筛选 → 去重 → LLM 分组(带组数限制) → 查询昨日日报 → 并行生成(Call A + C×K) → **泳道内主题分桶**（见「泳道内主题分桶」需求；未达阈值或分桶失败时保持单 section） → section 内容化 embedding 生成（文本来源见 section-content-embedding 能力，基于该 section 所聚 tag 的 label/description/代表文章摘录，而非 cluster_label 标题文本；分桶产出的每条 section 以其自身桶内 tag 计算 embedding） → 同日 section 两阶段合并（分桶产出的兄弟 section 之间 SHALL NOT 合并） → **Watch 物化追加**（keyword_topic / sentence_topic 物化轨按 watch-materialized-topic 能力产出追加 section；任一关注的物化失败 SHALL 降级跳过，SHALL NOT 阻断流水线） → section embedding 匹配写入关系表 → 组装 BoardDailyReport + DailyReportSection(含 best_tier/avg_score) → 存储。生成 SHALL 通过 goroutine 异步执行。

物化追加的 section SHALL NOT 参与同日合并，SHALL NOT 参与 section 关系计算。

流水线 SHALL NOT 执行 thread 级别的 tag 交集匹配或 prev_thread_id 赋值。

#### Scenario: 完整流水线执行

- **WHEN** 触发 SemanticBoard #5 在 2026-05-25 的日报生成
- **THEN** 系统 SHALL 按序执行：收集标签 → 质量筛选 → 去重 → LLM分组 → 查询昨日日报 → 并行生成 → 泳道内主题分桶 → 内容化 embedding 生成 → 同日合并 → Watch 物化追加 → section embedding 匹配写入关系表 → 组装存储 → status="done"

#### Scenario: 物化失败不阻断流水线

- **GIVEN** board #5 有一个 active 的 sentence_topic 关注
- **WHEN** 该关注的辅助标签检索在生成中失败
- **THEN** 系统 SHALL 跳过该关注的当期物化并记录日志，日报 SHALL 正常完成并保存，status SHALL NOT 为 failed

#### Scenario: 生成失败

- **WHEN** 流水线中任一步骤失败（如 LLM 调用超时）
- **THEN** 系统 SHALL 设置 status="failed"，保留已完成的中间结果（raw_clusters 等），WS 广播失败状态

#### Scenario: 分桶失败不阻断流水线

- **GIVEN** 某聚类达到分桶阈值
- **WHEN** 分桶的两个 LLM 阶段任一失败或返回非法 JSON
- **THEN** 系统 SHALL 记录 warn 日志并保持该聚类为单 section（`bucket_index=0`、`bucket_title=NULL`），日报 SHALL 正常完成，status SHALL NOT 为 failed

### Requirement: section lane 归属标记

`daily_report_sections` SHALL 包含 `lane_tier` 列（取值 l1_direct / l2_llm / l3_new / watch_keyword / watch_sentence），标识该 section 的分桶来源，供前端展示与下游分析。lane_tier SHALL 在 section 生成时与 `topic_match_confidence` 一同确定并持久化。

watch_keyword section 的 `persistent_topic_id` SHALL 为空；watch_sentence section SHALL 归属其关注的专属持久话题（见 watch-materialized-topic 能力）。

泳道内主题分桶产出的兄弟 section SHALL 继承其来源聚类的 `lane_tier`、`persistent_topic_id` 与 `topic_match_confidence`，并各自以其自身 embedding 计算 `topic_match_distance`。

#### Scenario: section 记录 lane 来源

- **GIVEN** 某 section 由 L1 直挂产生
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 l1_direct，topic_match_confidence 为 anchor_hit

#### Scenario: 物化 section 记录物化来源

- **GIVEN** 关键字物化追加产出一个 section
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 watch_keyword，persistent_topic_id SHALL 为空

#### Scenario: 分桶兄弟继承归属与来源

- **GIVEN** 某 L1 直挂聚类（topic #502，lane_tier=l1_direct，confidence=anchor_hit）被拆成 3 条 section
- **WHEN** 3 条 section 持久化
- **THEN** 三者 SHALL 均为 lane_tier=l1_direct、persistent_topic_id=502、confidence=anchor_hit
- **AND** 三者的 `topic_match_distance` SHALL 各自按其 embedding 与该话题质心计算（允许不同）

### Requirement: 日报聚类裁决 prompt 历史隔离

L2 泳道裁决（`buildL2Prompt`，operation `daily_report.decide_l2_tags`）的 LLM prompt SHALL NOT 注入候选话题的历史叙事文案（`daily_report_threads` 的 title / summary），切断「昨天幻觉 thread → 今天作为 briefs 喂回 → LLM 延续叙事」的渗透闭环。

L2 裁决 prompt MAY 注入以下**非叙事**信号辅助裁决：候选 topic 的 `label`、状态（active / candidate）、最近命中日期、累计命中天数、质心余弦距离、近期 section 的框架名（`cluster_label` / `section_label`，话题命名级别，**非** thread 文案）。

L2 system prompt 的裁决依据措辞 SHALL 与实际注入内容保持一致——基于「标签语义与近期 section 框架」，而非「实际近期内容（thread 文案）」，避免 prompt 自相矛盾误导 LLM。

`promptVersion` SHALL 单调递增且已发布版本 SHALL NOT 复用："3.0" → "4.0"（本约束生效）、"5.0"（section 展示标题内容化，2026-08-29）、"6.0"（泳道内主题分桶，本 change）。

#### Scenario: L2 prompt 不含历史 thread 文案

- **GIVEN** board 的某 active 话题近 7 天生成过 thread（title="半导体链全线跌停"）
- **WHEN** 当天 L2 裁决为某沾边 tag 构建 prompt
- **THEN** prompt SHALL NOT 出现该 thread 的 title 或 summary 文案
- **AND** prompt MAY 出现该话题的 label、状态、最近命中日期、质心距离、近期 section_label

#### Scenario: L2 prompt 保留话题框架信号以区分近似话题

- **GIVEN** 两个 active 话题 label 字面相近但近期 section 框架不同
- **WHEN** L2 裁决为某 tag 构建 prompt
- **THEN** prompt SHALL 提供两者的 section 框架信号以供区分
- **AND** SHALL NOT 提供任一话题的 thread title / summary 文案

#### Scenario: promptVersion 升级

- **WHEN** 日报按本约束生成
- **THEN** `board_daily_reports.generation_prompt_version` SHALL 为 "6.0"

## ADDED Requirements

### Requirement: 泳道内主题分桶

系统 SHALL 在 lane 归属分桶之后、section 组装之前，对**单个聚类内部**做主题方向分桶（intra-lane theme bucketing），把一条过宽的 section 拆成 2-6 条同归属的 section。分桶 SHALL NOT 跨聚类进行，SHALL NOT 改变任何 tag 的话题归属（`persistent_topic_id` / `lane_tier` / `topic_match_confidence` 由上游 lane 分桶决定）。

**触发阈值**：当某聚类的当日线索数 ≥ `daily_report_intra_bucket_min_threads`（默认 6）**或** 其 tag 数 ≥ `daily_report_intra_bucket_min_tags`（默认 10）时触发；总开关 `daily_report_intra_bucket_enabled`（默认开启）关闭时 SHALL 全局保持单 section。

**输入粒度**：分桶输入 SHALL 为该聚类当日的 thread 线索（标题 + 摘要 + 关联 tag），SHALL NOT 以原始 tag 列表逐条作为分组单位（tag 粒度会碎化，实测产出单例桶）。

**两阶段**：
1. **定组名**：组数 k SHALL 由代码计算（`k = clamp(round(线索数 / 2.5), 2, 6)`），并写入 prompt；LLM 只负责给出 k 个主题方向名（≤12 字）与每个方向的当日章节标题（≤30 字）。模型返回的组数不等于 k 时，系统 SHALL 做确定性收敛（合并最小的两组，或从尾部裁剪多余组）后再次校验。
2. **归属**：以第一阶段的方向名作为固定集合，LLM 只做逐条归属，SHALL NOT 新增、改名、合并或拆分分组；温度 SHALL 为 0。

**校验与修复**（确定性，不依赖模型自觉）：归属结果 SHALL 满足 ①每条线索恰好出现一次 ②每组线索数 ≥2 ③组数 ≤6。未覆盖的线索 SHALL 并入与其**最相近的组**——相近度用该线索的 tag 集合与该组 tag 并集的 Jaccard 重叠度衡量（无 embedding 时的确定性语义代理）；重叠度并列或无任何重叠时 SHALL 并入线索数最少的组（并列取组序较早者）；重复出现的线索 SHALL 只保留首处；单例组 SHALL 按同一相近度合并进其他组（无重叠时并入线索数最少者）；**空组（0 条线索）SHALL 直接丢弃、不产出 section**；修复后仍只有 1 组时 SHALL 回退为单 section。

**产出**：每个保留下来的组产出一条 section，共享来源聚类的 `cluster_index`，`bucket_index` 按组顺序 0..k-1，`bucket_title` 为主题方向名，`cluster_label` 为该组当日章节标题（SHALL 遵守「日报文案生成事实锚约束」，且 SHALL NOT 复述泳道/话题名；当日章节标题缺失时，展示标题默底链按**该桶内线索**执行：桶内首条 thread 标题 → 泳道 label，SHALL NOT 回退到整簇其他桶的内容），`cluster_tag_ids` 为该组线索关联 tag 的并集；**桶内线索均无 tag id 时 SHALL 回退为来源聚类的 tag 集**（避免产出无法计算 embedding / 无法归属的空壳 section）。

#### Scenario: 域型宽聚类被拆成多个主题

- **GIVEN** board #3030 某聚类含 18 个 tag、17 条 thread（做游戏、插件、MCP、harness、Skill、安全、推理基础设施混杂）
- **WHEN** 执行泳道内主题分桶
- **THEN** 系统 SHALL 产出 2-6 条 section，各自 `bucket_title` 为一个主题方向名、`cluster_label` 为该组当日章节标题，全部 section 共享同一 `cluster_index` 与同一 `persistent_topic_id`

#### Scenario: 未达阈值不触发分桶

- **GIVEN** 某聚类含 4 条 thread、5 个 tag
- **WHEN** 执行泳道内主题分桶
- **THEN** 系统 SHALL NOT 调用分桶 LLM，保持单 section（`bucket_index=0`、`bucket_title=NULL`）

#### Scenario: 归属漏掉线索被修复归位

- **GIVEN** 归属阶段返回的分组漏掉 2 条线索
- **WHEN** 系统执行确定性校验
- **THEN** 漏掉的线索 SHALL 被并入与其语义最近的组，最终覆盖率 SHALL 为 100%，SHALL NOT 丢线索

#### Scenario: 单例组被合并

- **GIVEN** 归属结果中某组只有 1 条线索
- **WHEN** 系统执行确定性校验
- **THEN** 该组 SHALL 被合并进线索主题最近的其他组，最终每组线索数 SHALL ≥2

#### Scenario: 全组收敛为一组时回退

- **GIVEN** 校验修复后只剩 1 个组
- **WHEN** 系统组装 section
- **THEN** SHALL 回退为单 section（`bucket_index=0`、`bucket_title=NULL`），行为与未分桶一致

#### Scenario: 分桶产物不参与同日合并

- **GIVEN** 某聚类被拆成 3 条兄弟 section，且 `daily_report_section_merge_enabled=true`
- **WHEN** 执行同日 section 相似合并
- **THEN** 三条兄弟 section SHALL NOT 被合并回一条

#### Scenario: 组数指令不被模型遵守时收敛

- **GIVEN** 代码计算的 k=4，LLM 实际返回 6 个方向名
- **WHEN** 系统校验组名集合
- **THEN** 系统 SHALL 按确定性规则合并到 4 组后再进入归属阶段，SHALL NOT 直接把 6 组写入日报

### Requirement: 重新分桶 API

系统 SHALL 提供 `POST /api/daily-reports/:id/rebucket`，请求体含 `section_id`（该日报内任一条 section），用于对**该 section 所在的桶组**（同一 `report_id` + `cluster_index`）重跑泳道内主题分桶。

服务端 SHALL：先取与日报重跑**同键**的事务级串行锁（`(semantic_board_id, period_date)` 粒度），并在锁内复核报告状态；以组内全部 thread 为输入重跑两阶段分桶；按新分桶结果替换该组的 section 行（保留/更新原有行以尽量减少 ID 变化，新增不足的桶行，删除多余的桶行）；thread 行 SHALL 复用并把 `section_id` 迁到新桶，SHALL NOT 删除 thread（关键事件注记因此不丢）；`topic_key_event_markers.source_section_id` SHALL 按 `source_thread_id` 重新派生；`topic_watch_hits.section_id` SHALL 随所在 section 的替换同步迁移或按组删除重建，SHALL NOT 留下悬空命中行；`report_annotations.section_id` SHALL 按 `thread_id` 重新派生（无 thread 的报头注记挂该组 `bucket_index=0` 的 section）；重新计算每条 section 的 `cluster_label` / `bucket_title` / `bucket_index` / `cluster_tag_ids` / `article_count` / `embedding` / `topic_match_distance` / `best_tier` / `avg_score` / `quality_breakdown`（并沿用报告期的 `topic_status_at_report` 快照）；其中 `article_count` SHALL 按该桶线索 `related_article_ids` 的**去重并集**重算（重分桶只持有 thread 事实、不重新选文，故与生成期「Σ tag 文章数」口径存在有意差异，属展示计数）；`best_tier` / `avg_score` / `quality_breakdown` SHALL 取自报告期冻结的 `quality_breakdown` 快照按桶内 tag 子集重算（不重新打分）；重算该话题质心；**双向**重建该组涉及的 section 关系（含跨天入边的反向引用）。

前置门槛：报告 status 非 `done` SHALL 返回 409；目标 section 的 `lane_tier` 为 `watch_keyword` / `watch_sentence` 时 SHALL 返回 400（物化组由关注轨自己管理，不接受主题分桶）；未达「泳道内主题分桶」触发阈值时 SHALL 直接返回 200 + `buckets=1` 且不改动任何数据。

失败（LLM 失败、解析失败、数据库事务失败）SHALL 整体回滚并返回错误；「校验后收敛为 1 组」SHALL NOT 视为失败（同上，200 + `buckets=1` 且不改数据）。响应 SHALL 返回该组最新的 section 列表与 `buckets` 计数。

#### Scenario: 重新分桶替换一组 section

- **GIVEN** 日报 #100 的 `cluster_index=3` 组当前为 1 条 18-tag 的 section
- **WHEN** 调用 `POST /api/daily-reports/100/rebucket` 传入该 section 的 id
- **THEN** 响应 SHALL 返回 2-6 条 section，全部 `cluster_index=3`，原 thread 行的 `section_id` SHALL 已指向新桶

#### Scenario: 重新分桶保留关键事件注记

- **GIVEN** 该组某条 thread 上有 confirmed 关键事件注记
- **WHEN** 执行重新分桶
- **THEN** 注记 SHALL 仍然存在，其 `source_thread_id` SHALL 不变，`source_section_id` SHALL 重新指向该 thread 现在所属的 section

#### Scenario: 拆不动时返回单桶且不改数据

- **GIVEN** 该组内容确实只有一个主题方向
- **WHEN** 调用重新分桶
- **THEN** 响应 SHALL 为 200 且 `buckets=1`，数据库 SHALL NOT 发生 section 替换

#### Scenario: 失败整体回滚

- **GIVEN** 重新分桶的归属阶段 LLM 调用失败
- **WHEN** 请求返回错误
- **THEN** 该组 section 与 thread 的 `section_id` SHALL 保持调用前状态，SHALL NOT 出现部分替换

#### Scenario: 拒绝非完成态日报

- **GIVEN** 日报 #100 的 status 为 generating
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 409，SHALL NOT 修改任何数据

#### Scenario: 拒绝 watch 物化组

- **GIVEN** 目标 section 的 `lane_tier=watch_sentence`
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 400，SHALL NOT 修改任何数据

#### Scenario: 未达阈值不改数据

- **GIVEN** 目标组只有 4 条 thread、5 个 tag（低于阈值）
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 200 + `buckets=1`，SHALL NOT 调用分桶 LLM，SHALL NOT 修改任何 section/thread 行

#### Scenario: 与同日重跑并发时串行化

- **GIVEN** 同一 board 同一天的重跑（SaveReport）与 rebucket 并发发起
- **WHEN** 两者竞争
- **THEN** 两者 SHALL 通过同键事务级 advisory lock 串行化，不出现「thread 已被重跑删除、rebucket 的 UPDATE 静默影响 0 行」或「向已重建报告插入旧 cluster_index 幽灵 section」

#### Scenario: watch 命中与页边注跟随重派生

- **GIVEN** 目标组关联了 `topic_watch_hits` 命中行与 `report_annotations` 页边注
- **WHEN** 重新分桶替换该组 section
- **THEN** `topic_watch_hits.section_id` SHALL 指向 thread 现在所属的 section（或按组重建），SHALL NOT 悬空
- **AND** `report_annotations.section_id` SHALL 按 `thread_id` 重新派生，注记内容不变
