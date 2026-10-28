package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 泳道滚动态势结算（overview-lane-dynamics → slim-lane-snapshot-trend）────
//
// 挂点：GenerateAndSaveReport 成功后 detached goroutine（松耦合红线：结算
// 失败/panic 绝不影响日报主流程，下个日报日自愈）。
// 素材：该泳道近 14 个报告日「日期 + section 标题 + 前 3 条 thread 标题」
// + 量级信号段（系统计算的每日线索量与近7/上7合计，不信 LLM 数数），
// 窗口锚定 MAX(period_date) 而非 now()（日报未跑时窗口不漂移）。
// 产出：单版演进叙事态势句（slim-lane-snapshot-trend）——150-200 字中文纯文本，
// 按「阶段→驱动/转折→当前落点→量级佐证」组织，事件明细职责归逐日事件。
// upsert 进 topic_lane_snapshots（每泳道一行覆盖）；rolling_detail 固定写空，
// 存量历史长版随下个日报日结算自然覆盖。

const (
	// laneSnapshotSettleWindowDays: 态势句素材窗口与时间线窗口同为滚动
	// 14 天，以最近一份已完成日报为期（spec）。
	laneSnapshotSettleWindowDays = 14
	// laneSnapshotMaxRunes mechanically clamps the LLM short-form output. The
	// prompt already demands ≤200字 (relax-lane-snapshot-length-caps: 实测
	// 100 字高频硬切烂尾); this guard protects the card layout from
	// over-verbose models. 2026-10-28 真库效果核对：38/38 条真实快照顶到上限、
	// 37 条被从字中间切断（页面显示半截话），故 clamp 优先在句末标点收口
	// （见 clampLaneSnapshotRunes），纯 rune 硬切只作无标点兜底。
	laneSnapshotMaxRunes = 200
	// laneSnapshotMaxTokens bounds the single-call output budget
	// (slim-lane-snapshot-trend D3): 单版 200 字中文 ≈ 400 token 上浮呼吸
	// 空间；旧 1536 是双版 JSON 时代的预算，不再需要。Package-level so
	// tests can pin it; the chat fn must keep referencing it.
	laneSnapshotMaxTokens = 500
	// laneSnapshotLaneTimeout bounds ONE lane's settlement (design D1).
	laneSnapshotLaneTimeout = 60 * time.Second
)

