## ADDED Requirements

### Requirement: 内部滚动区滚动条跟随主题

凡页面的**内部滚动区**（`overflow` 为 `auto`/`scroll` 的栏、栏内列表、横向日期条、边注栏、文章列表等，不含页面级 `body` 滚动条）其滚动条 SHALL 通过全局工具类 `.scroll-thin` 复用同一套样式，SHALL NOT 在组件 scoped 样式里另写 `scrollbar-*` 配色/宽度声明（`scrollbar-gutter: stable` 这类布局预留属性不在此列）。

`.scroll-thin` SHALL 同时声明 `scrollbar-width: thin` 与 `scrollbar-color: var(--color-border-strong) transparent`，并 MAY 同时声明 `::-webkit-scrollbar` 系列伪元素兜底。三条机制 MUST 被一并理解，缺一条就会退回缺陷：

1. 标准属性一旦声明，Chromium（≥121）/ Firefox 会接管绘制并**忽略同元素的 `::-webkit-scrollbar`**——所以颜色 MUST 走标准属性，伪元素只是老引擎兜底；
2. `scrollbar-color` **继承**、`scrollbar-width` **不继承**（CSS Scrollbars L1）——所以**每个**滚动容器 MUST 自己挂 `.scroll-thin`，SHALL NOT 依赖父级把粗细带下去（父级只能把颜色带下去）；
3. 标准通道**没有 hover 语法**——hover 变亮 MAY 由 `::-webkit-scrollbar-thumb:hover` 提供给老 Chromium，SHALL NOT 被写成现代浏览器的可见行为或验收点。

滚动条颜色 SHALL 只来自 Layer 2 semantic token（拇指 `--color-border-strong`），SHALL NOT 硬编码色值、SHALL NOT 用与主题无关的固定浅色——否则 dark 主题下会出现浅色条或（深色硬编码时）几乎看不见的拇指。

页面级滚动条（`body` / 页面壳的 10px 「杂志条」及其 `::-webkit-scrollbar` 全局段）不在本要求范围内，SHALL NOT 因本要求改变。

#### Scenario: dark 主题下的内部滚动区

- **GIVEN** 主题为 `dark`，且某内部滚动区（如叙事工坊雷达总览左右两栏、文章列表）内容溢出
- **WHEN** 滚动条渲染出来
- **THEN** 该容器的 computed `scrollbar-width` SHALL 为 `thin`
- **AND** computed `scrollbar-color` SHALL 为 `rgba(255, 255, 255, 0.18)`（`--color-border-strong` 的 dark 值）与透明轨道，SHALL NOT 为系统浅色原生条

#### Scenario: editorial 主题下的内部滚动区

- **GIVEN** 主题为 `editorial`
- **WHEN** 同一内部滚动区内容溢出
- **THEN** computed `scrollbar-color` SHALL 为 `rgba(26, 26, 26, 0.25)`（`--color-border-strong` 的 editorial 值）
- **AND** 细条语义与 dark 主题一致（同档 `thin`）

#### Scenario: 单一事实源与「类挂在滚动容器本身」

- **GIVEN** 仓库内任一内部滚动区容器
- **WHEN** 检查其滚动条声明
- **THEN** 该容器的 class 属性 SHALL 含 `.scroll-thin`（挂在滚动容器**本身**，不是相邻 wrapper）
- **AND** 该文件 SHALL NOT 残留 `scrollbar-*` 配色/宽度声明（生命线 `.drm-lifeline__scroll` 为显式豁免）
- **AND** `.scroll-thin` SHALL 同时包含标准属性与 `::-webkit-scrollbar` 兜底（回归测试机械断言）

#### Scenario: 页面级滚动条不受影响

- **WHEN** 应用任意页面整体滚动
- **THEN** 页面级滚动条 SHALL 保持既有 10px 杂志条样式（`--color-bg-sunken` 轨 + `--color-border-medium` 拇指）
- **AND** `html` 的 computed `scrollbar-color` SHALL 保持 `auto`（内部工具类的颜色继承不得从 `html` 起算）
