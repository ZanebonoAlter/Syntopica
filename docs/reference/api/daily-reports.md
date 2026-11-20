# 日报 API

基础地址：`http://localhost:5100/api`（开发模式亦可用同源代理 `http://localhost:3000/api`）

通用响应：成功为 `{"success": true, "data": ...}`（个别管理端点如 DELETE 仅返回 `{"success": true}`）；失败为 `{"success": false, "error": "..."}`。

## 端点汇总

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| POST | `/daily-reports/generate` | 手动触发日报生成（异步） |
| GET | `/daily-reports/today?date=YYYY-MM-DD` | 阅读台队列：指定日期已有日报的版块清单（每板一行轻量投影） |
| GET | `/daily-reports/:id` | 查询单篇日报详情 |
| POST | `/daily-reports/:id/rebucket` | 重新分桶：对某 section 所在的泳道内主题桶组重跑两阶段分桶 |
| GET | `/semantic-boards/:id/daily-reports` | 查询板块日报列表 |
| GET | `/semantic-boards/:id/section-timeline` | 板块 section 时间线 |
| GET | `/semantic-boards/:id/topics` | 列出板块全部持久话题（含归档/孤儿）+ section 计数 |
| GET | `/semantic-boards/:id/lane-dynamics?days=14` | 板块泳道动态聚合（活跃泳道卡+发展时间线+候选栏，单请求；事件线数/lane_heat 口径，add-event-story-heat） |
| GET | `/semantic-boards/overview` | 今日雷达：全部活跃版块的事件线密度、story_heat 升温口径与最新日报摘要（单请求，5s 降级；add-event-story-heat 起关注度=lane_heat；surface-story-events-in-radar 起顶层 `stories` 段为跨版块事件线清单；matrix-clue-fallback-and-rail-merge 起收敛为单清单「今日该读」（≤8）+ 行内徽标） |
| GET | `/daily-reports/sections/:id/lifecycle` | section 连通分量生命周期 |
| GET | `/daily-reports/topics/:id/lifeline` | 持久话题全量 section |
| GET | `/daily-reports/:id/thread-relations?days=7` | 该期日报每条线索的相关线索推荐（同话题、窗口内、thread 向量余弦 ≥0.75；单请求覆盖全期，replace-lane-lifeline-with-topic-events） |
| PATCH | `/daily-reports/topics/:id` | 更新话题标题/状态 |
| DELETE | `/daily-reports/topics/:id` | 硬删话题（解绑 section） |
| POST | `/daily-reports/topics/:id/merge` | 合并源话题到目标 |
| POST | `/daily-reports/topics/:id/split` | 拆分 section 为新话题 |
| PATCH | `/daily-reports/topics/:id/anchor` | 人工重设泳道首义（选定节点范围；lane-manual-correction-loop） |
| GET | `/daily-reports/topics/:id/anchor` | 读当前泳道首义状态（来源 + 节点集；纯读） |
| GET | `/daily-reports/topics/:id/anchor/history` | 首义改写完整历史（append-only 留痕，按时间升序；纯读） |
| PATCH | `/daily-reports/sections/:id/assignment` | 泳道节点归属变更（`topic_id` 为 null = 移除，为 id = 加回） |
| GET | `/daily-reports/sections/:id/assignment/history` | 节点归属变更完整历史（append-only 留痕，按时间升序；纯读） |
| POST | `/daily-reports/topics/:id/detach-date` | 批量移除某泳道某一天的全部归属 |
| GET | `/daily-reports/:id/annotations` | 日报页边注批注列表（含问答轮） |
| POST | `/daily-reports/:id/annotations` | 落锚批注 |
| DELETE | `/annotations/:id` | 删除批注（连带问答） |
| POST | `/annotations/:id/questions` | 批注提问/追问（SearXNG 联网补强） |
| GET | `/annotations` | 跨报告管理页列表（版块/关键词筛选） |
| POST | `/daily-reports/backfill-embeddings` | 回填 section 向量 |
| POST | `/daily-reports/backfill-relations` | 回填话题关系 |
| POST | `/daily-reports/backfill-topics` | 历史重建持久话题 |
| POST | `/daily-reports/recompute-topic-centroids` | 按当前公式重算话题质心 |
| GET | `/semantic-boards/:id/persistent-topics/compose-candidates` | 手动编排候选 section |
| POST | `/semantic-boards/:id/persistent-topics/embed-query` | 自然语言向量化 |
| POST | `/semantic-boards/:id/topic-watches` | 新建话题盯盘 |
| GET | `/semantic-boards/:id/topic-watches` | 列出盯盘规则 |
| PATCH | `/topic-watches/:id` | 更新盯盘 label/status |
| DELETE | `/topic-watches/:id` | 删除盯盘规则 |
| GET | `/daily-reports/:id/watch-hits` | 查询日报命中盯盘 |
| GET | `/daily-reports/:id/key-events` | 日报关键事件读端（推荐状态 + 全部注记行） |
| GET | `/daily-reports/topics/:id/key-events` | 话题已确认关键事件（总览回看；仅 confirmed） |
| PUT | `/daily-reports/topics/:id/key-events/threads/:thread_id` | thread 寻址人工动作（mark/dismiss） |
| PATCH | `/daily-reports/topics/:id/key-events/:marker_id` | 注记寻址维护（mark/dismiss/unmark，含失效快照取消） |

## POST `/daily-reports/generate`

手动异步触发日报生成。`board_id` 为空时生成所有当日有事件标签的板块。

作用域参数二选一（add-radar-pipeline-status）：

| 参数 | 语义 |
| --- | --- |
| `board_id` | 只生成该板块（既有语义） |
| `board_ids` | 只生成该集合内的板块；缺省或空数组 = 全部板块 |

`board_id` 与 `board_ids` **互斥**：同时给出返回 `400`，不启动管线、不取执行闸。`board_ids` 内重复 id 折叠（保留首次出现顺序，每板至多生成一次）；任一字段类型非法返回 `400`，**不得**降级为全量作用域。集合生成共享同一把执行闸，一次请求只占一个闸、只返回一个 `job_id`、只广播一次终态、只发一条终态通知；集合内单板失败不中断其余板块。

Request：

```json
{ "date": "2026-05-26", "board_id": 2849 }
```

```json
{ "date": "2026-05-26", "board_ids": [2849, 2850, 2851] }
```

Response `data`：

```json
{ "job_id": "...", "status": "processing" }
```

终态消息（`daily_report_done`）携带**三态计数**：`total_saved`（产出日报的板块数）/ `failed_count`（真失败）/ `empty_count`（当日无内容），恒有 `total_saved + failed_count + empty_count == total_boards`（`total_boards` 含当日无内容的板块）。消费方 SHALL 直读 `failed_count` / `empty_count`，不得用 `total_boards - total_saved` 反推失败数（那会把当日无内容的板块错算成失败）。

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

