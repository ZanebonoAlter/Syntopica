# 配置指南

RSS Reader 使用分层配置系统：后端 YAML 配置文件、覆盖文件值的环境变量，以及 Nuxt 运行时配置。AI 相关设置（LLM、Firecrawl、Digest）存储在数据库中，通过 Web UI 配置。

## 环境变量

### 后端（Go）

以下环境变量会覆盖 `backend-go/configs/config.yaml` 中的值。未设置时使用配置文件默认值。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `SERVER_PORT` | 否 | `"5000"` | 后端 HTTP 监听端口 |
| `SERVER_MODE` | 否 | `"debug"` | Gin 模式：`"debug"`、`"release"` 或 `"test"` |
| `DATABASE_DRIVER` | 否 | `"postgres"` | 数据库驱动，主分支仅支持 `"postgres"` |
| `DATABASE_DSN` | 否 | `"host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai"` | PostgreSQL 连接字符串 |
| `CORS_ORIGINS` | 否 | `"http://localhost:3000,http://localhost:3000"` | 逗号分隔的允许 CORS 来源列表 |
| `CRAWL_SERVICE_URL` | 否 | `"http://localhost:11235"` | 抓取/内容补全服务 URL |
| `REDIS_URL` | 否 | *(空)* | Topic 分析任务队列的 Redis URL。设置后使用 Redis 作为持久后端；否则回退到内存队列 |

### Topic Analysis 调优

以下环境变量控制 AI 话题分析模块，在 `internal/domain/topicanalysis/ai_analysis.go` 中通过 `parseEnvInt` / `parseEnvFloat` 读取。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `TOPIC_ANALYSIS_MAX_TOKENS` | 否 | `2000` | 话题分析 AI 调用最大 token 数 |
| `TOPIC_ANALYSIS_TEMPERATURE` | 否 | `0.2` | 话题分析 AI 调用温度 |
| `TOPIC_ANALYSIS_TIMEOUT_SECONDS` | 否 | `90` | 话题分析 AI 调用超时（秒） |
| `TOPIC_ANALYSIS_RETRY_COUNT` | 否 | `3` | 话题分析 AI 调用最大重试次数 |

### 前端（Nuxt）

通过 `nuxt.config.ts` 的 `runtimeConfig` 设置，可用环境变量覆盖。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `API_INTERNAL_BASE` | 否 | `"http://localhost:5000/api"` | 服务端 API 基础 URL（SSR 时使用） |
| `NUXT_PUBLIC_API_ORIGIN` | 否 | `"http://localhost:5000"` | 暴露给浏览器的公共 API 源 |
| `NUXT_PUBLIC_API_BASE` | 否 | `"http://localhost:5000/api"` | 暴露给浏览器的公共 API 基础 URL |

### Docker Compose

以下变量由 Docker Compose 文件使用，Docker 外部无效。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `FRONT_PORT` | 否 | `"3000"` | 前端容器映射到宿主机的端口 |
| `BACKEND_PORT` | 否 | `"5000"` | 后端容器映射到宿主机的端口 |
| `POSTGRES_DB` | 否 | `"rss_reader"` | PostgreSQL 数据库名 |
| `POSTGRES_USER` | 否 | `"postgres"` | PostgreSQL 用户名 |
| `POSTGRES_PASSWORD` | 否 | `"postgres"` | PostgreSQL 密码 |
| `POSTGRES_PORT` | 否 | `"5432"` | PostgreSQL 容器映射到宿主机的端口 |
| `TZ` | 否 | `"Asia/Shanghai"` | PostgreSQL 容器时区 |
| `GOPROXY` | 否 | *(空)* | 后端构建时的 Go 模块代理 |
| `GOSUMDB` | 否 | *(空)* | 后端构建时的 Go 校验数据库 |
| `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` | 否 | *(空)* | 代理设置，传递到构建上下文 |

## 配置文件格式

后端从 `backend-go/configs/config.yaml` 读取 YAML 配置文件，通过 Viper 在启动时加载。默认配置即为 PostgreSQL 连接，即使没有配置文件也能正常工作。

> **注意：主分支仅支持 PostgreSQL 数据库驱动。SQLite 支持仅在 `sqlite` 分支可用。**

```yaml
server:
  port: "5000"
  mode: "debug"           # debug | release | test

database:
  driver: "postgres"
  dsn: "host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai"
  postgres:
    max_idle_conns: 5
    max_open_conns: 25
    conn_max_lifetime_minutes: 60
    conn_max_idle_time_minutes: 10
```

### 主要配置段

- **server** — 控制 HTTP 服务器端口和 Gin 运行模式。`release` 模式下 Gin 会抑制调试输出。
- **database** — 配置持久化层。`driver` 字段始终为 `"postgres"`。PostgreSQL 有独立的连接池调优参数。
- **cors** — 跨域请求的允许来源、HTTP 方法和请求头。来源是列表形式；通过 `CORS_ORIGINS` 环境变量覆盖时，解析为逗号分隔的字符串。

