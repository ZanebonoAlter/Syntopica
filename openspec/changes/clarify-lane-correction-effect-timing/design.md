# clarify-lane-correction-effect-timing — 技术设计

## Context

修正工作面（`BoardComposeOverlay.vue` 修正模式）的写操作全部逐条即时落库：`useLaneCorrection.ts` 的 `removeMember` / `addMember` / `applyAnchor` / `commitRename` 各自独立发请求，成功即 toast。**没有任何批量提交概念**——因此界面里没有、也不该有「保存」按钮。

但「生效时机」这一信息当前只存在于 `.bco__foot`（页脚，两枚 10px 徽标）：工作面三栏 + 时间视窗条已占满高度，页脚常被视口底边裁掉（2026-11-22 用户截图实证：3840×1907 裁切区域内看不到页脚）。信息存在但用户看不到 = 等于没有。

## Goals / Non-Goals

**Goals**

- 生效时机**首屏可见**（顶部标题行），不需要滚到底部。
- 文案保持结论型（≤6 字/项），不违反 `lane-correction-surface`「产品界面不承载解说性长句」与 `standard/frontend/interaction.md`「界面文字只承担三类职责」。

**Non-Goals**

- 不引入保存 / 提交语义（写路径零改动）。
- 不改首义 / 成员 / 改名的交互流程与提示。
- 不动新建泳道模式。

## Decisions

### D1. 生效时机徽标上移到顶部标题行，页脚同义徽标撤除
- **结论**：徽标组渲染在 `.bco__top` 标题行右侧（与泳道名同一行），修正模式的 `.bco__foot` 整块移除。
- **落点**：front/app/features/tags/components/console/BoardComposeOverlay.vue:.bco__top
- **取舍**：备选「顶部加说明句 + 保留页脚徽标」——两处重复，且说明句触碰解说性红线，弃用；备选「只放大页脚徽标」——页脚仍可能被视口裁掉，问题不解，弃用。
- **右置判断（review R-7）**：徽标组用 `margin-left: auto` 右置，而 `standard/frontend/interaction-conventions.md` §1 要求「状态标记在左侧醒目位」。该条字面作用域是**列表项级**状态图标，本处是页级汇总徽标，且标题行右区**无其它 meta 元素**（无计数/箭头可挤压）；实测 1440×900 与 1920×1080 两档徽标组完整在视口内（1920 实测 x=1688 / w=212 / right ≤ innerWidth，见 `ui-verification/report.md` A-2）→ 右置不构成 meta 挤压，维持现状。

### D2. 文案用三枚结论型徽标，不写解释句
- **结论**：`立即生效`（accent 实线）/ `次日日报生效`（虚线）/ `无需保存`（中性）。
- **落点**：front/app/features/tags/components/console/BoardComposeOverlay.vue
- **取舍**：备选「一句话说明『本工作面改动即时生效，无需保存』」——属解说性长句，与 `interaction.md`「界面文字只承担三类职责」相撞，弃用；徽标是规范点名的「状态结论」形态。
- **作用域口径（review R-3）**：被删页脚原文写「归属 · 立即生效」，顶部徽标组**不加**作用域词——该组覆盖加入 / 移出 / 首义 / 改名四类动作，「归属立即生效」对改名不准确。分工：**顶部 = 全局生效时机摘要**（不加单一作用域），**左栏面板头 = 分块作用域**（`LaneMemberList`「归属 · 立即生效」= 成员归属；`LaneAnchorScope`「次日日报生效」= 首义范围）——两处语义不同层，不是同一信息的重复；左栏徽标属既有组件，本次不扩范围去重。

### D3. 不提供全局保存出口
- **结论**：工作面仍不出现「保存」按钮；用户「找不到保存按钮」的疑惑由 `无需保存` 徽标正面回答。
- **落点**：openspec/specs/workshop-lane-compose/spec.md
- **取舍**：备选「加全局保存 + 草稿式写入」——与 `workshop-lane-compose`「编辑模式的加入/移出/首义/改名 SHALL 逐条即时落库……界面 SHALL NOT 提供编辑模式的全局撤销或二次确认保存」正面冲突，需重写写模型，弃用。

## Risks / Trade-offs

- **顶部行拥挤**：标题行已有「← 返回 / 泳道名 / 窗口标签」；徽标组用 `flex-wrap` + 11px 字号，最坏换行到第二行，不裁切。
- **徽标被误读为按钮**：徽标用边框 + 淡底 + 文字（无 pointer 交互），与按钮视觉区分。

## Migration Plan

纯前端展示层改动，无数据迁移、无接口变化。回滚 = 还原 `BoardComposeOverlay.vue` 单文件。
