# Seam A2 交接 — review Low 两修极小批次（2026-10-28）

> 授权：父线程 A2 批次（修复 seam A 独立审查 A-L1/A-L2）。实际修复 run：96da6c9d-1e8f-4315-812d-a3b819be4a20（源会话盘点 run：9bc43d6e-a961-4ba9-b43a-ba45ef343aab），model=zai-coding-cn/glm-5.3:max。未重写/未重跑 seam A 原批（原始成功日志与恢复 hash 由父线程核实）；B1 已另行交接，未触碰。

## 改动文件（写入白名单内）

- `backend-go/internal/tagmanagement/service/boardhealth/boardhealth.go`：`parseFloatDefault` 增非有限值守卫（A-L1）。
- `backend-go/internal/tagmanagement/service/boardhealth/run.go`：`convertTriage` 非 rewrite 清空 Suggestion（A-L2）。
- `backend-go/internal/tagmanagement/service/boardhealth/review_regression_test.go`：新增纯函数回归（无 DB）。
- `openspec/changes/board-health-triage-advice/review.md`：A-L1/A-L2 验证列置已修并附命令证据。
- `openspec/changes/board-health-triage-advice/seam-a-handoff.md`：追加「A2 增量回归」节（未改原文其余内容）。

未动：B1 模型/repository/migration/handler/前端/tasks 2.*、其他任务 checkbox（todo12 由父线程管理）、存量 board_health_migration_test.go、任何 Git 写操作（零 stash/add/commit）。

## 修 1 — parseFloatDefault 非有限值守卫（A-L1）

- 变更：`err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 100` → 回退传入 fallback。
- 根因：NaN 与任何比较为 false，可绕过原 0..100 边界被静默当作阈值（磁铁 gap 等配置失效）。±Inf 原本会被边界拦住，一并显式拒收。
- 语义不变项：正常数值含 0/100 边界原样通过；越界/负数/坏文本/空串回退不变；既有四个配置键行为不变。

## 修 2 — convertTriage 清空非 rewrite suggestion（A-L2）

- 变更：仅 `category == "rewrite"` 时复制 Suggestion；其余类别落库为空（模型 omitempty → JSON 不输出 suggestion 键）。
- 语义不变项：合法类别/evidence 原样保留（不因多余文案丢弃）；rewrite 有效文案保留；输出为新 slice，输入 LLM 结果 slice 原值保持；validateTriage 校验逻辑不动（仍要求 rewrite 必有 suggestion）。

## 验证（仅本次新增纯函数测试 + 本包 lint）

1. `cd backend-go && GOMAXPROCS=2 go test -p 1 -count=1 -run 'TestParseFloatDefaultNonFiniteGuard|TestConvertTriageClearsNonRewriteSuggestion' ./internal/tagmanagement/service/boardhealth` → **ok**（-v 复核：TestParseFloatDefaultNonFiniteGuard 20 子用例全 PASS——NaN 大小写/带符号/首尾空白、Inf/+Inf/-Inf/-INFINITY/" Infinity "、边界 0/0.08/100、区间内 45.5、越下界/越上界/负数/坏文本/空串；TestConvertTriageClearsNonRewriteSuggestion PASS——keep/remove/insufficient 多余文案清空、类别/证据保留、rewrite 保留、JSON 省略非 rewrite suggestion、输入 slice 原值保持）。
2. `golangci-lint run ./internal/tagmanagement/service/boardhealth/` → **0 issues**。
3. gofmt 仅触碰的三文件（gofmt -l -w），无全目录格式化。

按授权**未**重跑：原三包 PG/short 全量套件、全域门禁、build/vet（既有绿不重复执行）。

## 剩余风险

- 无新增已知缺陷。tier2/tier3 截断优先级与异维历史向量仍属后续审查/效果观察项（review.md 原文保留）；真 LLM 分诊效果仍按 tasks 5.4 核对，不由桩测试替代。
