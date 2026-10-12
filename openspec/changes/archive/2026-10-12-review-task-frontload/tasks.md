# Tasks: review-task-frontload

## 1. 判定扩展（check-tasks-tail.sh，单一事实源）

- [x] 1.1 `scripts/harness/check-tasks-tail.sh` 追加 review 任务行判定：解析 proposal.md 头部机器可读注释区的 `<!-- complexity: complex -->` 与 `<!-- review: waived -->`（解析边界沿用 review-trace.sh 先例：开头连续 `<!-- -->` 行，遇正文停）；complex 且未 waived 时，tasks.md 须存在 `- [ ]`/`- [x]` 开头且含 review/审查（不区分大小写）的任务行，缺则退出码 1 + 两条修复指引；simple/无声明 change 行为不变。验证：`bash scripts/harness/check-tasks-tail.smoke.sh` 全绿
- [x] 1.2 `scripts/harness/check-tasks-tail.smoke.sh` 同步四用例：complex 缺行 fail / complex 含行 pass / complex+waived pass / simple 缺行 pass（既有用例不回归）。验证：同 1.1 命令全绿

## 2. skill 约定（propose 注入 + apply 提前暴露）

- [x] 2.1 `.agents/skills/openspec-propose/SKILL.md` 与 `.agents/skills/openspec-new-change/SKILL.md` 的 tasks 生成步骤注入约定：complex 档 tasks.md 含「Review」节（派子线程聚焦 review + review.md 建账或 proposal 头部 review: waived）。验证：`grep -n "Review" <两份 skill>` 命中新增约定
- [x] 2.2 `.agents/skills/openspec-apply-change/SKILL.md` 开头加一步「先跑 `bash scripts/harness/check-tasks-tail.sh <change-dir>`，红了先补 review 任务行再实现」。验证：`grep -n "check-tasks-tail" .agents/skills/openspec-apply-change/SKILL.md` 命中

## 3. 既有回归

- [x] 3.1 跑 `bash scripts/harness/check-tasks-tail.smoke.sh` 与 `bash scripts/harness/review-trace.smoke.sh` 确认既有判定零回归（review-trace.sh 本 change 不改，仅确认未被波及）。验证：两命令退出码 0

## 4. 测试

- [x] 4.1 编写 `test-cases.md`（complex 档强制）：主链路故事表（串 specs 新增 5 个 Scenario）、五组变体走查、⓪ 继承与调整表（`bash scripts/harness/test-assets.sh review-trace-gate` 反查）、白盒附加节（marker 解析边界/任务行匹配误伤/豁免优先级）。验证：文件存在于本 change 目录且五问句逐项有答案

## 5. 文档

<!-- doc-impact: none(纯 harness 工具链变更；改动的 开发执行规范.md 与 harness/pi-extensions.md 不属 8 域，如 verify 启发式误报再补 excuse) -->

- [x] 5.1 `docs/reference/开发执行规范.md`：§0.6 补「complex 档 tasks 模板固定含 Review 节」约定 + 步骤 4 前移说明 + §11 门禁③描述联动（「尾三节 + doc-impact + review 任务行」）。验证：`grep -n "Review 节\|review 任务行" docs/reference/开发执行规范.md` 命中
- [x] 5.2 `docs/reference/harness/pi-extensions.md`：spec-gate 检查③描述补 review 任务行判定。验证：`grep -n "review 任务行" docs/reference/harness/pi-extensions.md` 命中

## 6. Review（本 change 自吃狗粮：complex 档 tasks 携带 review 任务行）

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账（纯文字类才走 proposal 头部 `<!-- review: waived -->`，本 change 不适用）。验证：`bash scripts/harness/review-trace.sh openspec/changes/review-task-frontload` 退出码 0

## 7. 验证

| Scenario | 测试文件 |
| --- | --- |
| complex 档 tasks 缺 review 任务行被拦 | scripts/harness/check-tasks-tail.smoke.sh |
| complex 档 tasks 含 review 任务行通过 | scripts/harness/check-tasks-tail.smoke.sh |
| waived 豁免放行 | scripts/harness/check-tasks-tail.smoke.sh |
| simple 档向后兼容 | scripts/harness/check-tasks-tail.smoke.sh |
| apply 入口提前暴露 | 人工：下一真实 change 走 propose/apply 流程时观察（tasks 7.3 同观察点，skill 约定步非机器门禁） |

- [x] 7.1 `bash -n scripts/harness/check-tasks-tail.sh && bash scripts/harness/check-tasks-tail.smoke.sh` → 语法通过 + smoke 全绿
- [x] 7.2 `bash scripts/harness/archive-readiness.sh review-task-frontload` → ③项展示新判定生效（本 change 自身作为活体验证样本）
- [ ] 7.3 人工：下一个真实 change 走 propose 流程时观察 tasks 是否自带 Review 节（约定注入生效）；存量 complex in-flight change 归档撞检查③新红时补一行任务勾掉即可（设计④预期行为）
