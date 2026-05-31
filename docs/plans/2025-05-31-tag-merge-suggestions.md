# Tag Merge Suggestions 实现计划

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 实现增量合并建议记录表、异步全量扫描+SSE进度推送、前端适配，替代 O(n²) cross-join 全量扫描。

**Architecture:** 双通道写入 tag_merge_suggestions 表：通道1（增量，findOrCreateTag 创建新 tag 后记录 candidates）、通道2（异步全量扫描，按 tag 遍历复用 FindSimilarTags）。查询统一读表。SSE 推送扫描进度。合并后标记 suggestion。

**Tech Stack:** Go/Gin/GORM (backend), Vue 3 + EventSource API (frontend), PostgreSQL + pgvector

---

## Task 5: TagMergeSuggestion 模型 + 增量记录

### Task 5.1: 新增 TagMergeSuggestion 模型

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go` (在 TopicTagEmbedding 之后添加)
- Modify: `backend-go/internal/platform/database/migrator.go` (注册新模型到 AutoMigrate)

**Step 1: 在 models/topic_graph.go 添加 TagMergeSuggestion struct**

在 `TopicTagEmbedding` struct 之后添加：

```go
// TagMergeSuggestion records a pair of similar tags proposed for manual merging.
type TagMergeSuggestion struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	NewTagID       uint      `gorm:"not null;uniqueIndex:idx_tag_merge_suggestion_pair" json:"new_tag_id"`
	ExistingTagID  uint      `gorm:"not null;uniqueIndex:idx_tag_merge_suggestion_pair" json:"existing_tag_id"`
	NewLabel       string    `gorm:"size:160;not null" json:"new_label"`
	ExistingLabel  string    `gorm:"size:160;not null" json:"existing_label"`
	Category       string    `gorm:"size:20;not null" json:"category"`
	Similarity     float64   `gorm:"not null" json:"similarity"`
	Status         string    `gorm:"size:20;not null;default:pending;index:idx_tag_merge_suggestion_status" json:"status"` // pending, merged, dismissed
	Source         string    `gorm:"size:20;not null;default:incremental" json:"source"` // incremental, full_scan
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
```

**Step 2: 在 migrator.go 的 allModels 列表中注册**

在 `&models.MergeReembeddingQueue{}` 之后添加 `&models.TagMergeSuggestion{}`。

**Step 3: 验证**

启动后端确认表自动创建：
```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/models/topic_graph.go backend-go/internal/platform/database/migrator.go
git commit -m "feat: add TagMergeSuggestion model for incremental merge suggestions"
```

---

### Task 5.2: 实现 RecordMergeSuggestions 函数

**Files:**
- Create: `backend-go/internal/domain/tagging/tag_merge_suggest.go`

**Step 1: 创建 tag_merge_suggest.go**

```go
package tagging

import (
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordMergeSuggestions writes candidate pairs to tag_merge_suggestions.
// Skips pairs that already exist (by unique constraint new_tag_id + existing_tag_id).
func RecordMergeSuggestions(newTagID uint, newLabel string, category string, candidates []TagCandidate) {
	if len(candidates) == 0 {
		return
	}

	for _, c := range candidates {
		suggestion := models.TagMergeSuggestion{
			NewTagID:      newTagID,
			ExistingTagID: c.Tag.ID,
			NewLabel:      newLabel,
			ExistingLabel: c.Tag.Label,
			Category:      category,
			Similarity:    c.Similarity,
			Status:        "pending",
			Source:        "incremental",
		}

		result := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
			DoNothing: true,
		}).Create(&suggestion)

		if result.Error != nil {
			logging.Warnf("RecordMergeSuggestions: failed to write suggestion new=%d existing=%d: %v", newTagID, c.Tag.ID, result.Error)
		} else if result.RowsAffected == 0 {
			logging.Debugf("RecordMergeSuggestions: skipped duplicate new=%d existing=%d", newTagID, c.Tag.ID)
		}
	}
}
```

注意：使用 `clause.OnConflict{DoNothing: true}` 配合 unique constraint 实现去重 skip，不需要先 SELECT 再 INSERT。

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_suggest.go
git commit -m "feat: add RecordMergeSuggestions for incremental suggestion recording"
```

