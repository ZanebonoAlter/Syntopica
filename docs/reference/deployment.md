# 部署指南

Syntopica 为单用户自托管部署设计。主要部署方式是 Docker Compose，在独立容器中运行 Go 后端和 Nuxt 前端，配合持久化存储。

## 部署方式

| 目标 | 配置文件 | 说明 |
|--------|-------------|-------|
| Docker Compose（基础服务） | `docker-compose.yml` | **默认/推荐方式**。PostgreSQL + pgvector + 应用（Go 后端，内含同源托管的前端静态产物）两容器。 |
| Docker Compose（Firecrawl） | `docker-compose.firecrawl.yml` | **可选**。Firecrawl 全文抓取服务，需配合基础服务使用。 |

没有 PaaS 专用配置（Vercel、Netlify、Fly.io 等）。应用程序设计为通过 Docker Compose 在单机上运行。

> **注意：SQLite 版本已归档到独立的 `sqlite` 分支，主分支仅支持 PostgreSQL 数据库。**

### init.sh 一键部署

项目提供 `init.sh` 脚本，自动完成从环境检查到服务启动的全流程：

```
init.sh 部署流程
├── Phase 1: 基础服务
│   ├── 检查 Docker / Docker Compose 可用性
│   ├── 交互收集端口、密码（全默认值）
│   ├── 从 .env.example 生成 .env（不覆盖已有值）
│   ├── docker compose up -d
│   └── 轮询等待 postgres healthy + backend /health 200
├── Phase 2: AI 服务（可选）
│   ├── llama.cpp — 自动下载预编译二进制
│   ├── Ollama — 检测已有安装
│   ├── 远程 API — OpenAI 兼容云端服务
│   └── GPU 检测 + VRAM 推荐模型
├── Phase 2: Firecrawl（可选）
│   ├── 自部署 — docker-compose.firecrawl.yml
│   ├── 云 API — Firecrawl 云服务
│   └── 跳过
└── Phase 3: 确认与种子数据
    ├── 打印部署摘要
    ├── 下载模型文件（如选择了本地 AI）
    ├── 检测 AI 服务可达性
    └── 写入 Provider / 路由 / Firecrawl 配置
```

使用方式：

```bash
bash init.sh
```

### Docker Compose 拓扑

```
docker-compose.yml                    docker-compose.firecrawl.yml（可选）
┌─────────────────────────────┐      ┌──────────────────────────────────┐
│  postgres (:5432)           │      │  firecrawl (:3002)               │
│  ├─ pgvector 扩展           │      │  ├─ API + Worker                 │
│  └─ data/ 持久化            │      │  ├─ firecrawl-redis              │
│                             │      │  └─ firecrawl-playwright         │
│  syntopica (:5000 容器内)   │      └──────────────────────────────────┘
│  ├─ Go API 服务器           │
│  ├─ 前端静态产物（同源页面）│           │
│  └─ 依赖 postgres healthy   │           │ syntopica-net（外部网络）
└─────────────────────────────┘◄──────────┘
  宿主 ${PORT:-5100} → 浏览器访问 http://<host>:5100/
```

两个 Compose 文件通过 `syntopica-net` 外部网络互联。Firecrawl 容器启动后，后端通过 `http://firecrawl:3002` 访问全文抓取服务。

### 可选组件

| 组件 | 说明 | 何时需要 |
|------|------|---------|
| Firecrawl（自部署） | 全文抓取服务，将 RSS 摘要补全为完整正文 | 需要 Firecrawl 全文抓取且不想使用云服务时 |
| Firecrawl（云 API） | 使用 Firecrawl 官方云服务 | 需要全文抓取但不想自部署时 |
| llama.cpp | 本地 LLM 推理，运行 GGUF 模型 | 无远程 AI API、需要完全本地化部署时 |
| Ollama | 本地 LLM 推理，已有模型管理 | 已安装 Ollama 或偏好其模型管理时 |
| Redis | 持久化任务队列后端 | Topic 分析任务需要持久化队列时 |

## 构建流水线

没有配置 CI/CD 流水线 — 仓库中没有 `.github/workflows/` 文件。构建和部署为手动步骤。

### 容器构建过程

单一 `Dockerfile`（仓库根）多阶段构建出**一个**前后端合一的镜像：

