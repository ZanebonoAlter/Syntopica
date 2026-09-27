## ADDED Requirements

### Requirement: Explicit Query and Personalized Refresh
系统 SHALL 在现有发现页“为你推荐”顶部常驻手动查询框和“找订阅源”，并与“刷新推荐”分离。查询结果 SHALL 标注本次输入并提供返回个性化推荐入口；查询推荐归属全局，不以兴趣记录的版块归属代替查询上下文。系统 MUST NOT 以兴趣记录页替代查询入口。

#### Scenario: 手动查询完整流程
- **WHEN** 用户输入非空内容并点击找订阅源
- **THEN** 系统显示本次查询的执行状态及独立结果，返回为你推荐后显示原有个性化推荐，成功查询形成独立兴趣记录

#### Scenario: 输入错误和重复提交
- **WHEN** 输入为空、仅空白或超过输入上限，或同一查询正在执行
- **THEN** 系统对无效输入就地反馈且不执行查询，对执行中的重复提交不重复创建同一次请求的兴趣记录或推荐

#### Scenario: 查询失败后恢复
- **WHEN** 查询执行失败
- **THEN** 输入与之前已展示的结果保持可见并标明未更新，提供重试，不将失败显示为零匹配，也不发布半成品兴趣记录

### Requirement: Selected Recommendations Only
系统 SHALL 仅发布精排明确选择的有效候选，向精排提供本次查询或版块与行为上下文。精排未选择的候选 MUST NOT 因处于粗筛列表而入选；空选择 SHALL 作为成功的零推荐处理。精排未配置、调用失败或结果无效时 SHALL 明示失败，保留旧结果且不得伪装已更新。候选标识必须来自该轮提供给精排的集合，重复标识不得导致重复卡片。

#### Scenario: 精排选择子集
- **WHEN** 粗筛有六条且精排有效选择其中两条
- **THEN** 本轮仅两条成为有效结果，其余四条不以无理由推荐发布；重复标识只出现一次

#### Scenario: 零选择不是故障
- **WHEN** 精排有效返回空选择
- **THEN** 本轮展示没有合适来源的成功空态，不用粗筛候选凑数

#### Scenario: 服务或协议异常
- **WHEN** 精排不可用、返回不可解析内容或候选集合之外的标识
- **THEN** 本轮失败且不发布新结果，旧结果标记未更新，用户可重试并获得可定位的错误信息

### Requirement: Independent Interest Records and Bounded Influence
系统 SHALL 独立保存成功查询的兴趣表达、时间和匹配归属，不再将多次查询累积平均为一个种子向量。归属匹配 SHALL 仅考虑真实且有效的版块，无匹配则独立保留在未匹配组；MUST NOT 挂普通标签、自动创建版块或让 LLM 判断是否为持续兴趣。时间窗口、最大参与条数及行为成熟度对应的份额 SHALL 可配置且有校验；达到窗口上限的条目不再参与，历史保留并展示原因。其他条件相同时，记录变旧或所属范围行为成熟度上升 MUST NOT 提高其份额；列表条数增长不得无界增加问答总份额。

#### Scenario: 多主题查询不互相平均
- **WHEN** 用户分别查询体育和软件且两次均成功
- **THEN** 产生两条可独立查看的兴趣记录，不用合成的平均兴趣替代二者，未匹配记录不强挂版块

#### Scenario: 有界衰减
- **WHEN** 可用记录数超过配置上限，或记录年龄达到窗口上限
- **THEN** 参与数不超过配置上限，达到年龄上限的记录退出参与但仍显示为历史，不因此被当作拒绝

#### Scenario: 行为成熟后让位
- **WHEN** 记录与时间相同但所属范围的行为成熟度提高
- **THEN** 问答推荐份额不增加，且不挤占版块基础召回的保底份额；未匹配记录按全局范围同样受限

### Requirement: Independent Board and Behavior Recall
个性化刷新 SHALL 独立使用版块自身语义与行为画像召回候选，再合并去重和精排，不先把两路压成单个混合向量。配置 SHALL 为有合格候选的版块基础路保留正数召回份额；此保证是进入精排而非强制发布。兴趣记录仅作为受限补充。向量模型、配置与维度不兼容 SHALL 阻断该轮而非混算。

#### Scenario: 近期阅读不能挤掉版块方向
- **WHEN** 日本新闻版块近期行为集中于财经且两路均有合格候选
- **THEN** 进入精排的集合保留版块基础路候选，同时包含行为路允许份额内的财经候选；最终仍由精排选择

#### Scenario: 单路缺失与重复候选
- **WHEN** 某一路无可用画像或候选，或同一候选被多路命中
- **THEN** 缺失路不伪造候选，其他合格路径可继续；同一候选只展示一次并保留实际命中的来源信息

#### Scenario: 模型不兼容
- **WHEN** 画像与目录向量配置、模型或维度不同
- **THEN** 本轮明确失败且不发布新结果，既有推荐不伪装为已刷新

