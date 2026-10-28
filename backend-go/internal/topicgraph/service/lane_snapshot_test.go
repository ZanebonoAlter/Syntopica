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

// 素材拼接形状（slim-lane-snapshot-trend）：日期｜section 标题｜前 3 条
// thread 标题（渲染层自带 ≤3 截断）+ 量级信号段（每日线索量含无报告日
// 明示、多节日 ·N节、近7/上7合计；上7窗无数据时明示无报告数据）。
// （repository 层的同型截断另由 PG 素材测试验证。）
func TestBuildLaneSnapshotUserPrompt_MaterialShape(t *testing.T) {
	day := repository.NormalizeReportDate(time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	rows := []repository.LaneMaterialRow{
		{PeriodDate: day, SectionID: 1, SectionLabel: "国产大模型发布", ThreadTitles: []string{"厂商A发布新模型", "厂商B开源", "算力扩容", "第四条不该出现"}},
		{PeriodDate: day.AddDate(0, 0, -1), SectionID: 2, SectionLabel: "无线索节"},
	}
	counts := []repository.LaneDailyCount{
		{PeriodDate: day, SectionCnt: 2, ThreadCnt: 5},
		{PeriodDate: day.AddDate(0, 0, -1), SectionCnt: 1, ThreadCnt: 3},
	}
	prompt := buildLaneSnapshotUserPrompt("AI 军备赛", rows, counts, day)

	assert.Contains(t, prompt, "泳道：AI 军备赛")
	assert.Contains(t, prompt, "2026-09-08｜国产大模型发布｜厂商A发布新模型 / 厂商B开源 / 算力扩容")
	assert.NotContains(t, prompt, "第四条不该出现")
	assert.Contains(t, prompt, "2026-09-07｜无线索节")

	// 量级信号段：每日行（含多节标注）、无报告日明示、窗口口径注记。
	assert.Contains(t, prompt, "--- 量级信号（系统统计")
	assert.Contains(t, prompt, "09-08 5条·2节")
	assert.Contains(t, prompt, "09-07 3条")
	assert.Contains(t, prompt, "不代表热度为零，不计入趋势判断")

	// 近7/上7合计：anchor=09-08 → 近7=[09-02,09-08]、上7=[08-26,09-01]；
	// 上7窗内无数据 → 明示无报告数据，不输出误导性 0 合计。
	assert.Contains(t, prompt, "近7天（09-02～09-08）合计 8 条｜上个7天（08-26～09-01）无报告数据")
	assert.True(t, strings.Contains(prompt, "量级信号"), "量级段在 prompt 内")
}

// 量级信号段专项：窗口 15 槽逐日铺满、缺日槽明示无报告、近7/上7
// 合计按窗内数据求和（缺日不计入也不报负）。
func TestRenderLaneMagnitudeSignal_FullWindowAndRollups(t *testing.T) {
	anchor := repository.NormalizeReportDate(time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	counts := []repository.LaneDailyCount{
		// 近7窗 [09-02, 09-08]：三日有数据。
		{PeriodDate: anchor, SectionCnt: 1, ThreadCnt: 4},
		{PeriodDate: anchor.AddDate(0, 0, -2), SectionCnt: 1, ThreadCnt: 6},
		{PeriodDate: anchor.AddDate(0, 0, -6), SectionCnt: 3, ThreadCnt: 11}, // 窗首 09-02
		// 上7窗 [08-26, 09-01]：两日有数据。
		{PeriodDate: anchor.AddDate(0, 0, -8), SectionCnt: 1, ThreadCnt: 5},
		{PeriodDate: anchor.AddDate(0, 0, -13), SectionCnt: 1, ThreadCnt: 2}, // 窗首 08-26
	}
	sig := renderLaneMagnitudeSignal(counts, anchor)

	assert.Contains(t, sig, "09-08 4条")
	assert.Contains(t, sig, "09-06 6条")
	assert.Contains(t, sig, "09-02 11条·3节")
	assert.Contains(t, sig, "09-01 无报告", "窗口内缺日明示无报告")
	assert.Contains(t, sig, "08-26 2条", "上个7天窗首日计入")
	assert.Contains(t, sig, "08-25 无报告", "量级窗口首日=素材窗口起点（R-1：与事实清单同窗，anchor-14 也铺行）")
	assert.Contains(t, sig, "近7天（09-02～09-08）合计 21 条｜上个7天（08-26～09-01）合计 7 条")
}

// 纯文本解析三步守卫边界值（白盒附加 P1/P2）：0/1/199/200/201 rune。
func TestParseLaneSnapshotOutput_Boundaries(t *testing.T) {
	// P1: 空串与纯空白（含全角空格/tab/换行混合）→ error。
	for name, in := range map[string]string{
		"空串":      "",
		"纯空格":     "   ",
		"全角空格":    "　",
		"混合空白":    "\t\n 　",
		"超长后空白截断": strings.Repeat("　", 300),
	} {
		t.Run(name, func(t *testing.T) {
			// 300 个全角空格 trim 后为空 → 同空串走 P1 守卫。
			got, err := parseLaneSnapshotOutput(in)
			assert.Error(t, err, "空/纯空白输出必须失败")
			assert.Empty(t, got)
		})
	}

	// P2: 非空 → rune 安全截断（199 原样 / 200 原样 / 201 截断）。
	for n, want := range map[int]int{1: 1, 199: 199, 200: 200, 201: laneSnapshotMaxRunes} {
		got, err := parseLaneSnapshotOutput(strings.Repeat("态", n) + "  ")
		require.NoError(t, err)
		assert.Equal(t, strings.Repeat("态", want), got, "%d rune 输入", n)
	}

	// 模型残留围栏/引号 → 不解析不剥壳，原样当文本入库（行为记录用例）。
	fenced := "```\n" + strings.Repeat("围", 190) + "\n```"
	got, err := parseLaneSnapshotOutput(fenced)
	require.NoError(t, err)
	assert.Equal(t, truncateRunes(fenced, laneSnapshotMaxRunes), got, "围栏残留原样入库（截断后）")
}

// 句末收口（2026-10-28 真库效果核对后新增）：超限时优先在句末标点截断，
// 不在字中间切断；句末标点过早（低于 60% 预算）或整段无标点时退回 rune 硬切。
func TestParseLaneSnapshotOutput_ClampsAtSentenceBoundary(t *testing.T) {
	// 190 字正文 + 句号 + 40 字尾巴 → 在句号处收口（191 rune）。
	body := strings.Repeat("阶", 190) + "。" + strings.Repeat("尾", 40)
	got, err := parseLaneSnapshotOutput(body)
	require.NoError(t, err)
	assert.Equal(t, 191, len([]rune(got)), "在句号处收口，不在字中间切")
	assert.True(t, strings.HasSuffix(got, "。"))

	// 句号出现太早（第 31 rune，低于 60% 预算）+ 后面无标点 → 退回 rune 硬切。
	early := strings.Repeat("早", 30) + "。" + strings.Repeat("续", 220)
	got, err = parseLaneSnapshotOutput(early)
	require.NoError(t, err)
	assert.Equal(t, laneSnapshotMaxRunes, len([]rune(got)))
	assert.False(t, strings.HasSuffix(got, "。"), "过早的句号不作为收口点")

	// 整段无标点 → rune 硬切（保持机械上限）。
	got, err = parseLaneSnapshotOutput(strings.Repeat("长", 230))
	require.NoError(t, err)
	assert.Equal(t, laneSnapshotMaxRunes, len([]rune(got)))
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
		return strings.Repeat("长", 230), nil // >200 runes → mechanical clamp
	}}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 1, "one lane → one LLM call")

	user := rec.users[0]
	assert.Contains(t, user, "今天的节｜线索一 / 线索二 / 线索三")
	assert.Contains(t, user, "十四天前的节")
	assert.NotContains(t, user, "十五天前的节不该出现")
	assert.NotContains(t, user, "线索四")
	assert.Contains(t, rec.systems[0], "200字")

	// Second run: same row overwritten (idempotent), clamped to 200 runes.
	settleBoardLaneSnapshots(context.Background(), boardID)
	require.Len(t, rec.users, 2)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1, "重复结算覆盖同一行，不产生重复快照")
	assert.Equal(t, strings.Repeat("长", laneSnapshotMaxRunes), snaps[0].RollingSummary)
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

