
## --color-event token 落点与主线程并发改动提示

M5a 已在 `front/app/assets/css/main.css` 加完 `--color-event`（tasks 6.1 / design D12）：
- 亮色 `[data-theme="editorial"]` 第 166 行：`--color-event: #8f5a17; /* 关键事件语义色：圆点描边 / 引线 / 卡片首行日期（与 --color-tag-event 的文章事件标签口径不同） */`（插在 `--color-tag-keyword-border` 之后、Tag category colors 组末尾）。
- 暗色 `[data-theme="dark"]` 第 254 行：同结构，`--color-event: #e0a75a;`。
- 插入位置均紧跟各自主题块的 tag-category 组末尾；因只允许新增 2 行，注释走行尾 inline comment（与 `--color-bg-elevated-strong` 等既有风格一致），没有新增分节注释头。
- `--color-tag-event`（157/158/159 行）与 `--color-tag-event*`（245/246/247 行）取值逐字未动。
- 对比度已实测：#8f5a17 对 #f5f0e6 = 5.076:1、对 #faf7f2 = 5.395:1；原型色 #9a6219 对 #f5f0e6 仅 4.471:1（不合格）；#e0a75a 对暗 elevated #111b26 = 8.145:1。

⚠️ 并发改动提示：`front/app/assets/css/main.css` 在本模块开工前已有别的 change 的未提交改动（`.content-scroll` → `.scroll-thin` 滚动条统一，+19/-7，位于文件 526 行之后）。我这次只 +2/-0，故 `git diff --numstat` 显示 21/7 —— 7 行删除全部来自他人改动，不要回退。
M5b 登记 theming.md 时，token 落点与用途为「关键事件圆点描边 / 引线 / 卡片首行日期」，需明确与 `--color-tag-event`（文章事件标签既有口径）区分。

**引用**：front/app/assets/css/main.css:166、front/app/assets/css/main.css:254、openspec/changes/key-event-day-focus-cards/design.md:D12、openspec/changes/key-event-day-focus-cards/tasks.md:56

<!-- pinned 2026-11-24T03:47:26Z -->

## 关键事件双带布局：原型即参考实现 + M1 已定的三处口径

## 布局模型的权威参考实现 = 已批准原型（不是 design 文字）

`openspec/changes/key-event-day-focus-cards/ui-prototype/index.html` 里有与 ui-design Layout Contract 一一对应的可运行实现，M2 渲染模块**照它移植**即可，不要从 design 文字重新推导：

- 常量（index.html:866-881）：`HEAD_W=192 MIN_COL=64 LABEL_H=24 STEP=32 PAD=8 AXIS_H=18 ELBOW=12 SAME_DAY=24 GAP=4 CARD_W=280 BASE_ROW=70 MAX_LAYERS=4 NODE_R=9 CARD_MAX=4`。
- `evalSide(band,left,groupWidth,group)`（:946-971）：起始层 = 第一个容得下连续 k 层的层号（判据 `layer.right + GAP > left` 即塞不下）；代价 = `100*covering + 10*(L0+k) + 3*used`。
- `layoutBands(groups,bodyW,cardRect)`（:975-1021）：组近端 `left = maxDot + ELBOW`，`left + groupWidth > bodyW` 时 `left = max(0, bodyW - groupWidth)`；`downAllowed = !(cardRect && left < cardRect.right && left + groupWidth > cardRect.left)`；`downRes.cost <= upRes.cost` → down（并列取下带）；组内 k 条占连续层 `[L0, L0+k)`，层 right = `left + itemWidth`。
- 选中日跳过：原型在 `layoutBands` 循环里 `if (group.date === S.selectedDate) return`（:978）。
- 轴带与行高：`axisTop = 8 + STEP*La + PAD`、`lifelineY = axisTop + AXIS_H/2`、`axisBottom = axisTop + AXIS_H`（:1255-1257）；标签 top：上带 `axisTop - PAD - STEP*layer - LABEL_H`、下带 `axisBottom + PAD + STEP*layer`（:1335-1337）。
- 引线（:1349-1367）：竖段起点 = 圆点外缘（上带 `lifelineY - NODE_R`、下带 `lifelineY + NODE_R`）；圆点 x 落在标签横向范围内 → 竖段止于标签**近缘**（上带 = 标签下缘、下带 = 标签上缘）且无横段；否则竖段到标签垂直中线再横到 `p.left`。
- 卡片矩形（:1190-1196）：`colLeft = selIdx*COLW`；`left + CARD_W > bodyW` → `left = max(0, colRight - CARD_W)`。
- 行高（:1412-1415）：`labelRowH = max(BASE_ROW, 50 + STEP*(La+Lb))`（原型无 246 封顶，产品侧按 tasks 1.3 补 `min(246, ...)`）；`cardRowH = axisBottom + PAD + stackH + PAD`；`rowH = max(labelRowH, cardRowH)`。

