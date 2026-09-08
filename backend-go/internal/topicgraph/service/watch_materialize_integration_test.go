package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// fakeEmbedder returns an embedFunc answering every request with the given
// vectors (one per input), bypassing the real embedding provider.
func fakeEmbedder(vecs ...[]float64) embedFunc {
	return func(_ context.Context, _ airouter.EmbeddingRequest, _ airouter.Capability) (*airouter.EmbeddingResult, error) {
		return &airouter.EmbeddingResult{Embeddings: vecs}, nil
	}
}

// seedMaterializationWorld provisions the full watch-materialization world:
// board with two aux labels ([1,0,0]-aligned "AI 编程" and orthogonal
// "传统能源"), tags/articles wired per label, plus keyword-track articles
// (one tag-less, both containing "harness"). Returns the seeded board ID.
func seedMaterializationWorld(t *testing.T, db *gorm.DB) (boardID uint, today time.Time) {
	t.Helper()
	boardID = seedBoard(t, db)
	today = repository.NormalizeReportDate(time.Now())

	must := func(q string, args ...interface{}) {
		t.Helper()
		require.NoError(t, db.Exec(q, args...).Error)
	}
	day := today.Add(-12 * time.Hour) // inside today's window

	// Aux labels + board composition.
	must(`INSERT INTO semantic_labels (id, label, slug, label_type, embedding, status, created_at, updated_at)
		VALUES (660001, 'AI 编程', 'watch-int-ai', 'auxiliary', '[1,0,0]', 'active', now(), now()),
		       (660002, '传统能源', 'watch-int-energy', 'auxiliary', '[0,1,0]', 'active', now(), now())`)
	must(`INSERT INTO board_composition (board_id, auxiliary_label_id) VALUES (?, 660001), (?, 660002)`, boardID, boardID)

	// Tags: one linked to the AI aux label.
	must(`INSERT INTO topic_tags (id, label, slug, category, status, created_at)
		VALUES (660101, 'AI 编程工具', 'watch-int-ai-tag', 'event', 'active', now())`)
	must(`INSERT INTO topic_tag_semantic_labels (topic_tag_id, semantic_label_id) VALUES (660101, 660001)`)

	// Articles: feed + three articles — one tagged AI, one tagged but off-topic,
	// one UNTAGGED containing "harness" (the 漏网 article).
	must(`INSERT INTO feeds (id, title, url, created_at) VALUES (660190, 'watch int feed', 'https://example.com', now())`)
	must(`INSERT INTO articles (id, feed_id, title, ai_content_summary, pub_date, created_at)
		VALUES (660001, 660190, 'Copilot 发布新版本', 'AI 编程工具更新', ?, now()),
		       (660002, 660190, 'harness 工具链发布', NULL, ?, now()),
		       (660003, 660190, 'Harness v3 发布公告', '开源 harness 更新', ?, now())`, day, day, day)
	must(`INSERT INTO article_topic_tags (article_id, topic_tag_id, created_at)
		VALUES (660001, 660101, now()), (660002, 660101, now())`)

	t.Cleanup(func() {
		db.Exec(`DELETE FROM board_topic_watches WHERE semantic_board_id = ?`, boardID)
		db.Exec(`DELETE FROM board_persistent_topics WHERE semantic_board_id = ?`, boardID)
		db.Exec(`DELETE FROM board_daily_reports WHERE semantic_board_id = ?`, boardID)
		db.Exec(`DELETE FROM article_topic_tags WHERE article_id IN (660001, 660002, 660003)`)
		db.Exec(`DELETE FROM articles WHERE feed_id = 660190`)
		db.Exec(`DELETE FROM feeds WHERE id = 660190`)
		db.Exec(`DELETE FROM topic_tag_semantic_labels WHERE semantic_label_id IN (660001, 660002)`)
		db.Exec(`DELETE FROM topic_tags WHERE id = 660101`)
		db.Exec(`DELETE FROM board_composition WHERE board_id = ?`, boardID)
		db.Exec(`DELETE FROM semantic_labels WHERE id IN (660001, 660002) OR slug = 'test-board-watch'`)
	})
	return boardID, today
}

