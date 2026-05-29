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
