package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 关键事件推荐服务测试（add-topic-key-event-markers 2.1/2.2）──────────────
//
// design D3 白盒：单次有界调用、输入对照（候选/指纹/已处理）、输出校验（合法
// ID/理由/依据）、完全重复机械过滤、失败不改日报、空候选不调 AI、候选/预算
// 上限、超时边界、板块互斥、迟到 token（repo 层已覆盖）。

func setupKeyEventRecommendTest(t *testing.T) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
}

type keRecWorld struct {
	boardID   uint
	topicID   uint
	reportID  uint
	sectionID uint
	threadIDs []uint
}

// seedKeRecWorld creates board/topic/report/section and n threads titled
// "线索<i>" each with article evidence [1000+i].
func seedKeRecWorld(t *testing.T, n int) keRecWorld {
	t.Helper()
	board := models.SemanticLabel{Label: "rec-board", Slug: "rec-board", LabelType: "board", Status: "active"}
	require.NoError(t, repository.Repo.DB().Create(&board).Error)
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID, Label: "推荐泳道", Status: repository.TopicStatusActive,
		FirstSeenDate: repository.NormalizeReportDate(time.Now()),
		LastSeenDate:  repository.NormalizeReportDate(time.Now()),
	}
	require.NoError(t, repository.Repo.DB().Omit("embedding", "centroid").Create(&topic).Error)
	report := repository.BoardDailyReport{
		SemanticBoardID: board.ID, PeriodDate: repository.NormalizeReportDate(time.Now()),
		Status: "completed", Title: "t",
	}
	require.NoError(t, repository.Repo.DB().Create(&report).Error)
	section := repository.DailyReportSection{
		ReportID: report.ID, ClusterLabel: "章节", ArticleCount: n,
		Embedding:         repository.FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topic.ID,
	}
	require.NoError(t, repository.Repo.DB().Create(&section).Error)

	// Evidence articles so prompt renders titles.
	for i := 1; i <= n; i++ {
		art := models.Article{Title: "证据文章" + string(rune('A'+i-1%26))}
		art.ID = uint(1000 + i)
		require.NoError(t, repository.Repo.DB().Create(&art).Error)
	}

	ids := make([]uint, 0, n)
	for i := 1; i <= n; i++ {
		th := repository.DailyReportThread{
			ReportID: report.ID, SectionID: section.ID,
			Title: "线索" + string(rune('A'+i-1%26)), Summary: "摘要",
			TagIDs:            repository.JSON("[5]"),
			RelatedArticleIDs: repository.JSON("[" + itoa(1000+i) + "]"),
			Embedding:         repository.FloatsToPgVector([]float64{0}),
		}
		require.NoError(t, repository.Repo.DB().Create(&th).Error)
		ids = append(ids, th.ID)
	}
	return keRecWorld{boardID: board.ID, topicID: topic.ID, reportID: report.ID, sectionID: section.ID, threadIDs: ids}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// injectKeRecChat swaps the chat hook; returns a pointer to the captured
// user prompt. restore via cleanup.
func injectKeRecChat(t *testing.T, fn func(ctx context.Context, sessionID, system, user string) (string, error)) *string {
	t.Helper()
	orig := keyEventRecommendChatFn
	t.Cleanup(func() { keyEventRecommendChatFn = orig })
	var prompt string
	keyEventRecommendChatFn = func(ctx context.Context, sessionID, system, user string) (string, error) {
		prompt = user
		return fn(ctx, sessionID, system, user)
	}
	return &prompt
}

func runRec(ctx context.Context, boardID, reportID uint) {
	runKeyEventRecommendation(ctx, boardID, reportID, "daily_report_test")
}

func getRun(t *testing.T, reportID uint) *repository.KeyEventRecommendationRun {
	t.Helper()
	run, err := repository.Repo.GetKeyEventRecommendationRun(reportID)
	require.NoError(t, err)
	return run
}

