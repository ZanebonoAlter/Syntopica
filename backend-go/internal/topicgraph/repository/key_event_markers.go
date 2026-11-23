package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
)

// ── 话题关键事件注记（add-topic-key-event-markers design D1/D2）──────────────
//
// 一个注记 = 一条 thread 的一次进展的人工选择记录。仅在有建议或用户动作时
// 建行，无建议的普通 thread 不提前占行。来源指纹（source_key）判定"完全相同
// 的来源"，不是事件语义 ID；不靠标题相似度移植确认。重建（SaveReport 同日
// 覆盖）只允许按完全一致指纹重附指针，其余保留为来源替换快照（thread/section
// 指针置 NULL，快照正文不删）。
//
// 状态机（state，DB CHECK 强制）：
//
//	suggested → (人工确认 mark) → confirmed
//	suggested → (不采纳 dismiss) → dismissed
//	confirmed → (取消 unmark)    → dismissed
//	dismissed → (重新手动标记 mark，需来源仍有效) → confirmed
//
// AI 只允许写 suggested / 更新尚未处理的同来源建议（SaveSuggestedMarkers），
// 任何 confirmed/dismissed 状态只能由人工动作产生。

// Marker state values (DB CHECK enforces the enum).
const (
	KeyEventStateSuggested = "suggested"
	KeyEventStateConfirmed = "confirmed"
	KeyEventStateDismissed = "dismissed"
)

// LastAction values (audit only; not a state machine).
const (
	KeyEventActionMark      = "mark"
	KeyEventActionDismiss   = "dismiss"
	KeyEventActionUnmark    = "unmark"
	KeyEventActionAISuggest = "ai_suggest"
)

