# reading-article-pane Specification

## Purpose

阅读页右侧文章主体区（预览模式）的能力契约：reader 阅读列版式（840px 上限、去卡片化、宋体标题+红 kicker 编辑签名、标题/图片/表格 breakout）、暖米渐变无纹理背景、工具栏降噪（feed 名灰色、处理状态单图标四态+详情浮层、手动操作与内容源切换收 ⋯ 菜单按 feed 能力显隐）、description 导语 guard、正文元素编辑版式基准。来源于 change redesign-reading-pane（2026-09-17）。

## Requirements

### Requirement: 文章主体采用 reader 阅读列版式

阅读页右侧文章主体区（预览模式）MUST 以单列居中阅读版式呈现文章：正文、元信息、标签、导语、AI 整理稿同列对齐（标题与图片/表格允许 breakout 略宽于列——2026-09-17 用户反馈标题受限于列内显拘谨，改为随 breakout 机制扩展），列宽 MUST NOT 超过 840px（reader 语义放宽档，2026-09-17 用户超宽屏反馈后从 760 上调，约 50 汉字/行的舒适上限），面板窄于该宽度时取面板可用宽度。列内元素 MUST NOT 以带边框+投影的独立卡片呈现（去卡片化）；分区 MUST 依赖留白与细分隔线。文章标题 MUST 使用衬线字体栈（Noto Serif SC / 宋体栈）且基准尺寸在 1.7–2.25rem 区间，MUST NOT 恢复至旧版 clamp(2rem, 4vw, 2.9rem) 的通栏大标题形态。标题上方 MUST 呈现红色 kicker 小标（字距加宽）+ 短红色细线（编辑签名元素）。

#### Scenario: 宽屏阅读列收窄

- **WHEN** 用户在宽屏（≥1440px）打开选中文章的预览模式
- **THEN** 正文列宽不超过 840px 且居中，正文行文长度与列宽一致，无通栏长行

#### Scenario: 面板窄于阅读列

- **WHEN** 右侧面板可用宽度小于 840px（含分栏拖窄、窄视口）
- **THEN** 阅读列取面板可用宽度，内容不横向溢出

#### Scenario: 去卡片化

- **WHEN** 文章带 AI 整理稿或简介导语
- **THEN** 各信息块之间以留白与分隔线区分，不出现带边框、投影的层叠卡片

### Requirement: 主体背景为暖米色微渐变、无纹理

文章主体滚动区背景 MUST 为暖米色纵向微渐变（奶油色系到米白），MUST NOT 叠加噪点/纹理层（2026-09 用户对比试看后已否决噪点方案）。背景 MUST NOT 使用与列表侧 chrome 相同的单一纯色平铺。

#### Scenario: 打开文章

- **WHEN** 用户选中任一文章进入预览模式
- **THEN** 主体背景呈暖米色纵向渐变，正文区域无可见颗粒或纹理

### Requirement: 工具栏 feed 徽章降噪呈现

工具栏左侧 feed 徽章 MUST 保留 feed 图标原色，feed 名称文字 MUST 使用次要文本色（灰），MUST NOT 使用 feed 自定义颜色渲染文字。

#### Scenario: 查看工具栏

- **WHEN** 打开任一属于自定义颜色 feed 的文章
- **THEN** 工具栏 feed 名为灰色次要文字，仅 feed 图标携带原色

### Requirement: 处理状态以工具栏单图标加详情浮层呈现

文章处理链状态 MUST 收进工具栏一枚状态图标，四态语义与 reading-list-panel 一致：排队中（琥珀色时钟形态）、进行中（info 色旋转指示）、失败（error 色警示形态）、全部完成（淡灰完成形态）。正文滚动区 MUST NOT 出现处理状态横幅（整卡横幅、状态胶囊行、手动操作按钮行均不再位于正文上方）。点击状态图标 MUST 就地展开「处理详情」浮层，内容包含抓取、总结、标签三行状态；浮层内 MUST 包含抓取/总结时间与尝试次数等明细行（原 detailLines 信息）；失败时浮层内 MUST 附完整错误文案（超长滚动）。浮层 MUST 覆盖式定位（不挤动阅读流）；点击浮层外、再点图标或按 Esc MUST 关闭；同一时刻至多一个浮层打开。

#### Scenario: 状态图标四态

- **WHEN** 文章处理链分别为排队中/进行中/失败/全部完成
- **THEN** 工具栏状态图标呈对应四态形态，正文上方无状态横幅

#### Scenario: 查看处理详情

- **WHEN** 用户点击工具栏状态图标
- **THEN** 就地展开处理详情浮层，含抓取/总结/标签三行状态与时间明细，阅读流不被挤动

#### Scenario: 关闭浮层

- **WHEN** 浮层打开时用户点击浮层外部、再次点击图标或按 Esc
- **THEN** 浮层关闭，不遗留占位

