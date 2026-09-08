# SemanticBoard 与辅助标签 API

基础地址：`http://localhost:5000/api`

通用响应：成功为 `{"success": true, "data": ...}`；失败为 `{"success": false, "error": "..."}`。

## 端点汇总

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/semantic-boards` | 查询 active 版块列表 |
| GET | `/semantic-boards/:id` | 查询单个版块 |
| POST | `/semantic-boards` | 创建版块 |
| PUT | `/semantic-boards/:id` | 更新版块 |
| DELETE | `/semantic-boards/:id` | 软删除版块（status→disabled） |
| GET | `/semantic-boards/:id/composition` | 查看版块构成辅助标签 |
| POST | `/semantic-boards/:id/composition` | 新增 composition 辅助标签 |
| DELETE | `/semantic-boards/:id/composition/:auxiliary_label_id` | 移除 composition 辅助标签 |
| GET | `/semantic-boards/:id/articles` | 查询版块下的文章 |
| GET | `/semantic-boards/:id/suggest-auxiliaries` | 版块级辅助标签建议 |
| GET | `/semantic-boards/suggest-auxiliaries` | 全局辅助标签建议 |
| GET | `/semantic-boards/:id/match-detail/:tagId` | tag 与版块匹配明细 |
| GET | `/auxiliary-labels` | 查询辅助标签池 |
| GET | `/auxiliary-labels/clusters` | 辅助标签聚类 |
| POST | `/auxiliary-labels/merge-alias` | 合并辅助标签为 alias |
| POST | `/auxiliary-labels/gc` | 辅助标签垃圾回收 |
| POST | `/auxiliary-labels/:id/disable` | 禁用辅助标签 |
| GET | `/composite-labels` | 查询组合标签（?status=active/disabled） |
| POST | `/composite-labels` | 手动创建组合标签（source=manual，触发两级去重） |
| POST | `/composite-labels/:id/disable` | 禁用组合标签（向量置 NULL，组件与别名保留） |
| POST | `/composite-labels/:id/enable` | 启用组合标签（重算 embedding） |
| GET | `/composite-labels/component-options` | 组件候选推荐排序（挂载数→ref_count，带挂载版块名） |
| POST | `/semantic-boards/upgrade-suggest` | 四格参数触发 LLM 升级建议（内存响应，调试用） |
| POST | `/semantic-boards/upgrade-execute` | 执行升级建议 |
| GET | `/semantic-boards/upgrade-suggestions` | 列持久化升级建议 |
| POST | `/semantic-boards/upgrade-suggestions/:id/dismiss` | 忽略升级建议 |
| POST | `/semantic-boards/upgrade-suggestions/generate` | 生成升级建议 |
| POST | `/semantic-boards/backfill` | 触发匹配回填任务 |
| GET | `/semantic-boards/backfill/:id` | 查询回填进度 |
| POST | `/semantic-boards/backfill-embeddings` | 版块向量回填 |
| POST | `/semantic-boards/rematch-all` | 全量重新匹配 |
| GET | `/semantic-boards/matching-config` | 读取匹配参数 |
| PUT | `/semantic-boards/matching-config` | 更新匹配参数 |
| GET | `/tags/:id/auxiliary-labels` | tag 关联辅助标签 |
| GET | `/tags/:id/semantic-boards` | tag 所属版块 |
| POST | `/semantic-boards/:id/persistent-topics/manual` | 手动新建持久话题 |
| POST | `/semantic-boards/:id/topic-watches` | 创建版块级关注（label / keyword / keyword_topic / sentence_topic） |
| GET | `/semantic-boards/:id/topic-watches` | 列出该版块全部关注（含 paused） |
| PATCH | `/topic-watches/:id` | 更新关注 label / query 或 active / paused 状态 |
| DELETE | `/topic-watches/:id` | 删除关注及其命中记录（sentence_topic 需 confirm_archive_topic 确认归档专属话题） |
| GET | `/daily-reports/:id/watch-hits` | 查询某期日报的关注命中 |

## SemanticBoard CRUD

### GET `/semantic-boards`

查询 active SemanticBoard 列表。

Query：

- `search` 可选，按 `label` / `slug` 模糊搜索。
- `status` 可选；不传时只返回 `active`，可传 `disabled` 查看禁用项。

Response `data`：

```json
{
  "items": [
    {
      "id": 1,
      "label": "AI与机器学习",
      "slug": "ai-machine-learning",
      "aliases": [],
      "ref_count": 0,
      "tag_count": 12,
      "description": "追踪 AI 基础模型、应用和产业链变化",
      "display_order": 0,
      "source": "manual",
      "status": "active",
      "protected": true,
      "created_at": "2026-05-22T...",
      "updated_at": "2026-05-22T..."
    }
  ],
  "total": 1
}
```

### GET `/semantic-boards/:id`

查询单个 SemanticBoard，响应字段同列表单项。

### POST `/semantic-boards`

手动创建 SemanticBoard。后端会生成 embedding，并写入可选 composition。

Request：

```json
{
  "label": "AI与机器学习",
  "description": "追踪 AI 基础模型、应用和产业链变化",
  "display_order": 0,
  "protected": true,
  "auxiliary_labels": [10, 11, 12]
}
```

Response `data`：

```json
{ "id": 1 }
```

### PUT `/semantic-boards/:id`

更新 SemanticBoard。`label` 变更时会重新生成 embedding。

Request：

```json
{
  "label": "AI生态",
  "description": "更新后的描述",
  "display_order": 10,
  "protected": true,
  "status": "active",
  "enrichment_enabled": true,
  "window_days": 14,
  "relation_auto_discovery_enabled": false
}
```

可选字段（add-evidence-backed-cross-board-relations 起）：`enrichment_enabled`（数据增强总开关）、`window_days`、`context_layers`、`relation_auto_discovery_enabled`（跨版块关系自动发现板级开关，**默认 false**；开启后每份新简报按全局预算自动从观察发起发现，只生成待裁决建议、绝不自动确认）。字段缺省 = 不修改原值（nil 语义）。GET 列表/详情与 Board DTO 同步返回 `relation_auto_discovery_enabled`（旧数据缺省 false）。

Response `data`：

```json
{ "id": 1 }
```

### DELETE `/semantic-boards/:id`

软删除 SemanticBoard：将 `status` 置为 `disabled`。

Response `data`：

```json
{ "id": 1 }
```

## Board Composition

### GET `/semantic-boards/:id/composition`

查看 SemanticBoard 的构成辅助标签。

Response `data`：

```json
{
  "items": [
    {
      "id": 10,
      "label": "OpenAI",
      "slug": "openai",
      "aliases": ["Open AI"],
      "ref_count": 8,
      "description": "",
      "display_order": 0,
      "source": "llm_extract",
      "status": "active",
      "protected": false
    }
  ],
  "total": 1
}
```

### POST `/semantic-boards/:id/composition`

向版块 composition 新增一个辅助标签（幂等，重复添加不报错；不会自动回填历史 `topic_tag_board_labels`）。

Request：

```json
{ "auxiliary_label_id": 10 }
```

- `auxiliary_label_id` 必填，须为 `active` 的辅助标签。版块不存在返回 404；辅助标签不存在或非 active 返回 400。

Response `data`：

```json
{ "board_id": 1, "auxiliary_label_id": 10 }
```

### DELETE `/semantic-boards/:id/composition/:auxiliary_label_id`

从 board composition 中移除辅助标签；不会自动回填历史 `topic_tag_board_labels`，前端需要提示用户可手动触发回填。

Response `data`：

```json
{ "board_id": 1, "auxiliary_label_id": 10 }
```

## 辅助标签池

### GET `/auxiliary-labels`

查询辅助标签池。

Query：

- `search` 可选，按 `label` / `slug` 模糊搜索。
- `status` 可选，传 `active` 或 `disabled` 过滤；不传返回全部辅助标签。

Response `data`：

```json
{
  "items": [
    {
      "id": 10,
      "label": "OpenAI",
      "slug": "openai",
      "aliases": ["Open AI"],
      "ref_count": 8,
      "description": "",
      "display_order": 0,
      "source": "llm_extract",
      "status": "active",
      "protected": false
    }
  ],
  "total": 1
}
```

### POST `/auxiliary-labels/:id/disable`

禁用辅助标签。禁用后不会参与后续 board 匹配和升级候选。

Response `data`：

```json
{ "id": 10 }
```

### POST `/auxiliary-labels/merge-alias`

将 source 辅助标签合并为 target 的 alias，并迁移 `topic_tag_semantic_labels`。

Request：

```json
{ "source_id": 11, "target_id": 10 }
```

Response `data`：

```json
{ "source_id": 11, "target_id": 10 }
```

### GET `/auxiliary-labels/clusters`

对 active 且有 embedding 的辅助标签做余弦相似度聚类（相似度 > 0.8，即 cosine 距离 < 0.2 视为邻居），返回连通分量大小 ≥ 2 的簇，最多 50 簇。结果有 10 分钟内存缓存。

Query：

- `refresh=true` 或 `1` 可选，强制重算并刷新缓存。

Response `data`：

```json
{
  "clusters": [
    {
      "labels": [
        { "id": 10, "label": "OpenAI", "slug": "openai", "ref_count": 8 },
        { "id": 11, "label": "OpenAI Inc.", "slug": "openai-inc", "ref_count": 2 }
      ],
      "size": 2,
      "label": "OpenAI"
    }
  ],
  "unclustered_count": 3
}
```

`label` 为簇代表（`ref_count` 最高者）；`unclustered_count` 为未能进入任何 ≥2 簇的标签数。

### POST `/auxiliary-labels/gc`

辅助标签垃圾回收，清理无活跃引用的辅助标签。

Request：

```json
{ "mode": "dry_run", "grace_days": 7 }
```

- `mode` 必填，取值 `dry_run` / `disable` / `delete` / `recalculate`；非法值返回 400。
- `grace_days` 可选，宽限天数。

Response `data`：

```json
{
  "eligible_count": 5,
  "affected_count": 3,
  "corrected_count": 0,
  "total_count": 120,
  "preview": [{ "id": 99, "label": "废弃标签", "ref_count": 0, "created_at": "2026-01-01T..." }]
}
```

`preview` 在 `dry_run` 模式下列出候选标签。

## 升级候选与建议

### POST `/semantic-boards/upgrade-suggest`

四格参数触发 LLM 升级建议（内存响应，不落库；面板主链路走 `upgrade-suggestions/generate`）。split-board-upgrade-directions：`direction × source` 组合参数取代旧 `mode` 参数（旧参数与 `upgrade-candidates` 端点均已退役）。

Query 参数：

| 参数 | 说明 |
| ------ | ------ |
| `direction` | 必填：`create`（创建版块）/ `expand`（版块扩充） |
| `source` | 必填：`aux`（单标签）/ `composite`（组合标签） |
| `target_board_id` | `direction=expand` 必填（锁定单版块，本轮建议 target 恒等于它，须为活跃版块）；`create` 不得携带 |
| `days` | 可选时间窗（仅 create×aux 生效；`0`=不过滤） |

参数不合法（缺 direction/source、expand 缺 target、create 带 target、target 非活跃版块、携带旧 `mode` 参数）返回 400。

Response `data`：

```json
{
  "suggestions": [
    {
      "decision": "create_new",
      "board_label": "AI与机器学习",
      "description": "追踪 AI 模型与应用生态",
      "auxiliary_label_ids": [10, 11, 12],
      "reason": "候选标签语义集中且有共同事件上下文"
    },
    {
      "decision": "merge_into_existing",
      "target_board_id": 42,
      "auxiliary_label_ids": [13],
      "reason": "属于锁定版块「美债」"
    }
  ]
}
```

> skip 不返回（不落库不展示）；扩充方向的 merge/compose 建议 target 恒为锁定版块（服务端注入，LLM 输出不含目标字段）。

### POST `/semantic-boards/upgrade-execute`

确认执行一条升级建议。

创建新 board：

```json
{
  "decision": "create_new",
  "board_label": "AI与机器学习",
  "description": "追踪 AI 模型与应用生态",
  "auxiliary_label_ids": [10, 11, 12]
}
```

合并到已有 board：

```json
{
  "decision": "merge_into_existing",
  "target_board_id": 1,
  "auxiliary_label_ids": [13]
}
```

确认 compose 建议创建组合标签（add-composite-labels，同一事务：创建组合 + MarkConfirmed；去重命中按成功处理；split-board-upgrade-directions 起 `target_board_id` 非空时同事务内将组合挂载进目标版块——扩充方向建议，挂载失败整体回滚）：

```json
{
  "decision": "compose",
  "board_label": "美债收益率",
  "description": "美国国债与收益率的组合",
  "auxiliary_label_ids": [10, 11],
  "suggestion_id": 42
}
```

Response `data`（compose 时额外返回 `composite_label_id`）：

```json
{ "semantic_board_id": 1, "auxiliary_label_ids": [10, 11, 12] }
```

### GET `/semantic-boards/upgrade-suggestions`

查询已持久化的升级建议（区别于一次性的 legacy `upgrade-suggest`）。建议由调度器定期生成并落表，该接口只读持久表。

Query：

- `status` 可选，默认 `pending`。
- `decision` 可选；不传时默认排除观测池（`watch`），传 `watch` 仅返回观测池，传 `compose` 仅返回组合建议，传其它值则精确匹配该 decision。compose 建议的 `evidence` 携带共现证据（`compose_cooccurrence` / `compose_window_days` / `compose_representative_titles`）。

Response `data`：

```json
{
  "suggestions": [
    {
      "id": 3,
      "batch_id": "2026-07-01-abc",
      "mode": "discover_new",
      "decision": "create_new",
      "board_label": "AI与机器学习",
      "description": "...",
      "target_board_id": null,
      "auxiliary_label_ids": [10, 11],
      "auxiliary_labels": [{ "id": 10, "label": "OpenAI" }],
      "confidence": "high",
      "evidence": {},
      "status": "pending",
      "created_at": "2026-07-01T..."
    }
  ]
}
```

### POST `/semantic-boards/upgrade-suggestions/:id/dismiss`

忽略一条 pending 升级建议。请求体可选，可带 `reason` 记录忽略原因。

Request（可选）：

```json
{ "reason": "已手动处理" }
```

Response `data`：

```json
{ "id": 3, "status": "dismissed" }
```

### POST `/semantic-boards/upgrade-suggestions/generate`

四格参数触发一轮建议生成（同步执行并落库，面板主链路）。参数与校验同 [`upgrade-suggest`](#post-semantic-boards-upgrade-suggest)（`direction` / `source` / `target_board_id` / `days`）。受冷却时间限制，冷却期内同 hash 不写入。

无需请求体。Response `data`：

```json
{ "inserted": 3, "skipped": 1, "cooldown_blocked": 0 }
```

## 匹配回填

### POST `/semantic-boards/backfill`

触发异步回填任务。任务状态存在内存中，后端重启后会丢失。

Request：

```json
{ "mode": "all" }
```

```json
{ "mode": "unassigned" }
```

```json
{ "mode": "board", "board_id": 1 }
```

Response `data`：

```json
{
  "id": "semantic-board-backfill-1",
  "mode": "board",
  "board_id": 1,
  "total": 25,
  "processed": 0,
  "failed": 0,
  "status": "pending",
  "failures": [],
  "created_at": "2026-05-22T..."
}
```

### GET `/semantic-boards/backfill/:id`

查询回填进度。

Response `data.status`：`pending`、`running`、`completed`、`failed`。

### POST `/semantic-boards/backfill-embeddings`

为所有缺少 embedding 的版块（`label_type=board` 且 `embedding IS NULL`）补算向量。同步逐个嵌入并写库。

无需请求体。Response `data`：

```json
{ "backfilled": 2, "total": 3 }
```

### POST `/semantic-boards/rematch-all`

对所有已存在 `topic_tag_board_labels` 记录的 topic tag 全量重新匹配版块，同步执行。

无需请求体。Response `data`：

```json
{ "success": 120, "failed": 1, "total": 121 }
```

## 匹配参数配置

### GET `/semantic-boards/matching-config`

读取当前匹配参数。

Response `data`：

```json
{
  "semantic_board_match_sim_threshold": 0.6,
  "semantic_board_match_direct_hit_rate": 0.5,
  "semantic_board_match_direct_max_sim": 0.8,
  "semantic_board_match_direct_max_sim_min_hits": 2,
  "semantic_board_match_direct_max_sim_min_hit_rate": 0.3,
  "semantic_board_match_min_effective_sample": 3,
  "semantic_board_match_hit_rate_sim_blend": 0.7,
  "semantic_board_match_weight_sim": 0.6,
  "semantic_board_match_weight_density": 0.4,
  "semantic_board_match_weighted_threshold": 0.6,
  "semantic_board_match_max_boards": 3
}
```

### PUT `/semantic-boards/matching-config`

更新一个或多个匹配参数，值可以用数字或字符串传入。

Request：

```json
{
  "semantic_board_match_sim_threshold": 0.7,
  "semantic_board_match_max_boards": 2
}
```

Response `data`：返回更新后的完整配置。

## 匹配详情

### GET `/semantic-boards/:id/match-detail/:tagId`

按需实时计算某个 topic tag 与指定 SemanticBoard 的匹配明细。该接口不会修改匹配结果；`match_reason` 和 `score` 以 `topic_tag_board_labels` 中已存储值为准，`config` 和 `pairs` 用当前配置实时计算，便于解释“为什么这个 tag 属于这个版块”。

Response `data`：

```json
{
  "topic_tag_id": 42,
  "topic_tag_label": "人工智能监管政策",
  "semantic_board_id": 7,
  "match_reason": "hit_rate",
  "score": 0.845,
  "config": {
    "sim_threshold": 0.72,
    "hit_rate_sim_blend": 0.7,
    "min_effective_sample": 3,
    "direct_hit_rate": 0.5,
    "direct_max_sim": 0.8,
    "direct_max_sim_min_hits": 2,
    "direct_max_sim_min_hit_rate": 0.3,
    "weight_sim": 0.6,
    "weight_density": 0.4,
    "weighted_threshold": 0.6
  },
  "direct_hit_auxiliaries": [],
  "tag_auxiliary_count": 3,
  "hits": 2,
  "hit_rate": 0.6667,
  "max_similarity": 0.92,
  "pairs": [
    {
      "tag_auxiliary_id": 101,
      "tag_auxiliary_label": "AI安全监管",
      "board_auxiliary_id": 205,
      "board_auxiliary_label": "AI安全",
      "similarity": 0.92,
      "is_hit": true
    }
  ]
}
```

`direct_hit` 场景下，`direct_hit_auxiliaries` 会列出精确命中的辅助标签对，`pairs` 可为空。

## Tag 关联查询

### GET `/tags/:id/auxiliary-labels`

查询 topic tag 关联的辅助标签。

Response `data`：

```json
{
  "items": [
    { "id": 10, "label": "OpenAI", "slug": "openai", "aliases": [], "ref_count": 8, "status": "active" }
  ],
  "total": 1
}
```

### GET `/tags/:id/semantic-boards`

查询 topic tag 所属 SemanticBoard，按匹配分排序。

Response `data`：

```json
{
  "items": [
    {
      "board": {
        "id": 1,
        "label": "AI与机器学习",
        "slug": "ai-machine-learning",
        "tag_count": 0,
        "status": "active"
      },
      "score": 0.92,
      "match_reason": "direct_hit"
    }
  ],
  "total": 1
}
```

## 辅助标签建议

### GET `/semantic-boards/suggest-auxiliaries`

按自然语言查询全局推荐辅助标签（按与查询向量的余弦相似度排序），用于建/编版块时挑选构成标签。

Query：

- `label` 必填，查询文本。
- `description` 可选，拼接到查询文本。
- `search` 可选，按 label / slug 模糊过滤。
- `exclude_board_id` 可选，排除已在该版块 composition 中的标签。
- `page` / `page_size` 可选，默认 `1` / `20`，`page_size` 上限 100。

Response `data`：

```json
{
  "items": [
    { "id": 10, "label": "OpenAI", "slug": "openai", "aliases": [], "ref_count": 8, "similarity": 0.9123 }
  ],
  "total": 24,
  "page": 1,
  "page_size": 20
}
```

### GET `/semantic-boards/:id/suggest-auxiliaries`

以版块 `:id` 自身的 `label + description` 向量为查询，推荐可补充进该版块的辅助标签（自动排除已在该版块 composition 中的标签）。

Query：

- `search` / `page` / `page_size` 可选，同全局接口。

Response `data`：结构同全局接口。版块不存在返回 404。

## 版块文章与叙事

### GET `/semantic-boards/:id/articles`

查询版块 `:id` 下的文章（通过该版块的 topic tag 关联），支持按匹配质量或时间排序与分页。

Query：

- `page` / `per_page` 可选，默认 `1` / `20`，`per_page` 上限 100。
- `feed_id` 可选，按 feed 过滤。
- `auxiliary_label_id` 可选，按该辅助标签关联的 topic tag 过滤。
- `start_date` / `end_date` 可选，`YYYY-MM-DD`，按 `pub_date` 过滤（`end_date` 含当天）。
- `show_direction_mismatch=true` 可选，默认隐藏方向不一致的 tag。
- `sort` 可选，`quality`（默认，按匹配 tier/score 排序）或 `time`（按 `pub_date` 倒序）。

Response（`data` 为文章数组，`pagination` 与 `data` 同级）：

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "title": "...",
      "feed_name": "...",
      "filtered_tags": [
        { "id": 42, "label": "...", "category": "...", "match_reason": "hit_rate", "score": 0.84, "downgraded": false, "direction_mismatch": false }
      ]
    }
  ],
  "pagination": { "page": 1, "per_page": 20, "total": 35, "pages": 2 }
}
```

