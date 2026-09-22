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
