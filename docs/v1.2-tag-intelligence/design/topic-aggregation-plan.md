# 标签语义聚合实现方案

## 目标

在现有逐标签实时解析基础上，新增每日批量聚合层，将语义相近的标签（如"伊朗战事"、"以色列袭击伊朗"）聚类、AI验证、软合并为规范标签，并赋予权重，使规范标签在后续匹配中更不易被新标签绕过。

## 设计决策

| 决策项 | 选择 |
|--------|------|
| 运行时机 | 每日定时任务（02:00 CST）+ 手动触发 |
| 合并策略 | 软合并：保留原标签记录，引用迁移到规范标签，原名挂为别名 |
| 规范标签选择 | AI优先从已有标签中选择概括性最强的（不创造新标签） |
| 权重机制 | 加权匹配：合并过的规范标签在 embedding 匹配时获得相似度加成 |
| 聚类阈值 | T_cluster = 0.72 |
| 前端展示 | 嵌入现有定时任务 tab |

---

## Phase 1: 数据模型 + 迁移

### 1.1 新增模型 `backend-go/internal/domain/models/tag_merge_record.go`

```go
package models

import "time"

type TagMergeRecord struct {
    ID             uint      `gorm:"primaryKey" json:"id"`
    CanonicalTagID uint      `gorm:"index;not null" json:"canonical_tag_id"`
    MergedTagID    uint      `gorm:"index;not null" json:"merged_tag_id"`
    MergedLabel    string    `gorm:"size:160;not null" json:"merged_label"`
    MergedCategory string    `gorm:"size:20;not null" json:"merged_category"`
    Similarity     float64   `json:"similarity"`
    ClusterID      string    `gorm:"size:40;index" json:"cluster_id"` // 批次标识，如 "2026-03-30-02"
    MergedAt       time.Time `json:"merged_at"`

    CanonicalTag *TopicTag `gorm:"foreignKey:CanonicalTagID" json:"canonical_tag,omitempty"`
    MergedTag    *TopicTag `gorm:"foreignKey:MergedTagID" json:"merged_tag,omitempty"`
}

func (TagMergeRecord) TableName() string {
    return "tag_merge_records"
}
```

### 1.2 修改 `backend-go/internal/domain/models/topic_graph.go`

在 `TopicTag` struct 中新增字段：

```go
// 在 IsCanonical 字段之后添加
Weight float64 `gorm:"default:1.0" json:"weight"` // 规范权重，随合并递增，初始 1.0
```

### 1.3 迁移

在 `backend-go/cmd/server/main.go` 的 AutoMigrate 列表中追加：

```go
&models.TagMergeRecord{},
```

TopicTag 的 `Weight` 字段通过 GORM AutoMigrate 自动添加（GORM 会自动添加新列，默认值 1.0 适配已有行）。

---

## Phase 2: 聚合领域服务

### 2.1 新建 `backend-go/internal/domain/topicaggregation/` 包

#### 文件结构

```
backend-go/internal/domain/topicaggregation/
├── aggregation_service.go    # 主服务：编排整个聚合流程
├── cluster_engine.go         # 聚类算法：pairwise cosine similarity + Union-Find
├── ai_canonicalizer.go       # AI规范标签选择：验证聚类、选出概括标签
├── merge_engine.go           # 标签合并：引用迁移、别名挂载、权重更新
├── scheduler.go              # 定时调度器（实现 GetStatus/Trigger 接口）
├── handler.go                # HTTP API handler
└── aggregation_service_test.go
```

### 2.2 `cluster_engine.go` — 聚类引擎

**职责**：输入同 category 的 TopicTag（带 embedding），输出聚类分组。

**算法**：
1. 从 DB 加载过去 N 天（默认7天）内所有 `TopicTag`，按 category 分组
2. 对没有 embedding 的标签，调用 `EmbeddingService.GenerateEmbedding()` 生成并存储
3. 在每个 category 内计算 N×N pairwise cosine similarity
4. 以 `T_cluster = 0.72` 为阈值，用 Union-Find 归并：若 `sim(A,B) >= T_cluster`，归入同簇
5. 过滤：仅保留 `size >= 2` 的簇
6. 对每个簇内标签按 `(feed_count DESC, article_count DESC)` 排序

**核心结构**：

