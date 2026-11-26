# Review：fix-embedding-model-column-width

> 审查者：子线程（change-review-glm 档，只读审查制品 + 实现 diff）。
> 审查日期：2026-11-03。审查范围：change 制品（proposal / ui-design / design / tasks / 两份 delta spec）+ 实现 diff（`20261103_0001` 迁移、5 处 gorm tag、新增测试文件）。
> 审查结论总评：修复方向与四件套（列宽 / tag / 迁移 / 测试）一致，迁移在 PG 18.4 实测「不重写表体、不截断数据、重复执行无错」，8 个 Scenario 全有测试映射，**无「算法错」级缺陷**；但有 4 条 Medium 需处置，另由主线程复核发现 1 条 High 级回归（见 R-9）。

---

### R-1 2026-11-03【Medium】迁移无条件 ALTER + 取锁失败即阻断后端启动

- **状态**：已修复
- 审查发现：`Up` 对 5 张表无条件发 `ALTER TABLE … ALTER COLUMN model TYPE VARCHAR(100)`，即使列已是 100（生产当前就是 100；Phase 1 AutoMigrate 已先加宽）。每条 ALTER 要 `ACCESS EXCLUSIVE` 锁，且 `lock_timeout=5s` 到期 → 迁移报错 → `InitDB` 错误 → `main.go` `Fatalf` → 后端拒绝启动。审查实测：即使列已是 `varchar(100)`，重复执行该 ALTER 仍会**重建**该列上的索引（表 relfilenode 不变）。
- 处置：改为**条件执行**——列不存在跳过、列宽 ≥ 100 跳过（真 no-op：不 ALTER、不取锁、不重建索引）、列宽 < 100 才 ALTER；`withLockTimeout` 守卫保留在真正需要 ALTER 的分支上。落点：`backend-go/internal/platform/database/postgres_migrations.go` 的 `embeddingModelColumnWidthMigration()`。

### R-2 2026-11-03【Medium】lock_timeout 守卫落在了无效的一层（真正执行加宽的是无守卫的 AutoMigrate）

- **状态**：已采纳
- 审查发现：Phase 1 `RunAutoMigrate` 在 Phase 2 迁移之前，且它自己就会按新 tag 发出同一条 ALTER，而 **AutoMigrate 路径没有任何 lock_timeout 守卫**（无限等锁）；Phase 2 带 5s 守卫的迁移跑到时列已是 100，守卫护的是第二次冗余 ALTER。
- 处置：**不改代码，只改文档口径**。理由：给 AutoMigrate 路径加 `lock_timeout` 要动共享的 `db.go`（影响全库所有表的 AutoMigrate ALTER），影响面远超本 change，属越界；且本 change 已把 Phase 2 迁移改成条件 no-op（R-1），Phase 2 不再额外持锁。改为在 design「Risks」里如实写明「Phase 1 AutoMigrate 的无守卫 ALTER 是主风险，5s 守卫只覆盖 Phase 2 的条件 ALTER」，并在 proposal 的部署说明里补「选 embedding worker 空闲窗口部署」。
- 遗留（不阻塞归档）：「Phase 1 AutoMigrate 无 lock_timeout 守卫」是全库级既存问题（任何 size 变更的列都会遇到），归属另立 change，本 change 只在自己的 design 里如实标注。

### R-3 2026-11-03【Medium】列宽不变式测试只覆盖硬编码的 5 张表——「将来新增窄列即红」不成立

- **状态**：已修复
- 审查发现：测试断言的是一份写死的 5 张表清单；将来新增 `xxx_embeddings.model VARCHAR(50)`，测试照样绿，且测试清单是迁移清单的**手工副本**（迁移加到第 6 张表，测试不会报错）。
- 处置：不变式改为**数据驱动**——查 `information_schema.columns` 全库 `column_name='model'` 且有长度上限的列，逐张断言 ≥ `ai_models.name` 宽度；同时把迁移的表清单通过 `export_test.go` 导出给测试，消除副本漂移（断言「迁移清单 == 测试覆盖清单」）。落点：`backend-go/internal/platform/database/embedding_model_column_width_test.go`、`backend-go/internal/platform/database/export_test.go`。

