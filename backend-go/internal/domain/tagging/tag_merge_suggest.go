package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"syntopica-backend/internal/domain/models"
	airouter "syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm/clause"
)

// ScanProgress represents the current progress of a full scan.
type ScanProgress struct {
	Status          string `json:"status"`            // scanning, done, error
	Total           int    `json:"total"`
	Scanned         int    `json:"scanned"`
	CurrentCategory string `json:"current_category"`
	NewSuggestions  int    `json:"new_suggestions"`
	Error           string `json:"error,omitempty"`
}

// scanState manages the global full-scan singleton.
var scanState struct {
	mu       sync.Mutex
	running  atomic.Bool
	progress chan ScanProgress
	cancel   context.CancelFunc
}

// IsScanRunning returns whether a full scan is currently in progress.
func IsScanRunning() bool {
	return scanState.running.Load()
}

// StartFullScan starts an asynchronous full scan of all tags.
// Returns false if a scan is already running.
func StartFullScan() bool {
	scanState.mu.Lock()
	defer scanState.mu.Unlock()

	if scanState.running.Load() {
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	scanState.cancel = cancel
	scanState.progress = make(chan ScanProgress, 32)
	scanState.running.Store(true)

	go runFullScan(ctx)

	return true
}

// GetScanProgressChannel returns the channel for SSE streaming.
func GetScanProgressChannel() <-chan ScanProgress {
	scanState.mu.Lock()
	defer scanState.mu.Unlock()
	return scanState.progress
}

// runFullScan executes the full scan in a background goroutine.
func runFullScan(ctx context.Context) {
	defer func() {
		scanState.running.Store(false)
		close(scanState.progress)
	}()

	es := getEmbeddingService()
	if es == nil {
		scanState.progress <- ScanProgress{Status: "error", Error: "embedding service unavailable"}
		return
	}

	// Load all active tags
	var tags []models.TopicTag
	if err := database.DB.Where("status = 'active' OR status = '' OR status IS NULL").Find(&tags).Error; err != nil {
		scanState.progress <- ScanProgress{Status: "error", Error: err.Error()}
		return
	}

	total := len(tags)
	thresholds := DefaultThresholds
	newSuggestions := 0

	for i, tag := range tags {
		select {
		case <-ctx.Done():
			scanState.progress <- ScanProgress{Status: "error", Error: "cancelled"}
			return
		default:
		}

		candidates, err := es.FindSimilarTags(ctx, &tag, tag.Category, 10, EmbeddingTypeSemantic)
		if err != nil {
			logging.Warnf("runFullScan: FindSimilarTags failed for tag %d: %v", tag.ID, err)
			continue
		}

		for _, c := range candidates {
			if c.Similarity < thresholds.LowSimilarity {
				continue
			}
			if c.Tag.ID == tag.ID {
				continue
			}

			// Normalize direction: smaller ID always as new_tag_id to avoid A→B and B→A duplicates
			var newID, existingID uint
			var newLbl, existingLbl string
			if tag.ID < c.Tag.ID {
				newID, existingID = tag.ID, c.Tag.ID
				newLbl, existingLbl = tag.Label, c.Tag.Label
			} else {
				newID, existingID = c.Tag.ID, tag.ID
				newLbl, existingLbl = c.Tag.Label, tag.Label
			}

			suggestion := models.TagMergeSuggestion{
				NewTagID:      newID,
				ExistingTagID: existingID,
				NewLabel:      newLbl,
				ExistingLabel: existingLbl,
				Category:      tag.Category,
				Similarity:    c.Similarity,
				Status:        "pending",
				Source:        "full_scan",
			}

			result := database.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
				DoNothing: true,
			}).Create(&suggestion)

			if result.RowsAffected > 0 {
				newSuggestions++
			}
		}

		// Send progress every 10 tags or on the last tag
		if (i+1)%10 == 0 || i+1 == total {
			scanState.progress <- ScanProgress{
				Status:          "scanning",
				Total:           total,
				Scanned:         i + 1,
				CurrentCategory: tag.Category,
				NewSuggestions:  newSuggestions,
			}
		}
	}

	scanState.progress <- ScanProgress{
		Status:         "done",
		Total:          total,
		Scanned:        total,
		NewSuggestions: newSuggestions,
	}
}

// --- LLM Evaluation of merge suggestions ---

// EvaluateProgress represents progress of LLM evaluation of merge suggestions.
type EvaluateProgress struct {
	Status        string `json:"status"`          // evaluating, done, error
	TotalGroups   int    `json:"total_groups"`
	Completed     int    `json:"completed"`
	CurrentTarget string `json:"current_target"`  // label of the target tag being evaluated
	Error         string `json:"error,omitempty"`
}

