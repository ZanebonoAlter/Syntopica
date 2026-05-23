# SemanticBoard 与辅助标签 API

基础地址：`http://localhost:5000/api`

通用响应：成功为 `{"success": true, "data": ...}`；失败为 `{"success": false, "error": "..."}`。

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
  "status": "active"
}
```

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

## 升级候选与建议

### GET `/semantic-boards/upgrade-candidates`

查看满足 `semantic_board_upgrade_ref_count_threshold` 的未升级辅助标签，以及预聚类结果。

Response `data`：

```json
{
  "candidates": [
    { "id": 10, "label": "OpenAI", "slug": "openai", "ref_count": 8 }
  ],
  "clusters": [
    {
      "candidates": [{ "id": 10, "label": "OpenAI", "slug": "openai", "ref_count": 8 }],
      "existing_board_id": null,
      "existing_board_label": "",
      "existing_board_description": "",
      "existing_board_auxiliary_labels": []
    }
  ],
  "config": {
    "semantic_board_upgrade_ref_count_threshold": 5,
    "semantic_board_upgrade_cluster_distance_threshold": 0.7,
    "semantic_board_upgrade_cotag_window_days": 30,
    "semantic_board_upgrade_cotag_top_n": 20,
    "semantic_board_upgrade_cotag_dedupe_sim_threshold": 0.85,
    "semantic_board_upgrade_cotag_hard_limit": 15
  }
}
```

### POST `/semantic-boards/upgrade-suggest`

触发 LLM 升级建议。用户确认前不会写入 SemanticBoard 或 board composition。

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
      "target_board_id": 1,
      "auxiliary_label_ids": [13],
      "reason": "与现有 board 语义一致"
    },
    {
      "decision": "skip",
      "auxiliary_label_ids": [],
      "reason": "标签语义过散"
    }
  ]
}
```

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

Response `data`：

```json
{ "semantic_board_id": 1, "auxiliary_label_ids": [10, 11, 12] }
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

## 匹配参数配置

### GET `/semantic-boards/matching-config`

读取当前匹配参数。

Response `data`：

```json
{
  "semantic_board_match_sim_threshold": 0.6,
  "semantic_board_match_direct_hit_rate": 0.5,
  "semantic_board_match_direct_max_sim": 0.8,
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
