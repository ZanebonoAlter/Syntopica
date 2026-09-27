## MODIFIED Requirements

### Requirement: 归档清除衍生数据、内容字段全保留

归档时系统 SHALL 删除该文章的 `reading_behaviors` 记录并将 `search_vector` 置 NULL，MUST NOT 清除或截断任何文本字段，MUST NOT 删除该文章的 `article_topic_tags` 边（标签边由「标签边时间窗回收」requirement 按保留窗统一回收，归档不再承担删边职责）。

#### Scenario: 衍生数据清除

- **WHEN** 文章被归档
- **THEN** 其 reading_behaviors 行删除、search_vector 为 NULL；article_topic_tags 边保留（含迟到打标任务新挂的边），由时间窗 GC 统一回收

#### Scenario: 归档文章不被全文搜索命中

- **GIVEN** 文章已归档
- **WHEN** 用户在 reader 搜索关键词
- **THEN** 该文章不出现在搜索结果中

## ADDED Requirements

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
