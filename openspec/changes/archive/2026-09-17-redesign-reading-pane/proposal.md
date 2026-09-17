<!-- complexity: simple -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading -->

# Redesign Reading Pane：右侧文章主体克制阅读版式重排

## Why

阅读页右侧文章主体当前是「一摞卡片」堆叠：处理状态横幅、AI 摘要卡、简介卡、正文卡层层带边框+投影，正文列 `max-width: none` 在宽屏下通栏铺开（一行 100+ 字符），标题 2.9rem 霸屏，feed 徽章用 feed 原色染文字（紫色胶囊）抢视觉。用户反馈阅读主体「别扭、简陋」：扫描线太长无阅读重心、附属组件压过正文、纯色背景层次单调。需要在不动信息架构语义（哪些块存在）的前提下，重排为「克制阅读版式」——正文收窄居中、去卡片化、状态收进工具栏。

## What Changes

- **版式骨架**：正文列收窄至 ~680px（65–75ch）居中；元信息、标题、正文同列对齐；标题尺寸降级（约 1.6–2.1rem）；图片/表格/代码块允许略宽 breakout 形成节奏。
- **去卡片化**：正文外框、AI 摘要卡、简介卡的边框+投影移除，改用留白 + 细分隔线分区；引用块去大色块（轻左边线）；表格限宽可横滚。
- **阅读背景**：主体滚动区从纯色平铺改为极淡暖色微渐变（纸感），正文列与背景仅一丝层次差。
- **工具栏收编状态**：feed 徽章去彩色文字（图标 + 灰色次要文字）；抓取/总结处理状态收进工具栏小图标（进行中旋转 / 完成 ✓ / 失败 ⚠ + tooltip 详情），「手动抓取全文 / 生成总结 / 手动打标」收进「⋯」下拉菜单；顶部状态横幅与手动操作按钮行移除。
- **简介块收敛**：`shouldShowArticleDescription` guard 收紧（纯图/近空文本不渲染）；有实质内容时降级为标题下浅色导语段（无边框，同列宽）。
- 处理详情的三行状态（detailLines）随状态图标 tooltip 呈现，不再占正文上方空间。

## Capabilities

### New Capabilities

- `reading-article-pane`：阅读页右侧文章主体区（预览模式）的版式契约——阅读列宽与对齐、附属信息（状态/摘要/简介/标签）的呈现层级、正文内元素（引用/表格/代码/图片）样式基准、工具栏状态图标与操作菜单、简介 guard 规则。

### Modified Capabilities

- （无——中间栏列表、工具栏既有按钮语义、处理链行为均不变；`reading-list-panel` 不涉及本次改动）

## Impact

- **前端**（全部在 `front/`）：
  - `front/app/features/articles/components/ArticleContentPreviewPanel.vue`（结构重排：状态横幅移除、简介降级）
  - `front/app/features/articles/components/ArticleContentToolbar.vue`（状态图标 + ⋯ 菜单、feed 徽章降色）
  - `front/app/components/article/ArticleContent.css`（版式骨架重写：阅读列宽、去卡片化、背景、正文元素样式）
  - `front/app/utils/articleContentGuards.ts`（description guard 收紧）
  - `front/app/features/articles/components/ArticleContentView.vue`（previewProps 传参随面板结构微调）
- **纯展示层改动，无后端、无 API、无数据模型变化**；iframe 模式与全屏模式复用同一工具栏，行为不新增。
- 新增一个下拉菜单组件行为（⋯ 菜单），复用 unified-dialog/ui 既有主题 token，不引入新依赖。
