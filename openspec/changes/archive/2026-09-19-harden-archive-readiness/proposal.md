<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-19 harness 体检（`--days 2`，基线 `.pi/harness/retro-baseline-2d-0919.json`）：归档门禁 block 复发 5 组（`m7.block_recur_groups=5`），考古 18 条 `archive-check-failed` 事件——`harden-subagent-constraint-channel` 83 分钟内 7 连试、`improve-discovery-recommendations` 5 分钟内 7 连试，根因清一色 `trace`（scenario-trace 对账）与 `tasks` 尾三节未补齐。spec-gate 的 block reason 其实已携带脚本输出尾部 20 行（缺什么列得清清楚楚），但**归档就绪检查只在 `openspec archive` 时刻存在**：agent 实现完就试归档，被拦后进入「补一格 → 再试 → 还缺再拦」的试探循环，每次重试都是一次完整四项检查 + 会话轮次消耗（improve-discovery-recommendations 那次 5 分钟烧了 7 轮）。缺的不是信息，是**归档前的同口径自查入口**。

## What Changes

- **新增 `scripts/harness/archive-readiness.sh <change>`**：与 spec-gate 归档四项检查完全同口径的聚合自查（doc-impact verify / check-standards / tasks.md 尾三节 / scenario-trace），逐项输出绿/红与修复指引，任一红即退出码非 0。只读不跑测试，供 agent 在 archive 前一次看清全貌。
- **spec-gate block 文案增强**：`buildBlockReason` 输出追加一行指引——「先跑 `bash scripts/harness/archive-readiness.sh <change>` 自查全绿再归档，勿逐项试探重试」，把试探式重试变成自查到绿一次过。
- **开发执行规范 §11 归档动作前置自查**：把「归档前先跑 readiness 自查」写成规范动作（流程文档，不新增 block 机制、不新增豁免）。
- **harness 文档同步**：`docs/reference/harness/pi-extensions.md` 归档门禁节补自查入口说明。

不改动四项检查本身的判定逻辑与阈值（scenario-trace.sh / doc-impact.sh / check-standards.sh / tasks 尾三节规则原样复用）。

## Capabilities

### New Capabilities

- `archive-readiness`: 归档就绪自查——提供与 spec-gate 归档门禁同口径的四项聚合只读自查入口，与门禁 block 指引联动，消除试探式归档重试。

### Modified Capabilities

（无——scenario-trace-gate / doc-impact-gate 等既有 capability 的 requirement 均不变，本 change 只新增聚合入口与流程指引。）

## Impact

- `scripts/harness/archive-readiness.sh`（新增，POSIX bash）
- `.pi/extensions/spec-gate.ts`（仅 `buildBlockReason` 文案与测试断言同步）
- `docs/reference/开发执行规范.md` §11、`docs/reference/harness/pi-extensions.md`
- 无业务代码 / API / 数据库 / 前端改动
- 验收指标（可回检）：`m7.block_recur_groups` 当前 5 → 期望 ≤2（落地后 7 天观察窗，`harness-retro.sh --days 7` 复测）
