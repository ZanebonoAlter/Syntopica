package auxlabel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// 任务 3.2（design D1 原子性 + D5 testcontainer）：阶段二任一写失败 →
// 整体回滚，无部分挂载/部分新建/部分 alias 残留。
//
// 注入手段：BEFORE INSERT 触发器对特定 label 文本 RAISE（外科手术式，不依赖
// 列级 DDL——golden schema 的 vector 列是不定维的，维度注入不会失败）。
// 失败发生在 alias UPDATE（标签 A）之后的同事务内，验证整体回滚。
func TestAttachPhaseTwoWriteFailureRollsBackAll(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "原子性标签", Slug: "atomicity", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	// 既有标签 T：带 merge 向量，作为标签 A 的 L2 归并目标（aliases 有既有值）。
	targetVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	target := models.SemanticLabel{
		Label: "归并目标", Slug: "merge-target", LabelType: "auxiliary", Status: "active",
		MergeEmbedding: &targetVec, Aliases: []string{"既有别名"}, RefCount: 7,
	}
	require.NoError(t, db.Create(&target).Error)

	// 注入点：阶段二 INSERT semantic_labels(label='必失败标签') 时触发器报错。
	require.NoError(t, db.Exec(`
		CREATE OR REPLACE FUNCTION aux_atomicity_fail() RETURNS trigger AS $$
		BEGIN
			IF NEW.label = '必失败标签' THEN
				RAISE EXCEPTION 'forced insert failure for atomicity test';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER trg_aux_atomicity
		BEFORE INSERT ON semantic_labels
		FOR EACH ROW EXECUTE FUNCTION aux_atomicity_fail()`).Error)
	t.Cleanup(func() {
		_ = db.Exec("DROP TRIGGER IF EXISTS trg_aux_atomicity ON semantic_labels").Error
		_ = db.Exec("DROP FUNCTION IF EXISTS aux_atomicity_fail()").Error
	})

	// 批量 mock：A 的 merge 向量与 T cosine≈1（L2 命中），B 正常向量走 L3。
	batch := func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
		pgVectors := make([]string, len(inputs))
		vectors := make([][]float64, len(inputs))
		for i, input := range inputs {
			var vec []float64
			switch {
			case mode == AuxiliaryLabelEmbeddingModeMerge && input == "归并候选":
				vec = testutil.PadVector([]float64{0.999, 0.001, 0}, testutil.TestEmbeddingDim) // 与 T cosine≈1 → L2
			case mode == AuxiliaryLabelEmbeddingModeMerge && input == "必失败标签":
				vec = testutil.PadVector([]float64{0, 0, 1}, testutil.TestEmbeddingDim) // 与 T 正交 → L3 → INSERT
			default:
				vec = testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
			}
			pgVectors[i], vectors[i] = core.FloatsToPgVector(vec), vec
		}
		return pgVectors, vectors, nil
	}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch)

	err := service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "归并候选", Description: "应 L2 归并进目标"},  // 阶段二写 alias UPDATE
		{Label: "必失败标签", Description: "INSERT 必失败"}, // 阶段二写 INSERT 失败
	})
	require.Error(t, err, "trigger-injected INSERT failure must fail the attach")
	require.Contains(t, err.Error(), "forced insert failure")

	// 整体回滚：无新建行、无 link、alias 未追加、ref_count 未动。
	var created int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label IN ?", []string{"必失败标签", "归并候选"}).Count(&created).Error)
	require.Zero(t, created, "rolled-back attach must leave no new label rows")

	var links int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&links).Error)
	require.Zero(t, links, "no link may survive a phase-two failure")

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, target.ID).Error)
	require.Equal(t, []string{"既有别名"}, reloaded.Aliases, "the alias UPDATE earlier in the same tx must roll back with it")
	require.Equal(t, 7, reloaded.RefCount, "ref_count bump must roll back")
}
