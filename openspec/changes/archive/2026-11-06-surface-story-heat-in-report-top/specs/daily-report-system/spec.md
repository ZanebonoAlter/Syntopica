## MODIFIED Requirements

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


## ADDED Requirements

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

旧重点无 story_id SHALL 仅展示文案、不猜标签归属。无目标 SHALL 禁用定位并提示「本期正文暂无对应事件」；点击时目标消失 SHALL 原地轻提示，SHALL NOT 打开新详情页、近似跳 section 或自动重建。重复激活 SHALL 替换单个高亮与计时器，切 report/卸载清理；reduced-motion SHALL 不平滑滚动，划选 SHALL 不触发定位。

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
