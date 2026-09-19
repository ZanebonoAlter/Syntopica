<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: scheduler -->

## Why

AI 模型未就绪提示目前是固定悬浮在页面顶部居中的 banner（`AiHealthBanner`，`position: fixed; top: 12px`），在「用户意图运行但健康门未通过」期间**持续占据前台顶部展示空间**，遮挡/挤压内容且视觉干扰大。项目已有通知中心（铃铛 + 下拉面板）承载系统性提示，健康未就绪这类「全局运行状态」应收敛到通知中心，不再独占前台空间。

## What Changes

- **移除顶部悬浮 banner**：删除 `AiHealthBanner` 组件及其在 `app.vue` 的挂载（顶部栏已有的常驻 heart-pulse 健康指示保留不动）。
- **铃铛警示态**：通知铃铛在「意图运行但 AI 未就绪」（`analysisPaused=false && aiHealthy=false`）时进入警示态（警示图标/配色），与未读数角标正交叠加。
- **通知面板置顶系统状态条**：通知面板列表顶部新增**虚拟置顶条**（不落库、不计未读数、不参与淘汰），展示「AI 模型未就绪（LLM/Embedding 未连通），分析暂停运行」+「重新检测」按钮 +「去配置」链接（复用 `useHealthReprobe` / 跳 `settings?section=ai-health`）。
- **可见性条件不变**：用户主动暂停（`analysisPaused=true`）时不警示（已知暂停，无需再提示健康）；提示不修改/不禁用暂停/启动按钮（沿用 analysis-pause-control 红线）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `analysis-pause-control`: 「前端健康未就绪提示」需求的展示位置从「顶部 banner」改为「通知中心（铃铛警示 + 面板置顶条）」；「顶部栏常驻健康指示」需求中与 banner 并存的互斥表述同步修正。
- `notification-center`: 新增「客户端虚拟系统状态条」需求——通知面板支持非落库的置顶系统状态展示（不产生通知表行、不触发 WS、不计未读数），首例为 AI 健康未就绪。

## Impact

- **前端**：`front/app/app.vue`（移除挂载）、`front/app/components/ai/AiHealthBanner.vue`（删除）+ 其测试、`front/app/components/ui/NotificationBell.vue`（警示态）、`front/app/components/ui/NotificationPanel.vue`（置顶条）+ 相关测试。
- **后端**：无改动（AI 健康态由既有 `/api/schedulers` 轮询提供；`POST /api/ai/health/reprobe` 复用）。
- **数据**：无迁移；通知表不动（虚拟条目不落库）。
- **文档**：`docs/reference/flow/scheduler.md` 代码入口节 banner 表述更新。
