package entity

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	repository "syntopica-backend/internal/tagmanagement/repository"
)

// Resolution chain orchestration (design D5/D6/D7/D8).
//
// ONE mention walks: idempotency check → (gate) re-read latest pool → three
// candidate paths → second-round choice → atomic register — with technical
// failure and legal none STRICTLY separated:
//
//   - technical failure (illegal ID / unparsable / service failure / timeout,
//     including gate-wait timeout and query-embed failure): NOT degraded to
//     none, NO new entity, NO half-writes, ≤ MaxAttempts tries, then the
//     mention stays 未归一 with the reason recorded. Tag persistence is never
//     blocked (the hook owns that contract).
//   - legal none: atomic create in ONE short transaction (identity row +
//     mention evidence row + aux provenance rows); any step fails → full
//     rollback. The transaction NEVER spans a model call.
//
// Concurrency (D6): a single GLOBAL session advisory lock serializes
// 「re-read → second-round → atomic register」across processes. The gate must
// be held while re-reading the pool — an outside-gate stale none is void.
// The gate does NOT promise global uniqueness: a post-re-read legal none is
// allowed to create (duplicates are quantified by the evaluation, never
// promised away by the lock).

// ChatFunc is the second-round model call. Production impl goes through the
// unified airouter router (capability=topic_tagging, non-empty operation —
// audit rows land automatically per attempt); tests inject fakes.
type ChatFunc func(ctx context.Context, req airouter.ChatRequest) (string, error)

// EmbedFunc embeds one query text and reports the space triplet of the
// vector it produced (dim = len(vec)). Tests inject deterministic fakes.
type EmbedFunc func(ctx context.Context, text string) (QueryVector, error)

// KeyGenFunc mints short opaque entity keys. Injectable for tests.
type KeyGenFunc func() (string, error)

// ResolverConfig bounds the resolution. Every duration is finite — no
// unbounded waits, no unbounded optimistic retries.
type ResolverConfig struct {
	TopK            int           // per-path top-K (tunable eval config, NOT an identity threshold)
	MaxOptions      int           // merged candidate cap (same status)
	MaxAttempts     int           // second-round tries per mention (spec: ≤ 2)
	CallTimeout     time.Duration // one model call
	GateWaitTimeout time.Duration // waiting for the global gate
	TotalTimeout    time.Duration // whole resolution
}

func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		TopK:            10,
		MaxOptions:      12,
		MaxAttempts:     2,
		CallTimeout:     60 * time.Second,
		GateWaitTimeout: 120 * time.Second,
		TotalTimeout:    5 * time.Minute,
	}
}

// secondRoundOperation is the audit operation for the choice question
// (ai-summary red line: non-empty operation; capability stays topic_tagging
// — no new capability enum, no digest_polish borrowing).
const secondRoundOperation = "tagmanagement.entity_subject.resolve"

// MentionRequest is one subject mention to resolve.
type MentionRequest struct {
	MentionText     string // verbatim mention (first-round real output)
	SubjectDesc     string // stable short identity description (may be empty)
	ContextSnippet  string // context fragment for the choice question
	SourceTagID     *uint  // originating topic_tags.id (nil = eval synthetic)
	SourceArticleID *uint
	// AuxLabelIDs are the provenance matches ALREADY decided upstream by the
	// exact rule「normalized aux label text == normalized mention text」.
	AuxLabelIDs []uint
	SessionID   string // audit grouping (eval runs use the run id)
}

// Resolution statuses.
const (
	StatusCreated    = "created"    // legal none → new entity
	StatusReused     = "reused"     // second round picked an existing entity
	StatusIdempotent = "idempotent" // same-source replay reused the stored decision
	StatusUnresolved = "unresolved" // technical failure — mention stays 未归一
)

// Resolution is the outcome of one mention resolution.
type Resolution struct {
	Status    ResolutionStatus
	EntityID  uint
	EntityKey string
	Reason    string // failure reason (unresolved) / none cause (created)
	Attempts  int    // second-round attempts actually made (0 for idempotent / no-candidate paths)
}

// ResolutionStatus types the outcome.
type ResolutionStatus = string

// Resolver runs the chain. Construct via NewResolver (tests) or
// DefaultResolver (production singleton).
type Resolver struct {
	repo   *repository.EntityRepository
	chat   ChatFunc
	embed  EmbedFunc
	keyGen KeyGenFunc
	cfg    ResolverConfig
}

// NewResolver builds a resolver with explicit dependencies (tests).
func NewResolver(repo *repository.EntityRepository, chat ChatFunc, embed EmbedFunc, cfg ResolverConfig) *Resolver {
	return &Resolver{
		repo:   repo,
		chat:   chat,
		embed:  embed,
		keyGen: DefaultEntityKeyGen,
		cfg:    cfg,
	}
}

