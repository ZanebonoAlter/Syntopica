package boardhealth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// fakeBoardHealthLLM records calls and returns the scripted result/error.
type fakeBoardHealthLLM struct {
	calls   int
	prompts []string
	result  *BoardHealthLLMResult
	err     error
}

func (f *fakeBoardHealthLLM) ScoreBoardHealth(ctx context.Context, prompt string) (*BoardHealthLLMResult, error) {
	f.calls++
	f.prompts = append(f.prompts, prompt)
	return f.result, f.err
}

// setupTestDB spins the throwaway testcontainer PG (skipped under -short).
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.SetupTestDB(t)
}

func createBoardRow(t *testing.T, db *gorm.DB, label string, status string) models.SemanticLabel {
	t.Helper()
	board := models.SemanticLabel{Label: label, Slug: "bh-" + label, LabelType: "board", Status: status}
	require.NoError(t, db.Create(&board).Error)
	return board
}

// captureBoardHealthLogs 把全局日志重定向到 buffer（断言轮次日志口径）。
func captureBoardHealthLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	logging.SetWriters(buf, buf)
	t.Cleanup(func() { logging.ResetWriters() })
	return buf
}

// seedBoardHealthSnapshot 预置一条「有裁决」的快照（新鲜度守卫用例）：
// 守卫只把带评分的快照算新鲜（review M-1），所以必须带 llm_score。
func seedBoardHealthSnapshot(t *testing.T, db *gorm.DB, boardID uint, checkedAt time.Time) {
	t.Helper()
	score := 80.0
	comment := "种子裁决"
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID:      boardID,
		CheckedAt:            checkedAt,
		Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}},
		UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
		LLMScore:             &score,
		LLMComment:           &comment,
	}).Error)
}

// seedDegradedSnapshot 预置一条降级快照（llm_score = NULL）：它 MUST NOT 把
// 该板算成新鲜，否则用户想手动重跑补依据会被 6h 守卫挡住（review M-1）。
func seedDegradedSnapshot(t *testing.T, db *gorm.DB, boardID uint, checkedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID:      boardID,
		CheckedAt:            checkedAt,
		Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}},
		UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
	}).Error)
}

// composeAuxLabels 造 n 个辅助标签并挂到 board 上（多标签板用例）。
func composeAuxLabels(t *testing.T, db *gorm.DB, boardID uint, n int) []uint {
	t.Helper()
	ids := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		aux := createAuxLabel(t, db, fmt.Sprintf("构成%d", i), "active")
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: boardID, AuxiliaryLabelID: aux.ID}).Error)
		ids = append(ids, aux.ID)
	}
	return ids
}

// keepTriageItems 造 n 条合法分诊条目（aux id 取前 n 个）。
func keepTriageItems(auxIDs []uint, n int) []TriageItem {
	items := make([]TriageItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, TriageItem{
			AuxID:    auxIDs[i],
			Category: "keep",
			Evidence: fmt.Sprintf("cosine_mean=0.71 且无磁铁嫌疑 #%d", i+1),
		})
	}
	return items
}

// 用例来源：test-cases.md 主链路步 6/7（LLM 产出评分与评语 / LLM 失败降级不丢统计）。

