# board-lane-dynamics Specification

## Purpose
版块「板块内容」tab 的首屏泳道动态视图：以叙事级信息（滚动 14 天态势 + 逐日发展时间线）取代计数统计型态势版图，让用户一眼看懂每条关注/追踪泳道最近发生了什么；态势由每日日报生成后滚动结算维护。

## Requirements

### Requirement: 泳道动态视图入口

版块态势页（`/tags/:boardId`）SHALL 以「泳道 × 日期」矩阵作为版块选中后的默认首屏概览，取代原「板块内容」tab 内的泳道卡片区。矩阵的**日格** SHALL 保持只读展示（格内不提供任何写操作；治理右轨/抽屉的操作 SHALL NOT 触发矩阵以外的副作用）。矩阵的**行头** SHALL 提供泳道操作入口（见下），写操作 SHALL 统一由泳道编辑面板承载，SHALL NOT 在矩阵内就地改变话题生命周期状态。演进带作为矩阵的选中泳道明细区，与矩阵共用同一聚合响应。

矩阵行头 SHALL 提供「⋯」操作入口，至少包含「编辑泳道」项（打开泳道修正工作面，见 lane-correction-surface 能力）。行头操作 SHALL NOT 触发行选中、选中日变更或泳道选中副作用。

#### Scenario: 默认展示泳道动态区

- **WHEN** 用户选中某版块并进入版块态势页
- **THEN** 页面 SHALL 渲染泳道 × 日期矩阵（泳道动态视图），并在其下方渲染当前选中泳道的演进带

#### Scenario: 只读语义

- **WHEN** 用户点击矩阵的某个日格
- **THEN** 系统 SHALL 只改变当前选中日与选中格，SHALL NOT 在格内提供归档/转正/合并等生命周期写操作；日格 SHALL 保持只读，写操作统一由泳道编辑面板承载

#### Scenario: 行头提供编辑入口

- **WHEN** 用户查看矩阵某行的行头
- **THEN** 行头 SHALL 提供「⋯」操作入口，其中包含「编辑泳道」项，点击后 SHALL 打开泳道编辑面板

#### Scenario: 行头操作不误触发行选中

- **WHEN** 用户点击行头的「⋯」按钮
- **THEN** 系统 SHALL NOT 选中该行、SHALL NOT 改变当前选中日，SHALL NOT 触发泳道选中副作用

#### Scenario: 矩阵内不就地改生命周期

- **WHEN** 用户在矩阵区域操作
- **THEN** 系统 SHALL NOT 在矩阵内直接执行归档/转正/合并等话题生命周期写操作，此类写操作 SHALL 统一在泳道编辑面板内完成

### Requirement: 泳道卡片构成
原「每条泳道渲染为一张卡片」的形态 SHALL 退役，改由矩阵行与演进带共同承载：矩阵行 SHALL 展示泳道名、追踪标识与断档；矩阵格内数字 SHALL 为该泳道该日各主题桶（section）**非综合稿线索数之和**（当日线索合计），SHALL NOT 取桶峰值，SHALL NOT 用含综合稿的 thread 总数冒充，行尾 MUST NOT 再渲染重复总数列（2026-10-27 矩阵阶段修订；2026-11-16 线索数单一口径收敛）。选中泳道的演进带 SHALL 展示按日期排列的节点（节点数字与点径 = 当日各主题桶线索数之和，线索密集日显示标签）、滚动态势句入口（最新结算快照逐字，标注 as_of 与「非本视窗总结」）、以及当日线索工作面。事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排。矩阵日格的完整口径与伴生数字规则 SHALL 以 `workshop-board-console` 的「泳道热度口径」与「矩阵格伴生数字与常驻口径说明」为准，本 requirement SHALL NOT 与其冲突。

#### Scenario: 发展时间线结构
- **WHEN** 某泳道观察范围内多个日期有锚定 section
- **THEN** 演进带 SHALL 按日期顺序排列节点，密集日（≥3 条线索）显示事件标签
- **AND** 事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排

#### Scenario: 一句话态势展示
- **WHEN** 某泳道存在有效的滚动态势快照
- **THEN** 演进带区域 SHALL 提供该态势句并标注汇总截止日（as_of）与「非本视窗总结」，长句 SHALL 以折叠方式呈现而非直接铺满版面

#### Scenario: 单日多事件
- **WHEN** 某泳道同一天有 2 个锚定 section、各含多条 thread
- **THEN** 当日工作面 SHALL 列出该日全部事件的 thread 标题；聚合响应每 section 截前 N 条标题时，完整标题 SHALL 经日报详情按稳定 `section_id` 读取（截断提示仅作读取不可用时的降级）

