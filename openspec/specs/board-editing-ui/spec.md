## Purpose

前端板块编辑功能和方向不符标签的展示控制。

## Requirements

### Requirement: Board editing dialog

TagsPage 板块列表中每个板块新增编辑按钮，点击弹出编辑对话框。对话框支持修改 label 和 description，调用已有 `updateBoard` API。保存成功后刷新板块信息。

#### Scenario: edit board label
- **WHEN** user clicks edit button on a board, modifies label, and saves
- **THEN** updateBoard API called with new label, board list refreshed, board embedding regenerated

#### Scenario: edit board description
- **WHEN** user modifies description and saves
- **THEN** updateBoard API called with new description, board embedding regenerated

### Requirement: Default hide direction_mismatch tags

TagsPage 的 tag chip 列表默认不显示 direction_mismatch=true 的标签。提供"显示方向不符"开关（checkbox 或 toggle），开启后显示这些标签，用虚线边框 + "⊘" 后缀标记。

#### Scenario: default view
- **WHEN** article has 3 filtered_tags, 1 with direction_mismatch=true
- **THEN** only 2 tags shown in chip list

#### Scenario: show direction mismatch toggle
- **WHEN** user enables "显示方向不符" toggle
- **THEN** all 3 tags shown, direction_mismatch tag has dashed border and "⊘" suffix

### Requirement: MatchDetailPanel direction check display

MatchDetailPanel 步骤 ④（max_sim）展示方向校验结果：direction_sim 值和是否通过。direction_mismatch=true 时显示警告。

#### Scenario: direction check passed
- **WHEN** max_sim matched AND direction_sim >= threshold
- **THEN** step ④ shows "方向校验 ✓ sim=0.72≥0.5"

#### Scenario: direction check failed
- **WHEN** max_sim matched AND direction_sim < threshold
- **THEN** step ④ shows "⚠ 方向不符 sim=0.35<0.5"

### Requirement: API type updates

BoardArticleTag 接口新增 `direction_mismatch: boolean`。MatchDetailResponse 新增 `direction_sim: number | null`。

#### Scenario: type definitions updated
- **WHEN** frontend types are updated
- **THEN** BoardArticleTag has direction_mismatch field, MatchDetailResponse has direction_sim field
