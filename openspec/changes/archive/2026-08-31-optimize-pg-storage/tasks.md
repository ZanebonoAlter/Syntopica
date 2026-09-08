# Tasks — optimize-pg-storage

## 1. tracing 降采样

- [x] 1.1 改 `backend-go/internal/platform/config/config.go` viper 默认 + env `TRACE_SAMPLE_RATIO` 解析强化（范围校验 0.0–1.0，非法回退 0.05 并 warn）+ `tracing/config.go` 兑底 0.05。验证：`go test ./internal/platform/config/ -run TestTracingSampleRatio` 6 场景全 PASS。
- [x] 1.2 确认 `provider.go` 采样器为 `ParentBased(TraceIDRatioBased(ratio))`（代码既有）。验证：`go test ./internal/platform/tracing/ -run TestDefaultConfigSampleRatio` PASS。

## 2. embedding 缓存改 bytea

- [x] 2.1 新增 `[]float64` 向量 ↔ 小端 float32 字节序列化工具（`models/embedding_codec.go`，airouter 与 database 共用避免反向依赖）。验证：`go test ./internal/models/ -run TestEmbeddingCodec` 往返/NaN/Inf/截断/尺寸断言 PASS。
- [x] 2.2 改 `backend-go/internal/models/ai_models.go` `AIEmbeddingCache.Embedding`：`string type:jsonb` → `[]byte type:bytea`。验证：`go build ./...` 通过。
- [x] 2.3 改 `backend-go/internal/platform/airouter/router.go`：写入/读取接 codec，读失败降级 warn + 真实调用维持既有行为；适配既有测试种子为 float32 精确值。验证：`go test ./internal/platform/airouter/ -run "TestEmbed|TestLookup"` PASS（含往返/命中跳过 provider/信号量不占用）。
- [x] 2.4 `RunAutoMigrate` 开头接 `preMigrateEmbeddingCacheBytea`（`postgres_migrations.go`）：jsonb→bytea 非破坏转换（旧行重编码回填，单事务 + lock_timeout，幂等；不依赖 destructive 门禁）。实测依据：AutoMigrate 先于版本迁移跑，GORM 对 jsonb→bytea 自行 ALTER 报 "cannot cast" 中断启动，必须预迁移。验证：`go test ./internal/platform/database/ -run TestEmbeddingCacheBytea -count=1`（转换保留数据/幂等两用例 PASS，含真实启动顺序）。

## 3. docker-compose autovacuum 参数

- [x] 3.1 `docker-compose.pg.yml` postgres service 追加 command 参数（scale_factor=0.05 / insert_scale_factor=0.05 / insert_threshold=500 / cost_limit=2000 / wal_compression=on，完整参数见 design.md D4）。验证：`docker compose -f docker-compose.pg.yml config` 输出含全部参数；起容器后 `SHOW autovacuum_vacuum_scale_factor` = 0.05。

## 4. 测试

