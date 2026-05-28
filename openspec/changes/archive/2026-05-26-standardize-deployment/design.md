## Context

Syntopica 是单用户自托管 RSS 阅读器，技术栈为 Go 后端 (Gin/GORM) + Nuxt 4 前端 + PostgreSQL/pgvector。当前部署状态：

- `docker-compose.yml` 仅定义 postgres 服务，backend/front 未加入
- 两个 Dockerfile 存在但未接入 compose（backend Dockerfile 有 SQLite 残留 DSN）
- AI 配置（Provider、路由、Firecrawl）全部存数据库，通过 Web UI 手动填写，无预设默认值
- Crawl4AI 集成已废弃但代码残留未清理（`crawl4ai_client.go`、`CRAWL_SERVICE_URL`）
- 用户需要分别手动启动 postgres、后端、前端，再进 Web UI 配置 AI，上手门槛高

目标用户：希望快速部署使用的其他用户（非开发者），可能有 NVIDIA GPU。

## Goals / Non-Goals

**Goals:**
- 一条命令 `bash init.sh` 从零到可用状态
- 核心服务（PG + backend + front）零配置启动，全部默认值即可工作
- 可选组件（AI 连接、Firecrawl）逐步询问，每步可跳过
- AI 配置简化为连接配置：按类型（Ollama/llama.cpp/远程 API）预设默认端口，收集 IP 和模型名
- AI 健康检测：向文本端点发送 "hello" chat 请求验证可达
- README 提供 AI 本地部署引导（Ollama/llama.cpp 安装、GPU 检测、模型推荐、启动命令）
- 清理所有 SQLite 残留和 Crawl4AI 死代码
- README 快速开始章节重写，包含完整依赖说明

**Non-Goals:**
- 不支持 SQLite（已归档 sqlite 分支）
- 不做 CI/CD 流水线
- 不做 PaaS 部署
- 不做自动更新/升级机制
- init.sh 不管理 AI 进程生命周期（不下载、不启动、不守护 AI 服务）
- 不做 Provider 持续健康监控后端 API（仅 init.sh 时检测一次）
- 不提供 Windows PowerShell 版脚本（统一 bash，Windows 用户用 Git Bash）
- init.sh 不下载 llama.cpp 二进制或模型文件（移至 README 引导）

## Decisions

### D1: 两个 Docker Compose 文件

**决定**: 核心 `docker-compose.yml` + Firecrawl `docker-compose.firecrawl.yml`

**替代方案**: 三个文件（含 AI compose）/ 单文件 + Docker profiles

**理由**: AI 推理原生运行，不需要 Docker compose 文件。两个文件语义清晰，用户选择性启动 Firecrawl。

### D2: init.sh 三阶段流程

**决定**: Phase 1 核心启动 → Phase 2 逐步询问 + AI 连接配置 → Phase 3 确认后批量执行

**替代方案**: 每选一项立即部署写入

**理由**: Phase 2 结束时通过 AI 健康检测确认服务可用，再统一写入种子数据。用户在最终确认时看到完整摘要。

### D3: AI 配置按类型预设默认端口（连接配置，非下载）

**决定**: init.sh 的 AI 部分仅做连接配置，不下载任何二进制或模型文件

按类型预设：

| 类型 | provider_type | 默认端口 | api_key | model 字段 |
|------|--------------|---------|---------|-----------|
| Ollama | `ollama` | 11434（单一端口） | 空（Ollama 类型免校验） | 用户填写（如 `qwen3:8b`） |
| llama.cpp | `openai_compatible` | 8080（文本）+ 8081（嵌入） | `sk-local`（占位） | `loaded-model`（占位，服务端忽略） |
| 远程 API | `openai_compatible` | 用户自定义 | 用户填写 | 用户填写 |

用户可自定义 IP 和端口。IP 默认自动检测本机 IP（如 `192.168.5.3`），用户可改为 `localhost`。

**替代方案**: init.sh 下载 llama.cpp 二进制 + 模型文件 + GPU 检测推荐（原设计）

**理由**: 下载逻辑复杂脆弱（URL 过期、平台检测、ModelScope 网络），且用户可能已有自己的 AI 服务。改为纯连接配置后 init.sh 简单可靠，下载引导移至 README。

Docker 网络注意：backend 在 Docker 容器内，AI 服务在宿主机。`localhost` 从容器内无法访问宿主机，需使用本机 IP（如 `192.168.5.3`）。init.sh 自动检测本机 IP 作为默认值。

### D4: 种子数据通过后端 API 写入

**决定**: init.sh 用 curl 调用后端现有 API 写入 AI Provider、路由、Firecrawl 配置