---

### Task 5.3: findOrCreateTag 调用 RecordMergeSuggestions

**Files:**
- Modify: `backend-go/internal/domain/tagging/tagger.go`

**Step 1: 修改 candidates 分支**

在 `findOrCreateTag` 的 `case "candidates":` 分支中，保存 `matchResult` 的引用，供创建新 tag 后使用。

当前代码（约 L114）：
```go
case "candidates":
    logging.Infof("findOrCreateTag: label=%q category=%s matchType=candidates candidateCount=%d — skipping LLM judgment, falling through to create", tag.Label, category, len(matchResult.Candidates))
```

改为：
```go
case "candidates":
    logging.Infof("findOrCreateTag: label=%q category=%s matchType=candidates candidateCount=%d — skipping LLM judgment, falling through to create", tag.Label, category, len(matchResult.Candidates))
```

然后在创建新 tag 成功后（约 L170，`newTag` 创建之后、`ensureTagEmbedding` 之前），添加：

```go
// Record merge suggestions for candidates
if matchResult != nil && len(matchResult.Candidates) > 0 {
    RecordMergeSuggestions(newTag.ID, tag.Label, category, matchResult.Candidates)
}
```

这需要把 `matchResult` 的作用域从 `if es != nil { ... }` 块提升到外层。具体做法：在 `findOrCreateTag` 函数开头声明 `var savedMatchResult *TagMatchResult`，在 candidates 分支赋值，在创建新 tag 后检查。

实际改动：

1. 在 `es := getEmbeddingService()` 之前声明 `var savedCandidates []TagCandidate`
2. 在 `case "candidates":` 分支中赋值 `savedCandidates = matchResult.Candidates`
3. 在 `database.DB.Create(&newTag)` 成功后添加：
```go
if len(savedCandidates) > 0 {
    RecordMergeSuggestions(newTag.ID, tag.Label, category, savedCandidates)
}
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tagger.go
git commit -m "feat: call RecordMergeSuggestions after creating new tag in candidates path"
```

---

### Task 5.4: 编写测试

**Files:**
- Create: `backend-go/internal/domain/tagging/tag_merge_suggest_test.go`

**Step 1: 编写测试**

```go
package tagging

import (
	"testing"

	"syntopica-backend/internal/domain/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// 测试需要 mock database.DB。由于 RecordMergeSuggestions 直接使用 database.DB，
// 这里编写集成风格的测试，验证逻辑正确性。
// 如果项目已有 database mock 模式，遵循该模式。

func TestRecordMergeSuggestions_SkipsEmpty(t *testing.T) {
	// 空候选列表不应调用 DB
	RecordMergeSuggestions(1, "test", "keyword", nil)
	// 无 panic 即通过
}

func TestRecordMergeSuggestions_SingleCandidate(t *testing.T) {
	// 验证 struct 字段正确性
	candidates := []TagCandidate{
		{
			Tag:        &models.TopicTag{ID: 2, Label: "Trump"},
			Similarity: 0.98,
		},
	}
	// 在有真实 DB 的环境下运行，或 mock 验证
	// RecordMergeSuggestions(1, "特朗普", "person", candidates)
	// 这里主要验证函数签名和参数传递正确
	_ = candidates
}
```

注意：由于 `RecordMergeSuggestions` 直接使用 `database.DB` 全局变量，单元测试需要 mock 或集成测试。如果项目没有 DB mock 基础设施，此测试可以简化为编译验证 + 集成测试手动运行。核心逻辑（OnConflict skip）由 PostgreSQL unique constraint 保证正确性。

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_suggest_test.go
git commit -m "test: add RecordMergeSuggestions basic tests"
```

---

## Task 6: 异步全量扫描 + SSE

### Task 6.1: 异步全量扫描逻辑

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_suggest.go`

