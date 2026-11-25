## MODIFIED Requirements

### Requirement: 列表响应不携带正文类大字段

`GET /api/articles` SHALL 只返回列表消费所需字段（`id`/`feed_id`/`category_id`/`title`/`link`/`image_url`/`pub_date`/`author`/`read`/`favorite`/`summary_status`/`created_at`/`tag_count`/`ai_detection_status` 与 `excerpt`）；MUST NOT 返回 `content`、`firecrawl_content` 或完整 HTML `description`。

列表行 SHALL 承载的可扫读状态集（标题、作者、时间、已读、收藏、处理状态图标、文章级疑似 AI 标记） SHALL 保持稳定；新增文章级标记 SHALL 为非正文类小字段，SHALL NOT 携带正文类内容。

#### Scenario: 列表响应体积上限

- **WHEN** 客户端请求 `GET /api/articles?per_page=20`
- **THEN** 响应体（未压缩）SHALL < 100 KB，且响应项中不含 `content`、`firecrawl_content`、`description` 字段

#### Scenario: 各筛选分支字段集一致

- **WHEN** 客户端以任意筛选/排序组合请求列表（无筛选／`feed_id`／`category_id`／`watched_tags`+relevance／`concept_id`／`auxiliary_label_id`／`archived=true`）
- **THEN** 返回项的字段集合 SHALL 与无筛选时一致，MUST NOT 因排序分支而带上正文列

#### Scenario: 大字段按需到详情取

- **WHEN** 前端需要正文或完整导语（阅读页选中文章）
- **THEN** 数据 SHALL 来自 `GET /api/articles/:id`，且该接口 SHALL 继续返回 `content`/`description`/`firecrawl_content`/`ai_content_summary`

#### Scenario: 疑似 AI 标记随列表返回

- **WHEN** 客户端请求文章列表
- **THEN** 返回项 SHALL 携带该文章的疑似 AI 判定状态字段，且该字段 SHALL NOT 为正文内容

#### Scenario: 未判定文章不因标记字段改变列表体积口径

- **WHEN** 客户端请求文章列表且多数文章判定状态为未判定
- **THEN** 响应项 SHALL 仍携带该判定状态字段（值表达未判定），SHALL NOT 省略字段，SHALL NOT 影响正文大字段禁令
