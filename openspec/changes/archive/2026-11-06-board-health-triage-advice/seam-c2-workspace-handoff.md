# Seam C2 交接 — 问题处理工作面组件（tasks 3.2 + 3.3）

> board-health-triage-advice seam C 第 2 批：把**获批 v2 原型**做成真实组件 + 完整组件故事。
> 承接 seam-c1（C1 API/归一器/轮询 composable，已由独立复审定点通过）与 seam-c1-review-fixes.md。
> **本版含 C2 独立复审（779b5d47，判定「修复后放行」）的全部处置**：4 Medium + 2 Low 逐条见 `seam-c2-review-fixes.md`。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。本批**未改后端、未改宿主路由、未启动/未部署/未跑浏览器**。
> 已批准基线：`ui-design.md`（`ui-approval: approved`，用户「这个可以」）+ `ui-prototype/index.html` v2。

## 1. 改动文件

| 文件 | 改动 |
| --- | --- |
| `front/app/features/tags/components/console/board-health/BoardHealthWorkspace.vue` | **新增**：工作面根（主从布局 + 选中/草稿/行内确认/摘除事件；两个 composable 都在**根**创建） |
| `.../board-health/BoardHealthWorkspaceHead.vue` | **新增**：顶部一行（待核对数/当前进展）+ 非正常轮次说明 + stale/dropped 提示行 + 折叠区「本轮体检依据」（复用 `BoardHealthSignal`） |
| `.../board-health/BoardHealthIssueIndex.vue` | **新增**：左侧约 240px 问题索引（只列 remove/rewrite）+「其余标签」折叠 |
| `.../board-health/BoardHealthIssueDetail.vue` | **新增**：右侧当前项处理区（编辑器 / 只读执行态 / 未知结果 / 摘除四形态互斥） |
| `.../board-health/BoardHealthRepairEditor.vue` | **新增**：描述编辑器（三规则就地反馈 + 共享范围只读 + 行内确认） |
| `.../board-health/BoardHealthRepairProgress.vue` | **新增**：三真实阶段 + 真实计数 + 恢复动作 + 折叠修改摘要 |
| `.../board-health/boardHealthWorkspace.ts` | **新增**：视图模型与纯派生（拆分、状态映射、三阶段推导、计数格式化、进展文案） |
| `.../board-health/BoardHealthWorkspace.test.ts` | **新增**（14 例） |
| `.../board-health/BoardHealthWorkspace.repair.test.ts` | **新增**（10 例） |
| `.../board-health/boardHealthWorkspace.model.test.ts` | **新增**（13 例；纯派生单测） |
| `front/app/api/normalizers/boardHealthTriage.ts` | **最小扩展**：新增 `normalizeBoardHealthMagnetFlow`（服务端已算好的流向只搬运，不前端推 top1/可靠命中） |
| `front/app/api/normalizers/auxiliaryRepair.ts` | **最小扩展**：`directBoards`/`activeTagCount` 改 nullable（**范围缺失 = 未知**，不谎称 0 范围） |
| `front/app/features/tags/composables/useAuxiliaryDescriptionRepair.ts` | **最小扩展**：新增显式 `resendFrozen()`（重发原确认内容：原 key + 原 payload，草稿不丢）；`submitRepair`/`resendFrozen` 共用 `postRepair` 发送管线（互斥/epoch/生命周期不复制第二套）；改掉误导文案 |
| `front/app/api/normalizers/auxiliaryRepair.test.ts` / `boardHealthTriage.test.ts` / `useAuxiliaryDescriptionRepair.test.ts` | 随上述扩展补断言 |

**未改动**：任何既有宿主组件（`BoardWideWorkface` / `RailHealthBrief` / `BoardHealthPanel` / `BoardHealthDetail` / `BoardHealthSignal`）、
后端、共享 ApiClient、stores、全局 theme/CSS、其它 change 的脏文件。**未做任何 Git 写操作**。

## 2. 组件与数据流

```
BoardWideWorkface（既有 1040px 单级工作面，3.4 接入）
└── BoardHealthWorkspace.vue                 ← 根：持有两个 composable（405 行）
    ├── useBoardHealth(boardId)              ← 体检快照/趋势（既有读链，零新增请求）
    ├── useAuxiliaryDescriptionRepair()      ← C1 状态机（真实四资源 API）
    ├── BoardHealthWorkspaceHead.vue         ← 顶部一行 + 状态说明 + 提示行 + 折叠依据
    ├── BoardHealthIssueIndex.vue            ← 左：需核对项 + 「其余标签」折叠
    └── BoardHealthIssueDetail.vue           ← 右：当前项
        ├── BoardHealthRepairEditor.vue      ← 描述编辑（可编辑态）
        └── BoardHealthRepairProgress.vue    ← 只读执行态
```

