# 研究工作台实施任务

> 本文件是规划，全部实施项未开始。UI已批准仍不代表本轮可apply。按用户要求精准改动/测试：复用优先，不新增通用框架，不跑全量套件。

## 1. 准入与基线

- [x] 1.1 新请求进入apply后读取本change及相关AGENTS/flow红线，运行doc-impact suggest与concurrency-status；交付本change归属/冲突清单，不覆盖add-topic-key-event-markers。
- [x] 1.2 依据test-cases.md确认旧Scenario继承和新增风险，展示字段对照design D8；交付仅本次影响的测试/修改清单，不为每组件造一套测试。

## 2. 候选处理与查询

- [x] 2.1 新增独立disposition表与幂等迁移，不改候选/报告；在既有board_signal_migration_test和小型signal_disposition_test临时PG断言重复迁移、FK、忽略/恢复和raw不变。
- [x] 2.2 在原board门禁串行处理忽略与研究，补PUT和必要详情/筛选，复用原repo读取；既有research handler测试锁住两个竞争顺序、owner和已有报告幂等读取。
- [x] 2.3 从冻结Lanes/Articles解析关联及gap，不重装材料；在现有material测试补多话题/归档/失效代表，确认ConfirmedKeyEvents仍仅attention。

## 3. 成文与呈现

- [x] 3.1 在原composer仅加三类平面块、schema3和代码兼容text，保留四角色/引用/重试；扩展signal_compose_test验证合法稿、关键非法稿和schema2读取，不加通用AST。
- [x] 3.2 统一阅读短格式与raw、分源时效、真实日期和小图；复用SignalReportView并只提取真实复用的逻辑/组件。reportPresentation参数表及现有reader挂载验证长小数/非零小量/null/日期间距，不改源单位和账本。

## 4. 工作台与话题背景

- [x] 4.1 改现有BoardEnrichmentPanel/SignalCandidateList为已批准主从布局；保留发现/研究边界并接忽略恢复和真实版本。既有宿主测试验证无隐式模型调用及按钮允许性。
- [x] 4.2 在既有composable内分离当前视图和当前board任务；周期/版本可读、任务可找回、晚响应不串台。只在useSignalWorkbench/bootstrap测试补关键跨scope断言，不建全局任务中心。（researchTaskPeriod/Granularity 任务身份快照 + returnToResearchTask + epoch/请求级参数双守卫防晚响应串台；R-16 后恢复链路同样走快照）
- [x] 4.3 背景迁到话题上下文并复用原contexts API，提供研究中只读预览；单个TopicNewsBackground挂载测试验证scope、保存失败草稿、明确生成确认和不改研究。

## 5. 旧历史

- [x] 5.1 只补按board/topic/kind分页的只读历史列表，详情/QA/辩论/review复用既有GET；小型legacy_history repo和已有handler测试核对owner/归档topic/分页，读取零LLM，不新增重复历史详情API。
- [x] 5.2 一个只读外壳复用旧reader，退役旧生成/追问/沉淀控件；LegacyResearchHistory挂载一组旧kind与子产物fixture，未知单条降级但原记录可读。

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账；重点只看协议、竞态、兼容和真实回归，派发附必读文件与红线，model用provider/modelId全称。（2 High + 4 Medium + 15 Low 全部处置：High/中全部修复，Low 采纳 3/驳回 7/误报 1，见 review.md）

## 7. 测试

- [x] 7.1 扩展tasks下表对应的旧测试，必要新文件限disposition、legacy_history、TopicNewsBackground、LegacyResearchHistory、reportPresentation；相关边界合并参数用例，不另铺框架或未改引擎全矩阵。
- [x] 7.2 在隔离fixture执行test-cases.md的一条agent-browser主链与历史/背景短走查；证据写ui-verification/report.md，两个桌面/两个窄屏和双主题一并检查，不用全站回归替代。（A1-A16 建账：14 pass、E2E 正例 1 未覆盖（单测兑底+端点接线断言）、存量观察 1；走查发现 R-16/R-17/R-18 已修复回验）
- [x] 7.3 做最小效果核对：两个只读旧样本、一次已授权混合证据成文；记录可判定指标，不启动新发现或40轮研究，未授权/缺样本保持未完成。（候选08真实成文 result_id=21：schema3/attempts 1-0/块放置合规/无复制值/分源时效 gdelt 无观测如实/raw 全量保持；旧样本只读核对见 A4/A5）

