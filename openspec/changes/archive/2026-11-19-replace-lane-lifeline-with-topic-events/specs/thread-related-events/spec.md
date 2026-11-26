## ADDED Requirements

### Requirement: 线索相关事件读模型

系统 SHALL 提供 `GET /api/daily-reports/:reportId/thread-relations`，一次返回该期日报**所有线索**的相关推荐，按 `thread_id` 分组——MUST NOT 为每条线索各开一个端点或让前端发 N 个请求。

参数：

- `days`：窗口天数，只接受 `7` 与 `30`；其它值 SHALL 回落 `7`，响应 SHALL 以 `days` 字段回显实际生效值。
- 窗口 SHALL 为 `[report_date - days, report_date - 1]`，**不含当前期**。

响应 SHALL 包含 `report_id` / `days` / `window_start` / `window_end` / `status` / `threads`（`{ "<threadId>": [推荐项] }`）/ `total` / `skipped_no_embedding`。没有任何线索有推荐时 `threads` SHALL 为空对象（`{}`，不是 `null`），`status` 为 `empty`。

`reportId` 不存在时 SHALL 返回 404。

#### Scenario: 查询 7 天窗口

- **WHEN** 客户端以 `days=7` 请求一份 `period_date=2026-11-17` 的日报
- **THEN** 响应 `window_start` SHALL 为 `2026-11-10`、`window_end` SHALL 为 `2026-11-16`
- **AND** 当期（11.17）的线索 MUST NOT 作为其它线索的推荐来源

#### Scenario: 非法天数回落并回显

- **WHEN** 客户端以 `days=14` 请求
- **THEN** 系统 SHALL 以 7 天窗口查询
- **AND** 响应 `days` SHALL 为 `7`

#### Scenario: 日报不存在

- **WHEN** 请求不存在的 report id
- **THEN** 系统 SHALL 返回 404

#### Scenario: 无任何推荐

- **WHEN** 窗口内没有任何线索与当期线索相似度达到阈值
- **THEN** 响应 `threads` SHALL 为 `{}`（非 `null`）、`status` SHALL 为 `empty`

### Requirement: 候选集与相似度打分

候选 SHALL 限定为：窗口内、同一 `persistent_topic_id` 的其它 section 所辖 thread 中，`embedding` 与当前 thread `embedding` 的余弦相似度 ≥ **0.75** 的线索。

每条推荐 SHALL 给出 `thread_id` / `title` / `summary` / `period_date` / `score` / `level` / `confirmed` / `source_article`。

`score` SHALL 等于余弦相似度 `1 - (a <=> b)` 本身，MUST NOT 叠加时间衰减、篇数权重或任何二次加权。`level` SHALL 为 `score >= 0.85 ? "strong" : "related"`。

系统 SHALL NOT 使用事件标签向量、MUST NOT 调用 embedding 接口、MUST NOT 写 `related_event_embeddings`、MUST NOT 复用 `related_event_similarity_threshold`。候选匹配只读已存在的 `daily_report_threads.embedding`。

系统 SHALL NOT 对候选做语义归并：每条候选线索 SHALL 独立成一条推荐。

#### Scenario: 同话题高相似线索入选

- **WHEN** 窗口内某历史线索与当前线索的余弦相似度为 0.890
- **THEN** 该线索 SHALL 出现在当前线索的推荐列表中，`score=0.890`、`level="strong"`

#### Scenario: 低于阈值不入选

- **WHEN** 某历史线索与当前线索的余弦相似度为 0.72
- **THEN** 该线索 MUST NOT 出现在推荐列表中

#### Scenario: 分数不做二次加权

- **WHEN** 两条候选的余弦相似度分别为 0.86（较旧）与 0.80（较新）
- **THEN** `score` SHALL 分别为 0.86 与 0.80，MUST NOT 因时间远近被改写

#### Scenario: 不做语义归并

- **WHEN** 两条候选线索内容高度相似
- **THEN** 它们 SHALL 各自出现，MUST NOT 合并为一条

