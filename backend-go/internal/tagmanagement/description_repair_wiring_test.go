package tagging

// board-health-triage-advice seam B6 (task 2.6) — the PRODUCTION wiring of the
// description-repair chain, exercised end to end against a throwaway
// testcontainer Postgres (never the production library: the chain rewrites the
// board matches of EVERY active tag of its database).
//
// Covered: the domain lifecycle (one service instance shared by the routes and
// the worker), the four registered resources, and the complete backend story
//   GET context (zero writes) → POST create (202) → the background executor
//   runs embedding + atomic save + matching → GET the task (completed, real
//   counters) → a partial matching failure → POST retry → the queue picks it
//   up and continues ONLY the failed items (no new embedding) → a repeated
//   retry of a finished task is a complete no-op → the pre-existing auxiliary
//   label routes are still reachable.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/board"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// wiringStubEmbedder is the stub embedding unit of the wiring tests: it never
// calls a provider, it counts the calls and returns a valid vector.
type wiringStubEmbedder struct {
	mu    sync.Mutex
	calls int
}

func (s *wiringStubEmbedder) embed(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{0, 0, 1}, testutil.TestEmbeddingDim))
	return vec, nil, nil
}

func (s *wiringStubEmbedder) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// wiringFailMatcher fails ONCE for one topic tag (the partial-failure case of
// the matching phase) and succeeds afterwards — so the retry story needs no
// manual intervention between the retry and the automatic continuation.
type wiringFailMatcher struct {
	mu        sync.Mutex
	failTagID uint
	failed    map[uint]bool
	calls     []uint
}

func (m *wiringFailMatcher) MatchTopicTag(ctx context.Context, topicTagID uint) ([]board.SemanticBoardMatchResult, error) {
	m.mu.Lock()
	if m.failed == nil {
		m.failed = map[uint]bool{}
	}
	m.calls = append(m.calls, topicTagID)
	first := !m.failed[topicTagID]
	m.failed[topicTagID] = true
	m.mu.Unlock()
	if topicTagID == m.failTagID && first {
		return nil, errors.New("matcher exploded")
	}
	return nil, nil
}

func (m *wiringFailMatcher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// wiringSetup wires the domain lifecycle over the testcontainer and registers
// the domain routes — the SAME path the app uses (app → domain → handler).
func wiringSetup(t *testing.T, matcher *wiringFailMatcher) (*gorm.DB, *gin.Engine, *wiringStubEmbedder) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	stub := &wiringStubEmbedder{}
	opts := []service.AuxRepairOption{
		service.WithAuxRepairEmbedder(stub.embed),
	}
	if matcher != nil {
		opts = append(opts, service.WithAuxRepairMatcher(matcher))
	}
	svc, err := service.NewAuxRepairService(db, opts...)
	require.NoError(t, err)
	exec, err := service.NewAuxRepairExecutor(svc, service.AuxRepairExecutorConfig{
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     300 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  200 * time.Millisecond,
		MaxBatch:          1,
	})
	require.NoError(t, err)
	InstallDescriptionRepairChain(svc, exec)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterRoutes(api)
	return db, r, stub
}

