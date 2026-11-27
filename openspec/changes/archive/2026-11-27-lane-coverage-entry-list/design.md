## Context

版块态势矩阵（`ObservationMatrixGrid`）底部有一个「窗口内未进日报」覆盖组（lane-matrix-coverage-and-others-row §4.1）：summary 一行 + 默认展开的 `ObservationMatrixLaneRow`（`dormant` + `coverage`，逐日格全为「—」）。模板结构是 `.omx-matrix` > `.omx-scroll`（`overflow-x:auto; overflow-y:hidden`，横滚容器）> `.omx-grid`（CSS grid，组 summary 走 `grid-column: 1 / -1`）。

用户反馈（2026-11-27）：整行「—」在矩阵里太突兀。具体是三件事叠加：① 用一整行网格表达「这行没数据」；② 名字 + 日期在 summary、行头、行尾标签重复三遍；③ 项内「最近一次 MM-DD」是命中日期，与组名「未进日报」对着读自相矛盾（当天命中却「未进日报」）。

要保留的能力（原 change 的 Why）：活跃泳道不能因为窗口内没分区就整条消失——名字要常显、⋯ 的重命名 / 编辑 / 归档要可达、读取失败要诚实降级、空组不渲染、不参与 A 档排序 / 休眠 / 右轨。

## Goals / Non-Goals

**Goals**

- 覆盖组挪出矩阵网格：折叠条入口（常显名字、默认折叠）+ 点击展开的无格列表。
- 同一信息只出现一次：折叠条只列名字，日期只在列表项。
- 项内日期文案改准：「最近命中 MM-DD」。
- 数据源、过滤规则、⋯ 能力、失败降级、空组隐藏全部不变；零后端改动。

**Non-Goals**

- 不改 `lane-dynamics` 的行语义，不改 `coverageLanes.ts` 的过滤 / 排序。
- 不给覆盖组加排序 / 搜索 / 分页（真实数据 1~5 条）。
- 不动休眠组、其他话题行、当日其他话题组的任何表现。
- 不做展开态持久化（与休眠态一致：切版块 / 刷新回默认）。

## Decisions

### D1. 折叠条 + 列表渲染在 `.omx-scroll` 之外、`.omx-matrix` 之内（留在 Grid 组件）

- **结论**：错误行 / 折叠条 / 列表从 `.omx-grid` 挪到 `ObservationMatrixGrid` 模板里 `.omx-scroll` 闭合标签之后，作为 `.omx-matrix` 的最后一个子节点。
- **落点**：front/app/features/tags/components/console/ObservationMatrixGrid.vue（模板 `.omx-matrix` 内、`.omx-scroll` 之后；样式节的 `.omx-group-*` 族原地复用）
- **取舍**：备选 ① 留在 `.omx-grid` 内（`grid-column: 1 / -1`）——仍随横滚漂移、仍受 `overflow-y:hidden` 约束、视觉上仍是「网格的一部分」，正是用户嫌突兀的根源；② 挪到 `ObservationMatrix.vue` 的 Grid 之后——要把 Grid scoped 的 `omx-group-*` 样式抽成共享 css，Grid 的 coverage props / emits 还要上提一层，改动面翻倍。选留在 Grid 组件但出滚动容器：零样式抽取、props / emits 不变、测试文件不换（只重写其中的覆盖组用例），列表宽度 = 舞台可见宽。Grid 因此到 580 行（code-style §8 的 500 行线），拆分留待后续单独 change（本次不扩大面）。

### D2. 列表项复用 `ObservationMatrixLaneRow` 的 `coverage` 分支（`cells=[]`）

- **结论**：列表项 = `<li>` + `<ObservationMatrixLaneRow :cells="[]" :axis="[]" coverage …>`；不新建列表项组件，不抽改名 composable。`axis` 只在格的 v-for 里用到，`cells=[]` 时多根片段只剩行头 + 名称看全卡 Teleport。
- **落点**：front/app/features/tags/components/console/ObservationMatrixLaneRow.vue:coverage
- **取舍**：备选 ① 新建 `CoverageLaneItem.vue` + 从 LaneRow 抽 `useLaneRename()`——更「正统」，但要动 LaneRow 的改名状态机（editing / draft / saving / commitRename）加两份新文件，收益只是名字更好听；② 复制 ~50 行改名状态机——重复代码。按形态复用 LaneRow：⋯ 菜单、就地改名、悬停看全名全部白拿。`.omx-lane` 的 `sticky / left / z-index` 出了滚动容器是惰性样式，只需在列表里修正 `position: static` 与 `cursor: default`。

### D3. 展开态是 Grid 本地 `coverageOpen`（默认 false），不沿 `toggleDormant` 上提

