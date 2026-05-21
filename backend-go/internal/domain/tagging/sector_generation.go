package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/concept"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

type SectorProposal struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SectorDiff struct {
	Keep             []SectorKeepItem  `json:"keep"`
	Add              []SectorProposal  `json:"add"`
	Merge            []SectorMergeItem `json:"merge"`
	Split            []SectorSplitItem `json:"split"`
	AffectedTagCount int               `json:"affected_tag_count"`
}

type SectorKeepItem struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type SectorMergeItem struct {
	SourceIDs []uint `json:"source_ids"`
	TargetID  uint   `json:"target_id"`
	Name      string `json:"name"`
}

type SectorSplitItem struct {
	SourceID uint             `json:"source_id"`
	NewItems []SectorProposal `json:"new_items"`
}

type SectorDiffExecutionResult struct {
	Results          []SectorDiffExecutionItemResult `json:"results"`
	SuccessCount     int                             `json:"success_count"`
	FailedCount      int                             `json:"failed_count"`
	AffectedTagCount int                             `json:"affected_tag_count"`
	MovedTagCount    int                             `json:"moved_tag_count"`
	CreatedIDs       []uint                          `json:"created_ids"`
}

type SectorDiffExecutionItemResult struct {
	Operation        string `json:"operation"`
	Status           string `json:"status"`
	Name             string `json:"name,omitempty"`
	SourceID         uint   `json:"source_id,omitempty"`
	SourceIDs        []uint `json:"source_ids,omitempty"`
	TargetID         uint   `json:"target_id,omitempty"`
	AffectedTagCount int    `json:"affected_tag_count"`
	MovedTagCount    int    `json:"moved_tag_count"`
	CreatedIDs       []uint `json:"created_ids,omitempty"`
	Error            string `json:"error,omitempty"`
}

func (r *SectorDiffExecutionResult) addItem(item SectorDiffExecutionItemResult) {
	r.Results = append(r.Results, item)
	if item.Status == "success" {
		r.SuccessCount++
	} else {
		r.FailedCount++
	}
	r.AffectedTagCount += item.AffectedTagCount
	r.MovedTagCount += item.MovedTagCount
	r.CreatedIDs = append(r.CreatedIDs, item.CreatedIDs...)
}

const sectorAutoMaxTags = 50

