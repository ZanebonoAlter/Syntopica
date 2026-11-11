package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

const storyHighlightsSystemPrompt = `你是一名新闻编辑。从本次给出的最多8条候选事件线中选最多3条不同事件线，通常2–3条，只有1条可选1条。按候选热度顺序优先选择，不能选择候选外的故事。story_title和热度只帮助选题，不是事实证据；title/reason只能引用该候选facts中的label/description/代表文章事实，禁止编造事件、数字、情绪或因果。事实不足可以不选，输出合法空数组。每项必填story_id（正整数）、title（非空事实标题）、reason（非空事实理由）、tag_ids（该故事facts内的标签ID）。只输出 {"highlights":[...]}，不要额外字段，不输出热度或来源数。`

func storyHighlightSchema() *airouter.JSONSchema {
	schema := highlightsJSONSchema()
	item := schema.Properties["highlights"].Items
	item.Properties["story_id"] = airouter.SchemaProperty{Type: "integer"}
	item.Required = append(item.Required, "story_id")
	return schema
}

func eligibleStoryFacts(candidates []repository.ReportStoryCandidate, tags []repository.TagInput) []repository.ReportStoryCandidate {
	out := make([]repository.ReportStoryCandidate, 0, len(candidates))
	valid := make(map[uint]bool, len(tags))
	for _, tag := range tags {
		valid[tag.ID] = true
	}
	for _, c := range candidates {
		ids := make([]uint, 0, len(c.TagIDs))
		for _, id := range c.TagIDs {
			if valid[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			c.TagIDs = ids
			out = append(out, c)
		}
	}
	return out
}

// maxStoryHighlightCandidates caps the story-mode prompt whitelist. The cap
// is applied AFTER fact eligibility (storyHighlightCandidates) so a fact-less
// candidate never occupies a slot.
const maxStoryHighlightCandidates = 8

// storyHighlightCandidates prepares the story-mode prompt set: it augments the
// clustering-pinned fact input with the candidate threads' own event-tag facts
// — watch materialization resolves its tags AFTER inputs.tags is fixed, so
// materialized candidates would otherwise carry no legal facts and get
// dropped by eligibleStoryFacts — and only THEN truncates to the top-8 heat
// order: a candidate without usable facts never occupies a top-8 slot (the
// 9th candidate with facts backfills instead of degrading to tag mode).
func storyHighlightCandidates(tags []repository.TagInput, candidates []repository.ReportStoryCandidate, fetchEventTags func([]uint) ([]models.TopicTag, error)) ([]repository.TagInput, []repository.ReportStoryCandidate, error) {
	known := make(map[uint]bool, len(tags))
	for _, t := range tags {
		known[t.ID] = true
	}
	missing := make([]uint, 0)
	for _, c := range candidates {
		for _, id := range c.TagIDs {
			if !known[id] {
				known[id] = true
				missing = append(missing, id)
			}
		}
	}
	if len(missing) > 0 {
		rows, err := fetchEventTags(missing)
		if err != nil {
			return nil, nil, fmt.Errorf("load candidate event tags: %w", err)
		}
		for _, row := range rows {
			// Facts for materialized tags are label/description (the batch row);
			// representative-article context stays a clustering-input feature.
			tags = append(tags, repository.TagInput{
				ID: row.ID, Label: row.Label, Category: row.Category,
				Description: row.Description, Source: row.Source,
			})
		}
	}
	eligible := eligibleStoryFacts(candidates, tags)
	if len(eligible) > maxStoryHighlightCandidates {
		eligible = eligible[:maxStoryHighlightCandidates]
	}
	return tags, eligible, nil
}

func GenerateStoryHighlights(ctx context.Context, candidates []repository.ReportStoryCandidate, tags []repository.TagInput) ([]repository.Highlight, error) {
	if len(candidates) == 0 {
		return []repository.Highlight{}, nil
	}
	type candidateFacts struct {
		repository.ReportStoryCandidate
		Facts []repository.TagInput `json:"facts"`
	}
	facts := make([]candidateFacts, 0, len(candidates))
	for _, c := range candidates {
		facts = append(facts, candidateFacts{c, filterTagsByIDs(tags, c.TagIDs)})
	}
	prompt, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	callCtx := withHighlightsCallCounts(ctx, len(tags), len(candidates))
	schema := storyHighlightSchema()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		content, err := highlightsChatFn(callCtx, storyHighlightsSystemPrompt, string(prompt), schema, highlightsOperation)
		if err == nil {
			var highlights []repository.Highlight
			highlights, err = parseStoryHighlightsResponse(content, candidates)
			if err == nil {
				return highlights, nil
			}
		}
		lastErr = err
		logging.Warnf("daily-report: story highlights attempt %d/2 failed: %v", attempt+1, err)
	}
	return nil, lastErr
}

