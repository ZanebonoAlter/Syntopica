## Purpose

AI 能力路由系统：为每个 AI capability（summary、digest_polish、topic_tagging、embedding 等）维护独立的路由配置、provider 绑定与并发配额，使各业务流程通过其绑定的 capability 加载路由与 provider，互不干扰。

## Requirements

### Requirement: 能力与业务用途绑定

系统 SHALL 为每个 AI capability 维护与业务用途的唯一绑定：`summary` SHALL 驱动文章自动总结；`digest_polish` SHALL 驱动日报生成；`topic_tagging` SHALL 驱动事件标签提取与标签相关的语义操作；`embedding` SHALL 驱动向量嵌入；`open_notebook` SHALL 驱动日报页边注问答（margin notes QA，通用概念 + 当天文章上下文 RAG 作答与术语抽取）。每个业务流程 SHALL 仅通过其绑定的 capability 加载路由与 provider。

#### Scenario: 文章总结使用 summary 路由

- **WHEN** 文章自动总结流程（`summarizeContent`）调用 LLM
- **THEN** 系统 SHALL 通过 `summary` capability 加载路由与 provider

#### Scenario: 日报生成使用 digest_polish 路由

- **WHEN** 日报生成流程的任一 LLM 调用（聚类、要闻、叙事）执行
- **THEN** 系统 SHALL 通过 `digest_polish` capability 加载路由与 provider

#### Scenario: 页边注问答使用 open_notebook 路由

- **WHEN** 日报页边注批注的问答流程（回答生成与术语抽取）调用 LLM
- **THEN** 系统 SHALL 通过 `open_notebook` capability 加载路由与 provider，并按该 capability 的并发配额限流

### Requirement: 默认并发配额独立
系统 SHALL 为每个 capability 提供独立的默认并发上限信号量，可被路由级 `MaxConcurrency` 覆盖。不同 capability 的并发配额 SHALL 互不挤占。

#### Scenario: digest_polish 与 topic_tagging 并发隔离
- **WHEN** 日报生成与标签提取同时进行
- **THEN** `digest_polish` 与 `topic_tagging` SHALL 使用各自独立的信号量，一方占满配额时 SHALL NOT 阻塞另一方

### Requirement: 废弃的 article_completion
系统 SHALL NOT 定义 `article_completion` capability 常量，SHALL NOT 在任何 LLM 调用中使用 `article_completion` 作为 capability。

#### Scenario: 无 article_completion 调用
- **WHEN** 系统发起任何 LLM 调用
- **THEN** 该调用的 capability SHALL NOT 为 `article_completion`

### Requirement: Provider 模型类型区分（model_kind）

每个模型（ai_models）SHALL 声明其功能类型 `model_kind`，取值 `llm` 或 `embedding`，默认 `llm`。`model_kind` 表达模型功能（LLM 推理 / 向量嵌入），与表达协议的 vendor `provider_type`（openai_compatible / ollama）正交，互不替代。vendor 模型清单 API SHALL 返回每个模型的 `model_kind`；模型创建/更新 SHALL 接受并持久化 `model_kind`，缺省按 `llm`（约束细则见 ai-vendor-model「模型配置约束」）。

#### Scenario: 新建 provider 默认 llm

- **WHEN** 刷新或新建一个模型，未指定 model_kind
- **THEN** 系统 SHALL 保存 model_kind=llm

#### Scenario: 列表返回 model_kind

- **WHEN** 调用 vendor 模型清单 API（列表/刷新结果）
- **THEN** 每个模型对象 SHALL 包含 model_kind 字段（llm 或 embedding）

#### Scenario: 显式声明 embedding 类型

- **WHEN** 用户为本地 vendor 的模型选择 model_kind=embedding
- **THEN** 系统 SHALL 保存 model_kind=embedding

#### Scenario: model_kind 与 provider_type 正交

- **WHEN** 一个 provider_type=openai_compatible 的本地 vendor 下存在 model_kind=embedding 的模型（llama.cpp embedding 服务）
- **THEN** 系统 SHALL 接受该组合，两者互不冲突

### Requirement: 路由绑定的模型类型约束

能力路由的绑定对象 SHALL 为模型（ai_models），绑定关系由 `ai_route_model` 关联承载，并维护顺位顺序。系统 SHALL 在路由保存（UpsertRoute）时按 capability 类别校验绑定模型的 `model_kind`：`embedding` capability 路由 SHALL 只绑定 `model_kind=embedding` 的模型；其余（llm 类）capability 路由 SHALL 只绑定 `model_kind=llm` 的模型。违反时系统 SHALL 拒绝保存并返回明确错误（指明冲突的模型名与期望类型）。未启用（enabled=false）与 stale 的模型 SHALL NOT 出现在绑定候选中。

