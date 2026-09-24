package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	adminrepo "syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/database"
	content "syntopica-backend/internal/reader"
	readerservice "syntopica-backend/internal/reader/service"
)

// fakeCrawler implements content.Crawler for tests: configurable per-URL
// success/failure, optional latency, and in-flight concurrency tracking so
// tests can assert workers actually run in parallel.
type fakeCrawler struct {
	mu    sync.Mutex
	calls map[string]int
	// fail reports whether the given URL should simulate a crawl error.
	fail func(url string) bool
	// delay is how long each ScrapePage call takes (simulates target-site latency).
	delay time.Duration
	// cur is the current number of in-flight ScrapePage calls.
	cur int32
	// peak records the maximum observed cur value.
	peak int32
}

func newFakeCrawler(delay time.Duration, fail func(url string) bool) *fakeCrawler {
	return &fakeCrawler{
		calls: map[string]int{},
		fail:  fail,
		delay: delay,
	}
}

func (c *fakeCrawler) ScrapePage(_ context.Context, url string) (*readerservice.ScrapeResult, error) {
	c.mu.Lock()
	c.calls[url]++
	c.mu.Unlock()

	cur := atomic.AddInt32(&c.cur, 1)
	for {
		peak := atomic.LoadInt32(&c.peak)
		if cur <= peak || atomic.CompareAndSwapInt32(&c.peak, peak, cur) {
			break
		}
	}
	time.Sleep(c.delay)
	atomic.AddInt32(&c.cur, -1)

	if c.fail != nil && c.fail(url) {
		return nil, fmt.Errorf("simulated crawl failure for %s", url)
	}
	return &readerservice.ScrapeResult{
		Markdown: "content for " + url,
		Title:    url,
		OGImage:  "https://example.com/og.png",
	}, nil
}

func (c *fakeCrawler) callCount(url string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[url]
}

func (c *fakeCrawler) peakInFlight() int32 {
	return atomic.LoadInt32(&c.peak)
}

