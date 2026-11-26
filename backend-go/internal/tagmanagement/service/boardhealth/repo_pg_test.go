package boardhealth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 用例来源：test-cases.md 主链路步 5（事实画像 diff：testcontainer PG repository
// 测试，禁 SQLite）+ 步 8（快照落库）+ 步 13/14 前置（趋势/空态/404 的数据层）。

func createAuxLabel(t *testing.T, db *gorm.DB, label string, status string) models.SemanticLabel {
	t.Helper()
	aux := models.SemanticLabel{Label: label, Slug: "bhx-" + label, LabelType: "auxiliary", Status: status}
	require.NoError(t, db.Create(&aux).Error)
	return aux
}

func createTopicTagRow(t *testing.T, db *gorm.DB, slug string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: slug, Slug: slug, Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// linkAuxToTag mounts an aux label on a tag (topic_tag_semantic_labels).
func linkAuxToTag(t *testing.T, db *gorm.DB, tagID uint, auxID uint) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagID, SemanticLabelID: auxID}).Error)
}

// matchTagToBoard writes a topic_tag_board_labels row.
func matchTagToBoard(t *testing.T, db *gorm.DB, tagID uint, boardID uint) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagID, SemanticBoardID: boardID, Score: 0.7, MatchReason: "direct_hit",
	}).Error)
}

func TestUncapturedCandidatesAggregation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "AI前沿", "active")

	// 已挂载构成：大模型（不得进候选）。
	mounted := createAuxLabel(t, db, "大模型", "active")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: mounted.ID}).Error)
	// 未收编高频：算力（3 个成员 tag，2 篇文章）。
	uncaptured := createAuxLabel(t, db, "算力", "active")
	// disabled：禁用标签不得进候选。
	disabled := createAuxLabel(t, db, "隐私计算", "disabled")

	for i := 0; i < 3; i++ {
		tag := createTopicTagRow(t, db, "gpu-tag-"+string(rune('a'+i)))
		linkAuxToTag(t, db, tag.ID, uncaptured.ID)
		matchTagToBoard(t, db, tag.ID, board.ID)
		if i == 0 {
			// 文章覆盖：2 篇文章引用首个 gpu tag（挂算力）。
			for j := 0; j < 2; j++ {
				feed := models.Feed{Title: "feed-bh", URL: "https://example.com/bh/" + tag.Slug + "-" + string(rune('0'+j))}
				require.NoError(t, db.Create(&feed).Error)
				article := models.Article{FeedID: feed.ID, Title: "art", CreatedAt: time.Now()}
				require.NoError(t, db.Create(&article).Error)
				require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID}).Error)
			}
		}
	}
	// 两个 tag 同时携带「大模型」（已挂载）与「算力」：算力的 member_tags 计数。
	duoTag := createTopicTagRow(t, db, "duo-tag")
	linkAuxToTag(t, db, duoTag.ID, mounted.ID)
	linkAuxToTag(t, db, duoTag.ID, uncaptured.ID)
	matchTagToBoard(t, db, duoTag.ID, board.ID)
	// disabled 携带 tag：匹配了版块也不进候选。
	disabledTag := createTopicTagRow(t, db, "privacy-tag")
	linkAuxToTag(t, db, disabledTag.ID, disabled.ID)
	matchTagToBoard(t, db, disabledTag.ID, board.ID)

	candidates, err := UncapturedCandidates(ctx, db, board.ID)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "mounted and disabled labels excluded; only 算力 remains")
	require.Equal(t, uncaptured.ID, candidates[0].AuxID)
	require.Equal(t, "算力", candidates[0].Label)
	require.Equal(t, 4, candidates[0].MemberTags, "3 gpu tags + duo tag")
	require.Equal(t, 2, candidates[0].MemberArticles)
}

