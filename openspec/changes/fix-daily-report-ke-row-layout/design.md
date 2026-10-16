# Design — fix-daily-report-ke-row-layout

## 根因（浏览器实测坐实，2026-10-16）

`KeyEventMarkerActions.vue` 是多根 fragment 组件（模板根为 `.ke-detail` + `.ke-actions` 两个 div）。Vue 3 只给**单根**组件的根节点继承宿主的 scoped 属性；多根组件根节点只带自家 scope 属性。实测真实 DOM：

```
<div data-v-0ad49927="" class="ke-actions ke-actions--review" data-thread-id="14170" ...>
```

只有 `data-v-0ad49927`（组件自身），没有宿主 `DailyReportTopicSection` 的 `data-v-bea58fa8`。而宿主里的摆放规则编译后是：

```css
.drm-thread-row[data-v-bea58fa8] > .ke-detail[data-v-bea58fa8] { grid-column: 1/-1 }
.drm-thread-row[data-v-bea58fa8] > .ke-actions[data-v-bea58fa8] { grid-column: 2; grid-row: 1/span 2 }
```

**永不匹配 → 死代码**。于是三元素靠 grid auto 放置：header → r1c1、`.ke-detail`（存在时）→ r1c2、`.ke-actions` → r2c1。c2 为 auto 轨道，被长理由 max-content 撑宽，c1（1fr）被挤到 min-content ≈ 18%——正是用户截图的「窄标题列 + 宽建议列 + 按钮堆左下」。无 detail 的行 auto 放置恰好「按钮在右列」，纯属巧合，掩盖了死代码。

手工给假节点补齐双 scope 属性后宿主规则立即生效（computed gridColumn 从 `auto` 变 `1/-1`/`2`），证明规则内容没丢、仅选择器断链。部署静态构建（backend-go/frontend/_nuxt/BoardDailyReportTimeline.CUFauWWA.css + Iph3Phlw.js）与当前源码一致，排除「部署旧版」假设。

## 方案

### 摆放规则归属：子组件自身 scoped 样式（而非修 scope 继承）

备选：
- A. 子组件改单根 wrapper → 宿主 scope 属性恢复继承，宿主规则复活。**弃**：改 DOM 结构（`>` 子选择器失效需连带改宿主规则），且 wrapper 层要复刻两列布局，改动面更大。
- B. 宿主样式用 `:global()`。**弃**：全局泄漏风险，违背组件化摆放归属。
- C. **摆放规则移入子组件自身 scoped 样式**（根节点带自家 attr，必然命中）。**采纳**：改动最小、归属最合理（组件自己管自己的摆放）、并配套源码守卫测试防复发。

### 目标布局（用户口径：左右各 50% + 按钮右下角）

```
┌───────────────────┬───────────────────┐
│ 标题        [N 篇⌄] │ ┃ AI 理由（pre-line）│   r1: header(c1) 50% ｜ ke-detail(c2) 50%
│ 摘要…              │ ┃ 依据 N 篇文章      │
├───────────────────┴───────────────────┤
│              [AI建议] [确认为关键] [不采纳] │   r2: ke-actions(c 1/-1) 右对齐横排
└───────────────────────────────────────┘
```

宿主 `DailyReportTopicSection.vue`：
- `.drm-thread-row` → `grid-template-columns: minmax(0, 1fr) minmax(0, 1fr)`（等宽两列）。
- 删除死规则 `> .ke-detail` / `> .ke-actions`（含 720px 媒体查询内同款），留注释指路子组件。
- `> .drm-thread__header { grid-column: 1 }` 保留（header 编译在宿主模板内，scope 属性齐全，有效）。

子组件 `KeyEventMarkerActions.vue`（自身 scoped）：
- `.ke-detail { grid-column: 2; grid-row: 1; }`——右半列顶对齐；内部 flex column、`overflow-wrap: anywhere` 不变。
- `.ke-actions`——改横排全宽底行：`grid-column: 1 / -1; grid-row: 2; flex-direction: row; flex-wrap: wrap; justify-content: flex-end; align-items: center; max-width: none;`（去掉纵排的 min/max-width）。徽标 `align-self: center` 天然垂直居中。
- 媒体查询 ≤720px（与宿主断点数值同步，注释互指锚定）：`.ke-detail { grid-column: 1/-1; grid-row: 2 } .ke-actions { grid-row: 3 }`——窄屏宿主已回退单列。

### 状态矩阵下的布局行为

- 无 detail（dismissed/无标记/confirmed 无错误）：`.ke-actions` 显式落 r2 全宽右对齐，行底出现一行动作——所有状态动作位置一致（右下角），符合用户直觉。
- error/note 只有 detail 行：右半列显示，动作组仍 r2。
- overview 模式：渲染的取消入口同样走横排右下，与现状语义一致。

## 测试设计（⓪ 契约变更处置）

旧资产 `KeyEventMarkerActions.test.ts`：
- 「splits detail from the action column」用例：断言 detail/actions 兄弟块分离、按钮不嵌套、理由不在按钮块——**新布局下仍全部成立，断言继承**；用例内描述旧分区故事的注释改写为新分区（右半列+底行右对齐）。
- 其余用例（徽标/理由/证据、busy 互斥、error 局部、overview 不泄漏）不受影响，跑绿即可。

新增守卫（防 scope 断链复发，参照 `UpgradeSuggestionPanel.gen.test.ts` readFileSync 先例）：
- 断言 `KeyEventMarkerActions.vue` `<style>` 含 `.ke-detail`/`.ke-actions` 的 grid 摆放声明（`grid-column: 2`、`justify-content: flex-end`）。
- 断言宿主 `DailyReportTopicSection.vue` `<style>` **不含** `.drm-thread-row > .ke-detail` / `> .ke-actions` 选择器（死代码回归闸）。

端到端（agent-browser）：suggested 态 DOM 复现走查 + 超长理由 + 窄屏回退，结果建账 `ui-verification/report.md`。

## 影响面与不做的事

- 只动两个 .vue 的 `<style>`（宿主另有注释增删），不动 `<script>`/模板结构/API/后端。
- 不顺手重构其它 fragment 组件的同类隐患（记入 flow 溯源备忘，另行立项）。
- 测试只跑受影响文件（树莓派负载纪律）。
