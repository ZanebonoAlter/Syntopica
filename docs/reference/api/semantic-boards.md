# SemanticBoard 与辅助标签 API

基础地址：`http://localhost:5100/api`（开发模式亦可用同源代理 `http://localhost:3000/api`）

通用响应：成功为 `{"success": true, "data": ...}`；失败为 `{"success": false, "error": "..."}`。

## 端点汇总

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/semantic-boards` | 查询 active 版块列表 |
| GET | `/semantic-boards/:id` | 查询单个版块 |
| POST | `/semantic-boards` | 创建版块 |
| PUT | `/semantic-boards/:id` | 更新版块（label/description 变更会重算向量并失效匹配缓存） |
| DELETE | `/semantic-boards/:id` | 软删除版块（status→disabled） |
| GET | `/semantic-boards/:id/composition` | 查看版块构成辅助标签 |
| POST | `/semantic-boards/:id/composition` | 新增 composition 辅助标签 |
| DELETE | `/semantic-boards/:id/composition/:auxiliary_label_id` | 移除 composition 辅助标签 |
| GET | `/semantic-boards/:id/articles` | 查询版块下的文章 |
| GET | `/semantic-boards/:id/article-daily-counts` | 查询版块命中文章按自然日的计数（只读聚合，archive-time-chart-range） |
| GET | `/semantic-boards/:id/source-breakdown` | 查询版块来源构成（只读聚合，add-source-board-hit-rate） |
| GET | `/semantic-boards/:id/health-checkup` | 查询版块体检数据（最新快照+评分趋势，只读，add-board-health-checkup） |
| GET | `/semantic-boards/:id/key-events` | 版块级关键事件批量读（窗口内全部 confirmed 注记，只读，add-key-event-matrix-view） |
| GET | `/semantic-boards/:id/suggest-auxiliaries` | 版块级辅助标签建议 |
| GET | `/semantic-boards/suggest-auxiliaries` | 全局辅助标签建议 |
| GET | `/semantic-boards/:id/match-detail/:tagId` | tag 与版块匹配明细 |
| GET | `/auxiliary-labels` | 查询辅助标签池 |
| GET | `/auxiliary-labels/clusters` | 辅助标签聚类 |
| POST | `/auxiliary-labels/merge-alias` | 合并辅助标签为 alias |
| POST | `/auxiliary-labels/backfill-embeddings` | 辅助标签死向量回填（202 异步） |
| POST | `/auxiliary-labels/gc` | 辅助标签垃圾回收 |
| POST | `/auxiliary-labels/:id/disable` | 禁用辅助标签 |
| GET | `/auxiliary-labels/:id/description-repair-context` | 描述修复上下文（实时描述 + 不透明版本 + 直接引用版块 + 活跃标签数 + 最近任务，只读） |
| POST | `/auxiliary-labels/:id/description-repairs` | 创建共享描述修复任务（202，幂等键 + expected_version） |
| GET | `/auxiliary-labels/:id/description-repairs/:job_id` | 查询持久化修复任务（只读，校验归属该辅助标签） |
| POST | `/auxiliary-labels/:id/description-repairs/:job_id/retry` | 显式重试失败/中断的修复任务（202，同一任务） |
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
| GET | `/semantic-boards/:id/tags/:tagId/related-events` | 同版块相关事件（只读派生向量索引，enable-story-cross-day-continuation） |
| GET | `/semantic-boards/matching-config` | 读取匹配参数 |
| PUT | `/semantic-boards/matching-config` | 更新匹配参数 |
| GET | `/tags/:id/auxiliary-labels` | tag 关联辅助标签 |
| GET | `/tags/:id/semantic-boards` | tag 所属版块 |
| POST | `/semantic-boards/:id/persistent-topics/manual` | 手动新建持久话题 |
| GET | `/semantic-boards/:id/persistent-topics/compose-candidates` | 编排态候选池（section + 向量 + 线索数 + 关系/候选链/窗口；`omit_embeddings=1` 轻量档） |
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

更新 SemanticBoard。`label` 或 `description` 变更时会重新生成 embedding，并**在同一请求内失效 board embeddings 匹配缓存**（lane-manual-correction-loop board-match-cache）。

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

**缓存失效（lane-manual-correction-loop board-match-cache）**：`loadBoardEmbeddings` 缓存**无 TTL**（只靠显式失效），故 `label` / `description` 任一变化并重算出版块向量后，后端 SHALL 在同一请求内调 `InvalidateMatchCache()` 清缓存——SHALL NOT 让方向校验继续拿旧向量判定（缓存无 TTL 场景下尤其致命）。失效**只清缓存**，SHALL NOT 自动重跑 `topic_tag_board_labels`（匹配表保持旧结果，重跑/回填是用户的显式动作）。

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

### POST `/auxiliary-labels/backfill-embeddings`

重建 active 辅助标签缺失（NULL）的 embedding / merge_embedding 向量（fix-alias-save-vector-wipe 存量修复）。生成规则与 L3 新建一致（merge ← label，storage ← "label: description"，无 description 时仅 label），**只补缺失的那列**、已有向量不重算；行级生成失败跳过不阻塞。

幂等：重复触发只处理仍缺向量的行；无死向量时空操作（零 LLM 调用）。同步返回 pending 数后由后台 goroutine 异步执行，重触发至 `pending: 0` 即完成；完成后可选跑 `POST /semantic-boards/rematch-all` 让版块挂载重新竞争。

无请求体。Response `202` `data`：

```json
{ "pending": 1505 }
```

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

## 辅助标签描述修复（board-health-triage-advice）

> 体检分诊（🔴 建议摘除 / 🟡 建议重写描述）落到「改共享标签」上时走这条独立链：**人工确认 → 持久任务 → 事务外重嵌 + 事务内原子保存 → 既有匹配引擎逐项更新**。体检轮次本身只读（不写匹配表、不落扩充建议），这四个资源是**修改既有辅助标签** description / storage 向量的唯一 HTTP 入口（组合标签走独立的 composite 接口，且描述修复链不覆盖组合标签）。业务背景与状态机见 `docs/reference/flow/semantic-board.md` §版块体检三病分诊与共享描述修复。

四个资源共用同一个 service 实例（域根单例，与后台执行器同一扇门），路径前缀 `/auxiliary-labels/:id`（`:id` = 辅助标签 ID，`:job_id` = 修复任务 ID）。

**通用约定**：

- ID 在 wire 上是**数字**（前端 API 边界归一为字符串视图消费，模板不得直接消费数字 wire ID）。
- 任务 `data` 的字段集：`id` / `auxiliary_label_id` / `idempotency_key` / `expected_version` / `old_description` / `new_description` / `stage` / `status` / `description_committed` / `total` / `processed` / `failed` / `not_applicable` / `remaining` / `error` / `created_at` / `updated_at`。
- **内部执行列不进 wire**：`claim_owner` / `claimed_at` / `lease_expires_at` / `committed_version` / `committed_at` / `phase_token` 只在 DB 与服务端内部使用，任何响应都不透出。
- 错误一律 `{"success": false, "error": "..."}`；`500` 的 `error` 恒为 `internal server error`（原始 DB 细节只进服务端日志，不外泄）。

**错误码一览**：

| 状态 | 触发条件 | `error` 文案 |
| --- | --- | --- |
| `400` | `id` / `job_id` 非法（非数字、0、超过 MaxInt64）；请求体不是**恰好一个** JSON 对象（非法 JSON、第二个对象、尾随垃圾）；`idempotency_key` 缺失或超 64 字符；`expected_version` 缺失或超 64 字符；`description` trim 后为空 / 超 500 字符 / 只重复 label；retry 带了任何参数或第二个对象 | `invalid id` / `invalid job_id` / `invalid request body` / `idempotency_key is required` / `idempotency_key must be at most 64 characters` / `expected_version must be a non-empty string of at most 64 characters` / `description must not be empty` / `description must be at most 500 characters` / `description must not only repeat the label` |
| `404` | 辅助标签不存在或不是 `label_type='auxiliary'`；任务不存在或**不属于**该辅助标签（不跨标签泄露） | `auxiliary label not found` / `repair job not found` |
| `409` | ① 标签已 disabled；② 来源版本已变（`expected_version` 与锁定行不符）；③ 幂等键已用于**不同载荷**；④ 同标签已有**未结束**任务（互斥）。**retry 专用**：任务已处 `conflict`/`unavailable` 终态 | ① `auxiliary label is disabled` ② `auxiliary label has changed, reload and confirm again` ③ `idempotency key was already used with a different request` ④ `an unfinished description-repair job already exists for this auxiliary label (job <id>, status <status>)`；retry：`repair task cannot be retried in its current state` |
| `500` | 服务端内部错误（DB 异常等） | `internal server error`（原始错误只进服务端日志，不外泄） |

> `409` 是**「确定拒绝」**：客户端 MUST NOT 换新幂等键重发同一请求（换键会绕过幂等语义）；应保留用户草稿、按 `error` 指引重新核对（冲突类重新加载最新版本；互斥类对原任务走 `retry`）。

### GET `/auxiliary-labels/:id/description-repair-context`

用户核对修复影响面时的**实时只读上下文**：当前真实描述、不透明版本令牌、直接引用该标签的版块、活跃话题标签覆盖数、最近一次修复任务。**纯读**：不创建任务、不改任何东西（打开页面/看建议 MUST NOT 产生任务）。

Response `200` `data`：

```json
{
  "auxiliary_label": {
    "id": 42,
    "label": "上海",
    "description": "上海知名商业综合体",
    "status": "active",
    "version": "v1_9f2c1ab7d4e5f60718293a4b5c6d7e8f90a1b2c3d"
  },
  "direct_boards": [
    { "id": 5, "label": "中国国内新闻", "status": "active" },
    { "id": 9, "label": "长三角经济", "status": "disabled" }
  ],
  "active_tag_count": 137,
  "latest_task": {
    "id": 3,
    "auxiliary_label_id": 42,
    "status": "matching_failed",
    "stage": "matching",
    "total": 137,
    "processed": 120,
    "failed": 2,
    "not_applicable": 1,
    "remaining": 14,
    "description_committed": true,
    "created_at": "2026-10-29T10:00:00Z",
    "updated_at": "2026-10-29T10:03:00Z"
  }
}
```

- `auxiliary_label.version`：**服务端不透明版本令牌**（覆盖 label/description/status/更新时间语义）。前端只透传，**不得自己拼装或解析**；提交修复时原样回填到 `expected_version`。
- `direct_boards[]`：经 `board_composition` 直接引用该辅助标签的版块（disabled 版块也列出并带 `status`，用于告知影响面）。
- `active_tag_count`：全库活跃话题标签数（本次修复的匹配覆盖范围量级）。
- `latest_task`：该标签最近一次任务；从未有过则 `null`（重进恢复的数据源）。
- `status='disabled'` 的标签**照常返回**（可展示不可执行状态）；标签不存在或不是 `label_type='auxiliary'` → `404`。

### POST `/auxiliary-labels/:id/description-repairs`

把用户确认的新描述落成**一条持久任务**（`202`）。本端点**只记录确认请求**，不做 embedding、不做匹配（那是后台执行器的活）。

Request：

```json
{
  "description": "中国东部城市，上海市",
  "expected_version": "v1_9f2c1ab7d4e5f60718293a4b5c6d7e8f90a1b2c3d",
  "idempotency_key": "8f3c1d2e-6b7a-4c9d-9e10-2f5a7b3c8d41"
}
```

- `description`：trim 后 1..500 个 Unicode 字符，且**不得只重复 label**（纯分隔符等变体在当前三规则下合法，不另加黑名单）。服务端存 trim 后的值。
- `expected_version`：核对时看到的不透明版本令牌；与事务内锁定行的实时令牌不符 → `409`（来源版本已变）。
- `idempotency_key`：客户端**每次确认生成一个唯一键**（≤64 字符）；超时/网络失败重发必须复用同键同载荷。

Response `202` `data`：任务对象（同上通用字段集）。

**幂等语义**：

- 同键 **+ 同载荷**（标签 + expected_version + 新描述）→ 返回**同一任务**（不重复重嵌、不重复提交描述）；即使源标签事后被硬删或禁用，重放仍然有效。
- 同键 **+ 不同载荷** → `409`（绝不默默返回旧任务当成功）。
- 同标签已存在**未结束**任务（含 `embedding_failed`/`save_failed`/`matching_failed`）→ `409`（同标签互斥；此时只能对该任务走 `retry`）。

### GET `/auxiliary-labels/:id/description-repairs/:job_id`

查询持久化任务的真实阶段与计数。**只读**：不触发执行、不改状态。任务必须属于该辅助标签，否则 `404`（不跨标签泄露）。源标签事后被删的历史任务仍可查。

Response `200` `data`：任务对象（同上通用字段集）。

**状态词表**：`pending` → `embedding` → `saving` → `matching` → `completed`；失败态 `embedding_failed` / `save_failed` / `matching_failed`；终态死路 `conflict`（来源版本变化）/ `unavailable`（标签非 active 或已删）。`stage` 取 `embedding|saving|matching`（`pending` 时可为空）。

**计数字段（服务端单源，前端不得重算）**：

| 字段 | 含义 |
| --- | --- |
| `total` | 创建时**冻结**的范围数量（当时全部活跃 topic_tags，含未归属任何版块者）；后续失效也**不缩小** |
| `processed` | 匹配引擎**实际成功**处理的项数（合法零命中也算成功执行） |
| `failed` | 仍处于失败态的项数 |
| `not_applicable` | 处理时已失效/删除/合并而**持久跳过**的项数——**不是成功**，也不计入 `processed` |
| `remaining` | `total − processed − failed − not_applicable`；四者之和恒等于 `total` |

- 只有 `remaining=0` 且 `failed=0` 才是 `completed`；**部分失败恒为 `matching_failed`**，不得标全完成。
- `description_committed=true` 只表示「描述 + storage 向量已原子提交」，**不等于匹配全部完成**。

### POST `/auxiliary-labels/:id/description-repairs/:job_id/retry`

对失败或中断的任务发起**显式重试**。**不接受任何参数**（body 可为空或空 JSON 对象 `{}`；非法 JSON / 第二个对象 / 尾随垃圾 / 任何参数一律 `400`）。重试复用**同一任务**：不新建任务、不改 `expected_version`、不改新描述、不改冻结范围、不重嵌已提交的描述。

Response `202` `data`：任务对象（同上通用字段集）。

- 三失败态（`embedding_failed`/`save_failed`/`matching_failed`）→ **同一任务**重新排入队列：描述已提交时**只续匹配**（embedding 调用数不增加），未提交时从 embedding 阶段续跑。
- 运行中 / 已完成 → `202` + 当前任务的**幂等 no-op**（不重排、不另起 worker、不改计数与时间）。
- `conflict` / `unavailable` → `409`（终态不可原样重试，必须加载最新版本重新核对）。
- 后台**不自动无限重跑**失败任务（不自动烧 AI 额度）：三失败态一律等用户显式 `retry`。

## 升级候选与建议

### POST `/semantic-boards/upgrade-suggest`

四格参数触发 LLM 升级建议（内存响应，不落库；面板主链路走 `upgrade-suggestions/generate`）。split-board-upgrade-directions：`direction × source` 组合参数取代旧 `mode` 参数（旧参数与 `upgrade-candidates` 端点均已退役）。

Query 参数：

| 参数 | 说明 |
| ------ | ------ |
| `direction` | 必填：`create`（创建版块）/ `expand`（版块扩充） |
| `source` | 必填：`aux`（单标签）/ `composite`（组合标签） |
| `target_board_id` | `direction=expand` 必填（锁定单版块，本轮建议 target 恒等于它，须为活跃版块）；`create` 不得携带 |
| `days` | 可选时间窗（`0`=不过滤）：create×aux 按文章活动过滤候选；expand 相似路要求近 days 天文章引用、共现/compose 路窗口取 days 与 CoTagWindowDays 更严者；create×composite 携带但忽略 |

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
  "semantic_board_match_max_boards": 3,
  "semantic_board_match_direct_hit_min_overlap": 2,
  "semantic_board_match_direct_hit_min_ratio": 0.5,
  "semantic_board_match_direction_sim_threshold": 0.5,
  "related_event_similarity_threshold": 0.86,
  "lane_heat_candidate_weight": 0.5
}
```

