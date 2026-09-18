# Source Board Hit Rate

## Purpose

回答「哪个订阅源在给我喂有用的东西」：把既有正向链路（源 → 文章 → 标签 → 板块）反查为源视角与板块视角的只读聚合，让杂音源可被定位、可被比较，并为处置（降频/关打标/退订）提供依据。

## Requirements
### Requirement: 命中口径与统计窗口

统计 SHALL 以下列口径计算，任何实现（端点、面板、调试脚本）MUST 一致：

- **命中**：一篇文章至少有一个标签经 `topic_tag_board_labels` 挂到 `label_type='board'` 且 `status='active'` 的板块，即为命中。`auxiliary` / `composite` 标签本身不是板块。
- **窗口**：`coalesce(pub_date, created_at) >= now - N 天`，N 由调用方给出；允许值 SHALL 限定为 `7` / `30` / `90`，缺省 `7`，非法值 SHALL 返回 400。窗口不设上界。
- **含已归档文章**：窗口内 `archived = true` 的文章 MUST 计入统计。理由：超出 `max_articles` 的文章会被 `CleanupOldArticles` 归档，高频源的命中文章几乎全在归档区；排除归档会得出相反结论（实测：某源近 7 天 1671 篇中 1574 篇已归档，437 篇命中全部落在归档区）。
- **按文章去重**：任何计数口径（源总量、命中数、板块分布）SHALL 以文章为单位去重，MUST NOT 因一篇文章携带多个标签或命中多个板块而重复计数。
- **未打标两分**：无标签文章 SHALL 区分「打标排队中」（存在 `pending`/`leased` 的打标任务）与「已处理无标签」（无标签且无未完成任务），便于消费方区分管线滞后与真实零标签。

#### Scenario: 一篇文章多标签多板块只计一次
- **WHEN** 一篇文章有 3 个标签，其中 2 个标签分别挂到板块 A 与板块 B
- **THEN** 该源「命中文章数」SHALL 计 1（不是 2），板块 A、板块 B 各计 1

#### Scenario: 归档文章计入窗口统计
- **WHEN** 某源近 7 天入库 100 篇，其中 80 篇因超出 `max_articles` 被归档，仅 5 篇命中板块且全部在归档区
- **THEN** 该源窗口统计 SHALL 为「总量 100、命中 5」，命中率 5%，MUST NOT 报 0

#### Scenario: 非法窗口值被拒绝
- **WHEN** 请求携带 `window=14`
- **THEN** 系统 SHALL 返回 400，不回退默认值静默统计

#### Scenario: 打标未完成与零标签可区分
- **WHEN** 某源窗口内 10 篇无标签文章，其中 7 篇有未完成打标任务、3 篇已处理完
- **THEN** 统计 SHALL 输出「打标排队中 7、已处理无标签 3」，二者 MUST NOT 合并为单一数字

### Requirement: 按订阅源聚合端点

系统 SHALL 提供只读端点 `GET /api/feeds/board-hit-stats?window=7`，返回窗口内全部订阅源的统计（不逐源 N+1 查询，单次批量聚合）。每条记录 SHALL 含：`feed_id`、`title`、`tagging_enabled`、`articles`（窗口内文章总数）、`in_board`（命中文章数）、`tagged_no_board`（有标签但未命中）、`untagged_pending`（打标排队中）、`untagged_settled`（已处理无标签）、`hit_rate`（`in_board / articles`，`articles = 0` 时为 0）、`boards`（该源命中板块及篇数列表，同一文章命中多板块时各计一次）。

端点 SHALL 为只读：不写库、不触发打标或匹配、不改变任何源状态。

#### Scenario: 返回全量源的统计
- **WHEN** 系统有 23 个订阅源，请求 `GET /api/feeds/board-hit-stats?window=7`
- **THEN** 响应 SHALL 含 23 条记录（含窗口内 0 篇的源，`articles=0`、`hit_rate=0`）

