package database

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureAuxiliaryDescriptionRepairMutexIndex is the idempotent creator of the
// partial unique mutex index on auxiliary_description_repair_jobs
// (board-health-triage-advice task 2.1): one unfinished description-repair
// job per auxiliary label, enforced by the database (spec「修复任务持久且
// 幂等」, including the retryable failed stages, which are NOT terminal).
// It is called from TWO places:
//
//   - migration 20261028_0001's Up (the original creation path), and
//   - RunAutoMigrate, right after a successful model sync (review B-P2-2
//     self-heal): the migration may have run — and recorded its version — on
//     a boot whose AutoMigrate had not yet created the repair tables (its
//     no-op would then never re-run); the post-AutoMigrate call guarantees the
//     index exists whenever the table does, without touching migration
//     history or any other table.
//
// The status literals MUST stay in sync with
// tagmodels.AuxiliaryDescriptionRepairTerminalStatuses. Idempotent
// (CREATE UNIQUE INDEX IF NOT EXISTS); no-ops on non-postgres and when the
// table is absent (the repair tables are registered by the tagmanagement
// root's init; binaries that do not pull that registration build their schema
// without them). Only this change's new-job index is healed — no other
// historical table.
func ensureAuxiliaryDescriptionRepairMutexIndex(db *gorm.DB) error {
	if db.Dialector == nil || db.Name() != "postgres" {
		return nil
	}
	var tableExists bool
	if err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.tables WHERE table_name = 'auxiliary_description_repair_jobs')`).Scan(&tableExists).Error; err != nil {
		return fmt.Errorf("check auxiliary_description_repair_jobs: %w", err)
	}
	if !tableExists {
		return nil
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_aux_desc_repair_jobs_label_active
		ON auxiliary_description_repair_jobs(auxiliary_label_id)
		WHERE status NOT IN ('completed', 'conflict', 'unavailable')`).Error; err != nil {
		return fmt.Errorf("create uq_aux_desc_repair_jobs_label_active: %w", err)
	}
	return nil
}

// auxiliaryDescriptionRepairMigration implements 20261028_0001
// (board-health-triage-advice task 2.1 / seam B1): the partial unique mutex
// index via ensureAuxiliaryDescriptionRepairMutexIndex. AutoMigrate owns the
// two tables (tagmanagement/wire.go RegisterModels) and their full unique
// indexes (uq_aux_desc_repair_jobs_key on idempotency_key;
// uq_aux_desc_repair_items_job_tag on (job_id, topic_tag_id)); the NOT IN
// predicate cannot be expressed as a GORM tag, so it lives here — same split
// as uq_board_upgrade_suggestions_hash (20260717_0001).
//
// No FKs are added to these tables — see the model file
// (tagmanagement/models/auxiliary_description_repair.go) for the deliberate
// deletion-strategy rationale: frozen references must never block or lose
// history on semantic_labels/topic_tags deletion.
func auxiliaryDescriptionRepairMigration() Migration {
	return Migration{
		Version:     "20261028_0001",
		Description: "board-health-triage-advice: partial unique index on auxiliary_description_repair_jobs(auxiliary_label_id) WHERE status NOT IN ('completed','conflict','unavailable') — one unfinished repair job per auxiliary label (tables + full unique indexes owned by AutoMigrate; index re-healed after every successful AutoMigrate per review B-P2-2)",
		Up:          ensureAuxiliaryDescriptionRepairMutexIndex,
	}
}
