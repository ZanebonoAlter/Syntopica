package models

import "time"

// Auxiliary-label description-repair persistence (board-health-triage-advice
// seam B / design D7, spec auxiliary-label-description-repair). One job = one
// user-confirmed description rewrite of a shared auxiliary label; creation
// freezes the matching scope — ALL active topic_tags at that moment, mounted
// or not (spec「修复后更新匹配并独立重试」: 不得仅枚举匹配行或只处理当前版块)
// — as one item row per tag, so progress survives process restarts without
// any in-memory job map.
//
// FK / deletion strategy (deliberate): NO database foreign keys point at
// semantic_labels or topic_tags. A frozen reference must survive the
// referenced row's later deletion (aux-label GC delete mode, topic-tag
// deletion) without blocking it: a plain FK would regress existing deletion
// paths, ON DELETE CASCADE would silently drop repair history, ON DELETE SET
// NULL would erase the frozen ID. The stale ID is kept, and later execution
// seams record the item as no longer applicable via the open item-status
// string (no schema change needed). B1 (task 2.1) only writes item status
// 'pending' — matching execution, recovery and completion policies are later
// seams and are NOT claimed here.

// Job status vocabulary — the closed set of implementation-contract.md §1.
// pending → embedding → saving → matching → completed, with the failure/
// dead-end statuses embedding_failed | save_failed | matching_failed |
// conflict | unavailable (data-map §2 state diagram).
const (
	AuxiliaryDescriptionRepairStatusPending         = "pending"
	AuxiliaryDescriptionRepairStatusEmbedding       = "embedding"
	AuxiliaryDescriptionRepairStatusSaving          = "saving"
	AuxiliaryDescriptionRepairStatusMatching        = "matching"
	AuxiliaryDescriptionRepairStatusCompleted       = "completed"
	AuxiliaryDescriptionRepairStatusEmbeddingFailed = "embedding_failed"
	AuxiliaryDescriptionRepairStatusSaveFailed      = "save_failed"
	AuxiliaryDescriptionRepairStatusMatchingFailed  = "matching_failed"
	AuxiliaryDescriptionRepairStatusConflict        = "conflict"
	AuxiliaryDescriptionRepairStatusUnavailable     = "unavailable"
)

// Stage vocabulary: embedding | saving | matching. Empty while status=pending
// (implementation-contract.md §1: pending 可为空).
const (
	AuxiliaryDescriptionRepairStageEmbedding = "embedding"
	AuxiliaryDescriptionRepairStageSaving    = "saving"
	AuxiliaryDescriptionRepairStageMatching  = "matching"
)

// AuxiliaryDescriptionRepairTerminalStatuses are the statuses after which a
// job no longer holds the per-label creation mutex — the predicate of partial
// unique index uq_aux_desc_repair_jobs_label_active (migration 20261028_0001).
// The retryable failed stages (embedding_failed / save_failed /
// matching_failed) are deliberately NOT terminal: an unfinished job keeps
// blocking new jobs for the same label (spec「同标签未结束任务不能并发覆盖」),
// including its retry windows. Keep in sync with the migration's SQL literals.
var AuxiliaryDescriptionRepairTerminalStatuses = []string{
	AuxiliaryDescriptionRepairStatusCompleted,
	AuxiliaryDescriptionRepairStatusConflict,
	AuxiliaryDescriptionRepairStatusUnavailable,
}

// Item status vocabulary. succeeded/failed/not_applicable are written by the
// matching execution seam (task 2.4); B1 writes only pending. The column is
// an open string so later seams can also mark a frozen tag as no longer
// applicable (deleted/disabled after the freeze) without schema change.
//
// not_applicable is NOT a success: it records that the frozen tag became
// inapplicable before it could be processed (deleted / disabled / merged).
// It never counts towards processed, and it never shrinks total.
const (
	AuxiliaryDescriptionRepairItemPending       = "pending"
	AuxiliaryDescriptionRepairItemSucceeded     = "succeeded"
	AuxiliaryDescriptionRepairItemFailed        = "failed"
	AuxiliaryDescriptionRepairItemNotApplicable = "not_applicable"
)

