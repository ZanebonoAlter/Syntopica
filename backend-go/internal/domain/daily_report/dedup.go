package daily_report

import (
	"fmt"
	"sort"
)

// DeduplicateTags removes duplicate tags based on article ID overlap.
// Two rules:
//  1. Tags with identical article ID sets → merge (keep the one with higher
//     article_count or lower ID).
//  2. Tags with article_count=1 sharing the same article → merge.
//
// The function takes a slice of TagInput plus a parallel slice of article ID sets
// (one per tag, in the same order) and returns the deduplicated slice along with
// the surviving indices.
func DeduplicateTags(tags []TagInput, articleIDSets [][]uint) []TagInput {
	if len(tags) == 0 {
		return nil
	}
	if len(articleIDSets) != len(tags) {
		return tags
	}

	n := len(tags)
	// Build a set-key for each tag: sorted, deduplicated article IDs → compact string key.
	// We use index-based maps to track which tags survive.
	alive := make([]bool, n)
	for i := range alive {
		alive[i] = true
	}

	// Map from article set signature to the "best" tag index.
	// For single-article tags, also track by single article ID.
	setSigToIdx := make(map[string]int)   // full set signature → best index
	singleArtToIdx := make(map[uint]int)   // single article ID → best index

	for i, tag := range tags {
		arts := dedupAndSort(articleIDSets[i])

		if len(arts) == 0 {
			alive[i] = false
			continue
		}

		sig := uintSliceKey(arts)

		if existingIdx, ok := setSigToIdx[sig]; ok {
			// Rule 1: identical article sets → keep the better one.
			if shouldReplace(tags[existingIdx], tag) {
				alive[existingIdx] = false
				setSigToIdx[sig] = i
			} else {
				alive[i] = false
			}
			continue
		}
		setSigToIdx[sig] = i

		// Rule 2: single-article tags sharing the same article.
		if len(arts) == 1 {
			artID := arts[0]
			if existingIdx, ok := singleArtToIdx[artID]; ok {
				if shouldReplace(tags[existingIdx], tag) {
					alive[existingIdx] = false
					singleArtToIdx[artID] = i
					// Also update setSigToIdx if it still pointed to old index.
					if setSigToIdx[sig] == existingIdx {
						setSigToIdx[sig] = i
					}
				} else {
					alive[i] = false
					if setSigToIdx[sig] == i {
						delete(setSigToIdx, sig)
					}
				}
				continue
			}
			singleArtToIdx[artID] = i
		}
	}

	var result []TagInput
	for i, a := range alive {
		if a {
			result = append(result, tags[i])
		}
	}
	return result
}

// shouldReplace returns true if challenger should replace incumbent.
// Prefer higher article_count, then lower ID.
func shouldReplace(incumbent, challenger TagInput) bool {
	if challenger.ArticleCount > incumbent.ArticleCount {
		return true
	}
	if challenger.ArticleCount == incumbent.ArticleCount && challenger.ID < incumbent.ID {
		return true
	}
	return false
}

func dedupAndSort(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]bool, len(ids))
	var result []uint
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uintSliceKey(ids []uint) string {
	return fmt.Sprintf("%v", ids)
}