func AutoGenerateSectors(ctx context.Context, db *gorm.DB, category string, threshold int) error {
	var count int64
	if err := db.Model(&models.TopicTag{}).
		Where("category = ? AND status = ? AND concept_id IS NULL", category, "active").
		Count(&count).Error; err != nil {
		return fmt.Errorf("auto generate sectors: count unplaced: %w", err)
	}

	if int(count) < threshold {
		logging.Infof("sector-auto: category=%q has %d unplaced tags, need %d", category, count, threshold)
		logging.Infof("sector-execute: category=%q completed", category)
		return nil
	}

	var tags []models.TopicTag
	if err := db.Where("category = ? AND status = ? AND concept_id IS NULL", category, "active").
		Order("id DESC").
		Limit(sectorAutoMaxTags).
		Find(&tags).Error; err != nil {
		return fmt.Errorf("auto generate sectors: load tags: %w", err)
	}

	var labels []string
	for _, t := range tags {
		labels = append(labels, t.Label)
	}

	prompt := buildAutoGeneratePrompt(labels)

	temperature := 0.4
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: sectorAutoSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"sectors": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string", Description: "板块名称，2-6字"},
							"description": {Type: "string", Description: "板块描述，30-80字"},
						},
						Required: []string{"name", "description"},
					},
				},
			},
			Required: []string{"sectors"},
		},
		Metadata: map[string]any{
			"operation": "sector_auto_generate",
			"category":  category,
		},
	})
	if err != nil {
		return fmt.Errorf("auto generate sectors: LLM call: %w", err)
	}

	cleaned := jsonutil.SanitizeLLMJSON(result.Content)
	var raw struct {
		Sectors []SectorProposal `json:"sectors"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return fmt.Errorf("auto generate sectors: parse LLM response: %w", err)
	}

	existingConcepts, err := concept.ListActiveConcepts(category)
	if err != nil {
		return fmt.Errorf("auto generate sectors: load existing: %w", err)
	}

	type conceptVec struct {
		id   uint
		vec  []float64
		name string
	}
	var existingVecs []conceptVec
	for _, c := range existingConcepts {
		if c.Embedding == nil || *c.Embedding == "" {
			continue
		}
		v, err := parseConceptEmbedding(*c.Embedding)
		if err != nil {
			continue
		}
		existingVecs = append(existingVecs, conceptVec{id: c.ID, vec: v, name: c.Name})
	}

	router := airouter.NewRouter()
	var survivors []SectorProposal
	for _, prop := range raw.Sectors {
		prop.Name = strings.TrimSpace(prop.Name)
		prop.Description = strings.TrimSpace(prop.Description)
		if prop.Name == "" {
			continue
		}

		propText := prop.Name
		if prop.Description != "" {
			propText = prop.Name + "\n" + prop.Description
		}
		embResult, embErr := router.Embed(ctx, airouter.EmbeddingRequest{
			Input:    []string{propText},
			Metadata: map[string]any{"operation": "sector_dedup", "proposal": prop.Name},
		}, airouter.CapabilityEmbedding)
		if embErr != nil {
			logging.Warnf("sector-auto: embed proposal %q failed: %v, keeping", prop.Name, embErr)
			survivors = append(survivors, prop)
			continue
		}
		if len(embResult.Embeddings) == 0 || len(embResult.Embeddings[0]) == 0 {
			survivors = append(survivors, prop)
			continue
		}
		propVec := embResult.Embeddings[0]

		duplicate := false
		for _, ev := range existingVecs {
			if sim, _ := airouter.CosineSimilarity(ev.vec, propVec); sim >= 0.85 {
				logging.Infof("sector-auto: skipping duplicate proposal %q (similar to %q, sim=%.3f)", prop.Name, ev.name, sim)
				duplicate = true
				break
			}
		}
		if !duplicate {
			survivors = append(survivors, prop)
		}
	}

	for _, prop := range survivors {
		c, err := concept.CreateConcept(prop.Name, prop.Description, category)
		if err != nil {
			logging.Warnf("sector-auto: create concept %q failed: %v", prop.Name, err)
			continue
		}

		if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).Update("source", "auto").Error; err != nil {
			logging.Warnf("sector-auto: set source for concept %d: %v", c.ID, err)
		}

		if err := concept.GenerateConceptEmbedding(ctx, c); err != nil {
			logging.Warnf("sector-auto: generate embedding for concept %d: %v", c.ID, err)
		}

		logging.Infof("sector-auto: created concept %d (%s) for category=%q", c.ID, prop.Name, category)
	}

	return nil
}

func suggestInitialSectors(ctx context.Context, db *gorm.DB, category string) (*SectorDiff, error) {
	var tags []models.TopicTag
	if err := db.Where("category = ? AND status = ? AND concept_id IS NULL", category, "active").
		Order("id DESC").
		Limit(sectorAutoMaxTags).
		Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("suggest initial sectors: load tags: %w", err)
	}

	if len(tags) == 0 {
		logging.Infof("sector-suggest: category=%q has no unassigned tags, returning empty diff", category)
		return &SectorDiff{
			Keep:  []SectorKeepItem{},
			Add:   []SectorProposal{},
			Merge: []SectorMergeItem{},
			Split: []SectorSplitItem{},
		}, nil
	}

	var labels []string
	for _, t := range tags {
		label := t.Label
		if t.Description != "" {
			label += " — " + t.Description
		}
		labels = append(labels, label)
	}

	prompt := buildInitialSectorPrompt(labels)

	temperature := 0.4
	maxTokens := 3000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: sectorAutoSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"sectors": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string", Description: "板块名称，2-6字"},
							"description": {Type: "string", Description: "板块描述，30-80字"},
						},
						Required: []string{"name", "description"},
					},
				},
			},
			Required: []string{"sectors"},
		},
		Metadata: map[string]any{
			"operation": "sector_initial_suggest",
			"category":  category,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("suggest initial sectors: LLM call: %w", err)
	}

	cleaned := jsonutil.SanitizeLLMJSON(result.Content)
	var raw struct {
		Sectors []SectorProposal `json:"sectors"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return nil, fmt.Errorf("suggest initial sectors: parse response: %w", err)
	}

	diff := &SectorDiff{
		Keep:  []SectorKeepItem{},
		Add:   []SectorProposal{},
		Merge: []SectorMergeItem{},
		Split: []SectorSplitItem{},
	}

	for _, prop := range raw.Sectors {
		prop.Name = strings.TrimSpace(prop.Name)
		prop.Description = strings.TrimSpace(prop.Description)
		if prop.Name == "" {
			continue
		}
		diff.Add = append(diff.Add, prop)
		diff.AffectedTagCount += len(tags) / len(raw.Sectors)
	}

	logging.Infof("sector-suggest: category=%q generated %d initial sector proposals from %d unassigned tags",
		category, len(diff.Add), len(tags))

	return diff, nil
}