func TestCheckupBoardLLMSuccessAndDegrade(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "AI前沿", "active")

	// 正常路：score/comment 落快照。
	okLLM := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 78, Comment: "构成清晰", Notify: false}}
	config := Config{MinSampleSize: 5, AlertThreshold: 60, NotifyCooldownDays: 3}
	require.NoError(t, CheckupBoard(context.Background(), db, okLLM, config, &board, time.Now()))
	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).First(&snapshot).Error)
	require.NotNil(t, snapshot.LLMScore)
	require.InDelta(t, 78.0, *snapshot.LLMScore, 1e-9)
	require.NotNil(t, snapshot.LLMComment)
	require.Equal(t, "构成清晰", *snapshot.LLMComment)
	require.Nil(t, snapshot.Metrics.Triage, "零构成标签的合法空 triage 落库为 null")
	require.Equal(t, 1, okLLM.calls, "one merged adjudication call per board per round")
	require.Contains(t, okLLM.prompts[0], "AI前沿")
	require.Contains(t, okLLM.prompts[0], "体检统计数据")

	// 降级路：LLM 失败 → 统计照常落快照，score/comment/triage 为 NULL，不重试。
	badLLM := &fakeBoardHealthLLM{err: errors.New("timeout")}
	require.NoError(t, CheckupBoard(context.Background(), db, badLLM, config, &board, time.Now()))
	var degraded tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ? AND llm_score IS NULL", board.ID).First(&degraded).Error)
	require.Nil(t, degraded.LLMScore)
	require.Nil(t, degraded.LLMComment)
	require.Nil(t, degraded.Metrics.Triage, "triage 与 score 同降级（超时/坏 JSON 路径）")
	require.Equal(t, 1, badLLM.calls, "degraded round must not retry")
	// 两轮两条快照（同日多条并存，spec: 历史快照不被覆盖）。
	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

// 白盒附加：LLM 返回超范围分数 → 视为非法降级（score=NULL）。
func TestCheckupBoardLLMScoreOutOfRangeDegrades(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "越界", "active")
	badLLM := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 150}}
	err := CheckupBoard(context.Background(), db, badLLM, Config{}, &board, time.Now())
	require.NoError(t, err, "degradation is not a board failure")
	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).First(&snapshot).Error)
	require.Nil(t, snapshot.LLMScore)
}

// 用例来源：test-cases.md 白盒附加「轮次」组 + 主链路步 11/12。
func TestRunCheckupRound(t *testing.T) {
	db := setupTestDB(t)
	boardA := createBoardRow(t, db, "A版", "active")
	boardB := createBoardRow(t, db, "B版", "active")

	var order []uint
	original := checkupOneBoard
	defer func() { checkupOneBoard = original }()
	checkupOneBoard = func(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel) (boardOutcome, error) {
		order = append(order, board.ID)
		if board.ID == boardA.ID {
			return boardOutcome{}, errors.New("statistics blew up") // A 失败：记日志继续 B
		}
		return checkupBoardSafely(ctx, db, llm, config, board)
	}
	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80, Comment: "ok"}}
	RunCheckup(context.Background(), db, llm)

	require.Equal(t, []uint{boardA.ID, boardB.ID}, order, "id ASC order, failure isolated")
	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", boardB.ID).Count(&count).Error)
	require.Equal(t, int64(1), count, "sibling board still checked")
	var failedCount int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", boardA.ID).Count(&failedCount).Error)
	require.Zero(t, failedCount, "failed board leaves no snapshot")
}

func TestRunCheckupZeroBoards(t *testing.T) {
	db := setupTestDB(t)
	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{})
	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&count).Error)
	require.Zero(t, count, "empty round succeeds with no snapshots")
}

// 白盒附加「轮次：LLM 全失败」：全部快照 score=NULL，轮次成功，零通知。
func TestRunCheckupLLMAllFail(t *testing.T) {
	db := setupTestDB(t)
	createBoardRow(t, db, "全降级", "active")
	original := notifyBoardHealthAlert
	defer func() { notifyBoardHealthAlert = original }()
	var notified int
	notifyBoardHealthAlert = func(boardID uint, boardLabel string, score *float64, llmComment string) { notified++ }

	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{err: errors.New("llm down")})

	var snapshots []tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Find(&snapshots).Error)
	require.Len(t, snapshots, 1)
	require.Nil(t, snapshots[0].LLMScore)
	require.Zero(t, notified, "degraded rounds never notify")
}