`related_event_similarity_threshold`（enable-story-cross-day-continuation）与入板匹配参数**互相独立**：它只影响「相关事件」列表的过滤，缺项回退默认 **0.86**（2026-11-10 S3 实测后由 0.82 上调），存量坏值（非有限数/越界）告警后同样回退默认；改它**不需要重算向量**。

`lane_heat_candidate_weight`（retire-story-adopt-event-metrics D8）是候选泳道在「当天线索热度」里的权重：默认 **0.5**、域 `[0,1]` 有限数，缺项回退默认、坏存量值告警回退；它**只进热度**（版块关注度 = Σ 泳道当天热度），不参与入板匹配，也**不影响话题生命周期**（命中计数 / 转正门槛不变）。

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

约束（enable-story-cross-day-continuation）：

- 所有给到的键先全部校验、再在同一事务内写入 —— 任一键非法 → 整体 400、**不部分落库**；
- 数值键拒绝空白、非数字、`NaN`/`Infinity`、范围外（`related_event_similarity_threshold` 与 `lane_heat_candidate_weight` 均为 `[0,1]`，0 与 1 合法）；
- 白名单外的键 → 400 `unsupported config key`；
- `lane_heat_candidate_weight` 与相关事件阈值各自独立校验、互不影响：任一键非法即整体 400，不部分落库。

