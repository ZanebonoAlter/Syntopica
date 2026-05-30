## ADDED Requirements

### Requirement: 通用 depth-based 放置
PlaceTagInHierarchy SHALL 使用 depth（BFS 向上跳数）参数化所有放置逻辑，不硬编码层级名。depth=0 表示无 abstract parent，depth 越大越靠近根。

#### Scenario: 3 层模板 event 标签放置
- **WHEN** 新 event 标签 embedding 就绪，depth=0，tmpl.MaxLevel=3
- **THEN** targetDepth=1，执行 placeTagAtLevel(tag, tmpl, 1, concept)

#### Scenario: 2 层模板 person 标签放置
- **WHEN** 新 person 标签 embedding 就绪，depth=0，tmpl.MaxLevel=2
- **THEN** targetDepth=1，执行 placeTagAtLevel(tag, tmpl, 1, concept)

#### Scenario: Embedding 未就绪时返回 pending
- **WHEN** 新标签的 semantic embedding 尚未生成
- **THEN** PlaceTagInHierarchy 返回 {Action: "pending_embedding"}，不执行放置

#### Scenario: 已放置标签不重复放置
- **WHEN** 标签 depth > 0 且 source != 'abstract'
- **THEN** PlaceTagInHierarchy 返回 {Action: "already_placed"}

### Requirement: Per-template depth 上界
全局常量 maxHierarchyDepth=4 SHALL 被替换为函数 getMaxDepthForCategory(category)，返回 tmpl.MaxLevel - 1。

#### Scenario: Event 模板 depth 上界
- **WHEN** getMaxDepthForCategory('event')
- **THEN** 返回 2（MaxLevel=3）

#### Scenario: Person 模板 depth 上界
- **WHEN** getMaxDepthForCategory('person')
- **THEN** 返回 1（MaxLevel=2）

### Requirement: 逐层聚合触发
PlaceTagInHierarchy 成功放置后，如果新 parent 的子标签数 ≥ 3 且 targetDepth < maxDepth，SHALL 异步触发 aggregateToUpperLevel。

#### Scenario: 子标签达到阈值触发上层聚合
- **WHEN** abstract A（depth=1）获得第 3 个子标签，且 tmpl.MaxLevel=3（maxDepth=2）
- **THEN** 异步执行 aggregateToUpperLevel(A.ID, tmpl, 1)

#### Scenario: 已到根不触发
- **WHEN** abstract A（depth=2）获得第 3 个子标签，且 tmpl.MaxLevel=3（maxDepth=2）
- **THEN** 不触发聚合，A 已是根节点

### Requirement: 废弃 Level 概念
GetTagLevel、GetTagLevelByID、ResolveLevelFromDepth 函数 SHALL 被删除。所有调用点改用 getTagDepthFromRoot(tagID) + tmpl.Levels[depth]。

#### Scenario: depth 直接索引模板定义
- **WHEN** tag 的 depth=1 且 tmpl.Levels[1] = {Name: "事件主体", ...}
- **THEN** placeTagAtLevel 使用 tmpl.Levels[1] 的 Name 和 Description 构建 prompt

### Requirement: 通用 prompt 函数
buildMatchPrompt(child, candidates, tmpl, levelDef) 和 buildCreationPrompt(child, tmpl, levelDef) SHALL 替代 4 个硬编码 prompt 函数（buildL2MatchPrompt, buildL1MatchPrompt, buildL2CreationPrompt, buildL1CreationPrompt）。

#### Scenario: 匹配 prompt 从 levelDef 取层级信息
- **WHEN** buildMatchPrompt 被调用，levelDef.Name="事件主体"，levelDef.Description="..."
- **THEN** prompt 包含 "事件主体" 作为目标层级名称

### Requirement: RetryOrphanPlacements
系统 SHALL 重试 >10min 无 abstract parent 的活跃叶标签的放置。

#### Scenario: 10 分钟前的孤儿标签被重试
- **WHEN** 标签 status='active', source='llm', 无 abstract parent, created_at < NOW()-10min
- **THEN** 对该标签执行 PlaceTagInHierarchy

#### Scenario: 10 分钟内的标签不被重试
- **WHEN** 标签 created_at > NOW()-10min
- **THEN** 不重试，等待 embedding 生成

### Requirement: AggregateOrphanTags
系统 SHALL 从叶向根方向逐层查找孤儿 abstract（有子标签但自身无 parent），在同 concept 内批量向上聚合。

#### Scenario: 孤儿 abstract 批量聚合
- **WHEN** event 模板下有 5 个 depth=1 的孤儿 abstract（有子标签但无 parent）
- **THEN** 按 concept 分组，每组内用 placeTagAtLevel 向 depth=2 放置

### Requirement: Placement scheduler 1h 间隔
新增 TagHierarchyPlacementScheduler，间隔 1 小时，执行 RetryOrphanPlacements + AggregateOrphanTags。

#### Scenario: 每小时执行一次
- **WHEN** scheduler 触发
- **THEN** 先执行 RetryOrphanPlacements，再执行 AggregateOrphanTags
