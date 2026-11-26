## Purpose

版块域的对外接口契约，覆盖后端端点与前端交互两层：①版块 CRUD 与配置（description 变更必须刷新 embedding、匹配参数可调）；②版块文章列表（按 feed/时间/辅助标签筛选、分页排序、归档口径、AI 判定状态、按日计数）；③升级建议的接口面（查看候选、触发、DTO、逐项处理与重新生成、回填触发、rematch）；④前端编辑对话框（字段、direction_mismatch 默认隐藏、MatchDetailPanel 展示）。本族只规定「接口长什么样」；聚类与判定算法见 `board-upgrade`。

## ADDED Requirements

### Requirement: 板块 CRUD API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供 SemanticBoard（semantic_labels label_type=board）的增删改查 API，包括列表、详情、手动创建、编辑、删除。API SHALL 使用 `/api/semantic-boards` 命名空间。

#### Scenario: 手动创建板块
- **WHEN** 用户通过 API 创建板块 "量子计算"
- **THEN** 系统 SHALL 创建 semantic_label（label_type="board", source="manual", protected=true），生成 embedding

#### Scenario: 列出板块
- **WHEN** 用户请求板块列表
- **THEN** 系统 SHALL 返回所有 label_type="board" 且 status="active" 的 semantic_labels，包含 ref_count 和 tag_count

### Requirement: updateBoard refreshes embedding on description change

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->


PUT /api/semantic-boards/:id SHALL 当 description 变更时重新生成 board embedding（输入 label + description）。当前仅在 label 变更时生成 embedding。

#### Scenario: description changed
- **WHEN** updateBoard called with new description value
- **THEN** board embedding regenerated from `new_label + ". " + description`, saved to semantic_labels.embedding

#### Scenario: label changed
- **WHEN** updateBoard called with new label value
- **THEN** board embedding regenerated from `new_label + ". " + description`（统一输入格式）

#### Scenario: neither changed
- **WHEN** updateBoard called without label or description changes
- **THEN** embedding NOT regenerated

### Requirement: 辅助标签池查询 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供辅助标签池的查询 API，支持按 ref_count 排序、按 label 搜索、查看 aliases 和 merge 历史、禁用辅助标签、手动合并 alias。

#### Scenario: 查看辅助标签列表
- **WHEN** 用户请求辅助标签列表
- **THEN** 系统 SHALL 返回所有 label_type="auxiliary" 的 semantic_labels，包含 ref_count 和 aliases

#### Scenario: 禁用辅助标签
- **WHEN** 用户调用辅助标签禁用 API
- **THEN** 系统 SHALL 将该辅助标签 status 更新为 "disabled"

#### Scenario: 手动合并辅助标签 alias
- **WHEN** 用户调用辅助标签合并 API，将 source 合并到 target
- **THEN** 系统 SHALL 迁移 source 的 tag 关联，并将 source label 加入 target aliases

### Requirement: 升级建议 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供查看当前升级候选（ref_count ≥ 5 的辅助标签 + 聚类结果）和触发 LLM 升级建议的 API。

#### Scenario: 查看当前升级候选

- **WHEN** 用户请求升级候选列表
- **THEN** 系统 SHALL 返回 ref_count ≥ 5 的未升级辅助标签及其预聚类结果

#### Scenario: 触发 LLM 升级建议

- **WHEN** 用户通过 API 触发升级建议
- **THEN** 系统 SHALL 执行聚类 + LLM 判断流程，返回建议结果供用户确认/拒绝

### Requirement: 升级建议 DTO 包含标签名称和板块名称

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

升级建议 API 的响应 SHALL 在每条建议中包含 `auxiliary_labels`（数组，每个元素含 id 和 label）和 `target_board_label`（字符串，当 decision=merge_into_existing 时）。原有 `auxiliary_label_ids` 和 `target_board_id` 字段 SHALL 保留用于执行操作。

#### Scenario: create_new 建议包含辅助标签名称
- **WHEN** LLM 返回 create_new 建议，auxiliary_label_ids=[1, 5, 12]
- **THEN** 响应 SHALL 包含 auxiliary_labels=[{id:1, label:"AI"}, {id:5, label:"半导体"}, {id:12, label:"大模型"}]

#### Scenario: merge_into_existing 建议包含板块名称
- **WHEN** LLM 返回 merge_into_existing 建议，target_board_id=3
- **THEN** 响应 SHALL 包含 target_board_label="AI与机器学习"

