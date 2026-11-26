# design: fix-daily-report-highlights-null

> 主线程设计制品。决策按 §0.6 review 要点 0 逐条给「落点」（仓库文件全路径 / tasks.md 任务 id / delta spec Scenario 标题 / 「无代码落点」+ 理由）。

## 背景与根因（复核结论）

用户报「日报 top3 新闻消失」。2026-11-04 排查锁定四层根因（实证见 proposal.md 的 Why 节）：

| 层 | 位置 | 事实 |
| --- | --- | --- |
| 模型 | `ai_call_logs` id 2612630 | deepseek-flash 把 JSON schema 原样回抄（合法 JSON、缺 `highlights` 键），`success=t` |
| 解析 | `backend-go/internal/topicgraph/service/daily_report_llm.go:115-142` | `parseHighlightsResponse` 对「缺键」返回 `(nil, nil)`，**静默** |
| 落库 | `backend-go/internal/topicgraph/service/daily_report_orchestrator.go:180,207` | `json.Marshal(nil slice)` → `null`；`JSON` 底层 `[]byte`，`Value()` 的 `j == nil` 判断拦不住 4 字节 `"null"` |
| 展示 | `front/app/features/tags/components/daily-report/DailyReportMasthead.vue:12,37` | `null → [] → v-if 为假 → 整块无声消失` |

同指纹历史复发 14 次（id 503/584/598/668/690/708/727/763/775/859/871/973/1256/1303，2026-08-08 ~ 2026-11-03，跨 5 个板块）。

## 决策

### D1 — 响应合法性以「顶层含 `highlights` 键」为判据

- **决策**：解析层 unmarshal 到 `map[string]json.RawMessage` 先探测 `highlights` 键；缺键 → 返回明确 error；键在但值为 `[]` → 合法降级（空切片 + nil error）。
- **取舍**：不做完整 JSON Schema 校验（成本高、收益低）；不做「每条字段逐一校验后才算合法」（把「模型没给内容」与「模型给了垃圾」混为一谈，两者处置不同：前者是合法降级，后者是非法响应）。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_llm.go`（T1）；delta spec Scenario「schema 回抄响应被判定非法并触发重试」「合法空数组不触发重试」。
- **理由**：schema 回抄的 JSON 顶层键是 `type`/`properties`/`required`，**没有** `highlights` → 判非法天然覆盖该形态，无需特判「是不是 schema」。

### D2 — 非法响应与调用失败都重试 1 次

- **决策**：`GenerateHighlights` 内对「调用失败」与「解析判定非法」统一重试 1 次（同参重试，temperature 0.3 自带随机性）。
- **取舍**：不做指数退避 / 不换模型 / 不做多次重试——单用户产品、单点调用，重试 1 次的边际收益最大而成本最低；退避与换模型会引入新的行为面。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_llm.go`（T2）；delta spec Scenario「schema 回抄响应被判定非法并触发重试」「重试成功则采用重试结果」。

### D3 — 降级不阻断整份日报（本次最关键的取舍）

- **决策**：重试仍失败 → **保留整份日报**（sections / threads / 统计字段照常落库），`highlights` 置空数组 + Warn 日志。
- **取舍**：放弃「要么有 highlights 要么不出日报」的强一致。理由：现状 `hr.err != nil` 直接 return，会让**已算好的 sections/threads 全部作废**——为 2-3 条要闻陪葬整份日报，代价与收益严重倒挂；且「宁可不生成」（flow 约束 11）本就允许要闻缺席。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（T3）；delta spec Scenario「重试仍失败则降级保留日报」「降级有可观测日志」。
- **配套**：降级 SHALL NOT 用其它字段拼凑 highlights（不伪造内容）。
- **副效应（review R-4，已知并接受）**：降级日报以 `status=completed` 落库后，调度闸 `dailyReportExistsForDate` 命中 → **当天不再自动重跑**，即「一次降级 = 当天该板块无重点」。本次**不**为此新增持久化标记（新增列/状态值会扩大数据模型变更面，与 D6 一致）；补救手段是人工按 board + 日期重跑，因此降级 Warn 日志 SHALL 带 board id 与失败原因以便定位。

### D4 — 落库复用同包既有 `marshalJSONArray`

