# board-article-api — retire-downgraded-mount delta

## MODIFIED Requirements

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
