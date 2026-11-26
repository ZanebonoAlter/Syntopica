## MODIFIED Requirements

### Requirement: 日报数据模型
系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), created_at。线程数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, created_at。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

日报记录的落库 SHALL 以 (semantic_board_id, period_date) 为冲突键执行原子 upsert：同 board 同日重复落库（无论顺序还是并发）SHALL 收敛为一条记录，SHALL NOT 抛出唯一约束冲突错误。

#### Scenario: 创建日报记录
- **WHEN** 为 SemanticBoard #5 在 2026-05-25 生成日报
- **THEN** 系统 SHALL 在 `board_daily_reports` 表创建一条记录，status="generating"，period_date="2026-05-25"，semantic_board_id=5

#### Scenario: 日报记录唯一性
- **WHEN** SemanticBoard #5 在 2026-05-25 已有一条 status="done" 的日报
- **THEN** 系统 SHALL NOT 创建重复记录，而是更新已有记录

#### Scenario: 并发落库同板同日不炸唯一约束
- **WHEN** 两条写路径（正常情况下被生成闸互斥；异常绕过闸时）先后或并发为 SemanticBoard #5 落库 2026-05-25 的日报
- **THEN** 系统 SHALL 通过原子 upsert 收敛为一条记录（后写覆盖先写），SHALL NOT 出现 duplicate key 唯一约束错误，SHALL NOT 产生部分写入的残缺日报（sections 与主记录同事务落库）

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事线程 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

### Requirement: 日报生成 API — 异步触发
系统 SHALL 提供 `POST /api/daily-reports/generate` 端点，接受 `{date: string, board_id?: number}` 参数。board_id 为空时生成所有活跃 board 的日报。端点 SHALL 立即返回 `{job_id: string, status: "processing"}`，后台 goroutine 异步执行生成。

同一时刻 SHALL 至多存在一条日报生成管线，互斥范围 SHALL 覆盖共享同一 PostgreSQL 数据库的**全部后端实例**（定时调度或手动触发，无论发起自哪个实例，共享同一把数据库级执行闸）。已有管线在跑时，手动触发（本端点及调度器 `TriggerNowWithDate`）SHALL NOT 启动新管线，SHALL 返回 409 与 `reason: "already_running"`（文案：「日报生成正在执行中，请稍后再试。」）；定时调度路径获取闸失败时 SHALL 记 skip（reason `already_running`），闸由持闸管线在结束时释放（获取失败方无锁可释放，SHALL NOT 调用释放）。闸在管线结束（成功、失败或 panic）后 MUST 释放；持闸进程崩溃或连接断开时，数据库级闸 SHALL 随连接终结自动释放，SHALL NOT 造成永久死锁。数据库不可达导致闸无法探测时，SHALL 回退进程内互斥（单实例行为不劣于现状），SHALL NOT 因闸探测失败而阻断生成。被拒触发 SHALL NOT 产生任何完成/失败终态通知。

#### Scenario: 触发单板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_id: 5}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台开始为 board #5 生成日报

#### Scenario: 触发全板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25"}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台依次为所有活跃 board 生成日报

#### Scenario: 生成进行中重复手动触发
- **WHEN** 某日报生成管线（定时或手动）尚未结束，再次请求 `POST /api/daily-reports/generate`
- **THEN** 系统 SHALL 返回 HTTP 409、`{success: false, reason: "already_running"}`，SHALL NOT 启动第二条管线，SHALL NOT 广播进度或发送终态通知

#### Scenario: 生成进行中调度到期
- **WHEN** 手动管线在跑期间定时调度到期触发 `DailyReportJob`
- **THEN** 本次调度 SHALL 记为 skip（reason `already_running`），SHALL NOT 与手动管线并发生成，SHALL NOT 发送重复完成通知

#### Scenario: 跨实例互斥
- **WHEN** 后端实例 A 的日报管线在跑，连接同一数据库的后端实例 B（如临时验证实例、冒烟副本）收到手动触发或定时调度触发
- **THEN** 实例 B SHALL 被同一把数据库级闸拒绝（手动 409 `already_running` / 调度 skip），两实例 SHALL NOT 并发生成，ai 调用日志 SHALL NOT 出现同 board 同日两个并发生成 session

#### Scenario: 持闸实例崩溃后闸自动释放
- **WHEN** 持闸实例在管线中途崩溃（进程被杀、容器停止）导致数据库连接断开
- **THEN** 数据库级闸 SHALL 随连接终结自动释放，后续任意实例的下一次触发 SHALL 正常受理（不残留 409 / skip），SHALL NOT 需要人工清锁

#### Scenario: 数据库不可达时回退进程内互斥
- **WHEN** 实例尝试获取数据库级闸时数据库连接不可用
- **THEN** 系统 SHALL 回退到进程内互斥并照常开始生成（行为不劣于升级前），SHALL 在日志中记录回退原因，SHALL NOT 返回错误或无限重试

#### Scenario: 管线结束后闸释放
- **WHEN** 一次生成管线结束（全部版面成功、部分失败或 panic）后，再次请求 `POST /api/daily-reports/generate`
- **THEN** 端点 SHALL 正常受理（返回 `status: "processing"`），不残留 409

#### Scenario: 手动触发与定时调度互斥不产生重复通知
- **WHEN** 手动触发被 409 拒绝
- **THEN** 通知中心 SHALL NOT 出现同日期的第二条「日报已生成」条目（通知只由实际运行的管线发送一次）
