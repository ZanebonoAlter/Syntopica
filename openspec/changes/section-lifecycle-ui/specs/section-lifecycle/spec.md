## ADDED Requirements

### Requirement: Section 模型新增生命周期字段
`DailyReportSection` SHALL 新增 `status` 和 `prev_section_id` 两个字段。`status` 类型为 `VARCHAR(20)`，取值为 `emerging` 或 `continuing`。`prev_section_id` 类型为 `UINT NULL`，指向前一天同一话题的 section。数据库 migration SHALL 为 `status` 设置默认值 `emerging`。

#### Scenario: 新 section 无前一天匹配
- **WHEN** 日报生成器为某天的聚类 section 做匹配，前一天无任何 section 的 `cluster_tag_ids` 与该 section 的 Jaccard 相似度超过阈值
- **THEN** 该 section 的 `status` SHALL 为 `emerging`，`prev_section_id` SHALL 为 NULL

#### Scenario: section 匹配到前一天 section
- **WHEN** 日报生成器为某天的聚类 section 做匹配，前一天的 section A 与该 section 的 `cluster_tag_ids` Jaccard 相似度最高且超过阈值
- **THEN** 该 section 的 `status` SHALL 为 `continuing`，`prev_section_id` SHALL 为 section A 的 ID

### Requirement: Section 匹配算法
系统 SHALL 在日报生成流程中，为每个 section 通过 `cluster_tag_ids` 的 Jaccard 相似度匹配前一天的 section。匹配阈值：交集 ≥ 2 **或** Jaccard ≥ 0.3。每个 section SHALL 只取前一天相似度最高的 section 作为 `prev_section_id`，且必须超过阈值。

#### Scenario: 高相似度匹配
- **WHEN** 今天 section X 的 `cluster_tag_ids` 为 [10, 15, 22]，昨天 section Y 的 `cluster_tag_ids` 为 [10, 15, 30]
- **THEN** 交集为 {10, 15}，数量为 2 ≥ 2 → 匹配成功，X 的 `prev_section_id` SHALL 为 Y 的 ID

#### Scenario: 低相似度不匹配
- **WHEN** 今天 section X 的 `cluster_tag_ids` 为 [10, 20]，昨天 section Y 的 `cluster_tag_ids` 为 [15, 30]
- **THEN** 交集为空，Jaccard = 0 → 不匹配，X 的 `prev_section_id` SHALL 为 NULL

#### Scenario: 多个候选取最高相似度
- **WHEN** 今天 section X 的 `cluster_tag_ids` 为 [10, 15, 22]，昨天有 section Y1([10, 15, 30]) 和 section Y2([10, 22, 40])
- **THEN** X 与 Y1 交集={10,15}=2，与 Y2 交集={10,22}=2，取 Jaccard 更高者（Y1=2/4=0.5, Y2=2/4=0.5 相同时取第一个），X 的 `prev_section_id` SHALL 为相似度最高的那个

### Requirement: Section ending 状态查询时推导
系统 SHALL 在 section timeline 和 lifecycle API 返回时推导 ending 状态：如果某 section 的 ID 不出现在任何其他 section 的 `prev_section_id` 中，且该 section 不在时间范围的最新一天 → 状态 SHALL 为 `ending`。最新一天的 section SHALL NOT 被标记为 ending（因为后续天尚未生成）。

#### Scenario: 孤立历史 section 标记为 ending
- **WHEN** section #10 属于 5/25，无任何其他 section 的 `prev_section_id` 指向它，且时间范围最新天为 5/28
- **THEN** API 返回 section #10 时状态 SHALL 为 `ending`

#### Scenario: 最新一天 section 不标记 ending
- **WHEN** section #20 属于时间范围最新一天 5/28，且无其他 section 指向它
- **THEN** API 返回 section #20 时状态 SHALL 保持 `emerging` 或 `continuing`（不改为 ending）

### Requirement: Section Timeline API
系统 SHALL 提供 `GET /api/semantic-boards/:id/section-timeline?days=14` 端点，返回该板块最近 N 天所有 section 的扁平列表，按 `prev_section_id` 串联为生命周期行。每条记录 SHALL 包含 id、report_id、period_date、cluster_label、status（含 ending 推导）、article_count、thread_count、prev_section_id。