// AuxiliaryDescriptionRepairJob is one persisted description-repair task.
// Counters are server-owned single-source (implementation-contract.md §1):
// Total = frozen scope size (never shrinks); Processed = items the matching
// engine actually succeeded on (a legal zero-hit counts); Failed = items
// still failing; NotApplicable = items recorded as no longer applicable
// (deleted / disabled / merged before processing) — NOT a success.
// Remaining = Total - Processed - Failed - NotApplicable, so the four counts
// always add up to Total. The counters are a projection of the frozen item
// rows, never an in-memory increment.
type AuxiliaryDescriptionRepairJob struct {
	ID               uint   `gorm:"primaryKey" json:"id"`
	AuxiliaryLabelID uint   `gorm:"not null;index:idx_aux_desc_repair_jobs_label" json:"auxiliary_label_id"`
	IdempotencyKey   string `gorm:"size:64;not null;uniqueIndex:uq_aux_desc_repair_jobs_key" json:"idempotency_key"`
	ExpectedVersion  string `gorm:"size:64;not null" json:"expected_version"`
	// OldDescription snapshots the label description at confirmation time;
	// NewDescription is the server-trimmed confirmed text. Both stay for
	// audit/rollback inspection (design D7 Migration Plan).
	OldDescription       string `gorm:"type:text" json:"old_description"`
	NewDescription       string `gorm:"type:text;not null" json:"new_description"`
	Stage                string `gorm:"size:16;not null;default:''" json:"stage"`
	Status               string `gorm:"size:20;not null;default:pending;index:idx_aux_desc_repair_jobs_status" json:"status"`
	DescriptionCommitted bool   `gorm:"not null;default:false" json:"description_committed"`
	Total                int    `gorm:"not null;default:0" json:"total"`
	Processed            int    `gorm:"not null;default:0" json:"processed"`
	Failed               int    `gorm:"not null;default:0" json:"failed"`
	// NotApplicable counts the frozen items that became inapplicable before
	// processing (deleted / disabled / merged). Nullable-with-default so the
	// column is added to the existing table WITHOUT a NOT NULL constraint
	// (code-style.md: AutoMigrate adds columns, explicit migrations own the
	// NOT NULL rules). Old tasks read as 0.
	NotApplicable int    `gorm:"default:0" json:"not_applicable"`
	Error         string `gorm:"type:text" json:"error"`

	// Claim/lease reservation for the safe-execution seam (task 2.5: 并发
	// claim/lease 保护). Reserved here so that seam needs no schema change;
	// B1 never writes them (nil = unclaimed).
	ClaimOwner     *string    `gorm:"size:64" json:"claim_owner,omitempty"`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`

	// CommittedVersion / CommittedAt are the INTERNAL record of the atomic
	// description+embedding save (task 2.3, design D7「提交版本」):
	// CommittedVersion is the source-version token of the label AFTER the
	// commit, CommittedAt the server timestamp of that commit. They are
	// nullable (no commit yet) and deliberately NOT part of any HTTP DTO —
	// they are execution bookkeeping, not user-facing data, and no vector is
	// ever stored here.
	CommittedVersion *string    `gorm:"size:64" json:"-"`
	CommittedAt      *time.Time `json:"-"`

	// PhaseToken identifies the ONE attempt that currently owns the
	// execution right of the job's phase (task 2.3/2.4). It is claimed by an
	// atomic CAS, carried by every write of that attempt and cleared when the
	// attempt ends (commit / failure / terminal). A second attempt under the
	// same owner is refused (busy) instead of running the engine twice; a
	// takeover after an expired lease claims a NEW token, so the stale
	// in-flight attempt can no longer commit. Internal only — never part of
	// any HTTP DTO.
	PhaseToken *string `gorm:"size:64" json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AuxiliaryDescriptionRepairJob) TableName() string {
	return "auxiliary_description_repair_jobs"
}

// AuxiliaryDescriptionRepairItem is one frozen member of a job's matching
// scope: (job, topic_tag) is unique, so the scope is a fixed set — tags
// created after the job's creation are NOT in it (they ride the existing
// real-time matching path), and a later deletion of the tag neither drops
// this row nor is blocked by any FK; the stale ID keeps the history.
type AuxiliaryDescriptionRepairItem struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	JobID      uint      `gorm:"not null;uniqueIndex:uq_aux_desc_repair_items_job_tag,priority:1" json:"job_id"`
	TopicTagID uint      `gorm:"not null;uniqueIndex:uq_aux_desc_repair_items_job_tag,priority:2" json:"topic_tag_id"`
	Status     string    `gorm:"size:16;not null;default:pending" json:"status"`
	Error      string    `gorm:"type:text" json:"error"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AuxiliaryDescriptionRepairItem) TableName() string {
	return "auxiliary_description_repair_items"
}
