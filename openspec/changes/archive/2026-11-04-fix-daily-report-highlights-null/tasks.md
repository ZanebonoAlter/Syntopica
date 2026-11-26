# tasks: fix-daily-report-highlights-null

> 编排：主线程写制品（步骤 1-2），实现派子线程（步骤 3），review 派子线程（步骤 4），验收门禁（步骤 5），归档（步骤 6）。
> 制品：`proposal.md` / `ui-design.md` / `design.md` / `specs/daily-report-system/spec.md` / `test-cases.md`。

## 1. 后端：解析层语义校验（T1）

- [x] **T1** `backend-go/internal/topicgraph/service/daily_report_llm.go`：`parseHighlightsResponse` 改为「顶层含 `highlights` 键」判据——先 unmarshal 到 `map[string]json.RawMessage` 探测键；**缺键**（含 schema 回抄形态）→ 返回明确 error；**键存在但值为 `[]`** → 合法降级（空切片 + nil error）；`highlights` 值非数组 → 判非法 error；非法 JSON → 保持原 error 语义。**MUST NOT** 把「缺键」再当成 `(nil, nil)` 静默放过。
- [x] **T1b** 同文件：保留既有过滤语义（`title` 全空白条目剔除、`tag_ids` 只留当次标签的合法 id），本次**不改**这两条规则。
- 验收：`TestParseHighlightsResponse_RejectsSchemaEcho` / `..._RejectsNonObject` / `..._RejectsNonArrayHighlights` / `..._RejectsInvalidJSON` / `..._AcceptsEmptyArray` 全绿。

## 2. 后端：单次重试 + LLM 调用注入点（T2）

- [x] **T2** `backend-go/internal/topicgraph/service/daily_report_llm.go`：新增包级 `var highlightsChatFn`（签名与同包 `clusterChatFn` / `bucketChatFn` / `laneSnapshotChatFn` 同形），`GenerateHighlights` 改调它；默认实现发真实 airouter 调用（**参数与现状逐字一致**，不改 prompt、不改 temperature/maxTokens）。
- [x] **T2b** 同文件：`GenerateHighlights` 对「调用失败」与「解析判定非法」统一**重试 1 次**（同参重试）；两次均失败才返回 error。**MUST NOT** 无限重试；重试成功 SHALL 采用重试结果。
- 验收：`TestGenerateHighlights_RetriesOnSchemaEcho`（调用次数=2 且采用重试结果）/ `TestGenerateHighlights_GivesUpAfterOneRetry`（调用次数恰为 2）/ `TestGenerateHighlights_EmptyTagsSkipsCall`（不发调用）全绿。

## 3. 后端：降级不阻断 + 落库契约（T3）

- [x] **T3** `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`：`highlights` 结果收集处（现 `hr := <-highlightsCh` 后 `hr.err != nil` 直接 return）改为**降级不阻断**——失败时保留日报主体（sections / threads / 统计字段照常组装与落库），`highlights` 置空，**MUST NOT** 让整份日报生成失败。
- [x] **T3b** 同文件：`json.Marshal(hr.data)` → 复用同包 `marshalJSONArray`（`marshal_array.go:12`），使 `highlights` **恒为 jsonb 数组**、空时落 `[]`，**MUST NOT** 落 JSON `null` 标量。
- [x] **T3c** 同文件：降级时写一条 **Warn 级日志**（含 board id、失败原因、「已降级为空 highlights」语义），风格对齐同包 threads 降级日志。**MUST NOT** 用其它字段拼凑 highlights（不伪造要闻内容）。
- [x] **T3d** 同文件：抽出一个**纯函数**承载「生成结果 → 落库 payload + 是否降级」的映射，供单测直接覆盖（如 `resolveHighlightsPayload(data, err) (payload []byte, degraded bool)`）。
- 验收：`TestResolveHighlightsPayload_DegradesToEmptyArray`（payload 恰为 `[]`、degraded=true）/ `TestResolveHighlightsPayload_Normal`（payload 为 3 元素数组、degraded=false）全绿。

## 4. 后端：读路径归一（T4）

