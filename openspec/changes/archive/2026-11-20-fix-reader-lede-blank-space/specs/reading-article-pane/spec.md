# Delta Spec：reading-article-pane

## MODIFIED Requirements

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
