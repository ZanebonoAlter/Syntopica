# Design — 关键事件视角「引线树标签 + 日聚焦卡片」

> 制品角色：本文件是 **HOW**（怎么实现）。契约（做什么）在 `specs/workshop-board-console/spec.md`；动机与范围在 `proposal.md`；已批准的 UI 合同在 `ui-design.md`（八节 + Interaction Decisions + Data Provenance + 明确不做，本文件的任何决策 MUST NOT 越出该合同）。
> 阶段：design（planning 族）。落点契约见 `docs/reference/开发执行规范.md` §0.6 步骤 4「design 落点复核（design-anchor-tax）」。

## Context

- 现有视图 `front/app/features/tags/components/console/KeyEventTimelineView.vue`（1044 行）已实现「行 = 持久化话题、列 = 同一 `visibleAxis`、confirmed 注记落节点 + 单侧药丸标签 + 下方当日阅读区」。可测逻辑集中在 `front/app/features/tags/components/console/keyEventLayout.ts`（192 行，8 个纯函数），组件只做测量与渲染。
- 现状三处硬伤（`proposal.md` Why）：标签摊在圆点右下 46–200px 处、无连线，认不出是哪一天；同日多条按 12px 步长挤在 18px 圆点上互相压盖 6px；「那一天」的内容只在下方一份跨话题平铺列表里。
- 已批准原型 `ui-prototype/index.html` 用一份可运行 JS 实现了目标算法（`layoutBands` / `evalSide` / 引线 path / 卡片列），常量与 `ui-design.md` Layout Contract 逐字对齐：`LABEL_H=24` / `STEP=32` / `PAD=8` / `AXIS_H=18` / `ELBOW=12` / `SAME_DAY=24` / `GAP=4` / `CARD_W=280` / `BASE_ROW=70` / `MAX_LAYERS=4` / `CARD_MAX=4` / `HEAD_W=192` / `MIN_COL=64`。
- 数据层零改动：`useBoardKeyEvents.ts` / `front/app/api/boardKeyEvents.ts` / `front/app/api/normalizers/boardKeyEvents.ts` 已提供 `BoardKeyEventItem{id, topicId, topicLabelSnapshot, sourcePeriodDate, titleSnapshot, summarySnapshot, articleIds}`。
- 状态层零改动：选中日在 `front/app/features/tags/components/console/useConsoleSelection.ts:reconcileSelectedDate` 唯一持有（壳层 `selectedDate`），矩阵与本视图共用；休眠组展开态在 `front/app/features/tags/composables/useBoardConsole.ts:expandedDormant`。
- 上一个 change（`add-key-event-matrix-view`）实机验收暴露过两个接线层缺陷：组件 ref 拿不到 `offsetWidth`（P1-①，标签全叠第 0 层）、滚动容器晚于组件出现导致列宽钉死 64px（P2-①）。本次重构 MUST NOT 重犯：测宽对象与容器测量都要有回归护栏。

## Goals / Non-Goals

**Goals:**

- 标签与圆点之间有可见连线：读者静态扫读即可认出每条标签属于哪一天（ui-design D3）。
- 选中日在**所有**有该日事件的泳道行内就地展开日聚焦卡片列，一屏读完那一天（ui-design D4）。
- 收掉下方「当日关键事件」阅读区，同一份内容不留两份（ui-design D8 / FB-2）。
- 视图自带「所属日期」下拉 + 当日读数，行头补「当日 k 条」（ui-design Information Architecture）。
- 既有 MUST 一条不丢：只读、候选话题不占行、列宽 `max(64px, 可用宽/可见日数)` + 舞台内横滚、与矩阵共用跨度与选中态、标签完整不截断、收起按层数逐行判定、右轨选中先切回矩阵、失败不冒称 0 条、刷新失败保留旧内容。

**Non-Goals:**

- 不动数据层与后端（端点、表结构、注记三态、快照字段零改动）。
- 不改横向时间轴骨架（行 / 列 / 行头 sticky / 选中日滚入视口 / 拖拽平移）。
- 不改列宽公式、不改 4 层收起阈值、不改与矩阵共用的观察窗控件。
- 不做卡片虚拟滚动；不做每泳道独立大事记轴。
- 不引入第二份 `selectedLane` / `selectedDate` / 跨度状态。

## Decisions

### D1. 布局模型整体落在 `keyEventLayout.ts` 的纯函数层，组件只做测量与渲染

