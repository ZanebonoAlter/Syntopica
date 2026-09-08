## Purpose

日报系统替代旧叙事系统，为每个 SemanticBoard 每日生成结构化日报，包含今日重点、板块动态和聚类叙事线索。
## Requirements
### Requirement: 日报数据模型
系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), created_at。线程数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, created_at。

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
### Requirement: 事件标签去重
系统 SHALL 在生成日报前对收集到的事件标签进行程序化精确去重，不使用 LLM。去重 SHALL 应用两条规则：(1) 关联文章集合完全相同的标签合并为一个；(2) article_count=1 且关联同一篇文章的标签合并为一个。去重 SHALL 不改变原始标签数据，仅在生成流程中使用去重后的列表。

#### Scenario: 文章集合完全相同的标签去重
- **WHEN** 标签 A 和标签 B 关联的文章 ID 集合均为 {101, 102, 103}
- **THEN** 系统 SHALL 将二者合并为一个（保留 article_count 更大或 id 更小的标签）

#### Scenario: 单篇文章标签去重
- **WHEN** 标签 X (article_count=1, 关联文章 200) 和标签 Y (article_count=1, 关联文章 200) 为不同标签但指向同一文章
- **THEN** 系统 SHALL 将二者合并为一个

#### Scenario: 去重不影响原始数据
- **WHEN** 去重流程执行后
- **THEN** `topic_tags` 表中的原始标签记录 SHALL 保持不变，去重仅在内存中进行
### Requirement: 日报质量筛选
`collectBoardTags` 查询携带 `match_reason` 和 `score`（包括 fallback 路径产生的标签）。生成管线 SHALL 在聚类前按以下规则筛选标签：