// 主链路步 12（体检不改匹配数据）：轮次前后匹配表与建议表无写入。
func TestRunCheckupNeverWritesMatchData(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "只读", "active")
	// 预置一行匹配数据 + 一行 pending 建议：体检后必须原样。
	match := tagmodels.TopicTagBoardLabel{TopicTagID: 424242, SemanticBoardID: board.ID, Score: 0.7, MatchReason: "direct_hit"}
	require.NoError(t, db.Create(&match).Error)
	suggestion := tagmodels.BoardUpgradeSuggestion{SuggestionHash: "health-readonly-probe", Status: "pending", Mode: "expand:aux", Decision: "merge_into_existing", BoardLabel: "只读", TargetBoardID: &board.ID}
	require.NoError(t, db.Create(&suggestion).Error)

	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}})

	var afterMatch tagmodels.TopicTagBoardLabel
	require.NoError(t, db.Where("topic_tag_id = ?", 424242).First(&afterMatch).Error)
	require.Equal(t, 0.7, afterMatch.Score)
	require.Equal(t, "direct_hit", afterMatch.MatchReason)
	var pendingCount int64
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Where("status = ?", "pending").Count(&pendingCount).Error)
	require.Equal(t, int64(1), pendingCount, "checkup must not create upgrade suggestions")
}

// ── fix-board-health-triage-degrade：漏判补齐 / 日志口径 / 守卫 / 子超时 ──

// TestCheckupBoardFillsMissingTriage —— 漏判补齐的主链路落点：47 个构成标签
// 的板，桩 LLM 只回 45 条（覆盖率 95.7% ≥ 下限）→ 评分与评语保留，triage
// 补齐到 47 条，缺项为「证据不足」（MUST NOT 猜 keep/remove）。
func TestCheckupBoardFillsMissingTriage(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "多标签板", "active")
	auxIDs := composeAuxLabels(t, db, board.ID, 47)

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 78, Comment: "总体健康", Triage: keepTriageItems(auxIDs, 45),
	}}
	require.NoError(t, CheckupBoard(context.Background(), db, llm, Config{}, &board, time.Now()))

	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)
	require.NotNil(t, snapshot.LLMScore, "漏判不得作废整轮裁决")
	require.InDelta(t, 78.0, *snapshot.LLMScore, 1e-9)
	require.NotNil(t, snapshot.LLMComment)
	require.Len(t, snapshot.Metrics.Triage, 47, "补齐后 triage 长度 == 构成标签数")

	insufficient := 0
	for _, item := range snapshot.Metrics.Triage {
		if item.Category == "insufficient" {
			insufficient++
			require.Empty(t, item.Suggestion, "补齐项不得携带建议文案")
			require.Contains(t, item.Evidence, "本轮 LLM 未覆盖该标签")
		}
	}
	require.Equal(t, 2, insufficient, "补齐条数 == 模型漏判条数")
}

// TestCheckupBoardDegradesWhenCoverageTooLow —— 覆盖率过低仍整轮降级：
// 47 个构成标签只回 9 条（19%）→ score/comment/triage 全空，统计照落。
func TestCheckupBoardDegradesWhenCoverageTooLow(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "低覆盖板", "active")
	auxIDs := composeAuxLabels(t, db, board.ID, 47)

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 70, Comment: "x", Triage: keepTriageItems(auxIDs, 9),
	}}
	require.NoError(t, CheckupBoard(context.Background(), db, llm, Config{}, &board, time.Now()))

	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)
	require.Nil(t, snapshot.LLMScore, "覆盖率过低 → 整轮裁决降级")
	require.Nil(t, snapshot.LLMComment)
	require.Nil(t, snapshot.Metrics.Triage)
}

// TestRunCheckupRoundLogs —— 轮次日志口径：每板一行（含 filled= / degraded=），
// 整轮一行（含 boards= / degraded= / filled= / skipped_fresh=）。成功轮也打。
func TestRunCheckupRoundLogs(t *testing.T) {
	db := setupTestDB(t)
	createBoardRow(t, db, "日志板", "active")

	buf := captureBoardHealthLogs(t)
	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}})
	out := buf.String()

	require.Contains(t, out, "[board-health] board=")
	require.Contains(t, out, "filled=")
	require.Contains(t, out, "degraded=false", "成功轮也必须打一行")
	require.Contains(t, out, "[board-health] round finished boards=1 processed=1 degraded=0 filled=0 skipped_fresh=0")
}

