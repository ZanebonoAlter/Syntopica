package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/repository"
)

var (
	embeddingService          *EmbeddingService
	embeddingServiceOnce      sync.Once
	embeddingQueueService     *EmbeddingQueueService
	embeddingQueueServiceOnce sync.Once
)

func GetEmbeddingService() *EmbeddingService {
	embeddingServiceOnce.Do(func() {
		embeddingService = NewEmbeddingService()
	})
	return embeddingService
}

func getEmbeddingQueueService() *EmbeddingQueueService {
	embeddingQueueServiceOnce.Do(func() {
		embeddingQueueService = NewEmbeddingQueueService(nil)
	})
	return embeddingQueueService
}

// legacyExtractTopics is the old heuristic-based extraction (for fallback)
func legacyExtractTopics(input ExtractionInput) []TopicTag {
	// Use the existing extractor.go logic
	rawTags := ExtractTopics(input)
	result := make([]TopicTag, len(rawTags))
	for i, t := range rawTags {
		category := NormalizeDisplayCategory(t.Kind, t.Category)
		result[i] = TopicTag{
			Label:    t.Label,
			Slug:     t.Slug,
			Category: category,
			Kind:     t.Kind, // Keep for backward compat
			Score:    t.Score,
		}
	}
	return result
}

// findOrCreateTag finds an existing tag or creates a new one
// Uses three-level matching: exact/alias → embedding similarity → create new
func findOrCreateTag(ctx context.Context, tag TopicTag, source string, articleContext string, articleID uint) (*models.TopicTag, error) {
	slug := Slugify(tag.Label)
	category := NormalizeDisplayCategory(tag.Kind, tag.Category)
	logging.Infof("findOrCreateTag: start label=%q slug=%q category=%s source=%s", tag.Label, slug, category, source)

	if cached, ok := GetTagCache().Get(slug, category); ok {
		logging.Infof("findOrCreateTag: label=%q slug=%q category=%s cache=hit existingID=%d", tag.Label, slug, category, cached.ID)
		return cached, nil
	}

	// Build aliases string for TagMatch
	aliases := tag.Aliases
	if len(aliases) == 0 {
		aliases = []string{}
	}
	aliasesJSON, _ := json.Marshal(aliases)

	var savedCandidates []TagCandidate

	// Try embedding-based three-level matching
	es := GetEmbeddingService()
	if es != nil {
		matchResult, err := es.TagMatch(ctx, tag.Label, category, string(aliasesJSON))
		if err != nil {
			// Embedding unavailable — fall back to exact match
			logging.Warnf("TagMatch failed, falling back to exact match: %v", err)
		} else {
			switch matchResult.MatchType {
			case "exact":
				if matchResult.ExistingTag != nil {
					logging.Infof("findOrCreateTag: label=%q category=%s matchType=exact existingID=%d existingLabel=%q", tag.Label, category, matchResult.ExistingTag.ID, matchResult.ExistingTag.Label)
					existing := matchResult.ExistingTag
					existing.Label = tag.Label
					newSlug := Slugify(tag.Label)
					if newSlug != "" {
						existing.Slug = newSlug
					}
					existing.Category = category
					existing.Source = source
					if tag.Icon != "" {
						existing.Icon = tag.Icon
					}
					if len(tag.Aliases) > 0 {
						aJSON, _ := json.Marshal(tag.Aliases)
						existing.Aliases = string(aJSON)
					}
					if err := repository.Repo.DB().Save(existing).Error; err != nil {
						return nil, err
					}
					if category != "event" {
						go ensureTagEmbedding(es, existing.ID)
					}
					GetTagCache().Set(slug, category, existing)
					return existing, nil
				}

			case "candidates":
				logging.Infof("findOrCreateTag: label=%q category=%s matchType=candidates candidateCount=%d — skipping LLM judgment, falling through to create", tag.Label, category, len(matchResult.Candidates))
				savedCandidates = matchResult.Candidates

			case "no_match":
				logging.Infof("findOrCreateTag: label=%q category=%s matchType=no_match", tag.Label, category)
			}
		}
	} else {
		logging.Infof("findOrCreateTag: label=%q category=%s embeddingService=nil fallback=slug_or_create", tag.Label, category)
	}

	// Fallback: exact slug+category match (when embedding unavailable)
	// or creation path for no_match/candidates that fell through
	var dbTag models.TopicTag
	err := repository.Repo.DB().Where("slug = ? AND category = ?", slug, category).First(&dbTag).Error
	if err == nil {
		logging.Infof("findOrCreateTag: label=%q category=%s fallback=existing_by_slug existingID=%d existingLabel=%q", tag.Label, category, dbTag.ID, dbTag.Label)
		// Found existing tag - update label and source if needed
		dbTag.Label = tag.Label
		dbTag.Category = category
		dbTag.Source = source
		if tag.Icon != "" {
			dbTag.Icon = tag.Icon
		}
		if len(tag.Aliases) > 0 {
			aJSON, _ := json.Marshal(tag.Aliases)
			dbTag.Aliases = string(aJSON)
		}
		if err := repository.Repo.DB().Save(&dbTag).Error; err != nil {
			return nil, err
		}
		// Backfill embedding if missing (fallback path)
		if es != nil && category != "event" {
			go ensureTagEmbedding(es, dbTag.ID)
		}
		GetTagCache().Set(slug, category, &dbTag)
		return &dbTag, nil
	}

	// Create new tag
	logging.Infof("findOrCreateTag: label=%q category=%s fallback=create_new", tag.Label, category)
	newTag := models.TopicTag{
		Slug:        slug,
		Label:       tag.Label,
		Category:    category,
		Icon:        tag.Icon,
		Aliases:     string(aliasesJSON),
		IsCanonical: true,
		Source:      source,
		Status:      "active",
		Description: tagDescriptionForCategory(tag.Description, category),
	}
	if err := repository.Repo.DB().Create(&newTag).Error; err != nil {
		return nil, err
	}

	if len(savedCandidates) > 0 {
		RecordMergeSuggestions(newTag.ID, tag.Label, category, savedCandidates)
	}

	if es != nil && category != "event" {
		go ensureTagEmbedding(es, newTag.ID)
	}

	if category == "event" {
		go generateTagDescription(newTag.ID, tag.Label, category, articleContext) //nolint:gosec
	}

	GetTagCache().Set(slug, category, &newTag)
	return &newTag, nil
}

