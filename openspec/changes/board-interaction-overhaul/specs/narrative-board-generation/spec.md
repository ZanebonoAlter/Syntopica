## MODIFIED Requirements

### Requirement: SemanticBoard 派生每日 NarrativeBoard
系统 SHALL 从全局共享的 SemanticBoard 派生每日 NarrativeBoard。生成时，系统 SHALL 收集归属于该 SemanticBoard 的所有 active event tags（不区分 feed category），并为每个有事件的 SemanticBoard 创建一份 NarrativeBoard（scope_type="board"）。

#### Scenario: 单 board 单日单叙事板
- **WHEN** SemanticBoard "AI与机器学习" 在 2026-05-21 有来自科技和财经两个 category 的 8 个 event tags
- **THEN** 系统 SHALL 创建一个 scope_type="board"、semantic_board_id 指向该 SemanticBoard 的 NarrativeBoard，event_tag_ids 包含全部 8 个 event tags
- **NOTE** `CollectSemanticBoardNarrativeInputs` 需移除 scopeType/categoryID 参数，不再按 feed category 过滤 event tags；`matchPreviousSemanticBoard` 仅按 semantic_board_id + 前一日日期匹配续接，不再按 scope + category 过滤

#### Scenario: 多个 board 各自生成叙事板
- **WHEN** SemanticBoard "AI与机器学习" 有 8 个 event tags，SemanticBoard "能源安全" 有 3 个 event tags
- **THEN** 系统 SHALL 为每个 SemanticBoard 各创建一份 NarrativeBoard

### Requirement: NarrativeBoard 通过 semantic_board_id 续接
系统 SHALL 在 `narrative_boards` 中使用 `semantic_board_id` 关联 SemanticBoard，并按 semantic_board_id + 前一日日期匹配 prev_board_ids，不再按 scope + category 区分续接。

#### Scenario: 同一 SemanticBoard 连续两日续接
- **WHEN** 2026-05-20 和 2026-05-21 都生成了 semantic_board_id=42 的 NarrativeBoard
- **THEN** 2026-05-21 的 NarrativeBoard.prev_board_ids SHALL 包含 2026-05-20 的对应 board id

## REMOVED Requirements

### Requirement: 分类范围生成每日板
**Reason**: 取消 scope 分类，每个 SemanticBoard 每天只生成一份叙事板，不再按 feed category 区分
**Migration**: 旧数据中 scope_type="feed_category" 的 NarrativeBoard 保留但不影响新逻辑

### Requirement: 全局范围生成每日板
**Reason**: 取消 scope 分类，统一为 board 维度叙事
**Migration**: 旧数据中 scope_type="global" 的 NarrativeBoard 保留但不影响新逻辑
