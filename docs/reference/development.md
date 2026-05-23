<!-- generated-by: gsd-doc-writer -->

# 开发指南

本地开发、构建、测试和提交前检查的完整参考。如果你是首次参与，请先阅读 [Getting Started](../getting-started.md) 完成环境搭建。

## 本地开发环境搭建

### 前置条件

- Go 1.22+
- Node.js + pnpm
- Docker（用于运行 PostgreSQL）
- PostgreSQL with pgvector（通过 Docker 启动）

### 启动顺序

1. **先启动 PostgreSQL**（使用 Docker）：

```bash
docker run -d --name rss-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=rss_reader pgvector/pgvector:pg18-trixie
```

或使用项目自带的 docker-compose：

```bash
docker compose up -d
```

> SQLite 版本已归档到 `sqlite` 分支，主分支不再支持。

2. **再启动后端** — 后端需要先初始化数据库和调度器：

```bash
cd backend-go
go mod tidy
go run cmd/server/main.go
```

后端默认运行在 `http://localhost:5000`，首次启动会自动连接 PostgreSQL 数据库并执行版本化迁移。

开发时日志现在按级别分流：常规运行日志和 warning 走 `stdout`，error / fatal / panic 走 `stderr`。如果你在 PowerShell、Docker 或 systemd 里单独收集错误输出，可以直接利用这条分流。

3. **再启动前端**（新终端）：

```bash
cd front
pnpm install
pnpm dev
```

前端开发服务器运行在 `http://localhost:3000`。

4. **验证联调** — 打开 `http://localhost:3000`，确认前端能连接到 `http://localhost:5000/api`。

### Docker Compose 启动

#### 仅启动 PostgreSQL 数据库（开发用）

```bash
docker compose up -d
```

这会启动一个 pgvector 容器，端口和数据目录可在 `.env` 中配置。

#### SQLite 模式（已归档，仅供参考）

```bash
cp .env.example .env
docker compose -f docker-compose.sqlite.yml up --build
```

### 配置说明

本地开发无需任何配置文件或 `.env` 文件即可启动——后端和前端均有开箱即用的默认值。

后端配置文件位于 `backend-go/configs/config.yaml`，通过 Viper 加载，环境变量可覆盖文件值。详见 [Configuration](configuration.md)。

AI 相关设置（LLM、Firecrawl、Digest）通过 Web UI 的设置页面配置，存储在数据库中，无需手动编辑配置文件。

## 构建命令

### 前端命令（在 `front/` 目录执行）