- [x] **T4** `backend-go/internal/topicgraph/repository/daily_report_repository.go`：读出的 `highlights` 若是 JSON `null` 标量或 SQL NULL → 归一为 `[]`（与既有 threads 读路径归一 `daily_report_repository.go:416-422` 同形）；非 null 数组 SHALL 原样透传。**归一只施加在真正暴露 `highlights` 的读路径**：`GetReportByID`（详情接口，唯一实际承载 `highlights` 的读路径）+ `ListReportsForAllBoards`（返回完整模型，当前无生产调用方、预留）；`ListReports` 返回 `[]ReportListItem`（该结构体**无** `Highlights` 字段）→ 其归一是死代码，**已删除**（review R-5）。
- 验收：`TestGetReportByID_NormalizesNullHighlights` / `TestGetReportByID_KeepsHighlightsArray` 全绿；既有 `TestGetReportByID_SectionWithoutThreadsMarshalsEmptyArray` 照跑不回归。

## 5. 前端：刊头空态占位（T5）

- [x] **T5** `front/app/features/tags/components/daily-report/DailyReportMasthead.vue`：`highlights` 为空（`null` / `[]` / 字段缺失）时渲染**空态占位**（文案「本期暂无重点」，`data-testid="highlights-empty"`），SHALL NOT 整块隐藏；正常态（≥1 条）渲染**零变化**（保留 `id="report-highlights"`、最多 3 条）。
- [x] **T5b** 同文件：空态与正常态**互斥**渲染（`v-if` / `v-else`），SHALL NOT 出现重复 id；空态**不得**引入按钮 / 跳转 / 批注锚点（无 `data-mn-anchor`）。
- [x] **T5c** 同文件：样式只复用既有 CSS 变量与 `.drm-highlights` 边框语言（加 `--empty` 修饰类），**MUST NOT** 引入新色值、**MUST NOT** 改深色模式分支。
- 验收：见第 7 节前端测试 + `ui-design.md` 验收节的人工留痕。

## 6. 测试：后端（T6）

- [x] **T6a** `backend-go/internal/topicgraph/service/daily_report_llm_test.go`：按 `test-cases.md` 白盒分支表 B2~B11 补齐解析层与重试用例（含 **schema 回抄** 核心回归用例）。
- [x] **T6b** `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go`：补 `resolveHighlightsPayload` 降级/正常用例（B12 / B13）。
- [x] **T6c** `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`：补读路径归一用例（B14 / B15）。
- [x] **T6d** 既有 `backend-go/internal/topicgraph/service/daily_report_tracing_test.go`（含 `GenerateHighlights(ctx, nil, nil)` 提前返回路径）与 `marshal_array_test.go` **照跑不回归**。
- [x] **T6e**（review R-2/R-3/R-6 补测）`backend-go/internal/topicgraph/service/daily_report_llm_test.go`：补 4 条——`TestGenerateHighlights_RetriesOnCallError`（调用失败 → 重试成功，calls=2）、`TestGenerateHighlights_GivesUpAfterCallError`（两次调用失败 → `err != nil` 且 `errors.Is` 可取到原始错误，calls=2）、`TestGenerateHighlights_EmptyArrayDoesNotRetry`（合法空数组 → calls=**1**，不重试）、`TestHighlightsChatFn_CarriesTagAndClusterCounts`（重试两次读到同样的 tag/cluster 计数，覆盖「同参重试」）。

## 7. 测试：前端（T7）

- [x] **T7** 新建 `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`：三条断言——3 条 highlights → 渲染 3 条 `.drm-highlight` 且**无**空态占位；`[]` → 渲染 `[data-testid="highlights-empty"]` 且**无**要闻条目；`null` → 渲染占位且**不抛错**。
- [x] **T7b** 既有 `front/app/features/tags/components/DailyReportReader.test.ts`（用例「刊头保留『今日重点』top3 高亮（不再隐藏）」）**照跑不回归**。

## 8. 文档同步（T8）

- [x] **T8a** `docs/reference/flow/daily-report.md`：在「业务约束与不变量」补 **highlights 生成健壮性与数组契约**（顶层键校验 / 单次重试 / 降级不阻断 / 恒 `[]` 不写 `null`），并按 §12.2 补「变更溯源」行（archive 后）。
- [x] **T8b** `docs/reference/api/daily-reports.md`：写明 `highlights` 字段**恒为数组**（空为 `[]`，**不**返回 `null`）+ 前端空态占位行为。

