## 1. 窄屏基建

- [x] 1.1 新建 `useMediaQuery` composable（SSR 安全：client 端 matchMedia + 监听，server 端返回默认 false），导出 `useIsNarrowViewport()`（断点 768px）；验证：`pnpm test:unit composables/useMediaQuery --maxWorkers=2` 通过（断点边界 767/768 翻转、监听器清理）
- [x] 1.2 `FeedLayout.css` 高度改 `100dvh`（`@supports` fallback `100vh`）+ 新增 `@media (max-width: 767.98px)` 基础块（gutter 16px、`.feed-layout` 窄屏单栏化）；验证：`pnpm lint` + 手动缩窗确认 768px 以下侧栏隐藏、无横向滚动条（验收：375×667 常驻侧栏 display:none、docSW=375，见 verification/narrow-375x667-list.png）

## 2. 抽屉导航

- [x] 2.1 新建 `AppSidebarDrawer.vue`（Teleport + scrim + transform 滑入，宽 min(80vw, 320px)，260ms ease-out；props：open/内容 slot；emits：close/select）+ 组件测试；验证：`pnpm test:unit AppSidebarDrawer --maxWorkers=2` 通过（开/关过渡类、点 scrim 关闭、select 后 emit close）
- [x] 2.2 `AppHeaderView.vue` 窄屏变体：汉堡入口（窄屏显示，宽屏隐藏）+ 溢出项收「⋯」菜单（`matchMedia` CSS 驱动）；验证：`pnpm test:unit AppHeaderView --maxWorkers=2` + `pnpm exec nuxi typecheck`
- [x] 2.3 `FeedLayoutShell.vue` 接入抽屉：窄屏用 `AppSidebarDrawer` 渲染 `AppSidebarView`（同一数据源两处容器），选中订阅源/标签 → 应用筛选 + 关抽屉 + 回列表态；验证：`pnpm test:unit FeedLayoutShell --maxWorkers=2`（窄屏 mock 下抽屉交互链）+ 手动缩窗走查

## 3. 列表/阅读单栏切换

- [x] 3.1 `FeedLayoutShell.vue` 增加 `viewMode: 'list' | 'reading'`（仅窄屏分支读取）：点列表项进阅读态、返回回列表态；列表容器 `scrollTop` 记忆 + `nextTick` 恢复（超已加载范围先补一页，仍不足落顶）；验证：`pnpm test:unit FeedLayoutShell --maxWorkers=2`（切换、滚动恢复、深滚动补页用例）
- [x] 3.2 resize 跨断点：`useIsNarrowViewport()` 变化回调重置 `viewMode`、保留 `selectedArticle`（宽屏恢复双栏联动）；验证：`pnpm test:unit composables/useMediaQuery FeedLayoutShell --maxWorkers=2`（跨越断点用例）

## 4. 页面窄屏走查修复

- [x] 4.1 逐页 375×667 走查修复：settings / tags / discovery / 日报页 + 主工作台 empty（FeedEmptyGuide）与初始化 error 态——修横向溢出与不可点元素（chips 加 overflow-x-auto、按钮全宽等按需）；验证：agent-browser 375×667 逐页截图存 `openspec/changes/mobile-viewport-stage1/verification/`，每页 `document.documentElement.scrollWidth <= 375`（验收：discovery/settings/tags 均 375 ✓，截图 narrow-375x667-{page}.png；tags 页无独立日报只读入口——日报在 board 详情内，生成按钮会触发任务不在验收范围，跳过留痕）
- [x] 4.1+（验收后补遗，用户实测发现三缺陷并修复）：① 顶栏双汉堡 + logo-container(195px) 与进度 chip(204px) flex 溢出互叠——窄屏隐藏 `.logo-container`（logo+宽屏 toggleSidebar 汉堡让位）、chip 压缩 max-width:10rem+ellipsis；② 「⋯」溢出菜单被虚拟列表盖住——`.app-header` 的 backdrop-filter 自成 stacking context 锁住内部 z:60 弹层，窄屏 header 提升 z-index:900（抽屉 1000/1001 仍在其上）；③ 阅读态 feed 徽章(204px, shrink:0) 与操作组(shrink:0) 合计溢出、header-left 被压慴后 badge 溢出互叠——article-header wrap 两行化 + badge 文本截断；复验：elementFromPoint 命中菜单项本体、阅读态 header 两行高 92px 无相交、截图 narrow-375x667-{list-fixed,overflow-menu,reading-fixed}.png；测试 15/15（AppHeaderView 9 + TagQueueProgressChip 6）、lint 0 error、typecheck exit 0
- [x] 4.1++（静态发布验证补遜，用户实测）：④ 阅读态可横向滑出空白——微信源文章（rich_media_content 容器不走 .markdown-body 断词）正文含 40+ 连续长单词（「AAAA…」440px）撑破 preview-mode(scrollWidth 482)——窄屏块给 `.reading-col` 补 overflow-wrap/word-break 断词；⑤ 操作组含上下篇导航时 353px > 可用宽——header-actions 允许自身横滑；验证：静态发布（NUXT_PUBLIC_API_BASE=/api pnpm generate → 铺 backend-go/frontend → :5100）后 375 实测 docSW=375（不可再横滑）、preview 342/341、panel 341、菜单 elementFromPoint 命中本体、1920 宽屏零变化抽检 ✓；截图 narrow-375x667-{reading-longword,overflow-menu-static}.png 经主线程视觉复核；备注：验证两次被新手引导遮罩（driver overlay，静态版首访自动弹）污染 hitInMenu 断言，关闭引导后实测通过——引导弹窗本身窄屏布局正常

