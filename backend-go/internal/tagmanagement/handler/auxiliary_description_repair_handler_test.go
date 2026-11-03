package handler

// board-health-triage-advice seam B2 (task 2.2) — HTTP contract of the three
// auxiliary-label description-repair resources, against a real testcontainer
// Postgres (the repair chain is DB-backed; the handler package's light CRUD
// endpoints keep their SQLite setup, this one needs the real indexes).
//
// Covered: the context payload shape (label + opaque version + direct boards +
// active tag count + latest task), the pure-read guarantee, the disabled /
// non-auxiliary / missing 404s, the three description rules, the idempotency
// key and version boundaries (400, never a DB error → 500), the same-key
// replay, the same-key/different-payload conflict, the per-label mutex, the
// job ownership check, and the post-delete replay (review B-P2-1).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service"
)

// init registers the description-repair models for THIS test binary's
// AutoMigrate: production registers them via tagmanagement/wire.go's init →
// database.RegisterModels, but the handler package cannot import the
// tagmanagement root (root → handler import cycle). Same pattern as
// repository/auxiliary_description_repair_test.go.
func init() {
	database.RegisterModels(
		&tagmodels.AuxiliaryDescriptionRepairJob{},
		&tagmodels.AuxiliaryDescriptionRepairItem{},
	)
}

// ── setup & helpers ─────────────────────────────────────────────────────────

func setupAuxRepairHandler(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	svc, err := service.NewAuxRepairService(db)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterAuxiliaryDescriptionRepairRoutes(api, svc)
	return db, r
}

var auxRepairSeedSeq int

func seedAuxRepairLabel(t *testing.T, db *gorm.DB, label, labelType, status, description string) models.SemanticLabel {
	t.Helper()
	auxRepairSeedSeq++
	l := models.SemanticLabel{
		Label:       label,
		Slug:        fmt.Sprintf("aux-repair-%d", auxRepairSeedSeq),
		LabelType:   labelType,
		Status:      status,
		Description: description,
	}
	require.NoError(t, db.Create(&l).Error)
	return l
}

