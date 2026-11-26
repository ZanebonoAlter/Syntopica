# 审查报告：fix-stale-canvas-specs

- **审查者**：MiniMax M3.1 Flash（只读）
- **结论**：需改后进
- **放行判断**：9 条修正里 6 条（数值/枚举/文案类）与代码逐字对得上，但 4 条 High 全部是「承诺已兑现、实际只改了一半」或「新写了一条假 MUST」——与本 change 要消灭的缺陷同类，归档前必须补齐。

## 发现

### R-1 2026-11-26【High】三视图条款引入假 MUST：归档视图并不共用矩阵的观察轴与 `selectedDate`
- **状态**：已修复
- **处置**：三视图改为只替换「两个同级视图」这一子句，`selectedLane` / 持久化话题行序 / 「切换视图 MUST NOT 重置其中任何一项」/ 7-14-30-365 档全部保留；共用约束限定为「矩阵与关键事件」，归档视图明确写为例外（有自己的 90 天视窗，指向「归档视图入口与日期筛选」），只共用 `selectedLane`
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:5`（Requirement「关键事件视图切换与行集合」第 1 段）
- **问题**：新句「三个视图 MUST 共用同一条观察轴与同一个 `selectedDate`」对**归档视图不成立**，且与同一 delta 里 R26 的条款直接矛盾。归档视图有自己独立的 90 天视窗与起止选区（`initialDate` 只是一次性入口，再次切视图不保留旧视窗），矩阵的时间视窗（7/14/30/365 档）是另一份状态。另外这次整段重写把原文里仍然有效的契约一并删掉了：`selectedLane`、同一份持久化话题行序、「切换视图 MUST NOT 重置其中任何一项」、「时间范围 SHALL 由用户在同一个时间视窗控件里选择（7/14/30/365 档或自由区间）」、「关键事件视图 MUST NOT 维护第二套跨度状态」。视图从 2 个变 3 个之后，「切换不重置」这类约束只会更重要，不该在修数字时被抹掉。
- **证据**：
  - `front/app/features/tags/components/console/BoardArchiveView.vue:83-84`（`baseDate = initialDate || todayInShanghai()`）、`:89`（`viewportDays = ref(90)`）——归档视窗与矩阵视窗是两份独立状态；矩阵侧在 `ObservationRangeControls.vue`。
  - 同一 delta `specs/workshop-board-console/spec.md:238`「`initialDate` 仅作一次性入口，再次切换视图不保留旧视窗」、`:240`「时间导航块…90 天视窗主图…全历史导航条」——与新句「共用同一条观察轴」互斥。
  - `front/app/features/tags/composables/useBoardConsole.ts:327-331`（`setView` 只改 `activeView`，不重置选中态）——被删掉的「MUST NOT 重置」在代码里成立，属于**有效条款被误删**。
  - 原条款 `openspec/specs/workshop-board-console/spec.md:468`（两视图版本，含上述 5 项约束）。
- **建议**：把共用约束限定为「矩阵与关键事件」，归档视图改为指向 R26 的独立日期契约；并把 `selectedLane` / 持久化话题行序 / 「切换视图 MUST NOT 重置其中任何一项」/ 7-14-30-365 档与「不维护第二套跨度状态」补回（本 requirement 已保留的 Scenario「时间视窗两侧共用」只覆盖了关键事件，不覆盖归档）。

### R-2 2026-11-26【High】「治理抽屉 → 单级宽工作面」只改了 1/4 处，条款内部自相矛盾
- **状态**：已修复
- **处置**：4 处全改：正文两处「收进治理抽屉」→「单级宽工作面（`BoardWideWorkface`，xl 1040px，单级不套抽屉）」+ 补页签清单（本板块上下文 / 体检依据 / 构成 / 组合 / 候选 / 来源 / 设置）；`板块入口作用域` 里的「治理抽屉设置页签」→「单级宽工作面的设置页签」。**标题保留原样**——MODIFIED 按标题匹配，改名会被 openspec 当成新 capability；已在正文注明「标题中的『治理抽屉』是历史命名」
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:117`（Requirement 标题）、`:119`（已改的第 1 段）、`:141`（未改的末段）；另 `openspec/specs/workshop-board-console/spec.md` 「板块入口作用域与最新日报主入口」（R7）里的「治理抽屉设置页签」
- **问题**：第 1 段已改写成「该工作面为**单级宽工作面**，不再是抽屉形态」，但同一 Requirement 的**标题仍叫「治理右轨、治理抽屉与当日其他话题子块」**，**第 141 行仍写「需要动手的治理内容…SHALL 收进治理抽屉」**。改完后这条 Requirement 自己跟自己打架——这正是 design D4 说的「新增条款与既有条款矛盾」同类问题，且本 change 的审计报告 E-5 / 建议 #5 已经把 `:372`/`:394`/R7(`:191`) 三处一并点名，任务只做了其中一处。
- **证据**：
  - `front/app/features/tags/components/console/BoardWideWorkface.vue:1-6,29`（「**单级宽工作面**（AppDialog size=xl，1040px）…单级、不套 32rem 抽屉套抽屉」）——抽屉形态确已退役。
  - `front/app/features/tags/components/console/SectionDrawer.vue` 是唯一 `AppDrawer` 消费者，已退役（只剩自身测试引用）。
  - `front/app/features/tags/components/console/railContext.ts:56-66` 页签 = 本板块上下文 / 体检依据 / 构成 / 组合 / 候选 / 来源 / 设置（比 spec 多「本板块上下文」「体检依据」两页签，审计建议 #5 要求补上，也未补）。
  - 审计原文：`openspec/changes/fix-stale-canvas-specs/audit-board-console.md` E-5 与「建议 1. 第 5 条」。
