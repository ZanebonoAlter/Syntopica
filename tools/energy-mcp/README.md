# tools/energy-mcp — 只读能源数据 MCP 服务器

> **⚠️ 已停止演进（2026-09-19）**：数据源已转入 Syntopica 产品本体（Go 实现
> `backend-go/internal/datasources/`，openspec change `integrate-research-data-sources`）；
> 本目录继续可用（dsh 侧 live 预设依赖它），但后续口径以 Go 实现与
> `docs/reference/configuration.md`「研究数据源」节为准，不再更新。

固定只读 stdio MCP 服务器，向 dsh「能源研究」预设供给两个官方公开数据工具：
EIA WPSR Table 1（美国周度原油库存/供需）与 JODI Oil Primary（各经济体月度原油产量/进出口/期末库存）。
**无 API key、无凭据读写、无数据迁移**；工具参数里没有任何 URL/路径/代码类字段。

- `server.py`：薄 FastMCP 封装（官方 `mcp` SDK，`mcp.server.mcpserver.MCPServer`，mcp 1.x 自动回退 FastMCP）。stdout 只输出 MCP 协议消息，日志走 stderr。
- `sources.py`：固定官方 URL 取数（httpx）+ 纯解析（stdlib csv/Decimal）。
- `pyproject.toml`：依赖仅 `mcp>=1.2.0`、`httpx>=0.27`；`requires-python = ">=3.11"`；`package = false`。
- `tests/`：unittest（无新测试框架），网络 mock；E2E 一条真实联网（显式 opt-in）。

## 安装 / 依赖同步（Windows cmd）

```bat
cd /d D:\project\Syntopica\tools\energy-mcp
uv sync --frozen
```

- 已实测解释器：Windows CPython **3.13.3**（uv 0.7.9 托管 venv）。
- `uv.lock` 冻结依赖；dsh 启动命令带 `--frozen --no-sync`，会话建立零安装等待。
- **改了 `pyproject.toml` 后必须重新 `uv lock && uv sync`**，否则 `--frozen` 启动会失败（preset 配了 `failOnStartupError: true`，启动失败会中止预设应用并报错，而不是静默缺工具）。

## 启动（dsh 实际使用的精确命令）

```bat
uv run --frozen --no-sync --project D:/project/Syntopica/tools/energy-mcp python D:/project/Syntopica/tools/energy-mcp/server.py
```

stdio 通信，无需端口。Windows 绝对路径用正斜杠（bash→cmd 反斜杠会被吞）。此形态已被 E2E 测试与 live smoke 实测通过（见下文证据）。

## 测试

```bat
cd /d D:\project\Syntopica\tools\energy-mcp
uv run --frozen python -m unittest discover -s tests -v
```

- 离线默认（2026-09-09 评审前快照，修复后以实测为准）：**62 通过 + 1 skip**（`test_e2e_live`，双条件：Windows + `ENERGY_MCP_LIVE=1` 显式 opt-in 才真联网）。
- 真实联网 E2E（会真实下载两源文件）：

```bat
set ENERGY_MCP_LIVE=1
uv run --frozen python -m unittest tests.test_e2e_live -v
```

- 单测子集（纯 mock 无网络）：`uv run --frozen python -m unittest tests.test_eia tests.test_jodi tests.test_http tests.test_server_schema tests.test_preset_config -v`。

## 工具速查

### `eia_wpsr_table1(section: "stocks" | "supply" = "stocks")`

固定 URL `https://ir.eia.gov/wpsr/table1.csv`（CP1252、引号包裹、千分位、M/D/YY 日期）。仅美国、周度（期=周五截止）。

| section | 返回（均含当周+上周值与各自周结束日期） | 单位 |
| --- | --- | --- |
| `stocks` | 原油总库存（含 SPR）`crude_stocks_total_incl_spr`、商业库存 `crude_stocks_commercial`、SPR `crude_stocks_spr` | MMbbl（百万桶） |
| `supply` | 国内产量 `crude_production`、原油进口 `crude_imports`（不含 "Imports by SPR" 子项）、原油出口 `crude_exports`（Crude Oil Supply 组） | Mb/d（千桶/日） |

返回保留原始行标签、`source_row` 行号、`raw_value`；不含 4 周均/YTD 列值。

### `jodi_oil_primary(geo: "US", flow: "production", unit: null, month: null)`

年度模板 URL `https://www.jodidata.org/_resources/files/downloads/oil-data/annual-csv/primary/primaryyear<YYYY>.csv`；product 固定 **CRUDEOIL**。

