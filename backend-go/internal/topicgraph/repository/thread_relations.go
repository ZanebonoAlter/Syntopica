package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 线索相关事件读模型（replace-lane-lifeline-with-topic-events D2–D6）──────
//
// 一次请求返回一份日报里**所有线索**的相关线索推荐（按 thread_id 分组），
// 避免一屏 N 条线索发 N 个请求（design D2）。
//
// 匹配口径 = **thread 向量**（design D3）：窗口 [report_date-days, report_date-1]
// （不含当期）内、同一 persistent_topic_id 的其它 section 所辖线索中，与当前
// 线索 embedding 余弦相似度 ≥ 0.75 的那些。旧的「event 标签 id 交集 + 启发式
// 打分」口径已被评审否决并整体移除——实测二者 top-3 零重叠，且历史 tag_ids
// 里 65% 的标签行已被孤儿清理硬删，标签口径的候选池只有向量口径的 1/5。
//
// 只读红线（spec「推荐读模型只读」）：不调 embedding 接口、不计算/写入向量、
// 不写 related_event_embeddings、不复用 related_event_similarity_threshold、
// 不写任何业务表/缓存/索引；也不触达日报生成 prompt 的 ListTopicRecentBriefs
// 路径（prompt 注入是另一条线：另开查询、不改它）。
//
// 查询路径（全部只读，不做全表向量扫描）：先按 report 取当期线索，用它们的话题
// 收敛出 topic 集合；再按 topic + 窗口把历史 section/thread 收敛成小集合，余弦
// 在 Go 里精确计算。也正因如此不给 daily_report_threads.embedding 加索引。

// Response status values (spec 缺失向量的处置 + design D6).
const (
	// ThreadRelationStatusReady — 有推荐，且没有当期线索因缺向量被跳过。
	ThreadRelationStatusReady = "ready"
	// ThreadRelationStatusPartial — 有推荐，但部分当期线索没有向量、出不了推荐。
	ThreadRelationStatusPartial = "partial"
	// ThreadRelationStatusEmpty — 没有任何线索有推荐，`threads` 为 `{}`。
	ThreadRelationStatusEmpty = "empty"
)

// Similarity bands (design D4). 只有两档：分数本身就是余弦相似度，不是概率，
// 也不掺时间衰减/篇数权重；小数只放在浮层里供核对，chip 上只显示档位词。
const (
	ThreadRelationLevelStrong  = "strong"
	ThreadRelationLevelRelated = "related"
)

// Similarity cutoffs. 0.75 是「这条线之前也谈过这件事」的下限（低于此在人工
// 抽查里已明显泛化）；0.85 以上升 strong 档。常数集中在这里便于按真实分布调。
const (
	threadRelationSimilarityThreshold = 0.75
	threadRelationStrongThreshold     = 0.85
)

// Supported windows. Anything else falls back to the short one (spec「非法天数
// 回落并回显」: never a 400).
const (
	threadRelationDaysShort = 7
	threadRelationDaysLong  = 30
)

const threadRelationDateLayout = "2006-01-02"

// ErrThreadRelationReportNotFound is returned by BuildThreadRelations when the
// report id does not exist (handler maps it to 404).
var ErrThreadRelationReportNotFound = errors.New("daily report not found")

// ThreadRelationsResponse is the read model of
// GET /api/daily-reports/:reportId/thread-relations.
type ThreadRelationsResponse struct {
	ReportID uint `json:"report_id"`
	// Days is the window actually used (7 or 30), echoed for the client.
	Days        int    `json:"days"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	Status      string `json:"status"`
	// Threads is keyed by thread id and never nil (serializes as `{}`).
	Threads map[uint][]ThreadRelation `json:"threads"`
	Total   int                       `json:"total"`
	// SkippedNoEmbedding counts the report's own threads dropped because they
	// carry no usable vector (>0 forces status=partial, design D6). Historical
	// threads without a vector simply never enter the candidate pool.
	SkippedNoEmbedding int `json:"skipped_no_embedding"`
}

// ThreadRelation is one recommended historical thread for one current thread.
// Identity is the thread itself: no semantic merging across candidates (spec
// 不做语义归并), so two near-duplicate threads show up as two entries.
type ThreadRelation struct {
	ThreadID   uint   `json:"thread_id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	PeriodDate string `json:"period_date"`
	// Score is the cosine similarity of the two thread embeddings, unrounded
	// and never damped by recency/volume/confirmed (design D4).
	Score float64 `json:"score"`
	// Level is "strong" (>= 0.85) or "related" (>= 0.75).
	Level string `json:"level"`
	// Confirmed mirrors a human `state='confirmed'` key-event marker on the
	// topic whose frozen snapshot carries one of this thread's event tags.
	// Read-only: markers are never touched here.
	Confirmed bool `json:"confirmed"`
	// SourceArticle is the thread's first related article; nil when the thread
	// listed none or that row is gone (不编造标题).
	SourceArticle *ThreadRelationSourceArticle `json:"source_article"`
}

