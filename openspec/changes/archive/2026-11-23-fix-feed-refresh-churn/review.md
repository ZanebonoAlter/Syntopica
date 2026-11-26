# Review — fix-feed-refresh-churn

- 审查线程：change-review（minimax-cn/MiniMax-M3.1-Flash:max），2026-11-21
- 范围：change 制品（proposal/design D1-D4/specs×2/tasks）+ 工作区实现 diff 逐文件隔离
- 总评：四条主线（D1-D4）按设计落地且自洽；1 High（新增索引打破既有重试接口，当前线上零暴露、归档前必修）+ 3 Medium + 5 Low
- 核对通过项：比较归一（同一实现同一上限、二次清洗不可达）、归档冻结与红线 6/article-retention 边界、feed 元数据不受影响、迁移去重语义/谓词口径/幂等/PG-only 守卫/版本排序、seenInDoc 与 linkSet/DB 唯一索引同一规范化口径（剥 fragment）、核心测试断言真实性、其它批量状态流转（leased→pending 单行）均不触发唯一冲突

### R-1 2026-11-21【High】部分唯一索引打破 RetryTagQueueFailed 批量重试
- **状态**：已修复（重试改为 `id IN (SELECT MAX(id) FROM tag_jobs WHERE status='failed' AND article_id NOT IN (活动作业文章)) GROUP BY article_id`——已有活动作业的文章跳过、同文章多条 failed 只重试最新一条；`TestRetryTagQueueFailedSkipsArticlesWithActiveJob` + spec 场景「重试失败作业不得与已有活动作业冲突」）

- 证据：`tag_job_unique_migration.go:47`（索引）× `tag_queue_handler.go:195-215`（批量 `failed→pending`）——文章同时有 failed 行与活动行时整条 UPDATE 撞索引回滚 → 接口 500、0 行重试；该共存是常态（失败后再入队即产生）；端点零测试。
- 线上暴露面（2026-11-21 只读核查）：failed=0、冲突文章=0 → 潜伏非在炸。

### R-2 2026-11-21【Medium】seenInDoc「首现优先」依赖 newest-first 未校验约定
- **状态**：已修复（`RefreshFeed` 先经 `newestEntryByLink` 按 link 选 pubDate 最新变体、缺失/相等回退首现；spec 场景改「只按 pubDate 最新变体处理」、design D1/proposal 同步；`TestRefreshFeedDedupesRepeatedLinkWithinDocument` 四子例覆盖新/旧/缺失 pubDate）

- 证据：`feed_service.go:89-98`；design D1 自陈「惯例」。oldest-first 源会把库内行钉在最旧变体（稳态错误，非一次性抖动）。

### R-3 2026-11-21【Medium】insertTagJob 兜底分支返回错误，与 spec「MUST NOT 报错」字面冲突
- **状态**：已修复（两个队列两试皆空改 `logging.Warnf` + 返回 nil；兜底分支抽 `rideTagJobConflict`/`rideDetectionJobConflict` 闭包缝直测 `TestRide…DegradesWhenConflictRowTurnsTerminal`，spec 场景「冲突行转终态时不得报错中断调用方」）

- 证据：`tag_job_queue.go:89` / `detection_job_queue.go:124`；错误会上抛内容补全/手动重打标路径（500）。丢失窗口极窄（PG 推测插入锁已堵未提交窗口），但字面违约。

### R-4 2026-11-21【Medium】design D3「解档后恢复跟进」无产品路径支撑（冻结实为单向不可逆）
- **状态**：已修复（design D3 / proposal Impact / 红线 15 统一为「归档行永久冻结（当前无解档入口，需要解档另开 change）」，`refreshExistingArticle` 代码注释同步）

- 证据：全仓唯一写 `archived=true` 在 `feed_service.go:352`（CleanupOldArticles），无任何置 false 路径。代码行为正确、不违红线 6。

### R-5 2026-11-21【Low】迁移去重保留 MIN(id) 丢失较新行的 priority/force_retag/reason
- **状态**：已修复（两个迁移删重复前先 `UPDATE … FROM` 合并 priority=MAX / force_retag=OR / reason=最新非空到保留行，注释注明生产已执行、改动只对全新环境生效；PG 迁移用例断言合并值与幂等重跑）

- 证据：`tag_job_unique_migration.go:38-46`。生产已执行（版本已记录），改代码只对未来环境有效。

### R-6 2026-11-21【Low】「零开销」措辞与实测不符（仍有一次 unescape 分配 + 正则扫描）
- **状态**：已修复（红线 15 与 design D2 改「零 HTML 解析开销（仅一次实体解码 + 标记正则）」）


### R-7 2026-11-21【Low】「并发」用例被 SetMaxOpenConns(1) 串行化，易误读
- **状态**：已修复（保留用例名，注释点明单连接串行执行、验证的是重复入队幂等，真并发窗口由 PG 迁移用例 + 冲突分支用例覆盖）


### R-8 2026-11-21【Low】spec「feed 元数据刷新不受影响」无用例守护；归档用例绕过 RefreshFeed
- **状态**：已修复（新增 `TestRefreshFeedFreezesArchivedRowButUpdatesFeedMetadata`：完整 RefreshFeed 后断言 feed.title/last_refresh_at/refresh_status 更新、归档行内容与标签快照不变、firecrawl/tag 作业数 0）


### R-9 2026-11-21【Low】跨 change 记账落差：本 change 2.3 未勾 + detection 侧 spec 未写 DB 级唯一索引
- **状态**：已修复（2.3 已勾并注明落在 add-ai-generated-article-detection 3.5；detection spec 已补场景「数据库级唯一索引兜底并发入队」）

