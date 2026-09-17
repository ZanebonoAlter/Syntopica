# spa-loading-ux Specification

## Purpose

SPA 前端加载体验契约：覆盖首屏资源加载（JS 挂载前）、外链字体、路由切换、加载失败四个阶段的用户可感知行为，保证弱网/远程访问场景下任意阶段均有视觉反馈而非白屏。

## Requirements

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

### Requirement: 首屏 pre-FCP 背景色

SPA 首屏 HTML 文档 SHALL 在 first paint 前为根元素预设与主题匹配的背景色（editorial 浅色 / dark 深色），使 HTML 到达后到加载模板/应用首帧渲染之间的空窗不呈现浏览器默认白底。背景色 SHALL 与主题判定内联脚本同序生效（data-theme 属性先于首帧设置），且 SHALL 与正式应用主题令牌色值一致（同源维护）。

#### Scenario: 深色主题刷新无白闪

- **WHEN** 主题存储为 dark 的浏览器硬刷新任意路由，且高负载下 first paint 延迟数百毫秒
- **THEN** 该空窗期页面背景为深色（与 dark 主题背景色一致），全程无白色背景闪现

#### Scenario: 浅色主题背景一致

- **WHEN** 主题存储为 editorial（或未设置）的浏览器打开首屏
- **THEN** pre-FCP 背景为 editorial 主题背景色，与后续应用背景无缝衔接

### Requirement: 字体资源不阻塞首屏渲染

首屏渲染路径 SHALL NOT 包含渲染阻塞的外链字体样式（fonts.googleapis.com 等）；产品字体（Noto Serif SC）SHALL 以自托管或等效非阻塞方式提供，字体加载失败或离线时 SHALL 回退系统字体栈，不阻塞页面渲染与加载模板显示。

#### Scenario: 无外链字体样式请求
- **WHEN** 打开首屏并观察网络请求
- **THEN** 不存在指向 fonts.googleapis.com（或任何外部字体域）的渲染阻塞 stylesheet 请求

#### Scenario: 字体加载失败回退
- **WHEN** 自托管字体资源请求失败或离线
- **THEN** 文本以系统字体栈渲染，页面功能不受影响

### Requirement: 路由切换加载反馈

前端 SHALL 在路由切换（含页面 chunk 懒加载）期间提供可见加载反馈，反馈分两层：

1. 顶部进度条（既有）：路由切换期间显示，颜色适配当前主题；
2. 导航加载态（新增）：同一路由切换自导航开始起 **超过 250ms 仍未完成** 时，SHALL 在内容区显示轻量加载指示（复用既有加载视觉语言：旋转图标 + 主题 accent 色），导航完成、失败或被新导航取代时 SHALL 立即消失。完成时间 ≤250ms 的路由切换 SHALL NOT 出现导航加载态（避免快导航闪烁）。

导航加载态 SHALL 适配 editorial/dark 双主题；`prefers-reduced-motion` 下旋转动画 SHALL 停用/降速（反馈本身仍显示）。导航被取消（如用户转向另一路由）时旧反馈 SHALL 让位于新导航的判定，不残留。

#### Scenario: 慢 chunk 加载期间显示进度条

- **WHEN** 网络缓慢时切换到未加载过的页面路由
- **THEN** 页面顶部出现进度条并随路由完成而消失，颜色适配当前主题（既有行为保持）

#### Scenario: 慢路由切换出现加载态

- **WHEN** 路由切换目标页面 chunk 未加载（冷切换）且 250ms 内未完成
- **THEN** 内容区出现轻量加载指示，用户不再面对无反馈的空白内容区

#### Scenario: 快路由切换不出现加载态

- **WHEN** 路由切换在 250ms 内完成（如已加载页面的暖切换）
- **THEN** 全程无导航加载态出现，顶部进度条行为不变

#### Scenario: 导航完成或失败卸载反馈

- **WHEN** 导航加载态显示期间路由完成、失败或被新导航取代
- **THEN** 加载态立即消失，不遮挡已挂载页面内容

#### Scenario: 减弱动效偏好

- **WHEN** 用户系统开启 `prefers-reduced-motion` 且触发导航加载态
- **THEN** 反馈以无旋转动画（或降速）形式呈现，功能不受影响

### Requirement: 全局加载失败兜底页

前端 SHALL 提供全局错误兜底页（`app/error.vue`）：chunk 加载失败或未捕获错误导致应用无法渲染时，SHALL 展示错误态（错误图标 + 可读错误信息 + 「重新加载」操作），替代空白页面。兜底页视觉 SHALL 复用既有全屏居中错误态模式（与 `app.vue` 初始化错误态同构），并适配 editorial/dark 双主题。

#### Scenario: 页面 chunk 加载失败
- **WHEN** 路由目标页面的 chunk 请求失败（网络中断/弱网超时）
- **THEN** 显示全局兜底页，含错误提示与可点击的「重新加载」入口

#### Scenario: 兜底页重试恢复
- **WHEN** 用户在兜底页点击「重新加载」
- **THEN** 页面刷新并重新走首屏加载流程

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
