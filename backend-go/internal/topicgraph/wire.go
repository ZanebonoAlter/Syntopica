package topicgraph

import (
	"gorm.io/gorm"

	"syntopica-backend/internal/topicgraph/handler"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// Repo re-exports the repository singleton (decouple-backend-domains:
// admin's daily-report job consumed topicgraph/repository.Repo via a deep
// path; cross-domain callers now use topicgraph.Repo).
var Repo = repository.Repo

// InitRepository initializes the topicgraph repository singleton.
func InitRepository(db *gorm.DB) {
	repository.InitRepository(db)
	Repo = repository.Repo
}

// RegisterDailyReportRoutes registers all daily report routes.
var RegisterDailyReportRoutes = handler.RegisterDailyReportRoutes

// Service layer re-exports (used by admin scheduler)
var (
	CollectBoardIDsForDate = service.CollectBoardIDsForDate
	GenerateDailyReport    = service.GenerateDailyReport
	SaveReport             = service.SaveReport
	GenerateAndSaveReport  = service.GenerateAndSaveReport
)

// Daily report rebuild window (offline-catchup D6). Re-exported so the admin
// scheduler shares the exact predicate and wording the HTTP handler uses
// (the handler imports service/ directly — it cannot import this package, which
// imports handler/).
var (
	IsDateOutsideRebuildWindow    = service.IsDateOutsideRebuildWindow
	RebuildWindowRejectionMessage = service.RebuildWindowRejectionMessage
)