- **结论**：新增四个纯函数（归带分层 / 引线几何 / 卡片矩形 / 行高），全部无 DOM 依赖；`KeyEventTimelineView.vue` 只负责测量宽度、喂输入、按结果摆 DOM。
- **落点**：`front/app/features/tags/components/console/keyEventLayout.ts`
- **取舍**：把算法写成组件内的 `computed` 被否——上一轮的 P1-① 就是「可测逻辑与接线混在组件里」导致单测抓不到、实机才发现；继续沿用单侧贪心 `layoutEventLabels` 被否——双侧代价分配、卡片障碍、树形分叉都超出单侧贪心的表达能力（同一侧放不下时它只会开新层，不会换到另一侧）。

### D2. 保留 `computeKeyEventRequestDays` 与 `projectKeyEventDays`，请求窗口与窗外过滤口径零改动

- **结论**：两个函数逐字保留，签名与返回值不动；事件取数窗口仍按「覆盖可见轴起点」计算，窗外事件仍由 `projectKeyEventDays` 按轴过滤后不渲染。
- **落点**：`front/app/features/tags/components/console/keyEventLayout.ts:computeKeyEventRequestDays`
- **取舍**：改成「按可见轴取数 / 与池对齐 365 天」被否——后端 `days` 窗口是 `[今天−(days−1), 今天]`，轴左沿受池边界钳住（`[anchor−364, anchor]`），封顶会让轴最左若干天的 confirmed 事件**静默不返回**、被当成「无确认事件」渲染（review R-4 已固化为回归用例，见 `openspec/changes/archive/2026-11-23-add-key-event-matrix-view/review.md:123`）。

### D3. 归带与分层：按日期升序逐组做双侧代价分配

- **结论**：新增纯函数 `layoutKeyEventBands(groups, bodyWidth, cardRect)`。每个「同日组」占连续 k 层（k = 该日条数），上带与下带各算代价：

  ```
  cost = 100 ×（该带内已有标签横向覆盖本组任一引线的条数）
       + 10  ×（起始层 + k）
       +  3  ×（该带已用层数）
  ```

  取代价小的一侧（并列取下带）；下带额外规则——组的横向范围与卡片矩形相交时该组改走上带。组内标签默认近端对齐到「该日最右圆点 + 12px」；**右侧放不下时整组改摆到圆点左侧**（组右缘 = 该日最左圆点 − 12px），使组内竖引线全部落在标签横向范围外——「组内引线不穿组内标签」优先于「近端对齐最右圆点」（R-15 修正，原「左移贴右缘」会使圆点落入标签横向范围，必然穿标签）。仅在几何无解（组宽 > 最左圆点 − 12px）时才贴右缘，并以 `crossesGroupLabel: true` 标记、由调用方收起该行。
  （实现口径：覆盖条数按「带内横向范围覆盖本组任一引线的既有标签」统计；原型按层聚合为「该带内存在此类标签的层数」，两者在实测数据上等价，实现时以本行措辞为准并在单测里固化。）
- **落点**：tasks 1.1（新增纯函数 `layoutKeyEventBands`，落在 `front/app/features/tags/components/console/keyEventLayout.ts`）
- **取舍**：固定「上带优先 / 下带优先」被否——下带是卡片列的固定障碍区，上带又是轴上方的空白，单向分配会浪费一侧；按圆点 x 就近分侧被否——标签宽 150–330px 而列宽 64–165px，「就近」与「放得下」经常冲突，就近会把标签推到轴的另一侧之外；把卡片矩形直接排除出下带（不做相交改侧）被否——那会让靠右选中日的卡片无处安放，必须保留「相交则改上带」这一条。

### D4. 引线用单层 SVG 折线，不用 CSS 边框 / 伪元素

- **结论**：每行一个 `<svg>` 覆盖层（宽 = 轴宽，高 = 行高，`z-index` 在标签之下），每条标签一条 `<path>`：从圆点外缘垂直走到标签垂直中线，再水平 12px 接入标签近端边缘；圆点 x 落在标签横向范围内时只画竖段。线宽 1.5px，颜色取 `--color-event`。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：CSS `border` + 伪元素画折线被否——折线需要「竖段 + 水平段」两个方向的坐标计算，外加「圆点是否落在标签横向范围内」的分支；CSS 只能靠元素盒子拼，弯折点与圆点外缘的对齐不可控；每条引线一个独立 DOM 元素被否——一行 20+ 条引线会产生同等量级的元素，单 SVG 内多条 `path` 更省；SVG 而非 canvas 被否——SVG 的 `path` 可被 hover 命中、可被无障碍树读取、可被 `data-ev` 定位做悬停强调。

### D5. 保留两阶段 `offsetWidth` 测宽，但只在布局输入变化时批量跑一次

