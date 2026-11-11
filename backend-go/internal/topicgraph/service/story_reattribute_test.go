package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// 事件线身份显式重算（--reattribute）与快照还原（--restore）的用例。
//
// 隔离库：内存 SQLite（与 story_backfill_test.go 同款惯例），不碰真库。
// 判定链的 AI 依赖全部打桩（不调模型）。

// reattributeFixture 是一次重算用例的固定数据。
type reattributeFixture struct {
	db      *gorm.DB
	day1    time.Time
	day2    time.Time
	threads map[string]uint
	stories map[string]uint
}

// stubReattributeAI 把归属链上的模型调用全部打桩（判定逻辑本身不动）。
func stubReattributeAI(t *testing.T) {
	t.Helper()
	oldEmbed, oldFactory, oldTitleEmbed := storySubjectEmbedder, storyAttributeClassifierFactory, storyTitleEmbedder
	storySubjectEmbedder = func(string) ([]float64, error) { return []float64{1, 0}, nil }
	storyTitleEmbedder = func(string) ([]float64, error) { return []float64{0, 1}, nil }
	storyAttributeClassifierFactory = func() StoryClassifier { return nil }
	t.Cleanup(func() {
		storySubjectEmbedder, storyAttributeClassifierFactory, storyTitleEmbedder = oldEmbed, oldFactory, oldTitleEmbed
	})
}

// reattributeRecallStore 模拟生产环境的 pgvector 召回：SQLite 没有向量算子，
// 这里让召回返回版块内全部存活事件线（生产实现按 embedding <=> ? 取 top10），
// 判定层照常按指纹裁决 —— 被重置的事件重新挂回原事件线。
type reattributeRecallStore struct {
	storyStore
}

func (s reattributeRecallStore) RecallStoryCandidates(boardID uint, vector string, since time.Time, limit int) ([]repository.StoryCandidate, error) {
	return s.storyStore.ListRecentStoryCandidates(boardID, since, 100)
}