// TestRunCheckupDegradedBoardLogsReason —— 降级板的日志带 reason，且覆盖率
// 不足的 reason 必须含 covered=N/M (xx.x%) below min coverage。
func TestRunCheckupDegradedBoardLogsReason(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "降级板", "active")
	auxIDs := composeAuxLabels(t, db, board.ID, 10)

	buf := captureBoardHealthLogs(t)
	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 70, Triage: keepTriageItems(auxIDs, 4), // 40% < 50% → 整轮降级
	}})
	out := buf.String()

	require.Contains(t, out, "degraded=true")
	require.Contains(t, out, "reason=")
	require.Contains(t, out, "triage covers 4/10 (40.0%) below min coverage 50%")
	require.Contains(t, out, "round finished boards=1 processed=1 degraded=1")
}

// TestRunCheckupWithOptionsFreshnessGuard —— 新鲜度守卫：窗口内有快照的板
// 整板跳过（零 LLM 调用、不写快照）；边界 5h59m 跳过 / 6h01m 照跑。
func TestRunCheckupWithOptionsFreshnessGuard(t *testing.T) {
	db := setupTestDB(t)
	fresh := createBoardRow(t, db, "新鲜板", "active")
	stale := createBoardRow(t, db, "过期板", "active")

	seedBoardHealthSnapshot(t, db, fresh.ID, time.Now().Add(-(5*time.Hour + 59*time.Minute)))
	seedBoardHealthSnapshot(t, db, stale.ID, time.Now().Add(-(6*time.Hour + 1*time.Minute)))

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}}
	RunCheckupWithOptions(context.Background(), db, llm, Options{})

	require.Equal(t, 1, llm.calls, "只有过期板真跑（新鲜板被守卫跳过）")
	var freshCount, staleCount int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", fresh.ID).Count(&freshCount).Error)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", stale.ID).Count(&staleCount).Error)
	require.Equal(t, int64(1), freshCount, "被跳过的板不写新快照")
	require.Equal(t, int64(2), staleCount, "过期板写了新快照")

	// 守卫跳过计数进轮次日志。
	buf := captureBoardHealthLogs(t)
	RunCheckupWithOptions(context.Background(), db, llm, Options{})
	require.Contains(t, buf.String(), "skipped_fresh=2")
}

// TestRunCheckupGuardIgnoresDegradedSnapshot —— 降级快照（llm_score=NULL）
// 不把板块算成新鲜：6h 内刚降级过的板，手动重跑仍会真跑一轮（review M-1）。
func TestRunCheckupGuardIgnoresDegradedSnapshot(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "刚降级板", "active")
	seedDegradedSnapshot(t, db, board.ID, time.Now().Add(-10*time.Minute))

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}}
	RunCheckupWithOptions(context.Background(), db, llm, Options{})
	require.Equal(t, 1, llm.calls, "降级快照不得把板挡在 6h 窗口外")

	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&count).Error)
	require.Equal(t, int64(2), count, "降级快照 + 本轮新快照")
}

// TestRunCheckupZeroGuardSkipsNothing —— RunCheckup（零守卫全量）不查新鲜度：
// 6h 内刚体检过的板照跑（CLI 与既有调用点行为不变）。
func TestRunCheckupZeroGuardSkipsNothing(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "零守卫板", "active")
	seedBoardHealthSnapshot(t, db, board.ID, time.Now())

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}}
	RunCheckup(context.Background(), db, llm)
	require.Equal(t, 1, llm.calls, "零守卫路径不受新鲜度窗口影响")
}