- **结论**：沿用现有两阶段——先以自然宽度渲染（`visibility:hidden` 保留布局盒）→ `offsetWidth` 实测 → 归带分层。测量对象 MUST 是真实 DOM 元素（组件 ref 回调要解析 `$el`，见现有 `resolveLabelElement`）；实测宽按事件 ID 缓存（现有 `measuredLabelWidths`），重排时优先命中缓存；`measuring` 期间标签隐藏，避免宽度抖动。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:runLabelLayout`
- **取舍**：改用 CSS `container queries` / `ResizeObserver` 自动排版被否——标签与引线的相对位置依赖「实测宽度 + 自定义代价分配」，浏览器自动布局给不出层号与归属带，也就给不出引线几何；每帧重测被否——树莓派上强制回流成本高（见 D13），必须批量且只在输入变化时跑。

### D6. 卡片列矩形、翻转与行高算在纯函数里；卡片行高不受 246px 标签封顶约束

- **结论**：新增纯函数 `keyEventDayCardRect(selectedIndex, columnWidth, bodyWidth)`：卡片左缘 = 选中列左缘；当 `left + 280 > bodyWidth` 时改为 `left = max(0, 选中列右缘 − 280)`（向右缘对齐向左展开）。行高公式拆两段：标签行高 `min(246, max(70, 50 + 32 × (La + Lb)))`；卡片行高 = `轴带下缘 + 8 + 卡片堆叠高 + 8`；最终行高 = 两者取大。卡片堆叠高 = 前 `min(4, n)` 张卡片实测高之和 + 间距（**测量口径**：卡片渲染完成后一次性读 `offsetHeight` 求和，与标签同属一轮 `runLabelLayout` 的两阶段批次；MUST NOT 逐卡挂 `ResizeObserver`（同 D5 取舍）；卡内展开摘要 / 文章只改**该卡自身**高度，行高在同一轮重算里跟上）：n ≤ 4 时全部展开且**不设上限**，n > 4 时固定为该高度并内部滚动（容器挂 `.scroll-thin`）+ 标「还有 N 条」。
- **2026-11-24 补丁（用户反馈「点击卡片展开高度没自适应」）**：行高不能再只靠「点击 / `transitionend`」两个入口触发重测——卡片变高的原因还包括文章列表异步到位、字体回流、跨行折行变化，漏测一次行高就停在展开前的高度、卡片溢出压住下一行标签。改为**每行一个 `ResizeObserver` 观察 `.kev-cards` 容器**（不是逐卡挂，成本与行数同阶）；`ResizeObserver` 缺失（SSR / happy-dom）时静默退回原「点击 + 两轮重测」路径。同时 `cardColumnHeight` 去掉「`cardRect` 为空就返回 0」的前提——矩形只决定卡片列的横向位置，与行高无关，缺矩形时跳行高会让卡片直接压到下一行。
- **落点**：tasks 1.2 / 1.3（新增纯函数 `keyEventDayCardRect` 与 `keyEventRowHeightWithCards`，落在 `front/app/features/tags/components/console/keyEventLayout.ts`）
- **取舍**：卡片用 `position: fixed` / portal 浮出被否——会脱离行的横滚容器，横向滚动时卡片不跟随；卡片列沿用 246px 行高封顶被否——4 张紧凑卡约 116px/张，246px 只放得下 2 张，直接违背「≤ 4 张全展开」（delta spec Scenario「四张以内全部展开不设行高封顶」）；把卡片列做成行内的第二段流式内容被否——卡片矩形必须参与下带标签的避让判定（与标签相交即改上带），流式内容拿不到稳定的矩形。

### D7. 新组件 `KeyEventDayCard.vue`：单张卡片自持展开态，文章走既有缓存与既有预览

- **结论**：新增 `KeyEventDayCard.vue`。props：`event: BoardKeyEventItem`、`reading: LaneDayReadingStore`。emits：`openArticle(articleId: string)`。组件自持两个本地状态：「摘要是否展开」与「关联文章列表是否展开」（逐卡独立，不做全局单开手风琴）。关联文章复用既有 `LaneRelatedArticles.vue` + 壳层注入的 `reading` store（即 `laneDayRelatedArticles` 缓存：≤20 FIFO、在途合乘、晚到隔离），组件不自建缓存；点文章 `emit('openArticle', id)` → `BoardConsoleStage` → `BoardConsoleBody` → `BoardConsoleShell` 的既有 `ArticlePreviewModal` 打开。
  **接线细节（review 已确认无异议）**：卡片自带「关联文章 N 篇 ›」文字链（ui-design 卡片刻意的最后一段），而 `LaneRelatedArticles.vue` 内部**还有**一个自己的「相关文章 · N 篇」开关 —— 两者同时出现会造成双开关 + 两套文案。口径：给 `LaneRelatedArticles.vue` 加一个**可选** prop（默认 `true` = 保留现有内部开关，行为不变），卡片传 `false` 隐藏内部开关、由卡片自己的文字链驱动展开。现有消费方只有 `LaneChronicleThread.vue`（默认 `true`，零变化；review 已核其不受可选 prop 影响）。双开关不共存由 tasks 8.3 的用例守住（卡片文字链展开后不出现第二个内部开关）。
- **落点**：tasks 3.1（新增组件文件 `front/app/features/tags/components/console/KeyEventDayCard.vue`，由该任务创建）
- **取舍**：把 `KeyEventDayPanel.vue` 改名复用被否——面板是「当日全部事件的横向列表」，卡片是「单条事件在行内的窄条」，信息密度与展开语义都不同（面板单开手风琴、卡片逐卡独立）；在 `KeyEventTimelineView.vue` 内联卡片模板被否——该文件已 1044 行，再塞卡片既失控又无法独立测试；自建文章缓存被否——会与 `laneDayRelatedArticles` 形成第二份缓存（口径分裂 + 内存翻倍）；不碰 `LaneRelatedArticles` 而在卡片内自绘文章列表被否——那就复制了 `LaneRelatedArticles` 的条目渲染、按需读与单项重试逻辑（与 `OtherTopicReading` 注释里「MUST NOT 复制」的口径相悖）。

### D8. `KeyEventDayPanel.vue` 及其测试删除，能力由日聚焦卡片承接

- **结论**：删除组件与挂载点（`KeyEventTimelineView.vue:727`），能力去向逐条登记见下「旧能力去向表」。该面板仅被关键事件视图使用（矩阵 / 归档 / 研究工作台不引用），删除不波及其它视图。
- **落点**：`front/app/features/tags/components/console/KeyEventDayPanel.vue`
- **取舍**：保留面板作为「跨泳道当日总览」被否——用户 2026-11-23 明确「直接去掉」，且内容与卡片重复（FB-2）；降级为折叠式抽屉被否——同一内容留两份，违背 D8 与「明确不做」。

### D9. 休眠泳道自动展开用视图内的**本地派生**状态，不写壳层共用状态

- **结论**：`KeyEventTimelineView.vue` 内新增本地 `autoDormantDate: string | null`。规则：① `selectedDate` 变化时，若存在休眠行在该日有确认事件 → `autoDormantDate = selectedDate`；否则 `autoDormantDate = null`；② 用户点休眠组折叠按钮 → 清空 `autoDormantDate`（用户显式折叠优先，此后不再自动展开，直到选中日再变）；③ `renderedDormant` = `expandedDormant` 为真时取全部休眠行，否则只取「在 `autoDormantDate` 当日有事件」的行。自动展开的行与活跃行一样渲染卡片列。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:renderedDormant`
- **取舍**：写进 `useBoardConsole.ts` 的 `expandedDormant` 被否——那是矩阵与关键事件视图共用的状态，写进去会让矩阵视图的休眠组也自动展开，污染共用状态；新增一份持久状态（URL / store）被否——「选中日有没有事件」已是派生量，再存一份必然与事件数据不同步。