func seedAuxRepairTag(t *testing.T, db *gorm.DB, label, status string) models.TopicTag {
	t.Helper()
	auxRepairSeedSeq++
	tag := models.TopicTag{Label: label, Slug: fmt.Sprintf("aux-repair-tag-%d", auxRepairSeedSeq), Status: status}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// auxRepairGet issues a GET and decodes the response envelope.
func auxRepairGet(r *gin.Engine, url string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

// auxRepairPost issues a POST with a raw body (so malformed JSON can be sent
// verbatim) and decodes the response envelope.
func auxRepairPost(r *gin.Engine, url, rawBody string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(rawBody)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

// auxRepairVersion reads the live opaque version token from the context
// endpoint (the client never constructs it).
func auxRepairVersion(t *testing.T, r *gin.Engine, labelID uint) string {
	t.Helper()
	w, body := auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", labelID))
	require.Equal(t, http.StatusOK, w.Code)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	label, ok := data["auxiliary_label"].(map[string]any)
	require.True(t, ok)
	version, ok := label["version"].(string)
	require.True(t, ok)
	require.NotEmpty(t, version)
	return version
}

// auxRepairBody builds a create request body.
func auxRepairBody(description, version, key string) string {
	payload, _ := json.Marshal(map[string]string{
		"description":      description,
		"expected_version": version,
		"idempotency_key":  key,
	})
	return string(payload)
}

// ── GET /description-repair-context ──────────────────────────────────────────

// TestAuxiliaryDescriptionRepairContextEndpoint covers the read side: the
// payload shape, the pure-read guarantee, and the not-found / bad-request
// branches.
func TestAuxiliaryDescriptionRepairContextEndpoint(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "上海", "auxiliary", "active", "上海知名商业综合体")
	board := seedAuxRepairLabel(t, db, "中国地理", "board", "active", "")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{
		BoardID: board.ID, AuxiliaryLabelID: aux.ID,
	}).Error)
	disabledBoard := seedAuxRepairLabel(t, db, "停用引用版块", "board", "disabled", "")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{
		BoardID: disabledBoard.ID, AuxiliaryLabelID: aux.ID,
	}).Error)
	seedAuxRepairTag(t, db, "活跃标签", "active")
	seedAuxRepairTag(t, db, "未归属标签", "active")
	seedAuxRepairTag(t, db, "已合并标签", "merged")

	w, body := auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", aux.ID))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)

	// auxiliary_label: id / label / description / status / version — and no
	// vector columns.
	label, ok := data["auxiliary_label"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(aux.ID), label["id"])
	require.Equal(t, "上海", label["label"])
	require.Equal(t, "上海知名商业综合体", label["description"])
	require.Equal(t, "active", label["status"])
	require.NotEmpty(t, label["version"])
	require.NotContains(t, label, "embedding")
	require.NotContains(t, label, "merge_embedding")

	// direct_boards: the referencing boards incl. the disabled one.
	boards, ok := data["direct_boards"].([]any)
	require.True(t, ok)
	require.Len(t, boards, 2)
	first := boards[0].(map[string]any)
	require.Equal(t, float64(board.ID), first["id"])
	require.Equal(t, "中国地理", first["label"])
	require.Equal(t, "active", first["status"])
	second := boards[1].(map[string]any)
	require.Equal(t, "disabled", second["status"], "disabled boards stay visible with their status")

	// active_tag_count: the active population only (merged excluded).
	require.Equal(t, float64(2), data["active_tag_count"])
	require.Nil(t, data["latest_task"], "no task yet — latest_task is null")

	// Pure read: no task was created.
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(0), jobs)

	// A disabled auxiliary label stays readable (not executable).
	disabled := seedAuxRepairLabel(t, db, "停用辅助标签", "auxiliary", "disabled", "旧描述")
	w, body = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", disabled.ID))
	require.Equal(t, http.StatusOK, w.Code)
	disabledLabel := body["data"].(map[string]any)["auxiliary_label"].(map[string]any)
	require.Equal(t, "disabled", disabledLabel["status"])

	// A board id is not an auxiliary label → 404.
	w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", board.ID))
	require.Equal(t, http.StatusNotFound, w.Code)

	// Unknown id → 404.
	w, _ = auxRepairGet(r, "/api/auxiliary-labels/999999/description-repair-context")
	require.Equal(t, http.StatusNotFound, w.Code)

	// Malformed ids → 400 (never a DB error).
	for _, raw := range []string{"abc", "0", "-1", "1.5"} {
		w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repair-context", raw))
		require.Equal(t, http.StatusBadRequest, w.Code, "id=%s must be rejected", raw)
	}
}