// DefaultEntityKeyGen mints a short opaque key ("E" + 8 base32 chars).
// Opaque on purpose: the model echoes it back verbatim; it leaks nothing
// about global counts or identity ordering.
func DefaultEntityKeyGen() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("entity key entropy: %w", err)
	}
	return "E" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// ResolveMention runs the full chain for one mention. It NEVER returns an
// error for per-mention technical failures — those come back as
// StatusUnresolved with Reason set, so callers (the tag hook, the eval
// runner) treat resolution failure as data, not as an abort.
func (rs *Resolver) ResolveMention(ctx context.Context, req MentionRequest) *Resolution {
	ctx, cancel := context.WithTimeout(ctx, rs.cfg.TotalTimeout)
	defer cancel()

	mentionText := strings.TrimSpace(req.MentionText)
	mentionKey := textutil.NormalizeEntityText(mentionText)
	if mentionKey == "" {
		// 空/纯空白：不归一、不报错、不写任何行（test-cases 变体组「输入」）。
		return &Resolution{Status: StatusUnresolved, Reason: "empty or blank mention"}
	}

	// Idempotency (outside the gate — a stored decision never depends on the
	// latest pool; same source + same mention replays it without any model
	// call). Spec「同来源同提及重放幂等」.
	if m, ok := rs.repo.FindDecisionByIdempotency(ctx, req.SourceTagID, mentionKey); ok {
		return &Resolution{Status: StatusIdempotent, EntityID: m.EntityID, Attempts: 0}
	}

	// Query vectors OUTSIDE the gate (they don't depend on the pool); the
	// pool re-read + choice + register happen INSIDE. Embed failure is a
	// technical failure — never a silent degrade to name-hit-only (an
	// incomplete recall would mint wrong entities).
	nameText, nameDescText := EmbedQueryTexts(mentionText, strings.TrimSpace(req.SubjectDesc))
	nameQuery, err := rs.embed(ctx, nameText)
	if err != nil {
		return &Resolution{Status: StatusUnresolved, Reason: "query embed (name path) failed: " + err.Error()}
	}
	var nameDescQuery *QueryVector
	if nameDescText != "" {
		q, err := rs.embed(ctx, nameDescText)
		if err != nil {
			return &Resolution{Status: StatusUnresolved, Reason: "query embed (name+desc path) failed: " + err.Error()}
		}
		nameDescQuery = &q
	}

	// Acquire the GLOBAL resolution gate (bounded wait; timeout ends as an
	// explicit technical failure).
	gateCtx, gateCancel := context.WithTimeout(ctx, rs.cfg.GateWaitTimeout)
	release, err := rs.repo.AcquireEntityResolveGate(gateCtx)
	gateCancel()
	if err != nil {
		return &Resolution{Status: StatusUnresolved, Reason: "gate: " + err.Error()}
	}
	defer release() // finally: release + hand the dedicated connection back

	// === INSIDE THE GATE: re-read everything; outside-gate conclusions are void ===

	// Re-check idempotency: a concurrent process may have registered this
	// exact (source, mention) while we waited.
	if m, ok := rs.repo.FindDecisionByIdempotency(ctx, req.SourceTagID, mentionKey); ok {
		return &Resolution{Status: StatusIdempotent, EntityID: m.EntityID, Attempts: 0}
	}

	// Re-read the LATEST pool and run all three candidate paths.
	pool, err := rs.repo.LoadEntityPool(ctx)
	if err != nil {
		return &Resolution{Status: StatusUnresolved, Reason: "reload entity pool: " + err.Error()}
	}
	poolByID := make(map[uint]*tagmodels.EntityRegistry, len(pool))
	for i := range pool {
		poolByID[pool[i].ID] = &pool[i]
	}
	namePath := SearchNamePath(pool, nameQuery, rs.cfg.TopK)
	var nameDescPath []ScoredEntity
	if nameDescQuery != nil {
		nameDescPath = SearchNameDescPath(pool, *nameDescQuery, rs.cfg.TopK)
	}
	registeredHits, err := rs.repo.FindEntityIDsByMentionKey(ctx, mentionKey)
	if err != nil {
		return &Resolution{Status: StatusUnresolved, Reason: "registered-name lookup: " + err.Error()}
	}
	candidates := MergeCandidates(namePath, nameDescPath, registeredHits, poolByID, rs.cfg.MaxOptions)

	// Second round: empty candidate set → trivially none (the model has
	// nothing to choose from; a call would burn budget for a forced answer).
	// Non-empty → the choice question, attempts bounded by MaxAttempts.
	var answerKey string
	var isNone bool
	attempts := 0
	if len(candidates) == 0 {
		isNone = true
	} else {
		answerKey, isNone, attempts = rs.secondRound(ctx, req, mentionText, candidates)
		if answerKey == "" && !isNone {
			return &Resolution{Status: StatusUnresolved, Reason: "second round exhausted attempts (technical failure)", Attempts: attempts}
		}
	}

	mention := &tagmodels.EntityMention{
		MentionText:     mentionText,
		MentionKey:      mentionKey,
		SourceTagID:     req.SourceTagID,
		SourceArticleID: req.SourceArticleID,
	}

	if isNone {
		// Legal none → atomic create (identity + evidence + provenance in
		// ONE short transaction; rollback on any step; no model call inside).
		key, kerr := rs.keyGen()
		if kerr != nil {
			return &Resolution{Status: StatusUnresolved, Reason: "key gen: " + kerr.Error()}
		}
		registry := &tagmodels.EntityRegistry{
			EntityKey:     key,
			CanonicalName: mentionText, // first mention text seeds the canonical name (no unique constraint)
			StableDesc:    strings.TrimSpace(req.SubjectDesc),
			// Query vectors double as the entity's own representations:
			// canonical name == mention text on the create path.
			NameEmbedding:            strPtr(repository.EntityFloatsToPgVector(nameQuery.Vec)),
			NameEmbeddingModel:       nameQuery.Triplet.Model,
			NameEmbeddingVersion:     nameQuery.Triplet.Version,
			NameEmbeddingDim:         len(nameQuery.Vec),
			NameDescEmbeddingModel:   "",
			NameDescEmbeddingVersion: "",
			NameDescEmbeddingDim:     0,
		}
		if nameDescQuery != nil {
			registry.NameDescEmbedding = strPtr(repository.EntityFloatsToPgVector(nameDescQuery.Vec))
			registry.NameDescEmbeddingModel = nameDescQuery.Triplet.Model
			registry.NameDescEmbeddingVersion = nameDescQuery.Triplet.Version
			registry.NameDescEmbeddingDim = len(nameDescQuery.Vec)
		}
		if err := rs.repo.CreateEntityAtomic(ctx, registry, mention, req.AuxLabelIDs); err != nil {
			return &Resolution{Status: StatusUnresolved, Reason: "atomic create failed (rolled back): " + err.Error(), Attempts: attempts}
		}
		reason := "legal none"
		if len(candidates) == 0 {
			reason = "no candidates in pool"
		}
		return &Resolution{Status: StatusCreated, EntityID: registry.ID, EntityKey: registry.EntityKey, Reason: reason, Attempts: attempts}
	}

	// Legal candidate key → attach the mention evidence to that entity.
	entityID := uint(0)
	for _, c := range candidates {
		if c.EntityKey == answerKey {
			entityID = c.EntityID
			break
		}
	}
	if entityID == 0 {
		return &Resolution{Status: StatusUnresolved, Reason: "answer key vanished from candidates (technical failure)", Attempts: attempts}
	}
	if err := rs.repo.AttachMentionAtomic(ctx, entityID, mention, req.AuxLabelIDs); err != nil {
		return &Resolution{Status: StatusUnresolved, Reason: "attach mention failed (rolled back): " + err.Error(), Attempts: attempts}
	}
	return &Resolution{Status: StatusReused, EntityID: entityID, EntityKey: answerKey, Attempts: attempts}
}

