# Persistent Firecrawl And Tag Queues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the in-memory tag queue and implicit Firecrawl article queue with restart-safe SQLite-backed job queues.

**Architecture:** Add dedicated `firecrawl_jobs` and `tag_jobs` tables, implement shared lease/retry semantics in backend services, and switch producers/schedulers to enqueue and consume jobs instead of volatile process memory. Keep existing article status fields for UI compatibility.

**Tech Stack:** Go, Gin, GORM, SQLite, existing scheduler/runtime framework, Go tests.

---

### Task 1: Add Persistent Job Models And Schema

**Files:**
- Modify: `backend-go/internal/domain/models/article.go`
- Create: `backend-go/internal/domain/models/job_queue.go`
- Modify: `backend-go/internal/platform/database/db.go`
- Test: `backend-go/internal/platform/database/db_test.go`

- [ ] **Step 1: Write the failing schema test**

```go
func TestJobQueueTablesExist(t *testing.T) {
	setupTestDB(t)
	if !database.DB.Migrator().HasTable(&models.FirecrawlJob{}) {
		t.Fatal("expected firecrawl_jobs table to exist")
	}
	if !database.DB.Migrator().HasTable(&models.TagJob{}) {
		t.Fatal("expected tag_jobs table to exist")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/database -run TestJobQueueTablesExist -v`
Expected: FAIL because the job models are not migrated.

- [ ] **Step 3: Add job models and migration wiring**

```go
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusLeased    JobStatus = "leased"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type FirecrawlJob struct {
	ID             uint      `gorm:"primaryKey"`
	ArticleID      uint      `gorm:"index;not null"`
	Status         string    `gorm:"size:20;index;not null"`
	Priority       int       `gorm:"default:0;index"`
	AttemptCount   int       `gorm:"default:0"`
	MaxAttempts    int       `gorm:"default:5"`
	AvailableAt    time.Time `gorm:"index;not null"`
	LeasedAt       *time.Time
	LeaseExpiresAt *time.Time `gorm:"index"`
	LastError      string     `gorm:"type:text"`
	URLSnapshot    string     `gorm:"size:1000"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (FirecrawlJob) TableName() string { return "firecrawl_jobs" }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/database -run TestJobQueueTablesExist -v`
Expected: PASS.

### Task 2: Add Queue Services With Lease/Retry Semantics

**Files:**
- Create: `backend-go/internal/domain/contentprocessing/firecrawl_job_queue.go`
- Create: `backend-go/internal/domain/topicextraction/tag_job_queue.go`
- Test: `backend-go/internal/domain/contentprocessing/firecrawl_job_queue_test.go`
- Test: `backend-go/internal/domain/topicextraction/tag_job_queue_test.go`

- [ ] **Step 1: Write failing queue behavior tests**

```go
func TestEnqueueFirecrawlJobDedupesActiveJob(t *testing.T) {}
func TestClaimFirecrawlJobsReclaimsExpiredLease(t *testing.T) {}
func TestEnqueueTagJobUpgradesForceRetag(t *testing.T) {}
func TestFailTagJobRetriesUntilMaxAttempts(t *testing.T) {}
```

- [ ] **Step 2: Run targeted tests to verify failure**

Run: `go test ./internal/domain/contentprocessing ./internal/domain/topicextraction -run 'Test(Enqueue|Claim|Fail)' -v`
Expected: FAIL because queue services do not exist.

- [ ] **Step 3: Implement minimal queue services**

```go
type FirecrawlJobQueue struct{ db *gorm.DB }

func (q *FirecrawlJobQueue) Enqueue(article models.Article) error
func (q *FirecrawlJobQueue) Claim(limit int, lease time.Duration) ([]models.FirecrawlJob, error)
func (q *FirecrawlJobQueue) MarkCompleted(id uint) error
func (q *FirecrawlJobQueue) MarkRetry(id uint, maxAttempts int, errMsg string, backoff time.Duration) error
func (q *FirecrawlJobQueue) MarkPermanentFailure(id uint, errMsg string) error
```

- [ ] **Step 4: Run queue tests to verify they pass**

Run: `go test ./internal/domain/contentprocessing ./internal/domain/topicextraction -run 'Test(Enqueue|Claim|Fail)' -v`
Expected: PASS.

### Task 3: Switch Producers To Persistent Queue Enqueue

**Files:**
- Modify: `backend-go/internal/domain/feeds/service.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_handler.go`
- Test: `backend-go/internal/domain/feeds/service_test.go`
- Test: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`

- [ ] **Step 1: Add failing producer tests**

```go
func TestRefreshFeedEnqueuesFirecrawlJobWhenEnabled(t *testing.T) {}
func TestRefreshFeedEnqueuesTagJobWhenFirecrawlDisabled(t *testing.T) {}
func TestCompleteArticleEnqueuesRetagJob(t *testing.T) {}
```

- [ ] **Step 2: Run producer tests to verify failure**

