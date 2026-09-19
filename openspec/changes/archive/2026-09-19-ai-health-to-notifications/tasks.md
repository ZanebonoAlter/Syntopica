# Tasks — AI 健康提示移入通知中心

## 1. 通知中心警示态实现

- [x] 1.1 `NotificationPanel.vue` 增加置顶系统状态条：`!analysisPaused && !aiHealthy` 时在列表容器上方渲染「AI 模型未就绪（LLM/Embedding 未连通），分析暂停运行」+「重新检测」（`useHealthReprobe`，检测中禁用文案「检测中…」，完成后 `loadSchedulersStatus()` 刷新）+「去配置」（NuxtLink `/settings?section=ai-health`）；空列表/加载/错误态下仍可见。验证：新增/更新组件测试断言置顶条显隐与按钮行为
- [x] 1.2 `NotificationBell.vue` 增加警示态：同条件下铃铛图标切 `mdi:bell-alert` + warning 色令牌（未读角标逻辑不动）。验证：组件测试断言两种状态下图标/配色渲染

## 2. 移除顶部 banner

- [x] 2.1 删除 `AiHealthBanner.vue`、`AiHealthBanner.test.ts`，移除 `app.vue` 挂载与注释、更新 `error.vue` 相关注释；将原 banner 测试断言（显隐条件/重探行为/暂停态不显）迁移进 1.1 的面板测试。验证：`grep -rn "AiHealthBanner" front/app` 仅剩（或零残留）无引用

## 3. 测试与验证

- [x] 3.1 跑受影响前端测试：`cd front && pnpm test:unit components/ui/NotificationPanel.test.ts components/ui/NotificationBell.test.ts --maxWorkers=2`（以实际测试文件名为准）全绿
- [x] 3.2 前端门禁：`cd front && pnpm lint && pnpm exec nuxi typecheck` 通过
- [x] 3.3 人工验证（已部署实测）：健康未就绪时铃铛警示色（.notif-bell--warning + amber 色）+ 面板置顶条（重新检测/去配置可用）真实出现；顶部悬浮 banner 零残留；面板开合正常（`bash scripts/dev/deploy-frontend.sh` 已部署，/api/schedulers/status 实测 ai_healthy=false 且置顶条出现，行为与后端状态一致）

## 4. 文档

<!-- doc-impact: flow -->

- [x] 4.1 `docs/reference/flow/scheduler.md`：健康门约束节「顶部 banner」表述改为通知中心警示、心跳节「未就绪 banner」入口表述、代码入口节 banner 组件改为 NotificationBell/NotificationPanel 置顶条；变更溯源表行按 §12.2 归档后补
- [x] 4.2 归档前 `bash scripts/harness/doc-impact.sh verify openspec/changes/ai-health-to-notifications` 退出码 0（实测通过，声明 flow 文件 1 个）

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 意图运行但不健康时展示 banner | front/app/components/ui/NotificationPanel.test.ts |
| 健康恢复后警示消失 | front/app/components/ui/NotificationPanel.test.ts |
| 用户主动暂停时不展示健康 banner | front/app/components/ui/NotificationPanel.test.ts |
| 面板空列表时置顶条仍可见 | front/app/components/ui/NotificationPanel.test.ts |
| 虚拟条目不产生落库通知 | 人工验证（notifications 表无新行，后端无改动，grep 后端无通知写入点变更） |
| 虚拟条目不受清空/已读操作影响 | front/app/components/ui/NotificationPanel.test.ts |
| 铃铛警示态图标/配色 | front/app/components/ui/NotificationBell.test.ts |
| 设置页展示健康面板与总开关 | front/app/features/settings/components/SettingsSectionAiHealth.test.ts |
| 顶部栏常驻健康指示 | 人工验证（存量行为，非本 change 改动——AppHeaderView 常驻 heart-pulse 指示随 MODIFIED requirement 携带进 delta，实现组件实存） |
| 顶部 banner 移除无残留 | grep -rn "AiHealthBanner" front/app 零引用 + front lint/typecheck |

- `cd front && pnpm lint` → 退出码 0
- `cd front && pnpm exec nuxi typecheck` → 退出码 0
- `cd front && pnpm test:unit <受影响测试文件> --maxWorkers=2` → 全绿
