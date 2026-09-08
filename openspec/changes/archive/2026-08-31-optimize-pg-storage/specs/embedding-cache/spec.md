# embedding-cache Delta — optimize-pg-storage

## MODIFIED Requirements

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
