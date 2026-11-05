# Tag Embedding Management — Delta

## ADDED Requirements

### Requirement: 标签相似查询使用 halfvec ANN 两段式

系统在按 embedding 相似度查找标签（L2 相似命中路径）时，SHALL 先用 halfvec HNSW 索引召回候选集（候选池 ≥ 50、`hnsw.ef_search ≥ 120`），再在候选集内用 float32 向量距离精确重排后返回结果。两段式的最终排序口径 SHALL 与全量 float32 精确排序一致（top10 召回重合度 = 10/10）。

匹配阈值（MatchThreshold=0.92）、cosine 校验、L1/L2/L3 三级去重语义均不变。

#### Scenario: 相似查询走索引且结果与精确排序一致
- **WHEN** semantic 类向量 ≥ 2 万条且存在 halfvec HNSW partial 索引，随机取 20 个真实标签向量做相似查询
- **THEN** 每个查询的 top10 与全量 float32 精确排序 top10 完全一致（overlap=10/10），且单次查询 p95 < 50ms（EXPLAIN 确认 Index Scan）

#### Scenario: 查询 cast 必须带 typmod
- **WHEN** 相似查询 SQL 将向量 cast 为 halfvec
- **THEN** cast 表达式为 `::halfvec(2560)`（与索引表达式精确匹配），不带 typmod 的 `::halfvec` 写法视为缺陷（不走索引）

#### Scenario: 索引覆盖三类 embedding
- **WHEN** 查看 topic_tag_embeddings 索引
- **THEN** semantic / identity / event_keyword 三类各有一个 partial halfvec HNSW 索引（按 embedding_type 过滤），坏索引 `tmp_hnsw_test` 不存在

### Requirement: embedding 队列 worker 连续处理且有限并发

embedding 队列 worker SHALL 在处理完当前任务后立即取下一任务（无固定 2 秒轮询间隔），并 SHALL 以 ≤4 的并发度并行处理任务。任务领取 SHALL 使用 `FOR UPDATE SKIP LOCKED` 防止重复处理。

#### Scenario: 连续处理无空转
- **WHEN** 队列有 100 个 pending 任务且 provider 正常
- **THEN** 任务间间隔中位数 < provider 单次往返耗时 + 1s（无 2s 固定空转）

#### Scenario: 并发上限
- **WHEN** 队列大量积压
- **THEN** 同时处于 processing 状态的任务数 ≤ 4

#### Scenario: worker 重启不残留
- **WHEN** 队列服务 Stop 后再次 Start
- **THEN** 旧 worker goroutine 退出（进程内 worker 计数回到期望值），不存在多 worker 抢队列

### Requirement: 队列任务批量调用 embedding

worker 处理任务时，SHALL 将同类型（identity/semantic/event_keyword）的待处理标签合并为批量请求（每批 ≤32 条）调用 provider，并 SHALL 对批内每条文本独立命中/写入 embedding 缓存。

#### Scenario: 批量调用
- **WHEN** 队列积压 10 个标签任务（各含 identity+semantic 两种类型）
- **THEN** provider 收到 identity 批量请求 ≤1 次（≤32 条/批）+ semantic 批量请求 ≤1 次，而非 20 次单条请求

#### Scenario: 批量下缓存按单条命中
- **WHEN** 批量请求中某 3 条文本已有缓存
- **THEN** 该 3 条命中缓存不重复调 provider，其余 miss 条目正常出向量并写入缓存

### Requirement: 失败自动重试与可观测

任务失败时，若 retry_count < 3，SHALL 在延迟（指数退避，基座 30s）后自动回到 pending；worker SHALL 使用真实 logger 输出处理日志。`GET /api/embedding/queue/status` 响应 SHALL 额外包含 `oldest_pending_age_seconds` 与 `throughput_per_min` 两个字段。

#### Scenario: 失败自动重试
- **WHEN** 任务因 provider 暂时错误失败且 retry_count=1
- **THEN** 任务延迟后回到 pending 并被再次处理，不进入终态 failed

#### Scenario: 重试上限
- **WHEN** 任务第 4 次失败（retry_count=3）
- **THEN** 任务进入终态 failed，等待人工 retry 接口

#### Scenario: status 新增字段
- **WHEN** 调用 GET /api/embedding/queue/status
- **THEN** 响应含 oldest_pending_age_seconds（最老 pending 的年龄秒数）与 throughput_per_min（最近完成速率）
