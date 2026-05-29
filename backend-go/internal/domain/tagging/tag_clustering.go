package tagging

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
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
	KwMinOverlap        int
	SemThreshold        float64
}

var DefaultClusterConfig = ClusterConfig{
	MaxTags:             500,
	SimilarityThreshold: 0.85,
	MaxClusterSize:      8,
	KwMinOverlap:        2,
	SemThreshold:        0.80,
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
	if v, ok := config["event_cluster_kw_min_overlap"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.KwMinOverlap = n
		}
	}
	if v, ok := config["event_cluster_sem_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.SemThreshold = f
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

func FindSimilarTagsByKeywordOverlap(ctx context.Context, tagIDs []uint, kwMinOverlap int, semThreshold float64) ([]SimilarityEdge, []SimilarityEdge, error) {
	if len(tagIDs) < 2 {
		return nil, nil, nil
	}

	// Stage 1: keyword overlap via SQL jsonb_array_elements_text intersection
	type keywordPair struct {
		TagAID    uint `gorm:"column:tag_a_id"`
		TagBID    uint `gorm:"column:tag_b_id"`
		SharedKws int  `gorm:"column:shared_kws"`
	}
	var kwRows []keywordPair
	kwQuery := `
		SELECT a.id AS tag_a_id, b.id AS tag_b_id,
		       (SELECT COUNT(*) FROM jsonb_array_elements_text(a.metadata->'event_keywords') akw
		        WHERE akw IN (SELECT jsonb_array_elements_text(b.metadata->'event_keywords'))) AS shared_kws
		FROM topic_tags a
		JOIN topic_tags b ON a.id < b.id
		WHERE a.id IN ? AND b.id IN ?
		  AND a.metadata IS NOT NULL AND a.metadata::jsonb ? 'event_keywords'
		  AND b.metadata IS NOT NULL AND b.metadata::jsonb ? 'event_keywords'
	`
	if err := database.DB.Raw(kwQuery, tagIDs, tagIDs).Scan(&kwRows).Error; err != nil {
		return nil, nil, fmt.Errorf("keyword overlap query: %w", err)
	}

	kwEdges := make([]SimilarityEdge, 0, len(kwRows))
	var candidatePairs []struct{ a, b uint }
	for _, r := range kwRows {
		kwEdges = append(kwEdges, SimilarityEdge{
			TagAID:     r.TagAID,
			TagBID:     r.TagBID,
			Similarity: float64(r.SharedKws),
		})
		if r.SharedKws >= kwMinOverlap {
			candidatePairs = append(candidatePairs, struct{ a, b uint }{r.TagAID, r.TagBID})
		}
	}
	logging.Infof("FindSimilarTagsByKeywordOverlap: %d keyword pairs, %d passed kw_overlap >= %d",
		len(kwRows), len(candidatePairs), kwMinOverlap)

	if len(candidatePairs) == 0 {
		return kwEdges, nil, nil
	}

	// Stage 2: semantic filter via pgvector cosine distance on topic_tag_embeddings
	type semRow struct {
		TagAID   uint    `gorm:"column:tag_a_id"`
		TagBID   uint    `gorm:"column:tag_b_id"`
		Distance float64 `gorm:"column:distance"`
	}
	var semRows []semRow
	semQuery := `
		SELECT kp.tag_a_id, kp.tag_b_id, ea.embedding <=> eb.embedding AS distance
		FROM (VALUES `
	args := make([]interface{}, 0, len(candidatePairs)*2+1)
	for i, p := range candidatePairs {
		if i > 0 {
			semQuery += ", "
		}
		semQuery += "(?::bigint, ?::bigint)"
		args = append(args, p.a, p.b)
	}
	semQuery += `) AS kp(tag_a_id, tag_b_id)
		JOIN topic_tag_embeddings ea ON ea.topic_tag_id = kp.tag_a_id AND ea.embedding_type = 'semantic'
		JOIN topic_tag_embeddings eb ON eb.topic_tag_id = kp.tag_b_id AND eb.embedding_type = 'semantic'
		WHERE ea.embedding IS NOT NULL AND eb.embedding IS NOT NULL
		  AND ea.embedding <=> eb.embedding < ?
	`
	args = append(args, 1.0-semThreshold)
	if err := database.DB.Raw(semQuery, args...).Scan(&semRows).Error; err != nil {
		return nil, nil, fmt.Errorf("semantic filter query: %w", err)
	}

	semEdges := make([]SimilarityEdge, 0, len(semRows))
	for _, r := range semRows {
		semEdges = append(semEdges, SimilarityEdge{
			TagAID:     r.TagAID,
			TagBID:     r.TagBID,
			Similarity: 1.0 - r.Distance,
		})
	}
	logging.Infof("FindSimilarTagsByKeywordOverlap: %d pairs passed semantic filter (threshold=%.2f)",
		len(semEdges), semThreshold)

	return kwEdges, semEdges, nil
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

	var edges []SimilarityEdge
	var kwEdges []SimilarityEdge

	if category == "event" {
		kwEdges, edges, err = FindSimilarTagsByKeywordOverlap(ctx, tagIDs, cfg.KwMinOverlap, cfg.SemThreshold)
		if err != nil {
			return nil, fmt.Errorf("keyword-overlap similarity search for %s: %w", category, err)
		}
		result.EdgesFound = len(edges)
		result.EventKeywordEdgesFound = len(kwEdges)
		logging.Infof("ClusterUnclassifiedTags(%s): %d keyword-overlap edges, %d passed semantic filter (kw_min=%d, sem=%.2f)",
			category, len(kwEdges), len(edges), cfg.KwMinOverlap, cfg.SemThreshold)
	} else {
		edges, err = es.FindSimilarTagsAmongSet(ctx, tagIDs, cfg.SimilarityThreshold)
		if err != nil {
			return nil, fmt.Errorf("similarity search for %s: %w", category, err)
		}
		result.EdgesFound = len(edges)
		logging.Infof("ClusterUnclassifiedTags(%s): found %d similarity edges (threshold=%.2f)", category, len(edges), cfg.SimilarityThreshold)
	}

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

		var compIDs []uint
		for _, id := range comp {
			tag := tagsMap[id]
			if tag == nil {
				continue
			}
			compIDs = append(compIDs, id)
		}
		if len(compIDs) < 2 {
			continue
		}

		dateRanges := loadTagDateRanges(compIDs)

		var candidates []TagCandidate
		for _, id := range compIDs {
			tag := tagsMap[id]
			if tag == nil {
				continue
			}
			candidates = append(candidates, TagCandidate{
				Tag:        tag,
				Similarity: 1.0,
				DateRange:  dateRanges[id],
			})
		}
		if len(candidates) < 2 {
			continue
		}

		logging.Infof("ClusterUnclassifiedTags(%s): cluster of %d tags (labels: %s)",
			category, len(candidates), candidates[0].Tag.Label)
		result.AbstractsCreated++
	}

	return result, nil
}

