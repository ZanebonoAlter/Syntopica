# Review：fix-board-health-magnet-signal

> 审查方式：派 `change-review` 档子线程聚焦审查（制品 + diff），产出 14 条意见；主线程逐条处置并在此建账。
> 审查范围：本 change 的 7 份制品 + 后端 `boardhealth/{signals,metrics,llm,run,boardhealth}.go` + `models/board_health.go` + 4 个后端测试 + 前端 `semanticBoards.ts` / `boardHealthTriage.ts` / `boardHealthWorkspace.ts` + 3 个 `.vue` + 3 个前端测试。
> 未审：工作树中属于其它 change 的改动（`front/` 下大量文件、`backend-go/internal/tagmanagement/service/core/*`、`topicgraph/*`），按任务书要求不审。

### R-1 2026-11-10【High】MODIFIED 漏掉主 spec 既有 Scenario，`openspec validate --strict` 失败
- **状态**：已修复
- **位置**：`openspec/changes/fix-board-health-magnet-signal/specs/board-health-checkup/spec.md`（MODIFIED「标签分诊信号计算」）
- **问题**：主 spec 既有 Scenario「磁铁信号失败子集贴合差成立」在 delta 里被换成新标题，openspec 校验器判 MODIFIED 漏场景，归档会被硬阻断。
- **处置**：delta 内保留原标题「磁铁信号失败子集贴合差成立」，WHEN/THEN 改成新口径数值（failed 0.72 vs good 0.52）。实测 `openspec validate fix-board-health-magnet-signal --strict` → valid。

### R-2 2026-11-10【High】12 条继承 Scenario 未在验证节映射，`scenario-trace.sh` 失败
- **状态**：已修复
- **位置**：`openspec/changes/fix-board-health-magnet-signal/tasks.md` §10 映射表
- **问题**：MODIFIED 块复述了主 spec 既有 Scenario，脚本要求全部映射，缺 12 条。
- **处置**：映射表补齐 39 条（自动测试 39 / 人工留痕 0），实测 `scenario-trace.sh` 退出码 0。

### R-3 2026-11-10【High】delta Scenario 与实现互斥，design D3 的理由建立在事实错误上
- **状态**：已修复
- **位置**：delta spec「核心主体按版块文本判定」；`design.md` D3；`backend-go/internal/tagmanagement/service/boardhealth/metrics.go:computeCoreSubjects`
- **问题**：标题「中东地缘政治与美伊关系」用的是简称「美伊」，**不含「伊朗」**，纯子串匹配必然判 false；而实现自带的回归用例正断言 false —— spec 却写 SHALL 为 true。design D3「标题直接含『伊朗』→ 命中」是事实错误。
- **处置**：① spec Scenario 改为「description 含『伊朗』→ 标记 true」并新增「简称与泛化词不据标题推断为核心主体」；② design D3 理由行改为承认「简称 ≠ 全名、1974 靠 description 兜底」；③ 核心主体规则收紧为「description/aliases 命中即成立；仅标题命中需标签名 ≥ 3 字」（见 R-6）；④ proposal 的「数据补齐」项写明该板核心主体保护**依赖 description 明示「伊朗」**。

### R-4 2026-11-10【Medium】前端「旧口径快照」回退条件过宽，新快照会被误标为旧口径
- **状态**：已修复
- **位置**：`front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts`；`backend-go/internal/tagmanagement/models/board_health.go`
- **问题**：前端只按 `goodMean !== null` 区分新旧口径，但后端 `good_mean` 无 `omitempty` → 新快照在健康侧不可算时同样为 null，前端无法区分「旧快照」与「新快照但不可算」，于是把含失败行的全体均值（旧口径）摆回界面。
- **处置**：后端加 `caliber` 字段（`healthy_subset` / 空 = 旧口径；实测无 `omitempty`，健康侧不可算时也写新口径），前端按标记判定；口径标记的取数与文案见 `design.md` D12、任务 1.6 / 5.1–5.3 / 5.7。实测新快照详情页显示「失败子集 X vs 健康子集 Y」。

