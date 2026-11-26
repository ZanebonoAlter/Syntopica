<!-- constraint-domains: content-enrichment -->

# Tasks — fix-feed-refresh-churn

## 1. 刷新链三修复（feed_service.go）

- [x] 1.1 `RefreshFeed` 先按 link 分组选代表（`newestEntryByLink`：pubDate 最新胜出、缺失/相等回退首现，审查 R-2），再按文档顺序遍历并用 `seenInDoc` 挡其余副本（design D1）。验证：单测覆盖「同 link 后现 pubDate 更新 → 后现胜出 / 后现 pubDate 更旧 → 首现胜出 / pubDate 缺失 → 首现胜出」。—— `TestRefreshFeedDedupesRepeatedLinkWithinDocument`（四子例）PASS
- [x] 1.2 `refreshExistingArticle` 开头对 `existing.Archived` 短路返回 nil（design D3）。验证：单测「归档行 + 内容变化 → 不改写、标签保留、不入队」。—— `TestRefreshExistingArticleFreezesArchivedRow` PASS
- [x] 1.3 description 比较投影归一：存量侧 `HasMarkup` 时先 `PlainSummary(raw, 300)` 再比（design D2；`content`/`title` 比较不变）。验证：单测「库存 raw HTML + 源未变 → 判定未变化、零写库」与「库存 raw HTML + 源真变 → 正常更新」。—— `TestRefreshExistingArticleNormalizesStoredHTMLDescription` 两段断言均 PASS

## 2. 队列入队数据库级幂等

- [x] 2.1 新迁移文件：清 tag_jobs 存量同文章重复未完成行（保留最小 id）→ `CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending','leased')`；登记进 `postgresMigrations()`；MUST NOT 在真库手工执行（db-migration-safety：登记代码，启动时自动跑）。验证：迁移单测（含幂等重跑）。—— `internal/platform/database/tag_job_unique_migration.go`（20261120_0002）+ `tag_job_unique_migration_test.go`；重复 SQL 语义已在 sqlite 上验证（去重保留最小 id / 部分唯一索引拒重复 / 终态行不占位 / 重跑幂等），PG 版迁移测试待 Docker 恢复后跑（见下方环境阻塞）
- [x] 2.2 `tag_job_queue.go` Enqueue 插入分支加 `ON CONFLICT DO NOTHING`，撞索引后走与既有「已存在」分支同语义的原地更新。验证：单测模拟并发双入队 → 至多一条 pending（可用 sqlite 事务并发或直接测冲突分支）。—— `TestInsertTagJobRidesConflictPathWhenActiveJobExists` / `TestEnqueueTagJobRepeatedlyKeepsOneActiveJob` PASS
- [x] 2.3 （回活跃 change 补）detection_jobs 同型加固：部分唯一索引迁移 + `DetectionJobQueue.Enqueue` ON CONFLICT——在 `add-ai-generated-article-detection/tasks.md` 补任务并执行，spec 场景已有「重复触发判定不产生重复作业」。—— 已落地：该 change task 3.5 已勾（迁移 `detection_job_unique_migration.go` 20261120_0003 + Enqueue ON CONFLICT + 测试）

## 3. 测试

<!-- doc-impact: flow, database, api -->

- [x] 3.1 影响包门禁：`bash scripts/harness/change-scope.sh` 机械判定，`go test <影响包> -count=1` 全 PASS + `go vet ./...` + `golangci-lint run <影响包>` + `go build ./...`（树莓派禁全量 `go test ./...`）。—— 实现线程 2026-11-21：build/vet 退出 0、lint 0 issues、四包含 testcontainers PG 全绿（service 69s / tagmanagement 112s / reader-repo 112s / database 167s，Docker 修复后复跑）
- [x] 3.2 线上核验（部署后）：feed 31/24 下一轮刷新不再出现百级 `tag_jobs` 入队波（分组分钟查询）；`SELECT count(*) FROM articles WHERE description ~ '<[a-zA-Z]'` 稳定在回填完成值不回升；文章 167936 连续两轮刷新标题不再交替。—— 部署 2026-11-21 02:06（迁移 0002/0003 已执行、双索引已建）。2026-11-23 只读复核（`docker exec syntopica-postgres psql`，全部 SELECT）：全库近 4h `tag_jobs` 186 条，其中智源社区(feed 31) 7 条（修复前单小时 731 条）；文章 167936 近 48h 零作业、标题稳定在 ERNIE-5.5-Flash-Base；`pending/leased` 0 条、同文章重复活动作业 0 条；`description ~ '<[a-zA-Z][^>]*>'` 仅 1 行且是正文里的字面 `<me >`（非真标签，回填值稳定不回升）；`uq_tag_jobs_article_active` 与 `uq_detection_jobs_article_active` 均存在

