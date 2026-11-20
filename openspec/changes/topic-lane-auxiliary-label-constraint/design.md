# Design: topic-lane-auxiliary-label-constraint

## Context

见 `proposal.md` — Why。此处只记录塑造实现方案的现状事实：

- 归属链路现状：`service/daily_report_lane.go:BucketTagsByCentroid` 按 tag 到话题质心的余弦距离把 tag 分入 L1/L2/L3；L1（dist < 0.18 且话题 active 非 vacuum）**零 LLM 直挂**；L2 交 LLM 做「留/换/新」；`service/daily_report_lane.go:assembleLaneGroups` 把 L1+L2-keep 的 tag 按话题聚成 lane group；`repository/daily_report_assignment.go:planTopicAssignments` 只做 lane 结果到 confidence/distance 的映射，**不做归属决策**。
- 标签边按 `tag_edge_retention_days`（默认 7 天，`internal/tagmanagement/service/core/edge_gc.go`）回收，`article_topic_tags` 与 `topic_tags` 因此是滚动窗口；`daily_report_sections.cluster_tag_ids` 里的 tag id 实测 **92.8% 悬空**（14679/15820 指向已删除的 tag 行），而 `article_topic_tags` 悬空率 0%。
- 远程 embedding（`ai_vendors` id=1，`http://10.11.12.111:8081/v1`）自 2026-11-13 起 30 次 `network_error`、0 成功，airouter 已自动降级到本地 gguf（`ai_vendors` id=7，`127.0.0.1:8808`）。实测两者同文本余弦仅 **0.74**（本地 Q6_K ↔ 本地 Q4_K_M 为 0.98，说明差异不来自量化本身）。
- 标签文本构造：`internal/tagmanagement/service/core/embedding.go:buildTagEmbeddingText` = `label` +（semantic 时）`. description` + `aliases` + ` category`；已用 `text_hash` 校验复现命中率 60/60。

## Goals / Non-Goals

**Goals:**
- 让「双主体」话题（如以黎冲突）不被单侧内容殖民，且不依赖人工逐条纠正。
- 约束由用户以**短词**表达（不要求 description），判定全自动。
- 约束机制对**存量话题零影响**：无约束话题的归属行为与命中计数逐字不变。

**Non-Goals:**
- 不改质心公式、不改 L1/L2 距离阈值、不改话题生命周期状态机。
- 不做向量空间迁移与存量向量重算（远程恢复后自然回归单空间）。
- 不引入「同侧多锚投票」等扩展语义（本期一条约束一锚；误判率若不达标，先调 margin/max_distance）。

## Decisions

### D1. 约束锚以单个向量承载，不存 tag id 集合

- **结论**：约束行只存 `term`（用户短词）+ `term_embedding`（锚向量）+ `embedding_model`，不存「锚 tag id 列表」。
- **落点**：`specs/topic-lane-constraint/spec.md` — Scenario「添加约束写入完整行」
- **取舍**：否决「锚 = 检索出的 tag id 集合」——`article_topic_tags` 边 7 天回收、孤立 tag 随之清理，锚集合会自行失效（`daily_report_sections.cluster_tag_ids` 92.8% 悬空即该机制的实证）；向量不随 GC 生死。

### D2. 覆盖判定用「相对偏好 margin」，不用单一绝对距离阈值

- **结论**：约束 `i` 被覆盖 ⟺ 存在 tag `t` 使 `d(t, A_i) + margin_i < d(t, A_j)`（∀ 其它约束 `j`）；`max_distance` 非空时同时要求 `d(t, A_i) < max_distance_i`。
- **落点**：`specs/topic-lane-constraint/spec.md` — Scenario「双侧 required 被不同标签分腿覆盖」
- **取舍**：否决「单一绝对阈值」——本地空间 section 级模拟实测 τ=0.45 时正样本放行 42~58%、纯以色列误判 6~27%、纯黎巴嫩误判 55%，**ROC 不分离**；改用相对偏好后（margin=0.05、锚编码 event）正样本 64.4%、两类负样本误判均 **0%**。同时否决「同一标签须同时满足两锚」——两锚余弦距离仅 0.27~0.31，τ=0.50 时 88% 的 event 标签同时满足，命中的全是纯以色列组阁新闻。

