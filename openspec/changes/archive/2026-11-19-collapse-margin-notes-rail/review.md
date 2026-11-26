# Review 记录 — collapse-margin-notes-rail

审查者：`change-review` 只读子线程（minimax-cn/MiniMax-M3.1-Flash:max），审查基线为工作树快照；并发 change `replace-lane-lifeline-with-topic-events` 的 hunk 已在 brief 中显式排除。

### R-1 2026-11-19【High】浮标 `position: fixed` 被 `perspective` 祖先改写包含块，入口落到整页 50% 高度

- **状态**：已修复（实现期自查 + 审查复核双确认；证据见 `ui-verification/report.md` §1）
- 问题：`.drm-peel-host { perspective: 1400px }` 会把后代 `position: fixed` 的包含块改成本身 → 浮标跑到整篇日报 50% 高度并随正文滚动，首屏看不到入口。
- 处置：浮标与窄屏 FAB 移出 `.drm-peel-host` 子树，改为 `.drm-reader` 直接子元素；修复后实测 `rect.y+height/2 = 450 = innerHeight/2`、`x=1410`（右缘 1440），且不再随滚动位移。

### R-2 2026-11-19【Medium】`ui-design.md` 缺三条机器 marker 与 minor 档必填节

- **状态**：已修复
- 处置：补 `<!-- ui-impact: minor -->`、`<!-- ui-approval: not-required -->`、`<!-- ui-prototype: none -->`；补 `## 入口与入口变更`、`## 验收`（原有 布局/状态契约 已覆盖「状态」「复用或布局」两项）；按 `standard/frontend/interaction.md` 补 `## Interaction Decisions`（D1 `panel` / forbidden `dialog` / `pinned`）与 `## Data Provenance`（批注数=既有 annotations 端点、开合偏好=localStorage 派生）。

### R-3 2026-11-19【Medium】`localStorage` 持久化路径在 Vitest 下是死代码，spec 硬要求零覆盖

- **状态**：已修复
- 处置：守卫由 `import.meta.client`（Vitest 恒 undefined）改为 `typeof window === 'undefined'`（仓库既有 `usePollBundle` 同款），读写分支在 happy-dom 下真实执行；补 4 条断言：① 显式展开写 `'0'`、收起写 `'1'`；② `localStorage='0'` 时进入即三栏；③ 程序性展开（深链定位批注）后偏好仍为 `null`；④ 借 `__mediaQueryClient` + `matchMedia` 桩打开窄屏分支，断言不出现桌面浮标/第三列。

### R-4 2026-11-19【Medium】栏头收起按钮缺 `aria-expanded`

- **状态**：已修复
- 处置：按钮补 `aria-expanded="true"`（面板此刻为展开态），`MarginNotesRail.test.ts` 断言同步补上。

### R-5 2026-11-19【Low】浮标 `aria-controls` 指向恒不存在的面板 id

- **状态**：已修复
- 处置：删掉 `aria-controls` 悬空引用（面板由 `v-if` 卸载，收起态必然指向不存在的节点），保留 `aria-expanded` + 可访问名。

### R-6 2026-11-19【Low】浮标可访问名不含计数与空态引导

- **状态**：已修复
- 处置：`:aria-label` 改用 `notesRailTabTitle`（N 条 / 0 条时含「划选正文文字可添加批注」），`title` 同步，触屏与读屏都能拿到同一信息。

### R-7 2026-11-19【Low】面板进出动效未实现，与 ui-design 动效契约不符

- **状态**：已修复
- 处置：`<aside>` 套 `<Transition name="drm-notes-rail">`，进出 180ms 透明度 + 12px 右移；`prefers-reduced-motion` 段一并列入关闭。

### R-8 2026-11-19【Low】`MarginNotesRail.vue` 缺 `prefers-reduced-motion` 段

- **状态**：已修复
- 处置：文件末尾补 `@media (prefers-reduced-motion: reduce) { .mn-rail__collapse { transition: none } }`，与同目录 `MarginNoteCard.vue` 惯例一致。

### R-9 2026-11-19【Low】浮标定位依赖 CSS 源码顺序

- **状态**：已修复
- 处置：选择器提特异度为 `.drm-reader > .drm-notes-tab`（hover/focus 与窄屏 FAB 同步），不再依赖 `.drm-reader > *` 之后写这条规则。

### R-10 2026-11-19【Low】`daily-report.md` 正文仍把「页边注常驻」列为现行验收基线

- **状态**：已修复
- 处置：`docs/reference/flow/daily-report.md` 2026-10-27 复审段就地加「2026-11-19 修订」，把四件事中的「页边注常驻」改为「默认收起、按需展开」并指向变更溯源。

### R-11 2026-11-19【Low】栏头收起按钮图标方向与 ui-design 文案不一致

- **状态**：已修复
- 处置：保留 `mdi:chevron-right`（朝栏外方向语义更贴切），把 `ui-design.md` 的「⌄」改成「`→`，朝栏外方向」。

## 已核对无问题的维度

- 状态机（默认收起 / 显式落盘 / 程序性不落盘）：审毕无发现（逐条对 spec ADDED 与 ui-design 状态契约）。
- 「点了没反应」四条路径（新建批注 / 正文高亮 / 深链 / 管理页跳原日报）：审毕无发现。
- `railRef` 时序（展开后 `await nextTick()` 再定位）：审毕无发现。
- 断点冲突（1100px 媒体查询与收起类的优先级）：审毕无发现。
- 双主题 token 齐备：审毕无发现（dark 文字对比度在验证轮由 `--color-text-muted` → `--color-text-secondary` 提升，见 `ui-verification/report.md` §3）。
- 窄屏抽屉与批注卡交互零回归：审毕无发现。