#### Scenario: skip 建议不包含额外名称
- **WHEN** LLM 返回 skip 建议
- **THEN** auxiliary_labels 和 target_board_label SHALL 为空/省略

#### Scenario: 查看升级候选
- **WHEN** 用户请求升级候选列表
- **THEN** 系统 SHALL 返回 ref_count ≥ 5 的未升级辅助标签及其预聚类结果

#### Scenario: 触发 LLM 升级建议
- **WHEN** 用户通过 API 触发升级建议
- **THEN** 系统 SHALL 执行聚类 + LLM 判断流程，返回建议结果供用户确认/拒绝

#### Scenario: 确认执行升级建议
- **WHEN** 用户通过 API 确认 create_new 或 merge_into_existing 建议
- **THEN** 系统 SHALL 写入 SemanticBoard 和 board_composition，并返回执行结果

### Requirement: 升级建议面板支持逐项处理和重新生成

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

前端 SHALL 在升级建议面板中支持逐项处理 LLM 建议。用户确认某个 create_new 或 merge_into_existing 建议成功后，面板 SHALL 保持打开，并将该建议从待处理列表移除或标记为已处理，不得自动关闭整轮建议面板。前端 SHALL 提供重新生成升级建议的操作入口，允许用户在已有建议列表存在时重新调用升级建议 API 并替换当前建议列表。

#### Scenario: 确认单个建议后继续处理剩余建议
- **WHEN** 面板中存在多个升级建议，用户确认其中一个 create_new 或 merge_into_existing 建议且 API 返回成功
- **THEN** 面板 SHALL 继续保持打开，并保留剩余未处理建议供用户继续确认

#### Scenario: 重新生成升级建议
- **WHEN** 面板中已经存在升级建议，用户点击重新生成
- **THEN** 前端 SHALL 重新调用 POST /api/semantic-boards/upgrade-suggest，并用新的建议结果替换当前建议列表

#### Scenario: 处理完成后提示回填
- **WHEN** 用户确认执行至少一个升级建议
- **THEN** 前端 SHOULD 提示用户可手动触发匹配回填，使历史 tag-board 归属按最新 board composition 生效

### Requirement: 回填触发 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供手动触发回填的 API 端点，支持 all、unassigned、board 三种模式，并提供进度查询。

#### Scenario: 触发回填
- **WHEN** 用户调用 POST /api/semantic-boards/backfill
- **THEN** 系统 SHALL 将待回填的 tag 入队，返回任务 ID

#### Scenario: 查询回填进度
- **WHEN** 用户请求回填任务状态
- **THEN** 系统 SHALL 返回 total、processed、failed、status

### Requirement: Board embedding backfill API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->


POST /api/semantic-boards/backfill-embeddings SHALL 为所有 embedding IS NULL 且 label_type='board' 的板块生成 embedding（输入 `label + ". " + description`，description 为空时仅用 label），返回 backfill 数量。

#### Scenario: backfill null embeddings
- **WHEN** backfill-embeddings called
- **THEN** all boards with NULL embedding get embedding generated from `label + ". " + description`, count returned

### Requirement: Board rematch API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->


POST /api/semantic-boards/rematch-all SHALL 查询所有在 `topic_tag_board_labels` 中有记录的 tag，逐个调用 `MatchTopicTag` 重新匹配。用于 backfill embedding 后刷新 direction_mismatch 标记。

#### Scenario: rematch after backfill
- **WHEN** rematch-all called
- **THEN** all tags with existing board labels are re-matched, topic_tag_board_labels records updated with current direction_mismatch values

#### Scenario: partial failure
- **WHEN** individual tag rematch fails
- **THEN** log error, continue with remaining tags, return success/failure counts

### Requirement: getBoardArticles supports direction_mismatch filtering

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->


GET /api/semantic-boards/:id/articles SHALL 支持 query param `show_direction_mismatch`。默认 `false`，filtered_tags 排除 direction_mismatch=true 的标签；`true` 时包含全部。

#### Scenario: default (hide direction mismatch)
- **WHEN** request without show_direction_mismatch param
- **THEN** filtered_tags SQL adds `AND NOT COALESCE(tbl.direction_mismatch, false)` condition

#### Scenario: show direction mismatch
- **WHEN** request with show_direction_mismatch=true
- **THEN** filtered_tags returns all tags regardless of direction_mismatch

### Requirement: 匹配参数配置 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供读取和修改匹配参数的 API，参数存储在 ai_settings 表中。

