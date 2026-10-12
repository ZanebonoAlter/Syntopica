## ADDED Requirements

### Requirement: tasks review 任务行前移检查

complex 档 change（proposal.md 头部机器可读注释区声明 `<!-- complexity: complex -->`）的 tasks.md SHALL 从建立阶段起携带至少一条 review 任务行——`- [ ]` 或 `- [x]` 开头且含 `review` 或 `审查`（不区分大小写）的任务行，覆盖「派子线程聚焦 review + review.md 建账，或纯文档类以 proposal 头部 `<!-- review: waived -->` 豁免」。`check-tasks-tail.sh` SHALL 机械判定该要求：proposal 头部存在 `<!-- review: waived -->` 时 SHALL 豁免放行（与检查⑤同词表，不新造豁免机制）；缺行且未豁免 SHALL 退出码 1 并给出修复指引。该判定 SHALL 并入归档门禁检查③与 archive-readiness 自查（同一脚本单一事实源）。simple 档与无 complexity 声明的 change SHALL NOT 受此判定影响（向后兼容）。openspec-apply-change skill SHALL 在 apply 入口先运行 check-tasks-tail.sh，把缺行暴露提前到实现开始之前。

#### Scenario: complex 档 tasks 缺 review 任务行被拦

- **GIVEN** proposal.md 头部声明 `<!-- complexity: complex -->` 且无 `<!-- review: waived -->`，tasks.md 无任何 review 任务行
- **WHEN** 运行 `bash scripts/harness/check-tasks-tail.sh <change-dir>`（apply 入口提醒或归档门禁检查③）
- **THEN** SHALL 退出码 1，输出指出缺 review 任务行并给出两条修复路径（补任务行 / proposal 头部 waived）

#### Scenario: complex 档 tasks 含 review 任务行通过

- **GIVEN** complex 档 tasks.md 含 `- [ ] 7.1 派子线程聚焦 review 并 review.md 建账`，尾三节与 doc-impact 声明齐备
- **WHEN** 运行 `bash scripts/harness/check-tasks-tail.sh <change-dir>`
- **THEN** SHALL 退出码 0（尾三节等既有判定行为不变）

#### Scenario: waived 豁免放行

- **GIVEN** complex 档 proposal.md 头部存在 `<!-- review: waived -->`（理由写在正文），tasks.md 无 review 任务行
- **WHEN** 运行 `bash scripts/harness/check-tasks-tail.sh <change-dir>`
- **THEN** SHALL 退出码 0（豁免词表与检查⑤的 review-trace 同源）

#### Scenario: simple 档向后兼容

- **GIVEN** proposal.md 头部声明 `<!-- complexity: simple -->` 或无 complexity 声明，tasks.md 无 review 任务行，尾三节与 doc-impact 声明齐备
- **WHEN** 运行 `bash scripts/harness/check-tasks-tail.sh <change-dir>`
- **THEN** SHALL 按既有判定退出码 0，SHALL NOT 因缺 review 任务行而失败

#### Scenario: apply 入口提前暴露

- **GIVEN** complex 档 tasks.md 缺 review 任务行，agent 通过 openspec-apply-change 进入实现
- **WHEN** apply 工作流按 skill 约定在实现前运行 check-tasks-tail.sh
- **THEN** 缺行 SHALL 在实现开始前被看见并补上，SHALL NOT 待到归档阶段才暴露
