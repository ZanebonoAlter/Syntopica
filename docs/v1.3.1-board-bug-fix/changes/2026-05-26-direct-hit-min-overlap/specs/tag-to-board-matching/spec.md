## MODIFIED Requirements

### Requirement: 直接命中 board 构成标签

**原需求**: tag 的辅助标签与 board 的辅助标签有 1 个 ID 交集即视为直接命中。

**变更为**: tag 的辅助标签与 board 的辅助标签的交集数 ≥ `direct_hit_min_overlap`（默认 2）时才视为直接命中。交集数不足时，该 tag-board 对退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）。

#### Scenario: 交集数满足阈值 → direct_hit
- **WHEN** tag 的辅助标签为 {特朗普, 美联储, 白宫}，board 的辅助标签包含 {特朗普, 美联储, 标普500}，交集 = {特朗普, 美联储} = 2，`direct_hit_min_overlap` = 2
- **THEN** tag SHALL 以 match_reason="direct_hit", score=1.0 匹配该 board

#### Scenario: 交集数不足阈值 → 退回相似度匹配
- **WHEN** tag 的辅助标签为 {特朗普, 日菲安全合作, 马科斯}，board 的辅助标签包含 {特朗普, 美联储, 白宫}，交集 = {特朗普} = 1，`direct_hit_min_overlap` = 2
- **THEN** tag SHALL NOT 以 direct_hit 匹配该 board，而是退回到相似度匹配流程（hit_rate / max_sim / weighted 规则）

#### Scenario: 阈值设为 1 → 向后兼容原行为
- **WHEN** `direct_hit_min_overlap` = 1
- **THEN** 行为 SHALL 与变更前完全一致（1 个交集即 direct_hit）

### Requirement: 匹配参数用户可调

**原需求**: 已有 sim_threshold, direct_hit_rate, direct_max_sim 等参数。

**变更为**: 新增 `semantic_board_match_direct_hit_min_overlap`（默认 2），控制 direct_hit 所需的最小交集数。

#### Scenario: 用户调整 direct_hit 最小交集数
- **WHEN** 用户将 `semantic_board_match_direct_hit_min_overlap` 从 2 调整为 3
- **THEN** 后续匹配中，tag 与 board 的辅助标签交集需 ≥ 3 才能触发 direct_hit
