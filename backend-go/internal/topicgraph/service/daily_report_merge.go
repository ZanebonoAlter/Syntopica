package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

func populateThreadArticles(threads []repository.Thread, tagArticleMap map[uint][]uint) {
	for i := range threads {
		seen := make(map[uint]bool)
		var articleIDs []uint
		for _, tagID := range threads[i].TagIDs {
			for _, artID := range tagArticleMap[tagID] {
				if !seen[artID] {
					seen[artID] = true
					articleIDs = append(articleIDs, artID)
				}
			}
		}
		threads[i].RelatedArticleIDs = articleIDs
	}
}
func parsePgVector(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(s, ",")
	result := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("parse vector element %d: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}
func cosineDistance(vec1, vec2 string) (float64, error) {
	v1, err := parsePgVector(vec1)
	if err != nil {
		return 1.0, fmt.Errorf("parse vec1: %w", err)
	}
	v2, err := parsePgVector(vec2)
	if err != nil {
		return 1.0, fmt.Errorf("parse vec2: %w", err)
	}
	if len(v1) != len(v2) || len(v1) == 0 {
		return 1.0, fmt.Errorf("vector dimension mismatch or empty: %d vs %d", len(v1), len(v2))
	}

	var dot, norm1, norm2 float64
	for i := range v1 {
		dot += v1[i] * v2[i]
		norm1 += v1[i] * v1[i]
		norm2 += v2[i] * v2[i]
	}
	if norm1 == 0 || norm2 == 0 {
		return 1.0, nil
	}
	similarity := dot / (math.Sqrt(norm1) * math.Sqrt(norm2))
	return 1.0 - similarity, nil
}

// sameAnchorClass reports whether two sections may merge under the anchor
// boundary (fix-section-merge-blackhole): both anchored to the same persistent
// topic, or both unanchored (the L3 new-narrative pool). Pairs mixing NULL and
// non-NULL anchors, or different topics, must never merge — the lane
// pipeline's keep/switch/new adjudication is a system record that the
// presentation-layer merge must not override. Filtering BEFORE edges are
// added guarantees every union-find component has a consistent anchor.
func sameAnchorClass(a, b repository.DailyReportSection) bool {
	if a.MatchedTopicID == nil && b.MatchedTopicID == nil {
		return true
	}
	if a.MatchedTopicID != nil && b.MatchedTopicID != nil {
		return *a.MatchedTopicID == *b.MatchedTopicID
	}
	return false
}

// logMergeCandidatePair is the Stage 1 audit line: labels, anchors, lanes,
// distance, and outcome for every same-day merge candidate pair evaluated
// (merged edge / rejected-by-boundary / gray-zone→LLM).
func logMergeCandidatePair(a, b repository.DailyReportSection, dist float64, outcome string) {
	anchor := func(s repository.DailyReportSection) string {
		if s.MatchedTopicID == nil {
			return "new"
		}
		return fmt.Sprintf("%d", *s.MatchedTopicID)
	}
	lane := func(s repository.DailyReportSection) string {
		if s.LaneTier == "" {
			return "-"
		}
		return s.LaneTier
	}
	logging.Infof("daily-report merge candidate: [%s|t%s|%s] vs [%s|t%s|%s] dist=%.4f → %s",
		a.ClusterLabel, anchor(a), lane(a), b.ClusterLabel, anchor(b), lane(b), dist, outcome)
}

func MergeSimilarSections(
	ctx context.Context,
	sections []repository.DailyReportSection,
	threadBatches [][]repository.DailyReportThread,
	tags []repository.TagInput,
	mergeEnabled bool,
) ([]repository.DailyReportSection, [][]repository.DailyReportThread, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.MergeSimilarSections")
	defer span.End()
	if !mergeEnabled {
		logging.Infof("daily-report: section merge disabled by config (daily_report_section_merge_enabled)")
		return sections, threadBatches, nil
	}
	if len(sections) <= 1 {
		return sections, threadBatches, nil
	}

	// Build tag ID → label and tag ID → score lookups
	tagLabelMap := make(map[uint]string)
	tagScoreMap := make(map[uint]float64)
	for _, t := range tags {
		tagLabelMap[t.ID] = t.Label
		tagScoreMap[t.ID] = t.Score
	}

	var grayZonePairs []llmMergePair                 // 0.20 - 0.25
	deterministicPairs := make(map[int]map[int]bool) // i → set of j's for dist < 0.20

	for i := 0; i < len(sections); i++ {
		if strings.TrimSpace(sections[i].Embedding) == "" {
			continue
		}
		for j := i + 1; j < len(sections); j++ {
			if strings.TrimSpace(sections[j].Embedding) == "" {
				continue
			}
			dist, err := cosineDistance(sections[i].Embedding, sections[j].Embedding)
			if err != nil {
				continue
			}
			// Anchor boundary: reject cross-anchor pairs before any edge is
			// created (deterministic band AND gray zone) so the union-find
			// transitive closure can never bridge different lane adjudications.
			if !sameAnchorClass(sections[i], sections[j]) {
				logMergeCandidatePair(sections[i], sections[j], dist, "rejected-by-boundary")
				continue
			}
			if dist < 0.20 {
				logMergeCandidatePair(sections[i], sections[j], dist, "merged")
				if deterministicPairs[i] == nil {
					deterministicPairs[i] = make(map[int]bool)
				}
				deterministicPairs[i][j] = true
			} else if dist < 0.25 {
				logMergeCandidatePair(sections[i], sections[j], dist, "gray-zone-llm")
				grayZonePairs = append(grayZonePairs, llmMergePair{i: i, j: j, distance: dist})
			}
		}
	}

	// Union-find for deterministic pairs (transitive closure)
	n := len(sections)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(x int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		px, py := find(x), find(y)
		if px != py {
			parent[px] = py
		}
	}

	for i, js := range deterministicPairs {
		for j := range js {
			union(i, j)
		}
	}

	// Stage 2: LLM arbitration for gray-zone pairs
	if len(grayZonePairs) > 0 {
		mergePairs, err := llmArbitrateMerges(ctx, sections, grayZonePairs, tagLabelMap)
		if err != nil {
			logging.Warnf("MergeSimilarSections: LLM arbitration failed: %v", err)
		} else {
			for _, p := range mergePairs {
				union(p.i, p.j)
			}
		}
	}

	// Group sections by root
	groups := make(map[int][]int) // root → indices
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	// If no merging happened, return as-is
	if len(groups) == n {
		return sections, threadBatches, nil
	}

	// Merge each group: keep highest article_count as primary
	var mergedSections []repository.DailyReportSection
	var mergedThreadBatches [][]repository.DailyReportThread

	for _, indices := range groups {
		if len(indices) == 1 {
			mergedSections = append(mergedSections, sections[indices[0]])
			if indices[0] < len(threadBatches) {
				mergedThreadBatches = append(mergedThreadBatches, threadBatches[indices[0]])
			} else {
				mergedThreadBatches = append(mergedThreadBatches, nil)
			}
			continue
		}

		// Find primary: highest article_count
		primaryIdx := indices[0]
		for _, idx := range indices[1:] {
			if sections[idx].ArticleCount > sections[primaryIdx].ArticleCount {
				primaryIdx = idx
			}
		}

		primary := sections[primaryIdx]

		// Merge tag IDs
		tagIDSet := make(map[uint]bool)
		var primaryTagIDs []uint
		_ = json.Unmarshal(primary.ClusterTagIDs, &primaryTagIDs)
		for _, id := range primaryTagIDs {
			tagIDSet[id] = true
		}

		totalArticles := primary.ArticleCount
		bestTier := primary.BestTier

		// Merge threads from secondary sections
		var allThreads []repository.DailyReportThread
		if primaryIdx < len(threadBatches) {
			allThreads = append(allThreads, threadBatches[primaryIdx]...)
		}

		for _, idx := range indices {
			if idx == primaryIdx {
				continue
			}
			sec := sections[idx]

			var secTagIDs []uint
			_ = json.Unmarshal(sec.ClusterTagIDs, &secTagIDs)
			for _, id := range secTagIDs {
				tagIDSet[id] = true
			}

			totalArticles += sec.ArticleCount
			if sec.BestTier < bestTier {
				bestTier = sec.BestTier
			}

			if idx < len(threadBatches) {
				allThreads = append(allThreads, threadBatches[idx]...)
			}
		}

		// Recompute merged tag IDs
		var mergedTagIDs []uint
		for id := range tagIDSet {
			mergedTagIDs = append(mergedTagIDs, id)
		}
		// marshalJSONArray (not json.Marshal): a nil slice serializes to the JSON
		// null scalar, which jsonb_array_elements_text cannot read (SQLSTATE 22023).
		mergedTagIDsJSON := marshalJSONArray(mergedTagIDs)

		// Precisely recompute avgScore from tag scores
		totalScore := 0.0
		scoreCount := 0
		for _, id := range mergedTagIDs {
			if s, ok := tagScoreMap[id]; ok {
				totalScore += s
				scoreCount++
			}
		}
		avgScore := 0.0
		if scoreCount > 0 {
			avgScore = totalScore / float64(scoreCount)
		}

		// Recompute quality_breakdown from merged tag IDs.
		breakdownJSON := buildQualityBreakdownJSON(tags, mergedTagIDs)

		primary.ClusterTagIDs = mergedTagIDsJSON
		primary.ArticleCount = totalArticles
		primary.BestTier = bestTier
		primary.AvgScore = avgScore
		primary.QualityBreakdown = breakdownJSON

		mergedSections = append(mergedSections, primary)
		mergedThreadBatches = append(mergedThreadBatches, allThreads)
	}

	logging.Infof("MergeSimilarSections: merged %d sections → %d", n, len(mergedSections))
	return mergedSections, mergedThreadBatches, nil
}
