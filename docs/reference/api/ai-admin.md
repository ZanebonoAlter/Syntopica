# AI 管理 Admin

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/ai/settings` | 获取 AI 设置 |
| POST | `/api/ai/settings` | 保存 AI 设置 |
| POST | `/api/ai/test` | 测试 AI 连接 |
| GET | `/api/ai/providers` | 列出提供商 |
| POST | `/api/ai/providers` | 创建/更新提供商 |
| PUT | `/api/ai/providers/:provider_id` | 更新指定提供商 |
| DELETE | `/api/ai/providers/:provider_id` | 删除提供商 |
| GET | `/api/ai/health` | 获取 AI 模型健康快照 |
| PUT | `/api/ai/health/auto-start-models` | 更新本地模型自动拉起总开关 |
| POST | `/api/ai/health/reprobe` | 手动触发一次异步健康重探（幂等） |
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
| ------ | ------ | ------ | ------ |
| `api_key` | string | 否 | API Key（本地无认证服务可留空） |
| `base_url` | string | 否 | 默认 `https://api.openai.com/v1` |
| `model` | string | 否 | 默认 `gpt-4o-mini` |

同时更新 legacy 配置和 AI Provider/Route，并热更新 content completion 的 AI 凭据。

### POST /api/ai/test

两种取参方式：内联凭据（未保存的表单，如主模型编辑）或按已保存 provider 测试（备用模型卡片，密钥不出库）：

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `provider_id` | int | 否 | 传了则后端按 id 加载已保存配置（含库内密钥），忽略内联字段；不存在返回 `404` |
| `base_url` | string | 是* | 服务地址（*`provider_id` 路径下不需要） |
| `model` | string | 是* | 模型名（*同上） |
| `api_key` | string | 否 | ollama 可省略 |
| `provider_type` | string | 否 | 如 `ollama` |

### GET /api/ai/providers

提供商列表，含 `id`, `name`, `provider_type`, `base_url`, `model`, `enabled`, `timeout_seconds`, `max_tokens`, `temperature`, `metadata`, `model_kind`（`llm`/`embedding`），`start_command_configured`（bool，是否配了本地启动命令），`api_key_configured` 等。

### POST /api/ai/providers

创建提供商（`name` 全库唯一，重名返回 `400`，不覆已存在记录）：

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `name` | string | 是 | 名称 |
| `base_url` | string | 是 | 服务地址 |
| `model` | string | 是 | 模型名 |
| `api_key` | string | 否 | API Key |
| `provider_type` | string | 否 | 类型 |
| `enabled` | bool | 否 | 默认 `true` |
| `timeout_seconds` | int | 否 | 超时 |
| `max_tokens` | int* | 否 | 最大 tokens |
| `temperature` | float64* | 否 | 温度 |
| `enable_thinking` | bool | 否 | 是否开启模型推理思考（透传 `chat_template_kwargs.enable_thinking=true` 到请求体），默认 `false`。用于差异化控制：同一台本地模型（如 Qwythos）可配两条 provider——一条 `true` 挂 `digest_polish`（日报思考）、一条 `false` 挂 `topic_tagging`（打标签不思考） |
| `model_kind` | string | 否 | 模型类型：`llm`（默认）/ `embedding`。与 `provider_type`（协议维度）正交；embedding 路由只接 `embedding` provider，llm 路由只接 `llm` provider |
| `start_command` | string | 否 | 本地模型进程启动命令（如 `llama-server -m D:/models/qwen.gguf --port 8081`）。配了它 + 总开关 `auto_start_models` 开时，后端启动健康检测可自动拉起该进程；空 = 外部托管服务 |
| `clear_start_command` | bool | 否 | 显式清空已保存的 `start_command`（PUT 用） |
| `metadata` | string | 否 | 附加元数据 |

返回 `{"success": true, "data": {"id": ...}}`。

### PUT /api/ai/providers/:provider_id

