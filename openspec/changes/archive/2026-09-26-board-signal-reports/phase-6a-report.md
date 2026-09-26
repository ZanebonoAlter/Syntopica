# 阶段 6a 定向修复报告（review-final M1/M2/L1/L2/L3）

> 会话：阶段6a「定向修复」唯一写线程，2026-09-22。依据 `review-final.md` E 节 + controller 裁决修复清单执行，**未扩大范围、未顺手重构**。全部改动落在本 change 声明的未跟踪新文件内（signal 链 7 个文件 + doc），未触碰任何预存脏改与 L4/L5。

## ① 每项修复的 file:line 与改动摘要

### M1（WDI 期间键）— `backend-go/internal/dataenrichment/service/signal_research.go`

- `signal_research.go:375`（`signalCalcObservationFromMap` wb_wdi 分支）：新增 `entry.Period, _ = obs["year"].(string)`，覆盖函数入口处统一读 `obs["period"]` 的默认值（对照 `datasources/sources/wdi.go` `WDIObservation`，JSON 键为 `year`、无 `period`）。修复前 WDI 观测 Period 恒空串：同系列不同年 difference 被「同一期间」误拒、多年 mean 在第二个输入触发「重复期间 ""」误拒、折线期间排序退化。
- 修复范围仅此一处；cutoff 过滤层（`signalObservationPeriodEnd` 本就读 `year`）不受影响，未改动。

### M2（工具日志 Outcome，controller 选定方案①）— `signal_research.go`

- `signal_research.go:404-426`：新增 `normalizeSignalErrorFrame(result string) string`——检测到顶层 `error_code` 键且无 `error` 键时，附加 `"error"` 字符串键（取 `message`，空则回退 `error_code`）；非错误帧、已带 `error` 键的非类型化错误、非法 JSON 均原样返回。
- `signal_research.go:615`（`buildSignalResearchRegistry` 的 Execute 包装）：`return normalizeSignalErrorFrame(filtered), err`——归一发生在 `ledger.completeCall` 之后、返回给 agent 之前。账本 status/gap 判定走 `signalSourceErrorText`（本就识别 `error_code` 形状），字节不变；归一只影响 agent 历史（共享循环 `toolResultErrorText` 据此把 `result.tool_calls` 的 Outcome 记为 `error`）。**orchestrator.go 共享路径零改动**（方案①要点），`mergeRawResponsesIntoRecords` 写回的 ResultFull 仍为完整原始响应（OB-2 双侧可重建不受影响）。

### L1（背景摘要粒度过滤）— `backend-go/internal/dataenrichment/service/signal_material.go`

- `signal_material.go:208`（`selectSignalBackgroundSummary`）：跳过条件补 `r.Granularity != granularity`，与 `selectSignalPeriodSummary:192` 的写法对齐。混粒度归档下 year 行不再可能充当 month 周期背景。

### L2（doc 顺序）— `docs/reference/api/board-signals.md`

- POST /signals/:candidateId/research 状态表：拆开原合并的「body 非法 400；板块未开启 400」为两行，按 `handler/signal_research.go:triggerSignalResearch` 实现真实顺序排列：`400`（body 非法 JSON）→ `404`（候选不存在/跨板块）→ `400`（板块未开启 `enrichment_enabled`）→ `409`（先于 200 复用判断）→ `200`（幂等复用）→ `202`（新 job）。已逐条对照 handler 源码确认一致。

### L3（重复解析）— `backend-go/internal/dataenrichment/handler/signal_discovery.go`

- `signal_discovery.go:170-177`（`listSignalCandidates`）：before_id/limit 内联解析（约 18 行）替换为 `board_enrichment_handler.go:274 parseBeforeID` / `:289 parseListLimit` 共享 helper。行为等价：空缺省（beforeID=0 / limit=default）、非法 0/负/garbage → 400 相同错误消息（"invalid before_id"/"invalid limit"）、超上限 clamp；顺带移除不再使用的 `strconv` import。

## ② 新增测试清单