## 9. Review（T9）

- [x] **T9** 派 `change-review` 子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账（`### R-<N> <日期>【High|Medium|Low】<标题>` + 状态行）。重点复核项：① 解析层是否仍存在「静默返回空结果」的旁路；② 重试是否可能无限循环或改动生产参数；③ 降级路径是否真的保住整份日报；④ 读路径归一是否误改非 null 数组；⑤ 前端空态是否引入重复 id 或新交互元素。

## 10. 归档（T10）

- [x] **T10** 归档门禁（§11.1 六条件）：门禁命令实测通过 → 文档同步 → 验证可复现 → `scenario-trace.sh` 通过 → `review-trace.sh` 通过 → `ui-verification-trace.sh` 通过（minor 档需验收映射指名组件测试文件）→ `doc-impact.sh verify` + `check-standards.sh --change <name>` 零失败 → `openspec archive` → 补 flow 变更溯源链接。

## 11. 测试

- 后端（只跑受影响包，**不跑全量** `go test ./...`）：
  - `cd backend-go && go test ./internal/topicgraph/service -run 'Test(ParseHighlightsResponse|GenerateHighlights|ResolveHighlightsPayload)' -v` → PASS（新增用例全绿）
  - `cd backend-go && go test ./internal/topicgraph/repository -run 'TestGetReportByID' -v` → PASS（归一用例 + 既有 threads 归一用例不回归）
  - `cd backend-go && go test ./internal/topicgraph/service` → PASS（整包，含既有 tracing / marshal_array 护栏）
- 前端（只跑受影响文件，**参数不带 `--`**）：
  - `cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.test.ts --maxWorkers=2` → PASS
  - `cd front && pnpm test:unit app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` → PASS（既有回归）
- 质量门禁：
  - `cd backend-go && golangci-lint run ./internal/topicgraph/...` → 0 issues
  - `cd backend-go && go vet ./internal/topicgraph/...` → 无输出
  - `cd front && pnpm lint` → 无 error
  - `cd front && pnpm exec nuxi typecheck` → 无类型错误

## 12. 文档

<!-- doc-impact: flow, api -->

- [ ] `docs/reference/flow/daily-report.md` — 补 highlights 生成健壮性不变量 + 变更溯源行（T8a）
- [ ] `docs/reference/api/daily-reports.md` — `highlights` 字段恒为数组契约 + 前端空态占位行为（T8b）

> 不声明 `database` 域：本次**无 schema 变更**（不新增列、不加迁移），`board_daily_reports.highlights` 的读写逻辑变更属代码层，且 `docs/reference/database/tables/` 无该表条目。

## 13. 验证

### 13.1 门禁命令（每条须实测零失败）

