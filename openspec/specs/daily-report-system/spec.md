
## Purpose
日报系统替代旧叙事系统，为每个 SemanticBoard 每日生成结构化日报，包含今日重点、板块动态和聚类叙事线索。

## Requirements

### Requirement: 日报数据模型
系统 SHALL 使用 `board_daily_reports` 和 `daily_report_sections` 两张表承载日报。每个 SemanticBoard 每天至多一条 `BoardDailyReport` 记录。

**BoardDailyReport 字段**：id, semantic_board_id, period_date, title, summary, highlights(JSON), dynamics(TEXT), article_count, event_tag_count, cluster_count, status(generating/done/failed), raw_clusters(JSON), prev_report_id(可为空，指向前一日日报), generation_prompt_version, created_at, updated_at。

**DailyReportSection 字段**：id, report_id, cluster_index, cluster_label, cluster_tag_ids(JSON), article_count, best_tier, avg_score, embedding(向量列，维度由模型输出决定, 用于语义匹配), bucket_title(可空，主题方向名), bucket_index(整数，默认 0), created_at。事件数据已迁移至 `daily_report_threads` 表，通过 `section_id` 外键关联。跨天关系通过 `daily_report_section_relations` 关系表表达，status 通过关系拓扑动态推导。

**DailyReportThread 字段**：id, report_id, section_id, title, summary, tag_ids(JSONB), confidence, story_id(可空), scope(single/composite), is_development(布尔，默认 false), story_manual(布尔，默认 false), display_tier(body/flash/hidden, 默认 body), created_at。

`story_id` SHALL 指向 `board_stories` 的存活记录或经 `merged_into` 解析到存活记录；该列 SHALL 可空、SHALL NOT 设外键级联删除、SHALL NOT 参与事件的去重与展示过滤。`scope` SHALL 由 `tag_ids` 中 `category='event'` 的标签数推导：`≥2` 为 `composite`，否则为 `single`；`composite` 事件 SHALL NOT 建立或加入事件线，也 SHALL NOT 计入事件数。`is_development` SHALL 仅在判定为「同一事件的不同阶段」时置真。`story_manual` SHALL 标记该事件的身份由人工决定：人工指派时置真并写 `story_id`，人工拆分时置真且 `story_id` 保持 NULL；任何自动归属判定与历史回填 SHALL 跳过 `story_manual=true` 的事件，SHALL NOT 改写其 `story_id`。`display_tier` SHALL 为跨期去重的展示层载体（`body`=正文默认、`flash`=快讯、`hidden`=未展示预留）：身份、signals 与热度 SHALL NOT 读该列；本期日报详情读路径 SHALL 只返回 `body`（`flash`/`hidden` 丢弃不渲染，字段保留供未来快讯 UI）。

`highlights` SHALL 恒为数组。每项保留 title/reason/tag_ids，新增可空 story_id；故事模式还 SHALL 保存由服务端计算的 participant_count、story_heat、heat_rule_version、heat_as_of、heat_incomplete 快照，不接受模型填写这些数值。标签回退与历史数据 MAY 缺这些字段。正常目标 2–3 项，候选不足允许 1 项、事实不足允许 []。story_id 读取时 SHALL 同版块一跳解析存活身份，坏引用置空，不回写历史 JSON；数值 SHALL 保持生成时快照。

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