### Requirement: Recommendation Lifecycle and Exclusion
系统 SHALL 区分待处理、自动过期、暂时不看、长期排除及已订阅。过期窗口 SHALL 可配置；到期的旧卡退出默认列表并保留历史，自动过期 MUST NOT 等同拒绝。成功刷新对仍入选且待处理的相同推荐更新理由与时间而非追加；失败不得伪造成功刷新时间。暂时不看默认30天冷却，可配置；冷却未到期和长期排除 SHALL 跨问答与个性化来源生效。候选启用开关不得解除长期排除。

#### Scenario: 刷新与过期
- **WHEN** 旧待处理卡再次入选，另一张未入选卡达到过期窗口
- **THEN** 前者更新而非重复，后者退出默认列表且标自动过期；后者未来可重新入选，不被视为用户拒绝

#### Scenario: 冷却边界及长期排除
- **WHEN** 同一源被暂时不看且当前时间早于冷却结束，或被长期排除尚未恢复
- **THEN** 手动查询和刷新均不重新推荐；到达冷却结束可重新参与，长期排除则仍被阻断

#### Scenario: 恢复不是订阅
- **WHEN** 用户恢复长期排除或撤销暂时不看
- **THEN** 仅恢复推荐资格，不自动生成推荐、不创建订阅，也不改变已订阅源

### Requirement: Recommendation Display and Subscription Boundaries
推荐 SHALL 展示有效名称、内容介绍、选择理由、实际召回来源、可用性和检查时间；无检查记录必须明示未验证，不伪造时间或匹配百分比。原生 RSS 与 RSSHub SHALL 使用不同订阅路径：原生地址明确确认后创建，需参数的 RSSHub 填参并验证成功后才创建。相同有效地址重复确认不得创建重复订阅，失败保留输入可重试。

#### Scenario: 原生与参数订阅
- **WHEN** 用户订阅原生 RSS 或必须填参的 RSSHub 候选
- **THEN** 前者明确确认地址，后者按既有字典优先和输入兜底规则填参验证；失败均不显示已订阅，成功后显示已订阅并防止重复创建

#### Scenario: 多来源和缺失检查信息
- **WHEN** 一条推荐被版块与行为两路命中且尚未检查
- **THEN** 展示两种真实召回来源和未验证状态，不将其显示为已可用或虚构单一路由来源

### Requirement: Recoverable Legacy Discovery Migration
升级 SHALL 保留既有订阅、文章及拒绝历史，并记录旧种子与推荐的迁移结果。无法恢复原查询内容或有效版块归属的旧种子 SHALL 标记为历史且退出新召回，不编造原查询、不将普通标签显示成版块；迁移 SHALL 可重复执行而不重复生成记录。

#### Scenario: 错挂种子的升级
- **WHEN** 旧种子挂在普通标签且缺少可恢复原查询
- **THEN** 保留旧数据关联及迁移说明，默认不参与新推荐，不清除已有订阅；再次迁移不产生副本

## MODIFIED Requirements

### Requirement: Recommendation API Carries Param Options
`getRecommendations` 响应 SHALL 区分原生 RSS 与 RSSHub 候选。RSSHub 的 route 对象 SHALL 附带 `param_options`（按 `param_name` 分组的可选值数组），使前端一次拿全、无需二次请求。原生 RSS SHALL 提供实际订阅地址，不伪造参数规格。

#### Scenario: 响应包含 param_options
- **WHEN** 客户端请求推荐列表
- **THEN** 每条 RSSHub route 对象含 `param_options` 字段（无字典数据时为空集合）

#### Scenario: 原生候选无参数表单
- **WHEN** 推荐候选为原生 RSS
- **THEN** 响应标明原生类型及实际订阅地址，客户端不渲染 RSSHub 参数表单

### Requirement: Official Documentation Link
每条 RSSHub 推荐路由的填参表单 SHALL 提供“官方文档”链接，URL = `{doc_base}/routes/{namespace}#{slug}`（slug 基于路由 path 推导）。`doc_base` SHALL 可经 `aisettings` 配置（默认 `https://docs.rsshub.app`），以应对官方文档站访问受限时切换镜像；客户端获取配置失败 SHALL 兜底默认值。原生 RSS MUST NOT 伪造 RSSHub 文档链接。

#### Scenario: 表单提供文档链接
- **WHEN** 用户打开任意 RSSHub 推荐路由的填参表单
- **THEN** 表单底部显示“官方文档”按钮，链接指向该路由的文档页

#### Scenario: doc_base 可配置
- **WHEN** 管理员修改 `aisettings` 中的 `doc_base`
- **THEN** 后续文档链接使用新 base 生成

#### Scenario: 配置获取失败及原生来源
- **WHEN** 配置获取失败或当前候选为原生 RSS
- **THEN** RSSHub 候选使用默认文档 base，原生 RSS 不展示 RSSHub 文档链接
