<!-- complexity: simple -->

# scope-archive-standards-check

## Why

归档时的 `check-standards.sh` 会遍历全部 active change 的 doc-impact 状态，导致目标 change 已通过自身对账时仍被其他在途 change 阻断。归档门禁应继续检查仓库级规范，却不能把无关 change 的未完成文档对账当成目标 change 的失败。

## What Changes

- 为 `scripts/check-standards.sh` 增加可选的目标 change 参数，使 F 段仅校验该 change 的 doc-impact；未传参数时保持现有全仓巡检行为。
- 让 spec-gate 在归档预检时传入目标 change，保留目标自身 doc-impact、任务结构、Scenario 映射与其他仓库级标准检查。
- 增加脚本/门禁 smoke，验证无关 active change 失败不阻断目标归档，目标自身失败仍会阻断。
- 更新归档门禁规范与测试映射。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `doc-impact-gate`: 归档命令硬门禁的 standards 对账从全 active change 扫描收敛为目标 change 范围，手动全仓巡检行为保持不变。

## Impact

- `scripts/check-standards.sh`、`.pi/extensions/spec-gate.ts` 与对应 smoke 测试。
- `docs/reference/开发执行规范.md`、`openspec/specs/doc-impact-gate/spec.md`。
- 不影响产品 API、业务数据、数据库结构或日常手动全仓规范检查。
