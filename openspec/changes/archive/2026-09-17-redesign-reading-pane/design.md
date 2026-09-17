# Design — redesign-reading-pane

## Context

现状实现：右侧文章主体 = `ArticleContentView.vue`（普通/全屏两模式，全屏 Teleport 复制渲染树）→ `ArticleContentToolbar.vue`（工具栏）+ `ArticleContentPreviewPanel.vue`（25+ props 的展示面板，含处理状态横幅、内容源切换卡、AI 摘要卡、简介卡）→ 样式全在 `front/app/components/article/ArticleContent.css`。状态与手动操作逻辑在 `useArticleContentView.ts` composable，description guard 在 `front/app/utils/articleContentGuards.ts`。

关键约束（pin 已存档）：**ArticleContent.css 是共享样式文件**——tags 域 QAPanel / CausalAnalysisReport / BoardEnrichmentPanel 借用 `.markdown-body` 全局类渲染 markdown 且各自 scoped 覆盖字号。

已批准合同：ui-design.md（approval=approved）——reader 760px 列、去卡片化、宋体标题、红 kicker、暖米渐变无噪点、状态收工具栏、⋯ 菜单。

## Goals / Non-Goals

**Goals**
- 阅读列版式、编辑排版语言、工具栏状态/菜单三件事按合同落地；
- 阅读页改样式不波及 tags 面板（作用域隔离）；
- 组件结构收敛：PreviewPanel 减 props，新增一个状态/菜单组件。

**Non-Goals**
- 不改 FeedLayoutShell 三栏骨架与中间栏列表；
- 不改 iframe 模式行为（工具栏形态变化自然作用于它，无新增行为）；
- 不改处理链数据流（firecrawl/summary/tagging 的 store 与 API 调用全部复用）；
- 不引入外部字体/图标依赖（宋体走系统字体栈，SVG 图标用 @iconify/vue 既有 mdi 集，新增图标须重生成 iconify 子集——见红线）。

## Decisions

### D1 样式作用域：阅读版式新规则限定在 `.preview-mode` 上下文

`.markdown-body` 基础元素排版保持不动（tags 面板依赖）；blockquote 去色块、宋体 h2/h3、表格/图片 breakout 负边距等新规则一律以 `.preview-mode` 前缀或 `.markdown-article`/`.markdown-summary` 限定作用域。**理由**：breakout 负边距若进全局 `.markdown-body` 会击穿 tags 面板紧凑布局；blockquote 全局去色块也会未经批准地改变其他面板观感。**替代方案**（否决）：把 .markdown-body 拆成独立共享文件——牵连 3 个 tags 组件 import 路径，收益仅是「可读性」，超出本 change 范围。

### D2 阅读列实现：`.reading-col` 包裹层 + CSS 变量

PreviewPanel 模板加一层 `<div class="reading-col">`（max-width 760px，margin auto），列内所有区块不再自算 `width: calc(100% - 3rem)`。breakout 用 `margin-inline: calc(-1 * var(--breakout-extra)/2)`，`--breakout-extra: 160px`，媒体查询窄于 900px 归零。**理由**：旧 CSS 的 `calc(100% - 3rem)` × 6 处散落是「卡片贴贴纸」观感的技术根源，收敛到一个变量。

### D3 新组件 ArticleStatusMenu（工具栏内嵌）

新建 `ArticleStatusMenu.vue`，含：状态图标（四态，复用 `useArticleProcessingStatus` 的状态元数据映射，与 reading-list-panel 同语义）+ 「处理详情」浮层；⋯ 菜单（手动抓取/生成总结/手动打标签 + 内容源切换分段控件）。浮层用轻量覆盖式定位（absolute 锚定工具栏 + 点击外部/Esc 关闭），**不走 AppDialog**（非模态场景，弹窗契约管的是模态框）。挂在 `ArticleContentToolbar.vue` 右侧，普通/全屏/iframe 三处工具栏自然继承。**理由**：状态逻辑已在 composable，组件只是呈现；单独成组件避免 PreviewPanel 再膨胀、Toolbar 保持哑组件。Props 从 `useArticleContentView` 直取（Toolbar 由 ArticleContentView 渲染，不必经 PreviewPanel 转发）。

### D4 PreviewPanel 结构瘦身

删除：处理状态横幅整块、手动按钮行、内容源切换卡（迁入 D3 菜单）、描述卡边框（降级为 `.lede`）。保留：元信息/标题/标签行/导语/AI 整理稿/正文。对应 props 从 25+ 收敛（状态/手动操作相关 props 全部迁给 ArticleStatusMenu）。AI 整理稿内标签列表与关注开关不动。

### D5 description guard 收紧（utils 层，可单测）

`articleContentGuards.ts` 的 `normalizeText` 增加：剥离纯图片 markdown/HTML 后为空 → 视为无内容。规则：normalize 后长度 < 4（近空/纯符号）→ 不展示；保留既有「与正文重复」判定。**理由**：guard 是纯函数，加边界用例到 `articleContentGuards` 对应测试文件即可，无需组件层测空卡。

### D6 背景与排版 token 落点

- 渐变：`.preview-mode` 背景改三段纵向渐变（奶油→米白），色值用现有 token 组合近似（editorial 主题 `--color-bg-base` 系），不新增主题 token（暗色主题下靠现有 token 自动成立，渐变 stop 全部引用语义变量）；
- 宋体标题：CSS 变量 `--font-serif-display`（系统栈：Noto Serif SC → Source Han Serif SC → Songti SC → SimSun），加进 `.preview-mode` 局部作用域，不进全局主题（避免影响其他页面标题）；
- 红 kicker/短粗线：直接用 `--color-accent`（红色即主题 accent），无新 token。

### D7 全屏模式不重复实现

全屏 Teleport 分支继续复用同一 PreviewPanel/Toolbar（现状结构），样式类不变即自动生效；全屏滚动容器背景需同步 `.preview-mode` 渐变（`.fullscreen-article` 内同规则）。

## Risks / Trade-offs

- [breakout 负边距在极窄面板溢出] → 媒体查询（<900px）归零 + 面板 min-width 保障；spec 已定「面板窄于 760 取可用宽」。
- [宋体栈在无中文字体环境回退难看] → 栈尾兜底 serif，用户环境（Debian/树莓派 + 常见桌面）有 Noto Serif SC 覆盖；如缺字体回退系统 serif，可读性不损失。
- [tags 面板样式意外回归] → D1 作用域隔离 + 实现 tasks 里加「QAPanel/BoardEnrichmentPanel 视觉无变化」人工验证项。
- [PreviewPanel props 收敛引发 ArticleContentView 传参断裂] → TypeScript 编译期兜底（props 接口显式声明），typecheck 即拦。
- [滚动进度条/返回顶部按钮依赖 `.preview-mode` 容器] → 容器类名与 ref 保持不变，只改内部结构。

## Migration Plan

纯前端展示层，无数据迁移。部署后：阅读页即时呈现新版式；无配置项、无开关；回滚 = revert 前端构建。旧偏好/处理链行为零变化。

## Open Questions

无（浮层内「标签」行是否显示标签名列表——合同已定「计数级信息」，与 reading-list-panel 一致）。
