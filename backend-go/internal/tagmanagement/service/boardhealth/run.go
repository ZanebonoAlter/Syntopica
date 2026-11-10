package boardhealth

import (
	"context"
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

// defaultMinInterval 是体检的新鲜度守卫默认窗口（design D5）：同板块最近
// 6 小时内已有快照则整板跳过（不调 LLM、不写快照），重复触发零成本。
const defaultMinInterval = 6 * time.Hour

// DefaultMinInterval 是 defaultMinInterval 的跨包别名（scheduler / handler
// 需要显式复用同一窗口值）。
const DefaultMinInterval = defaultMinInterval

// perBoardTimeout 是单板块子超时（design D4）：一个慢板只降级自己，不吃掉
// 后续板块的预算。包级 var（测试可覆盖成小值）。
var perBoardTimeout = 6 * time.Minute

// Options 参数化体检入口（fix-board-health-triage-degrade D3）。
type Options struct {
	// BoardIDs 非空时只处理这些板块（仍限定 label_type='board' AND
	// status='active'：非版块 / 已停用的 id 静默不产出快照）。
	BoardIDs []uint
	// MinInterval 是新鲜度守卫窗口：距上次快照不足该时长的板块整板跳过。
	// <= 0 时取默认 defaultMinInterval（6h）。
	MinInterval time.Duration
}

// boardOutcome 汇总单板体检的口径计数，供轮次日志使用（design D2）。
type boardOutcome struct {
	Labels   int    // 构成标签数
	Covered  int    // LLM 实际给出的合法分诊条目数
	Filled   int    // 补齐（证据不足）条目数
	Degraded bool   // 裁决降级（score/comment/triage 为空）
	Reason   string // 降级原因
}

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
//
// 零守卫全量：新鲜度守卫被显式关闭（CLI 与既有调用点每次调用都跑一遍，
// 行为与本 change 之前完全一致）。
func RunCheckup(ctx context.Context, db *gorm.DB, llm BoardHealthLLM) {
	runCheckup(ctx, db, llm, Options{}, false)
}

// RunCheckupWithOptions executes one checkup round with an explicit scope
// (Options.BoardIDs) and the freshness guard (Options.MinInterval, default
// 6h). Boards checked less than MinInterval ago are skipped whole — no LLM
// call, no snapshot row, and the skip count lands in the round log.
func RunCheckupWithOptions(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, opts Options) {
	runCheckup(ctx, db, llm, opts, true)
}

// runCheckup is the shared round implementation. freshnessGuard=false keeps
// the legacy zero-guard behaviour of RunCheckup.
func runCheckup(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, opts Options, freshnessGuard bool) {
	started := time.Now()
	config := LoadConfig(ctx, db)

	boards, err := loadCheckupBoards(ctx, db, opts)
	if err != nil {
		logging.Errorf("[board-health] load active boards failed, round aborted: %v", err)
		return
	}

	// 新鲜度守卫：一次批量 SQL 取各板最近快照时间，窗口内的板整板跳过。
	skippedFresh := 0
	if freshnessGuard {
		fresh, err := boardsCheckedRecently(ctx, db, boards, minIntervalOf(opts))
		if err != nil {
			logging.Warnf("[board-health] freshness guard query failed, running round without guard: %v", err)
		} else {
			skippedFresh = len(fresh)
			boards = filterOutBoards(boards, fresh)
		}
	}

	degradedBoards := 0
	filledTotal := 0
	scopeTotal := len(boards)
	for i := range boards {
		board := &boards[i]
		boardStarted := time.Now()
		// 单板子超时：慢板只降级自己，兄弟板预算不受影响（design D4）。
		boardCtx, cancel := context.WithTimeout(ctx, perBoardTimeout)
		outcome, err := checkupOneBoard(boardCtx, db, llm, config, board)
		cancel()
		elapsed := time.Since(boardStarted).Round(time.Millisecond)
		if err != nil {
			degradedBoards++
			logging.Warnf("[board-health] board=%d labels=%d covered=%d filled=%d degraded=true elapsed=%s reason=%v",
				board.ID, outcome.Labels, outcome.Covered, outcome.Filled, elapsed, err)
			continue
		}
		filledTotal += outcome.Filled
		if outcome.Degraded {
			degradedBoards++
			logging.Warnf("[board-health] board=%d labels=%d covered=%d filled=%d degraded=true elapsed=%s reason=%v",
				board.ID, outcome.Labels, outcome.Covered, outcome.Filled, elapsed, outcome.Reason)
			continue
		}
		logging.Infof("[board-health] board=%d labels=%d covered=%d filled=%d degraded=false elapsed=%s",
			board.ID, outcome.Labels, outcome.Covered, outcome.Filled, elapsed)
	}
	logging.Infof("[board-health] round finished boards=%d processed=%d degraded=%d filled=%d skipped_fresh=%d elapsed=%s",
		scopeTotal, len(boards), degradedBoards, filledTotal, skippedFresh, time.Since(started).Round(time.Millisecond))
}

// loadCheckupBoards loads the boards in scope: every active board, or only
// the requested ones (Options.BoardIDs). Non-board / disabled ids are silently
// dropped — they never produce a snapshot.
func loadCheckupBoards(ctx context.Context, db *gorm.DB, opts Options) ([]models.SemanticLabel, error) {
	query := db.WithContext(ctx).
		Where("label_type = ? AND status = ?", "board", "active")
	if len(opts.BoardIDs) > 0 {
		query = query.Where("id IN ?", opts.BoardIDs)
	}
	var boards []models.SemanticLabel
	if err := query.Order("id ASC").Find(&boards).Error; err != nil {
		return nil, err
	}
	return boards, nil
}

// minIntervalOf resolves the freshness window: an explicit value wins, the
// package default (6h) applies otherwise.
func minIntervalOf(opts Options) time.Duration {
	if opts.MinInterval <= 0 {
		return defaultMinInterval
	}
	return opts.MinInterval
}

// boardsCheckedRecently returns the boards whose newest snapshot is younger
// than minInterval. One batched query for the whole round (no per-board
// round-trips).
func boardsCheckedRecently(ctx context.Context, db *gorm.DB, boards []models.SemanticLabel, minInterval time.Duration) (map[uint]bool, error) {
	if len(boards) == 0 || minInterval <= 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(boards))
	for i := range boards {
		ids = append(ids, boards[i].ID)
	}
	var rows []struct {
		SemanticBoardID uint
		LastCheckedAt   time.Time
	}
	if err := db.WithContext(ctx).
		Table("board_health_snapshots").
		Select("semantic_board_id, MAX(checked_at) AS last_checked_at").
		Where("semantic_board_id IN ?", ids).
		// 只把「真正产出过裁决」的快照算新鲜（review M-1）：降级轮也会写
		// checked_at、但 llm_score 为 NULL——若把降级快照也算新鲜，该板会在
		// 6h 内被守卫挡住，用户想手动重跑补依据都跑不了（正是本 change 要
		// 修的场景）。降级快照不挡重生。
		Where("llm_score IS NOT NULL").
		Group("semantic_board_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	fresh := make(map[uint]bool, len(rows))
	for _, row := range rows {
		if now.Sub(row.LastCheckedAt) < minInterval {
			fresh[row.SemanticBoardID] = true
		}
	}
	return fresh, nil
}

// filterOutBoards drops the boards present in the skip set.
func filterOutBoards(boards []models.SemanticLabel, skip map[uint]bool) []models.SemanticLabel {
	if len(skip) == 0 {
		return boards
	}
	kept := make([]models.SemanticLabel, 0, len(boards))
	for i := range boards {
		if skip[boards[i].ID] {
			continue
		}
		kept = append(kept, boards[i])
	}
	return kept
}

// checkupOneBoard is the per-board seam (package var) so tests can inject
// per-board failures/panics and observe ordering without fixtures.
var checkupOneBoard = checkupBoardSafely

// checkupBoardSafely wraps one board's checkup with panic recovery.
func checkupBoardSafely(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel) (outcome boardOutcome, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return checkupBoard(ctx, db, llm, config, board, time.Now())
}

// CheckupBoard runs the full pipeline for one board and persists the
// snapshot. The LLM verdict failing degrades (score/comment NULL) but still
// persists statistics — a degraded round never fails the board.
func CheckupBoard(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel, now time.Time) error {
	_, err := checkupBoard(ctx, db, llm, config, board, now)
	return err
}

// checkupBoard is the per-board pipeline with the round-log counters.
//
// 自带 panic 回收（review L-2）：panic 发生在采集之后时，已填好的
// outcome.Labels 不会被清零——轮次日志才能报出真实构成标签数；
// checkupBoardSafely 的 recover 仍作为第二道兑底。
func checkupBoard(ctx context.Context, db *gorm.DB, llm BoardHealthLLM, config Config, board *models.SemanticLabel, now time.Time) (outcome boardOutcome, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	metrics, candidates, err := collectBoardStatistics(ctx, db, board, config)
	if err != nil {
		return outcome, fmt.Errorf("collect statistics: %w", err)
	}
	outcome.Labels = len(metrics.Labels)

	var score *float64
	var comment *string
	notifyVerdict := false
	if llm != nil {
		result, coverage, llmErr := adjudicateBoardHealth(ctx, llm, board, metrics, candidates, config)
		if llmErr != nil {
			outcome.Degraded = true
			outcome.Reason = llmErr.Error()
			// covered 即使降级也要回传（review M-2）：覆盖率不足时真实覆盖数是
			// 68/96，只打 covered=0 会与同一行 reason 自相矛盾。降级日志统一由
			// runCheckup 的每板一行输出（不再在这里额外打一行，review L-3）。
			outcome.Covered = coverage.Covered
		} else if result != nil {
			s := float64(result.Score)
			c := result.Comment
			score, comment = &s, &c
			notifyVerdict = result.Notify
			// 闸门在归一（normalizeTriage）之后、持久化之前执行（design
			// D4/D5/D6）：核心主体拦截 → 摘除证据校验 → 可执行占比闸门。
			// 闸门只改分诊项的类别与证据，MUST NOT 改写评分与评语。
			guarded, guard := applyTriageGuards(result.Triage, metrics.Labels, config)
			metrics.Triage = convertTriage(guarded)
			metrics.TriageGuard = guard
			if guard != nil && guard.Degraded {
				logging.Warnf("[board-health] board=%d triage_guard degraded actionable=%d/%d ratio=%.3f threshold=%.2f reason=%s",
					board.ID, guard.Actionable, guard.Total, guard.Ratio, guard.Threshold, guard.Reason)
			}
			outcome.Covered = coverage.Covered
			outcome.Filled = coverage.Filled
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
		return outcome, fmt.Errorf("persist snapshot: %w", err)
	}

	notifyIfAlert(ctx, db, config, board, score, notifyVerdict, time.Now())
	return outcome, nil
}

// collectBoardStatistics gathers the pure-read part: board-level metrics over
// all matched rows + label-level affinity distributions (composition-label
// embedding × every mounted tag identity embedding) + the three triage
// signals (board-health-triage-advice seam A) + the factual profile diff.
// Signal computation is same-round same-memory (design D1): magnet grouping
// reuses the loaded matchedRows/tagEmbeddings; neighbors add one pgvector
// query per vector-bearing label; the flow adds one aggregation per board
// with failed mounts. A signal query error fails this board's round (the
// round moves on to siblings) rather than persisting a half-signaled
// snapshot.
//
// board 是版块指针（不只是 id）：核心主体标记要读版块标题/描述/别名
// （fix-board-health-magnet-signal D3），标题正是核心主体判定的主依据。
func collectBoardStatistics(ctx context.Context, db *gorm.DB, board *models.SemanticLabel, config Config) (*tagmodels.BoardHealthMetrics, []tagmodels.BoardHealthCandidate, error) {
	boardID := board.ID
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

	for i := range labels {
		labels[i].DescFlags = screenDescription(labels[i].Description)
	}
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, config)
	// 核心主体标记：标签名出现在版块标题/描述/别名中 → 供闸门 G1 与提示词使用。
	computeCoreSubjects(board, labels)
	if err := loadNeighborsForLabels(ctx, db, labels, auxEmb); err != nil {
		return nil, nil, fmt.Errorf("load neighbors: %w", err)
	}
	flow, err := loadFailedFlow(ctx, db, boardID, failedTagIDs(rows))
	if err != nil {
		return nil, nil, fmt.Errorf("load failed flow: %w", err)
	}
	boardMetrics.MagnetFlow = flow

	candidates, err := UncapturedCandidates(ctx, db, boardID)
	if err != nil {
		return nil, nil, fmt.Errorf("uncaptured candidates: %w", err)
	}
	return &tagmodels.BoardHealthMetrics{Board: boardMetrics, Labels: labels}, candidates, nil
}

// convertTriage maps the validated LLM triage items onto the persisted
// model shape (design D3/D4). A-L2（review Low）：suggestion 仅 rewrite
// 语义持有——其它类别携带的多余文案在持久化转换时清空，不因此丢弃合法
// 分类/证据，也不改写输入的 LLM 结果（新 slice，输入 slice 原值保持）。
func convertTriage(items []TriageItem) []tagmodels.BoardHealthTriageItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]tagmodels.BoardHealthTriageItem, 0, len(items))
	for _, item := range items {
		converted := tagmodels.BoardHealthTriageItem{
			AuxID:    item.AuxID,
			Category: item.Category,
			Evidence: item.Evidence,
		}
		if item.Category == "rewrite" {
			converted.Suggestion = item.Suggestion
		}
		out = append(out, converted)
	}
	return out
}

