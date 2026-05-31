# 文章 Articles

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/articles/stats` | 文章统计 |
| GET | `/api/articles` | 文章列表 |
| GET | `/api/articles/:article_id` | 单篇文章 |
| POST | `/api/articles/:article_id/tags` | 重新打标签 |
| PUT | `/api/articles/:article_id` | 更新文章 |
| PUT | `/api/articles/bulk-update` | 批量更新 |

---

### GET /api/articles/stats

```json
{
  "success": true,
  "data": { "total": 1500, "unread": 320, "favorite": 45 }
}
```

### GET /api/articles

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `page` | int | 1 | 页码 |
| `per_page` | int | 20 | 上限 100 |
| `feed_id` | int | - | 按订阅源 |
| `category_id` | int | - | 按分类 |
| `uncategorized` | string | - | `true` 未分类 |
| `read` | string | - | `true`/`false` |
| `favorite` | string | - | `true`/`false` |
| `search` | string | - | 标题或描述模糊搜索 |
| `start_date` | string | - | `YYYY-MM-DD` |
| `end_date` | string | - | `YYYY-MM-DD` |
| `watched_tag_ids` | string | - | 逗号分隔的标签 ID，自动展开抽象标签子标签 |
| `sort_by` | string | - | `relevance`（仅 watched_tag_ids 模式下有效） |

按发布日期降序，含 `tag_count`。使用 `watched_tag_ids` 时支持 `sort_by=relevance` 按标签相关度排序。

### GET /api/articles/:article_id

单篇文章，附带标签列表：

```json
{
  "success": true,
  "data": {
    "id": 42,
    "feed_id": 1,
    "category_id": 2,
    "title": "文章标题",
    "description": "...",
    "content": "...",
    "link": "https://...",
    "image_url": "https://...",
    "pub_date": "2025-03-10 08:00:00",
    "author": "...",
    "read": false,
    "favorite": false,
    "summary_status": "complete",
    "ai_content_summary": "...",
    "firecrawl_status": "completed",
    "firecrawl_content": "...",
    "tag_count": 3,
    "tags": [ ... ]
  }
}
```

### POST /api/articles/:article_id/tags

异步重新生成标签。接口会把任务写入 `tag_jobs` 队列，立即返回 `job_id`；前端需监听 WebSocket `tag_completed` 事件或轮询 job 状态获取最终标签结果。

```json
{
  "success": true,
  "message": "标签任务已提交，请稍后刷新查看结果",
  "data": {
    "job_id": 18,
    "article_id": 42,
    "status": "pending"
  }
}
```

对应的 WebSocket 完成消息：

```json
{
  "type": "tag_completed",
  "article_id": 42,
  "job_id": 18,
  "tags": [
    {
      "slug": "ai-agent",
      "label": "AI Agent",
      "category": "keyword",
      "score": 0.92,
      "icon": "mdi:robot"
    }
  ]
}
```

### PUT /api/articles/:article_id

更新已读/收藏状态：

```json
{ "read": true, "favorite": false }
```

返回更新后的文章（含 `tag_count`）。

### PUT /api/articles/bulk-update

至少提供一个更新字段和一个过滤条件：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ids` | uint[] | 否 | 按 ID 列表 |
| `feed_id` | uint* | 否 | 按订阅源 |
| `category_id` | uint* | 否 | 按分类 |
| `uncategorized` | bool* | 否 | 未分类 |
| `read` | bool* | 否 | 已读状态 |
| `favorite` | bool* | 否 | 收藏状态 |

过滤优先级：`ids` > `feed_id` > `category_id` > `uncategorized`。

成功时 `message` 为受影响的行数。
---
# 阅读行为与用户偏好

## 阅读行为 Reading Behavior

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/reading-behavior/track` | 记录单条 |
| POST | `/api/reading-behavior/track-batch` | 批量记录 |
| GET | `/api/reading-behavior/stats` | 阅读统计 |

### POST /api/reading-behavior/track

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `article_id` | uint | 是 | 文章 ID |
| `feed_id` | uint | 是 | 订阅源 ID |
| `session_id` | string | 是 | 会话 ID |
| `event_type` | string | 是 | open, close, scroll, favorite 等 |
| `category_id` | uint* | 否 | 留空自动从 feed 填充 |
| `scroll_depth` | int | 否 | 滚动深度 |
| `reading_time` | int | 否 | 秒 |

返回创建的记录。

### POST /api/reading-behavior/track-batch

```json
{ "events": [ { ...同 track 格式... }, ... ] }
```

返回 `{ "success": true, "message": 5 }`（`message` 为写入条数）。

### GET /api/reading-behavior/stats

```json
{
  "success": true,
  "data": {
    "total_articles": 200,
    "total_reading_time": 18000,
    "avg_reading_time": 90.5,
    "avg_scroll_depth": 72.3,
    "most_active_feed_id": 3,
    "most_active_category": 1
  }
}
```

---

## 用户偏好 User Preferences

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/user-preferences` | 偏好列表 |
| POST | `/api/user-preferences/update` | 触发偏好重算 |

### GET /api/user-preferences

| 参数 | 类型 | 说明 |
|------|------|------|
| `type` | string | `feed`/`category`，留空返回全部 |

按偏好分数降序，含关联 Feed/Category 信息。自动过滤已删除的 Feed/Category。

### POST /api/user-preferences/update

后台执行偏好重算。若调度器可用则通过 `TriggerNow()` 触发，否则启动 goroutine 异步执行。

调度器正忙时返回 `409`。
---
# 内容补全 Content Completion

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/content-completion/articles/:article_id/complete` | 补全单篇 |
| POST | `/api/content-completion/feeds/:feed_id/complete-all` | 补全整个订阅源 |
| GET | `/api/content-completion/articles/:article_id/status` | 补全状态 |
| GET | `/api/content-completion/overview` | 补全总览 |

---

### POST .../articles/:article_id/complete

触发单篇补全（Firecrawl + AI 整理）。

可选请求体：`{ "force": true }`

成功返回 `{ "success": true, "message": "Content completion initiated" }`。

### POST .../feeds/:feed_id/complete-all

补全 `incomplete` 或 `failed` 状态的文章：

```json
{
  "success": true,
  "completed": 5,
  "failed": 1,
  "total": 6
}
```

### GET .../articles/:article_id/status

```json
{
  "success": true,
  "data": {
    "summary_status": "complete",
    "attempts": 1,
    "error": "",
    "summary_generated_at": "2025-03-10 10:00:00",
    "ai_content_summary": "...",
    "firecrawl_content": "...",
    "firecrawl_status": "completed",
    "firecrawl_error": "",
    "firecrawl_crawled_at": "2025-03-10 09:58:00"
  }
}
```

### GET .../overview

```json
{
  "success": true,
  "data": {
    "pending_count": 10,
    "processing_count": 2,
    "completed_count": 500,
    "failed_count": 3,
    "blocked_count": 5,
    "total_count": 520,
    "ai_configured": true,
    "blocked_reasons": {
      "waiting_for_firecrawl_count": 3,
      "feed_disabled_count": 1,
      "ai_unconfigured_count": 0,
      "ready_but_missing_content_count": 1
    },
    "is_executing": false,
    "current_article": null,
    "last_processed": "2025-03-10 10:00:00",
    "next_run": 1710000000,
    "last_error": "",
    "database_state": { ... },
    "overview": { ... }
  }
}
```

`overview` 还会注入当前 content_completion 调度器的执行状态（`is_executing`、`current_article` 等）。
