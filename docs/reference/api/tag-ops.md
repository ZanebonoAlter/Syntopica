# 标签运维与管理 Tag Ops

> 涵盖三大后台队列（嵌入队列、合并再嵌入队列、打标队列）、标签搜索与合并、合并预览扫描/评估（含 SSE 流）、标签关注。
>
> 通用约定（响应信封、错误格式）见 [_conventions.md](_conventions.md)。SSE 端点的 `Content-Type` 为 `text/event-stream`。

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/embedding/queue/status` | 嵌入队列状态汇总 |
| GET | `/api/embedding/queue/tasks` | 嵌入队列任务列表（分页） |
| POST | `/api/embedding/queue/retry` | 重试所有失败的嵌入任务 |
| POST | `/api/embedding/queue/person-metadata/backfill` | 回填人物元数据 |
| GET | `/api/embedding/merge-reembedding/status` | 合并再嵌入队列状态汇总 |
| GET | `/api/embedding/merge-reembedding/tasks` | 合并再嵌入队列任务列表（分页） |
| POST | `/api/embedding/merge-reembedding/retry` | 重试所有失败的合并再嵌入任务 |
| GET | `/api/tag-queue/status` | 打标队列状态汇总 |
| GET | `/api/tag-queue/tasks` | 打标队列任务列表（分页） |
| POST | `/api/tag-queue/retry` | 重试所有失败的打标任务 |
| POST | `/api/tag-queue/retag-today` | 把今日全部文章重新入队打标 |
| GET | `/api/topic-tags/search` | 按关键词搜索活跃标签 |
| POST | `/api/topic-tags/merge` | 合并两个标签（保留目标） |
| GET | `/api/topic-tags/merge-preview` | 取合并候选分组（扫描结果） |
| GET | `/api/topic-tags/merge-preview/status` | 查询扫描/评估是否在运行 |
| POST | `/api/topic-tags/merge-with-name` | 合并并重命名目标标签 |
| POST | `/api/topic-tags/merge-preview/dismiss` | 忽略某条合并建议 |
| POST | `/api/topic-tags/merge-preview/scan` | 启动一次全量合并扫描 |
| GET | `/api/topic-tags/merge-preview/scan/stream` | 扫描进度 SSE 流 |
| POST | `/api/topic-tags/merge-preview/evaluate` | 启动 LLM 评估合并候选 |
| GET | `/api/topic-tags/merge-preview/evaluate/stream` | 评估进度 SSE 流 |
| POST | `/api/topic-tags/merge-preview/add-to-group` | 手动把标签加入某合并组 |
| GET | `/api/topic-tags/watched` | 列出已关注标签 |
| POST | `/api/topic-tags/:tag_id/watch` | 关注一个标签 |
| POST | `/api/topic-tags/:tag_id/unwatch` | 取消关注标签 |

---

## 通知中心 Notifications

单用户通知信箱（add-notification-center）：白名单 = **日报生成终态**（完成 / 一条失败汇总，互斥），高频过程类事件（打标、抓取、自动刷新）不产生通知。新通知写入后同时经 WS 广播 `notification` 事件（载荷含完整通知对象）。

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/notifications` | 通知分页列表 |
| GET | `/api/notifications/unread-count` | 未读数 |
| POST | `/api/notifications/:id/read` | 单条标已读 |
| POST | `/api/notifications/read-all` | 全部标已读 |
| DELETE | `/api/notifications` | 清空全部通知 |

### GET /api/notifications

**查询参数**

| 参数 | 默认 | 说明 |
| ------ | ------ | ------ |
| `limit` | `20` | 每页数量 |
| `offset` | `0` | 偏移量 |
| `unread` | 空 | `true` 时只返回未读 |

**响应示例**

```json
{
  "success": true,
  "data": {
    "notifications": [
      {
        "id": 1,
        "type": "success",
        "title": "日报已生成 · 2026-09-17",
        "summary": "共 6 个版面，保存 42 条条目。",
        "link_type": "daily-report",
        "link_id": "2026-09-17",
        "is_read": false,
        "created_at": "2026-09-17T04:12:00Z"
      }
    ],
    "total": 12
  }
}
```

`type` 取值：`success`（完成）/ `error`（失败汇总）。通知表上限 500 行，写入超限时淘汰最旧已读行（全未读才淘汰最旧未读），淘汰在写入路径内完成。

---

## 嵌入队列 Embedding Queue

### GET /api/embedding/queue/status

返回嵌入队列的状态汇总（各状态计数等）。失败返回 `500`。

**响应示例**

```json
{ "success": true, "data": { "pending": 3, "processing": 1, "failed": 2 } }
```

### GET /api/embedding/queue/tasks

分页查询嵌入队列任务。

**查询参数**

| 参数 | 默认 | 说明 |
| ------ | ------ | ------ |
| `status` | 空 | 按状态过滤 |
| `limit` | `50` | 每页数量；`<=0` 或 `>200` 时回退为 `50` |
| `offset` | `0` | 偏移量；`<0` 时回退为 `0` |

**响应示例**

```json
{
  "success": true,
  "data": { "tasks": [ /* ... */ ], "total": 120 }
}
```

