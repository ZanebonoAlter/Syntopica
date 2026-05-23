# Tag Quality Score Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a multi-dimensional `quality_score` to topic tags, computed hourly by a scheduler, used for sorting, hiding low-quality tags, and topic graph visualization.

**Architecture:** New scheduler job (`tag_quality_score`) computes scores from article frequency, co-occurrence, feed diversity, and embedding similarity. Scores are stored in `topic_tags.quality_score`. Frontend uses the score for tag sorting, hiding low-quality tags, and graph node sizing.

**Tech Stack:** Go/Gin/GORM backend, Vue 3/Nuxt frontend, cron-based scheduler

---

### Task 1: Add `QualityScore` field to TopicTag model

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Modify: `backend-go/internal/domain/topictypes/types.go`

**Step 1: Add field to model**

In `backend-go/internal/domain/models/topic_graph.go`, add to `TopicTag` struct after `WatchedAt`:

```go
QualityScore float64 `gorm:"default:0" json:"quality_score"`
```

**Step 2: Add field to API type**

In `backend-go/internal/domain/topictypes/types.go`, add to `TopicTag` struct after `ChildSlugs`:

```go
QualityScore float64  `json:"quality_score,omitempty"`
```

Also add to `TagHierarchyNode` struct after `IsActive`:

```go
QualityScore float64 `json:"quality_score,omitempty"`
```

**Step 3: Verify migration works**

Run: `cd backend-go && go build ./...`

**Step 4: Commit**

```bash
git add backend-go/internal/domain/models/topic_graph.go backend-go/internal/domain/topictypes/types.go
git commit -m "feat: add quality_score field to TopicTag model and API types"
```

---

### Task 2: Create the quality score calculation logic

**Files:**
- Create: `backend-go/internal/domain/topicextraction/quality_score.go`
- Create: `backend-go/internal/domain/topicextraction/quality_score_test.go`

**Step 1: Write the test**

Create `backend-go/internal/domain/topicextraction/quality_score_test.go`:

```go
package topicextraction

import (
	"math"
	"testing"
)

func TestPercentileRank(t *testing.T) {
	values := map[uint]float64{1: 10, 2: 20, 3: 30, 4: 40, 5: 50}
	result := percentileRank(values, 1)
	if result != 0.2 {
		t.Errorf("expected 0.2, got %f", result)
	}
	result = percentileRank(values, 5)
	if result != 1.0 {
		t.Errorf("expected 1.0, got %f", result)
	}
}

func TestPercentileRankEmpty(t *testing.T) {
	values := map[uint]float64{}
	result := percentileRank(values, 99)
	if result != 0.5 {
		t.Errorf("expected 0.5 for missing tag, got %f", result)
	}
}

func TestComputeQualityScore(t *testing.T) {
	score := computeQualityScore(0.8, 0.6, 0.7, 0.9)
	expected := 0.4*0.8 + 0.2*0.6 + 0.2*0.7 + 0.2*0.9
	if math.Abs(score-expected) > 0.001 {
		t.Errorf("expected %f, got %f", expected, score)
	}
}

func TestComputeQualityScoreDefaults(t *testing.T) {
	score := computeQualityScore(0, 0, 0, 0)
	if score != 0 {
		t.Errorf("expected 0 for all-zero dimensions, got %f", score)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -run TestPercentile -v`

**Step 3: Write the implementation**

Create `backend-go/internal/domain/topicextraction/quality_score.go`:

