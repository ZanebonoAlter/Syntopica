package analysis

import (
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	AnalysisTypeEvent   = "event"
	AnalysisTypePerson  = "person"
	AnalysisTypeKeyword = "keyword"

	WindowTypeDaily  = "daily"
	WindowTypeWeekly = "weekly"

	AnalysisPriorityHigh   = 1
	AnalysisPriorityMedium = 2
	AnalysisPriorityLow    = 3

	AnalysisStatusPending    = "pending"
	AnalysisStatusProcessing = "processing"
	AnalysisStatusCompleted  = "completed"
	AnalysisStatusFailed     = "failed"

	maxAnalysisRetry = 3
)

// AnalysisJob 分析任务
type AnalysisJob struct {
	ID           string
	TopicTagID   uint64
	AnalysisType string
	WindowType   string
	AnchorDate   time.Time
	Priority     int
	Status       string
	RetryCount   int
	ErrorMessage string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

// AnalysisQueue 分析队列接口
type AnalysisQueue interface {
	Enqueue(job *AnalysisJob) error
	Dequeue() (*AnalysisJob, error)
	UpdateStatus(jobID string, status string, errorMsg string) error
	Complete(jobID string) error
	Get(jobID string) (*AnalysisJob, error)
	GetPendingByTag(topicTagID uint64) ([]*AnalysisJob, error)
}

type QueueSnapshot struct {
	Job      *AnalysisJob
	Progress int
}

type AnalysisQueueWithSnapshot interface {
	AnalysisQueue
	GetLatestByKey(topicTagID uint64, analysisType string, windowType string, anchorDate time.Time) (*QueueSnapshot, bool)
	MarkProgress(jobID string, progress int) error
	Fail(jobID string, err error) error
}

type topicAnalysisJobRecord struct {
	ID           string `gorm:"primaryKey;size:64"`
	TopicTagID   uint64 `gorm:"index:idx_topic_analysis_job_lookup"`
	AnalysisType string `gorm:"size:32;index:idx_topic_analysis_job_lookup"`
	WindowType   string `gorm:"size:32;index:idx_topic_analysis_job_lookup"`
	AnchorDate   time.Time
	Priority     int
	Status       string `gorm:"size:32;index:idx_topic_analysis_job_status"`
	RetryCount   int
	ErrorMessage string `gorm:"type:text"`
	Progress     int
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

func (topicAnalysisJobRecord) TableName() string {
	return "topic_analysis_jobs"
}

func NewAnalysisQueue(db *gorm.DB, logger *zap.Logger) AnalysisQueueWithSnapshot {
	if logger == nil {
		logger = zap.NewNop()
	}

	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if redisURL != "" {
		if queue, err := newRedisAnalysisQueue(redisURL, logger); err == nil {
			logger.Info("topic analysis queue uses redis backend")
			return queue
		} else {
			logger.Warn("topic analysis redis queue unavailable, fallback to in-memory", zap.Error(err))
		}
	}

	queue := newInMemoryAnalysisQueue(db, logger)
	logger.Info("topic analysis queue uses in-memory backend")
	return queue
}

type analysisHeapItem struct {
	job       *AnalysisJob
	seq       int64
	createdAt time.Time
	index     int
	progress  int
}

type analysisHeap []*analysisHeapItem

func (h analysisHeap) Len() int { return len(h) }

func (h analysisHeap) Less(i, j int) bool {
	if h[i].job.Priority == h[j].job.Priority {
		if h[i].createdAt.Equal(h[j].createdAt) {
			return h[i].seq < h[j].seq
		}
		return h[i].createdAt.Before(h[j].createdAt)
	}
	return h[i].job.Priority < h[j].job.Priority
}

func (h analysisHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *analysisHeap) Push(x any) {
	item, ok := x.(*analysisHeapItem)
	if !ok || item == nil {
		return
	}
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *analysisHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]
	return item
}

type inMemoryAnalysisQueue struct {
	mu          sync.Mutex
	cond        *sync.Cond
	closed      bool
	jobs        analysisHeap
	byID        map[string]*analysisHeapItem
	activeByKey map[string]string
	latestByKey map[string]string
	seq         int64
	db          *gorm.DB
	logger      *zap.Logger
}

func newInMemoryAnalysisQueue(db *gorm.DB, logger *zap.Logger) *inMemoryAnalysisQueue {
	q := &inMemoryAnalysisQueue{
		jobs:        make(analysisHeap, 0),
		byID:        make(map[string]*analysisHeapItem),
		activeByKey: make(map[string]string),
		latestByKey: make(map[string]string),
		db:          db,
		logger:      logger,
	}
	q.cond = sync.NewCond(&q.mu)
	heap.Init(&q.jobs)
	q.restoreFromPersistence()
	return q
}

func (q *inMemoryAnalysisQueue) Enqueue(job *AnalysisJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return errors.New("analysis queue is closed")
	}

	normalized, err := normalizeAnalysisJob(job)
	if err != nil {
		return err
	}

	key := makeAnalysisDedupKey(normalized.TopicTagID, normalized.AnalysisType, normalized.WindowType, normalized.AnchorDate)
	if existingID, ok := q.activeByKey[key]; ok {
		if existing, exists := q.byID[existingID]; exists {
			if existing.job.Status == AnalysisStatusPending && normalized.Priority < existing.job.Priority {
				existing.job.Priority = normalized.Priority
				heap.Fix(&q.jobs, existing.index)
				q.persist(existing)
			}
			q.latestByKey[key] = existing.job.ID
			return nil
		}
	}

	q.seq++
	item := &analysisHeapItem{
		job:       normalized,
		seq:       q.seq,
		createdAt: normalized.CreatedAt,
		progress:  0,
	}

	q.byID[normalized.ID] = item
	q.activeByKey[key] = normalized.ID
	q.latestByKey[key] = normalized.ID
	heap.Push(&q.jobs, item)
	q.persist(item)
	q.cond.Signal()
	return nil
}

