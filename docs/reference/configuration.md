# 配置指南

Syntopica 使用分层配置系统：后端 YAML 配置文件、覆盖文件值的环境变量，以及 Nuxt 运行时配置。AI 相关设置（LLM、Firecrawl、Digest）存储在数据库中，通过 Web UI 配置。

## 环境变量

### 后端（Go）

以下环境变量会覆盖 `backend-go/configs/config.yaml` 中的值。未设置时使用配置文件默认值。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `SERVER_PORT` | 否 | `"5000"` | 后端 HTTP 监听端口 |
| `SERVER_MODE` | 否 | `"debug"` | Gin 模式：`"debug"`、`"release"` 或 `"test"` |
| `DATABASE_DRIVER` | 否 | `"postgres"` | 数据库驱动，主分支仅支持 `"postgres"` |
| `DATABASE_DSN` | 否 | `"host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai"` | PostgreSQL 连接字符串 |
| `CORS_ORIGINS` | 否 | `"http://localhost:3000,http://localhost:3000"` | 逗号分隔的允许 CORS 来源列表 |
| `MIGRATIONS_ALLOW_DESTRUCTIVE` | 否 | *(未设置)* | 仅设为 `"1"` 时启用破坏性数据库迁移（含 `TRUNCATE`/`DROP` 的历史数据清理迁移）。**生产环境绝不设置**；dev/本地开发设 `"1"` 以执行历史数据清理。见 [部署指南](deployment.md#破坏性迁移开关)。 |
| `TRACE_SAMPLE_RATIO` | 否 | `0.05` | OTel root span 采样比例（`ParentBased(TraceIDRatioBased)`）。`1.0`=全采；`<1.0` 按比例降采样；被采 root 的所有子 span（DB/出站 HTTP/业务）完整保留。非法值（不可解析或超出 0.0–1.0）回退默认 `0.05` 并打 warn。全采样下 otel_spans 日增 ~82 万行/600MB，故默认低采样；排障时临时设 `1.0` 重启即可恢复全采。 |
| `TRACE_INSTRUMENT_GORM` | 否 | *(未设置，等效启用)* | 设为 `"0"` 关闭 GORM DB 操作自动埋点（自写 `GORMTracePlugin`）。非 `"0"` 均视为启用。 |
| `TRACE_INSTRUMENT_HTTP` | 否 | *(未设置，等效启用)* | 设为 `"0"` 关闭出站 HTTP 自动埋点（`httpclient` 工厂的 otelhttp 包装）。非 `"0"` 均视为启用。 |
| `STORAGE_ICON_DIR` | 否 | `"data/icons"` | feed 图标本地化存储根目录（实际文件在 `feeds/` 子目录），由后端 `/icons` 静态路由对外服务 |
| `BOCHA_API_KEY` | 否 | *(空)* | **部署兜底**——博查 key 首选在设置界面「博查搜索」配（存 `ai_settings` 表，动态生效）。此 env 仅用于无界面/CI/容器部署，与 `configs/config.yaml` 的 `bocha.api_key` 同为兜底（优先级：界面 DB > env > config.yaml）。全空→`web_search` 降级 Noop（返回错误 JSON，agent 自降级、不阻断） |
| `BOCHA_ENDPOINT` | 否 | `"https://api.bochaai.com/v1/web-search"` | 博查通搜 endpoint（原始网页结果模式）的**兜底**值；界面可覆盖。仅在需要切换 endpoint（如代理/镜像）时设 |

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
| `POSTGRES_DB` | 否 | `"syntopica"` | PostgreSQL 数据库名 |
| `POSTGRES_USER` | 否 | `"postgres"` | PostgreSQL 用户名 |
| `POSTGRES_PASSWORD` | 否 | `"postgres"` | PostgreSQL 密码 |
| `POSTGRES_PORT` | 否 | `"5432"` | PostgreSQL 容器映射到宿主机的端口 |
| `TZ` | 否 | `"Asia/Shanghai"` | PostgreSQL 容器时区 |
| `GOPROXY` | 否 | *(空)* | 后端构建时的 Go 模块代理 |
| `GOSUMDB` | 否 | *(空)* | 后端构建时的 Go 校验数据库 |
| `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` | 否 | *(空)* | 代理设置，传递到构建上下文 |

### Docker Compose（Firecrawl）

以下变量由 `docker-compose.firecrawl.yml` 使用，仅在启动 Firecrawl 服务时有效。

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `FIRECRAWL_PORT` | 否 | `"3002"` | Firecrawl API 映射到宿主机的端口 |
| `FIRECRAWL_REDIS_PORT` | 否 | `"6380"` | Firecrawl Redis 映射到宿主机的端口（避免与本地 Redis 冲突） |

> **注意**：Firecrawl 的 API URL 和 API Key 通过 Web UI 配置（存储在 `ai_settings` 表），而非环境变量。自部署时 API URL 默认为 `http://firecrawl:3002`（容器间通信）或 `http://localhost:3002`（宿主机访问）。

> **Firecrawl 现为可选兜底**：文章正文抓取默认走进程内 readability（纯 Go，零外部依赖）。仅当 readability 提取不到合格正文（SPA 站点）时才降级调用 Firecrawl。Firecrawl 服务可随时关闭而不影响 SSR 站点的正文抓取。

## 配置文件格式

后端从 `backend-go/configs/config.yaml` 读取 YAML 配置文件，通过 Viper 在启动时加载。默认配置即为 PostgreSQL 连接，即使没有配置文件也能正常工作。

> **注意：主分支仅支持 PostgreSQL 数据库驱动。SQLite 支持仅在 `sqlite` 分支可用。**

```yaml
server:
  port: "5000"
  mode: "debug"           # debug | release | test

database:
  driver: "postgres"
  dsn: "host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai"
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
| Database DSN | `"host=127.0.0.1 user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai"` | `viper.SetDefault` in `config.go` |
| Postgres max idle conns | `5` | `viper.SetDefault` in `config.go` |
| Postgres max open conns | `25` | `viper.SetDefault` in `config.go` |
| Postgres conn max lifetime | `60` min | `viper.SetDefault` in `config.go` |
| Postgres conn max idle time | `10` min | `viper.SetDefault` in `config.go` |
| CORS origins | `localhost:3000`, `localhost:3000` | `viper.SetDefault` in `config.go` |
| CORS methods | `GET, POST, PUT, DELETE, OPTIONS` | `viper.SetDefault` in `config.go` |
| CORS headers | `Content-Type, Authorization` | `viper.SetDefault` in `config.go` |
| Tracing enabled | `true` | `tracing.DefaultConfig()` |
| Tracing retention | `7` days | `tracing.DefaultConfig()` |
| Tracing sample ratio | `0.05` | `viper.SetDefault("tracing.sample_ratio")` in `config.go`（optimize-pg-storage：全采样日产 82 万 span/600MB，降为 0.05） |
| Tracing instrument GORM | `true` | `tracing.DefaultConfig()` / viper `tracing.instrument_gorm` |
| Tracing instrument HTTP | `true` | `tracing.DefaultConfig()` / viper `tracing.instrument_http` |

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
docker run -d --name rss-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=syntopica pgvector/pgvector:pg18-trixie
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
| `rsshub_config` | RSSHub 实例配置（订阅源发现用，见下「订阅源发现」节；`rsshub_base_url` 缺省回落 `http://rsshub.app`） |
| `http_proxy_config` | 全局出站代理配置（feed 抓取 / Firecrawl / LLM 等所有外部请求；见下「出站代理」节；`http_proxy_url` 空=直连） |
| `daily_report_time` | 日报生成时刻（HH:MM 格式，默认 `21:00`） |
| `auto_start_models` | 本地模型自动拉起总开关（默认 `false`）：后端启动时，对「探测不通且配了 `start_command`」的 provider 自动执行启动命令拉起本地模型进程。见下「本地模型自动拉起」节 |
| `persistent_topic_match_threshold` | 新 section 锚定已有话题的余弦距离阈值（默认 `0.30`） |
| `persistent_topic_upgrade_threshold` | candidate 允许人工确认所需、同时为管理 UI 可见门槛的连续命中天数（默认 `3`；不会自动转 active） |
| `persistent_topic_candidate_decay_window` | candidate 被注入聚类 prompt 的观察窗口天数（默认 `7`）；仅用于 prompt 卫生过滤，不触发状态变更 |
| `persistent_topic_candidate_prompt_limit` | 每次聚类注入与归属锚定集合的 candidate 数量上限（默认 `20`） |
| `persistent_topic_cluster_threshold` | 历史回刷 complete-link 聚类阈值（默认 `0.28`） |
| `persistent_topic_lane_l1_threshold` | Lane L1 直挂阈值：当天 tag 到 topic **质心**余弦距离 < 此值直接归属该 topic（不调 LLM）。默认 `0.18`（实测把强挂率从首义向量 14% 提到 62%） |
| `persistent_topic_lane_l2_threshold` | Lane L2 弱区上限：距离在 `[l1, l2]` 的 tag 交 LLM 在 top-K 候选上做「留/换/新」三选一。默认 `0.30` |
| `persistent_topic_vacuum_ratio` | 吸尘器判定：topic 近窗口 `strong/(strong+mid)` < 此值则 `is_vacuum=true`，挂到它的 tag 从 L1 降级 L2 交 LLM 裁决。默认 `0.20` |
| `persistent_topic_centroid_window` | 质心计算取最近 N 条 section embedding 做均权平均；section<2 退化首义向量。默认 `30` |
| `persistent_topic_vacuum_window` | 吸尘器吸引统计窗口（天）：按近 N 天归属该 topic 的 section 统计 strong/mid。默认 `7` |
| `persistent_topic_l2_candidate_k` | L2 LLM prompt 注入的 top-K 候选 topic 数（按质心距离排序）。默认 `5` |
| `daily_report_section_merge_enabled` | 同日 section 两阶段合并（确定性 <0.20 + 灰区 LLM 仲裁）总开关。默认 `false`（关）——关闭时 section 按lane 管线原始分组落库；开启时合并仍受锚定边界约束（不同 `persistent_topic_id` / 新叙事↔锚定跨界禁止合并）。见 `flow/daily-report.md` 业务约束 12 |
| `persistent_topic_candidate_l1_gate_enabled` | **观察期门禁**：candidate topic 是否享有 L1 直挂资格。默认 `true`（开）——开启时最近话题为 candidate 的近距离 tag（距离 < `lane_l1_threshold`）降级进 L2 band 交 LLM 裁决，阻断「一次性新闻标题 candidate 靠同域 tag 无限续命」；关闭时回退 active/candidate 均可直挂的旧行为（在线回滚用，无需发版）。见 `flow/daily-report.md` 业务约束 14 |
这些设置通过 `aisettings.LoadSummaryConfig()`、`aisettings.LoadFirecrawlConfig()` 等函数加载，在前端设置页面中配置。

文章手动总结会在每次请求时重新读取 AI Provider 配置：优先使用 `summary` capability 的启用路由；未配置该路由时，回退到任一启用且具有 Base URL 和 Model 的 Provider。因此服务启动后新增 Provider 无需重启。API Key 对本地 Ollama、llama.cpp 或无需鉴权的 OpenAI-compatible 服务是可选项。

### Provider 思考开关（`enable_thinking`）

`ai_providers.enable_thinking` 字段控制是否让被调用的模型进行推理思考——后端在请求体中透传 `chat_template_kwargs.enable_thinking=true`（适用于 Qwen3.x / Qwythos 等带思考模板的 llama.cpp 模型）。**注意语义**：该字段只控制「模型是否思考」，服务器会把思考内容分离到 `reasoning_content` 字段，`content` 始终是干净答案。

**典型场景：同一台本地模型，打标签不思考、生成日报思考**。靠配两条 provider 指向同一服务实现差异化（无需改代码）：

1. 后台新建 provider `qwythos-nothink`：`base_url=http://127.0.0.1:8080`、`model=<模型名>`、`enable_thinking=false`。挂到 `topic_tagging` route（打标签走它，省 token、更快）。
2. 后台新建 provider `qwythos-think`：同样的 `base_url` 和 `model`、`enable_thinking=true`。挂到 `digest_polish` route（日报走它，思考后质量更高）。

> 历史语义提示：该字段曾表示「事后剥离 `<think>` 标签」，migration `20260626_0001` 已将所有 provider 的该字段重置为 `false` 以兜底语义反转。升级后请按需手动开启日报 provider 的思考。

### 本地模型自动拉起（`auto_start_models`）

存 `ai_settings`（默认 `false`），经 `GET /api/ai/health` 读取、`PUT /api/ai/health/auto-start-models` 更新（前端设置页「AI 健康」面板开关）。开启时，后端启动健康检测（`internal/platform/aihealth`）对「探测不通 **且** 配了 `start_command`」的 provider 自动执行启动命令拉起本地模型进程（fire-and-forget 脱离执行 + 轮询复探 ≤45s），拉起后健康翻绿、分析类任务恢复运行。

> **安全提示**：开启后会执行 provider 的 `start_command`（任意本地命令），对单用户本地工具可接受，但命令需可信；不建议在多人/不可信环境开启。

### 数据增强（Data Enrichment）

数据增强功能（循环A新闻汇总 + 循环B三角色编排）依赖以下部署配置：

#### 1. `ai_routes` 表 seed 两条 Capability

| capability | 路由建议名 | 说明 |
|------------|-----------|------|
| `data_enrichment_news` | `data-enrichment-news` | 循环A新闻汇总（`summarize_context`），量大可配便宜模型 |
| `data_enrichment_analysis` | `data-enrichment-analysis` | 循环B分析认知（`interpret` / `tool_use` / `analyze` / `review_judge`）与版块链（`board_brief` / `board_method_select` / `board_hypothesize` / `board_synthesize`；调查共享研究循环复用 `tool_use`） |

两条路由均需在 `ai_routes` 表 seed 为启用状态，并绑定到 `ai_route_providers` 中的至少一个 provider。

#### 2. Qwen3 / Qwythos Provider 需配 `enable_thinking=false`

根据设计决策（design.md §11 决策①），`data_enrichment_news` 和 `data_enrichment_analysis` 路由中使用 Qwen3 / Qwythos 模板的本地 provider 必须设置 `enable_thinking=false`。这类模型在 thinking 模式下可能烧光 token 并导致 `content` 为空，当前 agent loop 的 system prompt + max_tokens 预算不兼容这种隐式思考模板。其他兼容供应商按各自协议与模型配置决定，不要为了处理响应慢而一刀切修改 thinking 开关。

> **注意**：此配置是 provider 级别的（`ai_providers` 表 `enable_thinking` 字段），不是 per-request 参数。domain 代码不做特殊处理，照常调用 `airouter.Router.Chat`。airouter 请求层始终透传 `chat_template_kwargs.enable_thinking = provider.EnableThinking`（参见 `openai_compatible.go` 的 `buildPayload`）。

#### 3. 板块编辑需开 `enrichment_enabled=true`

循环B增强仅对 `enrichment_enabled=true` 的板块（SemanticLabel）允许触发。用户需先在板块详情页的「分析」tab 中：
1. 开启 `enrichment_enabled` 开关（默认 false）
2. （可选）调整 `window_days`（默认 14）和 `context_layers`（默认 `["week","month","year","all"]`）
3. （可选）绑定数据源（`board_data_sources`）——**金融 source_type（`etf_quote`/`exchange_rate`/`gdelt_event`）已移除**，`web_search`/`fetch_page`/内部导航为 always-on 工具（无需绑定即可用）；该绑定机制保留为未来接入结构化外部源的扩展点

管理员无需额外配置；以上操作均可通过 Web UI 完成。

#### 4. 博查 web_search（可选，影响深度层证据质量）

循环B 的 `web_search` 工具默认降级（未配 key 时返回错误 JSON，agent 自判降级、不阻断）。配博查 key 后启用真实联网检索，分析员的深度层 `evidence_chain` 才能产出可核查的 web/page 证据。

- **只用通搜原始结果模式**（`summary:false`），**禁用 AI 总结模式**——AI summary 有幻觉风险，不可作可核查证据。深度层引用的外部正文经 `fetch_page` 抓自真实文档，不来自 AI 转述。
- **配 key（首选·设置界面）**：设置页「博查搜索」section（`GET/POST /api/settings/bocha`，存 `ai_settings` 表 `bocha_config`，照 Firecrawl）。界面改**即时生效、无需重启**（`BochaWebSearcher` 每次 `Search` 动态读 DB）。GET 返回脱敏 key（不回显完整值）；保存时 api_key 留空=不改、输入新值才覆盖。
- **配 key（兜底·无界面/部署）**：`configs/config.yaml` 的 `bocha.api_key` 或环境变量 `BOCHA_API_KEY`。优先级 **界面 DB > env > config.yaml > 空**。
- 无 key 时：`web_search`/`fetch_page` 降级，深度层仍产出但证据链弱（仅靠分层新闻 context）。

#### 5. Provider 超时（`ai_providers.timeout_seconds`，慢模型必调）

`data_enrichment_analysis` 路由指向的 provider 每次调用可设 HTTP 超时（`ai_providers.timeout_seconds`，DB 默认 120 秒；设置页 AI 供应商面板 / `PUT /api/ai/providers/:id` 可改）。**没有专用环境变量**，这是纯 DB 配置。

- **慢供应商建议 600 秒（10 分钟）**：版块调查链（`board_hypothesize` → 共享研究循环多轮 `tool_use` → `board_synthesize`）单次 LLM 调用在慢速模型上实测可达 6 分半（glm-5.3-flash 两次 HTTP 200 均在 358-389 秒），默认 120 秒会把这类调用拦腰截断。按 2026-08-31 用户决定：慢速 `data_enrichment_analysis` 供应商建议/当前操作口径调到 600 秒。
- **总 job 上限不变**：单次分析（简报/调查/单泳道）后台 job 硬上限 30 分钟（`analysisJobTimeout`），provider 超时只影响单次 LLM 调用，不动总预算。
- **修改即时生效、无需重启**：airouter 每次调用经 `LoadRouteWithProviders` 现查路由与 provider（`openai_compatible.go` 按当次 `provider.TimeoutSeconds` 建 client 超时）。
- 另：HTTP 成功但 assistant content 为空的响应会被 airouter 统一规范化为可重试失败（`error_code=empty_response`）并走 ordered fallback，不会把空文本当成功结果消费（board-level-deep-analysis 2026-08-31 修复，见 flow/ai-summary 与 standard/backend/ai-logging）。

#### 6. 分析方法卡（UI 管理，无配置文件项）

方法卡（`analysis_methods` 表）经设置页「分析方法」section 或 `/api/analysis-methods` CRUD 管理（创建默认停用，启停即时生效）；仅调查链按问题选中 0-2 张注入，不进简报/事实阶段。旧参考角色（`reference_roles`）已退役只读。详见 [api/dataenrichment.md](api/dataenrichment.md)。

#### 6. 跨版块关系发现（add-evidence-backed-cross-board-relations，默认关闭）

全局预算/门槛存 `configs/config.yaml` 的 `cross_board_rel` 段（`CrossBoardRelationConfig`，代码默认值兜底见 `config.EffectiveCrossBoardRelationConfig`）：

| 键 | 默认 | 说明 |
| ---- | ---- | ---- |
| `auto_max_sources_per_brief` | 3 | 自动发现：每份新简报最多取几个 observation 发起发现。**-1 = 全局显式禁用**（effective 0）；未配置/0 = 用默认 3。**自动发现整体默认关闭**——板级开关不开启就永不自动跑 |
| `max_searches_per_run` | 4 | 单次发现 run 的 web_search 调用上限（scout 2-4 query + verifier 反证共享口径按阶段分别计数） |
| `max_fetches_per_run` | 2 | 单次 run 的 fetch_page 上限 |
| `max_loops_per_run` | 6 | scout loop 轮数上限 |
| `run_timeout_seconds` | 300 | 单次 run 整体超时 |
| `resolve_threshold` | 0.62 | 目标解析 top-1 最低分（embedding cosine 或词法归一分） |
| `resolve_margin` | 0.08 | top1-top2 最小差距——不足则 ambiguous（unresolved 不硬选） |
| `dismiss_cooldown_days` | 14 | 被驳回关系的同 suggestion_hash 冷却期 |
| `confirmed_ttl_hours` | 720 | confirmed 关系有效期（30 天，到期转 expired 不再注入简报） |
| `brief_max_relations` | 3 | 单份简报最多注入几条 confirmed 关系 |
| `brief_max_relation_runes` | 1200 | 简报注入块字符预算（超限截断并在快照记 truncated） |

**板级开关**（DB，非 yaml）：`semantic_labels.relation_auto_discovery_enabled`（默认 false，板块编辑弹窗可切）。层级关系：板级开关关 → 永不自动；开关开但全局 `auto_max_sources_per_brief` ≤ 0 → 仍不自动。手动「发现关联」不受这两个开关限制（按 source 互斥 + hash 幂等防重）。

博查 key 复用上文「数据增强 §4」同一配置（界面 > env > config.yaml）——跨版块发现的 Scout/Verify 检索走同一 web_search 后端；无 key 时诚实降级（gap 记录 `web_search_error`，不阻断）。

### 出站代理（`http_proxy_config`）

通过设置工作台「出站代理」section（`GET/POST /api/settings/proxy`）配置，存 `ai_settings.http_proxy_config`：

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `http_proxy_url` | *(空)* | 全局出站代理地址；feed 抓取、Firecrawl、LLM 等**外部**请求经此转发。支持 `http`/`https`/`socks5`。空=直连 |

- **回环地址自动直连**：即使配置了代理，目标为回环地址（`localhost` / `127.0.0.0/8` / `::1` / 空 host）的请求一律绕过代理直连（NO_PROXY 惯例）——本地托管模型（llama-server）的探测/推理不被代理 502 拦截（2026-08-19 修复，见 ai-health-reprobe）。

- 保存即时生效（`httpclient.SetProxy` 运行时替换全局 transport），重启后由 `cmd/server/main.go` 从该配置注入，无需重设。
- 与 `HTTP_PROXY`/`HTTPS_PROXY` 环境变量的关系：本配置优先；当 `http_proxy_url` 为空时，`httpclient` 回落 `http.DefaultTransport`，仍遵循标准库 `ProxyFromEnvironment`（即环境变量作兜底）。
- 实现入口：`internal/platform/httpclient/httpclient.go`（`SetProxy` + 包级 `proxyTransport`，Proxy 函数含回环直连 `isLoopbackHost`）；复用 `aisettings` 通用配置存储，与 RSSHub/Firecrawl 配置同机制。

### 订阅源发现（Feed Discovery）

订阅源发现（偏好向量画像 × RSSHub 路由目录 → 向量粗筛 + LLM 精排 → 推荐卡片）依赖以下配置。链路与业务约束见 [flow/discovery.md](flow/discovery.md)。

#### 1. RSSHub 实例地址（`rsshub_config`）

通过设置工作台「RSSHub」section（`GET/POST /api/settings/rsshub`）配置，存 `ai_settings.rsshub_config`：

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `rsshub_base_url` | `http://rsshub.app`（`DefaultRSSHubBaseURL`） | 自建 RSSHub 实例地址；缺省/空回落默认值。目录同步与推荐订阅落地共用，改一处全链路生效 |

**官方文档基址（`rsshub_doc_base`）**：同经 `GET/POST /api/settings/rsshub` 配置（响应附 `rsshub_doc_base` + `rsshub_doc_base_default`），存 `ai_settings.rsshub_doc_base`。默认 `https://docs.rsshub.app`，用于推荐卡片「官方文档」链接生成（`{doc_base}/routes/{namespace}#{slug}`）。官方文档站国内可能访问受限，可配换镜像。链路见 [flow/discovery.md](flow/discovery.md) §参数可选值字典。

#### 2. `ai_routes` 表 seed 一条 Capability

| capability | 路由建议名 | 说明 |
|------------|-----------|------|
| `feed_discovery` | `feed-discovery-rerank` | 推荐精排 LLM（手动刷新 / 问答低频突发，并发 2）。**不与总结（`summary`/`digest_polish`）共用**，便于分清用量与单独配模型。问答与路由 embedding 仍用通用 `embedding` capability |

需在 `ai_routes` 表 seed 为启用状态并绑定至少一个 provider。未配置时精排会失败，问答/刷新返回错误（粗筛与状态机仍可用）。

#### 3. 调度间隔（可由 `/api/schedulers/:name/interval` 覆盖）

| 调度器 | 默认间隔 | 说明 |
|--------|----------|------|
| `preference_profile_update` | 3600 秒 | 偏好向量画像全量重算（纯向量算术，零 LLM） |
| `rsshub_catalog_sync` | 86400 秒（每日） | 拉取实例 `/api/namespace` 全量同步 + 增量可用性校验 + 新路由 embedding |

#### 4. 发现参数默认值（代码常量，当前不可经 UI 配置）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `RecommendationTopNDefault` | 8 | 每版块粗筛 top-N |
| `DismissCooldownDaysDefault` | 30 天 | 推荐 dismiss 冷却期（跨 source 生效） |
| `SeedMatchThresholdDefault` | 0.5 | 问答 embedding 与板块向量匹配阈值（低于则落全局桶） |
| `SeedMergeAlphaDefault` | 0.4 | 种子加权合并系数 `new = normalize(α×incoming + (1−α)×existing)` |
| 可用性校验速率 | 2 req/s | example 路径异步限流 GET 的默认速率（`CheckAvailability` 参数可覆盖） |

管理员无需额外配置即可使用；以上发现参数为代码默认值，调优需改 `internal/admin/service/discovery_helpers.go` 常量。

## dsh 能源研究本地预设（外部工具，非 Syntopica 应用配置）

本地 dsh（DeepSeek Harness，版本前提 **0.1.2-rc.1**，Web UI `http://127.0.0.1:3080`）的「能源研究」用户预设：原油供需研究专用受限 agent（仅网页检索/抓取 + 提问 + 压缩；无 Shell、文件编辑、委派能力）。本节只描述外部工具配置，不影响 Syntopica 前后端。

| 位置 | 路径 | 说明 |
|------|------|------|
| 仓库配置源（唯一编辑点） | `config/dsh/presets/energy-research/`（`preset.yml` + `agent.cordis.yml`） | 进 git 可追溯；改动后需手动同步到 live |
| live 部署 | `C:/Users/Admin/.dsh/.agent-presets/energy-research/`（同两文件） | dsh 用户预设根目录，下一次 roster 读取自动发现，无需重启 |

- **同步方式**：将仓库源两文件逐字节复制到 live 目录（部署时已验证 SHA256 一致：preset.yml `afa39270…`、agent.cordis.yml `a3fd3073…`）。
- **选用**：dsh Web UI 新建会话时在预设选择器选「能源研究」（排在「标准模式」之后，`order: 20`）。预设只在会话尚未产出内容时可选；**旧会话与部署默认预设不受影响**（未改 `settings.yaml`/模型/权限/凭据）。
- **回退**：删除 `C:/Users/Admin/.dsh/.agent-presets/energy-research/` 目录即停止向新会话提供该预设；已运行会话及其历史不会被删除/撤销，仓库源保留。
- **能力边界（诚实声明）**：当前**未接入** EIA/JODI/STEO 等专业数据接口（无 MCP 行、无虚构端点），取材仅限网页检索/抓取；persona 已约束不编造数值、缺证据停止。工具白名单≠OS 沙箱隔离。搜索 `maxUses` 与 host `maxParallelToolCalls` 保持原样，**尚无整场 token/硬预算限制**。
- 行为契约为 openspec change `configure-dsh-energy-research`（spec：`dsh-research-preset`）。

