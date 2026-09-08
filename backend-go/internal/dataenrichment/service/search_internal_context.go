package service

import (
	"context"
	"encoding/json"
	"strings"
)

// ── search_internal_context: compact internal knowledge search ──────────────
//
// add-evidence-backed-cross-board-relations (design D3). A discovery-first
// entry point for investigations: it returns ONLY compact board/lane
// overviews (id, owner board, label, status, hit count, short summary) so the
// agent can pick a target before any detail is granted. Full timelines /
// lifeline archives stay behind get_lane_detail and its dynamic grant gate —
// the spec forbids the search from leaking detail before authorization.

// InternalContextHit is one compact search hit. Kind is "board" or "lane";
// LaneID is nil for board hits.
type InternalContextHit struct {
	Kind     string  `json:"kind"`
	BoardID  uint    `json:"board_id"`
	LaneID   *uint   `json:"lane_id,omitempty"`
	Label    string  `json:"label"`
	Status   string  `json:"status,omitempty"`
	HitCount int     `json:"hit_count"`
	Summary  string  `json:"summary"`
	Score    float64 `json:"score"`
}

// InternalContextSearcher backs the search_internal_context tool. Production
// implementation lives in the dataenrichment package (lexical + vector
// scoring over semantic_labels / board_persistent_topics).
type InternalContextSearcher interface {
	SearchInternalContext(ctx context.Context, query string, maxResults int) ([]InternalContextHit, error)
}

func (r *Registry) executeSearchInternalContext(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return jsonError("参数错误: query 必须为非空字符串"), nil
	}
	if r.internalContextSearcher == nil {
		return jsonError("search_internal_context 未配置数据源"), nil
	}
	maxResults := 8
	if n, ok := toUint(args["max_results"]); ok && n > 0 && n <= 20 {
		maxResults = int(n)
	}
	hits, err := r.internalContextSearcher.SearchInternalContext(ctx, query, maxResults)
	if err != nil {
		return jsonError("内部上下文检索失败: " + err.Error()), nil
	}
	if hits == nil {
		hits = []InternalContextHit{}
	}
	b, _ := jsonMarshal(map[string]any{
		"query":     query,
		"hit_count": len(hits),
		"hits":      hits,
		"note":      "紧凑概要（无时间线）。命中的 lane_id 本次调查已获临时授权，可用 get_lane_detail(lane_id) 下钻。",
	})
	return string(b), nil
}

// ── Dynamic lane grant set ──────────────────────────────────────────────────
//
// Session-scoped capability grants for get_lane_detail (design D3). The set
// starts empty (the static whitelist from the parent brief is held by the
// investigation policy); ONLY structured results of trusted tools
// (search_internal_context / list_lanes) actually returned by the server may
// add lanes. Model text, web content and other sessions can never grant.

// LaneGrant records one runtime grant with its provenance.
type LaneGrant struct {
	LaneID  uint   `json:"lane_id"`
	BoardID uint   `json:"board_id,omitempty"`
	Tool    string `json:"tool"` // granting tool name
	Step    int    `json:"step"` // agent loop step of the granting call
}

// DynamicLaneGrantSet is a mutex-guarded, session-scoped grant store.
type DynamicLaneGrantSet struct {
	grants map[uint]LaneGrant
	order  []uint // grant order for deterministic audit output
}

// NewDynamicLaneGrantSet creates an empty grant set.
func NewDynamicLaneGrantSet() *DynamicLaneGrantSet {
	return &DynamicLaneGrantSet{grants: make(map[uint]LaneGrant)}
}

// Has reports whether the lane was granted this session.
func (g *DynamicLaneGrantSet) Has(laneID uint) bool {
	if g == nil {
		return false
	}
	_, ok := g.grants[laneID]
	return ok
}

// Grant records one lane grant (idempotent per lane; first provenance wins).
func (g *DynamicLaneGrantSet) Grant(laneID, boardID uint, tool string, step int) {
	if laneID == 0 || g == nil {
		return
	}
	if _, exists := g.grants[laneID]; exists {
		return
	}
	g.grants[laneID] = LaneGrant{LaneID: laneID, BoardID: boardID, Tool: tool, Step: step}
	g.order = append(g.order, laneID)
}

// lookup returns the grant record for a lane (ok=false when not granted).
func (g *DynamicLaneGrantSet) lookup(laneID uint) (LaneGrant, bool) {
	if g == nil {
		return LaneGrant{}, false
	}
	gr, ok := g.grants[laneID]
	return gr, ok
}

// GrantedIDs returns granted lane ids in grant order (deterministic).
func (g *DynamicLaneGrantSet) GrantedIDs() []uint {
	if g == nil {
		return nil
	}
	out := make([]uint, 0, len(g.order))
	out = append(out, g.order...)
	return out
}

// Audit returns the grants in grant order for snapshot freezing.
func (g *DynamicLaneGrantSet) Audit() []LaneGrant {
	if g == nil {
		return []LaneGrant{}
	}
	out := make([]LaneGrant, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.grants[id])
	}
	return out
}

// trustedGrantTools are the only tools whose structured results may extend the
// grant set. web_search / fetch_page / model output are deliberately absent.
var trustedGrantTools = map[string]bool{
	"search_internal_context": true,
	"list_lanes":              true,
}

// ObserveTrustedResult inspects one executed tool result and, when the tool is
// trusted and the result is a successful JSON payload, grants every lane the
// SERVER actually returned. Error payloads grant nothing. Unparseable text
// grants nothing (defensive: trusted tools always emit JSON).
func (g *DynamicLaneGrantSet) ObserveTrustedResult(step int, toolName, resultFull string) {
	if g == nil || !trustedGrantTools[toolName] {
		return
	}
	var payload struct {
		Error string `json:"error"`
		Hits  []struct {
			Kind    string `json:"kind"`
			BoardID uint   `json:"board_id"`
			LaneID  *uint  `json:"lane_id"`
		} `json:"hits"`
		BoardID uint `json:"board_id"`
		Lanes   []struct {
			LaneID uint `json:"lane_id"`
		} `json:"lanes"`
	}
	if err := json.Unmarshal([]byte(resultFull), &payload); err != nil {
		return
	}
	if payload.Error != "" {
		return
	}
	for _, hit := range payload.Hits {
		if hit.Kind == "lane" && hit.LaneID != nil {
			g.Grant(*hit.LaneID, hit.BoardID, toolName, step)
		}
	}
	for _, lane := range payload.Lanes {
		g.Grant(lane.LaneID, payload.BoardID, toolName, step)
	}
}
