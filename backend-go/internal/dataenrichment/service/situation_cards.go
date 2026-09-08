package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ── 态势卡装配器（design D2/D6，tasks 3.1）───────────────────────────────────
//
// 每活跃泳道一张卡：lifeline week 事实摘要（缺失降级近期 section 事实指纹）+
// 命中统计 + 质量信号（活跃度/密度/sparse 历史，查询期现算不落库）。按质量排序
// 控制详略（full/brief）。纯机械拼装，零 LLM 成本——agent 需要论据细节时经
// get_lane_detail 下钻（已有工具，无需新建）。

const (
	// situationCardMaxLanes caps cards per board to protect the interpret
	// prompt budget; lanes beyond the cap (lowest quality) are dropped.
	situationCardMaxLanes = 12
	// situationCardDigestRunes bounds the lifeline-based facts digest.
	situationCardDigestRunes = 120
	// situationCardBriefRunes bounds the brief-mode digest.
	situationCardBriefRunes = 48
	// situationCardLifelineWeeks is how many recent week-granularity lifeline
	// periods feed the digest.
	situationCardLifelineWeeks = 2
	// situationCardLifelineMonths is how many recent month-granularity
	// periods feed the digest when week rows are missing (production shape:
	// month is maintained fleet-wide by the monthly job; week lags).
	situationCardLifelineMonths = 2
	// situationCardFingerprintSections is how many recent daily-report
	// sections feed the fallback fingerprint (and the density signal).
	situationCardFingerprintSections = 3
	// situationCardFingerprintThreads is how many thread titles represent one
	// section in the fallback fingerprint (substantive facts, not the cluster
	// label which usually just repeats the lane name).
	situationCardFingerprintThreads = 3
	// situationCardFullQuality is the quality threshold for full-detail cards.
	situationCardFullQuality = 10.0
	// situationCardSparseDegraded: lanes with >= this many past sparse-form
	// results are forced to brief detail (proven not to sustain depth).
	situationCardSparseDegraded = 2
)

// LaneSituationCard is one active lane's situation card (D2). Purely
// mechanical; no LLM in assembly.
type LaneSituationCard struct {
	LaneID          uint   `json:"lane_id"`
	Label           string `json:"label"`
	HitCount        int    `json:"hit_count"`
	ConsecutiveHits int    `json:"consecutive_hits"`
	LastSeenDate    string `json:"last_seen_date"`
	DaysSinceSeen   int    `json:"days_since_seen"`
	// FactsDigest: lifeline-week summary when available, else lifeline-month
	// (production backstop), else recent-section fingerprint, else the lane
	// description, else empty.
	FactsDigest string `json:"facts_digest"`
	// FactsSource: "lifeline_week" | "lifeline_month" | "section_fingerprint" |
	// "description" | "none".
	FactsSource string `json:"facts_source"`
	// QualityScore = activity + density − sparse penalty (formula in
	// computeSituationQuality). Used for ordering and full/brief detail level.
	QualityScore float64          `json:"quality_score"`
	Signals      SituationSignals `json:"signals"`
	DetailLevel  string           `json:"detail_level"` // "full" | "brief"
	Extra        map[string]any   `json:"extra,omitempty"`
}

// SituationSignals are the query-time quality signals (D6): no table, computed
// per assembly.
type SituationSignals struct {
	ActivityScore float64 `json:"activity_score"` // 2·min(consecutive_hits,14)+max(0,14−days_since_seen)
	DensityScore  float64 `json:"density_score"`  // min(recent_articles,40)/4 + 2·lifeline-backed
	SparseHistory int     `json:"sparse_history"` // past topic-scope results with form=sparse
}

// laneRow mirrors the board_persistent_topics columns the assembler reads
// (raw query stays consistent with board_listers_impl.go style).
type laneRow struct {
	ID              uint
	Label           string
	Description     string
	Status          string
	LastSeenDate    time.Time
	HitCount        int
	ConsecutiveHits int
}

