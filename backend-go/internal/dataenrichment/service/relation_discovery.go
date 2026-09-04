package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/logging"
)

// ── Relation discovery orchestration (design D1/D4/D6) ──────────────────────
//
// Source snapshot → Scout (external evidence) → Resolve (conservative internal
// binding) → Verify (blind competing explanations) → Persist (unresolved /
// proposed only; never confirmed automatically).

// RelationSourceRef identifies the immutable source inside a parent brief.
type RelationSourceRef struct {
	ParentResultID uint
	SourceKind     string // observation | question
	SourceKey      string // stable id inside the brief payload
}

// RelationDiscoveryInput is the manual/auto trigger payload. The server
// re-fetches and validates everything from the parent brief — client-supplied
// source text is never trusted.
type RelationDiscoveryInput struct {
	BoardID     uint
	Source      RelationSourceRef
	TriggerKind string // manual | auto
}

// runToolCall is the per-run external call audit trail persisted onto the run
// row (web_search calls from scout + verifier counter-searches).
type runToolCall struct {
	Ref     string `json:"ref"`
	Tool    string `json:"tool"`
	Query   string `json:"query"`
	Results int    `json:"result_count"`
}

// RelationDiscoveryOutput summarizes one run.
type RelationDiscoveryOutput struct {
	RunID            uint                             `json:"run_id"`
	Status           string                           `json:"status"`
	RelationsCreated int                              `json:"relations_created"`
	RelationsSkipped int                              `json:"relations_skipped"` // cooldown / idempotent no-ops
	Gaps             []relationEvidenceGap            `json:"gaps"`
	Relations        []*repository.CrossBoardRelation `json:"relations"`

	// toolCalls is unexported on purpose: it feeds the run-row audit snapshot
	// (cross_board_relation_runs.tool_calls), not the API payload.
	toolCalls []runToolCall
}

// relationRunBudgetSnapshot is frozen into the run row for audit/replay.
type relationRunBudgetSnapshot struct {
	MaxSearchesPerRun int     `json:"max_searches_per_run"`
	MaxFetchesPerRun  int     `json:"max_fetches_per_run"`
	MaxLoopsPerRun    int     `json:"max_loops_per_run"`
	TimeoutSeconds    int     `json:"timeout_seconds"`
	ResolveThreshold  float64 `json:"resolve_threshold"`
	ResolveMargin     float64 `json:"resolve_margin"`
	AutoMaxSources    int     `json:"auto_max_sources,omitempty"`
}

// fetchRelationSource re-reads and validates the source snapshot from the
// parent brief (spec: 从观察手动发现 / 客户端伪造 source 防线).
func fetchRelationSource(ctx context.Context, repo *repository.Repository, boardID uint, ref RelationSourceRef) (string, error) {
	parent, err := repo.GetTopicEnrichmentResultByID(ctx, ref.ParentResultID)
	if err != nil {
		return "", fmt.Errorf("relation source: load parent %d: %w", ref.ParentResultID, err)
	}
	if !repository.BoardIDMatches(parent.SemanticBoardID, boardID) {
		return "", fmt.Errorf("relation source: parent %d belongs to another board", ref.ParentResultID)
	}
	if repository.EffectiveResultKind(parent) != repository.ResultKindBoardBrief {
		return "", fmt.Errorf("relation source: parent %d is not a board_brief", ref.ParentResultID)
	}
	brief := &BoardBriefPayload{}
	if err := json.Unmarshal(parent.Sectors, brief); err != nil {
		return "", fmt.Errorf("relation source: parent sectors unreadable: %w", err)
	}
	switch ref.SourceKind {
	case repository.RelationSourceObservation:
		for _, obs := range brief.Observations {
			if obs.ID == ref.SourceKey {
				return obs.Statement, nil
			}
		}
		return "", fmt.Errorf("relation source: observation %q not in parent brief %d", ref.SourceKey, ref.ParentResultID)
	case repository.RelationSourceQuestion:
		for _, q := range brief.ResearchQuestions {
			if q.ID == ref.SourceKey {
				return q.Question, nil
			}
		}
		return "", fmt.Errorf("relation source: question %q not in parent brief %d", ref.SourceKey, ref.ParentResultID)
	default:
		return "", fmt.Errorf("relation source: illegal source kind %q", ref.SourceKind)
	}
}