// ── 泳道态势单版生成（slim-lane-snapshot-trend，test-cases 主链路 1/4/5/7）──
// 以下用例共同前置：单泳道/双泳道 + 一份窗口内报告；响应由 recorder 脚本化。

// 主链路1: 合法纯文本单版 → summary=全文、detail=空串、as_of=anchor。
func TestSettleLaneSnapshot_PlainTextSingleVersionUpsert(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(7)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "两版话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	rec := &laneChatRecorder{response: func(int) (string, error) {
		return "局势平稳推进，量级温和上升。", nil
	}}
	swapLaneChat(t, rec)

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, "局势平稳推进，量级温和上升。", snaps[0].RollingSummary)
	assert.Empty(t, snaps[0].RollingDetail, "detail 固定写空串")
	assert.Equal(t, repository.NormalizeReportDate(now), repository.NormalizeReportDate(snaps[0].AsOfDate), "as_of=anchor")
	// 主链路2接线断言（R-2）：settle 链路确实把量级信号段注入 user prompt。
	require.NotEmpty(t, rec.users)
	assert.Contains(t, rec.users[0], "量级信号", "量级信号段随素材注入（删掉 ListDailyThreadCounts 接线时此断言必红）")
}

// 主链路4: 超长纯文本（无句末标点）→ rune 安全截断至 200，detail 空串。
func TestSettleLaneSnapshot_OverlongClamped(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(8)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "超长话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return strings.Repeat("长", 230) + "尾随空白  ", nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1)
	assert.Equal(t, strings.Repeat("长", laneSnapshotMaxRunes), snaps[0].RollingSummary, "超长输出截断至上限")
	assert.Empty(t, snaps[0].RollingDetail, "detail 恒空")
}

