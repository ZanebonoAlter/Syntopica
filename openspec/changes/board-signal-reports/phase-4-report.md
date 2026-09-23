# 阶段4交付报告：集成与文档（phase-4-report）

> 2026-09-22，develop 主仓库直改。范围=tasks 6.T1~T3、7.1~7.4、8.1~8.3（阶段0-3 已交付的实现零业务代码改动；本批只做集成验证收口 + 参考文档同步）。8.4~8.6 与 6.T4/T5 人工项按 brief 不做，留阶段6。

## ① 文档改动清单

| 文件 | 改了什么节 |
| --- | --- |
| `docs/reference/flow/data-enrichment.md` | ①需求说明：补一段「板块信号解读报告」定位（工作台主视图、两阶段人工、无评审）；②链路设计：新增「板块信号解读报告（board-signal-reports）」整节（发现/研究两阶段管线、cutoff 过滤先于 agent、受限计算、报告契约与引用渲染、无评审链、持久化与派生状态、job 内存态与旧 API deprecated 边界）；③「11 个 LLM Operation 速查」改「14 个」并补 signal_detect/signal_research/signal_compose 三行；④SessionID 规则补 board_signal_discovery_{board_id}_{hex8} 与 board_signal_report_{candidate_id}_{hex8} 两条；⑤「任务互斥与轮询」补两种新 kind 加入同板互斥、前端工作台主视图更替与旧视图卸载保留的事实；⑥业务约束与不变量新增 **#28~#33** 六条红线（仅手动两阶段/原子保存与复合 FK/派生状态不持久化 running/40 轮预算+cutoff 先于 agent/受限计算无任意执行/无评审不可变报告），全部按首行加粗红线句格式；⑦代码入口新增「后端板块信号解读报告」「前端信号工作台」两条；⑧REST API 路由表补 5 条 signal 端点；⑨变更溯源新增 2026-09-22 行，归档位置=**实现完成待归档（归档后按 §12 补链接）**——占位不冒充 |
| `docs/reference/flow/research-data-sources.md` | ①链路设计图调用方补「板块信号研究 loop（唯一已授权方）+ cutoff 包装层消费」；②口径红线补 EIA `weeks` 1~12 官方归档版次 CSV（缓存键 `eia:table1:weeks=N`）与 JODI `years` 1~5 显式多年（与 month 互斥、历史年 gap、显式 month 404 不回退）；③工具面不变量改写：旧流程零变化 + **唯一例外** signal_research 显式授权四源白名单+calculate（explorationToolNames 逐字不变）；④代码入口 sources 行补 weeks/years 扩展；⑤变更溯源新增 2026-09-22 占位行（同上待归档） |
| `docs/reference/api/board-signals.md` | **新建**（与 dataenrichment.md 同粒度独立成文）：五端点路由表 + 周期参数约定 + POST signal-discoveries（400/409/202 逐状态表）+ GET signals（参数/派生状态/响应字段表）+ POST signals/:candidateId/research（**按判定顺序** 400→404→409→200 复用→202，regenerate 语义）+ **job 状态字段时机表**（discovery 与 research 分列：phase/outcome/error_stage/identity/result_id 出现时机；重启 404 恢复语义与 409 按 job_kind 接管）+ GET signal-reports[/detail]（列表/详情差异、404 不泄漏存在性、无 review 字段）+ sectors schema_version=2 payload 全形状 + 引用渲染约定 + 计算字段表 |
| `docs/reference/api/_index.md` | 增量注册 board-signals.md 一行（**保留其他 change 预存的 datasources.md 行**，只增不删） |
| `docs/reference/database/tables/data-enrichment.md` | ①§10.3 topic_enrichment_result：result_kind 枚举补 `signal_report`、新增 granularity/period/source_signal_id 三列行（NULL 语义/CHECK 形状）、sectors 多态说明补 signal_report payload；②约束体系块扩写：形状约束 signal_report 分支（NULL-aware 显式拒绝+周期正则）、复合 FK `fk_topic_enrichment_result_signal_candidate`（MATCH SIMPLE）、两个部分索引（board-period-id / source_signal_id）、20260922_0001 DROP+re-ADD CHECK 与旧行 NULL 不回填（DB-4）、signal_report 不入 legacy isBoardResultKind；③新增 **§10.11 board_signal_discovery** 与 **§10.12 board_signal_candidate** 两表完整字段表（含唯一约束/复合 FK/索引/去重键/不可变语义）；④域 ER 图补两条边与两个实体块 |
| `docs/reference/database/tables/_conventions.md` | 「迁移显式引入的 FK」权威清单增量补 2 行（fk_board_signal_candidate_discovery / fk_topic_enrichment_result_signal_candidate，均 RESTRICT、复合、`20260922_0001`） |
| `docs/reference/database/_index.md` | 数据增强域表清单增量补 board_signal_discovery / board_signal_candidate 两行（**保留预存 research-data-sources 行**） |
| `docs/reference/architecture/map.md` | 业务域索引「数据富化编排」行增量：后端入口补 signal_* 五文件与 signal_discovery/signal_research 路由，前端入口改为信号解读工作台主视图（旧三分派视图与绑定入口卸载保留、BoardRelationPanel 不再从面板挂载） |

