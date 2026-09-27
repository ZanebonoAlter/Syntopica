# 订阅 Feeds

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/feeds` | 获取订阅列表 |
| GET | `/api/feeds/:feed_id` | 获取单个订阅 |
| GET | `/api/feeds/board-hit-stats` | 源 × 板块命中统计（只读聚合） |
| POST | `/api/feeds` | 创建订阅 |
| PUT | `/api/feeds/:feed_id` | 更新订阅 |
| DELETE | `/api/feeds/:feed_id` | 删除订阅 |
| POST | `/api/feeds/:feed_id/refresh` | 刷新单个订阅 |
| POST | `/api/feeds/fetch` | 预览 Feed URL |
| POST | `/api/feeds/refresh-all` | 刷新所有订阅 |

---

### GET /api/feeds

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `page` | int | 1 | 页码 |
| `per_page` | int | 20 | 每页条数，≥10000 返回全部 |
| `category_id` | int | - | 按分类过滤 |
| `uncategorized` | string | - | `true` 查未分类 |

返回带分页的订阅列表，含 `article_count` 和 `unread_count`。

### GET /api/feeds/:feed_id

单个订阅详情，含文章统计。

### GET /api/feeds/board-hit-stats

源视角只读聚合：窗口内每个订阅源的文章三分解（入板块 / 有标签未入板块 / 未打标两分）与板块分布。全量源一次批量返回（不逐源 N+1）；**只读**：不写库、不触发打标或匹配。

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `window` | int | 7 | 统计窗口天数，白名单 `7`/`30`/`90`；非法值（如 `14`/`0`/`abc`）返回 `400`，**不回退默认值** |

返回 `{ "success": true, "data": { "items": [...] } }`，`items` 每条：

| 字段 | 类型 | 说明 |
|------|------|------|
| `feed_id` | uint | 订阅源 ID |
| `title` | string | 订阅源名 |
| `tagging_enabled` | bool | 是否开启打标（关闭的源照常返回，不特殊处理） |
| `articles` | int | 窗口内文章总数 |
| `in_board` | int | 命中文章数（去重后） |
| `tagged_no_board` | int | 有标签但未命中板块 |
| `untagged_pending` | int | 无标签且打标排队中（存在 pending/leased 任务） |
| `untagged_settled` | int | 无标签且已处理完（completed/failed/从未入队） |
| `hit_rate` | float | `in_board / articles`，`articles=0` 时为 0 |
| `boards` | array | 命中板块分布 `[{board_id, label, articles}]`，空数组非 null |

口径三条硬约束（唯一权威 `openspec/specs/source-board-hit-rate/spec.md`，唯一实现 `internal/tagmanagement/service/sourcestats/`）：

1. **按文章去重**：所有计数以文章为单位去重（`COUNT(DISTINCT id)`，布尔位用 `EXISTS` 不 JOIN），不因一篇文章多标签/多板块重复计数；
2. **含已归档文章**：窗口内 `archived=true` 的文章计入，**不过滤归档位**（高频源超 `max_articles` 被归档的文章往往是命中主力，排除会得出相反结论）；
3. **限窗口**：按 `coalesce(pub_date, created_at) >= now - window 天` 界定，不做全量统计（老文章打标覆盖低，全量会把所有老源冤枉成杂音源）。

窗口内 0 篇的源照常返回（`articles=0`、`hit_rate=0`、`boards=[]`）。恒等式：每源 `articles == in_board + tagged_no_board + untagged_pending + untagged_settled`；`boards` 各项之和可大于 `in_board`（同一文章命中多板块各计一次）。

### POST /api/feeds

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `url` | string | 是 | RSS feed URL |
| `title` | string | 否 | 默认 `Untitled Feed` |
| `description` | string | 否 | 描述 |
| `category_id` | uint* | 否 | 分类 ID |
| `icon` | string | 否 | 图标值（iconify id、图片 URL，或系统本地化后的 `/icons/feeds/<id>.<ext>` 同源路径——由 RefreshFeed 自动管理，不建议手填）。传入非空值时 `icon_source` 置为 `custom`；不传则默认 `mdi:rss` + `icon_source=fallback` |
| `color` | string | 否 | 默认 `#8b5cf6` |
| `max_articles` | int | 否 | 默认 `100` |
| `refresh_interval` | int | 否 | 刷新间隔（分钟），默认 `60` |
| `ai_summary_enabled` | bool | 否 | 启用 AI 总结 |
| `article_summary_enabled` | bool | 否 | 启用文章级总结 |
| `completion_on_refresh` | bool | 否 | 刷新时自动补全 |
| `max_completion_retries` | int | 否 | 补全最大重试次数 |
| `firecrawl_enabled` | bool | 否 | 启用 Firecrawl |

`201`：返回创建的订阅。`409`：URL 已存在。

### PUT /api/feeds/:feed_id

只更新请求体中明确提供的字段。布尔字段需显式包含才生效。

传入非空 `icon` 时，`icon_source` 自动置为 `custom`（用户主权图标，后续 RefreshFeed 不会覆盖）。

### DELETE /api/feeds/:feed_id

删除订阅及其关联文章；本地化图标文件（`data/icons/feeds/<feed_id>.*`，若存在）一并清理，清理失败不阻断删除。

### GET /icons/*

静态路由，直接服务 `storage.icon_dir` 目录（默认 `data/icons`），提供本地化后的 feed 图标（如 `/icons/feeds/42.png`）。独立于前端产物托管，dev 与生产模式均可用；文件不存在返回 `404`。

### POST /api/feeds/:feed_id/refresh

后台刷新，`202 Accepted`：

```json
{ "success": true, "message": "Started refreshing feed in background" }
```

### POST /api/feeds/fetch

预览 RSS URL：

```json
{ "url": "https://example.com/feed.xml" }
```

返回 `{ "title": "...", "description": "..." }`。

### POST /api/feeds/refresh-all

`202`：

```json
{
  "success": true,
  "message": "Started refreshing all feeds in background",
  "data": { "total_feeds": 15 }
}
```
