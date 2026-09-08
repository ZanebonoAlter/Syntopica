## Purpose

Tag 通过辅助标签匹配 SemanticBoard 的规则和参数，包括间接匹配三规则、多 board 归属和回填。

## Requirements

### Requirement: Tag 通过辅助标签匹配 Board
系统 SHALL 按以下优先级判定 tag 与 SemanticBoard 的匹配：组合标签直接命中（composite direct_hit）为最强信号——tag 关联的组合标签与 board composition 挂载的组合标签存在交集时，score=1.0，match_reason="composite_hit"，MUST NOT 标记 direction_mismatch（组合命中即指向一致）。单辅助标签重叠 direct_hit 降级为弱信号：交集数 ≥ direct_hit_min_overlap（默认 2）时不再给 score=1.0，SHALL 给予 direct_hit_score_factor（默认 0.7，ai_settings 可配）的折扣分并执行方向校验（与间接匹配规则相同的 direction check，低于 direction_sim_threshold 则标记 direction_mismatch=true）。所有满足条件的 SemanticBoard SHALL 按分数排序后持久化到 topic_tag_board_labels，默认最多挂载 3 个。间接匹配三规则（hit_rate / max_sim / weighted）行为不变。

#### Scenario: 组合标签直接命中
- **WHEN** tag「美债收益率创两周新高」关联组合标签「美债收益率」，版块「美债观察」的 composition 挂载同一组合标签
- **THEN** tag SHALL 挂载到该版块，match_reason="composite_hit"，score=1.0，direction_mismatch=false

#### Scenario: 直接命中 board 构成标签
- **WHEN** tag 的辅助标签中包含 "AI" 和 "机器学习"，而它们都是 board #100 "AI与机器学习" 的构成标签，交集数 ≥ direct_hit_min_overlap（默认 2），且 tag identity embedding 与 board embedding cosine ≥ direction_sim_threshold
- **THEN** tag SHALL 在 topic_tag_board_labels 中挂载到 board #100，match_reason="direct_hit"，score=direct_hit_score_factor（默认 0.7），direction_mismatch=false

#### Scenario: direct_hit_min_overlap=1 向后兼容
- **WHEN** direct_hit_min_overlap 设为 1，tag 的辅助标签与 board 构成标签交集=1
- **THEN** 1 个交集即以 direct_hit 匹配（降级语义：score=direct_hit_score_factor + 强制方向校验），交集判定行为与变更前一致

#### Scenario: 降级 direct_hit 方向校验不通过
- **WHEN** tag 的辅助标签 {国债, 收益率} 与 board composition 交集=2 ≥ direct_hit_min_overlap，且 tag identity embedding 与 board embedding cosine=0.55 < direction_sim_threshold
- **THEN** tag SHALL 以 match_reason="direct_hit"、score=direct_hit_score_factor（默认 0.7）挂载，direction_mismatch=true（前端默认隐藏）

#### Scenario: 组合命中与单标签重叠同时存在
- **WHEN** tag 同时满足组合命中（board A）与单标签重叠（board A、B），且组合命中的 board 也满足单标签重叠
- **THEN** board A SHALL 以 composite_hit（score=1.0）记录而非 direct_hit（每对 tag-board 仅记录最高优先级 match_reason）

#### Scenario: 无组合标签时行为
- **WHEN** 系统中不存在任何组合标签（或该 tag/board 均未关联组合标签）
- **THEN** 匹配 SHALL 走降级后的单标签 direct_hit 与间接三规则，与变更前相比仅 direct_hit 分数与 direction_mismatch 行为不同

#### Scenario: 交集数不足退回相似度匹配
- **WHEN** tag 的辅助标签中仅 1 个与 board composition 交集，direct_hit_min_overlap=2，且无组合命中
- **THEN** tag SHALL NOT 以 direct_hit 匹配，退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）

### Requirement: 间接匹配三规则
系统 SHALL 对无法直接命中的 tag，计算每个 SemanticBoard 的命中率和 max_sim，按以下规则判断挂载：
1. 调整命中率 > direct_hit_rate（默认 0.5）→ 直接挂载，score = hit_rate_sim_blend × maxSimilarity + (1 - hit_rate_sim_blend) × adjustedHitRate
2. max_sim ≥ direct_max_sim（默认 0.8）**且 hits ≥ effective_min_hits（effective_min_hits = min(direct_max_sim_min_hits, N)，当 effective_min_hits < direct_max_sim_min_hits 时标记为降级匹配）且调整命中率 ≥ direct_max_sim_min_hit_rate（默认 0.3）**→ 直接挂载，score = maxSimilarity
3. 加权综合分 ≥ weighted_threshold → 挂载，score = 加权综合分