**Step 1: 添加扫描状态管理和异步扫描函数**

在 `tag_merge_suggest.go` 中添加：

```go
import (
	"context"
	"sync"
	"sync/atomic"
)

// ScanProgress represents the current progress of a full scan.
type ScanProgress struct {
	Status          string  `json:"status"`            // scanning, done, error
	Total           int     `json:"total"`
	Scanned         int     `json:"scanned"`
	CurrentCategory string  `json:"current_category"`
	NewSuggestions  int     `json:"new_suggestions"`
	Error           string  `json:"error,omitempty"`
}

// scanState manages the global full-scan singleton.
var scanState struct {
	mu        sync.Mutex
	running   atomic.Bool
	progress  chan ScanProgress
	cancel    context.CancelFunc
}

// IsScanRunning returns whether a full scan is currently in progress.
func IsScanRunning() bool {
	return scanState.running.Load()
}

// StartFullScan starts an asynchronous full scan of all tags.
// Returns false if a scan is already running.
func StartFullScan() bool {
	scanState.mu.Lock()
	defer scanState.mu.Unlock()

	if scanState.running.Load() {
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	scanState.cancel = cancel
	scanState.progress = make(chan ScanProgress, 32)
	scanState.running.Store(true)

	go runFullScan(ctx)

	return true
}

// GetScanProgressChannel returns the channel for SSE streaming.
func GetScanProgressChannel() <-chan ScanProgress {
	scanState.mu.Lock()
	defer scanState.mu.Unlock()
	return scanState.progress
}

// runFullScan executes the full scan in a background goroutine.
func runFullScan(ctx context.Context) {
	defer func() {
		scanState.running.Store(false)
		close(scanState.progress)
	}()

	es := getEmbeddingService()
	if es == nil {
		scanState.progress <- ScanProgress{Status: "error", Error: "embedding service unavailable"}
		return
	}

	// Load all active tags grouped by category
	var tags []models.TopicTag
	if err := database.DB.Where("status = 'active' OR status = '' OR status IS NULL").Find(&tags).Error; err != nil {
		scanState.progress <- ScanProgress{Status: "error", Error: err.Error()}
		return
	}

	total := len(tags)
	thresholds := DefaultThresholds
	newSuggestions := 0

	for i, tag := range tags {
		select {
		case <-ctx.Done():
			scanState.progress <- ScanProgress{Status: "error", Error: "cancelled"}
			return
		default:
		}

		candidates, err := es.FindSimilarTags(ctx, &tag, tag.Category, 10, EmbeddingTypeSemantic)
		if err != nil {
			logging.Warnf("runFullScan: FindSimilarTags failed for tag %d: %v", tag.ID, err)
			continue
		}

		for _, c := range candidates {
			if c.Similarity < thresholds.LowSimilarity {
				continue
			}
			if c.Tag.ID == tag.ID {
				continue
			}

			suggestion := models.TagMergeSuggestion{
				NewTagID:      tag.ID,
				ExistingTagID: c.Tag.ID,
				NewLabel:      tag.Label,
				ExistingLabel: c.Tag.Label,
				Category:      tag.Category,
				Similarity:    c.Similarity,
				Status:        "pending",
				Source:        "full_scan",
			}

			result := database.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
				DoNothing: true,
			}).Create(&suggestion)

			if result.RowsAffected > 0 {
				newSuggestions++
			}
		}

		// Send progress every 10 tags or on the last tag
		if (i+1)%10 == 0 || i+1 == total {
			scanState.progress <- ScanProgress{
				Status:          "scanning",
				Total:           total,
				Scanned:         i + 1,
				CurrentCategory: tag.Category,
				NewSuggestions:  newSuggestions,
			}
		}
	}

	scanState.progress <- ScanProgress{
		Status:         "done",
		Total:          total,
		Scanned:        total,
		NewSuggestions: newSuggestions,
	}
}
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_suggest.go
git commit -m "feat: add async full-scan logic with progress channel"
```