```go
package topicaggregation

import (
    "context"
    "math"
    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/domain/topicanalysis"
    "my-robot-backend/internal/platform/airouter"
    "my-robot-backend/internal/platform/database"
)

const defaultClusterThreshold = 0.72

type TagCluster struct {
    ID       string
    Category string
    Tags     []ClusterMember
}

type ClusterMember struct {
    Tag          *models.TopicTag
    Similarity   float64  // 与簇内最高权重标签的相似度
    ArticleCount int
}

type ClusterEngine struct {
    embeddingService *topicanalysis.EmbeddingService
    threshold        float64
    lookbackDays     int
}

func NewClusterEngine() *ClusterEngine {
    return &ClusterEngine{
        embeddingService: topicanalysis.NewEmbeddingService(),
        threshold:        defaultClusterThreshold,
        lookbackDays:     7,
    }
}

// Cluster 返回所有聚类结果
func (ce *ClusterEngine) Cluster(ctx context.Context) ([]TagCluster, error) {
    // 1. 加载近 N 天内有引用的标签
    tags := ce.loadActiveTags()
    if len(tags) < 2 {
        return nil, nil
    }

    // 2. 确保所有标签都有 embedding
    ce.ensureEmbeddings(ctx, tags)

    // 3. 按 category 分组
    byCategory := ce.groupByCategory(tags)

    // 4. 对每个 category 聚类
    var clusters []TagCluster
    for category, categoryTags := range byCategory {
        catClusters := ce.clusterCategory(categoryTags, category)
        clusters = append(clusters, catClusters...)
    }

    return clusters, nil
}

// loadActiveTags 加载近 lookbackDays 内有 article_topic_tags 引用的标签
func (ce *ClusterEngine) loadActiveTags() []*models.TopicTag {
    // SQL 思路：
    // SELECT DISTINCT tt.* FROM topic_tags tt
    //   JOIN article_topic_tags att ON att.topic_tag_id = tt.id
    //   JOIN articles a ON a.id = att.article_id
    //   WHERE a.created_at >= date('now', '-7 days')
    //   AND tt.is_canonical = true
    // 实现用 GORM
}

// ensureEmbeddings 确保所有标签都有 embedding，没有的生成
func (ce *ClusterEngine) ensureEmbeddings(ctx context.Context, tags []*models.TopicTag) {
    for _, tag := range tags {
        var existing models.TopicTagEmbedding
        err := database.DB.Where("topic_tag_id = ?", tag.ID).First(&existing).Error
        if err != nil {
            emb, err := ce.embeddingService.GenerateEmbedding(ctx, tag)
            if err == nil {
                tag.Embedding = emb
                ce.embeddingService.SaveEmbedding(emb)
            }
        } else {
            tag.Embedding = &existing
        }
    }
}

// clusterCategory 对同 category 内的标签聚类
func (ce *ClusterEngine) clusterCategory(tags []*models.TopicTag, category string) []TagCluster {
    // 1. 提取向量
    // 2. N×N pairwise cosine similarity
    // 3. Union-Find 聚类
    // 4. 过滤 size < 2 的簇
    // 5. 排序并返回
}
```

**Union-Find 实现**（简洁内联版）：

```go
type unionFind struct {
    parent []int
    rank   []int
}

func newUnionFind(n int) *unionFind {
    uf := &unionFind{parent: make([]int, n), rank: make([]int, n)}
    for i := range uf.parent {
        uf.parent[i] = i
    }
    return uf
}

func (uf *unionFind) find(x int) int {
    if uf.parent[x] != x {
        uf.parent[x] = uf.find(uf.parent[x])
    }
    return uf.parent[x]
}

func (uf *unionFind) union(x, y int) {
    px, py := uf.find(x), uf.find(y)
    if px == py { return }
    if uf.rank[px] < uf.rank[py] { px, py = py, px }
    uf.parent[py] = px
    if uf.rank[px] == uf.rank[py] { uf.rank[px]++ }
}
```

### 2.3 `ai_canonicalizer.go` — AI 规范标签选择

**职责**：对每个聚类簇，调用 AI 验证聚类合理性，并从已有标签中选出概括性最强的作为规范标签。

