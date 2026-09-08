package repository_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

func setupResultKindRepositoryDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	require.NoError(t, db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-repo-%'`).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-repo-%' AND parent_result_id IS NOT NULL`).Error
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-repo-%'`).Error
	})
	return repository.NewRepository(db)
}

func stringPtr(value string) *string { return &value }

func TestQuestionKeyNormalizationHash(t *testing.T) {
	key := repository.ComputeQuestionKey("\t  美国\u3000  制造业\n回流  ")
	require.Equal(t, repository.ComputeQuestionKey("美国 制造业 回流"), key)
	require.Len(t, key, 64)
	_, err := hex.DecodeString(key)
	require.NoError(t, err)
	require.NotEqual(t, key, repository.ComputeQuestionKey("美国制造业回流"), "normalization must preserve token boundaries")
}

func TestResultKindOwnerShapeValidation(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	persistentTopicID := uint(98291)
	semanticBoardID := uint(98292)

	invalid := map[string]*repository.TopicEnrichmentResult{
		"topic-missing-owner": {
			AnalysisScope: "topic", ResultKind: repository.ResultKindTopicAnalysis,
			SessionID: "result-kind-repo-topic-missing-owner",
		},
		"topic-mixed-owner": {
			PersistentTopicID: &persistentTopicID, SemanticBoardID: &semanticBoardID,
			AnalysisScope: "topic", ResultKind: repository.ResultKindTopicAnalysis,
			SessionID: "result-kind-repo-topic-mixed-owner",
		},
		"board-missing-owner": {
			AnalysisScope: "board", ResultKind: repository.ResultKindBoardBrief,
			SessionID: "result-kind-repo-board-missing-owner",
		},
		"board-mixed-owner": {
			PersistentTopicID: &persistentTopicID, SemanticBoardID: &semanticBoardID,
			AnalysisScope: "board", ResultKind: repository.ResultKindLegacyBoardAnalysis,
			SessionID: "result-kind-repo-board-mixed-owner",
		},
		"unknown-kind": {
			PersistentTopicID: &persistentTopicID, AnalysisScope: "topic", ResultKind: "invalid_kind",
			SessionID: "result-kind-repo-unknown-kind",
		},
	}
	for name, result := range invalid {
		t.Run(name, func(t *testing.T) {
			require.Error(t, repo.CreateTopicEnrichmentResult(ctx, result))
			require.Zero(t, result.ID)
		})
	}
}

func TestBoardResultKindQueries(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	boardID := uint(98301)
	otherBoardID := uint(98302)

	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "result-kind-repo-legacy",
	}
	brief1 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-brief-1",
	}
	brief2 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-brief-2",
	}
	otherBoardBrief := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-other-board-brief",
	}
	for _, result := range []*repository.TopicEnrichmentResult{legacy, brief1, brief2, otherBoardBrief} {
		require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, result))
	}

	briefs, err := repo.ListBoardEnrichmentResultsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Len(t, briefs, 2)
	require.Equal(t, brief2.ID, briefs[0].ID)
	require.Equal(t, brief1.ID, briefs[1].ID)
	require.NotEqual(t, otherBoardBrief.ID, briefs[0].ID)
	require.NotEqual(t, otherBoardBrief.ID, briefs[1].ID)

	latest, err := repo.GetLatestBoardEnrichmentResultByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Equal(t, brief2.ID, latest.ID)
	prev, err := repo.GetPrevLatestBoardEnrichmentResultByKind(ctx, boardID, repository.ResultKindBoardBrief, brief2.ID)
	require.NoError(t, err)
	require.Equal(t, brief1.ID, prev.ID)

	all, err := repo.ListBoardEnrichmentResults(ctx, boardID)
	require.NoError(t, err)
	require.Len(t, all, 3, "legacy unfiltered API must remain compatible and cross-board rows must stay isolated")

	_, err = repo.ListBoardEnrichmentResultsByKind(ctx, boardID, "invalid_kind")
	require.Error(t, err)
	_, err = repo.GetLatestBoardEnrichmentResultByKind(ctx, boardID, repository.ResultKindTopicAnalysis)
	require.Error(t, err, "topic kind is invalid in a board-kind query")
	_, err = repo.GetPrevLatestBoardEnrichmentResultByKind(ctx, boardID, "invalid_kind", brief2.ID)
	require.Error(t, err)
}

