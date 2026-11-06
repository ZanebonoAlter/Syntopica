# Seam C3 交接 — 宿主接线（tasks 3.4）

> board-health-triage-advice seam C 第 3 批：把 C2 的问题处理工作面**接进既有宿主**，
> 并把摘除走**既有 DELETE 链**（行内确认 / 取消 / 失败重试 / 明确「历史匹配不会自动回填」）。
> 承接 seam-c1（C1 API 层）与 seam-c2（C2 工作面组件，已由独立复审放行）。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。
> 本批**未改后端**、未改共享 client、未改真库；未启动服务/未构建/未部署/未跑浏览器/未跑全量测试；
> **未做任何 Git 写操作**。

## 1. 改动文件

| 文件 | 改动 |
| --- | --- |
| `front/app/features/tags/components/console/BoardWideWorkface.vue` | **体检依据页签改渲染 `BoardHealthWorkspace`**（替换原 `RailHealthBrief variant=full`）；新增 prop `removedAuxIds` 与 emit `remove-composition`；旧「构成/组合/候选/来源/设置」页签**原样保留** |
| `front/app/features/tags/components/console/BoardConsoleShell.vue` | 新增 `removedAuxIds` 状态与 `handleRemoveComposition()`（**复用既有 remove 链**）；把 `removedAuxIds` 与 `@remove-composition` 接到工作面；右轨「完整评语与依据」落点改到工作面「体检依据」页签（获批原型的落点）；切版块清空 `removedAuxIds` |
| `front/app/features/tags/components/console/useConsoleRail.ts` | `removeComposition()` 返回**是否成功**（原为 void），供宿主回填「已摘除」反馈 |
| `front/app/features/tags/composables/useBoardComposition.ts` | `removeLabel()` 返回**是否成功**（原为 void）；通知仍只在这里发一次（成功/失败各一次），调用方不叠加 toast |
| `front/app/features/tags/composables/useBoardHealth.ts` | **透出 `errorStatus`**（最近一次失败的 HTTP 状态码），供消费方区分「刷新失败（网络/5xx → stale）」与「资源不存在/无权（404/403）」 |
| `.../board-health/boardHealthWorkspace.ts` | 状态判定新增 **`gone`** 形态（404/403 → 版块可能已不存在，不当作 stale）；新增 `errorStatus` 入参 |
| `.../board-health/BoardHealthWorkspace.vue` | 传入 `errorStatus`；`showStale` 在 `gone` 形态下**不再**显示误导性 stale 提示行 |
| `.../board-health/boardHealthWorkspace.model.test.ts` | 新增 404/403 → `gone` 的纯函数用例 |
| `.../board-health/BoardHealthWorkspace.repair.test.ts` | 新增 404 用例：不再出现「刷新失败显示上次快照」，旧清单不再可操作 |
| `front/app/features/tags/components/console/BoardWideWorkface.test.ts` | 体检页签断言改指向工作面；新增接线用例（boardId/boardLabel/removedAuxIds 传入 + `remove-composition` 外抛） |
| `front/app/features/tags/components/console/BoardConsoleShellRepair.test.ts` | **新增**：壳层摘除接线用例（工作面 → 既有 DELETE 链 → 成功才回填 `removedAuxIds`；失败不回填） |
| `front/app/features/tags/components/console/boardConsoleShellTestHarness.ts` | 工作面桩补 `removedAuxIds` prop 与 `remove-composition` emit（可触发摘除） |

**未改动**：`RailHealthBrief.vue` / `BoardContextRail.vue`（右轨 brief 保持轻结论，**不塞清单**）、
`BoardHealthDetail.vue` / `BoardHealthSignal.vue` / `BoardHealthPanel.vue`（旧面板与统计实现）、
后端、共享 ApiClient、stores、全局 theme/CSS、其它 change 的脏文件。

## 2. 接线点与事件流

```
L1 右轨 RailHealthBrief（variant=brief，轻结论 + 「完整评语与依据」按钮）
  └─ @open-detail → 壳层 openWorkface('health')            ← 落点改为「体检依据」页签（获批原型同）
       └─ BoardWideWorkface（既有 1040px AppDialog size=xl 单级工作面）
            └─ health 页签 → BoardHealthWorkspace（C2 工作面）
                 ├─ remove-composition(auxId: string)
                 │    → 壳层 handleRemoveComposition
                 │         → Number(auxId) → 既有 removeComposition()
                 │              → useBoardComposition.removeLabel()
                 │                   → DELETE /semantic-boards/:boardId/composition/:auxId
                 │                   → 成功：notify 一次 + 重取构成 + onRemoved
                 │                   → 失败：notifyError 一次（不叠加 toast）
                 │              → health.load()（体检重取）
                 │         → 成功才回填 removedAuxIds（失败不谎称已摘除）
                 └─ 其它页签（构成/组合/候选/来源/设置）→ 原样复用既有真实面板
```

