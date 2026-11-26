// Package auxrepair is the thin application service of the auxiliary-label
// description-repair chain (board-health-triage-advice seam B2 / task 2.2,
// extended by seam B3 / task 2.3).
// It maps the storage layer (repository.AuxiliaryDescriptionRepairRepository)
// onto response DTOs for the three HTTP endpoints, and orchestrates the
// execution phase (storage embedding + atomic save).
//
// Scope: creating/reading repair jobs and the embedding+save execution
// phase. The matching execution (task 2.4), the claim/heartbeat/restart
// recovery (task 2.5) and the production wiring (task 2.6) are NOT
// implemented here; no goroutines are started.
package auxrepair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/board"
)

// CreateJobRequest is the POST /description-repairs body
// {description, expected_version, idempotency_key} (implementation-contract
// §1). Validation rules live in the repository (single source); this service
// passes them through so no second formula can drift.
type CreateJobRequest struct {
	Description     string `json:"description"`
	ExpectedVersion string `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// AuxiliaryLabelView is the context's auxiliary_label payload: id/label/
// description/status plus the opaque server-side version token. Vectors are
// deliberately absent (contract: 不泄露向量).
type AuxiliaryLabelView struct {
	ID          uint   `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Version     string `json:"version"`
}

// DirectBoardView is one board directly referencing the auxiliary label via
// board_composition (shared-impact disclosure; disabled boards included with
// their status shown).
type DirectBoardView struct {
	ID     uint   `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

// RepairContext is the GET /description-repair-context data payload. Pure
// read: fetching it never creates or mutates anything.
type RepairContext struct {
	AuxiliaryLabel AuxiliaryLabelView `json:"auxiliary_label"`
	DirectBoards   []DirectBoardView  `json:"direct_boards"`
	ActiveTagCount int64              `json:"active_tag_count"`
	LatestTask     *JobView           `json:"latest_task"`
}

// JobView is the persisted task DTO — at least the field set of
// implementation-contract §1 (id/auxiliary_label_id/status/stage/
// old_description/new_description/description_committed/total/processed/
// failed/not_applicable/remaining/error/created_at/updated_at), plus the
// request's idempotency key and expected version.
//
// The counters are server-owned single-source: processed = items the
// matching engine actually succeeded on (a legal zero-hit counts), failed =
// items still failing, not_applicable = frozen items that became
// inapplicable (deleted / disabled / merged) — NOT a success and NOT a
// shrinkage of the frozen total, remaining = total - processed - failed -
// not_applicable. The four always add up to total; the frontend never
// recomputes them and never presents a skip or a failure as a success.
type JobView struct {
	ID                   uint      `json:"id"`
	AuxiliaryLabelID     uint      `json:"auxiliary_label_id"`
	IdempotencyKey       string    `json:"idempotency_key"`
	ExpectedVersion      string    `json:"expected_version"`
	OldDescription       string    `json:"old_description"`
	NewDescription       string    `json:"new_description"`
	Stage                string    `json:"stage"`
	Status               string    `json:"status"`
	DescriptionCommitted bool      `json:"description_committed"`
	Total                int       `json:"total"`
	Processed            int       `json:"processed"`
	Failed               int       `json:"failed"`
	NotApplicable        int       `json:"not_applicable"`
	Remaining            int       `json:"remaining"`
	Error                string    `json:"error"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// Service exposes the description-repair read/create surface over an
// injectable *gorm.DB. It holds no job state in memory — everything is
// persisted, so jobs survive restarts and new service instances.
//
// The execution phase (task 2.3) adds three injectable collaborators: the
// storage-embedding unit, the board match-cache invalidation and the clock.
// Defaults are the production ones; the options override them for tests.
type Service struct {
	repo *repository.AuxiliaryDescriptionRepairRepository
	// db is the explicit connection this service works on (never the global
	// repository) — the default matcher is bound to it.
	db *gorm.DB
	// embedder generates the storage embedding of the confirmed description
	// (default: auxlabel.DefaultAuxiliaryLabelEmbedder → airouter, one call
	// per invocation, logged under tagmanagement.auxlabel_embedding).
	embedder Embedder
	// matcher is the EXISTING board matching engine (default: the production
	// engine bound to db). The matching algorithm is not copied here.
	matcher Matcher
	// invalidateCache drops the TTL-less board match cache after a committed
	// save and before every matching run (default: board.InvalidateMatchCache).
	invalidateCache func()
	// now is the clock used for the lease computation and the ownership
	// pre-check (default: time.Now).
	now func() time.Time
	// wakeups are the registered worker wakeup callbacks (the executor
	// registers itself). A wakeup may only WAKE an already running worker —
	// it never starts one and never spawns a goroutine.
	wakeupMu sync.Mutex
	wakeups  []func()
}

// NewService builds the service over the given DB. A nil DB is an explicit
// error — never a silent fallback to a global connection (which could write
// to a production database from a mis-wired test).
//
// NewService(db) stays valid: the collaborators are optional overrides.
// Construction runs neither the matcher nor the embedder.
func NewService(db *gorm.DB, opts ...Option) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("auxrepair: db is required")
	}
	s := &Service{
		repo:            repository.NewAuxiliaryDescriptionRepairRepository(db),
		db:              db,
		embedder:        auxlabel.DefaultAuxiliaryLabelEmbedder,
		matcher:         board.NewSemanticBoardMatchingService(db),
		invalidateCache: board.InvalidateMatchCache,
		now:             time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s, nil
}

