# Seam C2 复审修复记录（tasks 3.2 / 3.3）

> board-health-triage-advice seam C 第 2 批的**独立只读审查**修复。
> 审查对象：不可变快照 `/tmp/board-health-c2-review-B4ApztsQ`（17 文件；manifest 比对确认 `semanticBoards.ts`
> 哈希与 C1 一致 → wire 未动）。
> 报告：`…/outputs/a29dfeed-701a-4b41-bbf6-ff6f73b496bf/.pi/subagents/board-health-c1-independent-review.779b5d47.md`
> 判定：**修复后放行（4 Medium + 2 Low；无 High）**。模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。
> 本批**只改** C2 组件 + `boardHealthWorkspace.ts` + C1 最小必要处 + 精准测试 + 本 change 账本；
> **未改**宿主、后端、其它 change 脏文件；未做任何 Git 写操作；未跑 build / 浏览器 / 服务 / 部署 / 全量测试。

## 0. 勾选处置（账本纪律）

| 时点 | 动作 |
| --- | --- |
| 审查判定「修复后放行」后 | **tasks 3.2 / 3.3 勾先撤**（`[x]` → `[ ]`），本记录建账 |
| 本批修复完成且新增回归全绿后 | **恢复 3.2 / 3.3 勾**（见 tasks.md 该行的新证据） |

## 1. 已接受问题与处置

### M1（Medium）降级轮次被误标「历史快照」，`'degraded'` 分支在真 wire 下不可达

- **后端事实（先读后改）**：`backend-go/internal/tagmanagement/service/boardhealth/run.go:95`
  —— `metrics.Triage = convertTriage(result.Triage) // degraded rounds keep triage nil (JSON null)`。
  降级轮（LLM 裁决失败）= `llm_score` **与** `llm_comment` 均 NULL，且 `metrics.triage` 为 **JSON null**；
  `convertTriage` 对空清单同样返回 nil（构成标签为空的边界轮）。
  原实现把 `!triage.available` 判在降级判定**之前** → 真降级轮显示「历史快照」，`degraded` 分支不可达；
  测试夹具又只造了 `triage: []`（真 wire 从未走到）。
- **处置**（`BoardHealthWorkspace.vue` `workspaceState`）：
  - 顺序改为：`loading` → `error`（且**无**旧快照）→ `no-snapshot` → **降级**（`llm_score === null && llm_comment === null`）
    → `old-snapshot`（有评分无 triage）→ `no-triage`（triage 空数组）→ `normal`；
  - 「历史快照」只留给**有评分无 triage**；降级轮显示「本轮未生成评分与分诊…不能据此推断当前标签健康」；
  - 夹具不再用 `triage: []` 冒充降级：降级用例改真 wire（`triage: null` + score/comment null）。
- **回归**：`BoardHealthWorkspace.test.ts` →
  「降级轮次（真 wire：score/comment 均 NULL 且 metrics.triage=null）：明说不能推断健康」（并断言**不含**「历史快照」）、
  「历史快照（有评分但无 triage 字段）：与降级轮区分开」。

### M2（Medium）正常轮次没有「本轮体检依据」折叠区，评分/统计/趋势不可达

- **依据**：`ui-design.md`「评分、统计、趋势在『本轮体检依据』按需展开」+ 已批原型正常态也渲染 `basisHTML()`；
  handoff §3 承接表亦按该折叠区承诺。
- **处置**：`showBasis` 由「三种异常态」改为 **「快照存在即渲染」**（默认折叠）；
  折叠区内容仍是既有 `BoardHealthSignal`（零额外请求、零重复统计实现）；异常态行为不变。
- **回归**：「正常轮也有「本轮体检依据」折叠区（评分/统计/趋势按需可达，默认折叠）」。

### M3（Medium）remove 项处理区无证据

- **依据**：已批原型对**每项**都渲染「为什么建议处理 · 展开证据」；ui-design 把证据列为当前项必备。
- **处置**：证据 `<details>` 从 rewrite 分支**移到四形态共用**位置（`v-if="!task"`：有任务时由执行态的
  「折叠修改摘要」承载证据，避免重复；remove / 未知态 / 可编辑态都能展开磁铁·最近邻·描述预筛·失败挂载流向）。
- **回归**：「remove 项也能展开证据（磁铁/流向）再确认摘除」（夹具带 `magnet_flow`，断言「失败挂载流向」可见）。

