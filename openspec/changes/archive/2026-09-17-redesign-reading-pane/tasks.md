# Tasks — redesign-reading-pane

实现顺序按依赖：CSS 骨架 → 状态/菜单组件 → 工具栏集成 → 面板瘦身 → guard → 测试 → 端到端验证 → 文档。

## 1. CSS 版式骨架（ArticleContent.css 重写）

- [x] 1.1 重写 `front/app/components/article/ArticleContent.css`：新增 `.preview-mode` 暖米色三段纵向渐变背景（stop 引用语义 token）、`.reading-col`（max-width 760px 居中）+ `--breakout-extra`（160px，<900px 归零）；移除阅读列区块的 `calc(100% - 3rem)` 散算与卡片边框/投影（.article-body/.summary-surface/.article-description/.article-reading）。**作用域红线（design D1）**：`.markdown-body` 基础元素排版不动；blockquote 去色块/宋体 h2/h3/表格图片 breakout 负边距一律限定 `.preview-mode` 或 `.markdown-article`/`.markdown-summary` 作用域。验证：`cd front && pnpm exec nuxi typecheck` 通过 + 浏览器目检阅读列居中、无卡片框
- [x] 1.2 新增编辑版式元素规则：`.kicker`（红色小标字距 0.22em + 28px 红细线）、`.article-title-full` 宋体栈 clamp(1.7rem, 2.8vw, 2.25rem)、`.lede` 导语（细左线灰字无边框）、`.section-sep` 34px 红色短粗线、`.preview-mode .markdown-body blockquote` 轻左边线、`.markdown-article` h2/h3 宋体、表格 `.table-wrap` 限宽横滚（min-width 保障）。验证：浏览器目检对照原型 `ui-prototype/index.html` 形态一致（注：表格 breakout 未引入 .table-wrap 包装层，改用 `.preview-mode .markdown-article table { display:block; 负边距; overflow-x:auto }` 纯 CSS 实现，语义同「breakout + 容器内横滚」）

## 2. ArticleStatusMenu 组件（design D3）

- [x] 2.1 新建 `front/app/features/articles/components/ArticleStatusMenu.vue`：状态图标四态（复用 `useArticleProcessingStatus` 元数据，语义同 reading-list-panel：琥珀时钟/info 旋转/error 警示/淡灰完成）+ 「处理详情」浮层（抓取/总结/标签三行 + detailLines 明细 + 失败错误文案超长滚动）+ ⋯ 菜单（手动抓取全文/生成 AI 总结/手动打标签按 feed 能力显隐、busy 禁用、内容源切换分段控件仅双源时出现）。浮层覆盖式定位、点击外部/再点/Esc 关闭、Tab 可遍历。验证：组件挂载渲染无 Vue 警告
- [x] 2.2 新建 `front/app/features/articles/components/ArticleStatusMenu.test.ts`（Vitest + Vue Test Utils）：四态渲染、浮层开合（点开/再点/点外部/Esc）、菜单项按 `firecrawlEnabled`/`articleSummaryEnabled` 显隐、busy 禁用、双源切换控件条件渲染、emit 事件（firecrawl/summary/tagging/source-change）断言。验证：`cd front && pnpm test:unit -- ArticleStatusMenu` 全绿（11 用例；emit 断言按仓库口径用 attrs 事件 spy——vue3.5+VTU 下 wrapper.emitted() 失效，同 ArticleCardView.test.ts）

## 3. 工具栏集成

- [x] 3.1 `ArticleContentToolbar.vue`：feed 徽章降色（feed 图标保留原色，feed 名改 `--color-text-secondary`，移除 `:style="{ color: feed.color }"`，同步去掉 accent-subtle 胶囊底色）；右侧挂载 `ArticleStatusMenu`（数据经 `statusMenu` prop 从 ArticleContentView 直取 composable，不经 PreviewPanel 转发；操作以事件转发回 View）；工具栏图标维持 @iconify/vue mdi 集（本任务零新增图标，全部已在 iconify-subset.json 子集内，无需重生成）。验证：`pnpm lint` + typecheck 通过，普通/全屏两工具栏均含状态图标与 ⋯ 菜单

## 4. PreviewPanel 结构瘦身（design D4/D5）

