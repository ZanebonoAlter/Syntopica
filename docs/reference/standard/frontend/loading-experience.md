# SPA 加载体验（Loading Experience）

> **权威源**：本文件是 SPA 首屏加载/路由切换/加载失败兜底的唯一权威（change `spa-loading-ux` 建立）。
> 互补：`theming.md`（主题令牌，模板色值的取值来源）；`layout.md`（页面布局契约，本文件的全屏 fallback 不属于 page shell 四模式）。

doc-impact-applies: front/app/spa-loading-template.html, front/app/app.vue, front/app/composables/useFakeProgress.ts, front/app/error.vue, front/app/plugins/chunk-error-fallback.ts, front/app/assets/css/main.css, front/nuxt.config.ts | section=全部

## 四层加载体验契约

| 阶段 | 实现 | 行为 |
|------|------|------|
| 首屏（JS 挂载前） | `app/spa-loading-template.html` | 纯静态内联 CSS/spinner + 拟真进度条/百分比 + 随机游戏风短句，Nuxt 自动注入入口 HTML；JS 挂载后被应用替换；`<style>` 内含 `html` 级双主题背景色（主题判定脚本先于样式块执行），pre-FCP 空窗即为主题底色不闪白（fix-spa-nav-loading-ux D2） |
| 初始化数据加载 | `app.vue` loading 分支 + `composables/useFakeProgress.ts` | 拟真进度条（爬升≤90%，完成 100% 停留 250ms 后卸载）+ 随机短句（≥4s 每 4s 轮换）；失败停推进切错误屏 |
| 路由切换（含懒加载 chunk） | `app.vue` 的 `NuxtLoadingIndicator` + `plugins/nav-loading.ts` 驱动的 `NavLoadingOverlay` | 顶部进度条（`color="var(--color-accent)"` 随主题）+ 导航超 250ms 未完成的居中轻量 spinner（`pointer-events:none`/`z-30`/`role=status`，完成/失败/被新导航取代即消失，快导航不出现；状态机锚点 `composables/useNavLoading.ts`，fix-spa-nav-loading-ux D1） |
| chunk 失败（瞬时） | Nuxt 内建 `nuxt:chunk-reload` 插件 | 自动整页自愈 reload（10s TTL 防循环），用户无感 |
| chunk 失败（持续）/路由 404/未捕获错误 | `app/error.vue` + `app/plugins/chunk-error-fallback.ts` | 全屏兜底页（居中/`mdi:alert-circle`/重试按钮），chunk 类提示网络问题并整页 reload，应用内错误 `clearError({ redirect: '/' })` |

## 红线

- **首屏渲染路径禁止外链资源**：字体/图标一律自托管（字体 `@fontsource/noto-serif-sc` 于 `main.css` 顶部 import；图标 iconify 本地子集见 `ui-icon-localization` spec）。新增任何 `fonts.googleapis.com` 类外链 stylesheet 视为回归。
- **改主题令牌必须同步加载模板**：`spa-loading-template.html` 在 CSS bundle 之前渲染，内联复制了 editorial/dark 的 bg/text/accent/track 色值（文件内有令牌出处注释）。改 `main.css` 主题色时**必须**同步模板内联值，否则挂载前视觉与应用不一致。
- **字体回退栈必须保留系统 serif**：`"Noto Serif SC", Georgia, "Songti SC", serif`——字体分片加载失败/离线时文本渲染不被阻塞（分片按 unicode-range 按需加载，无文本消费则不拉取属正常行为）。
- **兜底页视觉复用 app.vue 错误态**：全屏居中单列结构，不改成其他形态；局部可恢复错误仍走 `AiHealthBanner`/`NotifyContainer`，不得升级为全屏兜底。

## 拟真进度与游戏风短句契约（loading-progress-tips）

- **两层同构、双份内联**：首帧模板（bundle 加载前运行，不能 import 模块）与 `useFakeProgress.ts` 各自持有一份爬升参数与短句清单的副本；**改参数或改短句 MUST 两处同步**（两处头部注释互指）。短句为游戏风玩梗、与产品语境相关，纯前端静态清单，不做后台配置。
- **拟真曲线约定**：ease-out 爬升（tick 120ms、系数 0.06）、天花板 90%——真实完成前不谎报 100%；`finish()` 收尾保留最小展示 400ms，100% 完成态停留 250ms 再卸载加载屏（防进度条闪跳）；初始化失败停推进直接切错误分支，不假完成。
- **无障碍**：进度条 `role="progressbar"` + `aria-valuemin/max/now`；短句容器 `aria-live="polite"`；`prefers-reduced-motion` 下进度宽度变化无过渡动画（功能不降级）；两层行为一致。
- **测试锚点**：进度曲线/收尾/计时器清理逻辑在 `useFakeProgress.test.ts`（fake timers）；改短句或曲线跑该文件即可，全量验证仍走 §11 门禁。

## chunk 持续失败兜底机制（维护要点）

`plugins/chunk-error-fallback.ts`：同一路径 30s 窗口内第二次 `app:chunkError`（即 Nuxt 自愈 reload 无效）时，先推后 `nuxt:reload` sessionStorage TTL 阻止再次整页刷新，再 `showError` 进兜底页。调整窗口/触发策略时同步更新 `chunk-error-fallback.test.ts`（决策函数 `shouldFallbackToErrorPage` 纯函数锚点）。

## 验收证据惯例

弱网类验证用「entry JS 阻断/改名」模拟极限形态（等价于下载期间任意时刻），headless Chromium 断言 computed style + 截图；证据归档至 change 目录 `evidence/`（参照 `spa-loading-ux`）。
