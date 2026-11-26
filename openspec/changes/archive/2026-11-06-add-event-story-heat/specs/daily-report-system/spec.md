## MODIFIED Requirements

### Requirement: 日报数据模型
系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。事件数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, story_id(可空), scope(single/composite), is_development(布尔，默认 false), story_manual(布尔，默认 false), display_tier(body/flash/hidden, 默认 body), created_at。

`story_id` SHALL 指向 `board_stories` 的存活记录或经 `merged_into` 解析到存活记录；该列 SHALL 可空、SHALL NOT 设外键级联删除、SHALL NOT 参与事件的去重与展示过滤。`scope` SHALL 由 `tag_ids` 中 `category='event'` 的标签数推导：`≥2` 为 `composite`，否则为 `single`；`composite` 事件 SHALL NOT 建立或加入事件线，也 SHALL NOT 计入事件数。`is_development` SHALL 仅在判定为「同一事件的不同阶段」时置真。`story_manual` SHALL 标记该事件的身份由人工决定：人工指派时置真并写 `story_id`，人工拆分时置真且 `story_id` 保持 NULL；任何自动归属判定与历史回填 SHALL 跳过 `story_manual=true` 的事件，SHALL NOT 改写其 `story_id`。`display_tier` SHALL 为跨期去重的展示层载体（`body`=正文默认、`flash`=快讯、`hidden`=未展示预留）：身份、signals 与热度 SHALL NOT 读该列；本期日报详情读路径 SHALL 只返回 `body`（`flash`/`hidden` 丢弃不渲染，字段保留供未来快讯 UI）。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

**`highlights` SHALL 恒为 JSON 数组**：落库时 SHALL 写 `[]`（空数组），SHALL NOT 写 JSON `null` 标量——`null` 标量会让 `jsonb_array_elements_text` 抛 SQLSTATE 22023，且在读路径上被前端当作「无内容」而整块隐藏。API 读出时 SHALL 同样恒为数组（历史遗留的 `null` 值 SHALL 在读路径归一为 `[]`），SHALL NOT 向客户端返回 `"highlights": null`。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

日报记录的落库 SHALL 以 (semantic_board_id, period_date) 为冲突键执行原子 upsert：同 board 同日重复落库（无论顺序还是并发）SHALL 收敛为一条记录，SHALL NOT 抛出唯一约束冲突错误。

**同一 report 内 `cluster_index` MAY 重复**：一个聚类/话题的当日内容被拆成多条 section 时（泳道内主题分桶、手动编排、watch 物化），兄弟 section SHALL 共享同一 `cluster_index`，以 `bucket_index`（0 起、组内唯一）区分先后；(report_id, cluster_index) SHALL NOT 建唯一约束。未分桶的 section SHALL 取 `bucket_index=0`、`bucket_title=NULL`。

#### Scenario: 创建日报记录
- **WHEN** 为 SemanticBoard #5 在 2026-05-25 生成日报
- **THEN** 系统 SHALL 在 `board_daily_reports` 表创建一条记录，status="generating"，period_date="2026-05-25"，semantic_board_id=5

#### Scenario: 日报记录唯一性
- **WHEN** SemanticBoard #5 在 2026-05-25 已有一条 status="done" 的日报
- **THEN** 系统 SHALL NOT 创建重复记录，而是更新已有记录

#### Scenario: 并发落库同板同日不炸唯一约束
- **WHEN** 两条写路径（正常情况下被生成闸互斥；异常绕过闸时）先后或并发为 SemanticBoard #5 落库 2026-05-25 的日报
- **THEN** 系统 SHALL 通过原子 upsert 收敛为一条记录（后写覆盖先写），SHALL NOT 出现 duplicate key 唯一约束错误，SHALL NOT 产生部分写入的残缺日报（sections 与主记录同事务落库）

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事事件 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

#### Scenario: 兄弟桶共享聚类序号

- **GIVEN** 某聚类被泳道内主题分桶拆成 3 个桶
- **WHEN** 日报落库
- **THEN** 3 条 section SHALL 共享同一 `cluster_index`，`bucket_index` 分别为 0/1/2，`bucket_title` 各自为主题方向名

#### Scenario: 未分桶 section 取默认桶序号