#### Scenario: 读取参数
- **WHEN** 用户请求 GET /api/semantic-boards/matching-config
- **THEN** 系统 SHALL 返回当前 semantic_board_match_sim_threshold, semantic_board_match_direct_hit_rate, semantic_board_match_direct_max_sim, semantic_board_match_weight_sim, semantic_board_match_weight_density, semantic_board_match_weighted_threshold, semantic_board_match_max_boards, semantic_board_match_direct_max_sim_min_hits（默认 2）, semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3）, semantic_board_match_min_effective_sample（默认 3）, semantic_board_match_hit_rate_sim_blend（默认 0.7）, semantic_board_match_direction_sim_threshold（默认 0.5）

#### Scenario: 修改参数
- **WHEN** 用户通过 PUT /api/semantic-boards/matching-config 修改 semantic_board_match_sim_threshold 为 0.7
- **THEN** 系统 SHALL 更新 ai_settings 中的对应配置，后续匹配使用新值

#### Scenario: 修改 direction_sim_threshold
- **WHEN** 用户通过 PUT /api/semantic-boards/matching-config 修改 semantic_board_match_direction_sim_threshold 为 0.6
- **THEN** 系统 SHALL 更新 ai_settings 中的对应配置，后续方向校验使用 0.6

### Requirement: 标签关联的辅助标签和板块查询

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供查询 tag 关联的辅助标签列表和所属板块列表的 API。

#### Scenario: 查看 tag 的辅助标签
- **WHEN** 用户请求 tag 的辅助标签
- **THEN** 系统 SHALL 返回该 tag 通过 topic_tag_semantic_labels 关联的所有 semantic_labels

#### Scenario: 查看 tag 的板块
- **WHEN** 用户请求 tag 的板块归属
- **THEN** 系统 SHALL 返回该 tag 通过 topic_tag_board_labels 关联的所有 label_type="board" 的 semantic_labels，按匹配分排序

### Requirement: Board composition 管理 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供查看和修改 SemanticBoard 构成辅助标签的 API，支持从 board_composition 移除辅助标签。

#### Scenario: 查看 board composition
- **WHEN** 用户请求 SemanticBoard 的构成标签
- **THEN** 系统 SHALL 返回该 board 的所有辅助标签、ref_count 和 aliases

#### Scenario: 移除 board 构成标签
- **WHEN** 用户从 SemanticBoard 中移除辅助标签
- **THEN** 系统 SHALL 删除对应 board_composition 记录，且不自动回填历史 tag-board 归属

### Requirement: 辅助标签推荐 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供基于 embedding 相似度的辅助标签推荐 API，用于手动创建或编辑 SemanticBoard 时人工填充 board_composition。API SHALL 接受 label（必选）和 description（可选），生成 board label + description embedding 后与全量 active 辅助标签的 storage embedding（semantic_labels.embedding）计算余弦相似度，按相似度从高到低排序返回，不设阈值。推荐结果 SHALL 仅供用户选择，不自动写入 board_composition，也不参与自动 tag-board 匹配规则。API SHALL 支持分页（page/page_size）和搜索过滤（排除已在 board_composition 中的标签）。

#### Scenario: 创建时推荐辅助标签
- **WHEN** 用户调用 GET /api/semantic-boards/suggest-auxiliaries?label=量子计算
- **THEN** 系统 SHALL 生成 "量子计算" 的 board 查询 embedding，与所有 active 辅助标签的 storage embedding 计算相似度，按相似度降序返回分页列表，包含 id、label、ref_count、aliases、similarity

#### Scenario: 编辑时推荐辅助标签
- **WHEN** 用户调用 GET /api/semantic-boards/:id/suggest-auxiliaries
- **THEN** 系统 SHALL 使用已有 board 的 label+description 生成 embedding，返回推荐列表，排除已在 board_composition 中的辅助标签

#### Scenario: 搜索过滤
- **WHEN** 用户调用 GET /api/semantic-boards/suggest-auxiliaries?label=AI&search=图像
- **THEN** 系统 SHALL 先按搜索词过滤辅助标签（label/slug 模糊匹配），再计算相似度排序

### Requirement: Board composition 添加 API

<!-- from: board-management-api (2026-05-26 board-interaction-overhaul) -->

系统 SHALL 提供向 SemanticBoard 的 board_composition 中添加辅助标签的 API。

#### Scenario: 添加单个辅助标签
- **WHEN** 用户调用 POST /api/semantic-boards/:id/composition，body 含 auxiliary_label_id
- **THEN** 系统 SHALL 验证辅助标签存在且 active，写入 board_composition 记录（幂等），且不自动回填历史 tag-board 归属

