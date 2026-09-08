# Purpose

数据增强（data enrichment）：持久话题的认知分析能力。以分层上下文为输入，经「解读员 → 探索/查询 agent → 分析员 → review judge」角色链产出分析结果，并支持报告追问。本 capability 由两个 change 合并建立：`data-enrichment-orchestration`（骨架：三表分离 / agent loop 三防御 / 可观测性 / 循环A 新闻汇总 / 板块配置）+ `causal-analysis-agent`（分析主线：话题形态判断 / 视角选择 / 分层见解 / 见解依据与确定性 / 报告追问）。

> 变更溯源：data-enrichment-orchestration（2026-07，骨架）+ causal-analysis-agent（2026-07，分析主线接管）。后者推翻前者的「金融走向预测」与「演进定位」两版主线，重定为「探索判断 agent——形态随话题变 + 见解为核心」。
## Requirements
### Requirement: 板块数据源绑定

系统 SHALL 提供 `board_data_sources` 表，存储 SemanticBoard 与数据源的绑定关系。每行 SHALL 包含：`semantic_board_id`（外键）、`source_type`（系统已注册的数据源类型枚举）、`config`（板块级参数 jsonb）、`enabled`（默认 true）、时间戳。`(semantic_board_id, source_type)` SHALL 受唯一约束（同板块同源只绑一次）。`source_type` SHALL 受 CHECK 约束为系统已注册的数据源类型。`source_type` 枚举 SHALL 可扩展（未来接入结构化外部源时新增），但内置 SHALL NOT 包含任何金融行情类源（`etf_quote` / `exchange_rate` / `gdelt_event` 等已被移除）。建表 SHALL 通过显式迁移完成（开发执行规范 §10）。

#### Scenario: 同板块同源唯一

- **WHEN** 尝试对同一 board 插入两条相同 source_type
- **THEN** 系统 SHALL 因唯一约束拒绝第二条

#### Scenario: 金融 source_type 已移除

- **WHEN** 尝试插入 `source_type='etf_quote'`
- **THEN** 系统 SHALL 因 CHECK 约束拒绝（枚举已不含金融源）

### Requirement: 板块分析配置

系统 SHALL 在板块级提供分析配置，含：`enrichment_enabled`（循环B增强开关，默认 false）、`window_days`（循环B实时详情窗口，默认 14）、`context_layers`（解读员读取的上下文层，默认 `["week","month","year","all"]`）。循环 A（新闻汇总）SHALL NOT 受板块配置控制——它是新闻汇总基础设施，全局定时跑。

#### Scenario: 默认关闭增强

- **WHEN** 新建板块
- **THEN** `enrichment_enabled` 默认 false，循环B增强不跑

#### Scenario: context_layers 裁剪

- **WHEN** 板块配置 `context_layers=["week","month"]`（去掉 year/all）
- **THEN** 解读员 SHALL 只读 week + month，跳过未选的层

### Requirement: 数据源工具注册表

系统 SHALL 维护一个数据源工具注册表（`internal/dataenrichment/service/tool_registry.go`），每个工具 SHALL 暴露 `Name`、`Description`、`InputSchema`、`Execute`。注册表 SHALL 至少注册：内部多级导航工具（`list_boards` / `list_lanes` / `get_lane_detail`）、`web_search`（真实联网检索，见『web 搜索与正文抓取数据源』）、`fetch_page`（网页正文抓取）。注册表 SHALL NOT 注册任何 A 股 / ETF / 金融行情类工具（`list_etf_by_keyword` / `get_etf_quote` / `list_sectors` 已移除）。工具 Execute SHALL 返回完整命中结果（不截断条目数量），仅精简字段以控制 token。未注册的工具名调用 SHALL 返回错误并列出可用工具。

#### Scenario: 工具返回完整命中

- **WHEN** `web_search(query="...")` 命中多条结果
- **THEN** 返回 SHALL 包含全部命中，不得截断条目数（避免 agent 误判死循环）

#### Scenario: 金融工具不可见

- **WHEN** agent loop 尝试调用 `get_etf_quote`
- **THEN** Execute SHALL 返回"未知工具"错误，可用工具列表中 SHALL NOT 含任何金融行情工具

#### Scenario: 未知工具拒绝

- **WHEN** agent loop 调用未注册的工具名 `get_stock_price`
- **THEN** Execute SHALL 返回错误，列出可用工具名

### Requirement: 分层新闻汇总上下文（循环 A）

系统 SHALL 维护一个独立于分析的新闻汇总循环（循环 A），产出每个持久话题的分层上下文。新表 `topic_lifeline_context` SHALL 存 week/month/year 四粒度的新闻叙事汇总（含相关数据波动快照），**按 `period` 档案式存储**（每周期独立一行，历史可翻，不再滚动覆盖；UNIQUE 为 `(topic_id, granularity, period)`）。每行 SHALL 带 `as_of_date`（汇总截止日，用于时效判断与检查自愈）。新闻汇总 SHALL 只基于话题的 sections（新闻原文），SHALL NOT 消费 `topic_enrichment_result` 或 `topic_enrichment_review`。

#### Scenario: 汇总独立于分析

- **WHEN** 循环 A 运行生成汇总
- **THEN** 输入 SHALL 为话题的 sections，SHALL NOT 读取任何增强结果或 review