1. `front-build` 阶段（`node:22-alpine`）：corepack 装 pnpm → `pnpm install --frozen-lockfile` → 以 `NUXT_PUBLIC_API_BASE=/api`（可用 `--build-arg` 覆盖）运行 `pnpm generate`，产出静态 SPA 到 `.output/public`。
2. 运行阶段（`alpine:3.22`）：拷入**本地预先编译**的 Go 二进制（`ARG BINARY_PATH`，默认 `./backend-go/syntopica`）、`backend-go/configs/`，以及上阶段的静态产物到 `/app/frontend/`。以非 root 用户 `appuser`（UID 10001）运行，默认 `SERVER_PORT=5000`。

运行时后端通过 `internal/app/static.go` 直接托管 `/app/frontend/`（含 SPA 兜底），因此**浏览器与 API 天然同源**：前端不需要单独容器，也不产生跨域请求。

> 构建前需要先在本地编好二进制（运行阶段是 alpine，必须静态链接）：
> `cd backend-go && CGO_ENABLED=0 go build -o syntopica ./cmd/server`

### Docker Compose 快速部署

```bash
# 启动基础服务（PostgreSQL + 应用）
docker compose up --build -d

# 可选：启动 Firecrawl 全文抓取服务
docker compose -f docker-compose.firecrawl.yml up -d
```

启动两个核心服务：

- **postgres**: PostgreSQL（pgvector:pg18-trixie）端口 5432，带健康检查（`pg_isready`）。数据持久化在 `./data/` 目录。初始化脚本 `docker/postgres/init/01-enable-pgvector.sql` 在首次启动时执行 `CREATE EXTENSION IF NOT EXISTS vector`。
- **syntopica**: 应用容器（容器内 5000，宿主映射默认 `${PORT:-5100}`），内部连接 postgres 服务。**同一个端口同时提供 API、WebSocket、feed 图标与前端静态页面** —— 浏览器访问 `http://<host>:5100/` 即可，无需另起前端服务。

可选的 Firecrawl 服务（通过 `docker-compose.firecrawl.yml`）：

- **firecrawl**: Firecrawl API + Worker，端口 3002，提供全文抓取能力。
- **firecrawl-redis**: Firecrawl 内部 Redis，用于任务队列。
- **firecrawl-playwright**: Playwright 浏览器实例，用于 JavaScript 渲染页面抓取。

两个 Compose 文件共享 `syntopica-net` 外部网络，Firecrawl 容器可通过 `http://firecrawl:3002` 被后端访问。

启动后：
- 应用（前端页面 + API + WebSocket + feed 图标）：`http://localhost:5100`（宿主映射默认 5100；`.env` 显式设置过 `PORT` 的用户不受影响）
- API 基址：`http://localhost:5100/api`

> 前端不需要单独起服务：同一个端口就是页面入口。浏览器与后端不同机时的两种做法见「前端服务的三种形态」。

## 环境设置

完整环境变量列表见 [配置指南](configuration.md)。

### Docker 部署最小配置

`.env.example` 文件包含基础变量：

```bash
PORT=5100              # 宿主映射的应用端口（容器内固定 5000）
POSTGRES_DB=syntopica
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_PORT=5432
```

所有值都有默认值 — 应用程序可以零配置启动。唯一会导致启动失败的场景是数据库 DSN 无效或不可达。

> 旧版 `.env` 里的 `FRONT_PORT` / `BACKEND_PORT` 已不再被任何 compose 文件读取（前端不再有独立容器、后端端口改用 `PORT`），留着无害但会误导。

### 生产环境注意事项

生产部署时需要检查以下设置：

| 变量 | 重要原因 |
|---|---|
| `SERVER_MODE` | Docker Compose 中设置为 `"release"` 以抑制 Gin 调试输出。Docker 外默认为 `"debug"`。 |
| `POSTGRES_PASSWORD` | 使用 PostgreSQL compose 时，应从默认的 `"postgres"` 修改。 |
| `CORS_ORIGINS` | 仅「浏览器与后端**不同 origin**」时需要（见「多机 / 远程访问」）；同源部署下不参与。 |
| `NUXT_PUBLIC_API_BASE` | 同上 —— 静态产物在**构建期**内联该值，跨 origin 部署时必须是浏览器可达的地址。 |

