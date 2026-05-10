# Narrative Summary 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在现有标签体系之上增加"叙事总结"定时任务，每天从活跃标签中提炼跨 category 的叙事主线，以 git tree 模型追踪叙事演进。

**Architecture:** 独立的 `narrative` 域包 + 独立的 `narrative_summaries` 数据表。定时任务收集当天活跃的根级抽象标签和未分类标签，连同上一周期的叙事，交给 AI 归纳叙事主线。叙事以 git tree 结构存储（`parent_ids` 多对多），不修改任何标签数据。前端在 Topic Graph 页面增加叙事视角。

**Tech Stack:** Go (Gin, GORM) / Vue 3 + TypeScript / AI Router (topic_tagging capability)

---

## 业务流程模拟

### 输入（某一天触发时）

```
日期: 2026-04-15

活跃根级抽象标签:
  - "中东地缘冲突" (event, 12 articles, description: "以色列与伊朗及其代理人的军事对抗升级")
  - "AI 大模型竞赛" (keyword, 8 articles, description: "各科技公司发布新一代大语言模型")
  - "全球供应链重构" (keyword, 5 articles, description: "各国推进供应链多元化和近岸外包")
  
活跃未分类标签:
  - "红海航运危机" (event, 6 articles, description: "胡塞武装袭击红海商船")
  - "GPT-5" (keyword, 4 articles)
  - "Elon Musk" (person, 3 articles)
  - "台积电" (keyword, 3 articles)

上一周期叙事 (2026-04-14):
  1. {title: "中东冲突外溢冲击全球航运", status: continuing, generation: 3}
  2. {title: "AI 行业进入多模态竞赛新阶段", status: emerging, generation: 1}
```

### AI 处理逻辑

```
Prompt 要求:
1. 从以上标签中归纳叙事主线
2. 每条叙事必须跨至少两个 category
3. 标题是带判断的短句，不是名词
4. 和上一期叙事做延续/分裂/合并/新建判定
```

### 输出

```json
{
  "narratives": [
    {
      "title": "中东冲突升级叠加油价波动，加剧全球供应链风险",
      "summary": "以色列对伊朗的军事打击引发中东局势全面升级，胡塞武装扩大红海航运袭击范围，推动各国重新评估能源和贸易路线安全。",
      "status": "continuing",
      "parent_ids": [1],
      "related_tag_ids": [101, 205, 302],
      "related_article_ids": [1001, 1002, 1003, 1005, 1008]
    },
    {
      "title": "AI 军备竞赛从模型能力转向基础设施争夺",
      "summary": "GPT-5 发布引发新一轮模型竞赛，但行业焦点正从模型性能转向算力供应和芯片自主可控，台积电产能成为关键博弈点。",
      "status": "merging",
      "parent_ids": [2],
      "related_tag_ids": [102, 303, 304],
      "related_article_ids": [1010, 1011, 1012, 1020]
    }
  ]
}
```

### Git Tree 演进示意

```
Day 1:                    Day 2:                     Day 3:
A──B (continuing)         A──B──D (continuing)       A──B──D──F (continuing)
     └──C (splitting)          └──C──E (continuing)       └──C──E (ending, no successor)
F (emerging)              F──G (continuing)           F──G──H (continuing)
                                                     I (emerging)
```

---

## Task 1: 数据模型与迁移

**Files:**
- Create: `backend-go/internal/domain/models/narrative.go`
- Modify: `backend-go/internal/platform/database/migrator.go` (添加 AutoMigrate)

**Step 1: 创建 NarrativeSummary 模型**

```go
// internal/domain/models/narrative.go
package models

import "time"

type NarrativeSummary struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:300;not null" json:"title"`
	Summary   string    `gorm:"type:text;not null" json:"summary"`
	Status    string    `gorm:"size:20;not null;index" json:"status"`
	Period    string    `gorm:"size:20;not null;default:daily" json:"period"`
	PeriodDate time.Time `gorm:"index:idx_narrative_period_date;not null" json:"period_date"`
	Generation int       `gorm:"not null;default:0" json:"generation"`
	ParentIDs  string    `gorm:"type:text" json:"parent_ids"`
	RelatedTagIDs string `gorm:"type:text" json:"related_tag_ids"`
	RelatedArticleIDs string `gorm:"type:text" json:"related_article_ids"`
	Source     string    `gorm:"size:20;default:ai" json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (NarrativeSummary) TableName() string {
	return "narrative_summaries"
}

const (
	NarrativeStatusEmerging   = "emerging"
	NarrativeStatusContinuing = "continuing"
	NarrativeStatusSplitting  = "splitting"
	NarrativeStatusMerging    = "merging"
	NarrativeStatusEnding     = "ending"
)
```