func LLMSuggestSectors(ctx context.Context, db *gorm.DB, category string) (*SectorDiff, error) {
	type sectorRow struct {
		ID        uint   `gorm:"column:id"`
		Name      string `gorm:"column:name"`
		Protected bool   `gorm:"column:protected"`
		Source    string `gorm:"column:source"`
		TagCount  int    `gorm:"column:tag_count"`
	}
	var sectors []sectorRow
	query := `
		SELECT c.id, c.name, c.protected, c.source,
		       COALESCE(t.tag_count, 0) AS tag_count
		FROM board_concepts c
		LEFT JOIN (
			SELECT concept_id, COUNT(*) AS tag_count
			FROM topic_tags
			WHERE category = ? AND status = 'active' AND concept_id IS NOT NULL
			GROUP BY concept_id
		) t ON t.concept_id = c.id
		WHERE c.category = ? AND c.status = 'active'
		ORDER BY c.display_order ASC, c.id ASC
	`
	if err := db.Raw(query, category, category).Scan(&sectors).Error; err != nil {
		return nil, fmt.Errorf("llm suggest sectors: load sectors: %w", err)
	}

	if len(sectors) == 0 {
		return suggestInitialSectors(ctx, db, category)
	}

	var sb strings.Builder
	sb.WriteString("以下是当前板块及其标签数量：\n\n")
	for _, s := range sectors {
		sb.WriteString(fmt.Sprintf("- [id=%d] %s (来源:%s, 标签数:%d", s.ID, s.Name, s.Source, s.TagCount))
		if s.Protected {
			sb.WriteString(", 受保护")
		}
		sb.WriteString(")\n")
	}
	sb.WriteString("\n请分析并返回JSON，包含以下字段：\n")
	sb.WriteString("- keep: 保留的板块数组，每项含 id, name\n")
	sb.WriteString("- add: 建议新增的板块数组，每项含 name, description\n")
	sb.WriteString("- merge: 建议合并的数组，每项含 source_ids(要合并的板块ID数组), target_id(目标板块ID), name\n")
	sb.WriteString("- split: 建议拆分的数组，每项含 source_id(要拆分的板块ID), new_items(新板块数组，每项含name,description)\n")
	sb.WriteString("- affected_tag_count: 受影响的标签总数\n")
	sb.WriteString("\n注意：受保护的板块不能被删除或合并，必须保留在keep中。\n")

	temperature := 0.4
	maxTokens := 3000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: sectorSuggestSystemPrompt},
			{Role: "user", Content: sb.String()},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"keep": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"id":   {Type: "integer"},
							"name": {Type: "string"},
						},
						Required: []string{"id", "name"},
					},
				},
				"add": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":        {Type: "string"},
							"description": {Type: "string"},
						},
						Required: []string{"name", "description"},
					},
				},
				"merge": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"source_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"target_id":  {Type: "integer"},
							"name":       {Type: "string"},
						},
						Required: []string{"source_ids", "target_id", "name"},
					},
				},
				"split": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"source_id": {Type: "integer"},
							"new_items": {
								Type: "array",
								Items: &airouter.SchemaProperty{
									Type: "object",
									Properties: map[string]airouter.SchemaProperty{
										"name":        {Type: "string"},
										"description": {Type: "string"},
									},
									Required: []string{"name", "description"},
								},
							},
						},
						Required: []string{"source_id", "new_items"},
					},
				},
				"affected_tag_count": {Type: "integer"},
			},
			Required: []string{"keep", "add", "merge", "split", "affected_tag_count"},
		},
		Metadata: map[string]any{
			"operation": "sector_llm_suggest",
			"category":  category,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("llm suggest sectors: LLM call: %w", err)
	}

	cleaned := jsonutil.SanitizeLLMJSON(result.Content)
	var diff SectorDiff
	if err := json.Unmarshal([]byte(cleaned), &diff); err != nil {
		return nil, fmt.Errorf("llm suggest sectors: parse response: %w", err)
	}

	protectedIDs := make(map[uint]bool)
	protectedNames := make(map[uint]string)
	for _, s := range sectors {
		if s.Protected {
			protectedIDs[s.ID] = true
			protectedNames[s.ID] = s.Name
		}
	}

	var filteredKeep []SectorKeepItem
	for _, k := range diff.Keep {
		filteredKeep = append(filteredKeep, k)
	}
	for id, name := range protectedNames {
		found := false
		for _, k := range filteredKeep {
			if k.ID == id {
				found = true
				break
			}
		}
		if !found {
			filteredKeep = append(filteredKeep, SectorKeepItem{ID: id, Name: name})
		}
	}
	diff.Keep = filteredKeep

	var filteredMerge []SectorMergeItem
	for _, m := range diff.Merge {
		violates := false
		for _, sid := range m.SourceIDs {
			if protectedIDs[sid] {
				violates = true
				break
			}
		}
		if protectedIDs[m.TargetID] {
			violates = true
		}
		if !violates {
			filteredMerge = append(filteredMerge, m)
		}
	}
	diff.Merge = filteredMerge

	var filteredSplit []SectorSplitItem
	for _, sp := range diff.Split {
		if !protectedIDs[sp.SourceID] {
			filteredSplit = append(filteredSplit, sp)
		}
	}
	diff.Split = filteredSplit

	return &diff, nil
}

