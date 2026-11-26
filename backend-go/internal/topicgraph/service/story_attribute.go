package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// Story attribution orchestration — add-event-story-heat batch 2c wiring
// half (tasks 3.2/3.3/3.4/3.5). The pure decisions were built in 2b:
// ParseEventFingerprint (fingerprint parse + trust), ResolveSubjectKey
// (three-tier subject normalization), DecideAssignment (five-tier rule/LLM
// verdict). This file fetches real rows through a storyStore, feeds those
// pure functions, then persists the verdict (attach / merge / create).
//
// 红线（pinned 约束，全部继承不得重写）：
//   - subject 否决优先：DecideAssignment 已内置预筛，这里不再加任何
//     「untrusted 先送 LLM」短路；
//   - ROUNDUP 保守新建：1 个 event 标签但 LLM 判 ROUNDUP 时按新建事件线
//     处理（origin=model）；只有 tag 数 ≥2 的 composite 才不建身份；
//   - 人工保护：story_manual=true 的线程跳过（查询层已过滤，循环内防御）；
//     合并只移动非 manual 线程（仓储层负责）；
//   - 幂等：已归属/人工线程不处理；召回失败保持 story_id=NULL 可重试；
//   - 单条失败 log+continue，绝不中断整个 board 的归属。

// storyStore is the persistence surface the attribution needs. The
// production adapter wraps repository.Repo; tests inject a fake via
// storyStoreForTest — no repository.Repo global needed for unit tests.
type storyStore interface {
	ListUnassignedThreads(reportID uint) ([]repository.DailyReportThread, error)
	GetSectionTopicIDs(reportID uint) (map[uint]uint, error)
	GetEventTags(tagIDs []uint) ([]models.TopicTag, error)
	SetTagFingerprintTrusted(tagID uint, trusted bool) error
	RecallStoryCandidates(boardID uint, vector string, since time.Time, limit int) ([]repository.StoryCandidate, error)
	ListRecentStoryCandidates(boardID uint, since time.Time, limit int) ([]repository.StoryCandidate, error)
	GetStoryArticleIDs(storyIDs []uint) (map[uint][]uint, error)
	EarliestArticleAt(articleIDs []uint) (time.Time, error)
	CreateBoardStory(story *repository.BoardStory) error
	AttachThreadToBoardStory(reportID, threadID, storyID uint, isDevelopment bool) error
	MergeBoardStories(fromID, intoID uint) (uint, error)
	ResolveBoardStoryID(id uint) (uint, error)
	RecomputeBoardStoryCounts(storyID uint) error
}

// repoStoryStore adapts repository.Repo to storyStore (plain delegation).
type repoStoryStore struct {
	repo *repository.TopicGraphRepository
}

func (s repoStoryStore) ListUnassignedThreads(reportID uint) ([]repository.DailyReportThread, error) {
	return s.repo.ListUnassignedThreads(reportID)
}

func (s repoStoryStore) GetSectionTopicIDs(reportID uint) (map[uint]uint, error) {
	return s.repo.GetSectionTopicIDs(reportID)
}

func (s repoStoryStore) GetEventTags(tagIDs []uint) ([]models.TopicTag, error) {
	return s.repo.GetEventTags(tagIDs)
}

func (s repoStoryStore) SetTagFingerprintTrusted(tagID uint, trusted bool) error {
	return s.repo.SetTagFingerprintTrusted(tagID, trusted)
}

func (s repoStoryStore) RecallStoryCandidates(boardID uint, vector string, since time.Time, limit int) ([]repository.StoryCandidate, error) {
	return s.repo.RecallStoryCandidates(boardID, vector, since, limit)
}

func (s repoStoryStore) ListRecentStoryCandidates(boardID uint, since time.Time, limit int) ([]repository.StoryCandidate, error) {
	return s.repo.ListRecentStoryCandidates(boardID, since, limit)
}

func (s repoStoryStore) GetStoryArticleIDs(storyIDs []uint) (map[uint][]uint, error) {
	return s.repo.GetStoryArticleIDs(storyIDs)
}

func (s repoStoryStore) EarliestArticleAt(articleIDs []uint) (time.Time, error) {
	return s.repo.EarliestArticleAt(articleIDs)
}

func (s repoStoryStore) CreateBoardStory(story *repository.BoardStory) error {
	return s.repo.CreateBoardStory(story)
}

