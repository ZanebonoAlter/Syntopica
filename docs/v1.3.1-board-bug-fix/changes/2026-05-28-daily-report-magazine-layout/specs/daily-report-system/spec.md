## MODIFIED Requirements

### Requirement: 日报时间线组件 BoardDailyReportTimeline
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件。组件 SHALL 采用双态展示：默认收起态展示概要列表（日期 + summary 截断前 30 字 + 文章数 + status），点击概要卡片弹出全屏旧报纸弹窗展示单日完整内容。弹窗使用泛黄纸张底色 + 衬线字体排版，按章节分页（第 1 页 = 今日重点 + 板块动态，后续每页 = 一个叙事线索 section）。弹窗导航：左右边缘按钮翻内容页，顶栏上下按钮切换日报日期。支持键盘快捷键（←→翻页、↑↓换天、Esc关闭）。组件 SHALL 嵌入 TagsPage 的 "日报" Tab 中。

#### Scenario: 展示概要列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张概要卡片，按日期倒序

#### Scenario: 弹出旧报纸弹窗
- **WHEN** 用户点击某概要卡片
- **THEN** SHALL 弹出全屏遮罩 + 居中纸张面板，显示该天日报完整内容

#### Scenario: 旧报纸排版
- **WHEN** 弹窗渲染完成
- **THEN** SHALL 以旧报纸排版展示：泛黄纸张底色、衬线字体标题、今日重点大字标题（1.3rem）、正文（0.82rem）、板块动态、聚类叙事线索

#### Scenario: 按章节分页
- **WHEN** 某天日报包含 highlights + dynamics + 3 个 sections
- **THEN** SHALL 分为 4 页，左右边缘按钮翻页

#### Scenario: 叙事线索 status 颜色
- **WHEN** 线索 status 为 emerging/continuing/splitting/merging/ending
- **THEN** 对应颜色 SHALL 为 绿/蓝/橙/紫/灰

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"

#### Scenario: 加载更早
- **WHEN** 用户在概要列表点击"加载更早"
- **THEN** 组件 SHALL 增大 days 参数重新请求，追加展示更早的概要卡片
