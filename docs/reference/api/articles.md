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
| `per_page` | int | 20 | 缺省 20；`≤0` 视作 20；超上限按 100 返回并记 WARN 日志 |
| `feed_id` | int | - | 按订阅源 |
| `category_id` | int | - | 按分类 |
| `uncategorized` | string | - | `true` 未分类 |
| `read` | string | - | `true`/`false` |
| `favorite` | string | - | `true`/`false` |
| `search` | string | - | 标题或描述模糊搜索 |
| `start_date` | string | - | `YYYY-MM-DD` |
| `end_date` | string | - | `YYYY-MM-DD` |
| `watched_tag_ids` | string | - | 逗号分隔的标签 ID |
| `sort_by` | string | - | `relevance`（仅 watched_tag_ids 模式下有效） |

按发布日期降序，含 `tag_count`。使用 `watched_tag_ids` 时支持 `sort_by=relevance` 按标签相关度排序。

**窄投影（slim-article-list-payload）**：列表接口只返回扫描-选择字段。列表字段：`id` / `feed_id` / `category_id` / `title` / `link` / `image_url` / `pub_date` / `author` / `read` / `favorite` / `archived` / `summary_status` / `summary_generated_at` / `firecrawl_status` / `firecrawl_error` / `firecrawl_crawled_at` / `completion_error` / `created_at` / `tag_count` / `relevance_score`（仅 `sort_by=relevance` 分支有值）/ `excerpt`。**不返回** `content` / `description` / `firecrawl_content` / `ai_content_summary`（正文类字段均为**详情接口专属**），也不返回 `summary_processing_started_at` / `completion_attempts` / `content_form`——正文与完整导语走详情接口。

- `excerpt`：由 `description`（抽取后为空则回退 `content`）去 HTML 标签、还原实体、折叠空白后的纯文本，**≤200 字符**；源无实质内容（无字母/数字）时为空串 `""`；**与正文重复时（归一化后相同，或导语 ≥40 字符且被正文包含）也为空串**——去重 guard 本就会隐藏这类导语，下发只会造成首帧闪现；导语来自正文兜底（`description` 无实质文本）且该文章无 Firecrawl 正文时也为空串——展示正文就是 `content`，导语必然重复；字段恒存在。
- `per_page`：缺省 20；`≤0` 视作 20；`>100` 按 100 返回**并记一条 WARN 日志**（含请求值与路径，不再静默截断）。

### GET /api/articles/:article_id

单篇文章，附带标签列表。正文类字段（`content` / `description` / `firecrawl_content` / `ai_content_summary`）为**详情接口专属**，列表接口不返回（列表只给 `excerpt`）；本接口契约不变：

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
