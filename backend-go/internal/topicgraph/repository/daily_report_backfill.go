package repository

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// 日报缺档补填（add-daily-report-gap-backfill design D1/D4）的数据访问层：
// ① 缺口查询（谓词复制生成 Step1 的严格口径）；② 补填运行留档的写入与读取。
//
// schema 由版本化迁移 20261125_0001 创建（模型不注册 AutoMigrate，SQL 是唯一
// 权威——jsonb 逐日明细 + 四态 status CHECK 由迁移固化）。

// 补填运行终态四态（spec「补填运行留档与终态告知」）。running 是运行中的
// 中间态，收尾时改写为下面四态之一。
const (
	BackfillRunStatusRunning         = "running"
	BackfillRunStatusSuccess         = "success"
	BackfillRunStatusPartial         = "partial"
	BackfillRunStatusFailed          = "failed"
	BackfillRunStatusBudgetExhausted = "budget_exhausted"
)

// 补填触发来源。
const (
	BackfillTriggerScopeManual    = "manual"
	BackfillTriggerScopeScheduled = "scheduled"
)

// DailyReportBackfillRun — 一次「日报缺档补填」运行的留档行。
//
// 逐日明细（DailyDetails）是 jsonb 数组契约：[{date,filled,failed,remaining}]，
// 空写 [] 不写 null（对齐 thread 引用数组契约）。
type DailyReportBackfillRun struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	RunID          string     `gorm:"size:64;not null" json:"run_id"`
	TriggerScope   string     `gorm:"size:20;not null;default:manual" json:"trigger_scope"`
	WindowStart    time.Time  `gorm:"type:date;not null" json:"window_start"`
	WindowEnd      time.Time  `gorm:"type:date;not null" json:"window_end"`
	Status         string     `gorm:"size:24;not null;default:running" json:"status"`
	DailyDetails   JSON       `gorm:"type:jsonb;not null" json:"daily_details"`
	FilledCount    int        `gorm:"not null;default:0" json:"filled_count"`
	FailedCount    int        `gorm:"not null;default:0" json:"failed_count"`
	RemainingCount int        `gorm:"not null;default:0" json:"remaining_count"`
	StartedAt      time.Time  `gorm:"not null" json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
}

func (DailyReportBackfillRun) TableName() string { return "daily_report_backfill_runs" }

// DailyReportGap — 一个「该板块当天有候选内容、但没有报告」的格子。
// Date 是 YYYY-MM-DD 报告日；BoardLabel 为空表示板块标签行缺失（前端回退
// 显示「Board #N」）。
type DailyReportGap struct {
	Date       string `json:"date"`
	BoardID    uint   `json:"board_id"`
	BoardLabel string `json:"board_label"`
}

// BackfillDayDetail — 逐日明细项（落 daily_report_backfill_runs.daily_details）。
type BackfillDayDetail struct {
	Date      string `json:"date"`
	Filled    int    `json:"filled"`
	Failed    int    `json:"failed"`
	Remaining int    `json:"remaining"`
}

// crossBoardExclusivePredicate 是跨板排他谓词（fix-board-match-cross-domain）：
// 同一 tag 在别的板块有强 ≥ 本板块分 + gap 的挂载时，本板块不收。
//
// **必须与生成 Step1（service.collectBoardTags）逐字一致**——缺口判定若用宽松
// 口径，会把生成时必然被剔的板块算成缺口，补填反复空跑烧 LLM（design D1）。
// 该片段不带表别名，故两侧都用裸表名 `topic_tag_board_labels` 引用。
const crossBoardExclusivePredicate = `NOT EXISTS (
	SELECT 1 FROM topic_tag_board_labels stronger
	WHERE stronger.topic_tag_id = topic_tag_board_labels.topic_tag_id
		AND stronger.semantic_board_id <> topic_tag_board_labels.semantic_board_id
		AND stronger.score >= topic_tag_board_labels.score + ?
)`

// ListDailyReportGaps returns every (board, date) cell inside the window that
// has candidate content but no report — the read side of the gap contract.
//
// 谓词与生成 Step1 一致（active + event + NOT direction_mismatch + 跨板排他
// NOT EXISTS），不使用宽松的 CollectBoardIDsForDate（后者不看 direction_mismatch
// 与排他分差，会把生成时必然被剔的板块算成缺口）。
//
// exclusiveGap <= 0 时排他子句整体关闭（与 collectBoardTags 的 Go 侧开关
// 同口径：gap 作为左操作数绑定参数在 pgx 上会静默误判，故不做 SQL 内联比较）。
//
// 窗口是闭区间 [windowStart, windowEnd]（调用方传「今天-保留天数」到
// 「昨天」）。已有报告的格子被剔除——只补空是补填的核心不变量。
func (r *TopicGraphRepository) ListDailyReportGaps(windowStart, windowEnd time.Time, exclusiveGap float64) ([]DailyReportGap, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list daily report gaps: repository not initialized")
	}
	// 依赖表缺席（最小部署 / 只迁了日报表的测试库）= 当天没有任何事件候选，
	// 返回空而不是整条路径报 SQL 错（同 CollectBoardIDsForDate 的退化契约）。
	for _, table := range []string{"topic_tags", "topic_tag_board_labels", "article_topic_tags", "articles"} {
		if !r.db.Migrator().HasTable(table) {
			return []DailyReportGap{}, nil
		}
	}
	if windowEnd.Before(windowStart) {
		return []DailyReportGap{}, nil
	}

	existing, err := r.existingReportBoardDates(windowStart, windowEnd)
	if err != nil {
		return nil, err
	}

	gaps := make([]DailyReportGap, 0)
	boardIDs := make(map[uint]struct{})
	for day := windowStart; !day.After(windowEnd); day = day.AddDate(0, 0, 1) {
		startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		ids, err := r.candidateBoardIDsForDate(startOfDay, endOfDay, exclusiveGap)
		if err != nil {
			return nil, err
		}
		dateKey := NormalizeReportDate(day).Format("2006-01-02")
		for _, boardID := range ids {
			if _, ok := existing[boardDateKey{boardID: boardID, date: dateKey}]; ok {
				continue // 已有报告 → 不是缺口（只补空）
			}
			gaps = append(gaps, DailyReportGap{Date: dateKey, BoardID: boardID})
			boardIDs[boardID] = struct{}{}
		}
	}

	labels := r.boardLabels(boardIDs)
	for i := range gaps {
		gaps[i].BoardLabel = labels[gaps[i].BoardID]
	}
	return gaps, nil
}

// candidateBoardIDsForDate runs the strict Step1 candidate predicate for one
// day and returns the boards that keep at least one candidate tag.
func (r *TopicGraphRepository) candidateBoardIDsForDate(startOfDay, endOfDay time.Time, exclusiveGap float64) ([]uint, error) {
	type row struct {
		BoardID uint `gorm:"column:semantic_board_id"`
	}
	// 排他子句按 Go 侧开关拼进 SQL（gap <= 0 关闭）——同 collectBoardTags 的
	// 口径：gap 绑成比较左操作数在 pgx 上会静默误判。
	sql := `
		SELECT DISTINCT topic_tag_board_labels.semantic_board_id AS semantic_board_id
		FROM topic_tag_board_labels
		JOIN topic_tags ON topic_tags.id = topic_tag_board_labels.topic_tag_id
		JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id
		JOIN articles ON articles.id = article_topic_tags.article_id
		WHERE topic_tags.status = ?
			AND topic_tags.category = ?
			AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)
			AND articles.pub_date >= ? AND articles.pub_date < ?`
	args := []interface{}{"active", models.TagCategoryEvent, startOfDay, endOfDay}
	if exclusiveGap > 0 {
		sql += " AND " + crossBoardExclusivePredicate
		args = append(args, exclusiveGap)
	}
	var rows []row
	if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query candidate boards for %s: %w", startOfDay.Format("2006-01-02"), err)
	}
	ids := make([]uint, 0, len(rows))
	for _, rw := range rows {
		ids = append(ids, rw.BoardID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// boardDateKey is the (board, report date) identity of a daily report.
type boardDateKey struct {
	boardID uint
	date    string
}

// existingReportBoardDates loads the (board, date) pairs that already have a
// report inside the window. The pull is bounded by ±48h and the comparison is
// done in memory through NormalizeReportDate — the same dialect-proof shape as
// ReportExistsForBoardDate (Postgres DATE vs SQLite TEXT timestamps).
func (r *TopicGraphRepository) existingReportBoardDates(windowStart, windowEnd time.Time) (map[boardDateKey]struct{}, error) {
	out := make(map[boardDateKey]struct{})
	if !r.db.Migrator().HasTable("board_daily_reports") {
		return out, nil
	}
	type row struct {
		BoardID    uint      `gorm:"column:semantic_board_id"`
		PeriodDate time.Time `gorm:"column:period_date"`
	}
	low := NormalizeReportDate(windowStart).Add(-48 * time.Hour)
	high := NormalizeReportDate(windowEnd).Add(48 * time.Hour)
	var rows []row
	if err := r.db.Model(&BoardDailyReport{}).
		Select("semantic_board_id, period_date").
		Where("period_date > ? AND period_date < ?", low, high).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load existing report dates: %w", err)
	}
	for _, rw := range rows {
		out[boardDateKey{boardID: rw.BoardID, date: NormalizeReportDate(rw.PeriodDate).Format("2006-01-02")}] = struct{}{}
	}
	return out, nil
}

// boardLabels resolves board ids to their display labels in one query.
// 表缺席或查不到时返回空 map（调用方回退成「Board #N」）。
func (r *TopicGraphRepository) boardLabels(boardIDs map[uint]struct{}) map[uint]string {
	out := make(map[uint]string, len(boardIDs))
	if len(boardIDs) == 0 || !r.db.Migrator().HasTable("semantic_labels") {
		return out
	}
	ids := make([]uint, 0, len(boardIDs))
	for id := range boardIDs {
		ids = append(ids, id)
	}
	type row struct {
		ID    uint   `gorm:"column:id"`
		Label string `gorm:"column:label"`
	}
	var rows []row
	if err := r.db.Model(&models.SemanticLabel{}).
		Select("id, label").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return out
	}
	for _, rw := range rows {
		out[rw.ID] = rw.Label
	}
	return out
}

// CreateBackfillRun inserts a fresh run row in the `running` state. The row is
// the audit trail of one补填 attempt — it is written BEFORE the first board is
// generated so an interrupted run is still visible (spec「每次补填运行 SHALL 持久化
// 一条运行记录」).
//
// 表缺席（迁移未跑 / 最小部署）时静默跳过：留档是增强，不是补填的前置条件。
func (r *TopicGraphRepository) CreateBackfillRun(run *DailyReportBackfillRun) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("create backfill run: repository not initialized")
	}
	if run == nil {
		return fmt.Errorf("create backfill run: nil run")
	}
	if !r.db.Migrator().HasTable(DailyReportBackfillRun{}.TableName()) {
		return nil
	}
	if run.Status == "" {
		run.Status = BackfillRunStatusRunning
	}
	if run.DailyDetails == nil {
		run.DailyDetails = JSON("[]")
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now()
	}
	if err := r.db.Create(run).Error; err != nil {
		return fmt.Errorf("create backfill run: %w", err)
	}
	return nil
}

// FinalizeBackfillRun writes the terminal state of a run.
//
// 幂等：按 run_id 原地 UPDATE，绝不新建行——重复收尾（重试 / 补偿任务）不会
// 把一次运行拆成两行留档。
func (r *TopicGraphRepository) FinalizeBackfillRun(runID string, status string, details JSON, filled, failed, remaining int, finishedAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("finalize backfill run: repository not initialized")
	}
	if runID == "" {
		return fmt.Errorf("finalize backfill run: empty run id")
	}
	if !r.db.Migrator().HasTable(DailyReportBackfillRun{}.TableName()) {
		return nil
	}
	if details == nil {
		details = JSON("[]")
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	if err := r.db.Model(&DailyReportBackfillRun{}).
		Where("run_id = ?", runID).
		Updates(map[string]interface{}{
			"status":          status,
			"daily_details":   details,
			"filled_count":    filled,
			"failed_count":    failed,
			"remaining_count": remaining,
			"finished_at":     finishedAt,
		}).Error; err != nil {
		return fmt.Errorf("finalize backfill run %s: %w", runID, err)
	}
	return nil
}

// LatestBackfillRun returns the most recent run ledger row (newest first),
// or (nil, nil) when there is none / the table is absent. Read-only.
func (r *TopicGraphRepository) LatestBackfillRun() (*DailyReportBackfillRun, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("latest backfill run: repository not initialized")
	}
	if !r.db.Migrator().HasTable(DailyReportBackfillRun{}.TableName()) {
		return nil, nil
	}
	var run DailyReportBackfillRun
	if err := r.db.Order("started_at DESC, id DESC").First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load latest backfill run: %w", err)
	}
	return &run, nil
}
