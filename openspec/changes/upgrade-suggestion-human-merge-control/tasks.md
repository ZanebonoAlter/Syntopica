## 1. 后端：数据结构与聚类逻辑

- [x] 1.1 在 `semantic_board_upgrade.go` 中新增 `BoardAffinity` 结构体（`BoardID uint`, `BoardLabel string`, `MatchingCandidates int`, `AvgDistance float64`），在 `SemanticBoardUpgradeCluster` 中添加 `BoardAffinities []BoardAffinity` 字段；同时移除 Phase A 遗留字段（`ExistingBoardID`、`ExistingBoardLabel`、`ExistingBoardDescription`、`ExistingBoardAuxiliaryLabels`）
- [x] 1.2 重构 `clusterCandidates()`：移除 Phase A 对聚类的直接影响（删除 `closestBoardContext` 调用、`boardClusterByID` 分支、`boardDetails` 计算），所有候选统一走纯自聚类（原 Phase B 逻辑），保留 `loadExistingBoardContexts()` 调用用于 affinity 计算
- [x] 1.3 **两趟聚类（Pass 2 重分配）**：Pass 1 不变（贪心初始分群 + running-mean centroid），然后计算稳定 centroid（`computeStableCentroid`），Pass 2 将每个候选重新分配到距离最近的稳定 centroid（距离 > threshold 则独立成簇），解决第一簇过度吸积和大量单标签簇问题
- [x] 1.4 在 `clusterCandidates()` 末尾添加 board affinity 计算逻辑：对每个簇遍历 `boardContexts`，统计簇内候选与每个已有板块辅助标签的距离 ≤ threshold 的数量和平均距离，生成 `BoardAffinities` 列表
- [x] 1.5 更新 `buildSemanticBoardUpgradePrompt()`：移除 `merge_into_existing` 选项说明和 `target_board_id` 字段要求；移除 `if cluster.ExistingBoardID != nil` 死代码分支；将 `board_affinities` 信息注入 prompt（"该簇与已有板块 X 相似（N 个候选匹配，平均距离 Y）"）
- [x] 1.6 更新 `filterSemanticBoardUpgradeSuggestions()`：显式过滤掉 `merge_into_existing` 决策（防御性编程）

## 2. 后端：Handler DTO 与 suggestion 内嵌 board_affinities

- [x] 2.1 更新 `semanticBoardUpgradeClusterDTO`：移除 `ExistingBoardID`、`ExistingBoardLabel`、`ExistingBoardDescription`、`ExistingBoardAuxiliaryLabels` 字段；新增 `BoardAffinities` 字段（含 JSON tag `board_affinities`）
- [x] 2.2 更新 `semanticBoardUpgradeSuggestionDTO`：新增 `BoardAffinities` 字段（JSON tag `board_affinities`）
- [x] 2.3 更新 `upgradeClustersToDTO()`：移除旧字段映射，添加 `BoardAffinities` 序列化
- [x] 2.4 更新 `suggestionsToDTO()`：接收 `clusters []SemanticBoardUpgradeCluster` 参数，根据 suggestion 的 `auxiliary_label_ids` 找到对应 cluster，将 `board_affinities` 汇总内嵌到 suggestion DTO 中；保留 `target_board_id` 和 `target_board_label` 字段（用于前端 merge 操作回填）
- [x] 2.5 更新 `suggestUpgrades` handler：将 `GenerateSuggestions` 返回的 clusters 传递给 `suggestionsToDTO`

## 3. 后端：测试更新

- [x] 3.1 重写 `TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards`：验证纯自聚类不产生 `ExistingBoardID` 非 nil 的簇（原断言需反转），验证 `BoardAffinities` 正确计算（匹配候选数、平均距离）
- [x] 3.2 新增 `TestClusterCandidatesBoardAffinities`：覆盖无已有板块（空 affinities）、簇与板块有匹配、簇与板块无匹配场景
- [x] 3.3 新增 `TestClusterCandidatesPass2Reassignment`：验证候选被重新分配到最近的稳定 centroid
- [x] 3.4 新增 `TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster`：验证链式嵌入不会被第一簇吞掉
- [x] 3.5 验证 `ConfirmSuggestion` 的 `merge_into_existing` 路径仍正常工作（现有 `TestSemanticBoardUpgradeConfirmMergeIntoExisting` 应通过，无需修改）