### Requirement: 手动操作与内容源切换收进更多菜单

「手动抓取全文」「生成 AI 总结」「手动打标签」三个手动操作 MUST 收进工具栏「更多操作」（⋯）下拉菜单，菜单项按 feed 能力开关显隐（feed 未开 Firecrawl/总结时对应项不出现），操作进行中 MUST 呈禁用与加载指示，同一时刻至多一个手动操作。「内容源切换」（原始内容 / Firecrawl 全文）MUST 位于同一菜单内且仅在双源可用时出现。菜单 MUST 覆盖式定位；点击外部、再点按钮或按 Esc MUST 关闭；手动操作错误 MUST 以行内错误提示反馈，MUST NOT 恢复正文上方错误横幅。Esc MUST 关闭菜单且菜单项支持键盘 Tab 遍历。

选择「原始内容」源时，渲染前 MUST 对源站原文施加保守清洗（移除脚本、样式元素与事件处理属性；**视觉隐藏声明**（`visibility:hidden`/`opacity:0`/`font-size:0`）MUST 被剥离使内容重新可见；**语义不展示**（`display:none`/`hidden` 属性）MUST 连同子树移除），保留可见正文结构；清洗 MUST 在渲染路径生效，MUST NOT 依赖数据是否已回填。

#### Scenario: 按能力显隐

- **WHEN** feed 开启 Firecrawl 且开启 AI 总结
- **THEN** ⋯ 菜单含「手动抓取全文」「生成 AI 总结」项；未开启的能力对应项不出现

#### Scenario: 操作进行中

- **WHEN** 用户在 ⋯ 菜单触发任一手动操作
- **THEN** 菜单内该项呈加载/禁用状态，其他手动操作不可同时触发，正文阅读不受影响

#### Scenario: 双源切换

- **WHEN** 文章同时存在原始内容与 Firecrawl 全文
- **THEN** ⋯ 菜单内出现内容源切换分段控件，切换后正文即时切换数据源

#### Scenario: 原始内容源渲染前清洗

- **WHEN** 用户切到「原始内容」源，而该源内容为含内联样式、隐藏声明与事件属性的源站 HTML
- **THEN** 渲染结果 MUST 为清洗后的可见正文，视觉隐藏声明 MUST 被剥离（被藏起来的正文重新可见）、语义不展示的隐藏块 MUST 被剔除、事件属性 MUST NOT 被绑定

### Requirement: 简介导语段按实质内容呈现

文章导语 MUST 仅在有实质内容时以标题下导语段呈现（浅色、细左线、无边框）；为空、纯空白、纯图片、纯符号或与正文重复时 MUST NOT 渲染任何占位块。导语文本来源 MUST 按优先级取值：详情接口的 `description`（权威，由 `GET /api/articles/:id` 提供）优先；详情未就绪或详情 `description` 为空时 SHALL 回退列表项的 `excerpt`（纯文本，≤200 字符）。段落结构、guard 判定与去重逻辑 MUST 保持不变。

导语 MUST 以**去标签纯文本**渲染：详情 `description` 与回退 `excerpt` 都 MUST 先剥除标签与隐藏内容再呈现，呈现长度 MUST ≤300 字符（超长截断并以省略号收尾）；MUST NOT 将来源中的 HTML 作为标签注入 DOM。不可见内容（声明隐藏的元素）MUST NOT 在导语段形成空白占位。

#### Scenario: 有实质内容

- **WHEN** 文章 description 含与正文不重复的实质文本
- **THEN** 标签行下呈现导语段，无边框卡片

#### Scenario: 无实质内容

- **WHEN** description 为空、纯图片标签、仅空白字符或与正文归一化后相同
- **THEN** 不渲染导语段，不留空白占位块

#### Scenario: 详情未就绪时的首帧导语

- **WHEN** 用户点选一篇文章，列表项不再携带完整 HTML `description`（只有 `excerpt`），且详情请求尚未返回
- **THEN** 导语段 SHALL 以 `excerpt` 文本呈现（不留空白占位、不产生布局跳动）；详情返回后 SHALL 切换为详情 `description`

#### Scenario: 详情返回后以详情为准

- **WHEN** 详情接口返回的 `description` 与列表 `excerpt` 内容不同
- **THEN** 导语段 SHALL 呈现详情 `description` 的去标签文本

#### Scenario: HTML 型详情摘要不产生空白

- **WHEN** 详情接口返回的 `description` 是含内联样式与隐藏节点的整篇 HTML（存量数据）
- **THEN** 导语段 SHALL 呈现其去标签后的纯文本（≤300 字符），MUST NOT 出现不可见的空白占位，页面首屏 MUST 直接可见正文

### Requirement: AI 整理稿以无卡框小节呈现

