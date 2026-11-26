## MODIFIED Requirements

### Requirement: 日报查询 API
系统 SHALL 提供以下查询端点：
- `GET /api/semantic-boards/:id/daily-reports?days=7`：查询该 board 最近 N 天的日报列表
- `GET /api/daily-reports/:id`：查询单篇日报详情（含关联 sections，每个 section 通过 GORM Preload 包含 threads 列表）
- `GET /api/semantic-boards/:id/section-timeline?days=14`：查询板块 section 时间线（含关系）

`GET /api/semantic-boards/:id/daily-reports` SHALL 在未提供有效正数 `days` 时使用 7 天默认值，并对任意正数 `days` 查询实际请求范围，不得静默截断到 30 天。

`GetReportByID` SHALL 使用嵌套 Preload `"Sections.Threads"` 加载 section 及其关联线程。`DailyReportSection` 的 JSON 响应中 `threads` 字段 SHALL 包含 `[]DailyReportThread` 对象数组（每条含 id、report_id、section_id、title、summary、tag_ids、confidence），section 和 thread 均不含 status/prev_*_id 字段。

`GET /api/daily-reports/:id` 返回的每条 thread 的 `related_article_ids` SHALL 是**读路径投影**：系统 SHALL 先按「归一 link 相同即同一篇内容」折叠同内容副本，再返回折叠后的数组。折叠 SHALL 保持原数组顺序、SHALL 以**首个出现**为准只保留一条（沿用日报数组契约的「保序 + 首个出现为准」口径）。判重键 SHALL 为归一 link：归一 SHALL 复用 `textutil.NormalizeArticleLink` 的既有规则（去 fragment、去 query、scheme/host 小写），SHALL NOT 另立口径，SHALL NOT 引入标题相似度、时间窗口等其它判据。`link` 为空或归一后为空、以及该 id 在库里查不到时**内容身份不可判定**：该 id SHALL NOT 参与**基于 link 的**折叠、SHALL 原样保留（不删除、不改指其它 id）。同一 article id 在数组内重复出现时 SHALL 折叠为一条（首个出现为准）——同一 id 必属同一篇文章，该折叠与「归一 link 相同」等价，故在内容身份不可判定时同样适用。折叠 SHALL 只作用于响应：落库值 `daily_report_threads.related_article_ids` SHALL 保持为该 thread `tag_ids` 对应文章的并集，SHALL NOT 被任何身份/去重逻辑改写。折叠所需的文章 link SHALL 在单次读路径中批量获取；获取失败 SHALL 降级为原样返回（不折叠、不阻断详情接口）。


#### Scenario: 查询板块日报列表
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，每条含 id、title、summary、period_date、status、cluster_count、article_count

#### Scenario: 查询日报详情含线程
- **WHEN** 请求 `GET /api/daily-reports/42`
- **THEN** 系统 SHALL 返回日报 #42 完整内容，每个 section 包含 threads 列表（每条含 id、title、summary、tag_ids、confidence、related_article_ids），section 和 thread 均不含 status/prev_*_id 字段

#### Scenario: API 查询超过 30 天
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=42`
- **THEN** 系统 SHALL 返回最近 42 天范围内的全部匹配日报

#### Scenario: 无日报时返回空
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`，但该 board 无日报
- **THEN** 系统 SHALL 返回空数组

#### Scenario: 线索下同 link 副本折叠
- **WHEN** 某条 thread 的 `related_article_ids` 含两个 `link` 相同的文章 id（跨 feed 副本）
- **THEN** `GET /api/daily-reports/:id` 返回的该 thread `related_article_ids` SHALL 只含其中首个出现的 id，数组长度减一、顺序不变
- **AND** `daily_report_threads.related_article_ids` 的落库值 SHALL 保持未折叠

#### Scenario: 归一后 link 相同也折叠
- **WHEN** 同一条 thread 的两篇文章 link 仅在 fragment、query 或 host 大小写上不同（归一后相同）
- **THEN** 返回的 `related_article_ids` SHALL 折叠为一条（首个出现为准）

#### Scenario: link 为空不折叠
- **WHEN** 某条 thread 的 `related_article_ids` 含 `link` 为空的文章
- **THEN** 该文章 SHALL 原样保留在返回的 `related_article_ids` 中，SHALL NOT 被当作同内容副本剔除

#### Scenario: 同标题不同 link 不折叠
- **WHEN** 同一条 thread 的两篇文章标题相同但 `link` 不同（多源转载）
- **THEN** 返回的 `related_article_ids` SHALL 保留两个 id（本口径只认归一 link 相同）

#### Scenario: 同一 id 重复引用折叠为一条
- **WHEN** 同一条 thread 的 `related_article_ids` 含重复出现的同一个 article id
- **THEN** 返回的 `related_article_ids` SHALL 只保留该 id 的首个出现（与 link 是否可判定无关）
