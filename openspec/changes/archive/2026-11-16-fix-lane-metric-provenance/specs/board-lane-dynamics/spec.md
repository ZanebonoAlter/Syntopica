# board-lane-dynamics — 矩阵格数字口径与权威载体对齐 delta

## MODIFIED Requirements

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
