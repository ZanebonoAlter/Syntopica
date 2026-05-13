package concept

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

const bootstrapMinTags = 10
const neighborDistanceThreshold = 0.65

// bootstrapTag holds a tag with its embedding for clustering.
type bootstrapTag struct {
	ID          uint
	Label       string
	Description string
}

// BootstrapConcepts loads active tags with semantic embeddings, clusters them
// using pgvector distance, names each cluster via LLM, and creates BoardConcept
// entries with status='pending'.
func BootstrapConcepts(ctx context.Context, category string) ([]models.BoardConcept, error) {
	// Step 1: Load all active tags of the category with semantic embeddings
	tags, err := loadActiveTagsWithEmbeddings(category)
	if err != nil {
		return nil, fmt.Errorf("load tags: %w", err)
	}

	if len(tags) < bootstrapMinTags {
		logging.Infof("bootstrap: only %d tags for category %s, need at least %d", len(tags), category, bootstrapMinTags)
		return nil, nil
	}

	// Step 3: Build connected graph using pgvector distance
	adj := buildNeighborGraph(tags)

	// Step 4: Find connected components via BFS
	clusters := findConnectedComponents(tags, adj)
	logging.Infof("bootstrap: found %d clusters for category %s from %d tags", len(clusters), category, len(tags))

	// Step 5 & 6: For each cluster, call LLM to name and create concept
	var created []models.BoardConcept
	for i, cluster := range clusters {
		concept, err := processCluster(ctx, category, cluster, i)
		if err != nil {
			logging.Warnf("bootstrap: failed to process cluster %d: %v", i, err)
			continue
		}
		created = append(created, *concept)
	}

	return created, nil
}

// loadActiveTagsWithEmbeddings loads tags with their semantic embeddings.
func loadActiveTagsWithEmbeddings(category string) ([]bootstrapTag, error) {
	type tagRow struct {
		ID           uint   `gorm:"column:topic_tag_id"`
		Label        string `gorm:"column:label"`
		Description  string `gorm:"column:description"`
		EmbeddingVec string `gorm:"column:embedding"`
	}

	var rows []tagRow
	err := database.DB.Table("topic_tag_embeddings tte").
		Select("tte.topic_tag_id, tt.label, tt.description, tte.embedding::text as embedding").
		Joins("JOIN topic_tags tt ON tt.id = tte.topic_tag_id").
		Where("tte.embedding_type = ? AND tt.status = ? AND tt.category = ?", "semantic", "active", category).
		Where("tte.embedding IS NOT NULL").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query active tags with embeddings: %w", err)
	}

	tags := make([]bootstrapTag, 0, len(rows))
	for _, row := range rows {
		// Validate embedding is parseable
		vec, err := parseConceptEmbeddingVec(&row.EmbeddingVec)
		if err != nil || len(vec) == 0 {
			continue
		}
		tags = append(tags, bootstrapTag{
			ID:          row.ID,
			Label:       row.Label,
			Description: row.Description,
		})
	}

	return tags, nil
}

// buildNeighborGraph builds an adjacency list using pgvector <=> distance.
func buildNeighborGraph(tags []bootstrapTag) map[uint][]uint {
	adj := make(map[uint][]uint, len(tags))
	for _, tag := range tags {
		adj[tag.ID] = nil
	}

	for _, tag := range tags {
		type neighborRow struct {
			TopicTagID uint `gorm:"column:topic_tag_id"`
		}
		var neighbors []neighborRow
		database.DB.Raw(
			`SELECT t2.topic_tag_id
			 FROM topic_tag_embeddings t1
			 JOIN topic_tag_embeddings t2 ON t1.topic_tag_id != t2.topic_tag_id
			 WHERE t1.topic_tag_id = ?
			   AND t1.embedding_type = 'semantic'
			   AND t2.embedding_type = 'semantic'
			   AND t1.embedding <=> t2.embedding < ?`,
			tag.ID, neighborDistanceThreshold,
		).Scan(&neighbors)

		for _, n := range neighbors {
			adj[tag.ID] = append(adj[tag.ID], n.TopicTagID)
		}
	}

	return adj
}

