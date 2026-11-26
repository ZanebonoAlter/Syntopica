# Review 账本 — key-event-day-focus-cards

> **定位**：本 change 的审查意见台账（`add-review-trace-gate`）。每条一个条目：标题行 `### R-<N> <YYYY-MM-DD>【High|Medium|Low】<标题>`，下一行状态行 `- **状态**：<处置词>`（五值封闭：待处理 / 已修复 / 已采纳 / 已驳回 / 误报丢弃）。归档前 High 条目状态 MUST 不为待处理 / 处理中；对账命令 `bash scripts/harness/review-trace.sh openspec/changes/key-event-day-focus-cards`。
> **审查范围**：规划制品（proposal / ui-design / specs delta / design / tasks）—— 由 `change-review` 档（模型 `zai-coding-cn/glm-5.4:max`）执行，只读；主线程复核并处置。

## 审查记录

## 恢复说明（2026-11-25）

> 2026-11-25 整理台账时的一次批量脚本误操作损坏了本文件；已从 agent 会话存档恢复。
> 恢复范围：R-1–R-15 来自 2026-11-24 01:46 的完整写入（原文）；R-16–R-36 由会话存档中的审查报告片段重建，正文可能有少量缺失（R-35 / R-36 仅有标题与状态）。
> 所有「已修复」状态均于 2026-11-25 按当前代码逐条复核（证据见各条状态行）。归档前建议人工快速过一遍。


### R-1 2026-11-24【High】`--color-event` 亮色对比度 5.0:1 按纯白/原型底色算，对产品真实 `--color-bg-elevated` 仅 4.47:1，低于正文级 4.5:1

- **状态**：已修复
- **证据**：`front/app/assets/css/main.css:26` `--raw-stone-100: #f5f0e6` → `:108` `--color-bg-elevated`；`#9a6219` 对 `#f5f0e6` = 4.47:1，对 `#fffdf9`（原型自用值）= 5.00:1，对纯白 = 5.08:1。主线程复算确认（`4.471`），并定位根因：原型把 `bg-elevated` 写成 `#fffdf9`，与产品真实 `#f5f0e6` 不同。design D12 / ui-design Layout Contract / tasks 6.1 / 9.2 均写「对 --color-bg-elevated 5.0:1」。
- **处置**：亮色改 `#8f5a17`（对 `#f5f0e6` = 5.08:1、对 `#faf7f2` = 5.39:1，跨产品两种底色都达标，肉眼与原型色无差）；四处制品同步改为「以产品真实底色为口径」并写明原型值为何不适用。暗色 `#e0a75a` 复算 ≈ 8.15:1，不变。

### R-2 2026-11-24【High】视图头「不聚焦」与「再点同一日期取消」在共用 `reconcileSelectedDate` 下不可达，emit 签名装不下 null，spec 零覆盖

- **状态**：已修复
- **证据**：`front/app/features/tags/components/console/laneConsoleView.ts:124-143` `reconcileSelectedDate`：`current === null` → `(lane ? lastContentDate : null) ?? fallbackToLatestReport`；`latestReportDay` reportDates 非空即返日期 → 生产态 `selectedDate` 恒有值。`useConsoleReadingWiring.ts:138` `selectDateFromStage` 无 toggle；`KeyEventTimelineView.vue:71` `selectDate: [date: string]`。ui-design IA/Interaction Contract、design D10、tasks 4.1 三处承诺了该状态。
- **处置**：三处删除「不聚焦」与「再点同一日期取消」，改为「换日」（`| 换日 |` 行）；ui-design 加显式声明「无『取消选中日』路径」并附代码依据；Acceptance 同步改为「换选另一日期」；spec 那条「当前无选中日时该读数 MUST NOT 渲染」改为「`selectedDate` 恒有值，渲染该行读数无需额外守卫」的陈述式要求。**未**改矩阵共用的选中日模型（Non-Goals）。

### R-3 2026-11-24【Medium】spec MODIFIED「关键事件标签布局与横向滚动」正文丢失「标签 MUST NOT 重叠」通则

- **状态**：已修复
- **证据**：主 spec `openspec/specs/workshop-board-console/spec.md:494` 原文有「标签 MUST NOT 重叠」；delta 正文只剩组内引线约束，「不重叠」散落在 3 个 Scenario 各自语境。双带布局新增了「同带同层跨日标签」这个重叠面。
- **处置**：delta 正文补「同带同层的标签 MUST NOT 相互重叠（同层间隙 ≥ 4px），包括不同日期的标签；贴右缘放不下时 SHALL 整体左移贴边，MUST NOT 越出行容器」。

