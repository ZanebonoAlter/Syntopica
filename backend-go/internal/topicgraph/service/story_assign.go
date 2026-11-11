package service

import (
	"context"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// Story assignment uses deterministic fingerprint rules only for merges.
// Vector distance is recall-only (and a stage-hit tiebreaker). Explicit key
// mismatches remain absolute vetoes. Untrusted/keyless evidence creates a
// new line; the optional classifier can identify ROUNDUP but cannot attach.

// StoryCandidate is one recalled board_stories row (2c's recall query,
// design §5.1: same board, 14-day window, merged_into IS NULL, active,
// vector top-N with dist <= 0.5 — wide net, this file does the precision).
type StoryCandidate struct {
	// Title and Summary are evidence from the latest thread, not the whole story.
	Title      string
	Summary    string
	ID         uint
	Subject    string
	SubjectKey string
	Action     string
	Object     string
	OccurredAt string
	// PeriodDate is the report date of the representative thread, used only when occurred_at is absent.
	PeriodDate time.Time
	// ArticleIDs is carried for 2c's evidence bookkeeping; unused here.
	ArticleIDs []uint
	// Distance is the recall embedding distance (ascending = nearer); the
	// SAME_STORY tier uses it only to pick the nearest among several hits.
	Distance float64
	// Origin is rule|model|manual (board_stories.origin), informational.
	Origin string
}

// AssignmentInput is the new event (daily_report_thread) plus its
// fingerprint verdict from ParseEventFingerprint.
type AssignmentInput struct {
	ThreadID   uint
	Title      string
	Summary    string
	Subject    string
	SubjectKey string
	Action     string
	Object     string
	OccurredAt string
	// PeriodDate is the report date of the representative thread, used only when occurred_at is absent.
	PeriodDate time.Time
	// FingerprintTrusted mirrors EventTagFingerprint.Trusted.
	FingerprintTrusted bool
	ArticleIDs         []uint
	// Embedding is the event's title embedding — recall-only, unused here.
	Embedding []float64
}

// StoryDecision values — the spec's four-way classification.
type StoryDecision string

const (
	StoryDecisionSameOccurrence StoryDecision = "SAME_OCCURRENCE" // 同一次发生的复述
	StoryDecisionSameStory      StoryDecision = "SAME_STORY"      // 同一事件线的新阶段
	StoryDecisionUnrelated      StoryDecision = "UNRELATED"       // 与候选均非同一事件 → 新建
	StoryDecisionRoundup        StoryDecision = "ROUNDUP"         // 综合稿/盘点 → 不建身份
)

// AssignmentResult is DecideAssignment's outcome. Convention for 2c:
// StoryIDs empty → create a new story line (whatever the Decision label —
// ROUNDUP keeps its label so the caller can mark scope). SAME_STORY results
// always carry exactly one StoryID and IsDevelopment=true.
type AssignmentResult struct {
	Decision StoryDecision
	// StoryIDs carries ALL SAME_OCCURRENCE hits (rule tier) so 2c can merge
	// duplicate story lines linked by one occurrence.
	StoryIDs []uint
	// IsDevelopment marks a SAME_STORY attach as a stage development
	// (daily_report_threads.is_development).
	IsDevelopment bool
	// Origin is repository.StoryOriginRule for deterministic tiers,
	// StoryOriginModel for LLM non-merge verdicts.
	Origin string
	// Reason is a short english enum for logging.
	Reason string
	// Confidence: rule tiers → 1, LLM → classifier confidence.
	Confidence float64
}

// StoryClassifier is the injectable classifier, used only for ROUNDUP decisions (production impl in
// story_assign_llm.go).
type StoryClassifier func(ctx context.Context, input AssignmentInput, candidates []StoryCandidate) (candidateID uint, decision StoryDecision, confidence float64, reason string, err error)

// StoryClassifierMinConfidence — below this the LLM verdict degrades to a
// new story line without preserving the ROUNDUP label.
const StoryClassifierMinConfidence = 0.75

// Assignment reason enums (logging; not part of any contract).
const (
	assignReasonNoCandidates             = "no_candidates"
	assignReasonMergeGateUntrusted       = "merge_gate_untrusted"
	assignReasonFingerprintMatch         = "fingerprint_match"
	assignReasonStageDevelopment         = "stage_development"
	assignReasonStageDateVeto            = "stage_date_veto"
	assignReasonSubjectVeto              = "subject_veto"
	assignReasonObjectVeto               = "object_veto"
	assignReasonLLMError                 = "llm_error"
	assignReasonLLMLowConfidence         = "llm_low_confidence"
	assignReasonLLMUnrelated             = "llm_unrelated"
	assignReasonLLMRoundup               = "llm_roundup"
	assignReasonLLMInvalidID             = "llm_invalid_candidate"
	assignReasonLLMSameOccurrenceBlocked = "llm_same_occurrence_blocked"
	assignReasonLLMSameStoryBlocked      = "llm_same_story_blocked"
)

// DecideAssignment applies the subject veto, then the trusted/keyed merge
// gate, then strict occurrence and dated stage rules. Reverse actions skip
// stage merging. Only unresolved gated candidates may be checked for ROUNDUP;
// no model verdict may select an existing story line.
func DecideAssignment(ctx context.Context, input AssignmentInput, candidates []StoryCandidate, classifier StoryClassifier) AssignmentResult {
	// Pre-filter — the subject one-vote veto runs before every tier, even
	// for untrusted fingerprints: a key mismatch is a rule fact the LLM may
	// not revisit (spec layer 1: "SHALL NOT 被向量相似度或 object 相近推翻").
	kept, hadSubjectVeto := subjectPreFilter(input, candidates)
	if len(kept) == 0 {
		if hadSubjectVeto {
			return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonSubjectVeto)
		}
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonNoCandidates)
	}

	if !input.FingerprintTrusted || !assignmentFingerprintComplete(input) {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonMergeGateUntrusted)
	}
	// A missing candidate key is not evidence of shared identity. Drop it
	// before either the rules or classifier can consider the candidate.
	keyed := make([]StoryCandidate, 0, len(kept))
	for _, c := range kept {
		if textutil.NormalizeSubjectKey(c.SubjectKey) != "" {
			keyed = append(keyed, c)
		}
	}
	kept = keyed
	if len(kept) == 0 {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonMergeGateUntrusted)
	}

	var occurrenceIDs []uint
	storyIdx := -1
	subjectVetoed := false
	stageDateVetoed := false
	var undecidable []StoryCandidate
	for i, c := range kept {
		switch classifyStoryCandidate(input, c) {
		case verdictSubjectVeto:
			// Defensive: the pre-filter already dropped these, so reaching
			// here means a future caller bypassed it — still veto, never merge.
			subjectVetoed = true
		case verdictStageDateVeto:
			stageDateVetoed = true
		case verdictObjectVeto:
			// No flag needed: subject veto dominates the reason label; an
			// all-object-vetoed set lands in the final object_veto branch.
		case verdictOccurrence:
			occurrenceIDs = append(occurrenceIDs, c.ID)
		case verdictStory:
			if storyIdx < 0 || c.Distance < kept[storyIdx].Distance {
				storyIdx = i
			}
		case verdictUndecidable:
			undecidable = append(undecidable, c)
		}
	}

	if len(occurrenceIDs) > 0 {
		return AssignmentResult{
			Decision:   StoryDecisionSameOccurrence,
			StoryIDs:   occurrenceIDs,
			Origin:     repository.StoryOriginRule,
			Reason:     assignReasonFingerprintMatch,
			Confidence: 1,
		}
	}
	if storyIdx >= 0 {
		return AssignmentResult{
			Decision:      StoryDecisionSameStory,
			StoryIDs:      []uint{kept[storyIdx].ID},
			IsDevelopment: true,
			Origin:        repository.StoryOriginRule,
			Reason:        assignReasonStageDevelopment,
			Confidence:    1,
		}
	}
	if len(undecidable) > 0 {
		// Only non-vetoed candidates reach the LLM — a veto is absolute and
		// must not be overturned by the model tier.
		return classifyAssignment(ctx, input, undecidable, classifier)
	}
	if subjectVetoed {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonSubjectVeto)
	}
	if stageDateVetoed {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonStageDateVeto)
	}
	return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonObjectVeto)
}

