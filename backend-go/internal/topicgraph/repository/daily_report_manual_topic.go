package repository

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// aggregateEmbeddings computes the mean pooling vector from a set of float64 slices.
// Vectors with mismatched dimensions (compared to the first usable vector) or empty
// slices are skipped and counted in `skipped`. Returns (nil, len(vectors)) when all
// input vectors are unusable.
func aggregateEmbeddings(vectors [][]float64) (mean []float64, skipped int) {
	if len(vectors) == 0 {
		return nil, 0
	}

	// Find the first usable vector to determine expected dimension.
	var dim int
	var usableCount int
	for _, v := range vectors {
		if len(v) == 0 {
			skipped++
			continue
		}
		if dim == 0 {
			dim = len(v)
			mean = make([]float64, dim)
		}
		if len(v) != dim {
			skipped++
			continue
		}
		for j := 0; j < dim; j++ {
			mean[j] += v[j]
		}
		usableCount++
	}

	if usableCount == 0 {
		return nil, skipped
	}
	for j := 0; j < dim; j++ {
		mean[j] /= float64(usableCount)
	}
	return mean, skipped
}

// detectOutliers returns a boolean slice marking vectors whose distance exceeds
// threshold * 1.3. The caller decides how to use this information (highlight /
// suggest removal in the UI).
//
//nolint:unused // called by upcoming compose-mode UI (task 3.6); test covers it now
func detectOutliers(distances []float64, threshold float64) []bool {
	if len(distances) == 0 {
		return nil
	}
	cutoff := threshold * 1.3
	flags := make([]bool, len(distances))
	for i, d := range distances {
		if d > cutoff {
			flags[i] = true
		}
	}
	return flags
}

// (formatPgVector removed — use the existing FloatsToPgVector helper in daily_report_models.go.)

// ComposeWindowMaxDays is the supported window length of the compose
// workbench (both ends included). unify-lane-workbench OQ-2（2026-11-16
// 用户裁决）：由 90 放宽到 365——新增泳道可看一年真实可选池。三处必须一致：
// 本常量（repository 钳位 + handler 校验共用）、parseComposeWindowParams 的
// 报错文案（引用本常量）、前端 validateCustomRange 的上限。
const ComposeWindowMaxDays = 365

// ComposeCandidateSection is a section with its embedding exposed as a float
// slice, dedicated to the manual-compose workbench. The regular timeline node
// never serializes the 4096-dim vector; this struct is the only payload that
// does, so the frontend can compute the aggregate anchor and outlier distances
// in real time without round-tripping the server on every checkbox toggle.
type ComposeCandidateSection struct {
	ID                   uint                  `json:"id"`
	ReportID             uint                  `json:"report_id"`
	PeriodDate           time.Time             `json:"period_date"`
	ClusterLabel         string                `json:"cluster_label"`
	Embedding            []float64             `json:"embedding,omitempty"`
	PersistentTopicID    *uint                 `json:"persistent_topic_id,omitempty"`
	TopicMatchConfidence string                `json:"topic_match_confidence,omitempty"`
	PersistentTopic      *PersistentTopicBrief `json:"persistent_topic,omitempty"`
	// ClueCount is the section's non-composite (非综合稿) thread count
	// (unify-lane-workbench D2)：曲线与可选池的线索数口径，与
	// listLaneSectionThreads 的 Clues 同源（综合稿不计入）；无 thread 的
	// section 为已知 0（必序列化，不加 omitempty），不是文章总数。
	ClueCount int `json:"clue_count"`
	// ConfirmedKeyEvents lists the confirmed key-event markers under this
	// section (★ read-only projection; add-lane-compose-topic-event-tags).
	// Always serialized as an array — empty is [] (no omitempty): "no marker"
	// is a known state, not unknown. Never participates in selection,
	// centroid, threshold or candidate-chain eligibility.
	ConfirmedKeyEvents []ComposeConfirmedKeyEvent `json:"confirmed_key_events"`
}

