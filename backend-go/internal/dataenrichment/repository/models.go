package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
)

// SourceType enumerates valid data source types. The type + CHECK mechanism
// is retained as an extension point for future structured external sources,
// but after the financial-direction removal there are NO built-in source types
// (etf_quote/exchange_rate/gdelt_event have been removed). ValidateSourceType
// therefore rejects every value until a new source type is registered here.
type SourceType string

var (
	sourceTypeMu     sync.RWMutex
	validSourceTypes = map[SourceType]bool{}
)

// RegisterSourceType adds st to the set of valid source types. The source_type
// enum is intentionally extensible (spec "板块数据源绑定"): future structured
// external sources register their type at wiring time rather than hard-coding a
// constant here. The removed built-in financial types (etf_quote /
// exchange_rate / gdelt_event) are NOT registered and stay rejected.
func RegisterSourceType(st SourceType) {
	sourceTypeMu.Lock()
	defer sourceTypeMu.Unlock()
	validSourceTypes[st] = true
}

// ValidateSourceType returns an error if sourceType is not a known value.
func ValidateSourceType(sourceType string) error {
	sourceTypeMu.RLock()
	defer sourceTypeMu.RUnlock()
	if validSourceTypes[SourceType(sourceType)] {
		return nil
	}
	return errors.New("unknown source_type: " + sourceType)
}

// ── Models ───────────────────────────────────────────────────────────────────

// BoardDataSource binds a data source to a semantic board.
// Table: board_data_sources  UNIQUE(semantic_board_id, source_type)
type BoardDataSource struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	SemanticBoardID uint           `gorm:"uniqueIndex:idx_board_src;not null" json:"semantic_board_id"`
	SourceType      string         `gorm:"size:40;uniqueIndex:idx_board_src;not null" json:"source_type"`
	Config          map[string]any `gorm:"type:jsonb;serializer:json;default:'{}'" json:"config"`
	Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (BoardDataSource) TableName() string { return "board_data_sources" }

// LifelineGranularity enumerates context granularity levels.
type LifelineGranularity string

const (
	GranularityWeek  LifelineGranularity = "week"
	GranularityMonth LifelineGranularity = "month"
	GranularityYear  LifelineGranularity = "year"
	GranularityAll   LifelineGranularity = "all"
)