```go
package topicaggregation

import (
    "context"
    "encoding/json"
    "fmt"
    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/domain/topicanalysis"
    "my-robot-backend/internal/platform/airouter"
)

type CanonicalizationResult struct {
    ClusterID       string
    IsValid         bool     // AI 确认聚类合理
    CanonicalTagID  uint     // 选中的规范标签 ID
    CanonicalLabel  string   // 规范标签名
    Aliases         []string // 非规范标签名
    Reason          string   // AI 决策理由
}

type AICanonicalizer struct {
    router *airouter.Router
}

func NewAICanonicalizer() *AICanonicalizer {
    return &AICanonicalizer{router: airouter.NewRouter()}
}

// CanonicalizeCluster 对单个聚类簇进行 AI 规范化
func (ac *AICanonicalizer) CanonicalizeCluster(ctx context.Context, cluster TagCluster) (*CanonicalizationResult, error) {
    // 1. 构建 prompt
    systemPrompt := ac.buildCanonicalizationPrompt()
    userPrompt := ac.buildClusterContext(cluster)

    // 2. 调用 AI
    maxTokens := 300
    temperature := 0.15
    result, err := ac.router.Chat(ctx, airouter.ChatRequest{
        Capability: airouter.CapabilityTopicTagging,
        Messages: []airouter.Message{
            {Role: "system", Content: systemPrompt},
            {Role: "user", Content: userPrompt},
        },
        Temperature: &temperature,
        MaxTokens:   &maxTokens,
    })
    if err != nil {
        return nil, fmt.Errorf("AI canonicalization failed: %w", err)
    }

    // 3. 解析响应
    return ac.parseCanonicalizationResponse(result.Content, cluster)
}

func (ac *AICanonicalizer) buildCanonicalizationPrompt() string {
    return `你是一个标签规范化助手。你将收到一组语义相近的标签聚类，需要从中选择最合适的一个标签作为规范标签（canonical tag）。

选择标准（按优先级排列）：
1. 概括性最强：能涵盖聚类中其他标签的含义
2. 简洁中性：避免过于具体或偏向某一方面
3. 使用频率高：已有更多引用的标签优先
4. 必须从已有标签中选择（不要创造新标签名）

判断规则：
- 如果聚类中的标签确实指向同一主题/事件/概念，设置 is_valid_cluster = true
- 如果标签含义有明显差异（不应合并），设置 is_valid_cluster = false
- 有任何不确定时，宁可不合并（is_valid_cluster = false）

返回严格 JSON 格式：
{"is_valid_cluster": true, "canonical_tag_id": 42, "reason": "简短理由"}`
}

func (ac *AICanonicalizer) buildClusterContext(cluster TagCluster) string {
    // 格式化为：
    // 类别: event
    // 标签列表:
    // - ID 42: "伊朗战事" (文章数: 15, 订阅源数: 5)
    // - ID 78: "以色列袭击伊朗" (文章数: 8, 订阅源数: 3)
}

func (ac *AICanonicalizer) parseCanonicalizationResponse(content string, cluster TagCluster) (*CanonicalizationResult, error) {
    // 解析 JSON，验证 canonical_tag_id 存在于 cluster 中
}
```

### 2.4 `merge_engine.go` — 合并引擎

**职责**：执行标签软合并——引用迁移、别名挂载、权重更新。