#### Scenario: 时效可见

- **WHEN** 解读员/分析员读取 context
- **THEN** SHALL 能看到 `as_of_date`，以判断该汇总是否滞后于最新进展

### Requirement: 循环 A 触发与汇总算法

循环 A SHALL 定时触发（week 每周、month 每月、year 每年），**每个周期各产一条独立 period 行（新周期不覆盖旧周期）**。SHALL 内置检查自愈机制（扫描缺失历史 period 的 topic，按 period 逐个补），SHALL 支持手动重生成任意 period，SHALL 归档清理超期行（week>8 周、month>12 月）。

汇总算法：**每个 period 独立汇总成一条**——读「该 period 范围内的 sections」一次性汇总，SHALL NOT 与旧汇总合并覆盖。`all`（`period='all'`）为例外，滚动单行，读全部历史 sections + 旧 all 汇总增量合并。各 period SHALL 平行独立维护，不搞 week→month→year 层层金字塔合并（避免误差累积）。

#### Scenario: 每 period 独立汇总

- **WHEN** 每月定时刷新某 topic 的 month（period=2026-06）
- **THEN** SHALL 读 2026-06 范围内 sections 独立汇总成一条，SHALL NOT 覆盖 2026-05 的 month 汇总

#### Scenario: 历史周期可翻

- **WHEN** 用户在前端按周期筛选查看 2026-05 的 month
- **THEN** 系统 SHALL 返回该 period 独立存的汇总行（未被新周期合并覆盖）

#### Scenario: 检查自愈补遗漏 period

- **WHEN** 某 topic 从未生成过 2026-05 的 month 汇总
- **THEN** 检查自愈 SHALL 补生成该 period，`as_of_date` 顺序推进

#### Scenario: 用户手动首次生成缺失 period

- **WHEN** 用户在前端①选一个该 topic 从未生成过的 period（如 2026-W25，表1无对应行）
- **THEN** 系统 SHALL 允许手动触发该 period 的汇总（调 `POST .../contexts/:granularity/regenerate?period=...`），通过 Upsert 首次插入该 period 行，SHALL NOT 要求用户等待检查自愈排队补；生成后该 period 立即出现在可翻阅列表中。此入口与「翻历史」并列，覆盖「已存在重算」和「从未生成首次生成」两种情况

### Requirement: 分层上下文驱动的数据增强编排

数据增强编排的入口 SHALL 是分层上下文（`topic_lifeline_context` + 14天窗口详情 + 历史 applied review），**不是单篇新闻，也不是单一 lifeline**。本条描述单泳道（topic 粒度）编排；版块简报与调查见 `board-level-analysis` capability。单泳道编排 SHALL 继续由三角色组成：

1. **解读员（结构化分析编辑）**：全层读分层上下文（按版块 `context_layers`，未生成的层跳过），提炼需补数据的研究方向，输出 JSON。SHALL NOT 硬编码特定金融方向。
2. **研究助理（agent loop）**：对研究方向使用 `web_search`、`fetch_page` 与内部导航工具搜集可核查材料；相同工具与参数的重复调用仍须拦截。
3. **分析员（结构化分析师）**：结合分层上下文与检索数据产出事实层、见解层和既有兼容的深度内容，显式给出反过度解读边界。

编排 SHALL 对单次 LLM 调用设 max_loops 上限（默认 6）。解读员 SHALL 读取历史 applied review。编排 SHALL NOT 产出已废弃的走向预测字段 `direction` / `confidence` / `horizon` / `trigger_up` / `trigger_down`。

从版块简报或调查下钻时，单泳道入口 MAY 接收可修改的预填研究问题/观察点；该输入只用于聚焦，不得作为不可推翻的既定命题。本 change 的方法卡自动选择仅适用于 `board_investigation`；单泳道不新增多假设 schema，也不自动选择方法卡，但 MUST 停止注入旧作者画像，并按「证据适配与反证纪律」移除固定证据类型配额。

#### Scenario: 消费分层上下文

- **WHEN** 触发某 topic 的数据增强
- **THEN** 解读员输入 SHALL 含配置的 context 层 + 14天窗口详情 + 历史 applied review，不得只含单篇 article

#### Scenario: 解读员领域自适应

- **WHEN** 解读员处理非金融结构话题
- **THEN** 研究方向按话题事实与用户问题生成，SHALL NOT 强制提炼 A 股 ETF 等固定方向

#### Scenario: 分析员产出深度层而非走向预测

- **WHEN** 单泳道分析员产出非 sparse 形态结果
- **THEN** 结果 SHALL 保持既有 `depth` 块兼容，且 SHALL NOT 含 `direction` / `trigger_up` / `trigger_down` 字段

#### Scenario: 死循环防御

- **WHEN** 研究助理尝试用相同参数重复调用同一工具
- **THEN** 系统 SHALL 拦截并返回已调用提示，不执行重复调用

#### Scenario: 下钻问题可修改且可推翻

- **WHEN** 用户从版块简报观察或调查证据发起单泳道分析
- **THEN** 解读员收到对应研究问题/观察点作为预填 lens，用户可修改，后续研究 MAY 得出与预填方向不同的结论

#### Scenario: 单泳道不继承作者画像

