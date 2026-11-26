# workshop-board-console 过期审计

> 审计对象：`openspec/specs/workshop-board-console/spec.md`（1120 行 / 33 Requirement）
> 代码基准：仓库 HEAD（spec 最后同步 `01ed9bcd` 2026-11-26；console 目录最后改动 `fd650949`/`6cafbf3e` 2026-11-26）
> 方法：逐条抽取可验证主张 → 到 `front/app/features/tags/components/console/`、`front/app/features/tags/composables/`、`front/app/api/`、`front/app/assets/css/main.css`、`backend-go/internal/**/handler` 核实；必要时 `git log --since=2026-11-02` 定位改动

## 总览

| 分类 | 条数 | 涉及 Requirement |
| --- | --- | --- |
| 有效 | 26 | 其余 |
| **过期** | **6** | R2、R13、R14、R16、R26、R30 |
| 无法核实 | 1 | R29（事件卡 popover 细节条款） |
| 代码有但 spec 没写（缺口） | 8 | G-1 … G-8（下方「代码有但 spec 没写」节） |

**一句话结论：这份 spec 现在还能当契约用——33 条里 26 条与代码逐条对得上，量化口径（三态 `—`/`0`/`?`、非综合稿线索数、行内峰值归一、14 格共同轴、阈值 2、31 日横滚、90 天归档窗、4 张卡片上限、64px 列宽、`--color-event` vs `--color-accent`）几乎全部仍然成立。但有 3 条必须先修**：R14 的「两个同级视图」已与代码的三个视图正面冲突、R16 的「24px 固定步长」在窄列会被钳制、R2 的图例口径与同文件 R11 自相矛盾。其余 3 条是**定位描述/术语漂移**（抽屉→宽工作面、治理→上下文、矩阵历史格→编年头、排序项多了「id 倒序」），行为契约本身仍成立。

真正的风险不是「条款说错了」，而是「**照 R14 写实现会漏掉归档视图**」和「**照 R2 补图例会违反 R11**」。

---

## 过期条款（按严重度）

### E-1 [严重] R14 关键事件视图切换与行集合 —「两个同级视图」
- **spec 说**（`openspec/specs/workshop-board-console/spec.md:468`）：「版块态势页主舞台 SHALL 提供两个同级视图：『矩阵 / 追踪』（现状）与『关键事件』。」
- **代码实际**：`front/app/features/tags/composables/laneMatrixModel.ts:11` → `export type ConsoleView = 'matrix' | 'archive' | 'events'`（**三个**视图）；`front/app/features/tags/components/console/BoardConsoleHeader.vue:97/107/116/124` 渲染 **4 个** seg 按钮：矩阵 / 追踪、关键事件、文章归档、研究工作台。
- **判定依据**：`9f86138c`（2026-11-23，归档 `add-key-event-matrix-view`）引入第三视图；`def2f382`（2026-11-26，`merge-lane-ops-specs`）把归档视图并入本 spec 后**没有回头修 R14 的「两个」**。同一文件 R26（:867）已写「归档 SHALL 提供『归档』视图，与『矩阵』同层切换」——spec 内部自相矛盾。
- **影响**：照 R14 枚举视图的实现/测试会认为 archive 不是主舞台视图，可能把它做成抽屉或子页；`setView('archive')` 在类型层就是第三态。

### E-2 [中等] R2 矩阵视图 — 图例口径「颜色 = 行内相对强度」
- **spec 说**（`:27`）：「矩阵 SHALL 在表头或 footnote 提供一条图例，说明数字为**线索数**、颜色为**行内相对强度**」；Scenario（`:41`）：「SHALL 显示『数字 = 当日线索数；颜色 = 行内相对强度』图例」。
- **代码实际**：`front/app/features/tags/components/console/ObservationMatrix.vue:132` → `<span data-testid="matrix-legend">当日线索数（不含综合稿） · — 无分区 · ? 数据不可用</span>`（**只有三项，没有颜色口径**）。
- **判定依据**：`f908cc53`（2026-11-20，`simplify-workshop-matrix-copy`）按 R11（`:310`「矩阵区 SHALL **仅**常驻一条图例，说明当日线索数（不含综合稿）、`—` 无分区、`?` 数据不可用」）收敛了文案，R2 没同步。色深本身仍在代码里（`observationLanes.ts:225` 行内峰值线性归一），只是**不再写进图例**。
- **影响**：若按 R2 补「颜色 = 行内相对强度」文字，会直接违反 R11 的「仅一条图例」；两个条款当前互斥，实现者无从取舍。

