# Postgres + pgvector 单库数据库迁移 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 SQLite 单文件持久化切换为 `Postgres + pgvector` 单库架构，在不引入独立向量库的前提下，承接现有 RSS、摘要、Topic Graph、调度任务和后续向量检索增长。

**Architecture:** 先把数据库接入层从 SQLite 专用实现拆成“驱动感知 + 版本化迁移 + 单独数据搬运工具”三层，再把 `topic_tag_embeddings` 从 JSON 文本改为 Postgres `vector` 列，并将相似度计算从 Go 内存全表扫描下推到数据库。切换方式采用一次性停机迁移，不做长期双写，不保留 SQLite 和 Postgres 的并行运行路径。

**Tech Stack:** Go, Gin, GORM, PostgreSQL 17, pgvector, SQLite（旧库数据源）, Python integration tests, Docker, GitNexus

---

## 这份计划的定位

- 这是“架构级主计划”，不是单个功能计划。
- 它覆盖数据库接入、Schema 管理、数据搬运、向量检索、测试体系、运维切换。
- 执行时可以继续拆成阶段性子计划，但子计划不能改变这里定义的主约束。

## 主约束

- 单库优先：目标架构固定为 `Postgres + pgvector`，不同时引入 Qdrant / Milvus / Weaviate。
- 不做长期双写：迁移方式是“停机导出 + 校验 + 切换 + 保留 SQLite 备份回滚”。
- 不保留数据库兼容分支：运行时代码最终同时支持 `sqlite` 和 `postgres` 作为启动驱动，但生产主路径切到 Postgres 后，不再为旧 SQLite 行为继续追加新能力。
- 第一阶段只迁移现有 tag embedding 能力，不把“文章级全文向量检索”一起塞进首批 cutover。
- 首批迁移不强推 `ai_summaries.articles` 正规化；该字段保留为现状，但计划内必须明确为后续优化点。

## 文件结构与责任

### 数据库接入与迁移

- Modify: `backend-go/internal/platform/config/config.go`
- Modify: `backend-go/configs/config.yaml`
- Modify: `backend-go/internal/platform/database/db.go`
- Create: `backend-go/internal/platform/database/connect_sqlite.go`
- Create: `backend-go/internal/platform/database/connect_postgres.go`
- Create: `backend-go/internal/platform/database/bootstrap_sqlite.go`
- Create: `backend-go/internal/platform/database/bootstrap_postgres.go`
- Create: `backend-go/internal/platform/database/migrator.go`
- Create: `backend-go/internal/platform/database/postgres_migrations.go`
- Create: `backend-go/internal/platform/database/sqlite_legacy_migrations.go`

### 数据迁移工具

- Create: `backend-go/cmd/migrate-db/main.go`
- Create: `backend-go/internal/platform/database/datamigrate/reader_sqlite.go`
- Create: `backend-go/internal/platform/database/datamigrate/writer_postgres.go`
- Create: `backend-go/internal/platform/database/datamigrate/verify.go`
- Create: `backend-go/internal/platform/database/datamigrate/types.go`

### 向量与 Topic 相关改造

- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Create: `backend-go/internal/domain/topicanalysis/embedding_repository.go`
- Create: `backend-go/internal/domain/topicanalysis/embedding_repository_test.go`

### 测试支撑与集成测试

- Create: `backend-go/internal/testsupport/testdb/factory.go`
- Modify: `backend-go/internal/platform/database/db_test.go`
- Modify: `backend-go/internal/domain/feeds/service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_job_queue_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_handler_test.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue_test.go`
- Modify: `backend-go/internal/domain/topicgraph/handler_test.go`
- Modify: `backend-go/internal/domain/topicextraction/metadata_test.go`
- Modify: `backend-go/internal/domain/topicextraction/tag_job_queue_test.go`
- Modify: `tests/workflow/utils/database.py`
- Modify: `tests/firecrawl/test_firecrawl_integration.py`
- Modify: `tests/workflow/requirements.txt`

### 文档与切换手册

- Modify: `docs/architecture/backend-go.md`
- Modify: `docs/operations/database.md`
- Create: `docs/operations/postgres-cutover.md`

