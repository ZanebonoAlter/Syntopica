package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 跨版块关系发现 API 契约（add-evidence-backed-cross-board-relations 4.1/4.2）──

// relationBriefRow seeds a board_brief carrying observation o1 + question q1.
func relationBriefRow(t *testing.T, db *gorm.DB, boardID uint) *repository.TopicEnrichmentResult {
	t.Helper()
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardBrief,
		Sectors: []byte(`{"scope":"board","result_kind":"board_brief","summary":"观察与问题。",` +
			`"observations":[{"id":"o1","lane_id":901,"statement":"日债收益率走高","basis":"周摘要","as_of_date":"2026-09-01"}],` +
			`"research_questions":[{"id":"q1","question":"外部驱动是什么","rationale":"r","related_lane_ids":[901]}]}`),
		SessionID: fmt.Sprintf("rel-handler-brief-%d", boardID),
	}
	return res
}

func postRelationDiscover(t *testing.T, r *gin.Engine, boardID uint, body string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/relations/discover", boardID), bodyReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w.Code, parsed
}

func callRelationAPI(t *testing.T, r *gin.Engine, method string, path string, body string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body != "" {
		req, _ = http.NewRequest(method, path, bodyReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, path, nil)
	}
	r.ServeHTTP(w, req)
	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w.Code, parsed
}

// TestRelationDiscoveryTrigger_ObservationAndQuestion: 202 job envelope for
// both source kinds; the background run receives the server-validated ref.
func TestRelationDiscoveryTrigger_ObservationAndQuestion(t *testing.T) {
	db := setupHandlerTestDB(t)
	repo := repository.NewRepository(db)
	brief := relationBriefRow(t, db, 5)
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctxBG(), brief))

	orch := &mockOrchestrator{relationBlock: make(chan struct{})}
	r := newBoardAnalysisRouter(t, orch, db)

	code, body := postRelationDiscover(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"o1"}`, brief.ID))
	require.Equal(t, http.StatusAccepted, code)
	env := triggerEnvelope(t, body)
	require.Equal(t, "started", env["status"])
	require.Equal(t, "relation_discovery", env["job_kind"])
	jobID := env["job_id"].(string)
	require.NotEmpty(t, jobID)

	// Duplicate submit while running → 409 carrying current identity.
	code2, body2 := postRelationDiscover(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"o1"}`, brief.ID))
	require.Equal(t, http.StatusConflict, code2)
	require.NotNil(t, body2["data"])

	// Question kind also accepted (distinct source → distinct job target).
	code3, _ := postRelationDiscover(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"question","source_key":"q1"}`, brief.ID))
	require.Equal(t, http.StatusAccepted, code3)

	close(orch.relationBlock)
	require.Eventually(t, func() bool { return orch.relationCalls == 2 }, 5*time.Second, 50*time.Millisecond)
	// The orchestrator received exactly the two validated refs (no client
	// text ever reaches the pipeline).
	keys := map[string]bool{}
	for _, in := range orch.relationInputs {
		require.Equal(t, uint(5), in.BoardID)
		keys[in.Source.SourceKey] = true
	}
	require.True(t, keys["o1"])
	require.True(t, keys["q1"])
}

// TestRelationDiscoveryTrigger_PreFlightRejections: bad kind / missing key /
// cross-board parent / non-brief parent / unknown source key → 4xx with zero
// background calls.
func TestRelationDiscoveryTrigger_PreFlightRejections(t *testing.T) {
	db := setupHandlerTestDB(t)
	repo := repository.NewRepository(db)
	brief := relationBriefRow(t, db, 5)
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctxBG(), brief))
	legacy := legacyResultRow(t, db, 5, "旧论点")

	orch := &mockOrchestrator{}
	r := newBoardAnalysisRouter(t, orch, db)

	cases := []struct {
		name string
		body string
		code int
	}{
		{"missing briefing id", `{"source_kind":"observation","source_key":"o1"}`, http.StatusBadRequest},
		{"illegal kind", fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"wild","source_key":"o1"}`, brief.ID), http.StatusBadRequest},
		{"empty key", fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"  "}`, brief.ID), http.StatusBadRequest},
		{"unknown parent", `{"briefing_result_id":999999,"source_kind":"observation","source_key":"o1"}`, http.StatusNotFound},
		{"legacy parent kind", fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"o1"}`, legacy.ID), http.StatusBadRequest},
		{"unknown source key", fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"o404"}`, brief.ID), http.StatusBadRequest},
		{"unknown question key", fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"question","source_key":"q404"}`, brief.ID), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := postRelationDiscover(t, r, 5, tc.body)
			require.Equal(t, tc.code, code)
		})
	}
	// Cross-board parent → 404.
	code, _ := postRelationDiscover(t, r, 6, fmt.Sprintf(`{"briefing_result_id":%d,"source_kind":"observation","source_key":"o1"}`, brief.ID))
	require.Equal(t, http.StatusNotFound, code)
	require.Zero(t, orch.relationCalls, "all rejections must cost zero background calls")
}

