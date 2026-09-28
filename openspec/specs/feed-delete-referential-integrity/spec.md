# feed-delete-referential-integrity Specification

## Purpose
订阅源与分类的删除必须真正可完成：指向被删实体的 `NO ACTION` 引用行（`reading_behaviors` / `user_preferences`）由删除路径在同一事务内显式清理，不依赖数据库外键级联、不指望已废弃的 `user_preferences` 表消失。本能力关闭「点了删除、报了错、记录还在库里」这一类静默失败，并让删除语义与前端确认文案一致。

## Requirements

### Requirement: 订阅源删除必须清理全部 NO ACTION 引用行

删除订阅源的路径 SHALL 在同一事务内、删除 `feeds` 行之前显式清理指向该源的引用行：`reading_behaviors`（按 `feed_id`）与 `user_preferences`（按 `feed_id`）。MUST NOT 依赖外键级联，也 MUST NOT 以 `user_preferences` 表已被 DROP 为前提。

`user_preferences` 的清理 SHALL 以表存在性探测为前置：表不存在（窄部署 / 已执行 DROP 的环境）时跳过该步、不报错，其余步骤照常完成。任一步失败 SHALL 让整个删除事务回滚，错误向上返回。

#### Scenario: 有 user_preferences 引用的订阅源可删除

- **WHEN** 删除一个在 `user_preferences` 有引用行的订阅源
- **THEN** 删除成功返回（不再因 `fk_user_preferences_feed` 报错）
- **AND** 该源的 `feeds` 行、其 `articles` 行、按 `feed_id` 关联的 `reading_behaviors` 与 `user_preferences` 行全部消失

#### Scenario: 有 reading_behaviors 引用的订阅源可删除

- **WHEN** 删除一个在 `reading_behaviors` 有引用行的订阅源
- **THEN** 删除成功，且这些 `reading_behaviors` 行随之消失（不论库中是否存在 `fk_reading_behaviors_feed`）

#### Scenario: 引用表缺失时跳过而不报错

- **WHEN** 目标库中不存在 `user_preferences` 表
- **THEN** 该表的清理被跳过，订阅源与其文章仍正常删除，整体不报错

#### Scenario: 无引用行的订阅源照常删除

- **WHEN** 删除一个在两张引用表中都没有行的订阅源
- **THEN** 删除正常完成，不产生针对这两张表的额外报错

#### Scenario: 清理失败整体回滚

- **WHEN** 引用行清理或文章删除的任一步失败
- **THEN** 订阅源行、文章行与引用行全部保持原样（事务回滚），且错误向上返回

### Requirement: 分类删除必须清理其下订阅源与分类级引用行

删除分类的路径 SHALL 在同一事务内显式清理两类引用行：其下全部订阅源的 `reading_behaviors`（按该分类下 `feed_id` 集合），以及 `user_preferences` 中 `feed_id ∈ 该分类下订阅源` 或 `category_id = 该分类` 的行。清理 SHALL 在删除该分类下的 `articles` 行之前完成，以避免 `reading_behaviors.article_id` 的 `NO ACTION` 外键阻断文章删除。表存在性探测、失败回滚语义与订阅源删除一致。

#### Scenario: 分类下有阅读行为与偏好引用时可删除

- **WHEN** 删除一个分类，其下订阅源在 `reading_behaviors` 与 `user_preferences` 均有引用行
- **THEN** 删除成功返回（不再因 `fk_reading_behaviors_feed` / `fk_reading_behaviors_article` / `fk_user_preferences_feed` / `fk_user_preferences_category` 失败）
- **AND** 该分类行、其下全部 `feeds` 行与 `articles` 行、以及这些引用的 `reading_behaviors` / `user_preferences` 行全部消失

#### Scenario: 分类级偏好行（无 feed 归属）一并清理

- **WHEN** `user_preferences` 存在 `category_id` 指向被删分类的行
- **THEN** 该行在删除分类时被清理，删除不因 `fk_user_preferences_category` 失败

#### Scenario: 分类外数据不受影响

- **WHEN** 删除分类 A，而分类 B 下的订阅源也有 `reading_behaviors` / `user_preferences` 行
- **THEN** 分类 B 及其订阅源的引用行保持原样

#### Scenario: daily-report 引用同步维护

- **WHEN** 删除分类导致其下文章行被删除，且某 `daily_report_threads` 引用了这些文章 id
- **THEN** 这些 id 从 `related_article_ids` 中剔除（既有 `article-reference-integrity` 语义不变），且该维护与被删行的删除在同一事务内完成

### Requirement: 删除范围必须与前端确认文案一致且不越权清理

删源 / 删分类的破坏性范围 SHALL 为「该实体 + 其下订阅源（分类场景）+ 其下文章 + 指向它们的引用行」，与前端确认弹窗（`FeedLayoutShell.vue` 删分类、设置页删源）声明的范围一致。删除路径 MUST NOT 越权清理与该实体无关的数据。

#### Scenario: 不越权清理孤儿标签

- **WHEN** 删除订阅源或分类导致若干 `topic_tags` 失去全部边
- **THEN** 这些 `topic_tags` 行仍存在，由 `aux_label_cleanup` 维护任务后续回收，删除路径不做越权清理

#### Scenario: 其他订阅源的文章与引用不受影响

- **WHEN** 删除订阅源 X，而订阅源 Y 的文章与引用行存在
- **THEN** Y 的 `articles` / `reading_behaviors` / `user_preferences` 行保持原样