1. 过滤 `direction_mismatch = true`
2. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`（含 downgraded）
3. 过滤 `weighted`（最弱规则）
4. 如果剩余 < 10 个标签，把 weighted 也拉回来（保底）
5. 如果剩余 > 30 个标签，按 `(tier, score)` 排序后截断到 top-30

Fallback 标签同等对待：fallback 路径产生的标签也携带 `match_reason`/`score`，筛选规则完全一致。

#### Scenario: 质量筛选过滤 weighted
- **WHEN** 收集到 20 个标签，其中 5 个 weighted
- **THEN** 系统 SHALL 过滤掉 weighted 标签，15 个进入聚类

#### Scenario: 保底机制
- **WHEN** 收集到 12 个标签，过滤 weighted 后只剩 8 个 (< 10)
- **THEN** 系统 SHALL 把 weighted 标签拉回，12 个全部进入聚类

#### Scenario: 截断机制
- **WHEN** 收集到 40 个标签，过滤后剩 35 个 (> 30)
- **THEN** 系统 SHALL 按 (tier, score) 排序后截断到 top-30
### Requirement: 聚类数限制

日报聚类 SHALL 采用「embedding 质心先分桶 → LLM 弱区裁决/兜底」流程，取代 LLM 对全部当天 tag 自由聚类：

1. 对去重 + 质量筛选后的当天 event tag，按 PersistentTopic 质心做最近邻分桶：L1（< `lane_l1_threshold`）/ L2（[`lane_l1_threshold`, `lane_l2_threshold`]）/ L3（> `lane_l2_threshold`）。
2. L1 tag 按 topic 直接成组（同 topic 的 L1 tag 合并为该 topic 当日 section 候选），不调用 LLM。
3. L2 tag 交 `ClusterTags` 在 embedding 预筛的 top-K 候选 topic 上做「留/换/新」三选一。
4. L3 tag 交 `ClusterTags` 起新叙事成组。
5. section 天生挂 topic（L1 直挂 / L2 LLM 挂 / L3 新建），无事后 section 标题↔topic 匹配环节。

`len(去重后 tags) <= 2` 或 L2+L3 tag 合计不足以成组时，SHALL 跳过 LLM，每个 tag 独立成组（沿用现有兜底）。

#### Scenario: 三层分桶聚类

- **GIVEN** board 当天 40 个 event tag，质心分桶后 L1=18 / L2=20 / L3=2
- **WHEN** 系统执行聚类
- **THEN** L1 的 18 个 tag 按 topic 直接成组（不调 LLM），L2 的 20 个交 LLM 三选一，L3 的 2 个交 LLM 起新叙事

#### Scenario: 极少 tag 跳过 LLM

- **GIVEN** board 当天去重后仅 2 个 event tag
- **WHEN** 系统聚类
- **THEN** 系统 SHALL 跳过 LLM，每个 tag 独立成组（沿用 `len<=2` 兜底）
### Requirement: 聚类排序字段

生成报告时，系统 SHALL 为每个 DailyReportSection 计算：
- `BestTier int`：该 section 中所有 tag 的最高 tier（match_reason + downgraded 映射）
- `AvgScore float64`：该 section 中所有 tag 的 score 平均值

前端用 `best_tier ASC, avg_score DESC` 排序聚类。

#### Scenario: 计算 best_tier
- **WHEN** section 的 tags 包含 direct_hit(tier=0) 和 weighted(tier=3)
- **THEN** best_tier=0

#### Scenario: 前端按质量排序聚类
- **WHEN** 有 3 个 section，best_tier 分别为 0、2、1
- **THEN** 前端按 0→1→2 顺序展示
### Requirement: LLM 语义分组
系统 SHALL 使用单次 LLM call 对去重后的事件标签做语义分组。分组粒度 SHALL 为"同一核心事件"，每组 2-8 个标签，超过 8 个 SHALL 拆分为多组，单个标签可自成一组。LLM 调用 SHALL 使用 temperature=0.1，输出 SHALL 遵循 JSON schema 约束 `[{group_name: string, tag_ids: uint[]}]`。

#### Scenario: 正常分组
- **WHEN** 去重后有 30 个事件标签
- **THEN** LLM SHALL 输出 5-10 个分组，每组含 group_name 和对应的 tag_ids 列表

#### Scenario: 标签数少于 3
- **WHEN** 去重后只有 2 个事件标签，且语义不相关
- **THEN** LLM SHALL 输出 2 个分组，每个分组含 1 个标签

#### Scenario: 分组结果持久化
- **WHEN** LLM 分组完成
- **THEN** 分组结果 SHALL 存入 `BoardDailyReport.raw_clusters` JSON 字段，用于调试审计
### Requirement: 日报分段并行生成
系统 SHALL 并行执行两类 LLM 生成调用：
- **Call A（今日重点）**：输入全部标签(label+desc+article_count) + 昨日日报，输出 2-3 个重点项（含标题、选择理由、关联标签 ID）
- **Call C×K（聚类叙事线索）**：每个聚类一次调用，输入该聚类标签，输出 0-N 条线索（仅 title + summary + tag_ids + confidence）

Call C 生成完成后，系统 SHALL 将线程作为 `daily_report_threads` 表的行持久化。

`GenerateDailyReport` 函数 SHALL 返回 `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)`，其中第三项为每个 cluster 对应的 `[]DailyReportThread` 列表，供 `SaveReport` 批量写入 `daily_report_threads` 表。`[][]DailyReportThread` 中的索引 SHALL 与 `[]DailyReportSection` 一一对应。

Thread 生成 SHALL 移除 status、prev_thread_id 相关的 prompt 要求和 JSON schema 字段。Thread 的 system prompt SHALL 简化为仅要求输出 title、summary、tag_ids、confidence。

prompt version 升级为 "3.0"。

#### Scenario: 并行生成成功
- **WHEN** 有 5 个聚类
- **THEN** 系统 SHALL 同时发起 Call A + Call C×5，共 6 个并行 LLM 调用

#### Scenario: Thread 输出不含 status
- **WHEN** Call C 为某聚类生成 3 条 thread
- **THEN** 每条 thread SHALL 只包含 title、summary、tag_ids、confidence，不包含 status 和 prev_thread_id

#### Scenario: 昨日日报不存在
- **WHEN** 某板某日为首次生成日报
- **THEN** Call A 的"昨日日报"输入 SHALL 为空，Call C 不传入任何历史线索上下文（已移除 getPrevThreadSummaries 调用）
### Requirement: 日报生成编排流水线

系统 SHALL 提供 `GenerateDailyReport(ctx, boardID, date)` 编排函数，按顺序执行：收集板内事件标签 → 质量筛选 → 去重 → LLM 分组(带组数限制) → 查询昨日日报 → 并行生成(Call A + C×K) → section 内容化 embedding 生成（文本来源见 section-content-embedding 能力，基于该 section 所聚 tag 的 label/description/代表文章摘录，而非 cluster_label 标题文本） → 同日 section 两阶段合并 → **Watch 物化追加**（keyword_topic / sentence_topic 物化轨按 watch-materialized-topic 能力产出追加 section；任一关注的物化失败 SHALL 降级跳过，SHALL NOT 阻断流水线） → section embedding 匹配写入关系表 → 组装 BoardDailyReport + DailyReportSection(含 best_tier/avg_score) → 存储。生成 SHALL 通过 goroutine 异步执行。

物化追加的 section SHALL NOT 参与同日合并，SHALL NOT 参与 section 关系计算。

流水线 SHALL NOT 执行 thread 级别的 tag 交集匹配或 prev_thread_id 赋值。

#### Scenario: 完整流水线执行

- **WHEN** 触发 SemanticBoard #5 在 2026-05-25 的日报生成
- **THEN** 系统 SHALL 按序执行：收集标签 → 质量筛选 → 去重 → LLM分组 → 查询昨日日报 → 并行生成 → 内容化 embedding 生成 → 同日合并 → Watch 物化追加 → section embedding 匹配写入关系表 → 组装存储 → status="done"

#### Scenario: 物化失败不阻断流水线

- **GIVEN** board #5 有一个 active 的 sentence_topic 关注
- **WHEN** 该关注的辅助标签检索在生成中失败
- **THEN** 系统 SHALL 跳过该关注的当期物化并记录日志，日报 SHALL 正常完成并保存，status SHALL NOT 为 failed

#### Scenario: 生成失败

- **WHEN** 流水线中任一步骤失败（如 LLM 调用超时）
- **THEN** 系统 SHALL 设置 status="failed"，保留已完成的中间结果（raw_clusters 等），WS 广播失败状态
### Requirement: 日报存储
系统 SHALL 提供 `SaveReport`（创建或更新日报+关联 sections）、`GetReport(boardID, date)`（查询单篇）、`ListReports(boardID, days)`（查询列表）三个存储接口。

`ListReports` SHALL 在 `days <= 0` 时使用 7 天默认值。对于任意正数 `days`，系统 SHALL 按请求天数查询，不得静默截断到 30 天。

#### Scenario: 保存日报和分组
- **WHEN** 流水线完成生成
- **THEN** 系统 SHALL 创建一条 BoardDailyReport 记录（status="done"）和多条 DailyReportSection 记录（每个聚类一条），并在事务中完成

#### Scenario: 查询最近 7 天日报
- **WHEN** 请求 `ListReports(boardID=5, days=7)`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，按 period_date 倒序

#### Scenario: 查询超过 30 天日报
- **WHEN** 请求 `ListReports(boardID=5, days=42)`
- **THEN** 系统 SHALL 查询最近 42 天，不得将范围截断为 30 天

#### Scenario: 非正数天数使用默认值
- **WHEN** 请求 `ListReports(boardID=5, days=0)`
- **THEN** 系统 SHALL 查询最近 7 天
### Requirement: 日报生成 API — 异步触发
系统 SHALL 提供 `POST /api/daily-reports/generate` 端点，接受 `{date: string, board_id?: number}` 参数。board_id 为空时生成所有活跃 board 的日报。端点 SHALL 立即返回 `{job_id: string, status: "processing"}`，后台 goroutine 异步执行生成。

#### Scenario: 触发单板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_id: 5}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台开始为 board #5 生成日报

#### Scenario: 触发全板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25"}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台依次为所有活跃 board 生成日报
### Requirement: 日报生成 WebSocket 进度广播
生成 goroutine SHALL 通过 `ws.GetHub().BroadcastRaw()` 广播两类消息：
- `daily_report_progress`：每完成一个 board 后广播 `{"type": "daily_report_progress", "job_id": "...", "board_id": N, "board_name": "...", "status": "completed|failed", "saved": N, "progress": "current/total"}`
- `daily_report_done`：全部完成后广播 `{"type": "daily_report_done", "job_id": "...", "total_saved": N, "total_boards": N}`

#### Scenario: 单板生成完成广播
- **WHEN** board #5 的日报生成成功
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_progress", "job_id": "xxx", "board_id": 5, "board_name": "AI与机器学习", "status": "completed", "saved": 1, "progress": "1/1"}`

