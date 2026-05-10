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
