## ADDED Requirements

### Requirement: 问答联网状态可见性

页边注问答每次调用 SHALL 采集联网搜索状态 `web_search_status` 并持久化到问答轮（`annotation_qas.web_search_status`）：`disabled`（SearXNG 未配置或未启用）｜`ok`（搜索链路启用且有结果进入参考池）｜`empty`（链路启用但全部搜索失败或结果为空）。该字段 SHALL 随 QA 响应一并返回（加法字段，旧消费方不受影响）。旧行（列值为 NULL）SHALL 视为历史数据，不显示任何联网标记。

批注卡问答轮 WHEN `web_search_status=empty` SHALL 显示「联网搜索无结果」弱化标记（含 title 提示前往设置面板排查）；`ok`/`disabled`/NULL SHALL 不显示标记。标记为纯展示，SHALL NOT 影响回答内容、引用校验与静默降级行为（「搜索失败静默降级」既有语义不变）。

#### Scenario: 联网失效时回答带降级标记

- **WHEN** SearXNG 启用但其上游引擎全部超时，读者完成一次提问
- **THEN** 问答轮带 `web_search_status=empty` 落库并在批注卡显示「联网搜索无结果」标记，回答正常返回

#### Scenario: 未启用联网时不显示标记

- **WHEN** searxng_config 未配置或 enabled=false 时完成一次提问

- **THEN** 问答轮 `web_search_status=disabled` 落库，批注卡不显示任何联网标记

#### Scenario: 联网正常时不显示标记

- **WHEN** 搜索链路启用且有结果进入参考池（无论回答是否引用）
- **THEN** 问答轮 `web_search_status=ok` 落库，批注卡不显示联网标记

#### Scenario: 历史问答不显示标记

- **WHEN** 打开本 change 上线前创建的日报批注
- **THEN** 旧问答轮（web_search_status 为 NULL）不显示联网标记
