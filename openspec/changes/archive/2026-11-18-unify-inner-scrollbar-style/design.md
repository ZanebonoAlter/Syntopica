## Context

`front/app/assets/css/main.css` 里已有一套「杂志风」滚动条（`::-webkit-scrollbar` 10px + `--color-bg-sunken` 轨 + `--color-border-medium` 拇指 + `.content-scroll` 6px 变体）。组件层另有 9 处各自声明滚动条，写法分裂成三类：

| 写法 | 位置 | 真实渲染（Chromium 当代版本） |
| --- | --- | --- |
| 只写 `scrollbar-width: thin` | `.rd-stage`、`.rd-rail-scroll`、`.rd-rail`、`.settings-tabs-nav__track`、`.bns-strip-track`、`.drm-sidebar`、`.ddi` | **标准属性接管 → 全局伪元素样式失效 → UA 默认配色（dark 下浅色条）** |
| `scrollbar-width` + `scrollbar-color` | `.drm-reader`、`.drm-notes-rail__scroller`、`.drm-lifeline__scroll` | 主题色细条（正确形态） |
| 只用伪元素 | `.virtual-list`（硬编码 `rgba(26,26,26,0.1)`）、`.sidebar-content`（`--color-border-medium` + 0.375rem）、`.content-scroll`（硬编码，**全仓无使用点**） | 主题色但深浅不一：`.virtual-list` 在 dark 底上几乎看不见；`.content-scroll` 是死代码 |

触发点是用户看到的雷达首页右栏浅色条：它属于第一类。

关键机制（已核实——本机 Chromium 152 实测 + CSS Scrollbars L1）：

1. `scrollbar-width` / `scrollbar-color` 自 Chromium 121 起稳定，**声明后 `::-webkit-scrollbar` 对该元素不再生效**；
2. **`scrollbar-color` 继承、`scrollbar-width` 不继承**（实测：父级 `thin` → 子级 computed `auto`；父级 `red blue` → 子级继承同值）；
3. 标准通道**没有 hover 语法**（`scrollbar-color` 取值只有 `auto | <color>{2}`）。

这三条直接决定了下面五个决策，也决定了「父级挂类、子元素自动变细」这条路走不通。

## Decisions

### D1 用工具类 `.scroll-thin` 收敛，不再逐组件手写

`main.css` 顶层新增：

```css
.scroll-thin {
  scrollbar-width: thin;
  scrollbar-color: var(--color-border-strong) transparent;
}
.scroll-thin::-webkit-scrollbar { width: 6px; height: 6px; }
.scroll-thin::-webkit-scrollbar-track { background: transparent; }
.scroll-thin::-webkit-scrollbar-thumb { background: var(--color-border-strong); border-radius: 3px; }
.scroll-thin::-webkit-scrollbar-thumb:hover { background: var(--color-text-muted); }
```

- **落点**：`front/app/assets/css/main.css` 的 `.scroll-thin` 块；验收落到 Scenario「内部滚动区滚动条跟随主题」与回归测试断言。
- **取舍**：被否方案——保持手写、逐处补 `scrollbar-color`。否决理由：颜色值会散落在 9 个 `.vue` 的 scoped 块里，下次改视觉要改 9 处，且现任 3 种写法已证明会继续漂移。
- **取舍**：被否方案——把标准属性放到 `html` 全局（“一次解决全部”）。否决理由：`scrollbar-color` 会**继承给全站每个滚动区**（包括页面级 10px 杂志条），同时让全局 `::-webkit-scrollbar` 在全站失效——杂志条会被改色、平台控件观感一并变，是未验证的全站视觉变更；而 `scrollbar-width` 又**不继承**，写了也解决不了子容器的粗细。

### D2 标准属性与 webkit 伪元素**双写**（不是二选一）

- **落点**：`.scroll-thin` 块内两组声明；回归测试 `scrollbar-style.test.ts` 同时断言两组存在。
- **取舍**：被否方案——只留 `::-webkit-scrollbar`（Firefox 会退回 12px 原生条，且颜色不受控）。否决理由：Firefox 无伪元素通道，只写伪元素等于放弃 Firefox 的主题色。
- **取舍**：被否方案——只留标准属性。否决理由：Chromium <121（老 Edge/内嵌 WebView）会退回 UA 默认条，正是本 change 要消灭的「原生条」。

