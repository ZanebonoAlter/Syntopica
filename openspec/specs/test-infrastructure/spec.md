<!-- domain: dev -->

# Test Infrastructure

## Purpose

Provide a shared Go test utility package that gives integration tests access to a real Postgres database (with pgvector), replacing per-test SQLite setup. Define test tier conventions and file naming standards so developers can run fast unit tests or full integration tests with a single command.
## Requirements
### Requirement: testutil package with OpenTestDB — isolated testcontainer

系统 SHALL 在 `backend-go/internal/platform/testutil/` 提供 `OpenTestDB(t *testing.T) *gorm.DB` 函数，通过 testcontainers-go 启动一个隔离的 pgvector Postgres 容器（镜像 `pgvector/pgvector:pg18-trixie`）并返回连接到该容器的 `*gorm.DB`。该函数**不得**读取任何环境变量、**不得**存在默认 DSN、**不得**连接到 `docker-compose.pg.yml` 运行的数据库（那是生产数据所在）。容器由 Testcontainers Ryuk sidecar 在测试进程退出时自动销毁。

#### Scenario: 启动隔离容器并返回有效连接

- **WHEN** Docker daemon 可用且 `OpenTestDB` 被调用
- **THEN** 启动一个隔离的 pgvector 容器，返回连接到该容器的 `*gorm.DB`，该容器与开发库（`localhost:5432/syntopica`）完全隔离

#### Scenario: Docker 不可用时测试立即失败

- **WHEN** Docker daemon 未运行且 `OpenTestDB` 被调用
- **THEN** `t.Fatalf` 被调用，错误消息提示需要 Docker

#### Scenario: 无任何环境变量可重定向到生产库

- **WHEN** 设置任何环境变量（包括 `TEST_DB_DSN`）
- **THEN** `OpenTestDB` 忽略它，仍然启动隔离容器——这是防止测试连到生产库的安全保证

### Requirement: testutil package with SetupTestDB — single entry point

系统 SHALL 提供 `SetupTestDB(t *testing.T) *gorm.DB` 函数，作为唯一测试 DB 入口。该函数的行为分两阶段：

1. **首次调用（进程内）**：建立"黄金 schema"——调用 `OpenTestDB` 启动/复用单例 pgvector 容器，执行一次 `runTestMigrations`（生产 `RunAutoMigrate` + `RunMigrations`，含迁移注入的 seed 数据），结果缓存为进程级黄金 schema。
2. **后续调用（进程内）**：**不再重建 schema**，改为调用 `ResetTestData`（seed 保护的 truncate）清空业务数据、保留迁移 seed，使每次调用后数据库状态等价于"生产首次启动后的状态"。

每次调用 SHALL 设置 `database.DB = db` 并返回 `*gorm.DB`。函数首行 SHALL 执行 `testing.Short()` 跳过守卫（见"测试分层"requirement）。

#### Scenario: 首次调用建立黄金 schema（含 seed）

- **WHEN** 进程内首次调用 `SetupTestDB`
- **THEN** 启动/复用单例 pgvector 容器，执行一次生产 AutoMigrate 与版本迁移（含 seed）
- **AND** 返回的 `*gorm.DB` 所有 domain model 表已创建（含 `topic_tag_embeddings.embedding` vector 列），迁移 seed 数据（`ai_settings`、`embedding_config`）已就位

#### Scenario: database.DB 全局变量被设置

- **WHEN** `SetupTestDB` 被调用（首次或后续）
- **THEN** `database.DB` 被设为同一 `*gorm.DB` 实例，被测代码通过 `database.DB` 读取时能正常工作

#### Scenario: 后续调用走轻量重置而非重建 schema

- **WHEN** 同一进程内 `SetupTestDB` 被第 N 次（N>1）调用
- **THEN** 不执行 `DROP SCHEMA` / `CREATE SCHEMA` / `runTestMigrations`
- **AND** 仅执行 `ResetTestData`（truncate 业务表，保留 seed），返回连接到同一黄金 schema 的 `*gorm.DB`

#### Scenario: 后续调用后状态等价于生产首次启动

