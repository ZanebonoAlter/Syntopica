package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/logging"
)

// ── 素材补全门（fix-board-analysis-material 7：新鲜度门升级）──────────────
//
// EnrichBoard/EnrichTopic 装配前，对活跃泳道 month/year 档执行「补全」而非仅
// 保鲜：从 section 数据推出有料周期集，无行→补建（含首份）、行最后写于 72h
// 前→重算覆盖（已结束周期得到完整版，修复历史半月档；进行中得到至今快照）。
// 全局限额防大板块首跑爆量，溢出降级留日志；失败降级不阻塞；串行限流。
// week 退出检查集（近期记忆归 14 天窗口详情，长期归 month/year；存量 week
// 行保留可被态势卡取材链消费）。任何写入 as_of 钳制 ≤ now（7.2）。

// freshnessStaleThreshold: an existing row last written earlier than this is
// re-summarized (period ended → complete version; in-progress → up-to-now).
const freshnessStaleThreshold = 72 * time.Hour

// freshnessGranularities is the checked set — month/year only. week is out
// (near-horizon memory lives in the 14-day section window; week rows already
// in store remain consumable via the situation-card chain).
var freshnessGranularities = []string{"month", "year"}

// freshnessMaxCalls caps LLM summarize calls per analysis trigger (first run
// on a big board can otherwise burst; overflow degrades with old archives).
const freshnessMaxCalls = 40

// FreshnessRefresher abstracts the cycle-A refresh entry (for tests).
type FreshnessRefresher interface {
	RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error
	// RefreshPeriod regenerates one specific (granularity, period) archive.
	RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error
	// SectionDates returns the data-bearing dates for a topic (derives which
	// periods are worth having).
	SectionDates(ctx context.Context, topicID uint) ([]time.Time, error)
}

// FreshnessGateDetail records one lane×granularity×period decision (audit trail).
type FreshnessGateDetail struct {
	TopicID     uint   `json:"topic_id"`
	Granularity string `json:"granularity"`
	Period      string `json:"period,omitempty"`
	AsOfDate    string `json:"as_of_date,omitempty"`
	LagDays     int    `json:"lag_days"`
	Action      string `json:"action"` // skip_no_data|missing_period|stale_row→refreshed|skip_fresh|refresh_failed|budget_exhausted
	Error       string `json:"error,omitempty"`
}

// FreshnessGateReport aggregates the gate outcome; embedded in result metadata
// so 补全耗时透明化 (cost guardrail).
type FreshnessGateReport struct {
	Checked    int                   `json:"checked"`
	Refreshed  int                   `json:"refreshed"`
	Failed     int                   `json:"failed"`
	DurationMS int64                 `json:"duration_ms"`
	Details    []FreshnessGateDetail `json:"details"`
}

// FreshnessRefresherForTest reads the optional refresher (nil = gate disabled).
func (o *OrchestratorService) FreshnessRefresherForTest() FreshnessRefresher {
	return o.freshnessRefresher
}

// SetFreshnessRefresher wires the cycle-A refresher post-construction
// (constructor signature stays stable; production wiring in wire.go).
func (o *OrchestratorService) SetFreshnessRefresher(r FreshnessRefresher) { o.freshnessRefresher = r }