func (q *inMemoryAnalysisQueue) Dequeue() (*AnalysisJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.jobs.Len() == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil, errors.New("analysis queue is closed")
	}

	raw := heap.Pop(&q.jobs)
	item, ok := raw.(*analysisHeapItem)
	if !ok || item == nil {
		return nil, errors.New("analysis queue pop failed")
	}

	now := time.Now()
	item.job.Status = AnalysisStatusProcessing
	item.job.StartedAt = &now
	item.job.ErrorMessage = ""
	item.progress = maxInt(item.progress, 10)
	q.persist(item)

	return cloneAnalysisJob(item.job), nil
}

func (q *inMemoryAnalysisQueue) UpdateStatus(jobID string, status string, errorMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, err := q.getJob(jobID)
	if err != nil {
		return err
	}

	now := time.Now()
	status = normalizeJobStatus(status)
	switch status {
	case AnalysisStatusCompleted:
		item.job.Status = AnalysisStatusCompleted
		item.job.CompletedAt = &now
		item.job.ErrorMessage = ""
		item.progress = 100
		q.releaseDedupKey(item.job)
	case AnalysisStatusFailed:
		item.job.ErrorMessage = strings.TrimSpace(errorMsg)
		if item.job.ErrorMessage == "" {
			item.job.ErrorMessage = "analysis failed"
		}
		if item.job.RetryCount < maxAnalysisRetry {
			item.job.RetryCount++
			item.job.Status = AnalysisStatusPending
			item.job.StartedAt = nil
			item.job.CompletedAt = nil
			item.progress = maxInt(item.progress, 10)
			q.seq++
			item.seq = q.seq
			item.createdAt = now
			heap.Push(&q.jobs, item)
			q.persist(item)
			q.cond.Signal()
			return nil
		}
		item.job.Status = AnalysisStatusFailed
		item.job.CompletedAt = &now
		item.progress = 100
		q.releaseDedupKey(item.job)
	case AnalysisStatusPending:
		item.job.Status = AnalysisStatusPending
		item.job.StartedAt = nil
		item.job.CompletedAt = nil
		item.job.ErrorMessage = strings.TrimSpace(errorMsg)
		if item.index < 0 {
			q.seq++
			item.seq = q.seq
			item.createdAt = now
			heap.Push(&q.jobs, item)
			q.cond.Signal()
		}
	case AnalysisStatusProcessing:
		item.job.Status = AnalysisStatusProcessing
		item.job.ErrorMessage = ""
		item.job.StartedAt = &now
	default:
		return fmt.Errorf("unsupported status: %s", status)
	}

	q.persist(item)
	return nil
}