| 参数 | 规则 |
| --- | --- |
| `geo` | 两个大写字母且数据中实际存在（拒绝 URL/路径）；实测 96 个经济体码 |
| `flow` | `production`→INDPROD、`imports`→TOTIMPSB、`exports`→TOTEXPSB、`closing_stocks`→CLOSTLV |
| `unit` | `KBD`（产量/进出口默认）、`KBBL`（期末库存默认）；**不自动换算**，`closing_stocks`+`KBD` 组合拒绝（INVALID_ARGUMENT，无意义且不换算） |
| `month` | `YYYY-MM`（2002..当前年，未来月份拒绝）；`null` → 本年文件中该维度最新月；本年文件 404 → **最多**回退上一年文件一次，结果注明 `year_used`/`year_strategy` |

## 数据口径（红线）

- **缺值不转 0、不凑数**：EIA 成对 en-dash（CP1252 0x96）→ `null`；JODI `-`/`..`/`x` → `value=null` + `raw_value` 保留 + `missing_reason`；真实 `0` 仍为 0。
- **单位不换算、不跨源合并**：EIA 周度（MMbbl/Mb/d）与 JODI 月度（KBBL/KBD）口径不同，绝不自动换算或合并；JODI 不使用 CONVBBL 换算系数。
- **JODI `assessment_code` 原样透传**（实测值 1/2/3），官方语义未核实，**不是**质量好坏标记，不得解释。
- **统计期与抓取时刻分开**：`TIME_PERIOD`/`period` 是数据期（JODI 文件滞后数据月约 1.5~2 个月）；`retrieved_at` 是本次抓取时刻。
- **`last_modified` 原样归一化自 HTTP `Last-Modified` 头，不是已证实的发布日期**（EIA 惯例为发布时刻，未核实），不得当作独立发布证据；JODI 文件覆盖式更新、无修订历史列。
- EIA 仅美国数据，**不得称全球**；JODI 非每月全覆盖，无匹配返回明确 `no_data`（空 observations）。
- 未匹配到的来源或 `no_data` 与网络/格式错误区分：`no_data` 是成功返回。

## 网络与缓存

- 仅请求两个固定官方 host（`ir.eia.gov`、`www.jodidata.org`）；重定向仅允许同 host、最多 3 次；单响应 ≤32MB；单请求超时 45s。
- 内存 TTL 缓存 15 分钟、按源/年；**错误不缓存**；缓存命中保留原 `retrieved_at`/`source_sha256`（不刷新成假新时间）。
- 无写文件/shell/后台下载工具，不读环境密钥。

## 错误码

| 码 | 含义 |
| --- | --- |
| `INVALID_ARGUMENT` | 参数错误（本地可判定，不发网络请求） |
| `SOURCE_UNAVAILABLE` | 网络/超时/404/大小超限/HTML 冒充 CSV |
| `SCHEMA_CHANGED` | 源结构漂移（缺必需列/重复冲突/头部变化），不静默取错列 |

工具异常用 MCP 标准 `isError`/`ToolError` 表达，消息含稳定错误码；不吞异常返回空成功。

## 故障与回退

- dsh 新会话中工具不出现或预设应用失败：检查 (a) Windows `uv` 在 PATH；(b) 已 `uv sync --frozen`；(c) 启动命令与上方一致（`failOnStartupError: true` 时 dsh 会报启动错误）。
- **回退**：将 `openspec/changes/connect-dsh-energy-data-sources/evidence/preset-backup/` 中改动前的 `agent.cordis.yml` 和 `preset.yml` 同时恢复到仓库配置源与 live 目录，并核对两处逐字一致。不能只删 MCP 行：persona 和预设描述也必须同步恢复为专业接口未接入的状态。恢复后新建会话使用旧工具面；已有会话不会被删除或自动重新挂载。
- live 部署目录：`C:/Users/Admin/.dsh/.agent-presets/energy-research/`（与仓库源 `config/dsh/presets/energy-research/` 逐字一致，两处 diff 需零输出）。
- 不影响 `standard` 预设、旧会话与 host 全局配置（settings/credentials/profiles/默认模型/权限均未触碰）。

## 行为契约

详见 openspec change `connect-dsh-energy-data-sources`（spec：`dsh-energy-mcp-tools`）与证据：
`openspec/changes/connect-dsh-energy-data-sources/evidence/`（源样本、live-smoke 摘要、preset-backup）。