#### Scenario: 全部生成完成广播
- **WHEN** 3 个 board 的日报全部生成完毕
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 3, "total_boards": 3}`
### Requirement: 日报查询 API
系统 SHALL 提供以下查询端点：
- `GET /api/semantic-boards/:id/daily-reports?days=7`：查询该 board 最近 N 天的日报列表
- `GET /api/daily-reports/:id`：查询单篇日报详情（含关联 sections，每个 section 通过 GORM Preload 包含 threads 列表）
- `GET /api/semantic-boards/:id/section-timeline?days=14`：查询板块 section 时间线（含关系）

`GET /api/semantic-boards/:id/daily-reports` SHALL 在未提供有效正数 `days` 时使用 7 天默认值，并对任意正数 `days` 查询实际请求范围，不得静默截断到 30 天。

`GetReportByID` SHALL 使用嵌套 Preload `"Sections.Threads"` 加载 section 及其关联线程。`DailyReportSection` 的 JSON 响应中 `threads` 字段 SHALL 包含 `[]DailyReportThread` 对象数组（每条含 id、report_id、section_id、title、summary、tag_ids、confidence），section 和 thread 均不含 status/prev_*_id 字段。

#### Scenario: 查询板块日报列表
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，每条含 id、title、summary、period_date、status、cluster_count、article_count

#### Scenario: 查询日报详情含线程
- **WHEN** 请求 `GET /api/daily-reports/42`
- **THEN** 系统 SHALL 返回日报 #42 完整内容，每个 section 包含 threads 列表（每条含 id、title、summary、tag_ids、confidence、related_article_ids），section 和 thread 均不含 status/prev_*_id 字段

#### Scenario: API 查询超过 30 天
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=42`
- **THEN** 系统 SHALL 返回最近 42 天范围内的全部匹配日报