- **WHEN** 数据库仍保留旧参考角色文档时触发单泳道分析
- **THEN** 其内容不再全局注入单泳道三角色 prompt，单泳道现有结果 schema 保持不变

### Requirement: agent loop 的三个强制防御

数据增强 agent loop SHALL 内置三项防御（PoC 验证过的坑）：

1. **关闭 Qwen3 thinking**：对本地 Qwen3 模型，LLM 请求 SHALL 传 `chat_template_kwargs.enable_thinking=false`，避免 thinking 烧光 token 导致 content 空。
2. **历史结果不截断**：agent loop 累积的工具调用历史 SHALL 给完整结果，不得截断（截断会导致 agent 误判"没拿全"而死循环重查）。
3. **去重拦截**：相同工具名 + 相同参数的调用 SHALL 被拦截并返回提示。

#### Scenario: thinking 关闭

- **WHEN** agent loop 调用本地 Qwen3
- **THEN** 请求 payload SHALL 包含 `chat_template_kwargs: {enable_thinking: false}`

#### Scenario: 历史完整传递

- **WHEN** agent loop 进入第 N 轮
- **THEN** 喂给 LLM 的历史调用块 SHALL 包含前 N-1 步的完整工具结果（非截断摘要）

### Requirement: 分析认知循环 review judge

每次增强产出 result 后，系统 SHALL 按 `result_kind` 选择同类前序结果并执行认知对比，MUST NOT 在 topic、board brief、board investigation 与 legacy 之间串档：

- `topic_analysis`：对比新见解、推翻项与确定性变化；
- `board_brief`：对比 observations、relationships 与 uncertainties 的新增、消失和置信变化；
- `board_investigation`：仅对比相同 `parent_result_id + question_key` 的重跑，记录 hypotheses assessment 与证据变化；
- `legacy_board_analysis`：只读兼容。仅当历史结果确有旧 direction 字段时，旧 hit/part/miss 兑现度可继续展示，但新简报与调查不得生成预测兑现字段。

review SHALL 关联 prev/curr result；无同类前序结果时跳过。`should_review=false` 时 SHALL 不写噪音 review。用户仍可对任一 result 手动批注，手动 review 为 `source=manual`、prev 可空、applied 默认 true。review 继续不得回写新闻记忆。

#### Scenario: 半自动判断避免噪音

- **WHEN** review judge 判断同类前后结果没有实质认知变化
- **THEN** SHALL 不写自动 review 行

#### Scenario: 兑现度结算

- **WHEN** 用户查看确实含旧 direction/verdict 的 legacy 预测报告
- **THEN** 系统可继续展示其 hit/part/miss 历史结算；新 `board_brief` 与 `board_investigation` SHALL NOT 生成该字段

#### Scenario: 用户手动批注

- **WHEN** 用户在前端对某 result 手动写一条批注
- **THEN** SHALL 创建 review（source=manual，prev_result_id 可空，applied 默认 true），不依赖 review_judge

#### Scenario: result kind 隔离

- **WHEN** 最新 board result 是 investigation，而系统完成一份新 brief
- **THEN** 新 brief 只查上一份 `board_brief` 作为 prev，不把 investigation 或 legacy 当作前序简报

### Requirement: review 不回写新闻记忆

review 的 `applied` 标记 SHALL NOT 触发对 `topic_lifeline_context` 的任何写操作。表1 SHALL 永远只随循环 A 新闻汇总变（保持新闻事实客观）。`applied=true` 仅表示"该认知已被用户采纳"，下次增强解读员 SHALL 读取历史 `applied=true` 的 review 作为输入，以避免重蹈已知偏差。

#### Scenario: 采纳不污染记忆

- **WHEN** 用户对某 review 点"采纳"（`applied=true`）
- **THEN** `topic_lifeline_context` SHALL NOT 被修改

#### Scenario: 读历史 review 避错

- **WHEN** 下次增强解读员读取输入
- **THEN** SHALL 包含历史 `applied=true` 的 review（如"上次因 X 误判黄金跌"）

### Requirement: 仅手动触发（不挂日报管线）

数据增强（循环 B）SHALL **仅由用户手动触发**（CRUD 界面"重新分析某话题"），SHALL NOT 自动挂载到日报管线。理由：不是所有板块对金融数据有影响（如"开发工具"板块），自动增强无意义且浪费成本。

- SHALL 只对 `enrichment_enabled=true` 的板块允许触发。
- 增强结果 SHALL 写入独立的 `topic_enrichment_result` 表（快照不可变，不含 report_id），**不得修改** `daily_report_sections.persistent_topic_id` 或任何 topic 的 status/lifecycle。
- 增强失败 SHALL 只记日志告警，不影响其他（手动触发天然隔离）。

#### Scenario: 仅手动触发

- **WHEN** 用户在 CRUD 界面对某 topic 点"重新分析"
- **THEN** SHALL 立即跑一次增强，不依赖任何日报管线调度

#### Scenario: 只读不污染主数据

- **WHEN** 数据增强完成
- **THEN** `daily_report_sections` 与 `board_persistent_topics` 的所有字段 SHALL 不被增强流程修改

### Requirement: 数据增强全程可观测与可追溯

