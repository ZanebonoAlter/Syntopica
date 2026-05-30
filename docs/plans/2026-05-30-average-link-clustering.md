# Average-link Greedy Clustering 实施计划

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 将 `clusterCandidates()` 从 centroid 阈值聚类替换为 average-link greedy，通过 DB 配置可切换回 centroid。

**Architecture:** 新增 `ClusterMethod` 配置字段（`ai_settings` 表），`clusterCandidates()` 根据 config 分支执行。average-link 单遍扫描，候选需与簇内真实成员 pairwise 接近（连通性 + 平均距离 ≤ threshold）。保留旧 centroid 逻辑作为回退。

**Tech Stack:** Go (Gin/GORM), PostgreSQL, Vue 3/TypeScript (Nuxt 4), Tailwind CSS v4

---

## Task 1: 后端 — ClusterMethod 配置字段 (6.2)

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:29` (SemanticBoardUpgradeConfig struct)
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:469` (LoadUpgradeConfig)
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:1627` (isSemanticBoardConfigKey)
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:1637` (validateSemanticBoardConfigValue)
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:1603` (semanticBoardUpgradeConfigToMap)

**Step 1: 修改 SemanticBoardUpgradeConfig 结构体**

在 `backend-go/internal/domain/tagging/semantic_board_upgrade.go` 第 29 行，添加 `ClusterMethod` 字段：

```go
type SemanticBoardUpgradeConfig struct {
	RefCountThreshold        int
	ClusterDistanceThreshold float64
	CoTagWindowDays          int
	CoTagTopN                int
	CoTagDedupeSimThreshold  float64
	CoTagHardLimit           int
	ClusterMethod            string
}
```

**Step 2: 修改 LoadUpgradeConfig — 添加默认值和加载逻辑**

在 `LoadUpgradeConfig` 函数中：
- 默认值设置 `ClusterMethod: "average_link"`
- `Where("key IN ?"...)` 的 slice 添加 `"semantic_board_upgrade_cluster_method"`
- switch 添加 case：
```go
case "semantic_board_upgrade_cluster_method":
	if v := strings.TrimSpace(setting.Value); v == "average_link" || v == "centroid" {
		config.ClusterMethod = v
	}
```

**Step 3: 修改 handler 三个函数**

`isSemanticBoardConfigKey` switch 添加：
```go
"semantic_board_upgrade_cluster_method":
```

`validateSemanticBoardConfigValue` switch 添加：
```go
case "semantic_board_upgrade_cluster_method":
	if value != "average_link" && value != "centroid" {
		return fmt.Errorf("%s must be 'average_link' or 'centroid'", key)
	}
	return nil
