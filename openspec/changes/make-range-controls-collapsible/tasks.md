## 1. 折叠状态

- [x] 1.1 `ObservationRangeControls.vue` 引入 `collapsePref` ref + `useIsNarrowViewport()`，`collapsed = pref ?? isNarrow`
- [x] 1.2 `toggleCollapsed()` 写 `localStorage['syntopica.orc.collapsed']`；`onMounted` 读取（用 `typeof localStorage` 运行时探测，预渲染期与测试环境都安全）
- [x] 1.3 `rangeSummary` computed：拼 `<档位> · <起始 MM-DD>~<结束 MM-DD>`；`range` 为 null 时显示「数据未就绪」

## 2. 模板与样式

- [x] 2.1 标题行改为 `<button class="orc-toggle">`（整行可点），带 `aria-expanded` / `aria-controls` 与 ▾/▸ 指示
- [x] 2.2 折叠时隐藏档位按钮组与副标题；主体包 `<div v-show="!collapsed">`（保留 DOM，手势状态不丢）
- [x] 2.3 标题由 `<h3>` 改 `<span class="orc-title">`（button 内不得放 heading），视觉样式不变
- [x] 2.4 新增 `.orc-toggle` / `.orc-summary` / `.orc-chevron` / `.orc.is-collapsed` 样式，折叠态高度 ≈32–36px

## 3. 用例

- [x] 3.1 C1 宽屏默认展开：`aria-expanded="true"`，无摘要行
- [x] 3.2 C2 点击收起源：摘要含档位与起止日，主体内联 `display: none`
- [x] 3.3 C3 窄屏默认收起（模块级开关驱动 `useIsNarrowViewport`）
- [x] 3.4 C4 手动展开写 `localStorage`，重新挂载仍展开（手动优先于窄屏默认）
- [x] 3.5 C5 折叠切换不 emit `preview`/`commit`/`source-change`

## 4. 测试

- [x] 4.1 `pnpm test:unit app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → **41 passed**（含 C1–C5）
- [x] 4.2 既有使用处回归：`BoardComposeOverlay.test.ts` + `BoardComposeOverlay.correction.test.ts` → **62 passed**
- [x] 4.3 `pnpm exec nuxi typecheck` exit 0（仅 DEP0205 噪音）、`pnpm exec eslint` 0 error



## 5. 文档

<!-- doc-impact: none(纯前端组件折叠态，不改 flow 业务口径与接口字段；行为约束写入本 change delta spec) -->

- [x] 5.1 `ui-design.md` 记录折叠态、窄屏断点（767.98px）与宽屏零变化红线
- [x] 5.2 `proposal.md` Impact 记录三个使用处的一致行为与画布可视高度收益
- [x] 5.3 无 flow 影响：本 change 只改前端展示形态，不改业务规则、接口或数据结构

## 6. 验证

- [x] 6.1 `bash scripts/dev/deploy-frontend.sh` → 产物 21:42 铺盘，`/health` 200
- [ ] 6.2 窄屏实机确认：画布可视高度增加、展开后功能完整
- [ ] 6.3 review 建账（review.md）与归档

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 折叠后只留一行摘要 | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` |
| 窄屏默认收起、宽屏默认展开 | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` |
| 手动选择覆盖默认并持久化 | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` |
| 折叠不触发范围变更 | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` |