#### Scenario: 无日报时返回空
- **WHEN** 请求 `GET /api/semantic-boards/5/daily-reports?days=7`，但该 board 无日报
- **THEN** 系统 SHALL 返回空数组
### Requirement: 日报时间线组件 BoardDailyReportTimeline（报纸布局）
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件，展示板块日报列表，并以全屏长滚动阅读层呈现选中日报。详情 SHALL 保持在 TagsPage 内，不新增独立路由。

**详情容器**：使用 `position: fixed; inset: 0` 占满 viewport，提供明确关闭按钮、Esc 关闭、背景滚动锁和焦点恢复。关闭详情 SHALL 返回原日报列表位置。

**宽屏布局**：大于 1100px 时采用 sticky 边栏 + 正文双栏结构。边栏包含本期目录、active 话题索引和历史日报日期；头条与 highlights 可跨正文通栏。721px–1100px SHALL 降为单栏并把目录/日期收拢到正文顶部；720px 及以下 SHALL 保持单栏且不得产生页面级横向溢出。

**主题**：详情的 surface、文字、边框、阴影和交互状态 SHALL 使用现有 semantic theme token，并同时支持 editorial/dark。topic 的稳定 `persistent_topic.color` MAY 用作身份强调色。组件 SHALL NOT 以固定浅色背景为前提硬编码黑色透明色。

**内容映射**：
1. masthead 展示真实日报标题、日期、article_count 和 cluster_count；刊头 SHALL NOT 固定声称"号外"。
2. 头条优先取 `highlights[0]`；highlights 为空时回退到质量最高的 section，且 SHALL NOT 生成 API 中不存在的新闻文案。
3. highlights 按 API 顺序最多展示三项；为空时不渲染该区块。
4. section 按 `persistent_topic.status` 分为"关心的话题"(active)、"突发的新话题"(candidate/已归属但未 active)和"其他动态"(未归属)。分区内继续按 best_tier 升序、avg_score 降序排列。
5. `dynamics` 为空时不渲染"板块动态"区块。

