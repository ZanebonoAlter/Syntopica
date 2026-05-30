## MODIFIED Requirements

### Requirement: 日报数据模型
系统 SHALL 新建 `board_daily_reports` 和 `daily_report_sections` 两张表，独立于旧 `narrative_boards`/`narrative_summaries`。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), threads(JSON), article_count, best_tier INT, avg_score FLOAT8, **status VARCHAR(20) DEFAULT 'emerging'**, **prev_section_id UINT NULL**, created_at。`status` 取值为 `emerging` 或 `continuing`，由后端通过 `cluster_tag_ids` Jaccard 相似度匹配前一天 section 推导。`prev_section_id` 指向前一天同一话题的 section。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

`threads` JSON 结构：`[{title: string, summary: string, status: string(emerging/continuing/splitting/merging/ending), related_tag_ids: uint[], related_article_ids: uint[], parent_thread_id: string(可为空)}]`。

`related_article_ids` 在生成报告时根据 thread 的 `tag_ids` 从 `collectBoardTags` 已收集的 tag→article 映射中查关联文章 ID，去重后写入。前端浮窗使用该字段展示文章列表。

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

#### Scenario: Section 生成时推导 status 和 prev_section_id
- **WHEN** 日报生成器为 2026-05-25 的某个 section 保存到数据库
- **THEN** 系统 SHALL 在保存前通过 cluster_tag_ids Jaccard 匹配前一天 section，设置 prev_section_id 和 status（emerging/continuing）

### Requirement: 日报时间线组件 BoardDailyReportTimeline（报纸布局）
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件，替代 `BoardNarrativeTimeline.vue`。组件 SHALL 展示板块日报列表，采用长滚动报纸布局。

**纸张尺寸**：`min(1100px, 92vw)` × `92vh`，单页长滚动（不分页）。

**布局结构**（从上到下）：
1. 报头：日期大标题
2. 今日重点：highlights 展示（title + reason）
3. **质量分区**：按 `best_tier` 将聚类分为区域
   - 核心事件（Tier 0-1）：双列 CSS Grid
   - 相关事件（Tier 2）：单列
   - 其他动态（Tier 3+）：单列
4. 每个分区显示区头标签 + 聚类数

**每个聚类卡片**：默认折叠状态，显示聚类名称、文章数、section 级状态徽章（emerging=绿/continuing=蓝）、「N 条线索 ▸」文本。点击卡片或「N 条线索」文本 SHALL 展开显示所有线索（title + summary），线索不显示独立状态徽章。点击 section 的 header 区域（名称+状态） SHALL 打开右侧 SectionLifecyclePanel。

**线索文章浮窗**：使用 `@floating-ui/vue`，展示 `related_article_ids` 对应的文章标题列表。首批加载 5 篇，支持"加载更多"。点选文章→emit `openArticle(articleId)`。

`dynamics` 为空时前端不渲染"板块动态"区块。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 展开日报详情
- **WHEN** 用户点击某日报卡片
- **THEN** 组件 SHALL 展开长滚动报纸布局：highlights 列表、质量分区（核心/相关/其他）

#### Scenario: Section 状态徽章颜色
- **WHEN** section status 为 emerging/continuing/ending
- **THEN** 对应颜色 SHALL 为 绿/蓝/灰

#### Scenario: 核心事件双列布局
- **WHEN** 有 3 个聚类分别属于 tier 0、tier 2、tier 3
- **THEN** tier 0 聚类在"核心事件"双列区域，tier 2 在"相关事件"单列，tier 3 在"其他动态"单列

#### Scenario: 线索默认折叠
- **WHEN** 某聚类有 3 条线索
- **THEN** 该聚类卡片 SHALL 默认只显示 section 状态徽章和「3 条线索 ▸」，不显示线索标题和摘要

#### Scenario: 展开线索详情
- **WHEN** 用户点击聚类卡片或「N 条线索 ▸」
- **THEN** 卡片 SHALL 展开显示全部线索的 title + summary + 文章图标，线索不显示独立状态徽章

#### Scenario: 点击 section header 打开 Lifecycle Panel
- **WHEN** 用户点击聚类卡片的 header 区域（名称+状态）
- **THEN** 系统 SHALL 在 viewport 右侧弹出 SectionLifecyclePanel，展示该 section 的跨天生命周期链

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"

#### Scenario: 加载更早
- **WHEN** 用户点击"加载更早"
- **THEN** 组件 SHALL 增大 days 参数重新请求，追加展示更早的日报

### Requirement: 日报生成编排流水线
系统 SHALL 提供 `GenerateDailyReport(ctx, boardID, date)` 编排函数，按顺序执行：收集板内事件标签 → 质量筛选 → 去重 → LLM 分组(带组数限制) → 查询昨日日报 → 并行生成(Call A + C×K) → 连续性匹配 → **section 生命周期匹配（cluster_tag_ids Jaccard）** → 组装 BoardDailyReport + DailyReportSection(含 best_tier/avg_score/status/prev_section_id) → 存储。生成 SHALL 通过 goroutine 异步执行。

#### Scenario: 完整流水线执行
- **WHEN** 触发 SemanticBoard #5 在 2026-05-25 的日报生成
- **THEN** 系统 SHALL 按序执行：收集标签(20个) → 质量筛选(过滤后15个) → 去重(剩12个) → LLM分组(4个聚类) → 查询昨日日报(id=42) → 并行生成(1+4=5个LLM调用) → 连续性匹配 → section 生命周期匹配 → 组装存储(含best_tier/avg_score/status/prev_section_id) → status="done"

#### Scenario: 生成失败
- **WHEN** 流水线中任一步骤失败（如 LLM 调用超时）
- **THEN** 系统 SHALL 设置 status="failed"，保留已完成的中间结果（raw_clusters 等），WS 广播失败状态