## 关键事件（版块级批量读）

### GET `/semantic-boards/:id/key-events?days=14`

一次取回版块 `:id` 在观察窗内的**全部人工确认关键事件**（add-key-event-matrix-view，design D3）。供版块态势页的「关键事件」视图一次取全，不再逐话题请求。**只读**：不创建 / 确认 / 修改任何注记，不改写任何快照字段。

口径：

- **状态**：只返回 `state='confirmed'`；`suggested` / `dismissed` 永不进入载荷。
- **窗口**：按 `source_period_date` 落在闭区间 `[今天 − (days−1), 今天]`（服务端上海时区今天）筛选；窗外事件不返回。
- **排序**：`source_period_date ASC, persistent_topic_id ASC, id ASC` —— 前端可直接按「话题 + 日期」聚合，同日多事件保持接口序。
- **快照字段不反查**：事件名称、话题名、摘要、关联文章都是**确认时冻结的快照**（话题被改名 / 归档都不回写）。
- **空结果**：窗口内 0 条时返回 `200` 与空数组（**不是错误**，也不拿建议态补足）。
- **越权与非法参数**：版块不存在或不是 board → `404`；`id` 非数字 → `400`；`days` 非数字 / 负数 / **0** → `400`（本端点 `days` 必须是**正整数**；与话题级端点 `days=0=全部时间` 的旧契约不同——那里是既有契约，MUST NOT 连带改坏）。