func (s repoStoryStore) AttachThreadToBoardStory(reportID, threadID, storyID uint, isDevelopment bool) error {
	return s.repo.AttachThreadToBoardStory(reportID, threadID, storyID, isDevelopment)
}

func (s repoStoryStore) MergeBoardStories(fromID, intoID uint) (uint, error) {
	return s.repo.MergeBoardStories(fromID, intoID)
}

func (s repoStoryStore) ResolveBoardStoryID(id uint) (uint, error) {
	return s.repo.ResolveBoardStoryID(id)
}

func (s repoStoryStore) RecomputeBoardStoryCounts(storyID uint) error {
	return s.repo.RecomputeBoardStoryCounts(storyID)
}

// storyStoreForTest swaps the whole store for unit tests (nil = production).
var storyStoreForTest storyStore

func activeStoryStore() storyStore {
	if storyStoreForTest != nil {
		return storyStoreForTest
	}
	return repoStoryStore{repo: repository.Repo}
}

// Injectables (test seams; same convention as keyEventRecommendChatFn /
// laneSnapshotChatFn).
var (
	// storyAttributeClassifierFactory yields the four-way LLM classifier
	// (nil classifier would degrade every undecidable case to a new story —
	// DecideAssignment treats nil conservatively).
	storyAttributeClassifierFactory = NewStoryClassifier
	// storySubjectEmbedder embeds a subject_key for the normalization tier /
	// new story lines (design §4: 只为已建立的事件线计算一次，不逐篇算).
	storySubjectEmbedder = defaultStorySubjectEmbedder
	// storyTitleEmbedder embeds an event title into the story-line recall
	// vector (enable-story-cross-day-continuation D13: 建线必须留下向量).
	// A failure is NOT fatal — the caller only warns and the story line is
	// still created with a NULL embedding (today's behavior).
	storyTitleEmbedder = defaultStoryTitleEmbedder
)

// defaultStorySubjectEmbedder is the production embed call: one input, one
// operation (topicgraph.story_subject_embedding), CapabilityEmbedding. A
// failure returns the error — ResolveSubjectKey degrades to literal/new and
// the new-story path retries at most once.
func defaultStorySubjectEmbedder(subjectKey string) ([]float64, error) {
	return storySubjectEmbedWithContext(context.Background(), subjectKey)
}

var storySubjectEmbedWithContext = func(parent context.Context, subjectKey string) ([]float64, error) {
	key := strings.TrimSpace(subjectKey)
	if key == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	result, err := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{key},
		Operation: "topicgraph.story_subject_embedding",
		Metadata: map[string]any{
			"operation": "topicgraph_story_subject_embedding",
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, errors.New("story subject embedding: empty result")
	}
	return result.Embeddings[0], nil
}

// defaultStoryTitleEmbedder is the production embed call for an event title:
// one input, one operation (topicgraph.story_title_embedding),
// CapabilityEmbedding, 30s timeout — the same shape as the subject embedder.
func defaultStoryTitleEmbedder(title string) ([]float64, error) {
	return EmbedStoryTitle(context.Background(), title)
}

// EmbedStoryTitle embeds one event title with the story-line title vector
// operation. Exported so cmd/backfill-stories --fill-story-embeddings shares
// the exact same operation name / model route / timeout as the online path
// (D13: 存量回填与建线现算必须同源，否则两批向量不在同一空间)。
func EmbedStoryTitle(ctx context.Context, title string) ([]float64, error) {
	text := strings.TrimSpace(title)
	if text == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{text},
		Operation: "topicgraph.story_title_embedding",
		Metadata: map[string]any{
			"operation": "topicgraph_story_title_embedding",
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, errors.New("story title embedding: empty result")
	}
	return result.Embeddings[0], nil
}

// attributeStoriesForReport attributes the report's unassigned threads to
// story lines (task 3.5's core). Idempotent by construction: only
// story_id-IS-NULL non-manual threads are loaded, and every attach/merge is
// guarded at the row level. A per-thread failure logs and continues (the
// thread keeps story_id NULL and is retried on the next run); only a
// ListUnassignedThreads failure aborts.
func attributeStoriesForReport(boardID, reportID uint, periodDate time.Time) error {
	_, err := attributeStoriesForReportWithStats(context.Background(), activeStoryStore(), boardID, reportID, periodDate, false)
	return err
}

