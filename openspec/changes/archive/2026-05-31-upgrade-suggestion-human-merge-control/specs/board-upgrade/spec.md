## MODIFIED Requirements

### Requirement: 预聚类压缩候选
系统 SHALL 在 LLM 判断前，对候选辅助标签进行纯自聚类（不参考已有板块），使用 cosine 距离和配置的 cluster_distance_threshold。所有候选 SHALL 统一参与自聚类，不受已有板块影响。聚类完成后，系统 SHALL 为每个簇计算 board_affinities（与已有板块的亲和度元数据）。

#### Scenario: 65 个候选纯自聚类
- **WHEN** 有 50 个新候选辅助标签 + 15 个已有 board（但 board 不再参与聚类）
- **THEN** 系统 SHALL 对 50 个新候选进行纯 embedding 自聚类，聚类结果不受已有 board 影响

#### Scenario: 候选与已有板块相似但不影响聚类
- **WHEN** 候选 "AI" 与已有板块 "人工智能" 的辅助标签 embedding 距离 ≤ threshold
- **THEN** 系统 SHALL NOT 将 "AI" 单独归入已有板块的簇，而是正常参与自聚类；系统 SHALL 在该簇的 board_affinities 中记录与 "人工智能" 板块的亲和关系

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

## ADDED Requirements

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
