package boardhealth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
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
	require.Equal(t, 1, okLLM.calls, "one merged adjudication call per board per round")
	require.Contains(t, okLLM.prompts[0], "AI前沿")
	require.Contains(t, okLLM.prompts[0], "体检统计数据")

	// 降级路：LLM 失败 → 统计照常落快照，score/comment 为 NULL，不重试。
	badLLM := &fakeBoardHealthLLM{err: errors.New("timeout")}
	require.NoError(t, CheckupBoard(context.Background(), db, badLLM, config, &board, time.Now()))
	var degraded tagmodels.BoardHealthSnapshot
	require.NoError(t, db.Where("semantic_board_id = ? AND llm_score IS NULL", board.ID).First(&degraded).Error)
	require.Nil(t, degraded.LLMScore)
	require.Nil(t, degraded.LLMComment)
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
	checkupOneBoard = func(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel) error {
		order = append(order, board.ID)
		if board.ID == boardA.ID {
			return errors.New("statistics blew up") // A 失败：记日志继续 B
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
