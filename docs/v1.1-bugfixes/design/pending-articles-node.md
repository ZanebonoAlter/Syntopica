# Timeline "正在整理"节点设计

## 背景

Topic Graph 页面的 timeline 区域展示与选中 tag 相关的 digest 列表。但存在一种情况：文章已被打上 tag，但尚未被整理进任何 digest。用户需要看到这些"待整理"的文章。

## 需求

- 在 timeline 中增加"正在整理"节点，展示已打 tag 但未出现在任何 digest 中的文章数量
- 点击该节点后，右侧列表展示这些待整理文章
- 节点固定在日报列表最前面

## 设计

### 1. 后端 API

**新增接口**：`GET /api/topic-graph/tag/:slug/pending-articles`

**入参**：
- `slug`：标签 slug（路径参数）
- `type`：daily/weekly（查询参数，默认 daily）
- `date`：锚点日期（查询参数，可选）

**返回**：
```json
{
  "success": true,
  "data": {
    "articles": [
      {
        "id": 123,
        "title": "文章标题",
        "link": "https://...",
        "pub_date": "2026-03-26T10:00:00Z",
        "feed_name": "订阅源名称",
        "feed_icon": "mdi:rss",
        "feed_color": "#3b6b87"
      }
    ],
    "total": 5
  }
}
```

**判断逻辑**：
1. 查询时间窗口内所有带该 tag 的文章（通过 `article_topic_tags` 表）
2. 查询时间窗口内所有 digest/summary 的 `articles` JSON 字段，收集已覆盖的文章 ID
3. 返回差集（有 tag 但未被 digest 覆盖的文章）

### 2. 前端实现

#### 2.1 API 层

`front/app/api/topicGraph.ts` 新增：
```typescript
async getPendingArticlesByTag(slug: string, type: TopicGraphType, date?: string) {
  return apiClient.get<{ articles: PendingArticle[]; total: number }>(
    withQuery(`/topic-graph/tag/${slug}/pending-articles`, { type, date })
  )
}
```

#### 2.2 类型定义

`front/app/types/timeline.ts` 新增：
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
```

#### 2.3 Timeline 组件改造

`TopicTimeline.vue`：
- Props 新增 `pendingArticleCount: number`
- Props 新增 `selectedPendingNode: boolean`
- Emit 新增 `select-pending: []`
- 在 timeline-list 最前面渲染"正在整理"节点
- 节点样式：特殊的虚线样式，带"整理中"图标

`TimelineItem.vue` 不改动，新建 `TimelinePendingItem.vue` 专门处理待整理节点。

#### 2.4 TopicGraphPage 状态

新增状态：
```typescript
const pendingArticles = ref<PendingArticle[]>([])
const selectedPendingNode = ref(false)
const loadingPendingArticles = ref(false)
```

数据加载逻辑：
- `handleTagSelect` 和 `handleNodeClick` 时，并行调用 `loadPendingArticles`
- 点击"正在整理"节点时，设置 `selectedPendingNode = true`，清空 `selectedDigestId`

#### 2.5 右侧展示

`TopicGraphSidebar.vue`：
- Props 新增 `pendingArticles: PendingArticle[]`
- Props 新增 `selectedPendingNode: boolean`
- 当 `selectedPendingNode` 为 true 时，展示 pending articles 列表而非 digest articles

### 3. 数据流

```
用户点击热点标签
  ↓
handleTagSelect(slug, category)
  ↓
并行加载：
  - loadHotspotDigests(slug)     → hotspotDigests
  - loadPendingArticles(slug)    → pendingArticles
  - loadTopicDetail(slug)        → detail
  ↓
Timeline 展示：
  - "正在整理"节点（pendingArticles.length）
  - digest 列表
  ↓
用户点击"正在整理"节点
  ↓
selectedPendingNode = true
selectedDigestId = null
  ↓
右侧 Sidebar 展示 pendingArticles 列表
```

### 4. 文件改动清单

**后端**：
- `backend-go/internal/domain/topicgraph/handler.go`：新增 handler
- `backend-go/internal/domain/topicgraph/service.go`：新增 `GetPendingArticlesByTag` 函数
- `backend-go/internal/app/router.go`：注册新路由

**前端**：
- `front/app/api/topicGraph.ts`：新增 API 方法
- `front/app/types/timeline.ts`：新增 `PendingArticle` 类型
- `front/app/features/topic-graph/components/TopicTimeline.vue`：新增 pending 节点展示
- `front/app/features/topic-graph/components/TimelinePendingItem.vue`：新建组件
- `front/app/features/topic-graph/components/TopicGraphPage.vue`：新增状态和加载逻辑
- `front/app/features/topic-graph/components/TopicGraphSidebar.vue`：支持展示 pending articles