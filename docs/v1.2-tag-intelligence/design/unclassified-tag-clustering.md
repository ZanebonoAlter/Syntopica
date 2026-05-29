# 未分类标签离线聚类 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在定时清理调度器中新增"未分类标签聚类"阶段，用 pgvector embedding 构建相似度图 + 连通分量 + 批量 LLM 判断，自动合并或分组积累的未分类事件标签。

**Architecture:** 新增 Phase 3.5（在 Phase 3 层级修剪之后、Phase 3.5 遗留队列清理之前），处理所有无抽象父关系的 active event 标签。核心流程：收集未分类标签 → pgvector 批量相似度查询 → 构建无向图 → BFS 连通分量 → 每簇送 LLM 批量判断 → 执行 merge/abstract。阈值通过 `embedding_config` 表配置。

**Tech Stack:** Go, GORM, pgvector, 现有 `callLLMForTagJudgment` / `ExtractAbstractTag` 基础设施

---

### Task 1: 新增 embedding_config 配置项（迁移）

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go` — 新增迁移版本

**Step 1: 在迁移文件末尾添加新迁移**

在 `postgres_migrations.go` 的 `Migrations` 切片中，找到最后一个迁移版本，在其后追加：

```go
{
    Version:     "20260427_0003",
    Description: "Add clustering config keys to embedding_config.",
    Up: func(db *gorm.DB) error {
        defaults := []models.EmbeddingConfig{
            {Key: "cluster_similarity_threshold", Value: "0.70", Description: "Offline clustering: minimum embedding similarity to connect two unclassified tags"},
            {Key: "cluster_max_tags", Value: "200", Description: "Offline clustering: max unclassified tags to process per category per run"},
            {Key: "cluster_max_cluster_size", Value: "8", Description: "Offline clustering: max tags per cluster sent to LLM"},
        }
        for _, d := range defaults {
            var existing models.EmbeddingConfig
            if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
                if err := db.Create(&d).Error; err != nil {
                    logging.Warnf("Warning: failed to seed embedding_config key %s: %v", d.Key, err)
                }
            }
        }
        return nil
    },
},
```

**Step 2: 验证迁移**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

---

### Task 2: 扩展 EmbeddingConfigService 加载聚类配置

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/config_service.go`

**Step 1: 添加聚类配置结构体和加载方法**

在 `config_service.go` 末尾追加：

```go
type ClusterConfig struct {
	SimilarityThreshold float64
	MaxTags             int
	MaxClusterSize      int
}

var DefaultClusterConfig = ClusterConfig{
	SimilarityThreshold: 0.70,
	MaxTags:             200,
	MaxClusterSize:      8,
}

func (s *EmbeddingConfigService) LoadClusterConfig() ClusterConfig {
	config, err := s.LoadConfig()
	if err != nil {
		return DefaultClusterConfig
	}
	cfg := DefaultClusterConfig
	if v, ok := config["cluster_similarity_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.SimilarityThreshold = f
		}
	}
	if v, ok := config["cluster_max_tags"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxTags = n
		}
	}
	if v, ok := config["cluster_max_cluster_size"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxClusterSize = n
		}
	}
	return cfg
}
```

同时在 `UpdateConfig` 方法中，对新的配置 key 做校验（在已有的 `high_similarity_threshold` 校验逻辑后面追加）：

```go
if key == "cluster_similarity_threshold" {
    f, err := strconv.ParseFloat(value, 64)
    if err != nil {
        return fmt.Errorf("invalid threshold value %q: must be a number", value)
    }
    if f <= 0 || f > 1.0 {
        return fmt.Errorf("invalid threshold value %f: must be between 0 and 1.0", f)
    }
}
if key == "cluster_max_tags" || key == "cluster_max_cluster_size" {
    n, err := strconv.Atoi(value)
    if err != nil || n <= 0 {
        return fmt.Errorf("invalid value %q for %s: must be a positive integer", value, key)
    }
}
```

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

---

### Task 3: 实现 `FindSimilarTagsAmongSet` — 在指定标签子集中搜索相似标签

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`

**Step 1: 在 `FindSimilarTags` 方法后面添加新方法**

在 `embedding.go` 中 `FindSimilarTags` 函数之后（约 line 208）添加：

```go
type SimilarityEdge struct {
	TagAID     uint
	TagBID     uint
	Similarity float64
}

