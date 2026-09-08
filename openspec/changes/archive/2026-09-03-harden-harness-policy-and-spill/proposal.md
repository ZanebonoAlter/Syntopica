<!-- complexity: complex -->

# harden-harness-policy-and-spill

## Why

当前事实库只完整记录 quality/entry gate，spec-gate、quota-gate、test-scope-guard 的阻断、提醒、豁免与 fail-open 缺少统一可查询事实，定期 harness 健康复盘存在盲区。同时 spill 文件名仅由时间戳和工具名组成，并行同名工具存在理论覆盖风险，文件权限也未主动收紧。

## What Changes

- 新增 `policy.decision` 事实事件，统一记录 spec-gate、quota-gate、test-scope-guard 的显著决策：`block`、`warn`、`bypass`、`fail-open`；普通成功放行不全量记账，避免再次制造写放大。
- `policy.decision` 使用固定的有界 payload（policy、action、reasonCode，按需附 target/duration），保留 30 天；记账失败不得改变原门禁的放行/阻断裁决。
- spill 文件名加入基于 `toolCallId` 的短哈希，避免同毫秒同工具并行结果覆盖。
- 新建 spill 会话目录与文件分别使用 `0700` / `0600` 权限（Windows 上按平台能力 best-effort），不迁移既有文件。
- 补充 smoke 回归与事实库文档；不引入定时 Health Review、行为 Benchmark、第二套 Trace 或 spill 文件大小上限。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `harness-fact-log`：扩充 `policy.decision` 事件词汇、保留期、显著决策记账与低噪声约束。
- `tool-output-spill`：增强 spill 文件唯一命名和本地权限约束。

## Impact

- 代码：`.pi/extensions/lib/harness-log.ts`、新增或复用共享 policy 记账 helper、`.pi/extensions/spec-gate.ts`、`.pi/extensions/quota-gate.ts`、`.pi/extensions/test-scope-guard.ts`、`.pi/extensions/tool-output-spill.ts` 及对应 smoke。
- 文档：`AGENTS.md` 的扩展全景、`.agents/skills/harness-facts/SKILL.md`、`docs/reference/` 中相关 harness 说明。
- 数据：events 表仍为 TEXT kind + JSON payload，无 SQLite schema 迁移；旧事件与旧 spill 文件保持不变。
- 产品 API、前后端业务行为和数据库业务数据均不受影响。
