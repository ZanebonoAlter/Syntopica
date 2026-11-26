## ADDED Requirements

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
