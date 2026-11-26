package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 日报缺档补填编排（add-daily-report-gap-backfill design D2/D3/D6）──
//
// 一条流水线、一个闸门：补填复用既有生成入口（GenerateAndSaveReport）与既有
// 闸门（TryAcquireDailyReportGeneration），不新增第二条生成路径。
//
// 三条不变量：
//  ① 只补空——已有报告的格子一律跳过，绝不重建/覆盖（重建会删 section/thread，
//     页边注与人工锚定全部错位；重建只由用户显式发起）；
//  ② 旧到新——按报告日升序处理，保证 prev_report_id 链成立；
//  ③ 幂等——中断后重跑只补仍缺的格子。

// BackfillBudgetMinutesKey is the ai_settings key driving the whole-round
// budget of one gap-backfill run (minutes). Default 60 minutes.
const BackfillBudgetMinutesKey = "daily_report_backfill_budget_minutes"

// DefaultBackfillBudgetMinutes is the fallback when the ai_settings row is
// missing or invalid.
const DefaultBackfillBudgetMinutes = 60

// LoadBackfillBudget reads the whole-round budget from ai_settings
// (daily_report_backfill_budget_minutes, default 60 minutes). Reading is
// best-effort: a missing/invalid row falls back to the default.
func LoadBackfillBudget(db *gorm.DB) time.Duration {
	minutes := DefaultBackfillBudgetMinutes
	if db == nil {
		return time.Duration(minutes) * time.Minute
	}
	var row struct {
		Value string
	}
	if err := db.Table("ai_settings").
		Select("value").
		Where("key = ?", BackfillBudgetMinutesKey).
		Take(&row).Error; err != nil {
		return time.Duration(minutes) * time.Minute
	}
	if v, err := strconv.Atoi(strings.TrimSpace(row.Value)); err == nil && v > 0 {
		minutes = v
	}
	return time.Duration(minutes) * time.Minute
}

// Package-level seams (same pattern as the scheduler's generateAndSaveReport):
// tests substitute them without touching the real LLM pipeline / clock.
var (
	// backfillGenerateReport is the generation entry of one cell.
	backfillGenerateReport = GenerateAndSaveReport
	// backfillNow is the clock used for the budget guard (tests advance it).
	backfillNow = time.Now
	// backfillListGaps is the gap scan seam.
	backfillListGaps = ListDailyReportGaps
)

// ListDailyReportGaps returns every (board, date) cell inside the window that
// has candidate content but no report.
//
// 谓词与生成 Step1 一致（active + event + NOT direction_mismatch + 跨板排他
// NOT EXISTS），排他分差取与 collectBoardTags 同一个 ai_settings 键
// （semantic_board_report_exclusive_gap），故两处口径不会漂移。
func ListDailyReportGaps(windowStart, windowEnd time.Time) ([]repository.DailyReportGap, error) {
	cfg := loadDailyReportCollectConfig(repository.Repo.DB())
	return repository.Repo.ListDailyReportGaps(windowStart, windowEnd, cfg.ExclusiveGap)
}

// GapBackfillOptions — one gap-backfill run's parameters.
type GapBackfillOptions struct {
	// WindowStart / WindowEnd bound the scanned window (inclusive).
	WindowStart time.Time
	WindowEnd   time.Time
	// TriggerScope: manual（手动端点）/ scheduled（调度器）。
	TriggerScope string
	// RunID identifies the run (also the WS terminal message's run_id).
	// Empty → the service generates one.
	RunID string
	// BoardIDs scopes the run to a board subset (empty = all boards).
	BoardIDs []uint
	// Budget bounds the whole round. <= 0 → read from ai_settings.
	Budget time.Duration
	// Progress is called after every cell (nil = no progress reporting).
	Progress func(GapBackfillProgress)
}

