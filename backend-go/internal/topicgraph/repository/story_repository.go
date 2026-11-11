package repository

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// Story repository — persistence half of add-event-story-heat batch 2c
// (tasks 3.1 DB 召回 / 3.2 / 3.3 / 3.4 / 3.5). The pure decision lives in
// service/story_assign.go (2b); this file only fetches candidates, persists
// verdicts, merges story lines and recomputes counts.
//
// 红线（pinned 2c 编排约束）：
//   - 计数用重算不用自增：RecomputeBoardStoryCounts 是唯一写
//     thread_count/article_count/first_seen_date/last_seen_date 的入口
//     （SaveReport 重跑会删旧 threads，自增会虚高）。
//   - 合并保护人工线程：MergeBoardStories 只移动 story_manual=false 的
//     threads；manual threads 保持 story_id=from，读取经 merged_into 一跳
//     解析到存活者。
//   - merged_into 只允许一层：拒绝在已 merged 的行上再指/再合并。

// Sentinel errors for story identity operations.
var (
	// ErrStoryThreadNotAttachable — the thread is story_manual=true or
	// already carries a story_id (or belongs to another report); the caller
	// treats it as a benign skip (idempotent re-run / human protection).
	ErrStoryThreadNotAttachable = errors.New("story: thread not attachable")
	// ErrStoryTwoLevelChain — a merged_into chain longer than one hop was
	// detected or would be created; reading refuses, merging rejects.
	ErrStoryTwoLevelChain = errors.New("story: two-level merged_into chain")
)

// Story recall tuning (design §5.1: 宽进严出 — the vector only RECALLS, it
// never decides; the precision tiers live in service/story_assign.go).
const (
	// StoryRecallWindowDays is the recall window (14 天，含当天 — test-cases
	// §3.3: 边界为闭区间).
	StoryRecallWindowDays = 14
	// StoryRecallLimit caps the candidate list (design §5.1 top 10).
	StoryRecallLimit = 10
	// StoryRecallMaxDistance is the wide cosine-distance cutoff for recall
	// (dist <= 0.5, deliberately loose — merging is decided elsewhere).
	StoryRecallMaxDistance = 0.5
)

// StoryCandidate is one recalled board_stories row (design §5.1). The
// service layer converts this into its own assignment/normalization shapes
// (service.StoryCandidate / service.SubjectCandidate); ArticleIDs is left
// empty by recall/list and filled by the caller via GetStoryArticleIDs when
// the article-overlap fallback path needs it.
type StoryCandidate struct {
	// Title and Summary are evidence from the latest thread, not the whole story.
	Title      string
	Summary    string
	ID         uint
	Subject    string
	SubjectKey string
	Action     string
	Object     string
	OccurredAt string
	PeriodDate time.Time
	// ArticleIDs aggregates the story's threads' related_article_ids
	// (union); filled only where the caller needs evidence overlap.
	ArticleIDs []uint
	// Distance is the recall embedding cosine distance (embedding <=> vec).
	Distance float64
	// Origin is rule|model|manual (board_stories.origin).
	Origin string
	// SubjectEmbedding is the parsed subject_key embedding vector (empty on
	// parse failure — the normalization tier then skips this candidate).
	SubjectEmbedding []float64
}

// ── Unassigned threads & report geometry ────────────────────────────────────

// ListUnassignedThreads returns the report's threads awaiting story
// attribution: story_id IS NULL and story_manual=false (人工拆分/指派一律
// 跳过 — specs/event-story「人工纠正优先」), ordered by id ascending so the
// attribution walks events in report order (deterministic merges).
func (r *TopicGraphRepository) ListUnassignedThreads(reportID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := r.db.Where("report_id = ? AND story_id IS NULL AND story_manual = ?",
		reportID, false).
		Order("id ASC").
		Find(&threads).Error
	if err != nil {
		return nil, fmt.Errorf("list unassigned threads for report %d: %w", reportID, err)
	}
	return threads, nil
}

