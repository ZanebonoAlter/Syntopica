package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
)

// Context 端点的可选 semantic_board_id 归属校验（redesign-enrichment-research-workbench
// 4.3 / spec「话题新闻背景独立上下文」Scenario「背景失效与晚响应」）：
//   - 传入时：persistent_topic 必须存在且属于该板块，否则 404（topic 不存在与
//     板块不符不可区分，不泄露存在性）；
//   - 不传：行为完全不变（旧客户端兼容，零行为差异）。
//
// 独立文件避免与并行会话共享的 handler_test.go 冲突；测试脚手架（setupHandlerTestDB /
// newTestRouter / doRequest / expectJSON* / mockLifelineService）复用同包既有实现。

// seedGuardTopic 种一个属于板块 boardID 的话题行，返回话题 id。
func seedGuardTopic(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	if err := db.Exec(`INSERT INTO board_persistent_topics
		(semantic_board_id, label, status, first_seen_date, last_seen_date, created_at, updated_at)
		VALUES (?, '测试话题', 'active', '2026-08-01', '2026-08-31', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		boardID).Error; err != nil {
		t.Fatalf("seed topic: %v", err)
	}
	var id uint
	if err := db.Raw(`SELECT id FROM board_persistent_topics ORDER BY id DESC LIMIT 1`).Scan(&id).Error; err != nil {
		t.Fatalf("read seeded topic id: %v", err)
	}
	return id
}

// seedGuardContext 为 topicID 种一条 week 周期背景。
func seedGuardContext(t *testing.T, topicID uint, content string) {
	t.Helper()
	lc := &repository.TopicLifelineContext{
		PersistentTopicID: topicID, Granularity: "week", Period: "2026-W27", Content: content,
	}
	if err := repository.Repo.UpsertTopicLifelineContext(context.Background(), lc); err != nil {
		t.Fatalf("seed context: %v", err)
	}
}

func TestContextBoardGuard_ListMatchAndMismatch(t *testing.T) {
	db := setupHandlerTestDB(t)
	topicID := seedGuardTopic(t, db, 5)
	seedGuardContext(t, topicID, "week summary")

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)
	base := fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts", topicID)

	// 不传 → 行为不变。
	w := doRequest(t, r, "GET", base, "")
	var list []repository.TopicLifelineContext
	expectJSONSuccess(t, w, &list)
	if len(list) != 1 {
		t.Fatalf("legacy no-param list count = %d, want 1", len(list))
	}

	// 传入且归属正确 → 200。
	w = doRequest(t, r, "GET", base+"?semantic_board_id=5", "")
	list = nil
	expectJSONSuccess(t, w, &list)
	if len(list) != 1 {
		t.Fatalf("matched list count = %d, want 1", len(list))
	}

	// 传入但板块不符 → 404（与 topic 不存在不可区分）。
	w = doRequest(t, r, "GET", base+"?semantic_board_id=6", "")
	expectJSONError(t, w, http.StatusNotFound)
}

func TestContextBoardGuard_GetContextMatchAndMismatch(t *testing.T) {
	db := setupHandlerTestDB(t)
	topicID := seedGuardTopic(t, db, 5)
	seedGuardContext(t, topicID, "week summary")

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)
	base := fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts/week/2026-W27", topicID)

	w := doRequest(t, r, "GET", base+"?semantic_board_id=5", "")
	var got repository.TopicLifelineContext
	expectJSONSuccess(t, w, &got)
	if got.Content != "week summary" {
		t.Fatalf("content = %q, want 'week summary'", got.Content)
	}

	w = doRequest(t, r, "GET", base+"?semantic_board_id=6", "")
	expectJSONError(t, w, http.StatusNotFound)
}

func TestContextBoardGuard_TopicMissingOrForeignIs404(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// topic 不存在 → 404（不泄露存在性，与板块不符同码）。
	w := doRequest(t, r, "GET", "/api/persistent-topics/99/enrichment/contexts?semantic_board_id=5", "")
	expectJSONError(t, w, http.StatusNotFound)

	// topic 存在但属板块 5，传板块 6 → 404。
	topicID := seedGuardTopic(t, db, 5)
	w = doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts?semantic_board_id=6", topicID), "")
	expectJSONError(t, w, http.StatusNotFound)
}

func TestContextBoardGuard_UpdateMismatchKeepsContent(t *testing.T) {
	db := setupHandlerTestDB(t)
	topicID := seedGuardTopic(t, db, 5)
	seedGuardContext(t, topicID, "original content")

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)
	put := fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts/week/2026-W27", topicID)

	// 板块不符 → 404 且内容不被改动。
	w := doRequest(t, r, "PUT", put+"?semantic_board_id=6", `{"content": "hacked"}`)
	expectJSONError(t, w, http.StatusNotFound)
	lc, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, "week", "2026-W27")
	if err != nil {
		t.Fatalf("reload context: %v", err)
	}
	if lc.Content != "original content" {
		t.Fatalf("content mutated on 404 path: %q", lc.Content)
	}

	// 归属正确 → 200，旧 PUT 语义不变（source=manual）。
	w = doRequest(t, r, "PUT", put+"?semantic_board_id=5", `{"content": "edited content"}`)
	var updated repository.TopicLifelineContext
	expectJSONSuccess(t, w, &updated)
	if updated.Content != "edited content" || updated.Source != "manual" {
		t.Fatalf("updated = %q/%q, want edited content/manual", updated.Content, updated.Source)
	}
}

func TestContextBoardGuard_RegenerateMismatchBlocked(t *testing.T) {
	db := setupHandlerTestDB(t)
	topicID := seedGuardTopic(t, db, 5)
	seedGuardContext(t, topicID, "old content")

	mockLifeline := &mockLifelineService{}
	h := newTestHandler(db, mockLifeline, nil, nil)
	r := newTestRouter(h)
	post := fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts/week/regenerate?period=2026-W27", topicID)

	// 板块不符 → 404 且不触发生成服务。
	w := doRequest(t, r, "POST", post+"&semantic_board_id=6", "")
	expectJSONError(t, w, http.StatusNotFound)
	if mockLifeline.lastTopicID != 0 {
		t.Fatalf("RefreshPeriod called despite 404 guard (topicID=%d)", mockLifeline.lastTopicID)
	}

	// 归属正确 → 200 并触发 RefreshPeriod。
	w = doRequest(t, r, "POST", post+"&semantic_board_id=5", "")
	expectJSONSuccess(t, w, nil)
	if mockLifeline.lastTopicID != topicID || mockLifeline.lastPeriod != "2026-W27" {
		t.Fatalf("RefreshPeriod = (%d, %s), want (%d, 2026-W27)",
			mockLifeline.lastTopicID, mockLifeline.lastPeriod, topicID)
	}
}

func TestContextBoardGuard_InvalidBoardParamIs400(t *testing.T) {
	db := setupHandlerTestDB(t)
	topicID := seedGuardTopic(t, db, 5)
	seedGuardContext(t, topicID, "week summary")

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET",
		fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts?semantic_board_id=abc", topicID), "")
	expectJSONError(t, w, http.StatusBadRequest)

	w = doRequest(t, r, "GET",
		fmt.Sprintf("/api/persistent-topics/%d/enrichment/contexts?semantic_board_id=0", topicID), "")
	expectJSONError(t, w, http.StatusBadRequest)
}
