<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/reader.html -->

## User Journey

- **入口**：TagsPage → 日报阅读层（全屏杂志层），与现状一致，无新导航入口。
- **主任务**：读报遇到不懂的术语/段落（如「央行逆回购」）→ 划选文本 → 选区上方浮出「问一问」气泡 → 点击落锚（正文高亮 + 右侧页边注栏出现锚点卡）→ 输入问题 → 获得带引用的解答（通用概念 + 当天文章上下文）→ 继续读。
- **次任务**：回看已有批注（右栏「↗ 跳回原文」按钮跳回原文并闪现；引用摘要点击展开全文）；对已有问答追问（卡片内继续输入）；删除批注（✕ 常显）；翻旧日报时看到当时的问答；在设置「页边注」分区统一管理全部批注（筛选/跳转/删除）。

## Information Architecture

- `drm-layout` 两列 → 三列：**左导航 14rem（现有不动）｜ 报纸正文 minmax(0,1fr)（新增批注高亮标记）｜ 页边注栏 clamp(15rem, 17vw, 17rem)（新）**。
- **全局管理页（新）**：设置工作台新增「页边注」分区（`?section=margin-notes` 深链，settings sections 数组注册）——跨报告列出全部批注：筛选栏（版块下拉 + 关键词搜索）+ 批注行（版块/日期/期号、术语 chips、划词引用、问答折叠展开、操作「↗ 原日报」「✕ 删除」）。
- 页边注栏 = **当前这份报告**的批注清单，按正文出现顺序排列；跨报告的术语聚合属 P2 术语库，不进本层。
- 批注锚点卡结构：引用文本摘要（2 行截断，点击跳原文）+ 问答轮列表（1:N）+ 问题输入框（首发问/追问复用）。
- 术语 chips 附在问答轮尾部（派生展示：「本次涉及 · 已入术语库」），P1 无独立术语页面。

## Interaction Contract

### 与现有收展/溯源交互的共存（冲突消解，红线）

日报阅读层现有交互**零变化**，且作为回归验收项进 specs：thread header 点击=展开/收起相关文章（溯源主入口，`ensureArticles` 预取不变）、topic header 点击=话题收展（active zone 附带 ensureLifeline）、离群 thread 折叠/watch badge 不变。冲突消解三规则：

1. **选区吞噬（selection guard）**：mouseup 产生有效选区（≥2 字符）后，同一点击序列的 click 在 capture 阶段被吞（stopPropagation + preventDefault），thread/topic toggle 不触发；选区清除后恢复正常点击。划选与点击互不误伤。
2. **mark 点击分层**：落锚高亮 `mark` 挂自己的 click（跳右栏对应卡并闪现）并 stopPropagation 阻断 toggle；mark 之外的 header 区域维持原有展开行为。mark 为非交互流内容，嵌 button 内合法。
3. **批注范围限定叙事文本**：头条 lead 摘要（纯 `<p>`，无冲突）与 thread 摘要开放划选；topic/zone header 是纯交互控件不开放批注。实现层若需把 thread 摘要移出 button（结构等价重构），MUST 保持视觉与「点标题/摘要任意处展开」行为完全不变并在 design.md 记录。

### 主操作与新交互

- **主操作**：划选正文 → 气泡「问一问」→ 点击落锚（高亮淡入 + 右栏新卡 + 输入聚焦）→ 提交问题 → 生成中（骨架呼吸）→ 回答 + 引用 chips + 术语 chips。
- **次操作**：锚点卡引用摘要点击 → 展开/收起全文（长划词可完整阅读，默认两行截断）；**折叠态必须带独立、常显的「⌄ 展开全文」提示行（2026-09-22 用户批准反馈：不能只靠截断文字暗示可点，不得把提示藏在 line-clamp 盒内被截断吃掉）**；「↗ 跳回原文」按钮 → 正文对应段滚动闪现；追问提交；引用 chip 点击 → 走日报现有文章打开通道（`ensureArticles` → 文章预览，与 thread 展开同源同链路）；高亮段点击 → 右栏对应卡高亮呼应；✕ 删除常显（低调，悬停强调）。
- **管理页操作**：版块下拉/关键词搜索筛选（命中划词、提问、术语）；「↗ 原日报」→ 打开 TagsPage 日报层并定位到报告与锚点（高亮闪现）；「✕ 删除」与日报内删除同源同确认（连带问答）。
- **危险操作**：删除批注 = 连带其全部问答，AppDialog sm 确认；删除后正文高亮同步移除，不可恢复（确认弹窗明示）。
- **反馈**：落锚淡入动画；生成中骨架；失败行内错误 + 重试（锚点与已输入问题保留不丢）；术语入库极轻角标，不打断阅读。

## State Matrix