// seedReattributeFixture 铺一条最小可判定的版块：
//
//	alpha  甲公司/发布/芯片   → 旧线 alpha（11-04 建线）
//	beta   乙公司/收购/标的公司 → 旧线 beta（11-05 建线）
//	gamma  丙公司/发布/服务器 → 旧线 gamma（11-05 建线）
//	delta  丁公司/发布/芯片   → 原本未归属（story_id = NULL）
//	eta    己公司/发布/服务器 → 旧线 zeta（指纹与事件不符 → 重算后另起新线）
//	manual 人工标记事件 + 人工事件线
//	composite 综合稿事件
//	epsilon 两条同指纹、跨天建线的孤儿事件线（只用于样本对）
func seedReattributeFixture(t *testing.T, db *gorm.DB) reattributeFixture {
	t.Helper()
	fixture := reattributeFixture{
		db:      db,
		day1:    backfillTestDay(t, "2026-11-04"),
		day2:    backfillTestDay(t, "2026-11-05"),
		threads: map[string]uint{},
		stories: map[string]uint{},
	}
	tags := []models.TopicTag{
		{ID: 11, Label: "甲公司发布芯片", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{
			"event_keywords": []string{"甲公司", "芯片"},
			"event_fingerprint": map[string]any{
				"subject": "甲公司", "subject_key": "company:alpha", "action": "发布", "object": "芯片", "occurred_at": "2026-11-03",
			},
		}},
		{ID: 12, Label: "乙公司收购标的公司", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{
			"event_keywords": []string{"乙公司", "标的公司"},
			"event_fingerprint": map[string]any{
				"subject": "乙公司", "subject_key": "company:beta", "action": "收购", "object": "标的公司", "occurred_at": "2026-11-04",
			},
		}},
		{ID: 13, Label: "丙公司发布服务器", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{
			"event_keywords": []string{"丙公司", "服务器"},
			"event_fingerprint": map[string]any{
				"subject": "丙公司", "subject_key": "company:gamma", "action": "发布", "object": "服务器", "occurred_at": "2026-11-04",
			},
		}},
		{ID: 14, Label: "丁公司发布芯片", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{
			"event_keywords": []string{"丁公司", "芯片"},
			"event_fingerprint": map[string]any{
				"subject": "丁公司", "subject_key": "company:delta", "action": "发布", "object": "芯片", "occurred_at": "2026-11-04",
			},
		}},
		{ID: 15, Label: "己公司发布服务器", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{
			"event_keywords": []string{"己公司", "服务器"},
			"event_fingerprint": map[string]any{
				"subject": "己公司", "subject_key": "company:eta", "action": "发布", "object": "服务器", "occurred_at": "2026-11-05",
			},
		}},
	}
	for i := range tags {
		require.NoError(t, db.Create(&tags[i]).Error)
	}
	for _, id := range []uint{101, 102, 103, 104, 105, 107} {
		article := models.Article{ID: id, FeedID: 1, Title: "evidence", Link: "https://example.test/reattribute"}
		require.NoError(t, db.Create(&article).Error)
	}
	report1 := repository.BoardDailyReport{SemanticBoardID: 7, PeriodDate: fixture.day1}
	report2 := repository.BoardDailyReport{SemanticBoardID: 7, PeriodDate: fixture.day2}
	require.NoError(t, db.Create(&report1).Error)
	require.NoError(t, db.Create(&report2).Error)
	section1 := repository.DailyReportSection{ReportID: report1.ID}
	section2 := repository.DailyReportSection{ReportID: report2.ID}
	require.NoError(t, db.Create(&section1).Error)
	require.NoError(t, db.Create(&section2).Error)

	stories := []repository.BoardStory{
		{ID: 1, BoardID: 7, Title: "甲公司发布芯片", Subject: "甲公司", SubjectKey: "company:alpha", Action: "发布", Object: "芯片", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day1, LastSeenDate: fixture.day1},
		{ID: 2, BoardID: 7, Title: "人工事件线", Subject: "人工", SubjectKey: "company:manual", Action: "指派", Object: "事件", Status: repository.StoryStatusActive, Origin: repository.StoryOriginManual, FirstSeenDate: fixture.day1, LastSeenDate: fixture.day1},
		{ID: 3, BoardID: 7, Title: "乙公司收购标的公司", Subject: "乙公司", SubjectKey: "company:beta", Action: "收购", Object: "标的公司", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day2, LastSeenDate: fixture.day2},
		{ID: 4, BoardID: 7, Title: "综合稿事件线", Subject: "行业", SubjectKey: "company:composite", Action: "综述", Object: "行业", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day2, LastSeenDate: fixture.day2},
		{ID: 5, BoardID: 7, Title: "丙公司发布服务器", Subject: "丙公司", SubjectKey: "company:gamma", Action: "发布", Object: "服务器", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day2, LastSeenDate: fixture.day2},
		{ID: 6, BoardID: 7, Title: "同指纹事件线甲", Subject: "戊公司", SubjectKey: "company:epsilon", Action: "发布", Object: "芯片", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day1, LastSeenDate: fixture.day1},
		{ID: 7, BoardID: 7, Title: "同指纹事件线乙", Subject: "戊公司", SubjectKey: "company:epsilon", Action: "发布", Object: "芯片", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day2, LastSeenDate: fixture.day2},
		{ID: 8, BoardID: 7, Title: "指纹不符的旧线", Subject: "己公司", SubjectKey: "company:zeta", Action: "发布", Object: "服务器", Status: repository.StoryStatusActive, Origin: repository.StoryOriginRule, FirstSeenDate: fixture.day2, LastSeenDate: fixture.day2},
	}
	for i := range stories {
		require.NoError(t, db.Create(&stories[i]).Error)
	}
	fixture.stories = map[string]uint{
		"alpha": 1, "manual": 2, "beta": 3, "composite": 4, "gamma": 5, "epsilonA": 6, "epsilonB": 7, "zeta": 8,
	}

	threads := []repository.DailyReportThread{
		{ReportID: report1.ID, SectionID: section1.ID, Title: "甲公司发布芯片", TagIDs: repository.JSON(`[11]`), RelatedArticleIDs: repository.JSON(`[101]`), Embedding: "[1,0]", StoryID: uintPtr(1), DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "人工标记事件", RelatedArticleIDs: repository.JSON(`[104]`), Embedding: "[1,0]", StoryID: uintPtr(2), StoryManual: true, DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "乙公司收购标的公司", TagIDs: repository.JSON(`[12]`), RelatedArticleIDs: repository.JSON(`[102]`), Embedding: "[1,0]", StoryID: uintPtr(3), DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "综合稿事件", Scope: repository.ThreadScopeComposite, RelatedArticleIDs: repository.JSON(`[104]`), Embedding: "[1,0]", StoryID: uintPtr(4), DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "丙公司发布服务器", TagIDs: repository.JSON(`[13]`), RelatedArticleIDs: repository.JSON(`[103]`), Embedding: "[1,0]", StoryID: uintPtr(5), DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "丁公司发布芯片", TagIDs: repository.JSON(`[14]`), RelatedArticleIDs: repository.JSON(`[105]`), Embedding: "[1,0]", DisplayTier: "body"},
		{ReportID: report2.ID, SectionID: section2.ID, Title: "己公司发布服务器", TagIDs: repository.JSON(`[15]`), RelatedArticleIDs: repository.JSON(`[107]`), Embedding: "[1,0]", StoryID: uintPtr(8), DisplayTier: "body"},
	}
	require.NoError(t, db.Create(&threads).Error)
	fixture.threads = map[string]uint{
		"alpha": threads[0].ID, "manual": threads[1].ID, "beta": threads[2].ID,
		"composite": threads[3].ID, "gamma": threads[4].ID, "delta": threads[5].ID, "eta": threads[6].ID,
	}
	// 召回层换成「返回版块内全部存活事件线」的模拟实现（SQLite 无 pgvector）。
	oldStore := storyStoreForTest
	storyStoreForTest = reattributeRecallStore{storyStore: repoStoryStore{repo: repository.NewTopicGraphRepository(db)}}
	t.Cleanup(func() { storyStoreForTest = oldStore })
	return fixture
}