// TestWatchMaterializationIntegration_KeywordAndSentence asserts the full
// materialization → SaveReport flow against a testcontainer PG:
//
//  1. keyword_topic: tag-less "harness" articles aggregate into an ephemeral
//     watch_keyword section with one thread each;
//  2. sentence_topic: aux-label retrieval materializes a watch_sentence
//     section owned by the watch's dedicated topic, whose lifecycle advances
//     (consecutive_hits=1 after the first day);
//  3. the keyword section never gains a persistent topic (no candidate
//     adoption), no relations touch watch sections;
//  4. hint tracks never produce hits for materialized watches.
func TestWatchMaterializationIntegration_KeywordAndSentence(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()

	boardID, today := seedMaterializationWorld(t, db)

	// One keyword_topic watch and one sentence_topic watch (cached vector
	// aligned with the AI aux label).
	kwWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID, Label: "harness", Type: repository.WatchTypeKeywordTopic,
	})
	require.NoError(t, err)
	stWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID,
		Label:           "AI 编程工具进展",
		Type:            repository.WatchTypeSentenceTopic,
		Query:           "AI coding assistant 的进展",
		EmbeddingCache:  strPtrHelper(repository.FloatsToPgVector([]float64{1, 0, 0})),
	})
	require.NoError(t, err)

	// ── Materialize both tracks (the orchestrator's Step 7.5 internals) ──
	cfg := DefaultWatchSentenceConfig()
	stSec, stThreads, err := MaterializeSentenceWatch(ctx, *stWatch, today, cfg, WatchMaterializeConfig{Enabled: false}, fakeEmbedder([]float64{1, 0, 0}), nil)
	require.NoError(t, err)
	require.NotNil(t, stSec, "aligned aux label + day articles ⇒ sentence section")
	require.Len(t, stThreads, 2, "tagged articles of the AI tag")
	require.NotNil(t, stSec.PersistentTopicID, "section owns the dedicated topic")

	kwSecs, kwBatches, err := MaterializeKeywordWatches(ctx, boardID, []repository.BoardTopicWatch{*kwWatch}, 1, WatchMaterializeConfig{Enabled: false}, nil)
	require.NoError(t, err)
	require.Len(t, kwSecs, 1)
	require.Len(t, kwBatches[0], 2, "harness articles: one tagged + one tag-less (漏网捞回)")
	assert.Nil(t, kwSecs[0].PersistentTopicID)

	// The dedicated topic was created and linked.
	linked, err := repository.Repo.GetWatchByID(stWatch.ID)
	require.NoError(t, err)
	require.NotNil(t, linked.PersistentTopicID)
	assert.Equal(t, *stSec.PersistentTopicID, *linked.PersistentTopicID)

	// ── SaveReport with regular + materialized sections ──
	report := &repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      today,
		Title:           "materialization int test",
		Status:          "completed",
	}
	regular := repository.DailyReportSection{
		ClusterIndex: 0, ClusterLabel: "常规聚类节", ClusterTagIDs: repository.JSON("[]"),
		Embedding: repository.FloatsToPgVector([]float64{0, 1, 0}), LaneTier: "l3_new",
	}
	stSec.ClusterIndex = 1
	kwSecs[0].ClusterIndex = 2
	sections := []repository.DailyReportSection{regular, *stSec, kwSecs[0]}
	threadsBy := [][]repository.DailyReportThread{nil, stThreads, kwBatches[0]}
	require.NoError(t, repository.Repo.SaveReport(report, sections, threadsBy))

	// 1. keyword section stays topic-less and lane-preserved.
	var kwReload repository.DailyReportSection
	require.NoError(t, db.Where("report_id = ? AND lane_tier = ?", report.ID, LaneTierWatchKeyword).First(&kwReload).Error)
	assert.Nil(t, kwReload.PersistentTopicID, "keyword section must never gain a topic")
	assert.Equal(t, 2, kwReload.ArticleCount)
	assert.Equal(t, "关键字『harness』相关话题", kwReload.ClusterLabel)

	// No extra candidate topic was adopted for the KEYWORD section: the
	// board's topics are exactly the watch topic + the regular section's own
	// l3 candidate (normal lane behavior — not keyword adoption).
	var topicCount int64
	db.Model(&repository.BoardPersistentTopic{}).Where("semantic_board_id = ?", boardID).Count(&topicCount)
	assert.Equal(t, int64(2), topicCount, "watch topic + regular l3 candidate — nothing for the keyword section")

	// 2. sentence topic lifecycle advanced by its section: day one ⇒ 1.
	var watchTopic repository.BoardPersistentTopic
	require.NoError(t, db.First(&watchTopic, *stSec.PersistentTopicID).Error)
	assert.Equal(t, repository.TopicStatusActive, watchTopic.Status)
	assert.Equal(t, 1, watchTopic.HitCount, "day-1 hit counted once (no double count)")
	assert.Equal(t, 1, watchTopic.ConsecutiveHits)

	// 3. no relations involve watch sections.
	var relCount int64
	db.Raw(`SELECT COUNT(*) FROM daily_report_section_relations rel
		JOIN daily_report_sections s ON s.id = rel.from_section_id
		WHERE s.report_id = ? AND s.lane_tier LIKE 'watch_%'`, report.ID).Scan(&relCount)
	assert.Zero(t, relCount, "watch sections must have no relations")

	// 4. hint tracks: materialized watches produce zero hits (both tracks off).
	hits, err := repository.Repo.GetWatchHitsByReport(report.ID)
	require.NoError(t, err)
	assert.Empty(t, hits)

	// ── Day 2: sentence topic continues (consecutive_hits → 2) ──
	day2 := today.AddDate(0, 0, 1)
	// Move the articles into day 2 first (same feed rows, bump pub_date),
	// then materialize — day-2 retrieval must hit the moved articles.
	require.NoError(t, db.Exec(`UPDATE articles SET pub_date = ? WHERE id IN (660001, 660002)`, day2.Add(-12*time.Hour)).Error)
	stSec2, stThreads2, err := MaterializeSentenceWatch(ctx, *linked, day2, cfg, WatchMaterializeConfig{Enabled: false}, fakeEmbedder([]float64{1, 0, 0}), nil)
	require.NoError(t, err)
	require.NotNil(t, stSec2, "day-2 retrieval hits again")

	report2 := &repository.BoardDailyReport{
		SemanticBoardID: boardID, PeriodDate: day2, Title: "day 2", Status: "completed",
	}
	stSec2.ClusterIndex = 0
	require.NoError(t, repository.Repo.SaveReport(report2, []repository.DailyReportSection{*stSec2}, [][]repository.DailyReportThread{stThreads2}))
	require.NoError(t, db.First(&watchTopic, *stSec.PersistentTopicID).Error)
	assert.Equal(t, 2, watchTopic.ConsecutiveHits, "second materialized day advances the streak")
	assert.Equal(t, 2, watchTopic.HitCount)
}