func (s *EmbeddingService) FindSimilarTagsAmongSet(ctx context.Context, tagIDs []uint, threshold float64) ([]SimilarityEdge, error) {
	if len(tagIDs) < 2 {
		return nil, nil
	}

	idStrs := make([]string, len(tagIDs))
	for i, id := range tagIDs {
		idStrs[i] = fmt.Sprintf("%d", id)
	}
	idList := strings.Join(idStrs, ",")

	query := fmt.Sprintf(`
		SELECT a.topic_tag_id AS tag_a_id, b.topic_tag_id AS tag_b_id,
		       1.0 - (a.embedding <=> b.embedding) AS similarity
		FROM topic_tag_embeddings a
		JOIN topic_tag_embeddings b ON a.topic_tag_id < b.topic_tag_id
		WHERE a.embedding_type = 'semantic'
		  AND b.embedding_type = 'semantic'
		  AND a.embedding IS NOT NULL
		  AND b.embedding IS NOT NULL
		  AND a.topic_tag_id IN (%s)
		  AND b.topic_tag_id IN (%s)
		  AND (1.0 - (a.embedding <=> b.embedding)) >= ?
		ORDER BY similarity DESC
	`, idList, idList)

	var edges []SimilarityEdge
	if err := database.DB.Raw(query, threshold).Scan(&edges).Error; err != nil {
		return nil, fmt.Errorf("failed to compute pairwise similarities: %w", err)
	}
	return edges, nil
}
```

这个方法用单条 SQL 计算指定标签集合内所有 pair 的相似度，只返回 >= threshold 的边。利用 pgvector 的 `<=>` 运算符在数据库层完成计算，避免在 Go 中逐对查询。

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

---

### Task 4: 实现连通分量算法

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/tag_clustering.go`

**Step 1: 创建聚类核心文件**