func wiringGet(r *gin.Engine, url string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func wiringPost(r *gin.Engine, url, raw string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func wiringSeedLabel(t *testing.T, db *gorm.DB, label, status, description string) models.SemanticLabel {
	t.Helper()
	l := models.SemanticLabel{
		Label:       label,
		Slug:        fmt.Sprintf("wiring-%s-%d", label, time.Now().UnixNano()),
		LabelType:   "auxiliary",
		Status:      status,
		Description: description,
	}
	require.NoError(t, db.Create(&l).Error)
	return l
}

func wiringSeedTag(t *testing.T, db *gorm.DB, label string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{
		Label:    label,
		Slug:     fmt.Sprintf("wiring-tag-%d", time.Now().UnixNano()),
		Category: "event",
		Status:   "active",
	}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func wiringWaitFor(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", message)
}

// TestDescriptionRepairWiringFullBackendStory: the complete backend story
// over the really registered routes and the really started worker.
func TestDescriptionRepairWiringFullBackendStory(t *testing.T) {
	db, r, stub := wiringSetup(t, nil)
	aux := wiringSeedLabel(t, db, "上海", "active", "上海知名商业综合体")
	tag := wiringSeedTag(t, db, "直辖市新闻")

	// ① GET context — pure read, no task created.
	w, body := wiringGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", aux.ID))
	require.Equal(t, http.StatusOK, w.Code)
	data := body["data"].(map[string]any)
	label := data["auxiliary_label"].(map[string]any)
	require.Equal(t, "上海", label["label"])
	require.NotEmpty(t, label["version"])
	require.Nil(t, data["latest_task"])
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(0), jobs, "the context read must not create a task")

	// ② POST create — 202, the task is queued in the database.
	createBody := fmt.Sprintf(`{"description":"中国东部沿海的直辖市","expected_version":%q,"idempotency_key":"wiring-create-1"}`, label["version"])
	w, body = wiringPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID), createBody)
	require.Equal(t, http.StatusAccepted, w.Code)
	job := body["data"].(map[string]any)
	jobID := uint(job["id"].(float64))
	require.Equal(t, "pending", job["status"])

	// ③ The background worker (started on the APPLICATION context) runs the
	// whole chain: embedding → atomic save → matching.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	require.NoError(t, StartDescriptionRepairExecutor(appCtx))
	defer StopDescriptionRepairExecutor()

	wiringWaitFor(t, 30*time.Second, func() bool {
		var current tagmodels.AuxiliaryDescriptionRepairJob
		if err := db.Where("id = ?", jobID).First(&current).Error; err != nil {
			return false
		}
		return current.Status == tagmodels.AuxiliaryDescriptionRepairStatusCompleted
	}, "the worker to finish the task")

	// ④ GET the task — completed with the real counters.
	w, body = wiringGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d", aux.ID, jobID))
	require.Equal(t, http.StatusOK, w.Code)
	job = body["data"].(map[string]any)
	require.Equal(t, "completed", job["status"])
	require.Equal(t, "matching", job["stage"])
	require.Equal(t, true, job["description_committed"])
	require.Equal(t, float64(1), job["total"])
	require.Equal(t, float64(1), job["processed"])
	require.Equal(t, float64(0), job["failed"])
	require.Equal(t, float64(0), job["not_applicable"])
	require.Equal(t, float64(0), job["remaining"])
	require.NotContains(t, job, "claim_owner", "the owner must not leak into the wire")
	require.NotContains(t, job, "phase_token", "the phase token must not leak into the wire")
	require.Equal(t, 1, stub.callCount(), "the worker embedded exactly once")

	// The label really got the new description (the atomic save happened).
	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", aux.ID).First(&reloaded).Error)
	require.Equal(t, "中国东部沿海的直辖市", reloaded.Description)
	require.NotNil(t, reloaded.Embedding)
	_ = tag
}

// TestDescriptionRepairWiringRetryContinuesOnlyFailedItems: a partial
// matching failure → POST retry → the queue picks the SAME task up again and
// continues ONLY the failed item — the embedding count does not grow.
func TestDescriptionRepairWiringRetryContinuesOnlyFailedItems(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	// The first tag fails to match, the second one succeeds.
	broken := wiringSeedTag(t, db, "会失败的标签")
	wiringSeedTag(t, db, "会成功的标签")

	matcher := &wiringFailMatcher{failTagID: broken.ID}
	stub := &wiringStubEmbedder{}
	svc, err := service.NewAuxRepairService(db,
		service.WithAuxRepairEmbedder(stub.embed),
		service.WithAuxRepairMatcher(matcher),
	)
	require.NoError(t, err)
	exec, err := service.NewAuxRepairExecutor(svc, service.AuxRepairExecutorConfig{
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     300 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  200 * time.Millisecond,
		MaxBatch:          1,
	})
	require.NoError(t, err)
	InstallDescriptionRepairChain(svc, exec)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterRoutes(api)

	aux := wiringSeedLabel(t, db, "北京", "active", "旧描述")
	_, body := wiringGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", aux.ID))
	version := body["data"].(map[string]any)["auxiliary_label"].(map[string]any)["version"].(string)

	createBody := fmt.Sprintf(`{"description":"中国的首都","expected_version":%q,"idempotency_key":"wiring-retry-1"}`, version)
	w, body := wiringPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID), createBody)
	require.Equal(t, http.StatusAccepted, w.Code)
	jobID := uint(body["data"].(map[string]any)["id"].(float64))

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	require.NoError(t, StartDescriptionRepairExecutor(appCtx))
	defer StopDescriptionRepairExecutor()

	// The task ends as matching_failed (the broken tag stays failed).
	wiringWaitFor(t, 30*time.Second, func() bool {
		var current tagmodels.AuxiliaryDescriptionRepairJob
		if err := db.Where("id = ?", jobID).First(&current).Error; err != nil {
			return false
		}
		return current.Status == tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed
	}, "the task to report the partial matching failure")

	var failed tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", jobID).First(&failed).Error)
	require.Equal(t, 1, failed.Failed)
	require.Equal(t, 1, failed.Processed)
	require.Equal(t, 1, stub.callCount(), "the embedding must not run again for the retry")

	// The explicit retry re-queues the SAME task.
	w, body = wiringPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", aux.ID, jobID), "")
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, float64(jobID), body["data"].(map[string]any)["id"])

	// The broken tag now matches (the matcher fails only ONCE) → the task
	// completes — the queue did the work, nobody touched the task by hand.
	wiringWaitFor(t, 30*time.Second, func() bool {
		var current tagmodels.AuxiliaryDescriptionRepairJob
		if err := db.Where("id = ?", jobID).First(&current).Error; err != nil {
			return false
		}
		return current.Status == tagmodels.AuxiliaryDescriptionRepairStatusCompleted
	}, "the retried task to complete")
	require.Equal(t, 1, stub.callCount(), "the retry must not embed again")
}