**话题卡片与 mini 生命线**：active 话题卡片 SHALL 支持原位展开。首次展开调用 `getTopicLifeline(topicId)`，展示以当前日报日期为终点的最近七个自然日；加载成功后按 topic id 缓存，重复展开不再次请求。加载、失败、重试和空状态 SHALL 明确可见。

mini 生命线 SHALL 按日期列展示真实 section，同日多 section 合并为一个节点并显示数量。连线只使用响应中 `relation_type="identity"` 的真实关系，采用贝塞尔路径连接端点；空白日期不生成假节点，系统 SHALL NOT 根据日期相邻关系臆造连线或混入 similarity relation。节点点击 SHALL 原位展开当日 threads。

active topic SHALL 提供进入侦探墙完整生命线的出口；无 topic id 或设备不支持该入口时隐藏。现有"话题总览"入口 SHALL 继续打开 BoardThreadBrowser。

**thread 文章**：thread 有 `related_article_ids` 时 SHALL 可原位展开文章标题列表。前端只加载尚未缓存的 article id，相同文章跨 thread 复用缓存；单篇失败不阻塞其他文章并提供重试。点击文章 SHALL `emit('openArticle', articleId)`。

**section 生命周期**：点击 section header SHALL 继续打开右侧 SectionLifecyclePanel，不因新增 topic mini 生命线而移除 section 维度入口。

"加载更早" SHALL 每次将 `days` 增加 7 并重新请求该完整时间范围。组件 SHALL 用响应替换当前列表，避免累计请求造成重复日报。切换 board SHALL 将 `days` 重置为 7，并清理与旧 board 相关的展开状态。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 打开全屏日报详情
- **WHEN** 用户点击某日报卡片
- **THEN** 组件 SHALL 在当前 TagsPage 内打开占满 viewport 的长滚动日报阅读层
- **AND** 关闭后 SHALL 返回原日报列表位置

#### Scenario: 宽屏双栏与窄屏降级
- **WHEN** 分别以 1440px、1000px 和 720px viewport 打开同一日报
- **THEN** 1440px SHALL 显示 sticky 边栏与正文双栏
- **AND** 1000px 与 720px SHALL 使用单栏降级，720px 不产生页面级横向溢出

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 日报 surface、文字、边框、SVG 和交互状态 SHALL 使用对应主题语义 token 并保持可读

#### Scenario: 按持久话题状态分区
- **WHEN** 日报包含 2 个 active、1 个 candidate 和 1 个未归属 section
- **THEN** 页面 SHALL 分别在"关心的话题""突发的新话题""其他动态"展示 2、1、1 个 section

#### Scenario: 展开并缓存话题生命线
- **WHEN** 用户首次展开一个 active 话题卡片并在加载完成后收起、再次展开
- **THEN** 系统 SHALL 仅调用一次 `getTopicLifeline(topicId)`
- **AND** 再次展开 SHALL 使用缓存结果

#### Scenario: identity 连线跨越空白日期
- **WHEN** lifeline 在周一和周三有节点、周二无节点，且响应包含周一到周三的 identity relation
- **THEN** mini 生命线 SHALL 用一条贝塞尔路径连接周一和周三的真实节点
- **AND** SHALL NOT 为周二生成假节点
- **AND** 该跨天连线 SHALL 以弱化不透明度呈现，与相邻节点的强连线区分

#### Scenario: lifeline 加载失败
- **WHEN** `getTopicLifeline(topicId)` 返回错误
- **THEN** 话题卡片 SHALL 显示局部错误和重试操作，不关闭日报详情或影响其他话题

#### Scenario: 展开 thread 文章并复用缓存
- **WHEN** 两个 thread 引用同一个 article id，用户依次展开两个 thread
- **THEN** 前端 SHALL 只为该 article id 请求一次文章详情
- **AND** 点击文章 SHALL 发出 `openArticle(articleId)`

#### Scenario: 进入完整话题生命线
- **WHEN** active topic 具有 topic id 且当前设备支持侦探墙入口
- **THEN** 页面 SHALL 显示"在侦探墙打开完整生命线"操作并进入对应 topic lifeline

#### Scenario: 键盘关闭详情
- **WHEN** 日报详情打开且用户按 Esc
- **THEN** 详情 SHALL 关闭、背景滚动锁 SHALL 解除，并将焦点恢复到打开详情的日报卡片

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"

