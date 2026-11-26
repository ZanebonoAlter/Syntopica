## Purpose

标签匹配到版块（Board）是版块自动化的核心：用 embedding 相似度加辅助标签做匹配，产出候选与分数。本族定义匹配输入、规则与阈值、缓存、自动触发、分数展示与详情、命中率统计、跨版块关系发现。

## ADDED Requirements

### Requirement: Tag 通过辅助标签匹配 Board

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->


系统 SHALL 按以下优先级判定 tag 与 SemanticBoard 的匹配：组合标签直接命中（composite direct_hit）为最强信号——tag 关联的组合标签与 board composition 挂载的组合标签存在交集时，score=1.0，match_reason="composite_hit"，MUST NOT 标记 direction_mismatch（组合命中即指向一致）。单辅助标签重叠 direct_hit 降级为弱信号：交集数 ≥ direct_hit_min_overlap（默认 2）**且交集数 / tag 的 active 辅助标签总数 ≥ direct_hit_min_ratio（默认 0.5）**时不再给 score=1.0，SHALL 给予 direct_hit_score_factor（默认 0.7，ai_settings 可配）的折扣分并执行方向校验（与间接匹配规则相同的三层 direction check：全库 gap 豁免 → 绝对门 → 相对门（详见 board-direction-check）；未获豁免且低于 direction_sim_threshold 或相对门 gap 达标时标记 direction_mismatch=true）。**交集数达标但占比不足时 SHALL NOT 以 direct_hit 挂载，退回相似度匹配流程（hit_rate / max_sim / weighted 规则）判定，且重叠标签 SHALL NOT 作为该版块的相似度证据参与退回判定（同一批标签不得既作直接命中证据、又作相似度证据——自相似恒 1.0 会变相重新挂载）；剩余无重叠标签的相似度均不达标时该 tag 不挂载该版块**。所有满足条件的 SemanticBoard SHALL 按分数排序后持久化到 topic_tag_board_labels，默认最多挂载 3 个。间接匹配三规则（hit_rate / max_sim / weighted）行为不变。

#### Scenario: 组合标签直接命中
- **WHEN** tag「美债收益率创两周新高」关联组合标签「美债收益率」，版块「美债观察」的 composition 挂载同一组合标签
- **THEN** tag SHALL 挂载到该版块，match_reason="composite_hit"，score=1.0，direction_mismatch=false

#### Scenario: 直接命中 board 构成标签
- **WHEN** tag 的辅助标签中包含 "AI" 和 "机器学习"，而它们都是 board #100 "AI与机器学习" 的构成标签，交集数 ≥ direct_hit_min_overlap（默认 2）且占比 ≥ direct_hit_min_ratio（默认 0.5，2 个交集落在 tag 全部辅助标签中占比过半），且 tag identity embedding 与 board embedding cosine ≥ direction_sim_threshold 或该版块为 tag 全库最优版块
- **THEN** tag SHALL 在 topic_tag_board_labels 中挂载到 board #100，match_reason="direct_hit"，score=direct_hit_score_factor（默认 0.7），direction_mismatch=false

#### Scenario: 交集数达标但占比不足退回相似度
- **WHEN** tag 的 active 辅助标签共 5 个 {美联储, 港股, A股, 美元指数, 非农就业数据}，与 board「中国国内新闻」构成标签交集=2（港股、A股）≥ direct_hit_min_overlap=2，但占比 2/5=0.4 < direct_hit_min_ratio=0.5
- **THEN** tag SHALL NOT 以 direct_hit 挂载到该版块；退回相似度判定时港股/A股 SHALL NOT 计入相似度证据，剩余 3 个标签与该版块相似度均不达阈值时该 tag SHALL NOT 挂载该版块

#### Scenario: direct_hit_min_overlap=1 向后兼容
- **WHEN** direct_hit_min_overlap 设为 1，tag 的辅助标签与 board 构成标签交集=1，且该交集占比 ≥ direct_hit_min_ratio
- **THEN** 1 个交集即以 direct_hit 匹配（降级语义：score=direct_hit_score_factor + 强制三层方向校验），交集判定行为与变更前一致

#### Scenario: 降级 direct_hit 方向校验不通过
- **WHEN** tag 的辅助标签 {国债, 收益率}（占比 2/2=1.0）与 board composition 交集=2 ≥ direct_hit_min_overlap，该版块非 tag 的全库最优版块，且 tag identity embedding 与 board embedding cosine=0.55 < direction_sim_threshold=0.6
- **THEN** tag SHALL 以 match_reason="direct_hit"、score=direct_hit_score_factor（默认 0.7）挂载，direction_mismatch=true（前端默认隐藏）

#### Scenario: 降级 direct_hit 全库最优豁免
- **WHEN** 同上交集条件，但该版块为 tag 对全部 active 版块 identity cosine 的最大者（如具体事件 tag × 宽泛类目版块，cosine=0.55 < 0.6 但全库最高）且未配置地板线或 cosine ≥ 地板线
- **THEN** tag SHALL 以 direct_hit 挂载，direction_mismatch=false（全库最优豁免，计入日报）

#### Scenario: 组合命中与单标签重叠同时存在
- **WHEN** tag 同时满足组合命中（board A）与单标签重叠（board A、B），且组合命中的 board 也满足单标签重叠
- **THEN** board A SHALL 以 composite_hit（score=1.0）记录而非 direct_hit（每对 tag-board 仅记录最高优先级 match_reason）

#### Scenario: 无组合标签时行为
- **WHEN** 系统中不存在任何组合标签（或该 tag/board 均未关联组合标签）
- **THEN** 匹配 SHALL 走降级后的单标签 direct_hit 与间接三规则，与变更前相比仅 direct_hit 的占比门槛、分数与 direction_mismatch 判定结构（新增全库 gap 豁免层）不同

#### Scenario: 交集数不足退回相似度匹配
- **WHEN** tag 的辅助标签中仅 1 个与 board composition 交集，direct_hit_min_overlap=2，且无组合命中
- **THEN** tag SHALL NOT 以 direct_hit 匹配，退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）

#### Scenario: 占比拦下但剩余标签真相似仍可挂载
- **WHEN** tag 的 5 个辅助标签中 2 个与 board 构成交集（占比 0.4 被 direct_hit 拦下），剩余 3 个标签与该版块其它构成标签存在真实语义相似（命中率 > direct_hit_rate）
- **THEN** tag SHALL 以 hit_rate / max_sim / weighted 之一挂载该版块（拦的是证据换赛道，不是真跨界语义相似）

#### Scenario: 占比门槛不影响交集数不足的判定
- **WHEN** tag 的辅助标签交集数 < direct_hit_min_overlap（无论占比多少）
- **THEN** direct_hit 判定 SHALL NOT 通过，与占比门槛无关（两条件是 AND 关系）

### Requirement: 间接匹配三规则

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

系统 SHALL 对无法直接命中的 tag，计算每个 SemanticBoard 的命中率和 max_sim，按以下规则判断挂载：
1. 调整命中率 > direct_hit_rate（默认 0.5）→ 直接挂载，score = hit_rate_sim_blend × maxSimilarity + (1 - hit_rate_sim_blend) × adjustedHitRate
2. max_sim ≥ direct_max_sim（默认 0.8）**且 hits ≥ effective_min_hits（effective_min_hits = min(direct_max_sim_min_hits, N)）且调整命中率 ≥ direct_max_sim_min_hit_rate（默认 0.3）**→ 直接挂载，score = maxSimilarity
3. 加权综合分 ≥ weighted_threshold → 挂载，score = 加权综合分

**【2026-11-12 本 change 起生效】匹配结果不再包含 `Downgraded` 字段，`topic_tag_board_labels` 不再有 `downgraded` 列。**（本 requirement 下部分 Scenario 标题沿用历史命名含「降级」字样——openspec 的 MODIFIED 块按场景名对齐，标题保留以承接主 spec 身份锚，判定语义以各场景正文为准。）min(2, N) 放宽准入本身保留（行为不变）——单辅助标签（keyword 实体标签天然形态）经 max_sim 挂载是普通命中，不再是「降级匹配」，不产生任何标记。

**调整命中率** = hits / max(tag 辅助标签总数, min_effective_sample)。当 tag 辅助标签数 ≥ min_effective_sample（默认 3）时，调整命中率 = 原始命中率。当 tag 辅助标签数 < min_effective_sample 时，分母补到 min_effective_sample，避免样本不足导致命中率虚高。

max_sim 为所有 tag-board 辅助标签对中的最高余弦相似度。hits 为 cosine_sim ≥ sim_threshold 的辅助标签数量。加权综合分 = weight_sim × max_sim + weight_density × 调整命中率。

hit_rate 规则的 score 为 maxSimilarity 和 adjustedHitRate 的加权混合（hit_rate_sim_blend 默认 0.7），确保 score 反映实际匹配质量而非仅密度比例。

#### Scenario: 命中率超阈值直接挂载（multi-aux）
- **WHEN** tag 有 4 个辅助标签（≥ min_effective_sample=3），其中 3 个与 board "地缘政治" 的 sim ≥ sim_threshold，调整命中率 = 3/4=75% > 50%
- **THEN** tag SHALL 挂载到 board "地缘政治"，score = 0.7×maxSim + 0.3×0.75（混合打分）