// TestRunCheckupWithOptionsBoardIDsScope —— BoardIDs 限定范围：只处理请求的
// 板块；非版块 / 已停用的 id 静默不产出快照。
func TestRunCheckupWithOptionsBoardIDsScope(t *testing.T) {
	db := setupTestDB(t)
	boardA := createBoardRow(t, db, "范围A", "active")
	boardB := createBoardRow(t, db, "范围B", "active")
	disabled := createBoardRow(t, db, "停用板", "disabled")
	aux := createAuxLabel(t, db, "非版块", "active")

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}}
	RunCheckupWithOptions(context.Background(), db, llm, Options{
		BoardIDs: []uint{boardA.ID, disabled.ID, aux.ID},
	})

	require.Equal(t, 1, llm.calls, "只有范围内的活跃版块被体检")
	var aCount, bCount, disabledCount int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", boardA.ID).Count(&aCount).Error)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", boardB.ID).Count(&bCount).Error)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", disabled.ID).Count(&disabledCount).Error)
	require.Equal(t, int64(1), aCount)
	require.Zero(t, bCount, "范围外的板块不体检")
	require.Zero(t, disabledCount, "已停用版块不产快照")
}

// TestRunCheckupPerBoardTimeout —— 单板子超时：慢板只降级自己（无快照），
// 兄弟板照常完成（轮次不中断）。
func TestRunCheckupPerBoardTimeout(t *testing.T) {
	db := setupTestDB(t)
	slow := createBoardRow(t, db, "慢板", "active")
	fast := createBoardRow(t, db, "快板", "active")

	prevTimeout := perBoardTimeout
	perBoardTimeout = 50 * time.Millisecond
	t.Cleanup(func() { perBoardTimeout = prevTimeout })

	original := checkupOneBoard
	t.Cleanup(func() { checkupOneBoard = original })
	checkupOneBoard = func(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel) (boardOutcome, error) {
		if board.ID == slow.ID {
			<-ctx.Done() // 卡到子超时
			return boardOutcome{}, ctx.Err()
		}
		return checkupBoardSafely(ctx, db, llm, config, board)
	}

	buf := captureBoardHealthLogs(t)
	RunCheckup(context.Background(), db, &fakeBoardHealthLLM{result: &BoardHealthLLMResult{Score: 80}})
	out := buf.String()

	var slowCount, fastCount int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", slow.ID).Count(&slowCount).Error)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", fast.ID).Count(&fastCount).Error)
	require.Zero(t, slowCount, "超时板不写快照")
	require.Equal(t, int64(1), fastCount, "兄弟板不受慢板超时影响")
	require.Contains(t, out, "degraded=true")
	require.Contains(t, out, "elapsed=")
}

// ── fix-board-health-magnet-signal：规则层闸门落库 ──────────────────────

// TestCheckupBoardActionableRatioGate —— 2026-11-09 事故形态的端到端回归：
// 47 个构成标签全被判 rewrite（可执行占比 100%）→ G3 占比闸门把全部可执行
// 项降级为证据不足，快照记录 triage_guard（degraded / ratio / threshold），
// 但评分与评语照常保留（闸门不是整轮降级）。
func TestCheckupBoardActionableRatioGate(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "闸门测试板", "active")
	auxIDs := composeAuxLabels(t, db, board.ID, 47)

	items := make([]TriageItem, 0, 47)
	for i, id := range auxIDs {
		items = append(items, TriageItem{
			AuxID:      id,
			Category:   "rewrite",
			Evidence:   fmt.Sprintf("desc 预筛命中此次 #%d", i+1),
			Suggestion: "百科式新描述",
		})
	}
	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 58, Comment: "贴合度分化明显", Triage: items,
	}}
	config := Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5, MaxActionableRatio: 0.8}
	require.NoError(t, CheckupBoard(context.Background(), db, llm, config, &board, time.Now()))

	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)

	// 闸门结论落库。
	require.NotNil(t, snapshot.Metrics.TriageGuard, "闸门结论必须随快照落库")
	guard := snapshot.Metrics.TriageGuard
	require.True(t, guard.Degraded)
	require.Equal(t, 47, guard.Actionable)
	require.Equal(t, 47, guard.Total)
	require.InDelta(t, 1.0, guard.Ratio, 1e-9)
	require.InDelta(t, 0.8, guard.Threshold, 1e-9)
	require.Contains(t, guard.Reason, "超过阈值")

	// 分诊清单全部降级为证据不足（不再呈现为可执行建议）。
	require.Len(t, snapshot.Metrics.Triage, 47)
	for _, item := range snapshot.Metrics.Triage {
		require.Equal(t, "insufficient", item.Category)
		require.Empty(t, item.Suggestion, "降级项不得携带建议文案")
		require.Contains(t, item.Evidence, "超过阈值")
	}

	// 闸门不影响评分与评语。
	require.NotNil(t, snapshot.LLMScore)
	require.InDelta(t, 58.0, *snapshot.LLMScore, 1e-9)
	require.NotNil(t, snapshot.LLMComment)
	require.Equal(t, "贴合度分化明显", *snapshot.LLMComment)
}

