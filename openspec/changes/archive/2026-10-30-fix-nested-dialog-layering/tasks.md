# Tasks: fix-nested-dialog-layering

## 1. 浮层层级契约（单一事实源）

- [x] 1.1 `front/app/components/ui/layout-contract.ts`：新增 `Z_INDEX` 档位表（`dialog` 1000 / `dialogNested` 1100 / `readingDialog` 9100 / `readingFullscreen` 9200 / `toast` 9999）与 `nestedOverlayZ(hostZ: number): number` helper（严格大于 hostZ、跳过已占档位、不越过 `toast`——review R-1 收紧后的口径），头注释写明权威源 `docs/reference/standard/frontend/layout.md`「浮层层级契约」节与「组件不得手写层级数字（存量例外见登记表）」。验证：`cd front && pnpm test:unit app/components/ui/layout-contract.test.ts --maxWorkers=2` → 8 用例全绿（档位顺序单调 + 数值锚 + 撞档锚 + 占位清单锚 + 封顶锚）
- [x] 1.2 `front/app/components/ui/AppDialog.vue`：`zIndex` prop 默认值改为 `Z_INDEX.dialog`（数值仍 1000，渲染行为零变化，只换取值口）。验证：`cd front && pnpm test:unit app/components/ui/AppDialog.test.ts --maxWorkers=2` → 17 用例全绿（含新增「默认遮罩层级 = `Z_INDEX.dialog`」「显式传 z-index 按传入值渲染」）

## 2. 组合创建弹窗层级修复（用户报障点）

- [x] 2.1 `front/app/features/tags/components/CompositeLabelEditDialog.vue`：新增 `zIndex?: number` prop（默认 `Z_INDEX.dialogNested` = 1100），遮罩由 CSS 手写 `z-index: 60` 改为内联 `:style="{ zIndex }"`（样式表里的写死层级删除，避免双源）；弹窗其余结构/文案/校验/创建链零改动。验证：`cd front && pnpm test:unit app/features/tags/components/CompositeLabelEditDialog.test.ts --maxWorkers=2` → 18 用例全绿（含新增「嵌套浮层内联层级严格大于宿主 AppDialog 遮罩内联层级」）
- [x] 2.2 回归确认既有调用方不受影响：`BoardHealthDetail.vue`（治理工作面组合区）与 `CompositeLabelPool.vue`（标签池页）均不传 `zIndex` → 走默认嵌套档；`BoardHealthPanel.test.ts` 的 `CompositeLabelEditDialog` stub 断言不受影响。验证：`cd front && pnpm test:unit app/features/tags/components/BoardHealthPanel.test.ts app/features/tags/components/console --maxWorkers=2` → 50 文件 / 597 用例全绿（子线程实跑）

## 3. 测试