// triageCoverage records how much of the composition the LLM actually
// adjudicated (design D2 log counters).
type triageCoverage struct {
	Labels  int // 构成标签总数
	Covered int // LLM 实际给出的合法分诊条目数
	Filled  int // 补齐（证据不足）条目数
}

// adjudicateBoardHealth renders the prompt and calls the LLM. Errors degrade
// upstream; a nil result with nil error is treated as degraded too. The
// score range check and the triage contract check live HERE (the persistence
// gate) so every LLM implementation — production or test fake — is bounded
// the same way: a structurally invalid triage list degrades the whole verdict
// (score/comment/triage all empty), while a merely incomplete list is padded
// with 「证据不足」 entries (normalizeTriage) — the verdict is never surfaced
// as a partially actionable advice list.
//
// config 传入是为了把磁铁阈值写进提示词（模型需要知道差值门槛才能引用证据）。
func adjudicateBoardHealth(ctx context.Context, llm BoardHealthLLM, board *models.SemanticLabel, metrics *tagmodels.BoardHealthMetrics, candidates []tagmodels.BoardHealthCandidate, config Config) (*BoardHealthLLMResult, triageCoverage, error) {
	candidateLines := make([]string, 0, len(candidates))
	for i, c := range candidates {
		if i >= scorePromptCandidates {
			break
		}
		candidateLines = append(candidateLines, fmt.Sprintf("%s（成员 tags=%d，文章覆盖=%d）", c.Label, c.MemberTags, c.MemberArticles))
	}
	result, err := llm.ScoreBoardHealth(ctx, buildScorePrompt(board, metrics, candidateLines, config.MagnetGap))
	if err != nil {
		return nil, triageCoverage{}, err
	}
	if result == nil || result.Score < 0 || result.Score > 100 {
		return nil, triageCoverage{}, fmt.Errorf("llm score out of range")
	}
	normalized, covered, err := normalizeTriage(result.Triage, metrics.Labels)
	if err != nil {
		// 覆盖率不足等致命错误也要回传真实 covered（review M-2）。
		return nil, triageCoverage{Labels: len(metrics.Labels), Covered: covered}, fmt.Errorf("llm triage invalid: %w", err)
	}
	coverage := triageCoverage{
		Labels:  len(metrics.Labels),
		Covered: covered,
		Filled:  len(normalized) - covered,
	}
	// Copy before writing back: the LLM result may be shared by the caller
	// (test fakes reuse one struct), the normalized list must not leak back
	// into it.
	normalizedResult := *result
	normalizedResult.Triage = normalized
	return &normalizedResult, coverage, nil
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
