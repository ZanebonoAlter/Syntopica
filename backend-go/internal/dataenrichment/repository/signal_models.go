package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ── Board signal discovery persistence (board-signal-reports) ────────────────
//
// Two entities, deliberately separated (design §3):
//   - BoardSignalDiscovery: one discovery batch (one manual trigger). Only
//     successful batches are written — including zero-candidate batches; a
//     half batch is never persisted (candidates + batch share one transaction).
//   - BoardSignalCandidate: one immutable detect candidate. Its owner/period
//     columns are stamped from the batch at write time and pinned by the
//     composite foreign key candidate(discovery_id, semantic_board_id,
//     granularity, period) → discovery(id, …) (DB-level backstop for direct
//     SQL writes; the repository stamps them in code as well).

// Signal granularity values (design §2: month|year only; week/all rejected).
const (
	SignalGranularityMonth = "month"
	SignalGranularityYear  = "year"
)

// Analysis modes (design §2: ended historical periods are retrospective;
// they claim the data version obtained now, never a point-in-time backtest).
const (
	SignalAnalysisModeCurrent       = "current"
	SignalAnalysisModeRetrospective = "retrospective"
)

// ResultKindSignalReport is declared in models.go next to the other kind
// constants. It is deliberately NOT in isBoardResultKind: legacy
// brief/investigation APIs keep their old semantics and never surface
// signal reports.

// BoardSignalDiscovery is one discovery batch. Table: board_signal_discovery.
type BoardSignalDiscovery struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	SemanticBoardID uint   `gorm:"not null;index:idx_board_signal_discovery_board_period,priority:1" json:"semantic_board_id"`
	Granularity     string `gorm:"size:10;not null;index:idx_board_signal_discovery_board_period,priority:2" json:"granularity"`
	Period          string `gorm:"size:12;not null;index:idx_board_signal_discovery_board_period,priority:3" json:"period"`
	AnalysisMode    string `gorm:"size:16;not null;default:current" json:"analysis_mode"`
	// Cutoff is the discovery's data boundary: the job start for the current
	// period, the period end for a historical one (design §2).
	Cutoff         time.Time       `gorm:"not null" json:"cutoff"`
	InputSnapshot  json.RawMessage `gorm:"type:jsonb" json:"input_snapshot"`
	SessionID      string          `gorm:"size:120" json:"session_id"`
	CandidateCount int             `gorm:"not null;default:0" json:"candidate_count"`
	CreatedAt      time.Time       `json:"created_at"`
}

func (BoardSignalDiscovery) TableName() string { return "board_signal_discovery" }

