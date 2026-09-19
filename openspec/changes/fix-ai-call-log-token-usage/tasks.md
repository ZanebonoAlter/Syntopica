<!-- doc-impact: flow, standard -->

# Tasks — fix-ai-call-log-token-usage

## 1. TokenUsage 双键族解析（design D1）

- [x] 1.1 `airouter.TokenUsage` 增加自定义 `UnmarshalJSON`（内部 wire struct：标准键 `prompt_tokens`/`completion_tokens`/`total_tokens` 用 `*int`，兼容历史键 `prompt`/`completion`/`total`；不加自定义 MarshalJSON，存储形状保持 `{"prompt":N,"completion":N,"total":N}`）。验证：`cd backend-go && go test ./internal/platform/airouter -run TokenUsage` 全绿，覆盖三用例——标准键解析非零、历史键兼容、缺键/缺 usage 不报错
- [x] 1.2 确认存储形状未变：既有 `TestEncodeTokenUsage` 与 chat 成功日志的 token_usage 序列化仍为 `prompt`/`completion`/`total` 键。验证：`cd backend-go && go test ./internal/platform/airouter -run TestEncodeTokenUsage` 通过

## 2. Embedding 用量管道（design D2）

- [x] 2.1 `EmbeddingResult` 增加 `Usage *TokenUsage` 字段；`openAICompatibleClient.Embed` 响应解析 struct 增加 `Usage *TokenUsage`（吃 1.1 的双键解析）。验证：`cd backend-go && go test ./internal/platform/airouter -run Embed` 全绿，新增用例——embed 响应含 `prompt_tokens`/`total_tokens` 时 `EmbeddingResult.Usage` 非零
- [x] 2.2 `Router.Embed` provider 成功路径 `LogCall` 增加 `TokenUsage: encodeTokenUsage(res.Usage)`；缓存命中路径保持不设置（NULL）。验证：router 层单测——成功调用写入的日志 token_usage 非零、缓存命中行为 NULL（`cd backend-go && go test ./internal/platform/airouter`）

## 3. 失败与无 usage 兜底（design D3）

- [x] 3.1 router 层单测固化：调用失败或成功但响应无 usage 块时，落库 token_usage 为 NULL、不产生全零 jsonb。验证：`cd backend-go && go test ./internal/platform/airouter -run 'Chat|LogCall'` 全绿

## 4. 测试（§11 固定尾节）

- [x] 4.1 影响包测试：`bash scripts/harness/change-scope.sh` 判定影响包，仅跑受影响包（预期 `internal/platform/airouter`；`internal/admin/handler` 未改动不跑）。验证：`cd backend-go && go test ./internal/platform/airouter` 全绿
- [x] 4.2 lint/vet/build：`cd backend-go && golangci-lint run ./internal/platform/airouter && go vet ./internal/platform/airouter && go build ./...` 全部通过

## 5. 文档（doc-impact: ai-summary）

- [x] 5.1 `docs/reference/flow/ai-summary.md` 变更溯源表补一行（本 change：token_usage 双键解析修复 + embedding 用量管道补齐）。验证：表格新行含 change 链接，`openspec validate` 不因文档引用报错
- [x] 5.2 `docs/reference/standard/backend/ai-logging.md` 核对 token_usage 行表述（R2 表格 `prompt_tokens / completion_tokens / total` 与解析行为一致，如需补一句「存储形状为 prompt/completion/total、解析接受标准键」则改）。验证：人工核对无新旧表述冲突

## 6. 验证（§11 固定尾节，每条 = 命令 + 期望结果）

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/platform/airouter` → 无新增告警
- [x] 6.2 `cd backend-go && go vet ./internal/platform/airouter` → 无输出（通过）
- [x] 6.3 `cd backend-go && go build ./...` → 构建成功无报错
- [x] 6.4 `cd backend-go && go test ./internal/platform/airouter` → 全部 PASS
- [x] 6.5 真实调用端到端（需后端运行中）：触发任一 AI 调用（如文章打标或 embedding 任务）后执行 `docker exec syntopica-postgres psql -U postgres -d syntopica -c "SELECT capability, token_usage FROM ai_call_logs WHERE token_usage IS NOT NULL AND created_at > now() - interval '5 minutes' ORDER BY id DESC LIMIT 5"` → 新增 chat 行 total > 0；embedding 行若为 provider 直连调用则非 NULL，缓存命中行保持 NULL
