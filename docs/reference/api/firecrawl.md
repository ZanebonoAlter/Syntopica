# Firecrawl

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/firecrawl/article/:id` | 抓取单篇全文 |
| POST | `/api/firecrawl/feed/:id/enable` | 启用/禁用 |
| GET | `/api/firecrawl/status` | 全局状态 |
| POST | `/api/firecrawl/settings` | 保存设置 |

---

### POST /api/firecrawl/article/:id

需要订阅源已启用 Firecrawl 且全局已开启。

```json
{
  "success": true,
  "data": {
    "firecrawl_content": "# markdown content...",
    "firecrawl_status": "completed",
    "summary_status": "incomplete"
  }
}
```

### POST /api/firecrawl/feed/:id/enable

```json
{ "enabled": true }
```

启用时将该订阅源下未抓取的文章标记为 `pending`。返回 `{ "success": true, "data": { "firecrawl_enabled": true } }`。

### GET /api/firecrawl/status

```json
{
  "success": true,
  "data": {
    "enabled": true,
    "api_url": "https://api.firecrawl.dev/v0",
    "mode": "scrape",
    "timeout": 60,
    "max_content_length": 50000,
    "api_key_configured": true
  }
}
```

### POST /api/firecrawl/settings

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `enabled` | bool | 是 | 是否启用 |
| `api_url` | string | 是 | API 地址 |
| `api_key` | string | 否 | 留空保留现有 |
| `mode` | string | 否 | 默认 `scrape` |
| `timeout` | int | 否 | 默认 `60` |
| `max_content_length` | int | 否 | 默认 `50000` |

保存成功后返回更新后的配置（含 `api_key_configured`）。
