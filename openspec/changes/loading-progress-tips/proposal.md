<!-- complexity: simple -->
<!-- ui-impact: minor -->

## Why

初始化加载屏（app.vue 全屏 loading）在数据量大的账号上要转圈很久（feeds 列表 `per_page: 10000`），只有一个转圈 + "正在加载..."，用户感知是"卡死了"。参考游戏加载画面（拟真进度条 + 随机小短句），用可感知的进度和趣味文案缓解等待焦虑。

## What Changes

- 初始化加载屏（app.vue）：新增拟真进度条（快进到 ~90% 后等待真实完成推进到 100%）+ 百分比数字 + 游戏风小短句（随机一条，长加载时轮换）。
- SPA 首帧模板（spa-loading-template.html）：同步加自包含进度条 + 短句（bundle 下载阶段的体验对齐，内联实现、零外部依赖不变）。
- 短句风格：游戏风玩梗（与产品语境相关，如"正在唤醒语义向量…"），中英不混排；文案集中一处定义，两层各自内联/引用，避免跨层共享模块破坏首帧自包含。
- 主题适配：进度条/文字色值沿用现有主题令牌（editorial/dark 双套），首帧模板沿用内联色值约定。
- 无障碍：短句容器 `aria-live="polite"`，进度条 `role="progressbar"`；`prefers-reduced-motion` 下进度条不做动画推进。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `spa-loading-ux`: 首屏加载模板与初始化加载屏从「转圈 + 静态文案」升级为「拟真进度条 + 游戏风小短句」，两层体验一致；自包含、双主题、reduced-motion 红线延续。

## Impact

- `front/app/spa-loading-template.html`（首帧模板，内联 CSS/JS 扩展）
- `front/app/app.vue`（初始化加载屏重写 loading 分支）
- 短句文案与拟真进度逻辑为纯前端展示层，不改 API、store 数据流与初始化时序；`apiStore.initialize()` 调用方式不变。
- 文档：`docs/reference/standard/frontend/loading-experience.md` 增补进度条/短句契约节。
