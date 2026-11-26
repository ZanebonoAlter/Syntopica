<!-- domain: daily-report, semantic-board -->


<!-- domain: daily-report, semantic-board -->

## Purpose
日报系统替代旧叙事系统，为每个 SemanticBoard 每日生成结构化日报，包含今日重点、板块动态和聚类叙事线索。

## Requirements

### Requirement: 日报数据模型

系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。事件数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, story_id(可空，只读历史列), scope(single/composite), is_development(布尔，默认 false), story_manual(布尔，只读历史列), display_tier(body/flash/hidden，默认 body), created_at。

`story_id` SHALL 保留为**只读历史列**：新生成或重建的日报 SHALL 恒为 NULL，归属判定链 SHALL NOT 再写入该列；历史日报中已存在的 `story_id` 与其指向的 `board_stories` 行 SHALL 保留不动，读路径 MAY 解析存活身份但 SHALL NOT 对外透出事件线身份。`scope` SHALL 继续由 `tag_ids` 中 `category='event'` 的标签数推导：`≥2` 为 `composite`，否则为 `single`；`composite` 事件 SHALL NOT 进入任何同事件聚簇，也 SHALL NOT 计入聚簇文章数。`is_development` SHALL 恒为 false（其唯一驱动源——事件线阶段判定——已退役），SHALL NOT 作任何判定输入。`story_manual` SHALL 保留为只读历史列（人工指派 / 拆分过的历史行仍为 true），SHALL NOT 触发任何判定或写动作。`display_tier` SHALL 恒为 `body`（唯一写入方——依赖 `story_id` 的跨期去重——已退役），SHALL NOT 影响任何计数、排序或展示过滤。

`highlights` SHALL 恒为数组。每项保留 title/reason/tag_ids，并新增可空 `cluster_id`（当天同事件聚簇 id，服务端计算，模型不输出）；同事件聚簇模式还 SHALL 保存由服务端计算的 `article_count`（簇内文章数，按内容身份去重）、`cluster_mode`（向量聚簇 / 语义向量兜底 / 主体近似）、`data_as_of`（数据截至日）与 `research_at`（研究时点）快照，SHALL NOT 接受模型填写这些数值。标签回退与历史数据 MAY 缺这些字段。正常目标 2–3 项，候选不足允许 1 项、事实不足允许 []。`cluster_id` 读取时 SHALL 同版块一跳解析，坏引用置空，不回写历史 JSON；数值 SHALL 保持生成时快照。

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
- **WHEN** 两条写路径（正常情况下由生成闸互斥；异常绕过闸时）先后或并发为 SemanticBoard #5 落库 2026-05-25 的日报
- **THEN** 系统 SHALL 通过原子 upsert 收敛为一条记录（后写覆盖先写），SHALL NOT 出现 duplicate key 唯一约束错误，SHALL NOT 产生部分写入的残缺日报（sections 与主记录同事务落库）

#### Scenario: 日报关联昨日报告
- **WHEN** SemanticBoard #5 在 2026-05-24 有一条已完成日报 (id=42)
- **THEN** 2026-05-25 的日报记录 SHALL 设置 prev_report_id=42

#### Scenario: 线程存储在独立表中
- **WHEN** 日报生成完成
- **THEN** 每个聚类的叙事事件 SHALL 作为独立行存储在 `daily_report_threads` 表中，通过 `section_id` 关联到对应的 section

#### Scenario: 兄弟桶共享聚类序号

- **GIVEN** 某聚类被泳道内主题分桶拆成 3 个桶
- **WHEN** 日报落库
- **THEN** 3 条 section SHALL 共享同一 `cluster_index`，`bucket_index` 分别为 0/1/2，`bucket_title` 各自为主题方向名

#### Scenario: 未分桶 section 取默认桶序号

- **GIVEN** 某聚类未达到分桶阈值（事件 < 6 且 tags < 10）
- **WHEN** 日报落库
- **THEN** 该 section SHALL 为 `bucket_index=0`、`bucket_title=NULL`，且其 `cluster_index` 在该 report 内仍然唯一

#### Scenario: highlights 落库恒为数组
- **WHEN** 日报生成时未产出任何 highlight（合法空降级或生成失败降级）
- **THEN** `board_daily_reports.highlights` SHALL 落 JSON 空数组 `[]`，SHALL NOT 落 JSON `null` 标量

#### Scenario: 历史 null highlights 读出归一为数组
- **WHEN** 读取一篇 `highlights` 列为 JSON `null` 的历史日报（2026-11-03 前的存量坏数据）
- **THEN** API 返回的 `highlights` SHALL 为 `[]`，SHALL NOT 为 `null`

#### Scenario: 事件落库时 scope 由事件标签数推导
- **WHEN** 某事件的 `tag_ids` 含 3 个 `category='event'` 标签
- **THEN** 该事件落库时 `scope` SHALL 为 `composite`，`story_id` SHALL 保持 NULL