- **建议**：按 design D4 的纪律 grep 全文（`grep -n "治理抽屉" openspec/specs/workshop-board-console/spec.md` 当前 3 处命中），把标题、`:141` 末段、R7 的「治理抽屉设置页签」统一改成「单级宽工作面」，并补两页签；否则就明确把 R7 登记为「不做」。

### R-3 2026-11-26【High】承诺「在 spec 里记一条死代码说明」完全未兑现
- **状态**：已修复
- **处置**：`矩阵视图` 加一条 blockquote：「已知与条款相悖的死代码 `BoardLaneEvolution.vue:215-217`……待单独 change 删除；删除前 MUST NOT 复活该组件」，承诺兑现
- **位置**：`proposal.md`「不做事」段（「不删死代码…本 change 只在 spec 里记一条说明」）、`tasks.md` 3.4、审计建议 #9
- **问题**：proposal 与 tasks 都白纸黑字承诺要在 spec 里留一条「存在与条款相悖的死代码，待清理」的说明，但 delta 里**零处提及**（`grep -rn "死代码\|最密集\|待清理" openspec/changes/fix-stale-canvas-specs/specs/` → NONE FOUND）。承诺过的「登记为不做」动作没有落到制品上，读者看不到任何痕迹。
- **证据**：
  - `front/app/features/tags/components/console/BoardLaneEvolution.vue:215` 「<h4>最密集的三天</h4>」区块确实在仓库里，且其口径（clueCount / 最密集）与矩阵「SHALL NOT 用事件线数等已退役量纲分档」的 MUST NOT 相悖。
  - `grep -rn "BoardLaneEvolution" front/ --include=*.vue --include=*.ts`（排除自身文件）→ 只命中 `BoardLaneEvolution.test.ts` 与 `LaneTrendChart.vue:6` 的一句注释，**确为死代码**（无渲染路径）。
  - 审计建议 #9 原文：「R4（`:97`）：加一句「历史组件 `BoardLaneEvolution.vue` 已退役，最密集三天区块仅存于死代码」，或直接删掉该组件」。
- **建议**：二选一——(a) 在 R4 加一句并给出 file:line；(b) 把 tasks 3.4 改成「本 change 只在 tasks 登记，不动 spec」并写明去处。现状是承诺与制品两头落空。

