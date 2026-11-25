# Tasks — key-event-day-focus-cards

> 顺序：布局模型（纯函数）→ 渲染 → 卡片 → 视图头与行头 → 移除阅读区与休眠展开 → token → Review → 测试 → 文档 → 验证。
> 每条任务带真实文件路径 + 行号提示（行号基于本 change 开工时的 HEAD，行号会随实现漂移，以符号名为准）+ 最小验证命令。
> 契约见 `specs/workshop-board-console/spec.md`；实现决策见 `design.md`；已批准 UI 合同见 `ui-design.md`。

## 1. 布局模型（keyEventLayout.ts 纯函数）

- [x] 1.1 在 `front/app/features/tags/components/console/keyEventLayout.ts` 新增 `layoutKeyEventBands(groups, bodyWidth, cardRect)`：按日期升序逐组做**双侧代价分配**——上/下带各算 `100×(该带内已有标签横向覆盖本组任一圆点 x 的条数) + 10×(起始层 + k) + 3×(该带已用层数)`，取代价小的一侧（并列取下带）；下带组的横向范围与卡片矩形相交时改走上带；组内标签近端统一对齐「该日最右圆点 + 12px」，整组贴右缘放不下时左移贴边（`left = max(0, bodyWidth - groupWidth)`）。验证：`cd front && pnpm test:unit keyEventLayout --maxWorkers=2` 全绿，且新增用例覆盖三条断言（覆盖多的带被让开 / 并列取下带 / 与卡片相交改上带）。

- [x] 1.2 在同文件新增 `keyEventLeaderPath(...)`（引线折点：竖段自圆点外缘到标签垂直中线，横段 12px 接入标签近端；圆点 x 落在标签横向范围内时只出竖段）与 `keyEventDayCardRect(selectedIndex, columnWidth, bodyWidth)`（左缘 = 选中列左缘；`left + 280 > bodyWidth` 时改为右缘对齐向左展开）。验证：同 1.1 的命令，新增用例断言「两种翻转」与「只画竖段」两个分支。

- [x] 1.3 在同文件把 `keyEventRowHeight`（现 143 行）改为 `min(246, max(70, 50 + 32 × (La + Lb)))`，并新增 `keyEventRowHeightWithCards(labelRowHeight, cardStackHeight)`（卡片行高**不受** 246 封顶约束，最终行高取两者较大值）；`keyEventLabelsCollapsed`（现 154 行）入参改为「上带 + 下带合计层数」，阈值常量 `KEY_EVENT_MAX_LABEL_LAYERS = 4`（现 51 行）与 `KEY_EVENT_MAX_ROW_HEIGHT = 246`（现 54 行）不动。验证：同 1.1 的命令，断言 4 层 = 178px、5 层收起、卡片行高不被 246 截断。

- [x] 1.4 在同文件把 `KEY_EVENT_NODE_FAN_STEP`（现 45 行）由 `12` 改为 `24`；**逐字保留** `computeKeyEventRequestDays`（现 83 行）与 `projectKeyEventDays`（现 182 行）的签名与实现（design D2：改口径会让轴最左若干天的 confirmed 事件静默不返回）。验证：同 1.1 的命令，既有 `computeKeyEventRequestDays` / `projectKeyEventDays` 用例**零改动**仍绿（review R-4 回归护栏不得被改松）。

## 2. 渲染：双带标签 + 引线 + 圆点

- [x] 2.1 改 `front/app/features/tags/components/console/KeyEventTimelineView.vue`（行体模板约 613–650 行 / 686–723 行，布局入口 `runLabelLayout` 约 357 行）：行体由「单侧标签」改为「上带 / 轴带（18px）/ 下带」三段；轴带纵向位置随上带层数下移（`axisTop = 8 + 8 + 32 × La`——顶部留白 8 + 带间留白 8，与行高公式 `50 + 32×(La+Lb)` 自洽；生命线 `= axisTop + 9`，圆点中心骑生命线、上沿 `= axisTop`）；行高取 1.3 的新公式。验证：`cd front && pnpm test:unit KeyEventTimelineView --maxWorkers=2` 断言 4 层标签行高 = 178px、轴线 top 随 La 变化。