```go
package topicextraction

import (
	"fmt"
	"log"
	"sort"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

type tagMetrics struct {
	TagID          uint
	ArticleCount   int
	FeedDiversity  int
	AvgCooccurrence float64
}

func computeQualityScore(freqPct, coocPct, feedDivPct, similarityPct float64) float64 {
	return 0.4*freqPct + 0.2*coocPct + 0.2*feedDivPct + 0.2*similarityPct
}

func percentileRank(values map[uint]float64, tagID uint) float64 {
	if len(values) == 0 {
		return 0.5
	}
	val, ok := values[tagID]
	if !ok {
		return 0.5
	}
	var count int
	for _, v := range values {
		if v <= val {
			count++
		}
	}
	return float64(count) / float64(len(values))
}

func computeAllQualityScores() error {
	// 1. Collect raw metrics per tag
	var metrics []tagMetrics
	err := database.DB.Raw(`
		SELECT
			t.id AS tag_id,
			COUNT(DISTINCT att.article_id) AS article_count,
			COUNT(DISTINCT a.feed_id) AS feed_diversity,
			COALESCE(AVG(cooc.cooc_count), 0) AS avg_cooccurrence
		FROM topic_tags t
		LEFT JOIN article_topic_tags att ON att.topic_tag_id = t.id
		LEFT JOIN articles a ON a.id = att.article_id
		LEFT JOIN (
			SELECT att1.article_id, COUNT(DISTINCT att1.topic_tag_id) - 1 AS cooc_count
			FROM article_topic_tags att1
			GROUP BY att1.article_id
		) cooc ON cooc.article_id = att.article_id
		WHERE t.status = 'active'
		GROUP BY t.id
	`).Scan(&metrics).Error
	if err != nil {
		return fmt.Errorf("query tag metrics: %w", err)
	}

	if len(metrics) == 0 {
		return nil
	}

	// 2. Build per-dimension maps
	freqMap := make(map[uint]float64, len(metrics))
	coocMap := make(map[uint]float64, len(metrics))
	feedDivMap := make(map[uint]float64, len(metrics))

	tagIDs := make([]uint, 0, len(metrics))
	for _, m := range metrics {
		tagIDs = append(tagIDs, m.TagID)
		freqMap[m.TagID] = float64(m.ArticleCount)
		coocMap[m.TagID] = m.AvgCooccurrence
		feedDivMap[m.TagID] = float64(m.FeedDiversity)
	}

	// 3. Default similarity to 0.7 for tags without embedding data
	similarityMap := make(map[uint]float64, len(metrics))
	for _, id := range tagIDs {
		similarityMap[id] = 0.7
	}

	// 4. Compute quality scores for normal tags
	type scoreUpdate struct {
		TagID        uint
		QualityScore float64
	}
	updates := make([]scoreUpdate, 0, len(metrics))
	for _, m := range metrics {
		if m.ArticleCount == 0 {
			updates = append(updates, scoreUpdate{TagID: m.TagID, QualityScore: 0})
			continue
		}
		score := computeQualityScore(
			percentileRank(freqMap, m.TagID),
			percentileRank(coocMap, m.TagID),
			percentileRank(feedDivMap, m.TagID),
			percentileRank(similarityMap, m.TagID),
		)
		updates = append(updates, scoreUpdate{TagID: m.TagID, QualityScore: score})
	}

	// 5. Write scores to database
	for _, u := range updates {
		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", u.TagID).Update("quality_score", u.QualityScore).Error; err != nil {
			log.Printf("[WARN] Failed to update quality_score for tag %d: %v", u.TagID, err)
		}
	}

	// 6. Compute quality scores for abstract tags (weighted average of children)
	computeAbstractQualityScores()

	log.Printf("[INFO] Quality scores computed for %d tags", len(updates))
	return nil
}

func computeAbstractQualityScores() {
	type abstractChild struct {
		ParentID     uint
		ChildID      uint
		ArticleCount int
	}
	var relations []abstractChild
	database.DB.Raw(`
		SELECT r.parent_id, r.child_id,
			(SELECT COUNT(*) FROM article_topic_tags att WHERE att.topic_tag_id = r.child_id) AS article_count
		FROM topic_tag_relations r
		WHERE r.relation_type = 'abstract'
	`).Scan(&relations)

	// Group by parent
	children := make(map[uint][]abstractChild)
	for _, r := range relations {
		children[r.ParentID] = append(children[r.ParentID], r)
	}

	// Load existing quality scores for children
	childIDs := make(map[uint]bool)
	for _, group := range children {
		for _, c := range group {
			childIDs[c.ChildID] = true
		}
	}

	childScores := make(map[uint]float64)
	if len(childIDs) > 0 {
		ids := make([]uint, 0, len(childIDs))
		for id := range childIDs {
			ids = append(ids, id)
		}
		var tags []models.TopicTag
		database.DB.Select("id, quality_score").Where("id IN ?", ids).Find(&tags)
		for _, t := range tags {
			childScores[t.ID] = t.QualityScore
		}
	}

	// Compute weighted average for each parent
	for parentID, group := range children {
		var totalWeight float64
		var weightedSum float64
		for _, c := range group {
			weight := float64(c.ArticleCount)
			if weight == 0 {
				weight = 1
			}
			weightedSum += childScores[c.ChildID] * weight
			totalWeight += weight
		}
		if totalWeight == 0 {
			continue
		}
		score := weightedSum / totalWeight
		database.DB.Model(&models.TopicTag{}).Where("id = ?", parentID).Update("quality_score", score)
	}

	// Sort helper for consistency
	_ = sort.Slice
}
```

