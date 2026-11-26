## MODIFIED Requirements

### Requirement: 日报数据模型
系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。线程数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, created_at。

`highlights` JSON 结构：`[{title: string, reason: string, tag_ids: uint[]}]`，2-3 个重点项。

**`highlights` SHALL 恒为 JSON 数组**：落库时 SHALL 写 `[]`（空数组），SHALL NOT 写 JSON `null` 标量——`null` 标量会让 `jsonb_array_elements_text` 抛 SQLSTATE 22023，且在读路径上被前端当作「无内容」而整块隐藏。API 读出时 SHALL 同样恒为数组（历史遗留的 `null` 值 SHALL 在读路径归一为 `[]`），SHALL NOT 向客户端返回 `"highlights": null`。

`raw_clusters` JSON 结构：`[{group_name: string, tag_ids: uint[]}]`，LLM 分组原始结果，用于调试。

日报记录的落库 SHALL 以 (semantic_board_id, period_date) 为冲突键执行原子 upsert：同 board 同日重复落库（无论顺序还是并发）SHALL 收敛为一条记录，SHALL NOT 抛出唯一约束冲突错误。

**同一 report 内 `cluster_index` MAY 重复**：一个聚类/话题的当日内容被拆成多条 section 时（泳道内主题分桶、手动编排、watch 物化），兄弟 section SHALL 共享同一 `cluster_index`，以 `bucket_index`（0 起、组内唯一）区分先后；(report_id, cluster_index) SHALL NOT 建唯一约束。未分桶的 section SHALL 取 `bucket_index=0`、`bucket_title=NULL`。

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

#### Scenario: 兄弟桶共享聚类序号

- **GIVEN** 某聚类被泳道内主题分桶拆成 3 个桶
- **WHEN** 日报落库
- **THEN** 3 条 section SHALL 共享同一 `cluster_index`，`bucket_index` 分别为 0/1/2，`bucket_title` 各自为主题方向名

#### Scenario: 未分桶 section 取默认桶序号

- **GIVEN** 某聚类未达到分桶阈值（threads < 6 且 tags < 10）
- **WHEN** 日报落库
- **THEN** 该 section SHALL 为 `bucket_index=0`、`bucket_title=NULL`，且其 `cluster_index` 在该 report 内仍然唯一

#### Scenario: highlights 落库恒为数组
- **WHEN** 日报生成时未产出任何 highlight（合法空降级或生成失败降级）
- **THEN** `board_daily_reports.highlights` SHALL 落 JSON 空数组 `[]`，SHALL NOT 落 JSON `null` 标量

#### Scenario: 历史 null highlights 读出归一为数组
- **WHEN** 读取一篇 `highlights` 列为 JSON `null` 的历史日报（2026-11-03 前的存量坏数据）
- **THEN** API 返回的 `highlights` SHALL 为 `[]`，SHALL NOT 为 `null`

## ADDED Requirements

### Requirement: 日报 highlights 生成健壮性（语义校验 / 单次重试 / 降级不阻断）
日报编排的 highlights 调用 SHALL 对「语义缺失」的模型响应显式判非法并重试，且在重试仍失败时降级保留整份日报，SHALL NOT 让 highlights 缺失静默变成「整块内容消失」，SHALL NOT 让单点失败连带整份日报作废。

**响应合法性判定**：解析层 SHALL 以「顶层 JSON 对象含 `highlights` 键」为合法判据。
- 顶层**缺** `highlights` 键（含模型把 JSON schema 原样回抄的形态，如 `{"type":"object","properties":{...},"required":["highlights"]}`）→ SHALL 判定为**非法响应**并返回错误，SHALL NOT 静默当作空结果；
- `highlights` 键存在但值为空数组 `[]` → SHALL 判定为**合法降级**（事实锚约束下「信息不足宁可不生成」），SHALL NOT 触发重试；
- `highlights` 键存在但值**非数组**（含 `null`、对象、字符串）→ SHALL 判定为**非法响应**并触发重试；
- 非法 JSON → SHALL 判定为非法响应。

