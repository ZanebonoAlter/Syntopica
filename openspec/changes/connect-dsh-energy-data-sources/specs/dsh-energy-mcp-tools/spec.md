## Purpose

能源数据只读 MCP 服务器（EIA WPSR + JODI Oil Primary）的工具契约、解析口径、网络与缓存约束、dsh preset 接线与配置文档。

## ADDED Requirements

### Requirement: EIA WPSR 周报工具

`eia_wpsr_table1(section)` SHALL 从固定官方 URL（ir.eia.gov/wpsr/table1.csv，CP1252、引号包裹、千分位逗号、M/D/YY 日期）解析并返回美国周度数据：`section="stocks"` 返回三种原油库存（Crude Oil 含 SPR 总量、Commercial (Excluding SPR)、Strategic Petroleum Reserve (SPR)，单位百万桶 MMbbl，依据 WPSR Table 1 官方表定义而非 CSV 内标注）；`section="supply"` 返回 Crude Oil Supply 组下国内产量（"(1) Domestic Production"）、原油进口（"(8) Imports"，不得混入 "Imports by SPR" 等子项）、原油出口（"(12) Exports"，单位千桶/日 Mb/d）。每个观测值 MUST 含当周与上周值及各自周结束日期；返回 MUST 保留原始行标签、source_row 行号、raw_value；标签匹配 MUST 在 trim/规范化空白与脚注号后按分区精确定位；MUST NOT 返回 4 周均/YTD 列当周值。范围仅美国，MUST NOT 声称全球。

#### Scenario: stocks 分区返回三类库存
- **WHEN** 调用 `eia_wpsr_table1(section="stocks")` 且源文件为 evidence 样本结构
- **THEN** 返回三条观测（含 SPR 总量 711.064、商业 424.460、SPR 286.604 量级），单位 MMbbl，含当周/上周值与各自周结束日期、原始行标签与 source_row

#### Scenario: supply 分区返回三项供需
- **WHEN** 调用 `eia_wpsr_table1(section="supply")` 且源文件为 evidence 样本结构
- **THEN** 返回国内产量 13,862、原油进口 6,770、原油出口 4,483（Mb/d，千分位已去逗号），行标签为 "(1)     Domestic Production"、"(8)        Imports"、"(12)        Exports"，不含 4 周均/YTD 列值

#### Scenario: 缺失标记映射 null
- **WHEN** 单元格值为成对 en-dash "– –"（CP1252 0x96）
- **THEN** 该值映射为 null，不解析为 0 或乱码；数值 0 仍为 0

#### Scenario: 结构漂移显式报错
- **WHEN** 必需列/指标缺失、或同一目标行匹配到多个冲突候选
- **THEN** 返回错误码 SCHEMA_CHANGED，不静默取错列

### Requirement: JODI Oil Primary 工具

`jodi_oil_primary(geo, flow, unit, month)` SHALL 从年度模板 URL（jodidata.org …/primary/primaryyear<YYYY>.csv，UTF-8 七列明文 CSV）解析：product 固定 CRUDEOIL；flow 映射 production→INDPROD、imports→TOTIMPSB、exports→TOTEXPSB、closing_stocks→CLOSTLV；geo MUST 校验两个大写字母且在实际数据中存在（拒绝 URL/路径）；单位 KBD=千桶/日、KBBL=千桶，不使用 CONVBBL、不跨单位换算，`closing_stocks` 与 `unit="KBD"` 组合 MUST 拒绝（INVALID_ARGUMENT）；month 格式 YYYY-MM，年份限定 2002..当前年，坏月份/未来月份拒绝；month 未给时取本年文件中该维度最新月份，即使该期 OBS_VALUE 为缺值也 MUST NOT 静默回退旧有效值；本年文件 404 时最多尝试上一年一次，结果 MUST 注明实际使用年份与选择策略，其他错误不伪装 fallback 成功。缺值标记 '-'、'..'、'x' 一律 value=null 并保留 raw_value 与 missing_reason；assessment_code 原样透传，MUST NOT 映射为质量好坏语义（官方语义未核实）。无匹配返回明确 no_data 与空 observations，不凑 0；库存口径 MUST NOT 假定与 EIA 商业库存同口径。

#### Scenario: 指定月份取对应年度数据
- **WHEN** 调用 `jodi_oil_primary(geo="US", flow="production", month="2026-06")` 且 2026 文件含该行
- **THEN** 返回 value=13818.0、unit=KBD、period=2026-06、assessment_code="1"、frequency=monthly、原始行溯源与源 URL/sha256

#### Scenario: 默认流量单位与库存单位
- **WHEN** 不传 unit 且 flow 分别为 production 与 closing_stocks
- **THEN** production 默认 KBD、closing_stocks 默认 KBBL

#### Scenario: 三种缺值标记
- **WHEN** OBS_VALUE 为 '-'、'..' 或 'x'
- **THEN** value=null，raw_value 保留原标记，missing_reason 标注；US 2026-06 CLOSTLV/KBD 实测 'x' 不转 0

#### Scenario: 最新期缺值不回退
- **WHEN** month 未给且最新期目标维度 OBS_VALUE 为缺值
- **THEN** 返回该最新期的 null 观测（带 missing_reason），不返回旧期有效值

