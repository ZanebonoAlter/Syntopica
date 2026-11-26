<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

# fix-daily-report-ke-row-layout

## Why

日报阅读器里带 AI 建议的线索行左右占比失调（用户截图：标题/摘要列被挤成约 18% 窄条，AI 理由占右侧约 80%，确认/不采纳按钮堆在窄列下方），用户要求左右各占 50%，且把 AI 建议动作按钮放到行右下角（符合传统操作直觉）。

根因不是简单的比例参数，而是一个 Vue scoped CSS 断链 bug：

- `KeyEventMarkerActions.vue` 是**多根 fragment 组件**（根节点为 `.ke-detail` + `.ke-actions`）。Vue 3 中**多根组件的根节点不继承宿主的 scope 属性**（实测 DOM：根节点只有 `data-v-0ad49927`，无宿主的 `data-v-bea58fa8`）。
- 宿主 `DailyReportTopicSection.vue` 里的摆放规则编译为 `.drm-thread-row[data-v-bea58fa8] > .ke-detail[data-v-bea58fa8]` / `… > .ke-actions[data-v-bea58fa8]`，**永远匹配不上真实 DOM**，是死代码。
- 于是三个子元素全靠 grid auto 放置：`.drm-thread__header` → r1c1，`.ke-detail`（存在时）→ r1c2，`.ke-actions` → r2c1。c2 是 auto 轨道被长理由文本撑宽，c1（1fr）被挤到 min-content——即用户看到的「窄条标题列 + 宽建议列 + 按钮堆左下」。
- 无 detail 的行（dismissed/普通）恰好 auto 放置成「按钮在右列」，属于碰巧没错，进一步掩盖了死代码事实。

浏览器实测证据（2026-10-16，静态托管构建与源码一致）：真实 DOM `.ke-actions` computed `grid-column: auto`；手工补齐宿主 scope 属性后 `1/-1`、`2`、`1 / span 2` 规则立即生效，证明规则本身没丢、只是选择器断链。

## What Changes

- **`KeyEventMarkerActions.vue`**：把自身两个根节点的 grid 摆放规则**移入组件自身 scoped 样式**（根节点带自家 scope 属性，规则可靠命中）——`.ke-detail` 放右半列（c2/r1），`.ke-actions` 改横排、占全宽底行（c 1/-1 / r2）、内容右对齐（= 行右下角）；≤720px 窄屏回退单列全宽（与宿主断点同步，注释锚定）。
- **`DailyReportTopicSection.vue`**：`.drm-thread-row` 改为两等宽列 `minmax(0, 1fr) minmax(0, 1fr)`（左右各 50%）；删除两段因 scope 断链永不命中的 `> .ke-detail` / `> .ke-actions` 死规则（含 720px 媒体查询里的同款死规则），留注释指明摆放规则归属子组件。
- spec `topic-key-event-markers`「条目标记交互」Requirement 的呈现句修订：AI 理由与标题/摘要左右等宽分栏、动作组（徽标+按钮）行底右对齐；交互语义（单步动作、互斥禁用、局部反馈、不嵌套 header 按钮）不变。

## Impact

- **代码**：`front/app/features/tags/components/KeyEventMarkerActions.vue`、`front/app/features/tags/components/daily-report/DailyReportTopicSection.vue`（均纯样式/模板注释，无脚本逻辑变更）。
- **行为**：日报（当日 review / 历史 history 模式）线索行的建议理由、动作按钮位置与行布局；`overview` 模式与其它宿主不受影响（本组件仅此一个宿主）。
- **无 API / 数据 / 后端变更**；不新增依赖。
- **风险**：低——纯 CSS 摆放迁移；回归面是日报线索行四种标记状态（suggested/confirmed/dismissed/无标记）的行内布局与窄屏回退。

## 验收（用户口径）

1. 带 AI 建议的线索行：左半标题+摘要（含「N 篇」计数），右半 AI 理由+依据数，各占 50%。
2. 「AI 建议」徽标、「确认为关键」「不采纳」（及其它状态的对应动作）出现在线索行**右下角**，横排。
3. 窄屏（≤720px）回退单列全宽横排，不挤压内容。
