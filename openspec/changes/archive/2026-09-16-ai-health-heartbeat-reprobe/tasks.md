# Tasks: ai-health-heartbeat-reprobe

## 1. 测试用例先行

- [x] 1.1 创建 `test-cases.md`：以 specs/ 两个 ADDED requirement 的 13 个 Scenario 串主链路表（步/动作/来源 Scenario/期望/层/落点），附变体走查（去抖计数边界：0/1/2 次连续失败 × 上一状态；探测成功重置；in-flight 互斥）与白盒附加节（healthy⇄not-healthy 状态机分支表 + 边界值 + 不适用划除留痕）。验证：文件存在且 13 个 Scenario 全部落点。

## 2. 核心实现

- [x] 2.1 `internal/platform/aihealth`：`StartPeriodicReprobe` 心跳循环无条件化（移除 `if Healthy() { continue }` 短路，`aihealth.go:234` 附近），新增连续失败计数——连续 2 次探测失败将快照降级 not healthy，单次成功恢复 healthy 并清零计数；常量 `heartbeatInterval=60s`、`degradeFailures=2` 落包内；复用既有全局互斥与拉起冷却，不新增探测通道。验证：`go test ./internal/platform/aihealth` 新增单测全绿（healthy 态持续探测 / 2 连败降级 / 单次成功恢复 / in-flight 跳过）。实际落点：`reprobeInterval`（保留原名，注释更新为心跳语义）+ 新增 `degradeFailures`/`failStreak` + `applyProbeVerdict` 去抖写入口径。
- [x] 2.2 `internal/platform/analysispause`：扩展 `health_gate_compose_test.go` 降级场景——快照 healthy 时 2 连败 → `IsPaused()==true`；降级后单次探通 → `IsPaused()==false`。另补「慢而活不降级」compose 证明（delay 300ms httptest + 真实 TestConnection）。验证：`go test ./internal/platform/analysispause` 全绿。
- [x] 2.3 回归确认零外溢：`git status --short` 确认本 change 触碰仅 `internal/platform/aihealth`（aihealth.go 产品码 + 3 个测试文件）与 `internal/platform/analysispause`（health_gate_compose_test.go）+ 文档/openspec 制品；未触碰 `airouter`、`admin/scheduler`、`tagmanagement`。

## 3. 测试

- [x] 3.1 白盒状态机表驱动单测（aihealth）：降级计数边界（第 1 次失败不降级、第 2 次降级、成功清零重计）、跨触发源累计（心跳×TryStartProbe + 手动×RunStartupProbe 共享 streak）、not-ready 首探 fail 不走去抖、去抖窗内明细照实更新、in-flight 互斥/拉起冷却/ctx 取消回归（既有用例保持绿）。验证：`go test ./internal/platform/aihealth -run 'Heartbeat|Degrade|Reprobe' -v` 全 PASS（12 用例）。落点：`heartbeat_degrade_test.go`（新）+ `reprobe_test.go`（旧 StopsWhenHealthy 拆为 SelfHeals/HeartbeatContinuesWhenHealthy）。
- [x] 3.2 真实效果人工核对：本地起后端 + 本地模型停止（或改 base_url 指向不监听端口），观察 ≤2 个心跳周期内 `/schedulers/status` 的 `ai_healthy` 翻 false、分析任务停租约；恢复端点后 ≤60s 翻 true。验证：人工记录时间线（test-cases.md 效果核对节留痕）。【留痕：2026-09-16 用户确认已验/信任自动化覆盖（单测+testcontainer compose 已锁降级/恢复行为），不单独留观察时间线，直接归档】

## 4. 文档

<!-- doc-impact: scheduler, ai-summary -->

- [x] 4.1 `docs/reference/flow/scheduler.md`、`docs/reference/flow/ai-summary.md`：检索"健康后停止重探/仅不健康时重探/永不主动打回"类表述，更新为双向心跳语义（含 60s/2 连败参数与 provider 超时忙容忍说明）。变更溯源表历史条目（2026-08-19 行）属史实不改，归档时按 §12 追加本 change 新行。验证：`grep -rn "健康即停\|永不.*打回\|不再定时探测\|停止定时重探" docs/reference/flow/` 仅余变更溯源表历史行。

## 5. 验证

- [x] 5.1 `cd backend-go && go test ./internal/platform/aihealth ./internal/platform/analysispause` → 全部 PASS（ok 1.6s / ok 7.9s，含 testcontainer 集成）。
- [x] 5.2 `cd backend-go && golangci-lint run ./...` → 0 issues（全仓）。
- [x] 5.3 `cd backend-go && go vet ./... && go build ./...` → 均退出码 0。
- [x] 5.4 `grep -rn "if Healthy() { continue }" backend-go/internal/platform/aihealth/` → 零命中（心跳无条件化的机械锚）。

| Scenario | 测试文件 |
|---|---|
| 启动时探测每条路由主 provider | backend-go/internal/platform/aihealth/aihealth_test.go |
| 仅探主 provider 不探 fallback | backend-go/internal/platform/aihealth/aihealth_test.go |
| 无 provider 的路由跳过 | backend-go/internal/platform/aihealth/aihealth_test.go |
| ListRoutes 瞬态失败时重试 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 快照健康后仍按心跳周期复检且可降级 | backend-go/internal/platform/aihealth/reprobe_test.go backend-go/internal/platform/aihealth/heartbeat_degrade_test.go |
| 快照不健康时定时复检直至自愈 | backend-go/internal/platform/aihealth/reprobe_test.go |
| 健康态心跳持续探测 | backend-go/internal/platform/aihealth/reprobe_test.go |
| 端点秒拒连续两次即降级 | backend-go/internal/platform/aihealth/heartbeat_degrade_test.go backend-go/internal/platform/analysispause/health_gate_compose_test.go |
| 忙服务器不被误降级 | backend-go/internal/platform/analysispause/health_gate_compose_test.go |
| 降级后自动恢复 | backend-go/internal/platform/aihealth/heartbeat_degrade_test.go backend-go/internal/platform/analysispause/health_gate_compose_test.go |
| 慢加载模型加载完成后自动自愈 | backend-go/internal/platform/aihealth/reprobe_test.go |
| 探测 in-flight 时定时触发被跳过 | backend-go/internal/platform/aihealth/reprobe_test.go |
| 心跳失败触发拉起且遵守冷却 | backend-go/internal/platform/aihealth/aihealth_launch_guard_test.go |
