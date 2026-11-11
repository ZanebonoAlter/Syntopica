# Design：anchor-topic-centroid-with-embedding

## Context

- 质心刷新链路：日报保存/改挂/rebucket → `UpdateCentroidOnSectionChange`（`daily_report_topic_repository.go:957`）→ `ComputeTopicCentroid`（:898，最近 `CentroidWindow=30` 条命中 section 等权均值）→ 写 `board_persistent_topics.centroid`。
- 匹配锚点：`topicAnchorVec`（centroid 优先，无则首义 embedding）；lane 分桶与 `planTopicAssignments` 只消费锚点，**不感知质心怎么算**——改公式不需要动 lane 层。
- 首义向量：manual 话题 = 创建时所选 section 均值（`CreateManualTopic`）；auto 话题 = 首条命中 section 的内容 embedding（L3 新建 candidate 时写入）。两者都存 `embedding` 列，创建后不可变。
- 配置：`LoadPersistentTopicConfig` 读 `embedding_config` 表，switch-case 解析 `persistent_topic_*` key。

## 决策

### D1 混合公式与归一化（不额外归一化）

```
centroid = α · embedding首义 + (1-α) · mean(最近 centroid_window 条命中 section embedding)
```

- 匹配全走 cosine 距离，只看方向不看模长 → 混合后**不做 L2 归一化**，少一步、少一个出错面。
- α（首义占比）= `persistent_topic_centroid_anchor_weight`，默认 0.5。
- **维度护栏**：首义与滚动均值维度不一致（换 embedding 模型后未重算首义的存量场景）→ 跳过混合、回退纯滚动均值并 `logging.Warnf`，不阻塞日报。均值侧沿用 `meanPgVectors` 现状（截断到最短维度）。
- **首义为 NULL / 不可解析**：同上回退纯滚动。auto 话题全部有 embedding，manual 也有（创建时强制），此路径主要为防御。

### D2 <2 条 section 的退化路径不变

`ComputeTopicCentroid` 现有「可解析 section <2 → 返回首义」保持原样（α=1 的自然特例，无信息可滚动时不引入零向量）。

### D3 新配置项

`PersistentTopicConfig` 增 `CentroidAnchorWeight float64`（默认 0.5）。`LoadPersistentTopicConfig` 增 case `persistent_topic_centroid_anchor_weight`：ParseFloat，值域 [0,1] 外或解析失败 → 回退默认 + Warnf（与 `centroid_window` 惯例一致）。存量真库 `embedding_config` 只有 2 个无关 key，无迁移。

### D4 存量重算入口：HTTP 端点，不做启动迁移

- 新端点 `POST /api/daily-reports/recompute-topic-centroids?board_id=<可选>`，异步 goroutine（30min timeout，沿 `triggerBackfillEmbeddings` 的 cancel-in-goroutine 模式），遍历目标话题逐个 `UpdateCentroidOnSectionChange`，逐话题 error 记日志不中断。
- 选端点而非启动迁移：①与 backfill-* 端点族先例一致；②幂等可重跑、可按板灰度；③不给每次重启加无谓开销。**部署后手动调用一次**（全局，无 board_id）即完成存量纠正，写入部署影响汇报。
- 不做 dry_run/模拟模式：真库重算前用只读 SQL 记录基线距离（验证节有配方），风险面 = 纯函数重算、可再次调用旧数据无关回滚（质心只影响后续匹配，不删数据）。

### D5 明确不做

- 不改 `lane_l1_threshold`/`lane_l2_threshold`（错挂的主观感受问题先靠锚定缓解，阈值另行观察）。
- 不改首义 embedding 生成/更新（"重置首义"运营操作留后续 change）。
- 不动 watch 物化话题（watch-materialized-topic）的首义语义——它们同样走 `ComputeTopicCentroid`，自动获得新公式。

## 影响面

| 调用点 | 变化 |
| --- | --- |
| `daily_report_repository.go:401/1265`（日报保存/改挂后刷新） | 自动走新公式，无代码改动 |
| `daily_report_rebucket.go:111`（重分桶刷新） | 同上 |
| `daily_report_backfill_topics.go`（回刷重建） | 新建话题首条 section → 走 D2 退化路径，行为不变 |
| `daily_report_assignment.go` / `daily_report_lane.go` 锚点选择 | 不改（centroid 优先逻辑不变） |

## 测试策略（详见 test-cases.md）

- 混合纯函数抽 `blendCentroidWithAnchor(mean, anchor []float64, alpha float64) []float64`：函数单测覆盖 α∈{0, 0.5, 1}、维度不匹配、空首义。
- `ComputeTopicCentroid`：testcontainer PG 集成测试（既有 `daily_report_centroid_test.go` 改造断言）。
- recompute 端点：handler 测试（参数校验 + 异步触发不阻塞）。
- 效果核对：真库只读——α=0.5 模拟混合质心下，陈杰寻人 tag（268293）到 1009 的距离预期 > 0.18（降级 L2）；重算后实测复核。
