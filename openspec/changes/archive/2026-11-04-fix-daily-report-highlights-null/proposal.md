<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

## Why

2026-11-03 用户报「日报的 top3 新闻消失了」。排查结论（2026-11-04）：

**症状**：board 1980 最新一期日报（`board_daily_reports.id=1303`，period_date=2026-11-03）在页面上整块「今日重点」消失，刊头只剩头条。

**根因链**（四层，每层都有实证）：

1. **模型层**：`ai_call_logs` id `2612630`（2026-11-03 21:02:40，operation=`daily_report.highlights`，model=`deepseek-flash`，success=`t`）——模型把 JSON schema **原样回抄**当成了回答：
   `{"type":"object","properties":{"highlights":{...}},"required":["highlights"]}`。
   这是合法 JSON，`airouter` 的 `correctStructuredJSON` 判定 `json.Valid` 即放行，**路由层不拦**。
2. **解析层**：`backend-go/internal/topicgraph/service/daily_report_llm.go:115-142` `parseHighlightsResponse` 把响应 unmarshal 进 `struct{ Highlights []repository.Highlight }`——顶层没有 `highlights` 键 → `raw.Highlights` 为 nil → `result` 保持 nil → `return result, nil`。**合法 JSON 但语义缺失，解析层静默放过且不报错**。
   > **同一指纹的第二条路径**（review R-7 补充）：即使顶层**有** `highlights` 键，只要值为空数组 `[]`，改前的 `var result []repository.Highlight` 同样保持 nil → 同样返回 `(nil, nil)` → 同样落 jsonb `null`、同样让标题回落「日报 YYYY-MM-DD」+ summary 空。即「模型给了合法但空的 highlights」也是 null 的生产路径。对 id=1303 有 `ai_call_logs` id 2612630 的 schema 回抄直接实证；其余 13 行未逐一区分来源（两条路径同指纹、无法从落库结果反推）。本次修复**两条路径一并收口**（缺键 → 非法重试；空数组 → 合法降级但落 `[]`）。
3. **落库层**：`daily_report_orchestrator.go:180` `json.Marshal(hr.data)`（nil slice → `[]byte("null")`）→ `daily_report_models.go:27` 的 `Highlights JSON`（底层 `[]byte`，`Value()` 只判 `j == nil`，4 字节 `"null"` 非 nil）→ jsonb 落 **JSON `null` 标量**。**违反 flow 红线**（`docs/reference/flow/daily-report.md:247`：空引用写 `[]`、MUST NOT 写 JSON `null` 标量）。
4. **前端层**：`front/app/features/tags/components/daily-report/DailyReportMasthead.vue:12,37`——`highlights` 为 null → `|| []` → `v-if="highlights.length"` 为假 → **整块无声消失**，用户完全看不出「本来应该有 3 条」。

**这不是新回归，是长期潜伏的偶发缺陷**：同指纹（标题退化成 `日报 YYYY-MM-DD` + summary 空 + highlights 为 jsonb null）在 `board_daily_reports` 历史上共 **14 行**（id 503/584/598/668/690/708/727/763/775/859/871/973/1256/1303，跨 5 个板块、2026-08-08 至 2026-11-03）。每命中一次，那个板块当天的「今日重点」就静默消失一次。**触发源有两条**（schema 回抄 / 合法空数组），落库指纹相同、无法从结果反推，本次一并修。

**为什么现在必须改**：解析层「静默吞掉语义缺失」+ 落库层「null 逃过 [] 契约」+ 前端层「空即隐藏」三重叠加，把一个模型侧的偶发输出放大成了**用户可见的静默数据丢失**。任一层单独加固都能止血，本次三层一起收口。

**用户拍板（2026-11-04）**：只改代码、**不重跑**历史与当前这份日报；前端补空态占位，让「本来就没有重点」这件事**可见**而不是无声消失。

## What Changes

### 后端（`backend-go/internal/topicgraph/service/`）

- **解析层语义校验**（`daily_report_llm.go` `parseHighlightsResponse`）：顶层 JSON 必须是**含 `highlights` 键**的对象。
  - 缺 `highlights` 键（含 schema 回抄形态）→ 返回**明确 error**，不再返回 `(nil, nil)`；
  - `highlights` 存在但为空数组 `[]` → 视为**合法降级**（事实锚约束下「宁可不生成」，见 flow 约束 11），返回空切片 + nil error；
  - 非法 JSON → 保持原有 error 语义。