- [x] 2.2 在同组件内加**单层 SVG 引线覆盖层**（每行一个 `<svg>`，逐标签一条 `<path>`，线宽 1.5px、`z-index` 在标签之下、描边取 `--color-event`），折点几何由 1.2 的纯函数给出。验证：同 2.1 的命令，断言「每条标签恰有一条引线 path」且 path 起点 x 与该事件圆点 x 一致。

- [x] 2.3 改 `front/app/features/tags/components/console/KeyEventLabel.vue`（props 定义 12–19 行、样式 40–84 行）：接收双带定位（band + layer 或直接 left/top），保留「自然宽度 + 不截断」（`white-space: nowrap`、**无** `text-overflow`），去掉标签本体上的旧圆点（日期语义由轴上圆点 + 引线承担），描边改 `--color-border-strong`。验证：`cd front && pnpm test:unit KeyEventLabel --maxWorkers=2` 绿，且源码契约断言仍不含 `text-overflow`。

- [x] 2.5 生命线颜色提升：`front/app/features/tags/components/console/KeyEventTimelineView.vue` 的 `.kev-axis-line`（约 965–971 行）由 `--color-border-medium`（对比度 1.67:1）改为 `--color-text-muted`（3.23:1）——引线树的主干从这条线长出来，原色太淡看不见（ui-design Layout Contract）。验证：同 2.1 的命令，断言 `.kev-axis-line` 的 `border-top-color` 不再含 `border-medium`。

- [x] 2.4 改 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 的节点渲染（约 625–640 行 / 698–712 行）与轴头（`.kev-day`）：同日多条按 24px 步长**横向**散开（1.4）、节点描边与文字用 `--color-event`、当前选中日的节点呈实心；选中日强调仍用 `--color-accent`（与事件色区分，design D12）。**并删除 `nodeTop()`（约 396–400 行）里收起行的竖向叠放分支**——delta spec 明写节点 MUST NOT 竖向溢出生命线之外，收起行与其余行一样骑在生命线上。验证：同 2.1 的命令，断言「同日 3 节点 transform 各不相同且步长 24px」「收起行节点的 top 与同层非收起行一致」「选中日节点带实心 class」。

## 3. 日聚焦卡片

- [x] 3.1 新增 `front/app/features/tags/components/console/KeyEventDayCard.vue`（design D7）：props = `event: BoardKeyEventItem` + `reading: LaneDayReadingStore`；emits = `openArticle(articleId: string)`；结构 = 首行「MM/DD 周X · 所属话题」→ 标题（完整、可折行、**不截断**）→ 摘要 1 行节选（点击就地展开全文）→「关联文章 N 篇 ›」（卡片自己的文字链，展开时渲染 `front/app/features/tags/components/console/LaneRelatedArticles.vue`）；摘要与文章的展开态为组件本地状态、**逐卡独立**。同时给 `LaneRelatedArticles.vue` 加一个**可选** prop（默认 `true` = 保留现有内部「相关文章 · N 篇」开关，行为不变），卡片传 `false` 隐藏内部开关、避免双开关与两套文案；现有消费方 `LaneChronicleThread.vue` 不受影响。验证：`cd front && pnpm test:unit KeyEventDayCard --maxWorkers=2` 绿（用例见 8.3）；`LaneChronicleThread` 相关用例仍绿。
  每个可展开控件（摘要区 / 「关联文章 N 篇 ›」）MUST 用 `<button>` + `aria-expanded`（与 `LaneRelatedArticles.vue:54` 的 `.lra-toggle` 同口径），不得用 `div` + `@click`——旧面板卡头就是 `<button aria-expanded>`（`KeyEventDayPanel.vue:72-83`），键盘可达性随面板删除必须由卡片承接。

