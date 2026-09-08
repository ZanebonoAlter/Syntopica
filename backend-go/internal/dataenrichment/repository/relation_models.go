package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// ── Cross-board relation discovery persistence (add-evidence-backed-cross-board-relations) ──
//
// Two entities, deliberately separated (design D4):
//   - CrossBoardRelationRun: one discovery run (scout/resolve/verify pipeline
//     execution) with its immutable source snapshot, budget, full tool calls
//     and gaps. Runs are audit records, not relations.
//   - CrossBoardRelation: the lifecycle record a human adjudicates. Only rows
//     the user confirmed (status='confirmed', expires_at in the future) ever
//     reach downstream consumers such as board brief generation.

// Relation run statuses.
const (
	RelationRunStatusQueued    = "queued"
	RelationRunStatusRunning   = "running"
	RelationRunStatusSucceeded = "succeeded"
	RelationRunStatusPartial   = "partial"
	RelationRunStatusFailed    = "failed"
)

// Relation lifecycle statuses (spec cross-board-relation-discovery).
const (
	RelationStatusUnresolved = "unresolved"
	RelationStatusProposed   = "proposed"
	RelationStatusConfirmed  = "confirmed"
	RelationStatusDismissed  = "dismissed"
	RelationStatusExpired    = "expired"
)

// Relation types and verification verdicts (spec: fixed enums).
const (
	RelationTypeCausal       = "causal"
	RelationTypeCommonDriver = "common_driver"
	RelationTypeDivergence   = "divergence"
	RelationTypeCorrelated   = "correlated"
	RelationTypeContextual   = "contextual"
	RelationTypeUnclear      = "unclear"

	RelationVerdictSupported    = "supported"
	RelationVerdictContested    = "contested"
	RelationVerdictInsufficient = "insufficient"
	RelationVerdictRejected     = "rejected"
)

// Program-computed quality grades (ordering: high > medium > low lexicographically,
// so `ORDER BY quality_grade DESC` puts the strongest first).
const (
	RelationGradeHigh   = "high"
	RelationGradeMedium = "medium"
	RelationGradeLow    = "low"
)

// Relation source kinds.
const (
	RelationSourceObservation = "observation"
	RelationSourceQuestion    = "question"
)

// Discovery trigger kinds.
const (
	RelationTriggerManual = "manual"
	RelationTriggerAuto   = "auto"
)

