
## 实体归一研究基线与初版验收边界

用户已批准方向：LLM提取主谓宾主体提及→embedding候选→LLM只选统一实体ID/none；合法none直接新建，不设待处理队列、不做人工种子表、不做后缀/词表启发式。全局实体身份跨board复用，事件线仍按board及事件归属，entity相同不等于story相同。现存aux全局池有label-only merge_embedding与label+description embedding，但非实体权威表，alias可由相似度自动添加，不能视作同体金标。旧Qwen研究run qwen-eval-20261109-181947：60人工选候选、人工query短说明、11候选描述整理；非自匹配6题name-only Top10 6/6、name-desc 6/6，已有实体选择15/15、none4/5。Q20误选Federal Reserve，prompt含“可能对应就选最可能”强选偏置；应问严格同一实体。Q09换序选不同aux_id别名都语义正确，证明需稳定entity ID不能直接aux ID。小测25Chat授权已消耗完，不自动扩大真实模型批量调用。用户要求新初版重点看效果：验收必须真实文章→真实第一轮抽取，不用人工理想说明/修过的候选宣称生产效果；分开测误合、漏召回、重复新建、换序、跨板块和耗时/用量。默认不重写旧tags/stories、不批量真实重归属；先隔离/影子对照，后续启用需效果证据。

**引用**：docs/research/subject-entity-resolution/evaluation/main-review-qwen-eval-20261109-181947.md、docs/research/subject-entity-resolution/explore-findings.md、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go、backend-go/internal/topicgraph/service/story_subject_normalize.go

<!-- pinned 2026-11-09T12:43:51Z -->

## 会话级咨询锁既有实现范式（D6 直接可抄）

D6 要的「PostgreSQL 会话级咨询锁 + 独占独立连接」在仓库里已有成熟范式：`backend-go/internal/topicgraph/service/generation_gate.go`。

关键点（照抄这套纪律，不要另创）：
- 取专用连接：`sqlDB := g.db()` → `conn, err := sqlDB.Conn(ctx)`（`*sql.Conn` 是**从池里 checkout 的槽位**，必须 Close 归还，否则泄漏池槽）。
- 加锁：`conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked)`；已有 `pg_try_advisory_lock` 用法先例（story_backfill.go:125 也有）。
- **探活失败的危险处理**：超时可能「客户端超时但服务端已授予锁」→ 直接 Close 会把带锁的健康 session 还回池，造成永久泄漏。既有代码的做法是 `killSession(ctx, conn)` 终止该 session（死 session 自动释放 advisory lock，池会丢弃坏连接），然后再 `conn.Close()`。
- 释放：`release()` 取 timeout ctx → `pg_advisory_unlock` → Close 归还；解锁失败只记日志不重试（死连接已自动释放锁）。
- 进程内 `atomic.CompareAndSwap` 只作本地快速短路，**不是**跨进程保证。

本 change 需要的是**单一全局锁键**（D6 明确否决「按规范化提及名分片」），串行化「最新池检索 → 二判 → 原子登记」，第一轮抽取在闸外，DB 事务只包写库段。

**引用**：backend-go/internal/topicgraph/service/generation_gate.go:80-160、backend-go/internal/topicgraph/service/story_backfill.go:125

<!-- pinned 2026-11-09T14:07:39Z -->

## 隔离 PostgreSQL 测试基建（仓储层/闸门测试唯一允许的库）

仓储层与闸门测试**必须**用隔离 PG（testcontainers），禁止 SQLite、禁止写真库。基建已就绪：

`backend-go/internal/platform/testutil/testutil.go`
- `OpenTestDB(t)` / `SetupTestDB(t)` — testcontainers-go postgres 模块起容器并跑迁移
- `ReimportTestDB(t, db)`、`ResetTestData(t, db)`、`TruncateAllTables(t, db)`
- `runTestMigrations(t, db)` — 走真实 `postgresMigrations()` 链，因此**新表迁移注册后测试容器里会自动建表**
- `PadVector(vec, dim)` — 向量维度对齐辅助

依赖已在 `backend-go/go.mod`：`github.com/testcontainers/testcontainers-go/modules/postgres v0.42.0`、`jackc/pgx/v5`、`gorm.io/gorm v1.25.12`。

用法先例：`internal/tagmanagement/service/core/edge_gc_test.go`、`event_tag_anchor_test.go`、`internal/reader/service/content_form_pg_test.go`。

注意：容器启动慢，测试里用 `-short` 跳过或复用既有 `SetupTestDB` 缓存策略；树莓派上**只跑影响包**，别全量。

**引用**：backend-go/internal/platform/testutil/testutil.go:95-260、backend-go/internal/tagmanagement/service/core/edge_gc_test.go

<!-- pinned 2026-11-09T14:07:39Z -->

## 冲突面：待改文件已被并发 change 修改（纯增量纪律）

