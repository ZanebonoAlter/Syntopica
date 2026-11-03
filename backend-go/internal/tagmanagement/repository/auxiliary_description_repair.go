package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// AuxiliaryDescriptionRepairRepository is the storage foundation of the
// shared-label description-repair chain (board-health-triage-advice seam B1 /
// task 2.1). It owns job creation (atomic scope freeze + idempotency + mutex),
// the server-side opaque source-version token, and the zero-write repair
// context reads. Embedding generation, atomic description+vector commit,
// matching execution and recovery are later seams (tasks 2.3-2.5) and are NOT
// implemented here — creation writes only the job/item rows.

// MaxRepairDescriptionRunes is the upper bound of the confirmed new
// description: trim 后 1..500 Unicode 字符 (spec「共享描述修复须人工确认」).
const MaxRepairDescriptionRunes = 500

// MaxRepairIdempotencyKeyRunes / MaxRepairVersionTokenRunes match the
// varchar(64) columns (PG counts characters) — inputs beyond them are
// rejected at this boundary instead of failing at INSERT with 22001.
const (
	MaxRepairIdempotencyKeyRunes = 64
	MaxRepairVersionTokenRunes   = 64
)

// repairItemInsertBatchSize caps one INSERT statement set when persisting the
// frozen scope (thousands of active tags per job).
const repairItemInsertBatchSize = 500

// activeTopicTagsPredicate is the tagmanagement "active tag" population
// definition (same predicate as ListActiveTags in repository.go). The frozen
// repair scope and the context ActiveTagCount MUST share it so the count
// matches the frozen set.
const activeTopicTagsPredicate = "(status = 'active' OR status = '' OR status IS NULL)"

// Errors of the description-repair storage seam. The future service/handler
// seams (tasks 2.2+) map these onto the contract's 4xx/409/202 responses via
// errors.Is; the three description rules are separate sentinels so the UI can
// give in-place feedback per rule (test-cases 变体走查·输入).
var (
	// ErrRepairLabelNotFound: the id does not exist or is not label_type='auxiliary'.
	ErrRepairLabelNotFound = errors.New("auxiliary label not found")
	// ErrRepairLabelNotActive: the label is auxiliary but disabled (creation rejected).
	ErrRepairLabelNotActive = errors.New("auxiliary label is not active")
	// ErrRepairVersionConflict: expected_version no longer matches the label's
	// current source version — reload and re-confirm, never overwrite
	// (spec「来源版本已变化」).
	ErrRepairVersionConflict = errors.New("auxiliary label source version has changed")
	// ErrRepairIdempotencyConflict: the idempotency key was already used with a
	// different request payload — explicit conflict, NOT a silent replay of the
	// old job (implementation-contract.md §1).
	ErrRepairIdempotencyConflict = errors.New("idempotency key already used with a different request payload")
	// ErrRepairJobAlreadyActive: an unfinished job already exists for this
	// auxiliary label (mutex, including retryable failed stages).
	ErrRepairJobAlreadyActive = errors.New("an unfinished description-repair job already exists for this auxiliary label")
	// ErrRepairIdempotencyKeyEmpty: the request lacks the client-generated key.
	ErrRepairIdempotencyKeyEmpty = errors.New("idempotency key is required")
	// ErrRepairIdempotencyKeyTooLong: the key exceeds the varchar(64) column —
	// rejected by rune count (PG varchar(n) counts characters), so a DB 22001
	// never surfaces as a 500 (review B-P2-3).
	ErrRepairIdempotencyKeyTooLong = errors.New("idempotency key exceeds 64 characters")
	// ErrRepairExpectedVersionInvalid: the version token is missing or exceeds
	// the varchar(64) column capacity. The token stays opaque — presence and
	// capacity are checked, never decoded (review B-P2-3).
	ErrRepairExpectedVersionInvalid = errors.New("expected_version must be a non-empty string of at most 64 characters")

	// The three closed-set description rules — and nothing more: no symbol
	// blacklist or other client-side heuristics may be added here
	// (implementation-contract.md §2 / test-cases: 纯分隔符在本闭集下合法).
	ErrRepairDescriptionEmpty        = errors.New("description is empty after trim")
	ErrRepairDescriptionTooLong      = errors.New("description exceeds 500 characters")
	ErrRepairDescriptionRepeatsLabel = errors.New("description only repeats the label")
)

