# 系统信息

### GET /health

健康检查。

```json
{
  "status": "healthy",
  "database": "connected"
}
```

### GET /api/image-proxy

外链图片代理：对目标图片注入 Referer/UA 后转发，绕开图床防盗链；内置磁盘缓存（change add-image-proxy，链路说明见 flow/reading.md §外链图片代理加载）。

**请求**：`GET /api/image-proxy?url=<urlencoded>`

| 参数 | 说明 |
|---|---|
| `url` | 目标图片地址，须 urlencode；仅 `http`/`https` |

**行为**：

- 注入 `Referer`（默认取图片自身 `scheme://host/`，per-host 可覆盖）与浏览器 UA 后转发上游，15s 超时。
- 命中 `data/image-cache/` 直接返回（响应头 `X-Image-Proxy-Cache: HIT`），未命中回源后落缓存（`MISS`；仅 200 + `image/*` 缓存，上限 `IMAGE_CACHE_MAX_MB` 默认 256MB）。
- 上游非 200 响应原样透传状态码（含 403），不写缓存——前端按既有降级渲染。
- 拒绝指向代理自身 host 的 URL（防循环）。

**错误码**：

| 状态码 | 条件 |
|---|---|
| `400` | 缺 `url` / 非 http(s) / 无 host / 指向代理自身 host |
| `502` | 上游不可达 |
| `504` | 上游超时（15s） |

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

### GET /api/poll

前端常驻状态的单一批量对账端点（harden-go-same-origin-serving，client-poll-budget 契约）：一次请求同时返回调度器状态、标签队列计数与通知未读数，替代前端三处独立轮询。复用三个分项端点的既有查询，**三个旧端点保留**（`/api/schedulers/status`、`/api/tag-queue/status`、`/api/notifications/unread-count`）供已打开的旧标签页继续工作。

**响应**：

```json
{
  "success": true,
  "data": {
    "schedulers": [],
    "tag_queue": { "pending": 0, "processing": 0, "completed": 0, "failed": 0, "total": 0, "completed_today": 0 },
    "notifications": { "unread": 0 }
  },
  "analysis_paused": false,
  "analysis_paused_at": "",
  "ai_healthy": true,
  "ai_health_routes": [],
  "server_time": "2026-09-24T22:00:00+08:00"
}
```

| 字段 | 说明 |
|---|---|
| `data.schedulers` | 调度器状态数组，结构同 `GET /api/schedulers/status` 的 `data` |
| `data.tag_queue` | 队列计数，结构同 `GET /api/tag-queue/status` 的 `data` |
| `data.notifications.unread` | 未读数，同 `GET /api/notifications/unread-count` |
| `analysis_paused` / `ai_healthy` / `ai_health_routes` | 顶层全局态，语义同 `GET /api/schedulers/status` 顶层字段 |
| `server_time` | 服务端时间（RFC3339），供前端时钟对齐 |

只读 demo 模式（调度器未注册）下 `data.schedulers` 为空数组、整体 HTTP 200（非 5xx，契约见 `flow/scheduler.md` 业务约束 #8）。