// setupFirecrawlJobTest wires a single-connection in-memory sqlite into both
// the platform database global (read by GetFirecrawlConfig) and the admin
// repository singleton (read by the job for article/feed lookups), seeds the
// firecrawl settings row, and shrinks the worker rate-limit sleep so tests
// stay fast. Single connection (MaxOpenConns=1) serializes concurrent worker
// writes at the driver level, avoiding shared-cache table deadlocks.
func setupFirecrawlJobTest(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	prevPlatformDB := database.DB
	prevRepo := adminrepo.Repo
	database.DB = db
	adminrepo.InitRepository(db)
	t.Cleanup(func() {
		database.DB = prevPlatformDB
		adminrepo.Repo = prevRepo
	})

	if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.FirecrawlJob{}, &models.TagJob{}, &models.AISettings{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	// GetFirecrawlConfig reads this row; enabled=true so the job actually runs.
	if err := db.Create(&models.AISettings{Key: "firecrawl_config", Value: `{"enabled":true,"timeout":60}`}).Error; err != nil {
		t.Fatalf("seed firecrawl config: %v", err)
	}

	prevRateLimit := firecrawlRateLimit
	firecrawlRateLimit = 2 * time.Millisecond
	t.Cleanup(func() { firecrawlRateLimit = prevRateLimit })

	return db
}

func seedFirecrawlArticles(t *testing.T, db *gorm.DB, queue *content.FirecrawlJobQueue, n int, failOdd bool) (feed models.Feed, links []string) {
	t.Helper()

	feed = models.Feed{
		Title:                 "Firecrawl Test Feed",
		URL:                   fmt.Sprintf("https://example.com/feed-%d", time.Now().UnixNano()),
		ArticleSummaryEnabled: true,
		CompletionOnRefresh:   true,
		TaggingEnabled:        true,
	}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	for i := 0; i < n; i++ {
		link := fmt.Sprintf("https://example.com/article/%d", i)
		if failOdd && i%2 == 1 {
			link = fmt.Sprintf("https://example.com/fail/%d", i)
		}
		article := models.Article{
			FeedID:          feed.ID,
			Title:           fmt.Sprintf("Article %d", i),
			Link:            link,
			FirecrawlStatus: "pending",
		}
		if err := db.Create(&article).Error; err != nil {
			t.Fatalf("create article %d: %v", i, err)
		}
		if err := queue.Enqueue(article); err != nil {
			t.Fatalf("enqueue article %d: %v", i, err)
		}
		links = append(links, link)
	}
	return feed, links
}

// Test 1 (task 2.2): concurrent counting correctness — a mixed success/failure
// batch must end with completed+failed == total, every job processed exactly
// once, and per-job terminal states matching the crawl outcome.
func TestFirecrawlJobParallelCountingAndJobStates(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	_, links := seedFirecrawlArticles(t, db, queue, 9, true)

	crawler := newFakeCrawler(5*time.Millisecond, func(url string) bool {
		return len(url) >= len("https://example.com/fail/") && url[:len("https://example.com/fail/")] == "https://example.com/fail/"
	})

	res, err := firecrawlJobWithCrawler(queue, "test-batch", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	if err != nil {
		t.Fatalf("job run: %v", err)
	}

	completed := res.Data["completed"].(int)
	failed := res.Data["failed"].(int)
	total := res.Data["total"].(int)
	if total != 9 {
		t.Fatalf("total = %d, want 9", total)
	}
	if completed+failed != total {
		t.Fatalf("completed(%d) + failed(%d) != total(%d)", completed, failed, total)
	}
	if completed != 5 || failed != 4 {
		t.Fatalf("completed = %d, failed = %d; want 5/4", completed, failed)
	}

	// Every job processed exactly once.
	for _, link := range links {
		if got := crawler.callCount(link); got != 1 {
			t.Fatalf("url %s crawled %d times, want exactly 1", link, got)
		}
	}

	// Successful jobs → status completed; article marked completed with content.
	var completedJobs []models.FirecrawlJob
	if err := db.Where("status = ?", string(models.JobStatusCompleted)).Find(&completedJobs).Error; err != nil {
		t.Fatalf("query completed jobs: %v", err)
	}
	if len(completedJobs) != 5 {
		t.Fatalf("completed jobs in db = %d, want 5", len(completedJobs))
	}
	for _, job := range completedJobs {
		var art models.Article
		if err := db.First(&art, job.ArticleID).Error; err != nil {
			t.Fatalf("load article %d: %v", job.ArticleID, err)
		}
		if art.FirecrawlStatus != "completed" {
			t.Fatalf("article %d firecrawl_status = %q, want completed", art.ID, art.FirecrawlStatus)
		}
		if art.FirecrawlContent == "" {
			t.Fatalf("article %d has empty firecrawl_content", art.ID)
		}
		if art.SummaryStatus != "incomplete" {
			t.Fatalf("article %d summary_status = %q, want incomplete (feed enables summary)", art.ID, art.SummaryStatus)
		}
		// Retag enqueued after successful crawl.
		var tagJobCount int64
		if err := db.Model(&models.TagJob{}).Where("article_id = ? AND reason = ?", art.ID, "firecrawl_completed").Count(&tagJobCount).Error; err != nil {
			t.Fatalf("count tag jobs: %v", err)
		}
		if tagJobCount != 1 {
			t.Fatalf("article %d firecrawl_completed tag jobs = %d, want 1", art.ID, tagJobCount)
		}
	}

	// Failed (non-terminal) jobs → back to pending with future availability.
	var pendingJobs []models.FirecrawlJob
	if err := db.Where("status = ?", string(models.JobStatusPending)).Find(&pendingJobs).Error; err != nil {
		t.Fatalf("query pending jobs: %v", err)
	}
	if len(pendingJobs) != 4 {
		t.Fatalf("pending jobs in db = %d, want 4", len(pendingJobs))
	}
	for _, job := range pendingJobs {
		var art models.Article
		if err := db.First(&art, job.ArticleID).Error; err != nil {
			t.Fatalf("load article %d: %v", job.ArticleID, err)
		}
		if art.FirecrawlStatus != "failed" {
			t.Fatalf("article %d firecrawl_status = %q, want failed", art.ID, art.FirecrawlStatus)
		}
		if !job.AvailableAt.After(time.Now()) {
			t.Fatalf("job %d available_at %v not in future (backoff missing)", job.ID, job.AvailableAt)
		}
	}
}

// Test 2 (task 2.2): workers genuinely run concurrently — with 3 workers and
// a crawler whose calls take longer than the rate-limit sleep, peak in-flight
// ScrapePage calls must exceed 1.
func TestFirecrawlJobWorkersRunConcurrently(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	seedFirecrawlArticles(t, db, queue, 6, false)

	crawler := newFakeCrawler(150*time.Millisecond, nil)

	res, err := firecrawlJobWithCrawler(queue, "test-batch", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	if err != nil {
		t.Fatalf("job run: %v", err)
	}

	completed := res.Data["completed"].(int)
	failed := res.Data["failed"].(int)
	if completed+failed != 6 {
		t.Fatalf("completed(%d) + failed(%d) != 6", completed, failed)
	}

	if peak := crawler.peakInFlight(); peak < 2 {
		t.Fatalf("peak in-flight crawls = %d, want >= 2 (workers did not overlap)", peak)
	}
}

// Test 3 (task 2.2): terminal failure path — when a crawl fails at max
// attempts, the article falls back to RSS content: summary_status flips to
// incomplete and a fallback retag job is enqueued, same as serial behavior.
func TestFirecrawlJobTerminalFailureFallsBackToRSS(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	_, links := seedFirecrawlArticles(t, db, queue, 1, true)

	// Bump attempt count so the claim pushes it to max attempts → terminal.
	if err := db.Model(&models.FirecrawlJob{}).Where("article_id IS NOT NULL").Update("attempt_count", 4).Error; err != nil {
		t.Fatalf("bump attempt_count: %v", err)
	}

	crawler := newFakeCrawler(5*time.Millisecond, func(url string) bool { return true })

	res, err := firecrawlJobWithCrawler(queue, "test-batch", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	if err != nil {
		t.Fatalf("job run: %v", err)
	}

	if failed := res.Data["failed"].(int); failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}

	var job models.FirecrawlJob
	if err := db.Where("url_snapshot = ?", links[0]).First(&job).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != string(models.JobStatusFailed) {
		t.Fatalf("terminal job status = %q, want failed", job.Status)
	}

	var art models.Article
	if err := db.First(&art, job.ArticleID).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}
	if art.SummaryStatus != "incomplete" {
		t.Fatalf("article summary_status = %q, want incomplete after terminal crawl failure", art.SummaryStatus)
	}
	if art.FirecrawlStatus != "failed" {
		t.Fatalf("article firecrawl_status = %q, want failed", art.FirecrawlStatus)
	}

	var fallbackTagJobs int64
	if err := db.Model(&models.TagJob{}).Where("article_id = ? AND reason = ?", art.ID, "firecrawl_failed_fallback").Count(&fallbackTagJobs).Error; err != nil {
		t.Fatalf("count fallback tag jobs: %v", err)
	}
	if fallbackTagJobs != 1 {
		t.Fatalf("fallback retag jobs = %d, want 1", fallbackTagJobs)
	}
}

// Gate semantics (unify-feed-summary-toggles): with article_summary_enabled
// on but completion_on_refresh off, neither the success path nor the terminal
// failure path may mark summary_status=incomplete; tagging fallback is
// unaffected by the gate.
func TestFirecrawlJobCompletionGateOffSkipsSummaryMarking(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	feed, links := seedFirecrawlArticles(t, db, queue, 1, false)

	// Main toggle on, auto gate off (manual mode).
	if err := db.Model(&models.Feed{}).Where("id = ?", feed.ID).Update("completion_on_refresh", false).Error; err != nil {
		t.Fatalf("gate off: %v", err)
	}

	// Reload feed so the job sees the gate value (job loads feed fresh per
	// batch; simulate by reading back and asserting the persisted value).
	var fresh models.Feed
	if err := db.First(&fresh, feed.ID).Error; err != nil {
		t.Fatalf("reload feed: %v", err)
	}
	if fresh.CompletionOnRefresh {
		t.Fatalf("completion_on_refresh should be false")
	}

	crawler := newFakeCrawler(5*time.Millisecond, nil)

	res, err := firecrawlJobWithCrawler(queue, "test-batch", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	if err != nil {
		t.Fatalf("job run: %v", err)
	}
	if completed := res.Data["completed"].(int); completed != 1 {
		t.Fatalf("completed = %d, want 1", completed)
	}

	var art models.Article
	if err := db.Where("link = ?", links[0]).First(&art).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}
	// BF-2: success path leaves summary_status untouched (seeded "").
	if art.SummaryStatus == "incomplete" {
		t.Fatalf("summary_status = incomplete, want untouched when gate off")
	}
	if art.FirecrawlStatus != "completed" {
		t.Fatalf("firecrawl_status = %q, want completed", art.FirecrawlStatus)
	}
	// RG-2: tagging still enqueued after successful crawl.
	var tagJobCount int64
	if err := db.Model(&models.TagJob{}).Where("article_id = ? AND reason = ?", art.ID, "firecrawl_completed").Count(&tagJobCount).Error; err != nil {
		t.Fatalf("count tag jobs: %v", err)
	}
	if tagJobCount != 1 {
		t.Fatalf("article %d firecrawl_completed tag jobs = %d, want 1 (gate must not affect tagging)", art.ID, tagJobCount)
	}
}

// BF-5: terminal crawl failure with gate off — summary_status untouched,
// fallback retag still enqueued.
func TestFirecrawlJobTerminalFailureGateOffSkipsSummaryMarking(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	feed, links := seedFirecrawlArticles(t, db, queue, 1, true)

	if err := db.Model(&models.Feed{}).Where("id = ?", feed.ID).Update("completion_on_refresh", false).Error; err != nil {
		t.Fatalf("gate off: %v", err)
	}

	// Push to terminal attempts.
	if err := db.Model(&models.FirecrawlJob{}).Where("article_id IS NOT NULL").Update("attempt_count", 4).Error; err != nil {
		t.Fatalf("bump attempt_count: %v", err)
	}

	crawler := newFakeCrawler(5*time.Millisecond, func(url string) bool { return true })

	res, err := firecrawlJobWithCrawler(queue, "test-batch", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	if err != nil {
		t.Fatalf("job run: %v", err)
	}
	if failed := res.Data["failed"].(int); failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}

	var art models.Article
	if err := db.Where("link = ?", links[0]).First(&art).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}
	if art.SummaryStatus == "incomplete" {
		t.Fatalf("summary_status = incomplete, want untouched when gate off (terminal failure)")
	}
	if art.FirecrawlStatus != "failed" {
		t.Fatalf("firecrawl_status = %q, want failed", art.FirecrawlStatus)
	}

	var fallbackTagJobs int64
	if err := db.Model(&models.TagJob{}).Where("article_id = ? AND reason = ?", art.ID, "firecrawl_failed_fallback").Count(&fallbackTagJobs).Error; err != nil {
		t.Fatalf("count fallback tag jobs: %v", err)
	}
	if fallbackTagJobs != 1 {
		t.Fatalf("fallback retag jobs = %d, want 1 (gate must not affect tagging fallback)", fallbackTagJobs)
	}
}

// TestFirecrawlCompletionEnqueuesTagJobsWhilePaused（A3，night-window-alignment）：
// 暂停态下 firecrawl 完成回调只落状态位 + tag_jobs 照常入队（pending 等待），
// 不发起任何 LLM 调用——enqueue 非 LLM 工作，worker 暂停天然不消费。
func TestFirecrawlCompletionEnqueuesTagJobsWhilePaused(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	seedFirecrawlArticles(t, db, queue, 2, false)

	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })

	res, err := firecrawlJobWithCrawler(queue, "paused-downstream", func(*content.FirecrawlConfig) content.Crawler {
		return newFakeCrawler(time.Millisecond, nil)
	})(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 2, res.Data["completed"].(int), "crawl itself must complete while paused")

	// 抓取结果照常落 firecrawl 状态位。
	var completedArticles int64
	require.NoError(t, db.Model(&models.Article{}).Where("firecrawl_status = ?", "completed").Count(&completedArticles).Error)
	require.EqualValues(t, 2, completedArticles)

	// 下游 tag_jobs 照常入队且保持 pending：无 worker 消费、无 LLM 路径触发
	//（ai_call_logs 不新增行的结构性前提——本测试进程内没有任何打标/嵌入调用）。
	var tagJobs []models.TagJob
	require.NoError(t, db.Find(&tagJobs).Error)
	require.Len(t, tagJobs, 2)
	for _, job := range tagJobs {
		require.Equal(t, string(models.JobStatusPending), job.Status, "tag job must stay pending while paused")
		require.Equal(t, "firecrawl_completed", job.Reason)
		require.True(t, job.ForceRetag)
	}
}