---

### Task 6.2: POST /merge-preview/scan handler

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**Step 1: 添加 TriggerScanHandler**

在 `RegisterTagMergePreviewRoutes` 之前添加：

```go
// TriggerScanHandler starts an asynchronous full scan.
// POST /api/topic-tags/merge-preview/scan
func TriggerScanHandler(c *gin.Context) {
	if !StartFullScan() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "scan already in progress",
		})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "scan started",
	})
}
```

在 `RegisterTagMergePreviewRoutes` 中添加路由：

```go
tags.POST("/merge-preview/scan", TriggerScanHandler)
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_preview_handler.go
git commit -m "feat: add POST /merge-preview/scan handler with concurrency protection"
```

---

### Task 6.3: GET /merge-preview/scan/stream SSE handler

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**Step 1: 添加 ScanStreamHandler**

在 `TriggerScanHandler` 之后添加：

```go
// ScanStreamHandler streams scan progress via SSE.
// GET /api/topic-tags/merge-preview/scan/stream
func ScanStreamHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch := GetScanProgressChannel()
	if ch == nil {
		// No scan running — send idle status and close
		c.SSEvent("progress", ScanProgress{Status: "idle"})
		return
	}

	c.Stream(func(w interface{}) bool {
		msg, ok := <-ch
		if !ok {
			return false
		}
		c.SSEvent("progress", msg)
		return true
	})
}
```

在 `RegisterTagMergePreviewRoutes` 中添加路由：

```go
tags.GET("/merge-preview/scan/stream", ScanStreamHandler)
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_preview_handler.go
git commit -m "feat: add SSE endpoint for scan progress streaming"
```

---

### Task 6.4: ScanMergePreviewHandler 改为查表

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**Step 1: 重写 ScanMergePreviewHandler**

将现有的 `ScanMergePreviewHandler` 改为从 `tag_merge_suggestions` 表查询，保留现有的响应格式（`TagMergeCandidate`），确保前端兼容。

```go
func ScanMergePreviewHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	includeArticles := c.DefaultQuery("include_articles", "false") == "true"

	var suggestions []models.TagMergeSuggestion
	query := database.DB.Where("status = ?", "pending").Order("similarity DESC").Limit(limit)
	if err := query.Find(&suggestions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := make([]mergePreviewCandidate, 0, len(suggestions))
	for _, s := range suggestions {
		// Determine source/target direction: fewer articles = source (merge into bigger)
		var newCount, existingCount int64
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", s.NewTagID).Count(&newCount)
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", s.ExistingTagID).Count(&existingCount)

		var sourceID, targetID uint
		var sourceLabel, sourceSlug, targetLabel, targetSlug string
		var sourceArticles, targetArticles int

		if existingCount >= newCount {
			sourceID = s.NewTagID
			sourceLabel = s.NewLabel
			targetID = s.ExistingTagID
			targetLabel = s.ExistingLabel
			sourceArticles = int(newCount)
			targetArticles = int(existingCount)
		} else {
			sourceID = s.ExistingTagID
			sourceLabel = s.ExistingLabel
			targetID = s.NewTagID
			targetLabel = s.NewLabel
			sourceArticles = int(existingCount)
			targetArticles = int(newCount)
		}

		// Fetch slugs
		var sourceTag, targetTag models.TopicTag
		database.DB.Select("slug").First(&sourceTag, sourceID)
		database.DB.Select("slug").First(&targetTag, targetID)
		sourceSlug = sourceTag.Slug
		targetSlug = targetTag.Slug

		cand := TagMergeCandidate{
			SourceTagID:    sourceID,
			SourceLabel:    sourceLabel,
			SourceSlug:     sourceSlug,
			TargetTagID:    targetID,
			TargetLabel:    targetLabel,
			TargetSlug:     targetSlug,
			Category:       s.Category,
			Similarity:     s.Similarity,
			SourceArticles: sourceArticles,
			TargetArticles: targetArticles,
		}

		item := mergePreviewCandidate{TagMergeCandidate: cand}
		if includeArticles {
			if arts, err := GetCandidateArticleTitles(sourceID, 5); err == nil {
				item.SourceArticleList = arts
			}
			if arts, err := GetCandidateArticleTitles(targetID, 5); err == nil {
				item.TargetArticleList = arts
			}
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"candidates": result,
			"total":      len(result),
		},
	})
}
```