// evalState manages the global evaluation singleton.
var evalState struct {
	mu       sync.Mutex
	running  atomic.Bool
	cancel   context.CancelFunc
	progress chan EvaluateProgress
}

// IsEvaluateRunning returns whether LLM evaluation is currently in progress.
func IsEvaluateRunning() bool {
	return evalState.running.Load()
}

// StartEvaluation starts an asynchronous LLM evaluation of pending merge suggestions.
// Returns false if evaluation is already running.
func StartEvaluation() bool {
	evalState.mu.Lock()
	defer evalState.mu.Unlock()

	if evalState.running.Load() {
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	evalState.cancel = cancel
	evalState.progress = make(chan EvaluateProgress, 32)
	evalState.running.Store(true)

	go runEvaluation(ctx)

	return true
}

// GetEvaluateProgressChannel returns the channel for SSE streaming.
func GetEvaluateProgressChannel() <-chan EvaluateProgress {
	evalState.mu.Lock()
	defer evalState.mu.Unlock()
	return evalState.progress
}

// CancelEvaluation cancels a running LLM evaluation.
func CancelEvaluation() {
	evalState.mu.Lock()
	defer evalState.mu.Unlock()
	if evalState.cancel != nil {
		evalState.cancel()
	}
}

func runEvaluation(ctx context.Context) {
	defer func() {
		evalState.running.Store(false)
		close(evalState.progress)
	}()

	// Query pending suggestions, grouped by existing_tag_id
	var suggestions []models.TagMergeSuggestion
	if err := database.DB.Where("status = ?", "pending").Find(&suggestions).Error; err != nil {
		evalState.progress <- EvaluateProgress{Status: "error", Error: err.Error()}
		return
	}

	if len(suggestions) == 0 {
		evalState.progress <- EvaluateProgress{Status: "done", TotalGroups: 0, Completed: 0}
		return
	}

	// Group by existing_tag_id
	type groupInfo struct {
		ExistingTag models.TopicTag
		Suggestions []models.TagMergeSuggestion
	}
	groups := make(map[uint]*groupInfo)
	var groupOrder []uint

	for _, sug := range suggestions {
		if _, ok := groups[sug.ExistingTagID]; !ok {
			var tag models.TopicTag
			if err := database.DB.Select("id, label, slug, category, feed_count").First(&tag, sug.ExistingTagID).Error; err != nil {
				continue
			}
			groups[sug.ExistingTagID] = &groupInfo{ExistingTag: tag}
			groupOrder = append(groupOrder, sug.ExistingTagID)
		}
		groups[sug.ExistingTagID].Suggestions = append(groups[sug.ExistingTagID].Suggestions, sug)
	}

	totalGroups := len(groupOrder)

	// Pre-compute article counts for all tags
	articleCounts := make(map[uint]int64)
	allTagIDs := make([]uint, 0, len(groupOrder)*3)
	for _, existingID := range groupOrder {
		allTagIDs = append(allTagIDs, existingID)
		for _, sug := range groups[existingID].Suggestions {
			allTagIDs = append(allTagIDs, sug.NewTagID)
		}
	}
	if len(allTagIDs) > 0 {
		type countRow struct {
			TopicTagID uint
			Count      int64
		}
		var rows []countRow
		database.DB.Model(&models.ArticleTopicTag{}).
			Select("topic_tag_id, count(*) as count").
			Where("topic_tag_id IN ?", allTagIDs).
			Group("topic_tag_id").
			Find(&rows)
		for _, r := range rows {
			articleCounts[r.TopicTagID] = r.Count
		}
	}

	// Batch groups into chunks for fewer LLM calls
	const batchSize = 10
	router := airouter.NewRouter()

	for batchStart := 0; batchStart < totalGroups; batchStart += batchSize {
		select {
		case <-ctx.Done():
			evalState.progress <- EvaluateProgress{Status: "error", Error: "cancelled"}
			return
		default:
		}

		batchEnd := batchStart + batchSize
		if batchEnd > totalGroups {
			batchEnd = totalGroups
		}
		batchIDs := groupOrder[batchStart:batchEnd]

		evalState.progress <- EvaluateProgress{
			Status:        "evaluating",
			TotalGroups:   totalGroups,
			Completed:     batchStart,
			CurrentTarget: groups[batchIDs[0]].ExistingTag.Label,
		}

		// Build multi-group prompt
		var groupSections []string
		for _, existingID := range batchIDs {
			group := groups[existingID]
			targetLabel := group.ExistingTag.Label
			targetCount := articleCounts[existingID]

			var candidateLines []string
			for _, sug := range group.Suggestions {
				newCount := articleCounts[sug.NewTagID]
				candidateLines = append(candidateLines, fmt.Sprintf("- %s (id:%d)，相似度 %.2f，有 %d 篇文章", sug.NewLabel, sug.NewTagID, sug.Similarity, newCount))
			}

			groupSections = append(groupSections, fmt.Sprintf(`## 目标标签：%s (id:%d，%d 篇文章)
候选：
%s`, targetLabel, existingID, targetCount, strings.Join(candidateLines, "\n")))
		}

		prompt := fmt.Sprintf(`你是标签合并专家。以下是多组需要评估的标签合并建议。对每个候选标签判断是否应合并到对应目标标签。

%s

输出纯 JSON：
{
  "verdicts": [
    {
      "target_tag_id": 0,
      "new_tag_id": 0,
      "should_merge": true,
      "suggested_name": "合并后的名称",
      "reason": "理由"
    }
  ]
}`, strings.Join(groupSections, "\n\n"))

		maxTokens := 4000
		temperature := 0.2
		resp, err := router.Chat(ctx, airouter.ChatRequest{
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: "你是标签合并专家。只输出合法 JSON，不要额外解释。"},
				{Role: "user", Content: prompt},
			},
			MaxTokens:   &maxTokens,
			Temperature: &temperature,
			Metadata: map[string]any{
				"source": "merge_suggestion_evaluation",
			},
		})

		if err != nil {
			logging.Warnf("runEvaluation: LLM call failed for batch %d-%d: %v", batchStart, batchEnd, err)
			continue
		}

		verdicts := parseEvaluateResponse(resp.Content)
		if verdicts == nil {
			logging.Warnf("runEvaluation: failed to parse LLM response for batch %d-%d", batchStart, batchEnd)
			continue
		}

		// Update suggestions with verdicts
		for _, existingID := range batchIDs {
			for _, sug := range groups[existingID].Suggestions {
				key := verdictKey{TargetTagID: existingID, NewTagID: sug.NewTagID}
				if v, ok := verdicts[key]; ok {
					verdictJSON, _ := json.Marshal(v)
					database.DB.Model(&models.TagMergeSuggestion{}).
						Where("id = ?", sug.ID).
						Update("llm_verdict", string(verdictJSON))
				}
			}
		}
	}

	evalState.progress <- EvaluateProgress{
		Status:      "done",
		TotalGroups: totalGroups,
		Completed:   totalGroups,
	}
}

