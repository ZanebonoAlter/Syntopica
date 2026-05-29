## Why

日报 tab 的当前展示存在三个核心问题：
1. **视觉层次缺失** — 所有内容块（今日重点、板块动态、叙事线索）用相同的卡片样式，用户扫一眼无法抓住重点
2. **交互路径低效** — 点击展开/收起操作频繁，且展开后内容堆叠在一个区域内，信息密度过高
3. **布局未充分利用大屏** — 整体限宽 1440px，匹配详情面板不跟随滚动，时间筛选缺少快捷操作

## What Changes

- **日报 tab 双态展示**：默认收起态（概要列表）+ 点击进入全屏旧报纸弹窗（按章节分页），替代现有的平铺展开
- **旧报纸排版风格**：泛黄纸张底色 + 微弱噪点 + 衬线字体，报纸式章节分页，左右翻内容页、上下换日报
- **全屏弹窗导航**：顶栏日期+上下换天，左右边缘按钮翻内容页，支持键盘快捷键
- **TagsPage 默认 tab 改为「文章」**
- **布局宽度放宽**：1440px → min(1800px, 95vw)
- **匹配详情面板 sticky 定位**：跟随文章列表滚动
- **时间筛选默认今天 + 快捷 chip**：今天/3天/7天/30天

## Capabilities

### New Capabilities

- `daily-report-magazine-layout`: 日报 tab 的旧报纸弹窗展示 — 收起态概要列表、全屏旧报纸弹窗排版、按章节分页、双维度导航

### Modified Capabilities

- `daily-report-system`: 日报时间线组件 `BoardDailyReportTimeline.vue` 的 Requirement 需更新 — 从「卡片展开详情」改为「概要列表 + 全屏旧报纸弹窗」
- `board-article-api`: TagsPage 默认 tab 从 `composition` 改为 `articles`；匹配详情面板增加 sticky 定位；布局 max-width 放宽
- `feed-settings-ui`: 文章时间筛选增加快捷 chip 行（今天/3天/7天/30天），默认选中「今天」

## Impact

- **前端组件**：`BoardDailyReportTimeline.vue` 重写模板和样式（全屏弹窗 + 旧报纸排版 + 分页逻辑）；`TagsPage.vue` 修改默认 tab、布局宽度、匹配详情 sticky、时间筛选 UI
- **API**：无变更，复用现有 `GET /api/semantic-boards/:id/daily-reports?days=7` 和 `GET /api/daily-reports/:id` 端点
- **数据模型**：无变更
- **用户体验**：日报 tab 交互路径从「展开/收起」变为「概要列表 → 全屏旧报纸弹窗」，视觉风格从暗色卡片式变为泛黄纸张报纸式
