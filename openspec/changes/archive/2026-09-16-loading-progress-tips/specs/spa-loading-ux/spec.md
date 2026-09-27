## MODIFIED Requirements

### Requirement: 首屏加载模板

前端 SHALL 提供自定义 SPA 首屏加载模板（`app/spa-loading-template.html`），在 JS bundle 下载与挂载完成前渲染全屏加载动画（拟真进度条 + 百分比 + 游戏风小短句）。模板 SHALL 保持纯静态 HTML/CSS/内联 JS（不依赖任何外部资源，bundle/字体/图标均未加载时可正常渲染）。模板 SHALL 根据主题存储（localStorage `syntopica-theme`）适配 editorial 与 dark 双主题，与 `app.vue` head 内联脚本的主题判定逻辑保持一致；JS 挂载完成后模板 SHALL 被 Nuxt 应用内容替换。短句 SHALL 随机选取一条；进度 SHALL 拟真推进（缓慢爬升、不到 100%，避免在真实加载完成前谎报完成）。

#### Scenario: JS 下载期间显示加载动画
- **WHEN** 弱网环境下（如 throttle Slow 3G）打开任意路由，JS bundle 尚未下载完成
- **THEN** 页面显示全屏加载态：进度条与百分比数字拟真爬升（不到 100%），并随机显示一条游戏风小短句，而非空白或默认 Nuxt logo 模板

#### Scenario: 暗色主题不闪白
- **WHEN** 主题存储为 `dark` 的浏览器打开首屏
- **THEN** 加载模板以暗色背景渲染，从模板出现到应用挂载全程无白色背景闪现

#### Scenario: 模板无外部依赖
- **WHEN** 完全断网时打开已缓存 HTML 的首屏
- **THEN** 加载模板正常渲染（进度条、百分比、短句均可见），无任何指向外部域名的请求（字体、图标、脚本）

## ADDED Requirements

### Requirement: 初始化加载屏进度反馈

前端 SHALL 在应用初始化数据加载期间（`app.vue` 全屏 loading 态）显示拟真进度反馈：进度条与百分比数字 SHALL 缓动爬升至不超过 90% 的水位等待真实完成，初始化完成时 SHALL 推进至 100% 并卸载加载屏；初始化失败（error 态）SHALL 停止进度推进并正常展示既有错误分支。加载屏 SHALL 随机显示一条游戏风小短句，加载持续超过 4 秒时 SHALL 以约 4 秒间隔轮换短句。进度条与短句 SHALL 适配 editorial/dark 双主题；短句容器 SHALL 以 `aria-live="polite"` 播报，进度条 SHALL 带 `role="progressbar"` 与 `aria-valuenow`；`prefers-reduced-motion` 环境下 SHALL 不做动画过渡（进度跳变直接呈现）。加载屏卸载时 SHALL 清理全部计时器。

#### Scenario: 初始化期间拟真进度
- **WHEN** 应用启动、初始化数据接口尚未返回
- **THEN** 全屏加载态显示进度条与百分比，数值缓动爬升但不超过 90%，同时显示一条随机游戏风短句

#### Scenario: 初始化完成推进到 100% 并卸载
- **WHEN** `initialize()` 的全部请求完成
- **THEN** 进度推进至 100%，随后加载屏卸载并渲染正常页面，所有进度/轮换计时器被清理

#### Scenario: 初始化失败停止进度
- **WHEN** 初始化请求失败进入 error 态
- **THEN** 进度推进停止，展示既有错误分支（错误图标 + 错误信息 + 重新加载），不显示 100% 假完成

#### Scenario: 长加载轮换短句
- **WHEN** 加载持续超过 4 秒
- **THEN** 短句以约 4 秒间隔自动轮换为新的随机短句

#### Scenario: 减弱动效偏好
- **WHEN** 用户系统开启 `prefers-reduced-motion`
- **THEN** 进度数值变化直接呈现，无 CSS 过渡动画；功能（进度、短句）不受影响
