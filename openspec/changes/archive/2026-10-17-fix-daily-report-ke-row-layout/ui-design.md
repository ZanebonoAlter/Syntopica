<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

<!--
  marker 契约（ui-design-gate 机器读取）：minor 档——复用既有 thread-row 分区与
  KeyEventMarkerActions 组件，仅修订其 grid 摆放契约（50/50 分栏 + 动作组行底右对齐），
  无新组件/新模式/新弹层，无需原型与用户审批。
-->

## 入口与入口变更

- 入口不变：日报阅读器（`BoardDailyReportTimeline` → `DailyReportTopicSection`）内每条线索行，当 thread 处于 suggested/confirmed/dismissed/无标记四种状态之一时，行内出现 `KeyEventMarkerActions` 的 detail 块与动作组。总览（overview）模式入口不变（仅取消维护入口）。
- 入口变更：无新增/删除入口；仅同一入口内的**摆放位置**变化——AI 理由块从「错乱的右侧宽列（scope 断链 auto 放置）」改为右半等宽列；动作组从「窄列纵排 / 错乱堆叠」改为全宽底行右对齐（行右下角）。

## 受影响状态（逐个交代）

| 状态 | 受影响元素 | 变化 | 不变量 |
| --- | --- | --- | --- |
| success（suggested） | `.ke-detail__suggestion`（理由+依据） | 移至右半列（c2/r1），带左分隔线 | 理由 `pre-line` 折行不截断、依据数文案不变 |
| success（suggested） | `.ke-actions__badge` + 确认为关键/不采纳按钮 | 徽标+按钮横排，全宽底行右对齐 | 单步动作、互斥禁用（busy 时兄弟动作禁用）、不嵌套 header 按钮 |
| success（confirmed） | 已标记徽标 + 取消标记按钮 | 同动作组横排右下 | 取消纠错语义不变 |
| success（dismissed） | 已不采纳注记 + 仍标为关键按钮 | 同上 | 不采纳不是否定、可再选 |
| success（无标记） | 标为关键按钮 | 底行右对齐 | 无 AI 建议不出现徽标/理由 |
| error | `.ke-detail__error`（保存/读取失败） | 随 detail 块落右半列；无 detail 时动作组仍底行右对齐 | 局部反馈保留原文可重试，不阻塞其他条目 |
| empty（无 detail 内容） | 不渲染 `.ke-detail`（零内容不占行） | 动作组直接落 r2 底行 | 行高不虚增 |
| loading | busy 标签（确认中…/处理中…/取消中…） | 位置随动作组变化 | 按钮禁用态不变 |
| 窄屏 ≤720px | 整行回退单列 | detail r2 全宽、动作组 r3 全宽横排（与宿主断点同步） | 不挤压标题/摘要 |

## 复用组件与布局模式

- 复用组件：`KeyEventMarkerActions.vue`（零 API 副作用展示组件）、宿主 `.drm-thread-row` grid 分区模式、`.ke-detail__suggestion` 左分隔线样式、现有按钮/徽标样式类——不新增基础组件、不新增 layout mode。
- 布局模式：仍属日报阅读器 workspace 型 page shell 内的内容分区，不触碰 page shell 四模式与 dialog 档位；`.drm-thread-row` 由 `minmax(0,1fr) auto` 改为 `minmax(0,1fr) minmax(0,1fr)`（两等宽列，左右各 50%），溢出策略：列内 `min-width: 0` + 理由 `overflow-wrap: anywhere`，不横向溢出。
- **scope 归属契约（本次根因的防线）**：多根 fragment 组件的根节点不继承宿主 scope 属性——`.ke-detail` / `.ke-actions` 的 grid 摆放规则 MUST 写在 `KeyEventMarkerActions.vue` 自身 scoped 样式（根节点带自家 scope attr，可靠命中）；宿主 scoped 样式 MUST NOT 再出现 `.drm-thread-row > .ke-detail/.ke-actions` 选择器（死代码，源码守卫测试拦截）。

## 验收映射（指名道姓）

- 组件测试：`front/app/features/tags/components/KeyEventMarkerActions.test.ts`——继承「结构分离」用例（detail/actions 兄弟块、按钮不嵌套、长理由不截断断言不变，注释措辞随新布局更新）；新增源码守卫用例（参照 `UpgradeSuggestionPanel.gen.test.ts` 的 readFileSync 先例）：断言摆放规则在子组件 scoped 样式内、宿主 `DailyReportTopicSection.vue` 样式不含 `.drm-thread-row > .ke-` 选择器。
- 端到端：agent-browser 验证（suggested 态 DOM 复现 + 四状态走查 + 超长理由）在 `ui-verification/report.md` 建账（### A-N 场景【pass|fail】+ 依据行）；部署静态 :5100 后复核。
- 无需原型（minor 档，approval=not-required）。