```

`semanticBoardUpgradeConfigToMap` 添加：
```go
"semantic_board_upgrade_cluster_method": config.ClusterMethod,
```

**Step 4: 验证编译**

```bash
cd /mnt/d/project/Syntopica/backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade.go backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat(upgrade): add ClusterMethod config field for clustering algorithm selection"
```

---

## Task 2: 后端 — 实现 average-link greedy 聚类 (6.3)

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:243` (clusterCandidates)
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:508` (candidateFitsCluster 附近)

**Step 1: 新增 `candidateFitsClusterAverageLink` 函数**

在 `candidateFitsCluster` 函数附近添加：

```go
// candidateFitsClusterAverageLink checks if a candidate can join a cluster using
// average-link greedy rules: (1) connectivity — at least one real member within threshold,
// (2) average distance — mean pairwise distance to all members ≤ threshold.
// Returns (fits bool, avgDist float64).
func candidateFitsClusterAverageLink(candidate SemanticBoardUpgradeCandidate, cluster *SemanticBoardUpgradeCluster, threshold float64) (bool, float64) {
	if len(cluster.Candidates) == 0 {
		return false, 1
	}
	totalDist := 0.0
	hasConnected := false
	for _, member := range cluster.Candidates {
		dist := semanticBoardUpgradeDistance(candidate.Embedding, member.Embedding)
		totalDist += dist
		if dist <= threshold {
			hasConnected = true
		}
	}
	avgDist := totalDist / float64(len(cluster.Candidates))
	return hasConnected && avgDist <= threshold, avgDist
}
```

**Step 2: 修改 `clusterCandidates()` — 根据 ClusterMethod 分支**

将 `clusterCandidates` 中的 Pass 1 循环改为：

```go
func (s *SemanticBoardUpgradeService) clusterCandidates(ctx context.Context, candidates []SemanticBoardUpgradeCandidate, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCluster, error) {
	boardContexts, err := s.loadExistingBoardContexts(ctx)
	if err != nil {
		return nil, err
	}

	var clusters []SemanticBoardUpgradeCluster
	if config.ClusterMethod == "average_link" {
		clusters = clusterAverageLink(candidates, config.ClusterDistanceThreshold)
	} else {
		clusters = clusterCentroid(candidates, config.ClusterDistanceThreshold)
	}

	// Compute board affinities (unchanged — keep existing code below)
	// ... (board affinity calculation stays exactly the same)
```

**Step 3: 提取旧 centroid 逻辑为 `clusterCentroid` 函数**

将当前 Pass 1 + Pass 2 逻辑提取到独立函数：

```go
func clusterCentroid(candidates []SemanticBoardUpgradeCandidate, threshold float64) []SemanticBoardUpgradeCluster {
	// Pass 1: greedy with running-mean centroid (existing code)
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	for _, candidate := range candidates {
		matched := false
		for i := range clusters {
			if candidateFitsCluster(candidate, &clusters[i], threshold) {
				addCandidateToCluster(candidate, &clusters[i])
				matched = true
				break
			}
		}
		if !matched {
			clusters = append(clusters, SemanticBoardUpgradeCluster{
				Candidates: []SemanticBoardUpgradeCandidate{candidate},
				Centroid:   candidate.Embedding,
			})
		}
	}

	// Pass 2: reassign to stable centroids (existing code)
	if len(clusters) > 1 {
		stableCentroids := make([][]float64, len(clusters))
		for i, cl := range clusters {
			stableCentroids[i] = computeStableCentroid(cl.Candidates)
		}
		newClusters := make([]SemanticBoardUpgradeCluster, 0, len(clusters))
		for _, candidate := range candidates {
			bestIdx := -1
			bestDist := threshold + 1
			for i := range stableCentroids {
				if len(stableCentroids[i]) == 0 {
					continue
				}
				dist := semanticBoardUpgradeDistance(candidate.Embedding, stableCentroids[i])
				if dist <= threshold && dist < bestDist {
					bestDist = dist
					bestIdx = i
				}
			}
			if bestIdx >= 0 {
				found := false
				for j := range newClusters {
					if newClusters[j].origIdx == bestIdx {
						newClusters[j].Candidates = append(newClusters[j].Candidates, candidate)
						found = true
						break
					}
				}
				if !found {
					newClusters = append(newClusters, SemanticBoardUpgradeCluster{
						Candidates: []SemanticBoardUpgradeCandidate{candidate},
						origIdx:    bestIdx,
					})
				}
			} else {
				newClusters = append(newClusters, SemanticBoardUpgradeCluster{
					Candidates: []SemanticBoardUpgradeCandidate{candidate},
					origIdx:    -1,
				})
			}
		}
		for i := range newClusters {
			newClusters[i].Centroid = computeStableCentroid(newClusters[i].Candidates)
		}
		clusters = newClusters
	}
	return clusters
}
```

**Step 4: 新增 `clusterAverageLink` 函数**

```go
func clusterAverageLink(candidates []SemanticBoardUpgradeCandidate, threshold float64) []SemanticBoardUpgradeCluster {
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	for _, candidate := range candidates {
		bestIdx := -1
		bestAvgDist := threshold + 1
		for i := range clusters {
			fits, avgDist := candidateFitsClusterAverageLink(candidate, &clusters[i], threshold)
			if fits && avgDist < bestAvgDist {
				bestAvgDist = avgDist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			clusters[bestIdx].Candidates = append(clusters[bestIdx].Candidates, candidate)
		} else {
			clusters = append(clusters, SemanticBoardUpgradeCluster{
				Candidates: []SemanticBoardUpgradeCandidate{candidate},
			})
		}
	}
	// Compute centroids for each cluster (for display purposes, not used in clustering)
	for i := range clusters {
		clusters[i].Centroid = computeStableCentroid(clusters[i].Candidates)
	}
	return clusters
}
```

**Step 5: 验证编译 + 现有测试**

```bash
cd /mnt/d/project/Syntopica/backend-go && go build ./...
go test ./internal/domain/tagging/ -run TestSemanticBoardUpgrade -v
```

**Step 6: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade.go
git commit -m "feat(upgrade): implement average-link greedy clustering with config switch"
```

---

## Task 3: 数据库 — 插入新配置行 (6.4)

**Files:** N/A (直接 SQL)

**Step 1: 连接数据库执行 INSERT**

```bash
docker exec syntopica-postgres psql -U postgres -d syntopica -c \
  "INSERT INTO ai_settings (key, value, description, created_at, updated_at)
   VALUES ('semantic_board_upgrade_cluster_method', 'average_link', '聚类算法选择: average_link (平均链接贪心) 或 centroid (质心阈值)', NOW(), NOW())
   ON CONFLICT (key) DO NOTHING;"
```

**Step 2: 验证**

```bash
docker exec syntopica-postgres psql -U postgres -d syntopica -c \
  "SELECT key, value FROM ai_settings WHERE key = 'semantic_board_upgrade_cluster_method';"
```

预期输出一行 `semantic_board_upgrade_cluster_method | average_link`

---

## Task 4: 前端 — MatchingConfig 接口 + 对话框 (6.5)

**Files:**
- Modify: `front/app/api/semanticBoards.ts:100` (MatchingConfig interface)
- Modify: `front/app/features/tags/components/MatchingConfigDialog.vue` (升级建议区块)

**Step 1: 更新 MatchingConfig 接口**

在 `front/app/api/semanticBoards.ts` 的 `MatchingConfig` 接口末尾（`semantic_board_upgrade_cotag_hard_limit` 之后）添加：

```typescript
  semantic_board_upgrade_cluster_method: string
```

**Step 2: 更新 MatchingConfigDialog.vue**

在升级建议区块的 `mc-grid` 中（`semantic_board_upgrade_cotag_hard_limit` 字段之后），添加聚类算法下拉：

```vue
<label class="mc-field">
  <span class="mc-label">聚类算法</span>
  <span class="mc-hint">average_link: 平均链接贪心，候选需与簇内真实成员 pairwise 接近（推荐）；centroid: 质心阈值聚类（旧算法，仅回退用）</span>
  <select v-model="form.semantic_board_upgrade_cluster_method" class="mc-input">
    <option value="average_link">average_link（推荐）</option>
    <option value="centroid">centroid（旧算法）</option>
  </select>
</label>
```

注意：使用 `<select>` 而非 `<input>` 因为只有两个合法值。

**Step 3: 验证 lint**

```bash
cd /mnt/d/project/Syntopica/front && pnpm lint
```

**Step 4: Commit**

```bash
git add front/app/api/semanticBoards.ts front/app/features/tags/components/MatchingConfigDialog.vue
git commit -m "feat(front): add cluster method selector to matching config dialog"
```

---

## Task 5: 测试 — 更新聚类测试 (6.6)

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: 添加 TestClusterCandidatesAverageLinkBasic**

在 `TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster` 之后添加新测试：

```go
func TestClusterCandidatesAverageLinkBasic(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "OpenAI", RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: "GPT", RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: "Battery", RefCount: 5, Embedding: []float64{0, 1, 0}},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "average_link"

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)
	require.Len(t, clusters, 2)

	// Find cluster with A and B
	var abCluster *SemanticBoardUpgradeCluster
	for i := range clusters {
		for _, c := range clusters[i].Candidates {
			if c.ID == candidateA.ID {
				abCluster = &clusters[i]
				break
			}
		}
	}
	require.NotNil(t, abCluster)
	require.Len(t, abCluster.Candidates, 2)
	abIDs := upgradeCandidateIDs(abCluster.Candidates)
	require.Contains(t, abIDs, candidateA.ID)
	require.Contains(t, abIDs, candidateB.ID)

	// Cluster with C should be separate
	var cCluster *SemanticBoardUpgradeCluster
	for i := range clusters {
		for _, c := range clusters[i].Candidates {
			if c.ID == candidateC.ID {
				cCluster = &clusters[i]
				break
			}
		}
	}
	require.NotNil(t, cCluster)
	require.Len(t, cCluster.Candidates, 1)
}

func TestClusterCandidatesAverageLinkNoGiantCluster(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	// Chain of 5 embeddings: each close to neighbor, endpoints far apart
	embA := []float64{1, 0, 0}
	embB := []float64{0.8, 0.6, 0}
	embC := []float64{0.5, 0.87, 0}
	embD := []float64{0.2, 0.98, 0}
	embE := []float64{-0.1, 0.995, 0}

	candidateA := createUpgradeLabel(t, db, "A", "a", "auxiliary", "active", 5, embA)
	candidateB := createUpgradeLabel(t, db, "B", "b", "auxiliary", "active", 5, embB)
	candidateC := createUpgradeLabel(t, db, "C", "c", "auxiliary", "active", 5, embC)
	candidateD := createUpgradeLabel(t, db, "D", "d", "auxiliary", "active", 5, embD)
	candidateE := createUpgradeLabel(t, db, "E", "e", "auxiliary", "active", 5, embE)

	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "A", RefCount: 5, Embedding: embA},
		{ID: candidateB.ID, Label: "B", RefCount: 5, Embedding: embB},
		{ID: candidateC.ID, Label: "C", RefCount: 5, Embedding: embC},
		{ID: candidateD.ID, Label: "D", RefCount: 5, Embedding: embD},
		{ID: candidateE.ID, Label: "E", RefCount: 5, Embedding: embE},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "average_link"
	config.ClusterDistanceThreshold = 0.20

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)

	maxSize := 0
	for _, c := range clusters {
		if len(c.Candidates) > maxSize {
			maxSize = len(c.Candidates)
		}
	}
	require.Less(t, maxSize, 5, "no single cluster should contain all 5 candidates")

	totalCandidates := 0
	for _, c := range clusters {
		totalCandidates += len(c.Candidates)
	}
	require.Equal(t, 5, totalCandidates)
}

func TestClusterCandidatesCentroidFallback(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "OpenAI", RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: "GPT", RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: "Battery", RefCount: 5, Embedding: []float64{0, 1, 0}},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "centroid"

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)
	// Centroid method should still produce the same results
	require.Len(t, clusters, 2)
}
```

**Step 2: 修改现有测试 — 注入 ClusterMethod**

现有测试调用 `service.LoadUpgradeConfig(context.Background())`，默认值现在是 `"average_link"`。检查以下测试是否仍通过：

- `TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards` — 期望 2 簇 {A,B}+{C}，average-link 也会产生相同结果
- `TestClusterCandidatesBoardAffinities` — 不依赖聚类算法，只测 affinity
- `TestClusterCandidatesPass2Reassignment` — 用 `centroid` 模式（th=0.25），需要设 `config.ClusterMethod = "centroid"`
- `TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster` — 同上，需设 `config.ClusterMethod = "centroid"`

在 Pass2 两个测试中添加 `config.ClusterMethod = "centroid"` 行。

**Step 3: 运行测试**

```bash
cd /mnt/d/project/Syntopica/backend-go && go test ./internal/domain/tagging/ -v -run "TestCluster|TestSemanticBoardUpgrade(Clusters|LoadsCoTag|Generate|Prompt|Confirm)"
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade_test.go
git commit -m "test(upgrade): add average-link clustering tests, keep centroid fallback"
```

---

## Task 6: 文档更新 (6.7)

**Files:**
- Modify: `docs/reference/api/semantic-boards.md:220` (config 示例)
- Modify: `docs/reference/architecture/backend.md:247` (升级说明)
- Modify: `docs/reference/database/DATABASE_FIELDS.md` (changelog)

**Step 1: 更新 `docs/reference/api/semantic-boards.md`**

在 config 示例中添加 `cluster_method`：

```json
"config": {
    "semantic_board_upgrade_ref_count_threshold": 5,
    "semantic_board_upgrade_cluster_distance_threshold": 0.35,
    "semantic_board_upgrade_cluster_method": "average_link",
    ...
}
```

**Step 2: 更新 `docs/reference/architecture/backend.md`**

将当前描述：
```
2. embedding 预聚类（cosine 距离 < 0.7）
```
改为：
```
2. embedding 预聚类（average-link greedy，默认 cosine 距离阈值 0.35；可通过 cluster_method 切换回 centroid 模式）
```

将：
```
4. LLM 判断：create_new / merge_into_existing / skip
```
改为：
```
4. LLM 判断：create_new / skip
```

**Step 3: 更新 `docs/reference/database/DATABASE_FIELDS.md`**

在 changelog 中添加：

```markdown
### 2026-05-30

- ai_settings 新增 `semantic_board_upgrade_cluster_method` 配置（average_link / centroid）
```

**Step 4: Commit**

```bash
git add docs/reference/api/semantic-boards.md docs/reference/architecture/backend.md docs/reference/database/DATABASE_FIELDS.md
git commit -m "docs: update clustering algorithm references to average-link greedy"
```

---

## Task 依赖关系

```
Task 1 (config字段) ──→ Task 2 (实现聚类) ──→ Task 5 (测试)
                  ──→ Task 3 (数据库)  ──┘
                  ──→ Task 4 (前端)    (独立)
                  ──→ Task 6 (文档)    (独立)
```

- Task 1 必须先完成（Task 2 依赖结构体字段）
- Task 2 必须在 Task 5 之前（测试需要新函数）
- Task 3、4、6 可与 Task 2 并行（但建议先完成 Task 1）
- Task 3 可在 Task 2 之前的任意时间执行
