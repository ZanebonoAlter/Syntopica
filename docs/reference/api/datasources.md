# 研究数据源 API（datasources）

> 通用约定（响应格式、错误码、分页）见 [_conventions.md](_conventions.md)。配置与口径速查见 [configuration.md](../configuration.md)「研究数据源」节；行为契约为 openspec spec `research-data-sources`。

只读目录 + 连通验证。七源取数本身不暴露为 HTTP API——它们以 agent 工具形态（`eia_wpsr_table1` / `jodi_oil_primary` / `wb_wdi` / `un_comtrade_trade` / `yahoo_chart` / `fred_series` / `gdelt_doc`）供信号研究 loop 消费（add-fast-frequency-datasources 扩三快频源）。

## GET /api/datasources

返回数据源目录全量（`data_sources` 表投影，含运行时状态）。

- 响应：`{ "data_sources": [ { code, name, provider, homepage_url, coverage, topics[], frequency, typical_lag, unit_policy, requires_key, config_key_name?, status, status_reason?, last_probe_at?, last_error? } ] }`
- `status`：`enabled` / `disabled`（需 key 的源在 key 未配置时 disabled，`status_reason` 指向配置项名）。**key-requiring 源的 status 每次现读配置链**（界面 DB > env > config.yaml），配置变化免重启翻转。
- 示例：`un_comtrade` 在 key 全未配置时 `status="disabled"`、`status_reason` 含配置项名。

## POST /api/datasources/{code}/probe

对指定源执行一次真实取数（各源预定义最小参数集：EIA stocks / JODI US production / WDI NE.EXP.GNFS.ZS×CHN / Comtrade A·156·2709·M·2024 / Yahoo CL=F·1mo / FRED DCOILWTICO / GDELT oil·timelinevol·7d），验证连通性与 key 配置。GDELT 免费档限流 1 次/5 秒，连续 probe 可能 429（`SOURCE_UNAVAILABLE`，属速率限制非故障）。

- 成功：`200 { code, elapsed_ms, summary: {rows, ...}, retrieved_at }`。
- 失败：`400 { error_code: "INVALID_ARGUMENT", message }`（code 非法）或 `502 { error_code: "SOURCE_UNAVAILABLE" | "SCHEMA_CHANGED", message, detail, elapsed_ms }`（配置缺失/网络/结构漂移可区分）。
- **probe 不改目录状态**（`status`/`last_probe_at`/`last_error` 不写库）。

## GET /api/settings/comtrade

读 UN Comtrade 订阅 key 配置（bocha 同款脱敏语义）。返回 `{ api_key_configured, api_key_hint(末 4 位), enabled }`，**绝不回显完整 key**。

## POST /api/settings/comtrade

写 Comtrade 配置，即时生效（动态读，无需重启）。请求体 `{ api_key?, enabled? }`：`api_key` 空串=保留原值（防表单回填清 key）；`enabled` 缺省=不改，`false` 时跳过界面值走 env/config.yaml 兜底。

## GET /api/settings/fred

读 FRED API key 配置（comtrade 同款脱敏语义，add-fast-frequency-datasources）。返回 `{ api_key_configured, api_key_hint(末 4 位), enabled }`，**绝不回显完整 key**。

## POST /api/settings/fred

写 FRED 配置，即时生效（动态读，无需重启）。请求体 `{ api_key?, enabled? }`：`api_key` 空串=保留原值；`enabled` 缺省=不改，`false` 时跳过界面值走 `FRED_API_KEY` env / config.yaml 兜底。保存成功即回写目录 `fred` 行 status。
