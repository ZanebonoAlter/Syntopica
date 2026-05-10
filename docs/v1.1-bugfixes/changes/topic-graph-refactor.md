# Topics Page Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the Topics (Topic Graph) page to remove unused panels, add category-based navigation, transform history into meaningful category insights, and implement graph selection synchronization.

**Architecture:** 
- Backend adds `TopicTagAnalysis` table for storing AI-generated tag insights (events timeline, person summaries, keyword context)
- Frontend removes footer panels, adds category filter tabs, transforms history panel into category-based insights view
- Selection state synchronizes between sidebar, category view, and graph canvas

**Tech Stack:** Go + Gin + GORM (backend), Vue 3 + Nuxt 4 + TypeScript + Pinia (frontend), SQLite (database)

---

## Phase 1: Database Schema

### Task 1.1: Create TopicTagAnalysis Model

**Files:**
- Create: `backend-go/internal/domain/models/topic_tag_analysis.go`

**Step 1: Write the model file**

```go
package models

import "time"

// TopicTagAnalysis stores AI-generated analysis for a topic tag
// Supports incremental updates and category-specific insights
type TopicTagAnalysis struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TopicTagID  uint      `gorm:"uniqueIndex;not null" json:"topic_tag_id"`
	Category    string    `gorm:"size:20;not null;index" json:"category"` // event, person, keyword
	
	// Analysis content (JSON stored)
	Events      string    `gorm:"type:text" json:"events"`       // JSON array of TimelineEvent
	Summary     string    `gorm:"type:text" json:"summary"`      // AI-generated summary for this tag
	Context     string    `gorm:"type:text" json:"context"`     // Additional context/insights
	Entities    string    `gorm:"type:text" json:"entities"`     // JSON array of related entities
	
	// Metadata
	Source      string    `gorm:"size:20;default:ai" json:"source"`      // ai, manual
	Version     int       `gorm:"default:1" json:"version"`
	
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	
	TopicTag     *TopicTag `gorm:"foreignKey:TopicTagID" json:"topic_tag,omitempty"`
}

// TableName specifies the table name
func (TopicTagAnalysis) TableName() string {
	return "topic_tag_analyses"
}

// TimelineEvent represents a milestone/event in a topic's history
type TimelineEvent struct {
	Date        string `json:"date"`         // ISO date: 2024-03-15
	Title       string `json:"title"`        // Event title
	Description string `json:"description"`  // Brief description
	Source      string `json:"source"`       // Where this event came from
}

// RelatedEntity represents a related person/organization/concept
type RelatedEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // person, org, concept
	Relation    string `json:"relation"`    // How they relate to the topic
}
```

**Step 2: Run migration to create table**

Run: `cd backend-go && go run cmd/server/main.go` (GORM auto-migrate will create the table)

**Step 3: Commit**

```bash
git add backend-go/internal/domain/models/topic_tag_analysis.go
git commit -m "feat(db): add TopicTagAnalysis model for tag insights storage"
```

---

### Task 1.2: Add Analysis Field to AISummaryTopic

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go`

**Step 1: Add analysis snippet field to AISummaryTopic**

Find the `AISummaryTopic` struct and add a field for storing per-summary analysis snippets:

```go
// AISummaryTopic represents the many-to-many relationship between summaries and tags
type AISummaryTopic struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SummaryID  uint      `gorm:"index;not null" json:"summary_id"`
	TopicTagID uint      `gorm:"index;not null" json:"topic_tag_id"`
	Score      float64   `gorm:"default:0" json:"score"`
	Source     string    `gorm:"size:20;default:llm" json:"source"`
	
	// Per-summary analysis snippet (why this tag was assigned)
	AnalysisSnippet string `gorm:"type:text" json:"analysis_snippet"` 
	
	CreatedAt  time.Time `json:"created_at"`
	TopicTag   *TopicTag `gorm:"foreignKey:TopicTagID" json:"topic_tag,omitempty"`
}
```

**Step 2: Commit**

```bash
git add backend-go/internal/domain/models/topic_graph.go
git commit -m "feat(db): add analysis_snippet to AISummaryTopic junction"
```

---

## Phase 2: Backend API

### Task 2.1: Add Category Filter to Graph Endpoint

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/types.go`
- Modify: `backend-go/internal/domain/topicgraph/service.go`
- Modify: `backend-go/internal/domain/topicgraph/handler.go`

**Step 1: Add category filter to TopicGraphResponse**

In `types.go`, add category filter support:

```go
// TopicGraphQueryParams holds query parameters for graph endpoint
type TopicGraphQueryParams struct {
	Type     string `form:"type" binding:"required,oneof=daily weekly"`
	Date     string `form:"date"`     // YYYY-MM-DD format
	Category string `form:"category"` // Optional: event, person, keyword
}

// TopicGraphResponse - add category_counts field
type TopicGraphResponse struct {
	Type           string      `json:"type"`
	AnchorDate     string      `json:"anchor_date"`
	PeriodLabel    string      `json:"period_label"`
	Nodes          []GraphNode `json:"nodes"`
	Edges          []GraphEdge  `json:"edges"`
	TopicCount     int         `json:"topic_count"`
	SummaryCount   int         `json:"summary_count"`
	FeedCount      int         `json:"feed_count"`
	TopTopics      []TopicTag  `json:"top_topics"`
	CategoryCounts map[string]int `json:"category_counts,omitempty"` // event: 5, person: 3, keyword: 12
}
```

**Step 2: Modify BuildTopicGraph to support category filter**