// generateTagDescription generates a description for a tag via LLM and updates the database.
// Runs in a goroutine — never blocks tag creation. Failures are logged and swallowed.
// Retries up to 3 times on LLM call or parse failure.
func generateTagDescription(tagID uint, label, category, articleContext string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("generateTagDescription panic for tag %d: %v", tagID, r)
		}
	}()

	isEvent := category == models.TagCategoryEvent

	desc, keywords, ok := requestTagDescription(isEvent, tagID, label, category, articleContext)
	if !ok {
		logging.Warnf("Failed to generate description for tag %d after 3 attempts", tagID)
		return
	}

	if err := saveTagDescription(isEvent, tagID, desc, keywords); err != nil {
		logging.Warnf("Failed to persist generated description for tag %d: %v", tagID, err)
		return
	}
	if isEvent && len(keywords) > 0 {
		logging.Infof("Generated description + %d keywords for event tag %d (%s): %v", len(keywords), tagID, label, keywords)
	}
}

// buildEventTagDescriptionPrompt renders the user prompt for event-tag
// description generation. Shared by generateTagDescription (new-tag path) and
// the anchor-fix regeneration CLI so both enforce the same time-anchor
// discipline. Examples are deliberately fictional/time-neutral: real events
// carry historical date anchors the LLM tends to copy instead of reading the
// article Context (fix-event-tag-time-anchor).
func buildEventTagDescriptionPrompt(label, category, articleContext string) string {
	return fmt.Sprintf(`Generate a concise description (1-2 sentences) for this event tag.
Tag: %q
Category: %s
Context from article: %s

Description requirements:
- Must be in Chinese (中文)
- Objective, factual statement — no subjective opinions or qualifiers
- Must explain what the event is about, including key entities and actions
- Keep under 500 characters
- Time anchor discipline (mandatory):
  * The event time MUST be anchored to the [日期: …] (article publish date) given in the Context above; describe the event within the article's own timeframe
  * MUST NOT introduce any time information absent from the Context, including historical event dates from your prior knowledge
  * If the tag name coincides with a well-known historical event but the Context is about something else, follow the Context, not your memory
- Examples (fictional; time anchors may only come from the Context):
  * Tag "某国央行上调基准利率" → "某国央行上调基准利率的货币政策事件，发生于文章报道当期"
  * Tag "科技公司数据中心投产" → "某科技公司新建数据中心正式投入运营的事件，具体主体与时间以文章报道为准"

Also extract 3-5 keywords: key entity names, locations, and action words that define this event.
- Keywords should be concise nouns or verbs (e.g., "央行", "加息", "通胀", "汇率", "货币政策")
- Avoid generic words (e.g., "事件", "情况", "问题")

Respond with JSON: {"description": "your answer", "keywords": ["keyword1", "keyword2", ...]}`, label, category, articleContext)
}

