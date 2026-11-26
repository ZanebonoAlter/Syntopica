## 1. 后端：聚簇候选入口放宽

- [x] 1.1 `ListReportEventThreads` 删除 `COALESCE(scope,'') <> 'composite'` 过滤并同步函数注释（composite 线索的 event 标签同权参与聚簇，防混编由聚簇相似度门槛承担）。验证：repository 层测试——造「单事件多标签 composite 线索 + 1 篇小簇 single 线索」夹具，断言聚簇候选含大簇且按去重文章数排首位。
- [x] 1.2 聚簇防污染与回退边界测试：混编标签超阈值自成小簇不并入单事件簇；全线索 composite 且含 event 标签时不再走标签回退；线索均无 event 标签时仍走标签回退。验证：`cd backend-go && go test ./internal/topicgraph/...` 全绿。

## 2. 效果核对（真库量化，五问句之四）

- [x] 2.1 重建 2026-11-12「全球科技巨头动态」（board 15220）日报：断言候选白名单第一簇 = 黄仁勋簇（簇内去重文章数 20+），记录 LLM 实际挑选的头条；按「触发原因 / 方法 / 量化结果 / 结论」四要素记入本 change 目录 `effect-check.md`。验证：重建后 `curl --noproxy '*' -s http://localhost:5100/api/daily-reports/1473 | jq '.report.highlights'` 首条为大热点簇或记录 LLM 波动重试。

## 3. 测试设计

- [x] 3.1 test-cases.md：主链路表（聚簇入口→聚簇→排序→白名单→LLM→快照）+ 变体走查五组（输入空/前置全 composite/时间窗口即报告期/幂等重建/不适用 UI 组划除留痕）+ 白盒附加节（scope 过滤删除点的输入集合变化分支表）。⓪契约盘点：REMOVED「事件线热度先筛重点候选」→ `bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产，逐行填「继承与调整」表处置。

## 4. 文档

<!-- doc-impact: daily-report -->

- [x] 4.1 `docs/reference/flow/daily-report.md`「今日重点的选题与热度」节：候选入口口径更新（不再按 scope 排除，注明 2026-11-15 change 与误伤案例一句话）；顺手将 200-224 行过时段（`rankReportStoryCandidates`/`ReportStoryCandidates`）替换为现行聚簇口径。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/widen-highlight-cluster-candidates` 通过。

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/... && go test ./internal/topicgraph/...` — 全部通过、无新增告警
- [x] 5.2 `bash scripts/harness/change-scope.sh`（裸跑，基于 git diff 自动分桶）—— 本 change 代码影响包 = `go test ./internal/topicgraph/...`，与 5.1 一致，无遗漏
- [x] 5.3 `openspec validate widen-highlight-cluster-candidates --strict` — Change is valid

**Scenario 映射（scenario-trace 对账）**

| Scenario | 测试文件 |
| --- | --- |
| 单事件多标签大簇进候选 | backend-go/internal/topicgraph/service/daily_report_event_cluster_composite_test.go |
| 混编标签自成小簇不污染 | backend-go/internal/topicgraph/service/daily_report_event_cluster_composite_test.go |
| 全 composite 日报不再走标签回退 | backend-go/internal/topicgraph/service/daily_report_event_cluster_composite_test.go |
| 候选按簇内文章数截断 | backend-go/internal/topicgraph/repository/report_story_highlights_test.go |
| 模型只从白名单挑且不输出数字 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 零候选走标签回退 | backend-go/internal/topicgraph/service/daily_report_event_cluster_composite_test.go |
| 旧重点快照读取兼容 | 人工：读取本 change 前落库的日报详情（2026-11-12 report 1473 等），确认 highlights 快照字段原样返回、不被回填；后端只读验证路径见 effect-check.md |
| 并行生成成功 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| Thread 输出不含 status | 人工：现有 Call C prompt/schema 未随本 change 改动，既有回归网覆盖（本轮未涉及）；如需留痕可复跑 `go test ./internal/topicgraph/service -run TestDailyReport` |
| 昨日日报不存在 | 人工：首期生成路径未随本 change 改动，既有回归网覆盖；本轮未涉及 |

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