### M4（Medium）非 409 的提交失败被吞，用户零反馈；范围未知时仍可点确认

- **处置**：
  1. 根保存 `submitRepair()` 返回的 `error` → 新增 `submitFailure` 状态 → 详情区渲染**独立错误行**
     （`data-testid="bhw-submit-failure"`，与 conflict 行并列；服务端 message **原样透传**，不编文案）；
  2. **行内确认失败时不静默关闭**：`confirmSubmit()` 只在 `result.ok` 时收起确认块，失败则保留确认块并显示错误行；
  3. **范围未知/未加载/读取失败 → 禁用确认按钮并说明**（编辑器新增 `scopeBlockReason`：
     读取中 / 读取失败（带服务端原因）/ 尚未加载 / 范围未知（服务端未提供），
     同时禁用「修复描述并更新匹配」与最终确认按钮）。
- **回归**：「非 409 提交失败（如 404）：独立错误行透传服务端 message，行内确认不静默关闭」、
  「共享范围未知：禁用确认按钮并说明原因（不谎称 0 范围）」。

### L1（Low）切项瞬间编辑器闪现上一项草稿

- **处置**：根的 `selectIssue()` 在 `await repair.loadContext()` **之前**先 `repair.setDraft('')`（切项时；
  重新选中同一项仍保留草稿），`draftTouched` 语义不变（仍只用于"预填建议不覆盖用户输入"）。
- **回归**：「切项先清草稿：编辑器不闪现上一项内容」（第二项上下文在途时断言 textarea 值为空串）。

### L2（Low）stale 用例未断言提示行

- **处置**：测试改为**真正触发一次同版块刷新失败**（宿主可用新增的 `reload()` 暴露触发），
  断言 `bhw-stale-hint` 出现且含「显示上次快照」与「核对实时版本」，并断言旧快照仍在（未清空成空态）。
  顺带把工作面的读取失败判定改为「**无旧快照**才进错误态」——有旧快照时按 stale 保留上次快照
  （与既有 `useBoardHealth` 语义一致），否则刷新失败会把仍可用的快照藏掉。
- **回归**：「刷新失败保留上次快照（stale）：给出提示行且旧快照仍在」。

## 2. 顺带补的最小能力（3.4 需要）

- `defineExpose({ reload })`：`reload()` = 同版块刷新体检快照（失败按 stale 处理，**不创建任何任务**）。
  既给 3.4 一个刷新入口，也让 stale 提示行成为可达状态（此前工作面内没有同版块刷新触发点）。

## 3. 验证

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd front && pnpm test:unit app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.repair.test.ts app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts --maxWorkers=2` | **ok**：3 文件 / **51 例**全 PASS（修复前 40 例） |
| 2 | `cd front && pnpm test:unit app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` | **ok**：40 例全 PASS（本批未改 C1 composable 行为） |
| 3 | `cd front && pnpm test:unit app/api/normalizers/auxiliaryRepair.test.ts app/api/normalizers/boardHealthTriage.test.ts app/api/auxiliaryLabels.test.ts app/features/tags/components/BoardHealthPanel.test.ts app/api/normalizers/consoleRail.test.ts app/api/semanticBoards.test.ts --maxWorkers=2` | **ok**：6 文件 / 69 例全 PASS（C1 归一器 + 旧件回归） |
| 4 | 合并跑 #1+#2+#3（10 文件） | **ok**：10 文件 / **160 例**全 PASS |
| 5 | `cd front && npx eslint app/features/tags/components/console/board-health/` | 0 问题 |
| 6 | `cd front && pnpm exec nuxi typecheck` | 本批文件**零错误**；全局 20 处报错全部落在**其它 change 并行中的脏文件**，本批未触碰未修、**不宣称全局绿** |

未跑：`pnpm build` / 静态部署 / 浏览器 / 全量前端测试 / 后端门禁（按本批边界）。

## 4. 未做 / 留给下一批

- **3.4 宿主接线**未做（`BoardWideWorkface` 的 health 页签仍渲染 `RailHealthBrief`）——本批只交付组件与契约；
- **3.5 真实几何/截图**未做（本批只保证断点与行为分支正确，JSDOM 不构成尺寸验收）；
- 逐标签质量表是否也进工作面、`magnet_flow` 展示密度、顶部「已处理 N 项」口径等未决项仍按 handoff §7 挂着。