// eventDescriptionJSONSchema is the JSON-schema half of the event description
// call, split out so the anchor-fix CLI shares the exact contract.
func eventDescriptionJSONSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"description": {Type: "string", Description: "事件标签的中文客观描述，不超过500字符"},
			"keywords":    {Type: "array", Items: &airouter.SchemaProperty{Type: "string", Description: "3-5个关键实体/动作词"}, Description: "事件关键词列表"},
		},
		Required: []string{"description", "keywords"},
	}
}

// requestTagDescription runs the description LLM call with retries and returns
// the parsed description plus event keywords (empty for non-event tags).
// ok=false when every attempt failed on call or parse. Shared by the async
// new-tag path and the anchor-fix regeneration CLI.
func requestTagDescription(isEvent bool, tagID uint, label, category, articleContext string) (string, []string, bool) {
	router := airouter.NewRouter()

	var prompt string
	var jsonSchema *airouter.JSONSchema

	if isEvent {
		prompt = buildEventTagDescriptionPrompt(label, category, articleContext)
		jsonSchema = eventDescriptionJSONSchema()
	} else {
		prompt = fmt.Sprintf(`Generate a concise description (1-2 sentences) for this tag.
Tag: %q
Category: %s
Context from article: %s

Description requirements:
- Must be in Chinese (中文)
- Objective, factual statement — no subjective opinions or qualifiers
- Must explain what the tag refers to, not just repeat the label
- Keep under 500 characters
- Examples of good descriptions:
  * Tag "ChatGPT" → "OpenAI开发的大型语言模型聊天机器人，基于GPT架构，支持多轮对话和文本生成"
  * Tag "苹果WWDC 2024" → "苹果公司于2024年6月举办的全球开发者大会，发布了Apple Intelligence等多项更新"
  * Tag "Sam Altman" → "OpenAI首席执行官，曾多次参与AI安全与治理相关的公开讨论"

Respond with JSON: {"description": "your answer"}`, label, category, articleContext)

		jsonSchema = &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"description": {Type: "string", Description: "标签的中文客观描述，不超过500字符"},
			},
			Required: []string{"description"},
		}
	}

	req := airouter.ChatRequest{
		Operation:  "tagmanagement.tag_description",
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一个标签分类助手，只输出合法JSON。"},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		JSONSchema:  jsonSchema,
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation": "tag_description",
			"tag_id":    tagID,
			"tag_label": label,
			"category":  category,
		},
	}

	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := router.Chat(context.Background(), req)
		if err != nil {
			logging.Warnf("Description LLM call failed for tag %d (attempt %d/%d): %v", tagID, attempt, maxRetries, err)
			continue
		}

		if isEvent {
			var parsed struct {
				Description string   `json:"description"`
				Keywords    []string `json:"keywords"`
			}
			if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil || parsed.Description == "" {
				logging.Warnf("Failed to parse description for event tag %d (attempt %d/%d)", tagID, attempt, maxRetries)
				continue
			}
			return parsed.Description, parsed.Keywords, true
		}

		var parsed struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil || parsed.Description == "" {
			logging.Warnf("Failed to parse description for tag %d (attempt %d/%d)", tagID, attempt, maxRetries)
			continue
		}
		return parsed.Description, nil, true
	}

	return "", nil, false
}