匹配结果 SHALL 包含 `Downgraded` 布尔字段，标识该匹配是否因辅助标签数量不足而降低了 minHits 阈值。`replaceTopicTagBoardLabels` SHALL 将 `downgraded` 持久化到 `topic_tag_board_labels.downgraded` 列。

**调整命中率** = hits / max(tag 辅助标签总数, min_effective_sample)。当 tag 辅助标签数 ≥ min_effective_sample（默认 3）时，调整命中率 = 原始命中率。当 tag 辅助标签数 < min_effective_sample 时，分母补到 min_effective_sample，避免样本不足导致命中率虚高。

max_sim 为所有 tag-board 辅助标签对中的最高余弦相似度。hits 为 cosine_sim ≥ sim_threshold 的辅助标签数量。加权综合分 = weight_sim × max_sim + weight_density × 调整命中率。

hit_rate 规则的 score 为 maxSimilarity 和 adjustedHitRate 的加权混合（hit_rate_sim_blend 默认 0.7），确保 score 反映实际匹配质量而非仅密度比例。

#### Scenario: 命中率超阈值直接挂载（multi-aux）
- **WHEN** tag 有 4 个辅助标签（≥ min_effective_sample=3），其中 3 个与 board "地缘政治" 的 sim ≥ sim_threshold，调整命中率 = 3/4=75% > 50%
- **THEN** tag SHALL 挂载到 board "地缘政治"，score = 0.7×maxSim + 0.3×0.75（混合打分），downgraded=false

#### Scenario: max_sim 超阈值且双因子满足（正常匹配）
- **WHEN** tag 有 5 个辅助标签，与 board "AI与机器学习" 的 max_sim=0.85 ≥ 0.8，且其中 2 个辅助标签 sim ≥ sim_threshold（hits=2 ≥ min(2,5)=2），hit_rate=2/5=0.4 ≥ 0.3
- **THEN** tag SHALL 挂载到 board "AI与机器学习"，match_reason="max_sim"，downgraded=false

#### Scenario: max_sim 超阈值但 hits 不足
- **WHEN** tag 有 5 个辅助标签，与 board "科技行业ETF" 的 max_sim=0.85 ≥ 0.8，但只有 1 个辅助标签 sim ≥ sim_threshold（hits=1 < min(2,5)=2）
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载到 board "科技行业ETF"（但可能通过加权综合分规则挂载）

#### Scenario: max_sim 超阈值但 hit_rate 不足
- **WHEN** tag 有 10 个辅助标签，与 board 的 max_sim=0.82 ≥ 0.8，hits=2 ≥ min(2,10)=2，但 hit_rate=2/10=0.2 < 0.3
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载（但可能通过加权综合分规则挂载）

#### Scenario: N=1 时降级匹配
- **WHEN** tag 只有 1 个辅助标签（keyword 直入），1 个 hit，max_sim=0.85 ≥ 0.8，hits=1 ≥ min(2,1)=1（降级），调整命中率 0.333 ≥ 0.3
- **THEN** tag SHALL 通过 max_sim 规则挂载，score=0.85，downgraded=true

#### Scenario: N=2 时正常匹配
- **WHEN** tag 有 2 个辅助标签，与 board 的 max_sim=0.85 ≥ 0.8，hits=2 ≥ min(2,2)=2，调整命中率 2/max(2,3)=0.667 ≥ 0.3
- **THEN** tag SHALL 通过 max_sim 规则挂载，score=0.85，downgraded=false（min(2,2)=2 未降级）

#### Scenario: 加权综合分挂载（无降级概念）
- **WHEN** tag 的辅助标签与 board "中东" 的 max_sim=0.72, 调整命中率=0.4，加权分 = 0.6×0.72 + 0.4×0.4 = 0.592
- **THEN** 如果 0.592 ≥ weighted_threshold，tag SHALL 挂载到 board "中东"，downgraded=false；否则不挂载

#### Scenario: 无任何 board 匹配
- **WHEN** tag 的辅助标签与所有 board 的匹配均不满足任何规则
- **THEN** tag 暂时无板块归属

### Requirement: 匹配参数用户可调
系统 SHALL 允许用户通过配置调整以下匹配参数：semantic_board_match_sim_threshold（默认 0.6）、semantic_board_match_direct_hit_rate（默认 0.5）、semantic_board_match_direct_max_sim（默认 0.8）、semantic_board_match_direct_hit_min_overlap（默认 2，direct_hit 所需最小交集数）、semantic_board_match_weight_sim（默认 0.6）、semantic_board_match_weight_density（默认 0.4）、semantic_board_match_weighted_threshold、semantic_board_match_max_boards（默认 3）、semantic_board_match_direct_max_sim_min_hits（默认 2）、semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3）、semantic_board_match_min_effective_sample（默认 3）、semantic_board_match_hit_rate_sim_blend（默认 0.7）。

