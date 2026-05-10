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
