# 实现任务：问题驱动新闻研究

> 仅规划，以下全部未执行。前置：redesign-enrichment-research-workbench 完成并同步主spec；本change复用其UI/schema3，不与其并发改同文件。测试故事和旧契约处置见test-cases.md。新增测试文件仅service/signal_sources_test.go、service/signal_page_fetch_test.go，其余扩既有资产。

## 1. 开工与用例先行

- [x] 1.1 核对前置change、并发脏文件与当前代码；重跑 `bash scripts/harness/test-assets.sh board-signal-reports` 和 `bash scripts/harness/test-assets.sh research-data-sources`，期望本change的继承表与当前主规格逐项一致；有漂移先更新设计，不覆盖无关改动。
- [x] 1.2 跑 `bash scripts/harness/doc-impact.sh suggest --change add-question-driven-news-research` 并核对约束注入中的data-enrichment/topic-graph；当前脚本context已由constraint-injection替代，不执行已退役子命令。交付：test-cases.md的S1～S10与继承表经实施基线复核。
- [x] 1.3 按S1～S10先写核心断言fixture，使用TestQuestion前缀；文件存在：两个计划新增service测试文件与既有测试增量；先记录未实现行为的失败，不为凑全量添加通用测试框架。

## 2. 问题发现与冻结来源

- [x] 2.1 修改detect输出和候选JSONB可空research_brief迁移，保留旧NULL/score阈值与两次尝试；GET返回冻结文案与协议。验收：`cd backend-go && go test ./internal/dataenrichment/service ./internal/dataenrichment/handler -run 'Test(Signal|Question)'`，期望S2/S3非法输入失败、新旧字段分支成立。
- [x] 2.2 按D2批量查询有效section关联存文，去重、有界截取、保存source_ids与发现协议；不增加网络或逐篇LLM。验收：`cd backend-go && go test ./internal/dataenrichment/repository ./internal/dataenrichment/service -run 'Test(Signal|Question)'`，期望S1/S3冻结后编辑/删除原文不影响、DB故障无半批。
- [x] 2.3 在临时PostgreSQL验证nullable迁移、原子保存与旧报告约束；扩既有board_signal_migration_test.go。验收：`cd backend-go && go test ./internal/platform/database -run 'Test.*Signal|TestQuestion'`，期望旧NULL不回填、非法owner/周期仍拒绝；禁止真库验证DDL。

## 3. 有界来源研究

- [x] 3.1 在signal子Registry按question-v1授权read_source/web_search/fetch_page并复用原runToolLoop；实现先读门、3搜/5页/40总预算、重复拦截与未知协议拒绝。验收：`cd backend-go && go test ./internal/dataenrichment/service -run 'Test(Signal|Question)'`，期望S4/S10旧候选仍仅七源，新候选早停或有限补证。
- [x] 3.2 复用BochaWebSearcher与既有配置，signal专用safe page adapter经safefetch后解析正文与来源时间；只允许登记URL，未知/晚于cutoff日期拒绝引用。验收：`cd backend-go && go test ./internal/dataenrichment/service -run 'TestQuestion(SafePageFetch|SourceTime|SourceBudget|EvidenceEligibility)'`，期望S8危险目标请求为0、日期边界和typed gap成立。
- [x] 3.3 扩ledger与progress保留文字来源/搜索读取记录；区分数值/文字工具计数，错误不冒充成功，输入源内容不获得指令权。验收：`cd backend-go && go test ./internal/dataenrichment/service ./internal/dataenrichment/repository -run 'Test(Signal|Question)'`，期望S4/S6失败保留进展、成功附录可重放。
- [x] 3.4 在wire中接入新协议适配，不改旧调查/QA Registry行为或七源参数；LLM仍全部经airouter并共用研究session。验收：`cd backend-go && go test ./internal/datasources/wiring ./internal/dataenrichment/service -run 'Test.*(Signal|Question|Tool|Registry)'`，期望S10旧授权不变、发现零工具调用。

## 4. 答题范围与现有UI

