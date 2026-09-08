# Design — optimize-pg-storage

## Context

见 proposal.md「Why」。现状关键事实：`otel_spans` 全采样日产 82 万 span（`tracing/config.go` DefaultConfig `SampleRatio=1.0`，含 GORM/HTTP 插桩）；`ai_embedding_cache` 5.3 万行 × 31.5KB jsonb 向量（`ai_models.go:159-171` Embedding 字段 `type:jsonb`），缓存值最终会写入 `topic_tag_embeddings` 的全精度 vector 列；库已做过一轮 VACUUM FULL（12.4→8.9GB），本 change 解决"稳态产量"。

## Goals / Non-Goals

**Goals:**
- otel_spans 日增量降 ~95%（600MB/天 → ~30MB/天）
- ai_embedding_cache 单条体积 31.5KB → ~10KB，存量 1.7GB → ~0.55GB（按当前行数）
- autovacuum 对高插入/高删除表及时介入，防止再次膨胀

**Non-Goals:**
- otel_spans 分区/TTL 改造（单独 change）
- semantic_labels / topic_tag_embeddings 向量降精度或降维（单独 change）
- 缓存淘汰策略（LRU/容量上限）变更——维持现有 14 天 TTL

## Decisions

### D1: 缓存列 jsonb → bytea（float32 小端字节流），不用 halfvec

**选择 bytea**。理由：缓存值命中后会被反序列化并最终写入 `topic_tag_embeddings` 的**全精度** `vector` 列；halfvec 是 fp16，会在缓存层丢精度、污染下游全精度向量。bytea 每维 4 字节、无文本编码膨胀，2560 维 = 10,240 字节，与 jsonb 的 ~31.5KB 相比省 67%，且无损。

- 备选 halfvec(2560)（5.1KB/条）被否：精度损失传导下游，收益再省 5KB 不值得。
- 备选"保留 jsonb + 换 zstd 压缩"被否：浮点文本高熵，压缩收益有限且读写 CPU 开销大；二进制才是根治。
- 序列化：Go 侧 `[]float32` ↔ 小端字节（`math.Float32bits` 循环或 `encoding/binary`），不引入 unsafe。

### D2: 存量缓存非破坏转换（pre-AutoMigrate），不依赖 destructive 门禁

实测发现顺序硬约束：启动时 `RunAutoMigrate` 先于版本迁移执行，GORM 发现模型声明 bytea 而列是 jsonb 会自行 ALTER——PG 无 jsonb→bytea 隐式 cast，直接报 "cannot cast" 中断启动。因此转换必须是 **pre-migrate**（`RunAutoMigrate` 开头调用 `preMigrateEmbeddingCacheBytea`），而非版本迁移。

转换非破坏：读旧 jsonb 行 → Go 侧 codec 重编码 → 新列回填 → 换列名（ADD COLUMN bytea / UPDATE 回填 / DROP 旧列 / RENAME，单事务 + lock_timeout）。5.3 万条存量缓存价值保留，无重算成本；坏行降级 NULL + warn。因无数据丢失，不涉及 `MIGRATIONS_ALLOW_DESTRUCTIVE` 门禁，生产默认路径直接生效。幂等：列类型非 jsonb 即跳过。

codec 落在 `models/embedding_codec.go`（导出），airouter 与 database 共用——database 直接 import airouter 会引入反向依赖。

- 回滚兼容：代码回滚后旧代码按 jsonb 读 bytea 列会失败，但缓存读失败仅 warn + 走真实调用（既有行为：`router.go` 读缓存失败不阻断主流程），影响可控。

### D3: 采样用 ParentBased(TraceIDRatioBased(0.05))，env 可覆盖

OTel SDK 标准组合：根 span 按 TraceID 确定性采样 5%；被采中的 trace 的**全部后代 span（含 GORM/HTTP 子 span）都保留**，保证拿到的是完整调用树而非碎片。env `TRACE_SAMPLE_RATIO`（0.0–1.0，`strconv.ParseFloat` + 范围校验）非法值回退 0.05 并 warn。仅改 `tracing/config.go` 的默认值与解析逻辑，插桩开关不动。

- 备选"只对 GORM 插桩降采样"被否：碎片化 span 树排障价值低，且非正统做法。
- 错误可观测性不依赖 trace：`ai_call_logs` 全量记录不受采样影响。

### D4: autovacuum 参数走 docker-compose command，一次到位

`docker-compose.pg.yml` 的 postgres service 追加：

```yaml
command:
  - postgres
  - -c
  - autovacuum_vacuum_scale_factor=0.05        # 默认 0.2，大表触发严重滞后
  - -c
  - autovacuum_vacuum_insert_scale_factor=0.05  # 纯 INSERT 表（otel_spans）的触发器，死元组模型管不到
  - -c
  - autovacuum_vacuum_insert_threshold=500
  - -c
  - autovacuum_vacuum_cost_limit=2000           # 默认 200 追不上写入速度
  - -c
  - wal_compression=on                          # 高写入库的 WAL 全页镜像压缩
```

需 `docker compose -f docker-compose.pg.yml up -d` recreate 容器生效（数据在 bind mount，不丢）。备选"表级 reloptions（ALTER TABLE SET）"被否：改动分散在各表、易漂移，且 insert scale factor 这类参数全局设置即可。

## Risks / Trade-offs

- [5% 采样漏掉偶发问题的 trace] → 排障时 `TRACE_SAMPLE_RATIO=1.0` 重启后端即可恢复全采样；`ai_call_logs`/应用日志不受影响。
- [迁移清空缓存后首夜重算 5.3 万次 embedding 调用] → 一次性成本，夜间窗口消化；命中路径自愈，无功能损失。
- [bytea 列 + 旧代码回滚组合下缓存读失败] → 既有降级路径（warn + 真实调用）兜底，无功能性破坏。
- [autovacuum cost_limit 提高增加后台 IO] → 单用户本地库，前台负载低，2000 远低于会干扰的水平。
- [VACUUM 只回收不还空间，otel_spans 仍会缓慢膨胀] → 降采样后日增量 30MB，7 天 TTL + insert-triggered vacuum 下膨胀速度降 95%；根治靠后续分区 change。

## Migration Plan

1. 部署新代码：后端启动时版本迁移自动执行（TRUNCATE + ALTER COLUMN → bytea），随后写入路径改二进制。
2. `docker compose -f docker-compose.pg.yml up -d`：recreate 容器，autovacuum 参数生效。
3. 验证：观察 `pg_stat_user_tables`（n_dead_tup 回收及时性）、`otel_spans` 日增行数、缓存行 `pg_column_size` ≈ 10240。
4. 回滚：代码回滚即可（缓存读失败自动降级）；docker-compose 参数独立回滚（git revert + up -d）。