> 未虚构：所有字段/状态机/预算数字以 phase-2a/2b 报告②节与代码（handler/signal_discovery.go、signal_research.go、repository/signal_models.go、postgres_migrations.go boardSignalMigration）为准；归档链接均为占位，不写假路径。

## ② 测试收口结果（命令 + 退出码）

**6.T1 影响包**：`bash scripts/harness/change-scope.sh` 输出的多域映射含其他 change 预存脏改；按 brief 辖区只跑 dataenrichment 域 + platform/database：

| 命令（cwd=backend-go） | 退出码 |
| --- | --- |
| `go test -short ./internal/dataenrichment/... -count=1` | 0（4 包 ok） |
| `go test ./internal/platform/database/ -run TestBoardSignal -count=1` | 0（隔离 PG，迁移幂等/形状拒绝用例） |

**6.T2 故事映射表**（S1-S5 全绿；实际命令已回填 tasks.md 6.T2）：

| 故事 | 落点 | 命令 | 用例 | 退出码 |
| --- | --- | --- | --- | --- |
| S1 | handler/signal_discovery_test.go | `go test ./internal/dataenrichment/handler/ -short -run "TestSignalDiscovery\|TestSignalList" -count=1` | 11 | 0 |
| S1 | repository/signal_candidate_test.go | `go test ./internal/dataenrichment/repository/ -run "TestCreateSignalDiscoveryBatch\|TestListSignalCandidatesByPeriod\|TestSignalEvidenceRefHelpers" -count=1` | 6（隔离 PG） | 0 |
| S2 | handler/signal_research_test.go | `go test ./internal/dataenrichment/handler/ -short -run "TestSignalResearch\|TestSignalReports" -count=1` | 9 | 0 |
| S3 | service/signal_research_test.go | `go test ./internal/dataenrichment/service/ -short -run "TestSignalResearch_" -count=1` | 16（纯逻辑无 DB） | 0 |
| S4 | service/signal_calculation_test.go | `go test ./internal/dataenrichment/service/ -short -run "TestSignalCalc_" -count=1` | 9 | 0 |
| S5 | service/signal_compose_test.go | `go test ./internal/dataenrichment/service/ -short -run "TestSignalCompose_" -count=1` | 9 | 0 |
| PG 约束 | repository/signal_report_test.go | `go test ./internal/dataenrichment/repository/ -run "TestCreateSignalReportResult\|TestSignalReportDBConstraints\|TestSignalReportVersions\|TestListSignalReportResults" -count=1` | 4（隔离 PG，非法形状被 CHECK/FK 拒绝） | 0 |