In `service.go`, update `BuildTopicGraph`:

```go
func BuildTopicGraph(kind string, anchor time.Time, categoryFilter string) (*TopicGraphResponse, error) {
	windowStart, windowEnd, periodLabel, err := resolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	summaries, err := fetchSummaries(windowStart, windowEnd)
	if err != nil {
		return nil, err
	}

	nodes, edges, topTopics := buildGraphPayload(summaries)
	
	// Apply category filter if specified
	if categoryFilter != "" {
		nodes, edges, topTopics = filterByCategory(nodes, edges, topTopics, categoryFilter)
	}
	
	// Count by category
	categoryCounts := countByCategory(topTopics)

	feedCount := 0
	for _, node := range nodes {
		if node.Kind == "feed" {
			feedCount++
		}
	}

	return &TopicGraphResponse{
		Type:           kind,
		AnchorDate:     windowStart.Format("2006-01-02"),
		PeriodLabel:    periodLabel,
		Nodes:          nodes,
		Edges:          edges,
		TopicCount:     len(topTopics),
		SummaryCount:   len(summaries),
		FeedCount:      feedCount,
		TopTopics:      topTopics,
		CategoryCounts: categoryCounts,
	}, nil
}

func filterByCategory(nodes []GraphNode, edges []GraphEdge, topTopics []TopicTag, category string) ([]GraphNode, []GraphEdge, []TopicTag) {
	// Filter topTopics
	filteredTopics := make([]TopicTag, 0)
	topicSlugs := make(map[string]bool)
	for _, t := range topTopics {
		if t.Category == category {
			filteredTopics = append(filteredTopics, t)
			topicSlugs[t.Slug] = true
		}
	}
	
	// Filter nodes
	filteredNodes := make([]GraphNode, 0)
	nodeIDs := make(map[string]bool)
	for _, n := range nodes {
		if n.Kind == "feed" || (n.Kind == "topic" && topicSlugs[n.Slug]) {
			filteredNodes = append(filteredNodes, n)
			nodeIDs[n.ID] = true
		}
	}
	
	// Filter edges (only keep edges where both endpoints exist)
	filteredEdges := make([]GraphEdge, 0)
	for _, e := range edges {
		if nodeIDs[e.Source] && nodeIDs[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	
	return filteredNodes, filteredEdges, filteredTopics
}

func countByCategory(topics []TopicTag) map[string]int {
	counts := make(map[string]int)
	for _, t := range topics {
		counts[t.Category]++
	}
	return counts
}
```

**Step 3: Update handler to accept category parameter**

In `handler.go`:

```go
func GetTopicGraph(c *gin.Context) {
	kind := c.Param("type")
	category := c.Query("category") // Optional filter
	anchor, err := parseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	graph, err := BuildTopicGraph(kind, anchor, category)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": graph})
}
```

**Step 4: Run tests**

Run: `cd backend-go && go test ./internal/domain/topicgraph -v`

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicgraph/
git commit -m "feat(api): add category filter to topic graph endpoint"
```

---

### Task 2.2: Add Tag Analysis Endpoint

**Files:**
- Create: `backend-go/internal/domain/topicgraph/analysis.go`
- Modify: `backend-go/internal/domain/topicgraph/handler.go`
- Modify: `backend-go/internal/app/router.go`

**Step 1: Create analysis service**

Create `backend-go/internal/domain/topicgraph/analysis.go`:

```go
package topicgraph