AI 相关设置（LLM 凭证、Firecrawl、Digest 导出）通过 Web UI 配置并存储在数据库中 — 不通过环境变量设置。详见 [配置指南](configuration.md#数据库存储的设置ai-功能)。

### 前端服务的三种形态

前端**不是**独立容器 —— `docker-compose.yml` 只有 `postgres` 与 `syntopica` 两个服务，前端产物由后端同源托管。按场景三选一：

> 静态产物体积说明：前端含 Noto Serif SC 自托管字体分片（`@fontsource`，约 490 个 woff2 小分片，`_nuxt/` 下按 unicode-range 按需加载，单页实际只拉用到的分片，几 KB～百 KB 级）——首屏零外域请求（弱网/离线友好），代价是镜像/静态目录磁盘占用增加；维护约定见 [loading-experience.md](standard/frontend/loading-experience.md)。

| 形态 | 做法 | 适用 | 跨域配置 |
|---|---|---|---|
| **同源（单镜像，默认）** | `docker compose up --build -d` → 访问 `http://<host>:5100/` | 常规自托管 | 不需要 |
| **同源（反代）** | 见下节（Caddy 或 nginx；前端跑 dev 或静态产物皆可） | 后端已在裸跑、不想重建镜像 | 不需要 |
| **dev 直连** | `cd front && pnpm dev` → 访问 `http://<host>:3000` | 本地开发（有 HMR） | 浏览器与后端同机时不需要 |

#### 本地裸跑静态托管（树莓派日常形态，2026-09-17 用户决策）

树莓派上 Nuxt dev 既重又脆弱（客户端连接中断即触发 `ECONNRESET` 整机重启循环，每次重建 nitro 吃满 CPU），redesign-reading-pane 验收期落地了「构建一次 → 后端静态托管」的日常访问形态——上表「同源（单镜像）」的本地裸跑版：

```bash
# 构建静态产物（内含完整 build；apiBase=/api 走同源相对路径）
cd front && NUXT_PUBLIC_API_BASE=/api pnpm generate

# 铺到后端静态目录 + 重启后端（同端口 :5100 出 API + 页面）
rm -rf backend-go/frontend && cp -r front/.output/public backend-go/frontend
bash scripts/dev/start-dev.sh back --restart   # 或手动重启 go run
```

> **一键脚本**：以上三步封装为 `bash scripts/dev/deploy-frontend.sh`（`--no-restart` 只铺盘不重启）；前端改动收尾顺手跑（AGENTS.md §Build & Verify 已固化），勿让用户打开页面还是旧版。

- 访问 `http://<pi-ip>:5100/`（API、WebSocket、feed 图标、静态页面同端口，与 Docker 部署形态一致）；代价是无 HMR，改前端代码须重新 generate + 拷贝。
- `backend-go/frontend/` 是构建产物（不入 git 语义的部署产物），**别手改**；后端启动时该目录不存在则自动纯 API 模式（`internal/app/static.go`）。
- dev server 仅留作需要 HMR 调样式时临时用，用完即停；注意 nginx 同源入口的 `/` 上游仍指向 :3000，dev 不在跑时走 nginx 入口会 502，直连 :5100 即可。

#### 发现 v2 部署注意（improve-discovery-recommendations，2026-09-19）

- **部署后可见变化**：发现页换 v2 链路（问答/刷新 run 化异步执行，前端轮询产出）；推荐数量可能减少甚至为空（精排真正筛选，零选择合法）；推荐卡片可能自动过期退出默认列表（**≠ 拒绝**）；新增「候选源库」页签（入库 ≠ 订阅）与推荐历史/恢复入口。
- **人工操作**：无需手动数据迁移（迁移自动执行，7 新表 + 旧 seed 转 inactive 历史、旧 pending 转 legacy）；若需停后台噪声（可用性检查/向量回补/run 维护三 job），设 `ai_settings.discovery_v2 = {"enabled": false}`——只停后台任务，推荐主链不受影响。
- **旧数据降级**：旧 `preference_vectors.source=seed` 行仅作迁移历史不参与召回；已发布旧推荐不自动删除；已订阅源与文章不受任何影响；候选向量回补 20 条/批每小时自然补齐（3099 候选首日仅部分有向量，发现召回与推荐质量随回补进度提升，非故障）。
- **回滚**：真回滚到 v1 推荐引擎需另做 candidate_preferences → 旧路由资格投影（design 迁移计划 6），v2 开关不包含该投影；仅迁移回滚时配合 SPEC_GATE_BYPASS 类逃生口走数据库回滚脚本。

### 同源反代部署（Caddy / nginx）

后端已在既有方式下运行（裸二进制 / `go run` / 单独容器）时，用一个反代入口把前端与后端拼成同一个 origin —— 两个环境变量都不用配。制品在 [`deploy/same-origin/`](../../deploy/same-origin/README.md)，两种入口 × 两种模式：

| 入口 | 模式 | 前端跑什么 | 要不要构建 | 起入口的命令 |
|---|---|---|---|---|
| Caddy（Docker） | dev 反代 | Pi 上 `pnpm dev`（`:3000`） | 不用 | `NUXT_PUBLIC_API_BASE=/api pnpm dev` + `CADDYFILE=./Caddyfile.dev docker compose -f deploy/same-origin/docker-compose.yml up -d` |
| Caddy（Docker） | 静态产物 | `pnpm generate` 出的 `.output/public` | 要 | `docker compose -f deploy/same-origin/docker-compose.yml up -d` |
| nginx（系统包） | dev 反代 | 同上 | 不用 | `NUXT_PUBLIC_API_BASE=/api pnpm dev --host` + `sudo bash deploy/same-origin/install-nginx.sh dev` |
| nginx（系统包） | 静态产物 | 同上（铺到 `/srv/www`） | 要 | `sudo bash deploy/same-origin/install-nginx.sh static` |

两种入口都把 `/api`、`/ws`、`/icons`、`/health` 反代到 `127.0.0.1:5100`，其余路径要么由静态产物提供（`index.html` 兜 SPA 路由）、要么反代给 dev server（含 Vite HMR 的 WebSocket）——浏览器始终只面对一个 origin。

> 选哪个入口：宿主能拉 `caddy:2-alpine` 就用 Caddy（一条 compose 命令）；拉不到 Docker Hub（实测树莓派上 `registry-1.docker.io` i/o timeout）就用系统 nginx —— `apt install nginx` 后跑一次 `install-nginx.sh`（幂等，`nginx -t` 失败自动回滚）。

> 静态模式下 `NUXT_PUBLIC_API_BASE=/api` **必须在构建期**给（静态 SPA 的 `runtimeConfig.public` 构建期内联，部署后再设环境变量无效）；dev 模式则是**启动时**读，带变量重启 dev server 即可。

完整步骤与排障表见 [`deploy/same-origin/README.md`](../../deploy/same-origin/README.md)。

> 为何不用 Nitro `devProxy` / Vite `server.proxy` 做同源：两条路径都已实测否决（`proxyRequest` 不处理 WebSocket upgrade；Nuxt middlewareMode 下 Vite 不接 upgrade 事件），见 `openspec/changes/archive/2026-09-16-fix-wsl-dev-networking/design.md` D2 与该 change 的 evidence。

### 多机 / 远程访问（浏览器与后端不同机）

**症状**：页面能打开但列表全空，Console 报 `ERR_CONNECTION_REFUSED`（请求打到 `localhost:5100`）；或把地址改成后端 IP 后变成 CORS 被拦。

**根因**：`localhost` 是**浏览器所在那台机器**的环回地址，不是后端主机。前端 `apiBase` 默认 `http://localhost:5100/api`（`front/nuxt.config.ts`）、后端 CORS 白名单默认只认 `http://localhost:3000`（`backend-go/internal/platform/config/config.go`） —— 两个默认值都只在「浏览器与后端同机」时成立。

**用环境变量补救**（不改代码；两处都要配，少一个就换一种报错）：

| 位置 | 变量 | 值 |
|---|---|---|
| 后端 | `CORS_ORIGINS` | 逐个列出浏览器地址栏里的 origin，如 `http://10.11.12.55:3000,http://localhost:3000` |
| 前端 | `NUXT_PUBLIC_API_BASE` | 浏览器可达的绝对地址，如 `http://10.11.12.55:5100/api` |

- `CORS_ORIGINS` 是**精确匹配**（`middleware/cors.go` 逐条比对，无通配回退），地址栏换个端口或主机名就要跟着加。
- `NUXT_PUBLIC_API_BASE` 在 **dev 模式启动时**读取，改完要重启 dev server；**静态产物则必须在构建期给**（见上节）。
- 一个变量修好三样东西：API、WebSocket（`ws://<host>:5100/ws`）、feed 图标（`http://<host>:5100/icons/...`） —— 它们共用同一个 origin 解析（`front/app/utils/api.ts`）。
- 验算：`curl -D - -o /dev/null -H "Origin: http://<前端地址>" http://<后端地址>:5100/api/categories | grep -i access-control` → 应出现 `Access-Control-Allow-Origin`。

**推荐做法**：把主机地址写死进环境变量很脆（换网段、加设备、手机访问都要重配）。浏览器与后端不同机的常驻部署优先用**同源**（单镜像或反代），两个变量都不用配。

### PostgreSQL autovacuum 调优（docker-compose.pg.yml）

`docker-compose.pg.yml` 为 postgres 服务追加了 autovacuum 调优参数（optimize-pg-storage，2026-08-28）：

```yaml
command:
  - postgres
  - -c
  - autovacuum_vacuum_scale_factor=0.05        # 默认 0.2，大表触发严重滞后
  - -c
  - autovacuum_vacuum_insert_scale_factor=0.05 # 纯 INSERT 表（otel_spans）的触发器
  - -c
  - autovacuum_vacuum_insert_threshold=500
  - -c
  - autovacuum_vacuum_cost_limit=2000          # 默认 200 追不上写入速度
  - -c
  - wal_compression=on                         # 高写入库 WAL 全页镜像压缩
```

**为什么**：默认参数下高写入量表（otel_spans 日增 ~82 万行、embedding 缓存反复重写）的死元组回收严重滞后，实测 TOAST 膨胀 13 倍、库体积膨胀到 12GB；收紧触发阈值 + 提高 cost limit 后 vacuum 跟得上写入。

**生效方式**：参数变更需 recreate 容器（`docker compose -f docker-compose.pg.yml up -d`，数据在 `./data` bind mount 不丢）。验证：`docker exec syntopica-postgres psql -U postgres -d syntopica -c "SHOW autovacuum_vacuum_scale_factor;"` → `0.05`。

### 破坏性迁移开关

部分历史版本迁移会 `TRUNCATE` 业务表以清理与旧 schema 不兼容的数据（如 `20260706_0001`、`20260712_0001`、`20260718_0001`）。这些迁移由 `MIGRATIONS_ALLOW_DESTRUCTIVE` 环境变量控制：

| 环境 | 是否设置 `MIGRATIONS_ALLOW_DESTRUCTIVE=1` | 行为 |
|---|---|---|
| **生产** | ❌ **绝不设置** | 破坏性迁移自动跳过（仅打 WARN 日志），业务数据保留，迁移版本仍标记为已应用。这是安全默认。 |
| **dev / 本地开发** | ✅ 建议设置 | 历史清理迁移正常执行，dev 库等价于「全量生产迁移 + 历史数据清理」。 |

> **为什么默认拒绝**：破坏性迁移一旦执行不可逆（TRUNCATE 清空全表）。生产环境若误设此变量，启动时即清空 `topic_lifeline_context`/`topic_enrichment_result` 等表数据。dev/测试库常重建，清理历史数据无成本，故建议 dev 设、生产不设。

> **测试环境**：testcontainer 集成测试路径（`testutil.runTestMigrations`）自动 `t.Setenv("MIGRATIONS_ALLOW_DESTRUCTIVE","1")`，无需手动设置，维持「测试库跑全量生产迁移」不变量。

完整环境变量说明见 [配置指南](configuration.md#环境变量)。

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

**定时备份（每日自动，add-pg-key-tables-backup）**：

- 脚本：`scripts/db/backup-key-tables.sh`（也可手动跑）；输出 `backups/pg-key-<timestamp>.dump`（`pg_dump -Fc` 自定义格式，单份约 580MB）
- 范围：**排除法**——排除日志/追踪/缓存/队列等可再生表（`ai_call_logs`、`otel_spans`、`ai_embedding_cache`、`firecrawl_jobs`、`tag_jobs`、`embedding_queues`、`merge_reembedding_queues`、`notifications`、`discovery_runs`、`discovery_run_items`、`cross_board_relation_runs`、`topic_watch_hits`、`scheduler_tasks`、`reading_behaviors`、`schema_migrations`），其余表全部自动纳入；**新加日志/缓存类表须手动补进脚本内 `EXCLUDED` 清单**。已知边界：排除表的 serial 序列（如 `ai_call_logs_id_seq`）会作为空壳进档（pg_dump 18 无序列级排除），恢复无害——PG 对同名序列直接复用，AutoMigrate 建表不受影响（已实测）
- 定时：crontab 每日 04:00（Asia/Shanghai），日志追加 `logs/db-backup.log`；同一时刻至多一个备份进程（flock）
- 保留：仅成功备份后轮转，按档名时间戳保留最近 7 份；失败不删旧档、不留残档

重装/检查 crontab 条目（幂等）：

```bash
crontab -l | grep backup-key-tables || \
  (crontab -l 2>/dev/null; echo '0 4 * * * cd '$PWD' && bash scripts/db/backup-key-tables.sh >> logs/db-backup.log 2>&1') | crontab -
```

**手工备份（即时全量 SQL 文本，供快速肉眼检视）**：

```bash
docker exec syntopica-postgres pg_dump -U postgres syntopica > backup.sql
```

**恢复（`-Fc` 定时备份档）**：恢复用同一容器镜像执行（保证 pg_restore 与 dump 版本一致）：

```bash
# 1. 校验档完整性（列出目录）
docker exec -i syntopica-postgres pg_restore -l < backups/pg-key-<timestamp>.dump > /dev/null && echo OK

# 2a. 整库恢复到空库（--clean 先删后建，覆盖现有库；--if-exists 防止对象不存在报错）
docker exec -i syntopica-postgres pg_restore -U postgres -d syntopica --clean --if-exists < backups/pg-key-<timestamp>.dump

# 2b. 只恢复单表（示例：articles 及其数据）
docker exec -i syntopica-postgres pg_restore -U postgres -d syntopica --clean --if-exists -t articles < backups/pg-key-<timestamp>.dump
```

恢复后 `schema_migrations` 无需手工补：后端启动时 GORM AutoMigrate 会自动重跑迁移。

### feed 图标目录（运行时资产，`data/icons/`）

`backend-go/data/icons/`（图标文件在 `feeds/` 子目录，`storage.icon_dir` 相对进程 cwd 解析，默认即 `backend-go/data/icons/`）是**运行时资产，两个备份渠道都不携带它**：

- `.gitignore` 的 `data/` 规则把它排除在 git 之外（不会进仓库、不会进镜像）；
- DB dump 只含 `feeds.icon = /icons/feeds/<id>.<ext>` 这个**路径字符串**，不含文件本身。

因此换机、恢复 dump、清理 `data/` 目录之后，会出现「DB 说图标已本地化、磁盘一个文件都没有」→ `/icons/feeds/*` 全线 404（2026-09 生产实测 203 请求全 404）。处置方式（三选一，推荐第 1 或第 2）：

1. **同步文件（立即恢复，无外网抓取）**：`rsync -a <旧机>/backend-go/data/icons/ <新机>/backend-go/data/icons/`
2. **等自愈（无需操作）**：`heal-missing-feed-icons`（2026-09-16）起 auto + `/icons/` 路径会先校验磁盘文件，缺失即重跑抓取管线重新落盘；等 feed 下一次刷新即可（刷新周期即 `refresh_interval`，默认 60 分钟）
3. **手动触发单个 feed 重抓**：

   ```bash
   curl -X POST http://<host>:5100/api/feeds/<id>/refresh
   curl -s -o /dev/null -w '%{http_code}\n' http://<host>:5100/icons/feeds/<id>.<ext>   # 期望 200
   ```

确实抓不到 favicon 的源会收敛为 `mdi:rss` + `icon_source=fallback`（前端显示 RSS 占位图标）；文件缺失且重抓失败时**不会**继续指向悬空路径。迁移检查清单里请把 `data/icons/` 与数据库备份并列——它是「和 DB 有隐式引用关系、但不在 DB 里」的那一类资产（定时备份有意不携带它，缺失时自愈机制重抓）。

## 公开只读 Demo

公开演示环境使用独立的 demo compose 文件和脱敏 seed 数据启动，不复用生产数据卷：

```bash
docker compose -f demo/docker-compose.demo.yml up -d --build
```

该模式会构建前后端自包含镜像，导入 `demo/seed/seed.sql`，并通过 `DEMO_READ_ONLY=1` 禁止写入、后台任务和 WebSocket。详细启动、刷新 seed 与安全注意事项见 [demo/README.md](../../demo/README.md)。

## 升级注意事项（board-level-deep-analysis，2026-08-31）

本批变更（版块简报/问题调查重做）升级后注意：

- **自动迁移，重启后端即生效**：`20260828_0001`（result_kind/parent/question_key + 复合 FK/触发器）、`20260828_0002`（旧参考角色字节复制为 disabled legacy 方法卡）、`20260831_0001`（未编辑过的 seed 画像翻 disabled）均在启动时自动执行，仅向上无 Down；无需手动跑脚本。
- **无需清理/重新生成旧结果**：存量版块分析自动回填 `legacy_board_analysis`，前端只读渲染并标「旧版分析」，QA/详情/列表照常；调查的父约束只在新增行上生效。旧 `topic_analysis` 行不受影响。既有 `board_investigation` 快照同样不回写；若历史调查出现“零证据但 supported/refuted/weakened”，升级后从父简报手动重跑即可生成受新一致性门保护的新快照，旧行继续作为历史记录保留。
- **拒绝即报警**：若存在 scope/owner 混用的脏行或非法调查父行，迁移会拒绝执行（启动报错）——这是防数据损坏被掩盖，修数据后再重启。
- **旧参考角色**：用户编辑过的 `reference_roles` 行不会被强制禁用（原样保留），但**任何行都不再注入 prompt**；写 API 一律 410，改用设置页「分析方法」（旧内容已按原文字节复制为停用的 legacy 方法卡，人工整理后启用）。
- **慢供应商需调 provider 超时**：`data_enrichment_analysis` 若指向慢速模型（实测调查链单次 LLM 可达 6 分半），把对应 `ai_providers.timeout_seconds` 调到 600（默认 120 会拦腰截断）；设置页 AI 供应商面板改，即时生效无需重启。总 job 上限仍 30 分钟。详见 [configuration.md](configuration.md) §数据增强。

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

后端内置了 OpenTelemetry 分布式追踪，使用自定义的 GORM Span Exporter。追踪数据写入 PostgreSQL 的 `otel_spans` 表。

> **命名说明**：代码中该导出器名为 `SQLiteSpanExporter`，这是从早期 SQLite 版本遗留下来的历史命名，实际功能是将 span 数据写入 PostgreSQL，与 SQLite 无关。保留此命名仅避免不必要的破坏性重命名。

主要追踪配置：
- **库**：`go.opentelemetry.io/otel`，全局应用 `otelgin` HTTP 中间件到所有路由
- **导出器**：自定义 `SQLiteSpanExporter`，通过 GORM 将 span 写入 `otel_spans` 表
- **HTTP 中间件**：`otelgin.Middleware(tracing.ServiceName)` 为所有 HTTP handler 捕获请求级 span
- **追踪的调度器操作**：auto_refresh、firecrawl、content_completion、auto_summary、preference_update、digest
- **追踪的领域操作**：AI summary 队列批处理、AI router chat
- **数据保留**：7 天（通过 `tracing.DefaultConfig()` 配置）
- **缓冲区**：100 个 span，每 5 秒刷新
- **查询 API**：后端通过 `internal/platform/tracing/handler.go` 暴露追踪查询端点 — 最近追踪、按 trace ID 查询、时间线视图、统计、按操作/状态/时长搜索、OTLP JSON 导出

没有配置外部监控服务（Sentry、Datadog、New Relic）。内置追踪为 feed 刷新周期、AI 操作和 HTTP 请求延迟提供基础可观测性。

可通过应用内置的追踪 UI 或直接查询 `otel_spans` 表查看追踪数据。