## 阶段边界

- Phase A: 驱动抽象与迁移框架落地
- Phase B: Postgres Schema 与数据搬运能力落地
- Phase C: pgvector 与向量检索切换
- Phase D: 测试体系切换
- Phase E: 演练、切换、回滚手册固化

### Task 1: 拆出驱动感知的数据库初始化入口

**Files:**
- Modify: `backend-go/internal/platform/config/config.go`
- Modify: `backend-go/configs/config.yaml`
- Modify: `backend-go/internal/platform/database/db.go`
- Create: `backend-go/internal/platform/database/connect_sqlite.go`
- Create: `backend-go/internal/platform/database/connect_postgres.go`
- Test: `backend-go/internal/platform/database/db_test.go`

- [ ] **Step 1: 写驱动分支测试，先让当前实现失败**

目标测试：

```go
func TestInitDBUsesSQLiteDriver(t *testing.T) {}
func TestInitDBUsesPostgresDriver(t *testing.T) {}
func TestInitDBRejectsUnknownDriver(t *testing.T) {}
```

- [ ] **Step 2: 运行目标测试，确认当前只支持 SQLite**

Run: `go test ./internal/platform/database -run TestInitDB -v`
Expected: `TestInitDBUsesPostgresDriver` FAIL，因为当前 `InitDB()` 写死了 `sqlite.Open(...)`。

- [ ] **Step 3: 把连接逻辑拆成按驱动分发的入口**

目标结构：

```go
func InitDB(cfg *config.Config) error {
	conn, err := openDatabase(cfg)
	if err != nil {
		return err
	}
	DB = conn
	return bootstrapDatabase(cfg)
}

func openDatabase(cfg *config.Config) (*gorm.DB, error) {
	switch cfg.Database.Driver {
	case "sqlite":
		return openSQLite(cfg)
	case "postgres":
		return openPostgres(cfg)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}
}
```

- [ ] **Step 4: 把 SQLite 专属 PRAGMA 和单连接池设置移出通用路径**

SQLite 保留：

```go
DB.Exec("PRAGMA journal_mode=WAL")
DB.Exec("PRAGMA busy_timeout=5000")
sqlDB.SetMaxIdleConns(2)
sqlDB.SetMaxOpenConns(1)
```

Postgres 目标：

```go
sqlDB.SetMaxIdleConns(5)
sqlDB.SetMaxOpenConns(20)
sqlDB.SetConnMaxLifetime(30 * time.Minute)
```

- [ ] **Step 5: 更新默认配置和示例 DSN**

目标配置片段：

```yaml
database:
  driver: "sqlite"
  dsn: "rss_reader.db"
```

并在注释或示例文档里补一份 Postgres DSN：

```text
host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai
```

- [ ] **Step 6: 运行数据库包测试确认通过**

Run: `go test ./internal/platform/database -v`
Expected: PASS。

### Task 2: 把 SQLite 专用建表逻辑替换为版本化迁移框架

**Files:**
- Modify: `backend-go/internal/platform/database/db.go`
- Create: `backend-go/internal/platform/database/migrator.go`
- Create: `backend-go/internal/platform/database/postgres_migrations.go`
- Create: `backend-go/internal/platform/database/sqlite_legacy_migrations.go`
- Test: `backend-go/internal/platform/database/db_test.go`

- [ ] **Step 1: 写迁移状态表和版本执行测试**

目标测试：

```go
func TestRunMigrationsCreatesSchemaMigrationsTable(t *testing.T) {}
func TestRunMigrationsAppliesEachVersionOnce(t *testing.T) {}
func TestSQLiteLegacyBootstrapStillWorksForExistingTests(t *testing.T) {}
```

- [ ] **Step 2: 运行迁移测试，确认当前框架不满足要求**

Run: `go test ./internal/platform/database -run 'TestRunMigrations|TestSQLiteLegacyBootstrap' -v`
Expected: FAIL，因为当前没有版本化迁移表，只有 `EnsureTables()` 和一组 SQLite 专用补字段逻辑。

- [ ] **Step 3: 引入统一迁移注册器**

目标结构：