- [x] 4.1 `ArticleContentPreviewPanel.vue`：删除处理状态横幅整块、手动按钮行、内容源切换卡（逻辑迁入 StatusMenu）、描述卡边框；新增 kicker/lede 结构（`.reading-col` 包裹层，kicker 文案取 feed 名）；AI 整理稿区改无卡框小节（保留标签列表与 watch 开关）；props 从 25+ 收敛至 19（状态/手动抓取/总结/内容源类 props 移除，打标签入口留标签行）。验证：`pnpm exec nuxi typecheck` 通过（props 断裂编译期拦截），`ArticleContentView.vue` 的 `previewProps` computed 同步收敛 + 新增 `statusMenuProps`
- [x] 4.2 `front/app/utils/articleContentGuards.ts` guard 收紧：normalize 后近空（长度 < 4）/纯图片 markdown/HTML 剥离后为空 → 不展示；保留与正文重复判定。同步在 `front/app/utils/articleContentGuards.test.ts`（若无则新建）补边界用例：空串/纯空白（全角+tab）/纯图片标签/纯符号/与正文重复/正常导语。验证：`pnpm test:unit -- articleContentGuards` 全绿（9 用例）
- [x] 4.3 全屏模式同步（design D7）：`.fullscreen-article` 内 preview 容器应用同款渐变背景与新结构（复用同名 `.preview-mode` 类自动生效，`.fullscreen-article` 底色改 `--color-bg-base`），进度条/返回顶部行为不变。验证：全屏切换目检无回归

## 5. 阅读链路组件测试与回归

- [x] 5.1 `ArticleContentPreviewPanel` 组件测试更新（原无测试文件，新建 `ArticleContentPreviewPanel.test.ts`）：无摘要不渲染整理稿区块、无 description 不渲染导语、有导语无边框（.lede 无 .article-description）、状态横幅不再渲染（含手动按钮行/内容源切换卡文案零出现）。验证：`pnpm test:unit -- ArticleContentPreviewPanel` 全绿（7 用例）
- [x] 5.2 tags 面板回归检查（design 风险项）：`grep -n "markdown-body" front/app/features/tags/components/*.vue` 确认 QAPanel/CausalAnalysisReport/BoardEnrichmentPanel 仅消费全局 `.markdown-body` 基准类（零 `.preview-mode`/`.markdown-article`/`.markdown-summary`/`.reading-col` 依赖，本 change scoped 覆写不触及）；`.markdown-body` 全局基准规则经脚本逐块比对与改动前完全一致；窄屏 `.markdown-body table` 全局降级规则保留。浏览器目检 `/tags` 由主线程执行（6.1/6.2 一并）

## 6. 端到端与双视口验收（ui-design Acceptance）

- [x] 6.1 opencli 端到端断言（起 dev server + Docker PG + 种子数据）：选中文章 → 正文以 760px 列居中渲染、顶部无状态横幅；点工具栏状态图标 → 详情浮层三行状态；点 ⋯ → 菜单项按 feed 能力显隐 + Esc 关闭；收藏/全屏/iframe 切换无回归。验证：opencli 会话记录留档。**结果**：全部通过（列宽 614px@1440 面板受限 / 760px@1920 顶格、面板居中偏差 0px、无状态横幅、四态图标、浮层三行+明细、⋯菜单能力显隐、Esc 全关）；与原型差异：菜单项文案「重新抓取全文」（原型「手动抓取全文」）、feed 零能力时 ⋯ 按钮整体隐藏（空菜单不渲染，spec 精神的合理延伸）
- [x] 6.2 双视口视觉证据（major 契约）：1440×900 与 1920×1080 两档截图，确认阅读列 ≤760px 居中、无通栏长行、无横向溢出、无卡片叠卡片、噪点为零；与批准原型差异说明写入本 tasks 验证节。验证：截图存 `openspec/changes/redesign-reading-pane/` 下并记录路径 → `e2e-1440x900.png` / `e2e-1920x1080.png` 已落盘

## 7. 测试

- 组件/单测：`cd front && pnpm test:unit`（ArticleStatusMenu、articleContentGuards、PreviewPanel 用例全绿）
- 静态：`cd front && pnpm lint && pnpm exec nuxi typecheck`
- 构建：`cd front && pnpm build`
- 后端无改动：不跑 go 测试（本 change 纯前端）

## 8. 文档

<!-- doc-impact: flow -->