### Requirement: 展示范围与排序
泳道动态区 SHALL 展示「关注（active）」与「我在追踪（watch 关联）」的泳道，按**近 14 天有锚定 section 的天数**（同日多条只计一次）降序排列，二次排序按最近命中日期倒序、再按 topic id 升序；watch 关联泳道 SHALL 带追踪标识。近 14 天无锚定 section 的 active 泳道（沉寂）SHALL NOT 展示。candidate 泳道 SHALL NOT 出现在主卡片区。

排序口径 SHALL NOT 使用 section 条数：泳道内主题分桶会把同一话题同一天的内容拆成多条 section，按条数排序会让"当天拆得多的泳道"虚高。

#### Scenario: 活跃排序

- **GIVEN** 泳道 A 近 14 天有 6 天出现锚定 section（其中某天 3 条），泳道 B 有 3 天（每天 1 条）
- **WHEN** 渲染泳道动态区
- **THEN** A 卡 SHALL 排在 B 卡前（6 天 > 3 天），SHALL NOT 因为 A 的 section 条数被拆分放大而进一步加权

#### Scenario: 同日多条只计一天

- **GIVEN** 泳道 A 仅有 1 天出现锚定 section，但该天被拆成 4 条 section；泳道 B 有 2 天、每天 1 条
- **WHEN** 渲染泳道动态区
- **THEN** B 卡 SHALL 排在 A 卡前（2 天 > 1 天）

#### Scenario: watch 标识

- **WHEN** 某泳道被用户 watch（追踪）关联
- **THEN** 该泳道卡 SHALL 带追踪标识（与普通 active 卡视觉区分）

#### Scenario: 沉寂不展示

- **WHEN** 某 active 泳道近 14 天无任何锚定 section
- **THEN** 该泳道 SHALL NOT 渲染卡片

#### Scenario: 候选不进主区

- **WHEN** 某 candidate 泳道近 14 天有锚定 section
- **THEN** 该泳道 SHALL NOT 出现在主卡片区（仅可能出现在候选栏）

### Requirement: 候选栏
泳道动态区底部 SHALL 提供候选栏，列出最近达可见门槛的 candidate 泳道（名字 + 最近动向摘要），作为「值得注意」的只读提示。候选栏 SHALL NOT 提供转正等生命周期操作（转正走既有话题管理入口）。

#### Scenario: 候选列出
- **WHEN** 某 candidate 泳道 hit_count 达可见门槛
- **THEN** 候选栏 SHALL 列出其名称与最近动向

#### Scenario: 无候选
- **WHEN** 无达门槛的 candidate
- **THEN** 候选栏 SHALL NOT 渲染（或明确提示无候选，二选一于 ui-design 定稿）

### Requirement: 卡片跳转联动
点击泳道卡片（含候选栏条目）SHALL 跳转「话题总览」tab 并聚焦该话题（focus 视图），沿用既有联动交互模式。

#### Scenario: 点卡片聚焦
- **WHEN** 用户点击泳道动态区某张泳道卡
- **THEN** 界面 SHALL 切换到「话题总览」tab 并进入该话题的 focus 视图

### Requirement: 空态处理
板块无日报时，泳道动态区 SHALL 展示空态引导用户生成日报（沿用既有生成入口与进度反馈模式）；生成完成后 SHALL 自动刷新泳道动态。

#### Scenario: 空态引导
- **WHEN** 板块无任何日报
- **THEN** 泳道动态区 SHALL 展示「生成日报」引导而非空白

#### Scenario: 生成后刷新
- **WHEN** 空态触发的日报生成完成（进度反馈结束）
- **THEN** 泳道动态区 SHALL 自动加载新数据

