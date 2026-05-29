# Timeline "正在整理"节点实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 topic graph 的 timeline 中展示"正在整理"节点，显示已打 tag 但未被 digest 覆盖的文章。

**Architecture:** 后端新增 API 查询待整理文章，前端在 timeline 最前添加特殊节点，点击后右侧展示文章列表。

**Tech Stack:** Go (Gin/GORM), Vue 3 (Composition API), TypeScript

---

## Task 1: 后端类型定义

**Files:**
- Modify: `backend-go/internal/domain/topictypes/types.go`

**Step 1: 添加 PendingArticle 类型**

在 `topictypes/types.go` 文件末尾添加：

```go
// PendingArticle represents an article that has a tag but is not yet in any digest
type PendingArticle struct {
	ID        uint   `json:"id"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	PubDate   string `json:"pub_date,omitempty"`
	FeedName  string `json:"feed_name"`
	FeedIcon  string `json:"feed_icon,omitempty"`
	FeedColor string `json:"feed_color,omitempty"`
}

// PendingArticlesResponse is the response for pending articles API
type PendingArticlesResponse struct {
	Articles []PendingArticle `json:"articles"`
	Total    int              `json:"total"`
}
```

**Step 2: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topictypes/types.go
git commit -m "feat(topic-graph): add PendingArticle type definition"
```

---

## Task 2: 后端 Service 层实现

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/service.go`

**Step 1: 添加 GetPendingArticlesByTag 函数**

在 `service.go` 文件末尾添加：

```go
// GetPendingArticlesByTag retrieves articles that have the given tag but are not in any digest
func GetPendingArticlesByTag(tagSlug string, kind string, anchor time.Time) (*topictypes.PendingArticlesResponse, error) {
	windowStart, windowEnd, _, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	// Step 1: Get the topic tag
	var topicTag models.TopicTag
	err = database.DB.Where("slug = ?", tagSlug).First(&topicTag).Error
	if err != nil {
		return nil, fmt.Errorf("topic tag not found: %w", err)
	}

	// Step 2: Get articles with this tag in the time window
	var taggedArticles []models.Article
	err = database.DB.
		Joins("JOIN article_topic_tags ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id = ?", topicTag.ID).
		Where("articles.created_at >= ? AND articles.created_at < ?", windowStart, windowEnd).
		Preload("Feed").
		Find(&taggedArticles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tagged articles: %w", err)
	}

	if len(taggedArticles) == 0 {
		return &topictypes.PendingArticlesResponse{Articles: []topictypes.PendingArticle{}, Total: 0}, nil
	}

	// Step 3: Get all article IDs that are already in digests
	taggedArticleIDs := make([]uint, len(taggedArticles))
	for i, a := range taggedArticles {
		taggedArticleIDs[i] = a.ID
	}

	var summaries []models.AISummary
	err = database.DB.
		Where("created_at >= ? AND created_at < ?", windowStart, windowEnd).
		Where("articles IS NOT NULL AND articles != ''").
		Find(&summaries).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get summaries: %w", err)
	}

	// Build set of article IDs that are in any digest
	digestArticleIDs := make(map[uint]bool)
	for _, summary := range summaries {
		ids := parseSummaryArticleIDs(summary.Articles)
		for _, id := range ids {
			digestArticleIDs[id] = true
		}
	}

	// Step 4: Filter articles that are not in any digest
	var pendingArticles []topictypes.PendingArticle
	for _, article := range taggedArticles {
		if digestArticleIDs[article.ID] {
			continue
		}

		pa := topictypes.PendingArticle{
			ID:    article.ID,
			Title: article.Title,
			Link:  article.Link,
		}

		if article.PubDate != nil {
			pa.PubDate = article.PubDate.In(topictypes.TopicGraphCST).Format(time.RFC3339)
		}

		if article.Feed.ID != 0 {
			pa.FeedName = article.Feed.Title
			pa.FeedIcon = article.Feed.Icon
			pa.FeedColor = article.Feed.Color
		} else {
			pa.FeedName = "未知订阅源"
		}

		pendingArticles = append(pendingArticles, pa)
	}

	return &topictypes.PendingArticlesResponse{
		Articles: pendingArticles,
		Total:    len(pendingArticles),
	}, nil
}
```

**Step 2: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicgraph/service.go
git commit -m "feat(topic-graph): add GetPendingArticlesByTag service function"
```

---

