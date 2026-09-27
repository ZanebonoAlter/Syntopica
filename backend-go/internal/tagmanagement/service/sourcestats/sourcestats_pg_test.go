package sourcestats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
)

// 用例来源：openspec/changes/archive/2026-09-18-add-source-board-hit-rate/test-cases.md B7
//（真库不变量抽检 5 条）。对照 docs/research/source-board-hit-rate/
// explore-findings.md 的实测快照，只断言不变量，不冻结数字。
//
// 运行方式（Docker Postgres 在跑，configs/config.yaml 的 DSN）：
//
//	go test -run TestSourceStatsPostgres ./internal/tagmanagement/service/sourcestats/ -v
//
// -short 下 skip（见 docs/reference/standard/backend/testing.md 巡检纪律）。

func TestSourceStatsPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker Postgres (real dev database); run without -short")
	}
	if err := config.LoadConfig("../../../configs"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db := database.DB
	ctx := context.Background()

	// ── TC-B7-01 全源恒等式 + TC-B7-02 去重不变量（7 天窗口）──
	stats, err := FeedBoardHitStats(ctx, db, 7)
	require.NoError(t, err)
	require.NotEmpty(t, stats, "expected real feeds in dev database")

	archiveSensitive := 0 // 有「归档命中差异」的源数（TC-B7-03 敏感性开关）

	for _, s := range stats {
		// TC-B7-01：articles == in_board + tagged_no_board + untagged_pending + untagged_settled
		require.Equal(t, s.Articles, s.InBoard+s.TaggedNoBoard+s.UntaggedPending+s.UntaggedSettled,
			"identity broken for feed %d (%s)", s.FeedID, s.Title)

		// TC-B7-02：SUM(boards[].articles) >= in_board（多板块各计一次 ≥ 去重命中数）
		var boardSum int64
		for _, b := range s.Boards {
			boardSum += b.Articles
			require.Greater(t, b.Articles, int64(0), "board entries count distinct articles")
		}
		require.GreaterOrEqual(t, boardSum, s.InBoard, "board sum invariant for feed %d", s.FeedID)

		// TC-B7-03 准备：找「真库含归档文章的源」，后续与参照 SQL 对比。
		if s.InBoard > 0 {
			var archivedHits int64
			require.NoError(t, db.WithContext(ctx).Raw(`SELECT COUNT(DISTINCT a.id)
FROM articles a
WHERE a.feed_id = ? AND a.archived = TRUE
	AND COALESCE(a.pub_date, a.created_at) >= ?
	AND EXISTS (
		SELECT 1 FROM article_topic_tags att
		JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
		JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
		WHERE att.article_id = a.id AND sl.label_type = 'board' AND sl.status = 'active')`,
				s.FeedID, time.Now().AddDate(0, 0, -7)).Scan(&archivedHits).Error)
			if archivedHits > 0 {
				archiveSensitive++
			}
		}
	}

	// ── TC-B7-03 归档口径：服务结果 == 不带 archived 过滤的参照 SQL ──
	//
	// 参照 SQL 在测试里独立写两遍（含/不含 archived 过滤）。若某个源的两种
	// 结果不同（说明真库存在「只有归档区才有命中」的文章），服务的 in_board
	// 必须等于不过滤的那份——实现若误加 archived=false 过滤，此断言必失败。
	cutoff := time.Now().AddDate(0, 0, -7)
	refWithArchived := refInBoardByFeed(ctx, t, db, cutoff, false)
	refWithoutArchived := refInBoardByFeed(ctx, t, db, cutoff, true)
	statByID := map[uint]FeedStat{}
	for _, s := range stats {
		statByID[s.FeedID] = s
	}
	for feedID, unfiltered := range refWithArchived {
		filtered := refWithoutArchived[feedID]
		if filtered < unfiltered { // 该源存在归档区命中
			s := statByID[feedID]
			require.Equal(t, unfiltered, s.InBoard,
				"feed %d: archived hits must be counted (unfiltered=%d, filtered=%d, service=%d)",
				feedID, unfiltered, filtered, s.InBoard)
			archiveSensitive++
		}
	}
	if archiveSensitive == 0 {
		t.Log("current window has no archived-range hits; archived-inclusion path not exercised by this snapshot (data-dependent, not frozen)")
	} else {
		t.Logf("archived-inclusion exercised on %d feed(s)/signal(s)", archiveSensitive)
	}

	// ── TC-B7-04 板块合计：对每个 board，Σ sources[].articles == total_articles ──
	var boardIDs []uint
	require.NoError(t, db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ?", "board").Order("id ASC").Pluck("id", &boardIDs).Error)
	require.NotEmpty(t, boardIDs)
	for _, boardID := range boardIDs {
		breakdown, err := BoardSourceBreakdown(ctx, db, boardID, 7)
		require.NoError(t, err, "board %d", boardID)
		var sum int64
		for _, s := range breakdown.Sources {
			sum += s.Articles
			require.Greater(t, s.Articles, int64(0))
		}
		require.Equal(t, breakdown.TotalArticles, sum, "board %d sources sum", boardID)
		require.Equal(t, len(breakdown.Sources), breakdown.SourceCount)
	}

	// ── TC-B7-05 窗口单调：同源 articles(7d) <= articles(30d) <= articles(90d) ──
	byWindow := map[int][]FeedStat{}
	for _, w := range []int{7, 30, 90} {
		list, err := FeedBoardHitStats(ctx, db, w)
		require.NoError(t, err)
		byWindow[w] = list
	}
	stats30 := map[uint]int64{}
	stats90 := map[uint]int64{}
	for _, s := range byWindow[30] {
		stats30[s.FeedID] = s.Articles
	}
	for _, s := range byWindow[90] {
		stats90[s.FeedID] = s.Articles
	}
	for _, s := range byWindow[7] {
		require.LessOrEqual(t, s.Articles, stats30[s.FeedID], "7d <= 30d for feed %d", s.FeedID)
		require.LessOrEqual(t, stats30[s.FeedID], stats90[s.FeedID], "30d <= 90d for feed %d", s.FeedID)
	}
}

// refInBoardByFeed 是测试内独立手写的参照聚合：每源窗口内命中文章数
// （口径同 spec 命中判定），withArchivedFilter=false 即服务的应有结果。
func refInBoardByFeed(ctx context.Context, t *testing.T, db *gorm.DB, cutoff time.Time, withArchivedFilter bool) map[uint]int64 {
	t.Helper()
	query := `SELECT a.feed_id, COUNT(DISTINCT a.id)
FROM articles a
WHERE COALESCE(a.pub_date, a.created_at) >= ?
	AND EXISTS (
		SELECT 1 FROM article_topic_tags att
		JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
		JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
		WHERE att.article_id = a.id AND sl.label_type = 'board' AND sl.status = 'active')`
	if withArchivedFilter {
		query += ` AND a.archived = FALSE`
	}
	query += ` GROUP BY a.feed_id`

	var rows []struct {
		FeedID uint
		Count  int64
	}
	require.NoError(t, db.WithContext(ctx).Raw(query, cutoff).Scan(&rows).Error)
	out := make(map[uint]int64, len(rows))
	for _, r := range rows {
		out[r.FeedID] = r.Count
	}
	return out
}
