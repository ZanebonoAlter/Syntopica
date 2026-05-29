# PostgreSQL 迁移操作手册

从 SQLite 迁移到 PostgreSQL + pgvector 的完整操作指南。

## 前置条件

- Docker & Docker Compose
- Go 1.22+
- 现有 SQLite 数据库文件（如 `backend-go/rss_reader.db`）

## 架构概述

迁移采用**停机一次性切换**，不做长期双写：

1. Docker Compose 启动 PostgreSQL + pgvector 容器
2. Go 后端首次连接时自动创建全部表结构（版本化迁移）
3. 运行数据迁移工具，将 SQLite 数据导入 PostgreSQL
4. 验证数据完整性后，日常运行切到 PostgreSQL

## 第一步：启动 PostgreSQL 容器

```bash
docker compose up -d
```

容器启动后：

- PostgreSQL 监听 `localhost:5432`（可通过 `POSTGRES_PORT` 环境变量覆盖）
- 默认数据库名 `syntopica`，用户 `postgres`，密码 `postgres`
- 首次初始化时自动执行 `docker/postgres/init/01-enable-pgvector.sql`，启用 pgvector 扩展
- 数据持久化在 `./data` 目录

确认容器健康：

```bash
docker compose ps
# 状态应为 healthy
```

也可手动连接确认：

```bash
docker exec -it syntopica-postgres psql -U postgres -d syntopica -c "SELECT extname FROM pg_extension WHERE extname = 'vector';"
```

## 第二步：配置后端连接

编辑 `backend-go/configs/config.yaml`：

```yaml
database:
  driver: "postgres"
  dsn: "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

或通过环境变量覆盖（不修改配置文件）：

```bash
export DATABASE_DRIVER=postgres
export DATABASE_DSN="host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

## 第三步：启动后端，自动建表

```bash
cd backend-go
go run cmd/server/main.go
```

首次连接 PostgreSQL 时，后端自动执行**版本化迁移**，顺序如下：

| 序号 | 版本号 | 说明 |
|------|--------|------|
| 1 | `20260403_0001` | 启用 pgvector 扩展（`CREATE EXTENSION IF NOT EXISTS vector`） |
| 2 | `20260403_0002` | 创建全部基础表结构（GORM AutoMigrate 21 个模型表 + 列类型调整 + 10 个性能索引） |
| 3 | `20260403_0003` | 为 `topic_tag_embeddings` 表添加 `embedding vector(1536)` 列 |
| 4 | `20260413_0001` | 为 `topic_tag_embeddings.embedding` 创建 HNSW 向量索引 |
| 5 | `20260413_0002` | 创建 `embedding_config` 表并写入默认配置 |
| 6 | `20260413_0003` | 为 `topic_tags` 增加 `status`、`merged_into_id` 字段与索引 |
| 7 | `20260413_0004` | 创建 `embedding_queues` 表 |
| 8 | `20260413_0005` | 创建 `merge_reembedding_queues` 表 |
| 9 | `20260414_0001` | 为 `topic_tags` 增加 `description` 字段 |
| 10 | `20260414_0002` | 创建 `topic_tag_relations` 表 |
| 11 | `20260414_0003` | 为 `articles` 增加 `feed_summary_id`、`feed_summary_generated_at` 及对应索引 |
| 12 | `20260415_0001` | 为 `topic_tags` 增加 `is_watched`、`watched_at` 字段 |

迁移记录写入 `schema_migrations` 表，每个版本只会执行一次。

迁移完成后，还会依次执行：

- `digest.Migrate()` — 创建 `digest_configs` 表并插入默认配置
- `airouter.EnsureLegacySummaryConfigMigrated()` — 将旧版 `ai_settings` 键值对迁移到新的 AI Provider/Route 表
- `tracing.InitTracerProvider()` — 创建 `otel_spans` 追踪表

启动后观察日志，确认无报错即可。此时表结构已就绪，但表中无数据。

> **注意：** 如果是全新部署（无历史数据需要迁移），到这一步就完成了，可以跳过第四步和第五步。

## 第四步：执行数据迁移

停掉后端服务（Ctrl+C），然后运行数据迁移工具。

### 4.1 预检（Dry Run）

检查 SQLite 源数据概况，不写入任何数据：

```bash
cd backend-go
go run cmd/migrate-db/main.go \
  --mode dry-run \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable"
```

输出示例：

```
Mode: dry-run
Import order:
- categories: 5 rows
- feeds: 12 rows
- articles: 2340 rows
- ai_summaries: 180 rows
...
```

确认表和行数符合预期。

### 4.2 执行导入

```bash
go run cmd/migrate-db/main.go \
  --mode execute \
  --force \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable"
```

`--force` 标志是必需的，因为执行模式会先清空目标表再导入。

执行流程：

