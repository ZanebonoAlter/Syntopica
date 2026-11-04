# test-cases: fix-daily-report-highlights-null

> 单元模型：一个 Requirement = 一个用户故事。Scenario 是断言片段，本文档串起完整故事。
> 锚定需求：`specs/daily-report-system/spec.md`（MODIFIED 日报数据模型 / ADDED 日报 highlights 生成健壮性 / ADDED 刊头「今日重点」空态可见性）。

## 故事 S1: 模型吐了垃圾响应时，日报照样出得来，而且「今天没有重点」看得见（锚 Requirement: 日报 highlights 生成健壮性）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 模型把 JSON schema 原样回抄 | schema 回抄响应被判定非法并触发重试 | 解析层返回错误（不是空结果），触发 1 次重试 | 函数单测 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go`（`TestParseHighlightsResponse_RejectsSchemaEcho` / `TestGenerateHighlights_RetriesOnSchemaEcho`） |
| 2 | 重试调用返回 3 条合法 highlights | 重试成功则采用重试结果 | 日报采用重试结果落 3 条；首次的垃圾响应被丢弃 | 函数单测（注入 fake LLM） | 同上（`TestGenerateHighlights_RetriesOnSchemaEcho`） |
| 3 | 连续两次都失败 | 重试仍失败则降级保留日报 | 日报主体完整落库；`highlights` 为 `[]`；**不**写 `null`；整份日报**不**失败 | 函数单测 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go`（`TestResolveHighlightsPayload_DegradesToEmptyArray`） |
| 4 | 降级发生 | 降级有可观测日志 | 一条 Warn 级日志，含 board id 与失败原因 | 人工（日志核对） | 人工：跑一次失败注入的本地生成，读 stderr/日志确认含 board id 与原因 |
| 5 | 模型返回 `{"highlights":[]}` | 合法空数组不触发重试 | 判为合法降级，不重试；`highlights` 落 `[]` | 函数单测 | `daily_report_llm_test.go`（`TestParseHighlightsResponse_AcceptsEmptyArray`） |
| 6 | 模型返回 3 条正常 highlights | 正常 highlights 正常落库 | 3 条落库，title/reason 原样，tag_ids 只留合法 id | 函数单测 | `daily_report_llm_test.go`（`TestParseHighlightsResponse_Normal`）+ 既有 `TestParseThreadsResponse` 同形 |
| 7 | 读者打开刚生成的日报 | （前端故事 S3） | 刊头显示要闻或「本期暂无重点」，**不**整块空白 | 组件测试 | `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 输入 1 | 模型返回**非法 JSON**（截断的 `{`） | 判非法 → 重试 1 次 → 仍失败则降级 | 函数单测 | `daily_report_llm_test.go` |
| 输入 2 | 模型返回**合法 JSON 但顶层是数组**（`[{...}]`） | 判非法（顶层非对象 / 无 `highlights` 键）→ 重试 | 函数单测 | `daily_report_llm_test.go` |
| 输入 3 | 模型返回**含 `highlights` 键但值为对象**（非数组） | 判非法（类型不符）→ 重试 | 函数单测 | `daily_report_llm_test.go` |
| 输入 4 | 模型返回 3 条但**某条 title 全空白** | 该条被过滤，剩 2 条；不为空则正常落库 | 函数单测 | `daily_report_llm_test.go`（既有行为，回归护栏） |
| 输入 5 | 模型返回的 `tag_ids` 含**不存在的标签 id** | 非法 id 被剔除，合法 id 保留 | 函数单测 | `daily_report_llm_test.go` |
| 输入 6 | 模型返回 4 条（超出 2-3 条上限） | 后端原样落 4 条；前端只展示前 3 条（既有 `slice(0,3)`） | 函数单测 + 组件测试 | `daily_report_llm_test.go` / `DailyReportMasthead.test.ts` |
| 前置 1 | 当日**无任何事件标签**（tags 空） | 提前返回空、不发 LLM 调用（既有行为，零回归） | 函数单测 | 既有 `daily_report_tracing_test.go`（`GenerateHighlights(ctx, nil, nil)`）照跑 |
| 前置 2 | 部分失败后重试 | 首次失败 → 重试成功 → 采用重试结果（**不**把首次失败当终态）；失败分**调用失败**与**解析非法**两路，均走同一条重试路径 | 函数单测 | `daily_report_llm_test.go`（B10 / B10a / B11 / B11a） |
| 幂等 1 | 同日重跑（幂等覆盖重建） | 红线 1 语义不变：整份覆盖、不产生重复报告 | 人工 | 人工：连跑两次生成，确认库中同 board 同日仅 1 行 |
| 可用性 1 | **空态**（高亮为空） | 刊头显示「本期暂无重点」占位 | 组件测试 | `DailyReportMasthead.test.ts` |
| 可用性 2 | **错误态**（highlights 为 null） | 显示占位且**不抛错** | 组件测试 | `DailyReportMasthead.test.ts` |
| 可用性 3 | **加载态 / 报告未返回** | 刊头整体不渲染（既有行为，零回归） | 人工 | 人工：打开阅读台观察首帧 |