#### Scenario: 加载超过 30 天
- **WHEN** 当前 `days=28`，用户连续两次点击"加载更早"
- **THEN** 组件 SHALL 依次以 `days=35` 和 `days=42` 请求，并展示对应范围内的日报

#### Scenario: 切换 board 重置状态
- **WHEN** 用户从一个 board 切换到另一个 board
- **THEN** 组件 SHALL 将 `days` 重置为 7、加载新 board 日报，并清理旧 board 的 topic/thread 展开状态
### Requirement: 日报生成进度前端
前端 SHALL 提供 `useDailyReportProgress.ts` composable，连接 `/ws`，过滤 `daily_report_progress`/`daily_report_done` 消息。`NarrativeGenerateDialog.vue` SHALL 改为触发日报生成（调用 `generateDailyReport`），触发后显示进度板模式：每个 board 一行，实时更新状态（等待/生成中/完成+条数），使用 `useDailyReportProgress` composable。

#### Scenario: 触发生成并显示进度
- **WHEN** 用户在 NarrativeGenerateDialog 选择日期和 board，点击"生成"
- **THEN** 对话框 SHALL 切换为进度板模式，实时显示每个 board 的生成状态

#### Scenario: 生成完成
- **WHEN** 收到 `daily_report_done` 消息
- **THEN** 进度板 SHALL 显示"全部完成"提示和总数统计
### Requirement: TagsPage 内容 Tab
TagsPage 选中 board 时 SHALL 显示三个 Tab：板块内容(composition)、日报(daily-reports)、文章(articles)。Tab 切换 SHALL 用 `v-if` 控制三个面板的显隐。默认 Tab 为"板块内容"。"日报" Tab 面板 SHALL 使用 `BoardDailyReportTimeline` 组件。

#### Scenario: Tab 切换到日报
- **WHEN** 用户点击"日报" Tab
- **THEN** 系统 SHALL 显示 BoardDailyReportTimeline 面板，隐藏 BoardCompositionPanel 和文章列表

#### Scenario: Tab 切换到文章
- **WHEN** 用户点击"文章" Tab
- **THEN** 系统 SHALL 显示带筛选的文章列表，隐藏其他面板
### Requirement: 定时任务复用
系统 SHALL 复用 `scheduler_tasks` 表中的 `narrative_summary` 任务，改造执行逻辑调用 `daily_report.GenerateDailyReport`。check_interval 保持 86400s。定时任务 SHALL 异步执行并通过 WS 广播进度。

#### Scenario: 定时触发日报生成
- **WHEN** `narrative_summary` 定时任务按 check_interval 触发
- **THEN** 系统 SHALL 为所有活跃 board 生成当日日报，使用与手动触发相同的异步 WS 流程
### Requirement: 日报 LLM 调用路由绑定
日报生成的所有 LLM 调用（事件标签语义聚类、要闻 highlights 生成、叙事线程 narrative 生成）SHALL 通过 `digest_polish` capability 加载路由与 provider，SHALL NOT 复用 `topic_tagging` 路由。这使得日报可独立配置 provider、并发上限与温度，不再与标签提取共享配额。

#### Scenario: 语义聚类调用使用 digest_polish
- **WHEN** `daily_report_cluster` 对去重后的事件标签执行 LLM 语义分组
- **THEN** LLM 调用 SHALL 使用 `digest_polish` capability

#### Scenario: 要闻与叙事调用使用 digest_polish
- **WHEN** `daily_report_llm` 生成要闻（highlights）或叙事线程（narrative）
- **THEN** LLM 调用 SHALL 使用 `digest_polish` capability

#### Scenario: 日报独立配置 provider
- **WHEN** 用户在能力路由面板为 `digest_polish` 配置了与 `topic_tagging` 不同的 provider
- **THEN** 日报生成 SHALL 使用 `digest_polish` 配置的 provider，标签提取 SHALL NOT 受影响
### Requirement: section lane 归属标记