文章字段同 `GET /articles/:id`（`ToDict()`），额外带 `feed_name` 与 `filtered_tags`；无数据时 `data` 为空数组。

## 持久话题手动编排

### POST `/semantic-boards/:id/persistent-topics/manual`

手动新建一条持久话题（用户编排的"泳道"），将选中的一组 section 聚合成一条 active 话题。后端不走 LLM/AI，纯向量运算 + 单事务。配合前端编排态（`BoardThreadBrowser` 的 `compose` 视图）使用：候选池来自 GET `/semantic-boards/:id/persistent-topics/compose-candidates`，自然语言筛选向量来自 POST `/semantic-boards/:id/persistent-topics/embed-query`。

Request：

```json
{
  "label": "美伊博弈与油价联动",
  "section_ids": [101, 102, 105]
}
```

- `label` 必填，新话题展示名。
- `section_ids` 必填，选中的 section ID 列表；无 embedding 或维度不一致的 section 会被跳过（不计入失败）。

事务语义（任一步失败全部回滚，不留半建话题）：

1. **聚合**：解析每个选中 section 的 embedding，按维度一致性筛选可用集合，对可用向量做 mean pooling 得到聚合锚点。
2. **建话题**：`CreateTopic(status="active", source="manual")`，`embedding` 写聚合向量，`hit_count` / `consecutive_hits` 初始化为可用 section 数，`first_seen_date` / `last_seen_date` 取可用 section 报告期的最远 / 最近日期。手动话题直接 active，绕过 candidate→active 的连续命中门禁。
3. **改归属**：对每个可用 section 调 `UpdateSectionTopicAssignment(persistent_topic_id=新话题, topic_match_confidence="manual")`，覆盖原归属（`persistent_topic_id` 为单值外键，"移动"即覆盖，非共享）。
4. **重建关系**：`RebuildBoardRelations`，幂等清空并重建该 board 全部 similarity + identity 边，保证血统一致。