### 效果核对（触发问句④时）

- 触发原因：结果依赖**真实模型的输出形态**（schema 回抄是偶发行为，单测只能模拟不能复现真实概率）。
- 核对方法：改动上线后跑一次真实日报生成，随后只读查询 `board_daily_reports` 最新行的 `jsonb_typeof(highlights)` 与 `jsonb_array_length(highlights)`；再查 `ai_call_logs` 中 `operation='daily_report.highlights'` 的最近若干条响应片段，确认没有新的「schema 回抄」形态。
- 量化结果：（改动上线后填）新生成日报 `jsonb_typeof(highlights)` 全部为 `array`；schema 回抄形态出现 0 次。
- 结论：达标交付 ｜ 瓶颈在<上游> ｜ 需调整预期。

### 继承与调整（涉及 MODIFIED Requirement 时必填）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 创建日报记录 | 继承 | — | 结构未变，照跑 |
| 日报记录唯一性 | 继承 | — | 结构未变，照跑 |
| 并发落库同板同日不炸唯一约束 | 继承 | — | 结构未变，照跑 |
| 日报关联昨日报告 | 继承 | — | 结构未变，照跑 |
| 线程存储在独立表中 | 继承 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` | 照跑（回归网） |
| 兄弟桶共享聚类序号 | 继承 | 同上 | 照跑 |
| 未分桶 section 取默认桶序号 | 继承 | 同上 | 照跑 |
| （新增）highlights 落库恒为数组 | 新增 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` | 新写 |
| （新增）历史 null highlights 读出归一为数组 | 新增 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` | 新写 |

## 故事 S2: 读旧日报时接口不吐 null（锚 Requirement: 日报数据模型）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| 1 | 库里存在 `highlights` 为 JSON `null` 的历史行（id=1303） | 历史 null highlights 读出归一为数组 | API 返回 `highlights: []`，**不**是 `null` | repository 单测（真库 testcontainer 或本地 PG） | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`（`TestGetReportByID_NormalizesNullHighlights`） |
| 2 | 库里 `highlights` 为正常数组 | （回归） | 原样透传，条数与内容不变 | repository 单测 | 同文件（`TestGetReportByID_KeepsHighlightsArray`） |
| 3 | 新生成的日报落库 | highlights 落库恒为数组 | `jsonb_typeof(highlights)='array'`，**不**是 `null` | 人工（真库只读核对） | 人工：只读 SELECT 查最新行 `jsonb_typeof(highlights)` |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 输入 1 | `highlights` 列为 SQL NULL（空列） | 归一为 `[]` | repository 单测 | `daily_report_repository_test.go` |
| 输入 2 | `highlights` 为 `[]` | 保持 `[]` | repository 单测 | 同上 |
| 输入 3 | `highlights` 为 3 条数组 | 原样透传 | repository 单测 | 同上 |
| 前置 1 | 报告不存在 | 既有行为不变（返回未找到） | 既有测试 | 照跑 |
| 可用性 1 | 前端拿到 `[]` | 显示空态占位 | 组件测试 | `DailyReportMasthead.test.ts` |

### 效果核对

