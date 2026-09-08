
## optimize-pg-storage 落地进度与生产库已执行操作

optimize-pg-storage apply 阶段关键事实（2026-08-28，生产库已实际执行的部分）：

1. **生产库 ai_embedding_cache 已完成 jsonb→bytea 转换**（TestInitDBConnectsToPostgres 长超时跑真实 InitDB 链，279s 分批流式完成，53055 行全部保留无损）+ 已 VACUUM FULL（1682MB→653MB）。库总大小 12.4GB（会话起点）→ 7.8GB。
2. **preMigrateEmbeddingCacheBytea 必须先于 AutoMigrate**（migrator.go RunAutoMigrate 开头调用）：GORM 发现模型 bytea vs 列 jsonb 会自行 ALTER，PG 无 jsonb→bytea 隐式 cast 直接报错中断启动。已挂载。
3. **迁移实现要点**（postgres_migrations.go）：keyset 分批（500 行/批，cache_key 游标）流式转换防 OOM（全量 map 攒内存实测被 OOM kill）；四阶段（ADD COLUMN→分批回填→pending 校验→短事务 DROP+RENAME），中断可重入；非破坏（不依赖 MIGRATIONS_ALLOW_DESTRUCTIVE）。
4. **codec 在 models/embedding_codec.go**（Encode/DecodeEmbeddingVectors 导出，airouter 与 database 共用）：float64 向量数组 ↔ 小端 float32 字节流，格式 [8B count][per vec: 8B dim + dim*4B]，2560 维单向量 10256 字节。cache_key 是 sha256 hex（排序安全）。
5. **docker-compose.pg.yml autovacuum 参数已生效**（容器已 recreate，2026-08-28）：scale_factor 0.05 / insert_scale_factor 0.05 / insert_threshold 500 / cost_limit 2000 / wal_compression=on（PG18 SHOW 显示 pglz）。
6. **TestInitDBConnectsToPostgres 直连生产库跑全迁移链**（configs/config.yaml DSN）——以后写涉及迁移的代码时，database 包测试可能在生产库上真实执行 DDL，跑包测试前想清楚副作用。
7. 待部署验证：TRACE_SAMPLE_RATIO 默认 0.05 生效（otel_spans 日增应降一个数量级）、新缓存行 pg_column_size≈10256。

<!-- pinned 2026-08-28T05:31:41Z -->
