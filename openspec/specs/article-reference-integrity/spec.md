# article-reference-integrity Specification

## Purpose
文章行被物理删除时（去重归并、删订阅源的 FK 级联、仓库删行原语），保证「按 ID 引用文章」的 jsonb 数组同步维护：能映射到保留条就改指保留条，不能映射就剔除；日报写入的引用必须全部指向存在的文章；存量悬空引用一次性修复；悬空引用可观测。

## Requirements

### Requirement: 文章行删除时必须同步维护 jsonb 引用数组

任何删除 `articles` 行的路径在删行前 SHALL 维护 `daily_report_threads.related_article_ids`：能确定保留条时把被删 id 改指保留条，不能确定时剔除被删 id。维护 SHALL 在同一事务内完成，失败 SHALL 让整个删除操作回滚。数组 SHALL 保持原有顺序、SHALL NOT 出现重复元素、SHALL NOT 写入 JSON `null` 标量（空引用写 `[]`）。

#### Scenario: 归并副本时引用改指保留条

- **WHEN** 去重归并把 loser 副本 id `X` 合并进保留条 `Y`，且某 thread 的 `related_article_ids` 含 `X`
- **THEN** 该数组中的 `X` 被替换为 `Y`，数组内不出现重复的 `Y`，其余元素顺序不变

#### Scenario: 保留条已在数组内时只去重

- **WHEN** 某 thread 的 `related_article_ids` 同时含 `X` 与 `Y`，且归并把 `X` 合并进 `Y`
- **THEN** 数组中只保留一个 `Y`（首次出现的位置），长度减一

#### Scenario: 删除订阅源时引用被剔除

- **WHEN** 删除一个订阅源，其文章行随之删除，且某 thread 引用了其中若干文章 id
- **THEN** 这些 id 从该 thread 的 `related_article_ids` 中被剔除，其余元素顺序不变
- **AND** 若数组因此变空，写入 `[]` 而非 `null`

#### Scenario: 删除分类的两级同样维护引用

- **WHEN** 删除一个分类，其下的 feed 与文章随之被删除（代码显式删除，不依赖遗留 FK 是否存在于该库），且某 thread 引用了这些文章中的若干 id
- **THEN** 这些 id 从该 thread 的 `related_article_ids` 中被剔除，其余元素顺序不变
- **AND** 该维护与被删分类及其 feed / 文章的行删除在同一事务内完成（任一步失败则整体回滚）

#### Scenario: 无引用命中时不产生写操作

- **WHEN** 被删除的文章 id 未被任何 thread 引用
- **THEN** 不产生任何 `daily_report_threads` 的 UPDATE

#### Scenario: 单引用的空数组规范化

- **WHEN** 某 thread 的 `related_article_ids` 只含被删除的那个 id
- **THEN** 更新后为 `[]`，且 `jsonb_typeof(related_article_ids) = 'array'`

### Requirement: 被删文章的从属行必须显式清理，不依赖 FK 级联

任何删除 `articles` 行的路径 SHALL 在同一事务内、删文章行之前显式删除其从属行（`article_topic_tags` / `tag_jobs` / `firecrawl_jobs`），MUST NOT 依赖数据库遗留外键的级联（`articles` 相关的 FK 只存在于历史库，`DisableForeignKeyConstraintWhenMigrating: true` 下的新建库没有它们）。删行 SHALL 分块进行（避免超出绑定参数上限），从属行的表存在性 SHALL 在分块循环外解析一次。失去全部边的 `topic_tags` 孤儿 SHALL 由既有 `aux_label_cleanup` 维护任务回收，删除路径 MUST NOT 自行清理 `topic_tags`。

#### Scenario: 删除订阅源后不留孤儿从属行

- **WHEN** 删除一个订阅源，其文章被删除，且这些文章在 `article_topic_tags` / `tag_jobs` / `firecrawl_jobs` 中有行
- **THEN** 这些从属行全部消失（无论库中是否存在遗留级联 FK）
- **AND** 其他订阅源的从属行不受影响

#### Scenario: 删除分类后不留孤儿从属行

- **WHEN** 删除一个分类，其下全部 feed 的文章被删除
- **THEN** 这些文章的从属行全部消失，分类外 feed 的从属行不受影响

#### Scenario: 孤儿标签不归删除路径清理

- **WHEN** 删除文章导致某些 `topic_tags` 失去全部边
- **THEN** 这些 tag 行仍存在（由 `aux_label_cleanup` 维护任务后续回收），删除路径不做越权清理

#### Scenario: 从属表缺失时跳过而不报错

- **WHEN** 目标库中不存在某张从属表（窄部署 / 旧库）
- **THEN** 该表的清理被跳过，文章行删除仍正常完成，整体不报错

### Requirement: 存量悬空引用必须一次性修复

系统 SHALL 提供一次性修复迁移：先把 `related_article_ids` 的非数组值（JSON `null` / SQL NULL）规范化为 `[]`，再剔除所有指向不存在文章行的 id。修复 SHALL 可重复执行（幂等），SHALL 分批进行，SHALL 记录处理行数与剔除引用数。修复 SHALL NOT 修改 `article_count` 等生成时快照计数。

#### Scenario: 悬空 id 被剔除

- **WHEN** 迁移执行且某 thread 的 `related_article_ids` 含不存在的文章 id
- **THEN** 该 id 被剔除，其余元素顺序不变，且迁移日志记录命中行数与剔除数

#### Scenario: JSON null 规范化为数组

- **WHEN** 迁移执行且某 thread 的 `related_article_ids` 为 JSON `null` 或 SQL NULL
- **THEN** 该列被写为 `[]`，`jsonb_array_elements_text(related_article_ids)` 不再报错

#### Scenario: 重复执行不改变已修复数据

- **WHEN** 迁移在已修复的数据上再次执行
- **THEN** 命中行数为 0，无任何 UPDATE

#### Scenario: 无悬空数据时零写入

- **WHEN** 全库 `related_article_ids` 均指向存在的文章
- **THEN** 迁移不产生任何 UPDATE，仅记录 0 计数

### Requirement: 日报写入的引用必须全部指向存在的文章

日报生成在写入 thread 之前 SHALL 校验候选文章 id 的存在性，只把仍存在的 id 写入 `related_article_ids`，以关闭「读取候选后、写库前文章行被删除」的 TOCTOU 窗口。校验失败 SHALL 记录告警并降级（不阻断日报生成）。

#### Scenario: 候选文章在写库前被删除

- **WHEN** 生成过程收集到文章 id `X` 后、写库前 `X` 的行被删除
- **THEN** 写入的 `related_article_ids` 不含 `X`，其余候选引用保留

#### Scenario: 全部候选都已消失

- **WHEN** 某 thread 的全部候选文章 id 在写库前均不存在
- **THEN** 写入 `[]` 而非 `null`，thread 本身仍正常落库

#### Scenario: 存在性校验查询失败时降级

- **WHEN** 存在性校验查询报错
- **THEN** 记录告警日志，仍按既有候选写入（不阻断日报生成）

### Requirement: 悬空引用必须可观测

系统 SHALL 提供只读的悬空引用计数能力，并在日报生成 job 收尾记录该计数；计数大于 0 时 SHALL 以 Warn 级别记录，SHALL NOT 自动删除数据。

#### Scenario: 日报 job 收尾记录悬空计数

- **WHEN** 日报生成 job 完成当日报告
- **THEN** 日志出现 `dangling article refs=` 计数，计数大于 0 时级别为 Warn

#### Scenario: 计数查询失败不影响 job

- **WHEN** 悬空计数查询报错
- **THEN** 记录告警日志，日报 job 仍视为成功
