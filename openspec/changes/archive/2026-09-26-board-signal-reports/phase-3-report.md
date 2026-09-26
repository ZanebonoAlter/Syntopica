# 阶段3交付报告：前端（phase-3-report）

> 2026-09-22，develop 主仓库直改（树上其他 change 脏改未触碰、未还原）。范围=tasks 5.1~5.4；后端零改动（未 import、未编辑任何 backend-go 文件）；旧 `boardEnrichment.ts` API 模块零改动。

## ① 文件清单

**新增（7 个）**

| 文件 | 内容 |
| --- | --- |
| `front/app/api/boardSignals.ts` | 5.1 信号契约模块：发现/候选/研究/报告全部类型 + 6 端点函数（POST signal-discoveries、GET signals、POST signals/:id/research、GET signal-reports[/列表]、GET signal-reports/:rid、GET analysis-status?job_id= signal 版）。`SignalReportPayload`（schema_version=2：signal_snapshot/report/appendix/generation_meta）、`SignalCandidateRow`（派生 status + latest_result_id）、`SignalAnalysisJobStatus`（phase/outcome/error_stage/discovery_id/candidate_id/candidate_count/result_id 全可选，旧 kind 输出零变化语义保留）。逐字段对齐 phase-2a/2b 报告②及后端 handler 序列化 |
| `front/app/features/tags/composables/useSignalWorkbench.ts` | 工作台状态与轮询状态机：周期选择、候选/报告列表（错误态独立）、discovery/research 两个轮询（串行 setTimeout 3s、epoch+boardId 身份守卫、迟到响应丢弃）、报告阅读视图状态、setBoard 切板块隔离。首次直接使用自动绑定板块（bindBoard），此后仅 setBoard 可重绑 |
| `front/app/features/tags/components/SignalCandidateList.vue` | 周期工具栏（月/年+period+发现信号）+ 候选行列表（signal/why_it_matters/research_question/依据可展开/发现时间/派生状态徽标/深入分析或阅读报告/显式重新研究）；score 不渲染；错误态≠空态 |
| `front/app/features/tags/components/SignalReportList.vue` | 按周期报告列表：标题/目标周期/生成时间/真实取数与计算与决策计数（generation_meta，不信模型自报）；`analysis_mode=retrospective` 标「事后回顾 · 本次数据版本」 |
| `front/app/features/tags/components/SignalReportView.vue` | AppPageShell reader≤760 阅读视图：四段（thesis lede 版式/facts/causal/implication+结构化字段 verdict/direction/horizon/trigger_condition/self_doubt）、引用渲染、SVG 图表、代码附录（调用/观测/计算/缺口）、budget_exhausted 缺口提示、显式重新研究入口；全插值安全渲染 |
| `front/app/features/tags/components/signalReport.ts` | **6.T3 可测纯逻辑**：`parseSignalReferenceTokens`（token 切分）、`resolveDataRef`/`resolveCalcRef`（appendix 查表，raw_value 源精度优先、缺失保真绝不转 0、悬空 null）、`buildSegmentViews`/`buildParagraphViews`（正文渲染模型）、`buildSignalChartData`/`splitLineSegments`（图表数据变换，null 断点不补零不连线、来源标 data/calc）、`buildAppendixObsRows`/`buildAppendixCalcRows`（附录行全集）、`candidateStatusLabel`/`directionLabel`/`validateSignalPeriodFormat`/`isRetrospectiveReport`/`findSection` |
| `front/app/features/tags/composables/useSignalWorkbench.test.ts` | 轮询状态机用例（12 个，见④） |

**新增测试（2 个）**：`SignalCandidateList.test.ts`（16 用例，FE-1~5）、`SignalReportView.test.ts`（27 用例，FE-6~10 + 纯函数）。

**增量编辑（3 个，均为受影响测试/组件，未覆盖预存脏改）**

| 文件 | 改动 |
| --- | --- |
| `front/app/features/tags/components/BoardEnrichmentPanel.vue` | **增量**：①第一 section（版块简报主视图）替换为信号解读工作台（SignalCandidateList + SignalReportList ↔ SignalReportView 阅读视图切换）；②卸载旧 brief/investigation/legacy 视图挂载、BoardRelationPanel、版块报告 QAPanel、生成简报按钮、历史下拉、数据源绑定管理（adv 折叠区+编辑弹窗）；③新增 `useSignalWorkbench` 接线（commit/discover/research/openReport/closeReport，重新研究 confirm 预算提示）；④bootstrap 增 `sig.setBoard`/`sig.loadPeriod`（切板块才重置信号视图，刷新不误杀在跑轮询），保留 loadDataSources/loadBoardAnalysisResults/syncBoardAnalysisStatus（旧任务互斥提示仍真实、旧 API 可读）；⑤**新闻背景折叠区、聚焦分析折叠区、FinGenius 辩论、补生成周期弹窗原样保留**（含历史翻阅/叙事内联编辑） |
| `front/app/features/tags/components/BoardEnrichmentPanel.test.ts` | 重写受影响断言：信号主视图挂载、FE-11 旧产出/绑定入口不在 DOM、新闻背景与聚焦分析保留、信号事件接线（含 regenerate confirm 门禁）；bootstrap 顺序契约用例保留 |
| `front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts` | 仅增量：补 `~/api/boardSignals` mock（bootstrap 新增拉取信号列表）；原 2 用例零改动通过 |