// 主链路5：空输出守卫——合法轮落值后，纯空白输出 → 整次结算失败跳过，
// 快照保持最后一轮合法值。
func TestSettleLaneSnapshot_EmptyOutputFailsKeepsOldSnapshot(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(12)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "空输出话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	// 存量旧快照（含历史长版）先落库。
	require.NoError(t, repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "旧态势。",
		RollingDetail:     "旧历史长版。",
		AsOfDate:          repository.NormalizeReportDate(now.AddDate(0, 0, -1)),
	}))

	responses := []string{
		"第一轮正常演进叙事。",     // 合法纯文本 → 覆盖（detail 置空）
		"  \n\t \u3000 ", // 纯空白（含全角空格/tab/换行）→ 失败跳过
	}
	call := 0
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		resp := responses[call]
		call++
		return resp, nil
	}})

	settleBoardLaneSnapshots(context.Background(), boardID) // 覆盖成功
	settleBoardLaneSnapshots(context.Background(), boardID) // 空输出 → 跳过
	require.Equal(t, 2, call)

	var snaps []repository.TopicLaneSnapshot
	require.NoError(t, db.Find(&snaps).Error)
	require.Len(t, snaps, 1, "失败轮次不产生新行")
	assert.Equal(t, "第一轮正常演进叙事。", snaps[0].RollingSummary, "快照保持最后一轮合法值")
	assert.Empty(t, snaps[0].RollingDetail, "第一轮合法结算已置空 detail")
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
		return "B 新态势。", nil
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
	assert.Equal(t, "A 旧长版。", byTopic[topicA.ID].RollingDetail, "失败轮不覆盖，存量 detail 原样保留")
	assert.Equal(t, "B 新态势。", byTopic[topicB.ID].RollingSummary)
	assert.Empty(t, byTopic[topicB.ID].RollingDetail, "新结算 detail 恒空")
}