### D3. 约束锚文本按 `term + " " + "event"` 编码，与被测对象同构

- **结论**：锚向量的输入文本为用户短词拼接固定类别后缀，与 `buildTagEmbeddingText` 的模板同构（`category` 段固定为 `event`，因被测对象是 section 的 event 类 tag）。
- **落点**：`backend-go/internal/tagmanagement/service/core/embedding.go:buildTagEmbeddingText`
- **取舍**：实测分离度排序 V1_prod×event 64.4% > V1_prod×keyword 53.6% > V2_bare×keyword 46.8% > V1_prod×裸词 26.2%；裸词锚最差（两锚间距仅 0.139，判别力坍塌）。

### D4. `margin` 默认 0.05、`max_distance` 默认 0.55，均可按约束覆写

- **结论**：两个参数都存于约束行，per-constraint 可调；默认值作为起点，最终值以真库标定为准。
- **落点**：`specs/topic-lane-constraint/spec.md` — Scenario「添加约束写入完整行」
- **取舍**：margin 取 0.05 而非 0.10——margin=0.10 时正样本放行率从 64.4% 掉到 25.2%；margin=0.15 起正样本基本清零。`max_distance` 保留绝对上限是为了剔除离锚过远、相对偏好不具判别意义的 tag（实测纯以色列 tag 到「黎巴嫩」锚 p50=0.593）。

### D5. 闸门落在 lane group 级（`assembleLaneGroups` 之后、`planTopicAssignments` 之前）

- **结论**：约束覆盖以 **section 的标签集合**为单位判定；lane group 违反其话题的 required 约束时，该 group 不锚定该话题。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups`
- **取舍**：否决 tag 级落点——分腿覆盖要求「不同标签分别满足不同约束」，tag 级判定时 section 的标签集合尚不存在，AND 语义无法成立；且 tag 级无法知道当前 tag 是否是「唯一满足某条约束腿的那个」。

### D6. 违例降级为「补一次 L2 裁决」，LLM 仍选中违例话题则降级 new

- **结论**：lane group 违例 → 走 L2 裁决通道重新选归属（最终 `lane_tier` 仍为 `l2_llm`）；若 LLM 选定的话题其 required 约束仍未被覆盖，沿用既有「switch 到候选集外 → 降级为 new」范式，转 L3 新建 candidate。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_lane.go:parseL2Response`
- **取舍**：否决「直接拒绝归属」——会砍掉本应进入该话题的内容，且丢掉 LLM 的重新判断能力（LLM 可能识别出该内容更属于另一个话题）；补一次 LLM 调用只发生在违例 group 上，量级受约束效果本身限制。

### D7. 向量空间隔离靠 `embedding_model` 标记，跨空间 tag 跳过而非判违反

- **结论**：约束行记录锚向量的模型标识；判定时模型标识与锚不一致的 tag 一律跳过——既不计入覆盖也不判违反。
- **落点**：`specs/topic-lane-constraint/spec.md` — Scenario「跨向量空间的标签被跳过」
- **取舍**：否决「按 model 过滤 tag 集合」——那会把整个 section 判为无覆盖，等同于判违反，把「无法判定」误当成「不满足」；否决「不记 model 直接比距离」——远程↔本地同文本余弦仅 0.74，比出来的距离无意义。

### D8. 锚向量计算走 airouter 当前顺位

- **结论**：调用 `airouter.Router.Embed` 计算锚向量，模型名取自返回值并落库为 `embedding_model`。
- **落点**：`backend-go/internal/platform/airouter/router.go:Embed`
- **取舍**：否决「硬编码指定远程模型」——远程当前不可达（30 次 network_error），且 airouter 的 `ordered_failover` 路由已是既有降级机制（`ai_routes` id=4，priority 1 远程 / priority 2 本地 gguf），不应在业务层重复实现顺位。

### D9. 约束独立成表，不改既有表结构