#### Scenario: 添加后需要用户手动回填
- **WHEN** 用户向 SemanticBoard 添加辅助标签
- **THEN** 系统 SHALL NOT 自动启动回填；前端 SHOULD 提示用户可手动触发 board 模式回填使历史 tag-board 归属生效

### Requirement: 板块文章列表独立 API

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->

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

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->

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

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->

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

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->

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

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->


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

<!-- from: board-article-api (2026-05-26 board-interaction-overhaul) -->


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

### Requirement: Board editing dialog

<!-- from: board-editing-ui (2026-05-29 board-direction-check-and-board-editing) -->


雷达列表页（L0 `/tags`）每个版块行 SHALL 提供编辑入口：行尾「⋯」操作菜单中的「编辑版块」项，点击弹出编辑对话框。对话框 SHALL 支持修改 label 与 description（并 SHALL 支持 display_order 与 protected），调用已有 `updateBoard` API。保存成功后 SHALL 刷新板块信息。

该入口 SHALL 是版块本体编辑在**版块列表页**的可达路径——L0 雷达为版块列表所在处，用户 SHALL NOT 需要先进入版块态势页并下钻到治理工作面的第 N 个页签才能改名。

行尾「⋯」按钮 SHALL NOT 触发行点击进入版块态势页（须阻止事件冒泡）。

保存成功后，系统 SHALL 使版块 embedding 的方向校验使用新向量（缓存失效由后端保证，见 board-match-cache 能力）；对话框 SHALL NOT 声称已重跑版块匹配（匹配表重跑保持手动触发）。

#### Scenario: 雷达行提供编辑入口

- **GIVEN** 用户在 L0 雷达页看到版块「AI 前沿」一行
- **WHEN** 用户点该行行尾「⋯」并选「编辑版块」
- **THEN** 系统 SHALL 打开版块编辑对话框，且 SHALL NOT 触发整行跳转进入版块态势页

#### Scenario: edit board label

- **WHEN** user 在雷达行编辑对话框修改 label 并保存
- **THEN** updateBoard API called with new label, board list refreshed, board embedding regenerated

#### Scenario: edit board description

- **WHEN** user 在雷达行编辑对话框修改 description 并保存
- **THEN** updateBoard API called with new description, board embedding regenerated

#### Scenario: 行尾操作不误触发跳转

- **WHEN** 用户点击行尾「⋯」按钮
- **THEN** 系统 SHALL NOT 打开该版块的态势页，SHALL NOT 触发 openBoard 副作用

### Requirement: Default hide direction_mismatch tags

<!-- from: board-editing-ui (2026-05-29 board-direction-check-and-board-editing) -->


TagsPage 的 tag chip 列表默认不显示 direction_mismatch=true 的标签。提供"显示方向不符"开关（checkbox 或 toggle），开启后显示这些标签，用虚线边框 + "⊘" 后缀标记。

#### Scenario: default view
- **WHEN** article has 3 filtered_tags, 1 with direction_mismatch=true
- **THEN** only 2 tags shown in chip list

#### Scenario: show direction mismatch toggle
- **WHEN** user enables "显示方向不符" toggle
- **THEN** all 3 tags shown, direction_mismatch tag has dashed border and "⊘" suffix

### Requirement: MatchDetailPanel direction check display

<!-- from: board-editing-ui (2026-05-29 board-direction-check-and-board-editing) -->


MatchDetailPanel 步骤 ④（max_sim）展示方向校验结果：direction_sim 值和是否通过。direction_mismatch=true 时显示警告。

#### Scenario: direction check passed
- **WHEN** max_sim matched AND direction_sim >= threshold
- **THEN** step ④ shows "方向校验 ✓ sim=0.72≥0.5"

#### Scenario: direction check failed
- **WHEN** max_sim matched AND direction_sim < threshold
- **THEN** step ④ shows "⚠ 方向不符 sim=0.35<0.5"

### Requirement: API type updates

<!-- from: board-editing-ui (2026-05-29 board-direction-check-and-board-editing) -->
<!-- kind: impl -->


BoardArticleTag 接口新增 `direction_mismatch: boolean`。MatchDetailResponse 新增 `direction_sim: number | null`。

#### Scenario: type definitions updated
- **WHEN** frontend types are updated
- **THEN** BoardArticleTag has direction_mismatch field, MatchDetailResponse has direction_sim field
