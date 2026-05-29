package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

var (
	embeddingService          *EmbeddingService
	embeddingServiceOnce      sync.Once
	embeddingQueueService     *EmbeddingQueueService
	embeddingQueueServiceOnce sync.Once
)

func getEmbeddingService() *EmbeddingService {
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
	kind := NormalizeTopicKind(tag.Kind, category)
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

	// Try embedding-based three-level matching
	es := getEmbeddingService()
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
					existing.Kind = kind
					if err := database.DB.Save(existing).Error; err != nil {
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
	err := database.DB.Where("slug = ? AND category = ?", slug, category).First(&dbTag).Error
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
		dbTag.Kind = kind
		if err := database.DB.Save(&dbTag).Error; err != nil {
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
		Kind:        kind,
		Icon:        tag.Icon,
		Aliases:     string(aliasesJSON),
		IsCanonical: true,
		Source:      source,
		Description: tagDescriptionForCategory(tag.Description, category),
	}
	if err := database.DB.Create(&newTag).Error; err != nil {
		return nil, err
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

	router := airouter.NewRouter()

	var prompt string
	var jsonSchema *airouter.JSONSchema

	if isEvent {
		prompt = fmt.Sprintf(`Generate a concise description (1-2 sentences) for this event tag.
Tag: %q
Category: %s
Context from article: %s

Description requirements:
- Must be in Chinese (中文)
- Objective, factual statement — no subjective opinions or qualifiers
- Must explain what the event is about, including key entities and actions
- Keep under 500 characters
- Examples:
  * Tag "苹果WWDC 2024" → "苹果公司于2024年6月举办的全球开发者大会，发布了Apple Intelligence等多项更新"
  * Tag "伊朗袭击以色列" → "2024年4月伊朗对以色列发动的大规模导弹和无人机袭击，系两国直接军事冲突的标志性事件"

Also extract 3-5 keywords: key entity names, locations, and action words that define this event.
- Keywords should be concise nouns or verbs (e.g., "美国", "伊朗", "袭击", "制裁", "核协议")
- Avoid generic words (e.g., "事件", "情况", "问题")

Respond with JSON: {"description": "your answer", "keywords": ["keyword1", "keyword2", ...]}`, label, category, articleContext)

		jsonSchema = &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"description": {Type: "string", Description: "事件标签的中文客观描述，不超过500字符"},
				"keywords":    {Type: "array", Items: &airouter.SchemaProperty{Type: "string", Description: "3-5个关键实体/动作词"}, Description: "事件关键词列表"},
			},
			Required: []string{"description", "keywords"},
		}
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
	var desc string
	var keywords []string
	var success bool

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
			desc = parsed.Description
			keywords = parsed.Keywords
		} else {
			var parsed struct {
				Description string `json:"description"`
			}
			if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil || parsed.Description == "" {
				logging.Warnf("Failed to parse description for tag %d (attempt %d/%d)", tagID, attempt, maxRetries)
				continue
			}
			desc = parsed.Description
		}

		success = true
		break
	}

	if !success {
		logging.Warnf("Failed to generate description for tag %d after %d attempts", tagID, maxRetries)
		return
	}

	if len([]rune(desc)) > 500 {
		desc = string([]rune(desc)[:500])
	}

	if isEvent && len(keywords) > 0 {
		var existing models.TopicTag
		if err := database.DB.Select("metadata").Where("id = ?", tagID).First(&existing).Error; err != nil {
			logging.Warnf("Failed to load existing metadata for event tag %d: %v", tagID, err)
			return
		}
		if existing.Metadata == nil {
			existing.Metadata = models.MetadataMap{}
		}
		existing.Metadata["event_keywords"] = keywords
		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tagID).Updates(map[string]any{
			"description": desc,
			"metadata":    existing.Metadata,
		}).Error; err != nil {
			logging.Warnf("Failed to save description+keywords for event tag %d: %v", tagID, err)
			return
		}
		logging.Infof("Generated description + %d keywords for event tag %d (%s): %v", len(keywords), tagID, label, keywords)
	} else {
		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tagID).Update("description", desc).Error; err != nil {
			logging.Warnf("Failed to save description for tag %d: %v", tagID, err)
			return
		}
	}

	if qs := getEmbeddingQueueService(); qs != nil {
		if err := qs.Enqueue(tagID); err != nil {
			logging.Warnf("Failed to enqueue re-embedding after description update for tag %d: %v", tagID, err)
		}
	}
}