## 4. 验证

- [x] 4.1 派 `change-review` 子线程聚焦审查（制品与实现 diff 对照，重点：seenInDoc 标记前置是否引入丢更新风险、二次插入兜底的丢失窗口、归档冻结与红线 6 的边界）；意见按 High/Medium/Low 处置后在 change 目录建 `review.md` 建账。—— 审查完成（2026-11-21，R-1 至 R-9 建账，R-9 已处置，其余待修）；`bash scripts/harness/review-trace.sh openspec/changes/fix-feed-refresh-churn` 退出 0
- [x] 4.2 审查意见 R-1 至 R-8 修复（逐条证据见 `review.md` 状态行）；同步 spec 场景（重试冲突 / 冲突行转终态 / pubDate 最新变体）、design D1-D3、proposal Impact、红线 15。—— 修复轮 2026-11-21：R-1 重试排除活动作业、R-2 重复 link 取 pubDate 最新、R-3 入队兜底降级 nil、R-4 解档文案改永久冻结、R-5 迁移合并语义、R-6 措辞、R-7 测试注释、R-8 RefreshFeed 级用例；门禁证据见下方「验证证据（2026-11-21 修复轮）」

## 5. 文档

- [x] 5.1 docs/reference/flow/content-enrichment.md — 业务约束节补红线 15（刷新判变双侧同投影 / 同文档重复 link 只处理 pubDate 最新变体 / 归档行永久冻结 / 队列部分唯一索引幂等）、代码入口行与变更溯源链接；`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-feed-refresh-churn` 通过
- [x] 5.2 docs/reference/database/tables/job-queues.md — §7.2 `tag_jobs` 补部分唯一索引 `uq_tag_jobs_article_active`（迁移 `20261120_0002`）的约束与合并语义说明
- [x] 5.3 docs/reference/api/tag-ops.md — `POST /api/tag-queue/retry` 补「已有活动作业的文章跳过、同文章多条 failed 只重试最新一条」语义

## 背景事实（已执行的数据面处置，非任务）

- 回填：`go run ./cmd/clean-article-descriptions --apply`（2026-11-20 晚执行中，幂等可断点重跑；dry-run 基线 affected=62332 / 释放 346.8 MiB）。
- 队列清理：pending 2048 → 897（删归档文章作业 1,150 + 竞态双行 1）。

## 验证证据（2026-11-20 实现轮）

- `cd backend-go && go build ./...` → PASS（退出码 0）
- `cd backend-go && go vet ./...` → PASS（退出码 0）
- `cd backend-go && golangci-lint run ./internal/reader/service/... ./internal/tagmanagement/... ./internal/reader/repository/... ./internal/platform/database/...` → `0 issues.`（退出码 0）
- `cd backend-go && go test ./internal/reader/service -short -count=1` → PASS（ok 18.9s；新增 3 个 feed_service 用例均绿）
- `cd backend-go && go test ./internal/tagmanagement/repository -short -count=1` → PASS（ok 0.05s；新增 2 个 tag_jobs 幂等用例均绿）
- `cd backend-go && go test ./internal/reader/repository -short -count=1` → PASS（ok 0.09s；detection_jobs 2 个幂等用例均绿）
- `cd backend-go && go test ./internal/platform/database -short -count=1` → PASS（ok 0.07s）
- ⛔ **环境阻塞（非本 change 代码问题）**：testcontainers 启动 PG 容器在本机当前不可用——`docker create` 单次耗时 50s~200s+ 且逐次变慢（containerd/daemon 退化，`docker rm -f` 同样要 60~120s），`testutil.OpenTestDB` 的 2 分钟容器启动预算被耗尽，三个 PG 依赖包的非 `-short` 用例均报 `container create: ... context deadline exceeded`（复测 4 次：22:10 / 22:29 / 22:47 / 22:57，含 `TESTCONTAINERS_RYUK_DISABLED=true` 一次，全部卡在同一处）。需运维执行 `sudo systemctl restart docker`（会一并重启 `syntopica-postgres` 并中断在跑的后端，需用户授权）后重跑：`cd backend-go && go test ./internal/reader/service ./internal/tagmanagement/repository ./internal/reader/repository ./internal/platform/database -count=1`。已就位的 PG 用例：`tag_job_unique_migration_test.go`、`detection_job_unique_migration_test.go`，以及本包既有 `tag_job_queue_test.go` 的 Claim/MarkFailed 系列。
- 迁移 SQL 语义替代验证（同一份 SQL 在 sqlite 上跑通后已删除临时探针）：去重保留最小 id、部分唯一索引拒第二条未完成行、终态行不占位、二次执行幂等——四个断言在 `tag_jobs` / `detection_jobs` 两张表上均成立。
- 不依赖 Docker 的迁移接线用例（新增）：`TestJobQueueUniqueMigrationsRegistered`（两个版本各注册一次且排在 20261120_0001 之后）、`TestJobQueueUniqueMigrationsSkipNonPostgres`（sqlite 上 Up 为 no-op）→ PASS。