// ComposeConfirmedKeyEvent is one confirmed key-event marker under a compose
// candidate section, projected read-only for the canvas ★ marker. Title is the
// server-frozen snapshot (client never rewrites it); ThreadID/ConfirmedAt are
// nullable because a rebuild can detach the source thread.
type ComposeConfirmedKeyEvent struct {
	MarkerID    uint       `json:"marker_id"`
	ThreadID    *uint      `json:"thread_id,omitempty"`
	Title       string     `json:"title"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

// ComposeCandidatesQuery describes the window of the compose-candidates
// endpoint. From/To are an inclusive YYYY-MM-DD range; when both are set they
// override Days (the handler rejects a half-specified range up front).
type ComposeCandidatesQuery struct {
	BoardID uint
	// Days is the shortcut window length, anchored to the
	// board's latest completed report. days<=0 means "approximately all
	// history", days over ComposeWindowMaxDays is capped at that bound
	// (unify-lane-workbench OQ-2: 90 → 365).
	Days int
	// From/To are the explicit window bounds (UTC midnight). Both are set
	// together or both are nil.
	From *time.Time
	To   *time.Time
	// AllMembersTopicID opts into the 「全量成员」 mode of the lane
	// correction workbench: when set, the members of that topic whose report
	// period precedes the window's first day are returned in EarlyMembers
	// (the in-window members stay in Sections). A nil pointer keeps the
	// compose workbench behaviour untouched — the response is byte-identical
	// and carries no early_members field at all.
	AllMembersTopicID *uint
	// OmitEmbeddings drops the per-section embedding vectors from the
	// response (unify-lane-workbench OQ-2：曲线走轻量取数). The curve only
	// needs period_date + persistent_topic_id + clue_count; skipping the
	// 10,244 B-per-row vectors shrinks a 365-day response from ≈10 MB to the
	// ≈200 KB range. The compose canvas path leaves it unset.
	OmitEmbeddings bool
}

// ComposeWindow echoes the resolved window back to the client so the frontend
// can label the canvas without recomputing the server-side window.
type ComposeWindow struct {
	// From/To are "YYYY-MM-DD" strings. A nil bound means "open" — no
	// completed report yet (nothing to anchor on) or all-history mode.
	From *string `json:"from"`
	To   *string `json:"to"`
	// Days is the effective window length in days (inclusive, both ends
	// included); 0 means "all history".
	Days int `json:"days"`
	// LatestCompletedDate is the board's latest completed report day, or
	// null when the board has no completed report yet.
	LatestCompletedDate *string `json:"latest_completed_date"`
}

// ComposeEarlyMember is one member of a persistent topic whose report period
// precedes the compose window's first day — the 「更早成员」 of the lane
// correction workbench (数据全量、展示分层).
//
// The shape mirrors ComposeCandidateSection (same fact columns, no vector:
// the early members are not drawn on the canvas, so their embedding is not
// shipped).
type ComposeEarlyMember struct {
	ID                   uint      `json:"id"`
	ReportID             uint      `json:"report_id"`
	PeriodDate           time.Time `json:"period_date"`
	ClusterLabel         string    `json:"cluster_label"`
	PersistentTopicID    *uint     `json:"persistent_topic_id,omitempty"`
	TopicMatchDistance   *float64  `json:"topic_match_distance,omitempty"`
	TopicMatchConfidence string    `json:"topic_match_confidence,omitempty"`
	// Embedding 是该 section 的内容向量：修正工作面的关联推荐质心需要窗口外成员
	// 参与（否则窗口内成员为 0 时质心为空、候选区全空）。无可用向量的成员不入集
	// （与窗口内 sections 同一向量可用性判据，fix-lane-metric-provenance D9）。
	Embedding []float64 `json:"embedding,omitempty"`
}

// ComposeCandidatesResponse is the payload for GET
// /api/semantic-boards/:id/persistent-topics/compose-candidates.
type ComposeCandidatesResponse struct {
	Sections []ComposeCandidateSection `json:"sections"`
	// Relations are the original section relations whose two endpoints are
	// both visible in the window (identity + similarity, verbatim).
	Relations []ComposeRelation `json:"relations"`
	// CandidateChains is the server-side single source of truth for the
	// candidate chain semantics (see BuildComposeCandidateChains).
	CandidateChains []ComposeCandidateChain `json:"candidate_chains"`
	MatchThreshold  float64                 `json:"match_threshold"`
	Window          ComposeWindow           `json:"window"`
	// EarlyMembers are the topic's members whose report period is earlier
	// than the window's first day (ascending by period date, then id).
	//
	// A nil pointer means the request did not ask for the full member set —
	// the field is then absent from the response (the compose workbench
	// behaviour is unchanged). A non-nil pointer always carries an array
	// (possibly empty), so the client can tell "no early members" apart from
	// "this build does not know the full member set".
	EarlyMembers *[]ComposeEarlyMember `json:"early_members,omitempty"`
}

// LatestCompletedReportDate returns the period_date of the board's latest
// completed daily report. It returns (nil, nil) when the board has no
// completed report yet — the compose window then has no anchor and the
// endpoint answers with empty collections instead of a 404.
func (r *TopicGraphRepository) LatestCompletedReportDate(boardID uint) (*time.Time, error) {
	var rep BoardDailyReport
	err := r.db.Where("semantic_board_id = ? AND status = 'completed'", boardID).
		Order("period_date DESC").Select("period_date").First(&rep).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest completed report date: %w", err)
	}
	day := time.Date(rep.PeriodDate.UTC().Year(), rep.PeriodDate.UTC().Month(), rep.PeriodDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return &day, nil
}

// resolveComposeWindow resolves the effective query window.
//
//   - An explicit from/to range wins over days (inclusive on both ends).
//   - The shortcut path keeps the legacy semantics: the window is anchored to
//     the board's latest completed report and spans days−1 days before it
//     (so days=30 covers 30 report days, both ends included). days<=0 keeps
//     the legacy "approximately all history" behaviour — both bounds open
//     (from/to reported as null), days>90 is capped at 90 (section-timeline
//     window contract).
//
// The bounds are computed in Go so the query stays portable across SQLite
// (tests) and Postgres (prod), avoiding a PG-only INTERVAL literal.
func resolveComposeWindow(q ComposeCandidatesQuery, latest *time.Time) (cutoff, upper time.Time, from, to *time.Time, effectiveDays int) {
	if q.From != nil && q.To != nil {
		from, to = q.From, q.To
		cutoff, upper = *q.From, *q.To
		// Inclusive span in days.
		effectiveDays = int(q.To.Sub(*q.From)/(24*time.Hour)) + 1
		return cutoff, upper, from, to, effectiveDays
	}

	effectiveDays = q.Days
	if effectiveDays > ComposeWindowMaxDays {
		effectiveDays = ComposeWindowMaxDays
	}
	if effectiveDays < 0 {
		effectiveDays = 0
	}
	if latest == nil {
		// No completed report: the window is empty, but keep the query
		// harmless (lower bound far in the past, upper bound far ahead).
		return time.Time{}.AddDate(0, 0, -99999), time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), nil, nil, effectiveDays
	}
	upper = *latest
	if effectiveDays > 0 {
		// Shortcut window: bounded on both ends — the latest completed
		// report minus (days−1) days … the latest completed report.
		start := latest.AddDate(0, 0, -(effectiveDays - 1))
		from = &start
		to = latest
		cutoff = start
	} else {
		// All-history mode: no day count, both bounds open — from/to stay
		// nil and the window is reported as unbounded (contract §1.1). The
		// query still stops at the latest completed report (no completed
		// report can be later), so the returned data set is unchanged.
		cutoff = latest.AddDate(0, 0, -99999)
	}
	return cutoff, upper, from, to, effectiveDays
}

// GetComposeCandidates loads the sections in the time window that carry a
// usable embedding, exposing the vector (parsed from the pgvector column) for
// the compose UI, plus the window relations and the server-side candidate
// chains of the compose workbench.
//
// The window is either the explicit from/to range (inclusive) or the
// days-based shortcut anchored to the board's latest completed report.
func (r *TopicGraphRepository) GetComposeCandidates(q ComposeCandidatesQuery) (ComposeCandidatesResponse, error) {
	latest, err := r.LatestCompletedReportDate(q.BoardID)
	if err != nil {
		return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: %w", err)
	}
	cutoff, upper, windowFrom, windowTo, effectiveDays := resolveComposeWindow(q, latest)

	type secRow struct {
		ID                   uint
		ReportID             uint
		PeriodDate           time.Time
		ClusterLabel         string
		Embedding            string
		PersistentTopicID    *uint
		TopicMatchConfidence string
	}
	var rows []secRow
	if err := r.db.Raw(`
		SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label, ds.embedding,
		       ds.persistent_topic_id, ds.topic_match_confidence
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE bdr.semantic_board_id = ?
		  AND bdr.status = 'completed'
		  AND bdr.period_date >= ?
		  AND bdr.period_date <= ?
		  AND ds.embedding IS NOT NULL
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, q.BoardID, cutoff, upper).Scan(&rows).Error; err != nil {
		return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: query sections: %w", err)
	}

	out := make([]ComposeCandidateSection, 0, len(rows))
	topicIDs := make(map[uint]bool)
	for _, rw := range rows {
		vec, err := repoParsePgVector(rw.Embedding)
		if err != nil || len(vec) == 0 {
			continue
		}
		if rw.PersistentTopicID != nil {
			topicIDs[*rw.PersistentTopicID] = true
		}
		out = append(out, ComposeCandidateSection{
			ID:                   rw.ID,
			ReportID:             rw.ReportID,
			PeriodDate:           rw.PeriodDate,
			ClusterLabel:         rw.ClusterLabel,
			Embedding:            vec,
			PersistentTopicID:    rw.PersistentTopicID,
			TopicMatchConfidence: rw.TopicMatchConfidence,
		})
	}

	// 每条 section 的非综合稿线索数（unify-lane-workbench D2）：对已筛出的有效
	// section 一次批量复用 listLaneSectionThreads（Clues 排除 composite）；无
	// thread 的 section 为已知 0（零值即 0，必序列化）；查询失败使请求失败，
	// 不伪造 0。
	if len(out) > 0 {
		ids := make([]uint, len(out))
		for i := range out {
			ids[i] = out[i].ID
		}
		threadsBySection, err := r.listLaneSectionThreads(ids)
		if err != nil {
			return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: section threads: %w", err)
		}
		for i := range out {
			out[i].ClueCount = threadsBySection[out[i].ID].Clues
		}

		// ★ 只读投影：该 section 下被人工确认的关键线索（add-lane-compose-
		// topic-event-tags）。一次批量查、按 section 分组；无标记的 section
		// 恒为 []（不省略、不给 null），前端据此决定是否画星形。
		keyEvents, err := r.ListConfirmedKeyEventsBySections(ids)
		if err != nil {
			return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: confirmed key events: %w", err)
		}
		for i := range out {
			markers := keyEvents[out[i].ID]
			list := make([]ComposeConfirmedKeyEvent, 0, len(markers))
			for _, marker := range markers {
				list = append(list, ComposeConfirmedKeyEvent{
					MarkerID:    marker.ID,
					ThreadID:    marker.SourceThreadID,
					Title:       marker.TitleSnapshot,
					ConfirmedAt: marker.ConfirmedAt,
				})
			}
			out[i].ConfirmedKeyEvents = list
		}
	}

	// 轻量取数（OQ-2）：曲线只需要日期/归属/线索数，每条向量实占 10,244 B——
	// 365 天含向量 ≈10 MB，省略后压到 ≈200 KB 量级。画布路径不传该参数。
	if q.OmitEmbeddings {
		for i := range out {
			out[i].Embedding = nil
		}
	}

	// Attach topic briefs (id/label/status/colour) in one batch, mirroring
	// attachTopicBriefs but against the compose candidate slice shape.
	if len(topicIDs) > 0 {
		ids := make([]uint, 0, len(topicIDs))
		for id := range topicIDs {
			ids = append(ids, id)
		}
		briefByID := loadTopicBriefMap(r.db, ids)
		for i := range out {
			if out[i].PersistentTopicID == nil {
				continue
			}
			if brief, ok := briefByID[*out[i].PersistentTopicID]; ok {
				out[i].PersistentTopic = &brief
			}
		}
	}

	// Relations of the window: the original edges whose two endpoints are
	// both visible (i.e. both endpoints are in the returned section set).
	// Identity and similarity edges are kept verbatim — no edge is invented.
	relations := make([]ComposeRelation, 0)
	if len(out) > 0 {
		ids := make([]uint, len(out))
		visible := make(map[uint]bool, len(out))
		for i, s := range out {
			ids[i] = s.ID
			visible[s.ID] = true
		}
		var rels []ComposeRelation
		if err := r.db.Raw(`
			SELECT from_section_id AS from_id, to_section_id AS to_id, distance, relation_type
			FROM daily_report_section_relations
			WHERE from_section_id IN ? OR to_section_id IN ?
		`, ids, ids).Scan(&rels).Error; err != nil {
			return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: query relations: %w", err)
		}
		for _, rel := range rels {
			if visible[rel.FromID] && visible[rel.ToID] {
				relations = append(relations, rel)
			}
		}
	}

	// Candidate chains: server-side single source of truth (contract §2.1).
	chainSections := make([]ComposeChainSection, 0, len(out))
	for _, s := range out {
		cs := ComposeChainSection{
			ID:                s.ID,
			PeriodDate:        s.PeriodDate,
			PersistentTopicID: s.PersistentTopicID,
		}
		if s.PersistentTopic != nil {
			cs.TopicLabel = s.PersistentTopic.Label
			cs.TopicStatus = s.PersistentTopic.Status
		}
		chainSections = append(chainSections, cs)
	}
	chains := BuildComposeCandidateChains(chainSections, relations)

	resp := ComposeCandidatesResponse{
		Sections:        out,
		Relations:       relations,
		CandidateChains: chains,
		MatchThreshold:  LoadPersistentTopicConfig(r.db).MatchThreshold,
		Window: ComposeWindow{
			From:                formatComposeDate(windowFrom),
			To:                  formatComposeDate(windowTo),
			Days:                effectiveDays,
			LatestCompletedDate: formatComposeDate(latest),
		},
	}

	// 「全量成员」 mode (lane correction workbench): the topic's members
	// whose report period precedes the window's first day. Only loaded when
	// the caller asks for it — the compose workbench response stays
	// byte-identical.
	if q.AllMembersTopicID != nil {
		// 窗口外成员 = 窗口 [cutoff, upper] 之外的**全部**本话题成员。显式 from/to 且
		// to < 最新 completed 日时，上界之后还有成员——不取它们会让「窗口 N + 窗口外数」
		// 少于成员总数（review R-3）。
		early, err := r.getComposeEarlyMembers(q.BoardID, *q.AllMembersTopicID, cutoff, upper)
		if err != nil {
			return ComposeCandidatesResponse{}, fmt.Errorf("GetComposeCandidates: query early members: %w", err)
		}
		resp.EarlyMembers = &early
	}

	return resp, nil
}

