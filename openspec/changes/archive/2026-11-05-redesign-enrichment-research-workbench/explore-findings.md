
## 话题关联可直接从冻结材料解析，不必重新装配新闻

service/signal_material.go 的 SignalMaterial.Lanes[] 为 SignalLaneMaterial，含 LaneID/Label/Status、Summary/Background、Articles[]；SignalArticleSlice 含原始 SectionID 等期间切片信息。因此候选 EvidenceRefs 可经既有兼容解码器，查冻结 Lanes[].Articles[].SectionID→LaneID，当前话题表仅验证目标仍存在且属于允许board，不用今天active列表重新归属，也不修改InputSnapshot。新加的 ConfirmedKeyEvents 是相邻add-topic-key-event-markers的attention提示，不能拿来扩展候选证据白名单；本change解析时保持兼容且不写回。真实话题总览宿主是 TagsPage.vue 的 BoardThreadBrowser，不是 BoardTimelinePanel（后者是文章视图）。数据库增量迁移在 internal/platform/database/postgres_migrations.go，测试在同包；不存在假定的独立migrations/20260922目录。

**引用**：backend-go/internal/dataenrichment/service/signal_material.go:SignalMaterial、backend-go/internal/dataenrichment/service/signal_material.go:SignalLaneMaterial、front/app/features/tags/components/TagsPage.vue:318、backend-go/internal/platform/database/postgres_migrations.go

<!-- pinned 2026-10-15T15:20:34Z -->

## 任务4.1 宿主改造关键事实

BoardEnrichmentPanel.vue（865行）现结构：signal 工作台 section（头部 h2「信号解读」+ SignalCandidateList + SignalReportList + activeReportId 激活时 SignalReportView 替换视图）+ 三个待退役区块（focus-analysis 折叠区含泳道下拉/CausalAnalysisReport/QAPanel、sec1 新闻背景折叠区含周期筛选器/叙事编辑/补生成 AppDialog、DebateSection）。useBoardEnrichment 解构需缩到 bootstrap 最小集：selectedTopicId/loadTopics/loadDataSources/loadBoardAnalysisResults/syncBoardAnalysisStatus/syncTopicAnalysisStatus/activateBoardContext/loadAllTopicTables（watch(selectedTopicId)→loadAllTopicTables 保留）。useSignalWorkbench 已有全部状态：candidateVisibility/setCandidateVisibility/activeCandidate(+Loading/Error)/openCandidate/closeCandidate/ignoreCandidate/restoreCandidate/researchTaskPeriod/researchTaskGranularity/returnToResearchTask/reports/openReport。SignalCandidateDetail 字段：ignored_at/related_topics{topic_id,label,frozen_label,frozen_status}/related_gaps/cutoff；SignalReportRow.source_signal_id 用于版本过滤。
布局契约：AppPageShell mode="split" aside-side="left"，#aside 放索引、默认 slot 放详情；asideMin 默认 280px 内联 --aside-min（窄档 244px 需 !important 覆盖），--aside-max 非内联可直接 CSS 覆盖为 280px；组件需显式 import（vitest 无 auto-import）。useIsNarrowViewport()（~/composables/useMediaQuery）在 vitest 下恒 false（import.meta.client undefined 短路），测试环境安全。
现有坑：panel 未给 SignalCandidateList 传 :research-progress（4.8 行内 trace 实际失效），本次补上。测试基建：wrapper.emitted() 不可用（用 attrs handler spy）；panel 测试 mock 整个 useSignalWorkbench（signal 对象需扩 activeCandidate* / ignore / restore / candidateVisibility / researchTaskPeriod 等）；bootstrap.test 走真实 composable 只 mock API，SignalCandidateList 以真实组件挂载（新 props 需带默认值）。panel 测试 stub SignalReportList 需改「不渲染」断言（split 后报告列表退役，版本挂在候选详情）。

**引用**：front/app/features/tags/components/BoardEnrichmentPanel.vue、front/app/features/tags/components/SignalCandidateList.vue、front/app/features/tags/composables/useSignalWorkbench.ts、front/app/components/ui/AppPageShell.vue

<!-- pinned 2026-10-15T17:17:45Z -->

## 5.2 LegacyResearchHistory 只读历史外壳：契约与实现锚