- [x] 3.1 复现先行（bug 复现测试，§2 不可豁免）：先写「组合弹窗渲染层级 vs 宿主 `AppDialog` 遮罩层级」比较用例，在修复前跑出**红**（`AssertionError: expected 0 to be greater than 1000`，`Tests 1 failed | 14 passed`，退出码 1），红输出与实机 A/B 对照（把层级改回 60 → 命中宿主内容）已贴进 `review.md`「复现证据」节；再实施 1/2 节的修复转绿。验证：红→绿两态均有命令输出留档
- [x] 3.2 新增 `front/app/components/ui/layout-contract.test.ts`：① 档位顺序单调断言（`dialog < dialogNested < readingDialog < readingFullscreen < toast`）；② 档位数值锚（防手写漂移）；③ `nestedOverlayZ(hostZ) > hostZ` 且 `nestedOverlayZ(Z_INDEX.dialog) === Z_INDEX.dialogNested`；④ **撞档锚**（`nestedOverlayZ(Z_INDEX.readingDialog) !== Z_INDEX.readingFullscreen`，review R-1 追加）；⑤ 占位清单锚 + 封顶锚。验证：`cd front && pnpm test:unit app/components/ui/layout-contract.test.ts --maxWorkers=2` → 8 用例全绿
- [x] 3.3 扩展 `front/app/features/tags/components/CompositeLabelEditDialog.test.ts`：新增「层级契约」describe——挂载宿主 `AppDialog` + 组合弹窗的组合宿主，读取两个遮罩的**内联** z-index，断言嵌套浮层严格大于宿主遮罩（组件层「不被覆盖」机械断言，vitest 不加载 SFC 样式，必须断言内联值而非 `getComputedStyle`）；另补「浮层展示合理性锚」两条（源码正则锚 + teleport-to-body 锚，review R-3 追加）。验证：`cd front && pnpm test:unit app/features/tags/components/CompositeLabelEditDialog.test.ts --maxWorkers=2` → 18 用例全绿
- [x] 3.4 扩展 `front/app/components/ui/AppDialog.test.ts`：新增「默认遮罩层级取基础档」用例（断言遮罩内联 z-index = `Z_INDEX.dialog`）与「显式传 z-index 按传入值渲染」用例。验证：`cd front && pnpm test:unit app/components/ui/AppDialog.test.ts --maxWorkers=2` → 17 用例全绿
- [x] 3.5 用例文档落地：`test-cases.md` 已随 change 产出（主链路 + 变体走查 + 效果核对 + 继承与调整表），本节只做勾选与「落点文件真实存在」核对（`ls front/app/features/tags/components/CompositeLabelEditDialog.test.ts front/app/components/ui/AppDialog.test.ts front/app/components/ui/layout-contract.test.ts` → 三个文件均在）

## 4. 文档

<!-- doc-impact: standard, flow -->

- [x] 4.1 `docs/reference/standard/frontend/layout.md`：新增「浮层层级契约」Requirement——档位表（dialog 1000 / dialogNested 1100 / readingDialog 9100 / readingFullscreen 9200 / toast 9999）+ 规则「弹窗内再开的浮层 MUST 严格高于宿主弹窗遮罩；宿主在更高档时按 `nestedOverlayZ(hostZ)` 抬升（跳过已占档位、不越过 `toast`）」+ 「新写浮层必须取档位表，不得手写层级数字」+ 存量例外与遗留手写层级登记表 + 组合创建弹窗这次事故一行（2026-10-30 用户报障）。验证：`grep -c "浮层层级契约" docs/reference/standard/frontend/layout.md` 期望 ≥ 1 且 `grep -c "dialogNested" docs/reference/standard/frontend/layout.md` 期望 ≥ 1（实跑均通过）
- [x] 4.2 `docs/reference/flow/semantic-board.md`：业务链路本身不变（无接口/无数据/无状态机改动），仅在「变更溯源」表补本 change 一行（指向 `archive/2026-10-30-fix-nested-dialog-layering`）。验证：`grep -c "fix-nested-dialog-layering" docs/reference/flow/semantic-board.md` 期望 1（**归档后**已补，见 §12.2 与验证节 5.12）

## 5. 验证