### POST /api/embedding/queue/retry

把所有失败状态的嵌入任务重置为可重试。返回重试条数提示。

**响应示例**

```json
{
  "success": true,
  "message": "已重试 2 个失败任务"
}
```

### POST /api/embedding/queue/person-metadata/backfill

触发人物元数据回填。

**响应示例**

```json
{ "success": true, "data": { "processed": 15 } }
```

---

## 合并再嵌入队列 Merge Reembedding Queue

### GET /api/embedding/merge-reembedding/status

返回合并再嵌入队列的状态汇总。失败返回 `500`。

### GET /api/embedding/merge-reembedding/tasks

分页查询合并再嵌入队列任务。

**查询参数**

| 参数 | 默认 | 说明 |
| ------ | ------ | ------ |
| `status` | 空 | 按状态过滤；非空时取值必须为 `pending` / `processing` / `completed` / `failed`，否则 `400` |
| `limit` | `50` | 每页数量，需 `1–200`，否则 `400` |
| `offset` | `0` | 偏移量，需 `>=0`，否则 `400` |

**响应示例**

```json
{
  "success": true,
  "data": { "tasks": [ /* ... */ ], "total": 8 }
}
```

### POST /api/embedding/merge-reembedding/retry

重试所有失败的合并再嵌入任务。

**响应示例**

```json
{ "success": true, "message": "已重试 1 个失败任务" }
```

---

## 打标队列 Tag Queue

### GET /api/tag-queue/status

返回打标队列（`tag_jobs` 表）按状态分组的计数。

**响应示例**

```json
{
  "success": true,
  "data": {
    "pending": 5,
    "processing": 1,
    "completed": 320,
    "failed": 2,
    "total": 328,
    "completed_today": 40
  }
}
```

`completed_today` 为今日（服务器本地时区 00:00 起）completed 行计数。由于 `log_cleanup` 每日清理昨日以前的 completed 行（保留策略见 [DATA_LIFECYCLE.md](../database/DATA_LIFECYCLE.md)），`completed` 为近 1 天内的累计值；前端展示语义以**活跃量（pending+processing）为主、今日完成为辅**，累计 total 不再用于展示。

### GET /api/tag-queue/tasks

分页查询打标队列任务（最新优先，预加载文章标题）。

**查询参数**

| 参数 | 默认 | 说明 |
| ------ | ------ | ------ |
| `status` | 空 | 按状态过滤 |
| `limit` | `50` | 每页数量；`<=0` 或 `>200` 时回退为 `50` |
| `offset` | `0` | 偏移量；`<0` 时回退为 `0` |

**响应示例**

```json
{
  "success": true,
  "data": {
    "tasks": [
      {
        "id": 451,
        "article_id": 9821,
        "article_title": "某文章标题",
        "feed_name_snapshot": "某订阅源",
        "category_name_snapshot": "财经",
        "priority": 0,
        "status": "failed",
        "attempt_count": 3,
        "max_attempts": 5,
        "force_retag": false,
        "reason": "",
        "last_error": "...",
        "created_at": "2026-07-07T08:00:00Z",
        "leased_at": null
      }
    ],
    "total": 1
  }
}
```

### POST /api/tag-queue/retry

把所有 `failed` 打标任务重置为 `pending`（清零尝试次数与租约）。

**响应示例**

```json
{ "success": true, "message": "已重试 2 个失败任务" }
```

### POST /api/tag-queue/retag-today

把当日（自然日 0 点起）发布的全部文章以 `force_retag=true`、`reason=retag_today` 重新入打标队列。

**响应示例**

```json
{
  "success": true,
  "message": "已提交 12 篇今日文章的重新打标任务",
  "data": { "total": 12, "enqueued": 12 }
}
```

当日无文章时返回 `enqueued: 0`。

---

## 标签搜索与合并 Search & Merge

### GET /api/topic-tags/search

按关键词模糊搜索活跃标签（`status` 为 `active` / 空 / `NULL`），按 `feed_count` 降序排序。

**查询参数**

| 参数 | 默认 | 说明 |
| ------ | ------ | ------ |
| `q` | — | 搜索关键词（`label ILIKE %q%`）；为空时返回空数组 |
| `category` | 空 | 按分类精确过滤 |
| `limit` | `20` | 返回数量；`<1` 或 `>100` 时回退为 `20` |

**响应示例**

```json
{ "success": true, "data": [ { "id": 5, "label": "原油", "slug": "原油", "feed_count": 8 } ] }
```

### POST /api/topic-tags/merge

把源标签合并进目标标签（保留目标）。合并为不可逆操作。

**请求体**