- [x] 3.2 在 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 挂载卡片列：选中日在**所有**有该日事件的泳道行内展开（活跃行 + 自动展开的休眠行）；卡片列放下带，左缘 = 选中列左缘，靠右放不下时按 1.2 的 `keyEventDayCardRect` 向左翻转且右缘不超过行体宽；卡片矩形参与 1.1 的归带判定（相交则该组改上带）。验证：同 2.1 的命令，断言「两条泳道在同一天各有事件 → 两行都出现卡片列」「最右列的卡片右缘 ≤ 行体宽」。

- [x] 3.3 卡片列 > 4 张时固定高度内部滚动（滚动容器 MUST 挂 `.scroll-thin`——theming.md:38 内部滚动条红线）并标「还有 N 条」（N = 总数 − 4）；≤ 4 张全部展开且该行**不设行高封顶**（design D6）。卡片高度测量口径见 D6：渲染完成后一次性读 `offsetHeight` 求和，MUST NOT 逐卡挂 `ResizeObserver`。验证：同 2.1 的命令，断言「6 张卡片 → 卡片列高度固定 + 出现「还有 2 条」文案」。

## 4. 视图头与行头读数

- [x] 4.1 在 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 视图头（约 504–520 行 `.kev-head`）新增「所属日期」`<select>`：样式沿用 `front/app/features/tags/components/console/BoardArchiveView.vue` 的 `.ar-select` 口径；选项 = 当前 `visibleAxis` 全部日期（**无「不聚焦」项**：`laneConsoleView.ts:reconcileSelectedDate` 对 `current === null` 立即回落到最近报日，`emit('selectDate')` 签名也只收 `string`，该态永远到不了）；选中时 `emit('selectDate', date)`（写入与矩阵同一个 `selectedDate`，不新建状态）；同处显示选中日在**全部泳道**的当日条数。验证：同 2.1 的命令，断言「下拉选项数 = 轴长 + 1」「选某日期 emit `selectDate` 且参数为该日期」。

- [x] 4.2 行头补「当日 k 条」：在 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 的行头模板（活跃行 `.kev-row-head` 约 604 行 / 休眠行约 677 行）追加当前选中日在该泳道的条数（`selectedDate` 恒有值，不需空值守卫）。验证：同 2.1 的命令，断言「选中日某泳道 2 条 → 行头含「当日 2 条」」。

- [x] 4.3 选中日无事件时的说明行（`ui-design.md` State Matrix「empty（选中日无事件）」，去向表第 7 行声称已迁移到这里）：在轴上方渲染一行「MM/DD 没有确认的关键事件」，条件是当日全泳道确认事件总数 = 0；MUST NOT 用该行代替视图头的「MM/DD · 0 条」读数。验证：同 2.1 的命令，断言选中 11-13 时出现该行且视图头为「11/13 · 0 条」。

## 5. 移除下方阅读区与休眠泳道自动展开

- [x] 5.1 删除 `front/app/features/tags/components/console/KeyEventDayPanel.vue`，并摘掉 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 的 import（约 37 行）与挂载（约 727 行）；`front/app/features/tags/components/console/BoardConsoleStage.vue` 的阅读槽条件（约 276 行 `activeView !== 'events' || isNarrow`）**保持不变**（窄屏深链仍要渲染阅读槽）。验证：`grep -rn "KeyEventDayPanel" front/app` 零命中；同 2.1 的命令断言「视图下方不再出现 `key-event-day-panel`」。
  并清掉随面板移除而失效的文案与注释：`KeyEventTimelineView.vue:511-512` 的 `kev-goto` 按钮 `:title="查看下方 ${selectedDate} 的事件"` 改为「所选日居中」（按钮保留）；同文件头注释 :26 的「下方阅读区（KeyEventDayPanel）」、`BoardConsoleStage.vue:192` 与 :273 的「事件视图自带当日事件阅读区」注释一并改写（条件判断本身保持不变）。