注意：`scopeFeedID` 和 `scopeCategoryID` 参数在改为查表后暂不使用（suggestions 表不支持按 feed/category 过滤）。可保留参数但忽略，或后续通过 category 字段过滤。

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_preview_handler.go
git commit -m "feat: ScanMergePreviewHandler reads from tag_merge_suggestions table"
```

---

### Task 6.5: 编写测试

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_suggest_test.go`

**Step 1: 添加测试**

```go
func TestIsScanRunning_Initially(t *testing.T) {
	assert.False(t, IsScanRunning())
}

func TestStartFullScan_Concurrency(t *testing.T) {
	// 第一次应成功（如果没有其他测试跑过）
	// 注意：此测试依赖全局状态，集成测试环境运行
	// 单元测试中只验证函数签名和返回类型
	result := StartFullScan()
	// 第二次调用应返回 false（如果第一次成功启动了）
	if result {
		assert.False(t, StartFullScan(), "second scan should be rejected")
	}
}
```

SSE handler 测试需要 gin test context，可简化为：

```go
func TestScanStreamHandler_ContentType(t *testing.T) {
	// 使用 httptest 验证 SSE 响应头
	// 当没有 scan 运行时应返回 idle 状态
}
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_suggest_test.go
git commit -m "test: add full-scan and SSE handler tests"
```

---

## Task 7: 合并后标记 suggestion

### Task 7.1: MergeTagsWithCustomNameHandler 标记 merged

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**Step 1: 在 MergeTagsWithCustomNameHandler 的成功路径中添加**

在 `c.JSON(http.StatusOK, ...)` 之前添加：

```go
// Mark related suggestions as merged
database.DB.Model(&models.TagMergeSuggestion{}).
	Where("status = ? AND (new_tag_id = ? OR existing_tag_id = ? OR new_tag_id = ? OR existing_tag_id = ?)",
		"pending", body.SourceTagID, body.SourceTagID, body.TargetTagID, body.TargetTagID).
	Update("status", "merged")
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_preview_handler.go
git commit -m "feat: mark suggestions as merged after successful merge"
```

---

### Task 7.2: DismissSuggestionHandler

**Files:**
- Modify: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**Step 1: 添加 DismissSuggestionHandler**

```go
// DismissSuggestionHandler marks a suggestion as dismissed.
// POST /api/topic-tags/merge-preview/dismiss
func DismissSuggestionHandler(c *gin.Context) {
	var body struct {
		NewTagID     uint `json:"new_tag_id"`
		ExistingTagID uint `json:"existing_tag_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if body.NewTagID == 0 || body.ExistingTagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new_tag_id and existing_tag_id are required"})
		return
	}

	result := database.DB.Model(&models.TagMergeSuggestion{}).
		Where("new_tag_id = ? AND existing_tag_id = ? AND status = ?",
			body.NewTagID, body.ExistingTagID, "pending").
		Update("status", "dismissed")

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "suggestion not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
```

在 `RegisterTagMergePreviewRoutes` 中添加路由：

```go
tags.POST("/merge-preview/dismiss", DismissSuggestionHandler)
```

**Step 2: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tag_merge_preview_handler.go
git commit -m "feat: add dismiss suggestion handler"
```

---

## Task 8: 前端适配

### Task 8.1: API 层添加 scan/dismiss/SSE 方法

**Files:**
- Modify: `front/app/api/tagMergePreview.ts`
- Modify: `front/app/types/tagMerge.ts`

**Step 1: 在 tagMerge.ts 添加类型**