## M1（keyEventLayout.ts）已定的三处口径（M2 接线须对齐）

1. **覆盖条数按「标签」计，不按层聚合**：design D3 的实现口径与 tasks 1.1 措辞都是「条数」；原型 `evalSide` 是按层计（`band.forEach(layer => hit ? covering++ : 0)`）。design 已注明「两者在实测数据上等价，实现时以本行措辞为准」→ M1 取**标签条数**（同一层里两条标签各盖住本组不同圆点 x 时计 2）。M1 的 `evalBandSide` 已按标签计。
2. **`layoutKeyEventBands` 多一个可选第 4 参 `selectedDate?: string | null`**：三参形态 `(groups, bodyWidth, cardRect)` 完全兼容；传了则跳过该日分组（落实 design D3 伪代码的「选中日不出药丸」），不传则不跳过。M2 接线时**二选一**即可（传 selectedDate 或自己先过滤），别两边都做（无害但冗余）。
3. **`keyEventRowHeightWithCards(labelRowHeight, cardStackHeight)` 的第二个参数是「卡片列从行顶起占的高度」**（= 轴带下缘 + 8 + 卡片堆叠高 + 8），不是纯堆叠高——纯函数拿不到轴带位置（M1 无法从 labelRowHeight 反解 La/Lb），故该偏移由 M2 在调用处算好传进来。

## 已知跨模块瞬时红（M1 引入，M2 负责修）

`keyEventRowHeight` 公式从 `46+44L` 改为 `min(246, max(70, 50+32*层数))`（tasks 1.3 强制）后，`KeyEventTimelineView.test.ts` 有 2 处断言暂时变红（期望 90 / 246，实际 82 / 210）——该文件属 M2 辖区（tasks 2.1 要求改断言为「4 层 = 178px」），M1 未越界改它。

**引用**：openspec/changes/key-event-day-focus-cards/ui-prototype/index.html:866-881,946-1021,1190-1196,1255-1257,1335-1367,1412-1415；front/app/features/tags/components/console/keyEventLayout.ts（layoutKeyEventBands / evalBandSide / keyEventLeaderPath / keyEventDayCardRect / keyEventRowHeightWithCards）

<!-- pinned 2026-11-24T03:58:27Z -->

## M1 修复 R-15/16/18/19 后的新返回值字段与 ui-design 措辞偏离

M1（keyEventLayout.ts）已按 review R-15/R-16/R-18/R-19 修复，**M2 接线必须知道的新增字段**（都是新增，老字段语义未变）：

1. `KeyEventBandLayout.overflowed?: boolean` —— 该行放不下（有组在两带都找不到可用的连续 k 层）时为 true。调用方据此**收起该行**（MUST NOT 把未校验的层当可用）。
2. `KeyEventBandLayout.unplacedKeys?: string[]` —— 溢出时没放下的标签 key（诊断用）。
3. `KeyEventBandPlacement.crossesGroupLabel?: true` —— 仅在**几何无解**兜底情形（组宽 > 最左圆点 − 12px 且右边又放不下）出现：标签在圆点**左侧**时该标签的引线仍会穿过同组其它标签 → 调用方据此收起该行。正常路径恒不出现。
4. `KeyEventCardRect.clamped?: true` —— 卡片矩形被钳制过（行体窄于 280px / 选中列越出行体右缘）。此时矩形只覆盖行体的一部分，**实际卡片仍按 280px 渲染**，溢出部分由舞台容器内部横滚承接。
5. 新导出常量 `KEY_EVENT_MAX_LAYER_SEARCH = 400`（分层搜索上限）。

**行为变化（M2 渲染需对齐）**：
- R-15：整组放不下时**不再**「左移贴右缘」，改为「摆到圆点左侧」（组右缘 = 该日最左圆点 − 12px）。因此标签**可能在圆点左边**，此时引线近端是标签的**右缘**（`keyEventLeaderPath` 已自动按 dotX 与 labelLeft/labelRight 判定朝向，M2 不必改，但 MUST NOT 假定近端恒为左缘）。
- ⚠️ **偏离 ui-design.md Layout Contract 措辞**：该文档写「标签近端统一对齐到『该日最右圆点 + 12px』」，与同句的「组内引线不穿组内标签」在「靠右贴边」场景冲突（R-15 场景）。实现取「组内引线不穿组内标签」（delta spec 的 MUST，优先），仅在**右边放不下**时近端改为「最左圆点 − 12px」。ui-design.md 的措辞建议后续由主线程订正。
- R-18：`keyEventDayCardRect` 在行体窄于 280px 时返回钳制后的矩形（右缘 = 行体宽），不再越界；调用方负责「舞台内横滚承接卡片溢出」。