type ClusteringResult struct {
	TagsCollected          int `json:"tags_collected"`
	EdgesFound             int `json:"edges_found"`
	EventKeywordEdgesFound int `json:"event_keyword_edges_found"`
	ClustersFound          int `json:"clusters_found"`
	MergesApplied          int `json:"merges_applied"`
	AbstractsCreated       int `json:"abstracts_created"`
	Errors                 int `json:"errors"`
}

func loadTagDateRanges(tagIDs []uint) map[uint]string {
	if len(tagIDs) == 0 {
		return nil
	}

	type row struct {
		TagID   uint   `gorm:"column:topic_tag_id"`
		MinDate string `gorm:"column:min_date"`
		MaxDate string `gorm:"column:max_date"`
	}
	var rows []row
	if err := database.DB.Raw(`
		SELECT att.topic_tag_id,
		       MIN(a.pub_date) AS min_date,
		       MAX(a.pub_date) AS max_date
		FROM article_topic_tags att
		JOIN articles a ON a.id = att.article_id
		WHERE att.topic_tag_id IN ?
		  AND a.pub_date IS NOT NULL
		GROUP BY att.topic_tag_id
	`, tagIDs).Scan(&rows).Error; err != nil {
		logging.Warnf("loadTagDateRanges: %v", err)
		return nil
	}

	result := make(map[uint]string, len(rows))
	for _, r := range rows {
		minDate := r.MinDate
		maxDate := r.MaxDate
		if minDate != "" {
			minDate = minDate[:10]
		}
		if maxDate != "" {
			maxDate = maxDate[:10]
		}
		if minDate == maxDate {
			result[r.TagID] = fmt.Sprintf("(文章日期: %s)", minDate)
		} else if minDate != "" && maxDate != "" {
			result[r.TagID] = fmt.Sprintf("(最早文章: %s, 最新: %s)", minDate, maxDate)
		}
	}
	return result
}
