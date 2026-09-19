# Tasks — harden-archive-readiness

## 1. 脚本层（单一事实源）

- [x] 1.1 新增 `scripts/harness/check-tasks-tail.sh <changeDir>`：从 spec-gate.ts `checkTasksMd` 保真迁移尾三节 + `<!-- doc-impact:` 标记判定（POSIX ERE `^##[[:space:]]+[0-9]+\.[[:space:]]*测试/文档/验证`，`grep -F` 标记），缺失清单文案与 TS 版逐字一致；退出码 0/1。配 `check-tasks-tail.smoke.sh`：全过 / 缺节 / 缺标记 / 子节 `### 2.x 测试` 不命中 / 正文引用不命中 / tasks.md 不存在 六 case 全绿
- [x] 1.2 新增 `scripts/harness/archive-readiness.sh <change>`：四项聚合自查（doc-impact.sh verify / check-standards.sh --change / check-tasks-tail.sh / scenario-trace.sh），逐项 ✓/✗ + 红项透传输出尾部 20 行，末行固定「UI 验收证据项不在自查范围（仅归档门禁校验）」；退出码 0 全绿 / 1 有红 / 2 用法错误或目录不存在；参数容忍 `openspec/changes/` 前缀。配 `archive-readiness.smoke.sh`：全绿 / 多红汇总 / change 不存在 / 只读安全（跑前后 `git status --short` 一致）四 case 全绿

## 2. spec-gate 接线（行为不变，实现搬家）

- [x] 2.1 `.pi/extensions/spec-gate.ts`：检查③改 `runScript(["scripts/harness/check-tasks-tail.sh", changeDir])`，删除 `checkTasksMd` / `TAIL_SECTIONS` / `DOC_IMPACT_MARKER`；`failedChecks.push("tasks")` 语义不变。验证：`spec-gate.smoke.cjs` 既有 7a/7e case 断言不回归（target 仍为 `doc-impact,standards,tasks,trace` 形态）
- [x] 2.2 `buildBlockReason` 「修复指引」前插自查指引行（含 `archive-readiness.sh` 命令形态与「勿补一项就试一次」）。验证：smoke 7a case 新增断言 `reason` 含 `archive-readiness.sh`，`bash .pi/extensions/tests/run-harness-smoke.sh` 全绿

## 3. 文档同步

- [x] 3.1 `docs/reference/开发执行规范.md` §11 归档流程：在归档动作前补「先跑 `bash scripts/harness/archive-readiness.sh <change>` 自查全绿」规范步骤（不新增门禁/豁免）；grep 确认小节内出现该命令形态
- [x] 3.2 `docs/reference/harness/pi-extensions.md` 归档门禁节：补 check-tasks-tail.sh 抽出说明与 archive-readiness.sh 自查入口一行；grep 确认两脚本名出现

## 4. 测试

- [x] T1 `bash scripts/harness/check-tasks-tail.smoke.sh` → exit 0（六 case 全过）
- [x] T2 `bash scripts/harness/archive-readiness.smoke.sh` → exit 0（四 case 全过，覆盖 spec Scenario：全绿/多红汇总/change 不存在/只读安全）
- [x] T3 `bash .pi/extensions/tests/run-harness-smoke.sh` → exit 0（spec-gate 文案新断言过、既有断言零回归）
- [x] T4 真实对账（人工，一次性）：对本 change 自身跑 `bash scripts/harness/archive-readiness.sh harden-archive-readiness` → 四项判定与门禁口径逐项一致（本 change 未收尾前 tasks/trace 项红属预期，验证的是输出与逐项脚本直跑结果一致）

## 5. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；本 change 实改 开发执行规范.md 与 pi-extensions.md 均为流程/机制文档，随本 change 就地同步) -->
<!-- §12.2：无 flow 影响——本 change 仅改 pi harness 脚本/扩展及其机制文档，不触及任何业务 flow -->

- [x] D1 `docs/reference/开发执行规范.md` §11（同 3.1）
- [x] D2 `docs/reference/harness/pi-extensions.md`（同 3.2）

## 6. 验证

- [x] V1 `openspec validate harden-archive-readiness` → 输出 valid
- [x] V2 `bash scripts/harness/check-tasks-tail.smoke.sh && bash scripts/harness/archive-readiness.smoke.sh && bash .pi/extensions/tests/run-harness-smoke.sh` → 全部 exit 0
- [x] V3 `cd backend-go && go vet ./... && go build ./...` → 无告警、构建成功（确认改动未波及后端）
- [x] V4 `git status --short` → 本 change 变更仅含 `scripts/harness/check-tasks-tail.sh`、`scripts/harness/archive-readiness.sh`、两个 `.smoke.sh`、`.pi/extensions/spec-gate.ts`、`.pi/extensions/tests/spec-gate.smoke.cjs`、`docs/reference/开发执行规范.md`、`docs/reference/harness/pi-extensions.md`、`openspec/changes/harden-archive-readiness/`（树上其余脏文件属并行 change，不碰）
- [x] V5 Scenario→测试映射对账：5 个 delta Scenario 逐条注明落点（见上表），`bash scripts/harness/scenario-trace.sh openspec/changes/harden-archive-readiness` 退出码 0
- [ ] V6 效果回检挂账（人工，7 天后）：`bash scripts/harness/harness-retro.sh --days 7` → `m7.block_recur_groups` ≤2 且 concurrent-dirty-tree 复测计数记录在案（体检第三条保留观察项）

| Scenario | 测试文件 |
| --- | --- |
| 全绿通过 | scripts/harness/archive-readiness.smoke.sh |
| 多项红一次性汇总 | scripts/harness/archive-readiness.smoke.sh |
| change 不存在 | scripts/harness/archive-readiness.smoke.sh |
| 只读安全 | scripts/harness/archive-readiness.smoke.sh |
| block 文案含自查指引 | .pi/extensions/tests/spec-gate.smoke.cjs |