// 主链路步 8：每轮落一条快照 + 历史快照不被覆盖 + 趋势读取。
func TestSnapshotPersistAndTrendRead(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "趋势板", "active")

	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		score := 50.0 + float64(i)
		comment := "评语"
		snapshot := tagmodels.BoardHealthSnapshot{
			SemanticBoardID: board.ID, CheckedAt: base.AddDate(0, 0, i),
			Metrics: tagmodels.BoardHealthMetrics{
				Board:  tagmodels.BoardHealthBoardMetrics{MatchedTags: i + 1, ReasonDist: map[string]int{}},
				Labels: []tagmodels.BoardHealthLabelMetrics{},
			},
			UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
			LLMScore:             &score, LLMComment: &comment,
		}
		require.NoError(t, db.Create(&snapshot).Error)
	}

	latest, err := LatestSnapshot(ctx, db, board.ID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.InDelta(t, 52.0, *latest.LLMScore, 1e-9, "latest = newest checked_at")

	// 体检数据查询：latest + 趋势升序（spec: 按时间升序的最近 N 条）。
	viewLatest, trend, err := HealthCheckupView(ctx, db, board.ID, 7)
	require.NoError(t, err)
	require.NotNil(t, viewLatest)
	require.Len(t, trend, 3)
	require.Less(t, trend[0].CheckedAt.Unix(), trend[2].CheckedAt.Unix(), "ascending order")
	require.InDelta(t, 52.0, *trend[2].LLMScore, 1e-9)

	// 版块不存在 → ErrBoardNotFound（404 语义）。
	_, _, err = HealthCheckupView(ctx, db, 999999, 7)
	require.ErrorIs(t, err, ErrBoardNotFound)

	// disabled / 非 board → 空态（非错误；disabled 不是 404）。
	disabledBoard := createBoardRow(t, db, "停用板", "disabled")
	emptyLatest, emptyTrend, err := HealthCheckupView(ctx, db, disabledBoard.ID, 7)
	require.NoError(t, err)
	require.Nil(t, emptyLatest)
	require.Empty(t, emptyTrend)

	// 无快照的活跃版块 → 空态（spec: 无快照返回空态）。
	freshBoard := createBoardRow(t, db, "新板", "active")
	freshLatest, freshTrend, err := HealthCheckupView(ctx, db, freshBoard.ID, 7)
	require.NoError(t, err)
	require.Nil(t, freshLatest)
	require.Empty(t, freshTrend)
}

// 变体「重复：同日两轮体检」：两条快照并存，趋势原样展示。
func TestSameDayDuplicateRoundsCoexist(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "同日板", "active")
	now := time.Now()
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
			SemanticBoardID: board.ID, CheckedAt: now,
			Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}, Labels: []tagmodels.BoardHealthLabelMetrics{}},
			UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
		}).Error)
	}
	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&count).Error)
	require.Equal(t, int64(2), count)
	_, trend, err := HealthCheckupView(context.Background(), db, board.ID, 7)
	require.NoError(t, err)
	require.Len(t, trend, 2)
}

// 用例来源：test-cases.md 主链路步 3（贴合度分布）PG 落点——锁定
// loadCompositionLabels 的 embedding 列映射。2026-10-07 回归：结构体字段
// 缺 gorm:"column:embedding" 标签时蛇形名对不上查询列名，向量永远扫空 →
// 全标签 insufficient（真库 12 版块 max_samples=0 才暴露）。
func TestLoadCompositionLabelsEmbeddingMapping(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "映射板", "active")

	withVec := createAuxLabel(t, db, "推理", "active")
	noVec := createAuxLabel(t, db, "无向量", "active")
	for _, aux := range []models.SemanticLabel{withVec, noVec} {
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: aux.ID}).Error)
	}
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0,1]'::vector WHERE id = ?`, withVec.ID).Error)

	labels, auxEmb, err := loadCompositionLabels(context.Background(), db, board.ID)
	require.NoError(t, err)
	require.Len(t, labels, 2)
	require.Contains(t, auxEmb, withVec.ID, "embedding column must map into the aux vector map")
	require.Equal(t, []float64{1, 0, 1}, auxEmb[withVec.ID])
	require.NotContains(t, auxEmb, noVec.ID)
}

// 4.1 M1：构成里混有组合标签（board_composition.auxiliary_label_id 列复用），
// label_type 必须逐条透出——前端据此把组合标签的「重写描述」标为不可执行
// （描述修复链全程只认 auxiliary），避免用户点进一条永远 404 的修复路径。
func TestLoadCompositionLabelsCarriesLabelType(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "类型透传板", "active")

	aux := createAuxLabel(t, db, "推理", "active")
	composite := models.SemanticLabel{Label: "推理 × 芯片", Slug: "bh-composite-1", LabelType: "composite", Status: "active"}
	require.NoError(t, db.Create(&composite).Error)
	for _, label := range []models.SemanticLabel{aux, composite} {
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: label.ID}).Error)
	}

	labels, _, err := loadCompositionLabels(context.Background(), db, board.ID)
	require.NoError(t, err)
	require.Len(t, labels, 2)
	typeByID := map[uint]string{}
	for _, label := range labels {
		typeByID[label.AuxID] = label.LabelType
	}
	require.Equal(t, "auxiliary", typeByID[aux.ID])
	require.Equal(t, "composite", typeByID[composite.ID])
}

