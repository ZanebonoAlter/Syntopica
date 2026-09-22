# Tasks: 泳道趋势概览（lane-trend-overview）

## 1. 开工核定

- [x] 1.1 跑 `bash scripts/harness/doc-impact.sh suggest openspec/changes/lane-trend-overview` 与 `context`；核定 `lane_snapshot.go` 结算/prompt/upsert、`daily_report_models.go` TopicLaneSnapshot、`lane_snapshot_repository.go` 聚合、`api/laneDynamics.ts`、`boardEnrichment.ts` fetchContexts、`DailyReportTopicSection.vue` 挂载点的精确现状，记录到 explore-findings，不另造结构。

## 2. 后端：结算长短两版

- [x] 2.1 改造 `laneSnapshotSystemPrompt` 为两版 JSON 输出契约（summary ≤100 字 + detail ≤500 字成段叙述、同一事实集），`laneSnapshotChatFn` maxTokens 512→768；实现解析函数（code fence 剥壳 → JSON → 字段校验）与降级路径（非 JSON/缺 detail → 短版=截断全文、detail 空串、warn 日志；缺 summary 维持既有空输出守卫按失败跳过），单测覆盖 test-cases SN-1~SN-9（目标 `service/lane_snapshot_test.go`，fake chat fn 注入），预期 `go test ./internal/topicgraph/service` 全绿。
- [x] 2.2 `settleLaneSnapshot` upsert 写入 RollingDetail（rune clamp 各自上限）；存量兼容断言 SN-10 并入 2.1 测试文件，预期旧空 detail 行读侧不报错、下周期覆盖补齐。

## 3. 后端：存储与聚合响应

- [x] 3.1 `daily_report_models.go` TopicLaneSnapshot 增 `RollingDetail string`（text，空=缺失）；确认 AutoMigrate 启动加列无需手工迁移（无 FK/索引），预期启动日志列变更无错、旧行 detail 为空。
- [x] 3.2 `lane_snapshot_repository.go`：`LaneDynamicsSnapshot` 增 `Detail *string`（空串归一 nil）、`GetLaneSnapshotsByTopicIDs` 与 `GetBoardLaneDynamics` 装配携带；单测覆盖 AG-1~AG-4（目标 `repository/lane_snapshot_repository_test.go`），预期 `go test ./internal/topicgraph/repository` 全绿。

## 4. 前端：API 层与趋势区组件

- [x] 4.1 `api/laneDynamics.ts`：`LaneSnapshot` 增 `detail?: string | null` 注释化契约（nil=长版缺失）；类型检查通过。
- [x] 4.2 新建 `features/tags/components/daily-report/LaneTrendOverview.vue`：三档分段切换（默认 14d，每泳道独立态）、长版全文/短版回退/快照缺失降级、月/年最新归档（period 取 max）与空归档占位、逐日事件默认收起 + 单日 5 条折叠 + folded_count 如实标注、加载/错误内联重试、pre-line 纯文本渲染；vitest 覆盖 FD-1~FD-15（目标 `LaneTrendOverview.test.ts`），预期 `pnpm test:unit LaneTrendOverview --maxWorkers=2` 全绿。
- [x] 4.3 `DailyReportTopicSection.vue` 泳道展开体顶部挂载趋势区（active zone + topicId 非空，置于节点图/当日明细之前），props 传递 lane 数据；FD-13/FD-16 断言并入 `DailyReportTopicSection.test.ts`，预期既有用例不红。

## 5. 前端：宿主取数

- [x] 5.1 `BoardDailyReportTimeline.vue`（或抽 `useLaneTrendData` 组合式）：首个泳道展开触发一次板块级 lane-dynamics、页面级缓存、切报告日不重拉、按 topic_id 过滤传泳道；contexts 月/年按需拉取 + `Map<topicId, granularity, rows>` 缓存；HD-1~HD-4 断言落对应测试文件，预期 `pnpm test:unit <受影响文件> --maxWorkers=2` 全绿。

## 6. 测试

- [x] 6.T1 `bash scripts/harness/change-scope.sh` 机械确定影响包，按输出跑目标 Go 测试（预期 `internal/topicgraph/service`、`internal/topicgraph/repository`）；预期全绿，不顺手全量。
- [x] 6.T2 对照 test-cases.md 复核 SN/AG/FD/HD 分组全部有对应断言落点（映射表回填本节），缺口补齐再进验证。

  SN/AG/FD/HD → 测试文件映射（全部有落点，无缺口）：

