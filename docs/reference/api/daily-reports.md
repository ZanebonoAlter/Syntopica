# 日报 API

基础地址：`http://localhost:5000/api`

通用响应：成功为 `{"success": true, "data": ...}`（个别管理端点如 DELETE 仅返回 `{"success": true}`）；失败为 `{"success": false, "error": "..."}`。

## 端点汇总

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| POST | `/daily-reports/generate` | 手动触发日报生成（异步） |
| GET | `/daily-reports/:id` | 查询单篇日报详情 |
| GET | `/semantic-boards/:id/daily-reports` | 查询板块日报列表 |
| GET | `/semantic-boards/:id/section-timeline` | 板块 section 时间线 |
| GET | `/semantic-boards/:id/topics` | 列出板块全部持久话题（含归档/孤儿）+ section 计数 |
| GET | `/semantic-boards/:id/lane-dynamics?days=14` | 板块泳道动态聚合（活跃泳道卡+发展时间线+候选栏，单请求） |
| GET | `/daily-reports/sections/:id/lifecycle` | section 连通分量生命周期 |
| GET | `/daily-reports/topics/:id/lifeline` | 持久话题全量 section |
| PATCH | `/daily-reports/topics/:id` | 更新话题标题/状态 |
| DELETE | `/daily-reports/topics/:id` | 硬删话题（解绑 section） |
| POST | `/daily-reports/topics/:id/merge` | 合并源话题到目标 |
| POST | `/daily-reports/topics/:id/split` | 拆分 section 为新话题 |
| POST | `/daily-reports/backfill-embeddings` | 回填 section 向量 |
| POST | `/daily-reports/backfill-relations` | 回填话题关系 |
| POST | `/daily-reports/backfill-topics` | 历史重建持久话题 |
| GET | `/semantic-boards/:id/persistent-topics/compose-candidates` | 手动编排候选 section |
| POST | `/semantic-boards/:id/persistent-topics/embed-query` | 自然语言向量化 |
| POST | `/semantic-boards/:id/topic-watches` | 新建话题盯盘 |
| GET | `/semantic-boards/:id/topic-watches` | 列出盯盘规则 |
| PATCH | `/topic-watches/:id` | 更新盯盘 label/status |
| DELETE | `/topic-watches/:id` | 删除盯盘规则 |
| GET | `/daily-reports/:id/watch-hits` | 查询日报命中盯盘 |

## POST `/daily-reports/generate`

手动异步触发日报生成。`board_id` 为空时生成所有当日有事件标签的板块。

Request：

```json
{ "date": "2026-05-26", "board_id": 2849 }
```

Response `data`：

```json
{ "job_id": "...", "status": "processing" }
```

WebSocket 进度消息：

```json
{
  "type": "daily_report_progress",
  "job_id": "...",
  "board_id": 2849,
  "board_name": "刚果（金）局势",
  "status": "generating",
  "saved": 0,
  "progress": "0/1",
  "timestamp": "2026-05-26T..."
}
```

**重建窗口守卫（offline-catchup）**：`date` 早于标签边保留窗口下界（`ai_settings.tag_edge_retention_days`，默认 7 天，即 `date < today - N*24h`）时返回 **4xx**，错误消息说明「标签边已按 N 天窗口回收、超窗日期候选不全，拒绝重建」——防止超窗日期的空报告覆盖既有好报告（同日重建是整份覆盖语义，见 `flow/daily-report.md` 业务约束 20）。窗口内日期（含恰好等于下界）不受影响，照常异步触发。窗口口径与 `aux_label_cleanup` 的边 GC、日报自动补档扫描同键同口径。

## GET `/semantic-boards/:id/section-timeline?days=30`

查询板块日报 section 时间线，供 2D 线索浏览和 3D 侦探墙复用。

`days` 以该板块最新 completed 日报为终点，精确包含最近 N 个自然日（允许 1-90，默认 30）。section 的 `status` 只由 `relation_type=similarity` 的匈牙利关系推导，identity 关系不参与状态计算。

Response `data`：

```json
{
  "sections": [
    {
      "id": 101,
      "report_id": 12,
      "period_date": "2026-05-26T12:00:00Z",
      "cluster_label": "...",
      "status": "continuing",
      "article_count": 5,
      "thread_count": 2,
      "image_url": "https://...",
      "persistent_topic_id": 9,
      "topic_match_confidence": "anchor_hit",
      "persistent_topic": {
        "id": 9,
        "label": "美伊谈判",
        "status": "candidate",
        "color": "#60a5fa",
        "consecutive_hits": 3,
        "can_activate": true
      }
    }
  ],
  "relations": [
    { "from_id": 100, "to_id": 101, "distance": 0.23, "relation_type": "similarity" }
  ]
}
```

