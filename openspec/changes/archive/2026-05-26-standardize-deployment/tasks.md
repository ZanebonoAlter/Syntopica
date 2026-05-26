## 1. SQLite 残留清理

- [x] 1.1 修正 `backend-go/Dockerfile`：将 `DATABASE_DSN=/app/data/syntopica.db` 改为 PostgreSQL 容器服务 DSN `host=postgres user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable`
- [x] 1.2 精简 `.env.example`：移除 `SQLITE_DB_FILE=syntopica.db`，仅保留 `FRONT_PORT=3000`、`BACKEND_PORT=5000`、`POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_PORT`、`TZ`、`GOPROXY`、`NPM_CONFIG_REGISTRY`
- [x] 1.3 清理 `.gitignore`：移除 `backend-go/syntopica.db`、`backend-go/cmd/server/syntopica.db`、`backend-go/syntopica_backup.db`、`data/*.db`、`data/*.db-shm`、`data/*.db-wal`
- [x] 1.4 更新 `docs/reference/architecture/overview.md`：项目结构树中移除 `docker-compose.sqlite.yml` 条目，`data/` 描述改为 "PostgreSQL 数据持久化"
- [x] 1.5 更新 `docs/reference/development.md`：移除 SQLite 模式相关命令（`docker compose -f docker-compose.sqlite.yml`）
- [x] 1.6 更新 `docs/reference/testing.md`：明确说明单元测试使用内存 SQLite 是测试隔离手段，生产仅用 PostgreSQL
- [x] 1.7 更新 `docs/reference/deployment.md`：移除 `SQLiteSpanExporter` 历史命名说明中的困惑点（保留命名但加注释说明是历史命名）

## 2. Crawl4AI 死代码清理

- [x] 2.1 删除 `backend-go/internal/domain/content/crawl4ai_client.go`
- [x] 2.2 清理 `backend-go/internal/app/runtime.go`：移除 `CRAWL_SERVICE_URL` 环境变量读取（第 116-120 行），`InitContentCompletionHandler` 不再传参
- [x] 2.3 清理 `backend-go/internal/domain/content/content_completion_service.go`：移除 `crawlClient *Crawl4AIClient` 字段、`NewContentCompletionService` 的 `crawlBaseURL` 参数、`SetCrawlAPIToken` 方法
- [x] 2.4 清理 `backend-go/internal/domain/content/content_completion_handler.go`：`InitContentCompletionHandler` 不再接收 `crawlBaseURL` 参数
- [x] 2.5 更新 `docs/reference/configuration.md`：移除 `CRAWL_SERVICE_URL` 环境变量说明
- [x] 2.6 验证：`cd backend-go && go vet ./... && go build ./... && go test ./...`

## 3. 核心 docker-compose.yml 补全

- [x] 3.1 在 `docker-compose.yml` 中添加 `backend` 服务：构建自 `backend-go/`，依赖 postgres healthy，环境变量覆盖 `DATABASE_DSN` 指向 `host=postgres`、`SERVER_MODE=release`、`CORS_ORIGINS`，暴露 `${BACKEND_PORT:-5000}:5000`
- [x] 3.2 在 `docker-compose.yml` 中添加 `front` 服务：构建自 `front/`，依赖 backend，环境变量 `API_INTERNAL_BASE=http://backend:5000/api`、`NUXT_PUBLIC_API_ORIGIN=http://localhost:${BACKEND_PORT:-5000}`、`NUXT_PUBLIC_API_BASE=http://localhost:${BACKEND_PORT:-5000}/api`，暴露 `${FRONT_PORT:-3000}:3000`，`build.args` 传入 `NPM_CONFIG_REGISTRY`
- [x] 3.3 定义顶级 `networks: syntopica-net:` 命名网络，三个服务全部加入
- [x] 3.4 为 backend 和 front 添加 `restart: unless-stopped`
- [x] 3.5 验证：`docker compose up --build -d` 后 `http://localhost:3000` 和 `http://localhost:5000/api` 可访问

## 4. docker-compose.firecrawl.yml

- [x] 4.1 创建 `docker-compose.firecrawl.yml`，定义 `firecrawl`（API + worker 合一或分开）、`firecrawl-redis`、`firecrawl-playwright` 服务
- [x] 4.2 Firecrawl API 服务名统一为 `firecrawl`（非 firecrawl-api），暴露端口 3002，`USE_DB_AUTHENTICATION=false`
- [x] 4.3 Redis 配置持久化 volume
- [x] 4.4 所有服务加入 `syntopica-net` 网络（`external: true`）
- [x] 4.5 验证：`docker compose -f docker-compose.firecrawl.yml up -d` 后 `http://localhost:3002` 可访问

