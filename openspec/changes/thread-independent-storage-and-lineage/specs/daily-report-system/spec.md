## MODIFIED Requirements

### Requirement: 日报数据模型
系统 SHALL 新建 `board_daily_reports` 和 `daily_report_sections` 两张表，独立于旧 `narrative_boards`/`narrative_summaries`。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。叙事线程 SHALL 存储在独立的 `daily_report_threads` 表中，而非 `daily_report_sections.threads` JSON 列。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, created_at。~~threads(JSON)~~ — 线程数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, status, tag_ids(JSONB), confidence, prev_thread_id(可为空，自引用), created_at。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

#### Scenario: 创建日报记录
- **WHEN** 为 SemanticBoard #5 在 2026-05-25 生成日报
- **THEN** 系统 SHALL 在 `board_daily_reports` 表创建一条记录，status="generating"，period_date="2026-05-25"，semantic_board_id=5

#### Scenario: 日报记录唯一性
- **WHEN** SemanticBoard #5 在 2026-05-25 已有一条 status="done" 的日报
- **THEN** 系统 SHALL NOT 创建重复记录，而是更新已有记录

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事线程 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

### Requirement: 叙事线索连续性匹配
系统 SHALL 通过 tag 交集策略匹配昨日线索：如果今日聚类与昨日线索有 tag ID 交集 → 续接，status 设为 continuing/merging/splitting。匹配结果 SHALL 设置 `prev_thread_id` 指向昨日对应线程的数据库 ID。无匹配 → 标记为 emerging，prev_thread_id 为 NULL。

#### Scenario: Tag 交集匹配成功
- **WHEN** 今日聚类 #1 含 tag IDs [10, 15, 22]，昨日线程 #42 含 tag IDs [10, 15]
- **THEN** 今日聚类 #1 的线索 SHALL 续接昨日线程，prev_thread_id=42

#### Scenario: 多候选取最优匹配
- **WHEN** 今日线程与昨日线程 #42(overlap=1) 和 #55(overlap=3) 均有交集
- **THEN** prev_thread_id SHALL 设为 55（最大交集）

#### Scenario: 完全无匹配
- **WHEN** 今日聚类 #3 与昨日所有线索既无 tag 交集
- **THEN** 该聚类的所有线索 SHALL 标记为 emerging，prev_thread_id 为 NULL

### Requirement: 日报分段并行生成
系统 SHALL 并行执行三类 LLM 生成调用：
- **Call A（今日重点）**：输入全部标签(label+desc+article_count) + 昨日日报，输出 2-3 个重点项（含标题、选择理由、关联标签 ID）
- **Call C×K（聚类叙事线索）**：每个聚类一次调用，输入该聚类标签 + 昨日匹配线索，输出 0-N 条线索（emerging/continuing/splitting/merging/ending）

Call C 生成完成后，系统 SHALL 将线程作为 `daily_report_threads` 表的行持久化，而非 JSON 嵌入 section。

`GenerateDailyReport` 函数 SHALL 返回 `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)`，其中第三项为每个 cluster 对应的 `[]DailyReportThread` 列表（从 `[]Thread` 转换而来），供 `SaveReport` 批量写入 `daily_report_threads` 表。`[][]DailyReportThread` 中的索引 SHALL 与 `[]DailyReportSection` 一一对应。`generateSingleBoard` 调用方 SHALL 适配新签名。

#### Scenario: 并行生成成功
- **WHEN** 有 5 个聚类
- **THEN** 系统 SHALL 同时发起 Call A + Call C×5，共 6 个并行 LLM 调用

#### Scenario: 某个聚类无需线索
- **WHEN** Call C 对某聚类判断无值得报告的线索
- **THEN** 该聚类 SHALL 输出空线索列表，不阻塞其他聚类

#### Scenario: 昨日日报不存在
- **WHEN** 某板某日为首次生成日报（无 prev_report）
- **THEN** Call A 的"昨日日报"输入 SHALL 为空，Call C 的"昨日匹配线索" SHALL 为空，所有线索标记为 emerging，prev_thread_id 为 NULL

### Requirement: 日报查询 API
系统 SHALL 提供以下查询端点：
- `GET /api/semantic-boards/:id/daily-reports?days=7`：查询该 board 最近 N 天的日报列表，按 period_date 倒序
- `GET /api/daily-reports/:id`：查询单篇日报详情（含关联的 DailyReportSection 列表，每个 section 通过 GORM Preload 关联查询包含 `daily_report_threads` 表的线程列表）
- `GET /api/daily-reports/threads/:id/lineage`：查询线程血统链
- `GET /api/semantic-boards/:id/thread-timeline?days=30`：查询板块线程时间线

`GetReportByID` SHALL 使用嵌套 Preload `"Sections.Threads"` 加载 section 及其关联线程。`DailyReportSection` 的 JSON 响应中 `threads` 字段 SHALL 包含 `[]DailyReportThread` 对象数组（每条含 id、prev_thread_id、report_id、section_id、title、summary、status、tag_ids、confidence）。

#### Scenario: 查询板块日报列表
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，每条含 id、title、summary、period_date、status、cluster_count、article_count

#### Scenario: 查询日报详情含线程
- **WHEN** 请求 `GET /api/daily-reports/42`
- **THEN** 系统 SHALL 返回日报 #42 的完整内容，包括 highlights、以及关联的所有 DailyReportSection，每个 section 包含从 `daily_report_threads` 表查询的线程列表（含 id、title、summary、status、prev_thread_id）

#### Scenario: 无日报时返回空
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`，但该 board 无日报
- **THEN** 系统 SHALL 返回空数组

### Requirement: 日报时间线组件 BoardDailyReportTimeline
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件。组件 SHALL 展示板块日报列表，每条日报以卡片形式展示。点击卡片 SHALL 展开报纸模态框显示详情。模态框中每个线程 SHALL 可点击，点击后打开 ThreadLineagePanel 侧面板显示该线程的血统链时间线。

组件 SHALL 提供入口按钮/链接，切换到 BoardThreadBrowser 视图展示板块级线程 Gantt 图。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 展开日报详情
- **WHEN** 用户点击某日报卡片
- **THEN** 组件 SHALL 展开报纸模态框显示：highlights 列表、按质量区域分组的聚类叙事线索卡片

#### Scenario: 线程可点击查看血统
- **WHEN** 用户在报纸模态框中点击某线程的标题/内容区域
- **THEN** SHALL 打开 ThreadLineagePanel 侧面板，显示该线程跨日血统链
- **AND** 线程右侧的文章图标点击 SHALL 保留现有的关联文章 popup 功能（两者不冲突）

#### Scenario: 切换到线程浏览器
- **WHEN** 用户点击 "线程总览" 按钮
- **THEN** SHALL 显示 BoardThreadBrowser 组件，展示 Gantt 图时间线

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"
