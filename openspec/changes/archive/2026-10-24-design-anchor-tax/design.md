# Design: design-anchor-tax

## Context

动机见 `proposal.md` - Why。现状基线（本 change 探索阶段实测，2026-10-19，全库 138 份 design / 831 条 Decision）：

- 文件锚点 185 条（22%）、tasks 引用 226 条（27%）、任一落点 346 条（42%）、符号级可验证 10 条（1%）。
- 每条 Decision 平均字数中位 293、P75 340；`add-question-driven-news-research` 554、`data-enrichment-orchestration` 1716。
- 取舍（写出被否方案与理由）覆盖 38%。
- 口径（探索基线 + retro 复算共用判定）：落点四选一（仓库文件、任务 id、Scenario 标题、无代码落点 + 承接制品），以「可解析」为准，空泛表述与同名歧义不计；每决策字数 = Decision 正文（去标题行）非空白字符数，取中位。探索脚本与 retro 解析器实现细节可能略有差异，准 A/B 只比较同一解析器的改前/改后，绝对值不作跨工具对账。
- 注意：探索期 42% 是宽松计数（路径字符串/任务编号出现即算），retro 的 `m7.design_landing_rate` 用严格可解析判定（文件存在、符号可检索、任务 id/Scenario 存在），故绝对基线更低（同窗口实测约 20% 量级（随仓库状态浮动））——A/B 只看同口径改前改后，不与 42% 直接对减。
- 门禁现状：requirements 档无质量门禁；`quality-gate` 对纯文档回合零成本放行；`spec-gate` 只在 archive 检查制品完备性。
- review 现状：`change-review-glm` 只按主线程写的 brief 审查；`开发执行规范` §「review 执行要点」没有制品/设计维度。

## Goals / Non-Goals

**Goals:**

- 让每条 Decision 至少有一个可解析的承接物；PPT 式决策在 review 阶段有明确抓手。
- 用「模板 + review + 指标」三层轻量闭合，不新增硬门禁。
- 指标可回检：改动前后能用 `harness-retro` 对比，且不牺牲 review 的 High 发现数。

**Non-Goals:**

- 不回溯重写存量 design，不阻断已归档 change。
- 不用 `file:line` 单一口径；不要求每条决策都改代码。
- 不新增事件类型、不新增独立门禁脚本、不新增独立 metrics 脚本。
- 不把「每决策字数」做成硬上限（只观测）。

## Decisions

### D1. 落点四选一，而不是 file:line

- **结论**：落点允许 `file[:symbol]` / 任务 id / Scenario 标题 / 无代码落点（承接制品 + 理由）四类；判定以「可解析」为准，不以「写过」为准。
- **落点**：`openspec/schemas/syntopica-ui/schema.yaml`（design instruction 与 template）与任务 2.1/2.2。
- **取舍**：严格 `file:symbol`（现基线 1%，会逼假锚点）与只认文件路径（22%，流程决策误伤）都试算过 → 选四选一（42% 基线，覆盖流程/契约决策）。

### D2. 只改模板 + review，不上门禁

- **结论**：本 change 只改 design 模板/指令与 review 执行要点，retro 加指标；不新增入口/归档硬门禁。
- **落点**：`docs/reference/开发执行规范.md`（review 执行要点）与任务 3.1。
- **取舍**：entry-gate 式软提醒见效快，但当前门禁噪声已高（30 天 fail-open 254 条、情报型 warn 668 条）→ 先用零噪声的两层，4-6 个 change 后看指标再决定是否升级。

### D3. 落点必须 resolve，防假锚点

- **结论**：文件必须存在（全路径或唯一 basename），任务 id 必须存在于当前 change 的 tasks.md，Scenario 必须存在于 delta spec；同名 basename 歧义按未解析；「见 tasks」「后续补」「待定」不算。
- **落点**：Scenario「空泛落点不算」。
- **取舍**：只查格式最便宜，但会把「落点：见 tasks」变成新的形式主义 → 选可解析判定；代价是需要一个 cheap resolver，本 change 由模板审查/人工复核承担，不写门禁脚本。

### D4. 指标进 retro，不另起脚本

- **结论**：决策数、落点率、每决策字数、取舍率并入 `harness-retro.sh` ⑦段，复用其只读消费方定位；不新增事件、不写账本。
- **落点**：`scripts/harness/harness-retro.sh` 与任务 4.1。
- **取舍**：独立 `design-metrics.sh` 更容易单测，但会多一个入口与一份口径 → 选并入 retro，smoke 复用 `harness-retro.smoke.sh`。

### D5. 存量不回溯

- **结论**：规则只对新 change 与进行中的 change 生效；已归档 change 不补、不阻断。
- **落点**：Scenario「存量 change 不回溯」。
- **取舍**：全量回填能立刻统一口径，但会制造 138 份文档的返工且不产生新信息 → 选不回溯，指标按新样本观察。

## Risks / Trade-offs

- [D1 落点写成空话] → 「可解析」判定 + review 复核；空泛落点不计入覆盖，流程决策必须写承接制品与理由。
- [D2 无强制力，模型照旧] → 4-6 个 change 后看 `m7.design_landing_rate`；低于 50% 再升级为 entry-gate 软提醒（不在本 change）。升级判据只看**新模板 change 子集**（或同窗口改前/改后差值），不对全样本绝对值套 50%——存量旧格式走正文兜底，会永久拉低全量口径（同窗口实测约 20% 量级（随仓库状态浮动））。
- [D3 resolver 误判（同名文件、动态 Scenario 标题）] → 文件允许唯一 basename；Scenario 只做标题包含匹配；歧义按「未解析」记，不阻断。
- [D4 指标被当成新 KPI] → 只用于准 A/B，不合成总分；与 review High 发现数一起看，防止「为短而短」。

## Migration Plan

1. 合入后新 change 自动走新模板；进行中的 change 由作者在下次编辑 design 时补齐。
2. 存量归档 change 不改；`harness-retro` 指标只统计新样本。
3. 回滚：恢复 `schema.yaml` 与 `templates/design.md` 两处文本即可；无数据/状态迁移。

## Open Questions

（无）