```json
{ "source_tag_id": 12, "target_tag_id": 5 }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `source_tag_id` | 是 | 被合并的源标签 ID |
| `target_tag_id` | 是 | 保留的目标标签 ID |

任一缺失、两者相同、或源/目标不存在分别返回 `400` / `400` / `404`。

**响应示例**

```json
{
  "success": true,
  "message": "tags merged",
  "data": { "source_id": 12, "target_id": 5, "target_label": "原油" }
}
```

---

## 合并预览 Merge Preview

### GET /api/topic-tags/merge-preview

返回按目标标签分组的合并候选（取自 `tag_merge_suggestions` 表 `pending` 记录，按相似度降序）。已判定 `should_merge:false` 的建议会被过滤掉。

**查询参数**

| 参数 | 默认 | 说明 |
|------|------|------|
| `limit` | `50` | 取值 `1–200`，超出回退为 `200`，非法回退为 `50` |

**响应示例**

```json
{
  "success": true,
  "data": {
    "groups": [
      {
        "target_tag_id": 5,
        "target_label": "原油",
        "target_slug": "原油",
        "target_articles": 18,
        "category": "财经",
        "suggestions": [
          {
            "id": 91,
            "new_tag_id": 33,
            "new_label": "原油ETF",
            "new_slug": "原油etf",
            "similarity": 0.92,
            "new_articles": 4,
            "llm_verdict": "",
            "source": "scan"
          }
        ]
      }
    ],
    "total_groups": 1,
    "evaluated": false
  }
}
```

`evaluated` 表示是否存在任何已带 LLM 判定的建议。

### GET /api/topic-tags/merge-preview/status

查询扫描与评估的运行态（轻量轮询用）。

**响应示例**

```json
{ "scan_running": false, "eval_running": true }
```

### POST /api/topic-tags/merge-with-name

合并两个标签并（可选）把目标标签重命名为新名称。重命名时会做 slug 冲突检测；合并提交后再把相关建议置为 `merged`。

**请求体**

```json
{ "source_tag_id": 33, "target_tag_id": 5, "new_name": "原油及原油ETF" }
```

| 字段 | 必填 | 说明 |
| ------ | ------ | ------ |
| `source_tag_id` | 是 | 被合并的源标签 ID |
| `target_tag_id` | 是 | 保留的目标标签 ID |
| `new_name` | 是 | 目标标签的新名称（去空格后不可为空） |

参数缺失、两者相同、`new_name` 为空分别返回 `400`；源/目标不存在返回 `404`；新名与已有活跃标签 slug 冲突返回 `400`。

**响应示例**

```json
{
  "success": true,
  "data": {
    "source_id": 33,
    "target_id": 5,
    "new_label": "原油及原油ETF",
    "merged_at": "2026-07-07T12:00:00Z"
  }
}
```

### POST /api/topic-tags/merge-preview/dismiss

把某条 `pending` 合并建议标记为 `dismissed`。

**请求体**

```json
{ "new_tag_id": 33, "existing_tag_id": 5 }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `new_tag_id` | 是 | 建议中的新标签 ID |
| `existing_tag_id` | 是 | 建议中的目标（已存在）标签 ID |

参数缺失返回 `400`；找不到匹配的 `pending` 建议返回 `404`。

### POST /api/topic-tags/merge-preview/scan

启动一次异步全量合并扫描。已有扫描在跑时返回 `409`。

**响应示例**（`202`）

```json
{ "success": true, "message": "scan started" }
```

### GET /api/topic-tags/merge-preview/scan/stream

扫描进度的 SSE 流。`Content-Type: text/event-stream`，事件名 `progress`，载荷为扫描进度对象。连接建立后阻塞读取进度，扫描空闲或上下文取消时推送一次 `{ "status": "idle" }` 后关闭。

### POST /api/topic-tags/merge-preview/evaluate

启动一次异步 LLM 评估（对合并候选做 `should_merge` 判定）。已有评估在跑时返回 `409`。

**响应示例**（`202`）

```json
{ "success": true, "message": "evaluation started" }
```

### GET /api/topic-tags/merge-preview/evaluate/stream

评估进度的 SSE 流。`Content-Type: text/event-stream`，事件名 `progress`，载荷为评估进度对象。空闲或取消时推送 `{ "status": "idle" }` 后关闭。

### POST /api/topic-tags/merge-preview/add-to-group

手动把一个标签加入某个合并组（写入一条 `source=manual` 的 `pending` 建议）。若该 (new, existing) 建议已存在则返回 `409`。

**请求体**

```json
{ "target_tag_id": 5, "new_tag_id": 44 }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `target_tag_id` | 是 | 目标（已存在）标签 ID |
| `new_tag_id` | 是 | 被加入的新标签 ID |

参数缺失、两者相同分别返回 `400`；源/目标标签不存在返回 `404`；建议已存在返回 `409`。

---

## 标签关注 Watched Tags

### GET /api/topic-tags/watched

列出全部已关注标签（含其抽象标签元数据）。

### POST /api/topic-tags/:tag_id/watch

把某标签标记为关注。

**响应示例**

```json
{
  "success": true,
  "data": { "id": 5, "is_watched": true, "watched_at": "2026-07-07T12:00:00Z" }
}
```

`:tag_id` 非法返回 `400`；标签不存在返回 `404`。

### POST /api/topic-tags/:tag_id/unwatch

取消某标签的关注。

**响应示例**

```json
{ "success": true, "data": { "id": 5, "is_watched": false } }
```

`:tag_id` 非法返回 `400`；标签不存在返回 `404`。