- **摘除语义**（沿用 C2 工作面形态 + 宿主既有 remove 链）：行内确认 → 取消（零请求）→ 确认发一次；
  失败由既有链通知一次（`useBoardComposition.removeLabel` 内），壳层**不**再弹 toast；
  **防重复提交**由 C2 工作面侧（确认块 + `removeConfirmOpen`）承担，宿主不另加并发锁。
- **「已摘除」只由真实成功驱动**：工作面收到 `removedAuxIds` 才显示「已摘除」；未回传前不标（C2 已实现该侧）。
- **历史匹配不自动回填**：既有链只删构成关系；工作面文案与既有 `removeLabel` 通知都保留这句语义
  （既有通知文案「已从版块构成移除」+ 工作面「历史匹配尚未重算，需要时仍须通过既有手动回填处理」）。

## 3. 旧能力 → 去向表（HEAD 真实入口为证据）

| 旧能力 | HEAD 真实入口 | 本批后的去向 |
| --- | --- | --- |
| 体检依据页签的完整评语/统计/趋势 | `BoardWideWorkface` health 页签 → `RailHealthBrief variant=full` | **换渲染**为 `BoardHealthWorkspace`；评语/统计/趋势收进工作面内**默认折叠的「本轮体检依据」**（复用 `BoardHealthSignal`，零额外请求） |
| 右轨「完整评语与依据」落点 | 壳层 `@open-health="openWorkface('quality')"` | 改为 `openWorkface('health')`（获批原型即落在「体检依据」页签）；「构成」页签仍在工作面页签条上，**入口未丢** |
| 右轨轻结论 | `BoardContextRail` → `RailHealthBrief variant=brief` | **不动**（仍只给结论 + 一句摘录 + 入口，不塞问题清单） |
| 构成 / 组合 / 候选 / 来源 / 设置页签 | `BoardWideWorkface` 的 `BoardHealthDetail`（mode=quality/composites/candidates）、`BoardSourcePanel`、`BoardSettingsForm` | **原样保留**（本批只换 health 页签的渲染） |
| 逐标签质量表 / 组合挂载 / 未收编候选 | `console/BoardHealthDetail.vue` | 仍在既有页签（工作面不复制统计实现、不空占位） |
| 摘除构成（真实 DELETE） | `useBoardComposition.removeLabel` → `removeFromComposition` | **复用**（工作面只发事件；宿主转数字后走同一条链；成功才回填「已摘除」） |
| 体检快照 / 趋势读取 | `useBoardHealth`（`GET /semantic-boards/:id/health-checkup`） | 复用；本批只**透出 `errorStatus`**（区分 404/403 与网络/5xx） |

## 4. 新 Low（本批随修）：404/403 不得被说成「刷新失败」

- **问题**：`useBoardHealth` 不看状态码 → 版块已删（404/无权 403）带旧快照时被判 stale，
  界面说「刷新失败，显示上次快照」，且旧清单**仍可操作**（误导 + 越权风险）。
- **处置**：`useBoardHealth` 透出 `errorStatus`；工作面纯函数在 **404/403** 时判 **`gone`** 形态：
  标题「这块内容可能已经不存在」、说明「体检数据读取被拒绝或版块已删除（404/403）…此处不再展示可操作的清单」，
  **不展示问题索引/处理区**（清单不再可操作）；**网络错/5xx 仍按 stale**（保留上次快照 + 提示行）。
  写入侧不变：描述修复必经 C1 实时 context + 版本校验。
- **回归**：组件用例「404（版块可能已删除）：不再说「刷新失败显示上次快照」，旧清单不再可操作」+
  纯函数用例（404/403 → `gone`）。