func TestKeyEventRecommendation_HappyPathSavesSuggested(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 3)

	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		return `[{"source_thread_id":` + itoa(int(w.threadIDs[1])) + `,"reason":"该事件正式落地，对泳道有实质影响","new_progress":true,"evidence_refs":[` + itoa(1000+2) + `]}]`, nil
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	require.NotNil(t, run)
	assert.Equal(t, repository.KeyEventRunStateDone, run.State)
	assert.Equal(t, 3, run.CandidateCount)
	assert.Equal(t, 1, run.SuggestedCount)

	var markers []repository.TopicKeyEventMarker
	require.NoError(t, repository.Repo.DB().Where("source_thread_id = ?", w.threadIDs[1]).Find(&markers).Error)
	require.Len(t, markers, 1)
	m := markers[0]
	assert.Equal(t, repository.KeyEventStateSuggested, m.State, "一次推荐仅存 suggested，未转 confirmed")
	assert.Equal(t, "该事件正式落地，对泳道有实质影响", m.RecommendationReason)
	assert.Contains(t, string(m.RecommendationBasis), "1002")
	assert.Equal(t, w.topicID, *m.PersistentTopicID)
}

func TestKeyEventRecommendation_EmptyCandidatesNoAICall(t *testing.T) {
	setupKeyEventRecommendTest(t)
	// Report with an ORPHAN section (no topic attribution) → zero candidates.
	board := models.SemanticLabel{Label: "rec-empty", Slug: "rec-empty", LabelType: "board", Status: "active"}
	require.NoError(t, repository.Repo.DB().Create(&board).Error)
	report := repository.BoardDailyReport{
		SemanticBoardID: board.ID, PeriodDate: repository.NormalizeReportDate(time.Now()),
		Status: "completed",
	}
	require.NoError(t, repository.Repo.DB().Create(&report).Error)
	section := repository.DailyReportSection{
		ReportID: report.ID, ClusterLabel: "orphan", Embedding: repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, repository.Repo.DB().Create(&section).Error)
	th := repository.DailyReportThread{
		ReportID: report.ID, SectionID: section.ID, Title: "无归属线索",
		Embedding: repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, repository.Repo.DB().Create(&th).Error)

	called := false
	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		called = true
		return "[]", nil
	})
	runRec(context.Background(), board.ID, report.ID)

	assert.False(t, called, "无有效候选不调 AI")
	run := getRun(t, report.ID)
	require.NotNil(t, run)
	assert.Equal(t, repository.KeyEventRunStateDone, run.State)
	assert.Equal(t, 0, run.CandidateCount)
}

func TestKeyEventRecommendation_ExactDuplicateFilteredFromCandidates(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 2)

	// Thread #1 already has ANY marker row (dismissed = 已处理) → mechanical
	// filter must keep it OUT of the AI candidate set (纯转载靠对照，完全重复
	// 靠指纹机械拦截).
	_, err := repository.Repo.ApplyThreadKeyEvent(w.topicID, w.threadIDs[0], repository.KeyEventActionDismiss, nil)
	require.NoError(t, err)

	prompt := injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		return "[]", nil
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	assert.Equal(t, 1, run.CandidateCount, "完全重复来源不进候选")
	// The candidate block (before the fingerprint block) must NOT list the
	// processed thread; the fingerprint block legitimately still shows it as
	// recent fact, and the dismissed对照区 must list it for progress comparison.
	candidateBlock := (*prompt)[:strings.Index(*prompt, "--- 泳道近期14天事实指纹")]
	assert.NotContains(t, candidateBlock, "线索A", "已处理线索不进 prompt 候选清单")
	assert.Contains(t, candidateBlock, "线索B")
	assert.Contains(t, *prompt, "用户已不采纳")
	assert.Contains(t, (*prompt)[strings.Index(*prompt, "用户已不采纳"):], "线索A", "已处理标题出现在对照区")
}

