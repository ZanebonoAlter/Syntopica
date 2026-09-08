# Tasks: section 展示标题内容化

## 1. LLM 产出扩展（daily_report_llm.go）

- [x] 1.1 threads JSON schema 顶层增加可选 `section_title` 字段（description 含事实锚与「不得复述候选话题名」约束），`buildThreadsPrompt` 补对应指令；promptVersion 常量升至 "5.0"。验证：`go test ./internal/topicgraph/service -run TestBuildThreadsPrompt`（或新增用例断言 prompt 含 section_title 指令）。
- [x] 1.2 `parseThreadsResponse` 解析 `section_title` 并随返回值带出（实现时按现有风格选择返回结构，见 design Open Question）。验证：新增表驱动用例——含 section_title / 缺失 / 空串三种响应，断言解析结果与降级为空的事实。

## 2. 标题解析与兜底链（daily_report_orchestrator.go）

- [x] 2.1 section 构建处实现兜底链：threads `section_title` → 首条 thread `title` → 话题 label → `cluster.GroupName`；删除 `clusterLabel = topic.Label` 的无条件覆盖。验证：新增单测覆盖链上每一级（LLM 标题命中 / threads 空 / 无 thread 且命中话题 / 纯 L3），断言 `ClusterLabel` 取值符合 spec Scenario。
- [x] 2.2 确认 `buildSectionEmbedText` 传入的 `clusterLabel` 为新标题（无需改函数，仅验证调用点传值正确）。验证：现有 embedding 相关测试全绿。

## 3. 用例先行与回归（按 standard/shared/test-design.md 故事锚点）

- [x] 3.1 spec Scenario 映射单测：「命中既有话题的 section 标题反映当天内容」「标题生成失败时降级兜底」「L3 新话题标题行为不变」「话题归属字段不受标题影响」「标题遵守事实锚约束」。验证：`go test ./internal/topicgraph/service -short` 全绿且新用例名与 Scenario 一一对应。
- [x] 3.2 回归验证 lane 归属不受影响：现有 lane/归属相关测试（`daily_report_lane*_test.go`、orchestrator 测试）全绿。验证：`bash scripts/change-scope.sh` 判定影响包后按范围跑 `go test`。

## 4. 门禁与文档

- [x] 4.1 质量门禁：`golangci-lint run ./...`、`go vet ./...`、影响包 `go test`、`go build ./...` 全绿。验证：命令实跑输出为证。
- [x] 4.2 更新 `docs/reference/flow/daily-report.md`：STEP3/约束节补「section 展示标题内容化」表述 + 变更溯源表登记（promptVersion 5.0）。验证：`doc-impact.sh verify` 通过、文档与 spec 表述一致。
- [x] 4.3 按 §11 归档门禁完成收尾（含「部署后影响」汇报：下一期日报起标题语义变化、历史不回刷、无用户手动操作）。

## 5. 测试

- 后端（影响包，经 `bash scripts/change-scope.sh` 判定）：`go test ./internal/topicgraph/service -short`
- 本 change 专项：`go test ./internal/topicgraph/service -run 'TestResolveClusterLabel|TestThreadsSystemPrompt|TestParseThreadsResponse|TestSynthesizeFallbackThreads|TestBuildSectionEmbedText' -v`
- 静态门禁：`golangci-lint run ./...` + `go vet ./...` + `go build ./...`

## 6. 文档

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: database=result_kind 迁移属 dataenrichment 并行 change 脏文件（postgres_migrations.go / result_kind_migration_test.go），非本 change 改动 -->

- [x] 6.1 `docs/reference/flow/daily-report.md`：链路 STEP3 + 业务约束新增第 17 条「section 展示标题内容化」（promptVersion 5.0、四级兜底链、200 runes 截断、归属字段正交、历史不回刷）；变更溯源表登记本 change。

## 7. 验证

- [x] 7.1 `go test ./internal/topicgraph/service -short` → `ok syntopica-backend/internal/topicgraph/service`（2026-08-29 实跑）
- [x] 7.2 `go test ./internal/topicgraph/service -run 'TestResolveClusterLabel|TestThreadsSystemPrompt|TestParseThreadsResponse|TestSynthesizeFallbackThreads|TestBuildSectionEmbedText' -v` → 全部 PASS（fallback 链四级/长标题截断/prompt 指令/解析三态/兜底 threads，2026-08-29 实跑）
- [x] 7.3 `golangci-lint run ./...` → 0 issues；`go vet ./...` → 零输出；`go build ./...` → 成功（2026-08-29 实跑）
- [x] 7.4 `bash scripts/doc-impact.sh verify openspec/changes/daily-report-section-title-decouple` → 通过（database 并行 change 脏文件豁免声明见文档节）
- [x] 7.5 `bash scripts/scenario-trace.sh openspec/changes/daily-report-section-title-decouple` → 退出码 0（7 个 Scenario 映射齐全：自动测试 5 / 人工留痕 2）
- [x] 7.6 线上验收（2026-08-29 手动补生 8-28 日报）：board 2128 report 645 section 挂话题 935（label「日本首相高市早苗宣布不于7月释放石油储备」）但 `cluster_label`＝「日本三党合并协商破局」——当日内容派生，非话题名复读；`lane_tier=l2_llm`/`topic_match_confidence=anchor_hit` 归属照常

| Scenario | 测试文件 |
| 命中既有话题的 section 标题反映当天内容 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 标题生成失败时降级兜底 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| L3 新话题标题行为不变 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 话题归属字段不受标题影响 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 标题遵守事实锚约束 | backend-go/internal/topicgraph/service/daily_report_llm_test.go |
| 历史数据不回刷 | 人工：部署后抽查历史日报标题未变（无回刷代码即语义保证；7.6 佐证 report 628 及更早标题未变） |
| 时间线跨天串联不依赖标题一致 | 人工：时间线按 persistent_topic_id 归并同话题多日 section（7.6 佐证 935 跨天归属仍在） |

> 测试锦点：TestResolveClusterLabel_FallbackChain（orchestrator，前四行 Scenario）、TestThreadsSystemPrompt_RequestsSectionTitle（llm，事实锚约束）；线上验收佐证见 7.6。
