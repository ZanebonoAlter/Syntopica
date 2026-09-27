package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Side-effect: register TopicLaneSnapshot (+ the rest of the topicgraph
	// models) so RunAutoMigrate creates the table — AutoMigrate runs with
	// DisableForeignKeyConstraintWhenMigrating=true, so it creates NO FK; the
	// versioned migration under test is the source of truth for the FK.
	_ "syntopica-backend/internal/topicgraph/repository"
)

// TestLaneSnapshotFKMigration exercises migration 20260910_0001 end-to-end
// against a testcontainer PG in the production first-run scenario
// (overview-lane-dynamics tasks 1.1):
//  1. AutoMigrate creates topic_lane_snapshots WITHOUT any FK (an orphan
//     snapshot row can exist — the insert succeeds precisely because the FK
//     is absent);
//  2. the migration deletes the orphan BEFORE adding the FK (else ADD
//     CONSTRAINT would validate existing rows and fail);
//  3. the new FK is ON DELETE CASCADE — hard-deleting a topic removes its
//     snapshot row.
//
// Docker required. Skipped under -short.
func TestLaneSnapshotFKMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	// Hermetic setup against the shared process-singleton container: drop the
	// FK if a prior run left it, clear probe rows.
	require.NoError(t, db.Exec(`ALTER TABLE topic_lane_snapshots DROP CONSTRAINT IF EXISTS fk_topic_lane_snapshots_topic`).Error)
	require.NoError(t, db.Exec(`DELETE FROM topic_lane_snapshots WHERE persistent_topic_id IN (9001, 99999)`).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_persistent_topics WHERE id = 9001`).Error)

	// Precondition: AutoMigrate must not have created the FK.
	var fkExists bool
	require.NoError(t, db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name='fk_topic_lane_snapshots_topic' AND table_name='topic_lane_snapshots')`).Scan(&fkExists).Error)
	require.False(t, fkExists, "precondition: AutoMigrate must not create the FK (migrations are the source of truth)")

	// Seed pre-migration state: 1 topic (9001) + 1 valid snapshot + 1 orphan
	// snapshot (topic 99999 references nothing). The orphan insert only
	// succeeds because the FK is absent.
	require.NoError(t, db.Exec(`INSERT INTO board_persistent_topics
		(id, semantic_board_id, label, embedding, status, source, first_seen_date, last_seen_date, hit_count, consecutive_hits, created_at, updated_at)
		VALUES (9001, 1, 'lane-snapshot-fk-test', '[0]', 'active', 'auto', CURRENT_DATE, CURRENT_DATE, 1, 0, now(), now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO topic_lane_snapshots (persistent_topic_id, rolling_summary, as_of_date, created_at, updated_at)
		VALUES (9001, 'valid snapshot', CURRENT_DATE, now(), now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO topic_lane_snapshots (persistent_topic_id, rolling_summary, as_of_date, created_at, updated_at)
		VALUES (99999, 'orphan snapshot', CURRENT_DATE, now(), now())`).Error)

	// Locate migration 20260910_0001's Up closure and run it in-tx (mirrors
	// the production default in-transaction path).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260910_0001" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20260910_0001 must exist in the production list")
	require.NoError(t, db.Transaction(up))

	// The orphan is gone; the valid row survived.
	var orphans int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM topic_lane_snapshots WHERE persistent_topic_id = 99999`).Scan(&orphans).Error)
	require.Zero(t, orphans, "migration must delete orphan snapshots before adding the FK")
	var valid int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM topic_lane_snapshots WHERE persistent_topic_id = 9001`).Scan(&valid).Error)
	require.EqualValues(t, 1, valid)

	// The FK now exists.
	require.NoError(t, db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name='fk_topic_lane_snapshots_topic' AND table_name='topic_lane_snapshots')`).Scan(&fkExists).Error)
	require.True(t, fkExists, "migration must add fk_topic_lane_snapshots_topic")

	// ON DELETE CASCADE: deleting the topic removes its snapshot.
	require.NoError(t, db.Exec(`DELETE FROM board_persistent_topics WHERE id = 9001`).Error)
	var remaining int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM topic_lane_snapshots WHERE persistent_topic_id = 9001`).Scan(&remaining).Error)
	require.Zero(t, remaining, "FK must be ON DELETE CASCADE")

	// Idempotency: a second run is a no-op.
	require.NoError(t, db.Transaction(up))
}