// GetSectionTopicIDs maps each section id of the report to its persistent
// topic id (sections without a topic are skipped). Used by the attribution
// to fill new story lines' lane_id (首次归属，可空).
func (r *TopicGraphRepository) GetSectionTopicIDs(reportID uint) (map[uint]uint, error) {
	var rows []struct {
		ID                uint `gorm:"column:id"`
		PersistentTopicID uint `gorm:"column:persistent_topic_id"`
	}
	err := r.db.Model(&DailyReportSection{}).
		Select("id, persistent_topic_id").
		Where("report_id = ? AND persistent_topic_id IS NOT NULL", reportID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get section topic ids for report %d: %w", reportID, err)
	}
	out := make(map[uint]uint, len(rows))
	for _, row := range rows {
		out[row.ID] = row.PersistentTopicID
	}
	return out, nil
}

// GetEventTags loads the event-category tags among tagIDs (full rows — the
// attribution needs metadata.event_fingerprint/event_keywords and aliases
// from the DB, never from the tag cache — pinned finding: 缓存对象可能无
// metadata).
func (r *TopicGraphRepository) GetEventTags(tagIDs []uint) ([]models.TopicTag, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	var tags []models.TopicTag
	err := r.db.Where("id IN ? AND category = ?", tagIDs, models.TagCategoryEvent).
		Find(&tags).Error
	if err != nil {
		return nil, fmt.Errorf("get event tags: %w", err)
	}
	return tags, nil
}

// SetTagFingerprintTrusted persists the keyword-consistency verdict as the
// FLAT metadata key fingerprint_trusted (task 2.3/2.4 contract), preserving
// every other metadata key (event_fingerprint / event_keywords included).
// Read-modify-write against the stored row so a concurrent description save
// does not get clobbered. A no-op when the value is unchanged.
func (r *TopicGraphRepository) SetTagFingerprintTrusted(tagID uint, trusted bool) error {
	var tag models.TopicTag
	if err := r.db.Select("id", "metadata").First(&tag, "id = ?", tagID).Error; err != nil {
		return fmt.Errorf("load tag %d for fingerprint_trusted: %w", tagID, err)
	}
	if current, ok := tag.Metadata["fingerprint_trusted"].(bool); ok && current == trusted {
		return nil // value unchanged — skip the write
	}
	merged := models.MetadataMap{}
	for k, v := range tag.Metadata {
		merged[k] = v
	}
	merged["fingerprint_trusted"] = trusted
	if err := r.db.Model(&models.TopicTag{}).
		Where("id = ?", tagID).
		Update("metadata", merged).Error; err != nil {
		return fmt.Errorf("set fingerprint_trusted for tag %d: %w", tagID, err)
	}
	return nil
}

// ── Candidate recall ────────────────────────────────────────────────────────

// storyCandidateFilters is the shared WHERE shape for both recall paths:
// same board, alive (merged_into IS NULL + status active), not human-created
// (origin='manual' lines are human artifacts — never auto-merged into), and
// inside the 14-day last_seen window.
func storyCandidateFilters(boardID uint, since time.Time) (string, []any) {
	return "board_stories.board_id = ? AND board_stories.merged_into IS NULL AND board_stories.status = ? AND board_stories.origin <> ? AND board_stories.last_seen_date >= ?",
		[]any{boardID, StoryStatusActive, StoryOriginManual, since.Format("2006-01-02")}
}