#### Scenario: 事件未归组不影响日报
- **WHEN** 某事件的 `story_id` 为 NULL（未回填或判定失败）
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
3. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`（含 downgraded）
4. 过滤 `weighted`（最弱规则）
5. 如果剩余 < 10 个标签，把 weighted 也拉回来（保底；被排他剔除的标签不回捞）
6. 如果剩余 > 30 个标签，按 `(tier, score)` 排序后截断到 top-30

Fallback 标签同等对待：fallback 路径产生的标签也携带 `match_reason`/`score`，筛选规则完全一致（含跨版块排他）。

#### Scenario: 跨版块排他剔除弱归属标签
- **WHEN** tag「美国非农数据超预期降温」挂载「美国新闻」score=1.0（max_sim）与「中国国内新闻」score=0.7（direct_hit），分差 0.3 ≥ exclusive_gap=0.3，生成「中国国内新闻」日报
- **THEN** 该标签 SHALL NOT 进入「中国国内新闻」日报的聚类候选（「美国新闻」日报不受影响）

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
系统 SHALL 保持 Call C×K 并行，Call A SHALL 改为落库、归属与跨期去重之后执行：
- **Call A（今日重点）**：输入热度 top 8 事件线及其本期标签事实，输出含 story_id/title/reason/tag_ids 的重点；归属失败或无可用线时 SHALL 选择既有标签聚类回退模式，story_id 为空
- **Call C×K（聚类叙事线索）**：每个聚类一次调用，输入该聚类标签，输出 0-N 条线索（仅 title + summary + tag_ids + confidence）

Call C 生成完成后，系统 SHALL 将线程作为 `daily_report_threads` 表的行持久化。

`GenerateDailyReport` 函数 SHALL 返回 `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)`，其中第三项为每个 cluster 对应的 `[]DailyReportThread` 列表，供 `SaveReport` 批量写入 `daily_report_threads` 表。`[][]DailyReportThread` 中的索引 SHALL 与 `[]DailyReportSection` 一一对应。

Thread 生成 SHALL 移除 status、prev_thread_id 相关的 prompt 要求和 JSON schema 字段。Thread 的 system prompt SHALL 简化为仅要求输出 title、summary、tag_ids、confidence。

promptVersion SHALL 从当前 6.0 升级未使用版本，以区分旧并行与新故事候选合同。

#### Scenario: 并行生成成功
- **WHEN** 有 5 个聚类
- **THEN** 系统 SHALL 并行发起 Call C×5，在落库、归属与去重后发起一次 Call A（既有非法响应重试除外），SHALL NOT 提前调用或异步二次刷新重点

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

### Requirement: 日报生成 API — 异步触发
系统 SHALL 提供 `POST /api/daily-reports/generate` 端点，接受 `{date: string, board_id?: number, board_ids?: number[]}` 参数。board_id 为空时生成所有活跃 board 的日报。端点 SHALL 立即返回 `{job_id: string, status: "processing"}`，后台 goroutine 异步执行生成。

端点 SHALL 额外接受可选 `board_ids` 数组表示**多板块集合作用域**：非空时后台仅为该集合内的板块生成日报，一次请求 SHALL 只占用一个执行闸、只返回一个 `job_id`、只广播一次终态、只发送一次终态通知。`board_id` 与 `board_ids` SHALL 互斥：两者同时非空时端点 SHALL 返回 HTTP 400 且 SHALL NOT 启动任何管线。`board_ids` 为空数组或缺省时，SHALL 保持「生成所有活跃 board」的既有语义。集合作用域下的板块 SHALL 逐个生成，任一板块失败 SHALL NOT 中断其余板块，最终失败计数 SHALL 体现在终态通知中。

集合内的重复板块 id SHALL 按集合语义折叠（保留首次出现顺序，每个 id 至多生成一次），SHALL NOT 因重复 id 产生重复生成或重复计入终态计数。`board_id` / `board_ids` 任一字段类型非法（如 `board_ids` 传入非数组、`board_id` 传入非数字）时，端点 SHALL 返回 HTTP 400 且 SHALL NOT 降级为全量作用域——畸形请求 SHALL NOT 触发全站生成管线。

同一时刻 SHALL 至多存在一条日报生成管线，互斥范围 SHALL 覆盖共享同一 PostgreSQL 数据库的**全部后端实例**（定时调度或手动触发，无论发起自哪个实例，共享同一把数据库级执行闸）。已有管线在跑时，手动触发（本端点及调度器 `TriggerNowWithDate`）SHALL NOT 启动新管线，SHALL 返回 409 与 `reason: "already_running"`（文案：「日报生成正在执行中，请稍后再试。」）；定时调度路径获取闸失败时 SHALL 记 skip（reason `already_running`），闸由持闸管线在结束时释放（获取失败方无锁可释放，SHALL NOT 调用释放）。闸在管线结束（成功、失败或 panic）后 MUST 释放；持闸进程崩溃或连接断开时，数据库级闸 SHALL 随连接终结自动释放，SHALL NOT 造成永久死锁。数据库不可达导致闸无法探测时，SHALL 回退进程内互斥（单实例行为不劣于现状），SHALL NOT 因闸探测失败而阻断生成。闸探测失败路径 SHALL 归还该次探测占用的数据库连接，SHALL NOT 泄漏连接池连接。被拒触发 SHALL NOT 产生任何完成/失败终态通知。

#### Scenario: 触发单板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_id: 5}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台开始为 board #5 生成日报

#### Scenario: 触发全板日报生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25"}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台依次为所有活跃 board 生成日报

#### Scenario: 触发多板块集合生成
- **WHEN** 请求 `POST /api/daily-reports/generate {date: "2026-05-25", board_ids: [3, 7, 9]}`
- **THEN** 系统 SHALL 立即返回 `{job_id: "xxx", status: "processing"}`，后台仅为 board #3、#7、#9 生成日报，SHALL NOT 为集合外的板块生成

#### Scenario: 多板块集合共用一个执行闸与一次终态
- **WHEN** 请求带 `board_ids: [3, 7]` 的生成
- **THEN** 整个集合 SHALL 只占用一个执行闸并只返回一个 `job_id`，SHALL 只广播一次 `daily_report_done` 终态消息，SHALL 只发送一条终态通知

#### Scenario: 集合内单板失败不影响其余板块
- **WHEN** 请求带 `board_ids: [3, 7]` 的生成，其中 board #7 生成失败
- **THEN** 系统 SHALL 继续为 board #3 完成生成，终态通知 SHALL 同时体现成功与失败计数，SHALL NOT 因单板失败丢弃其余板块结果

#### Scenario: 单板块与集合参数互斥
- **WHEN** 请求同时携带 `board_id` 与非空 `board_ids`
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 启动任何生成管线、SHALL NOT 获取执行闸

#### Scenario: 集合内重复板块折叠
- **WHEN** 请求携带 `board_ids: [3, 7, 3]`
- **THEN** 端点 SHALL 只为 board #3 与 #7 各生成一次，SHALL NOT 重复生成 #3，终态计数 SHALL NOT 重复计入 #3

#### Scenario: 集合字段类型非法不降级为全量
- **WHEN** 请求携带类型非法的 `board_ids`（如传入对象而非数组）
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 按全量作用域启动生成管线、SHALL NOT 获取执行闸

#### Scenario: 单板块字段类型非法不降级为全量
- **WHEN** 请求携带类型非法的 `board_id`（如传入字符串而非数字）
- **THEN** 端点 SHALL 返回 HTTP 400 且 SHALL NOT 按全量作用域启动生成管线、SHALL NOT 获取执行闸

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

### Requirement: 日报生成 WebSocket 进度广播
生成 goroutine SHALL 通过 `ws.GetHub().BroadcastRaw()` 广播两类消息：
- `daily_report_progress`：每完成一个 board 后广播 `{"type": "daily_report_progress", "job_id": "...", "board_id": N, "board_name": "...", "status": "completed|failed", "saved": N, "progress": "current/total"}`
- `daily_report_done`：全部完成后广播 `{"type": "daily_report_done", "job_id": "...", "total_saved": N, "total_boards": N, "failed_count": N, "empty_count": N}`

`daily_report_done` SHALL 自带本次运行的**三态计数**：`total_saved`（产出日报的板块数）、`failed_count`（生成报错、真正失败的板块数）、`empty_count`（当日无内容、`report == nil` 的板块数）。三态互斥且穷尽于本次运行涉及的板块，`total_saved + failed_count + empty_count == total_boards` SHALL 恒成立。`failed_count` / `empty_count` SHALL 与终态通知口径一致（`notification.DailyReportTerminal`）：当日无内容的板块既不计入成功数、也不计入失败数。消费方 SHALL 直接读 `failed_count`，SHALL NOT 用 `total_boards - total_saved` 反推失败数——那会把当日无内容的板块错算成失败，与通知中心口径不一致。`failed_count` / `empty_count` 为新增字段：只读 `total_saved` / `total_boards` 的既有消费方 SHALL 继续正常工作。

#### Scenario: 单板生成完成广播
- **WHEN** board #5 的日报生成成功
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_progress", "job_id": "xxx", "board_id": 5, "board_name": "AI与机器学习", "status": "completed", "saved": 1, "progress": "1/1"}`

#### Scenario: 全部生成完成广播
- **WHEN** 3 个 board 的日报全部生成完毕
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 3, "total_boards": 3, "failed_count": 0, "empty_count": 0}`

#### Scenario: 终态消息给出三态计数
- **WHEN** 一次生成涉及 3 个 board，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 1, "total_boards": 3, "failed_count": 1, "empty_count": 1}`，且 `total_saved + failed_count + empty_count == total_boards` 成立

