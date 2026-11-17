# 任务 — add-ui-stage-accounting
## 1. 纯函数层（`lib/ui-design-gate.ts`）

- [x] 1.1 新增 `UI_STAGES` 五档常量（`l0`/`skeleton`/`interaction`/`data`/`approved`）与 `parseUiStage(uiDesignText)`：复用 `parseSingleMarker` 口径，多次出现或取值非法 → `null`；返回 `{stage: UiStage} | {invalid: true} | null` 三态以区分 legacy 与非法；验证：smoke §F 分支用例全绿
- [x] 1.2 新增 `changeNameFromPath(target)`：把 `\` 归一化为 `/` 后匹配 `openspec/changes/<name>/`，返回 change 名或 `null`；`changes/archive/<date>-<name>/` 归为 `null`（归档目录不记账）；验证：smoke 覆盖 posix / windows / archive / 非 change 路径四组
- [x] 1.3 新增 `STAGE_SCOPE` 映射与 `checkStageScope(stage, relPathFromChangeRoot)`：白名单前缀判定，返回 `null`（合规）或 `{reasonCode, detail}`；`approved` 一律返回 `null`；验证：smoke 表驱动覆盖 5×4 组合
- [x] 1.4 新增 `UI_STAGE_WARN_REASON_CODES` 三个 warn 侧码（`ui-stage-missing` / `ui-stage-invalid` / `ui-stage-scope-out`），**不加入 `UI_REASON_CODES`**；验证：smoke 断言两数组交集为空、且 `UI_REASON_CODES.length === 9`

## 2. 触发点（`ui-design-gate.ts`）

- [x] 2.1 在第 132 行的 mode 早退**之前**插入 `accountStageScope(pi, ctx, tool)`：路径不在 `openspec/changes/` 下即直接 return（零开销）；验证：smoke 断言 requirements 档写入被记账、非 change 路径零记录
- [x] 2.2 记账去重：同 session × change × reasonCode 只记一次（复用 `warn-legacy` 的 `Set` 口径）；验证：smoke 连续两次写入仅产生 1 条事件
- [x] 2.3 严格不改裁决：`accountStageScope` 只调用 `auditPolicy(... action: "warn")` 后 return，不读写 `decideUiToolGate` 的任何输出；验证：既有 §A–§E 全部用例仍绿（回归零破坏）

## 3. 审查（review 后补的修正，全部已处置）

- [x] 3.1 派 `change-review` 审查（10 条：1 High / 5 Medium / 4 Low），全部处置为「已修复」；验证：`bash scripts/harness/review-trace.sh openspec/changes/add-ui-stage-accounting` 零待处理
- [x] 3.2 R-1（High）`design.md` D3 改为与实现/delta spec 一致的「原型目录门禁」口径（非原型目录一律放行），并写明 `data` 与 `interaction` 同写域是刻意取舍；验证：三处写域表逐档对读一致
- [x] 3.3 R-2 `scripts/harness/harness-retro.sh` 的 `INFORMATIVE_REASONS` 登记三码，并在两处 `action IN ('block','warn')`（门禁催修时距）排除三码；`.agents/skills/harness-facts/SKILL.md` 的 reasonCode 词汇表补三码与「情报型 warn」定性；验证：`bash -n scripts/harness/harness-retro.sh` 通过
- [x] 3.4 R-3/R-4/R-5 `accountStageScope` 三重防假阳性：只对 `ui-prototype/` 内写入记账（补 marker 写 `ui-design.md` 不再误报）、change 目录须真实存在（仓库外路径不记到 ghost change）、去重键去掉 sessionId（子线程共用模块实例不乘数）；验证：smoke G7/G8/G9 三项全绿

## 4. 测试

- [x] 4.1 `tests/ui-design-gate.smoke.cjs` 追加 §F：`parseUiStage` 三态、`changeNameFromPath` 四组路径、`checkStageScope` 表驱动、warn 码与阻断码不相交；验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 退出码 0
- [x] 4.2 集成用例：真 tmp 仓库 + 真 events.db，`requirements` 档写 `ui-prototype/index.html`（有 marker `skeleton`）→ 断言 1 条 `ui-stage-scope-out` warn 且无 block；写 `ui-prototype/skeleton/x.html` → 断言零事件（合规不记账）；验证：同上
- [x] 4.3 legacy 用例：无 marker 的 change 写入 `ui-prototype/` → 1 条 `ui-stage-missing`，无 block；验证：同上

## 5. 文档

<!-- doc-impact: none(harness 工具链 change：仅改 .pi/extensions/ 与其 smoke、docs/reference/harness/pi-extensions.md 的门禁分层节；均不在 8 个文档域路径内，无 flow 业务域变更) -->

- [x] 5.1 更新 `docs/reference/harness/pi-extensions.md`「门禁分层」节：补 `ui-design-gate` 的按路径触发通道与 stage 记账（与既有 mode 触发通道并列，写明两者互不替代）
- [x] 5.2 更新 `docs/reference/开发执行规范.md` §3.1.1 的 L0–L3 表：末尾补一行「stage 标记 `<!-- ui-stage: ... -->`，当前只记账不阻断」
- [x] 5.3 运行 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-ui-stage-accounting` 与 `bash scripts/harness/check-standards.sh`；验证：两者无阻断项


> **无 flow 影响**：本 change 只改前端设计规范与编排纪律（不涉及任何业务域），`docs/reference/flow/*.md` 无对应域，故免 §12 溯源校验。

## 6. 验证

- [x] 6.1 smoke 全量：`bash .pi/extensions/tests/run-smoke.sh` → 期望结果：退出码 0，constraint-injection 与 harness 全家桶均通过
- [x] 6.2 openspec 制品：`openspec validate add-ui-stage-accounting` → 期望结果：`is valid`
- [x] 6.3 归档就绪：`bash scripts/harness/archive-readiness.sh add-ui-stage-accounting` → 期望结果：六项全绿
- [x] 6.4 反向检查：`git status --porcelain -- front/ backend-go/` 中无本 change 引入的改动（本 change 只碰 `.pi/extensions/` 与文档）
> **归档后回检锚点（观察项，非本 change 完成任务）**：归档两周后跑 `bash scripts/harness/harness-retro.sh`，看 `ui-design-gate` 三个 stage warn 码的分布——`ui-stage-missing` 占比 < 30% 且 `ui-stage-scope-out` 人工抽查误报 < 5% 才可转阻断（判据见 design D5）。

### Scenario → 落地对账

| Scenario | 测试文件 |
| --- | --- |
| requirements 档写入原型目录也会被记账 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 缺少 stage 标记视为 legacy | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 骨架阶段写业务数据越域 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 已批准阶段不再记账 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| marker 取值非法 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
