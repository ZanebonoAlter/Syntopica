# 后端代码规范（Code Style）

> **权威源**：本文件是后端代码规范的唯一权威。`backend-go/AGENTS.md` 的 Backend-Specific Conventions 红线要点深链指向本文件。
> 包结构见 [package-layout.md](./package-layout.md)。

## 格式与静态分析

- 使用 `gofmt` 格式化
- 静态分析 `golangci-lint run ./...`（配置与启用 linter见 [lint.md](./lint.md)）
- 不要新增 linter / formatter / 其他工具，除非明确要求

## 导入顺序

```
标准库

第三方库

项目内部包
```

## 命名

- 导出符号 `PascalCase`，私有符号 `lowerCamelCase`
- 包名简短

## 错误处理

- 用 `fmt.Errorf("context: %w", err)` 包装
- **禁止 `panic` 处理错误**
- 校验参数再碰 DB；错误早返回（early return）

## 响应与序列化

- Handler 响应格式：`gin.H{"success": bool, "data"|"error"|"message": ...}`
- JSON struct tag 使用 `snake_case`

## 日志

- 优先复用 `internal/platform/logging`
- 避免继续用裸 `log.Printf` + 文本前缀人工区分级别

## 业务归属

- 业务逻辑按 domain 组织在 `internal/<domain>/service/`（domain 白名单见 package-layout.md）
- HTTP 路由注册在 `internal/app/router.go`
- Handler 不写复杂业务，不直接访问 DB

## GORM model tag 与迁移

**显式迁移管的表，model tag 不写 `not null`——让显式迁移（`postgres_migrations.go`）唯一管 NOT NULL 约束，AutoMigrate 不重复施加。** 但 **`default:` tag 是例外，必须保留**（见下「default tag 不可删」）。

**为什么禁 `not null`**：model tag 写 `not null` 会让 AutoMigrate 启动时尝试 `ADD COLUMN ... NOT NULL`，与显式迁移的"ADD NULL → 回填 → SET NOT NULL"三步竞争；在有历史数据的库上 AutoMigrate 先失败（`column ... contains null values`），污染启动日志（`ai-call-logging-schema` 的事故教训）。

**⚠️ default tag 不可删（2026-07-23 事故教训）**：`default:xxx` 不仅是 DB 约束，还**控制 GORM 的零值省略行为**——有 `default:` tag 的字段，GORM 在 Go 零值（`""`/`0`/`false`）时**省略该列**，让 DB DEFAULT 生效；没有 `default:` tag 时，GORM **显式插入零值**，覆盖 DB DEFAULT。曾一次性删除 `topic_tags.status` 的 `default:active` tag，导致新建 tag 被写成 `status=""`（而非 DB DEFAULT 的 'active'），使 `getBoardArticles`（按 `status='active'` 过滤）查不到相关 tag 和文章。**结论：标量字段（string/int/bool）的 `default:` tag 是功能必需，不可移除**；NOT NULL 由显式迁移兜底即可（见 `postgres_migrations.go` 迁移 + `constraints_test.go` 约束断言）。

**正确做法**：
- model tag：`gorm:"size:20;default:active;index"`（类型 + **default 保留** + 索引 + json），不写 `not null`
- DB 约束（NOT NULL/CHECK）：写在显式迁移；DEFAULT 由 `default:` tag + 迁移双保险
- "必填"语义：靠代码入口校验（如 `Router.Chat` 强制 `Operation != ""`），不靠 model tag 反射

**JSONB 列空值**：非指针 `string` 字段写 `gorm:"type:jsonb"` 列时，零值 `""` 不是合法 JSON——入库前用 `db.Omit("col").Create()` 跳过空值列（DB 置 NULL），或改 `*string`/`datatypes.JSON`。`serializer:json` 的集合字段（如 `[]string`/`map`）必须保留 `default:'[]'`/`default:'{}'`（同 default tag 零值省略原理）。详见 testing.md「JSONB 列空串陷阱」。

## 迁移编写规范（`postgres_migrations.go`）

显式迁移结构体 `Migration` 除 `Version`/`Description`/`Up` 外，还有两个声明性字段 + 一个锁守卫 helper，用于应对事务兼容性与大表锁表风险：

**`RunOutsideTx bool`——仅用于单条事务不兼容 DDL**。默认 false：`Up` 与版本记录共用一个事务（原子提交/回滚，失败下次重试）。设 true 时 `Up` 在裸连接上跑（无外层事务），成功后单独记录版本——**这是 `CREATE INDEX CONCURRENTLY` 等事务内必报错（SQLSTATE 25001）操作的唯一出路**。⚠️ 只用于单条事务不兼容语句；需要原子性的多步操作（ALTER → UPDATE → DROP）必须留在事务内（默认路径）。事务外 `Up` 失败不记录版本（下次重试），因此迁移自身必须幂等（`IF NOT EXISTS` / 开头清残留），尤其 CONCURRENTLY 失败会留 INVALID 索引，闭包应先 `DROP INDEX IF EXISTS` 再建。

**`Down func(db *gorm.DB) error`——声明性占位，nil = 不可逆**。当前无回滚执行器（无 CLI/HTTP 入口，按 AGENTS.md「Simplicity First」不预先实现）。破坏性迁移（TRUNCATE/DROP）`Down` 留 nil，**在 `Description` 末尾标注「⚠️ 不可逆 TRUNCATE（破坏性，受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫；生产执行前需备份）」**——TRUNCATE 数据物理不可恢复，写"恢复 DDL"是假象，诚实标注比假回滚好。