#### Scenario: max_sim 超阈值且双因子满足（正常匹配）
- **WHEN** tag 有 5 个辅助标签，与 board "AI与机器学习" 的 max_sim=0.85 ≥ 0.8，且其中 2 个辅助标签 sim ≥ sim_threshold（hits=2 ≥ min(2,5)=2），hit_rate=2/5=0.4 ≥ 0.3
- **THEN** tag SHALL 挂载到 board "AI与机器学习"，match_reason="max_sim"

#### Scenario: max_sim 超阈值但 hits 不足
- **WHEN** tag 有 5 个辅助标签，与 board "科技行业ETF" 的 max_sim=0.85 ≥ 0.8，但只有 1 个辅助标签 sim ≥ sim_threshold（hits=1 < min(2,5)=2）
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载到 board "科技行业ETF"（但可能通过加权综合分规则挂载）

#### Scenario: max_sim 超阈值但 hit_rate 不足
- **WHEN** tag 有 10 个辅助标签，与 board 的 max_sim=0.82 ≥ 0.8，hits=2 ≥ min(2,10)=2，但 hit_rate=2/10=0.2 < 0.3
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载（但可能通过加权综合分规则挂载）

#### Scenario: N=1 时降级匹配
- **WHEN** tag 只有 1 个辅助标签（keyword 直入），1 个 hit，max_sim=0.85 ≥ 0.8，hits=1 ≥ min(2,1)=1，调整命中率 0.333 ≥ 0.3
- **THEN** tag SHALL 通过 max_sim 规则挂载，score=0.85，match_reason="max_sim"，与多辅助标签命中无任何字段差异（**场景名沿用历史命名；降级标记已退役，N=1 命中不再携带任何标记**）

#### Scenario: N=2 时正常匹配
- **WHEN** tag 有 2 个辅助标签，与 board 的 max_sim=0.85 ≥ 0.8，hits=2 ≥ min(2,2)=2，调整命中率 2/max(2,3)=0.667 ≥ 0.3
- **THEN** tag SHALL 通过 max_sim 规则挂载，score=0.85

#### Scenario: 加权综合分挂载（无降级概念）
- **WHEN** tag 的辅助标签与 board "中东" 的 max_sim=0.72, 调整命中率=0.4，加权分 = 0.6×0.72 + 0.4×0.4 = 0.592
- **THEN** 如果 0.592 ≥ weighted_threshold，tag SHALL 挂载到 board "中东"；否则不挂载（该规则本就不产生降级标记）

#### Scenario: 无任何 board 匹配
- **WHEN** tag 的辅助标签与所有 board 的匹配均不满足任何规则
- **THEN** tag 暂时无板块归属

### Requirement: 匹配参数用户可调

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

系统 SHALL 允许用户通过配置调整以下匹配参数：semantic_board_match_sim_threshold（默认 0.6）、semantic_board_match_direct_hit_rate（默认 0.5）、semantic_board_match_direct_max_sim（默认 0.8）、semantic_board_match_direct_hit_min_overlap（默认 2，direct_hit 所需最小交集数）、semantic_board_match_direct_hit_min_ratio（默认 0.5，direct_hit 交集数占 tag active 辅助标签总数的最低占比；设为 0 时该门槛关闭，回到仅交集数判定）、semantic_board_match_weight_sim（默认 0.6）、semantic_board_match_weight_density（默认 0.4）、semantic_board_match_weighted_threshold、semantic_board_match_max_boards（默认 3）、semantic_board_match_direct_max_sim_min_hits（默认 2）、semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3）、semantic_board_match_min_effective_sample（默认 3）、semantic_board_match_hit_rate_sim_blend（默认 0.7）。

#### Scenario: 用户修改阈值
- **WHEN** 用户将 semantic_board_match_sim_threshold 从 0.6 调整为 0.7
- **THEN** 后续匹配中，辅助标签与 board 的相似度需 ≥0.7 才计入命中率

#### Scenario: 用户修改 min_hits
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hits 从 2 调整为 3
- **THEN** 后续匹配中，max_sim 规则需要 hits ≥ min(3, N) 才能挂载

#### Scenario: 用户修改 min_hit_rate
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hit_rate 从 0.3 调整为 0.4
- **THEN** 后续匹配中，max_sim 规则需要 hit_rate ≥ 0.4 才能挂载

#### Scenario: 用户调整 direct_hit 占比门槛
- **WHEN** 用户将 semantic_board_match_direct_hit_min_ratio 从 0.5 调整为 0.6
- **THEN** 后续匹配中，direct_hit 需要 交集数/tag active 辅助标签总数 ≥ 0.6 才挂载；调整为 0 时占比门槛关闭

### Requirement: Tag 可属于多个 Board

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

系统 SHALL 允许一个 tag 同时属于多个 SemanticBoard。所有满足匹配规则的 board SHALL 按匹配分从高到低排序，默认最多保留 3 个。同一 event tag 及其文章 SHALL 允许出现在多个版块的日报产物中。

#### Scenario: 多视角挂载含降级标记
- **WHEN** tag "霍尔木兹海峡" 同时满足 board "地缘政治"（命中率 75%）和 board "能源安全"（max_sim 0.82，N=1）
- **THEN** tag SHALL 同时挂载到两个 board；两行记录除 score 与 match_reason 外无任何字段差异（**场景名沿用历史命名；降级标记已退役**）

#### Scenario: 超过归属上限时截断
- **WHEN** tag "AI芯片出口管制" 匹配到 5 个 SemanticBoard，semantic_board_match_max_boards=3
- **THEN** 系统 SHALL 仅保留匹配分最高的 3 个 topic_tag_board_labels 记录

### Requirement: 匹配结果可回填重算

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

系统 SHALL 支持手动触发匹配回填，回填模式包括 all、unassigned、board。回填 SHALL 异步执行，并用最新配置重写受影响 tag 的 topic_tag_board_labels。

#### Scenario: 全量回填
- **WHEN** 用户修改匹配阈值后触发 mode="all" 回填
- **THEN** 系统 SHALL 重新计算所有 active tag 的 SemanticBoard 归属

#### Scenario: 仅无归属回填
- **WHEN** 用户触发 mode="unassigned" 回填
- **THEN** 系统 SHALL 仅处理没有 topic_tag_board_labels 记录的 active tag

#### Scenario: 指定 board 回填
- **WHEN** 用户修改 board #100 的 board_composition 后触发 mode="board" 回填
- **THEN** 系统 SHALL 重新计算可能匹配到 board #100 的 tag 归属

### Requirement: Embedding 配置入口唯一性

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

系统 SHALL NOT 提供独立的 embedding 阈值、维度或模型的全局配置入口。Tag 与 SemanticBoard 匹配参数的唯一用户可调入口 SHALL 为 `semantic_board_match_*` 系列配置（通过标签管理-匹配规则对话框维护）。embedding 的 model、provider、dimension SHALL 由 AI 路由（capability-routes）决定，不接受用户在设置页直接覆盖。

#### Scenario: 设置页不含 embedding 配置栏
- **WHEN** 用户打开设置页查看可用配置分区
- **THEN** 系统 SHALL NOT 展示名为 "Embedding" 或包含相似度阈值/维度/模型字段的配置分区

#### Scenario: 旧 embedding 配置 API 不可用
- **WHEN** 客户端请求 `GET /embedding/config` 或 `PUT /embedding/config`
- **THEN** 系统 SHALL 返回 404（路由未注册）

#### Scenario: 板块匹配阈值滑块不存在
- **WHEN** 用户在设置页查找 "板块匹配阈值" 滑块
- **THEN** 系统 SHALL NOT 提供该控件；调整匹配行为 SHALL 通过 `semantic_board_match_*` 参数进行

#### Scenario: 唯一可调入口仍生效
- **WHEN** 用户在标签管理-匹配规则对话框将 `semantic_board_match_sim_threshold` 从 0.6 调整为 0.72
- **THEN** 后续 tag-board 匹配 SHALL 使用新阈值（验证配置入口收敛后唯一入口的功能完整性）

### Requirement: Backfill 批处理并行

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->


`processJob` SHALL 使用 `errgroup.Group` 并行处理 topic tags，并发数由 `semantic_board_backfill_concurrency`（默认 4）控制。单个 tag 失败 SHALL NOT 取消整个 job。

#### Scenario: Parallel processing with default concurrency

- **WHEN** backfill job 包含 100 个 tags，`semantic_board_backfill_concurrency=4`
- **THEN** 系统 SHALL 最多同时处理 4 个 tag，前一个完成即开始下一个

#### Scenario: Individual failure does not cancel job

- **WHEN** tag #123 匹配失败（如 embedding 缺失），但其他 tags 正常
- **THEN** 系统 SHALL 记录 tag #123 的失败，继续处理剩余 tags

#### Scenario: Concurrency configurable via ai_settings

- **WHEN** 用户将 `semantic_board_backfill_concurrency` 从 4 改为 8
- **THEN** 下一个 backfill job SHALL 以最多 8 个并发处理 tags

#### Scenario: All tags processed despite failures

- **WHEN** backfill job 包含 50 个 tags，其中 3 个失败
- **THEN** 系统 SHALL 处理全部 50 个 tags，最终报告 Processed=47, Failed=3，状态为 completed

### Requirement: 匹配输入加载组合标签

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->