### D3 颜色取 `--color-border-strong`（拇指）/ `--color-text-muted`（仅老 Chromium hover 兜底），轨道透明

- **落点**：`.scroll-thin` 的颜色声明与 `::-webkit-scrollbar-thumb:hover`；与既有正确写法（`.drm-reader`）一致，故那两处可无损并入工具类。
- **取舍**：被否方案——用 `--color-border-medium`。否决理由：dark 下 alpha 0.10 在 `#080c12` 底上几乎不可见（滚动条要“看得出在哪”），0.18 才够；editorial 下 0.25 的淡墨条也符合报纸风格。
- **取舍**：被否方案——给轨道加底色（像页面级杂志条）。否决理由：内部滚动区贴着内容，带底色会切出一条竖带，反而显脏。
- **已知限制**：标准 `scrollbar-color` 没有 hover 语法，而声明标准属性后伪元素被忽略——所以 **hover 变亮只在老 Chromium（<121）与伪元素兜底通道生效**，不得写进验收点（原稿把 hover 当主通道承诺是错的，已由 review R-3 纠正）。

### D4 不动 `color-scheme`，不动 `DailyReportMiniLifeline`，不动页面级 10px 条

- **落点**：无代码改动，显式记在 ui-design「不受影响」表；`DailyReportMiniLifeline.vue` 保持 `scrollbar-color` + 内缩伪元素（`border: 2px solid transparent; background-clip: padding-box`）的现有做法。
- **取舍**：被否方案——在 `[data-theme="dark"]` 上加 `color-scheme: dark`。否决理由：会连带改变原生表单控件（select/date picker/checkbox）外观，属于未评估的全局视觉变更，与本 change 的范围不符。
- **取舍**：被否方案——把生命线横向条也并进 `.scroll-thin`。否决理由：它的内缩 2px 边框是刻意的（横向条更细更贴轨道），并入会变粗；视觉上已统一，无收益。

### D5 老伪元素通道的活容器一并收编，死代码删除

`.virtual-list`（文章列表）与 `.sidebar-content`（全局侧栏）属于第三类：只用伪元素、且颜色是硬编码或弱化 token，在 dark 主题下拇指几乎不可见；`.content-scroll` 则是**零使用点**的死代码。

- **落点**：`ArticleListPanelView.vue` 的 `.virtual-list`、`AppSidebarView.vue` 的 `.sidebar-content`（样式在 `AppSidebar.css`）各挂 `scroll-thin` 并删掉 4 段伪元素规则；`main.css` 删除 `.content-scroll` 整块。
- **取舍**：被否方案——不改它们，只把 delta spec 收窄到“本 change 覆盖的容器”。否决理由：`.virtual-list` 是阅读页主列表，dark 主题下“看不见滚动条”正是用户本次反馈的同类缺陷；留着一半等于下次再开一个 change。
- **取舍**：被否方案——删除 `.content-scroll` 的同时改成 `.scroll-thin` 别名保留。否决理由：无使用点即无兼容包袱，留着只会让下一个 candidate 继续抄它。

## Risks

- **WebKit 语义变更风险**：`.scroll-thin` 声明标准属性后，该元素上任何自定义 `::-webkit-scrollbar` 规则都将失效（包括老代码）。本次涉及的容器里，只有 `DailyReportMiniLifeline` 有自定义伪元素规则——它**不加类**，故不受影响；`.virtual-list` / `.sidebar-content` 的自定义伪元素已随迁移删除。
- **继承边界**：`scrollbar-color` 继承 → 加类的容器内所有子滚动区**颜色**跟随（符合意图，dark 主题不会再冒浅色条）；`scrollbar-width` **不继承** → 宽度必须每个滚动容器自己挂类，未来新增内部滚动区时不要指望父级带下去。
- **可回退**：纯 CSS，回退只需删 `.scroll-thin` 的 `scrollbar-color` 行（恢复到当前“原生 thin 条”），无数据影响。
