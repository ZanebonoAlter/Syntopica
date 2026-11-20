# 任务 · unify-inner-scrollbar-style

<!-- complexity: simple -->

> 纯前端样式 change：不跑后端、不碰数据库。实现完成后逐条勾选；验证节命令需实际执行并记录结果。
> 进度：全部完成并已归档前自查六项全绿。

## 1. 单一事实源

- [x] 1.1 `front/app/assets/css/main.css` 新增全局工具类 `.scroll-thin`：`scrollbar-width: thin` + `scrollbar-color: var(--color-border-strong) transparent`，并同处声明 `::-webkit-scrollbar`（6px）、`::-webkit-scrollbar-track`（transparent）、`::-webkit-scrollbar-thumb`（`var(--color-border-strong)` + `border-radius: 3px`）、`::-webkit-scrollbar-thumb:hover`（`var(--color-text-muted)`）；注释写清三条机制（标准属性关掉伪元素通道 / `scrollbar-color` 继承而 `scrollbar-width` 不继承 / 标准通道无 hover 语法）
- [x] 1.2 删除死代码：`main.css` 的 `.content-scroll` 整块（4 条伪元素规则 + 注释）；删前 `grep -rn "content-scroll" front backend-go docs scripts` 零使用点
- [x] 1.3 不在任何 `.vue` scoped 样式里新增硬编码滚动条色值

## 2. 容器迁移（7 处裸 thin + 2 处手写色值 + 2 处只走伪元素）

- [x] 2.1 `BoardRadarView.vue`：`<main class="rd-stage ...">` 与 `<div class="rd-rail-scroll">` 加 `scroll-thin`，删除两处 scoped `scrollbar-width: thin`
- [x] 2.2 `BoardRadarReadingRail.vue`：`<aside class="rd-rail">` 加 `scroll-thin`，删除 scoped `scrollbar-width: thin`
- [x] 2.3 `SettingsTabsNav.vue`：`.settings-tabs-nav__track` 元素加 `scroll-thin`，删 scoped `scrollbar-width: thin`
- [x] 2.4 `BoardNarrowDayStrip.vue`：`.bns-strip-track` 元素加 `scroll-thin`，删 scoped `scrollbar-width: thin`
- [x] 2.5 `daily-report/DailyReportSidebar.vue`：`aside.drm-sidebar` 加 `scroll-thin`，删 scoped `scrollbar-width: thin`
- [x] 2.6 `daily-report/DailyReportDeskIndex.vue`：`.ddi` 元素加 `scroll-thin`，删 scoped `scrollbar-width: thin`（该组件当前无挂载点，仅收敛样式）
- [x] 2.7 `DailyReportReader.vue`：`article.drm-reader` 与 `div.drm-notes-rail__scroller` 加 `scroll-thin`，删除各自手写的 `scrollbar-width` + `scrollbar-color` 两行（值等同工具类，视觉零变化）
- [x] 2.8 `features/shell/components/ArticleListPanelView.vue`：`.virtual-list` 加 `scroll-thin`，删除 4 段伪元素规则（含硬编码 `rgba(26,26,26,0.1)` 拇指）
- [x] 2.9 `features/shell/components/AppSidebarView.vue` + `components/layout/AppSidebar.css`：`.sidebar-content` 加 `scroll-thin`，删除 4 段伪元素规则
- [x] 2.10 不动 `DailyReportMiniLifeline.vue`（横向生命线内缩做法刻意保留）、`scrollbar-gutter: stable`（布局预留）与页面级 `::-webkit-scrollbar`

## 3. Review

- [x] 3.1 派 `change-review` 子线程审 diff + 制品，产出 7 条意见（R-1 High / R-2~R-5 Medium / R-6~R-7 Low），逐条处置并建账 `review.md`
- [x] 3.2 R-2/R-3/R-4/R-5 的代码与测试修复已落地（继承事实纠正、hover 降级、两处伪元素容器收编 + 死代码删除、测试补 3 类假绿口子）
- [x] 3.3 R-1/R-6/R-7 为制品修正（映射表格式、日报索引未挂载说明、typecheck 期望值），已并入本文件与 proposal