type verdictKey struct {
	TargetTagID uint
	NewTagID    uint
}

type tagVerdict struct {
	TargetTagID   uint   `json:"target_tag_id"`
	NewTagID      uint   `json:"new_tag_id"`
	ShouldMerge   bool   `json:"should_merge"`
	SuggestedName string `json:"suggested_name"`
	Reason        string `json:"reason"`
}

func parseEvaluateResponse(content string) map[verdictKey]*tagVerdict {
	raw := extractEvalJSON(content)
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var result struct {
		Verdicts []tagVerdict `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		logging.Warnf("parseEvaluateResponse: JSON parse error: %v", err)
		return nil
	}

	m := make(map[verdictKey]*tagVerdict)
	for i := range result.Verdicts {
		key := verdictKey{TargetTagID: result.Verdicts[i].TargetTagID, NewTagID: result.Verdicts[i].NewTagID}
		m[key] = &result.Verdicts[i]
	}
	return m
}

func extractEvalJSON(content string) string {
	trimmed := strings.TrimSpace(content)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return ""
	}
	return trimmed[start : end+1]
}

// RecordMergeSuggestions writes candidate pairs to tag_merge_suggestions.
// Skips pairs that already exist (by unique constraint new_tag_id + existing_tag_id).
func RecordMergeSuggestions(newTagID uint, newLabel string, category string, candidates []TagCandidate) {
	if len(candidates) == 0 {
		return
	}

	for _, c := range candidates {
		// Normalize direction: smaller ID always as new_tag_id
		var nID, eID uint
		var nLbl, eLbl string
		if newTagID < c.Tag.ID {
			nID, eID = newTagID, c.Tag.ID
			nLbl, eLbl = newLabel, c.Tag.Label
		} else {
			nID, eID = c.Tag.ID, newTagID
			nLbl, eLbl = c.Tag.Label, newLabel
		}

		suggestion := models.TagMergeSuggestion{
			NewTagID:      nID,
			ExistingTagID: eID,
			NewLabel:      nLbl,
			ExistingLabel: eLbl,
			Category:      category,
			Similarity:    c.Similarity,
			Status:        "pending",
			Source:        "incremental",
		}

		result := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
			DoNothing: true,
		}).Create(&suggestion)

		if result.Error != nil {
			logging.Warnf("RecordMergeSuggestions: failed to write suggestion new=%d existing=%d: %v", newTagID, c.Tag.ID, result.Error)
		} else if result.RowsAffected == 0 {
			logging.Infof("RecordMergeSuggestions: skipped duplicate new=%d existing=%d", newTagID, c.Tag.ID)
		}
	}
}