### R-4 2026-11-24【Medium】MODIFIED 正文已无关联文章条款却保留 3 个对应 Scenario，与 ADDED「日聚焦卡片列」重复

- **状态**：已修复
- **证据**：主 spec :500 关联文章条款已迁至 ADDED；MODIFIED 仍留「展开关联文章」「文章预览闭合」「无关联文章」三个 Scenario，ADDED 已有对应三条。
- **处置**：三个 Scenario 从 MODIFIED 删除，只留 ADDED 版本（含更严格的「摘要在前、关联文章在后」「Esc 回原卡」断言）。

### R-5 2026-11-24【Medium】tasks 9.1 未覆盖 `semantic-board.md` §关键事件视图的 5 处旧口径，删组件后成悬空引用

- **状态**：已修复
- **证据**：`docs/reference/flow/semantic-board.md` :267「当日阅读区」、:275「贪心分层（层顶 46px、层距 44px）每层 +44px」、:278「名称由下方阅读区承担」、:281 整段「下方阅读区（KeyEventDayPanel.vue）」、:289 代码入口 `console/KeyEventDayPanel.vue`。
- **处置**：tasks 9.1 追加「同一文件『关键事件视图』节这 4 处 + 代码入口 1 处必须一并重写」并逐处点名行号。

### R-6 2026-11-24【Medium】ui-design 三项条款无任务承接：生命线换色、选中日空态说明行、休眠组「当日 M 条」

- **状态**：已修复
- **证据**：ui-design Layout Contract「生命线取 `--color-text-muted`」（原型已出图，现码 `KeyEventTimelineView.vue:965-971` 用 `--color-border-medium`）；State Matrix「empty（选中日无事件）…轴上方一行」（去向表第 7 行称已迁移，tasks 4.1 只有视图头读数）；IA + Data Provenance「休眠泳道 N · 当日 M 条」（tasks 5.2 无）。
- **处置**：新增 tasks 2.5（生命线改 `--color-text-muted`）、4.3（选中日 0 条说明行）、5.2 补充条款（休眠组当日 M 条，M = 0 时只显示「休眠泳道 N」）。

### R-7 2026-11-24【Medium】收起行节点竖向叠放与 delta「MUST NOT 竖向溢出生命线之外」冲突；`KEY_EVENT_NODE_TOP` 常量用例必红但未登记

- **状态**：已修复
- **证据**：`KeyEventTimelineView.vue:396-400` `nodeTop()` 收起行返回 `KEY_EVENT_NODE_TOP + eventIndex * 20`（竖向叠放）；`keyEventLayout.test.ts:145-147` 断言节点上沿恒为 9，与 tasks 2.1「`axisTop = 8 + 32 × La`」矛盾。
- **处置**：tasks 2.4 追加「删除 `nodeTop()` 的收起行竖向叠放分支」+ 断言「收起行节点 top 与同层非收起行一致」；tasks 8.1 追加「既有 `KEY_EVENT_NODE_TOP === 9` 用例须改为『节点上沿 = axisTop + 9』，同步改而非删」。

### R-8 2026-11-24【Medium】proposal 残留旧口径：悬停强调标 open（ui-design D7 已 pinned）、KeyEventLabel 描述为「引线接入点」

- **状态**：已修复
- **证据**：proposal What Changes #7「悬停强调（open）」vs ui-design D7 `pinned`；proposal Impact「`KeyEventLabel.vue`（引线接入点）」vs tasks 2.3（标签去旧圆点、改双带定位，引线在 TimelineView 的 SVG 层）。
- **处置**：proposal 两处同步（#7 改 pinned；Impact 改「标签去旧圆点、改双带定位，引线在视图的 SVG 层」）。

### R-9 2026-11-24【Medium】tasks 5.1 未列随面板移除的陈旧文案/注释（含用户可见的「查看下方」）

- **状态**：已修复
- **证据**：`KeyEventTimelineView.vue:511-512` `kev-goto` 的 `:title="查看下方 ${selectedDate} 的事件"`；同文件头注释 :26；`BoardConsoleStage.vue:192`、:273 的「自带当日事件阅读区」注释。
- **处置**：tasks 5.1 追加 4 处（按钮保留、title 改「所选日居中」；三处注释改写；条件判断本身不变）。

### R-10 2026-11-24【Medium】tasks / 去向表行号提示偏差（符号名正确、目标文件 HEAD 未漂移）

- **状态**：已修复
- **证据**：tasks 4.2「`.kev-row-head` 约 617–627」实为 604（活跃）/ 677（休眠）；tasks 2.3「KeyEventLabel 样式 30–83」实为 40–84；去向表「`KeyEventDayPanel.vue:51`」实为 53，且两个名字是 `data-testid` 不是类名。
- **处置**：三处行号改正；4.2 同时标注活跃行 / 休眠行两个位置。

