<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: configuration, data-enrichment -->

## Why

用户研究方向已从 dsh（DeepSeek Shell）侧工具转为 Syntopica 产品本体的「研究对话助手」（决策记录见 `docs/research/research-assistant-data-sources/explore-findings.md`）：助手需要内置结构化官方数据源，而不是让模型靠网页正文抄数。现有资产 `tools/energy-mcp/`（Python，dsh 侧 MCP 服务器）已停止演进，其口径知识需 Go 化进产品；同时首批源需按 2026-09-19 实测结论补充世行 WDI（匿名可用）与 UN Comtrade（强制 key）。

## What Changes

- 新增后端数据源域 `backend-go/internal/datasources/`，可插拔多源架构：
  - **取数器**（每源一个，纯函数式：ctx + 参数 → 结构化结果 + 元数据）：EIA WPSR 周度（stocks/supply，口径照 energy-mcp 规格书 Go 化）、JODI Oil Primary 月度（含 404 回退一次/缺失标记不转 0/单位不换算）、世行 WDI 宏观（新，匿名 API）、UN Comtrade 商品贸易（新，需订阅 key）。
  - **统一错误语义**：INVALID_ARGUMENT / SOURCE_UNAVAILABLE / SCHEMA_CHANGED（沿用 energy-mcp 三码），结构漂移显式失败不做模糊容错。
  - **网络与缓存约束**：host 白名单、45s 时长 / 32MB 大小预算、内存 TTL 缓存（错误不缓存、命中保留原 retrieved_at）；数据**不落库**（取数快照由后续研究会话的 tool 结果存储承担，见 design）。
- 新增**数据源目录表** `data_sources`（code/名称/覆盖/主题/频度/典型滞后/单位纪律/是否需 key/状态），目录 metadata 首期进 schema，为后期 pgvector「研究问题→数据源」匹配留底；含目录查询 API。
- 新增 probe API：`GET /api/datasources`（目录）、`POST /api/datasources/{code}/probe`（触发一次真实取数返回摘要+耗时，验证连通与 key 配置）。
- UN Comtrade 订阅 key 走后端配置（缺失时该源在目录中显式 disabled 且 probe 报明确原因，不假装可用）；`docs/reference/configuration.md` 增补配置项与四源口径速查。
- **工具适配**：数据源取数器适配为 `dataenrichment/service.Tool`（Name/Description/InputSchema/Execute）并经依赖注入挂进现有 Registry（模式同 WithWebSearcher）；**不改现有探索/QA 的 allowedTools 默认集**——研究助手消费这些工具是后续 change 的事。
- 不改前端（front/ 零触碰）、不改现有 AI 调用行为、无数据库迁移以外的存量数据变化（新表为空表 + seed 目录）。

## Capabilities

### New Capabilities

- `research-data-sources`: 研究数据源域行为契约——目录表与查询 API、四源取数语义（参数/返回结构/单位与缺失纪律/错误码）、网络与缓存约束、Comtrade key 配置语义、probe 端点、Tool 适配注入而不改变现有 enrichment 工具面。

### Modified Capabilities

（无——不触及任何现有 openspec spec 的需求；`data-enrichment` 域代码仅新增可选注入点，不改行为契约。）

## Impact

- 新增：`backend-go/internal/datasources/**`（取数器/缓存/HTTP 预算层/工具适配/handler）、`data_sources` 表迁移 + seed、`/api/datasources*` 路由。
- 修改：`backend-go/internal/app/runtime.go`（装配注入）、`docs/reference/configuration.md`（数据源配置节）、`docs/reference/flow/`（新域 flow 文档，归档后按 §12 溯源）。
- 外部依赖：UN Comtrade 免费 key（用户注册一次，配置后启用）；其余三源匿名。
- 部署后影响：现有功能零变化（新表、新端点、新可选注入均为增量）；用户需在配置 Comtrade key 后该源才可用，未配置不影响其余三源。
