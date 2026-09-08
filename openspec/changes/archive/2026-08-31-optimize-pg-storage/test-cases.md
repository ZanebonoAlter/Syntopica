# Test Cases — optimize-pg-storage

主链故事：**开发者重启后端 → tracing 默认低采样（可 env 调高）→ 缓存表 jsonb 旧数据无损转 bytea → 新缓存写入/命中二进制往返 → autovacuum 参数生效，库体积稳态下降。**

## 主链路

| # | 步/动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 无 env 启动，读采样配置 | ADDED: Default sampling is low | ratio=0.05 | 单元 | `config_test.go::TestTracingSampleRatioDefaultsAndOverrides`（defaults 子用例） |
| 2 | `TRACE_SAMPLE_RATIO=1.0` 启动 | ADDED: Environment override raises sampling | ratio=1.0 | 单元 | 同上（env override 子用例） |
| 3 | env=abc / 2.5 / -0.1 启动 | ADDED: Invalid override falls back safely | 0.05 + warn 输出 | 单元 | 同上（3 个 fallback 子用例） |
| 4 | 采样到的根 span 触发子 span | ADDED: Sampled root keeps its span tree | 子 span 全保留（ParentBased 语义） | 代码走查（既有实现） | `tracing/provider.go` 采样器构造 + `tracing/config.go` 注释 |
| 5 | jsonb 旧库上启动（AutoMigrate→Migrations 真实顺序） | MODIFIED: AutoMigrate 不中断启动 + 旧行无损转换 | 列=bytea，3 旧行保留可解码，NULL 行保持 NULL | 集成（PG testcontainer） | `database/embedding_cache_bytea_migration_test.go::TestEmbeddingCacheByteaConversionPreservesRows` |
| 6 | 二次完整启动迁移序列 | MODIFIED: 迁移幂等 | no-op，无报错无重复 | 集成 | 同文件 `::TestEmbeddingCacheByteaConversionIdempotent` |
| 7 | 白名单 embedding 成功落缓存 | MODIFIED: 写入 bytea ~10KB | 2560 维行 pg_column_size=10256（8+8+2560×4） | 单元 | `models/embedding_codec_test.go::TestEmbeddingCodecSizeMatchesExpectation`；线上核对见验证 6.3 |
| 8 | 相同输入二次 Embed 调用 | 既有命中 requirement（不变） | 0 次 provider 调用，向量一致 | 单元 | `airouter/embed_cache_router_test.go::TestEmbedCacheHitSkipsProviderCall`（种子改 float32 精确值） |
| 9 | 缓存读/解码失败 | 既有降级路径（不变） | warn + 真实调用 | 单元（行为保持） | `router.go` decode 失败分支；codec 截断负向 `TestEmbeddingCodecRejectsTruncatedPayload` |
| 10 | recreate 容器后查参数 | tasks 3.1 | `SHOW autovacuum_vacuum_scale_factor`=0.05 | 人工/线上 | 验证 6.4 |

## ⓪ 继承与调整（MODIFIED Requirements 反查旧测试资产）

| 旧 Scenario/资产 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 写缓存（jsonb 字段契约） | MODIFIED（bytea） | `airouter/embed_cache_test.go`（jsonb 字符串种子） | 适配：种子改 `[]byte` + 往返断言改 codec 解码 |
| 缓存命中跳过 provider | 不变 | `airouter/embed_cache_router_test.go` | 适配：fake 向量改 float32 精确值 {0.5,0.25}（0.1/0.2 非 float32 精确，往返后不等） |
| log_cleanup 14 天清理（种子 jsonb `"[]"`） | 不变 | `admin/scheduler/job_log_cleanup_test.go` | 适配：种子 `Embedding: "[]"` → `[]byte("[]")`（列类型随模型变化） |
| TRUNCATE 清理缓存行（本 change 曾设计，已废弃） | N/A（未上线） | 无 | — |

## 变体走查

- **输入**（codec）：空 payload（nil→nil，nil vectors ✅）｜NaN/±Inf（NaN-ness 断言 ✅）｜负数/-0.0（✅）｜多向量混合空向量（✅）｜float64 超 float32 精度（InDelta 1e-8 契约断言 ✅）｜截断/尾部垃圾（6 分支 ✅）
- **前置**（迁移）：表不存在（新库，跳过 ✅ 代码分支+测试 setup 变体）｜空表｜有 3 类行（可转×2、NULL×1 ✅）｜坏 jsonb 行（降级 NULL+warn，代码分支）｜列已 bytea（幂等跳过 ✅）
- **时间窗口**：不适用（无时间窗口逻辑，TTL 清理既有行为不变）——划除
- **幂等**：迁移双跑 ✅（6）｜缓存 upsert ON CONFLICT 既有 ✅
- **可用性**：无 UI 改动——划除（前三项不适用）

## 效果核对

| 效果 | 触发 | 方法 | 量化预期 | 结论挂钩 |
|---|---|---|---|---|
| otel_spans 日增 | 部署后 24h | 验证 6.5 SQL | 从 ~82 万行/天降约一个数量级 | 采样生效 |
| 缓存行体积 | 新缓存写入后 | 验证 6.3 | ≈10240 B/行（原 ~31.5KB） | bytea 生效 |
| 库稳态体积 | 部署一周后 | `pg_database_size` | otel_spans 稳态 ~4.3GB→~0.3GB + 缓存 1.7GB→0.55GB | 综合降幅过半 |

## 白盒附加（复杂档：codec 解析 + 迁移状态分支）

**codec decode 分支表**：

| 分支 | 输入 | 期望 | 用例 |
|---|---|---|---|
| B1 空输入 | len=0 | (nil,nil) | RejectsTruncated/empty |
| B2 count 头截断 | len<8 | truncated | RejectsTruncated/count |
| B3 count 溢出 | count > len/8 | truncated（G115 上界防护） | 代码走查+上界断言 |
| B4 dim 头截断/溢出 | pos+8>len 或 dim>(len-pos)/4 | truncated | RejectsTruncated/dim、float |
| B5 尾部垃圾 | pos≠len | truncated | RejectsTruncated/trailing |

**迁移分支表**：

| 分支 | 条件 | 动作 | 用例 |
|---|---|---|---|
| M1 新库 | 表不存在 | return（AutoMigrate 建 bytea 新表） | PreservesRows setup 反向 |
| M2 已迁移 | 列类型≠jsonb | return | Idempotent |
| M3 转换 | 列=jsonb | ADD 列→回填→DROP→RENAME（单事务+lock_timeout） | PreservesRows |
| M4 坏行 | json.Unmarshal 失败 | 该行 NULL+warn，不中断 | 代码分支（合成坏行测试略，风险=丢一行缓存可接受） |
| M5 AutoMigrate 竞态 | 无——pre-migrate 先于 RunAutoMigrate，无并发窗口 | — | 启动顺序本身由主链 5 覆盖 |

**边界值**：count=0（合法空载荷）｜dim=0（空向量合法）｜float32 精确值断言用 2 的幂分数（0.5/0.25/0.75/1.5/-0.5）｜超出范围 dim/count 拒绝。
