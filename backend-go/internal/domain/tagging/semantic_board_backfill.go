package tagging

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

const (
	SemanticBoardBackfillModeAll        = "all"
	SemanticBoardBackfillModeUnassigned = "unassigned"
	SemanticBoardBackfillModeBoard      = "board"

	SemanticBoardBackfillStatusPending   = "pending"
	SemanticBoardBackfillStatusRunning   = "running"
	SemanticBoardBackfillStatusCompleted = "completed"
	SemanticBoardBackfillStatusFailed    = "failed"
)

type semanticBoardMatcher interface {
	MatchTopicTag(ctx context.Context, topicTagID uint) ([]SemanticBoardMatchResult, error)
}

type SemanticBoardBackfillService struct {
	db      *gorm.DB
	matcher semanticBoardMatcher

	mu        sync.RWMutex
	jobs      map[string]*semanticBoardBackfillJob
	nextJobID uint64
}

func NewSemanticBoardBackfillService(db *gorm.DB) *SemanticBoardBackfillService {
	if db == nil {
		db = database.DB
	}
	return &SemanticBoardBackfillService{
		db:      db,
		matcher: NewSemanticBoardMatchingService(db),
		jobs:    map[string]*semanticBoardBackfillJob{},
	}
}

type SemanticBoardBackfillRequest struct {
	Mode    string `json:"mode"`
	BoardID *uint  `json:"board_id,omitempty"`
}

type SemanticBoardBackfillJob struct {
	ID          string                         `json:"id"`
	Mode        string                         `json:"mode"`
	BoardID     *uint                          `json:"board_id,omitempty"`
	Total       int                            `json:"total"`
	Processed   int                            `json:"processed"`
	Failed      int                            `json:"failed"`
	Status      string                         `json:"status"`
	Failures    []SemanticBoardBackfillFailure `json:"failures"`
	CreatedAt   time.Time                      `json:"created_at"`
	StartedAt   *time.Time                     `json:"started_at,omitempty"`
	CompletedAt *time.Time                     `json:"completed_at,omitempty"`
}

type SemanticBoardBackfillFailure struct {
	TopicTagID uint   `json:"topic_tag_id"`
	Error      string `json:"error"`
}

type semanticBoardBackfillJob struct {
	SemanticBoardBackfillJob
	topicTagIDs []uint
}

func (s *SemanticBoardBackfillService) Enqueue(ctx context.Context, req SemanticBoardBackfillRequest) (*SemanticBoardBackfillJob, error) {
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	topicTagIDs, err := s.collectTopicTagIDs(ctx, req)
	if err != nil {
		return nil, err
	}

	jobID := fmt.Sprintf("semantic-board-backfill-%d", atomic.AddUint64(&s.nextJobID, 1))
	job := &semanticBoardBackfillJob{
		SemanticBoardBackfillJob: SemanticBoardBackfillJob{
			ID:        jobID,
			Mode:      req.Mode,
			BoardID:   req.BoardID,
			Total:     len(topicTagIDs),
			Status:    SemanticBoardBackfillStatusPending,
			CreatedAt: time.Now(),
		},
		topicTagIDs: topicTagIDs,
	}

	s.mu.Lock()
	s.jobs[jobID] = job
	s.mu.Unlock()

	//nolint:gosec // intentional background goroutine for async backfill processing
	go s.processJob(context.Background(), jobID)
	snapshot, _ := s.GetJob(jobID)
	return snapshot, nil
}

func (s *SemanticBoardBackfillService) GetJob(jobID string) (*SemanticBoardBackfillJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	snapshot := job.SemanticBoardBackfillJob
	if job.BoardID != nil {
		boardID := *job.BoardID
		snapshot.BoardID = &boardID
	}
	snapshot.Failures = append([]SemanticBoardBackfillFailure(nil), job.Failures...)
	return &snapshot, true
}

func (s *SemanticBoardBackfillService) normalizeRequest(ctx context.Context, req SemanticBoardBackfillRequest) (SemanticBoardBackfillRequest, error) {
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = SemanticBoardBackfillModeAll
	}

	switch req.Mode {
	case SemanticBoardBackfillModeAll, SemanticBoardBackfillModeUnassigned:
		return req, nil
	case SemanticBoardBackfillModeBoard:
		if req.BoardID == nil || *req.BoardID == 0 {
			return req, fmt.Errorf("board_id is required for board backfill")
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id = ? AND label_type = ? AND status = ?", *req.BoardID, "board", "active").
			Count(&count).Error; err != nil {
			return req, err
		}
		if count == 0 {
			return req, fmt.Errorf("active semantic board %d not found", *req.BoardID)
		}
		return req, nil
	default:
		return req, fmt.Errorf("unsupported semantic board backfill mode %q", req.Mode)
	}
}

