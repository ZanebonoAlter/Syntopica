package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxConcurrentHierarchyMatches = 3

type AbstractTagUpdateQueueService struct {
	db        *gorm.DB
	embedding *EmbeddingService
	logger    *zap.Logger

	mu     sync.Mutex
	closed bool
	stopCh chan struct{}

	hierarchySem chan struct{}
}

func NewAbstractTagUpdateQueueService(logger *zap.Logger) *AbstractTagUpdateQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AbstractTagUpdateQueueService{
		db:           database.DB,
		embedding:    NewEmbeddingService(),
		logger:       logger,
		stopCh:       make(chan struct{}),
		hierarchySem: make(chan struct{}, maxConcurrentHierarchyMatches),
	}
}

func (s *AbstractTagUpdateQueueService) Enqueue(abstractTagID uint, triggerReason string) error {
	if abstractTagID == 0 {
		return nil
	}

	var activeCount int64
	err := s.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("abstract_tag_id = ? AND status IN ?", abstractTagID, []string{
			models.AbstractTagUpdateQueueStatusPending,
			models.AbstractTagUpdateQueueStatusProcessing,
		}).Count(&activeCount).Error
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return nil
	}

	var recentCompleted int64
	err = s.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("abstract_tag_id = ? AND status = ? AND completed_at > ?",
			abstractTagID,
			models.AbstractTagUpdateQueueStatusCompleted,
			time.Now().Add(-5*time.Minute),
		).Count(&recentCompleted).Error
	if err != nil {
		return err
	}
	if recentCompleted > 0 {
		return nil
	}

	task := models.AbstractTagUpdateQueue{
		AbstractTagID: abstractTagID,
		TriggerReason: triggerReason,
		Status:        models.AbstractTagUpdateQueueStatusPending,
	}
	return s.db.Create(&task).Error
}

func (s *AbstractTagUpdateQueueService) Start() {
	s.mu.Lock()
	if s.closed {
		s.closed = false
		s.stopCh = make(chan struct{})
	}
	s.mu.Unlock()

	result := s.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("status = ?", models.AbstractTagUpdateQueueStatusProcessing).
		Updates(map[string]interface{}{
			"status":     models.AbstractTagUpdateQueueStatusPending,
			"started_at": nil,
		})
	if result.Error != nil {
		s.logger.Error("failed to reset stale processing abstract tag update tasks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		s.logger.Info("reset stale processing abstract tag update tasks", zap.Int64("count", result.RowsAffected))
	}

	go s.worker()
	s.logger.Info("abstract tag update queue worker started")
}

func (s *AbstractTagUpdateQueueService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stopCh)
	s.logger.Info("abstract tag update queue worker stopped")
}

func (s *AbstractTagUpdateQueueService) worker() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("abstract tag update queue worker panic recovered", zap.Any("panic", r))
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if !closed {
				go s.worker()
			}
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processNext()
		}
	}
}

func (s *AbstractTagUpdateQueueService) processNext() {
	var tasks []models.AbstractTagUpdateQueue

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ?", models.AbstractTagUpdateQueueStatusPending).
			Order("created_at ASC").
			Limit(1).
			Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		now := time.Now()
		result := tx.Model(&models.AbstractTagUpdateQueue{}).
			Where("id = ? AND status = ?", tasks[0].ID, models.AbstractTagUpdateQueueStatusPending).
			Updates(map[string]interface{}{
				"status":     models.AbstractTagUpdateQueueStatusProcessing,
				"started_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			tasks[0].ID = 0
		}
		return nil
	})
	if err != nil {
		s.logger.Error("failed to claim abstract tag update task", zap.Error(err))
		return
	}

	if len(tasks) == 0 || tasks[0].ID == 0 {
		return
	}

	task := tasks[0]

	if err := s.refreshAbstractTag(task.AbstractTagID, task.TriggerReason); err != nil {
		s.markFailed(task.ID, err.Error())
		return
	}

	now := time.Now()
	if err := s.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("id = ?", task.ID).
		Updates(map[string]interface{}{
			"status":       models.AbstractTagUpdateQueueStatusCompleted,
			"completed_at": now,
		}).Error; err != nil {
		s.logger.Error("failed to mark abstract tag update task completed", zap.Uint("task_id", task.ID), zap.Error(err))
	}

	s.logger.Info("abstract tag update completed",
		zap.Uint("abstract_tag_id", task.AbstractTagID),
		zap.String("trigger", task.TriggerReason))
}