// GapBackfillProgress — per-cell progress of a gap-backfill run.
type GapBackfillProgress struct {
	RunID      string `json:"run_id"`
	Date       string `json:"date"`
	BoardID    uint   `json:"board_id"`
	BoardLabel string `json:"board_label"`
	// Status: completed / failed / empty.
	Status    string `json:"status"`
	Filled    int    `json:"filled"`
	Failed    int    `json:"failed"`
	Total     int    `json:"total"`
	Remaining int    `json:"remaining"`
}

// GapBackfillResult — the terminal outcome of one gap-backfill run.
type GapBackfillResult struct {
	RunID       string                         `json:"run_id"`
	Status      string                         `json:"status"`
	TotalCells  int                            `json:"total_cells"`
	Filled      int                            `json:"filled"`
	Failed      int                            `json:"failed"`
	Empty       int                            `json:"empty"`
	Skipped     int                            `json:"skipped"`
	Remaining   int                            `json:"remaining"`
	WindowStart time.Time                      `json:"window_start"`
	WindowEnd   time.Time                      `json:"window_end"`
	StartedAt   time.Time                      `json:"started_at"`
	FinishedAt  time.Time                      `json:"finished_at"`
	Details     []repository.BackfillDayDetail `json:"details"`
}

// RunGapBackfill fills the missing daily reports inside the window.
//
// 顺序：先扫缺口（失败即整体失败，不留半截运行记录），再建运行留档行，然后
// 逐格补：只补空、旧到新、单格失败不阻断兄弟格、预算耗尽即停并记剩余缺口。
//
// 终态四态：success（全补齐或无缺口）/ partial（部分失败）/ failed（全部失败）/
// budget_exhausted（预算耗尽，已完成的保留）。
func RunGapBackfill(ctx context.Context, opts GapBackfillOptions) (*GapBackfillResult, error) {
	if opts.TriggerScope == "" {
		opts.TriggerScope = repository.BackfillTriggerScopeManual
	}
	if opts.Budget <= 0 {
		opts.Budget = LoadBackfillBudget(repository.Repo.DB())
	}
	startedAt := backfillNow()

	// 缺口扫描先行：扫描失败 = 这次运行根本没开始，不留运行记录、不发通知。
	gaps, err := backfillListGaps(opts.WindowStart, opts.WindowEnd)
	if err != nil {
		return nil, fmt.Errorf("list daily report gaps: %w", err)
	}

	runID := opts.RunID
	if runID == "" {
		runID = uuid.New().String()
	}
	run := &repository.DailyReportBackfillRun{
		RunID:        runID,
		TriggerScope: opts.TriggerScope,
		WindowStart:  opts.WindowStart,
		WindowEnd:    opts.WindowEnd,
		Status:       repository.BackfillRunStatusRunning,
		DailyDetails: repository.JSON("[]"),
		StartedAt:    startedAt,
	}
	if err := repository.Repo.CreateBackfillRun(run); err != nil {
		return nil, fmt.Errorf("create backfill run: %w", err)
	}

	cells := orderBackfillCells(gaps)
	if len(opts.BoardIDs) > 0 {
		allowed := make(map[uint]struct{}, len(opts.BoardIDs))
		for _, id := range opts.BoardIDs {
			allowed[id] = struct{}{}
		}
		filtered := cells[:0]
		for _, c := range cells {
			if _, ok := allowed[c.gap.BoardID]; ok {
				filtered = append(filtered, c)
			}
		}
		cells = filtered
	}
	result := &GapBackfillResult{
		RunID:       runID,
		TotalCells:  len(cells),
		WindowStart: opts.WindowStart,
		WindowEnd:   opts.WindowEnd,
		StartedAt:   startedAt,
		Details:     []repository.BackfillDayDetail{},
	}

	dayStats := newBackfillDayStats(cells)
	budgetExhausted := false
	processed := 0

	for i, cell := range cells {
		// 预算守卫：耗尽即停止启动新的生成（不打断在途的那一格），
		// 已完成的保留，剩下的记进 remaining 留给下一轮（顺延不丢）。
		if backfillNow().Sub(startedAt) >= opts.Budget {
			budgetExhausted = true
			logging.Warnf("daily-report: backfill budget exhausted after %s; remaining cells deferred",
				backfillNow().Sub(startedAt).Round(time.Second))
			break
		}
		if ctx != nil && ctx.Err() != nil {
			logging.Warnf("daily-report: backfill interrupted: %v", ctx.Err())
			break
		}
		processed = i + 1

		// 只补空：扫描与执行之间可能已有报告落地（并发生成 / 上一轮补齐），
		// 执行前再查一次，绝不覆盖既有报告。
		//
		// 竞态由域闸兜住：补填整轮持有日报生成闸（service.TryAcquireDailyReportGeneration），
		// 定时任务与手动生成共用同一把闸——闸内不可能有并发的日报生成在写同一格。
		exists, existsErr := repository.Repo.ReportExistsForBoardDate(cell.gap.BoardID, cell.date)
		if existsErr != nil {
			logging.Warnf("daily-report: backfill existence check failed for board %d on %s: %v",
				cell.gap.BoardID, cell.gap.Date, existsErr)
			dayStats.failed(cell.gap.Date)
			result.Failed++
			continue
		}
		if exists {
			result.Skipped++
			continue
		}

		report, genErr := backfillGenerateReport(ctx, cell.gap.BoardID, cell.date)
		if genErr != nil {
			// 单格失败不阻断兄弟格（兄弟格继续补）。
			logging.Warnf("daily-report: backfill failed for board %d on %s: %v",
				cell.gap.BoardID, cell.gap.Date, genErr)
			dayStats.failed(cell.gap.Date)
			result.Failed++
			emitBackfillProgress(opts.Progress, GapBackfillProgress{
				RunID: runID, Date: cell.gap.Date, BoardID: cell.gap.BoardID,
				BoardLabel: cell.gap.BoardLabel, Status: "failed",
				Filled: result.Filled, Failed: result.Failed, Total: result.TotalCells,
			})
			continue
		}
		if report == nil {
			// 当日无内容（report == nil）：不是失败，但也没补出报告——仍算缺口。
			dayStats.empty(cell.gap.Date)
			result.Empty++
			emitBackfillProgress(opts.Progress, GapBackfillProgress{
				RunID: runID, Date: cell.gap.Date, BoardID: cell.gap.BoardID,
				BoardLabel: cell.gap.BoardLabel, Status: "empty",
				Filled: result.Filled, Failed: result.Failed, Total: result.TotalCells,
			})
			continue
		}
		result.Filled++
		dayStats.filled(cell.gap.Date)
		emitBackfillProgress(opts.Progress, GapBackfillProgress{
			RunID: runID, Date: cell.gap.Date, BoardID: cell.gap.BoardID,
			BoardLabel: cell.gap.BoardLabel, Status: "completed",
			Filled: result.Filled, Failed: result.Failed, Total: result.TotalCells,
		})
	}

	// 未跑到的格子（预算耗尽 / 中断）也要如实进逐日明细：只靠循环里的计数
	// 会让没轮到的日子 remaining=0，与运行级 remaining_count 对不上（review R-3）。
	for _, cell := range cells[processed:] {
		dayStats.remaining(cell.gap.Date)
	}

	// 剩余缺口 = 扫到的格子里此刻仍没有报告的那些（失败 + 无内容 + 预算未跑到）。
	result.Remaining = result.TotalCells - result.Filled - result.Skipped
	result.Details = dayStats.details()
	result.Status = backfillStatus(result, budgetExhausted)
	result.FinishedAt = backfillNow()

	if err := repository.Repo.FinalizeBackfillRun(
		runID,
		result.Status,
		marshalBackfillDetails(result.Details),
		result.Filled,
		result.Failed,
		result.Remaining,
		result.FinishedAt,
	); err != nil {
		logging.Warnf("daily-report: finalize backfill run %s failed: %v", runID, err)
	}
	return result, nil
}