MatchTopicTag 在加载辅助标签之外，SHALL 加载 tag 关联的组合标签（含 embedding）与各 board composition 挂载的组合标签（含 embedding），参与 composite direct_hit 判定。组合标签数据 SHALL 纳入 board match cache，与 board auxiliaries / board embeddings 同等的缓存失效语义。

#### Scenario: 加载组合标签参与匹配
- **WHEN** tag 关联组合标签「美债收益率」且某 board composition 挂载同名组合
- **THEN** 匹配过程 SHALL 识别交集并产出 composite_hit

#### Scenario: 组合标签纳入缓存
- **WHEN** board composition 的组合标签挂载发生变更
- **THEN** board match cache SHALL 失效并在下次匹配时重载组合标签数据

#### Scenario: 组件齐全推导组合
- **WHEN** tag 未显式关联任何组合标签，但挂载的辅助标签集合包含某 active 组合的全部组件
- **THEN** 匹配 SHALL 视为该 tag 挂载此组合（显式关联 ∪ 推导命中），组合命中判定据此生效；缺任一组件或组合 disabled 时 SHALL NOT 推导

### Requirement: SaveEmbedding 清理同 tag 同 type 的旧记录

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


`SaveEmbedding` 在保存新 embedding 记录时，SHALL 删除同一 `topic_tag_id + embedding_type` 下 `text_hash` 不匹配的所有旧记录。

#### Scenario: 保存新 embedding 时清理旧记录
- **WHEN** tag 94712 已有 10 条 identity embedding（不同 text_hash），保存一条新的 identity embedding
- **THEN** 旧的 10 条记录被删除，只保留新的 1 条

#### Scenario: text_hash 匹配时更新而非清理
- **WHEN** 保存的 embedding 的 `topic_tag_id + embedding_type + text_hash` 与已有记录完全匹配
- **THEN** 更新已有记录（当前行为不变），不删除其他记录

### Requirement: tag 删除时 embedding 级联清理（DB FK 兜底）

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


系统 SHALL 保证 `topic_tag_embeddings` 中不存在指向已删除 `topic_tags` 的孤儿行：数据库层 MUST 建立外键约束（topic_tag_id → topic_tags.id）并声明 `ON DELETE CASCADE`，与 GORM 模型的 `OnDelete:CASCADE` 声明保持一致。

#### Scenario: 删除 tag 触发向量级联删除
- **WHEN** 通过任何路径删除一条 topic_tag 记录
- **THEN** 该 tag 关联的所有 topic_tag_embeddings 行被数据库自动删除，无孤儿残留

#### Scenario: 外键约束生效
- **WHEN** 向 topic_tag_embeddings 插入一条 topic_tag_id 指向不存在 tag 的记录
- **THEN** 数据库拒绝该写入（FK 违反）

### Requirement: embedding 模型名列宽不窄于模型注册表

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


存 embedding 模型名的列 SHALL NOT 窄于模型注册表 `ai_models.name` 的列宽（当前两者均为 100 字符）。`topic_tag_embeddings.model` SHALL 接受注册表里能存下的**任意**模型名——包括本地 llama.cpp / GGUF 模型以文件绝对路径注册的名称——SHALL NOT 因超限导致向量保存失败。

模型定义（gorm tag）与 DDL SHALL 保持一致：模型定义中的 `model` 字段长度 SHALL 等于该列实际列宽，SHALL NOT 出现「DDL 已加宽但 tag 仍是旧窄值」的漂移（否则下次 AutoMigrate 会把列改回窄列）。

#### Scenario: 长模型名写入成功

- **WHEN** 一条 `topic_tag_embeddings` 记录的 `model` 为 61 字符的本地模型路径（`/home/zanebono/.syntopica/models/Qwen3-Embedding-4B-Q6_K.gguf`）
- **THEN** 写入成功，`model` 列原样保存该字符串，不抛 `SQLSTATE 22001`（value too long for type character varying）

#### Scenario: 列宽不窄于模型注册表

- **WHEN** 比对 `topic_tag_embeddings.model` 与 `ai_models.name` 的列宽
- **THEN** `model` 列宽 ≥ `name` 列宽（当前均为 100）

#### Scenario: 模型定义与 DDL 一致

- **WHEN** GORM AutoMigrate 依据模型定义同步表结构
- **THEN** `topic_tag_embeddings.model` 列宽保持 100，不被改回 50

#### Scenario: 模型定义变更不得顺带丢掉存量列的 NOT NULL

- **WHEN** 模型定义的 `model` 字段长度变更（`size:50` → `size:100`）触发 GORM AutoMigrate 对**存量列**发 ALTER
- **THEN** `topic_tag_embeddings.model` 的 NOT NULL 约束 SHALL 保持（不因 ALTER 被顺带 DROP）；若已被丢弃，版本化迁移 SHALL 把它恢复（幂等）

#### Scenario: 既有向量不受影响

- **WHEN** 列宽变更在有存量行的表上执行（含既有 `model` 为短名的历史行）
- **THEN** 存量行的 `model` 值与 `embedding` 向量原样保留，无截断、无丢行

### Requirement: 标签相似查询使用 halfvec ANN 两段式

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


系统在按 embedding 相似度查找标签（L2 相似命中路径）时，SHALL 先用 halfvec HNSW 索引召回候选集（候选池 ≥ 50、`hnsw.ef_search ≥ 120`），再在候选集内用 float32 向量距离精确重排后返回结果。两段式的最终排序口径 SHALL 与全量 float32 精确排序一致（top10 召回重合度 = 10/10）。

匹配阈值（MatchThreshold=0.92）、cosine 校验、L1/L2/L3 三级去重语义均不变。

#### Scenario: 相似查询走索引且结果与精确排序一致
- **WHEN** semantic 类向量 ≥ 2 万条且存在 halfvec HNSW partial 索引，随机取 20 个真实标签向量做相似查询
- **THEN** 每个查询的 top10 与全量 float32 精确排序 top10 完全一致（overlap=10/10），且单次查询 p95 < 50ms（EXPLAIN 确认 Index Scan）

#### Scenario: 查询 cast 必须带 typmod
- **WHEN** 相似查询 SQL 将向量 cast 为 halfvec
- **THEN** cast 表达式为 `::halfvec(2560)`（与索引表达式精确匹配），不带 typmod 的 `::halfvec` 写法视为缺陷（不走索引）

#### Scenario: 索引覆盖三类 embedding
- **WHEN** 查看 topic_tag_embeddings 索引
- **THEN** semantic / identity / event_keyword 三类各有一个 partial halfvec HNSW 索引（按 embedding_type 过滤），坏索引 `tmp_hnsw_test` 不存在

### Requirement: embedding 队列 worker 连续处理且有限并发

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


embedding 队列 worker SHALL 在处理完当前任务后立即取下一任务（无固定 2 秒轮询间隔），并 SHALL 以 ≤4 的并发度并行处理任务。任务领取 SHALL 使用 `FOR UPDATE SKIP LOCKED` 防止重复处理。

#### Scenario: 连续处理无空转
- **WHEN** 队列有 100 个 pending 任务且 provider 正常
- **THEN** 任务间间隔中位数 < provider 单次往返耗时 + 1s（无 2s 固定空转）

#### Scenario: 并发上限
- **WHEN** 队列大量积压
- **THEN** 同时处于 processing 状态的任务数 ≤ 4

#### Scenario: worker 重启不残留
- **WHEN** 队列服务 Stop 后再次 Start
- **THEN** 旧 worker goroutine 退出（进程内 worker 计数回到期望值），不存在多 worker 抢队列

### Requirement: 失败自动重试与可观测

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->


任务失败时，若 retry_count < 3，SHALL 在延迟（指数退避，基座 30s）后自动回到 pending；worker SHALL 使用真实 logger 输出处理日志。`GET /api/embedding/queue/status` 响应 SHALL 额外包含 `oldest_pending_age_seconds` 与 `throughput_per_min` 两个字段。

#### Scenario: 失败自动重试
- **WHEN** 任务因 provider 暂时错误失败且 retry_count=1
- **THEN** 任务延迟后回到 pending 并被再次处理，不进入终态 failed

#### Scenario: 重试上限
- **WHEN** 任务第 4 次失败（retry_count=3）
- **THEN** 任务进入终态 failed，等待人工 retry 接口

#### Scenario: status 新增字段
- **WHEN** 调用 GET /api/embedding/queue/status
- **THEN** 响应含 oldest_pending_age_seconds（最老 pending 的年龄秒数）与 throughput_per_min（最近完成速率）

### Requirement: 簇级别 Board Affinity 元数据计算

<!-- from: board-affinity-metadata -->

系统 SHALL 在聚类完成后，为每个簇计算与所有已有板块的亲和度（BoardAffinity），作为参考元数据附加到簇上，不影响聚类结果。

#### Scenario: 簇与已有板块有相似候选
- **WHEN** 簇包含候选 [AI, 大语言模型, GPT]，且已有板块 "人工智能"（board_id=42）包含 embedding 与 "AI" 候选距离 ≤ threshold
- **THEN** 系统 SHALL 在该簇的 board_affinities 中包含 `{board_id: 42, board_label: "人工智能", matching_candidates: 1, avg_distance: <计算值>}`

#### Scenario: 簇与已有板块无相似候选
- **WHEN** 簇包含候选 [新能源, 光伏, 储能]，且无已有板块的任何辅助标签 embedding 与这些候选距离 ≤ threshold
- **THEN** 系统 SHALL 在该簇的 board_affinities 中不包含该板块

