package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 首义来源（读侧口径）。
//
// 与写侧常量分开：写侧记录的是**输入模式**（"selected_sections"——首义由选定节点范围定义），
// 读侧回答的是「当前首义是怎么来的、对应哪些节点」：
//   - TopicAnchorSourceAuto：没有人工改写记录 → 首义是创建话题时写入的（无节点集）；
//   - TopicAnchorSourceManual：有人工改写记录 → 首义是人工选定的那组节点（节点集可查）。
//
// 「自动首义无节点集」是契约硬约束（lane-correction-surface「自动首义不虚构节点列表」）：
// 自动首义只有一条向量，没有对应的节点列表，界面不得替它编一个。
const (
	// TopicAnchorSourceAuto —— 首义来自创建话题时写入，无节点集。
	TopicAnchorSourceAuto = "auto"
	// TopicAnchorSourceManual —— 首义来自人工选定的节点范围，节点集见 SectionIDs。
	TopicAnchorSourceManual = "manual"
)

// TopicAnchorOrigin describes the node the current 首义 can be traced to.
//
// resolved is the only semantic switch (design D5):
//   - true  → the source node is knowable: SectionID / Title / PeriodDate carry
//     the node that the trail recorded;
//   - false → the source is **not** knowable: the three node fields are null.
//
// A false resolved is a final shape, not a placeholder: an auto anchor is a
// single vector with no node list, so the read side never substitutes a guess
// (the "最早成员顶替" trick is explicitly forbidden — 虚构即编造).
type TopicAnchorOrigin struct {
	// Resolved tells whether the source node is knowable.
	Resolved bool `json:"resolved"`
	// SectionID is the source node's id; nil when not resolved.
	SectionID *uint `json:"section_id"`
	// Title is the source node's title (section cluster_label); nil when not resolved.
	Title *string `json:"title"`
	// PeriodDate is the source node's report period (YYYY-MM-DD); nil when not resolved.
	PeriodDate *string `json:"period_date"`
}

// CentroidWindowMember is one section that participates in the rolling
// centroid computation — the read-only detail behind centroid.section_ids.
type CentroidWindowMember struct {
	// SectionID is the participating section's id.
	SectionID uint `json:"section_id"`
	// Title is the section's cluster label (the node title in the workbench).
	Title string `json:"title"`
	// PeriodDate is the owning report's period date (YYYY-MM-DD).
	PeriodDate string `json:"period_date"`
}

// TopicAnchorCentroid describes the rolling-centroid 口径 and its inputs:
// the window size, the 首义 weight (α), the sections that participate in the
// rolling mean and whether the current centroid is degraded.
//
// The member set comes from the same 取数实现 the centroid computation uses
// (ListCentroidWindowSections), so what the workbench shows is what the
// centroid actually consumed — one implementation, no drift.
type TopicAnchorCentroid struct {
	// WindowSize is the rolling window size (persistent_topic_centroid_window,
	// default 30).
	WindowSize int `json:"window_size"`
	// AnchorWeight is α (persistent_topic_centroid_anchor_weight, default 0.5).
	AnchorWeight float64 `json:"anchor_weight"`
	// SectionIDs are the sections that actually participate in the rolling
	// mean (same order as Members).
	SectionIDs []uint `json:"section_ids"`
	// SectionCount is len(SectionIDs) — carried explicitly so the documented
	// response shape (docs/reference/api/daily-reports.md) is reproducible
	// without the client re-deriving it from the array length.
	SectionCount int `json:"section_count"`
	// Members is the read-only detail of SectionIDs.
	Members []CentroidWindowMember `json:"members"`
	// Degraded is true when the current centroid fell back to the 首义 vector
	// (fewer than 2 parseable vectors) or when the 首义 itself is unusable
	// (empty / unparseable / dimension mismatch).
	Degraded bool `json:"degraded"`
}

