# Review: speed-up-tag-embedding

主线程对实现子线程（zai-coding-cn/glm-5.3:max）产出做的验收抽检。

### R-1 2026-11-05【Medium】关键实现点抽检
- **状态**：已修复（无需修复，抽检通过）

抽检项与结论：
1. 两段式 SQL typmod：`embedding::halfvec(%d)` 动态带维度，>4000 维回退全表精确扫描（口径不变）——通过
2. `SET LOCAL hnsw.ef_search=120` 在事务内，不污染连接池——通过
3. claim：`FOR UPDATE SKIP LOCKED ... LIMIT ?` + `next_retry_at <= now` 过滤——通过
4. 缓存兼容：`embedSingle` 单条路径 key 生成原样保留，`embedBatch` 走批查批写——通过
5. 索引迁移 SQL 头部带 🛑 手动执行警告、维度前置检查、CONCURRENTLY 不包事务——通过
6. 测试报告复核：影响包全绿、lint 0 issues（中途红为外部 pg_restore 演练 IO 打满，恢复后复绿，非代码问题）

- 审毕无发现（审查范围：core/embedding.go 两段式、embedding_queue.go worker/重试、airouter router.go 批量缓存拆分、索引 SQL 头部、测试清单与结果报告；未逐行复读全部新测试文件，以执行结果 + 关键断言抽检为准）