func LLMExecuteSectorDiff(ctx context.Context, db *gorm.DB, category string, diff *SectorDiff) (*SectorDiffExecutionResult, error) {
	result := &SectorDiffExecutionResult{
		Results:    []SectorDiffExecutionItemResult{},
		CreatedIDs: []uint{},
	}
	if diff == nil {
		return result, nil
	}

	logging.Infof("sector-execute: category=%q add=%d merge=%d split=%d", category, len(diff.Add), len(diff.Merge), len(diff.Split))
	for _, add := range diff.Add {
		add.Name = strings.TrimSpace(add.Name)
		add.Description = strings.TrimSpace(add.Description)
		item := SectorDiffExecutionItemResult{Operation: "add", Name: add.Name}
		if add.Name == "" {
			item.Status = "failed"
			item.Error = "name is required"
			result.addItem(item)
			continue
		}

		c, err := concept.CreateConcept(add.Name, add.Description, category)
		if err != nil {
			logging.Warnf("sector-diff: add concept %q failed: %v", add.Name, err)
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}
		item.CreatedIDs = []uint{c.ID}

		if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).Update("source", "llm").Error; err != nil {
			logging.Warnf("sector-diff: set source for concept %d: %v", c.ID, err)
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}

		if err := concept.GenerateConceptEmbedding(ctx, c); err != nil {
			logging.Warnf("sector-diff: generate embedding for concept %d: %v", c.ID, err)
		}

		item.Status = "success"
		result.addItem(item)
		logging.Infof("sector-diff: added concept %d (%s)", c.ID, add.Name)
	}

	for _, m := range diff.Merge {
		item := SectorDiffExecutionItemResult{Operation: "merge", Name: m.Name, SourceIDs: m.SourceIDs, TargetID: m.TargetID}
		if m.TargetID == 0 || len(m.SourceIDs) == 0 {
			item.Status = "failed"
			item.Error = "target_id and source_ids are required"
			result.addItem(item)
			continue
		}

		target, err := concept.GetConceptByID(m.TargetID)
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}

		validSources := make([]uint, 0, len(m.SourceIDs))
		for _, srcID := range m.SourceIDs {
			if srcID == m.TargetID {
				continue
			}
			if _, err := concept.GetConceptByID(srcID); err != nil {
				item.Status = "failed"
				item.Error = err.Error()
				result.addItem(item)
				validSources = nil
				break
			}
			validSources = append(validSources, srcID)
		}
		if item.Status == "failed" {
			continue
		}
		if len(validSources) == 0 {
			item.Status = "failed"
			item.Error = "no merge source remains after filtering target_id"
			result.addItem(item)
			continue
		}

		for _, srcID := range validSources {
			update := db.Model(&models.TopicTag{}).
				Where("concept_id = ? AND category = ?", srcID, category).
				Update("concept_id", m.TargetID)
			if update.Error != nil {
				logging.Warnf("sector-diff: move tags from %d to %d: %v", srcID, m.TargetID, update.Error)
				item.Status = "failed"
				item.Error = update.Error.Error()
				result.addItem(item)
				break
			}
			item.MovedTagCount += int(update.RowsAffected)
			item.AffectedTagCount += int(update.RowsAffected)

			if err := concept.DeactivateConcept(srcID); err != nil {
				logging.Warnf("sector-diff: deactivate source concept %d: %v", srcID, err)
				item.Status = "failed"
				item.Error = err.Error()
				result.addItem(item)
				break
			}

			logging.Infof("sector-diff: merged concept %d into %d", srcID, m.TargetID)
		}
		if item.Status == "failed" {
			continue
		}
		if err := concept.GenerateConceptEmbedding(ctx, target); err != nil {
			logging.Warnf("sector-diff: regenerate embedding for target %d: %v", m.TargetID, err)
		}
		item.Status = "success"
		result.addItem(item)
	}

	for _, sp := range diff.Split {
		item := SectorDiffExecutionItemResult{Operation: "split", SourceID: sp.SourceID}
		source, err := concept.GetConceptByID(sp.SourceID)
		if err != nil {
			logging.Warnf("sector-diff: split: load source concept %d: %v", sp.SourceID, err)
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}

		type splitConcept struct {
			id  uint
			vec []float64
		}
		var newConcepts []splitConcept
		for _, newItem := range sp.NewItems {
			newItem.Name = strings.TrimSpace(newItem.Name)
			newItem.Description = strings.TrimSpace(newItem.Description)
			if newItem.Name == "" {
				continue
			}

			c, err := concept.CreateConcept(newItem.Name, newItem.Description, category)
			if err != nil {
				logging.Warnf("sector-diff: split create concept %q: %v", newItem.Name, err)
				item.Status = "failed"
				item.Error = err.Error()
				continue
			}
			item.CreatedIDs = append(item.CreatedIDs, c.ID)

			if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).Update("source", "llm").Error; err != nil {
				logging.Warnf("sector-diff: set source for split concept %d: %v", c.ID, err)
				item.Status = "failed"
				item.Error = err.Error()
				continue
			}

			if err := concept.GenerateConceptEmbedding(ctx, c); err != nil {
				logging.Warnf("sector-diff: generate embedding for split concept %d: %v", c.ID, err)
			}

			refreshed, loadErr := concept.GetConceptByID(c.ID)
			var vec []float64
			if loadErr == nil && refreshed.Embedding != nil && *refreshed.Embedding != "" {
				vec, _ = parseConceptEmbedding(*refreshed.Embedding)
			}
			if len(vec) > 0 {
				newConcepts = append(newConcepts, splitConcept{id: c.ID, vec: vec})
			}

			logging.Infof("sector-diff: split created concept %d (%s) from %d", c.ID, newItem.Name, sp.SourceID)
		}
		if item.Status == "failed" {
			result.addItem(item)
			continue
		}

		if len(newConcepts) == 0 {
			item.Status = "failed"
			item.Error = "no split sectors with embeddings were created"
			result.addItem(item)
			continue
		}

		splitRouter := airouter.NewRouter()
		var tags []models.TopicTag
		if err := db.Where("concept_id = ? AND category = ?", sp.SourceID, category).Find(&tags).Error; err != nil {
			logging.Warnf("sector-diff: split: load tags for concept %d: %v", sp.SourceID, err)
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}

		for _, tag := range tags {
			tagText := tag.Label
			if tag.Description != "" {
				tagText += " " + tag.Description
			}

			bestID := uint(0)
			bestSim := -1.0

			tagEmb, tagEmbErr := splitRouter.Embed(ctx, airouter.EmbeddingRequest{
				Input:    []string{tagText},
				Metadata: map[string]any{"operation": "sector_split_reassign", "tag_id": tag.ID},
			}, airouter.CapabilityEmbedding)
			if tagEmbErr == nil && len(tagEmb.Embeddings) > 0 && len(tagEmb.Embeddings[0]) > 0 {
				tagVec := tagEmb.Embeddings[0]
				for _, nc := range newConcepts {
					sim, _ := airouter.CosineSimilarity(nc.vec, tagVec)
					if sim > bestSim {
						bestSim = sim
						bestID = nc.id
					}
				}
			} else if len(newConcepts) > 0 {
				bestID = newConcepts[0].id
			}

			if bestID != 0 {
				update := db.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Update("concept_id", bestID)
				if update.Error != nil {
					logging.Warnf("sector-diff: split: reassign tag %d to %d: %v", tag.ID, bestID, update.Error)
					item.Status = "failed"
					item.Error = update.Error.Error()
					break
				}
				item.MovedTagCount += int(update.RowsAffected)
				item.AffectedTagCount += int(update.RowsAffected)
			}
		}
		if item.Status == "failed" {
			result.addItem(item)
			continue
		}

		var remainingCount int64
		if err := db.Model(&models.TopicTag{}).Where("concept_id = ? AND category = ?", sp.SourceID, category).Count(&remainingCount).Error; err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			result.addItem(item)
			continue
		}
		if remainingCount == 0 {
			if err := concept.DeactivateConcept(sp.SourceID); err != nil {
				logging.Warnf("sector-diff: split: deactivate source %d: %v", sp.SourceID, err)
				item.Status = "failed"
				item.Error = err.Error()
				result.addItem(item)
				continue
			}
			logging.Infof("sector-diff: split: deactivated empty source concept %d", sp.SourceID)
		} else {
			if err := concept.GenerateConceptEmbedding(ctx, source); err != nil {
				logging.Warnf("sector-diff: split: regenerate source embedding %d: %v", sp.SourceID, err)
			}
		}
		item.Status = "success"
		result.addItem(item)
	}

	return result, nil
}