## Task 3: 后端 Handler 层实现

**Files:**
- Modify: `backend-go/internal/domain/topicgraph/handler.go`

**Step 1: 添加 GetPendingArticlesByTagHandler 函数**

在 `handler.go` 文件中，`GetDigestsByArticleTagHandler` 函数之后添加：

```go
// GetPendingArticlesByTagHandler returns articles with the given tag that are not in any digest
func GetPendingArticlesByTagHandler(c *gin.Context) {
	tagSlug := c.Param("slug")
	if tagSlug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "tag slug is required"})
		return
	}

	kind := c.DefaultQuery("type", "daily")
	anchor, err := topictypes.ParseAnchorDate(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := GetPendingArticlesByTag(tagSlug, kind, anchor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
```

**Step 2: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicgraph/handler.go
git commit -m "feat(topic-graph): add GetPendingArticlesByTagHandler"
```

---

## Task 4: 后端路由注册

**Files:**
- Modify: `backend-go/internal/app/router.go`

**Step 1: 添加路由**

在 `router.go` 的 `topicGraph` 路由组中，在 `topicGraph.GET("/tag/:slug/digests", ...)` 之后添加：

```go
topicGraph.GET("/tag/:slug/pending-articles", topicgraphdomain.GetPendingArticlesByTagHandler)
```

**Step 2: 验证编译和启动**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add backend-go/internal/app/router.go
git commit -m "feat(topic-graph): register pending-articles route"
```

---

## Task 5: 前端类型定义

**Files:**
- Modify: `front/app/types/timeline.ts`

**Step 1: 添加 PendingArticle 类型**

在 `timeline.ts` 文件末尾添加：

```typescript
export interface PendingArticle {
  id: number
  title: string
  link: string
  pubDate?: string
  feedName: string
  feedIcon?: string
  feedColor?: string
}

export interface PendingArticlesResponse {
  articles: PendingArticle[]
  total: number
}
```

**Step 2: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 3: Commit**

```bash
git add front/app/types/timeline.ts
git commit -m "feat(topic-graph): add PendingArticle type definition"
```

---

## Task 6: 前端 API 层

**Files:**
- Modify: `front/app/api/topicGraph.ts`

**Step 1: 导入类型**

在文件顶部的导入中添加：

```typescript
import type { PendingArticlesResponse } from '~/types/timeline'
```

**Step 2: 添加 API 方法**

在 `useTopicGraphApi()` 返回的对象中，`getTopicArticles` 方法之后添加：

```typescript
async getPendingArticlesByTag(slug: string, type: TopicGraphType, date?: string) {
  return apiClient.get<PendingArticlesResponse>(withQuery(`/topic-graph/tag/${slug}/pending-articles`, {
    type,
    date,
  }))
},
```

**Step 3: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 4: Commit**

```bash
git add front/app/api/topicGraph.ts
git commit -m "feat(topic-graph): add getPendingArticlesByTag API method"
```

---

## Task 7: TimelinePendingItem 组件

**Files:**
- Create: `front/app/features/topic-graph/components/TimelinePendingItem.vue`

**Step 1: 创建组件**

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'

interface Props {
  count: number
  isActive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  isActive: false,
})

const emit = defineEmits<{
  select: []
}>()

function handleSelect() {
  emit('select')
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    handleSelect()
  }
}
</script>

<template>
  <article class="timeline-pending-item">
    <div class="timeline-pending-item__marker">
      <div class="timeline-pending-item__dot" :class="{ 'timeline-pending-item__dot--active': props.isActive }" />
      <div class="timeline-pending-item__line" />
    </div>

    <div class="timeline-pending-item__content">
      <div
        class="timeline-pending-item__body"
        :class="{ 'timeline-pending-item__body--active': props.isActive }"
        role="button"
        tabindex="0"
        @click="handleSelect"
        @keydown="handleKeydown"
      >
        <div class="timeline-pending-item__header">
          <Icon icon="mdi:file-document-edit-outline" width="18" />
          <span class="timeline-pending-item__title">正在整理</span>
          <span class="timeline-pending-item__count">{{ props.count }} 篇文章</span>
        </div>
        <p class="timeline-pending-item__hint">
          已打标签但尚未生成日报的文章，点击查看详情
        </p>
      </div>
    </div>
  </article>
</template>

<style scoped>
.timeline-pending-item {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 0.9rem;
  position: relative;
}

