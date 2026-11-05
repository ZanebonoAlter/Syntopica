# Tasks: speed-up-tag-embedding

<!-- doc-impact: flow, database, deployment -->

## 1. 索引与两段式查询

- [x] 1.1 编写索引迁移 SQL（低负载窗口执行）：CONCURRENTLY 建 semantic/identity/event_keyword 三个 partial halfvec HNSW 索引（`embedding::halfvec(2560)` halfvec_cosine_ops），DROP INDEX `tmp_hnsw_test`
- [x] 1.2 `core/embedding.go` FindSimilarTags 改两段式：`::halfvec(2560)` 索引召回 LIMIT 50 + `SET LOCAL hnsw.ef_search=120` + 候选集 `::vector` 精确重排；查询超时与错误路径保持现状
- [x] 1.3 单测/集成测试：真实 PG 下 20 探针 overlap=10/10 + EXPLAIN 确认 Index Scan（落点见 test-cases.md 主链路）

## 2. 队列 worker 改造

- [x] 2.1 `core/embedding_queue.go`：去 2s ticker（空队列时 200ms 退避），并发 4 goroutine，claim 沿用 FOR UPDATE SKIP LOCKED
- [x] 2.2 修 Stop/Start stopCh 竞态：Stop 等待全部 worker goroutine 退出后返回；Start 前断言无残留 worker
- [x] 2.3 批量处理：claim 时攒同类型批（≤32），一次 provider 批量调用，逐条落库/标记完成
- [x] 2.4 `airouter/router.go` Embed 批量缓存：按单条 key `IN` 批查、miss 批调、单条 key 批写；单条路径 key 不变（兼容历史缓存）
- [x] 2.5 失败自动重试：markFailed 加 next_retry_at 指数退避（30s 基座，<3 次），claim 条件过滤未到期任务；Router.Embed 对 Retryable 错误做 1 次退避重试
- [x] 2.6 workers.go 接真实 logger；status API 增 oldest_pending_age_seconds / throughput_per_min

## 3. 验证与收尾

- [x] 3.1 按 test-cases.md 跑全量用例：影响包全绿（go test ./internal/tagmanagement/... ./internal/platform/airouter/...；golangci-lint 0 issues；go vet/build 通过，实现子线程执行主线程复核）
- [x] 3.2 效果核对（真库量化）：积压 6 任务自动消化清零（oldest_pending 41673s→null）；索引生效（semantic idx_scan 增长，稳态 9.9ms vs 1366ms 基线）；100 任务压测因避免生产副作用未跑，以分环节证据替代
- [x] 3.3 部署：低负载窗口执行 1.1 索引 SQL（三索引 valid，tmp_hnsw_test 已清）→ 发版 → 队列恢复处理、DB 无异常
- [x] 3.4 文档：flow/semantic-board.md 变更溯源已补；doc-impact verify 通过（声明: flow, database, deployment）；check-standards 264/0

## 4. 测试

影响包全绿（见 3.1）。用例清单见 test-cases.md 主链路 11 步 + 变体走查 + 白盒边界表。

## 5. 文档

<!-- doc-impact: flow, database, deployment -->

- flow：`docs/reference/flow/semantic-board.md` 变更溯源补 speed-up-tag-embedding 行（与 board-health-triage-advice 共享文件，最小追加）
- database：索引 SQL `scripts/db/tag-embedding-halfvec-hnsw-20261104.sql`（含 shm_size 前置与 typmod 红线）
- deployment：`docker-compose.pg.yml` 加 `shm_size: 1g`（halfvec HNSW 构建前置）

## 6. 验证

| Scenario | 测试文件 |
| --- | --- |
| 相似查询走索引且结果与精确排序一致 | backend-go/internal/tagmanagement/service/core/embedding_index_recall_test.go |
| 查询 cast 必须带 typmod | backend-go/internal/tagmanagement/service/core/embedding_index_recall_test.go |
| 索引覆盖三类 embedding | backend-go/internal/tagmanagement/repository/embedding_index_test.go |
| 连续处理无空转 | backend-go/internal/tagmanagement/service/core/embedding_queue_lifecycle_test.go |
| 并发上限 | backend-go/internal/tagmanagement/service/core/embedding_queue_lifecycle_test.go |
| worker 重启不残留 | backend-go/internal/tagmanagement/service/core/embedding_queue_lifecycle_test.go |
| 批量调用 | backend-go/internal/tagmanagement/service/core/embedding_queue_batch_test.go |
| 批量下缓存按单条命中 | backend-go/internal/platform/airouter/embed_batch_cache_test.go |
| 单条路径 key 兼容 | backend-go/internal/platform/airouter/embed_batch_cache_test.go |
| 失败自动重试 / 重试上限 | backend-go/internal/tagmanagement/service/core/embedding_queue_retry_test.go |
| 失败自动重试 | backend-go/internal/tagmanagement/service/core/embedding_queue_retry_test.go |
| 重试上限 | backend-go/internal/tagmanagement/service/core/embedding_queue_retry_test.go |
| 批量部分命中 | backend-go/internal/platform/airouter/embed_batch_cache_test.go |
| status 新增字段 | backend-go/internal/tagmanagement/handler/embedding_queue_status_test.go |
| 效果核对（真库量化） | 人工：生产部署后实测——semantic idx_scan 增长、稳态 9.9ms、积压清零（oldest_pending 41673s→null），记录见 3.2 |
