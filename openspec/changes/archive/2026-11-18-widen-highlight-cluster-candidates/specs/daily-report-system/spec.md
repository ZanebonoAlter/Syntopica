## REMOVED Requirements

### Requirement: 事件线热度先筛重点候选
**Reason**: 事件线热度链路（`heat-v1-48h-hl24h` 的 H/P 公式、事件线存活 id 候选）已随 retire-story-adopt-event-metrics 退役，现行实现为当天 event 标签聚簇候选（Top 8、簇内去重文章数排序），spec 文本未同步形成文档债。本 change 将现行口径连同 composite 放宽收敛为新 requirement「今日重点聚簇候选」。
**Migration**: 行为契约由 ADDED 的「今日重点聚簇候选」承接；已落库的 highlights 快照（含旧故事模式字段）不回填、读取兼容不变；重建历史日报按新口径重算重点（预期内行为）。

## MODIFIED Requirements

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

## ADDED Requirements

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
