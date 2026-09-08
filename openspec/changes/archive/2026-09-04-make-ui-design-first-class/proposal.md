<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

当前 OpenSpec 流程把技术方案写进 `design.md`，却没有把页面入口、用户任务、交互状态、布局宽度和视觉方向作为正式制品；现有原型要求又只覆盖复杂逻辑，导致前端常在实现阶段临场设计，直到开发完成后才暴露交互黑盒与居中/宽屏不一致。需要把 UI 影响识别、原型确认和视觉验收前移并机器化，避免继续依赖用户临时提醒。

## What Changes

- 为所有 change 增加机器可读 `ui-impact` 声明，分为 `none`、`minor`、`major`，与逻辑复杂度独立判定。
- **BREAKING（开发工作流）**：基于项目本地 OpenSpec schema 增加必经的 `ui-design` 规划制品；无 UI 影响时产出最小 N/A 记录，minor change 写交互契约，major change 必须写完整 UI 设计并提供可丢弃原型。
- major UI change 在原型获得用户明确确认前不得开始前端实现；声明与实际前端影响不一致时由机器门禁反向质询或阻断。
- 建立统一布局契约，定义 reader、contained、workspace、split 等页面 shell，以及 dialog 尺寸档、响应式和组件复用要求，消除自由散落的宽度决策。
- 将实现后验收升级为“对照已确认原型”的双层检查：opencli 验证交互主链路，视觉子代理检查约定视口下的布局、样式和响应式。
- 将 UI 门禁裁决写入 harness 事实库，便于统计哪些 change 被提醒、阻断、确认或豁免。

## Capabilities

### New Capabilities

- `ui-design-workflow`: 定义 UI 影响分档、`ui-design` 制品、major 原型确认、布局契约、实现入口门禁和实现后交互/视觉验收行为。

### Modified Capabilities

- 无。

## Impact

- OpenSpec：新增项目本地 workflow schema 与 `ui-design` 模板，更新 `openspec/config.yaml` 默认 schema；后续新 change 的 artifact 顺序发生变化。
- Harness：新增或扩展 UI 设计入口门禁、事实库裁决记录及对应 smoke/单元测试；需与 `harden-harness-policy-and-spill` 的 `policy.decision` 契约兼容。
- 开发规范：更新 `docs/reference/开发执行规范.md`、`docs/reference/standard/frontend/`、根与前端 `AGENTS.md` 的速查入口。
- UI 验证：复用 `frontend-design`、`ui-verify`、opencli 与视觉子代理，不引入 Storybook 或新的前端运行时依赖。
- 产品运行时、数据库和现有用户数据不受影响；该 change 只改变后续开发流程。
