package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── board_persistent_topic_constraints（topic-lane-auxiliary-label-constraint）──
//
// 本文件承载 tasks.md 第 12 组映射表里落在 repository 的 Scenario：
//   添加约束写入完整行 / 模式取值受约束 / margin 与 max_distance 越界受约束 /
//   同一话题内短词重复被拒 / 话题删除级联清理约束 /
//   约束违例不减少命中计数 / 增删约束不改变话题状态
// 外加 tasks 1.3 的仓储契约（全量替换幂等、非法项整体回滚）与 tasks 4.1 的
// 判定输入来源（cluster_tag_ids 悬空时仍能取到有效 tag 集合）。
// PG testcontainer，不用 SQLite（repository 层禁止 SQLite——CHECK / 唯一索引 /
// 外键级联都是本 change 的契约本体，SQLite 复现不了）。

func seedConstraintTopic(t *testing.T, db *gorm.DB, boardID uint, label, status string, hitCount int) BoardPersistentTopic {
	t.Helper()
	now := NormalizeReportDate(time.Now())
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   now,
		LastSeenDate:    now,
		HitCount:        hitCount,
		ConsecutiveHits: 0,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

func floatPtr(v float64) *float64 { return &v }

// TestTopicLaneConstraint_AddWritesFullRow covers Scenario「添加约束写入完整行」.
func TestTopicLaneConstraint_AddWritesFullRow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "以黎冲突升级：停火破裂与军事打击", TopicStatusActive, 1)

	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{{
		Term:           "以色列",
		Mode:           LaneConstraintModeRequired,
		TermEmbedding:  FloatsToPgVector([]float64{1, 0, 0}),
		EmbeddingModel: "qwen3-embedding",
	}}))

	rows, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, topic.ID, rows[0].TopicID)
	assert.Equal(t, "以色列", rows[0].Term)
	assert.Equal(t, "qwen3-embedding", rows[0].EmbeddingModel)
	assert.Equal(t, LaneConstraintModeRequired, rows[0].Mode)
	// margin / max_distance 留空 = 取 ai_settings 全局默认（design D4）.
	assert.Nil(t, rows[0].Margin)
	assert.Nil(t, rows[0].MaxDistance)
	// 锤向量确实落库（判定要用），但不是 API 字段。
	assert.NotEmpty(t, rows[0].TermEmbedding)
}

// TestTopicLaneConstraint_ModeCheckConstraint covers Scenario「模式取值受约束」.
// The row is inserted through the model (bypassing the Go validator) so the DB
// CHECK constraint itself is what rejects it.
func TestTopicLaneConstraint_ModeCheckConstraint(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "受约束话题", TopicStatusActive, 1)

	row := TopicLaneConstraint{
		TopicID:        topic.ID,
		Term:           "以色列",
		TermEmbedding:  FloatsToPgVector([]float64{1, 0, 0}),
		EmbeddingModel: "qwen3-embedding",
		Mode:           "optional",
	}
	err := db.Create(&row).Error
	require.Error(t, err, "CHECK (mode IN ('required','preferred')) SHALL reject 'optional'")
	assert.Contains(t, err.Error(), "ck_board_persistent_topic_constraints_mode")
}

// TestTopicLaneConstraint_MarginMaxDistanceCheck covers Scenario
// 「margin 与 max_distance 越界受约束」.
func TestTopicLaneConstraint_MarginMaxDistanceCheck(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "越界话题", TopicStatusActive, 1)

	badMargin := TopicLaneConstraint{
		TopicID:        topic.ID,
		Term:           "以色列",
		TermEmbedding:  FloatsToPgVector([]float64{1, 0, 0}),
		EmbeddingModel: "qwen3-embedding",
		Mode:           LaneConstraintModeRequired,
		Margin:         floatPtr(1.5),
	}
	err := db.Create(&badMargin).Error
	require.Error(t, err, "margin=1.5 SHALL be rejected")
	assert.Contains(t, err.Error(), "ck_board_persistent_topic_constraints_margin")

	badMaxDistance := TopicLaneConstraint{
		TopicID:        topic.ID,
		Term:           "黎巴嫩",
		TermEmbedding:  FloatsToPgVector([]float64{0, 1, 0}),
		EmbeddingModel: "qwen3-embedding",
		Mode:           LaneConstraintModeRequired,
		MaxDistance:    floatPtr(2.0),
	}
	err = db.Create(&badMaxDistance).Error
	require.Error(t, err, "max_distance=2.0 SHALL be rejected")
	assert.Contains(t, err.Error(), "ck_board_persistent_topic_constraints_max_distance")
}