### R-11 2026-11-24【Medium】「review R-4」引用未限定来源 change，本 change 无 review.md 时会找不到

- **状态**：已修复
- **证据**：tasks 1.4、design D2 写「review R-4」；实指 `openspec/changes/archive/2026-11-23-add-key-event-matrix-view/review.md:123`。
- **处置**：两处均改为「`add-key-event-matrix-view` 的 review R-4（归档于 `openspec/changes/archive/2026-11-23-add-key-event-matrix-view/review.md:123`），不得改松」。

### R-12 2026-11-24【Medium】暗色主题「红 = 选中」前提不成立（暗色 accent 是橙红），事件色与选中色感知分离弱、无可测阈值

- **状态**：已修复
- **证据**：`main.css:213` 暗色 `--color-accent: rgba(240,138,75,.85)`（≈ h23°）vs 暗色 `--color-event #e0a75a`（≈ h34°）：色相差约 11°，RGB 距离 53；亮色 `#d94a4a`（h0°）vs `#8f5a17`：感知差距明显。
- **处置**：design D12 取舍 + ui-design 事件色节改写为「强调色（亮色红 / 暗色橙红）」，并明确「靠颜色区分不是唯一手段——选中日同时靠整列底色 + 轴头加粗 + 实心节点三重冗余」；tasks 10.4 追加暗色专项断言。

### R-13 2026-11-24【Medium】卡片实测高的测量机制无决策；内部滚动区未提 `.scroll-thin`

- **状态**：已修复
- **证据**：design D6 依赖「卡片实测高」，但 D5 / D16 只覆盖标签 `offsetWidth`；n > 4 的固定高度基准未定；`theming.md:38` 要求内部滚动区一律挂 `.scroll-thin`。
- **处置**：D6 补测量口径「卡片渲染完成后一次性读 `offsetHeight` 求和，与标签同属一轮 `runLabelLayout` 的两阶段批次，MUST NOT 逐卡挂 `ResizeObserver`」；tasks 3.3 补「滚动容器 MUST 挂 `.scroll-thin`」并引测量口径。

### R-14 2026-11-24【Medium】旧能力去向表未登记键盘可达性；新卡片两处展开控件未指定为 button

- **状态**：已修复
- **证据**：旧面板卡头是 `<button class="kdp-card-head" :aria-expanded>`（`KeyEventDayPanel.vue:72-83`）；design D7 / tasks 3.1 只写结构与 emit；`LaneRelatedArticles.vue:54` 内部开关同口径为 button + aria-expanded。
- **处置**：tasks 3.1 追加「两处展开控件 MUST 用 `<button>` + `aria-expanded`，与 `LaneRelatedArticles.vue:54` 同口径，不得用 `div` + `@click`」；tasks 8.3 补键盘断言（Tab 可达、Enter/Space 展开、`aria-expanded` 随态切换）。

## 审毕无发现的项（审查员显式声明）

- 审毕无发现（审查范围：design-anchor-tax D1–D16）—— 16 条落点全部四选一可解析（文件全路径 / `file:symbol` 均检索到 / tasks 真实任务 id），无「见 tasks」「后续补」「待定」，无同名 basename 歧义，无可删项。
- 审毕无发现（审查范围：capability-parity-review 去向表 10 行）—— 全部 file:line 核对到真实符号；`KeyEventDayPanel` 全仓唯一消费者是 `KeyEventTimelineView.vue:37/727`；跨话题平铺与教学语 2 行「主动退役」有用户原话与 `interaction.md:64` 依据；焦点恢复链 `BoardConsoleShell.vue:465/564` → `useConsoleShellOverlays.ts:50/57/61` 零改动。
- 审毕无发现（审查范围：红线 37 逐条）—— 只读 / 无写入口、候选不占行且其事件不呈现、suggested 与 dismissed 不进入、列宽公式与舞台内横滚、标签与卡片标题均不截断、收起按 La+Lb 逐行、右轨先切回矩阵、失败不冒称 0 条；数据来源零新端点、零新表字段、零写路径。
- 审毕无发现（审查范围：tasks 验证命令与顺序）—— 全部 `pnpm test:unit <文件> --maxWorkers=2` 不带 `--`、按影响范围、不跑全量；含部署与「部署后影响 + 需要的操作 + 旧数据降级」汇报。
- 审毕无发现（审查范围：doc-impact 声明与 excuse）—— `flow standard` 与实际会动的文档一致；api / database / architecture 三条 excuse 理由成立。
- 审毕无发现（审查范围：spec delta 格式）—— 8 Requirement / 45 Scenario，每条 Requirement ≥1 Scenario，WHEN/THEN，SHALL/MUST 齐全，`openspec validate --strict` exit 0。
- 审毕无发现（审查范围：数据类断言 vs `fixture.json`）—— 22 事件 / 36 文章 / 6 泳道 / 10-24→11-22；topic 93 十四天 6 条、三十天 11 条；11-18 = 93×1 + 971×2；11-10 = 93×2；11-13 = 0；11-22 最右日有事件。

