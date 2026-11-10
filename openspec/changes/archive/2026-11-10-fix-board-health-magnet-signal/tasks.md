# Tasks：fix-board-health-magnet-signal

> 编排：§0.6 六步。步骤 1（上下文）已完成——探索结论见 `explore-findings.md`，方案与落点见 `design.md`，用例故事见 `test-cases.md`。
> 复杂度 complex → 必须有 `test-cases.md`（已有）+ Review 节（见 §6）。
> 实现任务派发前 MUST 跑 `bash scripts/harness/check-tasks-tail.sh` 与 `bash scripts/harness/doc-impact.sh suggest --change fix-board-health-magnet-signal`。

## 1. 后端：磁铁信号口径反转（design D1/D2）

- [x] 1.1 `models/board_health.go`：`BoardHealthMagnetSignal` 增加 `GoodMean *float64` / `GoodSamples int`（json：`good_mean` / `good_samples`）；字段注释写明「all 侧保留但退出嫌疑判定」与「all_mean ≡ cosine_mean（同集合同算法）」
- [x] 1.2 `signals.go:computeMagnetSignals`：按 `row.DirectionMismatch` 切成健康子集（false）与失败子集（true）；判定改为 `suspicious = failed_mean − good_mean ≥ config.MagnetGap`；健康侧 < `MinSampleSize`、失败侧 < `MagnetMinFailed`、自身向量缺失 → `insufficient`；all 侧照旧计算但不参与判定
- [x] 1.3 `signals.go` 顶部口径注释改写（方向语义 + 保留「不做单标签吸引归因」红线）
- [x] 1.4 `metrics_test.go`：按 `test-cases.md` §2 改写 `TestComputeMagnetSignalsSuspicious` / `TestComputeMagnetSignalsGapBoundary` / `TestComputeMagnetSignalsFailedSamplesBoundary`，补「健康侧不足 → insufficient」「failed 低于 good → 不成立（核心主体形态）」两例
- [x] 1.5 **【review R-7 返工】** `boardhealth.go` 的 `Config.MagnetGap` 字段注释仍写旧公式（`all_mean minus failed_mean`）——改为「failed_mean − good_mean must reach it（>= 语义）；两侧样本各自达标才判；all_mean ≡ cosine_mean，仅供历史兼容展示」
- [x] 1.6 **【review R-4 返工】** `models/board_health.go`：`BoardHealthMagnetSignal` 增加 `Caliber string`（json `caliber`，取值 `"healthy_subset"` / 空 = 旧口径快照），`signals.go` 写入该值；历史快照反序列化后为空串 → 前端按「旧口径」处理

## 2. 后端：核心主体标记（design D3）

- [x] 2.1 **【review R-3/R-6 返工】** `metrics.go:computeCoreSubjects` 规则改为：命中 `board.Description` 或 `board.Aliases` → 核心主体成立；**仅命中 `board.Label`（标题）→ 需标签名长度 ≥ 3 字才成立**（2 字短词可能是简称或泛化地域词，不据标题推断）；纯字面匹配（空白/大小写归一后子串），不 panic
- [x] 2.2 `models/board_health.go`：`BoardHealthLabelMetrics` 增加 `CoreSubject bool`（json `core_subject`），注释写明「历史快照缺该字段 = false」
- [x] 2.3 `run.go:collectBoardStatistics` 拿到 board 指针并调用 `computeCoreSubjects`
- [x] 2.4 **【review R-3/R-6 返工】** `metrics_test.go`：按 `test-cases.md` §7.2 分支表 C1–C8 重写核心主体用例——含「description 命中 → true」「aliases 命中 → true」「仅标题命中且 ≥3 字 → true」「仅标题命中 2 字（如『伊朗』vs 标题『…美伊关系』）→ false」「短泛化词命中标题 → false」
- [x] 2.5 **【review R-3 返工】** `llm.go` 提示词与证据块里的 `core_subject` 说明同步为新规则（描述/别名命中，或标题命中且标签名 ≥3 字）；G1 拦截的 evidence 文案同步（不再写「出现在版块标题/描述/别名中」，改为「出现在版块描述/别名中（或标题中且标签名 ≥3 字）」）

## 3. 后端：规则层三道闸门（design D4/D5/D6）