// RunRelationDiscovery executes one full pipeline run for a single source.
// LLM/search failures degrade to gaps (partial); only setup failures error.
func (o *OrchestratorService) RunRelationDiscovery(ctx context.Context, in RelationDiscoveryInput) (*RelationDiscoveryOutput, error) {
	if o.repo == nil {
		return nil, fmt.Errorf("relation discovery: repository not wired")
	}
	cfg := config.EffectiveCrossBoardRelationConfig()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RunTimeoutSeconds)*time.Second)
	defer cancel()

	sourceText, err := fetchRelationSource(ctx, o.repo, in.BoardID, in.Source)
	if err != nil {
		return nil, err
	}

	budgetJSON, _ := json.Marshal(relationRunBudgetSnapshot{
		MaxSearchesPerRun: cfg.MaxSearchesPerRun,
		MaxFetchesPerRun:  cfg.MaxFetchesPerRun,
		MaxLoopsPerRun:    cfg.MaxLoopsPerRun,
		TimeoutSeconds:    cfg.RunTimeoutSeconds,
		ResolveThreshold:  cfg.ResolveThreshold,
		ResolveMargin:     cfg.ResolveMargin,
	})
	run := &repository.CrossBoardRelationRun{
		SourceBoardID:  in.BoardID,
		ParentResultID: in.Source.ParentResultID,
		SourceKind:     in.Source.SourceKind,
		SourceKey:      in.Source.SourceKey,
		SourceText:     sourceText,
		TriggerKind:    in.TriggerKind,
		Status:         repository.RelationRunStatusRunning,
		BudgetSnapshot: budgetJSON,
	}
	if in.TriggerKind == "" {
		run.TriggerKind = repository.RelationTriggerManual
	}
	if err := o.repo.CreateRelationRun(ctx, run); err != nil {
		return nil, fmt.Errorf("relation discovery: create run: %w", err)
	}

	out := &RelationDiscoveryOutput{RunID: run.ID, Status: repository.RelationRunStatusRunning, Gaps: []relationEvidenceGap{}}
	finish := func(status string) *RelationDiscoveryOutput {
		gapsJSON, _ := json.Marshal(out.Gaps)
		toolCallsJSON, _ := json.Marshal(out.toolCalls)
		if err := o.repo.DB().Model(&repository.CrossBoardRelationRun{}).Where("id = ?", run.ID).
			Updates(map[string]any{
				"status": status, "gaps": string(gapsJSON), "tool_calls": string(toolCallsJSON), "updated_at": time.Now(),
			}).Error; err != nil {
			logging.Warnf("relation discovery run %d: persist gaps: %v", run.ID, err)
		}
		out.Status = status
		return out
	}

	// ── Scout ──
	sessionID := fmt.Sprintf("relation_scout_%d_%d", run.ID, time.Now().Unix())
	scout, err := o.runRelationScout(ctx, sessionID, sourceText, cfg.MaxSearchesPerRun)
	if err != nil {
		out.Gaps = append(out.Gaps, scout.Gaps...)
		return finish(repository.RelationRunStatusFailed), nil
	}
	out.Gaps = append(out.Gaps, scout.Gaps...)
	for _, s := range scout.Searches {
		out.toolCalls = append(out.toolCalls, runToolCall{Ref: s.Ref, Tool: "web_search", Query: s.Query, Results: len(s.Results)})
	}
	if len(scout.Candidates) == 0 {
		return finish(repository.RelationRunStatusSucceeded), nil
	}

	// ── Per candidate: Resolve → Verify → Persist ──
	for _, cand := range scout.Candidates {
		if ctx.Err() != nil {
			out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "run_timeout"})
			return finish(repository.RelationRunStatusPartial), nil
		}
		o.processRelationCandidate(ctx, processRelationCandidateInput{
			run: run, scout: scout, cand: cand, cfg: cfg, out: out,
		})
	}
	if out.RelationsCreated == 0 && len(out.Gaps) > 0 && len(scout.Candidates) > 0 {
		return finish(repository.RelationRunStatusPartial), nil
	}
	return finish(repository.RelationRunStatusSucceeded), nil
}

// processRelationCandidateInput bundles the per-candidate pipeline deps.
type processRelationCandidateInput struct {
	run   *repository.CrossBoardRelationRun
	scout *relationScoutOutcome
	cand  scoutCandidate
	cfg   config.CrossBoardRelationConfig
	out   *RelationDiscoveryOutput
}