**Step 4: Run tests**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -run "TestPercentile|TestCompute" -v`

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicextraction/quality_score.go backend-go/internal/domain/topicextraction/quality_score_test.go
git commit -m "feat: add quality score calculation logic with percentile normalization"
```

---

### Task 3: Create the scheduler job

**Files:**
- Create: `backend-go/internal/jobs/tag_quality_score.go`

**Step 1: Write the scheduler**

Create `backend-go/internal/jobs/tag_quality_score.go`, following the `auto_tag_merge.go` pattern:

```go
package jobs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicextraction"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/tracing"

	"context"
)

type TagQualityScoreScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	isRunning      bool
	executionMutex sync.Mutex
	isExecuting    bool
}

type TagQualityRunSummary struct {
	TriggerSource string `json:"trigger_source"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	TagsUpdated   int    `json:"tags_updated"`
	Reason        string `json:"reason"`
}

func NewTagQualityScoreScheduler(checkInterval int) *TagQualityScoreScheduler {
	return &TagQualityScoreScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
		isRunning:     false,
	}
}

func (s *TagQualityScoreScheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("tag quality score scheduler already running")
	}
	s.initSchedulerTask()

	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.runCycle); err != nil {
		return fmt.Errorf("failed to schedule tag quality score: %w", err)
	}

	s.cron.Start()
	s.isRunning = true
	logging.Infof("Tag quality score scheduler started with interval: %v", s.checkInterval)
	return nil
}

func (s *TagQualityScoreScheduler) Stop() {
	if !s.isRunning {
		return
	}
	s.cron.Stop()
	s.isRunning = false
	logging.Infoln("Tag quality score scheduler stopped")
}

func (s *TagQualityScoreScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	wasRunning := s.isRunning
	if wasRunning {
		s.Stop()
	}
	s.cron = cron.New()
	s.checkInterval = time.Duration(interval) * time.Second
	if wasRunning {
		return s.Start()
	}
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}
	return nil
}

func (s *TagQualityScoreScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err != nil {
		return err
	}
	nextRun := time.Now().Add(s.checkInterval)
	return database.DB.Model(&task).Updates(map[string]interface{}{
		"status": "idle", "last_error": "", "last_error_time": nil,
		"total_executions": 0, "successful_executions": 0, "failed_executions": 0,
		"consecutive_failures": 0, "last_execution_time": nil,
		"last_execution_duration": nil, "last_execution_result": "",
		"next_execution_time": &nextRun,
	}).Error
}

func (s *TagQualityScoreScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted": false, "started": false, "reason": "already_running",
			"message": "质量评分正在计算中，请稍后再试。", "status_code": http.StatusConflict,
		}
	}
	s.isExecuting = true
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in tag quality score: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.execute("manual")
	}()
	return map[string]interface{}{
		"accepted": true, "started": true, "reason": "manual_run_started",
		"message": "标签质量评分计算已开始。",
	}
}

func (s *TagQualityScoreScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil {
		updates := map[string]interface{}{
			"description":         "Compute tag quality scores from multi-dimensional signals",
			"check_interval":      int(s.checkInterval.Seconds()),
			"next_execution_time": &nextRun,
		}
		if task.Status == "" || task.Status == "success" || task.Status == "failed" {
			updates["status"] = "idle"
		}
		database.DB.Model(&task).Updates(updates)
		return
	}
	task = models.SchedulerTask{
		Name: "tag_quality_score", Description: "Compute tag quality scores from multi-dimensional signals",
		CheckInterval: int(s.checkInterval.Seconds()), Status: "idle", NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *TagQualityScoreScheduler) runCycle() {
	tracing.TraceSchedulerTick("tag_quality_score", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infoln("Tag quality score already in progress, skipping")
			return
		}
		s.isExecuting = true
		defer func() {
			s.executionMutex.Unlock()
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in tag quality score cycle: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.execute("scheduled")
	})
}

