# spa-loading-ux 变更（fix-spa-nav-loading-ux）

## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: 首屏 pre-FCP 背景色

SPA 首屏 HTML 文档 SHALL 在 first paint 前为根元素预设与主题匹配的背景色（editorial 浅色 / dark 深色），使 HTML 到达后到加载模板/应用首帧渲染之间的空窗不呈现浏览器默认白底。背景色 SHALL 与主题判定内联脚本同序生效（data-theme 属性先于首帧设置），且 SHALL 与正式应用主题令牌色值一致（同源维护）。

#### Scenario: 深色主题刷新无白闪

- **WHEN** 主题存储为 dark 的浏览器硬刷新任意路由，且高负载下 first paint 延迟数百毫秒
- **THEN** 该空窗期页面背景为深色（与 dark 主题背景色一致），全程无白色背景闪现

#### Scenario: 浅色主题背景一致

- **WHEN** 主题存储为 editorial（或未设置）的浏览器打开首屏
- **THEN** pre-FCP 背景为 editorial 主题背景色，与后续应用背景无缝衔接

## 未改动 Requirements（对照）

- 首屏加载模板、字体不阻塞、全局加载失败兜底页、初始化加载屏进度反馈：本 change 不改动。
