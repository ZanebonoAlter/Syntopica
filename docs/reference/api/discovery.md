# 订阅源发现（Feed Discovery）

> 偏好向量画像 × RSSHub 路由目录 → 向量粗筛 + LLM 精排 → 推荐卡片状态机 + 问答冷启动。链路与业务约束见 [../flow/discovery.md](../flow/discovery.md)。

## RSSHub 路由目录

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/discovery/catalog/sync` | 触发 RSSHub 实例 `/api/namespace` 全量同步（content_hash diff + 参数标记 + gone） |
| GET | `/api/discovery/catalog/status` | 目录状态（路由总数 / 各 status 计数 / 最近同步时间） |

### POST /api/discovery/catalog/sync

后台拉取自建 RSSHub 实例全量路由元数据入库。返回同步摘要（新增/变更/消失计数）。

```json
{ "success": true, "data": { "total": 3245, "added": 18, "updated": 2, "gone": 0 } }
```

> 实例地址读 `ai_settings.rsshub_config.rsshub_base_url`（缺省回落 `http://rsshub.app`），见 [settings/rsshub](#rsshub-实例配置)。

### GET /api/discovery/catalog/status

```json
{
  "success": true,
  "data": { "total_routes": 3245, "ok": 2100, "broken": 32, "unknown": 1113, "last_synced_at": "2026-07-25T03:00:00Z" }
}
```

## 订阅源推荐

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/discovery/recommendations` | 推荐卡片列表（默认 `status=pending`；`scope=history` 历史聚合视图） |
| POST | `/api/discovery/recommendations/refresh` | **异步受理刷新 run**：秒回 `{run_id,status}`（占位计数字段恒 0，产出经 runs/:id 轮询） |
| POST | `/api/discovery/recommendations/:id/accept` | 接受推荐 → 订阅落地 |
| POST | `/api/discovery/recommendations/:id/dismiss` | 拒绝（冷却默认 30 天，跨 source） |
| POST | `/api/discovery/recommendations/:id/restore` | 恢复长期排除（仅恢复推荐资格，不自动订阅） |
| GET | `/api/discovery/runs/:id` | run 详情/轮询（status=running→succeeded/failed；succeeded 附 items 精排选中列表） |

### GET /api/discovery/recommendations

返回推荐卡片（含路由元数据 + 相似度 score + 匹配版块 + 参数说明 + `param_options` 参数可选值字典）。`param_options` 按参数名分组，无字典数据时为 `{}`（向后兼容）；每项 `source` ∈ {`manual`, `scraped`}，永不出现 `llm`（见 [../flow/discovery.md](../flow/discovery.md) §参数可选值字典）。

```json
{
  "success": true,
  "data": [
    {
      "id": "57",
      "route_id": "2103",
      "route_namespace": "36kr",
      "route_path": "newsflashes",
      "route_name": "36氪 快讯",
      "route_example": "36kr/newsflashes",
      "usable_directly": true,
      "requires_parameters": false,
      "parameters": "",
      "param_options": {},
      "route_status": "ok",
      "board_id": "12",
      "board_label": "AI 芯片",
      "score": 0.78,
      "llm_reason": "覆盖半导体与 AI 芯片产业快讯，与你「芯片/制程」兴趣高度匹配。",
      "status": "pending"
    }
  ]
}
```

### POST /api/discovery/recommendations/refresh

手动刷新一轮推荐（每版块 top-N 粗筛 → LLM 精排 → 幂等落 pending）。返回本轮摘要。

```json
{ "success": true, "data": { "generated": 12, "skipped": 3 } }
```

### POST /api/discovery/recommendations/:id/accept

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `category_id` | uint | 否 | 订阅到分类 |
| `parameters` | map[string]string | 否 | `requires_parameters` 路由必填（参数 → 值） |

- `usable_directly`：直接 `CreateFeed`（一键订阅）。
- `requires_parameters`：用 `parameters` 走 `POST /feeds/fetch` 验证通过才 `CreateFeed`。
- 成功置 `status=accepted` + 记录 `accepted_feed_id`，返回创建的 feed。

```json
{ "success": true, "data": { /* feed 对象 */ }, "message": "feed created" }
```

### POST /api/discovery/recommendations/:id/dismiss

拒绝推荐，进入冷却期（默认 30 天，跨 source 生效——同 hash 的 qa/手动刷新重出都会被拦截）。

```json
{ "success": true, "message": "dismissed" }
```

## 问答发现

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/discovery/ask` | 自然语言提问 → 即时粗筛 + 精排推荐 + 种子偏好写入 |

### POST /api/discovery/ask

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `question` | string | 是 | 自然语言兴趣表达（如「我想看 AI 芯片相关资讯」） |

**v2 异步契约（同 refresh）**：受理秒回 `{run_id, status: "running"}`，查询 run 后台推进（embedding → 版块匹配 → 双路召回 → 精排 → 原子发布）；产出经 `GET /api/discovery/runs/:id` 轮询获取（`items` 附每条候选的 name/description/reason/recall_origins/availability），**不再直接返回推荐卡片**。成功查询形成独立兴趣记录（`discovery_interest_entries` 逐条独立，不合成平均画像）。