## 8. 文档

<!-- doc-impact: flow, api, database, architecture, standard -->

- [x] 8.1 更新flow/data-enrichment.md、flow/topic-graph.md、api/board-signals.md、api/dataenrichment.md、database/tables/data-enrichment.md；写明新接口/字段和旧入口退役，不删除原数据；逐项与实现核对。
- [x] 8.2 更新architecture/ui-navigation.md及现有standard/frontend/interaction-conventions.md的必要呈现约定，不新建一套规范；补部署影响/用户操作/旧数据降级及归档溯源。

## 9. 验证

- [x] 9.1 `bash scripts/harness/change-scope.sh --json` + `bash scripts/harness/concurrency-status.sh redesign-enrichment-research-workbench` → 确认本change影响包/文件与他人改动分开；不照着混合脏树跑全量。
- [x] 9.2 `cd backend-go && go test -p 1 ./internal/dataenrichment/repository ./internal/dataenrichment/service ./internal/dataenrichment/handler ./internal/platform/database -run 'Signal|Compose|LegacyHistory|Context|BoardAnalysis|QA'` → 本次映射与必要兼容用例通过，PG仅临时fixture；不跑go test ./...。
- [x] 9.3 `cd backend-go && golangci-lint run ./internal/dataenrichment/... ./internal/platform/database/... && go vet ./internal/dataenrichment/... ./internal/platform/database && go build ./...` → 无本change引入的lint/vet/构建错误，串行执行。
- [x] 9.4 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit app/features/tags/components/BoardEnrichmentPanel app/features/tags/components/SignalCandidateList.test.ts app/features/tags/components/SignalReportView.test.ts app/features/tags/components/reportPresentation.test.ts app/features/tags/components/TopicNewsBackground.test.ts app/features/tags/components/LegacyResearchHistory.test.ts app/features/tags/components/BoardThreadBrowser.focus.test.ts app/features/tags/components/QAPanel.test.ts app/features/tags/composables/useSignalWorkbench.test.ts app/api/boardEnrichment.test.ts --maxWorkers=2` → 仅相关文件通过；不带额外--，不与构建/浏览器并行。
- [x] 9.5 `openspec validate redesign-enrichment-research-workbench --strict && bash scripts/harness/check-tasks-tail.sh openspec/changes/redesign-enrichment-research-workbench && bash scripts/harness/doc-impact.sh verify --change redesign-enrichment-research-workbench && bash scripts/harness/check-standards.sh` → 制品/任务尾节/文档对账无本change阻断；apply前先完成suggest。（check-standards 唯一 FAIL 在 fix-board-ghost-false-positive，非本 change 欠账）
- [x] 9.6 `bash scripts/harness/test-patrol.sh --report` → 无未处理的本change红；非本change红按台账登记，不因此顺手跑全量。（fe-composables 抽片首跑检出 useMermaidRender 主题切换 1 红，单文件复跑 9/9 绿＋批跑复绿，系并发构建噪声；已登记台账待关）
- [x] 9.7 在确认部署环境与真实任务可安全重启后 `bash scripts/dev/deploy-frontend.sh`，再 `bash scripts/dev/start-dev.sh status` → 静态构建/铺盘/健康通过，浏览器按A项证据验收；未获中断授权不得擅停在跑研究。（用户已授权重启；首次构建 node OOM，限堆重试成功；http://localhost:5100/ 健侧 200）

### Scenario → 测试文件

下列路径中新增五个小文件尚不存在，须由apply实现；其余优先扩展旧资产。一条参数化测试可承接多行，人工完整故事另由7.2留痕。

| Scenario | 测试文件 |
| --- | --- |
| 刷新与重启保留候选 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 重复发现不覆盖 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 列表字段可追溯 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 点击一条只研究一条 | backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 重复点击与重新研究 | backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 跨板块拒绝 | backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 忽略与研究启动竞争 | backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 已忽略报告仍能只读 | backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 不足以转多也能成文 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 结构与篇幅校验 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 图表降级 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 无数字仍可成文 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 块内引用必须核查 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 正文与阅读视图都标数据截至 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 日度源参与时效声明 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 时效缺失回注重试 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 零观测不伪造时点 | backend-go/internal/dataenrichment/service/signal_compose_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 精确引用与定位 | front/app/features/tags/components/reportPresentation.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 图表口径不混淆 | front/app/features/tags/components/reportPresentation.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 完整账本 | front/app/features/tags/components/reportPresentation.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 长小数与极小值 | front/app/features/tags/components/reportPresentation.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 真实日期与图表断点 | front/app/features/tags/components/reportPresentation.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| PG直接写入约束 | backend-go/internal/dataenrichment/repository/signal_report_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 报告完成直接可读 | backend-go/internal/dataenrichment/repository/signal_report_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 旧数据与新版并存 | backend-go/internal/dataenrichment/repository/signal_report_test.go front/app/features/tags/components/SignalReportView.test.ts |
| 研究失败可重试 | front/app/features/tags/composables/useSignalWorkbench.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 重启恢复 | front/app/features/tags/composables/useSignalWorkbench.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 周期分页 | front/app/features/tags/composables/useSignalWorkbench.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 忽略筛选与分页兼容 | front/app/features/tags/composables/useSignalWorkbench.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 运行中仍能浏览 | front/app/features/tags/composables/useSignalWorkbench.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 索引切换与返回 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 关闭开关仍可阅读 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 加载失败不是空态 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |
| 忽略后重进与恢复 | backend-go/internal/dataenrichment/repository/signal_disposition_test.go backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 运行中的忽略不可达 | backend-go/internal/dataenrichment/repository/signal_disposition_test.go backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 处理写入故障与越权 | backend-go/internal/dataenrichment/repository/signal_disposition_test.go backend-go/internal/dataenrichment/handler/signal_research_test.go front/app/features/tags/components/SignalCandidateList.test.ts |
| 多话题和历史话题关联 | backend-go/internal/dataenrichment/service/signal_material_test.go front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 无法验证的归属 | backend-go/internal/dataenrichment/service/signal_material_test.go front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 展示不发明状态和质量 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 版本选择按真实结果 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/SignalReportView.test.ts |
| 工作台呈现候选与报告 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 周期筛选翻历史 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 证据链 tooltip 不跳转 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 兑现度复盘可见 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 契约为侦探墙铺路 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 简报到调查由用户确认 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 报告阅读无需评审 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 追问复用探索工具 | front/app/features/tags/components/QAPanel.test.ts front/app/api/boardEnrichment.test.ts front/app/features/tags/components/LegacyResearchHistory.test.ts |
| 多轮追问持久化 | front/app/features/tags/components/QAPanel.test.ts front/app/api/boardEnrichment.test.ts front/app/features/tags/components/LegacyResearchHistory.test.ts |
| 追问发现手动沉淀 | front/app/features/tags/components/QAPanel.test.ts front/app/api/boardEnrichment.test.ts front/app/features/tags/components/LegacyResearchHistory.test.ts |
| 从话题直接查看背景 | front/app/features/tags/components/TopicNewsBackground.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 背景编辑与研究隔离 | front/app/features/tags/components/TopicNewsBackground.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 补生成必须明确确认 | front/app/features/tags/components/TopicNewsBackground.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 背景失效与晚响应 | front/app/features/tags/components/TopicNewsBackground.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 从简报观察下钻 | front/app/features/tags/components/LegacyResearchHistory.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 从调查证据下钻 | front/app/features/tags/components/LegacyResearchHistory.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 查看旧报告 | front/app/features/tags/components/LegacyResearchHistory.test.ts backend-go/internal/dataenrichment/handler/handler_test.go |
| 单一下拉选择泳道 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 新闻背景入口保留 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 旧结果与所有子产物可读 | backend-go/internal/dataenrichment/repository/legacy_history_test.go backend-go/internal/dataenrichment/handler/handler_test.go front/app/features/tags/components/LegacyResearchHistory.test.ts |
| 历史归属与损坏隔离 | backend-go/internal/dataenrichment/repository/legacy_history_test.go backend-go/internal/dataenrichment/handler/handler_test.go front/app/features/tags/components/LegacyResearchHistory.test.ts |
| 历史分页与开关无关 | backend-go/internal/dataenrichment/repository/legacy_history_test.go backend-go/internal/dataenrichment/handler/handler_test.go front/app/features/tags/components/LegacyResearchHistory.test.ts |