// candidateVerdict is one candidate's rule-tier outcome.
type candidateVerdict int

const (
	verdictSubjectVeto   candidateVerdict = iota // keys differ (absolute)
	verdictStageDateVeto                         // insufficient temporal evidence for stage merge
	verdictObjectVeto                            // keys equal, object differs
	verdictOccurrence                            // key+action+object all match
	verdictStory                                 // key+object match, action differs (non-reverse)
	verdictUndecidable                           // no rule basis (keyless side / reverse action); never a rule merge
)

// subjectPreFilter drops candidates whose normalized subject key differs
// from the input's (both sides non-empty) — the one-vote veto applied
// before every tier so neither the rule loop nor the LLM ever sees a
// key-mismatched candidate. Keyless sides compare nothing: an empty key can
// neither match nor veto (test-cases §3.1: never "missing = equal").
func subjectPreFilter(input AssignmentInput, candidates []StoryCandidate) (kept []StoryCandidate, hadSubjectVeto bool) {
	inKey := textutil.NormalizeSubjectKey(input.SubjectKey)
	kept = make([]StoryCandidate, 0, len(candidates))
	for _, c := range candidates {
		candKey := textutil.NormalizeSubjectKey(c.SubjectKey)
		if inKey != "" && candKey != "" && inKey != candKey {
			hadSubjectVeto = true
			continue
		}
		kept = append(kept, c)
	}
	return kept, hadSubjectVeto
}