// TestTopicLaneConstraint_DuplicateTermRejected covers Scenario
// 「同一话题内短词重复被拒」— the unique index is the last line of defence
// behind the Go-side duplicate check (both must hold).
func TestTopicLaneConstraint_DuplicateTermRejected(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "重复短词话题", TopicStatusActive, 1)

	first := TopicLaneConstraint{
		TopicID:        topic.ID,
		Term:           "以色列",
		TermEmbedding:  FloatsToPgVector([]float64{1, 0, 0}),
		EmbeddingModel: "qwen3-embedding",
		Mode:           LaneConstraintModeRequired,
	}
	require.NoError(t, db.Create(&first).Error)

	second := first
	second.ID = 0
	err := db.Create(&second).Error
	require.Error(t, err, "duplicate (topic_id, term) SHALL be rejected")
	assert.Contains(t, err.Error(), "uq_board_persistent_topic_constraints_topic_term")

	// The Go-side validator rejects it before the DB is touched, and no second
	// row appears.
	err = repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
		{Term: "以色列", Mode: LaneConstraintModePreferred, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
	})
	require.Error(t, err)
	rows, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// TestTopicLaneConstraint_TopicDeleteCascades covers Scenario
// 「话题删除级联清理约束」.
func TestTopicLaneConstraint_TopicDeleteCascades(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "级联话题", TopicStatusActive, 1)

	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
		{Term: "黎巴嫩", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{0, 1, 0}), EmbeddingModel: "qwen3-embedding"},
	}))
	require.NoError(t, repo.DeleteTopic(topic.ID))

	var left int64
	require.NoError(t, db.Model(&TopicLaneConstraint{}).Where("topic_id = ?", topic.ID).Count(&left).Error)
	assert.Zero(t, left, "topic delete SHALL cascade-delete its constraints")
}

// TestTopicLaneConstraint_ReplaceIsIdempotent covers tasks 1.3 / Scenario
// 「批量替换幂等」at the repository level: two identical submissions leave the
// same set, and the omitted entry is gone after the first one.
func TestTopicLaneConstraint_ReplaceIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "幂等话题", TopicStatusActive, 1)

	seed := []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
		{Term: "黎巴嫩", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{0, 1, 0}), EmbeddingModel: "qwen3-embedding"},
	}
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, seed))
	before, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, before, 2)

	// "只保留以色列" — 两次提交完全相同。
	keep := []LaneConstraintInput{{
		Term: "以色列", Mode: LaneConstraintModeRequired,
		TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding",
	}}
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, keep))
	first, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, keep))
	second, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)

	require.Len(t, first, 1, "omitted term is deleted")
	require.Len(t, second, 1)
	assert.Equal(t, first[0].ID, second[0].ID, "repeat submission updates in place (id preserved)")
	assert.Equal(t, first[0].Term, second[0].Term)
	assert.Equal(t, first[0].Mode, second[0].Mode)
	assert.Equal(t, "以色列", first[0].Term)
}

