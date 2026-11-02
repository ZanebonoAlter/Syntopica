## MODIFIED Requirements

### Requirement: 泳道动态视图入口
版块态势页（`/tags/:boardId`）SHALL 以「泳道 × 日期」矩阵作为版块选中后的默认首屏概览，取代原「板块内容」tab 内的泳道卡片区。矩阵 SHALL 只读展示（不含任何话题生命周期操作；治理右轨/抽屉的操作 SHALL NOT 触发矩阵以外的副作用）。演进带作为矩阵的选中泳道明细区，与矩阵共用同一聚合响应。

#### Scenario: 默认展示泳道动态区
- **WHEN** 用户选中某版块并进入版块态势页
- **THEN** 页面 SHALL 渲染泳道 × 日期矩阵（泳道动态视图），并在其下方渲染当前选中泳道的演进带

#### Scenario: 只读语义
- **THEN** 矩阵与演进带 SHALL 不提供归档/转正/合并等生命周期操作按钮（写入口在编排态与治理抽屉）

### Requirement: 泳道卡片构成
原「每条泳道渲染为一张卡片」的形态 SHALL 退役，改由矩阵行与演进带共同承载：矩阵行 SHALL 展示泳道名、追踪标识与断档；矩阵格内数字 SHALL 为同日各主题桶（section）线索数的最大值（桶峰值），行尾 MUST NOT 再渲染重复总数列（2026-10-27 矩阵阶段修订）。选中泳道的演进带 SHALL 展示按日期排列的节点（节点数字与点径 = 当日各主题桶线索数之和，线索密集日显示标签）、滚动态势句入口（最新结算快照逐字，标注 as_of 与「非本视窗总结」）、以及当日线索工作面。事件与日期的对应关系 SHALL 来自后端聚合响应，前端 SHALL NOT 自行推断或重排。

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

## ADDED Requirements

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
