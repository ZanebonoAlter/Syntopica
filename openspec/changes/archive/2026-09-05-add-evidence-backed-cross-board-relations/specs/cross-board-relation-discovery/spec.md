## Purpose

为任意版块观察或研究问题提供与领域无关、可核查且可人工裁决的跨版块关系发现能力，使外部证据成为内部知识之间的桥梁，同时避免未经验证的模型判断污染简报。

## ADDED Requirements

### Requirement: 证据优先的关系候选发现
系统 SHALL 从一个明确的内部来源快照（版块、父简报以及 observation 或 research question）启动关系发现，先检索外部证据并提取目标概念，再尝试映射内部目标；系统 MUST NOT 以预置领域关键词或全量版块两两组合代替该流程。手动发现 SHALL 默认可用；自动发现 SHALL 按版块配置且默认关闭，并受每次运行的候选数、搜索次数和超时预算约束。

#### Scenario: 从观察手动发现
- **WHEN** 用户从一条仍属于当前版块简报的 observation 发起关系发现
- **THEN** 系统异步创建以该 observation 不可变快照为来源的发现任务，且不要求用户预先选择目标版块

#### Scenario: 从研究问题手动发现
- **WHEN** 用户从父简报的一条 research question 发起关系发现
- **THEN** 系统以问题及父简报上下文作为来源运行同一发现引擎

#### Scenario: 自动发现默认关闭
- **WHEN** 版块未显式启用自动关系发现并生成一份新简报
- **THEN** 系统 MUST NOT 自动发起外部搜索或创建关系建议

#### Scenario: 自动发现遵守预算
- **WHEN** 已启用自动发现的版块生成新简报且可处理观察数量超过配置预算
- **THEN** 系统只处理确定性排序后的预算内来源并记录跳过数量，MUST NOT 扫描全量版块对

### Requirement: 外部概念到内部目标的保守解析
系统 SHALL 根据外部证据中提取的目标概念检索内部版块和泳道，并记录候选、映射依据与解析结果。只有唯一且达到配置门槛的目标才能成为 resolved target；多候选、低于门槛或无候选时 SHALL 保留 `target_concept` 并标记为 `unresolved`，MUST NOT 强制选择最相似目标。

#### Scenario: 唯一目标解析成功
- **WHEN** 一个外部概念仅有一个内部候选达到解析门槛
- **THEN** 建议记录目标版块/泳道 ID、映射依据和解析得分

#### Scenario: 多个候选无法消歧
- **WHEN** 多个内部目标均达到门槛且无法根据证据消歧
- **THEN** 建议保持 `unresolved` 并返回候选列表，不自动绑定其中任何一个

#### Scenario: 外部概念尚无内部目标
- **WHEN** 外部证据提到的概念在内部没有合格版块或泳道
- **THEN** 系统保留该外部概念及证据，且允许后续重新解析

### Requirement: 发现与验证相互隔离
候选发现与关系验证 SHALL 使用分离的阶段和快照。验证阶段 SHALL 仅依据关系假设、原始证据、内部目标材料、反证和替代解释，输出 `supported`、`contested`、`insufficient` 或 `rejected`；关系类型 SHALL 限于 `causal`、`common_driver`、`divergence`、`correlated`、`contextual`、`unclear`。模型自行声明的置信度 MUST NOT 直接决定验证结论或排序。

#### Scenario: 支持证据和反证共同进入验证
- **WHEN** 发现阶段产生一个已解析的关系候选
- **THEN** 验证阶段同时接收支持证据、已执行的反证查询结果和替代解释，不继承发现阶段的自评分

#### Scenario: 共同驱动而非直接因果
- **WHEN** 原始材料支持两个对象受第三方因素共同影响但不支持对象之间直接传导
- **THEN** 验证结果为 `supported/common_driver` 或 `contested`，MUST NOT 标为 `causal`

#### Scenario: 证据不足
- **WHEN** 验证材料不能区分竞争解释或缺少可核查依据
- **THEN** 验证结果为 `insufficient`，系统不强造获胜解释

#### Scenario: 关系被反证
- **WHEN** 可核查反证否定候选关系的核心陈述
- **THEN** 验证结果为 `rejected` 并保留反证来源

### Requirement: 可核查的外部证据契约
每条持久化建议 SHALL 保留来源 URL、标题、原始片段或正文引用、检索时间、支持/反证用途和工具调用来源。系统 SHALL 对引用执行可机械核对的保守校验；无法在工具原文中核对的引用 MUST 被剔除或标为不可核查。联网、正文抓取或模型步骤失败时 SHALL 记录结构化 gap，MUST NOT 生成伪造证据。

#### Scenario: 引用可在原文中核对
- **WHEN** 候选建议引用博查命中或抓取正文中的文字
- **THEN** 系统保存原始工具结果和可回溯引用，详情接口能够返回其 URL 与检索时间

#### Scenario: 模型输出不存在的引用
- **WHEN** 模型给出的引用无法在对应工具原文中匹配
- **THEN** 系统剔除该引用并降低为材料不足或 gap，MUST NOT 把它计入支持证据

#### Scenario: 博查不可用
- **WHEN** 博查未配置、超时或返回错误且不存在足够的其他可核查材料
- **THEN** 任务诚实结束为材料不足或失败状态并记录原因，不创建 `supported` 建议

### Requirement: 关系建议生命周期与幂等裁决
关系建议 SHALL 使用 `unresolved`、`proposed`、`confirmed`、`dismissed`、`expired` 状态。解析或验证不足的候选可保留为 `unresolved`；只有通过验证的候选可进入 `proposed`；只有用户可将 `proposed` 确认为 `confirmed` 或驳回为 `dismissed`。已确认关系超过有效期后 SHALL 进入 `expired`，所有终态转换 SHALL 留存操作者、时间和理由。相同来源快照、目标、关系陈述和证据版本 SHALL 生成稳定幂等键。

#### Scenario: 验证通过仍不自动确认
- **WHEN** 验证结果为 `supported`
- **THEN** 系统最多创建 `proposed` 建议，MUST NOT 自动进入 `confirmed`

#### Scenario: 用户确认建议
- **WHEN** 用户确认一条仍为 `proposed` 且证据未失效的建议
- **THEN** 系统原子地将其转换为 `confirmed` 并记录裁决时间

#### Scenario: 重复发现同一建议
- **WHEN** 同一幂等键已有未解决建议并再次运行发现
- **THEN** 系统不创建重复待处理行，并可更新运行留痕

#### Scenario: 驳回建议进入冷却
- **WHEN** 用户 dismiss 一条建议并提供理由
- **THEN** 系统记录 `dismissed`，并在配置冷却期内不重复创建相同幂等键的建议

#### Scenario: 已确认关系过期
- **WHEN** 当前时间超过已确认关系的有效期
- **THEN** 系统将其视为 `expired`，不再作为有效关系提供给下游

### Requirement: 关系建议可审阅和追溯
系统 SHALL 提供按来源版块、目标版块、状态和时间筛选的建议列表与详情，并允许用户确认、dismiss 及重新解析 unresolved 建议。详情 SHALL 展示来源快照、目标解析依据、关系类型、验证结论、支持证据、反证、gap 和生命周期记录。

#### Scenario: 审阅待处理建议
- **WHEN** 用户打开某版块的关系建议视图
- **THEN** 系统展示与该版块相关的 `proposed` 和 `unresolved` 建议，并明确区分证据、反证与未解决项

#### Scenario: 追溯已确认关系
- **WHEN** 用户从一条已确认关系进入详情
- **THEN** 用户能够追溯到原始版块简报、source observation/question、内部目标和外部证据 URL