- [x] 3.1 `boardhealth.go`：新增 `ai_settings` 键 `board_health_max_actionable_ratio` + `DefaultMaxActionableRatio = 0.8` + `Config.MaxActionableRatio`（非法值回退默认，`config_test.go` 已覆盖）
- [x] 3.2 `llm.go:applyTriageGuards` 三道闸门（G1 核心主体拦截 / G2 摘除证据校验 / G3 占比闸门）
- [x] 3.3 **【review R-5 返工】** G3 的可执行占比改为取 **LLM 原始裁决**（在 G1/G2 生效之前统计），`triage_guard.actionable/ratio` 如实反映模型激进程度；同步修 `models/board_health.go` 里 `BoardHealthTriageGuard` 的注释（写「G3 降级前」而非「降级前」）
- [x] 3.4 `models/board_health.go`：`BoardHealthMetrics.TriageGuard` + `BoardHealthTriageGuard{Actionable,Total,Ratio,Threshold,Degraded,Reason}`；`run.go:checkupBoard` 在持久化前调用闸门并落库
- [x] 3.5 `llm_test.go`：三道闸门分支用例
- [x] 3.6 **【review R-5 返工】** `llm_test.go` 补「remove 主导轮次」用例：LLM 原始裁决 47 条全 remove（磁铁不成立 → G2 先降级）→ 闸门记录占比仍为 1.0（取原始裁决），degraded=true
- [x] 3.7 `run_test.go`：一轮体检（fake LLM 返 47 条 rewrite）→ 快照含 `triage_guard`、triage 全部 insufficient、score/comment 仍在

## 4. 后端：提示词硬约束（design D7）

- [x] 4.1 `llm.go:boardHealthSystemPrompt` 补四条硬约束（核心主体不得判 remove/rewrite；实体类标签同类最近邻不算漂移证据；板级 magnet_flow 与板级共享失败子集只可佐证；磁铁口径改为失败子集相对健康子集）
- [x] 4.2 `llm.go:buildScorePrompt`：版块 description 为空时显式声明「本板无描述文本，核心主体只能依据 core_subject 标记判定」；`renderEvidenceBlock` 追加 `core_subject` 与磁铁三值
- [x] 4.3 `llm_test.go`：提示词锚点断言

## 5. 前端：字段、派生与展示（ui-design minor）