- [x] 4.1 扩schema3的research_contract/answer与source引用验证，保持3000可见字符、四角色、3次成文上限和data/calc路径；非方向题用conditional。验收：`cd backend-go && go test ./internal/dataenrichment/service -run 'Test(Signal|Question)'`，期望S5/S6无证据explained、悬空source、source进calculate均拒绝，unresolved可成文。
- [x] 4.2 扩boardSignals API类型与既有候选详情/报告核查区，展示保存的未知、核查方向、答题范围、来源时间/片段与外搜说明；不新增任何页面/弹窗/Store。验收：`cd front && pnpm test:unit app/features/tags/components/SignalCandidateList.test.ts app/features/tags/components/SignalReportView.test.ts app/features/tags/components/reportPresentation.test.ts app/features/tags/composables/useSignalWorkbench.test.ts --maxWorkers=2`，期望S9新旧兼容、点击定位、安全URL与错误态成立。
- [x] 4.3 保留默认200/显式202/运行409/忽略边界与旧协议重研究；不自动升级旧候选或报告。验收：`cd backend-go && go test ./internal/dataenrichment/handler ./internal/dataenrichment/repository -run 'Test.*Signal|TestQuestion'`，期望S7及原ignore竞争回归不变。

## 5. Review

- [x] 5.1 派子线程聚焦 review（审查制品 + diff），重点检查来源安全、冻结时间、旧协议/旧UI兼容、答题范围与不越界；意见按High/Medium/Low处置后review.md建账。验收：文件存在且每条意见有处置；若派发仍故障先报告，不冒充已审、不绕CLI；豁免须显式另行记录。

## 6. 测试

- [x] 6.1 跑 `bash scripts/harness/change-scope.sh` 机械判定影响包，然后按本次路径执行受影响测试；不顺手跑全量go test或前端全量单测。期望test-cases.md每个故事的确定性断言通过，失败仍保持任务未完成。
- [ ] 6.2 按test-cases.md E1～E3向用户确认真实样本/模型/预算，再在隔离验收库顺序运行；交付research-validation.md（真实计数、引用定位比例、答题对照、成本及结论），无未授权调用。若授权未获或样本不足，记录blocked而非PASS。
- [ ] 6.3 agent-browser在静态:5100完成S9主链路与旧报告回读，1440×900/390×844、双主题、运行态和长来源文本；核对点击目标、scrollWidth及重复提交，关闭浏览器。交付ui-verification/report.md按A-N记录依据；没有运行不得写pass。

## 7. 文档
<!-- doc-impact: flow api database configuration -->