数据增强的所有 LLM 调用 SHALL 经 airouter 并带 `Operation` 与统一 `SessionID`，写入 `ai_call_logs`。`Operation` SHALL 至少包含：`data_enrichment.interpret` / `data_enrichment.tool_use` / `data_enrichment.analyze` / `data_enrichment.review_judge` / `data_enrichment.summarize_context`。

可观测基础（`ai_call_logs` 表 + `airouter/store.go` 写日志 + `SessionIDFromContext`）SHALL 视为已就绪——不阻塞本 change 开工。

**可追溯（变更大、易出错，强化）**：所有切片的输入输出 SHALL 可查、可重建：

- 每次 LLM 调用的输入（messages）+ 输出（content）→ `ai_call_logs`。
- 每次工具调用的参数 + 返回摘要 + 耗时 → `topic_enrichment_result.tool_calls`（jsonb）。
- 编排元数据（本次增强读了哪些 context 层 + 各层 `as_of_date`、14 天详情的 section 范围、引用的历史 review id 列表）→ `topic_enrichment_result.input_snapshot`（jsonb）。
- 三表（context/result/review）SHALL 持久化全部中间结论。

一次循环 B 增强内的全部 LLM 调用 SHALL 能通过 `session_id` 在 `GET /api/ai/call-logs` 重建（含解读 + N 次工具循环 + 分析 + review_judge）。

#### Scenario: 按 session 回放增强

- **WHEN** 查询 `GET /api/ai/call-logs?session_id=data_enrichment_7_<uuid>`
- **THEN** SHALL 返回该 topic 增强期间的全部 LLM 调用（解读 + N 次工具循环 + 分析 + review_judge），按时间正序

#### Scenario: input_snapshot 记录编排上下文

- **WHEN** 一次增强完成
- **THEN** `topic_enrichment_result.input_snapshot` SHALL 记录读了哪些 context 层、各层 as_of、section 范围、引用的 review id

### Requirement: 板块 tab 「认知工作台」界面

板块详情页的数据增强工作台 SHALL 按用户认知任务组织，而不是把结果表或后端术语平铺出来：

- **① 新闻背景**：周期筛选、历史翻阅与既有 inline 编辑能力保留；
- **② 版块简报**：展示最新/历史 brief 的关键观察、关系、不确定项和可调查问题；
- **③ 深入调查与认知变化**：用户显式选择问题后查看 investigation、支持/反证/gaps、QA 与同类 review；legacy 报告在历史兼容入口只读展示；
- **④ 数据源/参数**：折叠高级区。

工作台 SHALL 使用人话，提供 loading/empty/error 与双主题。简报和调查通过 job id/job kind 分别显示当前异步任务状态；同一 board 虽串行执行，前端 MUST NOT 把调查完成误报为“新简报完成”。数据契约 SHALL 保持可被后续侦探墙视图复用。

#### Scenario: 周期筛选翻历史

- **WHEN** 用户在新闻背景选择月粒度并翻到历史 period
- **THEN** 系统展示该 period 独立存储的新闻汇总，不被当前周期覆盖

#### Scenario: 证据链 tooltip 不跳转

- **WHEN** 用户查看 news/web/page 证据的原文摘录
- **THEN** 可在原地 tooltip/展开区查看 quote；lane 引用则按 board-level-analysis 契约允许下钻

#### Scenario: 兑现度复盘可见

- **WHEN** 用户打开认知变化区
- **THEN** 新结果展示见解/关系/假设状态变化；legacy 预测报告若有 hit/part/miss 则在旧版兼容视图继续可见

#### Scenario: 契约为侦探墙铺路

- **WHEN** 前端消费 brief/investigation/review 数据
- **THEN** 观察、关系、假设和证据均为可复用结构字段，不依赖连续论文文本解析

#### Scenario: 简报到调查由用户确认

- **WHEN** 简报生成候选研究问题
- **THEN** 工作台只展示“深入调查”入口，不在简报完成后自动触发调查

### Requirement: 话题形态判断

系统 SHALL 在分析前判断持久话题的形态，归入 `event_chain`（事件链）/ `theme_vein`（主题脉络）/ `single_point`（单点影响）/ `structural`（结构演化）/ `sparse`（骨感）之一。判据 SHALL 综合 `hit_count`（丰满度）、`section` 数（聚合度）、`cluster_label` 发散度（线性 vs 平行）与内容语义。`structural` 形态适用于**无离散事件、呈长时段结构演化**的话题（如"人民币国际化进程""美元霸权演变"）。形态枚举 SHALL 可扩展，新增形态无需改架构。

#### Scenario: 高频线性因果归事件链型

- **WHEN** 话题 hit_count ≥ 10 且 section 时序呈线性因果演进（如"官宣→否认→条款"）
- **THEN** 系统 SHALL 判 `event_chain`，产出因果链 + 推演见解

#### Scenario: 长时段结构演化归结构型

- **WHEN** 话题为持续性结构命题、无单一离散事件驱动（如"人民币国际化"）
- **THEN** 系统 SHALL 判 `structural`，产出结构演化叙述 + 深度层

#### Scenario: 单次命中归骨感型

- **WHEN** 话题 hit_count = 1（料严重不足）
- **THEN** 系统 SHALL 判 `sparse`，诚实标注信息不足，不产出推演见解与深度层