// ── POST /description-repairs ────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairCreateEndpoint covers the create side: the
// 202 payload, the zero side effects on the label, the idempotent replay, the
// conflicts and the input boundaries.
func TestAuxiliaryDescriptionRepairCreateEndpoint(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "上海", "auxiliary", "active", "上海知名商业综合体")
	tag := seedAuxRepairTag(t, db, "活跃标签", "active")
	version := auxRepairVersion(t, r, aux.ID)
	url := fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID)

	// 202 + the persisted pending job.
	w, body := auxRepairPost(r, url, auxRepairBody("中国东部沿海的直辖市", version, "create-1"))
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, true, body["success"])
	job, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(aux.ID), job["auxiliary_label_id"])
	require.Equal(t, "pending", job["status"])
	require.Equal(t, "", job["stage"])
	require.Equal(t, false, job["description_committed"])
	require.Equal(t, "上海知名商业综合体", job["old_description"])
	require.Equal(t, "中国东部沿海的直辖市", job["new_description"])
	require.Equal(t, float64(1), job["total"], "the frozen scope is every active tag")
	require.Equal(t, float64(0), job["processed"])
	require.Equal(t, float64(0), job["failed"])
	jobID := job["id"].(float64)

	// The label itself is untouched: description, both vectors, and the match
	// table stay as they were (this seam only records the request).
	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", aux.ID).First(&reloaded).Error)
	require.Equal(t, "上海知名商业综合体", reloaded.Description)
	require.Nil(t, reloaded.Embedding)
	require.Nil(t, reloaded.MergeEmbedding)
	var matches int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Count(&matches).Error)
	require.Equal(t, int64(0), matches)
	var items []tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ?", uint(jobID)).Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, tag.ID, items[0].TopicTagID, "the frozen scope holds the seeded active tag")
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairItemPending, items[0].Status)

	// Same key + same payload → the SAME job (double click / timeout retry).
	w, body = auxRepairPost(r, url, auxRepairBody("中国东部沿海的直辖市", version, "create-1"))
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, jobID, body["data"].(map[string]any)["id"], "the replay must return the stored job")
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs, "a replay must not create a second task")

	// Same key + different payload → 409 (never a silent replay).
	w, _ = auxRepairPost(r, url, auxRepairBody("改了主意的描述", version, "create-1"))
	require.Equal(t, http.StatusConflict, w.Code)

	// Another key while an unfinished task exists → 409 (per-label mutex).
	w, _ = auxRepairPost(r, url, auxRepairBody("另一个描述", version, "create-2"))
	require.Equal(t, http.StatusConflict, w.Code)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)

	// A disabled label → 409 (readable, not executable).
	disabled := seedAuxRepairLabel(t, db, "停用辅助标签", "auxiliary", "disabled", "旧描述")
	disabledVersion := auxRepairVersion(t, r, disabled.ID)
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", disabled.ID),
		auxRepairBody("新描述", disabledVersion, "disabled-1"))
	require.Equal(t, http.StatusConflict, w.Code)

	// Unknown / non-auxiliary label → 404.
	w, _ = auxRepairPost(r, "/api/auxiliary-labels/999999/description-repairs",
		auxRepairBody("新描述", version, "missing-1"))
	require.Equal(t, http.StatusNotFound, w.Code)

	// Malformed bodies → 400.
	for _, raw := range []string{"", "not json", `{"description":`, `{"description": 123}`} {
		w, _ = auxRepairPost(r, url, raw)
		require.Equal(t, http.StatusBadRequest, w.Code, "body=%q must be rejected", raw)
	}

	// Missing / oversized idempotency key → 400 (the varchar(64) column must
	// never surface as a DB error → 500).
	w, _ = auxRepairPost(r, url, auxRepairBody("新描述", version, ""))
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = auxRepairPost(r, url, auxRepairBody("新描述", version, strings.Repeat("k", 65)))
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Missing / oversized expected_version → 400.
	w, _ = auxRepairPost(r, url, auxRepairBody("新描述", "", "version-missing"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = auxRepairPost(r, url, auxRepairBody("新描述", strings.Repeat("v", 65), "version-too-long"))
	require.Equal(t, http.StatusBadRequest, w.Code)

	// A stale version → 409 (reload and confirm again).
	w, _ = auxRepairPost(r, url, auxRepairBody("新描述", "v1_stale_token", "version-stale"))
	require.Equal(t, http.StatusConflict, w.Code)

	// The three description rules → 400 (empty / too long / repeats the label).
	w, _ = auxRepairPost(r, url, auxRepairBody("   ", version, "rule-empty"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = auxRepairPost(r, url, auxRepairBody(strings.Repeat("字", 501), version, "rule-too-long"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = auxRepairPost(r, url, auxRepairBody("上海", version, "rule-repeats-label"))
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 500 characters are allowed — but the label still holds the unfinished
	// task from above, so the mutex answers 409 (not 400).
	w, _ = auxRepairPost(r, url, auxRepairBody(strings.Repeat("字", 500), version, "rule-500"))
	require.Equal(t, http.StatusConflict, w.Code)

	// Not one rejected request created a task.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
}

// TestAuxiliaryDescriptionRepairCreateAccepts499And500Runes checks the
// accepted end of the length rule on labels without an unfinished task: 499
// and 500 characters are accepted (202) and the stored text is the trimmed
// confirmation (the repo-level rule test covers 501 → rejected).
func TestAuxiliaryDescriptionRepairCreateAccepts499And500Runes(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	for _, size := range []int{499, 500} {
		aux := seedAuxRepairLabel(t, db, fmt.Sprintf("长描述标签%d", size), "auxiliary", "active", "旧描述")
		version := auxRepairVersion(t, r, aux.ID)
		text := strings.Repeat("字", size)

		w, body := auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID),
			auxRepairBody(text, version, fmt.Sprintf("len-%d", size)))
		require.Equal(t, http.StatusAccepted, w.Code, "a %d-character description must be accepted", size)
		require.Equal(t, text, body["data"].(map[string]any)["new_description"])
	}
}

// ── GET /description-repairs/:job_id ─────────────────────────────────────────

// TestAuxiliaryDescriptionRepairJobEndpoint covers the job read: the payload,
// the ownership check, the missing job and the historical job of a deleted
// label.
func TestAuxiliaryDescriptionRepairJobEndpoint(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "上海", "auxiliary", "active", "旧描述")
	other := seedAuxRepairLabel(t, db, "别的标签", "auxiliary", "active", "旧描述")
	version := auxRepairVersion(t, r, aux.ID)

	w, body := auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID),
		auxRepairBody("新的共享描述", version, "job-read-1"))
	require.Equal(t, http.StatusAccepted, w.Code)
	jobID := int(body["data"].(map[string]any)["id"].(float64))

	// The job of its own label.
	w, body = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d", aux.ID, jobID))
	require.Equal(t, http.StatusOK, w.Code)
	job, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(jobID), job["id"])
	require.Equal(t, float64(aux.ID), job["auxiliary_label_id"])
	require.Equal(t, "pending", job["status"])
	require.Equal(t, "新的共享描述", job["new_description"])
	require.NotContains(t, job, "claim_owner", "the private claim columns must not leak")
	require.NotContains(t, job, "lease_expires_at")

	// The same job id under another label → 404 (ownership check).
	w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d", other.ID, jobID))
	require.Equal(t, http.StatusNotFound, w.Code)

	// Unknown job → 404.
	w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/999999", aux.ID))
	require.Equal(t, http.StatusNotFound, w.Code)

	// Malformed ids → 400.
	for _, raw := range []string{"abc", "0", "-1"} {
		w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%s", aux.ID, raw))
		require.Equal(t, http.StatusBadRequest, w.Code, "job_id=%s must be rejected", raw)
	}

	// A deleted source label keeps its historical job readable.
	require.NoError(t, db.Delete(&models.SemanticLabel{}, aux.ID).Error)
	w, body = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d", aux.ID, jobID))
	require.Equal(t, http.StatusOK, w.Code, "the history of a deleted label stays queryable")
	require.Equal(t, float64(jobID), body["data"].(map[string]any)["id"])
}