// ── 三病信号 PG 落点（board-health-triage-advice seam A 1.2/1.4）──
// 用例来源：test-cases 主链路 A1/A6 + 白盒「最近邻」「磁铁：流向」组。
// 最近邻与流向都走真实 pgvector/聚合 SQL——按 testing.md 红线只跑临时
// testcontainer，禁 SQLite。

// vectorColumnDim returns the golden-schema dimension of a vector column
// (0 = untyped `vector`). The golden schema fixes topic_tag_embeddings at a
// deployment dimension — seeds must match it or pgvector rejects the insert.
func vectorColumnDim(t *testing.T, db *gorm.DB, table, column string) int {
	t.Helper()
	var formatted string
	require.NoError(t, db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = ? AND a.attname = ?`, table, column).Scan(&formatted).Error)
	trimmed := strings.TrimSuffix(strings.TrimPrefix(formatted, "vector("), ")")
	dim, err := strconv.Atoi(strings.TrimSpace(trimmed))
	if err != nil {
		return 0
	}
	return dim
}

// unitVecLiteral returns the pgvector literal of the e_idx unit vector in
// dim dimensions (e1 = aligned anchor, e2 = orthogonal direction).
func unitVecLiteral(dim, idx int) string {
	parts := make([]string, dim)
	for i := range parts {
		if i == idx {
			parts[i] = "1"
		} else {
			parts[i] = "0"
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// blendVecLiteral returns the pgvector literal of (x, y, 0...) in dim
// dimensions — x·e1 + y·e2 with |v| = sqrt(x²+y²).
func blendVecLiteral(dim int, x, y float64) string {
	parts := make([]string, dim)
	for i := range parts {
		switch i {
		case 0:
			parts[i] = strconv.FormatFloat(x, 'f', -1, 64)
		case 1:
			parts[i] = strconv.FormatFloat(y, 'f', -1, 64)
		default:
			parts[i] = "0"
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// seedIdentityEmbedding writes an identity embedding row for a tag.
func seedIdentityEmbedding(t *testing.T, db *gorm.DB, tagID uint, vector string) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tagID, EmbeddingType: "identity", EmbeddingVec: vector,
		Dimension: len(strings.Split(strings.Trim(vector, "[]"), ",")), Model: "test",
	}).Error)
}

// matchTagToBoardMismatch writes a direction_mismatch match row.
func matchTagToBoardMismatch(t *testing.T, db *gorm.DB, tagID uint, boardID uint, mismatch bool) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagID, SemanticBoardID: boardID, Score: 0.7, MatchReason: "max_sim",
		DirectionMismatch: mismatch,
	}).Error)
}

// TestQueryAuxNeighbors —— spec「最近邻证据含 cosine 与排除自身」+ 白盒
// 「3/2/0 条、排除自身/disabled/非 aux、缺自身向量」。
func TestQueryAuxNeighbors(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	self := createAuxLabel(t, db, "上海", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0]'::vector WHERE id = ?`, self.ID).Error)
	near := createAuxLabel(t, db, "举办地点", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[0.96,0.28]'::vector WHERE id = ?`, near.ID).Error)
	other := createAuxLabel(t, db, "小商品城", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[0,1]'::vector WHERE id = ?`, other.ID).Error)
	// 排除项：disabled aux（有向量）、非 aux（board 型，有向量）。
	disabledAux := createAuxLabel(t, db, "已停用", "disabled")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[0.99,0.14]'::vector WHERE id = ?`, disabledAux.ID).Error)
	boardLabel := models.SemanticLabel{Label: "板块不是aux", Slug: "bh-nbr-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardLabel).Error)
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[0.995,0.1]'::vector WHERE id = ?`, boardLabel.ID).Error)

	neighbors, err := queryAuxNeighbors(ctx, db, self.ID, []float64{1, 0}, 3)
	require.NoError(t, err)
	// 空间内只剩 2 个 active aux（近/远），disabled 与 board 型排除。
	require.Len(t, neighbors, 2)
	require.Equal(t, near.ID, neighbors[0].AuxID, "cosine 降序")
	require.Equal(t, "举办地点", neighbors[0].Label)
	require.InDelta(t, 0.96, neighbors[0].Cosine, 1e-6)
	require.Equal(t, other.ID, neighbors[1].AuxID)
	require.InDelta(t, 0.0, neighbors[1].Cosine, 1e-6)

	// k=1 截 top1；k 超过空间仍只返回实有条数。
	one, err := queryAuxNeighbors(ctx, db, self.ID, []float64{1, 0}, 1)
	require.NoError(t, err)
	require.Len(t, one, 1)

	// 「0 条」：空间里只有自身 → 计算结果为空（非 nil 语义由调用方区分）。
	solo := createAuxLabel(t, db, "孤标签", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0]'::vector WHERE id = ?`, solo.ID).Error)
	// 清掉其它 active aux 向量，使 solo 空间仅剩自身。
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = NULL WHERE id IN (?, ?, ?)`, self.ID, near.ID, other.ID).Error)
	zero, err := queryAuxNeighbors(ctx, db, solo.ID, []float64{1, 0}, 3)
	require.NoError(t, err)
	require.Empty(t, zero)
}

// TestLoadNeighborsForLabelsVectorMissing —— 缺自身向量标签近邻保持 nil
// （信号显式不足，不硬算）。
func TestLoadNeighborsForLabelsVectorMissing(t *testing.T) {
	db := setupTestDB(t)
	withVec := createAuxLabel(t, db, "有向量", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0]'::vector WHERE id = ?`, withVec.ID).Error)
	noVec := createAuxLabel(t, db, "无向量", "active")
	target := createAuxLabel(t, db, "邻居", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[0.9,0.44]'::vector WHERE id = ?`, target.ID).Error)

	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: withVec.ID, Label: "有向量"}, {AuxID: noVec.ID, Label: "无向量"},
	}
	require.NoError(t, loadNeighborsForLabels(context.Background(), db, labels,
		map[uint][]float64{withVec.ID: {1, 0}})) // noVec 无自身向量 → 不进 auxEmb
	require.Len(t, labels[0].Neighbors, 1)
	require.Equal(t, target.ID, labels[0].Neighbors[0].AuxID)
	require.Nil(t, labels[1].Neighbors, "缺自身向量：近邻信号不足（nil，非空数组）")
}

