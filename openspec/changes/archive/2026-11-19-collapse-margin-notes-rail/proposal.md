<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

## Why
日报阅读台桌面三栏把页边注栏（第三列 `clamp(15rem, 17vw, 17rem)` + 栏间距 ≈ 300px）常驻在右侧，而批注是低频操作——多数时间只读正文的读者被白拿走了一整栏宽度。2026-11-18 用户要求：默认收起，需要时点开，把右侧空间还给正文。

## What Changes
- 桌面（≥1100px）页边注栏**默认收起**：网格退成两列（目录 + 正文），正文吃满可用宽度；视口右边缘中部浮出竖向「页边注 · N」入口标签，点击展开。
- 展开态栏头新增收起按钮（⌄），与浮标构成一次点击即可切换的双向入口。
- 收起状态写 `localStorage` **记忆用户上次显式选择**；无记录（首次访问/换浏览器）即收起。
- 收起态下新建批注（划选 → 「问一问」落锚成功）**自动展开并定位新卡、聚焦输入框**；点正文高亮、深链「跳原日报」同样自动展开定位（与窄屏自动开抽屉同口径）。程序性展开不回写偏好。
- 窄屏（<1100px）形态不变（右下 FAB + 右抽屉）；空态（0 条批注）同样默认收起，浮标计数显示 0 并以 title 提示「划选正文可添加」。
- 不改接口、数据表、后端；不改批注卡内部交互。

## Capabilities
### Modified Capabilities
- `daily-report-margin-notes`：新增「页边注栏收起与展开」需求；「空态与窄屏形态」补充桌面默认收起、浮标入口与收起态空态口径。

## Impact
- 前端：`front/app/features/tags/components/DailyReportReader.vue`（布局类别、收起状态与记忆、浮标、自动展开定位）、`front/app/features/tags/components/daily-report/MarginNotesRail.vue`（栏头收起按钮 + 事件）。
- 新增一个 localStorage 键 `syntopica.daily-report.margin-notes-collapsed`。
- 回归测试：`DailyReportReader.test.ts`（三栏断言改为默许收起的两列 + 展开后三栏）、`MarginNotesRail.test.ts`（收起按钮存在/上抛）。
- 文档：`docs/reference/flow/daily-report.md`（变更溯源自 + 页边注栏形态描述）。
