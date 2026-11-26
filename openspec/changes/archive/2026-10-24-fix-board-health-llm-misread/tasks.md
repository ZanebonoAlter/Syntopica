# Tasks: fix-board-health-llm-misread

## 1. 评分判据校准

- [x] 1.1 `boardhealth/llm.go` boardHealthSystemPrompt 三处校准：①「分化」判据加绝对锚（双条件：low_sim_ratio 大面积偏高 且多数达标标签 cosine_mean < 0.6）；②短实体标签条款（人名/地名/技术名词语义聚焦，cosine_mean 低于泛概念标签属正常形态）；③reason_dist 条款限定（composite 仅板挂组合标签时可能，无组合挂载全单标签路径是常态；该病征须弱路径畸高且 ghost 占比同时偏高）。验证：`cd backend-go && go build ./...` 通过

## 2. 测试

- [x] 2.1 prompt 防回归断言（字符串级，4 条锚定条款 + 既有 insufficient 条款不回退）：新建 `llm_test.go`（TestSystemPromptAnchors，5 组锚）断言 system prompt 含双条件/短实体/composite 常态/insufficient 不扣分各条款。验证：`go test ./internal/tagmanagement/service/boardhealth -run TestSystemPromptAnchors -v` 通过（review M1：锚命令原写 TestPrompt 空转，已修正为精确名）

## 3. 文档

<!-- doc-impact: semantic-board -->

- [x] 3.1 `docs/reference/flow/semantic-board.md` 无需改动（评分判据在红线 22 无对应条目——22 只锁统计口径与降级语义，本次只改 LLM 消费层判据）。验证：人工确认（判据校准不触碰统计口径红线）

## 4. 验证

- [x] 4.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/service/boardhealth/...`，期望 0 issues
- [x] 4.2 `cd backend-go && go test ./internal/tagmanagement/service/boardhealth`，期望 ok
- [x] 4.3 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-board-health-llm-misread`，期望退出码 0
- [x] 4.4 效果核对：`cd backend-go && go run ./cmd/admin board-checkup` 后查 `board_health_snapshots` 最新轮：俄乌冲突(2165)/卫生健康(246030)/中国国内(2272) 评分 ≥ 60（相对垫底但绝对健康的板回归真实档位）。验证：SQL 留痕本节
  - 实测（2026-10-22 22:49）：假低分板全部回归——俄乌 55→92、开发者 58→85、日本 65→85、卫生 55→72、美国 58→72、影视 58→68、中国国内 58→65；健康板稳定——生成式AI 82→82、中东 75→75
  - 科技巨头 65→72：同统计跨 prompt 版本升档，显式豁免论证（review M2）——ghost 177/439 全部为 mismatch 行，已被隔离机制兜底（不进日报、前端默认隐藏、不进标签级统计样本分布的构成侧），「幽灵匹配占比高」病征在本板已被中和，72 属「总体健康有小瑕疵」档的合理判定，非判据放松
  - 刚果疫情 90→60：insufficient 误读残留（review H1）——降档评语「所有标签样本不足无法评估」违反「insufficient 不扣分」锚，本次三处校准未覆盖此型；prompt 已补强（全 insufficient 板按剩余真实指标定档，证据缺失不得进入评语/降档理由），待下轮体检复验回升留痕
  - 复验（2026-10-22 23:xx 第二轮）：刚果 60 持平未滑入 <60 病征档（prompt 锚起效的底线守住），但评语仍提「证据不足」（LLM 对全 insufficient 板的评语惯性，约束力有限）；根治需结构性方案（全 insufficient 板跳过 LLM 评分或固定档），超出本 change 范围留待另立；其余板 ±5 内波动（俄乌 92、生成式AI 82 核心锚稳定）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| LLM 产出评分与评语 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| LLM 失败降级不丢统计 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 相对垫底但绝对健康的标签不判分化 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 无组合挂载的板不因 reason_dist 判病 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 真分化仍被拦截 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