func uintPtr(value uint) *uint { return &value }

func requireThreadStory(t *testing.T, db *gorm.DB, threadID uint, want *uint) {
	t.Helper()
	var thread repository.DailyReportThread
	require.NoError(t, db.Select("id", "story_id", "story_manual", "scope").First(&thread, threadID).Error)
	if want == nil {
		require.Nil(t, thread.StoryID, "thread %d must stay unassigned", threadID)
		return
	}
	require.NotNil(t, thread.StoryID, "thread %d must be attributed", threadID)
	require.Equal(t, *want, *thread.StoryID, "thread %d attribution", threadID)
}

func requireStoryExists(t *testing.T, db *gorm.DB, id uint, exists bool) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&repository.BoardStory{}).Where("id = ?", id).Count(&count).Error)
	if exists {
		require.EqualValues(t, 1, count, "story %d must exist", id)
		return
	}
	require.Zero(t, count, "story %d must be gone", id)
}

func countStories(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&repository.BoardStory{}).Count(&count).Error)
	return count
}

// readSnapshotRecords 读回快照文件的每一行。
func readSnapshotRecords(t *testing.T, path string) []snapshotRecord {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // G304: 测试读自己的快照文件
	require.NoError(t, err)
	var records []snapshotRecord
	for _, line := range splitLines(string(raw)) {
		var record snapshotRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func splitLines(raw string) []string {
	lines := []string{}
	current := ""
	for _, r := range raw {
		if r == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
			continue
		}
		current += string(r)
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// snapshotThreadMap 抽出快照里的「事件 → 事件线」映射。
func snapshotThreadMap(records []snapshotRecord) map[uint]*uint {
	out := map[uint]*uint{}
	for _, record := range records {
		if record.Type != snapshotRecordThread || record.ThreadID == nil {
			continue
		}
		out[*record.ThreadID] = record.StoryID
	}
	return out
}

// TestReattributeDryRunDoesNotWrite dry-run 只读：不写库、不出快照文件。
func TestReattributeDryRunDoesNotWrite(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	before := countStories(t, db)
	snapshot := filepath.Join(t.TempDir(), "snapshot.jsonl")

	stats, err := ReattributeStories(context.Background(), ReattributeOptions{
		BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2, DryRun: true},
	})
	require.NoError(t, err)
	require.True(t, stats.DryRun)
	require.Equal(t, 4, stats.ResettableThreads, "只有自动归属的四条事件会被重置")
	require.Equal(t, 4, stats.OldStories)
	require.Zero(t, stats.ThreadsReset)
	require.Zero(t, stats.Created+stats.Attached+stats.Merged+stats.StoriesDeleted)
	require.Equal(t, 1, stats.SamplePairs, "epsilon 两条同指纹跨天线构成一对")
	require.Len(t, stats.Pairs, 1)
	require.Equal(t, uint(6), stats.Pairs[0].StoryID)
	require.Equal(t, uint(7), stats.Pairs[0].PeerStoryID)
	require.False(t, stats.Pairs[0].HasDistance, "SQLite 无 pgvector 算不了距离")

	// 库里的归属与事件线数量不变。
	requireThreadStory(t, db, fixture.threads["alpha"], uintPtr(fixture.stories["alpha"]))
	requireThreadStory(t, db, fixture.threads["beta"], uintPtr(fixture.stories["beta"]))
	requireThreadStory(t, db, fixture.threads["gamma"], uintPtr(fixture.stories["gamma"]))
	requireThreadStory(t, db, fixture.threads["manual"], uintPtr(fixture.stories["manual"]))
	requireThreadStory(t, db, fixture.threads["composite"], uintPtr(fixture.stories["composite"]))
	requireThreadStory(t, db, fixture.threads["delta"], nil)
	require.Equal(t, before, countStories(t, db))
	_, err = os.Stat(snapshot)
	require.True(t, os.IsNotExist(err), "dry-run 不得落快照文件")
}

// TestReattributeResetsOnlyAutomaticThreads 写模式只重置自动归属面：人工标记
// 事件、人工事件线、综合稿一律不动。
func TestReattributeResetsOnlyAutomaticThreads(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	snapshot := filepath.Join(t.TempDir(), "snapshot.jsonl")

	stats, err := ReattributeStories(context.Background(), ReattributeOptions{
		BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2},
		SnapshotPath:    snapshot,
	})
	require.NoError(t, err)
	require.False(t, stats.DryRun)
	require.Equal(t, 4, stats.ThreadsReset, "alpha/beta/gamma/eta 四条被重置")
	require.Equal(t, 2, stats.Reports)
	require.Equal(t, 2, stats.Created, "delta 与 eta 无同指纹候选 → 另起新线")
	require.Equal(t, 3, stats.Attached, "alpha/beta/gamma 挂回原线")
	require.Zero(t, stats.Merged)
	require.Equal(t, 1, stats.StoriesDeleted, "zeta 旧线变空 → 删除")
	require.Zero(t, stats.Failed)

	// 人工与综合稿不动。
	requireThreadStory(t, db, fixture.threads["manual"], uintPtr(fixture.stories["manual"]))
	requireThreadStory(t, db, fixture.threads["composite"], uintPtr(fixture.stories["composite"]))
	requireStoryExists(t, db, fixture.stories["manual"], true)
	requireStoryExists(t, db, fixture.stories["composite"], true)

	// 自动事件重新归属到原有事件线。
	requireThreadStory(t, db, fixture.threads["alpha"], uintPtr(fixture.stories["alpha"]))
	requireThreadStory(t, db, fixture.threads["beta"], uintPtr(fixture.stories["beta"]))
	requireThreadStory(t, db, fixture.threads["gamma"], uintPtr(fixture.stories["gamma"]))
	requireStoryExists(t, db, fixture.stories["zeta"], false)
	require.FileExists(t, snapshot)
}

// TestReattributeSnapshotContent 快照含「事件 → 事件线」映射与相关事件线的
// 全量字段（向量以 float32 数组保存）。
func TestReattributeSnapshotContent(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	snapshot := filepath.Join(t.TempDir(), "snapshot.jsonl")

	_, err := ReattributeStories(context.Background(), ReattributeOptions{
		BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2},
		SnapshotPath:    snapshot,
	})
	require.NoError(t, err)

	records := readSnapshotRecords(t, snapshot)
	types := map[string]int{}
	for _, record := range records {
		types[record.Type]++
	}
	require.Equal(t, 1, types[snapshotRecordMeta])
	require.Equal(t, 5, types[snapshotRecordThread], "alpha/beta/gamma/eta + 原本未归属的 delta")
	require.Equal(t, 4, types[snapshotRecordStory], "alpha/beta/gamma/zeta 四条旧线的全量快照")
	require.Equal(t, 2, types[snapshotRecordCreated], "本次重算新建的两条线")

	mapping := snapshotThreadMap(records)
	require.Equal(t, uintPtr(fixture.stories["alpha"]), mapping[fixture.threads["alpha"]])
	require.Equal(t, uintPtr(fixture.stories["beta"]), mapping[fixture.threads["beta"]])
	require.Equal(t, uintPtr(fixture.stories["gamma"]), mapping[fixture.threads["gamma"]])
	require.Equal(t, uintPtr(fixture.stories["zeta"]), mapping[fixture.threads["eta"]])
	require.Nil(t, mapping[fixture.threads["delta"]], "原本未归属的事件快照记 NULL")
	require.NotContains(t, mapping, fixture.threads["manual"], "人工标记事件不在重算面内")
	require.NotContains(t, mapping, fixture.threads["composite"], "综合稿不在重算面内")

	// 事件线快照带全量字段。
	var storyRecord *snapshotRecord
	for i := range records {
		if records[i].Type == snapshotRecordStory && records[i].Story != nil && records[i].Story.ID == fixture.stories["alpha"] {
			storyRecord = &records[i]
			break
		}
	}
	require.NotNil(t, storyRecord)
	require.Equal(t, "company:alpha", storyRecord.Story.SubjectKey)
	require.Equal(t, "发布", storyRecord.Story.Action)
	require.Equal(t, "芯片", storyRecord.Story.Object)
	require.Equal(t, repository.StoryOriginRule, storyRecord.Story.Origin)
	require.Nil(t, storyRecord.Story.Embedding, "无向量时是 null")
}