// task 3.5: brief1 → 合法 investigation（id 更高，夹在两份 brief 之间）→ brief2
// 时，kind 隔离查询必须仍沿 board_brief 链取回 brief1——investigation 行
// 永不混入简报链的 prev/latest/list 查询。
func TestBoardBriefKindChainSkipsInterleavedInvestigation(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	boardID := uint(98331)

	brief1 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-chain-brief-1",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, brief1))

	// Legal investigation child of brief1: same board, valid question_key,
	// id strictly between the two briefs.
	questionKey := repository.ComputeQuestionKey("夹在两份简报之间的合法调查问题？")
	investigation := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(brief1.ID), QuestionKey: stringPtr(questionKey),
		SessionID: "result-kind-repo-chain-investigation",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, investigation))
	require.Equal(t, repository.ResultKindBoardInvestigation, investigation.ResultKind)
	require.Greater(t, investigation.ID, brief1.ID, "investigation must sit between the two briefs")

	brief2 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-chain-brief-2",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, brief2))
	require.Greater(t, brief2.ID, investigation.ID, "brief2 must be the newest row")

	prev, err := repo.GetPrevLatestBoardEnrichmentResultByKind(ctx, boardID, repository.ResultKindBoardBrief, brief2.ID)
	require.NoError(t, err)
	require.Equal(t, brief1.ID, prev.ID, "prev-by-kind must stay on the brief chain, skipping the interleaved investigation")

	briefs, err := repo.ListBoardEnrichmentResultsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Len(t, briefs, 2, "kind list must not include the investigation")
	require.Equal(t, brief2.ID, briefs[0].ID)
	require.Equal(t, brief1.ID, briefs[1].ID)
}

func TestBoardInvestigationParentValidationAndMultipleChildren(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	boardID := uint(98311)
	otherBoardID := uint(98312)

	brief := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-parent",
	}
	otherBrief := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-other-parent",
	}
	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "result-kind-repo-non-brief",
	}
	for _, result := range []*repository.TopicEnrichmentResult{brief, otherBrief, legacy} {
		require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, result))
	}

	questionKey1 := repository.ComputeQuestionKey("为什么制造业投资增加？")
	questionKey2 := repository.ComputeQuestionKey("就业是否同步改善？")
	child1 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(brief.ID), QuestionKey: stringPtr(questionKey1),
		SessionID: "result-kind-repo-child-1",
	}
	child2 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(brief.ID), QuestionKey: stringPtr(questionKey2),
		SessionID: "result-kind-repo-child-2",
	}
	for _, child := range []*repository.TopicEnrichmentResult{child1, child2} {
		require.NoError(t, repo.CreateBoardInvestigationResult(ctx, child))
		require.Equal(t, repository.ResultKindBoardInvestigation, child.ResultKind)
	}
	otherChild := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(otherBrief.ID), QuestionKey: stringPtr(questionKey1),
		SessionID: "result-kind-repo-other-child",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, otherChild))

	children, err := repo.ListBoardEnrichmentResultsByParent(ctx, brief.ID)
	require.NoError(t, err)
	require.Len(t, children, 2, "children from another parent must stay isolated")
	require.Equal(t, child2.ID, children[0].ID)
	otherChildren, err := repo.ListBoardEnrichmentResultsByParent(ctx, otherBrief.ID)
	require.NoError(t, err)
	require.Len(t, otherChildren, 1)
	require.Equal(t, otherChild.ID, otherChildren[0].ID)

	for name, parentID := range map[string]uint{
		"cross-board": otherBrief.ID,
		"non-brief":   legacy.ID,
	} {
		t.Run(name, func(t *testing.T) {
			invalid := &repository.TopicEnrichmentResult{
				SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
				ParentResultID: repository.TopicIDPtr(parentID), QuestionKey: stringPtr(questionKey1),
				SessionID: "result-kind-repo-invalid-" + name,
			}
			require.Error(t, repo.CreateBoardInvestigationResult(ctx, invalid))
			require.Zero(t, invalid.ID)
		})
	}

	missingKey := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(brief.ID), SessionID: "result-kind-repo-missing-key",
	}
	require.Error(t, repo.CreateBoardInvestigationResult(ctx, missingKey))

	illegalParent := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, ParentResultID: repository.TopicIDPtr(brief.ID),
		SessionID: "result-kind-repo-illegal-parent",
	}
	require.Error(t, repo.CreateTopicEnrichmentResult(ctx, illegalParent))
}