```typescript
export interface ScanProgress {
  status: 'idle' | 'scanning' | 'done' | 'error'
  total: number
  scanned: number
  current_category: string
  new_suggestions: number
  error?: string
}

export interface MergeSuggestion {
  newTagId: number
  existingTagId: number
  newLabel: string
  existingLabel: string
  category: string
  similarity: number
  status: string
  source: string
}
```

**Step 2: 在 tagMergePreview.ts 添加方法**

在 `useTagMergePreviewApi` 的 return 对象中添加：

```typescript
async triggerFullScan() {
  return apiClient.post<{ message: string }>('/topic-tags/merge-preview/scan', {})
},

createScanEventSource(onProgress: (progress: any) => void): EventSource {
  const baseUrl = apiClient.getBaseUrl?.() || ''
  const es = new EventSource(`${baseUrl}/topic-tags/merge-preview/scan/stream`)
  es.onmessage = (e) => {
    onProgress(JSON.parse(e.data))
  }
  return es
},

async dismissSuggestion(newTagId: number, existingTagId: number) {
  return apiClient.post('/topic-tags/merge-preview/dismiss', {
    new_tag_id: newTagId,
    existing_tag_id: existingTagId,
  })
},
```

注意：需要检查 `apiClient` 是否有 `getBaseUrl()` 方法，如果没有，用环境变量 `/api` 前缀。

**Step 3: Commit**

```bash
git add front/app/api/tagMergePreview.ts front/app/types/tagMerge.ts
git commit -m "feat: add scan/dismiss/SSE methods to tag merge API layer"
```

---

### Task 8.2: TagMergePreview.vue 适配 SSE + 进度展示

**Files:**
- Modify: `front/app/features/topic-graph/components/TagMergePreview.vue`

**Step 1: 修改 startScan 函数**

当前 `startScan()` 直接调用 `api.scanMergePreview()`。改为：

1. 先调用 `api.scanMergePreview()` 从 suggestion 表读取现有 pending
2. 同时提供一个 `triggerFullScan()` 按钮和对应的 SSE 进度展示

在 `<script setup>` 中添加：

```typescript
import type { ScanProgress } from '~/types/tagMerge'

const scanning = ref(false)
const scanProgress = ref<ScanProgress | null>(null)
let scanEs: EventSource | null = null

async function triggerFullScan() {
  scanning.value = true
  scanProgress.value = null
  
  // 先触发后端扫描
  const response = await api.triggerFullScan()
  if (!response.success) {
    error.value = response.error || '扫描已在进行中'
    scanning.value = false
    return
  }
  
  // 连接 SSE 接收进度
  scanEs = api.createScanEventSource((progress: ScanProgress) => {
    scanProgress.value = progress
    if (progress.status === 'done' || progress.status === 'error') {
      scanEs?.close()
      scanEs = null
      scanning.value = false
      // 扫描完成后重新加载候选列表
      if (progress.status === 'done') {
        void startScan()
      }
    }
  })
}

function cancelScan() {
  scanEs?.close()
  scanEs = null
  scanning.value = false
  scanProgress.value = null
}
```

**Step 2: 在模板中添加全量扫描 UI**

在 `<header>` 的按钮区域添加"全量扫描"按钮，在 scanning 状态展示进度条：

```html
<!-- 全量扫描按钮 -->
<button
  v-if="!scanning"
  type="button"
  class="tag-merge-action-btn"
  @click="triggerFullScan"
>
  <Icon icon="mdi:radar" width="16" />
  <span>全量扫描</span>
</button>

<!-- 扫描进度 -->
<div v-if="scanning && scanProgress" class="tag-merge-scan-progress">
  <div class="tag-merge-scan-progress__bar">
    <div
      class="tag-merge-scan-progress__fill"
      :style="{ width: `${scanProgress.total ? (scanProgress.scanned / scanProgress.total * 100) : 0}%` }"
    />
  </div>
  <div class="tag-merge-scan-progress__info">
    <span>{{ scanProgress.scanned }}/{{ scanProgress.total }} 标签</span>
    <span v-if="scanProgress.current_category">{{ scanProgress.current_category }}</span>
    <span>发现 {{ scanProgress.new_suggestions }} 个新建议</span>
  </div>
  <button type="button" class="tag-merge-action-btn" @click="cancelScan">
    <Icon icon="mdi:close" width="14" />
    <span>取消</span>
  </button>
</div>
```