// TestReattributeRestoreRoundTrip 还原把「事件 → 事件线」归属带回重算前：
// 被删的旧线按显式 id 重建，本次新建且已空的线被清掉。
func TestReattributeRestoreRoundTrip(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	snapshot := filepath.Join(t.TempDir(), "snapshot.jsonl")

	_, err := ReattributeStories(context.Background(), ReattributeOptions{
		BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2},
		SnapshotPath:    snapshot,
	})
	require.NoError(t, err)

	stats, err := RestoreStoryIdentity(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, 5, stats.ThreadsRestored)
	require.Equal(t, 1, stats.StoriesRestored, "zeta 旧线被删 → 按显式 id 重建")
	require.Equal(t, 2, stats.StoriesDeleted, "本次新建且已空的两条线被清掉")
	require.Zero(t, stats.Failed)

	requireThreadStory(t, db, fixture.threads["alpha"], uintPtr(fixture.stories["alpha"]))
	requireThreadStory(t, db, fixture.threads["beta"], uintPtr(fixture.stories["beta"]))
	requireThreadStory(t, db, fixture.threads["gamma"], uintPtr(fixture.stories["gamma"]))
	requireThreadStory(t, db, fixture.threads["eta"], uintPtr(fixture.stories["zeta"]))
	requireThreadStory(t, db, fixture.threads["delta"], nil) // 原本未归属的事件回到 NULL
	requireThreadStory(t, db, fixture.threads["manual"], uintPtr(fixture.stories["manual"]))
	requireStoryExists(t, db, fixture.stories["zeta"], true)
	requireStoryExists(t, db, fixture.stories["alpha"], true)

	// 幂等：再还原一次，状态不变，也不重复删/建。
	again, err := RestoreStoryIdentity(context.Background(), snapshot)
	require.NoError(t, err)
	require.Zero(t, again.StoriesRestored)
	require.Zero(t, again.StoriesDeleted)
	requireThreadStory(t, db, fixture.threads["alpha"], uintPtr(fixture.stories["alpha"]))
	requireThreadStory(t, db, fixture.threads["delta"], nil)
}