// TestWatchMaterializationAdjudication_KeywordFilterTitleConfidence verifies
// the keyword track's adjudication layer end-to-end (watch-materialize-
// llm-adjudication): recall hits are filtered to fitting articles, thread
// confidence carries the verdict, and the LLM same-batch title overrides the
// fixed derived name.
func TestWatchMaterializationAdjudication_KeywordFilterTitleConfidence(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, _ := seedMaterializationWorld(t, db)

	kwWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID, Label: "harness", Type: repository.WatchTypeKeywordTopic,
	})
	require.NoError(t, err)

	chat := &capturingChat{resp: `{"verdicts":[
		{"article_id":660002,"related":false,"confidence":0.9,"reason":"顺带提及"},
		{"article_id":660003,"related":true,"confidence":0.85,"reason":"以 harness 为主题"}
	],"section_title":"开源 harness 更新"}`}
	secs, batches, err := MaterializeKeywordWatches(ctx, boardID, []repository.BoardTopicWatch{*kwWatch}, 0, DefaultWatchMaterializeConfig(), chat.f)
	require.NoError(t, err)
	require.Len(t, secs, 1)
	require.Len(t, batches, 1)

	assert.Equal(t, 1, secs[0].ArticleCount, "only the adjudicated article aggregates")
	assert.Equal(t, "开源 harness 更新", secs[0].ClusterLabel, "LLM same-batch title overrides the fixed name")
	require.Len(t, batches[0], 1)
	assert.Equal(t, "Harness v3 发布公告", batches[0][0].Title)
	assert.InEpsilon(t, 0.85, batches[0][0].Confidence, 1e-9, "thread confidence = verdict confidence")
	assert.Equal(t, 1, chat.calls, "one batch adjudication call per watch")
	assert.Contains(t, chat.lastReq.Messages[1].Content, "harness", "keyword expression travels as the intent")
}