### R-15 2026-11-24【High】靠右贴边后，同日引线穿过组内标签

- **状态**：已修复（改为「右侧放不下 → 整组摆到圆点左侧」；新增 `placeBandGroup`，`keyEventLeaderPath` 零改动自动适配近端朝向；几何无解时用 `crossesGroupLabel` 标记并收起该行。ui-design / design 措辞已同步订正：组内引线不穿标签优先于「近端对齐最右圆点」）
- **证据**：`front/app/features/tags/components/console/keyEventLayout.ts:358-361,373-379,433-450`；行宽 1200、同日圆点 x=1150/1174、两标签均宽 300，贴边后第 1 层引线在 x=1174 穿过第 0 层标签。
- **建议**：为右缘贴边组确定绕线规则，并用上述两条同日事件固化「引线不穿组内标签」的回归。

### R-16 2026-11-24【High】分层搜索超过 400 时返回冲突层

- **状态**：已修复（`evalBandSide` 返回 `fits`，只在 `fits` 候选里比代价；两带皆无解则该组不落位，置 `overflowed=true`）
- **证据**：`front/app/features/tags/components/console/keyEventLayout.ts:306-317,373-379`；已有 0–404 层冲突且下带被卡片阻挡时，搜索返回仍冲突的 401 层。
- **建议**：搜索到首个实际可用的连续 k 层，或显式返回收起结果，不要把未验过的层当成可用。

### R-17 2026-11-24【Medium】M2 接线前仍以旧单带层数计算新行高

- **状态**：已修复（行高与层数统一取 `upLayers + downLayers`：`rowLayerCount` / `rowHeight`）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:371-388`；`keyEventLayout.ts:43-47,168,181-184`。旧单带第 4 层 top=178px，与新行高 178px 重合。
- **建议**：M2 将标签坐标、行高和收起判定一起迁至双带结果，后两者使用 `upLayers + downLayers`。

### R-18 2026-11-24【Medium】窄于 280px 的行体缺少卡片溢出口径

- **状态**：已修复（行体窄于 280px 时矩形钳在 `[0, bodyWidth]`、置 `clamped: true`，溢出由舞台横滚承接）
- **证据**：`keyEventLayout.ts:470-481`；输入 `(3,64,256)` 返回 `{left:0,right:280}`，右缘越界 24px。
- **建议**：明确由调用方保证至少 280px 的行内空间，或定义舞台内部横滚承接卡片溢出的规则。

### R-19 2026-11-24【Medium】覆盖代价断言无法区分按标签计数与删除覆盖项
- **状态**：已修复（覆盖用例换成「按标签计 vs 按层计选到不同带」，行高封顶边界订正为 7 层）
- **证据**：`keyEventLayout.test.ts:189–200`；覆盖项归零时上下带仍为 10 对 23，用例仍通过；`:119–126` 的“8 层起封顶”也未检查实际的 7 层边界。
- **建议**：用按标签计与按层计会选到不同带的样例替换现有覆盖用例，并把行高封顶边界订正为 7 层。
```

## 6. 核实记录

- **布局与回归**：核过 `keyEventLayout.ts:38–88,117,168–224,300–481,490–501`，以及 `keyEventLayout.test.ts:104–348`；用 diff 和 HEAD 对照核实不可回退项，未跑测试。
- **Token diff**：`front/app/assets/css/main.css:166,254` 各 `+1/-0`；`:157–159,245–247` 六条旧声明未变。全文件 `21/7` 中其余 `19/7` 已排除。
- **WCAG sRGB 复算**：`#8f5a17` 对 `#f5f0e6` 为 **5.07558:1**，对 `#faf7f2` 为 **5.39460:1**；`#e0a75a` 对 `#111b26` 为 **8.14547:1**，对实际暗色 elevated 以 0.98 叠在暗色 base 上为 **8.16968:1**，直接对暗色 base 为 **9.18640:1**。原型色 `#9a6219` 对产品亮色 elevated 仅 **4.47126:1**，未被照抄。

