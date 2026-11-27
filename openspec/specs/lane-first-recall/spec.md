# lane-first-recall Specification

## Purpose

日报候选收集在「版块挂载通道」之外增加一条**泳道召回通道**：以每条 active 泳道的 centroid 为锚，取当日 event 标签中距离最近且落在阈值内的一小簇（默认每泳道 8 条、余弦距离 < 0.16），与版块通道取并集。
解决的问题是「新闻确实存在、但既没挂到版块、又在质量分区里被整批丢弃」——泳道侧只问「像不像这条线」（窄），版块侧问「属不属于科技」（宽），两者互补。
边界：召回是**单日行为**，不写挂载表（挂载是永久事实）；被本板 `direction_mismatch` 或跨版块排他排除的标签不进；L2 判 `new` 的召回标签直接丢弃、不据此新建持久话题；当天没有专属新闻的泳道保持空白，不放宽距离硬拉邻座内容。

## Requirements

### Requirement: 日报候选收集包含泳道召回通道

日报生成的候选标签收集 SHALL 包含两条召回通道，取并集：

- **版块通道**（既有）：已挂载到本版块、`direction_mismatch=false`、`status=active`、`category=event`、当日有文章的标签。
- **泳道通道**（本 change 新增）：对每条 `status=active` 且 `embedding IS NOT NULL` 的泳道，在**当日全量 event 标签池**（不限是否已挂载到本版块）中按语义向量召回距离最近的至多 `lane_recall_top_k` 个标签，且距离 SHALL 小于 `lane_recall_max_distance`。

泳道通道 SHALL NOT 写入 `topic_tag_board_labels`（挂载记录是永久事实，召回是单日行为，两者不得混同）。

#### Scenario: 新事件标签经泳道通道进入候选集

- **GIVEN** 某版块存在 active 泳道 L，某个 `weighted`（仅 1 篇文章支撑）的新事件标签 T 未挂载到该版块
- **AND** T 到 L 质心的余弦距离小于 `lane_recall_max_distance`，且 T 在 L 的 Top-`lane_recall_top_k` 之内
- **WHEN** 生成该版块当日日报
- **THEN** T SHALL 出现在候选集中，且 SHALL NOT 新增 `topic_tag_board_labels` 行

#### Scenario: 当天没有专属新闻的泳道保持空白

- **GIVEN** 某 active 泳道 L 在当日全量 event 标签池中最近的标签距离 `>= lane_recall_max_distance`
- **WHEN** 生成当日日报
- **THEN** 泳道通道 SHALL NOT 为该泳道召回任何标签（SHALL NOT 硬拉邻座内容凑数）
- **AND** 该泳道当日 SHALL 保持无 section

#### Scenario: 召回参数可在线覆盖

- **WHEN** `ai_settings` 中 `lane_recall_top_k` 或 `lane_recall_max_distance` 存在合法值
- **THEN** 泳道通道 SHALL 使用该值；缺失或非法时 SHALL 回退默认值（`8` / `0.16`），且 SHALL NOT 中断日报生成

#### Scenario: 泳道向量缺失时跳过该泳道

- **GIVEN** 某 active 泳道的 `embedding IS NULL` 或其向量维度与标签向量空间不符
- **WHEN** 执行泳道通道召回
- **THEN** 系统 SHALL 跳过该泳道并记录日志，SHALL NOT 中断日报生成

### Requirement: 泳道证据替代版块统计证据通过质量分级

经泳道通道召回的标签 SHALL 视为已具备准入证据，SHALL NOT 参与 `filterTagsByQuality` 的「`kept >= 10` 则整批丢弃 `weighted`」判定。

理由：该二元开关回答的是「该标签能否证明自己属于这个版块」，用的是统计证据（命中篇数 / 命中率 / 重叠占比），单篇文章产出的新事件在结构上永远无法满足；而泳道通道回答的是更强的问题——「该标签离某条已成立泳道足够近」，用的是语义证据。

泳道通道召回的标签如果同时属于 `weighted`，SHALL 原样进入后续去重与 L1/L2/L3 聚类，SHALL NOT 因其 `weighted` 身份被丢弃。

#### Scenario: 高质量标签充足时新事件仍能入场

- **GIVEN** 当日已挂载质量标签（`composite_hit` / `direct_hit` / `hit_rate` / `max_sim`）数量 `>= 10`
- **AND** 有若干 `weighted` 新事件标签经泳道通道被召回
- **WHEN** 执行质量分级
- **THEN** 这些经泳道召回的 `weighted` 标签 SHALL 保留；未经泳道召回的其他 `weighted` 标签 SHALL 按既有规则丢弃

### Requirement: 泳道召回的标签 L2 判 new 时不得进入 L3

经泳道通道召回的标签，若 L2 裁决结果为 `new`（不归属任何既有话题，含「switch 集外降级」情形），系统 SHALL 直接丢弃该标签，SHALL NOT 将其送入 L3 新叙事分组建新泳道。

理由：L3 建新泳道的准入证据应当是版块级统计证据。经泳道通道进来的标签只有「离某泳道较近」这一条弱证据，若允许其自建新泳道，等于让第二道门变成新的污染入口（2026-10-04 非农事故的同类风险）。

L2 调用失败时的既有行为 SHALL 保持不变：`ClusterTagsLane` 返回错误、**该版块该轮生成失败**（不落半成品报告），SHALL NOT 退化为「整批降 L3 自建新泳道」。本要求只约束 L2 **成功返回 `new`** 的情形。

#### Scenario: 泳道召回标签被 L2 判 new 时丢弃

- **GIVEN** 标签 T 仅经泳道通道进入候选集
- **WHEN** L2 裁决 T 为 `new`
- **THEN** T SHALL 被丢弃，SHALL NOT 出现在 L3 分组中，SHALL NOT 成为新泳道的触发标签

#### Scenario: L2 调用失败时该版块生成失败且不降级自建泳道

- **GIVEN** 候选集中同时存在版块通道与泳道通道召回标签
- **WHEN** L2 LLM 调用失败
- **THEN** 系统 SHALL 返回错误、该版块该轮 SHALL NOT 落库报告（既有行为，保持不变）
- **AND** 系统 SHALL NOT 退化为「整批降 L3」、SHALL NOT 借泳道召回标签自建新泳道