// processRelationCandidate runs resolve→verify→persist for one scout
// candidate. Failures inside degrade to gaps; the loop continues.
func (o *OrchestratorService) processRelationCandidate(ctx context.Context, in processRelationCandidateInput) {
	run, cand, cfg, out := in.run, in.cand, in.cfg, in.out

	// 1) Conservative resolve (program-only binding).
	resolveRes, err := ResolveTarget(ctx, ResolveTargetInput{
		Concept:   cand.TargetConcept,
		Searcher:  o.internalContextSearcherForRelations(),
		TopK:      5,
		Threshold: cfg.ResolveThreshold,
		Margin:    cfg.ResolveMargin,
	})
	mappingJSON := []byte("{}")
	var targetBoardID, targetLaneID *uint
	if err != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "resolve_error", Detail: err.Error()})
		return // resolver failure: keep the candidate only in the run record
	}
	if b, mErr := json.Marshal(resolveRes); mErr == nil {
		mappingJSON = b
	}
	internalBrief := ""
	if resolveRes.Outcome == ResolveOutcomeResolved && resolveRes.Best != nil {
		bid := resolveRes.Best.BoardID
		targetBoardID = &bid
		if resolveRes.Best.LaneID != nil {
			lid := *resolveRes.Best.LaneID
			targetLaneID = &lid
		}
		internalBrief = fmt.Sprintf("内部对象：%s %q（board=%d）", resolveRes.Best.Kind, resolveRes.Best.Label, resolveRes.Best.BoardID)
	}

	// 2) Evidence scrub: verify quotes against raw tool results BEFORE any
	// verdict can lean on them (spec: 幽灵 quote 不计入支持证据).
	support, dropped := scrubRelationEvidence(stampEvidenceUse(cand.Evidence, "support"), in.scout.RawByRef)
	if dropped > 0 {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "evidence_dropped_no_provenance", Detail: fmt.Sprintf("%d 条引用无工具原文对应", dropped)})
	}
	counterFromScout, _ := scrubRelationEvidence(stampEvidenceUse(cand.CounterEvidence, "counter"), in.scout.RawByRef)

	// 3) Blind verify (own session; scout self-scores never leak).
	supportMD := renderEvidenceMarkdown(support)
	verifySession := fmt.Sprintf("relation_verify_%d_%s", run.ID, sanitizeSessionToken(cand.TargetConcept))
	verdict, counterSearches, vGaps, _ := o.runRelationVerifier(ctx, verifySession, relationVerifyInput{
		SourceText:    run.SourceText,
		Claim:         cand.Claim,
		ClaimedType:   cand.RelationType,
		ClaimedMech:   cand.Mechanism,
		SupportMD:     supportMD,
		InternalBrief: internalBrief,
	}, cfg.MaxSearchesPerRun)
	out.Gaps = append(out.Gaps, vGaps...)
	for _, cs := range counterSearches {
		out.toolCalls = append(out.toolCalls, runToolCall{Ref: cs.Ref, Tool: "web_search", Query: cs.Query, Results: len(cs.Results)})
	}
	if verdict == nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "verify_unavailable", Detail: cand.TargetConcept})
		return
	}
	counterSearched := len(counterSearches) > 0

	// 4) Persist decision (design D6):
	//    rejected → run record only; unresolved/insufficient-quality →
	//    unresolved row; resolved + supported → proposed row (never confirmed).
	relationType := verdict.RelationType
	verifiedSupport, _ := scrubRelationEvidence(support, in.scout.RawByRef)
	counterEvidence := append([]repository.RelationEvidence{}, counterFromScout...)
	for _, cs := range counterSearches {
		for _, r := range cs.Results {
			counterEvidence = append(counterEvidence, repository.RelationEvidence{
				Ref: cs.Ref, Tool: "web_search", URL: r.URL, Title: r.Title,
				Quote: r.Snippet, RetrievedAt: time.Now().Format(time.RFC3339), Use: "counter",
			})
			break // one representative hit per counter query
		}
	}
	grade := relationQualityGrade(verifiedSupport, counterSearched)

	if verdict.Verdict == repository.RelationVerdictRejected {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "candidate_rejected", Detail: cand.Claim})
		return // stays only in the run snapshot
	}

	status := repository.RelationStatusUnresolved
	if resolveRes.Outcome == ResolveOutcomeResolved && verdict.Verdict == repository.RelationVerdictSupported {
		status = repository.RelationStatusProposed
	}
	if verdict.Verdict == repository.RelationVerdictSupported && resolveRes.Outcome != ResolveOutcomeResolved {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "supported_but_unresolved_target", Detail: resolveRes.Outcome})
	}

	rel := &repository.CrossBoardRelation{
		RunID:               &run.ID,
		SourceBoardID:       run.SourceBoardID,
		TargetBoardID:       targetBoardID,
		TargetLaneID:        targetLaneID,
		TargetConcept:       truncateRunes(strings.TrimSpace(cand.TargetConcept), 200),
		MappingSnapshot:     mappingJSON,
		RelationType:        relationType,
		Claim:               truncateRunes(strings.TrimSpace(cand.Claim), 500),
		Mechanism:           truncateRunes(strings.TrimSpace(verdict.Mechanism), 500),
		VerificationVerdict: verdict.Verdict,
		QualityGrade:        grade,
		Evidence:            verifiedSupport,
		Counterevidence:     counterEvidence,
		Status:              status,
		EvidenceVersion:     relationEvidenceVersion(verifiedSupport),
	}
	rel.SuggestionHash = repository.ComputeRelationHash(
		run.SourceBoardID, run.SourceKind, run.SourceKey,
		targetBoardID, rel.TargetConcept, rel.RelationType, rel.Claim, rel.EvidenceVersion,
	)

	// Dismiss cooldown: a recently dismissed identical suggestion must not
	// re-appear (spec: 驳回建议进入冷却).
	if cooldown, cErr := o.repo.CountDismissedRelationsInCooldown(ctx, rel.SuggestionHash, cfg.DismissCooldownDays); cErr == nil && cooldown > 0 {
		out.RelationsSkipped++
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "dismiss_cooldown", Detail: rel.Claim})
		return
	}

	inserted, err := o.repo.InsertOpenRelation(ctx, rel)
	if err != nil {
		out.Gaps = append(out.Gaps, relationEvidenceGap{Reason: "persist_error", Detail: err.Error()})
		return
	}
	if !inserted {
		out.RelationsSkipped++ // idempotent no-op: same open hash exists
		return
	}
	out.RelationsCreated++
	out.Relations = append(out.Relations, rel)
}