```go
package topicaggregation

import (
    "encoding/json"
    "fmt"
    "log"
    "time"

    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/platform/database"
)

type MergeEngine struct{}

func NewMergeEngine() *MergeEngine { return &MergeEngine{} }

type MergePlan struct {
    CanonicalTag *models.TopicTag
    MergedTags   []*models.TopicTag
    ClusterID    string
    Aliases      []string
}

type MergeResult struct {
    ClusterID          string
    CanonicalTagID     uint
    CanonicalLabel     string
    MergedCount        int
    MigratedReferences int
    AliasesAdded       []string
}

// ExecuteMerge 执行单个聚类簇的合并
func (me *MergeEngine) ExecuteMerge(plan *MergePlan) (*MergeResult, error) {
    tx := database.DB.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    canonical := plan.CanonicalTag

    // 1. 收集新别名
    var newAliases []string
    existingAliases := parseAliasesFromJSON(canonical.Aliases)
    aliasSet := make(map[string]bool)
    for _, a := range existingAliases {
        aliasSet[a] = true
    }
    for _, merged := range plan.MergedTags {
        if !aliasSet[merged.Label] {
            newAliases = append(newAliases, merged.Label)
            aliasSet[merged.Label] = true
        }
    }

    // 2. 更新规范标签的 aliases + weight
    allAliases := append(existingAliases, newAliases...)
    aliasesJSON, _ := json.Marshal(allAliases)
    newWeight := canonical.Weight + float64(len(plan.MergedTags))*0.1

    if err := tx.Model(canonical).Updates(map[string]interface{}{
        "aliases": string(aliasesJSON),
        "weight":  newWeight,
    }).Error; err != nil {
        tx.Rollback()
        return nil, fmt.Errorf("failed to update canonical tag: %w", err)
    }

    // 3. 标记被合并标签为非规范
    mergedIDs := make([]uint, len(plan.MergedTags))
    for i, merged := range plan.MergedTags {
        mergedIDs[i] = merged.ID
        if err := tx.Model(merged).Updates(map[string]interface{}{
            "is_canonical": false,
        }).Error; err != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to mark tag %d as non-canonical: %w", merged.ID, err)
        }
    }

    // 4. 迁移 ai_summary_topics 引用
    migratedSummaries := 0
    for _, mergedID := range mergedIDs {
        // 先删除已有指向 canonical 的重复记录
        if err := tx.Exec(`
            DELETE FROM ai_summary_topics
            WHERE topic_tag_id = ?
            AND summary_id IN (
                SELECT summary_id FROM ai_summary_topics WHERE topic_tag_id = ?
            )`, canonical.ID, mergedID).Error; err != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to deduplicate summary references: %w", err)
        }

        // 迁移引用
        result := tx.Exec(`
            UPDATE ai_summary_topics
            SET topic_tag_id = ?
            WHERE topic_tag_id = ?`, canonical.ID, mergedID)
        if result.Error != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to migrate summary references: %w", result.Error)
        }
        migratedSummaries += int(result.RowsAffected)
    }

    // 5. 迁移 article_topic_tags 引用（同样的去重+迁移逻辑）
    migratedArticles := 0
    for _, mergedID := range mergedIDs {
        if err := tx.Exec(`
            DELETE FROM article_topic_tags
            WHERE topic_tag_id = ?
            AND article_id IN (
                SELECT article_id FROM article_topic_tags WHERE topic_tag_id = ?
            )`, canonical.ID, mergedID).Error; err != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to deduplicate article references: %w", err)
        }

        result := tx.Exec(`
            UPDATE article_topic_tags
            SET topic_tag_id = ?
            WHERE topic_tag_id = ?`, canonical.ID, mergedID)
        if result.Error != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to migrate article references: %w", result.Error)
        }
        migratedArticles += int(result.RowsAffected)
    }

    // 6. 更新 canonical 的 feed_count
    me.recountFeedCount(tx, canonical.ID)

    // 7. 记录 TagMergeRecord
    for _, merged := range plan.MergedTags {
        similarity := 0.0 // 从 cluster 信息中获取
        record := models.TagMergeRecord{
            CanonicalTagID: canonical.ID,
            MergedTagID:    merged.ID,
            MergedLabel:    merged.Label,
            MergedCategory: merged.Category,
            Similarity:     similarity,
            ClusterID:      plan.ClusterID,
            MergedAt:       time.Now(),
        }
        if err := tx.Create(&record).Error; err != nil {
            log.Printf("[WARN] Failed to create merge record for tag %d: %v", merged.ID, err)
        }
    }

    if err := tx.Commit().Error; err != nil {
        return nil, fmt.Errorf("failed to commit merge: %w", err)
    }

    return &MergeResult{
        ClusterID:          plan.ClusterID,
        CanonicalTagID:     canonical.ID,
        CanonicalLabel:     canonical.Label,
        MergedCount:        len(plan.MergedTags),
        MigratedReferences: migratedSummaries + migratedArticles,
        AliasesAdded:       newAliases,
    }, nil
}

func (me *MergeEngine) recountFeedCount(tx *gorm.DB, tagID uint) {
    // 重新统计指向此标签的文章所涉及的 distinct feed 数量
    // UPDATE topic_tags SET feed_count = (SELECT COUNT(DISTINCT feeds.id) ...)
    //   FROM article_topic_tags JOIN articles JOIN feeds WHERE tag_id = tagID
}

func parseAliasesFromJSON(aliases string) []string {
    // 复用 topicextraction 中的同名函数逻辑
}
```

### 2.5 `aggregation_service.go` — 主服务

**编排整个流程**：