Response `data`：

```json
{
  "topic": {
    "id": 42,
    "semantic_board_id": 7,
    "label": "美伊博弈与油价联动",
    "description": "",
    "status": "active",
    "source": "manual",
    "first_seen_date": "2026-06-20",
    "last_seen_date": "2026-07-01",
    "hit_count": 3,
    "consecutive_hits": 3,
    "created_at": "2026-07-04T...",
    "updated_at": "2026-07-04T..."
  },
  "skipped": []
}
```

`skipped` 列出因无 embedding 或维度不一致被跳过的 section ID；非空时响应额外带 `message` 字段说明跳过数量（如 `"3 条 section 因无向量被跳过"`）。

错误情况（均返回 400）：

- `label` 为空。
- `section_ids` 为空。
- 可用 section 数为 0（所有选中 section 都无可用向量）—— 事务不提交，返回错误。
- 事务任一步失败（含 `RebuildBoardRelations` 失败）—— 整事务回滚。

**与日报生成的关系**：手动建泳道是用户即时操作，不在 `SaveReport` 日报生成流程内；建好的 active topic 在下一期日报生成时被 `ListAnchorableTopicsByBoard` 纳入 AND-gate，与算法生成的 active topic 一视同仁。

## 版块关注（topic-watch）

### POST `/semantic-boards/:id/topic-watches`

