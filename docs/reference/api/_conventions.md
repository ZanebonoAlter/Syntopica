# API 通用约定

基路径：`/api`，后端默认 `http://localhost:5000`。

## 响应格式

所有接口返回 JSON，统一信封：

```json
{
  "success": true,
  "data": { ... },
  "message": "操作描述（可选）"
}
```

错误响应：

```json
{
  "success": false,
  "error": "错误描述"
}
```

## 分页

列表接口支持 `page`（默认 `1`）和 `per_page`（默认 `20`）：

```json
{
  "success": true,
  "data": [ ... ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 150,
    "pages": 8
  }
}
```

## 认证

个人/单用户部署，无认证系统。

## WebSocket

实时端点：`ws://localhost:5000/ws`，用于 AI 总结进度推送等。

## 全局任务状态

`GET /api/tasks/status` 返回所有后台任务的即时汇总（summary queue、content completion、firecrawl 的队列大小和活跃任务数）。

```json
{
  "success": true,
  "data": {
    "queue_size": 5,
    "active_tasks": 2,
    "tasks": [
      {
        "type": "content_completion",
        "status": "running",
        "pending_count": 5,
        "processing_count": 1,
        "overview": { ... }
      }
    ]
  }
}
```