**重建窗口守卫（offline-catchup）**：`date` 早于标签边保留窗口下界（`ai_settings.tag_edge_retention_days`，默认 7 天，即 `date` 早于保留窗口下界日（本地日历天零点 − N 天））时返回 **4xx**，错误消息说明「标签边已按 N 天窗口回收、超窗日期候选不全，拒绝重建」——防止超窗日期的空报告覆盖既有好报告（同日重建是整份覆盖语义，见 `flow/daily-report.md` 业务约束 20）。窗口内日期（含恰好等于下界）不受影响，照常异步触发。窗口口径与 `aux_label_cleanup` 的边 GC、日报自动补档扫描同键同口径。

**防重入闸（already_running）**：日报生成管线全局互斥（调度/手动同一把域闸，见 `flow/daily-report.md` 业务约束 26）——已有生成在跑（定时、`TriggerNowWithDate` 或本端点）时返回 **409**：

```json
{ "success": false, "error": "日报生成正在执行中，请稍后再试。", "reason": "already_running" }
```

调度侧同闸获取失败则记 skip（`skipped: "already_running"`，不标 failed、不发第二条终态通知）。窗口校验在闸检查之前：超窗日期不碰闸直接 4xx。

## GET `/daily-reports/today`

阅读台左栏队列：返回**指定日期已有日报的版块**清单，每板一行轻量投影（不含 sections / threads 载荷）。纯读、无副作用、不触发生成。

**参数：**

- `date`：`YYYY-MM-DD`，缺省取服务端本地日历当天；格式非法 → `400`（不回落默认日期）。

Response `data`（按 `semantic_board_id` 升序）：

```json
{
  "success": true,
  "data": {
    "date": "2026-11-03",
    "reports": [
      { "report_id": 1284, "semantic_board_id": 2849, "period_date": "2026-11-03", "title": "日报 2026-11-03", "status": "completed" }
    ]
  }
}
```

| 字段 | 说明 |
| --- | --- |
| `date` | 生效的查询日期（回显；未传时即服务端今天） |
| `reports[].report_id` | 报告主键（可交给 `GET /daily-reports/:id` 取详情） |
| `reports[].semantic_board_id` | 版块 ID |
| `reports[].period_date` | 报告期日（`YYYY-MM-DD`，`period_date` 列投影） |
| `reports[].title` | 报告标题（生成时写成 `日报 <日期>`） |
| `reports[].status` | 报告行 `status` 列原值（生成成功落库的日报恒为 `completed`） |

格式非法时 `400 { "success": false, "error": "invalid date" }`；查询失败 `500`。当天无任何日报时 `reports` 为 `[]`（不是 `null`，也不是 404）。`(semantic_board_id, period_date)` 唯一索引保证每板当日至多一行。

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

Response 的 `sections[].topic_match_distance` = 该 section 到话题质心的归属距离（float，**恒下发**）：距离恰为 `0` 是合法结果（完美锚定），不得被省略（fix-lane-metric-provenance）——省略会让客户端把接近度读成「未知」而不是 `1 − 0 = 1.00`。

## GET `/daily-reports/:id/thread-relations?days=7`

给日报阅读台的**每条当日线索**返回「相关线索」推荐，用于线索行内的 chip（replace-lane-lifeline-with-topic-events）。一次请求覆盖整期日报全部线索，按 `thread_id` 分组——不要求前端逐条发请求。

**参数**：`days` 只接受 `7` 与 `30`，其它值回落 `7` 并以响应字段回显实际生效值；窗口为 `[report_date - days, report_date - 1]`，**不含当前期**（当期线索已在同一屏可见，不推荐自己）。

**候选与打分**：同 `persistent_topic_id`、窗口内、`embedding` 非空的其它 thread，按 `1 - (a <=> b)` 余弦相似度筛选，**只有 ≥ 0.75 进入列表**；`score` 就是余弦值本身，**不叠加时间衰减/篇数/confirmed 加权**；`level = score >= 0.85 ? "strong" : "related"`（两档）。排序 `score` desc → `period_date` desc → `thread_id` asc，**后端返回全量不截断**（前端只渲染前 3）。

**为什么不用事件标签做匹配**：实测「事件标签 id 交集」有候选的线索仅 1.6%（标签 id 不跨天复用 + 孤儿清理硬删）；「事件标签向量」的 top-3 与 thread 向量**零重叠**，候选池只有 19%。选型实测见 change 的 `design.md` D3。

**响应**（`data` 下）：

| 字段 | 说明 |
| --- | --- |
| `report_id` / `days` / `window_start` / `window_end` | 参数与实际窗口 |
| `status` | `ready` / `partial`（有当期线索缺向量被跳过）/ `empty`（无任何推荐） |
| `threads` | `{ "<threadId>": [ { thread_id, title, summary, period_date, score, level, confirmed, source_article } ] }`，无推荐时为空对象 `{}`（不是 `null`） |
| `total` | 全部推荐条数 |
| `skipped_no_embedding` | 当期缺 embedding 被跳过的线索数 |

`source_article` 取候选 thread 的 `related_article_ids` **首篇**（`id` / `title` / `feed` / `published_at`），查不到时为 `null`（不编造标题）。`confirmed` 表示候选 thread 的 event 标签在本话题有 `state='confirmed'` 的关键事件注记。

**只读**：不写任何表/缓存/索引、不调模型、不算 embedding。`reportId` 不存在返回 404。

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

## 泳道人工修正（lane-manual-correction-loop）

> 四个写 / 读端点 + 两个只读历史端点共同构成「人工修正闭环」：首义只由**选定节点范围**定义（不收自然语言描述），归属变更是双向的（移除 / 加回，不接受改挂），一切修正只写**事实层**，派生缓存（质心 / 吸尘器标记 / 泳道快照）由算法在**下一次日报生成**统一重算——不存在「立即重算」端点。

### PATCH `/daily-reports/topics/:id/anchor`

人工重设泳道首义（`board_persistent_topics.embedding`）：把用户选定的节点范围按 mean pooling 聚合（与手动建话题同口径）写成新首义。

Request：

```json
{ "section_ids": [101, 105, 110] }
```

- `section_ids` 必填：选定的泳道节点（section）id 列表；空数组 → `400`。
- **只接受节点 id 列表**：传自然语言描述字符串 → `400`（首义只有一种输入模式，不做向量化）。
- 每个选中节点必须同时满足：属于同一 board、当前归属该话题、embedding 存在且维度一致；任一不满足 → **整体拒绝**（不静默跳过、不落库）。

Response `data`：

```json
{
  "topic_id": 7,
  "source": "selected_sections",
  "selected_sections": [101, 105, 110],
  "override_count": 2,
  "effective_from": "next_report"
}
```

- `source` 恒为 `selected_sections`（首义的输入模式）。
- `selected_sections` = 去重后的选定节点 id。
- `override_count` = 该话题首义被人工改写的累计次数（`topic_anchor_overrides` 行数）。
- `effective_from` 恒为 `next_report`：事实层立即改，匹配锚点（质心）由下一次日报生成重算。

副作用（单事务）：写 `board_persistent_topics.embedding` + 追加一行 `topic_anchor_overrides` 留痕（改前向量 / 改后向量 / 输入模式 / 选定节点 id / 操作者 / 时间，只追加）+ 置 `centroid_pending_recompute=true`。**不改写质心**（`centroid` 列只由下一次日报生成刷新）。