// RecallStoryCandidates recalls candidate story lines via pgvector cosine
// distance (design §5.1: same board, 14-day window, alive, non-manual,
// dist <= 0.5, top limit). Vector distance RECALLS ONLY — never decides.
// On non-Postgres dialects (SQLite unit tests) it returns (nil, nil) so the
// caller degrades to the ListRecentStoryCandidates + article-overlap path.
func (r *TopicGraphRepository) RecallStoryCandidates(boardID uint, vector string, since time.Time, limit int) ([]StoryCandidate, error) {
	if r.db.Name() != "postgres" {
		return nil, nil
	}
	if limit <= 0 {
		limit = StoryRecallLimit
	}
	where, args := storyCandidateFilters(boardID, since)
	query := fmt.Sprintf(`
		SELECT board_stories.id, subject, subject_key, action, object, occurred_at,
		       evidence.title, evidence.summary, evidence_report.period_date,
		       board_stories.embedding <=> ?::vector AS distance, origin, subject_embedding
		FROM board_stories
		LEFT JOIN daily_report_threads evidence ON evidence.id = (
		    SELECT t.id FROM daily_report_threads t
		    JOIN board_daily_reports report ON report.id = t.report_id
		    WHERE t.story_id = board_stories.id
		    ORDER BY report.period_date DESC, t.created_at DESC, t.id DESC LIMIT 1
		)
		LEFT JOIN board_daily_reports evidence_report ON evidence_report.id = evidence.report_id
		WHERE %s AND board_stories.embedding IS NOT NULL AND board_stories.embedding <=> ?::vector <= %g
		ORDER BY distance ASC, board_stories.id ASC
		LIMIT ?`, where, StoryRecallMaxDistance)
	rows, err := scanStoryCandidateRows(r.db, query,
		append([]any{vector}, append(args, vector, limit)...))
	if err != nil {
		return nil, fmt.Errorf("recall story candidates for board %d: %w", boardID, err)
	}
	return rows, nil
}

// ListRecentStoryCandidates lists candidate story lines without a vector
// (the recall fallback for embedding-less events, design §9 降级 3): same
// filters as recall minus the vector clauses, newest first.
func (r *TopicGraphRepository) ListRecentStoryCandidates(boardID uint, since time.Time, limit int) ([]StoryCandidate, error) {
	if limit <= 0 {
		limit = StoryRecallLimit
	}
	where, args := storyCandidateFilters(boardID, since)
	query := fmt.Sprintf(`
		SELECT board_stories.id, subject, subject_key, action, object, occurred_at,
		       evidence.title, evidence.summary, evidence_report.period_date,
		       0 AS distance, origin, subject_embedding
		FROM board_stories
		LEFT JOIN daily_report_threads evidence ON evidence.id = (
		    SELECT t.id FROM daily_report_threads t
		    JOIN board_daily_reports report ON report.id = t.report_id
		    WHERE t.story_id = board_stories.id
		    ORDER BY report.period_date DESC, t.created_at DESC, t.id DESC LIMIT 1
		)
		LEFT JOIN board_daily_reports evidence_report ON evidence_report.id = evidence.report_id
		WHERE %s
		ORDER BY last_seen_date DESC, board_stories.id DESC
		LIMIT ?`, where)
	rows, err := scanStoryCandidateRows(r.db, query, append(args, limit))
	if err != nil {
		return nil, fmt.Errorf("list recent story candidates for board %d: %w", boardID, err)
	}
	return rows, nil
}

// storyCandidateRow mirrors the candidate SELECT shape (occurred_at is a
// timestamptz and scans NULL as empty).
type storyCandidateRow struct {
	Title            string     `gorm:"column:title"`
	Summary          string     `gorm:"column:summary"`
	ID               uint       `gorm:"column:id"`
	Subject          string     `gorm:"column:subject"`
	SubjectKey       string     `gorm:"column:subject_key"`
	Action           string     `gorm:"column:action"`
	Object           string     `gorm:"column:object"`
	PeriodDate       time.Time  `gorm:"column:period_date"`
	OccurredAt       *time.Time `gorm:"column:occurred_at"`
	Distance         float64    `gorm:"column:distance"`
	Origin           string     `gorm:"column:origin"`
	SubjectEmbedding string     `gorm:"column:subject_embedding"`
}