**替代方案**: 直接 SQL 写数据库 / 后端内置种子逻辑

**理由**: 后端 API 已完备（`POST /api/ai/providers`、`PUT /api/ai/routes/:capability`、`POST /api/firecrawl/settings`），走 API 保证数据校验一致性。不需要改后端代码。

Ollama 类型：api_key 留空（后端 `ProviderTypeOllama` 免校验）。
llama.cpp / 远程类型：api_key 传用户值或占位值 `sk-local`。

写入结构：
- 文本 Provider：绑定到 `article_completion`、`topic_tagging`、`open_notebook` 路由
- 嵌入 Provider：绑定到 `embedding` 路由
- Ollama 模式：两个 Provider 共享同一 base_url，靠 model 字段区分文本/嵌入
- llama.cpp 模式：两个 Provider 各自 base_url（端口不同）

### D5: AI 健康检测 — 发送 "hello"

**决定**: init.sh 通过 curl 向文本端点发送最小化 chat 请求验证服务可达

```bash
curl -s -w "\n%{http_code}" \
  http://${AI_IP}:${TEXT_PORT}/v1/chat/completions \
  -d '{"model":"${MODEL}","messages":[{"role":"user","content":"hello"}],"max_tokens":5}'
```

HTTP 200 → 通过。非 200 或超时 → 提示重试/跳过。

**替代方案**: 简单 health 端点检测（`/health`、`/api/tags`）

**理由**: 发送实际 chat 请求能验证模型已加载且可推理，比 health 端点更可靠。成本极低（5 token）。

### D6: Firecrawl 自部署独立 compose

**决定**: Firecrawl 自部署放在 `docker-compose.firecrawl.yml`，包含 api + worker + redis + playwright 四个服务

**理由**: Firecrawl 栈重量级（4 容器），与核心服务解耦。用户可选择云 API 替代。

### D7: Crawl4AI 死代码清理

**决定**: 清理 Crawl4AI 相关代码和配置

清理范围：
- `backend-go/internal/domain/content/crawl4ai_client.go`：整个文件删除
- `backend-go/internal/app/runtime.go`：移除 `CRAWL_SERVICE_URL` 环境变量读取和 `InitContentCompletionHandler` 调用
- `backend-go/internal/domain/content/content_completion_service.go`：移除 `crawlClient` 字段及 `SetCrawlAPIToken` 方法
- `backend-go/internal/domain/content/content_completion_handler.go`：`InitContentCompletionHandler` 不再接收 `crawlBaseURL` 参数
- 文档中 `CRAWL_SERVICE_URL` 相关说明

**理由**: Crawl4AI 已废弃，`crawlClient` 仅有 `SetAPIToken` 调用无实际 crawl 调用，属于死代码。清理后减少维护负担和用户困惑。

### D8: README 提供 AI 本地部署引导

**决定**: 原本 init.sh 中的下载、GPU 检测、模型推荐逻辑移至 README 作为引导推荐

README 引导内容：
- Ollama 安装指引（官网链接、基本命令）
- llama.cpp 安装指引（GitHub Release 页面、平台选择）
- VRAM 推荐模型表（保留原推荐表）
- 模型文件下载来源（ModelScope 链接、HuggingFace 链接）
- llama.cpp 启动命令参考（文本 + 嵌入双实例）
- Docker 网络说明（容器内需用本机 IP 而非 localhost）

**理由**: 引导信息不需要脚本化，README 形式更灵活、可维护。用户可按需选择，不受脚本逻辑限制。

### D9: 脚本统一 bash，不分平台

**决定**: 单一 `init.sh` bash 脚本，Windows 用户通过 Git Bash 执行

**理由**: 避免维护两套脚本逻辑。Git Bash 在 Windows 开发者中普遍安装。使用 POSIX 兼容语法，避免 bash-only 扩展。

## Risks / Trade-offs

- **[Docker 网络]** → backend 在 Docker 内访问宿主机 AI 服务需用本机 IP，用户填 `localhost` 会导致连接失败。init.sh 应自动检测并提示
- **[模型推荐主观性]** → README 推荐表基于经验估算，实际占用受 KV cache 和 context length 影响
- **[Windows Git Bash 兼容]** → 部分 bash 特性在 Git Bash 下行为可能不同，需测试
- **[init.sh 幂等性]** → .env 合并不覆盖已有值、Provider upsert、重复运行安全
- **[Crawl4AI 清理范围]** → 需确认无其他模块引用 crawlClient，避免清理导致编译失败

## Open Questions

<!-- 无未解决问题 -->
