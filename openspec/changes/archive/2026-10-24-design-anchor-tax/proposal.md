<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

design.md 是 requirements 阶段唯一跨模型、跨会话的交接物，但当前没有任何规则要求「每条决策有下游承接」：实测 138 份 design 的 831 条 Decision 中，只有 42% 能指到仓库内真实存在的文件、任务或 Scenario，符号级可验证的只有 1%。结果 design 可以写得很长、很防御，却没人能证明哪条决策真的发生了——这是「PPT 感」与「过度防御」的共同土壤。

## What Changes

- design 模板与生成指令：每条 Decision 固定写「结论 / 落点 / 取舍」；落点四选一（仓库文件、任务 id、Scenario 标题、无代码落点 + 承接制品），且必须可解析。
- review 执行要点：新增「design 落点复核」维度；有结论无承接的决策记 Medium，标注可删项。
- harness-retro：新增 design 落点率、每决策字数、取舍率三个只读指标，供改动前后准 A/B。
- 不做：新增硬门禁、修改 spec-gate、回溯重写存量 design。存量 change 不因缺少落点被阻断。

## Capabilities

### New Capabilities

- `design-decision-landing`: design 决策落点的四类定义、可解析判定、review 复核与 retro 观测口径。

### Modified Capabilities

（无）

## Impact

- 模板/指令：`openspec/schemas/syntopica-ui/schema.yaml`、`openspec/schemas/syntopica-ui/templates/design.md`
- 流程规范：`docs/reference/开发执行规范.md` §review 执行要点
- 观测：`scripts/harness/harness-retro.sh` ⑦段（只读聚合，不写账本）
- 产品代码、API、数据库零改动；不新增门禁、不新增事件类型。
- 部署后：新 change 的 design 多出两行（落点/取舍）；旧 change 与已归档 change 不变。无需用户迁移、无需重跑。
- 验证基线：landing 42% / words-per-decision 293 / alt 38%（4-6 个 change 后回检；这是探索期宽松口径——路径字符串/任务编号出现即算，`harness-retro` 的 `m7.design_landing_rate` 用严格可解析口径（同窗口约 20% 量级（随仓库状态浮动）），两者不可直接对减，详见 design Context）。