#### Scenario: 当日无内容的板块既不算成功也不算失败
- **WHEN** 某 board 当日无内容（生成无报错但 `report == nil`）
- **THEN** 该 board SHALL 计入终态消息的 `empty_count`，SHALL NOT 计入 `failed_count`、SHALL NOT 计入 `total_saved`；终态通知的成功/失败计数 SHALL 与终态消息同口径（无内容的板块不进失败数）

#### Scenario: 单板块生成失败
- **WHEN** board #5 的日报生成报错
- **THEN** 系统 SHALL 广播 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 0, "total_boards": 1, "failed_count": 1, "empty_count": 0}`，SHALL NOT 把该板块计入 `empty_count`

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
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件，展示板块日报列表，并以**阅读台**（`/tags/:boardId/:date`）呈现选中日报。阅读台 SHALL 沿用旧版阅读层的**三栏**：左侧目录栏（本期目录 / 持续追踪 / 历史日报 / 版块切换）、正文（刊头 + 分区正文）、宽屏下常驻的右侧页边注栏；MUST NOT 渲染已撤下的右栏演进/本期话题。页边注在 <1100px SHALL 收为右下浮动入口 + 抽屉。日期历史 SHALL 由左侧目录栏的历史列表承载（「加载更早」增量），不再使用日期下拉。

**入口与容器**：阅读台 SHALL 作为独立路由页面渲染，取代原先 `position: fixed; inset: 0` 的全屏浮层。离开阅读台（返回按钮或 Esc）SHALL 回到该版块的矩阵视图，并保留进入前的选中泳道。阅读台 SHALL 在页面容器内整体滚动（顶栏 sticky，左目录与页边注栏各自 sticky），MUST NOT 把正文锁进固定高度的小滚动窗。

**宽屏布局**：桌面宽屏（>1100px）SHALL 采用三栏——左目录定宽、正文自适应铺满剩余宽度（MUST NOT 再压成 40rem 居中窄栏）、右侧页边注栏 `clamp(15rem, 17vw, 17rem)`；刊头（含头条与「今日重点」最多 3 条 highlights）SHALL 在正文上方全宽展示，MUST NOT 隐藏 highlights。1100px 及以下 SHALL 单栏（页边注收为右下浮动按钮 + 抽屉）；767.98px 及以下 SHALL 单栏且不得产生页面级横向溢出。

**主题**：阅读台的 surface、文字、边框、阴影和交互状态 SHALL 使用现有 semantic theme token，并同时支持 editorial/dark。topic 的稳定 `persistent_topic.color` MAY 用作身份强调色。组件 SHALL NOT 以固定浅色背景为前提硬编码黑色透明色。

**内容映射**：
1. masthead 展示真实日报标题、日期、article_count 和 cluster_count；刊头 SHALL NOT 固定声称"号外"。
2. 头条优先取 `highlights[0]`；highlights 为空时回退到质量最高的 section，且 SHALL NOT 生成 API 中不存在的新闻文案。
3. highlights 按 API 顺序最多展示三项；为空时不渲染该区块。
4. section 按 `persistent_topic.status` 分为"关心的话题"(active)、"突发的新话题"(candidate/已归属但未 active)和"其他动态"(未归属)。分区内继续按 best_tier 升序、avg_score 降序排列。
5. `dynamics` 为空时不渲染"板块动态"区块。

**目录与日期联动**：左侧目录栏 SHALL 提供「头条」「话题正文」锚点、持续追踪话题、历史日报日期列表与版块切换；历史日报 SHALL 按日期倒序列出当前已加载日报，点击 SHALL 切换日期；「加载更早」SHALL 扩充该列表的日期范围。分区索引下拉与 scroll-spy 已于 2026-10-27 用户复审撤下，阅读台 SHALL NOT 依赖它们。

**话题卡片与 mini 生命线**：active 话题卡片 SHALL 支持原位展开。首次展开调用 `getTopicLifeline(topicId)`，展示以当前日报日期为终点的最近七个自然日；加载成功后按 topic id 缓存，重复展开不再次请求。加载、失败、重试和空状态 SHALL 明确可见。

mini 生命线 SHALL 按日期列展示真实 section，同日多 section 合并为一个节点并显示数量。连线只使用响应中 `relation_type="identity"` 的真实关系，采用贝塞尔路径连接端点；空白日期不生成假节点，系统 SHALL NOT 根据日期相邻关系臆造连线或混入 similarity relation。节点点击 SHALL 原位展开当日 threads。

原「话题总览」入口与 desk 索引里的「在矩阵中查看这条线」入口已于 2026-10-27 复审随新版索引一并撤下；泳道/矩阵聚焦入口保留在版块态势页自身，阅读台不再内嵌该出口。

**thread 文章**：thread 有 `related_article_ids` 时 SHALL 可原位展开文章标题列表。前端只加载尚未缓存的 article id，相同文章跨 thread 复用缓存；单篇失败不阻塞其他文章并提供重试。点击文章 SHALL 直接打开站内阅读预览弹窗（详情拉取期间弹窗呈现加载态、失败给出可见错误提示），并保留阅读台上下文。

**section 生命周期**：点击 section header SHALL 继续打开右侧 SectionLifecyclePanel，不因新增 topic mini 生命线而移除 section 维度入口。

"加载更早" SHALL 每次将 `days` 增加 7 并重新请求该完整时间范围。组件 SHALL 用响应替换当前列表，避免累计请求造成重复日报。切换 board SHALL 将 `days` 重置为 14（阅读台按 14 天展示历史列表，可继续「加载更早」），并清理与旧 board 相关的展开状态。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 打开全屏日报详情
<!-- 保留主 spec 的 Scenario 身份锚以维持旧测试资产追溯；本 change 的设计标签为「打开日报阅读台」，行为以以下 WHEN/THEN 为准。此处仅修复标题兼容，不代表日报设计已随矩阵阶段扩批或交付。 -->
- **WHEN** 用户点击某日报卡片或从版块态势页点击「打开今日日报」
- **THEN** 路由 SHALL 变为 `/tags/:boardId/:date` 并渲染占满视口的三栏阅读台（左目录 / 正文 / 页边注）
- **AND** 返回后 SHALL 回到该版块矩阵并保留进入前的选中泳道

#### Scenario: 宽屏双栏与窄屏降级
<!-- 保留主 spec 的历史 Scenario 身份锚；本 change 拟议的「宽屏三栏与窄屏降级」仍由以下 WHEN/THEN 描述，本轮不改正文、不扩大矩阵阶段的 UI 批准范围。 -->
- **WHEN** 分别以 1920px、1100px 和 720px viewport 打开同一日报
- **THEN** 1920px SHALL 显示「左目录 + 宽屏正文 + 右页边注栏」三栏，刊头与「今日重点」top3 可见，不出现右栏演进
- **AND** 1100px SHALL 单栏并把页边注收为右下浮动入口 + 抽屉
- **AND** 720px SHALL 单栏且不产生页面级横向溢出

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
- **AND** 点击文章 SHALL 直接打开站内阅读预览弹窗并保留阅读台上下文

#### Scenario: 进入完整话题生命线
- **WHEN** 用户点击 active topic 的「在矩阵中查看这条线」
- **THEN** 系统 SHALL 返回版块态势页并选中该泳道；无 topic id 时该入口 SHALL 隐藏

#### Scenario: 键盘关闭详情
- **WHEN** 阅读台打开且用户按 Esc
- **THEN** 系统 SHALL 返回矩阵视图，并将焦点恢复到打开阅读台的入口

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"并提供生成日报入口

#### Scenario: 加载超过 30 天
- **WHEN** 当前 `days=28`，用户连续两次点击"加载更早"
- **THEN** 组件 SHALL 依次以 `days=35` 和 `days=42` 请求，并展示对应范围内的日报

#### Scenario: 切换 board 重置状态
- **WHEN** 用户从一个 board 切换到另一个 board
- **THEN** 组件 SHALL 将 `days` 重置为 14、加载新 board 日报，并清理旧 board 的 topic/thread 展开状态

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

### Requirement: 缺档日报自动补档
定时日报任务在生成完当天报告后 SHALL 自动补档：在保留窗口内（`tag_edge_retention_days`，与标签边回收同键同口径）逐日检查每个当日应生成报告的板块是否存在 `(board, period_date)` 报告，缺失则按既有生成流水线重建（幂等 upsert，只补缺失、不重建已存在报告）。补档 SHALL 以处理队列已清空（无 pending/leased 的打标任务）为前置条件；队列未清空时本轮 SHALL 跳过补档并于下一轮重试，缺档在窗口内 SHALL NOT 因顺延而丢失。

#### Scenario: 停机缺档次日自动补齐

- **GIVEN** 系统停机 3 天，那 3 天的日报缺失，恢复后打标积压已清空
- **WHEN** 次日 21:00 定时日报任务执行
- **THEN** 生成当天报告后自动重建窗口内缺失的 3 天报告，零人工干预

#### Scenario: 队列未清空顺延

- **GIVEN** 恢复当天 21:00 时打标队列仍有 pending 任务
- **WHEN** 定时日报任务执行
- **THEN** 当天报告照常生成，补档本轮跳过，次日队列清空后再补

#### Scenario: 只补缺不重建已有

- **GIVEN** 窗口内某板块昨日报告已存在
- **WHEN** 补档扫描执行
- **THEN** 该 (board, date) 被跳过，既有报告不被重建

### Requirement: 日报重建窗口守卫
对早于保留窗口下界的日期，系统 SHALL 拒绝重建日报：`POST /api/daily-reports/generate` 返回错误（4xx，说明标签边已按窗口回收、候选不全），调度器 `TriggerNowWithDate` 同口径拒绝。目的：防止超窗日期的"空报告覆盖好报告"（同日重建是整份覆盖语义）。窗口内日期的手动重建 SHALL 不受影响。

#### Scenario: 超窗日期拒绝重建

- **GIVEN** 保留窗口 7 天，请求重建 10 天前的日报
- **WHEN** 调用 POST /api/daily-reports/generate {date: 10天前}
- **THEN** 返回 4xx 错误并说明窗口约束，既有报告（若存在）不被覆盖

#### Scenario: 窗口内日期正常重建

- **GIVEN** 保留窗口 7 天，请求重建 3 天前的日报
- **WHEN** 调用 POST /api/daily-reports/generate {date: 3天前}
- **THEN** 照常异步触发重建

#### Scenario: 调度器指定日期触发同口径

- **WHEN** 经 TriggerNowWithDate 触发早于窗口下界的日期
- **THEN** 拒绝执行（accepted=false），与 API 守卫口径一致

### Requirement: 日报推荐与历史兼容
日报读取 SHALL 以可选附加信息区分未处理建议与已确认标记，不重命名原 thread 字段或改变原文章展开行为。新日报推荐 SHALL 不阻塞日报保存及终态；历史缺字段 SHALL 降为无建议，但仍可从标记读取结果恢复已确认状态。读取历史 SHALL NOT 触发批量 AI 回刷。

#### Scenario: 旧日报可直接手动标记
- **WHEN** 打开没有推荐字段的旧日报且条目仍具有效泳道归属
- **THEN** 正常阅读并可手动标记，不调用 AI 回刷；已有持久标记仍可显示

#### Scenario: 推荐失败不改变日报终态
- **WHEN** 日报已保存而推荐失败
- **THEN** 报告仍为成功，不发第二条日报终态通知，不删除人工节点

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

### Requirement: 重新分桶 API
系统 SHALL 提供 `POST /api/daily-reports/:id/rebucket`，请求体含 `section_id`（该日报内任一条 section），用于对**该 section 所在的桶组**（同一 `report_id` + `cluster_index`）重跑泳道内主题分桶。

服务端 SHALL：先取与日报重跑**同键**的事务级串行锁（`(semantic_board_id, period_date)` 粒度），并在锁内复核报告状态；以组内全部 thread 为输入重跑两阶段分桶；按新分桶结果替换该组的 section 行（保留/更新原有行以尽量减少 ID 变化，新增不足的桶行，删除多余的桶行）；thread 行 SHALL 复用并把 `section_id` 迁到新桶，SHALL NOT 删除 thread（关键事件注记因此不丢）；`topic_key_event_markers.source_section_id` SHALL 按 `source_thread_id` 重新派生；`topic_watch_hits.section_id` SHALL 随所在 section 的替换同步迁移或按组删除重建，SHALL NOT 留下悬空命中行；`report_annotations.section_id` SHALL 按 `thread_id` 重新派生（无 thread 的报头注记挂该组 `bucket_index=0` 的 section）；重新计算每条 section 的 `cluster_label` / `bucket_title` / `bucket_index` / `cluster_tag_ids` / `article_count` / `embedding` / `topic_match_distance` / `best_tier` / `avg_score` / `quality_breakdown`（并沿用报告期的 `topic_status_at_report` 快照）；其中 `article_count` SHALL 按该桶线索 `related_article_ids` 的**去重并集**重算（重分桶只持有 thread 事实、不重新选文，故与生成期「Σ tag 文章数」口径存在有意差异，属展示计数）；`best_tier` / `avg_score` / `quality_breakdown` SHALL 取自报告期冻结的 `quality_breakdown` 快照按桶内 tag 子集重算（不重新打分）；重算该话题质心；**双向**重建该组涉及的 section 关系（含跨天入边的反向引用）。

前置门槛：报告 status 非 `done` SHALL 返回 409；目标 section 的 `lane_tier` 为 `watch_keyword` / `watch_sentence` 时 SHALL 返回 400（物化组由关注轨自己管理，不接受主题分桶）；未达「泳道内主题分桶」触发阈值时 SHALL 直接返回 200 + `buckets=1` 且不改动任何数据。

失败（LLM 失败、解析失败、数据库事务失败）SHALL 整体回滚并返回错误；「校验后收敛为 1 组」SHALL NOT 视为失败（同上，200 + `buckets=1` 且不改数据）。响应 SHALL 返回该组最新的 section 列表与 `buckets` 计数。

#### Scenario: 重新分桶替换一组 section

- **GIVEN** 日报 #100 的 `cluster_index=3` 组当前为 1 条 18-tag 的 section
- **WHEN** 调用 `POST /api/daily-reports/100/rebucket` 传入该 section 的 id
- **THEN** 响应 SHALL 返回 2-6 条 section，全部 `cluster_index=3`，原 thread 行的 `section_id` SHALL 已指向新桶

#### Scenario: 重新分桶保留关键事件注记

- **GIVEN** 该组某条 thread 上有 confirmed 关键事件注记
- **WHEN** 执行重新分桶
- **THEN** 注记 SHALL 仍然存在，其 `source_thread_id` SHALL 不变，`source_section_id` SHALL 重新指向该 thread 现在所属的 section

#### Scenario: 拆不动时返回单桶且不改数据

- **GIVEN** 该组内容确实只有一个主题方向
- **WHEN** 调用重新分桶
- **THEN** 响应 SHALL 为 200 且 `buckets=1`，数据库 SHALL NOT 发生 section 替换

#### Scenario: 失败整体回滚

- **GIVEN** 重新分桶的归属阶段 LLM 调用失败
- **WHEN** 请求返回错误
- **THEN** 该组 section 与 thread 的 `section_id` SHALL 保持调用前状态，SHALL NOT 出现部分替换

#### Scenario: 拒绝非完成态日报

- **GIVEN** 日报 #100 的 status 为 generating
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 409，SHALL NOT 修改任何数据

#### Scenario: 拒绝 watch 物化组

- **GIVEN** 目标 section 的 `lane_tier=watch_sentence`
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 400，SHALL NOT 修改任何数据

#### Scenario: 未达阈值不改数据

- **GIVEN** 目标组只有 4 条 thread、5 个 tag（低于阈值）
- **WHEN** 调用重新分桶
- **THEN** 系统 SHALL 返回 200 + `buckets=1`，SHALL NOT 调用分桶 LLM，SHALL NOT 修改任何 section/thread 行

#### Scenario: 与同日重跑并发时串行化

- **GIVEN** 同一 board 同一天的重跑（SaveReport）与 rebucket 并发发起
- **WHEN** 两者竞争
- **THEN** 两者 SHALL 通过同键事务级 advisory lock 串行化，不出现「thread 已被重跑删除、rebucket 的 UPDATE 静默影响 0 行」或「向已重建报告插入旧 cluster_index 幽灵 section」

#### Scenario: watch 命中与页边注跟随重派生

- **GIVEN** 目标组关联了 `topic_watch_hits` 命中行与 `report_annotations` 页边注
- **WHEN** 重新分桶替换该组 section
- **THEN** `topic_watch_hits.section_id` SHALL 指向 thread 现在所属的 section（或按组重建），SHALL NOT 悬空
- **AND** `report_annotations.section_id` SHALL 按 `thread_id` 重新派生，注记内容不变

### Requirement: 日报调度窗口的生成时刻与兜底时刻可读
调度器状态载荷 SHALL 为 `daily_report` 调度器同时暴露**生成时刻**（`schedule_time`，取 `ai_settings.daily_report_time`，缺省 21:00）与**强制兜底时刻**（`schedule_deadline`，取 `ai_settings.daily_report_deadline`，缺省 23:30）两个字段，使前端能够同时呈现「计划何时生成」与「最迟必须出结果」两个时刻。两个字段 SHALL 出现在 `/api/poll` 与 `/api/schedulers/status` 的 `daily_report` 条目上。兜底时刻早于生成时刻时 SHALL 沿用既有回落逻辑（回落到默认 23:30）并 SHALL NOT 输出非法值。

#### Scenario: 状态载荷同时给出两个时刻
- **WHEN** 请求 `GET /api/poll`
- **THEN** `data.schedulers` 中 `name` 为 `daily_report` 的条目 SHALL 同时包含 `schedule_time` 与 `schedule_deadline` 两个字段

#### Scenario: 兜底时刻缺省回落默认值
- **WHEN** `ai_settings` 中不存在 `daily_report_deadline` 配置行
- **THEN** `schedule_deadline` SHALL 为默认值 `23:30`

#### Scenario: 兜底时刻早于生成时刻时回落
- **WHEN** `ai_settings` 中 `daily_report_deadline` 早于 `daily_report_time`
- **THEN** `schedule_deadline` SHALL 回落到默认值 `23:30`，SHALL NOT 输出早于生成时刻的非法组合

### Requirement: 日报终态通知的计数口径与 WS 终态消息一致
通知中心的日报终态通知 SHALL 与 `daily_report_done` 终态消息共用同一套**三态计数口径**：通知文案中的「共 N 个版面」SHALL 等于终态消息的 `total_boards`（**含**当日无内容的板块），通知中的成功数 SHALL 等于 `total_saved`，失败数 SHALL 等于 `failed_count`。当日无内容（`report == nil`）的板块 SHALL NOT 计入成功数、SHALL NOT 计入失败数，SHALL 以「其中 M 个当日无内容」显式给出，且 M 计入 N。成功侧文案 SHALL 表达「产出日报的板块数」，SHALL NOT 把板块数表述为「条目数」。

该口径 SHALL 同时适用于**手动 / 集合生成路径**与**定时调度路径**（两者 SHALL 共用同一三态写入口 `notification.DailyReportTerminal`）：定时调度路径统计的板块集合 SHALL 完整计入该次运行的板块总数，SHALL NOT 因「不计成功也不计失败」而把当日无内容的板块排除在总数之外——否则同一个「生成日报」动作在两条路径下会给出不同的「共 N 个版面」。定时调度路径的终态通知 SHALL 与手动路径同口径；其 WS 终态消息（`daily_report_complete`）协议 SHALL 保持不变。定时调度路径的收集阶段早退 SHALL 记作「共 1 个版面，0 成功 1 失败」（`TotalBoards=1` / `Saved=0` / `Failed=1` / `Empty=0`），与手动路径的收集阶段失败口径一致。

一次运行 SHALL 至多产生一条终态通知：存在失败板块时 SHALL 只发一条失败汇总（SHALL NOT 另发完成通知），无失败板块时 SHALL 只发一条完成通知——两条通道互斥。

#### Scenario: 1 成功 + 1 失败 + 1 无内容 → 通知与 WS 的总版面数一致
- **WHEN** 一次生成涉及 3 个板块，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 终态消息 SHALL 为 `{"type": "daily_report_done", "job_id": "xxx", "total_saved": 1, "total_boards": 3, "failed_count": 1, "empty_count": 1}`，且终态通知 SHALL 表述为「共 3 个版面，1 成功 1 失败」并给出「其中 1 个当日无内容」，SHALL NOT 把当日无内容的板块计入失败数、SHALL NOT 从「共 N 个版面」中扣除

#### Scenario: 当日无内容的板块走完成通知
- **WHEN** 一次生成只涉及 1 个当日无内容的板块（无失败板块）
- **THEN** 系统 SHALL 只发一条完成通知（SHALL NOT 发失败汇总），通知 SHALL 表述为「共 1 个版面」并给出「其中 1 个当日无内容」，与终态消息的 `{"total_saved": 0, "total_boards": 1, "failed_count": 0, "empty_count": 1}` 同口径

#### Scenario: 收集阶段失败的通知口径
- **WHEN** 生成在收集板块阶段失败
- **THEN** 系统 SHALL 广播 `{"total_saved": 0, "total_boards": 1, "failed_count": 1, "empty_count": 0}` 并只发一条失败汇总通知「共 1 个版面，0 成功 1 失败」

#### Scenario: 定时调度路径与手动路径的总版面数一致
- **WHEN** 定时调度路径的一次运行涉及 3 个板块，其中 1 个产出日报、1 个生成报错、1 个当日无内容
- **THEN** 终态通知 SHALL 表述为「共 3 个版面，1 成功 1 失败」并给出「其中 1 个当日无内容」，SHALL 与手动 / 集合路径对同一次「生成日报」动作给出的口径一致，SHALL NOT 把当日无内容的板块从「共 N 个版面」中扣除

#### Scenario: 定时调度路径收集阶段失败的通知口径
- **WHEN** 定时调度路径在收集板块阶段失败
- **THEN** 系统 SHALL 只发一条失败汇总通知「共 1 个版面，0 成功 1 失败」，SHALL 与手动路径的收集阶段失败口径一致

#### Scenario: 当日无可生成板块不发终态通知
- **WHEN** 当日没有任何可生成的板块（`total_boards == 0`）
- **THEN** 系统 SHALL 只广播一次终态消息 `{"total_saved": 0, "total_boards": 0, "failed_count": 0, "empty_count": 0}`，SHALL NOT 产生终态通知

### Requirement: 站内阅读预览的全屏层必须盖过宿主浮层
文章预览弹窗（`ArticlePreviewModal`，日报阅读台 / 版块态势页 / 搜索页共用）内的「全屏」SHALL 展示覆盖整个视口的正文层，且该层 SHALL 盖过宿主弹窗遮罩——用户点「全屏」后 SHALL 立即看到全屏正文，SHALL NOT 仍停留在弹窗视图（表现为「点了没反应」）。全屏层层级 SHALL 由宿主显式传入（`ArticleContentView` 的 `fullscreenZIndex` prop），SHALL NOT 在正文组件内硬编码低于宿主遮罩的层级；阅读页原地全屏 SHALL 保持既有层级行为不变。

#### Scenario: 弹窗内进入全屏
- **WHEN** 用户在文章预览弹窗内点击「全屏」
- **THEN** 全屏正文层 SHALL 覆盖视口并显示在弹窗遮罩之上
- **AND** 视口中心命中元素 SHALL 落在全屏层内（而非弹窗内容区）

#### Scenario: 退出全屏
- **WHEN** 用户在全屏层内点击「退出全屏」
- **THEN** 系统 SHALL 回到弹窗内的常规阅读视图

#### Scenario: 阅读页原地全屏不回归
- **WHEN** 用户在主阅读页（非弹窗宿主）点击「全屏」
- **THEN** 全屏层 SHALL 保持既有层级行为（不被页面浮层压住）

### Requirement: 阅读台线索文章直达站内阅读预览

日报阅读台（`/tags/:boardId/:date`）内点击线索引用文章 SHALL **直接打开站内原文预览弹窗**（复用既有 `ArticlePreviewModal`），SHALL NOT 先弹出中间文章抽屉。打开弹窗前 SHALL 拉取文章详情：详情请求进行中弹窗 SHALL 呈现加载态；请求失败 SHALL 给出可见错误提示且不留空弹窗。预览弹窗内的收藏等状态动作 SHALL 真实落库并即时反映到界面：收藏 SHALL 采用乐观更新——点击后星标立即翻转，不等待网络往返；请求失败 SHALL 回滚到点击前状态并给出可见错误提示。SHALL NOT 出现「点击无响应」的空转实现（事件被空函数吞掉）或状态长时间不更新。

**适用范围**：本 Requirement 覆盖日报阅读台（`DailyReportDeskShell`）与版块矩阵页日报时间线（`useTagsPage.openArticlePreview`）两条预览入口；其他宿主的收藏入口沿用同一乐观更新契约。预览弹窗内部的全屏层级、内嵌网页高度等契约由既有 Requirement 承载，本 Requirement 不重复约束。

#### Scenario: 点击线索文章直达预览弹窗

- **WHEN** 用户在阅读台点击线索展开列表中的一篇引用文章
- **THEN** 系统 SHALL 直接打开站内阅读预览弹窗展示该文章
- **AND** SHALL NOT 出现中间文章抽屉层级

#### Scenario: 详情加载中呈现加载态

- **WHEN** 用户点击文章且详情请求尚未返回
- **THEN** 预览弹窗 SHALL 呈现加载态
- **AND** 请求返回后 SHALL 以完整文章内容替换加载态

#### Scenario: 详情加载失败可见反馈

- **WHEN** 文章详情请求失败
- **THEN** 系统 SHALL 给出可见的错误提示
- **AND** SHALL NOT 留下空白无内容的弹窗

#### Scenario: 站内阅读预览收藏即时生效

- **WHEN** 用户在日报阅读台打开的预览弹窗内点击工具栏的收藏
- **THEN** 星标 SHALL 立即切换为已收藏
- **AND** 系统 SHALL 调用文章更新接口持久化该收藏状态

#### Scenario: 收藏失败回滚并提示

- **WHEN** 收藏请求失败
- **THEN** 星标 SHALL 回滚为点击前的状态
- **AND** 系统 SHALL 给出可见的错误提示

#### Scenario: 连点不丢更新

- **WHEN** 用户在收藏请求返回前再次点击收藏
- **THEN** 界面 SHALL 以最新一次点击为准展示状态
- **AND** 迟到的失败响应 SHALL NOT 覆盖更新后的状态

#### Scenario: 预览内文章状态更新回流

- **WHEN** 预览弹窗发出文章状态更新
- **THEN** 弹窗当前文章 SHALL 应用该更新

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

### Requirement: 跨期重复降级

日报生成时 SHALL 利用事件线身份识别跨期重复：对每个候选事件，若其 `story_id` 非空且该事件线在最近 7 天已进入过日报正文，系统 SHALL 判定本次为「重述」或「进展」：

- **重述**：本事件的 `related_article_ids` 与事件线已有文章集合的重叠率 ≥ 0.6，且未出现新的独立来源（feed）→ SHALL 不进正文，降级为快讯或丢弃。
- **进展**：重叠率 < 0.6、或出现新 feed、或当事方自身有新动作（`is_development=true`）→ SHALL 进正文。
- `story_id` 为空或为综合稿的事件 SHALL 维持现有行为，SHALL NOT 因缺少身份而被误判为重复。

降级 SHALL 只作用于展示：事件 SHALL 照常落库、signals SHALL 照常写入、热度 SHALL 不受影响。降级结果 SHALL 写入 `display_tier`（`body`=正文、`flash`=快讯、`hidden`=未展示预留）并在读取路径可区分（本期日报详情只返回 `body`，`flash`/`hidden` 丢弃不渲染——字段保留供未来快讯 UI），SHALL NOT 删除历史记录。`story_manual=true`（人工指派）的事件 SHALL 不参与自动去重，保持 `body`。降级判定 SHALL 幂等：重复执行结果一致；条件变化（如新 feed 补入）时按当前集合重算，已降级事件可恢复 `body`。

#### Scenario: 重述降级为快讯
- **WHEN** 某事件线昨日已进正文，今日新事件的文章集合与之重叠 0.8 且无新 feed
- **THEN** 今日该事件 SHALL 不进正文（`display_tier='flash'`，本期读路径丢弃不渲染）；其 signals 与热度 SHALL 正常累计

#### Scenario: 新来源判为进展
- **WHEN** 某事件线昨日已进正文，今日新事件文章重叠 0.7 但引入了 1 个新 feed
- **THEN** 今日该事件 SHALL 进正文并标记为进展

#### Scenario: 当事方新动作判为进展
- **WHEN** 某事件线昨日已进正文，今日对应事件的 `is_development=true`
- **THEN** 该事件 SHALL 进正文，SHALL NOT 被降级

#### Scenario: 未归组事件不参与去重
- **WHEN** 某事件的 `story_id` 为 NULL
- **THEN** 该事件 SHALL 按现有逻辑展示，SHALL NOT 被判定为重述

#### Scenario: 降级不改变热量
- **WHEN** 日报把某事件从正文降级为快讯
- **THEN** 该事件线的 `story_signals` 与热度 SHALL 与降级前完全一致

#### Scenario: 读取路径只回正文线程
- **WHEN** 某日报含 `body`/`flash`/`hidden` 三种 display_tier 的事件
- **THEN** 日报详情读路径 SHALL 只返回 `body` 事件；`flash`/`hidden` 行 SHALL 仍在库中可查（供未来快讯 UI），直接数据库读取 SHALL 仍可见

### Requirement: 事件线热度先筛重点候选
系统 SHALL 在本 report 的 single、已归属且至少有一条可见 body thread 的存活事件线中去重筛选候选。热度 SHALL 复用 `heat-v1-48h-hl24h`，以 report.period_date 北京时间 23:59:59 为 t，窗口 `[t-48h,t]` 内每 participant_key 最后 observed_at 的 `0.5^((t-last_at)/24h)` 之和为 H、去重参与者数为 P。排序 SHALL 为原始 H 降序 → P 降序 → 存活 id 升序，最多 8 条，不按 rounded heat 排序、不用 lane_heat 或文章/分区数替代。

每个候选 SHALL 附标题、来源数、热度和本期标签 label/description/代表文章事实；LLM SHALL 只从这 8 条以内候选中选至多 3 条不同故事写 title/reason，SHALL NOT 编造事实、数字、情绪或因果。来源数与热度 SHALL 由服务端补齐快照。模型 story_id SHALL 属当次候选白名单，tag_ids SHALL 属该 story 的本期事实白名单；过滤后空 tag、空白标题理由、集外 id 或类型错误 SHALL 按既有语义非法响应同参重试一次；合法 [] 不重试，最终失败仍 []。重复 story SHALL 保留首条。候选不足允许 1 条，零候选 SHALL 在调用前选择标签回退，不伪造身份。

#### Scenario: 候选按热度而非桶数截断
- **WHEN** 本期有 10 条可用事件线且热度排序明确，第 9 条拥有最多 section
- **THEN** LLM SHALL 只收到排序前 8 条的事实与热度，第 9 条 SHALL NOT 因桶数进入候选

#### Scenario: 同分与精度不反转
- **WHEN** H 相同但 P 不同，或 H/P 均相同，或两个 H 四舍五入后相同但原值不同
- **THEN** SHALL 依次按 P 降序/id 升序破同分，未舍入 H 更高者仍排前

#### Scenario: 来源窗口与半衰期
- **WHEN** t 前 24h、48h、49h 各有不同 feed 报道，另有 t 后信号
- **THEN** H SHALL 为 0.75、P SHALL 为 2，49h 前与未来信号 SHALL NOT 计入，同 feed 多报道只计最后一次

#### Scenario: 只可从候选事实写文案
- **WHEN** 模型返回集外 story_id 或仅含别的故事 tag_ids 的重点
- **THEN** SHALL 判非法并按既有上限重试，SHALL NOT 将该条作为合法重点落库

#### Scenario: 无事件线走标签回退
- **WHEN** 今日全部 composite、全部未归属或所有已归属事件均无 body 落点
- **THEN** SHALL 选择现有标签聚类重点路径，story_id 为空，正文/归属/signals SHALL NOT 为了凑候选被修改

#### Scenario: 少量候选与合法空返回
- **WHEN** 只有一条候选或候选事实不足导致模型返回 []
- **THEN** SHALL 分别允许一条重点或合法空数组，SHALL NOT 为凑 2 条编造内容或重试合法 []

#### Scenario: 重点快照与旧数据兼容
- **WHEN** 读取新故事重点、旧 tag_ids-only 重点或 JSON null
- **THEN** 新重点 SHALL 返回生成时来源/热度/版本/截止时刻/不完整标记，旧重点 SHALL 不伪造快照，JSON null SHALL 归一为 []，首条重点仍为报告 title/summary 来源，无重点仍用既有兜底

### Requirement: 昨日重点延续徽标
每条重点的 `is_progress` SHALL 在详情读取时按同 board、精确前一北京时间日历日已完成报告的 highlights 集合派生。今天与昨天非空 story_id SHALL 同时经同版块 merged_into 一跳解析为存活 id 后比较。同线 SHALL 标「进展」，可读提示 SHALL 为「昨日重点再次入选」；无昨日、空/旧重点、坏引用或跨 board SHALL 不标。SHALL NOT 用最近一期代替昨日，SHALL NOT 用线程 is_development 代替集合比较或据此修改跨期去重。

#### Scenario: 昨日同线再入选
- **WHEN** 今日重点 story_id=10 且昨日重点含同一存活 id
- **THEN** SHALL 返回 is_progress=true 并在标题旁标「进展」，不改 thread.is_development

#### Scenario: 缺昨日不借前日
- **WHEN** 前日有同线重点但昨日无报告、无重点或仅旧 tag_ids
- **THEN** is_progress SHALL 为 false，即使 prev_report_id 指向前日也 SHALL NOT 标进展

#### Scenario: 合并与坏引用
- **WHEN** 昨日重点 A 已一跳合并进今日 B，另有一项指向坏引用/跨版块/异常多跳
- **THEN** A/B SHALL 视为同线，异常项 SHALL 保守不标，SHALL NOT 回写人工归属或历史 highlights

### Requirement: 重点依据与正文定位
前端 SHALL 在故事重点旁显示服务端快照「N 个来源」（0 是合法值），可读提示附 48h 热度、截止时刻及不完整标记。字段缺失 SHALL 省略数字而非伪造 0。带有效 story_id 且有正文目标的重点 SHALL 可键盘激活，定位本 report 当前正文 DOM 顺序中该线第一条 thread；必要时展开目标所在正文容器，滚动、聚焦并高亮 2s。锚点 SHALL 为 `report-thread-{thread.id}`，归并只读解析存活 id。

旧重点无 story_id SHALL 仅展示文案、不猜标签归属。无目标 SHALL 禁用定位并提示「本期正文暂无对应事件」；点击时目标消失 SHALL 原地轻提示，SHALL NOT 打开新详情页、近似跳 section 或自动重建。重复激活 SHALL 替换单个高亮与计时器，切 report/卸载清理；reduced-motion SHALL 不平滑滚动，划选 SHALL NOT 触发定位。

跨页寻址 SHALL 通过日报阅读台路由的 `story` 查询参数（`/tags/:boardId/:date?story=<story_id>`）：参数 SHALL 在该期日报详情就绪后，按存活身份解析并定位本 report 当前正文 DOM 顺序中该事件线第一条 thread，展开/滚动/聚焦/高亮 2s，与刊头重点定位同一路径。参数所指事件线在本期正文无落点时 SHALL NOT 滚动、SHALL NOT 跳到近似位置，SHALL 提示「本期正文暂无对应事件」。翻页或切换 report 后 SHALL 清理该参数，SHALL NOT 在其它报告重复定位；无效或非法的参数值 SHALL 忽略且 SHALL NOT 报错阻断阅读。

#### Scenario: 点击第一条可见正文线程
- **WHEN** 同一 story 在本期两个正文位置出现，用户点击或键盘激活重点
- **THEN** SHALL 展开需要展开的正文容器并滚动/聚焦 DOM 中第一条对应 thread，高亮 2s，SHALL NOT 按最小 id 或跳别的 report

#### Scenario: 失去目标与旧重点
- **WHEN** 重点无 story_id 或有 id 但正文已无对应 thread
- **THEN** 无 id 的重点 SHALL 保持静态文案；无目标的故事重点 SHALL 提示本期正文暂无对应事件，SHALL NOT 产生空跳转

#### Scenario: 重复定位与减弱动画
- **WHEN** 连续定位两条重点，随后切换 report，且用户设置 reduced-motion
- **THEN** SHALL 始终最多一个高亮，切换时清理，无平滑动画，划选文案 SHALL NOT 触发定位

#### Scenario: 展示筛选不改变热度
- **WHEN** 某线程被既有 7 天/0.6 去重规则降为 flash
- **THEN** 其 story SHALL 在无其他 body thread 时不进入可定位重点候选，但身份、signals、H/P SHALL 不变；昨日重点徽标 SHALL NOT 把该线程恢复 body

#### Scenario: 深链参数定位与失去目标
- **WHEN** 用户从雷达打开 `/tags/1974/2026-11-06?story=42`，该期正文含事件线 42 的 thread
- **THEN** 阅读台 SHALL 定位该事件线在正文 DOM 顺序中的第一条 thread 并高亮 2s
- **AND** 当该期正文已无事件线 42 的 thread 时 SHALL 提示「本期正文暂无对应事件」且不滚动，SHALL NOT 跳到近似位置