- **结论**：`const coverageOpen = ref(false)` 放 Grid；清单刷新（改名 / 归档后重取）不重置展开态。
- **落点**：front/app/features/tags/components/console/ObservationMatrixGrid.vue:coverageOpen
- **取舍**：`expandedDormant` 上提到 `useBoardConsole` 是因为它与舞台级动作联动；覆盖列表的展开只有这一处消费，上提要走 Grid → Matrix → Stage → Body → Shell 四跳，纯增接线。刷新不重置：用户展开着看，归档一条后剩下的还在原地，不会突然收起。

### D4. 文案：折叠条只列名字；项内日期写「最近命中 MM-DD」

- **结论**：`coverageSummaryText` 只用「、」连接名字；LaneRow 的 tag / `rowTitle` / `nameCardMeta` 三处「最近一次」改「最近命中」。
- **落点**：front/app/features/tags/components/console/ObservationMatrixLaneRow.vue:rowTitle
- **取舍**：`last_seen_date` 是算法侧命中计数的日期（repository 注释「hit counters belong to the algorithm」），不是进日报的日期。原 summary 带「最近一次 MM-DD」、行头一遍、行尾标签又一遍，三处重复；现在折叠条只列名字，日期只在列表项出现一次。

### D5. `coverageEntries` 去掉 `cells` 投影

- **结论**：`coverageEntries` 只保留 `{ row }`；删掉为每条覆盖泳道投影 14 个 `gap` 格的逻辑。
- **落点**：front/app/features/tags/components/console/ObservationMatrixGrid.vue:coverageEntries
- **取舍**：列表无格，留着只会让后来者以为还有网格行，也白算一遍 `project()`。

### D6. flow 文档句随实现同步

- **结论**：`docs/reference/flow/semantic-board.md`「业务约束与不变量」里该组的描述改为折叠条 + 列表口径，并追加变更溯源。
- **落点**：无代码落点：tasks.md 4.1 / 4.2 文档任务，因为这是文档同步决策，不改代码。
- **取舍**：不改 spec 的 Requirement 标题（「窗口内未进日报的活跃泳道可见」），只改正文与两个 Scenario——能力没变，变的是载体。

## 核心逻辑（伪代码）

```
// Grid 本地状态
coverageOpen = ref(false)                       // 默认折叠；清单刷新不重置
coverageEntries = coverageLanes.map(l => ({ row: toLaneRow(l) }))   // 不再投影 cells

// 模板（.omx-matrix 内，.omx-scroll 之后）
if (coverageError)
    <div coverage-error-row>「窗口外泳道清单未取到」 <button @click=emit('retryCoverage')>重试</button></div>
else if (!coverageLoading && coverageEntries.length > 0)            // 读取中 / 空 → 整组不渲染
    <button omx-group-btn :aria-expanded=coverageOpen @click=coverageOpen = !coverageOpen>
        chevron(coverageOpen)  「窗口内未进日报 · N」  names.join('、')（截断，title 全文）  「展开查看 / 收起」
    </button>
    if (coverageOpen)
        <ul coverage-list>
            for e in coverageEntries:
                <li> <LaneRow row=e.row cells=[] axis=[] coverage coverageLastSeen renameLane @editLane @archiveLane /> </li>
        </ul>
// 其余（活跃行 / 其他话题行 / 当日其他话题组 / 休眠组）不动

// 边界
// - 读取中：不占位、不冒「无覆盖」
// - 清单为空：条与列表都不渲染（不显示「· 0」）
// - 失败：只出一行提示 + 重试；MUST NOT 渲染成「没有窗口外泳道」
// - 展开中改名 / 归档 → 壳层重取 A 档与清单 → coverageEntries 更新、coverageOpen 保持
//   （归档后该泳道 status≠active → 从清单消失；若清空 → 整组隐藏，coverageOpen 留 true 无副作用）
// - LaneRow cells=[]：不渲染任何 .omx-cell；selectCell 无从触发；名字静态 → 不 emit selectLane
```

## Risks / Trade-offs

- **按形态复用 LaneRow**（`cells=[]`）而不是按名字：在 LaneRow 头注写明「`coverage` 分支同时服务矩阵底部列表（无格形态）」，避免后人以为可以删。
- **列表展开会把下方「当前泳道」面板往下推**：与休眠展开同款行为；条数多时可考虑分页，本期不做（真实数据 1~5 条）。
- **展开态不持久**：切版块 / 刷新回折叠态，与休眠态一致，用户已习惯。
- **测试改写面**：覆盖组相关断言分散在 4 个测试文件（见 tasks 3.x），旧断言删干净、新断言覆盖四个 Scenario。
