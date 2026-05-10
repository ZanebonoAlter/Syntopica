# 主题图谱 Topic Graph

## 图谱查询

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/topic-graph/:type` | 获取图谱 |
| GET | `/api/topic-graph/topic/:slug` | 主题详情 |
| GET | `/api/topic-graph/by-category` | 按类别分组 |
| GET | `/api/topic-graph/topic/:slug/articles` | 主题关联文章 |
| GET | `/api/topic-graph/tag/:slug/digests` | 标签关联 Digest |
| GET | `/api/topic-graph/tag/:slug/pending-articles` | 未收录文章 |

---

### GET /api/topic-graph/:type

`type` 如 `daily`。

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD` |
| `category_id` | uint | 按分类 |
| `feed_id` | uint | 按订阅源 |

```json
{
  "success": true,
  "data": {
    "type": "daily",
    "anchor_date": "2024-01-15",
    "period_label": "2024-01-15 当日",
    "topic_count": 10,
    "summary_count": 25,
    "feed_count": 5,
    "top_topics": [
      { "slug": "ai-agent", "label": "AI Agent", "category": "keyword", "score": 5.2 }
    ],
    "nodes": [...],
    "edges": [...]
  }
}
```

### GET /api/topic-graph/topic/:slug

查询参数：`type`（默认 `daily`）、`date`、`category_id`、`feed_id`

```json
{
  "success": true,
  "data": {
    "topic": { "slug": "ai-agent", "label": "AI Agent", "category": "keyword" },
    "articles": [...],
    "total_articles": 50,
    "related_tags": [...],
    "summaries": [...],
    "history": [...],
    "related_topics": [...],
    "search_links": { "youtube_videos": "...", "youtube_live": "..." },
    "app_links": { "digest_view": "/digest/daily", "topic_graph": "/topics" }
  }
}
```

### GET /api/topic-graph/by-category

返回标签按 event/person/keyword 分组。

查询参数：`type`、`date`、`category_id`、`feed_id`

```json
{
  "success": true,
  "data": {
    "events": [{ "slug": "...", "label": "...", "category": "event", "score": 3.5 }],
    "people": [{ "slug": "...", "label": "...", "category": "person", "score": 2.8 }],
    "keywords": [{ "slug": "...", "label": "...", "category": "keyword", "score": 5.2 }]
  }
}
```

### GET /api/topic-graph/topic/:slug/articles

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `type` | string | daily | 窗口类型 |
| `date` | string | - | 锚点日期 |
| `page` | int | 1 | 页码 |
| `page_size` | int | 15 | 上限 100 |

```json
{
  "success": true,
  "data": {
    "articles": [
      {
        "id": "123",
        "title": "文章标题",
        "summary": "...",
        "pub_date": "2024-01-15T10:30:00Z",
        "feed_name": "Feed名称",
        "feed_id": "1",
        "link": "https://...",
        "tags": [{ "slug": "ai-agent", "label": "AI Agent", "category": "keyword" }],
        "image_url": "https://..."
      }
    ],
    "total": 100,
    "page": 1,
    "page_size": 15
  }
}
```

### GET /api/topic-graph/tag/:slug/digests

查询参数：`type`（默认 `daily`）、`date`、`limit`（默认 `20`，上限 100）

返回包含该标签文章的 digest 列表，附带 `total`。

### GET /api/topic-graph/tag/:slug/pending-articles

查询参数：`type`（默认 `daily`）、`date`

指定标签下未收录到任何 Digest 的文章。

---

## 主题分析 Topic Analysis

路由注册在 `/api/topic-graph/analysis` 下：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/topic-graph/analysis` | 按查询参数获取分析 |
| GET | `/api/topic-graph/analysis/status` | 按查询参数获取状态 |
| POST | `/api/topic-graph/analysis/rebuild` | 按查询参数重建 |
| POST | `/api/topic-graph/analysis/retry` | 同 rebuild |
| GET | `/api/topic-graph/analysis/:tagID/:analysisType` | 获取指定分析 |
| POST | `/api/topic-graph/analysis/:tagID/:analysisType/rebuild` | 重建指定分析 |
| GET | `/api/topic-graph/analysis/:tagID/:analysisType/status` | 获取状态 |

### 查询参数方式

GET `/api/topic-graph/analysis`：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `tag_id` | uint | 是 | 标签 ID |
| `analysis_type` | string | 是 | 分析类型 |
| `windowType` | string | 否 | 窗口类型 |
| `anchorDate` | string | 否 | `YYYY-MM-DD` |

### 路径参数方式

GET `/api/topic-graph/analysis/:tagID/:analysisType`：

| 参数 | 说明 |
|------|------|
| `tagID` | 标签 ID |
| `analysisType` | 分析类型 |

查询参数：`windowType`（或 `window_type`/`window`）、`anchorDate`（或 `anchor_date`/`date`）

POST 请求还支持 JSON body 传入 `windowType` 和 `anchorDate`。

### 分析状态响应

```json
{
  "success": true,
  "data": {
    "status": "processing",
    "progress": 65
  }
}
```

---

## 标签管理 Topic Tags

路由注册在 `/api/topic-tags` 下：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/topic-tags/search` | 搜索标签 |
| POST | `/api/topic-tags/merge` | 合并标签 |
| POST | `/api/topic-tags/merge-with-name` | 合并标签并重命名 |
| GET | `/api/topic-tags/merge-preview` | 扫描相似标签对 |
| GET | `/api/topic-tags/hierarchy` | 获取标签层级树 |
| POST | `/api/topic-tags/organize` | 异步整理未分类标签 |
| PUT | `/api/topic-tags/:tag_id/abstract-name` | 重命名抽象标签 |
| POST | `/api/topic-tags/:tag_id/detach` | 从抽象父标签分离子标签 |
| POST | `/api/topic-tags/:tag_id/reassign` | 将标签移到新父标签 |
| GET | `/api/topic-tags/watched` | 列出关注标签 |
| POST | `/api/topic-tags/:tag_id/watch` | 关注标签 |
| POST | `/api/topic-tags/:tag_id/unwatch` | 取消关注 |

