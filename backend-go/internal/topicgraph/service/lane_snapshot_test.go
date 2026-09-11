package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── 泳道态势结算 service 测试（overview-lane-dynamics tasks 1.2/1.3）─────────
// LLM 经 laneSnapshotChatFn 注入 mock，零真实 airouter 调用；DB 用内存
// SQLite（service 包惯例——repository 层的 PG 集成测试另见
// lane_snapshot_repository_test.go）。

// laneSnapshotTestDB provisions the settlement tables and swaps the global
// repository singleton (mirrors watchTestDB).
func laneSnapshotTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
		&repository.BoardPersistentTopic{},
		&repository.BoardTopicWatch{},
		&repository.TopicLaneSnapshot{},
	))
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })
	return db
}

// laneChatRecorder captures chat calls and replays scripted responses.
type laneChatRecorder struct {
	systems  []string
	users    []string
	response func(callIdx int) (string, error)
}

func (r *laneChatRecorder) fn() func(context.Context, string, string) (string, error) {
	return func(_ context.Context, system, user string) (string, error) {
		r.systems = append(r.systems, system)
		r.users = append(r.users, user)
		if r.response == nil {
			return "默认态势。", nil
		}
		return r.response(len(r.users) - 1)
	}
}

func swapLaneChat(t *testing.T, rec *laneChatRecorder) {
	t.Helper()
	orig := laneSnapshotChatFn
	laneSnapshotChatFn = rec.fn()
	t.Cleanup(func() { laneSnapshotChatFn = orig })
}

func seedLaneSnapshotReport(t *testing.T, db *gorm.DB, boardID uint, date time.Time) uint {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      repository.NormalizeReportDate(date),
		Title:           "t",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func seedLaneSnapshotSection(t *testing.T, db *gorm.DB, reportID, topicID uint, label string, threadTitles ...string) {
	t.Helper()
	sec := repository.DailyReportSection{
		ReportID:         reportID,
		ClusterLabel:     label,
		Embedding:        repository.FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topicID,
	}
	require.NoError(t, db.Create(&sec).Error)
	for _, title := range threadTitles {
		require.NoError(t, db.Create(&repository.DailyReportThread{
			ReportID:  reportID,
			SectionID: sec.ID,
			Title:     title,
		}).Error)
	}
}

func seedLaneSnapshotTopic(t *testing.T, db *gorm.DB, boardID uint, label string, lastSeen time.Time) repository.BoardPersistentTopic {
	t.Helper()
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       repository.FloatsToPgVector([]float64{0}),
		Status:          repository.TopicStatusActive,
		Source:          repository.TopicSourceAuto,
		FirstSeenDate:   repository.NormalizeReportDate(lastSeen),
		LastSeenDate:    repository.NormalizeReportDate(lastSeen),
		HitCount:        1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

// 素材拼接形状（tasks 1.2）：日期｜section 标题｜前 3 条 thread 标题
//（渲染层自带 ≤3 截断，超出的第 4 条不进 prompt）；无线索标题的行省略
// 第三段。（repository 层的同型截断另由 PG 素材测试验证。）
func TestBuildLaneSnapshotUserPrompt_MaterialShape(t *testing.T) {
	day := repository.NormalizeReportDate(time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	rows := []repository.LaneMaterialRow{
		{PeriodDate: day, SectionID: 1, SectionLabel: "国产大模型发布", ThreadTitles: []string{"厂商A发布新模型", "厂商B开源", "算力扩容", "第四条不该出现"}},
		{PeriodDate: day.AddDate(0, 0, -1), SectionID: 2, SectionLabel: "无线索节"},
	}
	prompt := buildLaneSnapshotUserPrompt("AI 军备赛", rows)

	assert.Contains(t, prompt, "泳道：AI 军备赛")
	assert.Contains(t, prompt, "2026-09-08｜国产大模型发布｜厂商A发布新模型 / 厂商B开源 / 算力扩容")
	assert.NotContains(t, prompt, "第四条不该出现")
	assert.Contains(t, prompt, "2026-09-07｜无线索节")
	assert.True(t, strings.HasSuffix(prompt, "\n"), "每行素材以换行结尾")
}

// 窗口锚定 MAX(period_date)（design D3）：14 天前含、15 天前不含；
// upsert 幂等：重复结算覆盖同一行且 as_of=最新报告期（tasks 1.2）。
func TestSettleBoardLaneSnapshots_WindowAnchorAndUpsertIdempotent(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(1)
	now := time.Now()

	reportAnchor := seedLaneSnapshotReport(t, db, boardID, now)
	report14 := seedLaneSnapshotReport(t, db, boardID, now.AddDate(0, 0, -14))
	report15 := seedLaneSnapshotReport(t, db, boardID, now.AddDate(0, 0, -15))

	topic := seedLaneSnapshotTopic(t, db, boardID, "锚定话题", now)
	seedLaneSnapshotSection(t, db, reportAnchor, topic.ID, "今天的节", "线索一", "线索二", "线索三", "线索四")
	seedLaneSnapshotSection(t, db, report14, topic.ID, "十四天前的节")
	seedLaneSnapshotSection(t, db, report15, topic.ID, "十五天前的节不该出现")

	rec := &laneChatRecorder{response: func(i int) (string, error) {
		if i == 0 {
			return "第一版态势。", nil
		}
		return strings.Repeat("长", 130), nil // >100 runes → mechanical clamp
	}}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 1, "one lane → one LLM call")

	user := rec.users[0]
	assert.Contains(t, user, "今天的节｜线索一 / 线索二 / 线索三")
	assert.Contains(t, user, "十四天前的节")
	assert.NotContains(t, user, "十五天前的节不该出现")
	assert.NotContains(t, user, "线索四")
	assert.Contains(t, rec.systems[0], "100字")

	// Second run: same row overwritten (idempotent), clamped to 100 runes.
	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 2)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1, "重复结算覆盖同一行，不产生重复快照")
	assert.Equal(t, strings.Repeat("长", 100), snaps[0].RollingSummary)
	wantAsOf := repository.NormalizeReportDate(now)
	assert.Equal(t, wantAsOf, repository.NormalizeReportDate(snaps[0].AsOfDate), "as_of=最新报告期")
}

