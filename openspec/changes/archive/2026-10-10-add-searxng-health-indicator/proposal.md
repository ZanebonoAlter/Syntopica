<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, data-enrichment -->

## Why

2026-10-01 21:22 起宿主机 Clash 国外出口静默失效，SearXNG 全部引擎 timeout、返回空结果，页边注问答按「联网是增益不是依赖」设计静默降级——两天内系统无任何用户可见信号，用户误以为「LLM 失去了联网能力」（诊断存档：docs/research/margin-note-websearch-silent-degrade/）。本 change 给联网链路补观测面：设置面板可主动探测健康，回答卡片在联网失效时可见降级标记。

## What Changes

- **后端健康探测端点**：新增 `GET /api/settings/searxng/health`——复用现有 `marginNoteSearxngEndpoint` 配置解析（DB > env > yaml，现读现生效），发起一次真实探针搜索（固定简短 query），返回 `{status, latency_ms, detail}`。状态枚举：
  - `disabled`：未配置/未启用（预检不过）
  - `unreachable`：endpoint 连接失败/超时
  - `degraded`：HTTP 可达但 results 为空（引擎全挂，本次事故形态）
  - `ok`：可达且返回结果非空
- **设置面板健康标记**：SearXNG 配置面板（数据源网络 section 内）新增「测试连接」按钮 + 四态状态标记（中性色点 + 短文案），点击即时探测，保存配置后自动探测一次。
- **QA 联网状态落库与透出**：`annotation_qas` 新增 `web_search_status` 列（`disabled` / `ok` / `empty`；empty=链路启用但搜索失败或结果为空），QA 响应与持久化带出该字段。
- **回答卡片降级标记**：MarginNoteCard 问答区在 `web_search_status=empty` 时显示「联网搜索无结果」小标记（含 title 提示去设置面板探测）；`disabled` 时不显示（用户主动关闭，不打扰）。

不改动：搜索链路本身（预取+注入模式）、静默降级行为（回答绝不因联网失败而阻断）、`web_sources` 白名单防编造校验。

## Impact

- Affected code: `backend-go/internal/topicgraph/service/margin_notes_websearch.go`（探测函数复用既有钩子）、`backend-go/internal/topicgraph/service/margin_notes_qa.go`（状态采集与写入）、`backend-go/internal/topicgraph/repository/margin_notes_repository.go`（模型字段）、`backend-go/internal/discovery/handler/discovery_handler.go`（健康端点）、`backend-go/internal/app/router.go`（路由）、`front/app/api/searxng.ts`、`front/app/components/dialog/SearxngConfigPanel.vue`、`front/app/features/tags/components/daily-report/MarginNoteCard.vue`
- Data migration: `annotation_qas` 加一列（非破坏，旧行 NULL 视为历史数据不显示标记）
- Modified specs:
  - `[daily-report-margin-notes]` 问答联网状态可见性 — spec status: MODIFIED
  - `[settings-workspace]` 数据源连通性探测 — spec status: MODIFIED