func (s *AbstractTagUpdateQueueService) refreshAbstractTag(abstractTagID uint, triggerReason string) error {
	var tag models.TopicTag
	if err := s.db.First(&tag, abstractTagID).Error; err != nil {
		return fmt.Errorf("load abstract tag %d: %w", abstractTagID, err)
	}

	children, err := s.loadChildren(abstractTagID)
	if err != nil {
		return fmt.Errorf("load children of abstract tag %d: %w", abstractTagID, err)
	}
	if len(children) == 0 {
		return nil
	}

	newLabel, newDesc, err := regenerateAbstractLabelAndDescription(context.Background(), &tag, children)
	if err != nil {
		return fmt.Errorf("regenerate label/description for abstract tag %d: %w", abstractTagID, err)
	}

	updates := map[string]interface{}{}

	if newDesc != "" && newDesc != tag.Description {
		updates["description"] = newDesc
		tag.Description = newDesc
	}

	if newLabel != "" && newLabel != tag.Label {
		newSlug := Slugify(newLabel)
		if newSlug != "" && newSlug != tag.Slug {
			if isAbstractRoot(s.db, abstractTagID) {
				logging.Infof("Skipping label update for root abstract tag %d (%q): root labels are protected", abstractTagID, tag.Label)
			} else {
				var conflictCount int64
				s.db.Model(&models.TopicTag{}).
					Where("slug = ? AND id != ? AND status = ?", newSlug, abstractTagID, "active").
					Count(&conflictCount)
				if conflictCount == 0 {
					updates["label"] = newLabel
					updates["slug"] = newSlug
					tag.Label = newLabel
					tag.Slug = newSlug
				} else {
					logging.Warnf("Skipping label update for abstract tag %d: slug %q already in use", abstractTagID, newSlug)
				}
			}
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(&models.TopicTag{}).Where("id = ?", abstractTagID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update abstract tag %d: %w", abstractTagID, err)
		}
		s.logger.Info("abstract tag updated",
			zap.Uint("abstract_tag_id", abstractTagID),
			zap.Any("fields", func() []string {
				var f []string
				for k := range updates {
					f = append(f, k)
				}
				return f
			}()))
	}

	emb, err := s.embedding.GenerateEmbedding(context.Background(), &tag, EmbeddingTypeIdentity)
	if err != nil {
		return fmt.Errorf("generate embedding for abstract tag %d: %w", abstractTagID, err)
	}
	emb.TopicTagID = abstractTagID
	if err := s.embedding.SaveEmbedding(emb); err != nil {
		return fmt.Errorf("save embedding for abstract tag %d: %w", abstractTagID, err)
	}

	var semOpts []EmbeddingTextOptions
	if tag.Category == "event" {
		titles := GetTagContextTitles(tag.ID, 5)
		if len(titles) > 0 {
			semOpts = append(semOpts, EmbeddingTextOptions{ContextTitles: titles})
		}
	}
	semanticEmb, semErr := s.embedding.GenerateEmbedding(context.Background(), &tag, EmbeddingTypeSemantic, semOpts...)
	if semErr != nil {
		return fmt.Errorf("generate semantic embedding for abstract tag %d: %w", abstractTagID, semErr)
	}
	semanticEmb.TopicTagID = abstractTagID
	if err := s.embedding.SaveEmbedding(semanticEmb); err != nil {
		return fmt.Errorf("save semantic embedding for abstract tag %d: %w", abstractTagID, err)
	}

	if triggerReason != "multi_parent_resolved" {
		s.hierarchySem <- struct{}{}
		go func() {
			defer func() { <-s.hierarchySem }()
			MatchAbstractTagHierarchy(context.Background(), abstractTagID)
		}()
	}

	return nil
}

func isAbstractRoot(db *gorm.DB, tagID uint) bool {
	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("child_id = ? AND relation_type = ?", tagID, "abstract").
		Count(&count)
	return count == 0
}

func (s *AbstractTagUpdateQueueService) loadChildren(abstractTagID uint) ([]models.TopicTag, error) {
	var children []models.TopicTag
	err := s.db.Joins("JOIN topic_tag_relations ON topic_tag_relations.child_id = topic_tags.id").
		Where("topic_tag_relations.parent_id = ? AND topic_tag_relations.relation_type = ?", abstractTagID, "abstract").
		Find(&children).Error
	return children, err
}

func (s *AbstractTagUpdateQueueService) markFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := s.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.AbstractTagUpdateQueueStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"retry_count":   gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
		s.logger.Error("failed to mark abstract tag update task failed", zap.Uint("task_id", taskID), zap.Error(err))
	}
	s.logger.Warn("abstract tag update task failed", zap.Uint("task_id", taskID), zap.String("error", errMsg))
}