func scanStoryCandidateRows(db *gorm.DB, query string, args []any) ([]StoryCandidate, error) {
	var rows []storyCandidateRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]StoryCandidate, 0, len(rows))
	for _, row := range rows {
		occurredAt := ""
		if row.OccurredAt != nil {
			occurredAt = row.OccurredAt.Format("2006-01-02")
		}
		subjectEmb, _ := repoParsePgVector(row.SubjectEmbedding) // parse failure → empty (degrade)
		out = append(out, StoryCandidate{
			ID:               row.ID,
			Title:            row.Title,
			Summary:          row.Summary,
			Subject:          row.Subject,
			SubjectKey:       row.SubjectKey,
			Action:           row.Action,
			Object:           row.Object,
			OccurredAt:       occurredAt,
			PeriodDate:       row.PeriodDate,
			Distance:         row.Distance,
			Origin:           row.Origin,
			SubjectEmbedding: subjectEmb,
		})
	}
	return out, nil
}

// GetStoryArticleIDs aggregates each story line's article evidence from its
// threads' related_article_ids (union, deduped). Postgres uses
// jsonb_array_elements_text; other dialects parse the JSON in Go — both
// paths behave identically (NULL / non-array JSON degrade to empty).
func (r *TopicGraphRepository) GetStoryArticleIDs(storyIDs []uint) (map[uint][]uint, error) {
	if len(storyIDs) == 0 {
		return map[uint][]uint{}, nil
	}
	out := make(map[uint][]uint, len(storyIDs))
	if r.db.Name() == "postgres" {
		var rows []struct {
			StoryID   uint   `gorm:"column:story_id"`
			ArticleID string `gorm:"column:article_id"`
		}
		//nolint:gosec // G201: storyIDs are bound via ? placeholders
		if err := r.db.Raw(`
			SELECT t.story_id, art.article_id
			FROM daily_report_threads t,
			     jsonb_array_elements_text(
			       CASE WHEN jsonb_typeof(t.related_article_ids) = 'array'
			            THEN t.related_article_ids ELSE '[]'::jsonb END
			     ) AS art(article_id)
			WHERE t.story_id IN ?
		`, storyIDs).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("get story article ids (pg): %w", err)
		}
		for _, row := range rows {
			var id uint
			if _, err := fmt.Sscanf(row.ArticleID, "%d", &id); err != nil {
				continue
			}
			out[row.StoryID] = appendUniqueUint(out[row.StoryID], id)
		}
		return out, nil
	}
	// Portable path (SQLite tests): parse the JSON arrays in Go.
	var rows []DailyReportThread
	if err := r.db.Select("story_id", "related_article_ids").
		Where("story_id IN ?", storyIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get story article ids: %w", err)
	}
	for _, row := range rows {
		if row.StoryID == nil {
			continue
		}
		for _, id := range jsonUintIDs(row.RelatedArticleIDs) {
			out[*row.StoryID] = appendUniqueUint(out[*row.StoryID], id)
		}
	}
	return out, nil
}