// TestCheckupBoardCoreSubjectProtected —— 核心主体被 LLM 判 rewrite → 规则层
// 改判 keep（证据含「核心主体保护」+ 原判），且不进入「待核对」可执行集。
//
// 核心主体的命中路径按 review R-6 收紧后的规则：description / aliases 命中即
// 成立；**仅**标题命中需标签名 ≥ 3 字符（「伊朗」2 字、只出现在标题里 → 不
// 成立）。本用例走 description 命中路径（design D10 的实际保护路径）。
func TestCheckupBoardCoreSubjectProtected(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "伊朗局势观察", "active")
	// description 明示核心主体（design D10：补齐描述才是本板保护「伊朗」的
	// 实际路径——标题里的「伊朗」只有 2 字，标题命中不成立）。
	board.Description = "本板讨论伊朗局势，核心主体是伊朗与美伊关系。"
	core := createAuxLabel(t, db, "伊朗", "active")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: core.ID}).Error)

	llm := &fakeBoardHealthLLM{result: &BoardHealthLLMResult{
		Score: 70, Comment: "ok",
		Triage: []TriageItem{{
			AuxID: core.ID, Category: "rewrite",
			Evidence: "近邻窄化", Suggestion: "伊朗，中东国家",
		}},
	}}
	config := Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5, MaxActionableRatio: 0.8}
	require.NoError(t, CheckupBoard(context.Background(), db, llm, config, &board, time.Now()))

	var snapshot tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ?", board.ID).Order("id DESC").First(&snapshot).Error)

	// 核心主体标记随快照落库。
	require.Len(t, snapshot.Metrics.Labels, 1)
	require.True(t, snapshot.Metrics.Labels[0].CoreSubject, "description 含标签名 → 核心主体")

	// 分诊项被规则层改判 keep（不再是可执行建议）。
	require.Len(t, snapshot.Metrics.Triage, 1)
	item := snapshot.Metrics.Triage[0]
	require.Equal(t, "keep", item.Category)
	require.Contains(t, item.Evidence, "核心主体保护")
	require.Contains(t, item.Evidence, "出现在板块描述/别名中（或标题中且标签名 ≥ 3 字）")
	require.Contains(t, item.Evidence, "rewrite")
	require.Empty(t, item.Suggestion, "改判 keep 不得携带重写文案")

	// 闸门记录：占比取 LLM 原始裁决（本轮原始 1 条 rewrite / 总数 1 = 1.0）
	// → 超阈值 → degraded=true；但该条已被 G1 改判 keep，不是可执行项。
	require.NotNil(t, snapshot.Metrics.TriageGuard)
	require.Equal(t, 1, snapshot.Metrics.TriageGuard.Actionable)
	require.Equal(t, 1, snapshot.Metrics.TriageGuard.Total)
	require.InDelta(t, 1.0, snapshot.Metrics.TriageGuard.Ratio, 1e-9)
	require.True(t, snapshot.Metrics.TriageGuard.Degraded)
}