### Requirement: 分析视角候选与选择

系统 SHALL 采用「agent 提候选 + 用户选」模式（模式丙）：解读员 SHALL 输出多个**具体可讨论的视角候选**（问题式，如"美国为何反复横跳"，非"博弈论"等抽象标签），用户选定一个视角后，探索 agent 按该视角推演。第一版 SHALL 为单视角。视角来源 SHALL 抽象为可扩展接口（首批 agent 生成，预留外部源如视频评论员/研报）。

#### Scenario: agent 提具体视角候选

- **WHEN** 解读员完成形态判断
- **THEN** 系统 SHALL 输出 ≥ 2 个具体问题式视角候选，每个带视角名 + 一句话说明

#### Scenario: 用户选定视角驱动推演

- **WHEN** 用户从候选选一个视角（或自填）
- **THEN** 探索 agent SHALL 仅按该视角推演见解，不发散到未选视角

#### Scenario: 视角来源可扩展

- **WHEN** 未来接入外部视角源（如视频评论员）
- **THEN** 系统 SHALL 通过 LensSource 接口注入候选，不动核心编排

### Requirement: 探索 agent 工具集

系统 SHALL 向探索 agent（研究助理）注册多级入口探索工具 + `web_search` 联网检索 + `fetch_page` 正文抓取，**不注册任何金融行情工具**（金融工具已彻底移除，不再"降为可选"）。多级入口工具 SHALL 支持分层下钻：`list_boards`（看版块全景）→ `list_lanes`（版块下泳道）→ `get_lane_detail`（泳道详情按需取）。**`get_lane_detail` 的输出 SHALL 包含该泳道的历史背景记忆摘要（month/year 档 lifeline 归档行，受字符预算约束截断、标注粒度与 period），与近期 section 时间线并列呈现**——下钻不得只能取到 section 标题链。agent SHALL 自主决定下钻深度、检索查询与何时停止。

#### Scenario: 多级入口按需下钻

- **WHEN** 探索 agent 判断某版块可能与视角相关
- **THEN** agent SHALL 调 `list_lanes` 查该版块泳道，仅对相关泳道调 `get_lane_detail`，无关版块跳过

#### Scenario: 下钻可读历史背景记忆

- **WHEN** agent 调 `get_lane_detail` 查询存在 month/year 档 lifeline 的泳道
- **THEN** 输出含该泳道背景记忆摘要（预算内截断、标注粒度），agent 可将历史记忆用作论据，不得只拿到近期 section 标题时间线

#### Scenario: 无背景记忆时不报错

- **WHEN** 泳道无任何 lifeline 归档行
- **THEN** `get_lane_detail` 正常返回近期 section 详情，背景记忆段如实标注缺失（不静默省略段落标记）

#### Scenario: web_search 与 fetch_page 配合取证

- **WHEN** 探索 agent 需验证事实节点或抓取一手原文支撑深度层
- **THEN** agent SHALL 调 `web_search` 检索，对关键命中调 `fetch_page` 取正文，结果纳入深度层 `evidence_chain`

#### Scenario: 金融工具彻底不可见

- **WHEN** 话题为任意形态（含金融相关）
- **THEN** 系统 SHALL NOT 注册金融行情工具，agent 全程不可见
### Requirement: 分层见解产出

系统 SHALL 产出分层分析：**事实层**（梳理 + 验证，铺垫）+ **见解层**（推演/假设/提问/视角，★产出主体）+ **深度层**（非 sparse 形态强制，见『分析深度层产出』）。见解层 SHALL 发挥 AI 多层推演 + 跨领域联想 + 假设性提问优势。产出结构 SHALL 随形态变（事件链=因果链+推演见解 / 脉络=平行线索+跨线索洞察 / 单点=影响评估 / 结构=演化叙述 / 骨感=诚实标注）。

#### Scenario: 事实层与见解层分离

- **WHEN** 系统产出分析
- **THEN** 输出 SHALL 明确区分事实层（已验证 claim）与见解层（推演 insight），读者可辨哪是真哪是猜

#### Scenario: 非 sparse 形态强制深度层

- **WHEN** 系统产出 `event_chain`/`theme_vein`/`single_point`/`structural` 形态分析
- **THEN** 输出 SHALL 含完整深度层 `depth` 块；`sparse` 形态 SHALL NOT 产出深度层

### Requirement: 见解依据与确定性分级

每条见解 SHALL 挂**文章依据**（新闻原文引用）+ **时间线依据**（事件时序节点），不悬空发散。每条见解 SHALL 标注确定性分级：`high`（已验证）/ `medium`（推演·有据）/ `low`（假设·情景）/ `question`（提问·指出条件非预言成败）。见解 SHALL 给推演逻辑（凭什么 A→B），关键中间环节 SHALL 可 web 验证。

#### Scenario: 见解必须挂依据

- **WHEN** 探索 agent 产出一条见解
- **THEN** 该见解 SHALL 引用 ≥ 1 篇文章依据或时间线节点，无依据的见解 SHALL 被拒绝

#### Scenario: 确定性分级标注

- **WHEN** 见解为推演而非已验证事实
- **THEN** 系统 SHALL 标注 `medium`/`low`/`question` 之一，前端按级视觉区分

#### Scenario: 提问式见解不预言成败

