<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: topic-graph, daily-report, semantic-board -->

## Why

话题泳道会把「不该属于它」的 section 吸进来，且一旦吸进去就会自我强化——被吸进的内容成为质心滚动窗口的一员，质心随之偏移，下一批同类内容距离更近、吸附更容易，形成闭环。实测案例：`board_persistent_topics` id=2「以黎冲突升级：停火破裂与军事打击」（board 1974「中东地缘政治与美伊关系」，active）本是以色列+黎巴嫩双边话题，2026-06~08 吸收的 34 条 section 中 33 条含黎巴嫩侧主体词（8 月 13/13 全命中），2026-10~11 吸收的 32 条 section 中含黎巴嫩侧主体词的为 **0 条**（全部是以色列大选/组阁内政）。

已有的 `anchor-topic-centroid-with-embedding` 修复治不了这个案例，因为它管的是「质心离首义多远」，而本案例的病根在**归属判定链路**：tag 到话题质心的余弦距离小于 `persistent_topic_lane_l1_threshold`（0.18）即 `l1_direct` 直挂，**该段完全不过 LLM、也没有任何语义二次门**（旧的双重确认 AND-gate 已移除，见 `repository/daily_report_assignment.go:145-146`）；且 L2 裁决的 briefs 取自该话题自己已归属 section 的标签，磁铁用自己的历史内容自证（`repository/daily_report_repository.go:1553-1556`）。

更根本的是**向量无法直接表达「双主体」**：以黎语料组与以色列内政语料组的组质心距离仅 **0.0521**，而 L1/L2 阈值是 0.18/0.30（分别大 3.5 倍与 6 倍）。若把「双侧在场」实现为「同一个标签同时满足两个约束锚」，该判据在原理上不成立——实测「以色列」与「黎巴嫩」两个主体锚之间的余弦距离仅 **0.27~0.31**，τ=0.50 时 **88% 的 event 标签同时满足两侧锚**，命中的全是纯以色列组阁新闻。原因是**向量的「近」=「语义同类」而非「语义包含」**：真正的双主体事件句（如「以色列国防部长卡茨警告黎巴嫩真主党」）到单侧主体锚的距离（0.538）反而比纯单侧主体词「以色列总理」（0.236）更远一倍。

可行的判据是**分腿覆盖**：section 的标签集合中，**存在一个标签满足「以色列」约束、另一个标签满足「黎巴嫩」约束**（不同标签可分腿命中）。本地空间实测该判据在 τ=0.45 时正样本 3/3 全部放行、纯以色列误判 15/104（14.4%）；τ=0.40 时误判降至 10/104 但正样本开始被拒——最终阈值需按真实内容标定。

因此本 change 引入「话题辅助标签约束」：用户为话题绑定若干主体短词（如「以色列」「黎巴嫩」），每个约束词带向量；section 的标签逐个与约束词比对，**只有全部 required 约束被分腿覆盖时该 section 才能锚定到该话题**。

## What Changes

- **新增话题约束数据模型**：`board_persistent_topic_constraints`（`topic_id` / `term` 用户输入短词 / `term_embedding` 约束锚向量 / `embedding_model` 向量空间标识 / `mode`（`required`｜`preferred`）/ `threshold` / `created_at` / `updated_at`）。约束以**向量**为载体而非 tag id 集合——`article_topic_tags` 边按 `tag_edge_retention_days`（默认 7 天）回收、孤立 tag 随之清理，任何基于 tag id 的约束会在 7 天后自行失效，向量则与 GC 无关。
- **新增约束锚向量计算**：用户在话题编辑面输入短词（如「以色列」），后端用**当前 embedding 顺位**（`ai_routes` id=4 `ordered_failover`，priority 1 = 远程 `qwen3-embedding`、priority 2 = 本地 gguf）计算向量并落库，同时记录 `embedding_model` 作为向量空间标识。用户**只输入短词**，不输入 description——描述由用户补齐的心智负担过高，约束锚的语义由短词本身承载。
- **新增归属闸门**：section 锚定到话题前，逐个 tag 与该话题的约束锚比对距离。`required` 约束要求 section 的标签集合**覆盖全部** required 约束（AND 语义，`preferred` 仅加权不拦截）。不满足时该 section **不得** `l1_direct` 直挂，改走 L2 交 LLM 重新裁决（复用 `candidate-topic-l2-gate` 的 candidate 降级范式）；LLM 若仍 keep 到约束不满足的话题，按 `parseL2Response` 既有「switch 到候选集外 → 降级为 new」的范式处理。
- **新增约束管理 API**：话题约束的读取与批量替换（幂等），供话题编辑面调用。
- **新增前端配置区**：话题编辑面内新增「辅助标签」区（输入短词、选 `required`/`preferred`、调整阈值、展示已绑定约束）。
- **约束语义按话题配置**：`required`（硬性 AND）与 `preferred`（软性加权）可逐条配置，适配「以黎冲突」这类双边话题与「以色列安全焦虑」这类单边话题。

### 依赖