// TestLoadFailedFlow —— 白盒「磁铁：流向」：对手有效挂载 top1、失败对手挂载
// 不作可靠流向、无可靠流向显式未知、无失败行 nil。
func TestLoadFailedFlow(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "磁铁板", "active")

	failedA := createTopicTagRow(t, db, "flow-a")
	failedB := createTopicTagRow(t, db, "flow-b")
	matchTagToBoardMismatch(t, db, failedA.ID, board.ID, true)
	matchTagToBoardMismatch(t, db, failedB.ID, board.ID, true)

	// 对手板 F：两个失败 tag 都在 F 有有效挂载（direction_mismatch=false）。
	rival := createBoardRow(t, db, "对手板F", "active")
	matchTagToBoardMismatch(t, db, failedA.ID, rival.ID, false)
	matchTagToBoardMismatch(t, db, failedB.ID, rival.ID, false)
	// 板 H：failedA 在 H 的挂载本身也是 direction_mismatch=true —— 不可靠
	// 流向，不得计入/抢 top1。
	mismatchBoard := createBoardRow(t, db, "失败板H", "active")
	matchTagToBoardMismatch(t, db, failedA.ID, mismatchBoard.ID, true)
	// disabled 目标板：failedB 的有效挂载落 disabled 板，不进可靠流向。
	disabledDest := createBoardRow(t, db, "停用板D", "disabled")
	matchTagToBoardMismatch(t, db, failedB.ID, disabledDest.ID, false)

	flow, err := loadFailedFlow(ctx, db, board.ID, []uint{failedA.ID, failedB.ID})
	require.NoError(t, err)
	require.NotNil(t, flow)
	require.Equal(t, 2, flow.FailedTags)
	require.Equal(t, 2, flow.FlowTags, "两个失败 tag 都在对手板 F 有有效挂载（H 的失败挂载不算）")
	require.NotNil(t, flow.TopBoard)
	require.Equal(t, "对手板F", *flow.TopBoard)
	require.Equal(t, 2, flow.TopTags)

	// 显式未知：失败 tag 的其他挂载全是 direction_mismatch（或无其他挂载）。
	board2 := createBoardRow(t, db, "未知板", "active")
	failedC := createTopicTagRow(t, db, "flow-c")
	matchTagToBoardMismatch(t, db, failedC.ID, board2.ID, true)
	matchTagToBoardMismatch(t, db, failedC.ID, mismatchBoard.ID, true)
	flow2, err := loadFailedFlow(ctx, db, board2.ID, []uint{failedC.ID})
	require.NoError(t, err)
	require.NotNil(t, flow2)
	require.Equal(t, 1, flow2.FailedTags)
	require.Zero(t, flow2.FlowTags)
	require.Nil(t, flow2.TopBoard, "无可靠流向 → 显式未知（top_board=null），不伪造目标")

	// 无失败行 → nil。
	flow3, err := loadFailedFlow(ctx, db, board2.ID, nil)
	require.NoError(t, err)
	require.Nil(t, flow3)
}