### R-4 2026-11-26【High】E-6 只修了一半：「看归档」入口仍在 spec 里写成「矩阵历史格子」，代码里不存在
- **状态**：已修复
- **处置**：「看归档」入口由「矩阵历史格子」改为**编年头**（含 Scenario），并明写「矩阵格子 MUST NOT 提供该出口（矩阵工作面只读）」
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:238`（正文）与同 Requirement 的 Scenario「历史格子带入日期优先」（`:253` 附近）
- **问题**：proposal 的「顺带修」表第 9 行写的是「R26 归档入口位置：**入口在编年头**、导航块在工具行**上方**」，tasks 2.6 只做了后半句。前半句没做，于是 delta 里仍写「由**矩阵历史格子**「看归档」带入的日期」，Scenario 也仍写「用户从矩阵某历史格子点「看归档」」——而矩阵格根本没有这个出口，且同文件 R2 明写「矩阵 SHALL 只读展示，MUST NOT 在格内提供归档/合并/生命周期写操作」，两条自相矛盾。
- **证据**：
  - `front/app/features/tags/components/console/ObservationMatrixGrid.vue` 无 `view-archive` emit（grep 全组件目录，`view-archive` 只出现在 `BoardConsoleBody.vue:171`、`BoardConsoleStage.vue:300/317`、`LaneChronicle.vue:225`、`BoardConsoleShell.vue:546`，源头是阅读槽组件）。
  - 真实入口在当日工作面编年头：`front/app/features/tags/components/console/LaneChronicleHeader.vue:76-79`「看本板块此日归档」；另 `OtherTopicReading.vue:212`「看此日归档」。
  - 审计 E-6 原文与建议 #6：「把「矩阵历史格子『看归档』」改为「**当日工作面（编年头）『看本板块此日归档』**」」。
  - `initialDate` 优先于今天的行为契约本身成立（`useConsoleShellActions.ts:58-64`、`BoardArchiveView.vue:83-84`），所以错的只是**入口位置**这一句。
- **建议**：改正文与 Scenario 的入口位置表述（编年头 / 其他话题阅读区），保留 `initialDate` 优先于今天的契约不变。

### R-5 2026-11-26【Medium】首义来源：前端渲染契约写成代码里不存在的四值矩阵，且与本 Requirement 保留的 Scenario 冲突
- **状态**：已修复
- **处置**：改为代码里的真实文案：按 `source` 显示「人工（选定的 N 个节点）」或「自动（创建话题时写入）」，再按 `anchor_origin.resolved` 显示标题+日期或「创建时写入 · 来源不可考」。原写的「两行 ×（人工留痕/创建时写入）×（来源可考/来源不可考）」是我编的，已删
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/lane-correction-surface/spec.md:219`
- **问题**：「`source`（`auto`/`manual`）与 `anchor_origin.resolved`（布尔）」这半句是对的 ✓，但紧跟的「前端 SHALL 分两行渲染（「**人工留痕 / 创建时写入**」×「**来源可考 / 来源不可考**」）」是代码里不存在的渲染契约：UI 从不出现「人工留痕」或「来源可考」字样；可考态第二行给的是**节点标题 · 日期**而不是「来源可考」；「×」暗示的 4 种组合代码并不渲染。同时该 Requirement 保留的 Scenario 仍写「SHALL 显示「创建时写入 · 来源不可考」」，与新 body 的「来源可考 / 来源不可考」轴说法自相矛盾。这次改写还把 body 层的「SHALL NOT 虚构节点、SHALL NOT 用「最早成员」等推测值冒充」删掉了（该约束在 Go 后端注释里被当作硬约束引用）。
- **证据**：
  - `front/app/features/tags/composables/useLaneCorrection.ts:408-412`：`anchorSourceLabel` = `人工（选定的 ${n} 个节点）` / `自动（创建话题时写入）`（无「人工留痕」）。
  - `front/app/features/tags/components/console/LaneAnchorScope.vue:177-179`（第 1 行「当前首义来源：」+ `sourceLabel`）、`:190-197`（第 2 行「首义来源节点：」+ 可考→`originText`=「标题 · 日期」/ 不可考→「创建时写入 · 来源不可考」/ 读取中→「读取中…」/ 读失败→「读取失败 · 重试」）。
  - `backend-go/internal/topicgraph/repository/daily_report_anchor_read.go:20` 「「自动首义无节点集」是契约硬约束（lane-correction-surface「自动首义不虚构节点列表」）」——后端把这条 spec 条款当锚点引用。
  - 该 Requirement 保留的 Scenario「首义来源不可考时显式不可考」仍写「该块 SHALL 显示「创建时写入 · 来源不可考」」。
- **建议**：按代码的真实两行取值写——来源行 = 「人工（选定的 N 个节点）」/「自动（创建话题时写入）」（读失败未确证时整行不渲染），来源节点行 = 「标题 · 日期」/「创建时写入 · 来源不可考」/「读取中…」/「读取失败 · 重试」；不要在 spec 里造 UI 里没有的字样；并把「SHALL NOT 虚构节点 / 最早成员顶替」补回 body 层。