func regenerateAbstractLabelAndDescription(ctx context.Context, abstractTag *models.TopicTag, children []models.TopicTag) (string, string, error) {
	var childParts []string
	for _, c := range children {
		entry := fmt.Sprintf("- %q", c.Label)
		if contextInfo := formatTagPromptContext(&c); contextInfo != "" {
			entry += fmt.Sprintf(" (%s)", TruncateStr(contextInfo, 160))
		}
		childParts = append(childParts, entry)
	}

	var prompt string
	switch abstractTag.Category {
	case "person":
		prompt = fmt.Sprintf(`你是一个标签分类助手。给定一个人物类型的抽象标签及其子标签，重新生成抽象标签的 label 和 description。

抽象标签: %q
当前描述: %s

子标签:
%s

要求:
- label: 概括所有子标签指向的人物身份（1-160字）。保持当前 label 如果仍然准确。
- description: 中文，1-2 句话，客观说明这是谁、什么身份。不要延伸到观点、立场、事件。不要评价。500 字以内。
- 重点：这个抽象标签代表的是"人"，不是"人的观点"。label 围绕人物身份，description 说明身份背景。
- 如果子标签都是同一个人的不同称谓，label 应该是最标准的称谓，description 说明其身份。

示例:
- 子标签 "贝森特", "美国财长贝森特" → label: "贝森特", description: "美国财政部长斯科特·贝森特（Scott Bessent），曾任对冲基金经理"
- 子标签 "马斯克", "Elon Musk", "特斯拉CEO" → label: "马斯克", description: "特斯拉和 SpaceX 首席执行官，X（原 Twitter）所有者"

返回 JSON: {"label": "your answer", "description": "your answer"}`,
			abstractTag.Label,
			abstractTag.Description,
			strings.Join(childParts, "\n"))

	case "event":
		prompt = fmt.Sprintf(`你是一个标签分类助手。给定一个事件类型的抽象标签及其子标签，重新生成抽象标签的 label 和 description。

抽象标签: %q
当前描述: %s

子标签:
%s

要求:
- label: 概括所有子标签涉及的事件主线（1-160字）。保持当前 label 如果仍然准确。
- description: 中文，1-2 句话，客观说明事件是什么、涉及哪些方面。不要延伸到影响分析、价值判断。500 字以内。
- 重点：这个抽象标签代表的是"事件/事态"，聚焦于事实经过和涉及方。
- 如果子标签之间存在明显不相关的事件（例如一个是外交谈判，另一个是科技产品发布），请在 description 中如实标注各个子标签涉及的不同事件领域，而不是试图强行将它们概括为一个统一主题。例如："该标签涵盖多个不相关的独立事件：XXX和YYY。"但这种情况更理想的做法是通过 label 变更来缩小范围。

返回 JSON: {"label": "your answer", "description": "your answer"}`,
			abstractTag.Label,
			abstractTag.Description,
			strings.Join(childParts, "\n"))

	default:
		prompt = fmt.Sprintf(`Given this abstract topic tag and its child tags, regenerate the abstract tag's label and description.

Abstract tag: %q
Current description: %s

Child tags:
%s

Label and description requirements:
- label: A concise name (1-160 chars) that encompasses ALL child tags. Keep the current label if it still accurately represents the child tags. Only change it if the child tag scope has clearly shifted. Must be in the original language of the tags.
- description: Must be in Chinese (中文). Objective, factual summary that encompasses ALL child tags. 1-2 sentences, under 500 characters. Must explain the concept, not just restate the name. Should be broader than any single child tag's description.

Respond with JSON: {"label": "your answer", "description": "your answer"}`,
			abstractTag.Label,
			abstractTag.Description,
			strings.Join(childParts, "\n"))
	}

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
				"label":       {Type: "string", Description: "抽象标签名称"},
				"description": {Type: "string", Description: "抽象标签的中文客观描述"},
			},
			Required: []string{"label", "description"},
		},
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":       "abstract_tag_label_description_refresh",
			"abstract_tag_id": abstractTag.ID,
			"child_count":     len(children),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return "", "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	label := strings.TrimSpace(parsed.Label)
	if len(label) > maxAbstractNameLen {
		label = label[:maxAbstractNameLen]
	}

	desc := strings.TrimSpace(parsed.Description)
	if len([]rune(desc)) > 500 {
		desc = string([]rune(desc)[:500])
	}

	return label, desc, nil
}

func EnqueueAbstractTagUpdate(abstractTagID uint, triggerReason string) {
	if abstractTagID == 0 {
		return
	}
	if database.DB == nil {
		return
	}
	svc := getAbstractTagUpdateQueueService()
	if err := svc.Enqueue(abstractTagID, triggerReason); err != nil {
		logging.Warnf("Failed to enqueue abstract tag update for tag %d: %v", abstractTagID, err)
	}
}