错误码：

| 状态 | 条件 |
| ---- | ---- |
| 400 | body 不是 id 数组 / `section_ids` 缺失或为空 / 节点不存在 / 节点属其它版块 / 节点不属该话题 / 节点无可用向量 / 节点向量维度与其它节点不一致 |
| 404 | 话题不存在 |
| 409 | 话题已归档（archived 泳道不可改首义） |

### GET `/daily-reports/topics/:id/anchor`

读当前泳道首义状态。**纯读端点**：不产生任何写入、不重算任何派生值。

Response `data`：

```json
{
  "topic_id": 7,
  "source": "manual",
  "section_ids": [101, 109],
  "override_count": 2,
  "effective_from": "next_report"
}
```

- `source`：`auto` = 首义是创建话题时写入的（无节点集）/ `manual` = 首义由人工选定的节点范围定义。
- `section_ids`：当前首义节点集（取 `topic_anchor_overrides` 最新一行反解，可能含当前取数窗口之外的节点）；`source=auto` 时**恒为空数组**——自动首义只有一条向量，没有对应节点列表，接口不得虚构。
- `override_count`：人工改写次数。
- `effective_from`：`manual` 为 `next_report`；`auto` 为空串（已在用）。

错误码：`404` 话题不存在；`500` 读取失败。

### GET `/daily-reports/topics/:id/anchor/history`

首义改写的**完整历史**（只读）。`GET .../anchor` 只回答「当前首义是什么」，本端点回答「这条泳道的定义面什么时候被谁改过」。

留痕表 `topic_anchor_overrides` 是 append-only 的：每次人工改写追加一行，既有行既不改也不删。

Response `data`：

```json
{
  "topic_id": 7,
  "overrides": [
    {
      "id": 1,
      "topic_id": 7,
      "source": "selected_sections",
      "section_ids": [101, 105, 110],
      "old_vector": { "dimension": 768, "head": [0.0123, -0.0456], "norm": 1.02 },
      "new_vector": { "dimension": 768, "head": [0.0311, -0.0222], "norm": 1.04 },
      "operator": "user",
      "created_at": "2026-11-15T09:00:00Z"
    }
  ]
}
```

- `overrides` 按**时间升序**返回（`created_at ASC, id ASC`——同毫秒追加的行靠 id 兜底排序）。
- `old_vector` / `new_vector` 是向量**摘要**（维度 + 前 8 个分量 + 模长），不是整条向量；改前无向量时 `old_vector` 为 `null`。
- `section_ids` = 该次改写选定的节点 id；摘要读不出来时为空数组（不虚构节点列表）。
- `operator` = 操作者（单用户部署恒为 `user`）。
- 留痕**不随话题硬删而消失**：话题已被硬删但留痕仍在时，本端点照常返回留痕。

错误码：`404` 话题不存在且无留痕；`500` 读取失败。

### PATCH `/daily-reports/sections/:id/assignment`

泳道节点归属变更，双向共用一个端点。移除不指定去处（用户表达的是「它不该在这儿」），加回接受当前无归属的节点；**已归属的节点只在来源泳道为 candidate（动态）时允许改挂**（来源为 active 已转正泳道 → `409`，已转正泳道的素材不因人工编辑而流动）。

Request（移除）：

```json
{ "topic_id": null }
```

Request（加回）：

```json
{ "topic_id": 7 }
```

- `topic_id` 必填且必须显式给出：`null` = 移除，正整数 = 加回到该话题；字段缺失 / 字符串 / 负数 / 0 → `400`。

快照口径（两向都保留 `lane_tier`）：