> 单组件体量：全部新组件 **< 500 行 / < 15KB**（根 405 行 / 14.6KB；头部块与索引/详情/编辑器/进度各自独立）。

- **生命周期正确放置**：两个 composable 都在**根组件** `setup` 内创建（组件作用域 → 卸载自动 dispose）。
  编辑器/进度子组件只吃 props + emit，**收起编辑器不会 dispose 掉轮询**；根也**不按 selectedAuxId 给子组件加 key**
  （避免重挂丢未知冻结）。
- **组件不直调 API**：所有 HTTP 走 C1 composable → `api/auxiliaryLabels.ts` → ApiClient。
- **不在组件算病型/统计**：分诊与信号来自 `api/normalizers/boardHealthTriage.ts`；阶段与计数来自服务端任务；
  `boardHealthWorkspace.ts` 只做形状翻译（纯函数，有单测）。

### 2.1 状态矩阵（与 ui-design v2 对齐）

| 形态 | 触发 | 展示 | 动作 |
| --- | --- | --- | --- |
| 正常 | 快照有 triage 且有条目 | 顶部一行「本轮需核对 N 项」+ 索引 + 当前项 + **折叠的「本轮体检依据」** | 选择 → 编辑/核对 |
| 降级轮 | `llm_score` 与 `llm_comment` **均 NULL**（真 wire：`metrics.triage` 为 JSON null） | 「本轮未生成评分与分诊…不能据此推断当前标签健康」+ 原统计折叠 | 只读，不臆造问题 |
| 旧快照 | **有评分**但无 triage 字段 | 「历史快照」说明 + 原统计折叠 | 只读，不臆造问题 |
| 空清单 | `triage=[]`（有评分） | 「本轮没有分诊清单」+ 原统计折叠 | 同上 |
| 读取失败 | health error 且**无旧快照** | 「暂时无法读取体检」+ 重试 | 重试（不建任务） |
| stale | 刷新失败保留旧快照 | 一行「刷新失败，显示上次快照…」+ 重试 | 提交前仍核对实时版本 |
| 分诊有丢弃项 | `triage.dropped > 0` | 一行「N 条记录不可执行（已剔除），清单不完整」 | 不加信息卡 |
| 可编辑（rewrite） | 无任务、无未知态 | 原描述 + 三规则编辑器 + 共享范围 | 行内确认 → 最终确认才 POST |
| 只读执行态 | 有持久任务 | 三阶段 + 真实计数 + 折叠修改摘要 | 按状态给重试/恢复/下一项 |
| 未知结果 | 提交结果未知且无任务 | 只读保留原确认内容 | **重发原确认内容**（原 key+原 payload）/ 重新加载核对 |
| 冲突 | 409 | 服务端原文 + 保留草稿 | 加载最新描述再核对 |
| 摘除 | remove 类建议 | 说明 + 行内确认 | 发 `remove-composition`（3.4 注入真实 DELETE） |

### 2.2 确认前零写入 / 只写一次

- 打开处理区、编辑、改稿、展开证据、**取消确认** → 只有 GET（`description-repair-context` + 体检快照）；
- 行内确认 → **最终确认**才发一次 `POST description-repairs`（`idempotency_key` 由 C1 生成并冻结）；
- 确认视图是**行内**的（`data-testid="bhw-confirm"`），**不新叠弹窗**（不新增 AppDialog）。

### 2.3 三规则 / 共享范围 / 版本

- 三规则复用 C1 的 `validateRepairDescription`（trim 非空 / ≤500 码点 / 不得恰好重复标签名），**不额外黑名单**；
  字数统计用 `[...draft].length`（码点），与后端 `utf8.RuneCountInString` 同口径。
- `version` 只透传（前端不解析、不拼接、不本地构造）。
- 共享范围来自 `repair-context`：直接引用版块 + 覆盖的活跃话题标签数；
  **范围缺失 = 未知**（`directBoards === null` / `activeTagCount === null` → 显示「未知」，不谎称 0 范围）；
  合法 0 范围（服务端明确返回 `[]` / `0`）才显示「当前没有版块直接引用它」。

### 2.4 计数与完成措辞

- `processed`（真实成功）/ `failed` / `notApplicable`（跳过，**不是成功**）/ `remaining`（可能 null → 不展示）
  全部**服务端单源**，前端不重算、不用差值推 remaining、不以 P+F 判完成；