## 5. init.sh 脚本简化（AI 连接配置）

> **变更说明**：原设计包含 llama.cpp 下载、GPU 检测、VRAM 推荐、模型下载等逻辑，现简化为纯连接配置。下载/安装引导移至 README（Task 7）。

- [x] 5.1 实现 Phase 1：检查 Docker/docker compose 可用性，交互收集端口/密码（全默认值），从 `.env.example` 生成 `.env`（不覆盖已有值），执行 `docker compose up -d`，轮询等待 postgres healthy + backend `/health` 200
- [x] 5.2 重写 Phase 2 AI 询问：展示选项（Ollama / llama.cpp / 远程 API / 跳过），按类型预设默认端口，收集 IP 地址（默认自动检测本机 IP）、端口、模型名
- [x] 5.3 实现 IP 自动检测：`hostname -I`（Linux）、`ipconfig getifaddr en0`（Mac）、`ipconfig`（Windows Git Bash），作为默认值建议
- [x] 5.4 实现 Ollama 配置：端口默认 11434，收集文本模型名（默认 `qwen3:8b`）和嵌入模型名（默认 `nomic-embed-text`），provider_type=`ollama`
- [x] 5.5 实现 llama.cpp 配置：文本端口默认 8080，嵌入端口默认 8081，model 占位 `loaded-model`，provider_type=`openai_compatible`，api_key=`sk-local`
- [x] 5.6 实现远程 API 配置：收集 base_url、api_key、文本模型名、嵌入模型名
- [x] 5.7 实现 AI 健康检测：curl 向文本端点 `/v1/chat/completions` 发送 `{"model":"...","messages":[{"role":"user","content":"hello"}],"max_tokens":5}`，HTTP 200 通过，非 200 提示重试/跳过
- [x] 5.8 实现 Phase 2 Firecrawl 询问：展示选项（自部署 / 云 API / 跳过）
- [x] 5.9 重写 Phase 3 确认摘要：打印部署摘要（服务列表、AI 连接信息），用户确认后执行
- [x] 5.10 重写种子数据写入：创建两个 Provider（文本+嵌入），按类型设置 provider_type/base_url/api_key/model，绑定路由（article_completion、topic_tagging、open_notebook → 文本 Provider；embedding → 嵌入 Provider），写入 Firecrawl 配置
- [x] 5.11 重写最终输出：打印访问地址、AI 连接信息
- [x] 5.12 删除旧逻辑：移除 `setup_llamacpp`（下载二进制）、`detect_gpu`、`recommend_models`、`download_models` 函数及相关状态变量
- [ ] 5.13 验证：全新环境中执行 `bash init.sh` / `pwsh init.ps1`，确认简化流程通过

## 6. 文档更新

- [x] 6.1 重写 `README.md` 快速开始章节：包含 Docker 依赖说明、init.sh 使用方式、Firecrawl 可选说明
- [x] 6.2 更新 README.md 项目结构：移除 `docker-compose.sqlite.yml`，`data/` 描述改为 PostgreSQL 持久化，新增 `init.sh`、`docker-compose.firecrawl.yml`
- [x] 6.3 更新 `docs/reference/deployment.md`：添加 init.sh 部署流程说明，更新 docker-compose 拓扑图（两个 compose 文件），移除 docker-compose.ai.yml 引用，补充可选组件说明
- [x] 6.4 更新 `docs/reference/configuration.md`：移除 `CRAWL_SERVICE_URL`，补充 `docker-compose.firecrawl.yml` 相关环境变量

## 7. README AI 本地部署引导

- [x] 7.1 新增 README 章节「AI 模型配置指南」：Ollama 安装指引（官网链接、`ollama pull` 命令）、llama.cpp 安装指引（GitHub Releases 页面链接、平台选择说明）
- [x] 7.2 添加 VRAM 推荐模型表：按 GPU VRAM 大小推荐文本/嵌入模型组合（Qwen3 系列），包含 ModelScope 下载链接
- [x] 7.3 添加 llama.cpp 启动命令参考：文本实例命令（含 GPU flag `-ngl`）、嵌入实例命令、端口说明
- [x] 7.4 添加 Docker 网络说明：backend 在 Docker 内需用本机 IP 访问宿主机 AI 服务，不能使用 localhost
- [x] 7.5 更新 README 快速开始中 AI 相关描述：init.sh 仅做连接配置，安装和模型下载参见配置指南