`daily_report_sections` SHALL 包含 `lane_tier` 列（取值 l1_direct / l2_llm / l3_new / watch_keyword / watch_sentence），标识该 section 的分桶来源，供前端展示与下游分析。lane_tier SHALL 在 section 生成时与 `topic_match_confidence` 一同确定并持久化。

watch_keyword section 的 `persistent_topic_id` SHALL 为空；watch_sentence section SHALL 归属其关注的专属持久话题（见 watch-materialized-topic 能力）。

#### Scenario: section 记录 lane 来源

- **GIVEN** 某 section 由 L1 直挂产生
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 l1_direct，topic_match_confidence 为 anchor_hit

#### Scenario: 物化 section 记录物化来源

- **GIVEN** 关键字物化追加产出一个 section
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 watch_keyword，persistent_topic_id SHALL 为空
### Requirement: 日报聚类裁决 prompt 历史隔离

L2 泳道裁决（`buildL2Prompt`，operation `daily_report.decide_l2_tags`）的 LLM prompt SHALL NOT 注入候选话题的历史叙事文案（`daily_report_threads` 的 title / summary），切断「昨天幻觉 thread → 今天作为 briefs 喂回 → LLM 延续叙事」的渗透闭环。

L2 裁决 prompt MAY 注入以下**非叙事**信号辅助裁决：候选 topic 的 `label`、状态（active / candidate）、最近命中日期、累计命中天数、质心余弦距离、近期 section 的框架名（`cluster_label` / `section_label`，话题命名级别，**非** thread 文案）。

L2 system prompt 的裁决依据措辞 SHALL 与实际注入内容保持一致——基于「标签语义与近期 section 框架」，而非「实际近期内容（thread 文案）」，避免 prompt 自相矛盾误导 LLM。

本约束随 `promptVersion` 由 "3.0" 升至 "4.0" 一并生效。

#### Scenario: L2 prompt 不含历史 thread 文案

- **GIVEN** board 的某 active 话题近 7 天生成过 thread（title="半导体链全线跌停"）
- **WHEN** 当天 L2 裁决为某沾边 tag 构建 prompt
- **THEN** prompt SHALL NOT 出现该 thread 的 title 或 summary 文案
- **AND** prompt MAY 出现该话题的 label、状态、最近命中日期、质心距离、近期 section_label

#### Scenario: L2 prompt 保留话题框架信号以区分近似话题

- **GIVEN** 两个 active 话题 label 字面相近但近期 section 框架不同
- **WHEN** L2 裁决为某 tag 构建 prompt
- **THEN** prompt SHALL 提供两者的 section 框架信号以供区分
- **AND** SHALL NOT 提供任一话题的 thread title / summary 文案

#### Scenario: promptVersion 升级

- **WHEN** 日报按本约束生成
- **THEN** `board_daily_reports.generation_prompt_version` SHALL 为 "4.0"
### Requirement: 日报文案生成事实锚约束

日报要闻（`GenerateHighlights`，operation `daily_report.highlights`）与叙事线程（`GenerateClusterThreads`，operation `daily_report.threads`）的 system prompt SHALL 包含「事实锚」约束，要求生成的 title / reason / summary 仅基于所提供标签的事实（`label` / `description` / `代表文章`）。

事实锚约束 SHALL 明确禁止以下编造行为（当对应信息未在所列标签中出现时）：

1. 编造未列举的具体事件
2. 编造具体数字（涨跌幅 / 金额 / 连板数 / 百分比 / 跌停涨停）
3. 编造市场情绪判断（恐慌 / 狂热 / 崩盘 / 抛售）
4. 编造因果推断（「引发」/「导致」/「因此」）

当标签信息不足以支撑某条叙事时，系统 SHALL 选择不生成该条（如返回 `{"threads":[]}`），而非补全编造。

JSON schema 中 `summary` / `reason` 字段的 description SHALL 追加「须基于所列标签事实，禁止编造」作双重强化。

本约束随 `promptVersion` "4.0" 一并生效。

#### Scenario: thread summary 不编造数字与情绪

- **GIVEN** 某 cluster 的 tag 仅为几个半导体公司名（label），无任何涨跌 / 情绪描述
- **WHEN** `GenerateClusterThreads` 生成 thread
- **THEN** thread 的 title / summary SHALL NOT 出现「全线跌停」「引发恐慌」等未由标签支撑的数字或情绪判断

