<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

以单个 change 为生命周期回顾 harness 流程执行情况（测试执行、UI 工作流卡点、每任务成本、催修健康度）目前只能靠手写 SQL 拼装：数据 80% 已在 events.db（session.rollup×change.archive 每任务成本、ui-design-gate 五种卡点、gate.check 失败聚类、pin 复用），但 retro 报告只提供全局七段 + `--change` 过滤，没有"单 change 体检卡"视图；且实测拼装过程暴露三个数据坑（催修时距用 gate.check 转绿判解除对 ui-design-gate/quota-gate 是错配、UI 审批通过零记录只能推断、无 abandon 终态导致 40+ 活跃 change 仅 25 个有下文）。2026-09-28 探索已产出双面板趋势基线图（docs/research/change-lifecycle/retro-cost-trend-2026-09.html）与支撑度评估（docs/research/change-lifecycle/explore-findings.md）。

## What Changes

- **retro 新增 `--lifecycle <change>` 模式**：输出单 change 生命周期体检卡——流程规范段（mode.set→constraint.inject→pin 接力→gate 循环→archive 锚点）、UI 工作流段（ui-design-gate 卡点链 × 放行态）、成本卡（session.rollup 终值按 models map 归一聚合）、健康度段（block 催修时距、回归翻转、返工波次）。联合 `openspec/changes/<change>/proposal.md` 头注释元数据（ui-impact/complexity/constraint-domains，账本外的声明侧）。
- **催修时距按 policy 分派解除信号**：ui-design-gate 的 block 靠补文档解除、quota-gate 靠换 provider 解除，不再统一用 gate.check 转绿判定（实测该口径产出 89 分钟假慢）。
- **UI 审批通过显式记账**：ui-design-gate 放行侧（ui-approval 通过、ui-design 文档补齐放行）记 policy.decision，体检卡不再靠"block 之后无新 block"推断通过。
- **change 终态事件**：引入 abandon/close 语义记账（触发路径进 design 决策：bash 命令拦截对齐 change.archive 模式，或 retro 报告侧静默判定），消除"僵尸 change"盲区（有事件无 archive 无终态）。

## Capabilities

### New Capabilities

（无——体检卡是 harness-retro-loop 既有能力的新报告模式，归入修改）

### Modified Capabilities

- `harness-retro-loop`: 新增 `--lifecycle` 单 change 体检卡报告模式；催修时距指标按 policy 类型分派解除信号口径。
- `harness-fact-log`: policy.decision 词汇表新增 ui-design-gate 放行事件与 change 终态事件（abandon/close，具体 reasonCode 词汇进 design 决策并同步 smoke 断言）。

## Impact

- `scripts/harness/harness-retro.sh`：新增 `--lifecycle <change>` 模式与体检卡输出（纯只读消费，不改账本写入）。
- `.pi/extensions/ui-design-gate.ts`：放行侧显式记账。
- `.pi/extensions/harness-telemetry.ts`（或 design 决策的触发路径）：change 终态事件记账。
- `docs/reference/harness/pi-extensions.md`、`.agents/skills/harness-retro/SKILL.md`、`.agents/skills/harness-facts/SKILL.md`：报告段说明、词汇表、指标口径同步。
- 不涉及业务代码与前端 UI。
