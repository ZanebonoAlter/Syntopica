## Context

研究助手方向已定（见 proposal Why + `docs/research/research-assistant-data-sources/explore-findings.md`）：数据源以 Go 进产品，EIA/JODI 口径规格书 = `tools/energy-mcp/` 源码与其 100 个测试；编排层复用现有 `runToolLoop`（`dataenrichment/service/orchestrator.go:576`）与 `Tool`/`Registry`（`tool_registry.go`）。装配点：`internal/dataenrichment/wire.go:87`（`service.NewRegistry(fetcher, opts...)`）。配置模式：`internal/platform/config/config.go`（mapstructure + 环境变量覆盖，参照 `BOCHA_API_KEY`）。建表惯例：GORM AutoMigrate（`postgres_migrations.go` 仅兜底 AutoMigrate 处理不了的类型变更，新表不需要）。

## Goals / Non-Goals

**Goals:** 四源取数器可用且口径可溯源；目录表+查询 API 落地；probe 可验证连通与 key；Tool 适配可注入但不改现有工具面；测试翻译 energy-mcp 口径。

**Non-Goals:** 研究会话/对话页（后续 change）、观测值落库与时序存储、pgvector 源匹配、单位换算、跨源合并、数据看板、原生 function calling 扩展 airouter。

## Decisions

1. **包布局**：新域 `internal/datasources/`（`sources/` 每源一文件 + `cache.go` + `fetch.go` + `handler/` + `tools.go`）。备选（放 dataenrichment 下）被否：数据源域不该耦合 agent loop 域；研究助手后续 change 也要 import 它。
2. **取数器接口**：每源 `Fetch(ctx, 强类型参数) (*Observations, error)`，typed error 三态（InvalidArgument / SourceUnavailable / SchemaChanged，携 detail），不搞 `map[string]any` 贯穿——map 形态只在 Tool.Execute 适配层做一次转换。
3. **HTTP 预算层自建**（`fetch.go`）：host 白名单、有界重定向（仅 https 同源）、45s 时长 / 32MB 流式大小预算、HTML 内容检测。备选（复用 dataenrichment `HTTPFetcher`，5s）被否：JODI 年度文件实测 4.76MB，5s 不够且无大小预算。
4. **缓存**：进程内 TTL 900s（energy-mcp 实证口径），key=源+规范化参数；错误不缓存；缓存命中保留原 retrieved_at；SCHEMA_CHANGED/HTML 冒充驱逐。
5. **data_sources 表**：源静态属性（覆盖/主题/频度/滞后/单位纪律/requires_key）的**唯一真源是 Go 目录常量**（每源一处定义，同处派生 Tool Description 与 seed 数据）；表存静态属性快照 + 运行时状态（status/last_probe_at/last_error），首启及每次启动 upsert seed（以 code 幂等）。`GET /api/datasources` 读表（含运行时状态）。备选（表为唯一真源）被否：目录属性与工具行为必须一致，双头维护会漂移。
6. **Comtrade key**：`config.go` 增 `Comtrade.APIKey`（`mapstructure: comtrade.api_key`），环境变量 `COMTRADE_API_KEY` 覆盖，风格照 `BOCHA_API_KEY`；空值 → 源 disabled + probe/取数报「配置缺失」型 SOURCE_UNAVAILABLE（消息指向配置项名）。
7. **Tool 适配注入**：`datasources/tools.go` 产出 `[]service.Tool`（Execute 内做 map→强类型转换 + typed error→错误 JSON）；Registry 增最小侵入的动态注册口（`Register(tools ...Tool)` 或等价 option，实现时按 `tool_registry.go` 现结构择最小改动），`wire.go` 装配时调用。**不加入任何现有 agent 的 allowedTools 列表**——默认工具面不变（spec 场景锚定）。
8. **HTTP 状态映射**（probe/取数类端点）：成功 200；INVALID_ARGUMENT→400；SOURCE_UNAVAILABLE→502；SCHEMA_CHANGED→502 且 body.error_code 区分。错误 body 统一 `{error_code, message, detail}`。
9. **EIA/JODI 解析口径**：从 `tools/energy-mcp/sources.py` 逐条翻译（CP1252+en-dash 对→null、千分位逗号、M/D/YY 日期、JODI 七列/三种缺失标记/CONVBBL 不当流量值/404 回退一次），fixture 从 energy-mcp 测试样本裁剪；结构漂移一律 SCHEMA_CHANGED，不做模糊容错。
10. **测试**：每源表驱动单测（httptest server mock 上游）+ HTTP 预算/缓存层单测 + Comtrade key 缺失路径真实测（无 key 也能测）；一条真联网 smoke（`DATASOURCES_LIVE=1` opt-in，照 energy-mcp `test_e2e_live` 模式），WDI/EIA/JODI 三源匿名可 smoke，Comtrade smoke 仅在 key 已配置时执行。后端测试红线遵守 `standard/backend/testing.md`（不碰真实业务库、mock 外部依赖）。

## Risks / Trade-offs

- [Comtrade 响应字段细节未实测（key 未注册）] → 参数面按官方 API 文档惯例定窄，mock 按文档建模；key 到位后 smoke 校准字段映射，必要时窄化参数（属实现细节修正，不改 spec 语义）。
- [Comtrade 免费 key 限速未知] → TTL 缓存 + 单请求串行化缓解；注册后实测若严苛，再加请求间隔/配额（后续小改）。
- [EIA/JODI 上游结构漂移（历史事实）] → SCHEMA_CHANGED 显式失败 + 驱逐缓存，由 probe 暴露给用户，不做模糊容错。
- [树莓派上 JODI 4.76MB 下载时长] → 45s 预算 + TTL 缓存；首次慢属预期，实测校准预算值。
- [目录常量与表快照漂移] → 每次启动 upsert seed，表只增不删（删除源属人工操作）。

## Migration Plan

1. AutoMigrate 建 `data_sources` 表 → 启动 upsert seed 四源 → 部署。
2. 配置 `COMTRADE_API_KEY`（可选，缺省三源可用）；probe 逐源验证。
3. 回滚：本 change 全增量（新表无观测值、新包、新路由、注入点不激活默认行为），回滚部署即回退；表可保留无副作用。

## Open Questions

- Comtrade 免费 key 的具体限速与响应字段细节——注册后 smoke 实测校准（不阻塞实现，mock 先行）。
- data_sources 目录是否需要管理界面/启停操作——后续 change 视研究会话需要再定。
