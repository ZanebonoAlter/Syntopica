<!-- domain: daily-report, topic-graph -->

## Purpose

管理 DailyReportSection 之间的多对多跨天关系，通过 `daily_report_section_relations` 关系表存储。支持同日 section 两阶段合并（embedding 确定性合并 + LLM 仲裁），以及跨日 section embedding 匹配写入关系。

## Requirements

### Requirement: 同日 Section 两阶段合并
日报生成管线中、`SaveReport()` 落库之前，系统 SHALL 对同日 sections 执行两阶段合并以消除聚类过碎问题。合并整体受配置开关 `daily_report_section_merge_enabled`（存 `ai_settings`，默认 false）控制：开关关闭时系统 SHALL 跳过两阶段合并，sections 按上游 lane 管线原始分组原样落库。

**锚定边界（前置过滤）**
lane 管线的 keep/switch/new 归因裁决是系统记录，展示层合并不得跨越。所有合并候选对（Stage 1 确定性与 Stage 2 灰区）在建边前 SHALL 先过锚定边界校验，仅以下两类 pair 允许合并：

- 双方 `MatchedTopicID` 均非 NULL 且相等（同话题当日分组）；
- 双方 `MatchedTopicID` 均 NULL（同属新叙事/未锚定池）。

`MatchedTopicID` 不同、或 NULL 与非 NULL 混合的 pair SHALL 被拒绝：不进入确定性合并、不进入 LLM 仲裁、不参与传递闭包。边界过滤在建边前执行保证传递闭包的连通分量内锚定必然一致。

**Stage 1：确定性合并（embedding）**
系统 SHALL 计算所有通过锚定边界的同日 section pairs 的 embedding cosine distance。distance < 0.20 的 pairs SHALL 自动合并为一个 section。

合并规则：
- 保留 `article_count` 最大的 section 作为主 section
- 被合并 section 的 threads SHALL 迁移到主 section 下
- 主 section 的 `cluster_label` 不变
- `cluster_tag_ids` SHALL 合并两个 section 的 tag IDs
- `article_count`、`best_tier`、`avg_score` SHALL 重新计算（合并后的值）
- 被合并 section SHALL 从 sections 列表中移除

连通性：如果 A↔B 和 B↔C 都是合法边且距离均 < 0.20，则 A、B、C SHALL 合并为一个 section（使用传递闭包；边界过滤在建边前执行，闭包不会跨越锚定）。

**Stage 1 审计**
确定性合并的每个候选对（含被锚定边界拒绝的对）SHALL 记录审计日志：双方 `cluster_label`、`MatchedTopicID`、lane_tier、距离、合并或拒绝结果。与 Stage 2 灰区仲裁的 LLM 调用日志共同构成可回放审计面。

**Stage 2：LLM 仲裁（灰色地带）**
通过锚定边界且距离在 0.20 - 0.25 之间的 pairs SHALL 批量送 LLM 判断是否合并。

LLM 输入：每个 candidate pair 的 `(section_a_label, section_a_tag_labels[], section_b_label, section_b_tag_labels[])`。
LLM 输出：`merge_pairs: [[index_a, index_b], ...]` 列表。
LLM 判定为合并的 pairs SHALL 按 Stage 1 相同规则合并。

合并完成后，系统 SHALL 继续 relation 写入逻辑（基于合并后的 sections）。

#### Scenario: 合并开关关闭
- **WHEN** `daily_report_section_merge_enabled=false`（默认）且同日存在多个语义相近的 sections
- **THEN** 系统 SHALL 跳过两阶段合并，sections 按 lane 管线原始分组落库，不产生任何合并

#### Scenario: Stage 1 确定性合并（同话题）
- **WHEN** sections [A, B] 同属 topic 7（MatchedTopicID 均为 7），distance=0.15
- **THEN** 系统 SHALL 合并 A 和 B 为一个 section（保留 article_count 更大的），移除另一个

#### Scenario: 不同话题拒绝合并
- **WHEN** sections [A(topic 7), B(topic 12)] distance=0.11
- **THEN** 系统 SHALL 拒绝合并，A、B 各自独立落库，该 pair 不进入 LLM 仲裁