func parseStoryHighlightsResponse(content string, candidates []repository.ReportStoryCandidate) ([]repository.Highlight, error) {
	content = jsonutil.SanitizeLLMJSON(content)
	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("invalid story highlights JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var top struct {
		Highlights json.RawMessage `json:"highlights"`
	}
	if err := decoder.Decode(&top); err != nil {
		return nil, err
	}
	raw := bytes.TrimSpace(top.Highlights)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("story highlights must be an array")
	}
	var entries []struct {
		StoryID uint   `json:"story_id"`
		Title   string `json:"title"`
		Reason  string `json:"reason"`
		TagIDs  []uint `json:"tag_ids"`
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}
	byID := make(map[uint]repository.ReportStoryCandidate)
	for _, c := range candidates {
		byID[c.StoryID] = c
	}
	selected := make(map[uint]repository.Highlight)
	for _, h := range entries {
		c, ok := byID[h.StoryID]
		if !ok || h.StoryID == 0 {
			return nil, fmt.Errorf("story id outside candidate whitelist")
		}
		if _, duplicate := selected[h.StoryID]; duplicate {
			continue
		}
		title, reason := strings.TrimSpace(h.Title), strings.TrimSpace(h.Reason)
		allowed := make(map[uint]bool)
		for _, id := range c.TagIDs {
			allowed[id] = true
		}
		tags := make([]uint, 0)
		for _, id := range h.TagIDs {
			if allowed[id] {
				tags = appendUintSet(tags, []uint{id})
			}
		}
		if title == "" || reason == "" || len(tags) == 0 {
			return nil, fmt.Errorf("story highlight has no grounded facts or text")
		}
		selected[h.StoryID] = c.Highlight(title, reason, tags)
	}
	out := make([]repository.Highlight, 0, 3)
	// Model selects and writes; trusted candidate order remains the display order.
	for _, c := range candidates {
		if h, ok := selected[c.StoryID]; ok {
			out = append(out, h)
			if len(out) == 3 {
				break
			}
		}
	}
	return out, nil
}

const reportAttributionBudget = 60 * time.Second

// Synchronous on purpose: ctx reaches DB, embedding and classifier; there is
// no timeout-race goroutine that can outlive this report's attribution budget.
var attributeReportOnline = func(ctx context.Context, boardID, reportID uint, date time.Time) error {
	store := storyStore(repoStoryStore{repository.NewTopicGraphRepository(repository.Repo.DB().WithContext(ctx))})
	if storyStoreForTest != nil {
		store = storyStoreForTest
	}
	_, err := attributeStoriesForReportWithStats(ctx, store, boardID, reportID, date, false, true)
	return err
}

var reportAttributionTimeout = reportAttributionBudget

func boundedReportAttribution(ctx context.Context, report *repository.BoardDailyReport) (err error) {
	started := time.Now()
	budget, cancel := context.WithTimeout(ctx, reportAttributionTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("story attribution panic: %v", recovered)
		}
		logging.Infof("daily-report: attribution report=%d elapsed=%s timeout=%t error=%v", report.ID, time.Since(started), budget.Err() == context.DeadlineExceeded, err)
	}()
	err = attributeReportOnline(budget, report.SemanticBoardID, report.ID, report.PeriodDate)
	if err == nil {
		err = budget.Err()
	}
	return err
}

type reportHighlightInputsKey struct{}
type reportHighlightInputs struct {
	tags     []repository.TagInput
	clusters []repository.ClusterGroup
}

// All degradation decisions precede the sole highlights mode invocation.
func finalizeDailyReportHighlights(ctx context.Context, report *repository.BoardDailyReport, inputs *reportHighlightInputs) {
	attributionErr := boundedReportAttribution(ctx, report)
	_, dedupErr := applyStoryDedupForReport(report.ID, report.PeriodDate)
	tags, clusters := inputs.tags, inputs.clusters
	// storyTags augments the clustering-pinned input with the candidate
	// threads' own event-tag facts (story mode only); the tag fallback below
	// keeps the original clustering whitelist.
	var storyTags []repository.TagInput
	var candidates []repository.ReportStoryCandidate
	var candidateErr error
	if attributionErr == nil && dedupErr == nil {
		candidates, candidateErr = repository.Repo.ReportStoryCandidates(report.SemanticBoardID, report.ID, report.PeriodDate)
		if candidateErr == nil {
			storyTags, candidates, candidateErr = storyHighlightCandidates(tags, candidates, repository.Repo.GetEventTags)
		}
	}
	if attributionErr != nil || dedupErr != nil || candidateErr != nil {
		logging.Warnf("daily-report: tag highlights fallback report=%d attribution=%v dedup=%v candidates=%v", report.ID, attributionErr, dedupErr, candidateErr)
	}
	var highlights []repository.Highlight
	var err error
	if len(candidates) > 0 {
		highlights, err = GenerateStoryHighlights(ctx, candidates, storyTags)
	} else {
		highlights, err = GenerateHighlights(ctx, tags, clusters)
	}
	if err != nil {
		logging.Warnf("daily-report: highlights failed: %v", err)
		highlights = nil
	}
	if updated, err := repository.Repo.FinalizeReportHighlights(report, highlights); err != nil || !updated {
		logging.Warnf("daily-report: final highlights write skipped report=%d updated=%t error=%v", report.ID, updated, err)
	}
}
