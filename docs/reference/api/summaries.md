# AI 总结 Summaries

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/summaries` | 总结列表 |
| GET | `/api/summaries/:summary_id` | 单个总结 |
| DELETE | `/api/summaries/:summary_id` | 删除总结 |
| POST | `/api/summaries/queue` | 提交批量总结 |
| GET | `/api/summaries/queue/status` | 队列状态 |
| GET | `/api/summaries/queue/jobs/:job_id` | 任务详情 |
| GET | `/api/auto-summary/status` | 自动总结状态 |
| POST | `/api/auto-summary/config` | 更新自动总结配置 |

---

### GET /api/summaries

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `feed_id` | int | - | 按订阅源 |
| `category_id` | int | - | 按分类 |
| `page` | int | 1 | 页码 |
| `per_page` | int | 20 | 每页条数 |

返回带分页的总结列表，含关联 Feed 和 Category。

### GET /api/summaries/:summary_id

单个总结详情，含关联 Feed 和 Category。

### DELETE /api/summaries/:summary_id

删除指定总结。

### POST /api/summaries/queue

提交批量 AI 总结任务：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `category_ids` | uint[] | 否 | 分类 ID 列表 |
| `feed_ids` | uint[] | 否 | 订阅源 ID 列表 |
| `time_range` | int | 否 | 时间范围（天） |
| `base_url` | string | 否 | AI 服务地址 |
| `api_key` | string | 否 | API Key |
| `model` | string | 否 | 模型名 |

`category_ids` 和 `feed_ids` 至少提供一个。

`202`：

```json
{ "success": true, "message": "Summary job queued successfully", "data": { ... } }
```

### GET /api/summaries/queue/status

当前队列批次状态。无活跃任务时 `data` 为 `null`。

### GET /api/summaries/queue/jobs/:job_id

指定队列任务详情。

### GET /api/auto-summary/status

获取自动总结配置状态。优先从 AI Router 获取，回退到 legacy 配置。

```json
{
  "success": true,
  "data": {
    "enabled": true,
    "status": "configured",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o-mini",
    "route_name": "default",
    "provider_id": 1,
    "time_range": 180
  }
}
```

未配置时返回 `{"enabled": false, "status": "not_configured"}`。

### POST /api/auto-summary/config

更新自动总结配置：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `base_url` | string | 否 | AI 服务地址 |
| `api_key` | string | 否 | API Key |
| `model` | string | 否 | 模型名 |
| `time_range` | int | 否 | 默认 `180` 天 |

若 `base_url`/`api_key`/`model` 均非空，同步更新 AI Provider/Route 和 legacy 配置，并热重载调度器。
