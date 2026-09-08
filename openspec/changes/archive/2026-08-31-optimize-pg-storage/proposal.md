# optimize-pg-storage

<!-- 纯工具链/存储优化 change，不涉及业务域，无 constraint-domains 声明 -->
<!-- complexity: complex（向量二进制编解码解析 + pre-migrate 迁移状态分支，见 test-cases.md 白盒附加） -->

## Why

PG 库体积膨胀到 12.4GB（2026-08-28 VACUUM FULL 后仍有 8.9GB），两大存储源头失控：
1. `otel_spans` 全采样（SampleRatio=1.0 + GORM 每条 SQL 一个 span）日产 ~82 万 span ≈ 600MB/天，7 天 TTL 下稳态 4.3GB，且每天 82 万行 DELETE 持续制造死元组膨胀压力；
2. `ai_embedding_cache` 用 jsonb 存 embedding 向量（浮点数组文本形式），单条压缩后仍 31.5KB，5.3 万条 = 1.7GB——同样的 float32 数组按二进制存只要 10.2KB，选型浪费 3 倍。

## What Changes

- **tracing 降采样**：`SampleRatio` 默认值 1.0 → 0.05，并支持环境变量覆盖（当前 `tracing/config.go` 的 DefaultConfig 写死全采样）。预期 otel_spans 日增量降 ~95%。
- **embedding 缓存改二进制存储**：`ai_embedding_cache.embedding` 列 jsonb → bytea（float32 数组原始字节，全精度无损），Go 侧序列化/反序列化同步改造；存量 jsonb 数据直接清空（缓存表，命中集中在写入后 1-2 天，老数据无保留价值）。1.7GB → ~0.55GB。
- **PG autovacuum 调优**：`docker-compose.pg.yml` 为 postgres 加 autovacuum 参数（vacuum scale factor 收紧 + insert 触发 + cost limit 提高），缓解高频 INSERT/DELETE 表的死元组堆积。

不做什么（明确出圈）：otel_spans 分区改造、semantic_labels 向量降精度、embedding 降维——收益大但动静大，后续单独开 change。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `embedding-cache`: 缓存存储格式 requirement 变化——embedding 字段从 jsonb 文本形式改为 bytea 二进制（float32 小端字节流），写入/读取/迁移行为随之调整。
- `otel-business-tracing`: 新增采样率 requirement——默认采样 0.05、支持环境变量覆盖，span 覆盖行为从"全采样"变为"低采样+可调"。

## Impact

- 代码：`backend-go/internal/platform/tracing/config.go`（采样默认值 + env 读取）、`backend-go/internal/models/ai_models.go`（Embedding 字段类型）、`backend-go/internal/platform/airouter/router.go`（saveEmbeddingCache/读缓存的序列化）、`backend-go/internal/platform/database/postgres_migrations.go`（列类型迁移 + 存量清理）。
- 部署：`docker-compose.pg.yml`（autovacuum 参数，需 `docker compose down/up` 重建容器生效）。
- 行为变化：trace 覆盖率从 100% 降到 5%（排障时需临时调高）；缓存表首日为空、夜间任务重建；老缓存行在迁移时清空（重算成本 = 下次夜间任务对 5.3 万白名单输入的 embedding 调用，API 成本可接受）。
- 文档：`docs/reference/database/`（存储说明）、`docs/reference/configuration.md`（新增 tracing 采样配置项）。
