# Design: review-task-frontload

## Context

见 proposal.md（Why：daily-report-lifeline-heal 复盘——review 只有归档端对账与文字纪律，tasks 无 review 任务行时全程无机械触点，到 spec-gate ⑤ 才拦，返工成本最高）。

现状代码事实（探索阶段实查）：

- `scripts/harness/check-tasks-tail.sh`：tasks.md 结构判定的单一事实源——检查「测试/文档/验证」尾三节标题 + `<!-- doc-impact:` 声明标记，POSIX bash 只判不跑；被 spec-gate 归档门禁检查③与 `archive-readiness.sh` 自查③共同调用（harden-archive-readiness 迁移先例：判定抽出独立脚本，勿另立第二份）。
- `scripts/harness/review-trace.sh`（检查⑤）：对账 review 痕迹——proposal 头部 `<!-- review: waived -->`（优先）或 review.md 合法建账/审毕无发现声明；与检查③互不知晓。
- 三份 skill（openspec-propose / openspec-new-change / openspec-apply-change）均无 review 相关约定；tasks.md 由 propose/new-change 阶段的 agent 生成。
- complexity 判据先例：proposal.md 头部机器可读注释区 `<!-- complexity: complex -->`，与 test-cases.md 强制（⑤a）同一判据。

## Goals / Non-Goals

**Goals:**

- complex 档 change 的 tasks 从建立阶段起携带 review 任务行；apply 一进来就能看见缺行；归档门禁③同步收紧。
- 判定单一事实源：只扩展 check-tasks-tail.sh，③自动生效，不新增门禁通道/脚本。
- 豁免机制复用检查⑤的 `<!-- review: waived -->`（同词表，不新造第二套豁免）。
- simple 档与无声明 change 完全向后兼容。

**Non-Goals:**

- 不动 spec-gate.ts / quality-gate.ts 扩展代码（③已调脚本，自动吃到新判定）。
- 不给 simple 档加 review 任务要求（⑤已兜底：waived 或建账二选一）。
- 不做「review 任务行必须勾掉才能归档」的联动（勾没勾与干没干是两回事——⑤的痕迹对账才是「干了没」的权威判定，避免双重校验口径漂移）。
- 不回溯已归档 change。

## Decisions

### ① 判定落点：check-tasks-tail.sh 扩展（而非新脚本或新扩展）

**选择**：在 check-tasks-tail.sh 既有判定（尾三节 + doc-impact 标记）之后追加 review 任务行判定。

**理由**：该脚本已是 tasks 结构判定的单一事实源，归档门禁③与 archive-readiness 自查都调它——扩展一处、两处生效；apply skill 也只需指向同一个脚本。新脚本/新扩展会造第二份 tasks 判定逻辑，违背 harden-archive-readiness 的迁移先例。

**备选**：a) quality-gate turn_end 挂检查（拒绝：门禁通道+1，告警噪声，且 quality-gate 管的是代码质量不是 tasks 结构）；b) 独立 review-task-check.sh（拒绝：同上，单一事实源原则）。

### ② 分档判据与豁免：复用 proposal 头部 marker，豁免同检查⑤词表

**选择**：complex 判据 = proposal.md 头部机器可读注释区 `<!-- complexity: complex -->`；豁免判据 = 同区域 `<!-- review: waived -->`（存在即豁免，理由不解析——与 review-trace.sh 语义一致）。

**理由**：与 test-cases.md 强制同构（同一 marker、同一分档哲学）；豁免机制与检查⑤同词表，agent 只需记一套。头部注释区的解析边界沿用 review-trace.sh 的成熟实现（文件开头连续 `<!-- -->` 行，遇正文停）。

**备选**：tasks.md 文内放豁免标记（拒绝：豁免语义属于「整个 change 做不做 review」，归 proposal 头部与⑤一致；tasks 是执行计划不是豁免声明处）。

### ③ apply 层走 skill 约定（入口先跑 check-tasks-tail），不走门禁强制

**选择**：openspec-apply-change skill 开头加「先跑 check-tasks-tail.sh，红了先补 review 任务再实现」一步。

**理由**：apply 由 skill 驱动，agent 顺手跑一个只读脚本零成本；缺行时补一行 tasks 是秒级操作，不构成需要 block 级门禁的风险。门禁强制已在归档端③兜底。

**备选**：spec-gate 或 entry 扩展在 apply 阶段 block（拒绝：为秒级可修的问题加 block 级门禁，过重；且扩展改动面大）。

### ④ 存量 in-flight complex change 不豁免

**选择**：新判定对存量 in-flight complex change 立即生效——归档前若 tasks 无 review 行且未 waived，检查③会红；修复 = 补一行任务勾掉（或按实际执行情况写 review.md，⑤侧已有路径）。

**理由**：仿 add-review-trace-gate 先例（存量不豁免，补一行成本极低）；豁免存量会引入「按建立日期分叉」的判定复杂度。

## Risks / Trade-offs

- [propose 层是 skill 约定，agent 可能仍漏写] → 漏写被②apply 入口检查兜住（红了补一行即可），最终被③归档门禁兜住；三层递进本就是设计意图。
- [tasks 行匹配可能误伤（正文提到 review 的非任务行）] → 判定锚定 `- [ ]`/`- [x]` 任务行前缀 + 关键词，正文引用不命中；smoke 覆盖误伤用例。反向同理存在**正向宽松**：非 review 任务行蹭到关键词（如「review 见第 6 节」）也会被判在场——已知宽松不收紧（关键词是唯一机器可判信号），「干了没」由⑤痕迹对账兜底。
- [存量 change 归档时撞新红] → 修复成本一行，且 apply 入口提前暴露（④）；README/规范里注明。

## Migration Plan

1. 纯脚本 + skill + 文档变更，无 DB 迁移。
2. 部署即生效：下一个新建 change 即走新约定；存量 in-flight complex change 归档前补一行。
3. 回滚：revert 脚本与 skill 改动即可，无数据残留。

## Open Questions

（无——两个待定项已在决策②④定案：豁免复用 waived 词表、存量不豁免。）
