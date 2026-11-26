## D1. 收起 = 第三列退场，入口贴视口右边缘

- **落点**：`front/app/features/tags/components/DailyReportReader.vue` `.drm-layout`
- **取舍**：不保留 2rem 窄边列 —— 窄条仍在吃宽度、且留下视觉残桩；代价是浮标固定在视口右缘（不跟栏位对齐），换回正文完整 ~300px。切换用根类名 `.drm-layout--notes-collapsed` 覆盖 `grid-template-columns`（不新增断点，1100px 窄屏规则不变）。

## D2. 偏好只在显式切换时落盘，缺省收起

- **落点**：`DailyReportReader.vue` `notesRailCollapsed` / `setNotesRailCollapsed(next, { persist })`
- **取舍**：`localStorage` 键 `syntopica.daily-report.margin-notes-collapsed`（`'1'` 收起 / `'0'` 展开），无记录=收起。**程序性展开（新建批注、点高亮、深链定位）不写偏好** —— 否则读者某次划词建批注就把偏好永久翻成「展开」，`默认收起`名存实亡。读取带 `import.meta.client` 守卫（应用 `ssr: false`，属防御性预留，与 `useTheme` 同口径）。

## D3. 收起按钮下沉到栏头，抽屉实例不出现

- **落点**：`front/app/features/tags/components/daily-report/MarginNotesRail.vue` `.mn-rail__head`（新增 prop `canCollapse` + emit `collapse`）
- **取舍**：浮标负责「开」、栏头负责「关」，双向入口都显式可见；窄屏抽屉已有遮罩/Esc 关闭，传 `can-collapse=false` 不引入第二个关闭语义。不改批注卡内部任何交互。

## D4. 自动展开挂进现有 `focusAnnotation` 出口

- **落点**：`DailyReportReader.vue` `focusAnnotation()`（现窄屏 `notesDrawerOpen = true` 同处）
- **取舍**：新建批注落锚（`handleAnchorConfirm` → `focusAnnotation`）、正文高亮点卡、深链/管理页「跳原日报」三条路径共用同一出口，一处补偿即可；避免收起态点高亮「静默无反应」。新建路径保留 `focusInput: true`。

## D5. 空态可发现性补偿

- **落点**：`DailyReportReader.vue` `.drm-notes-fab` 同族新浮标 `.drm-notes-tab`
- **取舍**：收起态看不到「划选正文文字点问一问即可添加」那句教学文案，用 `title` + `aria-label` 在浮标上补一句；不为此加 toast/引导气泡（划词气泡本身就是入口，过度引导反而吵）。计数徽标始终显示（含 0），让「有没有批注」一眼可见。

## D6. 窄屏形态零改动

- **落点**：`DailyReportReader.vue` `isNotesNarrow` 分支
- **取舍**：`<1100px` 已由媒体查询隐藏第三列并走抽屉，收起状态对该分支无效；渲染浮标时加 `!isNotesNarrow` 条件，避免与右下 FAB 重复入口。