// TestCollectBoardStatisticsSignals —— 主链路 A1：一轮统计里三类信号与
// 流向同轮产出（纯读）；分布口径不变。
func TestCollectBoardStatisticsSignals(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "信号板", "active")

	// 向量维度跟随 golden schema（topic_tag_embeddings 为定长 vector(N)，
	// semantic_labels 不定长——两侧统一用同维单位向量）。
	dim := vectorColumnDim(t, db, "topic_tag_embeddings", "embedding")
	if dim == 0 {
		dim = 2
	}

	// 构成标签 A（有向量，desc 带事件词根）+ B（缺向量）。
	auxA := createAuxLabel(t, db, "磁铁嫌疑", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = ?::vector, description = '上海知名商业综合体，此次举办活动' WHERE id = ?`, unitVecLiteral(dim, 0), auxA.ID).Error)
	auxB := createAuxLabel(t, db, "缺向量", "active")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxB.ID}).Error)
	// 邻居空间：近邻 aux（与 e1 的 cosine=0.96）。
	neighbor := createAuxLabel(t, db, "举办地点", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = ?::vector WHERE id = ?`, blendVecLiteral(dim, 0.96, 0.28), neighbor.ID).Error)

	// 6 个正交挂载（cos=0）+ 5 个对齐失败挂载（cos=1, mismatch=true）——
	// 新口径下失败子集均值高于健康子集（磁铁形态）。
	for i := 0; i < 6; i++ {
		tag := createTopicTagRow(t, db, fmt.Sprintf("ok-tag-%d", i))
		seedIdentityEmbedding(t, db, tag.ID, unitVecLiteral(dim, 1))
		matchTagToBoardMismatch(t, db, tag.ID, board.ID, false)
	}
	rival := createBoardRow(t, db, "流向板", "active")
	for i := 0; i < 5; i++ {
		tag := createTopicTagRow(t, db, fmt.Sprintf("failed-tag-%d", i))
		seedIdentityEmbedding(t, db, tag.ID, unitVecLiteral(dim, 0))
		matchTagToBoardMismatch(t, db, tag.ID, board.ID, true)
		matchTagToBoardMismatch(t, db, tag.ID, rival.ID, false) // 失败 tag 在对手板有效挂载
	}

	metrics, candidates, err := collectBoardStatistics(ctx, db, &board,
		Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	require.NoError(t, err)
	require.Empty(t, candidates)

	byID := labelMetricsByAuxID(metrics.Labels)
	a := byID[auxA.ID]
	// 磁铁（新口径）：good_mean=0（6 个方向通过挂载）、failed_mean=1（5 个失败
	// 挂载）→ 失败侧显著高于健康侧 → 嫌疑成立。
	require.NotNil(t, a.Magnet)
	require.False(t, a.Magnet.Insufficient)
	require.True(t, a.Magnet.Suspicious)
	require.InDelta(t, 0.0, *a.Magnet.GoodMean, 1e-9)
	require.InDelta(t, 1.0, *a.Magnet.FailedMean, 1e-9)
	require.Equal(t, 6, a.Magnet.GoodSamples)
	require.Equal(t, 5, a.Magnet.FailedSamples)
	// all 侧保留但退出判定。
	require.InDelta(t, 5.0/11.0, *a.Magnet.AllMean, 1e-9)
	require.Equal(t, 11, a.Magnet.AllSamples)
	// 近邻：active aux 空间 top-3（排除自身；缺向量 aux 不在向量空间）。
	require.Len(t, a.Neighbors, 1)
	require.Equal(t, neighbor.ID, a.Neighbors[0].AuxID)
	require.InDelta(t, 0.96, a.Neighbors[0].Cosine, 1e-6)
	// desc 预筛命中「此次」。
	require.Equal(t, []string{"此次"}, a.DescFlags)
	require.Contains(t, a.Description, "上海知名商业综合体")

	// 缺向量标签：磁铁/近邻均显式不足。
	b := byID[auxB.ID]
	require.NotNil(t, b.Magnet)
	require.True(t, b.Magnet.Insufficient)
	require.False(t, b.Magnet.Suspicious)
	require.Nil(t, b.Neighbors)

	// 版块级：失败流向聚合到对手板。
	require.NotNil(t, metrics.Board.MagnetFlow)
	require.Equal(t, 5, metrics.Board.MagnetFlow.FailedTags)
	require.Equal(t, 5, metrics.Board.MagnetFlow.FlowTags)
	require.NotNil(t, metrics.Board.MagnetFlow.TopBoard)
	require.Equal(t, "流向板", *metrics.Board.MagnetFlow.TopBoard)
	// 既有口径不变：matched=11、ghost=5、vector_missing=0。
	require.Equal(t, 11, metrics.Board.MatchedTags)
	require.Equal(t, 5, metrics.Board.GhostMatches)
	require.Equal(t, 0, metrics.Board.VectorMissing)
}