// attributeStoriesForReportWithStats shares the production decision path with
// backfill. Dry runs stop before persistence (including fingerprint trust).
func attributeStoriesForReportWithStats(ctx context.Context, store storyStore, boardID, reportID uint, periodDate time.Time, dryRun bool, online ...bool) (BackfillStats, error) {
	stats := BackfillStats{DryRun: dryRun}
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	threads, err := store.ListUnassignedThreads(reportID)
	if err != nil {
		return stats, fmt.Errorf("story-attribute: list unassigned threads for report %d: %w", reportID, err)
	}
	if len(threads) == 0 {
		return stats, nil
	}

	// lane_id lookup for new story lines (首次归属话题，可空) — one query per
	// report, best-effort (a failure leaves new stories without a lane).
	sectionTopics := map[uint]uint{}
	if m, err := store.GetSectionTopicIDs(reportID); err != nil {
		if len(online) > 0 && online[0] {
			return stats, err
		}
		logging.Warnf("story-attribute: section topic lookup for report %d failed (new stories get no lane): %v", reportID, err)
	} else {
		sectionTopics = m
	}

	since := periodDate.AddDate(0, 0, -repository.StoryRecallWindowDays)
	for i := range threads {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		thread := threads[i]
		stats.Threads++
		if thread.StoryManual || thread.StoryID != nil || thread.Scope == "composite" {
			stats.Skipped++
			continue
		}
		if err := attributeOneThread(store, boardID, reportID, periodDate, since, sectionTopics, thread, storyAttributionRun{ctx: ctx, dryRun: dryRun, stats: &stats, strict: len(online) > 0 && online[0]}); err != nil {
			stats.Failed++
			if len(online) > 0 && online[0] {
				return stats, err
			}
			logging.Warnf("story-attribute: thread %d left unassigned (will retry next run): %v", thread.ID, err)
		}
	}
	return stats, ctx.Err()
}

// attributeOneThread runs the full attribution flow for ONE thread:
// composite interception → fingerprint → recall → subject normalization →
// five-tier decision → attach / merge / create.
type storyAttributionRun struct {
	ctx    context.Context
	dryRun bool
	stats  *BackfillStats
	strict bool
}