// TestReattributeIsIdempotent 连续跑两次结果一致（第二次无变更）。
func TestReattributeIsIdempotent(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	dir := t.TempDir()
	opts := ReattributeOptions{BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2}}

	opts.SnapshotPath = filepath.Join(dir, "first.jsonl")
	_, err := ReattributeStories(context.Background(), opts)
	require.NoError(t, err)
	first := map[string]*uint{}
	for key, threadID := range fixture.threads {
		first[key] = readThreadStory(t, db, threadID)
	}
	firstCount := countStories(t, db)

	opts.SnapshotPath = filepath.Join(dir, "second.jsonl")
	stats, err := ReattributeStories(context.Background(), opts)
	require.NoError(t, err)
	require.Zero(t, stats.StoriesDeleted, "第二次没有线变空")
	require.Zero(t, stats.Failed)
	for key, threadID := range fixture.threads {
		require.Equal(t, first[key], readThreadStory(t, db, threadID), "thread %s attribution must not change", key)
	}
	require.Equal(t, firstCount, countStories(t, db), "第二次不得增减事件线")
}

// readThreadStory 读一个事件当前的事件线归属。
func readThreadStory(t *testing.T, db *gorm.DB, threadID uint) *uint {
	t.Helper()
	var thread repository.DailyReportThread
	require.NoError(t, db.Select("id", "story_id").First(&thread, threadID).Error)
	return thread.StoryID
}

// TestReattributeRequiresSnapshot 写模式没有快照文件 → 直接拒绝（防不可逆）。
func TestReattributeRequiresSnapshot(t *testing.T) {
	db := setupStoryBackfillDB(t)
	stubReattributeAI(t)
	fixture := seedReattributeFixture(t, db)
	_, err := ReattributeStories(context.Background(), ReattributeOptions{
		BackfillOptions: BackfillOptions{From: fixture.day1, To: fixture.day2},
	})
	require.Error(t, err)
	requireThreadStory(t, db, fixture.threads["alpha"], uintPtr(fixture.stories["alpha"]))
}
