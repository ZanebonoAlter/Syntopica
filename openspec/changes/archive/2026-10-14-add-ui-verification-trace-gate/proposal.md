<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

review 痕迹机制（add-review-trace-gate）已做到「实物对账」强度：review.md 建账 + 正则全解析 + 空壳阻断 + 三道防线（建档注入 → apply 入口 → 归档门禁）。而 UI 验收的归档检查（spec-gate ⑥'）实质是**关键词字样匹配**——tasks 验证节含 `opencli`/`1440`/`1920` 字符串、ui-design.md 含「差异」二字即放行，不检查 `ui-verification/` 目录、report.md 与截图实物是否真实存在。写一句话就能过，防空转能力与 review 差一个数量级。

add-topic-key-event-markers 的 4.4 人工验收（用户手动要求才发生）抓到 P1 死按钮缺陷（内嵌总览漏绑 `@open-daily-report`，lint/typecheck/组件单测/功能断言全部天然失明的盲区），证明这类验收是唯一能拦某类缺陷的防线——但它目前靠人肉触发，无过程机制。同时存在三处口径分裂：证据目录名（规范 `ui-verification/` vs 实际 `ui-acceptance/`）、验证工具（规范 12 处 opencli vs 树莓派实战 agent-browser）、skill `ui-verify` 引用悬空（不存在）。

## What Changes

- **新增独立对账脚本** `scripts/harness/ui-verification-trace.sh <change-dir>`（仿 review-trace.sh：纯 bash 只读、正则全解析、中文修复指引、退出码 0/1），作为 UI 验收证据判定的**单一事实源**；spec-gate 检查⑥' 与 archive-readiness 聚合自查复用其结果，防止判定逻辑在扩展内二次耦合。
- **目录名统一为 `ui-verification/`**：新增/修改的规范口径全部指向该名；存量 `add-topic-key-event-markers/ui-acceptance/` 迁移重命名，tasks.md/report.md 内引用同步。
- **major 档轻量版实物对账**（强度先轻，双视口文件名锚点等强档检查暂不做）：
  - `ui-verification/` 目录存在；
  - `report.md` 存在且 ≥1 条格式合法判定条目（仿 review.md R-N 契约：`### A-<N> <场景>【pass|fail|未覆盖】` + 依据行；**未覆盖条目必须显式写原因**，不留悬空）；
  - 判定条目中**点名引用的截图文件**真实存在于目录内（掐死「勾了勾了、图不存在」的空转）。
- **minor 档指名道姓**（探索选型 B）：ui-design.md 验收节不再「出现关键词即过」——
  - 映射到**组件测试**：必须写出测试文件路径，脚本校验文件真实存在；
  - 映射到 **agent-browser/人工验证**：必须在 `ui-verification/report.md` 有 ≥1 条合法记录（一行即可，不强制截图）。
- **验证工具口径切 agent-browser**（树莓派实战口径，本次 4.4 即 agent-browser 无头浏览器代跑）：开发执行规范 12 处 opencli 引用、layout.md、spec-gate ⑥' 的 `/opencli/i` 检查、ui-design-workflow spec MUST 条款同步改口径；顺带清理 `ui-verify`/`opencli-browser` 悬空 skill 引用（指向存在的 agent-browser skill）。
- **spec-gate ⑥' 接线**：`missingArchiveUiEvidence` 中 tasks「opencli/1440/1920」字样弱检查替换为调用脚本结果（扩展只认退出码/结构化输出，判定逻辑全在脚本）。
- **三道防线对齐 review 模式**（细节 design 阶段定）：建档注入（major 档 tasks 模板含 UI 验收任务行）→ apply 入口 → 归档门禁兜底。
- **report.md 建账格式契约**入新 spec（脚本同源），格式对齐本次 add-topic-key-event-markers 实战形态（A-N 判定条目 + 截图清单锚点），不自创未经验证的结构。

## Capabilities

### New Capabilities
- `ui-verification-trace-gate`: UI 验收证据实物对账门禁——独立脚本判定规则、report.md 建账格式契约、minor 指名道姓映射规则、与 spec-gate/archive-readiness 的单一事实源接线关系（同构 review-trace-gate 三段式）。

### Modified Capabilities
- `ui-design-workflow`: 「UI 完成验收必须对照批准基线」requirement 修改——验收工具口径从 opencli 改为 agent-browser、major 验收证据从文本字样升级为实物对账（目录/report.md/截图存在性）、minor 验收映射从关键词匹配升级为指名道姓挂钩。

## Impact

- **新增**：`scripts/harness/ui-verification-trace.sh` + 冒烟脚本；`openspec/specs/ui-verification-trace-gate/spec.md`。
- **修改**：`.pi/extensions/spec-gate.ts`（⑥' 接线）、`.pi/extensions/lib/ui-design-gate.ts`（missingArchiveUiEvidence 弱检查下放）、`openspec/specs/ui-design-workflow/spec.md`（MUST 条款）、`scripts/harness/archive-readiness.sh`（聚合新脚本）、`docs/reference/开发执行规范.md`（§5.3 双层验收/四维度表、§11.1 检查⑥' 描述）、`docs/reference/standard/frontend/layout.md`（验收方式引用）、`docs/reference/standard/shared/test-design.md`（主体 + JIT 摘要共 3 处 opencli 口径同步）、`scripts/harness/check-tasks-tail.sh`（若采纳三道防线第一道）。
- **测试义务（complex 档 + 改契约）**：本 change 必须有 test-cases.md；因修改 ui-design-workflow MUST 条款触发⓪问句——`bash scripts/harness/test-assets.sh ui-design-workflow` 反查旧测试资产（spec-gate 既有 smoke 断言的是旧契约：tasks 含 opencli/1440 字样即过），test-cases.md 填「继承与调整」表逐行处置；新脚本判定逻辑层选纯逻辑测试 + 冒烟脚本（仿 review-trace.smoke.sh）。
- **存量迁移**：`openspec/changes/add-topic-key-event-markers/ui-acceptance/` → `ui-verification/`（含 tasks.md 4.4 与 report.md 内部引用同步）。
- **不改动**：ui-design-gate 前置门禁（原型审批流程）、quality-gate、constraint-injection；不影响后端任何代码。
- 本 change 自身 `ui-impact: none`（纯 harness/文档机制，无用户可见界面变化）。
