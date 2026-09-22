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
		ReportID:          reportID,
		ClusterLabel:      label,
		Embedding:         repository.FloatsToPgVector([]float64{0}),
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
// （渲染层自带 ≤3 截断，超出的第 4 条不进 prompt）；无线索标题的行省略
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

// ── 泳道态势两版生成（lane-trend-overview design D1，test-cases SN-1~SN-10）──
// 以下用例共同前置：单泳道/双泳道 + 一份窗口内报告；响应由 recorder 脚本化。

// SN-1: 合法 JSON 两版 → 分别入库，as_of=anchor。
func TestSettleLaneSnapshot_TwoVersionJSONUpsert(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(7)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "两版话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return `{"summary":"局势平稳推进。","detail":"过去两周该话题持续有新进展，多方参与度上升，节奏未见放缓。"}`, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, "局势平稳推进。", snaps[0].RollingSummary)
	assert.Equal(t, "过去两周该话题持续有新进展，多方参与度上升，节奏未见放缓。", snaps[0].RollingDetail)
	assert.Equal(t, repository.NormalizeReportDate(now), repository.NormalizeReportDate(snaps[0].AsOfDate), "as_of=anchor")
}

// SN-2: summary 超 100 rune / detail 超 600 rune → 各自 rune 安全截断，互不影响。
func TestSettleLaneSnapshot_ClampBothVersionsIndependent(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(8)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "超长话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return `{"summary":"` + strings.Repeat("短", 120) + `","detail":"` + strings.Repeat("长", 600) + `"}`, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, strings.Repeat("短", 100), snaps[0].RollingSummary, "短版独立截 100")
	assert.Equal(t, strings.Repeat("长", 500), snaps[0].RollingDetail, "长版独立截 500，不受短版截断影响")
}

// SN-3: detail 恰 500 rune 边界 → 不截断，原样入库。
func TestSettleLaneSnapshot_DetailExactBoundary(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(9)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "边界话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	detail := strings.Repeat("边", 500)
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return `{"summary":"短版。","detail":"` + detail + `"}`, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, detail, snaps[0].RollingDetail, "恰 500 rune 不截断")
}

// SN-4: 非 JSON 纯文本 → 降级：整段截 100 作短版、长版空串；upsert 照常
// （不算失败，下个日报日自愈）。
func TestSettleLaneSnapshot_NonJSONDegrades(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(10)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "纯文本话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	raw := strings.Repeat("这段输出完全不是JSON格式。", 10) // 130 runes > 100
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return raw, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID) // 不得因降级而中断

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1, "降级不算失败，upsert 照常")
	assert.Equal(t, truncateRunes(raw, 100), snaps[0].RollingSummary, "短版=整段截 100")
	assert.Empty(t, snaps[0].RollingDetail, "长版置空")
}

// SN-5: JSON 合法但 detail 缺失/空串 → summary 正常入库、长版空；不算失败。
func TestSettleLaneSnapshot_MissingDetailNotFailure(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(11)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "缺长版话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	responses := []string{
		`{"summary":"缺字段短版。"}`,            // detail 键缺失
		`{"summary":"空串短版。","detail":""}`, // detail 空串
	}
	call := 0
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		resp := responses[call]
		call++
		return resp, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)
	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Equal(t, 2, call, "两轮结算都发起了调用（不算失败）")

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, "空串短版。", snaps[0].RollingSummary, "summary 正常入库")
	assert.Empty(t, snaps[0].RollingDetail, "detail 缺失/空串入库为空")
}

// SN-6: JSON 合法但 summary 缺失/空、detail 有值 → 维持既有空输出守卫：整次
// 结算按失败跳过，快照保持旧值。
func TestSettleLaneSnapshot_MissingSummaryFailsKeepsOldSnapshot(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(12)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "缺短版话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	responses := []string{
		`{"summary":"旧短版。","detail":"旧长版叙述。"}`, // 第一轮合法，落旧值
		`{"detail":"只有长版。"}`,                   // summary 缺失 → 失败
		`{"summary":"","detail":"只有长版。"}`,      // summary 空 → 失败
	}
	call := 0
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		resp := responses[call]
		call++
		return resp, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID) // 落旧值
	settleBoardLaneSnapshots(context.Background(), boardID) // summary 缺失 → 跳过
	settleBoardLaneSnapshots(context.Background(), boardID) // summary 空 → 跳过
	require.Equal(t, 3, call)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1, "失败轮次不产生新行")
	assert.Equal(t, "旧短版。", snaps[0].RollingSummary, "快照保持旧值")
	assert.Equal(t, "旧长版叙述。", snaps[0].RollingDetail, "快照保持旧值")
}

// SN-7: JSON 外裹 markdown code fence → 剥壳后按 SN-1 处理。
func TestSettleLaneSnapshot_CodeFenceStripped(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(13)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "围栏话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return "```json\n{\"summary\":\"围栏内短版。\",\"detail\":\"围栏内长版。\"}\n```", nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, "围栏内短版。", snaps[0].RollingSummary)
	assert.Equal(t, "围栏内长版。", snaps[0].RollingDetail)
}