#### Scenario: 无已有板块
- **WHEN** 系统处于冷启动阶段，无任何 label_type="board" 的 semantic_labels
- **THEN** 系统 SHALL 返回空的 board_affinities 列表，聚类正常进行

### Requirement: Board Affinity 匹配候选计数

<!-- from: board-affinity-metadata -->

系统 SHALL 对每个簇的每个已有板块，统计簇内与该板块任意辅助标签 embedding 距离 ≤ cluster_distance_threshold 的候选数量作为 matching_candidates。

#### Scenario: 多个候选匹配同一板块
- **WHEN** 簇有 5 个候选，其中 3 个与板块 A 的辅助标签距离 ≤ threshold
- **THEN** 系统 SHALL 设置 matching_candidates=3，avg_distance 为这 3 个候选的最小距离的平均值

### Requirement: Board Affinity 数据随 Cluster 返回

<!-- from: board-affinity-metadata -->

系统 SHALL 将 board_affinities 附加到 UpgradeCluster 响应中，随 upgrade/candidates 接口一起返回前端。

#### Scenario: 前端获取候选数据
- **WHEN** 前端调用 `/api/semantic-boards/upgrade/candidates`
- **THEN** 每个 cluster 对象 SHALL 包含 `board_affinities` 数组，每项含 `board_id`、`board_label`、`matching_candidates`、`avg_distance`

### Requirement: Event tag embedding 完成后自动触发 board 匹配

<!-- from: board-match-auto-trigger -->

系统 SHALL 在 `EmbeddingQueueService.processNext` 完成 event tag 的所有 embedding（identity、semantic、event_keyword）生成后，自动调用 `MatchTopicTag`。匹配结果 SHALL 写入 `topic_tag_board_labels`。

#### Scenario: 新 event tag embedding 完成后自动匹配
- **WHEN** event tag #500 的所有 embedding 生成完毕，且该 tag 有辅助标签
- **THEN** 系统 SHALL 自动调用 `MatchTopicTag(ctx, 500)`，匹配结果写入 `topic_tag_board_labels`

#### Scenario: tag 无辅助标签时跳过匹配
- **WHEN** event tag #501 的 embedding 生成完毕，但该 tag 没有任何辅助标签
- **THEN** 系统 SHALL NOT 调用 `MatchTopicTag`，直接标记 embedding task 完成

#### Scenario: 非 event tag 不触发匹配
- **WHEN** keyword tag #502 的 embedding 生成完毕
- **THEN** 系统 SHALL NOT 调用 `MatchTopicTag`

#### Scenario: 自动匹配失败不阻塞 embedding 完成
- **WHEN** `MatchTopicTag` 调用返回错误
- **THEN** 系统 SHALL 记录 warning 日志，仍将 embedding task 标记为 completed

### Requirement: Board Auxiliary Labels Cache

<!-- from: board-match-cache -->


`SemanticBoardMatchingService` SHALL 缓存 `loadBoardAuxiliaries` 的结果。首次调用时从 DB 加载并缓存；后续调用直接返回缓存值。缓存 SHALL 在 board composition 变更（upgrade confirm、手动合并）后失效。

#### Scenario: Cache miss loads from DB

- **WHEN** `MatchTopicTag` 首次调用时缓存为空
- **THEN** 系统 SHALL 从 DB 加载 board auxiliaries，存入缓存，并返回结果

#### Scenario: Cache hit returns cached data

- **WHEN** `MatchTopicTag` 第二次调用时缓存有效（未失效）
- **THEN** 系统 SHALL 直接返回缓存的 board auxiliaries，不执行 DB 查询

#### Scenario: Cache invalidates on upgrade confirm

- **WHEN** 用户确认一个 board upgrade 建议（create_new 或 merge_into_existing）
- **THEN** 系统 SHALL 清除 board auxiliaries 缓存，下次 `MatchTopicTag` 调用时重新从 DB 加载

### Requirement: Board Embeddings Cache

<!-- from: board-match-cache -->


`SemanticBoardMatchingService` SHALL 缓存 `loadBoardEmbeddings` 的结果（已解析为 `map[uint][]float64`）。缓存 SHALL 在以下任一变更后失效，与 board auxiliaries 缓存同生命周期：

- board composition 变更（upgrade confirm、手动合并、手动增删构成标签）；
- **版块 label 或 description 变更**——该变更会重算版块 embedding（`embed("label. description")`），缓存中的旧向量 SHALL NOT 继续被方向校验使用。

缓存失效 SHALL 在版块本体更新请求的同一请求内完成（更新成功后即失效），SHALL NOT 依赖进程重启或 TTL 过期。

#### Scenario: Cache eliminates parsePgVector overhead

- **WHEN** `MatchTopicTag` 第二次调用时 board embeddings 缓存有效
- **THEN** 系统 SHALL 直接返回缓存的 `map[uint][]float64`，不执行 DB 查询和 `parsePgVector` 解析

#### Scenario: Cache invalidates with auxiliaries

- **WHEN** board composition 变更触发缓存失效
- **THEN** 系统 SHALL 同时清除 board embeddings 缓存

#### Scenario: Cache invalidates on board label change

- **GIVEN** board #1980 的 embedding 缓存中存有向量 E_old
- **WHEN** 用户通过 `PUT /api/semantic-boards/:id` 修改该版块的 label，接口重算出 E_new
- **THEN** 系统 SHALL 在该请求内清除 board embeddings 缓存，下次 `MatchTopicTag` SHALL 重新从 DB 加载 E_new，SHALL NOT 继续使用 E_old 做方向校验

#### Scenario: Cache invalidates on board description change

- **WHEN** 用户修改版块 description（接口重算 embedding）
- **THEN** 系统 SHALL 同样清除 board embeddings 缓存，SHALL NOT 因 description 变更而保留旧向量

#### Scenario: 缓存无 TTL 场景下的失效保证

- **GIVEN** board embeddings 缓存无 TTL（只靠显式失效）
- **WHEN** 版块本体更新接口返回成功
- **THEN** 系统 SHALL 已完成缓存失效，SHALL NOT 留下陈旧向量继续参与方向校验

### Requirement: Active Auxiliary Labels Cache (TTL)

<!-- from: board-match-cache -->


`AuxiliaryLabelService` SHALL 缓存 `loadActiveAuxiliaryLabels` 的结果，TTL 为 5 分钟。过期后下次调用重新从 DB 加载。缓存 SHALL 在新增或禁用辅助标签时立即失效。

#### Scenario: Labels returned from cache within TTL

- **WHEN** 距上次 `loadActiveAuxiliaryLabels` 加载不到 5 分钟，且 `ResolveAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 使用缓存的标签列表进行匹配，不执行全表扫描

#### Scenario: Labels reloaded after TTL

- **WHEN** 距上次加载超过 5 分钟，且 `ResolveAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 重新执行 `loadActiveAuxiliaryLabels` 全表扫描，更新缓存

#### Scenario: Cache invalidated on label creation

- **WHEN** `ResolveAuxiliaryLabel` 的 L3 阶段创建了新标签
- **THEN** 系统 SHALL 清除 active labels 缓存，下次调用重新从 DB 加载

#### Scenario: Cache invalidated on label disable

- **WHEN** `DisableAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 清除 active labels 缓存

### Requirement: Merge Embedding 向量缓存

<!-- from: board-match-cache -->


`AuxiliaryLabelService` SHALL 缓存所有活跃辅助标签的 `merge_embedding` 向量（已解析为 `map[uint][]float64`），TTL 为 10 分钟。`sqlMergeMatcher` SHALL 优先使用缓存数据，避免每次调用都从 DB 加载 ~216 MB 向量数据。

#### Scenario: sqlMergeMatcher 使用缓存向量

- **WHEN** `ResolveAuxiliaryLabel` 的 L2 阶段调用 `sqlMergeMatcher`，且 merge embedding 缓存有效
- **THEN** 系统 SHALL 使用缓存的 `map[uint][]float64` 计算余弦相似度，不执行 `SELECT id, merge_embedding` DB 查询

#### Scenario: 首次调用或缓存过期时从 DB 加载

- **WHEN** merge embedding 缓存为空或已过期（超过 10 分钟）
- **THEN** 系统 SHALL 执行 `SELECT id, merge_embedding FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`，调用 `ParsePgVector` 解析每行，存入缓存

#### Scenario: 缓存在标签新增时失效

- **WHEN** `ResolveAuxiliaryLabel` 的 L3 阶段创建了新标签
- **THEN** 系统 SHALL 同时清除 merge embedding 缓存和 active labels 缓存

#### Scenario: 缓存在标签禁用时失效

- **WHEN** `DisableAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 同时清除 merge embedding 缓存和 active labels 缓存

#### Scenario: 添加别名不影响 merge embedding 缓存

- **WHEN** `addAlias` 被调用（标签的 aliases 字段变化）
- **THEN** 系统 SHALL 清除 active labels 缓存，但保留 merge embedding 缓存（向量数据未变化）

#### Scenario: parsePgVector 仅在缓存加载时调用