```go
package topicanalysis

import (
	"context"
	"fmt"
	"sort"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type TagCluster struct {
	TagIDs    []uint
	Tags      []*models.TopicTag
	AvgSim    float64
}

func findConnectedComponents(tagIDs []uint, edges []SimilarityEdge) [][]uint {
	adj := make(map[uint][]uint, len(tagIDs))
	for _, e := range edges {
		adj[e.TagAID] = append(adj[e.TagAID], e.TagBID)
		adj[e.TagBID] = append(adj[e.TagBID], e.TagAID)
	}

	visited := make(map[uint]bool, len(tagIDs))
	var components [][]uint

	for _, id := range tagIDs {
		if visited[id] {
			continue
		}
		var comp []uint
		queue := []uint{id}
		visited[id] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			comp = append(comp, cur)
			for _, nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		if len(comp) >= 2 {
			components = append(components, comp)
		}
	}
	return components
}

func collectUnclassifiedTagIDs(category string, limit int) ([]uint, error) {
	var relatedIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("parent_id", &relatedIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("child_id", &childIDs)
	relatedSet := make(map[uint]bool, len(relatedIDs)+len(childIDs))
	for _, id := range relatedIDs {
		relatedSet[id] = true
	}
	for _, id := range childIDs {
		relatedSet[id] = true
	}

	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("source != 'abstract'").
		Where("category = ?", category).
		Where("id IN (SELECT DISTINCT topic_tag_id FROM article_topic_tags)")

	if len(relatedSet) > 0 {
		var excluded []uint
		for id := range relatedSet {
			excluded = append(excluded, id)
		}
		query = query.Where("id NOT IN ?", excluded)
	}

	query = query.Order("quality_score DESC, feed_count DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var ids []uint
	if err := query.Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("collect unclassified %s tags: %w", category, err)
	}
	return ids, nil
}

func loadTagModelsMap(ids []uint) (map[uint]*models.TopicTag, error) {
	var tags []models.TopicTag
	if err := database.DB.Where("id IN ?", ids).Find(&tags).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]*models.TopicTag, len(tags))
	for i := range tags {
		m[tags[i].ID] = &tags[i]
	}
	return m, nil
}

func ClusterUnclassifiedTags(ctx context.Context, category string) (*ClusteringResult, error) {
	cfg := NewEmbeddingConfigService().LoadClusterConfig()
	return ClusterUnclassifiedTagsWithConfig(ctx, category, cfg)
}

func ClusterUnclassifiedTagsWithConfig(ctx context.Context, category string, cfg ClusterConfig) (*ClusteringResult, error) {
	result := &ClusteringResult{}

	tagIDs, err := collectUnclassifiedTagIDs(category, cfg.MaxTags)
	if err != nil {
		return nil, err
	}
	if len(tagIDs) < 2 {
		logging.Infof("ClusterUnclassifiedTags(%s): only %d unclassified tags, skipping", category, len(tagIDs))
		return result, nil
	}
	result.TagsCollected = len(tagIDs)
	logging.Infof("ClusterUnclassifiedTags(%s): collected %d unclassified tags", category, len(tagIDs))

	es := NewEmbeddingService()

	edges, err := es.FindSimilarTagsAmongSet(ctx, tagIDs, cfg.SimilarityThreshold)
	if err != nil {
		return nil, fmt.Errorf("similarity search for %s: %w", category, err)
	}
	result.EdgesFound = len(edges)
	logging.Infof("ClusterUnclassifiedTags(%s): found %d similarity edges (threshold=%.2f)", category, len(edges), cfg.SimilarityThreshold)

	if len(edges) == 0 {
		return result, nil
	}

	components := findConnectedComponents(tagIDs, edges)
	result.ClustersFound = len(components)
	logging.Infof("ClusterUnclassifiedTags(%s): found %d connected components", category, len(components))

	tagsMap, err := loadTagModelsMap(tagIDs)
	if err != nil {
		return nil, fmt.Errorf("load tag models: %w", err)
	}

	for _, comp := range components {
		if len(comp) > cfg.MaxClusterSize {
			logging.Infof("ClusterUnclassifiedTags(%s): cluster of size %d exceeds max %d, truncating to top-%d by quality_score",
				category, len(comp), cfg.MaxClusterSize, cfg.MaxClusterSize)
			sort.Slice(comp, func(i, j int) bool {
				a, b := tagsMap[comp[i]], tagsMap[comp[j]]
				return a.QualityScore > b.QualityScore
			})
			comp = comp[:cfg.MaxClusterSize]
		}

		var candidates []TagCandidate
		for _, id := range comp {
			tag := tagsMap[id]
			if tag == nil {
				continue
			}
			candidates = append(candidates, TagCandidate{
				Tag:        tag,
				Similarity: 1.0,
			})
		}
		if len(candidates) < 2 {
			continue
		}

		clusterLabel := candidates[0].Tag.Label
		extracted, err := ExtractAbstractTag(ctx, candidates, clusterLabel, category, WithCaller("ClusterUnclassifiedTags"))
		if err != nil {
			logging.Warnf("ClusterUnclassifiedTags(%s): cluster judgment failed: %v", category, err)
			result.Errors++
			continue
		}

		if extracted.Merge != nil && extracted.Merge.Target != nil {
			logging.Infof("ClusterUnclassifiedTags(%s): merged %d tags into %q",
				category, len(candidates), extracted.Merge.Target.Label)
			result.MergesApplied++
		}
		if extracted.Abstract != nil {
			logging.Infof("ClusterUnclassifiedTags(%s): created abstract %q with %d children",
				category, extracted.Abstract.Tag.Label, len(extracted.Abstract.Children))
			result.AbstractsCreated++
		}
		if extracted.LLMExplicitNone {
			logging.Infof("ClusterUnclassifiedTags(%s): LLM judged cluster as independent (none)", category)
		}
	}

	return result, nil
}

type ClusteringResult struct {
	TagsCollected   int `json:"tags_collected"`
	EdgesFound      int `json:"edges_found"`
	ClustersFound   int `json:"clusters_found"`
	MergesApplied   int `json:"merges_applied"`
	AbstractsCreated int `json:"abstracts_created"`
	Errors          int `json:"errors"`
}
```

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

---

