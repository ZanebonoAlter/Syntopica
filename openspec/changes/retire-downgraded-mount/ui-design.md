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

## 3. 验收

- 机械断言：归档标签卡 DOM 中不再存在「已降级」文本与 `amp-flag` 徽标节点（Vitest 快照/DOM 断言）；
- 回归：max_sim 挂载标签的排序档位（MatchTier 合并后）与归档列表排序稳定。