// TestWatchMaterializationAdjudication_SentenceFilterAndTopicCreation
// verifies the sentence track's adjudication: the tag-union is filtered to
// the article fitting the sentence intent, confidence carries over, and the
// dedicated topic is still created on first materialization.
func TestWatchMaterializationAdjudication_SentenceFilterAndTopicCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, today := seedMaterializationWorld(t, db)

	stWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID,
		Label:           "AI 编程工具进展",
		Type:            repository.WatchTypeSentenceTopic,
		Query:           "AI coding assistant 的进展",
		EmbeddingCache:  strPtrHelper(repository.FloatsToPgVector([]float64{1, 0, 0})),
	})
	require.NoError(t, err)

	// Recall union: 660001 (Copilot, on-topic) + 660002 (harness, off-topic).
	chat := &capturingChat{resp: `{"verdicts":[
		{"article_id":660001,"related":true,"confidence":0.9,"reason":"AI 编程进展"},
		{"article_id":660002,"related":false,"confidence":0.8,"reason":"主题是 harness 工具链"}
	],"section_title":"Copilot 发布新版本"}`}
	sec, threads, err := MaterializeSentenceWatch(ctx, *stWatch, today, DefaultWatchSentenceConfig(), DefaultWatchMaterializeConfig(), fakeEmbedder([]float64{1, 0, 0}), chat.f)
	require.NoError(t, err)
	require.NotNil(t, sec)
	require.Len(t, threads, 1, "tag union 2 → adjudicated 1")
	assert.Equal(t, "Copilot 发布新版本", sec.ClusterLabel, "LLM same-batch title overrides the watch label")
	assert.Equal(t, "Copilot 发布新版本", threads[0].Title)
	assert.InEpsilon(t, 0.9, threads[0].Confidence, 1e-9)
	require.NotNil(t, sec.PersistentTopicID, "dedicated topic created at first materialization (kept articles exist)")

	linked, err := repository.Repo.GetWatchByID(stWatch.ID)
	require.NoError(t, err)
	require.NotNil(t, linked.PersistentTopicID)
}

// TestWatchMaterializationAdjudication_AllRejectedNoSection: valid ids but
// every candidate related=false → no section today AND no dedicated topic
// creation (spec: 当天无命中不产空 section，自然衰减).
func TestWatchMaterializationAdjudication_AllRejectedNoSection(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, today := seedMaterializationWorld(t, db)

	stWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID,
		Label:           "无贴合追踪",
		Type:            repository.WatchTypeSentenceTopic,
		Query:           "完全不相关的一句话",
		EmbeddingCache:  strPtrHelper(repository.FloatsToPgVector([]float64{1, 0, 0})),
	})
	require.NoError(t, err)

	chat := &capturingChat{resp: `{"verdicts":[
		{"article_id":660001,"related":false,"confidence":0.9,"reason":"无关"},
		{"article_id":660002,"related":false,"confidence":0.9,"reason":"无关"}
	]}`}
	sec, threads, err := MaterializeSentenceWatch(ctx, *stWatch, today, DefaultWatchSentenceConfig(), DefaultWatchMaterializeConfig(), fakeEmbedder([]float64{1, 0, 0}), chat.f)
	require.NoError(t, err)
	assert.Nil(t, sec, "all rejected → no section")
	assert.Nil(t, threads)

	linked, err := repository.Repo.GetWatchByID(stWatch.ID)
	require.NoError(t, err)
	assert.Nil(t, linked.PersistentTopicID, "no fitting article → dedicated topic NOT created")
}

