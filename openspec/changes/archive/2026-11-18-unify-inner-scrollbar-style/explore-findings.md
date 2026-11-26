
## 滚动条样式现状与根因（Chromium 标准属性接管）

根因：`scrollbar-width`/`scrollbar-color` 是标准属性，Chromium ≥121 一旦在元素上声明就接管滚动条绘制并**忽略 `::-webkit-scrollbar`**；两者均为继承属性，但伪元素不继承。

现状三派：
1) 只写 `scrollbar-width: thin`（→ UA 默认色，dark 下露浅色原生条）：`BoardRadarView.vue` `.rd-stage`(scoped ~L250) / `.rd-rail-scroll`(~L267)；`BoardRadarReadingRail.vue` `.rd-rail`(~L138)；`SettingsTabsNav.vue` `.settings-tabs-nav__track`(~L52)；`BoardNarrowDayStrip.vue` `.bns-strip-track`(~L159)；`daily-report/DailyReportSidebar.vue` `.drm-sidebar`(~L120)；`daily-report/DailyReportDeskIndex.vue` `.ddi`(~L134)。
2) 已正确（thin + scrollbar-color: var(--color-border-strong) transparent）：`DailyReportReader.vue` `.drm-reader`(~L998) / `.drm-notes-rail__scroller`(~L1134)；`DailyReportMiniLifeline.vue` `.drm-lifeline__scroll`(~L242，另有 `::-webkit-scrollbar height:6px` + thumb `border:2px solid transparent; background-clip:padding-box` 的内缩做法 → 刻意豁免)。
3) 只用伪元素（仍是主题色）：`.content-scroll`、`.sidebar-content`、`.virtual-list`。

全局杂志条在 `front/app/assets/css/main.css` ~L505-545（`::-webkit-scrollbar` 10px + `--color-bg-sunken` 轨 + `--color-border-medium` 拇指），页面级不动。

主题 token（`[data-theme]` 块，main.css ~L106 editorial / ~L193 dark）：`--color-border-strong` = editorial rgba(26,26,26,.25) / dark rgba(255,255,255,.18)；`--color-text-muted` = #8a8a8a / rgba(255,255,255,.35)。未设置 `color-scheme`（故 UA 条随 OS 走，dark 主题下是浅色）。

模板落点：`BoardRadarView.vue` 的 `<main class="rd-stage">` 与 `<div class="rd-rail-scroll">`；`BoardRadarReadingRail.vue` 的 `<aside class="rd-rail">`；`SettingsTabsNav.vue` 的 `<div class="settings-tabs-nav__track">`；`BoardNarrowDayStrip.vue` 的 `<div class="bns-strip-track">`；`DailyReportSidebar.vue` 的 `<aside class="drm-sidebar">`；`DailyReportDeskIndex.vue` 根 `<nav class="ddi">`；`DailyReportReader.vue` 的 `<article class="drm-reader">` 与 `<div class="drm-notes-rail__scroller">`。

测试约束：vitest include `app/**/*.test.ts`，环境 happy-dom（Node fs 可用）。前端命令须按宿主平台跑——当前 Linux 宿主本机直跑；`pnpm test:unit` 参数不带 `--`。

**引用**：front/app/assets/css/main.css:scrollbar、front/app/features/tags/components/console/BoardRadarView.vue:.rd-stage、front/app/features/tags/components/console/BoardRadarReadingRail.vue:.rd-rail、front/app/features/tags/components/DailyReportReader.vue:.drm-reader

<!-- pinned 2026-11-18T05:57:41Z -->