func ManualCreateSector(ctx context.Context, db *gorm.DB, category, label, description string) (*models.BoardConcept, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("manual create sector: label is required")
	}

	if strings.TrimSpace(description) == "" {
		generated, err := generateSectorDescription(ctx, label)
		if err != nil {
			logging.Warnf("manual-sector: generate description failed: %v, using label", err)
			description = label
		} else {
			description = generated
		}
	}

	c, err := concept.CreateConcept(label, description, category)
	if err != nil {
		return nil, fmt.Errorf("manual create sector: %w", err)
	}

	if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).
		Updates(map[string]interface{}{"source": "manual", "protected": true}).Error; err != nil {
		logging.Warnf("manual-sector: set source/protected for concept %d: %v", c.ID, err)
	}

	if err := concept.GenerateConceptEmbedding(ctx, c); err != nil {
		logging.Warnf("manual-sector: generate embedding for concept %d: %v", c.ID, err)
	}

	logging.Infof("manual-sector: created concept %d (%s) for category=%q", c.ID, label, category)
	return c, nil
}

func CheckSectorHealth(ctx context.Context, db *gorm.DB, category string) (deletedAuto, markedDeclining int, err error) {
	concepts, err := concept.ListActiveConcepts(category)
	if err != nil {
		return 0, 0, fmt.Errorf("check sector health: load concepts: %w", err)
	}

	for _, c := range concepts {
		if c.Source == "manual" {
			continue
		}

		var tagCount int64
		if err := db.Model(&models.TopicTag{}).
			Where("concept_id = ? AND category = ?", c.ID, category).
			Count(&tagCount).Error; err != nil {
			logging.Warnf("sector-health: count tags for concept %d: %v", c.ID, err)
			continue
		}

		currentCount := int(tagCount)
		peak := c.PeakTagCount
		if currentCount > peak {
			peak = currentCount
		}

		if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).
			Update("peak_tag_count", peak).Error; err != nil {
			logging.Warnf("sector-health: update peak for concept %d: %v", c.ID, err)
		}

		if c.Source == "auto" && currentCount == 0 {
			if err := concept.DeactivateConcept(c.ID); err != nil {
				logging.Warnf("sector-health: deactivate auto concept %d: %v", c.ID, err)
			} else {
				deletedAuto++
				logging.Infof("sector-health: deactivated empty auto concept %d (%s)", c.ID, c.Name)
			}
			continue
		}

		if c.Source == "llm" && peak > 0 && currentCount < peak/2 {
			if err := db.Model(&models.BoardConcept{}).Where("id = ?", c.ID).
				Update("declining", true).Error; err != nil {
				logging.Warnf("sector-health: mark declining for concept %d: %v", c.ID, err)
			} else {
				markedDeclining++
				logging.Infof("sector-health: marked concept %d (%s) as declining: %d/%d tags", c.ID, c.Name, currentCount, peak)
			}
		}
	}

	return deletedAuto, markedDeclining, nil
}