// TestAuxiliaryDescriptionRepairCreateRejectsTrailingJSON covers the B2
// review finding (Medium): a body with a SECOND JSON object or trailing
// garbage must be rejected — Gin's ShouldBindJSON decodes once and ignores
// the rest of the stream, so such a body used to create a repair task.
func TestAuxiliaryDescriptionRepairCreateRejectsTrailingJSON(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "尾随JSON", "auxiliary", "active", "旧描述")
	version := auxRepairVersion(t, r, aux.ID)
	url := fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID)
	valid := auxRepairBody("新的共享描述", version, "trailing-1")

	cases := []struct {
		name string
		body string
	}{
		{name: "second JSON object", body: valid + `{"description":"另一个对象"}`},
		{name: "trailing garbage", body: valid + ` garbage`},
		{name: "trailing JSON fragment", body: valid + `}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := auxRepairPost(r, url, tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code, "the trailing payload must be rejected")

			// No task and no scope row may have been created.
			var jobs, items int64
			require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
			require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).Count(&items).Error)
			require.Equal(t, int64(0), jobs, "a rejected body must not create a task")
			require.Equal(t, int64(0), items, "a rejected body must not create scope rows")
		})
	}

	// Trailing WHITESPACE stays legal (one object, nothing else).
	w, body := auxRepairPost(r, url, valid+"  \n\t ")
	require.Equal(t, http.StatusAccepted, w.Code, "trailing whitespace is still one object")
	require.NotEmpty(t, body["data"])
}

// TestAuxiliaryDescriptionRepairRejectsOutOfRangeIDs covers the B2 review
// finding (Low): the label / job id columns are PostgreSQL signed bigints, so
// an id above MaxInt64 must be a bad request (400) instead of reaching the
// driver (which would answer 500). MaxInt64 itself is a legal — but
// nonexistent — id and must answer 404.
func TestAuxiliaryDescriptionRepairRejectsOutOfRangeIDs(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "越界ID", "auxiliary", "active", "旧描述")
	version := auxRepairVersion(t, r, aux.ID)
	body := auxRepairBody("新的共享描述", version, "id-range-1")

	const (
		maxInt64     = "9223372036854775807"
		maxInt64Plus = "9223372036854775808"
		maxUint64    = "18446744073709551615"
	)

	// The endpoints must answer 404 (valid range, unknown id) — never 500.
	for _, raw := range []string{maxInt64} {
		w, _ := auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repair-context", raw))
		require.Equal(t, http.StatusNotFound, w.Code, "id=%s must be a not-found", raw)
		w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repairs", raw), body)
		require.Equal(t, http.StatusNotFound, w.Code, "id=%s must be a not-found", raw)
		w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repairs/1", raw))
		require.Equal(t, http.StatusNotFound, w.Code, "id=%s must be a not-found", raw)
	}

	// Above the signed bigint range → 400 on every id position.
	for _, raw := range []string{maxInt64Plus, maxUint64} {
		w, _ := auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repair-context", raw))
		require.Equal(t, http.StatusBadRequest, w.Code, "id=%s must be rejected", raw)
		w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repairs", raw), body)
		require.Equal(t, http.StatusBadRequest, w.Code, "id=%s must be rejected", raw)
		w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%s/description-repairs/1", raw))
		require.Equal(t, http.StatusBadRequest, w.Code, "id=%s must be rejected", raw)
		w, _ = auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%s", aux.ID, raw))
		require.Equal(t, http.StatusBadRequest, w.Code, "job_id=%s must be rejected", raw)
	}

	// None of the rejected requests created a task.
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(0), jobs)
}

// ── unknown (non-client) errors ─────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairUnknownErrorStaysInternal checks the last
// mapping branch: a storage failure that is not a client error answers 500
// with a generic message — the raw DB/connection detail stays in the log.
func TestAuxiliaryDescriptionRepairUnknownErrorStaysInternal(t *testing.T) {
	// A dialector that cannot reach any server (nothing listens on port 1 and
	// the automatic ping is disabled, so no connection is even attempted at
	// open time). No database is contacted by this test.
	broken, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 port=1 user=nobody dbname=none sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	svc, err := service.NewAuxRepairService(broken)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterAuxiliaryDescriptionRepairRoutes(api, svc)

	w, body := auxRepairGet(r, "/api/auxiliary-labels/1/description-repair-context")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Equal(t, false, body["success"])
	message, _ := body["error"].(string)
	require.Equal(t, "internal server error", message)
	require.NotContains(t, message, "127.0.0.1", "the raw DB detail must not leak to the client")
}

// ── B-P2-1 at the HTTP level ────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete covers review
// B-P2-1 through the HTTP surface: after the source label is hard deleted, the
// stored request still replays (same job), a different payload under the same
// key conflicts, and a new key is a plain 404.
func TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	aux := seedAuxRepairLabel(t, db, "硬删标签", "auxiliary", "active", "旧描述")
	version := auxRepairVersion(t, r, aux.ID)
	url := fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID)

	w, body := auxRepairPost(r, url, auxRepairBody("硬删前确认的描述", version, "hard-delete-1"))
	require.Equal(t, http.StatusAccepted, w.Code)
	jobID := body["data"].(map[string]any)["id"]

	require.NoError(t, db.Delete(&models.SemanticLabel{}, aux.ID).Error)

	// Same key + same payload → the stored job.
	w, body = auxRepairPost(r, url, auxRepairBody("硬删前确认的描述", version, "hard-delete-1"))
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, jobID, body["data"].(map[string]any)["id"])

	// Same key + different payload → 409.
	w, _ = auxRepairPost(r, url, auxRepairBody("改了主意的描述", version, "hard-delete-1"))
	require.Equal(t, http.StatusConflict, w.Code)

	// A new key → 404 (the label is gone).
	w, _ = auxRepairPost(r, url, auxRepairBody("删除后的新描述", version, "hard-delete-2"))
	require.Equal(t, http.StatusNotFound, w.Code)

	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
}

// ── the retry resource (task 2.5) ───────────────────────────────────────────

// TestAuxiliaryDescriptionRepairRetryEndpoint covers the fourth HTTP
// resource: POST .../description-repairs/:job_id/retry.
//
// The endpoint takes NO parameters: an empty body or {} is accepted, a
// malformed body / a second object / trailing garbage / any parameter is a
// bad request (the user must not believe a parameter was applied).
func TestAuxiliaryDescriptionRepairRetryEndpoint(t *testing.T) {
	db, r := setupAuxRepairHandler(t)

	// Each scenario needs its OWN auxiliary label (the per-label creation
	// mutex allows only one unfinished task per label).
	failedLabel := seedAuxRepairLabel(t, db, "失败标签", "auxiliary", "active", "旧描述")
	runningLabel := seedAuxRepairLabel(t, db, "运行中标签", "auxiliary", "active", "旧描述")
	completedLabel := seedAuxRepairLabel(t, db, "完成标签", "auxiliary", "active", "旧描述")
	conflictLabel := seedAuxRepairLabel(t, db, "冲突标签", "auxiliary", "active", "旧描述")
	unavailableLabel := seedAuxRepairLabel(t, db, "不可用标签", "auxiliary", "active", "旧描述")
	otherLabel := seedAuxRepairLabel(t, db, "别的标签", "auxiliary", "active", "旧描述")

	newJob := func(labelID uint, key, description string) uint {
		t.Helper()
		_, body := auxRepairGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", labelID))
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		version := data["auxiliary_label"].(map[string]any)["version"].(string)
		w, body := auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", labelID),
			auxRepairBody(description, version, key))
		require.Equal(t, http.StatusAccepted, w.Code)
		return uint(body["data"].(map[string]any)["id"].(float64))
	}

	// A FAILED task → the SAME task is re-queued (202).
	failedJob := newJob(failedLabel.ID, "retry-failed", "新的描述")
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", failedJob).
		Update("status", tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed).Error)
	w, body := auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", failedLabel.ID, failedJob), "")
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, float64(failedJob), data["id"], "the retry re-uses the same task")
	require.Equal(t, "pending", data["status"], "the failed task is re-queued for another attempt")
	require.Empty(t, data["error"], "the old failure message must be cleared")

	// A RUNNING task → 202 no-op (the task is not even touched).
	runningJob := newJob(runningLabel.ID, "retry-running", "运行中的描述")
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", runningJob).
		Updates(map[string]any{
			"status":           tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
			"claim_owner":      "executor-1",
			"lease_expires_at": time.Now().Add(time.Minute),
		}).Error)
	var beforeRunning tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", runningJob).First(&beforeRunning).Error)
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", runningLabel.ID, runningJob), "{}")
	require.Equal(t, http.StatusAccepted, w.Code)
	var afterRunning tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", runningJob).First(&afterRunning).Error)
	require.Equal(t, beforeRunning.UpdatedAt.UTC(), afterRunning.UpdatedAt.UTC(),
		"a running task must not even be touched by the retry")
	require.Equal(t, beforeRunning.Status, afterRunning.Status)

	// A COMPLETED task → 202 no-op.
	completedJob := newJob(completedLabel.ID, "retry-completed", "完成的描述")
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", completedJob).
		Update("status", tagmodels.AuxiliaryDescriptionRepairStatusCompleted).Error)
	var beforeCompleted tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", completedJob).First(&beforeCompleted).Error)
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", completedLabel.ID, completedJob), "")
	require.Equal(t, http.StatusAccepted, w.Code)
	var afterCompleted tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", completedJob).First(&afterCompleted).Error)
	require.Equal(t, beforeCompleted.UpdatedAt.UTC(), afterCompleted.UpdatedAt.UTC(),
		"a completed task must not even be touched by the retry")

	// A TERMINAL task (conflict / unavailable) → 409 (re-check the version).
	for _, tc := range []struct {
		status string
		label  models.SemanticLabel
	}{
		{tagmodels.AuxiliaryDescriptionRepairStatusConflict, conflictLabel},
		{tagmodels.AuxiliaryDescriptionRepairStatusUnavailable, unavailableLabel},
	} {
		terminalJob := newJob(tc.label.ID, "retry-"+tc.status, "终态的描述")
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", terminalJob).
			Update("status", tc.status).Error)
		w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", tc.label.ID, terminalJob), "")
		require.Equal(t, http.StatusConflict, w.Code, "status=%s must not be retryable", tc.status)
	}

	// An unknown task → 404; a task of ANOTHER label → 404.
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/999999/retry", failedLabel.ID), "")
	require.Equal(t, http.StatusNotFound, w.Code)
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", otherLabel.ID, failedJob), "")
	require.Equal(t, http.StatusNotFound, w.Code, "a task of another label must not be retryable")

	// Malformed bodies and any parameter → 400.
	bodyLabel := seedAuxRepairLabel(t, db, "坏请求标签", "auxiliary", "active", "旧描述")
	bodyJob := newJob(bodyLabel.ID, "retry-bad-body", "坏请求的描述")
	retryURL := fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", bodyLabel.ID, bodyJob)
	for _, raw := range []string{
		"not json",
		`{"description":"偷偷改描述"}`,
		`{} {"description":"第二个对象"}`,
		`{} garbage`,
		`null`, // a JSON null is NOT an empty body (the contract: empty body or {})
	} {
		w, _ = auxRepairPost(r, retryURL, raw)
		require.Equal(t, http.StatusBadRequest, w.Code, "body=%q must be rejected", raw)
	}
	// The rejected bodies changed nothing.
	var untouched tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", bodyJob).First(&untouched).Error)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, untouched.Status)

	// An empty body and an empty object are accepted.
	w, _ = auxRepairPost(r, retryURL, "")
	require.Equal(t, http.StatusAccepted, w.Code)
	w, _ = auxRepairPost(r, retryURL, "{}")
	require.Equal(t, http.StatusAccepted, w.Code)

	// Out-of-range ids → 400 (never 500).
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/9223372036854775808/retry", bodyLabel.ID), "")
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = auxRepairPost(r, fmt.Sprintf("/api/auxiliary-labels/18446744073709551615/description-repairs/%d/retry", bodyJob), "")
	require.Equal(t, http.StatusBadRequest, w.Code)
}
