# 日报 API

基础地址：`http://localhost:5000/api`

## POST `/daily-reports/generate`

手动异步触发日报生成。`board_id` 为空时生成所有当日有事件标签的板块。

Request：

```json
{ "date": "2026-05-26", "board_id": 2849 }
```

Response `data`：

```json
{ "job_id": "...", "status": "processing" }
```

WebSocket 进度消息：

```json
{
  "type": "daily_report_progress",
  "job_id": "...",
  "board_id": 2849,
  "board_name": "刚果（金）局势",
  "status": "generating",
  "saved": 0,
  "progress": "0/1",
  "timestamp": "2026-05-26T..."
}
```

终态总会广播：

```json
{
  "type": "daily_report_done",
  "job_id": "...",
  "total_saved": 1,
  "total_boards": 1,
  "timestamp": "2026-05-26T..."
}
```

`daily_report_progress.status` 使用 `generating`、`completed`、`failed`。

## GET `/semantic-boards/:id/daily-reports?days=7`

查询板块日报列表。

Response `data`：

```json
{
  "reports": [
    {
      "id": 1,
      "semantic_board_id": 2849,
      "period_date": "2026-05-26",
      "title": "...",
      "summary": "...",
      "status": "completed",
      "cluster_count": 2,
      "article_count": 3,
      "event_tag_count": 5,
      "created_at": "2026-05-26T..."
    }
  ]
}
```

## GET `/daily-reports/:id`

查询单篇日报详情（含 sections）。

Response `data`：

```json
{
  "report": {
    "id": 1,
    "semantic_board_id": 2849,
    "period_date": "2026-05-26T12:00:00Z",
    "title": "...",
    "summary": "...",
    "status": "completed",
    "highlights": [],
    "dynamics": "...",
    "sections": []
  }
}
```