- **决策**：`json.Marshal(hr.data)` → `marshalJSONArray(hr.data)`（`backend-go/internal/topicgraph/service/marshal_array.go:12`）。
- **取舍**：不新写 helper——同包已有实现 + 已有回归护栏 `marshal_array_test.go`，复用即零新增概念。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（T3）；delta spec Scenario「highlights 落库恒为数组」。

### D5 — 读路径归一历史 null

- **决策**：`GetReportByID` / `ListReports` 读出后，若 `highlights` 内容是 JSON `null` → 归一为 `[]`。
- **取舍**：不写 SQL 迁移去回填历史行——那是数据操作（用户已明确本 change 不动数据）；代码层归一同样让 API 契约恒定，且新数据不再产生 null。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_repository.go`（T4）；delta spec Scenario「历史 null highlights 读出归一为数组」。
- **同形先例**：既有 threads 读路径已做同款归一（`daily_report_repository.go:416-422`，测试 `daily_report_repository_test.go:135`）。

### D6 — 不动 `status` 状态机

- **决策**：`board_daily_reports.status` 仍写 `completed`，不新增 `degraded` 值。
- **取舍**：新增状态值会扩大下游兼容面（列表过滤、前端状态渲染、归档口径），而日报主体确实生成完成；降级可观测性由 Warn 日志 + 前端空态承担，够用。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（T3）；无代码落点（不改 status）——理由：本决策是「显式不改」，由 T3 的实现与 delta spec「重试仍失败则降级保留日报」共同固定。

### D7 — 前端空态占位而非隐藏

- **决策**：`highlights` 为空时渲染「本期暂无重点」占位（`data-testid="highlights-empty"`），正常态渲染零变化。
- **取舍**：不做「空态时用头条降级顶替」（未归档 change `redesign-daily-report-front-page` 的 proposal 有此设想）——本次只保证「缺失可见」，不擅自决定「用什么顶替」，避免与那个未批 change 打架。
- **落点**：`front/app/features/tags/components/daily-report/DailyReportMasthead.vue`（T5）；delta spec Scenario「highlights 为空数组时渲染空态占位」「highlights 为 null 时渲染空态占位」「highlights 正常时渲染要闻且无空态占位」。

### D8 — LLM 调用注入点 `highlightsChatFn`

- **决策**：新增包级 `var highlightsChatFn`，签名与同包 `clusterChatFn` / `bucketChatFn` / `laneSnapshotChatFn` 同形；生产默认实现发真实 airouter 调用，测试替换为 fake。
- **取舍**：不用 `airouter.NewRouterWithStore` + `RegisterClient`（需要 router 形参 DI，会改动函数签名并波及调用方）；不用 `watchChatFunc` 那种「公开壳 + 内部带 chat 形参」双层结构（本次只有一个调用点，一层包级 var 足够）。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_llm.go`（T2）；delta spec Scenario「重试成功则采用重试结果」「重试仍失败则降级保留日报」。

## 风险与影响面

| 风险 | 缓解 |
| --- | --- |
| 重试使日报生成耗时 +1 次 LLM 调用 | 仅在失败路径触发；成功路径零额外调用 |
| 降级后出现「有日报但无重点」 | 前端空态占位可见（D7）+ Warn 日志（可观测）；不再静默 |
| **降级日报占住当天生成闸**（review R-4）：降级以 `completed` 落库 → 当天 `already_exists` 不再自动重跑 | 本次接受（与「不自动重跑」的用户取向一致）；不新增持久化标记；补救 = 人工按 board+日期重跑；降级 Warn 日志带 board id 与失败原因便于定位 |
| 读路径归一改动了 API 响应形状 | 仅把 `null` 变 `[]`，非 `null` 值原样透传 |
| 前端空态与未归档 change 的刊头改版冲突 | 本 change 不动该 change 目录；文案与其 proposal 的「无 highlights 降级」口径一致 |

## 验证策略（详见 tasks.md 验证节与 test-cases.md）

- 后端：解析层纯函数单测（最便宜层）→ 重试路径（注入 fake）→ 落库/降级纯函数 → 读路径归一（repository 测试）。
- 前端：组件测试（`data-testid` 判定空态/正常态互斥）+ 既有 `DailyReportReader.test.ts` 回归。
- 真库核对（效果核对）：跑一次 `board_daily_reports` 只读查询，确认新生成日报的 `jsonb_typeof(highlights)` 为 `array`。
