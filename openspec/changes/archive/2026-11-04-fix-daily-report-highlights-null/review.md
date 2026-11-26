# review: fix-daily-report-highlights-null

> 审查执行：`change-review` 子线程聚焦审查（制品 6 份 + 实现 diff 8 个文件，只读）。
> 审查结论：**High 0 / Medium 4 / Low 7**；重点审查项 9 项中 7 项审毕无发现。
> 审查范围声明：工作树同时存在其它 active change（`fix-board-health-triage-degrade` / `add-dispatch-constraint-digest` 等）的脏文件，**本次审查与处置仅限本 change 的制品 + 8 个实现文件**。

### R-1 2026-11-04【Medium】「重试仍失败则降级保留日报」Scenario 只被半覆盖，映射表把纯函数当整条 Scenario
- **状态**：已修复
- **位置**：`openspec/changes/fix-daily-report-highlights-null/tasks.md`（§13.2 映射行）
- **问题**：delta spec 要求「日报照常组装并落库」，映射到 `daily_report_orchestrator_test.go`；但该文件只断言了纯函数 `resolveHighlightsPayload` 的 payload 与 degraded，「主体仍落库」这半条无断言（仓库既有编排测试一律走纯函数，无端到端 `GenerateDailyReport` 测试）。
- **处置**：按建议①收窄——映射单元格改为「**仅覆盖 payload 降级分支**；主体保留靠编排层代码路径保证，端到端编排测试属能力缺口」+ 追加「人工：注入一次失败降级，只读查库确认日报行存在且 highlights 为 `[]`」。delta spec 的 THEN 措辞同步收窄为「结构与统计字段完整；title / summary 按既有兜底链回落」（见 R-8）。

### R-2 2026-11-04【Medium】重试的「调用失败」分支零覆盖，两个重试用例都只走解析非法分支
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/daily_report_llm_test.go`
- **问题**：D2 与 delta spec 写的是「调用失败**或**解析非法 → 重试 1 次」，但两个 fake 都返回 `(schemaEcho, nil)`——`err` 恒为 nil，「airouter 调用本身失败」分支无任何用例；若有人把该分支的 `continue` 写成 `break`，现有用例仍全绿。
- **处置**：新增 `TestGenerateHighlights_RetriesOnCallError`（首次调用失败 → 重试成功 → 取重试结果，calls=2）与 `TestGenerateHighlights_GivesUpAfterCallError`（两次调用失败 → `err != nil` 且 `errors.Is` 可穿透 `%w` 取到原始错误，calls=2）。见 tasks.md T6e、test-cases.md 白盒表 B10a/B11a。

### R-3 2026-11-04【Medium】「合法空数组不触发重试」的「不重试」子句未被断言
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/daily_report_llm_test.go`
- **问题**：B6 的用例只调解析层（无 LLM 调用计数），检测不出「重试循环被改成对空数组也重试」这类回归。
- **处置**：新增 `TestGenerateHighlights_EmptyArrayDoesNotRetry`——fake 恒返回 `{"highlights":[]}` → 断言 `err == nil`、结果为空切片、**调用次数恰为 1**。见 tasks.md T6e、test-cases.md 白盒表 B19。

### R-4 2026-11-04【Medium】降级日报会占住当天生成闸，「一次降级 = 当天永久无重点」未进风险表与 spec
- **状态**：已采纳
- **位置**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（降级落库）+ `backend-go/internal/admin/scheduler/job_daily_report.go`（`already_exists` 闸）
- **问题**：改前 highlights 失败 → 整份日报失败 → 不落库 → 调度还能重跑；改后降级落库 `status=completed` → `dailyReportExistsForDate` 命中 → 当天不再重新生成。是 D3（降级不阻断）+ D6（不动 status）的必然副作用，原 design 风险表未列。
- **处置**：**主线程拍板接受该副效应**（不新增持久化标记）——理由：① 用户 2026-11-04 已明确「不自动重跑」的取向；② 让闸重跑需要新增列/状态值，扩大数据模型变更面，与 D6 冲突；③ 补救手段（人工按 board+日期重跑）成本可控。已落 `design.md` D3「副效应」段 + 风险表新增一行；flow 约束 41 保留「降级 SHALL 记含 board id 与失败原因的 warn 日志」以便定位重跑。

### R-5 2026-11-04【Low】`ListReports` 的归一是死代码（该 API 根本不返回 highlights）
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_repository.go`
- **问题**：`ListReports` 返回 `[]ReportListItem`（结构体无 `Highlights` 字段），归一写进的是随后被投影丢弃的本地 `[]BoardDailyReport`；tasks.md T4 对 ListReports 是空承诺。
- **处置**：删除 `ListReports` 内的归一循环（留 NOTE 注释说明契约由 `GetReportByID` + `ListReportsForAllBoards` 承担）；tasks.md T4 改述为「只施加在真正暴露 highlights 的读路径」。

### R-6 2026-11-04【Low】highlights 的 `tag_count`/`cluster_count` metadata 走 context 暗道，无测试且偏离同包惯例
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/daily_report_llm.go`
- **问题**：hook 签名无 metadata 槽位，实现把计数塞进 ctx value；若有人不经 `withHighlightsCallCounts` 直接调 hook，两个计数会静默变 0（`ai_call_logs` 审计字段失真，无报错）。
- **处置**：新增 `TestHighlightsChatFn_CarriesTagAndClusterCounts`——fake 内从 ctx 读计数，断言第一次调用读到 `len(tags)`/`len(clusters)`，且**重试的第二次调用读到同样的值**（同时覆盖「同参重试」）。见 tasks.md T6e、test-cases.md 白盒表 B20。

