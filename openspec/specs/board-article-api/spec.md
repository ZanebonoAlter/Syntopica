<!-- domain: semantic-board, api -->

## Purpose
SemanticBoard 文章列表独立 API，支持按 feed、时间范围、辅助标签筛选，每篇文章返回 feed_name 和 filtered_tags。

## Requirements

### Requirement: 板块文章列表独立 API
系统 SHALL 提供 `GET /api/semantic-boards/:id/articles` 端点，返回属于该 SemanticBoard 的文章列表。每篇文章 SHALL 包含 feed_name（来自 feeds 表 JOIN）和 filtered_tags（通过 topic_tag_board_labels 过滤，只返回属于当前 board 的 event/person/keyword 标签，含 id/label/category）。

#### Scenario: 按 board 查询文章列表
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?page=1&per_page=20`
- **THEN** 系统 SHALL 返回属于 SemanticBoard #5 的文章列表，每篇文章包含 feed_name 和 filtered_tags

#### Scenario: 按 feed 筛选
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?feed_id=10`
- **THEN** 系统 SHALL 只返回 feed_id=10 且属于 SemanticBoard #5 的文章

#### Scenario: 按时间范围筛选
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?start_date=2026-05-20&end_date=2026-05-25`
- **THEN** 系统 SHALL 只返回 pub_date 在 2026-05-20 至 2026-05-25 之间且属于 SemanticBoard #5 的文章

#### Scenario: 按辅助标签筛选
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?auxiliary_label_id=12`
- **THEN** 系统 SHALL 只返回关联了 auxiliary_label_id=12 的 tag 且该 tag 属于 SemanticBoard #5 的文章

### Requirement: filtered_tags 按板过滤
每篇文章的 filtered_tags SHALL 只包含通过 topic_tag_board_labels 确认属于当前 board 的标签。标签 SHALL 包含 id、label、category、match_reason、score 字段。属于 event/person/keyword 三种 category 的标签均应包含。

match_reason 表示该 tag 被归类到当前 board 的匹配规则（direct_hit / hit_rate / max_sim / weighted），score 表示匹配得分。

#### Scenario: 文章属于多个 board 时标签过滤（含匹配信息）
- **WHEN** 文章 #101 有 tags [GPT-5发布, AI竞赛, 科技股]，其中 GPT-5发布(score=0.85, match_reason="max_sim") 和 AI竞赛(score=0.92, match_reason="hit_rate") 通过 topic_tag_board_labels 归属于 board #5，科技股(score=1.0, match_reason="direct_hit") 归属于 board #8
- **THEN** 在 board #5 的文章列表中，文章 #101 的 filtered_tags SHALL 为 [{id:1, label:"GPT-5发布", category:"event", match_reason:"max_sim", score:0.85}, {id:5, label:"AI竞赛", category:"event", match_reason:"hit_rate", score:0.92}]
- **THEN** 在 board #8 的文章列表中，文章 #101 的 filtered_tags SHALL 为 [{id:9, label:"科技股", category:"keyword", match_reason:"direct_hit", score:1.0}]

#### Scenario: 文章无 board 归属标签
- **WHEN** 文章 #200 有 tags 但没有通过 topic_tag_board_labels 归属于 board #5 的标签
- **THEN** 该文章 SHALL NOT 出现在 board #5 的文章列表中

### Requirement: 分页和排序
系统 SHALL 支持 page/per_page 分页参数，默认按 quality 排序。返回 SHALL 包含 pagination 信息（page, per_page, total, pages）。

排序 key: `(tier ASC, score DESC, publish_time DESC)`。文章有多个 tag 时取 tier 最高的作为排序依据。排序在 Go 端内存中完成。

`sort` 查询参数支持两种模式：
- `quality`（默认）：按 tier + score + pub_date 排序
- `time`：DB 直接按 `pub_date DESC, id DESC` 排序，跳过内存质量排序

#### Scenario: 分页查询
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?page=2&per_page=10`，共有 25 篇文章
- **THEN** 系统 SHALL 返回第 11-20 篇文章，pagination.total=25，pagination.pages=3

#### Scenario: 默认质量排序
- **WHEN** 请求 `GET /api/semantic-boards/5/articles` 不指定 sort 参数
- **THEN** 文章 SHALL 按 (tier ASC, score DESC, pub_date DESC) 排序，direct_hit 排最前，weighted 排最后

#### Scenario: 时间排序模式
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?sort=time`
- **THEN** 文章 SHALL 按 pub_date DESC 排序，不使用内存质量排序

#### Scenario: 多 tag 文章取最高 tier
- **WHEN** 文章 #101 的 filtered_tags 含 direct_hit(tier=0) 和 weighted(tier=3)
- **THEN** 该文章按 tier=0 参与排序

### Requirement: 归档口径的入日报标记与匹配证据
`GET /api/semantic-boards/:id/articles` 返回的每篇文章 SHALL 附带：`in_report`（该文章 id 是否出现在查询窗口内任一日报分区线索的 `related_article_ids` 中）与既有 `filtered_tags` 的匹配证据（`match_reason`、`score`、`direction_mismatch`）；`filtered_tags` 每项 SHALL 另带该标签的 `description`（取 `topic_tags.description` 既有字段，随现有 JOIN 返回，缺失时为空串）。端点 SHALL 支持 `report_status` 过滤（`all` 默认 / `not_in_report` / `weak`），其中 `weak` 表示无匹配标签或匹配规则为 `max_sim`（降级标记已退役，不再有「已降级」这一独立弱证据类别）。`sort=quality` SHALL 返回**全局**质量序（跨分页稳定）：排序键（文章最优证据层级与对应分数）下推到查询层计算，先全局排序后分页；无可见匹配证据的文章排在有证据文章之后。口径实现 SHALL 复用既有日报分区线索与匹配结果数据，MUST NOT 在接口层复制匹配公式。