// backfillStatus adjudicates the four terminal states.
func backfillStatus(result *GapBackfillResult, budgetExhausted bool) string {
	if budgetExhausted {
		return repository.BackfillRunStatusBudgetExhausted
	}
	problems := result.Failed + result.Empty
	switch {
	case problems == 0:
		return repository.BackfillRunStatusSuccess
	case result.Filled == 0:
		return repository.BackfillRunStatusFailed
	default:
		return repository.BackfillRunStatusPartial
	}
}

// backfillCell is one gap cell bound to its report date.
type backfillCell struct {
	date time.Time
	gap  repository.DailyReportGap
}

// orderBackfillCells sorts the gap cells oldest-day-first (same day: board id
// ascending). 旧到新是 prev_report_id 链成立的前提（findPreviousReportBrief
// 取 period_date < date 的上一份报告）。
func orderBackfillCells(gaps []repository.DailyReportGap) []backfillCell {
	cells := make([]backfillCell, 0, len(gaps))
	for _, g := range gaps {
		day, err := time.ParseInLocation("2006-01-02", g.Date, time.Local)
		if err != nil {
			continue
		}
		cells = append(cells, backfillCell{date: day, gap: g})
	}
	sort.SliceStable(cells, func(i, j int) bool {
		if !cells[i].date.Equal(cells[j].date) {
			return cells[i].date.Before(cells[j].date)
		}
		return cells[i].gap.BoardID < cells[j].gap.BoardID
	})
	return cells
}