// saveTagDescription truncates the generated description to 500 runes,
// persists it (together with event_keywords metadata when present), and
// enqueues tag re-embedding. Idempotent: repeated calls overwrite the same
// row instead of creating duplicates. Shared by the async new-tag path and
// the anchor-fix regeneration CLI.
func saveTagDescription(isEvent bool, tagID uint, desc string, keywords []string) error {
	if len([]rune(desc)) > 500 {
		desc = string([]rune(desc)[:500])
	}

	if isEvent && len(keywords) > 0 {
		var existing models.TopicTag
		if err := repository.Repo.DB().Select("metadata").Where("id = ?", tagID).First(&existing).Error; err != nil {
			return fmt.Errorf("load existing metadata for event tag %d: %w", tagID, err)
		}
		if existing.Metadata == nil {
			existing.Metadata = models.MetadataMap{}
		}
		existing.Metadata["event_keywords"] = keywords
		if err := repository.Repo.DB().Model(&models.TopicTag{}).Where("id = ?", tagID).Updates(map[string]any{
			"description": desc,
			"metadata":    existing.Metadata,
		}).Error; err != nil {
			return fmt.Errorf("save description+keywords for event tag %d: %w", tagID, err)
		}
	} else if err := repository.Repo.DB().Model(&models.TopicTag{}).Where("id = ?", tagID).Update("description", desc).Error; err != nil {
		return fmt.Errorf("save description for tag %d: %w", tagID, err)
	}

	if qs := getEmbeddingQueueService(); qs != nil {
		if err := qs.Enqueue(tagID); err != nil {
			logging.Warnf("Failed to enqueue re-embedding after description update for tag %d: %v", tagID, err)
		}
	}
	return nil
}

// ensureTagEmbedding checks if a tag already has an embedding and generates one if missing.
// Used to backfill embeddings for tags created before the pgvector migration.
func ensureTagEmbedding(es *EmbeddingService, tagID uint) {
	qs := getEmbeddingQueueService()
	if err := qs.Enqueue(tagID); err != nil {
		logging.Warnf("Failed to enqueue embedding for tag %d: %v", tagID, err)
	}
}

// dedupeTagsWithCategory removes duplicate tags based on (category, slug)
func dedupeTagsWithCategory(items []TopicTag) []TopicTag {
	best := make(map[string]TopicTag)
	for _, item := range items {
		if item.Slug == "" {
			item.Slug = Slugify(item.Label)
		}
		if item.Category == "" {
			item.Category = "keyword"
		}
		key := item.Category + ":" + item.Slug
		current, exists := best[key]
		if !exists || current.Score < item.Score {
			best[key] = item
		}
	}

	result := make([]TopicTag, 0, len(best))
	for _, item := range best {
		result = append(result, item)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Label < result[j].Label
		}
		return result[i].Score > result[j].Score
	})

	return result
}

func tagDescriptionForCategory(desc, category string) string {
	if category == "person" {
		return ""
	}
	return desc
}