func TestKeyEventRecommendation_InvalidOutputItemsDropped(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 4)

	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		return `[
			{"source_thread_id": 999999, "reason":"越界ID","evidence_refs":[1001]},
			{"source_thread_id": ` + itoa(int(w.threadIDs[0])) + `, "reason":"   ","evidence_refs":[1001]},
			{"source_thread_id": ` + itoa(int(w.threadIDs[1])) + `, "reason":"无依据","evidence_refs":[888888]},
			{"source_thread_id": ` + itoa(int(w.threadIDs[2])) + `, "reason":"合法建议，正式开放","new_progress":true,"evidence_refs":[888888,1003]},
			{"source_thread_id": ` + itoa(int(w.threadIDs[3])) + `, "reason":"单token理由","evidence_refs":[1004],"new_progress":false}
		]`, nil
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	assert.Equal(t, 4, run.CandidateCount)
	assert.Equal(t, 2, run.SuggestedCount, "越界/空白理由/无有效依据丢弃；单 token 有事实理由不按长度武断拒绝")

	var states []string
	require.NoError(t, repository.Repo.DB().Model(&repository.TopicKeyEventMarker{}).
		Order("source_thread_id ASC").Pluck("state", &states).Error)
	assert.Equal(t, []string{repository.KeyEventStateSuggested, repository.KeyEventStateSuggested}, states)

	// The mixed-evidence item keeps ONLY its valid ref.
	var mixed repository.TopicKeyEventMarker
	require.NoError(t, repository.Repo.DB().Where("source_thread_id = ?", w.threadIDs[2]).First(&mixed).Error)
	assert.Contains(t, string(mixed.RecommendationBasis), "1003")
	assert.NotContains(t, string(mixed.RecommendationBasis), "888888")
}

func TestKeyEventRecommendation_FailureRunFailedReportUntouched(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 2)

	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		return "", context.DeadlineExceeded
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	require.NotNil(t, run)
	assert.Equal(t, repository.KeyEventRunStateFailed, run.State)

	// 日报本体不受推荐失败影响（spec: 推荐失败不改变日报终态）。
	var report repository.BoardDailyReport
	require.NoError(t, repository.Repo.DB().First(&report, w.reportID).Error)
	assert.Equal(t, "completed", report.Status)
	var markers int64
	require.NoError(t, repository.Repo.DB().Model(&repository.TopicKeyEventMarker{}).Count(&markers).Error)
	assert.Equal(t, int64(0), markers)

	// 打开页面只读：再次纯读取不产生新 run / 不重试付费调用。
	_, err := repository.Repo.ListReportKeyEventMarkers(w.reportID)
	require.NoError(t, err)
	run2 := getRun(t, w.reportID)
	assert.Equal(t, run.ID, run2.ID)
}

func TestKeyEventRecommendation_TimeoutBoundary(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 1)

	origTimeout := keyEventRecommendTimeoutVar
	keyEventRecommendTimeoutVar = 30 * time.Millisecond
	t.Cleanup(func() { keyEventRecommendTimeoutVar = origTimeout })

	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
			return "[]", nil
		}
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	require.NotNil(t, run)
	assert.Equal(t, repository.KeyEventRunStateFailed, run.State, "超时按失败落账，不重试")
}

func TestKeyEventRecommendation_CandidateCap40(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 45)

	prompt := injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		return "[]", nil
	})
	runRec(context.Background(), w.boardID, w.reportID)

	run := getRun(t, w.reportID)
	assert.Equal(t, 40, run.CandidateCount, "候选最多 40 条（按日报顺序、稳定 ID）")
	// Threads are created in order; the first 40 appear, the last 5 don't.
	assert.Contains(t, *prompt, itoa(int(w.threadIDs[39])))
	assert.NotContains(t, *prompt, " "+itoa(int(w.threadIDs[40]))+"｜")
}

// R-8 regression anchor: the prompt must render evidence as "id=标题" pairs —
// bare titles made the model return string refs that failed []uint unmarshal
// and every suggestion was dropped wholesale (suggested_count stayed 0).
func TestKeyEventRecommendation_CandidateLinesRenderEvidenceIDs(t *testing.T) {
	candidates := []keyEventCandidate{{
		ThreadID: 7, TopicID: 1, TopicLabel: "AI泳道", Title: "线索标题", Summary: "摘要",
		ArticleIDs: []uint{14157, 14162},
		Evidence:   []KeyEventEvidence{{ID: 14157, Title: "报道：纽约市主计长敦促Anthropic暂停推进IPO"}, {ID: 14162}},
	}}
	line := renderKeyEventCandidateLines(candidates)
	assert.Contains(t, line, "14157=报道：纽约市主计长敦促Anthropic暂停推进IPO",
		"依据必须带 id=标题 对，模型才有 id 可引用")
	assert.Contains(t, line, "14162", "无标题的文章也要裸 id 渲染，不得整条丢弃")
	assert.NotContains(t, line, "依据：报道：", "禁止退回裸标题渲染（R-8）")
}

