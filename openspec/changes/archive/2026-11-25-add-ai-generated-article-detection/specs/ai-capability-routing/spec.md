## MODIFIED Requirements

### Requirement: 能力与业务用途绑定

系统 SHALL 为每个 AI capability 维护与业务用途的唯一绑定：`summary` SHALL 驱动文章自动总结；`digest_polish` SHALL 驱动日报生成；`topic_tagging` SHALL 驱动事件标签提取与标签相关的语义操作；`embedding` SHALL 驱动向量嵌入；`open_notebook` SHALL 驱动日报页边注问答（margin notes QA，通用概念 + 当天文章上下文 RAG 作答与术语抽取）；`ai_detection` SHALL 驱动文章级 AI 生成疑似度判定。每个业务流程 SHALL 仅通过其绑定的 capability 加载路由与 provider。

#### Scenario: 文章总结使用 summary 路由

- **WHEN** 文章自动总结流程（`summarizeContent`）调用 LLM
- **THEN** 系统 SHALL 通过 `summary` capability 加载路由与 provider

#### Scenario: 日报生成使用 digest_polish 路由

- **WHEN** 日报生成流程的任一 LLM 调用（聚类、要闻、叙事）执行
- **THEN** 系统 SHALL 通过 `digest_polish` capability 加载路由与 provider

#### Scenario: 页边注问答使用 open_notebook 路由

- **WHEN** 日报页边注批注的问答流程（回答生成与术语抽取）调用 LLM
- **THEN** 系统 SHALL 通过 `open_notebook` capability 加载路由与 provider，并按该 capability 的并发配额限流

#### Scenario: AI 生成判定使用 ai_detection 路由

- **WHEN** 文章级 AI 生成疑似度判定流程调用 LLM
- **THEN** 系统 SHALL 通过 `ai_detection` capability 加载路由与 provider，并按该 capability 的并发配额限流

### Requirement: 能力路由 UI 展示业务用途说明

设置页「能力路由」的每条能力泳道行 SHALL 在能力名下方展示一行「供血对象」说明，说明该 capability 的真实 LLM 调用方与对应用户可见功能。全部在册能力（`summary` / `topic_tagging` / `digest_polish` / `embedding` / `open_notebook` / `feed_discovery` / `ai_detection` / `data_enrichment_news` / `data_enrichment_analysis`）SHALL 均有非空说明，且文案 SHALL 与该 capability 在后端的实际调用方一致，SHALL NOT 与真实供血关系矛盾。

#### Scenario: 九条能力均有供血说明

- **WHEN** 设置页能力路由泳道行渲染完成
- **THEN** 九条在册能力行 SHALL 各渲染一行非空说明文案

#### Scenario: 供血文案与后端实际调用方一致

- **WHEN** 用户查看任一能力泳道行的说明文案
- **THEN** 文案 SHALL 与后端该 capability 的实际调用方一致，并至少覆盖以下已验证分工：`digest_polish` 供血日报生成（聚类/要闻/叙事/关键节点 AI 推荐）及泳道动态与泳道趋势「近 14 天」档（`daily_report.lane_snapshot`、`daily_report.key_event_recommend`）；`topic_tagging` 供血标签提取与语义操作，含版块体检 LLM 评分（`tagmanagement.board_health_checkup`）；`ai_detection` 供血文章级 AI 生成疑似度判定且与标签打标互不共用模型绑定；`data_enrichment_news` 供血话题生命线周期归档摘要（`data_enrichment.summarize_context`，泳道趋势月/年档，与日报无关）

#### Scenario: AI 生成判定能力有供血说明

- **WHEN** 用户查看「AI 生成检测」（`ai_detection`）泳道行
- **THEN** 说明文案 SHALL 指向文章级 AI 生成疑似度判定，SHALL NOT 暗示其供血日报生成或标签提取

#### Scenario: 新闻总结说明与真实供血一致

- **WHEN** 用户查看「新闻总结」（`data_enrichment_news`）泳道行
- **THEN** 说明文案 SHALL 指向话题生命线周期归档（泳道趋势月/年档），SHALL NOT 暗示其供血日报

#### Scenario: 日报润色说明覆盖日报与近14天档分工

- **WHEN** 用户查看「日报润色」（`digest_polish`）泳道行
- **THEN** 说明文案 SHALL 涵盖日报生成与泳道趋势「近 14 天」档两个消费方

#### Scenario: 未收录能力不渲染空说明行

- **WHEN** 泳道行对应的 capability 不在说明文案表中（如未来新增能力尚未补文案）
- **THEN** 该行 SHALL NOT 渲染空的说明文案占位（直接不输出该小字行）
