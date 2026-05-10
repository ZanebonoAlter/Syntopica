# 部署指南

RSS Reader 为单用户自托管部署设计。主要部署方式是 Docker Compose，在独立容器中运行 Go 后端和 Nuxt 前端，配合持久化存储。

## 部署方式

| 目标 | 配置文件 | 说明 |
|--------|-------------|-------|
| Docker Compose（PostgreSQL + pgvector） | `docker-compose.yml` | **默认/推荐方式**，PostgreSQL 内置 pgvector 扩展支持向量检索，包含前后端 + 数据库三个服务。 |

没有 PaaS 专用配置（Vercel、Netlify、Fly.io 等）。应用程序设计为通过 Docker Compose 在单机上运行。

> **注意：SQLite 版本已归档到独立的 `sqlite` 分支，主分支仅支持 PostgreSQL 数据库。**

## 构建流水线

没有配置 CI/CD 流水线 — 仓库中没有 `.github/workflows/` 文件。构建和部署为手动步骤。

### 容器构建过程

两个 Dockerfile 都使用多阶段构建：

**后端**（`backend-go/Dockerfile`）：
1. `build` 阶段：`golang:1.25-alpine` — 下载 Go 模块，编译 `cmd/server` 为静态二进制文件（`CGO_ENABLED=0`）。
2. 最终阶段：`alpine:3.22` — 复制二进制文件和 `configs/` 目录，以非 root 用户 `appuser`（UID 10001）运行。

**前端**（`front/Dockerfile`）：
1. `build` 阶段：`node:22-alpine` — 通过 corepack 安装 pnpm，运行 `pnpm install --frozen-lockfile`，然后 `pnpm build`。
2. 最终阶段：`node:22-alpine` — 从构建阶段复制 `.output/`，运行 `node .output/server/index.mjs`。

### Docker Compose（PostgreSQL + pgvector）— 快速部署

```bash
docker compose up --build -d
```

启动三个服务：

- **postgres**: PostgreSQL（pgvector:pg18-trixie）端口 5432，带健康检查（`pg_isready`）。数据持久化在 `./data/` 目录。初始化脚本 `docker/postgres/init/01-enable-pgvector.sql` 在首次启动时执行 `CREATE EXTENSION IF NOT EXISTS vector`。
- **backend**: Go API 服务器端口 5000，内部连接 postgres 服务。
- **front**: Nuxt SSR 服务器内部端口 3000，通过 `${FRONT_PORT:-3000}` 映射到宿主机。内部通过 `http://backend:5000/api` 代理 API 请求。

启动后：
- 前端：`http://localhost:3000`
- 后端 API：`http://localhost:5000/api`

## 环境设置

完整环境变量列表见 [配置指南](configuration.md)。

### Docker 部署最小配置

`.env.example` 文件包含基础变量：

```bash
FRONT_PORT=3000
BACKEND_PORT=5000
```

所有值都有默认值 — 应用程序可以零配置启动。唯一会导致启动失败的场景是数据库 DSN 无效或不可达。

### 生产环境注意事项

生产部署时需要检查以下设置：

| 变量 | 重要原因 |
|---|---|
| `SERVER_MODE` | Docker Compose 中设置为 `"release"` 以抑制 Gin 调试输出。Docker 外默认为 `"debug"`。 |
| `CORS_ORIGINS` | 必须包含用户访问前端时的来源（如 `http://your-host:3000`）。 |
| `POSTGRES_PASSWORD` | 使用 PostgreSQL compose 时，应从默认的 `"postgres"` 修改。 |
| `NUXT_PUBLIC_API_ORIGIN` | 必须匹配外部可达的后端 URL。 |
| `NUXT_PUBLIC_API_BASE` | 必须匹配外部可达的 API URL。 |

AI 相关设置（LLM 凭证、Firecrawl、Digest 导出）通过 Web UI 配置并存储在数据库中 — 不通过环境变量设置。详见 [配置指南](configuration.md#数据库存储的设置ai-功能)。

### 代理设置（中国 / 受限网络）

两个 Dockerfile 接受构建参数代理用于依赖下载：

```bash
# 在 .env 或 shell 环境中
GOPROXY=https://goproxy.cn,direct
GOSUMDB=sum.golang.google.cn
NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
HTTP_PROXY=http://proxy:port
HTTPS_PROXY=http://proxy:port
```

这些通过两个 Docker Compose 文件的 `build.args` 部分传递。

## 数据持久化

### PostgreSQL

PostgreSQL 数据通过 `./data/` 目录挂载持久化（`docker-compose.yml` 将 `./data/` 映射到 `/var/lib/postgresql`）。

**备份**：

```bash
docker exec zanebono-rssreader-pgvector pg_dump -U postgres rss_reader > backup.sql
```

**恢复**：

```bash
cat backup.sql | docker exec -i zanebono-rssreader-pgvector psql -U postgres rss_reader
```

## 回滚步骤

没有 CI/CD 流水线，回滚为手动操作：

1. 停止正在运行的容器：
   ```bash
   docker compose down
   ```
2. 检出一个之前已知正常的 commit：
   ```bash
   git checkout <previous-commit-hash>
   ```
3. 重新构建并启动：
   ```bash
   docker compose up --build -d
   ```

如果使用 tag，也可以 `git checkout <tag>` 替代 commit hash。

**数据库回滚**：PostgreSQL 使用 GORM AutoMigrate，只支持向上迁移。升级前务必备份数据库。如果新版本包含破坏性的 schema 变更，恢复备份的 SQL 文件。

## 监控

后端内置了 OpenTelemetry 分布式追踪，使用自定义的 GORM Span Exporter（`SQLiteSpanExporter`，历史命名）。追踪数据写入 PostgreSQL 的 `otel_spans` 表。

主要追踪配置：
- **库**：`go.opentelemetry.io/otel`，全局应用 `otelgin` HTTP 中间件到所有路由
- **导出器**：自定义 `SQLiteSpanExporter`，通过 GORM 将 span 写入 `otel_spans` 表
- **HTTP 中间件**：`otelgin.Middleware("rss-reader-backend")` 为所有 HTTP handler 捕获请求级 span
- **追踪的调度器操作**：auto_refresh、firecrawl、content_completion、auto_summary、preference_update、digest
- **追踪的领域操作**：AI summary 队列批处理、AI router chat
- **数据保留**：7 天（通过 `tracing.DefaultConfig()` 配置）
- **缓冲区**：100 个 span，每 5 秒刷新
- **查询 API**：后端通过 `internal/platform/tracing/handler.go` 暴露追踪查询端点 — 最近追踪、按 trace ID 查询、时间线视图、统计、按操作/状态/时长搜索、OTLP JSON 导出

没有配置外部监控服务（Sentry、Datadog、New Relic）。内置追踪为 feed 刷新周期、AI 操作和 HTTP 请求延迟提供基础可观测性。

可通过应用内置的追踪 UI 或直接查询 `otel_spans` 表查看追踪数据。
