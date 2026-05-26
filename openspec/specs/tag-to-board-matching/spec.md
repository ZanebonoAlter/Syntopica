## Purpose

Tag 通过辅助标签匹配 SemanticBoard 的规则和参数，包括间接匹配三规则、多 board 归属和回填。

## Requirements

### Requirement: Tag 通过辅助标签匹配 Board
系统 SHALL 通过 tag 关联的辅助标签与 SemanticBoard 的构成标签计算匹配关系，不再使用 tag embedding 与 board embedding 直接比对。所有满足条件的 SemanticBoard SHALL 按分数排序后持久化到 topic_tag_board_labels，默认最多挂载 3 个。

#### Scenario: 直接命中 board 构成标签
- **WHEN** tag 的辅助标签中包含 "AI" 和 "机器学习"，而它们都是 board #100 "AI与机器学习" 的构成标签，交集数 ≥ direct_hit_min_overlap（默认 2）
- **THEN** tag SHALL 在 topic_tag_board_labels 中挂载到 board #100，match_reason="direct_hit"，score=1.0

#### Scenario: 交集数不足退回相似度匹配
- **WHEN** tag 的辅助标签中仅 "AI" 与 board #100 构成标签交集（交集=1），direct_hit_min_overlap=2
- **THEN** tag SHALL NOT 以 direct_hit 匹配，退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）

#### Scenario: direct_hit_min_overlap=1 向后兼容
- **WHEN** direct_hit_min_overlap 设为 1
- **THEN** 行为 SHALL 与变更前一致（1 个交集即 direct_hit）

### Requirement: 间接匹配三规则
系统 SHALL 对无法直接命中的 tag，计算每个 SemanticBoard 的命中率和 max_sim，按以下规则判断挂载：
1. 调整命中率 > direct_hit_rate（默认 0.5）→ 直接挂载，score = hit_rate_sim_blend × maxSimilarity + (1 - hit_rate_sim_blend) × adjustedHitRate
2. max_sim ≥ direct_max_sim（默认 0.8）**且 hits ≥ direct_max_sim_min_hits（默认 2，但不超过 tag 辅助标签总数 N，即 min(2, N)）且调整命中率 ≥ direct_max_sim_min_hit_rate（默认 0.3）**→ 直接挂载，score = maxSimilarity
3. 加权综合分 ≥ weighted_threshold → 挂载，score = 加权综合分

**调整命中率** = hits / max(tag 辅助标签总数, min_effective_sample)。当 tag 辅助标签数 ≥ min_effective_sample（默认 3）时，调整命中率 = 原始命中率。当 tag 辅助标签数 < min_effective_sample 时，分母补到 min_effective_sample，避免样本不足导致命中率虚高。

max_sim 为所有 tag-board 辅助标签对中的最高余弦相似度。hits 为 cosine_sim ≥ sim_threshold 的辅助标签数量。加权综合分 = weight_sim × max_sim + weight_density × 调整命中率。

hit_rate 规则的 score 为 maxSimilarity 和 adjustedHitRate 的加权混合（hit_rate_sim_blend 默认 0.7），确保 score 反映实际匹配质量而非仅密度比例。

#### Scenario: 命中率超阈值直接挂载（multi-aux）
- **WHEN** tag 有 4 个辅助标签（≥ min_effective_sample=3），其中 3 个与 board "地缘政治" 的 sim ≥ sim_threshold，调整命中率 = 3/4=75% > 50%
- **THEN** tag SHALL 挂载到 board "地缘政治"，score = 0.7×maxSim + 0.3×0.75（混合打分）

#### Scenario: max_sim 超阈值且双因子满足
- **WHEN** tag 有 5 个辅助标签，与 board "AI与机器学习" 的 max_sim=0.85 ≥ 0.8，且其中 2 个辅助标签 sim ≥ sim_threshold（hits=2 ≥ min(2,5)=2），hit_rate=2/5=0.4 ≥ 0.3
- **THEN** tag SHALL 挂载到 board "AI与机器学习"，match_reason="max_sim"

#### Scenario: max_sim 超阈值但 hits 不足
- **WHEN** tag 有 5 个辅助标签，与 board "科技行业ETF" 的 max_sim=0.85 ≥ 0.8，但只有 1 个辅助标签 sim ≥ sim_threshold（hits=1 < min(2,5)=2）
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载到 board "科技行业ETF"（但可能通过加权综合分规则挂载）

#### Scenario: max_sim 超阈值但 hit_rate 不足
- **WHEN** tag 有 10 个辅助标签，与 board 的 max_sim=0.82 ≥ 0.8，hits=2 ≥ min(2,10)=2，但 hit_rate=2/10=0.2 < 0.3
- **THEN** tag SHALL NOT 通过 max_sim 规则挂载（但可能通过加权综合分规则挂载）

#### Scenario: N=1 时 hit_rate 规则不再适用（被样本量惩罚推出）
- **WHEN** tag 只有 1 个辅助标签（keyword 直入），1 个 hit，调整命中率 = 1/max(1,3) = 0.333 < 0.5
- **THEN** tag SHALL NOT 通过 hit_rate 规则挂载；若 max_sim=0.85 ≥ 0.8 且 hits=1 ≥ min(2,1)=1 且调整命中率 0.333 ≥ 0.3，则通过 max_sim 规则挂载，score=0.85

#### Scenario: N=2 时 hit_rate 规则
- **WHEN** tag 有 2 个辅助标签，2 个都 hit，调整命中率 = 2/max(2,3) = 0.667 > 0.5
- **THEN** tag SHALL 通过 hit_rate 规则挂载，score = 0.7×maxSim + 0.3×0.667（混合打分，非 1.0）

#### Scenario: 加权综合分挂载
- **WHEN** tag 的辅助标签与 board "中东" 的 max_sim=0.72, 调整命中率=0.4，加权分 = 0.6×0.72 + 0.4×0.4 = 0.592
- **THEN** 如果 0.592 ≥ weighted_threshold，tag SHALL 挂载到 board "中东"；否则不挂载

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
系统 SHALL 允许一个 tag 同时属于多个 SemanticBoard。所有满足匹配规则的 board SHALL 按匹配分从高到低排序，默认最多保留 3 个。系统 SHALL 允许同一 event tag 及其文章在多个 NarrativeBoard 中重复出现。

#### Scenario: 多视角挂载
- **WHEN** tag "霍尔木兹海峡" 同时满足 board "地缘政治"（命中率 75%）和 board "能源安全"（max_sim 0.82）的挂载条件
- **THEN** tag SHALL 同时挂载到两个 board

#### Scenario: 超过归属上限时截断
- **WHEN** tag "AI芯片出口管制" 匹配到 5 个 SemanticBoard，semantic_board_match_max_boards=3
- **THEN** 系统 SHALL 仅保留匹配分最高的 3 个 topic_tag_board_labels 记录

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
