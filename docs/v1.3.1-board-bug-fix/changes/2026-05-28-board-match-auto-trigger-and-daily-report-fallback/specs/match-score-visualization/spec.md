## MODIFIED Requirements

### Requirement: 匹配分数可视化
前端 SHALL 在 tag chip 上展示 match_reason 色标和 score 数值。对于降级匹配（`downgraded=true`），前端 SHALL 降低视觉权重：使用更淡的边框色、更小的字号、以及 "↓" 后缀标记。

#### Scenario: 正常匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85，downgraded=false
- **THEN** tag chip SHALL 使用对应规则色（#f59e0b）的完整亮度边框，显示 "相似度 0.85"

#### Scenario: 降级匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85，downgraded=true
- **THEN** tag chip SHALL 使用对应规则色但降低 50% 不透明度的边框，显示 "相似度 0.85↓"

#### Scenario: direct_hit 和其他规则不受降级影响
- **WHEN** tag chip 的 match_reason="direct_hit" 或 match_reason="hit_rate" 或 match_reason="weighted"
- **THEN** tag chip SHALL 正常显示，不受降级样式影响

### Requirement: 匹配详情面板降级说明
`MatchDetailPanel.vue` 的匹配流程步骤 SHALL 在降级匹配时明确标注降级信息，包括原始阈值和实际使用的阈值。

#### Scenario: 降级匹配流程步骤
- **WHEN** tag 以 max_sim 匹配，downgraded=true，有 1 个辅助标签，direct_max_sim_min_hits=2
- **THEN** 匹配流程步骤 ④ SHALL 显示 "✓ 1≥1 命中 ⚠ 降级匹配（原阈值 2，因仅有 1 个辅助标签降为 1）"

#### Scenario: 正常匹配流程步骤不变
- **WHEN** tag 以 max_sim 匹配，downgraded=false
- **THEN** 匹配流程步骤 SHALL 按原有方式显示，无降级提示
