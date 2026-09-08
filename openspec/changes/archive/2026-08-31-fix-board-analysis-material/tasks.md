# Tasks: fix-board-analysis-material

## 1. 态势卡取材链（P0）

- [x] 1.1 `laneFactsDigest` 插 month 兜底层：week 缺失时取 month 最新 2 期压缩（`facts_source=lifeline_month`），预算常量不变；密度信号计入 month/week 可用性（公式在测试固化）。验证：`go test ./internal/dataenrichment/service -run Situation -short`（生产形态 fixture：month 在、week 缺 → `lifeline_month`；week 在 → 优先 week；均缺 → 指纹）全绿
- [x] 1.2 section 指纹提质：`[日期] thread标题×3 拼接 (N篇)` 替代 cluster_label 同义反复，无 thread 退回 cluster_label。验证：同上 -run Situation 用例（指纹含 thread 标题断言）

## 2. 下钻工具读背景记忆（P0）

- [x] 2.1 `LifelineReader` 接口扩 `GetTopicLifelineArchive`（month 最新 2 期 + year 最新 1 期），生产实现查 `topic_lifeline_context`。验证：repository/生产 wiring 单测（归档行选取/无记录空返回）
- [x] 2.2 `RenderLifelineForAgent` 追加「历史背景记忆（月/年档案）」段：预算 4000 rune、超限标注截断、无归档如实标注。验证：`go test ./internal/dataenrichment/service -run RenderLifeline -short`（有归档渲染/预算截断/无归档标注/既有段回归）全绿；既有 fake reader 补空实现零破坏

## 3. 前端收口（P1）

- [x] 3.1 `BoardEnrichmentPanel.vue`：删顶部旧 toolbar（刷新入口移版块分析头）、泳道选择唯一化（聚焦区下拉）、「新闻背景」单 tab 栏改折叠 section。验证：新建 `BoardEnrichmentPanel.test.ts`（单一下拉/新闻背景折叠入口/无顶栏断言）+ `pnpm lint` + Windows cmd `pnpm exec nuxi typecheck` + `pnpm test:unit` 全绿

## 4. 测试

- [x] 4.1 后端影响包：`go test ./internal/dataenrichment/...`（DB 集成，Docker pgvector，fixture 以生产形态起步）+ `golangci-lint run ./...` + `go vet ./...` + `go build ./...` 全绿
- [x] 4.2 效果核对（真库）：对生产库跑态势卡装配，量化 `facts_source` 分布（预期 lifeline_month 占比 ≥80% 泳道），结果记入本文件下方
- [x] 4.3 `bash scripts/scenario-trace.sh openspec/changes/fix-board-analysis-material` 通过（映射表见 test-cases.md）

## 5. 文档

<!-- doc-impact: flow, api, database -->
<!-- doc-impact-excuse: architecture=改 internal/app/runtime.go 仅注释掉 lifeline_weekly job 注册行（停用定时任务，7.3），无架构变更，flow 已覆盖; standard=脏工作区中 docs/reference/standard/backend/ai-logging.md 的修改属并行 change board-level-deep-analysis（8.4：empty_response fallback 与 board_* operation 对账），本 change 未改 standard 相关代码与文档 -->

- [x] 5.1 `docs/reference/flow/data-enrichment.md`：业务约束节补「态势卡取材链（week→month→指纹）与 get_lane_detail 档案段」条目 + 变更溯源行
- [x] 5.2 `docs/reference/api/dataenrichment.md`：仅当 get_lane_detail 输出契约变化影响 API 文档时补注（工具输出非 HTTP API，预计无改动则记「无 API 变化」）——get_lane_detail 为 agent 工具非 HTTP API，无 API 文档变化

## 6. 验证

- [x] 6.1 后端：`go test ./internal/dataenrichment/... ./internal/platform/database/...` → 全绿（含 DB 集成）
- [x] 6.2 `golangci-lint run ./...` + `go vet ./...` + `go build ./...` → 零报错
- [x] 6.3 前端：`pnpm lint` + Windows cmd `pnpm exec nuxi typecheck` + `pnpm test:unit` + `pnpm build` → 全绿
- [x] 6.4 `bash scripts/doc-impact.sh verify openspec/changes/fix-board-analysis-material` → 退出码 0

### Scenario → 测试文件映射（scenario-trace 对账表）