#### Scenario: 质量序跨分页稳定

- **WHEN** 窗口内文章数超过单页上限且分页拉取 `sort=quality`
- **THEN** 任意页首篇文章的证据层级 SHALL 不劣于其后任何页的文章，拼接后即为全局质量序

#### Scenario: 标记未入日报
- **WHEN** 版块近 6 份日报的分区线索共覆盖 12 篇文章，列表返回 20 篇
- **THEN** 其中 12 篇 `in_report` SHALL 为 true，其余 8 篇为 false

#### Scenario: 标签描述随标签返回
- **WHEN** 文章命中「霍尔木兹海峡」标签且该标签在 topic_tags 有 description
- **THEN** `filtered_tags` 中该项 SHALL 携带该 description 原文；标签无 description 时 SHALL 为空串而非字段缺失

#### Scenario: 按遗漏过滤
- **WHEN** 请求带 `report_status=not_in_report`
- **THEN** 返回的文章 SHALL 全部为 `in_report=false`

#### Scenario: 弱证据过滤
- **WHEN** 请求带 `report_status=weak`
- **THEN** 返回的文章 SHALL 仅含无匹配标签或 `match_reason=max_sim` 的条目

### Requirement: 版块文章按日计数

系统 SHALL 提供 `GET /api/semantic-boards/:id/article-daily-counts` 端点，返回该版块命中文章按自然日的聚合数，供归档时间曲线与导航条使用。

返回结构 SHALL 为 `{ success: true, data: { earliest_date: string|null, latest_date: string|null, days: [{ date: string, count: number }] } }`：

- `earliest_date` / `latest_date` SHALL 为该版块命中文章的最早 / 最新自然日；版块无命中文章时两者均为 `null`、`days` 为空数组；
- `days` SHALL 为**稀疏**数组，只包含 `count > 0` 的日期，按 `date` 升序；缺失日期由调用方按已知 0 处理，端点 MUST NOT 补齐全区间零值。

成员口径 SHALL 与 `GET /api/semantic-boards/:id/articles` 主查询逐字一致：文章与版块标签集合（`article_topic_tags` ⋈ `topic_tag_board_labels.semantic_board_id`）的命中关系、`feeds` 内连接、日期按 `DATE(articles.pub_date)` 归日；计数 SHALL 按文章 id 去重（一篇文章命中多个版块标签仍只计 1）。

端点 SHALL 为固定口径聚合：MUST NOT 接受或应用 `feed_id`、`auxiliary_label_id`、`report_status`、`show_direction_mismatch`、分页与日期窗口筛选；方向校验未通过的文章（默认在列表行内隐藏其标签）SHALL 仍计入当日的 `count`，以与归档列表的「当天篇数」对齐。

#### Scenario: 与列表当天篇数一致

- **WHEN** 归档查询 `start_date=2026-11-18&end_date=2026-11-18` 返回 145 篇
- **THEN** 该端点返回的 `days` 中 `2026-11-18` 的 `count` SHALL 等于 145

#### Scenario: 一篇文章命中多个版块标签只计一次

- **WHEN** 文章 #101 同时命中本版块的两个标签
- **THEN** 该文章所在日期 SHALL 只计 1 篇

#### Scenario: 稀疏返回

- **WHEN** 版块历史为 2026-11-01 至 2026-11-18，其中 11-03 与 11-04 无命中文章
- **THEN** `days` SHALL 不含 11-03 与 11-04，`earliest_date` 为 2026-11-01，`latest_date` 为 2026-11-18

#### Scenario: 无命中文章

- **WHEN** 版块没有任何命中文章
- **THEN** 端点 SHALL 返回 `earliest_date: null`、`latest_date: null`、`days: []`，HTTP 状态 SHALL 为 200

#### Scenario: 不受筛选影响

- **WHEN** 请求带 `report_status=weak` 或 `feed_id=10`
- **THEN** 结果 SHALL 与不带这些参数时一致（参数不参与该端点口径）

### Requirement: 版块文章列表携带疑似 AI 判定状态

`GET /api/semantic-boards/:id/articles` 返回的每篇文章 SHALL 附带该文章的 AI 生成疑似度判定状态字段（未判定／疑似／已确认非 AI）与置信分。判定状态为未判定时该字段 SHALL 仍存在于响应项中（值为未判定），SHALL NOT 省略字段。

该字段 SHALL 为非正文类小字段，SHALL NOT 携带正文内容；接口 SHALL NOT 因此新增筛选参数。

#### Scenario: 归档列表项携带判定状态

- **WHEN** 客户端请求 `GET /api/semantic-boards/5/articles`
- **THEN** 每篇文章的响应项 SHALL 携带该文章的 AI 生成疑似度判定状态字段

#### Scenario: 未判定文章字段不被省略

- **WHEN** 列表中某篇文章判定状态为未判定
- **THEN** 该文章响应项 SHALL 仍携带判定状态字段（值为未判定），SHALL NOT 缺失该字段

#### Scenario: 判定字段不改变排序与筛选口径

- **WHEN** 客户端以既有筛选参数与排序参数请求版块文章列表
- **THEN** 返回结果的成员与顺序 SHALL NOT 因疑似 AI 判定状态而改变