// getComposeEarlyMembers loads the members of one persistent topic that fall
// OUTSIDE the compose window [cutoff, upper] (window bounds inclusive, same as
// the sections query).
//
// 两个集合互补（fix-lane-metric-provenance D9 / review R-3）：`sections` 取窗口内、
// 本函数取窗口外（早于 from 的「更早成员」，以及显式 from/to 且 `to < 最新 completed 日`
// 时晚于 to 的成员）；两侧同判据（都只算 completed 报告 + 向量可用），因此
// 「窗口 N + 窗口外数 = 全部 M」恒成立、窗口内集合恒是全部集合的真子集。
// 只取 `period_date < cutoff` 会让上界之后的成员两侧都不出现，M 就不等于成员总数了。
//
// Ordering: period date ascending, then section id ascending.
func (r *TopicGraphRepository) getComposeEarlyMembers(boardID, topicID uint, cutoff, upper time.Time) ([]ComposeEarlyMember, error) {
	out := make([]ComposeEarlyMember, 0)
	var rows []struct {
		ID                   uint
		ReportID             uint
		PeriodDate           time.Time
		ClusterLabel         string
		PersistentTopicID    *uint
		TopicMatchDistance   *float64
		TopicMatchConfidence string
		Embedding            string
	}
	if err := r.db.Raw(`
		SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
		       ds.persistent_topic_id, ds.topic_match_distance, ds.topic_match_confidence,
		       ds.embedding
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE bdr.semantic_board_id = ?
		  AND bdr.status = 'completed'
		  AND ds.persistent_topic_id = ?
		  AND (bdr.period_date < ? OR bdr.period_date > ?)
		  AND ds.embedding IS NOT NULL
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, boardID, topicID, cutoff, upper).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("get compose early members: %w", err)
	}
	for _, rw := range rows {
		vec, err := repoParsePgVector(rw.Embedding)
		if err != nil || len(vec) == 0 {
			// 无可用向量 → 不入集（与 sections 侧同一判据）。
			continue
		}
		out = append(out, ComposeEarlyMember{
			ID:                   rw.ID,
			ReportID:             rw.ReportID,
			PeriodDate:           rw.PeriodDate,
			ClusterLabel:         rw.ClusterLabel,
			PersistentTopicID:    rw.PersistentTopicID,
			TopicMatchDistance:   rw.TopicMatchDistance,
			TopicMatchConfidence: rw.TopicMatchConfidence,
			Embedding:            vec,
		})
	}
	return out, nil
}

// PersistentTopicBoard returns the board a persistent topic belongs to.
// ok is false when the topic does not exist.
func (r *TopicGraphRepository) PersistentTopicBoard(topicID uint) (boardID uint, ok bool, err error) {
	var topic BoardPersistentTopic
	if err := r.db.Select("id", "semantic_board_id").First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("persistent topic board: %w", err)
	}
	return topic.SemanticBoardID, true, nil
}

// formatComposeDate renders a day-granular bound as "YYYY-MM-DD" (UTC).
// A nil bound stays nil so the JSON carries an explicit null.
func formatComposeDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}

// CreateManualTopic creates a new persistent topic with source=manual and
// status=active, reassigning the given sections to it. The topic embedding is
// the mean pooling of the selected sections' embeddings. Sections without valid
// embeddings are skipped and returned. The entire operation runs in a single
// transaction: any step that fails (including RebuildBoardRelations) rolls back
// the whole thing, leaving no half-created topic.
func (r *TopicGraphRepository) CreateManualTopic(boardID uint, label string, sectionIDs []uint) (topic *BoardPersistentTopic, skipped []uint, err error) {
	if len(sectionIDs) == 0 {
		return nil, nil, fmt.Errorf("CreateManualTopic: no sections provided")
	}

	// Load sections with their embedding and period_date.
	type secRow struct {
		ID        uint
		Embedding string
		LaneTier  string
		Day       time.Time
	}
	var rows []secRow
	if err := r.db.Raw(`
		SELECT s.id, s.embedding, s.lane_tier, r.period_date AS day
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		WHERE s.id IN ?
		ORDER BY r.period_date ASC
	`, sectionIDs).Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("CreateManualTopic: load sections: %w", err)
	}

	// Parse embeddings and collect date range.
	type usable struct {
		id       uint
		vec      []float64
		laneTier string
		day      time.Time
	}
	var usables []usable
	for _, rw := range rows {
		v, err := repoParsePgVector(rw.Embedding)
		if err != nil || len(v) == 0 {
			skipped = append(skipped, rw.ID)
			continue
		}
		// Validate dimension against first usable.
		if len(usables) > 0 && len(v) != len(usables[0].vec) {
			skipped = append(skipped, rw.ID)
			continue
		}
		usables = append(usables, usable{id: rw.ID, vec: v, laneTier: rw.LaneTier, day: rw.Day})
	}
	if len(usables) == 0 {
		return nil, skipped, fmt.Errorf("CreateManualTopic: no section with a usable vector")
	}

	// Aggregate mean embedding.
	vecs := make([][]float64, len(usables))
	for i, u := range usables {
		vecs[i] = u.vec
	}
	mean, _ := aggregateEmbeddings(vecs)

	// Date range.
	firstSeen := usables[0].day
	lastSeen := usables[0].day
	for _, u := range usables {
		if u.day.Before(firstSeen) {
			firstSeen = u.day
		}
		if u.day.After(lastSeen) {
			lastSeen = u.day
		}
	}

	// Execute in transaction. `created` is declared outside the closure so we can
	// return it directly — GORM backfills its ID during CreateTopic, so no reload
	// (a reload by label+source would be fragile when a board has multiple manual
	// topics sharing a label).
	var created BoardPersistentTopic
	err = r.db.Transaction(func(tx *gorm.DB) error {
		created = BoardPersistentTopic{
			SemanticBoardID: boardID,
			Label:           label,
			Embedding:       FloatsToPgVector(mean),
			Status:          TopicStatusActive,
			Source:          TopicSourceManual,
			FirstSeenDate:   firstSeen,
			LastSeenDate:    lastSeen,
			HitCount:        len(usables),
			ConsecutiveHits: len(usables),
		}
		if err := r.CreateTopic(tx, &created); err != nil {
			return fmt.Errorf("CreateManualTopic: %w", err)
		}

		// Assign each usable section to the new topic.
		for _, u := range usables {
			dist := cosineDistance(u.vec, mean)
			tid := created.ID
			if err := r.UpdateSectionTopicAssignment(tx, u.id, &tid, dist, TopicConfManual, u.laneTier, nil); err != nil {
				return fmt.Errorf("CreateManualTopic: assign section %d: %w", u.id, err)
			}
		}

		// Rebuild relations — failure here MUST roll back the entire transaction.
		if err := RebuildBoardRelations(tx, boardID); err != nil {
			return fmt.Errorf("CreateManualTopic: rebuild relations: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, skipped, err
	}
	return &created, skipped, nil
}
