# 快速开始

## 前置条件

| 工具 | 最低版本 | 说明 |
|------|----------------|-------|
| [Node.js](https://nodejs.org/) | >= 18 | Nuxt 4 前端必需 |
| [pnpm](https://pnpm.io/) | >= 10 | 前端包管理器 |
| [Go](https://go.dev/) | >= 1.25 | Gin 后端必需 |
| [Docker](https://www.docker.com/) | — | 可选，用于容器化部署 |
| [Git](https://git-scm.com/) | — | 克隆仓库 |
| [Python](https://www.python.org/) | >= 3.10 | 可选，用于运行 `tests/workflow/` 中的集成测试 |

本地开发无需 `.env` 文件 — 后端和前端都有可用的默认值。

## 安装步骤

### 1. 克隆仓库

```bash
git clone <repository-url>
cd my-robot
```

### 2. 启动 PostgreSQL

本地开发需要 PostgreSQL + pgvector 扩展。通过 Docker 快速启动：

```bash
docker run -d --name rss-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=syntopica \
  pgvector/pgvector:pg18-trixie
```

### 3. 启动后端

```bash
cd backend-go
go mod tidy
go run cmd/server/main.go
```

后端在 `http://localhost:5000` 启动，连接到本地 PostgreSQL 数据库。首次运行时 GORM 会自动迁移所有表。

### 4. 启动前端

新开一个终端：

```bash
cd front
pnpm install
pnpm dev
```

前端开发服务器在 `http://localhost:3000` 启动。

### 5. 验证连接

在浏览器中打开 `http://localhost:3000`。前端应该加载并连接到 `http://localhost:5000/api`。可以立即开始添加 RSS 订阅源。

## Docker Compose（替代方案）

如果希望容器化部署，一条命令启动所有服务：

```bash
docker compose up --build -d
```

启动三个服务：
- **postgres**: PostgreSQL + pgvector，数据持久化在 `./data/`
- **backend**: Go API 服务器端口 5000
- **front**: Nuxt SSR 服务器端口 3000

端口映射和其他 Docker 设置可通过 `.env` 文件自定义 — 详见 [配置指南](reference/configuration.md)。

## 首次使用

两个服务都运行后：

1. 在浏览器中打开 `http://localhost:3000`。
2. 通过订阅管理面板添加 RSS feed。
3. Feed 会被抓取，文章出现在三栏阅读布局中。
4. （可选）通过 Web UI 设置页面配置 AI 功能 — LLM API key、Firecrawl、Digest 设置。这些存储在数据库中，不需要编辑配置文件。

## 常见问题

### 端口已被占用

如果 `http://localhost:5000` 或 `http://localhost:3000` 被占用，通过环境变量设置端口：

- 后端：运行 `go run cmd/server/main.go` 前设置 `SERVER_PORT`。
- 前端：如果后端运行在非默认端口，设置 `NUXT_PUBLIC_API_BASE` 环境变量。
- Docker：在 `.env` 中设置 `FRONT_PORT` 和 `BACKEND_PORT`。

### 后端启动失败（数据库错误）

后端默认连接 PostgreSQL。确保 PostgreSQL 正在运行且 DSN 配置正确。如果使用 Docker 启动的 PostgreSQL，检查容器是否正常运行：

```bash
docker ps | grep rss-postgres
```

### 前端无法连接后端

确保后端在 `http://localhost:5000` 运行。前端 API 基础 URL 默认为 `http://localhost:5000/api`，如有需要可通过 `NUXT_PUBLIC_API_BASE` 环境变量覆盖。

### Go 模块下载失败（中国地区）

如果 `go mod tidy` 慢或失败，设置 Go 模块代理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

pnpm 也可以设置镜像：

```bash
pnpm config set registry https://registry.npmmirror.com
```

### Docker 构建代理设置

在代理后构建 Docker 镜像时，在 `.env` 文件中配置 `GOPROXY`、`NPM_CONFIG_REGISTRY`、`HTTP_PROXY` 和 `HTTPS_PROXY`。这些会传递到构建上下文。

## 下一步

- **[配置指南](reference/configuration.md)** — 完整的环境变量列表、配置文件选项和数据库存储的 AI 设置。
- **[开发指南](reference/development.md)** — 构建命令、测试命令、编码规范和提交检查清单。
- **[架构概览](reference/architecture/overview.md)** — 系统设计、组件关系、数据流和后台调度器详情。