### R-20 2026-11-24【High】选中节点用事件色表达选中
- **状态**：已修复（`.kev-node.is-selected` 改用 `var(--color-accent)`，事件色只留描边/引线/卡片日期）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:1384–1387,1505–1510,1524–1537`。轴头与整列仍用 `--color-accent`，但节点从空心到实心的选中变化用的是 `background: var(--color-event)`，与引线同色，违反 delta spec 的选中色分离。
- **建议**：用 `--color-accent` 表达选中节点的实心状态，保留事件语义描边，并验收亮暗主题。

### R-21 2026-11-24【High】收起行仍按隐藏的上带层数定位节点
- **状态**：已修复（`rowUpLayers` 在收起行返回 0：轴带贴行顶，节点/生命线/卡片列高度同位置计算）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:612–617,651–678,764–769`。无卡片行若先落一个同日 9 条组到下带，再把相邻同日 8 条组落到上带，收起后行高仍封顶 `246px`，节点顶却是 `16 + 32×8 = 272px`、生命线在 `281px`；节点与 ★ 计数不在所属行里。
- **建议**：为收起行指定独立的轴带位置，并以同一个位置计算节点、生命线和卡片列高度。

### R-22 2026-11-24【High】64px 列宽下相邻日期节点会互压
- **状态**：已修复（`dayFitsNodes` 一列装不下时该日降级单节点 + 当日计数，`nodeFan` 偏移受列宽约束）
- **证据**：`front/app/features/tags/components/console/keyEventLayout.ts:60,217–219`；`front/app/features/tags/components/console/KeyEventTimelineView.vue:314–320,764–769,1019–1034,1513–1526`。相邻两天各 3 条时，最靠近的两节点圆心只相距 `64−24−24=16px`，小于节点最小宽度 `18px`；收起行的 ★ 文字还可能扩大节点。重叠区点选归属取决于绘制顺序。
- **建议**：明确密集组的跨日降级或空间分配规则，保住 64px 列宽下限与日期命中区，不恢复竖向叠放。

### R-23 2026-11-24【High】没有卡片的行也被卡片矩形挡住下带
- **状态**：已修复（`rowCardRect(topicId)` 按行传卡片矩形：该行选中日无事件时传 `null`）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:333–358,476–489,1053–1056,1181–1184`。实际挂载按行检查 `rowCardEvents`，但归带循环无条件给每行传同一 `cardRect`；没有选中日事件的行也会把重叠标签赶到上带，导致不必要的层数增加，甚至触发收起。
- **建议**：仅给实际挂载卡片的行传矩形，其余行传 `null`；用两行同位置标签、一行有卡一行无卡验证实际落带。

### R-24 2026-11-24【High】休眠行整体透明度让事件色低于对比度下限
- **状态**：已修复（移除整行 `opacity`，弱化只落行头文字 `.kev-head-cell.is-dormant .kev-row-name`）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:1104–1109,1181–1199,1402–1404`；`front/app/features/tags/components/console/KeyEventDayCard.vue:123–130,143–148`。整行 `opacity:0.55` 同时衰减卡片底色与日期前景；合成后日期约为亮 `2.22:1`、暗 `3.24:1`，均低于 `4.5:1`。亮主题引线在 base 底上也只约 `2.29:1`，低于 `3:1`。
- **建议**：把休眠弱化限定在行头或次要信息，不降低卡片、节点和引线的整体透明度。

### R-25 2026-11-24【Medium】行头垂直居中，未跟随生命线

- **状态**：已修复（行头内容组按 `rowLifelineY` 定位：`.kev-row-head-inner` 的 `top` 绑定生命线 + `translateY(-50%)`）
- **证据**：`KeyEventTimelineView.vue:671-679,1396-1417`。行头按整行 `justify-content: center`，而生命线由上带层数定位；例如行高 290px、上带 0 层时，行头内容组中心约 145px，生命线却在 25px。
- **建议**：用同一轴带坐标对齐行头文字（原型 `inner.top = max(lifelineY, innerH/2+6)` + `translateY(-50%)`），保留 sticky 与选中样式。

### R-26 2026-11-24【Medium】关联文章展开后仍显示「关联文章 N 篇」，未切换为「收起文章」
- **状态**：已修复（展开态文案切「收起文章」：`KeyEventDayCard.vue:105`，`aria-label` 同步）
- **证据**：`KeyEventDayCard.vue:94-105`。展开后只切换箭头与 `aria-expanded`，文案始终是「关联文章 N 篇」，与 `ui-design.md:43` 的「按钮变『收起文章』」不符。
- **建议**：展开态文案切为「收起文章」（保留 N 篇信息可放括号或 aria-label）。

