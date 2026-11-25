<!-- domain: topic-graph, ui -->

## Purpose

共享的有界 BFS 高亮算法，供主题图（TopicGraphCanvas）和 Section 生命周期面板（SectionLifecyclePanel）在 hover/选中时计算高亮集合，避免在密集图上全图高亮。

## Requirements

### Requirement: 共享的有界 BFS 高亮算法

系统 SHALL 提供泛型共享工具函数 `bfsHighlight(focusId, edges, totalNodes)`。节点 ID SHALL 支持 `string` 或 `number`，边 SHALL 使用 `{ source, target }` 结构并按无向图处理。

算法 SHALL：

1. 从 `focusId` 执行 BFS，收集完整连通分量。
2. 当 `totalNodes <= 8` 时返回完整连通分量。
3. 当连通分量节点数小于 `totalNodes * 0.4` 时返回完整连通分量。
4. 其他情况下重新执行 BFS，同时限制最大深度为 4 跳、最大节点数为 `max(1, floor(totalNodes * 0.4))`。
5. 即使 focus 节点没有任何边，也在结果中包含 focus 节点。

系统 SHALL 导出 `COMPONENT_THRESHOLD = 0.4`、`MAX_HOPS = 4` 和 `SMALL_GRAPH_NODE_LIMIT = 8` 常量。

#### Scenario: 稀疏图高亮完整连通分量

- **WHEN** 图有 100 个节点，focus 节点所在连通分量包含 25 个节点
- **THEN** `bfsHighlight` SHALL 返回全部 25 个节点

#### Scenario: 密集低直径图不会全图高亮

- **WHEN** 图有 100 个节点，focus 节点一跳即可到达其余 99 个节点
- **THEN** `bfsHighlight` SHALL 返回不超过 40 个节点，并包含 focus 节点

#### Scenario: 有界 BFS 同时限制跳数

- **WHEN** 图有 100 个节点，focus 节点所在连通分量包含 80 个节点，部分节点距离 focus 超过 4 跳
- **THEN** 返回集合 SHALL 不包含距离 focus 超过 4 跳的节点，且总数 SHALL 不超过 40

#### Scenario: 小图保持完整上下文

- **WHEN** 图有 5 个节点且全部连通
- **THEN** `bfsHighlight` SHALL 返回全部 5 个节点

#### Scenario: 孤立节点

- **WHEN** focus 节点不出现在任何边中
- **THEN** `bfsHighlight` SHALL 返回仅包含 focus 节点的集合

#### Scenario: 支持数字节点 ID

- **WHEN** focus ID 为数字 `50`，边为 `{source: 50, target: 60}` 和 `{source: 60, target: 70}`
- **THEN** 返回集合 SHALL 使用数字 ID 并包含 `50`、`60`、`70`

### Requirement: 主题图使用共享 BFS 高亮

`useTopicGraph` 的 `highlightedNodeIds` SHALL 基于当前 `displayedGraph` 调用共享 `bfsHighlight`，替代只收集直接邻居的一跳逻辑。调用前 SHALL 将可能被图组件解析为节点对象的 edge 端点转换为字符串节点 ID。

`relatedEdgeIds` SHALL 仅包含两端节点均位于高亮集合中的边。

#### Scenario: 选中话题高亮多跳分支

- **WHEN** 用户选中 A，显示图中存在 A-B、B-C，且该连通分量未达到密集图阈值
- **THEN** `highlightedNodeIds` SHALL 包含 A、B、C，`relatedEdgeIds` SHALL 包含 A-B 和 B-C

#### Scenario: 取消选中清除高亮

- **WHEN** `selectedTopicSlug` 和 `selectedKeywordSlug` 均为空
- **THEN** `highlightedNodeIds` 和 `relatedEdgeIds` SHALL 返回空数组

### Requirement: 生命周期面板使用共享 BFS 高亮

`SectionLifecyclePanel` SHALL 将 `SectionRelation.from_id/to_id` 映射为数字 `{ source, target }` 边，并使用共享 `bfsHighlight` 计算 hover 高亮集合。

节点 SHALL 在其 ID 位于高亮集合时高亮；边 SHALL 仅在其两端节点都位于高亮集合时高亮。高亮结果 SHALL 由响应式计算属性缓存，并在 hover 节点或 relations 变化时重新计算。

#### Scenario: 悬停 section 高亮多跳上下游

- **WHEN** 面板包含 #50 → #60 → #70，用户悬停 #50
- **THEN** #50、#60、#70 及两条内部关系边 SHALL 高亮

#### Scenario: 悬停孤立 section

- **WHEN** 面板仅包含无 relation 的 section #20，用户悬停 #20
- **THEN** 仅 #20 SHALL 高亮，且没有边高亮