- [ ] 7.1 更新docs/reference/flow/data-enrichment.md的发现/研究/冻结/来源边界和费用说明；topic-graph不改生命周期契约。验收：文档所列流程与S1～S10一致，归档后补永久溯源链接。
- [ ] 7.2 更新docs/reference/api/board-signals.md和docs/reference/database/tables/data-enrichment.md，明确可空research_brief、协议与JSON增量、旧候选行为；更新docs/reference/configuration.md说明复用Bocha与缺配置降级。验收：字段名与DTO一致，未宣称新供应商/自动监控。
- [ ] 7.3 跑 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-question-driven-news-research` 与 `bash scripts/harness/check-standards.sh`，期望声明文档无缺失、格式一致；不以其他change的文档改动冒充本次更新。

## 8. 验证

- [ ] 8.1 `openspec validate add-question-driven-news-research --strict` 与 `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-question-driven-news-research` → 两者退出0。
- [ ] 8.2 `bash scripts/harness/scenario-trace.sh openspec/changes/add-question-driven-news-research` → 每Scenario有下表映射；存在映射不代表测试已运行，逐故事验收留痕才完成。
- [ ] 8.3 在backend-go按change-scope涉及包跑golangci-lint/go vet/go build及定向go test → 无本change回归；在front跑 `pnpm lint`、`pnpm exec nuxi typecheck` 和上述定向Vitest → 通过，随后串行 `pnpm build` → 成功；不与真实研究/浏览器并行。
- [ ] 8.4 前端验证通过后 `bash scripts/dev/deploy-frontend.sh` → 静态部署与健康检查通过；人工：核对新旧候选说明、旧报告原文不变、无自动付费任务，并汇报配置/重新发现操作。
- [ ] 8.5 归档前 `bash scripts/harness/test-patrol.sh --report` → 检查已有测试欠账；与本change无关的故障按台账登记，本change回归必须修复；无真实样本/UI依据不得称实现完成。

### Scenario → 测试文件

下表路径与test-cases.md故事一致；两个新测试文件为计划创建，不伪装当前已存在。多数条目继承已有断言，不为63个Scenario生成63个新测试文件。

| Scenario | 测试文件 |
| --- | --- |
| 周期校验 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 历史材料隔离 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 迟些点击仍用原快照 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 外部数据边界 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 只冻结合窗人工关注 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 后续撤销不改旧研究 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 失效依据不升级为有效信号 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 事后选择不冒充当时预测 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 新发现冻结有限原文 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 原文缺失与后改不偷换 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 旧候选不自动升级 | backend-go/internal/dataenrichment/service/signal_material_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/repository/signal_candidate_test.go |
| 达标只生成候选 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 零信号零打扰 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 检测故障可见 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 依据白名单 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 普通读者的问题推荐 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 问题无价值时不凑数 | backend-go/internal/dataenrichment/service/signal_detect_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go |
| 刷新与重启保留候选 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go, front/app/features/tags/components/SignalCandidateList.test.ts |
| 重复发现不覆盖 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go, front/app/features/tags/components/SignalCandidateList.test.ts |
| 列表字段可追溯 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go, front/app/features/tags/components/SignalCandidateList.test.ts |
| 新旧简要说明与非法输出 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go, front/app/features/tags/components/SignalCandidateList.test.ts |
| 来源冻结保存失败 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go, backend-go/internal/dataenrichment/handler/signal_discovery_test.go, front/app/features/tags/components/SignalCandidateList.test.ts |
| 空手源提示早停 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 进程重启收敛孤儿进展 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 十几轮查询正常完成 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 达到四十轮收束 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 重复与非法动作有限结束 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 工具失败和任务超时不同 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 超时后进展不白跑 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 成功后进展归档 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 原文已足够则早停 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 先核前提再补证 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 网页预算与配置失败 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 不把工具可用性当问题答案 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, backend-go/internal/dataenrichment/handler/signal_research_test.go |
| 不足以转多也能成文 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 结构与篇幅校验 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 图表降级 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 无数字仍可成文 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 块内引用必须核查 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 缺证据也有可读结果 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 答题范围不是可信度 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 来源内指令不是系统指令 | backend-go/internal/dataenrichment/service/signal_compose_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 精确引用与定位 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 图表口径不混淆 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 完整账本 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 长小数与极小值 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 真实日期与图表断点 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 文字来源可定位不可代算 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 摘要与悬空来源拒绝 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| 进展保留文字来源 | backend-go/internal/dataenrichment/service/signal_compose_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go, front/app/features/tags/components/reportPresentation.test.ts, backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |
| PG直接写入约束 | backend-go/internal/dataenrichment/repository/signal_report_test.go, backend-go/internal/platform/database/board_signal_migration_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 报告完成直接可读 | backend-go/internal/dataenrichment/repository/signal_report_test.go, backend-go/internal/platform/database/board_signal_migration_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 旧数据与新版并存 | backend-go/internal/dataenrichment/repository/signal_report_test.go, backend-go/internal/platform/database/board_signal_migration_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 新协议追加而不改历史 | backend-go/internal/dataenrichment/repository/signal_report_test.go, backend-go/internal/platform/database/board_signal_migration_test.go, front/app/features/tags/components/SignalReportView.test.ts |
| 公网与重定向安全 | backend-go/internal/dataenrichment/service/signal_page_fetch_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go |
| 来源时间不能冒充研究时点 | backend-go/internal/dataenrichment/service/signal_page_fetch_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go |
| 来源截取与空正文 | backend-go/internal/dataenrichment/service/signal_page_fetch_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go |
| 检索出站最小化 | backend-go/internal/dataenrichment/service/signal_page_fetch_test.go, backend-go/internal/dataenrichment/service/signal_sources_test.go |
| 从问题到来源的主链路 | front/app/features/tags/components/SignalCandidateList.test.ts, front/app/features/tags/components/SignalReportView.test.ts, front/app/features/tags/composables/useSignalWorkbench.test.ts |
| 新旧和异常态可读 | front/app/features/tags/components/SignalCandidateList.test.ts, front/app/features/tags/components/SignalReportView.test.ts, front/app/features/tags/composables/useSignalWorkbench.test.ts |
| 研究显式授权 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/datasources/wiring/tools_test.go |
| 注入后现有工具面不变 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/datasources/wiring/tools_test.go |
| 来源能力不改变数值工具语义 | backend-go/internal/dataenrichment/service/signal_research_test.go, backend-go/internal/datasources/wiring/tools_test.go |