- [x] 5.1 `cd front && pnpm lint` → 退出 0、0 error（8 个 warning 全在无关文件：ArticleCardView.test / useArticleContentView / DailyReportTopicSection.test / detective-wall utils.test / schedulerMeta.test；本增量 0 warning）
- [x] 5.2 `cd front && pnpm exec nuxi typecheck` → 实跑退出 0、无类型错误（子线程一度报红，定位为**其它在途 change** 的未跟踪新文件 `BoardRadarStatusBlock.test.ts` / `useRadarPipelineStatus.test.ts` 半成品状态，主线程复跑已恢复绿；本增量文件零类型错误）
- [x] 5.3 `cd front && pnpm test:unit app/components/ui/layout-contract.test.ts app/components/ui/AppDialog.test.ts app/features/tags/components/CompositeLabelEditDialog.test.ts app/features/tags/components/BoardHealthPanel.test.ts --maxWorkers=2` → 退出 0（本 change 影响面；不跑全量，树莓派红线）
- [x] 5.4 实机复核（agent-browser，静态发布实例 `http://localhost:5100`）：打开版块治理工作面 → 「组合」页签 → 点「创建并挂载」→ `elementFromPoint` 探测弹窗中心**命中组合弹窗本体**（`.cld-picker`，`hitInsideNested=true`；宿主 1000 / 嵌套 1100），并截图留档 `ui-verification/A-1-nested-dialog-visible.png`（判定条目见 `ui-verification/report.md`）。A/B 对照：把层级改回 60 → 命中宿主内容（故障态当场复现）
- [x] 5.5 `bash scripts/harness/change-scope.sh` → 退出 0，识别前端 lint/typecheck/test 影响面
- [x] 5.6 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-nested-dialog-layering` → 退出 0（6 个 Scenario 映射齐全；实跑输出「✓ 通过：6 个 Scenario 映射齐全（自动测试 6 / 人工留痕 0）」）
- [x] 5.7 `bash scripts/harness/review-trace.sh openspec/changes/fix-nested-dialog-layering` → 退出 0（实跑输出「✓ 通过：review.md 建账 7 条」）
- [x] 5.8 `bash scripts/harness/ui-verification-trace.sh openspec/changes/fix-nested-dialog-layering` → 退出 0（none 档：实跑输出「✓ 通过：none 档 N/A 一致性」）
- [x] 5.9 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-nested-dialog-layering` → 退出 0（实跑输出「通过（声明: standard, flow  文件:2 个）」）
- [x] 5.10 `bash scripts/harness/check-standards.sh --change fix-nested-dialog-layering` → A-D/F/G 段零失败（实跑「通过 252 / 失败 0」；E 段 flow 溯源在归档后补，3 天宽限期内）
- [x] 5.11 `bash scripts/harness/archive-readiness.sh fix-nested-dialog-layering` → 六项全绿（实跑「✓ 六项全绿，可以 openspec archive」）→ 已执行 `openspec archive fix-nested-dialog-layering --yes` → 归档为 `2026-10-30-fix-nested-dialog-layering`，主 spec `unified-dialog` 同步（+1 added / ~1 modified）
- [x] 5.12 归档后按 §12.2 补 `docs/reference/flow/semantic-board.md` 变更溯源行 → 已补（2026-10-30 / fix-nested-dialog-layering 行，指向 `archive/2026-10-30-fix-nested-dialog-layering`）；再跑 `bash scripts/harness/check-standards.sh` → 本 change 溯源行「宽限期内免检」无失败（详见下方「域外红」）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 弹窗内嵌套浮层高于宿主遮罩 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 嵌套浮层档位可按宿主抬升 | front/app/components/ui/layout-contract.test.ts front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 层级档位单一事实源 | front/app/components/ui/layout-contract.test.ts front/app/components/ui/AppDialog.test.ts front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 默认遮罩层级取基础档 | front/app/components/ui/AppDialog.test.ts |
| 新弹窗使用四档尺寸 | front/app/components/ui/AppDialog.test.ts |
| 主题自动适配 | front/app/components/ui/AppDialog.test.ts |

## 6. Review

- [x] 6.1 派 `change-review-glm` 子线程聚焦 review（审制品 + diff），High/Medium/Low 意见处置后 `review.md` 建账（复现红输出、契约取值合理性、回归面、文档口径一并记）
  - 实跑：审查子线程判定 **0 High / 4 Medium / 3 Low**；4 条 Medium（R-1 撞档 / R-2 存量无条件 MUST / R-3 缺展示合理性锚 / R-4 建账与红态留痕）全部已修复并复验，3 条 Low 中 R-5/R-6 已修、R-7（主 spec 同步）随归档执行。处置明细见 `review.md`。

## 7. 域外红登记（§11.4）

- 归档自检 `bash scripts/harness/check-standards.sh`（全量）撞见 2 条 **doc-impact 门禁红**，归属**其它在途 change**（`add-question-driven-news-research` / `board-health-triage-advice`），非本 change 引起：本 change 自身 `doc-impact verify` 实跑退出 0、`--change fix-nested-dialog-layering` 零失败。
- 已用 `bash scripts/harness/test-patrol.sh --register` 登记台账：`doc-impact:add-question-driven-news-research`、`doc-impact:board-health-triage-advice`（context = fix-nested-dialog-layering，domain = manual，status = open）。
