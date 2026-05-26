## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: 匹配参数新增配置
系统 SHALL 允许用户通过配置调整以下匹配参数：
- semantic_board_match_direct_max_sim_min_hits（默认 2，max_sim 规则要求的最小 hits 数）
- semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3，max_sim 规则要求的最小调整命中率）
- semantic_board_match_min_effective_sample（默认 3，命中率计算的分母下限，用于样本量惩罚）
- semantic_board_match_hit_rate_sim_blend（默认 0.7，hit_rate 规则中 maxSim 的权重，score = α×maxSim + (1-α)×adjustedHitRate）

#### Scenario: 用户修改 min_hits
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hits 从 2 调整为 3
- **THEN** 后续匹配中，max_sim 规则需要 hits ≥ min(3, N) 才能挂载

#### Scenario: 用户修改 min_hit_rate
- **WHEN** 用户将 semantic_board_match_direct_max_sim_min_hit_rate 从 0.3 调整为 0.4
- **THEN** 后续匹配中，max_sim 规则需要 hit_rate ≥ 0.4 才能挂载