import (
	"encoding/json"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

// GetTopicAnalysis retrieves or generates analysis for a topic tag
func GetTopicAnalysis(slug string) (*TopicAnalysisResponse, error) {
	var tag models.TopicTag
	if err := database.DB.Where("slug = ?", slug).First(&tag).Error; err != nil {
		return nil, err
	}

	var analysis models.TopicTagAnalysis
	err := database.DB.Where("topic_tag_id = ?", tag.ID).First(&analysis).Error
	
	if err != nil {
		// No analysis yet, return empty structure
		return &TopicAnalysisResponse{
			TopicTagID: tag.ID,
			Slug:       slug,
			Label:      tag.Label,
			Category:   tag.Category,
			HasAnalysis: false,
		}, nil
	}

	var events []TimelineEvent
	var entities []RelatedEntity
	if analysis.Events != "" {
		_ = json.Unmarshal([]byte(analysis.Events), &events)
	}
	if analysis.Entities != "" {
		_ = json.Unmarshal([]byte(analysis.Entities), &entities)
	}

	return &TopicAnalysisResponse{
		TopicTagID:  tag.ID,
		Slug:        slug,
		Label:       tag.Label,
		Category:    tag.Category,
		HasAnalysis: true,
		Events:      events,
		Summary:     analysis.Summary,
		Context:     analysis.Context,
		Entities:    entities,
		Version:     analysis.Version,
	}, nil
}

// TopicAnalysisResponse represents the analysis response
type TopicAnalysisResponse struct {
	TopicTagID  uint            `json:"topic_tag_id"`
	Slug        string          `json:"slug"`
	Label       string          `json:"label"`
	Category    string          `json:"category"`
	HasAnalysis bool            `json:"has_analysis"`
	Events      []TimelineEvent `json:"events,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Context     string          `json:"context,omitempty"`
	Entities    []RelatedEntity `json:"entities,omitempty"`
	Version     int             `json:"version,omitempty"`
}
```

**Step 2: Add handler**

In `handler.go`, add:

```go
func GetTopicAnalysis(c *gin.Context) {
	slug := c.Param("slug")
	
	analysis, err := GetTopicAnalysis(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true, "data": analysis})
}
```

**Step 3: Register route**

In `router.go`, add to topicGraph group:

```go
topicGraph := api.Group("/topic-graph")
{
	topicGraph.GET("/:type", topicgraphdomain.GetTopicGraph)
	topicGraph.GET("/topic/:slug", topicgraphdomain.GetTopicDetail)
	topicGraph.GET("/topic/:slug/analysis", topicgraphdomain.GetTopicAnalysis) // NEW
}
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicgraph/analysis.go
git add backend-go/internal/domain/topicgraph/handler.go
git add backend-go/internal/app/router.go
git commit -m "feat(api): add topic analysis endpoint"
```

---

### Task 2.3: Enhance TopicDetail with Category History

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/types.go`
- Modify: `backend-go/internal/domain/topicgraph/service.go`

**Step 1: Add CategoryHistory to TopicDetail**

In `types.go`:

```go
// CategoryHistoryPoint represents history grouped by category
type CategoryHistoryPoint struct {
	Category string   `json:"category"`        // event, person, keyword
	Label    string   `json:"label"`           // 事件, 人物, 关键词
	Count    int      `json:"count"`           // Number of occurrences
	Topics   []string `json:"topics"`          // Top topic labels in this category
}

// TopicDetail - add category_history field
type TopicDetail struct {
	Topic           TopicTag               `json:"topic"`
	Summaries       []TopicSummaryCard     `json:"summaries"`
	History         []TopicHistoryPoint    `json:"history"`
	CategoryHistory []CategoryHistoryPoint `json:"category_history"` // NEW
	RelatedTopics   []TopicTag             `json:"related_topics"`
	SearchLinks     map[string]string      `json:"search_links"`
	AppLinks        map[string]string      `json:"app_links"`
}
```

**Step 2: Build category history in BuildTopicDetail**

In `service.go`, add to `BuildTopicDetail`:

```go
// Build category history
categoryHistory := buildCategoryHistory(summaries, slug)

return &TopicDetail{
	Topic:           canonical,
	Summaries:       matchingSummaries,
	History:         history,
	CategoryHistory: categoryHistory,
	RelatedTopics:   related,
	SearchLinks:     searchLinks,
	AppLinks:        appLinks,
}, nil

func buildCategoryHistory(summaries []models.AISummary, currentSlug string) []CategoryHistoryPoint {
	categoryMap := make(map[string]*CategoryHistoryPointBuilder)
	categoryLabels := map[string]string{
		"event":   "事件",
		"person":  "人物",
		"keyword": "关键词",
	}
	
	for _, summary := range summaries {
		topics := summaryTopics(summary)
		for _, topic := range topics {
			if topic.Slug == currentSlug {
				continue
			}
			if _, exists := categoryMap[topic.Category]; !exists {
				categoryMap[topic.Category] = &CategoryHistoryPointBuilder{
					Category: topic.Category,
					Label:    categoryLabels[topic.Category],
					Topics:   make(map[string]int),
				}
			}
			categoryMap[topic.Category].Count++
			categoryMap[topic.Category].Topics[topic.Label]++
		}
	}
	
	// Convert to slice and get top topics per category
	result := make([]CategoryHistoryPoint, 0)
	for _, builder := range categoryMap {
		point := CategoryHistoryPoint{
			Category: builder.Category,
			Label:    builder.Label,
			Count:    builder.Count,
			Topics:   getTopTopics(builder.Topics, 3),
		}
		result = append(result, point)
	}
	
	// Sort by count descending
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	
	return result
}

type CategoryHistoryPointBuilder struct {
	Category string
	Label    string
	Count    int
	Topics   map[string]int
}

func getTopTopics(topicCounts map[string]int, limit int) []string {
	type topicCount struct {
		label string
		count int
	}
	
	sorted := make([]topicCount, 0, len(topicCounts))
	for label, count := range topicCounts {
		sorted = append(sorted, topicCount{label, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	
	result := make([]string, 0, limit)
	for i := 0; i < len(sorted) && i < limit; i++ {
		result = append(result, sorted[i].label)
	}
	return result
}
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicgraph/
git commit -m "feat(api): add category_history to topic detail response"
```

---

## Phase 3: Frontend Refactor

### Task 3.1: Update Frontend Types

**Files:**
- Modify: `front/app/api/topicGraph.ts`

**Step 1: Add new types**

```typescript
import { apiClient } from './client'

export type TopicGraphType = 'daily' | 'weekly'
export type TopicCategory = 'event' | 'person' | 'keyword'

export interface TopicTag {
  label: string
  slug: string
  category: TopicCategory
  icon?: string
  aliases?: string[]
  score: number
}

export interface GraphNode {
  id: string
  label: string
  slug?: string
  kind: 'topic' | 'feed'
  category?: TopicCategory
  icon?: string
  weight: number
  summary_count?: number
  color?: string
  feed_name?: string
  category_name?: string
}

export interface TopicGraphEdge {
  id: string
  source: string
  target: string
  kind: 'topic_topic' | 'topic_feed'
  weight: number
}

export interface TopicGraphPayload {
  type: TopicGraphType
  anchor_date: string
  period_label: string
  nodes: GraphNode[]
  edges: TopicGraphEdge[]
  topic_count: number
  summary_count: number
  feed_count: number
  top_topics: TopicTag[]
  category_counts?: Record<TopicCategory, number>
}

export interface TopicGraphSummaryCard {
  id: number
  title: string
  summary: string
  feed_name: string
  feed_color: string
  category_name: string
  article_count: number
  created_at: string
  topics: TopicTag[]
  articles: Array<{
    id: number
    title: string
    link: string
  }>
}

export interface TopicHistoryPoint {
  anchor_date: string
  count: number
  label: string
}

export interface CategoryHistoryPoint {
  category: TopicCategory
  label: string  // 事件, 人物, 关键词
  count: number
  topics: string[]
}

export interface TopicAnalysisResponse {
  topic_tag_id: number
  slug: string
  label: string
  category: TopicCategory
  has_analysis: boolean
  events?: TimelineEvent[]
  summary?: string
  context?: string
  entities?: RelatedEntity[]
  version?: number
}

export interface TimelineEvent {
  date: string
  title: string
  description: string
  source: string
}

export interface RelatedEntity {
  name: string
  type: string
  relation: string
}

export interface TopicGraphDetailPayload {
  topic: TopicTag
  summaries: TopicGraphSummaryCard[]
  history: TopicHistoryPoint[]
  category_history: CategoryHistoryPoint[]
  related_topics: TopicTag[]
  search_links: Record<string, string>
  app_links: Record<string, string>
}

function withQuery(endpoint: string, params: Record<string, string | undefined>) {
  const query = apiClient.buildQueryParams(params)
  return query ? `${endpoint}?${query}` : endpoint
}

export function useTopicGraphApi() {
  return {
    async getGraph(type: TopicGraphType, date?: string, category?: TopicCategory) {
      return apiClient.get<TopicGraphPayload>(withQuery(`/topic-graph/${type}`, { date, category }))
    },

    async getTopicDetail(slug: string, type: TopicGraphType, date?: string) {
      return apiClient.get<TopicGraphDetailPayload>(withQuery(`/topic-graph/topic/${slug}`, { type, date }))
    },

    async getTopicAnalysis(slug: string) {
      return apiClient.get<TopicAnalysisResponse>(`/topic-graph/topic/${slug}/analysis`)
    },
  }
}
```

**Step 2: Commit**

```bash
git add front/app/api/topicGraph.ts
git commit -m "feat(frontend): add category filter and analysis types to topicGraph API"
```

---

### Task 3.2: Create Category Filter Component

**Files:**
- Create: `front/app/features/topic-graph/components/TopicCategoryFilter.vue`

**Step 1: Create the component**

```vue
<script setup lang="ts">
import type { TopicCategory } from '~/api/topicGraph'

interface Props {
  selectedCategory: TopicCategory | null
  categoryCounts?: Record<TopicCategory, number>
}

const props = withDefaults(defineProps<Props>(), {
  selectedCategory: null,
  categoryCounts: () => ({ event: 0, person: 0, keyword: 0 }),
})

const emit = defineEmits<{
  'update:category': [value: TopicCategory | null]
}>()

const categories: Array<{ key: TopicCategory; label: string; icon: string }> = [
  { key: 'event', label: '事件', icon: 'mdi:calendar-star' },
  { key: 'person', label: '人物', icon: 'mdi:account' },
  { key: 'keyword', label: '关键词', icon: 'mdi:tag' },
]

const categoryColors: Record<TopicCategory, string> = {
  event: '#f59e0b',
  person: '#10b981',
  keyword: '#6366f1',
}

function selectCategory(key: TopicCategory | null) {
  emit('update:category', key)
}
</script>

<template>
  <div class="topic-category-filter">
    <button
      type="button"
      class="topic-category-chip"
      :class="{ 'topic-category-chip--active': props.selectedCategory === null }"
      @click="selectCategory(null)"
    >
      <Icon icon="mdi:filter-variant" width="14" />
      全部
    </button>
    
    <button
      v-for="cat in categories"
      :key="cat.key"
      type="button"
      class="topic-category-chip"
      :class="{
        'topic-category-chip--active': props.selectedCategory === cat.key,
        [`topic-category-chip--${cat.key}`]: props.selectedCategory === cat.key,
      }"
      :style="props.selectedCategory === cat.key ? { '--cat-color': categoryColors[cat.key] } : {}"
      @click="selectCategory(cat.key)"
    >
      <Icon :icon="cat.icon" width="14" />
      {{ cat.label }}
      <span v-if="props.categoryCounts?.[cat.key]" class="topic-category-chip__count">
        {{ props.categoryCounts[cat.key] }}
      </span>
    </button>
  </div>
</template>

<style scoped>
.topic-category-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.topic-category-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 0.45rem 0.85rem;
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.7);
  background: rgba(255, 255, 255, 0.04);
  cursor: pointer;
  transition: all 0.2s ease;
}

.topic-category-chip:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: white;
}

.topic-category-chip--active {
  border-color: var(--cat-color, rgba(240, 138, 75, 0.72));
  background: rgba(255, 255, 255, 0.08);
  color: white;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
}

.topic-category-chip--event {
  --cat-color: rgba(245, 158, 11, 0.72);
}

.topic-category-chip--person {
  --cat-color: rgba(16, 185, 129, 0.72);
}

.topic-category-chip--keyword {
  --cat-color: rgba(99, 102, 241, 0.72);
}

.topic-category-chip__count {
  margin-left: 0.15rem;
  font-size: 0.7rem;
  opacity: 0.7;
}
</style>
```

**Step 2: Commit**

```bash
git add front/app/features/topic-graph/components/TopicCategoryFilter.vue
git commit -m "feat(frontend): create TopicCategoryFilter component"
```

---

### Task 3.3: Create Category History Panel

**Files:**
- Create: `front/app/features/topic-graph/components/TopicCategoryHistory.vue`

**Step 1: Create the component**

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed } from 'vue'
import type { CategoryHistoryPoint, TopicCategory } from '~/api/topicGraph'

interface Props {
  categoryHistory: CategoryHistoryPoint[]
  selectedCategory: TopicCategory | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'select:category': [value: TopicCategory]
}>()

const categoryIcons: Record<TopicCategory, string> = {
  event: 'mdi:calendar-star',
  person: 'mdi:account',
  keyword: 'mdi:tag',
}

const categoryColors: Record<TopicCategory, string> = {
  event: '#f59e0b',
  person: '#10b981',
  keyword: '#6366f1',
}

const sortedHistory = computed(() => {
  return [...props.categoryHistory].sort((a, b) => b.count - a.count)
})

function selectCategory(cat: TopicCategory) {
  emit('select:category', cat)
}
</script>

<template>
  <article class="topic-category-history rounded-[28px] p-4 md:p-5">
    <div class="flex items-start justify-between gap-3">
      <div>
        <p class="topic-category-history__eyebrow">分类热度</p>
        <p class="topic-category-history__lede">按事件、人物、关键词分类查看话题分布。</p>
      </div>
    </div>

    <div v-if="sortedHistory.length" class="topic-category-list mt-5 grid gap-3">
      <button
        v-for="point in sortedHistory"
        :key="point.category"
        type="button"
        class="topic-category-card"
        :class="{ 'topic-category-card--active': props.selectedCategory === point.category }"
        :style="{ '--cat-color': categoryColors[point.category] }"
        @click="selectCategory(point.category)"
      >
        <div class="topic-category-card__header">
          <Icon :icon="categoryIcons[point.category]" width="18" />
          <span class="topic-category-card__label">{{ point.label }}</span>
        </div>
        
        <div class="topic-category-card__body">
          <span class="topic-category-card__count">{{ point.count }}</span>
          <span class="topic-category-card__unit">次关联</span>
        </div>
        
        <div v-if="point.topics.length" class="topic-category-card__topics">
          <span v-for="(topic, idx) in point.topics.slice(0, 3)" :key="idx" class="topic-category-card__topic">
            {{ topic }}
          </span>
        </div>
      </button>
    </div>
    
    <div v-else class="topic-category-history__empty">
      选中一个主题后，这里会显示按分类的热度分布。
    </div>
  </article>
</template>

<style scoped>
.topic-category-history {
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(11, 18, 24, 0.72);
  box-shadow: 0 24px 80px rgba(6, 10, 16, 0.18);
  backdrop-filter: blur(12px);
}

.topic-category-history__eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.24em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.5);
}

.topic-category-history__lede {
  margin-top: 0.75rem;
  max-width: 22rem;
  color: rgba(220, 230, 239, 0.78);
  font-size: 0.95rem;
  line-height: 1.6;
}

.topic-category-history__empty {
  margin-top: 1rem;
  border-radius: 1rem;
  border: 1px dashed rgba(255, 255, 255, 0.14);
  padding: 0.9rem 1rem;
  color: rgba(255, 255, 255, 0.6);
  font-size: 0.92rem;
}

.topic-category-list {
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
}

.topic-category-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: linear-gradient(180deg, rgba(18, 27, 38, 0.96), rgba(10, 16, 24, 0.98));
  padding: 1rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.22s ease;
}

.topic-category-card:hover {
  border-color: var(--cat-color);
  transform: translateY(-2px);
}

.topic-category-card--active {
  border-color: var(--cat-color);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
}

.topic-category-card__header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--cat-color);
}

.topic-category-card__label {
  font-size: 0.85rem;
  font-weight: 600;
  color: rgba(248, 251, 255, 0.94);
}

.topic-category-card__body {
  display: flex;
  align-items: baseline;
  gap: 0.25rem;
}

.topic-category-card__count {
  font-family: Georgia, 'Times New Roman', serif;
  font-size: 1.6rem;
  line-height: 1;
  color: rgba(255, 240, 229, 0.96);
}

.topic-category-card__unit {
  font-size: 0.72rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(178, 196, 216, 0.68);
}

.topic-category-card__topics {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.25rem;
}

.topic-category-card__topic {
  font-size: 0.7rem;
  color: rgba(214, 225, 236, 0.72);
  background: rgba(255, 255, 255, 0.05);
  padding: 0.2rem 0.5rem;
  border-radius: 999px;
}
</style>
```

**Step 2: Commit**

```bash
git add front/app/features/topic-graph/components/TopicCategoryHistory.vue
git commit -m "feat(frontend): create TopicCategoryHistory component"
```

---

### Task 3.4: Refactor TopicGraphHeader - Add Home Button

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphHeader.vue`

**Step 1: Add home button below refresh**

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { TopicGraphType } from '~/api/topicGraph'

interface Props {
  selectedType: TopicGraphType
  selectedDate: string
  loading?: boolean
  heroLabel: string
  heroSubline: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})

const emit = defineEmits<{
  'update:type': [value: TopicGraphType]
  'update:date': [value: string]
  refresh: []
}>()

const typeOptions: TopicGraphType[] = ['daily', 'weekly']

function updateDate(event: Event) {
  emit('update:date', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <header class="topic-hero">
    <div class="flex flex-col gap-5">
      <div class="space-y-2">
        <p class="text-[0.65rem] uppercase tracking-[0.34em] text-white/50">Topic Field</p>
        <h1 class="font-serif text-2xl text-white md:text-3xl">{{ heroLabel }}</h1>
        <p class="text-xs leading-5 text-white/60">{{ heroSubline }}</p>
      </div>

      <div class="topic-toolbar">
        <div class="topic-toolbar__switcher" role="tablist" aria-label="主题图谱窗口切换">
          <button
            v-for="type in typeOptions"
            :key="type"
            type="button"
            class="topic-toolbar__switch"
            :class="{ 'topic-toolbar__switch--active': props.selectedType === type }"
            @click="emit('update:type', type)"
          >
            {{ type === 'daily' ? '日报图谱' : '周报图谱' }}
          </button>
        </div>

        <label class="topic-toolbar__date">
          <span class="topic-toolbar__eyebrow">时间锚点</span>
          <input class="topic-toolbar__input" :value="props.selectedDate" type="date" @input="updateDate">
        </label>

        <button class="topic-toolbar__button" type="button" :disabled="props.loading" @click="emit('refresh')">
          {{ props.loading ? '图谱载入中...' : '刷新图谱' }}
        </button>

        <!-- NEW: Home button -->
        <NuxtLink to="/" class="topic-toolbar__home">
          <Icon icon="mdi:home" width="16" />
          返回首页
        </NuxtLink>
      </div>
    </div>
  </header>
</template>

<style scoped>
/* ... existing styles ... */

.topic-toolbar__home {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  min-height: 2.5rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.8rem;
  text-decoration: none;
  transition: all 0.2s ease;
}

.topic-toolbar__home:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: white;
  background: rgba(255, 255, 255, 0.08);
}
</style>
```

**Step 2: Commit**

```bash
git add front/app/features/topic-graph/components/TopicGraphHeader.vue
git commit -m "feat(frontend): add home button to TopicGraphHeader"
```

---

### Task 3.5: Refactor TopicGraphPage - Remove Footer Panels, Add Category Filter

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphPage.vue`

**Step 1: Update imports and state**

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, ref, watch } from 'vue'
import { useArticlesApi } from '~/api/articles'
import { useTopicGraphApi, type TopicGraphDetailPayload, type TopicGraphType, type TopicCategory } from '~/api/topicGraph'
import type { Article } from '~/types'
import ArticleContentView from '~/features/articles/components/ArticleContentView.vue'
import TopicGraphCanvas from '~/features/topic-graph/components/TopicGraphCanvas.client.vue'
import TopicGraphHeader from '~/features/topic-graph/components/TopicGraphHeader.vue'
import TopicGraphSidebar from '~/features/topic-graph/components/TopicGraphSidebar.vue'
import TopicCategoryFilter from '~/features/topic-graph/components/TopicCategoryFilter.vue'
import TopicCategoryHistory from '~/features/topic-graph/components/TopicCategoryHistory.vue'
import { buildTopicGraphViewModel } from '~/features/topic-graph/utils/buildTopicGraphViewModel'

const topicGraphApi = useTopicGraphApi()
const articlesApi = useArticlesApi()

// ... existing date formatting and state ...

const selectedCategory = ref<TopicCategory | null>(null)

// ... existing computed properties ...

const categoryCounts = computed(() => graphPayload.value?.category_counts ?? { event: 0, person: 0, keyword: 0 })

// ... existing functions ...

async function loadGraph() {
  loadingGraph.value = true
  notice.value = null

  try {
    const response = await topicGraphApi.getGraph(selectedType.value, selectedDate.value, selectedCategory.value ?? undefined)
    if (!response.success || !response.data) {
      notice.value = response.error || '主题图谱没拉下来'
      graphPayload.value = null
      detail.value = null
      return
    }

    graphPayload.value = response.data
    selectedTopicSlug.value = response.data.top_topics[0]?.slug || null

    if (selectedTopicSlug.value) {
      void loadTopicDetail(selectedTopicSlug.value)
    } else {
      detail.value = null
    }
  } catch (error) {
    console.error('Failed to load topic graph:', error)
    notice.value = error instanceof Error ? error.message : '主题图谱加载失败'
  } finally {
    loadingGraph.value = false
  }
}

function handleCategorySelect(category: TopicCategory | null) {
  selectedCategory.value = category
  void loadGraph()
}

// ... existing watch handlers ...
</script>
```

**Step 2: Update template to remove footer panels and add category filter**

```vue
<template>
  <div
    class="topic-stage min-h-screen px-4 py-5 md:px-6 md:py-7"
    data-testid="topic-graph-page"
    :data-state="pageState"
  >
    <div class="topic-shell mx-auto w-full">
      <section class="topic-layout grid gap-5 2xl:grid-cols-[minmax(0,2.15fr)_minmax(430px,0.95fr)]">
        <div class="space-y-5">
          <article class="topic-canvas-shell rounded-[34px] p-4 md:p-5">
            <div class="topic-studio grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
              <aside class="topic-studio__rail rounded-[30px] p-4 md:p-5">
                <TopicGraphHeader
                  :selected-type="selectedType"
                  :selected-date="selectedDate"
                  :loading="loadingGraph"
                  :hero-label="viewModel.stats.heroLabel"
                  :hero-subline="viewModel.stats.heroSubline"
                  @update:type="selectedType = $event"
                  @update:date="selectedDate = $event"
                  @refresh="loadGraph"
                />

                <div class="mt-6">
                  <p class="text-xs uppercase tracking-[0.3em] text-white/42">Graph Field</p>
                  <h2 class="mt-2 font-serif text-2xl text-white md:text-[2.25rem]">{{ graphPayload?.period_label || '话题网络' }}</h2>
                  <p class="mt-3 text-sm leading-6 text-[rgba(255,255,255,0.68)]">
                    默认只保留重点标签常显，点中节点后再展开完整名字和一跳关系，减少视觉重叠。
                  </p>
                </div>

                <div class="mt-6 grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
                  <article v-for="card in statCards" :key="card.label" class="topic-stat-card rounded-[24px] px-4 py-3">
                    <p class="topic-stat-card__label">{{ card.label }}</p>
                    <p class="topic-stat-card__value">{{ card.value }}</p>
                  </article>
                </div>

                <!-- NEW: Category Filter -->
                <div class="mt-6">
                  <p class="text-xs uppercase tracking-[0.24em] text-white/42">分类筛选</p>
                  <div class="mt-3">
                    <TopicCategoryFilter
                      :selected-category="selectedCategory"
                      :category-counts="categoryCounts"
                      @update:category="handleCategorySelect"
                    />
                  </div>
                </div>

                <div class="mt-6">
                  <p class="text-xs uppercase tracking-[0.24em] text-white/42">热点题材</p>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <button
                      v-for="topic in topTopicLabels"
                      :key="topic.slug"
                      type="button"
                      class="topic-badge text-left"
                      :class="{
                        'topic-badge--event': topic.category === 'event',
                        'topic-badge--person': topic.category === 'person',
                        'topic-badge--keyword': topic.category === 'keyword',
                        'topic-badge--active': selectedTopicSlug === topic.slug,
                      }"
                      @click="loadTopicDetail(topic.slug)"
                    >
                      <Icon v-if="topic.icon" :icon="topic.icon" width="14" />
                      {{ topic.label }}
                    </button>
                  </div>
                </div>
              </aside>

              <div class="space-y-4">
                <TopicGraphCanvas
                  :nodes="viewModel.graph.nodes"
                  :edges="viewModel.graph.edges"
                  :featured-node-ids="viewModel.graph.featuredNodeIds"
                  :active-node-id="activeTopicNode?.id || null"
                  @node-click="handleNodeClick"
                />

                <article class="topic-note rounded-[30px] px-5 py-4 text-sm leading-6 text-[rgba(255,255,255,0.78)]">
                  <div class="flex items-start gap-3">
                    <Icon icon="mdi:orbit-variant" width="20" height="20" class="mt-1 text-[rgba(240,138,75,0.92)]" />
                    <p>
                      先看结构，再读内容：亮色主节点是当前焦点，周边只保留一跳关系的高亮，更多细节放到右侧阅读栏。
                    </p>
                  </div>
                </article>

                <!-- REPLACED: TopicCategoryHistory instead of TopicGraphFooterPanels -->
                <TopicCategoryHistory
                  v-if="detail"
                  :category-history="detail.category_history"
                  :selected-category="selectedCategory"
                  @select:category="handleCategorySelect"
                />
              </div>
            </div>
          </article>

          <p v-if="notice" class="rounded-[24px] border border-[rgba(240,138,75,0.28)] bg-[rgba(240,138,75,0.1)] px-4 py-3 text-sm text-[rgba(255,233,220,0.88)]">
            {{ notice }}
          </p>
        </div>

        <div class="topic-reading-rail" data-testid="topic-graph-sidebar-region">
          <TopicGraphSidebar
            :detail="detail"
            :loading="loadingDetail"
            :error="notice"
            :data-state="detail ? 'detail' : (loadingDetail ? 'loading' : 'empty')"
            @open-article="openArticlePreview"
          />
        </div>
      </section>
    </div>

    <!-- Article preview modal (unchanged) -->
    <Teleport to="body">
      <!-- ... existing modal code ... -->
    </Teleport>
  </div>
</template>
```

**Step 3: Commit**

```bash
git add front/app/features/topic-graph/components/TopicGraphPage.vue
git commit -m "feat(frontend): refactor TopicGraphPage - remove footer panels, add category filter"
```

---

### Task 3.6: Delete TopicGraphFooterPanels Component

**Files:**
- Delete: `front/app/features/topic-graph/components/TopicGraphFooterPanels.vue`

**Step 1: Remove the file**

```bash
rm front/app/features/topic-graph/components/TopicGraphFooterPanels.vue
```

**Step 2: Commit**

```bash
git add -A
git commit -m "refactor(frontend): remove TopicGraphFooterPanels component"
```

---

### Task 3.7: Update TopicGraphSidebar for Selection Sync

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphSidebar.vue`

**Step 1: Add category click handler**

```vue
<script setup lang="ts">
import { computed } from 'vue'
import type { TopicCategory, TopicGraphDetailPayload } from '~/api/topicGraph'

interface Props {
  detail: TopicGraphDetailPayload | null
  loading?: boolean
  error?: string | null
  dataState?: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  error: null,
  dataState: 'empty',
})

