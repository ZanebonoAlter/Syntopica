## ADDED Requirements

### Requirement: 版块调查可引用经授权的跨版块泳道
每份版块调查仍 SHALL 关联发起版块的一份不可变 `board_brief`，但研究循环 MAY 读取并引用本次会话动态授权的其他版块泳道。跨版块 lane reference SHALL 同时携带 lane ID 和所属 board ID；综合阶段 MUST 剔除未授权、已删除或无法确认归属的引用。

#### Scenario: 调查引用其他版块泳道
- **WHEN** 研究循环通过内部搜索发现并读取另一个版块的授权泳道，且该材料与问题相关
- **THEN** 调查报告可以引用该泳道，并显示其所属版块和证据用途

#### Scenario: 父简报归属保持不变
- **WHEN** 调查引用了多个其他版块泳道
- **THEN** 调查仍归属于原始 board 和 parent brief，MUST NOT 改写父简报的版块归属

#### Scenario: 未授权跨版块引用被剔除
- **WHEN** 综合输出包含未在本次会话授权记录中的跨版块 lane reference
- **THEN** 系统机械剔除该引用，且不得把它用于支持调查结论

### Requirement: 简报消费已确认的跨版块关系
生成新版块简报前，系统 SHALL 查询与该版块相关、状态为 `confirmed` 且未过期的跨版块关系，在确定性数量和字符预算内作为“已确认外部关系背景”注入。版块简报本身 MUST NOT 因此主动调用联网工具；选中的关系及证据引用 SHALL 冻结到本次输入快照。原有 `relationships` 继续表示本版块态势卡之间的当次关系，跨版块关系 SHALL 以独立字段返回，MUST NOT 混入本版块 lane ID 校验集合。

#### Scenario: 新简报消费确认关系
- **WHEN** 版块存在仍有效的 `confirmed` 跨版块关系并生成新简报
- **THEN** 系统在预算内注入该关系，结果以独立跨版块关系字段返回，并可追溯关系建议 ID

#### Scenario: 未确认关系不进入简报
- **WHEN** 与版块相关的关系仅为 `unresolved`、`proposed`、`dismissed` 或 `expired`
- **THEN** 这些关系均不进入简报 prompt、输入快照或跨版块关系字段

#### Scenario: 简报生成期间不联网
- **WHEN** 简报消费已确认关系
- **THEN** 简报仍只执行原有单次生成调用，不调用 `web_search` 或 `fetch_page`

#### Scenario: 旧简报保持不可变
- **WHEN** 用户在一份简报生成后确认或撤销跨版块关系
- **THEN** 已有简报不被改写，关系变更仅影响之后生成的简报

#### Scenario: 关系数量超过预算
- **WHEN** 有效确认关系超过简报注入上限
- **THEN** 系统按确定性排序选择预算内关系并记录截断数量，MUST NOT 随数据库返回顺序漂移
