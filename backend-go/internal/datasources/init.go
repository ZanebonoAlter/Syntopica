package datasources

import (
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// Init seeds the data_sources catalog at startup (called from main.go after
// config load). resolve is the LIVE key resolver (main passes
// wiring.ComtradeKeyResolver: UI-configured key wins over env/config) —
// seeding with a config-only resolver left the status stuck on disabled even
// after the key was configured in the UI (2026-09-23 实战校准). Non-fatal: a
// seed failure logs a warning and the endpoints degrade to an empty catalog
// rather than blocking server start.
func Init(db *gorm.DB, resolve KeyResolver) {
	if err := UpsertCatalog(db, resolve); err != nil {
		logging.Errorf("datasources init: upsert catalog: %v", err)
	}
}