### GET /api/topic-tags/search

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `q` | string | - | 搜索关键词（必填，空则返回空列表） |
| `category` | string | - | 按分类过滤 |
| `limit` | int | 20 | 上限 100 |

### POST /api/topic-tags/merge

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source_tag_id` | uint | 是 | 源标签 ID（将被合并） |
| `target_tag_id` | uint | 是 | 目标标签 ID（保留） |

返回合并后的 `source_id`、`target_id`、`target_label`。

### POST /api/topic-tags/merge-with-name

合并标签并可选重命名目标标签：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source_tag_id` | uint | 是 | 源标签 ID |
| `target_tag_id` | uint | 是 | 目标标签 ID |
| `new_name` | string | 是 | 目标标签新名称 |

### GET /api/topic-tags/merge-preview

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `limit` | int | 50 | 上限 100 |
| `include_articles` | string | false | `true` 附带文章标题 |
| `feed_id` | uint | - | 按订阅源过滤 |
| `category_id` | uint | - | 按分类过滤 |

返回相似标签候选列表。

### GET /api/topic-tags/hierarchy

| 参数 | 类型 | 说明 |
|------|------|------|
| `category` | string | 按分类过滤 |
| `unclassified` | string | `true` 查未分类标签 |
| `time_range` | string | 时间范围 |
| `feed_id` | uint | 按订阅源 |
| `category_id` | uint | 按分类 |

返回 `nodes`（层级节点列表）和 `total`。

### POST /api/topic-tags/organize

异步整理未分类标签。接口接受请求后返回 `202`，实际整理进度通过 WebSocket 推送 `organize_progress` 消息。

| 参数 | 类型 | 说明 |
|------|------|------|
| `category` | string | 可选。指定时只整理该分类；不指定时按每个标签自身分类查找相似候选 |

整理流程会先用 embedding 查找同分类相似标签，过滤当前标签自身和低相似度候选，再交给 LLM 判断是否合并或创建抽象父标签。LLM 判定为 merge 时会调用标签合并流程落库；判定为 abstract 时会创建抽象标签关系。

### PUT /api/topic-tags/:tag_id/abstract-name

重命名抽象标签：

```json
{ "new_name": "新名称" }
```

名称不能超过 160 字符。标签名冲突时返回 `409`。

### POST /api/topic-tags/:tag_id/detach

从抽象父标签分离子标签：

```json
{ "child_id": 42 }
```

### POST /api/topic-tags/:tag_id/reassign

将标签移到新的抽象父标签：

```json
{ "parent_id": 10 }
```

### GET /api/topic-tags/watched

列出所有关注标签，含抽象标签元数据。

### POST /api/topic-tags/:tag_id/watch

关注指定标签。返回 `id`、`is_watched`、`watched_at`。

### POST /api/topic-tags/:tag_id/unwatch

取消关注指定标签。返回 `id`、`is_watched`。

---

## Embedding 配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/embedding/config` | 获取所有 embedding 配置 |
| PUT | `/api/embedding/config/:key` | 更新单个配置项 |

### GET /api/embedding/config

返回所有 embedding 配置项列表。

### PUT /api/embedding/config/:key

```json
{ "value": "新值" }
```

---

## Embedding 队列

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/embedding/queue/status` | 队列状态 |
| GET | `/api/embedding/queue/tasks` | 任务列表 |
| POST | `/api/embedding/queue/retry` | 重试失败任务 |

### GET /api/embedding/queue/tasks

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `status` | string | - | 按状态过滤 |
| `limit` | int | 50 | 上限 200 |
| `offset` | int | 0 | 偏移 |

返回 `tasks` 和 `total`。

---

## Merge Reembedding 队列

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/embedding/merge-reembedding/status` | 队列状态 |
| GET | `/api/embedding/merge-reembedding/tasks` | 任务列表 |
| POST | `/api/embedding/merge-reembedding/retry` | 重试失败任务 |