创建版块级关注。请求体：

```json
{
  "label": "ASML|镓锗 出口",
  "type": "keyword"
}
```

- `label` 必填。
- `type` 可选：`label`（默认，AI 语义单信号判定）/ `keyword`（确定性文本匹配，提示轨）/ `keyword_topic`（关键字物化轨）/ `sentence_topic`（一句话物化轨）。旧客户端不传 `type` 时按 `label` 创建。
- `keyword` / `keyword_topic` 语法：空格表示 AND、`|` 表示 OR；例如 `ASML|镓锗 出口` 表示「(ASML 或 镓锗) 且含 出口」。匹配不区分大小写。`keyword` 扫描日报 section 的叙事线标题与摘要（提示轨）；`keyword_topic` 扫描当天全部未归档文章的标题+摘要层（物化轨，可捞 tag 体系漏网文章），命中聚合成固定名「关键字『X』相关话题」板块（`lane_tier=watch_keyword`，零 AI，无持久话题）。
- `sentence_topic` 携带可选 `query`（检索句，embedding 输入，缺省回退 label）：每期日报按其向量在版块辅助标签池余弦检索 top-K（阈值/上限可配 `watch_sentence_retrieval_threshold` 0.55 / `watch_sentence_retrieval_top_k` 8），命中标签解析为当天有文章的 event tag，文章并集聚合为挂**专属持久话题**的板块（`lane_tier=watch_sentence`，`topic_match_confidence=manual`，跨天延续）。向量缓存于 `embedding_cache`，PATCH label/query 时失效、下次日报生成时惰性补算。
- keyword 家族表达式为空、纯空白、纯分隔符或以 `|` 结尾时返回 400；前导或中间空分支忽略，与服务端解析器一致。
- 物化轨从下一期日报开始生效（无历史回填）；物化失败降级跳过不阻断日报。

