# UI 验证报告 — collapse-margin-notes-rail（minor）

<table>
<tr><td>验证日期</td><td>2026-11-19</td></tr>
<tr><td>形态</td><td>静态托管 <code>http://localhost:5100</code>（<code>bash scripts/dev/deploy-frontend.sh --no-restart</code> 产物）</td></tr>
<tr><td>工具</td><td>agent-browser（headless Chromium，CDP `getBoundingClientRect` + 截图）</td></tr>
<tr><td>页面</td><td><code>/tags/1974/2026-11-18</code>（中东版块，24 篇 / 10 话题）</td></tr>
</table>

## 1. 桌面默认收起（1440×900，editorial 主题）

| 断言 | 实测 |
| --- | --- |
| 浮标存在、面板不渲染 | `[data-testid=drm-notes-tab]` = true，`.drm-notes-rail` = false |
| 浮标贴视口右边缘、垂直居中 | `rect.x=1410`（宽 30 → 右缘 1440），`centerY=450` = `innerHeight 900 / 2` |
| 网格退两列 | `gridTemplateColumns = 224px 1057.62px` |
| 无偏好记录 = 收起 | `localStorage['syntopica.daily-report.margin-notes-collapsed'] = null` |
| 可访问名带计数/引导 | `aria-label = "展开页边注 · 划选正文文字可添加批注"`（0 条时） |

> 修复前对照：同一浮标在 `.drm-peel-host`（`perspective`）子树内时 `top=1774.55px`、`rect.y≈1776`（视口高 633），即被摆到整页 50% 高度、首屏不可见（review R-1）。现浮标与窄屏 FAB 均渲染在 `.drm-reader` 直接子级，`position: fixed` 的包含块回到视口。

## 2. 显式展开 / 收起与偏好（1440×900）

| 操作 | 实测 |
| --- | --- |
| 点浮标 | `.drm-notes-rail` = true、浮标消失、`gridTemplateColumns = 224px 769.64px 244.80px`、`localStorage='0'`；栏头收起按钮 `aria-expanded="true"` |
| 刷新页面 | `.drm-notes-rail` = true、浮标 = false（偏好被记住） |
| 点栏头 `→` | 面板卸载、浮标回归、`gridTemplateColumns = 224px 1057.62px`、`localStorage='1'` |

## 3. 暗色主题（1440×900）

| 断言 | 实测 |
| --- | --- |
| 主题生效 | `documentElement.dataset.theme = 'dark'` |
| 浮标底色 / 文字 | `background = rgba(17, 27, 38, 0.98)`、`color = rgba(255, 255, 255, 0.6)`（secondary，替代 muted 的 0.35） |
| 计数徽标 | `background = rgba(240, 138, 75, 0.85)`、文字白色 |
| 两列布局同桌面 | `224px 1057.62px` |

## 4. 窄屏回归（960×800）

| 断言 | 实测 |
| --- | --- |
| 不出现桌面浮标 / 第三列 | 浮标 false、`.drm-notes-rail` false |
| 单列 | `gridTemplateColumns = 883.22px` |
| 右下 FAB 条件 | 当前报告 0 条批注 → FAB false（既有 `annotations.length` 条件不变） |

## 5. 组件回归

`cd front && pnpm test:unit app/features/tags/components/DailyReportReader.test.ts app/features/tags/components/daily-report/MarginNotesRail.test.ts --maxWorkers=2` → **30 passed / 2 files**（含默认收起、双向切换与偏好落盘、`localStorage='0'` 初始展开、程序性展开不写偏好、窄屏分支、`aria-expanded`）。

## 未覆盖 / 说明

- 「新建批注落锚后自动展开并聚焦输入框」未在真机走通：需要真实划词 + LLM 问答，会向真实批注表写入测试数据。该路径由 `focusAnnotation`（与窄屏开抽屉同一出口）承接，并有「程序性展开不写偏好」的深链用例覆盖同一函数；实现层风险见 `review.md` R-1 之后的 DOM 时序核对（`railRef` 在 `nextTick` 后必已就位）。
- 截图为本机会话产物（`/tmp/final-collapsed.png`、`/tmp/final-dark2.png`），不入库；上表数值可由同样命令复现。
