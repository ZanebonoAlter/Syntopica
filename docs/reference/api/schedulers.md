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