func (q *inMemoryAnalysisQueue) Complete(jobID string) error {
	return q.UpdateStatus(jobID, AnalysisStatusCompleted, "")
}

func (q *inMemoryAnalysisQueue) Fail(jobID string, err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return q.UpdateStatus(jobID, AnalysisStatusFailed, msg)
}

func (q *inMemoryAnalysisQueue) Get(jobID string) (*AnalysisJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, err := q.getJob(jobID)
	if err != nil {
		return nil, err
	}
	return cloneAnalysisJob(item.job), nil
}

func (q *inMemoryAnalysisQueue) GetPendingByTag(topicTagID uint64) ([]*AnalysisJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]*AnalysisJob, 0)
	for _, item := range q.byID {
		if item.job.TopicTagID == topicTagID && item.job.Status == AnalysisStatusPending {
			out = append(out, cloneAnalysisJob(item.job))
		}
	}
	return out, nil
}

func (q *inMemoryAnalysisQueue) GetLatestByKey(topicTagID uint64, analysisType string, windowType string, anchorDate time.Time) (*QueueSnapshot, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := makeAnalysisDedupKey(topicTagID, analysisType, windowType, normalizeQueueAnchorDate(anchorDate))
	id, ok := q.latestByKey[key]
	if !ok {
		return nil, false
	}
	item, ok := q.byID[id]
	if !ok {
		return nil, false
	}
	return &QueueSnapshot{Job: cloneAnalysisJob(item.job), Progress: item.progress}, true
}

func (q *inMemoryAnalysisQueue) MarkProgress(jobID string, progress int) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, err := q.getJob(jobID)
	if err != nil {
		return err
	}
	item.progress = clampInt(progress, 0, 99)
	q.persist(item)
	return nil
}

func (q *inMemoryAnalysisQueue) getJob(jobID string) (*analysisHeapItem, error) {
	item, ok := q.byID[jobID]
	if !ok {
		return nil, fmt.Errorf("analysis job %s not found", jobID)
	}
	return item, nil
}

func (q *inMemoryAnalysisQueue) releaseDedupKey(job *AnalysisJob) {
	if job == nil {
		return
	}
	key := makeAnalysisDedupKey(job.TopicTagID, job.AnalysisType, job.WindowType, job.AnchorDate)
	if existing, ok := q.activeByKey[key]; ok && existing == job.ID {
		delete(q.activeByKey, key)
	}
}

func (q *inMemoryAnalysisQueue) persist(item *analysisHeapItem) {
	if item == nil || q.db == nil {
		return
	}
	record := topicAnalysisJobRecord{
		ID:           item.job.ID,
		TopicTagID:   item.job.TopicTagID,
		AnalysisType: item.job.AnalysisType,
		WindowType:   item.job.WindowType,
		AnchorDate:   item.job.AnchorDate,
		Priority:     item.job.Priority,
		Status:       item.job.Status,
		RetryCount:   item.job.RetryCount,
		ErrorMessage: item.job.ErrorMessage,
		Progress:     item.progress,
		CreatedAt:    item.job.CreatedAt,
		StartedAt:    item.job.StartedAt,
		CompletedAt:  item.job.CompletedAt,
	}
	if err := q.db.Save(&record).Error; err != nil {
		q.logger.Warn("failed to persist topic analysis job", zap.Error(err), zap.String("job_id", item.job.ID))
	}
}