func (s *TagQualityScoreScheduler) execute(triggerSource string) {
	logging.Infoln("Starting tag quality score computation")
	startTime := time.Now()
	summary := &TagQualityRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	err := topicextraction.ComputeAllQualityScores()
	if err != nil {
		logging.Errorf("Tag quality score computation failed: %v", err)
		summary.Reason = "failed"
		summary.FinishedAt = time.Now().Format(time.RFC3339)
		s.updateSchedulerStatus("idle", err.Error(), &startTime, summary)
		return
	}

	logging.Infof("Tag quality score computation completed in %v", time.Since(startTime))
	summary.Reason = "completed"
	summary.FinishedAt = time.Now().Format(time.RFC3339)
	s.updateSchedulerStatus("idle", "", &startTime, summary)
}

func (s *TagQualityScoreScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *TagQualityRunSummary) {
	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error
	now := time.Now()
	resultJSON := ""
	if summary != nil {
		if data, marshalErr := json.Marshal(summary); marshalErr == nil {
			resultJSON = string(data)
		}
	}
	if err == nil {
		task.Status = status
		task.LastError = lastError
		if lastError != "" {
			errTime := now
			task.LastErrorTime = &errTime
		}
		if resultJSON != "" {
			task.LastExecutionResult = resultJSON
		}
		if startTime != nil {
			duration := time.Since(*startTime)
			durationFloat := duration.Seconds()
			task.LastExecutionDuration = &durationFloat
			task.LastExecutionTime = &now
			if status == "idle" && lastError == "" {
				task.SuccessfulExecutions++
				task.ConsecutiveFailures = 0
			} else if lastError != "" {
				task.FailedExecutions++
				task.ConsecutiveFailures++
			}
			task.TotalExecutions++
			nextExecution := now.Add(s.checkInterval)
			task.NextExecutionTime = &nextExecution
		}
		database.DB.Save(&task)
	} else {
		nextExecution := now.Add(s.checkInterval)
		var durationFloat *float64
		if startTime != nil {
			d := time.Since(*startTime)
			df := d.Seconds()
			durationFloat = &df
		}
		task = models.SchedulerTask{
			Name: "tag_quality_score", Description: "Compute tag quality scores from multi-dimensional signals",
			CheckInterval: int(s.checkInterval.Seconds()), Status: status, LastError: lastError,
			NextExecutionTime: &nextExecution, LastExecutionDuration: durationFloat, LastExecutionResult: resultJSON,
		}
		if startTime != nil {
			task.LastExecutionTime = &now
		}
		database.DB.Create(&task)
	}
}

func (s *TagQualityScoreScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()
	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}
	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error
	status := SchedulerStatusResponse{
		Name: "Tag Quality Score",
		Status: func() string {
			if s.isExecuting {
				return "running"
			}
			if s.isRunning {
				return "idle"
			}
			return "stopped"
		}(),
		CheckInterval: int64(s.checkInterval.Seconds()),
		NextRun:       nextRun,
		IsExecuting:   s.isExecuting,
	}
	if err == nil && task.NextExecutionTime != nil {
		status.NextRun = task.NextExecutionTime.Unix()
	}
	return status
}

func (s *TagQualityScoreScheduler) IsExecuting() bool {
	return s.isExecuting
}
```

**Step 2: Verify compilation**

Run: `cd backend-go && go build ./...`

**Step 3: Commit**

```bash
git add backend-go/internal/jobs/tag_quality_score.go
git commit -m "feat: add tag quality score scheduler job"
```

---

### Task 4: Export calculation function and wire scheduler into runtime

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/quality_score.go`
- Modify: `backend-go/internal/app/runtime.go`
- Modify: `backend-go/internal/app/runtimeinfo/schedulers.go`
- Modify: `backend-go/internal/jobs/handler.go`

**Step 1: Export the computation function**

In `quality_score.go`, rename `computeAllQualityScores` to `ComputeAllQualityScores` (export it). This is needed by the scheduler.

**Step 2: Add to runtimeinfo**

In `backend-go/internal/app/runtimeinfo/schedulers.go`, add:

```go
var TagQualityScoreSchedulerInterface interface{}
```

**Step 3: Wire into runtime**

In `backend-go/internal/app/runtime.go`:
- Add `TagQualityScore *jobs.TagQualityScoreScheduler` to `Runtime` struct
- After the AutoTagMerge block (around line 112), add:

```go
	runtime.TagQualityScore = jobs.NewTagQualityScoreScheduler(3600)
	if err := runtime.TagQualityScore.Start(); err != nil {
		logging.Warnf("Failed to start tag quality score scheduler: %v", err)
	} else {
		logging.Infoln("Tag quality score scheduler started successfully")
	}
```

- Add `runtimeinfo.TagQualityScoreSchedulerInterface = runtime.TagQualityScore`
- Add stop in `SetupGracefulShutdown`:

```go
if runtime.TagQualityScore != nil {
	logging.Infoln("Stopping tag quality score scheduler...")
	runtime.TagQualityScore.Stop()
}
```

**Step 4: Add scheduler descriptor**

In `backend-go/internal/jobs/handler.go`, add to `schedulerDescriptors()`:

```go
{
	Name:        "tag_quality_score",
	DisplayName: "Tag Quality Score",
	Description: "Compute tag quality scores from multi-dimensional signals",
	Get: func() interface{} {
		return runtimeinfo.TagQualityScoreSchedulerInterface
	},
},
```

**Step 5: Verify build**

Run: `cd backend-go && go build ./...`

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicextraction/quality_score.go backend-go/internal/app/runtime.go backend-go/internal/app/runtimeinfo/schedulers.go backend-go/internal/jobs/handler.go
git commit -m "feat: wire tag quality score scheduler into runtime and handler"
```

---

### Task 5: Use `quality_score` for sorting in topic graph APIs

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/service.go`

**Step 1: Update sorting in GetTopicsByCategory**

In the `sortTagsByScoreMap` function results and `GetTopicsByCategory`, change the sort to use `quality_score` instead of `score` where appropriate. The key locations:

- `GetTopicsByCategory` (around line 625-675): where tags are grouped by category
- `sortTagsByScoreMap` (around line 680): sort by `QualityScore` descending

Replace sorting comparisons that use `Score` with `QualityScore` for the final display order.

**Step 2: Update GetUnclassifiedTags sorting**

In `abstract_tag_service.go` `GetUnclassifiedTags`, change `Order("feed_count DESC")` to `Order("quality_score DESC")` (around line 803).

**Step 3: Add QualityScore to TagHierarchyNode population**

In `abstract_tag_service.go` `GetTagHierarchy`, when building `TagHierarchyNode` structs, include the `QualityScore` from the tag model.

**Step 4: Verify build**

Run: `cd backend-go && go build ./...`

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicgraph/service.go backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat: sort tags by quality_score in topic graph APIs"
```

---

### Task 6: Frontend - Add quality_score to API types and use in topic graph

**Files:**
- Modify: `front/app/api/topicGraph.ts`
- Modify: `front/app/features/topic-graph/utils/buildTopicGraphViewModel.ts`

**Step 1: Add quality_score to frontend TopicTag type**

In `front/app/api/topicGraph.ts` `TopicTag` interface, add:

```ts
quality_score?: number
```

Also add to `TopicTreeNode` or `TagHierarchyNode` if present.

**Step 2: Use quality_score in graph visualization**

In the topic graph build utility, use `quality_score` to influence node opacity or size. Tags with `quality_score < 0.3` should have reduced opacity (e.g., 0.4).

**Step 3: Verify frontend**

Run: `cd front && pnpm exec nuxi typecheck`

**Step 4: Commit**

```bash
git add front/app/api/topicGraph.ts front/app/features/topic-graph/
git commit -m "feat: use quality_score in frontend topic graph visualization"
```

---

### Task 7: Frontend - Hide low-quality tags with toggle

**Files:**
- Modify tag list components in `front/app/features/topic-graph/`

**Step 1: Add low-quality filtering logic**

In tag list / tag cloud components, filter out tags with `quality_score < 0.3` by default. Add a toggle button to "show all tags".

**Step 2: Verify**

Run: `cd front && pnpm exec nuxi typecheck && pnpm build`

**Step 3: Commit**

```bash
git add front/app/features/topic-graph/
git commit -m "feat: hide low-quality tags by default with toggle"
```

---

### Task 8: Verify end-to-end

**Step 1: Backend tests**

Run: `cd backend-go && go test ./... -count=1`

**Step 2: Frontend typecheck**

Run: `cd front && pnpm exec nuxi typecheck && pnpm build`

**Step 3: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: address test/typecheck issues from quality score integration"
```