| 命令 | 说明 |
|------|------|
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
pnpm test:unit -- app/utils/articleContentSource.test.ts
```

按测试名称过滤：

```bash
pnpm test:unit -- app/utils/articleContentSource.test.ts -t "prefers firecrawl"
```

### 后端命令（在 `backend-go/` 目录执行）

| 命令 | 说明 |
|------|------|
| `go mod tidy` | 整理 Go 模块依赖 |
| `go run cmd/server/main.go` | 启动后端服务 |
| `go build ./...` | 编译所有包 |
| `go test ./...` | 运行所有 Go 测试 |
| `go vet ./...` | Go 官方静态检查 |
| `golangci-lint run ./...` | 综合静态分析（staticcheck, revive, gosec 等） |

运行单个包的测试：

```bash
go test ./internal/domain/feeds -v
```

运行单个测试：

```bash
go test ./internal/domain/feeds -run TestBuildArticleFromEntryTracksOnlyRunnableStates -v
```

### 辅助工具命令

| 命令 | 目录 | 说明 |
|------|------|------|
| `go run cmd/migrate-digest/main.go` | `backend-go/` | Digest 数据迁移 |
| `go run cmd/test-digest/main.go` | `backend-go/` | Digest 测试入口 |
| `go run cmd/migrate-tags/main.go` | `backend-go/` | 标签数据迁移 |
| `go run cmd/migrate-db/main.go` | `backend-go/` | SQLite → PostgreSQL 数据迁移 |

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

> **注意**：Python 集成测试需要 Go 后端运行在 `localhost:5000`。

### Firecrawl 集成检查（在 `tests/firecrawl/` 目录执行）

先启动后端，然后运行：

```bash
python test_firecrawl_integration.py
```

## 代码风格

本项目已配置 ESLint 代码检查工具。代码风格通过以下方式维持：

### 前端

- TypeScript 全覆盖，新增代码使用 `<script setup lang="ts">`
- ESLint 配置：`front/eslint.config.js`（flat config + typescript-eslint）
- 大部分前端文件不使用分号——保持与周围代码一致的风格
- 源码必须保持 UTF-8 编码
- 质量门禁：`pnpm lint && pnpm exec nuxi typecheck && pnpm build`

### 后端

- 使用 `gofmt` 格式化 Go 代码
- 静态分析：`golangci-lint run ./...`（配置：`backend-go/.golangci.yml`）
- 导入分组：标准库 → 空行 → 第三方库 → 空行 → 本地包
- JSON 字段使用 `snake_case` struct tag
- 导出符号使用 `PascalCase`，私有符号使用 `lowerCamelCase`
- 错误包装使用 `fmt.Errorf("...: %w", err)`
- 后端日志优先复用 `internal/platform/logging`，避免继续用裸 `log.Printf` + 文本前缀人工区分级别

## 目录结构约定

### 前端目录约定

| 目录 | 职责 |
|------|------|
| `front/app/api/` | HTTP 请求层（唯一网络请求边界） |
| `front/app/features/` | 业务逻辑实现主体 |
| `front/app/components/` | 通用可复用 UI 组件 |
| `front/app/composables/` | 跨 feature 的通用 composable |
| `front/app/stores/` | Pinia 状态管理 |
| `front/app/types/` | 领域类型定义 |
| `front/app/utils/` | 常量和纯工具函数 |
| `front/app/pages/` | Nuxt 路由入口（只做挂载，不放业务逻辑） |

### 前端状态约定

- `useApiStore` 是主数据源，其他 store 从中派生
- `useFeedsStore` 和 `useArticlesStore` 只做派生视图
- 不新增 `syncToLocalStores()` 一类的副本同步逻辑

### 前端数据映射约定

- 后端数字 ID 在 API/store 边界转为字符串
- `snake_case → camelCase` 映射集中在 API 或 store 层，不在组件里做
- 字段重命名直接在类型和 store 映射层切换，不在组件里做兼容
- API 返回值统一通过 `ApiResponse<T>` 包装

### 前端样式约定

- 保持 editorial / magazine 主题风格
- 不回退到蓝紫色 SaaS 视觉
- 复用 `app/assets/css/main.css` 里的主题变量
- 对话框、卡片、状态标签优先沿用现有语义类
- 图标使用 Iconify

### 后端目录约定

| 目录 | 职责 |
|------|------|
| `cmd/server/` | 应用入口 |
| `internal/app/` | HTTP 路由、中间件、运行时装配 |
| `internal/domain/` | 业务域逻辑（feeds, articles, summaries, digest, contentprocessing, categories, topicanalysis, topicextraction, topicgraph, topictypes, aiadmin, preferences 等） |
| `internal/domain/models/` | GORM 数据模型 |
| `internal/jobs/` | 后台调度任务 |
| `internal/platform/` | 共享基础设施（config, database, ws, ai, airouter, aisettings, middleware, tracing, opennotebook） |
| `configs/` | 配置文件 |

业务逻辑放在 `internal/domain/*`，HTTP 路由注册在 `internal/app/router.go`，不在 handler 中写复杂业务。

## 测试

### 前端测试

- **框架**：Vitest（单元测试）、Playwright（E2E 测试）
- **单元测试配置**：`front/vitest.config.ts`，使用 `happy-dom` 环境
- **E2E 测试配置**：`front/playwright.config.ts`，针对 Chromium
- **测试文件命名**：`*.test.ts`，与被测文件同目录
- **运行全部单元测试**：`pnpm test:unit`
- **运行 E2E 测试**：`pnpm test:e2e`

### 后端测试

- **框架**：Go 标准 `testing` 包，`testify` 用于部分断言
- **测试文件命名**：`*_test.go`，与被测文件同目录
- **偏好 table-driven 测试**
- **运行全部测试**：`go test ./...`
- **运行单个包**：`go test ./internal/domain/feeds -v`

### 集成测试

位于 `tests/workflow/`，使用 Python + pytest，验证后端调度器和完整工作流。这些测试需要后端运行在 `localhost:5000`。

## 提交前检查

### 前端改动

至少执行以下其中一项：

```bash
pnpm lint                      # ESLint 检查
pnpm build                     # 生产构建
pnpm exec nuxi typecheck       # TypeScript 类型检查
pnpm test:unit                 # 单元测试
```

### 后端改动

至少执行以下其中一项：

```bash
golangci-lint run ./...        # 综合静态分析
go vet ./...                   # Go 官方静态检查
go build ./...                 # 编译检查
go test ./internal/domain/feeds -v   # 针对范围的单元测试
go test ./...                  # 全量测试
```

### 文档改动

如果改动涉及功能、接口或结构变化，需同步更新对应文档：

- `docs/reference/architecture/frontend.md`
- `docs/reference/architecture/backend.md`
- `docs/reference/architecture/data-flow.md`
- `docs/reference/content-processing.md`
- `docs/reference/database/DATABASE_FIELDS.md`

## Branch 规范与 PR 流程

### Branch 规范

本项目没有文档化的分支命名规范。主分支为 `main`。

### PR 流程

本项目没有预配置的 Pull Request 模板（无 `.github/PULL_REQUEST_TEMPLATE.md`）。提交 PR 时请确保：

- 前端改动通过 `pnpm build` 或 `pnpm exec nuxi typecheck`
- 后端改动通过 `go build ./...` 和对应的单元测试
- 文档与代码变更保持同步
- 在 PR 描述中说明改动范围和原因

## 编码注意事项

- 前端源码必须使用 **UTF-8** 编码
- PowerShell 写文件时要显式保持 UTF-8
- 如果构建报 Vue/Vite 编码错误，先检查文件编码，不要先怀疑业务逻辑
- 后端 handler 应返回 `gin.H{"success": bool, "data"|"error"|"message": ...}` 格式
- 不要添加新的 linter、formatter 或其他工具，除非明确要求（已配置 golangci-lint 和 ESLint）

## 相关文档

- [Getting Started](../getting-started.md) — 环境搭建与首次运行
- [Configuration](configuration.md) — 环境变量、配置文件、AI 设置
- [Architecture Overview](architecture/overview.md) — 系统架构、组件关系、数据流
- [Troubleshooting](../operations/troubleshooting.md) — 常见问题排查
- [Database Fields](database/DATABASE_FIELDS.md) — 数据库字段详细说明
