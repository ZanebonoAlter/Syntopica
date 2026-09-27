<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- 入口不变：两层加载屏都在既有入口上增强——
  1. SPA 首帧模板 `front/app/spa-loading-template.html`（HTML 解析即显示，bundle 挂载后被替换）；
  2. 初始化加载屏 `front/app/app.vue` 的 `v-if="loading"` 分支（`apiStore.initialize()` 期间）。
- 变更仅限加载屏内部视觉：转圈 + 静态文案 → 进度条 + 百分比 + 游戏风小短句。不新增页面、面板、导航。

## 受影响状态

- **loading**：两层加载屏视觉升级（本次唯一受影响状态）。进度拟真推进（0→90% 快进缓动，真实完成后→100%）；短句随机一条，加载超过 4s 时每 4s 轮换一次。
- **error**：app.vue 错误分支保持现状不变（错误屏不加短句，保持严肃语境）。
- **empty / success**：不涉及（加载完成即卸载加载屏，无残留 UI）。

## 复用组件与布局模式

- 复用主题令牌：`--color-accent` / `--color-text-secondary` / `--color-bg-base`；首帧模板沿用其内联色值约定（与 main.css 同源，双主题 editorial/dark）。
- 复用既有全屏居中 fallback 布局（loading-experience.md：全屏 fallback 不属于 page shell 四模式，无 layout mode 变更）。
- 复用 `mdi:loading` 转圈图标（app.vue 侧）与既有 spinner CSS（首帧模板侧）；新增的进度条为纯 CSS div，不引入新依赖组件。
- 不新增自由宽度容器：进度条宽度固定 ~280px 居中，窄视口 `min(280px, 60vw)`。

## 验收映射

- **组件/单测**：app.vue 加载屏的进度推进逻辑（拟真缓动 + 完成跳 100% + 卸载清计时器）抽为可测纯函数/组合式，配 vitest 用例。
- **人工验证**：dev server 下节流 Slow 3G 观察——首帧模板显示进度条与短句；初始化屏百分比递增、≥4s 短句轮换、完成后加载屏卸载无残留；editorial/dark 双主题目视色值正确。
- **无障碍**：进度条 `role="progressbar"` + `aria-valuenow`；短句容器 `aria-live="polite"`；`prefers-reduced-motion` 下禁用过渡动画。
