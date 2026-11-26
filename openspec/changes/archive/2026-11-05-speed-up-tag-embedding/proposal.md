<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, content-enrichment -->

# Proposal: speed-up-tag-embedding

## Why

用户反馈「给标签生成 embedding 向量太慢」。经生产库实测（详见 `docs/research/tag-embedding-slow/explore-findings.md`），瓶颈不是 provider（响应 353ms），而是自家代码四层叠加：

1. **FindSimilarTags 无可用 ANN 索引**：2560 维超过 pgvector float32 HNSW 的 2000 维上限（当年建索引失败，库里留有坏索引尸体），每次 L2 相似查询全表 seq scan **1366ms**、940MB buffer 流量/次，高峰 1900 次/小时，把 Postgres CPU 打到 116%，全局拖慢所有查询（队列 UPDATE 实测 3093ms）
2. **embedding 队列 worker 单 goroutine + 2s 轮询**：吞吐钉死 0.37 任务/秒，38% 时间在空转。1000 标签 ≈ 45 分钟
3. **API 逐条调用**：每请求只嵌 1 条文本，每标签串行 2 次（identity + semantic）；底层 client 原生支持批量且 auxlabel 路径已用 batch=32，唯独队列路径没用
4. **杂项**：单任务 ~14 次 DB 往返（含每次查 pg_catalog）、worker 日志被 `zap.NewNop()` 吞掉、失败无自动重试

方案 1（halfvec HNSW + 两段式查询）已完成生产库实测验证：**查询 1366ms → 9.3ms（146×）**，索引构建 54 秒（shm_size 1g + 512MB 并行，docker-compose 已改），召回口径 5 探针全中。详见验证结论（research 库）。

## What Changes

**核心（性能修复）**：
1. 给 `topic_tag_embeddings` 的 semantic / identity / event_keyword 三类向量建 partial halfvec HNSW 索引（`embedding::halfvec(2560)`，typmod 必须精确匹配）；删除坏索引尸体 `tmp_hnsw_test`
2. `FindSimilarTags` 改两段式：halfvec HNSW 索引召回候选（`::halfvec(2560)` + `hnsw.ef_search=120`）→ 候选集内 float32 `::vector` 精确重排。cosine 校验口径不变
3. embedding 队列 worker 去 2s 轮询空转（处理完立即取下一条）+ 有限并发（N=4）
4. 队列任务批量 embedding：同类型标签合并批量调 provider（batch=32，对齐 auxlabel 路径）

**顺带（可观测性与稳定性，小改动）**：
5. worker 换真实 logger（替换 `zap.NewNop()`），status API 增加 `oldest_pending_age_seconds` / `throughput_per_min`
6. 失败自动重试（retry_count < 3 时延迟回 pending）+ 消费 `ProviderError.Retryable` 做一次退避重试
7. 修 `Stop()`/`Start()` 的 stopCh 竞态（旧 worker 不退出）

**不做**（明确出界）：MRL 降维、外接向量库、健康门禁分级（analysispause）、merge 队列 SKIP LOCKED、aux 回填批量化——留后续 change。

## Capabilities

### Modified
- `tag-embedding-management`：相似查询走索引两段式；队列 worker 并发/批量/重试/可观测
- `embedding-cache`：批量调用下缓存 key 拆分口径

## Impact

- 预期效果：1000 标签嵌入 45min → ~5min；DB CPU 从 116% 回落；inline 打标同步路径同样受益
- 部署操作：需在低负载窗口建索引（54s/类，CONCURRENTLY）；`docker-compose.pg.yml` 的 `shm_size: 1g` 已生效
- 旧数据：无迁移、无降级，向量与阈值（0.92/0.95/0.8）全部不变