// clamp 20（design D1）：活跃泳道超上限时按 last_seen_date 降序截断并
// 记日志（tasks 1.3 验证项）。
func TestSettleBoardLaneSnapshots_ClampActiveLanes(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(2)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)

	total := repository.LaneSnapshotMaxPerBoard + 5
	for i := 0; i < total; i++ {
		// last_seen_date = now - i days → clamp keeps the first 20 by recency.
		topic := seedLaneSnapshotTopic(t, db, boardID, fmt.Sprintf("话题%d", i), now.AddDate(0, 0, -i))
		seedLaneSnapshotSection(t, db, reportID, topic.ID, topic.Label+"的节")
	}

	rec := &laneChatRecorder{}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	assert.Equal(t, repository.LaneSnapshotMaxPerBoard, len(rec.users),
		"active lanes clamped to %d (had %d)", repository.LaneSnapshotMaxPerBoard, total)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, repository.LaneSnapshotMaxPerBoard)

	// The clamped tail (oldest 5 by last_seen_date = 话题20..24) must have no
	// snapshot; the kept head (话题0..19) must all have one — asserted by
	// topic id set, not by summary text.
	var topics []repository.BoardPersistentTopic
	require.NoError(t, db.Order("id ASC").Find(&topics).Error)
	require.Len(t, topics, total)
	settled := map[uint]bool{}
	for _, s := range snaps {
		settled[s.PersistentTopicID] = true
	}
	for i, tp := range topics {
		if i >= repository.LaneSnapshotMaxPerBoard {
			assert.False(t, settled[tp.ID], "lane %d (%s) is beyond the clamp and must not settle", i, tp.Label)
		} else {
			assert.True(t, settled[tp.ID], "lane %d (%s) is within the clamp and must settle", i, tp.Label)
		}
	}
}