#### Scenario: 新叙事不被锚定 section 吸收
- **WHEN** sections [A(topic 7, l1_direct), B(MatchedTopicID=NULL, l3_new)] distance=0.14
- **THEN** 系统 SHALL 拒绝合并，B 作为独立新叙事 section 落库并在 SaveReport 时走 auto_new 创建 candidate topic

#### Scenario: 两个新叙事 section 可合并
- **WHEN** sections [A(NULL, l3_new), B(NULL, l3_new)] distance=0.18
- **THEN** 系统 SHALL 允许该 pair 进入正常两阶段合并流程

#### Scenario: 传递闭包不跨越锚定边界
- **WHEN** sections [A(topic 7), B(topic 7), C(topic 12)]，A↔B=0.15（合法边），B↔C=0.18（跨界被拒）
- **THEN** 系统 SHALL 仅合并 A、B，C 独立落库

#### Scenario: Stage 2 LLM 仲裁合并
- **WHEN** sections [A, B] 同属 topic 7，distance=0.21，LLM 判定为 merge=true
- **THEN** 系统 SHALL 合并 A 和 B

#### Scenario: Stage 2 LLM 仲裁拒绝合并
- **WHEN** sections [A, B] 同属 topic 7，distance=0.23，LLM 判定为 merge=false
- **THEN** 系统 SHALL 保留 A 和 B 为独立 section

#### Scenario: 无灰色地带 pairs
- **WHEN** 同日所有通过边界校验的 section pairs 距离均 < 0.20 或 > 0.25
- **THEN** 系统 SHALL 跳过 Stage 2，不调用 LLM

#### Scenario: 确定性合并审计日志
- **WHEN** Stage 1 处理候选对 [A(topic 7), B(topic 12)] distance=0.19 且被锚定边界拒绝
- **THEN** 系统 SHALL 记录含双方 label、MatchedTopicID、lane_tier、距离、拒绝原因的审计日志


### Requirement: Section 关系表
系统 SHALL 创建 `daily_report_section_relations` 表，存储 Section 之间的多对多跨天关系。

字段：id (SERIAL PK), from_section_id (INT NOT NULL REFERENCES daily_report_sections(id)), to_section_id (INT NOT NULL REFERENCES daily_report_sections(id)), distance (FLOAT NOT NULL), created_at (TIMESTAMP DEFAULT NOW())。

约束：
- `from_section_id` 指向较早日期的 section，`to_section_id` 指向较晚日期的 section
- `(from_section_id, to_section_id)` SHALL 有 UNIQUE 约束防止重复关系

索引：
- `idx_section_relations_from` ON (from_section_id)
- `idx_section_relations_to` ON (to_section_id)

#### Scenario: 创建关系表
- **WHEN** 数据库迁移运行
- **THEN** 系统 SHALL 创建 `daily_report_section_relations` 表，包含上述字段、约束和索引

#### Scenario: 防止重复关系
- **WHEN** 尝试插入 (from=10, to=20, distance=0.25) 而该关系已存在
- **THEN** 数据库 SHALL 拒绝插入

### Requirement: 关系写入逻辑
`SaveReport()` 事务中，新 sections 写入数据库后（含两阶段合并），系统 SHALL 对该 board 执行全量关系重建：清除该 board 所有 relation，按相邻天对逐对执行三阶段二分图匹配（Phase 1 匈牙利 1:1 + Phase 2 split/merge + Phase 3 skip-day），重新写入 relation。

embedding 为空的 section SHALL 跳过关系写入。section 的 embedding SHALL 基于 section 内容聚合文本生成（见 section-content-embedding 能力：所聚 tag 的 label/description/代表文章摘录），SHALL NOT 再基于 `cluster_label` 标题文本。

distance 字段存储 pgvector 计算的 cosine distance 值，写入方式使用 raw SQL `INSERT ... ON CONFLICT DO UPDATE` 确保 upsert 时 distance 被正确写入。

以下旧函数/逻辑 SHALL 被删除，由二分图匹配取代：
- `shouldWriteRelation`（两层时间过滤）→ 由 Phase 1 penalty 机制取代
- `competitiveFilter`（候选竞争过滤）→ 由 Phase 2 gap 检测取代
- `hasContinuationInIntermediateDays`（中间天延续检查）→ 由 Phase 3 skip-day 逻辑取代
- `matchCandidate` 结构体 → 由新的匹配结果结构取代

