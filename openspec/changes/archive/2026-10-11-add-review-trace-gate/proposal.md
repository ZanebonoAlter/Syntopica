<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

开发执行规范 §0.6 步骤 4「子线程聚焦 review」自 2026-07-06 立规以来没有任何机制承载（events.db 考古 2026-08-23 ~ 2026-10-11）：gate.check 19,983 次中 review 相关 0 次；38 次归档（约 32 个 change）仅 ~6 个有 review 派发（覆盖率 ~16%）；2026-09-29 之后 review 派发完全归零而 10 月仍归档了 6 个 change。跳过 review 零成本（不红灯、不拦归档、无统计），而一条写着「不得跳步」的规范被系统性跳过会侵蚀整个规范体系的权威性（其他纯文本 MUST 会跟着被「看情况遵守」）。同时 subagent.dispatch 事件 169 条中 change 归属 0 条（complete 回填已带 change，dispatch 本体漏带），review 覆盖率连统计都无法进行。

## What Changes

- **C（可观测先行）**：`harness-telemetry.ts` 的 subagent.dispatch 落账补 change 归属——复用既有同源基础设施（`detectActiveChange` + `agentChangeBindings`），新增行带值不改写旧行，与事件不可变约束兼容；`harness-retro.sh --lifecycle` 体检卡新增「review 覆盖」段（review.md 痕迹判定、review 类派发计数、waived 豁免占比），让「没做 review」先可见。
- **A（门禁跟上）**：新增 `scripts/harness/review-trace.sh` 归档对账——机械判据二选一：change 目录存在 `review.md`（格式契约仿 human-feedback.md 建账模式：`### R-<N>【High|Medium|Low】<标题>` + 状态行，显式「审了，无发现」也算合法痕迹），或 proposal.md 头部显式 `<!-- review: waived -->` 豁免声明（纯文档 change 用，waived 占比进 retro 统计防滥用）；两者皆无则红。`archive-readiness.sh` 聚合自查四项扩为五项（硬检查，⑤ human-feedback 提醒层顺延为 ⑥）；spec-gate 归档门禁同口径挂第五项 block 检查，逃生口沿用既有 `--force` / `SPEC_GATE_BYPASS=1` 不新增。
- **规范语义微调**：review 从「每次都做、无痕迹要求」收紧为「做了要留痕（review.md）、不做要显式豁免（waived 声明）」——单文件小修不豁免（规范原文「仍须走 review 不得跳步」维持），纯文档 change 走显式 waived。
- **文档链同步**：开发执行规范 §0.6 步骤 4 / §11 归档门禁清单、doc-authoring.md 注册点速查（review.md 格式契约）、harness/pi-extensions.md 扩展全景、harness-facts skill（dispatch 事件新字段）。

## Capabilities

### New Capabilities

- `review-trace-gate`: review 痕迹归档门禁与建账契约——review.md 条目格式、waived 显式豁免声明、review-trace.sh 只读校验规则、spec-gate 归档门禁第五项集成、retro --lifecycle review 覆盖统计。

### Modified Capabilities

- `archive-readiness`: 归档就绪聚合自查从四项硬检查扩为五项（+review-trace），⑤ human-feedback 提醒层顺延编号且保持不阻断语义。
- `harness-fact-log`: subagent.dispatch 事件携带 change 归属（新增行带值；既有行不可变约束不变）。

## Impact

- **代码**：`.pi/extensions/harness-telemetry.ts`（dispatch 落账补 change 字段，一处）、`.pi/extensions/spec-gate.ts`（gateArchive 挂第五项）、`scripts/harness/review-trace.sh`（新增）、`scripts/harness/archive-readiness.sh`（+第⑤项硬检查）、`scripts/harness/harness-retro.sh`（lifecycle +review 段）+ 三个对应 smoke。
- **主 spec 同步**：archive-readiness / harness-fact-log 两份 MODIFIED delta 归档时合并。
- **文档**：docs/reference/开发执行规范.md、docs/reference/standard/shared/doc-authoring.md、docs/reference/harness/pi-extensions.md、.agents/skills/harness-facts/SKILL.md。
- **存量兼容**：存量 in-flight change 归档时同样受第五项检查（仿 scenario-trace-gate 先例，不豁免）；历史 dispatch 事件无 change 归属属既成事实，retro review 覆盖统计自本 change 落地后有效，不回填历史。
- **不涉及**：前端零改动（ui-impact: none）；不涉业务域（纯 harness 工具链，无 constraint-domains 声明）；events.db schema 零迁移（change 列已存在，只是开始写入）。