| 方向 | 写入列 |
| ---- | ------ |
| 移除（detach） | `persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `topic_status_at_report` 四列置 NULL |
| 加回（attach） | `persistent_topic_id` = 目标话题、`topic_match_confidence` = `manual`、`topic_match_distance` = 到目标锚点（centroid 优先、embedding 兜底）的实算距离（节点无可用向量时写 NULL）、`topic_status_at_report` = NULL |
| 改挂（来源为 candidate 泳道） | 同「加回」，但 `manual_assignment_from_topic_id` = 来源泳道；留痕追加 `from=来源话题, to=目标话题`；来源与目标**两条泳道**均置 `centroid_pending_recompute=true` |

两向都写留痕三列：`manual_assignment_at`（时间）/ `manual_assignment_from_topic_id`（变更前所属话题）/ `manual_assignment_action`（`detach` / `attach`）。这三列的语义是「**最近一次**人工归属变更的快照」——下一次人工变更会覆盖它们；**完整历史**在 append-only 的 `section_assignment_overrides` 表（见下方历史端点）。

Response `data`：

```json
{
  "section_id": 42,
  "topic_id": 7,
  "previous_topic_id": null,
  "action": "attach",
  "changed": true,
  "topic_match_distance": 0.183,
  "board_id": 3
}
```

- `action`：`detach` / `attach`。
- `changed`：是否真的写了行；「已属该话题」的幂等加回返回 `changed=false`（不写列、不追加留痕）。
- `board_id`：被重建关系图的版块。

副作用（单事务）：置受影响话题 `centroid_pending_recompute=true` + 调 `RebuildBoardRelations(tx, board_id)`；重建失败整体回滚（归属与留痕一并撤销）。**不动**话题的命中计数与状态（`hit_count` / `consecutive_hits` / `status` / `first_seen_date` / `last_seen_date` 均不变）。

错误码：

| 状态 | 条件 |
| ---- | ---- |
| 400 | body 非法（`topic_id` 缺失 / 非数字 / 非正数 / 字符串）/ 目标泳道属其它版块 |
| 404 | section 不存在 / 目标话题不存在 |
| 409 | 该节点无归属可移除 / 该节点已属其它泳道（改挂被拒）/ 目标泳道已归档 |
| 500 | 其它失败（含关系重建失败 → 整体回滚） |

### GET `/daily-reports/sections/:id/assignment/history`

节点泳道归属变更的**完整历史**（只读）。「移除 → 加回」两步各留一行：加回不会把先前的 detach 痕迹抹掉。

留痕表 `section_assignment_overrides` 是 append-only 的：每次人工归属变更追加一行，既有行既不改也不删。

Response `data`：

```json
{
  "section_id": 501,
  "entries": [
    {
      "id": 1,
      "section_id": 501,
      "topic_id_from": 8,
      "topic_id_to": null,
      "action": "detach",
      "operator": "user",
      "created_at": "2026-11-15T09:00:00Z"
    },
    {
      "id": 2,
      "section_id": 501,
      "topic_id_from": null,
      "topic_id_to": 8,
      "action": "attach",
      "operator": "user",
      "created_at": "2026-11-15T09:05:00Z"
    }
  ]
}
```

- `entries` 按**时间升序**返回（`created_at ASC, id ASC`）。
- `topic_id_from` = 变更前所属话题（`null` = 此前无归属）；`topic_id_to` = 变更后所属话题（`null` = 移除）。
- `action` = `detach`（移除）/ `attach`（加回）。
- 留痕**不随节点硬删而消失**：节点已被硬删但留痕仍在时，本端点照常返回留痕。

错误码：`404` 节点不存在且无留痕；`500` 读取失败。

### POST `/daily-reports/topics/:id/detach-date`

批量移除某泳道某一天的全部归属（逐条同「移除」口径，事务末尾只重建一次整板关系）。

Request：

```json
{ "period_date": "2026-11-10" }
```

- `period_date` 必填，`YYYY-MM-DD`；格式非法 → `400`。

Response `data`：

```json
{
  "topic_id": 7,
  "period_date": "2026-11-10",
  "detached": 3,
  "section_ids": [42, 43, 44],
  "board_id": 3
}
```

- `detached` = 被释放的节点数；该日无归属节点时返回 `detached: 0` 与空 `section_ids`（不是错误）。
- `section_ids` = 被释放的节点 id（升序）。

错误码：`400` `period_date` 格式非法；`404` 话题不存在；`500` 其它失败（含关系重建失败 → 整体回滚）。

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

## GET `/semantic-boards/overview`

今日雷达单次聚合（rework-workshop-into-radar-and-desk 8.2；retire-story-adopt-event-metrics 1.5/3.2 换当天线索口径）：只读，返回所有 `status=active` 版块的行。**关注度 `attention` = 该版块各泳道当天热度之和（含候选泳道按 `lane_heat_candidate_weight` 加权，保留一位小数）**——泳道当天热度 = 当天线索数按话题状态加权（`heat-v2-lane-clues-1d`：active × 1.0 + candidate × 权重），只表达当天、不衰减不跨天；不再是事件线热度（`heat-v1-48h-hl24h`）之和，也不是旧 section 数加权公式。分档阈值按新关注度真实分布标定（前端 14.5 / 2.3 = P80/P20，实测记录见 change design D9），不沿用旧 3.3/0.7 或 50/15。

字段变更（相对事件线口径）：**移除** `today_sections` / `last3_sections` / `prev3_sections` / `days` / `today_stories` / `last3_stories` / `prev3_stories` / `today_heat` / `last3_heat` / `prev3_heat` / `heat_days` / `stories_backfilled` / `story_identity_state` / `heat_incomplete` / `heat_stale_feed_ids`；**新增** `today_clues` / `last3_clues` / `prev3_clues`（同轴窗口的当天线索合计）与 `clue_days`（14 天逐日线索合计，oldest → newest，未知不补 0）、`lanes[].clue_counts`（14 天逐日线索合计）；`heat_rule_version` 恒 `heat-v2-lane-clues-1d`。泳道移除 `counts` / `story_counts` / `section_count_14d`，保留 `thread_count_14d`（全窗线索合计，排序次键）。泳道与分区口径复用 `lane-dynamics`，最新日报取 `board_daily_reports`；无数据版块返回零值行（**真实 0**），单版块超时未完成或泳道动态计算失败则该行标 `data_unavailable=true` 且计数类字段落 `null`（**未知**，不造 0），不阻塞整屏。窗口锚定该版块最新日报日（无日报则无密度），`date` 为全部版块最新日报日的最大值（**全部版块都未取到时为空串**，不冒充今天）。

```json
{
  "date": "2026-11-10",
  "boards": [{
    "board_id": 1974,
    "label": "中东地缘政治与美伊关系",
    "tag_count": 144,
    "report_date": "2026-11-10",
    "report_title": "伊朗警告打击海湾海底电缆",
    "report_article_count": 14,
    "report_event_count": 13,
    "anchor_date": "2026-11-10",
    "today_clues": 7,
    "last3_clues": 13,
    "prev3_clues": 10,
    "attention": 7.5,
    "heat_rule_version": "heat-v2-lane-clues-1d",
    "clue_days": [0, 1, 2, 1, 0, 3, 1, 2, 0, 1, 2, 1, 0, 7],
    "data_delayed": false,
    "data_unavailable": false,
    "lanes": [{
      "topic_id": 1204,
      "label": "美伊博弈升级：从经济威胁到全面孤立",
      "thread_count_14d": 38,
      "clue_counts": [0, 1, 1, 0, 0, 2, 1, 1, 0, 1, 1, 0, 0, 3]
    }]
  }],
  "top_lanes": {
    "anchor_date": "2026-11-10",
    "lanes": [{
      "board_id": 1974,
      "board_label": "中东地缘政治与美伊关系",
      "topic_id": 1204,
      "topic_label": "美伊博弈升级：从经济威胁到全面孤立",
      "clue_count": 3,
      "anchor_date": "2026-11-10"
    }],
    "partial": false,
    "degraded_board_ids": [],
    "lagging_board_ids": []
  }
}
```

- `attention` 为 float（Σ 泳道当天热度，1 位小数）；行按 attention 降序（平手 board_id 升序）；`attention` 未知（未取到）的行沉到末尾。
- `today_clues` / `last3_clues` / `prev3_clues` / `clue_days` / `lanes[].clue_counts` 缺省（旧后端）时归一为 `null`，前端标「线索数未知」，**不造 0、不拿分区数冒充**；`clue_count` 已排除综合稿（`scope=composite`）。
- **`data_unavailable`**（fix-radar-overview-false-zero）：true = 该版块**这次没取到**（端点超时未完成，或该版块泳道动态计算失败）。此时 `attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count` 均为 `null`（未知）。前端 SHALL 把这些行显示为「数据未取到」，SHALL NOT 当成「0」渲染成「今天没有热点 / 0 篇文章」；这些行也不参与关注度分档与「N 个板块值得关注」计数。**真实零值**（近 14 天无日报无泳道、取数成功）仍是 `data_unavailable=false` + 计数为 0。
- `data_delayed` 保留原义（端点超时降级标记）；计算失败路径不标 `data_delayed`，只标 `data_unavailable`。
- 滞后版块（自身锚点日 ≠ 全局 `date`）的计数照旧返回实数，但前端 SHALL 把它们排在主列表末尾（不进分档、不占前排名次）。
- `heat_rule_version` 恒 `heat-v2-lane-clues-1d`（与 lane-dynamics 同源）；旧热度字段不再下发。

### 顶层 `top_lanes` 段（当天线索最多的泳道，D10）

响应新增**顶层 `top_lanes` 段**：雷达右栏清单的**条目单位是泳道**（版块 + 话题），不是事件线、也不是版块日报标题。跨版块归并、排序（泳道当天线索数降序 → 版块名 → 泳道名 → 泳道 id）、截断（**上限 8**，第 9 名不展示）全部在服务端完成，客户端不做二次合并或重排。**该段缺失** = 旧后端响应，客户端 SHALL 回退既有的版块级日报标题列表（不渲染成业务空态）。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `anchor_date` | string | 全局锚点日（= 响应 `date`），仅用于「N 个版块滞后」提示 |
| `lanes[]` | array | 条目：`board_id` / `board_label` / `topic_id` / `topic_label` / `clue_count`（该泳道在**它自己版块锚点日**的线索合计）/ `anchor_date`（条目所属版块自己的锚点日） |
| `partial` | bool | true = 部分版块超时或泳道数据计算失败，本次排序不完整 |
| `degraded_board_ids` | array[int] | 降级版块 id（不产条目） |
| `lagging_board_ids` | array[int] | 自身锚点日落后于全局锚点日的版块 id（仍照常出条目，按自己的日期计） |

- 当天线索为 0 的泳道不进清单；清单为空时前端渲染空态（不得说成「今天没有事件」）。
- 事件线清单段（`stories` / `unbackfilled_board_ids` / `warming_count` / `cross_day_story_count` 等）随事件线退役整体移除；条目不再提供 `?story=` 深链，点击进既有控制台 `?lane=`。

### 消费方 ②：L1 版块态势页左轨（console-rails-live-signals，2026-11-19）

同一份 overview 响应也是版块态势页（`/tags/:boardId`）**左轨版块栏**的数据源：每行 = 名称 + 14 格逐日线索条码，格值取 `boards[].clue_days`，色阶按行内峰值归一。

> ⚠️ **共同轴对齐（消费方必须做，否则是假比较）**：本端点的 `clue_days` 由 `radarAxis(anchor)` 按**每个版块自己的锚点日**生成 14 天窗口（`backend-go/internal/topicgraph/repository/board_radar_repository.go:433,501-510`），因此不同版块的「最后一格」代表**不同日期**。消费方 MUST 按**全局锚点**（响应 `date`）重对齐再并列展示：
> `offset = daysBetween(date, boards[i].anchor_date)`；`clue_days[j]` 在共同轴 `[date-13 … date]` 上的位置 = `j - offset`；位置越界 = **该位未取到**（MUST 画未取到占位，MUST NOT 补 0）。对齐后「停更」（末段未取到）与「安静」（真实 0）才可区分。
>
> `boards[].data_unavailable = true` 的行，其计数类字段为 `null`（未知），消费方 MUST 显示未取到，MUST NOT 当成 0。`boards[].anchor_date` 落后的行（滞后版块）按各板自己的日期表达，不得冒充全局锚点日。

**口径不新增**：本端点的 `clue_days` / `today_clues` / `last3_clues` / `prev3_clues` 口径与 §字段变更处一致，本节只声明第二个消费点与共同轴对齐规则。

## GET `/semantic-boards/:id/lane-dynamics?days=14`

板块泳道动态聚合：「板块内容」tab 首屏泳道动态视图的单请求数据源（活跃泳道卡 + 滚动态势 + 发展时间线 + 候选栏）。取代已退役的 topic-landscape 端点。

**参数：**

- `id`：semantic_board_id。
- `days`：时间线与统计窗口天数，默认 14；非正值或非法值回落 14。窗口锚定该板块最新一份**已完成**日报的 `period_date`（`[anchor-(days-1), anchor]` 双端闭区间，非 now()——避免日报未跑时窗口漂移；未完成日报（`status != 'completed'`）不作锚点，其分区也不进窗口内容，与 compose-candidates 窗口同口径，fix-lane-metric-provenance）。

**泳道集合与排序**：`lanes` = 该板块 active 泳道 ∪ watch 关联泳道，按 **`lane_heat` 降序（平手依次全窗线索合计 `thread_count_14d` 降序、`topic_id` 升序）**；近窗口无 section 的 active（沉寂）不返回；candidate 不进 `lanes`（只在 `candidates`）。

**量纲（retire-story-adopt-event-metrics 1.4）**：`lane_heat` = 当天线索数按话题状态加权（`heat-v2-lane-clues-1d`，active × 1.0 + candidate × `lane_heat_candidate_weight`）；逐日 `clue_count` = 该泳道该日各主题桶**非综合稿** thread 数之和（矩阵日格 / 演进节点 / 顶部曲线唯一量纲）。事件线量纲（`story_count` / `max_participant_count` / `hottest_story_id` / `hottest_story_title` / `unassigned_event_count` / `story_count_14d` / `stories_backfilled`）与热度元数据 `heat_incomplete` / `heat_stale_feed_ids` 整体移除。

Response `data`：

```json
{
  "window_days": 14,
  "has_reports": true,
  "anchor_date": "2026-09-09",
  "heat_rule_version": "heat-v2-lane-clues-1d",
  "lanes": [
    {
      "topic_id": 12,
      "label": "芯片战",
      "watch_linked": true,
      "section_count_14d": 9,
      "thread_count_14d": 27,
      "lane_heat": 6.8,
      "snapshot": {
        "summary": "近两周围绕出口管制升级与国产替代进展交替演进……",
        "as_of": "2026-09-09"
      },
      "timeline": [
        {
          "date": "2026-09-08",
          "clue_count": 7,
          "sections": [
            {
              "section_id": 341,
              "label": "新规生效首日影响",
              "thread_count": 2,
              "composite_count": 0,
              "events": ["厂商A暂停部分订单", "替代供应商订单量抬升"],
              "folded_count": 0
            }
          ]
        }
      ]
    }
  ],
  "candidates": [
    {
      "topic_id": 55,
      "label": "稀土出口博弈",
      "last_seen_date": "2026-09-08",
      "recent_hint": "稀土出口博弈：管制清单扩容传闻",
      "anchor_date": "2026-09-08",
      "clue_count": 2,
      "lane_heat": 1.0
    }
  ]
}
```

- `heat_rule_version` 恒 `heat-v2-lane-clues-1d`（与 overview 同源）。
- `lanes[].lane_heat`：该泳道当天线索数按话题状态加权（含候选权重；无线索为 0）——排序唯一热度输入，section/thread 计数不得参与。`lanes[].thread_count_14d`：全窗线索合计（排序次键）；`section_count_14d` 保留展示兼容。
- `timeline[].clue_count`：当日线索合计（**已排除综合稿**）；矩阵日格/演进节点/曲线统一消费它；展示值 MUST NOT 用各桶 `thread_count` 合计替代（section 的 `thread_count` 未排除综合稿，只能作 aria 辅助/旧响应回退）。
- `timeline[].sections[].events`：该 section 的 thread 标题，最多前 5 条；`folded_count` 为被截断数量（恒输出，0 也带）。`timeline` 按日期倒序（最新日在最前）。
- `timeline[].sections[].composite_count`（fix-lane-metric-provenance）：该桶**综合稿**条数（thread 的 event 标签数 ≥ 2）。桶级线索数 = `thread_count − composite_count`（客户端唯一公式；两者缺任一 → 该桶线索数未知，显示 `?`）。客户端 SHALL NOT 把 `thread_count` 当线索数——它是含综合稿的 thread 总数。
- `anchor_date`：窗口锚点日 = 该板块最新**已完成**日报的 `period_date`（YYYY-MM-DD）；无已完成日报时为空串。前端矩阵轴末日与泳道工作面窗口末日统一取它（compose-candidates 的 `window.latest_completed_date` 同值）。
- `candidates`：达可见门槛（`FilterVisibleTopics`）的 candidate + 最新 section 标题 `recent_hint`；新增 `anchor_date`（该版块最新日报日）、`clue_count`（锚点日当天线索合计）、`lane_heat`（= clue_count × 候选权重，已含在版块 `attention` 内，前端勿重复相加）；只读提示，转正走话题管理入口。
- `snapshot`：滚动 14 天态势（每日日报生成后异步结算，见 `flow/daily-report.md` §泳道态势结算）。`detail` 不再产出（存量历史长版仅随下个日报日结算覆盖自愈）。
- 缺失语义：`snapshot: null` = 快照整体缺失（新泳道或结算未跑），前端降级「待结算」占位、时间线照常渲染。
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
        "lane_tier": "l1_direct",
        "bucket_title": "工具调用与编排",
        "bucket_index": 0,
        "threads": []
      }
    ]
  }
}
```