按 id 更新，改名安全（改名只改本行，不新增记录）；新名字撞其他 provider 返回 `400`。`api_key` 仅非空时更新；传 `clear_api_key: true` 可显式清空已保存的密钥。`start_command` 同样仅非空时更新；传 `clear_start_command: true` 可显式清空已保存的启动命令。`model_kind` 可省略，空值归一化为 `llm`（PUT 不传等同重置为 `llm`，不沿用现值）。

### DELETE /api/ai/providers/:provider_id

仍被路由引用时返回 `409`。

### GET /api/ai/health

返回进程内 AI 模型健康快照（后端启动探活后填充，仅存内存不落库）+ 本地模型自动拉起总开关。

```json
{
  "success": true,
  "data": {
    "healthy": true,
    "checked_at": "2026-08-02T10:00:00+08:00",
    "auto_start_models": false,
    "routes": [
      {
        "route_name": "default",
        "capability": "summary",
        "primary_provider": "OpenAI",
        "model_kind": "llm",
        "reachable": true,
        "launched_by_backend": false,
        "last_checked": "2026-08-02T10:00:00+08:00",
        "error": ""
      }
    ]
  }
}
```

- `healthy`：**核心路由硬条件**——≥1 条 embedding 路由主 provider 可达 **且** `topic_tagging` 路由主 provider 可达（ai-health-gate-tagging-required，语义为「文章入库前置拦截」）；两条任一不满足为 `false`，`topic_tagging` 路由缺失/未绑定启用 provider/主 provider 不可达一律 `false`（fail-closed）；其余非核心路由（summary / digest_polish / data_enrichment_* / feed_discovery / open_notebook）不参与判定，其通断明细仍照常出现在 `routes`。首轮探活未完成（启动竞态期）为 `false`（fail-closed）。
- `checked_at`：快照生成时间；未就绪时为 `null`。
- `routes`：每条 enabled 且有启用 provider 的路由的主 provider（priority 最高）通断明细；`reachable` 为探活结果，`launched_by_backend` 表示本轮是否由后端 `start_command` 拉起，`last_checked` 为 RFC3339 时间，`error` 为不通原因。
- `auto_start_models`：本地模型自动拉起总开关（存 `ai_settings`，默认 `false`）。

### PUT /api/ai/health/auto-start-models

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `enabled` | bool | 是 | 是否在后端启动时自动拉起「探测不通且配了 `start_command`」的本地模型进程 |

返回 `{"success": true, "data": {"enabled": ...}}`，值为持久化后的开关状态。

### POST /api/ai/health/reprobe

手动触发一次异步健康重探（复用启动探测全流程：探测 + 自动拉起 + 45s 轮询 + 冷却互斥），立即返回，不等待探测完成。无 body。

返回 `{"success": true, "data": {"triggered": bool, "skipped": bool}}`：`triggered=true` 表示本次调用实际启动了一次探测；`skipped=true`（= !triggered）表示已有探测 in-flight（幂等跳过，不排队、不并发）。探测完成后 `GET /api/ai/health` 的 `checked_at` 前移、`routes` 更新；前端「重新检测」按钮会轮询等待该结果。

### GET /api/ai/routes

所有路由及关联提供商。每条路由包含 `id`, `name`, `capability`, `enabled`, `strategy`, `description`, `route_providers`（含 provider 详情和优先级）。

### PUT /api/ai/routes/:capability

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `provider_ids` | uint[] | 是 | 关联提供商 ID |
| `name` | string | 否 | 路由名称（空则用默认） |
| `enabled` | bool | 否 | 默认 `true` |
| `description` | string | 否 | 描述 |

`capability` 取值及业务绑定：`summary`（文章自动总结）、`digest_polish`（日报生成）、`topic_tagging`（事件标签提取）、`embedding`（向量嵌入）。`article_completion` 已废弃，前端面板不再显示。