func appendUniqueUint(list []uint, v uint) []uint {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

// EarliestArticleAt returns MIN(COALESCE(pub_date, created_at)) over the
// articles — the occurred_at fallback for new story lines whose fingerprint
// carries no date. Empty input or query failure degrades to the zero time
// (best-effort field; never blocks attribution). The aggregate is scanned
// as a string and parsed in Go: SQLite returns TEXT for datetime aggregates
// (unscannable into *time.Time), Postgres returns an ISO timestamp string.
func (r *TopicGraphRepository) EarliestArticleAt(articleIDs []uint) (time.Time, error) {
	if len(articleIDs) == 0 {
		return time.Time{}, nil
	}
	var rows []struct {
		Earliest *string `gorm:"column:earliest"`
	}
	if err := r.db.Model(&models.Article{}).
		Select("MIN(COALESCE(pub_date, created_at)) AS earliest").
		Where("id IN ?", articleIDs).
		Scan(&rows).Error; err != nil {
		logging.Warnf("story: earliest article At for %v failed: %v", articleIDs, err)
		return time.Time{}, nil
	}
	if len(rows) == 0 || rows[0].Earliest == nil {
		return time.Time{}, nil
	}
	parsed, err := parseStoryTimestamp(*rows[0].Earliest)
	if err != nil {
		logging.Warnf("story: earliest article At for %v unparseable (%q): %v", articleIDs, *rows[0].Earliest, err)
		return time.Time{}, nil
	}
	return parsed, nil
}

// parseStoryTimestamp parses a datetime literal from either dialect
// (Postgres "2026-11-05 12:00:00+00", SQLite "2026-11-05 12:00:00+00:00"
// and friends) into time.Time.
func parseStoryTimestamp(raw string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999-07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable timestamp %q", raw)
}

// ── Story line writes ───────────────────────────────────────────────────────

// CreateBoardStory inserts a new story line. The caller pre-computes the
// identity fields and leaves counts at zero; RecomputeBoardStoryCounts (via
// the subsequent attach) is the only count writer. Blank vector strings are
// omitted so pgvector never sees an empty literal (same treatment as
// SaveThreads' no-embedding batch).
func (r *TopicGraphRepository) CreateBoardStory(story *BoardStory) error {
	if story.Status == "" {
		story.Status = StoryStatusActive
	}
	if story.Origin == "" {
		story.Origin = StoryOriginRule
	}
	omit := []string{}
	if story.Embedding == "" {
		omit = append(omit, "embedding")
	}
	if story.SubjectEmbedding == "" {
		omit = append(omit, "subject_embedding")
	}
	if err := r.db.Omit(omit...).Create(story).Error; err != nil {
		return fmt.Errorf("create board story: %w", err)
	}
	return nil
}

// AttachThreadToBoardStory links a thread to a story line in one transaction:
//  1. the thread must be story_manual=false, story_id IS NULL and belong to
//     the given report — otherwise ErrStoryThreadNotAttachable (the caller
//     skips; human verdicts are never overwritten, specs/event-story 人工
//     纠正优先);
//  2. story_id + is_development are written on the thread (the target is
//     resolved through merged_into first so a merged target never gets new
//     threads);
//  3. counts/dates are RECOMPUTED (never incremented — SaveReport reruns
//     delete old threads; recomputation is the only honest count).
func (r *TopicGraphRepository) AttachThreadToBoardStory(reportID, threadID, storyID uint, isDevelopment bool) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var thread DailyReportThread
		if err := tx.Where("id = ? AND report_id = ? AND story_manual = ? AND story_id IS NULL",
			threadID, reportID, false).
			First(&thread).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStoryThreadNotAttachable
			}
			return fmt.Errorf("load thread %d for attach: %w", threadID, err)
		}
		survivorID, err := resolveBoardStoryIDWith(tx, storyID)
		if err != nil {
			return err
		}
		if err := tx.Model(&DailyReportThread{}).
			Where("id = ?", threadID).
			Updates(map[string]any{
				"story_id":       survivorID,
				"is_development": isDevelopment,
			}).Error; err != nil {
			return fmt.Errorf("attach thread %d to story %d: %w", threadID, survivorID, err)
		}
		if err := recomputeSignalsForStoryWith(tx, survivorID); err != nil {
			return err
		}
		return recomputeBoardStoryCountsWith(tx, survivorID)
	})
}

// RecomputeBoardStoryCounts recomputes thread_count / article_count /
// first_seen_date / last_seen_date from the threads currently pointing at
// the story line. A story with zero threads gets counts zeroed but keeps its
// dates (a merged-out line keeps its historical window; NULL dates would
// violate NOT NULL).
func (r *TopicGraphRepository) RecomputeBoardStoryCounts(storyID uint) error {
	return recomputeBoardStoryCountsWith(r.db, storyID)
}