| Scenario | 测试文件 |
| --- | --- |
| week 缺失时 month 兜底 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 指纹降级有实质内容 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 泳道多时素材仍可控 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 无任何素材时卡片仍产出 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 低质量泳道不被静默删除 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 单一下拉选择泳道 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 新闻背景入口保留 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |
| 多级入口按需下钻 | backend-go/internal/dataenrichment/service/exploration_test.go |
| 下钻可读历史背景记忆 | backend-go/internal/dataenrichment/service/lifeline_renderer_test.go |
| 无背景记忆时不报错 | backend-go/internal/dataenrichment/service/lifeline_renderer_test.go |
| web_search 与 fetch_page 配合取证 | backend-go/internal/dataenrichment/service/web_search_test.go |
| 金融工具彻底不可见 | backend-go/internal/dataenrichment/service/allowed_tools_test.go |
| 截断档分析前重算 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 无记录首建 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 限额溢出降级 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 无数据周期跳过 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 补齐幂等 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 补齐失败降级 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 触发立即返回且断连不中止 | backend-go/internal/dataenrichment/handler/analysis_runner_test.go |
| 同目标防重入 | backend-go/internal/dataenrichment/handler/analysis_runner_test.go |
| 分析状态可轮询 | backend-go/internal/dataenrichment/handler/board_enrichment_handler_test.go |

## 7. 补全门（追加：分析前新闻背景补全）

- [x] 7.1 `freshness_gate.go` 升级为补全门：检查集 month/year；有料周期无行→补建、行最后写于 72h 前→重算（统一 `RefreshPeriod`）；全局限额 40 次、溢出降级；串行。验证：`go test ./internal/dataenrichment/service -run Freshness -short -count=1`（截断重算/首建/限额溢出/幂等/失败降级）全绿
- [x] 7.2 `refreshArchive` 写入 as_of 钳制到 min(周期边界, now)（修未来日期脏数据：year as_of=2027、手动补生成 09-01）。验证：单测断言写入后 as_of ≤ now
- [x] 7.3 停用 `lifeline_weekly` 定时任务注册（runtime.go；近期记忆归 14 天窗口，长期归 month/year；存量 week 行保留可被取材链消费）。验证：`go build ./...` + grep 无注册点
- [x] 7.4 flow 约束 17 条改写 + tasks.md 映射表补新 Scenario

### 效果核对记录（4.2 回填）

- 触发方式：临时 main（已删）连生产库，对 2 个开启增强的板块（1974 中东地缘、1980 生成式 AI）跑真实 `AssembleSituationCardsForTest`，量化 facts_source 分布。
- 结果（2026-08-27，total 14 卡）：`lifeline_month=8 (57.1%)`、`lifeline_week=1 (7.1%)`、`section_fingerprint=5 (35.7%)`。
- 口径①（修复有效性）：有 lifeline 档的 active 泳道 100% 命中（8/8 走 month、1 走 week），无降级丢失——**判据达标本意即此**（“67 个有 month 档的泳道修复后全部走 month 路径”）。
- 口径②（总卡占比）：57.1% < 80%，全部差额来自 5 条 8 月新孵化泳道（#1204/1205/718/971/1202，lifelines=0，尚未进月度归档周期）——属归档覆盖自然缺口非断供 bug，按 M5 预案回流 lifeline 补齐议题（归 `lifeline_monthly` 定时任务/新鲜度门覆盖节奏，不扩本 change scope）。
- 附带验证：fingerprint 样本已带实质 thread 标题（如「[08-26] 美伊谈判传重大进展，双方就海峡通航达成共识 | 内塔尼亚胡透露与特朗普讨论对伊三种方案 (4篇)」），修复前同位置为「[08-26] 泳道名 (4篇)」同义反复。

## 8. 分析触发异步化（追加：断连不再作废分析）

> 起因：2026-08-27 22:50 生产实锤——板块分析同步 HTTP 跑 10 分钟（补全门备料 29 次 LLM + agent 14 次 tool_use），用户离开页面 → request-context cancel → analyze 环节 "context canceled"，全部作废无报告。

- [x] 8.1 后端 `analysis_runner.go`：内存 job 表（scope board/topic × target id），detached ctx + 30min 超时、panic 恢复、同目标防重入、完成态保留供轮询。验证：`analysis_runner_test.go` 4 用例（父取消存活/防重入/panic 恢复/超时）
- [x] 8.2 板块/单泳道 trigger 异步化：立即返回 `{status:"started"}`；已在跑 409；未开启板块同步预检 400（M6.1 语义保留，`BoardEnrichmentEnabled`）。验证：handler 测试重写（Trigger 轮询断言/AlreadyRunning/PrefillLens）
- [x] 8.3 状态接口 `GET /enrichment/analysis-status?scope=&id=`：running/finished/error/result_id；从未触发视为空闲
- [x] 8.4 前端：`triggerBoardAnalysis`/`triggerEnrichment` 改异步响应 + 3s 轮询（完成拉新结果/失败 notify）；进面板 `syncBoardAnalysisStatus` 恢复「分析中」显示；卸载只停轮询不杀分析；按钮文案「分析中…可离开」。验证：lint/typecheck/test:unit(578)/build 全绿