**`withLockTimeout(db, timeout, fn)`——长锁 DDL 必须用**。`ALTER COLUMN TYPE`（全表重写）和 `ADD CONSTRAINT UNIQUE`（扫全表验证）会拿 AccessExclusiveLock，大表上无限阻塞写入。helper 用 `SET LOCAL lock_timeout`（事务内有效，结束自动复位，且有防御性显式复位防连接池泄漏）包裹语句，超时让语句失败而非无限阻塞——**有意的安全收紧**。默认 timeout `"5s"`。用法：

```go
if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
    if err := tx.Exec("ALTER TABLE t ALTER COLUMN c TYPE vector(4096)").Error; err != nil {
        return fmt.Errorf("alter column: %w", err)
    }
    return nil
}); err != nil {
    return err
}
```

短操作（小表 `CREATE INDEX IF NOT EXISTS`、CHECK 约束的 `DO $$ ... END $$`、单行级联 FK）不需要守卫——统一注入会误杀正常路径。

## 日报聚类（topicgraph）代码规约

日报归属采用**泳道驱动**（lane-driven，`daily-report-lane-driven-clustering`）：当天 tag 按 topic **质心**距离分 L1/L2/L3 三桶，LLM 退化为弱区裁决。改 `internal/topicgraph/` 聚类 / 归属代码前必读，违反会重建形态 4 错锚根因。

**分层归属（铁律）**：
- 归属逻辑在 **repository 层**（`daily_report_assignment.go` 的 `planTopicAssignments` / `assignAndUpdateTopics`），**不在 service**。service 只负责分桶（`daily_report_lane.go`）+ 编排（`daily_report_orchestrator.go`）。
- 归属路由键是 `section.LaneTier` + `section.MatchedTopicID`（由上游分桶设定）：`l1_direct`/`l2_llm` 且有 `MatchedTopicID` → `anchor_hit`；`l3_new` 或无 `MatchedTopicID` → `auto_new`；section 无 embedding → `unmatched`。**旧的「embedding AND-gate 双重确认」已移除，不要再加回来。**

**质心是匹配锚点（取代首义向量）**：
- topic 的匹配锚点 = `board_persistent_topics.centroid`（近 `centroid_window` 默认 30 条 section embedding 均权平均），`ComputeTopicCentroid` 计算；section<2 退化首义向量（`embedding` 字段）。
- `embedding`（首义）字段**保留**作退化兑底，不要删。`topicAnchorVec` 优先 centroid、退化 embedding。
- SaveReport 提交后异步刷新质心：`UpdateCentroidOnSectionChange`（事务外、读 r.db、失败仅告警不阻断保存）。

**吸尘器降级**：质心过宽的 topic（`strong/(strong+mid) < vacuum_ratio`）标 `is_vacuum=true`；挂到它的 tag 从 L1 降级 L2 交 LLM 裁决。`RecomputeVacuumStats` 在 SaveReport 提交后按 `vacuum_window`（默认 7 天）重算。

**LLM 只处理 L2/L3**（`ClusterTagsLane`）：L1 直挂不调 LLM；L2 在 top-K（`l2_candidate_k` 默认 5）候选上做「留/换/新」，target 不在候选集降级 new；L3 起新叙事。遗留 `ClusterTags`（全量自由聚类）**仅调试 CLI `cmd/verify-cluster-prompt` 使用**，生产管线不走。

**section embedding 不可删**：用途从「匹配 topic」变为「更新 topic 质心 + `computeThreadFitDistances`」，计算入口仍在 orchestrator（`cluster_label` 文本向量）。

**配置集中**：6 个阈值在 `PersistentTopicConfig`（`daily_report_topic_repository.go`）+ `DefaultPersistentTopicConfig`（默认值）+ `LoadPersistentTopicConfig`（从 `ai_settings` 加载，缺失降级默认），迁移 `20260727_0001` seed。新增阈值走这三处 + 迁移 seed，不要散落代码常量。

## Anti-Patterns（硬禁）

- ❌ `router.go` 里写业务逻辑
- ❌ Handler 直接访问 DB
- ❌ `panic` 处理错误
- ❌ 在 service 层重写日报归属逻辑（归属在 `repository/daily_report_assignment.go`）
- ❌ 删 section embedding 或 topic centroid/首义向量（质心是匹配锚点，embedding 是退化兑底 + thread fit 源）
- ❌ 给新 section 不设 `LaneTier` 就调 SaveReport（新契约要求分桶阶段设 lane_tier；历史 section lane_tier=NULL 视为旧数据）
- ❌ **在事务闭包内调慢 I/O（外部 AI API / 长网络往返 / 大结果集全量拉）**：事务开着等 HTTP 响应即 `idle in transaction`，持 xmin 阻塞 vacuum、与 autovacuum 调优直接对抗（fix-aux-label-idle-tx-cache：辅助标签入库曾因此在单事务内逐标签调 embedding，生产实测 4–40s）。正确形态是两阶段——事务外预计算（LLM embedding / 批量向量化，参考 `auxlabel.planAuxiliaryLabels` 与 `CompositeLabelService.PlanCompositeLabel`），短事务内纯写（`executeAuxiliaryPlans` / `ApplyCompositeLabel`）；`pg_stat_activity` 出现秒级 idle in transaction 即违本条。兜底防线：`docker-compose.pg.yml` 的 `idle_in_transaction_session_timeout=60s` 会掐断违例事务（日志可见 terminating connection due to idle-in-transaction timeout）。

## 资料来源

收敛自原 `backend-go/AGENTS.md`（Backend-Specific Conventions / Anti-Patterns）与 `development.md` §后端代码风格、《开发执行规范》§4.4。