// ensureLaneFreshness runs the completeness gate serially over topicIDs ×
// {month, year}. Never returns an error: failures degrade (old archives +
// log). Budget-capped LLM calls; overflow degrades with a logged skip.
func (o *OrchestratorService) ensureLaneFreshness(ctx context.Context, topicIDs []uint) *FreshnessGateReport {
	report := &FreshnessGateReport{Details: []FreshnessGateDetail{}}
	start := time.Now()
	if o.freshnessRefresher == nil || len(topicIDs) == 0 {
		report.DurationMS = time.Since(start).Milliseconds()
		return report
	}
	now := time.Now()
	budget := freshnessMaxCalls
	for _, tid := range topicIDs {
		// Data-bearing periods per granularity for this lane.
		dates, err := o.freshnessRefresher.SectionDates(ctx, tid)
		if err != nil {
			logging.Warnf("freshness gate: topic %d: read section dates: %v", tid, err)
			continue
		}
		for _, gran := range freshnessGranularities {
			periods := dataPeriodsFor(dates, gran, now)
			if len(periods) == 0 {
				// No section data at all for this granularity → nothing
				// worth having; skip (a lane born this week has no month
				// history — stated, not silently dropped from the report).
				report.Checked++
				report.Details = append(report.Details, FreshnessGateDetail{
					TopicID: tid, Granularity: gran, Action: "skip_no_data",
				})
				continue
			}

			existing, err := o.repo.ListTopicLifelineContextsByGranularity(ctx, tid, gran)
			if err != nil {
				report.Checked++
				report.Failed++
				logging.Warnf("freshness gate: topic %d %s: list rows: %v", tid, gran, err)
				report.Details = append(report.Details, FreshnessGateDetail{
					TopicID: tid, Granularity: gran, Action: "refresh_failed", Error: err.Error(),
				})
				continue
			}
			have := make(map[string]bool, len(existing))
			lastWrite := map[string]time.Time{}
			for _, row := range existing {
				have[row.Period] = true
				if row.UpdatedAt.After(lastWrite[row.Period]) {
					lastWrite[row.Period] = row.UpdatedAt
				}
			}

			for _, p := range periods {
				report.Checked++
				detail := FreshnessGateDetail{TopicID: tid, Granularity: gran, Period: p}

				need, why := false, ""
				if !have[p] {
					need, why = true, "missing_period" // includes first-ever rows
				} else if now.Sub(lastWrite[p]) > freshnessStaleThreshold {
					need, why = true, "stale_row" // truncated half-month archive → recompute
				}
				if !need {
					detail.Action = "skip_fresh"
					report.Details = append(report.Details, detail)
					continue
				}

				if budget <= 0 {
					detail.Action, detail.Error = "budget_exhausted",
						fmt.Sprintf("%d-call cap reached, degrading with old archives", freshnessMaxCalls)
					report.Details = append(report.Details, detail)
					continue
				}
				budget--

				if err := o.freshnessRefresher.RefreshPeriod(ctx, tid, gran, p, now); err != nil {
					detail.Action, detail.Error = "refresh_failed", err.Error()
					report.Failed++
					logging.Warnf("freshness gate: topic %d %s %s (%s): refresh failed, degrading: %v", tid, gran, p, why, err)
					report.Details = append(report.Details, detail)
					continue
				}
				// Post-write clamp for any legacy future-dated as_of rows (7.2).
				o.clampAsOfDate(ctx, tid, gran, now)
				detail.Action = "refreshed"
				detail.AsOfDate = now.Format("2006-01-02")
				report.Refreshed++
				report.Details = append(report.Details, detail)
			}
		}
	}
	report.DurationMS = time.Since(start).Milliseconds()
	return report
}

// dataPeriodsFor maps data-bearing dates to the sorted unique period set for a
// granularity, capped at current period (no future periods).
func dataPeriodsFor(dates []time.Time, granularity string, now time.Time) []string {
	currentPeriod := PeriodForGranularity(now, granularity)
	seen := map[string]bool{}
	var out []string
	for _, d := range dates {
		p := PeriodForGranularity(d, granularity)
		if ComparePeriods(p, currentPeriod) < 0 || p == currentPeriod {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// clampAsOfDate caps the lane's as_of_date rows at now (best effort; failure
// only logged — a future-dated as_of merely suppresses the gate, never breaks
// the analysis).
func (o *OrchestratorService) clampAsOfDate(ctx context.Context, topicID uint, granularity string, now time.Time) {
	db := o.repo.DB().WithContext(ctx)
	res := db.Model(&repository.TopicLifelineContext{}).
		Where("persistent_topic_id = ? AND granularity = ? AND as_of_date > ?",
			topicID, granularity, now).
		Updates(map[string]any{"as_of_date": now, "updated_at": now})
	if res.Error != nil {
		logging.Warnf("freshness gate: topic %d %s: clamp as_of_date: %v", topicID, granularity, res.Error)
	}
}

// EnsureLaneFreshnessForTest exposes the gate to the external test package.
func (o *OrchestratorService) EnsureLaneFreshnessForTest(ctx context.Context, topicIDs []uint) *FreshnessGateReport {
	return o.ensureLaneFreshness(ctx, topicIDs)
}