// TestWatchMaterializationAdjudication_DegradeToRecall: adjudication AI
// failure (call error / bad JSON) degrades to the full recall result —
// default confidence, fallback titles, report never blocked.
func TestWatchMaterializationAdjudication_DegradeToRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, today := seedMaterializationWorld(t, db)

	kwWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID, Label: "harness", Type: repository.WatchTypeKeywordTopic,
	})
	require.NoError(t, err)
	stWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID,
		Label:           "AI 编程工具进展",
		Type:            repository.WatchTypeSentenceTopic,
		Query:           "AI coding assistant 的进展",
		EmbeddingCache:  strPtrHelper(repository.FloatsToPgVector([]float64{1, 0, 0})),
	})
	require.NoError(t, err)

	// keyword: call error → full recall + fixed derived name + confidence 1.0.
	errChat := &capturingChat{err: errors.New("provider down")}
	secs, batches, err := MaterializeKeywordWatches(ctx, boardID, []repository.BoardTopicWatch{*kwWatch}, 0, DefaultWatchMaterializeConfig(), errChat.f)
	require.NoError(t, err, "degradation MUST NOT surface as a materialization error")
	require.Len(t, secs, 1)
	assert.Equal(t, 2, secs[0].ArticleCount, "degraded → full recall result")
	assert.Equal(t, "关键字『harness』相关话题", secs[0].ClusterLabel, "fallback title = fixed derived name")
	for _, th := range batches[0] {
		assert.InEpsilon(t, 1.0, th.Confidence, 1e-9, "degraded threads keep default confidence")
	}

	// sentence: bad JSON → full union + watch-label title + confidence 1.0.
	badChat := &capturingChat{resp: "not-json{"}
	sec, threads, err := MaterializeSentenceWatch(ctx, *stWatch, today, DefaultWatchSentenceConfig(), DefaultWatchMaterializeConfig(), fakeEmbedder([]float64{1, 0, 0}), badChat.f)
	require.NoError(t, err)
	require.NotNil(t, sec)
	assert.Equal(t, 2, sec.ArticleCount, "degraded → full tag union")
	assert.Equal(t, "AI 编程工具进展", sec.ClusterLabel, "fallback title = watch label")
	for _, th := range threads {
		assert.InEpsilon(t, 1.0, th.Confidence, 1e-9)
	}
}

// TestWatchMaterializationAdjudication_DisabledZeroCalls: the adjudication
// toggle off → pure pre-change behavior: zero AI calls, full recall.
func TestWatchMaterializationAdjudication_DisabledZeroCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, _ := seedMaterializationWorld(t, db)

	kwWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID, Label: "harness", Type: repository.WatchTypeKeywordTopic,
	})
	require.NoError(t, err)

	chat := &capturingChat{resp: `{"verdicts":[]}`}
	secs, _, err := MaterializeKeywordWatches(ctx, boardID, []repository.BoardTopicWatch{*kwWatch}, 0, WatchMaterializeConfig{Enabled: false, CandidateLimit: 40}, chat.f)
	require.NoError(t, err)
	require.Len(t, secs, 1)
	assert.Equal(t, 2, secs[0].ArticleCount, "disabled → recall aggregated as-is")
	assert.Zero(t, chat.calls, "disabled → zero AI calls")
	assert.Equal(t, "关键字『harness』相关话题", secs[0].ClusterLabel)
}

