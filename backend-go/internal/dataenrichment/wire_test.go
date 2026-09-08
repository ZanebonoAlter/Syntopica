package dataenrichment

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// TestInitEnablesRegisterRoutes is a regression test for a production startup
// panic: handler.RegisterRoutes dereferences a package-level handler singleton
// and panics with "handler.InitHandler must be called before RegisterRoutes"
// when that singleton is nil.
//
// Root cause that this test guards against: the wiring (InitHandler + the
// service construction) was originally placed inside app.StartRuntime, which
// runs AFTER app.SetupRoutes. SetupRoutes calls dataenrichment.RegisterRoutes,
// so the singleton was still nil at registration time and the server panicked
// on boot — yet every unit test passed because handler tests build a standalone
// handler via NewHandler and never exercise the singleton path.
//
// Contract: dataenrichment.Init MUST wire the handler singleton so that
// RegisterRoutes is safe to call during app.SetupRoutes. Init runs in main.go
// before SetupRoutes, mirroring how the other domains call InitRepository.
func TestInitEnablesRegisterRoutes(t *testing.T) {
	setupDataEnrichmentTestDB(t)
	Init(database.DB) // wires repo + cycle-A/B services + handler singleton

	gin.SetMode(gin.TestMode)
	g := gin.New()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRoutes panicked after Init (regression: handler "+
				"singleton must be wired before route registration, see main.go "+
				"startup order): %v", r)
		}
	}()
	RegisterRoutes(g.Group("/api"))
}

// TestInitWiresBoardConfigResolver is a regression test for a production 500:
// POST /api/semantic-boards/:id/enrichment/analysis/trigger returned
// "enrich board %d: board config resolver not wired" because EnrichBoard's
// config-gate resolver is attached via a post-construction setter
// (SetBoardConfigResolver) that Init never called. Unit tests passed because
// they construct the orchestrator themselves and wire the resolver manually.
//
// Contract: Init MUST wire the board config resolver so EnrichBoard's first
// gate (enrichment_enabled) actually runs. With a seeded board that has
// enrichment disabled, the trigger must return the business error
// "enrichment not enabled for this board" — never "not wired".
func TestInitWiresBoardConfigResolver(t *testing.T) {
	setupDataEnrichmentTestDB(t)

	// Seed a board with enrichment disabled: the gate must reject it with the
	// business error, which proves the resolver was actually consulted.
	board := models.SemanticLabel{
		Label: "wire-test-board", Slug: "wire-test-board",
		LabelType: "board", Status: "active", Source: "manual",
	}
	if err := database.DB.Create(&board).Error; err != nil {
		t.Fatalf("seed board: %v", err)
	}

	Init(database.DB)

	gin.SetMode(gin.TestMode)
	g := gin.New()
	RegisterRoutes(g.Group("/api"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/trigger", board.ID), nil)
	g.ServeHTTP(w, req)

	if strings.Contains(w.Body.String(), "not wired") {
		t.Fatalf("board config resolver not wired after Init (regression: Init "+
			"must call orchestrator.SetBoardConfigResolver): %s", w.Body.String())
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (board-not-enabled business error); body: %s",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "enrichment not enabled") {
		t.Fatalf("expected enrichment-not-enabled business error, got: %s", w.Body.String())
	}
}