// internalContextSearcherForRelations returns the registry-backed searcher
// when wired; nil-safe for tests via ResolveTarget's own guard.
func (o *OrchestratorService) internalContextSearcherForRelations() InternalContextSearcher {
	if o.toolRegistry == nil {
		return nil
	}
	return o.toolRegistry.internalContextSearcher
}

// stampEvidenceUse forces the use field on scout-extracted evidence.
func stampEvidenceUse(entries []repository.RelationEvidence, use string) []repository.RelationEvidence {
	out := make([]repository.RelationEvidence, 0, len(entries))
	for _, e := range entries {
		e.Use = use
		e.Tool = "web_search"
		if e.RetrievedAt == "" {
			e.RetrievedAt = time.Now().Format(time.RFC3339)
		}
		out = append(out, e)
	}
	return out
}

// renderEvidenceMarkdown renders evidence rows for the verifier prompt.
func renderEvidenceMarkdown(entries []repository.RelationEvidence) string {
	if len(entries) == 0 {
		return "（无）"
	}
	var sb strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&sb, "[%d] %s\n    URL: %s\n    标题: %s\n    引用: %s\n", i+1, e.Ref, e.URL, e.Title, e.Quote)
	}
	return sb.String()
}

func sanitizeSessionToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return "x"
	}
	return sb.String()
}

// RelationReResolveOutput reports one re-resolution attempt.
type RelationReResolveOutput struct {
	RelationID uint   `json:"relation_id"`
	NewStatus  string `json:"new_status"`
	Outcome    string `json:"outcome"` // resolver outcome
}