#### Scenario: embedding 路由拒绝 llm provider

- **WHEN** 保存 embedding capability 路由，绑定的某模型 model_kind=llm
- **THEN** 系统 SHALL 拒绝保存，返回错误（如「embedding 路由不能挂 llm 模型『X』」），路由绑定不变

#### Scenario: llm 路由拒绝 embedding provider

- **WHEN** 保存 summary capability 路由，绑定的某模型 model_kind=embedding
- **THEN** 系统 SHALL 拒绝保存并返回错误

#### Scenario: 同类绑定放行

- **WHEN** 保存 embedding 路由绑定 model_kind=embedding 的模型，或 summary 路由绑定 model_kind=llm 的模型
- **THEN** 系统 SHALL 接受保存

#### Scenario: 存量迁移后冲突告警

- **WHEN** 版本化迁移执行后，存在「同一模型同时绑定在 embedding 路由与 llm 路由」的存量数据
- **THEN** 迁移 SHALL 以日志告警列出冲突模型，且后续任何一次该路由的保存 SHALL 被类型校验拦截，直至用户手动修正

#### Scenario: stale 与未启用模型不可绑定

- **WHEN** 路由绑定候选列表加载，某模型处于 stale 或 enabled=false
- **THEN** 该模型 SHALL NOT 出现在候选中；直接提交绑定该模型的保存请求 SHALL 被拒绝

### Requirement: 被摘空的线路允许存在

线路因模型随 vendor 删除（级联解绑）或模型单独停用/摘除而不再持有任何绑定时，系统 SHALL 保留该线路及其配置。后续调用该能力的 LLM/嵌入时，系统 SHALL 走既有的「无可用模型」错误路径，SHALL NOT 因线路为空而 panic 或产生不一致状态。

#### Scenario: 线路被摘空后调用该能力

- **WHEN** 某能力的全部绑定均被删除（线路被摘空）后，业务流程调用该能力
- **THEN** 系统 SHALL 返回既有的无可用模型错误，错误信息可预期

### Requirement: 绑定项可携带思考档覆盖

`ai_route_model` 绑定项 MAY 携带 `thinking_level` 覆盖值（空=继承模型默认档），实现按场景分配思考等级；覆盖语义与解析优先级由 ai-thinking-levels 能力约束。绑定项的保存（增删、顺位调整、覆盖档设置）SHALL 在同一路由保存操作内原子完成。

#### Scenario: 顺位调整与覆盖档同时保存

- **WHEN** 用户在能力路由界面调整某能力绑定顺序并把首位的覆盖档设为 high 后点击保存
- **THEN** 系统 SHALL 在一次 UpsertRoute 中同时持久化新顺序与覆盖值

### Requirement: 能力路由 UI 展示业务用途说明

设置页「能力路由」的每条能力泳道行 SHALL 在能力名下方展示一行「供血对象」说明文案，说明该 capability 的真实 LLM 调用方与对应用户可见功能。全部在册能力（`summary` / `topic_tagging` / `digest_polish` / `embedding` / `open_notebook` / `feed_discovery` / `data_enrichment_news` / `data_enrichment_analysis`）SHALL 均有非空说明；说明文案 SHALL 与该 capability 在后端的实际调用方一致，SHALL NOT 与真实供血关系矛盾。

已验证的关键分工事实（文案必须如实反映）：
- `digest_polish` 供血日报生成（聚类/要闻/叙事）及泳道动态与泳道趋势「近 14 天」档（`daily_report.lane_snapshot` 等 topicgraph LLM 调用）；
- `data_enrichment_news` 供血话题生命线周期归档摘要（`data_enrichment.summarize_context`，月/年定时任务生成），即泳道趋势「月」「年」档的数据源——与日报无关。

#### Scenario: 八条能力均有供血说明

- **WHEN** 设置页能力路由泳道行渲染完成
- **THEN** 八条在册能力行 SHALL 各渲染一行非空说明文案

#### Scenario: 新闻总结说明与真实供血一致

- **WHEN** 用户查看「新闻总结」（`data_enrichment_news`）泳道行
- **THEN** 说明文案 SHALL 指向话题生命线周期归档（泳道趋势月/年档），SHALL NOT 暗示其供血日报

#### Scenario: 日报润色说明覆盖日报与近14天档分工

- **WHEN** 用户查看「日报润色」（`digest_polish`）泳道行
- **THEN** 说明文案 SHALL 涵盖日报生成与泳道趋势「近 14 天」档两个消费方

#### Scenario: 未收录能力不渲染空说明行

- **WHEN** 泳道行对应的 capability 不在说明文案表中（如未来新增能力尚未补文案）
- **THEN** 该行 SHALL NOT 渲染空的说明文案占位（直接不输出该小字行）