`image_url` 始终回传；无可用图片时为空字符串。后端优先从该 section 的线程关联文章中选择第一张非空图片，找不到时再从该 section 的 cluster tags 当天文章里选择第一张非空图片。

## GET `/daily-reports/sections/:id/lifecycle`

查询一个 section 所在连通分量的完整生命周期。响应结构与 `section-timeline` 相同，同样可能包含可选 `image_url`。

## GET `/daily-reports/topics/:id/lifeline`

按 `persistent_topic_id` 查询一个持久话题的全部 section 与内部关系，不受时间窗限制。

## PATCH `/daily-reports/topics/:id`

更新话题标题或状态，`label` 与 `status` 均为可选，省略的字段保持不变。`status` 仅接受 `active` / `archived`：candidate 只有在 `hit_count >= persistent_topic_upgrade_threshold` 后才允许人工更新为 active，未达门禁返回 400；active 才进入独立持久话题泳道。

```json
{ "label": "美伊博弈", "status": "active" }
```

Response `data`：更新后的持久话题对象（含 `id` / `label` / `status` / `hit_count` / `consecutive_hits` / `first_seen_date` / `last_seen_date` 等）。

终态总会广播：

```json
{
  "type": "daily_report_done",
  "job_id": "...",
  "total_saved": 1,
  "total_boards": 1,
  "timestamp": "2026-05-26T..."
}
```

`daily_report_progress.status` 使用 `generating`、`completed`、`failed`。

## DELETE `/daily-reports/topics/:id`

硬删除一条持久话题，并把其名下 section 的 `persistent_topic_id` 置空（section 内容保留，仅解除归属）。操作不可逆；可逆路径是 PATCH `status=archived`。

无需请求体。Response：

```json
{ "success": true }
```

## POST `/daily-reports/topics/:id/merge`

把一组源话题的全部 section 归并到目标话题 `:id` 下，源话题被归档（不物理删除，保留审计痕迹）。源话题必须与目标同属一个 board 且不能等于目标；后端会重建 board 关系保证血统一致。

Request：

```json
{ "source_topic_ids": [2, 3] }
```

- `source_topic_ids` 必填，源话题 id 列表；为空或全部等于目标时返回 400。

Response `data`：合并后的目标话题对象（结构同 PATCH）。

## POST `/daily-reports/topics/:id/split`

从源话题 `:id` 切出一组 section，聚合新建一条话题；被切的 section 重新归属到新话题。

Request：

```json
{ "section_ids": [10, 11], "label": "新拆出的话题" }
```

- `section_ids` 必填，要切出的 section id 列表。
- `label` 新话题展示名。

Response `data`：新建的话题对象（结构同 PATCH）。

## GET `/semantic-boards/:id/topics`

列出某板块下的全部持久话题（active / candidate / archived / 孤儿），并附带每个话题的 section 计数。与 `section-timeline` 不同，该接口不受时间窗限制，专门用于管理 UI 暴露 backfill 产生的异常话题；返回前会过滤掉 `hit_count < persistent_topic_upgrade_threshold` 的观测期 candidate。

Response `data`：

```json
{
  "topics": [
    {
      "id": 9,
      "semantic_board_id": 7,
      "label": "美伊谈判",
      "description": "",
      "status": "active",
      "source": "auto",
      "first_seen_date": "2026-06-01",
      "last_seen_date": "2026-07-01",
      "hit_count": 12,
      "consecutive_hits": 5,
      "created_at": "2026-06-01T...",
      "updated_at": "2026-07-01T...",
      "section_count": 8,
      "color": "#60a5fa",
      "can_activate": false
    }
  ]
}
```

`color` 由话题 id 稳定哈希得到；`can_activate` 表示 candidate 是否已达激活门禁。

## GET `/semantic-boards/:id/lane-dynamics?days=14`

板块泳道动态聚合：「板块内容」tab 首屏泳道动态视图的单请求数据源（活跃泳道卡 + 滚动态势 + 发展时间线 + 候选栏）。取代已退役的 topic-landscape 端点。

**参数：**

- `id`：semantic_board_id。
- `days`：时间线与统计窗口天数，默认 14；非正值或非法值回落 14。窗口锚定该板块最新一份已完成日报的 `period_date`（`[anchor-(days-1), anchor]` 双端闭区间，非 now()——避免日报未跑时窗口漂移）。

**泳道集合与排序**：`lanes` = 该板块 active 泳道 ∪ watch 关联泳道（`board_topic_watches.persistent_topic_id` 非空且 watch active），按窗口内锚定 section 数降序；近窗口无 section 的 active（沉寂）不返回；candidate 不进 `lanes`（只在 `candidates`）。

Response `data`：