当 section 已归属持久话题时，详情响应会附带轻量 `persistent_topic` 描述，供前端按
`topic_status_at_report` 快照分区（`active` / `candidate` / `null`）。历史未归属 section 返回 `null`。

> **threads 的 display_tier 读取口径（add-event-story-heat）**：thread 落库携带 `display_tier`（`body`=正文默认 / `flash`=快讯（跨期重述降级） / `hidden`=预留）；本端点的 threads 只返回 **`body`** 行——`flash`/`hidden` 本期丢弃不渲染（快讯 UI 是未来工作，列仍可查）。降级只影响展示：flash 行照常落库、signals 照常写、热度不变（详见 `flow/daily-report.md` 约束 44/45）。

> **`related_article_ids` 的折叠语义（dedupe-thread-article-copies）**：本端点返回的每条 thread 的 `related_article_ids` 是**读路径投影**——同一篇内容被两个 feed 各抓一份（同 `link`）时只保留**首个出现**的一条，数组顺序不变；`link` 为空（或归一后为空）、以及库里查不到的 id **不参与折叠**、原样保留。归一口径 = `textutil.NormalizeArticleLink`（去 fragment、去 query、scheme/host 小写），只认「归一 link 相同」这一条信号——同标题不同 URL 的多源转载**不折叠**。落库值 `daily_report_threads.related_article_ids` 不变（仍是该 thread `tag_ids` 对应文章的并集），故响应数组长度可能短于落库值；折叠所需的 link 取不到时降级为原样返回（本端点仍 200）。详见 `flow/daily-report.md` 约束 45。

