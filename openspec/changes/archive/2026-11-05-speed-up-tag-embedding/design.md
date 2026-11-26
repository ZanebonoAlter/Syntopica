# Design: speed-up-tag-embedding

> 复杂度：**complex**（并发模型改造 + 索引/SQL 两段式 + 批量化，含 test-cases.md 白盒附加节）

## Context

生产实测瓶颈与可行性验证见 `docs/research/tag-embedding-slow/explore-findings.md`（两篇：链路慢因实测分析、halfvec 可行性验证结论）。核心结论：halfvec HNSW 2560 维可建（54s），两段式查询 1366ms → 9.3ms，typmod 必须精确匹配 `::halfvec(2560)`，`hnsw.ef_search` 需 ≥120。

## Goals / Non-Goals

**Goals**：相似查询走索引；队列去空转+4 并发；批量 embedding；失败自动重试；可观测字段。
**Non-Goals**：MRL 降维、analysispause 门禁分级、merge 队列 SKIP LOCKED、aux 回填批量化（后续 change）。

## Decisions

### D1 相似查询两段式（FindSimilarTags）
- SQL：候选池 `ORDER BY embedding::halfvec(2560) <=> $1::halfvec(2560) LIMIT 50`（partial HNSW per embedding_type），再候选集内 `embedding <=> $1::vector` 精确重排
- 会话级 `SET hnsw.ef_search = 120`：连接池场景用 `SET LOCAL`（事务内）或连接获取后设置；实现取「查询前 `SET LOCAL`」保证不污染连接
- **红线遵守**：cosine 校验口径不变；L2 命中仍只 addAlias（`core/embedding.go` 上游语义不动）
- 部署：`CREATE INDEX CONCURRENTLY` ×3（semantic/identity/event_keyword），drop `tmp_hnsw_test`；须在低负载窗口执行（验证期间高负载叠加建索引曾触发后端崩溃 ×3 + 整机恢复 ×1）

### D2 队列 worker 改造
- 2s ticker → `for { claim; if none → 短暂退避 sleep(200ms) }`；并发 = 4 goroutine，各自 `FOR UPDATE SKIP LOCKED` claim（沿用现有 claim 事务）
- 批量处理：每次 claim 攒批（同 embedding_type 最多 32 条），一次 provider 批量调用后逐条落库、逐条 mark completed
- stopCh 竞态修复：`Start()` 时若旧 worker 未退出（waitgroup/原子计数）先等待；Stop 等待全部 goroutine 退出

### D3 批量缓存
- `airouter/router.go` Embed 路径：input 长度 >1 时先按单条 key 批量查缓存（`WHERE cache_key IN (...)`），miss 部分发 provider，结果按单条 key 批量写入；单条路径行为不变
- 缓存 key 生成复用现有 `embeddingCacheKey` 的单条口径

### D4 失败重试
- `markFailed`：retry_count < 3 → `next_retry_at = now + 30s * 2^(retry_count-1)`，worker claim 条件加 `AND (next_retry_at IS NULL OR next_retry_at <= now())`；≥3 终态 failed（保留手动 retry 接口）
- `Router.Embed` 对 `ProviderError.Retryable=true` 做 1 次立即退避重试（200ms）

### D5 可观测
- `workers.go` 传入真实 logger（去掉 `zap.NewNop()`）
- status 增 `oldest_pending_age_seconds`（`min(created_at) where status=pending`）/ `throughput_per_min`（最近 60s completed 计数）

## Risks / Trade-offs

- **halfvec 精度**：两段式第二段用 float32 精排，排序口径不变；唯一风险是 ANN 召回不全（候选漏掉真 top10）→ ef_search=120 + 20 探针 overlap=10/10 验收兜底
- **4 并发 × provider**：qwen3-embedding 为自托管（10.11.12.111:8081），并发 4 在其承载范围；如遇限流，失败重试（D4）兜底
- **建索引窗口**：低负载执行，必要时暂停抓取；CONCURRENTLY 不锁写
- **批量与增量混合**：批任务落库逐条进行，中途失败只影响批内剩余，已落库的不回滚（幂等：唯一索引兜底）

## Migration Plan

1. 部署前：确认 `shm_size: 1g` 生效（已改 compose）
2. 低负载窗口：CONCURRENTLY 建 3 索引（各约 1 分钟）+ drop tmp_hnsw_test
3. 发新版（两段式查询 + 队列改造同时上线）
4. 回滚：代码回滚即回全表扫描（索引共存无害）；索引可保留