### R-6 2026-11-26【Medium】步长公式里的「可用宽度」未定义，读者按列宽算会与代码差 18px
- **状态**：已修复
- **处置**：步长公式改为 `min(24px, (列宽 − 节点宽 18px) / (节点数 − 1))`，并注明「分母的『可用宽度』是列宽扣掉节点自身宽度，不是列宽」
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:93`、`:110`
- **问题**：新写法 `min(24px, 可用宽度 / (节点数 − 1))` 把「固定 24px」纠正成了「钳制」✓，但「可用宽度」这个术语在 spec 里没有定义。代码里的 `usable` 是**列宽 − 节点宽(18px)**，不是列宽。读者若按「可用宽度 = 列宽」实现，窄列下步长会多出 18px，与代码不符。这条同时与 design D3「写约束 + 当前取值 + 唯一出处」的自我要求不一致——同批的修正 8 就点名了 `MEMBER_BASE_RADIUS`/`LABEL_BOX_PAD`/`NODE_BLOCK_H` 三个常量，这里却留了一个未定义的量。
- **证据**：
  - `front/app/features/tags/components/console/KeyEventTimelineView.vue:127`（`NODE_SIZE = NODE_RADIUS * 2` = 18）、`:689-691`（`usable = Math.max(0, columnWidth.value - NODE_SIZE)`；`Math.min(KEY_EVENT_NODE_FAN_STEP, usable / Math.max(1, count - 1))`）、`:675` 头注释「step = min(24, (列宽 − 节点宽) / (k − 1))」。
  - `front/app/features/tags/components/console/keyEventLayout.ts:60`（`KEY_EVENT_NODE_FAN_STEP = 24` 为上限常量）。
- **建议**：写成 `min(24px, (列宽 − 节点宽 18px) / (节点数 − 1))` 并指明上限常量 `KEY_EVENT_NODE_FAN_STEP`；Scenario 的 `min(24px, 可用宽度 / 2)` 同步改写。

### R-7 2026-11-26【Medium】lane-correction-surface 还有 3 处「过期」既没修也没登记去处（E-5 / E-6 / E-7）
- **状态**：已采纳
- **处置**：E-5 / E-6 / E-7 三处在 tasks 3.5 逐条登记去处（补缺写 change / 代码侧清理 / 跨族清扫裁定），本 change 不修
- **位置**：`tasks.md` 3.1–3.5（「不做」清单）；`openspec/changes/fix-stale-canvas-specs/audit-lane-correction.md` E-5/E-6/E-7
- **问题**：审计把 lane-correction-surface 的过期项列成 E-1..E-7（归到 R1 / R7 / R6 / R3 四条 Requirement）。本 change 修了 E-1..E-4，**E-5、E-6、E-7 三条既未修、也没写进 tasks 3.x 的「不做 + 去处」**——3.2 只登记了「跨族重复」那 7 条，3.3 只登记了缺写，3.5 只登记「无法核实」。这三条是**过期**（spec 说的和代码做的不一致），不是缺写，正好落在本 change「让 spec 追上代码」的核心目标内，静默丢弃等于把问题留在原地。
  - E-5（中）：Requirement「新增泳道的时间视窗曲线口径」装着**新建泳道**的曲线口径（应归 `workshop-lane-compose`），且修正模式自己的 365 日曲线一字未写；
  - E-6（轻）：Requirement「产品界面不承载解说性长句」已被代码越界两处（`LaneAnchorScope.vue:199`「用下方的节点定一次首义即可留痕。」、`BoardComposeOverlay.vue:399`「本泳道曲线未知（…）」）——这是 MUST 与实现相反，不是缺写；
  - E-7（轻）：Requirement「轻量弹窗只承载单字段操作」的三行表漏了「版块归档 md 560」一行，且同段「用 `useConfirm()` 二次确认」与「手打版块名」互相否证。
- **证据**：
  - `openspec/changes/fix-stale-canvas-specs/audit-lane-correction.md:47-64`（E-5/E-6/E-7 全文与 file:line 证据）、`:17` 总览「过期 4：R1、R7、R6、R3」——R6 与 R3 对应的两条过期项即 E-5 / E-6。
  - `tasks.md` 3.1–3.5 全文无 E-5/E-6/E-7 字样。
  - 主 spec 仍含 `新增泳道` 曲线条款（`openspec/specs/lane-correction-surface/spec.md` 约 `:277`）与「产品界面不承载解说性长句」（约 `:205-221`）。
- **建议**：至少补登记去处：E-5 的「该搬」并入跨族引用清扫批次、E-6 开一条代码 change（文案二选一）、E-7 补表行或登记为不做；不要让「过期」类条目无痕消失。

### R-8 2026-11-26【Medium】proposal 与 tasks 对「补 5 条缺写」给出相反决定，proposal 未回写
- **状态**：已修复
- **处置**：proposal 撤掉「补缺写 5 条」的承诺，改为「缺写不在本 change 范围」一节；与 tasks 3.3 一致
- **位置**：`proposal.md`「补缺写（挑高价值 5 条，其余登记）」表（第 11–15 行）vs `tasks.md` 3.3「不补 13 条缺写」
- **问题**：proposal 的 What Changes 段承诺要补 5 条缺写（`?view=` URL 键 / 行头就地改名 / 修正侧 365 日曲线 / 「↩加回」第三写入口 / 质心池含更早成员向量），并逐条给了 file:line；tasks 3.3 决定的却是「一条都不补 → 另开 change」。两份制品对同一件事给出相反决定，而实际 delta 里 **0 条 ADDED**（只有 9 条 MODIFIED）。归档后读者按 proposal 会以为存在 5 条新增契约。design D5 的「本 change 只做一件事」的原则本身是对的，但决定改了以后 proposal 必须同步——这是制品一致性缺口，不是范围问题。
- **证据**：
  - `openspec/changes/fix-stale-canvas-specs/proposal.md`「### 补缺写（挑高价值 5 条，其余登记）」表 5 行。
  - `openspec/changes/fix-stale-canvas-specs/tasks.md` 3.3「不补 13 条缺写（…）→ 另开 change」。
  - `openspec validate fix-stale-canvas-specs --strict` → 只有 `## MODIFIED Requirements`（6 + 3），无 `## ADDED Requirements`。