func (q *inMemoryAnalysisQueue) restoreFromPersistence() {
	if q.db == nil {
		return
	}
	if err := q.db.AutoMigrate(&topicAnalysisJobRecord{}); err != nil {
		q.logger.Warn("failed to migrate topic analysis job table", zap.Error(err))
		return
	}

	var records []topicAnalysisJobRecord
	if err := q.db.Where("status IN ?", []string{AnalysisStatusPending, AnalysisStatusProcessing}).Order("created_at ASC").Find(&records).Error; err != nil {
		q.logger.Warn("failed to load persisted topic analysis jobs", zap.Error(err))
		return
	}

	for _, record := range records {
		job := &AnalysisJob{
			ID:           record.ID,
			TopicTagID:   record.TopicTagID,
			AnalysisType: normalizeAnalysisType(record.AnalysisType),
			WindowType:   normalizeWindowType(record.WindowType),
			AnchorDate:   normalizeQueueAnchorDate(record.AnchorDate),
			Priority:     normalizePriority(record.Priority),
			Status:       AnalysisStatusPending,
			RetryCount:   record.RetryCount,
			ErrorMessage: strings.TrimSpace(record.ErrorMessage),
			CreatedAt:    record.CreatedAt,
		}
		if job.CreatedAt.IsZero() {
			job.CreatedAt = time.Now()
		}

		q.seq++
		item := &analysisHeapItem{job: job, seq: q.seq, createdAt: job.CreatedAt, progress: clampInt(record.Progress, 0, 99)}
		q.byID[job.ID] = item
		key := makeAnalysisDedupKey(job.TopicTagID, job.AnalysisType, job.WindowType, job.AnchorDate)
		q.activeByKey[key] = job.ID
		q.latestByKey[key] = job.ID
		heap.Push(&q.jobs, item)
	}
}

type redisAnalysisQueue struct {
	client *redis.Client
	logger *zap.Logger
}

func newRedisAnalysisQueue(redisURL string, logger *zap.Logger) (*redisAnalysisQueue, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &redisAnalysisQueue{client: client, logger: logger}, nil
}

