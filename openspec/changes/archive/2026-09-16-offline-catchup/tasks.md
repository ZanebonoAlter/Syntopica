# Tasks: offline-catchup

## 1. 测试用例先行

- [x] 1.1 创建 `test-cases.md`：specs 三个 requirement（MODIFIED 1 + ADDED 2，共 12 个 Scenario）串主链路表；变体走查（窗口边界：恰好 7 天/7 天+1ms；队列 pending/leased/空 × 补档动作；配置 非法/缺失/0/负数；孤儿有无剩余边）；白盒附加节（边 GC 状态：边∈窗内/窗外 × tag 有/无剩余边；补档前置状态机）。验证：文件存在且 12 个 Scenario 全部落点。

## 2. 核心实现

- [x] 2.1 `internal/tagmanagement`：新增边 GC 服务函数（`EdgeGC(ctx, EdgeGCRequest{RetentionDays})`：收集受影响 tag → DELETE 超窗边 → `CleanupOrphanedTags`）+ `tag_edge_retention_days` 配置读取（照 `persistent_topic_*` cfg 模式，默认 7、非法/≤0 回退+warn）。验证：`go test ./internal/tagmanagement/...` 新增单测全绿（超窗删/窗内留/回退默认/孤儿回收）。
- [x] 2.2 `internal/admin/scheduler/job_aux_label_cleanup.go`：aux label GC 后串接边 GC，JobResult 汇总两项计数。验证：`go test ./internal/admin/scheduler` 通过，job 结果含边回收计数。
- [x] 2.3 `internal/reader/service/feed_service.go`：`CleanupOldArticles` 移除删边+孤儿清理段（affectedTagIDs/删 ArticleTopicTag/CleanupOrphanedTags 三步），其余（behaviors/search_vector/翻标志/窗口计数）不动。验证：`go test ./internal/reader/service` 既有归档测试改造后全绿（归档不再删边的断言更新）。
- [x] 2.4 `internal/admin/scheduler/job_daily_report.go`：DailyReportJob 尾部补档扫描——队列空前置（tag_jobs 无 pending/leased）→ 窗口内逐日 × 板块查 `(board, period_date)` 存在性 → 缺失则 `GenerateAndSaveReport`；JobResult 记补档计数与跳过原因。验证：`go test ./internal/admin/scheduler` 新增用例全绿（补缺/顺延/不重建已有）。
- [x] 2.5 超窗守卫：`daily_report_handler.go` generate 端点 + `TriggerNowWithDate` 对 `date < today - retentionDays` 拒绝（4xx / accepted=false），错误消息含窗口说明。验证：handler 测试覆盖超窗拒绝与窗内放行。

## 3. 测试

- [x] 3.1 白盒表驱动单测：边 GC 边界（恰好窗口边界内/外、批量删除、孤儿有/无剩余边）；补档前置状态机（队列 pending→跳过、leased→跳过、空→执行）；守卫边界（today-window / today-window±1 天）。验证：`go test ./internal/tagmanagement/... ./internal/admin/scheduler ./internal/admin/handler -run 'EdgeGC|Backfill|RetentionGuard|CleanupOldArticles' -v` 全 PASS。
- [x] 3.2 真实效果人工核对：本地停 AI 数日（或模拟）→ 恢复 drain → 次日 21:00 观察缺档自动补齐；手动请求超窗日期确认 4xx。验证：人工记录时间线回填 test-cases.md 效果核对节。【留痕：2026-09-16 用户确认已验/信任自动化覆盖（job 单测锁补档/顺延/不重建、handler 单测锁 4xx 守卫），停机数日观察不再单独留时间线，直接归档】

## 4. 文档

<!-- doc-impact: flow, api, configuration -->

- [x] 4.1 `docs/reference/flow/reading.md` 约束 #6：更新"归档同时清除衍生数据"表述——标签边不再随归档删除，改由时间窗回收（指向新键）。`docs/reference/flow/scheduler.md`：aux_label_cleanup job 描述补边回收职责；daily_report job 描述补自动补档与顺延语义。`docs/reference/flow/daily-report.md`：补缺档自动补档 + 超窗守卫。`docs/reference/api/daily-reports.md`：generate 端点补超窗 4xx 行为与错误语义。`docs/reference/configuration.md`：ai_settings 键表新增 `tag_edge_retention_days`（默认 7，缺失/非法/≤0 回退默认+warn，边 GC/补档扫描/重建守卫三处同口径）。验证：`grep -rn "归档同时清除衍生数据\\|article_topic_tags 边" docs/reference/flow/reading.md` 输出与新语义一致；configuration.md 含 tag_edge_retention_days 键行。

## 5. 验证

- [x] 5.1 `cd backend-go && go test ./internal/tagmanagement/... ./internal/reader/service ./internal/admin/scheduler ./internal/admin/handler` → 全部 PASS。
- [x] 5.2 `cd backend-go && golangci-lint run ./...` → 0 issues。
- [x] 5.3 `cd backend-go && go vet ./... && go build ./...` → 均退出码 0。
- [x] 5.4 `grep -n "CleanupOrphanedTags" backend-go/internal/reader/service/feed_service.go` → 零命中（归档不再删边的机械锚；孤儿清理移交 GC job）。

| Scenario | 测试文件 |
|---|---|
| 衍生数据清除 | backend-go/internal/reader/service/feed_service_cleanup_test.go |
| 归档文章不被全文搜索命中 | backend-go/internal/reader/service/feed_service_cleanup_test.go |
| 超窗边被回收 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 未归档文章的超窗边保留 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 窗口内边保留供补档消费 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 配置非法回退默认 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 回收不受分析暂停影响 | backend-go/internal/admin/scheduler/pause_test.go |
| 停机缺档次日自动补齐 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 队列未清空顺延 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 只补缺不重建已有 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 超窗日期拒绝重建 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 窗口内日期正常重建 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 调度器指定日期触发同口径 | backend-go/internal/admin/scheduler/job_daily_report_test.go |

## 6. review 修复

- [x] 6.1 修复 review 发现：H1 EdgeGC 滚动窗→日历天口径（代码+测试+spec/design/test-cases/文档同步）；H2 board_daily_reports 补 (board, period_date) 唯一索引（模型 tag，存量重复风险附去重 SQL）；M1 failed 计数入 Data；M3 存在性查询改 Count；M4 补档循环 ctx 短路。M5（reader 标签过滤受边回收影响）拉用户决策，不在本任务。验证：影响包全绿 + lint 零 issue + 文档口径 grep 零残留。