test-cases.md 已按现有格式补执行证据（头部证据块 + 白盒分支覆盖说明 + OB-3/6.T4/T5 留待办标注）；故事定义零改动。

**6.T3 前端**（cwd=front，串行）：

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `pnpm lint` | 0 | 0 errors（7 warnings 全部位于其他 change 预存文件） |
| `pnpm exec nuxi typecheck` | 0 | 0 error |
| `pnpm test:unit app/features/tags/components/SignalCandidateList.test.ts app/features/tags/components/SignalReportView.test.ts --maxWorkers=2` | 0 | 2 文件 43 用例全绿（参数未带 `--`） |

## ③ validate / verify / standards 结果

| 检查 | 结果 |
| --- | --- |
| 8.1 `openspec validate board-signal-reports --strict` | ✅ `Change 'board-signal-reports' is valid`，exit 0 |
| 8.2 `doc-impact.sh verify openspec/changes/board-signal-reports` | ✅ 通过（声明: flow, api, database；exit 0） |
| 8.2 `check-standards.sh --change board-signal-reports` | ✅ 整体 exit 0（179 通过 / 15 失败，本 change 相关检查全 OK 含 doc-impact 对账与主 spec 132 validate） |
| 8.3 `golangci-lint run ./...` | ✅ 0 issues |
| 8.3 `go vet ./...` | ✅ exit 0 |
| 8.3 `go build ./...` | ✅ exit 0 |

## ④ 发现并修复的接缝问题

- **无业务代码接缝 bug**：阶段0-3 交付与本批文档/对账未发现冲突；实现零改动。
- **备知（未修，非本 change 辖区）**：check-standards 的 15 个 FAIL 全部是其他 change（2026-09-17~19 harness/dev 类：dev-process-guard、fix-bulk-markall-all-scope、harness-retro-loop 等）归档后未在 flow 溯源表补行的欠账，与 board-signal-reports 无关，按硬边界不代修——留主线程/相应 owner 收口。
- **change-scope.sh 用法备注**：该脚本不接受 change 名参数（支持 `--base <ref>` / `--json`），直接跑时扫全树脏改、会报出其他 change 的域；6.T1 按 brief 预期口径（dataenrichment 域 + platform/database）人工选定目标命令，未跑全量。

## ⑤ 未做项（按 brief）

- **8.4** `deploy-frontend.sh` 静态部署——留阶段6。
- **8.5** 人工双视口 UI 验收（1440×900/1920×1080/375px）——留阶段6。
- **8.6** `test-patrol.sh --report` + `archive-readiness.sh`——留阶段6。
- **6.T4** 人工静态 :5100 全链路走查（S1→S2、网络请求与截图）——留阶段6。
- **6.T5** 人工真实三类板块样本验收（OB-3 量化）——留阶段6。
- tasks.md 8.4~8.6 未勾选；两处 flow 变更溯源为「实现完成待归档补链接」占位，归档后按 §12 补。

## ⑥ 裁决请求

1. **check-standards 15 个跨 change 溯源 FAIL 的归属**：均为 2026-09-17~19 已归档 harness/dev change 的 flow 欠账（脚本整体仍 exit 0，不阻塞本 change）。请裁决由主线程统一补溯源行，还是留给各 change owner。
2. **change-scope.sh 在多 change 并行脏改树上的口径**：当前无 `--change` 参数，脏改树会混入他 change 域映射。阶段2a/2b/4 三次都按「brief 预期 + 人工选定」收敛，是否值得给脚本加 change 名过滤（属 harness 改进，非本 change）。
3. **前端遗留裁决项（phase-3 ⑥已提，随归档需收口）**：①旧报告组件文件（BoardBriefReport/BoardInvestigationReport/BoardAnalysisReport/BoardRelationPanel 及测试约 1400 行）是否物理删除；②聚焦分析折叠区去留。本批文档按「卸载保留」现状落笔，若裁决删除需同步 flow/map 文档两处表述。
