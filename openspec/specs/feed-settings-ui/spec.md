# Feed Settings UI

## Purpose

TBD

## Requirements

### Requirement: Feed 卡片展示完整处理管线 toggle
每个 feed 卡片 SHALL 展示 4 个 toggle 开关，按管线执行顺序排列：Firecrawl 全文抓取、打标签、AI 摘要、内容补全。

#### Scenario: 展示所有 toggle
- **WHEN** 用户打开全局设置的订阅源配置 tab
- **THEN** 每个 feed 卡片底部显示 4 个 toggle：Firecrawl、打标签、AI 摘要、内容补全

#### Scenario: Toggle 状态与后端数据一致
- **WHEN** feed 的 firecrawl_enabled = true, tagging_enabled = false, article_summary_enabled = true, completion_on_refresh = true
- **THEN** 对应 toggle 分别为 开/关/开/开

### Requirement: Toggle 变更即时保存
用户切换 toggle SHALL 立即调用 feed update API 保存到后端，无需额外提交按钮。

#### Scenario: 切换 toggle
- **WHEN** 用户点击某 feed 的"打标签" toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 tagging_enabled，并刷新 feed 列表

### Requirement: 分类标题可折叠
订阅源配置 tab 中的分类标题 SHALL 支持点击折叠/展开。默认展开。折叠状态在当前页面生命周期内保持。

#### Scenario: 折叠分类
- **WHEN** 用户点击某分类标题
- **THEN** 该分类下的 feed 列表收起，标题显示折叠图标（▶）

#### Scenario: 展开分类
- **WHEN** 用户点击已折叠的分类标题
- **THEN** 该分类下的 feed 列表展开，标题显示展开图标（▼）

#### Scenario: 刷新页面后恢复默认
- **WHEN** 用户关闭并重新打开全局设置对话框
- **THEN** 所有分类恢复为展开状态

### Requirement: 最大文章数"无限制"语义正确
最大文章数选项 SHALL 使用 `0` 表示无限制。后端 `CleanupOldArticles` SHALL 将 `maxArticles <= 0` 视为不限制。

#### Scenario: 选择无限制
- **WHEN** 用户选择"无限制"选项
- **THEN** 前端发送 `max_articles: 0` 到后端

#### Scenario: 后端不清理 max_articles=0 的 feed
- **WHEN** feed.MaxArticles = 0 且文章数超过任意值
- **THEN** CleanupOldArticles 不删除任何文章

#### Scenario: 兼容旧的 9999 值
- **WHEN** feed.MaxArticles = 9999
- **THEN** 前端显示"无限制"，后端行为与 max_articles=0 一致（不删除）

### Requirement: Feed 卡片展示 firecrawl 和补全 toggle
Firecrawl toggle SHALL 控制 `firecrawl_enabled` 字段，内容补全 toggle SHALL 控制 `completion_on_refresh` 字段。

#### Scenario: 切换 Firecrawl toggle
- **WHEN** 用户点击 Firecrawl toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 firecrawl_enabled

#### Scenario: 切换内容补全 toggle
- **WHEN** 用户点击内容补全 toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 completion_on_refresh