### D10. 视图头「所属日期」下拉放在 `KeyEventTimelineView.vue` 的头部行，写入共用 `selectedDate`

- **结论**：视图头新增一个 `<select>`（样式沿用 `front/app/features/tags/components/console/BoardArchiveView.vue` 既有日期 `<select>` 口径），选项 = 当前 `visibleAxis` 的全部日期（**无「不聚焦」项**，见取舍）；选中时 `emit('selectDate', date)`，经 `BoardConsoleStage` → `BoardConsoleBody` → 壳层写入与矩阵同一个 `selectedDate`。视图不自己存选中日；选中日落到窗外时由壳层既有的 `reconcileSelectedDate` 回落，视图侧只表现为「没有卡片列」。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：放进共用的 `ObservationRangeControls` 被否——用户 D6 拍板「关键事件视图自带」，放进共用控件会让矩阵视图跟着出现新控件；用自由日期输入框被否——观察窗内按日跳选是「选一个现成日子」，`<select>` 更快且不引入「输入窗外日期」的新校验面；下拉值写本地状态再同步被否——会造成第二份 `selectedDate`，直接违反「不新增第二份选中日状态」；**加「不聚焦」项 / 再点同一日期取消被否**——`front/app/features/tags/components/console/laneConsoleView.ts:reconcileSelectedDate` 对 `current === null` 立刻回落到最近报日（`latestReportDay`），`emit('selectDate')` 签名也只收 `string`（`KeyEventTimelineView.vue:71`），做出按钮也永远到不了该态；真要支持取消得改矩阵共用的选中日模型，超出本 change Non-Goals。