func attributeOneThread(store storyStore, boardID, reportID uint, periodDate, since time.Time, sectionTopics map[uint]uint, thread repository.DailyReportThread, runs ...storyAttributionRun) (err error) {
	// Keep the existing single-thread entry point for production/test callers;
	// the report runner supplies cancellation, dry-run and shared counters.
	run := storyAttributionRun{ctx: context.Background(), stats: &BackfillStats{}}
	if len(runs) > 0 {
		run = runs[0]
	}
	ctx, dryRun, stats := run.ctx, run.dryRun, run.stats
	if err := ctx.Err(); err != nil {
		return err
	}
	var dependencyErr error
	embed := storySubjectEmbedder
	if run.strict {
		embed = func(key string) ([]float64, error) {
			vec, err := storySubjectEmbedWithContext(ctx, key)
			if err != nil {
				dependencyErr = err
			}
			return vec, err
		}
	}
	// Counters describe successful verdicts, not partial writes on error.
	created, attached, merged := 0, 0, 0
	defer func() {
		if errors.Is(err, repository.ErrStoryThreadNotAttachable) {
			stats.Skipped++
			err = nil
			return
		}
		if err == nil {
			stats.Created += created
			stats.Attached += attached
			stats.Merged += merged
		}
	}()
	tagIDs := jsonUintIDsExported(thread.TagIDs)
	var eventTags []models.TopicTag
	if len(tagIDs) > 0 {
		var err error
		eventTags, err = store.GetEventTags(tagIDs)
		if err != nil {
			// Cannot even tell composite from single — leave the thread NULL
			// (retry next run) rather than mislabeling it.
			return fmt.Errorf("load event tags: %w", err)
		}
	}

	// Composite interception (task 3.2): >=2 event tags → 综合稿，不建身份.
	// scope itself was already derived at SaveThreads; this is the attribution
	// side of the same rule.
	if len(eventTags) >= 2 {
		stats.Skipped++
		return nil
	}

	input := AssignmentInput{
		ThreadID:   thread.ID,
		PeriodDate: periodDate,
		Title:      thread.Title,
		Summary:    thread.Summary,
		ArticleIDs: jsonUintIDsExported(thread.RelatedArticleIDs),
	}
	if thread.Embedding != "" {
		if vec, perr := parsePgVector(thread.Embedding); perr == nil {
			input.Embedding = vec
		}
	}
	if len(eventTags) == 1 {
		tag := eventTags[0]
		fp, _ := ParseEventFingerprint(tag)
		// Persist the trust verdict (task 2.4 调用侧) — best-effort only:
		// a failed write warns and never blocks attribution.
		if !dryRun {
			if terr := store.SetTagFingerprintTrusted(tag.ID, fp.Trusted); terr != nil {
				if run.strict {
					return terr
				}
				logging.Warnf("story-attribute: persist fingerprint_trusted for tag %d failed: %v", tag.ID, terr)
			}
		}
		input.Subject = fp.Subject
		input.SubjectKey = fp.SubjectKey
		input.Action = fp.Action
		input.Object = fp.Object
		input.OccurredAt = fp.OccurredAt
		input.FingerprintTrusted = fp.Trusted
	}
	// len(eventTags)==0 → untrusted, empty identity fields (zero values):
	// the LLM tier decides over whatever survives the subject pre-filter.

	candidates, subjectCands, err := recallCandidates(store, boardID, thread, since, input.ArticleIDs)
	if err != nil {
		// Recall failed — keep the thread NULL; the next run retries.
		return fmt.Errorf("recall candidates: %w", err)
	}

	// Subject normalization (design §4): the input key may be rewritten to
	// the matched candidate's canonical key (embedding/literal tiers) so the
	// five-tier rules and any NEW story line share one identity namespace.
	match := ResolveSubjectKey(SubjectInput{Subject: input.Subject, SubjectKey: input.SubjectKey}, subjectCands, embed)
	if match.SubjectKey != "" {
		input.SubjectKey = match.SubjectKey
	}

	if dependencyErr != nil {
		return dependencyErr
	}
	classifier := storyAttributeClassifierFactory()
	if run.strict && classifier != nil {
		base := classifier
		classifier = func(ctx context.Context, input AssignmentInput, candidates []StoryCandidate) (uint, StoryDecision, float64, string, error) {
			id, decision, confidence, reason, err := base(ctx, input, candidates)
			if err != nil {
				dependencyErr = err
			}
			return id, decision, confidence, reason, err
		}
	}
	result := DecideAssignment(ctx, input, candidates, classifier)
	if dependencyErr != nil {
		return dependencyErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch len(result.StoryIDs) {
	case 0:
		created = 1
	case 1:
		attached = 1
	default:
		merged, attached = 1, 1
	}
	if dryRun {
		return nil
	}

	switch {
	case len(result.StoryIDs) > 1:
		// Multiple SAME_OCCURRENCE hits: the candidates are duplicate story
		// lines of one occurrence — merge them into the deterministic
		// survivor (smallest id), then attach.
		survivor := minUintID(result.StoryIDs)
		for _, id := range result.StoryIDs {
			if id == survivor {
				continue
			}
			merged, merr := store.MergeBoardStories(id, survivor)
			if merr != nil {
				return fmt.Errorf("merge story %d into %d: %w", id, survivor, merr)
			}
			survivor = merged
		}
		return attachThreadToStory(store, reportID, thread.ID, survivor, result.IsDevelopment)
	case len(result.StoryIDs) == 1:
		return attachThreadToStory(store, reportID, thread.ID, result.StoryIDs[0], result.IsDevelopment)
	default:
		// No hit — new story line. UNRELATED and ROUNDUP both create here
		// (spec conservative: 宁可新建不可错并; ROUNDUP with a single event
		// tag still gets an identity, origin=model).
		if run.strict {
			return createStoryForThread(store, boardID, reportID, periodDate, sectionTopics, thread, input, result, match, embed)
		}
		return createStoryForThread(store, boardID, reportID, periodDate, sectionTopics, thread, input, result, match)
	}
}

// recallCandidates fetches the candidate story lines for a thread.
//
// With an embedding (regular threads): pgvector recall — wide net, precision
// is DecideAssignment's job. Without one (watch-materialized threads, design
// §9 降级 3): list recent candidates and keep only those sharing at least
// one article with the thread (article-set overlap > 0; no overlap → the
// thread will found a new story line).
func recallCandidates(store storyStore, boardID uint, thread repository.DailyReportThread, since time.Time, threadArticleIDs []uint) ([]StoryCandidate, []SubjectCandidate, error) {
	var recalled []repository.StoryCandidate
	if thread.Embedding != "" {
		var err error
		recalled, err = store.RecallStoryCandidates(boardID, thread.Embedding, since, repository.StoryRecallLimit)
		if err != nil {
			return nil, nil, err
		}
	} else {
		listed, err := store.ListRecentStoryCandidates(boardID, since, repository.StoryRecallLimit)
		if err != nil {
			return nil, nil, err
		}
		if len(listed) > 0 {
			ids := make([]uint, 0, len(listed))
			for _, c := range listed {
				ids = append(ids, c.ID)
			}
			articleMap, aerr := store.GetStoryArticleIDs(ids)
			if aerr != nil {
				return nil, nil, aerr
			}
			recalled = make([]repository.StoryCandidate, 0, len(listed))
			for _, c := range listed {
				if hasArticleOverlap(threadArticleIDs, articleMap[c.ID]) {
					articles := articleMap[c.ID]
					c.ArticleIDs = articles
					recalled = append(recalled, c)
				}
			}
		}
	}
	candidates := make([]StoryCandidate, 0, len(recalled))
	subjectCands := make([]SubjectCandidate, 0, len(recalled))
	for _, rc := range recalled {
		candidates = append(candidates, StoryCandidate{
			ID:         rc.ID,
			Title:      rc.Title,
			Summary:    rc.Summary,
			Subject:    rc.Subject,
			SubjectKey: rc.SubjectKey,
			Action:     rc.Action,
			Object:     rc.Object,
			OccurredAt: rc.OccurredAt,
			PeriodDate: rc.PeriodDate,
			ArticleIDs: rc.ArticleIDs,
			Distance:   rc.Distance,
			Origin:     rc.Origin,
		})
		subjectCands = append(subjectCands, SubjectCandidate{
			StoryID:          rc.ID,
			Subject:          rc.Subject,
			SubjectKey:       rc.SubjectKey,
			SubjectEmbedding: rc.SubjectEmbedding,
		})
	}
	return candidates, subjectCands, nil
}

// hasArticleOverlap reports whether the two article sets share at least one
// id (design §9: 无 embedding 只走文章集合重叠，无重叠则新建).
func hasArticleOverlap(a, b []uint) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[uint]bool, len(a))
	for _, id := range a {
		set[id] = true
	}
	for _, id := range b {
		if set[id] {
			return true
		}
	}
	return false
}