### R-4 2026-11-03【Medium】「存量行原样保留」断言无鉴别力 + spec 承诺的「向量原样保留」无断言

- **状态**：已修复
- 审查发现：(a) 播种的存量值是 15 字符短名，**会截断的实现同样能让该断言通过**——证明不了「加宽不截断」；(b) spec 的 Scenario 写的是「`model` 值与 `embedding` 向量原样保留」，而 5 张表的播种行一律不写 `embedding`，向量这一半零断言（spec 与实现之间有未兑现的断言缺口）。
- 处置：存量值改为**恰好 50 字符**（旧列宽上限，对截断敏感）+ 保留 61 字符长名行，迁移后逐字节断言；向量部分改为插入**真实向量**（运行时读 `format_type` 拿列的实际维度，用 `testutil.PadVector` 造同维度向量），迁移前后断言 `embedding::text` 逐字节相等。

### R-5 2026-11-03【Low】`//nolint:gosec` 是无效抑制

- **状态**：已修复
- 审查发现：gosec G201 只覆盖 `database/sql` 的 `DB/Conn/Tx.Exec`，gorm 的 `tx.Exec` 不在其调用表内；且其 SQL 关键字模式也不匹配 `ALTER TABLE …`。该抑制既非必需、又会在将来 SQL 含 `UPDATE … WHERE` 时静默压掉真实告警。
- 处置：删除该 `//nolint:gosec`（删前先跑 lint 确认仍 0 issues）。

### R-6 2026-11-03【Low】`tableExists` 只守「表不存在」，不守「列不存在」

- **状态**：已修复
- 审查发现：表存在但 `model` 列不存在时 ALTER 报 `column "model" does not exist` → 迁移失败 → 启动失败。
- 处置：与 R-1 合并处理——条件执行时同时校验**列存在且有长度上限**，列缺失跳过（列的创建由 Phase 1 AutoMigrate 负责）。

### R-7 2026-11-03【Low】design / proposal 的规模前提与生产实际不符

- **状态**：已修复
- 审查发现（审查方生产只读实测）：`candidate_embeddings` 实际 **3099 行**（design 写「当前 0 活跃行」）；`topic_tag_embeddings` 的 868 MB 是**含两个 HNSW 索引的 relation 总大小**，堆（heap）只有 10 MB。按「0 活跃行」估锁持有时间会低估。
- 处置：design「Risks」与 proposal「Impact → 数据」改成实测值，并注明 868 MB 主要是 HNSW 索引占用。

### R-8 2026-11-03【Low】文档的 `NOT NULL` 与实现不符

- **状态**：已修复（归因纠正，详见 R-9）
- 审查发现：`docs/reference/database/tables/topic-tags.md` 写 `topic_tag_embeddings.model` 是 `NOT NULL`，而生产 `information_schema` 显示 `is_nullable = YES`；审查方判断为「既有偏差（改宽前就写 NOT NULL）」。
- 主线程复核**推翻**了「既有偏差」的归因：迁移前的 `\d topic_tag_embeddings` 快照显示该列**确实是 NOT NULL**（由迁移 `20260723_0001` 用 `ensureNotNullDefault` 固化），是**本 change 引入的回归**——详见 R-9。文档维持 `NOT NULL` 表述（修复后即与库一致），并在 R-9 修复落地后复核确认。

### R-9 2026-11-03【High】本 change 的 gorm tag 变更触发 GORM AutoMigrate 顺带 DROP NOT NULL（生产约束被静默丢弃）