**Step 2: 添加到 AutoMigrate**

在 `migrator.go` 的 `autoMigrateModels` 中添加 `&models.NarrativeSummary{}`。

**Step 3: 验证迁移**

Run: `cd backend-go && go build ./...`
Expected: 编译通过

**Step 4: Commit**

```bash
git add backend-go/internal/domain/models/narrative.go backend-go/internal/platform/database/migrator.go
git commit -m "feat: add NarrativeSummary model for daily narrative summaries"
```

---

## Task 2: 叙事域包 — 输入采集

**Files:**
- Create: `backend-go/internal/domain/narrative/collector.go`

**Step 1: 实现 NarrativeCollector**

负责收集 AI 的输入素材：当天活跃的根级抽象标签 + 未分类标签。

```go
// internal/domain/narrative/collector.go
package narrative

import (
	"fmt"
	"time"

	"myrobot/internal/domain/models"
	"myrobot/internal/platform/database"
)

type TagInput struct {
	ID          uint   `json:"id"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Description string `json:"description"`
	ArticleCount int   `json:"article_count"`
	IsAbstract   bool   `json:"is_abstract"`
	Source       string `json:"source"`
}

type PreviousNarrative struct {
	ID         uint64 `json:"id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Generation int    `json:"generation"`
}

func CollectTagInputs(date time.Time) ([]TagInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var inputs []TagInput

	rootAbstractTags, err := collectRootAbstractTags(startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("collect root abstract tags: %w", err)
	}
	inputs = append(inputs, rootAbstractTags...)

	unclassifiedTags, err := collectUnclassifiedTags(startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("collect unclassified tags: %w", err)
	}
	inputs = append(inputs, unclassifiedTags...)

	return inputs, nil
}

func collectRootAbstractTags(since, until time.Time) ([]TagInput, error) {
	// 1. 找所有 abstract relation 的 parent_id
	var parentIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Distinct("parent_id").
		Pluck("parent_id", &parentIDs)

	if len(parentIDs) == 0 {
		return nil, nil
	}

	// 2. 找这些 parent 中同时也是 child 的（即中间节点，非根）
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ? AND parent_id IN ?", "abstract", parentIDs).
		Distinct("child_id").
		Pluck("child_id", &childIDs)

	childSet := make(map[uint]bool, len(childIDs))
	for _, id := range childIDs {
		childSet[id] = true
	}

	// 3. 过滤出根：在 parentIDs 中但不在 childSet 中的
	var rootIDs []uint
	for _, id := range parentIDs {
		if !childSet[id] {
			rootIDs = append(rootIDs, id)
		}
	}

	if len(rootIDs) == 0 {
		return nil, nil
	}

	// 4. 查这些标签，附带当天文章数
	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ?", rootIDs, "active").Find(&tags)

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", rootIDs, since, until).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, tag := range tags {
		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			IsAbstract:   true,
			Source:        "abstract",
		})
	}
	return inputs, nil
}

func collectUnclassifiedTags(since, until time.Time) ([]TagInput, error) {
	// 找所有在 abstract relation 中的 tag
	var allRelated []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("parent_id", &allRelated)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("child_id", &childIDs)
	allRelated = append(allRelated, childIDs...)
	relatedSet := make(map[uint]bool, len(allRelated))
	for _, id := range allRelated {
		relatedSet[id] = true
	}

	// 找当天有文章的活跃非 abstract 标签
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND source != ?", "active", "abstract")

	if len(relatedSet) > 0 {
		excl := make([]uint, 0, len(relatedSet))
		for id := range relatedSet {
			excl = append(excl, id)
		}
		query = query.Where("id NOT IN ?", excl)
	}

	// 限定当天有文章
	activeSubquery := database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.pub_date >= ? AND articles.pub_date < ?", since, until)
	query = query.Where("id IN (?)", activeSubquery)

	var tags []models.TopicTag
	if err := query.Order("quality_score DESC, feed_count DESC").Limit(100).Find(&tags).Error; err != nil {
		return nil, err
	}

	var inputs []TagInput
	for _, tag := range tags {
		inputs = append(inputs, TagInput{
			ID:          tag.ID,
			Label:       tag.Label,
			Category:    tag.Category,
			Description: tag.Description,
			ArticleCount: 0,
			IsAbstract:  false,
			Source:       tag.Source,
		})
	}
	return inputs, nil
}

func CollectPreviousNarratives(date time.Time) ([]PreviousNarrative, error) {
	yesterday := date.AddDate(0, 0, -1)
	var narratives []models.NarrativeSummary
	if err := database.DB.
		Where("period = ? AND period_date >= ? AND period_date < ?", "daily", yesterday, date).
		Order("id ASC").
		Find(&narratives).Error; err != nil {
		return nil, err
	}

	var result []PreviousNarrative
	for _, n := range narratives {
		result = append(result, PreviousNarrative{
			ID:         uint64(n.ID),
			Title:      n.Title,
			Summary:    n.Summary,
			Status:     n.Status,
			Generation: n.Generation,
		})
	}
	return result, nil
}
```

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/collector.go
git commit -m "feat: add narrative collector for tag inputs and previous narratives"
```

---

## Task 3: 叙事域包 — AI Prompt 与调用

**Files:**
- Create: `backend-go/internal/domain/narrative/generator.go`

**Step 1: 实现 AI 叙事生成器**

```go
// internal/domain/narrative/generator.go
package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myrobot/internal/platform/airouter"
	"myrobot/internal/platform/logging"
)