// classifyStoryCandidate applies tiers ①-④ to a single candidate. Keyless
// sides compare nothing: an empty key can neither match nor veto (never
// "missing field equals equal"). Callers pre-filter subject vetoes; the
// verdictSubjectVeto branch stays as defense-in-depth so the function is
// correct even when called directly.
func classifyStoryCandidate(input AssignmentInput, c StoryCandidate) candidateVerdict {
	inKey := textutil.NormalizeSubjectKey(input.SubjectKey)
	candKey := textutil.NormalizeSubjectKey(c.SubjectKey)
	if inKey != "" && candKey != "" && inKey != candKey {
		return verdictSubjectVeto
	}
	if inKey == "" || candKey == "" {
		return verdictUndecidable
	}
	if !matchEntityText(input.Object, c.Object) {
		return verdictObjectVeto
	}
	if !sameObjectStrict(input.Object, c.Object) {
		return verdictObjectVeto
	}
	if matchEntityText(input.Action, c.Action) {
		return verdictOccurrence
	}
	if IsReverseAction(input.Action, c.Action) {
		return verdictUndecidable
	}
	if !stageDatesWithinOneDay(input, c) {
		return verdictStageDateVeto
	}
	return verdictStory
}

// classifyAssignment only preserves ROUNDUP classification. All other model
// verdicts, including SAME_OCCURRENCE and SAME_STORY, create a new line.
func classifyAssignment(ctx context.Context, input AssignmentInput, candidates []StoryCandidate, classifier StoryClassifier) AssignmentResult {
	if len(candidates) == 0 {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginRule, assignReasonNoCandidates)
	}
	if classifier == nil {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMError)
	}
	id, decision, confidence, _, err := classifier(ctx, input, candidates)
	if err != nil {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMError)
	}
	// Without trusted fingerprints, LLM stage merges are unconstrained: measured
	// false merges reached 44%. Stage merging belongs to trusted-fingerprint
	// rules only; never reinterpret SAME_STORY as SAME_OCCURRENCE.
	if decision == StoryDecisionSameStory {
		logging.Warnf("story classify: blocked LLM SAME_STORY for thread %d candidate %d (confidence %.2f)", input.ThreadID, id, confidence)
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMSameStoryBlocked)
	}
	if confidence < StoryClassifierMinConfidence {
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMLowConfidence)
	}
	switch decision {
	case StoryDecisionSameOccurrence:
		if !storyCandidateIDsContain(candidates, id) {
			return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMInvalidID)
		}
		// Models may recognize a roundup, never decide occurrence membership.
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMSameOccurrenceBlocked)
	case StoryDecisionRoundup:
		// Preserve the label so 2c can mark scope; empty StoryIDs = no attach.
		return newStoryResult(StoryDecisionRoundup, repository.StoryOriginModel, assignReasonLLMRoundup)
	case StoryDecisionUnrelated:
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMUnrelated)
	default:
		return newStoryResult(StoryDecisionUnrelated, repository.StoryOriginModel, assignReasonLLMError)
	}
}

// storyCandidateIDsContain reports whether id belongs to the candidate set
// (guards hallucinated LLM candidate ids).
func storyCandidateIDsContain(candidates []StoryCandidate, id uint) bool {
	for _, c := range candidates {
		if c.ID == id {
			return true
		}
	}
	return false
}

// newStoryResult builds the "create a new story line" outcome.
func newStoryResult(decision StoryDecision, origin, reason string) AssignmentResult {
	return AssignmentResult{Decision: decision, Origin: origin, Reason: reason}
}

// assignmentFingerprintComplete mirrors the four required fingerprint fields
// on the assignment input (occurred_at optional).
func assignmentFingerprintComplete(in AssignmentInput) bool {
	return strings.TrimSpace(in.Subject) != "" &&
		strings.TrimSpace(in.SubjectKey) != "" &&
		strings.TrimSpace(in.Action) != "" &&
		strings.TrimSpace(in.Object) != ""
}