// laneSnapshotChatFn is the swappable LLM hook (test injection; mirrors
// clusterChatFn / watchChatFunc). Default: single airouter Chat —
// operation=daily_report.lane_snapshot writes the AICallLog row via the
// router (ai-logging 规范), CapabilityDigestPolish consistent with every
// other daily-report LLM call.
var laneSnapshotChatFn = func(ctx context.Context, system, user string) (string, error) {
	temperature := 0.3
	maxTokens := laneSnapshotMaxTokens
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.lane_snapshot",
		SessionID:  fmt.Sprintf("lane_snapshot_%s", uuid.NewString()[:8]),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func laneSnapshotSystemPrompt() string {
	return "你是新闻态势总结助手。基于一条话题泳道近14天的日报事实清单与量级信号，写一段150-200字的中文态势总结，" +
		"按演进叙事组织：先划分阶段（各阶段带大致日期锚），说明每阶段的驱动与转折（事件仅作转折标记点出，不展开细节），" +
		"再给出当前落点与局势判断，最后用量级信号佐证走向。" +
		"要求：禁止逐日罗列事件；禁止预测未来走向（只描述已发生的变化）；" +
		"禁止使用清单与量级信号之外的数字；「无报告」日不计入趋势判断。" +
		"篇幅硬约束：总长（含标点）不得超过200字，并且必须以句号收尾；" +
		"写不下时压缩阶段描述或合并阶段，宁可少写一个细节，也不得写到一半停下。" +
		"直接输出这一段纯文本，不要JSON、标题、前后缀或代码块围栏。"
}

// buildLaneSnapshotUserPrompt renders the material rows: one line per section,
// "YYYY-MM-DD｜section 标题｜thread1 / thread2 / thread3"（无线索标题时省略
// 第三段），followed by the system-computed magnitude signal block
// (slim-lane-snapshot-trend D1).
func buildLaneSnapshotUserPrompt(label string, rows []repository.LaneMaterialRow, counts []repository.LaneDailyCount, anchor time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "泳道：%s\n\n--- 近14天事实清单 ---\n", label)
	for _, row := range rows {
		fmt.Fprintf(&sb, "%s｜%s", row.PeriodDate.Format("2006-01-02"), row.SectionLabel)
		// Defensive cap at the render layer too (design D3: 前 3 条)：the
		// repository query already truncates, but the builder guarantees the
		// ≤3 invariant on its own so no caller can inflate the prompt.
		titles := row.ThreadTitles
		if len(titles) > repository.LaneSnapshotThreadTitlesPerSection {
			titles = titles[:repository.LaneSnapshotThreadTitlesPerSection]
		}
		if len(titles) > 0 {
			fmt.Fprintf(&sb, "｜%s", strings.Join(titles, " / "))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(renderLaneMagnitudeSignal(counts, anchor))
	return sb.String()
}

// renderLaneMagnitudeSignal renders the magnitude block: per-day thread counts
// across the material window (days without a report shown explicitly as
// 无报告 so a gap is never mistaken for a dip) plus a 7-day vs
// previous-7-day rollup anchored at as_of. All numbers are system-computed
// from the repository counts — the LLM never counts for itself (D1).
// Window semantics: the daily rows walk [anchor-14, anchor] — the exact
// same inclusive window the material rows and ListDailyThreadCounts load
// under (15 day slots, R-1); the rollups are [anchor-6, anchor] (near) and
// [anchor-13, anchor-7] (previous).
func renderLaneMagnitudeSignal(counts []repository.LaneDailyCount, anchor time.Time) string {
	byDay := make(map[string]repository.LaneDailyCount, len(counts))
	for _, c := range counts {
		byDay[c.PeriodDate.Format("2006-01-02")] = c
	}

	var sb strings.Builder
	sb.WriteString("\n--- 量级信号（系统统计；「无报告」=当日未出日报，不代表热度为零，不计入趋势判断）---\n每日线索量：")
	windowStart := anchor.AddDate(0, 0, -14)
	first := true
	for d := windowStart; !d.After(anchor); d = d.AddDate(0, 0, 1) {
		if !first {
			sb.WriteString("｜")
		}
		first = false
		if c, ok := byDay[d.Format("2006-01-02")]; ok {
			fmt.Fprintf(&sb, "%s %d条", d.Format("01-02"), c.ThreadCnt)
			if c.SectionCnt > 1 {
				fmt.Fprintf(&sb, "·%d节", c.SectionCnt)
			}
		} else {
			fmt.Fprintf(&sb, "%s 无报告", d.Format("01-02"))
		}
	}

	// Rollups anchored at as_of: near = [anchor-6, anchor], previous =
	// [anchor-13, anchor-7]. Days-with-report counted per window so an empty
	// previous window renders as 无报告数据 instead of a misleading 0-total.
	nearFrom := anchor.AddDate(0, 0, -6)
	prevFrom := anchor.AddDate(0, 0, -13)
	nearTotal, prevTotal, prevDays := 0, 0, 0
	for _, c := range counts {
		switch {
		case !c.PeriodDate.Before(nearFrom):
			nearTotal += c.ThreadCnt
		case !c.PeriodDate.Before(prevFrom):
			prevTotal += c.ThreadCnt
			prevDays++
		}
	}
	sb.WriteString("\n")
	if prevDays == 0 {
		fmt.Fprintf(&sb, "近7天（%s～%s）合计 %d 条｜上个7天（%s～%s）无报告数据",
			nearFrom.Format("01-02"), anchor.Format("01-02"), nearTotal,
			prevFrom.Format("01-02"), nearFrom.AddDate(0, 0, -1).Format("01-02"))
	} else {
		fmt.Fprintf(&sb, "近7天（%s～%s）合计 %d 条｜上个7天（%s～%s）合计 %d 条",
			nearFrom.Format("01-02"), anchor.Format("01-02"), nearTotal,
			prevFrom.Format("01-02"), nearFrom.AddDate(0, 0, -1).Format("01-02"), prevTotal)
	}
	return sb.String()
}

// truncateRunes is package-shared (watch_materialize_keyword.go) — rune-safe
// clamp used by the watch-materialize keyword guards.

// parseLaneSnapshotOutput applies the plain-text three-step guard
// (slim-lane-snapshot-trend D3): trim → empty/whitespace fails the settlement
// (snapshot keeps its old value, retried next report day) → clamp to
// laneSnapshotMaxRunes at a sentence boundary. There is deliberately NO JSON
// parsing and no degrade branch: the prompt demands a single plain paragraph,
// and stray fences/quotes (if a model emits them anyway) land in the text as-is.
func parseLaneSnapshotOutput(content string) (string, error) {
	summary := clampLaneSnapshotRunes(strings.TrimSpace(content), laneSnapshotMaxRunes)
	if summary == "" {
		return "", fmt.Errorf("empty llm output")
	}
	return summary, nil
}

// clampLaneSnapshotRunes clamps text to max runes without cutting a sentence
// in half: when the over-cap window contains a sentence terminator (。！？… or
// a newline) late enough to keep ≥60% of the budget, the text is cut right
// after the last such terminator; otherwise it falls back to a plain rune cut.
//
// 2026-10-28 真库效果核对（slim-lane-snapshot-trend 5.3）: every settled lane
// hit the cap and 37/38 sentences were cut mid-word (「…停用并移除DeepSeek」
// 「…SK」). The prompt now also demands a complete final sentence; this is the
// mechanical backstop that keeps a dangling fragment off the reading view.
func clampLaneSnapshotRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	window := runes[:max]
	floor := max * 3 / 5
	for i := len(window) - 1; i >= floor; i-- {
		switch window[i] {
		case '。', '！', '？', '…', '\n':
			return strings.TrimSpace(string(window[:i+1]))
		}
	}
	return string(window)
}

// settleBoardLaneSnapshots settles the rolling snapshot for the board's
// active lanes (design D1): serial per lane, clamp 20 by last_seen_date DESC
// (overflow logged), per-lane 60s timeout. Every failure is logged and
// skipped — this function MUST NOT propagate errors to the report pipeline.
// Lanes with no in-window material are skipped (沉寂泳道无素材可结，快照
// 保持缺失/旧值).
func settleBoardLaneSnapshots(ctx context.Context, boardID uint) {
	anchor, ok, err := repository.Repo.GetBoardReportAnchor(boardID)
	if err != nil {
		logging.Warnf("lane-snapshot: board %d report anchor lookup failed: %v", boardID, err)
		return
	}
	if !ok {
		return // board has no reports — nothing to settle
	}
	from := anchor.AddDate(0, 0, -laneSnapshotSettleWindowDays)

	// +1 probe row to detect clamp overflow without loading every lane.
	lanes, err := repository.Repo.ListActiveLanesForSnapshot(boardID, repository.LaneSnapshotMaxPerBoard+1)
	if err != nil {
		logging.Warnf("lane-snapshot: board %d list active lanes failed: %v", boardID, err)
		return
	}
	if len(lanes) > repository.LaneSnapshotMaxPerBoard {
		logging.Warnf("lane-snapshot: board %d has more than %d active lanes; keeping the %d most recently seen (design D1 clamp)",
			boardID, repository.LaneSnapshotMaxPerBoard, repository.LaneSnapshotMaxPerBoard)
		lanes = lanes[:repository.LaneSnapshotMaxPerBoard]
	}

	settled := 0
	for _, lane := range lanes {
		select {
		case <-ctx.Done():
			logging.Warnf("lane-snapshot: board %d settlement canceled: %v", boardID, ctx.Err())
			return
		default:
		}
		laneCtx, cancel := context.WithTimeout(ctx, laneSnapshotLaneTimeout)
		settleErr := settleLaneSnapshot(laneCtx, lane, from, anchor)
		cancel()
		if settleErr != nil {
			logging.Warnf("lane-snapshot: settle lane %d (%s) failed: %v — snapshot kept as-is, retried next report day",
				lane.ID, lane.Label, settleErr)
			continue
		}
		settled++
	}
	logging.Infof("lane-snapshot: board %d settlement done: %d/%d lanes processed (as_of=%s)",
		boardID, settled, len(lanes), anchor.Format("2006-01-02"))
}

// settleLaneSnapshot settles one lane: material → single LLM call → upsert
// (as_of = anchor). Returns an error only to be logged by the caller.
func settleLaneSnapshot(ctx context.Context, lane repository.BoardPersistentTopic, from, anchor time.Time) error {
	rows, err := repository.Repo.ListLaneSnapshotMaterial(
		lane.ID, from, anchor, repository.LaneSnapshotThreadTitlesPerSection)
	if err != nil {
		return fmt.Errorf("load material: %w", err)
	}
	if len(rows) == 0 {
		return nil // silent lane — nothing to settle, no LLM call
	}
	counts, err := repository.Repo.ListDailyThreadCounts(lane.ID, from, anchor)
	if err != nil {
		return fmt.Errorf("load daily counts: %w", err)
	}
	content, err := laneSnapshotChatFn(ctx, laneSnapshotSystemPrompt(), buildLaneSnapshotUserPrompt(lane.Label, rows, counts, anchor))
	if err != nil {
		return fmt.Errorf("llm call: %w", err)
	}
	summary, err := parseLaneSnapshotOutput(content)
	if err != nil {
		return err
	}
	// slim-lane-snapshot-trend: rolling_detail 停止产出，固定写空串；存量
	// 历史长版随本次覆盖自然清空（列保留，无 migration）。
	return repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: lane.ID,
		RollingSummary:    summary,
		RollingDetail:     "",
		AsOfDate:          anchor,
	})
}

// runLaneSnapshotSettlement is the injectable settlement runner (tests swap
// it to force panics — the red-line guard below must survive them).
var runLaneSnapshotSettlement = settleBoardLaneSnapshots

// settleLaneSnapshotsSafe runs the settlement with a full recover: any panic
// is swallowed and logged (松耦合纪律, design D1 — 结算 panic 绝不外溢).
func settleLaneSnapshotsSafe(boardID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("lane-snapshot: settlement panicked for board %d: %v", boardID, r)
		}
	}()
	runLaneSnapshotSettlement(context.Background(), boardID)
}

// spawnLaneSnapshotSettlement launches the settlement as a detached goroutine
// after a board's report was saved (design D1). Injectable so tests can force
// synchronous/panicking spawns without timing races.
var spawnLaneSnapshotSettlement = func(boardID uint) {
	go settleLaneSnapshotsSafe(boardID)
}