const emit = defineEmits<{
  openArticle: [articleId: number]
  selectCategory: [category: TopicCategory]
}>()

// ... existing computed properties ...

const topicCategoryLabels: Record<TopicCategory, string> = {
  event: '事件',
  person: '人物',
  keyword: '关键词',
}

function handleCategoryClick(category: TopicCategory) {
  emit('selectCategory', category)
}
</script>

<template>
  <!-- ... existing template, add category click handlers to related topics ... -->
  <section class="topic-panel rounded-[26px] p-4">
    <p class="topic-sidebar__eyebrow">相关主题</p>
    <div class="mt-4 flex flex-wrap gap-2">
      <button
        v-for="item in props.detail.related_topics"
        :key="item.slug"
        type="button"
        class="topic-pill"
        :class="`topic-pill--${item.category}`"
        @click="handleCategoryClick(item.category)"
      >
        {{ item.label }}
      </button>
    </div>
  </section>
</template>
```

**Step 2: Commit**

```bash
git add front/app/features/topic-graph/components/TopicGraphSidebar.vue
git commit -m "feat(frontend): add category selection emit to TopicGraphSidebar"
```

---

## Phase 4: AI Integration (Optional Enhancement)

### Task 4.1: Create Tag Analysis Service

**Files:**
- Create: `backend-go/internal/domain/topicgraph/analysis_generator.go`

**Step 1: Create AI analysis generator**

```go
package topicgraph