- [x] 5.2 休眠泳道选中日自动展开（design D9）：`front/app/features/tags/components/console/KeyEventTimelineView.vue` 内新增本地 `autoDormantDate`——`selectedDate` 变化且存在休眠行在该日有确认事件时置位、否则清空；用户点休眠组折叠按钮时清空（**用户显式折叠优先**，直到选中日再变）；`renderedDormant`（约 179 行）改为「`expandedDormant` 为真取全部休眠行，否则只取 `autoDormantDate` 当日有事件的行」；自动展开的行同样渲染卡片列。验证：同 2.1 的命令，断言「选中日命中休眠行 → 休眠行出现且带卡片列」「换选无事件日 → 休眠行消失」。
  休眠组标题 MUST 同时显示「休眠泳道 N · 当日 M 条」（`ui-design.md` IA + Data Provenance：M = 选中日在休眠行里的确认事件数；M = 0 时只显示「休眠泳道 N」）。

## 6. 事件语义色 token

- [x] 6.1 在 `front/app/assets/css/main.css` 的 `[data-theme="editorial"]`（约 106 行起）与 `[data-theme="dark"]`（约 193 行起）两块各新增 `--color-event`：亮色 `#8f5a17`、暗色 `#e0a75a`（design D12；不硬编码到组件里）；**取值口径以产品真实底色为准**（对 `--color-bg-elevated #f5f0e6` 亮 5.08:1，对 `--color-bg-base #faf7f2` 5.39:1，暗色 ≈ 8.15:1——均过正文级 4.5:1；proto 里的 `#9a6219` 是按原型自己的 `#fffdf9` 算的，对产品底色只有 4.47:1，不得照抄）；**不改** `--color-tag-event`（约 157 行 / 244 行）的值。验证：`grep -nE "^\s+--color-event:" front/app/assets/css/main.css` 命中两处；`grep -nE "^\s+--color-tag-event:" front/app/assets/css/main.css` 的取值与改动前逐字一致。

## 7. Review

- [x] 7.1 派子线程（`change-review` 档，只读）聚焦 **review** 本 change 的规划制品（`proposal.md` / `ui-design.md` / `specs/workshop-board-console/spec.md` / `design.md` / `tasks.md`）与本轮实现 diff：逐条核对 design 每条 Decision 的「落点」可解析性（design-anchor-tax）、旧能力去向表判定（capability-parity-review）、红线回归（只读 / 候选话题不占行 / 列宽公式 / 不截断 / 4 层收起逐行判定 / 失败不冒称 0 条）；**审查**意见按 High / Medium / Low 分类处置后，在 change 目录 `review.md` 建账（`### R-<N> <日期>【High|Medium|Low】<标题>` + `- **状态**：<处置词>`；审毕无发现写显式声明行）。验证：`bash scripts/harness/review-trace.sh openspec/changes/key-event-day-focus-cards` 退出码 0。

## 8. 测试

- [x] 8.1 补 `front/app/features/tags/components/console/keyEventLayout.test.ts`（现 199 行）：新增 `layoutKeyEventBands` / `keyEventLeaderPath` / `keyEventDayCardRect` / 新行高公式 / 收起判据（上带 + 下带 > 4）/ 24px 步长的纯函数用例；**既有** `KEY_EVENT_NODE_TOP === 9`（`keyEventLayout.test.ts:145-147`）用例需改为「圆点中心 = 生命线 = axisTop + 9、上沿 = axisTop」（注意 9 是**半径**不是上沿偏移；轴带随上带层数下移后该常量断言必红，须同步改而非删）；既有 `computeKeyEventRequestDays` / `projectKeyEventDays` 用例零改动保留。验证：`cd front && pnpm test:unit keyEventLayout --maxWorkers=2` 全绿。

- [x] 8.2 改 `front/app/features/tags/components/console/KeyEventTimelineView.test.ts`（现 630 行）：改既有断言（层顶 `46 + 44L` → 新行高公式；行高 90 / 246 断言；底部面板相关用例删除），补「每条标签有引线」「卡片列在全部有事件泳道展开」「下拉写入 `selectDate`」「悬停不改变 `selectedDate`」「休眠自动展开」用例。验证：`cd front && pnpm test:unit KeyEventTimelineView --maxWorkers=2` 全绿。

