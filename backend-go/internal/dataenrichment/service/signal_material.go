package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/models"
)

// ── Signal discovery period & material helpers (board-signal-reports) ────────
//
// Phase-0 gap fix: ParsePeriodRange (period.go) builds month/year boundaries
// in time.UTC because the lifeline callers depend on that; the signal chain
// needs BUSINESS-TIMEZONE (Asia/Shanghai) half-open boundaries plus a
// not-in-the-future rule, so these helpers are written fresh here and the
// existing period helpers stay untouched (其他消费方依赖不变).

var signalPeriodMonthRe = regexp.MustCompile(`^([0-9]{4})-(0[1-9]|1[0-2])$`)
var signalPeriodYearRe = regexp.MustCompile(`^[0-9]{4}$`)

// SignalPeriod is the parsed, validated discovery window.
type SignalPeriod struct {
	Granularity string    // month | year
	Period      string    // YYYY-MM | YYYY
	From        time.Time // inclusive, business timezone
	To          time.Time // exclusive, business timezone (half-open [From,To))
}

// ParseSignalPeriod validates granularity+period for signal discovery and
// computes the half-open [From,To) window in the business timezone
// (models.ShanghaiTZ). Only month|year are legal (design §2); errors are
// user-facing — the handler maps them to 400 without starting a job (PC-1),
// future periods are rejected (PC-2).
func ParseSignalPeriod(granularity, period string, now time.Time) (SignalPeriod, error) {
	switch granularity {
	case repository.SignalGranularityMonth, repository.SignalGranularityYear:
	default:
		return SignalPeriod{}, fmt.Errorf("granularity 必须是 month|year，收到 %q", granularity)
	}
	if period == "" || len(period) > 32 {
		return SignalPeriod{}, fmt.Errorf("period 非法：%q", period)
	}
	nowLocal := now.In(models.ShanghaiTZ)
	switch granularity {
	case repository.SignalGranularityMonth:
		m := signalPeriodMonthRe.FindStringSubmatch(period)
		if m == nil {
			return SignalPeriod{}, fmt.Errorf("period 必须是 YYYY-MM，收到 %q", period)
		}
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if year < 2000 || year > 2100 {
			return SignalPeriod{}, fmt.Errorf("period 年份 %d 超出支持范围 2000..2100", year)
		}
		if period > nowLocal.Format("2006-01") {
			return SignalPeriod{}, fmt.Errorf("period %s 是未来周期（当前 %s）", period, nowLocal.Format("2006-01"))
		}
		from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, models.ShanghaiTZ)
		return SignalPeriod{Granularity: granularity, Period: period, From: from, To: from.AddDate(0, 1, 0)}, nil
	default: // year (validated above)
		if !signalPeriodYearRe.MatchString(period) {
			return SignalPeriod{}, fmt.Errorf("period 必须是 YYYY，收到 %q", period)
		}
		year, _ := strconv.Atoi(period)
		if year < 2000 || year > 2100 {
			return SignalPeriod{}, fmt.Errorf("period 年份 %d 超出支持范围 2000..2100", year)
		}
		if period > strconv.Itoa(nowLocal.Year()) {
			return SignalPeriod{}, fmt.Errorf("period %s 是未来周期（当前 %d）", period, nowLocal.Year())
		}
		from := time.Date(year, 1, 1, 0, 0, 0, 0, models.ShanghaiTZ)
		return SignalPeriod{Granularity: granularity, Period: period, From: from, To: from.AddDate(1, 0, 0)}, nil
	}
}

// SignalCutoff computes the discovery data boundary (design §2): the CURRENT
// period cuts off at the discovery job's start (now); an ENDED historical
// period cuts off at the period end (the exclusive To boundary). PC-2.
func SignalCutoff(p SignalPeriod, now time.Time) time.Time {
	nowLocal := now.In(models.ShanghaiTZ)
	if nowLocal.Before(p.To) {
		return nowLocal
	}
	return p.To
}

// SignalAnalysisMode derives the batch's analysis mode: an ended historical
// period is retrospective (事后回顾 · 本次取得的数据版本, design §2).
func SignalAnalysisMode(p SignalPeriod, cutoff time.Time) string {
	if !cutoff.Before(p.To) {
		return repository.SignalAnalysisModeRetrospective
	}
	return repository.SignalAnalysisModeCurrent
}

// ── period-scoped material (pure core; PC-3/4/5) ─────────────────────────────

