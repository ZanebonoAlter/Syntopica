# 定时任务 Schedulers

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/schedulers/status` | 所有调度器状态 |
| GET | `/api/schedulers/:name/status` | 指定调度器状态 |
| POST | `/api/schedulers/:name/trigger` | 手动触发 |
| POST | `/api/schedulers/:name/reset` | 重置统计 |
| PUT | `/api/schedulers/:name/interval` | 更新间隔 |
| PUT | `/api/schedulers/:name/schedule-time` | 更新固定时间点 |

---

### 支持的调度器

| 名称 | 别名 | 说明 |
| ------ | ------ | ------ |
| `auto_refresh` | - | 自动刷新 RSS |
| `preference_update` | - | 更新阅读偏好 |
| `content_completion` | `ai_summary` | 文章内容补全 |
| `firecrawl` | - | Firecrawl 全文抓取 |
| `daily_report` | - | 生成每日叙事摘要 |
| `tag_quality_score` | - | 重算标签质量分数 |
| `log_cleanup` | - | 清理过期的 AI 调用日志和追踪数据 |
| `aux_label_cleanup` | - | 清理无活跃标签引用的辅助标签 |
| `blocked_article_recovery` | - | 恢复被阻塞的文章 |

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

除 `data`（调度器状态数组）外，顶层附全局状态字段：

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `analysis_paused` | bool | **用户意图**的暂停态（`UserPaused`，持久化标志）。只反映用户开关，**不受模型健康影响**——健康未就绪不会让该字段/前端暂停按钮翻转 |
| `analysis_paused_at` | string | 用户暂停时间（RFC3339），未暂停为 `null` |
| `ai_healthy` | bool | AI 模型层是否健康（后端启动探活后生效，见 `flow/scheduler.md` §业务约束 #7 健康门） |
| `ai_health_routes` | array | 各路由主 provider 简明通断明细：`[{route_name, capability, primary_provider, reachable}]`，供前端渲染未就绪 banner |

**按调度器的额外字段**（add-radar-pipeline-status）：

| 调度器 | 字段 | 说明 |
| --- | --- | --- |
| `daily_report` | `schedule_time` | 每日常规生成时刻（`HH:MM`，默认 `21:00`） |
| `daily_report` | `schedule_deadline` | 兜底强制生成时刻（`HH:MM`，默认 `23:30`）；配置值早于 `schedule_time` 时回落默认。两个时刻同一次读取产出，不会出现非法组合。旧后端不返回时前端降级到默认 `23:30` |

> 何时出报看的是**双队列清空**（`tag_jobs` 与 `embedding_queues` 均无活跃量），`schedule_time` 只是最早可生成时刻；未清空时等到 `schedule_deadline` 强制出报。

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

### PUT /api/schedulers/:name/schedule-time

为支持「固定时间点执行」的调度器设置每日触发时间（目前仅 `daily_report`、`board_upgrade_suggest` 支持，其它调度器返回 `400`）。

```json
{ "time": "08:30" }
```

`time`：`HH:MM` 格式（24 小时制，必填）。返回 `{"success": true}`。