type NarrativeOutput struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	ParentIDs   []uint64 `json:"parent_ids"`
	RelatedTagIDs []uint `json:"related_tag_ids"`
}

type GenerateResult struct {
	Narratives []NarrativeOutput `json:"narratives"`
}

func GenerateNarratives(ctx context.Context, tagInputs []TagInput, prevNarratives []PreviousNarrative) ([]NarrativeOutput, error) {
	if len(tagInputs) == 0 {
		return nil, nil
	}

	prompt := buildNarrativePrompt(tagInputs, prevNarratives)

	maxTokens := 4000
	temperature := 0.3
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是资深新闻编辑。只输出合法 JSON，不要额外解释。"},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		Metadata:    map[string]any{"task": "narrative_summary"},
	})
	if err != nil {
		return nil, fmt.Errorf("narrative ai call failed: %w", err)
	}

	return parseNarrativeResponse(result.Content)
}

func buildNarrativePrompt(tags []TagInput, prev []PreviousNarrative) string {
	var sb strings.Builder

	sb.WriteString("## 今日活跃主题标签\n\n")
	for i, t := range tags {
		abstract := ""
		if t.IsAbstract {
			abstract = " [抽象标签]"
		}
		desc := ""
		if t.Description != "" {
			desc = fmt.Sprintf("\n   简介: %s", t.Description)
		}
		articleInfo := ""
		if t.ArticleCount > 0 {
			articleInfo = fmt.Sprintf(", %d 篇文章", t.ArticleCount)
		}
		sb.WriteString(fmt.Sprintf("%d. \"%s\" (类别: %s%s%s)%s\n", i+1, t.Label, t.Category, abstract, articleInfo, desc))
	}

	if len(prev) > 0 {
		sb.WriteString("\n## 上一周期叙事\n\n")
		for i, n := range prev {
			sb.WriteString(fmt.Sprintf("%d. [ID:%d] \"%s\" (状态: %s, 第 %d 代)\n   %s\n", i+1, n.ID, n.Title, n.Status, n.Generation, n.Summary))
		}
	}

	sb.WriteString("\n## 任务\n\n")
	sb.WriteString("请从以上标签中归纳今日的叙事主线。\n\n")
	sb.WriteString("要求:\n")
	sb.WriteString("1. 每条叙事必须横跨至少两个类别（event/person/keyword）\n")
	sb.WriteString("2. 标题必须是带判断的短句（如\"中东冲突外溢冲击全球航运\"），不能是纯名词\n")
	sb.WriteString("3. 按因果、影响、主题关联分组，不要按语义相似度归类\n")
	sb.WriteString("4. 数量不固定，有几条写几条，没有就返回空数组\n")
	sb.WriteString("5. 对比上一周期叙事，判断每条叙事的演进状态:\n")
	sb.WriteString("   - emerging: 新出现的叙事，parent_ids 为空\n")
	sb.WriteString("   - continuing: 延续上一周期某条叙事，parent_ids 填前驱 ID\n")
	sb.WriteString("   - splitting: 从上一周期某条叙事分裂出来，parent_ids 填前驱 ID\n")
	sb.WriteString("   - merging: 多条前驱叙事合并为一条，parent_ids 填多个前驱 ID\n")
	sb.WriteString("6. related_tag_ids 填这条叙事引用的标签 ID\n")
	sb.WriteString("7. 不要为了凑数而强行合并不相关的标签\n")

	if len(prev) == 0 {
		sb.WriteString("\n注意: 这是首次生成，没有上一周期叙事，所有叙事的 status 都应为 emerging。\n")
	}

	sb.WriteString("\n输出 JSON:\n```json\n")
	sb.WriteString(`{"narratives": [{"title": "...", "summary": "2-3 叒宏观解读", "status": "emerging|continuing|splitting|merging", "parent_ids": [], "related_tag_ids": []}]}`)
	sb.WriteString("\n```")

	return sb.String()
}

