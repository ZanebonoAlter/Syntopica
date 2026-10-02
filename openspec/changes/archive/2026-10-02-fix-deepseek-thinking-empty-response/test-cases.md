# Test Cases — fix-deepseek-thinking-empty-response

主故事：**一次失败的 provider 调用，账目不丢观测证据，且灾区路由的思考被关掉**

- 复杂档：simple（配置修正 + 观测补强，无算法/状态机）
- 白盒附加：不适用（无复杂分支拓扑；ProviderError 载荷为直通字段）

## ⓪ 继承与调整（契约 M：ai-logging「AICallLog 完整记录字段」）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 无 usage 块不落全零 | MODIFIED（失败+带 usage→落库；无 usage→NULL 不变） | `backend-go/internal/platform/airouter/openai_compatible_test.go` | 改断言：拆两态——无响应体失败仍 NULL（保留原断言），新增"HTTP200+usage+空content 失败→落库"用例 |
| token 用量记录 | 继承（成功路径不变，reasoning 为可选附加键） | `openai_compatible_test.go` / `router_test.go` | 继承原用例 + 新增 reasoning 细分断言 |
| 完整 prompt 落库 | 继承（不动） | `router_test.go` | 继承 |
| 历史行回填 / operation 列约束 | 继承（不动） | 历史 change 资产 | 继承 |

## 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | mock provider 返回 HTTP 200 + content 空 + usage{1536 全 reasoning} + finish_reason=length | 失败但响应携带 usage 块落库 | ProviderError{empty_response, Retryable} 携带 Usage 与 FinishReason="length" | 函数单测 | `openai_compatible_test.go` |
| 2 | router 走降级链，首档空响应失败 | 失败行携带 finish_reason | 失败行 `success=false`、`token_usage`={"prompt","completion","total","reasoning"}、`response_snippet` 前缀 `finish_reason=length` | 函数单测 | `router_test.go` |
| 3 | mock provider 返回 http_502（无响应体） | 无 usage 块不落全零 | 失败行 `token_usage` NULL、snippet 无 finish_reason 前缀 | 函数单测 | `router_test.go` |
| 4 | mock 返回成功 + usage 带 reasoning_tokens 细分 | token 用量记录 | 成功行 jsonb 基础三键 + `reasoning` 可选键 | 函数单测 | `openai_compatible_test.go` |
| 5 | 数据步骤执行后真库查 flash/v4-pro 思考档 | （数据核对，非自动化） | flash 模型级+绑定级均 off；v4-pro 保持 low | 真库 | 人工：`SELECT m.name,m.thinking_level,rm.thinking_level ...` |
| 6 | 部署后手动触发一次全版面日报，24h 后聚合 | （效果核对，task 4.1） | DeepSeek 空响应失败骤降至个位、失败行开始带 usage、账单回落 | 真库量化 | 人工：ai_call_logs 聚合 + DeepSeek 后台对照 |

## 变体走查

- 输入：content 全空白（纯空格/tab）→ 归一化为空同主链路 1（`strings.TrimSpace` 后空，用例覆盖）｜usage 块缺 completion_tokens_details → 无 reasoning 键，三键照落（步 4 补变体）
- 前置：降级链第二档成功 → 失败档行仍落 usage（IsFallback=true 行）｜链穷尽 → 逐行留痕语义不变（既有用例继承）
- 时间窗口/幂等/并发：不适用（无窗口、无重放语义；并发记账由既有 LogCall 串行路径保证）——划除留痕
- 可用性(UI)：不适用（无前端改动）——划除留痕

## 负向节拍（SHALL NOT）

| 断言 | 落点 |
| --- | --- |
| 失败且无响应体 usage → token_usage SHALL NOT 全零 jsonb | `router_test.go`（步 3 断言 NULL） |
| reasoning 附加键 SHALL NOT 破坏基础三键形状（消费方兼容） | `openai_compatible_test.go`（步 4 断言键集合） |
| Retryable 语义 SHALL NOT 因新增载荷改变（empty_response 仍走降级链） | `router_test.go`（步 2 降级链继续） |

## 效果核对（依赖外部 LLM 行为 → 真库量化）

- 触发原因：空响应率与账单金额依赖 DeepSeek 实际行为，单测不可证
- 方法：task 4.1/4.2（部署后真库聚合 + DeepSeek 后台对照）
- 量化基线（修正前，本 change 诊断数据）：digest_polish DeepSeek 日均失败 60-170 次、日均空思考烧掉 ~15-26 万 completion token、账单 ≈¥0.5-0.8/日
- 结论落点：归档前回填 task 4.1 实测数字
