package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ProcessPendingAdoptNarrowerTasks() (int, error) {
	var tasks []models.AdoptNarrowerQueue
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ?", models.AdoptNarrowerQueueStatusPending).
			Order("created_at ASC").
			Limit(50).
			Find(&tasks).Error; err != nil {
			return err
		}

		if len(tasks) == 0 {
			return nil
		}

		now := time.Now()
		var taskIDs []uint
		for _, t := range tasks {
			taskIDs = append(taskIDs, t.ID)
		}

		if err := tx.Model(&models.AdoptNarrowerQueue{}).
			Where("id IN ? AND status = ?", taskIDs, models.AdoptNarrowerQueueStatusPending).
			Updates(map[string]interface{}{
				"status":     models.AdoptNarrowerQueueStatusProcessing,
				"started_at": now,
			}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	if len(tasks) == 0 {
		return 0, nil
	}

	logging.Infof("adopt narrower batch: found %d pending tasks", len(tasks))

	es := NewEmbeddingService()

	var enriched []adoptTaskWithCandidates
	for _, task := range tasks {
		var abstractTag models.TopicTag
		if err := database.DB.First(&abstractTag, task.AbstractTagID).Error; err != nil {
			markAdoptNarrowerFailed(task.ID, err.Error())
			continue
		}

		candidates, err := es.FindSimilarAbstractTags(context.Background(), task.AbstractTagID, abstractTag.Category, 0)
		if err != nil {
			markAdoptNarrowerFailed(task.ID, err.Error())
			continue
		}

		thresholds := es.GetThresholds()
		var eligible []TagCandidate
		for _, c := range candidates {
			if c.Tag != nil && c.Similarity >= thresholds.LowSimilarity {
				eligible = append(eligible, c)
			}
		}

		enriched = append(enriched, adoptTaskWithCandidates{
			AbstractTagID: task.AbstractTagID,
			Label:         abstractTag.Label,
			Category:      abstractTag.Category,
			Candidates:    eligible,
			TaskModel:     task,
		})
	}

	batchSize := 5
	batches := groupAdoptTasksByCategory(enriched, batchSize)

	for _, e := range enriched {
		if len(e.Candidates) == 0 {
			now := time.Now()
			err := database.DB.Model(&models.AdoptNarrowerQueue{}).
				Where("id = ?", e.TaskModel.ID).
				Updates(map[string]interface{}{
					"status":       models.AdoptNarrowerQueueStatusCompleted,
					"completed_at": now,
				}).Error
			if err != nil {
				logging.Warnf("adopt narrower: failed to mark zero-candidate task %d completed: %v", e.TaskModel.ID, err)
			}
		}
	}

	processed := 0
	for _, batch := range batches {
		judgment, err := batchJudgeAdoptNarrower(context.Background(), batch)
		if err != nil {
			logging.Warnf("adopt narrower batch LLM failed: %v", err)
			for _, t := range batch {
				markAdoptNarrowerFailed(t.TaskModel.ID, err.Error())
			}
			continue
		}

		judgmentMap := make(map[uint][]uint)
		if judgment != nil {
			for _, r := range judgment.Results {
				judgmentMap[r.AbstractTagID] = r.NarrowerIDs
			}
		}

		for _, t := range batch {
			narrowerIDs, ok := judgmentMap[t.AbstractTagID]
			if !ok {
				narrowerIDs = []uint{}
			}

			adopted := 0
			for _, cid := range narrowerIDs {
				if err := reparentOrLinkAbstractChild(context.Background(), cid, t.AbstractTagID); err != nil {
					logging.Warnf("adopt narrower batch: failed to link %d under %d: %v", cid, t.AbstractTagID, err)
					continue
				}
				adopted++
			}

			if adopted > 0 {
				EnqueueAbstractTagUpdate(t.AbstractTagID, "adopted_narrower_children")
			}

			now := time.Now()
			err := database.DB.Model(&models.AdoptNarrowerQueue{}).
				Where("id = ?", t.TaskModel.ID).
				Updates(map[string]interface{}{
					"status":       models.AdoptNarrowerQueueStatusCompleted,
					"completed_at": now,
				}).Error
			if err != nil {
				logging.Warnf("adopt narrower: failed to mark task %d completed: %v", t.TaskModel.ID, err)
			}
			processed++
		}
	}

	logging.Infof("adopt narrower batch: processed %d/%d tasks", processed, len(tasks))
	return processed, nil
}

func ProcessPendingAbstractTagUpdateTasks() (int, error) {
	var tasks []models.AbstractTagUpdateQueue
	if err := database.DB.
		Where("status = ?", models.AbstractTagUpdateQueueStatusPending).
		Order("created_at ASC").
		Limit(50).
		Find(&tasks).Error; err != nil {
		return 0, err
	}

	if len(tasks) == 0 {
		return 0, nil
	}

	logging.Infof("abstract tag update batch: found %d pending tasks", len(tasks))

	svc := NewAbstractTagUpdateQueueService(nil)

	var entries []abstractTagWithChildren
	for _, task := range tasks {
		var tag models.TopicTag
		if err := database.DB.First(&tag, task.AbstractTagID).Error; err != nil {
			markAbstractTagUpdateFailed(task.ID, err.Error())
			continue
		}

		children, err := svc.loadChildren(task.AbstractTagID)
		if err != nil {
			markAbstractTagUpdateFailed(task.ID, err.Error())
			continue
		}
		if len(children) == 0 {
			now := time.Now()
			err := database.DB.Model(&models.AbstractTagUpdateQueue{}).
				Where("id = ?", task.ID).
				Updates(map[string]interface{}{
					"status":       models.AbstractTagUpdateQueueStatusCompleted,
					"completed_at": now,
				}).Error
			if err != nil {
				logging.Warnf("abstract tag update: failed to mark zero-children task %d completed: %v", task.ID, err)
			}
			continue
		}

		entries = append(entries, abstractTagWithChildren{
			Tag:      tag,
			Children: children,
			TaskID:   task.ID,
		})
	}

	batchSize := 5
	processed := 0
	embSvc := NewEmbeddingService()
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		judgment, err := batchRegenerateLabelsAndDescriptions(context.Background(), batch)
		if err != nil {
			logging.Warnf("abstract tag update batch LLM failed: %v", err)
			for _, e := range batch {
				markAbstractTagUpdateFailed(e.TaskID, err.Error())
			}
			continue
		}

		resultMap := make(map[uint]struct {
			Label       string
			Description string
		})
		if judgment != nil {
			for _, r := range judgment.Results {
				resultMap[r.AbstractTagID] = struct {
					Label       string
					Description string
				}{Label: r.Label, Description: r.Description}
			}
		}

		for _, e := range batch {
			r, ok := resultMap[e.Tag.ID]
			if !ok {
				markAbstractTagUpdateFailed(e.TaskID, "no result in batch response")
				continue
			}

			updates := map[string]interface{}{}
			if r.Description != "" && r.Description != e.Tag.Description {
				updates["description"] = r.Description
			}

			if r.Label != "" && r.Label != e.Tag.Label {
				newSlug := Slugify(r.Label)
				if newSlug != "" && newSlug != e.Tag.Slug && !isAbstractRoot(database.DB, e.Tag.ID) {
					var conflictCount int64
					database.DB.Model(&models.TopicTag{}).
						Where("slug = ? AND id != ? AND status = ?", newSlug, e.Tag.ID, "active").
						Count(&conflictCount)
					if conflictCount == 0 {
						updates["label"] = r.Label
						updates["slug"] = newSlug
					}
				}
			}

			if len(updates) > 0 {
				if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", e.Tag.ID).Updates(updates).Error; err != nil {
					markAbstractTagUpdateFailed(e.TaskID, err.Error())
					continue
				}
			}

			emb, err := embSvc.GenerateEmbedding(context.Background(), &e.Tag, EmbeddingTypeIdentity)
			if err == nil {
				emb.TopicTagID = e.Tag.ID
				_ = embSvc.SaveEmbedding(emb)
			}

			now := time.Now()
			err = database.DB.Model(&models.AbstractTagUpdateQueue{}).
				Where("id = ?", e.TaskID).
				Updates(map[string]interface{}{
					"status":       models.AbstractTagUpdateQueueStatusCompleted,
					"completed_at": now,
				}).Error
			if err != nil {
				logging.Warnf("abstract tag update: failed to mark task %d completed: %v", e.TaskID, err)
			}
			processed++
		}
	}

	logging.Infof("abstract tag update batch: processed %d/%d tasks", processed, len(tasks))
	return processed, nil
}

func markAdoptNarrowerFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := database.DB.Model(&models.AdoptNarrowerQueue{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.AdoptNarrowerQueueStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"retry_count":   gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
		logging.Warnf("adopt narrower batch: failed to mark task %d failed: %v", taskID, err)
	}
}

func markAbstractTagUpdateFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := database.DB.Model(&models.AbstractTagUpdateQueue{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.AbstractTagUpdateQueueStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"retry_count":   gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
		logging.Warnf("abstract tag update batch: failed to mark task %d failed: %v", taskID, err)
	}
}

type adoptTaskWithCandidates struct {
	AbstractTagID uint
	Label         string
	Category      string
	Candidates    []TagCandidate
	TaskModel     models.AdoptNarrowerQueue
}

func groupAdoptTasksByCategory(tasks []adoptTaskWithCandidates, batchSize int) [][]adoptTaskWithCandidates {
	var withCandidates []adoptTaskWithCandidates
	for _, t := range tasks {
		if len(t.Candidates) > 0 {
			withCandidates = append(withCandidates, t)
		}
	}
	var batches [][]adoptTaskWithCandidates
	for i := 0; i < len(withCandidates); i += batchSize {
		end := i + batchSize
		if end > len(withCandidates) {
			end = len(withCandidates)
		}
		batches = append(batches, withCandidates[i:end])
	}
	return batches
}