- **WHEN** merge embedding 缓存有效，10 次 `ResolveAuxiliaryLabel` 调用发生
- **THEN** 系统 SHALL NOT 为 merge embeddings 调用 `ParsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Tag chip 匹配方式颜色编码

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->

Board 文章列表中每个 tag chip SHALL 根据 `match_reason` 字段用颜色区分匹配方式：
- `direct_hit` → 绿色(#22c55e)
- `hit_rate` → 蓝色(#3b82f6)
- `max_sim` → 橙色(#f59e0b)
- `weighted` → 灰色(#94a3b8)

颜色 SHALL 应用于 chip 的 border 或 background。**【2026-11-12 本 change 起生效】前端 SHALL NOT 再因「降级匹配」降低视觉权重**（降级标记已退役）——所有 chip 一律使用规则色的完整亮度。数据来自 `topic_tag_board_labels` 表的 `match_reason` 与 `score` 字段。

#### Scenario: 文章有三种匹配方式的标签
- **WHEN** 文章 #101 的 filtered_tags 含 GPT-5发布(match_reason="max_sim")、AI芯片(direct_hit)、AI竞赛(weighted)
- **THEN** tag chips SHALL 分别显示为 橙色、绿色、灰色

#### Scenario: 所有标签同一种匹配方式
- **WHEN** 文章 #102 的 filtered_tags 全部为 match_reason="hit_rate"
- **THEN** 所有 tag chips SHALL 显示为 蓝色

### Requirement: Tag chip 分数文字显示

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->

每个 tag chip 内 SHALL 显示分数文字，chip 格式为 `[标签名 分数]`（如 `[GPT-5发布 0.85]`）。分数 SHALL 保留两位小数。direct_hit 的 score 固定为 1.00。**【2026-11-12 本 change 起生效】chip SHALL NOT 再追加 "↓" 后缀**（降级标记已退役）。

#### Scenario: 正常匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85
- **THEN** tag chip SHALL 使用对应规则色（#f59e0b）的完整亮度边框，显示 "相似度 0.85"

#### Scenario: 降级匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85（历史 downgraded=true 的行现已无此字段）
- **THEN** tag chip SHALL 与其它 max_sim chip 一致：完整亮度规则色边框 + "相似度 0.85"，SHALL NOT 追加 "↓"、SHALL NOT 降低不透明度

#### Scenario: direct_hit 和其他规则不受降级影响
- **WHEN** tag chip 的 match_reason="direct_hit" 或 match_reason="hit_rate" 或 match_reason="weighted"
- **THEN** tag chip SHALL 正常显示

#### Scenario: 显示分数
- **WHEN** tag chip "GPT-5发布" 的 score=0.85
- **THEN** chip 文字 SHALL 显示为 `GPT-5发布 0.85`

#### Scenario: direct_hit 分数
- **WHEN** tag chip "AI芯片" 的 match_reason="direct_hit", score=1.0
- **THEN** chip 文字 SHALL 显示为 `AI芯片 1.00`

### Requirement: 文章行最强匹配信息

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->

每篇文章行右侧 end 处 SHALL 显示该文章在当前 board 中的最强匹配信息：匹配方式中文名（直接命中/命中率/相似度/综合）+ 最高分数。系统 SHALL 从该文章所有 filtered_tags 中选取 score 最高的 tag 作为最强匹配。

#### Scenario: 多标签取最高分
- **WHEN** 文章 #101 的 filtered_tags 为 [GPT-5发布(max_sim, 0.85), AI竞赛(hit_rate, 0.92), AI芯片(direct_hit, 1.00)]
- **THEN** 文章行右侧 SHALL 显示"直接命中 1.00"

#### Scenario: 仅一个标签
- **WHEN** 文章 #102 的 filtered_tags 仅有一个 tag (max_sim, 0.78)
- **THEN** 文章行右侧 SHALL 显示"相似度 0.78"

### Requirement: 默认隐藏 direction_mismatch 标签

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->


TagsPage 的 tag chip 列表默认不显示 direction_mismatch=true 的标签。提供"显示方向不符"toggle 开关。开启后显示这些标签，用虚线边框 + "⊘" 后缀标记。

#### Scenario: 默认隐藏
- **WHEN** 文章的 filtered_tags 中存在 direction_mismatch=true 的 tag
- **THEN** chip 列表中只显示非 direction_mismatch 的标签

#### Scenario: toggle 开启
- **WHEN** 用户启用 show_direction_mismatch toggle
- **THEN** 所有标签均显示，direction_mismatch 标签使用虚线边框 + "⊘" 后缀

### Requirement: 匹配详情面板降级说明和方向校验展示

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->

`MatchDetailPanel.vue` 的匹配流程步骤 SHALL 在 max_sim 命中时展示实际使用的 hits 门槛与方向校验结果。步骤 ④（max_sim）展示方向校验结果和 direction_sim 值。**【2026-11-12 本 change 起生效】步骤 SHALL NOT 再显示「⚠ 降级匹配（原阈值 X，因仅有 N 个辅助标签降为 M）」**（降级标记已退役）——minHits 的实际取值由前端自算门槛表达，不带降级语义。

#### Scenario: 降级匹配流程步骤
- **WHEN** tag 以 max_sim 匹配，有 1 个辅助标签，direct_max_sim_min_hits=2
- **THEN** 匹配流程步骤 ④ SHALL 显示命中数值与门槛（如 "✓Smax=0.85 ✓1≥1 命中 ✓R=0.33 → 满足！"），SHALL NOT 出现「⚠ 降级匹配」字样

#### Scenario: 正常匹配流程步骤不变
- **WHEN** tag 以 max_sim 匹配
- **THEN** 匹配流程步骤 SHALL 按原有方式显示，无降级提示

#### Scenario: 方向校验通过
- **WHEN** max_sim 匹配且 direction_sim >= threshold
- **THEN** 步骤 ④ 展示 "方向校验 ✓ sim=X≥Y"

#### Scenario: 方向校验未通过
- **WHEN** max_sim 匹配且 direction_sim < threshold
- **THEN** 步骤 ④ 展示方向校验未通过标记（沿用既有文案）

### Requirement: 匹配详情按需实时计算

<!-- from: match-detail-ondemand -->

系统 SHALL 提供 `GET /api/semantic-boards/:id/match-detail/:tagId` 端点，用户点击 tag chip 时按需实时计算并返回辅助标签逐对匹配明细。不修改匹配流程、不修改表结构、不存储中间数据。

**【2026-11-12 本 change 起生效】响应 SHALL NOT 再包含 `downgraded` 字段**（降级标记已随 retire-downgraded-mount 退役）。`effective_min_hits` 保留：它仍是本次计算实际使用的 minHits 值（= min(direct_max_sim_min_hits, N)），供前端展示判定用门槛，本身不再是「降级」信号。

#### Scenario: 非 direct_hit 场景返回完整 pairs
- **WHEN** tag 以 hit_rate / max_sim / weighted 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries` 为空，`pairs` 展示所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度