- **WHEN** 见解为 `question` 级
- **THEN** 系统 SHALL 指出决定成败的条件，而非预言成败结果

### Requirement: 骨感型诚实标注

对料严重不足的话题（如 hit_count = 1，占库 65%），系统 SHALL 诚实标注"信息不足"，不强行推演见解。系统 SHALL 将此类话题列入观察，持续命中后自动升级分析。知道何时不该分析 SHALL 视为核心判断力。

#### Scenario: 骨感型不硬推演

- **WHEN** 话题被判 `sparse` 形态
- **THEN** 系统 SHALL 产出"信息不足"标注 + 轻量摘要，不产出推演见解

#### Scenario: 持续命中后升级

- **WHEN** 原 sparse 话题持续命中、积累演进特征
- **THEN** 系统 SHALL 重新判形态并升级为对应分析

### Requirement: 分析认知对比

系统 SHALL 在每次分析后对照上次，记录 `new_findings`（新见解）/ `overturned`（推翻的旧见解）/ `confidence_shift`（确定性变化），取代旧的"定位变化对比"。review SHALL 不回写新闻记忆（表1），仅标记认知采纳。

#### Scenario: 记录新发现与推翻

- **WHEN** 本次分析完成
- **THEN** review SHALL 对比上次 insight_layer，输出新增/推翻/确定性变化

#### Scenario: 不回写新闻记忆

- **WHEN** review 标记认知采纳
- **THEN** 系统 SHALL 仅在表3 记录，不污染表1（新闻记忆保持客观）

### Requirement: 多形态分析报告

前端 SHALL 按话题形态渲染不同结构的分析报告（事件链=因果链+见解层 / 脉络=平行线索+跨线索洞察 / 单点=影响评估 / 结构=演化叙述 / 骨感=信息不足标注）。报告 SHALL 展示形态判断依据、视角选择、时间线依据轴、引用（📰新闻/🌐网页/📄正文）、见解确定性分级视觉。报告 SHALL 渲染**深度层**区块（系统重定位 / 多层机制 / 历史类比 / 范式转折 / 边界限定 / 可核查证据链），证据链 SHALL 支持点击查看原文 URL。

#### Scenario: 按形态渲染不同结构

- **WHEN** 话题形态为 `theme_vein`
- **THEN** 报告 SHALL 渲染平行线索 + 跨线索洞察，不画因果链箭头

#### Scenario: 深度层区块可见

- **WHEN** 报告展示非 sparse 形态分析
- **THEN** 报告 SHALL 渲染深度层各字段（系统重定位/多层机制/历史类比/边界），证据链条目 SHALL 带可点击 URL

#### Scenario: 见解确定性视觉区分

- **WHEN** 报告展示见解层
- **THEN** 不同确定性（high/medium/low/question）SHALL 用不同视觉（颜色/标签）区分

### Requirement: 报告追问交互

系统 SHALL 支持用户在分析报告页针对报告内容多轮追问。追问 agent SHALL 复用探索 agent 循环（多级入口 + web_search），上下文 SHALL 带报告本身（analysis + 依据 + 视角 + 形态）。回答 SHALL 带双类引用 + 确定性标注。追问会话 SHALL 持久化（多轮历史）。追问新依据 SHALL 支持用户手动沉淀回报告（标记来源），不自动改报告。

#### Scenario: 追问复用探索工具

- **WHEN** 用户问"这条见解还有别的证据吗"
- **THEN** 追问 agent SHALL 调 web_search/get_lane_detail 补充，回答带新依据引用

#### Scenario: 多轮追问持久化

- **WHEN** 用户在同一报告下连续追问
- **THEN** 系统 SHALL 保存多轮问答历史（topic_enrichment_qa），支持回看

#### Scenario: 追问发现手动沉淀

- **WHEN** 追问中产生有价值的新依据
- **THEN** 系统 SHALL 支持用户手动沉淀回报告（标记 source=qa），不自动改报告

### Requirement: web 搜索与正文抓取数据源

系统 SHALL 提供真实的 web 检索与正文抓取能力作为数据增强的通用外部数据源。`web_search` 工具 SHALL 接入真实搜索后端（首个实现为博查 Bocha，`open.bocha.cn`），**SHALL 使用原始网页结果模式（通搜 / raw web results），返回带 `url` 的原始网页清单 `[{title,url,snippet,site_name}]`；SHALL NOT 使用 AI 总结模式**（AI summary 有失真与幻觉风险，不可作可核查证据）。`fetch_page` 工具 SHALL 复用 `reader` 域 `readability_crawler` 抓取网页正文，返回 `{title,url,main_text}`。深度层 `evidence_chain` 引用的外部原始正文 SHALL 经 `fetch_page` 抓取自真实文档，SHALL NOT 来自 AI 转述。`web_search` SHALL 通过 `WebSearcher` 接口注入实现，便于未来替换/增加服务商而不动编排。搜索后端 API key SHALL 以**设置界面**（`ai_settings` 表 `bocha_config`，照 Firecrawl）为主注入，`configs/config.yaml` + 环境变量 `BOCHA_API_KEY` 作兜底；SHALL 动态读取（界面修改即时生效，无需重启），优先级 DB > env > config.yaml；SHALL NOT 硬编码。当 key 未配置或后端不可达时，`web_search` / `fetch_page` SHALL 返回错误 JSON 让 agent 自降级（沿用 registry 单工具失败约定），SHALL NOT 阻断 agent loop。

