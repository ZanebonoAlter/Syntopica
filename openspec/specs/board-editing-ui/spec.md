## Purpose

前端板块编辑功能和方向不符标签的展示控制。

## Requirements

### Requirement: Board editing dialog

雷达列表页（L0 `/tags`）每个版块行 SHALL 提供编辑入口：行尾「⋯」操作菜单中的「编辑版块」项，点击弹出编辑对话框。对话框 SHALL 支持修改 label 与 description（并 SHALL 支持 display_order 与 protected），调用已有 `updateBoard` API。保存成功后 SHALL 刷新板块信息。

该入口 SHALL 是版块本体编辑在**版块列表页**的可达路径——L0 雷达为版块列表所在处，用户 SHALL NOT 需要先进入版块态势页并下钻到治理工作面的第 N 个页签才能改名。

行尾「⋯」按钮 SHALL NOT 触发行点击进入版块态势页（须阻止事件冒泡）。

保存成功后，系统 SHALL 使版块 embedding 的方向校验使用新向量（缓存失效由后端保证，见 board-match-cache 能力）；对话框 SHALL NOT 声称已重跑版块匹配（匹配表重跑保持手动触发）。

#### Scenario: 雷达行提供编辑入口

- **GIVEN** 用户在 L0 雷达页看到版块「AI 前沿」一行
- **WHEN** 用户点该行行尾「⋯」并选「编辑版块」
- **THEN** 系统 SHALL 打开版块编辑对话框，且 SHALL NOT 触发整行跳转进入版块态势页

#### Scenario: edit board label

- **WHEN** user 在雷达行编辑对话框修改 label 并保存
- **THEN** updateBoard API called with new label, board list refreshed, board embedding regenerated

#### Scenario: edit board description

- **WHEN** user 在雷达行编辑对话框修改 description 并保存
- **THEN** updateBoard API called with new description, board embedding regenerated

#### Scenario: 行尾操作不误触发跳转

- **WHEN** 用户点击行尾「⋯」按钮
- **THEN** 系统 SHALL NOT 打开该版块的态势页，SHALL NOT 触发 openBoard 副作用

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