### GET /api/embedding/merge-reembedding/tasks

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `status` | string | - | `pending`/`processing`/`completed`/`failed` |
| `limit` | int | 50 | 上限 200 |
| `offset` | int | 0 | 偏移 |

返回 `tasks` 和 `total`。

---

## 叙事 Narratives

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/narratives/timeline` | 叙事时间线 |
| GET | `/api/narratives/scopes` | 作用域列表（按分类聚合 Board） |
| GET | `/api/narratives` | 按日期或 Board 获取叙事列表 |
| DELETE | `/api/narratives` | 删除指定日期叙事 |
| POST | `/api/narratives/regenerate` | 重新生成叙事 |
| GET | `/api/narratives/:id` | 叙事详情（含树形结构） |
| GET | `/api/narratives/:id/history` | 叙事历史链 |
| GET | `/api/narratives/boards/timeline` | Board 时间线 |
| GET | `/api/narratives/boards/:id` | Board 详情 |
| GET | `/api/narratives/board-concepts` | 板块概念列表 |
| POST | `/api/narratives/board-concepts` | 创建板块概念 |
| PUT | `/api/narratives/board-concepts/:id` | 更新板块概念 |
| DELETE | `/api/narratives/board-concepts/:id` | 停用板块概念 |
| POST | `/api/narratives/board-concepts/suggest` | LLM 建议板块概念 |
| GET | `/api/narratives/unclassified` | 未分类标签桶 |

### GET /api/narratives/timeline

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，锚点日期，默认今天 |
| `days` | int | 天数范围，默认 7 |
| `scope_type` | string | `global` / `feed_category` |
| `category_id` | uint | 分类 ID |

返回按日期分组的叙事数量时间线。

### GET /api/narratives/scopes

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，锚点日期，默认今天 |
| `days` | int | 查询天数范围，默认 7 |

返回分类列表，每个分类包含 `board_count`（该时间范围内的 Board 数量）。数据源从 `narrative_boards` 聚合。

```json
{
  "success": true,
  "data": [
    {
      "category_id": 1,
      "name": "AI",
      "icon": "brain",
      "color": "#3b82f6",
      "board_count": 5
    }
  ]
}
```

### GET /api/narratives

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，默认今天 |
| `board_id` | uint | 指定 Board ID 获取该 Board 下的叙事 |
| `scope_type` | string | `global` / `feed_category` |
| `category_id` | uint | 分类 ID |

`board_id` 和 `date` 互斥，优先使用 `board_id`。

### DELETE /api/narratives

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，默认今天 |
| `scope_type` | string | 可选，限定作用域 |
| `category_id` | uint | 可选，限定分类 |

删除指定日期和范围的叙事及关联 Board。

### POST /api/narratives/regenerate

请求体：

```json
{
  "date": "2026-05-01",
  "scope_type": "feed_category",
  "category_id": 1
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，默认今天 |
| `scope_type` | string | 可选，`feed_category` 时需搭配 `category_id` |
| `category_id` | uint | 可选，指定分类重新生成 |

先删除再重新生成。返回 `{"success": true, "data": {"saved": 12}}`。

### GET /api/narratives/:id

返回完整的叙事树形结构。

### GET /api/narratives/:id/history

返回指定叙事的历史记录列表。

### GET /api/narratives/boards/timeline

| 参数 | 类型 | 说明 |
|------|------|------|
| `date` | string | `YYYY-MM-DD`，锚点日期，默认今天 |
| `days` | int | 天数范围，默认 7 |
| `scope_type` | string | `global` / `feed_category` |
| `category_id` | uint | 分类 ID |

返回按日期分组的 Board 列表时间线。每个 Board 包含 `id`、`name`、`description`、`board_concept_id`、`is_system` 等字段。

### GET /api/narratives/boards/:id

返回 Board 详情，包含关联的叙事列表。

### GET /api/narratives/board-concepts

返回所有活跃的板块概念列表。

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "AI 前沿",
      "description": "人工智能领域最新研究与应用",
      "scope_type": "global",
      "scope_category_id": null,
      "is_system": false,
      "is_active": true,
      "display_order": 0
    }
  ]
}
```

### POST /api/narratives/board-concepts

请求体：

```json
{
  "name": "AI 前沿",
  "description": "人工智能领域最新研究与应用",
  "scope_type": "global",
  "scope_category_id": null
}
```

创建后自动生成 embedding。返回创建的概念对象。

### PUT /api/narratives/board-concepts/:id

请求体：

```json
{
  "name": "新名称",
  "description": "新描述"
}
```

更新后自动重新生成 embedding。

### DELETE /api/narratives/board-concepts/:id

软删除（设置 `is_active=false`）。

### POST /api/narratives/board-concepts/suggest

LLM 扫描所有活跃抽象标签，建议初始板块概念列表。

```json
{
  "success": true,
  "data": [
    {
      "name": "编程工具",
      "description": "开发者工具、IDE、框架更新"
    }
  ]
}
```

### GET /api/narratives/unclassified

返回当前未匹配到任何概念板的标签列表（未归类桶）。