#### Scenario: direct_hit 场景同时返回精确匹配和完整 pairs
- **WHEN** tag 以 direct_hit 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries`（精确匹配列表）和完整的 `pairs`（所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度），以及 `hits` / `hit_rate` / `max_similarity` 聚合指标

#### Scenario: max_sim 降级匹配返回 downgraded 标记
- **WHEN** tag 以 max_sim 方式匹配 board，且 `effective_min_hits < direct_max_sim_min_hits`（即 minHits 被降级）
- **THEN** API 返回 SHALL 包含 `effective_min_hits`（实际使用的 minHits 值），且 SHALL NOT 包含任何 `downgraded` 字段

#### Scenario: 正常匹配返回 downgraded=false
- **WHEN** tag 以任何方式匹配 board
- **THEN** API 返回 SHALL NOT 包含 `downgraded` 字段；`effective_min_hits` 等于 min(direct_max_sim_min_hits, N)

### Requirement: getTagMatchDetail 返回 direction_sim

<!-- from: match-detail-ondemand -->


getTagMatchDetail 响应 SHALL 新增 `direction_sim` 字段（float64 或 null）。计算方式：在 handler 层实时计算 cosine(tag identity embedding, board embedding)，**不在** `computeMatchDetail` 内部——direction_sim 是匹配后校验结果，非匹配过程的一部分。

#### Scenario: direction_sim available
- **WHEN** tag identity embedding 和 board embedding 均存在
- **THEN** handler 加载两者 embedding，计算 cosine 值，返回 direction_sim

#### Scenario: direction_sim unavailable
- **WHEN** tag identity embedding 或 board embedding 为 NULL
- **THEN** 响应中 direction_sim 为 null

### Requirement: 返回当前匹配配置参数

<!-- from: match-detail-ondemand -->

系统 SHALL 在匹配详情响应中返回当前生效的匹配配置参数（从 ai_settings 读取），包括 sim_threshold、direct_hit_min_overlap、direct_hit_rate、hit_rate_sim_blend、min_effective_sample、direct_max_sim、direct_max_sim_min_hits、direct_max_sim_min_hit_rate、weight_sim、weight_density、weighted_threshold。

#### Scenario: 响应包含当前生效配置

- **WHEN** 客户端调用 `GET /api/semantic-boards/:id/match-detail/:tagId`
- **THEN** 响应 SHALL 携带从 ai_settings 读取的当前生效配置参数（sim_threshold、direct_hit_min_overlap、direct_hit_rate、hit_rate_sim_blend、min_effective_sample、direct_max_sim、direct_max_sim_min_hits、direct_max_sim_min_hit_rate、weight_sim、weight_density、weighted_threshold）

### Requirement: 命中口径与统计窗口

<!-- from: source-board-hit-rate (2026-09-18 add-source-board-hit-rate) -->

统计 SHALL 以下列口径计算，任何实现（端点、面板、调试脚本）MUST 一致：

- **命中**：一篇文章至少有一个标签经 `topic_tag_board_labels` 挂到 `label_type='board'` 且 `status='active'` 的板块，即为命中。`auxiliary` / `composite` 标签本身不是板块。
- **窗口**：`coalesce(pub_date, created_at) >= now - N 天`，N 由调用方给出；允许值 SHALL 限定为 `7` / `30` / `90`，缺省 `7`，非法值 SHALL 返回 400。窗口不设上界。
- **含已归档文章**：窗口内 `archived = true` 的文章 MUST 计入统计。理由：超出 `max_articles` 的文章会被 `CleanupOldArticles` 归档，高频源的命中文章几乎全在归档区；排除归档会得出相反结论（实测：某源近 7 天 1671 篇中 1574 篇已归档，437 篇命中全部落在归档区）。
- **按文章去重**：任何计数口径（源总量、命中数、板块分布）SHALL 以文章为单位去重，MUST NOT 因一篇文章携带多个标签或命中多个板块而重复计数。
- **未打标两分**：无标签文章 SHALL 区分「打标排队中」（存在 `pending`/`leased` 的打标任务）与「已处理无标签」（无标签且无未完成任务），便于消费方区分管线滞后与真实零标签。

#### Scenario: 一篇文章多标签多板块只计一次
- **WHEN** 一篇文章有 3 个标签，其中 2 个标签分别挂到板块 A 与板块 B
- **THEN** 该源「命中文章数」SHALL 计 1（不是 2），板块 A、板块 B 各计 1

#### Scenario: 归档文章计入窗口统计
- **WHEN** 某源近 7 天入库 100 篇，其中 80 篇因超出 `max_articles` 被归档，仅 5 篇命中板块且全部在归档区
- **THEN** 该源窗口统计 SHALL 为「总量 100、命中 5」，命中率 5%，MUST NOT 报 0

#### Scenario: 非法窗口值被拒绝
- **WHEN** 请求携带 `window=14`
- **THEN** 系统 SHALL 返回 400，不回退默认值静默统计

#### Scenario: 打标未完成与零标签可区分
- **WHEN** 某源窗口内 10 篇无标签文章，其中 7 篇有未完成打标任务、3 篇已处理完
- **THEN** 统计 SHALL 输出「打标排队中 7、已处理无标签 3」，二者 MUST NOT 合并为单一数字

### Requirement: 按订阅源聚合端点

<!-- from: source-board-hit-rate (2026-09-18 add-source-board-hit-rate) -->

系统 SHALL 提供只读端点 `GET /api/feeds/board-hit-stats?window=7`，返回窗口内全部订阅源的统计（不逐源 N+1 查询，单次批量聚合）。每条记录 SHALL 含：`feed_id`、`title`、`tagging_enabled`、`articles`（窗口内文章总数）、`in_board`（命中文章数）、`tagged_no_board`（有标签但未命中）、`untagged_pending`（打标排队中）、`untagged_settled`（已处理无标签）、`hit_rate`（`in_board / articles`，`articles = 0` 时为 0）、`boards`（该源命中板块及篇数列表，同一文章命中多板块时各计一次）。

端点 SHALL 为只读：不写库、不触发打标或匹配、不改变任何源状态。

#### Scenario: 返回全量源的统计
- **WHEN** 系统有 23 个订阅源，请求 `GET /api/feeds/board-hit-stats?window=7`
- **THEN** 响应 SHALL 含 23 条记录（含窗口内 0 篇的源，`articles=0`、`hit_rate=0`）

#### Scenario: 板块分布可超过命中数
- **WHEN** 某源窗口内命中 437 篇，其板块分布为 A=142、B=118、其他若干
- **THEN** `boards` 各项之和 MAY 大于 `in_board`（同一文章命中多板块时各计一次），但 `in_board` MUST 为去重后的文章数

#### Scenario: 只读端点无副作用
- **WHEN** 连续两次调用同一窗口的端点
- **THEN** 两次响应 SHALL 一致（除时间戳字段），且 MUST NOT 新增打标任务或修改任何源配置

### Requirement: 按板块聚合端点

<!-- from: source-board-hit-rate (2026-09-18 add-source-board-hit-rate) -->

系统 SHALL 提供只读端点 `GET /api/semantic-boards/:id/source-breakdown?window=7`，返回该板块窗口内的来源构成：`total_articles`（该板块窗口内命中文章总数，按文章去重）、`source_count`（来源数）与 `sources[]`，每条含 `feed_id`、`title`、`articles`（该源在本板块的命中篇数）、`share`（`articles / total_articles`，`total_articles = 0` 时为 0）、以及该源的 `feed_articles`（窗口内总量）与 `feed_hit_rate`（该源整体命中率）。

板块不存在或非 `board` 类型时 SHALL 返回 404。

#### Scenario: 来源篇数合计等于板块总数
- **WHEN** 板块 #5 窗口内有 437 篇命中文章，来源 8 个
- **THEN** `sources[].articles` 之和 SHALL 等于 `total_articles`（437），`source_count` 为 8

#### Scenario: 上下文列帮助识别只偶尔命中的源
- **WHEN** 某源在本板块有 12 篇，但其窗口内总量 162 篇、整体命中率 44%
- **THEN** 该来源项 SHALL 同时给出 `feed_articles=162` 与 `feed_hit_rate=0.44`

#### Scenario: 板块不存在
- **WHEN** 请求 `GET /api/semantic-boards/999999/source-breakdown`
- **THEN** 系统 SHALL 返回 404

#### Scenario: 空板块
- **WHEN** 该板块窗口内没有任何命中文章
- **THEN** 响应 SHALL 为 `total_articles=0`、`sources=[]`（正常结果，非错误）

### Requirement: 板块页板块内容内的来源构成面板

<!-- from: source-board-hit-rate (2026-09-18 add-source-board-hit-rate) -->

来源构成 SHALL 常驻于版块态势页（`/tags/:boardId`）的治理右轨，形态为「前 3 个来源 + 篇数 + 占比条」，并提供「全部 N 个来源」入口打开治理抽屉中的完整来源表。完整表 SHALL 包含：窗口分段控件（7/30/90 天，默认 7）、汇总行（本板块篇数 / 来源数 / 最大来源及其占比）、排序控件（按篇数 ↓ 默认 / 按该源入板块率 ↑）、来源表（订阅源 | 本板块篇数 | 占本板块比例 | 该源入板块率 | 该源窗口内总量）。

面板 SHALL 随选中板块变化重新拉取数据；旧板块的请求结果 MUST NOT 覆盖新选中板块的面板。面板 SHALL 为只读：MUST NOT 提供降频、关打标、退订等写动作，MUST NOT 因面板操作改变板块或订阅源状态。

#### Scenario: 显示来源构成
- **WHEN** 用户进入某版块的态势页
- **THEN** 右轨 SHALL 按窗口展示前 3 个来源的篇数与占比条

#### Scenario: 打开完整来源表
- **WHEN** 用户点击右轨「全部 N 个来源」
- **THEN** 治理抽屉 SHALL 打开并定位到来源页签，展示完整来源表与窗口/排序控件

#### Scenario: 切换板块重新拉取
- **WHEN** 用户从板块 #5 切换到板块 #8
- **THEN** 面板 SHALL 按板块 #8 重新拉取并展示，旧板块的请求结果 MUST NOT 覆盖新选中板块的面板

#### Scenario: 空态引导切窗口
- **WHEN** 该板块近 7 天没有命中文章
- **THEN** 面板 SHALL 显示「近 7 天没有文章归入本板块」并提示可切换 30/90 天窗口

#### Scenario: 面板无写动作
- **WHEN** 用户查看任一来源行
- **THEN** 行内 SHALL NOT 出现停用、退订、改配置等动作控件（处置入口仍在设置 → 订阅源）

### Requirement: 证据优先的关系候选发现

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

系统 SHALL 从一个明确的内部来源快照（版块、父简报以及 observation 或 research question）启动关系发现，先检索外部证据并提取目标概念，再尝试映射内部目标；系统 MUST NOT 以预置领域关键词或全量版块两两组合代替该流程。手动发现 SHALL 默认可用；自动发现 SHALL 按版块配置且默认关闭，并受每次运行的候选数、搜索次数和超时预算约束。

#### Scenario: 从观察手动发现
- **WHEN** 用户从一条仍属于当前版块简报的 observation 发起关系发现
- **THEN** 系统异步创建以该 observation 不可变快照为来源的发现任务，且不要求用户预先选择目标版块

#### Scenario: 从研究问题手动发现
- **WHEN** 用户从父简报的一条 research question 发起关系发现
- **THEN** 系统以问题及父简报上下文作为来源运行同一发现引擎

#### Scenario: 自动发现默认关闭
- **WHEN** 版块未显式启用自动关系发现并生成一份新简报
- **THEN** 系统 MUST NOT 自动发起外部搜索或创建关系建议

#### Scenario: 自动发现遵守预算
- **WHEN** 已启用自动发现的版块生成新简报且可处理观察数量超过配置预算
- **THEN** 系统只处理确定性排序后的预算内来源并记录跳过数量，MUST NOT 扫描全量版块对

### Requirement: 外部概念到内部目标的保守解析

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

系统 SHALL 根据外部证据中提取的目标概念检索内部版块和泳道，并记录候选、映射依据与解析结果。只有唯一且达到配置门槛的目标才能成为 resolved target；多候选、低于门槛或无候选时 SHALL 保留 `target_concept` 并标记为 `unresolved`，MUST NOT 强制选择最相似目标。

#### Scenario: 唯一目标解析成功
- **WHEN** 一个外部概念仅有一个内部候选达到解析门槛
- **THEN** 建议记录目标版块/泳道 ID、映射依据和解析得分

#### Scenario: 多个候选无法消歧
- **WHEN** 多个内部目标均达到门槛且无法根据证据消歧
- **THEN** 建议保持 `unresolved` 并返回候选列表，不自动绑定其中任何一个

#### Scenario: 外部概念尚无内部目标
- **WHEN** 外部证据提到的概念在内部没有合格版块或泳道
- **THEN** 系统保留该外部概念及证据，且允许后续重新解析

### Requirement: 发现与验证相互隔离

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

候选发现与关系验证 SHALL 使用分离的阶段和快照。验证阶段 SHALL 仅依据关系假设、原始证据、内部目标材料、反证和替代解释，输出 `supported`、`contested`、`insufficient` 或 `rejected`；关系类型 SHALL 限于 `causal`、`common_driver`、`divergence`、`correlated`、`contextual`、`unclear`。模型自行声明的置信度 MUST NOT 直接决定验证结论或排序。

#### Scenario: 支持证据和反证共同进入验证
- **WHEN** 发现阶段产生一个已解析的关系候选
- **THEN** 验证阶段同时接收支持证据、已执行的反证查询结果和替代解释，不继承发现阶段的自评分

#### Scenario: 共同驱动而非直接因果
- **WHEN** 原始材料支持两个对象受第三方因素共同影响但不支持对象之间直接传导
- **THEN** 验证结果为 `supported/common_driver` 或 `contested`，MUST NOT 标为 `causal`

#### Scenario: 证据不足
- **WHEN** 验证材料不能区分竞争解释或缺少可核查依据
- **THEN** 验证结果为 `insufficient`，系统不强造获胜解释

#### Scenario: 关系被反证
- **WHEN** 可核查反证否定候选关系的核心陈述
- **THEN** 验证结果为 `rejected` 并保留反证来源

### Requirement: 可核查的外部证据契约

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

每条持久化建议 SHALL 保留来源 URL、标题、原始片段或正文引用、检索时间、支持/反证用途和工具调用来源。系统 SHALL 对引用执行可机械核对的保守校验；无法在工具原文中核对的引用 MUST 被剔除或标为不可核查。联网、正文抓取或模型步骤失败时 SHALL 记录结构化 gap，MUST NOT 生成伪造证据。

#### Scenario: 引用可在原文中核对
- **WHEN** 候选建议引用博查命中或抓取正文中的文字
- **THEN** 系统保存原始工具结果和可回溯引用，详情接口能够返回其 URL 与检索时间

#### Scenario: 模型输出不存在的引用
- **WHEN** 模型给出的引用无法在对应工具原文中匹配
- **THEN** 系统剔除该引用并降低为材料不足或 gap，MUST NOT 把它计入支持证据

#### Scenario: 博查不可用
- **WHEN** 博查未配置、超时或返回错误且不存在足够的其他可核查材料
- **THEN** 任务诚实结束为材料不足或失败状态并记录原因，不创建 `supported` 建议

### Requirement: 关系建议生命周期与幂等裁决

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

关系建议 SHALL 使用 `unresolved`、`proposed`、`confirmed`、`dismissed`、`expired` 状态。解析或验证不足的候选可保留为 `unresolved`；只有通过验证的候选可进入 `proposed`；只有用户可将 `proposed` 确认为 `confirmed` 或驳回为 `dismissed`。已确认关系超过有效期后 SHALL 进入 `expired`，所有终态转换 SHALL 留存操作者、时间和理由。相同来源快照、目标、关系陈述和证据版本 SHALL 生成稳定幂等键。

#### Scenario: 验证通过仍不自动确认
- **WHEN** 验证结果为 `supported`
- **THEN** 系统最多创建 `proposed` 建议，MUST NOT 自动进入 `confirmed`

#### Scenario: 用户确认建议
- **WHEN** 用户确认一条仍为 `proposed` 且证据未失效的建议
- **THEN** 系统原子地将其转换为 `confirmed` 并记录裁决时间

#### Scenario: 重复发现同一建议
- **WHEN** 同一幂等键已有未解决建议并再次运行发现
- **THEN** 系统不创建重复待处理行，并可更新运行留痕

#### Scenario: 驳回建议进入冷却
- **WHEN** 用户 dismiss 一条建议并提供理由
- **THEN** 系统记录 `dismissed`，并在配置冷却期内不重复创建相同幂等键的建议

#### Scenario: 已确认关系过期
- **WHEN** 当前时间超过已确认关系的有效期
- **THEN** 系统将其视为 `expired`，不再作为有效关系提供给下游

### Requirement: 关系建议可审阅和追溯

<!-- from: cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) -->

系统 SHALL 提供按来源版块、目标版块、状态和时间筛选的建议列表与详情，并允许用户确认、dismiss 及重新解析 unresolved 建议。详情 SHALL 展示来源快照、目标解析依据、关系类型、验证结论、支持证据、反证、gap 和生命周期记录。

#### Scenario: 审阅待处理建议
- **WHEN** 用户打开某版块的关系建议视图
- **THEN** 系统展示与该版块相关的 `proposed` 和 `unresolved` 建议，并明确区分证据、反证与未解决项

#### Scenario: 追溯已确认关系
- **WHEN** 用户从一条已确认关系进入详情
- **THEN** 用户能够追溯到原始版块简报、source observation/question、内部目标和外部证据 URL

### Requirement: evaluateSemanticBoardMatches 接收方向校验数据

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->
<!-- kind: impl -->


evaluateSemanticBoardMatches 函数签名新增两个参数：`tagEmbedding []float64`（tag identity embedding，可为 nil）和 `boardEmbeddings map[uint][]float64`（boardID → board embedding，缺失的 boardID 跳过方向校验）。仅 max_sim 规则匹配成功后执行方向校验。

#### Scenario: max_sim 有方向数据
- **WHEN** max_sim 匹配成功 AND tagEmbedding != nil AND boardEmbeddings[boardID] 存在
- **THEN** 计算 cosine(tagEmbedding, boardEmbeddings[boardID])，相应设置 direction_mismatch

#### Scenario: max_sim 无方向数据
- **WHEN** tagEmbedding 为 nil 或 boardEmbeddings[boardID] 缺失
- **THEN** 跳过方向校验，direction_mismatch=false

### Requirement: MatchTopicTag 加载方向校验数据

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->
<!-- kind: impl -->


MatchTopicTag 在加载 tagAuxiliaries 和 boardAuxiliaries 之外，额外加载 tag identity embedding 和所有活跃 board embedding，传入 evaluateSemanticBoardMatches。

#### Scenario: 加载成功
- **WHEN** tag 有 identity embedding 且 boards 有 embedding
- **THEN** 两者传入 evaluateSemanticBoardMatches

#### Scenario: 部分数据
- **WHEN** 部分 board 缺乏 embedding
- **THEN** 仅含 embedding 的 board 加入 boardEmbeddings map，匹配正常进行

### Requirement: MatchTopicTag 使用缓存数据

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->
<!-- kind: impl -->


`MatchTopicTag` SHALL 从 board match cache 获取 board auxiliaries、board embeddings 和 AI config，而非每次调用都查询数据库。缓存未命中时由缓存层负责加载。

#### Scenario: Cached board data used in match

- **WHEN** `MatchTopicTag` 被调用且 board auxiliaries 缓存有效
- **THEN** 系统 SHALL 使用缓存的 board auxiliaries 和 board embeddings 进行匹配，不执行 `loadBoardAuxiliaries` 或 `loadBoardEmbeddings` 的 DB 查询

#### Scenario: Cache transparent on miss

- **WHEN** `MatchTopicTag` 被调用且缓存为空（首次调用或失效后）
- **THEN** 缓存层 SHALL 自动从 DB 加载数据、缓存并返回，`MatchTopicTag` 逻辑无需感知缓存存在

### Requirement: 存量匹配重算

<!-- from: tag-to-board-matching (2026-05-10 narrative-concept-boards) -->
<!-- kind: impl -->

匹配规则变更（composite_hit 新增 + 单标签 direct_hit 降级）上线后，系统 SHALL 提供一次性全量匹配重算（mode="all" backfill），使存量 topic_tag_board_labels 按新规则重写。重算 SHALL 复用现有回填机制（异步、批处理并行、单 tag 失败不阻塞）。

#### Scenario: 全量重算后 direct_hit 降级生效
- **WHEN** 一次性全量重算完成，某 tag 原以 direct_hit score=1.0 挂载 board X（单标签重叠且方向不符）
- **THEN** 该记录 SHALL 被重写为 score=direct_hit_score_factor、direction_mismatch=true

### Requirement: 所有 tagmanagement 测试迁移到 Postgres

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


`internal/tagmanagement/` 下全部 14 个测试文件 SHALL 使用 `testutil.SetupTestDB(t)` 替代各自的 SQLite setup 函数。其中 13 个直接 import `glebarez/sqlite`，另 1 个（`semantic_board_backfill_test.go`）间接复用即将删除的 `setupSemanticBoardMatchingTestDB`。迁移文件列表：

- `repository/tagger_embedding_test.go`
- `repository/tag_job_queue_test.go`
- `service/core/embedding_test.go`
- `service/core/quality_score_test.go`
- `service/core/metadata_test.go`
- `service/core/hard_merge_test.go`
- `service/core/merge_tags_reembedding_test.go`
- `service/auxlabel/auxiliary_label_service_test.go`
- `service/board/semantic_board_matching_test.go`
- `service/board/semantic_board_upgrade_test.go`
- `service/board/semantic_board_backfill_test.go`
- `service/board/tag_clustering_test.go`
- `handler/semantic_board_handler_test.go`
- `handler/merge_reembedding_queue_test.go`

#### Scenario: 迁移后的测试文件不引用 glebarez/sqlite

- **WHEN** 检查上述文件的 import 声明
- **THEN** 不存在 `"github.com/glebarez/sqlite"` 导入

#### Scenario: 迁移后的测试在 Postgres 下全部通过

- **WHEN** Docker Postgres 运行且执行 `go test ./internal/tagmanagement/... -v`
- **THEN** 上述所有文件中的测试通过

### Requirement: embedding 测试使用真实 Postgres + pgvector

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


`embedding_test.go` 和 `tagger_embedding_test.go` 中的测试 SHALL 使用 `testutil.SetupTestDB(t)`，确保 embedding 写入和相似度查询走 pgvector 代码路径。

#### Scenario: embedding 写入测试验证 vector 列

- **WHEN** 测试通过 `testutil.SetupTestDB` 获取 Postgres 连接并创建一条 `TopicTagEmbedding` 记录
- **THEN** `embedding` (vector) 列被正确写入，查询 `SELECT embedding FROM topic_tag_embeddings` 返回非空结果

#### Scenario: 相似度查询使用 pgvector 距离操作符

- **WHEN** 测试调用 `FindSimilarTags` 或类似相似度搜索函数
- **THEN** 底层 SQL 使用 pgvector `<=>` 操作符，而非 Go 侧遍历比较

### Requirement: auxiliary_label_service 测试使用 Postgres

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


`auxiliary_label_service_test.go` SHALL 使用 `testutil.SetupTestDB(t)`，确保辅助标签匹配逻辑在 Postgres 环境下验证。

#### Scenario: 辅助标签匹配测试在 Postgres 下通过

- **WHEN** 运行 `TestAuxiliaryLabel*` 系列测试
- **THEN** 所有测试通过，包括 embedding 相似度匹配、别名合并、新建标签等场景

#### Scenario: merge embedding 比较使用与生产一致的路径

- **WHEN** `sqlMergeMatcher` 在测试中被调用
- **THEN** 相似度计算行为与生产一致（pgvector SQL 或保留的 Go 侧优化路径）

### Requirement: Board Embeddings 缓存存储已解析数据

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


Board embeddings 缓存 SHALL 存储 `map[uint][]float64`（已解析的 float64 切片），而非原始 pgvector 字符串。缓存加载时执行 `parsePgVector`，后续读取直接使用解析结果。

#### Scenario: parsePgVector called once per cache load

- **WHEN** board embeddings 缓存首次加载（或失效后重新加载），DB 中有 200 个 board embeddings
- **THEN** 系统 SHALL 调用 `parsePgVector` 恰好 200 次（每个 embedding 一次），结果存入缓存

#### Scenario: Subsequent reads skip parsePgVector

- **WHEN** 10 次 `MatchTopicTag` 调用发生在缓存有效期内
- **THEN** 系统 SHALL NOT 为 board embeddings 调用 `parsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Merge Embeddings 缓存存储已解析数据

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