// ValidateRepairDescription applies the three closed-set input rules and
// returns the canonical (trimmed) description that gets persisted. Rules:
// trim; 1..500 runes; must not merely repeat the label (exact match after
// trim — case/separator variants stay legal, no extra rules).
func ValidateRepairDescription(label, description string) (string, error) {
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return "", ErrRepairDescriptionEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxRepairDescriptionRunes {
		return "", ErrRepairDescriptionTooLong
	}
	if trimmed == label {
		return "", ErrRepairDescriptionRepeatsLabel
	}
	return trimmed, nil
}

// AuxiliaryVersionToken derives the opaque server-side source-version token
// of an auxiliary label (implementation-contract.md §2: covers
// label/description/status/update-time semantics so any server-side change
// after the user's confirmation invalidates it). Clients treat it as opaque
// and MUST NOT construct it — the token is generated here and compared here,
// against the row locked in the create transaction.
func AuxiliaryVersionToken(l *models.SemanticLabel) string {
	h := sha256.New()
	// hash.Writer never returns an error; explicit discard satisfies errcheck.
	_, _ = fmt.Fprintf(h, "v1|id=%d|label=%s|description=%s|status=%s|updated=%d",
		l.ID, l.Label, l.Description, l.Status, l.UpdatedAt.UTC().UnixNano())
	sum := h.Sum(nil)
	return "v1_" + hex.EncodeToString(sum[:20])
}

// CreateRepairJobParams is one confirmed repair request (POST
// .../description-repairs). IdempotencyKey is generated by the client per
// confirmation (timeout retries reuse it); ExpectedVersion MUST equal the
// AuxiliaryVersionToken the client saw at confirmation time.
type CreateRepairJobParams struct {
	AuxiliaryLabelID uint
	IdempotencyKey   string
	ExpectedVersion  string
	NewDescription   string
}

// RepairContextBoard is one board directly referencing the auxiliary label
// via board_composition (shared-impact disclosure; disabled boards included —
// their status is displayed, the contract's direct_boards payload).
type RepairContextBoard struct {
	ID     uint
	Label  string
	Status string
}

// AuxiliaryDescriptionRepairContext is the zero-write read model for the
// repair-context endpoint: live label state + version token + direct boards +
// active-tag population size + latest task (nil when none). Reads never
// create or mutate anything (spec: 只查看建议/打开页面 MUST NOT 创建任务).
type AuxiliaryDescriptionRepairContext struct {
	Label          models.SemanticLabel
	Version        string
	DirectBoards   []RepairContextBoard
	ActiveTagCount int64
	LatestTask     *tagmodels.AuxiliaryDescriptionRepairJob
}

// AuxiliaryDescriptionRepairRepository persists repair jobs and their frozen
// item scope. Construct one per caller; it holds no job state in memory —
// every query hits Postgres, so jobs survive restarts and repo instances.
type AuxiliaryDescriptionRepairRepository struct {
	db *gorm.DB
}

func NewAuxiliaryDescriptionRepairRepository(db *gorm.DB) *AuxiliaryDescriptionRepairRepository {
	return &AuxiliaryDescriptionRepairRepository{db: db}
}

