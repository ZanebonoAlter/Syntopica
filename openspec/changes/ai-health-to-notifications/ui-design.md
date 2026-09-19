<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design — AI 健康提示移入通知中心

## 入口与入口变更

- **移除**：`app.vue` 顶部悬浮 `AiHealthBanner`（fixed 居中横幅不再渲染，组件删除）。
- **入口 1（警示态入口）**：顶部栏通知铃铛 `NotificationBell`（既有入口）——「意图运行但 AI 未就绪」时图标/配色进入警示态，点击行为不变（开合通知面板）。
- **入口 2（详情入口）**：通知面板 `NotificationPanel` 列表顶部置顶系统状态条（新增展示位，面板内不新增导航层级）。
- 「去配置」沿用既有路由 `/settings?section=ai-health`；「重新检测」复用 `useHealthReprobe`。

## 受影响状态

| 状态 | 交代 |
| --- | --- |
| 警示（等价 error 语义） | `analysisPaused=false && aiHealthy=false`：铃铛警示图标+警示配色；面板置顶条可见，含「重新检测」（检测中禁用态文案「检测中…」）与「去配置」 |
| 正常（success/其余） | 健康恢复或用户主动暂停后：铃铛回归普通态、置顶条消失（状态驱动，无空态占位） |
| loading | 「重新检测」点击后的 `reprobing` 禁用态即全部 loading 语义；面板列表自身 loading/empty/error 状态矩阵不受影响，置顶条独立于列表状态之外常显（在列表骨架/空态/错误态之上仍可见） |
| empty | 面板空列表时置顶条仍可见（系统状态与通知列表正交） |

## 复用组件与布局模式

- 面板壳、Teleport 定位、z-index、宽度 `min(380px, 92vw)` 全部沿用 `NotificationPanel` 既有契约（layout 契约见 `standard/frontend/layout.md`，popover 模式不变）。
- 置顶条复用 `--color-warning*` 色令牌（与原 banner 同语义），文案与按钮结构平移自 `AiHealthBanner`。
- 铃铛警示态复用 `header-btn` 既有按钮类，仅叠加图标/配色变体；未读数角标逻辑不动（与警示态正交叠加）。
- 不新增 dialog、不改页面 layout mode。

## 验收映射

- 组件测试（首选）：`NotificationBell` 警示态渲染断言 + `NotificationPanel` 置顶条显隐/按钮行为断言 + `app.vue` 层级不再渲染 banner 的回归断言（对应既有 `AiHealthBanner.test.ts` 用例迁移）。
- 人工验证：健康未就绪时铃铛警示 + 面板置顶条可见、重新检测可触发、去配置可跳转；暂停意图下全部消失。
