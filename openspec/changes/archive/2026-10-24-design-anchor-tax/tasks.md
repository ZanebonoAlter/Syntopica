# Tasks: design-anchor-tax

## 1. 基线与口径固化

- [x] 1.1 把探索基线写进 change 记录（138 designs / 831 decisions / landing 42% / words-per-decision 293 / alt 38%），并记录口径（四选一 + 可解析）。验证：`grep -n "831\|42%" openspec/changes/design-anchor-tax/design.md` 命中
- [x] 1.2 固化 resolver 边界：文件全路径或唯一 basename、任务 id、Scenario 标题；歧义按未解析。验证：design D3 与 spec Scenario「空泛落点不算」表述一致（人工核对一行）

## 2. 模板与生成指令

- [x] 2.1 `openspec/schemas/syntopica-ui/templates/design.md` 的 Decisions 段改为「结论 / 落点 / 取舍」三行骨架，并注释四类落点与「可解析」要求。验证：`grep -n "落点\|取舍" openspec/schemas/syntopica-ui/templates/design.md` 命中
- [x] 2.2 `openspec/schemas/syntopica-ui/schema.yaml` 的 design instruction 补「每条 Decision 必须有可解析落点与取舍」段，保留原「不写实现细节」表述。验证：`openspec instructions design --change design-anchor-tax --json | grep -E "落点|取舍"` 命中
- [x] 2.3 用本 change 的 design.md 自吃狗粮：5 条 Decision 全部带落点/取舍。验证：`grep -c "落点" openspec/changes/design-anchor-tax/design.md` ≥ 5

## 3. review 维度

- [x] 3.1 `docs/reference/开发执行规范.md` §「review 执行要点」新增第 0 条「design 落点复核」：可解析判定、Medium 记法、「可删项」标注、流程决策不误伤。验证：`grep -n "design 落点复核\|可删项" docs/reference/开发执行规范.md` 命中

## 4. retro 指标

- [x] 4.1 `scripts/harness/harness-retro.sh` ⑦段新增 `m7.design_decisions` / `m7.design_landing_rate` / `m7.design_words_per_decision` / `m7.design_alt_rate`（只读聚合，口径同 design D1/D3）。验证：`bash scripts/harness/harness-retro.sh --days 30 | grep -E "design_landing_rate|design_words_per_decision"` 命中
- [x] 4.2 复用 `scripts/harness/harness-retro.smoke.sh` 增 fixture：空 design（0 decisions）/ 有落点 / 空泛落点三种，断言 landing_rate 与 words_per_decision。验证：`bash scripts/harness/harness-retro.smoke.sh` 退出码 0

## 5. Review

- [x] 5.1 派子线程聚焦 review（审查模板、规范、retro 口径与本 change 制品），意见按 High/Medium/Low 处置后 `review.md` 建账。验证：`bash scripts/harness/review-trace.sh openspec/changes/design-anchor-tax` 退出码 0

## 6. 测试

- [x] 6.1 模板静态断言：design 模板含三行骨架与四类落点注释；`openspec instructions design` 输出含「落点」「取舍」。验证：命令与期望见 8.2 / 8.3
- [x] 6.2 retro fixture 三态断言（空 design / 有落点 / 空泛落点）。验证：`bash scripts/harness/harness-retro.smoke.sh` 退出码 0
- [x] 6.3 本 change design 自检：5 条 Decision 均有落点与取舍。验证：`grep -c "落点" openspec/changes/design-anchor-tax/design.md` ≥ 5（命令见 8.4）

## 7. 文档

<!-- doc-impact: none(纯 harness/工具链变更；改动的 openspec 模板、开发执行规范 与 harness-retro 不属 8 个 flow 域，如 verify 启发式误报再补 excuse) -->
<!-- doc-impact-excuse: flow=edit.map 归属混入并发 change 的 backend-go/internal/dataenrichment/service/signal_compose_test.go，本 change 未改 flow 域 -->

- [x] 7.1 `docs/reference/开发执行规范.md` §review 执行要点落文（同 3.1）。验证：`grep -n "design 落点复核" docs/reference/开发执行规范.md` 命中
- [x] 7.2 `.agents/skills/harness-retro/SKILL.md` ⑦段指标说明补四个 `m7.design_*` 键与读法（只读、准 A/B、与 review High 发现数一起看）。验证：`grep -n "design_landing_rate" .agents/skills/harness-retro/SKILL.md` 命中

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| 落点指向真实代码 | 人工：本 change design D1/D2 抽查（模板审查） |
| 落点指向任务 | 人工：本 change design D1/D2/D4 抽查 |
| 落点指向 Scenario | 人工：本 change design D3/D5 抽查 |
| 无代码落点必须写承接制品 | 人工：本 change Risks/流程类决策抽查 |
| 空泛落点不算 | 人工：review 执行要点第 0 条口径复核 |
| 有备选 | 人工：本 change D1-D5 取舍行抽查 |
| 无备选 | 人工：模板注释与规范口径复核 |
| review 记录无承接决策 | 人工：下一个 change 的 review.md 观察 |
| 流程决策不误伤 | 人工：同上观察 |
| retro 输出设计落点指标 | scripts/harness/harness-retro.smoke.sh |
| 存量 change 不回溯 | 人工：抽查一份归档 change 不被新增检查阻断 |

- [x] 8.1 `openspec validate design-anchor-tax --strict` → 通过
- [x] 8.2 `grep -n "落点\|取舍" openspec/schemas/syntopica-ui/templates/design.md` → 命中三行骨架与四类落点注释
- [x] 8.3 `openspec instructions design --change design-anchor-tax --json | grep -E "落点|取舍"` → 命中
- [x] 8.4 `grep -c "落点" openspec/changes/design-anchor-tax/design.md` → ≥ 5
- [x] 8.5 `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0
- [x] 8.6 `bash scripts/harness/harness-retro.sh --days 30 | grep -E "design_landing_rate|design_words_per_decision"` → 命中
- [x] 8.7 `bash scripts/harness/check-tasks-tail.sh openspec/changes/design-anchor-tax` → 退出码 0
- [x] 8.8 `bash scripts/harness/scenario-trace.sh openspec/changes/design-anchor-tax` → 退出码 0
- [x] 8.9 `bash scripts/harness/review-trace.sh openspec/changes/design-anchor-tax` → 退出码 0（5.1 完成后）
- [x] 8.10 `bash scripts/harness/archive-readiness.sh design-anchor-tax` → 除 UI 验收项外全绿
