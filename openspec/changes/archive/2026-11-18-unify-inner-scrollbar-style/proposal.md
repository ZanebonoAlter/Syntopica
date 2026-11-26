# Change: 内部滚动区滚动条统一为主题色细条（不再露系统原生条）

<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

用户反馈：叙事工坊首页（`/tags` 未选版块时的雷达总览）左栏列表与右栏「当天线索最多的泳道」滚动条**是系统原生的浅色条**，与站内其它内部滚动区不是一套样式（截图：dark 主题下右栏露出一条浅灰系统条，横穿整列）。

根因不是"忘了写样式"，而是**两种滚动条 API 打架**：

| 事实 | 后果 |
| --- | --- |
| `main.css` 的全局主题色滚动条写在 `::-webkit-scrollbar` 伪元素上（10px 杂志条） | 只在**没有**声明标准属性的容器上生效 |
| `scrollbar-width` / `scrollbar-color` 是标准属性，Chromium ≥121 / Firefox 一旦检测到就**接管绘制并忽略 `::-webkit-scrollbar`** | `BoardRadarView.vue` 的 `.rd-stage` / `.rd-rail-scroll` 只写了 `scrollbar-width: thin`，于是全局主题样式被整套跳过 |
| 只写 `scrollbar-width: thin` 不写 `scrollbar-color` 时，颜色由 UA / 操作系统决定 | dark 主题下拿到浅色原生条——正是截图里的样子 |
| `scrollbar-color` **继承**、`scrollbar-width` **不继承**（CSS Scrollbars L1，本机 Chromium 152 实测） | 父级挂类不会把粗细带给子滚动区，**每个滚动容器必须自己挂类** |
| 标准通道（`scrollbar-color`）**没有 hover 语法** | 「hover 变亮」只能由 `::-webkit-scrollbar-thumb:hover` 给老 Chromium 兜底，不能当现代浏览器的验收点 |

同根问题在全站 **7 处内部滚动区**重复（雷达左栏、雷达右栏容器、右栏 `aside`、设置页 tab 条、窄屏日期条、日报侧栏、日报索引组件），另有两处（`DailyReportReader` 的 `.drm-reader` / `.drm-notes-rail__scroller`）**手写了正确的 `scrollbar-color`**；还有两处活组件只走伪元素通道且**硬编码了浅色**（文章列表 `.virtual-list` 的 `rgba(26,26,26,0.1)`、全局侧栏 `.sidebar-content` 的 `--color-border-medium`），dark 主题下几乎看不见——所以站内出现"有的地方细而暗、有的地方一条浅色原生条、有的地方基本没有条"的不统一。

## What Changes

