# Syntopica 公开只读 Demo

一个**只读、脱敏、公网可部署**的 Syntopica 演示实例。任何人只需装好 Docker，一条命令即可浏览完整产品形态——无需准备数据库、无需配 AI 凭证、无需添加 RSS 源。

适合给第一次接触 Syntopica 的人快速上手查看：首页文章流、主题图谱、标签管理 + 侦探墙、设置页全部可点开看。

## 快速启动

```bash
# 在仓库根目录执行（构建镜像 + 启动 postgres + 导入脱敏数据）
docker compose -f demo/docker-compose.demo.yml up -d --build

# 等待约 30-60 秒（首次需构建镜像 + 导入 seed），然后访问：
#   http://localhost:5000
```

停止并清理（数据库每次都是全新的，不保留状态）：

```bash
docker compose -f demo/docker-compose.demo.yml down
```

## 这是什么

- **数据来源**：从一个真实运行的 Syntopica 实例导出最近 30 天的业务数据，经保守脱敏后作为只读快照（见 `demo/seed/seed.sql`）。
- **只读**：所有非 GET 请求返回 `405 {"error":"read-only demo"}`；两个会触发后台计算的 GET SSE 端点也被拦截；后台定时任务（RSS 刷新、日报生成、Firecrawl 爬取）全部关闭；WebSocket 未启用。
- **数据安全（文章不会被清理）**：demo 快照的两层防护确保 `articles` 数据稳定不变——① `DEMO_READ_ONLY=1` 时 `main.go` 跳过 `StartRuntime`，所有 scheduler（含按 `MaxArticles` 上限裁剪旧文的 `auto_refresh`→`CleanupOldArticles`）都不注册；② `ReadOnly` 中间件拦截所有写/刷新入口（`POST /feeds/:id/refresh`、`PUT /feeds/:id`、`DELETE /feeds/:id`、`POST /feeds/refresh-all`、`POST /import-opml`）。无任何残留路径会 DELETE articles。
- **脱敏**：AI 凭证（`api_key`/`base_url`/`metadata`）已清空，`ai_settings.value` 配置整体清空，`reading_behaviors.session_id` 已 SHA-256 哈希，错误日志/Firecrawl 原文已清空，URL query 已剥离，正文中的敏感 token 字面量（如 `api_key`）已替换，向量列置 NULL，`ai_call_logs`/`schema_migrations` 等敏感表完全不导出。详见 [`seed/README.md`](seed/README.md)。
- **自包含镜像**：`Dockerfile.demo` 多阶段构建前端（`NUXT_PUBLIC_API_BASE=/api` 同源相对路径）+ 后端 Go 二进制，运行时由后端在 5000 端口同源 serve 前端静态文件。

## 可浏览的内容

| 页面 | 路径 | 说明 |
| --- | --- | --- |
| 首页 | `/` | 文章流、分类、订阅源、关注标签 |
| 主题图谱 | `/topics` | 事件/人物/关键词图谱、时间线、叙事摘要 |
| 标签管理 + 侦探墙 | `/tags` | 语义板块、辅助标签池、板块日报、**3D 侦探墙**（点击板块日报进入） |
| 设置 | `/settings` | 订阅源/AI/路由/队列/偏好/Firecrawl/调度（只读展示，写按钮无响应） |

> 侦探墙是核心亮点：`/tags` → 选一个板块 → 切到「日报」标签 → 点进全屏 3D 卡牌墙 + 红色线索绳。

## 重新生成脱敏数据

若要刷新 demo 数据（例如近期有新的真实数据想纳入演示）：

```bash
cd backend-go
go run ./cmd/dump-sanitizer
# → 覆盖 demo/seed/seed.sql，然后重新 up --build
```

参数与脱敏策略见 [`seed/README.md`](seed/README.md)。

## ⚠️ 重要警告

- **不可用于生产**：这是只读演示，无认证、无持久化、写操作全禁。
- **数据是脱敏快照**：所有内容为导出时刻的样本，不会实时更新；AI 功能（摘要生成、日报重算、标签提取）因凭证已清空而无法实际调用。
- **公网部署须知**：如要部署到公网，请务必：
  - 在反向代理层（nginx/caddy）进一步限制只放行 `GET`；
  - 不暴露 postgres 端口（compose 默认未映射 5432 到宿主）；
  - 用 TLS；
  - 定期重新导出 seed 以保证内容新鲜（已由本机用户级 timer `sync-demo-seed.timer` 每周日 05:00 自动完成：导出→安全断言→归档→`deploy-remote.sh --demo` 全链路，见 `docs/reference/deployment.md`「定时同步」；手动一次性刷新 `cd backend-go && go run ./cmd/dump-sanitizer` + `bash scripts/deploy/deploy-remote.sh --demo` 仍可作为备选路径）。

## 文件清单

```
demo/
├── README.md                      本文档
├── entrypoint.sh                  容器启动：起后端 → 等 health → TRUNCATE 清场 → 导入 seed
├── docker-compose.demo.yml        demo 编排（postgres + syntopica-demo）
└── seed/
    ├── seed.sql                   脱敏数据快照（由 dump-sanitizer 生成）
    └── README.md                  seed 生成与脱敏策略说明

Dockerfile.demo                    多阶段自包含镜像构建
backend-go/cmd/dump-sanitizer/     导出脱敏工具（main.go / sanitize.go / tables.go）
```

## 相关文档

- 设计与决策：[`../openspec/changes/public-read-only-demo/design.md`](../openspec/changes/public-read-only-demo/design.md)
- 任务与验证清单：[`../openspec/changes/public-read-only-demo/tasks.md`](../openspec/changes/public-read-only-demo/tasks.md)
- 生产部署（非 demo）：[`../docs/reference/deployment.md`](../docs/reference/deployment.md)
