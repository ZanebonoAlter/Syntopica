## ADDED Requirements

### Requirement: 日报同事件聚簇

日报生成时，系统 SHALL 在**同一版块当天**的 event 标签范围内做同事件聚簇，作为「今日重点」的候选源。聚簇 SHALL 复用 related-events 的事件语义表示 `related-event-v1`（名称 + 描述 + 当前有效辅助标签）与同一相似度阈值 `related_event_similarity_threshold`（默认 0.86，有效值 `[0,1]`，缺项回退默认、坏值告警回退、非法提交拒绝且不覆盖），SHALL NOT 为聚簇另立表示或另立阈值。

聚簇 SHALL 为**当天口径**：候选集 SHALL 为该版块当天日报中命中的 event 标签（去重后），SHALL NOT 跨天、不引入前一日标签，SHALL NOT 做事件身份归并、合并、传递扩展或阶段判定。聚簇结果 SHALL 只在内存中计算，SHALL NOT 写入 `related_event_embeddings` 或任何业务表、缓存、身份表与关系表。

聚簇 SHALL 产生向量时走标准 AI 路由（capability 为 embedding），请求 SHALL 携带非空 operation（与索引刷新 operation 区分），每次调用尝试无论成败 SHALL 落 `ai_call_logs` 审计（operation / capability / token / latency / metadata；embedding 路径按仓内 N/A 惯例不记 prompt / response_snippet，截断保护只适用 chat 路径）。同一版块同一天的向量 SHALL 以批量调用取得（分批上限与单批超时沿用既有索引刷新口径），SHALL NOT 逐标签单独调用，也 SHALL NOT 为同一批标签重复计算。

每个聚簇 SHALL 给出：成员 event 标签集合、簇内文章数（按内容身份去重，见 `lane-heat-metric`「线索计数的内容身份去重口径」）、代表标签与描述。综合稿（`scope=composite`）的 thread SHALL NOT 进入聚簇，也不 SHALL NOT 计入簇内文章数。

向量不可得时，系统 SHALL 按「语义向量兜底 → 主体近似兜底」逐级降级，并 SHALL 如实标注本次聚簇口径（向量聚簇 / 语义向量兜底 / 主体近似），SHALL NOT 把不同口径的结果混为一谈，也 SHALL NOT 静默把降级结果呈现为向量聚簇。

#### Scenario: 同版块当天同事件聚成一簇

- **WHEN** 某版块当天的两个 event 标签语义相似度达到实际阈值
- **THEN** 二者 SHALL 归入同一聚簇
- **AND** 该簇 SHALL 给出成员标签集合与按内容身份去重后的簇内文章数

#### Scenario: 聚簇不跨天不做身份

- **WHEN** 某版块昨天的 event 标签与今天的某个标签语义高度相似
- **THEN** 昨天的标签 SHALL NOT 进入今天的候选集与聚簇
- **AND** 聚簇 SHALL NOT 产生事件身份、合并关系或阶段标记

#### Scenario: 聚簇不落库不改索引

- **WHEN** 日报生成完成聚簇
- **THEN** `related_event_embeddings` 与任何业务表 SHALL 保持写入前状态
- **AND** 既有 related-events 查询结果 SHALL 不受影响

#### Scenario: 批量调用并留审计

- **WHEN** 某版块当天有 N 个候选 event 标签
- **THEN** 聚簇 SHALL 以批量 embedding 调用取得向量（分批上限与单批超时沿用既有索引刷新口径），SHALL NOT 逐标签单独调用
- **AND** 每次调用 SHALL 携带非空 operation 并在 `ai_call_logs` 留下成功或失败记录

#### Scenario: 阈值在线可调且与相关事件同源

- **WHEN** 把 `related_event_similarity_threshold` 从 0.86 调到 0.9
- **THEN** 次日日报的聚簇 SHALL 使用 0.9 过滤
- **AND** 该键 SHALL 与相关事件查询共用同一生效值，不另设独立阈值

#### Scenario: 向量不可得时如实降级

- **WHEN** 聚簇所需的 embedding 调用失败或无可用路由
- **THEN** 系统 SHALL 降级到语义向量兜底，再不可得时降级到主体近似
- **AND** 本次聚簇 SHALL 标注实际口径，SHALL NOT 呈现为向量聚簇

#### Scenario: 综合稿不进聚簇

- **WHEN** 某 thread 的 `scope=composite`（命中 2 个及以上 event 标签）
- **THEN** 该 thread SHALL NOT 进入任何聚簇
- **AND** SHALL NOT 计入任何簇的簇内文章数
