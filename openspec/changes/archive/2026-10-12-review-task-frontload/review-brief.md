# 审查 brief：review-task-frontload（全量增量，2026-10-12）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令（允许 read/grep/find/ls 与只读验证如 `bash scripts/harness/check-tasks-tail.smoke.sh` / `bash -n`；不要求跑测试；优先 grep 与精读）。

## 一、审查对象（本 change 辖区，未 commit，在工作树）

**背景**：daily-report-lifeline-heal 复盘发现 review 全部压在归档端（spec-gate 检查⑤ review-trace 痕迹对账），tasks.md 从建档起无 review 任务行时全程无机械触点，返工成本最高。本 change 把 review 前移三层：propose 建档注入 → apply 入口检查 → 归档门禁③收紧（complex 档强制，simple 行为不变）。

**改动清单**（本 change 唯一辖区，审查只看这些）：

| 任务 | 文件 |
| --- | --- |
| 1.1 判定扩展 | `scripts/harness/check-tasks-tail.sh`（新增 head_markers + review 任务行判定 + 修复指引输出） |
| 1.2 smoke 同步 | `scripts/harness/check-tasks-tail.smoke.sh`（新增 ⑦-⑪ 五用例，①-⑥ 既有不动） |
| 2.1 skill 约定 | `.agents/skills/openspec-propose/SKILL.md`（Artifact Creation Guidelines 加 Review 节条目）、`.agents/skills/openspec-new-change/SKILL.md`（步骤 6 后、Output 节前加 blockquote 约定） |
| 2.2 apply 入口 | `.agents/skills/openspec-apply-change/SKILL.md`（步骤 1 加 Review 任务行前置检查段） |
| 4.1 用例账本 | `openspec/changes/review-task-frontload/test-cases.md`（新文件） |
| 5.1 规范联动 | `docs/reference/开发执行规范.md`（§0.6 加 complex 档 Review 节约定 blockquote + §11 检查③描述扩写） |
| 5.2 门禁文档 | `docs/reference/harness/pi-extensions.md`（spec-gate 行 + archive-readiness 行的③描述 + 变更记录表新行） |

设计权威源：`openspec/changes/review-task-frontload/design.md`（决策①-④、Risks、Non-Goals）。
验收契约（delta spec，ADDED 1 Requirement 5 Scenario）：`specs/review-trace-gate/spec.md`。
用例账本：`test-cases.md`（主链路 5 步 + 变体走查 + ⓪继承 + 白盒附加）。

## 二、审查维度

1. **精读 `scripts/harness/check-tasks-tail.sh` 新增判定**（对照 `git diff -- scripts/harness/check-tasks-tail.sh`）：
   - `head_markers` 与 `review-trace.sh` 同名函数是否逐字同实现（同边界：开头连续 `<!-- -->` 行、空行穿插、遇正文停）；不一致即口径漂移（B1 分支，design 决策②明确要求沿用先例）。
   - complex/waived 两个 marker 正则是否与 review-trace.sh 的 waived 正则同款全串匹配（`^<!--\s*key\s*:\s*value\s*-->$`）；marker 出现在正文是否确认不识别。
   - 任务行正则 `^- \[[x ]\].*(review|审查)`（grep -Ei）：勾选框锚定是否可靠；`- [X]` 大写被 -i 放行是否可接受（GFM 兼容口径）；会不会漏掉合法形态或误伤正文（设计风险②声称「smoke 覆盖误伤用例」——确认 ⑦ fixture 的误伤样本是否真测到了防线）。
   - proposal.md 不存在/不可读路径：review 判定是否整体跳过且不影响既有尾三节判定（向后兼容红线）。
   - 输出路径：missing 数组与 review_missing 双红并发时两段输出是否都出；exit code 是否恒正确（成功 0 / 任一红 1）；heredoc 修复指引文案与 spec Scenario ① 的「两条修复路径」契约是否一致。
   - `set -u` 下 `markers` 为空时的行为（`printf '%s\n' ""` → 空行，grep 不命中——确认无误报红）。