func generateSectorDescription(ctx context.Context, label string) (string, error) {
	temperature := 0.4
	maxTokens := 200
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是内容架构师，为给定板块名称生成30-80字的描述。"},
			{Role: "user", Content: fmt.Sprintf("请为板块「%s」生成一段描述。", label)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"description": {Type: "string"},
			},
			Required: []string{"description"},
		},
		Metadata: map[string]any{
			"operation": "sector_manual_description",
		},
	})
	if err != nil {
		return "", err
	}

	cleaned := jsonutil.SanitizeLLMJSON(result.Content)
	var raw struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return "", fmt.Errorf("parse description response: %w", err)
	}

	desc := strings.TrimSpace(raw.Description)
	if desc == "" {
		return "", fmt.Errorf("empty description from LLM")
	}
	return desc, nil
}

func buildInitialSectorPrompt(labels []string) string {
	var sb strings.Builder
	sb.WriteString("以下是一组未分配板块的标签，请分析并建议初始板块结构：\n\n")
	for i, l := range labels {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, l))
	}
	sb.WriteString("\n请返回JSON，包含sectors数组，每项有name（2-6字）和description（30-80字）字段。")
	sb.WriteString("\n建议的板块应能概括一组语义相近的标签，避免过于宽泛或过于具体。")
	sb.WriteString(fmt.Sprintf("\n当前共有%d个标签需要分配，建议3-8个板块。", len(labels)))
	return sb.String()
}

