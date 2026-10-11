package boardhealth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// ErrBoardNotFound maps to HTTP 404 at the handler (endpoint discipline:
// disabled / non-board is NOT 404 — it returns the empty state).
var ErrBoardNotFound = errors.New("semantic board not found")

// TrendPoint is one point of the score trend series (ascending by time).
type TrendPoint struct {
	CheckedAt time.Time `json:"checked_at"`
	LLMScore  *float64  `json:"llm_score"`
}

// scorePromptCandidates limits how many uncaptured candidates enter the LLM
// prompt (the full list is persisted; the prompt needs the head only).
const scorePromptCandidates = 10

// RunCheckup executes one full checkup round over every active board:
// statistics → factual profile diff → LLM adjudication → snapshot insert →
// alert adjudication. Chained synchronously after a successful daily-report
// run (design D3). Semantics:
//   - pure read for everything except board_health_snapshots and (rarely)
//     one notification per board — matching results are never touched and no
//     upgrade suggestion is ever created (spec: 体检不改匹配数据 / 体检轮次
//     不落扩充建议);
//   - a per-board failure (statistics error, LLM timeout, insert error) is
//     logged and the round moves on to sibling boards;
//   - a panic inside a board's checkup is recovered so the daily-report job
//     outcome is never polluted.
func RunCheckup(ctx context.Context, db *gorm.DB, llm BoardHealthLLM) {
	config := LoadConfig(ctx, db)
	var boards []models.SemanticLabel
	if err := db.WithContext(ctx).
		Where("label_type = ? AND status = ?", "board", "active").
		Order("id ASC").
		Find(&boards).Error; err != nil {
		logging.Errorf("[board-health] load active boards failed, round aborted: %v", err)
		return
	}
	for i := range boards {
		board := &boards[i]
		if err := checkupOneBoard(ctx, db, llm, config, board); err != nil {
			logging.Warnf("[board-health] board %d (%s) checkup failed, continuing with siblings: %v", board.ID, board.Label, err)
		}
	}
}

// checkupOneBoard is the per-board seam (package var) so tests can inject
// per-board failures/panics and observe ordering without fixtures.
var checkupOneBoard = checkupBoardSafely

// checkupBoardSafely wraps one board's checkup with panic recovery.
func checkupBoardSafely(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return CheckupBoard(ctx, db, llm, config, board, time.Now())
}

// CheckupBoard runs the full pipeline for one board and persists the
// snapshot. The LLM verdict failing degrades (score/comment NULL) but still
// persists statistics — a degraded round never fails the board.
func CheckupBoard(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel, now time.Time) error {
	metrics, candidates, err := collectBoardStatistics(ctx, db, board.ID, config)
	if err != nil {
		return fmt.Errorf("collect statistics: %w", err)
	}

	var score *float64
	var comment *string
	notifyVerdict := false
	if llm != nil {
		result, llmErr := adjudicateBoardHealth(ctx, llm, board, metrics, candidates)
		if llmErr != nil {
			logging.Warnf("[board-health] board %d LLM adjudication degraded (score=NULL, no retry): %v", board.ID, llmErr)
		} else if result != nil {
			s := float64(result.Score)
			c := result.Comment
			score, comment = &s, &c
			notifyVerdict = result.Notify
		}
	}

	snapshot := &tagmodels.BoardHealthSnapshot{
		SemanticBoardID:      board.ID,
		CheckedAt:            now,
		Metrics:              *metrics,
		UncapturedCandidates: candidates,
		LLMScore:             score,
		LLMComment:           comment,
	}
	if err := db.WithContext(ctx).Create(snapshot).Error; err != nil {
		return fmt.Errorf("persist snapshot: %w", err)
	}

	notifyIfAlert(ctx, db, config, board, score, notifyVerdict, time.Now())
	return nil
}