### 消费方 ②：L1 版块态势页右轨「当日日报」（console-rails-live-signals，2026-11-19）

本端点的 `report.highlights` / `report.sections[]` 也是版块态势页右轨「当日日报」块的数据源（**跟随矩阵选中日**，右轨不提供日期选择器）：

- **点评层** = `report.highlights`（2–3 条，元素字段见 `highlights` 字段契约）。消费方 MUST NOT 同时展示 `report.title` / `report.summary`——二者由后端派生自**首条重点**（`title = highlights[0].Title`、`summary = highlights[0].Reason`，`repository/report_story_highlights.go:78-81`），并排即重复渲染同一条。`highlights[].article_count` 缺省时消费方 MUST NOT 显示「N 篇来源」，MUST NOT 补 0。
- **泳道层** = `report.sections[]` 中 `topic_status_at_report = 'active'` 且有 `persistent_topic` 的分区；代表线索取该分区首条 thread 的 `title`（无 thread 则退回 `cluster_label`）。该层的**线索数 MUST 取 `lane-dynamics` 的逐日 `clue_count`**（与矩阵日格同源同值），MUST NOT 用本端点 sections 内的 thread 条数（含综合稿，另一套量纲）；按线索数降序展示，并列时保持本端点 sections 原序。`topic_status_at_report = 'candidate'` 的分区（其他动态）不进该块。

**口径不新增**：本端点响应结构不变，消费方只是把此前被 normalizer 丢弃的 `title` / `summary` / `highlights` 字段接上（前端补字段，零后端改动、零新增请求）。

## POST `/daily-reports/:id/rebucket`

对「某个 section 所在的泳道内主题桶组」（同一 `report_id` + `cluster_index`）重跑两阶段分桶，整组替换 section 行。属于 add-intra-lane-theme-bucketing。

Request body：

```json
{ "section_id": 7149 }
```

Response `data`：

```json
{
  "buckets": 6,
  "sections": [
    { "id": 7149, "cluster_index": 3, "bucket_index": 0, "bucket_title": "工具调用与编排", "cluster_label": "...", "threads": [] }
  ]
}
```

状态码三态：

| 状态 | 条件 | 语义 |
| ------ | ------ | ------ |
| 200 + `buckets ≥ 2` | 拆分成功 | 该组 section 已替换；组头 section 行 ID 保留（旧深链仍可用），其余桶可能新建/删除 |
| 200 + `buckets = 1` | 未达分桶阈值（threads<6 且 tags<10，可经 `ai_settings` 调）或校验后收敛为 1 组 | 合法「拆不动」，**不改任何数据**、不调用分桶 LLM |
| 409 | 报告 status 非 `completed`，或桶组在过程中被同日重跑替换（重试即可） | 不写库 |
| 400 | 目标 section 的 `lane_tier` 为 `watch_keyword` / `watch_sentence` | 物化组由关注轨自管，不接受主题分桶 |
| 404 | 报告不存在 / section 不属于该报告 | — |

服务端保证：与同日重跑（`SaveReport`）取**同键** `(semantic_board_id, period_date)` 事务级 advisory lock 串行化；thread 行只迁移 `section_id` 不删除（关键事件注记、文章引用不丢），`topic_key_event_markers.source_section_id` 按 `source_thread_id` 重派生、`topic_watch_hits.section_id` 与 `report_annotations.section_id` 同步重派生、该组跨天关系双向重建；派生字段（`cluster_label`/`bucket_title`/`bucket_index`/`cluster_tag_ids`/`article_count`/`embedding`/`topic_match_distance`/`best_tier`/`avg_score`/`quality_breakdown`）按新桶重算（`article_count` = 桶内线索 `related_article_ids` 的去重并集；`best_tier`/`avg_score`/`quality_breakdown` 取自报告期冻结快照按桶内 tag 子集重算），`topic_status_at_report` 沿用报告期快照；失败整体回滚。

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

## POST `/daily-reports/recompute-topic-centroids`

异步按当前公式与配置重算持久话题质心（`centroid = α·首义 embedding + (1-α)·滚动窗口均值`，anchor-topic-centroid-with-embedding）。用于公式/参数升级后的一次性存量纠正；幂等可重跑。

Query：

- `board_id` 可选，限定单个板块；缺省全部板块的非 archived（active+candidate）话题。非法值（非数字/≤0）返回 `400 {"error": "invalid board_id"}`。

无需请求体。行为：逐话题重算（首义不可用回退纯滚动均值+告警），单话题失败记日志不中断其余；完成写日志 `recompute topic centroids complete: N topics updated, M failed`。Response `data`：

```json
{ "status": "processing" }
```

## GET `/semantic-boards/:id/persistent-topics/compose-candidates`

为手动编排 UI（`POST /semantic-boards/:id/persistent-topics/manual`）加载候选 section（带可用向量）、**窗口内原关系**、**服务端口径的候选链**及配置的 `match_threshold`，前端据此实时计算成员均值质心与补充推荐。

Query：

