package service_test

import (
	"context"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── board_interpret（tasks 3.2 / M2）────────────────────────────────────────

func newBoardInterpretOrch(t *testing.T) (*service.OrchestratorService, *mockAirRouter, *repository.Repository) {
	t.Helper()
	repo := setupOrchTestDB(t)
	router := newMockAirRouter()
	orch := service.NewOrchestratorService(
		router, repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		service.NewRegistry(&nilFetcher{}), &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	return orch, router, repo
}

func boardCardsMD() string {
	cards := []service.LaneSituationCard{
		{LaneID: 1, Label: "美债供给", HitCount: 30, ConsecutiveHits: 9, LastSeenDate: "2026-08-26", DaysSinceSeen: 0, QualityScore: 30, DetailLevel: "full", FactsDigest: "本周拍卖遇冷，长端利率上行"},
		{LaneID: 2, Label: "美元信用", HitCount: 22, ConsecutiveHits: 7, LastSeenDate: "2026-08-25", DaysSinceSeen: 1, QualityScore: 25, DetailLevel: "full", FactsDigest: "多国央行增持黄金"},
	}
	return service.RenderSituationCardsMarkdown(cards)
}

func topCard() *service.LaneSituationCard {
	return &service.LaneSituationCard{LaneID: 1, Label: "美债供给", FactsDigest: "本周拍卖遇冷，长端利率上行"}
}

// M2.1 正常候选：chosen ∈ candidates，每候选钩子×切角。
func TestBoardInterpret_ValidCandidates(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	router.addResponse(`{"form":"board","candidates":[
		{"thesis":"美债不是无风险资产，而是政治议价的抵押品","hook":"拍卖遇冷","angle":"概念重命名"},
		{"thesis":"美元信用正在从制度信任转向抵押信任","hook":"央行购金","angle":"机制保质期"}],
		"chosen_index":1,"reason":"购金潮跨多泳道"}`)

	out, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s1", CardsMD: boardCardsMD(), TopCard: topCard(),
	})
	if err != nil {
		t.Fatalf("board interpret: %v", err)
	}
	if out.Form != "board" || len(out.Candidates) != 2 || out.ChosenIndex != 1 {
		t.Fatalf("parse wrong: %+v", out)
	}
	if out.Candidates[1].Thesis == "" || out.Candidates[1].Hook == "" || out.Candidates[1].Angle == "" {
		t.Fatalf("candidate fields missing: %+v", out.Candidates[1])
	}
	if out.Degraded {
		t.Fatal("valid response must not be degraded")
	}
	// Operation name recorded for the ai-logging contract.
	if op := router.Calls[0].Operation; op != "data_enrichment.board_interpret" {
		t.Fatalf("operation: want data_enrichment.board_interpret, got %s", op)
	}
}

// M2.2 LLM 输出烂两次 → 机械降级单候选，标注 degraded，不静默失败。
func TestBoardInterpret_DegradedFallback(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	router.addResponse(`{"form":"board","candidates":`) // truncated
	router.addResponse(`not json at all`)

	out, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s2", CardsMD: boardCardsMD(), TopCard: topCard(),
	})
	if err != nil {
		t.Fatalf("degraded path must not error: %v", err)
	}
	if !out.Degraded {
		t.Fatal("fallback must be marked degraded")
	}
	if len(out.Candidates) != 1 {
		t.Fatalf("degraded: want 1 mechanical candidate, got %d", len(out.Candidates))
	}
	if !strings.Contains(out.Candidates[0].Thesis, "美债供给") {
		t.Fatalf("degraded thesis should come from top card: %q", out.Candidates[0].Thesis)
	}
	if out.DegradedWhy == "" {
		t.Fatal("degraded_why must record the cause")
	}
	if router.callCount != 2 {
		t.Fatalf("must retry once: got %d calls", router.callCount)
	}
}

// M2.2b 一次烂一次好：重试成功即用重试结果。
func TestBoardInterpret_RetryRecovers(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	router.addResponse(`broken`)
	router.addResponse(`{"form":"board","candidates":[{"thesis":"T1","hook":"H1","angle":"A1"}],"chosen_index":0,"reason":"r"}`)

	out, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s3", CardsMD: boardCardsMD(), TopCard: topCard(),
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if out.Degraded || out.Candidates[0].Thesis != "T1" {
		t.Fatalf("retry result wrong: %+v", out)
	}
}

// M2.3 悬空 chosen → 解析失败 → 降级路径。
func TestBoardInterpret_DanglingChosen(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	// chosen_index=5 outside candidates; both attempts dangle → degraded.
	router.addResponse(`{"form":"board","candidates":[{"thesis":"T","hook":"H","angle":"A"}],"chosen_index":5,"reason":"r"}`)
	router.addResponse(`{"form":"board","candidates":[{"thesis":"T2","hook":"H2","angle":"A2"}],"chosen_index":9,"reason":"r"}`)

	out, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s4", CardsMD: boardCardsMD(), TopCard: topCard(),
	})
	if err != nil {
		t.Fatalf("dangling chosen must fall to degraded, got err %v", err)
	}
	if !out.Degraded {
		t.Fatal("dangling chosen must not be accepted")
	}
}

// M2.4 全 sparse → form=sparse 诚实降级，thesis 明示素材不足。
func TestBoardInterpret_SparseHonest(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	router.addResponse(`{"form":"sparse","reason":"多数泳道无事实摘要","candidates":[{"thesis":"素材不足，无法形成结构命题","hook":"","angle":""}]}`)

	out, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s5", CardsMD: "（空）", AllSparse: true, TopCard: topCard(),
	})
	if err != nil {
		t.Fatalf("sparse: %v", err)
	}
	if out.Form != "sparse" {
		t.Fatalf("want sparse, got %s", out.Form)
	}
	if !strings.Contains(out.Candidates[0].Thesis, "素材不足") {
		t.Fatalf("sparse thesis must be honest about shortfall: %q", out.Candidates[0].Thesis)
	}
	// Prompt carries the sparse note when AllSparse.
	found := false
	for _, c := range router.Calls {
		if strings.Contains(c.Messages[0].Content, "素材信号都很稀薄") {
			found = true
		}
	}
	if !found {
		t.Fatal("AllSparse prompt must carry the sparse honesty note")
	}
}

// M2.5 态势卡为空（无泳道）→ 不调 LLM，直接拒绝且错误可区分。
func TestBoardInterpret_NoLanes(t *testing.T) {
	orch, router, _ := newBoardInterpretOrch(t)
	_, err := orch.BoardInterpretForTest(context.Background(), service.BoardInterpretInputForTest{
		SessionID: "s6", CardsMD: "", TopCard: nil,
	})
	if err == nil {
		t.Fatal("empty card set must be rejected")
	}
	if !strings.Contains(err.Error(), "no lanes") {
		t.Fatalf("error must be distinguishable (no lanes), got: %v", err)
	}
	if len(router.Calls) != 0 {
		t.Fatal("must not call LLM on empty card set")
	}
}
