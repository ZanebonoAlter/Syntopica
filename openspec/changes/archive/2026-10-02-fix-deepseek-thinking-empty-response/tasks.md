<!-- doc-impact: flow, standard -->
<!-- doc-impact-excuse: api=session_handler_test.go 仅新增聚合口径锁定测试，无端点/契约改动；GET /api/ai/sessions 文档口径「所有 call_logs 的 token_usage 之和」本就未限定 success，修正后行为与文档一致，同目录其余 handler 脏文件属其它 change（guard-daily-report-generate）; database=无 schema 变更（proposal 明确），启发式命中来自测试文件的 AutoMigrate 测试基建（setupSessionTestDB） -->

## 1. ProviderError 观测载荷（代码）

- [x] 1.1 `ProviderError` 增加可选载荷字段 `Usage *TokenUsage` 与 `FinishReason string`（零值语义不变，Retryable 逻辑不动）；空响应归一化与结构化 parse_error 构造点（均位于 `router.go`，`ChatResponse` 增 `FinishReason` 观测字段作上行通道）携带 `parsed.Usage` 与 `choices[0].finish_reason`。验证：`go test ./internal/platform/airouter/ -run 'TestChatParsesFinishReason|TestRouterEmptyResponseFailureRow|TestRouterParseErrorFailureRow'` 全绿（HTTP200+usage+空 content → err 携带二者；网络错误 → 均为空由 TestRouterHTTP502 覆盖）。
- [x] 1.2 `TokenUsage` 解析支持 `completion_tokens_details.reasoning_tokens` → 附加 `reasoning` 可选键（基础三键形状不变）。验证：`TestTokenUsageUnmarshalJSONKeyFamilies`（含 4 个 reasoning 子用例）+ `TestTokenUsageMarshalReasoningOptionalKey` 断言 jsonb 输出形状。

## 2. Router 失败记账（代码）

- [x] 2.1 `router.go` 降级链失败档逐行审计与最终失败行的 `LogCall`：`TokenUsage` 取 `ProviderError.Usage`（有则落库，无则 NULL 保持），`response_snippet` 前缀 `finish_reason=<值>` 后接原错误文本。验证：`TestRouterEmptyResponseFailureRowCarriesUsageAndFinishReason`（空响应失败行四键+snippet 前缀）、`TestRouterHTTP502FailureRowUsageNullAndNoFinishReason`（http_502 失败行 NULL）、`TestRouterParseErrorFailureRowCarriesUsage`（parse_error 同携）全绿；降级链逐行审计为 4xx 档天然无响应体 usage，行为不变（既有 structured_degrade 用例继承）。
- [x] 2.2 核对 session 聚合口径：聚合对失败行 usage 计入总量（修正前失败行恒 NULL 故等价只含成功行；修正后计入 = 账实相符，空思考烧掉的 token 在 session 视图可见，与本 change 动机一致）；`tokenUsageAgg` 只读基础三键，reasoning 附加键被 json.Unmarshal 忽略不破坏兼容；airouter observability（stats/events）无 token 聚合不受影响。验证：`go test ./internal/admin/handler/ -run TestGetSession` 全绿（新增 `TestGetSession_FailureRowUsageCountedAndReasoningKeyIgnored` 锁定口径）。

## 3. 思考档配置修正（数据步骤）

- [x] 3.1 部署后执行：`UPDATE ai_models SET thinking_level='off' WHERE id=16 AND name='deepseek-flash';`（2026-10-01 执行，UPDATE 1，前置留底 low）；digest_polish×deepseek-flash 绑定行（id=63）核对时已是 `off` 无需再改。**v4-pro（id=17，模型级 low 保持，绑定 NULL 继承）与 data_enrichment_analysis 路由未动**。验证：真库查询确认 flash 模型级+绑定级均 off、v4-pro 保持 low。

## 4. 效果核对（真库量化）