// task 3.5: applied-review digest 必须按 result_kind 隔离——brief 链 digest 只
// 能命中“同 board + kind=board_brief 的 curr 结果”的 applied review；legacy /
// 他板块 / 未 applied 一律隔离。
func TestListAppliedBoardEnrichmentReviewsByKind(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	boardID := uint(98321)
	otherBoardID := uint(98322)

	// Cleanup reviews BEFORE the session-scoped result cleanup (FK-free but
	// deterministic).
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_review WHERE deviation_summary LIKE 'kind-review-%'`).Error
	})

	brief1 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-brief-review-1",
	}
	brief2 := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-brief-review-2",
	}
	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "result-kind-repo-brief-review-legacy",
	}
	otherBoardBrief := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-brief-review-other",
	}
	for _, result := range []*repository.TopicEnrichmentResult{brief1, brief2, legacy, otherBoardBrief} {
		require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, result))
	}

	mk := func(boardID, currResultID uint, summary string, applied bool) *repository.TopicEnrichmentReview {
		return &repository.TopicEnrichmentReview{
			SemanticBoardID: repository.BoardIDPtr(boardID), CurrResultID: currResultID,
			DeviationSummary: summary, Applied: applied, Source: "llm_assisted",
		}
	}
	appliedBrief := mk(boardID, brief2.ID, "kind-review-brief-chain", true)                    // in
	appliedLegacy := mk(boardID, legacy.ID, "kind-review-legacy-chain", true)                  // out: kind
	pendingBrief := mk(boardID, brief1.ID, "kind-review-pending", false)                       // out: not applied
	appliedOtherBoard := mk(otherBoardID, otherBoardBrief.ID, "kind-review-other-board", true) // out: board
	for _, rv := range []*repository.TopicEnrichmentReview{appliedBrief, appliedLegacy, pendingBrief, appliedOtherBoard} {
		require.NoError(t, repo.CreateTopicEnrichmentReview(ctx, rv))
	}

	list, err := repo.ListAppliedBoardEnrichmentReviewsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, appliedBrief.ID, list[0].ID)

	// task 3.5: 同 created_at 并列时必须按 id DESC 稳定排序（digest 只取前 N
	// 条，bulk seed / 秒级时钟粒度下的确定性保证）。按生产时序追加两条同 kind
	// applied review——先 brief1 链（时序旧），后 brief2 链（时序新，id 更高）；
	// 把同 kind 三条全部钉到同一 created_at，纯 id DESC 必须让时序最新的
	// brief2 链 review 排最前。
	appliedBriefChainOld := mk(boardID, brief1.ID, "kind-review-brief-chain-old", true)
	require.NoError(t, repo.CreateTopicEnrichmentReview(ctx, appliedBriefChainOld))
	appliedBriefChainNew := mk(boardID, brief2.ID, "kind-review-brief-chain-new", true)
	require.NoError(t, repo.CreateTopicEnrichmentReview(ctx, appliedBriefChainNew))
	require.Greater(t, appliedBriefChainNew.ID, appliedBriefChainOld.ID, "insertion order must give the chronologically-newer review the higher id")
	sameTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.DB().Exec(
		`UPDATE topic_enrichment_review SET created_at = ? WHERE id IN (?, ?, ?)`,
		sameTime, appliedBrief.ID, appliedBriefChainOld.ID, appliedBriefChainNew.ID).Error)

	list, err = repo.ListAppliedBoardEnrichmentReviewsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Greater(t, list[0].ID, list[1].ID, "same created_at must tie-break by id DESC")
	require.Greater(t, list[1].ID, list[2].ID)
	require.Equal(t, appliedBriefChainNew.ID, list[0].ID, "highest-id (newest) brief-chain review must lead the digest order")
	require.Equal(t, appliedBriefChainOld.ID, list[1].ID)
	require.Equal(t, appliedBrief.ID, list[2].ID)

	_, err = repo.ListAppliedBoardEnrichmentReviewsByKind(ctx, boardID, "invalid_kind")
	require.Error(t, err)
	_, err = repo.ListAppliedBoardEnrichmentReviewsByKind(ctx, boardID, repository.ResultKindTopicAnalysis)
	require.Error(t, err, "topic kind is invalid in a board-kind review query")
}

// task 4.7: GetPrevBoardInvestigationByQuestion 严格链查询——只沿同 board +
// kind=board_investigation + 同 parent_result_id + 同 question_key 取 id 更小
// 的最新一行为 prev；不同问题/不同 parent/别版块/legacy/brief 夹行、以及
// question_key 为 NULL 的回填前旧行一律不命中；非法 key 参数直接报错。
func TestGetPrevBoardInvestigationByQuestion(t *testing.T) {
	repo := setupResultKindRepositoryDB(t)
	ctx := context.Background()
	boardID := uint(98361)
	otherBoardID := uint(98362)

	briefA := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-invq-brief-a",
	}
	briefB := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-invq-brief-b",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, briefA))
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, briefB))

	key1 := repository.ComputeQuestionKey("同一调查问题重跑？")
	key2 := repository.ComputeQuestionKey("另一个调查问题？")

	mkInv := func(parent uint, key string, session string) *repository.TopicEnrichmentResult {
		return &repository.TopicEnrichmentResult{
			SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
			ParentResultID: repository.TopicIDPtr(parent), QuestionKey: stringPtr(key),
			SessionID: session,
		}
	}

	// inv1：链上第一份（briefA + key1）。
	inv1 := mkInv(briefA.ID, key1, "result-kind-repo-invq-1")
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, inv1))

	// 夹行：同 board legacy（无 parent/key）、同 board 新简报、
	// 别 parent 同 key、同 parent 异 key、别版块同 key。
	legacyNoise := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "result-kind-repo-invq-legacy",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, legacyNoise))
	briefNoise := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-invq-brief-noise",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, briefNoise))
	invOtherParent := mkInv(briefB.ID, key1, "result-kind-repo-invq-other-parent")
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, invOtherParent))
	invOtherKey := mkInv(briefA.ID, key2, "result-kind-repo-invq-other-key")
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, invOtherKey))
	otherBoardBrief := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "result-kind-repo-invq-other-board-brief",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, otherBoardBrief))
	invOtherBoard := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(otherBoardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(otherBoardBrief.ID), QuestionKey: stringPtr(key1),
		SessionID: "result-kind-repo-invq-other-board",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, invOtherBoard))

	// inv2：链上第二份（briefA + key1，id 最高）。
	inv2 := mkInv(briefA.ID, key1, "result-kind-repo-invq-2")
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, inv2))

	// prev 查询必须落在同链 inv1 上，跳过全部夹行。
	prev, err := repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefA.ID, key1, inv2.ID)
	require.NoError(t, err)
	require.Equal(t, inv1.ID, prev.ID, "prev must stay on the same parent+key chain")

	// 链上第一份：自身为 current 时无 prev。
	_, err = repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefA.ID, key1, inv1.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 隔离正向验证：异 key / 异 parent / 别版块的链各自查回自己的行，
	// 永远不会把 inv1/inv2（briefA+key1 链）当成 prev。
	prevOtherKey, err := repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefA.ID, key2, inv2.ID)
	require.NoError(t, err)
	require.Equal(t, invOtherKey.ID, prevOtherKey.ID, "different key must not compare against the key1 chain")
	prevOtherParent, err := repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefB.ID, key1, inv2.ID)
	require.NoError(t, err)
	require.Equal(t, invOtherParent.ID, prevOtherParent.ID, "different parent must not compare against the briefA chain")
	prevOtherBoard, err := repo.GetPrevBoardInvestigationByQuestion(ctx, otherBoardID, otherBoardBrief.ID, key1, inv2.ID)
	require.NoError(t, err)
	require.Equal(t, invOtherBoard.ID, prevOtherBoard.ID, "other board must not compare against this board's chain")

	// 真空链（briefB+key2 无任何行）：NotFound。
	_, err = repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefB.ID, key2, inv2.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 非法 key（非 SHA-256 hex）直接报错，不落到 SQL。
	_, err = repo.GetPrevBoardInvestigationByQuestion(ctx, boardID, briefA.ID, "not-a-sha256-key", inv2.ID)
	require.Error(t, err)
}