// CreateRepairJob atomically creates one repair job:
//
//  0. validates request shape (key/version presence + varchar(64) rune
//     capacity — review B-P2-3: never let a DB 22001 surface as a 500);
//  1. judges idempotency by the STORED request FIRST, via a pre-lock key
//     lookup — same key + same payload (aux label + expected version +
//     confirmed description) returns the stored job even after the source
//     label was hard-deleted or disabled (review B-P2-1: the replay must not
//     die on the label-row 404); same key + different payload is an explicit
//     conflict, never a silent replay;
//  2. locks the source auxiliary label row (FOR UPDATE), verifies
//     label_type, re-checks the key (race: a job committed between the
//     pre-lock lookup and the lock);
//  3. verifies the label is active, applies the three description rules
//     (stored form = trimmed) and checks the opaque source version against
//     the LOCKED row;
//  4. enforces the per-label mutex on unfinished jobs (retryable failed
//     stages included) via pre-flight read + partial unique index;
//  5. freezes ALL active topic_tags (unmounted included — never just the
//     match table or one board) as pending item rows.
//
// The transaction writes ONLY job/item rows: no label description, no
// embeddings, no match results, no upgrade suggestions (体检写链隔离).
// Unique-index races that beat the pre-flight checks are resolved against the
// committed winner after rollback.
func (r *AuxiliaryDescriptionRepairRepository) CreateRepairJob(ctx context.Context, params CreateRepairJobParams) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	if strings.TrimSpace(params.IdempotencyKey) == "" {
		return nil, ErrRepairIdempotencyKeyEmpty
	}
	if utf8.RuneCountInString(params.IdempotencyKey) > MaxRepairIdempotencyKeyRunes {
		return nil, ErrRepairIdempotencyKeyTooLong
	}
	if params.ExpectedVersion == "" || utf8.RuneCountInString(params.ExpectedVersion) > MaxRepairVersionTokenRunes {
		return nil, ErrRepairExpectedVersionInvalid
	}
	var created, replayed *tagmodels.AuxiliaryDescriptionRepairJob
	trimmed := strings.TrimSpace(params.NewDescription)
	// checkStoredRequest resolves the idempotency decision against the stored
	// job for this key: hit=true (replay captured), conflict error, or not-found
	// (fall through). Shared by the pre-lock and post-lock checks.
	checkStoredRequest := func(tx *gorm.DB) (bool, error) {
		var byKey tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Where("idempotency_key = ?", params.IdempotencyKey).First(&byKey).Error; err == nil {
			if repairPayloadMatches(&byKey, &params, trimmed) {
				replayed = &byKey
				return true, nil
			}
			return false, ErrRepairIdempotencyConflict
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("lookup idempotency key: %w", err)
		}
		return false, nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Idempotent replay, pre-lock (review B-P2-1): decided by the STORED
		// request only, before any current-state gate, so a retry of a stored
		// request keeps winning even after the source label was hard-deleted or
		// disabled (implementation-contract.md §2: 先以已存请求判断幂等).
		if hit, err := checkStoredRequest(tx); err != nil || hit {
			return err
		}

		// Lock the source label so no other writer can change it between the
		// version check and commit (spec「来源版本已变化」→ 保存前锁行校验).
		var label models.SemanticLabel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND label_type = ?", params.AuxiliaryLabelID, "auxiliary").
			First(&label).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRepairLabelNotFound
			}
			return fmt.Errorf("load auxiliary label %d: %w", params.AuxiliaryLabelID, err)
		}

		// Post-lock re-check: a job with this key may have committed between
		// the pre-lock lookup and the lock acquisition.
		if hit, err := checkStoredRequest(tx); err != nil || hit {
			return err
		}

		if label.Status != "active" {
			return ErrRepairLabelNotActive
		}
		if _, err := ValidateRepairDescription(label.Label, trimmed); err != nil {
			return err
		}
		if AuxiliaryVersionToken(&label) != params.ExpectedVersion {
			return ErrRepairVersionConflict
		}

		// Per-label mutex on unfinished jobs (retryable failures included).
		var active tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Where("auxiliary_label_id = ? AND status NOT IN ?",
			params.AuxiliaryLabelID, tagmodels.AuxiliaryDescriptionRepairTerminalStatuses).
			Order("id DESC").First(&active).Error; err == nil {
			if active.IdempotencyKey == params.IdempotencyKey && repairPayloadMatches(&active, &params, trimmed) {
				replayed = &active
				return nil
			}
			return fmt.Errorf("%w (job %d, status %s)", ErrRepairJobAlreadyActive, active.ID, active.Status)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup unfinished repair job: %w", err)
		}

		// Freeze the scope: every active topic_tag, unmounted ones included.
		var tagIDs []uint
		if err := tx.Model(&models.TopicTag{}).
			Where(activeTopicTagsPredicate).
			Order("id ASC").
			Pluck("id", &tagIDs).Error; err != nil {
			return fmt.Errorf("freeze active topic tag scope: %w", err)
		}

		job := &tagmodels.AuxiliaryDescriptionRepairJob{
			AuxiliaryLabelID: params.AuxiliaryLabelID,
			IdempotencyKey:   params.IdempotencyKey,
			ExpectedVersion:  params.ExpectedVersion,
			OldDescription:   label.Description,
			NewDescription:   trimmed,
			Stage:            "",
			Status:           tagmodels.AuxiliaryDescriptionRepairStatusPending,
			Total:            len(tagIDs),
		}
		if err := tx.Create(job).Error; err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			items := make([]tagmodels.AuxiliaryDescriptionRepairItem, len(tagIDs))
			for i, id := range tagIDs {
				items[i] = tagmodels.AuxiliaryDescriptionRepairItem{
					JobID:      job.ID,
					TopicTagID: id,
					Status:     tagmodels.AuxiliaryDescriptionRepairItemPending,
				}
			}
			if err := tx.CreateInBatches(items, repairItemInsertBatchSize).Error; err != nil {
				return fmt.Errorf("persist frozen scope items: %w", err)
			}
		}
		created = job
		return nil
	})
	if err != nil {
		replay, rerr := r.resolveConcurrentInsert(ctx, err, params)
		switch {
		case rerr != nil:
			return nil, rerr
		case replay != nil:
			return replay, nil
		default:
			return nil, err
		}
	}
	if replayed != nil {
		return replayed, nil
	}
	return created, nil
}

