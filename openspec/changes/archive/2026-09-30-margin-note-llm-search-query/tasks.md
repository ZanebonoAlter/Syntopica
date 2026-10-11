# Tasks: 页边注问答 LLM 自由选词搜索

## 1. Service 层实现（margin_notes_qa.go / margin_notes_websearch.go）

- [x] 1.1 query 生成调用层（design D1/D2/D6/D8）：`margin_notes_websearch.go` 新增 `marginNoteQueryGenFn` 包级可替换钩子（对齐 `marginNoteChatFn`/`marginNoteWebSearchFn` 先例），内部走 airouter `open_notebook`（operation `daily_report.margin_note_search_query`，`maxTokens` 128、temperature 0.1、JSONMode+JSONSchema `{"queries": string[]}`）；解析容错（失败/超时/非 JSON/空数组/全空白/超 3 条截前 3，单条超长截 ≤60 runes）；prompt 依 D6 要点（问题语义优先、精炼词组、不输出解释）。验证：`go test ./internal/topicgraph/ -run TestMarginNoteQueryGen` 全绿（stub 钩子断言 schema/operation/截断行为）。
- [x] 1.2 端点预检 + 主链路接线（design D3/D5）：`AskMarginNote` 联网链路入口先调 `marginNoteSearxngEndpoint()`——未配置/禁用直接跳过（不发起 query 生成，行为与现状一致）；已配置则先 query 生成：成功出 1~3 条 → 并行搜索；失败/空 → 静默回退锚文本 query（复用 `MarginNoteSearchQuery` 现状规则）单 query 搜索。验证：`go test ./internal/topicgraph/ -run TestAskMarginNote` 降级树分支用例全绿。（实现注：端点预检经 `marginNoteSearxngEndpointFn` 包级钩子暴露，测试可控，B1 可断言零调用。）
- [x] 1.3 多 query 并行搜索与合并（design D4）：1~3 个 query 各自 goroutine 并行调 `searxng.Search`（单次超时 8s 不变，最坏墙钟 8s）；单个 query 失败/空不拖垮其他；全部失败 → 无联网参考块（静默，不再回退锚文本——SearXNG 同轮已挂）；结果按 URL（trim 精确匹配）去重保序（query 顺序 × 各自结果顺序），截前 5 进 prompt。验证：`go test ./internal/topicgraph/ -run TestMergeSearchResults` 覆盖去重保序/部分失败/全失败。
- [x] 1.4 白名单池接线：`filterWebSourcesAgainstCandidates` 的候选集从「单 query 结果」改为「合并结果池」（函数逻辑不变，调用点传入合并池）；prompt 联网参考块与红线措辞照旧。验证：既有白名单剔除用例（编造 URL 集外剔除）在新池子下仍绿。

## 2. 测试（complex 档：test-cases.md 主链路 + 白盒附加全分支）

- [x] 2.1 query 生成与解析变体单测（test-cases §单测 QG 组）：成功 1 条/3 条、返回 5 条截 3、含空串与纯空白条目过滤后为空 → 回退、非 JSON 解析失败 → 回退、锚文本+问题+线索上下文正确拼入 prompt。验证：`go test ./internal/topicgraph/ -run TestMarginNoteQueryGen -v` 与用例清单一一对应。（落点：`TestMarginNoteQueryGen_ParseVariants` + `TestMarginNoteQueryGen_GenerateFlow`，锚文本 200 边界在 GenerateFlow 覆盖。）
- [x] 2.2 降级决策树白盒用例（test-cases §白盒附加 分支表全分支）：endpoint 未配置跳过（断言 query 生成钩子零调用）、生成成功 × 搜索（全成/部分成/全败）、生成失败 → 回退锚文本 × 搜索（成/败）、生成空 → 同回退。验证：`go test ./internal/topicgraph/ -run TestAskMarginNoteDegrade -v` 每分支有落点。（B1~B8 子测试全落 + `TestAskMarginNote_QueryChangesWithQuestion` 覆盖搜索词随问题语义变化。）
- [x] 2.3 旧用例继承与调整（test-cases §继承与调整表逐行处置）：`margin_notes_qa_test.go` 既有联网用例显式 stub `marginNoteQueryGenFn`（防「未 stub 走默认 airouter 失败→静默回退锚文本」的意外绿路径），逐用例核对断言仍锁目标行为；既有非联网用例确认零影响。验证：`go test ./internal/topicgraph/ -count=1` 全绿且无意外回退路径（日志断言无 query 生成 WARN）。（实现：`swapMarginNoteChat` 默认 stub endpoint=false + queryGen err 记录器，非联网用例零调用即无回退；联网两用例改显式三钩子布置，白名单剔除改合并池断言。）
- [x] 2.4 handler 集成回归：现有 margin_notes handler 测试全量跑（`POST /annotations/:id/questions` 响应形状不变）。验证：`go test ./internal/topicgraph/handler/ -count=1` 全绿。

