package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

func TestAnalysisMethodRepositoryCRUDSoftDeleteAndEnabledSummaries(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	repo := repository.NewRepository(db)
	ctx := context.Background()
	const prefix = "analysis-method-repository-test-"
	t.Cleanup(func() { _ = db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.AnalysisMethod{}).Error })
	require.NoError(t, db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.AnalysisMethod{}).Error)

	enabled := &repository.AnalysisMethod{
		Name: prefix + "enabled", Title: "因果链检验", Summary: "核查必要条件与替代解释",
		SelectionMeta: repository.AnalysisMethodSelectionMeta{
			ApplicableWhen: []string{"提出因果关系"}, AvoidWhen: []string{"无时间序列"},
			RequiredEvidence: []string{"时间序列"}, FailureModes: []string{"相关当因果"},
		},
		Content: "步骤一：列出竞争假设。", Enabled: true,
	}
	disabled := &repository.AnalysisMethod{
		Name: prefix + "legacy", Title: "旧画像", Content: "原文", Enabled: false, Legacy: true,
	}
	require.NoError(t, repo.CreateAnalysisMethod(ctx, enabled))
	require.NoError(t, repo.CreateAnalysisMethod(ctx, disabled))

	got, err := repo.GetAnalysisMethodByID(ctx, enabled.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"无时间序列"}, got.SelectionMeta.AvoidWhen)

	got.Summary = "已编辑摘要"
	require.NoError(t, repo.UpdateAnalysisMethod(ctx, got))
	got, err = repo.GetAnalysisMethodByID(ctx, enabled.ID)
	require.NoError(t, err)
	require.Equal(t, "已编辑摘要", got.Summary)

	summaries, err := repo.ListEnabledAnalysisMethodSummaries(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, enabled.ID, summaries[0].ID)
	require.Empty(t, summaries[0].Content, "selection summaries must not preload full content")

	loaded, err := repo.GetAnalysisMethodsByIDs(ctx, []uint{enabled.ID, disabled.ID})
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	require.Equal(t, enabled.ID, loaded[0].ID, "load order must follow requested relevance order")
	require.Equal(t, "步骤一：列出竞争假设。", loaded[0].Content)
	require.True(t, loaded[1].Legacy)

	require.NoError(t, repo.SetAnalysisMethodEnabled(ctx, enabled.ID, false))
	summaries, err = repo.ListEnabledAnalysisMethodSummaries(ctx)
	require.NoError(t, err)
	require.Empty(t, summaries)

	// Soft-deleting an enabled card must remove it from the selector view too.
	require.NoError(t, repo.SetAnalysisMethodEnabled(ctx, enabled.ID, true))
	require.NoError(t, repo.DeleteAnalysisMethod(ctx, enabled.ID))
	summaries, err = repo.ListEnabledAnalysisMethodSummaries(ctx)
	require.NoError(t, err)
	require.Empty(t, summaries)

	require.NoError(t, repo.DeleteAnalysisMethod(ctx, disabled.ID))
	_, err = repo.GetAnalysisMethodByID(ctx, disabled.ID)
	require.Error(t, err)
	all, err := repo.ListAnalysisMethods(ctx)
	require.NoError(t, err)
	require.Empty(t, all)
	var deleted repository.AnalysisMethod
	require.NoError(t, db.Unscoped().First(&deleted, disabled.ID).Error)
	require.True(t, deleted.DeletedAt.Valid)
	require.True(t, deleted.Legacy)
}