// SN-8: LLM 调用报错 → 沿既有失败路径：快照保持旧值、不阻塞兄弟泳道。
func TestSettleLaneSnapshot_LLMErrorKeepsOldSnapshotContinuesSiblings(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(14)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)

	topicA := seedLaneSnapshotTopic(t, db, boardID, "A报错话题", now.AddDate(0, 0, -1))
	topicB := seedLaneSnapshotTopic(t, db, boardID, "B成功话题", now)
	seedLaneSnapshotSection(t, db, reportID, topicA.ID, "A的节")
	seedLaneSnapshotSection(t, db, reportID, topicB.ID, "B的节")

	// A 的旧快照（含旧长版）先落库。
	require.NoError(t, repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: topicA.ID,
		RollingSummary:    "A 旧短版。",
		RollingDetail:     "A 旧长版。",
		AsOfDate:          repository.NormalizeReportDate(now.AddDate(0, 0, -1)),
	}))

	rec := &laneChatRecorder{}
	rec.response = func(i int) (string, error) {
		if strings.Contains(rec.users[i], "A报错话题") {
			return "", fmt.Errorf("provider down")
		}
		return `{"summary":"B 新短版。","detail":"B 新长版。"}`, nil
	}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 2, "兄弟泳道不被阻塞")

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 2)
	byTopic := map[uint]repository.TopicLaneSnapshot{}
	for _, s := range snaps {
		byTopic[s.PersistentTopicID] = s
	}
	assert.Equal(t, "A 旧短版。", byTopic[topicA.ID].RollingSummary, "A 快照保持旧值")
	assert.Equal(t, "A 旧长版。", byTopic[topicA.ID].RollingDetail)
	assert.Equal(t, "B 新短版。", byTopic[topicB.ID].RollingSummary)
	assert.Equal(t, "B 新长版。", byTopic[topicB.ID].RollingDetail)
}

// SN-9: prompt 断言：两版结构、字数约束、同事实集要求、既有纪律句；
// maxTokens 提为包级常量且值为 768（chatFn 引用它）。
func TestLaneSnapshotSystemPrompt_TwoVersionContractAndMaxTokens(t *testing.T) {
	sys := laneSnapshotSystemPrompt()
	assert.Contains(t, sys, `"summary"`)
	assert.Contains(t, sys, `"detail"`)
	assert.Contains(t, sys, "100字")
	assert.Contains(t, sys, "500字")
	assert.Contains(t, sys, "同一")
	assert.Contains(t, sys, "成段")
	assert.Contains(t, sys, "不得另起炉灶")
	// 既有纪律句保留。
	assert.Contains(t, sys, "只基于清单内列出的事实")
	assert.Contains(t, sys, "不得编造事件、数字、情绪与因果")
	assert.Contains(t, sys, "不做事态预测或走向判断")
	assert.Contains(t, sys, "直接输出 JSON 本身")

	assert.Equal(t, 768, laneSnapshotMaxTokens, "design D1: maxTokens 512→768")
}

// SN-10: 存量行 detail 空 → 读侧不报错（Detail=nil 语义）；下个结算周期
// 覆盖补齐两版。
func TestSettleLaneSnapshot_LegacyRowDetailHeals(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(15)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "存量话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	// 旧行：仅短版，detail 空。
	require.NoError(t, repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "旧短版。",
		AsOfDate:          repository.NormalizeReportDate(now),
	}))

	// 读侧 1：批量加载不报错，detail 空。
	m, err := repository.Repo.GetLaneSnapshotsByTopicIDs([]uint{topic.ID})
	require.NoError(t, err)
	require.Contains(t, m, topic.ID)
	assert.Equal(t, "旧短版。", m[topic.ID].RollingSummary)
	assert.Empty(t, m[topic.ID].RollingDetail, "存量行 detail 空")

	// 读侧 2：聚合不报错，Detail=nil（长版缺失语义）。
	resp, err := repository.Repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	require.NotNil(t, resp.Lanes[0].Snapshot)
	assert.Equal(t, "旧短版。", resp.Lanes[0].Snapshot.Summary)
	assert.Nil(t, resp.Lanes[0].Snapshot.Detail)

	// 下个结算周期：两版覆盖补齐。
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return `{"summary":"新短版。","detail":"新长版叙述。"}`, nil
	}})
	settleBoardLaneSnapshots(context.Background(), boardID)

	m2, err := repository.Repo.GetLaneSnapshotsByTopicIDs([]uint{topic.ID})
	require.NoError(t, err)
	assert.Equal(t, "新短版。", m2[topic.ID].RollingSummary, "短版覆盖补齐")
	assert.Equal(t, "新长版叙述。", m2[topic.ID].RollingDetail, "长版覆盖补齐")

	resp2, err := repository.Repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.NotNil(t, resp2.Lanes[0].Snapshot)
	require.NotNil(t, resp2.Lanes[0].Snapshot.Detail)
	assert.Equal(t, "新长版叙述。", *resp2.Lanes[0].Snapshot.Detail)
}