Query：

- `days` 可选，整数正数，缺省 `14`；非法值（非数字 / ≤0）返回 `400` 且错误信息指明 `days` 不合法。

Response `data`：

```json
{
  "markers": [
    {
      "id": 42,
      "semantic_board_id": 1980,
      "source_key": "thread:1024",
      "persistent_topic_id": 93,
      "source_period_date": "2026-11-18T00:00:00Z",
      "topic_label_snapshot": "LLM生态相关新闻",
      "title_snapshot": "GPT-6.2 Astra 与 Sol 双模型发布",
      "summary_snapshot": "OpenAI 同日发布两个模型，分层定位。",
      "tag_ids_snapshot": [7, 9],
      "article_ids_snapshot": [178945, 178947],
      "state": "confirmed",
      "confirmed_at": "2026-11-19T02:11:00Z",
      "revision": 1,
      "created_at": "2026-11-19T02:10:00Z",
      "updated_at": "2026-11-19T02:11:00Z"
    }
  ]
}
```

`persistent_topic_id` 可为 `null`（来源话题被删 / 重建断开引用，FK `ON DELETE SET NULL` 保留确认快照）；`article_ids_snapshot` 可为 `null`（确认时未冻结关联文章）——两者都是合法态，前端分别按「无话题归属」与「无关联文章」渲染，MUST NOT 造 0 / 造 ID。

## 相关事件

### GET `/semantic-boards/:id/tags/:tagId/related-events`

读取某个 event 标签在**同一版块**内的直接相关事件（enable-story-cross-day-continuation，能力 `related-events`）。用于归档文章 event 标签卡的「相关事件」按需展开。

口径：

- **只读**：不调用模型、不写任何表（含派生缓存）；索引由显式刷新 `cmd/related-events --refresh --board <id>` 生成。
- **输入**：`tag_id` + `board_id`；比较对象是 event 标签的确定性文本表示（名称 + 描述 + 当前有效辅助标签，版本 `related-event-v1`）。**不读** `subject_key`、实体 ID、`fingerprint_trusted`、thread 向量。
- **范围**：同版块当前有效（`topic_tags.status='active'` 且入板未 `direction_mismatch`）的 event 标签，`topic_tags.created_at` 落在 `[as_of-30d, as_of]`（含两端；`as_of` = 请求开始时服务端 UTC 时间）。**这是标签收录时间，不是事件发生时间。**
- **打分**：在当前 `space_key`（vendor + model + 显式 index generation）内用精确 pgvector 余弦距离算全部候选，`score >= 生效阈值` 过滤后按 `score` 降序、`tag_id` 升序取前 **10**；阈值前不做无关 top-K 截断；**不传递**（A≈B、B≈C 不会让 A 返回 C）。
- **来源文章**：每条结果取该候选标签在本版块的最新一篇可读文章（`pub_date` 降序、`id` 降序）；已归档文章按既有「站内阅读可读」策略**不排除**；无来源为 `null`（前端禁用入口，不伪造 ID）。

