## MODIFIED Requirements

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
