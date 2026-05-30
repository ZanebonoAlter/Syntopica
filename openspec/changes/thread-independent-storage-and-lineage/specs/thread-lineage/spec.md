## ADDED Requirements

### Requirement: prev_thread_id 赋值
`matchPreviousThreads()` 函数 SHALL 在检测到 tag-overlap 匹配时，将匹配到的上一日线程的数据库 ID 赋值给当前线程的 `PrevThreadID` 字段。函数 SHALL 接收上一日线程列表（包含 DB ID）作为参数。

`getPrevThreadSummaries()` 函数 SHALL 改为从 `daily_report_threads` 表查询上一日线程，返回 `[]DailyReportThread`（包含 DB ID），供 `matchPreviousThreads()` 使用。

`findPreviousReport()` SHALL 改为 Preload sections 的关联 threads（通过 GORM Preload 嵌套），使 `matchPreviousThreads()` 和 `getPrevThreadSummaries()` 无需额外查询即可访问上一日线程数据。

#### Scenario: Tag 交集匹配赋值
- **WHEN** 今日线程 A (tag_ids=[10,15,22]) 与昨日线程 B (id=42, tag_ids=[10,15]) 有 tag 交集 (overlap=2)
- **THEN** matchPreviousThreads SHALL 设置线程 A 的 PrevThreadID=42

#### Scenario: 多个候选匹配取最优
- **WHEN** 今日线程 A 与昨日线程 B (overlap=1) 和线程 C (overlap=3) 均有交集
- **THEN** matchPreviousThreads SHALL 设置 PrevThreadID 为线程 C 的 ID（最大交集）

#### Scenario: 无匹配不赋值
- **WHEN** 今日线程 A 与昨日所有线程无 tag 交集
- **THEN** PrevThreadID SHALL 保持 NULL，status 保持 LLM 原始输出（通常为 emerging）

### Requirement: 线程血统链查询 API
系统 SHALL 提供 `GET /api/daily-reports/threads/:id/lineage` 端点，返回给定线程的完整血统链。

API SHALL 使用 PostgreSQL recursive CTE 查询：
1. 从给定线程 ID 向后遍历 `prev_thread_id` 直到链头
2. 从链头向前遍历所有引用该链头的后代线程
3. 返回完整的链，每条线程附带 `period_date`（从关联报告 join）

响应格式：`{success: true, data: {chain: [{id, title, summary, status, prev_thread_id, report_id, period_date, section_id}, ...]}}`

#### Scenario: 查询中间线程的血统链
- **WHEN** 请求 `GET /api/daily-reports/threads/50/lineage`，线程 50 的 prev_thread_id=42，线程 42 的 prev_thread_id=30，线程 30 无 prev，且线程 55 的 prev=50
- **THEN** 响应 SHALL 返回链 [30, 42, 50, 55]，按 period_date 升序

#### Scenario: 查询链头线程
- **WHEN** 请求 `GET /api/daily-reports/threads/30/lineage`，线程 30 无 prev_thread_id，且有后代 42→50→55
- **THEN** 响应 SHALL 返回完整链 [30, 42, 50, 55]

#### Scenario: 线程不存在
- **WHEN** 请求 `GET /api/daily-reports/threads/9999/lineage`，线程 9999 不存在
- **THEN** 响应 SHALL 返回 404

### Requirement: 板块线程时间线 API
系统 SHALL 提供 `GET /api/semantic-boards/:id/thread-timeline?days=30` 端点，返回该板块在指定天数内的所有线程（含 lineage 信息），供 Gantt 图视图使用。

响应格式：`{success: true, data: {threads: [{id, report_id, section_id, title, summary, status, tag_ids, confidence, prev_thread_id, period_date, cluster_label}, ...]}}`

参数 `days` 默认 30，最大 90。`period_date` 从关联的 `board_daily_reports` join 获取，`cluster_label` 从关联的 `daily_report_sections` join 获取。

#### Scenario: 查询板块线程时间线
- **WHEN** 请求 `GET /api/semantic-boards/5/thread-timeline?days=7`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的所有线程，每条包含 period_date 和 cluster_label

#### Scenario: 无线程数据
- **WHEN** 请求板块的 thread-timeline，但该板块在指定天数内无线程
- **THEN** 系统 SHALL 返回空数组 `{threads: []}`

### Requirement: Thread detail panel 组件 (View A)
前端 SHALL 提供 `ThreadLineagePanel.vue` 组件，在报纸模态框内作为侧面板展示。

组件 SHALL：
1. 当用户点击报纸模态框中的某个线程标题区域时触发打开（展示血统链）
2. 当用户点击线程右侧的文章图标时，保留现有的关联文章 popup 功能（两者不冲突）
3. 调用 `GET /api/daily-reports/threads/:id/lineage` 获取血统链
4. 渲染垂直时间线：从最早到最新排列，每个节点显示日期、标题、摘要、status 色标
5. 高亮当前线程节点
6. 提供关闭按钮回到报纸主视图

#### Scenario: 点击线程打开血统面板
- **WHEN** 用户在报纸模态框中点击 "中美关税" 线程
- **THEN** 右侧 SHALL 滑出 ThreadLineagePanel，显示该线程 3 天的血统链：5/25(新兴) → 5/26(持续) → 5/27(持续)

#### Scenario: 历史线程无血统
- **WHEN** 用户点击一个历史线程（prev_thread_id=NULL，无后代）
- **THEN** ThreadLineagePanel SHALL 显示单节点时间线，标注"独立线程"

#### Scenario: 关闭面板
- **WHEN** 用户点击面板关闭按钮
- **THEN** ThreadLineagePanel SHALL 关闭，回到报纸主视图

### Requirement: Board thread browser 组件 (View B)
前端 SHALL 提供 `BoardThreadBrowser.vue` 组件，展示板块所有线程的 Gantt 图时间线。

组件 SHALL：
1. 可从 BoardDailyReportTimeline 面板中通过按钮/链接访问
2. 调用 `GET /api/semantic-boards/:id/thread-timeline?days=30` 获取线程数据
3. 渲染 Gantt 图：
   - 列 = 日期（左到右，最近 N 天）
   - 行 = 线程血统链（用 prev_thread_id 在前端组装链）
   - 节点 = 圆形/方块，颜色按 status（emerging=绿, continuing=蓝, splitting=橙, merging=紫, ending=灰）
   - 同一链的节点之间用线段连接
4. 点击节点显示线程详情（title + summary）
5. 提供 days 参数调节（7/14/30/60）

#### Scenario: 展示板块线程 Gantt 图
- **WHEN** 用户打开 board "贸易战" 的 Thread Browser（days=30）
- **THEN** 组件 SHALL 渲染 Gantt 图，包含该板块所有线程，按血统链分组，节点按日期排列

#### Scenario: 同一链的节点连线
- **WHEN** 线程 A(5/25, prev=NULL) → 线程 B(5/26, prev=A) → 线程 C(5/27, prev=B)
- **THEN** Gantt 图 SHALL 在这三个节点之间画连接线，并在同一行中排列

#### Scenario: 点击节点查看详情
- **WHEN** 用户点击 Gantt 图中某个线程节点
- **THEN** SHALL 显示该线程的 title、summary、status、日期

#### Scenario: 无数据空状态
- **WHEN** 板块在指定天数内无线程数据
- **THEN** 组件 SHALL 显示"暂无线程数据"提示