此外，`SaveReport` 中**旧 relations 删除逻辑**（line 64-70，删除当天旧 section 的 relation）SHALL 一并移除——`RebuildBoardRelations` 会清除整个 board 的 relations，旧逻辑变为冗余。

**回刷支持**：系统 SHALL 提供 `RebuildBoardRelations(tx *gorm.DB, boardID uint) error` 核心函数，接收 `*gorm.DB`（事务或裸 DB），内部不做事务管理，由调用方控制事务生命周期。

`BackfillRelations(boardID uint)` SHALL 为薄包装：自行开启事务，调用 `RebuildBoardRelations(tx, boardID)`，提交事务。`BackfillAllRelations()` SHALL 查询所有有 embedding section 的 board 逐个调用 `BackfillRelations`。

**失败处理**：`RebuildBoardRelations` 任何阶段失败时，整个 board 的 relation 重建回滚（由调用方事务保证），报告保存本身不受影响（仍以 warn 日志记录，不中止报告保存）。半完成的拓扑比没有拓扑更糟糕，选择全有或全无。

#### Scenario: SaveReport 触发全量重连
- **WHEN** 新报告保存到 board 2853
- **THEN** 系统 SHALL 清除 board 2853 的所有 relation，按相邻天对逐对执行三阶段二分图匹配，写入新的 relation

#### Scenario: 单个相邻天完美延续
- **WHEN** Day_i section "美伊谈判进展" (id=692) 和 Day_{i+1} section "美伊谈判动态" (id=788) embedding distance=0.073
- **THEN** Phase 1 匈牙利匹配 SHALL 产生 primary relation (from=692, to=788, distance=0.073)

#### Scenario: 不相关 section 不被强制匹配
- **WHEN** Day_i section "日本推进北约合作" 和 Day_{i+1} section "塞尔维亚经济危机" distance=0.337 > penalty
- **THEN** 匈牙利算法 SHALL 不匹配这两个 section（penalty 机制阻止硬凑）

#### Scenario: 距离超阈值不写关系
- **WHEN** 某新 section 与相邻天所有旧 section 的 distance 均 > penalty=0.28
- **THEN** 系统 SHALL 不为该 section 写入任何 relation，其 status 为 emerging

#### Scenario: 空 embedding 跳过
- **WHEN** 新 section 的 embedding 为 NULL
- **THEN** 系统 SHALL 跳过该 section 的关系写入

#### Scenario: 日期不连续时视为相邻天
- **WHEN** board 只有 6/1 和 6/3 的报告（6/2 无报告）
- **THEN** 6/1 和 6/3 SHALL 被视为相邻天，执行 Phase 1 匹配

#### Scenario: BackfillRelations 全量回刷
- **WHEN** 运行 `BackfillRelations(boardID=2853)`
- **THEN** 系统 SHALL 删除 board 2853 涉及的所有 relation，按相邻天对逐对执行三阶段二分图匹配

#### Scenario: BackfillAllRelations 批量回刷
- **WHEN** 运行 `BackfillAllRelations()`
- **THEN** 系统 SHALL 查询所有有 embedding section 的 board，逐个调用 `BackfillRelations`

#### Scenario: BackfillSectionEmbeddings Phase 2
- **WHEN** 运行 `BackfillSectionEmbeddings`
- **THEN** Phase 1 按 section-content-embedding 能力的内容化规则生成/重算 embedding 后，Phase 2 SHALL 对每个 board 调用 `BackfillRelations(boardID)`

### Requirement: Section status 动态推导（两阶段）
系统 SHALL 在 timeline/lifecycle API 返回时动态推导 section 的 `status`（关系状态）和 `ended`（结束标记），不存储在数据库中。

**阶段一：关系状态（status）**——描述该 section 与前驱的关系（按优先级从高到低）：
1. 无 from 关系（无 relation 的 to_section_id 指向它）→ `emerging`
2. 有多个 from 关系（to 入度 > 1，多个旧 section 指向它）→ `merge`
3. from 的出度 > 1（前驱 section 还被其他新 section 指向，该 section 是分化出的子叙事之一）→ `split`
4. 有 from 关系且 from 出度 = 1 → `continuing`