| 命令 | 期望结果 |
| --- | --- |
| `cd backend-go && go test ./internal/topicgraph/service` | PASS |
| `cd backend-go && go test ./internal/topicgraph/repository` | PASS |
| `cd backend-go && golangci-lint run ./internal/topicgraph/...` | 0 issues |
| `cd backend-go && go vet ./internal/topicgraph/...` | 无输出 |
| `cd backend-go && go build ./...` | 成功 |
| `cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.test.ts --maxWorkers=2` | PASS |
| `cd front && pnpm test:unit app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` | PASS |
| `cd front && pnpm lint` | 无 error |
| `cd front && pnpm exec nuxi typecheck` | 无类型错误 |
| `bash scripts/harness/scenario-trace.sh openspec/changes/fix-daily-report-highlights-null` | 退出码 0 |
| `bash scripts/harness/review-trace.sh openspec/changes/fix-daily-report-highlights-null` | 退出码 0 |
| `bash scripts/harness/ui-verification-trace.sh openspec/changes/fix-daily-report-highlights-null` | 退出码 0 |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-daily-report-highlights-null` | 零失败 |
| `bash scripts/harness/check-standards.sh --change fix-daily-report-highlights-null` | A-D/F/G 段零失败 |

### 13.2 Scenario → 测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
| --- | --- |
| 创建日报记录 | 人工：走 `POST /api/daily-reports/generate` 生成一次，只读查库确认落一行 `status=generating`（既有行为，红线 1 幂等语义不变） |
| 日报记录唯一性 | 人工：同一 board 同日连跑两次生成，只读查库确认仍为一行 |
| 并发落库同板同日不炸唯一约束 | 人工：同日并发触发两次生成，只读查库确认收敛为一行且无 duplicate key 报错 |
| 日报关联昨日报告 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 线程存储在独立表中 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 兄弟桶共享聚类序号 | backend-go/internal/topicgraph/service/daily_report_orchestrator_bucket_test.go |
| 未分桶 section 取默认桶序号 | backend-go/internal/topicgraph/service/daily_report_orchestrator_bucket_test.go |
| highlights 落库恒为数组 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 历史 null highlights 读出归一为数组 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| schema 回抄响应被判定非法并触发重试 | backend-go/internal/topicgraph/service/daily_report_llm_test.go |
| 重试成功则采用重试结果 | backend-go/internal/topicgraph/service/daily_report_llm_test.go |
| 重试仍失败则降级保留日报 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 合法空数组不触发重试 | backend-go/internal/topicgraph/service/daily_report_llm_test.go |
| 正常 highlights 正常落库 | backend-go/internal/topicgraph/service/daily_report_llm_test.go |
| 降级有可观测日志 | 人工：注入一次失败降级，读应用日志确认有含 board id 与失败原因的 Warn 行 |
| highlights 为空数组时渲染空态占位 | front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts |
| highlights 为 null 时渲染空态占位 | front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts |
| highlights 正常时渲染要闻且无空态占位 | front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts |

**映射注记（review R-1）**：「重试仍失败则降级保留日报」这一行的自动化用例**只覆盖 payload 降级分支**（payload = `[]` + degraded = true），「日报主体仍组装并落库」靠编排层代码路径保证——仓库既有编排测试一律走纯函数，无端到端 `GenerateDailyReport` 测试（能力缺口，非本次漏写）。主体保留的**人工核对**方式：注入一次 highlights 失败降级（fake hook 恒失败），只读查库确认日报行存在、`sections` / `threads` 非空、`jsonb_typeof(highlights)='array'`。

### 13.3 效果核对（真库只读；禁止任何写操作）

- 触发原因：结果依赖真实模型的输出形态与真实 jsonb 值形态，单测只覆盖模拟。
- 核对方法：
  1. 只读查询新生成日报的 `jsonb_typeof(highlights)` 与 `jsonb_array_length(highlights)` → 期望恒为 `array`；
  2. 只读查询 `ai_call_logs` 中 `operation='daily_report.highlights'` 最近若干条响应片段 → 确认无新的 schema 回抄形态；
  3. 只读反查全部历史行 `jsonb_typeof(highlights)='null'` 的记录（review R-7：null 有**两条**生产路径——schema 回抄与合法空数组——落库同指纹、无法从结果反推，核对时不得只查 `ai_call_logs` 就断言已清零）；
  4. 静态发布路径打开一篇坏日报（board 1980 / 2026-11-03，历史 null 行）与一篇正常日报 → 确认前者显示「本期暂无重点」占位、后者 3 条要闻渲染与改动前一致。
- 量化结果（2026-11-04 实测）：
  1. 读路径归一生效：重启后端后 `GET /api/daily-reports/1303` → `highlights` 返回 `array[0]`（**不再**是 `null`）；`GET /api/daily-reports/1332` → `array[3]`（正常数组原样透传）。
  2. 前端产物含新空态代码：重建后 `backend-go/frontend/_nuxt/CfjZppCj.js` 含「本期暂无重点」与 `highlights-empty`。
  3. 浏览器实测：坏日报（`/tags/1980/2026-11-03`）显示占位「本期暂无重点」且 0 条要闻；正常日报（`/tags/1980/2026-11-02`）渲染 3 条要闻、无占位；两页 `.drm-highlights` 区块数均为 1（id 互斥）。
  4. 历史行未物理清理（本次**不改数据**，用户 2026-11-04 拍板）：`board_daily_reports` 中仍有 14 行 `jsonb_typeof(highlights)='null'`，读路径已归一、前端显示占位；新生成日报写 `[]`，不再产生 null。
- 结论：**达标交付**。

> **红线**：真库只允许 SELECT；任何带副作用的 SQL 一律不碰真库（`standard/backend/testing.md` §🛑）。本次**不重跑、不修数据**——用户 2026-11-04 明确拍板。