Query：无。

Response `data`：

```json
{
  "board_id": 1980,
  "tag_id": 267115,
  "status": "ready",
  "reason": "",
  "threshold": 0.86,
  "window_days": 30,
  "window_start": "2026-10-11T17:17:03Z",
  "window_end": "2026-11-10T17:17:03Z",
  "candidate_count": 543,
  "indexed_count": 543,
  "representation_version": "related-event-v1",
  "space_key": "provider=qwen3-embedding|model=qwen3-embedding|gen=g1",
  "items": [
    {
      "tag_id": 251798,
      "label": "Anthropic 推出 AI 智能体 Claude Duet",
      "description": "2026 年 10 月 22 日，Anthropic 推出基于 Opus 5.5 模型构建的 AI 智能体 Claude Duet…",
      "similarity": 0.9044,
      "source_article": { "id": 163601, "title": "Anthropic 推出 AI 智能体 Claude Duet" }
    }
  ]
}
```

覆盖状态（**缺索引不得冒充空结果**）：

| `status` | 含义 | `reason` |
| --- | --- | --- |
| `ready` | 源与全部窗口内候选都在当前空间/表示版本下有有效向量（`items` 可为空） | — |
| `partial` | 源就绪，但部分候选缺有效向量；`items` 只含已就绪结果 | — |
| `not_ready` | 源自身没有有效向量，或没有可用 embedding 路由 | `source_not_indexed` / `route_unavailable` / `invalid_source` / `text_too_long` |

`candidate_count` = 窗口内有效候选数（不含自身）；`indexed_count` = 其中在当前空间可用数。

