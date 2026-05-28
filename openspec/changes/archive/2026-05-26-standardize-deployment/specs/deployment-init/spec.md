## ADDED Requirements

### Requirement: init.sh 作为部署唯一入口
系统 SHALL 提供 `init.sh` 脚本作为用户部署 Syntopica 的唯一入口。脚本 SHALL 在项目根目录执行，使用 bash（Windows 用户通过 Git Bash）。

#### Scenario: 用户首次部署
- **WHEN** 用户执行 `bash init.sh`
- **THEN** 脚本启动三阶段流程：核心服务启动 → AI 连接配置 + Firecrawl → 确认并执行

#### Scenario: init.sh 不存在时
- **WHEN** 项目仓库被 clone 但 init.sh 未执行
- **THEN** 用户可按 README 手动执行 `docker compose up -d`，init.sh 非强制

#### Scenario: 重复运行 init.sh
- **WHEN** 用户第二次执行 init.sh
- **THEN** .env 合并不覆盖已有值，Provider 通过 upsert 不重复创建

### Requirement: Phase 1 核心服务零配置启动
init.sh Phase 1 SHALL 启动 postgres + backend + front 三个核心服务，全部使用默认值即可工作。脚本 SHALL 检查 Docker 和 docker compose 是否可用。

#### Scenario: 核心服务启动成功
- **WHEN** 用户在 Phase 1 接受所有默认值
- **THEN** 脚本生成 `.env` 文件（不覆盖已有），执行 `docker compose up -d`，等待 postgres healthy 和 backend `/health` 返回 200

#### Scenario: Docker 不可用
- **WHEN** 系统未安装 Docker 或 docker compose
- **THEN** 脚本输出错误信息并终止，提示安装 Docker

#### Scenario: 端口被占用
- **WHEN** 默认端口 5000 或 3000 被占用
- **THEN** 脚本提示用户输入替代端口

### Requirement: Phase 2 AI 连接配置
init.sh Phase 2 SHALL 逐步询问用户是否配置 AI 模型和 Firecrawl。每个配置项 SHALL 提供"跳过"选项。AI 配置为纯连接配置，不下载二进制或模型文件。

#### Scenario: 用户跳过所有可选组件
- **WHEN** 用户在 AI 和 Firecrawl 两个询问中都选择跳过
- **THEN** 脚本跳过 Phase 3，直接打印访问地址，用户可后续在 Web UI 配置

#### Scenario: 用户选择 Ollama
- **WHEN** 用户选择 Ollama
- **THEN** 脚本收集 IP 地址（默认自动检测本机 IP）、端口（默认 11434）、文本模型名（默认 `qwen3:8b`）、嵌入模型名（默认 `nomic-embed-text`）

#### Scenario: 用户选择 llama.cpp
- **WHEN** 用户选择 llama.cpp
- **THEN** 脚本收集 IP 地址（默认自动检测本机 IP）、文本端口（默认 8080）、嵌入端口（默认 8081）。模型名使用占位值 `loaded-model`，api_key 使用 `sk-local`

#### Scenario: 用户选择远程 API
- **WHEN** 用户选择远程 OpenAI 兼容 API
- **THEN** 脚本交互式收集 base_url、api_key、文本模型名、嵌入模型名

#### Scenario: IP 地址自动检测
- **WHEN** init.sh 进入 AI 配置步骤
- **THEN** 脚本自动检测本机 IP（Linux: `hostname -I`、Mac: `ipconfig getifaddr en0`、Windows Git Bash: `ipconfig`），作为 IP 默认值建议。用户可手动修改为 `localhost` 或其他地址

### Requirement: AI 健康检测 — 发送 hello
init.sh SHALL 通过 curl 向文本端点发送最小化 chat 请求验证 AI 服务可达。

#### Scenario: 健康检测通过
- **WHEN** curl 向 `http://{IP}:{TEXT_PORT}/v1/chat/completions` 发送 `{"model":"...","messages":[{"role":"user","content":"hello"}],"max_tokens":5}` 返回 HTTP 200
- **THEN** 脚本继续写入 Provider 和 Route 种子数据