- **新增全局工具类 `.scroll-thin`（`front/app/assets/css/main.css`）作为内部滚动条样式的单一事实源**：`scrollbar-width: thin` + `scrollbar-color: var(--color-border-strong) transparent`，两条通道同时声明（标准属性给 Chromium ≥121 / Firefox，`::-webkit-scrollbar` 伪元素给老 Chromium 兜底），颜色**只取主题 token**，dark 与 editorial 同时成立。hover 变亮只保留在伪元素兜底通道，不作为现代浏览器承诺。
- **把 7 处散落的裸 `scrollbar-width: thin` 换成 `.scroll-thin`**：雷达左栏 `.rd-stage`、右栏滚动容器 `.rd-rail-scroll`、右栏 `aside.rd-rail`、`.settings-tabs-nav__track`、`.bns-strip-track`、`.drm-sidebar`、`.ddi`。
- **把 2 处手写 `scrollbar-color` 收敛到 `.scroll-thin`**：`.drm-reader`、`.drm-notes-rail__scroller`（值与工具类完全一致，视觉零变化，只去掉重复定义）。
- **把 2 处只走伪元素通道、硬编码浅色的活容器一并收编**：文章列表 `.virtual-list`（dark 主题下拇指几乎不可见，属同类缺陷）、全局侧栏 `.sidebar-content`（`--color-border-medium` + 0.375rem，与统一档不一致）。
- **删除死代码**：`.content-scroll`（`main.css` 内 4 条规则；全仓零使用点，`grep -rn content-scroll front backend-go docs scripts` 只命中定义自身）。
- **加一条机械回归测试**（`front/app/assets/css/scrollbar-style.test.ts`，26 例）：断言 `.scroll-thin` 双通道齐全、颜色取 token；11 个滚动容器的 class 属性上**挂对了元素**（不是相邻 wrapper）；迁移过的文件不再含裸 `scrollbar-*` 声明（含硬编码色值）；死代码已清；页面级杂志条未被波及；生命线豁免不回退。
- **不改**：任何布局、栏宽、内边距、交互行为；`DailyReportMiniLifeline` 的横向生命线滚动条（自带 `border: 2px solid transparent; background-clip: padding-box` 的内缩做法，视觉已统一，本轮不动）；`scrollbar-gutter: stable`（布局预留属性，非配色声明）；页面级 10px 杂志条（全局 `::-webkit-scrollbar`）保持原样。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `theme-system`：新增「内部滚动区滚动条跟随主题」要求——内部滚动区 SHALL 通过 `.scroll-thin` 用主题 token 绘制、SHALL NOT 硬编码色值或落回 UA 默认配色（含 dark 主题下露浅色原生条的反例），并固化两条实现陷阱：「标准属性会关掉同元素的 webkit 伪元素通道」「`scrollbar-color` 继承而 `scrollbar-width` 不继承，容器必须自己挂类」。

## Impact

- **前端**（12 个文件，全部 CSS/模板 class 级改动，无 TS 逻辑）：
  - `front/app/assets/css/main.css`（新增 `.scroll-thin`，删除死代码 `.content-scroll`）
  - `front/app/features/tags/components/console/BoardRadarView.vue`（`.rd-stage` / `.rd-rail-scroll`）
  - `front/app/features/tags/components/console/BoardRadarReadingRail.vue`（`.rd-rail`）
  - `front/app/features/tags/components/console/BoardNarrowDayStrip.vue`（`.bns-strip-track`）
  - `front/app/features/settings/components/SettingsTabsNav.vue`（`.settings-tabs-nav__track`）
  - `front/app/features/tags/components/daily-report/DailyReportSidebar.vue`（`.drm-sidebar`）
  - `front/app/features/tags/components/daily-report/DailyReportDeskIndex.vue`（`.ddi`；该组件当前无挂载点，本次只做样式收敛，将来复活时自动生效）
  - `front/app/features/tags/components/DailyReportReader.vue`（`.drm-reader` + `.drm-notes-rail__scroller`）
  - `front/app/features/shell/components/ArticleListPanelView.vue`（`.virtual-list`）
  - `front/app/features/shell/components/AppSidebarView.vue` + `front/app/components/layout/AppSidebar.css`（`.sidebar-content`）
  - 新增 `front/app/assets/css/scrollbar-style.test.ts`
- **后端 / 数据 / 接口**：无。无迁移、无新端点。
- **风险**：极低——只改滚动条绘制属性与宽度（`thin` 语义不变），不参与布局计算。继承边界已核实：`scrollbar-color` 继承、`scrollbar-width` 不继承，因此页面级杂志条不会被“顺带改细”；唯一可能的继承副作用是加了类的容器内子滚动区**颜色**跟随（符合意图），宽度不跟随（故每个容器单独挂类）。
- **部署后用户可见变化**：雷达首页左右两栏、设置页 tab 条、窄屏日期条、日报侧栏、文章列表、全局侧栏的滚动条从「系统浅色原生条 / 几乎看不见的深色拇指」变成同一套「暗底细条」；浅色主题下是淡墨条。无需用户做任何操作，无旧数据降级问题（纯样式，刷新即生效）。
- **旧能力变化**：删除的 `.content-scroll` 为无引用死代码（4 条规则），不影响任何页面；`DailyReportMiniLifeline` 的生命线横向条刻意不动。
