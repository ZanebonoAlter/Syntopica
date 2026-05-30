## ADDED Requirements

### Requirement: daily_report_threads table
系统 SHALL 创建 `daily_report_threads` 表，每条记录代表一个叙事线索，具有独立的数据库身份。

**DailyReportThread 字段**：id (SERIAL PK), report_id (INT NOT NULL REFERENCES board_daily_reports(id)), section_id (INT NOT NULL REFERENCES daily_report_sections(id)), title (TEXT NOT NULL), summary (TEXT), status (VARCHAR(20) DEFAULT 'emerging'), tag_ids (JSONB DEFAULT '[]'), confidence (FLOAT DEFAULT 0), prev_thread_id (INT REFERENCES daily_report_threads(id)), created_at (TIMESTAMP DEFAULT NOW()).

系统 SHALL 创建以下索引：
- `idx_daily_report_threads_report_id` ON (report_id)
- `idx_daily_report_threads_section_id` ON (section_id)
- `idx_daily_report_threads_prev_thread_id` ON (prev_thread_id) WHERE prev_thread_id IS NOT NULL

#### Scenario: 新表创建
- **WHEN** 数据库迁移运行
- **THEN** 系统 SHALL 创建 `daily_report_threads` 表，包含上述所有字段和索引

#### Scenario: 外键约束
- **WHEN** 尝试插入一条 report_id 不存在于 board_daily_reports 的 thread 记录
- **THEN** 数据库 SHALL 拒绝插入并抛出外键约束错误

### Requirement: DailyReportThread GORM 模型
后端 SHALL 定义 `DailyReportThread` GORM 模型结构体，映射到 `daily_report_threads` 表，包含所有表字段的 Go 对应类型。`TagIDs` 字段 SHALL 使用 JSONB 类型。`PrevThreadID` SHALL 使用 `*uint`（可为空）。JSON tag SHALL 使用 `tag_ids`（与前端一致）。

`DailyReportSection` 结构体 SHALL 添加 GORM 关联字段 `Threads []DailyReportThread`（`gorm:"foreignKey:SectionID"`），使 GORM Preload 可以自动加载关联线程。`Threads` 字段的 JSON tag SHALL 为 `threads`，确保 API 响应结构兼容前端。

#### Scenario: 模型映射
- **WHEN** GORM AutoMigrate 或查询使用 DailyReportThread 结构体
- **THEN** 结构体 SHALL 正确映射到 daily_report_threads 表的所有列

### Requirement: Thread 数据迁移
系统 SHALL 提供迁移步骤，将 `daily_report_sections.threads` JSONB 中的现有线程数据提取到 `daily_report_threads` 表。

迁移 SHALL：
1. 对每条有非空 `threads` JSONB 的 `daily_report_sections` 记录，使用 `jsonb_array_elements` 展开线程数组
2. 为每个线程插入一条 `daily_report_threads` 记录，从 JSON 提取 title、summary、status、tag_ids、confidence
3. 设置 `report_id` 为对应 section 的 `report_id`
4. 设置 `section_id` 为对应 section 的 `id`
5. 设置 `prev_thread_id` 为 NULL（历史数据无法回溯匹配）

#### Scenario: 迁移现有线程数据
- **WHEN** daily_report_sections 中有 3 条记录，其 threads JSONB 分别包含 2、3、1 个线程
- **THEN** 迁移 SHALL 在 daily_report_threads 表中创建 6 条记录，每条正确关联到对应的 report_id 和 section_id

迁移 SQL SHALL 正确映射现有 JSON 字段名到新表列名：`related_tag_ids` → `tag_ids`，`parent_thread_id` → 忽略（历史数据 prev_thread_id 为 NULL）。迁移 SHALL 跳过 `related_article_ids` 字段（该信息不在新表中存储）。

#### Scenario: 空 threads 列
- **WHEN** 某条 daily_report_section 的 threads 字段为 NULL 或空数组
- **THEN** 迁移 SHALL 不为该 section 创建任何 thread 记录

### Requirement: 移除 daily_report_sections.threads 列
在数据迁移完成并验证后，系统 SHALL 通过后续迁移删除 `daily_report_sections` 表的 `threads` JSONB 列。DailyReportSection GORM 模型 SHALL 移除 `Threads JSON` 字段（`gorm:"type:jsonb" json:"threads"`），替换为 GORM 关联字段 `Threads []DailyReportThread`。

#### Scenario: 列移除
- **WHEN** 迁移执行 DROP COLUMN
- **THEN** `daily_report_sections` 表 SHALL 不再包含 `threads` 列

#### Scenario: 前端兼容性
- **WHEN** 报告详情 API 返回 section 数据
- **THEN** section 对象 SHALL 通过关联查询的 threads 列表返回线程数据（JSON key 仍为 `threads`），而非内嵌 JSONB。每条线程 SHALL 包含 `id`、`prev_thread_id`、`report_id`、`section_id`、`title`、`summary`、`status`、`tag_ids`、`confidence` 字段。

### Requirement: Thread 存储接口
系统 SHALL 提供以下存储接口：
- `SaveThreads(sectionID uint, reportID uint, threads []DailyReportThread) error`：批量保存线程（在事务中，设置 section_id 和 report_id）
- `GetThreadsBySection(sectionID uint) ([]DailyReportThread, error)`：获取某 section 的所有线程
- `GetThreadsByReport(reportID uint) ([]DailyReportThread, error)`：获取某报告的所有线程
- `GetThreadByID(id uint) (*DailyReportThread, error)`：获取单条线程
- `DeleteThreadsByReport(reportID uint) error`：删除某报告的所有线程（用于 upsert 场景）

#### Scenario: 批量保存线程
- **WHEN** 调用 SaveThreads(sectionID=10, reportID=5, threads=[...3 threads...])
- **THEN** 系统 SHALL 在 daily_report_threads 表创建 3 条记录，每条的 section_id=10, report_id=5

#### Scenario: 按 section 查询线程
- **WHEN** 调用 GetThreadsBySection(sectionID=10)
- **THEN** 系统 SHALL 返回该 section 的所有线程，按 id 升序
