# article-retention Specification

## Purpose
TBD - created by archiving change article-archive-instead-of-delete. Update Purpose after archive.

## Requirements

### Requirement: 超限文章归档而非删除
`CleanupOldArticles` SHALL 将超出 `MaxArticles` 的文章归档（`archived = true`）而非物理删除，文章行、ID 及全部文本字段（title/description/content/firecrawl_content/ai_content_summary/link/image_url/pub_date/author）MUST 保持可读。

#### Scenario: 超出上限的文章被归档
- **GIVEN** feed 的活跃文章数超过 MaxArticles=100
- **WHEN** CleanupOldArticles 执行
- **THEN** 最旧的超限文章 `archived = true`，文章行仍存在于 articles 表，原文字段不变

#### Scenario: favorite 文章免死
- **GIVEN** 某文章 favorite = true 且位于超限区
- **WHEN** CleanupOldArticles 执行
- **THEN** 该文章不被归档，非 favorite 的更新文章代替其进入归档候选

#### Scenario: 无上限 feed 不归档
- **WHEN** feed.MaxArticles = 0 或 9999
- **THEN** CleanupOldArticles 不归档任何文章

#### Scenario: 归档幂等
- **GIVEN** 文章 A 已处于 archived = true
- **WHEN** CleanupOldArticles 再次执行
- **THEN** A 不重复进入候选集，无额外写操作

### Requirement: 归档清除衍生数据、内容字段全保留

归档时系统 SHALL 删除该文章的 `reading_behaviors` 记录并将 `search_vector` 置 NULL，MUST NOT 清除或截断任何文本字段，MUST NOT 删除该文章的 `article_topic_tags` 边（标签边由「标签边时间窗回收」requirement 按保留窗统一回收，归档不再承担删边职责）。

#### Scenario: 衍生数据清除

- **WHEN** 文章被归档
- **THEN** 其 reading_behaviors 行删除、search_vector 为 NULL；article_topic_tags 边保留（含迟到打标任务新挂的边），由时间窗 GC 统一回收

#### Scenario: 归档文章不被全文搜索命中

- **GIVEN** 文章已归档
- **WHEN** 用户在 reader 搜索关键词
- **THEN** 该文章不出现在搜索结果中

### Requirement: 活跃窗口计数排除归档行
CleanupOldArticles 的数量统计与排序候选 SHALL 仅基于 `archived = false` 的文章，防止归档行侵蚀活跃窗口。

#### Scenario: 归档行不占用窗口计数
- **GIVEN** feed 有 100 篇 archived=false 与 500 篇 archived=true 文章，MaxArticles=100
- **WHEN** CleanupOldArticles 执行
- **THEN** 活跃计数为 100，不触发任何新归档

### Requirement: 归档文章对列表与统计默认不可见
reader 列表与统计查询 SHALL 默认过滤 `archived = false`；文章列表 API SHALL 支持 `archived=true` 查询参数显式返回归档集。

#### Scenario: 列表默认不含归档文章
- **GIVEN** feed 存在归档文章
- **WHEN** 用户请求文章列表（不带 archived 参数）
- **THEN** 归档文章不出现在结果中

#### Scenario: 统计口径为活跃数
- **GIVEN** 全库存在归档文章
- **WHEN** 请求全局统计（total/unread/favorite）或 feed 级统计（article_count/unread_count）
- **THEN** 计数仅统计 archived = false 的文章

#### Scenario: 显式查询归档集
- **WHEN** 请求文章列表带 `archived=true`
- **THEN** 返回归档文章

### Requirement: 按文章 ID 的读取豁免归档
按 ID 获取文章详情、RSS 刷新去重、日报引用反查 SHALL NOT 过滤归档文章。

#### Scenario: 日报线索可读归档文章原文
- **GIVEN** 日报 thread 的 related_article_ids 引用的文章已归档
- **WHEN** 按该文章 ID 请求详情
- **THEN** 返回完整文章（含原文字段）

#### Scenario: RSS 去重包含归档标题
- **GIVEN** 归档文章的标题与 RSS 条目相同
- **WHEN** feed 刷新
- **THEN** 该条目不重复入库

### Requirement: 标签边时间窗回收

系统 SHALL 按可配置的保留窗口（`ai_settings` 键 `tag_edge_retention_days`，默认 7 天，缺失/非法/非正值回退默认并记 warn）回收 `article_topic_tags`：周期性删除 `created_at` 早于「当天（本地时区）零点减窗口天数」的边（日历天口径，与日报补档扫描、重建守卫同键同口径，防边界日击穿），随后对受影响的 topic tag 复用既有 `CleanupOrphanedTags` 清理孤儿。回收范围 SHALL 限于**已归档文章**的边（删边与受影响 tag 收集的谓词同时要求 `article_id IN (SELECT id FROM articles WHERE archived = true)`）：未归档文章的边 MUST NOT 被回收，归档后才进入窗口倒计时——活跃文章仍在分析面上，其标签是活数据（阅读页标签角标/过滤直接消费边）。窗口从边创建时刻（打标落库时刻）起算。回收 SHALL 由既有维护类调度任务承载（随 aux label 清理任务执行），SHALL NOT 依赖 AI 健康（不受分析暂停门禁约束）。标签聚合消费方（日报候选、cotag 窗口、升级建议）继续按各自时间窗界定范围，不按归档位过滤。

#### Scenario: 超窗边被回收

- **GIVEN** 已归档文章的标签边 created_at 为 8 天前，保留窗口 7 天
- **WHEN** 回收任务执行
- **THEN** 该边被删除，若其 topic tag 因此无任何剩余边则被孤儿清理回收

#### Scenario: 未归档文章的超窗边保留

- **GIVEN** 未归档文章的标签边 created_at 为 8 天前，保留窗口 7 天
- **WHEN** 回收任务执行
- **THEN** 该边保留，且仅靠该边存活的 topic tag 不被孤儿清理回收（归档后才进入窗口倒计时）

#### Scenario: 窗口内边保留供补档消费

- **GIVEN** 停机 5 天后恢复，积压打标刚为 3 天前的文章挂上新边
- **WHEN** 恢复次日报补档重建那几天的日报
- **THEN** 这些边仍在窗口内，日报候选能匹配到对应文章

#### Scenario: 配置非法回退默认

- **WHEN** `tag_edge_retention_days` 缺失、非数字或 ≤0
- **THEN** 系统使用默认 7 天并记 warn，SHALL NOT 拒绝执行回收

#### Scenario: 回收不受分析暂停影响

- **WHEN** AI 不可达、分析处于暂停
- **THEN** 边回收任务照常执行（维护类，不经 PauseAware 门禁）
