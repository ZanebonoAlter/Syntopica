# persistent-topic — 质心首义锚定 delta

## MODIFIED Requirements

### Requirement: PersistentTopic 质心表示

PersistentTopic 的 embedding 匹配锚点 SHALL 为「首义向量与滚动质心的加权混合」：`centroid = α·首义 embedding + (1-α)·最近 centroid_window 条命中 section embedding 等权均值`。α = `persistent_topic_centroid_anchor_weight`（默认 0.5，embedding_config 可配，见配置项）。manual 话题的首义为创建时所选 section 的均值向量，auto 话题的首义为首条命中 section 的内容 embedding——两者均持久化于 `board_persistent_topics.embedding`，创建后不可变。

质心 SHALL 物化存储于 `board_persistent_topics.centroid` 列（与 embedding 同维度），section 新增或归属变更时增量更新。漂移 SHALL 有界：质心随话题下 section 的实际内容自适应移动（保留内容漂移打破标题回声闭环的能力），但首义以 α 权重持续锚定，滑出窗口的历史 section 不再使锚点脱离话题创建定义——防止宽口径话题质心漂移为「泛新闻磁铁」。

首义 embedding 与滚动均值维度不一致、或首义为空/不可解析时，质心 SHALL 回退为纯滚动均值（记警告日志，不阻塞刷新）。历史 section 数 <2 的 topic，质心退化为首义向量（等价于现有 embedding 列语义）。

系统 SHALL 提供存量质心重算入口（`POST /api/daily-reports/recompute-topic-centroids`，可选 board_id 过滤，异步执行、幂等可重跑），按当前公式与配置重算既有话题质心；用于公式升级后的一次性存量纠正。

section embedding SHALL 基于 section 实际聚合内容生成（见 section-content-embedding 能力）。

#### Scenario: 质心按近期 section 计算

- **GIVEN** topic「Claude Code 工具链崛起」有历史 section 40 条，首义 embedding 为 E0，最近 30 条 section 均值为 M，α=0.5
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL = 0.5·E0 + 0.5·M（按元素加权），centroid_window 之外的 section 不参与 M

#### Scenario: 质心随内容漂移

- **GIVEN** topic「美军机从以色列境内基地起飞对伊朗实施打击」质心原锚定首义方向，连续两日被归因了「阿联酋与伊朗贸易」相关 section（内容 embedding 与首义语义距离约 0.24），α=0.5
- **WHEN** 质心重算
- **THEN** 新质心 SHALL 向阿联酋贸易内容方向移动（内容漂移仍生效、标题回声闭环仍被打破），但移动幅度 SHALL 不超过纯滚动均值位移的 (1-α)，质心到首义的距离 SHALL 保持有界

#### Scenario: section 不足退化首义向量

- **GIVEN** topic 只有 1 条历史 section
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL 等于该 section 的内容 embedding（首义向量语义）

#### Scenario: 质心增量更新

- **GIVEN** 新 section 归属 topic「X」
- **WHEN** section 持久化
- **THEN** topic X 的 centroid SHALL 增量重算（新公式混合，纳入新 section，窗口溢出的旧 section 淘汰）

#### Scenario: 首义不可用时回退纯滚动

- **GIVEN** topic 的 embedding 列为 NULL 或与滚动均值维度不一致，有 5 条可解析历史 section
- **WHEN** 系统计算其质心
- **THEN** centroid SHALL 等于滚动窗口均值（无首义混合），并 SHALL 记录警告日志

#### Scenario: 锚定权重配置边界

- **WHEN** embedding_config 设置 persistent_topic_centroid_anchor_weight=0（或 1）
- **THEN** 质心 SHALL 分别退化为纯滚动均值（旧行为）/ 纯首义（话题冻结）

#### Scenario: 存量质心重算

- **GIVEN** 部署混合公式后，存量话题质心仍为旧公式（纯滚动）产物
- **WHEN** 调用 POST /api/daily-reports/recompute-topic-centroids
- **THEN** 全部（或指定 board）话题质心 SHALL 按新公式与当前配置重算；重复调用 SHALL 幂等（同输入同输出）；单个话题重算失败 SHALL 不中断其余话题

### Requirement: 配置项

系统 SHALL 在 embedding_config 新增/维护以下配置项（含默认值）：
- `persistent_topic_match_threshold`：0.30（section 命中 topic 的 embedding distance 上限）
- `persistent_topic_upgrade_threshold`：3（candidate 连续命中天数转 active）
- `persistent_topic_decay_window`：30（active 无命中转 archived 的天数）
- `persistent_topic_cluster_threshold`：0.30（回刷时历史 section 聚类成 topic 的阈值）
- `persistent_topic_centroid_anchor_weight`：0.5（质心混合的首义权重 α；0=纯滚动旧行为，1=纯首义冻结；非法值回退默认并告警）

#### Scenario: 读取默认配置

- **WHEN** embedding_config 未设置上述 key
- **THEN** 系统 SHALL 使用默认值

#### Scenario: 自定义升级阈值

- **WHEN** embedding_config 设置 persistent_topic_upgrade_threshold=5
- **THEN** candidate 连续命中 5 天才转 active

#### Scenario: 自定义锚定权重

- **WHEN** embedding_config 设置 persistent_topic_centroid_anchor_weight=0.3
- **THEN** 质心混合 SHALL 采用 α=0.3（首义 30% + 滚动均值 70%）