错误：版块不存在/非 board → **404**；标签不是该版块的有效 event 成员 → **404**；参数非法 → 400。

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
        { "id": 42, "label": "...", "category": "...", "description": "...", "match_reason": "hit_rate", "score": 0.84, "direction_mismatch": false }
      ]
    }
  ],
  "pagination": { "page": 1, "per_page": 20, "total": 35, "pages": 2 }
}
```

文章字段同 `GET /articles/:id`（`ToDict()`），额外带 `feed_name` 与 `filtered_tags`；`filtered_tags` 每项的 `description` 取 `topic_tags.description`（标签无描述时为空串）；无数据时 `data` 为空数组。

> **疑似 AI 标记（add-ai-generated-article-detection）**：因为走 `ToDict()`，每行天然带判定四元组 `ai_detection_status` / `ai_detection_score` / `ai_detection_source` / `ai_detection_at`（取值语义同 `GET /articles/:id`；未判定时 `ai_detection_status` 归一为 `unknown`，字段不缺失）。它只是投影字段：不影响成员筛选、`filtered_tags`、排序（`sort=quality` / `sort=time`）与分页，也不进任何匹配口径。前端归档视图只据此渲染「疑似 AI 生成」chip。

### GET `/semantic-boards/:id/article-daily-counts`

查询版块 `:id` 命中文章按自然日的计数（archive-time-chart-range）：供归档时间曲线与全历史导航条使用。**只读聚合**：不写库、不触发打标或匹配。

无查询参数——**固定口径聚合**：`feed_id` / `auxiliary_label_id` / `report_status` / `show_direction_mismatch` / 分页 / 日期窗口一律不参与（传了也不解析、不返回 400；这是给未来留的余地，不是契约）。

**成员口径与 [`/articles`](#get-semantic-boards-idarticles) 主查询逐字一致**：同一 `boardTagIDs` 取法（`topic_tag_board_labels.semantic_board_id`，不过滤标签 `status`）、同 `feeds` 内连接、同 `DATE(articles.pub_date)` 归日；计数按文章 id 去重（一篇文章命中多个版块标签仍只计 1）。方向校验未通过的文章（列表行内默认隐藏其标签）**仍计入**当日 `count`，以与归档列表「当天篇数」对齐。`pub_date IS NULL` 的存量行**排除**（按日曲线无法定位无日期文章；NULL 分组会让扫描目标整体报错）。

**时区前提**：`DATE(pub_date)` 取的是数据库**会话时区**的日历日。默认 DSN 钉了 `TimeZone=Asia/Shanghai`（`internal/platform/config` 默认值），与前端「今天」口径一致；若部署时用 `DATABASE_DSN` 整体覆盖或改容器 `TZ`，归日会随之偏移，曲线日与归档列表分组/「今天」就会错位。

Response `data`：

```json
{
  "earliest_date": "2026-01-05",
  "latest_date": "2026-11-18",
  "days": [
    { "date": "2026-11-17", "count": 211 },
    { "date": "2026-11-18", "count": 246 }
  ]
}
```

- `earliest_date` / `latest_date`：该版块命中文章的最早 / 最新自然日；**版块无命中文章时两者均为 `null`**。
- `days`：**稀疏**数组（只含 `count > 0` 的日期，按 `date` 升序）；缺失日期由调用方按已知 0 处理，端点 MUST NOT 补齐全区间零值。
- 版块无标签 / 版块不存在：返回 `{"earliest_date": null, "latest_date": null, "days": []}`，HTTP `200`（与 `/articles` 的空列表口径一致，不返回 404）。

### GET `/semantic-boards/:id/source-breakdown`

查询版块 `:id` 窗口内的来源构成（add-source-board-hit-rate）：哪些订阅源在供血、各占多少篇与比例。**只读聚合**：不写库、不触发打标或匹配。

与 [`/articles`](#get-semantic-boards-idarticles) 的口径差异：`/articles` 是版块文章列表（可分页/过滤，含 tag 匹配质量明细）；`source-breakdown` 是按源聚合的命中构成——**含已归档文章、按文章去重（同版块内多标签只计一次；同一文章命中多版块时在各版块各计一次）、必限窗口**（三条口径硬约束同 `GET /api/feeds/board-hit-stats`，见 `docs/reference/api/feeds.md`，唯一实现 `internal/tagmanagement/service/sourcestats/`）。

Query：

- `window` 可选，白名单 `7`/`30`/`90`，缺省 `7`；非法值返回 `400`。

Response `data`：

```json
{
  "total_articles": 437,
  "source_count": 8,
  "sources": [
    {
      "feed_id": 17,
      "title": "华尔街见闻 - 最热文章",
      "articles": 142,
      "share": 0.325,
      "feed_articles": 67,
      "feed_hit_rate": 0.836
    }
  ]
}
```

- `total_articles`：该版块窗口内命中文章总数（按文章去重）。
- `sources[].articles`：该源在本版块的命中篇数；各项之和恒等于 `total_articles`。
- `sources[].share`：`articles / total_articles`，`total_articles=0` 时为 0。
- `feed_articles` / `feed_hit_rate`：该源自身窗口内总量与整体命中率（上下文列，用于识别只偶尔命中本版块的源）。

- 版块不存在或非 `board` 类型返回 `404`；disabled 版块**不是** 404（按命中口径聚合为 0）。
- 窗口内无命中文章时 `total_articles=0`、`source_count=0`、`sources=[]`，属正常结果（200）。

### GET `/semantic-boards/:id/health-checkup`

查询版块 `:id` 的体检数据（add-board-health-checkup）：最新体检快照（两级统计 + 未收编候选 + LLM 评分/评语）与评分趋势序列。**只读**：不写库、不触发体检执行（体检由日报生成成功路径尾步自动跑，见 `docs/reference/flow/semantic-board.md` §版块体检；收编/添加/移除复用既有 composition 接口）。统计口径唯一实现 `internal/tagmanagement/service/boardhealth/`。

> 磁铁信号口径（fix-board-health-magnet-signal）：`suspicious = failed_mean − good_mean ≥ 阈值`（**方向对齐「磁铁把跑题的挂载吸过来」**；旧口径 `all_mean − failed_mean` 已废止，因其把与本板方向不符的失败挂载当成「越不像越有嫌疑」，导致核心主体被系统性点名）。`all_mean`/`all_samples` 保留供历史对比展示（**含**失败行，且 `all_mean ≡ cosine_mean`），不参与判定。`caliber` 为口径标记：`healthy_subset` = 新口径，空 = 历史快照旧口径——消费方 MUST 按此区分「旧口径快照」与「新快照但健康侧不可算」，MUST NOT 把新快照误标为旧口径。

Query：

- `trend` 可选，整数白名单 `[1, 90]`，缺省 `7`；非法值（0/91/负数/非整数）返回 `400`。

Response `data`：

```json
{
  "latest": {
    "id": 12,
    "semantic_board_id": 5,
    "checked_at": "2026-10-02T08:00:00Z",
    "metrics": {
      "board": {
        "matched_tags": 320,
        "reason_dist": { "direct_hit": 210, "composite_hit": 30 },
        "ghost_matches": 12,
        "vector_missing": 3,
        "magnet_flow": {
          "failed_tags": 12,
          "flow_tags": 9,
          "top_board": "中国国内新闻",
          "top_tags": 6
        }
      },
      "labels": [
        {
          "aux_id": 42,
          "label": "推理",
          "label_type": "auxiliary",
          "core_subject": false,
          "samples": 18,
          "cosine_mean": 0.81,
          "cosine_median": 0.83,
          "cosine_p25": 0.76,
          "cosine_p75": 0.88,
          "low_sim_ratio": 0.11,
          "insufficient": false,
          "description": "推理能力相关话题",
          "magnet": {
            "all_mean": 0.68,
            "failed_mean": 0.52,
            "good_mean": 0.70,
            "all_samples": 320,
            "good_samples": 280,
            "failed_samples": 40,
            "caliber": "healthy_subset",
            "suspicious": false,
            "insufficient": false
          },
          "neighbors": [
            { "aux_id": 88, "label": "举办地点", "cosine": 0.796 },
            { "aux_id": 91, "label": "城市名", "cosine": 0.742 },
            { "aux_id": 93, "label": "地标建筑", "cosine": 0.701 }
          ],
          "desc_flags": []
        }
      ],
      "triage_guard": {
        "actionable": 2,
        "total": 4,
        "ratio": 0.5,
        "threshold": 0.8,
        "degraded": false,
        "reason": ""
      },
      "triage": [
        {
          "aux_id": 42,
          "category": "rewrite",
          "evidence": "最近邻为实例标签（举办地点 0.796），语义被 description 窄化",
          "suggestion": "模型推理与推理能力相关主题"
        },
        {
          "aux_id": 77,
          "category": "remove",
          "evidence": "磁铁信号成立：失败子集均值 0.72 vs 健康子集均值 0.52（差 0.20），失败挂载 40 个且集中流向「中国国内新闻」"
        },
        {
          "aux_id": 91,
          "category": "keep",
          "evidence": "短实体名天花板：cosine_mean 0.57 但方向通过率 100%，无磁铁信号"
        },
        {
          "aux_id": 93,
          "category": "insufficient",
          "evidence": "失败子集样本仅 2 个，低于最小样本量"
        }
      ]
    },
    "uncaptured_candidates": [
      { "aux_id": 77, "label": "算力", "member_tags": 12, "member_articles": 5 }
    ],
    "llm_score": 78,
    "llm_comment": "构成清晰，方向匹配整体健康。",
    "created_at": "2026-10-02T08:00:01Z"
  },
  "trend": [
    { "checked_at": "2026-10-01T08:00:00Z", "llm_score": 70 },
    { "checked_at": "2026-10-02T08:00:00Z", "llm_score": 78 }
  ]
}
```

- `latest`：最新一条快照；从未跑过体检时为 `null`（空态，非错误）。
- `metrics.board`：版块级指标（全部匹配行口径）；`metrics.labels[]`：标签级贴合度分布（构成标签 embedding × 全部挂载 tags identity embedding 现场重算，不按 aux 挂载数量筛样本），`insufficient=true` 时分布字段全 `null`（样本不足不硬凑）。
- `metrics.board.magnet_flow`：版块级**流向证据**（失败挂载的有效流向 top-1）；`failed_tags` = 本轮失败挂载数，`flow_tags` = 其中在其他 active 版块有**有效**挂载的数（只用失败挂载的其它 direction_mismatch 挂载不算可靠流向），`top_board` = top-1 目的版块名，`top_tags` = 在该版块有效挂载的失败 tag 数；`top_board=null` = 显式未知（本轮有失败挂载但无可信流向），不得臆造目标。
- `metrics.labels[]` 新增字段（board-health-triage-advice）：`label_type`（`auxiliary` / `composite`；**历史快照缺该字段 = 类型未知**，消费方不得臆造）、`description`（体检时刻的描述快照，事件性预筛的证据源，omitempty）、`magnet`（磁铁信号：`all_mean`/`failed_mean`/两侧样本数/`suspicious`/`insufficient`；任一侧向量缺失时均值为 `null`、`insufficient=true`）、`neighbors`（全库 active 辅助标签 top-3 最近邻，排除自身；`null` = 自身向量缺失信号不足，非 `null` 空数组 = 已计算但空间内无其他标签）、`desc_flags`（命中的文章式词根，空描述不标记）。
- `metrics.labels[]` 口径更新（fix-board-health-magnet-signal）：`magnet.good_mean`/`magnet.good_samples` = 健康挂载子集（`direction_mismatch=false`）的贴合均值与样本数，是磁铁判定的**健康侧**（`suspicious = failed_mean − good_mean ≥ 阈值`）；`magnet.caliber` = 口径标记（`healthy_subset` = 新口径，空 = 历史快照旧口径）；`labels[].core_subject` = 核心主体标记（版块 description/aliases 命中，或标题命中且标签名 ≥ 3 字符；历史快照缺该字段 = `false`）。核心主体 MUST NOT 作为摘除/重写对象（分诊层规则强制改判 `keep`）。
- `metrics.triage_guard`（fix-board-health-magnet-signal，板级闸门结论）：`actionable`/`total`/`ratio` 取 **LLM 原始裁决**的可执行条数与占比（不是降级后的值），`threshold` = 配置阈值（默认 0.8），`degraded` = 本轮是否因占比超限把可执行项统一降级为 `insufficient`，`reason` = 降级原因；闸门不改评分与评语。
- `metrics.triage[]`：标签级分诊清单（board-health-triage-advice）；`category` 闭集 `remove`（建议摘除）/ `rewrite`（建议重写描述）/ `keep`（保留澄清）/ `insufficient`（证据不足）；`evidence` 必带（真实信号数值或统计事实）；`suggestion` 仅 `rewrite` 必填（新描述文案），其余可省。**分诊清单是建议不是执行**：摘除走既有 `DELETE /semantic-boards/:id/composition/:auxiliary_label_id`（须用户确认），重写走 `POST /auxiliary-labels/:id/description-repairs`（须人工确认）；`label_type` 非 `auxiliary`（含类型未知）的 `rewrite` 项**不可执行**（描述修复链只覆盖辅助标签），但 `remove` 项不受类型限制。
- **降级轮**（LLM 坏 JSON/超时）：统计与信号照常返回，`llm_score`/`llm_comment` 为 `null`、`metrics.triage` 为 `null`（= 本轮无分诊，历史快照同样为 `null`，两者靠快照时间/有无评分区分）。
- `uncaptured_candidates[]`：未收编候选（成员 tags 高频携带但不在构成中的 active aux，按 member_tags 降序 top 50）。
- `llm_score` / `llm_comment`：LLM 综合评分（0-100）与一句评语；LLM 降级轮为 `null`（统计照常）。
- `trend[]`：按 `checked_at` **升序**的最近 N 条评分序列；同日多条保留不去重（手动补跑日报是真实数据点）。
- 版块不存在返回 `404`；disabled / 非 board 版块返回空态（`latest=null`、`trend=[]`，非 404）。

## 持久话题手动编排

### GET `/semantic-boards/:id/persistent-topics/compose-candidates`

编排态候选池（unify-lane-workbench 后同时为「泳道编排 / 泳道修正」两个工作面的唯一取数端点）：返回窗口内 **有可用向量** 的 section（含向量本体）、窗口内两端可见的原关系（identity + similarity）、服务端唯一口径的候选链、`match_threshold`、回显窗口，以及可选的「更早成员」。前端据此实时算聚合锚点 / 离群 / 质心推荐，不在每次勾选时往返。

Query：

| 参数 | 说明 |
| --- | --- |
| `days` | 快捷档窗口长度（以该版块**最新 completed 日报**为末日、含首末）。上限 `ComposeWindowMaxDays = 365`（unify-lane-workbench OQ-2；此前 90），超过按 365 钳位；`from`/`to` 给出时忽略。 |
| `from` / `to` | 自选起止（闭区间 `YYYY-MM-DD`，UTC）。**必须成对**给出；给出时覆盖 `days`。 |
| `all_members_topic_id` | 泳道修正模式（全量成员口径）：给出时额外返回该话题在窗口外成员的 `early_members`；不给 = 编排画布口径（响应无该字段）。话题须属本版块，否则 404。 |
| `omit_embeddings` | `1` = 轻量取数，**不返回每条 section 的 `embedding`**（见下方「轻量取数」）。仅曲线/可选池统计路径使用，画布取数必须保留向量。 |

Response `data`（节选关键字段）：

```json
{
  "sections": [
    {
      "id": 7345,
      "report_id": 311,
      "period_date": "2026-11-02T00:00:00Z",
      "cluster_label": "霍尔木兹油轮遇袭",
      "embedding": [0.0123, "..."],
      "persistent_topic_id": 103,
      "topic_match_confidence": "anchor_hit",
      "persistent_topic": { "id": 103, "label": "中东局势", "status": "active" },
      "clue_count": 2
    }
  ],
  "relations": [{ "from_id": 7345, "to_id": 7409, "distance": 0.12, "relation_type": "identity" }],
  "candidate_chains": [{ "key": "1984:7345", "topic_id": 1984, "topic_status": "candidate", "section_ids": [7345, 7409], "first_date": "2026-10-27", "last_date": "2026-10-30", "span_days": 3 }],
  "match_threshold": 0.3,
  "window": { "from": "2026-10-27", "to": "2026-11-02", "days": 7, "latest_completed_date": "2026-11-02" },
  "early_members": [{ "id": 6102, "period_date": "2026-09-30T00:00:00Z", "cluster_label": "…", "persistent_topic_id": 103, "embedding": [0.0088, "..."] }]
}
```

- **`sections[].clue_count`**（unify-lane-workbench D2）：该 section 的**非综合稿（非 composite）线索数**，与 `listLaneSectionThreads().Clues` 同源——`scope=composite` 的综合稿不计入，**不是**文章数也不是 section 数。无 thread 的 section 为**已知 0**，因此该字段**不加 omitempty、0 也必定返回**（`"clue_count": 0`）。前端只把「字段缺失 / `null` / 非有限数 / 负数」判为**未知**（显示未知而不是 0），0 是合法值原样保留；旧后端无该字段时前端降级为未知，不补 0。查询失败使整个请求失败，不以 0 冒充。
- **`early_members[]`**：仅 `all_members_topic_id` 给出时出现，为该话题报告期早于窗口首日的成员（升序，同样只算向量可用者），带 `embedding` 供修正工作面的质心把窗口外成员算进去；**字段整体缺失**（= 前端未请求全量口径，与「空数组」不同）时前端计数回退只显示窗口数。
- **`window`**：服务端实际生效的窗口回显（`from`/`to` 可为 `null` = 无锚点 / 全历史档），前端不自行推算末日。

**轻量取数（`omit_embeddings=1`）**：曲线/可选池只需要 `period_date + persistent_topic_id + clue_count`，而每条 section 的向量实占 10,244 B，为画一条曲线拉全年向量不划算。该档位下 `sections[].embedding` 缺省（前端归一为「无向量」，不参与质心）。

实测（2026-11-16，版块 15220，`days=365`）：含向量 `25,772,225 B / 1.45 s`；`omit_embeddings=1` `553,570 B / 0.55 s` —— **压缩约 46.8 倍**。曲线路径 MUST 走轻量档；含向量档 365 天已逼近 30 MB 预算，只允许画布按需使用。

**窗口上限与错误码**（`composeWindowMaxDays = repository.ComposeWindowMaxDays = 365`，前后端三处同源）：

| 情形 | 状态码 | `error` |
| --- | --- | --- |
| `from`/`to` 只给一个 | 400 | `from and to must be provided together` |
| 日期格式非法 | 400 | `invalid from date "…": expected YYYY-MM-DD`（`to` 同形） |
| 起晚于止 | 400 | `from must not be later than to` |
| 跨度超过 365 天（含首末） | 400 | `date range must not exceed 365 days` |
| `from` 早于取数下界（最新 completed 日 − 364 天） | 400 | `from is outside the fetchable window (earliest YYYY-MM-DD)` |
| `all_members_topic_id` 非法 | 400 | `invalid all_members_topic_id` |
| `all_members_topic_id` 指别的版块的话题 | 404 | `topic not found on this board` |

超限一律报错，**不静默截断**（前端自选起止同一口径，见 `standard/frontend` 与 `useLaneCompose.ts`）。

> **没有单独的「365 天可选池端点」**：新增泳道的 365 日可选池曲线与编辑泳道的本泳道曲线，各自复用本端点（前者 `days=365&omit_embeddings=1` 后在前端按非 active section 逐日聚合，后者走 `GET /semantic-boards/:id/lane-dynamics?days=365`）。不要为该用途新增端点或迁移。

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