成功响应的 `data` 是创建的 watch。所有 watch 都带 `type`；keyword 创建额外带 `instant_hit_count`，表示已同步回扫本版块近 14 天日报后命中的 section 数（回扫失败不阻断创建，计数返回 0 并记服务端告警）：

```json
{
  "success": true,
  "data": {
    "id": 42,
    "semantic_board_id": 7,
    "label": "ASML|镓锗 出口",
    "type": "keyword",
    "status": "active",
    "instant_hit_count": 3,
    "created_at": "2026-08-24T...",
    "updated_at": "2026-08-24T..."
  }
}
```

### 关注管理与命中查询

- `GET /semantic-boards/:id/topic-watches` 返回该版块的 active 与 paused 关注；`type` 始终可用，历史行默认返回 `label`；sentence_topic 额外带 `query` 与 `persistent_topic_id`。
- `PATCH /topic-watches/:id` 可更新 `label`、`query` 或 `status`（仅 `active` / `paused`）；更新 `label` / `query` 会使 sentence_topic 的 `embedding_cache` 失效（下次日报生成时惰性补算）。
- `DELETE /topic-watches/:id` 会通过外键级联清理其 `topic_watch_hits`。sentence_topic 删除需携带查询参数 `confirm_archive_topic=true`：后端先软归档其专属持久话题（UpdateTopic status=archived，历史物化 section 保留且归属不变），再删关注行；未确认返回 400（错误信息含话题名）。keyword_topic 直接删除，历史物化板块保留。
- 日报列表响应的每个 report 额外带 `active_watch_summaries`：`[{ watch_id, label, type }]`。它由一次批量关联查询回填，按 watch 去重，仅含 active watch；列表用于在时间线记录下预告最多两个 `# keyword` / `✦ label` 命中，余项由前端显示 `+N`。
- `GET /daily-reports/:id/watch-hits` 仅返回该期 active watch 的命中记录，并额外含 `watch_label`、`watch_type`；label 的 `reason` 是 AI 理由，keyword 的 `reason` 是机械文本「含关键字『…』」。暂停或删除关注后重新读取不得返回其历史命中。