// TopicKeyEventMarker — one human-curated key-event annotation on a thread.
// Snapshots freeze the source content at decision time; the server derives
// them from the thread (never client-supplied text — 作用域与来源保护).
type TopicKeyEventMarker struct {
	ID                uint   `gorm:"primarykey" json:"id"`
	SemanticBoardID   uint   `gorm:"not null;uniqueIndex:idx_topic_key_events_board_source,priority:1" json:"semantic_board_id"`
	SourceKey         string `gorm:"size:64;not null;uniqueIndex:idx_topic_key_events_board_source,priority:2" json:"source_key"`
	PersistentTopicID *uint  `gorm:"index:idx_topic_key_events_topic_state,priority:1" json:"persistent_topic_id,omitempty"`
	// Source pointers are nullable: rebuilds delete threads/sections and the
	// FK ON DELETE SET NULL keeps the user's confirmed snapshot (design D1:
	// 重建先断开来源引用，确认快照不删).
	SourceReportID   *uint     `gorm:"index" json:"source_report_id,omitempty"`
	SourceSectionID  *uint     `json:"source_section_id,omitempty"`
	SourceThreadID   *uint     `json:"source_thread_id,omitempty"`
	SourcePeriodDate time.Time `gorm:"type:date;not null;index:idx_topic_key_events_topic_state,priority:3" json:"source_period_date"`
	// Frozen snapshots (server-derived; confirmed rows are never rewritten by AI).
	TopicLabelSnapshot string `gorm:"size:200" json:"topic_label_snapshot"`
	TitleSnapshot      string `gorm:"type:text" json:"title_snapshot"`
	SummarySnapshot    string `gorm:"type:text" json:"summary_snapshot"`
	TagIDsSnapshot     JSON   `gorm:"type:jsonb" json:"tag_ids_snapshot"`
	ArticleIDsSnapshot JSON   `gorm:"type:jsonb" json:"article_ids_snapshot"`
	State              string `gorm:"size:16;not null;default:suggested;index:idx_topic_key_events_topic_state,priority:2" json:"state"`
	// RecommendationReason/Basis: validated AI output only; empty for manual
	// marks. Basis shape: {"evidence_article_ids":[...],"new_progress":bool}.
	RecommendationReason string     `gorm:"type:text" json:"recommendation_reason,omitempty"`
	RecommendationBasis  JSON       `gorm:"type:jsonb" json:"recommendation_basis,omitempty"`
	LastAction           string     `gorm:"size:16" json:"last_action,omitempty"`
	ConfirmedAt          *time.Time `json:"confirmed_at,omitempty"`
	DecidedAt            *time.Time `json:"decided_at,omitempty"`
	Revision             int        `gorm:"not null;default:1" json:"revision"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (TopicKeyEventMarker) TableName() string {
	return "topic_key_event_markers"
}

// KeyEventRecommendationRun — per-report recommendation task state (design D3).
// One row per report; a rebuild overwrites state/token so a late task's save
// can detect staleness (token mismatch → discard the whole batch).
type KeyEventRecommendationRun struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	ReportID        uint      `gorm:"not null;uniqueIndex:idx_key_event_runs_report" json:"report_id"`
	SemanticBoardID uint      `gorm:"not null;index" json:"semantic_board_id"`
	State           string    `gorm:"size:16;not null;default:running" json:"state"` // running|done|failed
	GenerationToken string    `gorm:"size:64;not null" json:"generation_token"`
	CandidateCount  int       `json:"candidate_count"`
	SuggestedCount  int       `json:"suggested_count"`
	Error           string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (KeyEventRecommendationRun) TableName() string {
	return "key_event_recommendation_runs"
}

// KeyEventRunState values.
const (
	KeyEventRunStateRunning = "running"
	KeyEventRunStateDone    = "done"
	KeyEventRunStateFailed  = "failed"
)

// ── 来源指纹（design D1）────────────────────────────────────────────────────

// normalizeKeyEventTitle collapses whitespace runs (Unicode whitespace incl.
// full-width space/tab/newline) to a single ASCII space and trims. Case and
// emoji are preserved on purpose — the fingerprint must not silently merge
// different titles (test-cases S3: 指纹仅归一空白).
func normalizeKeyEventTitle(title string) string {
	var sb strings.Builder
	inSpace := false
	for _, r := range title {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x3000 {
			inSpace = true
			continue
		}
		if inSpace && sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		inSpace = false
		sb.WriteRune(r)
	}
	return sb.String()
}

// sortedUniqueUintIDs returns a sorted, de-duplicated copy of ids.
func sortedUniqueUintIDs(ids []uint) []uint {
	out := append([]uint(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	dedup := out[:0]
	var last uint
	for i, v := range out {
		if i == 0 || v != last {
			dedup = append(dedup, v)
		}
		last = v
	}
	return dedup
}

// jsonUintIDs parses a JSON array of numbers ([]uint as JSON); nil/empty on
// any error (fingerprint treats unreadable evidence as absent, never fails).
func jsonUintIDs(raw JSON) []uint {
	if len(raw) == 0 {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// ComputeKeyEventSourceKey derives the deterministic source fingerprint
// (design D1): SHA-256 over board|topic|来源日报日期(上海)|规范化标题|排序去重
// tag_ids|排序去重 article_ids. Same content ⇒ same key; ANY drift in title,
// evidence, date or owner ⇒ different key (no similarity transfer).
func ComputeKeyEventSourceKey(boardID uint, topicID uint, periodDate time.Time, title string, tagIDs, articleIDs []uint) string {
	dateStr := periodDate.In(models.ShanghaiTZ).Format("2006-01-02")
	parts := []string{
		fmt.Sprintf("b=%d", boardID),
		fmt.Sprintf("t=%d", topicID),
		fmt.Sprintf("d=%s", dateStr),
		fmt.Sprintf("title=%s", normalizeKeyEventTitle(title)),
		fmt.Sprintf("tags=%v", sortedUniqueUintIDs(tagIDs)),
		fmt.Sprintf("arts=%v", sortedUniqueUintIDs(articleIDs)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// ThreadKeyEventSourceKey computes the fingerprint of a thread within its
// owning section (topic owner resolved by the caller).
func ThreadKeyEventSourceKey(boardID, topicID uint, periodDate time.Time, thread DailyReportThread) string {
	return ComputeKeyEventSourceKey(boardID, topicID, periodDate, thread.Title,
		jsonUintIDs(thread.TagIDs), jsonUintIDs(thread.RelatedArticleIDs))
}

// ── 错误类型（handler 映射 400/404/409）────────────────────────────────────

// KeyEventActionError carries the HTTP semantics for action failures so the
// handler can map them without parsing strings. Marker carries the latest
// server state for 409 revision conflicts (client re-reads and retries).
type KeyEventActionError struct {
	HTTPStatus int
	Message    string
	Reason     string
	Marker     *TopicKeyEventMarker
}

func (e *KeyEventActionError) Error() string { return e.Message }

func keyEventBadRequest(format string, args ...any) *KeyEventActionError {
	return &KeyEventActionError{HTTPStatus: 400, Reason: "invalid_action", Message: fmt.Sprintf(format, args...)}
}

func keyEventNotFound(format string, args ...any) *KeyEventActionError {
	return &KeyEventActionError{HTTPStatus: 404, Reason: "not_found", Message: fmt.Sprintf(format, args...)}
}

func keyEventConflict(marker *TopicKeyEventMarker, format string, args ...any) *KeyEventActionError {
	return &KeyEventActionError{HTTPStatus: 409, Reason: "revision_conflict", Message: fmt.Sprintf(format, args...), Marker: marker}
}

// ── 人工动作（design D2）────────────────────────────────────────────────────

// keyEventTargetState maps an action to the resulting marker state.
func keyEventTargetState(action string) (string, bool) {
	switch action {
	case KeyEventActionMark:
		return KeyEventStateConfirmed, true
	case KeyEventActionDismiss, KeyEventActionUnmark:
		return KeyEventStateDismissed, true
	}
	return "", false
}

// ApplyThreadKeyEvent executes a thread-addressed manual action
// (PUT /api/daily-reports/topics/:id/key-events/threads/:thread_id).
// The marker row is resolved by the server-derived source fingerprint; the
// client never supplies snapshot text. Idempotent for same-target repeats;
// state changes on an existing row require the latest revision (else 409
// with the current marker).
func (r *TopicGraphRepository) ApplyThreadKeyEvent(topicID, threadID uint, action string, expectedRevision *int) (*TopicKeyEventMarker, error) {
	// Thread-addressed actions are ONLY mark|dismiss (design D2)；unmark 是
	// marker-addressed 纠错动作，不能凭 thread 端点绕过来源校验。
	if action != KeyEventActionMark && action != KeyEventActionDismiss {
		return nil, keyEventBadRequest("action 必须是 mark|dismiss，收到 %q", action)
	}
	targetState, ok := keyEventTargetState(action)
	if !ok {
		return nil, keyEventBadRequest("action 必须是 mark|dismiss，收到 %q", action)
	}

	var result *TopicKeyEventMarker
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var topic BoardPersistentTopic
		if err := tx.First(&topic, topicID).Error; err != nil {
			return keyEventNotFound("话题 %d 不存在", topicID)
		}
		var thread DailyReportThread
		if err := tx.First(&thread, threadID).Error; err != nil {
			return keyEventNotFound("线索 %d 不存在", threadID)
		}
		var section DailyReportSection
		if err := tx.First(&section, thread.SectionID).Error; err != nil {
			return keyEventNotFound("线索 %d 所属章节不存在", threadID)
		}
		if section.PersistentTopicID == nil {
			// 无归属新标记：thread 所在 section 没有持久话题归属（spec 400）。
			return keyEventBadRequest("线索 %d 无有效话题归属，不能标记", threadID)
		}
		if *section.PersistentTopicID != topicID {
			return keyEventNotFound("线索 %d 不属于话题 %d", threadID, topicID)
		}
		var report BoardDailyReport
		if err := tx.First(&report, thread.ReportID).Error; err != nil {
			return keyEventNotFound("线索 %d 所属日报不存在", threadID)
		}
		if report.SemanticBoardID != topic.SemanticBoardID {
			return keyEventNotFound("线索 %d 不属于话题 %d 所在版块", threadID, topicID)
		}

		boardID := topic.SemanticBoardID
		periodDate := NormalizeReportDate(report.PeriodDate)
		sourceKey := ThreadKeyEventSourceKey(boardID, topicID, periodDate, thread)

		// Lock the row when it exists (来源单行；与 AI 保存/并发动作串行).
		var marker TopicKeyEventMarker
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("semantic_board_id = ? AND source_key = ?", boardID, sourceKey).
			First(&marker).Error

		now := time.Now()
		if err == gorm.ErrRecordNotFound {
			created := TopicKeyEventMarker{
				SemanticBoardID:    boardID,
				PersistentTopicID:  &topicID,
				SourceKey:          sourceKey,
				SourceReportID:     &report.ID,
				SourceSectionID:    &section.ID,
				SourceThreadID:     &thread.ID,
				SourcePeriodDate:   periodDate,
				TopicLabelSnapshot: topic.Label,
				TitleSnapshot:      thread.Title,
				SummarySnapshot:    thread.Summary,
				TagIDsSnapshot:     thread.TagIDs,
				ArticleIDsSnapshot: thread.RelatedArticleIDs,
				State:              targetState,
				LastAction:         action,
				Revision:           1,
			}
			if targetState == KeyEventStateConfirmed {
				created.ConfirmedAt = &now
			}
			created.DecidedAt = &now
			// Concurrent first-writers race on the unique index; the loser
			// re-reads the winner's row and continues through the same logic.
			if err := tx.Create(&created).Error; err != nil {
				var again TopicKeyEventMarker
				if reErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("semantic_board_id = ? AND source_key = ?", boardID, sourceKey).
					First(&again).Error; reErr != nil {
					return fmt.Errorf("create key event marker: %w", err)
				}
				marker = again
			} else {
				result = &created
				return nil
			}
		} else if err != nil {
			return fmt.Errorf("load key event marker: %w", err)
		}

		if marker.State == targetState {
			// Idempotent repeat: return the saved final state unchanged.
			result = &marker
			return nil
		}
		if expectedRevision == nil || *expectedRevision != marker.Revision {
			m := marker
			return keyEventConflict(&m, "revision 冲突：需要最新 revision=%d", marker.Revision)
		}
		// Re-anchor snapshots to the live server source (manual re-mark after
		// a same-fingerprint rebuild keeps the snapshot current; the client
		// still cannot supply text).
		updates := map[string]any{
			"state":                targetState,
			"last_action":          action,
			"revision":             marker.Revision + 1,
			"decided_at":           now,
			"persistent_topic_id":  topicID,
			"source_report_id":     report.ID,
			"source_section_id":    section.ID,
			"source_thread_id":     thread.ID,
			"source_period_date":   periodDate,
			"topic_label_snapshot": topic.Label,
			"title_snapshot":       thread.Title,
			"summary_snapshot":     thread.Summary,
			"tag_ids_snapshot":     thread.TagIDs,
			"article_ids_snapshot": thread.RelatedArticleIDs,
		}
		if targetState == KeyEventStateConfirmed {
			updates["confirmed_at"] = now
		}
		if err := tx.Model(&TopicKeyEventMarker{}).Where("id = ?", marker.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update key event marker: %w", err)
		}
		if err := tx.First(&marker, marker.ID).Error; err != nil {
			return fmt.Errorf("reload key event marker: %w", err)
		}
		result = &marker
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ApplyMarkerKeyEvent executes a marker-addressed action
// (PATCH /api/daily-reports/topics/:id/key-events/:marker_id).
// unmark works on source-invalid confirmed rows (纠错维护); mark on a
// detached snapshot is rejected — the old source cannot legitimize a NEW
// confirmation (design D2: 不能凭旧来源新建/重新 mark).
func (r *TopicGraphRepository) ApplyMarkerKeyEvent(topicID, markerID uint, action string, expectedRevision *int) (*TopicKeyEventMarker, error) {
	if expectedRevision == nil {
		return nil, keyEventBadRequest("PATCH 需要 revision")
	}
	targetState, ok := keyEventTargetState(action)
	if !ok {
		return nil, keyEventBadRequest("action 必须是 mark|dismiss|unmark，收到 %q", action)
	}

	var result *TopicKeyEventMarker
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var topic BoardPersistentTopic
		if err := tx.First(&topic, topicID).Error; err != nil {
			return keyEventNotFound("话题 %d 不存在", topicID)
		}
		var marker TopicKeyEventMarker
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&marker, markerID).Error; err != nil {
			return keyEventNotFound("注记 %d 不存在", markerID)
		}
		if marker.PersistentTopicID == nil || *marker.PersistentTopicID != topicID {
			return keyEventNotFound("注记 %d 不属于话题 %d", markerID, topicID)
		}

		if action == KeyEventActionUnmark {
			// unmark semantics: cancel a confirmed node (纠错). Applying it to
			// an already-dismissed row is the same target state — idempotent.
			if marker.State != KeyEventStateConfirmed && marker.State != KeyEventStateDismissed {
				return keyEventBadRequest("unmark 只适用于 confirmed 注记")
			}
		}

		if marker.State == targetState {
			result = &marker
			return nil
		}
		if marker.State == KeyEventStateSuggested && action == KeyEventActionUnmark {
			return keyEventBadRequest("unmark 只适用于 confirmed 注记")
		}
		if *expectedRevision != marker.Revision {
			m := marker
			return keyEventConflict(&m, "revision 冲突：需要最新 revision=%d", marker.Revision)
		}

		if action == KeyEventActionMark && !keyEventSourceAlive(tx, &marker) {
			return keyEventBadRequest("来源已失效，不能凭旧来源重新标记；请到日报对新条目重新选择")
		}

		now := time.Now()
		updates := map[string]any{
			"state":       targetState,
			"last_action": action,
			"revision":    marker.Revision + 1,
			"decided_at":  now,
		}
		if targetState == KeyEventStateConfirmed {
			updates["confirmed_at"] = now
		}
		if err := tx.Model(&TopicKeyEventMarker{}).Where("id = ?", marker.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update key event marker: %w", err)
		}
		if err := tx.First(&marker, marker.ID).Error; err != nil {
			return fmt.Errorf("reload key event marker: %w", err)
		}
		result = &marker
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// keyEventSourceAlive reports whether the marker's source thread still exists
// and its section still belongs to the marker's topic. Detached snapshots
// (thread replaced by a rebuild) are NOT alive.
func keyEventSourceAlive(tx *gorm.DB, marker *TopicKeyEventMarker) bool {
	if marker.SourceThreadID == nil || marker.PersistentTopicID == nil {
		return false
	}
	var thread DailyReportThread
	if err := tx.First(&thread, *marker.SourceThreadID).Error; err != nil {
		return false
	}
	var section DailyReportSection
	if err := tx.First(&section, thread.SectionID).Error; err != nil {
		return false
	}
	return section.PersistentTopicID != nil && *section.PersistentTopicID == *marker.PersistentTopicID
}

// ── AI 建议写入（design D3：AI 只写 suggested）──────────────────────────────

// SuggestedKeyEvent is one validated AI recommendation to persist.
type SuggestedKeyEvent struct {
	SourceThreadID uint
	Reason         string
	NewProgress    bool
	EvidenceIDs    []uint
}

// markerBasis is the persisted recommendation_basis JSON shape.
type markerBasis struct {
	EvidenceArticleIDs []uint `json:"evidence_article_ids"`
	NewProgress        bool   `json:"new_progress"`
}

// SaveSuggestedMarkers inserts AI suggestions as suggested-state rows. Rows
// already existing for the same source fingerprint keep their state (late AI
// saves can NEVER touch confirmed/dismissed — 人工主权); an unprocessed
// suggested row from the same source may refresh its reason/basis.
func (r *TopicGraphRepository) SaveSuggestedMarkers(boardID uint, reportID uint, generationToken string, suggestions []SuggestedKeyEvent) (int, error) {
	if len(suggestions) == 0 {
		return 0, nil
	}
	// Token staleness check happens OUTSIDE the write tx first (cheap path),
	// and again inside via the run row lock below.
	var report BoardDailyReport
	if err := r.db.First(&report, reportID).Error; err != nil {
		return 0, fmt.Errorf("load report %d: %w", reportID, err)
	}
	periodDate := NormalizeReportDate(report.PeriodDate)

	saved := 0
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Re-verify the generation token under the run-row lock: a rebuild in
		// between replaced the token → the whole batch is stale, discard.
		var run KeyEventRecommendationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("report_id = ?", reportID).First(&run).Error; err != nil {
			return fmt.Errorf("load recommendation run for report %d: %w", reportID, err)
		}
		if run.GenerationToken != generationToken {
			return nil // stale batch — silently drop (design D3)
		}

		for _, s := range suggestions {
			var thread DailyReportThread
			if err := tx.First(&thread, s.SourceThreadID).Error; err != nil {
				continue // thread vanished mid-flight (rebuild) — drop item
			}
			var section DailyReportSection
			if err := tx.First(&section, thread.SectionID).Error; err != nil {
				continue
			}
			if section.PersistentTopicID == nil || section.ReportID != reportID {
				continue
			}
			topicID := *section.PersistentTopicID
			var topic BoardPersistentTopic
			if err := tx.First(&topic, topicID).Error; err != nil {
				continue
			}
			sourceKey := ThreadKeyEventSourceKey(boardID, topicID, periodDate, thread)

			basisBytes, _ := json.Marshal(markerBasis{EvidenceArticleIDs: s.EvidenceIDs, NewProgress: s.NewProgress})
			basis := JSON(basisBytes)

			var existing TopicKeyEventMarker
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("semantic_board_id = ? AND source_key = ?", boardID, sourceKey).
				First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				created := TopicKeyEventMarker{
					SemanticBoardID:      boardID,
					PersistentTopicID:    &topicID,
					SourceKey:            sourceKey,
					SourceReportID:       &reportID,
					SourceSectionID:      &section.ID,
					SourceThreadID:       &thread.ID,
					SourcePeriodDate:     periodDate,
					TopicLabelSnapshot:   topic.Label,
					TitleSnapshot:        thread.Title,
					SummarySnapshot:      thread.Summary,
					TagIDsSnapshot:       thread.TagIDs,
					ArticleIDsSnapshot:   thread.RelatedArticleIDs,
					State:                KeyEventStateSuggested,
					RecommendationReason: s.Reason,
					RecommendationBasis:  basis,
					LastAction:           KeyEventActionAISuggest,
					DecidedAt:            nil,
					Revision:             1,
				}
				if err := tx.Create(&created).Error; err != nil {
					// Unique race → another writer won; treat as not-saved.
					continue
				}
				saved++
			} else if err == nil && existing.State == KeyEventStateSuggested {
				// Refresh an unprocessed suggestion for the SAME source only.
				if err := tx.Model(&TopicKeyEventMarker{}).Where("id = ?", existing.ID).
					Updates(map[string]any{
						"recommendation_reason": s.Reason,
						"recommendation_basis":  basis,
						"source_report_id":      reportID,
						"source_section_id":     section.ID,
						"source_thread_id":      thread.ID,
					}).Error; err != nil {
					continue
				}
				saved++
			}
			// existing confirmed/dismissed → leave untouched (late AI never
			// overrides human decisions).
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return saved, nil
}

// ── 查询（design D2）────────────────────────────────────────────────────────

// ListReportKeyEventMarkers returns every marker whose source report is the
// given report (attached to live threads of the report, or detached snapshots
// left by a rebuild of this board+date). Reader maps them by source_thread_id.
func (r *TopicGraphRepository) ListReportKeyEventMarkers(reportID uint) ([]TopicKeyEventMarker, error) {
	var markers []TopicKeyEventMarker
	if err := r.db.Where("source_report_id = ?", reportID).
		Order("id ASC").Find(&markers).Error; err != nil {
		return nil, fmt.Errorf("list report key event markers: %w", err)
	}
	return markers, nil
}

// ListTopicKeyEventMarkers returns the topic's markers filtered by state and
// a rolling-days window on the SOURCE report date (days=0 → all time;
// days>0 → source_period_date ≥ today-days in the business timezone).
// Ordered source date DESC, id DESC (stable). The overview read path only
// ever passes state=confirmed (topic 读端默认/只允许 confirmed).
func (r *TopicGraphRepository) ListTopicKeyEventMarkers(topicID uint, days int, state string) ([]TopicKeyEventMarker, error) {
	q := r.db.Where("persistent_topic_id = ?", topicID)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if days > 0 {
		today := time.Now().In(models.ShanghaiTZ)
		lower := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, models.ShanghaiTZ).
			AddDate(0, 0, -(days - 1))
		q = q.Where("source_period_date >= ?", lower.Format("2006-01-02"))
	}
	var markers []TopicKeyEventMarker
	if err := q.Order("source_period_date DESC, id DESC").Find(&markers).Error; err != nil {
		return nil, fmt.Errorf("list topic key event markers: %w", err)
	}
	return markers, nil
}

// ListBoardKeyEventMarkers returns the board's CONFIRMED markers whose
// source_period_date falls in the closed [from, to] window (add-key-event-
// matrix-view design D3: 板块级批量读，供矩阵级关键事件视图一次取全).
// Suggested/dismissed never enter the payload; ordering is date ASC →
// topic ASC → id ASC so the frontend can aggregate by topic+date and keep
// same-day multi-event order straight from the wire.
func (r *TopicGraphRepository) ListBoardKeyEventMarkers(boardID uint, from, to time.Time) ([]TopicKeyEventMarker, error) {
	var markers []TopicKeyEventMarker
	if err := r.db.
		Where("semantic_board_id = ? AND state = ? AND source_period_date >= ? AND source_period_date <= ?",
			boardID, KeyEventStateConfirmed, from.Format("2006-01-02"), to.Format("2006-01-02")).
		Order("source_period_date ASC, persistent_topic_id ASC, id ASC").
		Find(&markers).Error; err != nil {
		return nil, fmt.Errorf("list board key event markers: %w", err)
	}
	return markers, nil
}

// CountConfirmedThreadMarkers returns confirmed marker counts grouped by
// source_thread_id for a set of threads (overview ★n badges: count is the
// number of independent confirmed thread markers under that source, never a
// section-level flag).
func (r *TopicGraphRepository) CountConfirmedThreadMarkers(threadIDs []uint) (map[uint]int, error) {
	out := make(map[uint]int, len(threadIDs))
	if len(threadIDs) == 0 {
		return out, nil
	}
	type row struct {
		SourceThreadID uint
		Count          int
	}
	var rows []row
	if err := r.db.Model(&TopicKeyEventMarker{}).
		Select("source_thread_id, COUNT(*) AS count").
		Where("source_thread_id IN ? AND state = ?", threadIDs, KeyEventStateConfirmed).
		Group("source_thread_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count thread key event markers: %w", err)
	}
	for _, r := range rows {
		out[r.SourceThreadID] = r.Count
	}
	return out, nil
}

// ListConfirmedKeyEventsBySections returns confirmed markers grouped by
// source_section_id for the given sections (compose ★ read-only projection).
//
// One batch query, no thread join: source_section_id and source_thread_id are
// written together when a marker is created and re-attached together on
// rebuild, so section lookup is complete and also covers markers whose thread
// row is gone. Markers whose source_section_id is NULL (detached snapshots
// after a rebuild fingerprint mismatch) are never returned — the compose ★
// marker must not guess an owner (daily-report 约束 31).
func (r *TopicGraphRepository) ListConfirmedKeyEventsBySections(sectionIDs []uint) (map[uint][]TopicKeyEventMarker, error) {
	out := make(map[uint][]TopicKeyEventMarker, len(sectionIDs))
	if len(sectionIDs) == 0 {
		return out, nil
	}
	var markers []TopicKeyEventMarker
	if err := r.db.
		Where("source_section_id IN ? AND state = ?", sectionIDs, KeyEventStateConfirmed).
		Order("source_section_id ASC, id ASC").
		Find(&markers).Error; err != nil {
		return nil, fmt.Errorf("list confirmed key events by sections: %w", err)
	}
	for _, marker := range markers {
		if marker.SourceSectionID == nil {
			continue
		}
		out[*marker.SourceSectionID] = append(out[*marker.SourceSectionID], marker)
	}
	return out, nil
}

// invalidateKeyEventRunToken rotates generation_token for the (board, date)
// run row inside the rebuild transaction (design D3): an in-flight task from
// the PREVIOUS build must drop its whole batch on save (token mismatch)
// instead of writing suggestions against replaced threads; the next spawn
// overwrites the row anyway. Missing rows (first build / never spawned) are
// not created — best-effort UPDATE only.
func invalidateKeyEventRunToken(tx *gorm.DB, boardID uint, periodDate time.Time) error {
	return tx.Model(&KeyEventRecommendationRun{}).
		Where("semantic_board_id = ? AND report_id IN (SELECT id FROM board_daily_reports WHERE semantic_board_id = ? AND period_date = ?)",
			boardID, boardID, periodDate.Format("2006-01-02")).
		Updates(map[string]any{
			"generation_token": uuid.NewString(),
			"updated_at":       time.Now(),
		}).Error
}

// ── 重建保留（design D1：SaveReport 同事务重附）────────────────────────────

// ReAttachKeyEventMarkers re-anchors detached markers onto newly inserted
// threads whose source fingerprint matches EXACTLY. Called inside the
// SaveReport transaction after threads are persisted: identical
// board/topic/date/title/tag/article sets ⇒ the user's marker follows the
// new row; anything else stays detached (来源替换快照). Only rows currently
// detached (or pointing at replaced rows) are touched — confirmed content
// snapshots are refreshed from the new identical source.
func ReAttachKeyEventMarkers(tx *gorm.DB, boardID uint, periodDate time.Time, sections []DailyReportSection, threadBatches [][]DailyReportThread) error {
	// Rebuild invalidates the recommendation run's generation token (D3).
	if err := invalidateKeyEventRunToken(tx, boardID, periodDate); err != nil {
		return err
	}
	type candidate struct {
		key     string
		topicID uint
		thread  DailyReportThread
		section DailyReportSection
	}
	var candidates []candidate
	for secIdx, sec := range sections {
		if sec.PersistentTopicID == nil {
			continue
		}
		if secIdx >= len(threadBatches) {
			continue
		}
		for _, th := range threadBatches[secIdx] {
			candidates = append(candidates, candidate{
				key:     ThreadKeyEventSourceKey(boardID, *sec.PersistentTopicID, periodDate, th),
				topicID: *sec.PersistentTopicID,
				thread:  th,
				section: sec,
			})
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	for _, cand := range candidates {
		var marker TopicKeyEventMarker
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("semantic_board_id = ? AND source_key = ?", boardID, cand.key).
			First(&marker).Error
		if err != nil {
			continue // no marker for this exact source — nothing to re-anchor
		}
		threadID := cand.thread.ID
		sectionID := cand.section.ID
		reportID := cand.thread.ReportID
		topicID := cand.topicID
		updates := map[string]any{
			"source_thread_id":    threadID,
			"source_section_id":   sectionID,
			"source_report_id":    reportID,
			"persistent_topic_id": topicID,
		}
		// Refresh content snapshots from the new (identical-fingerprint)
		// source so title/summary stay current; state is NEVER touched here.
		if marker.State == KeyEventStateSuggested {
			updates["title_snapshot"] = cand.thread.Title
			updates["summary_snapshot"] = cand.thread.Summary
			updates["tag_ids_snapshot"] = cand.thread.TagIDs
			updates["article_ids_snapshot"] = cand.thread.RelatedArticleIDs
		}
		if err := tx.Model(&TopicKeyEventMarker{}).Where("id = ?", marker.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("re-attach key event marker %d: %w", marker.ID, err)
		}
	}
	return nil
}

// ListBoardKeyEventMarkersByState returns the board's newest markers in a
// given state (used by the recommendation prompt's processed-history block;
// never exposes other boards' rows).
func (r *TopicGraphRepository) ListBoardKeyEventMarkersByState(boardID uint, state string, limit int) ([]TopicKeyEventMarker, error) {
	if limit <= 0 {
		limit = 1
	}
	var markers []TopicKeyEventMarker
	if err := r.db.Where("semantic_board_id = ? AND state = ?", boardID, state).
		Order("source_period_date DESC, id DESC").Limit(limit).
		Find(&markers).Error; err != nil {
		return nil, fmt.Errorf("list board key event markers: %w", err)
	}
	return markers, nil
}

// ── 推荐任务状态（design D3）────────────────────────────────────────────────

// UpsertKeyEventRecommendationRun starts (or restarts) the run row for a
// report with a fresh generation token. A rebuild calls this again, which is
// exactly how a late task detects its own staleness.
func (r *TopicGraphRepository) UpsertKeyEventRecommendationRun(reportID, boardID uint, token string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var run KeyEventRecommendationRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("report_id = ?", reportID).First(&run).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&KeyEventRecommendationRun{
				ReportID:        reportID,
				SemanticBoardID: boardID,
				State:           KeyEventRunStateRunning,
				GenerationToken: token,
			}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&KeyEventRecommendationRun{}).Where("id = ?", run.ID).
			Updates(map[string]any{
				"state":            KeyEventRunStateRunning,
				"generation_token": token,
				"candidate_count":  0,
				"suggested_count":  0,
				"error":            "",
			}).Error
	})
}

// FinishKeyEventRecommendationRun records the terminal state of a run. The
// token guards staleness: a run whose token no longer matches the row (a
// rebuild restarted the recommendation) is left alone — the row belongs to
// the newer run.
func (r *TopicGraphRepository) FinishKeyEventRecommendationRun(reportID uint, token, state string, candidateCount, suggestedCount int, errMsg string) error {
	return r.db.Model(&KeyEventRecommendationRun{}).
		Where("report_id = ? AND generation_token = ?", reportID, token).
		Updates(map[string]any{
			"state":           state,
			"candidate_count": candidateCount,
			"suggested_count": suggestedCount,
			"error":           errMsg,
		}).Error
}

// GetKeyEventRecommendationRun loads the run row for a report (nil when none).
func (r *TopicGraphRepository) GetKeyEventRecommendationRun(reportID uint) (*KeyEventRecommendationRun, error) {
	var run KeyEventRecommendationRun
	err := r.db.Where("report_id = ?", reportID).First(&run).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}