#### Scenario: 板块分布可超过命中数
- **WHEN** 某源窗口内命中 437 篇，其板块分布为 A=142、B=118、其他若干
- **THEN** `boards` 各项之和 MAY 大于 `in_board`（同一文章命中多板块时各计一次），但 `in_board` MUST 为去重后的文章数

#### Scenario: 只读端点无副作用
- **WHEN** 连续两次调用同一窗口的端点
- **THEN** 两次响应 SHALL 一致（除时间戳字段），且 MUST NOT 新增打标任务或修改任何源配置

### Requirement: 按板块聚合端点

系统 SHALL 提供只读端点 `GET /api/semantic-boards/:id/source-breakdown?window=7`，返回该板块窗口内的来源构成：`total_articles`（该板块窗口内命中文章总数，按文章去重）、`source_count`（来源数）与 `sources[]`，每条含 `feed_id`、`title`、`articles`（该源在本板块的命中篇数）、`share`（`articles / total_articles`，`total_articles = 0` 时为 0）、以及该源的 `feed_articles`（窗口内总量）与 `feed_hit_rate`（该源整体命中率）。

板块不存在或非 `board` 类型时 SHALL 返回 404。

#### Scenario: 来源篇数合计等于板块总数
- **WHEN** 板块 #5 窗口内有 437 篇命中文章，来源 8 个
- **THEN** `sources[].articles` 之和 SHALL 等于 `total_articles`（437），`source_count` 为 8

#### Scenario: 上下文列帮助识别只偶尔命中的源
- **WHEN** 某源在本板块有 12 篇，但其窗口内总量 162 篇、整体命中率 44%
- **THEN** 该来源项 SHALL 同时给出 `feed_articles=162` 与 `feed_hit_rate=0.44`

#### Scenario: 板块不存在
- **WHEN** 请求 `GET /api/semantic-boards/999999/source-breakdown`
- **THEN** 系统 SHALL 返回 404

#### Scenario: 空板块
- **WHEN** 该板块窗口内没有任何命中文章
- **THEN** 响应 SHALL 为 `total_articles=0`、`sources=[]`（正常结果，非错误）

### Requirement: 板块页板块内容内的来源构成面板

`/tags` 板块页主区 tab 栏 SHALL 保持既有五个 tab 不变（不新增「来源」tab）。「板块内容」tab（默认 tab）内、既有板块构成面板之后 SHALL 追加只读「来源构成」面板，内容 SHALL 包含：窗口分段控件（7/30/90 天，默认 7）、汇总行（本板块篇数 / 来源数 / 最大来源及其占比）、排序控件（按篇数 ↓ 默认 / 按该源入板块率 ↑）、来源表（订阅源 | 本板块篇数 | 占本板块比例 | 该源入板块率 | 该源窗口内总量）。

面板 SHALL 随选中板块变化重新拉取数据（与既有板块构成数据并行）；切换到其它 tab 时面板不渲染。面板 SHALL 为只读：MUST NOT 提供降频、关打标、退订等写动作，MUST NOT 因面板操作改变板块或订阅源状态。

#### Scenario: 显示来源构成
- **WHEN** 用户选中板块 #5，「板块内容」tab 底部展示来源构成面板
- **THEN** 面板 SHALL 按窗口展示供血源列表，每行显示本板块篇数、占比与该源自身入板块率

#### Scenario: 切换板块重新拉取
- **WHEN** 用户从板块 #5 切换到板块 #8
- **THEN** 面板 SHALL 按板块 #8 重新拉取并展示，旧板块的请求结果 MUST NOT 覆盖新选中板块的面板

#### Scenario: 空态引导切窗口
- **WHEN** 该板块近 7 天没有命中文章
- **THEN** 面板 SHALL 显示「近 7 天没有文章归入本板块」并提示可切换 30/90 天窗口

#### Scenario: 面板无写动作
- **WHEN** 用户查看任一来源行
- **THEN** 行内 SHALL NOT 出现停用、退订、改配置等动作控件（处置入口仍在设置 → 订阅源）