// ThreadRelationSourceArticle is the first article of a recommended thread.
type ThreadRelationSourceArticle struct {
	ID          uint    `json:"id"`
	Title       string  `json:"title"`
	Feed        string  `json:"feed"`
	PublishedAt *string `json:"published_at"`
}

// NormalizeThreadRelationDays clamps the `days` query value to the two
// supported windows (7 / 30). Everything else — empty, unparsable, 0, 14,
// 365 — falls back to 7 and is echoed back in the response.
func NormalizeThreadRelationDays(days int) int {
	if days == threadRelationDaysLong {
		return threadRelationDaysLong
	}
	return threadRelationDaysShort
}

// threadRelationDay flattens a date to UTC midnight so whole-day arithmetic
// cannot drift across the several timezones a DATE column may be read in.
func threadRelationDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// threadRelationLevel maps a similarity to its band. Only reached for
// candidates at or above the inclusion threshold.
func threadRelationLevel(score float64) string {
	if score >= threadRelationStrongThreshold {
		return ThreadRelationLevelStrong
	}
	return ThreadRelationLevelRelated
}

// threadRelationSimilarity is the cosine similarity `1 - (a <=> b)`, computed
// directly as dot / (|a|·|b|) rather than as 1 minus the distance: the
// `1 - (1 - s)` round trip loses an ulp and can push an exact band boundary
// (0.85) into the lower band. Mismatched or zero-length vectors return NaN,
// which callers exclude via an explicit `>= threshold` test.
func threadRelationSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return math.NaN()
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return math.NaN()
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// threadRelationParseVector turns a pgvector text literal into floats. Empty or
// unparsable input means "no usable vector" (the caller treats it as missing).
func threadRelationParseVector(raw string) []float64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	vec, err := repoParsePgVector(raw)
	if err != nil || len(vec) == 0 {
		return nil
	}
	return vec
}

// threadRelationCurrentThread — one thread of the requested report.
type threadRelationCurrentThread struct {
	ThreadID          uint
	PersistentTopicID *uint
	// Embedding is pgvector text, empty when the column is NULL.
	Embedding string
}

// threadRelationCandidate — one window thread under the same topic, vector
// already parsed (nil = unusable → never a candidate).
type threadRelationCandidate struct {
	ThreadID          uint
	PersistentTopicID uint
	PeriodDate        time.Time
	Title             string
	Summary           string
	TagIDs            JSON
	ArticleIDs        JSON
	Embedding         []float64
}