```json
{ "success": true, "data": { "run_id": 7, "status": "running" } }
```

> 问答推荐候选与刷新共享幂等池（同一发布事务）；召回无依据（无行为画像/版块向量/种子）时合法零选择，run 仍 succeeded 且 items=[]。见 [../flow/discovery.md](../flow/discovery.md) §业务约束 16-18。

## 发现 v2：兴趣记录与候选源库（improve-discovery-recommendations）

### 兴趣记录

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/discovery/interests` | 逐条问答兴趣列表（query_text/board_label/status/created_at） |

### 候选源库（feed-candidate-catalog；入库 ≠ 订阅）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/discovery/candidates` | 分页列表（默认 30/页上限 100；`q`≤500 rune 命中 name/description/url，`kind`，`recommendation_enabled`） |
| POST | `/api/discovery/candidates` | 手动新增（仅入库不订阅；地址字段 wire 名 `feed_url`；重复地址 409 附已有条目） |
| GET | `/api/discovery/candidates/:id` | 单条详情（rsshub 附上游 Route 原始资料；订阅弹窗数据源） |
| PATCH | `/api/discovery/candidates/:id` | 编辑人工字段 / 原生 RSS 地址 / 推荐启停（不触订阅） |
| GET | `/api/discovery/candidates/export` | 导出（默认安全脱敏：私有地址与凭据不导出） |
| POST | `/api/discovery/candidates/import/preview` | 导入预览（文件级问题 400 就地展示；revision 冲突 409 stale_preview） |
| POST | `/api/discovery/candidates/import/confirm` | 导入确认（回传预览凭据 + 文件本体；不触发订阅） |

候选响应的有效展示字段（name/description/language/region）经 `EffectiveMetadata` 计算（人工非空覆盖上游；出口统一清洗 markdown 格式噪音），另有 `subscribed`（feeds.url 精确匹配只读判断）、`availability`（unknown/ok/broken/requires_parameters，无记录=unknown 明示未验证）。上游路由消失（`route.status=gone`）仅禁止再次订阅，条目与已有订阅保留。

### 目录同步

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/discovery/catalog/sync` | 同步 RSSHub 上游目录 → 候选联动（新建/关联候选；人工元数据不被覆盖） |
| GET | `/api/discovery/catalog/status` | 目录状态（路由总数 / 各 status 计数 / 最近同步时间） |

### 后台任务（ai_settings.discovery_v2 开关，fail-open）

`candidate_availability_check`（周期检查候选实际端点可用性，维护类）／`candidate_embedding_backfill`（有效文本指纹增量重嵌，默认 20 条/批每小时，分析类遵守 analysis_paused）／`discovery_run_maintenance`（僵尸 run 置 failed，维护类）。开关关闭 = 三 job 良性跳过 + 检查/回补入口返 503；推荐主链不受影响。见 [../flow/discovery.md](../flow/discovery.md) §业务约束 24。

## RSSHub 实例配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/settings/rsshub` | 读取 RSSHub 实例配置 |
| POST | `/api/settings/rsshub` | 保存 RSSHub 实例配置 |

### GET /api/settings/rsshub

```json
{ "success": true, "data": { "rsshub_base_url": "http://rsshub.app", "rsshub_doc_base": "https://docs.rsshub.app", "rsshub_doc_base_default": "https://docs.rsshub.app" } }
```

### POST /api/settings/rsshub

```json
{ "rsshub_base_url": "http://rsshub.app", "rsshub_doc_base": "https://docs.rsshub.app" }
```

`rsshub_doc_base` 可选，缺省回落默认 `https://docs.rsshub.app`；提供非空值则写入 `ai_settings.rsshub_doc_base`（仅改 `rsshub_base_url` 时不影响 doc_base）。

> 实例地址存 `ai_settings.rsshub_config`，缺省回落 `DefaultRSSHubBaseURL=http://rsshub.app`。改一处全链路（目录同步 + 推荐订阅落地）生效。
>
> `rsshub_doc_base` 存 `ai_settings.rsshub_doc_base`（官方文档基址，用于推荐卡片「官方文档」链接生成）。缺省 `https://docs.rsshub.app`；官方文档站国内可能访问受限，可配换镜像。

## 路由参数字典（admin）

需填参路由的参数可选值字典，注入 recommendation 响应 `param_options`。链路与铁律见 [../flow/discovery.md](../flow/discovery.md) §参数可选值字典。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/route-param-options` | 字典列表（可选 `?route_id=N` 过滤） |
| POST | `/api/admin/route-param-options` | 新建字典条目 |
| PUT | `/api/admin/route-param-options/:id` | 更新（value/label/source） |
| DELETE | `/api/admin/route-param-options/:id` | 删除 |

字典条目字段：`route_id` + `param_name` + `value` + `label` + `source`（`manual`/`scraped`，**拒 `llm`**），UNIQUE(route_id, param_name, value)。
