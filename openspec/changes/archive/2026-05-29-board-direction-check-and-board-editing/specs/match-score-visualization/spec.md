## MODIFIED Requirements

### Requirement: Default hide direction_mismatch tags in chip list

TagsPage 的 tag chip 列表默认不显示 direction_mismatch=true 的标签。提供"显示方向不符"toggle 开关。开启后显示这些标签，用虚线边框 + "⊘" 后缀标记。

#### Scenario: default hide
- **WHEN** article has filtered_tags with some direction_mismatch=true
- **THEN** only non-mismatch tags shown in chip list

#### Scenario: toggle on
- **WHEN** user enables show_direction_mismatch toggle
- **THEN** all tags shown, direction_mismatch tags have dashed border + "⊘" suffix

### Requirement: MatchDetailPanel direction check display

MatchDetailPanel 步骤 ④ 展示方向校验结果和 direction_sim 值。

#### Scenario: direction check passed
- **THEN** step ④ shows "方向校验 ✓ sim=X≥Y"

#### Scenario: direction check failed
- **THEN** step ④ shows "⚠ 方向不符 sim=X<Y"
