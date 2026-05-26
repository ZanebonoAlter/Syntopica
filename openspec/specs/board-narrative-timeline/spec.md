## Purpose

板块叙事时间线 API 和前端组件，展示每个 SemanticBoard 的叙事历史。

## Requirements

### Requirement: 板块叙事时间线 API
系统 SHALL 提供 `GET /api/semantic-boards/:id/narratives?days=7` 端点，返回该 SemanticBoard 最近 N 天的叙事列表（按日期倒序）。每条叙事 SHALL 包含 id、title、summary、status、related_tags（含 id/label）、article_count、related_article_ids（关联文章 ID 列表）、scope_type、period_date。

#### Scenario: 查询最近 7 天叙事
- **WHEN** 请求 `GET /api/semantic-boards/5/narratives?days=7`
- **THEN** 系统 SHALL 返回 SemanticBoard #5 最近 7 天内的所有叙事，按 period_date 倒序

#### Scenario: 板块无叙事
- **WHEN** 请求 `GET /api/semantic-boards/5/narratives?days=7`，但该 board 最近 7 天无叙事
- **THEN** 系统 SHALL 返回空数组

#### Scenario: 默认天数
- **WHEN** 请求 `GET /api/semantic-boards/5/narratives`（未指定 days）
- **THEN** 系统 SHALL 默认返回最近 7 天的叙事

### Requirement: 叙事卡片组件 BoardNarrativeTimeline
前端 SHALL 提供 BoardNarrativeTimeline.vue 组件，展示板块叙事时间线。每条叙事 SHALL 以"小文章卡片"形式展示，包含：status 标签（emerging/continuing/splitting/merging/ending，各有颜色）、日期、标题、摘要、关联标签 chips、文章数。组件 SHALL 嵌入 TagsPage 的 board 详情区域（composition 下方）。

#### Scenario: 展示叙事卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 条叙事
- **THEN** BoardNarrativeTimeline SHALL 渲染 3 张卡片，每张显示 status、日期、标题、摘要、标签、文章数

#### Scenario: 点击叙事展开文章
- **WHEN** 用户点击某条叙事卡片
- **THEN** 系统 SHALL 展开该叙事的关联文章列表（使用叙事记录中的 `related_article_ids` 批量加载文章详情）

#### Scenario: 无叙事时展示空状态
- **WHEN** 选中 board 但该 board 无叙事
- **THEN** 组件 SHALL 展示空状态提示"暂无叙事"

### Requirement: 叙事卡片加载更多
组件 SHALL 支持"加载更早"功能，允许用户扩展 days 参数查看更早的叙事。

#### Scenario: 加载更早叙事
- **WHEN** 用户点击"加载更早"按钮
- **THEN** 系统 SHALL 以更大的 days 参数重新请求叙事列表，追加展示更早的叙事

### Requirement: 取消 scope 分类
系统 SHALL 为每个 SemanticBoard 每天只生成一份叙事，不再区分 global/feed_category scope。NarrativeBoard.scope_type 对新数据 SHALL 统一使用 "board" 值。事件标签收集 SHALL 不再按 category 过滤，而是收集该 board 下所有文章对应的 event tags。

#### Scenario: 单 board 单日单叙事
- **WHEN** SemanticBoard "AI与机器学习" 在 2026-05-25 有来自科技、财经两个 category 的 8 个 event tags
- **THEN** 系统 SHALL 只生成一份叙事（包含所有 8 个 event tags 的上下文），scope_type="board"

#### Scenario: 旧 scope 数据兼容查询
- **WHEN** 查询 board #5 的叙事时间线，但 narrative_summaries 中有 scope_type="global" 或 "feed_category" 的旧数据
- **THEN** 系统 SHALL 仍返回这些旧数据（按 semantic_board_id 匹配），不受 scope_type 值影响，每条叙事包含 scope_type 字段供前端区分
- **NOTE** 旧数据中同一 board 同一天可能有多条不同 scope 的叙事（scope 废弃前的遗留），前端按日期分组展示即可，旧数据会随时间自然沉底