#### Scenario: web_search 返回真实结果

- **WHEN** agent 调 `web_search(query="1973 石油危机 资本回流")` 且博查 key 已配置
- **THEN** SHALL 返回真实搜索命中 `[{title,url,snippet}]`，SHALL NOT 返回"not configured"

#### Scenario: 原始结果模式非 AI 总结

- **WHEN** agent 调 `web_search` 为深度层取证
- **THEN** SHALL 返回原始网页结果（通搜模式，带 url），SHALL NOT 返回 AI 总结/转述；深度层引用的外部正文 SHALL 经 `fetch_page` 取自真实文档，SHALL NOT 来自 AI summary

#### Scenario: fetch_page 抓正文

- **WHEN** agent 调 `fetch_page(url="https://...")`
- **THEN** SHALL 返回该页 readability 提取的 `main_text`（超长截断），供深度层 `evidence_chain` 引用原文

#### Scenario: 无 key 优雅降级

- **WHEN** `BOCHA_API_KEY` 未配置
- **THEN** `web_search` SHALL 返回"web_search 未配置"错误 JSON，agent loop SHALL 继续运行（不阻断），深度层仍产出但证据链弱

#### Scenario: 搜索后端可替换

- **WHEN** 未来将博查换为 Tavily
- **THEN** 改动 SHALL 局限于新增一个 `WebSearcher` 实现 + wire 注入，SHALL NOT 触碰编排与 analyze prompt

### Requirement: 分析深度层产出

非 sparse 形态（`event_chain` / `theme_vein` / `single_point` / `structural`）的分析结果 SHALL 强制产出深度层 `depth` 块，映射结构化深度分析基因：`system_reframe`（一句话把事件放进哪个大系统讲）/ `mechanism_layers`（多层子机制拆解，每层给 `deep_logic` 深层逻辑 + 依据）/ `historical_analogy`（历史案例 + 机制类比 + 何处不同）/ `regime_shift`（范式转折判断，无则 null）/ `boundary`（显式反过度解读：什么还不能下结论）/ `evidence_chain`（可核查证据链，`source_type ∈ news|web|page`，web/page 带 `url`+`quote`+`institution`+`date`）。`boundary` SHALL 非空（强制反过度解读）。`sparse` 形态 SHALL NOT 产出深度层。

#### Scenario: 非 sparse 强制完整深度层

- **WHEN** 分析员产出 `structural` 形态结果
- **THEN** `depth` 块 SHALL 含 `system_reframe` / `mechanism_layers` / `historical_analogy` / `boundary` 非空，`evidence_chain` 至少 1 条

#### Scenario: 反过度解读强制

- **WHEN** 分析员产出任意非 sparse 结果
- **THEN** `depth.boundary` SHALL 非空，显式标注不能下结论的边界，SHALL NOT 空泛

#### Scenario: 证据链可核查

- **WHEN** 深度层引用外部依据
- **THEN** `evidence_chain` 中 web/page 类条目 SHALL 带 `url` + `quote` 原文摘录 + 来源机构 + 日期，可供前端点击核查

#### Scenario: 范式转折可缺省

- **WHEN** 话题无范式转折迹象
- **THEN** `depth.regime_shift` SHALL 为 null（不强行编造）

#### Scenario: sparse 不产深度层

- **WHEN** 形态为 `sparse`
- **THEN** 结果 SHALL 只含 `notice` + `summary`，SHALL NOT 含 `depth` 块

### Requirement: 泳道证据引用槽位

见解、简报与调查证据的 `source_type` 枚举 SHALL 为 `news | web | page | lane`。`lane` 类证据 MUST 携带泳道标识与引用说明，供前端点击下钻；既有三值证据行为不变。证据 MAY 携带可选 `kind`（`quote | series | chart`），其与 `source_type` 正交；`kind` 缺省时按旧数据处理。

#### Scenario: lane 类证据下钻

- **WHEN** 分析产物中存在 `source_type=lane` 的证据
- **THEN** 该证据携带 lane id，前端可解析并打开对应泳道

#### Scenario: 旧枚举不受影响

- **WHEN** 读取仅含 `news|web|page` 的历史结果
- **THEN** 解析与渲染行为与扩展前一致

#### Scenario: kind 可选兼容

- **WHEN** 证据无 `kind` 或 `kind` 为空
- **THEN** 解析与渲染不报错；合法 kind 透传，非法 kind 降级为空并留痕

### Requirement: 分析前素材新鲜度门

分析编排入口（版块级与单泳道）在装配背景素材前 SHALL 对各活跃泳道 **month / year 档**生命线执行**补全门**（并非仅保鲜）：

1. **缺失补建**：有 section 数据的周期无行时 MUST 先补建（含无任何记录时的首份——首建归分析路径，不再留给定时任务）；
2. **截断重算**：已有行但最后写于 72h 前 MUST 重算覆盖（周期已结束的得到完整版，进行中的得到至今快照）；
3. **限流**：单次分析补全调用设全局限额，溢出降级用旧档继续分析并留结构化日志，不得阻塞；
4. **钳制**：任何写入路径的 `as_of_date` MUST 钳制到不超过当前时刻（周期边界未来日期属脏数据）；
5. week 档退出分析路径检查集（近期记忆由 14 天窗口详情承担，长期记忆由 month/year 承担；存量 week 行保留可被消费）。