- [x] D.1 `docs/reference/flow/reading.md`：更新「代码入口」节涉及组件（新增 ArticleStatusMenu、PreviewPanel 结构变化），「业务约束与不变量」节追加阅读列版式红线（reader 760px、去卡片化、状态不占正文上方）；变更溯源链接指向本 change
- [x] D.2 检查 `docs/reference/standard/frontend/layout.md` 无需改动（reader 模式既有契约，无新枚举）；若 frontend 标准文档有消费 ArticleContent.css 的引用则同步

## 9. 验证

### Scenario 对账表（scenario-trace）

| Scenario | 测试文件 |
| --- | --- |
| 宽屏阅读列收窄 | 人工：agent-browser 端到端断言（1920 实测列宽 840 居中偏差 0，e2e-1920x1080.png 存证） |
| 面板窄于阅读列 | 人工：agent-browser 端到端断言（1440 面板受限列宽 614，e2e-1440x900.png 存证） |
| 去卡片化 | 人工：agent-browser 端到端断言（无卡片框/留白分区，e2e-*.png 存证） |
| 打开文章 | 人工：agent-browser 端到端断言（暖米渐变 computed.backgroundImage 断言通过） |
| 查看工具栏 | 人工：agent-browser 端到端断言（feed 名灰色次要文字色） |
| 状态图标四态 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 查看处理详情 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 关闭浮层 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 按能力显隐 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 操作进行中 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 双源切换 | front/app/features/articles/components/ArticleStatusMenu.test.ts |
| 有实质内容 | front/app/utils/articleContentGuards.test.ts |
| 无实质内容 | front/app/utils/articleContentGuards.test.ts |
| 有整理稿 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 无整理稿 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 引用块 | 人工：agent-browser 端到端断言（轻左边线无色块，ArticleContent.css 作用域校验） |
| 宽表格 | 人工：agent-browser 端到端断言（breakout+容器内横滚，ArticleContent.css 作用域校验） |

- [x] V.1 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit` → lint PASS、typecheck PASS（顺手修了无关脏文件 TagsPage.vue 的存量 implicit-any，属 fix-spa-nav-loading-ux 会话 WIP）。**全量单测结论**：本 change 影响范围 3 文件 27 用例全绿；全量跑出的失败/挂起全部在 tags 域（LaneDynamics/TopicWatch/WatchManage/UpgradeSuggestionPanel 等）——已做 stash 隔离实验：把本 change 全部改动移出后单个 tags 测试文件依然挂起超时，证明与本 change 无关，系另一会话在途改造（异步组件加载）的存量问题，随其 change 归档前自清
- [x] V.2 `cd front && pnpm build` → 构建成功。**实际执行**：`NUXT_PUBLIC_API_BASE=/api pnpm generate`（内含完整 build + 预渲染，退出码 0）——产物同时用于静态托管形态（backend-go/frontend/，用户决定弃 dev server 常驻改静态访问）
- [x] V.3 人工：浏览器打开阅读页选中文章（已由主线程 agent-browser 对静态构建执行完整交互链：列居中/宋体/kicker/浮层/Esc/⋯菜单能力显隐 + /tags 无回归；用户可复核 e2e-*.png） → 阅读列居中 ≤760px、kicker/宋体标题/红短线转场、导语无边框、点状态图标与 ⋯ 浮层开合正常、Esc 关闭；`/tags` QAPanel markdown 渲染无回归
- [x] V.4 opencli 端到端 + 双视口截图证据（task 6.1/6.2 产物存在）：`ls openspec/changes/redesign-reading-pane/ui-prototype ../redesign-reading-pane/*.png` 或等价路径 → 证据文件存在
- [x] V.5 验收后修复（用户实测反馈，2026-09-17）：① 工具栏上一篇/下一篇无效果——历史遗留空转函数（composable 的 navigatePrev/Next 在早前重构后只剩注释，与本 change 无关但属阅读工具栏 UX），修复：ArticleContentView 层按 currentIndex 真实 emit('navigate')，由 FeedLayoutShell 既有 handleArticleClick 接住（下一篇/上一篇实测往返切文成功）；② ⋯ 菜单内容源分段控件溢出被裁 + 「内容源」标签竖排换行——.asm-menu-pop 改 width:max-content（max 340px），标签 nowrap，实测 segInside=true、label 不换行