**不变**：`layoutKeyEventBands` / `keyEventLeaderPath` / `keyEventDayCardRect` / `keyEventRowHeightWithCards` 的参数顺序与必填性、返回结构主字段均未改。

**引用**：front/app/features/tags/components/console/keyEventLayout.ts:placeBandGroup、front/app/features/tags/components/console/keyEventLayout.ts:layoutKeyEventBands、front/app/features/tags/components/console/keyEventLayout.ts:keyEventDayCardRect、front/app/features/tags/components/console/keyEventLayout.ts:KeyEventBandPlacement

<!-- pinned 2026-11-24T04:39:55Z -->

## R-31~R-34 修复后的口径（行头独立列 / 单条组贴右缘 / 轴头 button）

实机验收缺陷 R-31~R-34 已修，实现口径如下（后续评审/复验按此判断）：

1. **R-32 根因=单条组走了左侧镜像**：`keyEventLayout.ts:placeBandGroup` 现在对 `items.length === 1` 的组在「右侧放不下」时**贴右缘**（原型口径，圆点落进自身横向范围 → `keyEventLeaderPath` 的 inside 分支，引线退化为竖线段，不违规）；只有同日多条组（k≥2）才改摆圆点左侧（R-15 不变式只对组内多条有意义）。实测（30 天、11 条真实标题、64px 列宽、体宽 1920）：全镜像 = 上 2 + 下 3 = 5 层 → 收起成 ★N；单条组贴右缘 = 上 2 + 下 2 = 4 层。14 天/1440（列宽 67、体宽 938）同样 5 → 4 层。`KEY_EVENT_MAX_LABEL_LAYERS`/列宽/标题截断规则全部未动。真实标题宽实测 211~317px（`--color` 无关；`.kev-label` font-size .74rem、padding 0 .45rem、border 1px）。

2. **R-33 根因=行头 sticky 在滚动容器内**（不是内边距算错）：`.kev-row-head` 曾 `position: sticky; left: 0` 且在不透明白底的滚动容器里，横向滚 243px 后覆盖 body 内容 192px（实机 1440×900 量得 scroller.left=290、head=[290,482]、body.left=239、scrollLeft=243）。现在拆成 `.kev-stage = [.kev-head-col(192px, 不滚)] + [.kev-scroll(横滚)]`：行头单元 `.kev-head-cell` 高度 = `rowHeight()`；轴头行与休眠组标题用共享变量 `--kev-axis-head` / `.kev-head-summary-spacer`（同款 `kev-group-btn`，nowrap）等高占位；`containerWidth` 改量 `.kev-stage`（保持 `(舞台宽−192)/日数` 的列宽口径）；`scrollSelectedDateIntoView` 目标 x 不再叠加 192；`innerMinWidth` 只含 bodyWidth。测试断言：滚动区内无 `.kev-row-head`、行头单元与行体逐行等高、标签 left ≥ 0。

3. **R-34**：轴头 `.kev-day` 由 `div` 改原生 `<button type=button>` + `:aria-pressed`，点击 `emit('selectDate', date)`；键盘 Enter/Space 由原生 button 承载；focus 用 `outline`（选中日的 `box-shadow` 下边框强调条会与之打架）。注意 `hoveredDate` 是 computed，MUST NOT 在模板里赋值。

4. **R-31**：卡片高度重测改为**逐轮最多两轮**（`readCardStacks` + `sameCardStacks` + 循环），并在 `.kev-cards` 上加 `@transitionend.passive="scheduleCardMeasure"`；卡片组件本身无高度过渡（无需去掉）。测试用 `offsetHeight` 队列桩（300→420）证明单轮实现会停在 350px、两轮实现到 470px。

**引用**：front/app/features/tags/components/console/keyEventLayout.ts:placeBandGroup、front/app/features/tags/components/console/KeyEventTimelineView.vue（kev-stage / measureCardStacks / axis button）、KeyEventTimelineView.test.ts（azure R-31~R-34 describe）

**引用**：front/app/features/tags/components/console/keyEventLayout.ts:placeBandGroup、front/app/features/tags/components/console/KeyEventTimelineView.vue:measureCardStacks、front/app/features/tags/components/console/KeyEventTimelineView.vue:scrollSelectedDateIntoView、front/app/features/tags/components/console/KeyEventTimelineView.test.ts

<!-- pinned 2026-11-24T10:00:50Z -->