```go
type Migration struct {
	Version string
	Driver  string
	Up      func(db *gorm.DB) error
}

func RunMigrations(db *gorm.DB, driver string) error
```

- [ ] **Step 4: 保留 SQLite 旧迁移，仅作为历史兼容路径**

要求：

- SQLite 路径继续保证现有单测和旧库可启动。
- SQLite 的 `sqlite_master` / `pragma_table_info` 查询只能留在 `sqlite_legacy_migrations.go`。
- Postgres 路径不能调用任何 SQLite 元数据查询。

- [ ] **Step 5: 把 Postgres 初始 Schema、索引、扩展创建迁移化**

第一批 Postgres 迁移至少覆盖：

- `CREATE EXTENSION IF NOT EXISTS vector`
- 核心表创建
- 主索引创建
- `topic_tag_embeddings.embedding vector(1536)` 列创建

- [ ] **Step 6: 运行数据库测试确认迁移框架稳定**

Run: `go test ./internal/platform/database -v`
Expected: PASS。

### Task 3: 落地 Postgres 基础 Schema 与高频索引

**Files:**
- Create: `backend-go/internal/platform/database/bootstrap_postgres.go`
- Modify: `backend-go/internal/domain/models/article.go`
- Modify: `backend-go/internal/domain/models/feed.go`
- Modify: `backend-go/internal/domain/models/ai_models.go`
- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Test: `backend-go/internal/platform/database/db_test.go`

- [ ] **Step 1: 为高频查询补基线索引测试或迁移断言**

至少覆盖这些查询路径：

- `articles(feed_id, created_at)`
- `articles(pub_date)`
- `ai_summaries(feed_id, created_at)`
- `ai_summaries(category_id, created_at)`
- `article_topic_tags(topic_tag_id, article_id)`
- `ai_summary_topics(topic_tag_id, summary_id)`
- `reading_behaviors(feed_id, created_at)`

- [ ] **Step 2: 在 Postgres 迁移里创建正式索引，而不是继续靠启动时拼 SQL**

目标 SQL 片段：

```sql
CREATE INDEX IF NOT EXISTS idx_articles_feed_created_at ON articles(feed_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_summaries_feed_created_at ON ai_summaries(feed_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_article_topic_tags_topic_article ON article_topic_tags(topic_tag_id, article_id);
```

- [ ] **Step 3: 明确首批不做的表结构重构**

这一步写进实现注释和文档：

- `ai_summaries.articles` 暂不拆表。
- `payload_json`、`metadata` 等 JSON 文本字段首批保留。
- 首批目标是“先切库、先把向量下推、先稳住主链路”。

- [ ] **Step 4: 用 Postgres 实例做一次空库启动验证**

Run:

```bash
go run cmd/server/main.go
```

Expected: 在 Postgres 空库上完成扩展、建表、建索引，服务能正常启动。

### Task 4: 建立 SQLite -> Postgres 一次性数据搬运工具

**Files:**
- Create: `backend-go/cmd/migrate-db/main.go`
- Create: `backend-go/internal/platform/database/datamigrate/reader_sqlite.go`
- Create: `backend-go/internal/platform/database/datamigrate/writer_postgres.go`
- Create: `backend-go/internal/platform/database/datamigrate/verify.go`
- Create: `backend-go/internal/platform/database/datamigrate/types.go`
- Test: `backend-go/internal/platform/database/datamigrate/verify_test.go`

- [ ] **Step 1: 写数据迁移校验测试**

目标测试：

```go
func TestVerifyCountsMatch(t *testing.T) {}
func TestVerifySequenceIsResetAfterImport(t *testing.T) {}
func TestVerifyEmbeddingRowCountMatches(t *testing.T) {}
```

- [ ] **Step 2: 运行校验测试，确认迁移工具尚不存在**

Run: `go test ./internal/platform/database/datamigrate -v`
Expected: FAIL，因为新包和校验逻辑还不存在。

- [ ] **Step 3: 定义搬运顺序并保留原主键**

导入顺序固定为：