- [x] 4.1 影响包单测：`cd backend-go && go test ./internal/platform/tracing/... ./internal/platform/airouter/... ./internal/models/...`（包路径以实际改动为准，用 `bash scripts/change-scope.sh` 机械判定）→ 全部 PASS。
- [x] 4.2 质量门禁：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` → 零报错。

## 5. 文档

<!-- doc-impact: database, configuration, deployment, flow -->
<!-- doc-impact-excuse: api=工作区 dataenrichment/handler/ 脏文件属其他在途 change，本 change 未触及 handler 层; standard=工作区 docs/reference/standard/backend/ai-logging.md 脏文件属其他在途 change，本 change 未触及 standard 域 -->

- [x] 5.1 `docs/reference/database/DATA_LIFECYCLE.md` + `_index.md`：更新 ai_embedding_cache 存储格式说明（jsonb → bytea 二进制，~10KB/条）与本 change 溯源。
- [x] 5.2 `docs/reference/configuration.md`：新增 `TRACE_SAMPLE_RATIO` 配置项（默认 0.05、范围、排障时临时调 1.0 的用法）。
- [x] 5.3 `docs/reference/deployment.md`：补 docker-compose autovacuum 参数说明与生效方式（recreate 容器）。
- [x] 5.4 `docs/reference/flow/ai-summary.md`：业务约束 #10 缓存条目补 bytea 二进制存储格式（~10KB vs 原 ~31.5KB），修正白名单外 rationale 中失真的 ~30KB 数字；archive 后按 §12.2 补变更溯源行。

## 6. 验证

- [x] 6.1 `cd backend-go && go test ./internal/platform/tracing/...` → PASS，覆盖三采样场景（默认 0.05 / env=1.0 生效 / env=abc 回退 0.05）。
- [x] 6.2 `docker exec syntopica-postgres psql -U postgres -d syntopica -c "\d ai_embedding_cache"`（53055 行全部转换，生产库 2026-08-28 已迁移） → embedding 列类型为 bytea（迁移后）。
- [x] 6.3 触发一次白名单 embedding 落缓存后：`SELECT pg_column_size(embedding) FROM ai_embedding_cache ORDER BY created_at DESC LIMIT 1;` → ≈10256（8B count+8B dim+2560×4B，dimensions=2560 时）。实测（2026-08-31）：部署后新写入 12623 行（08-28/29/31 三天）全部 pg_column_size=10256、dimensions=2560，无一偏差。
- [x] 6.4 `docker exec syntopica-postgres psql -U postgres -d syntopica -c "SHOW autovacuum_vacuum_scale_factor;"`（0.05 / insert 0.05 / cost 2000 / wal_compression pglz，容器已 recreate） → 0.05（容器 recreate 后）。
- [x] 6.5 部署 24h 后观察：`SELECT count(*) FROM otel_spans WHERE start_time_unix_nano > (extract(epoch from now())*1e9 - 86400e9);` → 较全采样基线（~82 万/天）降约一个数量级。实测（2026-08-31）：按日聚合 08-25~27 为 51.3/70.1/84.2 万 span/天，08-28 部署后 08-29=3.06 万、08-31=3.14 万（近 24h=31745），降幅 ~96%；otel_spans last_autovacuum=当日 20:45（insert 触发生效），ai_embedding_cache n_dead_tup=0。
- [x] 6.6 验证节全量重跑（归档前，2026-08-31 实测）：6.1/1.1/1.2/2.1/2.3/2.4 各包测试全部 `ok`；6.2 embedding 列=bytea；6.3 新写入 12623 行全部 10256；6.4 五参数全中；`golangci-lint run ./...` → 0 issues；`go vet ./...`、`go build ./...` → 零输出成功。
- [x] 6.7 `bash scripts/scenario-trace.sh openspec/changes/optimize-pg-storage` → 退出码 0（10 个 Scenario：自动测试 9 / 人工留痕 1）；`bash scripts/doc-impact.sh verify openspec/changes/optimize-pg-storage` → 通过；`bash scripts/check-standards.sh` → A-D/F/G 段零失败。

| Scenario | 测试文件 |
| 未命中调用成功后落缓存 | backend-go/internal/platform/airouter/embed_cache_router_test.go, backend-go/internal/platform/airouter/embed_cache_test.go |
| 缓存行大小符合二进制预期 | backend-go/internal/models/embedding_codec_test.go |
| 旧 jsonb 行无损转换且幂等 | backend-go/internal/platform/database/embedding_cache_bytea_migration_test.go |
| AutoMigrate 不因列类型差异中断启动 | backend-go/internal/platform/database/embedding_cache_bytea_migration_test.go |
| 并发同 key 双写幂等 | backend-go/internal/platform/airouter/embed_cache_router_test.go |
| 缓存写失败不影响主流程 | backend-go/internal/platform/airouter/embed_cache_router_test.go |
| Default sampling is low | backend-go/internal/platform/config/config_test.go |
| Environment override raises sampling | backend-go/internal/platform/config/config_test.go |
| Invalid override falls back safely | backend-go/internal/platform/config/config_test.go |
| Sampled root keeps its span tree | 人工：代码走查 provider.go 采样器为 ParentBased(TraceIDRatioBased)（既有实现，设计 D3） |

> 锦点：缓存行大小 Scenario 的线上佐证见 6.3（12623 行全部 10256B）；Default sampling 线上佐证见 6.5（日增降 ~96%）；并发双写=TestEmbedCacheConcurrentSameKeySingleRow、写失败降级=TestEmbedCacheWriteFailureDoesNotAffectResult（embed_cache_router_test.go）；jsonb 转换/幂等/AutoMigrate 启动顺序=TestEmbeddingCacheByteaConversionPreservesRows/Idempotent（embedding_cache_bytea_migration_test.go，PG testcontainer）。
