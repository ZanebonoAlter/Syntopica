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
系统 SHALL 支持 page/per_page 分页参数，默认按 pub_date 倒序排列。返回 SHALL 包含 pagination 信息（page, per_page, total, pages）。

#### Scenario: 分页查询
- **WHEN** 请求 `GET /api/semantic-boards/5/articles?page=2&per_page=10`，共有 25 篇文章
- **THEN** 系统 SHALL 返回第 11-20 篇文章，pagination.total=25，pagination.pages=3
