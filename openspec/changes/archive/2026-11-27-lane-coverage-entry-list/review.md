# review — lane-coverage-entry-list（2026-11-27）
> 范围：`git diff -- front/app/features/tags/components/console/`（Grid.vue / LaneRow.vue + 4 个测试文件）对照 delta spec「窗口内未进日报的活跃泳道可见」5 个 Scenario、design D1–D6、ui-design 受影响状态表与 Interaction Decisions；只读走查，未跑 lint / typecheck / 测试（主线程已持有：lint 0 error、typecheck 0、5 文件 106 条全绿）。

### R-1 2026-11-27【High】列表项 ⋯ →「归档泳道」/「编辑泳道」打开的弹层泳道名为空（主故事 a 在确认步断链）
- **状态**：已修复（`BoardConsoleShell.vue:laneLabelOf` 在 A 档名单查不到时回退板块话题清单 `boardTopics`；`BoardConsoleShell.integration.test.ts` 新增「列表项 ⋯「归档泳道」→ 弹层文案带泳道名 → 二次确认 → 走既有归档接口 → 项消失且展开态保持」，反向验证：退回旧实现该用例即红）
- **位置**：`BoardConsoleShell.vue:laneLabelOf`（原 `:232-233` 只查 `allLanes`）→ `:240` `archiveLaneLabel` / `:237` `correctionLaneLabel` → `front/app/components/dialog/LaneArchiveDialog.vue:50,85`；`LaneCorrectionCanvas.vue:1278`（band 标题）。入口：`ObservationMatrixGrid.vue:371-372`。
- **问题**：覆盖泳道按定义不在 A 档 `allLanes`（`coverageLanes.ts:45` 正是用 `allLanes` 的 id 集合把它们排除掉），`laneLabelOf` 回 `''`，于是归档二次确认渲染成「确定归档泳道「」？」、提示「归档「」后，该泳道不再吸附新内容」；「编辑泳道」进修正画布的带标题同样为空。归档动作本身能完成（`updateTopic(id,{status:'archived'})` → `handleLaneArchived` → 重取 → 项消失、`coverageOpen` 保持），但用户在危险操作前看不到要归档的是谁。根因在上一 change 的壳层（当时 ⋯ 测试是假阴性，从未真走到菜单），本 change 的 spec Scenario「覆盖行可维护」与 ui-design「编辑 / 归档（列表项 ⋯）与矩阵行头一致」重新承诺了这条路径，tasks §5 把「覆盖行可维护」登记为集成测试已覆盖，而集成测试只走了重命名。
- **建议**：`laneLabelOf` 回退到话题清单（`boardTopics.value.find(t => String(t.id) === topicId)?.label ?? ''`）；集成测试补「列表项 ⋯ → 归档 → `LaneArchiveDialog` 文案含泳道名 → 确认 → `coverage-list` 项消失且 `coverage-toggle` 仍 `aria-expanded="true"`」。

### R-2 2026-11-27【Medium】ui-design Acceptance 承诺的 agent-browser 走查 / `ui-verification/report.md` 未建账，tasks 也无对应任务
- **状态**：已采纳（已补走查建账：`ui-verification/report.md` A-1~A-5 + 四张截图；tasks §5 补了对应验证行）
- **位置**：`openspec/changes/lane-coverage-entry-list/ui-design.md` Acceptance「视觉：…在 `ui-verification/report.md` 建账」；change 目录下无 `ui-verification/`；`tasks.md:27-40` §5 只列测试命令与 grep。
- **问题**：本次起因是「看着突兀」，ui-design 自己把「折叠态 1440×900 / 1920×1080 各一张、展开态一张、⋯ 菜单一张、列表不产生页面级横向溢出」写成验收项。代码走查能证明列表在 `.omx-scroll` 外（`.omx-matrix` 无任何样式规则，`<ul>` 是块级兄弟节点，宽 = 舞台可见宽），但「矩阵底部只剩一条、展开不挤、不溢出」只有视觉能证。制品承诺与仓库现状不一致，且没有登记为待办，归档门禁对账时会悬空。
- **建议**：要么补走查建账（四张 + 溢出结论），要么 ui-design Acceptance 明示改为归档前做并在 tasks §5 加一行，二选一别留悬空项。

