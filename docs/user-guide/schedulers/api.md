<<<<<<< Updated upstream:docs/reference/api/schedulers.md
# 定时任务 Schedulers

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/schedulers/status` | 所有调度器状态 |
| GET | `/api/schedulers/:name/status` | 指定调度器状态 |
| POST | `/api/schedulers/:name/trigger` | 手动触发 |
| POST | `/api/schedulers/:name/reset` | 重置统计 |
| PUT | `/api/schedulers/:name/interval` | 更新间隔 |

---

### 支持的调度器

| 名称 | 别名 | 说明 |
|------|------|------|
| `auto_refresh` | - | 自动刷新 RSS |
| `preference_update` | - | 更新阅读偏好 |
| `content_completion` | `ai_summary` | 文章内容补全 |
| `firecrawl` | - | Firecrawl 全文抓取 |
| `digest` | - | Digest 日报/周报 |
| `tag_quality_score` | - | 重算标签质量分数 |
| `narrative_summary` | - | 生成每日叙事摘要 |
| `tag_hierarchy_cleanup` | - | 按三阶段策略清理 tag 体系 |

`tag_hierarchy_cleanup` 的 `last_run_summary` 现在主要看这几个字段：
- `zombie_deactivated`: 这一轮停用了多少长期没用的标签
- `flat_merges_applied`: 合并了多少明显重复的标签
- `orphaned_relations`: 删掉了多少失效的层级关系
- `multi_parent_fixed`: 修好了多少“一个标签挂了多个父标签”的问题
- `empty_abstracts`: 已废弃

### GET /api/schedulers/status

返回所有已注册调度器的状态列表。每个调度器包含：

```json
{
  "name": "content_completion",
  "status": "running",
  "check_interval": 300,
  "next_run": 1710000000,
  "is_executing": false,
  "description": "Complete article content and generate article summaries",
  "database_state": { ... },
  "overview": { ... },
  "last_run_summary": { ... }
}
```

### GET /api/schedulers/:name/status

返回单个调度器状态，同上结构。`404` 表示调度器不存在。

### POST /api/schedulers/:name/trigger

手动触发调度器。部分调度器支持 `?date=YYYY-MM-DD` 查询参数。

触发成功时返回执行结果或任务状态；调度器正忙时返回 `409`。

### POST /api/schedulers/:name/reset

重置调度器的统计信息（执行次数、错误计数等）。

### PUT /api/schedulers/:name/interval

```json
{ "interval": 30 }
```

`interval`：正整数，单位取决于调度器（一般为秒）。返回更新后的 `name` 和 `check_interval`。
=======
# 定时任务 Schedulers

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/schedulers/status` | 所有调度器状态 |
| GET | `/api/schedulers/:name/status` | 指定调度器状态 |
| POST | `/api/schedulers/:name/trigger` | 手动触发 |
| POST | `/api/schedulers/:name/reset` | 重置统计 |
| PUT | `/api/schedulers/:name/interval` | 更新间隔 |

---

### 支持的调度器

| 名称 | 别名 | 说明 |
|------|------|------|
| `auto_refresh` | - | 自动刷新 RSS |
| `preference_update` | - | 更新阅读偏好 |
| `content_completion` | `ai_summary` | 文章内容补全 |
| `firecrawl` | - | Firecrawl 全文抓取 |
| `digest` | - | Digest 日报/周报 |
| `tag_quality_score` | - | 重算标签质量分数 |
| `narrative_summary` | - | 生成每日叙事摘要 |
| `tag_hierarchy_cleanup` | - | 按三阶段策略清理 tag 体系 |

`tag_hierarchy_cleanup` 的 `last_run_summary` 现在主要看这几个字段：
- `zombie_deactivated`: 这一轮停用了多少长期没用的标签
- `flat_merges_applied`: 合并了多少明显重复的抽象标签
- `orphaned_relations`: 删掉了多少失效的层级关系
- `multi_parent_fixed`: 修好了多少“一个标签挂了多个父标签”的问题
- `empty_abstracts`: 停用了多少已经没有子标签的抽象标签

### GET /api/schedulers/status

返回所有已注册调度器的状态列表。每个调度器包含：

```json
{
  "name": "content_completion",
  "status": "running",
  "check_interval": 300,
  "next_run": 1710000000,
  "is_executing": false,
  "description": "Complete article content and generate article summaries",
  "database_state": { ... },
  "overview": { ... },
  "last_run_summary": { ... }
}
```

### GET /api/schedulers/:name/status

返回单个调度器状态，同上结构。`404` 表示调度器不存在。

### POST /api/schedulers/:name/trigger

手动触发调度器。部分调度器支持 `?date=YYYY-MM-DD` 查询参数。

触发成功时返回执行结果或任务状态；调度器正忙时返回 `409`。

### POST /api/schedulers/:name/reset

重置调度器的统计信息（执行次数、错误计数等）。

### PUT /api/schedulers/:name/interval

```json
{ "interval": 30 }
```

`interval`：正整数，单位取决于调度器（一般为秒）。返回更新后的 `name` 和 `check_interval`。
---
# 链路追踪 Traces

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/traces` | 按 trace_id 查询 |
| GET | `/api/traces/recent` | 最近链路 |
| GET | `/api/traces/search` | 搜索链路 |
| GET | `/api/traces/stats` | 追踪统计 |
| GET | `/api/traces/:trace_id/timeline` | 时间线 |
| GET | `/api/traces/:trace_id/otlp` | OTLP 导出 |

---

### GET /api/traces

查询参数：`trace_id`（必填）。返回该 trace 下所有 span。

### GET /api/traces/recent

查询参数：`limit`（默认 `50`）

### GET /api/traces/search

| 参数 | 类型 | 说明 |
|------|------|------|
| `operation` | string | 按操作名过滤 |
| `status` | string | `error` 查错误链路 |
| `min_duration_ms` | int64 | 按最小耗时过滤 |
| `limit` | int | 默认 `50` |

优先级：`status=error` > `operation` > `min_duration_ms` > 默认 recent。

### GET /api/traces/stats

追踪统计汇总。

### GET /api/traces/:trace_id/timeline

span 树形结构时间线。

### GET /api/traces/:trace_id/otlp

OTLP JSON 格式导出。
---
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
>>>>>>> Stashed changes:docs/user-guide/schedulers/api.md