```json
{
  "window_days": 14,
  "has_reports": true,
  "lanes": [
    {
      "topic_id": 12,
      "label": "芯片战",
      "watch_linked": true,
      "section_count_14d": 9,
      "snapshot": {
        "summary": "近两周围绕出口管制升级与国产替代进展交替演进……",
        "as_of": "2026-09-09"
      },
      "timeline": [
        {
          "date": "2026-09-08",
          "sections": [
            {
              "section_id": 341,
              "label": "新规生效首日影响",
              "events": ["厂商A暂停部分订单", "替代供应商订单量抬升"],
              "folded_count": 0
            }
          ]
        }
      ]
    }
  ],
  "candidates": [
    { "topic_id": 55, "label": "稀土出口博弈", "last_seen_date": "2026-09-08", "recent_hint": "稀土出口博弈：管制清单扩容传闻" }
  ]
}
```

- `snapshot`：滚动 14 天态势句（每日日报生成后异步结算，见 `flow/daily-report.md` §泳道态势结算）；`null` 表示暂无快照（新泳道或结算未跑），前端降级「待结算」占位、时间线照常渲染。
- `timeline[].sections[].events`：该 section 的 thread 标题，最多前 5 条；`folded_count` 为被截断数量（恒输出，0 也带）。`timeline` 按日期倒序（最新日在最前）。
- `candidates`：达可见门槛（`FilterVisibleTopics`）的 candidate + 最新 section 标题 `recent_hint`；只读提示，转正走话题管理入口。
- `has_reports: false` 表示板块无任何日报（前端空态引导生成日报）。

## GET `/semantic-boards/:id/daily-reports?days=7`

查询板块日报列表。

**参数：**

- `days`：查询最近 N 天的日报。默认值为 7（当 `days` 未提供或 `days <= 0` 时）。对于任意正数 `days`，系统将按实际请求天数查询，不再静默截断到 30 天。

Response `data`：

```json
{
  "reports": [
    {
      "id": 1,
      "semantic_board_id": 2849,
      "period_date": "2026-05-26",
      "title": "...",
      "summary": "...",
      "status": "completed",
      "cluster_count": 2,
      "article_count": 3,
      "event_tag_count": 5,
      "created_at": "2026-05-26T..."
    }
  ]
}
```

## GET `/daily-reports/:id`

查询单篇日报详情（含 sections）。

Response `data`：

```json
{
  "report": {
    "id": 1,
    "semantic_board_id": 2849,
    "period_date": "2026-05-26T12:00:00Z",
    "title": "...",
    "summary": "...",
    "status": "completed",
    "highlights": [],
    "dynamics": "...",
    "sections": [
      {
        "id": 366,
        "cluster_label": "...",
        "persistent_topic_id": 5,
        "persistent_topic": {
          "id": 5,
          "label": "...",
          "status": "active",
          "color": "#...",
          "consecutive_hits": 3,
          "can_activate": false
        },
        "topic_status_at_report": "active",
        "threads": []
      }
    ]
  }
}
```

当 section 已归属持久话题时，详情响应会附带轻量 `persistent_topic` 描述，供前端按
`topic_status_at_report` 快照分区（`active` / `candidate` / `null`）。历史未归属 section 返回 `null`。

## POST `/daily-reports/backfill-embeddings`

异步回填/重算 section 向量（最长运行 30 分钟，进度写日志，不广播 WebSocket）。嵌入文本按内容化规则组装（section 所聚 tag 的 label/description/代表文章摘录，与主流水线一致）。完成后重算受影响 topic 质心并重建相关板块跨日关系。

Query：

- `recompute` 可选，默认 `false`。`true` 时对范围内**全部** section 重算 embedding（含已有值）；`false` 仅补 `embedding IS NULL` 的 section。
- `board_id` 可选，仅 `recompute=true` 时生效，限定单个板块；缺省全部板块。
- `since_days` 可选，仅 `recompute=true` 时生效，默认 `30`（回刷最近 30 天），`0` 表示不限。

无需请求体。Response `data`：

```json
{ "status": "processing" }
```

## POST `/daily-reports/backfill-relations`

异步重建 section 间关系。可选 query `?board_id=` 限定单个板块；不传则重建全部板块。

Query：

- `board_id` 可选，限定单个板块。

Response `data`：

```json
{ "status": "processing" }
```

## POST `/daily-reports/backfill-topics`

异步从历史未归属 section 重建持久话题。可选 query `?board_id=` 限定单个板块；不传则处理所有含未归属 section 的板块。

Query：

- `board_id` 可选，限定单个板块。

Response `data`：

```json
{ "status": "processing" }
```

## GET `/semantic-boards/:id/persistent-topics/compose-candidates`