- **建议**：把 proposal 的「补缺写」整段移到「不做的事」并写明去处（与 tasks 3.3 口径统一）；顺带在 proposal 的「Impact」里把「补 5 条缺写」从 What Changes 移除。

### R-9 2026-11-26【Medium】tasks 2.10（补 Purpose）标 [x] 但主 spec 未改
- **状态**：已修复
- **处置**：主 spec 的 `lane-correction-surface` Purpose 已从 `TBD - created by archiving change ...` 补成一段真实描述（三栏布局 / 画布几何纪律 / 三类写入口 / 外壳），`openspec validate --specs` 146 全过
- **位置**：`tasks.md` 2.10；`openspec/specs/lane-correction-surface/spec.md:6`
- **问题**：tasks 2.10「lane-correction-surface 的 Purpose（原为 `TBD - created by archiving change ...`）」已勾选完成，但主 spec 第 6 行**仍是** `TBD - created by archiving change lane-manual-correction-loop. Update Purpose after archive.`，proposal 的「已知残留」也仍写着「本 change 一并补上」。schema 规定：已有 capability 的 Purpose **不能**通过 delta 改（delta 的会被忽略），必须直接编辑主 spec——本 change 正确地没有在 delta 里加 `## Purpose`，但也没有按 schema 指定的路径直接改主 spec，于是两头落空。
- **证据**：
  - `openspec/schemas/syntopica-ui/schema.yaml:196-205`：「Do NOT add `## Purpose` to a delta for an existing capability … To change an existing capability's Purpose - including a leftover `TBD` placeholder - edit `<planningHome.root>/openspec/specs/<capability-path>/spec.md` directly.」
  - `openspec/specs/lane-correction-surface/spec.md:6` 仍是 TBD 占位。
- **建议**：归档前按 schema 路径直接编辑主 spec 的 Purpose（这不破坏守恒：仍是同一个 spec 文件），或把 tasks 2.10 改回未完成并在 proposal 里说明留给归档后手改。