### D11. 悬停强调用视图内瞬态 `hoveredEventId`，不落持久状态

- **结论**：视图内本地 `ref<string | null>`；标签与节点的 `mouseenter` / `mouseleave` 写入；命中时同时加粗该事件的引线（SVG `path` 的 `stroke-width`）、圆点（`box-shadow`）与轴头该日期；MUST NOT 改 `selectedDate` / `selectedLane`。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：借 `selectedDate` 兼表悬停被否——直接违反 delta spec Scenario「悬停强调不改变选中态」；纯 CSS `:hover` 加粗被否——引线是 SVG `path`，与标签 / 圆点不在同一元素树上，CSS 选择器串不起来，必须有 JS 状态把三者关联（同一 `data-ev` 键）；把悬停写进 store / URL 被否——悬停是瞬态噪声，落持久状态会让 URL 与共享状态被高频写入。

### D12. 新增语义 token `--color-event`（亮 / 暗各一套），不复用也不改动 `--color-tag-event`

- **结论**：在 `front/app/assets/css/main.css` 的 `[data-theme="editorial"]` 与 `[data-theme="dark"]` 两块各加一个 `--color-event`：亮色 `#8f5a17`、暗色 `#e0a75a`。用于事件圆点描边、引线、卡片首行日期。**对比度以产品真实底色为口径**（`--color-bg-elevated` = `--raw-stone-100` = `#f5f0e6`，`--color-bg-base` = `--raw-stone-50` = `#faf7f2`）：亮色对 elevated **5.08:1**、对 base 5.39:1；暗色对暗底 ≈ 8.15:1，均过正文级 4.5:1。（原型取 `#9a6219` 是按原型自己的 `bg-elevated: #fffdf9` 算出的 5.0:1，对产品真实底色只有 4.47:1，故重标定；`#8f5a17` 与原型色肉眼几乎无差。）**不复用** `--color-tag-event`（亮色对 elevated 仅 2.46:1，画 1.5px 细线不可读），也**不改**它的值。登记进 `docs/reference/standard/frontend/theming.md` 的 token 段。
- **落点**：`front/app/assets/css/main.css`
- **取舍**：复用 `--color-accent`（红）被否——强调色代表「我选中的那天」（亮色红 `#d94a4a`、暗色橙红 `rgba(240,138,75,.85)`），琥珀代表「事件」，同色表达两义会让用户读不出「选中」（delta spec Requirement「事件语义色与选中色分离」）；**暗色下两者色相差仅约 11°，故「靠颜色区分」不是唯一手段——选中日同时靠整列底色 + 轴头加粗 + 实心节点三重冗余**，事件色只用于线 / 圆点 / 卡片日期；直接硬编码色值被否——暗色主题会打脸，违反 theming.md「组件只引用 Layer 2 语义 token」；把 `--color-tag-event` 调深后复用被否——那个 token 是文章事件标签的既有口径（多处消费），改值影响面外溢。

### D13. 同日圆点散开步长 12px → 24px

- **结论**：`KEY_EVENT_NODE_FAN_STEP` 由 12 改 24；圆点仍恒在生命线上（不竖向下压到别层标签）。
- **落点**：`front/app/features/tags/components/console/keyEventLayout.ts:KEY_EVENT_NODE_FAN_STEP`
- **取舍**：保持 12px 被否——18px 圆点 + 12px 步长 = 相邻圆点互相压盖 6px（proposal 硬伤 2）；改 32px 被否——64px 的列宽放不下 3 个以上，横向散开会被推出列边界。

### D14. 收起判据仍是「上带 + 下带合计层数 > 4」，行高封顶常量 246px 保留但不再被 4 层触顶

- **结论**：`keyEventLabelsCollapsed` 的入参从「单侧层数」改为「La + Lb」；阈值常量 `KEY_EVENT_MAX_LABEL_LAYERS = 4` 与 `KEY_EVENT_MAX_ROW_HEIGHT = 246` 均不改；标签行高公式从 `46 + 44 × 层数` 改为 `min(246, max(70, 50 + 32 × (La + Lb)))`（4 层 = 178px，仍低于 246px 封顶）。
- **落点**：`front/app/features/tags/components/console/keyEventLayout.ts:keyEventLabelsCollapsed`
- **取舍**：改收起阈值被否——红线 37 与 ui-design 双双锁死「4 层 / 逐行判定」，且真实数据实测 `LLM生态相关新闻` 近 30 天 11 条在 64px 列宽下恰好排 4 层（上 2 / 下 2）不触发收起，改阈值会让用户天天看到收起的行；保留旧的 `46 + 44L` 行高公式被否——双带布局下层距是 32px（原型常量），沿用 44px 会让轴带位置与实际排布错位。