**单次重试**：调用失败或响应非法时 SHALL **重试 1 次**（同参重试）；两次均失败才向上返回错误。SHALL NOT 无限重试，SHALL NOT 在重试成功时丢弃首次响应。

**降级不阻断**：重试仍失败时，编排 SHALL **保留日报主体**（sections / threads / 统计字段照常组装与落库），`highlights` 置为空数组，SHALL 记录一条含失败原因的 **Warn 级日志**，SHALL NOT 因 highlights 单点失败让整份日报生成失败。降级 SHALL NOT 伪造任何要闻内容（SHALL NOT 用其它字段拼凑出 highlights）。

**可观测**：每次降级 SHALL 有一条 Warn 级日志，SHALL 含 board id、失败原因与「已降级为空 highlights」语义，使人能不查库即判断某期日报为何没有重点。

#### Scenario: schema 回抄响应被判定非法并触发重试
- **WHEN** 模型把 JSON schema 原样回抄（响应为 `{"type":"object","properties":{"highlights":{...}},"required":["highlights"]}`，合法 JSON 但无 `highlights` 键）
- **THEN** 解析层 SHALL 返回错误而非空结果，编排 SHALL 重试 1 次

#### Scenario: 重试成功则采用重试结果
- **WHEN** 首次调用返回 schema 回抄、重试调用返回 3 条合法 highlights
- **THEN** 日报 SHALL 采用重试得到的 3 条 highlights 落库，SHALL NOT 落空数组

#### Scenario: 重试仍失败则降级保留日报
- **WHEN** highlights 调用连续两次都失败
- **THEN** 日报 SHALL 照常组装并落库（sections / threads / article_count 等**结构与统计**字段完整；title / summary 在无 highlights 时按既有兜底链回落，本次改动 SHALL NOT 改动该兜底链），`highlights` SHALL 为空数组 `[]`，SHALL NOT 写 `null`，且 SHALL NOT 因该失败导致整份日报生成失败

#### Scenario: 合法空数组不触发重试
- **WHEN** 模型返回 `{"highlights":[]}`（键存在、值为空）
- **THEN** 解析层 SHALL 判定为合法降级、SHALL NOT 报错、SHALL NOT 触发重试，日报 `highlights` 落空数组 `[]`

#### Scenario: 正常 highlights 正常落库
- **WHEN** 模型返回 3 条含 title / reason / tag_ids 的合法 highlights
- **THEN** 日报 SHALL 落 3 条 highlights，title / reason SHALL 保留原文，tag_ids SHALL 仍只保留命中当次标签的合法 id

#### Scenario: 降级有可观测日志
- **WHEN** highlights 连续两次失败触发降级
- **THEN** SHALL 产生一条 Warn 级日志，SHALL 含 board id 与失败原因

### Requirement: 刊头「今日重点」空态可见性
日报刊头组件 SHALL 在 `highlights` 为空（`null` 或 `[]`）时渲染**可见的空态占位**，SHALL NOT 整块隐藏。正常态（至少 1 条 highlights）的渲染 SHALL 保持不变。

空态占位 SHALL 复用刊头既有边框语言与空态文案家族（「本期」+「暂无 X」），SHALL 携带可被组件测试定位的 `data-testid`，SHALL NOT 引入可交互元素（无按钮 / 无跳转 / 无批注锚点）。空态与正常态 SHALL 互斥渲染，SHALL NOT 出现重复 id。

#### Scenario: highlights 为空数组时渲染空态占位
- **WHEN** 日报 `highlights` 为 `[]`
- **THEN** 刊头 SHALL 渲染空态占位（文案含「本期暂无重点」）且 SHALL NOT 渲染要闻条目

#### Scenario: highlights 为 null 时渲染空态占位
- **WHEN** 日报 `highlights` 为 `null`（历史坏数据或接口异常）
- **THEN** 刊头 SHALL 正常渲染空态占位，SHALL NOT 抛错，SHALL NOT 整块消失

#### Scenario: highlights 正常时渲染要闻且无空态占位
- **WHEN** 日报 `highlights` 含 3 条
- **THEN** 刊头 SHALL 渲染 3 条要闻（最多展示 3 条），SHALL NOT 出现空态占位