// TopicLifelineContext stores the news narrative summary at a given granularity + period.
// Table: topic_lifeline_context  UNIQUE(persistent_topic_id, granularity, period)
// See design.md §2.1: period-archival storage (one row per period, not rolling overwrite).
type TopicLifelineContext struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	PersistentTopicID uint      `gorm:"uniqueIndex:idx_topic_gran_period;not null" json:"persistent_topic_id"`
	Granularity       string    `gorm:"size:10;uniqueIndex:idx_topic_gran_period;not null" json:"granularity"`
	Period            string    `gorm:"size:12;uniqueIndex:idx_topic_gran_period;not null" json:"period"`
	Content           string    `gorm:"type:text;not null" json:"content"`
	AsOfDate          time.Time `gorm:"type:date;not null" json:"as_of_date"`
	Source            string    `gorm:"size:12;not null;default:manual" json:"source"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (TopicLifelineContext) TableName() string { return "topic_lifeline_context" }

const (
	ResultKindTopicAnalysis       = "topic_analysis"
	ResultKindBoardBrief          = "board_brief"
	ResultKindBoardInvestigation  = "board_investigation"
	ResultKindLegacyBoardAnalysis = "legacy_board_analysis"
)

// TopicEnrichmentResult is an immutable snapshot of one enhancement run.
// Table: topic_enrichment_result
// analysis_scope: "topic" (single-lane, PersistentTopicID set) or "board"
// (board-level, SemanticBoardID set, PersistentTopicID NULL).
type TopicEnrichmentResult struct {
	ID                  uint            `gorm:"primarykey" json:"id"`
	PersistentTopicID   *uint           `gorm:"index" json:"persistent_topic_id"`
	SemanticBoardID     *uint           `gorm:"index" json:"semantic_board_id"`
	AnalysisScope       string          `gorm:"size:12;not null;default:'topic'" json:"analysis_scope"`
	ResultKind          string          `gorm:"size:32;not null;default:'topic_analysis'" json:"result_kind"`
	ParentResultID      *uint           `json:"parent_result_id"`
	QuestionKey         *string         `gorm:"size:64" json:"question_key"`
	EvolutionAssessment string          `gorm:"type:text" json:"evolution_assessment"`
	Sectors             json.RawMessage `gorm:"type:jsonb" json:"sectors"`
	CausalChain         string          `gorm:"type:text" json:"causal_chain"`
	ToolCalls           json.RawMessage `gorm:"type:jsonb" json:"tool_calls"`
	InputSnapshot       json.RawMessage `gorm:"type:jsonb" json:"input_snapshot"`
	SessionID           string          `gorm:"size:120" json:"session_id"`
	CreatedAt           time.Time       `json:"created_at"`
}

func (TopicEnrichmentResult) TableName() string { return "topic_enrichment_result" }

// ComputeQuestionKey normalizes display-equivalent questions by trimming and
// folding consecutive Unicode whitespace to one ASCII space, then returns the
// full lowercase SHA-256 hex digest. The original question remains in sectors.
func ComputeQuestionKey(question string) string {
	normalized := strings.Join(strings.Fields(question), " ")
	digest := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(digest[:])
}

// IsValidQuestionKey reports whether key is a full lowercase SHA-256 hex digest.
func IsValidQuestionKey(key string) bool {
	if len(key) != sha256.Size*2 || key != strings.ToLower(key) {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

// EffectiveResultKind keeps in-memory historical fixtures readable before the
// explicit migration backfill has populated result_kind.
func EffectiveResultKind(result *TopicEnrichmentResult) string {
	if result.ResultKind != "" {
		return result.ResultKind
	}
	if result.AnalysisScope == "board" || result.SemanticBoardID != nil {
		return ResultKindLegacyBoardAnalysis
	}
	return ResultKindTopicAnalysis
}

// TopicIDMatches reports whether an enrichment row belongs to the given topic.
// Board-scoped rows (nil topic id) never match a topic id.
func TopicIDMatches(pid *uint, topicID uint) bool { return pid != nil && *pid == topicID }

// BoardIDMatches reports whether an enrichment row belongs to the given board.
// Topic-scoped rows (nil board id) never match a board id.
func BoardIDMatches(bid *uint, boardID uint) bool { return bid != nil && *bid == boardID }

// TopicIDPtr returns a pointer to id — convenience for constructing topic-
// scoped enrichment rows (PersistentTopicID is nullable for board-scope rows).
func TopicIDPtr(id uint) *uint { return &id }

// BoardIDPtr returns a pointer to id (board-scope rows).
func BoardIDPtr(id uint) *uint { return &id }

// TopicEnrichmentReview records a comparison between two result snapshots.
// Table: topic_enrichment_review
// Board-level reviews carry SemanticBoardID and leave PersistentTopicID NULL.
type TopicEnrichmentReview struct {
	ID                uint            `gorm:"primarykey" json:"id"`
	PersistentTopicID *uint           `gorm:"index" json:"persistent_topic_id"`
	SemanticBoardID   *uint           `gorm:"index" json:"semantic_board_id"`
	PrevResultID      *uint           `gorm:"index" json:"prev_result_id"`
	CurrResultID      uint            `gorm:"not null;index" json:"curr_result_id"`
	Verdict           json.RawMessage `gorm:"type:jsonb" json:"verdict"`
	DeviationSummary  string          `gorm:"type:text;not null" json:"deviation_summary"`
	AffectedContext   string          `gorm:"size:10" json:"affected_context"`
	Confidence        *float64        `json:"confidence"`
	Applied           bool            `gorm:"not null;default:false" json:"applied"`
	Source            string          `gorm:"size:12;not null;default:llm_assisted" json:"source"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (TopicEnrichmentReview) TableName() string { return "topic_enrichment_review" }

// StockDebateResult stores FinGenius multi-role debate output per stock.
// See design.md §4.2b: append-only by (result_id, sector, code).
// Table: stock_debate_result
type StockDebateResult struct {
	ID                      uint            `gorm:"primarykey" json:"id"`
	TopicEnrichmentResultID uint            `gorm:"not null;index" json:"topic_enrichment_result_id"`
	PersistentTopicID       uint            `gorm:"not null;index" json:"persistent_topic_id"`
	Sector                  string          `gorm:"size:80;not null" json:"sector"`
	Code                    string          `gorm:"size:20;not null" json:"code"`
	Name                    string          `gorm:"size:60" json:"name"`
	Verdict                 string          `gorm:"size:8;not null" json:"verdict"`
	Consensus               string          `gorm:"size:12" json:"consensus"`
	Agents                  json.RawMessage `gorm:"type:jsonb" json:"agents"`
	Votes                   json.RawMessage `gorm:"type:jsonb" json:"votes"`
	FingeniusResearch       json.RawMessage `gorm:"type:jsonb" json:"fingenius_research"`
	FingeniusBattle         json.RawMessage `gorm:"type:jsonb" json:"fingenius_battle"`
	FingeniusTaskID         string          `gorm:"size:120" json:"fingenius_task_id"`
	DistillStatus           string          `gorm:"size:12;not null;default:done" json:"distill_status"`
	HTMLContent             string          `gorm:"type:text" json:"html_content"`
	CreatedAt               time.Time       `json:"created_at"`
}

func (StockDebateResult) TableName() string { return "stock_debate_result" }

