# UI Design — retire-downgraded-mount

> ui-impact: **minor**（复用既有契约：删除「已降级 ↓」徽标与相关标记渲染，无新布局、无新组件、无新交互模式）

## 1. 变更面（纯删除，无新增 UI）

| 位置 | 现状 | 变更后 |
| --- | --- | --- |
| `ArchiveMatchPopover.vue` 标签卡 | `tag.downgraded` 时渲染 `amp-flag`「已降级 ↓」 | 徽标删除，标签卡其余布局不变 |
| `ArchiveMatchPopover.vue` 相关事件列表 | `item.downgraded` 时渲染「↓」角标 | 角标删除 |
| `ArchiveMatchPopover.vue` ruleColor | `matchReasonColor(reason, downgraded)` 双参 | 移除 downgraded 参数，max_sim 统一色 |
| `archiveMatchFlow.ts` 规则解读文案 | max_sim 分支追加「 ⚠降级匹配」 | 后缀删除，其余文案逐字不变 |

## 2. 契约影响

- 后端 DTO 删除 `downgraded` 字段后，前端 TS 类型里该字段为可选残留（`undefined`），`v-if="tag.downgraded"` 类条件自然为假，**过渡期两版本任意组合均安全**（前端旧版 + 后端新版：字段 undefined 不渲染；前端新版 + 后端旧版：字段恒有值但新前端不消费）。
- 无需弹窗/尺寸档/双视口验收（layout.md 契约不涉及）。

## 3. 验收（minor 档：组件测试映射，指名道姓挂实物）

本 change 为纯删除（无新布局 / 无新组件 / 无新交互模式），不涉及弹窗尺寸档、双视口与 major 原型审批。验收全部落在既有组件与工具函数的组件测试上：

| 验收点 | 组件测试（仓库根相对路径） |
| --- | --- |
| 标签卡不再渲染「已降级 ↓」徽标与「↓」角标 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| `matchReasonColor` 单参化、恒返回裸规则色 token（无 color-mix 淡化分支） | front/app/utils/matchQuality.test.ts |
| `matchInfoLabel` 不再追加「↓」后缀 | front/app/utils/matchQuality.test.ts |
| 行内标签 chip 无 `is-deg` 淡化 class、无降级角标 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| ④ max_sim 命中步无「⚠ 降级匹配」文案 + 方向校验三态 | front/app/features/tags/components/MatchDetailPanel.maxsim.test.ts |
| 通用匹配流程 ④ 命中步无降级后缀 | front/app/features/tags/components/console/archiveMatchFlow.test.ts |
| 弱文章判定（无标签 / max_sim 为弱，direct_hit 等为强） | front/app/features/tags/composables/useBoardArchive.test.ts |
| 日报质量探索 chip 无 `--downgraded` 分档、无「↓」 | front/app/features/tags/components/daily-report/SectionQualityExplore.test.ts |

- 机械断言：归档标签卡 DOM 中不再存在「已降级」文本与 `amp-flag` 徽标节点（Vitest DOM 断言，见上表对应文件）。
- 回归：max_sim 挂载标签的排序档位（MatchTier 合并后）与归档列表排序稳定；上述 8 个测试文件全部通过（`pnpm test:unit <文件> --maxWorkers=2`，114 passed）。
