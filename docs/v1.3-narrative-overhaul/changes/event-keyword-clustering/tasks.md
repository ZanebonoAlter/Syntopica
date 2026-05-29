## 1. 数据层：配置项

- [x] 1.1 新增 `embedding_config` 种子数据：`event_cluster_kw_min_overlap` (值=2) 和 `event_cluster_sem_threshold` (值=0.80)，写入 migration 或 seed 脚本
- [x] 1.2 在 `ClusterConfig` 结构体中新增 `KwMinOverlap int` 和 `SemThreshold float64` 字段，扩展 `LoadClusterConfig` 读取新配置项

## 2. 核心聚类逻辑

- [x] 2.1 在 `tag_clustering.go` 新增 `FindSimilarTagsByKeywordOverlap(ctx, tagIDs, kwMinOverlap, semThreshold)` 函数：Stage 1 用 SQL `jsonb_array_elements_text` 计算关键词交集，Stage 2 用 `topic_tag_embeddings` (semantic) 过滤
- [x] 2.2 为 `FindSimilarTagsByKeywordOverlap` 编写单元测试：验证 shared_kws>=2 过滤、semantic 过滤、空输入、无 event_keywords 的 tag 处理

## 3. ClusterUnclassifiedTags 集成

- [x] 3.1 修改 `ClusterUnclassifiedTagsWithConfig`：当 category="event" 时调用 `FindSimilarTagsByKeywordOverlap` 替代 `FindSimilarTagsAmongSet`
- [x] 3.2 修改 `ClusteringResult` 统计字段：新增 `EventKeywordEdgesFound` 记录 Stage 1 产出的边数
- [x] 3.3 为 event 分支编写集成测试：mock tag 数据验证两阶段过滤链路

## 4. Scheduler 集成

- [x] 4.1 在 `tag_hierarchy_cleanup.go` 的 `runCleanupCycle` 中，Phase 2 之后新增 Phase 2.5：调用 `ClusterUnclassifiedTags(ctx, "event")`，受 budget 控制
- [x] 4.2 更新 `TagHierarchyCleanupRunSummary` 新增 `EventClustersFound`、`EventKeywordEdges` 统计字段
- [x] 4.3 更新 scheduler description 字符串包含 "event-clustering"
- [x] 4.4 验证 scheduler 注册逻辑（`registerOrSyncTask`）自动更新 description

## 5. 验证

- [x] 5.1 后端：`golangci-lint run ./...` + `go vet ./...` + `go test ./...` + `go build ./...`
- [ ] 5.2 手动触发一次 cleanup cycle，观察日志确认 Phase 2.5 执行并产出聚合结果
- [ ] 5.3 查询数据库验证孤立 event tag 数量下降