// resolveConcurrentInsert inspects a failed create for a unique violation on
// the repair-job indexes — a concurrent transaction committed between our
// pre-flight checks and the insert, and our own transaction rolled back. It
// resolves the committed winner: same key + same payload → replay that job;
// same key + different payload → explicit conflict; otherwise the unfinished
// same-label job → mutex error. Returns (nil, nil) when txErr is not a unique
// violation, leaving the original error in place.
func (r *AuxiliaryDescriptionRepairRepository) resolveConcurrentInsert(ctx context.Context, txErr error, params CreateRepairJobParams) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	var pgErr *pgconn.PgError
	if !errors.As(txErr, &pgErr) || pgErr.Code != "23505" { // SQLSTATE unique_violation
		return nil, nil
	}
	var byKey tagmodels.AuxiliaryDescriptionRepairJob
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", params.IdempotencyKey).
		First(&byKey).Error; err == nil {
		if repairPayloadMatches(&byKey, &params, strings.TrimSpace(params.NewDescription)) {
			return &byKey, nil
		}
		return nil, ErrRepairIdempotencyConflict
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("resolve idempotency key conflict: %w", err)
	}
	var active tagmodels.AuxiliaryDescriptionRepairJob
	if err := r.db.WithContext(ctx).
		Where("auxiliary_label_id = ? AND status NOT IN ?",
			params.AuxiliaryLabelID, tagmodels.AuxiliaryDescriptionRepairTerminalStatuses).
		Order("id DESC").First(&active).Error; err == nil {
		return nil, fmt.Errorf("%w (job %d, status %s)", ErrRepairJobAlreadyActive, active.ID, active.Status)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("resolve unfinished repair job conflict: %w", err)
	}
	return nil, fmt.Errorf("create description repair job (unique violation, unresolved): %w", txErr)
}