// TestRelationListFilterAndDetail: list filters by status (invalid → 400),
// matches either side; detail returns evidence + run linkage.
func TestRelationListFilterAndDetail(t *testing.T) {
	db := setupHandlerTestDB(t)
	repo := repository.NewRepository(db)
	target := uint(9)
	run := &repository.CrossBoardRelationRun{
		SourceBoardID: 5, ParentResultID: 1, SourceKind: "observation", SourceKey: "o1",
		SourceText: "x", TriggerKind: "manual", Status: repository.RelationRunStatusSucceeded,
	}
	require.NoError(t, repo.CreateRelationRun(ctxBG(), run))
	relProposed := &repository.CrossBoardRelation{
		RunID: &run.ID, SourceBoardID: 5, TargetBoardID: &target, TargetConcept: "日债",
		RelationType: repository.RelationTypeCausal, Claim: "c1", VerificationVerdict: repository.RelationVerdictSupported,
		QualityGrade: repository.RelationGradeMedium, Status: repository.RelationStatusProposed,
		SuggestionHash: "rel-h-list-1", EvidenceVersion: "v1",
	}
	require.NoError(t, db.Create(relProposed).Error)
	relConfirmed := &repository.CrossBoardRelation{
		SourceBoardID: 5, TargetBoardID: &target, TargetConcept: "通胀",
		RelationType: repository.RelationTypeCommonDriver, Claim: "c2",
		VerificationVerdict: repository.RelationVerdictSupported, QualityGrade: repository.RelationGradeHigh,
		Status: repository.RelationStatusConfirmed, SuggestionHash: "rel-h-list-2",
	}
	require.NoError(t, db.Create(relConfirmed).Error)

	orch := &mockOrchestrator{}
	r := newBoardAnalysisRouter(t, orch, db)

	// Source side list with status filter.
	code, body := callRelationAPI(t, r, "GET", "/semantic-boards/5/enrichment/analysis/relations?status=proposed", "")
	require.Equal(t, http.StatusOK, code)
	items := body["data"].([]any)
	require.Len(t, items, 1)

	// Target side list sees both.
	code2, body2 := callRelationAPI(t, r, "GET", "/semantic-boards/9/enrichment/analysis/relations", "")
	require.Equal(t, http.StatusOK, code2)
	require.Len(t, body2["data"].([]any), 2)

	// Invalid status filter → 400.
	code3, _ := callRelationAPI(t, r, "GET", "/semantic-boards/5/enrichment/analysis/relations?status=bogus", "")
	require.Equal(t, http.StatusBadRequest, code3)

	// Detail carries evidence fields + run linkage.
	code4, body4 := callRelationAPI(t, r, "GET", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d", relProposed.ID), "")
	require.Equal(t, http.StatusOK, code4)
	detail := body4["data"].(map[string]any)
	require.Equal(t, "c1", detail["claim"])
	require.Equal(t, "proposed", detail["status"])
	runLink := detail["run"].(map[string]any)
	require.Equal(t, float64(run.ID), runLink["id"])

	// Foreign board → 404 (ownership enforced on either side).
	code5, _ := callRelationAPI(t, r, "GET", fmt.Sprintf("/semantic-boards/77/enrichment/analysis/relations/%d", relProposed.ID), "")
	require.Equal(t, http.StatusNotFound, code5)
}

// TestRelationConfirmDismissLifecycle: confirm proposed → 200 confirmed with
// expires_at; confirm non-proposed → 409; dismiss without reason → 400;
// dismiss unresolved → 200; re-dismiss terminal → 409.
func TestRelationConfirmDismissLifecycle(t *testing.T) {
	db := setupHandlerTestDB(t)
	target := uint(9)
	// A live board label for the confirm-time target re-validation.
	require.NoError(t, db.Exec(`INSERT INTO semantic_labels (id, label, slug, label_type, status, created_at, updated_at)
		VALUES (9, '目标板块', 'rel-h-target', 'board', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`).Error)
	relProposed := &repository.CrossBoardRelation{
		SourceBoardID: 5, TargetBoardID: &target, TargetConcept: "日债",
		RelationType: repository.RelationTypeCausal, Claim: "c1",
		VerificationVerdict: repository.RelationVerdictSupported, QualityGrade: repository.RelationGradeMedium,
		Status: repository.RelationStatusProposed, SuggestionHash: "rel-h-cd-1",
	}
	require.NoError(t, db.Create(relProposed).Error)
	relUnres := &repository.CrossBoardRelation{
		SourceBoardID: 5, TargetConcept: "通胀", RelationType: repository.RelationTypeUnclear,
		Claim: "c2", VerificationVerdict: repository.RelationVerdictInsufficient,
		Status: repository.RelationStatusUnresolved, SuggestionHash: "rel-h-cd-2",
	}
	require.NoError(t, db.Create(relUnres).Error)

	orch := &mockOrchestrator{}
	r := newBoardAnalysisRouter(t, orch, db)

	// Confirm proposed → 200, expires_at populated.
	code, body := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/confirm", relProposed.ID), "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "confirmed", body["data"].(map[string]any)["status"])
	require.NotNil(t, body["data"].(map[string]any)["expires_at"])

	// Double confirm → 409.
	code2, _ := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/confirm", relProposed.ID), "")
	require.Equal(t, http.StatusConflict, code2)

	// Confirm unresolved (nil target) → 409.
	code3, _ := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/confirm", relUnres.ID), "")
	require.Equal(t, http.StatusConflict, code3)

	// Dismiss without reason → 400.
	code4, _ := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/dismiss", relUnres.ID), `{}`)
	require.Equal(t, http.StatusBadRequest, code4)

	// Dismiss unresolved with reason → 200.
	code5, body5 := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/dismiss", relUnres.ID), `{"reason":"噪音"}`)
	require.Equal(t, http.StatusOK, code5)
	require.Equal(t, "dismissed", body5["data"].(map[string]any)["status"])

	// Re-dismiss terminal → 409.
	code6, _ := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/dismiss", relUnres.ID), `{"reason":"again"}`)
	require.Equal(t, http.StatusConflict, code6)
}

