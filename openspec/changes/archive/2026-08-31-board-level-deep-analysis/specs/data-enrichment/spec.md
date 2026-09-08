## MODIFIED Requirements

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

## ADDED Requirements

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

版块简报、版块调查与单泳道分析在装配背景素材前 SHALL 对相关活跃泳道的 **month / year 有数据周期**执行补全门：

1. 有 section 数据的周期缺行时 MUST 补建，包含无任何 lifeline 行时的首份；
2. 已有行最后写入时间超过 72h 时 MUST 重算；
3. week 退出检查集，近期事实由 14 天窗口详情承担；
4. 补全串行且单次有全局调用上限（当前 40 次），溢出或失败用旧档继续并留结构化日志；
5. 所有写入的 `as_of_date` MUST 不晚于当前时刻。

#### Scenario: 截断档分析前重算

- **WHEN** 某 month/year 有数据周期已有行但最后写于 72h 前
- **THEN** 分析装配前重算该周期，并基于更新后的档案生成素材

#### Scenario: 无记录首建

- **WHEN** 新泳道有 month/year 周期内的 section 数据但没有对应 lifeline 行
- **THEN** 分析路径先创建该周期首份档案，不跳过等待定时任务

#### Scenario: 无数据周期跳过

- **WHEN** 某粒度没有任何 section 数据可形成周期
- **THEN** 系统跳过该粒度，不为无数据创建空档案

#### Scenario: 限额溢出降级

- **WHEN** 单次补全需求超过调用上限
- **THEN** 超出部分记录 budget_exhausted 并用旧档继续，不阻塞当前操作

#### Scenario: 补齐幂等

- **WHEN** 同一版块同一天内连续触发简报与调查且首次已补全
- **THEN** 后续触发不重复补全最后写入时间仍新鲜的周期

#### Scenario: 补齐失败降级

- **WHEN** 补全调用失败
- **THEN** 当前操作继续使用旧档，失败日志可按 topic/granularity/period 查询

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