.timeline-pending-item__marker {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.timeline-pending-item__dot {
  width: 12px;
  height: 12px;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.28);
  border: 2px dashed rgba(240, 138, 75, 0.56);
  flex-shrink: 0;
}

.timeline-pending-item__dot--active {
  background: rgba(240, 138, 75, 0.56);
  box-shadow: 0 0 12px rgba(240, 138, 75, 0.34);
}

.timeline-pending-item__line {
  width: 2px;
  flex: 1;
  min-height: 24px;
  margin-top: 4px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.28), rgba(255, 255, 255, 0.03));
}

.timeline-pending-item__content {
  display: grid;
  gap: 0.55rem;
  padding-bottom: 1.4rem;
}

.timeline-pending-item__body {
  display: grid;
  gap: 0.55rem;
  text-align: left;
  border-radius: 1.15rem;
  border: 1px dashed rgba(240, 138, 75, 0.28);
  background: linear-gradient(180deg, rgba(40, 30, 25, 0.88), rgba(25, 18, 14, 0.94));
  padding: 1rem;
  transition: all 0.18s ease;
}

.timeline-pending-item__body:hover,
.timeline-pending-item__body--active {
  border-color: rgba(240, 138, 75, 0.48);
  background: linear-gradient(180deg, rgba(48, 36, 28, 0.94), rgba(30, 22, 16, 0.98));
}

.timeline-pending-item__header {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: rgba(240, 138, 75, 0.92);
}

.timeline-pending-item__title {
  font-size: 0.95rem;
  font-weight: 600;
}

.timeline-pending-item__count {
  margin-left: auto;
  font-size: 0.78rem;
  padding: 0.22rem 0.58rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.15);
  color: rgba(255, 231, 213, 0.88);
}

.timeline-pending-item__hint {
  font-size: 0.82rem;
  line-height: 1.5;
  color: rgba(214, 225, 236, 0.62);
}

@media (max-width: 640px) {
  .timeline-pending-item {
    grid-template-columns: 34px minmax(0, 1fr);
    gap: 0.65rem;
  }

  .timeline-pending-item__body {
    padding: 0.85rem;
  }
}
</style>
```

**Step 2: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 3: Commit**

```bash
git add front/app/features/topic-graph/components/TimelinePendingItem.vue
git commit -m "feat(topic-graph): add TimelinePendingItem component"
```

---

## Task 8: TopicTimeline 组件改造

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicTimeline.vue`

**Step 1: 添加导入**

在 `<script setup>` 顶部添加：

```typescript
import TimelinePendingItem from './TimelinePendingItem.vue'
```

**Step 2: 添加 Props**

在 `interface Props` 中添加：

```typescript
pendingArticleCount?: number
selectedPendingNode?: boolean
```

在 `withDefaults` 中添加默认值：

```typescript
pendingArticleCount: 0,
selectedPendingNode: false,
```

**Step 3: 添加 Emit**

在 `defineEmits` 中添加：

```typescript
'select-pending': []
```

**Step 4: 添加事件处理函数**

在 `handlePreviewDigest` 函数之后添加：

```typescript
function handleSelectPending() {
  emit('select-pending')
}
```

**Step 5: 更新模板**

在 `<div class="timeline-list">` 内部开头添加 pending 节点：

```vue
<TimelinePendingItem
  v-if="selectedTopic && props.pendingArticleCount > 0"
  :count="props.pendingArticleCount"
  :is-active="props.selectedPendingNode"
  @select="handleSelectPending"
/>
```

**Step 6: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 7: Commit**

```bash
git add front/app/features/topic-graph/components/TopicTimeline.vue
git commit -m "feat(topic-graph): add pending node to TopicTimeline"
```

---

## Task 9: TopicGraphSidebar 支持展示 Pending Articles

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphSidebar.vue`

**Step 1: 添加类型导入**

在 `<script setup>` 顶部添加类型导入：

```typescript
import type { PendingArticle } from '~/types/timeline'
```

**Step 2: 添加 Props**

在 `interface Props` 中添加：

```typescript
pendingArticles?: PendingArticle[]
selectedPendingNode?: boolean
```

**Step 3: 添加 Pending Articles 列表展示**

在模板中，找到展示 digest articles 的区域，添加条件渲染：

```vue
<!-- Pending Articles Section -->
<section v-if="props.selectedPendingNode && props.pendingArticles && props.pendingArticles.length > 0" class="sidebar-section">
  <h3 class="sidebar-section__title">
    <Icon icon="mdi:file-document-edit-outline" width="16" />
    待整理文章
  </h3>
  <ul class="sidebar-articles-list">
    <li
      v-for="article in props.pendingArticles"
      :key="article.id"
      class="sidebar-article-item"
    >
      <button
        type="button"
        class="sidebar-article-link"
        @click="emit('open-article', article.id)"
      >
        {{ article.title }}
      </button>
      <span class="sidebar-article-meta">
        {{ article.feedName }}
      </span>
    </li>
  </ul>