- `total/processed/failed` 为 null → 显示「未知」（C1 已把三者改 nullable，**不捏造 0**）；
- 完成判定只看服务端 `status === 'completed'`；文案严格「更新完成，效果待下一轮体检验证」，主动作变「处理下一项」
  （全部处理完时变「返回问题清单」，**不说所有标签健康**）。

### 2.5 证据（四形态共用）、失败不吞、范围未知禁确认

- **证据**：可编辑 / 未知 / **摘除**三种形态都能展开「为什么建议处理 · 展开证据」（remove 项也要看磁铁/最近邻/词根/失败挂载流向再确认）；
  已有任务时证据收进执行态的「折叠修改摘要」（不重复两处）。
- **失败不吞**：非 409 的提交失败走**独立错误行**（服务端 message 原样透传，不编文案），
  且**行内确认在失败时不收起**（用户看得见失败、可重试或取消）；409 走 conflict 形态（同样透传服务端 message）。
- **范围未知禁确认**：repair-context 未加载 / 读取失败 / 字段缺失（范围未知）时**禁用**「修复描述并更新匹配」与最终确认，
  并在原地说明原因（不谎称 0 范围）。

### 2.6 未知结果 = 保留原意图，不是新意图

- 未知态**只读**展示原确认内容，**不摆 textarea**；
- 入口「**重发原确认内容**」→ C1 的 `resendFrozen()`：严格用**冻结的幂等键 + 冻结的 payload**，
  用户当前草稿**不参与、也不被清空**；与新确认共用 `postRepair` 发送管线（互斥/epoch/生命周期守卫一致）；
- 结果确定后才解冻；**来源仍是服务端**（`context.latest_task` 同键同 payload 才算已受理），不自动新创建；
- 文案不向用户展示技术 key 值，也不把「刷新」说成「已撤销 / 原请求未受理」。

## 3. 旧能力 → 承接（HEAD 真实入口为证据）

| 旧能力 | HEAD 真实入口 | 本工作面的承接方式 |
| --- | --- | --- |
| 评分 / 评语 / 挂载·幽灵·向量缺失 / 评分趋势 | `console/BoardHealthSignal.vue`（纯 props，被 `BoardHealthPanel`、`BoardWideWorkface` 复用） | **直接复用**：折叠区「本轮体检依据」渲染同一组件（**有快照即渲染、默认折叠**，props 驱动，**零额外请求**、零重复统计实现） |
| 构成标签质量表（逐标签 样本/均值/中位数/低分占比） | `console/BoardHealthDetail.vue`（`mode="quality"`，宿主「构成」页签） | **仍由既有「构成」页签承载**；本工作面**不复制**统计实现，也不做空 slot/占位图 |
| 组合标签挂载/创建 | `console/BoardHealthDetail.vue`（`mode="composites"`） | 仍由既有「组合」页签承载 |
| 未收编候选全量 + 收编 | `console/BoardHealthDetail.vue`（`mode="candidates"`）+ `useBoardHealth.capture` | 仍由既有「候选」页签承载；本工作面折叠区不重复候选区（避免双收编入口/动作循环） |
| 摘除构成（真实 DELETE） | `useSemanticBoardsApi().removeFromComposition` + `BoardHealthPanel`/`BoardWideWorkface` 的 `remove` 事件 | 本工作面只定义 `remove-composition` 事件契约（3.4 注入既有链）；**未收到宿主真实成功反馈前不标「已摘除」** |
| 体检快照 / 趋势读取 | `useBoardHealth`（`GET /semantic-boards/:id/health-checkup`） | 本工作面**自持一份** `useBoardHealth`（不嵌套旧 Panel → 不产生额外请求） |

## 4. 3.4 接入契约（宿主 / L1 入口 / 真实 DELETE）

```vue
<!-- 宿主（BoardWideWorkface 的 health 页签，3.4 接线） -->
<BoardHealthWorkspace
  :board-id="boardId"
  :board-label="boardLabel"
  :removed-aux-ids="removedAuxIds"
  @remove-composition="onRemoveComposition"
/>
```

| 契约 | 形状 | 说明 |
| --- | --- | --- |
| prop `boardId` | `number` | 版块 ID（体检快照与共享范围用） |
| prop `boardLabel` | `string?` | 版块名（展示用，可省） |
| prop `removedAuxIds` | `string[]?` | **宿主回传**已真实摘除成功的 aux id（canonical 字符串数组）；未回传前 UI 不标「已摘除」 |
| emit `remove-composition` | `(auxiliaryLabelId: string) => void` | 宿主转数字后调用既有 `removeFromComposition(boardId, Number(auxId))`；成功后再回填 `removedAuxIds` |
| expose `reload()` | `() => void` | 同版块刷新体检快照（失败按 stale：保留上次快照 + 提示行，不创建任何任务） |