func parseNarrativeResponse(content string) ([]NarrativeOutput, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("no json object found in response")
	}
	raw := content[start : end+1]

	var result GenerateResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse narrative json: %w", err)
	}

	var valid []NarrativeOutput
	for _, n := range result.Narratives {
		if strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Summary) == "" {
			logging.Warnf("skipping narrative with empty title or summary")
			continue
		}
		if n.Status == "" {
			n.Status = "emerging"
		}
		valid = append(valid, n)
	}
	return valid, nil
}
```

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/generator.go
git commit -m "feat: add narrative AI generator with prompt and response parsing"
```

---

## Task 4: 叙事域包 — 存储与查询

**Files:**
- Create: `backend-go/internal/domain/narrative/service.go`

**Step 1: 实现 NarrativeService**

```go
// internal/domain/narrative/service.go
package narrative

import (
	"encoding/json"
	"fmt"
	"time"

	"myrobot/internal/domain/models"
	"myrobot/internal/platform/database"
	"myrobot/internal/platform/logging"
)

type NarrativeService struct{}

func NewNarrativeService() *NarrativeService {
	return &NarrativeService{}
}

func (s *NarrativeService) GenerateAndSave(date time.Time) (int, error) {
	tagInputs, err := CollectTagInputs(date)
	if err != nil {
		return 0, fmt.Errorf("collect tag inputs: %w", err)
	}
	logging.Infof("narrative: collected %d tag inputs for %s", len(tagInputs), date.Format("2006-01-02"))

	if len(tagInputs) == 0 {
		logging.Infoln("narrative: no active tags, skipping")
		return 0, nil
	}

	prevNarratives, err := CollectPreviousNarratives(date)
	if err != nil {
		return 0, fmt.Errorf("collect previous narratives: %w", err)
	}
	logging.Infof("narrative: found %d previous narratives", len(prevNarratives))

	outputs, err := GenerateNarratives(nil, tagInputs, prevNarratives)
	if err != nil {
		return 0, fmt.Errorf("generate narratives: %w", err)
	}
	logging.Infof("narrative: generated %d narratives", len(outputs))

	if len(outputs) == 0 {
		return 0, nil
	}

	saved, err := s.saveNarratives(outputs, date)
	if err != nil {
		return 0, fmt.Errorf("save narratives: %w", err)
	}

	s.markEndedNarratives(date, outputs, prevNarratives)

	return saved, nil
}

func (s *NarrativeService) saveNarratives(outputs []NarrativeOutput, date time.Time) (int, error) {
	saved := 0
	for _, out := range outputs {
		parentIDsJSON, _ := json.Marshal(out.ParentIDs)
		tagIDsJSON, _ := json.Marshal(out.RelatedTagIDs)

		articleIDs := s.resolveArticleIDs(out.RelatedTagIDs, date)
		articleIDsJSON, _ := json.Marshal(articleIDs)

		narrative := models.NarrativeSummary{
			Title:            out.Title,
			Summary:          out.Summary,
			Status:           out.Status,
			Period:           "daily",
			PeriodDate:       time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC),
			Generation:       s.resolveGeneration(out, date),
			ParentIDs:        string(parentIDsJSON),
			RelatedTagIDs:    string(tagIDsJSON),
			RelatedArticleIDs: string(articleIDsJSON),
			Source:           "ai",
		}

		if err := database.DB.Create(&narrative).Error; err != nil {
			logging.Warnf("narrative: failed to save '%s': %v", out.Title, err)
			continue
		}
		saved++
	}
	return saved, nil
}

func (s *NarrativeService) resolveGeneration(out NarrativeOutput, date time.Time) int {
	if len(out.ParentIDs) == 0 {
		return 0
	}
	var parent models.NarrativeSummary
	if err := database.DB.Where("id IN ?", out.ParentIDs).Order("generation DESC").First(&parent).Error; err != nil {
		return 0
	}
	return parent.Generation + 1
}

func (s *NarrativeService) resolveArticleIDs(tagIDs []uint, date time.Time) []uint64 {
	if len(tagIDs) == 0 {
		return nil
	}
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var articleIDs []uint64
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.article_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagIDs, startOfDay, endOfDay).
		Pluck("article_id", &articleIDs)
	return articleIDs
}

func (s *NarrativeService) markEndedNarratives(date time.Time, currentOutputs []NarrativeOutput, prev []PreviousNarrative) {
	if len(prev) == 0 {
		return
	}

	referencedParents := make(map[uint64]bool)
	for _, out := range currentOutputs {
		for _, pid := range out.ParentIDs {
			referencedParents[pid] = true
		}
	}

	for _, p := range prev {
		if !referencedParents[p.ID] {
			database.DB.Model(&models.NarrativeSummary{}).
				Where("id = ?", p.ID).
				Updates(map[string]interface{}{
					"status":     models.NarrativeStatusEnding,
					"updated_at": time.Now(),
				})
			logging.Infof("narrative: marked %d ('%s') as ending", p.ID, p.Title)
		}
	}
}

type NarrativeListItem struct {
	ID         uint64   `json:"id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Status     string   `json:"status"`
	Period     string   `json:"period"`
	PeriodDate string   `json:"period_date"`
	Generation int      `json:"generation"`
	ParentIDs  []uint64 `json:"parent_ids"`
	RelatedTags []TagBrief `json:"related_tags"`
	ChildIDs   []uint64 `json:"child_ids"`
}