#### Scenario: 本年 404 回退上一年一次
- **WHEN** 本年文件 HTTP 404
- **THEN** 最多请求上一年文件一次，成功则结果注明所用年份与回退策略；上一年也 404 或其他 HTTP/网络错误返回 SOURCE_UNAVAILABLE，不伪装成功

#### Scenario: 非法参数拒绝
- **WHEN** geo 非 2 大写字母（如 "usa"、URL、路径）、month 非 YYYY-MM、未来月份、或 closing_stocks+KBD
- **THEN** 返回 INVALID_ARGUMENT，不发网络请求（能本地判定的）

### Requirement: 统一结果元数据与溯源

两工具成功返回 MUST 含 source（源标识）、url、retrieved_at、last_modified（可空）、source_sha256、observations 数组；每条观测含 geo、product、flow、period、frequency、unit、value（Decimal 计算后安全 JSON 数值）、raw_value、原始行溯源；JODI 额外含 assessment_code 与 missing_reason（如有）。工具异常 MUST 用 MCP 标准 isError/ToolError 表达，错误消息含稳定错误码（INVALID_ARGUMENT / SOURCE_UNAVAILABLE / SCHEMA_CHANGED），不吞异常返回空成功；常规 no_data/缺值与网络/格式错误 MUST 区分。

#### Scenario: 成功结果带完整元数据
- **WHEN** 任一工具成功返回
- **THEN** 顶层含 url/retrieved_at/last_modified/sha256，每条观测含单位/频度/时期/溯源，且不宣称实时供需或全球全覆盖

#### Scenario: HTML 冒充 CSV 被拒
- **WHEN** HTTP 200 但响应体为 HTML（错误页）
- **THEN** 返回 SOURCE_UNAVAILABLE 或 SCHEMA_CHANGED，不误解析为 CSV

### Requirement: 网络与资源约束

服务器 MUST 仅请求核定官方 HTTPS host（ir.eia.gov、www.jodidata.org），不暴露自定义 URL/env 端点参数；重定向仅允许同 host 且次数有界；单响应上限 32MB、单次工具调用 HTTP 总预算约 45s、请求次数有界；实现内存 TTL 缓存（按源/年，约 15min）限制重复下载，错误不缓存、不用过期缓存充新值，缓存命中仍返回原 retrieved_at/sha256；MUST NOT 提供写文件/shell 工具或后台定时下载。

#### Scenario: 不接受任意 URL
- **WHEN** 检查工具参数 schema
- **THEN** 无 URL/路径/代码类参数，仅维度过滤参数

#### Scenario: 缓存命中保留原元数据
- **WHEN** 同一源在 TTL 内第二次请求
- **THEN** 不重复下载，返回的 retrieved_at/sha256 与首次一致（不刷新成假新时间）

### Requirement: dsh preset 单服务器接线

energy-research 预设的 agent.cordis.yml MUST 追加且仅追加一条 `@deepseek-ai/dsh-mcp-client` 行：transport stdio、serverName `energy_data`、command `uv`、args `run --frozen --no-sync --project <Windows 项目绝对路径> python <server.py 绝对路径>`、cwd 为项目目录、`failOnStartupError: true`、`toolCallTimeoutMs: 90000`；两工具由同一服务器提供，MUST NOT 为两来源各启一台。启动形态 MUST 与测试中实测通过的精确命令一致（live 启动无依赖安装等待：lock 冻结且已 uv sync）。persona MUST 去掉「未接入 EIA/JODI、无 MCP 工具」旧声明，改为：先调用这两个工具取对应官方数据、按工具 metadata（统计期/地域/单位/来源）写报告、工具失败时承认证据缺口；STEO 及其他专业接口仍未接入；研究 web 工具保留；不编造数值、不越权。preset.yml description 同步去掉「专业数据接口待接入」。MUST NOT 改 host 全局配置（settings/credentials/profiles/默认模型/权限）。

#### Scenario: 配置行形态正确
- **WHEN** 解析 agent.cordis.yml 的 mcp-client 行
- **THEN** 恰有一条，serverName=energy_data，command/args/cwd 与实测启动命令一致，failOnStartupError=true

#### Scenario: persona 不再虚报也不夸大
- **WHEN** 读取 persona 与 description 文本
- **THEN** 无「未接入 EIA/JODI、无 MCP」旧声明；明确先调两工具、尊重 metadata、工具失败承认缺口；不宣称已接 STEO 或拥有整场 LLM 预算/OS 沙箱

#### Scenario: live 与源一致且保护文件未动
- **WHEN** 部署（仅同步两个配置文件）前后
- **THEN** live 两文件与仓库源逐字一致；`.credentials.yaml`/`settings.yaml`/profiles 未被读取或修改

### Requirement: 配置文档与验证留痕

`docs/reference/configuration.md` 的 dsh 节 SHALL 说明：项目安装与依赖同步命令（uv sync）、dsh 启动形态、新会话生效方式、两工具参数与数据口径（单位/频度/地域/时期选择）、故障与回退（删 mcp-client 行即回退旧工具面）、无 API key、无数据迁移。change evidence SHALL 留存 live smoke 的显式命令与小体积 JSON 结果摘要（无密钥），EIA 与 JODI 分开记；MUST NOT 用硬编码日期值断言最新数据。

#### Scenario: 文档指导部署与回退
- **WHEN** 用户按 configuration.md 该节操作
- **THEN** 能完成依赖同步、理解两工具口径、知道故障时如何回退