func recomputeBoardStoryCountsWith(db *gorm.DB, storyID uint) error {
	type threadRow struct {
		RelatedArticleIDs JSON      `gorm:"column:related_article_ids"`
		PeriodDate        time.Time `gorm:"column:period_date"`
	}
	var rows []threadRow
	if err := db.Table("daily_report_threads AS t").
		Select("t.related_article_ids, r.period_date").
		Joins("JOIN board_daily_reports AS r ON r.id = t.report_id").
		Where("t.story_id = ?", storyID).
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("recompute story %d counts: load threads: %w", storyID, err)
	}
	updates := map[string]any{
		"thread_count":  len(rows),
		"article_count": 0,
	}
	articleCount := 0
	for _, row := range rows {
		articleCount += len(jsonUintIDs(row.RelatedArticleIDs))
	}
	updates["article_count"] = articleCount
	if len(rows) > 0 {
		first, last := rows[0].PeriodDate, rows[0].PeriodDate
		for _, row := range rows[1:] {
			if row.PeriodDate.Before(first) {
				first = row.PeriodDate
			}
			if row.PeriodDate.After(last) {
				last = row.PeriodDate
			}
		}
		updates["first_seen_date"] = first
		updates["last_seen_date"] = last
	}
	if err := db.Model(&BoardStory{}).
		Where("id = ?", storyID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("recompute story %d counts: write: %w", storyID, err)
	}
	return recomputeStoryParticipantsWith(db, storyID)
}

// MergeBoardStories merges fromID into intoID and returns the survivor.
//
// Chain discipline (design §5.4 / 2c pinned 约束): merged_into allows ONE
// hop only —
//   - an already-merged endpoint (from or into) is rejected
//     (ErrStoryTwoLevelChain);
//   - any story previously merged INTO from is re-pointed at the survivor so
//     no two-level chain appears (旧记录直接指向最终存活者);
//   - only threads with story_manual=false move to the survivor; manual
//     threads keep story_id=from and read-path consumers resolve them
//     through merged_into one hop (人工指派不被自动覆盖).
//
// Source signals are NOT migrated — heat resolves merged_into in Go.
func (r *TopicGraphRepository) MergeBoardStories(fromID, intoID uint) (uint, error) {
	var from, into BoardStory
	if err := r.db.Select("id", "merged_into", "status").First(&from, fromID).Error; err != nil {
		return 0, fmt.Errorf("load story %d for merge: %w", fromID, err)
	}
	if err := r.db.Select("id", "merged_into", "status").First(&into, intoID).Error; err != nil {
		return 0, fmt.Errorf("load story %d for merge: %w", intoID, err)
	}
	if from.MergedInto != nil || into.MergedInto != nil {
		return 0, ErrStoryTwoLevelChain
	}
	if fromID == intoID {
		return intoID, nil // no-op: same survivor
	}

	survivorID := intoID
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Flatten any stories previously merged into `from` — they must point
		// at the FINAL survivor, never at an intermediate (two-hop) node.
		if err := tx.Model(&BoardStory{}).
			Where("merged_into = ?", fromID).
			Update("merged_into", survivorID).Error; err != nil {
			return fmt.Errorf("flatten children of story %d: %w", fromID, err)
		}
		if err := tx.Model(&BoardStory{}).
			Where("id = ?", fromID).
			Updates(map[string]any{
				"merged_into": survivorID,
				"status":      StoryStatusMerged,
			}).Error; err != nil {
			return fmt.Errorf("mark story %d merged: %w", fromID, err)
		}
		// Human-assigned threads stay on `from` (readers resolve one hop);
		// every automatic attribution moves to the survivor.
		if err := tx.Model(&DailyReportThread{}).
			Where("story_id = ? AND story_manual = ?", fromID, false).
			Update("story_id", survivorID).Error; err != nil {
			return fmt.Errorf("move threads from story %d to %d: %w", fromID, survivorID, err)
		}
		if err := recomputeSignalsForStoryWith(tx, survivorID); err != nil {
			return err
		}
		if err := recomputeBoardStoryCountsWith(tx, survivorID); err != nil {
			return err
		}
		return recomputeBoardStoryCountsWith(tx, fromID)
	})
	if err != nil {
		return 0, err
	}
	return survivorID, nil
}