### R-3 2026-11-27【Medium】`.omx-lane-coverage-seen` 由 LaneRow 渲染、样式却挂在 Grid 的 `:deep`（ui-design 只列了三条本地样式）
- **状态**：已采纳（规则原样挪到 `observationMatrixRows.css`，与 `.omx-lane-dormant-tag` 并列；Grid 里删掉 `:deep` 版本，ui-design「复用组件与布局模式」同步改口）
- **位置**：`ObservationMatrixLaneRow.vue:257`（新可见标签）；`ObservationMatrixGrid.vue:548-556`（`.omx-coverage-item :deep(.omx-lane-coverage-seen)`）；`ui-design.md`「复用组件与布局模式」：列表只补三条本地样式（`ul` reset / 点线分隔 / `.omx-lane` 修正）。
- **问题**：同形态的 `.omx-lane-dormant-tag` 样式随元素放在 `observationMatrixRows.css:68-77`，而新标签的边框 / 字号 / 内边距只有在 `.omx-coverage-item` 包裹下才生效——LaneRow 头注明示 `coverage` 分支「同时服务列表形态」，一旦别处复用就是裸文本。实现与 ui-design 的「三条」计数也对不上（实际四条：L531 / L537 / L542 / L548）。视觉本身没问题（参数与 dormant-tag 一致），属合理但依赖包裹上下文。
- **建议**：把这条规则挪进 `observationMatrixRows.css` 与 `.omx-lane-dormant-tag` 并列（元素在哪、样式在哪），或 ui-design 改口为四条并注明「标签样式挂在列表容器上」。

## 附录

### A. 旧能力去向表（旧入口以 `git show HEAD:…` 行号为准）

| # | 旧能力 | 旧入口（HEAD） | 新承接 | 判定 | 证据 |
| --- | --- | --- | --- | --- | --- |
| ① | 名字常显（不以缺席表达沉默） | `ObservationMatrixGrid.vue:335-343`（常显 summary，默认展开；`coverageSummaryText` L160-162 = 名字 · 最近一次） | Grid `:338-356` 折叠条 `coverage-group-summary` > `button[data-testid=coverage-toggle]`；`coverage-group-lanes` 只列名字（L164 `join('、')`） | 已迁移 | Grid.test L382-404「默认折叠：只有折叠条（数量 + 两个名字）」；integration「折叠条（名字常显）」 |
| ② | ⋯ 三项菜单（重命名 / 编辑泳道 / 归档泳道） | Grid `:345-361` 网格内 LaneRow `coverage` 分支 + `rename-lane`；LaneRow `laneMenuItems` | Grid `:360-374` 列表项同一 LaneRow（`cells=[]`）+ `rename-lane`，`@edit-lane/@archive-lane` 透传 | 已迁移（壳层侧弹层泳道名为空 → R-1） | LaneRow.test「⋯ 仍提供重命名 / 编辑 / 归档」（真查 `document.body`，修掉旧假阴性）；Grid.test L437-455 |
| ③ | 就地改名（Enter / Esc / 失败保留输入） | LaneRow `commitRename` / `cancelRename`（未改）+ 旧 `rename-lane` 下传 | 同一状态机（`:104-129`）；Grid `:370` `:rename-lane="renameLane"` | 已迁移 | integration「列表项 ⋯「重命名」→ 行内输入框 → Enter 走既有 PATCH 并重取清单（展开态保持）」 |
| ④ | 名字静态不可选中、不 emit `selectLane` | LaneRow `v-else` 静态 span；旧 Grid 不接 `@select-lane` | 同（`:249-258`）；Grid 列表项也不接 `@select-lane` | 已迁移 | LaneRow.test「名字是静态标签（点不动）」；Grid.test L425-428 |
| ⑤ | 清单失败一行提示 + 重试 | Grid `:329-332`（`.omx-grid` 内 noterow） | Grid `:332-335`（`.omx-scroll` 外）；样式 `.omx-group-noterow` 直选 + `.omx-grid :deep` 双写 L509-510，`DayOtherTopicsGroup.vue:134` 的网格内消费方不受影响 | 已迁移 | Grid.test L456-466；Matrix.test「清单失败」；integration「覆盖清单失败…重试」 |
| ⑥ | 空 / 读取中不渲染 | Grid `:175-177` `showCoverageGroup` | 同名 computed `:180-182`，语义未改 | 已迁移 | Grid.test L467-472 |
| ⑦ | 不参与 A 档行序 / 休眠分组 / 右轨名单 | `coverageEntries` 独立 computed（旧 `:149-157`），不进 `activeEntries/dormantEntries` | 新 `:151-162` 仍独立、只给模板 v-for；`coverageLanes` 仍由壳层 `buildCoverageLanes(allLanes, boardTopics)` 派生（Shell `:295`） | 已迁移（未被破坏） | grep：`coverageEntries` 仅 L151 定义 + L359 消费；`activeEntries/dormantEntries` L142-143 不引用 coverage；假行 `dormant:false` 下 LaneRow `rowTitle/nameCardMeta` 的 coverage 分支不读 `clueTotal/sectionTotal/snapshot` |
| ⑧ | 多版块同 ID 不重复 | `coverageLanes.ts:45`（`present = new Set(rows.map(topicId))`） | 未动 | 已迁移（未动） | `git status` 该文件无改动；`coverageLanes.test.ts` 6 用例回归 |
| ⑨ | 主动退役：矩阵网格行与逐日「—」格 | Grid 旧 `:149-157` `project(..., true)` 投影 gap 格；`:345-361` 网格内 LaneRow | 删除（design D5）；`coverageEntries` 只剩 `{ row }` | 主动退役 | 新 Grid `project(` 仅 L128 定义、L142-143 活跃/休眠调用；无 `gap` 格投影；Grid.test L401-404 断言 `matrix-grid` 内无 summary、无 `matrix-lane-20`、无 `matrix-cell-20-*` |