// TestTopicLaneConstraint_InvalidItemRollsBackWholeReplace covers Scenario
// 「非法载荷不部分写入」at the repository level: a valid + an invalid entry must
// leave the stored set exactly as it was.
func TestTopicLaneConstraint_InvalidItemRollsBackWholeReplace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "回滚话题", TopicStatusActive, 1)

	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
	}))

	err := repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{
		{Term: "黎巴嫩", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{0, 1, 0}), EmbeddingModel: "qwen3-embedding"},
		{Term: "真主党", Mode: "optional", TermEmbedding: FloatsToPgVector([]float64{0, 0, 1}), EmbeddingModel: "qwen3-embedding"},
	})
	require.Error(t, err)

	rows, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "SHALL NOT partially write")
	assert.Equal(t, "以色列", rows[0].Term)
}

// TestTopicLaneConstraint_DoesNotTouchTopicLifecycle covers the two lifecycle
// Scenarios: 「约束违例不减少命中计数」/「增删约束不改变话题状态」.
func TestTopicLaneConstraint_DoesNotTouchTopicLifecycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "生命周期话题", TopicStatusActive, 10)
	// consecutive_hits 先置非 0，让「约束违例 = 未命中」在 planLifecycle 里真的
	// 产生一条变更（否则 miss 分支不触发，断言会变成空转）。
	require.NoError(t, db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("consecutive_hits", 2).Error)

	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
		{Term: "黎巴嫩", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{0, 1, 0}), EmbeddingModel: "qwen3-embedding"},
	}))
	after, err := repo.GetTopic(topic.ID)
	require.NoError(t, err)
	assert.Equal(t, 10, after.HitCount)
	assert.Equal(t, TopicStatusActive, after.Status, "增删约束 SHALL NOT 改变话题状态")

	// 约束违例 = 该 section 没有归属该话题，不构成一次「未命中」：hit_count 原地不动
	// （planLifecycle 只更新计数，miss 只重置 consecutive_hits）。
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, nil))
	topics, err := repo.ListAllTopicsByBoard(boardID)
	require.NoError(t, err)
	changes := planLifecycle(topics, NormalizeReportDate(time.Now()), map[uint]bool{})
	var found bool
	for _, ch := range changes {
		if ch.topicID == topic.ID {
			found = true
			assert.Equal(t, 10, ch.hitCount, "constraint violation SHALL NOT decrement hit_count")
			assert.Equal(t, 0, ch.consecutiveHits, "未命中只重置 consecutive_hits（既有规则）")
			assert.Equal(t, TopicStatusActive, ch.status, "planLifecycle SHALL NOT change status")
		}
	}
	require.True(t, found, "miss 分支必须产生一条变更，否则本条断言是空转")
}

// TestResolveSectionValidTagIDs_IgnoresDanglingClusterTagIDs covers tasks 4.1:
// the judgement input MUST come from article_topic_tags, so a deleted tag id
// left in cluster_tag_ids can never enter the tag set.
func TestResolveSectionValidTagIDs_IgnoresDanglingClusterTagIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	topic := seedConstraintTopic(t, db, boardID, "判定输入话题", TopicStatusActive, 1)

	liveTag := seedTestTopicTag(t, db, "真主党与以军交火", "active")
	const danglingTagID = 999999 // 不在 topic_tags 里——模拟 7 天 GC 后的悬空 id

	feed := models.Feed{Title: "constraint-feed", URL: "https://example.com/constraint-feed"}
	require.NoError(t, db.Create(&feed).Error)
	pub := now
	article := models.Article{FeedID: feed.ID, Title: "以军空袭黎南部", Link: "https://example.com/c/1", PubDate: &pub}
	require.NoError(t, db.Create(&article).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: liveTag}).Error)

	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSectionWithTopic(t, db, reportID, "以黎冲突", topic.ID, liveTag, danglingTagID)
	relatedIDs, err := json.Marshal([]uint{article.ID})
	require.NoError(t, err)
	// embedding 是 vector 列，零值 "" 不是合法字面量 → 省略列落 NULL。
	require.NoError(t, db.Omit("embedding").Create(&DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "线索",
		TagIDs: JSON(`[]`), RelatedArticleIDs: JSON(relatedIDs), Confidence: 0.9,
	}).Error)

	resolved, err := repo.ResolveSectionValidTagIDs([]uint{sectionID})
	require.NoError(t, err)
	assert.Equal(t, []uint{liveTag}, resolved[sectionID], "dangling cluster_tag_id SHALL NOT survive")
}