### R-5 2026-11-10【Medium】G3 占比取数时点与 spec 不一致，「降级前」注释与实现不符
- **状态**：已修复
- **位置**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go:applyTriageGuards`；`models/board_health.go:BoardHealthTriageGuard`
- **问题**：G3 在 G1/G2 之后统计占比；remove 主导轮次里 G2 先把 47 条降为 insufficient，G3 再看到 actionable=0 → 记录 ratio=0、degraded=false，闸门痕迹被抹掉。
- **处置**：占比改为取 **LLM 原始裁决**（G1/G2 生效之前统计，G3 降级动作仍作用于已被 G1/G2 改过的清单），spec 增「闸门占比取 LLM 原始裁决而非降级后状态」Scenario，注释改为「G3 降级前」；任务 3.3 / 3.6。

### R-6 2026-11-10【Medium】核心主体纯子串匹配会误命中短泛化标签并被 G1 强制 keep
- **状态**：已修复
- **位置**：`backend-go/internal/tagmanagement/service/boardhealth/metrics.go:computeCoreSubjects`
- **问题**：标题含「中东」而构成标签名为「中东」时命中 → G1 强制 keep → 真污染无法被摘除；design 原「零误判」表述不成立。
- **处置**：规则收紧为「仅标题命中需标签名 ≥ 3 字」（description/aliases 命中不受限）；`test-cases.md` §7.2 增 C1–C8 分支表、§4.1 增两条变体；任务 2.1 / 2.4。

### R-7 2026-11-10【Medium】`Config.MagnetGap` 字段注释仍写旧公式
- **状态**：已修复
- **位置**：`backend-go/internal/tagmanagement/service/boardhealth/boardhealth.go`
- **问题**：注释仍写 `all_mean minus failed_mean`，与同包已改的 `signals.go` / `models` 口径相反，是「后人照注释写反」的高危位置。
- **处置**：改为「failed_mean − good_mean（>= 语义）；两侧样本各自达标才判；all_mean ≡ cosine_mean 仅供历史兼容展示」；任务 1.5。

### R-8 2026-11-10【Medium】红线文档仍描述旧口径（tasks 9.1 未做）
- **状态**：已修复
- **位置**：`docs/reference/flow/semantic-board.md`（§体检口径「磁铁失灵」、业务约束红线 24）
- **问题**：红线文档是 constraint-injection 的注入源，继续告诉后续 Agent 旧口径——正是本次事故的根因表述。
- **处置**：归档前完成 tasks 9.1（更新红线 24 + 新增核心主体/闸门红线并顺延编号 + §三病分诊节同步），并跑 `doc-impact.sh verify` + `check-standards.sh` 复核。已改：红线 24 口径反转、红线 32（核心主体保护）、红线 33（分诊合理性闸门）、§三病分诊节磁铁段 + 核心主体标记段。

### R-9 2026-11-10【Medium】design D4 落点引用不存在的符号 `applyCoreSubjectGuard`
- **状态**：已修复
- **位置**：`openspec/changes/fix-board-health-magnet-signal/design.md` D4
- **问题**：该符号零命中（实现把 G1 合进 `applyTriageGuards`），按 design-anchor-tax 记未解析。
- **处置**：D4 落点改为 `backend-go/internal/tagmanagement/service/boardhealth/llm.go:applyTriageGuards`（G1 分支），并注明「不单列函数」。

### R-10 2026-11-10【Medium】design D11 承接制品引用错位
- **状态**：已修复
- **位置**：`openspec/changes/fix-board-health-magnet-signal/design.md` D11
- **问题**：delta 的 ADDED Requirement 里没有 Out-Of-Scope 声明（实际写在 proposal）。
- **处置**：D11 承接制品改为 `proposal.md` 的「### Out of Scope」节。

### R-11 2026-11-10【Medium】工作树污染：已归档 change 的实现未入库且与本 change 同块交错
- **状态**：待处理
- **位置**：`backend-go/internal/tagmanagement/service/boardhealth/{llm.go,run.go,config_test.go,run_test.go}`；`front/app/api/semanticBoards.ts`
- **问题**：已归档 change `fix-board-health-triage-degrade` 的实现（`normalizeTriage`/`minTriageCoverage`、新鲜度守卫、单板超时、每板一行轮次日志）**从未 commit**，其归档目录也是未跟踪状态；本 change 若直接提交，会把它的实现以本 change 名义入库，后续无法按 change 归因/回滚。另 `semanticBoards.ts` 夹带其它会话未提交的 boardRadar 类型导出。
- **处置**：提交时先给 `fix-board-health-triage-degrade` 补一次提交（实现 + 归档制品），再切分本 change 的提交；夹带的 boardRadar 导出行从本 change 提交中剔除或显式留痕。任务 6.12。

### R-12 2026-11-10【Low】tasks 未勾选、真库效果核对未闭环
- **状态**：已修复
- **问题**：实现与测试已就位且全绿，但 tasks 一项未勾；tasks 7.1–7.4（补 description / 重跑 / 只读核对 / 人工核对）未做，`test-cases.md` §5 仍「待填」。
- **处置**：随实施进度勾选；重跑后回填 §5 量化结果与结论。任务 6.13、7.1–7.4。
- **实测结果**：tasks 已逐项勾选（仅提交归属 6.12、归档后溯源 9.4 留待对应时点）；`test-cases.md` §5 已回填真库量化（1974：47 条全 keep / 可执行 0 / 板分 58→88 / 伊朗 core_subject=true + suspicious=false）。

### R-13 2026-11-10【Low】旧快照的 magnet verdict 文案不区分新旧口径
- **状态**：已修复
- **位置**：`front/app/features/tags/components/console/board-health/BoardHealthIssueDetail.vue`
- **问题**：旧快照 `suspicious` 是旧公式算出来的（语义与新口径相反），verdict 文案仍显示「差值显著，疑似磁铁污染」，只靠对比行尾部的弱提示。
- **处置**：旧快照 verdict 带口径标记（「旧口径快照的磁铁结论（口径已反转，勿据此操作）」）；任务 5.5 / 5.7。派生层新增 `boardHealthMagnetVerdictText`，组件不再自带口径分支。

### R-14 2026-11-10【Low】新口径下「建议摘除」可能事实上不可达
- **状态**：已修复
- **问题**：新口径要求 `failed_mean − good_mean ≥ 0.08`，而真实板上 failed 侧样本少且与本板方向不符，`failed_mean` 普遍低于 `good_mean` → `suspicious` 极少成立，G2 会把几乎所有 remove 降为 insufficient。
- **处置**：在 tasks 7.3 的只读核对里量化「本轮 `suspicious=true` 的标签数」，如实回填 `test-cases.md` §5；若为 0，在 design 里补注「无成立磁铁信号时 remove 通道需人工确认路径」，不把「G2 全拦」说成「体检更准了」。任务 6.15、7.3。
- **实测结果**：2026-11-10 重跑后 13 个板的 `suspicious=true` 标签数 **全部为 0** —— 「🔴 建议摘除」事实上不可达，已在 `design.md` D5 加注记（含恢复摘除通道的后续方向）；真库量化已回填 `test-cases.md` §5。

---

## 审查方核实记录（子线程实测）

| 检查 | 结果 |
| --- | --- |
| `go test -count=1 ./internal/tagmanagement/service/boardhealth/...` | ok |
| `go vet ./internal/tagmanagement/service/boardhealth/...` | 无输出 |
| 前端定向测试（normalizer + board-health 4 文件） | 100 用例全绿 |
| `grep "AllMean\|all_mean" backend-go/internal/tagmanagement/` | 判定侧零引用（all 侧确已退出判定） |
| `grep "Suspicious\|MagnetGap" .../boardhealth/*.go`（非测试） | 判定方向唯一：`signals.go` 的 `failed_mean − good_mean ≥ gap` + `llm.go` G2 只读 `Suspicious` |
| `grep "applyTriageGuards\|normalizeTriage" backend-go/` | 闸门唯一调用点在 `normalizeTriage` 之后、`db.Create` 之前 |
| `grep "collectBoardStatistics" backend-go/` | 2 处调用方均已改签名，无漏改 |
| `grep "v-html" front/.../board-health/` | 零命中（模板插值自动转义，无 XSS 风险） |
| 纯读红线核对 | 闸门只改内存中的类别与证据，落库仍只有 `board_health_snapshots`（+ 冷却内通知） |
| 配置非法值 | `parseRatioDefault` 拒绝 ≤0 / >1 / NaN / Inf → 回退 0.8 |