| 分组 | 落点 |
| --- | --- |
| SN-1~SN-10（含存量兼容） | `backend-go/internal/topicgraph/service/lane_snapshot_test.go`（TestSettleLaneSnapshot_TwoVersionJSONUpsert / ClampBothVersionsIndependent / DetailExactBoundary / NonJSONDegrades / MissingDetailNotFailure / MissingSummaryFailsKeepsOldSnapshot / CodeFenceStripped / LLMErrorKeepsOldSnapshotContinuesSiblings / LegacyRowDetailHeals + TestLaneSnapshotSystemPrompt_TwoVersionContractAndMaxTokens） |
| AG-1~AG-4 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go`（TestGetBoardLaneDynamics_SnapshotDetailLevels，testcontainer PG） |
| FD-1~FD-15 | `front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts`（17 用例含 D6 加载/错误附加） |
| FD-13 / FD-16 | `front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts`（挂载/不渲染/顺序/props 传递，既有 20 例不红） |
| HD-1~HD-4 | `front/app/features/tags/composables/useLaneTrendData.test.ts` + `front/app/features/tags/components/BoardDailyReportTimeline.test.ts`（单测 + 接线集成） |

  ⓪ 继承与调整表见 test-cases.md（board-lane-dynamics MODIFIED 反查，5 个受影响旧 Scenario 全部旧用例保留绿，无旧断言改写）。

## 7. 文档

<!-- doc-impact: flow, api, database -->

- [x] 7.1 `docs/reference/flow/daily-report.md`：结算节同步两版产物、降级路径、存量自愈口径；`docs/reference/flow/topic-graph.md` 变更溯源表补本 change 行（归档时补链接）。
- [x] 7.2 `docs/reference/api/`：lane-dynamics 响应 snapshot.detail 字段说明（两级缺失语义）；月/年复用既有 contexts 端点不新增条目，仅交叉引用。
- [x] 7.3 `docs/reference/database/`：topic_lane_snapshots.rolling_detail 列说明（可空派生缓存、无迁移动作）。

## 8. 验证

- [x] 8.1 `openspec validate lane-trend-overview`，预期通过。
- [x] 8.2 `bash scripts/harness/doc-impact.sh verify openspec/changes/lane-trend-overview` 与 `bash scripts/harness/check-standards.sh --change lane-trend-overview`，预期文档对账无遗漏。（本 change 相关项全过；10 个失败为 2026-09-17/18 历史欠账，与本 change 无关）
- [x] 8.3 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，预期通过；测试仅用 6.T1 确认的影响范围。（lint 0 issues / vet 0 / build 0；影响包 -count=1 全量复跑全绿）
- [x] 8.4 `cd front && pnpm lint && pnpm exec nuxi typecheck` 及 `pnpm test:unit <受影响文件> --maxWorkers=2`（实现时回填真实路径），预期通过，不与 build/浏览器并行。（lint 0 errors；typecheck exit=0；受影响 4 文件 54 用例 + 既有 lane-dynamics 套件 25 用例全绿，NODE_ENV=test）
- [x] 8.5 `bash scripts/dev/deploy-frontend.sh`，预期静态部署与健康检查成功；部署前向用户说明存量快照首日无长版的降级表现。（健康 200；rolling_detail 列 AutoMigrate 自动加上，存量 43 行 detail 全空）
- [x] 8.6 `agent-browser` 打开 `/tags` 日报展开泳道，按 ui-design Acceptance 人工核对三档切换、长版全文、降级占位、逐日折叠与 1440×900 无横向溢出，截图留证；结束 `close`。（7 张截图 acceptance/01-07：三档切换✓、14 天档存量降级短版+提示✓、月/年归档全文✓、逐日默认收起+按日倒序+每日≤5条✓、overflow:none✓；实际归档时 14 天档长版全文待下次日报结算后可复验）
- [x] 8.7 `bash scripts/harness/test-patrol.sh --report` 与 `bash scripts/harness/archive-readiness.sh lane-trend-overview`，预期无本 change 未解决测试/归档阻塞；实际归档另获用户指令。（①③④ 全绿；② 剩 10 红为 2026-09-17/18 harness 类 archive 未溯源的历史欠账，与本 change 无关，归档前需用户决策处置）

| Scenario | 测试文件 |
| --- | --- |
| 展开泳道见趋势区 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 非话题分组不渲染 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 默认 14 天档 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 切换月档 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 概要不截断 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 长版就绪 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 长版缺失回退 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 快照缺失 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 月档展示最新归档 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 年档空归档 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 默认收起 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 展开见逐日事件 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 截断如实标注 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 月档请求失败 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 加载态可见 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 日报完成后结算 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 两版同窗同素材单次生成 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 机械截断保护 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 结算失败不阻塞 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 存量快照兼容 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 态势句与时间线同窗 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 单请求聚合 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 携带长版叙述 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 无态势快照降级 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 长版缺失明示 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