补全 MUST 串行执行；失败降级不阻塞分析。

#### Scenario: 截断档分析前重算

- **WHEN** 泳道某已结束周期存在行但该行写于周期结束前（半月档），触发分析
- **THEN** 装配前该周期被重算为完整版，分析素材基于补全后的档案

#### Scenario: 无记录首建

- **WHEN** 泳道无任何 lifeline 行（新孵化泳道），触发分析
- **THEN** 装配前为其 month/year 当前期建首份，而非跳过留给定时任务

#### Scenario: 限额溢出降级

- **WHEN** 补全需求超过单次分析限额
- **THEN** 超出部分留结构化日志并继续分析（用旧档），不阻塞不报错

#### Scenario: 补齐幂等

- **WHEN** 同一板块同一天内连续两次触发分析
- **THEN** 第二次触发不重复补（已补档最后写入时间新鲜）

#### Scenario: 补齐失败降级

- **WHEN** 补齐调用失败
- **THEN** 分析继续（用旧档），失败写入结构化日志可查

#### Scenario: 无数据周期跳过

- **WHEN** 某粒度没有任何 section 数据可形成周期
- **THEN** 系统跳过该粒度，不为无数据创建空档案
### Requirement: 证据适配与反证纪律

系统 SHALL 以证据与研究问题的直接相关性、可核查性和来源质量为优先级，MUST NOT 为满足固定数量或类型配额而强行加入历史类比、报告或数据序列。`board_investigation` SHALL 分别记录支持证据、反证与缺口；单泳道结果无需改成多假设 schema，但其 prompt 同样不得以“至少三类证据”驱动牵强取材，并应保留发现的冲突材料。外部原文仍应优先经 fetch_page 核查。

#### Scenario: 直接证据少于三类

- **WHEN** 某问题只有一类或两类高度相关的一手证据，其他类型与问题无关
- **THEN** 系统使用现有直接证据并标注局限，不为凑足三类加入牵强历史案例或无关报告

#### Scenario: 支持与反证并列保存

- **WHEN** board investigation 的同一假设同时存在支持材料和冲突材料
- **THEN** 两类材料分别记录并共同影响 assessment，反证不得被丢弃或只写进模糊免责声明

#### Scenario: 单泳道不扩 schema

- **WHEN** 单泳道研究发现与当前 lens 冲突的材料
- **THEN** 结果在既有 evidence/boundary 结构中保留冲突与局限，不要求迁移为 board hypotheses schema

#### Scenario: 条件不足诚实降级

- **WHEN** 外部检索仅获得新闻转述且缺乏一手来源
- **THEN** 结果降低置信并记录核查缺口，不把转述包装成原始证据

### Requirement: 调查内部上下文的动态授权
版块调查 SHALL 提供只返回紧凑版块/泳道概要的内部上下文搜索能力。调查开始时 `get_lane_detail` 仅授权父简报派生的泳道；本次调查中由受信任的搜索或导航工具实际返回的泳道 ID SHALL 被加入本次临时授权集，随后方可读取详情。授权 MUST 绑定单次调查会话，MUST NOT 因模型猜测 ID、其他调查结果或历史会话而扩大。

#### Scenario: 搜索返回的泳道获得授权
- **WHEN** 调查调用内部上下文搜索并返回另一个版块的泳道 ID
- **THEN** 该 ID 加入本次会话临时授权集，后续 `get_lane_detail` 可以读取其详情并保留授权来源

#### Scenario: 猜测的泳道仍被拦截
- **WHEN** agent 调用一个既不属于父简报、也未由本次受信工具返回的 lane ID
- **THEN** 工具在执行前拒绝调用并以结构化 blocked 结果留痕

#### Scenario: 动态授权不跨会话泄漏
- **WHEN** 调查 A 授权了一个跨版块泳道而调查 B 尚未发现该泳道
- **THEN** 调查 B 仍无权读取该泳道详情

#### Scenario: 搜索只暴露紧凑概要
- **WHEN** agent 尚未获得某候选泳道的详情授权
- **THEN** 内部搜索只返回用于选择的 ID、所属版块、标题和紧凑摘要，MUST NOT 提前返回完整时间线或历史记忆

### Requirement: 跨版块研究留痕
版块调查的不可变输入快照 SHALL 记录初始泳道白名单、运行时新增授权、每次授权的工具调用来源以及实际引用泳道的所属版块。跨版块研究继续 SHALL 保存完整工具结果、研究 gap 和证据覆盖情况，且不得回写父简报或新闻记忆。

#### Scenario: 跨版块泳道进入调查快照
- **WHEN** 调查读取并引用了经动态授权的其他版块泳道
- **THEN** 持久化调查快照包含该泳道 ID、所属版块、授权来源和对应工具调用

#### Scenario: 调查失败不改写父数据
- **WHEN** 动态探索后验证或综合阶段失败
- **THEN** 系统不创建半成品调查结果，也不修改父简报、泳道 lifeline 或关系建议状态
