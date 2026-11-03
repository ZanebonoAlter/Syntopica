package repository

// board-health-triage-advice seam B2 (task 2.2) — review fixes B-P2-1 and
// B-P2-3 at the storage layer, against a real testcontainer Postgres
// (repository 层禁 SQLite).
//
// B-P2-1: the idempotency decision is made against the STORED request BEFORE
// any current-state gate, so a retry of an already stored request keeps
// returning its job even after the source auxiliary label was hard deleted
// (the aux-GC delete path). A different payload under the same key is an
// explicit conflict — never a silent replay; a NEW key is a plain not-found
// (the label is gone).
//
// B-P2-3: the idempotency key and the version token are bounded by their
// varchar(64) columns. The bound is checked in Go (a client error), so a DB
// 22001 can never surface as a 500 — and a rejected request writes nothing.

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete covers B-P2-1:
// the aux label is hard deleted after the job was created (aux GC delete path
// — not blocked by the repair tables, history kept). The stored request then
// still replays; a different payload under the same key conflicts; a new key
// is not found; and no second job row appears.
func TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)

	// A filler label makes the topic-tag id and the label id differ on
	// purpose — the assertions below must not pass by coincidence. It is
	// seeded FIRST so the label under test gets the next semantic_labels id
	// while the topic tag keeps the first topic_tags id.
	seedRepairLabel(t, db, "占位标签", "active", "占位")
	label := seedRepairLabel(t, db, "硬删标签", "active", "旧描述")
	tag := seedRepairTag(t, db, "硬删范围标签", "active")
	require.NotEqual(t, tag.ID, label.ID, "the fixture must keep the two ids apart")
	labelCtx, err := repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)

	params := CreateRepairJobParams{
		AuxiliaryLabelID: label.ID,
		IdempotencyKey:   "hard-delete-replay-1",
		ExpectedVersion:  labelCtx.Version,
		NewDescription:   "硬删前确认的描述",
	}
	job, err := repo.CreateRepairJob(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, 1, job.Total)

	// The source label is hard deleted (no soft-delete flag involved).
	require.NoError(t, db.Delete(&models.SemanticLabel{}, label.ID).Error)
	var gone models.SemanticLabel
	require.Error(t, db.Where("id = ?", label.ID).First(&gone).Error,
		"the label row must really be gone for this regression to mean anything")

	// 1. Same key + same payload → the stored job (replay), NOT a not-found.
	replay, err := repo.CreateRepairJob(context.Background(), params)
	require.NoError(t, err, "a stored request must replay after the label was deleted")
	require.Equal(t, job.ID, replay.ID)
	require.Equal(t, "硬删前确认的描述", replay.NewDescription)

	// 2. Same key + different payload → explicit conflict.
	_, err = repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID,
		IdempotencyKey:   params.IdempotencyKey,
		ExpectedVersion:  params.ExpectedVersion,
		NewDescription:   "改了主意的描述",
	})
	require.ErrorIs(t, err, ErrRepairIdempotencyConflict)

	// 3. A new key → the label no longer exists (plain not-found).
	_, err = repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID,
		IdempotencyKey:   "hard-delete-new-key",
		ExpectedVersion:  params.ExpectedVersion,
		NewDescription:   "删除后的新请求",
	})
	require.ErrorIs(t, err, ErrRepairLabelNotFound)

	// 4. Exactly one job row; the frozen scope history survived the delete.
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs)
	var items []tagmodels.AuxiliaryDescriptionRepairItem
	require.NoError(t, db.Where("job_id = ?", job.ID).Order("id ASC").Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, tag.ID, items[0].TopicTagID, "the frozen scope must hold the seeded topic tag")
	require.Equal(t, label.ID, job.AuxiliaryLabelID, "the job belongs to the auxiliary label")
	require.NotEqual(t, tag.ID, label.ID,
		"the fixture must keep the topic-tag id and the label id apart — otherwise the assertions above are a coincidence")
}

// TestAuxiliaryDescriptionRepairRequestBoundaryRejections covers B-P2-3: the
// idempotency key and the opaque version token are bounded by their
// varchar(64) columns. 64 characters are accepted, 65 are rejected in Go
// (a client error, never a DB 22001 → 500), and a rejected request writes no
// job row.
func TestAuxiliaryDescriptionRepairRequestBoundaryRejections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewAuxiliaryDescriptionRepairRepository(db)

	label := seedRepairLabel(t, db, "边界", "active", "旧描述")
	seedRepairTag(t, db, "边界标签", "active")
	labelCtx, err := repo.GetRepairContext(context.Background(), label.ID)
	require.NoError(t, err)

	// 64 characters = the column capacity → accepted.
	key64 := strings.Repeat("k", 64)
	job, err := repo.CreateRepairJob(context.Background(), CreateRepairJobParams{
		AuxiliaryLabelID: label.ID,
		IdempotencyKey:   key64,
		ExpectedVersion:  labelCtx.Version,
		NewDescription:   "边界描述",
	})
	require.NoError(t, err, "a 64-character key fits the column and must be accepted")
	require.Equal(t, key64, job.IdempotencyKey)

	// The label now holds an unfinished job, so the mutex would also reject
	// the following creates — every rejection below must be the boundary
	// error, never the mutex error.
	cases := []struct {
		name    string
		params  CreateRepairJobParams
		wantErr error
	}{
		{
			name: "key 65 characters exceeds the varchar(64) column",
			params: CreateRepairJobParams{
				AuxiliaryLabelID: label.ID,
				IdempotencyKey:   strings.Repeat("k", 65),
				ExpectedVersion:  labelCtx.Version,
				NewDescription:   "边界描述",
			},
			wantErr: ErrRepairIdempotencyKeyTooLong,
		},
		{
			name: "key empty after trim",
			params: CreateRepairJobParams{
				AuxiliaryLabelID: label.ID,
				IdempotencyKey:   "   ",
				ExpectedVersion:  labelCtx.Version,
				NewDescription:   "边界描述",
			},
			wantErr: ErrRepairIdempotencyKeyEmpty,
		},
		{
			name: "version token missing",
			params: CreateRepairJobParams{
				AuxiliaryLabelID: label.ID,
				IdempotencyKey:   "boundary-version-missing",
				ExpectedVersion:  "",
				NewDescription:   "边界描述",
			},
			wantErr: ErrRepairExpectedVersionInvalid,
		},
		{
			name: "version token exceeds the varchar(64) column",
			params: CreateRepairJobParams{
				AuxiliaryLabelID: label.ID,
				IdempotencyKey:   "boundary-version-too-long",
				ExpectedVersion:  strings.Repeat("v", 65),
				NewDescription:   "边界描述",
			},
			wantErr: ErrRepairExpectedVersionInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.CreateRepairJob(context.Background(), tc.params)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}

	// No rejected request created a task.
	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs, "only the accepted 64-character key may create a task")
}