## 4. 测试

- [x] 4.1 新增 `front/app/assets/css/scrollbar-style.test.ts`（26 例）：`.scroll-thin` 双通道齐全且颜色取 token；`main.css` 不含 `content-scroll`；页面级杂志条段未被波及；11 个 `{file, cls}` 组合断言"带该 class token 的元素**本身**也带 `scroll-thin`"（按静态 class 属性 token 精确比对，排除 `:class` 动态绑定）；迁移过的文件整文件不含 `scrollbar-`；`AppSidebar.css` 同；`DailyReportMiniLifeline.vue` 豁免成对声明不回退
- [x] 4.2 反向验证（两次）：scoped 里加硬编码 `scrollbar-color` → 断言 4 变红；把 `scroll-thin` 从 `.drm-sidebar` 改挂相邻 wrapper → 断言 3 变红；均还原后复跑 26 passed

## 5. 文档

<!-- doc-impact: none(纯前端滚动条配色收敛，不改 flow 业务口径、不改接口字段；行为契约写在本 change 的 theme-system delta spec。归档后按 §12.4 补了 docs/reference/standard/frontend/theming.md 的 .scroll-thin 红线行) -->

- [x] 5.1 `openspec/changes/unify-inner-scrollbar-style/` 内 proposal / design / ui-design / specs / tasks / test-cases 自洽（文件清单与 12 个改动文件一致；继承事实与 hover 口径已按 R-2/R-3 纠正）
- [x] 5.2 归档后 §12.4 补活文档：`docs/reference/standard/frontend/theming.md` 「统一组件」表新增 `.scroll-thin` 一行 + 滚动条红线两条机制（继承边界 / 标准属性关掉伪元素通道），供后续新增内部滚动区直接查；本 change 无 flow 影响，故不补 flow 变更溯源行

## 6. 验证

- [x] 6.1 `cd front && pnpm test:unit app/assets/css/scrollbar-style.test.ts --maxWorkers=2` → **26 passed**（实测）
- [x] 6.2 `cd front && pnpm exec nuxi typecheck` → **exit 0，0 个 TS error**（实测；先前那处 `ObservationRangeControls.vue` 红点是其它会话的瞬时脏状态，复跑已消失）
- [x] 6.3 `cd front && pnpm lint` → **exit 0，0 error（8 warning 均在无关文件）**
- [x] 6.4 `cd front && pnpm build` → **exit 0**（由 6.5 的 `pnpm generate` 承担并成功）
- [x] 6.5 `bash scripts/dev/deploy-frontend.sh` → **exit 0**，产物铺盘 + 后端重启 + `/health` 200（注：首次执行中被宿主机重启打断，已重跑成功）
- [x] 6.6 agent-browser 实测（`http://localhost:5100`，Chromium 152）：dark 下 `.rd-stage`/`.rd-rail-scroll`/`.rd-rail`/`.virtual-list`/`.sidebar-content` 均为 `thin` + `rgba(255, 255, 255, 0.18) rgba(0, 0, 0, 0)`；editorial 下同位置为 `rgba(26, 26, 26, 0.25) rgba(0, 0, 0, 0)`；`html` 的 computed `scrollbar-color` 恒为 `auto`（页面级未受影响）
- [x] 6.7 `bash scripts/harness/scenario-trace.sh unify-inner-scrollbar-style` → **4 个 Scenario 映射齐全**（自动测试 4 / 人工留痕 0）
- [x] 6.8 `bash scripts/harness/archive-readiness.sh unify-inner-scrollbar-style` → **六项全绿**

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| dark 主题下的内部滚动区 | `front/app/assets/css/scrollbar-style.test.ts` |
| editorial 主题下的内部滚动区 | `front/app/assets/css/scrollbar-style.test.ts` |
| 单一事实源与「类挂在滚动容器本身」 | `front/app/assets/css/scrollbar-style.test.ts` |
| 页面级滚动条不受影响 | `front/app/assets/css/scrollbar-style.test.ts` |