- **状态**：已修复
- 主线程复核新增（review 未覆盖归因）：`topic_tag_embeddings.model` 的 NOT NULL **是本 change 破坏的**。机制链（已逐环实证）：① 迁移前该列 NOT NULL（`20260723_0001` 的 `ensureNotNullDefault` 固化，仓库自己的 `constraints_test.go` 也断言 `{"topic_tag_embeddings", "model", true, ""}`）；② 本 change 把 tag 从 `size:50` 改成 `size:100` → Phase 1 AutoMigrate 判定列宽不符 → 发出 ALTER → **gorm.io/driver/postgres v1.5.11 `AlterColumn`（migrator.go:370 附近）在 `field.NotNull == false` 时执行 `ALTER COLUMN … DROP NOT NULL`**（本仓库约定「not null 不写进 tag、由迁移固化」→ `field.NotNull` 恒 false）；③ 我们的 `20261103_0001` 只改列宽、不碰空值性 → 约束就此丢失。生产实测确认：`pg_attribute.attnotnull = false`（迁移前为 true）；`NULL` 行 0、`''` 行 0；其余 4 张表的 `model` 列**本来就是可空**（迁移前 `\d` 快照已确认），未受影响。既有 `constraints_test.go` 之所以没红：它在全新 testcontainer 上建库（列由 AutoMigrate 新建 + `20260723_0001` 固化），测不到「存量列被 ALTER」这条路径。
- 处置：新增迁移 `20261103_0002`，用同文件既有的 `ensureNotNullDefault(db, "topic_tag_embeddings", "model", "''")` 恢复 NOT NULL（幂等；生产无 NULL 行，回填为空操作）。**不**给 gorm tag 加 `not null`——本仓库约定是 tag 不写 not null、约束由迁移固化（`20260723_0001` 的 Description 明示），加 tag 会违背约定。测试侧补断言：`topic_tag_embeddings.model` 在迁移后必须仍是 NOT NULL（把「tag 变更顺带改约束」这条路径纳入回归网）。
- 遗留观察（不阻塞归档）：任何**存量列**的 size 变更都会踩同一个 GORM 行为（ALTER 顺带 DROP NOT NULL），影响面是全库所有被 `20260723_0001` 固化过 NOT NULL 的列。归属另立 change；本 change 的 design「Risks」已如实标注。

---

## 审查方原始报告要点（保留追溯）

- **无发现**：design D1~D6 的「落点」全部四选一可解析（无空泛表述、无 basename 歧义、无「可删项：无下游承接」的决策）；迁移的数据安全性（加宽不截断、不删行、存量有行的表上跑通）；gosec 豁免的事实部分（表名来自包内常量）；spec ↔ 实现一致性（8 个 Scenario 全有映射，标题一字不差，`scenario-trace.sh` 机器对账通过）；文档一致性（三处列宽均写 100，全库扫描无遗漏的窄 `model` 列）。
- **审查方已实跑**：`go test -count=1 -run 'TestEmbeddingModelColumnWidth'`（2 用例 PASS，日志中如期出现 `SQLSTATE 22001` 复现断言）、`golangci-lint`（0 issues）、`go vet`（退出码 0）、`scenario-trace.sh`（8/8 通过）、`check-tasks-tail.sh`（通过）、`check-standards.sh --change`（257/0）、`doc-impact.sh verify`（通过）；生产库全程只读 SELECT；一次性本地 PG 18.4 容器验证「varchar 长度变更不重写表体、含该列的索引会重建」。
- **审查方读码推断（未实跑）**：把 `size:100` 改回 `size:50` 测试会红（推断路径：golden schema AutoMigrate 建为 50 → 迁移拉到 100 → tag≠DDL 断言失败）；「新增第 6 张窄表不被现有测试抓住」（读码推断，本轮已修）；「锁等不到 ⇒ 后端拒绝启动」的完整链路（读码确认 `InitDB` → `Fatalf`，未在生产构造锁冲突）。