// 沉寂泳道（无窗口内素材）跳过：零 LLM 调用、不写快照（tasks 1.2/1.3）。
// 窗口锚定 MAX(period_date)：今天的报告必须存在（否则它自己就成了锚），
// 沉寂泳道的唯一 section 落在 30 天前的旧报告上（窗口外）。
func TestSettleBoardLaneSnapshots_SilentLaneSkipped(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(3)
	now := time.Now()
	// Today's report exists (owned by another active lane → keeps the anchor
	// at today) …
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	anchorTopic := seedLaneSnapshotTopic(t, db, boardID, "活跃话题", now)
	seedLaneSnapshotSection(t, db, reportID, anchorTopic.ID, "今天的节", "线索")
	// … while the silent lane only has a section on a 30-day-old report.
	oldReport := seedLaneSnapshotReport(t, db, boardID, now.AddDate(0, 0, -30))
	silent := seedLaneSnapshotTopic(t, db, boardID, "沉寂泳道", now.AddDate(0, 0, -30))
	seedLaneSnapshotSection(t, db, oldReport, silent.ID, "旧报告里的节")

	rec := &laneChatRecorder{}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 1, "only the in-window lane settles")
	assert.Contains(t, rec.users[0], "今天的节")
	assert.NotContains(t, rec.users[0], "旧报告里的节")

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, anchorTopic.ID, snaps[0].PersistentTopicID, "silent lane must not write a snapshot")
}

// 无报告板块：直接返回，零 LLM 调用。
func TestSettleBoardLaneSnapshots_NoReportsNoCalls(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(4)
	seedLaneSnapshotTopic(t, db, boardID, "无报告话题", time.Now())

	rec := &laneChatRecorder{}
	swapLaneChat(t, rec)
	settleBoardLaneSnapshots(context.Background(), boardID)
	assert.Empty(t, rec.users)
}

// 红线（tasks 1.3 / spec）：结算 panic 不影响已落库报告——settleLaneSnapshotsSafe
// 全吞 panic，报告行原样保留。
func TestSettleLaneSnapshotsSafe_PanicDoesNotAffectReport(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(5)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)

	origRun := runLaneSnapshotSettlement
	runLaneSnapshotSettlement = func(_ context.Context, _ uint) {
		panic("settlement exploded")
	}
	t.Cleanup(func() { runLaneSnapshotSettlement = origRun })

	require.NotPanics(t, func() { settleLaneSnapshotsSafe(boardID) }, "recover 必须吞掉结算 panic")

	var reports []repository.BoardDailyReport
	require.NoError(t, db.Find(&reports).Error)
	require.Len(t, reports, 1, "已落库报告不受结算 panic 影响")
	assert.Equal(t, reportID, reports[0].ID)
}

// LLM 失败：单泳道失败只记日志并继续兄弟泳道，快照保持缺失（spec 结算
// 失败不阻塞）。
func TestSettleBoardLaneSnapshots_LLMFailureContinuesSiblings(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(6)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)

	topicA := seedLaneSnapshotTopic(t, db, boardID, "A失败话题", now.AddDate(0, 0, -1))
	topicB := seedLaneSnapshotTopic(t, db, boardID, "B成功话题", now)
	seedLaneSnapshotSection(t, db, reportID, topicA.ID, "A的节")
	seedLaneSnapshotSection(t, db, reportID, topicB.ID, "B的节")

	rec := &laneChatRecorder{}
	rec.response = func(i int) (string, error) {
		// Lane order is last_seen_date DESC → B settles first; fail A's call.
		if strings.Contains(rec.users[i], "A失败话题") {
			return "", fmt.Errorf("provider down")
		}
		return "B 的态势。", nil
	}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 2, "both lanes attempted serially")

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, topicB.ID, snaps[0].PersistentTopicID, "失败泳道无快照，成功泳道照常结算")
}
