package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// seedDupAuxLabels creates three active auxiliary labels that all normalize to
// the same key ("sk海力士") but carry distinct slugs (slug is unique-constrained),
// plus topic_tag + board_composition references. Returns the three labels, the
// board ids, and the distinct topic-tag ids so the caller can assert post-merge
// state. refCount order makes primaryA the expected primary (highest ref_count).
func seedDupAuxLabels(t *testing.T, db *gorm.DB) (primaryA, secondaryB, secondaryC models.SemanticLabel) {
	t.Helper()

	// Topic tags (the entities that reference aux labels).
	topics := []models.TopicTag{
		{Label: "t1", Slug: "t1", Category: "event", Status: "active"},
		{Label: "t2", Slug: "t2", Category: "event", Status: "active"},
		{Label: "t3", Slug: "t3", Category: "event", Status: "active"},
		{Label: "t4", Slug: "t4", Category: "event", Status: "active"},
	}
	for i := range topics {
		require.NoError(t, db.Create(&topics[i]).Error)
	}

	// Three text-variant aux labels: NormalizeLabelKey("SK海力士") == "sk海力士"
	// for all three, but Slugify differs / slugs are manually distinct.
	primaryA = models.SemanticLabel{Label: "SK海力士", Slug: "dup-sk-1", LabelType: "auxiliary", Status: "active", RefCount: 24, Aliases: []string{}}
	secondaryB = models.SemanticLabel{Label: "SK 海力士", Slug: "dup-sk-2", LabelType: "auxiliary", Status: "active", RefCount: 14, Aliases: []string{"SK海力士别名"}}
	secondaryC = models.SemanticLabel{Label: "SK海力士 ", Slug: "dup-sk-3", LabelType: "auxiliary", Status: "active", RefCount: 4, Aliases: []string{}}
	require.NoError(t, db.Create(&primaryA).Error)
	require.NoError(t, db.Create(&secondaryB).Error)
	require.NoError(t, db.Create(&secondaryC).Error)

	// Board labels (mount points).
	board1 := models.SemanticLabel{Label: "Board One", Slug: "board-one", LabelType: "board", Status: "active"}
	board2 := models.SemanticLabel{Label: "Board Two", Slug: "board-two", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board1).Error)
	require.NoError(t, db.Create(&board2).Error)

	// topic_tag_semantic_labels references (these drive ref_count recount).
	//   primaryA <- t1 ; secondaryB <- t2,t3 ; secondaryC <- t4
	refs := []tagmodels.TopicTagSemanticLabel{
		{TopicTagID: topics[0].ID, SemanticLabelID: primaryA.ID},
		{TopicTagID: topics[1].ID, SemanticLabelID: secondaryB.ID},
		{TopicTagID: topics[2].ID, SemanticLabelID: secondaryB.ID},
		{TopicTagID: topics[3].ID, SemanticLabelID: secondaryC.ID},
	}
	for i := range refs {
		require.NoError(t, db.Create(&refs[i]).Error)
	}

	// board_composition references:
	//   board1 <- primaryA, secondaryB ; board2 <- secondaryB
	comps := []tagmodels.BoardComposition{
		{BoardID: board1.ID, AuxiliaryLabelID: primaryA.ID},
		{BoardID: board1.ID, AuxiliaryLabelID: secondaryB.ID},
		{BoardID: board2.ID, AuxiliaryLabelID: secondaryB.ID},
	}
	for i := range comps {
		require.NoError(t, db.Create(&comps[i]).Error)
	}

	return primaryA, secondaryB, secondaryC
}