- **WHEN** `SetupTestDB` 后续调用完成
- **THEN** 业务表数据为空，迁移 seed 数据（`ai_settings`、`embedding_config`）从快照恢复为 seed-only 状态
- **AND** 数据库状态等价于生产首次启动后的 schema + 默认数据

### Requirement: 测试数据库使用生产初始化逻辑

系统 SHALL 使用生产的 `database.RunAutoMigrate` 和 `database.RunMigrations` 初始化隔离的测试数据库。生产迁移 SHALL 是测试 schema、约束和系统默认数据的唯一来源。黄金 schema 在进程内只构建一次；需要"干净迁移态"的少数回归测试 SHALL 显式调用 `ReimportTestDB` 逃生口。为维持"测试库跑全量生产迁移（含破坏性 TRUNCATE 清理迁移）"的不变量，测试迁移路径 SHALL 在执行前显式开启破坏性迁移开关（`t.Setenv("MIGRATIONS_ALLOW_DESTRUCTIVE","1")`），与生产默认拒绝破坏性迁移的安全收紧对齐。

#### Scenario: 黄金 schema 首次构建

- **WHEN** 进程内首次调用 `testutil.SetupTestDB`
- **THEN** 系统在 testcontainers-go 创建的临时 Postgres 中执行生产 AutoMigrate 与版本迁移
- **AND** 导入生产版本迁移定义的系统默认数据（seed）
- **AND** 该 schema 在进程内被缓存复用，不重复构建

#### Scenario: 测试迁移路径开启破坏性迁移开关

- **WHEN** `runTestMigrations` 执行前
- **THEN** 系统 SHALL 通过 `t.Setenv("MIGRATIONS_ALLOW_DESTRUCTIVE","1")` 开启破坏性迁移开关
- **AND** 破坏性迁移（如 `20260706_0001` TRUNCATE `topic_lifeline_context`）在测试库照常执行，维持测试库等价于"生产首次启动 + 全量历史清理"的状态

#### Scenario: 回归测试重新导入（逃生口）

- **WHEN** 回归测试显式调用 `testutil.ReimportTestDB`
- **THEN** 系统重建临时数据库的 `public` schema，重新执行生产 AutoMigrate 与版本迁移
- **AND** 数据库恢复为生产首次启动后的 schema 与默认数据状态
- **AND** 重新导入前的测试数据不残留
- **AND** `ReimportTestDB` 末尾的重开连接池逻辑（修 pgvector OID 漂移）不被移除

#### Scenario: 测试初始化保持物理隔离

- **WHEN** 测试初始化、重置或重新导入数据库
- **THEN** 所有操作只作用于 testcontainers-go 创建的临时 Postgres
- **AND** 系统不读取可将测试连接重定向到开发或生产数据库的 DSN

#### Scenario: seed 表名清单是受控元数据而非数据副本

- **WHEN** 生产迁移向新的表注入 seed 数据
- **THEN** 该 seed 表名 MUST 被加入 `ResetTestData` 的快照表名列表
- **AND** 快照**内容**始终运行时从黄金 schema 的 DB 读取，testutil 中不存在 seed 数据副本
- **AND** seed 数据始终来自生产迁移，不会 drift

#### Scenario: 变更范围受限

- **WHEN** 实施本 change
- **THEN** 只修改测试数据库初始化、重置、重新导入入口及对应文档
- **AND** 不修改业务逻辑、业务测试断言、生产迁移或生产默认数据

### Requirement: 进程内单例连接与单次迁移

系统 SHALL 在 testutil 包内通过 `sync.Once` 实现：(1) pgvector 容器与 `*gorm.DB` 连接的进程内单例——首次 `OpenTestDB` 启动容器并执行 `gorm.Open` 建立连接，缓存于包级 `sync.Once`（`startOnce`），后续调用复用同一实例，不重复启动容器或建池；(2) **黄金 schema 的进程内单次构建**——`runTestMigrations`（AutoMigrate 全量 model + 版本迁移 + seed）在进程内只发生一次（`migrateOnce`），无论 `SetupTestDB` 被调用多少次。

