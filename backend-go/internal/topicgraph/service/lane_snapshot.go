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

// ── 泳道滚动态势结算（overview-lane-dynamics design D1/D3）──────────────────
//
// 挂点：GenerateAndSaveReport 成功后 detached goroutine（松耦合红线：结算
// 失败/panic 绝不影响日报主流程，下个日报日自愈）。
// 素材：该泳道近 14 个报告日「日期 + section 标题 + 前 3 条 thread 标题」，
// 窗口锚定 MAX(period_date) 而非 now()（日报未跑时窗口不漂移）。
// 产出：≤100 字中文态势句，upsert 进 topic_lane_snapshots（每泳道一行覆盖）。

const (
	// laneSnapshotSettleWindowDays: 态势句素材窗口与时间线窗口同为滚动
	// 14 天，以最近一份已完成日报为期（spec）。
	laneSnapshotSettleWindowDays = 14
	// laneSnapshotMaxRunes mechanically clamps the LLM output. The prompt
	// already demands ≤100字; this guard protects the card layout from
	// over-verbose models.
	laneSnapshotMaxRunes = 100
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
	maxTokens := 512
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
		"请用一句不超过100字的中文总结该话题这段时间的整体态势。" +
		"要求：只基于清单内列出的事实，不得编造事件、数字、情绪与因果；不做事态预测或走向判断；" +
		"直接输出这一句话本身，不要任何前后缀、引号或标号。"
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
// clamp, reused for the ≤100字 mechanical guard.

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
	summary := truncateRunes(strings.TrimSpace(content), laneSnapshotMaxRunes)
	if summary == "" {
		return fmt.Errorf("empty llm output")
	}
	return repository.Repo.UpsertLaneSnapshot(&repository.TopicLaneSnapshot{
		PersistentTopicID: lane.ID,
		RollingSummary:    summary,
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