#### Scenario: 查询板块 section 时间线
- **WHEN** 请求 `GET /api/semantic-boards/5/section-timeline?days=14`
- **THEN** 系统 SHALL 返回 board #5 最近 14 天的所有 section，按 period_date 倒序，含推导后的 status

#### Scenario: 无 section 时返回空
- **WHEN** 请求的板块无日报或无 section
- **THEN** 系统 SHALL 返回空数组

### Requirement: Section Lifecycle API
系统 SHALL 提供 `GET /api/daily-reports/sections/:id/lifecycle` 端点，返回一个 section 的跨天生命周期链。链 SHALL 沿 `prev_section_id` 向前追溯至头（无 prev 的 section），向后扩展至所有以该 section 为 prev 的后续 section。每条记录 SHALL 包含 id、report_id、period_date、cluster_label、status、article_count、thread_count、prev_section_id。

#### Scenario: 查询 section 生命周期
- **WHEN** 请求 `GET /api/daily-reports/sections/15/lifecycle`，section #15 的 prev 链为 #15→#10→#5
- **THEN** 系统 SHALL 返回链 [section#5, section#10, section#15]，按时间正序

#### Scenario: 孤立 section 的生命周期
- **WHEN** 请求 `GET /api/daily-reports/sections/20/lifecycle`，section #20 无 prev 也无后续
- **THEN** 系统 SHALL 返回只包含 section #20 的单元素链

### Requirement: 话题总览组件（BoardThreadBrowser 改造）
前端 `BoardThreadBrowser` SHALL 从 thread 粒度改为 section 粒度的 Gantt 图。横轴为日期列（最近 14 天），纵轴为话题行（通过 `prev_section_id` 串联的 section 生命周期）。每行显示 `cluster_label`。每个节点（圆点）颜色对应 section 状态：emerging=绿、continuing=蓝、ending=灰。鼠标悬停圆点 SHALL 显示 tooltip（聚类名称、状态、文章数、线索数）。点击圆点 SHALL 打开该 section 所属日报的 Modal 并弹出 Section Lifecycle Panel 定位到该 section。

#### Scenario: 展示话题生命周期 Gantt
- **WHEN** 板块有 14 天数据，3 个持续话题 + 2 个新兴话题
- **THEN** Gantt 图 SHALL 显示 5 行话题，持续话题显示连续的蓝色圆点，新兴话题显示单个绿色圆点

#### Scenario: 点击圆点打开详情
- **WHEN** 用户点击 Gantt 图中 5/27 列的「中美贸易谈判」圆点
- **THEN** 系统 SHALL 打开 5/27 日报的报纸 Modal，并弹出 Section Lifecycle Panel 定位到该 section

### Requirement: Section Lifecycle Panel
前端 SHALL 提供 `SectionLifecyclePanel` 组件，替代 `ThreadLineagePanel`。面板 SHALL 以 `position: fixed` 定位在 viewport 右侧（right: 0），不移动或缩窄 Modal。面板宽度 320px，与 Modal 可独立滚动。

面板 SHALL 展示 section 的跨天生命周期链（调用 Section Lifecycle API），从上到下按时间排列。每个节点 SHALL 显示：日期、聚类名称、状态徽章、文章数、线索数。当前选中的 section 用高亮背景标记。节点之间用竖线连接。

#### Scenario: 展示 section 生命周期链
- **WHEN** 用户在报纸 Modal 内点击某个 cluster card 的 header
- **THEN** 右侧 SHALL 弹出 Section Lifecycle Panel，展示该 section 的跨天演进链

#### Scenario: 面板中点击节点切换日期
- **WHEN** 用户在 Section Lifecycle Panel 中点击 5/26 的 section 节点
- **THEN** 系统 SHALL 切换 Modal 内容到 5/26 的日报，滚动到对应 section

#### Scenario: 关闭面板
- **WHEN** 用户点击面板右上角 ✕ 按钮或关闭 Modal
- **THEN** Section Lifecycle Panel SHALL 关闭
