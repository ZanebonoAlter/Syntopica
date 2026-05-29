# AI 管理 Admin

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/ai/settings` | 获取 AI 设置 |
| POST | `/api/ai/settings` | 保存 AI 设置 |
| POST | `/api/ai/summarize` | AI 总结文章 |
| POST | `/api/ai/test` | 测试 AI 连接 |
| GET | `/api/ai/providers` | 列出提供商 |
| POST | `/api/ai/providers` | 创建/更新提供商 |
| PUT | `/api/ai/providers/:provider_id` | 更新指定提供商 |
| DELETE | `/api/ai/providers/:provider_id` | 删除提供商 |
| GET | `/api/ai/routes` | 列出路由 |
| PUT | `/api/ai/routes/:capability` | 更新指定路由 |

---

### GET /api/ai/settings

优先从 AI Router 获取当前 summary 能力的主 Provider 和 Route，回退到 legacy 配置。

```json
{
  "success": true,
  "data": {
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o-mini",
    "provider_id": 1,
    "provider_name": "OpenAI",
    "route_name": "default",
    "time_range": 180,
    "api_key_configured": true
  }
}
```

无配置时 `data` 为 `null`。

### POST /api/ai/settings

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `api_key` | string | 是 | API Key |
| `base_url` | string | 否 | 默认 `https://api.openai.com/v1` |
| `model` | string | 否 | 默认 `gpt-4o-mini` |

同时更新 legacy 配置和 AI Provider/Route，并热更新 content completion 的 AI 凭据。

### POST /api/ai/summarize

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 是 | 文章标题 |
| `content` | string | 是 | 文章内容 |
| `base_url` | string | 否 | 覆盖服务地址 |
| `api_key` | string | 否 | 覆盖 API Key |
| `model` | string | 否 | 覆盖模型名 |
| `language` | string | 否 | 默认 `zh` |

若 `base_url`/`api_key`/`model` 均提供则直接调用，否则走 AI Router。

### POST /api/ai/test

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `base_url` | string | 是 | 服务地址 |
| `model` | string | 是 | 模型名 |
| `api_key` | string | 否 | ollama 可省略 |
| `provider_type` | string | 否 | 如 `ollama` |

### GET /api/ai/providers

提供商列表，含 `id`, `name`, `provider_type`, `base_url`, `model`, `enabled`, `timeout_seconds`, `max_tokens`, `temperature`, `metadata`, `api_key_configured` 等。

### POST /api/ai/providers

按 name 匹配创建/更新：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `base_url` | string | 是 | 服务地址 |
| `model` | string | 是 | 模型名 |
| `api_key` | string | 否 | API Key |
| `provider_type` | string | 否 | 类型 |
| `enabled` | bool | 否 | 默认 `true` |
| `timeout_seconds` | int | 否 | 超时 |
| `max_tokens` | int* | 否 | 最大 tokens |
| `temperature` | float64* | 否 | 温度 |
| `metadata` | string | 否 | 附加元数据 |

返回 `{"success": true, "data": {"id": ...}}`。

### PUT /api/ai/providers/:provider_id

同上。`api_key` 仅非空时更新。

### DELETE /api/ai/providers/:provider_id

仍被路由引用时返回 `409`。

### GET /api/ai/routes

所有路由及关联提供商。每条路由包含 `id`, `name`, `capability`, `enabled`, `strategy`, `description`, `route_providers`（含 provider 详情和优先级）。

### PUT /api/ai/routes/:capability

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `provider_ids` | uint[] | 是 | 关联提供商 ID |
| `name` | string | 否 | 路由名称（空则用默认） |
| `enabled` | bool | 否 | 默认 `true` |
| `description` | string | 否 | 描述 |

`capability` 如 `summary`, `article_completion`。
