package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── 跨版块调查链（add-evidence-backed-cross-board-relations test-cases S8/S9）──
//
// testcontainer PG 全链路（mock LLM + stub 工具 + mock 内部检索）：
//   - 调查经 search_internal_context 发现并授权跨版块泳道 → get_lane_detail
//     可读取 → 综合引用携带 board_id → 落库快照含 dynamic_grants 审计
//   - 父简报 sectors 字节不变（父简报归属保持不变）
//   - 未授权跨版块引用被综合剔除（parse 级）
//   - 授权后泳道归属漂移 → 落库前 DB 复验剔除
//   - 综合失败 = 0 调查行、父数据零改写（调查失败不改写父数据）

// crossBoardSearcher returns one foreign-board lane (id 977, board 5) so the
// research loop's search_internal_context call grants it.
type crossBoardSearcher struct{ queries []string }

func (c *crossBoardSearcher) SearchInternalContext(_ context.Context, query string, _ int) ([]service.InternalContextHit, error) {
	c.queries = append(c.queries, query)
	id := uint(977)
	return []service.InternalContextHit{
		{Kind: "lane", BoardID: 5, LaneID: &id, Label: "日债收益率", Status: "active", HitCount: 28, Summary: "收益率走高"},
	}, nil
}

func newCrossBoardOrch(t *testing.T, enabled bool, router service.AirRouter) (*service.OrchestratorService, *repository.Repository) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	repo := repository.NewRepository(db)
	registry := service.NewRegistry(&nilFetcher{},
		service.WithLaneDetailRenderer(invLaneRenderer{}),
		service.WithWebSearcher(invWebSearcher{}),
		service.WithInternalContextSearcher(&crossBoardSearcher{}))
	orch := service.NewOrchestratorService(
		router, repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		registry, &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	orch.SetBoardConfigResolver(&mockBoardResolver{enabled: enabled})
	return orch, repo
}