// TopicEnrichmentQA stores an append-only report follow-up Q&A round tied to
// one immutable TopicEnrichmentResult snapshot. Multiple rounds may share a
// result_id; the report itself is never rewritten (报告不可变).
// Table: topic_enrichment_qa
//
// Added by causal-analysis-agent: 分析目标从「演进定位」改为「探索判断 agent」，
// 报告追问交互层需要独立的新表，旧演进定位数据(result/review)由迁移清空。
type TopicEnrichmentQA struct {
	ID                      uint            `gorm:"primarykey" json:"id"`
	TopicEnrichmentResultID uint            `gorm:"not null;index" json:"topic_enrichment_result_id"`
	Question                string          `gorm:"type:text;not null" json:"question"`
	Answer                  string          `gorm:"type:text" json:"answer"`
	ToolCalls               json.RawMessage `gorm:"type:jsonb" json:"tool_calls"`
	Source                  string          `gorm:"size:12;not null;default:qa" json:"source"`
	// Sedimented marks a Q&A round the user manually pinned as a durable note.
	// Sediment only flips this flag on the qa row; the report itself (result) is
	// never rewritten (业务约束#2: result 不可变). Default false.
	Sedimented bool      `gorm:"not null;default:false" json:"sedimented"`
	CreatedAt  time.Time `json:"created_at"`
}

func (TopicEnrichmentQA) TableName() string { return "topic_enrichment_qa" }

// ReferenceRole is a reusable methodology profile injected into the analysis
// prompt as an appendix (design D5: 方法非事实 — roles shape HOW the agent
// analyzes, never inject facts/evidence).
// Table: reference_roles
// Added by board-level-deep-analysis.
type ReferenceRole struct {
	ID      uint   `gorm:"primarykey" json:"id"`
	Name    string `gorm:"size:120;uniqueIndex;not null" json:"name"`
	Title   string `gorm:"size:200" json:"title"`
	Content string `gorm:"type:text;not null" json:"content"`
	// No gorm default tag: a default tag makes GORM skip zero-value (false)
	// fields on INSERT, silently flipping Enabled=false creations to the DB
	// default. Handlers set Enabled explicitly (true when omitted).
	Enabled   bool      `gorm:"not null" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ReferenceRole) TableName() string { return "reference_roles" }

// AnalysisMethodSelectionMeta declares when a method is suitable, when it must
// not be selected, which evidence it needs, and its known failure modes. The
// concrete type keeps JSONB payloads validated and avoids unstructured maps.
type AnalysisMethodSelectionMeta struct {
	ApplicableWhen   []string `json:"applicable_when"`
	AvoidWhen        []string `json:"avoid_when"`
	RequiredEvidence []string `json:"required_evidence"`
	FailureModes     []string `json:"failure_modes"`
}

func (m *AnalysisMethodSelectionMeta) normalize() {
	if m.ApplicableWhen == nil {
		m.ApplicableWhen = []string{}
	}
	if m.AvoidWhen == nil {
		m.AvoidWhen = []string{}
	}
	if m.RequiredEvidence == nil {
		m.RequiredEvidence = []string{}
	}
	if m.FailureModes == nil {
		m.FailureModes = []string{}
	}
}

// AnalysisMethod is a globally managed process/checklist card. It is never
// injected globally: future investigation selection loads summaries first and
// full Content only for explicitly selected IDs.
type AnalysisMethod struct {
	ID            uint                        `gorm:"primarykey" json:"id"`
	Name          string                      `gorm:"size:120;uniqueIndex;not null" json:"name"`
	Title         string                      `gorm:"size:200" json:"title"`
	Summary       string                      `gorm:"type:text" json:"summary"`
	SelectionMeta AnalysisMethodSelectionMeta `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"selection_meta"`
	Content       string                      `gorm:"type:text;not null" json:"content"`
	Enabled       bool                        `gorm:"not null" json:"enabled"`
	Legacy        bool                        `gorm:"not null;default:false" json:"legacy"`
	DeletedAt     gorm.DeletedAt              `gorm:"index" json:"deleted_at,omitempty"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}

func (AnalysisMethod) TableName() string { return "analysis_methods" }

func (m *AnalysisMethod) BeforeSave(_ *gorm.DB) error {
	m.SelectionMeta.normalize()
	return nil
}

func (m *AnalysisMethod) AfterFind(_ *gorm.DB) error {
	m.SelectionMeta.normalize()
	return nil
}

func init() {
	database.RegisterModels(
		&BoardDataSource{},
		&TopicLifelineContext{},
		&TopicEnrichmentResult{},
		&TopicEnrichmentReview{},
		&StockDebateResult{},
		&TopicEnrichmentQA{},
		&ReferenceRole{},
		&AnalysisMethod{},
		&CrossBoardRelationRun{},
		&CrossBoardRelation{},
	)
}