Run: `go test ./internal/domain/feeds ./internal/domain/contentprocessing -run 'TestRefreshFeedEnqueues|TestCompleteArticleEnqueues' -v`
Expected: FAIL because producers still use the old flow.

- [ ] **Step 3: Replace direct queue/channel usage with DB-backed enqueue**

```go
if feed.FirecrawlEnabled {
	if err := contentprocessing.NewFirecrawlJobQueue(database.DB).Enqueue(article); err != nil {
		return err
	}
} else {
	if err := topicextraction.NewTagJobQueue(database.DB).Enqueue(topicextraction.TagJobRequest{
		ArticleID: article.ID,
		FeedName:  feed.Title,
	}); err != nil {
		return err
	}
}
```

- [ ] **Step 4: Run producer tests to verify they pass**

Run: `go test ./internal/domain/feeds ./internal/domain/contentprocessing -run 'TestRefreshFeedEnqueues|TestCompleteArticleEnqueues' -v`
Expected: PASS.

### Task 4: Replace Firecrawl Article Scanning With Job Claims

**Files:**
- Modify: `backend-go/internal/jobs/firecrawl.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_service.go`
- Test: `backend-go/internal/jobs/firecrawl_test.go`

- [ ] **Step 1: Add failing scheduler tests**

```go
func TestFirecrawlSchedulerProcessesClaimedJobs(t *testing.T) {}
func TestFirecrawlSchedulerRetriesFailedJobs(t *testing.T) {}
```

- [ ] **Step 2: Run scheduler tests to verify failure**

Run: `go test ./internal/jobs -run 'TestFirecrawlScheduler(ProcessesClaimedJobs|RetriesFailedJobs)' -v`
Expected: FAIL because scheduler still scans articles directly.

- [ ] **Step 3: Implement job-based Firecrawl execution**

```go
jobs, err := s.queue.Claim(50, s.leaseDuration())
for _, job := range jobs {
	// load article/feed, scrape, update article, enqueue retag, mark job status
}
```

- [ ] **Step 4: Run scheduler tests to verify they pass**

Run: `go test ./internal/jobs -run 'TestFirecrawlScheduler(ProcessesClaimedJobs|RetriesFailedJobs)' -v`
Expected: PASS.

### Task 5: Replace In-Memory TagQueue With Job Worker

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tag_queue.go`
- Modify: `backend-go/internal/app/runtime.go`
- Test: `backend-go/internal/domain/topicextraction/metadata_test.go`
- Test: `backend-go/internal/domain/topicextraction/tag_job_queue_test.go`

- [ ] **Step 1: Add failing worker tests**

```go
func TestTagQueueProcessesPersistentJobs(t *testing.T) {}
func TestTagQueueRecoversExpiredLease(t *testing.T) {}
```

- [ ] **Step 2: Run worker tests to verify failure**

Run: `go test ./internal/domain/topicextraction -run 'TestTagQueue(ProcessesPersistentJobs|RecoversExpiredLease)' -v`
Expected: FAIL because the worker still uses a channel.

- [ ] **Step 3: Implement DB-backed tag worker behind the existing runtime entry point**

```go
type TagQueue struct {
	queue *TagJobQueue
	// worker lifecycle fields
}

func (q *TagQueue) Enqueue(articleID uint, feedName, categoryName string) error {
	return q.queue.Enqueue(TagJobRequest{ArticleID: articleID, FeedName: feedName, CategoryName: categoryName})
}
```

- [ ] **Step 4: Run worker tests to verify they pass**

Run: `go test ./internal/domain/topicextraction -run 'TestTagQueue(ProcessesPersistentJobs|RecoversExpiredLease)' -v`
Expected: PASS.

### Task 6: Verify Recovery And Compatibility

**Files:**
- Modify: `backend-go/internal/jobs/firecrawl_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`
- Modify: `backend-go/internal/domain/feeds/service_test.go`

- [ ] **Step 1: Add restart/recovery regression tests**

```go
func TestFirecrawlJobSurvivesRestartLikeLeaseExpiry(t *testing.T) {}
func TestTagJobSurvivesRestartLikeLeaseExpiry(t *testing.T) {}
func TestFirecrawlCompletionEnqueuesSingleActiveRetagJob(t *testing.T) {}
```

- [ ] **Step 2: Run regression tests to verify failure**

Run: `go test ./internal/jobs ./internal/domain/contentprocessing ./internal/domain/feeds -run 'Test(FirecrawlJobSurvivesRestartLikeLeaseExpiry|TagJobSurvivesRestartLikeLeaseExpiry|FirecrawlCompletionEnqueuesSingleActiveRetagJob)' -v`
Expected: FAIL until the full queue flow is wired.

- [ ] **Step 3: Fill any remaining gaps and keep article status compatibility**

```go
// preserve article.firecrawl_status and article.summary_status updates
// expose queue stats without changing existing response shape unexpectedly
```

- [ ] **Step 4: Run focused verification and broad backend tests**

Run: `go test ./internal/domain/contentprocessing ./internal/domain/topicextraction ./internal/domain/feeds ./internal/jobs -v`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.
