## ADDED Requirements

### Requirement: section 展示标题内容化

当日志报 section 的展示标题（`cluster_label`）SHALL 由该 section 当天实际聚合的内容派生（所聚标签事实 + 代表文章），SHALL NOT 默认取所挂持久话题的 label 作为展示标题——话题 label 仅承担归属锚与兜底职责。

标题生成 SHALL 遵守日报文案生成事实锚约束（禁编造事件/数字/情绪/因果，信息不足时降级兜底而非凑数），随 `promptVersion` 升级生效。

话题归属信号（`persistent_topic_id` / `lane_tier` / `topic_match_confidence` / `topic_match_distance`）SHALL 不因标题来源改变而变化。

#### Scenario: 命中既有话题的 section 标题反映当天内容

- **GIVEN** 某 active 话题 label 为「日本首相高市早苗宣布不于7月释放石油储备」（创建于 7 月的旧事件）
- **WHEN** 8 月某日该话题命中的 section 实际聚合了当天"执政基础不稳"相关的标签与文章
- **THEN** 该 section 的 `cluster_label` SHALL 为基于当天标签事实拟定的标题（如「高市执政基础不稳引发党内反弹担忧」）
- **AND** SHALL NOT 为话题 label「日本首相高市早苗宣布不于7月释放石油储备」

#### Scenario: 标题生成失败时降级兜底

- **GIVEN** 某 section 命中既有话题，但当日标题生成 LLM 调用失败或返回不可用结果
- **THEN** `cluster_label` SHALL 按兜底链取值：当日代表 thread 标题 → 话题 label（或 L3 场景下的分组名）
- **AND** SHALL NOT 出现空标题

#### Scenario: L3 新话题标题行为不变

- **GIVEN** 某 section 走 L3 泳道新建话题
- **WHEN** section 持久化
- **THEN** `cluster_label` SHALL 为当天 LLM 命名的分组名（现有行为），`lane_tier` 为 l3_new

#### Scenario: 话题归属字段不受标题影响

- **GIVEN** 某 section 以 l1_direct 挂上 active 话题且标题已内容化
- **WHEN** section 持久化
- **THEN** `persistent_topic_id`、`lane_tier=l1_direct`、`topic_match_confidence=anchor_hit`、`topic_match_distance` SHALL 与标题来源无关地照常记录

#### Scenario: 标题遵守事实锚约束

- **GIVEN** 某 section 所聚标签仅有公司名与行业名，无任何涨跌数字或情绪词
- **WHEN** 生成该 section 标题
- **THEN** 标题 SHALL NOT 出现未由标签事实支撑的数字、情绪或因果表述

### Requirement: 历史与跨天可读性边界

历史 section 的 `cluster_label` SHALL NOT 回刷——变更生效前的旧 section 保留原标题（话题名复读期），生效后的新 section 走内容化标题，形成自然分界。

前端时间线展示同话题连续演进时，SHALL 继续以话题归属（`persistent_topic_id`）串联跨天 section，标题仅表达当天内容，两者职责分离不互相替代。

#### Scenario: 历史数据不回刷

- **GIVEN** 变更生效前已存在的 section 其 `cluster_label` 为旧话题名
- **WHEN** 变更部署完成
- **THEN** 该 section 的 `cluster_label` SHALL 保持不变

#### Scenario: 时间线跨天串联不依赖标题一致

- **GIVEN** 同一话题连续三天的三个 section 标题各不相同（每天内容不同）
- **WHEN** 用户在时间线查看该话题的演进
- **THEN** 三个 section SHALL 通过相同 `persistent_topic_id` 归并为同一话题链
