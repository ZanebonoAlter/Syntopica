<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约 · unify-inner-scrollbar-style

## 入口与入口变更

**入口不变**：`/tags`（叙事工坊雷达总览）、阅读页文章列表与全局侧栏、`/settings`（数据源 / 运行时两个分段里的 tab 条）、泳道工作面的窄屏日期条、日报阅读台（侧栏 + 阅读区 + 边注栏）——所有被触及的滚动区**位置、宽度、出现时机全部不变**，只改滚动条本身的绘制配色与粗细（由 UA 默认 / 硬编码浅色 → `thin` + 主题 token）。

不改任何按钮、文案、栏宽、内边距、路由。

## 受影响状态

| 状态 / 区域 | 是否受影响 | 说明 |
| --- | --- | --- |
| 雷达总览 左栏 `.rd-stage`（内容溢出时） | **是** | 从 UA 默认浅色条 → 6px 透明轨 + 主题色拇指 |
| 雷达总览 右栏 `.rd-rail-scroll` + 内层 `aside.rd-rail` | **是** | 用户反馈的那条浅色条；两处各自挂类（宽度不继承） |
| 阅读页文章列表 `.virtual-list` | **是** | 原硬编码 `rgba(26,26,26,0.1)` 拇指在 dark 底上几乎不可见 → 改为主题色（**这是可见改善**） |
| 全局侧栏 `.sidebar-content` | 是（轻微） | 原 `--color-border-medium` + 0.375rem → 统一 `--color-border-strong` + `thin` |
| 设置页 tab 条（横向溢出时） | 是（轻微） | 横向细条，同样主题色 |
| 窄屏日期条 `.bns-strip-track` | 是（轻微） | 横向细条 |
| 日报侧栏 `.drm-sidebar` | 是（轻微） | 纵向细条 |
| 日报索引 `.ddi` | 是（样式收敛） | 组件当前**无挂载点**，用户眼下看不到差异；将来复活时自动生效 |
| 日报阅读区 `.drm-reader` / 边注栏 `.drm-notes-rail__scroller` | 否（视觉零变化） | 原本已是 `--color-border-strong` 细条，本次只是收敛到工具类 |
| 生命线横向条 `.drm-lifeline__scroll` | 否 | 刻意保留内缩做法（design D4） |
| 页面级滚动条（body / 页面壳） | 否 | 全局 10px 杂志条保持原样（继承可行性已核实：`scrollbar-width` 不继承，不会被动改细） |
| hover 态 | **不作承诺** | 标准 `scrollbar-color` 没有 hover 语法，现代 Chromium 又不走伪元素——hover 变亮只在老 Chromium 兜底通道生效 |
| loading / empty / error / 窄屏布局 | 否 | 不参与布局计算，滚动区出现条件不变 |

## 复用组件与布局模式

- **复用**：不新建组件、不改 `AppPageShell` / `AppDialog` 任何档位，不改 `standard/frontend/layout.md` 已登记的布局模式（雷达总览仍为 `workspace`，日报阅读台仍为其既有模式）。
- **样式落点**：全局工具类 `.scroll-thin`（`main.css`），组件只加 class，不在 scoped 块里再写颜色或宽度。
- **契约**：滚动条颜色**只能**来自 Layer 2 semantic token（`--color-border-strong`），禁止硬编码色值——这是 `theme-system` 里「不使用 Tailwind `dark:` 前缀 / 组件不直接用 primitive token」的延伸。宽度不会继承，所以**每个滚动容器都要自己挂类**。

## 验收映射

| 验收点 | 落点（真实路径） |
| --- | --- |
| `.scroll-thin` 同时含标准属性与 webkit 兜底，颜色只取 token | `front/app/assets/css/scrollbar-style.test.ts` |
| 11 个滚动容器各自把 `scroll-thin` 挂在**自己的** class 属性上（非相邻 wrapper） | 同上（静态 class token 精确比对） |
| 迁移过的文件不再残留裸 `scrollbar-*` 声明（含硬编码色值） | 同上 |
| 死代码 `.content-scroll` 已清、页面级 10px 杂志条段未被波及 | 同上 |
| dark 主题下雷达左右两栏 computed `scrollbar-color` = `rgba(255,255,255,0.18)` + 透明轨 | agent-browser 实测（`localhost:5100/tags`，Chromium） |
| editorial 主题下同位置 = `rgba(26,26,26,0.25)` | agent-browser 实测（切主题后 reload） |
| `html` 的 computed `scrollbar-color` 仍为 `auto`（页面级不受继承影响） | agent-browser 实测 |
| 无布局位移（栏宽 / 内容起点不变） | `pnpm build` 通过 + 截图与改动前同视口对比 |

不需 major 原型：`ui-impact: minor`，只改既有容器的绘制属性，无新组件、无新交互。
