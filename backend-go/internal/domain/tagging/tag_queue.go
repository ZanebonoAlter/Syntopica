package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/platform/ws"
)

const drainTimeout = 10 * time.Second

type TagQueue struct {
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started      bool
	mu           sync.Mutex
	queue        *TagJobQueue
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	concurrency  int
}

var (
	instance *TagQueue
	once     sync.Once
)

func GetTagQueue() *TagQueue {
	once.Do(func() {
		instance = &TagQueue{
			stopChan:     make(chan struct{}),
			queue:        NewTagJobQueue(database.DB),
			pollInterval: time.Second,
			lease:        10 * time.Minute,
			batchSize:    20,
			concurrency:  3,
		}
	})
	if instance.queue == nil {
		instance.queue = NewTagJobQueue(database.DB)
	}
	return instance
}

func (q *TagQueue) Enqueue(articleID uint, feedName, categoryName string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.started {
		return fmt.Errorf("tag queue is not started")
	}

	return q.queue.Enqueue(TagJobRequest{
		ArticleID:    articleID,
		FeedName:     feedName,
		CategoryName: categoryName,
		Reason:       "article_created",
	})
}

func (q *TagQueue) EnqueueAsync(articleID uint, feedName, categoryName string) {
	if err := q.Enqueue(articleID, feedName, categoryName); err != nil {
		logging.Warnf("Failed to enqueue tag job for article %d: %v", articleID, err)
	}
}

func (q *TagQueue) Start() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.started {
		return fmt.Errorf("tag queue already started")
	}

	q.stopChan = make(chan struct{})
	err := q.tryStart()
	if err == nil {
		logging.Infof("Tag queue started successfully")
		return nil
	}

	logging.Warnf("TagQueue initial start failed: %v, retrying in background", err)
	go q.backgroundRetry()

	return nil // 立即返回，不阻塞应用启动
}

func (q *TagQueue) tryStart() error {
	if database.DB == nil {
		return fmt.Errorf("database is not initialized")
	}

	sqlDB, err := database.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	queue := NewTagJobQueue(database.DB)
	if queue == nil {
		return fmt.Errorf("failed to create tag job queue")
	}

	q.queue = queue
	q.started = true
	q.wg.Add(1)
	go q.worker()
	return nil
}

func (q *TagQueue) backgroundRetry() {
	const (
		maxRetries    = 10
		retryInterval = 30 * time.Second
	)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-q.stopChan:
			return
		case <-time.After(retryInterval):
		}

		q.mu.Lock()
		if q.started {
			q.mu.Unlock()
			return
		}

		err := q.tryStart()
		if err == nil {
			q.mu.Unlock()
			logging.Infof("TagQueue started after %d retry attempts", attempt)
			return
		}

		q.mu.Unlock()
		logging.Infof("TagQueue retry attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logging.Errorf("TagQueue failed to start after %d attempts, giving up", maxRetries)
}

func (q *TagQueue) Stop() {
	q.mu.Lock()
	if !q.started {
		select {
		case <-q.stopChan:
		default:
			close(q.stopChan)
		}
		q.mu.Unlock()
		return
	}
	q.started = false
	close(q.stopChan)
	q.mu.Unlock()

	q.wg.Wait()
	logging.Infof("Tag queue stopped")
}

func (q *TagQueue) worker() {
	defer q.wg.Done()

	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopChan:
			q.drainRemaining()
			return
		case <-ticker.C:
			q.processAvailableJobs()
		}
	}
}

func (q *TagQueue) drainRemaining() {
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			logging.Infof("Tag queue drain timed out after %v, remaining jobs will be processed on next start", drainTimeout)
			return
		default:
		}

		jobs, err := q.queue.Claim(q.batchSize, q.lease)
		if err != nil {
			logging.Warnf("Failed to claim tag jobs during drain: %v", err)
			return
		}
		if len(jobs) == 0 {
			return
		}

		sem := make(chan struct{}, q.concurrency)
		var jobWg sync.WaitGroup

		for _, job := range jobs {
			if ctx.Err() != nil {
				logging.Infof("Tag queue drain timed out, some job(s) remaining for next start")
				jobWg.Wait()
				return
			}
			jobWg.Add(1)
			sem <- struct{}{}
			go func(j models.TagJob) {
				defer func() { <-sem; jobWg.Done() }()
				q.processJob(context.Background(), j)
			}(job)
		}
		jobWg.Wait()
	}
}

