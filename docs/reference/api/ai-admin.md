# AI 管理 Admin

> **2026-10 ai-provider-model-overhaul**：AI 配置面从 provider 单层切换为 vendor/model 两层——旧 `/api/ai/providers*` 与 `POST /api/ai/test` 端点**已删除**（单用户产品，前后端同 change 原子切换，无兼容窗口）；`GET /api/ai/health` 字段升级（`primary_provider` → `primary_model` + `vendor`）；新增降级观测查询端点。

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/ai/settings` | 获取 AI 设置（summary 主 vendor/模型 + 通用键值） |
| POST | `/api/ai/settings` | 保存 AI 设置（当前仅 `daily_report_time`） |
| GET | `/api/ai/vendors` | 列出供应商（含模型数/能力扇出数） |
| POST | `/api/ai/vendors` | 新增供应商（保存后自动触发一次模型清单刷新） |
| GET | `/api/ai/vendors/:id` | 供应商详情（含模型清单） |
| PUT | `/api/ai/vendors/:id` | 更新供应商（含 `clear_api_key` / `clear_start_command`） |
| DELETE | `/api/ai/vendors/:id` | 删除供应商（单事务级联：解绑→删模型→删 vendor） |
| POST | `/api/ai/vendors/:id/models/refresh` | 刷新该供应商模型清单（GET /models 对账，diff 返回） |
| POST | `/api/ai/vendors/:id/test` | 测试已保存供应商连通性（GET /models 探测） |
| POST | `/api/ai/vendors/test` | 内联测试未保存表单（不走库） |
| GET | `/api/ai/models` | 全部模型（含 vendor 摘要与 usable 布尔，供绑定候选） |
| PUT | `/api/ai/models/:id` | 更新模型配置（enabled/kind/thinking_level/supports_xhigh_max） |
| GET | `/api/ai/routes` | 列出路由（含模型绑定链） |
| PUT | `/api/ai/routes/:capability` | 保存路由模型绑定链 |
| GET | `/api/ai/health` | 获取 AI 模型健康快照（primary_model + vendor） |
| PUT | `/api/ai/health/auto-start-models` | 更新本地模型自动拉起总开关 |
| POST | `/api/ai/health/reprobe` | 手动触发一次异步健康重探（幂等） |
| GET | `/api/ai/observability/degradations` | 降级观测统计 + 事件流（纯读 ai_call_logs） |

调用日志与会话查询（`GET /api/ai/call-logs`、`GET /api/ai/sessions/:session_id`）见 [ai-call-logs.md](ai-call-logs.md)。

---

### GET /api/ai/settings

从 AI Router 解析 summary 能力主模型及其 vendor，响应字段名沿用 `provider_id`/`provider_name`（前端 §4 重写前的最小兼容），值填 vendor/模型。

```json
{
  "success": true,
  "data": {
    "provider_id": 1,
    "provider_name": "deepseek",
    "base_url": "https://api.deepseek.com/v1",
    "model": "deepseek-chat",
    "api_key_configured": true,
    "time_range": 180
  }
}
```

解析失败时缺省 vendor/model 字段；其余键值（`ai_settings` 表，排除 `summary_config`/`firecrawl_config`/`bocha_config`）原样平铺。

### POST /api/ai/settings

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `daily_report_time` | string | 否 | 日报生成时间 |

仅接受 `daily_report_time`（非字符串返回 `400`）；其余字段忽略。

### GET /api/ai/vendors

供应商列表（enabled 优先、按名称排序）。每项含 `id`, `name`, `preset_id`, `provider_type`, `base_url`, `timeout_seconds`, `thinking_dialect`, `structured_output`, `enabled`, `api_key_configured`（bool）, `start_command_configured`（bool），另附扇出统计：`models_count`（该 vendor 模型数）、`capabilities_count`（其模型被绑定的去重 capability 数，前端扇出徽章数据源）。**密钥与启动命令原文不出库**，只回 configured 布尔。

### POST /api/ai/vendors

新增供应商（`name` 全局唯一，重名返回 `400`）：

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `name` | string | 是 | 名称 |
| `base_url` | string | 是 | 服务地址 |
| `preset_id` | string | 否 | 预设标识（deepseek/zai/…），自定义留空 |
| `provider_type` | string | 否 | `openai_compatible`（默认）/ `ollama` |
| `api_key` | string | 否 | API Key（本地无认证服务可留空） |
| `timeout_seconds` | int | 否 | 超时（默认 120） |
| `start_command` | string | 否 | 本地模型进程启动命令；空=外部托管 |
| `thinking_dialect` | string | 否 | 思考方言：`qwen-chat-template` / `deepseek` / `reasoning-effort` / `none`（默认 `none`） |
| `structured_output` | string | 否 | 结构化输出档位：`json_schema`（默认）/ `json_object` / `none` |
| `enabled` | bool | 否 | 默认 `true` |

返回 `{"success": true, "data": {"id": ..., "models_refreshed": bool, "added": N}}`。保存成功后**自动触发一次模型清单刷新**（D7）：刷新失败不回滚创建——仍 `success=true`，带 `models_refreshed:false` + `refresh_error`。

### GET /api/ai/vendors/:id

供应商详情（字段同列表项）+ `models` 数组（按名称排序，每项含 `id`, `vendor_id`, `name`, `model_kind`, `thinking_level`（null=未配置）, `supports_xhigh_max`, `enabled`, `status`（`ok`/`stale`）, `last_seen_at`）。不存在返回 `404`。

### PUT /api/ai/vendors/:id

按 id 全量覆盖更新（name/provider_type/base_url/timeout_seconds/thinking_dialect/structured_output/enabled）。密钥与启动命令特殊语义（原文不回显、前端无法回传）：

- `api_key`：非空串→更新；空串且未传 `clear_api_key`→**沿用已存值**（不丢 key）；`clear_api_key: true`→显式清空。
- `start_command`：同规则（`clear_start_command: true` 显式清空）。

改名撞其他 vendor 返回 `400`；不存在返回 `404`。

### DELETE /api/ai/vendors/:id

单事务级联删除：解绑 `ai_route_models` → 删 `ai_models` → 删 vendor，**不因仍被路由引用而拒绝**（被摘空的线路保留）。响应告知级联计数：

```json
{
  "success": true,
  "message": "已删除 2 个模型、解除 3 条绑定",
  "data": { "deleted_models": 2, "detached_bindings": 3 }
}
```

### POST /api/ai/vendors/:id/models/refresh

按 vendor 触发模型清单刷新（`GET {base_url}/models` 对账）：上游新出现的模型 INSERT（**默认未启用**、kind=llm）；已可见的更新 `last_seen_at`（stale 复活为 ok）；库内有而清单无的标 `status=stale` **不删**（路由绑定保留）。

```json
{
  "success": true,
  "data": { "added": ["qwen3-32b"], "stale": ["qwen2-72b"], "visible_count": 5 }
}
```

刷新失败（网络/超时/非 2xx/vendor 不存在）返回 `502`，**库内清单保持刷新前状态**（零改动）。

### POST /api/ai/vendors/:id/test

测试已保存 vendor 连通性（`GET {base_url}/models` 零 token 探测，api_key 为空不带 Authorization 头）。不存在返回 `404`。

- 成功：`{"success": true, "message": "连接成功，共 N 个可用模型", "data": {"models": ["id", ...]}}`
- 失败：HTTP 200 + `{"success": false, "error": "..."}`（测试失败是业务结果而非服务端错误，前端 toast 展示）

### POST /api/ai/vendors/test

内联测试：两段式弹窗保存前测试未保存表单，不走库、不落库。

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `base_url` | string | 是 | 服务地址（缺失返回 `400`） |
| `api_key` | string | 否 | ollama/本地无认证可省略 |
| `provider_type` | string | 否 | 如 `ollama` |
| `timeout_seconds` | int | 否 | 超时 |

响应语义同 `POST /api/ai/vendors/:id/test`。

### GET /api/ai/models

全部模型（按 vendor_id, name 排序），每项含模型字段 + `vendor` 摘要（`id`, `name`, `provider_type`, `base_url`）+ `usable` 布尔（`enabled && status=="ok" && vendor.enabled`）——**stale/未启用模型不出现在绑定候选**（usable=false）。

### PUT /api/ai/models/:id

按 id 更新模型配置：

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `enabled` | bool | 否 | 是否启用 |
| `model_kind` | string | 否 | `llm`（默认）/ `embedding`，空串归一为 `llm`；非法值 `400` |
| `thinking_level` | string\|null | 否 | 七档 `off..max`；**空串=清除（归一 NULL，继承全局默认 medium）**；embedding 模型配置思考档 `400` |
| `supports_xhigh_max` | bool | 否 | 是否声明支持 xhigh/max（false 时运行时钳制为 high） |

校验（违反返回 `400`，文案透传）：云端 vendor（`hostIsLocal(base_url)=false`）不得配 embedding 模型；`unique(vendor_id, name)` 撞名拒绝。不存在返回 `404`。返回更新后的模型视图。

### GET /api/ai/routes

所有路由及模型绑定链（按 capability, name 排序，绑定按 priority 升序）。每条路由含 `id`, `name`, `capability`, `enabled`, `description`, `max_concurrency`, `route_models`：

```json
{
  "id": 1, "route_id": 3, "model_id": 5, "priority": 1, "enabled": true,
  "thinking_level": null,
  "model": { "id": 5, "name": "qwen3-32b", "model_kind": "llm", "thinking_level": "high", "supports_xhigh_max": false, "enabled": true, "status": "ok" },
  "vendor": { "id": 1, "name": "qwen" }
}
```

`thinking_level: null` = 继承模型默认档；非空 = 场景覆盖档（七档）。

### PUT /api/ai/routes/:capability

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `bindings` | `[{model_id: uint, thinking_level?: string}]` | 是 | 模型绑定链（**全删全建**：顺位=数组顺序，priority 写 1..n；空数组=清空绑定） |
| `name` | string | 否 | 路由名称（空则用 `default`） |
| `enabled` | bool | 否 | 默认 `true` |
| `description` | string | 否 | 描述 |
| `max_concurrency` | int | 否 | 0 = 用各能力默认值 |

`capability` 取值及业务绑定：`summary`（文章自动总结）、`digest_polish`（日报生成）、`topic_tagging`（事件标签提取）、`open_notebook`（页边注问答）、`embedding`（向量嵌入）。

校验在写入前完成（拒绝时不动任何已存绑定），违反返回 `400`（中文文案原样透传）：模型不存在（`ai model not found`）；模型 stale 或未启用（`模型「X」当前不可绑定（stale 或未启用）`）；类型不匹配（embedding 路由不能挂 llm 模型 / llm 路由不能挂 embedding 模型）；embedding 绑定携带思考档（`embedding 绑定不能携带思考档覆盖`）；thinking_level 非法值。

### GET /api/ai/health

返回进程内 AI 模型健康快照（仅存内存不落库）+ 本地模型自动拉起总开关。**探测对象=每条路由首位（priority 最高）可用模型所属 vendor**（`GET {base_url}/models` 零 token 探测）。

```json
{
  "success": true,
  "data": {
    "healthy": true,
    "checked_at": "2026-10-05T10:00:00+08:00",
    "auto_start_models": false,
    "routes": [
      {
        "route_name": "default",
        "capability": "summary",
        "primary_model": "qwen3-32b",
        "vendor": "qwen",
        "model_kind": "llm",
        "reachable": true,
        "launched_by_backend": false,
        "last_checked": "2026-10-05T10:00:00+08:00",
        "error": ""
      }
    ]
  }
}
```

- `healthy`：**核心路由硬条件**——≥1 条 embedding 路由主绑定可达 **且** `topic_tagging` 路由主绑定可达（文章入库前置拦截，fail-closed）；路由缺失/链路全禁用/主绑定不可达一律 `false`；首轮探活未完成（启动竞态期）为 `false`。其余非核心路由（summary / digest_polish / data_enrichment_* / feed_discovery / open_notebook）不参与判定，通断明细照常出现在 `routes`。
- `checked_at`：快照生成时间；未就绪时为 `null`。
- `routes[].primary_model` + `vendor`：取代旧 `primary_provider` 字段（vendor/model 两层化，D7）。
- `launched_by_backend`：本轮是否由后端 `start_command` 拉起；`last_checked` 为 RFC3339 时间；`error` 为不通原因。
- `auto_start_models`：本地模型自动拉起总开关（存 `ai_settings`，默认 `false`）。

### PUT /api/ai/health/auto-start-models

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `enabled` | bool | 是 | 是否在后端启动时自动拉起「探测不通且配了 `start_command`」的本地模型进程 |

返回 `{"success": true, "data": {"enabled": ...}}`，值为持久化后的开关状态。

### POST /api/ai/health/reprobe

手动触发一次异步健康重探（复用启动探测全流程：探测 + 自动拉起 + 45s 轮询 + 冷却互斥），立即返回，不等待探测完成。无 body。

返回 `{"success": true, "data": {"triggered": bool, "skipped": bool}}`：`triggered=true` 表示本次调用实际启动了一次探测；`skipped=true`（= !triggered）表示已有探测 in-flight（幂等跳过，不排队、不并发）。探测完成后 `GET /api/ai/health` 的 `checked_at` 前移、`routes` 更新。

### GET /api/ai/observability/degradations

降级观测（`ai-route-observability`）：对既有 `ai_call_logs` 的**纯读聚合**（不写库、不改 schema；7d 窗口受 7 天日志清理约束）。

| 参数 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `window` | string | 是 | `24h` 或 `7d`，其他值 `400`（`window must be 24h or 7d`） |
| `capability` | string | 否 | 只裁剪 `events`（精确匹配）；`stats` 恒为全窗口口径 |

```json
{
  "success": true,
  "data": {
    "window": "24h",
    "stats": { "calls": 120, "degradations": 4, "failures": 1 },
    "events": [
      {
        "occurred_at": "2026-10-05T03:12:00+08:00",
        "capability": "digest_polish",
        "from": 0,
        "to": 1,
        "model": "deepseek-chat",
        "type": "fallback",
        "code": "http_400"
      }
    ]
  }
}
```

口径：同一 `Router.Chat` 的全部尝试共享 `trace_id`——`calls`=窗口内调用组数（空 trace_id 每行自成组，历史行不丢）；`degradations`=structured+fallback 迁移事件数（**全链失败不计入**）；`failures`=全链失败组数单独统计。事件类型二分：

- `fallback`：供应商失败顺位迁移（from/to=组内 0-based 尝试序号，`code`=失败行错误码）；**from/to 均为 null 表达全链失败**（`model`=最后尝试的模型，`code`=最后错误码）。
- `structured`：结构化输出 400 降档（同 vendor+model 相邻两行 `structured_stage` 不同还原，`code`=降级方向如 `json_schema→json_object`；三档连降产生两条事件）。

`events` 按 `occurred_at` 降序。

---

## 已删除端点（ai-provider-model-overhaul，勿再调用）

| 已删端点 | 替代 |
| ------ | ------ |
| `GET/POST /api/ai/providers`、`PUT/DELETE /api/ai/providers/:provider_id` | `/api/ai/vendors*` + `/api/ai/models*` |
| `POST /api/ai/test` | `POST /api/ai/vendors/:id/test` / `POST /api/ai/vendors/test` |
| `PUT /api/ai/routes/:capability` 的 `provider_ids` 入参 | `bindings: [{model_id, thinking_level?}]` |