- `days` 可选，时间窗天数；不传或 `<=0` 取全部历史，`>90` 截断到 90。窗口以该板块最新 completed 日报为终点（含首末报告日）。
- `all_members_topic_id`（可选，uint，修正模式专用）：传入时响应追加 `early_members` 数组——该话题成员中报告期早于窗口首日的 section（升序，与 `sections` 同形）；话题不属该版块 404；**不传时字段整个缺席**（编排画布行为不变）。来源 lane-manual-correction-loop 方案 A。**成员集合对称**（fix-lane-metric-provenance）：窗口内 `sections` 与窗口外 `early_members` 用**同一向量可用性判据**（都要求 `embedding IS NOT NULL` 且向量可解析）——因此「窗口内成员数 + 窗口外成员数 = 全部成员数」恒成立、窗口内集合恒是全部集合的真子集（无可用向量的成员两侧都不入集）。`early_members` = **窗口外**的本话题成员：档位窗口（`days`）下它们都在窗口首日之前；显式 `from`/`to` 且 `to` 早于最新 completed 日时，上界之后的成员也归入此集合（保证成员总数完整，不出现「两侧都不算」的成员）。
- `from` / `to` 可选，自选起止日（`YYYY-MM-DD`，**闭区间**）；两者必须成对给出，给出时忽略 `days`。校验（任一不满足 → `400`）：只给单边 / `from > to` / 跨度 > 90 天（**不静默截断**）/ `from` 早于「最新 completed 日报 − 89 天」取数窗 / 日期格式非法。`to` 晚于最新 completed 日报**允许**（窗口内本无更晚数据）。

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
  "relations": [
    { "from_id": 101, "to_id": 102, "distance": 0.0651, "relation_type": "identity" }
  ],
  "candidate_chains": [
    {
      "key": "1984:7345",
      "topic_id": 1984,
      "topic_label": "...",
      "topic_status": "candidate",
      "section_ids": [7345, 7409, 7514],
      "first_date": "2026-10-27",
      "last_date": "2026-10-30",
      "span_days": 3
    }
  ],
  "match_threshold": 0.3,
  "window": {
    "from": "2026-10-01",
    "to": "2026-10-30",
    "days": 30,
    "latest_completed_date": "2026-10-30"
  }
}
```

- `relations`：**窗口内两端都可见**的原关系全集（`identity` 与 `similarity` 都保留，`relation_type` 原样带出）。**只读、不新增边**。
- `candidate_chains`：**服务端唯一实现**的候选链口径（见 `flow/daily-report.md` 业务约束）；前端只渲染不重算。
- `window.days`：**生效**窗口天数（闭区间）。`days` 档：`>90` 回显 90、`<=0` 回显 0；`from`/`to` 档：回显 `to-from+1`。
- `window.from` / `window.to`：**可能为 `null`**（无已完成日报、或 `days<=0` 全历史档），表示开边界；`window.latest_completed_date` 在该板块**没有已完成日报**时为 `null`。

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

## 关键事件注记（Key Event Markers，add-topic-key-event-markers）

人工主权的话题关键事件：AI 只产出 `suggested` 建议，`confirmed`/`dismissed` 仅由用户动作产生；展示文本全部服务端快照派生，客户端只能表达动作与读到的 revision（未知字段忽略，**不接受客户端伪造快照**）。

### GET `/daily-reports/:id/key-events`

日报读端（打开报告页面只读拉取；不触发推荐生成）。返回推荐任务状态与该报告来源的全部注记行（三种 state 都可能出现在读端，由前端区分展示）。

Response `data`：

```json
{
  "status": "running",
  "markers": [ { "id": 11, "state": "suggested", "source_thread_id": 1001, "title_snapshot": "...", "recommendation_reason": "...", "revision": 1 } ]
}
```

- `status`：`idle`（无任务行）/ `running`（本进程内存活任务）/ `done` / `failed`。持久化 `running` 行但进程内无任务=重启杀掉了任务，诚实降级读为 `failed`（不自动恢复付费调用）。
- `markers[].state`：`suggested` / `confirmed` / `dismissed`；`recommendation_reason`/`recommendation_basis` 仅 AI 建议行携带。
- 报告不存在 → 404；id 非法 → 400。

### GET `/daily-reports/topics/:id/key-events?state=confirmed&days=14`

话题读端（泳道/焦点总览回看）。**只返回 confirmed**——`state` 查询参数省略或必须等于 `confirmed`，传其他值 400（总览不承担待处理状态，建议不泄漏）。

- `days`：默认 14；`0` = 不限窗；负数/非整数 → 400。
- 话题不存在 → 404。
- 窗口按 `source_period_date` 过滤；来源已失效（section/thread 被重建删除）的 confirmed 行仍返回（快照回看），前端以独立快照注记呈现、不伪造 section。

### PUT `/daily-reports/topics/:id/key-events/threads/:thread_id`

thread 寻址动作：确认建议 / 手动标记 / 不采纳。Body：

```json
{ "action": "mark", "expected_revision": 1 }
```

- `action`：`mark`（→confirmed）/ `dismiss`（→dismissed）；**不接受 unmark**（取消走 PATCH）。
- `expected_revision`：可选乐观锁；仅在变更状态时校验，冲突返回 409 且 `data` 附服务端最新注记行。
- 服务端从 thread 反解来源（board/topic/section/report/指纹快照），客户端无法指定快照内容。
- 语义：同目标同状态重复提交幂等返回已保存行；无归属 topic 的线索 → 404；thread 的 section 已被删除（重建后孤儿）→ 404（此路径只服务在册线索，失效快照维护走 PATCH）。

### PATCH `/daily-reports/topics/:id/key-events/:marker_id`

注记寻址维护：取消确认（含来源失效快照）、失效后重新标记。

```json
{ "action": "unmark", "revision": 3 }
```

- `action`：`mark`（→confirmed，需来源仍有效）/ `dismiss`（→dismissed）/ `unmark`（取消确认 → dismissed）。
- 状态机（DB CHECK 强制）：`suggested →(mark/dismiss)→ confirmed|dismissed`；`confirmed →(unmark)→ dismissed`；`dismissed →(mark)→ confirmed`（死来源拒绝）。
- `unmark` 只适用于 confirmed/dismissed 行（对 suggested 行 400）；来源已脱离（detached 快照）时仍可用——这是失效快照取消纠错的唯一入口；来源已死且当前 dismissed 的行再 `mark` → 400。
- `revision`：必填乐观锁（与 PUT 的 `expected_revision` 同义、字段名不同）；冲突 → 409 附最新行；marker 不存在或非该 topic → 404。

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

## 页边注（批注/问答，daily-report-margin-notes）

页边注四端点 + 管理页列表端点。问答单次 LLM 调用产出回答 + 引用 + 网络来源 + 术语；`cited_article_ids` / `cited_web_sources` / `extracted_terms` 三字段统一「空写 `[]` 不写 null」数组契约。

### GET `/daily-reports/:id/annotations`

列出该日报全部批注（含问答轮与术语）。Response `data.annotations`：

```json
[
  {
    "id": 1, "report_id": 896, "section_id": 100, "thread_id": 7,
    "quoted_text": "以利率招标…", "anchor_offset_start": 0, "anchor_offset_end": 12,
    "created_at": "...",
    "qas": [
      {
        "id": 9, "annotation_id": 1, "question": "…", "answer": "…",
        "cited_article_ids": [102, 103],
        "cited_web_sources": [{ "title": "逆回购_财经百科", "url": "https://…" }],
        "extracted_terms": ["逆回购", "DR007"],
        "operation": "daily_report.margin_note_qa", "provider": "…", "model": "",
        "created_at": "..."
      }
    ]
  }
]
```

- `thread_id=null` + `section_id=0` = 头条 lead 批注（PR-4 放行路径）。
- `cited_web_sources` 为 D7 联网白名单来源（url 均在提问当次的 SearXNG 搜索结果集内），前端外链新窗口打开。

### POST `/daily-reports/:id/annotations`

落锚。Request：`{ "section_id": 100, "thread_id": 7, "quoted_text": "划词（必填，≤500 字符）", "anchor_offset_start": 0, "anchor_offset_end": 12 }`。`thread_id` 可空（头条批注传 0）。返回 `data.annotation`（形状同上，`qas` 为 `[]`）。

错误：report 不存在 404；section/thread 归属不符 400；quoted_text 空或超长 400。

### DELETE `/api/annotations/:id`

删除批注连带全部问答（服务端直接删，确认在前端）。返回 `{"success": true}`；不存在 404。

### POST `/api/annotations/:id/questions`

提问/追问（同一端点，追加新问答轮）。Request：`{ "question": "…（必填，≤1000 字符）" }`。

提问时先以划词（截 80 runes，空则回退问题）调本地 SearXNG（失败/未配置静默降级），原始结果作为参考并入同一次 LLM 调用（operation `daily_report.margin_note_qa`，capability `open_notebook`）。Response `data.qa` 形状同 GET 的 qas 条目，另带：

```json
{
  "qa": {
    "…": "同 GET 条目",
    "extracted_terms": [{ "term": "逆回购", "is_new": true }]
  },
  "answer": "…", "cited_article_ids": [102], "cited_web_sources": [{"title":"…","url":"…"}],
  "new_terms": ["逆回购"], "terms": ["逆回购"],
  "pure_model_knowledge": false, "provider": "…"
}
```

- `pure_model_knowledge`：本地文章引用与网络来源均为空时为 true（前端据此标注）。
- LLM/路由失败 502（行内可重试，已提交问题与历史轮次不受影响）；annotation 不存在 404。

### GET `/api/annotations?board_id=&q=&page_size=&page=`

跨报告管理页列表，按日期倒序；`q` 命中划词/提问/术语；`board_id` 版块筛选。Response `data.rows` 每行含 `board_id`/`board_label`（join semantic_labels）、`terms`（跨轮去重后的术语对象数组）与 `qas`（同上形状）。`data.total` 供分页。

## `highlights` 字段契约

`highlights` 是**恒为数组**的字段：`GET /daily-reports/:id` 返回的 `report.highlights` SHALL 是 JSON 数组，空时返回 `[]`，**SHALL NOT** 返回 `null`。元素基础字段为 `{title: string, reason: string, tag_ids: number[]}`；聚簇模式条目（retire-story-adopt-event-metrics 2.3/3.3 起）追加聚簇身份与服务端快照字段（服务端计算、模型不填数字）：

```json
{
  "title": "黄仁勋风波与英伟达调查",
  "reason": "多家媒体报道指控、当事方回应与市场反应……",
  "tag_ids": [267178, 267275, 267597],
  "cluster_id": 267178,
  "article_count": 3,
  "cluster_mode": "向量聚簇",
  "data_as_of": "2026-11-10",
  "research_at": "2026-11-10T21:32:11+08:00"
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `cluster_id` | number \| null，可缺省 | 聚簇代表 event 标签 id（= 本期正文定位键：DOM 顺序第一条 `tag_ids` 含该 id 的 thread）；**标签回退模式为空/缺省**——缺省 = 无聚簇身份（无定位按钮），不得猜 tag→cluster |
| `article_count` | number \| null，可缺省 | 服务端快照：簇内文章数（内容身份去重后的篇数）；真实 0 是合法值。缺省不显示，不得用 0 冒充 |
| `cluster_mode` | string，可缺省 | 聚簇口径中文标签：`向量聚簇` / `语义向量兜底` / `主体近似`（降级链逐级如实标注，不混用） |
| `data_as_of` | string，可缺省 | 数据截至日（YYYY-MM-DD） |
| `research_at` | string，可缺省 | 研究时点（RFC3339，生成时刻） |

快照字段保持生成时刻值；旧报告与标签回退条目缺省全部新字段（`omitempty`）——前端缺字段就省略对应文案（「N 篇来源 / 聚簇口径 / 数据截至日 / 研究时点」），不伪造 0。事件线字段（`story_id` / `participant_count` / `story_heat` / `heat_rule_version` / `heat_as_of` / `heat_incomplete` / `is_progress`）与线程 `story_id` 随事件线退役整体移除。选题/时序/回退口径见 `flow/daily-report.md` 红线 46-47。

> 历史存量（2026-11-04 修复前落库的 14 行）曾因 LLM 把 JSON schema 原样回抄、解析层静默放过而落成 JSON `null` 标量。修复后读路径统一归一为 `[]`（`backend-go/internal/topicgraph/repository/daily_report_repository.go` 的 `normalizeHighlightsArray`），新生成日报落库时写 `[]` 而非 `null`。

## 前端消费约定

日报阅读层不新增 API：列表和详情分别消费 `GET /semantic-boards/:id/daily-reports` 与 `GET /daily-reports/:id`，按 `topic_status_at_report` 快照分区：

- `"active"` → "关心的话题"
- `"candidate"` / `null`（含旧数据缺失）→ "其他动态"

前端不再读取 `persistent_topic.status` 做历史日报分区，避免 topic 后续状态变化改变历史归位。masthead 顶部使用板块标题，日报 `title` / `summary` 映射为头条；highlights 最多展示三项。

`highlights` 为空（`[]` 或 `null`）时，刊头 SHALL 渲染**可见的空态占位**（「本期暂无重点」，`data-testid="highlights-empty"`），SHALL NOT 整块隐藏——空即消失会让「本期没有重点」与「接口出错」在界面上无法区分。占位为纯静态文本：无按钮、无跳转、无批注锚点；与正常态互斥渲染（`id="report-highlights"` 任一时刻只存在一个）。

active 话题展开时按 `persistent_topic.id` 懒加载 `GET /daily-reports/topics/:id/lifeline`。前端只截取当前日报向前七个自然日，同日多个 section 聚合为一个带数量角标的节点；连线只消费 `relation_type=identity`，`similarity` 不进入 mini 泳道。节点点击按 `report_id` 复用日报详情缓存，在时间线模块内原位展示当日 section/thread。

thread 的 `related_article_ids` 由现有 `GET /articles/:id` 逐项解析标题并按 article id 去重缓存；单篇 404/失败局部显示重试，不阻塞同话题其他文章。
