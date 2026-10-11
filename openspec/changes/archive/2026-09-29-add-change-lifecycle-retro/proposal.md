<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

以单个 change 为生命周期回顾 harness 流程执行情况（测试执行、UI 工作流卡点、每任务成本、催修健康度）目前只能靠手写 SQL 拼装：数据 80% 已在 events.db（session.rollup×change.archive 每任务成本、ui-design-gate 五种卡点、gate.check 失败聚类、pin 复用），但 retro 报告只提供全局七段 + `--change` 过滤，没有"单 change 体检卡"视图；且实测拼装过程暴露三个数据坑（催修时距用 gate.check 转绿判解除对 ui-design-gate/quota-gate 是错配、UI 审批通过零记录只能推断、无 abandon 终态导致 40+ 活跃 change 仅 25 个有下文）。2026-09-28 探索已产出双面板趋势基线图（docs/research/change-lifecycle/retro-cost-trend-2026-09.html）与支撑度评估（docs/research/change-lifecycle/explore-findings.md）。

另接 ai-provider-model-overhaul FB-4 转挂诉求：人类反馈进 harness 计量闭环。实测该 change（反馈最丰富的样本，FB-1~10 含两轮原型返工）体检卡输出对其反馈零覆盖——事件侧观测不到人类真实不满，human-feedback.md 是旁路数据源；且该文件为机制首例孤例，尚未制度化（零规范零工具）。

## What Changes

- **retro 新增 `--lifecycle <change>` 模式**：输出单 change 生命周期体检卡——流程规范段（mode.set→constraint.inject→pin 接力→gate 循环→archive 锚点）、UI 工作流段（ui-design-gate 卡点链 × 放行态）、成本卡（session.rollup 终值按 models map 归一聚合）、健康度段（block 催修时距、回归翻转、返工波次）。联合 `openspec/changes/<change>/proposal.md` 头注释元数据（ui-impact/complexity/constraint-domains，账本外的声明侧）。
- **催修时距按 policy 分派解除信号**：ui-design-gate 的 block 靠补文档解除、quota-gate 靠换 provider 解除，不再统一用 gate.check 转绿判定（实测该口径产出 89 分钟假慢）。
- **UI 审批通过显式记账**：ui-design-gate 放行侧（ui-approval 通过、ui-design 文档补齐放行）记 policy.decision，体检卡不再靠"block 之后无新 block"推断通过。
- **change 终态事件**：引入 abandon/close 语义记账（触发路径进 design 决策：bash 命令拦截对齐 change.archive 模式，或 retro 报告侧静默判定），消除"僵尸 change"盲区（有事件无 archive 无终态）。
- **human-feedback 反馈建账与计量**：change 可选制品 human-feedback.md 制度化——条目级格式契约（标题行 `【类型|归因】` 标签、状态五值封闭、归因五类封闭，参考 doc-impact 白名单模式）；新增 `scripts/harness/feedback.sh` 三件套（menu 选择题 / add 自动编号套模板 / verify 归档对账拦截未处置条目）；`--lifecycle` 新增第五段「人类反馈段」（条数/归因分布/未处置数）。归因高发层直接映射规范整理去向（frontend→standard/frontend/、test-gap→test-design.md、fact-check→探索纪律）。不进 events.db（文件随 change 归档永久保存，账本有 TTL）。
- **制品清单制度化**：doc-authoring.md 与开发执行规范 §0.6 把 human-feedback.md 纳入 change 可选制品清单（FB-4 后续动作①），存量 ai-provider-model-overhaul 10 条补归因标注与状态词归一。

## Capabilities

### New Capabilities

（无——体检卡是 harness-retro-loop 既有能力的新报告模式，归入修改）

### Modified Capabilities

- `harness-retro-loop`: 新增 `--lifecycle` 单 change 体检卡报告模式（含人类反馈段）；催修时距指标按 policy 类型分派解除信号口径；新增 human-feedback 反馈建账能力（格式契约 + feedback.sh 工具）。
- `harness-fact-log`: policy.decision 词汇表新增 ui-design-gate 放行事件与 change 终态事件（abandon/close，具体 reasonCode 词汇进 design 决策并同步 smoke 断言）。

## Impact

- `scripts/harness/harness-retro.sh`：新增 `--lifecycle <change>` 模式与体检卡输出（纯只读消费，不改账本写入）。
- `scripts/harness/feedback.sh`：新增（menu/add/verify 三件套）。
- `scripts/harness/archive-readiness.sh`：归档对账顺带调用 feedback verify。
- `openspec/changes/ai-provider-model-overhaul/human-feedback.md`：存量 10 条补归因 + 状态词归一（以处置记录为准）。
- `.pi/extensions/ui-design-gate.ts`：放行侧显式记账。
- `.pi/extensions/harness-telemetry.ts`（或 design 决策的触发路径）：change 终态事件记账。
- `docs/reference/standard/shared/doc-authoring.md`、`docs/reference/开发执行规范.md`：可选制品清单纳入 human-feedback.md。
- `docs/reference/harness/pi-extensions.md`、`.agents/skills/harness-retro/SKILL.md`、`.agents/skills/harness-facts/SKILL.md`：报告段说明、词汇表、指标口径同步。
- 不涉及业务代码与前端 UI。