import (
	"encoding/json"
	"fmt"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/ai"
)

// GenerateTagAnalysis creates or updates AI analysis for a topic tag
func GenerateTagAnalysis(tagID uint, category string, summaries []models.AISummary) error {
	// Build context from summaries
	context := buildAnalysisContext(summaries)
	
	// Call AI to generate analysis
	prompt := buildAnalysisPrompt(category, context)
	response, err := ai.CallAI(prompt)
	if err != nil {
		return fmt.Errorf("AI call failed: %w", err)
	}
	
	// Parse response
	analysis, err := parseAnalysisResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse AI response: %w", err)
	}
	
	// Store in database
	return saveTagAnalysis(tagID, category, analysis)
}

func buildAnalysisContext(summaries []models.AISummary) string {
	// Extract key content from summaries
	var contexts []string
	for _, s := range summaries {
		contexts = append(contexts, fmt.Sprintf("- %s: %s", s.Title, truncate(s.Summary, 200)))
	}
	return strings.Join(contexts, "\n")
}

func buildAnalysisPrompt(category string, context string) string {
	categoryPrompts := map[string]string{
		"event": `分析以下内容，提取关键事件时间线。以JSON格式返回：
{
  "events": [
    {"date": "YYYY-MM-DD", "title": "事件标题", "description": "简述", "source": "来源"}
  ],
  "summary": "整体事件概述"
}`,
		"person": `分析以下内容，提取人物相关信息。以JSON格式返回：
{
  "summary": "人物简介",
  "context": "相关背景",
  "entities": [
    {"name": "相关人物/组织", "type": "person/org", "relation": "关系描述"}
  ]
}`,
		"keyword": `分析以下内容，提取关键词相关概念。以JSON格式返回：
{
  "summary": "概念解释",
  "context": "相关背景",
  "entities": [
    {"name": "相关概念", "type": "concept", "relation": "关联说明"}
  ]
}`,
	}
	
	return fmt.Sprintf("%s\n\n内容：\n%s", categoryPrompts[category], context)
}

