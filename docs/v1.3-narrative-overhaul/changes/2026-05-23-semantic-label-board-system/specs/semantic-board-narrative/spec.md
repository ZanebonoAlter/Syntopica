## ADDED Requirements

### Requirement: SemanticBoard 派生每日 NarrativeBoard
系统 SHALL 从全局共享的 SemanticBoard 派生每日 NarrativeBoard。生成时，系统 SHALL 在指定日期和 scope 范围内收集归属于该 SemanticBoard 的 active event tags，并为每个有事件的 SemanticBoard 创建对应 NarrativeBoard。

#### Scenario: 分类范围生成每日板
- **WHEN** feed category #5 在 2026-05-21 有 4 个 event tags 归属于 SemanticBoard "AI与机器学习"
- **THEN** 系统 SHALL 创建一个 scope_type="feed_category"、scope_category_id=5、semantic_board_id 指向该 SemanticBoard 的 NarrativeBoard

#### Scenario: 全局范围生成每日板
- **WHEN** 全局范围在 2026-05-21 有 event tags 归属于 SemanticBoard "能源安全"
- **THEN** 系统 SHALL 创建一个 scope_type="global" 的 NarrativeBoard

### Requirement: 取消 abstract tree 热点板路径
系统 SHALL NOT 通过 abstract tag tree、topic_tag_relations 或 abstract_tag_id 创建热点 NarrativeBoard。所有每日 NarrativeBoard SHALL 由 SemanticBoard 派生。

#### Scenario: 无 abstract tree 输入
- **WHEN** NarrativeBoard 生成运行
- **THEN** 系统 SHALL 不读取 topic_tag_relations 来构建热点板

### Requirement: 冷启动无 SemanticBoard 时不生成 NarrativeBoard
系统 SHALL 允许冷启动阶段没有 SemanticBoard。没有 active SemanticBoard 或没有匹配 event tags 时，系统 SHALL 不生成对应 NarrativeBoard，且不报错。

#### Scenario: 没有 SemanticBoard
- **WHEN** 系统没有 label_type="board" 且 narrative 生成运行
- **THEN** 系统 SHALL 返回 0 个新 NarrativeBoard，并保持 tag/auxiliary label 积累流程正常

### Requirement: 多板块归属允许事件重复展示
如果一个 event tag 通过 topic_tag_board_labels 归属于多个 SemanticBoard，系统 SHALL 允许该 event tag 及其文章出现在多个 NarrativeBoard 中。

#### Scenario: 同一事件进入多个每日板
- **WHEN** event tag "霍尔木兹海峡" 同时归属于 SemanticBoard "地缘政治" 和 "能源安全"
- **THEN** 当日 narrative 生成 SHALL 允许该 event tag 出现在两个 NarrativeBoard 的 event_tag_ids 中

### Requirement: NarrativeBoard 通过 semantic_board_id 续接
系统 SHALL 在 `narrative_boards` 中使用 `semantic_board_id` 关联 SemanticBoard，并按 semantic_board_id + scope + 前一日日期匹配 prev_board_ids。

#### Scenario: 同一 SemanticBoard 连续两日续接
- **WHEN** 2026-05-20 和 2026-05-21 都生成了 semantic_board_id=42、scope_type="global" 的 NarrativeBoard
- **THEN** 2026-05-21 的 NarrativeBoard.prev_board_ids SHALL 包含 2026-05-20 的对应 board id

### Requirement: Board 叙事上下文来自 SemanticBoard
系统 SHALL 使用 SemanticBoard 的 label 和 description 作为 NarrativeBoard 叙事生成的 board context，不再使用 abstract tag 或 board_concepts 作为上下文。

#### Scenario: 生成 board narrative prompt
- **WHEN** 为 SemanticBoard "AI与机器学习" 派生的 NarrativeBoard 生成摘要
- **THEN** LLM prompt SHALL 包含 SemanticBoard label、description 和当日 event tags