### E-3 [中等] R16 关键事件标签布局 —「24px 的固定步长」
- **spec 说**（`:511`）：「同一日期内的多条事件 SHALL 以 24px 的**固定**步长在该日期列中心横向散开」；Scenario（`:528`）：「节点之间的横向间距 SHALL 为 24px」。
- **代码实际**：`front/app/features/tags/components/console/keyEventLayout.ts:60` → `KEY_EVENT_NODE_FAN_STEP = 24`（常量仍是 24）；但 `front/app/features/tags/components/console/KeyEventTimelineView.vue:689-691` 实际取值为 `Math.min(KEY_EVENT_NODE_FAN_STEP, usable / Math.max(1, count - 1))`——**列宽不够时按列宽钳制，实际间距 < 24px**。
- **判定依据**：同文件注释（`:683-688`）「24px 固定步长在 64px 列宽下会让相邻两天的最近两个节点圆心只距 16px < 节点宽 18px」；改动来自 `1b727000`（2026-11-25，引线树改造）。
- **影响**：任何「间距 === 24px」的断言型测试在窄列必失败；正确表述是「≤24px 的自适应步长，列内钳制以保证不压盖」。

### E-4 [中等] R30 归档排序 —「发布时间/**id 倒序**」
- **spec 说**（`:1028`）：「组内 SHALL 默认按质量排序…，并 SHALL 支持切换为发布时间/**id 倒序**。」
- **代码实际**：`front/app/features/tags/components/console/BoardArchiveView.vue:686-688` → 只有两个 option：`quality`（强证据优先）、`time`（发布时间 ↓）；`:306` `setSortMode(... === 'time' ? 'time' : 'quality')`。**没有 id 倒序这一项**。
- **判定依据**：`BoardArchiveView.vue:9` 文件头注释也只写「组内排序（质量/时间）」；后端归档列表没有 id 序分支。
- **影响**：照 spec 写「id 倒序」选项会造出一个不存在的排序档；spec 多写了一个动作。

### E-5 [轻微] R13 治理右轨 / 治理抽屉 — 术语与入口名漂移
- **spec 说**（`:372`）：「≥1200px；768–1199 收进顶栏『**治理**』入口，抽屉头部仍显示体检分与趋势摘要」；`:394`「…SHALL 收进**治理抽屉**」；R7（`:191`）也写「治理抽屉设置页签」。
- **代码实际**：
  - 顶栏入口文案是「**上下文**」：`BoardConsoleHeader.vue:167-174`（`data-testid="console-gov-toggle"`，`aria-label="打开本板块上下文"`）；`:449-453` CSS 确认**只在 768–1199.98px 显示**（与 spec 的断点一致）。
  - 治理载体已不是抽屉：`front/app/features/tags/components/console/BoardWideWorkface.vue:1-2,29`「**单级宽工作面**（AppDialog size=xl，1040px）…单级、不套 32rem 抽屉套抽屉」；页签源 `railContext.ts:56-66` = 本板块上下文 / 体检依据 / 构成 / 组合 / 候选 / 来源 / 设置（比 spec 多「本板块上下文」「体检依据」两页签）。
  - `front/app/features/tags/components/console/SectionDrawer.vue`（唯一 AppDrawer 消费者）已退役，只剩自身测试引用。
- **影响**：术语过期。「抽屉」在当前 UI 词表里已无对应物，实施者可能重新造一个抽屉并违反「单级」约束。

### E-6 [轻微] R26 归档视图入口与日期筛选 — 入口位置与导航块方位
- **spec 说**（`:867`）：「由**矩阵历史格子**「看归档」带入的日期（`initialDate`）SHALL 优先于今天」；`:869`「归档的日期筛选 SHALL 由**工具行下方**的**时间导航块**承担」。
- **代码实际**：
  - 「看归档」入口在**编年头/其他话题阅读区**，不在矩阵格：`front/app/features/tags/components/console/LaneChronicleHeader.vue:76-79`「看本板块此日归档」、`front/app/features/tags/components/console/OtherTopicReading.vue:212`「看此日归档」。矩阵格本身是只读的，无该动作（`ObservationMatrixGrid.vue` 无 view-archive 出口）。`initialDate` 优先于今天的**行为契约成立**（`useConsoleShellActions.ts:58-64`、`BoardArchiveView.vue:83-84`）。
  - 导航块 DOM 在工具行**之上**：`BoardArchiveView.vue:607`（`ar-time-nav`）→ `:653`（`ar-list-head`）→ `:657`（`ar-toolbar`）。
- **影响**：找入口的人按 spec 去矩阵格里找「看归档」会找不到；重做归档布局会按 spec 把导航块放到工具行下方。

---

## 代码有但 spec 没写

### G-1 `?view=` URL 键（视图态进深链）
- **代码**：`front/app/features/tags/composables/useBoardConsole.ts:168-172`（`writeViewQuery`：非 matrix 才写 `query.view`）；解析 `laneMatrixModel.ts:201 parseViewParam`。
- **建议**：值得补。R8（`:209`）枚举 URL 键时只写「`days/start/end/lane`」，深链实现会漏 `view`。

### G-2 泳道名 / 版块名「看全」悬停聚焦卡
- **代码**：`front/app/features/tags/composables/useNameHoverCard.ts`；`ObservationMatrixLaneRow.vue`、`BoardRail.vue`（提交 `2058e695` 2026-11-26）。
- **建议**：可选补。属纯增量交互，不写不构成误导。

### G-3 行头**就地**改名（不是对话框）
- **代码**：`front/app/features/tags/components/console/ObservationMatrixLaneRow.vue:19-25`（菜单第一项「重命名」→ 行头变输入框，Esc/失焦取消、空名拒收）、`:77-133`。
- **建议**：值得补。R24（`:809`）只写「菜单 SHALL 提供『重命名』」，没写改名的交互形态（就地编辑 vs 弹窗）。

### G-4 归档时间导航块可折叠
- **代码**：`front/app/features/tags/components/console/BoardArchiveView.vue:629-630`（`default-collapsed` + `collapse-storage-key="syntopica.archive.time.collapsed"`）。
- **建议**：可选补。R26（`:869-872`）把导航块描述成恒定三段结构，未提可折叠。

### G-5 矩阵标签列宽按剩余空间分配
- **代码**：`front/app/features/tags/components/console/observationMatrixRows.css`（提交 `2058e695` 2026-11-26）。
- **建议**：可选补。纯布局细节。

### G-6 面包屑多一级「主页」
- **代码**：`front/app/features/tags/components/console/BoardConsoleHeader.vue:81-84`（`console-home` → `/`）。
- **建议**：可选补。R1（`:11`）写「面包屑（工坊 › 版块名）」是**两段**，实际是三段（主页 / 叙事工坊 / 版块名）。R1 的场景（「点『叙事工坊』→ 回 `/tags` 渲染雷达」）仍然成立。

### G-7 死代码 `BoardLaneEvolution.vue` 仍带「最密集的三天」
- **代码**：`front/app/features/tags/components/console/BoardLaneEvolution.vue:89-90`、`:215-217`（「最密集的三天」区块 + 「今天这条线发生的事」）。该组件**已无活路径**——全仓只有自身测试引用（`BoardLaneEvolution.test.ts`），线上走 `ObservationLaneEvolution.vue` + `LaneStancePanel.vue`。
- **建议**：值得补（作为清理项写进 spec 或删组件）。R4（`:97`）写「演进带 MUST NOT 再提供『最密集的三天』区块」——活路径确实没有，但仓库里还留着一份与之相悖的实现，容易被误复活。

### G-8 归档筛选 chip 的计数只在 title
- **代码**：`front/app/features/tags/components/console/BoardArchiveView.vue:659-670`（chip 数字进 `title`，deck 才出现数字）；`:589`（deck 的「弱证据」计数）。
- **建议**：可选补。R28（`:957`）只写「三个筛选」，未写计数呈现位置。

---

## 无法核实

| Requirement | 为什么无法核实 |
| --- | --- |
| **R29 事件卡 popover**（`:973-1023`） | ⑤「收编 M/N 标『相似 ≥ 阈值』、① 交集标『完全相同』」两口径并排、⑥「命中步就地展开代入式、历史计算缺失如实标注」等条款依赖**真实历史样本**与实时拉取结果；代码结构在位（`ArchiveMatchPopover.vue:551-584` 判定过程逐条 + `:574` 「这一步怎么算的 ▸」就地展开 + `:584/:738` 「未走到（第 N 步命中即停）」），但数值真实性只能实机核对。 |
| **R21 事件语义色与选中色分离**（`:728-747`） | 结构有效：`KeyEventTimelineView.vue:2310/2346-2349` 用 `--color-event`，选中日实心用 `--color-accent`；`front/app/assets/css/main.css:166`（亮）/`:254`（暗）两套主题都有该 token。但「正文级 ≥4.5:1 / 非文本级 ≥3:1」需实测对比度，代码里读不出来。 |
| **R3 聚合响应截断降级**（`:68-95`） | 「每桶前 N 条标题」中的 N 与「另有 K 条折叠未读取」的实际降级路径，需要在报告缺失/读取失败时实机触发才能确认（`observationLanes.ts` 只覆盖了 `—`/`?` 两态）。 |

---

## 建议

### 1. 该删 / 该改 / 该补

**必须改（3 条，会误导实现）**
1. **R14（`:468`）**：「两个同级视图」→ 改为「三个同级视图：矩阵 / 追踪、关键事件、文章归档（研究工作台为顶栏导航入口，非视图态）」，并把 `ConsoleView = 'matrix' | 'archive' | 'events'` 写进条款。
2. **R2（`:27` + `:41`）**：删掉「颜色 = 行内相对强度」的图例要求，改为与 R11 对齐的「仅一条图例：当日线索数（不含综合稿）/ `—` 无分区 / `?` 数据不可用」；色深作为**行内归一**写进 R10（`:267`）而不是图例。
3. **R16（`:511` + `:528`）**：「24px 的固定步长」→ 「≤24px 的步长，列宽不足时按列内可用宽度钳制以保证节点不压盖（`KEY_EVENT_NODE_FAN_STEP` 为上限）」。

**该改（3 条，措辞/定位漂移）**
4. **R30（`:1028`）**：删「/id 倒序」，改为「发布时间倒序（现有两档：质量 / 时间）」。
5. **R13（`:372`/`:394`）+ R7（`:191`）**：把「治理抽屉」统一改为「**单级宽工作面**（1040px 宽对话框，单级、不叠抽屉）」；把顶栏入口名从「治理」改为「**上下文**」；补上多出的两个页签（本板块上下文 / 体检依据）。
6. **R26（`:867`/`:869`）**：把「矩阵历史格子『看归档』」改为「**当日工作面（编年头）『看本板块此日归档』**」；把「工具行下方」改为「**工具行上方**」。

**该补（3 条）**
7. **R8（`:209`）**：URL 键补 `view`。
8. **R24（`:809`）**：明确重命名为**行头就地编辑**（Esc/失焦取消、空名拒收）。
9. **R4（`:97`）**：加一句「历史组件 `BoardLaneEvolution.vue` 已退役，最密集三天区块仅存于死代码」，或直接删掉该组件（连带 `BoardLaneEvolution.test.ts`）。

### 2. 这份 spec 是否还值得拆分（1120 行）

**先修过期，再拆。** 判断依据：

- **过期集中在「窄条」而非「宽条」**：33 条里 26 条有效且**逐条有代码锚点**（路由、组件、端点、量化值都对得上）。真正过期的 6 条集中在 4 个窄面上——视图层枚举（R14）、图例文案（R2 vs R11）、节点步长（R16）、排序档位（R30）、术语命名（R13）、入口定位（R26）。这些是**局部口径问题，不是结构性腐化**。
- **1120 行的真实压力不来自过期，来自能力边界混杂**：本 spec 同时承担了 (a) 版块态势页骨架/矩阵/日期轴/观察窗（约 R1–R12）、(b) 治理右轨与上下文（R13）、(c) **关键事件视图整整 10 条**（R14–R23，约 250 行）、(d) 矩阵覆盖组与「其他话题」整行（R24–R25）、(e) **归档视图整整 8 条**（R26–R33，约 260 行）。(c) 与 (e) 都是**独立视图**、独立读取链（`useBoardKeyEvents.ts` / `useBoardArchive.ts`）、独立组件树，各自已经够格是独立 spec。
- **建议顺序**：
  1. 先按上面 1–9 修完 6 条过期 + 3 条缺口（预计改动 < 60 行，不动结构）；
  2. 再把 (c) 关键事件视图 10 条、（e）归档视图 8 条各拆成独立 spec（如 `workshop-key-events-view` / `workshop-archive-view`），本 spec 保留骨架 + 矩阵 + 右轨（15 条左右，约 600 行）；
  3. 拆完把拆分记进 `docs/reference/flow/semantic-board.md` 的域路由，避免违反「域清单唯一真相源 = `docs/reference/flow/*.md` basename」。
- **不建议现在做的**：不要顺手重排 R 编号、不要合并 R2/R11（两条都有效，只是图例那一段互斥）。过度整理会制造一次大 diff，反而更难核对。

### 3. 一句话风险提示

这份 spec 的过期**不会让实现写错**（关键口径都还对），但**会让实现少写一块**——R14 的「两个视图」最可能让新同事把归档视图降级成子页。优先修 R14、R2、R16 三条即可恢复契约可信度。
