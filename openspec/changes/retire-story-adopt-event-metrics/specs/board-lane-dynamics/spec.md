## MODIFIED Requirements

### Requirement: 泳道卡片构成

原「每条泳道渲染为一张卡片」的形态 SHALL 退役，改由矩阵行与演进带共同承载：矩阵行 SHALL 展示泳道名、追踪标识与断档；矩阵格内数字 SHALL 为该泳道当日的线索合计（各主题桶线索数之和，事件线退役后不再有事件线量纲，也不再有桶峰值口径），行尾 MUST NOT 再渲染重复总数列。矩阵格 SHALL NOT 渲染颜色通道，格值与 title/aria SHALL 承载全部信息。选中泳道的演进带 SHALL 展示按日期排列的节点（节点数字与点径 = 当日各主题桶线索数之和，与矩阵日格同一量纲，线索密集日显示标签）、滚动态势句入口（最新结算快照逐字，标注 as_of 与「非本视窗总结」）、以及当日线索工作面。事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排。

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

#### Scenario: 矩阵格与演进节点同量纲
- **WHEN** 某泳道某日两个主题桶分别含 3 条与 4 条线索
- **THEN** 矩阵日格 SHALL 显示 7，演进带该日节点数字 SHALL 同为 7
- **AND** SHALL NOT 一个取桶峰值 4、另一个取合计 7

### Requirement: 展示范围与排序

泳道动态区 SHALL 展示「关注（active）」与「我在追踪（watch 关联）」的泳道，排序 SHALL 为泳道当天热度降序 → 全窗线索合计降序 → topic id 升序（热度口径与权重见 `lane-heat-metric`）；watch 关联泳道 SHALL 带追踪标识。近 14 天无锚定 section 的 active 泳道（沉寂）SHALL NOT 展示。candidate 泳道 SHALL NOT 出现在主卡片区。

排序口径 SHALL NOT 使用 section 条数：泳道内主题分桶会把同一话题同一天的内容拆成多条 section，按条数排序会让"当天拆得多的泳道"虚高。排序 SHALL NOT 使用事件线数或事件数（事件线退役后无此输入）。

#### Scenario: 活跃排序

- **GIVEN** 泳道 A 近 14 天有 6 天出现锚定 section（其中某天 3 条），泳道 B 有 3 天（每天 1 条），A 的当天热度高于 B
- **WHEN** 渲染泳道动态区
- **THEN** A SHALL 排在 B 前（泳道当天热度降序），SHALL NOT 因为 A 的 section 条数被拆分放大而进一步加权

#### Scenario: 同日多条只计一天

- **GIVEN** 泳道 A 仅有 1 天出现锚定 section，但该天被拆成 4 条 section；泳道 B 有 2 天、每天 1 条
- **WHEN** 渲染泳道动态区
- **THEN** 行序 SHALL 由泳道当天热度决定，A 被拆成 4 条 section 本身 SHALL NOT 抬高其热度

#### Scenario: watch 标识

- **WHEN** 某泳道被用户 watch（追踪）关联
- **THEN** 泳道行 SHALL 带追踪标识（与普通 active 行视觉区分）

#### Scenario: 沉寂不展示

- **WHEN** 某 active 泳道近 14 天无任何锚定 section
- **THEN** 该泳道 SHALL NOT 渲染

#### Scenario: 候选不进主区

- **WHEN** 某 candidate 泳道近 14 天有锚定 section
- **THEN** 该泳道 SHALL NOT 出现在主区（仅可能出现在候选栏）

### Requirement: 泳道动态聚合端点

系统 SHALL 提供 `GET /semantic-boards/:id/lane-dynamics?days=14` 端点，单次返回该板块全部泳道行数据：泳道元信息（含 watch 关联标识）、泳道当天热度（`lane_heat`，新口径）、全窗线索合计、滚动态势（`summary` 单版演进叙事态势句 + `as_of` 与快照存在性；`detail` 仅存量历史值透出，新结算恒空、omitempty 不出现）、逐日发展时间线（日期 → 该日线索合计与事件 thread 标题列表）、候选清单。days 参数 SHALL 支持指定时间线窗口（默认 14）。

逐日条目 SHALL 给出该泳道该日的线索合计（thread 数），SHALL NOT 再下发事件线数、最热事件线身份或未归组事件数。候选清单 SHALL 附带候选泳道当天（锚点日）的线索合计，使版块关注度能够对候选泳道按候选权重计入；SHALL NOT 给候选泳道编造时间线。

#### Scenario: 单请求聚合

- **WHEN** 前端请求某板块的 lane-dynamics
- **THEN** 响应 SHALL 包含主区全部数据与候选栏数据，前端无需再发 N 次逐泳道请求

#### Scenario: 存量长版透出

- **WHEN** 某泳道快照含存量历史长版（其泳道尚未经历新结算）
- **THEN** 响应中该泳道态势对象 SHALL 原样透出存量 `detail` 值（读旧值无害）；新结算后该字段恒空、不再出现，不存在「长版缺失」二级降级语义

#### Scenario: 无态势快照降级

- **WHEN** 某泳道无滚动态势快照（如新泳道或首个日报日前）
- **THEN** 响应 SHALL 明示快照缺失，前端 SHALL 展示「待结算」占位且发展时间线照常渲染

#### Scenario: 逐日负载为线索合计

- **WHEN** 前端读取某泳道某日的时间线条目
- **THEN** 该条目 SHALL 带该日线索合计
- **AND** SHALL NOT 带事件线数、最热事件线标题或未归组事件数

#### Scenario: 候选泳道带当天线索数

- **WHEN** 候选栏列出某 candidate 泳道
- **THEN** 该条目 SHALL 带其锚点日的线索合计
- **AND** 版块关注度 SHALL 能据此按候选权重计入该泳道