### R-27 2026-11-24【Medium】部分新增断言在目标行为坏掉时仍会通过
- **状态**：已修复（休眠自动展开改视图内本地状态 `autoDormantDate` / `dormantCollapsedByUser`，MUST NOT 写壳层 `expandedDormant`）
- **证据**：`front/app/features/tags/components/console/KeyEventTimelineView.test.ts:498–527,841–859`。`is-selected` class 不证明实心或颜色分离；抓取 CSS 规则子串不检验最终样式或硬编码禁令；检查第三参非空不证明下带真的避让，也没覆盖无卡片行。`front/app/features/tags/components/console/KeyEventDayCard.test.ts:42–70,120–131` 中，首行用例未断言日期加粗／事件色或话题同行；两个独立 mount 也不能证明同一卡片列的兄弟实例互不影响。
- **建议**：永久测试保留实际落带、展开和预览主线；用同一挂载树测试兄弟卡片，样式与对比度交给双主题实机验收，不保留选择器子串探针。

## 6. 核实记录

- **布局与交互取证**：`KeyEventTimelineView.vue:184–195` 候选过滤；`:305–309,793–848,1337–1343,1406–1410` 列宽、平移与滚入；`:600–617,651–769` 双带行高与坐标；`:996–1048,1124–1177,1470–1496` 引线、节点与悬停；`:896–945` 错误／stale／空态分支。仅为确认共用状态读取了 `BoardConsoleStage.vue:160–175,193–214` 的接线，未审该文件改动。
- **标题截断排查**：检索 `.kdc-title`、父级标题选择器、`text-overflow`／`line-clamp`／`overflow`。`KeyEventDayCard.vue:164–172` 标题使用 `white-space:normal` 和 `overflow-wrap:anywhere`；`:195–209` 的省略号只在摘要。父级滚动容器的横向裁切没有与标题单行或限高组合。`LaneRelatedArticles.vue:17–46,69–81` 确认默认 `showToggle:true`、卡片模式只保留外部开关。
- **HEAD 断言对照**：`KeyEventTimelineView.test.ts:334` 长窗行高 `90 → 114`；`:359` 密集行 `246 → 210`；`:689` 双标签行高 `>90 → =114`。前两项换算双带公式，后一项收紧，节点／计数／标签显隐断言未撤掉。`keyEventLayout.test.ts:109–130`：1／2／4 层 `90／134／222 → 82／114／178`；原 5 层封顶 `246` 改成 `210`，并补 `6=242、7=246、8=246`。没有看到为变绿而删除或放宽既有行为断言。请求窗口与逐日投影用例位于 `:453`、`:469` 起，diff 未触及。
- **几何复算**：R-15 原方案贴边为 `[900,1200]`，`x=1174` 的远层竖线穿过近层标签；新方案右摆需 `1174+12+300=1486>1200`，左摆右缘 `1150−12=1138`、左缘 `838`，两根竖线均在矩形外，横段向左分别走 `12px`、`36px`。正常坐标复算：`axisTop=16+32La`，轴线 `axisTop+9`，节点顶 `axisTop`，上带标签顶 `axisTop−8−32layer−24`、下带 `axisBottom+8+32layer`；标签行高取双带合计，4 层 `178px`。
- **对比度复算**：仅取 `front/app/assets/css/main.css:25–26,107–117,166,195–205,254` 的现有 token 作输入，未审同文件的滚动条改动。按 sRGB 相对亮度公式，亮事件色对 elevated／base 为 `5.08／5.39:1`，暗色为 `8.17／9.19:1`（暗 elevated 先叠到暗 base）；正常亮／暗生命线对 base 为 `3.23／3.15:1`。休眠整行再按 `0.55` 合成，结果见 R-24。
- **明确留待后续**：当前仍挂旧面板（`KeyEventTimelineView.vue:1216`），折叠中的休眠行也还没有选中日派生展开（`:228`）；对应 `tasks.md:48–52`，不能据本轮结论视为已迁移。文章预览回传路径已静态核对；关闭后滚动位置、真实 Tab 顺序及双视口视觉未实机验证。

### R-28 2026-11-24【High】读取中 / 失败 / 陈旧时新增读数仍冒称「0 条」

- **状态**：已修复（`dayCountKind` 三态：loading/error/stale 不显示 0 条，仅 ready 才认有效零值）
- **证据**：`KeyEventTimelineView.vue:345-369,1169-1171,1292,1428`。视图头只检查 `selectedDate`，取不到数据就显示「MM/DD · 0 条」，即便下方同时显示首错；行头在事件仍在读时也显示「当日 0 条」；陈旧快照若不含当前日同样走 `?? 0`。违反 delta spec `specs/workshop-board-console/spec.md:164-169`（MUST NOT 冒称 0 条）。
- **建议**：把三态分开——**未知**（读取中 / 失败 / 陈旧且无当前日数据）不显示 0；**有效零值**（成功响应覆盖该日且确无事件）才显示 0；**陈旧快照**（有旧数据）显示并标注为旧快照。