## 组合标签（add-composite-labels）

组合标签是 `semantic_labels.label_type="composite"` 的行（指向性主题，如「美债收益率」= 美国国债×收益率），由 2-5 个有序 active 辅助标签组成，参与 tag→board 匹配（composite_hit 最强信号）。

### GET `/composite-labels`

查询组合标签列表（组件序列按 position 排序）。

Query：

- `status` 可选；`active` / `disabled`，不传返回全部。

Response `data`：

```json
{
  "items": [
    {
      "id": 9, "label": "美债收益率", "slug": "mei-zhai-shou-yi-lu",
      "description": "美国国债与收益率的组合", "source": "manual",
      "status": "active", "ref_count": 1, "aliases": [],
      "created_at": "2026-09-02T...",
      "components": [
        { "label_id": 10, "label": "美国国债", "position": 1 },
        { "label_id": 11, "label": "收益率", "position": 2 }
      ]
    }
  ],
  "total": 1
}
```

### POST `/composite-labels`

手动创建组合标签（source=manual）。创建前 LLM 对「label. description」短语生成 embedding（失败整体不落库），再过两级去重：L1 组件 ID 集合与既有组合（含 disabled）一致 → 复用（ref_count++）；L2 组合 embedding ≥ `composite_label_dedupe_sim`（默认 0.95）→ 只加 alias。去重命中不报错，`outcome` 字段区分。