func (s *SemanticBoardBackfillService) collectTopicTagIDs(ctx context.Context, req SemanticBoardBackfillRequest) ([]uint, error) {
	var ids []uint
	query := s.db.WithContext(ctx).Model(&models.TopicTag{}).Where("topic_tags.status = ?", "active")

	switch req.Mode {
	case SemanticBoardBackfillModeAll:
		// No extra filter.
	case SemanticBoardBackfillModeUnassigned:
		query = query.Where("NOT EXISTS (SELECT 1 FROM topic_tag_board_labels WHERE topic_tag_board_labels.topic_tag_id = topic_tags.id)")
	case SemanticBoardBackfillModeBoard:
		return s.collectBoardModeTopicTagIDs(ctx, *req.BoardID)
	}

	if err := query.Order("topic_tags.id ASC").Pluck("topic_tags.id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *SemanticBoardBackfillService) collectBoardModeTopicTagIDs(ctx context.Context, boardID uint) ([]uint, error) {
	ids := map[uint]struct{}{}

	var existingIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&models.TopicTag{}).
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Where("topic_tags.status = ? AND topic_tag_board_labels.semantic_board_id = ?", "active", boardID).
		Pluck("topic_tags.id", &existingIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range existingIDs {
		ids[id] = struct{}{}
	}

	var boardAuxiliaries []boardAuxiliaryLabel
	if err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
		Where("board_composition.board_id = ?", boardID).
		Scan(&boardAuxiliaries).Error; err != nil {
		return nil, err
	}
	if len(boardAuxiliaries) == 0 {
		return sortedSemanticBoardBackfillIDs(ids), nil
	}

	tagAuxiliaries, err := s.loadActiveTagAuxiliaries(ctx)
	if err != nil {
		return nil, err
	}
	config := NewSemanticBoardMatchingService(s.db).loadConfig(ctx)
	for topicTagID, auxiliaries := range tagAuxiliaries {
		matches := evaluateSemanticBoardMatches(auxiliaries, boardAuxiliaries, config, nil, nil)
		if len(matches) > 0 {
			ids[topicTagID] = struct{}{}
		}
	}

	return sortedSemanticBoardBackfillIDs(ids), nil
}

func (s *SemanticBoardBackfillService) loadActiveTagAuxiliaries(ctx context.Context) (map[uint][]models.SemanticLabel, error) {
	type row struct {
		TopicTagID uint
		ID         uint
		Embedding  *string
	}

	var rows []row
	if err := s.db.WithContext(ctx).
		Table("topic_tag_semantic_labels").
		Select("topic_tag_semantic_labels.topic_tag_id, semantic_labels.id, semantic_labels.embedding").
		Joins("JOIN topic_tags ON topic_tags.id = topic_tag_semantic_labels.topic_tag_id AND topic_tags.status = ?", "active").
		Joins("JOIN semantic_labels ON semantic_labels.id = topic_tag_semantic_labels.semantic_label_id AND semantic_labels.label_type = ? AND semantic_labels.status = ?", "auxiliary", "active").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	grouped := make(map[uint][]models.SemanticLabel)
	for _, row := range rows {
		grouped[row.TopicTagID] = append(grouped[row.TopicTagID], models.SemanticLabel{ID: row.ID, Embedding: row.Embedding})
	}
	return grouped, nil
}

func sortedSemanticBoardBackfillIDs(ids map[uint]struct{}) []uint {
	result := make([]uint, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	slices.Sort(result)
	return result
}

func (s *SemanticBoardBackfillService) processJob(ctx context.Context, jobID string) {
	if !s.markRunning(jobID) {
		return
	}

	s.mu.RLock()
	topicTagIDs := append([]uint(nil), s.jobs[jobID].topicTagIDs...)
	s.mu.RUnlock()

	for _, topicTagID := range topicTagIDs {
		if err := ctx.Err(); err != nil {
			s.recordFailure(jobID, topicTagID, err)
			break
		}
		if _, err := s.matcher.MatchTopicTag(ctx, topicTagID); err != nil {
			s.recordFailure(jobID, topicTagID, err)
		}
		s.markProcessed(jobID)
	}

	s.markCompleted(jobID)
}

func (s *SemanticBoardBackfillService) markRunning(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok || job.Status != SemanticBoardBackfillStatusPending {
		return false
	}
	now := time.Now()
	job.Status = SemanticBoardBackfillStatusRunning
	job.StartedAt = &now
	return true
}

func (s *SemanticBoardBackfillService) recordFailure(jobID string, topicTagID uint, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	job.Failed++
	job.Failures = append(job.Failures, SemanticBoardBackfillFailure{TopicTagID: topicTagID, Error: err.Error()})
}

func (s *SemanticBoardBackfillService) markProcessed(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if ok {
		job.Processed++
	}
}

func (s *SemanticBoardBackfillService) markCompleted(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	now := time.Now()
	job.CompletedAt = &now
	if job.Failed > 0 {
		job.Status = SemanticBoardBackfillStatusFailed
		return
	}
	job.Status = SemanticBoardBackfillStatusCompleted
}