// assembleSituationCards builds one card per ACTIVE lane of the board, sorted
// by quality DESC (tie: lane_id ASC), capped at situationCardMaxLanes.
func (o *OrchestratorService) assembleSituationCards(ctx context.Context, boardID uint) ([]LaneSituationCard, error) {
	var lanes []laneRow
	if err := o.repo.DB().WithContext(ctx).
		Table("board_persistent_topics").
		Select("id, label, description, status, last_seen_date, hit_count, consecutive_hits").
		Where("semantic_board_id = ? AND status = ?", boardID, "active").
		Order("id ASC").
		Scan(&lanes).Error; err != nil {
		return nil, fmt.Errorf("situation cards: list active lanes for board %d: %w", boardID, err)
	}

	now := time.Now()
	cards := make([]LaneSituationCard, 0, len(lanes))
	for _, ln := range lanes {
		sparse := o.countLaneSparseHistory(ctx, ln.ID)
		digest, source, recentArticles := o.laneFactsDigest(ctx, ln)

		days := int(now.Sub(ln.LastSeenDate).Hours() / 24)
		if days < 0 {
			days = 0
		}
		sig := SituationSignals{
			ActivityScore: situationActivity(ln.ConsecutiveHits, days),
			DensityScore:  situationDensity(recentArticles, source == "lifeline_week" || source == "lifeline_month"),
			SparseHistory: sparse,
		}
		quality := sig.ActivityScore + sig.DensityScore - 3.0*float64(sig.SparseHistory)

		card := LaneSituationCard{
			LaneID:          ln.ID,
			Label:           ln.Label,
			HitCount:        ln.HitCount,
			ConsecutiveHits: ln.ConsecutiveHits,
			LastSeenDate:    ln.LastSeenDate.Format("2006-01-02"),
			DaysSinceSeen:   days,
			FactsDigest:     digest,
			FactsSource:     source,
			QualityScore:    quality,
			Signals:         sig,
			DetailLevel:     "brief",
		}
		// Sparse-history degradation is an explicit rule, not just a weight:
		// a lane whose past analyses repeatedly came out sparse is forced to
		// brief regardless of activity (it has proven it doesn't sustain depth).
		if quality >= situationCardFullQuality && source != "none" && sparse < situationCardSparseDegraded {
			card.DetailLevel = "full"
		}
		// Brief cards still carry a digest, but tighter.
		if card.DetailLevel == "brief" {
			card.FactsDigest = truncateRunes(card.FactsDigest, situationCardBriefRunes)
		}
		cards = append(cards, card)
	}

	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].QualityScore != cards[j].QualityScore {
			return cards[i].QualityScore > cards[j].QualityScore
		}
		return cards[i].LaneID < cards[j].LaneID
	})
	if len(cards) > situationCardMaxLanes {
		cards = cards[:situationCardMaxLanes]
	}
	return cards, nil
}

// countLaneSparseHistory counts past topic-scope results whose form is sparse.
// Form lives inside sectors jsonb, so results are loaded and dispatched in Go
// (result counts per lane are small; extractForm is the shared reader).
func (o *OrchestratorService) countLaneSparseHistory(ctx context.Context, laneID uint) int {
	results, err := o.repo.ListTopicEnrichmentResultsByTopic(ctx, laneID)
	if err != nil {
		return 0 // signal only; degrade to 0 rather than fail assembly
	}
	n := 0
	for _, r := range results {
		if extractForm(r.Sectors) == "sparse" {
			n++
		}
	}
	return n
}

// laneFactsDigest builds the lane's facts digest. Preference: week-granularity
// lifeline summary (latest N periods) → month-granularity lifeline summary
// (production backstop: month is fleet-wide, week lags) → recent daily-report
// section fingerprint (thread titles first, cluster label fallback) → lane
// description → empty. Also returns recent article volume for the density
// signal (sections consulted whenever readable).
func (o *OrchestratorService) laneFactsDigest(ctx context.Context, ln laneRow) (digest, source string, recentArticles int) {
	// Density signal: recent daily-report sections (also the fallback digest).
	sections, secErr := o.recentLaneSections(ln.ID)
	for _, s := range sections {
		recentArticles += s.ArticleCount
	}

	// 1. Week lifeline digest (preferred source).
	if weeks, err := o.repo.ListTopicLifelineContextsByGranularity(ctx, ln.ID, "week"); err == nil && len(weeks) > 0 {
		parts := make([]string, 0, situationCardLifelineWeeks)
		for i, w := range weeks { // ordered period DESC
			if i >= situationCardLifelineWeeks {
				break
			}
			parts = append(parts, fmt.Sprintf("[%s] %s", w.Period, compressSpaces(w.Content)))
		}
		return truncateRunes(strings.Join(parts, " / "), situationCardDigestRunes), "lifeline_week", recentArticles
	}
	// 2. Month lifeline digest (production backstop; rows whose content is
	// blank are skipped — a bare period prefix is not a digest).
	if months, err := o.repo.ListTopicLifelineContextsByGranularity(ctx, ln.ID, "month"); err == nil && len(months) > 0 {
		parts := make([]string, 0, situationCardLifelineMonths)
		for i, m := range months { // ordered period DESC
			if i >= situationCardLifelineMonths {
				break
			}
			if c := compressSpaces(m.Content); c != "" {
				parts = append(parts, fmt.Sprintf("[%s] %s", m.Period, c))
			}
		}
		if len(parts) > 0 {
			return truncateRunes(strings.Join(parts, " / "), situationCardDigestRunes), "lifeline_month", recentArticles
		}
	}
	if secErr == nil && len(sections) > 0 {
		// 3. Recent-section fingerprint: [date] thread titles (or cluster
		// label fallback) (xN articles).
		parts := make([]string, 0, len(sections))
		for _, s := range sections {
			parts = append(parts, fmt.Sprintf("[%s] %s (%d篇)", s.PeriodDate.Format("01-02"), fingerprintSectionLabel(s), s.ArticleCount))
		}
		return truncateRunes(strings.Join(parts, " / "), situationCardDigestRunes), "section_fingerprint", recentArticles
	}
	// 4. Lane description, else nothing.
	if strings.TrimSpace(ln.Description) != "" {
		return truncateRunes(compressSpaces(ln.Description), situationCardDigestRunes), "description", recentArticles
	}
	return "", "none", recentArticles
}