- **单次重试**（`daily_report_llm.go` `GenerateHighlights`）：调用失败或解析层判定非法 → **重试 1 次**（同参重试，temperature 0.3 有随机性，第二次通常正常）；两次都失败才向上抛错。
  - 新增包级注入点 `highlightsChatFn`（照抄同包既有惯例 `clusterChatFn` / `bucketChatFn` / `laneSnapshotChatFn`），让重试路径可被单测覆盖。
- **降级不阻断整份日报**（`daily_report_orchestrator.go`）：highlights 重试后仍失败 → **不再让整份日报失败**（现状是 `hr.err != nil` 直接 return，整份日报连同已算好的 sections/threads 全丢）。改为：保留日报主体，highlights 置空，写 **Warn 级日志**（与同包 threads 降级 `falling back to tag-anchored synthesis` 同风格），日报照常落库。
- **落库契约对齐**（`daily_report_orchestrator.go`）：`json.Marshal(hr.data)` → 复用同包既有 helper **`marshalJSONArray`**（`marshal_array.go:12`，nil/空切片一律归一为 `[]`，已有回归护栏 `marshal_array_test.go`）——highlights 恒为 jsonb 数组，**不再出现 null 标量**。

### 读路径归一（`backend-go/internal/topicgraph/repository/daily_report_repository.go`）

- `GetReportByID` / `ListReports` 读出的 `highlights` 若是 JSON `null`（**历史 14 行坏数据**）→ 归一为 `[]`，与既有 threads 读路径归一（`daily_report_repository.go:416-422`）同形。效果：API 层 `highlights` **恒为数组**，前端类型 `highlights: DailyReportHighlight[]` 成立。

### 前端（`front/app/features/tags/components/daily-report/`）

- **空态占位**（`DailyReportMasthead.vue`）：`highlights` 为空（`null` 或 `[]`）时，刊头渲染**「本期暂无重点」**占位块（`data-testid="highlights-empty"`），而不是整块消失。文案对齐目录既有空态家族（「本期分区 / 本期目录 / 本期话题」+「暂无日报节点 / 暂无归档摘要 / 暂无事件脉络」）。
- 正常 3 条时的渲染**零变化**（`id="report-highlights"` 保留，全仓无其它锚点引用它）。

### 文档

- `docs/reference/flow/daily-report.md`：补「highlights 生成健壮性与数组契约」不变量（解析层语义校验 / 单次重试 / 降级不阻断 / 恒 `[]`）+ 变更溯源行。
- `docs/reference/api/daily-reports.md`：`highlights` 字段契约（恒为数组，空为 `[]`）+ 空态占位行为。

## Out of Scope

- **不重跑任何日报**（含 board 1980 的 2026-11-03 那一期）——用户 2026-11-04 明确拍板。重跑属写操作且会覆盖既有内容，不在本 change 内；后续需要时可另行触发 `POST /api/daily-reports/generate`。
- **不清理历史 14 行 null 坏数据**（读路径归一后它们在前端表现为「本期暂无重点」，不再消失；物理修数据属数据操作，另行处理）。
- **不改 `board_daily_reports.status` 状态机**：日报主体确实生成了，status 仍 `completed`；降级可观测性靠 Warn 日志 + 前端空态，不新增状态值（避免下游兼容面扩大）。
- **不改 highlights 的 system prompt 与事实锚约束**（flow 约束 11）——本次只修「响应如何被接收与落库」，不动「要闻如何生成」。
- **不动未归档 change `redesign-daily-report-front-page`**：该 change 在改同一块刊头 UI（其 proposal 已写「无 highlights 降级为当前 lead 摘要」），本次空态口径与其一致，但不在其目录内做任何改动；若该 change 后续获批落地，需对齐占位文案。
- **不做 A/B 或灰度**：单用户产品，直接改。

## Impact

**后端**
- `backend-go/internal/topicgraph/service/daily_report_llm.go`（解析层校验 + 重试 + `highlightsChatFn` 注入点）
- `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（降级不阻断 + `marshalJSONArray`）
- `backend-go/internal/topicgraph/repository/daily_report_repository.go`（读路径归一）
- 测试：`backend-go/internal/topicgraph/service/daily_report_llm_test.go`（扩）、`backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go`（扩）、`backend-go/internal/topicgraph/repository/daily_report_repository_test.go`（扩）

**前端**
- `front/app/features/tags/components/daily-report/DailyReportMasthead.vue`（空态占位）
- 测试：新建 `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`
- 回归：`front/app/features/tags/components/DailyReportReader.test.ts`（已有「刊头保留『今日重点』top3 高亮」用例，继承不改）

**文档**
- `docs/reference/flow/daily-report.md`、`docs/reference/api/daily-reports.md`

**数据库**
- 无 schema 变更（不新增列、不加迁移；读路径归一是纯代码层）
