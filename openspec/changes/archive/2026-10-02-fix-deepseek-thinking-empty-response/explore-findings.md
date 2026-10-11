
## DeepSeek 空响应根因与修复改动点

根因：deepseek-flash 模型 thinking_level=low（ai_models id=16）→ thinkingToWire("deepseek","low") 发 thinking={"type":"enabled"}，DeepSeek 推理模型 max_tokens 是含 reasoning 的输出总预算，思考烧满预算后 content 空（finish_reason=length）→ router.go:271 判 empty_response（retryable）→ fallback 本地 qwen；DeepSeek 侧 1536 token 照常计费。复现：并发 20 路真实 prompt 重放 18/20 空（completion_tok=1536=reasoning_tok）。

改动点：
1. ProviderError（router.go 定义处）加可选载荷 Usage *TokenUsage + FinishReason string，Retryable 语义不动
2. openai_compatible.go:263-315：空响应归一化（router.go:271）与结构化 parse_error 路径构造错误时带 parsed.Usage 与 choices[0].finish_reason；网络/HTTP 错误路径天然无值
3. TokenUsage 解析支持 completion_tokens_details.reasoning_tokens → jsonb 附加 "reasoning" 键，基础三键 {"prompt","completion","total"} 形状不变
4. router.go 失败记账两处（降级链逐行审计 ~L240-263 + 最终失败行）：LogCall 的 TokenUsage 取 err.Usage，response_snippet 前缀 "finish_reason=<值>"
5. 数据步骤：UPDATE ai_models SET thinking_level='off' WHERE id=16 AND name='deepseek-flash'；digest_polish×deepseek-flash 的 ai_route_models 绑定级补 thinking_level='off'。v4-pro（id=17，data_enrichment_analysis 路由需要思考）不动
6. 模型同步（store_vendor.go SyncModels ~L454）只更新 last_seen_at，不回洗 thinking_level——配置持久

旧测试资产（契约 M，ai-logging）：token用量记录→openai_compatible_test.go+router_test.go；无usage块不落全零→openai_compatible_test.go（其"失败→NULL"断言要拆两态：无响应体失败 NULL 保留，新增 HTTP200+usage 失败→落库）。

验证影响包：go test ./internal/platform/airouter/（change-scope.sh 口径）

<!-- pinned 2026-10-01T13:28:07Z -->