// TestCheckupRoundPersistsSignalsAndTriage —— 主链路 A2/A3/A6 的 PG 落点：
// triage 随快照落库；坏裁决 score/comment/triage 空而信号统计照落；体检零写。
func TestCheckupRoundPersistsSignalsAndTriage(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "分诊板", "active")

	dim := vectorColumnDim(t, db, "topic_tag_embeddings", "embedding")
	if dim == 0 {
		dim = 2
	}
	aux := createAuxLabel(t, db, "大模型", "active")
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = ?::vector, description = '大规模预训练模型' WHERE id = ?`, unitVecLiteral(dim, 0), aux.ID).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: aux.ID}).Error)
	for i := 0; i < 6; i++ {
		tag := createTopicTagRow(t, db, fmt.Sprintf("tri-tag-%d", i))
		seedIdentityEmbedding(t, db, tag.ID, unitVecLiteral(dim, 0))
		matchTagToBoardMismatch(t, db, tag.ID, board.ID, false)
	}
	config := Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5}

	// 预置匹配/建议探针：体检前后必须原样（A6 零写回归）。
	probeTag := createTopicTagRow(t, db, "probe-tag")
	matchProbe := tagmodels.TopicTagBoardLabel{TopicTagID: probeTag.ID, SemanticBoardID: board.ID, Score: 0.7, MatchReason: "direct_hit"}
	require.NoError(t, db.Create(&matchProbe).Error)
	suggestion := tagmodels.BoardUpgradeSuggestion{SuggestionHash: "triage-readonly-probe", Status: "pending", Mode: "expand:aux", Decision: "merge_into_existing", BoardLabel: "分诊板", TargetBoardID: &board.ID}
	require.NoError(t, db.Create(&suggestion).Error)

	// 正常路：合法 triage 覆盖唯一构成标签 → 随 metrics 落库。
	okLLM := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 75, Comment: "总体健康", Notify: false,
		Triage: []TriageItem{{AuxID: aux.ID, Category: "keep", Evidence: "cosine_mean=1.000 且无磁铁嫌疑"}},
	}}
	require.NoError(t, CheckupBoard(ctx, db, okLLM, config, &board, time.Now()))
	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)
	require.NotNil(t, snapshot.LLMScore)
	require.Len(t, snapshot.Metrics.Triage, 1)
	require.Equal(t, aux.ID, snapshot.Metrics.Triage[0].AuxID)
	require.Equal(t, "keep", snapshot.Metrics.Triage[0].Category)
	require.Equal(t, "cosine_mean=1.000 且无磁铁嫌疑", snapshot.Metrics.Triage[0].Evidence)
	// 信号同轮落库。
	require.NotNil(t, snapshot.Metrics.Labels[0].Magnet)
	require.NotNil(t, snapshot.Metrics.Labels[0].Neighbors)
	require.Nil(t, snapshot.Metrics.Board.MagnetFlow, "零失败挂载 → 流向 null")

	// 坏裁决（未知 aux id 的 triage）→ 整体裁决降级：score/comment/triage
	// 空，信号统计照落，不重试。
	badLLM := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 70, Comment: "x",
		Triage: []TriageItem{{AuxID: 999999, Category: "remove", Evidence: "幻觉"}},
	}}
	require.NoError(t, CheckupBoard(ctx, db, badLLM, config, &board, time.Now()))
	var degraded tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ? AND llm_score IS NULL", board.ID).Order("id DESC").First(&degraded).Error)
	require.Nil(t, degraded.LLMScore)
	require.Nil(t, degraded.LLMComment)
	require.Nil(t, degraded.Metrics.Triage, "坏裁决 triage 不得透出可执行建议")
	require.NotNil(t, degraded.Metrics.Labels[0].Magnet, "降级轮信号统计照常落库")
	require.NotNil(t, degraded.Metrics.Labels[0].Neighbors, "近邻已计算（非 nil）")
	require.Empty(t, degraded.Metrics.Labels[0].Neighbors, "active aux 向量空间内无其他标签 → 空清单")

	// 零写回归：匹配行/建议行原样，快照数=2。
	var afterMatch tagmodels.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", probeTag.ID).First(&afterMatch).Error)
	require.Equal(t, 0.7, afterMatch.Score)
	require.Equal(t, "direct_hit", afterMatch.MatchReason)
	require.False(t, afterMatch.DirectionMismatch)
	var pendingCount int64
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Where("status = ?", "pending").Count(&pendingCount).Error)
	require.Equal(t, int64(1), pendingCount, "体检不得创建/修改升级建议")
	var snapCount int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&snapCount).Error)
	require.Equal(t, int64(2), snapCount)
}

// TestCheckupRoundPersistsFilledTriage —— 补齐后的 triage 落库可读
// （fix-board-health-triage-degrade 6.6）：LLM 漏判的构成标签以
// insufficient 落库（不伪造可执行建议），清单长度 == 构成标签数。
func TestCheckupRoundPersistsFilledTriage(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "补齐板", "active")

	auxIDs := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		aux := createAuxLabel(t, db, fmt.Sprintf("补齐%d", i), "active")
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: aux.ID}).Error)
		auxIDs = append(auxIDs, aux.ID)
	}

	// LLM 只回 2 条（2/3 = 66.7% ≥ 下限）→ 补 1 条。
	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 66, Comment: "部分证据不足",
		Triage: []TriageItem{
			{AuxID: auxIDs[0], Category: "keep", Evidence: "cosine_mean=0.71"},
			{AuxID: auxIDs[2], Category: "remove", Evidence: "magnet failed=0.72 good=0.52 failed_n=40"},
		},
	}}
	require.NoError(t, CheckupBoard(ctx, db, llm, Config{}, &board, time.Now()))

	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)
	require.NotNil(t, snapshot.LLMScore, "漏判不得作废整轮裁决")
	require.Len(t, snapshot.Metrics.Triage, 3, "补齐后 triage 长度 == 构成标签数")

	categories := map[string]int{}
	for _, item := range snapshot.Metrics.Triage {
		categories[item.Category]++
		if item.Category == "insufficient" {
			require.Empty(t, item.Suggestion, "补齐项不得携带建议文案")
		}
	}
	// 两条 insufficient：① 漏判补齐项；② 摘除项被 G2 降级（本板构成标签
	// 无向量 → 磁铁信号不足，摘除 MUST NOT 成立）。
	require.Equal(t, 2, categories["insufficient"])
	require.Equal(t, 1, categories["keep"])
	require.Zero(t, categories["remove"], "无成立磁铁信号的摘除建议不得原样落库")
}