#### Scenario: 新日报不写事件身份
- **WHEN** 生成或重建任意一期的日报
- **THEN** 该期所有 thread 的 `story_id` SHALL 为 NULL，`story_manual` SHALL 为 false，`display_tier` SHALL 为 `body`
- **AND** 系统 SHALL NOT 新建、合并或改写任何 `board_stories` / `story_signals` 行

#### Scenario: 旧事件身份只读保留
- **WHEN** 读取一篇历史日报，其 thread 带非空 `story_id` 且指向存活的 `board_stories` 行
- **THEN** 该历史值 SHALL 原样保留在库中，SHALL NOT 被清空或改写
- **AND** 对外响应 SHALL NOT 透出事件线身份字段

#### Scenario: 事件未归组不影响日报
- **WHEN** 某事件的 `story_id` 为 NULL
- **THEN** 该事件 SHALL 照常在日报正文展示，SHALL NOT 被过滤或隐藏

### Requirement: 事件标签去重
系统 SHALL 在生成日报前对收集到的事件标签进行程序化精确去重，不使用 LLM。去重 SHALL 应用两条规则：(1) 关联文章集合完全相同的标签合并为一个；(2) article_count=1 且关联同一篇文章的标签合并为一个。合并时存活者 SHALL 按以下优先级选出：

1. article_count 更大者存活；
2. article_count 相同时，tag identity embedding 与当前版块 embedding 的 cosine 更高者存活（版块相关性 tiebreak）；
3. cosine 相同或任一 embedding 缺失时，id 更小者存活（回退现行为）。

去重 SHALL 不改变原始标签数据，仅在生成流程中使用去重后的列表。

#### Scenario: 文章集合完全相同的标签去重
- **WHEN** 标签 A 和标签 B 关联的文章 ID 集合均为 {101, 102, 103}
- **THEN** 系统 SHALL 将二者合并为一个（按三级 tiebreak 保留存活者）

#### Scenario: 单篇文章标签去重
- **WHEN** 标签 X (article_count=1, 关联文章 200) 和标签 Y (article_count=1, 关联文章 200) 为不同标签但指向同一文章
- **THEN** 系统 SHALL 将二者合并为一个（存活者按三级 tiebreak 选出）

#### Scenario: 同文章双事件标签保留贴近版块者
- **WHEN** 同一篇 digest 文章拆出事件标签 X（如「OpenAI 营收口径反转」，identity 与当前版块 cosine=0.60）和事件标签 Y（如「特朗普与普京达成柴油供应协议」，identity 与当前版块 cosine=0.80），二者 article_count 均为 1 且关联同一文章
- **THEN** 系统 SHALL 保留 Y（cosine 更高，贴近当前版块），丢弃 X

#### Scenario: embedding 缺失回退 ID 顺序
- **WHEN** 标签 X 与 Y 同文章、article_count 相同，且任一标签无 identity embedding 或版块无 embedding
- **THEN** 系统 SHALL 保留 id 更小者（现行为兜底）

#### Scenario: 去重不影响原始数据
- **WHEN** 去重流程执行后
- **THEN** `topic_tags` 表中的原始标签记录 SHALL 保持不变，去重仅在内存中进行

### Requirement: 日报质量筛选
`collectBoardTags` 查询携带 `match_reason` 和 `score`（包括 fallback 路径产生的标签）。生成管线 SHALL 在聚类前按以下规则筛选标签：