| 状态 | 页边注栏 | QA 生成 | 恢复路径 |
| --- | --- | --- | --- |
| 进入报告加载批注 | 骨架行 ×N | — | 失败整栏重试 |
| 无批注 | 极轻引导「划选正文即可批注提问」 | — | 划选即创建 |
| 生成中 | 卡片正常 | 输入锁定 + 骨架 | 超时/失败 → 行内错误 |
| 生成失败 | 卡片正常 | 行内错误 + 重试按钮 | 重试不重发已成功轮 |
| 生成成功 | 卡片正常 | 回答 + 引用 chips + 术语 chips | — |
| 删除确认中 | 卡片半透明 | — | 取消恢复 / 确认删除 |
| **管理页无匹配** | 空态文案 | — | 清空筛选恢复 |
| **管理页删除中** | 行半透明 + 确认弹窗 | — | 取消恢复 / 确认后行移除计数刷新 |
| 引用文章打开失败 | — | 沿用现有文章预览错误路径 | 现有重试机制 |
| **划选 vs 收展冲突** | — | selection guard 吞 click，toggle 不触发 | 现有溯源/收展交互零回归（opencli 断言） |

## Layout Contract

- 日报阅读层维持**全屏沉浸层**定位（现状，非 AppPageShell 标准页），以 **workspace 语义**记录：填满可用宽度、双侧栏常驻。
- **管理页**：设置工作台内，沿用 **contained=1120** 居中（设置类页面默认模式，不随父容器拉宽），目标视口同桌面双档。
- 三列：`14rem ｜ minmax(0, 1fr) ｜ clamp(15rem, 17vw, 17rem)`；列间距沿用 `clamp(2rem, 3vw, 2.75rem)`；正文栏溢出随页滚动，页边注栏内容超高时**栏内滚动**（sticky 定位）。
- 目标视口：**1440×900、1920×1080**（major 双档验收）。
- 窄屏降级（触及，另列 **375×667 / 390×844**）：<1100px 三列塌单列（沿用现有断点），页边注栏收为**右下浮动批注按钮 + 右侧抽屉**（复用 `AppSidebarDrawer` 右侧形态）；正文批注保留高亮，点击高亮打开抽屉中对应卡。
- 无新增 dialog 档位：删除确认 = `AppDialog sm=420`。

## Component Reuse

- **复用**：`AppButton`（提问/重试）、`AppInput`/textarea（问题输入）、`AppDialog` sm（删除确认）、`AppSidebarDrawer`（窄屏批注抽屉）、文章打开沿用日报现有 `ensureArticles` 通道与 `ArticleContentPreviewPanel`。
- **新组件**：`SelectionAskBubble`（划词气泡）、`MarginNotesRail`（页边注栏容器）、`MarginNoteCard`（批注卡 = 锚点 + QA 轮 + 输入 + 展开态 + ↗/✕）、`SettingsSectionMarginNotes`（管理页，注册进 `SettingsWorkspace` sections 数组：key `margin-notes`，深链兼容）。

## 行为核定（2026-09-22，任务 1.2）

- **结论：保守方案（design D2 主路径）在目标引擎上可用，不启用备选等价重构。**
- 方法：真实 app（dev 栈同源入口，TagsPage → 日报阅读层，真实 `BoardDailyReportTimeline.vue` DOM）+ agent-browser 无头 Chromium（HeadlessChrome/152）真实鼠标拖选；动态注入与实现一致的 `user-select: text`（`.drm-thread__header` 及后代）与 document capture 阶段 guard（选区 trim 后 ≥2 字符 → stopPropagation + preventDefault）。
- 断言结果：
  - **RG-2 划选不误展开**：在 `<button class="drm-thread__header">` 内真实拖选得 79 字符有效选区（user-select:text 生效，Chromium 允许 button 内选区），拖选结束产生的 click 被 capture guard 吞掉（guardHits=1），toggle 处理器未执行、`aria-expanded` 保持 `false`、文章列表未展开。
  - **RG-1/RG-3 纯点击正常展开**：`removeAllRanges()` 清选区后点击行头，guard 未吞（guardHits 不变），toggle 正常执行、`aria-expanded` 翻转、溯源文章列表展开——行为与引入批注前一致。
  - **WB-1 边界**：5px 微拖（选区塌缩为空）guard 不吞（guardHits=0），点击按原有 toggle 语义执行。
- 附带事实：拖选结束（mouseup）在 button 上**确实会**产生一次 click 事件（guardHits=1 证明其到达 document capture 阶段），即「不设 guard 则拖选必误触 toggle」的前提成立，guard 为必要消解手段。
- 遗留：Firefox / Safari 的 button 内选区行为差异留 5.1 端到端手测复核（本机仅有 Chromium）；若届时不可用，按 D2 备选路径记录后重构。

## Prototype

