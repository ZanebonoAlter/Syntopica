## 1. 健康判定改造（aihealth）

- [x] 1.1 在 `backend-go/internal/platform/aihealth/aihealth.go` 新增核心 capability 常量（`embedding` + `topic_tagging`，design D1/D4），`runProbeLocked` 判定改为「集合内每个 capability ≥1 条主 provider 可达」，移除 `hasReachableCapabilityOtherThan` 宽松 or 运算；快照明细构建、探测/拉起/心跳/去抖逻辑一律不动（design D2）——验证：`go build ./internal/platform/aihealth` 通过
- [x] 1.2 改写 `backend-go/internal/platform/aihealth/aihealth_test.go` 判定用例：按 spec 6 个 Scenario 各落一个表驱动用例（核心双路由均通 / 缺 embedding / 缺打标路由 / 云端可达不救命回归 / 打标路由未配置 / 未配置任何路由），并清理仍断言旧「两类各一条」口径的既有用例——验证：`go test ./internal/platform/aihealth` PASS

## 2. 暂停门组合测试同步（analysispause）

- [x] 2.1 `backend-go/internal/platform/analysispause/health_gate_compose_test.go`：3 个既有 compose 场景（ProbeComposes / ProbeUnhealthy / HeartbeatDegrade）的路由 fixture 按新口径调整（健康态 fixture 必须同时含可达的 embedding 与 topic_tagging），`TestHealthGate_EmbeddingUp_LLMDown_NotHealthy`（实现时改名为 `TestHealthGate_EmbeddingUp_TaggingDown_NotHealthy`）按「打标路由决定生死」改写，`TestHealthGate_SlowProviderWithinTimeout_Healthy` 保留并补齐打标路由可达前提——验证：`go test ./internal/platform/analysispause` PASS
- [x] 2.2 新增回归场景落点：embedding 可达 + topic_tagging 不可达 + digest_polish/data_enrichment_analysis 云端可达 → 有效暂停（锁定 2026-09-28 实测，spec「云端 LLM 可达不救命」）——验证：`go test ./internal/platform/analysispause` PASS（含该用例）

## 3. test-cases 制品

- [x] 3.1 产出 `openspec/changes/ai-health-gate-tagging-required/test-cases.md`（simple 档精简形：主链路表串 spec 6 场景的节拍与落点 + 变体走查 + 效果核对）——验证：文件存在，且 6 个 delta Scenario 标题逐字覆盖（grep 对账）

## 4. 测试

- `cd backend-go && go test ./internal/platform/aihealth ./internal/platform/analysispause` → PASS（判定 6 场景 + 组合 4 场景全绿；影响包由 `bash scripts/harness/change-scope.sh` 机械判定，不跑全量）

## 5. 文档

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=仅测试文件内 SQLite AutoMigrate（测试夹具），无 schema/迁移变更 -->

- [x] 5.1 `docs/reference/flow/scheduler.md` §业务约束 7「健康门维度」：判定口径由「宽松判定：≥1 embedding 路由主 provider 通 且 ≥1 llm 路由主 provider 通」改为「核心路由硬条件：embedding 主通 且 topic_tagging 主通（打标路由缺失/未绑定/不可达一律 NOT 健康，fail-closed——文章入库前置拦截）」，并注明非核心路由退出判定
- [x] 5.2 `docs/reference/flow/scheduler.md` 变更溯源表加一行（2026-09-28 / ai-health-gate-tagging-required / 一句话；archive 后按 §12.2 补链接）
- [x] 5.3 一致性检查：`docs/reference/flow/ai-summary.md` §代码入口的健康门指引仅指向 scheduler §7、未复述口径 → 不改动（若 apply 时发现其复述了旧口径则一并改并保持 doc-impact 声明为 flow）
- [x] 5.4 头部 `doc-impact-applies` 辖区补全：`scheduler.md` 原仅声明 `internal/admin/scheduler/`，健康门判定代码（`internal/platform/aihealth/`、`internal/platform/analysispause/`）不在辖区内 → JIT 注入静默失效；按 `standard/shared/doc-authoring.md`「辖区路径写全」补齐三段路径
- [x] 5.5 `docs/reference/api/ai-admin.md` `GET /api/ai/health` 的 `healthy` 字段说明：由旧口径「宽松判定——≥1 embedding 且 ≥1 llm 路由主 provider 可达」改为「核心路由硬条件——embedding 且 topic_tagging 主 provider 可达；其余非核心路由不参与判定，明细仍照常返回」（doc-impact suggest 启发式命中 api 域，经核对为真实遗漏而非误报）

## 6. 验证

1. `cd backend-go && go test ./internal/platform/aihealth ./internal/platform/analysispause` → `ok`（PASS，全部用例绿）
2. `cd backend-go && golangci-lint run ./internal/platform/aihealth/... ./internal/platform/analysispause/...` → 无违规输出，退出码 0
3. `cd backend-go && go vet ./internal/platform/aihealth ./internal/platform/analysispause && go build ./...` → 无输出，编译成功
4. `openspec validate ai-health-gate-tagging-required --strict` → 通过（0 error）
5. `grep -n "topic_tagging" docs/reference/flow/scheduler.md` → ≥1 处命中（约束节写入打标硬条件）；`grep -c "≥1 llm 路由主 provider 通" docs/reference/flow/scheduler.md` → 约束节旧口径表述清零（历史溯源行不计）；`grep -n "核心路由硬条件" docs/reference/api/ai-admin.md` → ≥1 处命中（api 域 healthy 口径同步）
6. `bash scripts/harness/doc-impact.sh verify openspec/changes/ai-health-gate-tagging-required` → 零 FAIL

| Scenario | 测试文件 |
| --- | --- |
| 两类各通一条即健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 缺 embedding 则不健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 缺 llm 则不健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 云端 LLM 可达不救命（2026-09-28 回归） | backend-go/internal/platform/aihealth/aihealth_test.go, backend-go/internal/platform/analysispause/health_gate_compose_test.go |
| 打标路由未配置不健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 未配置任何路由不健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
