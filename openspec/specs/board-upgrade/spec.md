## Purpose

辅助标签聚类升级为 SemanticBoard 的流程，包括候选收集、预聚类、LLM 建议、用户确认和冷启动。

## Requirements

### Requirement: 辅助标签聚类升级触发
系统 SHALL 在 ref_count ≥ semantic_board_upgrade_ref_count_threshold（默认 5）的未升级辅助标签数量达到阈值时，允许用户手动触发板块升级流程。系统 SHALL NOT 自动触发升级。

#### Scenario: 达到触发阈值
- **WHEN** 系统检测到 8 个 ref_count ≥ 5 的未升级辅助标签
- **THEN** 系统 SHALL 在升级候选列表中展示这些辅助标签，等待用户手动触发 LLM 建议

#### Scenario: 未达阈值
- **WHEN** 仅有 3 个 ref_count ≥ 5 的未升级辅助标签
- **THEN** 系统 SHALL 不触发升级

### Requirement: 预聚类压缩候选
系统 SHALL 在 LLM 判断前，对候选辅助标签进行纯自聚类（不参考已有板块），使用 cosine 距离和配置的 cluster_distance_threshold。所有候选 SHALL 统一参与自聚类，不受已有板块影响。聚类完成后，系统 SHALL 为每个簇计算 board_affinities（与已有板块的亲和度元数据）。

#### Scenario: 65 个候选纯自聚类
- **WHEN** 有 50 个新候选辅助标签 + 15 个已有 board（但 board 不再参与聚类）
- **THEN** 系统 SHALL 对 50 个新候选进行纯 embedding 自聚类，聚类结果不受已有 board 影响

#### Scenario: 候选与已有板块相似但不影响聚类
- **WHEN** 候选 "AI" 与已有板块 "人工智能" 的辅助标签 embedding 距离 ≤ threshold
- **THEN** 系统 SHALL NOT 将 "AI" 单独归入已有板块的簇，而是正常参与自聚类；系统 SHALL 在该簇的 board_affinities 中记录与 "人工智能" 板块的亲和关系

### Requirement: 簇内补充 co-tag 事件上下文
系统 SHALL 为每个簇补充关联的 co-tag 事件作为 LLM 判断上下文。事件 SHALL 按以下规则筛选：(1) 时间窗口近 30 天；(2) 按共现频率排序取 top 20；(3) 事件间 embedding 相似度 >0.85 的去重；(4) 每簇硬上限 15 个事件。

#### Scenario: 簇补充事件
- **WHEN** 簇 A 包含辅助标签 [AI, 大语言模型, GPT]
- **THEN** 系统 SHALL 查找这些辅助标签关联 tag 的 co-tag 事件，筛选后附加到簇 A 的 LLM prompt 中

### Requirement: LLM 判断升级/跳过
系统 SHALL 在用户手动触发时，将每个簇的辅助标签列表 + co-tag 事件发送给 LLM，由 LLM 判断：create_new（升级为新 board）或 skip（暂不升级）。LLM SHALL NOT 产出 merge_into_existing 决策。

#### Scenario: LLM 判断创建新 board
- **WHEN** 簇 [新能源, 光伏, 储能] LLM 判断应升级
- **THEN** 系统 SHALL 返回 create_new 建议，包含 board 名称、描述和候选辅助标签，等待用户确认

#### Scenario: LLM 判断跳过
- **WHEN** 簇内辅助标签过于分散，LLM 判断不足以形成板块
- **THEN** 系统 SHALL 返回 skip 建议，跳过该簇，不创建 board

#### Scenario: LLM 不再产出 merge_into_existing
- **WHEN** 簇 [AI, transformer, 深度学习] 中部分标签与已有板块 "人工智能" 相似
- **THEN** 系统 SHALL NOT 让 LLM 产出 merge_into_existing 建议；board_affinity 元数据 SHALL 供前端展示，由用户决定是否手动合并

### Requirement: 用户确认后执行升级建议
系统 SHALL 仅在用户确认升级建议后创建或更新 SemanticBoard，并写入 board_composition。确认执行后，系统 SHALL 允许用户触发匹配回填。