// collectBoardStatistics gathers the pure-read part: board-level metrics over
// all matched rows + label-level affinity distributions (composition-label
// embedding × every mounted tag identity embedding) + the factual profile
// diff.
func collectBoardStatistics(ctx context.Context, db *gorm.DB, boardID uint, config Config) (*tagmodels.BoardHealthMetrics, []tagmodels.BoardHealthCandidate, error) {
	rows, err := loadMatchedRows(ctx, db, boardID)
	if err != nil {
		return nil, nil, fmt.Errorf("load matched rows: %w", err)
	}
	boardMetrics := ComputeBoardMetrics(rows)

	compositions, auxEmb, err := loadCompositionLabels(ctx, db, boardID)
	if err != nil {
		return nil, nil, fmt.Errorf("load composition: %w", err)
	}

	tagIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		tagIDs = append(tagIDs, row.TopicTagID)
	}
	tagEmb, err := loadTagIdentityEmbeddings(ctx, db, tagIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("load tag embeddings: %w", err)
	}

	labels, vectorMissing := computeLabelAttribution(rows, compositions, auxEmb, tagEmb, config)
	boardMetrics.VectorMissing = vectorMissing

	candidates, err := UncapturedCandidates(ctx, db, boardID)
	if err != nil {
		return nil, nil, fmt.Errorf("uncaptured candidates: %w", err)
	}
	return &tagmodels.BoardHealthMetrics{Board: boardMetrics, Labels: labels}, candidates, nil
}

// adjudicateBoardHealth renders the prompt and calls the LLM. Errors degrade
// upstream; a nil result with nil error is treated as degraded too. The
// score range check lives HERE (the persistence gate) so every LLM
// implementation — production or test fake — is bounded the same way.
func adjudicateBoardHealth(ctx context.Context, llm BoardHealthLLM, board *models.SemanticLabel, metrics *tagmodels.BoardHealthMetrics, candidates []tagmodels.BoardHealthCandidate) (*BoardHealthLLMResult, error) {
	metricsJSON, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("marshal metrics: %w", err)
	}
	candidateLines := make([]string, 0, len(candidates))
	for i, c := range candidates {
		if i >= scorePromptCandidates {
			break
		}
		candidateLines = append(candidateLines, fmt.Sprintf("%s（成员 tags=%d，文章覆盖=%d）", c.Label, c.MemberTags, c.MemberArticles))
	}
	result, err := llm.ScoreBoardHealth(ctx, buildScorePrompt(board, string(metricsJSON), candidateLines))
	if err != nil {
		return nil, err
	}
	if result == nil || result.Score < 0 || result.Score > 100 {
		return nil, fmt.Errorf("llm score out of range")
	}
	return result, nil
}

// ── Read side (endpoint + expand recall; read-only, no side effects) ────────

// LatestSnapshot returns the board's newest snapshot, or (nil, nil) when the
// board has never been checked.
func LatestSnapshot(ctx context.Context, db *gorm.DB, boardID uint) (*tagmodels.BoardHealthSnapshot, error) {
	var snapshot tagmodels.BoardHealthSnapshot
	err := db.WithContext(ctx).
		Where("semantic_board_id = ?", boardID).
		Order("checked_at DESC, id DESC").
		First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &snapshot, nil
}

// HealthCheckupView assembles the read-model for GET /health-checkup: the
// latest snapshot plus the trend series (ascending checked_at, newest N).
// Board missing entirely → ErrBoardNotFound (404); disabled / non-board →
// empty view (latest=nil, trend=[]) per the endpoint discipline that
// disabled is not 404.
func HealthCheckupView(ctx context.Context, db *gorm.DB, boardID uint, trendN int) (*tagmodels.BoardHealthSnapshot, []TrendPoint, error) {
	var board models.SemanticLabel
	err := db.WithContext(ctx).
		Select("id, label_type, status").
		Where("id = ?", boardID).
		First(&board).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrBoardNotFound
		}
		return nil, nil, err
	}
	if board.LabelType != "board" || board.Status != "active" {
		return nil, []TrendPoint{}, nil
	}

	latest, err := LatestSnapshot(ctx, db, boardID)
	if err != nil {
		return nil, nil, err
	}

	var rows = make([]TrendPoint, 0, trendN)
	if err := db.WithContext(ctx).
		Table("board_health_snapshots").
		Select("checked_at, llm_score").
		Where("semantic_board_id = ?", boardID).
		Order("checked_at DESC, id DESC").
		Limit(trendN).
		Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	// Descending fetch (newest N), returned ascending (spec: 按时间升序).
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return latest, rows, nil
}
