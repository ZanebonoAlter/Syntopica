## ADDED Requirements

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