// GetRepairContext returns the live label state, version token, direct
// boards, active-tag population size and latest task of the label (nil task
// when none). Disabled labels are returned (displayed as not-executable);
// nonexistent/non-auxiliary ids yield repository.ErrRepairLabelNotFound.
// Zero-write.
func (s *Service) GetRepairContext(ctx context.Context, auxiliaryLabelID uint) (*RepairContext, error) {
	rc, err := s.repo.GetRepairContext(ctx, auxiliaryLabelID)
	if err != nil {
		return nil, err
	}
	boards := make([]DirectBoardView, len(rc.DirectBoards))
	for i, b := range rc.DirectBoards {
		boards[i] = DirectBoardView{ID: b.ID, Label: b.Label, Status: b.Status}
	}
	return &RepairContext{
		AuxiliaryLabel: AuxiliaryLabelView{
			ID:          rc.Label.ID,
			Label:       rc.Label.Label,
			Description: rc.Label.Description,
			Status:      rc.Label.Status,
			Version:     rc.Version,
		},
		DirectBoards:   boards,
		ActiveTagCount: rc.ActiveTagCount,
		LatestTask:     jobView(rc.LatestTask),
	}, nil
}

// CreateJob persists one confirmed repair request (idempotent replay and
// per-label mutex enforced by the storage layer) and returns the task view.
// It performs no embedding call and no matching — those are later seams.
func (s *Service) CreateJob(ctx context.Context, auxiliaryLabelID uint, req CreateJobRequest) (*JobView, error) {
	job, err := s.repo.CreateRepairJob(ctx, repository.CreateRepairJobParams{
		AuxiliaryLabelID: auxiliaryLabelID,
		IdempotencyKey:   req.IdempotencyKey,
		ExpectedVersion:  req.ExpectedVersion,
		NewDescription:   req.Description,
	})
	if err != nil {
		return nil, err
	}
	// The task is queued in the DATABASE; the wakeup only hints the running
	// worker that there is work (it never starts one).
	s.notifyWakeup()
	return jobView(job), nil
}

// GetJob returns the persisted job owned by the given auxiliary label. The
// source label may already be deleted — historical jobs stay queryable; only
// a wrong label/job pairing yields gorm.ErrRecordNotFound.
func (s *Service) GetJob(ctx context.Context, auxiliaryLabelID, jobID uint) (*JobView, error) {
	job, err := s.repo.GetRepairJob(ctx, auxiliaryLabelID, jobID)
	if err != nil {
		return nil, err
	}
	return jobView(job), nil
}

// jobView maps the model to the DTO; nil-safe (absent latest task → nil).
// remaining is computed here from the PERSISTED counters — the backend owns
// the formula, the frontend never recomputes it.
func jobView(j *tagmodels.AuxiliaryDescriptionRepairJob) *JobView {
	if j == nil {
		return nil
	}
	remaining := j.Total - j.Processed - j.Failed - j.NotApplicable
	if remaining < 0 {
		remaining = 0
	}
	return &JobView{
		ID:                   j.ID,
		AuxiliaryLabelID:     j.AuxiliaryLabelID,
		IdempotencyKey:       j.IdempotencyKey,
		ExpectedVersion:      j.ExpectedVersion,
		OldDescription:       j.OldDescription,
		NewDescription:       j.NewDescription,
		Stage:                j.Stage,
		Status:               j.Status,
		DescriptionCommitted: j.DescriptionCommitted,
		Total:                j.Total,
		Processed:            j.Processed,
		Failed:               j.Failed,
		NotApplicable:        j.NotApplicable,
		Remaining:            remaining,
		Error:                j.Error,
		CreatedAt:            j.CreatedAt,
		UpdatedAt:            j.UpdatedAt,
	}
}