```go
package topicaggregation

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/platform/database"
)

type AggregationResult struct {
    StartedAt        time.Time
    FinishedAt       time.Time
    TotalClusters    int
    ValidClusters    int
    MergedClusters   int
    TotalMerged      int
    TotalMigrated    int
    ClusterDetails   []ClusterDetail
    Errors           []string
}

type ClusterDetail struct {
    ClusterID      string
    Tags           []string
    CanonicalTag   string
    IsValid        bool
    MergedCount    int
    MigratedRefs   int
}

type AggregationService struct {
    cluster       *ClusterEngine
    canonicalizer *AICanonicalizer
    merger        *MergeEngine

    mu          sync.Mutex
    isRunning   bool
    lastResult  *AggregationResult
}

var (
    globalService *AggregationService
    serviceOnce   sync.Once
)

func GetService() *AggregationService {
    serviceOnce.Do(func() {
        globalService = &AggregationService{
            cluster:       NewClusterEngine(),
            canonicalizer: NewAICanonicalizer(),
            merger:        NewMergeEngine(),
        }
    })
    return globalService
}

// RunAggregation 执行一次完整的聚合流程
func (s *AggregationService) RunAggregation(ctx context.Context) (*AggregationResult, error) {
    s.mu.Lock()
    if s.isRunning {
        s.mu.Unlock()
        return nil, fmt.Errorf("aggregation already running")
    }
    s.isRunning = true
    s.mu.Unlock()

    defer func() {
        s.mu.Lock()
        s.isRunning = false
        s.mu.Unlock()
    }()

    result := &AggregationResult{StartedAt: time.Now()}

    // Step 1: 聚类
    clusters, err := s.cluster.Cluster(ctx)
    if err != nil {
        result.Errors = append(result.Errors, fmt.Sprintf("clustering failed: %v", err))
        result.FinishedAt = time.Now()
        s.lastResult = result
        return result, err
    }
    result.TotalClusters = len(clusters)

    if len(clusters) == 0 {
        result.FinishedAt = time.Now()
        s.lastResult = result
        return result, nil
    }

    // Step 2: AI 规范化（逐簇）
    for _, cluster := range clusters {
        canonicalResult, err := s.canonicalizer.CanonicalizeCluster(ctx, cluster)
        if err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("cluster %s canonicalization failed: %v", cluster.ID, err))
            continue
        }

        detail := ClusterDetail{
            ClusterID:    cluster.ID,
            IsValid:      canonicalResult.IsValid,
            CanonicalTag: canonicalResult.CanonicalLabel,
        }
        for _, m := range cluster.Tags {
            detail.Tags = append(detail.Tags, m.Tag.Label)
        }

        if !canonicalResult.IsValid {
            result.ClusterDetails = append(result.ClusterDetails, detail)
            continue
        }

        result.ValidClusters++

        // Step 3: 构建 MergePlan 并执行合并
        plan := s.buildMergePlan(cluster, canonicalResult)
        mergeResult, err := s.merger.ExecuteMerge(plan)
        if err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("merge cluster %s failed: %v", cluster.ID, err))
            continue
        }

        detail.MergedCount = mergeResult.MergedCount
        detail.MigratedRefs = mergeResult.MigratedReferences
        result.MergedClusters++
        result.TotalMerged += mergeResult.MergedCount
        result.TotalMigrated += mergeResult.MigratedReferences

        result.ClusterDetails = append(result.ClusterDetails, detail)
    }

    // Step 4: 更新规范标签的 embedding（aliases 变了）
    s.updateCanonicalEmbeddings(ctx, result)

    result.FinishedAt = time.Now()
    s.lastResult = result

    log.Printf("[TopicAggregation] Done: %d clusters, %d merged, %d refs migrated in %v",
        result.TotalClusters, result.TotalMerged, result.TotalMigrated,
        result.FinishedAt.Sub(result.StartedAt))

    return result, nil
}

func (s *AggregationService) buildMergePlan(cluster TagCluster, canonical *CanonicalizationResult) *MergePlan {
    var canonicalTag *models.TopicTag
    var mergedTags []*models.TopicTag
    for _, m := range cluster.Tags {
        if m.Tag.ID == canonical.CanonicalTagID {
            canonicalTag = m.Tag
        } else {
            mergedTags = append(mergedTags, m.Tag)
        }
    }
    return &MergePlan{
        CanonicalTag: canonicalTag,
        MergedTags:   mergedTags,
        ClusterID:    cluster.ID,
        Aliases:      canonical.Aliases,
    }
}

func (s *AggregationService) updateCanonicalEmbeddings(ctx context.Context, result *AggregationResult) {
    // 对所有涉及的规范标签重新生成 embedding（因为 aliases 变了）
    // 用 embeddingService.GenerateEmbedding + SaveEmbedding
}

// GetStatus 返回当前聚合状态（供 scheduler GetStatus 调用）
func (s *AggregationService) GetStatus() map[string]interface{} {
    s.mu.Lock()
    defer s.mu.Unlock()

    status := map[string]interface{}{
        "running": s.isRunning,
    }

    if s.lastResult != nil {
        status["last_run"] = map[string]interface{}{
            "started_at":      s.lastResult.StartedAt.Format(time.RFC3339),
            "finished_at":     s.lastResult.FinishedAt.Format(time.RFC3339),
            "total_clusters":  s.lastResult.TotalClusters,
            "valid_clusters":  s.lastResult.ValidClusters,
            "merged_clusters": s.lastResult.MergedClusters,
            "total_merged":    s.lastResult.TotalMerged,
            "total_migrated":  s.lastResult.TotalMigrated,
            "errors":          s.lastResult.Errors,
        }
    }

    return status
}

// GetMergeHistory 返回最近的合并记录
func (s *AggregationService) GetMergeHistory(limit int) ([]models.TagMergeRecord, error) {
    var records []models.TagMergeRecord
    err := database.DB.Preload("CanonicalTag").Preload("MergedTag").
        Order("merged_at DESC").
        Limit(limit).
        Find(&records).Error
    return records, err
}

// PreviewClusters 仅运行聚类，不执行合并（dry run）
func (s *AggregationService) PreviewClusters(ctx context.Context) ([]TagCluster, error) {
    return s.cluster.Cluster(ctx)
}
```