1. 在目标库运行 PostgreSQL 迁移（确保表结构存在）
2. 解析 SQLite 和 PostgreSQL 两端都存在的表
3. 按外键依赖顺序（categories → feeds → articles → ... → topic_tag_embeddings）逐表导入
4. `topic_tag_embeddings.vector` 字段自动从 SQLite 的 JSON 文本转换为 PostgreSQL 的 `vector(1536)` 类型
5. 导入完成后重置所有序列（`setval()`）
6. 自动运行验证

### 4.3 单独验证

如需单独验证数据完整性：

```bash
go run cmd/migrate-db/main.go \
  --mode verify-only \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable"
```

验证内容：

| 检查项 | 说明 |
|--------|------|
| 行数对比 | 每张表 SQLite 行数 vs PostgreSQL 行数 |
| 序列状态 | 每个序列的 `nextval` 应大于表中最大 ID |
| 抽样比对 | 每张表抽取最多 10 行，逐字段比对源和目标值 |
| 向量校验 | 抽样比对 embedding 向量转换的正确性 |

## 第五步：确认运行

```bash
cd backend-go
go run cmd/server/main.go
```

启动后检查：

- 日志中无迁移报错
- 前端能正常加载订阅源和文章
- Topic 分析功能正常

## 回滚方案

如果迁移后发现问题，可以回退到 SQLite：

1. 停掉后端
2. 修改 `config.yaml` 中的 `driver` 为 `sqlite`，`dsn` 改回 SQLite 文件路径
3. 原始 SQLite 文件未被修改，直接重启即可
4. PostgreSQL 容器可停止或保留用于事后分析

```bash
docker compose down
```

> SQLite 数据库文件在迁移过程中只读不写，始终保留完整备份。

## 迁移涉及的 25 张表

按导入顺序：

| 表名 | 说明 |
|------|------|
| `categories` | 分类 |
| `feeds` | 订阅源 |
| `articles` | 文章 |
| `ai_summaries` | AI 摘要 |
| `ai_summary_feeds` | 摘要关联的订阅源 |
| `scheduler_tasks` | 调度任务 |
| `ai_settings` | AI 配置（键值对） |
| `ai_providers` | AI 供应商 |
| `ai_routes` | AI 路由 |
| `ai_route_providers` | AI 路由-供应商绑定 |
| `ai_call_logs` | AI 调用日志 |
| `reading_behaviors` | 阅读行为 |
| `user_preferences` | 用户偏好 |
| `topic_tags` | 主题标签 |
| `topic_tag_embeddings` | 主题标签向量（vector 类型转换） |
| `topic_tag_analyses` | 主题分析 |
| `topic_analysis_cursors` | 主题分析游标 |
| `ai_summary_topics` | 摘要-主题关联 |
| `article_topic_tags` | 文章-主题关联 |
| `firecrawl_jobs` | Firecrawl 抓取任务 |
| `tag_jobs` | 标签任务 |
| `digest_configs` | 摘要配置（可选） |
| `topic_analysis_jobs` | 主题分析任务（可选） |

> 迁移后新增的表（如 `topic_tag_relations`、`embedding_config`、`embedding_queues`、`merge_reembedding_queues`、`narrative_summaries`）不涉及 SQLite 数据导入，由版本化迁移自动创建。

## 关键代码位置

| 文件 | 职责 |
|------|------|
| `backend-go/internal/platform/database/db.go` | 数据库初始化入口，连接 PostgreSQL |
| `backend-go/internal/platform/database/migrator.go` | 版本化迁移框架，`schema_migrations` 追踪表 |
| `backend-go/internal/platform/database/postgres_migrations.go` | 12 个 PostgreSQL 迁移注册 |
| `backend-go/internal/platform/database/bootstrap_postgres.go` | Postgres schema 引导：AutoMigrate + 索引 |
| `backend-go/internal/platform/database/connect_postgres.go` | PostgreSQL 连接与连接池配置 |
| `backend-go/internal/platform/database/datamigrate/types.go` | 数据迁移类型定义和表规格 |
| `backend-go/cmd/migrate-db/main.go` | 数据迁移 CLI 工具入口 |
| `docker-compose.yml` | PostgreSQL 容器定义 |

## 注意事项

- **停机操作**：数据迁移期间后端必须停机，否则会出现数据不一致
- **不可重复执行**：`execute` 模式会先 TRUNCATE 目标表，重复运行会清空已有数据
- **pgvector 版本**：容器镜像为 `pgvector/pgvector:pg18-trixie`，基于 PostgreSQL 18
- **连接池**：PostgreSQL 连接池参数可在 `config.yaml` 中配置（`max_idle_conns`、`max_open_conns`、`conn_max_lifetime_minutes`、`conn_max_idle_time_minutes`）
- **环境变量优先**：`DATABASE_DRIVER` 和 `DATABASE_DSN` 环境变量会覆盖配置文件
- **容器名称**：docker-compose 中 PostgreSQL 容器名为 `syntopica-postgres`