</section>
```

**Step 4: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 5: Commit**

```bash
git add front/app/features/topic-graph/components/TopicGraphSidebar.vue
git commit -m "feat(topic-graph): add pending articles display to sidebar"
```

---

## Task 10: TopicGraphPage 状态与逻辑集成

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphPage.vue`

**Step 1: 添加状态**

在 `selectedPreviewArticle` 定义之后添加：

```typescript
const pendingArticles = ref<PendingArticle[]>([])
const selectedPendingNode = ref(false)
const loadingPendingArticles = ref(false)
```

**Step 2: 添加 loadPendingArticles 函数**

在 `loadHotspotDigests` 函数之后添加：

```typescript
async function loadPendingArticles(tagSlug: string) {
  loadingPendingArticles.value = true
  try {
    const response = await topicGraphApi.getPendingArticlesByTag(
      tagSlug,
      selectedType.value,
      selectedDate.value
    )
    if (response.success && response.data) {
      pendingArticles.value = response.data.articles || []
    } else {
      pendingArticles.value = []
    }
  } catch (error) {
    console.error('Failed to load pending articles:', error)
    pendingArticles.value = []
  } finally {
    loadingPendingArticles.value = false
  }
}
```

**Step 3: 更新 handleTagSelect 函数**

在 `handleTagSelect` 函数中，`loadHotspotDigests(slug)` 调用之后添加：

```typescript
void loadPendingArticles(slug)
```

**Step 4: 更新 handleNodeClick 函数**

在 `handleNodeClick` 函数中，`void loadHotspotDigests(node.slug)` 调用之后添加：

```typescript
void loadPendingArticles(node.slug)
```

**Step 5: 添加 handleSelectPending 函数**

在 `handlePreviewDigest` 函数之后添加：

```typescript
function handleSelectPending() {
  selectedPendingNode.value = true
  selectedDigestId.value = null
  previewDigestId.value = null
}
```

**Step 6: 更新 selectedDigest watcher**

在 `watch(effectiveTimelineItems, ...)` 中，`selectedDigestId.value = items[0]?.id || null` 这行之后添加：

```typescript
selectedPendingNode.value = false
```

**Step 7: 更新 TopicTimeline 组件调用**

在模板中找到 `<TopicTimeline ... />`，添加新 props：

```vue
:pending-article-count="pendingArticles.length"
:selected-pending-node="selectedPendingNode"
@select-pending="handleSelectPending"
```

**Step 8: 更新 TopicGraphSidebar 组件调用**

在模板中找到 `<TopicGraphSidebar ... />`，添加新 props：

```vue
:pending-articles="selectedPendingNode ? pendingArticles : []"
:selected-pending-node="selectedPendingNode"
```

**Step 9: 验证类型检查**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: 类型检查通过

**Step 10: Commit**

```bash
git add front/app/features/topic-graph/components/TopicGraphPage.vue
git commit -m "feat(topic-graph): integrate pending articles into TopicGraphPage"
```

---

## Task 11: 端到端验证

**Step 1: 启动后端**

Run: `cd backend-go && go run cmd/server/main.go`

**Step 2: 启动前端**

Run: `cd front && pnpm dev`

**Step 3: 手动测试**

1. 打开 `http://localhost:3000/topics`
2. 点击一个热点标签
3. 验证 timeline 最前面出现"正在整理"节点
4. 点击节点，验证右侧展示待整理文章列表
5. 点击文章标题，验证文章预览弹窗正常打开

**Step 4: 运行前端类型检查**

Run: `cd front && pnpm exec nuxi typecheck`

**Step 5: 运行后端测试**

Run: `cd backend-go && go test ./internal/domain/topicgraph/... -v`

**Step 6: 最终 Commit**

```bash
git add -A
git commit -m "feat(topic-graph): complete pending articles node implementation"
```