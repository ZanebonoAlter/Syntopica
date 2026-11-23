# rss-article-dedup Delta — fix-feed-refresh-churn

## MODIFIED Requirements

### Requirement: link 命中已有条目时的更新语义

同 feed 同 link 的新条目不再插入，改为按内容差异决定更新动作；变化判定 SHALL 对摘要清洗口径漂移免疫，归档条目 SHALL 冻结不跟进。

#### Scenario: 内容未实质变化时跳过

- **WHEN** 命中已有条目且标题与摘要内容均未发生实质变化
- **THEN** 不更新文章内容，不触发任何处理链（不打标、不抓取、不生成摘要）

#### Scenario: 存量旧口径摘要不误判为变化

- **WHEN** 命中已有条目，其库存摘要仍是旧口径（含源站标记），而源条目内容未实质变化
- **THEN** 比较 SHALL 在把库存摘要经同一清洗投影（`PlainSummary`，≤300 字符）归一后进行，判定为未变化，不触发任何处理链

#### Scenario: 内容实质变化时更新并重走处理链

- **WHEN** 命中已有条目且标题或摘要内容发生实质变化
- **THEN** 更新该条目的标题/摘要/发布时间等内容字段
- **AND** 按 feed 自身开关重置并重走正文抓取与摘要处理链
- **AND** 处理链完成事件按既有路径触发重新打标（旧标签先清理再重打）

#### Scenario: 同一文档内重复 link 只按 pubDate 最新变体处理

- **WHEN** 同一次刷新的 RSS 文档内同一 link 出现多次（无论内容是否相同）
- **THEN** SHALL 只处理 `pubDate` 最新的那个变体（新建或刷新），`pubDate` 缺失或相等时回退首次出现
- **AND** 该 link 的其余变体 SHALL 跳过，SHALL NOT 对同一条目二次改写、二次清理标签或二次入队

#### Scenario: 归档条目冻结不跟进

- **WHEN** 命中已有条目为归档状态（`archived = true`）
- **THEN** SHALL 跳过改写与处理链重入队，保留归档时的内容与标签快照
- **AND** feed 自身的元数据刷新不受影响
