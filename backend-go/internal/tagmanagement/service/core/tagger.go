package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/reader/search"
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

// findOrCreateTag finds an existing tag or creates a new one
// Uses three-level matching: exact/alias → embedding similarity → create new
func findOrCreateTag(ctx context.Context, tag TopicTag, source string, articleContext string, articleID uint) (*models.TopicTag, error) {
	slug := Slugify(tag.Label)
	category := NormalizeDisplayCategory(tag.Kind, tag.Category)
	logging.Infof("findOrCreateTag: start label=%q slug=%q category=%s source=%s", tag.Label, slug, category, source)

	if cached, ok := GetTagCache().Get(slug, category); ok {
		logging.Infof("findOrCreateTag: label=%q slug=%q category=%s cache=hit existingID=%d", tag.Label, slug, category, cached.ID)
		// 事件指纹持久化（缓存命中分支）：缓存对象可能带旧 metadata，
		// persistEventFingerprint 读 DB 真值合并；先写者赢，不翻转已有指纹。
		if err := persistEventFingerprint(cached, tag.Fingerprint); err != nil {
			logging.Warnf("findOrCreateTag: persist event fingerprint for cached tag %d failed: %v", cached.ID, err)
		}
		GetTagCache().Set(slug, category, cached)
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
					// 事件指纹持久化（exact 分支）
					if err := persistEventFingerprint(existing, tag.Fingerprint); err != nil {
						logging.Warnf("findOrCreateTag: persist event fingerprint for tag %d failed: %v", existing.ID, err)
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
		// 标签写入失联想词典缓存（add-article-search 2.8 接线：label 变更后
		// 下次搜索重新装载词典）
		search.DefaultDict().InvalidateDict()
		// Backfill embedding if missing (fallback path)
		if es != nil && category != "event" {
			go ensureTagEmbedding(es, dbTag.ID)
		}
		// 事件指纹持久化（slug 兑底分支）
		if err := persistEventFingerprint(&dbTag, tag.Fingerprint); err != nil {
			logging.Warnf("findOrCreateTag: persist event fingerprint for tag %d failed: %v", dbTag.ID, err)
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
	// 新标签入库 → 搜索词典缓存失效重载（同上接线）
	search.DefaultDict().InvalidateDict()

	if len(savedCandidates) > 0 {
		RecordMergeSuggestions(newTag.ID, tag.Label, category, savedCandidates)
	}

	if es != nil && category != "event" {
		go ensureTagEmbedding(es, newTag.ID)
	}

	if category == "event" {
		go generateTagDescription(newTag.ID, tag.Label, category, articleContext) //nolint:gosec
	}

	// 事件指纹持久化（新建分支）：Create 后读 DB metadata 合并写入
	if err := persistEventFingerprint(&newTag, tag.Fingerprint); err != nil {
		logging.Warnf("findOrCreateTag: persist event fingerprint for new tag %d failed: %v", newTag.ID, err)
	}

	GetTagCache().Set(slug, category, &newTag)
	return &newTag, nil
}

// eventFingerprintMetadataKey is the topic_tags.metadata key holding the
// structured event fingerprint nested object (add-event-story-heat).
const eventFingerprintMetadataKey = "event_fingerprint"

// entitySubjectMetadataKey is the topic_tags.metadata key holding the
// shadow-mode entity-subject record (normalize-event-subject-entities D10):
// the FIRST-round verbatim mention + stable short identity description plus
// the resolution outcome. Independent lazy key — WRITTEN ONLY in shadow mode,
// has NO read path this iteration, never touches event_fingerprint.
const entitySubjectMetadataKey = "entity_subject"

// fingerprintMapComplete reports whether a metadata-loaded fingerprint map
// carries all four identity fields (occurred_at optional) — same completeness
// rule as (*EventFingerprint).Complete.
func fingerprintMapComplete(m map[string]any) bool {
	if m == nil {
		return false
	}
	for _, key := range []string{"subject", "subject_key", "action", "object"} {
		v, ok := m[key].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

// mergeFingerprintMetadata merges an event fingerprint into a tag's existing
// metadata map (pure function; caller owns the DB read-modify-write). Rules
// (add-event-story-heat batch 2.4):
//   - nil/incomplete fp or existing COMPLETE fingerprint → unchanged（先写者赢：
//     同一标签不因不同文章反复翻身份）；
//   - otherwise → write the nested event_fingerprint object, omitting empty
//     fields (occurred_at optional)；
//   - existing sibling keys (event_keywords 等) must survive.
func mergeFingerprintMetadata(meta models.MetadataMap, fp *EventFingerprint) models.MetadataMap {
	if fp == nil || !fp.Complete() {
		return meta
	}
	if meta == nil {
		meta = models.MetadataMap{}
	}
	if existing, ok := meta[eventFingerprintMetadataKey].(map[string]any); ok && fingerprintMapComplete(existing) {
		return meta
	}
	nested := map[string]any{}
	if fp.Subject != "" {
		nested["subject"] = fp.Subject
	}
	if fp.SubjectKey != "" {
		nested["subject_key"] = fp.SubjectKey
	}
	if fp.Action != "" {
		nested["action"] = fp.Action
	}
	if fp.Object != "" {
		nested["object"] = fp.Object
	}
	if fp.OccurredAt != "" {
		nested["occurred_at"] = fp.OccurredAt
	}
	meta[eventFingerprintMetadataKey] = nested
	return meta
}

// persistEventFingerprint writes an event tag's structured fingerprint into
// topic_tags.metadata (nested under event_fingerprint). No-op when fp is nil
// or incomplete (incomplete fingerprints never block the tag). The tag's
// metadata is read fresh from the DB — NOT from the passed object or the tag
// cache, which may carry stale values — then merged (first writer wins: an
// already-complete fingerprint is kept verbatim) and persisted via Updates,
// preserving sibling keys like event_keywords. On success the passed tag's
// Metadata is synced to the merged value so callers can refresh the cache.
func persistEventFingerprint(tag *models.TopicTag, fp *EventFingerprint) error {
	if tag == nil || fp == nil || !fp.Complete() {
		return nil
	}
	var existing models.TopicTag
	if err := repository.Repo.DB().Select("metadata").Where("id = ?", tag.ID).First(&existing).Error; err != nil {
		return fmt.Errorf("load metadata for event fingerprint (tag %d): %w", tag.ID, err)
	}
	merged := mergeFingerprintMetadata(existing.Metadata, fp)
	if err := repository.Repo.DB().Model(&models.TopicTag{}).Where("id = ?", tag.ID).Updates(map[string]any{"metadata": merged}).Error; err != nil {
		return fmt.Errorf("save event fingerprint for tag %d: %w", tag.ID, err)
	}
	tag.Metadata = merged
	return nil
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
//
// 与 event_fingerprint 共存：这里是对 metadata 的读-改-写（先 Select metadata
// 再只覆盖 event_keywords/description），不会碰 persistEventFingerprint 写入的
// event_fingerprint 嵌套对象；两侧都必须保持读-改-写，禁止盲写整个 metadata。
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

// EntitySubjectRecord is the payload persisted under the lazy
// metadata.entity_subject key (shadow mode only). Carries the first-round
// verbatim产出 plus the resolution outcome for later wiring / auditing.
type EntitySubjectRecord struct {
	// First-round verbatim outputs (same call as the fingerprint).
	Subject     string `json:"subject,omitempty"`
	SubjectKey  string `json:"subject_key,omitempty"`
	SubjectDesc string `json:"subject_desc,omitempty"`
	// Resolution outcome (entity layer).
	EntityID  uint   `json:"entity_id,omitempty"`
	EntityKey string `json:"entity_key,omitempty"`
	Status    string `json:"status"` // created | reused | idempotent | unresolved
	Reason    string `json:"reason,omitempty"`
	Mode      string `json:"mode"`
}

// mergeEntitySubjectMetadata merges the entity-subject record into a tag's
// metadata map (pure function; caller owns the DB read-modify-write).
// First-writer-wins EXCEPT unresolved: an unresolved record is not a stored
// decision (the mention was never normalized), so a later successful
// resolution MAY replace it. A successful record is never flipped — the same
// tag does not re-decide its identity per article. Sibling keys
// (event_fingerprint, event_keywords, …) always survive.
func mergeEntitySubjectMetadata(meta models.MetadataMap, rec *EntitySubjectRecord) models.MetadataMap {
	if rec == nil {
		return meta
	}
	if meta == nil {
		meta = models.MetadataMap{}
	}
	if existing, ok := meta[entitySubjectMetadataKey].(map[string]any); ok {
		if status, _ := existing["status"].(string); status != "" && status != "unresolved" {
			return meta // a successful decision is frozen (first writer wins)
		}
	}
	meta[entitySubjectMetadataKey] = map[string]any{
		"subject":      rec.Subject,
		"subject_key":  rec.SubjectKey,
		"subject_desc": rec.SubjectDesc,
		"entity_id":    rec.EntityID,
		"entity_key":   rec.EntityKey,
		"status":       rec.Status,
		"reason":       rec.Reason,
		"mode":         rec.Mode,
	}
	return meta
}

// PersistEntitySubject writes the shadow-mode entity-subject record into
// topic_tags.metadata under the independent entity_subject key. Same
// read-modify-write discipline as persistEventFingerprint: metadata is loaded
// FRESH from the DB (never from the passed object or the tag cache), only
// this ONE key is merged, sibling keys survive, and the write goes through a
// scoped Updates. NEVER blindly overwrites the whole metadata column.
func PersistEntitySubject(tag *models.TopicTag, rec *EntitySubjectRecord) error {
	if tag == nil || rec == nil {
		return nil
	}
	var existing models.TopicTag
	if err := repository.Repo.DB().Select("metadata").Where("id = ?", tag.ID).First(&existing).Error; err != nil {
		return fmt.Errorf("load metadata for entity subject (tag %d): %w", tag.ID, err)
	}
	merged := mergeEntitySubjectMetadata(existing.Metadata, rec)
	if err := repository.Repo.DB().Model(&models.TopicTag{}).Where("id = ?", tag.ID).Updates(map[string]any{"metadata": merged}).Error; err != nil {
		return fmt.Errorf("save entity subject for tag %d: %w", tag.ID, err)
	}
	tag.Metadata = merged
	return nil
}