#### Scenario: 健康检测失败
- **WHEN** AI 服务不可达或返回非 200
- **THEN** 脚本提供"重试"和"跳过（稍后在 Web UI 配置）"两个选项

#### Scenario: 远程 API 跳过检测
- **WHEN** 用户选择远程 API
- **THEN** 跳过健康检测（用户保证服务可用），直接写入种子数据

### Requirement: Phase 3 确认并执行
init.sh Phase 3 SHALL 展示部署摘要（服务列表、AI 连接信息），用户确认后执行。

#### Scenario: 用户确认部署
- **WHEN** 用户确认部署摘要
- **THEN** 脚本通过 curl 写入种子数据（创建 Provider、绑定路由、写入 Firecrawl 配置）

#### Scenario: 用户取消
- **WHEN** 用户在确认步骤选择取消
- **THEN** 核心服务保持运行，可选组件不部署

### Requirement: 种子数据通过后端 API 写入
init.sh SHALL 通过 curl 调用后端现有 API 写入 AI Provider、路由绑定和 Firecrawl 配置。

#### Scenario: Ollama 模式写入
- **WHEN** 用户选择 Ollama 且健康检测通过
- **THEN** 创建两个 Provider（provider_type=`ollama`，同一 base_url，不同 model），api_key 留空。绑定 article_completion/topic_tagging/open_notebook → 文本 Provider，embedding → 嵌入 Provider

#### Scenario: llama.cpp 模式写入
- **WHEN** 用户选择 llama.cpp 且健康检测通过
- **THEN** 创建两个 Provider（provider_type=`openai_compatible`，不同 base_url/端口，model 占位 `loaded-model`，api_key=`sk-local`）。绑定路由同上

#### Scenario: 写入 Firecrawl 种子
- **WHEN** 用户选择自部署 Firecrawl 且服务健康
- **THEN** 脚本调用 `POST /api/firecrawl/settings` 写入 `api_url` 指向 `http://firecrawl:3002`

#### Scenario: 用户选择远程 API
- **WHEN** 用户选择远程 OpenAI 兼容 API
- **THEN** 脚本直接写入 Provider 和路由（无需健康检测）

### Requirement: README AI 本地部署引导
README SHALL 提供 AI 模型本地部署的引导推荐，包含安装指引、模型推荐表和启动命令参考。

#### Scenario: 用户需要安装 Ollama
- **WHEN** 用户查阅 README 的 AI 配置指南
- **THEN** 可看到 Ollama 官网链接、基本使用命令（ollama pull/serve）、推荐模型列表

#### Scenario: 用户需要安装 llama.cpp
- **WHEN** 用户查阅 README 的 AI 配置指南
- **THEN** 可看到 GitHub Releases 页面链接、平台选择说明、VRAM 推荐模型表、启动命令参考

#### Scenario: 用户使用 Docker 部署后端
- **WHEN** 后端运行在 Docker 容器内
- **THEN** README 说明需使用本机 IP（非 localhost）访问宿主机上的 AI 服务

### Requirement: SQLite 残留清理
部署相关文件 SHALL 移除所有 SQLite 引用，仅支持 PostgreSQL。

#### Scenario: Dockerfile 修正
- **WHEN** backend Dockerfile 中 DATABASE_DSN 指向 SQLite
- **THEN** 改为 PostgreSQL 容器服务名

#### Scenario: .env.example 清理
- **WHEN** .env.example 包含 SQLITE_DB_FILE 变量
- **THEN** 移除该变量

### Requirement: Crawl4AI 死代码清理
部署变更 SHALL 清理已废弃的 Crawl4AI 集成代码。

#### Scenario: crawl4ai_client.go 删除
- **WHEN** `crawl4ai_client.go` 文件存在
- **THEN** 删除该文件

#### Scenario: CRAWL_SERVICE_URL 移除
- **WHEN** runtime.go 读取 CRAWL_SERVICE_URL 环境变量
- **THEN** 移除相关代码，InitContentCompletionHandler 不再接收 crawlBaseURL 参数

#### Scenario: 编译验证
- **WHEN** Crawl4AI 代码清理完成
- **THEN** `go vet ./...` 和 `go build ./...` 通过，无编译错误
