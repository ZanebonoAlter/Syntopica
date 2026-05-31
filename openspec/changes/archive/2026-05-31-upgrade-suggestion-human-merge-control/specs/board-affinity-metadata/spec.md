## ADDED Requirements

### Requirement: 簇级别 Board Affinity 元数据计算
系统 SHALL 在聚类完成后，为每个簇计算与所有已有板块的亲和度（BoardAffinity），作为参考元数据附加到簇上，不影响聚类结果。

#### Scenario: 簇与已有板块有相似候选
- **WHEN** 簇包含候选 [AI, 大语言模型, GPT]，且已有板块 "人工智能"（board_id=42）包含 embedding 与 "AI" 候选距离 ≤ threshold
- **THEN** 系统 SHALL 在该簇的 board_affinities 中包含 `{board_id: 42, board_label: "人工智能", matching_candidates: 1, avg_distance: <计算值>}`

#### Scenario: 簇与已有板块无相似候选
- **WHEN** 簇包含候选 [新能源, 光伏, 储能]，且无已有板块的任何辅助标签 embedding 与这些候选距离 ≤ threshold
- **THEN** 系统 SHALL 在该簇的 board_affinities 中不包含该板块

#### Scenario: 无已有板块
- **WHEN** 系统处于冷启动阶段，无任何 label_type="board" 的 semantic_labels
- **THEN** 系统 SHALL 返回空的 board_affinities 列表，聚类正常进行

### Requirement: Board Affinity 匹配候选计数
系统 SHALL 对每个簇的每个已有板块，统计簇内与该板块任意辅助标签 embedding 距离 ≤ cluster_distance_threshold 的候选数量作为 matching_candidates。

#### Scenario: 多个候选匹配同一板块
- **WHEN** 簇有 5 个候选，其中 3 个与板块 A 的辅助标签距离 ≤ threshold
- **THEN** 系统 SHALL 设置 matching_candidates=3，avg_distance 为这 3 个候选的最小距离的平均值

### Requirement: Board Affinity 数据随 Cluster 返回
系统 SHALL 将 board_affinities 附加到 UpgradeCluster 响应中，随 upgrade/candidates 接口一起返回前端。

#### Scenario: 前端获取候选数据
- **WHEN** 前端调用 `/api/semantic-boards/upgrade/candidates`
- **THEN** 每个 cluster 对象 SHALL 包含 `board_affinities` 数组，每项含 `board_id`、`board_label`、`matching_candidates`、`avg_distance`