// secondRound runs the bounded choice-question loop. Returns
// (entityKey,false) on a legal pick, ("",true) on a legal none, and
// ("",false) when attempts are exhausted — the caller maps that to a
// technical failure (未归一), NEVER to none.
func (rs *Resolver) secondRound(ctx context.Context, req MentionRequest, mentionText string, candidates []Candidate) (string, bool, int) {
	system, user := BuildSecondRoundPrompts(mentionText, strings.TrimSpace(req.SubjectDesc), req.ContextSnippet, candidates)
	schema := SecondRoundSchema(candidates)
	legal := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		legal[c.EntityKey] = true
	}

	metadata := map[string]any{
		"operation":  "entity_subject_resolve",
		"mention":    mentionText,
		"candidates": len(candidates),
	}
	if req.SourceTagID != nil {
		metadata["tag_id"] = *req.SourceTagID
	}
	if req.SessionID != "" {
		metadata["session_id"] = req.SessionID
	}

	for attempt := 1; attempt <= rs.cfg.MaxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, rs.cfg.CallTimeout)
		content, err := rs.chat(callCtx, airouter.ChatRequest{
			Operation:  secondRoundOperation,
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: user},
			},
			Temperature: nil,
			JSONMode:    true,
			JSONSchema:  schema,
			Metadata:    metadata,
			SessionID:   req.SessionID,
		})
		cancel()
		if err != nil {
			logging.Warnf("entity resolve: second-round call failed (attempt %d/%d): %v", attempt, rs.cfg.MaxAttempts, err)
			continue
		}
		key, isNone, perr := ParseSecondRoundAnswer(content, legal)
		if perr != nil {
			logging.Warnf("entity resolve: second-round answer invalid (attempt %d/%d): %v", attempt, rs.cfg.MaxAttempts, perr)
			continue
		}
		return key, isNone, attempt
	}
	return "", false, rs.cfg.MaxAttempts
}

func strPtr(s string) *string { return &s }

// trimMention trims leading/trailing whitespace, preserving the verbatim
// interior — the mention text is EVIDENCE and must stay byte-faithful.
