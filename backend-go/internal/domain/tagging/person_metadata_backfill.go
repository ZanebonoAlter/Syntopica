package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

func BackfillPersonMetadata() (int, error) {
	var tags []models.TopicTag
	if err := database.DB.Where("category = ? AND status = ? AND (metadata IS NULL OR metadata = '{}'::jsonb OR metadata = '')", "person", "active").
		Limit(100).
		Find(&tags).Error; err != nil {
		return 0, fmt.Errorf("query person tags without metadata: %w", err)
	}

	logging.Infof("person metadata backfill: found %d tags to process", len(tags))

	processed := 0
	for _, tag := range tags {
		if err := backfillSinglePersonMetadata(tag); err != nil {
			logging.Warnf("person metadata backfill failed for tag %d (%s): %v", tag.ID, tag.Label, err)
			continue
		}
		processed++
		time.Sleep(500 * time.Millisecond)
	}

	logging.Infof("person metadata backfill: processed %d/%d", processed, len(tags))
	return processed, nil
}

func backfillSinglePersonMetadata(tag models.TopicTag) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	router := airouter.NewRouter()

	prompt := fmt.Sprintf(`Extract structured attributes for this person tag.

Tag: %q
Description: %s

Extract:
- country: nationality or primary country (中文)
- organization: primary organization (中文)
- role: primary position or title (中文)
- domains: areas of expertise as array (中文)

Respond with JSON: {"country": "...", "organization": "...", "role": "...", "domains": [...]}`, tag.Label, tag.Description)

	result, err := router.Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一个人物属性提取助手，只输出合法JSON。"},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"country":      {Type: "string"},
				"organization": {Type: "string"},
				"role":         {Type: "string"},
				"domains":      {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
			},
			Required: []string{"country", "organization", "role", "domains"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "backfill_person_metadata",
			"tag_id":    tag.ID,
			"tag_label": tag.Label,
		},
	})
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	var attrs struct {
		Country      string   `json:"country"`
		Organization string   `json:"organization"`
		Role         string   `json:"role"`
		Domains      []string `json:"domains"`
	}
	if err := json.Unmarshal([]byte(result.Content), &attrs); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	metadataMap := models.MetadataMap{
		"country":      attrs.Country,
		"organization": attrs.Organization,
		"role":         attrs.Role,
		"domains":      attrs.Domains,
	}

	if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
		Update("metadata", metadataMap).Error; err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}

	qs := NewEmbeddingQueueService(nil)
	if err := qs.Enqueue(tag.ID); err != nil {
		logging.Warnf("Failed to enqueue re-embedding after metadata backfill for tag %d: %v", tag.ID, err)
	}

	logging.Infof("person metadata backfilled for tag %d (%s): country=%s, org=%s, role=%s",
		tag.ID, tag.Label, attrs.Country, attrs.Organization, attrs.Role)
	return nil
}
