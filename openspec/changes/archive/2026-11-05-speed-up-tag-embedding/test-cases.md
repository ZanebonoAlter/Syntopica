# Test Cases: speed-up-tag-embedding

> 复杂度 complex（design.md 已声明）。测试单元 = 「标签向量相似查找与队列吞吐恢复」用户故事，主链路表串节拍，变体走查 + 效果核对 + 白盒附加见后。

## 主链路表（故事：给标签查相似/生成向量不再被全表扫描和空转拖死）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 索引存在性：三类 embedding 各有 partial halfvec HNSW，tmp_hnsw_test 已删 | 索引覆盖三类 | pg_index 查询返回 3 个 valid 索引、无 tmp_hnsw_test | 集成（testcontainer PG） | `repository/embedding_index_test.go` |
| 2 | 20 个真实向量探针做相似查询 | top10 与精确排序一致 | overlap=10/10 × 20；EXPLAIN 为 Index Scan；p95 < 50ms | 集成（真实 PG + pgvector） | `service/core/embedding_index_recall_test.go` |
| 3 | cast 带 typmod（`::halfvec(2560)`） | 查询 cast 必须带 typmod | grep 代码无裸 `::halfvec`（不带维度）的距离排序 SQL；EXPLAIN 非 Index Scan 判定失败 | 集成 | 同上 |
| 4 | 队列 100 任务、provider 正常 | 连续处理无空转 | 任务间隔中位数 < 单次往返 + 1s；同时 processing ≤ 4 | 集成（PG + fake provider） | `service/core/embedding_queue_concurrency_test.go` |
| 5 | 批量调用：10 任务（identity+semantic 各 10） | 批量调用 | provider 收到 ≤2 次批量请求（每类型 ≤1 次）而非 20 次单条 | 集成（fake provider 计数） | `service/core/embedding_queue_batch_test.go` |
| 6 | 批内 3 条已有缓存 | 批量下缓存按单条命中 | 3 条不进 provider 请求；miss 条目按单条 key 写缓存 | 集成 | `platform/airouter/embed_batch_cache_test.go` |
| 7 | 单条路径缓存兼容 | 单条路径 key 兼容 | 预置历史 key 缓存行 → inline 调用命中不调 provider | 集成 | 同上 |
| 8 | 任务失败 retry_count=1 | 失败自动重试 | next_retry_at 到期后回 pending 被再处理 | 单测（时间可控 fake） | `service/core/embedding_queue_retry_test.go` |
| 9 | 任务第 4 次失败 | 重试上限 | 终态 failed，仅手动 retry 接口可回 | 单测 | 同上 |
| 10 | status API | status 新增字段 | 响应含 oldest_pending_age_seconds / throughput_per_min，值与库内一致 | handler 测试 | `handler/embedding_queue_status_test.go` |
| 11 | Stop 后再 Start | worker 重启不残留 | goroutine 计数回到期望，无多 worker 抢队列（用计数 hook 断言） | 单测 | `service/core/embedding_queue_lifecycle_test.go` |

## 继承与调整（⓪ 契约盘点）

改动契约：tag-embedding-management 的相似查询实现方式（对外结果口径不变）、embedding-cache 的批量 key 口径。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| SaveEmbedding 清理同 tag/type 旧记录 | 不变 | `service/core/embedding_test.go` | 保留全跑 |
| embedding 测试使用真实 Postgres + pgvector | 不变 | 同上 | 保留 |
| embedding cache 单条查/写（embedding-cache 既有） | 行为不变、key 口径加批量条目 | `platform/airouter` 既有缓存测试 | 保留 + 增批量用例（步 6/7） |
| queue status 既有字段（pending/processing/...） | 不变（只增字段） | `handler/tag_queue_status_test.go` | 保留 + 增字段断言（步 10） |

`bash scripts/harness/test-assets.sh tag-embedding-management` 反查旧资产清单，归档前逐行核对。

## 变体走查（五组）

- **输入**：相似查询空候选（索引无该类型向量）→ 返回空集不报错；批量批内含空文本 → 跳过该条不中断批。其余输入变体（空白/分隔符/大小写）在本层不适用（文本已在生成端归一），划除。
- **前置**：空队列 → 200ms 退避轮询不空转打 DB（断言 poll 次数）；单任务队列；重复 Enqueue 同 tag（幂等：唯一索引兜底，不重复处理）。越界引用不适用，划除。
- **时间窗口**：next_retry_at 边界两端（恰好到期→可 claim；差 1ms→不可）；throughput_per_min 空窗口（60s 内无完成→0）；跨分钟归一化。当天口径不适用（无 completed_today 改动），划除。
- **幂等**：重复执行批量落库（唯一索引兜底 + upsert）；部分失败重试（批内 3 条失败→只重试该 3 条）；并发 claim（SKIP LOCKED 下两 worker 不领同一行，集成测试双连接断言）。
- **可用性（API 消费方）**：status 新字段在 pending=0 时 oldest_pending_age_seconds=null（前端空态不炸）；provider 全挂 → 任务全部进重试轨道而非崩溃；加载态/超长文本不适用（非 UI），划除。

## 效果核对（问题 4）

- 触发原因：相似查询全表扫描（1366ms/次、DB CPU 116%）+ 队列 0.37 任务/s
- 方法：3.2 步真库量化——改造前后各灌 100 队列任务测吞吐；EXPLAIN + pg_stat_user_tables.idx_scan 确认索引被用；DB CPU 对比
- 量化结果（目标）：吞吐 ≥ 2 任务/s（≥5×）；FindSimilarTags p95 < 50ms；idx_scan 增长、seq_scan 停增
- 结论写入归档前 verify 报告

## 白盒附加（分支表 + 边界值）

| 分支 | 边界值 | 用例落点 |
|---|---|---|
| 批量切批：任务数 0/1/31/32/33/64 | 32 上限、33 触发第二批 | 步 5 扩展变体 |
| 重试退避：retry_count 0/1/2/3 | 3 为终态边界；退避 30s/60s/120s | 步 8/9 |
| 并发 claim：worker 4、任务 1/4/5 | 5>4 时 1 个 worker 空转等待 | 步 4 |
| ef_search 候选截断：LIMIT 50 与 ef=120 | 候选 < 50（小表）时全量精排 | 步 2 小表变体 |
| 批量缓存部分命中：0/1/N-1/N 条 hit | 全 miss / 全 hit 边界 | 步 6 变体 |
| typmod 失配回退：无索引环境（testcontainer 未建索引） | 查询仍正确（走 seq scan 慢但不错） | 步 2 前置环境断言 |

不适用划除：UI 状态走查（无 UI）、时间窗口「当天算不算」（不改该口径）。