// TopicAnchorState is the read-side view of a topic's current 首义.
//
// 前端泳道修正工作面用它展示「当前首义来源」（自动 / 人工 + 节点数）并决定
// 首义选择池的边界（窗口内节点 + 当前首义集，含窗口外节点）。
type TopicAnchorState struct {
	TopicID uint `json:"topic_id"`
	// Source is "auto" or "manual"（见上方常量）。
	Source string `json:"source"`
	// SectionIDs is the current anchor node set. Empty for an auto anchor:
	// an auto anchor is a single vector, it has no node list to show.
	SectionIDs []uint `json:"section_ids"`
	// OverrideCount is the topic's 首义留痕条数 (topic_anchor_overrides rows).
	// A manual topic created through CreateManualTopic contributes one
	// creation-trail row, so it counts here too (design D9).
	OverrideCount int `json:"override_count"`
	// EffectiveFrom marks when the current anchor took effect on the matching
	// side. "next_report" for a manual rewrite (the centroid is recomputed by
	// the next daily-report round); empty for an auto anchor (already in use).
	EffectiveFrom string `json:"effective_from"`
	// AnchorOrigin is the source node of the current 首义 (可考 / 不可考).
	AnchorOrigin TopicAnchorOrigin `json:"anchor_origin"`
	// Centroid is the rolling-window 口径 behind the current centroid.
	Centroid TopicAnchorCentroid `json:"centroid"`
}

// GetTopicAnchorState reads the topic's current 首义 state.
//
// Read-only: it never writes and never recomputes anything. The audit trail
// (`topic_anchor_overrides`, append-only) is the source of truth for the
// manual case — the latest row carries the chosen section ids in
// `input_summary`; the auto case has no row and therefore no node set.
//
// The response carries two extra read-only blocks (align-lane-correction-workface
// D5/D7): the source node of the 首义 (可考 / 不可考) and the rolling-centroid
// 口径 (window size, α, participating sections, degraded flag). Both are
// derived by SELECT only — the endpoint writes nothing, never touches
// centroid/embedding and never flags a recompute.
func (r *TopicGraphRepository) GetTopicAnchorState(topicID uint) (*TopicAnchorState, error) {
	var topic BoardPersistentTopic
	if err := r.db.Select("id").First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}

	var count int64
	if err := r.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", topicID).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count anchor overrides for topic %d: %w", topicID, err)
	}

	state := &TopicAnchorState{
		TopicID:       topicID,
		Source:        TopicAnchorSourceAuto,
		SectionIDs:    []uint{},
		OverrideCount: int(count),
	}
	if count > 0 {
		var latest TopicAnchorOverride
		if err := r.db.Where("topic_id = ?", topicID).
			Order("created_at DESC, id DESC").
			First(&latest).Error; err != nil {
			return nil, fmt.Errorf("load latest anchor override for topic %d: %w", topicID, err)
		}
		state.Source = TopicAnchorSourceManual
		state.SectionIDs = parseAnchorInputSummary(latest.InputSummary)
		state.EffectiveFrom = AnchorEffectiveFromNextReport
	}

	if err := r.resolveAnchorOrigin(state); err != nil {
		return nil, err
	}
	if err := r.assembleAnchorCentroid(state, topicID); err != nil {
		return nil, err
	}
	return state, nil
}

// anchorOriginSection is one still-existing node of the 首义留痕 (id + the
// display fields the workbench shows).
type anchorOriginSection struct {
	ID         uint
	Title      string
	PeriodDate time.Time
}