- **结论**：新建 `board_persistent_topic_constraints`；`board_persistent_topics` 与 `daily_report_sections` 的列结构保持不变。
- **落点**：`specs/topic-lane-constraint/spec.md` — Scenario「添加约束写入完整行」
- **取舍**：否决「在 `board_persistent_topics` 上加约束字段」——约束是集合（一条话题可绑多条），塞不进单行；且存量话题无约束行时闸门天然直通，改既有表会带来无谓的迁移面。

### D10. 前端配置区依赖 `lane-manual-correction-loop` 的编辑面

- **结论**：本 change 只交付数据模型、后端闸门、API 与独立组件；编辑面的接线待前置 change 就绪。
- **落点**：无代码落点：`ui-design.md` 的「入口与入口变更」节，因为编辑容器由 `lane-manual-correction-loop` 提供，本 change 不新建编辑入口
- **取舍**：不与前置 change 合并实现——该 change 尚未开工（0/60 任务）且 `ui-impact=major`，合并会让两个 change 的 ui-design 审批互相阻塞；后端部分不依赖它，可先行。

## Risks / Trade-offs

- **[margin 的最终值未经真库标定]** → 本地模拟的正样本仅 3 个（黎巴嫩侧 tag 库被 7 天 GC 削减），统计力弱 → 实现阶段必须做真库效果核对（见 `test-cases.md` 效果核对节），核对前 margin 视为起点而非定值；核对结论若显示误判不可接受，先调 margin/max_distance，仍不达标则转人工纠正（`lane-manual-correction-loop` 的移除能力）。
- **[远程 embedding 不可用时锚与 tag 向量不同空间]** → 跨空间 tag 被跳过（D7），表现为「约束看起来没生效」而非「误判」；远程恢复（用户计划当晚恢复）后需重算受影响的约束锚 → 提供约束的重算入口（复用 `RecomputeTopicCentroids` 的异步幂等范式）。
- **[违例 group 触发额外 LLM 调用]** → 仅违例时触发，量级受约束效果限制；LLM 调用有成本与延迟 → 闸门在无约束话题上零开销，且违例降级沿用既有 L2 通道，不新增调用路径。
- **[约束话题的 hit_count 增长放缓]** → 这是预期效果（避免单边内容殖民），但会改变话题的活跃度指标 → 在变更说明中明确；「今日重点」「泳道关注度」等依赖命中计数的口径 SHALL NOT 因本 change 改动。
- **[section 的 tag 集合在 `daily_report_sections.cluster_tag_ids` 中 92.8% 悬空]** → 闸门不能用该列做判定输入 → 判定输入改用 `article_topic_tags`（悬空率 0%）经 section→article 关联取得，或在 section 生成时物化当时有效的 tag id 快照。

## Migration Plan

1. **迁移**：`postgres_migrations.go` 新增显式版本迁移建 `board_persistent_topic_constraints`（无维度限定 `vector` 列，与 `unify-vector-column-dimension` 的终态一致）；不依赖 gorm AutoMigrate。
2. **部署**：新表为空 → 所有存量话题无约束 → 闸门直通 → 行为与现状逐字一致；无数据迁移、无旧数据降级问题。
3. **启用**：用户为话题绑定辅助标签后生效；绑定操作需 embedding 服务可用（远程优先，失败降级本地并记录 `embedding_model`）。
4. **回滚**：删除约束行即恢复原行为（闸门在无约束话题上直通）；必要时 `DROP TABLE` + 回滚迁移版本。代码侧回滚走 git revert。

## Open Questions

- margin 的最终取值需真库标定（本地模拟正样本仅 3 个，统计力不足）——不影响 spec 与任务拆分，可在实现阶段的效果核对环节回答。
- 远程 embedding 恢复后，是否需要重算降级期产出的 5 条本地 `topic_tag_embeddings` 向量——属存量向量空间治理，不在本 change 范围。
- 是否需要「同侧多锚投票」以进一步压低误判（当前一条约束一锚）——可在真库核对显示误判不达标时评估，届时可能需要新的 spec Requirement 而非仅调参。