- [x] 8.3 新增 `front/app/features/tags/components/console/KeyEventDayCard.test.ts`：标题完整不截断、摘要就地展开、关联文章就地展开且复用同一 `reading` store（卡片文字链展开后**不出现**第二个内部开关）、无文章不显示入口、点文章 `emit('openArticle')`；键盘可达（摘要区与关联文章入口是 `<button>`，Tab 可达、Enter / Space 可展开、`aria-expanded` 随态切换）。验证：`cd front && pnpm test:unit KeyEventDayCard --maxWorkers=2` 全绿。

- [x] 8.4 删除 `front/app/features/tags/components/console/KeyEventDayPanel.test.ts`（随组件删除）。验证：`cd front && pnpm test:unit keyEventLayout KeyEventLabel KeyEventDayCard KeyEventTimelineView BoardConsoleShell.integration --maxWorkers=2` 全绿且无「找不到模块」错误（`BoardConsoleShell.integration.test.ts` 覆盖 `LaneRelatedArticles` 的另一消费方 `LaneChronicleThread`，确保 3.1 的可选 prop 未改坏它）。

## 9. 文档

<!-- doc-impact: flow standard -->
<!-- doc-impact-excuse: api=本 change 后端零改动（端点 / 表结构 / 注记三态 / 快照字段全不动），不触及 docs/reference/api/; database=同上，不触及 docs/reference/database/; architecture=本 change 只改前端视图内呈现，不改架构定位与骨架映射，不触及 docs/reference/architecture/ -->

- [x] 9.1 改 `docs/reference/flow/semantic-board.md`：约束 37（约 515 行）措辞「其确认事件在本视图（时间轴与当日阅读区）都不呈现」改为「（时间轴与日聚焦卡片）都不呈现」——**语义不变，只换承载面名称**；文末「变更溯源」表（约 551 行起）追加本 change 一行。验证：`grep -n "日聚焦卡片" docs/reference/flow/semantic-board.md` 命中；`grep -n "key-event-day-focus-cards" docs/reference/flow/semantic-board.md` 命中。
  **同一文件「关键事件视图」节还有 4 处旧口径必须一并重写**（删掉 `KeyEventDayPanel.vue` 后会成悬空引用）：:267「其确认事件在时间轴与当日阅读区都不呈现」；:275 标签布局段（「贪心分层（层顶 46px、层距 44px…）行高…每层 +44px」）；:278「名称由下方阅读区承担」；:281 整段「下方阅读区（`KeyEventDayPanel.vue`）…」；:289 代码入口列表里的 `console/KeyEventDayPanel.vue（当日事件阅读区）`。

- [x] 9.2 登记 `docs/reference/standard/frontend/theming.md`：在 token 段新增 `--color-event`（亮 `#8f5a17` / 暗 `#e0a75a`；用途 = 关键事件圆点描边 / 引线 / 卡片首行日期；注明与 `--color-tag-event` 是两套口径、后者不改值；标注对比度依据（对产品真实 `--color-bg-elevated #f5f0e6`：亮 5.08:1；对 `--color-bg-base #faf7f2`：亮 5.39:1；暗色对暗底 ≈ 8.15:1））。验证：`grep -n "color-event" docs/reference/standard/frontend/theming.md` 命中。

## 10. 验证

- [x] 10.1 前端静态检查（按影响范围）：`cd front && pnpm lint && pnpm exec nuxi typecheck`，期望 0 error。

- [x] 10.2 前端单测（**只跑受影响文件**，不跑全量；参数不带 `--`）：`cd front && pnpm test:unit keyEventLayout KeyEventLabel KeyEventDayCard KeyEventTimelineView BoardConsoleShell.integration --maxWorkers=2`，期望全绿。