#### Scenario: 重复调用 SetupTestDB 只启动一次容器

- **WHEN** 同一测试进程内 `SetupTestDB` 被多次调用
- **THEN** 底层 testcontainer 容器与 `gorm.Open` 均仅执行一次，所有调用复用同一容器与连接池

#### Scenario: 重复调用 SetupTestDB 只迁移一次

- **WHEN** 同一测试进程内 `SetupTestDB` 被多次调用
- **THEN** `runTestMigrations`（AutoMigrate + 版本迁移 + seed）仅在首次执行
- **AND** 后续调用仅执行 `ResetTestData`（seed 保护的 truncate），不重建 schema

#### Scenario: 黄金 schema 构建后 OID 稳定

- **WHEN** 黄金 schema 已构建，后续测试通过 `ResetTestData` 重置
- **THEN** 不发生 `DROP SCHEMA` / `CREATE EXTENSION`，pgvector `vector` 类型 OID 保持不变
- **AND** 连接池的 prepared statement 不失效，不出现 `cache lookup failed for type <oid>` 错误

#### Scenario: 首次连接或迁移失败不被吞掉

- **WHEN** 首次 `gorm.Open` 或 `runTestMigrations` 失败
- **THEN** 错误被记录到包级变量，后续每次 `SetupTestDB` 调用通过 `t.Fatal` 报出该错误，不静默返回无效 `*gorm.DB`

### Requirement: testutil package with TruncateAllTables

系统 SHALL 提供 `TruncateAllTables(t *testing.T, db *gorm.DB)` 函数，使用 `TRUNCATE TABLE ... CASCADE` 清空所有 domain 表数据，保证测试间隔离。

#### Scenario: 清空所有表数据

- **WHEN** 数据库中有 3 个表各含 10 行数据，`TruncateAllTables` 被调用
- **THEN** 所有 3 个表的行数为 0

#### Scenario: 外键关联表被 CASCADE 清空

- **WHEN** `topic_tags` 表有 1 行，`topic_tag_embeddings` 表通过外键关联且有 3 行
- **THEN** Truncate 后两张表均为空

### Requirement: 测试分层 — short 模式跳过集成测试

系统 SHALL 将 `testing.Short()` 跳过逻辑内置在 `SetupTestDB` 中：函数首行执行 `if testing.Short() { t.Skip("requires Postgres") }`。集成测试函数本身无需（也不应）重复编写该守卫——调用 `SetupTestDB` 即自动获得分层保护。

依赖真实 LLM 等其他外部资源但不走 `SetupTestDB` 的测试（如 `tag_context_dump_test.go`）SHALL 自行维护 `testing.Short()` 守卫。单元测试（`*_unit_test.go`）不调用 `SetupTestDB`，在 `-short` 下自然运行。

#### Scenario: go test -short 跳过 DB 测试

- **WHEN** 运行 `go test -short ./internal/tagmanagement/...`
- **THEN** 所有依赖 Postgres 的测试被跳过，无 Docker 依赖，纯单元测试正常运行

#### Scenario: go test 运行集成测试

- **WHEN** 运行 `go test ./internal/tagmanagement/...`（不带 `-short`）且 Docker Postgres 可用
- **THEN** 所有集成测试运行并通过，使用真实 Postgres + pgvector

### Requirement: 测试文件命名规范区分单元与集成

系统 SHALL 使用文件命名规范在同一 package 内区分单元测试和集成测试：

| 文件模式 | 含义 | 需要 DB | `testing.Short()` |
|---------|------|---------|-------------------|
| `xxx_test.go` | 集成测试 | 是 (Postgres) | 是 (跳过) |
| `xxx_unit_test.go` | 单元测试 | 否 | 否 |

真实 LLM 验证测试（如 `tag_context_dump_test.go`）保持现有 `testing.Short()` 保护模式不变。

#### Scenario: 单元测试文件无需 Docker 即可通过

- **WHEN** 运行 `go test -short ./internal/tagmanagement/service/core/...`
- **THEN** `embedding_unit_test.go` 中的 10 个测试全部运行并通过，无需 Docker Postgres

#### Scenario: 集成测试文件在 -short 下被跳过