// TestTopicLaneConstraint_GlobalDefaultsFromAISettings covers design D4: the
// margin / max_distance defaults are online-tunable through ai_settings, so a
// row that leaves them empty follows the global value.
func TestTopicLaneConstraint_GlobalDefaultsFromAISettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	require.NoError(t, db.Model(&models.AISettings{}).
		Where("key = ?", "persistent_topic_constraint_margin_default").
		Update("value", "0.08").Error)
	require.NoError(t, db.Model(&models.AISettings{}).
		Where("key = ?", "persistent_topic_constraint_max_distance_default").
		Update("value", "0.42").Error)

	cfg := LoadPersistentTopicConfig(db)
	assert.InDelta(t, 0.08, cfg.ConstraintMarginDefault, 1e-9)
	assert.InDelta(t, 0.42, cfg.ConstraintMaxDistanceDefault, 1e-9)
}

// TestTopicLaneConstraint_ListByTopicIDsBatch covers the batch read the lane
// pipeline uses (one query per report): topics without constraints stay absent
// so the gate passes them through untouched.
func TestTopicLaneConstraint_ListByTopicIDsBatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	constrained := seedConstraintTopic(t, db, boardID, "有约束话题", TopicStatusActive, 1)
	plain := seedConstraintTopic(t, db, boardID, "无约束话题", TopicStatusActive, 1)

	require.NoError(t, repo.ReplaceTopicLaneConstraints(constrained.ID, []LaneConstraintInput{
		{Term: "以色列", Mode: LaneConstraintModeRequired, TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding"},
	}))

	got, err := repo.ListTopicLaneConstraintsByTopicIDs([]uint{constrained.ID, plain.ID})
	require.NoError(t, err)
	require.Contains(t, got, constrained.ID)
	assert.NotContains(t, got, plain.ID)
	assert.Len(t, got[constrained.ID], 1)
}

// TestTopicLaneConstraint_MarginMaxDistanceRoundTrip pins the numeric round-trip:
// the columns are NUMERIC (nullable overrides, design D4), so a non-nil value must
// come back as the same float — otherwise every per-constraint override would
// silently fall back to the global default.
func TestTopicLaneConstraint_MarginMaxDistanceRoundTrip(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	topic := seedConstraintTopic(t, db, boardID, "覆写话题", TopicStatusActive, 1)

	margin, maxDistance := 0.12, 0.42
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{{
		Term:           "以色列",
		Mode:           LaneConstraintModeRequired,
		Margin:         &margin,
		MaxDistance:    &maxDistance,
		TermEmbedding:  FloatsToPgVector([]float64{1, 0, 0}),
		EmbeddingModel: "qwen3-embedding",
	}}))

	rows, err := repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Margin)
	require.NotNil(t, rows[0].MaxDistance)
	assert.InDelta(t, 0.12, *rows[0].Margin, 1e-9)
	assert.InDelta(t, 0.42, *rows[0].MaxDistance, 1e-9)

	// 显式覆写后改回「留空」= 重新取全局默认。
	require.NoError(t, repo.ReplaceTopicLaneConstraints(topic.ID, []LaneConstraintInput{{
		Term: "以色列", Mode: LaneConstraintModeRequired,
		TermEmbedding: FloatsToPgVector([]float64{1, 0, 0}), EmbeddingModel: "qwen3-embedding",
	}}))
	rows, err = repo.ListTopicLaneConstraints(topic.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Margin)
	assert.Nil(t, rows[0].MaxDistance)
}