---

## Phase 3: 定时调度器

### 3.1 `scheduler.go` — 聚合调度器

遵循项目现有调度器模式（参考 `digest/scheduler.go`），实现 `GetStatus` 和 `TriggerNow` 接口。

```go
package topicaggregation

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/robfig/cron/v3"
)

type AggregationScheduler struct {
    cron       *cron.Cron
    isRunning  bool
    mu         sync.Mutex
    service    *AggregationService
}

var globalScheduler *AggregationScheduler
var schedulerOnce sync.Once

func GetAggregationScheduler() *AggregationScheduler {
    schedulerOnce.Do(func() {
        globalScheduler = &AggregationScheduler{
            cron:    cron.New(),
            service: GetService(),
        }
    })
    return globalScheduler
}

func (s *AggregationScheduler) Start() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.isRunning {
        return nil
    }

    // 每日 02:00 CST 执行
    if _, err := s.cron.AddFunc("0 2 * * *", s.runAggregation); err != nil {
        return fmt.Errorf("failed to schedule topic aggregation: %w", err)
    }

    s.cron.Start()
    s.isRunning = true
    log.Println("Topic aggregation scheduler started (daily at 02:00)")
    return nil
}

func (s *AggregationScheduler) Stop() {
    s.mu.Lock()
    defer s.mu.Unlock()

    if !s.isRunning {
        return
    }
    ctx := s.cron.Stop()
    <-ctx.Done()
    s.isRunning = false
    log.Println("Topic aggregation scheduler stopped")
}

func (s *AggregationScheduler) runAggregation() {
    log.Println("Starting daily topic aggregation...")
    result, err := s.service.RunAggregation(context.Background())
    if err != nil {
        log.Printf("Topic aggregation failed: %v", err)
        return
    }
    log.Printf("Topic aggregation completed: %d clusters, %d merged", result.TotalClusters, result.TotalMerged)
}

// TriggerNow 手动触发（供 API 调用）
func (s *AggregationScheduler) TriggerNow() map[string]interface{} {
    s.mu.Lock()
    if s.service.isRunning {
        s.mu.Unlock()
        return map[string]interface{}{
            "accepted": false,
            "message":  "聚合任务正在执行中，请稍后再试",
        }
    }
    s.mu.Unlock()

    go s.runAggregation()

    return map[string]interface{}{
        "accepted": true,
        "started":  true,
        "message":  "标签聚合任务已触发",
    }
}

// GetStatus 返回调度器状态（遵循 jobs 包的约定）
func (s *AggregationScheduler) GetStatus() map[string]interface{} {
    status := s.service.GetStatus()
    status["running"] = s.isRunning

    entries := s.cron.Entries()
    if len(entries) > 0 {
        status["next_run"] = entries[0].Next.Format(time.RFC3339)
    }

    return status
}
```

### 3.2 注册到 Runtime

**修改 `backend-go/internal/app/runtime.go`**：

```go
// 在 import 中添加
"my-robot-backend/internal/domain/topicaggregation"

// 在 Runtime struct 中添加
Aggregation *topicaggregation.AggregationScheduler

// 在 StartRuntime() 中添加（在 Digest scheduler 之后）
runtime.Aggregation = topicaggregation.GetAggregationScheduler()
if err := runtime.Aggregation.Start(); err != nil {
    log.Printf("Warning: Failed to start topic aggregation scheduler: %v", err)
} else {
    log.Println("Topic aggregation scheduler started successfully")
}

// 在 runtimeinfo 中注册
runtimeinfo.AggregationSchedulerInterface = runtime.Aggregation
```

**修改 `backend-go/internal/app/runtimeinfo/schedulers.go`**：

```go
var AggregationSchedulerInterface interface{}
```

### 3.3 注册到 scheduler descriptors

**修改 `backend-go/internal/jobs/handler.go`**：

在 `schedulerDescriptors()` 中添加：

```go
{
    Name:        "topic_aggregation",
    Description: "Aggregate semantically similar tags daily",
    Get: func() interface{} {
        return runtimeinfo.AggregationSchedulerInterface
    },
},
```

### 3.4 在 graceful shutdown 中添加

**修改 `backend-go/internal/app/runtime.go` 的 `SetupGracefulShutdown`**：

在 `runtime.Digest.Stop()` 之后添加：