- [x] 10.3 UI 实机验收（agent-browser）：按 `ui-design.md` Acceptance 断言——① 每条标签有一根连到其圆点的引线，同日两条呈分叉且标签互不重叠，标签不越出行容器；② 30 天窗口列宽守 64px 下限 + 舞台内横滚 + `LLM生态相关新闻` 行标签全部显示名称；③ 点轴头选日 → 该日在**所有**有事件的泳道内同时展开卡片列，标题完整无省略号、摘要可展开、关联文章可展开并打开既有文章预览；④ 选最右侧日期 → 卡片不伸出舞台右缘；⑤ 取消选日 → 卡片列收起、标签回到上下两带、行高复原；⑥ 视图下方不再出现「当日关键事件」阅读区；⑦ 悬停标签 → 对应引线 / 圆点 / 轴头日期加重、移开恢复、`selectedDate` 不变；⑧ 红线回归：候选话题行不出现、`suggested` / `dismissed` 不出现、右轨选话题仍先切回矩阵 / 追踪、本视图无注记写入口。证据在 `openspec/changes/key-event-day-focus-cards/ui-verification/report.md` 建账（`### A-<N> <场景>【pass|fail|未覆盖】` + 依据行；未覆盖写原因）。验证：`bash scripts/harness/ui-verification-trace.sh openspec/changes/key-event-day-focus-cards` 退出码 0。

- [x] 10.4 双视口截图：1440×900 与 1920×1080 各出亮色 + 暗色共 4 张，确认无页面级横向滚动、卡片不越出舞台、行头与生命线对齐；**暗色下额外断言**「选中日的整列底色 / 轴头加粗 / 实心节点与事件色圆点、引线可一眼区分」（暗色强调色是橙红 `rgba(240,138,75,.85)`，与琥珀事件色色相差仅约 11°，靠三重冗余而非仅靠色相）；「与批准原型的差异说明」写入 `ui-verification/report.md`（信息架构 / 主流程 / 布局模式有变时 MUST 重置 `ui-approval: pending`）。验证：同 10.3 的 `ui-verification-trace.sh` 退出码 0，且 report.md 点名的截图文件真实在盘。

- [x] 10.5 构建与部署（收尾跑一次 build）：`cd front && pnpm build`；`bash scripts/dev/deploy-frontend.sh`（构建静态产物 → 铺 `backend-go/frontend/` → 重启后端 → 健康检查），期望健康检查 200。完工汇报 MUST 含「部署后影响 + 需要的操作」：① 用户可见行为变化——关键事件视图底部「当日关键事件」列表**消失**（改由泳道内日聚焦卡片承接）、标签改为上下两带并带引线、休眠泳道在选中日有事件时**自动展开**、视图新增「所属日期」下拉、行头新增「当日 k 条」；② 需要的操作——无（纯前端展示层改造，无数据迁移、无配置变更、无需重新生成日报或重建注记）；③ 旧数据降级——不适用（事件数据与表结构零改动，回滚即恢复上一版静态产物）。

- [x] 10.6 反馈账本与归档门禁：把 `openspec/changes/key-event-day-focus-cards/human-feedback.md` 中 FB-1 / FB-2 的状态由「处理中」更新为「已消化」（FB-3 已是「已消化」）；然后依次跑 `bash scripts/harness/doc-impact.sh verify openspec/changes/key-event-day-focus-cards`、`bash scripts/harness/check-standards.sh --change key-event-day-focus-cards`、`bash scripts/harness/review-trace.sh openspec/changes/key-event-day-focus-cards`、`bash scripts/harness/feedback.sh verify openspec/changes/key-event-day-focus-cards`、`openspec validate key-event-day-focus-cards --strict`，期望全部退出码 0。

- [x] 10.7 收起行节点补日期前缀 `11.05 ★N`（用户反馈：看不出是哪天）

- [x] 10.8 移除行头选中（用户反馈：本视图无联动，假选中易误解）

- [x] 10.9 卡片行高改为「每行一个 ResizeObserver 观察卡片列」兜底（用户反馈 R-35：点开摘要后行高没跟上）；`cardColumnHeight` 去掉 `cardRect` 空值前提；补区分度用例（摘掉观察者即红）