// TestAuxLabelDupMergeMergesTextVariants verifies the one-shot dup-merge:
// primary kept active, secondaries disabled, topic_tag + board_composition
// references repointed, ref_count recounted, aliases folded into primary.
func TestAuxLabelDupMergeMergesTextVariants(t *testing.T) {
	db := testutil.SetupTestDB(t)
	primaryA, secondaryB, secondaryC := seedDupAuxLabels(t, db)

	require.NoError(t, database.ExportedRunAuxLabelDupMerge(db), "dup-merge should run without error")

	// Reload all three.
	var a, b, c models.SemanticLabel
	require.NoError(t, db.First(&a, primaryA.ID).Error)
	require.NoError(t, db.First(&b, secondaryB.ID).Error)
	require.NoError(t, db.First(&c, secondaryC.ID).Error)

	// Primary kept active; secondaries disabled.
	require.Equal(t, "active", a.Status, "primary must remain active")
	require.Equal(t, "disabled", b.Status, "secondary B must be disabled")
	require.Equal(t, "disabled", c.Status, "secondary C must be disabled")

	// Primary ref_count recounted = 4 distinct topic_tags (t1..t4) now all
	// pointing at primaryA.
	require.Equal(t, 4, a.RefCount, "primary ref_count must be recounted to distinct topic_tag count")

	// No topic_tag_semantic_labels reference the secondaries anymore.
	var bLinks, cLinks int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", secondaryB.ID).Count(&bLinks).Error)
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", secondaryC.ID).Count(&cLinks).Error)
	require.Zero(t, bLinks, "no topic_tag links should reference secondary B")
	require.Zero(t, cLinks, "no topic_tag links should reference secondary C")

	// 4 distinct topic_tags now reference primaryA.
	var aLinks int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", primaryA.ID).Count(&aLinks).Error)
	require.Equal(t, int64(4), aLinks)

	// No board_composition references the secondaries.
	var bComps, cComps int64
	require.NoError(t, db.Model(&tagmodels.BoardComposition{}).Where("auxiliary_label_id = ?", secondaryB.ID).Count(&bComps).Error)
	require.NoError(t, db.Model(&tagmodels.BoardComposition{}).Where("auxiliary_label_id = ?", secondaryC.ID).Count(&cComps).Error)
	require.Zero(t, bComps, "no board_composition should reference secondary B")
	require.Zero(t, cComps, "no board_composition should reference secondary C")

	// Primary's aliases fold in the secondary variant label + pre-existing alias.
	require.Contains(t, a.Aliases, secondaryB.Label, "primary aliases must contain secondary B label")
	require.Contains(t, a.Aliases, "SK海力士别名", "primary aliases must contain secondary B pre-existing alias")
}

// TestAuxLabelDupMergeIdempotent verifies a second run is a no-op: disabled
// secondaries are excluded from grouping, so no further changes occur.
func TestAuxLabelDupMergeIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	primaryA, secondaryB, secondaryC := seedDupAuxLabels(t, db)

	require.NoError(t, database.ExportedRunAuxLabelDupMerge(db))

	// Snapshot post-first-merge state.
	var aAfter1 models.SemanticLabel
	require.NoError(t, db.First(&aAfter1, primaryA.ID).Error)

	// Re-run — must succeed and change nothing.
	require.NoError(t, database.ExportedRunAuxLabelDupMerge(db))

	var a, b, c models.SemanticLabel
	require.NoError(t, db.First(&a, primaryA.ID).Error)
	require.NoError(t, db.First(&b, secondaryB.ID).Error)
	require.NoError(t, db.First(&c, secondaryC.ID).Error)

	require.Equal(t, "active", a.Status)
	require.Equal(t, "disabled", b.Status)
	require.Equal(t, "disabled", c.Status)
	require.Equal(t, aAfter1.RefCount, a.RefCount, "ref_count must not change on second run")
	require.Equal(t, aAfter1.Aliases, a.Aliases, "aliases must not change on second run")

	// Still no dangling references to secondaries.
	var dangling int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id IN ?", []uint{secondaryB.ID, secondaryC.ID}).Count(&dangling).Error)
	require.Zero(t, dangling)
}

// TestAuxLabelDupMergeNoOpOnClean verifies the merge is a no-op when there are
// no text-variant duplicates (every normalize key is unique).
func TestAuxLabelDupMergeNoOpOnClean(t *testing.T) {
	db := testutil.SetupTestDB(t)

	unique1 := models.SemanticLabel{Label: "量子计算", Slug: "q-compute", LabelType: "auxiliary", Status: "active", RefCount: 3}
	unique2 := models.SemanticLabel{Label: "光子计算", Slug: "p-compute", LabelType: "auxiliary", Status: "active", RefCount: 1}
	require.NoError(t, db.Create(&unique1).Error)
	require.NoError(t, db.Create(&unique2).Error)

	require.NoError(t, database.ExportedRunAuxLabelDupMerge(db))

	var u1, u2 models.SemanticLabel
	require.NoError(t, db.First(&u1, unique1.ID).Error)
	require.NoError(t, db.First(&u2, unique2.ID).Error)
	require.Equal(t, "active", u1.Status)
	require.Equal(t, "active", u2.Status)
	require.Equal(t, 3, u1.RefCount, "clean labels must keep their ref_count")
}