### D15. 测试策略：纯函数补测 + 组件行为断言 + 组件删除，遵守「永久业务测试只留主线与真实回归」

- **结论**：① `keyEventLayout.test.ts` 补纯函数用例——双侧代价分配（覆盖数高的带被让开、并列取下带、相交改上带）、引线几何（竖段 + 水平段、圆点在标签范围内时只画竖段）、卡片矩形（左缘 / 右缘翻转两种）、行高（4 层 = 178px、卡片行高不受 246 封顶）、收起判据（La + Lb > 4）。② `KeyEventTimelineView.test.ts` 改既有断言（层顶 / 行高 / 底部面板）+ 补组件断言（每条标签有引线、卡片列在选中日全泳道展开、下拉写入 `selectDate`、悬停不改变 `selectedDate`、休眠自动展开）。③ 新增 `KeyEventDayCard.test.ts`（标题完整不截断、摘要与文章就地展开、无文章不显示入口、点文章 `emit('openArticle')`）。④ 删除 `KeyEventDayPanel.test.ts`（随组件删除）。⑤ **不新增** API 主线测试——本 change 数据层零改动，无新端点；组件行为断言属「UI 验收证据」范畴，按 `standard/shared/test-design.md`「纯 UI 用现行 UI 验收证据」执行，不为覆盖度补接口。
- **落点**：`front/app/features/tags/components/console/keyEventLayout.test.ts`
- **取舍**：把每个 Scenario 都翻译成一条永久单测被否——`standard/shared/test-design.md` 明确「不按 Scenario 数 / 覆盖率要求新增永久业务测试」，白盒探针用完即弃；保留 `KeyEventDayPanel.test.ts` 改成卡片的测试被否——组件已删除，测试应随实现走（否则指向不存在的模块直接红）。

### D16. 性能：测宽批量化 + 按行布局，量级在树莓派可接受

- **结论**：量级估算——30 天窗 × 6 泳道 × 平均 2 条事件 ≈ 360 条标签。测宽按「每轮一次、逐标签一次 `offsetWidth`」执行（360 次/轮），且只在布局输入（轴 / 事件集合 / 行集合 / 列宽）变化时重跑，不在每次渲染时重跑；实测宽按事件 ID 缓存（`measuredLabelWidths`）使重排轮次多数命中缓存。归带分层是每行一次 O(事件数 × 带内层数) 的纯函数，360 条规模下单轮耗时可忽略。引线是单 SVG 内 360 条 `path`（同量级 DOM，但无强制回流）。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:runLabelLayout`
- **取舍**：为每行引入虚拟滚动被否——行内是绝对定位的散点布局，虚拟化会破坏「一屏读完那一天」；用 `ResizeObserver` 逐标签监听宽度变化被否——360 个观察者的回调成本高于一次性批量测量；引入 canvas 绘制标签被否——标签要可点击、可被无障碍树读取、要能承载中文标题的完整显示。

#### 附录：布局算法伪代码（与 `ui-design.md` Layout Contract 一致）

```text
输入：groups（按日聚合的事件，各带实测宽）、bodyWidth、cardRect（选中日卡片矩形）
bands = { up: [], down: [] }        # 每层记录 {right, labels[]}
for g in groups（日期升序）:
    if g.date == selectedDate: continue        # 选中日由卡片承接，不出药丸
    k = len(g.items)                           # 该组条数 = 占用层数
    left = max(g.dotX) + 12
    if left + g.width > bodyWidth: left = max(0, bodyWidth - g.width)
    for side in [up, down]:
        L0 = 首个满足「该层右边界 + 4 ≤ left」的层号
        covering = 该带内横向覆盖本组任一引线的既有标签条数
        cost[side] = 100*covering + 10*(L0 + k) + 3*(该带已用层数)
    band = (cardRect 相交 ? up : cost 更小的一侧，并列取 down)
    for i, item in g: place(item, band, L0 + i)