- 三文件静态原型（`proto.css` 共享样式 + fixture 数据）：**`reader.html` 日报页边注、`manage.html` 设置·页边注管理页**，顶栏链接互跳（manage「↗ 原日报」→ `reader.html#hl-demo` 深链闪现）；不连接真实 API、不修改产品代码。管理页 fixture 与阅读视图独立（真实实现共数据源）。（2026-09-22 拆分：原单文件双视图 hash 路由方案用户反馈体验崩坏，拆为两页一 CSS）
- fixture：版块「中国宏观」第 128 期，头条与「利率与流动性」section 含「央行逆回购」thread，其中一段**预置已完成批注**（含回答、引用、术语 chips、追问示例），供直接看到终态。
- 交互演示：划选正文任意文本 → 气泡 → 落锚 → 输入 → 模拟生成（假延迟 + 骨架）→ 回答；锚点跳转闪现、引用 chip 展开、删除确认、<1100px 窄屏浮动按钮 + 抽屉形态。
- **共存演示**：thread header 可点击展开/收起相关文章列表（现有溯源交互，含文章行可点）；selection guard 已实现——划选结束时 thread 不误展开；点击高亮 mark 跳右栏卡、不触发 thread 展开。
- 审批记录：2026-09-21 用户批准原型（含交互共存方案）；同日 IA 修订（新增管理页视图等）approval 重置 pending。**2026-09-22 用户重批通过（拆分双页后）：①单文件双视图 hash 路由拆为 reader.html / manage.html + 共享 proto.css；②修复预置高亮点击不亮卡（mark→card 反查 data-jump）；③引用折叠态新增独立「⌄ 展开全文」提示行（原 ::after 提示被 line-clamp 截断盒吃掉，用户反馈「不能全是文字，要留足展开提示空间」）——真实实现必须遵守同一要求。**
- 与真实接口解耦：所有数据硬编码，AI 生成过程为 `setTimeout` 模拟。

## Acceptance

- 实现后补：opencli 主链路断言（划选 → 气泡 → 落锚 → 提问 → 回答渲染 → 引用打开 → 追问 → 删除确认；管理页筛选 → 跳原日报定位 → 删除）+ **现有交互零回归断言（thread 点击展开/收起相关文章、topic 点击收展、划选不误触发 toggle、mark 点击跳卡不 toggle）** + **1440×900 / 1920×1080** 视觉检查证据 + **375×667 / 390×844** 窄屏降级检查 + 与批准原型的差异说明（重大差异需重新审批）。

## 实现与批准原型的差异说明（2026-09-24）

| # | 差异 | 来源 | 判定 |
| --- | --- | --- | --- |
| 1 | 页边注栏锚定/定位：原型 `.rail{position:sticky;top:3rem;max-height:calc(100vh-4rem)}`；实现改为「外层 sticky 视口居中 + 内层滚动容器」——`top:4.25rem`（让开阅读层 `.drm-toolbar` 3.5rem）、`height:calc(100vh - 4.25rem - 0.75rem)`、外层 flex 居中、内层 `max-height:100%` 滚动 | 用户 2026-09-24 反馈：原实现 `top:1rem` 被工具条压住且卡片顶格，「可以居中然后自适应高度」 | 同一元素的排布细化，无结构/信息架构变化 → **不触发重审**（记录备查） |
| 2 | 删除确认弹窗 z-index 由默认 1000 → 9100 | 走查发现弹窗被阅读层 overlay（z=9000）盖住、按钮不可点 | 修复性对齐既有约定（`ArticlePreviewModal` 同用 9100） |
| 3 | 落锚失败新增栏内行内提示「落锚失败：…（请重新划选文字）」 | 走查发现 `anchorError` 无消费方、失败静默（= 用户报障「点击没反应」之一） | 补错误态（原型的演示数据无失败分支） |

- 差异 1 的双视口实测（agent-browser，1440×900，`report=896` 4 条批注）：滚动锁定后栏框 top=68px / bottom=888px、工具条底 56px → 间距 12px，内容超出时栏内滚动；空态报告（4 条批注=0）实测 scroller 上下留白各 319px（居中）。

## 联网来源 chips 增补（2026-09-24，SearXNG 联网扩充）

> 扩充背景与三决策（触发策略/落点/沉淀方式）见 design.md D7；本节只记 UI 面。**用户已于当日会话批准增补布局**（问答选项预览即示意稿），属已批准原型的增量演进，不重置整体审批。

### 增补内容

1. **批注卡问答轮新增「网络来源」行**：本地引用文章 chips 之下分行展示（不混排）；每个 chip = 标题/域名 + 外链标识（`↗`），整 chip 可点，`target=_blank rel=noopener` 新窗口打开原网页；视觉复用现有 chips 体系（同圆角/边框/字级 token），仅加外链图标与悬停下划线区分站内/站外。
2. **「纯模型知识」标注口径更新**：本地文章引用**且**网络来源**均为空**才显示标注；仅有网络来源不标。
3. **管理页问答轮折叠内同步**展示网络来源 chips（同一数据源 `cited_web_sources`）。
4. **设置工作台新增「SearXNG 搜索」分区**（key `searxng`，图标 `mdi:web` 档；对齐「博查搜索」section 的表单结构：endpoint + enabled 开关，保存即生效）。
5. **原型增补**：`ui-prototype/reader.html` 预置批注 fixture 的回答尾部补一组网络来源 chips 示例（静态演示，不接真实搜索）——实现验收时对齐此形态。

### 不变项

- 双视口验收（1440×900 / 1920×1080 + 375×667 / 390×844 窄屏抽屉）沿用现有 Acceptance，不因 chips 增补重跑全量——chips 在既有问答轮容器内流式排列，无新增布局分支。
- 搜索失败静默降级无独立 UI 态（无错误提示、无骨架差异）——与现状体验完全一致，故无新增 State Matrix 行。
