<!-- review: fix-nested-dialog-layering -->
<!-- 审查方式：change-review-glm 子线程只读聚焦 review（审制品 + 实现 diff），2026-10-30 -->

# Review 账本

结论：**0 High / 4 Medium / 3 Low**（审查子线程判定）。4 条 Medium 全部已修复并复验；3 条 Low 中 2 条已修（映射表补文件、效果核对改为实测数据）、1 条为归档期动作（主 spec 同步，随归档执行）。

审查范围：change 制品（proposal / delta spec / test-cases / tasks / ui-design）+ 实现增量（`layout-contract.ts` 与其新测试、`AppDialog.vue`/`.test.ts` 的层级相关 hunk、`CompositeLabelEditDialog.vue`/`.test.ts`、`layout.md` 的层级契约节）。`AppDialog` 的 `semanticDialog` 增量与 `layout.md` 的「UI 复审」「已登记例外」「AppDrawer 速查行」等 hunk 属**其它在途 change**的脏改动，不在本次范围。

## 复现证据（先红后绿，§2 不可豁免）

- **红态（修复前，子线程实现时实跑）**：`cd front && pnpm test:unit app/features/tags/components/CompositeLabelEditDialog.test.ts --maxWorkers=2` → `× 嵌套浮层内联层级严格大于宿主 AppDialog 遮罩内联层级` / `AssertionError: expected 0 to be greater than 1000`，`Test Files 1 failed (1) | Tests 1 failed | 14 passed (15)`，退出码 1。
  - 红值为 0 而非 60 的原因：vitest 未开启 css 处理，`<style scoped>` 不进 DOM，样式表里的 `z-index: 60` 读不到，遮罩无内联层级（`Number('') === 0`）——方向与浏览器一致（60 < 1000 同样被盖）。
- **绿态（修复后）**：同命令 + `layout-contract.test.ts` + `AppDialog.test.ts` → `Test Files 3 passed (3) | Tests 43 passed (43)`，退出码 0。
- **实机 A/B（agent-browser，静态实例 `http://localhost:5100/tags/1974`）**：治理工作面「组合」页签点「创建并挂载」后，宿主遮罩内联层级 1000、嵌套浮层内联层级 1100，`elementFromPoint` 探测弹窗中心命中 `.cld-picker`（嵌套浮层本体，`hitInsideNested=true`）；**同页把嵌套层级改回修复前的 60 再探测 → 命中 `.bhd-section`（宿主工作面内容，`hitInsideNested=false`）**，即故障态当场复现。截图见 `ui-verification/A-1-nested-dialog-visible.png`。

## 核实记录（审查子线程「已确认无问题清单」，主线程复核）

1. **根因成立**：宿主 `BoardWideWorkface.vue:112` 为 `<AppDialog size="xl">`（1000），`CompositeLabelEditDialog` 与 AppDialog 遮罩同为 `body` 直接子节点；全库无 `transform`/`filter`/`contain` 锁层，`backdrop-filter` 只作用于兄弟自身 → 60 < 1000 被盖、1100 > 1000 可见，链路完整。
2. **档位取值与存量真值一致**：9100（`MarginNotesRail.vue:125`、`ArticlePreviewModal.vue:38`）、9200（`ArticlePreviewModal.vue:39`）、9999（`AppTooltip.vue:94` / `FeedActionMenu.vue:127` / `NotifyContainer` `z-[9999]`）、1000（AppDialog 默认）逐一比对相符；**1100 与存量无冲突**（全库 z-index 清扫后 (1000, 1100] 区间无占用）。
3. **回归面干净**：1100 新盖住的只有页面级 chrome（topbar 30 / bottombar 40 / 全局 header 900 / 编排遮罩 900 / 矩阵标记 1000 / 窄屏抽屉 1000-1001），均属模态弹窗应盖对象；toast/tooltip/通知 9999 仍在其上；`UpgradeSuggestionPanel`(100) / `TagMergePreview`(78) 只在页面级 `BoardRadarGovernance`(60) 内打开，不受影响。
4. **测试有效性不漏网**：若把内联绑定删掉、写回 CSS `z-index: 60`，遮罩 `style.zIndex` 为 `''` → `Number('') === 0`，`toBe(Z_INDEX.dialogNested)` 与 `toBeGreaterThan(hostZ)` 双断言均红。
5. **口径一致**：delta spec（ADDED 3 + MODIFIED 3 = 6 Scenario）↔ tasks 映射表 ↔ test-cases.md 主链路对得上；`layout.md` / `ui-design.md` / `layout-contract.ts` 三处档位数值一致；`AppDialog` 默认值改取档位但数值不变（行为零变化）。

### R-1 2026-10-30【Medium】`nestedOverlayZ(9100)` 与 `readingFullscreen` 9200 撞档，抬升值可能落已占档位