Request：

```json
{ "label": "美债收益率", "description": "美国国债与收益率的组合", "component_label_ids": [10, 11] }
```

约束：组件必须为 active 辅助标签，2-5 个不同 ID（组件数与类型不符返回 400）。

Response `data`：

```json
{
  "id": 9, "label": "美债收益率", "outcome": "created",
  "message": "组合标签已创建"
}
```

`outcome`：`created`（新建）/ `reused_l1`（组件集合同既有组合完全一致，复用既有组合 ref_count+1）/ `alias_l2`（语义高度相似，作为别名并入既有组合）。

### POST `/composite-labels/:id/disable`

禁用组合标签：向量置 NULL（匹配输入即失效），行本体、组件与别名保留。重新计算匹配后该组合不再产出 composite_hit。

### POST `/composite-labels/:id/enable`

启用组合标签：重算 embedding（生成失败返回 400，行保持 disabled）。

### GET `/composite-labels/component-options`

手动创建对话框的组件候选（design D7 推荐交互）。返回 active 辅助标签，按推荐度排序：被 active 版块挂载者优先（挂载数多者在前，disabled 版块挂载不计）→ ref_count → label；每个候选携带挂载版块名列表，供前端展示「挂 N 版块」推荐信号。前端默认用本列表，用户搜索时降级全量模糊检索（`/auxiliary-labels?search=`）。

Query：

- `limit` 可选，默认 50，上限 200。

Response `data`：

```json
{
  "items": [
    {
      "id": 10, "label": "美国国债", "ref_count": 3, "board_count": 2,
      "mounted_boards": [
        { "id": 1, "label": "宏观版块" },
        { "id": 2, "label": "债券版块" }
      ]
    }
  ]
}
```