### Requirement: 排序与截断

推荐列表 SHALL 按 `score` 降序 → `period_date` 降序 → `thread_id` 升序排列。响应 SHALL 返回该线索的**全部**合格推荐（前端只渲染前 3 个），MUST NOT 在后端静默截断。

#### Scenario: 稳定排序

- **WHEN** 两条候选的 `score` 与 `period_date` 都相同
- **THEN** 它们 SHALL 按 `thread_id` 升序排列，同一输入两次查询结果 SHALL 等价

#### Scenario: 全部返回

- **WHEN** 某线索有 5 条合格推荐
- **THEN** 响应 SHALL 含 5 条，前端渲染前 3 条并把剩余折叠为「还有 2 条」

### Requirement: 来源文章与已确认标记

每条推荐 SHALL 携带 `source_article`：取该候选 thread 的 `related_article_ids` **首篇**，一次批量查 `articles` 得到 `id` / `title` / `feed` / `published_at`；查不到时 `source_article` SHALL 为 `null`（不编造标题）。

`confirmed` SHALL 为 `true` 当且仅当该候选 thread 的 `tag_ids` 中存在 event 标签，且该标签在本话题有一条 `state='confirmed'` 的注记（`topic_key_event_markers.tag_ids_snapshot` 命中）。读模型 MUST NOT 修改注记的任何字段或状态。

#### Scenario: 已确认线索加星

- **WHEN** 某候选 thread 的 event 标签在本话题有 confirmed 注记
- **THEN** 该推荐项 `confirmed` SHALL 为 `true`

#### Scenario: 只有 AI 建议不加星

- **WHEN** 某候选 thread 的标签只有 `state='suggested'` 或 `state='dismissed'` 的注记
- **THEN** 该推荐项 `confirmed` SHALL 为 `false`

#### Scenario: 来源文章不可用

- **WHEN** 候选 thread 的 `related_article_ids` 为空或其首篇文章已不存在
- **THEN** `source_article` SHALL 为 `null`
- **AND** 标题与摘要 SHALL 仍正常返回

### Requirement: 缺失向量的处置

当前 thread 没有 `embedding` 时，该线索 SHALL NOT 产出推荐，并计入 `skipped_no_embedding`；历史 thread 没有 `embedding` 时 SHALL NOT 进入候选池。

三态优先级 SHALL 为 `empty` → `partial` → `ready`：`total == 0` 时恒为 `empty`（即便同时有 `skipped_no_embedding > 0`）；`total > 0 && skipped_no_embedding > 0` 时为 `partial`；其余为 `ready`。MUST NOT 把含跳过项的响应标为 `ready`，MUST NOT 为凑结果而临时计算 embedding。

#### Scenario: 当期线索无向量

- **WHEN** 当期某条线索的 `embedding` 为空，且其它线索仍有推荐
- **THEN** 该线索 MUST NOT 出现在响应 `threads` 中
- **AND** `skipped_no_embedding` SHALL 大于 0、`status` SHALL 为 `partial`

#### Scenario: 全部线索缺向量时 empty 优先

- **WHEN** 当期所有线索的 `embedding` 都为空
- **THEN** `threads` SHALL 为 `{}`、`status` SHALL 为 `empty`（empty 优先于 partial）
- **AND** `skipped_no_embedding` SHALL 如实大于 0

#### Scenario: 历史线索无向量不进候选

- **WHEN** 窗口内某历史线索的 `embedding` 为空
- **THEN** 它 MUST NOT 成为任何线索的推荐项

### Requirement: 推荐读模型只读

本读模型 SHALL 只读：MUST NOT 写任何业务表、缓存、索引或注记；MUST NOT 调模型接口；MUST NOT 触达日报生成 prompt 的 `ListTopicRecentBriefs` 路径。

#### Scenario: 读请求不产生写

- **WHEN** 连续两次请求同一日报同一窗口
- **THEN** 数据库 SHALL 保持请求前状态（无新增/更新行）
- **AND** 两次响应 SHALL 等价