type TagBrief struct {
	ID       uint   `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

func (s *NarrativeService) GetByDate(date time.Time) ([]NarrativeListItem, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var narratives []models.NarrativeSummary
	if err := database.DB.
		Where("period = ? AND period_date >= ? AND period_date < ?", "daily", startOfDay, endOfDay).
		Order("id ASC").
		Find(&narratives).Error; err != nil {
		return nil, err
	}

	return s.toListItems(narratives), nil
}

func (s *NarrativeService) GetNarrativeTree(narrativeID uint64) (*NarrativeListItem, error) {
	var n models.NarrativeSummary
	if err := database.DB.First(&n, narrativeID).Error; err != nil {
		return nil, err
	}

	items := s.toListItems([]models.NarrativeSummary{n})
	return &items[0], nil
}

func (s *NarrativeService) GetNarrativeHistory(narrativeID uint64) ([]NarrativeListItem, error) {
	var history []models.NarrativeSummary
	visited := make(map[uint64]bool)
	s.walkHistory(narrativeID, &history, visited)
	return s.toListItems(history), nil
}

func (s *NarrativeService) walkHistory(id uint64, history *[]models.NarrativeSummary, visited map[uint64]bool) {
	if visited[id] {
		return
	}
	visited[id] = true

	var n models.NarrativeSummary
	if err := database.DB.First(&n, id).Error; err != nil {
		return
	}
	*history = append(*history, n)

	var parentIDs []uint64
	json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
	for _, pid := range parentIDs {
		s.walkHistory(pid, history, visited)
	}
}

func (s *NarrativeService) toListItems(narratives []models.NarrativeSummary) []NarrativeListItem {
	tagIDSet := make(map[uint]bool)
	for _, n := range narratives {
		var tagIDs []uint
		json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
		for _, id := range tagIDs {
			tagIDSet[id] = true
		}
	}

	tagMap := make(map[uint]TagBrief)
	if len(tagIDSet) > 0 {
		tagIDs := make([]uint, 0, len(tagIDSet))
		for id := range tagIDSet {
			tagIDs = append(tagIDs, id)
		}
		var tags []models.TopicTag
		database.DB.Where("id IN ?", tagIDs).Find(&tags)
		for _, t := range tags {
			tagMap[t.ID] = TagBrief{ID: t.ID, Label: t.Label, Category: t.Category}
		}
	}

	// Build child map: for each narrative, find who references it as parent
	idSet := make(map[uint64]bool, len(narratives))
	for _, n := range narratives {
		idSet[n.ID] = true
	}
	childMap := make(map[uint64][]uint64)
	for _, n := range narratives {
		var parentIDs []uint64
		json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		for _, pid := range parentIDs {
			if idSet[pid] {
				childMap[pid] = append(childMap[pid], n.ID)
			}
		}
	}

	items := make([]NarrativeListItem, 0, len(narratives))
	for _, n := range narratives {
		var parentIDs []uint64
		json.Unmarshal([]byte(n.ParentIDs), &parentIDs)

		var tagIDs []uint
		json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
		relatedTags := make([]TagBrief, 0, len(tagIDs))
		for _, id := range tagIDs {
			if t, ok := tagMap[id]; ok {
				relatedTags = append(relatedTags, t)
			}
		}

		children := childMap[n.ID]
		if children == nil {
			children = []uint64{}
		}

		items = append(items, NarrativeListItem{
			ID:          n.ID,
			Title:       n.Title,
			Summary:     n.Summary,
			Status:      n.Status,
			Period:      n.Period,
			PeriodDate:  n.PeriodDate.Format("2006-01-02"),
			Generation:  n.Generation,
			ParentIDs:   parentIDs,
			RelatedTags: relatedTags,
			ChildIDs:    children,
		})
	}
	return items
}
```

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/service.go
git commit -m "feat: add narrative service with generate, save, query, and tree history"
```

---

## Task 5: 定时任务

**Files:**
- Create: `backend-go/internal/jobs/narrative_summary.go`
- Modify: `backend-go/internal/app/runtime.go`
- Modify: `backend-go/internal/app/runtimeinfo/schedulers.go`
- Modify: `backend-go/internal/jobs/handler.go`

**Step 1: 创建定时任务**

遵循 `tag_quality_score.go` 的模式，实现 `NarrativeSummaryScheduler`。

核心字段:
- `cron *cron.Cron`
- `checkInterval int` (秒，默认 86400 = 每天一次)
- `sync.Mutex` 防并发
- `isRunning bool`

核心方法:
- `NewNarrativeSummaryScheduler(checkInterval int)`
- `Start() error` — initSchedulerTask, add cron func, cron.Start
- `Stop()`
- `TriggerNow() map[string]interface{}` — 接受可选 `date` 参数
- `runNarrativeCycle(triggerSource string, targetDate time.Time)`
- `GetStatus() SchedulerStatusResponse`

`TriggerNow()` 需要支持通过 metadata 传入指定日期。Handler 层解析 query param `date`。

**Step 2: 注册到 Runtime**

在 `runtime.go` 中:
- `Runtime` struct 加 `NarrativeSummary *jobs.NarrativeSummaryScheduler`
- `StartRuntime()` 中创建并启动（interval 86400）
- `SetupGracefulShutdown()` 中 stop
- `runtimeinfo.NarrativeSummarySchedulerInterface` 赋值

**Step 3: 注册到 runtimeinfo**

在 `schedulers.go` 中添加:
```go
var NarrativeSummarySchedulerInterface interface{}
```

**Step 4: 注册到 handler.go**

在 `schedulerDescriptors()` 中添加:
```go
{
    Name:        "narrative_summary",
    DisplayName: "Narrative Summary",
    Description: "Generate daily narrative summaries from active topic tags",
    Get: func() interface{} {
        return runtimeinfo.NarrativeSummarySchedulerInterface
    },
},
```

**Step 5: 编译验证**

Run: `cd backend-go && go build ./...`

**Step 6: Commit**

```bash
git add backend-go/internal/jobs/narrative_summary.go backend-go/internal/app/runtime.go backend-go/internal/app/runtimeinfo/schedulers.go backend-go/internal/jobs/handler.go
git commit -m "feat: add narrative summary scheduler with daily cron and manual trigger"
```

---

## Task 6: 后端 API

**Files:**
- Create: `backend-go/internal/domain/narrative/handler.go`
- Modify: `backend-go/internal/app/router.go`

**Step 1: 实现 handler**

```go
// internal/domain/narrative/handler.go
package narrative

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var service = NewNarrativeService()

func RegisterNarrativeRoutes(rg *gin.RouterGroup) {
	group := rg.Group("/narratives")
	{
		group.GET("", getNarratives)
		group.GET("/:id", getNarrative)
		group.GET("/:id/history", getNarrativeHistory)
	}
}

// GET /api/narratives?date=2026-04-15
func getNarratives(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	narratives, err := service.GetByDate(date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": narratives})
}

// GET /api/narratives/:id
func getNarrative(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	narrative, err := service.GetNarrativeTree(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "narrative not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": narrative})
}

// GET /api/narratives/:id/history
func getNarrativeHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	history, err := service.GetNarrativeHistory(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}
```

**Step 2: 注册路由**

在 `router.go` 的 `SetupRoutes` 中添加:
```go
narrativedomain.RegisterNarrativeRoutes(api)
```

**Step 3: 编译验证**

Run: `cd backend-go && go build ./...`

**Step 4: Commit**

```bash
git add backend-go/internal/domain/narrative/handler.go backend-go/internal/app/router.go
git commit -m "feat: add narrative API endpoints (list, detail, history)"
```

---

## Task 7: 单元测试 — Collector

**Files:**
- Create: `backend-go/internal/domain/narrative/collector_test.go`

**Step 1: 写测试**

测试 `collectRootAbstractTags` 和 `collectUnclassifiedTags` 的 SQL 逻辑。需要数据库连接（集成测试风格，参考现有测试）。

测试用例:
1. 没有活跃标签时返回空
2. 有根级抽象标签但当天无文章时返回空
3. 有标签且有当天文章时返回正确数据
4. 未分类标签排除已在 abstract relation 中的

**Step 2: 运行测试**

Run: `cd backend-go && go test ./internal/domain/narrative/... -v`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/collector_test.go
git commit -m "test: add collector tests for narrative tag input gathering"
```

---

## Task 8: 单元测试 — Generator

**Files:**
- Create: `backend-go/internal/domain/narrative/generator_test.go`

**Step 1: 写测试**

测试 `parseNarrativeResponse`：
1. 正常 JSON 解析
2. 缺少 title 的跳过
3. 空 narratives 数组
4. 无效 JSON 返回错误
5. 带 markdown fence 的 JSON 正确提取

测试 `buildNarrativePrompt`：
1. 无上一期叙事时包含 "首次生成" 提示
2. 有上一期叙事时正确列出

**Step 2: 运行测试**

Run: `cd backend-go && go test ./internal/domain/narrative/... -v`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/generator_test.go
git commit -m "test: add generator tests for prompt building and response parsing"
```

---

## Task 9: 前端 API 层

**Files:**
- Modify: `front/app/api/topicGraph.ts` (添加叙事相关 API)

**Step 1: 添加叙事 API 类型和方法**

```typescript
// 在 topicGraph.ts 中添加

export interface NarrativeItem {
  id: number
  title: string
  summary: string
  status: 'emerging' | 'continuing' | 'splitting' | 'merging' | 'ending'
  period: string
  period_date: string
  generation: number
  parent_ids: number[]
  related_tags: { id: number; label: string; category: TopicCategory }[]
  child_ids: number[]
}

export function useNarrativeApi() {
  const getNarratives = async (date: string) => {
    return apiClient.get<{ success: boolean; data: NarrativeItem[] }>(
      `/narratives?date=${date}`
    )
  }

  const getNarrativeHistory = async (id: number) => {
    return apiClient.get<{ success: boolean; data: NarrativeItem[] }>(
      `/narratives/${id}/history`
    )
  }

  return { getNarratives, getNarrativeHistory }
}
```

**Step 2: 类型检查**

Run: `cd front && pnpm exec nuxi typecheck`

**Step 3: Commit**

```bash
git add front/app/api/topicGraph.ts
git commit -m "feat: add narrative API types and client methods"
```

---

## Task 10: 前端叙事面板组件

**Files:**
- Create: `front/app/features/topic-graph/components/NarrativePanel.vue`

**Step 1: 实现叙事面板**

面板展示当前日期的叙事列表，每条叙事显示:
- 标题（带判断的短句）
- 摘要
- 状态标签（emerging/continuing/splitting/merging/ending）
- 关联标签列表
- 点击展开叙事历史（git tree 追溯）

接收 props: `date: string`

交互:
- 初始加载当日叙事列表
- 点击叙事标题展开详情
- 点击"历史"按钮加载 git tree 历史链
- 点击关联标签可联动到图谱选中

**Step 2: 集成到 TopicGraphPage**

在 `TopicGraphPage.vue` 中增加叙事面板区块，放在图谱下方或侧栏中。增加一个 tab 切换（热点标签 / 叙事）。

**Step 3: 类型检查 + 构建**

Run: `cd front && pnpm exec nuxi typecheck && pnpm build`

**Step 4: Commit**

```bash
git add front/app/features/topic-graph/components/NarrativePanel.vue front/app/features/topic-graph/components/TopicGraphPage.vue
git commit -m "feat: add narrative panel component integrated into topic graph page"
```

---

## Task 11: 端到端验证

**Step 1: 启动后端**

Run: `cd backend-go && go run cmd/server/main.go`

**Step 2: 手动触发叙事生成**

通过 scheduler API 手动触发:
```
POST /api/schedulers/narrative_summary/trigger?date=2026-04-15
```

**Step 3: 验证 API 输出**

```
GET /api/narratives?date=2026-04-15
```

检查返回的叙事:
- 标题是带判断的短句
- 每条叙事跨 category
- parent_ids 正确引用上一期
- ending 状态正确标记

**Step 4: 前端验证**

打开 `/topics` 页面，切换到叙事 tab，检查:
- 叙事列表正常展示
- 点击叙事展开详情
- 关联标签可点击
- 历史链路可追溯

**Step 5: 最终 commit**

```bash
git add -A
git commit -m "chore: end-to-end verification of narrative summary feature"
```

---

## 文件变更汇总

| 操作 | 文件 |
|------|------|
| Create | `backend-go/internal/domain/models/narrative.go` |
| Create | `backend-go/internal/domain/narrative/collector.go` |
| Create | `backend-go/internal/domain/narrative/generator.go` |
| Create | `backend-go/internal/domain/narrative/service.go` |
| Create | `backend-go/internal/domain/narrative/handler.go` |
| Create | `backend-go/internal/jobs/narrative_summary.go` |
| Create | `backend-go/internal/domain/narrative/collector_test.go` |
| Create | `backend-go/internal/domain/narrative/generator_test.go` |
| Create | `front/app/features/topic-graph/components/NarrativePanel.vue` |
| Modify | `backend-go/internal/platform/database/migrator.go` |
| Modify | `backend-go/internal/app/runtime.go` |
| Modify | `backend-go/internal/app/runtimeinfo/schedulers.go` |
| Modify | `backend-go/internal/jobs/handler.go` |
| Modify | `backend-go/internal/app/router.go` |
| Modify | `front/app/api/topicGraph.ts` |
| Modify | `front/app/features/topic-graph/components/TopicGraphPage.vue` |

## 叙事状态流转图

```
                    emerging
                       │
                       ▼
                 continuing ◄──────┐
                  │       │        │
            splitting  merging     │
                  │       │        │
                  └──┬────┘        │
                     ▼             │
              (next generation)    │
                     │             │
                     ▼             │
                 continuing ───────┘
                  (or)
                     │
                     ▼
                  ending (leaf, no successor)
```