func TestKeyEventRecommendation_BudgetClampedTo12000Runes(t *testing.T) {
	// Pure prompt-renderer test: oversized input clamps to the budget and
	// reports the truncation amount (no DB needed).
	candidates := make([]keyEventCandidate, 200)
	for i := range candidates {
		candidates[i] = keyEventCandidate{
			ThreadID: uint(i + 1), TopicID: 1, TopicLabel: "L", Title: strings.Repeat("长", 100),
			Summary: strings.Repeat("摘", 100), ArticleIDs: []uint{1}, Evidence: []KeyEventEvidence{{ID: 1, Title: strings.Repeat("据", 100)}},
		}
	}
	text, truncated := clampKeyEventPrompt(renderKeyEventCandidateLines(candidates))
	assert.Equal(t, keyEventRecommendContextRunes, len([]rune(text)))
	assert.Greater(t, truncated, 0, "超预算有明确截取量")
	// Boundary: exactly at budget → no truncation.
	small := strings.Repeat("短", keyEventRecommendContextRunes)
	got, cut := clampKeyEventPrompt(small)
	assert.Equal(t, small, got)
	assert.Equal(t, 0, cut)
}

func TestKeyEventRecommendation_PerBoardMutexSkipsSecondTask(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 1)

	mAny, _ := keyEventBoardMutexes.LoadOrStore(w.boardID, &sync.Mutex{})
	mu := mAny.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	called := false
	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		called = true
		return "[]", nil
	})
	runRec(context.Background(), w.boardID, w.reportID)

	assert.False(t, called, "每板块最多一个推荐任务：互斥让路不调 AI")
	run := getRun(t, w.reportID)
	assert.Nil(t, run, "让路任务不写 run 行（旧 token 由重建负责作废）")
}

func TestKeyEventRecommendation_LiveRunningVisibleAndCleared(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 1)

	release := make(chan struct{})
	entered := make(chan struct{})
	injectKeRecChat(t, func(ctx context.Context, sessionID, system, user string) (string, error) {
		close(entered)
		<-release
		return "[]", nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runRec(context.Background(), w.boardID, w.reportID)
	}()

	<-entered
	assert.True(t, IsKeyEventRecommendationActive(w.reportID), "在途任务进程内存活 → 读端显示 running")
	close(release)
	wg.Wait()
	assert.False(t, IsKeyEventRecommendationActive(w.reportID), "完成后清除存活标记 → 后续读端不再误报 running")
}

func TestKeyEventRecommendation_LateTaskTokenDropsOnRebuild(t *testing.T) {
	setupKeyEventRecommendTest(t)
	w := seedKeRecWorld(t, 1)

	// First task starts (token A held), then a rebuild restarts the run row
	// with token B; the first task's save lands late → whole batch dropped.
	require.NoError(t, repository.Repo.UpsertKeyEventRecommendationRun(w.reportID, w.boardID, "tok-A"))
	require.NoError(t, repository.Repo.UpsertKeyEventRecommendationRun(w.reportID, w.boardID, "tok-B"))
	saved, err := repository.Repo.SaveSuggestedMarkers(w.boardID, w.reportID, "tok-A", []repository.SuggestedKeyEvent{{
		SourceThreadID: w.threadIDs[0], Reason: "迟到建议", EvidenceIDs: []uint{1001},
	}})
	require.NoError(t, err)
	assert.Equal(t, 0, saved)
	var cnt int64
	require.NoError(t, repository.Repo.DB().Model(&repository.TopicKeyEventMarker{}).Count(&cnt).Error)
	assert.Equal(t, int64(0), cnt)
}
