# add-search-sort-modes UI 验收报告（ui-impact: minor）

- 验收时间：2026-10-23（agent-browser headless Chromium，静态托管 `http://localhost:5100`）
- 验收范围：搜索页过滤行「最新优先 / 相关度优先」分段控件；默认时间优先首屏；相关度切换；双主题；窄屏降级
- 断言工具：agent-browser session `sort-static-6d5795ec8cb0`；组件层证据见 `front/app/pages/search.test.ts`（16 tests）与 `front/app/features/search/composables/useSearch.test.ts`（17 tests）

## 判定条目

### A-1 默认「最新优先」与实时新闻首屏【pass】
默认 `sortBy=date`，控件「最新优先」active（`aria-pressed=true`）；搜索「英伟达」返回「共 1146 条结果」，首屏卡片为 16 小时前/13 小时前/1 小时前等当日文章，与改动前「首屏全是 8~9 月」形成对照。证据：`desktop-1440x900-editorial.png`。

### A-2 切「相关度优先」立即重查【pass】
点击后 `aria-pressed` 变为 date=false / relevance=true，active class 切到「相关度优先」；同一查询重查后首屏回到 1 个月前的老文章（旧 RRF 顺序），摘要耗时 3.01s。证据：`desktop-1440x900-relevance.png`。

### A-3 dark 主题零特判【pass】
切 dark 后页面/顶栏/结果卡片/分段控件均随语义 token 切换，无白底实块或未适配区块；「最新优先」active 态使用 `--color-accent-subtle` 在深色背景下仍可辨识。证据：`desktop-1440x900-dark.png`。

### A-4 375×667 窄屏降级【pass】
过滤行改为纵向堆叠，排序分段独占一行；容器无横向溢出（截图右缘完整），结果卡片正常折行。editorial 与 dark 各一张：`mobile-375x667-editorial.png`、`mobile-375x667-dark.png`。

### A-5 loading 期间排序控件 disabled【pass】
组件测试 `front/app/pages/search.test.ts`「首查 loading 期间排序按钮 disabled，响应落地后解禁」断言两个按钮 `disabled` 属性随 loading 出现/消失；视觉瞬态截图不稳定，未单独截屏。

## 证据文件清单（ui-verification/）

- `desktop-1440x900-editorial.png` —— 默认时间优先首屏（editorial）
- `desktop-1440x900-relevance.png` —— 相关度优先切换后（editorial）
- `desktop-1440x900-dark.png` —— 默认时间优先首屏（dark）
- `mobile-375x667-editorial.png` —— 窄屏默认排序（editorial）
- `mobile-375x667-dark.png` —— 窄屏默认排序（dark）
