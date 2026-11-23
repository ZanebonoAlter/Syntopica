package repository

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func TestEnqueueTagJobUpgradesForceRetag(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	queue := NewTagJobQueue(Repo.DB())
	request := TagJobRequest{ArticleID: 42, FeedName: "Feed", ForceRetag: false}
	if err := queue.Enqueue(request); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	request.ForceRetag = true
	if err := queue.Enqueue(request); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	var jobs []models.TagJob
	if err := Repo.DB().Order("id asc").Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("job count = %d, want 1", len(jobs))
	}
	if !jobs[0].ForceRetag {
		t.Fatal("expected active job to be upgraded to force retag")
	}
}

// ── night-window-alignment B1-B3：lease 消费顺序（新任务优先）──
//
// 顺序契约：priority DESC → available_at ASC（退避语义）→ created_at DESC
//（同顺位内新→旧）。用例直接在 testcontainer PG 上驱动真实 Claim SQL。

// seedTagJobRaw inserts one tag job with an explicit created_at/available_at
// and returns its id.
func seedTagJobRaw(t *testing.T, articleID uint, status string, priority int, createdAt, availableAt time.Time) uint {
	t.Helper()
	job := models.TagJob{
		ArticleID:   articleID,
		Status:      status,
		Priority:    priority,
		AvailableAt: availableAt,
		CreatedAt:   createdAt,
	}
	if err := Repo.DB().Create(&job).Error; err != nil {
		t.Fatalf("seed tag job: %v", err)
	}
	// created_at is auto-populated by GORM on insert; pin it explicitly to
	// exercise the lease ordering (same convention as tag_queue_status_test).
	if err := Repo.DB().Model(&models.TagJob{}).Where("id = ?", job.ID).
		Update("created_at", createdAt).Error; err != nil {
		t.Fatalf("set created_at: %v", err)
	}
	return job.ID
}