### R-10 2026-11-26【Medium】括号注释「顶栏三个切换按钮 + 一个日期入口」与代码不符
- **状态**：已修复
- **处置**：按钮构成改为「三个视图按钮 + 编年头入口」，不再写「一个日期入口」；归档入口描述已同步为编年头（见 R-4）
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:5`
- **问题**：新加的括号注释说顶栏是「三个切换按钮 + 一个**日期入口**」。代码里 `console-seg` 组的第 4 个按钮是「**研究工作台**」（顶栏导航入口，不是视图态、也不是日期入口），顶栏没有任何日期入口；而且「关键事件」按钮带 `v-if="!isNarrow"`，窄屏只剩 2 个视图按钮，固定写「三个切换按钮」不准确。审计建议 #1 已给出可直接抄的正确措辞（「三个同级视图：矩阵 / 追踪、关键事件、文章归档（研究工作台为顶栏导航入口，非视图态）」），实现时把它写反了。
- **证据**：
  - `front/app/features/tags/components/console/BoardConsoleHeader.vue:93-127`：矩阵 / 追踪、`v-if="!isNarrow"` 的关键事件、文章归档、研究工作台（`data-testid="console-view-research"`）。
  - 顶栏其余按钮为 打开日报 / 刷新 / 板块管理 / 上下文 / 新建泳道 / ThemeToggle（`:132-190`）——无日期入口。
  - `front/app/features/tags/composables/laneMatrixModel.ts:11` `ConsoleView = 'matrix' | 'archive' | 'events'`、`useBoardConsole.ts:169-172` `writeViewQuery`（非 matrix 才写 `query.view`）——三个视图态枚举本身 ✓ 正确。
- **建议**：改成「顶栏三个视图切换按钮（矩阵 / 追踪、关键事件、文章归档）+ 一个研究工作台导航入口（非视图态；窄屏隐藏关键事件按钮）」；视图名建议与 UI 按钮文案一致。

### R-11 2026-11-26【Low】左栏第五块名与 UI 可见标题不一致（「辅助标签约束」vs「辅助标签」）
- **状态**：已修复
- **处置**：块名统一用 UI 标题：验收条款写「辅助标签」，说明性文字写「辅助标签约束块（块头渲染为『辅助标签』）」，读者照名字能在界面上找到
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/lane-correction-surface/spec.md:7,34,42,96,154`
- **问题**：五块里前四块的名字都是 UI 上能看到的标题（本泳道成员 / 更早成员 / 已移出（本次）/ 首义范围），第五块写成「辅助标签约束」——那是组件名与注释里的叫法，界面上渲染的块头是「**辅助标签**」。修「四块→五块」时如果读者照名字去找，会找不到「辅助标签约束」这一块。
- **证据**：`front/app/features/tags/components/console/LaneConstraintScope.vue:3`（注释「左栏 · 辅助标签约束」）vs `:111` `<h3 class="lcs-title">辅助标签</h3>`（实际渲染标题）。
- **建议**：统一用 UI 标题「辅助标签」，或写成「辅助标签约束（块头显示『辅助标签』）」。