#### Scenario: highlights reason 不编造因果

- **GIVEN** 当天 tag 无任何关于事件因果的描述
- **WHEN** `GenerateHighlights` 生成要闻
- **THEN** reason SHALL NOT 出现「引发」「导致」等未由标签支撑的因果推断

#### Scenario: 信息不足时宁缺毋滥

- **GIVEN** 某 cluster 仅含 1 个 tag 且描述匮乏
- **WHEN** 生成 thread
- **THEN** 系统 MAY 返回空 threads（`{"threads":[]}`），SHALL NOT 编造内容凑数
### Requirement: section 展示标题内容化

当日志报 section 的展示标题（`cluster_label`）SHALL 由该 section 当天实际聚合的内容派生（所聚标签事实 + 代表文章），SHALL NOT 默认取所挂持久话题的 label 作为展示标题——话题 label 仅承担归属锚与兜底职责。

标题生成 SHALL 遵守日报文案生成事实锚约束（禁编造事件/数字/情绪/因果，信息不足时降级兜底而非凑数），随 `promptVersion` 升级生效。

话题归属信号（`persistent_topic_id` / `lane_tier` / `topic_match_confidence` / `topic_match_distance`）SHALL 不因标题来源改变而变化。

#### Scenario: 命中既有话题的 section 标题反映当天内容

- **GIVEN** 某 active 话题 label 为「日本首相高市早苗宣布不于7月释放石油储备」（创建于 7 月的旧事件）
- **WHEN** 8 月某日该话题命中的 section 实际聚合了当天"执政基础不稳"相关的标签与文章
- **THEN** 该 section 的 `cluster_label` SHALL 为基于当天标签事实拟定的标题（如「高市执政基础不稳引发党内反弹担忧」）
- **AND** SHALL NOT 为话题 label「日本首相高市早苗宣布不于7月释放石油储备」

#### Scenario: 标题生成失败时降级兜底

- **GIVEN** 某 section 命中既有话题，但当日标题生成 LLM 调用失败或返回不可用结果
- **THEN** `cluster_label` SHALL 按兜底链取值：当日代表 thread 标题 → 话题 label（或 L3 场景下的分组名）
- **AND** SHALL NOT 出现空标题

#### Scenario: L3 新话题标题行为不变

- **GIVEN** 某 section 走 L3 泳道新建话题
- **WHEN** section 持久化
- **THEN** `cluster_label` SHALL 为当天 LLM 命名的分组名（现有行为），`lane_tier` 为 l3_new

#### Scenario: 话题归属字段不受标题影响

- **GIVEN** 某 section 以 l1_direct 挂上 active 话题且标题已内容化
- **WHEN** section 持久化
- **THEN** `persistent_topic_id`、`lane_tier=l1_direct`、`topic_match_confidence=anchor_hit`、`topic_match_distance` SHALL 与标题来源无关地照常记录

#### Scenario: 标题遵守事实锚约束

- **GIVEN** 某 section 所聚标签仅有公司名与行业名，无任何涨跌数字或情绪词
- **WHEN** 生成该 section 标题
- **THEN** 标题 SHALL NOT 出现未由标签事实支撑的数字、情绪或因果表述

### Requirement: 历史与跨天可读性边界

历史 section 的 `cluster_label` SHALL NOT 回刷——变更生效前的旧 section 保留原标题（话题名复读期），生效后的新 section 走内容化标题，形成自然分界。

前端时间线展示同话题连续演进时，SHALL 继续以话题归属（`persistent_topic_id`）串联跨天 section，标题仅表达当天内容，两者职责分离不互相替代。

#### Scenario: 历史数据不回刷

- **GIVEN** 变更生效前已存在的 section 其 `cluster_label` 为旧话题名
- **WHEN** 变更部署完成
- **THEN** 该 section 的 `cluster_label` SHALL 保持不变

#### Scenario: 时间线跨天串联不依赖标题一致

- **GIVEN** 同一话题连续三天的三个 section 标题各不相同（每天内容不同）
- **WHEN** 用户在时间线查看该话题的演进
- **THEN** 三个 section SHALL 通过相同 `persistent_topic_id` 归并为同一话题链