- [x] 4.1 部署后下一次日报生成（手动触发一次全版面）观察 24h：digest_polish 路由 DeepSeek 空响应失败数（预期从日均 ~60-170 骤降至个位数以下）、失败行 token_usage 开始携带 reasoning 账目、DeepSeek 后台消耗回落。方法：`ai_call_logs` 按 provider×success 聚合查询 + DeepSeek 官方账单对照。记录量化结果到本 change 目录（可追加至 proposal 或归档时补 flow 溯源）。**量化结果（2026-10-02 归档时核对）**：新代码 10-02 19:08 部署，当晚 21:06 日报全版面跑完——`daily_report.*` 五个 operation（lane_snapshot/threads/highlights/cluster_new_narrative/decide_l2_tags）DeepSeek 行**全部 success=true、零失败零降级**；对照前夜（10-01 21:06）同管线 DeepSeek 失败 157 行（lane_snapshot 78 + threads 79，error_code=empty_response，随后由 qwen 降级接管）。失败行 usage/finish_reason 记账因当晚零失败无运行时样本，由单测覆盖（见验证节映射表）。
- [x] 4.2 回归核对：data_enrichment_analysis 路由调用行为不变（thinking 仍 enabled、无新增失败）。方法：`ai_call_logs` 查询 data_enrichment_analysis 相关行 provider=DeepSeek 的 success 率与 latency 与修正前同量级。**核对口径（2026-10-02）**：该路由近 14 天零调用（订阅/按需驱动，观察窗内无自然触发），运行时对比无从做起；行为不变由 3.1 配置核对（模型级 thinking=low 与绑定均未触碰）+ 降级链逐行审计单测（既有 structured_degrade 用例继承，2.1）保证，运行时证据留待该路由下次自然触发时抽查。

## 5. 测试

- [x] 5.1 `golangci-lint run ./... && go vet ./... && go test ./internal/platform/airouter/ && go build ./...` 全绿（另加跑影响包 `go test ./internal/admin/handler/`——session 聚合口径用例，同样全绿）。

<!-- doc-impact: flow, standard -->

## 6. 文档

- [x] 6.1 flow 文档变更溯源：`docs/reference/flow/ai-summary.md` 补约束 #17（新路由绑定推理模型 SHALL 显式评估绑定级思考档，含根因描述）与 #18（失败行 usage/finish_reason 记账契约）+ 变更溯源行（归档时附 4.1 量化结果）；daily-report flow 经核未引用 airouter 降级链行为（仅代码入口一行），无需补链。`standard/backend/ai-logging.md` R2 同步失败行落库口径。验证：`doc-impact.sh verify` 通过（api/database 疑似遗漏经 excuse 豁免——测试文件误报，无契约/schema 变更）。

## 7. 验证

- [x] 7.1 门禁复跑：`archive-readiness.sh` 四项全绿（含本节映射表对账 scenario-trace）。
- [x] 7.2 Scenario → 测试资产映射（ai-logging MODIFIED Requirement）：

| Scenario | 测试文件 |
| --- | --- |
| 完整 prompt 落库 | backend-go/internal/platform/airouter/router_test.go |
| token 用量记录 | backend-go/internal/platform/airouter/router_test.go |
| 失败但响应携带 usage 块落库 | backend-go/internal/platform/airouter/router_test.go |
| 失败行携带 finish_reason | backend-go/internal/platform/airouter/openai_compatible_test.go, backend-go/internal/platform/airouter/router_test.go |
| 无 usage 块不落全零 | backend-go/internal/platform/airouter/router_test.go |
| 历史行回填 | backend-go/internal/platform/database/postgres_migrations.go |
| operation 列约束 | backend-go/internal/platform/database/postgres_migrations.go, backend-go/internal/platform/database/constraints_test.go |

（对应用例：`TestTokenUsageRecordedInCallLog`/`TestRouterEmptyResponseFailureRowCarriesUsageAndFinishReason`/`TestRouterHTTP502FailureRowUsageNullAndNoFinishReason`/`TestRouterParseErrorFailureRowCarriesUsage`/`TestChatWithoutUsageLogsNullTokenUsage`/`TestChatFailureLogsNullTokenUsage`/`TestChatParsesFinishReasonOnEmptyContent`/`formatMessages` 截断子用例；历史行回填与 operation NOT NULL 为 `ensureNotNullDefault(ai_call_logs, operation, 'unknown')` 迁移守卫，既有约束核查 `constraints_test.go` 继承覆盖。）
