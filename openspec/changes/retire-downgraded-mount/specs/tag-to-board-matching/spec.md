# tag-to-board-matching — retire-downgraded-mount delta

## MODIFIED Requirements

### Requirement: 间接匹配三规则
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

### Requirement: Tag 可属于多个 Board
系统 SHALL 允许一个 tag 同时属于多个 SemanticBoard。所有满足匹配规则的 board SHALL 按匹配分从高到低排序，默认最多保留 3 个。同一 event tag 及其文章 SHALL 允许出现在多个版块的日报产物中。

#### Scenario: 多视角挂载含降级标记
- **WHEN** tag "霍尔木兹海峡" 同时满足 board "地缘政治"（命中率 75%）和 board "能源安全"（max_sim 0.82，N=1）
- **THEN** tag SHALL 同时挂载到两个 board；两行记录除 score 与 match_reason 外无任何字段差异（**场景名沿用历史命名；降级标记已退役**）

#### Scenario: 超过归属上限时截断
- **WHEN** tag "AI芯片出口管制" 匹配到 5 个 SemanticBoard，semantic_board_match_max_boards=3
- **THEN** 系统 SHALL 仅保留匹配分最高的 3 个 topic_tag_board_labels 记录