### Requirement: 矩阵观察窗读取边界
矩阵/追踪的自由观察日期 SHALL 仅消费既有 `GET /semantic-boards/:id/lane-dynamics?days=N` 端点的7/14/30/365四档响应：观察窗 SHALL NOT 新增 `start/end` 聚合参数、新端点或新的统计模式。sourceDays 仅由独立档按钮或明确 URL days 变更；A（source档）SHALL 是名单、source全轴前端 clueTotal→sectionTotal→ID 排序、休眠与候选的唯一基准。B（365池）SHALL 仅补 A 同 ID 历史内容；观察日期无论是否恰7/14/30/365日均 MUST NOT 换基准，B独有ID不得加入名单/候选。A/B失败 SHALL 分层：A成功B失败保留A可读，历史未取到不补0、不换名单或强改视窗；A首次失败B成功显示A错误态，A刷新失败保留旧A并提示stale，MUST NOT 用B冒档成功。sourceDays=365 SHALL 只发一次请求、档/池共用。当前板块source成功缓存 SHALL 限四档（365共用），报告列表索引仅1份、报告详情最多14份、文章详情最多20份；SHALL 用简单Map FIFO/既有工具，失败不入成功缓存、不逐出成功项，切板块失效、卸载释放。当前展开阅读对象 SHALL 不因逐出消失或回顶。完整线索标题、摘要与相关文章 SHALL 复用既有日报列表/详情与文章详情端点按稳定 ID（report `period_date` → `section_id` → thread `id` / `related_article_ids`）读取，MUST NOT 以线索标题文本匹配作为长期关联。报日索引 SHALL 复用既有板块日报列表端点（前端每板块至多 1 份成功缓存），MUST NOT 为日期轴新增聚合参数或端点；0 正式泳道版块的日期轴与「当日其他话题」SHALL 复用同一索引与既有单日报告详情端点（按稳定 `section_id` 读取全量 threads），MUST NOT 因窗内无泳道而禁止读取真实已存在的日报。报告优先读取 SHALL 与既有编年共用同一有界缓存与板块作用域（索引 1 份、报告 ≤14、文章 ≤20）并各持独立 epoch，同板块切换/卸载统一释放；MUST NOT 引入第二套读取器或重复 GET。报日与报告优先读取同样 SHALL 零写入。产品实现 MUST NOT 复制原型离线样本、原型常数或把样本切片冒充后端取数；本能力全部交互 SHALL 零写入（不改话题状态、不触发生成/回填）。

#### Scenario: 自由日期不触发新聚合
- **WHEN** 用户把source14的观察范围拖到任意合法区间（含恰7/30/365日）
- **THEN** 前端 SHALL NOT 发出带起止日期参数的lane-dynamics请求或换source档；A14仍是唯一基准，B仅补其同ID历史，已缓存其他档不得因日期命中被提升为base

#### Scenario: 365档池请求去重
- **WHEN** sourceDays明确选择365，或该板块365请求已在途
- **THEN** 档/池 SHALL 共用同一次365读取及成功缓存，不增加第五档缓存

#### Scenario: 有界缓存与当前阅读稳定
- **WHEN** 第15份报告或第21篇文章成功读取，且最先入缓存项正在展开阅读
- **THEN** SHALL FIFO逐出到14份/20篇上限，但当前内容、展开态、选日、焦点与滚动保持；切板块/卸载释放缓存，失败不计成功项

#### Scenario: 稳定 ID 关联
- **WHEN** 当日工作面读取某主题桶的线索与摘要
- **THEN** 关联 SHALL 经 `section_id`/thread id 命中日报详情数据；稳定ID缺失或读取失败时 SHALL 降级注明，MUST NOT 借用相似标题；标题文字不同不影响ID关联

#### Scenario: 零写边界
- **WHEN** 用户拖选观察日期、切换档位、展开摘要或相关文章
- **THEN** 全程 SHALL 只有只读 GET 请求，无任何写请求或状态变更

#### Scenario: 零泳道仍按报日读分区
- **WHEN** 版块 0 正式泳道但有日报，用户在日期轴选择任一报日并展开「当日其他话题」
- **THEN** 系统 SHALL 经既有报日索引与日报详情端点列出该日报中不属于正式/休眠泳道的分区与全量 threads；MUST NOT 新增端点/参数，MUST NOT 把候选池或管理数据当分区列出

#### Scenario: 报日索引不改聚合口径
- **WHEN** 前端为日期轴加载报日索引与所选日报告详情
- **THEN** SHALL 复用既有日报列表/详情端点与共享缓存；MUST NOT 改变 lane-dynamics 的名单/排序/休眠/候选口径，MUST NOT 产生写请求

### Requirement: 日报后泳道态势演进结算