// ResolveBoardStoryID resolves a story id through merged_into: an un-merged
// id returns itself; a merged id returns its survivor. A survivor that is
// itself still merged means a two-level chain — refused loudly (the invariant
// is one hop only).
func (r *TopicGraphRepository) ResolveBoardStoryID(id uint) (uint, error) {
	return resolveBoardStoryIDWith(r.db, id)
}

func resolveBoardStoryIDWith(db *gorm.DB, id uint) (uint, error) {
	var row BoardStory
	if err := db.Select("id", "merged_into").First(&row, id).Error; err != nil {
		return 0, fmt.Errorf("load story %d for resolve: %w", id, err)
	}
	if row.MergedInto == nil {
		return row.ID, nil
	}
	var survivor BoardStory
	if err := db.Select("id", "merged_into").First(&survivor, *row.MergedInto).Error; err != nil {
		return 0, fmt.Errorf("load survivor %d of story %d: %w", *row.MergedInto, row.ID, err)
	}
	if survivor.MergedInto != nil {
		return 0, ErrStoryTwoLevelChain
	}
	return survivor.ID, nil
}

// deriveThreadScope is the SINGLE point of scope derivation (task 3.2,
// specs/daily-report-system): a thread whose tag_ids contain >= 2 event tags
// is a composite (综合稿 — no story identity); everything else is single.
// Runs inside SaveThreads so orchestrator / watch materialization / rebuild
// all inherit it. Query failure degrades to single + warning — scope must
// never block the save.
func deriveThreadScope(tx *gorm.DB, threads []DailyReportThread) {
	eventTagIDs := make(map[uint]bool)
	var allTagIDs []uint
	for i := range threads {
		for _, id := range jsonUintIDs(threads[i].TagIDs) {
			allTagIDs = appendUniqueUint(allTagIDs, id)
		}
	}
	if len(allTagIDs) == 0 {
		for i := range threads {
			threads[i].Scope = ThreadScopeSingle
		}
		return
	}
	// Test binaries that do not register the topic-tag models (or minimal
	// deployments) have no tag table: degrade without a failed SQL round-trip
	// so the query error does not pollute logs/tests.
	if !tx.Migrator().HasTable(&models.TopicTag{}) {
		for i := range threads {
			threads[i].Scope = ThreadScopeSingle
		}
		return
	}
	var eventRows []struct {
		ID uint `gorm:"column:id"`
	}
	if err := tx.Model(&models.TopicTag{}).
		Select("id").
		Where("id IN ? AND category = ?", allTagIDs, models.TagCategoryEvent).
		Scan(&eventRows).Error; err != nil {
		logging.Warnf("SaveThreads: scope derivation degraded to single (event tag lookup failed): %v", err)
		for i := range threads {
			threads[i].Scope = ThreadScopeSingle
		}
		return
	}
	for _, row := range eventRows {
		eventTagIDs[row.ID] = true
	}
	for i := range threads {
		eventCount := 0
		for _, id := range jsonUintIDs(threads[i].TagIDs) {
			if eventTagIDs[id] {
				eventCount++
			}
		}
		if eventCount >= 2 {
			threads[i].Scope = ThreadScopeComposite
		} else {
			threads[i].Scope = ThreadScopeSingle
		}
	}
}

// Thread scope values (specs/daily-report-system: scope 由事件标签数推导).
const (
	ThreadScopeSingle    = "single"
	ThreadScopeComposite = "composite"
)

// ── Cross-period dedup persistence (add-event-story-heat tasks 6.1-6.3) ────
//
// 展示层专用：以下读写只触碰 display_tier，绝不删行、绝不改 signals /
// story_id / 热度输入。判定逻辑在 service/daily_report_story_dedup.go。