存在 AI 整理稿时 MUST 以正文前的小节呈现（小节标签 + markdown 内容），MUST NOT 使用边框卡片与投影；无整理稿时该区块整体不渲染（不留空卡）。整理稿内标签列表与关注开关行为保持不变。

#### Scenario: 有整理稿

- **WHEN** 文章含 AI 整理稿
- **THEN** 整理稿以带小节标签的无卡框区块出现在正文之前，与正文以红色短粗线转场分隔

#### Scenario: 无整理稿

- **WHEN** 文章无 AI 整理稿
- **THEN** 不出现整理稿区块或空占位

### Requirement: 正文元素编辑版式基准

正文 markdown 元素 MUST 遵循编辑版式基准：引用块为轻量左边线（细线 + 次要文本色），MUST NOT 使用大面积色块背景；表格与图片允许 breakout 略宽于阅读列（超出量 ≤160px 且不超过面板可用宽度），表格容器内横向滚动；代码块保持深底色块与横向滚动。衬线标题样式同样适用于正文 h2/h3。

#### Scenario: 引用块

- **WHEN** 正文包含 blockquote
- **THEN** 引用块仅左侧细线 + 灰色文字，无大色块背景

#### Scenario: 宽表格

- **WHEN** 正文包含超出阅读列宽的表格
- **THEN** 表格呈 breakout 宽度并可在容器内横向滚动，不撑破面板

### Requirement: mermaid 图以细分隔线轻容器呈现

阅读列内渲染完成的 mermaid 图 MUST 以「上下 hairline 细分隔线 + 小字图题」的轻容器呈现（用户 2026-09-19 原型审批选定方案 B）：上下线使用 `--color-border-subtle` 级令牌，图题位于图下方、小号次要文字色。容器 MUST NOT 使用边框+投影卡片形态（去卡片化红线）；图 MUST 在列宽内等比缩放（`max-width: 100%`），超宽图容器 MUST 以横向滚动兜底、MUST NOT 撑破 840px 列版式。

#### Scenario: 常规图在阅读列内呈现

- **WHEN** 宽屏（≥1440px）阅读含 mermaid 图的文章
- **THEN** 图位于阅读列内、上下以 hairline 细线与正文分区，图题小字在图下方，无卡片边框与投影

#### Scenario: 超宽图横向滚动兜底

- **WHEN** 文章含超宽 mermaid 图，等比缩放至列宽后不可读
- **THEN** 容器在列宽内横向可滚动，阅读列版式不被撑破

### Requirement: 文章详情呈现疑似 AI 标记

文章详情面板 SHALL 在既有标签区呈现「疑似 AI 生成」标记，SHALL NOT 把该标记做成卡片或改变阅读列版式（reader 版式去卡片化约束保持不变）。该标记 SHALL 为静态展示，SHALL NOT 复用带操作能力的标签组件。

标记仅在该文章判定状态为疑似时呈现；未判定与已确认非 AI 的文章 SHALL 不呈现任何标记。

#### Scenario: 疑似文章详情显示标记

- **WHEN** 用户打开一篇判定状态为疑似 AI 生成的文章详情
- **THEN** 详情标签区 SHALL 呈现「疑似 AI 生成」标记

#### Scenario: 未判定文章详情不显示标记

- **WHEN** 用户打开一篇判定状态为未判定的文章详情
- **THEN** 详情标签区 SHALL NOT 呈现疑似 AI 标记，也 SHALL NOT 出现占位文案

### Requirement: 文章详情可人工标记与取消疑似 AI

文章详情的「手动操作」菜单 SHALL 提供「标记为疑似 AI 生成」与「取消疑似 AI 标记」两项操作。操作 SHALL 就地生效并即时反映到界面，SHALL NOT 弹出二次确认弹窗；操作失败 SHALL 以既有菜单内联错误位呈现。

手动结论 SHALL 优先于自动结论：已人工标记或人工取消的文章，其后续自动判定 SHALL NOT 覆盖人工结论。

#### Scenario: 手动标记后界面即时反映

- **WHEN** 用户在文章详情菜单选择「标记为疑似 AI 生成」
- **THEN** 该文章 SHALL 立即呈现疑似 AI 标记，标记来源 SHALL 为人工标记

#### Scenario: 手动取消后界面即时反映

- **WHEN** 用户在文章详情菜单选择「取消疑似 AI 标记」
- **THEN** 该文章 SHALL 不再呈现疑似 AI 标记

#### Scenario: 操作失败内联报错

- **WHEN** 手动标记请求失败
- **THEN** 菜单 SHALL 内联展示错误信息，SHALL NOT 以弹窗或整页错误页呈现

#### Scenario: 手动结论不被自动判定覆盖

- **WHEN** 用户已手动取消疑似标记，随后该文章的自动判定完成且置信分高于阈值
- **THEN** 该文章 SHALL 仍不呈现疑似 AI 标记