// findConnectedComponents finds clusters using BFS on the neighbor graph.
func findConnectedComponents(tags []bootstrapTag, adj map[uint][]uint) [][]bootstrapTag {
	visited := make(map[uint]bool)
	var clusters [][]bootstrapTag

	for _, tag := range tags {
		if visited[tag.ID] {
			continue
		}

		// BFS to find all connected tags
		queue := []uint{tag.ID}
		visited[tag.ID] = true
		var component []uint

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)

			for _, neighbor := range adj[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}

		// Map component tag IDs back to bootstrapTag structs
		tagByID := make(map[uint]bootstrapTag, len(tags))
		for _, t := range tags {
			tagByID[t.ID] = t
		}

		var cluster []bootstrapTag
		for _, id := range component {
			if t, ok := tagByID[id]; ok {
				cluster = append(cluster, t)
			}
		}

		if len(cluster) > 0 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// processCluster names a cluster via LLM and creates a BoardConcept.
func processCluster(ctx context.Context, category string, cluster []bootstrapTag, idx int) (*models.BoardConcept, error) {
	name, description, err := nameClusterViaLLM(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("LLM naming for cluster %d: %w", idx, err)
	}

	concept := &models.BoardConcept{
		Name:        name,
		Description: description,
		Category:    category,
		Status:      "pending",
		ScopeType:   "global",
		IsSystem:    false,
	}

	if err := database.DB.Create(concept).Error; err != nil {
		return nil, fmt.Errorf("create bootstrap concept: %w", err)
	}

	logging.Infof("bootstrap: created concept %d (%s) for cluster %d with %d tags", concept.ID, name, idx, len(cluster))
	return concept, nil
}

// nameClusterViaLLM calls the LLM to generate a name and description for a tag cluster.
func nameClusterViaLLM(ctx context.Context, cluster []bootstrapTag) (string, string, error) {
	prompt := buildClusterPrompt(cluster)

	temperature := 0.4
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: bootstrapSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"concepts": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string", Description: "概念名称，2-6字"},
							"description": {Type: "string", Description: "概念描述，30-80字"},
						},
						Required: []string{"name", "description"},
					},
				},
			},
			Required: []string{"concepts"},
		},
		Metadata: map[string]any{
			"operation":     "concept_bootstrap",
			"cluster_size":  len(cluster),
			"cluster_index": 0,
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("bootstrap LLM call: %w", err)
	}

	logging.Infof("bootstrap: raw LLM response length=%d", len(result.Content))

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var raw struct {
		Concepts []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"concepts"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", "", fmt.Errorf("parse bootstrap LLM response: %w", err)
	}

	if len(raw.Concepts) == 0 {
		// Fallback: use first tag label as name
		fallbackName := truncateText(cluster[0].Label, 6)
		fallbackDesc := truncateText(cluster[0].Description, 80)
		return fallbackName, fallbackDesc, nil
	}

	name := strings.TrimSpace(raw.Concepts[0].Name)
	description := strings.TrimSpace(raw.Concepts[0].Description)

	if name == "" {
		name = truncateText(cluster[0].Label, 6)
	}
	if description == "" {
		description = name
	}

	return name, description, nil
}

func buildClusterPrompt(cluster []bootstrapTag) string {
	var sb strings.Builder
	sb.WriteString("以下是一组语义相近的标签，请为这个标签组起一个概念名称和描述：\n\n")

	for _, t := range cluster {
		fmt.Fprintf(&sb, "- [TagID:%d] %s", t.ID, t.Label)
		if t.Description != "" {
			fmt.Fprintf(&sb, "\n  描述: %s", t.Description)
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("\n请返回一个JSON对象，包含一个concepts数组，其中每个元素有name（2-6字）和description（30-80字）字段。")
	return sb.String()
}

func truncateText(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}

const bootstrapSystemPrompt = `你是一名内容架构师，负责为语义标签组命名。

## 核心原则
1. 名称 2-6 个字，描述 30-80 字
2. 名称应概括该组标签的共同主题
3. 描述说明覆盖范围，不说怎么做
4. 每个聚类只返回一个概念

## 输出格式
返回一个JSON对象：{"concepts": [{"name": "...", "description": "..."}]}`