## 5. 验证

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd front && pnpm test:unit app/features/tags/components/console/BoardWideWorkface.test.ts app/features/tags/components/console/BoardConsoleShellRepair.test.ts app/features/tags/components/console/BoardContextRail.test.ts app/features/tags/components/console/BoardConsoleShell.test.ts app/features/tags/components/console/BoardConsoleShellStates.test.ts app/features/tags/components/console/BoardConsoleShellNavigation.test.ts app/features/tags/components/console/BoardConsoleShellSelection.test.ts --maxWorkers=2` | **ok**：7 文件 / **60 例**全 PASS（宿主 + 旧件回归） |
| 2 | `cd front && pnpm test:unit app/features/tags/components/console/BoardConsoleShell.integration.test.ts app/features/tags/components/BoardHealthPanel.test.ts app/features/tags/composables/useBoardComposition.test.ts app/features/tags/components/console/board-health/ app/api/normalizers/auxiliaryRepair.test.ts app/api/normalizers/boardHealthTriage.test.ts app/api/normalizers/consoleRail.test.ts app/api/semanticBoards.test.ts app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` | **ok**：11 文件 / **168 例**全 PASS（C2 工作面 + C1 + 旧件） |
| 3 | `cd front && npx eslint app/features/tags/components/console/ app/features/tags/composables/useBoardHealth.ts app/features/tags/composables/useBoardComposition.ts` | 0 问题 |
| 4 | `cd front && pnpm exec nuxi typecheck` | 本批改动行**零错误**；全局仍有外部红：`BoardConsoleShell.vue:173`（`isNarrow` 类型：`useConsoleShellOverlays.ts` 要求 `ComputedRef<boolean>` 而壳层传 `Ref`——该文件由**另一 change** 在本批期间重写引入，与本批改动行 202/208/342/397/419/423 无关）、`useConsoleReading*.ts`、`ObservationMatrixGrid.test.ts`、`OtherTopicReading.signals.test.ts`、`useConsoleReturnContext.test.ts`、`SignalReportView.test.ts`——**未触碰未修，不宣称全局绿** |

> 未跑 `pnpm build` / 静态部署 / 浏览器 / 全量前端测试 / 后端门禁（按本批边界）。

## 6. 3.5 与 5.3 缺口

- **3.5 几何/截图**：真实几何与双视口（1440×900 / 1920×1080）+ 窄屏（375×667 / 390×844）验收**未做**；
  本批只落地基础响应式（767.98px 断点 + 粘性动作条），**JSDOM 不构成尺寸验收**。
- **5.3 真机故事**：agent-browser 走真后端（一次性 PG 隔离）的完整流程
  （右轨 → 工作面 → 核对 → 修复任务 → 匹配结果 → 待下轮验证）**未做**；本批的组件故事是 mock API 层的**前端**断言。
- **摘除真机**：真实 DELETE（`removeFromComposition`）在真后端上的行为仍待 5.3 验证。

## 7. 未决 / 需上层裁定

1. **右轨落点改动**：`openWorkface('quality')` → `openWorkface('health')` 是本批唯一的行为改动（获批原型同款），
   若认为应保留旧落点，回退只需改壳层一行（工作面仍可从页签条进入）。
2. **「已摘除」反馈的生命周期**：~~`removedAuxIds` 目前只增不减（切版块清空）；同一标签被重新挂回后不会自动撤销标记~~
   → **4.1 C3-L1 已收口**：徽标改为「会话摘除记录 ∩ 不在当前构成」，重新挂回后徽标自动撤销、摘除按钮恢复（详见 §8）。
3. **404/403 的宿主侧处理**：工作面只呈现「可能已经不存在」；宿主是否需要连带关闭工作面/跳回列表，待 3.4 之外的裁定。

## 8. 4.1 正式评审 Low 收口（C3-L1 / C3-L3）

### C3-L1：「已摘除」徽标 = 会话摘除记录 ∩ 不在当前构成

- **问题**：原实现只用宿主回填的 `removedAuxIds`（只增不减）判定「已摘除」——同一标签被**重新挂回**后，
  徽标仍在、摘除按钮仍灰着，用户看着一个已被撤销的「已摘除」且无法再操作。
- **处置**：`BoardWideWorkface` 持**当前构成**（`labels` + `composites`，壳层的 `compositionLabels` 同源），
  计算 `effectiveRemovedAuxIds = removedAuxIds.filter(id => 不在当前构成)` 再传给工作面；
  **会话记录本身不撤销**（仍可用于其它会话内语义），徽标按当前构成取差集 → 重挂后徽标自动撤销、按钮恢复。
- **改动文件**：`front/app/features/tags/components/console/BoardWideWorkface.vue`（**未动壳层**：`BoardConsoleShell.vue`
  的 `removedAuxIds` 状态与脏行 174/229/230 均未触碰）。
- **验证**：`BoardWideWorkface.test.ts` 新增「重新挂回后『已摘除』徽标自动撤销」用例（辅助标签与组合挂载两种重挂都覆盖）。

### C3-L3：工作面与右轨各持一次幂等 GET health-checkup（已知行为，doc-only）

- **事实**：右轨（壳层 `useConsoleShellRail` → `useBoardHealth`）与体检工作面（`BoardHealthWorkspace` → 自己的
  `useBoardHealth`）**各持一个独立实例**，因此进入工作面时同一只读端点 `GET /semantic-boards/:id/health-checkup`
  会被请求两次。两次都是**只读、幂等**的同一请求（端点 MUST NOT 写库、也不触发体检执行，见
  `specs/board-health-checkup`「体检数据查询端点只读」），无副作用、无状态分歧风险。
- **处置**：**记录为已知行为，本批不改**（合并成一次请求需要跨宿主/工作面的缓存共享，属另一个 seam 的结构改动，
  不在 4.1 复审修复范围）。台账见 `review.md`「4.1 正式评审」节与 `review-4.1-fixes.md`。