## 4. 前端：类型与 API

- [x] 4.1 更新 `front/app/api/semanticBoards.ts`：`UpgradeCluster` 接口移除 `existing_board_id`、`existing_board_label`、`existing_board_description`、`existing_board_auxiliary_labels`；新增 `board_affinities: { board_id: number; board_label: string; matching_candidates: number; avg_distance: number }[]`
- [x] 4.2 更新 `UpgradeSuggestion` 接口：新增 `board_affinities` 字段（同 UpgradeCluster 中的类型）；保留 `target_board_id` 和 `target_board_label` 用于 merge 操作
- [x] 4.3 修复 pre-existing bug：`UpgradeCluster.existing_board_auxiliary_labels` 类型原为 `number[]` 应为 `string[]`（已被 4.1 移除，无需单独修复）

## 5. 前端：UpgradeSuggestionPanel UI

- [x] 5.1 为每个非 skip 的建议卡片添加 board affinity 参考信息展示区（相似板块名称、匹配候选数、平均距离），数据来源为 suggestion 内嵌的 `board_affinities`
- [x] 5.2 为每个非 skip 且 board_affinities 非空的建议卡片添加"合并到..."下拉按钮，下拉选项按 avg_distance 升序排列，选项显示 board_label 和匹配候选数
- [x] 5.3 下拉选择后，构造新请求体 `{decision: "merge_into_existing", target_board_id, auxiliary_label_ids: s.auxiliary_label_ids}` 调用 executeUpgrade API（不修改原始 suggestion 对象的 decision 字段）
- [x] 5.4 无 board_affinities 时隐藏"合并到..."按钮

## 6. 聚类质量诊断与后续修正

- [x] 6.1 使用真实候选数据诊断当前 `centroid + Pass2` 聚类质量：记录最大簇、单标签簇、簇内 pairwise 距离、全局均值 hub 效应、阈值扫描、候选策略对比
- [x] 6.2 **后端：新增 ClusterMethod 配置字段**：在 `SemanticBoardUpgradeConfig` 中添加 `ClusterMethod string`（默认 `"average_link"`）；`LoadUpgradeConfig()` 加载新 key `semantic_board_upgrade_cluster_method`；handler 侧更新 `isSemanticBoardConfigKey`（加新 key）、`validateSemanticBoardConfigValue`（只允许 `"average_link"` / `"centroid"`）、`semanticBoardUpgradeConfigToMap`（输出新 key）
- [x] 6.3 **后端：实现 average-link greedy 聚类**：新增 `candidateFitsClusterAverageLink()` 函数（连通性约束 + 平均距离约束），修改 `clusterCandidates()` 根据 `config.ClusterMethod` 分支调用旧 centroid 逻辑或新 average-link 逻辑；移除 Pass 2 相关代码（`computeStableCentroid` Pass 2 调用、稳定 centroid 数组、重分配循环）；移除不再需要的 `candidateFitsCluster`、`addCandidateToCluster`、`updateCentroid`（仅当 `ClusterMethod == "centroid"` 时保留旧分支）
- [x] 6.4 **数据库：插入新配置行**：在 `ai_settings` 中插入 `semantic_board_upgrade_cluster_method = "average_link"`（默认值），description 标注聚类算法选择
- [x] 6.5 **前端：MatchingConfig 接口 + 对话框**：`MatchingConfig` 接口添加 `semantic_board_upgrade_cluster_method: string`；`MatchingConfigDialog.vue` 升级建议区块添加"聚类算法"下拉（`average_link` / `centroid`），含中文说明
- [x] 6.6 **测试**：重写现有 centroid 聚类测试为 average-link 版本（`TestClusterCandidatesAverageLink...`），覆盖：最大簇不应超过 30 成员、单簇和多簇场景、连通性约束阻隔不相关标签、board_affinity 计算仍正确；保留 1 个 centroid 模式测试确保回退兼容
- [x] 6.7 **文档**：更新 `docs/reference/api/semantic-boards.md` 的配置示例（加 `cluster_method`）；更新 `docs/reference/architecture/backend.md` 聚类说明；更新 `docs/reference/database/DATABASE_FIELDS.md` 配置参数列表