// attachThreadToStory links the thread. The caller counts the not-attachable
// sentinel as a benign skip (idempotent re-run / human-protected thread).
func attachThreadToStory(store storyStore, reportID, threadID, storyID uint, isDevelopment bool) error {
	err := store.AttachThreadToBoardStory(reportID, threadID, storyID, isDevelopment)
	if errors.Is(err, repository.ErrStoryThreadNotAttachable) {
		logging.Infof("story-attribute: thread %d skipped attach (already attributed or human-protected)", threadID)
		return err
	}
	return err
}

// createStoryForThread founds a new story line for the thread, then attaches
// it (attach recomputes counts — create+attach must not double-count).
func createStoryForThread(store storyStore, boardID, reportID uint, periodDate time.Time, sectionTopics map[uint]uint, thread repository.DailyReportThread, input AssignmentInput, result AssignmentResult, match SubjectMatch, embedders ...SubjectEmbedFunc) error {
	story := &repository.BoardStory{
		BoardID:       boardID,
		Title:         thread.Title,
		Subject:       input.Subject,
		SubjectKey:    input.SubjectKey,
		Action:        input.Action,
		Object:        input.Object,
		Status:        repository.StoryStatusActive,
		FirstSeenDate: periodDate,
		LastSeenDate:  periodDate,
		Origin:        result.Origin,
		Embedding:     thread.Embedding, // blank stays NULL unless the title fallback below fills it
	}
	// D13 召回盲区：建线必须留下向量 —— thread 没有向量时用事件标题现算一次
	// embedding 写入 board_stories.embedding。现算失败只记 warning、不阻断
	// 建线（story 仍以 NULL 建，与今天行为一致）。召回口径不变（仍只算
	// embedding <=> ?、dist ≤ 0.5 top10，不引入新召回面）。
	if thread.Embedding == "" && strings.TrimSpace(thread.Title) != "" {
		vec, terr := storyTitleEmbedder(thread.Title)
		switch {
		case terr != nil:
			logging.Warnf("story-attribute: title embedding failed for thread %d (story keeps NULL embedding): %v", thread.ID, terr)
		case len(vec) > 0:
			story.Embedding = repository.FloatsToPgVector(vec)
		}
	}
	// occurred_at: fingerprint first, else earliest related article.
	if input.OccurredAt != "" {
		if t, perr := parseStoryOccurredAt(input.OccurredAt); perr == nil {
			story.OccurredAt = &t
		}
	}
	if story.OccurredAt == nil && len(input.ArticleIDs) > 0 {
		if t, _ := store.EarliestArticleAt(input.ArticleIDs); !t.IsZero() {
			story.OccurredAt = &t
		}
	}
	// subject_embedding: reuse the vector ResolveSubjectKey already computed;
	// only when the resolve produced none (new subject, no usable candidates)
	// spend exactly one embed call — never per article (design §4).
	subjectEmb := match.Embedding
	if len(subjectEmb) == 0 && match.Method == SubjectMethodNew && story.SubjectKey != "" {
		embed := storySubjectEmbedder
		if len(embedders) > 0 {
			embed = embedders[0]
		}
		if v, eerr := embed(story.SubjectKey); eerr == nil {
			subjectEmb = v
		} else if len(embedders) > 0 {
			return eerr
		}
	}
	if len(subjectEmb) > 0 {
		story.SubjectEmbedding = repository.FloatsToPgVector(subjectEmb)
	}
	// lane_id: the thread's section's persistent topic (首次归属，可空).
	if topicID, ok := sectionTopics[thread.SectionID]; ok {
		story.LaneID = &topicID
	}

	if err := store.CreateBoardStory(story); err != nil {
		return fmt.Errorf("create story: %w", err)
	}
	return attachThreadToStory(store, reportID, thread.ID, story.ID, false)
}

