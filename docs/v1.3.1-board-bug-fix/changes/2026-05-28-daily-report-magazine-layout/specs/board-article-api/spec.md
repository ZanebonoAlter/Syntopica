## MODIFIED Requirements

### Requirement: TagsPage 内容 Tab
TagsPage 选中 board 时 SHALL 显示三个 Tab：板块内容(composition)、日报(daily-reports)、文章(articles)。Tab 切换 SHALL 用 `v-if` 控制三个面板的显隐。默认 Tab SHALL 为「文章」。"日报" Tab 面板 SHALL 使用 `BoardDailyReportTimeline` 组件。

#### Scenario: 默认 Tab 为文章
- **WHEN** 用户点击选中某个 board
- **THEN** TagsPage SHALL 默认激活「文章」Tab，显示文章列表

#### Scenario: Tab 切换到日报
- **WHEN** 用户点击"日报" Tab
- **THEN** 系统 SHALL 显示 BoardDailyReportTimeline 面板，隐藏其他面板

#### Scenario: Tab 切换到板块内容
- **WHEN** 用户点击"板块内容" Tab
- **THEN** 系统 SHALL 显示 BoardCompositionPanel，隐藏其他面板

### Requirement: TagsPage 布局宽度
TagsPage 的 `.tags-main` 和 `.tags-topbar-inner` 的 max-width SHALL 从 1440px 改为 min(1800px, 95vw)，以更好利用大屏空间。

#### Scenario: 大屏布局
- **WHEN** 用户在 2K 或 4K 显示器上打开 TagsPage
- **THEN** 内容区 SHALL 宽至 min(1800px, 95vw)，而非固定 1440px

### Requirement: 匹配详情面板 sticky 定位
TagsPage 文章 tab 内的 MatchDetailPanel SHALL 使用 `position: sticky; top: 1rem; align-self: flex-start;` 定位，使其在文章列表滚动时保持可见。面板 SHALL 设置 `max-height: calc(100vh - 6rem); overflow-y: auto;` 以防止超出视口。

#### Scenario: 文章列表滚动时面板跟随
- **WHEN** 用户在文章 tab 向下滚动文章列表
- **THEN** MatchDetailPanel SHALL 保持在可视区域内，随页面滚动而 sticky 定位

#### Scenario: 面板内容超出时滚动
- **WHEN** MatchDetailPanel 内容高度超过视口可用高度
- **THEN** 面板 SHALL 独立滚动，不撑长页面