// ListReportThreadsForDedup loads the report's threads with the identity and
// display fields the cross-period dedup judges (scope/story_manual/story_id
// decide participation; related_article_ids drives the article & feed sets).
// Ordered by id so verdicts (and their logs) are deterministic.
func (r *TopicGraphRepository) ListReportThreadsForDedup(reportID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := r.db.Select("id", "story_id", "scope", "story_manual", "is_development", "display_tier", "related_article_ids").
		Where("report_id = ?", reportID).
		Order("id ASC").
		Find(&threads).Error
	if err != nil {
		return nil, fmt.Errorf("list dedup threads for report %d: %w", reportID, err)
	}
	return threads, nil
}

// ListStoryBodyHistoryThreads loads the BODY threads of the given stories
// from OTHER reports within [since, untilExclusive) — display-tier history:
// only what actually rendered in a body counts as 已进过正文 (flash/hidden
// threads never rendered, so they are excluded). Grouping is by raw story_id:
// merges already re-point non-manual threads at the survivor, and manual
// threads keep their id on purpose (their tier is never rewritten anyway).
// Date bounds are formatted day strings and compared half-open — the same
// portable pattern as ListReports (SQLite stores dates as sortable text).
func (r *TopicGraphRepository) ListStoryBodyHistoryThreads(storyIDs []uint, excludeReportID uint, since, untilExclusive time.Time) ([]DailyReportThread, error) {
	if len(storyIDs) == 0 {
		return nil, nil
	}
	var threads []DailyReportThread
	err := r.db.Model(&DailyReportThread{}).
		Select("daily_report_threads.id, daily_report_threads.story_id, daily_report_threads.related_article_ids").
		Joins("JOIN board_daily_reports AS r ON r.id = daily_report_threads.report_id").
		Where("daily_report_threads.story_id IN ? AND daily_report_threads.report_id <> ? AND daily_report_threads.display_tier = ? AND r.period_date >= ? AND r.period_date < ?",
			storyIDs, excludeReportID, ThreadDisplayTierBody,
			since.Format("2006-01-02"), untilExclusive.Format("2006-01-02")).
		Find(&threads).Error
	if err != nil {
		return nil, fmt.Errorf("list body history threads for stories %v: %w", storyIDs, err)
	}
	return threads, nil
}

// ArticleFeeds maps article id → feed id, including archived articles.
// Archiving only rotates articles out of the feed's active window; their
// sources remain evidence for cross-period dedup, consistent with signals.
// Do not add an archived filter: the sourcestats package comment (rule 2,
// "window queries MUST NOT filter on `archived`") is the red line here;
// excluding retracted/malicious evidence someday requires a dedicated
// column, not a reuse of `archived`.
func (r *TopicGraphRepository) ArticleFeeds(articleIDs []uint) (map[uint]uint, error) {
	out := make(map[uint]uint)
	if len(articleIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID     uint `gorm:"column:id"`
		FeedID uint `gorm:"column:feed_id"`
	}
	if err := r.db.Model(&models.Article{}).
		Select("id", "feed_id").
		Where("id IN ?", articleIDs).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("map article feeds for %d ids: %w", len(articleIDs), err)
	}
	for _, row := range rows {
		out[row.ID] = row.FeedID
	}
	return out, nil
}

// SetThreadsDisplayTier batch-writes the dedup verdict for a set of threads.
// Display layer ONLY — callers must never route identity/heat writes here.
func (r *TopicGraphRepository) SetThreadsDisplayTier(threadIDs []uint, tier string) error {
	if len(threadIDs) == 0 {
		return nil
	}
	if err := r.db.Model(&DailyReportThread{}).
		Where("id IN ?", threadIDs).
		Update("display_tier", tier).Error; err != nil {
		return fmt.Errorf("set display_tier=%s for %d threads: %w", tier, len(threadIDs), err)
	}
	return nil
}