```go
if runtime.Aggregation != nil {
    log.Println("Stopping topic aggregation scheduler...")
    runtime.Aggregation.Stop()
}
```

---

## Phase 4: HTTP API

### 4.1 `handler.go` — 聚合 API

```go
package topicaggregation

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
)

// RegisterAggregationRoutes 注册聚合相关路由
func RegisterAggregationRoutes(r *gin.RouterGroup) {
    agg := r.Group("/topic-aggregation")
    {
        agg.GET("/status", GetAggregationStatus)
        agg.POST("/trigger", TriggerAggregation)
        agg.GET("/history", GetMergeHistory)
        agg.GET("/preview-clusters", PreviewClusters)
    }
}

func GetAggregationStatus(c *gin.Context) {
    service := GetService()
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    service.GetStatus(),
    })
}

func TriggerAggregation(c *gin.Context) {
    scheduler := GetAggregationScheduler()
    result := scheduler.TriggerNow()

    accepted, _ := result["accepted"].(bool)
    message, _ := result["message"].(string)

    if accepted {
        c.JSON(http.StatusOK, gin.H{"success": true, "message": message, "data": result})
    } else {
        c.JSON(http.StatusConflict, gin.H{"success": false, "error": message, "data": result})
    }
}

func GetMergeHistory(c *gin.Context) {
    limit := 50
    if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
        limit = l
    }

    service := GetService()
    records, err := service.GetMergeHistory(limit)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true, "data": records})
}

func PreviewClusters(c *gin.Context) {
    service := GetService()
    clusters, err := service.PreviewClusters(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true, "data": clusters})
}
```

### 4.2 注册路由

**修改 `backend-go/internal/app/router.go`**：

在 `api.Group` 内，topic graph 路由之后添加：

```go
topicaggregationdomain.RegisterAggregationRoutes(api)
```

在文件头 import 中添加：

```go
topicaggregationdomain "my-robot-backend/internal/domain/topicaggregation"
```

---

## Phase 5: 权重匹配（修改现有代码）

### 5.1 修改 `backend-go/internal/domain/topicextraction/extractor_enhanced.go`

在 `resolveCandidate` 方法中，embedding 匹配结果应用权重加成。

**修改位置**：`resolveCandidate` 函数中，`FindSimilarTags` 返回结果后的 `switch` 语句内。

在 `case "high_similarity"` 和 `case "ai_judgment"` 分支中，对 similarity 应用权重：

```go
// 在 high_similarity 分支中
effectiveSimilarity := matchResult.Similarity
if matchResult.ExistingTag.Weight > 1.0 {
    effectiveSimilarity *= (1.0 + (matchResult.ExistingTag.Weight-1.0)*0.15)
}

return &topictypes.TopicTag{
    // ...
    Score:     candidate.Confidence * effectiveSimilarity,
    // ...
}, false, nil
```

同样在 `ai_judgment` 分支中，将候选标签的 similarity 传入 AI 判断时也考虑权重。

### 5.2 修改 `backend-go/internal/domain/topicanalysis/embedding.go`

在 `TagMatch` 方法中，应用阈值判断时考虑权重：

```go
// 在 Step 4: Apply thresholds 处
adjustedHighThreshold := s.thresholds.HighSimilarity
adjustedLowThreshold := s.thresholds.LowSimilarity

if best.Tag.Weight > 1.0 {
    // 权重每增加 0.1，高阈值降低 0.005，低阈值降低 0.01
    // 这意味着高权重标签更容易被复用
    weightBonus := (best.Tag.Weight - 1.0) * 0.05
    adjustedHighThreshold -= weightBonus
    adjustedLowThreshold -= weightBonus * 2
}

if best.Similarity >= adjustedHighThreshold {
    // High similarity - auto-reuse
}
if best.Similarity < adjustedLowThreshold {
    // Low similarity - auto-create
}
```

---

## Phase 6: 前端集成

### 6.1 更新 `front/app/utils/schedulerMeta.ts`

在 `getSchedulerDisplayName` 中添加：

```typescript
'topic_aggregation': '标签聚合',
```

在 `getSchedulerIcon` 中添加：

```typescript
'topic_aggregation': 'mdi:tag-multiple',
```

在 `getSchedulerColor` 中添加：

```typescript
'topic_aggregation': 'from-violet-500 to-purple-500',
```

在 `isHotScheduler` 中添加：

```typescript
|| name === 'topic_aggregation'
```

### 6.2 更新 `front/app/components/dialog/GlobalSettingsDialog.vue`

在定时任务 tab 的 scheduler 卡片中，新增聚合运行结果的展示区块。