本 change 的**前端配置区依赖 `lane-manual-correction-loop`** 的泳道修正工作面（话题编辑入口）。该 change 尚未开工（0/60 任务），其 `topic_anchor_overrides`（人工重设首义）与本 change 的约束向量是同一族「人工主权改写话题定义面」的机制。数据模型、后端闸门与 API 不依赖该 change，可先行；前端配置区待其就绪后接线。

### 非目标

- **不改质心公式**：首义锚定 + 滚动均值混合（`anchor-topic-centroid-with-embedding` 红线 17）保持不变。质心漂移与主体准入是两个正交问题——前者治「话题漂成泛新闻磁铁」，本 change 治「话题被单边内容殖民」。
- **不改 L1/L2 阈值**：`persistent_topic_lane_l1_threshold` / `lane_l2_threshold` 保持现值。
- **不改命中计数与生命周期**：`planLifecycle` 仍只更新命中计数、不得自动变更任何 status（红线 1）；约束不影响话题的 candidate/active/archived 状态机。
- **不做向量空间迁移**：不在本 change 内统一远程与本地 embedding 的向量空间，也不重算存量向量。
- **不改 tag 生命周期**：`tag_edge_retention_days`（7 天）保持不变。

## Capabilities

### New Capabilities
- `topic-lane-constraint`: 话题泳道的辅助标签约束——约束锚向量模型、约束语义（required/preferred）、归属闸门判定、约束管理 API 与配置面

### Modified Capabilities
- `persistent-topic`: 话题数据模型新增约束关联；归属判定链在 `l1_direct` 直挂前增加约束准入检查（违例不得直挂），并新增约束的读写接口

## Impact

**后端**（`backend-go/internal/topicgraph/`）：
- `repository/daily_report_topic_repository.go` — 新增 `TopicLaneConstraint` 模型与 CRUD、`LoadTopicConstraints` 读取
- `repository/daily_report_assignment.go` — 约束准入检查接入归属映射（`planTopicAssignments` 上游）
- `service/daily_report_lane.go` — L1 直挂前的约束过滤 + 违例降 L2 的分桶调整
- `service/daily_report_constraint.go`（新）— 约束锚向量计算（调 airouter embedding）与判定纯函数（便于单测）
- `handler/daily_report_handler.go` — 约束读写端点
- `internal/platform/database/postgres_migrations.go` — 显式迁移建表

**前端**（`front/app/features/tags/`）：
- 话题编辑面「辅助标签」配置区（组件落点待 `lane-manual-correction-loop` 的编辑面确定）
- `app/api/dailyReports.ts` — 约束读写 API 客户端

**API**：
- `GET /api/daily-reports/topics/:id/constraints` — 读取话题约束列表
- `PUT /api/daily-reports/topics/:id/constraints` — 批量替换话题约束（幂等，全量覆盖语义）

**部署后影响**：
- 新增一张表 `board_persistent_topic_constraints`，**不改动任何既有表结构**——存量话题无约束记录，行为完全不变（约束为空时闸门直通，零额外开销、零 LLM 调用增量）
- 无数据迁移、无旧数据降级问题：未绑定约束的话题与今天表现逐字一致
- 用户需手动为话题绑定辅助标签才生效；不绑定即维持现状

**需要用户手动执行的操作**：
- 绑定约束：在话题编辑面为「以黎冲突升级：停火破裂与军事打击」绑定「以色列」与「黎巴嫩」两条 `required` 约束
- 存量纠正：本 change **不改历史归属**。已错挂到 id=2 的 2026-10~11 共 32 条 section 仍留在该话题名下，需通过 `lane-manual-correction-loop` 的「移除泳道窗口内的误入节点」能力人工纠正，或等约束生效后由后续日报不再吸入

**风险与已知限制**：
- **向量空间一致性**：约束锚向量由当前顺位计算，而库内 `topic_tag_embeddings` 绝大多数由远程 `qwen3-embedding` 计算（43154/43159）。实测远程与本地 gguf 的同文本余弦仅 **0.74**（而本地 Q6_K ↔ 本地 Q4_K_M 为 0.98，说明差异不来自量化本身）。因此约束锚**必须与 tag 向量同空间**计算——本 change 在计算锚向量时记录 `embedding_model`，并在判定时**只与同 `embedding_model` 的 tag 向量比对**；跨空间的 tag 一律跳过（不计入约束覆盖，也不判违反）。远程顺位不可用时降级到本地算出的约束锚将无法与既有 tag 向量比对，需重算该约束——此为已知限制，不在本 change 内解决。
- **短词锚与事件标签的粒度失配**：实测约束锚（短词）与 event 类标签（长句）的距离基线在 0.42~0.55，显著高于同尺度 tag 之间的距离，约束阈值需按锚逐条标定而非全局一刀切（本 change 支持 per-constraint `threshold`）。
- **约束覆盖率低的话题会显著减少吸附**：required 约束未被覆盖的 section 不再直挂，话题的 hit_count 增长会放缓——这是预期效果（避免单边内容殖民），但会改变话题的活跃度指标。