La = up 层数; Lb = down 层数
collapsed = (La + Lb) > 4
axisTop = 8 + 32*La + 8; axisBottom = axisTop + 18; lifelineY = axisTop + 9
labelTop(band, layer) = band==up ? axisTop-8-32*layer-24 : axisBottom+8+32*layer
leader(item) = 竖段(圆点外缘 → 标签中线) + 横段(12px 接入标签近端)
cardRect = 选中日有事件 ? { left: 翻转判定(S 列左缘, S 列右缘, 280) } : null
rowHeight = max( min(246, max(70, 50+32*(La+Lb))), cardRect ? axisBottom+8+stackH+8 : 0 )
```

## 旧能力去向表（capability-parity-review）

> 依据 `docs/reference/开发执行规范.md` §0.6 步骤 4 第 1.5 条：凡「让旧组件不再挂载」的替换型 change，review MUST 先建立旧能力去向表。旧入口均按 `git show HEAD:<path>` / 现网旧组件核对真实调用点，非「看新页面像不像」。

| 旧能力 | 旧入口（文件:行 / 组件·事件） | 新承接 | 判定 | 证据 |
| --- | --- | --- | --- | --- |
| 当日事件标题（完整不截断） | `front/app/features/tags/components/console/KeyEventDayPanel.vue:80`（`.kdp-card-title`） | 日聚焦卡片标题（`KeyEventDayCard.vue`，完整渲染、可折行） | 已迁移 | 卡片标题沿用同一 `titleSnapshot` 字段；「标题 MUST NOT 省略号截断」由 delta spec Scenario「卡片标题完整不截断」守住 |
| 所属话题快照 | `front/app/features/tags/components/console/KeyEventDayPanel.vue:81`（`.kdp-card-topic`） | 卡片首行「MM/DD 周X · 所属话题」 | 已迁移 | 同一 `topicLabelSnapshot` 字段；卡片首行日期取 `sourcePeriodDate` |
| 事件摘要（就地展开全文） | `front/app/features/tags/components/console/KeyEventDayPanel.vue:85`（`.kdp-summary`）+ 展开态 `KeyEventDayPanel.vue:36`（`expandedId`） | 卡片摘要节选 + 就地展开全文（逐卡独立展开态） | 已迁移 | 同一 `summarySnapshot` 字段；展开语义由「单开手风琴」改为「逐卡独立」，由 ui-design Interaction Contract「读卡片全文 → 点卡片摘要区就地展开」定义 |
| 关联文章列表（标题 / 来源 / 日期） | `front/app/features/tags/components/console/KeyEventDayPanel.vue:87`（`LaneRelatedArticles` 挂载）+ `front/app/features/tags/components/console/LaneRelatedArticles.vue:38`（`ensureRelatedArticles`） | 卡片内「关联文章 N 篇 ›」→ 复用同一个 `LaneRelatedArticles.vue` 与同一个 `reading` store | 已迁移 | 组件不变、store 不变（`laneDayRelatedArticles` 缓存：≤20 FIFO / 在途合乘 / 晚到隔离）；「无文章不显示入口」由卡片侧 `articleIds.length > 0` 判定守住 |
| 打开文章预览并闭合回原卡 | `front/app/features/tags/components/console/KeyEventDayPanel.vue:92`（`@open-article` → `emit('openArticle')`）→ `front/app/features/tags/components/console/BoardConsoleShell.vue:465` → `front/app/features/tags/components/console/useConsoleShellOverlays.ts:50`（`handleOpenArticle`）→ `front/app/features/tags/components/console/BoardConsoleShell.vue:564`（`ArticlePreviewModal`） | 卡片 `emit('openArticle')` → 同一条链路（`KeyEventTimelineView` → `BoardConsoleStage` → `BoardConsoleBody` → `BoardConsoleShell`） | 已迁移 | 链路中间只换了发出者（面板 → 卡片），终点（`ArticlePreviewModal`）与关闭语义（`handleCloseArticle` 恢复焦点）零改动 |
| 当日事件总览列表（跨泳道平铺） | `front/app/features/tags/components/console/KeyEventDayPanel.vue:65`（`v-for="event in dayEvents"`）+ 挂载点 `front/app/features/tags/components/console/KeyEventTimelineView.vue:727` | 无（该能力按 ui-design D8 / FB-2 由用户决定移除） | 主动退役 | 用户 2026-11-23「直接去掉」；能力已由泳道内卡片逐条承接；delta spec Requirement「日聚焦卡片列」末段与 ui-design「明确不做」同时写明下方阅读区不再渲染 |
| 当日无事件时的空态文案（「这一天没有确认的关键事件 · 在日报阅读台标记」） | `front/app/features/tags/components/console/KeyEventDayPanel.vue:61`（`.kdp-day-none`） | 视图头读数「MM/DD · 0 条」+ 轴上方说明行 | 已迁移 | 语义保留（诚实空态、不伪造 0）；位置从下方列表移到视图头 / 轴上方；由 delta spec Scenario「选中日无事件时如实显示零条」守住 |
| 「点时间轴上的事件节点选择日期，这里显示当天全部确认的关键事件」提示语 | `front/app/features/tags/components/console/KeyEventDayPanel.vue:51`（`.kdp-day-empty-selection`） | 无独立提示语；视图头「所属日期」下拉本身即入口 | 主动退役 | 该句是「下方阅读区」的存在性说明；阅读区退役后提示语无处安放（且属「界面内教学语」，interaction.md 禁止）；选中入口改由下拉与节点点击承担 |
| 摘要展开态在选中日切换时复位 | `front/app/features/tags/components/console/KeyEventDayPanel.vue:38`（`watch(() => props.selectedDate)`） | 卡片按「事件 ID」各自持局部展开态，跨选中日不复用 | 已迁移 | 卡片实例按事件渲染，选中日变化时该日卡片是不同实例，展开态天然隔离；delta spec Scenario「卡片内摘要与关联文章可就地展开」守住可展开性 |
| 当日事件按 ID 聚合的派生 | `front/app/features/tags/components/console/KeyEventTimelineView.vue:141`（`dayEventsByDate`） | 行内卡片按「行 × 选中日」聚合（`RowProjection.byDate` 已在用） | 已迁移 | 同一份 `eventsByDate` 数据源；过滤口径（候选话题不呈现）保持 `rowTopicIds` 过滤不变 |

**判定汇总**：10 行中 8 行「已迁移」、2 行「主动退役」（面板整体列表与其存在性提示语），0 行「遗漏」。

## Risks / Trade-offs

- **[卡片列把行高撑高，破坏「一屏读完」]** → 卡片宽度固定 280px、≤ 4 张全展开但每张约 116px；> 4 张固定高度内部滚动；卡片列是行内的一段，纵向滚动交给舞台容器，用户可滚到卡片区。
- **[下带被卡片占满后标签全被赶到上带，上带快速触顶 4 层]** → 收起判据按合计层数逐行判定，超限行收成「圆点 + ★N」，名称由该行卡片承接；真实数据实测 30 天最密行 4 层不触发收起。
- **[引线穿过别的标签（残余歧义）]** → 悬停强调（D11）把被穿过的引线与对应圆点、轴头日期一起加重，作为消歧手段；原型实测 14 天 1 次 / 30 天 2 次穿越。
- **[双侧代价分配是启发式，可能出现「上带 2 层 + 下带 3 层」的行高跳变]** → 行高按层数自适应，切换窗口时行高变化由两阶段测量的 `measuring` 态遮罩（标签先隐藏再定位），不产生中间抖动。
- **[卡片与引线在同一行体内，横滚时卡片跟着横移]** → 这是预期行为（卡片属于该日该泳道）；「靠右放不下时向左展开」保证卡片右缘不超过舞台右缘，不产生页面级横向滚动。
- **[删除 `KeyEventDayPanel.vue` 影响深链 / 外部引用]** → 该组件仅被 `KeyEventTimelineView.vue` 引用（已 grep 全仓确认），删除后无残留引用；归档前跑 `check-standards.sh` 的防孤立引用段。
- **[休眠自动展开与用户手动折叠互相打架]** → 用户显式折叠优先（清空 `autoDormantDate`），直到选中日再变才恢复自动展开；矩阵视图的休眠组不受影响（状态在视图内）。
- **[`--color-event` 与既有 `--color-tag-event` 视觉相近，用户可能混淆]** → 两者用途不同且 `--color-tag-event` 不改值；在 theming.md 登记时写清「琥珀 = 关键事件，标签事件色是另一套口径」。

## Migration Plan

1. 纯函数先行（D1–D6、D13–D14）：`keyEventLayout.ts` 扩函数 + 单测绿，旧组件行为不变。
2. 渲染层改造（D4–D5）：`KeyEventTimelineView.vue` 三段行体 + SVG 引线 + `KeyEventLabel.vue` 双带定位。
3. 卡片层（D7）：新增 `KeyEventDayCard.vue` + 挂载卡片列（D6）。
4. 头部读数（D10–D11）+ 休眠自动展开（D9）。
5. 移除旧面板（D8）+ 删测试；补齐 token（D12）与文档。
6. 部署：`bash scripts/dev/deploy-frontend.sh`（构建静态产物 → 铺盘 → 重启后端 → 健康检查）。
7. 回滚：纯前端改动，回滚即恢复上一版静态产物；无数据迁移、无后端回滚动作。

## Open Questions

- 无。全部决策均落在已批准的 `ui-design.md` 合同之内（D1–D8 对应 ui-design D3/D4/D5/D8 与 Layout Contract；D9–D12 对应 Information Architecture / Component Reuse / 事件色 token 条款），无需要用户拍板的分歧。