// RelationEvidence is one verifiable external citation attached to a relation.
// Quotes must be conservatively checked against the raw tool result before a
// row is persisted; Verified marks whether that mechanical check passed
// (unverified quotes never count toward quality grade).
type RelationEvidence struct {
	Ref         string `json:"ref"`  // tool-call provenance (run/step)
	Tool        string `json:"tool"` // web_search | fetch_page | ...
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Quote       string `json:"quote,omitempty"`
	Institution string `json:"institution,omitempty"`
	Date        string `json:"date,omitempty"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
	Use         string `json:"use"` // support | counter
	Verified    bool   `json:"verified"`
}

// CrossBoardRelationRun persists one discovery run: immutable source snapshot,
// frozen budget, full tool calls and gaps. Written by the pipeline, read for
// polling and audit. Table: cross_board_relation_runs.
type CrossBoardRelationRun struct {
	ID             uint            `gorm:"primarykey" json:"id"`
	SourceBoardID  uint            `gorm:"not null;index" json:"source_board_id"`
	ParentResultID uint            `gorm:"not null" json:"parent_result_id"`
	SourceKind     string          `gorm:"size:16;not null" json:"source_kind"` // observation | question
	SourceKey      string          `gorm:"size:128;not null;index" json:"source_key"`
	SourceText     string          `gorm:"type:text;not null" json:"source_text"`
	TriggerKind    string          `gorm:"size:8;not null;default:manual" json:"trigger_kind"`
	Status         string          `gorm:"size:12;not null;default:queued;index" json:"status"`
	BudgetSnapshot json.RawMessage `gorm:"type:jsonb" json:"budget_snapshot"`
	ToolCalls      json.RawMessage `gorm:"type:jsonb" json:"tool_calls"`
	Gaps           json.RawMessage `gorm:"type:jsonb" json:"gaps"`
	Candidates     json.RawMessage `gorm:"type:jsonb" json:"candidates"`
	Error          string          `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (CrossBoardRelationRun) TableName() string { return "cross_board_relation_runs" }

// CrossBoardRelation persists one cross-board relation suggestion with its
// full lifecycle. Target board/lane are nullable to express `unresolved`
// (concept kept, no internal object bound). Table: cross_board_relations.
type CrossBoardRelation struct {
	ID                  uint               `gorm:"primarykey" json:"id"`
	RunID               *uint              `gorm:"index" json:"run_id,omitempty"`
	SourceBoardID       uint               `gorm:"not null;index;index:idx_cbr_source_board" json:"source_board_id"`
	TargetBoardID       *uint              `gorm:"index:idx_cbr_target_board" json:"target_board_id,omitempty"`
	TargetLaneID        *uint              `gorm:"index" json:"target_lane_id,omitempty"`
	TargetConcept       string             `gorm:"type:text;not null" json:"target_concept"`
	MappingSnapshot     json.RawMessage    `gorm:"type:jsonb" json:"mapping_snapshot"`
	RelationType        string             `gorm:"size:16;not null" json:"relation_type"`
	Claim               string             `gorm:"type:text;not null" json:"claim"`
	Mechanism           string             `gorm:"type:text" json:"mechanism,omitempty"`
	VerificationVerdict string             `gorm:"size:16;not null" json:"verification_verdict"`
	QualityGrade        string             `gorm:"size:8;not null;default:low" json:"quality_grade"`
	Evidence            []RelationEvidence `gorm:"type:jsonb;serializer:json;default:'[]'" json:"evidence"`
	Counterevidence     []RelationEvidence `gorm:"type:jsonb;serializer:json;default:'[]'" json:"counterevidence"`
	Gaps                json.RawMessage    `gorm:"type:jsonb" json:"gaps"`
	Status              string             `gorm:"size:12;not null;default:unresolved;index" json:"status"`
	SuggestionHash      string             `gorm:"size:64;not null" json:"suggestion_hash"`
	EvidenceVersion     string             `gorm:"size:64;not null;default:v1" json:"evidence_version"`
	ExpiresAt           *time.Time         `gorm:"index" json:"expires_at,omitempty"`
	ConfirmedAt         *time.Time         `json:"confirmed_at,omitempty"`
	DismissedAt         *time.Time         `json:"dismissed_at,omitempty"`
	ExpiredAt           *time.Time         `json:"expired_at,omitempty"`
	DismissReason       *string            `gorm:"type:text" json:"dismiss_reason,omitempty"`
	ResolvedBy          *string            `gorm:"size:50" json:"resolved_by,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

func (CrossBoardRelation) TableName() string { return "cross_board_relations" }

// ComputeRelationHash derives the stable idempotency key for a relation:
// normalized source identity (board + kind + key), resolved target (board id)
// or the target concept when unresolved, relation type, normalized claim and
// the evidence version. Re-running discovery on an unchanged source/target/
// claim/evidence-version combination produces the same hash; the partial
// unique index on open statuses makes the insert a no-op.
func ComputeRelationHash(sourceBoardID uint, sourceKind, sourceKey string, targetBoardID *uint, targetConcept, relationType, claim, evidenceVersion string) string {
	var target string
	if targetBoardID != nil {
		target = "board:" + uint64ToString(uint64(*targetBoardID))
	} else {
		target = "concept:" + normalizeHashToken(targetConcept)
	}
	payload := strings.Join([]string{
		"cbr-v1",
		uint64ToString(uint64(sourceBoardID)),
		sourceKind,
		normalizeHashToken(sourceKey),
		target,
		relationType,
		normalizeHashToken(claim),
		evidenceVersion,
	}, "\x1f")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

// normalizeHashToken folds whitespace for hash stability without changing the
// stored text (mirrors ComputeQuestionKey's normalization semantics).
func normalizeHashToken(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func uint64ToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// ValidRelationStatus reports whether s is a known lifecycle status.
func ValidRelationStatus(s string) bool {
	switch s {
	case RelationStatusUnresolved, RelationStatusProposed, RelationStatusConfirmed, RelationStatusDismissed, RelationStatusExpired:
		return true
	}
	return false
}

// ValidRelationType reports whether t is a known relation type.
func ValidRelationType(t string) bool {
	switch t {
	case RelationTypeCausal, RelationTypeCommonDriver, RelationTypeDivergence, RelationTypeCorrelated, RelationTypeContextual, RelationTypeUnclear:
		return true
	}
	return false
}

// ValidRelationVerdict reports whether v is a known verification verdict.
func ValidRelationVerdict(v string) bool {
	switch v {
	case RelationVerdictSupported, RelationVerdictContested, RelationVerdictInsufficient, RelationVerdictRejected:
		return true
	}
	return false
}
