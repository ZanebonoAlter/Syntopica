## Purpose

版块「板块内容」tab 的首屏泳道动态视图：以叙事级信息（滚动 14 天态势 + 逐日发展时间线）取代计数统计型态势版图，让用户一眼看懂每条关注/追踪泳道最近发生了什么；态势由每日日报生成后滚动结算维护。

## ADDED Requirements

### Requirement: 泳道动态视图入口
「板块内容」tab SHALL 在构成标签管理区下方承载「泳道动态」区，作为板块选中后的默认首屏概览。泳道动态区 SHALL 只读展示（不含任何话题生命周期操作；构成标签管理区操作 SHALL NOT 触发泳道动态区以外的副作用）。

#### Scenario: 默认展示泳道动态区
- **WHEN** 用户选中某板块并停留在「板块内容」tab
- **THEN** 构成标签管理区下方 SHALL 渲染「泳道动态」区

#### Scenario: 只读语义
- **THEN** 泳道动态区 SHALL 不提供归档/转正/合并等生命周期操作按钮

### Requirement: 泳道卡片构成
每条展示中的泳道 SHALL 渲染为一张卡片，包含：泳道名、滚动 14 天态势句（一句话）、发展时间线（近 14 天逐日事件脉络）。发展时间线 SHALL 以「日期 → 当日事件列表」的结构组织数据并按时间顺序呈现，每条事件 SHALL 对应一个日报 thread 级标题（事件源），使用户能看出该话题的逐日演进关系。

#### Scenario: 一句话态势展示
- **WHEN** 某泳道存在有效的滚动 14 天态势快照
- **THEN** 卡片 SHALL 展示该态势句并标注汇总截止日（as_of）

#### Scenario: 发展时间线结构
- **WHEN** 某泳道近 14 天内多个日期有锚定 section
- **THEN** 时间线 SHALL 按日期组织（同日事件归组、日与日按日期倒序——最新日期在最前），每条事件带 thread 级标题
- **AND** 事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排

#### Scenario: 单日多事件
- **WHEN** 某泳道同一天有 2 个锚定 section、各含多条 thread
- **THEN** 该日期节点下 SHALL 列出该日全部事件的 thread 标题（数量可截断但 SHALL 提示被折叠数）

### Requirement: 展示范围与排序
泳道动态区 SHALL 展示「关注（active）」与「我在追踪（watch 关联）」的泳道，按近 14 天锚定 section 数降序排列；watch 关联泳道 SHALL 带追踪标识。近 14 天无锚定 section 的 active 泳道（沉寂）SHALL NOT 展示。candidate 泳道 SHALL NOT 出现在主卡片区。

#### Scenario: 活跃排序
- **WHEN** 泳道 A 近 14 天 10 个 section、泳道 B 3 个
- **THEN** A 卡 SHALL 排在 B 卡前

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

### Requirement: 泳道动态批量端点
系统 SHALL 提供 `GET /semantic-boards/:id/lane-dynamics?days=14` 端点，单次返回该板块全部泳道卡数据：泳道元信息（含 watch 关联标识）、滚动态势（含 as_of 与是否存在快照）、逐日发展时间线（日期 → 事件 thread 标题列表）、候选清单。days 参数 SHALL 支持指定时间线窗口（默认 14）。

#### Scenario: 单请求聚合
- **WHEN** 前端请求某板块的 lane-dynamics
- **THEN** 响应 SHALL 包含主卡区全部数据与候选栏数据，前端无需再发 N 次逐泳道请求

#### Scenario: 无态势快照降级
- **WHEN** 某泳道无滚动态势快照（如新泳道或首个日报日前）
- **THEN** 响应 SHALL 明示快照缺失，前端 SHALL 展示「待结算」占位且发展时间线照常渲染

### Requirement: 日报后滚动态势结算
每日日报生成完成（该板块报告保存成功）后，系统 SHALL 为该板块的活跃泳道异步结算滚动 14 天态势：输入为该泳道近 14 天锚定 section 的 thread 标题列表，输出为不超过 100 字的中文态势句，随每日结算覆盖更新（每泳道保留一份当前快照，含 as_of 汇总截止日）。结算 SHALL NOT 阻塞或失败中断日报主流程（松耦合：失败仅记日志，下个日报日自愈）。

#### Scenario: 日报完成后结算
- **WHEN** 某板块当日日报生成成功
- **THEN** 系统 SHALL 异步为该板块活跃泳道逐个刷新滚动 14 天态势快照

#### Scenario: 结算失败不阻塞
- **WHEN** 某泳道结算 LLM 调用失败
- **THEN** 日报主流程 SHALL 不受影响，失败记日志；该泳道快照保持旧值（或缺失态），下个日报日重试

#### Scenario: 态势句与时间线同窗
- **THEN** 态势句的素材窗口与时间线窗口 SHALL 同为滚动 14 天（以最近一份已完成日报为期）

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