// BuildThreadRelations builds the related-thread read model for one report.
// `days` is normalized to 7/30 before use and echoed in the response.
// Returns ErrThreadRelationReportNotFound when the report does not exist.
func (r *TopicGraphRepository) BuildThreadRelations(reportID uint, days int) (*ThreadRelationsResponse, error) {
	days = NormalizeThreadRelationDays(days)

	var report BoardDailyReport
	if err := r.db.Select("id", "period_date").First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrThreadRelationReportNotFound
		}
		return nil, fmt.Errorf("thread relations: load report %d: %w", reportID, err)
	}

	// Window [report_date-days, report_date-1]: the current period is
	// deliberately excluded — today's threads are on the same screen already.
	reportDay := threadRelationDay(report.PeriodDate)
	windowEnd := reportDay.AddDate(0, 0, -1)
	windowStart := reportDay.AddDate(0, 0, -days)

	resp := &ThreadRelationsResponse{
		ReportID:    reportID,
		Days:        days,
		WindowStart: windowStart.Format(threadRelationDateLayout),
		WindowEnd:   windowEnd.Format(threadRelationDateLayout),
		Threads:     map[uint][]ThreadRelation{},
	}

	current, err := r.threadRelationCurrentThreads(reportID)
	if err != nil {
		return nil, err
	}

	// 当期线索的话题集合是候选集的唯一边界（同话题才有「同一件事的进展」）。
	topicIDs := make([]uint, 0, len(current))
	seenTopic := map[uint]bool{}
	for _, ct := range current {
		if ct.PersistentTopicID == nil || seenTopic[*ct.PersistentTopicID] {
			continue
		}
		seenTopic[*ct.PersistentTopicID] = true
		topicIDs = append(topicIDs, *ct.PersistentTopicID)
	}

	var candidates []threadRelationCandidate
	if len(topicIDs) > 0 {
		candidates, err = r.threadRelationCandidateRows(topicIDs, reportID, windowStart, windowEnd)
		if err != nil {
			return nil, err
		}
	}

	// confirmed 星标所需的两次批量查询：候选 thread 的 event 标签集合 +
	// 各话题的 confirmed 注记快照。两者都不参与候选集（候选集只由向量决定），
	// 所以标签行被硬删也不会让候选消失——旧的「悬空标签 / stale_candidates」
	// 一整态随标签口径一起删除（design D6）。
	eventTagIDs, err := r.threadRelationEventTagIDs(candidates)
	if err != nil {
		return nil, err
	}
	confirmedTags, err := r.threadRelationConfirmedTags(topicIDs)
	if err != nil {
		return nil, err
	}
	confirmedThreads := threadRelationConfirmedCandidates(candidates, eventTagIDs, confirmedTags)

	// pending 只为来源文章的那一次批量查询做中转。
	type pendingRelation struct {
		threadID  uint
		articleID uint
		relation  ThreadRelation
	}
	pending := make([]pendingRelation, 0, len(candidates))

	for _, ct := range current {
		currentVector := threadRelationParseVector(ct.Embedding)
		if len(currentVector) == 0 {
			// 当期线索没有可用向量 → 这条线索不产出推荐，如实计数（B7）。
			// 不为了凑结果临时算 embedding（spec 缺失向量的处置）。
			resp.SkippedNoEmbedding++
			continue
		}
		if ct.PersistentTopicID == nil {
			continue // 无话题归属：不存在「同一话题内」的候选面
		}
		for _, cand := range candidates {
			if cand.PersistentTopicID != *ct.PersistentTopicID || len(cand.Embedding) == 0 {
				continue
			}
			score := threadRelationSimilarity(currentVector, cand.Embedding)
			// 写成 !(score >= 阈值) 让 NaN（维度不符/零向量）也落在排除侧；
			// 负相似度同样低于阈值，无需单独夹到 0（B6）。
			if !(score >= threadRelationSimilarityThreshold) {
				continue
			}
			pending = append(pending, pendingRelation{
				threadID:  ct.ThreadID,
				articleID: firstThreadRelationArticleID(threadRelationUintIDs(cand.ArticleIDs)),
				relation: ThreadRelation{
					ThreadID:   cand.ThreadID,
					Title:      cand.Title,
					Summary:    cand.Summary,
					PeriodDate: threadRelationDay(cand.PeriodDate).Format(threadRelationDateLayout),
					Score:      score,
					Level:      threadRelationLevel(score),
					Confirmed:  confirmedThreads[cand.ThreadID],
				},
			})
		}
	}

	// Source articles: the first related_article_ids entry of each recommended
	// thread, resolved in one batch (unresolvable → source_article null).
	articleIDs := make([]uint, 0, len(pending))
	seenArticle := map[uint]bool{}
	for _, p := range pending {
		if p.articleID == 0 || seenArticle[p.articleID] {
			continue
		}
		seenArticle[p.articleID] = true
		articleIDs = append(articleIDs, p.articleID)
	}
	articleMeta, err := r.threadRelationArticleMetadata(articleIDs)
	if err != nil {
		return nil, err
	}

	for _, p := range pending {
		rel := p.relation
		if meta, ok := articleMeta[p.articleID]; ok {
			rel.SourceArticle = &ThreadRelationSourceArticle{
				ID:          meta.ID,
				Title:       meta.Title,
				Feed:        meta.FeedTitle,
				PublishedAt: models.FormatDatetimeCSTPtr(meta.PubDate),
			}
		}
		resp.Threads[p.threadID] = append(resp.Threads[p.threadID], rel)
		resp.Total++
	}
	for threadID := range resp.Threads {
		sortThreadRelations(resp.Threads[threadID])
	}

	resp.Status = threadRelationStatus(resp.Total, resp.SkippedNoEmbedding)
	return resp, nil
}