func saveTagAnalysis(tagID uint, category string, analysis *TagAnalysisContent) error {
	eventsJSON, _ := json.Marshal(analysis.Events)
	entitiesJSON, _ := json.Marshal(analysis.Entities)
	
	tagAnalysis := models.TopicTagAnalysis{
		TopicTagID: tagID,
		Category:   category,
		Events:     string(eventsJSON),
		Summary:    analysis.Summary,
		Context:    analysis.Context,
		Entities:   string(entitiesJSON),
		Source:     "ai",
		Version:    1,
	}
	
	return database.DB.Save(&tagAnalysis).Error
}

type TagAnalysisContent struct {
	Events   []TimelineEvent   `json:"events"`
	Summary  string            `json:"summary"`
	Context  string            `json:"context"`
	Entities []RelatedEntity   `json:"entities"`
}
```

**Step 2: Commit**

```bash
git add backend-go/internal/domain/topicgraph/analysis_generator.go
git commit -m "feat(ai): add tag analysis generator service"
```

---

## Phase 5: Testing & Verification

### Task 5.1: Backend Tests

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/handler_test.go`

**Step 1: Add tests for category filter**

```go
func TestGetTopicGraphWithCategoryFilter(t *testing.T) {
	// Setup test database with sample data
	
	tests := []struct {
		name           string
		category       string
		expectCount    int
		expectCategory string
	}{
		{
			name:           "filter by event",
			category:       "event",
			expectCategory: "event",
		},
		{
			name:           "filter by person",
			category:       "person",
			expectCategory: "person",
		},
		{
			name:           "no filter",
			category:       "",
			expectCategory: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test implementation
		})
	}
}
```

**Step 2: Run tests**

```bash
cd backend-go && go test ./internal/domain/topicgraph -v
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicgraph/handler_test.go
git commit -m "test(backend): add category filter tests"
```

---

### Task 5.2: Frontend Type Check

**Step 1: Run typecheck**

```bash
cd front && pnpm exec nuxi typecheck
```

**Step 2: Fix any type errors**

**Step 3: Run build**

```bash
cd front && pnpm build
```

---

## Summary

This plan refactors the Topics page with:

1. **Database**: New `TopicTagAnalysis` table for storing AI-generated insights
2. **Backend**: Category filter on graph endpoint, new analysis endpoint, enhanced detail response
3. **Frontend**: 
   - Removed "站内动作" and "外部入口" panels
   - Added category filter tabs (事件/人物/关键词)
   - Transformed "历史温度" into category-based insights view
   - Added "返回首页" button in header
   - Selection synchronization between components

**Execution Options:**

1. **Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

2. **Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?