#### Scenario: 确认创建新 SemanticBoard
- **WHEN** 用户确认 create_new 建议 "新能源与储能"
- **THEN** 系统 SHALL 调用 semanticBoardLabelEmbedder 生成 embedding（输入 `label + ". " + description`，description 为空时仅用 label），再创建 semantic_labels（label_type="board", source="llm_suggest"），embedding 一并写入

#### Scenario: 创建新板块时 embedder 失败
- **WHEN** embedder returns error during create_new confirmation
- **THEN** confirmation fails with error, board NOT created

#### Scenario: 确认合并到已有 SemanticBoard
- **WHEN** 用户确认 merge_into_existing 到 board #42
- **THEN** 系统 SHALL 将建议中的辅助标签加入 board #42 的 board_composition

### Requirement: 前端展示 Board Affinity 参考信息
系统 SHALL 在板块升级建议面板中，为每个建议卡片展示该建议内嵌的 board_affinities 信息（相似已有板块名称、匹配候选数、平均距离），作为用户决策参考。board_affinities 由后端在 suggestionsToDTO 中根据 suggestion 的 auxiliary_label_ids 从对应 cluster 汇总而来。

#### Scenario: 建议卡片展示相似板块
- **WHEN** 用户查看一个 create_new 建议（簇包含 [AI, 大语言模型]），且该建议有 board_affinity：{board_label: "人工智能", matching_candidates: 2, avg_distance: 0.28}
- **THEN** 系统 SHALL 在建议卡片中展示 "相似板块: 人工智能 (2 candidates, avg distance 0.28)" 信息

### Requirement: 前端人工合并下拉操作
系统 SHALL 在每个非 skip 的建议卡片上提供"合并到..."下拉按钮，展示该簇的 board_affinities 列表（按 avg_distance 升序），用户选择后 SHALL 以 merge_into_existing 决策调用 ConfirmSuggestion API。

#### Scenario: 用户手动合并到已有板块
- **WHEN** 用户在 create_new 建议卡片上点击"合并到..."，选择 "人工智能 (board_id=42)"
- **THEN** 前端 SHALL 以 `{decision: "merge_into_existing", target_board_id: 42, auxiliary_label_ids: [...]}` 调用 ConfirmSuggestion API

#### Scenario: 无相似板块时不展示合并选项
- **WHEN** 建议卡片的簇 board_affinities 为空
- **THEN** 系统 SHALL 隐藏或禁用"合并到..."下拉按钮

### Requirement: ConfirmSuggestion API 合并路径保持不变
系统 SHALL 保持 ConfirmSuggestion API 的 merge_into_existing 路径不变，接受前端传入的 target_board_id 执行合并操作。

#### Scenario: 前端触发合并确认
- **WHEN** 前端发送 `{decision: "merge_into_existing", target_board_id: 42, auxiliary_label_ids: [101, 102]}`
- **THEN** 系统 SHALL 验证 target_board_id 为有效的活跃 board，将辅助标签加入 board #42 的 board_composition

### Requirement: 手动回填队列
系统 SHALL 提供手动触发的回填功能，将需要重新匹配 board 的 tag 放入异步队列逐个处理。回填 SHALL 不是自动触发的。

#### Scenario: 手动触发回填
- **WHEN** 用户点击"回填"按钮
- **THEN** 系统 SHALL 根据用户选择的 all / unassigned / board 模式入队，异步逐个执行 board 匹配

### Requirement: 冷启动允许无 SemanticBoard
系统 SHALL 允许冷启动阶段没有任何 SemanticBoard。无 SemanticBoard 时，tag SHALL 仍然提取和积累辅助标签；NarrativeBoard 生成 SHALL 跳过 semantic board 派生，直到用户确认创建第一批 SemanticBoard 并回填。

#### Scenario: 冷启动无 board
- **WHEN** 系统尚无 label_type="board" 的 semantic_labels
- **THEN** tag 提取 SHALL 正常写入辅助标签，board 匹配 SHALL 返回无归属且不报错

#### Scenario: 冷启动初始化建议
- **WHEN** 辅助标签池累计到升级阈值且用户手动触发升级建议
- **THEN** 系统 SHALL 基于当前辅助标签池生成第一批 SemanticBoard 建议