func (q *TagQueue) processAvailableJobs() {
	jobs, err := q.queue.Claim(q.batchSize, q.lease)
	if err != nil {
		logging.Warnf("Failed to claim tag jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	sem := make(chan struct{}, q.concurrency)
	var jobWg sync.WaitGroup

	for _, job := range jobs {
		jobWg.Add(1)
		sem <- struct{}{}
		go func(j models.TagJob) {
			defer func() { <-sem; jobWg.Done() }()
			q.processJob(context.Background(), j)
		}(job)
	}
	jobWg.Wait()
}

func (q *TagQueue) processJob(ctx context.Context, job models.TagJob) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			logging.Errorf("Panic in tag job for article %d: %v", job.ArticleID, r)
			_ = q.queue.MarkFailed(job, msg, q.failureBackoff(job.AttemptCount))
			q.broadcastTagFailed(job.ID, job.ArticleID, msg)
		}
	}()

	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "workflow.article_tagging")
	defer span.End()
	span.SetAttributes(
		attribute.String("workflow.name", "article_tagging"),
		attribute.String("workflow.domain", "tag_management"),
		attribute.String("workflow.trigger", "article_created"),
	)
	m1, _ := baggage.NewMember("workflow.name", "article_tagging")
	m2, _ := baggage.NewMember("workflow.domain", "tag_management")
	m3, _ := baggage.NewMember("workflow.trigger", "article_created")
	bag, _ := baggage.New(m1, m2, m3)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	var article models.Article
	if err := database.DB.First(&article, job.ArticleID).Error; err != nil {
		logging.Warnf("Failed to fetch article %d for tagging: %v", job.ArticleID, err)
		_ = q.queue.MarkFailed(job, err.Error(), q.failureBackoff(job.AttemptCount))
		q.broadcastTagFailed(job.ID, job.ArticleID, err.Error())
		return
	}

	var err error
	if job.ForceRetag {
		err = RetagArticle(ctx, &article, job.FeedNameSnapshot, job.CategoryNameSnapshot)
	} else {
		err = TagArticle(ctx, &article, job.FeedNameSnapshot, job.CategoryNameSnapshot)
	}
	if err != nil {
		logging.Warnf("Failed to tag article %d: %v", job.ArticleID, err)
		_ = q.queue.MarkFailed(job, err.Error(), q.failureBackoff(job.AttemptCount))
		q.broadcastTagFailed(job.ID, job.ArticleID, err.Error())
		return
	}

	if err := q.queue.MarkCompleted(job.ID); err != nil {
		logging.Warnf("Failed to mark tag job %d completed: %v", job.ID, err)
		return
	}

	logging.Infof("Successfully tagged article %d", job.ArticleID)
	tags, broadcastErr := GetArticleTags(job.ArticleID)
	if broadcastErr != nil {
		logging.Warnf("Failed to fetch tags for WebSocket broadcast: %v", broadcastErr)
		return
	}

	q.broadcastTagCompleted(job.ID, job.ArticleID, tags)
}

func (q *TagQueue) failureBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	backoff := time.Duration(1<<min(attempt-1, 4)) * time.Minute
	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}
	return backoff
}

func (q *TagQueue) broadcastTagCompleted(jobID, articleID uint, tags []TopicTag) {
	hub := ws.GetHub()
	tagItems := make([]ws.TagCompletedItem, len(tags))
	for i, tag := range tags {
		tagItems[i] = ws.TagCompletedItem{
			Slug:     tag.Slug,
			Label:    tag.Label,
			Category: tag.Category,
			Score:    tag.Score,
			Icon:     tag.Icon,
		}
	}

	msg := ws.TagCompletedMessage{
		Type:      "tag_completed",
		ArticleID: articleID,
		JobID:     jobID,
		Tags:      tagItems,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("Failed to marshal tag_completed message: %v", err)
		return
	}

	hub.BroadcastRaw(data)
}

func (q *TagQueue) broadcastTagFailed(jobID, articleID uint, errMsg string) {
	hub := ws.GetHub()
	msg := ws.TagFailedMessage{
		Type:      "tag_failed",
		ArticleID: articleID,
		JobID:     jobID,
		Error:     errMsg,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("Failed to marshal tag_failed message: %v", err)
		return
	}

	hub.BroadcastRaw(data)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