### R-7 2026-11-04【Low】proposal 根因归因不完整：合法空数组响应同样写 jsonb null，历史 14 行未必都是 schema 回抄
- **状态**：已修复
- **位置**：`openspec/changes/fix-daily-report-highlights-null/proposal.md`（Why 根因表）
- **问题**：改前「键存在但数组为空」同样返回 `(nil, nil)` → 同样落 null、同样让标题回落 + summary 空——与 schema 回抄完全同指纹，是 null 的**第二条**生产路径；proposal 只归到模型层 schema 回抄，tasks.md §13.3 的核对方法会漏掉这一路。
- **处置**：proposal 根因表补「同一指纹的第二条路径」说明块 + 症状段补「触发源有两条」；tasks.md §13.3 核对方法补第 3 条「按 `jsonb_typeof(highlights)='null'` 反查全部历史行并区分两路来源」。

### R-8 2026-11-04【Low】delta spec「字段完整」过度声称：降级时 title/summary 会退化
- **状态**：已修复
- **位置**：`openspec/changes/fix-daily-report-highlights-null/specs/daily-report-system/spec.md`
- **问题**：编排里 title/summary 取自 `hr.data[0]`，降级时回落「日报 YYYY-MM-DD」+ summary 空（**既有行为**，非本次引入）；但 spec 写「等字段完整」把 title/summary 也圈进「完整」。
- **处置**：THEN 措辞改为「sections / threads / article_count 等**结构与统计**字段完整；title / summary 在无 highlights 时按既有兜底链回落，本次改动 SHALL NOT 改动该兜底链」——**不**把标题回落写进契约。

### R-9 2026-11-04【Low】delta spec 合法性判据表述欠精确：`{"highlights":null}` / `{}` / `"x"` 未枚举
- **状态**：已修复
- **位置**：`openspec/changes/fix-daily-report-highlights-null/specs/daily-report-system/spec.md`
- **问题**：spec 只枚举「缺键→非法」「`[]`→合法降级」「非法 JSON→非法」；实现把「键在但非数组」一律判非法（比 spec 字面更严），spec 未写会导致将来分叉。
- **处置**：判据段补「`highlights` 键存在但值**非数组**（含 `null`、对象、字符串）→ SHALL 判定为非法响应并触发重试」，与 tasks.md T1 / 实现三方对齐。

### R-10 2026-11-04【Low】tasks.md 全部任务仍为未勾选，与实现完成状态不同步
- **状态**：已修复
- **位置**：`openspec/changes/fix-daily-report-highlights-null/tasks.md`
- **问题**：实现、测试、文档同步均已完成，但 T1~T10 一条未勾，归档时会被卡或留下不实记录。
- **处置**：按实际完成情况勾选 T1~T9；**T10（归档门禁）保持未勾**，归档完成后再勾。

### R-11 2026-11-04【Low】深色模式下空态占位可见度偏弱；人工验收清单未含深色模式
- **状态**：已修复
- **位置**：`front/app/features/tags/components/daily-report/DailyReportMasthead.vue`（`.drm-highlights__empty`）
- **问题**：占位用 `--color-text-muted`（浅色 ≈3.5:1 / 深色 ≈3.2:1，均低于小字号 4.5:1）；ui-design 人工验收清单未含深色模式。
- **处置**：颜色 token 改为 `--color-text-secondary`（既有 token，浅色 ≈6.9:1 / 深色 ≈7:1，仍不引入新色值、不改深色模式分支、不动正常态样式）；ui-verification/report.md 的 A-5 已注明深色模式目测留待部署后人工复验。

---

## 附：审查中「审毕无发现」的重点项（留痕）

- **解析层无静默旁路**：`parseHighlightsResponse` 5 条返回路径逐条核对，无「语义缺失但返回 nil error」的分支；`{"highlights":null}` / `{}` / `"x"` / 顶层数组 / 非 JSON 五种输入实测均判非法。
- **重试语义**：最多 2 次调用；解析成功即返回不丢首次合法结果；空 tags 提前返回不发调用；prompt/schema 在循环外构造一次两次复用（同参）；temperature 0.3 / maxTokens 2000 / JSONMode / JSONSchema / Capability / operation / Metadata 逐项与改前一致（仅新增 2 条 Warn 日志）。
- **降级链代码路径保住整份日报**：Step 6 组装段与 SaveReport 路径未改。
- **落库契约**：`resolveHighlightsPayload` → `marshalJSONArray` → 恒 jsonb 数组，不可能再出现 null 标量或 SQL NULL。
- **读路径归一只动 null/SQL NULL**：非 null 数组原样透传（B15 用 `assert.JSONEq` 逐字比对通过）。
- **前端空态**：与 HEAD 版本逐行比对，正常态模板与样式零变化；空态无按钮/链接/`data-mn-anchor`；未引入新色值；文案符合目录既有空态家族。
- **红线合规**：幂等 upsert 一行未改；jsonb 空引用写 `[]` 已落实 flow 约束 41；事实锚 prompt 一字未改；status 仍只写 `completed`；模型路由未改；无新增第三方依赖（新增 import 仅 `bytes` / `sort` 标准库）。