// SignalArticleSlice is one dated lane news slice inside the window.
type SignalArticleSlice struct {
	SectionID    uint      `json:"section_id"`
	PeriodDate   time.Time `json:"period_date"`
	ClusterLabel string    `json:"cluster_label"`
	ArticleCount int       `json:"article_count"`
	ThreadCount  int       `json:"thread_count"`
	ThreadTitles []string  `json:"thread_titles,omitempty"`
}

// SignalBackgroundSummary is one strictly-earlier archive summary kept as
// long-horizon background (constraint #20 lineage: 历史背景记忆段).
type SignalBackgroundSummary struct {
	Granularity string `json:"granularity"`
	Period      string `json:"period"`
	Content     string `json:"content"`
}

// SignalLaneMaterial is one lane's period-scoped material.
type SignalLaneMaterial struct {
	LaneID uint   `json:"lane_id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	// Summary is the lifeline summary for EXACTLY the target period; nil when
	// absent or unusable (as-of after the cutoff would mix in post-period
	// facts → falls back to dated raw slices, PC-3).
	Summary *string `json:"summary,omitempty"`
	// Background is the newest strictly-earlier archive summary with
	// as-of ≤ cutoff; nil when none (PC-3 / design §2 历史背景不晚于 cutoff).
	Background *SignalBackgroundSummary `json:"background,omitempty"`
	Articles   []SignalArticleSlice     `json:"articles"`
}

// SignalMaterialGap records honestly what could not be reconstructed.
type SignalMaterialGap struct {
	LaneID uint   `json:"lane_id,omitempty"`
	Reason string `json:"reason"`
}

// SignalMaterial is the frozen discovery input: the batch stores it in
// input_snapshot and research consumes the same frozen snapshot later
// (PC-5: 不偷偷换成最新材料).
type SignalMaterial struct {
	Granularity  string               `json:"granularity"`
	Period       string               `json:"period"`
	AnalysisMode string               `json:"analysis_mode"`
	From         time.Time            `json:"from"`
	To           time.Time            `json:"to"`
	Cutoff       time.Time            `json:"cutoff"`
	Lanes        []SignalLaneMaterial `json:"lanes"`
	Gaps         []SignalMaterialGap  `json:"gaps"`
}

// IsEmpty reports a zero-material window: detect need not be invoked and the
// batch saves as a quiet zero-candidate outcome (PC-4).
func (m *SignalMaterial) IsEmpty() bool {
	return len(m.Lanes) == 0
}

// filterSignalSections keeps slices whose PeriodDate ∈ [from, cutoff) and
// orders them oldest→newest. The cutoff bound is the freeze: anything
// published at/after the cutoff stays out even if the window technically
// extends further (PC-5: 候选生成后的新新闻不进本期材料).
func filterSignalSections(sections []TimelineSectionNode, from, cutoff time.Time) []SignalArticleSlice {
	out := make([]SignalArticleSlice, 0, len(sections))
	for _, s := range sections {
		if s.PeriodDate.Before(from) || !s.PeriodDate.Before(cutoff) {
			continue
		}
		out = append(out, SignalArticleSlice{
			SectionID:    s.SectionID,
			PeriodDate:   s.PeriodDate,
			ClusterLabel: s.ClusterLabel,
			ArticleCount: s.ArticleCount,
			ThreadCount:  s.ThreadCount,
			ThreadTitles: s.ThreadTitles,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PeriodDate.Before(out[j].PeriodDate) })
	return out
}

// selectSignalPeriodSummary picks the lifeline summary for EXACTLY the target
// period, but only when it was already on record at the cutoff (AsOfDate ≤
// cutoff): a summary written later may fold post-period facts into its
// narrative — unusable as period evidence, the dated raw slices stand in
// instead (PC-3: 混后期摘要排除/原切片降级).
func selectSignalPeriodSummary(rows []LifelineArchiveRow, granularity, period string, cutoff time.Time) *string {
	for _, r := range rows {
		if r.Granularity == granularity && r.Period == period && !r.AsOfDate.After(cutoff) && r.Content != "" {
			summary := r.Content
			return &summary
		}
	}
	return nil
}

// selectSignalBackgroundSummary picks the newest archive summary STRICTLY
// before the target period whose as-of is not after the cutoff — the lane's
// long-horizon background never reaches past the freeze (PC-3).
func selectSignalBackgroundSummary(rows []LifelineArchiveRow, granularity, period string, cutoff time.Time) *SignalBackgroundSummary {
	var best *SignalBackgroundSummary
	for _, r := range rows {
		if r.Granularity != granularity || r.Period >= period || r.AsOfDate.After(cutoff) || r.Content == "" {
			continue
		}
		if best == nil || r.Period > best.Period {
			best = &SignalBackgroundSummary{Granularity: r.Granularity, Period: r.Period, Content: r.Content}
		}
	}
	return best
}

// appendSignalMembershipGap records the honest membership gap for
// historical windows: lane membership history is not persisted, so the
// assembly approximates with the CURRENT lane list — exits/moves during the
// window cannot be reconstructed and are never guessed (PC-4).
func appendSignalMembershipGap(m *SignalMaterial) {
	m.Gaps = append(m.Gaps, SignalMaterialGap{
		Reason: "泳道无历史归属记录：材料按当前泳道清单近似，周期内的退出/迁移不可还原",
	})
}

// SignalMaterialBuilder assembles the period-scoped discovery input from the
// lane table and the lifeline reader. Freshness is NOT this builder's job —
// the discovery orchestration keeps the existing ensureLaneFreshness rules
// upstream (只读复用，不重写).
type SignalMaterialBuilder struct {
	db     *gorm.DB
	reader LifelineReader
}

func NewSignalMaterialBuilder(db *gorm.DB, reader LifelineReader) *SignalMaterialBuilder {
	return &SignalMaterialBuilder{db: db, reader: reader}
}

// AssembleSignalMaterial collects each lane's in-window slices and summaries.
// 候选泳道来自周期材料，不只用今天 active 泳道：ALL lanes are inspected (any
// status); a lane created after the cutoff cannot have been a board member
// during the window and is skipped; lanes with zero in-window material are
// omitted rather than padded (PC-4: 不捏造历史全貌).
func (b *SignalMaterialBuilder) AssembleSignalMaterial(ctx context.Context, boardID uint, p SignalPeriod, cutoff time.Time) (*SignalMaterial, error) {
	type laneRow struct {
		ID        uint      `gorm:"column:id"`
		Label     string    `gorm:"column:label"`
		Status    string    `gorm:"column:status"`
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	var lanes []laneRow
	if err := b.db.WithContext(ctx).
		Table("board_persistent_topics").
		Select("id, label, status, created_at").
		Where("semantic_board_id = ?", boardID).
		Order("id ASC").
		Scan(&lanes).Error; err != nil {
		return nil, fmt.Errorf("signal material: list lanes for board %d: %w", boardID, err)
	}

	material := &SignalMaterial{
		Granularity:  p.Granularity,
		Period:       p.Period,
		AnalysisMode: SignalAnalysisMode(p, cutoff),
		From:         p.From,
		To:           p.To,
		Cutoff:       cutoff,
		Lanes:        []SignalLaneMaterial{},
		Gaps:         []SignalMaterialGap{},
	}

	for _, lane := range lanes {
		if lane.CreatedAt.After(cutoff) {
			continue // 创建晚于 cutoff：窗口内必然不归属，不算 gap
		}
		data, err := b.reader.GetTopicLifeline(lane.ID)
		if err != nil {
			material.Gaps = append(material.Gaps, SignalMaterialGap{LaneID: lane.ID, Reason: fmt.Sprintf("泳道切片读取失败：%v", err)})
			continue
		}
		articles := filterSignalSections(data.Sections, p.From, cutoff)
		archive, err := b.reader.GetTopicLifelineArchive(lane.ID)
		if err != nil {
			material.Gaps = append(material.Gaps, SignalMaterialGap{LaneID: lane.ID, Reason: fmt.Sprintf("泳道归档摘要读取失败：%v", err)})
			archive = nil
		}
		summary := selectSignalPeriodSummary(archive, p.Granularity, p.Period, cutoff)
		background := selectSignalBackgroundSummary(archive, p.Granularity, p.Period, cutoff)
		if len(articles) == 0 && summary == nil {
			continue // 周期内零材料：不进材料，也不伪造
		}
		material.Lanes = append(material.Lanes, SignalLaneMaterial{
			LaneID:     lane.ID,
			Label:      lane.Label,
			Status:     lane.Status,
			Summary:    summary,
			Background: background,
			Articles:   articles,
		})
	}

	if material.AnalysisMode == repository.SignalAnalysisModeRetrospective {
		appendSignalMembershipGap(material)
	}
	return material, nil
}
