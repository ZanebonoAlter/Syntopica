## MODIFIED Requirements

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

## ADDED Requirements

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