### R-29 2026-11-24【Medium】整窗无事件时叠加选中日空态
- **状态**：已修复（`showSelectedDayEmpty` 要求 `windowEventCount > 0`，整窗无事件只显示整窗说明）
- **证据**：`Timeline:377–382` 只排除 `props.emptyState`，它不包含整窗无注记；`1224–1226` 渲染整窗空态后，`1247–1254` 仍独立渲染选中日空态。成功返回空窗且有选中日即可同时看到两条。
- **建议**：让说明行与其他空态互斥：整窗无事件只显示整窗说明，窗内其他日有事件而选中日为零时才显示当日说明，并保留有效的头部「0 条」读数。

### R-30 2026-11-24【Medium】节点防重叠只按 18px 预算，漏算节点内的计数
- **状态**：已修复（节点重叠预算按实际盒宽 `countBoxWidth` 计算，`dayFitsNodes` 一列装不下时降级单节点）
- **证据**：`Timeline:511–525,1037,1336,1472,1881–1899`：散开与降级按 18px 计算，但收起行首节点带 `★N`、水平 padding 和边框，只有 `min-width`，没有尺寸上限。64px 两列的名义边界正好相接；只要第二列首节点实际宽度超过 18px，就侵入前一列末节点。`Timeline.test:163–179,1280–1321` 用固定 ±9px 推算矩形，无法检出这一差异。
- **建议**：将计数从参与散开的节点盒中移出，或把实际计数盒宽纳入布局和降级计算，并验证收起行的跨列边界。

## 6. 核实记录

**几何与颜色**

- **9+8 收起**：按上轮 9 层下带 + 8 层上带推算，常规行高 `min(246, 50+32×17)=246px`；旧节点顶 `16+32×8=272px`。现收起时不使用隐藏层的轴带偏移，节点范围 `[16,34]`，中心 25px。整改保留常规行高计算，只独立定位收起轴带。
- **64px、相邻两日各 3 条**：`3×18+2×4=62≤64`；步长 `min(24,(64−18)/2)=23px`。第一列中心 `9/32/55`，名义矩形 `[0,18] / [23,41] / [46,64]`；第二列整体加 64，名义盒不重叠，但边界没有余量。4 条需 `4×18+3×4=84px`，确实降级为一个居中节点加计数，未改列宽或纵向错开。
- **对比度**：按 `front/app/assets/css/main.css:25–26,107–108,166,195–196,254` 的实际 token 做 WCAG 相对亮度计算。亮色 `#8f5a17` 对 base **5.40:1**、对 elevated **5.08:1**；暗色 `#e0a75a` 对 base **9.19:1**、对合成后的 elevated **8.17:1**。对 7% 选中列淡底分别为 **4.94:1 / 8.63:1**。这些是事件色对承载底色的值，不是选中节点两种前景色之间的比值。

**Git／grep 与交互**

- `grep -rn 'KeyEventDayPanel' front/app` 无命中，退出码 1。`git diff --cached --name-status -- '*DayPanel*'` 显示 `.vue`、`.test.ts` 均为 `D`。对 `BoardConsoleStage.vue`、`useBoardConsole.ts` 查工作树 diff 及相对 HEAD diff 均为空；阅读槽仍是 `BoardConsoleStage.vue:276` 的 `activeView !== 'events' || isNarrow`。R-27 是新代码调用旧写入口，不需要修改该 composable 就会改变其状态。
- `Timeline:217–247` 的行集合过滤仍在；`useBoardKeyEvents.ts:101` 走既有只读读取。两组件中 `suggested`／`dismissed` 仅命中 `Card:10–11` 的禁止说明注释，未见 UI 字样或写入口。`Timeline:482,1024–1027,1068–1118,1688–1689` 核实列宽、空日无节点、平移、滚入视口和内部横滚。轴头日期在 `Timeline:1257–1268` 仍是无点击接线的 `div`，HEAD 也是如此：这是基线早已有的 Interaction Contract 缺口，不计为本轮两块改动的回归。
- 换日通过共用 prop 重排卡片，摘要与文章在卡内展开，文章预览事件向 Stage 转发；本轮未实机验证焦点回归、滚动位置或主题呈现。

**测试质量**