// ReResolveRelation re-runs the conservative resolver on an unresolved
// relation's stored concept (new internal objects may have appeared since).
// resolved + previously-supported verdict → proposed; anything else stays
// unresolved (spec: 外部概念尚无内部目标时允许后续重新解析).
func (o *OrchestratorService) ReResolveRelation(ctx context.Context, boardID, relationID uint) (*RelationReResolveOutput, error) {
	if o.repo == nil {
		return nil, fmt.Errorf("re-resolve relation: repository not wired")
	}
	rel, err := o.repo.GetCrossBoardRelationByID(ctx, relationID)
	if err != nil {
		return nil, err
	}
	if rel.SourceBoardID != boardID && (rel.TargetBoardID == nil || *rel.TargetBoardID != boardID) {
		return nil, fmt.Errorf("re-resolve relation %d: not related to board %d", relationID, boardID)
	}
	if rel.Status != repository.RelationStatusUnresolved {
		return nil, fmt.Errorf("re-resolve relation %d: status is %s, not unresolved", relationID, rel.Status)
	}
	cfg := config.EffectiveCrossBoardRelationConfig()
	res, err := ResolveTarget(ctx, ResolveTargetInput{
		Concept:   rel.TargetConcept,
		Searcher:  o.internalContextSearcherForRelations(),
		TopK:      5,
		Threshold: cfg.ResolveThreshold,
		Margin:    cfg.ResolveMargin,
	})
	if err != nil {
		return nil, err
	}
	newStatus := repository.RelationStatusUnresolved
	var tb, tl *uint
	if res.Outcome == ResolveOutcomeResolved && res.Best != nil {
		bid := res.Best.BoardID
		tb = &bid
		if res.Best.LaneID != nil {
			lid := *res.Best.LaneID
			tl = &lid
		}
		if rel.VerificationVerdict == repository.RelationVerdictSupported {
			newStatus = repository.RelationStatusProposed
		}
	}
	mappingJSON, _ := json.Marshal(res)
	if err := o.repo.ReResolveCrossBoardRelation(ctx, relationID, newStatus, tb, tl, mappingJSON); err != nil {
		return nil, err
	}
	return &RelationReResolveOutput{RelationID: relationID, NewStatus: newStatus, Outcome: res.Outcome}, nil
}

// ValidateRelationSourceKey checks that (kind, key) exists inside the parent
// brief sectors WITHOUT trusting any client text — the handler pre-flight
// (mirror of fetchRelationSource for synchronous 400s).
func ValidateRelationSourceKey(parentSectors json.RawMessage, sourceKind, sourceKey string) error {
	brief := &BoardBriefPayload{}
	if err := json.Unmarshal(parentSectors, brief); err != nil {
		return fmt.Errorf("briefing result sectors unreadable")
	}
	switch sourceKind {
	case repository.RelationSourceObservation:
		for _, obs := range brief.Observations {
			if obs.ID == sourceKey {
				return nil
			}
		}
		return fmt.Errorf("source_key %q not found in briefing observations", sourceKey)
	case repository.RelationSourceQuestion:
		for _, q := range brief.ResearchQuestions {
			if q.ID == sourceKey {
				return nil
			}
		}
		return fmt.Errorf("source_key %q not found in briefing research_questions", sourceKey)
	default:
		return fmt.Errorf("illegal source_kind %q", sourceKind)
	}
}

// ── 7.1/7.2: 自动发现入口（新简报落库后 non-fatal enqueue）───────────────────

// SetAutoDiscoveryExec swaps the auto-discovery batch executor (tests inject
// a synchronous recorder). nil restores the production default.
func (o *OrchestratorService) SetAutoDiscoveryExec(fn func(ctx context.Context, boardID, parentID uint, sources []RelationSourceRef)) {
	o.autoMu.Lock()
	defer o.autoMu.Unlock()
	o.autoDiscoveryExec = fn
}

// selectAutoRelationSources picks at most max observation sources in the
// brief's stable order (spec: 按稳定 observation 顺序处理预算内 source).
// max <= 0 → empty.
func SelectAutoRelationSources(observations []BoardBriefObservation, max int) []RelationSourceRef {
	if max <= 0 || len(observations) == 0 {
		return nil
	}
	n := len(observations)
	if n > max {
		n = max
	}
	sources := make([]RelationSourceRef, 0, n)
	for i := 0; i < n; i++ {
		sources = append(sources, RelationSourceRef{
			ParentResultID: 0, // filled by caller (brief ID known only after persist)
			SourceKind:     repository.RelationSourceObservation,
			SourceKey:      observations[i].ID,
		})
	}
	return sources
}