- 触发原因：归一逻辑作用在真实 jsonb 值上（含 `null` 标量与 SQL NULL 两种形态）。
- 核对方法：改动后对本机存量坏行做只读查询 + 走一次读接口比对。
- 量化结果：（填）14 行历史 null 数据读出全部为 `[]`。
- 结论：达标交付 ｜ 需调整预期。

## 故事 S3: 读者打开一篇没有重点的日报，看到的是占位而不是空白（锚 Requirement: 刊头「今日重点」空态可见性）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| 1 | 打开 `highlights` 为 `[]` 的日报 | highlights 为空数组时渲染空态占位 | 渲染「本期暂无重点」占位；不渲染要闻条目 | 组件测试 | `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` |
| 2 | 打开 `highlights` 为 `null` 的日报 | highlights 为 null 时渲染空态占位 | 渲染占位，**不抛错**，**不**整块消失 | 组件测试 | 同上 |
| 3 | 打开正常日报 | highlights 正常时渲染要闻且无空态占位 | 渲染 3 条要闻；无空态占位元素 | 组件测试 | 同上 |
| 4 | 窄视口（≤720px）看空态 | — | 占位单列显示、不溢出、不与头条重叠 | 人工 | 人工：静态发布路径 + 窄视口目测 + 矩形不相交检测 |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 输入 1 | `highlights` 含 1 条 | 渲染 1 条要闻，无占位 | 组件测试 | `DailyReportMasthead.test.ts` |
| 输入 2 | `highlights` 含 5 条 | 只展示前 3 条（既有 `slice(0,3)`） | 组件测试 | 同上 |
| 前置 1 | 报告对象无 `highlights` 字段（undefined） | 按空处理、显示占位、**不抛错** | 组件测试 | 同上 |
| 可用性 1 | **空态** | 占位可见 | 组件测试 | 同上 |
| 可用性 2 | **错误态**（null） | 占位可见 | 组件测试 | 同上 |
| 可用性 3 | **加载态** | 刊头整体不渲染（既有） | 人工 | 人工观察 |
| 可用性 4 | **超长文本**（要闻标题 60 字） | 既有断词/换行兜底不破版 | 人工 | 人工：窄视口长内容检查 |

### 效果核对

- 触发原因：占位是否真的「可见」取决于 CSS（`.drm-highlights--empty` 是否被某条样式压掉）。
- 核对方法：静态发布路径打开坏日报（board 1980 / 2026-11-03）+ 正常日报各一次，肉眼 + 矩形不相交检测。
- 量化结果：（填）坏日报占位可见、正常日报 3 条要闻渲染与改动前一致。
- 结论：达标交付 ｜ 需调整预期。

## 白盒附加（复杂档：LLM 响应解析 / 重试 / 降级链）

### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| B1 | tags 为空提前返回 | `tags=nil` | 返回 `(nil, nil)`，**不发** LLM 调用 | `TestGenerateHighlights_EmptyTagsSkipsCall` |
| B2 | 响应非法 JSON | `{"highlights":` | 判非法 → error | `TestParseHighlightsResponse_RejectsInvalidJSON` |
| B3 | 响应顶层为数组 | `[{"title":"x"}]` | 判非法（无 `highlights` 键）→ error | `TestParseHighlightsResponse_RejectsNonObject` |
| B4 | 响应为 schema 回抄 | `{"type":"object","properties":{...},"required":["highlights"]}` | 判非法 → error（**核心回归用例**） | `TestParseHighlightsResponse_RejectsSchemaEcho` |
| B5 | `highlights` 键存在但值为对象 | `{"highlights":{}}` | 判非法（类型不符）→ error | `TestParseHighlightsResponse_RejectsNonArrayHighlights` |
| B6 | `highlights` 为空数组 | `{"highlights":[]}` | 合法降级：空切片 + nil error，**不重试** | `TestParseHighlightsResponse_AcceptsEmptyArray` |
| B7 | 正常 3 条 | `{"highlights":[{...},{...},{...}]}` | 返回 3 条 | `TestParseHighlightsResponse_Normal` |
| B8 | 条目 title 空白 | 某条 `"title":"   "` | 该条被过滤 | `TestParseHighlightsResponse_DropsBlankTitle` |
| B9 | tag_ids 含非法 id | `"tag_ids":[999999]` | 非法 id 剔除，合法 id 保留 | `TestParseHighlightsResponse_FiltersUnknownTagIDs` |
| B10 | 首次失败 + 重试成功 | fake 第一次返回 schema 回抄、第二次返回正常 | 采用重试结果；调用次数 = 2 | `TestGenerateHighlights_RetriesOnSchemaEcho` |
| B11 | 首次失败 + 重试失败 | fake 两次都返回 schema 回抄 | 返回 error；调用次数 = 2（**不无限重试**） | `TestGenerateHighlights_GivesUpAfterOneRetry` |
| B10a | **调用失败** + 重试成功 | fake 第一次返回 `("", err)`、第二次返回合法响应 | `err == nil`；取重试结果；调用次数 = 2 | `TestGenerateHighlights_RetriesOnCallError`（review R-2a 补） |
| B11a | **调用失败** × 2 | fake 两次都返回 `("", err)` | `err != nil`；`errors.Is` 能取到原始错误（`%w` 链保留）；调用次数 = 2 | `TestGenerateHighlights_GivesUpAfterCallError`（review R-2b 补） |
| B19 | 合法空数组**不重试** | fake 恒返回 `{"highlights":[]}` | `err == nil`；空切片；**调用次数 = 1** | `TestGenerateHighlights_EmptyArrayDoesNotRetry`（review R-3 补） |
| B20 | 重试**同参**（metadata 计数一致） | fake 记录每次调用从 ctx 读到的计数 | 两次调用读到同样的 `len(tags)` / `len(clusters)` | `TestHighlightsChatFn_CarriesTagAndClusterCounts`（review R-6 补） |
| B12 | 降级落库 | highlights 生成失败 | payload 为 `[]`（4 字节 `[]`，**非** `null`），degraded=true | `TestResolveHighlightsPayload_DegradesToEmptyArray` |
| B13 | 正常落库 | highlights 3 条 | payload 为 3 元素数组，degraded=false | `TestResolveHighlightsPayload_Normal` |
| B14 | 读路径归一 | 库中 `highlights` = `null` | 读出为 `[]` | `TestGetReportByID_NormalizesNullHighlights` |
| B15 | 读路径透传 | 库中 `highlights` = 3 条 | 原样，条数不变 | `TestGetReportByID_KeepsHighlightsArray` |
| B16 | 前端空态 | `highlights=[]` | 渲染 `[data-testid="highlights-empty"]`，不渲染 `.drm-highlight` | `DailyReportMasthead.test.ts`（`renders empty state when highlights empty`） |
| B17 | 前端 null | `highlights=null` | 渲染占位，不抛错 | 同上（`renders empty state when highlights null`） |
| B18 | 前端正常 | `highlights` 3 条 | 渲染 3 条，无占位 | 同上（`renders highlights without empty state`） |

### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| highlights 条数 | 0 | 空数组合法降级 | B6 |
| highlights 条数 | 1 | 正常落库 | B7（同形） |
| highlights 条数 | 3 | 正常落库（设计上限内） | B7 |
| highlights 条数 | 4（越界） | 后端原样落库；前端只展示 3 | B7 + 前端用例 |
| 重试次数 | 0 / 1 / 2 | 只允许 0（成功）或 2（1 次重试），**不出现 1 或 3+** | B10 / B11 |
| tag_ids | 空数组 | 保留为空（不报错） | B8（同形） |
| title | 单空格 / 全角空格 | 均视为空白并过滤 | B8 |
| 响应字节数 | 0（空串） | 判非法 → error | B2 |

### 不适用划除（留痕）

- **时间窗口变体**：不适用——本次改动不涉及按日期/窗口筛选，日报唯一键 (board, period_date) 语义未变（红线 1 幂等语义照旧）。
- **并发变体**：不适用——本次不新增写路径、不改 upsert 语义（红线 1 的原子 upsert 未动）；并发场景由既有日报生成闸 + 唯一约束覆盖。
- **输入变体「超长输入」**：不适用——输入是 LLM 响应而非用户输入；超长响应属既有 token 上限约束范围（`MaxTokens: 2000`），本次不改。
- **重复提交变体**：不适用——本次无新增用户可提交的写入口。
