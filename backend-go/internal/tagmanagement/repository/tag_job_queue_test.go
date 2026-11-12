package repository

import (
	"testing"
	"time"

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
