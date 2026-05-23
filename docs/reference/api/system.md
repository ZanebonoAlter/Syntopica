# 系统信息

### GET /

返回 API 名称和版本。

```json
{
  "name": "RSS Reader API (Go)",
  "version": "1.0.0",
  "endpoints": { ... }
}
```

### GET /health

健康检查。

```json
{
  "status": "healthy",
  "database": "connected"
}
```

### GET /api/tasks/status

全局任务状态汇总，返回所有后台队列的即时状态。

```json
{
  "success": true,
  "data": {
    "queue_size": 5,
    "active_tasks": 2,
    "tasks": [
      {
        "type": "summary_queue",
        "status": "running",
        "batch_id": "...",
        "total_jobs": 10,
        "completed_jobs": 5,
        "failed_jobs": 1,
        "pending_jobs": 4
      },
      {
        "type": "content_completion",
        "status": "running",
        "pending_count": 5,
        "processing_count": 1,
        "overview": { ... }
      },
      {
        "type": "firecrawl",
        "status": "running",
        "queue_size": 3,
        "processing_count": 1
      }
    ]
  }
}
```