- **GIVEN** 某聚类未达到分桶阈值（事件 < 6 且 tags < 10）
- **WHEN** 日报落库
- **THEN** 该 section SHALL 为 `bucket_index=0`、`bucket_title=NULL`，且其 `cluster_index` 在该 report 内仍然唯一

#### Scenario: highlights 落库恒为数组
- **WHEN** 日报生成时未产出任何 highlight（合法空降级或生成失败降级）
- **THEN** `board_daily_reports.highlights` SHALL 落 JSON 空数组 `[]`，SHALL NOT 落 JSON `null` 标量

#### Scenario: 历史 null highlights 读出归一为数组
- **WHEN** 读取一篇 `highlights` 列为 JSON `null` 的历史日报（2026-11-03 前的存量坏数据）
- **THEN** API 返回的 `highlights` SHALL 为 `[]`，SHALL NOT 为 `null`

#### Scenario: 事件落库时 scope 由事件标签数推导
- **WHEN** 某事件的 `tag_ids` 含 3 个 `category='event'` 标签
- **THEN** 该事件落库时 `scope` SHALL 为 `composite`，`story_id` SHALL 保持 NULL

#### Scenario: 事件未归组不影响日报
- **WHEN** 某事件的 `story_id` 为 NULL（未回填或判定失败）
- **THEN** 该事件 SHALL 照常在日报正文展示，SHALL NOT 被过滤或隐藏

## ADDED Requirements

### Requirement: 跨期重复降级

日报生成时 SHALL 利用事件线身份识别跨期重复：对每个候选事件，若其 `story_id` 非空且该事件线在最近 7 天已进入过日报正文，系统 SHALL 判定本次为「重述」或「进展」：

- **重述**：本事件的 `related_article_ids` 与事件线已有文章集合的重叠率 ≥ 0.6，且未出现新的独立来源（feed）→ SHALL 不进正文，降级为快讯或丢弃。
- **进展**：重叠率 < 0.6、或出现新 feed、或当事方自身有新动作（`is_development=true`）→ SHALL 进正文。
- `story_id` 为空或为综合稿的事件 SHALL 维持现有行为，SHALL NOT 因缺少身份而被误判为重复。

降级 SHALL 只作用于展示：事件 SHALL 照常落库、signals SHALL 照常写入、热度 SHALL 不受影响。降级结果 SHALL 写入 `display_tier`（`body`=正文、`flash`=快讯、`hidden`=未展示预留）并在读取路径可区分（本期日报详情只返回 `body`，`flash`/`hidden` 丢弃不渲染——字段保留供未来快讯 UI），SHALL NOT 删除历史记录。`story_manual=true`（人工指派）的事件 SHALL 不参与自动去重，保持 `body`。降级判定 SHALL 幂等：重复执行结果一致；条件变化（如新 feed 补入）时按当前集合重算，已降级事件可恢复 `body`。

#### Scenario: 重述降级为快讯
- **WHEN** 某事件线昨日已进正文，今日新事件的文章集合与之重叠 0.8 且无新 feed
- **THEN** 今日该事件 SHALL 不进正文（`display_tier='flash'`，本期读路径丢弃不渲染）；其 signals 与热度 SHALL 正常累计

#### Scenario: 新来源判为进展
- **WHEN** 某事件线昨日已进正文，今日新事件文章重叠 0.7 但引入了 1 个新 feed
- **THEN** 今日该事件 SHALL 进正文并标记为进展

#### Scenario: 当事方新动作判为进展
- **WHEN** 某事件线昨日已进正文，今日对应事件的 `is_development=true`
- **THEN** 该事件 SHALL 进正文，SHALL NOT 被降级

#### Scenario: 未归组事件不参与去重
- **WHEN** 某事件的 `story_id` 为 NULL
- **THEN** 该事件 SHALL 按现有逻辑展示，SHALL NOT 被判定为重述

#### Scenario: 降级不改变热量
- **WHEN** 日报把某事件从正文降级为快讯
- **THEN** 该事件线的 `story_signals` 与热度 SHALL 与降级前完全一致

#### Scenario: 读取路径只回正文线程
- **WHEN** 某日报含 `body`/`flash`/`hidden` 三种 display_tier 的事件
- **THEN** 日报详情读路径 SHALL 只返回 `body` 事件；`flash`/`hidden` 行 SHALL 仍在库中可查（供未来快讯 UI），直接数据库读取 SHALL 仍可见