工作区有 190 个脏文件来自 9 个并发活跃 change，本 change 要动的文件**已被别的 change 改过（未提交）**，实现必须纯增量、绝不回退：

1) `internal/tagmanagement/service/core/extractor_section.go`（脏，来自 enable-story-cross-day-continuation）
   - 已新增 `EventFingerprintSchema() *airouter.JSONSchema`（五字段 subject/subject_key/action/object/occurred_at 的**唯一真相源**）
   - `sectionExtractionSchema()` 现在这样拼：先建 base schema，再 `for name, property := range EventFingerprintSchema().Properties { schema.Properties["tags"].Items.Properties[name] = property }`
   - 提示词里有「事件指纹（仅 event 标签输出…）」段；正例 JSON 已带 subject/subject_key/action/object
   - `parseSectionTags` 里已调 `eventFingerprintFromRaw(t, cat)` + `fingerprint.Complete()` 校验 → `ExtractedTag.Fingerprint`
   → 本 change 要在**影子态**额外加 `subject_desc`：不能塞进 `EventFingerprintSchema()`（那会把新字段焊进 event_fingerprint），应另建影子态专属 schema 分支/Properties 追加，且 `off` 态拼出来的 schema 与提示**逐字不变**。

2) `internal/tagmanagement/service/core/tagger.go`（脏）
   - 已新增 `eventFingerprintMetadataKey = "event_fingerprint"`、`persistEventFingerprint`、`fingerprintMapComplete`
   - persistEventFingerprint 的纪律是「读 DB metadata 真值再合并写，先写者赢」
   → 本 change 写 `metadata.entity_subject` 必须沿用同一「读-改-写单键」纪律，**不得盲写整个 metadata**。

3) `internal/platform/database/postgres_migrations.go`（脏，+338 行，来自其它 change）
   → 本 change **只加一行** `migrations = append(migrations, entitySubjectMigration())`（append 区块在 2473–2490 附近，函数 `postgresMigrations()` 起于 282 行）；新迁移体放**独立文件** `internal/platform/database/entity_subject_migration.go`，返回 `Migration`。

4) `internal/tagmanagement/models/migration.go`
   - 该文件已有 `SeedEmbeddingConfigs`、`RunAuxLabelDupMerge` 等**包级函数式**迁移钩子（不是 Migration 结构），新模型注册按 `entity_subject.go` 里自带 `RegisterModels(db)` 的方式更贴合任务书（T1 明确写 models 里新增 `RegisterModels`）。

5) 配置：viper + `internal/platform/config/config.go` 的 `Config` 结构 + `LoadConfig` 里 `viper.SetDefault(...)`（如 `server.port`、`database.postgres.*`）。三模式开关按同一范式加 `entity_subject.mode`（默认 `off`，`active` 显式报错）。

**引用**：backend-go/internal/tagmanagement/service/core/extractor_section.go、backend-go/internal/tagmanagement/service/core/tagger.go、backend-go/internal/platform/database/postgres_migrations.go、backend-go/internal/tagmanagement/models/migration.go

<!-- pinned 2026-11-09T14:07:47Z -->

## 评测 CLI 可复用脚手架 + 真实调用形态

T4 评测 CLI 有现成参考脚手架（研究阶段遗留、未入库、可读可抄不给 git）：

`backend-go/.tmp-entity-resolution-eval/`（2.7MB，untracked）
- `guard.go` — **发送前预算拦截**的现成范式（research runner 的 guard 经验，D11 明确要求同款）
- `embed.go` / `llm.go` — 真实 Chat/Embed 调用与计数
- `plan.go` — 预算按阶段机械列清单
- `audit.go` — 调用审计
- `fixture.go` / `index_test.go` — 夹具与自测
- `scripts/` — fetch_contexts.py / build_fixture.py / render_results.py（Python 侧，用 uv 跑）
- `data/` — contexts*.json、aux_pool.tsv

关键路由事实（D11 取舍依据，别再算 ceil(text/32)）：
- `Router.Chat` — `internal/platform/airouter/router.go:210`，按 provider 列表降级、全失败 `errors.Join`
- `Router.Embed` — `router.go:482`，**只有 operation 在缓存白名单（router.go:446-451，现仅 2 个）时才批量**，否则**一条输入一次 HTTP**（router.go:502）。代码里**不存在 32 条一批的切分**。
- 评测专用 transport 包装只在评测入口注入，**不改生产路由架构**；无法做发送前计数就预先阻断。

本轮**不真跑评测**（无模型额度授权），只实现 CLI + fixture 测试。

**引用**：backend-go/.tmp-entity-resolution-eval/guard.go、backend-go/.tmp-entity-resolution-eval/embed.go、backend-go/.tmp-entity-resolution-eval/plan.go、backend-go/.tmp-entity-resolution-eval/audit.go、backend-go/internal/platform/airouter/router.go

<!-- pinned 2026-11-09T14:07:47Z -->
