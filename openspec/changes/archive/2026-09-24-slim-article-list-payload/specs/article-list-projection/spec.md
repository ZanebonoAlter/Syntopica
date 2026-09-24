## Purpose

文章列表接口的传输契约：列表只承载「扫描-选择」所需字段，正文类大字段仅由详情接口提供，使列表请求的搬运量与链路带宽无关地保持在低位。

## ADDED Requirements

### Requirement: 列表响应不携带正文类大字段

`GET /api/articles` SHALL 只返回列表消费所需字段（`id`/`feed_id`/`category_id`/`title`/`link`/`image_url`/`pub_date`/`author`/`read`/`favorite`/`summary_status`/`created_at`/`tag_count` 与 `excerpt`）；MUST NOT 返回 `content`、`firecrawl_content` 或完整 HTML `description`。

#### Scenario: 列表响应体积上限

- **WHEN** 客户端请求 `GET /api/articles?per_page=20`
- **THEN** 响应体（未压缩）SHALL < 100 KB，且响应项中不含 `content`、`firecrawl_content`、`description` 字段

#### Scenario: 各筛选分支字段集一致

- **WHEN** 客户端以任意筛选/排序组合请求列表（无筛选／`feed_id`／`category_id`／`watched_tags`+relevance／`concept_id`／`auxiliary_label_id`／`archived=true`）
- **THEN** 返回项的字段集合 SHALL 与无筛选时一致，MUST NOT 因排序分支而带上正文列

#### Scenario: 大字段按需到详情取

- **WHEN** 前端需要正文或完整导语（阅读页选中文章）
- **THEN** 数据 SHALL 来自 `GET /api/articles/:id`，且该接口 SHALL 继续返回 `content`/`description`/`firecrawl_content`/`ai_content_summary`

### Requirement: excerpt 承载列表层导语

列表响应每项 SHALL 提供 `excerpt`：由 `description`（为空时回退 `content`）去除 HTML 标签、折叠空白后的纯文本，长度 ≤ 200 字符；源为空、源无实质内容（无字母/数字）或**导语与正文重复**时 SHALL 返回空字符串（字段存在但不省略）。

「与正文重复」MUST 按阅读页既有去重 guard 的判据判定（正文归一化后与导语完全相同，或导语归一化后 ≥ 40 字符且被正文包含），且判据 MUST 是 guard 规则的**子集**——即 MUST NOT 抑制 guard 本会渲染的导语（否则 92% 文章会在点选时出现「首帧导语闪现→详情返回后消失」，实测 `description == content` 占非归档文章 92%）。

当导语来自**正文兜底**（`description` 无实质文本、导语取自 `content`）且该文章无 Firecrawl 正文（阅读页展示的正文就是 `content`）时，导语 SHALL 为空字符串——它与展示正文必然重复，guard 的相等/包含规则会隐藏它（实测此类文章 198 条，其中带 Firecrawl 正文的 0 条）。

#### Scenario: 长 HTML 源被截断

- **WHEN** 某文章 `description` 为 50 KB 的 HTML 片段
- **THEN** `excerpt` SHALL ≤ 200 字符、不含 HTML 标签

#### Scenario: 空源返回空串

- **WHEN** 某文章 `description` 与 `content` 均为空
- **THEN** `excerpt` SHALL 为 `""`，字段 SHALL 仍存在于响应项中

#### Scenario: 与正文重复的导语不下发

- **WHEN** 某文章的导语源与正文重复（`description` 与 `content` 为同一份文本，或导语归一化后 ≥ 40 字符且被归一化正文包含）
- **THEN** `excerpt` SHALL 为 `""`（阅读页去重 guard 本就会隐藏该导语，下发只会造成首帧导语闪现）
- **AND** 导语与正文不重复（含短导语 < 40 字符）时 `excerpt` SHALL 照常返回

#### Scenario: 正文兜底导语不下发

- **WHEN** 某文章 `description` 无实质文本（导语取自 `content`）且该文章无 Firecrawl 正文
- **THEN** `excerpt` SHALL 为 `""`（阅读页展示的正文就是 `content`，导语必然与之重复）
- **AND** 该文章有 Firecrawl 正文时 `excerpt` SHALL 照常返回（展示正文非 `content`，导语可能不重复）

### Requirement: per_page 超限语义可观测

`GET /api/articles` 收到 `per_page` 超过上限（100）时 SHALL 按上限返回结果，并 SHALL 记录一条 WARN 日志（含请求的 `per_page` 值与请求路径）；MUST NOT 静默截断而不留痕。

#### Scenario: 客户端请求 10000 条

- **WHEN** 客户端请求 `GET /api/articles?per_page=10000`
- **THEN** 响应 SHALL 返回 100 条，且服务端日志 SHALL 出现一条包含 `per_page=10000` 的超限 WARN

#### Scenario: 合法值不产生告警

- **WHEN** 客户端请求 `per_page=20`
- **THEN** 服务端 SHALL NOT 产生超限 WARN 日志