// backfillDayStats accumulates the per-day detail of a run.
type backfillDayStats struct {
	order  []string
	byDate map[string]*repository.BackfillDayDetail
}

func newBackfillDayStats(cells []backfillCell) *backfillDayStats {
	stats := &backfillDayStats{byDate: map[string]*repository.BackfillDayDetail{}}
	for _, c := range cells {
		if _, ok := stats.byDate[c.gap.Date]; ok {
			continue
		}
		stats.byDate[c.gap.Date] = &repository.BackfillDayDetail{Date: c.gap.Date}
		stats.order = append(stats.order, c.gap.Date)
	}
	return stats
}

func (s *backfillDayStats) filled(date string) {
	s.byDate[date].Filled++
}

// failed counts a generation error: the cell still has no report, so it also
// counts as a remaining gap of that day.
func (s *backfillDayStats) failed(date string) {
	s.byDate[date].Failed++
	s.byDate[date].Remaining++
}

// empty counts a generation that produced no report (当日无内容): not an
// error, but the cell is still a gap.
func (s *backfillDayStats) empty(date string) {
	s.byDate[date].Remaining++
}

// remaining counts a planned cell that was never attempted (budget exhausted /
// interrupted): it is still a gap of that day (review R-3).
func (s *backfillDayStats) remaining(date string) {
	s.byDate[date].Remaining++
}

func (s *backfillDayStats) details() []repository.BackfillDayDetail {
	out := make([]repository.BackfillDayDetail, 0, len(s.order))
	for _, date := range s.order {
		detail := s.byDate[date]
		out = append(out, *detail)
	}
	return out
}

// marshalBackfillDetails encodes the per-day detail array (jsonb 数组契约：
// 空写 [] 不写 null).
func marshalBackfillDetails(details []repository.BackfillDayDetail) repository.JSON {
	if details == nil {
		details = []repository.BackfillDayDetail{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return repository.JSON("[]")
	}
	return repository.JSON(encoded)
}

// emitBackfillProgress forwards one cell's progress to the caller's callback.
func emitBackfillProgress(fn func(GapBackfillProgress), p GapBackfillProgress) {
	if fn == nil {
		return
	}
	fn(p)
}