在现有的 `<div v-if="scheduler.name === 'auto_refresh' ...">` 类似的区块之后，添加：

```html
<!-- Topic Aggregation Run Summary -->
<div v-if="scheduler.name === 'topic_aggregation' && scheduler.last_run"
     class="mt-3 space-y-2 text-sm">
  <div class="grid grid-cols-3 gap-3">
    <div class="text-center">
      <div class="text-lg font-semibold text-purple-600">
        {{ scheduler.last_run.total_clusters || 0 }}
      </div>
      <div class="text-gray-500">聚类数</div>
    </div>
    <div class="text-center">
      <div class="text-lg font-semibold text-violet-600">
        {{ scheduler.last_run.merged_clusters || 0 }}
      </div>
      <div class="text-gray-500">已合并</div>
    </div>
    <div class="text-center">
      <div class="text-lg font-semibold text-fuchsia-600">
        {{ scheduler.last_run.total_merged || 0 }}
      </div>
      <div class="text-gray-500">标签数</div>
    </div>
  </div>
  <div v-if="scheduler.last_run.errors && scheduler.last_run.errors.length > 0"
       class="text-xs text-amber-600 bg-amber-50 rounded px-2 py-1">
    {{ scheduler.last_run.errors.length }} 个聚类处理出错
  </div>
</div>
```

### 6.3 新增类型定义（可选，复用现有 `SchedulerStatus`）

由于聚合调度器通过 `GetStatus()` 返回的数据会被 `schedulerDescriptors` 的 `safeGetStatus` 包装，它已经兼容 `SchedulerStatus` 类型。额外的 `last_run` 信息会作为额外字段附加在 status 对象上。

如果需要类型安全，可在 `front/app/types/scheduler.ts` 中添加：

```typescript
export interface AggregationRunSummary {
  started_at: string
  finished_at: string
  total_clusters: number
  valid_clusters: number
  merged_clusters: number
  total_merged: number
  total_migrated: number
  errors: string[]
}
```

然后在 `SchedulerStatus` 中添加可选字段：

```typescript
last_run?: AggregationRunSummary
```

---

## 实施顺序

| 步骤 | 内容 | 涉及文件 |
|------|------|----------|
| 1 | 数据模型 + 迁移 | `models/tag_merge_record.go`(新建), `models/topic_graph.go`(改) |
| 2 | 聚类引擎 | `topicaggregation/cluster_engine.go`(新建) |
| 3 | AI 规范化 | `topicaggregation/ai_canonicalizer.go`(新建) |
| 4 | 合并引擎 | `topicaggregation/merge_engine.go`(新建) |
| 5 | 主服务 | `topicaggregation/aggregation_service.go`(新建) |
| 6 | 调度器 | `topicaggregation/scheduler.go`(新建) |
| 7 | HTTP API | `topicaggregation/handler.go`(新建) |
| 8 | 注册路由 | `router.go`(改) |
| 9 | 注册 Runtime | `runtime.go`(改), `runtimeinfo/schedulers.go`(改) |
| 10 | 注册 Scheduler Descriptor | `jobs/handler.go`(改) |
| 11 | 权重匹配 | `extractor_enhanced.go`(改), `embedding.go`(改) |
| 12 | 前端 schedulerMeta | `schedulerMeta.ts`(改) |
| 13 | 前端调度器 UI | `GlobalSettingsDialog.vue`(改) |

---

## 测试策略

### 后端单元测试

1. **`cluster_engine_test.go`**：测试 pairwise similarity 计算、Union-Find 聚类、阈值过滤
2. **`ai_canonicalizer_test.go`**：mock AI router，测试 prompt 构建、响应解析
3. **`merge_engine_test.go`**：用 SQLite in-memory 测试引用迁移、去重、权重更新
4. **`aggregation_service_test.go`**：集成测试，mock 依赖，验证端到端流程

### 验证命令

```bash
cd backend-go
go test ./internal/domain/topicaggregation/... -v
go test ./internal/domain/topicextraction/... -v
go test ./internal/jobs/... -v
go build ./...
```

```bash
cd front
pnpm exec nuxi typecheck
```

---

## 风险和注意事项

1. **Embedding API 调用量**：首次运行可能需要为大量标签生成 embedding，注意 API 配额和速率限制
2. **事务安全**：合并引擎使用事务，确保引用迁移原子性
3. **并发安全**：聚合运行时通过 mutex 保护，防止重复触发
4. **向后兼容**：`Weight` 字段默认 1.0，不影响现有匹配逻辑；只有合并后才产生加成效果
5. **合并不可逆**：虽然软合并保留记录，但引用已迁移。建议先通过 `preview-clusters` API 验证聚类质量