## 必填与可选设置

所有设置都有默认值。应用程序无需任何配置文件或环境变量即可启动，使用 PostgreSQL 的合理默认值。

没有环境变量缺失会导致启动失败。配置加载代码（`config.go` 中的 `applyEnvOverrides`）仅当环境值非空时才覆盖，否则使用 YAML 文件或代码默认值。

唯一会导致启动失败的场景是数据库 DSN 无效或不可达 — `main.go` 中的 `database.InitDB` 调用会 `log.Fatalf`。

## 默认值

### 后端默认值

| 设置 | 默认值 | 来源 |
|---|---|---|
| Server port | `"5000"` | `viper.SetDefault` in `config.go` |
| Server mode | `"debug"` | `viper.SetDefault` in `config.go` |
| Database driver | `"postgres"` | `viper.SetDefault` in `config.go` |
| Database DSN | `"host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai"` | `viper.SetDefault` in `config.go` |
| Postgres max idle conns | `5` | `viper.SetDefault` in `config.go` |
| Postgres max open conns | `25` | `viper.SetDefault` in `config.go` |
| Postgres conn max lifetime | `60` min | `viper.SetDefault` in `config.go` |
| Postgres conn max idle time | `10` min | `viper.SetDefault` in `config.go` |
| CORS origins | `localhost:3000`, `localhost:3000` | `viper.SetDefault` in `config.go` |
| CORS methods | `GET, POST, PUT, DELETE, OPTIONS` | `viper.SetDefault` in `config.go` |
| CORS headers | `Content-Type, Authorization` | `viper.SetDefault` in `config.go` |
| Crawl service URL | `"http://localhost:11235"` | `runtime.go` fallback |
| Tracing enabled | `true` | `tracing.DefaultConfig()` |
| Tracing retention | `7` days | `tracing.DefaultConfig()` |
| Topic analysis max tokens | `2000` | `ai_analysis.go` `parseEnvInt` |
| Topic analysis temperature | `0.2` | `ai_analysis.go` `parseEnvFloat` |
| Topic analysis timeout | `90` s | `ai_analysis.go` `parseEnvInt` |
| Topic analysis retries | `3` | `ai_analysis.go` `parseEnvInt` |

### 前端默认值

| 设置 | 默认值 | 来源 |
|---|---|---|
| API internal base | `"http://localhost:5000/api"` | `nuxt.config.ts` |
| Public API origin | `"http://localhost:5000"` | `nuxt.config.ts` |
| Public API base | `"http://localhost:5000/api"` | `nuxt.config.ts` |

## 各环境覆盖

### 本地开发

本地开发时默认值开箱即用：

- 后端运行在 `http://localhost:5000`，使用 PostgreSQL 数据库。
- 前端开发服务器（`pnpm dev`）运行在 `http://localhost:3000`。
- 无需配置文件或 `.env` 文件。
- 需要本地运行 PostgreSQL + pgvector，可通过 Docker 启动：

```bash
docker run -d --name rss-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=rss_reader pgvector/pgvector:pg18-trixie
```

### Docker（PostgreSQL + pgvector）— 推荐方式

```bash
docker compose up -d
```

启动三个服务：

- **postgres**: PostgreSQL（pgvector:pg18-trixie）端口 5432，数据通过 `./data/` 目录持久化。
- **backend**: Go API 服务器端口 5000，内部连接 postgres 服务。
- **front**: Nuxt SSR 服务器内部端口 3000，通过 `${FRONT_PORT:-3000}` 映射到宿主机。内部通过 `http://backend:5000/api` 代理 API 请求。

启动后：
- 前端：`http://localhost:3000`
- 后端 API：`http://localhost:5000/api`

## 数据库存储的设置（AI 功能）

AI 相关配置不存储在文件或环境变量中 — 通过 Web UI 管理并持久化到 PostgreSQL 的 `ai_settings` 表。后端通过 `aisettings` 包在运行时读取。

| 配置键 | 说明 |
|---|---|
| `summary_config` | 文章摘要 LLM 凭证（base URL、API key、model） |
| `auto_summary_config` | 自动摘要调度器设置（时间范围、模型参数） |
| `firecrawl_config` | Firecrawl 集成设置（启用、API URL、API key、模式、超时、最大内容长度） |
| `open_notebook_config` | Open Notebook digest 导出设置（启用、base URL、API key、model、目标笔记本、prompt 模式、自动发送日报/周报） |

这些设置通过 `aisettings.LoadSummaryConfig()`、`aisettings.LoadFirecrawlConfig()` 等函数加载，在前端设置页面中配置。