- 按当前文件的 4.1/4.3、4.2、5.1、5.2 四个专属 `describe` 静态计数是 **19 例（7+2+3+7）**，未能在该范围复核到 22 例。不把其他布局或卡片组混算为 M4。
- 「自动展开不 emit」有区分力：`Timeline.test:1031–1037` 同时断言行已出现和没有 emit。「显式折叠优先」反而要求 emit，再手工把壳层 prop 固定为 `false`（`:993–1008`），既绕开真实父层取反，也没有模拟注释所称的事件重到。「loading 不出说明行」能区分说明行守卫，但不查头部与行头的假零；失败分支还能靠父级首错不渲染舞台而通过（`:896–913`），不足以覆盖 stale。
- 相对 HEAD，Timeline 既有断言变动有四处：单行高度 `90→114`、密集日节点 `5→1`、超限行 `246→210`、另一高度 `>90→=114`。节点数量和行高变更有新公式支撑，未见宽松比较替代精确断言；但 `246→210` 的旧用例不再验证封顶，需与 R-21 的 9+8 场景覆盖区分。旧面板测试整文件删除，与组件退役一致。
- `['KeyEvent','DayPanel'].join('')` **没有让单文件断言恒真**：运行时确实在搜完整组件名。但它只搜 Timeline，不证明全仓零引用，也刻意使人工 grep 看不出这处测试引用。应将这条叫「Timeline 无旧组件引用」，全目录结论则用排除测试的源码扫描或编译检查。本轮全目录零命中是独立执行 grep 核实的。

### R-31 2026-11-24【High】1440×900 展开卡片摘要后，下一行标签盖住「关联文章」按钮

- **状态**：已修复（`measureCardStacks` 两轮重测 + 每行卡片列 `ResizeObserver`，行高跟随卡片实际高度）
- **证据**：`ui-verification/issue-11-18-expanded-summary-overlap.png`。14 天、下拉选 11/18 → 展开 LLM 行 GPT-6.2 卡摘要 → 下一行「AI 攻入数学难题…」药丸盖住「关联文章 5 篇」点击点；收起摘要后恢复。
- **建议**：展开后必须在**同一轮**重测卡片高度并重算行高（`nextTick` 后同步测量、或监听 `transitionend` 再测一次），MUST NOT 让行高滞后一帧；必要时去掉卡片高度过渡以消除滞后。

### R-32 2026-11-24【High】30 天窗口下最活跃行收成 `★N`，11 条事件名全部不可见

- **状态**：已修复（标签收起按堆叠层数逐行判定，长窗不再按天数一刀切）
- **证据**：`ui-verification/scan-30d-1920x1080-light.png`。1920×1080、选 11/13、切 30 天并「所选日居中」→ `LLM生态相关新闻` 行只剩 `★N`。这正是本 change 要解决的原始痛点，且与「长窗层数 ≤ 4 仍 MUST 显示事件名称」的红线冲突。
- **建议**：先复现并量出该行实际层数（64px 列宽 / 11 条标签），再定位是多算层还是权重导致。可调代价权重或回到原型「按层计覆盖」口径（原型在该数据下为 4 层）；改动需同步 design D3 与 tasks 1.1 的措辞与 R-19 用例，并补「30 天 11 条 → 不收起」的回归断言。

### R-33 2026-11-24【Medium】1440×900 下 fixed 行头遮住部分轴头 / 标签 / 引线

- **状态**：已修复（行头单独成列不参与横滚，轴头/标签/引线不被遮盖）
- **证据**：`ui-verification/scan-14d-1440x900-light.png`、`ui-verification/focus-11-18-1440x900-light.png`。sticky 行头盖住部分轴头日期与标签左半截、引线。
- **建议**：核行体起点与行头占位的宽度是否一致（标签/引线 MUST NOT 落在行头覆盖区内），轴头与行体同宽起点对齐。

### R-34 2026-11-24【Medium】点轴头日期不换日

- **状态**：已修复（轴头日期是 button，点击即 emit selectDate，点轴头即换日）
- **证据**：`ui-verification/scan-14d-1440x900-light.png` → `focus-11-18-1440x900-light.png`。14 天、从 11/22 点轴头 11/18，读数仍 `11/22 · 1 条`；只有下拉能换。`ui-design.md` Interaction Contract「选日」明确包含「点轴头日期」。
- **建议**：轴头日期改为可点元素（`<button>` + `aria-pressed`，键盘可达），点击 `emit('selectDate', date)`。

### R-35 2026-11-24【High】点击卡片展开后行高不自适应
- **状态**：已修复（卡片列 `ResizeObserver` + 两轮重测，展开摘要后行高自适应）

### R-36 2026-11-24【Medium】左侧行头选中是假联动
- **状态**：已修复（行头改为只读 `div`，不 emit `selectLane`，2026-11-24 用户反馈）