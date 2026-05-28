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
系统 SHALL 在 LLM 判断前，对候选辅助标签（含已有 board）进行 embedding 预聚类，压缩为 8-10 个簇。聚类 SHALL 使用 cosine 距离，阈值约 0.7。

#### Scenario: 65 个候选聚类
- **WHEN** 有 50 个新候选辅助标签 + 15 个已有 board
- **THEN** 系统 SHALL 通过 embedding 预聚类压缩为 8-10 个簇

### Requirement: 簇内补充 co-tag 事件上下文
系统 SHALL 为每个簇补充关联的 co-tag 事件作为 LLM 判断上下文。事件 SHALL 按以下规则筛选：(1) 时间窗口近 30 天；(2) 按共现频率排序取 top 20；(3) 事件间 embedding 相似度 >0.85 的去重；(4) 每簇硬上限 15 个事件。

#### Scenario: 簇补充事件
- **WHEN** 簇 A 包含辅助标签 [AI, 大语言模型, GPT]
- **THEN** 系统 SHALL 查找这些辅助标签关联 tag 的 co-tag 事件，筛选后附加到簇 A 的 LLM prompt 中

### Requirement: LLM 判断升级/合并/跳过
系统 SHALL 在用户手动触发时，将每个簇的辅助标签列表 + co-tag 事件发送给 LLM，由 LLM 判断：merge_into_existing（候选归入已有 board）、create_new（升级为新 board）、skip（暂不升级）。LLM 结果 SHALL 作为建议返回，用户确认前不得写入 SemanticBoard 或 board_composition。

#### Scenario: LLM 判断创建新 board
- **WHEN** 簇 [新能源, 光伏, 储能] 无已有 board，LLM 判断应升级
- **THEN** 系统 SHALL 返回 create_new 建议，包含 board 名称、描述和候选辅助标签，等待用户确认

#### Scenario: LLM 判断归入已有 board
- **WHEN** 簇 [AI, transformer, 深度学习] 中 "AI" 已是 board #42 的构成标签
- **THEN** 系统 SHALL 返回 merge_into_existing 建议，target_board_id=#42，等待用户确认

#### Scenario: LLM 判断跳过
- **WHEN** 簇内辅助标签过于分散，LLM 判断不足以形成板块
- **THEN** 系统 SHALL 跳过该簇，不创建 board

### Requirement: 用户确认后执行升级建议
系统 SHALL 仅在用户确认升级建议后创建或更新 SemanticBoard，并写入 board_composition。确认执行后，系统 SHALL 允许用户触发匹配回填。

#### Scenario: 确认创建新 SemanticBoard
- **WHEN** 用户确认 create_new 建议 "新能源与储能"
- **THEN** 系统 SHALL 创建 semantic_labels(label_type="board", source="llm_suggest") 并写入对应 board_composition

#### Scenario: 确认合并到已有 SemanticBoard
- **WHEN** 用户确认 merge_into_existing 到 board #42
- **THEN** 系统 SHALL 将建议中的辅助标签加入 board #42 的 board_composition

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
