<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why
按复杂度强制提交白盒用例，再按 Scenario 逐条对账，增加了测试维护和归档工作，却不能证明验证确实通过。

## What Changes
- 永久业务测试只保留 API 主用户故事与真实缺陷最小回归；白盒验证可用完丢弃。
- 关闭白盒缺件提醒和逐 Scenario 归档对账。验证阶段记录本次适用的实际结果，不设替代映射表。
- 不清理存量测试，不改产品、UI 验收或其他归档门禁。

## Capabilities
### New Capabilities
无。
### Modified Capabilities
- `case-first-testing`、`test-case-design`：测试准入和可丢弃白盒。
- `scenario-trace-gate`、`archive-readiness`、`doc-impact-gate`、`review-trace-gate`：退役逐场景归档检查。
- `ui-design-workflow`：保留 UI 验收证据，解除与 Scenario 映射的耦合。

## Impact
开发规范、Pi 归档扩展与自查脚本；不部署应用，不迁移数据。