- **WHEN** 运行 `go test -short ./internal/tagmanagement/service/core/...`
- **THEN** `embedding_test.go` 中的 3 个集成测试被 `testing.Short()` 跳过

#### Scenario: 混合文件被正确拆分

- **WHEN** 检查 `internal/tagmanagement/service/core/` 目录
- **THEN** `embedding_unit_test.go` 存在且只包含无 DB 依赖的测试函数
- **THEN** `embedding_test.go` 只包含需要 DB 的测试函数，每个函数开头有 `testing.Short()` 检查

### Requirement: 测试与源码同目录共存

测试文件 SHALL 与被测源码放在同一目录（Go 同 package 约定），不使用独立的 `tests/` 目录。

#### Scenario: 测试文件能访问 unexported 符号

- **WHEN** 源码文件定义了 unexported 函数（如 `evaluateSemanticBoardMatches`）
- **THEN** 同目录的 `*_test.go` 文件可以直接调用该函数，无需 export

### Requirement: ResetTestData — 测试间黄金态恢复重置

系统 SHALL 提供 `ResetTestData(t *testing.T, db *gorm.DB) *gorm.DB` 函数（返回重开后的新连接池句柄），作为黄金 schema 模式下测试间的重置入口，维持"reset 后 = 干净黄金态"不变量。黄金 schema 首次构建后，系统 SHALL 快照：(a) 迁移 seed 表（`ai_settings`、`embedding_config`）的全部行，(b) 所有 vector 类型列的类型声明（`pg_catalog.format_type`）。`ResetTestData` 的流程 SHALL 为：① `TRUNCATE TABLE ... RESTART IDENTITY CASCADE` 清空所有业务表（仅跳过 `schema_migrations`），② 无条件 re-ALTER 每个 vector 列回黄金类型（撤销测试可能的 `ALTER COLUMN ... TYPE vector(N)` 变异；无条件是为了保护"执行中变异"的测试，详见 design 决策⑥），③ 从快照灌回 seed 行并推进序列，④ 重开连接池（黄金 schema 从不 DROP 扩展，OID 稳定；重开只清被 ALTER 作废的 prepared statement）。返回新连接句柄（调用方 MUST 重新赋值）。`TruncateAllTables`（清空一切，无快照恢复、无重开）SHALL 原义保留。

#### Scenario: 清空业务表数据

- **WHEN** 黄金 schema 中 3 个业务表各含 10 行数据，`ResetTestData` 被调用
- **THEN** 3 个业务表的行数均为 0

#### Scenario: 从快照恢复迁移 seed 数据

- **WHEN** 黄金 schema 已构建（`ai_settings`、`embedding_config` 含迁移 seed 行），`ResetTestData` 被调用
- **THEN** `ai_settings`、`embedding_config` 恢复为快照记录的 seed 行（等价迁移刚跑完的状态），业务表为空

#### Scenario: 清除测试插入的自定义 seed 表行

- **WHEN** 某测试在 `ai_settings` 插入了非迁移 seed 的自定义键（如 `semantic_board_match_sim_threshold`），下个测试调用 `ResetTestData`
- **THEN** 该自定义键被 truncate 清除，未出现在快照中，不被恢复
- **AND** 下个测试拿到的 `ai_settings` 只含迁移 seed 行（等价生产首次启动）

#### Scenario: 恢复被测试变异的 vector 列维度

- **WHEN** 某测试执行了 `ALTER COLUMN embedding TYPE vector(2560)` 变异了黄金态的 vector 列，下个测试调用 `ResetTestData`
- **THEN** 该 vector 列被 re-ALTER 回黄金构建时的类型声明（如无维度 `vector`）
- **AND** 下个测试插入 3 维向量 `[1,0,0]` 不再报维度不匹配

#### Scenario: 重开连接池清除失效的 prepared statement

- **WHEN** 某测试的 ALTER 变异使连接池的 prepared statement 失效，`ResetTestData` 被调用
- **THEN** 连接池被重开，返回新句柄，后续查询不再报 `cached plan must not change result type`
- **AND** 调用方 MUST 使用返回的新句柄（传入的旧句柄连接已关闭）