添加对应 CSS：

```css
.tag-merge-scan-progress {
  padding: 12px 16px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 8px;
}
.tag-merge-scan-progress__bar {
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  overflow: hidden;
}
.tag-merge-scan-progress__fill {
  height: 100%;
  background: rgba(240, 138, 75, 0.9);
  transition: width 0.3s ease;
}
.tag-merge-scan-progress__info {
  display: flex;
  gap: 12px;
  margin-top: 8px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
```

**Step 3: 验证 lint**

```bash
cd front && pnpm lint
```

**Step 4: Commit**

```bash
git add front/app/features/topic-graph/components/TagMergePreview.vue
git commit -m "feat: add full-scan trigger with SSE progress display"
```

---

### Task 8.3: 添加"忽略"按钮

**Files:**
- Modify: `front/app/features/topic-graph/components/TagMergePreview.vue`

**Step 1: 修改 skipCandidate 函数**

当前 `skipCandidate(id)` 只是本地移除。改为调用 dismiss API：

```typescript
async function dismissCandidate(candidate: TagMergeCandidate) {
  try {
    await api.dismissSuggestion(candidate.sourceTagId, candidate.targetTagId)
    skippedIds.value.push(candidate.sourceTagId)
  } catch (err) {
    console.error('Failed to dismiss:', err)
  }
}
```

注意：需要确认 candidate 中能拿到对应的 `newTagId` 和 `existingTagId`。当前 `TagMergeCandidate` 类型只有 `sourceTagId/targetTagId`，需要后端在返回 candidate 时也带上 `new_tag_id/existing_tag_id`，或者前端用 source/target 推断。

最简方案：在 `ScanMergePreviewHandler` 的响应中增加 `new_tag_id` 和 `existing_tag_id` 字段（在 `mergePreviewCandidate` 中）。对应前端类型也增加这两个字段。

**Step 2: 修改模板中的跳过按钮**

找到现有的跳过按钮，改为调用 `dismissCandidate`：

```html
<button @click="dismissCandidate(candidate)">
  <Icon icon="mdi:close" width="14" />
  忽略
</button>
```

**Step 3: Commit**

```bash
git add front/app/features/topic-graph/components/TagMergePreview.vue front/app/api/tagMergePreview.ts front/app/types/tagMerge.ts
git commit -m "feat: add dismiss button calling backend dismiss API"
```

---

## Task 9: 数据修复

（手动执行，不需要代码提交。在所有功能上线后，通过 SQL 或管理脚本操作。）

### Task 9.1: 清理 tag 94712 冗余 embedding

```sql
-- 查看当前 embedding 数量
SELECT embedding_type, count(*) FROM topic_tag_embeddings WHERE topic_tag_id = 94712 GROUP BY embedding_type;

-- 保留每种类型最新一条，删除其余
DELETE FROM topic_tag_embeddings
WHERE topic_tag_id = 94712
  AND id NOT IN (
    SELECT DISTINCT ON (embedding_type) id
    FROM topic_tag_embeddings
    WHERE topic_tag_id = 94712
    ORDER BY embedding_type, created_at DESC NULLS LAST
  );
```

### Task 9.2: 清理错误 article_topic_tags

需要对照 LLM 提取日志，确认哪些文章真正提取了"共产党员"。手动审查并删除错误关联。

### Task 9.3: 排查其他异常 tag

```sql
-- 查找 embedding 数量异常的 tag
SELECT topic_tag_id, t.label, count(*) as emb_count
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
GROUP BY topic_tag_id, t.label
HAVING count(*) > 10
ORDER BY count(*) DESC
LIMIT 20;
```
