# Tasks — add-searxng-health-indicator

## 1. 后端：模型与迁移

- [x] 1.1 `annotation_qas` 模型加 `WebSearchStatus *string` 字段（`margin_notes_repository.go`）+ ALTER 迁移，verify: `go test ./internal/platform/database -run Migration -count=1`（testcontainer PG）迁移后 NULL 列存在
- [x] 1.2 迁移守卫：旧行（NULL）读写路径不报错，verify: repository 单测插入 NULL 行后按现状读回

## 2. 后端：问答状态采集

- [x] 2.1 `searchWebForMarginNoteWithQueryGen` 返回 `(results, status)`，三态判定与降级树逐分支对齐（disabled/ok/empty），verify: `go test ./internal/topicgraph/service -run MarginNote -count=1` 覆盖三分支
- [x] 2.2 `AskMarginNote` 将 status 写入 `MarginNoteQAResult` 并落库 `annotation_qas`，QA API 响应带出 `web_search_status` 字段，verify: handler 测试断言响应字段与落库值

## 3. 后端：健康探测端点

- [x] 3.1 `GET /api/settings/searxng/health`（discovery handler + 路由注册）：四态判定（复用配置解析 + searxng.Search 探针），返回 `{status, latency_ms, detail}`，8s 超时，verify: handler 测试四态 + disabled 不返回错误码
- [x] 3.2 探测不走 airouter/不落 ai_call_logs（非 AI 调用），探测 WARN 日志与 QA 链路同前缀可 grep，verify: `grep -c 'margin-notes: health probe' ` 日志断言（测试内）

## 4. 前端：设置面板探测

- [x] 4.1 `front/app/api/searxng.ts` 加 `checkHealth()`，verify: API client 类型检查通过（nuxi typecheck）
- [x] 4.2 `SearxngConfigPanel.vue` 加「测试连接」按钮 + 四态标记 + loading 防重复 + 保存后自动探测，verify: `pnpm test:unit SearxngConfigPanel --maxWorkers=2`（四态渲染/点击/保存自动探测/重复提交防护）

## 5. 前端：批注卡降级标记

- [x] 5.1 `MarginNoteCard.vue` `web_search_status=empty` 渲染「联网搜索无结果」标记（title 提示），ok/disabled/NULL 不渲染，verify: `pnpm test:unit MarginNoteCard --maxWorkers=2`（四态条件渲染）

## 6. 测试

- [x] 6.1 test-cases.md 主链路故事全部绿（含 ⓪ 继承与调整表逐行处置完成），verify: `bash scripts/harness/change-scope.sh` 判定的影响包测试全绿（database 包 4 个域外红已登记 test_debt 台账：与 add-board-health-checkup 未提交迁移相关，隔离实验证明非本 change 引起）
- [x] 6.2 opencli 主交互链路（设置面板探测）落点或人工豁免留痕于 test-cases.md，verify: test-cases.md 该节非空（opencli 实录：点击探测 ok+耗时、死端口保存自动探测 unreachable、恢复后 ok；degraded/disabled 留人工）

## 7. 文档

<!-- doc-impact: flow, api, database -->
<!-- doc-impact-excuse: configuration=疑似遗漏来自并行 change 脏文件（platform/config、aisettings 等），本 change 未改配置面 -->

- [x] 7.1 `docs/reference/flow/daily-report.md` 页边注问答节补联网状态可见性 + 溯源行；`docs/reference/api/` 补 health 端点，verify: `doc-impact.sh verify` 通过
- [x] 7.2 `docs/reference/flow/` 涉及域红线句核查（联网状态标记不改变静默降级红线），verify: check-standards.sh 对应段通过

## 8. 验证

- [x] 8.1 后端全绿：`cd backend-go && golangci-lint run ./internal/topicgraph ./internal/discovery ./internal/platform/database && go vet ./... && go test ./internal/topicgraph ./internal/discovery ./internal/platform/database` — 期望 0 fail（database 包 4 个域外红已登记台账，其余全绿；另加跑 ./internal/platform/searxng 全绿）
- [x] 8.2 前端全绿：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit SearxngConfigPanel MarginNoteCard --maxWorkers=2` — 期望 0 error（lint 0 error/7 存量 warning；typecheck 0 error；19 tests passed）
- [ ] 8.3 端到端手验（部署后）：Clash 出口失效状态下提问一次 → 卡片出现「联网搜索无结果」标记；设置面板点「测试连接」→ 显示 degraded + 耗时；修复代理后再探测 → ok 且新提问无标记 — 期望三步全部命中（**人工待验**：当前出口已恢复，失效态不可自然复现；面板探测链路已由 opencli 实录覆盖 ok/unreachable 两态，degraded 与卡片 empty 标记待下次出口故障或人工模拟）
- [x] 8.4 部署：`bash scripts/dev/deploy-frontend.sh` 铺盘 + 后端重启迁移自动执行 — 期望健康检查通过、`annotation_qas.web_search_status` 列存在（已验：/health 200、列 nullable=YES、schema_migrations 含 20261007_0001、真机探测 ok·30 条结果·5132ms）

### Scenario → 测试映射

| Scenario | 测试文件 |
| --- | --- |
| 联网失效时回答带降级标记 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 未启用联网时不显示标记 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 联网正常时不显示标记 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 历史问答不显示标记 | backend-go/internal/topicgraph/repository/margin_notes_repository_test.go |
| 点击测试连接获得四态结果 | front/app/components/dialog/SearxngConfigPanel.test.ts |
| 引擎全挂时探测显示 degraded | backend-go/internal/discovery/handler/searxng_health_handler_test.go |
| 未配置时探测显示 disabled | backend-go/internal/discovery/handler/searxng_health_handler_test.go |
| 保存后自动探测 | front/app/components/dialog/SearxngConfigPanel.test.ts |

补充落点：卡片标记四态断言在 front/app/features/tags/components/daily-report/MarginNoteCard.test.ts；service 三态用例 TestAskMarginNote_WebSearchStatus_ThreeStates、repository 旧行 NULL 用例 TestAnnotationQA_WebSearchStatus_NullAndSetRoundTrip、handler 四态用例 TestGetSearxngHealth_FourStates、面板 HP-1～HP-5。

域外红记账：database 包 4 个红（TestBoardSignalMigrationIdempotent / TestBoardSignalMigrationShapeAndFKRejectIllegalRows / TestBoardSignalMigrationLegacyRowsKeepNullColumns / TestBoardLevelAnalysisScopeMigration）已在 test_debt 台账（context=add-board-health-checkup 未提交迁移，单跑绿验证非本 change 引起），已记账后继续。