5.2 完成态（3 文件，未 commit）：front/app/api/boardEnrichment.ts 新增 LegacyHistoryRow/LegacyHistoryKind/LegacyHistoryQuery 类型 + useBoardEnrichmentApi().listLegacyHistory(boardId, params)（GET /semantic-boards/:id/enrichment/analysis/history，驼峰转下划线 query）。新组件 front/app/features/tags/components/LegacyResearchHistory.vue：props {boardId:number; open:boolean}，emits ['update:open', 'open-topic'(topicId)]；自带 AppDialog size="xl"（宿主只需 `<LegacyResearchHistory v-model:open="x" :board-id="b" @open-topic="..."/>`，勿再包一层弹窗）。详情分发：board_brief→BoardBriefReport、board_investigation→BoardInvestigationReport、legacy_board_analysis→BoardAnalysisReport（均经 getBoardAnalysisResult(boardId,id)，载荷 sectors 为 null/非对象即降级「原始记录不可解析」+只读原始 JSON）；topic_analysis→CausalAnalysisReport（经 getResult(persistent_topic_id,id)）。子产物只读：board kind 走 listBoardQA，topic kind 走 listQA+listDebates+listReviews(按 curr_result_id 过滤)；QAPanel 因内置 ask/sediment 未复用，QA 静态渲染；DebateSection 以 stage="result" 仅在有数据时挂。未知 kind 或 topic 行缺 persistent_topic_id → 不调详情直接降级。关键机制：reader 生成入口（.bb-discover-chip/.bb-investigate-btn/.bb-custom/[data-test^=cross-open-board-]/[data-test^=inv-laneref-open-]/.bi-lane-cross）由组件 hideWriteControls() 置内联 display:none + scoped :deep 样式表双保险。坑：vitest(happy-dom) 下 SFC 样式表不注入、getComputedStyle().display 返回空 → VTU isVisible 不可靠，测隐藏要断言 inline style（测试里 hiddenInline 帮助函数）。宿主接线（后续任务）注意：组件内部 watch([open,boardId],{immediate}) 打开即重拉列表并清选中；drill-lane 已统一映射为 open-topic(laneId)，跨版块跳转按钮被藏、open-board 未接线。

**引用**：front/app/api/boardEnrichment.ts:listLegacyHistory、front/app/features/tags/components/LegacyResearchHistory.vue、front/app/features/tags/components/LegacyResearchHistory.test.ts

<!-- pinned 2026-10-15T17:38:48Z -->

## redesign-walkthrough apply 续作交接（19+6/25 完成，7.2/7.3 收尾中）

## 已完成（tasks.md 已勾）
1.x 全部、2.x 全部、3.x 全部、4.x 全部、5.x 全部、6.1（review.md 建账 R1-16 全处置）、8.x 全部、9.1-9.7 全部勾（9.7 部署过两次）。

## 进行中：7.2 浏览器走查 + 7.3 效果核对（证据目录 ui-verification/evidence/ 已有 A1-A9 截图）
- **7.3 指标已采集完毕**（result_id=21，真实研究）：schema3、attempts 1/0、块放置合规（thesis/facts=[paragraph,evidence_table,chart]/causal/implication）、无坏块、data_freshness 分源（gdelt 无观测如实）、表 label 无复制值、1 图、appendix 8 calls+5 calcs。只差写进 ui-verification/report.md。
- **刚修最后一个 bug（已测绿未部署）**：loadCandidates 无参数陈旧守卫 → bootstrap 慢响应覆盖 ignored 筛选结果（症状：已忽略 tab 显示 7 行）。修复=请求级 visibility/view 快照比对丢弃。**需重跑 deploy-frontend.sh（首次构建可能 node OOM，用 NODE_OPTIONS=--max-old-space-size=3072）**。
- **待补证据（重部署后在 :5100 板块1974 中东地缘）**：①忽略候选02→已忽略tab应显示1行+恢复按钮→恢复→8行；②返回任务按钮（R-16 修复后：切周期应出现 signal-return-task）；③1920/375/390 视口 + dark 主题截图；④viewport 用 agent-browser set viewport。
- **A 组证据已存**：A1 首屏/A2 研究tab/A3 主从索引/A4+A4b 历史弹层（四旧kind行）/A5 旧版分析#5只读（无写控件已断言）/A6 相关话题背景预览md（只读空态）/A7 话题总览背景管理lg（补生成显式）/A8 schema3报告渲染（分源时效行+证据表+引用值）/A9 旧构建忽略视图（作废重拍）。
- 走查会话：export AGENT_BROWSER_SESSION=$(cat /tmp/ab_session)（agent-browser 进程被 dev-process-guard 标记属正常，会话结束自动清）。
- **真实数据状态**：板块1974 有候选（2026-10 八条：#8 已研究成报告21、#2 已忽略态——恢复后才能回 visible）；2026-09 六条。研究已按用户授权跑过一次（预算已消耗，勿再跑）。

## 收尾清单（证据齐后）
1. 重部署+补证据 → 写 ui-verification/report.md（格式 `### A-N 场景【pass|fail|未覆盖】`，7.3 指标入 B 节）
2. 勾 7.2/7.3 → 跑 9.5 复核（check-standards 唯一 FAIL 是 fix-board-ghost-false-positive 的欠账，非本 change）→ 归档门禁走 §11
3. review.md R-9 提到的 loadCandidates 竞态=新 R-17，补进 review.md（已修复状态）
4. 部署影响汇报模板在 proposal.md 末节（用户无需手工操作，旧数据零降级）

**引用**：openspec/changes/redesign-enrichment-research-workbench/tasks.md、openspec/changes/redesign-enrichment-research-workbench/review.md、front/app/features/tags/composables/useSignalWorkbench.ts

<!-- pinned 2026-10-16T02:13:51Z -->
