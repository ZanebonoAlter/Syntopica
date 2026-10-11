## MODIFIED Requirements

### Requirement: Tag 通过辅助标签匹配 Board
系统 SHALL 按以下优先级判定 tag 与 SemanticBoard 的匹配：组合标签直接命中（composite direct_hit）为最强信号——tag 关联的组合标签与 board composition 挂载的组合标签存在交集时，score=1.0，match_reason="composite_hit"，MUST NOT 标记 direction_mismatch（组合命中即指向一致）。单辅助标签重叠 direct_hit 降级为弱信号：交集数 ≥ direct_hit_min_overlap（默认 2）**且交集数 / tag 的 active 辅助标签总数 ≥ direct_hit_min_ratio（默认 0.5）**时不再给 score=1.0，SHALL 给予 direct_hit_score_factor（默认 0.7，ai_settings 可配）的折扣分并执行方向校验（与间接匹配规则相同的 direction check，低于 direction_sim_threshold 则标记 direction_mismatch=true）。**交集数达标但占比不足时 SHALL NOT 以 direct_hit 挂载，退回相似度匹配流程（hit_rate / max_sim / weighted 规则）判定，且重叠标签 SHALL NOT 作为该版块的相似度证据参与退回判定（同一批标签不得既作直接命中证据、又作相似度证据——自相似恒为 1.0 会变相重新挂载）；剩余无重叠标签的相似度均不达标时该 tag 不挂载该版块**。所有满足条件的 SemanticBoard SHALL 按分数排序后持久化到 topic_tag_board_labels，默认最多挂载 3 个。间接匹配三规则（hit_rate / max_sim / weighted）行为不变。

#### Scenario: 组合标签直接命中
- **WHEN** tag「美债收益率创两周新高」关联组合标签「美债收益率」，版块「美债观察」的 composition 挂载同一组合标签
- **THEN** tag SHALL 挂载到该版块，match_reason="composite_hit"，score=1.0，direction_mismatch=false

#### Scenario: 直接命中 board 构成标签
- **WHEN** tag 的辅助标签中包含 "AI" 和 "机器学习"，而它们都是 board #100 "AI与机器学习" 的构成标签，交集数 ≥ direct_hit_min_overlap（默认 2）且占比 ≥ direct_hit_min_ratio（默认 0.5，2 个交集落在 tag 全部辅助标签中占比过半），且 tag identity embedding 与 board embedding cosine ≥ direction_sim_threshold
- **THEN** tag SHALL 在 topic_tag_board_labels 中挂载到 board #100，match_reason="direct_hit"，score=direct_hit_score_factor（默认 0.7），direction_mismatch=false

#### Scenario: 交集数达标但占比不足退回相似度
- **WHEN** tag 的 active 辅助标签共 5 个 {美联储, 港股, A股, 美元指数, 非农就业数据}，与 board「中国国内新闻」构成标签交集=2（港股、A股）≥ direct_hit_min_overlap=2，但占比 2/5=0.4 < direct_hit_min_ratio=0.5
- **THEN** tag SHALL NOT 以 direct_hit 挂载到该版块；退回相似度判定时港股/A股 SHALL NOT 计入相似度证据，剩余 3 个标签与该版块相似度均不达阈值时该 tag SHALL NOT 挂载该版块

#### Scenario: direct_hit_min_overlap=1 向后兼容
- **WHEN** direct_hit_min_overlap 设为 1，tag 的辅助标签与 board 构成标签交集=1，且该交集占比 ≥ direct_hit_min_ratio
- **THEN** 1 个交集即以 direct_hit 匹配（降级语义：score=direct_hit_score_factor + 强制方向校验），交集判定行为与变更前一致

#### Scenario: 降级 direct_hit 方向校验不通过
- **WHEN** tag 的辅助标签 {国债, 收益率}（占比 2/2=1.0）与 board composition 交集=2 ≥ direct_hit_min_overlap，且 tag identity embedding 与 board embedding cosine=0.55 < direction_sim_threshold
- **THEN** tag SHALL 以 match_reason="direct_hit"、score=direct_hit_score_factor（默认 0.7）挂载，direction_mismatch=true（前端默认隐藏）

#### Scenario: 组合命中与单标签重叠同时存在
- **WHEN** tag 同时满足组合命中（board A）与单标签重叠（board A、B），且组合命中的 board 也满足单标签重叠
- **THEN** board A SHALL 以 composite_hit（score=1.0）记录而非 direct_hit（每对 tag-board 仅记录最高优先级 match_reason）

#### Scenario: 无组合标签时行为
- **WHEN** 系统中不存在任何组合标签（或该 tag/board 均未关联组合标签）
- **THEN** 匹配 SHALL 走降级后的单标签 direct_hit 与间接三规则，与变更前相比仅 direct_hit 的占比门槛、分数与 direction_mismatch 行为不同

#### Scenario: 交集数不足退回相似度匹配
- **WHEN** tag 的辅助标签中仅 1 个与 board composition 交集，direct_hit_min_overlap=2，且无组合命中
- **THEN** tag SHALL NOT 以 direct_hit 匹配，退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）

#### Scenario: 占比拦下但剩余标签真相似仍可挂载
- **WHEN** tag 的 5 个辅助标签中 2 个与 board 构成交集（占比 0.4 被 direct_hit 拦下），剩余 3 个标签与该版块其它构成标签存在真实语义相似（命中率 > direct_hit_rate）
- **THEN** tag SHALL 以 hit_rate / max_sim / weighted 之一挂载该版块（拦的是证据换赛道，不是真跨界语义相似）

#### Scenario: 占比门槛不影响交集数不足的判定
- **WHEN** tag 的辅助标签交集数 < direct_hit_min_overlap（无论占比多少）
- **THEN** direct_hit 判定 SHALL NOT 通过，与占比门槛无关（两条件是 AND 关系）

### Requirement: 匹配参数用户可调
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