## 验证证据（2026-11-21 审查修复轮）

- `cd backend-go && go build ./...` → 退出 0
- `cd backend-go && go vet ./...` → 退出 0
- `cd backend-go && golangci-lint run ./internal/reader/service/... ./internal/reader/repository/... ./internal/tagmanagement/handler/... ./internal/tagmanagement/repository/... ./internal/platform/database/...` → `0 issues.`（退出 0）
- `cd backend-go && go test ./internal/reader/service ./internal/tagmanagement/handler ./internal/tagmanagement/repository ./internal/reader/repository ./internal/platform/database -short -count=1` → 五包全 PASS（退出 0；含新增 `TestRetryTagQueueFailedSkipsArticlesWithActiveJob`、`TestRide…DegradesWhenConflictRowTurnsTerminal`、`TestRefreshFeedFreezesArchivedRowButUpdatesFeedMetadata`、`TestRefreshFeedDedupesRepeatedLinkWithinDocument` 四子例）
- `cd backend-go && go test ./internal/platform/database -run 'TestTagJobUniqueMigration…|TestDetectionJobUniqueMigration…' -count=1`（testcontainers PG，非 short）→ PASS（22.8s；断言合并语义 MAX(priority)/OR(force_retag)/最新非空 reason 与幂等重跑）
- `openspec validate fix-feed-refresh-churn --strict` → valid（退出 0）；`openspec validate add-ai-generated-article-detection --strict` → valid（退出 0，R-9 遗留场景已补）
- `bash scripts/harness/review-trace.sh openspec/changes/fix-feed-refresh-churn` → 通过（9 条建账，状态行格式合法）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-feed-refresh-churn` → 通过（退出 0）

## 验证证据（2026-11-23 归档复核）

- `cd backend-go && go build ./...` → 退出 0；`go vet ./internal/reader/service ./internal/tagmanagement/... ./internal/platform/database` → 退出 0
- `cd backend-go && golangci-lint run ./internal/reader/service/... ./internal/tagmanagement/... ./internal/platform/database/...` → `0 issues.`
- `cd backend-go && go test ./internal/reader/service ./internal/tagmanagement/repository ./internal/tagmanagement/handler ./internal/platform/database -short -count=1` → 四包全 PASS
- `cd backend-go && go test ./internal/platform/database -run 'TestTagJobUniqueMigrationCollapsesDuplicatesAndAddsIndex' -count=1` → PASS（62.4s，testcontainers PG）；新增自包含接线用例 `TestTagJobUniqueMigrationRegistered` / `TestTagJobUniqueMigrationSkipsNonPostgres`（`tag_job_unique_migration_registration_test.go`，不依赖 Docker）随 `-short` 跑通
- `openspec validate fix-feed-refresh-churn --strict` → valid；`bash scripts/harness/check-tasks-tail.sh …` / `review-trace.sh …` / `doc-impact.sh verify …` → 全通过；`bash scripts/harness/check-standards.sh --change fix-feed-refresh-churn` → 通过 320 / 失败 0
- 线上只读复核（`docker exec syntopica-postgres psql`，全部 SELECT）数据见 3.2
