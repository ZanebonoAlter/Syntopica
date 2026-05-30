## ADDED Requirements

### Requirement: Concept 按 category 隔离
系统 SHALL 将 board_concept 按 category (event/keyword/person) 隔离。每个 category 有独立的 concept 集，concept 的 ScopeCategoryID 字段 SHALL 存储对应的 category 字符串值。

#### Scenario: Event 标签只匹配 event concept
- **WHEN** 一个 event 标签执行 MatchTagToConcept
- **THEN** 系统只在 category='event' 且 status='active' 的 concept 中搜索

#### Scenario: Bootstrap 按 category 独立聚类
- **WHEN** 手动触发 bootstrap 且指定 category='event'
- **THEN** 系统只使用 event 标签的 semantic embedding 做聚类

### Requirement: Concept 状态机
board_concepts SHALL 使用 status 字段（pending/active/inactive/merged）替代 is_active 字段。状态流转：pending → active（用户确认）→ inactive（停用）；active → merged（合并到其他 concept）；pending → inactive（忽略）。

#### Scenario: Bootstrap 生成 pending concept
- **WHEN** bootstrap 聚类完成并 LLM 命名后
- **THEN** 创建的 concept status='pending'，不出现在 MatchTagToConcept 的候选中

#### Scenario: 用户确认 pending concept
- **WHEN** 用户调用 POST /api/hierarchy/concepts/:id/confirm
- **THEN** concept 的 status 从 'pending' 变为 'active'，MatchTagToConcept 开始考虑该 concept

#### Scenario: 用户停用 active concept
- **WHEN** 用户调用 DELETE /api/hierarchy/concepts/:id
- **THEN** concept 的 status 变为 'inactive'，不再参与匹配和放置

### Requirement: Abstract 创建限定在 concept 围栏内
当 status='active' 的 concept 存在时，新创建的 abstract 标签 SHALL 关联到一个 concept（设置 concept_id）。abstract 的候选搜索和去重 SHALL 限定在同一 concept 内。

#### Scenario: 放置时 concept 约束过滤
- **WHEN** placeTagAtLevel 在 concept C 内放置标签 T
- **THEN** FindSimilarAbstractTags 返回的候选 SHALL 过滤为 concept_id=C.ID 的 abstract

#### Scenario: Abstract 创建时关联 concept
- **WHEN** placeTagAtLevel 在 concept C 内创建新 abstract A
- **THEN** A.concept_id SHALL 设为 C.ID

#### Scenario: 跨 concept 不创建 abstract
- **WHEN** concept A 内的 abstract 和 concept B 内的 abstract 语义相似
- **THEN** 系统 SHALL NOT 合并或关联它们，dedup 只在同 concept 内执行

### Requirement: Concept bootstrap 聚类
系统 SHALL 提供 embedding 聚类 + LLM 命名的 concept bootstrap 流程，仅通过手动触发执行。

#### Scenario: 手动触发 bootstrap
- **WHEN** POST /api/hierarchy/concepts/bootstrap 且 category='event'
- **THEN** 系统加载该 category 所有活跃标签的 semantic embedding，执行聚类，对每个 cluster 调用 LLM 命名，生成 status='pending' 的 concept

#### Scenario: 标签不足时不生成
- **WHEN** 指定 category 的活跃标签少于 10 个
- **THEN** bootstrap 返回空列表，不生成任何 concept

#### Scenario: 聚类使用 pgvector 原生能力
- **WHEN** bootstrap 执行聚类
- **THEN** 使用 pgvector `<=>` 距离操作符找近邻，构建连通图，连通分量即为 cluster

### Requirement: Concept bootstrap LLM 命名
每个聚类 cluster SHALL 交给 LLM 生成 concept 的 name（2-6 字）和 description（30-80 字）。

#### Scenario: LLM 接收 cluster 标签列表
- **WHEN** 一个 cluster 包含标签 ["GPT-5.5 发布", "DeepSeek V4 发布", "Claude 4 发布"]
- **THEN** LLM 接收这些标签列表，输出 {"name": "AI模型发布", "description": "涵盖主要 AI 模型的发布、更新与生态事件"}

#### Scenario: 多个 cluster 独立命名
- **WHEN** 聚类产生 4 个 cluster
- **THEN** 每个 cluster 独立调用 LLM 命名，生成 4 个 pending concept

### Requirement: 空心 abstract 回收
系统 SHALL 回收满足以下条件的 active abstract：子标签数 ≤ 1 且 文章引用数 = 0 且 存在时间 > 24h。

#### Scenario: 空心 abstract 被回收
- **WHEN** abstract A 有 1 个子标签、0 篇文章引用、创建于 25 小时前
- **THEN** A 被标记为 inactive，子标签断开关系并重新进入放置队列

#### Scenario: 有文章引用的不被回收
- **WHEN** abstract A 有 1 个子标签、3 篇文章引用、创建于 25 小时前
- **THEN** A 不被回收