**卸载（FE-11）**：`BoardBriefReport`/`BoardInvestigationReport`/`BoardAnalysisReport`/`BoardRelationPanel` 组件文件**保留未删**（有同名测试文件引用，删文件属破坏性动作留 controller 裁决，见⑥），仅从本面板卸载挂载，DOM 中不再出现。

## ② 前端数据流摘要

```
boardSignals.ts（API 唯一边界）
   ↓ useSignalWorkbench（组合函数：状态 + 轮询状态机）
     ├─ discover()：POST signal-discoveries → 202 存 job_id → 串行轮询
     │    running：更新 phase（prepare/detect）
     │    终态：discovered/no_signal → 重拉候选+报告列表（no_signal 安静，不弹消息）
     │           failed → discoveryError（与空态区分，可重试）
     │    404（重启）→ 停轮询 + 提示可重试 + 重拉列表
     ├─ research(candidateId, {regenerate})：POST signals/:id/research
     │    202 → 轮询（phase research/compose）→ succeeded 用 result_id 打开详情
     │                                    → failed → researchError + 重拉（候选回待研究）
     │    409 → 按冲突体 job_kind 接管：signal_report 恢复原 job 轮询 /
     │          signal_discovery 恢复发现轮询 / 旧 brief/investigation 仅提示不接管
     │    200 already_reported → 直接 getSignalReport(result_id)（零新费用）
     │    404（重启）→ 停轮询 + 提示 + 重拉（候选不卡「研究中」——派生自 live job）
     └─ openReport(resultId) → GET signal-reports/:id（错误态不伪装无数据）
   ↓ BoardEnrichmentPanel（事件接线 + confirm 门禁 + enrichment-off 快捷开启）
     ├─ SignalCandidateList（props in / events out，FE-1：emit discover 是全链唯一发现触发点，零 research 出口）
     ├─ SignalReportList → emit open
     └─ SignalReportView（reader 视图）→ emit back / regenerate(candidateId)
```

- FE-1 断言链：组件层（SignalCandidateList 零 research 事件）+ composable 层（discover 全链 `triggerSignalResearch` 恒 0 调用）双层钉住。
- 周期状态在 composable 持有：返回列表/切视图保持周期（FE-9）；同板块刷新按钮不重置任务态。

## ③ 引用/图表渲染实现说明（纯函数位置：`signalReport.ts`）

- **引用渲染**：正文 token `[[data:cN:oM]]`/`[[calc:kN]]`/`[[news:ID]]` 由 `parseSignalReferenceTokens` 切段 → `buildSegmentViews` 从 appendix 查表生成展示值：data=raw_value（源精度）+原生单位，hover title=指标/期间/查询问题；calc=规范十进制串+单位，title=op+expression+精度；news=「新闻依据 #ID」原地展示不跳转（切片内容不在 payload，2b ⑤已备案）。悬空 data/calc 引用 view=null → 渲染「数据引用缺失」占位不崩。点击 data/calc → 展开附录 `<details>` + `appendixAnchorId` 定位行高亮（accent-subtle）。
- **图表**：`buildSignalChartData` 把 refs 解析为数据点（折线按期间升序、null 断点保留；计算点标 origin=calc）→ `chartGeometry` 生成轻量 SVG（折线 y 域不从 0 起、null 经 `splitLineSegments` 切段不连线；对比柱 y 从 0 起）；figcaption 带单位与「数值由代码计算/缺失期间断开显示」来源标记；0 图不渲染图表区块（合法）。
- **附录**：`buildAppendixObsRows`（观测全集含未引用行，不做正文过滤）+ `buildAppendixCalcRows`（op/expression/inputs/精度/status）+ 缺口列表 + 调用账本（retrieved_at/last_modified/sha256）；表格容器 `overflow-x:auto` 局部横滚不撑宽整页。
- **安全渲染**：LLM 文本一律 `{{ }}` 插值 + DOM text，不经 v-html（测试断言 `<img onerror>` 不被解析）。
- **版式**：`.signal-report` 作用域（--font-serif-display 衬线标题、印刷红 kicker/section-sep、3px accent 左线后果段、附录去外框行 hairline），未动 `.markdown-body` 基础排版；双主题走 `var(--color-*)`。