## 5. 视觉验收（ui-design.md Acceptance）

- [x] 5.1 窄屏主链路 opencli 断言：375×667 下「列表无横向溢出 → 点文章进阅读态 → 返回恢复滚动位置 → 开抽屉 → 选筛选关抽屉回列表态」逐步断言 + 375×667 / 390×844 两档截图存 verification/（验收：七步全 ✓，含 scrollTop 400→400 精确恢复、抽屉选「ai新闻」后筛选生效；截图 15 张落盘）
- [x] 5.2 宽屏零变化对照：1440×900 / 1920×1080 两档对主工作台、discovery、settings、tags、日报逐页截图，与实现前基线截图对比（基线在动手前先截并存 verification/baseline/）；差异记录进验证节（验收：DOM 断言全 ✓——常驻侧栏可见/抽屉未挂载/无 is-narrow；新旧 bundle 结构指纹一致，唯一差异 header 按钮 10→12 为宽屏 display:none 休眠节点（汉堡+溢出），与设计一致）

## 测试

- [x] 6.1 影响范围单测全绿：`bash scripts/harness/change-scope.sh` 判定前端影响文件，`pnpm test:unit <受影响文件...> --maxWorkers=2`（不带 `--`）全通过；`pnpm lint` + `pnpm exec nuxi typecheck` 通过（实跑：FeedLayoutShell + useMediaQuery + AppSidebarDrawer + AppHeaderView 四文件 39/39；lint 0 error；typecheck exit 0）
- [ ] 6.2 不跑全量前端单测（树莓派负载纪律）；若归档门禁要求，先停其它 pi 会话再 `pnpm test:unit --maxWorkers=2`

## 文档

- [x] 7.1 `docs/reference/standard/frontend/layout.md` 新增「窄视口降级」节：768px 断点、抽屉模式、列表/阅读切换、375px 不破版基线、宽屏零变化要求；双视口验收节补窄屏档（375×667 / 390×844）为 major change 可选验收视口 <!-- doc-impact: flow, standard -->
- [x] 7.2 `docs/reference/flow/reading.md` 代码入口节补窄屏切换入口（`viewMode` / `AppSidebarDrawer`），变更溯源节补本 change 链接（代码入口已补；溯源行按 §12.2 归档后补）

## 验证

- [x] 8.1 `cd front && pnpm lint && pnpm exec nuxi typecheck` → 期望：0 error（实跑：lint 0 error / 7 存量 warning，typecheck exit 0）
- [x] 8.2 `cd front && pnpm test:unit features/shell components/ui composables --maxWorkers=2` → 期望：全部 pass（实跑：4 文件 39/39，含抽屉交互链、viewMode 切换、滚动恢复、断点跨越）
- [x] 8.3 `cd front && pnpm build` → 期望：构建成功（实跑：✨ Build complete，3.67 MB / 744 kB gzip）
- [x] 8.4 verification/ 目录四档截图齐全（375×667、390×844、1440×900、1920×1080）+ baseline 对照无宽屏回归 → 期望：与 ui-design.md Acceptance 一致，差异说明已记录（15 张截图落盘；宽屏 DOM 断言 + 新旧 bundle 指纹对照零变化，唯一差异 header 按钮 10→12 为 display:none 休眠节点）
- [x] 8.5 `bash scripts/harness/doc-impact.sh verify openspec/changes/mobile-viewport-stage1` → 期望：文档对账通过（实跑：通过，声明 flow, standard）

### Scenario → 测试映射（归档门禁）

| Scenario | 落点 |
| --- | --- |
| 打开与关闭抽屉（点遮罩/Esc/再点入口） | `front/app/components/ui/AppSidebarDrawer.test.ts` |
| 抽屉内应用筛选（关抽屉回列表态） | `front/app/features/shell/components/FeedLayoutShell.test.ts` |
| 进入与退出阅读态（含滚动位置恢复/超范围补页） | `front/app/features/shell/components/FeedLayoutShell.test.ts` |
| 断点跨越的状态保持（窄↔宽重置/保留选中） | `front/app/features/shell/components/FeedLayoutShell.test.ts` + `front/app/composables/useMediaQuery.test.ts` |
| 地址栏收起时高度自适应（100dvh） | 人工：CSS `@supports` 块源码 + verification/narrow-375x667-list.png 走查 |
| 375px 逐页走查（含空态/错误态） | 人工：agent-browser 断言（discovery/settings/tags docSW=375）+ verification/narrow-375x667-{page}.png；空态/错误态代码层修复见 4.1，截图未单独出（空库无空态可截，留人工豁免记录） |
| 宽屏布局零变化（1440/1920 两档） | 人工：verification/wide-after/ 8 张 + baseline/ 对照 + DOM 断言（常驻侧栏可见/抽屉未挂载/无 is-narrow） |