// threadRelationStatus derives the three-state status (design D6). An empty
// result is `empty` even when threads were skipped for missing vectors — the
// empty branch has no recommendations to be partial about.
func threadRelationStatus(total, skipped int) string {
	switch {
	case total == 0:
		return ThreadRelationStatusEmpty
	case skipped > 0:
		return ThreadRelationStatusPartial
	default:
		return ThreadRelationStatusReady
	}
}

// sortThreadRelations orders one thread's recommendations by score desc →
// period_date desc → thread_id asc. thread_id is unique, so the order is a
// strict total order (two identical queries always agree).
func sortThreadRelations(relations []ThreadRelation) {
	sort.SliceStable(relations, func(i, j int) bool {
		a, b := relations[i], relations[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.PeriodDate != b.PeriodDate {
			return a.PeriodDate > b.PeriodDate
		}
		return a.ThreadID < b.ThreadID
	})
}

// threadRelationConfirmedCandidates marks the candidate threads whose event
// tags have a human-confirmed key-event marker on their topic (design D5).
// Read-only: markers keep their state. A tag id missing from topic_tags (or not
// an event tag) never confirms — keyword/person tags must not earn a star.
func threadRelationConfirmedCandidates(
	candidates []threadRelationCandidate,
	eventTagIDs map[uint]struct{},
	confirmedTags map[uint]map[uint]struct{},
) map[uint]bool {
	out := make(map[uint]bool, len(candidates))
	for _, cand := range candidates {
		topicTags := confirmedTags[cand.PersistentTopicID]
		if len(topicTags) == 0 {
			continue
		}
		for _, id := range threadRelationUintIDs(cand.TagIDs) {
			if _, isEvent := eventTagIDs[id]; !isEvent {
				continue
			}
			if _, confirmed := topicTags[id]; confirmed {
				out[cand.ThreadID] = true
				break
			}
		}
	}
	return out
}

// threadRelationCurrentThreads loads the report's own threads with their
// section's topic and vector. Body-only, mirroring the report read path.
func (r *TopicGraphRepository) threadRelationCurrentThreads(reportID uint) ([]threadRelationCurrentThread, error) {
	var out []threadRelationCurrentThread
	err := r.db.Raw(`
		SELECT t.id AS thread_id, s.persistent_topic_id,
		       COALESCE(t.embedding::text, '') AS embedding
		FROM daily_report_threads t
		JOIN daily_report_sections s ON s.id = t.section_id
		WHERE t.report_id = ? AND t.display_tier = ?
		ORDER BY t.id ASC
	`, reportID, ThreadDisplayTierBody).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("thread relations: load current threads: %w", err)
	}
	return out, nil
}

// threadRelationCandidateRow is the raw scan target for window threads.
type threadRelationCandidateRow struct {
	ThreadID          uint
	PersistentTopicID uint
	PeriodDate        time.Time
	Title             string
	Summary           string
	TagIDs            JSON
	RelatedArticleIDs JSON
	Embedding         string
}

// threadRelationCandidateRows loads window threads under the current report's
// topics. Sections converge first (topic + period_date), threads follow by
// section id — never a global vector/jsonb scan. Threads without a vector are
// filtered out in SQL (they can never be candidates, B8).
func (r *TopicGraphRepository) threadRelationCandidateRows(topicIDs []uint, reportID uint, windowStart, windowEnd time.Time) ([]threadRelationCandidate, error) {
	var rows []threadRelationCandidateRow
	err := r.db.Raw(`
		SELECT t.id AS thread_id, s.persistent_topic_id, r.period_date,
		       t.title, t.summary, t.tag_ids, t.related_article_ids,
		       t.embedding::text AS embedding
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		JOIN daily_report_threads t ON t.section_id = s.id
		WHERE s.persistent_topic_id IN ?
		  AND r.period_date >= ? AND r.period_date <= ?
		  AND s.report_id <> ?
		  AND t.display_tier = ?
		  AND t.embedding IS NOT NULL
		ORDER BY r.period_date DESC, t.id ASC
	`, topicIDs, windowStart, windowEnd, reportID, ThreadDisplayTierBody).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("thread relations: load candidate threads: %w", err)
	}

	out := make([]threadRelationCandidate, 0, len(rows))
	for _, row := range rows {
		vector := threadRelationParseVector(row.Embedding)
		if len(vector) == 0 {
			continue // 向量不可用：不进候选池（与 NULL 同等对待）
		}
		out = append(out, threadRelationCandidate{
			ThreadID:          row.ThreadID,
			PersistentTopicID: row.PersistentTopicID,
			PeriodDate:        row.PeriodDate,
			Title:             row.Title,
			Summary:           row.Summary,
			TagIDs:            row.TagIDs,
			ArticleIDs:        row.RelatedArticleIDs,
			Embedding:         vector,
		})
	}
	return out, nil
}