// TestClaimOrderNewestFirst（B1，spec「新旧任务并存时新的先消费」）：昨日/今晨/
// 刚才三条 pending、priority 同为 0，lease 顺序必须 T2→T1→T0。
func TestClaimOrderNewestFirst(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	now := time.Now()
	t0 := seedTagJobRaw(t, 101, string(models.JobStatusPending), 0, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	t1 := seedTagJobRaw(t, 102, string(models.JobStatusPending), 0, now.Add(-8*time.Hour), now.Add(-8*time.Hour))
	t2 := seedTagJobRaw(t, 103, string(models.JobStatusPending), 0, now.Add(-time.Minute), now.Add(-time.Minute))

	jobs, err := Repo.NewTagJobQueue().Claim(3, time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("claimed %d jobs, want 3", len(jobs))
	}
	if jobs[0].ID != t2 || jobs[1].ID != t1 || jobs[2].ID != t0 {
		t.Fatalf("lease order = [%d %d %d], want newest-first [%d %d %d]",
			jobs[0].ID, jobs[1].ID, jobs[2].ID, t2, t1, t0)
	}
}

// TestClaimPriorityPreemptsFreshness（B2，spec「高优先级旧任务可插队」）：旧任务
// priority=10 先于新任务 priority=0。
func TestClaimPriorityPreemptsFreshness(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	now := time.Now()
	oldHigh := seedTagJobRaw(t, 201, string(models.JobStatusPending), 10, now.Add(-72*time.Hour), now.Add(-72*time.Hour))
	newLow := seedTagJobRaw(t, 202, string(models.JobStatusPending), 0, now.Add(-time.Minute), now.Add(-time.Minute))

	jobs, err := Repo.NewTagJobQueue().Claim(2, time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != oldHigh || jobs[1].ID != newLow {
		t.Fatalf("lease order = %v, want high-priority old first [%d %d]",
			jobIDs(jobs), oldHigh, newLow)
	}
}

// TestClaimSkipsBackoffJob（B3，spec「退避中的任务不被 lease」）：最新任务
// available_at 在未来时不被 lease，轮到下一顺位（同顺位内仍新→旧）。
func TestClaimSkipsBackoffJob(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	now := time.Now()
	backoff := seedTagJobRaw(t, 301, string(models.JobStatusPending), 0, now.Add(-time.Minute), now.Add(time.Hour))
	mid := seedTagJobRaw(t, 302, string(models.JobStatusPending), 0, now.Add(-8*time.Hour), now.Add(-8*time.Hour))
	oldest := seedTagJobRaw(t, 303, string(models.JobStatusPending), 0, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	jobs, err := Repo.NewTagJobQueue().Claim(3, time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("claimed %d jobs, want 2 (backoff job must not be leased)", len(jobs))
	}
	if jobs[0].ID != mid || jobs[1].ID != oldest {
		t.Fatalf("lease order = %v, want [%d %d]", jobIDs(jobs), mid, oldest)
	}
	var got models.TagJob
	if err := Repo.DB().First(&got, backoff).Error; err != nil {
		t.Fatalf("reload backoff job: %v", err)
	}
	if got.Status != string(models.JobStatusPending) {
		t.Fatalf("backoff job status = %q, want pending", got.Status)
	}
}

func jobIDs(jobs []models.TagJob) []uint {
	ids := make([]uint, 0, len(jobs))
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	return ids
}

// ── MarkFailed 两分支（retire-heuristic-tag-fallback：失败语义的唯一落点）──
//
// 本 change 把「提取失败」从「吞掉 + 写泛词」改成「上抛 + 任务失败」。任务失败
// 之后能不能被自动重试、重试次数耗尽后会不会停在终态，全由 MarkFailed 决定——
// 此前它零用例守护，是这条契约的单点。

// attempt < max_attempts：回到 pending 并推后 available_at（退避），不丢任务。
func TestMarkFailedBelowMaxGoesBackToPendingWithBackoff(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	availableAt := time.Now().Add(-time.Hour)
	jobID := seedTagJobRaw(t, 401, string(models.JobStatusPending), 0, time.Now(), availableAt)
	var job models.TagJob
	if err := Repo.DB().First(&job, jobID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	job.AttemptCount = 1
	job.MaxAttempts = 5

	if err := Repo.NewTagJobQueue().MarkFailed(job, "parse extraction result failed", 2*time.Minute); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var got models.TagJob
	if err := Repo.DB().First(&got, jobID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != string(models.JobStatusPending) {
		t.Fatalf("status = %q, want pending（未达上限必须可重试）", got.Status)
	}
	if got.LastError != "parse extraction result failed" {
		t.Fatalf("last_error = %q, want the failure reason preserved", got.LastError)
	}
	if !got.AvailableAt.After(availableAt) {
		t.Fatalf("available_at = %v, want pushed past the original %v（退避）", got.AvailableAt, availableAt)
	}
	if got.LeasedAt != nil || got.LeaseExpiresAt != nil {
		t.Fatalf("lease must be released on failure, got leased_at=%v lease_expires_at=%v", got.LeasedAt, got.LeaseExpiresAt)
	}

	// 退避期内不得被再次 lease。
	if jobs, err := Repo.NewTagJobQueue().Claim(10, time.Minute); err != nil {
		t.Fatalf("claim: %v", err)
	} else if len(jobs) != 0 {
		t.Fatalf("claimed %d job(s) during backoff window, want 0", len(jobs))
	}
}

// attempt >= max_attempts：停在 failed 终态（不再被 lease），失败原因可读——
// 这是「失败不得被伪装成成功」的最终保障。
func TestMarkFailedAtMaxAttemptsStopsAtFailed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	jobID := seedTagJobRaw(t, 402, string(models.JobStatusPending), 0, time.Now(), time.Now().Add(-time.Hour))
	var job models.TagJob
	if err := Repo.DB().First(&job, jobID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	job.AttemptCount = 5
	job.MaxAttempts = 5

	if err := Repo.NewTagJobQueue().MarkFailed(job, "merged extraction object has neither event_person_tags nor keyword_tags keys", 8*time.Minute); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var got models.TagJob
	if err := Repo.DB().First(&got, jobID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != string(models.JobStatusFailed) {
		t.Fatalf("status = %q, want failed（达上限必须停在终态）", got.Status)
	}
	if got.LastError == "" {
		t.Fatal("last_error must keep the failure reason so the queue panel can show it")
	}

	// 终态任务不得再被 lease。
	if jobs, err := Repo.NewTagJobQueue().Claim(10, time.Minute); err != nil {
		t.Fatalf("claim: %v", err)
	} else if len(jobs) != 0 {
		t.Fatalf("claimed %d failed job(s), want 0", len(jobs))
	}
}

// ── 入队数据库级幂等（fix-feed-refresh-churn D4）──
//
// 「同文章至多一条未完成作业」原来是应用层 check-then-insert 的约定，并发回调在
// read committed 下关不掉这个窗口（文章 178599 同秒两条 pending 实证）。以下是
// 数据库级兜底的两面：撞索引分支的复用语义，以及重复入队后的至多一条。
//
// 并发口径（review R-7）：夹具 `SetMaxOpenConns(1)` 把 goroutine 的写串行化在驱动
// 层，所以 `…Repeatedly…` 用例验证的是「重复入队幂等」，不是真并发双插；真并发
// 窗口由 PG 迁移用例（部分唯一索引拒重复）与冲突分支用例（TestInsertTagJob…）
// 覆盖。
//
// 夹具说明：sqlite 测试库由 AutoMigrate 建模，不走版本化迁移，因此这里按生产迁移
// 20261120_0002 的同一份 DDL 建出部分唯一索引——被验证的正是「索引存在时」的行为。

func setupTagJobQueueSQLite(t *testing.T) *gorm.DB {
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

	InitRepository(db)
	if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TagJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending', 'leased')`).Error; err != nil {
		t.Fatalf("create partial unique index: %v", err)
	}
	return db
}

// TestInsertTagJobRidesConflictPathWhenActiveJobExists 直接驱动插入分支的撞索引
// 路径：目标文章已有活动作业（等价于并发事务抢先插入并提交后的状态）时，
// INSERT ... ON CONFLICT DO NOTHING 返回零行，随即按「已存在」分支同语义原地更新
// ——不报错、不产生第二条未完成作业。
func TestInsertTagJobRidesConflictPathWhenActiveJobExists(t *testing.T) {
	setupTagJobQueueSQLite(t)

	active := models.TagJob{
		ArticleID: 178599, Status: string(models.JobStatusPending), MaxAttempts: 5,
		AvailableAt: time.Now(), FeedNameSnapshot: "旧源", Reason: "article_created",
	}
	if err := Repo.DB().Create(&active).Error; err != nil {
		t.Fatalf("seed active job: %v", err)
	}

	req := TagJobRequest{
		ArticleID: 178599, FeedName: "新源", CategoryName: "新分类",
		ForceRetag: true, Reason: "summary_completed",
	}
	if err := Repo.DB().Transaction(func(tx *gorm.DB) error {
		return insertTagJob(tx, req, time.Now())
	}); err != nil {
		t.Fatalf("insertTagJob must ride the conflict path, got error: %v", err)
	}

	var jobs []models.TagJob
	if err := Repo.DB().Where("article_id = ?", req.ArticleID).Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("job rows = %d, want 1 (conflict must reuse the active row)", len(jobs))
	}
	if jobs[0].ID != active.ID {
		t.Fatalf("job id = %d, want the existing active row %d reused", jobs[0].ID, active.ID)
	}
	if !jobs[0].ForceRetag || jobs[0].FeedNameSnapshot != "新源" || jobs[0].CategoryNameSnapshot != "新分类" || jobs[0].Reason != "summary_completed" {
		t.Fatalf("in-place update missed: force_retag=%v feed=%q category=%q reason=%q",
			jobs[0].ForceRetag, jobs[0].FeedNameSnapshot, jobs[0].CategoryNameSnapshot, jobs[0].Reason)
	}
}

// TestEnqueueTagJobRepeatedlyKeepsOneActiveJob 连发 8 次入队（同一秒多条回调的
// 顺序等价形态）：不管其中多少次走到插入分支，落库结果必须只有一条未完成作业，
// 且每次入队都成功（复用而非报错）。
//
// 并发窗口说明（review R-7）：夹具是单连接，8 个 goroutine 在驱动层串行执行，
// 本用例验证的是「重复入队幂等」；真并发双插窗口由 PG 迁移用例
// （TestTagJobUniqueMigration…）与冲突分支用例覆盖。
func TestEnqueueTagJobRepeatedlyKeepsOneActiveJob(t *testing.T) {
	setupTagJobQueueSQLite(t)

	queue := NewTagJobQueue(Repo.DB())
	const articleID = 178600

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- queue.Enqueue(TagJobRequest{
				ArticleID: articleID,
				FeedName:  fmt.Sprintf("feed-%d", i),
				Reason:    "article_created",
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
	if err := Repo.DB().Model(&models.TagJob{}).
		Where("article_id = ? AND status IN ?", articleID, []string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Count(&active).Error; err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if active != 1 {
		t.Fatalf("active jobs = %d, want 1 (repeat enqueues must not double-insert)", active)
	}
}

// TestRideTagJobConflictDegradesWhenConflictRowTurnsTerminal 直测兜底分支（review
// R-3）：两次插入都撞索引、重查又都找不到活动作业（冲突行在「插入零行」与「重查」
// 之间连转两次终态），此时 SHALL 返回 nil（按已存在语义降级接受），SHALL NOT 把
// 内部竞争报给调用方。真库内无法稳定构造这个微秒窗口——没有活动作业时下一次插入
// 必然成功——所以用闭包缝直接驱动循环。
func TestRideTagJobConflictDegradesWhenConflictRowTurnsTerminal(t *testing.T) {
	var inserts, lookups int
	err := rideTagJobConflict(tagJobInsertOps{
		articleID: 178602,
		insert: func() (bool, error) {
			inserts++
			return false, nil
		},
		findActive: func() (models.TagJob, bool, error) {
			lookups++
			return models.TagJob{}, false, nil
		},
		reapply: func(models.TagJob) error {
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

// TestRideTagJobConflictReusesActiveJobOnRetry 守住循环的正向分支：第二次重查找到
// 活动作业时走原地更新，不再落到兜底告警。
func TestRideTagJobConflictReusesActiveJobOnRetry(t *testing.T) {
	var inserts, lookups, reapplied int
	active := models.TagJob{ArticleID: 178603, Status: string(models.JobStatusPending)}
	err := rideTagJobConflict(tagJobInsertOps{
		articleID: 178603,
		insert: func() (bool, error) {
			inserts++
			return false, nil
		},
		findActive: func() (models.TagJob, bool, error) {
			lookups++
			if lookups < 2 {
				return models.TagJob{}, false, nil
			}
			return active, true, nil
		},
		reapply: func(job models.TagJob) error {
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

// TestEnqueueTagJobUpgradesActiveJobInPlace 走「已存在」分支（非撞索引）：二次入队
// 不新建行，按新请求刷新 reason / 快照，force_retag 只升不降。Enqueue 拆成
// find/update 两个辅助函数后由本用例守住这条既有契约。
func TestEnqueueTagJobUpgradesActiveJobInPlace(t *testing.T) {
	setupTagJobQueueSQLite(t)

	queue := NewTagJobQueue(Repo.DB())
	const articleID = 178601

	if err := queue.Enqueue(TagJobRequest{
		ArticleID: articleID, FeedName: "旧源", CategoryName: "旧分类", Reason: "article_created",
	}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := queue.Enqueue(TagJobRequest{
		ArticleID: articleID, FeedName: "新源", CategoryName: "新分类", ForceRetag: true, Reason: "summary_completed",
	}); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	var jobs []models.TagJob
	if err := Repo.DB().Where("article_id = ?", articleID).Find(&jobs).Error; err != nil {
		t.Fatalf("load jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("job rows = %d, want 1 (an active job must be updated in place)", len(jobs))
	}
	got := jobs[0]
	if !got.ForceRetag || got.FeedNameSnapshot != "新源" || got.CategoryNameSnapshot != "新分类" || got.Reason != "summary_completed" {
		t.Fatalf("in-place update missed: force_retag=%v feed=%q category=%q reason=%q",
			got.ForceRetag, got.FeedNameSnapshot, got.CategoryNameSnapshot, got.Reason)
	}
}
