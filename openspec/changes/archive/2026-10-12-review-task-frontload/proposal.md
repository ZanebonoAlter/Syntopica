<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

2026-10-12 daily-report-lifeline-heal 复盘：实现全部完成后才发现执行规范 §0.6 步骤 4 的 review 未执行——根因是 tasks.md 从建立阶段起就没有 review 任务行，apply 与门禁全程无任何机械提醒，直到归档 spec-gate 检查⑤（review-trace 痕迹对账）才第一次拦截，此时返工成本最高（审查发现的 Medium 需要补测试 + 改制品）。现有防线全部压在归档端：review-trace 只对账「review 干没干」，不检查「计划里有没有 review」；§0.6 步骤 4 是纯文字纪律，无机械触点。

## What Changes

review 前移三层（复杂度分档：**complex 强制，simple 档行为不变**）：

1. **建立时注入（propose 层）**：openspec-propose / openspec-new-change 的 tasks 生成约定加规则——complex 档 tasks.md SHALL 含「Review」节，至少一条 review 任务行（派子线程聚焦 review + review.md 建账，或纯文档类 proposal 头部 `<!-- review: waived -->` 豁免）；约定权威源落 `docs/reference/开发执行规范.md` §0.6，两份 skill 同步。
2. **apply/归档校验（机械层）**：`scripts/harness/check-tasks-tail.sh` 扩展——proposal.md 头部声明 `<!-- complexity: complex -->` 且无 `<!-- review: waived -->` 时，tasks.md SHALL 存在 review 任务行（`- [ ]`/`- [x]` 开头且含 review/审查），否则退出码 1 + 修复指引；smoke 测试同步。归档门禁检查③与 archive-readiness 自查共用此脚本（单一事实源），自动收紧。
3. **apply 提醒（入口层）**：openspec-apply-change skill 开头加一步「先跑 check-tasks-tail.sh」——红了先补 review 任务再实现，把拦截点从归档提前到 apply 入口；扩展层（spec-gate.ts / quality-gate.ts）不改，不新增门禁通道。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `review-trace-gate`：新增「tasks review 任务行前移检查」Requirement——complex 档 change 的 tasks.md SHALL 从建立阶段起携带 review 任务行（或 proposal 头部 `<!-- review: waived -->` 豁免）；`check-tasks-tail.sh` SHALL 机械判定，判定并入归档门禁检查③与 archive-readiness 自查（单一事实源）；simple 档与无 complexity 声明的 change SHALL NOT 受影响。

## Impact

- **harness 脚本**：`scripts/harness/check-tasks-tail.sh` + `check-tasks-tail.smoke.sh`——新增判定约 20 行，向后兼容（非 complex change 判定不变）
- **skill**：`.agents/skills/openspec-propose/SKILL.md`、`.agents/skills/openspec-new-change/SKILL.md`（tasks 生成约定注入）、`.agents/skills/openspec-apply-change/SKILL.md`（开头先跑 check-tasks-tail）
- **文档**：`docs/reference/开发执行规范.md`（§0.6 tasks 模板约定 + 步骤 4 前移说明 + §11 门禁③描述）、`docs/reference/harness/pi-extensions.md`（spec-gate 检查③描述联动）
- **扩展**：无改动（spec-gate ③ 调用同一脚本自动吃到新判定）
- **存量 change**：不豁免（仿 add-review-trace-gate 先例）——存量 complex in-flight change 归档前补一行 review 任务勾掉即可，成本一行；已归档 change 不回溯