### Task 5: 在调度器中接入 Phase 3.5 聚类

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`

**Step 1: 在 `TagHierarchyCleanupRunSummary` 中添加聚类统计字段**

在 `QueuedMultiParentResolved` 字段后（约 line 40）添加：

```go
ClusterTagsCollected    int `json:"cluster_tags_collected"`
ClusterEdgesFound       int `json:"cluster_edges_found"`
ClusterClustersFound    int `json:"cluster_clusters_found"`
ClusterMergesApplied    int `json:"cluster_merges_applied"`
ClusterAbstractsCreated int `json:"cluster_abstracts_created"`
```

**Step 2: 在 `runCleanupCycle` 中，Phase 3 层级修剪和 Phase 3.5 遗留队列之间插入聚类阶段**

在 Phase 3 空抽象清理（`CleanupEmptyAbstractNodes`）之后、原 Phase 3.5 遗留队列清理之前，插入：

```go
// Phase 3.5-pre: Unclassified tag clustering (LLM-assisted)
if budget.IsTimedOut() {
    logging.Infoln("Phase 3.5-pre: budget timed out, skipping unclassified tag clustering")
} else {
    phaseStart = time.Now()
    for _, category := range []string{"event", "keyword"} {
        if budget.IsTimedOut() {
            logging.Infoln("Phase 3.5-pre: budget timed out, skipping remaining categories")
            break
        }
        clusterResult, clusterErr := topicanalysis.ClusterUnclassifiedTags(context.Background(), category)
        if clusterErr != nil {
            logging.Errorf("Phase 3.5-pre clustering failed for %s: %v", category, clusterErr)
            summary.Errors++
            continue
        }
        summary.ClusterTagsCollected += clusterResult.TagsCollected
        summary.ClusterEdgesFound += clusterResult.EdgesFound
        summary.ClusterClustersFound += clusterResult.ClustersFound
        summary.ClusterMergesApplied += clusterResult.MergesApplied
        summary.ClusterAbstractsCreated += clusterResult.AbstractsCreated
        summary.Errors += clusterResult.Errors
        logging.Infof("Phase 3.5-pre (%s): collected=%d edges=%d clusters=%d merges=%d abstracts=%d",
            category, clusterResult.TagsCollected, clusterResult.EdgesFound,
            clusterResult.ClustersFound, clusterResult.MergesApplied, clusterResult.AbstractsCreated)
    }
    logging.Infof("Phase 3.5-pre completed in %v", time.Since(phaseStart))
}
```

**Step 3: 更新 `Reason` 字段的格式字符串**

在 `summary.Reason = fmt.Sprintf(...)` 行中，在 `empty_abstracts=%d` 后面追加 `,cluster_tags=%d,cluster_merges=%d,cluster_abstracts=%d`，并添加对应的参数：

```go
summary.ClusterTagsCollected, summary.ClusterMergesApplied, summary.ClusterAbstractsCreated,
```

**Step 4: 更新所有 "8-phase" 引用为 "9-phase"**

在整个文件中将所有 `8-phase` 替换为 `9-phase`（共 4 处：struct 注释、initSchedulerTask description x2、另一处注释）。

**Step 5: 更新调度器注释**

将 `TagHierarchyCleanupScheduler` 结构体上方的注释改为：

```go
// TagHierarchyCleanupScheduler runs a 9-phase tag cleanup cycle: zombie cleanup, flat merge, hierarchy pruning, unclassified tag clustering, queued multi-parent resolve, adopt narrower, abstract update, tree review, and description backfill
```

**Step 5: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

---

### Task 6: 更新文档

**Files:**
- Modify: `docs/guides/tagging-flow.md` — 第 12 节"标签清理机制"
- Modify: `docs/guides/topic-graph.md` — 质量分数相关部分（如有需要）

**Step 1: 在 `tagging-flow.md` 的定时调度流程图中插入 Phase 3.5-pre**

在第 12 节的 mermaid 流程图中，在 `P3D`（清理空抽象节点）和 `P3E`（Phase 3.5 遗留队列清理）之间插入：

```mermaid
P3D --> P3D5[Phase 3.5-pre: 未分类标签聚类]
P3D5 --> |收集无抽象父标签 + pgvector相似度图 + BFS连通分量 + LLM批量判断| P3E
```

**Step 2: 在阶段表格中添加新行**

在 Phase 3 和 Phase 3.5 之间插入：

| Phase 3.5-pre 聚类 | `tag_clustering.go` `ClusterUnclassifiedTags` | 是 | 无抽象父的 active event/keyword 标签，相似度 ≥ 配置阈值（默认 0.70） |

**Step 3: 在核心文件列表中添加新文件引用**

在 `核心文件` 行中添加 `topicanalysis/tag_clustering.go`（Phase 3.5-pre 未分类标签聚类）。

**Step 4: 更新 `topic-graph.md` 中的质量评分章节**

在"标签质量评分与低质量标签"章节末尾，追加一个子节：

```markdown
### 未分类标签离线聚类