### R-12 2026-11-26【Low】图例文案逐字与代码差一个空格
- **状态**：已修复
- **处置**：图例逐字对齐代码（`ObservationMatrix.vue:132`）：`当日线索数（不含综合稿） · — 无分区 · ? 数据不可用`，含第一个 `·` 前的空格
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/workshop-board-console/spec.md:48`（及同 Requirement 的 Scenario「图例可见」）
- **问题**：spec 声明「文案为「…」」，但写的串与代码里的不完全一致：代码是「当日线索数（不含综合稿） **·** — 无分区 · ? 数据不可用」（第一个 `·` 前有空格），spec 写成「（不含综合稿）· — 无分区…」（无空格）。文案是逐字断言，有空格差就会写出「按 spec 逐字断言失败」的测试。
- **证据**：`front/app/features/tags/components/console/ObservationMatrix.vue:132` `<span class="omx-hint" data-testid="matrix-legend">当日线索数（不含综合稿） · — 无分区 · ? 数据不可用</span>`。
- **建议**：逐字对齐代码串（含空格），或改成「文案语义为…（以 `matrix-legend` 为准）」避免逐字断言。

### R-13 2026-11-26【Low】顺手新增了一条未列入 9 条修正清单的约束
- **状态**：已采纳
- **处置**：保留该约束（代码成立：左栏五块与右栏「关联推荐」不同名），并在 tasks 2.7 补记「顺带新增『两侧不出现同名块』」，使 9 条清单与实际改动对得上
- **位置**：`openspec/changes/fix-stale-canvas-specs/specs/lane-correction-surface/spec.md:154`
- **问题**：Scenario「三栏内容不重复」末尾新增了「MUST NOT 出现两侧同名的块」——原文没有这条约束。它在代码里成立（左栏五块与右栏「关联推荐」不同名），所以不是错条款，但属于修正 7 之外的**范围外新增规范**，没有出现在 proposal / tasks 的 9 条清单里，也没有走「先审后加」的登记。
- **证据**：`front/app/features/tags/components/console/BoardComposeOverlay.vue:462-506`（左栏 aside 内为 LaneMemberList / LaneAnchorScope / LaneConstraintScope）；右栏为关联推荐清单（`:559` 之后的 `.bco__workbench` 右列）。
- **建议**：保留可以，但要在 tasks 2.7 补一句「顺带新增『两侧不出现同名块』」，让 9 条修正与实际改动对得上。

### R-14 2026-11-26【Low】代码头注释仍写「5.5 / 44px」，与本 change 改后的 7.5 / 52 相冲
- **状态**：已驳回
- **处置**：`LaneCorrectionCanvas.vue:9-11` 的头注释仍写 `5.5 / 44px`——这是**代码注释过期**，不是 spec 问题。本 change 只改 spec，不动代码（改注释属代码 change）。已记入 ledger 待清理
- **位置**：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue:9-11`（文件头注释）、`:369`（「不按半径推导——半径最大值仅 5.5」）
- **问题**：spec 侧已改成 7.5 / 48 / 52（本次修正 8 核实无误），但代码自己的注释还留着 5.5 / 40 / 44 的旧推导。审计 E-2 点名过这两处。本 change 声明「不改任何代码」，所以不改代码本身没错；但这条**也没有登记为「不做 + 去处」**，等于同一处漂移在 spec 与代码两侧并存——下一个读代码注释的人会把 5.5 当基准。
- **证据**：`LaneCorrectionCanvas.vue:161`（`MEMBER_BASE_RADIUS = 7.5`）与 `:157-160`、`:369` 注释中的 5.5 互相矛盾；审计原文 `audit-lane-correction.md` E-2「连代码自己的头注释都还写着「半径恒为 5.5 / 44px」（`LaneCorrectionCanvas.vue:9-11`）与「NODE_BLOCK_H（40）」（`:407`）」。
- **建议**：在 tasks 3.x 补一条「代码注释残留 5.5/44 → 随下次画布改动一并清理」，或在本次直接改注释（属于注释订正，不改行为，但需确认与「零代码改动」的声明不冲突）。

## 核实记录（逐条复核 9 条修正）