每日日报生成完成（该板块报告保存成功）后，系统 SHALL 为该板块的活跃泳道异步结算滚动 14 天态势：输入为该泳道近 14 天锚定 section 的「日期｜节标题｜thread 标题」清单，并附带**量级信号段**（系统计算的每日线索量与近 7 天 / 上个 7 天合计对比，非 LLM 产出、随素材注入）；输出为**单版演进叙事态势句**——一段 150-200 字的中文纯文本，按「阶段划分（带大致日期锚）→ 每阶段驱动/转折（事件仅作转折标记，不展开）→ 当前落点与局势判断 → 量级佐证」的演进叙事组织，随每日结算覆盖更新（每泳道保留一份当前快照，含 as_of 汇总截止日）。态势句 SHALL NOT 逐日罗列事件、SHALL NOT 预测未来走向（只描述已发生的变化）、SHALL NOT 编造清单与量级信号之外的数字。快照的 `rolling_detail` 字段 SHALL 固定写入空值（不再产长版）；存量快照的历史长版数据 SHALL 保留原值不清理不回填，下个日报日结算自然覆盖。单次结算调用的输出 token 预算 SHALL 覆盖单版上限。结算 SHALL NOT 阻塞或失败中断日报主流程（松耦合：失败仅记日志，下个日报日自愈）。

#### Scenario: 日报完成后结算

- **WHEN** 某板块当日日报生成成功
- **THEN** 系统 SHALL 异步为该板块活跃泳道逐个刷新滚动 14 天态势快照（单版演进叙事态势句，写入 `rolling_summary`；`rolling_detail` 置空）

#### Scenario: 量级信号随素材注入

- **WHEN** 结算调用构造 prompt 素材
- **THEN** 系统 SHALL 在事件清单之外附带量级信号段：窗口内每日线索量与近 7 天 / 上个 7 天合计对比，由系统按素材统计计算，SHALL NOT 依赖模型自行计数

#### Scenario: 演进叙事定位约束

- **WHEN** 结算调用构造 system 指令
- **THEN** 指令 SHALL 要求按阶段组织叙述（事件仅作转折标记）、SHALL 包含对逐日罗列与未来预测的显式禁止

#### Scenario: 机械截断保护

- **WHEN** 模型输出的态势句超过 200 字
- **THEN** 系统 SHALL 在 200 字窗口内最后一个句末标点（`。`/`！`/`？`/`…`）处收口后入库，SHALL NOT 从字中间切断；窗口内无可用句末标点（或标点过早，低于预算 60%）时 SHALL 退回 rune 安全截断

#### Scenario: 空输出守卫

- **WHEN** 模型输出为空或纯空白
- **THEN** 该泳道本次结算 SHALL 记为失败并跳过，快照保持旧值，下个日报日重试

#### Scenario: 结算失败不阻塞

- **WHEN** 某泳道结算 LLM 调用失败
- **THEN** 日报主流程 SHALL 不受影响，失败记日志；该泳道快照保持旧值（或缺失态），下个日报日重试

#### Scenario: 存量快照兼容

- **WHEN** 某泳道快照为存量数据（`rolling_detail` 含历史长版）
- **THEN** 系统 SHALL 保持该值原样入库不清理，读取方 SHALL 以 `rolling_summary` 为唯一态势内容源，下个日报日结算覆盖时 `rolling_detail` 自然置空

#### Scenario: 态势句与时间线同窗

- **THEN** 态势句的素材窗口与时间线窗口 SHALL 同为滚动 14 天（以最近一份已完成日报为期）

### Requirement: 泳道动态聚合端点

系统 SHALL 提供 `GET /semantic-boards/:id/lane-dynamics?days=14` 端点，单次返回该板块全部泳道卡数据：泳道元信息（含 watch 关联标识）、滚动态势（`summary` 单版演进叙事态势句 + `as_of` 与快照存在性；`detail` 仅存量历史值透出，新结算恒空、omitempty 不出现）、逐日发展时间线（日期 → 事件 thread 标题列表）、候选清单。days 参数 SHALL 支持指定时间线窗口（默认 14）。

#### Scenario: 单请求聚合

- **WHEN** 前端请求某板块的 lane-dynamics
- **THEN** 响应 SHALL 包含主卡区全部数据与候选栏数据，前端无需再发 N 次逐泳道请求

#### Scenario: 存量长版透出

- **WHEN** 某泳道快照含存量历史长版（其泳道尚未经历新结算）
- **THEN** 响应中该泳道态势对象 SHALL 原样透出存量 `detail` 值（读旧值无害）；新结算后该字段恒空、不再出现，不存在「长版缺失」二级降级语义

#### Scenario: 无态势快照降级

- **WHEN** 某泳道无滚动态势快照（如新泳道或首个日报日前）
- **THEN** 响应 SHALL 明示快照缺失，前端 SHALL 展示「待结算」占位且发展时间线照常渲染