宿主侧骨架（3.4 实现，本批只定义契约）：

```ts
const removedAuxIds = ref<string[]>([])
async function onRemoveComposition(auxiliaryLabelId: string) {
  const res = await api.removeFromComposition(props.boardId, Number(auxiliaryLabelId))
  if (!res.success) { notify.error(res.error || '摘除失败，请重试'); return }   // 一次处理，不多层 toast
  removedAuxIds.value = [...removedAuxIds.value, auxiliaryLabelId]
  emit('refresh')   // 刷新构成
}
```

接线位置建议：`BoardWideWorkface` 的 `health` 页签改为渲染本工作面（它已含折叠的「本轮体检依据」，
与原 `RailHealthBrief variant="full"` 的能力重叠，避免同一页签两套依据）。
**本批没有接宿主** —— 未接宿主不等于整故事交付（真机完整故事仍待 tasks 5.3 的 agent-browser）。

## 5. 验证

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd front && pnpm test:unit app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.repair.test.ts app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts --maxWorkers=2` | **ok**：3 文件 / **51 例**全 PASS（19 + 14 + 18；C2 复审修复后 +11 例） |
| 2 | `cd front && pnpm test:unit app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2`（C1 composable 因本批扩展重跑一次） | **ok**：**40 例**全 PASS（原 38 + resendFrozen 2 例） |
| 3 | `cd front && pnpm test:unit app/api/normalizers/auxiliaryRepair.test.ts app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/BoardHealthPanel.test.ts app/api/normalizers/consoleRail.test.ts --maxWorkers=2`（C1 归一器扩展 + 旧件回归） | **ok**：4 文件 / 55 例全 PASS |
| 4 | `cd front && npx eslint <本批 12 文件>` | 0 问题 |
| 4b | 合并跑本批 + C1 归一器/composable + 旧件（10 文件） | **ok**：**160 例**全 PASS |
| 5 | `cd front && pnpm exec nuxi typecheck` | 本批文件**零错误**；全局仍有 20 处报错，全部落在**其它 change 并行中的脏文件**（`SignalReportView.test.ts`、`console/ObservationMatrixGrid.test.ts`、`console/OtherTopicReading.signals.test.ts`、`console/useConsoleReading.ts`+`.test.ts`、`console/useConsoleReturnContext.test.ts`），本批未触碰未修、**不宣称全局绿** |

> 未跑 `pnpm build` / 静态部署 / 浏览器 / 全量测试 / 后端门禁（按本批边界与任务约束）。
> 组件故事**没有**自证式 mock 掉整个 composable：工作面挂载**真实 C1 composable**，只 mock API 层
> （体检快照 + 修复四资源），因此"确认前 0 POST""只提交一次""轮询真实 server 状态"都是真链路断言。

## 6. 3.5 缺口（本批只做基础响应式，不冒充几何验收）

- 真实几何与截图验收（1440×900 / 1920×1080 / 375×667 / 390×844）**待 3.5**；
- 窄屏断点已用 `767.98px`（与 layout 契约一致）+ `useIsNarrowViewport()`：索引/详情互斥 + 「返回问题清单」+ 粘性动作条；
  **JSDOM 不构成尺寸验收**（本批只断言行为分支，不声称几何通过）；
- 双主题：本批只用 Layer 2 语义 token（无新增硬编码色值），视觉复核随 3.5 截图。

## 7. 未决 / 需上层裁定

1. **宿主接线位置**：`health` 页签是否整体换成本工作面（推荐）还是与 `RailHealthBrief full` 并列 —— 3.4 决定。
2. **逐标签质量表是否也要进工作面**：当前留在既有「构成」页签（不复制统计实现）。若要在工作面内提供，
   需提炼**纯展示**版本并同步改旧 Detail（会产生一次旧组件改动），请裁定是否值得。
3. **`magnet_flow` 的展示密度**：已搬运为 camelCase 视图并在证据折叠区展示一行；
   若要独立成块需再定版式。
4. **「已处理 N 项」的进展口径**：当前顶部只报**当前项**真实进展（不统计本地处理过的项，避免冒充持久进度）；
   若要显示整体已处理数，需定义持久来源（当前后端无"本轮已处理"字段）。