// parseStoryOccurredAt parses a fingerprint occurred_at string; accepts the
// common date/datetime shapes (LLM output is a free-form date field).
func parseStoryOccurredAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006/01/02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable occurred_at %q", raw)
}

func minUintID(ids []uint) uint {
	min := ids[0]
	for _, id := range ids[1:] {
		if id < min {
			min = id
		}
	}
	return min
}

// ── Detached spawn (task 3.5 调度接入) ─────────────────────────────────────
//
// Mirrors spawnLaneSnapshotSettlement / spawnKeyEventRecommendation: a
// detached, fully-recovered goroutine after the report's terminal work —
// attribution failure/panic NEVER affects the report result. Unattributed
// threads keep story_id NULL and retry on the next report day.

// spawnAttributeStories launches the attribution as a detached goroutine.
// Injectable so tests can run it synchronously.
var spawnAttributeStories = func(boardID, reportID uint, periodDate time.Time) {
	go attributeStoriesSafe(boardID, reportID, periodDate)
}

// attributeStoriesSafe runs the attribution with a full recover: any panic is
// swallowed and logged (松耦合纪律 — same contract as settleLaneSnapshotsSafe).
// Cross-period dedup (tasks 6.1-6.3) runs right after attribution inside this
// same goroutine — even when attribution fails partway, threads already
// attributed (this run or an earlier one) still get their display tier
// recomputed; dedup failure stays a warning (display layer only).
func attributeStoriesSafe(boardID, reportID uint, periodDate time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("story-attribute: attribution panicked for board %d report %d: %v", boardID, reportID, r)
		}
	}()
	if err := attributeStoriesForReport(boardID, reportID, periodDate); err != nil {
		logging.Warnf("story-attribute: attribution for board %d report %d failed: %v", boardID, reportID, err)
	}
	runStoryDedupSafe(boardID, reportID, periodDate)
}

// SetAttributeStoriesSpawnForTest swaps the spawn hook and returns a restore
// function (same shape as SetKeyEventRecommendationSpawnForTest).
func SetAttributeStoriesSpawnForTest(spawn func(boardID, reportID uint, periodDate time.Time)) func() {
	orig := spawnAttributeStories
	spawnAttributeStories = spawn
	return func() { spawnAttributeStories = orig }
}

// SpawnAttributeStoriesForReport retains the detached attribution entry point.
// Online daily reports no longer attribute at all (review R-1: hard stop), and
// this helper must not be dispatched again for the same generation.
func SpawnAttributeStoriesForReport(boardID, reportID uint, periodDate time.Time) {
	spawnAttributeStories(boardID, reportID, periodDate)
}