// repairPayloadMatches compares a stored job's request payload with an
// incoming request: auxiliary label + expected version + confirmed
// description. The stored description is canonical-trimmed at creation, so
// the incoming one is compared trimmed. Only the STORED request decides
// idempotency — the label's current state is irrelevant on replay.
func repairPayloadMatches(stored *tagmodels.AuxiliaryDescriptionRepairJob, params *CreateRepairJobParams, trimmedDescription string) bool {
	return stored.AuxiliaryLabelID == params.AuxiliaryLabelID &&
		stored.ExpectedVersion == params.ExpectedVersion &&
		stored.NewDescription == trimmedDescription
}

// GetRepairContext is the zero-write live read behind the repair-context
// endpoint. Disabled auxiliary labels are returned (their status is shown as
// not-executable state); nonexistent/non-auxiliary ids are ErrRepairLabelNotFound.
func (r *AuxiliaryDescriptionRepairRepository) GetRepairContext(ctx context.Context, auxiliaryLabelID uint) (*AuxiliaryDescriptionRepairContext, error) {
	var label models.SemanticLabel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND label_type = ?", auxiliaryLabelID, "auxiliary").
		First(&label).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRepairLabelNotFound
		}
		return nil, fmt.Errorf("load auxiliary label %d: %w", auxiliaryLabelID, err)
	}
	var boards []RepairContextBoard
	if err := r.db.WithContext(ctx).
		Table("semantic_labels").
		Select("semantic_labels.id, semantic_labels.label, semantic_labels.status").
		Joins("JOIN board_composition ON board_composition.board_id = semantic_labels.id").
		Where("board_composition.auxiliary_label_id = ? AND semantic_labels.label_type = ?", auxiliaryLabelID, "board").
		Order("semantic_labels.id ASC").
		Scan(&boards).Error; err != nil {
		return nil, fmt.Errorf("load direct boards of auxiliary label %d: %w", auxiliaryLabelID, err)
	}
	var activeCount int64
	if err := r.db.WithContext(ctx).Model(&models.TopicTag{}).
		Where(activeTopicTagsPredicate).Count(&activeCount).Error; err != nil {
		return nil, fmt.Errorf("count active topic tags: %w", err)
	}
	latest, err := r.GetLatestRepairJob(ctx, auxiliaryLabelID)
	if err != nil {
		return nil, err
	}
	return &AuxiliaryDescriptionRepairContext{
		Label:          label,
		Version:        AuxiliaryVersionToken(&label),
		DirectBoards:   boards,
		ActiveTagCount: activeCount,
		LatestTask:     latest,
	}, nil
}

// GetRepairJob returns the persisted job owned by the given auxiliary label.
// A job id under a different label is gorm.ErrRecordNotFound — the GET
// endpoint must not leak cross-label jobs (implementation-contract.md §1:
// 校验 task 属于该辅助标签).
func (r *AuxiliaryDescriptionRepairRepository) GetRepairJob(ctx context.Context, auxiliaryLabelID, jobID uint) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	var job tagmodels.AuxiliaryDescriptionRepairJob
	if err := r.db.WithContext(ctx).
		Where("id = ? AND auxiliary_label_id = ?", jobID, auxiliaryLabelID).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("repair job %d not found for auxiliary label %d: %w",
				jobID, auxiliaryLabelID, gorm.ErrRecordNotFound)
		}
		return nil, fmt.Errorf("load repair job %d: %w", jobID, err)
	}
	return &job, nil
}

// GetLatestRepairJob returns the newest job of the label (nil when none) —
// the context.latest_task source for re-entry recovery (test-cases B9:
// 重新进入从 context.latest_task 恢复).
func (r *AuxiliaryDescriptionRepairRepository) GetLatestRepairJob(ctx context.Context, auxiliaryLabelID uint) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	var job tagmodels.AuxiliaryDescriptionRepairJob
	err := r.db.WithContext(ctx).
		Where("auxiliary_label_id = ?", auxiliaryLabelID).
		Order("created_at DESC, id DESC").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest repair job of label %d: %w", auxiliaryLabelID, err)
	}
	return &job, nil
}