// maybeAutoDiscoverRelations enqueues the auto discovery batch after a brief
// persisted. Gates (D8): per-board switch, global budget, all-sparse brief.
// Never returns an error — the brief is already committed and MUST NOT roll
// back on enqueue problems (spec: 任何 enqueue 失败均不回滚简报).
func (o *OrchestratorService) maybeAutoDiscoverRelations(cfg *BoardEnrichmentConfig, boardID, parentID uint, brief *BoardBriefPayload) {
	if brief == nil {
		return
	}
	if !cfg.RelationAutoDiscoveryEnabled {
		logging.Infof("relation auto discovery board %d: disabled by board switch, skipped", boardID)
		return
	}
	rcfg := config.EffectiveCrossBoardRelationConfig()
	if rcfg.AutoMaxSourcesPerBrief <= 0 {
		logging.Infof("relation auto discovery board %d: global budget is zero, skipped", boardID)
		return
	}
	if brief.AllSparse || len(brief.Observations) == 0 {
		logging.Infof("relation auto discovery board %d: brief has no observation material, skipped", boardID)
		return
	}
	sources := SelectAutoRelationSources(brief.Observations, rcfg.AutoMaxSourcesPerBrief)
	if len(sources) == 0 {
		return
	}
	for i := range sources {
		sources[i].ParentResultID = parentID
	}
	exec := o.defaultAutoDiscoveryExecLocked()
	if exec == nil {
		return
	}
	// Brief is committed; the batch runs detached so client disconnects never
	// cancel it (约束21: MUST NOT pass request-context into analysis chains).
	// The per-board in-flight guard wraps the executor so injected (test)
	// executors are serialized too (D8: 已有冲突任务时跳过并留痕).
	go func() {
		if !o.acquireAutoBatch(boardID) {
			logging.Infof("relation auto discovery board %d: previous batch still running, skipped", boardID)
			return
		}
		defer o.releaseAutoBatch(boardID)
		exec(context.Background(), boardID, parentID, sources)
	}()
}

// acquireAutoBatch takes the per-board in-flight slot (false = conflict).
func (o *OrchestratorService) acquireAutoBatch(boardID uint) bool {
	o.autoMu.Lock()
	defer o.autoMu.Unlock()
	if o.autoInFlight == nil {
		o.autoInFlight = map[uint]bool{}
	}
	if o.autoInFlight[boardID] {
		return false
	}
	o.autoInFlight[boardID] = true
	return true
}

// releaseAutoBatch drops the per-board in-flight slot.
func (o *OrchestratorService) releaseAutoBatch(boardID uint) {
	o.autoMu.Lock()
	defer o.autoMu.Unlock()
	delete(o.autoInFlight, boardID)
}

// defaultAutoDiscoveryExecLocked snapshots the executor under lock.
func (o *OrchestratorService) defaultAutoDiscoveryExecLocked() func(context.Context, uint, uint, []RelationSourceRef) {
	o.autoMu.Lock()
	defer o.autoMu.Unlock()
	if o.autoDiscoveryExec != nil {
		return o.autoDiscoveryExec
	}
	return o.runAutoDiscoveryBatch
}

// runAutoDiscoveryBatch serially processes the budgeted sources with a
// per-board in-flight guard (D8: 已有冲突任务时跳过并留痕). Every error is
// logged and swallowed — the triggering brief stays committed.
func (o *OrchestratorService) runAutoDiscoveryBatch(ctx context.Context, boardID, parentID uint, sources []RelationSourceRef) {
	rcfg := config.EffectiveCrossBoardRelationConfig()
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(rcfg.RunTimeoutSeconds*len(sources))*time.Second)
	defer cancel()
	for _, src := range sources {
		out, err := o.RunRelationDiscovery(runCtx, RelationDiscoveryInput{
			BoardID: boardID, Source: src, TriggerKind: repository.RelationTriggerAuto,
		})
		if err != nil {
			logging.Warnf("relation auto discovery board %d source %s/%s: %v (non-fatal)", boardID, src.SourceKind, src.SourceKey, err)
			continue
		}
		logging.Infof("relation auto discovery board %d source %s/%s: %s (%d relations)", boardID, src.SourceKind, src.SourceKey, out.Status, out.RelationsCreated)
	}
}

// RunAutoDiscoveryBatchForTest exposes the per-board guarded batch for tests.
func (o *OrchestratorService) RunAutoDiscoveryBatchForTest(ctx context.Context, boardID, parentID uint, sources []RelationSourceRef) {
	o.runAutoDiscoveryBatch(ctx, boardID, parentID, sources)
}

// MaybeAutoDiscoverRelationsForTest exposes the auto dispatch gate for tests.
func (o *OrchestratorService) MaybeAutoDiscoverRelationsForTest(cfg *BoardEnrichmentConfig, boardID, parentID uint, brief *BoardBriefPayload) {
	o.maybeAutoDiscoverRelations(cfg, boardID, parentID, brief)
}