// TestDescriptionRepairWiringRetryOnFinishedTaskIsNoop: repeating the retry of
// a finished task changes NOTHING (not even the timestamp).
func TestDescriptionRepairWiringRetryOnFinishedTaskIsNoop(t *testing.T) {
	db, r, _ := wiringSetup(t, nil)
	aux := wiringSeedLabel(t, db, "广州", "active", "旧描述")
	_, body := wiringGet(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repair-context", aux.ID))
	version := body["data"].(map[string]any)["auxiliary_label"].(map[string]any)["version"].(string)

	createBody := fmt.Sprintf(`{"description":"南方的中心城市","expected_version":%q,"idempotency_key":"wiring-noop-1"}`, version)
	w, body := wiringPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs", aux.ID), createBody)
	require.Equal(t, http.StatusAccepted, w.Code)
	jobID := uint(body["data"].(map[string]any)["id"].(float64))

	// Mark the task as completed (the worker is not started in this test).
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"status":                tagmodels.AuxiliaryDescriptionRepairStatusCompleted,
			"stage":                 tagmodels.AuxiliaryDescriptionRepairStageMatching,
			"description_committed": true,
		}).Error)
	var before tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", jobID).First(&before).Error)

	w, body = wiringPost(r, fmt.Sprintf("/api/auxiliary-labels/%d/description-repairs/%d/retry", aux.ID, jobID), "")
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, float64(jobID), body["data"].(map[string]any)["id"])

	var after tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", jobID).First(&after).Error)
	require.Equal(t, before.UpdatedAt.UTC(), after.UpdatedAt.UTC(),
		"a retry of a finished task must not even touch the row")
	require.Equal(t, before.Status, after.Status)
}

// TestDescriptionRepairWiringKeepsExistingAuxRoutes: the pre-existing
// auxiliary-label routes stay reachable — the wiring adds, it never replaces.
func TestDescriptionRepairWiringKeepsExistingAuxRoutes(t *testing.T) {
	_, r, _ := wiringSetup(t, nil)

	w, _ := wiringGet(r, "/api/auxiliary-labels")
	require.Equal(t, http.StatusOK, w.Code, "the auxiliary-label list must still work")

	w, _ = wiringGet(r, "/api/auxiliary-labels/clusters")
	require.Equal(t, http.StatusOK, w.Code, "the auxiliary-label clusters must still work")
}

// TestDescriptionRepairWiringOrderContract (B6 review Low 4): the chain is
// installed AFTER the database is ready and BEFORE the routes are registered,
// so the HTTP surface and the background worker share ONE service instance.
//
// The test pins the contract, not a hot-swap feature: replacing a RUNNING
// chain is not supported (the old router would keep the old service while the
// new worker used the new one — see the ORDERING CONTRACT in wire.go).
func TestDescriptionRepairWiringOrderContract(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	stub := &wiringStubEmbedder{}
	svc, err := service.NewAuxRepairService(db, service.WithAuxRepairEmbedder(stub.embed))
	require.NoError(t, err)
	exec, err := service.NewAuxRepairExecutor(svc, service.AuxRepairExecutorConfig{
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     300 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  200 * time.Millisecond,
		MaxBatch:          1,
	})
	require.NoError(t, err)

	// ① the chain is installed (the DB is ready) …
	InstallDescriptionRepairChain(svc, exec)
	require.Same(t, svc, DescriptionRepairService(), "the domain must serve the installed instance")
	require.False(t, DescriptionRepairExecutorRunning(), "installing must not start the worker")

	// ② … and only then the routes are registered.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterRoutes(api)

	// The repair routes are live and answered by the installed service (the
	// service's error envelope, not Gin's "route not found").
	w, body := wiringGet(r, "/api/auxiliary-labels/999999/description-repair-context")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, false, body["success"])
	require.Equal(t, "auxiliary label not found", body["error"],
		"the repair route must be registered and answered by the installed service")
}