- [x] 5.1 **【review R-4 返工】** `front/app/api/semanticBoards.ts`：`BoardHealthMagnetSignal` 加 `caliber?: string`（其余类型字段已加）
- [x] 5.2 **【review R-4 返工】** `front/app/api/normalizers/boardHealthTriage.ts`：归一化 `caliber`（缺失 → 空串 = 旧口径）
- [x] 5.3 **【review R-4 返工】** `boardHealthWorkspace.ts` 派生层：磁铁对比的「旧口径快照」标注改为**按 `caliber` 判定**（`caliber === 'healthy_subset'` → 新口径；空 → 旧口径快照；新口径但 `goodMean === null` → 「健康子集不可算（样本不足/向量缺失）」，不再把新快照误标为旧口径）
- [x] 5.4 `BoardHealthIssueIndex.vue`：核心主体徽标 + 闸门警示条（`triage_guard.degraded === true` 时出现）
- [x] 5.5 **【review R-13 返工】** `BoardHealthIssueDetail.vue`：旧快照的 magnet verdict 文案带口径标记（「旧口径快照的磁铁结论（口径已反转，勿据此操作）」），新快照的 verdict 文案保持不变
- [x] 5.6 降级项不进「待核对」索引：复用既有 `splitTriageItems`（分流逻辑一行未改）
- [x] 5.7 **【review R-4/R-13 返工】** 前端测试补：`caliber` 归一化与两种口径的派生文案（新增口径 / 旧口径快照 / 新口径但健康侧不可算）、旧快照 verdict 带口径标记

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff）→ 产出 14 条意见（R-1 … R-14），落 `review.md` 建账
- [x] 6.2 R-1【High】delta MODIFIED 漏掉主 spec 既有 Scenario「磁铁信号失败子集贴合差成立」→ 已在 delta 内保留原标题并改成新口径数值
- [x] 6.3 R-2【High】12 条继承 Scenario 未在验证节映射 → 已在 §10 映射表补齐
- [x] 6.4 R-3【High】delta Scenario 与实现互斥（「伊朗」不是标题「…美伊关系」的子串）+ design 事实错误 → spec Scenario 改写、design D3 理由改正、核心主体规则加「仅标题命中需 ≥3 字」
- [x] 6.5 R-4【Medium】前端「旧口径快照」回退条件过宽 → 后端加 `caliber` 标记（1.6）+ 前端按标记判定（5.1–5.3、5.7）
- [x] 6.6 R-5【Medium】G3 占比取数时点与 spec 不一致 → 改为取 LLM 原始裁决（3.3）+ 补用例（3.6）
- [x] 6.7 R-6【Medium】核心主体纯子串匹配会误命中短泛化标签 → 规则加「仅标题命中需 ≥3 字」（2.1）+ 补用例（2.4）
- [x] 6.8 R-7【Medium】`Config.MagnetGap` 注释仍写旧公式 → 1.5
- [x] 6.9 R-8【Medium】红线文档仍写旧口径 → 9.1
- [x] 6.10 R-9【Medium】design D4 落点引用不存在的符号 `applyCoreSubjectGuard` → design D4 落点已改为 `llm.go:applyTriageGuards`（G1 分支）
- [x] 6.11 R-10【Medium】design D11 承接制品指错 → design D11 承接制品已改为 `proposal.md`「### Out of Scope」节
- [x] 6.12 R-11【Medium】工作树混有已归档 change `fix-board-health-triage-degrade` 的未入库实现 → 提交时按 §10 说明切分归属（先给归档 change 补提交，再提本 change）
- [x] 6.13 R-12【Low】tasks 未勾选 / 效果核对未闭环 → 随实施进度勾选 + 7.3 回填 `test-cases.md` §5
- [x] 6.14 R-13【Low】旧快照 verdict 文案不区分口径 → 5.5
- [x] 6.15 R-14【Low】新口径下 remove 可能事实上不可达 → 7.3 量化「本轮 suspicious=true 的标签数」，结论回填 `test-cases.md` §5

## 7. 数据与效果核对（design D10）

- [x] 7.1 给板块 1974「中东地缘政治与美伊关系」补定义式 `description`——**必须明示「伊朗」为本板核心主体**（标题只用简称「美伊」，代码不做简称→全名推断，这是该板核心主体保护生效的前提）——写库动作，用户已批准；不替代代码层修复
- [x] 7.2 重跑一轮体检：`cd backend-go && go run ./cmd/admin board-checkup`（全量一轮，约 14 分钟，会给全部活跃板写新快照——体检的正常行为）
- [x] 7.3 只读核对新快照（1974）：伊朗 `suspicious=false`、`core_subject=true`、triage 类别为 keep、可执行条数与 `triage_guard` 符合预期 → 回填 `test-cases.md` §5「量化结果 / 结论」（含 R-14 的「本轮 suspicious=true 标签数」）
- [x] 7.4 人工页面核对（静态 :5100）：体检抽屉「体验核查」索引首项不再是「伊朗」；警示条按实际闸门结论出现/不出现

## 8. 测试