1. `categories`
2. `feeds`
3. `articles`
4. `ai_settings`
5. `ai_providers`
6. `ai_routes`
7. `ai_route_providers`
8. `ai_call_logs`
9. `ai_summaries`
10. `ai_summary_feeds`
11. `topic_tags`
12. `topic_tag_analyses`
13. `topic_analysis_cursors`
14. `ai_summary_topics`
15. `article_topic_tags`
16. `reading_behaviors`
17. `user_preferences`
18. `scheduler_tasks`
19. `firecrawl_jobs`
20. `tag_jobs`
21. `topic_tag_embeddings`

- [ ] **Step 4: 搬运工具必须做三类校验**

- 行数校验：源库和目标库逐表一致。
- 序列校验：导入后 `setval` 到最大主键。
- 抽样校验：文章、摘要、tag、embedding 至少各抽样 20 条比对关键字段。

- [ ] **Step 5: 为 embedding 搬运增加 JSON -> vector 转换**

要求：

- SQLite 读出 `topic_tag_embeddings.vector` 的 JSON 数组。
- 转为 Postgres `vector(1536)` 可写入格式。
- `dimension` 与 `model` 必须保留并校验。

- [ ] **Step 6: 提供 dry-run 和 verify-only 模式**

目标 CLI：

```bash
go run cmd/migrate-db/main.go --source-sqlite rss_reader.db --target-postgres "..." --dry-run
go run cmd/migrate-db/main.go --source-sqlite rss_reader.db --target-postgres "..." --execute
go run cmd/migrate-db/main.go --source-sqlite rss_reader.db --target-postgres "..." --verify-only
```

- [ ] **Step 7: 运行迁移工具包测试**

Run: `go test ./internal/platform/database/datamigrate -v`
Expected: PASS。

### Task 5: 把 tag embedding 从 JSON 文本切到 pgvector，并把相似度下推到数据库

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Create: `backend-go/internal/domain/topicanalysis/embedding_repository.go`
- Test: `backend-go/internal/domain/topicanalysis/embedding_repository_test.go`
- Test: `backend-go/internal/domain/topicextraction/metadata_test.go`

- [ ] **Step 1: 写相似度查询回归测试**

目标测试：

```go
func TestFindSimilarTagsUsesDatabaseOrderingInPostgres(t *testing.T) {}
func TestFindSimilarTagsFallsBackToInMemoryForSQLite(t *testing.T) {}
func TestSaveEmbeddingPersistsVectorColumn(t *testing.T) {}
```

- [ ] **Step 2: 运行目标测试，确认当前实现仍是全量加载再内存算**

Run: `go test ./internal/domain/topicanalysis ./internal/domain/topicextraction -run 'TestFindSimilarTags|TestSaveEmbedding' -v`
Expected: FAIL，因为当前 `FindSimilarTags()` 会 `Find(&existingEmbeddings)` 后逐条 `json.Unmarshal`。

- [ ] **Step 3: 改模型，明确 Postgres 路径的向量列语义**

目标模型语义：

```go
type TopicTagEmbedding struct {
	ID         uint
	TopicTagID uint
	Embedding  string
	Dimension  int
	Model      string
	TextHash   string
}
```

说明：

- Go 层可以保留一个序列化字段用于兼容搬运工具。
- 数据库层必须让 Postgres 实际列类型成为 `vector(1536)`。
- 不要继续把 Postgres 主路径当成 `TEXT` JSON 列来用。

- [ ] **Step 4: 新增 repository，把数据库方言差异收口**

目标接口：

```go
type EmbeddingRepository interface {
	Save(ctx context.Context, embedding *models.TopicTagEmbedding) error
	FindNearest(ctx context.Context, category string, query []float32, limit int) ([]TagCandidate, error)
}
```

- [ ] **Step 5: 在 Postgres 上使用数据库排序和距离运算符**

目标 SQL 形态：

```sql
SELECT t.id, t.label, t.slug, t.category,
       1 - (e.embedding <=> $1) AS similarity
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
WHERE t.category = $2
ORDER BY e.embedding <=> $1
LIMIT $3;
```

- [ ] **Step 6: 为向量列增加 HNSW 索引，但仅在数据量达到阈值后启用**

首批策略：

