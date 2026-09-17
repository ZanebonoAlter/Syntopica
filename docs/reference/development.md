# 开发指南

本地开发与构建命令参考。如果你是首次参与，本文件涵盖从环境搭建到构建命令的完整步骤。

> **规范已迁移**：代码风格、目录约定、测试规范、提交前检查、Branch/PR 流程、编码注意事项的**权威定义**已迁至 [`standard/`](standard/README.md)（前后端分文件夹）。本文件只保留**环境搭建**与**构建命令**。

## 本地开发环境搭建

### 前置条件

| 工具 | 最低版本 | 说明 |
| ------ | ---------------- | ------- |
| [Go](https://go.dev/) | >= 1.25 | Gin 后端必需 |
| [Node.js](https://nodejs.org/) | >= 18 | Nuxt 4 前端必需 |
| [pnpm](https://pnpm.io/) | >= 10 | 前端包管理器 |
| [Docker](https://www.docker.com/) | — | 用于运行 PostgreSQL |
| [Git](https://git-scm.com/) | — | 克隆仓库 |
| [Python](https://www.python.org/) | >= 3.10 | 可选，用于运行 `tests/workflow/` 中的集成测试 |

### 启动顺序

1. **先启动 PostgreSQL**（使用 Docker）：

```bash
docker run -d --name rss-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=syntopica pgvector/pgvector:pg18-trixie
```

或使用项目自带的 docker-compose：

```bash
docker compose up -d
```

> SQLite 版本已归档到 `sqlite` 分支，主分支不再支持。

1. **再启动后端** — 后端需要先初始化数据库和调度器：

```bash
cd backend-go
go mod tidy
go run cmd/server/main.go
```

后端默认运行在 `http://localhost:5100`（避开 Windows 5000 端口的 WSD/svchost 保留段冲突），首次启动会自动连接 PostgreSQL 数据库并执行版本化迁移。

开发时日志现在按级别分流：常规运行日志和 warning 走 `stdout`，error / fatal / panic 走 `stderr`。如果你在 PowerShell、Docker 或 systemd 里单独收集错误输出，可以直接利用这条分流。

1. **再启动前端**（新终端）：

```bash
cd front
pnpm install
pnpm dev
```

前端开发服务器运行在 `http://localhost:3000`。

> **一把梭（推荐）**：`bash scripts/start-dev.sh`（仓库根目录跑）把后端 + 前端一起起来，并**自动注入这两个容易漏的变量**——同源反代模式下注入前端 `NUXT_PUBLIC_API_BASE=/api`，同时给后端带上包含本机全部网段 IP 的 `CORS_ORIGINS`；已装的同源入口时还会把入口地址列出来。`--restart` 先停再起、`stop` 停掉、`status` 看端口/PID/健康/入口。两个变量漏一个就换一种报错（前端报 `ERR_CONNECTION_REFUSED` 或 CORS 拦），详见 [部署指南](deployment.md)「多机 / 远程访问」。

1. **验证联调** — 打开 `http://localhost:3000`，确认页面正常且 Network 面板中 API 请求直连 `http://localhost:5100/api`、WebSocket 连 `ws://localhost:5100/ws`。

### 首次使用

两个服务都运行后：

1. 在浏览器中打开 `http://localhost:3000`。
2. 通过订阅管理面板添加 RSS feed。
3. Feed 会被抓取，文章出现在三栏阅读布局中。
4. （可选）通过 Web UI 的设置页面配置 AI 功能 — LLM API key、Firecrawl、Digest 设置。这些存储在数据库中，无需编辑配置文件。

### Docker Compose 启动

#### 仅启动 PostgreSQL 数据库（开发用）

```bash
docker compose up -d
```

这会启动一个 pgvector 容器，端口和数据目录可在 `.env` 中配置。

#### Docker 镜像来源与可达性（中国大陆网络）

Docker Hub（`registry-1.docker.io`）在国内常不可达，而本仓库的 compose 与集成测试都按**原始镜像名**引用镜像（如 `pgvector/pgvector:pg18-trixie`）。因此**可达性在开发主机层面解决，不写进仓库**：仓库内不得出现加速站域名，也不得改写镜像名——否则换机/CI 即失效。

**做法**：给 Docker 守护进程配镜像加速来源（多列几个按序回退）：

```bash
# /etc/docker/daemon.json（不存在则新建）
{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.m.daocloud.io",
    "https://docker.1panel.live"
  ]
}
sudo systemctl restart docker
# 验证：docker info | grep -A5 'Registry Mirrors'
```

> ⚠️ **上面的域名是示例，不保证长期有效**（国内加速站开关频繁）。配前先自行验证当前可达性：`docker pull <域名>/library/hello-world` 能成功即可用；失效则换其他来源或走备选路径②。**不要把域名复制进仓库代码/配置文件**——仓库只按原始镜像名引用镜像。

配好后 `docker pull pgvector/pgvector:pg18-trixie` 等原始镜像名可直接拉取，`docker compose` 与 testcontainers 均无需任何改动。

**备选路径**（加速站失效时）：① 换用其他已实测可达的加速站；② 给 Docker 配 HTTP 代理（`/etc/systemd/system/docker.service.d/http-proxy.conf` 里的 `HTTP_PROXY`/`HTTPS_PROXY`）；③ 应急：带前缀拉取后重打 tag（`docker pull <mirror>/xxx && docker tag <mirror>/xxx xxx`），仅救急、不作长期方案。

> 集成测试必需的镜像清单，以及「镜像缺失导致容器泄漏」的识别与清理，见 [testing.md](testing.md) 集成测试节。

### 配置说明

本地开发无需任何配置文件或 `.env` 文件即可启动——后端和前端均有开箱即用的默认值。

后端配置文件位于 `backend-go/configs/config.yaml`，通过 Viper 加载，环境变量可覆盖文件值。详见 [Configuration](configuration.md)。

AI 相关设置（LLM、Firecrawl、Digest）通过 Web UI 的设置页面配置，存储在数据库中，无需手动编辑配置文件。

## 构建命令

### 前端命令（在 `front/` 目录执行）

| 命令 | 说明 |
| ------ | ------ |
| `pnpm install` | 安装依赖 |
| `pnpm dev` | 启动开发服务器（`http://localhost:3000`） |
| `pnpm build` | 生产构建 |
| `pnpm generate` | 静态站点生成 |
| `pnpm preview` | 预览生产构建 |
| `pnpm lint` | ESLint 代码检查 |
| `pnpm exec nuxi typecheck` | TypeScript 类型检查 |
| `pnpm test:unit` | 运行 Vitest 单元测试 |
| `pnpm test:e2e` | 运行 Playwright E2E 测试 |
| `pnpm test:e2e:ui` | Playwright 测试 UI 模式 |

运行单个单元测试文件：

```bash
pnpm test:unit app/utils/articleContentSource.test.ts
```

> 不要写成 `pnpm test:unit -- <参数>`：`--` 会被 vitest 吞掉、filter 静默失效并跑成全量（2026-09-17 实测，见 [`standard/frontend/testing.md`](standard/frontend/testing.md)）。

按测试名称过滤：

```bash
pnpm test:unit app/utils/articleContentSource.test.ts -t "prefers firecrawl"
```

### 后端命令（在 `backend-go/` 目录执行）

| 命令 | 说明 |
| ------ | ------ |
| `go mod tidy` | 整理 Go 模块依赖 |
| `go run cmd/server/main.go` | 启动后端服务 |
| `go build ./...` | 编译所有包 |
| `go test ./...` | 运行所有 Go 测试 |
| `go vet ./...` | Go 官方静态检查 |
| `golangci-lint run ./...` | 综合静态分析（staticcheck, revive, gosec 等） |

运行单个包的测试：

```bash
go test ./internal/reader/service -v
```

运行单个测试：

```bash
go test ./internal/reader/service -run TestBuildArticleFromEntryTracksOnlyRunnableStates -v
```

### Python 集成测试（在 `tests/workflow/` 目录执行）

```bash
uv venv
.venv\Scripts\activate    # Windows
uv pip install -r requirements.txt
pytest test_*.py -v
```

运行单个测试文件或测试用例：

```bash
pytest test_schedulers.py -v
pytest test_schedulers.py::TestAutoRefreshScheduler::test_name -v
```

带覆盖率报告：

```bash
pytest --cov=. --cov-report=html
```

> **注意**：Python 集成测试需要 Go 后端可达（默认 `http://localhost:5100`；存在系统代理时探测需绕过：设 `no_proxy=localhost,127.0.0.1,::1`）。

### Firecrawl 集成检查（在 `tests/firecrawl/` 目录执行）

先启动后端，然后运行：

```bash
python test_firecrawl_integration.py
```

### 常见问题

#### 端口已被占用

如果 `http://localhost:5100` 或 `http://localhost:3000` 被占用，通过环境变量设置端口：

- 后端：运行 `go run cmd/server/main.go` 前设置 `SERVER_PORT`（或改 `configs/config.yaml`）。
- 前端：后端非默认端口时设 `NUXT_PUBLIC_API_BASE` 指向实际后端地址。
- Docker：在 `.env` 中设置 `PORT`（宿主映射的应用端口，容器内固定 5000）。旧版 `.env` 里的 `FRONT_PORT` / `BACKEND_PORT` 已不再被读取。
- 想知道是谁占着：`bash scripts/start-dev.sh status`（列端口/PID/健康），`bash scripts/start-dev.sh --restart` 直接接管重起。

#### 后端启动失败（数据库错误）

后端默认连接 PostgreSQL。确保 PostgreSQL 正在运行且 DSN 配置正确。如果使用 Docker 启动的 PostgreSQL，检查容器是否正常运行：

```bash
docker ps | grep rss-postgres
```

#### 前端无法连接后端

前端 API 基础 URL 默认 `http://localhost:5100/api`（绝对直连，后端 CORS 白名单放行前端 origin）。排查顺序：后端是否在 5100 监听（`curl --noproxy '*' http://localhost:5100/health`）→ 是否被系统代理劫持（设 `no_proxy=localhost,127.0.0.1,::1`）→ 需要跨域/远程后端时用 `NUXT_PUBLIC_API_BASE` 覆盖。

#### Go 模块下载失败（中国地区）

如果 `go mod tidy` 慢或失败，设置 Go 模块代理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

pnpm 也可以设置镜像：

```bash
pnpm config set registry https://registry.npmmirror.com
```

#### Docker 构建代理设置

在代理后构建 Docker 镜像时，在 `.env` 文件中配置 `GOPROXY`、`NPM_CONFIG_REGISTRY`、`HTTP_PROXY` 和 `HTTPS_PROXY`。这些会传递到构建上下文。

---

## 规范文档导航

本文件原有的代码风格、目录约定、测试、提交前检查、Branch/PR、编码注意事项等内容已迁入 `standard/`，不再在此重复：

| 原内容 | 权威源 |
| -------- | -------- |
| 代码风格（前端 / 后端） | [`standard/frontend/code-style.md`](standard/frontend/code-style.md)、[`standard/backend/code-style.md`](standard/backend/code-style.md) |
| 目录结构约定 | [`standard/frontend/code-style.md`](standard/frontend/code-style.md) §2、[`standard/backend/package-layout.md`](standard/backend/package-layout.md) |
| 测试规范 | [`standard/frontend/testing.md`](standard/frontend/testing.md)、[`standard/backend/testing.md`](standard/backend/testing.md) |
| 提交前检查 / Branch / PR / 编码注意 | [`standard/shared/commit-pr.md`](standard/shared/commit-pr.md) |

其他参考：[架构总览](architecture/overview.md)、[详细设计地图](architecture/map.md)、[业务流程](flow/README.md)、[测试指南](testing.md)。