// 主链路3：system prompt 演进叙事定位约束；maxTokens 为包级常量
// （slim-lane-snapshot-trend D3: 1536→500，单版 200 字预算）。
func TestLaneSnapshotSystemPrompt_NarrativeContractAndMaxTokens(t *testing.T) {
	sys := laneSnapshotSystemPrompt()
	// 演进叙事定位（阶段/转折标记/落点/量级佐证）。
	assert.Contains(t, sys, "演进叙事")
	assert.Contains(t, sys, "阶段")
	assert.Contains(t, sys, "转折标记")
	assert.Contains(t, sys, "局势判断")
	assert.Contains(t, sys, "量级信号佐证走向")
	// 显式禁止（spec「演进叙事定位约束」Scenario）。
	assert.Contains(t, sys, "禁止逐日罗列事件")
	assert.Contains(t, sys, "禁止预测未来走向")
	assert.Contains(t, sys, "禁止使用清单与量级信号之外的数字")
	// 长度与输出形态。
	assert.Contains(t, sys, "150-200字")
	assert.Contains(t, sys, "不得超过200字", "篇幅硬约束（真库效果核对后新增）")
	assert.Contains(t, sys, "必须以句号收尾", "禁止写到一半停下")
	assert.Contains(t, sys, "纯文本")
	assert.Contains(t, sys, "不要JSON", "明确弃 JSON 输出")
	assert.NotContains(t, sys, `"summary"`)
	assert.NotContains(t, sys, `"detail"`)

	assert.Equal(t, 500, laneSnapshotMaxTokens, "slim-lane-snapshot-trend D3: 1536→500")
}

// 主链路7：存量行 detail 含历史长版 → 新结算覆盖时置空；读侧空串→nil
// （Detail=nil 语义沿用），读取方以 summary 为唯一内容源。
func TestSettleLaneSnapshot_LegacyRowDetailHeals(t *testing.T) {
	db := laneSnapshotTestDB(t)
	const boardID = uint(15)
	now := time.Now()
	reportID := seedLaneSnapshotReport(t, db, boardID, now)
	topic := seedLaneSnapshotTopic(t, db, boardID, "存量话题", now)
	seedLaneSnapshotSection(t, db, reportID, topic.ID, "节一", "线索一")

	// 旧行：短版 + 历史长版。
	require.NoError(t, repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "旧短版。",
		RollingDetail:     "旧历史长版。",
		AsOfDate:          repository.NormalizeReportDate(now),
	}))

	// 读侧 1：存量 detail 原样透出（无害；前端不再消费）。
	m, err := repository.Repo.GetLaneSnapshotsByTopicIDs([]uint{topic.ID})
	require.NoError(t, err)
	require.Contains(t, m, topic.ID)
	assert.Equal(t, "旧短版。", m[topic.ID].RollingSummary)
	assert.Equal(t, "旧历史长版。", m[topic.ID].RollingDetail)

	// 读侧 2：聚合不报错，存量 detail 透出为非 nil。
	resp, err := repository.Repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	require.NotNil(t, resp.Lanes[0].Snapshot)
	assert.Equal(t, "旧短版。", resp.Lanes[0].Snapshot.Summary)
	require.NotNil(t, resp.Lanes[0].Snapshot.Detail)
	assert.Equal(t, "旧历史长版。", *resp.Lanes[0].Snapshot.Detail)

	// 下个结算周期：单版覆盖，detail 置空。
	swapLaneChat(t, &laneChatRecorder{response: func(int) (string, error) {
		return "新演进叙事态势。", nil
	}})
	settleBoardLaneSnapshots(context.Background(), boardID)

	m2, err := repository.Repo.GetLaneSnapshotsByTopicIDs([]uint{topic.ID})
	require.NoError(t, err)
	assert.Equal(t, "新演进叙事态势。", m2[topic.ID].RollingSummary, "summary 覆盖")
	assert.Empty(t, m2[topic.ID].RollingDetail, "detail 覆盖置空")

	resp2, err := repository.Repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.NotNil(t, resp2.Lanes[0].Snapshot)
	assert.Equal(t, "新演进叙事态势。", resp2.Lanes[0].Snapshot.Summary)
	assert.Nil(t, resp2.Lanes[0].Snapshot.Detail, "空串→nil，长版缺失语义沿用")
}