// TestLoadWatchMaterializeConfig_FromAISettings verifies the ai_settings
// overrides: false disables, positive limit applies, invalid rows keep
// defaults.
func TestLoadWatchMaterializeConfig_FromAISettings(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	// Defaults with no rows.
	cfg := LoadWatchMaterializeConfig(db)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 40, cfg.CandidateLimit)

	// Overrides apply.
	require.NoError(t, db.Exec(`INSERT INTO ai_settings (key, value, created_at, updated_at) VALUES
		('watch_materialize_llm_filter_enabled', 'false', now(), now()),
		('watch_materialize_candidate_limit', '7', now(), now())`).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM ai_settings WHERE key LIKE 'watch_materialize_%'`).Error
	})
	cfg = LoadWatchMaterializeConfig(db)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 7, cfg.CandidateLimit)

	// Invalid values keep defaults (fail-open).
	require.NoError(t, db.Exec(`UPDATE ai_settings SET value = 'abc' WHERE key = 'watch_materialize_candidate_limit'`).Error)
	require.NoError(t, db.Exec(`UPDATE ai_settings SET value = '' WHERE key = 'watch_materialize_llm_filter_enabled'`).Error)
	cfg = LoadWatchMaterializeConfig(db)
	assert.True(t, cfg.Enabled, "only literal false disables")
	assert.Equal(t, 40, cfg.CandidateLimit)
}

// TestWatchLabelReadPath_TransientDecoration verifies the read-path watch
// decoration (watch-materialize-llm-adjudication task 3.1): new sections
// persist watch_id and the detail API fills the transient watch_label;
// historical rows (watch_id NULL, pre-change shape) fall back to fixed-name
// parsing (keyword) / the title itself (sentence). The label itself must
// never persist.
func TestWatchLabelReadPath_TransientDecoration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	ctx := context.Background()
	boardID, today := seedMaterializationWorld(t, db)

	kwWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID, Label: "harness", Type: repository.WatchTypeKeywordTopic,
	})
	require.NoError(t, err)
	stWatch, err := repository.Repo.CreateWatch(repository.CreateWatchInput{
		SemanticBoardID: boardID,
		Label:           "AI 编程工具进展",
		Type:            repository.WatchTypeSentenceTopic,
		Query:           "AI coding assistant 的进展",
		EmbeddingCache:  strPtrHelper(repository.FloatsToPgVector([]float64{1, 0, 0})),
	})
	require.NoError(t, err)

	stSec, stThreads, err := MaterializeSentenceWatch(ctx, *stWatch, today, DefaultWatchSentenceConfig(), WatchMaterializeConfig{Enabled: false}, fakeEmbedder([]float64{1, 0, 0}), nil)
	require.NoError(t, err)
	require.NotNil(t, stSec)
	kwSecs, kwBatches, err := MaterializeKeywordWatches(ctx, boardID, []repository.BoardTopicWatch{*kwWatch}, 1, WatchMaterializeConfig{Enabled: false}, nil)
	require.NoError(t, err)
	require.Len(t, kwSecs, 1)

	report := &repository.BoardDailyReport{SemanticBoardID: boardID, PeriodDate: today, Title: "watch label probe", Status: "completed"}
	regular := repository.DailyReportSection{ClusterIndex: 0, ClusterLabel: "常规聚类节", ClusterTagIDs: repository.JSON("[]"), Embedding: repository.FloatsToPgVector([]float64{0, 1, 0}), LaneTier: "l3_new"}
	stSec.ClusterIndex = 1
	kwSecs[0].ClusterIndex = 2
	require.NoError(t, repository.Repo.SaveReport(report, []repository.DailyReportSection{regular, *stSec, kwSecs[0]}, [][]repository.DailyReportThread{nil, stThreads, kwBatches[0]}))

	// ── New rows: watch_id persisted, watch_label filled on the read path ──
	detail, err := repository.Repo.GetReportByID(report.ID)
	require.NoError(t, err)
	var kwSec, stSecOut, regularSec *repository.DailyReportSection
	for i := range detail.Sections {
		switch detail.Sections[i].LaneTier {
		case LaneTierWatchKeyword:
			kwSec = &detail.Sections[i]
		case LaneTierWatchSentence:
			stSecOut = &detail.Sections[i]
		default:
			regularSec = &detail.Sections[i]
		}
	}
	require.NotNil(t, kwSec)
	require.NotNil(t, stSecOut)
	require.NotNil(t, regularSec)
	assert.Equal(t, "harness", kwSec.WatchLabel, "keyword section decorated with the watch name")
	assert.Equal(t, "AI 编程工具进展", stSecOut.WatchLabel, "sentence section decorated with the watch name")
	assert.Empty(t, regularSec.WatchLabel, "regular sections carry no watch decoration")
	// The label is transient: nothing named watch_label exists on the table.
	assert.False(t, db.Migrator().HasColumn(&repository.DailyReportSection{}, "watch_label"))
	require.NotNil(t, kwSec.WatchID)
	assert.Equal(t, kwWatch.ID, *kwSec.WatchID, "watch_id persisted for new sections")

	// ── Historical rows (watch_id NULL): fallback resolution ──
	require.NoError(t, db.Exec(`UPDATE daily_report_sections SET watch_id = NULL WHERE report_id = ? AND lane_tier LIKE 'watch_%'`, report.ID).Error)
	detail2, err := repository.Repo.GetReportByID(report.ID)
	require.NoError(t, err)
	for i := range detail2.Sections {
		s := &detail2.Sections[i]
		switch s.LaneTier {
		case LaneTierWatchKeyword:
			assert.Equal(t, "harness", s.WatchLabel, "historical keyword section: fixed-name parsing recovers the expression")
		case LaneTierWatchSentence:
			assert.Equal(t, "AI 编程工具进展", s.WatchLabel, "historical sentence section: title was the watch label")
		}
	}
}
