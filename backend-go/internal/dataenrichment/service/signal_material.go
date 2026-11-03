package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph"
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

// topicThreadSourceKeyForSignal mirrors the marker fingerprint using the
// SAME exported topicgraph implementation (exact-match semantics; a drifted
// local copy would silently break evidence verification).
func topicThreadSourceKeyForSignal(boardID, laneID uint, periodDate time.Time, title string, tagJSON, artJSON []byte) string {
	var tags, arts []uint
	_ = json.Unmarshal(tagJSON, &tags)
	_ = json.Unmarshal(artJSON, &arts)
	return topicgraph.ComputeKeyEventSourceKey(boardID, laneID, periodDate, title, tags, arts)
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

// SignalKeyEventAttention is one user-confirmed key event inside the window
// (add-topic-key-event-markers design D4): an ATTENTION hint only — never a
// signal candidate, never an evidence id. Frozen with the material batch;
// a later confirm/cancel does NOT rewrite old batches.
type SignalKeyEventAttention struct {
	MarkerID uint   `json:"marker_id"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	// SourcePeriodDate is the SOURCE REPORT date — not a claim about when the
	// event actually happened.
	SourcePeriodDate time.Time `json:"source_period_date"`
	ConfirmedAt      time.Time `json:"confirmed_at"`
	// CurrentRetrospectiveAttention marks choices made AFTER the cutoff
	// (e.g. today's confirmation of a past event) — 本次回顾关注，不是当时可得
	// 的预测信号 (board-signal-reports spec).
	CurrentRetrospectiveAttention bool `json:"current_retrospective_attention"`
	// EvidenceVerified: the marker's source thread maps to a whitelisted
	// slice of THIS material (live section or exact-fingerprint match).
	// false → the lane records a gap and the snapshot never poses as a valid
	// news slice (失效依据不升级为有效信号).
	EvidenceVerified bool `json:"evidence_verified"`
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
	// ConfirmedKeyEvents are the user's in-window confirmed key events
	// (attention hints; optional so pre-marker payloads stay readable).
	ConfirmedKeyEvents []SignalKeyEventAttention `json:"confirmed_key_events,omitempty"`
}

// SignalMaterialGap records honestly what could not be reconstructed.
type SignalMaterialGap struct {
	LaneID uint   `json:"lane_id,omitempty"`
	Reason string `json:"reason"`
}

// SignalMaterial is the frozen discovery input: the batch stores it in
// input_snapshot and research consumes the same frozen snapshot later
// (PC-5: 不偷偷换成最新材料).
// ResearchContract 标记发现时的研究协议版本（question-v1）——空 = 旧冻结研
// 究协议，未知版本拒绝研究；Sources 是 question-v1 发现后冻结的有界本地来
// 源快照（正文只在候选形成后从本地按关联取样冻结，不送入发现模型）。
type SignalMaterial struct {
	Granularity      string               `json:"granularity"`
	Period           string               `json:"period"`
	AnalysisMode     string               `json:"analysis_mode"`
	From             time.Time            `json:"from"`
	To               time.Time            `json:"to"`
	Cutoff           time.Time            `json:"cutoff"`
	ResearchContract string               `json:"research_contract,omitempty"`
	Sources          []SignalFrozenSource `json:"sources,omitempty"`
	Lanes            []SignalLaneMaterial `json:"lanes"`
	Gaps             []SignalMaterialGap  `json:"gaps"`
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

// collectConfirmedKeyEvents loads the lane's in-window confirmed key events
// (add-topic-key-event-markers design D4). Window predicate mirrors
// filterSignalSections: source report date ∈ [from, cutoff). Only confirmed
// rows enter (未确认建议不作为用户关注意图)； confirmations made after the
// cutoff are flagged CurrentRetrospectiveAttention (事后选择不冒充当时预测).
// Evidence verification: the marker's source must map to a whitelisted slice
// (live SourceSectionID or an EXACT source fingerprint on a whitelisted
// thread) — otherwise the lane records an honest gap and the attention keeps
// evidence_verified=false so the snapshot never poses as a valid news slice.
func (b *SignalMaterialBuilder) collectConfirmedKeyEvents(ctx context.Context, boardID uint, laneID uint, label string, articles []SignalArticleSlice, p SignalPeriod, cutoff time.Time) ([]SignalKeyEventAttention, []SignalMaterialGap) {
	var markers []struct {
		ID            uint      `gorm:"column:id"`
		Title         string    `gorm:"column:title_snapshot"`
		Summary       string    `gorm:"column:summary_snapshot"`
		PeriodDate    time.Time `gorm:"column:source_period_date"`
		ConfirmedAt   time.Time `gorm:"column:confirmed_at"`
		SourceKey     string    `gorm:"column:source_key"`
		SourceSection *uint     `gorm:"column:source_section_id"`
	}
	if err := b.db.WithContext(ctx).
		Table("topic_key_event_markers").
		Select("id, title_snapshot, summary_snapshot, source_period_date, confirmed_at, source_key, source_section_id").
		Where("persistent_topic_id = ? AND semantic_board_id = ? AND state = 'confirmed'", laneID, boardID).
		Order("source_period_date DESC, id DESC").
		Find(&markers).Error; err != nil {
		return nil, []SignalMaterialGap{{LaneID: laneID, Reason: fmt.Sprintf("已确认关键节点读取失败：%v", err)}}
	}
	if len(markers) == 0 {
		return nil, nil
	}

	whitelist := make(map[uint]bool, len(articles))
	for _, a := range articles {
		whitelist[a.SectionID] = true
	}
	// Exact-fingerprint fallback: compute source keys for the threads of the
	// whitelisted sections (bounded by window material).
	fingerprintSet := make(map[string]bool)
	if len(whitelist) > 0 {
		sectionIDs := make([]uint, 0, len(whitelist))
		for id := range whitelist {
			sectionIDs = append(sectionIDs, id)
		}
		var threads []struct {
			SectionID uint      `gorm:"column:section_id"`
			Title     string    `gorm:"column:title"`
			TagIDs    []byte    `gorm:"column:tag_ids"`
			ArtIDs    []byte    `gorm:"column:related_article_ids"`
			Period    time.Time `gorm:"column:period_date"`
		}
		if err := b.db.WithContext(ctx).
			Table("daily_report_threads t").
			Select("t.section_id, t.title, t.tag_ids, t.related_article_ids, r.period_date").
			Joins("JOIN daily_report_sections s ON s.id = t.section_id").
			Joins("JOIN board_daily_reports r ON r.id = s.report_id").
			Where("t.section_id IN ?", sectionIDs).
			Find(&threads).Error; err == nil {
			for _, th := range threads {
				fingerprintSet[topicThreadSourceKeyForSignal(boardID, laneID, th.Period, th.Title, th.TagIDs, th.ArtIDs)] = true
			}
		}
	}

	out := make([]SignalKeyEventAttention, 0, len(markers))
	var gaps []SignalMaterialGap
	for _, m := range markers {
		if m.PeriodDate.Before(p.From) || !m.PeriodDate.Before(cutoff) {
			continue // 窗外/晚于 cutoff：不进本次关注
		}
		// Fingerprint verification subsumes the live-pointer check: a live
		// source thread inside the whitelist computes the SAME key, while a
		// detached/replaced source (section alive but thread deleted) does not.
		verified := fingerprintSet[m.SourceKey]
		if !verified {
			gaps = append(gaps, SignalMaterialGap{LaneID: laneID, Reason: fmt.Sprintf("已确认关键节点「%s」在本次材料中无有效新闻切片（来源已失效），仅作关注线索，不作为证据", m.Title)})
		}
		confirmedAt := m.ConfirmedAt
		out = append(out, SignalKeyEventAttention{
			MarkerID:                      m.ID,
			Title:                         m.Title,
			Summary:                       m.Summary,
			SourcePeriodDate:              m.PeriodDate,
			ConfirmedAt:                   confirmedAt,
			CurrentRetrospectiveAttention: confirmedAt.After(cutoff),
			EvidenceVerified:              verified,
		})
	}
	return out, gaps
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
		keyEvents, keyGaps := b.collectConfirmedKeyEvents(ctx, boardID, lane.ID, lane.Label, articles, p, cutoff)
		material.Gaps = append(material.Gaps, keyGaps...)
		material.Lanes = append(material.Lanes, SignalLaneMaterial{
			LaneID:             lane.ID,
			Label:              lane.Label,
			Status:             lane.Status,
			Summary:            summary,
			Background:         background,
			Articles:           articles,
			ConfirmedKeyEvents: keyEvents,
		})
	}

	if material.AnalysisMode == repository.SignalAnalysisModeRetrospective {
		appendSignalMembershipGap(material)
	}
	return material, nil
}

// ── 候选依据 → 话题关联（redesign-enrichment-research-workbench 2.3 / D3）──────
//
// 关联只解现有冻结数据：候选 evidence_refs（材料切片 ID）对照批次冻结的
// input_snapshot（SignalMaterial），由 Lanes[].Articles[].SectionID 反查
// LaneID（= board_persistent_topics.id）。当前话题表仅做存在性/板块归属校验
// 并提供当前名称；冻结 label/status 保留为历史来源。可验证归属的归档话题
// 照常返回（不只限今天 active）；旧快照缺映射、话题删除/迁移板块 → gap，
// 不按今天 active 清单、标题相似度或最新摘要重建。ConfirmedKeyEvents 只是
// 关注提示（相邻 change 契约），绝不进入关联/证据白名单。

// SignalRelatedTopic is one verified topic relation derived from the
// candidate's frozen evidence.
type SignalRelatedTopic struct {
	TopicID      uint   `json:"topic_id"`
	Label        string `json:"label"`         // 当前名称（实时读取，仅展示用）
	FrozenLabel  string `json:"frozen_label"`  // 冻结材料里的历史 label
	FrozenStatus string `json:"frozen_status"` // 冻结时的状态
	SectionIDs   []uint `json:"section_ids"`   // 支撑本关联的证据切片
}

// MapSignalEvidenceToLanes decodes the candidate's evidence refs (slice-id
// strings) against the FROZEN material and returns laneID → supporting
// section ids. Unknown refs (not in the frozen whitelist) are returned as
// gaps and never guessed onto a lane.
func MapSignalEvidenceToLanes(material *SignalMaterial, refs []string) (map[uint][]uint, []string) {
	sectionLane := make(map[uint]uint)
	for _, lane := range material.Lanes {
		for _, article := range lane.Articles {
			sectionLane[article.SectionID] = lane.LaneID
		}
	}
	byLane := make(map[uint][]uint)
	var gaps []string
	seenRef := make(map[string]bool, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seenRef[ref] {
			continue
		}
		seenRef[ref] = true
		id, err := strconv.ParseUint(ref, 10, 64)
		if err != nil {
			gaps = append(gaps, fmt.Sprintf("依据 %q 不是材料切片 ID，无法关联话题", ref))
			continue
		}
		laneID, ok := sectionLane[uint(id)]
		if !ok {
			gaps = append(gaps, fmt.Sprintf("依据切片 %d 不在冻结材料内，归属不可验证", id))
			continue
		}
		byLane[laneID] = append(byLane[laneID], uint(id))
	}
	return byLane, gaps
}

// ResolveSignalRelatedTopics verifies the mapped lanes against the CURRENT
// topic table (existence + board ownership only — archived topics pass) and
// returns the verified relations plus honest gaps for lanes that vanished or
// moved boards. Zero LLM; the input snapshot is never rewritten.
func ResolveSignalRelatedTopics(ctx context.Context, db *gorm.DB, boardID uint, evidenceRefs, inputSnapshot json.RawMessage) ([]SignalRelatedTopic, []string, error) {
	refs := repository.SignalEvidenceRefs(evidenceRefs)
	if len(refs) == 0 || len(inputSnapshot) == 0 {
		return []SignalRelatedTopic{}, []string{"候选无证据引用或快照缺失，无法验证话题关联"}, nil
	}
	var material SignalMaterial
	if err := json.Unmarshal(inputSnapshot, &material); err != nil {
		// 快照损坏：如实记 gap，不按猜测重建（单条兼容错误，不伪装空关联）。
		return []SignalRelatedTopic{}, []string{fmt.Sprintf("冻结材料解析失败：%v", err)}, nil
	}
	byLane, gaps := MapSignalEvidenceToLanes(&material, refs)
	if len(byLane) == 0 {
		return []SignalRelatedTopic{}, gaps, nil
	}
	laneIDs := make([]uint, 0, len(byLane))
	for laneID := range byLane {
		laneIDs = append(laneIDs, laneID)
	}
	sort.Slice(laneIDs, func(i, j int) bool { return laneIDs[i] < laneIDs[j] })
	var rows []struct {
		ID     uint   `gorm:"column:id"`
		Label  string `gorm:"column:label"`
		Status string `gorm:"column:status"`
	}
	if err := db.WithContext(ctx).
		Table("board_persistent_topics").
		Select("id, label, status").
		Where("id IN ? AND semantic_board_id = ?", laneIDs, boardID).
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("resolve signal related topics: %w", err)
	}
	frozenByLane := make(map[uint]SignalLaneMaterial, len(material.Lanes))
	for _, lane := range material.Lanes {
		frozenByLane[lane.LaneID] = lane
	}
	verified := make(map[uint]bool, len(rows))
	out := make([]SignalRelatedTopic, 0, len(rows))
	for _, row := range rows {
		verified[row.ID] = true
		frozen := frozenByLane[row.ID]
		sectionIDs := byLane[row.ID]
		sort.Slice(sectionIDs, func(i, j int) bool { return sectionIDs[i] < sectionIDs[j] })
		out = append(out, SignalRelatedTopic{
			TopicID:      row.ID,
			Label:        row.Label,
			FrozenLabel:  frozen.Label,
			FrozenStatus: frozen.Status,
			SectionIDs:   sectionIDs,
		})
	}
	for _, laneID := range laneIDs {
		if !verified[laneID] {
			frozen := frozenByLane[laneID]
			gaps = append(gaps, fmt.Sprintf("话题 %d（冻结名「%s」）已删除或迁移出本板块，关联不可验证", laneID, frozen.Label))
		}
	}
	return out, gaps, nil
}