- 小规模 tag embedding：先 exact search。
- 当 `topic_tag_embeddings` 达到 1 万量级以上，再执行：

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_topic_tag_embeddings_hnsw_cosine
ON topic_tag_embeddings USING hnsw (embedding vector_cosine_ops);
```

- [ ] **Step 7: 跑 topic 相关回归测试**

Run: `go test ./internal/domain/topicanalysis ./internal/domain/topicextraction ./internal/domain/topicgraph -v`
Expected: PASS。

### Task 6: 建立统一测试数据库工厂，消除测试代码对 sqlite.Open 的直接依赖

**Files:**
- Create: `backend-go/internal/testsupport/testdb/factory.go`
- Modify: `backend-go/internal/platform/database/db_test.go`
- Modify: `backend-go/internal/domain/feeds/service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/firecrawl_job_queue_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_handler_test.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue_test.go`
- Modify: `backend-go/internal/domain/topicgraph/handler_test.go`
- Modify: `backend-go/internal/domain/topicextraction/metadata_test.go`
- Modify: `backend-go/internal/domain/topicextraction/tag_job_queue_test.go`
- Modify: `backend-go/internal/platform/airouter/store_test.go`
- Modify: `backend-go/internal/platform/aisettings/config_store_test.go`

- [ ] **Step 1: 写测试工厂自检测试**

目标测试：

```go
func TestNewSQLiteTestDB(t *testing.T) {}
func TestNewPostgresTestDBWhenDSNProvided(t *testing.T) {}
```

- [ ] **Step 2: 运行自检测试，确认当前没有统一工厂**

Run: `go test ./internal/testsupport/testdb -v`
Expected: FAIL，因为测试工厂包还不存在。

- [ ] **Step 3: 实现测试工厂，默认 SQLite，显式环境变量时切 Postgres**

目标环境变量：

```text
TEST_DB_DRIVER=sqlite|postgres
TEST_POSTGRES_DSN=host=127.0.0.1 ...
```

- [ ] **Step 4: 批量替换测试里的直接驱动创建**

需要清理的旧模式：

```go
gorm.Open(sqlite.Open(...), &gorm.Config{})
```

替换为：

```go
db := testdb.MustOpen(t)
database.DB = db
```

- [ ] **Step 5: 跑一轮关键后端测试**

Run:

```bash
go test ./internal/platform/database ./internal/domain/feeds ./internal/domain/contentprocessing ./internal/domain/summaries ./internal/domain/topicgraph ./internal/jobs -v
```

Expected: PASS。

### Task 7: 让 Python 集成测试支持 Postgres，而不是直接绑死 sqlite3 文件

**Files:**
- Modify: `tests/workflow/utils/database.py`
- Modify: `tests/workflow/test_workflow_integration.py`
- Modify: `tests/workflow/test_schedulers.py`
- Modify: `tests/workflow/test_error_handling.py`
- Modify: `tests/firecrawl/test_firecrawl_integration.py`
- Modify: `tests/workflow/requirements.txt`

- [ ] **Step 1: 把 Python DB helper 改成驱动感知接口**

目标接口：

```python
class DatabaseHelper:
    def __init__(self, driver: str, dsn: str):
        ...