去向表无「遗漏」。

### B. 主故事走查（读代码）

- **(a) 新建泳道窗口外 → 折叠条 → 点开 → ⋯ → 归档 → 项消失、展开态保持**：`handleComposeSaved` → `loadBoardTopics` → `coverageLanes` 含该泳道 → 折叠条列名 → 点 `coverage-toggle` → `coverageOpen=true` → `<ul>` 项 → `lane-menu-trigger-<id>`（RowActionMenu Teleport body）→「归档泳道」→ Grid `emit('archiveLane')` → Matrix → Shell `overlays.openLaneArchive(id)` → `LaneArchiveDialog` **泳道名空**（R-1）→ 确认 → `updateTopic(id,{status:'archived'})` → `handleLaneArchived`（Shell `:248-251`）→ `load()` + `loadBoardTopics()` → `buildCoverageLanes` 过滤 `status!=='active'` → 项消失；`coverageOpen` 是 Grid 本地 ref、Grid 本身不卸载 → 展开态保持（清空则整组隐藏，再出现时仍展开）。**结论：确认步断链（弹层无泳道名），其余通。**
- **(b) ⋯ → 重命名 → Enter → `renameLane` → 新名**：`startRename` → input → Enter → `commitRename` → Shell `renameLane`（`:273-292`）→ `updateTopic` → `load()` + `loadBoardTopics()` → 返回 true → `editing=false`。重取期间 `coverageLoading=true` → `showCoverageGroup=false` → `<ul>` 卸载再挂载（Grid 不卸载，`coverageOpen` 保持）→ 新 `label` 直出 `coverageSummaryText` 与列表项。**结论：通**（集成测试覆盖到 PATCH + 重取 + `aria-expanded` 保持；未断言新名文本，但名字是 computed 直出）。
- **(c) 清单失败 → 一行提示 → 重试 → 恢复折叠条**：`useConsoleShellData.ts:207-220` 失败置 `topicsError`、清 `boardTopics` → `coverage-error-row`；点 `coverage-retry` → `emit('retryCoverage')` → Shell `:536` `@retry-coverage="loadBoardTopics"` → 先 `loading=true / error=null`（提示行消失、不占位）→ 成功 → `showCoverageGroup` → 折叠条。**结论：通。**
- **(d) >31 天横滚模式下列表在 `.omx-scroll` 外**：`scrollable = axis.length > 31` → `.omx-grid.is-scrollable { min-width: max-content }` 只在 `.omx-scroll{overflow-x:auto}` 内滚；`<ul class="omx-coverage-items">`（Grid `:358`）是 `.omx-scroll` 之后的兄弟节点，`.omx-matrix` 无任何样式规则（grep 无定义）→ 宽 = 舞台可见宽、不随横滚漂移；⋯ 菜单与名称卡均 Teleport body。**结论：通（代码走查；视觉未实测 → R-2）。**
- **(e) `coverageOpen=true` 时清单变空再出现**：`showCoverageGroup=false` → 条与列表都不渲染（不显示「· 0」）；再出现 → 直接展开（`coverageOpen` 留 true）。design 伪代码明示接受；折叠态无 `aria-controls`，不存在悬空引用。**结论：合理。**

