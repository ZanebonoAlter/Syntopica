package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 泳道滚动态势结算（overview-lane-dynamics design D1/D3）──────────────────
//
// 挂点：GenerateAndSaveReport 成功后 detached goroutine（松耦合红线：结算
// 失败/panic 绝不影响日报主流程，下个日报日自愈）。
// 素材：该泳道近 14 个报告日「日期 + section 标题 + 前 3 条 thread 标题」，
// 窗口锚定 MAX(period_date) 而非 now()（日报未跑时窗口不漂移）。
// 产出：长短两版（lane-trend-overview design D1）——≤100 字中文态势句 +
// ≤500 字中文成段叙述（同一次调用、同一份素材），upsert 进
// topic_lane_snapshots（每泳道一行覆盖）。解析失败降级为仅短版，不算失败，
// 下个日报日自愈。

const (
	// laneSnapshotSettleWindowDays: 态势句素材窗口与时间线窗口同为滚动
	// 14 天，以最近一份已完成日报为期（spec）。
	laneSnapshotSettleWindowDays = 14
	// laneSnapshotMaxRunes mechanically clamps the LLM short-form output. The
	// prompt already demands ≤200字 (relax-lane-snapshot-length-caps: 实测
	// 100 字高频硬切烂尾); this guard protects the card layout from
	// over-verbose models.
	laneSnapshotMaxRunes = 200
	// laneSnapshotDetailMaxRunes mechanically clamps the long-form narrative
	// (design D1: 成段叙述, rune-safe like the short form;
	// relax-lane-snapshot-length-caps: 500→1000, 500 实测常态超限).
	laneSnapshotDetailMaxRunes = 1000
	// laneSnapshotMaxTokens bounds the single two-version call (design D1:
	// 两版合计 + JSON 结构开销). Package-level so tests can pin it (SN-9); the
	// chat fn must keep referencing it. relax-lane-snapshot-length-caps:
	// 768→1536 — 768 预算下啰嗦模型会把 JSON 掐断在半路，解析失败误走降级
	// （长版丢失+短版砍半），预算须容纳两版新上限。
	laneSnapshotMaxTokens = 1536
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
	return "你是新闻态势总结助手。给定一条话题泳道近14天的日报事实清单（每行：日期｜节标题｜线索标题），" +
		"请输出一个 JSON 对象，含两个字段：\"summary\" 为一句不超过200字的中文态势句；" +
		"\"detail\" 为一段不超过1000字的中文成段叙述。" +
		"两版必须基于同一份清单事实：detail 是同一事实集的成段展开，不得另起炉灶。" +
		"要求：只基于清单内列出的事实，不得编造事件、数字、情绪与因果；不做事态预测或走向判断；" +
		"直接输出 JSON 本身，不要任何前后缀、代码块围栏、引号或标号。"
}

// buildLaneSnapshotUserPrompt renders the material rows: one line per section,
// "YYYY-MM-DD｜section 标题｜thread1 / thread2 / thread3"（无线索标题时省略
// 第三段）。
func buildLaneSnapshotUserPrompt(label string, rows []repository.LaneMaterialRow) string {
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
	return sb.String()
}

// truncateRunes is package-shared (watch_materialize_keyword.go) — rune-safe
// clamp, reused for the ≤100字/≤500字 mechanical guards.

// parseLaneSnapshotOutput applies design D1's two-version protocol to the raw
// LLM output. Fence stripping reuses jsonutil.SanitizeLLMJSON — the same
// tolerance every other structured daily-report output relies on (SN-7).
// Returns (summary, detail, degraded):
//   - JSON valid: fields trimmed; summary empty → error (既有空输出守卫, SN-6);
//     detail empty/missing is legal (SN-5). Both clamped to their own caps,
//     rune-safe and independent (SN-2/SN-3).
//   - JSON broken: degrade (SN-4) — whole output clamped to 100 as summary,
//     detail empty, degraded=true (caller warns; NOT a settlement failure —
//     upsert proceeds, next report day heals). Degrade yielding an empty
//     summary (pure whitespace output) still fails the guard.
func parseLaneSnapshotOutput(content string) (summary, detail string, degraded bool, err error) {
	cleaned := jsonutil.SanitizeLLMJSON(content)
	var raw struct {
		Summary string `json:"summary"`
		Detail  string `json:"detail"`
	}
	if jerr := json.Unmarshal([]byte(cleaned), &raw); jerr != nil {
		summary = truncateRunes(strings.TrimSpace(content), laneSnapshotMaxRunes)
		if summary == "" {
			return "", "", true, fmt.Errorf("empty llm output")
		}
		return summary, "", true, nil
	}
	summary = strings.TrimSpace(raw.Summary)
	if summary == "" {
		return "", "", false, fmt.Errorf("empty llm output")
	}
	detail = strings.TrimSpace(raw.Detail)
	return truncateRunes(summary, laneSnapshotMaxRunes), truncateRunes(detail, laneSnapshotDetailMaxRunes), false, nil
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
	content, err := laneSnapshotChatFn(ctx, laneSnapshotSystemPrompt(), buildLaneSnapshotUserPrompt(lane.Label, rows))
	if err != nil {
		return fmt.Errorf("llm call: %w", err)
	}
	summary, detail, degraded, err := parseLaneSnapshotOutput(content)
	if err != nil {
		return err
	}
	if degraded {
		logging.Warnf("lane-snapshot: lane %d (%s) llm output not valid JSON — degraded to short form only, long form retried next report day",
			lane.ID, lane.Label)
	}
	return repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: lane.ID,
		RollingSummary:    summary,
		RollingDetail:     detail,
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