#### 概述

定时清理调度器在 Phase 3.5-pre 阶段对没有抽象父关系的 active event/keyword 标签执行离线聚类。与创建时的实时匹配（阈值 0.78）相比，离线聚类使用更宽松的阈值（默认 0.70），能捕获实时阶段漏掉的相似标签对。

#### 配置项

| Key | 默认值 | 说明 |
|-----|--------|------|
| `cluster_similarity_threshold` | `0.70` | 两个标签 embedding 相似度 ≥ 此值才建立边 |
| `cluster_max_tags` | `200` | 每个分类每次最多处理的未分类标签数 |
| `cluster_max_cluster_size` | `8` | 每个连通分量最大标签数，超出截取质量分最高的 N 个 |

配置存储在 `embedding_config` 表，可通过 API `PUT /api/embedding/config/:key` 修改。

#### 聚类流程

1. 收集指定 category 下无抽象父关系的 active 标签（上限 200）
2. 用 pgvector 单条 SQL 计算所有标签对的 cosine 相似度（≥ 阈值）
3. 构建无向相似度图，BFS 找连通分量
4. 每个连通分量（≤ 8 标签）送 `ExtractAbstractTag` 由 LLM 判断 merge/abstract/none
5. 执行 merge 或创建抽象父标签

#### 与实时匹配的区别

| 维度 | 实时匹配 (`findOrCreateTag`) | 离线聚类 (`ClusterUnclassifiedTags`) |
|------|------|------|
| 阈值 | 0.78 | 0.70（更宽松） |
| 触发时机 | 标签创建时 | 定时清理周期 |
| 覆盖范围 | 新标签 vs 已有标签 | 全量未分类标签之间 |
| 传递性 | 无 | 有（通过连通分量） |

#### 相关代码

| 文件 | 职责 |
|------|------|
| `backend-go/internal/domain/topicanalysis/tag_clustering.go` | 聚类核心逻辑：收集、相似度图、连通分量、LLM 判断 |
| `backend-go/internal/domain/topicanalysis/config_service.go` | `ClusterConfig` 配置加载 |
| `backend-go/internal/jobs/tag_hierarchy_cleanup.go` | Phase 3.5-pre 调度 |
```

---

### Task 7: 端到端验证

**Step 1: 编译整个后端**

Run: `cd backend-go && go build ./...`
Expected: 编译成功，无错误

**Step 2: 运行相关单元测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v -run "Cluster|Config"`
Expected: 新测试通过（如有），现有测试不受影响

**Step 3: 运行全量测试**

Run: `cd backend-go && go test ./... -count=1`
Expected: 全部通过

**Step 4: 启动后端确认迁移**

Run: `cd backend-go && go run cmd/server/main.go`
Expected: 日志中看到迁移 `20260427_0001` 执行成功

**Step 5: 手动触发清理验证**

```bash
curl -X POST http://localhost:5000/api/schedulers/trigger/tag_hierarchy_cleanup
```

Expected: 响应中包含 `cluster_tags_collected`、`cluster_merges_applied` 等新字段

**Step 6: 查看配置是否正确写入**

```bash
curl http://localhost:5000/api/embedding/config
```

Expected: 返回列表中包含 `cluster_similarity_threshold`、`cluster_max_tags`、`cluster_max_cluster_size` 三个新配置项

---

## 总结

改动范围：

| 文件 | 改动类型 |
|------|----------|
| `backend-go/internal/platform/database/postgres_migrations.go` | 新增迁移版本 seed 3 个配置项 |
| `backend-go/internal/domain/topicanalysis/config_service.go` | 新增 `ClusterConfig` 结构体和加载逻辑 |
| `backend-go/internal/domain/topicanalysis/embedding.go` | 新增 `FindSimilarTagsAmongSet` 和 `SimilarityEdge` |
| `backend-go/internal/domain/topicanalysis/tag_clustering.go` | **新建文件**，聚类核心逻辑 |
| `backend-go/internal/jobs/tag_hierarchy_cleanup.go` | 新增 Phase 3.5-pre 调用和统计字段 |
| `docs/guides/tagging-flow.md` | 文档更新 |
| `docs/guides/topic-graph.md` | 文档更新 |

无前端改动。纯后端新增一个清理阶段 + 3 个可配置参数。