func buildAutoGeneratePrompt(labels []string) string {
	var sb strings.Builder
	sb.WriteString("以下是一组未分配板块的标签，请分析并建议板块：\n\n")
	for _, l := range labels {
		sb.WriteString("- " + l + "\n")
	}
	sb.WriteString("\n请返回JSON，包含sectors数组，每项有name（2-6字）和description（30-80字）字段。")
	sb.WriteString("\n建议的板块应能概括一组语义相近的标签，避免过于宽泛或过于具体。")
	return sb.String()
}

func parseConceptEmbedding(embeddingStr string) ([]float64, error) {
	if embeddingStr == "" {
		return nil, fmt.Errorf("empty embedding string")
	}

	if strings.HasPrefix(embeddingStr, "[") {
		var vec []float64
		if err := json.Unmarshal([]byte(embeddingStr), &vec); err != nil {
			return nil, fmt.Errorf("parse embedding vector: %w", err)
		}
		return vec, nil
	}

	parts := strings.Split(embeddingStr, ",")
	vec := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var f float64
		if _, err := fmt.Sscanf(p, "%f", &f); err == nil {
			vec = append(vec, f)
		}
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("no valid floats in embedding string")
	}
	return vec, nil
}

const sectorAutoSystemPrompt = `你是一名内容架构师，负责为标签组建议板块概念。

## 核心原则
1. 名称 2-6 个字，描述 30-80 字
2. 名称应概括一组语义相近标签的共同主题
3. 描述应说明该板块涵盖的内容范围
4. 建议 3-8 个板块，按重要性排序
5. 每个板块应覆盖一组不同的标签
6. 避免建议名为"其他"或过于宽泛的板块`

const sectorSuggestSystemPrompt = `你是一名内容架构师，负责分析现有板块结构并提出优化建议。

## 核心原则
1. 保留仍有价值的板块
2. 合并语义相近的板块
3. 拆分过于宽泛的板块
4. 为未覆盖的标签组建议新板块
5. 受保护的板块不得被删除、合并或拆分
6. 返回的JSON必须包含keep、add、merge、split、affected_tag_count五个字段`
