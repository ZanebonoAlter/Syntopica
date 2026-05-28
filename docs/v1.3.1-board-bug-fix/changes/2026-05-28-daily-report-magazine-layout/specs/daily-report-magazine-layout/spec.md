## ADDED Requirements

### Requirement: 日报收起态概要列表
日报 tab 打开时 SHALL 默认展示收起态概要列表。每条日报渲染为一行概要卡片，包含：日期（格式化为「M.DD  星期X」）、状态标签、文章数、summary 字段截断前 30 字作为摘要。概要卡片数据来源于 list API（`GET /semantic-boards/:id/daily-reports`），该接口返回的 `DailyReportListItem` 包含 `title`、`summary`、`article_count`、`status`、`period_date`，不包含 `highlights`。概要卡片 SHALL 可点击，点击后弹出全屏旧报纸弹窗。

#### Scenario: 展示概要列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** 日报 tab SHALL 渲染 3 张概要卡片，按日期倒序，每张显示日期 + summary（截断前 30 字）+ article_count + status

#### Scenario: 无 highlights 时回退
- **WHEN** 某天日报的 summary 为空字符串
- **THEN** 概要卡片的摘要行 SHALL 显示 title 字段（截断前 30 字）作为回退

#### Scenario: 加载更早
- **WHEN** 概要列表底部显示「加载更早」按钮
- **THEN** 点击后 SHALL 增大 days 参数重新请求，追加展示更早的概要卡片

### Requirement: 日报全屏旧报纸弹窗
点击概要卡片后 SHALL 弹出全屏旧报纸弹窗。弹窗由暗色遮罩（rgba(0,0,0,0.7)）和居中纸张面板组成。纸张面板使用 `#f4eed7` 泛黄底色 + CSS 微弱噪点纹理 + 边缘泛黄渐变。标题使用衬线字体（Noto Serif SC），正文使用无衬线字体。弹窗 SHALL 支持按章节分页，每页显示当前页码。点击遮罩或顶栏关闭按钮 SHALL 关闭弹窗。

#### Scenario: 弹窗结构
- **WHEN** 用户点击某天概要卡片
- **THEN** SHALL 弹出全屏遮罩 + 居中纸张面板，面板内含：顶栏（日期 + 上下换天按钮 + 关闭×）、纸张内容区、左右边缘翻页按钮

#### Scenario: 旧报纸视觉风格
- **WHEN** 弹窗打开后
- **THEN** 纸张面板 SHALL 使用 `#f4eed7` 底色，带微弱噪点纹理和边缘泛黄渐变；标题 SHALL 使用 Noto Serif SC 衬线字体；正文 SHALL 使用无衬线字体

#### Scenario: 按章节分页
- **WHEN** 某天日报包含 2 个 highlights + dynamics + 3 个 sections
- **THEN** SHALL 分为 4 页：第 1 页 = 今日重点 + 板块动态，第 2-4 页每页 = 一个叙事线索 section

#### Scenario: 单页日报
- **WHEN** 某天日报只有 highlights 和 dynamics，没有 sections
- **THEN** SHALL 只有 1 页，左右翻页按钮隐藏

#### Scenario: 关闭弹窗
- **WHEN** 用户点击遮罩区域或顶栏 × 按钮
- **THEN** 弹窗 SHALL 关闭，回到概要列表视图

### Requirement: 弹窗导航 — 左右翻页 / 上下换天
弹窗 SHALL 提供两个维度的导航。左右边缘按钮（半透明箭头）用于翻内容页（同日内章节翻页），首页时 ← 置灰，末页时 → 置灰。顶栏上下按钮 `[↑上一天] 日期 [↓下一天]` 用于切换日报日期，第一天时 ↑ 置灰，最后一天时 ↓ 置灰。SHALL 支持键盘快捷键：← → 翻页、↑ ↓ 换天、Esc 关闭弹窗。

#### Scenario: 左右翻内容页
- **WHEN** 用户在 5.27 日报的第 1 页点击 → 边缘按钮
- **THEN** SHALL 切换到第 2 页，带水平 slide 动画（300ms ease-out）

#### Scenario: 上下换日报
- **WHEN** 用户点击顶栏 ↑ 按钮
- **THEN** SHALL 切换到上一天的日报内容，页码重置为第 1 页

#### Scenario: 键盘导航
- **WHEN** 弹窗打开且用户按下 ← 键
- **THEN** SHALL 翻到上一内容页（等同于点击 ← 按钮）
- **WHEN** 用户按下 Esc 键
- **THEN** SHALL 关闭弹窗

#### Scenario: 首末页/天禁用
- **WHEN** 在第 1 页时 ← 按钮 SHALL 置灰不可点击
- **WHEN** 在最后一页时 → 按钮 SHALL 置灰不可点击
- **WHEN** 在第一天时 ↑ 按钮 SHALL 置灰不可点击
- **WHEN** 在最后一天时 ↓ 按钮 SHALL 置灰不可点击

### Requirement: 旧报纸排版层级
旧报纸弹窗内 SHALL 使用字号和排版层级构建视觉层次。装饰性大号日期用 2.2rem 衬线体深色 20%，今日重点标题用 1.3rem 衬线体深色 90%，正文用 0.82rem 无衬线深色 55%，聚类标题用 0.92rem 衬线体深色 80%。板块间用实线分隔，聚类间用虚线分隔，thread 间用细虚线分隔。分隔线颜色 SHALL 使用深色系（适配纸张底色），非白色系。

#### Scenario: 排版字号
- **WHEN** 弹窗渲染完成
- **THEN** 日期 SHALL 以 2.2rem 衬线体渲染，重点标题 SHALL 以 1.3rem 衬线体渲染，正文 SHALL 以 0.82rem 无衬线体渲染，聚类标题 SHALL 以 0.92rem 衬线体渲染

#### Scenario: 分隔线
- **WHEN** 渲染今日重点与板块动态之间
- **THEN** SHALL 使用实线分隔（rgba(0,0,0,0.12)）
- **WHEN** 渲染两个聚类之间
- **THEN** SHALL 使用虚线分隔（rgba(0,0,0,0.08)）

### Requirement: 杂志页 thread 展示
旧报纸弹窗内的叙事线索 SHALL 每条 thread 显示状态标签 + 标题 + 摘要，thread 之间用虚线分隔。状态标签 SHALL 使用彩色（绿/蓝/橙/紫/灰），其余文字 SHALL 为深色系（适配纸张底色）。

#### Scenario: thread 渲染
- **WHEN** 聚类「模型竞争」含 2 条 thread
- **THEN** SHALL 渲染为：聚类标题行（模型竞争 + 5 篇文章）→ [新兴] thread 标题 + 摘要 → 虚线 → [持续] thread 标题 + 摘要

### Requirement: 弹窗动画
弹窗打开 SHALL 使用遮罩 fade-in 200ms + 纸张面板 scale(0.95→1) + fade-in 300ms。弹窗关闭 SHALL 使用反向 200ms 动画。左右翻页 SHALL 使用 translateX 水平滑动，300ms ease-out。概要卡片入场 SHALL 使用 staggered fade-in（200ms duration，50ms interval）。

#### Scenario: 弹窗打开动画
- **WHEN** 用户点击概要卡片
- **THEN** 遮罩 SHALL 在 200ms 内 fade-in，纸张面板 SHALL 在 300ms 内从 scale(0.95) + opacity(0) 过渡到 scale(1) + opacity(1)

#### Scenario: 翻页动画
- **WHEN** 用户点击 → 翻页按钮
- **THEN** 当前页 SHALL 向左滑出，新页 SHALL 从右滑入，300ms ease-out