| 测试 | 文件:line | 覆盖 |
| --- | --- | --- |
| `TestSignalCalc_WDIDifferenceAcrossYears` | `service/signal_calculation_test.go:243` | M1①：WDI 同系列 2023/2024 两年 difference 成功（值 1.4、unit 空——WDI 原生无单位列） |
| `TestSignalCalc_WDIMeanAcrossYears` | `service/signal_calculation_test.go:257` | M1②：WDI 三年 mean 成功（42.5；修复前第二个输入即触发「重复期间空串」拒绝） |
| `TestSignalCalc_WDIMixedWithEIARejected` | `service/signal_calculation_test.go:270` | M1③：WDI 观测混入 EIA 系列的 difference/percent_change/mean 三算子均跨源拒绝；并直接断言 `signalCalcObservationFromMap` 产出的 Period/SeriesID/Tool |
| `TestSelectSignalBackgroundSummaryFiltersGranularity` | `service/signal_material_test.go:185` | L1：混粒度 fixture——month 背景不被 year"2025"（字符串序更小）行命中；year 周期只看 year 归档；同粒度无候选 → nil 不拿异粒度凑数 |
| `TestSignalResearch_SourceFailureRecordsGapAndComposes`（扩展断言） | `service/signal_research_test.go:494-499` | M2：stub 源返回类型化错误帧（`error_code`）→ 落库 `tool_calls` 记录 `Outcome=error`（原仅断言附录 status，未覆盖记录 Outcome，即审查指出的暴露缺口） |

辅助 fixture：`wdiObs`/`wdiIndex`（`signal_calculation_test.go:215-241`）经 `signalCalcObservationFromMap` 构造索引（真实覆盖 year 期间键读取路径，非手工 `calcObs` 绕过；value 与 JSON 反序列化后同型 `float64`）。

## ③ 验证结果（命令 + 退出码）

| 命令（cwd=backend-go） | 结果 |
| --- | --- |
| `go test ./internal/dataenrichment/service/ -short -count=1 -run "Signal"` | ok 0.075s（exit 0，新增 4 用例 + 全部既有 Signal 用例回归通过） |
| `go test ./internal/dataenrichment/handler/ -short -count=1 -run "Signal"` | ok 1.106s（exit 0，L3 行为等价回归通过） |
| `golangci-lint run ./...` | 0 issues（exit 0） |
| `go vet ./...` | 无输出（exit 0） |
| `go build ./...` | 成功（exit 0） |

L2 为 doc 改动无需跑测试；已确认 doc 表顺序与 `triggerSignalResearch` 判定顺序逐条一致。

## ④ L4/L5 按 controller 裁决不改的确认记录

- **L4（候选列表派生状态 N+1，每行 2 次查询）**：未改动。单实例单用户产品可接受，batch 化（`source_signal_id IN (...)`）留待列表实际变慢时再做（review E 节原文口径）。
- **L5（EIA cutoff 边界口径：周结日结束时刻=当日零点 vs 契约 period<cutoff）**：未改动。仅当 cutoff 恰为周结日零点整时相差一例、方向保守（少保留），phase-2b ⑥③已备案，属收尾确认项非缺陷。

## ⑤ 残余风险

1. **归一后 agent 历史带双错误表述**：类型化错误帧归一后 agent 历史同时含 `error`/`error_code`/`message` 键（JSON 键序按字母重排）。语义无损，仅提示词略冗余；如后续在意可让归一只保留 `error` 键，但那会改变附录/日志可见形状，本次未动。
2. **tool_calls 记录 Outcome 与 ResultFull 的判定面不一致（既有设计，非本次引入）**：`mergeRawResponsesIntoRecords` 写回的原始帧无顶层 `error` 键，若有未来消费方对信号链记录用 `toolResultErrorText(ResultFull)` 重判（如 investigation synthesis 的做法），会得到与 Outcome 相反的结论。信号链自身的附录/gap 判定走 `signalSourceErrorText`（两形状都认），当前无此消费方；记录在案防误用。
3. **WDI 计算能力首次有测试覆盖**：M1 修复前 WDI difference/mean 全域被误拒（测试空白），本次补齐三条正向/拒绝路径；WDI 的 percent_change（不比较期间）修复前即可用、本次未单列用例（被 M1③ 的跨源拒绝用例间接覆盖来源侧）。
4. **L3 共享 helper 错误消息复用**：`parseBeforeID/parseListLimit` 的 400 消息与原内联实现逐字相同，行为等价由既有 handler Signal 回归钉住；无新增边界用例（原内联路径同样没有，未扩大范围）。
5. 审查未报的新问题：修复过程中未发现需上报的额外缺陷。