type batchDescResult struct {
	Description string
	Keywords    []string
}

// batchGenerateTagDescriptions generates descriptions for multiple tags in a single LLM call.
// Returns a map of tagID -> batchDescResult (keywords populated for event tags).
func batchGenerateTagDescriptions(tags []models.TopicTag) map[uint]*batchDescResult {
	if len(tags) == 0 {
		return nil
	}
	if len(tags) == 1 {
		articleContext := buildArticleContextForTag(tags[0].ID)
		if articleContext == "" {
			return nil
		}
		generateTagDescription(tags[0].ID, tags[0].Label, tags[0].Category, articleContext)
		return map[uint]*batchDescResult{tags[0].ID: {}} // empty = already saved by generateTagDescription
	}

	type tagContext struct {
		ID       uint   `json:"id"`
		Label    string `json:"label"`
		Category string `json:"category"`
		Context  string `json:"context"`
	}
	var items []tagContext
	for _, tag := range tags {
		ctx := buildArticleContextForTag(tag.ID)
		if ctx == "" {
			continue
		}
		items = append(items, tagContext{
			ID:       tag.ID,
			Label:    tag.Label,
			Category: tag.Category,
			Context:  ctx,
		})
	}
	if len(items) == 0 {
		return nil
	}

	itemsJSON, _ := json.MarshalIndent(items, "", "  ")
	prompt := fmt.Sprintf(`为以下标签批量生成 description（中文，每个 1-2 句话，客观事实，500 字以内）。

标签列表：
%s

规则：
- 每个标签的 description 必须解释该标签是什么，不能只重复标签名
- person 类标签说明人物身份
- event 类标签说明事件经过
- keyword 类标签说明概念领域
- 对 event 类标签，额外提取 3-5 个关键词（实体名、地名、动作词），避免泛泛的词如"事件""情况"

返回 JSON: {"descriptions": [{"id": 标签ID, "description": "描述内容", "keywords": ["关键词1", ...]}, ...]}
非 event 类标签的 keywords 字段留空数组 []`, string(itemsJSON))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一个标签分类助手，只输出合法JSON。"},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"descriptions": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"id":          {Type: "integer"},
							"description": {Type: "string"},
							"keywords": {
								Type:        "array",
								Items:       &airouter.SchemaProperty{Type: "string"},
								Description: "event 标签的关键词列表，非 event 标签为空数组",
							},
						},
						Required: []string{"id", "description"},
					},
				},
			},
			Required: []string{"descriptions"},
		},
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation": "tag_description_batch",
			"count":     len(items),
		},
	}

	const maxBatchRetries = 2
	var result *airouter.ChatResult
	var err error
	for attempt := 1; attempt <= maxBatchRetries; attempt++ {
		result, err = router.Chat(context.Background(), req)
		if err == nil {
			break
		}
		logging.Warnf("batchGenerateTagDescriptions: LLM call failed (attempt %d/%d): %v", attempt, maxBatchRetries, err)
	}
	if err != nil {
		logging.Warnf("batchGenerateTagDescriptions: all %d attempts failed", maxBatchRetries)
		return nil
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var parsed struct {
		Descriptions []struct {
			ID          uint     `json:"id"`
			Description string   `json:"description"`
			Keywords    []string `json:"keywords"`
		} `json:"descriptions"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		logging.Warnf("batchGenerateTagDescriptions: parse failed: %v", err)
		return nil
	}

	results := make(map[uint]*batchDescResult)
	validIDs := make(map[uint]bool, len(items))
	for _, item := range items {
		validIDs[item.ID] = true
	}
	for _, d := range parsed.Descriptions {
		if d.Description != "" && validIDs[d.ID] {
			desc := d.Description
			if len([]rune(desc)) > 500 {
				desc = string([]rune(desc)[:500])
			}
			results[d.ID] = &batchDescResult{
				Description: desc,
				Keywords:    d.Keywords,
			}
		}
	}
	return results
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