// BoardSignalCandidate is one immutable detect candidate. Table:
// board_signal_candidate. Candidate content never changes after insert;
// re-discovery appends new batches without touching old rows.
type BoardSignalCandidate struct {
	ID               uint   `gorm:"primarykey" json:"id"`
	DiscoveryID      uint   `gorm:"not null;index:idx_board_signal_candidate_discovery" json:"discovery_id"`
	SemanticBoardID  uint   `gorm:"not null;index:idx_board_signal_candidate_board_period,priority:1" json:"semantic_board_id"`
	Granularity      string `gorm:"size:10;not null;index:idx_board_signal_candidate_board_period,priority:2" json:"granularity"`
	Period           string `gorm:"size:12;not null;index:idx_board_signal_candidate_board_period,priority:3" json:"period"`
	Signal           string `gorm:"type:text;not null" json:"signal"`
	WhyItMatters     string `gorm:"type:text;not null" json:"why_it_matters"`
	ResearchQuestion string `gorm:"type:text;not null" json:"research_question"`
	// EvidenceRefs is a non-empty JSON array of refs into the batch's input
	// snapshot whitelist (stage-2 contract); the repository only enforces the
	// array shape, whitelist membership is detect's job.
	EvidenceRefs json.RawMessage `gorm:"type:jsonb" json:"evidence_refs"`
	// ResearchBrief 是 question-v1 的版本化研究简要说明（unknown/verification/
	// source_ids；source_ids 由代码核实填写，禁止信任模型自报）。旧行 NULL
	// 不猜填；一次写入后不可变。
	ResearchBrief json.RawMessage `gorm:"type:jsonb" json:"research_brief,omitempty"`
	Score         int             `gorm:"not null" json:"score"`
	Rationale     string          `gorm:"type:text;not null" json:"rationale"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (BoardSignalCandidate) TableName() string { return "board_signal_candidate" }

// Research progress status values (tasks 4.7): running=研究进行中每轮滚动
// upsert；abandoned=超时/失败终态（行保留供追溯/重试参考）；superseded=成功
// 落库报告后归档（行保留，不删不覆盖报告账本）。
const (
	SignalResearchProgressRunning    = "running"
	SignalResearchProgressAbandoned  = "abandoned"
	SignalResearchProgressSuperseded = "superseded"
)

// BoardSignalResearchProgress is one research job's rolling progress snapshot
// (tasks 4.7「断了不能白跑」). 同 job_id 一行滚动更新（每轮一次 upsert）：
// 轮次/取数/计算计数 + 全量账本 ledger jsonb（calls/calculations/gaps，与
// 报告 appendix 同源结构——服务层复用同一序列化结构体）。超时/失败后行保留
// （abandoned + stop_reason + error），成功落库报告后标 superseded。
// 进展表不回写候选快照、不写 topic_enrichment_result（成功报告仍不可变）。
type BoardSignalResearchProgress struct {
	ID uint `gorm:"primarykey" json:"id"`
	// JobID 是 analysis runner 的任务身份（handler 经 ctx 传入 service）：
	// 唯一——同一 job 永远命中同一行滚动更新。
	JobID           string `gorm:"size:64;not null;uniqueIndex:uq_board_signal_research_progress_job" json:"job_id"`
	SemanticBoardID uint   `gorm:"not null" json:"semantic_board_id"`
	// CandidateID 指向 board_signal_candidate(id)；owner/周期一致性由迁移
	// 20260922_0002 的复合 FK 钉死（与批次/候选同款约束思路）。
	CandidateID      uint   `gorm:"not null;index:idx_board_signal_research_progress_candidate,priority:1" json:"candidate_id"`
	Granularity      string `gorm:"size:10;not null" json:"granularity"`
	Period           string `gorm:"size:12;not null" json:"period"`
	RoundsDone       int    `gorm:"not null;default:0" json:"rounds_done"`
	SourceCalls      int    `gorm:"not null;default:0" json:"source_calls"`
	CalculationCalls int    `gorm:"not null;default:0" json:"calculation_calls"`
	// Ledger 是全量研究账本（calls/calculations/gaps；jsonb）。列默认 '{}'，
	// 服务层始终写完整 appendix 形状对象。
	Ledger     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"ledger"`
	Status     string          `gorm:"size:16;not null;default:running" json:"status"`
	StopReason string          `gorm:"size:32" json:"stop_reason"`
	Error      string          `gorm:"type:text" json:"error"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `gorm:"index:idx_board_signal_research_progress_candidate,priority:2" json:"updated_at"`
}

func (BoardSignalResearchProgress) TableName() string { return "board_signal_research_progress" }

// BoardSignalDisposition is the user's independent per-candidate ignore marker
// (redesign-enrichment-research-workbench D1): 有行即忽略，无行正常。候选内容、
// score、快照与报告均不改；归属从候选取得，不重复存 user/board/period。重复
// 忽略保留首次 ignored_at，恢复即删除标记行（幂等）。
type BoardSignalDisposition struct {
	// CandidateID is the ignored candidate (PK = FK → board_signal_candidate.id;
	// ON DELETE CASCADE added by migration 20261015_0001 — marker is meaningless
	// without its candidate).
	CandidateID uint      `gorm:"primarykey" json:"candidate_id"`
	IgnoredAt   time.Time `gorm:"not null" json:"ignored_at"`
}

func (BoardSignalDisposition) TableName() string { return "board_signal_disposition" }

// Signal candidate list visibility (redesign-enrichment-research-workbench D2):
// visible = 无忽略标记；ignored = 有标记；all = 不筛（旧客户端省略参数的兼容语义）。
const (
	SignalVisibilityVisible = "visible"
	SignalVisibilityIgnored = "ignored"
	SignalVisibilityAll     = "all"
)

// ValidSignalVisibility reports whether v is a listable visibility filter.
func ValidSignalVisibility(v string) bool {
	return v == SignalVisibilityVisible || v == SignalVisibilityIgnored || v == SignalVisibilityAll
}

// Signal candidate view 筛选（D2：在分页前生效）：pending = 无成功报告（live
// 研究候选由 handler 传入排除），readable = 存在成功报告（重研中仍返回），
// all = 不筛（省略时兼容旧客户端）。
const (
	SignalCandidateViewAll      = "all"
	SignalCandidateViewPending  = "pending"
	SignalCandidateViewReadable = "readable"
)

// ValidSignalCandidateView reports whether v is a listable view filter.
func ValidSignalCandidateView(v string) bool {
	return v == SignalCandidateViewAll || v == SignalCandidateViewPending || v == SignalCandidateViewReadable
}

// ValidSignalGranularity reports whether g is a signal-capable granularity.
func ValidSignalGranularity(g string) bool {
	return g == SignalGranularityMonth || g == SignalGranularityYear
}

// ValidSignalPeriod reports whether period matches its granularity's calendar
// shape (month=YYYY-MM with a real month; year=YYYY within the 2000–2100
// bounds used by the lifeline period helpers).
func ValidSignalPeriod(granularity, period string) bool {
	switch granularity {
	case SignalGranularityMonth:
		if len(period) != 7 || period[4] != '-' || !isDigits(period[:4]) || !isDigits(period[5:]) {
			return false
		}
		month := int(period[5]-'0')*10 + int(period[6]-'0')
		return month >= 1 && month <= 12
	case SignalGranularityYear:
		if len(period) != 4 || !isDigits(period) {
			return false
		}
		return period >= "2000" && period <= "2100"
	default:
		return false
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SignalEvidenceRefs decodes the evidence_refs jsonb as a string array
// (stage-2 contract). Non-string elements are skipped; the result is never
// nil so callers can range safely.
func SignalEvidenceRefs(raw json.RawMessage) []string {
	var refs []string
	if err := json.Unmarshal(raw, &refs); err != nil {
		return []string{}
	}
	return refs
}

// ComputeSignalCandidateDedupeKey derives the mechanical within-batch dedupe
// key: normalized signal title + the SORTED unique normalized evidence-ref
// set (design §3: 「同批完全相同标题+规范化证据集合机械去重」). Cross-batch
// similarity is intentionally NOT deduplicated.
func ComputeSignalCandidateDedupeKey(signal string, evidenceRefs []string) string {
	normalizedTitle := strings.Join(strings.Fields(signal), " ")
	refs := make([]string, 0, len(evidenceRefs))
	seen := make(map[string]bool, len(evidenceRefs))
	for _, r := range evidenceRefs {
		n := strings.Join(strings.Fields(r), " ")
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		refs = append(refs, n)
	}
	sort.Strings(refs)
	payload := strings.Join(append([]string{"bsc-v1", normalizedTitle}, refs...), "\x1f")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}