## ④ 测试结果（命令 + 退出码 + 用例数）

| 命令（cwd=front） | 退出码 | 用例 |
| --- | --- | --- |
| `pnpm lint` | 0 | 0 errors（7 warnings 全部位于其他 change 的预存文件，与本批无关） |
| `pnpm exec nuxi typecheck` | 0 | 0 error |
| `pnpm test:unit app/features/tags/components/SignalCandidateList.test.ts app/features/tags/components/SignalReportView.test.ts --maxWorkers=2` | 0 | 16+27=43 passed |
| `pnpm test:unit app/features/tags/components/BoardEnrichmentPanel.test.ts app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts --maxWorkers=2` | 0 | 19+2=21 passed |
| `pnpm test:unit app/features/tags/composables/useSignalWorkbench.test.ts --maxWorkers=1` | 0 | 12 passed（fake timers 驱动轮询：FE-1 零串发/no_signal 安静/failed/job404 停轮询重拉/409 恢复/200 复用/守卫隔离） |

合计 **76 用例全绿**；禁全量 vitest 遵守（树莓派纪律）。

## ⑤ 未做项 / 残余风险

- **6.T4/8.5 人工验收不做**（按 brief）：静态 :5100 全链路走查与 1440×900/1920×1080/375px 双视口验收留给人工/集成阶段。
- **`wrapper.emitted()` 环境级失效（重要知会）**：当前依赖树下 vitest 以非 dev 语义解析 Vue，`wrapper.emitted()` 不记录自定义事件——预存 `FeedDetailEditor.test.ts`（4 用例）因此已红，与本 change 无关，已按纪律登记台账（`test-patrol --register feed-detail-editor-emitted --context board-signal-reports`）。本批新测试改用 **attrs onXxx spy** 断言（handler 直调路径不受影响），后续测试编写建议沿用该模式。
- **VTU 全局 stubs 名称匹配在本环境不可靠**（实测 `stubs: { AppButton: … }` 不生效）：面板测试对信号子组件改用 `vi.mock` 模块级替换；既有 stubs 写法的存量用例未逐一验证，属预存环境问题非本批引入。
- **研究进行中无实时计数**：job 状态仅暴露 phase（research/compose），决策/取数/计算计数只在完成后经 generation_meta 展示——进行中界面显示「研究阶段 · 决策上限 40 轮」，不虚构计数（FE-6 的"真实计数"在完成态满足）。
- **新闻依据 tooltip 仅切片编号**：切片标题/日期不在报告 payload（2b ⑤备案），候选行依据区以「新闻切片 #ID」chip 原地展示。
- **旧组件文件未删**：BoardBriefReport/BoardInvestigationReport/BoardAnalysisReport/BoardRelationPanel 及各自测试仍在树上（无面板引用、测试可独立跑绿）；是否删除留 controller 裁决（删除会连带删 4 个测试文件约 1400 行）。
- **聚焦分析折叠区 + FinGenius 辩论保留**：brief 明确退役清单为「brief/investigation/legacy 视图 + 绑定管理」，聚焦区是新闻背景的泳道选择点故保留；若 controller 认为单泳道分析入口也属「旧产出」应退役，属一处增量删除。

## ⑥ 裁决请求

1. **周期工具栏语义拆分（与原型的偏差备案）**：原型「发现信号」点击兼做周期切换+校验；前端拆为两个动作——周期输入 change 只提交（重拉列表浏览，零 LLM），「发现信号」才触发 discovery（先对齐周期再 POST）。理由：浏览历史周期候选/报告不应被迫触发 LLM；符合「仅手动触发」边界。请知会/追认。
2. **409「已有报告 + 他任务在跑」返回 409 而非 200**（phase-2b ⑥⑤已备案）：前端按现状处理（409 恢复在跑 job 展示，完成后自然进入 succeeded/200 复用路径），无阻塞。
3. **`no_signal` 时 `candidate_count` 键省略**（2a ⑥②）：前端以 `outcome=="no_signal"` 判定，不需要显式 0——无需后端改动。
4. **旧报告组件文件是否删除**：见⑤，请 controller 裁决（不删不影响验收：DOM 断言保证旧入口不在工作台）。
5. **聚焦分析折叠区去留**：见⑤，若需退役请在阶段4/收尾明确，前端一行删除该 section 及其接线。
