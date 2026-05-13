package tagging

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type TagCluster struct {
	TagIDs []uint
	Tags   []*models.TopicTag
	AvgSim float64
}

type ClusterConfig struct {
	MaxTags             int
	SimilarityThreshold float64
	MaxClusterSize      int
}

var DefaultClusterConfig = ClusterConfig{
	MaxTags:             500,
	SimilarityThreshold: 0.85,
	MaxClusterSize:      8,
}

func (s *EmbeddingConfigService) LoadClusterConfig() ClusterConfig {
	cfg := DefaultClusterConfig
	config, err := s.LoadConfig()
	if err != nil {
		return cfg
	}
	if v, ok := config["cluster_max_tags"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxTags = n
		}
	}
	if v, ok := config["cluster_similarity_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.SimilarityThreshold = f
		}
	}
	if v, ok := config["cluster_max_size"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxClusterSize = n
		}
	}
	return cfg
}

func findConnectedComponents(tagIDs []uint, edges []SimilarityEdge) [][]uint {
	adj := make(map[uint][]uint, len(tagIDs))
	for _, e := range edges {
		adj[e.TagAID] = append(adj[e.TagAID], e.TagBID)
		adj[e.TagBID] = append(adj[e.TagBID], e.TagAID)
	}

	visited := make(map[uint]bool, len(tagIDs))
	var components [][]uint

	for _, id := range tagIDs {
		if visited[id] {
			continue
		}
		var comp []uint
		queue := []uint{id}
		visited[id] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			comp = append(comp, cur)
			for _, nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		if len(comp) >= 2 {
			components = append(components, comp)
		}
	}
	return components
}

func collectUnclassifiedTagIDs(category string, limit int) ([]uint, error) {
	var relatedIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("parent_id", &relatedIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("child_id", &childIDs)
	relatedSet := make(map[uint]bool, len(relatedIDs)+len(childIDs))
	for _, id := range relatedIDs {
		relatedSet[id] = true
	}
	for _, id := range childIDs {
		relatedSet[id] = true
	}

	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("source != 'abstract'").
		Where("category = ?", category).
		Where("id IN (SELECT DISTINCT topic_tag_id FROM article_topic_tags)")

	if len(relatedSet) > 0 {
		var excluded []uint
		for id := range relatedSet {
			excluded = append(excluded, id)
		}
		query = query.Where("id NOT IN ?", excluded)
	}

	query = query.Order("quality_score DESC, feed_count DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var ids []uint
	if err := query.Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("collect unclassified %s tags: %w", category, err)
	}
	return ids, nil
}

func loadTagModelsMap(ids []uint) (map[uint]*models.TopicTag, error) {
	var tags []models.TopicTag
	if err := database.DB.Where("id IN ?", ids).Find(&tags).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]*models.TopicTag, len(tags))
	for i := range tags {
		m[tags[i].ID] = &tags[i]
	}
	return m, nil
}

func ClusterUnclassifiedTags(ctx context.Context, category string) (*ClusteringResult, error) {
	cfg := NewEmbeddingConfigService().LoadClusterConfig()
	return ClusterUnclassifiedTagsWithConfig(ctx, category, cfg)
}

func ClusterUnclassifiedTagsWithConfig(ctx context.Context, category string, cfg ClusterConfig) (*ClusteringResult, error) {
	result := &ClusteringResult{}

	tagIDs, err := collectUnclassifiedTagIDs(category, cfg.MaxTags)
	if err != nil {
		return nil, err
	}
	if len(tagIDs) < 2 {
		logging.Infof("ClusterUnclassifiedTags(%s): only %d unclassified tags, skipping", category, len(tagIDs))
		return result, nil
	}
	result.TagsCollected = len(tagIDs)
	logging.Infof("ClusterUnclassifiedTags(%s): collected %d unclassified tags", category, len(tagIDs))

	es := NewEmbeddingService()

	edges, err := es.FindSimilarTagsAmongSet(ctx, tagIDs, cfg.SimilarityThreshold)
	if err != nil {
		return nil, fmt.Errorf("similarity search for %s: %w", category, err)
	}
	result.EdgesFound = len(edges)
	logging.Infof("ClusterUnclassifiedTags(%s): found %d similarity edges (threshold=%.2f)", category, len(edges), cfg.SimilarityThreshold)

	if len(edges) == 0 {
		return result, nil
	}

	components := findConnectedComponents(tagIDs, edges)
	result.ClustersFound = len(components)
	logging.Infof("ClusterUnclassifiedTags(%s): found %d connected components", category, len(components))

	tagsMap, err := loadTagModelsMap(tagIDs)
	if err != nil {
		return nil, fmt.Errorf("load tag models: %w", err)
	}

	for _, comp := range components {
		if len(comp) > cfg.MaxClusterSize {
			logging.Infof("ClusterUnclassifiedTags(%s): cluster of size %d exceeds max %d, truncating to top-%d by quality_score",
				category, len(comp), cfg.MaxClusterSize, cfg.MaxClusterSize)
			sort.Slice(comp, func(i, j int) bool {
				a, b := tagsMap[comp[i]], tagsMap[comp[j]]
				return a.QualityScore > b.QualityScore
			})
			comp = comp[:cfg.MaxClusterSize]
		}

		var candidates []TagCandidate
		for _, id := range comp {
			tag := tagsMap[id]
			if tag == nil {
				continue
			}
			candidates = append(candidates, TagCandidate{
				Tag:        tag,
				Similarity: 1.0,
			})
		}
		if len(candidates) < 2 {
			continue
		}

		clusterLabel := candidates[0].Tag.Label
		extracted, err := ExtractAbstractTag(ctx, candidates, clusterLabel, category, WithCaller("ClusterUnclassifiedTags"))
		if err != nil {
			logging.Warnf("ClusterUnclassifiedTags(%s): cluster judgment failed: %v", category, err)
			result.Errors++
			continue
		}

		if extracted.Merge != nil && extracted.Merge.Target != nil {
			logging.Infof("ClusterUnclassifiedTags(%s): merged %d tags into %q",
				category, len(candidates), extracted.Merge.Target.Label)
			result.MergesApplied++
		}
		if extracted.Abstract != nil {
			logging.Infof("ClusterUnclassifiedTags(%s): created abstract %q with %d children",
				category, extracted.Abstract.Tag.Label, len(extracted.Abstract.Children))
			result.AbstractsCreated++
		}
		if extracted.LLMExplicitNone {
			logging.Infof("ClusterUnclassifiedTags(%s): LLM judged cluster as independent (none)", category)
		}
	}

	return result, nil
}

type ClusteringResult struct {
	TagsCollected    int `json:"tags_collected"`
	EdgesFound       int `json:"edges_found"`
	ClustersFound    int `json:"clusters_found"`
	MergesApplied    int `json:"merges_applied"`
	AbstractsCreated int `json:"abstracts_created"`
	Errors           int `json:"errors"`
}