为手动编排 UI（`POST /semantic-boards/:id/persistent-topics/manual`）加载候选 section（带可用向量）及配置的 `match_threshold`，前端据此实时计算聚合锚点与离群距离。

Query：

- `days` 可选，时间窗天数；不传或 `<=0` 取全部历史，`>90` 截断到 90。窗口以该板块最新 completed 日报为终点。

Response `data`：

```json
{
  "sections": [
    {
      "id": 101,
      "report_id": 12,
      "period_date": "2026-07-01T12:00:00Z",
      "cluster_label": "...",
      "embedding": [0.0123, -0.0456],
      "persistent_topic_id": 9,
      "topic_match_confidence": "anchor_hit",
      "persistent_topic": { "id": 9, "label": "美伊谈判", "status": "active", "color": "#60a5fa" }
    }
  ],
  "match_threshold": 0.6
}
```

## POST `/semantic-boards/:id/persistent-topics/embed-query`

把自然语言查询向量化，供编排 UI 按余弦相似度排序候选。使用与 section embedding 相同的全局模型（`CapabilityEmbedding`），因此 `:id` 仅为路由占位（与同树的 manual / compose-candidates 并列，不实际读取该板块）。

Request：

```json
{ "query": "美伊博弈与油价联动" }
```

- `query` 必填，自然语言查询；空白返回 400。

Response `data`：

```json
{ "embedding": [0.0123, -0.0456] }
```

## 话题盯盘（Topic Watches）

话题盯盘是用户声明的单信号 AI 检测器：每期日报结束时按盯盘 label 检测命中的 section，命中记录为只读叠加层，**不改变**任何 section 的 `persistent_topic_id` 或话题状态。board 级路由参数用 `:id`（与同树 `/semantic-boards/:id/*` 共存所需），独立操作路由用 `/topic-watches/:id`。

### POST `/semantic-boards/:id/topic-watches`

为板块 `:id` 新建一条盯盘规则，默认 `status=active`。

Request：

```json
{ "label": "盯盘：美伊停火" }
```

- `label` 必填，盯盘展示名；为空返回 400。

Response `data`：

```json
{
  "id": 1,
  "semantic_board_id": 7,
  "label": "盯盘：美伊停火",
  "status": "active",
  "created_at": "2026-07-01T...",
  "updated_at": "2026-07-01T..."
}
```

### GET `/semantic-boards/:id/topic-watches`

列出板块 `:id` 的全部盯盘规则（含 paused）。Response `data` 直接为盯盘对象数组：

```json
[
  { "id": 1, "semantic_board_id": 7, "label": "盯盘：美伊停火", "status": "active", "created_at": "...", "updated_at": "..." }
]
```

### PATCH `/topic-watches/:id`

更新盯盘 `label` 和/或 `status`，二者皆可选，省略则不变。`status` 仅接受 `active` / `paused`，其余值返回 400。

Request：

```json
{ "label": "盯盘：美伊停火（暂停）", "status": "paused" }
```

Response `data`：更新后的盯盘对象（结构同 POST）。

### DELETE `/topic-watches/:id`

删除盯盘规则，关联命中记录随外键级联删除。Response：

```json
{ "success": true, "message": "deleted" }
```

### GET `/daily-reports/:id/watch-hits`

查询某篇日报命中盯盘的情况（`:id` 为日报 id）。Response `data` 直接为命中记录数组：

```json
[
  {
    "id": 5,
    "watch_id": 1,
    "section_id": 101,
    "report_id": 12,
    "period_date": "2026-07-01",
    "reason": "...",
    "created_at": "2026-07-01T..."
  }
]
```

每条记录由 `(watch_id, section_id, report_id)` 唯一索引去重。

## 前端消费约定

日报阅读层不新增 API：列表和详情分别消费 `GET /semantic-boards/:id/daily-reports` 与 `GET /daily-reports/:id`，按 `topic_status_at_report` 快照分区：

- `"active"` → "关心的话题"
- `"candidate"` / `null`（含旧数据缺失）→ "其他动态"

前端不再读取 `persistent_topic.status` 做历史日报分区，避免 topic 后续状态变化改变历史归位。masthead 顶部使用板块标题，日报 `title` / `summary` 映射为头条；highlights 最多展示三项。

active 话题展开时按 `persistent_topic.id` 懒加载 `GET /daily-reports/topics/:id/lifeline`。前端只截取当前日报向前七个自然日，同日多个 section 聚合为一个带数量角标的节点；连线只消费 `relation_type=identity`，`similarity` 不进入 mini 泳道。节点点击按 `report_id` 复用日报详情缓存，在时间线模块内原位展示当日 section/thread。

thread 的 `related_article_ids` 由现有 `GET /articles/:id` 逐项解析标题并按 article id 去重缓存；单篇 404/失败局部显示重试，不阻塞同话题其他文章。
