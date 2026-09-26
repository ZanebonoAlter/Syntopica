# 审查 brief：board-signal-reports「实战校准四组修复」增量（2026-09-23）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令（只允许 read/grep/find/ls 与只读验证，如 `go vet`/`go test -run` 单测属可，但优先 grep 与精读）。

## 一、审查对象（本轮增量，未 commit，全在工作树）

背景：首份真实 signal_report（`topic_enrichment_result.id=19`，40 轮全用满、数据滞后致文不对题）诊断出四组缺陷，本轮按 change `board-signal-reports` 的 design §10 实现修复。诊断事实全文见 `openspec/changes/board-signal-reports/explore-findings.md` 末节「2026-09-23 实战校准」。

本轮新增任务（全部已勾选，证据在 tasks.md）：

| 任务 | 辖区代码 |
| --- | --- |
| 3.6 EIA 归档 host 白名单 | `backend-go/internal/datasources/catalog.go`（eia_wpsr HostAllowlist 补 www.eia.gov）、新增 `datasources/catalog_allowlist_test.go` |
| 3.7 能力与覆盖前置声明 | 研究侧 `datasources/wiring/tools.go`（appendCatalogMetadata/appendUnavailableNotice，register.go BuildTools 挂接）；发现侧 `datasources/wiring/capability.go`（SourceCapabilityText）→ `dataenrichment/wire.go` → `service/signal_detect.go`（setter+detectSystemPrompt） |
| 3.8 无覆盖早停反馈 | `service/orchestrator.go`（可选接口 toolLoopFeedbackProvider + appendToolLoopFeedback 六处挂点）、`service/signal_research.go`（streak=3/sourceNudged/finaleNudged/trackSourceCoverage/RunLoopFeedback） |
| 3.9 data_sources status 状态源 | `datasources/init.go`（Init(db, resolve)）、`cmd/server/main.go`（传 wiring.ComtradeKeyResolver）、`datasources/repository.go`（StatusFor/UpdateStatus 抽公共）、`admin/handler/comtrade_settings_handler.go`（保存后回写） |
| 4.9 成文数据时效 | `service/signal_compose.go`（signalPeriodRank/signalSourceLatestPeriods/signalDataAvailabilityLine + prompt 时效纪律 + validateSignalReportDraft 增 facts「数据截至」校验）、`signal_research_test.go`（srComposeOK stub 补时效行——连带修 16 条全链用例） |
| 4.10 孤儿进展收敛 | `repository/signal_repository.go`（SweepOrphanedSignalResearchProgress）、`app/runtime.go`（StartRuntime 内 resetStaleStates 后调用）、wire re-export |
| 5.5 前端 as-of | `front/app/features/tags/components/signalReport.ts`（latestObservationPeriod）、`SignalReportView.vue`（as-of 机械行 data-testid=signal-asof）、`SignalReportView.test.ts` |

设计权威源：`openspec/changes/board-signal-reports/design.md` §10（10.1 能力与 host 声明 / 10.2 早停反馈 / 10.3 时效声明 / 10.4 残留与状态治理）。
验收契约：`specs/research-data-sources/spec.md` 新增 Requirement「源能力与host如实声明」三 Scenario；`specs/board-signal-reports/spec.md` 四十轮 Requirement 增「空手源提示早停」「进程重启收敛孤儿进展」两 Scenario + 新 Requirement「报告数据时效声明」两 Scenario。
Scenario→测试映射：tasks.md「Scenario落点」表新增 7 行。

## 二、审查维度（按开发执行规范 §0.6 步骤4）

1. **精读高风险文件**（至少覆盖）：
   - `service/orchestrator.go`：新增反馈注入是否真的不改 `toolLoopPolicy` 三方法签名、不 fork 循环、`policy=nil` 与未实现接口者字节不变；六个挂点是否遗漏/重复；反馈行是否可能被误当工具结果。
   - `service/signal_research.go`：计数/触发逻辑是否与 design §10.2 逐条对齐（3 连空手、每源恰一次、终局恰一次、calculate/被拦轮不计数、成功清 streak）；**40 轮预算与 stop_reason 分类（finished/budget_exhausted）是否零改动**。
   - `service/signal_compose.go`：新校验是否与既有校验同通道（problems 聚合）、是否放宽任何既有规则；可用性行是否机械计算（不引入模型自造期值的通道）；零观测分支。
   - `datasources/`（catalog/init/repository/wiring/capability + 两个新测试）：allowlist 是否单一事实来源、测试是否从 `Catalog()` 派生而非手写（这正是上一轮漏网原因）；StatusFor/UpdateStatus 是否真单一判断无漂移；Init 签名改动是否有漏改调用方。
   - `app/runtime.go` sweep：UPDATE 是否只碰 running、幂等、失败不阻塞启动；启动时点是否结构性保证内存无活 job；与 DEMO_READ_ONLY 的取舍。
   - `signalReport.ts` / `SignalReportView.vue`：period 规范化比较是否与后端 `signalPeriodRank` 同构、无观测回退、是否只读 payload。
2. **grep 全包验证不变量**（至少）：
   - 旧流程零变化：`allowedTools`/`explorationToolNames`/`buildAgentAllowedTools` 与四源的关系；`toolLoopPolicy` 方法集未被改签名（investigation policy 仍编译且未实现新接口）。
   - `signalResearchMaxLoops` 仍=40、budget 分类逻辑行未被改。
   - `service` 包不 import `datasources/wiring`（import 环）；`boardSignals.ts` 与 payload schema 零改动。
   - 测试不连业务库（grep DSN/localhost:5432 出现在测试文件即 High）。
   - 新增 UPDATE/迁移语句是否只在启动收敛路径。
3. **分级**：High（bug/安全/清晰错误，必修）/ Medium（合理但依赖上下文，选修）/ Low（误报，静默丢弃）。

## 三、输出要求（三段式，中文）

1. **总评**：本轮四组修复是否达成 design §10 意图、可否进入门禁收口（一句话结论 + 理由）。
2. **问题清单**：按 High/Medium/Low 分级，每条给 `file:line` + 复现/依据 + 建议修法；没有就明确写「无」。
3. **核实记录**：你实际跑过的只读命令与退出码、读过的关键文件清单（防幻觉）。

已知且已裁决、**不要**当问题报的事项：
- 反馈每源终身一次（成功不重新武装 nudged）——controller 已按 design §10.2「每源最多注入一次」裁决维持。
- 第 4 源触发轮与终局提示拼在同一返回值内（换行分隔）——机械结果，已接受。
- `StatusFor` 的 reason 文案仍含「配置后重启」（UI 路径实际已即时生效）——文案级已知残留。
- DEMO_READ_ONLY 下不跑 sweep——与 resetStaleStates 同取舍，已接受。
- 前端 as-of 只管显示不管正文旧稿——存量 id=19 正文无「数据截至」属设计内（机械行兜底）。
- 树上 374 个脏文件含其他 active change 预存改动——超出本轮辖区的一律不评。
