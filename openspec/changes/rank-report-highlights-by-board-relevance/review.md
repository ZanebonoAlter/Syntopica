# Review — rank-report-highlights-by-board-relevance

> 审查档：`change-review`（zai-coding-cn/glm-5.4），2026-11-27，主线程派发与建账。
> 审查范围：代码实现（`service/daily_report_highlight_relevance.go`、`service/daily_report_story_highlights.go`、`repository/report_event_clusters.go`、`repository/report_section_signals.go` + 4 个测试文件）、design D1–D13 落点复核、delta spec 22 个 Scenario 对照、flow 红线 5 / 46 / 41 / 47 / 49。
> 审查子线程实测：定向测试全绿（service 纯逻辑 + repository PG + `FinalizeAndTagFallback` / `CompositeOnly` 两条 testcontainer）；真库只读核对 11-27 / 11-26 板块信号与测试夹具逐条一致。

### R-1 2026-11-27【Medium】制品措辞与实现不符：`RankEventClusterCandidates` 已无生产调用方
- **状态**：已修复（比较器函数体已抽成 `OrderEventClusterCandidates`，`RankEventClusterCandidates` 只做截断委托并保留给既有测试；proposal「Impact」、tasks 2.4、design D5 落点改为 `service/daily_report_story_highlights.go:highlightCandidatePool`，design 伪代码改用 `OrderEventClusterCandidates`——原伪代码写的 `RankEventClusterCandidates(clusters, len(clusters))` 会被 limit 上限 8 截断，实现时已避开；D8 落点改指真实新文件 `report_section_signals.go:ListReportSectionSignals`）

### R-2 2026-11-27【Medium】`TestRankPickedHighlightsByRelevance`「同档保持候选序」断言偏弱
- **状态**：已修复（`service/daily_report_highlight_relevance_test.go`：伪同档子用例换成真同档——x(档 0, 1 篇) / y(档 5) / z(档 0, 9 篇)，输入 `[x, y, z]` → `[x, z, y]`，x 在前只因输入在前；新增「均属中档（在跟 / 缺失）时保持候选序」覆盖 spec「两条均为在跟或均缺失时 SHALL 保持候选序」）

### R-3 2026-11-27【Medium】读取板块信号失败的接线分支只测到 `highlightCandidatePool`，finalize 层无断言
- **状态**：已修复（`service/daily_report_highlights_cluster_test.go` 新增 testcontainer 集成测试 `TestHighlightsClusterSignalErrorKeepsCandidateOrder`：seam 返回 error → 落库顺序 = 既有候选序、条数不变、`title` 取首条、不报错；对照阶段用真实查询证明重排会生效，排除空断言；seam `defer` 恢复、无 `t.Parallel`）

### R-4 2026-11-27【Low】`highlightCandidatePool` 注释「tag-fallback never reaches here」不准，零聚簇时白读一次板块信号
- **状态**：已修复（`finalizeDailyReportHighlights` 加 `len(clusterSet.Clusters) > 0` 守卫：零聚簇不读板块信号、`candidates` / `sectionSignals` 保持 nil 走原标签回退；注释同步改准；`TestHighlightsClusterFinalizeAndTagFallback` 的零候选回退用例未动即通过）

### R-5 2026-11-27【Low】`TestOrderEventClusterCandidatesIsFullOrder` 只测篇数互异序，未覆盖标签数 / 代表 id 破同分
- **状态**：误报丢弃（完整序与截断版共用同一个比较器函数，标签数 / 代表 id 破同分由既有 `TestRankEventClusterCandidatesOrderAndLimit` 传递覆盖，新增测试只需证明「完整序 = 截断版前 8」）

### R-6 2026-11-27【Low】`ListReportSectionSignals` 与测试 seam 无 ctx（无 tracing）
- **状态**：已驳回（与被仿写的 `ListReportEventThreads` 同款：单条只读投影、无下游调用链，日报尾段本就不为同类只读查询单独穿 ctx，本 change 不扩大范围）

### R-7 2026-11-27【Low】Scenario「旧重点快照读取兼容」没有专测
- **状态**：已驳回（读路径零改动，旧报告原样返回由既有读取测试保证；「重建按新口径」走的是同一 finalize，已被 `TestHighlightsClusterFinalizeAndTagFallback` 系列覆盖，不为没有独立入口的行为造测试）

- 审毕无发现（审查范围：High 层——红线 5 / 46 / 41 / 47 / 49 无违反；section 数 / 泳道热度 / 模型返回顺序均未进任何比较；失败分支 Warn 含 report id 与 err、池退前 8、`signals=nil` 跳过后排；提示词与 `parseClusterHighlightsResponse` 零 diff；比较器三键逐字未变、`limit` 语义不变；`front/` 与 `FinalizeReportHighlights` 未碰；SQL 参数化只读、按 `report_id` 定序、NULL 安全、复用 `jsonUintIDs`；design 落点 D1–D13 全部可解析）

## 处置后复验（主线程实跑，2026-11-27）

- `cd backend-go && go build ./... && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/...` → OK / `0 issues` / OK
- `cd backend-go && go test -short -count=1 ./internal/topicgraph/...` → handler / repository / service 全 ok
- `cd backend-go && go test -short -count=1 ./internal/topicgraph/service -run 'CandidatePool|RankPicked|RelevanceRank|Highlight' -v` → 64 PASS / 0 FAIL（含 R-2 的两个新子用例）
- `cd backend-go && go test -count=1 ./internal/topicgraph/service -run 'TestHighlightsClusterSignalErrorKeepsCandidateOrder|TestHighlightsClusterFinalizeAndTagFallback|TestHighlightsCompositeOnlyReportNotTagFallback'` → 3 PASS（testcontainer，R-3 新测试与零候选回退同时通过）