Merge embedding 缓存（`AuxiliaryLabelService` 内）SHALL 存储 `map[uint][]float64`（已解析的 float64 切片），而非原始 pgvector 字符串。缓存加载时执行 `ParsePgVector`，后续读取直接使用解析结果。

#### Scenario: ParsePgVector called once per merge embedding cache load

- **WHEN** merge embedding 缓存首次加载（或失效后重新加载），DB 中有 10K 个活跃辅助标签
- **THEN** 系统 SHALL 调用 `ParsePgVector` 恰好 10K 次（每个 merge_embedding 一次），结果存入缓存

#### Scenario: Subsequent sqlMergeMatcher calls skip ParsePgVector

- **WHEN** 10 次 `ResolveAuxiliaryLabel` L2 调用发生在缓存有效期内
- **THEN** 系统 SHALL NOT 为 merge embeddings 调用 `ParsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Tag Embedding 不缓存

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


Tag identity embedding 和 tag auxiliary embeddings SHALL NOT 被缓存。每次 `MatchTopicTag` 调用 SHALL 从 DB 加载当前 tag 的 embedding 数据（按 topic_tag_id 索引查询，开销极低）。

#### Scenario: Tag embedding loaded per call

- **WHEN** `MatchTopicTag` 被连续调用 100 次处理不同 tags
- **THEN** 系统 SHALL 对每次调用执行 tag embedding 的 DB 查询，不缓存 tag embedding

### Requirement: 存量孤儿 embedding 一次性清理

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


提供一次性维护脚本，分批（单批 ≤5 万行）删除 `topic_tag_embeddings` 中 `topic_tag_id` 无对应 `topic_tags.id` 的行，执行前 MUST 备份相关表、执行后 MUST 复核孤儿计数为 0，然后才允许执行加 FK 迁移。

#### Scenario: 清理后加 FK
- **WHEN** 存量清理完成且孤儿计数复核为 0
- **THEN** FK 迁移执行成功，后续删除自动级联

### Requirement: 队列任务批量调用 embedding

<!-- from: tag-embedding-management (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->


worker 处理任务时，SHALL 将同类型（identity/semantic/event_keyword）的待处理标签合并为批量请求（每批 ≤32 条）调用 provider，并 SHALL 对批内每条文本独立命中/写入 embedding 缓存。

#### Scenario: 批量调用
- **WHEN** 队列积压 10 个标签任务（各含 identity+semantic 两种类型）
- **THEN** provider 收到 identity 批量请求 ≤1 次（≤32 条/批）+ semantic 批量请求 ≤1 次，而非 20 次单条请求

#### Scenario: 批量下缓存按单条命中
- **WHEN** 批量请求中某 3 条文本已有缓存
- **THEN** 该 3 条命中缓存不重复调 provider，其余 miss 条目正常出向量并写入缓存

### Requirement: AI Config Cache (TTL)

<!-- from: board-match-cache -->
<!-- kind: impl -->


`SemanticBoardMatchingService` SHALL 缓存 `loadConfig` 的结果，TTL 为 5 分钟。过期后下次调用重新从 `ai_settings` 加载。

#### Scenario: Config returned from cache within TTL

- **WHEN** 距上次 `loadConfig` 加载不到 5 分钟
- **THEN** 系统 SHALL 返回缓存的 `SemanticBoardMatchConfig`，不查询 `ai_settings`

#### Scenario: Config reloaded after TTL

- **WHEN** 距上次 `loadConfig` 加载超过 5 分钟
- **THEN** 系统 SHALL 重新从 `ai_settings` 加载 13 行配置，更新缓存并重置 TTL

### Requirement: Thread Safety

<!-- from: board-match-cache -->
<!-- kind: impl -->


所有缓存 SHALL 使用 `sync.RWMutex` 保护，支持并发读和互斥写。多个 goroutine 同时调用 `MatchTopicTag`（backfill 并行场景）SHALL NOT 产生数据竞争。

#### Scenario: Concurrent reads during parallel backfill

- **WHEN** 4 个 backfill worker 同时调用 `MatchTopicTag`
- **THEN** 所有 worker SHALL 成功读取缓存，无 panic 或数据竞争

#### Scenario: Concurrent read and cache invalidation

- **WHEN** 一个 goroutine 正在读取缓存，另一个 goroutine 触发缓存失效
- **THEN** 读操作 SHALL 返回失效前的旧数据或等待失效完成后重新加载，不产生竞争条件

### Requirement: 匹配工具函数

<!-- from: match-score-visualization (2026-05-26 board-interaction-overhaul) -->
<!-- kind: impl -->

前端 SHALL 提供 `matchReasonColor(reason: string): string` 和 `matchInfoLabel(tag: BoardArticleTag): string` 工具函数。`matchReasonColor` 返回对应颜色 HEX 值，`matchInfoLabel` 返回"匹配方式中文名 + 分数"字符串。**【2026-11-12 本 change 起生效】`matchReasonColor` SHALL 只接受 reason 单参**（不再有 downgraded 淡化分支），`matchInfoLabel` SHALL NOT 输出 "↓" 后缀。

#### Scenario: matchReasonColor
- **WHEN** 调用 `matchReasonColor("max_sim")`
- **THEN** SHALL 返回 `"#f59e0b"`

#### Scenario: matchInfoLabel
- **WHEN** 调用 `matchInfoLabel({match_reason: "hit_rate", score: 0.75})`
- **THEN** SHALL 返回 `"命中率 0.75"`
