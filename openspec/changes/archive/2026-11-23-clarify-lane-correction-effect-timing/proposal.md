<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

# clarify-lane-correction-effect-timing

## Why

泳道修正工作面（矩阵行「⋯」→「编辑泳道」进入）的写操作**逐条即时落库**（加入 / 移出 / 定首义 / 改名），但「生效时机」只以两枚 **10px 小徽标**写在工作面**最底部页脚**（`.bco__foot`）——在 1440×900 这类视口下常被底边裁出视野（2026-11-22 用户截图实证：截图中看不到页脚）。用户因此把「没有保存按钮」读成「没有保存」，报障「编辑泳道都没保存按钮」。

## What Changes

- 修正工作面**顶部标题行常驻**「生效时机」结论型徽标组：`立即生效` / `次日日报生效` / `无需保存`（结论型，各 ≤6 字）。
- 底部页脚的同义小徽标**撤除**（内容上移，不两处重复）。
- **零写路径变更**：加入 / 移出 / 定首义 / 改名仍逐条即时落库，仍不提供全局保存出口（`workshop-lane-compose` 契约不变）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `lane-correction-surface`（`openspec/specs/lane-correction-surface/spec.md`）：新增 requirement「修正工作面 SHALL 常驻展示生效时机结论」（顶部常驻 + 结论型徽标 + 新建模式不渲染）。既有「产品界面不承载解说性长句」requirement 保持不变，与新 requirement 同向（生效时机只以徽标表达）。

## Impact

- **受影响代码**：`front/app/features/tags/components/console/BoardComposeOverlay.vue`（修正模式 header + 页脚）、`front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts`（页脚断言迁到顶部徽标断言）。
- **受影响 spec**：`lane-correction-surface`。
- **后端 / API / 数据模型**：零改动（纯展示层，无网络请求变化）。
- **风险**：低——徽标为纯展示；标题行已 `flex-wrap`，最坏换行不裁切。
- **部署后用户可见变化**：打开「编辑泳道」工作面 → 顶部右侧常驻可见三枚生效时机徽标；底部不再重复显示。无需用户手动操作，无数据迁移、无旧数据降级问题。