// seedCrossBoardLane inserts a persistent topic lane owned by board 5 and
// returns its id (977).
func seedCrossBoardLane(t *testing.T, repo *repository.Repository, ownerBoard uint, laneID uint) {
	t.Helper()
	require.NoError(t, repo.DB().Exec(`INSERT INTO board_persistent_topics
		(id, semantic_board_id, label, status, hit_count, consecutive_hits, first_seen_date, last_seen_date, created_at, updated_at)
		VALUES (?, ?, '日债收益率', 'active', 28, 4, DATE '2026-08-01', DATE '2026-09-01', now(), now())
		ON CONFLICT (id) DO NOTHING`, laneID, ownerBoard).Error)
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM board_persistent_topics WHERE id = ?`, laneID).Error
	})
}

// invSynthesisCrossBoardLLM: e2 references lane 977 (cross-board, granted);
// lane_refs carries lane 977 (cross-board) + 901 (board-local) + 9999 (ghost).
const invSynthesisCrossBoardLLM = `{"hypotheses":[
 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},
 {"id":"h1","label":"外部传导推动","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
 {"id":"h2","label":"政策同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":["缺时间线"]}],
 "conclusion":{"summary":"外部传导获公告支持","confidence":"medium","scope":"两板块","boundary":"资金明细未核"},
 "evidence_chain":[
  {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例研究所","date":"2026-08-20","supports":["h1"],"counters":[]},
  {"id":"e2","source_type":"lane","ref":"977","lane_note":"外部泳道事实","supports":["h0"],"counters":[]}],
 "lane_refs":[{"lane_id":901,"note":"本版块"},{"lane_id":977,"note":"跨版块授权"},{"lane_id":9999,"note":"幽灵"}]}`

// addCrossBoardChain appends an investigation chain whose research loop first
// discovers lane 977 via search_internal_context (grant) and reads it.
func addCrossBoardChain(router *mockAirRouter, synthResp string) {
	router.addResponse(invHypothesesLLM)
	router.addResponse(`{"action":"call_tool","thought":"发现跨版块泳道","tool":"search_internal_context","args":{"query":"日债 走高 原因"},"purpose":"neutral","hypothesis_ids":[]}`)
	router.addResponse(`{"action":"call_tool","thought":"读取授权泳道","tool":"get_lane_detail","args":{"lane_id":977},"purpose":"support","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"独立 原因"},"purpose":"counter","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴 时间线"},"purpose":"counter","hypothesis_ids":["h2"]}`)
	router.addResponse(`{"action":"finish","thought":"纪律已补齐","summary":"h1 支持与反证齐备；中性检索完成。"}`)
	router.addResponse(synthResp)
}

type invSnapshotForCrossBoard struct {
	LaneWhitelist []uint `json:"lane_whitelist"`
	DynamicGrants []struct {
		LaneID  uint   `json:"lane_id"`
		BoardID uint   `json:"board_id"`
		Tool    string `json:"tool"`
		Step    int    `json:"step"`
	} `json:"dynamic_grants"`
}

func TestBoardInvestigationCrossBoardLaneRefPersists(t *testing.T) {
	router := newMockAirRouter()
	addCrossBoardChain(router, invSynthesisCrossBoardLLM)
	orch, repo := newCrossBoardOrch(t, true, router)

	boardID := seedBoardForInv(t, repo)
	seedLanesForInv(t, repo, boardID)
	seedCrossBoardLane(t, repo, 5, 977)
	parent := seedInvBrief(t, repo, boardID)

	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, parent.ID,
		service.BoardInvestigationQuestion{ID: "q1", Text: "外部传导是否成立", Source: "generated"})
	require.NoError(t, err)
	require.NotNil(t, out.Result)

	// Lane refs: ghost 9999 scrubbed; cross-board 977 survives WITH board id;
	// board-local 901 keeps board_id 0.
	var payload struct {
		LaneRefs []struct {
			LaneID  uint   `json:"lane_id"`
			BoardID uint   `json:"board_id"`
			Note    string `json:"note"`
		} `json:"lane_refs"`
	}
	require.NoError(t, json.Unmarshal(out.Result.Sectors, &payload))
	byLane := map[uint]struct {
		LaneID  uint   `json:"lane_id"`
		BoardID uint   `json:"board_id"`
		Note    string `json:"note"`
	}{}
	for _, r := range payload.LaneRefs {
		byLane[r.LaneID] = r
	}
	require.Contains(t, byLane, uint(901))
	require.Equal(t, uint(0), byLane[901].BoardID)
	require.Contains(t, byLane, uint(977), "granted cross-board lane must survive")
	require.Equal(t, uint(5), byLane[977].BoardID, "cross-board ref carries owning board id")
	require.NotContains(t, byLane, uint(9999), "ghost ref scrubbed")

	// Input snapshot freezes the grant audit (lane 977, board 5, tool, step).
	var snap invSnapshotForCrossBoard
	require.NoError(t, json.Unmarshal(out.Result.InputSnapshot, &snap))
	require.Contains(t, snap.LaneWhitelist, uint(901))
	found := false
	for _, g := range snap.DynamicGrants {
		if g.LaneID == 977 {
			found = true
			require.Equal(t, uint(5), g.BoardID)
			require.Equal(t, "search_internal_context", g.Tool)
			require.Equal(t, 1, g.Step)
		}
	}
	require.True(t, found, "grant audit must be frozen into input_snapshot")

	// Parent brief bytes unchanged (父简报归属保持不变).
	fresh, err := repo.GetTopicEnrichmentResultByID(context.Background(), parent.ID)
	require.NoError(t, err)
	require.JSONEq(t, invBriefSectors, string(fresh.Sectors))
}

func TestBoardInvestigationCrossBoardOwnerDriftReferenceDropped(t *testing.T) {
	router := newMockAirRouter()
	addCrossBoardChain(router, invSynthesisCrossBoardLLM)
	orch, repo := newCrossBoardOrch(t, true, router)

	boardID := seedBoardForInv(t, repo)
	seedLanesForInv(t, repo, boardID)
	// Lane 977 REALLY belongs to board 42 — the grant said board 5 (searcher
	// mock), so ownership drifted between grant and persist.
	seedCrossBoardLane(t, repo, 42, 977)
	parent := seedInvBrief(t, repo, boardID)

	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, parent.ID,
		service.BoardInvestigationQuestion{ID: "q1", Text: "外部传导是否成立", Source: "generated"})
	require.NoError(t, err)

	var payload struct {
		LaneRefs []struct {
			LaneID uint `json:"lane_id"`
		} `json:"lane_refs"`
	}
	require.NoError(t, json.Unmarshal(out.Result.Sectors, &payload))
	for _, r := range payload.LaneRefs {
		require.NotEqual(t, uint(977), r.LaneID, "drifted cross-board reference must be dropped at persist gate")
	}
}