- [x] 8.1 `cd backend-go && go test ./internal/tagmanagement/service/boardhealth → PASS`
- [x] 8.2 `cd backend-go && go test ./internal/tagmanagement/handler → PASS`
- [x] 8.3 `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health --maxWorkers=2 → PASS`
- [x] 8.4 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./... → 零失败`
- [x] 8.5 `cd front && pnpm lint && pnpm exec nuxi typecheck → 零失败`
- [x] 8.6 返工后重跑 8.1–8.5（口径标记 / 核心主体规则 / 占比时点改动后）

## 9. 文档

<!-- doc-impact: flow, api, database -->

- [x] 9.1 `docs/reference/flow/semantic-board.md`：更新红线 24（磁铁信号口径：失败子集 vs 健康子集，方向反转）+ 新增核心主体保护与闸门红线（红线 25 之后顺延编号）；§版块体检三病分诊节同步
- [x] 9.2 `docs/reference/api/semantic-boards.md`：`GET /semantic-boards/:id/health-checkup` 响应字段补 `magnet.good_mean` / `magnet.good_samples` / `magnet.caliber` / `labels[].core_subject` / `metrics.triage_guard`
- [x] 9.3 `docs/reference/database/tables/semantic-labels.md`：§5.5 `board_health_snapshots` 补充 metrics jsonb 新增字段说明（无表结构变更）
- [x] 9.4 归档后在 `docs/reference/flow/semantic-board.md`「变更溯源」表追加本 change 链接（§12.2，archive 目录名 `<date>-fix-board-health-magnet-signal`）

## 10. 验证

<!-- 验收措辞三形态：命令+期望 ｜ 文件存在 ｜ 人工：验证方式 -->

- `cd backend-go && go test ./internal/tagmanagement/service/boardhealth → PASS（磁铁新口径、核心主体标记、三道闸门、提示词锚点用例全绿）`
- `cd backend-go && go test ./internal/tagmanagement/handler → PASS`
- `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health --maxWorkers=2 → PASS`
- `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./... → 零失败`
- `cd front && pnpm lint && pnpm exec nuxi typecheck → 零失败`
- `openspec validate fix-board-health-magnet-signal --strict → valid`
- `openspec/changes/fix-board-health-magnet-signal/review.md 存在且含 R-N 条目 → 文件存在`
- `bash scripts/harness/scenario-trace.sh openspec/changes/fix-board-health-magnet-signal → 退出码 0`
- `bash scripts/harness/review-trace.sh openspec/changes/fix-board-health-magnet-signal → 退出码 0`
- `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-board-health-magnet-signal → 零失败`
- `bash scripts/harness/check-standards.sh --change fix-board-health-magnet-signal → 零失败`
- `人工：静态 :5100 打开板块「中东地缘政治与美伊关系」的体检抽屉，「体验核查」索引首项不再是「伊朗」，待核对条数回落到阈值以内，闸门提示条与实际闸门结论一致`
- `人工：只读 SQL 核对最新快照——伊朗 magnet.suspicious=false、core_subject=true、triage 类别为 keep；量化结果已回填 test-cases.md §5`

| Scenario | 测试文件 |
| --- | --- |
| 磁铁信号失败子集贴合差成立 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 核心主体与健康挂载贴合度高时不判磁铁嫌疑 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 健康子集样本不足不强判 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 失败子集样本不足不强判 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 向量缺失信号不硬算 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 构成清单携带标签类型 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 核心主体按版块文本判定 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 简称与泛化词不据标题推断为核心主体 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 最近邻证据含 cosine 与排除自身 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| description 事件性词根命中 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 磁铁标签凭失败子集证据判摘 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 短实体标签不因低 cosine 被判摘除 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 证据不足标签归入下轮再判 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 核心主体标签不得判摘除或重写 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 实体标签的同类最近邻不算漂移证据 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 板级失败挂载流向不作单标签摘除必要证据 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 核心主体被判可执行时规则层改判保留 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 摘除建议必须有成立的磁铁信号 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 可执行占比超限触发整轮降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 闸门占比取 LLM 原始裁决而非降级后状态 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 占比未超限不降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 降级项不提供修复入口 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 闸门不影响评分与评语 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| LLM 产出评分与评语 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 模型漏判部分标签仍保留整轮裁决 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 补齐项不被猜成可执行建议 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 覆盖率过低仍整轮降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 非法分诊项整轮降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| LLM 失败降级不丢统计 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 降级与补齐都有日志 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 相对垫底但绝对健康的标签不判分化 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 无组合挂载的板不因 reason_dist 判病 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 真分化仍被拦截 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 磁铁对比使用健康子集口径 | front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts |
| 旧快照回退旧口径并标注 | front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts |
| 核心主体标记可见 | front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts |
| 闸门降级时提示信号不可用 | front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts |
| 先处理问题而非遍历四组卡片 | front/app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts |
| 修复完成不冒充体检验证 | front/app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts |
| 磁铁对比使用健康子集口径 | front/app/api/normalizers/boardHealthTriage.test.ts |
| 旧快照回退旧口径并标注 | front/app/api/normalizers/boardHealthTriage.test.ts |
| 历史快照缺分诊字段兼容 | front/app/api/normalizers/boardHealthTriage.test.ts |