### C. 重点风险核对结果

- `:dormant="false"` + `coverage`：`.omx-lane` 不带 `is-dormant`，不吃 `observationMatrixRows.css:210` 的 0.55；spec 把这些泳道定义为 active 且 MUST NOT 计入休眠组，与 ui-design 一致。旧实现（HEAD Grid `:155/:354` `dormant:true`）是语义错误，本次顺手改对。
- HTML / a11y：`ul > li > div.omx-lane` + Teleport 占位注释，合法；`aria-expanded` 只在 `button[data-testid=coverage-toggle]` 上，无 `aria-controls` 悬空。静态名 `span[aria-label]`（generic role）为上一 change 既有，本次未改。
- `:deep` 作用域：`.omx-coverage-item :deep(.omx-lane)` 编译为 `.omx-coverage-item[data-v-grid] .omx-lane`（特异度 0,3,0），高于 rows.css 的 `.omx-lane[data-v-lanerow]`（0,2,0），`position: static / cursor: default` 生效且只命中列表内行头；活跃 / 休眠行头在 `.omx-grid` 下不被匹配。`z-index:2 / left:0` 残留对 static 非 flex-item 无效。
- `.omx-group-noterow`：网格内唯一别的消费方 `DayOtherTopicsGroup.vue:134` 仍由 `.omx-grid :deep(.omx-group-noterow)` 覆盖；`grid-column: 1 / -1` 出网格后惰性。
- `coverageEntries` 假行（sectionTotal 0 / snapshot null）：仅 L359 模板 v-for 消费；LaneRow `coverage` 分支（`:146-149 / :164`）不读 `clueTotal/sectionTotal/watchLinked`，`rowMax`/排序/休眠判定都在 `project()` 与壳层，不经过它。
- `Icon` chevron：`coverageOpen ? 'mdi:chevron-down' : 'mdi:chevron-right'` width 14，与休眠条逐字一致。
- 测试：4 文件 `toMatchSnapshot` 0 处；断言全是 testid / 文案 / emitted 行为；testid kebab-case 语义名合既有约定（`coverage-toggle` 直接落在 button 上，比 `dormant-toggle` 落在 wrapper 上更好点）。
- 不变量 grep：`grep -c "最近一次" ObservationMatrixLaneRow.vue` → 0；Grid 无 `project(` / `gap` 格用于 coverage；旧 `lastSeenText` 已删干净。
- design D1–D6 落点全部可解析：D1 Grid 模板 `.omx-matrix` 内 `.omx-scroll` 之后（`:330-375`）；D2 LaneRow `coverage` prop（`:55`）+ `cells=[]` 零格；D3 Grid `coverageOpen`（`:167`）；D4 LaneRow `rowTitle`（`:146`）+ Grid `coverageSummaryText`（`:164`）；D5 Grid `coverageEntries`（`:151`）只 `{ row }`；D6 flow 文档 `semantic-board.md:287` 已是折叠条口径。

### D. 观察（不计 R 条目）

- `:selected-lane-id` 仍下传：`useConsoleShellOverlays.ts:76` `handleComposeSaved` 会 `selectLane(新泳道)`，窗口外新建的泳道在列表展开时 `.omx-lane.is-selected`（accent 条 + 加粗）。非点击产生，不违反「点名字 MUST NOT 产生选中态」；右轨 `selectedLaneRow` 查 `allLanes` 为空是壳层既有行为。
- 改名 / 归档后重取期间整组短暂消失再出现：spec「读取中不渲染」的直接后果，与旧行为一致。
- design D1 取舍行「测试文件不动」与 Risks「测试改写面…4 个测试文件」/ tasks 3.x 自相矛盾（措辞）。
- tasks 4.1：flow 文档正文已改但未勾；4.2 溯源行未加（`grep lane-coverage-entry-list docs/reference/flow/semantic-board.md` 只命中 L287）。
- `BoardConsoleShell.integration.test.ts:516` describe 注释仍写「组（名字 + 最近一次）」。
- `ObservationMatrixGrid.vue` 580 行 / 24KB（code-style §8 >500 行应拆分；本次 +42 行，design D1 有意不拆）。
