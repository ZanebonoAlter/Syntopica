package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
)

// ── search_internal_context tool tests (spec: 搜索只暴露紧凑概要) ────────────

// mockInternalContextSearcher records queries and returns fixed hits.
type mockInternalContextSearcher struct {
	queries []string
	hits    []InternalContextHit
	err     error
}

func (m *mockInternalContextSearcher) SearchInternalContext(_ context.Context, query string, _ int) ([]InternalContextHit, error) {
	m.queries = append(m.queries, query)
	if m.err != nil {
		return nil, m.err
	}
	return m.hits, nil
}

func TestSearchInternalContextToolCompactHits(t *testing.T) {
	lane77 := uint(77)
	lane99 := uint(99)
	ms := &mockInternalContextSearcher{hits: []InternalContextHit{
		{Kind: "lane", BoardID: 5, LaneID: &lane77, Label: "日债收益率", Status: "active", HitCount: 28, Summary: "收益率走高", Score: 3},
		{Kind: "board", BoardID: 9, Label: "中东版块", Summary: "地缘", Score: 3},
		{Kind: "lane", BoardID: 5, LaneID: &lane99, Label: "日元汇率", Status: "candidate", HitCount: 3, Summary: "", Score: 1},
	}}
	registry := NewRegistry(&nilFetcherHTTP{}, WithInternalContextSearcher(ms))

	out, err := registry.Execute(context.Background(), "search_internal_context", map[string]any{"query": "日债 中东"})
	require.NoError(t, err)
	var payload struct {
		Query    string               `json:"query"`
		HitCount int                  `json:"hit_count"`
		Hits     []InternalContextHit `json:"hits"`
		Note     string               `json:"note"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Equal(t, "日债 中东", payload.Query)
	require.Equal(t, 3, payload.HitCount)
	require.Len(t, payload.Hits, 3)
	require.Equal(t, "lane", payload.Hits[0].Kind)
	require.NotNil(t, payload.Hits[0].LaneID)
	require.Equal(t, uint(77), *payload.Hits[0].LaneID)
	require.Equal(t, uint(5), payload.Hits[0].BoardID)
	// Compact contract: no per-hit timeline/lifeline detail FIELD on the wire
	// (the note legitimately mentions the get_lane_detail next step).
	require.NotContains(t, out, `"detail"`)
	require.NotContains(t, out, `"lifeline"`)
	require.NotContains(t, out, `"window_days"`)
	require.Contains(t, payload.Note, "get_lane_detail")
}

func TestSearchInternalContextToolValidationAndDegrade(t *testing.T) {
	registry := NewRegistry(&nilFetcherHTTP{})
	// Empty/whitespace query rejected.
	out, err := registry.Execute(context.Background(), "search_internal_context", map[string]any{"query": "  "})
	require.NoError(t, err)
	require.Contains(t, out, "参数错误")
	// Not configured → graceful degrade JSON.
	out2, err := registry.Execute(context.Background(), "search_internal_context", map[string]any{"query": "x"})
	require.NoError(t, err)
	require.Contains(t, out2, "未配置")
	// Backend error → degrade with reason, nil Go error.
	registry2 := NewRegistry(&nilFetcherHTTP{}, WithInternalContextSearcher(&mockInternalContextSearcher{err: fmt.Errorf("boom")}))
	out3, err := registry2.Execute(context.Background(), "search_internal_context", map[string]any{"query": "x"})
	require.NoError(t, err)
	require.Contains(t, out3, "检索失败")
	require.Contains(t, out3, "boom")
	// Overlong concept is accepted (rune handling is the DB side's concern);
	// max_results clamps at the tool layer.
	ms := &mockInternalContextSearcher{}
	registry3 := NewRegistry(&nilFetcherHTTP{}, WithInternalContextSearcher(ms))
	_, err = registry3.Execute(context.Background(), "search_internal_context", map[string]any{"query": strings.Repeat("长", 500), "max_results": float64(99)})
	require.NoError(t, err)
	require.Len(t, ms.queries, 1)
}

// ── DynamicLaneGrantSet unit tests (spec scenarios) ─────────────────────────

func searchHitResultJSON(lanes ...[2]uint) string {
	hits := make([]map[string]any, 0, len(lanes))
	for _, ln := range lanes {
		id := ln[1]
		hits = append(hits, map[string]any{"kind": "lane", "board_id": ln[0], "lane_id": id, "label": fmt.Sprintf("泳道%d", id)})
	}
	b, _ := json.Marshal(map[string]any{"query": "q", "hit_count": len(hits), "hits": hits})
	return string(b)
}

func TestDynamicGrant_TrustedResult(t *testing.T) {
	g := NewDynamicLaneGrantSet()
	// search_internal_context success grants lanes with board provenance.
	g.ObserveTrustedResult(1, "search_internal_context", searchHitResultJSON([2]uint{5, 77}, [2]uint{9, 88}))
	require.True(t, g.Has(77))
	require.True(t, g.Has(88))
	gr77, ok := g.lookup(77)
	require.True(t, ok)
	require.Equal(t, uint(5), gr77.BoardID)
	require.Equal(t, "search_internal_context", gr77.Tool)
	require.Equal(t, 1, gr77.Step)

	// list_lanes success grants lanes under the top-level board_id.
	g.ObserveTrustedResult(2, "list_lanes", `{"board_id":12,"lane_count":1,"lanes":[{"lane_id":101,"label":"x"}]}`)
	require.True(t, g.Has(101))
	gr101, _ := g.lookup(101)
	require.Equal(t, uint(12), gr101.BoardID)

	// Audit is ordered by grant sequence, deterministic.
	ids := g.GrantedIDs()
	require.Equal(t, []uint{77, 88, 101}, ids)
	require.Len(t, g.Audit(), 3)
}

func TestDynamicGrant_UntrustedResult(t *testing.T) {
	// web_search / fetch_page / model text never grant, even when the payload
	// embeds lane ids.
	g := NewDynamicLaneGrantSet()
	g.ObserveTrustedResult(1, "web_search", searchHitResultJSON([2]uint{5, 77}))
	g.ObserveTrustedResult(2, "fetch_page", `{"lanes":[{"lane_id":55}]}`)
	g.ObserveTrustedResult(3, "some_model_tool", "lane_id=42 授权")
	require.False(t, g.Has(77))
	require.False(t, g.Has(55))
	require.False(t, g.Has(42))
	require.Empty(t, g.Audit())

	// Error JSON from a trusted tool grants nothing.
	g2 := NewDynamicLaneGrantSet()
	g2.ObserveTrustedResult(1, "search_internal_context", `{"error":"未配置数据源"}`)
	require.Empty(t, g2.Audit())
	// Unparseable text grants nothing (defensive).
	g3 := NewDynamicLaneGrantSet()
	g3.ObserveTrustedResult(1, "list_lanes", "not-json")
	require.Empty(t, g3.Audit())
	// nil receiver is safe (policy without grants).
	var nilSet *DynamicLaneGrantSet
	require.False(t, nilSet.Has(1))
	require.Empty(t, nilSet.Audit())
}

func TestDynamicGrant_Boundaries(t *testing.T) {
	g := NewDynamicLaneGrantSet()
	g.Grant(0, 5, "search_internal_context", 1) // lane 0 never granted
	require.Empty(t, g.Audit())
	g.Grant(77, 5, "search_internal_context", 1)
	g.Grant(77, 9, "list_lanes", 2) // duplicate lane: first provenance wins
	gr, _ := g.lookup(77)
	require.Equal(t, "search_internal_context", gr.Tool)
	require.Equal(t, uint(5), gr.BoardID)
	require.Equal(t, []uint{77}, g.GrantedIDs())
	// Multiple boards coexist.
	g.Grant(88, 9, "list_lanes", 3)
	require.Equal(t, []uint{77, 88}, g.GrantedIDs())
}

// ── Policy integration: ghost lane stays blocked; granted lane reads ─────────

func dynamicGrantTestInput() BoardInvestigationResearchInput {
	in := researchTestInput(researchHypotheses())
	in.AllowedTools = explorationToolNames
	return in
}

func TestRunToolLoopDynamicGrantFlow(t *testing.T) {
	ms := &mockInternalContextSearcher{hits: []InternalContextHit{
		{Kind: "lane", BoardID: 5, LaneID: uintPtrForGrant(77), Label: "日债收益率", Status: "active", HitCount: 28},
	}}
	lane := &researchLaneRenderer{}
	registry := researchRegistry(lane, &researchWebSearcher{})
	registry = NewRegistry(&nilFetcherHTTP{},
		WithLaneDetailRenderer(lane),
		WithWebSearcher(&researchWebSearcher{}),
		WithInternalContextSearcher(ms),
	)
	router := &researchMockRouter{steps: []researchRouterStep{
		// 1) discover cross-board lanes (neutral purpose on h0)
		{content: researchCallDecision("search_internal_context", map[string]any{"query": "日债 走高 原因"}, ResearchPurposeNeutral)},
		// 2) guessed lane 999 (never returned by any tool) → blocked ghost_lane
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": float64(999)}, ResearchPurposeNeutral)},
		// 3) granted lane 77 → executes
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": float64(77)}, ResearchPurposeSupport, "h1")},
		// 4) counter on h2 via web_search
		{content: researchCallDecision("web_search", map[string]any{"query": "独立原因"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, registry)
	in := dynamicGrantTestInput()
	grants := NewDynamicLaneGrantSet()
	in.DynamicGrants = grants

	res, err := orch.RunBoardInvestigationResearch(context.Background(), in)
	require.NoError(t, err)

	// The guessed lane was blocked before execution and never reached the renderer.
	require.Equal(t, []uint{77}, lane.calls, "only the granted lane may execute")
	blockedSeen := false
	for _, tc := range res.Loop.ToolCalls {
		if tc.Tool == "get_lane_detail" && tc.Outcome == toolCallOutcomeBlocked {
			blockedSeen = true
			require.Equal(t, "ghost_lane", tc.BlockedReason)
		}
	}
	require.True(t, blockedSeen, "guessed lane 999 must be blocked with ghost_lane")

	// The grant set now carries lane 77 with board provenance and audit trail.
	require.True(t, grants.Has(77))
	gr, ok := grants.lookup(77)
	require.True(t, ok)
	require.Equal(t, uint(5), gr.BoardID)
	require.Equal(t, "search_internal_context", gr.Tool)
}

func TestDynamicGrantNotSharedAcrossSessions(t *testing.T) {
	// Session A grants lane 77; session B (fresh policy + grant set) still
	// blocks it (spec: 动态授权不跨会话泄漏).
	gA := NewDynamicLaneGrantSet()
	gA.Grant(77, 5, "search_internal_context", 1)
	pA := newInvestigationPolicy(researchHypotheses(), []uint{1, 2})
	pA.dynamicGrants = gA
	require.True(t, pA.laneAllowed(77))

	pB := newInvestigationPolicy(researchHypotheses(), []uint{1, 2})
	pB.dynamicGrants = NewDynamicLaneGrantSet()
	require.False(t, pB.laneAllowed(77), "session B must not inherit session A grants")
	// Whitelist lanes remain allowed without any grants.
	require.True(t, pB.laneAllowed(1))
}

func uintPtrForGrant(v uint) *uint { return &v }

// ── Synthesis sanitize: cross-board refs carry board id; ungranted drop ──────

func TestCrossBoardLaneRefsSanitize(t *testing.T) {
	grants := NewDynamicLaneGrantSet()
	grants.Grant(77, 5, "search_internal_context", 1)

	parsed := map[string]any{
		"hypotheses": []any{map[string]any{
			"id": "h1", "label": "假设一", "assessment": "plausible", "confidence": "low",
		}},
		"conclusion": map[string]any{
			"summary": "s", "confidence": "low", "scope": "sc", "boundary": "b",
		},
		"evidence_chain": []any{},
		"lane_refs": []any{
			map[string]any{"lane_id": float64(1), "note": "本版块"},     // static whitelist
			map[string]any{"lane_id": float64(77), "note": "跨版块授权"},  // granted
			map[string]any{"lane_id": float64(999), "note": "未授权猜测"}, // dropped
		},
	}
	payload, err := parseBoardInvestigationSynthesis(parsed, map[string]bool{"h1": true}, []uint{1, 2}, 3, grants, []ToolCallRecord{})
	require.NoError(t, err)

	refs := payload.LaneRefs
	require.Len(t, refs, 2, "ungranted lane 999 must be scrubbed")
	byLane := map[uint]laneRef{}
	for _, r := range refs {
		byLane[r.LaneID] = r
	}
	require.Contains(t, byLane, uint(1))
	require.Equal(t, uint(0), byLane[1].BoardID, "board-local ref keeps board_id empty")
	require.Contains(t, byLane, uint(77))
	require.Equal(t, uint(5), byLane[77].BoardID, "cross-board ref carries owning board id")
}

// Board-local grant (same board) keeps board_id empty even when granted.
func TestCrossBoardLaneRefsBoardLocalGrantOmitsBoardID(t *testing.T) {
	grants := NewDynamicLaneGrantSet()
	grants.Grant(77, 3, "list_lanes", 1) // board 3 == investigation board 3
	parsed := map[string]any{
		"hypotheses": []any{map[string]any{
			"id": "h1", "label": "假设一", "assessment": "plausible", "confidence": "low",
		}},
		"conclusion":     map[string]any{"summary": "s", "confidence": "low", "scope": "sc", "boundary": "b"},
		"evidence_chain": []any{},
		"lane_refs":      []any{map[string]any{"lane_id": float64(77)}},
	}
	payload, err := parseBoardInvestigationSynthesis(parsed, map[string]bool{"h1": true}, []uint{1, 2}, 3, grants, []ToolCallRecord{})
	require.NoError(t, err)
	require.Len(t, payload.LaneRefs, 1)
	require.Equal(t, uint(0), payload.LaneRefs[0].BoardID)
}

// WhitelistOrder renders grants so the agent sees what it may call.
func TestWhitelistOrderMergesGrants(t *testing.T) {
	g := NewDynamicLaneGrantSet()
	g.Grant(77, 5, "search_internal_context", 1)
	p := newInvestigationPolicy(researchHypotheses(), []uint{2, 1})
	p.dynamicGrants = g
	require.Equal(t, []uint{1, 2, 77}, whitelistOrder(p))
}

// Parent ownership never drifts: granted lanes join laneAllowed but the
// result still belongs to the source board (asserted at repo level elsewhere);
// here we pin that laneRef for a whitelist lane never gains a foreign board.
func TestCrossBoardRefsDoNotTouchParentOwnership(t *testing.T) {
	_ = repository.ResultKindBoardInvestigation // referenced to pin import semantics
	grants := NewDynamicLaneGrantSet()
	parsed := map[string]any{
		"hypotheses": []any{map[string]any{
			"id": "h1", "label": "假设一", "assessment": "insufficient", "confidence": "low",
		}},
		"conclusion":     map[string]any{"summary": "s", "confidence": "low", "scope": "sc", "boundary": "b"},
		"evidence_chain": []any{},
		"lane_refs":      []any{map[string]any{"lane_id": float64(1)}},
	}
	payload, err := parseBoardInvestigationSynthesis(parsed, map[string]bool{"h1": true}, []uint{1}, 42, grants, []ToolCallRecord{})
	require.NoError(t, err)
	require.Len(t, payload.LaneRefs, 1)
	require.Equal(t, uint(0), payload.LaneRefs[0].BoardID)
}