// TestRelationReResolve: non-unresolved → 409; unresolved → 200 via mock.
func TestRelationReResolve(t *testing.T) {
	db := setupHandlerTestDB(t)
	target := uint(9)
	relUnres := &repository.CrossBoardRelation{
		SourceBoardID: 5, TargetConcept: "通胀", RelationType: repository.RelationTypeUnclear,
		Claim: "c", VerificationVerdict: repository.RelationVerdictInsufficient,
		Status: repository.RelationStatusUnresolved, SuggestionHash: "rel-h-rr-1",
	}
	require.NoError(t, db.Create(relUnres).Error)
	relProp := &repository.CrossBoardRelation{
		SourceBoardID: 5, TargetBoardID: &target, TargetConcept: "x",
		RelationType: repository.RelationTypeCausal, Claim: "c2",
		VerificationVerdict: repository.RelationVerdictSupported, Status: repository.RelationStatusProposed,
		SuggestionHash: "rel-h-rr-2",
	}
	require.NoError(t, db.Create(relProp).Error)

	orch := &mockOrchestrator{reResolveOut: &service.RelationReResolveOutput{NewStatus: "proposed", Outcome: "resolved"}}
	r := newBoardAnalysisRouter(t, orch, db)

	code, body := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/re-resolve", relUnres.ID), "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "proposed", body["data"].(map[string]any)["new_status"])
	require.Equal(t, relUnres.ID, orch.lastReResolveID)

	code2, _ := callRelationAPI(t, r, "POST", fmt.Sprintf("/semantic-boards/5/enrichment/analysis/relations/%d/re-resolve", relProp.ID), "")
	require.Equal(t, http.StatusConflict, code2)
	require.Equal(t, 1, orch.reResolveCalls)
}

func ctxBG() context.Context { return context.Background() }