- **状态**：已修复（`nestedOverlayZ` 改为「跳过已占档位」的抬升：档位表是唯一占用清单，裸抬升撞 9200 时继续抬升落到 9300；`dialogNested` 作为嵌套浮层自己的落位档不算被占；并加 `toast` 封顶护栏。`layout-contract.test.ts` 补 3 条护栏断言：撞档锚 / 占位清单锚 / 封顶锚。`layout.md` 规则 2 与 delta spec「嵌套浮层档位可按宿主抬升」Scenario 同步为「返回值严格大于 hostZ 且不等于任何其它已占档位、且不超过 `Z_INDEX.toast`」）
- **遗留边界（已知并留痕）**：宿主本身 ≥ 9899 时封顶返回 `toast` 档，与全局提示同层（tie-break 退回 DOM 顺序）——属契约边界（不该在提示层宿主里再开浮层），已在代码注释与 `layout.md` 写明。

### R-2 2026-10-30【Medium】「组件不得手写层级数字」是无条件 MUST，与存量大面积手写现实脱节且未登记遗留

- **状态**：已修复（`layout.md`「浮层层级契约」Requirement 正文补**存量例外**（存量保持现状，主动重构该组件时 SHALL 收敛到档位表）；「存量迁移」节补层级同款条款 + **遗留手写层级登记表**（11 处：`MarginNotesRail` 9100、`ArticlePreviewModal` 9100/9200、`AppTooltip` / `FeedActionMenu` / `NotifyContainer` 9999、`AppSidebarDrawer` 1000/1001、`DateAssociationMarker` 1000、`AppDrawer` 60/61、`UpgradeSuggestionPanel` 100、`TagMergePreview` 78、`BoardRadarGovernance` 60）；delta spec Requirement 同步补存量例外句）

### R-3 2026-10-30【Medium】被修组件缺「浮层组件展示合理性锚」，定位规则无测试守护

- **状态**：已修复（`CompositeLabelEditDialog.test.ts` 补两条锚：① 源码锚——`.cld-overlay` 规则含 `position: fixed` + `inset: 0` 且**不含** `z-index`（层级已内联注入，避免双源）；② teleport-to-body 锚——不 stub teleport 时 `document.body.querySelector('.cld-overlay')` 命中且 `parentElement === document.body`，unmount 后 body 清空）
- **口径联动**：既有 Requirement「浮层组件展示合理性锚」第 1 条已由实现子线程改写为「层级按浮层层级契约由档位表内联绑定注入时，`<style>` 不再写死 `z-index`」——**主线程判定：保留该改写**（否则新契约与旧锚自相矛盾：一边要求内联注入、一边要求样式表必须写 z-index），改动已在本账本留痕。

### R-4 2026-10-30【Medium】复现红态零留档、review-trace 现红（建账缺失）

- **状态**：已修复（本文件即为 review.md 建账；复现红输出与实机 A/B 证据已记入「复现证据」节。说明：红态无法回放——修复已落地，红态证据以实现子线程当时的真实输出 + 实机把层级改回 60 的 A/B 对照共同留档，两者方向一致（层级低于宿主即被盖））

### R-5 2026-10-30【Low】test-cases.md「效果核对」预填实测结论但 5.4 未执行、无留痕

- **状态**：已修复（5.4 已实跑：agent-browser 打开静态实例 → 治理工作面 → 组合页签 → 创建并挂载，`elementFromPoint` 命中弹层本体，宿主 1000 / 嵌套 1100；A/B 对照把层级改回 60 → 命中宿主内容。截图 `ui-verification/A-1-nested-dialog-visible.png`，判定条目见 `ui-verification/report.md`；test-cases.md「效果核对」节改为实测数据）

### R-6 2026-10-30【Low】Scenario 映射表少列组件层证据文件

- **状态**：已修复（tasks.md「嵌套浮层档位可按宿主抬升」行补 `front/app/features/tags/components/CompositeLabelEditDialog.test.ts`——组件层 `RaisedLayerHost`（真挂 AppDialog@9100 + 组合弹窗按 `nestedOverlayZ` 传入）是该 Scenario 的实质证据）

### R-7 2026-10-30【Low】主 spec 归档同步提醒（`unified-dialog` 约束行与 API Props 段仍是旧口径）

- **状态**：已采纳（归档时执行，非代码缺陷）：① `openspec/specs/unified-dialog/spec.md` 的「不支持嵌套 dialog（如果业务需要，后续再加 z-index 管理）」约束行改为引用层级契约；② API Props 段补 `zIndex?: number`（默认 `Z_INDEX.dialog`）。注：`semanticDialog` / `dialogLabel` 两个 prop 属**另一在途 change**（rework-workshop-into-radar-and-desk），未随本次归档入主 spec，避免替他人归档未完成的工作）

## 归档期动作

- 归档（`openspec archive fix-nested-dialog-layering`）后按 §12.2 补 `docs/reference/flow/semantic-board.md` 变更溯源行，再跑 `bash scripts/harness/check-standards.sh` E 段零失败。