// resolveAnchorOrigin fills state.AnchorOrigin from the trail's node set.
//
// 可考 = 留痕里有一个仍然存在的节点；不可考 = 没有留痕（自动首义只有一条向量）
// 或留痕里的节点都已被删除。后者一律返回 null —— 不得拿「最早成员」顶替
// （design D5：不可考即不可考，resolved=false 是终态而非占位）。
//
// 取数走**一次** `WHERE id IN ?`（不再按留痕节点逐条查询），再按 SectionIDs
// 原顺序挑第一个仍存在的节点——「留痕顺序优先」的语义与逐条试同一结果。
func (r *TopicGraphRepository) resolveAnchorOrigin(state *TopicAnchorState) error {
	if len(state.SectionIDs) == 0 {
		// 无留痕 / 留痕为空 → 来源不可考（无需取数）。
		state.AnchorOrigin = TopicAnchorOrigin{Resolved: false}
		return nil
	}

	var rows []anchorOriginSection
	err := r.db.Raw(`
		SELECT s.id AS id, s.cluster_label AS title, rpt.period_date AS period_date
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.id IN ?
	`, state.SectionIDs).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("load anchor origin sections: %w", err)
	}

	byID := make(map[uint]anchorOriginSection, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, sectionID := range state.SectionIDs {
		row, ok := byID[sectionID]
		if !ok {
			// 该节点已被删除 → 试下一个留痕节点。
			continue
		}
		sourceID := row.ID
		title := row.Title
		periodDate := formatPeriodDate(row.PeriodDate)
		state.AnchorOrigin = TopicAnchorOrigin{
			Resolved:   true,
			SectionID:  &sourceID,
			Title:      &title,
			PeriodDate: &periodDate,
		}
		return nil
	}
	// 无留痕 / 留痕节点均已不存在 → 来源不可考。
	state.AnchorOrigin = TopicAnchorOrigin{Resolved: false}
	return nil
}

// assembleAnchorCentroid fills state.Centroid from the shared rolling-window
// 取数实现 (ListCentroidWindowSections) — the very same implementation the
// centroid computation uses, so the displayed member set is the set the
// centroid consumed (design D7：一份实现，两处生效).
//
// degraded mirrors ComputeTopicCentroid's degradation 口径: fewer than 2
// parseable vectors (the centroid fell back to the 首义 vector) or an unusable
// 首义 (empty / unparseable / dimension mismatch against the rolling mean).
func (r *TopicGraphRepository) assembleAnchorCentroid(state *TopicAnchorState, topicID uint) error {
	cfg := LoadPersistentTopicConfig(r.db)
	window := cfg.CentroidWindow
	if window <= 0 {
		window = DefaultPersistentTopicConfig().CentroidWindow
	}

	rows, err := r.ListCentroidWindowSections(topicID, window)
	if err != nil {
		return err
	}

	members := make([]CentroidWindowMember, 0, len(rows))
	ids := make([]uint, 0, len(rows))
	vecs := make([][]float64, 0, len(rows))
	for _, rw := range rows {
		v, perr := repoParsePgVector(rw.Embedding)
		if perr != nil || len(v) == 0 {
			// 不可解析的向量不参与滚动均值，也不进展示集合 —— 两者必须一致。
			continue
		}
		vecs = append(vecs, v)
		ids = append(ids, rw.SectionID)
		members = append(members, CentroidWindowMember{
			SectionID:  rw.SectionID,
			Title:      rw.Title,
			PeriodDate: formatPeriodDate(rw.PeriodDate),
		})
	}

	degraded := len(vecs) < 2
	if !degraded {
		anchor, err := r.topicEmbeddingVector(topicID)
		if err != nil {
			return err
		}
		mean := meanPgVectors(vecs)
		_, ok := blendCentroidWithAnchor(mean, anchor, cfg.CentroidAnchorWeight)
		degraded = !ok
	}

	state.Centroid = TopicAnchorCentroid{
		WindowSize:   window,
		AnchorWeight: cfg.CentroidAnchorWeight,
		SectionIDs:   ids,
		SectionCount: len(ids),
		Members:      members,
		Degraded:     degraded,
	}
	return nil
}

// formatPeriodDate renders a report period date as "YYYY-MM-DD" (UTC).
// The read side keeps the raw date column in the query (dialect-neutral) and
// formats it here, so the same SQL runs on PostgreSQL and on the SQLite-backed
// handler tests.
func formatPeriodDate(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// parseAnchorInputSummary reads the audit row's「输入摘要」(the JSON array of
// chosen section ids) back into a list. A malformed summary yields an empty
// set rather than a fabricated one — an unreadable trail must not turn into
// an invented node list.
func parseAnchorInputSummary(raw string) []uint {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []uint{}
	}
	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return []uint{}
	}
	return dedupeSectionIDs(ids)
}