// matchEntityText reports bidirectional containment after entity
// normalization: 「A 产品」 matches 「A产品」, 「股价」 matches 「股价暴涨」.
// An empty normalized side never matches (missing ≠ equal).
func matchEntityText(a, b string) bool {
	na, nb := textutil.NormalizeEntityText(a), textutil.NormalizeEntityText(b)
	if na == "" || nb == "" {
		return false
	}
	return strings.Contains(na, nb) || strings.Contains(nb, na)
}

// reverseActionPairs are the semantically opposite action pairs (design
// §5.2 exception): a stage rule must not fuse a launch with its recall.
var reverseActionPairs = [][2]string{
	{"发布", "召回"},
	{"发布", "下架"},
	{"发布", "否认"},
	{"宣布", "否认"},
	{"上架", "下架"},
	{"推出", "召回"},
	{"推出", "下架"}, // 对称补齐：发布↔下架 / 推出↔召回 已在，推出↔下架 是同一 launch↔withdraw 语义对的漏配
}

var reverseActionTable = func() map[string]map[string]bool {
	table := map[string]map[string]bool{}
	for _, pair := range reverseActionPairs {
		a, b := normalizeActionText(pair[0]), normalizeActionText(pair[1])
		if table[a] == nil {
			table[a] = map[string]bool{}
		}
		if table[b] == nil {
			table[b] = map[string]bool{}
		}
		table[a][b] = true
		table[b][a] = true
	}
	return table
}()

// reversePairsNormalized is reverseActionPairs through normalizeActionText,
// built once so the containment fallback does not renormalize per call.
var reversePairsNormalized = func() [][2]string {
	pairs := make([][2]string, 0, len(reverseActionPairs))
	for _, pair := range reverseActionPairs {
		pairs = append(pairs, [2]string{normalizeActionText(pair[0]), normalizeActionText(pair[1])})
	}
	return pairs
}()

// IsReverseAction reports whether two actions are a known opposite pair,
// after case/full-width/whitespace normalization. Exact table lookup first;
// when that misses because one side is a free-form compound verb phrase
// (提取侧 action 如「宣布召回」「正式下架」), a containment fallback fires:
// one side containing a table word while the other contains its paired
// opposite. Containment deliberately has NO length-gap cap — a false hit
// only detours the caller to the non-merging classifier (undecidable),
// whereas capping would mask long genuine reverses like「因质量问题全球召回」
// vs「发布」and let the stage rule fuse a launch with its recall — the exact
// spec layer-3 exception this guard exists to enforce.
func IsReverseAction(a, b string) bool {
	na, nb := normalizeActionText(a), normalizeActionText(b)
	if na == "" || nb == "" {
		return false
	}
	if reverseActionTable[na][nb] {
		return true
	}
	if na == nb {
		// Identical actions are stage-equal, never reverse — a phrase that
		// happens to contain both directions (「发布召回公告」 vs itself) must
		// not trip the containment fallback.
		return false
	}
	for _, pair := range reversePairsNormalized {
		if (strings.Contains(na, pair[0]) && strings.Contains(nb, pair[1])) ||
			(strings.Contains(na, pair[1]) && strings.Contains(nb, pair[0])) {
			return true
		}
	}
	return false
}

// normalizeActionText folds case and full-width forms and drops all
// whitespace so “Ｌａｕｎｃｈ ” and "launch" compare equal.
func normalizeActionText(s string) string {
	cleaned := norm.NFKC.String(strings.ToLower(strings.TrimSpace(s)))
	var sb strings.Builder
	for _, r := range cleaned {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// sameObjectStrict is required merge evidence; containment alone cannot avoid an object veto.
func sameObjectStrict(a, b string) bool {
	a = textutil.NormalizeEntityText(a)
	b = textutil.NormalizeEntityText(b)
	return a != "" && b != "" && a == b
}

// stageDatesWithinOneDay compares calendar dates, not timestamps. An invalid
// nonempty occurred_at is not replaced by a report date: it is missing evidence.
func stageDatesWithinOneDay(input AssignmentInput, c StoryCandidate) bool {
	day := func(occurredAt string, periodDate time.Time) (time.Time, bool) {
		date := periodDate
		if strings.TrimSpace(occurredAt) != "" {
			var err error
			date, err = parseStoryOccurredAt(occurredAt)
			if err != nil {
				return time.Time{}, false
			}
		}
		if date.IsZero() {
			return time.Time{}, false
		}
		y, m, d := date.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), true
	}
	a, aOK := day(input.OccurredAt, input.PeriodDate)
	b, bOK := day(c.OccurredAt, c.PeriodDate)
	if !aOK || !bOK {
		return false
	}
	delta := a.Sub(b)
	return delta >= -24*time.Hour && delta <= 24*time.Hour
}
