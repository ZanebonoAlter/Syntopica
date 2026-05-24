## Why

Syntopica 当前缺少标准化的部署流程：docker-compose.yml 仅定义了 postgres，backend/front 的 Dockerfile 存在 SQLite 残留，用户需要手动配置 AI 模型和 Firecrawl。对于希望快速上手的其他用户来说，从 clone 到可用状态的路径不清晰、步骤分散。

## What Changes

- 新建 `init.sh` 作为部署唯一入口，交互式引导用户完成核心服务启动和可选组件配置
- 补全 `docker-compose.yml` 加入 backend 和 front 服务定义
- 新建 `docker-compose.firecrawl.yml` 提供 Firecrawl 自部署栈
- 清理所有 SQLite 残留（Dockerfile、.env.example、README、.gitignore）
- init.sh AI 配置简化：不下载二进制/模型，仅按类型（Ollama/llama.cpp）预设默认端口，收集 IP + 模型名，通过后端 API 写入种子配置
- 健康检测通过发送 "hello" chat 请求验证文本端点可达
- AI 模型安装（Ollama/llama.cpp 二进制下载、GPU 检测、模型推荐表）移至 README 作为引导推荐

## Capabilities

### New Capabilities
- `deployment-init`: 交互式部署入口脚本，覆盖核心启动、AI 连接配置（按类型预设端口）、种子数据写入
- `compose-firecrawl`: Firecrawl 自部署 Docker Compose 配置（独立文件）

### Modified Capabilities
<!-- 无现有 specs 需要修改 -->

## Impact

- `docker-compose.yml`：加入 backend、front 服务，修正 postgres 服务依赖
- `backend-go/Dockerfile`：修正 DATABASE_DSN 环境变量（SQLite → PG）
- `front/Dockerfile`：无需修改
- `.env.example`：移除 SQLite 变量，补充 Docker Compose 和 AI 相关变量
- `README.md`：更新项目结构描述、快速开始章节，新增 AI 本地部署引导（Ollama/llama.cpp 安装、模型推荐表、启动命令参考）
- `.gitignore`：清理 SQLite 相关条目（可选）
- 新文件：`init.sh`、`docker-compose.firecrawl.yml`
- 后端 API 无需改动，种子数据通过现有 `/api/ai/providers`、`/api/ai/routes`、`/api/firecrawl/settings` 写入
- `backend-go/internal/platform/airouter/store.go`：api_key 校验对本地模型需支持占位值（`sk-local`）