2. **精读 smoke 新增 ⑦-⑪**：fixture 是否真造出了所声明的形状（⑦ 的误伤样本行是否真的不匹配任务行正则）；⑪ 正文 marker fixture 的 proposal.md 首行是 `## Why`，head_markers 是否立刻停（markers 为空）→ 判定不启用；既有 ①-⑥ 是否零改动（回归红线）。
3. **skill 三份与 spec/规范的一致性**：
   - propose/new-change 两份的 Review 节约定文字是否与 `docs/reference/开发执行规范.md` §0.6 新 blockquote 同口径（同词表：`- [ ]` 开头含 review/审查、waived 豁免、check-tasks-tail.sh 判定）；有无互相矛盾（如一处说 MUST 一处说 SHOULD）。
   - apply skill 的前置检查段是否说清了「红了先补再实现」与豁免路径；`<change-dir>` 形态说明是否足够 agent 照做。
   - skill 属 openspec 官方生成物（generatedBy 1.10.0）被仓库定制注入——评估注入位置是否会在 openspec update 重新生成时丢失风险加大（仅提示性意见，不算问题）。
4. **两份文档联动准确性**：§11 检查③ 与 pi-extensions.md spec-gate/archive-readiness 行的描述是否与脚本实际行为一致（③ 现在是「尾三节 + doc-impact + complex 档 review 任务行」三合一）；变更记录表新行格式与既有行是否一致。
5. **grep 验证关键不变量**（至少）：
   - `grep -n "head_markers" scripts/harness/review-trace.sh scripts/harness/check-tasks-tail.sh` → 两处实现 diff 为零（逐字对比）。
   - `bash scripts/harness/check-tasks-tail.smoke.sh` → 11 case 全过退出码 0。
   - `bash scripts/harness/review-trace.smoke.sh` → 退出码 0（review-trace.sh 本 change 不改，零回归）。
   - `bash -n scripts/harness/check-tasks-tail.sh` → 语法通过。
   - `grep -rn "complexity" scripts/harness/*.sh` → 除本 change 新增外无第二套 complex 判定实现漂移（harness-retro.smoke.sh 的 simple 分桶不受影响）。
   - `git diff -- scripts/harness/review-trace.sh scripts/harness/archive-readiness.sh .pi/extensions/` → **空**（design 非目标：不动扩展与 review-trace）。
6. **制品自洽**：test-cases.md 主链路 5 步是否真串了 delta spec 的全部 5 个 Scenario（逐一对名）；⓪继承表对既有 15 Scenario 的处置是否与 test-assets.sh 反查事实一致；spec Scenario 措辞（SHALL/SHALL NOT）与脚本行为有无出入。

## 三、输出要求（三段式，中文）

1. **总评**：改动是否达成 design/spec 意图、可否进入门禁收口（一句话结论 + 理由）。
2. **问题清单**：按 High/Medium/Low 分级，每条给 `file:line` + 复现/依据 + 建议修法；没有就明确写「无」。
3. **核实记录**：实际跑过的只读命令与退出码、读过的关键文件清单（防幻觉）。

## 已知且已裁决、**不要**当问题报的事项

- spec-gate.ts / quality-gate.ts 扩展代码**刻意不动**（③ 已调同一脚本自动吃新判定，design 非目标）。
- 不做「review 任务行必须勾掉才能归档」联动（design 决策：勾没勾与干没干是两回事，⑤管干了没）。
- 不给 simple 档加 review 要求；不回溯已归档 change；存量 in-flight complex change 归档撞新红属**预期行为**（design 决策④，补一行即可）。
- propose 层是 skill 约定、agent 仍可能漏写——三层递进兜底是设计意图，不算漏洞。
- tasks.md 匹配放行 `- [X]` 大写勾选框（grep -i 副作用，GFM 渲染器普遍兼容，宽容无害）。
- 工作树含其他 active change 脏文件（`backend-go/internal/**`、`docs/reference/flow/**`、`openspec/changes/add-topic-key-event-markers/**`、`openspec/changes/ai-provider-model-overhaul/**`、`.pi/agents/*.md` 等）——超出本 change 辖区，一律不评。