func (q *redisAnalysisQueue) Enqueue(job *AnalysisJob) error {
	normalized, err := normalizeAnalysisJob(job)
	if err != nil {
		return err
	}

	ctx := context.Background()
	key := makeAnalysisDedupKey(normalized.TopicTagID, normalized.AnalysisType, normalized.WindowType, normalized.AnchorDate)
	existingID, err := q.client.HGet(ctx, redisDedupKey(), key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	if existingID != "" {
		existing, getErr := q.Get(existingID)
		if getErr == nil && existing != nil && existing.Status == AnalysisStatusPending && normalized.Priority < existing.Priority {
			existing.Priority = normalized.Priority
			return q.save(existing, 0)
		}
		return nil
	}

	if err := q.save(normalized, 0); err != nil {
		return err
	}
	if err := q.client.HSet(ctx, redisDedupKey(), key, normalized.ID).Err(); err != nil {
		return err
	}
	if err := q.client.RPush(ctx, redisPriorityList(normalized.Priority), normalized.ID).Err(); err != nil {
		return err
	}
	if err := q.client.SAdd(ctx, redisTagPendingSet(normalized.TopicTagID), normalized.ID).Err(); err != nil {
		return err
	}
	if err := q.client.HSet(ctx, redisLatestKey(), key, normalized.ID).Err(); err != nil {
		return err
	}
	return nil
}

func (q *redisAnalysisQueue) Dequeue() (*AnalysisJob, error) {
	ctx := context.Background()
	result, err := q.client.BLPop(ctx, time.Second, redisPriorityList(AnalysisPriorityHigh), redisPriorityList(AnalysisPriorityMedium), redisPriorityList(AnalysisPriorityLow)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(result) != 2 {
		return nil, nil
	}

	jobID := result[1]
	job, err := q.Get(jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}

	now := time.Now()
	job.Status = AnalysisStatusProcessing
	job.StartedAt = &now
	job.ErrorMessage = ""
	if err := q.save(job, 10); err != nil {
		return nil, err
	}
	return job, nil
}

func (q *redisAnalysisQueue) UpdateStatus(jobID string, status string, errorMsg string) error {
	job, err := q.Get(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("analysis job %s not found", jobID)
	}

	now := time.Now()
	status = normalizeJobStatus(status)
	ctx := context.Background()
	switch status {
	case AnalysisStatusCompleted:
		job.Status = AnalysisStatusCompleted
		job.CompletedAt = &now
		job.ErrorMessage = ""
		if err := q.cleanupDedupAndPending(ctx, job); err != nil {
			return err
		}
		return q.save(job, 100)
	case AnalysisStatusFailed:
		job.ErrorMessage = strings.TrimSpace(errorMsg)
		if job.ErrorMessage == "" {
			job.ErrorMessage = "analysis failed"
		}
		if job.RetryCount < maxAnalysisRetry {
			job.RetryCount++
			job.Status = AnalysisStatusPending
			job.StartedAt = nil
			job.CompletedAt = nil
			if err := q.save(job, 10); err != nil {
				return err
			}
			return q.client.RPush(ctx, redisPriorityList(job.Priority), job.ID).Err()
		}
		job.Status = AnalysisStatusFailed
		job.CompletedAt = &now
		if err := q.cleanupDedupAndPending(ctx, job); err != nil {
			return err
		}
		return q.save(job, 100)
	default:
		return fmt.Errorf("unsupported status: %s", status)
	}
}

func (q *redisAnalysisQueue) Complete(jobID string) error {
	return q.UpdateStatus(jobID, AnalysisStatusCompleted, "")
}

func (q *redisAnalysisQueue) Fail(jobID string, err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return q.UpdateStatus(jobID, AnalysisStatusFailed, msg)
}

func (q *redisAnalysisQueue) Get(jobID string) (*AnalysisJob, error) {
	ctx := context.Background()
	value, err := q.client.HGet(ctx, redisJobsKey(), jobID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var payload struct {
		Job      *AnalysisJob `json:"job"`
		Progress int          `json:"progress"`
	}
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return nil, err
	}
	return payload.Job, nil
}

func (q *redisAnalysisQueue) GetPendingByTag(topicTagID uint64) ([]*AnalysisJob, error) {
	ctx := context.Background()
	ids, err := q.client.SMembers(ctx, redisTagPendingSet(topicTagID)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]*AnalysisJob, 0, len(ids))
	for _, id := range ids {
		job, getErr := q.Get(id)
		if getErr != nil || job == nil {
			continue
		}
		if job.Status == AnalysisStatusPending {
			out = append(out, job)
		}
	}
	return out, nil
}

func (q *redisAnalysisQueue) GetLatestByKey(topicTagID uint64, analysisType string, windowType string, anchorDate time.Time) (*QueueSnapshot, bool) {
	ctx := context.Background()
	key := makeAnalysisDedupKey(topicTagID, normalizeAnalysisType(analysisType), normalizeWindowType(windowType), normalizeQueueAnchorDate(anchorDate))
	jobID, err := q.client.HGet(ctx, redisLatestKey(), key).Result()
	if errors.Is(err, redis.Nil) || strings.TrimSpace(jobID) == "" {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	value, err := q.client.HGet(ctx, redisJobsKey(), jobID).Result()
	if err != nil {
		return nil, false
	}
	var payload struct {
		Job      *AnalysisJob `json:"job"`
		Progress int          `json:"progress"`
	}
	if err := json.Unmarshal([]byte(value), &payload); err != nil || payload.Job == nil {
		return nil, false
	}
	return &QueueSnapshot{Job: payload.Job, Progress: payload.Progress}, true
}

func (q *redisAnalysisQueue) MarkProgress(jobID string, progress int) error {
	job, err := q.Get(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("analysis job %s not found", jobID)
	}
	return q.save(job, clampInt(progress, 0, 99))
}

func (q *redisAnalysisQueue) save(job *AnalysisJob, progress int) error {
	ctx := context.Background()
	payload := struct {
		Job      *AnalysisJob `json:"job"`
		Progress int          `json:"progress"`
	}{Job: job, Progress: progress}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return q.client.HSet(ctx, redisJobsKey(), job.ID, bytes).Err()
}

func (q *redisAnalysisQueue) cleanupDedupAndPending(ctx context.Context, job *AnalysisJob) error {
	key := makeAnalysisDedupKey(job.TopicTagID, job.AnalysisType, job.WindowType, job.AnchorDate)
	if err := q.client.HDel(ctx, redisDedupKey(), key).Err(); err != nil {
		return err
	}
	if err := q.client.SRem(ctx, redisTagPendingSet(job.TopicTagID), job.ID).Err(); err != nil {
		return err
	}
	return nil
}

func normalizeAnalysisJob(job *AnalysisJob) (*AnalysisJob, error) {
	if job == nil {
		return nil, errors.New("job is required")
	}

	copyJob := *job
	copyJob.TopicTagID = job.TopicTagID
	copyJob.AnalysisType = normalizeAnalysisType(strings.TrimSpace(job.AnalysisType))
	copyJob.WindowType = normalizeWindowType(strings.TrimSpace(job.WindowType))
	copyJob.AnchorDate = normalizeQueueAnchorDate(job.AnchorDate)
	copyJob.Priority = normalizePriority(job.Priority)
	copyJob.Status = AnalysisStatusPending
	copyJob.RetryCount = maxInt(job.RetryCount, 0)
	copyJob.ErrorMessage = ""
	copyJob.StartedAt = nil
	copyJob.CompletedAt = nil

	if copyJob.ID == "" {
		copyJob.ID = newAnalysisJobID()
	}
	if copyJob.CreatedAt.IsZero() {
		copyJob.CreatedAt = time.Now()
	}
	if copyJob.TopicTagID == 0 {
		return nil, errors.New("topic_tag_id is required")
	}
	if copyJob.AnalysisType == "" {
		return nil, errors.New("analysis_type is required")
	}
	return &copyJob, nil
}

func normalizeAnalysisType(v string) string {
	switch v {
	case AnalysisTypeEvent, AnalysisTypePerson, AnalysisTypeKeyword:
		return v
	default:
		return ""
	}
}

func normalizeWindowType(v string) string {
	switch v {
	case WindowTypeDaily, WindowTypeWeekly:
		return v
	default:
		return WindowTypeDaily
	}
}

func normalizePriority(priority int) int {
	switch priority {
	case AnalysisPriorityHigh, AnalysisPriorityMedium, AnalysisPriorityLow:
		return priority
	default:
		return AnalysisPriorityMedium
	}
}

func normalizeJobStatus(status string) string {
	switch strings.TrimSpace(status) {
	case AnalysisStatusPending:
		return AnalysisStatusPending
	case AnalysisStatusProcessing:
		return AnalysisStatusProcessing
	case AnalysisStatusCompleted:
		return AnalysisStatusCompleted
	case AnalysisStatusFailed:
		return AnalysisStatusFailed
	default:
		return status
	}
}

func normalizeQueueAnchorDate(anchor time.Time) time.Time {
	if anchor.IsZero() {
		anchor = time.Now()
	}
	return time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, anchor.Location())
}

func makeAnalysisDedupKey(topicTagID uint64, analysisType string, windowType string, anchorDate time.Time) string {
	return strconv.FormatUint(topicTagID, 10) + ":" + analysisType + ":" + windowType + ":" + normalizeQueueAnchorDate(anchorDate).Format("2006-01-02")
}

func newAnalysisJobID() string {
	return "analysis_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func cloneAnalysisJob(job *AnalysisJob) *AnalysisJob {
	if job == nil {
		return nil
	}
	copyJob := *job
	if job.StartedAt != nil {
		started := *job.StartedAt
		copyJob.StartedAt = &started
	}
	if job.CompletedAt != nil {
		completed := *job.CompletedAt
		copyJob.CompletedAt = &completed
	}
	return &copyJob
}

func redisJobsKey() string {
	return "topicgraph:analysis:jobs"
}

func redisDedupKey() string {
	return "topicgraph:analysis:dedup"
}

func redisLatestKey() string {
	return "topicgraph:analysis:latest"
}

func redisPriorityList(priority int) string {
	return "topicgraph:analysis:priority:" + strconv.Itoa(priority)
}

func redisTagPendingSet(topicTagID uint64) string {
	return "topicgraph:analysis:tag:" + strconv.FormatUint(topicTagID, 10) + ":pending"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v int, minValue int, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}