1. **跨版块排他**：同一 tag 在其他版块存在 `score ≥ 本版块 score + exclusive_gap`（`semantic_board_report_exclusive_gap`，默认 0.3，ai_settings 可配；设为 0 时排他关闭）的挂载时，本版块 SHALL NOT 收该标签。排他剔除的标签 SHALL NOT 参与后续任何保底回捞（「有更强归属」是确定性判断，与弱规则保底语义不同）。主查询与 fallback 路径（现场计算匹配后择本版块结果）SHALL 应用同一规则。
2. 过滤 `direction_mismatch = true`
3. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`
4. 过滤 `weighted`（最弱规则）
5. 如果剩余 < 10 个标签，把 weighted 也拉回来（保底；被排他剔除的标签不回捞）
6. 如果剩余 > 30 个标签，按 `(tier, score)` 排序后截断到 top-30

Fallback 标签同等对待：fallback 路径产生的标签也携带 `match_reason`/`score`，筛选规则完全一致（含跨版块排他）。**【2026-11-12 本 change 起生效】筛选不再读取降级标记**（`downgraded` 列已退役）——保留的 max_sim 标签不再分「降级/非降级」两档。

#### Scenario: 跨版块排他剔除弱归属标签
- **WHEN** tag「美国非农数据超预期降温」挂载「美国新闻」score=1.0（max_sim）与「中国国内新闻」score=0.7（direct_hit），分差 0.3 ≥ exclusive_gap=0.3，生成「中国国内新闻」日报
- **THEN** 「中国国内新闻」SHALL NOT 收该标签

#### Scenario: 排他不影响分差不足的双挂载
- **WHEN** tag「中美贸易谈判」挂载「美国新闻」score=0.8 与「中国国内新闻」score=0.7，分差 0.1 < exclusive_gap=0.3
- **THEN** 两个版块日报 SHALL 均可收该标签（真跨界场景保留多版块呈现）

#### Scenario: 排他剔除不参与保底回捞
- **WHEN** 某版块当日收集 12 个标签，其中 3 个被跨版块排他剔除、5 个为 weighted，剩余 4 个 (< 10)
- **THEN** 系统 SHALL 把 weighted 拉回（9 个进入聚类），被排他剔除的 3 个标签 SHALL NOT 回捞

#### Scenario: 质量筛选过滤 weighted
- **WHEN** 收集到 20 个标签，其中 5 个 weighted
- **THEN** 系统 SHALL 过滤掉 weighted 标签，15 个进入聚类

#### Scenario: 保底机制
- **WHEN** 收集到 12 个标签，过滤 weighted 后只剩 8 个 (< 10)
- **THEN** 系统 SHALL 把 weighted 标签拉回，12 个全部进入聚类

#### Scenario: 截断机制
- **WHEN** 收集到 40 个标签，过滤后剩 35 个 (> 30)
- **THEN** 系统 SHALL 按 (tier, score) 排序后截断到 top-30

#### Scenario: fallback 路径同样应用排他
- **WHEN** fallback 现场计算某无挂载 tag 的匹配，结果含「美国新闻」score=1.0 与「中国国内新闻」score=0.7，当前正在生成「中国国内新闻」日报
- **THEN** fallback SHALL NOT 把该标签以 0.7 分收进「中国国内新闻」候选

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
- `BestTier int`：该 section 中所有 tag 的最高 tier（按 match_reason 映射：direct_hit=0 / hit_rate=1 / max_sim=2 / weighted=3）
- `AvgScore float64`：该 section 中所有 tag 的 score 平均值

前端用 `best_tier ASC, avg_score DESC` 排序聚类。**【2026-11-12 本 change 起生效】tier 映射不再考虑降级标记**（`downgraded` 列已退役）——原「降级 max_sim」档并入普通 max_sim 档（tier 2），max_sim 的 tier 不再因降级而后移。

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
系统 SHALL 保持 Call C×K 并行，Call A SHALL 改为落库、归属与跨期去重之后执行：
- **Call A（今日重点）**：输入当天同事件聚簇 Top 8 候选及其标签事实（composite 线索同权，见「今日重点聚簇候选」），输出含 title/reason/tag_ids 的重点；聚簇零候选时 SHALL 选择既有标签聚类回退模式，cluster_id 为空
- **Call C×K（聚类叙事线索）**：每个聚类一次调用，输入该聚类标签，输出 0-N 条线索（仅 title + summary + tag_ids + confidence）

Call C 生成完成后，系统 SHALL 将线程作为 `daily_report_threads` 表的行持久化。

`GenerateDailyReport` 函数 SHALL 返回 `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)`，其中第三项为每个 cluster 对应的 `[]DailyReportThread` 列表，供 `SaveReport` 批量写入 `daily_report_threads` 表。`[][]DailyReportThread` 中的索引 SHALL 与 `[]DailyReportSection` 一一对应。

Thread 生成 SHALL 移除 status、prev_thread_id 相关的 prompt 要求和 JSON schema 字段。Thread 的 system prompt SHALL 简化为仅要求输出 title、summary、tag_ids、confidence。

promptVersion SHALL 从当前 6.0 升级未使用版本，以区分旧并行与新故事候选合同。

#### Scenario: 并行生成成功
- **WHEN** 有 5 个聚类
- **THEN** 系统 SHALL 并行发起 Call C×5，在落库后发起一次 Call A（既有非法响应重试除外），SHALL NOT 提前调用或异步二次刷新重点

#### Scenario: Thread 输出不含 status
- **WHEN** Call C 为某聚类生成 3 条 thread
- **THEN** 每条 thread SHALL 只包含 title、summary、tag_ids、confidence，不包含 status 和 prev_thread_id

#### Scenario: 昨日日报不存在
- **WHEN** 某板某日为首次生成日报
- **THEN** Call A 的"昨日日报"输入 SHALL 为空，Call C 不传入任何历史线索上下文（已移除 getPrevThreadSummaries 调用）

### Requirement: 日报生成编排流水线
系统 SHALL 提供 `GenerateDailyReport(ctx, boardID, date)` 编排函数，按顺序执行：收集板内事件标签 → 质量筛选 → 去重 → LLM 分组(带组数限制) → 查询昨日日报 → 并行生成(Call C×K) → **泳道内主题分桶**（见「泳道内主题分桶」需求；未达阈值或分桶失败时保持单 section） → section 内容化 embedding 生成（文本来源见 section-content-embedding 能力，基于该 section 所聚 tag 的 label/description/代表文章摘录，而非 cluster_label 标题文本；分桶产出的每条 section 以其自身桶内 tag 计算 embedding） → 同日 section 两阶段合并（分桶产出的兄弟 section 之间 SHALL NOT 合并） → **Watch 物化追加**（keyword_topic / sentence_topic 物化轨按 watch-materialized-topic 能力产出追加 section；任一关注的物化失败 SHALL 降级跳过，SHALL NOT 阻断流水线） → section embedding 匹配写入关系表 → 组装 BoardDailyReport + DailyReportSection(含 best_tier/avg_score) → SaveReport → 有界尽力归属 → 跨期去重 → 热度候选 → highlights → 同步写入 title/summary。生成 SHALL 通过 goroutine 异步执行。

物化追加的 section SHALL NOT 参与同日合并，SHALL NOT 参与 section 关系计算。

流水线 SHALL NOT 执行 thread 级别的 tag 交集匹配或 prev_thread_id 赋值。

#### Scenario: 完整流水线执行

- **WHEN** 触发 SemanticBoard #5 在 2026-05-25 的日报生成
- **THEN** 系统 SHALL 按序执行：收集标签 → 质量筛选 → 去重 → LLM分组 → 查询昨日日报 → 并行生成(Call C×K) → 泳道内主题分桶 → 内容化 embedding 生成 → 同日合并 → Watch 物化追加 → section embedding 匹配写入关系表 → SaveReport → 有界尽力归属 → 跨期去重 → 热度候选 → highlights → title/summary → 既有完成终态

#### Scenario: 物化失败不阻断流水线

- **GIVEN** board #5 有一个 active 的 sentence_topic 关注
- **WHEN** 该关注的辅助标签检索在生成中失败
- **THEN** 系统 SHALL 跳过该关注的当期物化并记录日志，日报 SHALL 正常完成并保存，status SHALL NOT 为 failed

#### Scenario: 生成失败

- **WHEN** 正文关键步骤不可恢复地失败（不含本需求明确的归属/去重/重点降级）
- **THEN** 系统 SHALL 设置 status="failed"，保留已完成的中间结果（raw_clusters 等），WS 广播失败状态

#### Scenario: 分桶失败不阻断流水线

- **GIVEN** 某聚类达到分桶阈值
- **WHEN** 分桶的两个 LLM 阶段任一失败或返回非法 JSON
- **THEN** 系统 SHALL 记录 warn 日志并保持该聚类为单 section（`bucket_index=0`、`bucket_title=NULL`），日报 SHALL 正常完成，status SHALL NOT 为 failed

归属预算 SHALL 为每份报告 60s，超时/错误即停止该同步阶段；取消 SHALL 传入外部调用与写库，SHALL NOT 留下迟到归属写入。归属失败、去重失败、候选读取失败或无可定位事件线时 SHALL 走标签重点回退，SHALL NOT 阻断报告。LLM SHALL NOT 在 DB 事务中执行，生成终态 SHALL 在重点最终写入后发出。SHALL 移除重复 detached 归属派发，不改变 watch 命中独立失败边界。

#### Scenario: 归属失败标签回退仍出报告
- **WHEN** 归属报错，且本期有标签事实、标签模式 LLM 返回合法非空重点
- **THEN** 报告 SHALL 正常完成并保存非空重点，story_id SHALL 为空，title/summary SHALL 取首条重点，SHALL NOT 额外调用故事模式

#### Scenario: 归属超时不迟到写入
- **WHEN** 归属依赖阻塞到 60s 预算耗尽
- **THEN** SHALL 取消归属后继续标签回退，SHALL NOT 在取消后提交新的归属写入，正文 SHALL 保留

#### Scenario: 同板同日重建可重算
- **WHEN** 相同证据、截止时刻与固定模型响应下同板同日重建两次
- **THEN** SHALL 仅保留一份报告、本轮线程及同一候选/重点结果，无累计 signals/旧 highlights，迟到上一轮最终写入 SHALL 被修订条件拒绝

### Requirement: 日报存储
系统 SHALL 提供 `SaveReport`（创建或更新日报+关联 sections）、`GetReport(boardID, date)`（查询单篇）、`ListReports(boardID, days)`（查询列表）三个存储接口。

`ListReports` SHALL 在 `days <= 0` 时使用 7 天默认值。对于任意正数 `days`，系统 SHALL 按请求天数查询，不得静默截断到 30 天。

同 board 同日重跑 SHALL 继续整份覆盖 sections/threads，但 SHALL 保留独立保存的人工关键标记和已处理推荐记录。来源替换 SHALL 标明失效并保留确认快照，SHALL NOT 通过相似度把旧确认转移到其他新条目。

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

#### Scenario: 重生成保留选择而不移植确认
- **WHEN** 同日重生成替换旧 sections/threads，原条目有已确认或已处理建议
- **THEN** 选择记录仍在；非同一来源的新条目不自动继承确认；已处理的同一来源建议不重新推送

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

泳道内主题分桶产出的兄弟 section SHALL 继承其来源聚类的 `lane_tier`、`persistent_topic_id` 与 `topic_match_confidence`，并各自以其自身 embedding 计算 `topic_match_distance`。

#### Scenario: section 记录 lane 来源

- **GIVEN** 某 section 由 L1 直挂产生
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 l1_direct，topic_match_confidence 为 anchor_hit

#### Scenario: 物化 section 记录物化来源

- **GIVEN** 关键字物化追加产出一个 section
- **WHEN** section 持久化
- **THEN** lane_tier SHALL 为 watch_keyword，persistent_topic_id SHALL 为空

#### Scenario: 分桶兄弟继承归属与来源

- **GIVEN** 某 L1 直挂聚类（topic #502，lane_tier=l1_direct，confidence=anchor_hit）被拆成 3 条 section
- **WHEN** 3 条 section 持久化
- **THEN** 三者 SHALL 均为 lane_tier=l1_direct、persistent_topic_id=502、confidence=anchor_hit
- **AND** 三者的 `topic_match_distance` SHALL 各自按其 embedding 与该话题质心计算（允许不同）

### Requirement: 日报聚类裁决 prompt 历史隔离
L2 泳道裁决（`buildL2Prompt`，operation `daily_report.decide_l2_tags`）的 LLM prompt SHALL NOT 注入候选话题的历史叙事文案（`daily_report_threads` 的 title / summary），切断「昨天幻觉 thread → 今天作为 briefs 喂回 → LLM 延续叙事」的渗透闭环。

L2 裁决 prompt MAY 注入以下**非叙事**信号辅助裁决：候选 topic 的 `label`、状态（active / candidate）、最近命中日期、累计命中天数、质心余弦距离、近期 section 的框架名（`cluster_label` / `section_label`，话题命名级别，**非** thread 文案）。

L2 system prompt 的裁决依据措辞 SHALL 与实际注入内容保持一致——基于「标签语义与近期 section 框架」，而非「实际近期内容（thread 文案）」，避免 prompt 自相矛盾误导 LLM。

`promptVersion` SHALL 单调递增且已发布版本 SHALL NOT 复用："3.0" → "4.0"（本约束生效）、"5.0"（section 展示标题内容化，2026-08-29）、"6.0"（泳道内主题分桶，本 change）。

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
- **THEN** `board_daily_reports.generation_prompt_version` SHALL 为 "6.0"

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

### Requirement: 泳道内主题分桶
系统 SHALL 在 lane 归属分桶之后、section 组装之前，对**单个聚类内部**做主题方向分桶（intra-lane theme bucketing），把一条过宽的 section 拆成 2-6 条同归属的 section。分桶 SHALL NOT 跨聚类进行，SHALL NOT 改变任何 tag 的话题归属（`persistent_topic_id` / `lane_tier` / `topic_match_confidence` 由上游 lane 分桶决定）。

**触发阈值**：当某聚类的当日线索数 ≥ `daily_report_intra_bucket_min_threads`（默认 6）**或** 其 tag 数 ≥ `daily_report_intra_bucket_min_tags`（默认 10）时触发；总开关 `daily_report_intra_bucket_enabled`（默认开启）关闭时 SHALL 全局保持单 section。

**输入粒度**：分桶输入 SHALL 为该聚类当日的 thread 线索（标题 + 摘要 + 关联 tag），SHALL NOT 以原始 tag 列表逐条作为分组单位（tag 粒度会碎化，实测产出单例桶）。

**两阶段**：
1. **定组名**：组数 k SHALL 由代码计算（`k = clamp(round(线索数 / 2.5), 2, 6)`），并写入 prompt；LLM 只负责给出 k 个主题方向名（≤12 字）与每个方向的当日章节标题（≤30 字）。模型返回的组数不等于 k 时，系统 SHALL 做确定性收敛（合并最小的两组，或从尾部裁剪多余组）后再次校验。
2. **归属**：以第一阶段的方向名作为固定集合，LLM 只做逐条归属，SHALL NOT 新增、改名、合并或拆分分组；温度 SHALL 为 0。

**校验与修复**（确定性，不依赖模型自觉）：归属结果 SHALL 满足 ①每条线索恰好出现一次 ②每组线索数 ≥2 ③组数 ≤6。未覆盖的线索 SHALL 并入与其**最相近的组**——相近度用该线索的 tag 集合与该组 tag 并集的 Jaccard 重叠度衡量（无 embedding 时的确定性语义代理）；重叠度并列或无任何重叠时 SHALL 并入线索数最少的组（并列取组序较早者）；重复出现的线索 SHALL 只保留首处；单例组 SHALL 按同一相近度合并进其他组（无重叠时并入线索数最少者）；**空组（0 条线索）SHALL 直接丢弃、不产出 section**；修复后仍只有 1 组时 SHALL 回退为单 section。

**产出**：每个保留下来的组产出一条 section，共享来源聚类的 `cluster_index`，`bucket_index` 按组顺序 0..k-1，`bucket_title` 为主题方向名，`cluster_label` 为该组当日章节标题（SHALL 遵守「日报文案生成事实锚约束」，且 SHALL NOT 复述泳道/话题名；当日章节标题缺失时，展示标题默底链按**该桶内线索**执行：桶内首条 thread 标题 → 泳道 label，SHALL NOT 回退到整簇其他桶的内容），`cluster_tag_ids` 为该组线索关联 tag 的并集；**桶内线索均无 tag id 时 SHALL 回退为来源聚类的 tag 集**（避免产出无法计算 embedding / 无法归属的空壳 section）。

#### Scenario: 域型宽聚类被拆成多个主题

- **GIVEN** board #3030 某聚类含 18 个 tag、17 条 thread（做游戏、插件、MCP、harness、Skill、安全、推理基础设施混杂）
- **WHEN** 执行泳道内主题分桶
- **THEN** 系统 SHALL 产出 2-6 条 section，各自 `bucket_title` 为一个主题方向名、`cluster_label` 为该组当日章节标题，全部 section 共享同一 `cluster_index` 与同一 `persistent_topic_id`

#### Scenario: 未达阈值不触发分桶

- **GIVEN** 某聚类含 4 条 thread、5 个 tag
- **WHEN** 执行泳道内主题分桶
- **THEN** 系统 SHALL NOT 调用分桶 LLM，保持单 section（`bucket_index=0`、`bucket_title=NULL`）

#### Scenario: 归属漏掉线索被修复归位

- **GIVEN** 归属阶段返回的分组漏掉 2 条线索
- **WHEN** 系统执行确定性校验
- **THEN** 漏掉的线索 SHALL 被并入与其语义最近的组，最终覆盖率 SHALL 为 100%，SHALL NOT 丢线索

#### Scenario: 单例组被合并

- **GIVEN** 归属结果中某组只有 1 条线索
- **WHEN** 系统执行确定性校验
- **THEN** 该组 SHALL 被合并进线索主题最近的其他组，最终每组线索数 SHALL ≥2

#### Scenario: 全组收敛为一组时回退

- **GIVEN** 校验修复后只剩 1 个组
- **WHEN** 系统组装 section
- **THEN** SHALL 回退为单 section（`bucket_index=0`、`bucket_title=NULL`），行为与未分桶一致

#### Scenario: 分桶产物不参与同日合并

- **GIVEN** 某聚类被拆成 3 条兄弟 section，且 `daily_report_section_merge_enabled=true`
- **WHEN** 执行同日 section 相似合并
- **THEN** 三条兄弟 section SHALL NOT 被合并回一条

#### Scenario: 组数指令不被模型遵守时收敛

- **GIVEN** 代码计算的 k=4，LLM 实际返回 6 个方向名
- **WHEN** 系统校验组名集合
- **THEN** 系统 SHALL 按确定性规则合并到 4 组后再进入归属阶段，SHALL NOT 直接把 6 组写入日报

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

### Requirement: 今日重点聚簇候选
系统 SHALL 在日报落库尾部为每版块生成「今日重点」候选：当天线索命中的 event 标签按 related-event 向量相似度聚簇（向量聚簇 → 语义向量兜底 → 主体近似三级降级），候选排序 SHALL 为簇内内容身份去重文章数 DESC → 簇内标签数 DESC → 代表标签 id ASC，最多保留 8 条。LLM SHALL 只从候选白名单内选至多 3 条写 title/reason，SHALL NOT 编造事实或数字——簇文章数、簇身份等数值快照一律由服务端计算补齐（`cluster_id` / `article_count` / `cluster_mode` / `data_as_of` / `research_at`），展示序即服务端候选序。

**聚簇输入 SHALL NOT 因线索 `scope=composite` 被排除（本 change 起）**：composite 线索（同事件多表述标签聚合）的 event 标签 SHALL 与 single 线索同权参与聚簇。跨事件混编标签 SHALL 由聚簇相似度门槛隔离（自成小簇、不并入单事件簇）；composite「SHALL NOT 计入事件数、SHALL NOT 建立事件线」的既有语义 SHALL 不变。当天线索均无 event 标签或聚簇产出零候选时 SHALL 走标签回退模式（cluster_id 为空、快照字段缺省、不伪造篇数）。

#### Scenario: 单事件多标签大簇进候选
- **WHEN** 某线索聚合 17 个同事件不同表述的 event 标签、关联 20 篇文章（scope=composite），当天另有多个 1 篇文章的单标签线索
- **THEN** 聚簇 SHALL 将这 17 个标签聚为一簇、簇内去重文章数为 20 篇级，该簇 SHALL 排候选白名单首位

#### Scenario: 混编标签自成小簇不污染
- **WHEN** 某混编标签横跨 5 个事件共 12 篇文章，与任一单事件标签的向量距离均超聚簇阈值
- **THEN** 该标签 SHALL 自成一簇、SHALL NOT 并入任何单事件簇，其簇文章数按自身关联文章去重计算

#### Scenario: 全 composite 日报不再走标签回退
- **WHEN** 当天全部线索 scope 均为 composite 且均含 event 标签
- **THEN** 聚簇 SHALL 正常产出候选，SHALL NOT 仅因线索为 composite 而进入标签回退模式

#### Scenario: 候选按簇内文章数截断
- **WHEN** 本期聚出 10 个簇且去重文章数排序明确，第 9 簇的标签数最多
- **THEN** LLM SHALL 只收到前 8 簇的事实，第 9 簇 SHALL NOT 因标签数进入候选

#### Scenario: 模型只从白名单挑且不输出数字
- **WHEN** 模型返回白名单外的簇身份或自填 article_count 数值
- **THEN** SHALL 按既有非法语义同参重试，数值快照 SHALL 仅由服务端计算落库

#### Scenario: 零候选走标签回退
- **WHEN** 当天线索均无 event 标签或聚簇产出为空
- **THEN** SHALL 走标签回退模式，cluster_id 为空、快照字段缺省不伪造篇数，正文/归属/事件数不受影响

#### Scenario: 旧重点快照读取兼容
- **WHEN** 读取本 change 之前落库的重点（旧聚簇口径或标签回退产物）
- **THEN** 快照字段 SHALL 原样返回、不回填不重算；该日报被重建时 SHALL 按新口径重算重点

### Requirement: 当天同事件聚簇先筛重点候选

系统 SHALL 在本 report 当天同事件聚簇中去重筛选「今日重点」候选。候选 SHALL 为该版块当天日报命中的 event 标签聚成的簇（口径与降级链见 `related-events`「日报同事件聚簇」），每个候选 SHALL 附代表标签、label/description、簇内文章数（按内容身份去重）与聚簇口径标记；排序 SHALL 按簇内文章数降序 → 簇内 event 标签数降序 → 聚簇代表标签 id 升序，最多 8 条，不按四舍五入后的篇数排序、不用版块 section 数或泳道热度替代。

LLM SHALL 只从这 8 条以内候选中选至多 3 条不同聚簇写 title/reason，SHALL NOT 编造事实、数字、情绪或因果。模型返回的 `cluster_id` SHALL 属当次候选白名单，`tag_ids` SHALL 属该聚簇的本期标签白名单；过滤后空 tag、空白标题理由、集外 id 或类型错误 SHALL 按既有语义非法响应同参重试一次；合法 `[]` 不重试，最终失败仍 `[]`。重复聚簇 SHALL 保留首条。

文章数、聚簇口径、数据截至日与研究时点 SHALL 由服务端补齐快照，SHALL NOT 由模型输出。候选不足允许 1 条，零候选 SHALL 在调用前选择标签回退模式（`cluster_id` 为空、缺省快照字段），SHALL NOT 伪造身份或编造聚簇。旧报告 SHALL NOT 回填补造聚簇身份或篇数快照。

#### Scenario: 候选按簇内文章数截断
- **WHEN** 本期有 10 个候选聚簇，簇内文章数排序明确，第 9 名拥有最多 section
- **THEN** LLM SHALL 只收到排序前 8 个候选的事实与篇数，第 9 名 SHALL NOT 因 section 数进入候选

#### Scenario: 同分与精度不反转
- **WHEN** 两个候选簇内文章数相同，或四舍五入后相同但原值不同
- **THEN** SHALL 依次按簇内 event 标签数降序、代表标签 id 升序破同分，未舍入篇数更高者仍排前

#### Scenario: 只可从候选事实写文案
- **WHEN** 模型返回集外 `cluster_id` 或仅含别的聚簇 tag_ids 的重点
- **THEN** SHALL 判非法并按既有上限重试，SHALL NOT 将该条作为合法重点落库

#### Scenario: 无聚簇走标签回退
- **WHEN** 当天全部为综合稿、无候选 event 标签，或全部候选无正文落点
- **THEN** SHALL 选择现有标签聚类重点路径，`cluster_id` 为空，正文与统计字段 SHALL NOT 为了凑候选被修改

#### Scenario: 少量候选与合法空返回
- **WHEN** 只有一条候选或候选事实不足导致模型返回 `[]`
- **THEN** SHALL 分别允许一条重点或合法空数组，SHALL NOT 为凑 2 条编造内容或重试合法 `[]`

#### Scenario: 重点快照与旧数据兼容
- **WHEN** 读取新聚簇重点、旧 tag_ids-only 重点或 JSON null
- **THEN** 新重点 SHALL 返回生成时来源、篇数、聚簇口径与时点快照，旧重点 SHALL 不伪造快照，JSON null SHALL 归一为 `[]`，首条重点仍为报告 title/summary 来源，无重点仍用既有兜底

### Requirement: 日报线索文章携带疑似 AI 标记并排序后置

日报线索线程关联的文章 SHALL 携带该文章的 AI 生成疑似度判定状态，供日报前端在展开线索文章时呈现「疑似 AI 生成」标记。

线索线程的文章集合 SHALL 保持为该线程标签所对应文章的并集，SHALL NOT 因疑似标记而移除或剔除任何文章；线索的「N 篇」计数 SHALL NOT 因疑似标记而变化。展开线索文章时的呈现顺序 SHALL 为：非疑似文章在前、疑似文章稳定排至末尾；SHALL NOT 打乱非疑似文章之间的既有相对顺序。

该标记仅影响展开后的呈现顺序与视觉标记，SHALL NOT 改变「是否进入日报」的判定口径，SHALL NOT 影响日报文案生成的事实锚。

#### Scenario: 线索文章展开带出疑似标记

- **WHEN** 用户展开一条日报线索的文章列表，且其中某篇文章判定状态为疑似 AI 生成
- **THEN** 该文章 SHALL 呈现「疑似 AI 生成」标记

#### Scenario: 疑似文章排至展开列表末尾

- **WHEN** 一条线索关联 5 篇文章，其中 2 篇判定状态为疑似 AI 生成
- **THEN** 展开后 3 篇非疑似文章 SHALL 排在前面，2 篇疑似文章 SHALL 稳定排至末尾，且 5 篇的相对成员 SHALL 不变

#### Scenario: 篇数计数不受影响

- **WHEN** 一条线索的关联文章中有文章被标记为疑似 AI 生成
- **THEN** 该线索显示的「N 篇」计数 SHALL 与标记前的篇数一致

#### Scenario: 未判定文章不呈现标记

- **WHEN** 线索关联文章的判定状态为未判定
- **THEN** 展开列表 SHALL 不呈现疑似 AI 标记，SHALL NOT 出现占位文案

### Requirement: 候选标签的疑似占比参与名额竞争排序

日报候选标签 SHALL 可携带其名下当天文章的 AI 生成疑似度占比。日报质量筛选在候选标签数量超过上限而需要截断时，SHALL 把疑似占比作为排序的次级因子：占比高的标签 SHALL 排在占比低的标签之后；SHALL NOT 因此把任何标签移出候选集合。

疑似占比 SHALL 按比例而非按单篇命中计算：仅含个别疑似文章的标签 SHALL NOT 与全部文章均疑似的标签同等对待。

该因子 SHALL 只影响名额竞争时的截断排序，SHALL NOT 改变标签去重规则、泳道分桶规则、阈值口径与「进日报」判定口径。

#### Scenario: 占比高的标签在截断时靠后

- **WHEN** 日报候选标签数量超过上限需要截断，两个标签的匹配层级与分数相同，其中标签 A 的疑似占比为 1.0、标签 B 的疑似占比为 0.1
- **THEN** 截断后 SHALL 保留标签 B 而优先舍弃标签 A

#### Scenario: 按比例而非单篇命中计算

- **WHEN** 标签 A 名下 10 篇文章中 1 篇疑似 AI 生成，标签 B 名下 2 篇文章中 2 篇疑似 AI 生成
- **THEN** 标签 A 的疑似占比 SHALL 为 0.1、标签 B 的疑似占比 SHALL 为 1.0，SHALL NOT 因命中篇数相同而同等对待

#### Scenario: 不影响候选成员与去重口径

- **WHEN** 日报候选标签的疑似占比存在差异
- **THEN** 候选标签的成员集合 SHALL NOT 因疑似占比而改变，标签去重与泳道分桶结果 SHALL NOT 受该因子影响

### Requirement: 日报收集对无板块归属 tag 兜底补算

<!-- from: daily-report-match-fallback -->

系统 SHALL 在 `collectBoardTags` 执行时，对有辅助标签但无 `topic_tag_board_labels` 记录的 event tag 执行现场补算。补算 SHALL 调用 `MatchTopicTag`，匹配结果写入 `topic_tag_board_labels`，然后合并到日报输入。

#### Scenario: 正常情况不触发兜底
- **WHEN** 日期 2026-05-27 的所有 event tag 都有 `topic_tag_board_labels` 记录
- **THEN** `collectBoardTags` SHALL NOT 执行兜底补算，直接使用现有匹配结果

#### Scenario: 存在未匹配 tag 时兜底补算
- **WHEN** board #2853 在 2026-05-27 有 10 个 event tag，其中 3 个有辅助标签但无 `topic_tag_board_labels`
- **THEN** `collectBoardTags` SHALL 对这 3 个 tag 调用 `MatchTopicTag` 补算，将匹配到当前 board 的 tag 合并到日报输入中

#### Scenario: 补算失败不阻塞日报生成
- **WHEN** 兜底补算中某个 tag 的 `MatchTopicTag` 调用失败
- **THEN** 系统 SHALL 记录 warning 日志，跳过该 tag，继续生成日报

#### Scenario: 补算后 tag 仍未匹配到目标 board
- **WHEN** 兜底补算后某个 tag 匹配到了其他 board 但不匹配目标 board
- **THEN** 该 tag SHALL NOT 出现在目标 board 的日报中

### Requirement: 日报排除 direction_mismatch 标签

<!-- from: daily-report-match-fallback -->

collectBoardTags 的主查询和 fallback 补算均排除 direction_mismatch=true 的标签。主查询添加 `AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)` 条件。fallback 补算后过滤匹配结果时排除 direction_mismatch=true。

#### Scenario: 主查询排除 direction_mismatch
- **WHEN** tag 在 topic_tag_board_labels 中 direction_mismatch=true
- **THEN** tag 被排除出日报主查询结果

#### Scenario: fallback 排除 direction_mismatch
- **WHEN** fallback 补算产生的匹配结果 direction_mismatch=true
- **THEN** 该 tag 被排除出日报