#### Scenario: 用户修改阈值
- **WHEN** 用户将 semantic_board_match_sim_threshold 从 0.6 调整为 0.7
- **THEN** 后续匹配中，辅助标签与 board 的相似度需 ≥0.7 才计入命中率

#### Scenario: 用户修改 min_hits
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hits 从 2 调整为 3
- **THEN** 后续匹配中，max_sim 规则需要 hits ≥ min(3, N) 才能挂载

#### Scenario: 用户修改 min_hit_rate
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hit_rate 从 0.3 调整为 0.4
- **THEN** 后续匹配中，max_sim 规则需要 hit_rate ≥ 0.4 才能挂载

### Requirement: Tag 可属于多个 Board
系统 SHALL 允许一个 tag 同时属于多个 SemanticBoard。所有满足匹配规则的 board SHALL 按匹配分从高到低排序，默认最多保留 3 个。同一 event tag 及其文章 SHALL 允许出现在多个版块的日报产物中。每条 `topic_tag_board_labels` 记录 SHALL 包含 `downgraded` 标记。

#### Scenario: 多视角挂载含降级标记
- **WHEN** tag "霍尔木兹海峡" 同时满足 board "地缘政治"（命中率 75%，downgraded=false）和 board "能源安全"（max_sim 0.82，N=1 降级，downgraded=true）
- **THEN** tag SHALL 同时挂载到两个 board，各自的 downgraded 标记独立记录

#### Scenario: 超过归属上限时截断
- **WHEN** tag "AI芯片出口管制" 匹配到 5 个 SemanticBoard，semantic_board_match_max_boards=3
- **THEN** 系统 SHALL 仅保留匹配分最高的 3 个 topic_tag_board_labels 记录

### Requirement: evaluateSemanticBoardMatches 接收方向校验数据

evaluateSemanticBoardMatches 函数签名新增两个参数：`tagEmbedding []float64`（tag identity embedding，可为 nil）和 `boardEmbeddings map[uint][]float64`（boardID → board embedding，缺失的 boardID 跳过方向校验）。仅 max_sim 规则匹配成功后执行方向校验。

#### Scenario: max_sim 有方向数据
- **WHEN** max_sim 匹配成功 AND tagEmbedding != nil AND boardEmbeddings[boardID] 存在
- **THEN** 计算 cosine(tagEmbedding, boardEmbeddings[boardID])，相应设置 direction_mismatch

#### Scenario: max_sim 无方向数据
- **WHEN** tagEmbedding 为 nil 或 boardEmbeddings[boardID] 缺失
- **THEN** 跳过方向校验，direction_mismatch=false

### Requirement: MatchTopicTag 加载方向校验数据

MatchTopicTag 在加载 tagAuxiliaries 和 boardAuxiliaries 之外，额外加载 tag identity embedding 和所有活跃 board embedding，传入 evaluateSemanticBoardMatches。

#### Scenario: 加载成功
- **WHEN** tag 有 identity embedding 且 boards 有 embedding
- **THEN** 两者传入 evaluateSemanticBoardMatches

#### Scenario: 部分数据
- **WHEN** 部分 board 缺乏 embedding
- **THEN** 仅含 embedding 的 board 加入 boardEmbeddings map，匹配正常进行

### Requirement: 匹配结果可回填重算
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

### Requirement: MatchTopicTag 使用缓存数据

`MatchTopicTag` SHALL 从 board match cache 获取 board auxiliaries、board embeddings 和 AI config，而非每次调用都查询数据库。缓存未命中时由缓存层负责加载。

#### Scenario: Cached board data used in match

- **WHEN** `MatchTopicTag` 被调用且 board auxiliaries 缓存有效
- **THEN** 系统 SHALL 使用缓存的 board auxiliaries 和 board embeddings 进行匹配，不执行 `loadBoardAuxiliaries` 或 `loadBoardEmbeddings` 的 DB 查询

#### Scenario: Cache transparent on miss

- **WHEN** `MatchTopicTag` 被调用且缓存为空（首次调用或失效后）
- **THEN** 缓存层 SHALL 自动从 DB 加载数据、缓存并返回，`MatchTopicTag` 逻辑无需感知缓存存在

### Requirement: Backfill 批处理并行

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

### Requirement: 存量匹配重算
匹配规则变更（composite_hit 新增 + 单标签 direct_hit 降级）上线后，系统 SHALL 提供一次性全量匹配重算（mode="all" backfill），使存量 topic_tag_board_labels 按新规则重写。重算 SHALL 复用现有回填机制（异步、批处理并行、单 tag 失败不阻塞）。

#### Scenario: 全量重算后 direct_hit 降级生效
- **WHEN** 一次性全量重算完成，某 tag 原以 direct_hit score=1.0 挂载 board X（单标签重叠且方向不符）
- **THEN** 该记录 SHALL 被重写为 score=direct_hit_score_factor、direction_mismatch=true