| # | 修正项 | 结论 | 代码证据 |
|---|---|---|---|
| 1 | 三视图（matrix/archive/events） | **枚举正确**；括号注释与「共用观察轴」两句有问题 → R-1、R-10 | `laneMatrixModel.ts:11` `ConsoleView = 'matrix'｜'archive'｜'events'`；`useBoardConsole.ts:327-331`；`laneMatrixModel.ts:201-205` `parseViewParam` 只收三值 ✓ |
| 2 | 图例三态文案 | **语义正确**，逐字差一个空格 → R-12 | `ObservationMatrix.vue:132`「当日线索数（不含综合稿） · — 无分区 · ? 数据不可用」；组件头注释 `:5`「单条三态图例」；R11（同文件 `:310`）本就写「仅一条图例…`—` 无分区、`?` 数据不可用」→ R2 与 R11 现已一致 ✓；被删的「颜色=行内相对强度」在 R10（`:269`/`:300-304`）另有条款承接，**没有丢失** ✓ |
| 3 | 步长钳制 | **方向正确**，「可用宽度」未定义 → R-6 | `KeyEventTimelineView.vue:127` `NODE_SIZE=18`、`:689-691` `min(24, max(0,列宽−18)/max(1,k−1))`；`keyEventLayout.ts:60` `KEY_EVENT_NODE_FAN_STEP=24`；`:675`/`:683-688` 注释说明 64px 列宽下固定 24px 会压盖 ✓ |
| 4 | 治理右轨入口名「上下文」 | **入口名正确**；抽屉术语只改 1/4 处 → R-2 | `BoardConsoleHeader.vue:167-175`（`data-testid="console-gov-toggle"`、aria-label「打开本板块上下文」、文案「上下文」）；`:449-453` `@media (max-width:1199.98px) and (min-width:768px)` 显示 → 断点与 spec「768–1199 收进顶栏」一致 ✓；载体 `BoardWideWorkface.vue:1-6,29`「单级宽工作面…不套 32rem 抽屉套抽屉」✓ |
| 5 | 归档排序只有质量/时间 | **完全正确** ✓ | `BoardArchiveView.vue:685-688` 仅 `<option value="quality">强证据优先</option>` 与 `<option value="time">发布时间 ↓</option>`；文件头注释 `:9`「组内排序（质量/时间）」✓ |
| 6 | 时间导航块在工具行上方 | **位置正确**；入口位置（编年头）漏修 → R-4 | DOM：`:606-607` `<section class="ar-time-nav">` → `:657` `<div class="ar-toolbar">`；`boardArchiveView.css:5-9` `.archive{display:flex;flex-direction:column}`，两者间无 `order` 覆盖（`:544` 的 `order:-1` 属 `.ar-row-time`）→ 导航块确在工具行上方 ✓ |
| 7 | 三栏窄档 240/280 + 左栏五块 | **全部正确** ✓ | `BoardComposeOverlay.vue:860-861` `.bco__workbench--correction{380px minmax(0,1fr) 300px}`；`:1029-1031` `@media (max-width:1279.98px){.bco__workbench{240px minmax(0,1fr) 280px}}`（同特异性、源码在后 → 覆盖修正模式）✓；左栏 `:462-506` aside 内 LaneMemberList（成员/更早成员/已移出）+ LaneAnchorScope（首义范围）+ LaneConstraintScope（辅助标签约束）= 5 块 ✓；spec 主 spec 5 处「四块」已全部改完 ✓（唯一瑕疵见 R-11） |
| 8 | 占高 48 / 行距 52 / 半径 7.5 | **完全正确** ✓（本批最关键一条） | `LaneCorrectionCanvas.vue:161` `MEMBER_BASE_RADIUS=7.5`、`:170` `LABEL_TOP_GAP=5`、`:176` `LABEL_MAX_LINES=2`、`:177` `LABEL_LINE_H=12`、`:194` `LABEL_BOX_PAD=4`、`:195` `NODE_BLOCK_H=15+5+24+4=48`、`:376` `nodeRowGap=max(16,48+4)=52` ✓；新写法「当前取值、非契约 + 唯一出处」符合 design D3 ✓（残留见 R-14） |
| 9 | 首义来源两字段 | **字段正确**，渲染契约写错 → R-5 | 后端 `daily_report_anchor_read.go:22-28` `TopicAnchorSourceAuto/Manual = "auto"/"manual"`、`:39-47` `TopicAnchorOrigin{Resolved bool}` ✓；前端 `useLaneCorrection.ts:210` `anchorSource: 'auto'｜'manual'`、`:501` 落地、`:453-457` `normalizeAnchorOrigin` 按 `raw.resolved !== true` 归一 ✓ |

**守恒与范围**：
- `git status --short` → 仅 `?? openspec/changes/fix-stale-canvas-specs/`；`git diff --stat HEAD` 为空 —— 主 spec 与代码**零改动**，delta 全部落在 change 目录内，未外溢 ✓
- delta 结构：`## MODIFIED Requirements` × 2 文件，workshop-board-console 6 条 + lane-correction-surface 3 条 = **9 条**，与 tasks 2.1–2.9 逐条对应 ✓；`openspec validate fix-stale-canvas-specs` → **valid**（6 条 >500 字符的 WARNING 为既有形态，非本次引入）
- 条数守恒：主 spec 现状 `workshop-board-console` **33** 条、`lane-correction-surface` **19** 条；delta 无 `## ADDED` / `## REMOVED` / 无新 Requirement 标题（解析校验：delta 独有标题 = 0，主 spec 独有标题 = 27 / 16，全部被 MODIFIED 覆盖）→ 归档后仍是 33 / 19 ✓
- spec 总数 `ls openspec/specs | wc -l` = **146**，不变 ✓
- `bash scripts/harness/archive-readiness.sh fix-stale-canvas-specs`：①②③⑤⑥⑦ 绿，④ review-trace 因缺 `review.md` 红 —— 本文件即为该项交付物。
- 门禁红线核对（`docs/reference/flow/semantic-board.md`）：本 change 为纯 spec 文本修正，未触碰辅助标签去重（L1/L2/L3）、tag 挂版块判定、首义/embedding 校验等任何后端语义，**8 条 semantic-board 红线均无适用面**；红线层声明域 `semantic-board` 已在 `proposal.md` 与 `stage.md` 双处声明 ✓。