// fingerprintSectionLabel renders one section's substance for the fallback
// fingerprint: up to situationCardFingerprintThreads thread titles joined by
// " | "; falls back to the cluster label when no thread titles are loaded
// (never worse than the pre-fix behaviour).
func fingerprintSectionLabel(s TimelineSectionNode) string {
	if len(s.ThreadTitles) == 0 {
		return s.ClusterLabel
	}
	n := len(s.ThreadTitles)
	if n > situationCardFingerprintThreads {
		n = situationCardFingerprintThreads
	}
	return strings.Join(s.ThreadTitles[:n], " | ")
}

// recentLaneSections returns the lane's latest daily-report sections (newest
// first). Missing topic/sections are not errors — empty slice.
func (o *OrchestratorService) recentLaneSections(laneID uint) ([]TimelineSectionNode, error) {
	data, err := o.lifelineReader.GetTopicLifeline(laneID)
	if err != nil {
		return nil, err
	}
	sort.Slice(data.Sections, func(i, j int) bool {
		return data.Sections[i].PeriodDate.After(data.Sections[j].PeriodDate)
	})
	if len(data.Sections) > situationCardFingerprintSections {
		data.Sections = data.Sections[:situationCardFingerprintSections]
	}
	return data.Sections, nil
}

// situationActivity: 2·min(consecutive_hits,14) + max(0, 14−days_since_seen).
// Hot-and-recent lanes score highest; a lane unseen for 2+ weeks loses all
// recency points.
func situationActivity(consecutiveHits, daysSinceSeen int) float64 {
	act := consecutiveHits
	if act > 14 {
		act = 14
	}
	rec := 14 - daysSinceSeen
	if rec < 0 {
		rec = 0
	}
	return 2.0*float64(act) + float64(rec)
}

// situationDensity: min(recent_articles,40)/4 + 2 when the lane is backed by
// any lifeline archive (week or month) — memory-backed lanes carry more
// analyzable context than bare section lanes.
func situationDensity(recentArticles int, lifelineBacked bool) float64 {
	if recentArticles > 40 {
		recentArticles = 40
	}
	d := float64(recentArticles) / 4.0
	if lifelineBacked {
		d += 2.0
	}
	return d
}

// RenderSituationCardsMarkdown renders cards for prompt injection (board
// interpret, tasks 3.2).
func RenderSituationCardsMarkdown(cards []LaneSituationCard) string {
	if len(cards) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 泳道态势卡（按质量降序）\n")
	for _, c := range cards {
		fmt.Fprintf(&sb, "- 泳道#%d《%s》命中%d天/连续%d天/最近%s（%d天前）质量%.1f[%s]\n  事实: %s\n",
			c.LaneID, c.Label, c.HitCount, c.ConsecutiveHits, c.LastSeenDate, c.DaysSinceSeen,
			c.QualityScore, c.DetailLevel, c.FactsDigest)
	}
	return sb.String()
}

// compressSpaces collapses runs of whitespace/newlines into single spaces.
func compressSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// AssembleSituationCardsForTest exposes the assembler to the external test package.
func (o *OrchestratorService) AssembleSituationCardsForTest(ctx context.Context, boardID uint) ([]LaneSituationCard, error) {
	return o.assembleSituationCards(ctx, boardID)
}