```

- [ ] **Step 2: 给 Postgres 加依赖，并保留 SQLite 兼容运行方式**

建议依赖：

```text
psycopg[binary]
```

- [ ] **Step 3: 清理 workflow tests 里过期字段名**

这一步必须和数据库迁移一起做，至少替换：

- `content_completion_enabled` -> `article_summary_enabled`
- `content_status` -> `summary_status`
- `content_fetched_at` -> `summary_generated_at`

- [ ] **Step 4: 更新 Firecrawl 集成测试的数据库连接方式**

旧逻辑：

```python
sqlite3.connect(str(db_path))
```

目标：

- 从测试配置读取 `DB_DRIVER` 和 `DB_DSN`。
- SQLite 时继续支持文件连接。
- Postgres 时使用 `psycopg.connect(...)`。

- [ ] **Step 5: 运行 Python 集成测试**

Run:

```bash
pytest tests/workflow/test_*.py -v
python tests/firecrawl/test_firecrawl_integration.py
```

Expected: PASS。

### Task 8: 清理 SQLite 命名残留和 tracing 里的实现误导

**Files:**
- Modify: `backend-go/internal/platform/tracing/exporter.go`
- Modify: `backend-go/internal/platform/tracing/provider.go`
- Modify: `docs/architecture/backend-go.md`
- Modify: `docs/operations/database.md`

- [ ] **Step 1: 改 tracing exporter 命名，避免继续暗示系统只支持 SQLite**

目标重命名：

```go
type SQLSpanExporter struct { ... }
func NewSQLSpanExporter(db *gorm.DB, cfg Config) (*SQLSpanExporter, error)
```

- [ ] **Step 2: 文档里把“当前数据库 = SQLite”更新成“默认开发 SQLite，可切换 Postgres，生产目标 Postgres”**

- [ ] **Step 3: 增加专门的 Postgres 切换文档**

文档至少覆盖：

- 本地起 Postgres 的方式
- 安装 `pgvector`
- 配置 DSN
- 执行迁移命令
- 校验步骤
- 回滚步骤

- [ ] **Step 4: 运行一次文档对应命令自检**

Run:

```bash
go build ./...
go test ./...
```

Expected: PASS，文档中的命令和实际代码一致。

### Task 9: 做正式切换演练和生产切换 Runbook

**Files:**
- Create: `docs/operations/postgres-cutover.md`
- Reference: `backend-go/cmd/migrate-db/main.go`
- Reference: `backend-go/configs/config.yaml`

- [ ] **Step 1: 固化切换前检查表**

必须明确列出：

- SQLite 文件备份位置
- Postgres 目标库是否已开启 `vector` 扩展
- 迁移命令
- 校验命令
- 服务停机窗口
- 回滚负责人和回滚命令

- [ ] **Step 2: 写出正式切换步骤**

推荐顺序：

1. 停后端服务
2. 备份 `rss_reader.db`
3. 在目标 Postgres 上执行 schema migration
4. 执行 `cmd/migrate-db --execute`
5. 执行 `--verify-only`
6. 修改生产配置为 `database.driver=postgres`
7. 启动后端
8. 手工验证核心 API
9. 观察 scheduler、topic graph、digest

- [ ] **Step 3: 写出回滚步骤**

回滚顺序：

1. 停后端服务
2. 切回 SQLite 配置
3. 使用切换前备份的 SQLite 文件启动
4. 禁止在故障 Postgres 库上继续写入
5. 记录失败点，修复后重新做整轮演练

- [ ] **Step 4: 做一次 staging 演练并记录结果**

验收标准：

- 行数校验通过
- 后端启动通过
- `/api/articles`、`/api/summaries`、`/api/topic-graph`、`/api/digest` 返回正常
- topic tag 相似度匹配可正常命中
- scheduler 状态正常刷新

- [ ] **Step 5: 切换完成后补一次性能基线记录**

至少记录：

- `/api/articles` 分页响应时间
- `/api/topic-graph` 响应时间
- `TagMatch` 平均耗时
- `auto_summary` 一轮执行耗时
- Postgres CPU、连接数、慢查询

## 风险与延后项

- `ai_summaries.articles` 仍是 JSON 字符串，后续如需更强分析能力，再单独出二期计划拆成关联表。
- 文章级全文 embedding、混合检索、全文检索（TSVector）不进入本次首批 cutover。
- 如果 `topic_tag_embeddings` 数据量短期很小，不要过早引入 HNSW 调优，把复杂度留到真实瓶颈出现后。

## 完成标准

- 后端支持 `sqlite` 和 `postgres` 两种启动驱动。
- Postgres 路径具备版本化迁移，不再依赖 SQLite 元数据查询。
- SQLite 历史数据可一次性导入 Postgres，并完成行数、抽样、序列校验。
- `topic_tag_embeddings` 在 Postgres 中使用 `vector` 列，`FindSimilarTags()` 在 Postgres 上不再走 Go 全表扫描。
- Go 单测和 Python 集成测试都可切换到 Postgres 跑通。
- 有完整切换与回滚文档，可执行一次 staging 演练。