func TestBoardInvestigationCrossBoardSynthesisFailureZeroRows(t *testing.T) {
	router := newMockAirRouter()
	// Same discovery chain, but the synthesize step returns garbage twice →
	// the whole investigation fails with zero rows and untouched parents.
	router.addResponse(invHypothesesLLM)
	router.addResponse(`{"action":"call_tool","thought":"发现跨版块泳道","tool":"search_internal_context","args":{"query":"日债"},"purpose":"neutral","hypothesis_ids":[]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"独立"},"purpose":"counter","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴"},"purpose":"counter","hypothesis_ids":["h2"]}`)
	router.addResponse(`{"action":"finish","thought":"完成","summary":"素材汇总。"}`)
	router.addResponse(`not-json-at-all`)
	router.addResponse(`{"hypotheses":[]}`)
	orch, repo := newCrossBoardOrch(t, true, router)

	boardID := seedBoardForInv(t, repo)
	seedLanesForInv(t, repo, boardID)
	seedCrossBoardLane(t, repo, 5, 977)
	parent := seedInvBrief(t, repo, boardID)

	_, err := orch.InvestigateBoardQuestion(context.Background(), boardID, parent.ID,
		service.BoardInvestigationQuestion{ID: "q1", Text: "外部传导是否成立", Source: "generated"})
	require.Error(t, err, "synthesis failure must fail the whole investigation")

	// Zero investigation rows (调查失败不改写父数据).
	var count int64
	require.NoError(t, repo.DB().Model(&repository.TopicEnrichmentResult{}).
		Where("parent_result_id = ?", parent.ID).Count(&count).Error)
	require.Zero(t, count)

	// Parent brief bytes unchanged; lane lifeline untouched (no table writes
	// beyond reads).
	fresh, err := repo.GetTopicEnrichmentResultByID(context.Background(), parent.ID)
	require.NoError(t, err)
	require.JSONEq(t, invBriefSectors, string(fresh.Sectors))
}

// seedBoardForInv inserts an enabled board label and returns its id.
func seedBoardForInv(t *testing.T, repo *repository.Repository) uint {
	t.Helper()
	var id uint
	require.NoError(t, repo.DB().Raw(`INSERT INTO semantic_labels (label, slug, label_type, status, enrichment_enabled, created_at, updated_at)
		VALUES ('跨版块调查源板块', 'cbr-inv-board', 'board', 'active', true, now(), now()) RETURNING id`).Scan(&id).Error)
	require.NotZero(t, id)
	t.Cleanup(func() { _ = repo.DB().Exec(`DELETE FROM semantic_labels WHERE id = ?`, id).Error })
	return id
}

// seedLanesForInv inserts lanes 901/902 under the board (matching the seeded
// brief's whitelist).
func seedLanesForInv(t *testing.T, repo *repository.Repository, boardID uint) {
	t.Helper()
	for _, lane := range []uint{901, 902} {
		require.NoError(t, repo.DB().Exec(`INSERT INTO board_persistent_topics
			(id, semantic_board_id, label, status, hit_count, consecutive_hits, first_seen_date, last_seen_date, created_at, updated_at)
			VALUES (?, ?, ?, 'active', 5, 2, DATE '2026-08-01', DATE '2026-09-01', now(), now())
			ON CONFLICT (id) DO NOTHING`, lane, boardID, fmt.Sprintf("泳道%d", lane)).Error)
	}
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM board_persistent_topics WHERE semantic_board_id = ?`, boardID).Error
	})
}