type batchAdoptJudgment struct {
	Results []struct {
		AbstractTagID uint   `json:"abstract_tag_id"`
		NarrowerIDs   []uint `json:"narrower_ids"`
	} `json:"results"`
}

func batchJudgeAdoptNarrower(ctx context.Context, batch []adoptTaskWithCandidates) (*batchAdoptJudgment, error) {
	if len(batch) == 0 {
		return nil, nil
	}

	var entries []string
	for i, t := range batch {
		var candParts []string
		for _, c := range t.Candidates {
			if c.Tag == nil {
				continue
			}
			candParts = append(candParts, fmt.Sprintf("%q (相似度: %.4f)", c.Tag.Label, c.Similarity))
		}
		entries = append(entries, fmt.Sprintf("%d. 抽象标签 %q (ID:%d): 候选 [%s]",
			i+1, t.Label, t.AbstractTagID, strings.Join(candParts, ", ")))
	}

	prompt := fmt.Sprintf(`判断以下多个抽象标签各自应该收养哪些候选作为更窄概念子标签。

抽象标签及候选:
%s

规则:
- 对每个抽象标签，判断哪些候选是其更窄（更具体）的概念
- 如果候选与抽象标签同级或更宽泛，不选
- 如果候选的子标签与抽象标签的子标签高度重叠，不选
- 可以选择零个、一个或多个

返回 JSON: {"results": [{"abstract_tag_id": ID, "narrower_ids": [候选标签ID列表]}, ...]}`,
		strings.Join(entries, "\n"))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"results": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"abstract_tag_id": {Type: "integer"},
							"narrower_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
						},
						Required: []string{"abstract_tag_id", "narrower_ids"},
					},
				},
			},
			Required: []string{"results"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":  "adopt_narrower_batch",
			"batch_size": len(batch),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch adopt narrower LLM: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment batchAdoptJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse batch adopt response: %w", err)
	}
	return &judgment, nil
}

type abstractTagWithChildren struct {
	Tag      models.TopicTag
	Children []models.TopicTag
	TaskID   uint
}

type batchLabelDescResult struct {
	Results []struct {
		AbstractTagID uint   `json:"abstract_tag_id"`
		Label         string `json:"label"`
		Description   string `json:"description"`
	} `json:"results"`
}

func batchRegenerateLabelsAndDescriptions(ctx context.Context, entries []abstractTagWithChildren) (*batchLabelDescResult, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	var parts []string
	for i, e := range entries {
		var childParts []string
		for _, c := range e.Children {
			childParts = append(childParts, fmt.Sprintf("- %q", c.Label))
		}
		parts = append(parts, fmt.Sprintf(`%d. 抽象标签 %q (ID:%d, 类别:%s, 当前描述:%s)
子标签:
%s`,
			i+1, e.Tag.Label, e.Tag.ID, e.Tag.Category,
			TruncateStr(e.Tag.Description, 100),
			strings.Join(childParts, "\n")))
	}

	prompt := fmt.Sprintf(`为以下多个抽象标签重新生成 label 和 description。

抽象标签列表:
%s

规则:
- label: 概括所有子标签（1-160字）。保持当前 label 如果仍然准确
- description: 中文，1-2 句话，客观说明，500 字以内
- person 类标签说明人物身份
- event 类标签说明事件经过
- keyword 类标签说明概念领域

返回 JSON: {"results": [{"abstract_tag_id": ID, "label": "标签", "description": "描述"}, ...]}`,
		strings.Join(parts, "\n\n"))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"results": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"abstract_tag_id": {Type: "integer"},
							"label":           {Type: "string"},
							"description":     {Type: "string"},
						},
						Required: []string{"abstract_tag_id", "label", "description"},
					},
				},
			},
			Required: []string{"results"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":  "abstract_label_desc_batch",
			"batch_size": len(entries),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch label/desc LLM: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment batchLabelDescResult
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse batch label/desc response: %w", err)
	}
	return &judgment, nil
}