## 3. 文档

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: database=仅 margin_notes_qa_test.go 内既有 SQLite 测试 helper 的 AutoMigrate 被启发式误扫；本 change 零 migration（proposal 明确无 DB 变更） -->

- [x] 3.1 flow 文档对齐（doc-impact 见上方 marker）：`docs/reference/flow/daily-report.md` 红线 25 措辞更新——「单次调用契约」改「回答单次调用 + 前置 query 生成轻量调用（operation 区分审计）」、搜索 query 来源改「LLM 自由生成（1~3 条）+ 失败回退划词文本」、白名单口径改「合并结果池」；代码入口节补 `marginNoteQueryGenFn` 钩子；变更溯源表追加本 change 行。`docs/reference/flow/ai-summary.md` 能力绑定行补 operation `daily_report.margin_note_search_query`。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/margin-note-llm-search-query` 对账通过 + grep 关键词无旧措辞残留。（旧措辞仅存于历史溯源表行，历史记录不改写。）
- [x] 3.2 归档同步检查：`bash scripts/harness/check-standards.sh` 相关段通过（约束节红线句格式不变则零改动）。（实跑 222/222 通过。）

## 4. 验证

- [x] 4.1 `bash scripts/harness/change-scope.sh` 机械判定影响包（预期 `internal/topicgraph`），按判定结果只跑影响包：`go test ./internal/topicgraph/... -count=1`，期望全绿。（判定命令：`cd backend-go && go test -short ./internal/topicgraph/...`；实跑三包全绿，未加 -short 同样全绿。）
- [x] 4.2 `cd backend-go && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/...`，期望 0 error。（实跑 0 issues。）
- [x] 4.3 真机走查（SearXNG 已配置，deploy 后 :5100）：同一批注锚点先后问两个语义不同的问题（如「蓝白合是什么」→「2024 大选对两岸关系什么影响」），`ai_call_logs` 查 operation=`daily_report.margin_note_search_query` 两条记录的 queries 互不相同且贴合各问题语义（人工：psql 查询留证据）；停 SearXNG 容器再提问 → 问答照常、无网络来源行、无报错（静默降级零回归）。（2026-09-30 实测：批注 15 两问 queries 零重叠且各含核心实体——问「蓝白合是什么」生成 [蓝白合是什么/蓝白合 2026 地方选举/蓝白合 民进党 沈伯洋]，问「2026 地方选举对两岸关系什么影响」生成 [2026 台湾地方选举对两岸关系影响/民进党 2026 地方选举策略 两岸关系/沈伯洋 2026 选举 两岸议题]，ai_call_logs id 2095438/2095440；停 SearXNG 复问 → 回答照常、web_sources=[]、3 query 并行逐条 Warn、全败不回退锚文本（B4 日志实证）；另首轮后端 502 时意外实证 B6：query 生成失败→回退锚文本搜索成功进 prompt。）
- [x] 4.4 归档门禁：`bash scripts/harness/test-patrol.sh --report` 无本 change 未还欠账；`bash scripts/harness/archive-readiness.sh margin-note-llm-search-query` 无未解决问题（实际归档另获用户指令）。（实跑：test-patrol open 欠账均为其它 change 历史遗留；archive-readiness 四项全绿。）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 提问获得带引用回答 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go, backend-go/internal/topicgraph/handler/margin_notes_handler_test.go |
| 无文章上下文时明示 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 生成失败可重试不丢问题 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go, backend-go/internal/topicgraph/handler/margin_notes_handler_test.go |
| 追问 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 搜索词随问题语义变化 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| query 生成失败回退划词搜索 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 联网补强回答 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 搜索失败静默降级 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 编造 URL 白名单剔除 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 纯模型知识标注口径 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
