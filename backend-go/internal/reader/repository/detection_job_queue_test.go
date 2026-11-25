package repository

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ============================================================================
// 判定作业队列包级测试（add-ai-generated-article-detection task 3.1）
// ============================================================================

// setupDetectionJobQueueTestDB 装一份单连接内存 sqlite（单连接把并发写序列化在
// 驱动层，避免 shared-cache 抖动；乐观锁语义不受影响）。
func setupDetectionJobQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	database.DB = db
	InitRepository(db)
	if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.DetectionJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	// 生产迁移 20261120_0003 建出的部分唯一索引（sqlite 测试库由 AutoMigrate 建模，
	// 不走版本化迁移）：入队幂等的数据库级兜底就是它，整组用例都在这个约束下跑。
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_detection_jobs_article_active ON detection_jobs (article_id) WHERE status IN ('pending', 'leased')`).Error; err != nil {
		t.Fatalf("create partial unique index: %v", err)
	}
	return db
}

func seedDetectionArticle(t *testing.T, db *gorm.DB, title string) models.Article {
	t.Helper()

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s/%s", t.Name(), title)}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}
	article := models.Article{
		FeedID:      feed.ID,
		Title:       title,
		Link:        fmt.Sprintf("https://example.com/%s/%s", t.Name(), title),
		Description: "body text",
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	return article
}

func countDetectionJobs(t *testing.T, db *gorm.DB, where string, args ...any) int64 {
	t.Helper()

	query := db.Model(&models.DetectionJob{})
	if where != "" {
		query = query.Where(where, args...)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count detection jobs: %v", err)
	}
	return count
}

// 重复入队不产生第二条未完成作业：同文章第二次 Enqueue 原地更新（spec Scenario
// 「重复触发判定不产生重复作业」）。
func TestEnqueueDetectionJobDedupesActiveJob(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "dedupe")
	queue := NewDetectionJobQueue(db)

	for i := 0; i < 3; i++ {
		if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonArticleIngested}); err != nil {
			t.Fatalf("enqueue #%d: %v", i, err)
		}
	}

	if got := countDetectionJobs(t, db, "article_id = ?", article.ID); got != 1 {
		t.Fatalf("detection jobs = %d, want 1", got)
	}

	// 已完成/已失败的文章可重新入队（不是永久去重）。
	var job models.DetectionJob
	if err := db.First(&job).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if err := queue.MarkCompleted(job.ID); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonFirecrawlCompleted}); err != nil {
		t.Fatalf("re-enqueue after completion: %v", err)
	}
	if got := countDetectionJobs(t, db, "article_id = ?", article.ID); got != 2 {
		t.Fatalf("detection jobs after re-enqueue = %d, want 2", got)
	}
}

// 显式重判把既有未完成作业提升为 manual_recheck 并插队（而不是新建第二条）。
func TestEnqueueDetectionJobUpgradesActiveJobToRecheck(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "recheck")
	queue := NewDetectionJobQueue(db)

	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonArticleIngested}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := queue.Enqueue(DetectionJobRequest{
		ArticleID: article.ID,
		Reason:    DetectionJobReasonManualRecheck,
		Priority:  10,
	}); err != nil {
		t.Fatalf("enqueue recheck: %v", err)
	}

	var jobs []models.DetectionJob
	if err := db.Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("detection jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Reason != DetectionJobReasonManualRecheck {
		t.Fatalf("reason = %q, want %q", jobs[0].Reason, DetectionJobReasonManualRecheck)
	}
	if jobs[0].Priority != 10 {
		t.Fatalf("priority = %d, want 10", jobs[0].Priority)
	}
	if !IsForceDetectionReason(jobs[0].Reason) {
		t.Fatalf("manual_recheck must be a force reason")
	}
}

// 显式重判复用活跃作业时把重试预算归零（review R-15）：连点几次「交由模型重新判定」
// 不再把作业推到 max_attempts 判死；普通入队不动计数，已是 manual_recheck 的作业再
// 重判也照样重置（不能只在 reason 变化时才生效）。
func TestEnqueueDetectionJobRecheckResetsAttemptBudget(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "recheck-budget")
	queue := NewDetectionJobQueue(db)

	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonArticleIngested}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var job models.DetectionJob
	if err := db.First(&job).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}

	burn := func(attempts int) {
		t.Helper()
		if err := db.Model(&models.DetectionJob{}).Where("id = ?", job.ID).Update("attempt_count", attempts).Error; err != nil {
			t.Fatalf("burn attempts to %d: %v", attempts, err)
		}
	}
	reload := func() models.DetectionJob {
		t.Helper()
		var got models.DetectionJob
		if err := db.First(&got, job.ID).Error; err != nil {
			t.Fatalf("reload job: %v", err)
		}
		return got
	}

	// 普通入队（内容就绪节点）不动重试计数。
	burn(4)
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonArticleIngested}); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	if got := reload(); got.AttemptCount != 4 {
		t.Fatalf("attempt_count after plain enqueue = %d, want 4 (must not reset)", got.AttemptCount)
	}

	// 显式重判：预算归零，上限不动。
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonManualRecheck, Priority: 10}); err != nil {
		t.Fatalf("enqueue recheck: %v", err)
	}
	got := reload()
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count after recheck = %d, want 0", got.AttemptCount)
	}
	if got.Reason != DetectionJobReasonManualRecheck {
		t.Fatalf("reason = %q, want %q", got.Reason, DetectionJobReasonManualRecheck)
	}
	if got.MaxAttempts != defaultDetectionJobMaxAttempts {
		t.Fatalf("max_attempts = %d, want %d (reset must not touch the limit)", got.MaxAttempts, defaultDetectionJobMaxAttempts)
	}

	// 已是 manual_recheck 的作业再重判也必须重置（reason 未变化不影响）。
	burn(3)
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: article.ID, Reason: DetectionJobReasonManualRecheck, Priority: 10}); err != nil {
		t.Fatalf("second recheck: %v", err)
	}
	if got := reload(); got.AttemptCount != 0 {
		t.Fatalf("attempt_count after repeated recheck = %d, want 0", got.AttemptCount)
	}
}

// 并发 claim 不重复领取：两个 goroutine 同时 claim，并集恰好覆盖全部作业且无重复 id。
func TestClaimDetectionJobsConcurrentClaimYieldsEachJobOnce(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)

	const total = 5
	for i := 0; i < total; i++ {
		article := seedDetectionArticle(t, db, fmt.Sprintf("claim-%d", i))
		if err := NewDetectionJobQueue(db).Enqueue(DetectionJobRequest{
			ArticleID: article.ID,
			Reason:    DetectionJobReasonArticleIngested,
		}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	queue := NewDetectionJobQueue(db)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claimed []uint
		errs    []error
	)
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs, err := queue.Claim(total, 5*time.Minute)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, job := range jobs {
				claimed = append(claimed, job.ID)
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		t.Fatalf("concurrent claim: %v", err)
	}
	if len(claimed) != total {
		t.Fatalf("claimed jobs = %d, want %d (ids=%v)", len(claimed), total, claimed)
	}
	seen := map[uint]bool{}
	for _, id := range claimed {
		if seen[id] {
			t.Fatalf("job %d claimed twice: %v", id, claimed)
		}
		seen[id] = true
	}

	// 再 claim 一次：全部处于 leased 且租约未过期，不得再被领取。
	jobs, err := queue.Claim(total, 5*time.Minute)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("second claim got %d jobs, want 0", len(jobs))
	}
}

// 租约过期后可被重新领取（attempt_count 递增）。
func TestClaimDetectionJobsReclaimsExpiredLease(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "expired")

	past := time.Now().Add(-time.Hour)
	job := models.DetectionJob{
		ArticleID:      article.ID,
		Status:         string(models.JobStatusLeased),
		AttemptCount:   1,
		MaxAttempts:    5,
		AvailableAt:    past,
		LeasedAt:       &past,
		LeaseExpiresAt: &past,
		Reason:         DetectionJobReasonArticleIngested,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create leased job: %v", err)
	}

	jobs, err := NewDetectionJobQueue(db).Claim(1, 5*time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claimed jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Status != string(models.JobStatusLeased) {
		t.Fatalf("status = %q, want leased", jobs[0].Status)
	}
	if jobs[0].AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2", jobs[0].AttemptCount)
	}
	if jobs[0].LeaseExpiresAt == nil || !jobs[0].LeaseExpiresAt.After(time.Now()) {
		t.Fatalf("lease expiry = %#v, want future time", jobs[0].LeaseExpiresAt)
	}
}

// 失败重试达上限后终止为 failed（文章判定状态由 worker 侧保证不被触碰）。
func TestMarkFailedDetectionJobStopsAtMaxAttempts(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "exhausted")
	queue := NewDetectionJobQueue(db)

	now := time.Now()
	job := models.DetectionJob{
		ArticleID:    article.ID,
		Status:       string(models.JobStatusLeased),
		AttemptCount: 1,
		MaxAttempts:  1,
		AvailableAt:  now,
		LeasedAt:     &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := queue.MarkFailed(job, "boom", time.Minute); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var got models.DetectionJob
	if err := db.First(&got, job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if got.Status != string(models.JobStatusFailed) {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.LastError != "boom" {
		t.Fatalf("last_error = %q, want boom", got.LastError)
	}
}

// 未达上限的失败按退避重新排队（status 回到 pending，available_at 后移）。
func TestMarkFailedDetectionJobRequeuesBelowMaxAttempts(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "retry")
	queue := NewDetectionJobQueue(db)

	now := time.Now()
	job := models.DetectionJob{
		ArticleID:    article.ID,
		Status:       string(models.JobStatusLeased),
		AttemptCount: 1,
		MaxAttempts:  5,
		AvailableAt:  now,
		LeasedAt:     &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := queue.MarkFailed(job, "transient", 5*time.Minute); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var got models.DetectionJob
	if err := db.First(&got, job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if got.Status != string(models.JobStatusPending) {
		t.Fatalf("status = %q, want pending", got.Status)
	}
	if got.LeasedAt != nil || got.LeaseExpiresAt != nil {
		t.Fatalf("lease must be released on retry, got leased_at=%v lease_expires_at=%v", got.LeasedAt, got.LeaseExpiresAt)
	}
	if !got.AvailableAt.After(now) {
		t.Fatalf("available_at = %v, want after %v", got.AvailableAt, now)
	}
}

// 领取顺序：priority 降序（显式重判插队），同 priority 新任务优先。
func TestClaimDetectionJobsOrderedByPriorityThenNewest(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	queue := NewDetectionJobQueue(db)

	first := seedDetectionArticle(t, db, "first")
	second := seedDetectionArticle(t, db, "second")
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: first.ID, Reason: DetectionJobReasonArticleIngested}); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if err := queue.Enqueue(DetectionJobRequest{ArticleID: second.ID, Reason: DetectionJobReasonManualRecheck, Priority: 10}); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	jobs, err := queue.Claim(2, 5*time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("claimed jobs = %d, want 2", len(jobs))
	}
	if jobs[0].ArticleID != second.ID {
		t.Fatalf("first claimed article = %d, want recheck article %d", jobs[0].ArticleID, second.ID)
	}
	if jobs[1].ArticleID != first.ID {
		t.Fatalf("second claimed article = %d, want %d", jobs[1].ArticleID, first.ID)
	}
}

// ── 入队数据库级幂等（add-ai-generated-article-detection task 3.5，与 tag_jobs 同型）──
//
// 并发口径（review R-7）：夹具是单连接 sqlite，goroutine 的写在驱动层串行执行，
// `…Repeatedly…` 用例验证的是「重复入队幂等」，不是真并发双插；真并发窗口由 PG
// 迁移用例（部分唯一索引拒重复）与冲突分支用例（TestInsertDetectionJob…）覆盖。

// TestInsertDetectionJobRidesConflictPathWhenActiveJobExists 直接驱动插入分支的
// 撞索引路径：目标文章已有活动作业（等价于并发事务抢先插入并提交后的状态）时，
// INSERT ... ON CONFLICT DO NOTHING 返回零行，随即按「已存在」分支同语义原地更新
// ——不报错、不产生第二条未完成作业。
func TestInsertDetectionJobRidesConflictPathWhenActiveJobExists(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "conflict")

	active := models.DetectionJob{
		ArticleID: article.ID, Status: string(models.JobStatusPending), MaxAttempts: 5,
		AvailableAt: time.Now(), Reason: DetectionJobReasonArticleIngested,
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatalf("seed active job: %v", err)
	}

	req := DetectionJobRequest{
		ArticleID: article.ID,
		Reason:    DetectionJobReasonManualRecheck,
		Priority:  10,
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return insertDetectionJob(tx, req, time.Now())
	}); err != nil {
		t.Fatalf("insertDetectionJob must ride the conflict path, got error: %v", err)
	}

	var jobs []models.DetectionJob
	if err := db.Where("article_id = ?", article.ID).Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("detection jobs = %d, want 1 (conflict must reuse the active row)", len(jobs))
	}
	if jobs[0].ID != active.ID {
		t.Fatalf("job id = %d, want the existing active row %d reused", jobs[0].ID, active.ID)
	}
	if jobs[0].Reason != DetectionJobReasonManualRecheck || jobs[0].Priority != 10 {
		t.Fatalf("in-place update missed: reason=%q priority=%d", jobs[0].Reason, jobs[0].Priority)
	}
}

// TestEnqueueDetectionJobRepeatedlyKeepsOneActiveJob 连发 8 次入队（抓取完成 /
// 抓取失败 / 入库即判定可能同秒落到同篇文章）：落库结果必须只有一条未完成作业，
// 且每次入队都成功（复用而非报错）。
//
// 并发窗口说明（review R-7）：单连接串行执行，本用例验证「重复入队幂等」；
// 真并发窗口由 PG 迁移用例（TestDetectionJobUniqueMigration…）与冲突分支用例覆盖。
func TestEnqueueDetectionJobRepeatedlyKeepsOneActiveJob(t *testing.T) {
	db := setupDetectionJobQueueTestDB(t)
	article := seedDetectionArticle(t, db, "repeat")
	queue := NewDetectionJobQueue(db)

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- queue.Enqueue(DetectionJobRequest{
				ArticleID: article.ID,
				Reason:    DetectionJobReasonFirecrawlCompleted,
			})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("enqueue failed (must reuse, not error): %v", err)
		}
	}

	var active int64
	if err := db.Model(&models.DetectionJob{}).
		Where("article_id = ? AND status IN ?", article.ID, []string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Count(&active).Error; err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if active != 1 {
		t.Fatalf("active jobs = %d, want 1 (repeat enqueues must not double-insert)", active)
	}
}

// TestRideDetectionJobConflictDegradesWhenConflictRowTurnsTerminal 直测兜底分支
// （review R-3）：两次插入都撞索引、重查又都找不到活动作业（冲突行在「插入零行」
// 与「重查」之间连转两次终态），此时 SHALL 返回 nil（按已存在语义降级接受），
// SHALL NOT 把内部竞争报给调用方——判定入队失败本就不该阻断入库/抓取/打标。真库
// 内无法稳定构造这个微秒窗口（没有活动作业时下一次插入必然成功），所以用闭包缝
// 直接驱动循环。
func TestRideDetectionJobConflictDegradesWhenConflictRowTurnsTerminal(t *testing.T) {
	var inserts, lookups int
	err := rideDetectionJobConflict(detectionJobInsertOps{
		articleID: 9201,
		insert: func() (bool, error) {
			inserts++
			return false, nil
		},
		findActive: func() (models.DetectionJob, bool, error) {
			lookups++
			return models.DetectionJob{}, false, nil
		},
		reapply: func(models.DetectionJob) error {
			t.Fatal("reapply must not run when the re-check finds no active job")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("two conflicts without an active job must degrade to nil, got error: %v", err)
	}
	if inserts != 2 || lookups != 2 {
		t.Fatalf("attempts = %d inserts / %d lookups, want 2/2 (the full two-attempt loop must run before degrading)", inserts, lookups)
	}
}

// TestRideDetectionJobConflictReusesActiveJobOnRetry 守住循环的正向分支：第二次重查
// 找到活动作业时走原地更新，不再落到兜底告警。
func TestRideDetectionJobConflictReusesActiveJobOnRetry(t *testing.T) {
	var inserts, lookups, reapplied int
	active := models.DetectionJob{ArticleID: 9202, Status: string(models.JobStatusPending)}
	err := rideDetectionJobConflict(detectionJobInsertOps{
		articleID: 9202,
		insert: func() (bool, error) {
			inserts++
			return false, nil
		},
		findActive: func() (models.DetectionJob, bool, error) {
			lookups++
			if lookups < 2 {
				return models.DetectionJob{}, false, nil
			}
			return active, true, nil
		},
		reapply: func(job models.DetectionJob) error {
			reapplied++
			if job.ID != active.ID {
				t.Fatalf("reapplied job id = %d, want %d", job.ID, active.ID)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("retry that finds the active job must succeed, got: %v", err)
	}
	if inserts != 2 || lookups != 2 || reapplied != 1 {
		t.Fatalf("retry path = %d inserts / %d lookups / %d reapply, want 2/2/1", inserts, lookups, reapplied)
	}
}
