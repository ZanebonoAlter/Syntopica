## RENAMED Requirements

- FROM: `### Requirement: 健康就绪判定（宽松）`
- TO: `### Requirement: 健康就绪判定（核心路由硬条件）`

## MODIFIED Requirements

### Requirement: 健康就绪判定（核心路由硬条件）

系统 SHALL 以「文章入库前置拦截」口径判定整体「健康」：≥1 条 embedding 路由（enabled 且绑定了启用 provider）的主 provider 可达，且 `topic_tagging` 路由的主 provider 可达。两条任一不满足 SHALL 判定 NOT 健康。`topic_tagging` 路由缺失、未绑定任何启用 provider、或其主 provider 不可达时，SHALL 判定 NOT 健康（fail-closed——打标不可用即文章无法入库）。核心双路由之外的其他路由（summary / digest_polish / data_enrichment_news / data_enrichment_analysis / feed_discovery / open_notebook 等）SHALL NOT 参与健康判定：单条非核心路由不可达 SHALL NOT 单独导致 NOT 健康，其可达性明细仍照常展示在健康快照中。判定口径之外的探测机制（只探主 provider、启动自动拉起、心跳复检、连续失败去抖、启动竞态 fail-closed、快照与查询 API 结构）保持不变。

#### Scenario: 两类各通一条即健康

> 「两类」自本需求起指**核心两类**（embedding、topic_tagging），非旧口径的「embedding 类 + 任意 llm 类」。

- **GIVEN** embedding 路由主 provider 可达，topic_tagging 路由主 provider 可达，digest_polish 路由主 provider 不可达
- **WHEN** 健康判定计算
- **THEN** 系统 SHALL 判定 healthy=true（digest_polish 不通不推翻整体，其明细仍展示在快照）

#### Scenario: 缺 embedding 则不健康

- **WHEN** 所有 embedding 路由主 provider 均不可达，但 topic_tagging 路由主 provider 可达
- **THEN** 系统 SHALL 判定 healthy=false

#### Scenario: 缺 llm 则不健康

> 旧口径「所有 llm 路由不可达才不健康」收紧为：**打标这一条 llm 路由不可达即不健康**。

- **GIVEN** embedding 路由主 provider 可达，topic_tagging 路由主 provider 不可达
- **WHEN** 健康判定计算
- **THEN** 系统 SHALL 判定 healthy=false，其他 LLM 路由（含云端）可达不救命

#### Scenario: 云端 LLM 可达不救命（2026-09-28 回归）

- **GIVEN** topic_tagging 路由主 provider（本地 qwen）不可达，digest_polish 与 data_enrichment_analysis 的云端主 provider 均可达，embedding 路由主 provider 可达
- **WHEN** 健康判定计算
- **THEN** 系统 SHALL 判定 healthy=false——锁定「单开 embedding + 云端 deepseek 活着不能让门开」的实测回归

#### Scenario: 打标路由未配置不健康

- **GIVEN** 系统无 enabled 的 topic_tagging 路由，或该路由未绑定任何启用的 provider，但 embedding 路由可达
- **WHEN** 健康判定计算
- **THEN** 系统 SHALL 判定 healthy=false

#### Scenario: 未配置任何路由不健康

- **WHEN** 系统无任何 enabled 且绑定了 provider 的路由
- **THEN** 系统 SHALL 判定 healthy=false
