# embedding-cache Specification

## Purpose
TBD - created by archiving change nightly-throughput-embedding-cache-parallel-crawl. Update Purpose after archive.
## Requirements
### Requirement: Router.Embed 层 embedding 结果持久化缓存（仅限白名单 operation）
`Router.Embed`（`backend-go/internal/platform/airouter/router.go`）SHALL 仅对白名单 `embeddingCacheOperations`（当前仅 `tagmanagement.embedding`，输入由 tag 固定属性构成、跨文章重复）中的 operation 启用缓存：在加载 route 后、获取信号量之前，按 `(model, input_hash)` 组成的 cache_key 查询 `ai_embedding_cache` 表；命中时 SHALL 跳过 provider 调用与信号量占用，直接反序列化返回 `*EmbeddingResult`。白名单外的 operation（如 `section.embedding` 文章正文、`tagmanagement.auxlabel_embedding` 标签+文章上下文一次性组合、`discovery.route_embedding` 路由回填）SHALL 跳过缓存查询与写入，直接走真实调用（生产实测这些 operation 命中率 0-10%，每行 ~30KB，属纯存储浪费）。

#### Scenario: 同一输入第二次调用命中缓存
- **WHEN** `Router.Embed` 以相同 `req.Input` 被再次调用，且 route 的主 provider Model 未变
- **THEN** 不发起 provider HTTP 调用，从 `ai_embedding_cache` 返回与首次调用一致的结果

#### Scenario: 命中不占用 embedding 信号量
- **WHEN** 缓存命中时另一调用正持有 CapabilityEmbedding 信号量
- **THEN** 命中路径立即返回，不因信号量阻塞

#### Scenario: 不同 model 不串缓存
- **WHEN** 相同 input 但 route 主 provider 的 Model 不同（如切换 embedding 模型）
- **THEN** cache_key 不同，不命中旧模型的缓存，走真实调用并按新 model 落缓存

### Requirement: provider 调用成功后写入缓存（仅白名单 operation）
`Router.Embed` 在 provider 调用成功后 SHALL 仅对白名单 operation 将结果以 `(model, input_hash)` 为键 upsert 到 `ai_embedding_cache`（`ON CONFLICT DO NOTHING` 幂等），字段包括 model、operation、embedding、dimensions、input_preview（前 200 字符）。其中 embedding 列 SHALL 以 bytea 存储向量 float32 数组的原始小端字节流（每维 4 字节，无文本编码膨胀），替代原 jsonb 浮点数组文本形式（jsonb 形式单条 ~31KB，二进制形式 ~10KB）。白名单外 operation SHALL 不写缓存行。缓存写入失败 SHALL 仅记录 warn 日志，不影响调用结果返回。

存量迁移：列类型切换 SHALL 在 AutoMigrate 之前执行（GORM 对 jsonb→bytea 无可用 cast，放任 AutoMigrate 处理会报 "cannot cast type jsonb to bytea" 并中断启动）。迁移 SHALL 非破坏：旧 jsonb 行解码后按二进制格式回写（缓存价值保留），不依赖 MIGRATIONS_ALLOW_DESTRUCTIVE 门禁；无法解码的坏行降级为 NULL 并记 warn。迁移 SHALL 幂等可重跑（列已为 bytea 时跳过）。

#### Scenario: 未命中调用成功后落缓存
- **WHEN** 缓存未命中且 provider 调用成功
- **THEN** 结果以 bytea 二进制形式写入 `ai_embedding_cache`，下次相同输入直接命中，反序列化结果与首次调用的向量逐维一致

#### Scenario: 缓存行大小符合二进制预期
- **WHEN** 写入一条 dimensions=2560 的缓存行
- **THEN** embedding 列的 pg_column_size 约 10256 字节（8B count + 8B dim + 2560×4B），不随浮点数文本长度膨胀

#### Scenario: 旧 jsonb 行无损转换且幂等
- **WHEN** 迁移在已含 jsonb 旧数据的库上执行（destructive 门禁保持关闭），随后再次执行完整启动迁移序列
- **THEN** 旧行保留并可按二进制格式解码还原向量，列类型为 bytea，重复执行不报错、不产生重复数据

#### Scenario: AutoMigrate 不因列类型差异中断启动
- **WHEN** 库中 ai_embedding_cache.embedding 仍为 jsonb 时后端启动（AutoMigrate 先于版本迁移执行）
- **THEN** 预迁移在 AutoMigrate 比较列类型前把列转为 bytea，启动正常完成，无 "cannot cast" 错误

#### Scenario: 并发同 key 双写幂等
- **WHEN** 两个并发请求以相同 input 未命中并同时调用 provider 成功
- **THEN** upsert 幂等，表中该 key 只有一行，无报错

#### Scenario: 缓存写失败不影响主流程
- **WHEN** 缓存表写入失败（如临时 DB 错误）
- **THEN** 调用结果正常返回给上游，仅记录 warn 日志

### Requirement: 缓存命中写入 ai_call_logs 并标记 cache_hit
缓存命中时系统 SHALL 向 `ai_call_logs` 记录一条 `Success=true`、`LatencyMs` 为实际本地耗时（接近 0）的日志，且 `RequestMeta` SHALL 包含 `"cache_hit": true`，使命中率可经 SQL 量化。

#### Scenario: 命中日志可统计
- **WHEN** 查询 `SELECT count(*) FILTER (WHERE (request_meta::jsonb->>'cache_hit') = 'true')::float / count(*) FROM ai_call_logs WHERE capability = 'embedding'`（request_meta 列是 TEXT，需先 ::jsonb 再取键）
- **THEN** 可得到该时间段内 embedding 缓存命中率

### Requirement: 缓存记录 14 天自动清理
`log_cleanup` 定时任务 SHALL 顺带删除 `ai_embedding_cache` 中 `created_at` 早于当前时间 14 天的记录（命中几乎全部发生在写入后 1-2 天的夜间窗口内，长于 14 天的保留期对命中率无贡献，属纯磁盘浪费）。

#### Scenario: 过期记录被清理
- **WHEN** `log_cleanup` 执行且存在 created_at 超过 14 天的缓存记录
- **THEN** 这些记录被删除，近期记录保留

#### Scenario: 白名单外 operation 不落缓存
- **WHEN** `section.embedding` 或 `tagmanagement.auxlabel_embedding` 等 operation 调用成功
- **THEN** 不写 `ai_embedding_cache`，后续同输入调用直接走 provider，不产生 cache_hit 日志

