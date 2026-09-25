<!-- complexity: complex -->
<!-- ui-impact: minor -->

## Why

energy-research 预设当前只有网页检索/抓取，persona 明确声明「未接入 EIA/JODI 专业数据接口，不得假装查询过」——用户做原油供需研究时拿不到结构化、可溯源的官方数值，只能靠网页正文人工抄数。已实测 EIA WPSR 与 JODI Oil Primary 都是匿名公开 CSV（证据在 evidence/），可以零 key 接入：做一个固定只读的 stdio MCP 服务器把两个源的官方数值以带元数据（单位/频度/时期/溯源）的形式供给 dsh 会话。

## What Changes

- 新增 `tools/energy-mcp/`：独立 Python stdio MCP 服务器（官方 `mcp` SDK FastMCP，不手写 JSON-RPC），依赖仅 `mcp`+`httpx`，解析用 stdlib csv/Decimal：
  - `sources.py`（或 `energy_sources/` 包）：纯解析 + 固定官方 URL 取数，无任意 URL/路径/代码执行参数。
  - `server.py`：薄 MCP 封装，stdout 只出 MCP 协议消息，日志走 stderr。
  - `tests/`：unittest（无新测试框架），网络 mock；另含一条真实联网的官方 ClientSession/stdio 端到端测试（精确复刻 dsh 的 uv 启动命令）。
- 两个只读工具：
  - `eia_wpsr_table1(section)`：美国周度原油库存（stocks：商业/SPR/总量区分）或供需（supply：国内产量/原油进口/原油出口），返回当周+上周观测值与各自周结束日期、单位（MMbbl/Mb/d）、原始行标签与 source_row 溯源；缺列/重复冲突/格式漂移显式 SCHEMA_CHANGED。
  - `jodi_oil_primary(geo, flow, unit, month)`：product 固定 CRUDEOIL；产量 INDPROD/进口 TOTIMPSB/出口 TOTEXPSB/期末库存 CLOSTLV；单位 KBD/KBBL 不换算（closing_stocks+KBD 拒绝）；缺值 '-'/'..'/'x' 保留 null+missing_reason；month 未给取最新期（即使该期值为 null 不静默回退旧值）；本年 404 最多尝试上一年一次并注明策略。
  - 统一错误码 INVALID_ARGUMENT / SOURCE_UNAVAILABLE / SCHEMA_CHANGED（MCP isError/ToolError），常规 no_data 与网络/格式错误区分。
- 网络与资源约束：仅核定官方 HTTPS host、有界重定向/大小（32MB）/时长预算、内存 TTL 缓存（错误不缓存、缓存保留原 retrieved_at/sha256）、无后台下载、不写任意文件、不开 shell。
- preset 集成：`agent.cordis.yml` 追加**一条** `@deepseek-ai/dsh-mcp-client`（stdio、serverName `energy_data`、uv `run --frozen --no-sync --project ...` 启动、`failOnStartupError: true`、`toolCallTimeoutMs: 90000`）；persona 改为「先调用这两个工具取官方数据、按工具 metadata 写报告、工具失败承认证据缺口」，去掉「未接入/无 MCP」旧声明；`preset.yml` description 同步。live 部署仅同步两个配置文件到 `~/.dsh/.agent-presets/energy-research/`，改前核对源/live 一致，不一致停止；旧配置非敏感副本留 evidence/。
- `docs/reference/configuration.md` dsh 节增补：安装/启动/新会话生效/依赖同步/两工具参数与口径/故障与回退/无 key 无迁移。

无 Syntopica 应用代码改动（不改 front/、backend-go/）；不新增数据库/守护进程/linter。

## Capabilities

### New Capabilities

- `dsh-energy-mcp-tools`: 能源数据只读 MCP 服务器的行为契约——两工具的参数/返回/错误语义、来源解析口径（单位/缺失标记/时期选择）、网络与缓存约束、uv 冻结启动形态、preset 单服务器接线与 persona 纪律更新、配置文档。

### Modified Capabilities

（无——`dsh-research-preset` 主 spec 尚未同步（configure-dsh-energy-research 仍 active），本 change 不改旧 change 制品；其 delta 中「无 MCP 行/persona 声明未接入」场景与本 change 的冲突由归档时对账处理，已在 Impact 声明。）

## Impact

- 新增文件：`tools/energy-mcp/**`（pyproject.toml、uv.lock、server.py、sources、tests、README）。
- 修改文件：`config/dsh/presets/energy-research/agent.cordis.yml`、`preset.yml`、`docs/reference/configuration.md`（现有 dsh 节）。
- live 部署：`~/.dsh/.agent-presets/energy-research/` 两文件同步；不触碰 `.credentials.yaml`/`settings.yaml`/profiles/默认模型/权限/host 全局配置。
- 并发提示：active change `configure-dsh-energy-research` 的 spec delta 含「无 MCP 行、persona 声明专业接口未接入」场景，与本次接线冲突——其归档前需按新事实对账（主线程已知情，本 change 不改其制品）。
- 部署后影响：dsh 新建会话选「能源研究」预设后，模型可见并可直接调用 `mcp__energy_data__eia_wpsr_table1` / `mcp__energy_data__jodi_oil_primary`；`standard` 预设与旧会话不受影响；首次启动需 `uv sync` 已完成（lock 冻结，无安装等待）。无数据迁移，无 API key。