// threadRelationEventTagIDs returns the candidate tag ids that still exist and
// are event tags. Used only for the confirmed star: the candidate set never
// depends on tag rows surviving the orphan cleanup.
func (r *TopicGraphRepository) threadRelationEventTagIDs(candidates []threadRelationCandidate) (map[uint]struct{}, error) {
	seen := map[uint]struct{}{}
	ids := make([]uint, 0, len(candidates))
	for _, cand := range candidates {
		for _, id := range threadRelationUintIDs(cand.TagIDs) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	out := make(map[uint]struct{}, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []uint
	err := r.db.Model(&models.TopicTag{}).
		Where("id IN ? AND category = ?", ids, models.TagCategoryEvent).
		Pluck("id", &rows).Error
	if err != nil {
		return nil, fmt.Errorf("thread relations: load candidate event tags: %w", err)
	}
	for _, id := range rows {
		out[id] = struct{}{}
	}
	return out, nil
}

// threadRelationConfirmedTags loads the confirmed key-event markers of the
// given topics and returns, per topic, the tag ids their frozen
// tag_ids_snapshot carries. Read-only: markers are never touched.
func (r *TopicGraphRepository) threadRelationConfirmedTags(topicIDs []uint) (map[uint]map[uint]struct{}, error) {
	out := make(map[uint]map[uint]struct{}, len(topicIDs))
	if len(topicIDs) == 0 {
		return out, nil
	}
	type row struct {
		PersistentTopicID uint
		TagIDsSnapshot    JSON
	}
	var rows []row
	err := r.db.Model(&TopicKeyEventMarker{}).
		Select("persistent_topic_id, tag_ids_snapshot").
		Where("persistent_topic_id IN ? AND state = ?", topicIDs, KeyEventStateConfirmed).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("thread relations: load confirmed markers: %w", err)
	}
	for _, row := range rows {
		tags, ok := out[row.PersistentTopicID]
		if !ok {
			tags = map[uint]struct{}{}
			out[row.PersistentTopicID] = tags
		}
		for _, id := range threadRelationUintIDs(row.TagIDsSnapshot) {
			tags[id] = struct{}{}
		}
	}
	return out, nil
}

// threadRelationArticleMeta is the batch-loaded article metadata we need.
type threadRelationArticleMeta struct {
	ID        uint
	Title     string
	FeedTitle string
	PubDate   *time.Time
}

// threadRelationArticleMetadata batch-loads title/feed/published_at for the
// source articles (archived rows stay readable — 日报引用豁免 archived 过滤).
func (r *TopicGraphRepository) threadRelationArticleMetadata(articleIDs []uint) (map[uint]threadRelationArticleMeta, error) {
	out := make(map[uint]threadRelationArticleMeta, len(articleIDs))
	if len(articleIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID        uint
		Title     string
		FeedTitle *string
		PubDate   *time.Time
	}
	var rows []row
	err := r.db.Raw(`
		SELECT a.id, a.title, f.title AS feed_title, a.pub_date
		FROM articles a
		LEFT JOIN feeds f ON f.id = a.feed_id
		WHERE a.id IN ?
	`, articleIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("thread relations: load source articles: %w", err)
	}
	for _, row := range rows {
		meta := threadRelationArticleMeta{ID: row.ID, Title: row.Title, PubDate: row.PubDate}
		if row.FeedTitle != nil {
			meta.FeedTitle = *row.FeedTitle
		}
		out[row.ID] = meta
	}
	return out, nil
}

// firstThreadRelationArticleID returns the first referenced article (design
// D5: 来源文章取 related_article_ids 首篇). 0 means "no article".
func firstThreadRelationArticleID(articleIDs []uint) uint {
	for _, id := range articleIDs {
		if id != 0 {
			return id
		}
	}
	return 0
}

// threadRelationUintIDs parses a JSONB array of ids, tolerating the legacy
// string-encoded form (`["12"]`).
func threadRelationUintIDs(raw JSON) []uint {
	if len(raw) == 0 {
		return nil
	}
	var nums []uint
	if err := json.Unmarshal(raw, &nums); err == nil {
		return nums
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err != nil {
		return nil
	}
	out := make([]uint, 0, len(strs))
	for _, s := range strs {
		if v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32); err == nil {
			out = append(out, uint(v))
		}
	}
	return out
}
