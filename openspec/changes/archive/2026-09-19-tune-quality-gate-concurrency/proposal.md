<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

quality-gate 的 turn_end 门禁在「多会话共享工作树 + 4 核树莓派」环境下有两个结构性问题（数据：`.pi/harness/events.db` 2026-09-12~19，gate.check 记账 6116 条 / 111 会话）：

**1. 资源叠加（渠道 1）**：门禁命令是全仓库级重活——golangci-lint ./...（均值 2.1s / 最大 28.2s）、go vet ./...（2.0s / 34.8s）、go build ./...（2.6s / 26.0s），且 vet/build 以 Promise.all 并行执行、每个 go 进程内部默认 GOMAXPROCS=4 吃满全部核心。7 天内 **341 个分钟窗口有 ≥2 个会话同时在跑门禁**（冲突窗口均值 2.35 会话），叠加时其他并发会话正在跑的 build/test/浏览器自动化全部被拖慢；9/17（load 106 系统假死事故日）当天门禁命令累计 **1751s CPU 时间**。

**2. 跨会话误报催修（渠道 2，用户痛点主源）**：命令范围是 `./...` 全仓库而非改动包，A 会话的门禁会把 B 会话编辑中的半成品文件一并检查；B 的中间态让 A 的门禁变红，A 被 [回归] steer 催着修 B 的代码。现有外部归因（attribute-concurrent-gate-noise D7）要求失败路径**完全**属于其他会话（P∩mine=∅ ∧ P⊆foreign）才判 [外部]——7 天命中 **0 次**，最常见的混合归属（失败路径同时含本会话与其他会话文件）全部按本会话失败分级催修。失败记账高度集中在 go 侧（lint 562 / vet 465 / build 364，前端 pnpm lint 仅 5 次），与该机制预测一致。完整探索数据见 `docs/research/quality-gate-concurrency/explore-findings.md`。

## What Changes

三件事，全部落在 `.pi/extensions/quality-gate.ts`（门禁命令集、增量触发判定、粘性失败语义、归档前全绿硬要求均不变）：

1. **全局门禁互斥锁（方案 A）**：门禁命令执行前原子抢占 `.pi/harness/gate.lock`（O_EXCL 创建，内容 `{sessionId, ts, cmd}` 供考古）；抢不到 → 本轮门禁整体跳过（fail-open），显式记 `policy.decision skip gate-lock-held`，粘性失败集合保留、下回合锁空闲自然重跑催修；锁带 TTL 180s，过期视为 stale 直接覆盖，免疫进程崩溃残留。
2. **限核（方案 B）**：go 命令统一加 `GOMAXPROCS=2` 环境前缀，golangci-lint 加 `--concurrency=2`，vet+build 从 Promise.all 改串行——单会话门禁 CPU 峰值从吃满 4 核降到 ~2 核，多会话叠加留出余量。
3. **三态归因降级（方案 E-lite）**：失败归属从二值（[外部] / 本会话）扩为三态——新增**混合归属**分支（P∩mine≠∅ ∧ P∩foreign≠∅）：不标 [回归]，改标 `[并发]` 并列出他人/本会话各自的文件路径，说明「可能非本会话所致，归档前仍需全绿」；照进粘性重跑（本会话部分确实要修，回合末复检不断）；记 `policy.decision warn concurrent-mixed`。

## Capabilities

### New Capabilities
- `gate-concurrency-control`: quality-gate 在多会话共享机器上的门禁互斥（锁跳过语义）、单会话资源上限（限核）与混合归属失败降级（[并发] 分级）行为。

### Modified Capabilities

无——`gate-interop-health` / `gate-toolchain-health` 的环境故障归因语义、`concurrent-change-coordination` 的 edit.map 归属落库语义均不变；三态归因复用既有 mine/foreign 集合构建（step 3.7），只改判定分支。

## Impact

- 改动文件：`.pi/extensions/quality-gate.ts`（主）、`.pi/extensions/tests/`（白盒用例）、`docs/reference/harness/pi-extensions.md`（机制文档同步：锁/限核/三态归因）。
- 部署后行为变化（用户可见）：并发会话同时跑门禁时后到者本轮静默跳过（账本可见 skip 记录）；门禁命令变慢但不再拖垮机器；混合归属失败不再以 [回归] 必须修的措辞催修，改 [并发] 提示（归档前全绿硬要求不变）。
- 旧数据降级：无——锁文件是新产物，events.db 新增 reasonCode（gate-lock-held / concurrent-mixed），旧记录不受影响。
- 无需用户手动操作。