// TestTopicLaneConstraintSchema_MatchesMigration pins the physical shape the
// migration creates (工程等价于 `psql -c "\d board_persistent_topic_constraints"`，
// 但跑在 testcontainer 里、不碰真库）：列类型/可空性、无维度限定 vector、CHECK、
// 级联外键、唯一索引与 topic_id 索引。
func TestTopicLaneConstraintSchema_MatchesMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	type column struct {
		ColumnName    string
		DataType      string
		IsNullable    string
		ColumnDefault *string
		UdtName       string
	}
	var cols []column
	require.NoError(t, db.Raw(`
		SELECT column_name, data_type, is_nullable, column_default, udt_name
		FROM information_schema.columns
		WHERE table_name = 'board_persistent_topic_constraints'
		ORDER BY ordinal_position`).Scan(&cols).Error)
	require.NotEmpty(t, cols, "migration SHALL create the table")

	byName := make(map[string]column, len(cols))
	for _, c := range cols {
		byName[c.ColumnName] = c
	}
	// term: VARCHAR(100) NOT NULL
	term, ok := byName["term"]
	require.True(t, ok)
	assert.Equal(t, "character varying", term.DataType)
	assert.Equal(t, "NO", term.IsNullable)
	// term_embedding: 无维度限定 vector、NOT NULL
	emb, ok := byName["term_embedding"]
	require.True(t, ok)
	assert.Equal(t, "USER-DEFINED", emb.DataType)
	assert.Equal(t, "vector", emb.UdtName)
	assert.Equal(t, "NO", emb.IsNullable)
	// embedding_model: VARCHAR(100) NOT NULL
	assert.Equal(t, "NO", byName["embedding_model"].IsNullable)
	// mode: NOT NULL DEFAULT 'required'
	assert.Equal(t, "NO", byName["mode"].IsNullable)
	require.NotNil(t, byName["mode"].ColumnDefault)
	assert.Contains(t, *byName["mode"].ColumnDefault, "required")
	// margin / max_distance: 可空（空 = 取全局默认）
	assert.Equal(t, "YES", byName["margin"].IsNullable)
	assert.Equal(t, "numeric", byName["margin"].DataType)
	assert.Equal(t, "YES", byName["max_distance"].IsNullable)
	assert.Equal(t, "numeric", byName["max_distance"].DataType)
	// 时间戳
	assert.Equal(t, "NO", byName["created_at"].IsNullable)
	assert.Equal(t, "NO", byName["updated_at"].IsNullable)

	// 列类型确认无维度限定：format_type 只有 "vector"，没有 "vector(N)"。
	var columnType string
	require.NoError(t, db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'board_persistent_topic_constraints' AND a.attname = 'term_embedding'`).
		Scan(&columnType).Error)
	assert.Equal(t, "vector", columnType, "向量列 SHALL 为无维度限定 vector")

	// CHECK 约束三件套
	var checks []string
	require.NoError(t, db.Raw(`
		SELECT conname FROM pg_constraint
		WHERE conrelid = 'board_persistent_topic_constraints'::regclass AND contype = 'c'
		ORDER BY conname`).Scan(&checks).Error)
	assert.ElementsMatch(t, []string{
		"ck_board_persistent_topic_constraints_mode",
		"ck_board_persistent_topic_constraints_margin",
		"ck_board_persistent_topic_constraints_max_distance",
	}, checks)

	// 外键 ON DELETE CASCADE
	var deleteRule string
	require.NoError(t, db.Raw(`
		SELECT rc.delete_rule FROM information_schema.referential_constraints rc
		WHERE rc.constraint_name = 'fk_board_persistent_topic_constraints_topic'`).Scan(&deleteRule).Error)
	assert.Equal(t, "CASCADE", deleteRule)

	// 唯一索引 (topic_id, term) + topic_id BTree 索引
	var indexes []string
	require.NoError(t, db.Raw(`SELECT indexname FROM pg_indexes
		WHERE tablename = 'board_persistent_topic_constraints' ORDER BY indexname`).Scan(&indexes).Error)
	assert.Contains(t, indexes, "uq_board_persistent_topic_constraints_topic_term")
	assert.Contains(t, indexes, "idx_board_persistent_topic_constraints_topic")
}