注意：一个 section 可以同时是 split 和 merge。取优先级 merge > split > continuing。

**阶段二：结束标记（ended）**——描述该 section 是否无后继：
- 无 to 关系（无 relation 的 from_section_id 指向它）且 不是最新一天 → `ended = true`
- 最新一天的 section 即使无 to 关系 SHALL NOT 被标记为 ended

#### Scenario: 新兴叙事
- **WHEN** section #50 无任何 relation 的 to_section_id 指向它
- **THEN** status SHALL 为 `emerging`，ended SHALL 为 false

#### Scenario: 延续叙事
- **WHEN** section #60 的唯一 from relation 指向 section #50，且 section #50 只有 #60 这一个 to relation
- **THEN** section #60 的 status SHALL 为 `continuing`

#### Scenario: 叙事分化
- **WHEN** section #50 被 section #60 和 #61 同时指向（#50 的出度 = 2）
- **THEN** section #60 和 #61 的 status SHALL 为 `split`（它们是从 #50 分化出的子叙事）

#### Scenario: 叙事合并
- **WHEN** section #70 被 section #50 和 #55 同时指向（to 入度 = 2）
- **THEN** section #70 的 status SHALL 为 `merge`

#### Scenario: 延续后结束
- **WHEN** section #50 在 05-25（有 from 关系，status=continuing），无任何 relation 的 from_section_id 指向它（无后续），且时间范围最新天为 05-28
- **THEN** section #50 的 status SHALL 为 `continuing`，ended SHALL 为 true

#### Scenario: 新兴后结束（一日叙事）
- **WHEN** section #40 在 05-25（无 from 关系，status=emerging），无后续 relation，且时间范围最新天为 05-28
- **THEN** section #40 的 status SHALL 为 `emerging`，ended SHALL 为 true

#### Scenario: 最新一天不标记 ended
- **WHEN** section #80 在最新一天 05-28，无后续 relation
- **THEN** section #80 的 ended SHALL 为 false（无论 status 是什么）

### Requirement: Section 关系查询 API
系统 SHALL 提供 `GET /api/semantic-boards/:id/section-timeline?days=14` 端点，返回该板块最近 N 天所有 section 的扁平列表（含动态推导的 status），以及所有 section 间关系。

响应格式：
```json
{
  "sections": [SectionTimelineNode...],
  "relations": [{"from_id": 50, "to_id": 60, "distance": 0.25}, ...]
}
```

SectionTimelineNode SHALL 包含 id、report_id、period_date、cluster_label、status（动态推导，emerging/continuing/split/merge）、ended（boolean，动态推导）、article_count、thread_count。

#### Scenario: 查询板块 section 时间线和关系
- **WHEN** 请求 `GET /api/semantic-boards/5/section-timeline?days=14`
- **THEN** 系统 SHALL 返回 board #5 最近 14 天的所有 section 和它们之间的关系

#### Scenario: 无数据返回空
- **WHEN** 板块无 section
- **THEN** 系统 SHALL 返回 `{sections: [], relations: []}`

### Requirement: Section Lifecycle API 适配关系表
`GET /api/daily-reports/sections/:id/lifecycle` SHALL 改为基于关系表查询。返回目标 section 的完整关系网络：沿 from 关系向前追溯，沿 to 关系向后扩展，返回所有相关 section（含动态推导的 status 和 ended）和 relation。

#### Scenario: 查询 section 生命周期含分叉
- **WHEN** 请求 `GET /api/daily-reports/sections/60/lifecycle`，section #60 from → #50，#50 同时被 #60 和 #61 指向
- **THEN** 系统 SHALL 返回 sections [#50, #60, #61] 和 relations [{from:50,to:60}, {from:50,to:61}]

#### Scenario: 孤立 section
- **WHEN** 请求 `GET /api/daily-reports/sections/20/lifecycle`，section #20 无任何 relation
- **THEN** 系统 SHALL 返回 sections [#20] 和空 relations