#### Scenario: 保留 schema_migrations 版本记录

- **WHEN** `ResetTestData` 被调用
- **THEN** `schema_migrations` 表的迁移版本记录不被清空，避免误判迁移未应用

#### Scenario: 外键关联表被 CASCADE 清空

- **WHEN** `topic_tags` 表有 1 行，`topic_tag_embeddings` 表通过外键关联且有 3 行
- **THEN** `ResetTestData` 后两张业务表均为空

### Requirement: 全量门禁零失败基线

仓库后端 SHALL 维持全量门禁零失败：`golangci-lint run ./...` 与 `go test ./...`（含 testcontainer 集成测试）在全干工作树上执行 MUST 零失败。既存 lint 债（gofmt / unused / errcheck 等）与 pre-existing 坏测试 MUST 即时清理，不得积压阻塞《开发执行规范》§11.1 归档门禁。新增代码引入新 lint / 测试失败时，MUST 在同一 change 内修复。

#### Scenario: 全量 lint 零失败

- **WHEN** 在干净工作树上执行 `cd backend-go && golangci-lint run ./...`
- **THEN** SHALL 返回 0 issues（exit 0）

#### Scenario: 全量测试零失败

- **WHEN** 在 Docker daemon 可用的环境下，干净工作树上执行 `cd backend-go && go test ./...`
- **THEN** 所有包 SHALL PASS，无 pre-existing 失败用例

### Requirement: 数据访问层（repository）测试禁用内存 SQLite

数据访问层（`backend-go` 下所有 `*/repository/` 包）的测试 SHALL 使用 `testutil.SetupTestDB`（隔离 pgvector testcontainer）作为数据库来源，SHALL NOT 使用内存 SQLite（`github.com/glebarez/sqlite` / `sqlite.Open`）模拟数据库行为来测试数据访问逻辑。

**理由**：SQLite 与 PostgreSQL 在 GROUP BY 语义、JSONB/vector 类型校验、约束执行上存在系统性差异——SQLite 对上述宽松放过，PostgreSQL 严格拒绝。数据访问层是直接发 SQL 的层，用 SQLite 测试会掩盖生产 SQL 的运行时错误（已记录至少 4 起同类事故：GROUP BY 缺列、vector 空串、JSONB 空串、迁移 NOT NULL）。把 `standard/backend/testing.md` 已有的事故硬约束从「改发往真实 DB 的 SQL 时才补 PG 用例」前置为「repository 包常驻禁用 SQLite」，从源头消除假绿温床。

**例外**：不依赖数据库的纯算法/纯函数测试（如距离计算、匈牙利分配、向量聚合）SHALL 归入 `*_unit_test.go`（见「测试文件命名规范区分单元与集成」requirement），不受本约束——因其本就不产生 SQLite 与 PostgreSQL 的语义漂移。

#### Scenario: repository 包测试文件不含 SQLite 依赖

- **WHEN** 扫描 `backend-go` 下所有 `*/repository/*_test.go` 文件
- **THEN** 无任何文件 import `github.com/glebarez/sqlite` 或调用 `sqlite.Open`
- **AND** 数据访问层测试统一通过 `testutil.SetupTestDB` 获取 `*gorm.DB`

#### Scenario: repository 数据访问逻辑走真 PG

- **WHEN** `*/repository/` 包内测试需要操作数据库（CRUD / 查询 / 级联 / 唯一约束去重）
- **THEN** 该测试 SHALL 调用 `testutil.SetupTestDB(t)` 获取连接到隔离 pgvector 容器的 `*gorm.DB`
- **AND** 不使用内存 SQLite 模拟 PostgreSQL 行为

#### Scenario: repository 包内纯函数测试豁免迁移

- **WHEN** `*/repository/` 包内存在不依赖数据库的纯算法/纯函数测试
- **THEN** 该类测试 SHALL 归入 `*_unit_test.go` 文件，不调用 `SetupTestDB`
- **AND** 本 requirement 不强制其迁移，因其不产生 SQLite 与 PostgreSQL 的语义漂移

