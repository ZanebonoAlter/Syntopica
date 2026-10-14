# Research Data Sources

## Purpose

Syntopica 研究数据源域：为后续研究对话助手提供结构化、可溯源的官方数据取数能力（EIA 周度原油、JODI 月度全球、世行 WDI 宏观、UN Comtrade 商品贸易、Yahoo Finance 日度行情、FRED 宏观/价格序列、GDELT 事件热度时间线）、可查询的数据源目录与连通验证手段。

## Requirements

### Requirement: 数据源目录

系统 SHALL 维护数据源目录表（含 code、名称、覆盖范围、主题、频度、典型滞后、单位纪律、是否需 key、状态），并经 `GET /api/datasources` 返回全量目录。目录 metadata SHALL 为结构化字段（非自由文本堆砌），作为后续数据源智能匹配的基础。

#### Scenario: 目录查询返回全部已注册源

- **WHEN** 调用 `GET /api/datasources`
- **THEN** 返回 eia_wpsr、jodi_oil_primary、wb_wdi、un_comtrade、yahoo_finance、fred、gdelt 七源，每源含 code/名称/覆盖/主题/频度/典型滞后/单位纪律/requires_key/status

#### Scenario: 未配置 key 的源在目录中显式禁用

- **WHEN** UN Comtrade 订阅 key 或 FRED API key 未配置时查询目录
- **THEN** un_comtrade / fred 条目各自 status 为 disabled 且带缺失原因，其余源不受影响

#### Scenario: key-requiring 源的 status 按解析链现算

- **WHEN** 查询目录且 Comtrade/FRED key 已可用（界面 DB 或兜底任一配置）
- **THEN** requires_key 源 status 现算为 enabled（不依赖启动时快照），配置移除即回 disabled
### Requirement: Comtrade key 界面配置（bocha 同款动态链）

UN Comtrade 订阅 key SHALL 支持设置界面配置（`ai_settings` 表 `comtrade_config`：api_key + enabled），优先级为 界面 DB > 环境变量 `COMTRADE_API_KEY` > `config.yaml comtrade.api_key`。key 解析 SHALL 动态进行（取数/目录 status/ probe 每次现读），界面修改即时生效无需重启；enabled=false 时跳过 DB 值走兜底。设置 API SHALL 脱敏回显（已配置标志+末 4 位，绝不回显完整 key），空 api_key 保存 SHALL 保留原值。

#### Scenario: 界面保存 key 即时生效
- **WHEN** 通过 `POST /api/settings/comtrade` 保存非空 key
- **THEN** 后续 `GET /api/datasources` 中 un_comtrade status 翻转为 enabled，取数与 probe 立即使用新 key（无重启）

#### Scenario: GET 设置脱敏回显
- **WHEN** 调用 `GET /api/settings/comtrade`
- **THEN** 返回 api_key_configured/api_key_hint(末 4 位)/enabled，响应中不出现完整 key

#### Scenario: 空串保存不清除已有 key
- **WHEN** 保存 `{"api_key": ""}`（表单回填空场景）
- **THEN** 已存 key 保留不变


### Requirement: FRED key 界面配置（同款动态链）

FRED API key SHALL 支持设置界面配置（`ai_settings` 表 `fred_config`：api_key + enabled），优先级为 界面 DB > 环境变量 `FRED_API_KEY` > `config.yaml fred.api_key`，动态解析语义与 Comtrade 完全同款（界面修改即时生效无需重启；enabled=false 跳过 DB 值走兜底；设置 API 脱敏回显，空 api_key 保存保留原值）。

#### Scenario: 界面保存 key 即时生效

- **WHEN** 通过 `POST /api/settings/fred` 保存非空 key
- **THEN** 后续 `GET /api/datasources` 中 fred status 翻转为 enabled，取数与 probe 立即使用新 key（无重启）

#### Scenario: GET 设置脱敏回显

- **WHEN** 调用 `GET /api/settings/fred`
- **THEN** 返回 api_key_configured/api_key_hint(末 4 位)/enabled，响应中不出现完整 key

#### Scenario: 空串保存不清除已有 key

- **WHEN** 保存 `{"api_key": ""}`（表单回填空场景）
- **THEN** 已存 key 保留不变

### Requirement: EIA WPSR 周度取数语义

EIA 取数 SHALL 按分区（stocks 库存 / supply 供需）返回当周与上周观测值，各自带周结束日期；单位 SHALL 固定为 stocks=MMbbl、supply=Mb/d（依 WPSR 官方表定义，不从数值猜测）；缺失标记（en-dash 对）MUST 映射 null 且不转为 0；结果 MUST 带原始行标签与溯源信息。仅覆盖美国周度数据，不得表述为全球。

#### Scenario: stocks 分区返回三类库存
- **WHEN** 以 section=stocks 取数
- **THEN** 返回商业库存（不含 SPR）/SPR/含 SPR 总量三类，区分标注，各带当周/上周值与周结束日期，单位 MMbbl

#### Scenario: 表结构漂移显式失败
- **WHEN** 上游 CSV 列结构漂移（列数变化/重复列无法去重/未知数值标记）
- **THEN** 返回 SCHEMA_CHANGED 错误并驱逐对应缓存，不做模糊容错匹配

### Requirement: JODI 月度取数语义

JODI 取数 SHALL 固定 product 为原油，流量映射为产量/进口/出口/期末库存；单位 SHALL 保留原始单位不做跨单位换算，非法组合（如库存+流量单位）MUST 拒绝；缺失标记（`-`/`..`/`x`）MUST 映射 null 并保留原始标记；未指定月份时 SHALL 取该文件最新期，即使该期值为 null 也不静默回退旧值；本年文件 404 时 SHALL 最多回退上一年一次并在结果中注明回退策略。

#### Scenario: 指定月份取数带元数据
- **WHEN** 指定国家、流量、单位与月份取数
- **THEN** 返回该期观测值（或 null+缺失标记）与 TIME_PERIOD、抓取时刻等元数据

#### Scenario: 最新期缺值不回退
- **WHEN** 未指定月份且最新期值为缺失标记
- **THEN** 返回最新期的 null+缺失标记，不回退到旧期数值

#### Scenario: 本年 404 回退一次
- **WHEN** 本年年度文件 404
- **THEN** 尝试上一年文件一次并在结果注明 year_used 与回退策略；再 404 则 SOURCE_UNAVAILABLE

### Requirement: 世行 WDI 宏观取数语义

WDI 取数 SHALL 接受指标代码、国家列表（ISO3）与年份范围，匿名调用官方 API；返回每国每年观测值（可为 null）及源端 lastupdated 元数据；国家代码与指标代码 MUST 校验，非法值返回 INVALID_ARGUMENT。

#### Scenario: 多国多指标查询
- **WHEN** 以合法指标代码与多国列表取数
- **THEN** 返回各国各年观测值（含 null 期）与 lastupdated

#### Scenario: 非法参数拒绝
- **WHEN** 指标代码或国家代码不合法
- **THEN** 返回 INVALID_ARGUMENT，不发起网络请求

### Requirement: UN Comtrade 贸易取数语义

Comtrade 取数 SHALL 接受 reporter/partner/商品编码（HS）/流向/期间参数，返回数量与金额观测值并带单位、期间与数据集元数据；调用 MUST 携带订阅 key；key 未配置时该源 SHALL 在目录与 probe 中报明确缺失原因（SOURCE_UNAVAILABLE 指向配置项），不得假装数据不存在或网络故障。

#### Scenario: 双边贸易查询
- **WHEN** key 已配置且以合法 reporter/partner/HS 编码取数
- **THEN** 返回数量与金额观测值（含单位与期间元数据）

#### Scenario: key 缺失时显式报配置缺失
- **WHEN** key 未配置时取数或 probe
- **THEN** 返回 SOURCE_UNAVAILABLE 且消息明确指向缺失的配置项，目录中该源保持 disabled


### Requirement: Yahoo Finance 日度价格取数语义

yahoo_chart 取数 SHALL 仅接受核定 symbol 封闭枚举（首批：能源期货 CL=F/BZ=F/HO=F/RB=F/NG=F、贵金属 GC=F/SI=F、股指 ^GSPC/^IXIC/^DJI/^VIX、汇率 JPY=X/EUR=X/USDCNY=X、加密 BTC-USD；枚举外 symbol 返回 INVALID_ARGUMENT）；interval 固定 daily；返回 SHALL 为 OHLC 收盘序列，每点带交易日期与计价货币，缺失值（接口 null close）如实为 null 不插补；单位原样透传不换算。非官方接口：上游结构漂移、限流或鉴权变化 SHALL 显式失败（SCHEMA_CHANGED / SOURCE_UNAVAILABLE），MUST NOT 模糊容错或静默降级旧数据；典型滞后 T+0（收盘后当日可得）。取数 SHALL NOT 附带成交量口径承诺。

#### Scenario: 合法 symbol 返回日度序列

- **WHEN** 以 symbol=CL=F、range=1mo 取数
- **THEN** 返回逐交易日 OHLC 点（日期+开高低收+货币 USD），最新点为最近收盘日

#### Scenario: 枚举外 symbol 拒绝

- **WHEN** 以 symbol=AAPL（不在核定枚举）取数
- **THEN** 返回 INVALID_ARGUMENT，错误 detail 指向可用枚举，不发起网络请求

#### Scenario: 接口结构漂移显式失败

- **WHEN** 上游响应 JSON 结构变化（chart.result 缺失/时间戳与报价数组长度不齐）
- **THEN** 返回 SCHEMA_CHANGED 并驱逐缓存，不猜测列含义


### Requirement: FRED 序列取数语义

fred_series 取数 SHALL 仅接受格式合法的 series_id（`^[A-Z0-9_]{1,64}$`，非枚举——FRED 序列空间开放，格式即边界）；API key 必须已配置，缺失时返回配置缺失型 SOURCE_UNAVAILABLE（消息指向配置项名）；返回 SHALL 为观测序列（date + value），单位/频率/序列名原样透传（FRED 自带 units/frequency 字段，不换算不推断）；缺失观测（FRED `.` 值）MUST 映射 null 不转 0；观测窗口参数（起止）SHALL 受核定范围约束。

#### Scenario: key 未配置显式错误

- **WHEN** FRED key 未配置时调用 fred_series
- **THEN** 返回 SOURCE_UNAVAILABLE 且 detail 指向 fred_config 配置项，不发起上游请求

#### Scenario: 合法 series 返回带单位观测

- **WHEN** 以 series_id=DCOILWTICO 取数
- **THEN** 返回观测序列（date+value+units="Dollars per Barrel"+frequency="Daily"），最新观测接近当前交易日

#### Scenario: series_id 非法格式拒绝

- **WHEN** series_id 含小写/特殊字符或超长
- **THEN** 返回 INVALID_ARGUMENT，不发起上游请求


### Requirement: GDELT 事件热度取数语义

gdelt_doc 取数 SHALL 仅提供结构化时间线聚合（mode 封闭枚举：timelinevol 声量热度 / timelinetone 情绪基调），MUST NOT 返回新闻正文或观点文本；query 关键词 SHALL 受长度（≤120 字符）与字符集约束（拒绝控制字符/注入式串）；timespan SHALL 为核定枚举（默认 7d，可选 30d/90d）；返回 SHALL 为带时间戳的数据点序列（小时或日粒度随 timespan），零结果 SHALL 如实返回空序列不编造；典型滞后小时级（准实时新闻聚合）。该源是事件层结构化开口：供研究 loop 感知「近期某话题报道热度走势」，SHALL NOT 被表述为事实核查或新闻检索工具。

#### Scenario: 关键词热度时间线返回

- **WHEN** 以 query="iran oil"、mode=timelinevol、timespan=7d 取数
- **THEN** 返回逐小时（或逐日）热度点序列，带时间戳与值，最新点接近当前时刻

#### Scenario: 非法参数拒绝

- **WHEN** mode=artlist（不在枚举）或 query 超长/含控制字符
- **THEN** 返回 INVALID_ARGUMENT，不发起上游请求

#### Scenario: 空结果如实返回

- **WHEN** query 命中零覆盖（极冷门关键词）
- **THEN** 返回空数据点序列与零结果标注，不编造非零热度

### Requirement: 统一错误语义

所有源 SHALL 使用统一错误码：INVALID_ARGUMENT（参数非法，不发起网络请求）、SOURCE_UNAVAILABLE（网络/超时/预算超限/配置缺失）、SCHEMA_CHANGED（上游结构漂移）；三类错误 SHALL 可被调用方区分；SCHEMA_CHANGED MUST 驱逐对应缓存条目。

#### Scenario: 三类错误可区分
- **WHEN** 分别触发参数非法、上游不可达、结构漂移
- **THEN** 返回的错误中三类错误码可被程序化区分，且常规无数据（合法查询无观测值）不与错误混淆

### Requirement: 网络与缓存约束

取数 SHALL 仅访问各源核定的官方 HTTPS host 白名单，不提供任意 URL 参数；每次请求 SHALL 受时长与响应大小预算约束；成功响应 SHALL 进内存 TTL 缓存且错误响应不缓存、缓存命中 MUST 保留原响应的 retrieved_at 等元数据；HTTP 200 但内容非预期格式（如 HTML 冒充 CSV）SHALL 拒绝且不进缓存。

#### Scenario: 响应超预算
- **WHEN** 响应体超过大小预算或总时长超预算
- **THEN** 终止请求并返回 SOURCE_UNAVAILABLE

#### Scenario: 缓存命中保留原元数据
- **WHEN** 同参数在 TTL 内二次取数
- **THEN** 返回缓存结果且 retrieved_at 仍为首次抓取时刻

#### Scenario: HTML 冒充 CSV 被拒
- **WHEN** 上游返回 HTTP 200 但内容为 HTML
- **THEN** 拒绝解析、不进缓存，下次请求重新抓取

### Requirement: probe 端点

系统 SHALL 提供 `POST /api/datasources/{code}/probe`，以各源预定义的最小参数集执行一次真实取数，返回结果摘要、耗时与（失败时）错误详情；probe SHALL 用于验证连通性与 key 配置，结果不改变源状态（状态由配置与注册决定）。

#### Scenario: probe 成功
- **WHEN** 对匿名源执行 probe
- **THEN** 返回 200 与结果摘要、耗时、retrieved_at

#### Scenario: probe 失败带原因
- **WHEN** 对 key 未配置的源或上游故障执行 probe
- **THEN** 返回失败与具体错误码及原因（配置缺失/网络/结构漂移可区分）

### Requirement: 工具适配不改现有工具面

七源 SHALL 经Registry.Register动态注册。唯一新授权为board-signal-reports用户显式启动的研究loop，allowedTools仅eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade/yahoo_chart/fred_series/gdelt_doc；其最多40轮/40次执行预算由报告域控制，本地受限calculate不是通用Registry工具。发现阶段 SHALL NOT 取数；既有循环A/B、QA、调查、关系发现等白名单不变。官方host/安全参数封闭不变。

#### Scenario: 研究显式授权

- **WHEN** 用户点击一条候选的深入分析
- **THEN** 新研究loop获得七源工具，不获得web_search/脚本/任意URL能力

#### Scenario: 注入后现有工具面不变

- **WHEN** 任意既有增强/QA/调查/问答发现/关系发现运行，或仅执行新信号发现
- **THEN** 既有工具面保持原样，新发现不调用数据源；仅注册工具不等于默认授权
### Requirement: 取数结果不落库

取数结果 SHALL 仅存在于内存 TTL 缓存，不写入业务表；观测值的快照留存 SHALL 由消费方（后续研究会话的工具结果记录）承担，本域不提供历史时序存储。

#### Scenario: 取数后无业务表写入
- **WHEN** 任一源成功取数
- **THEN** 数据库中除数据源目录表外无观测值行写入

### Requirement: 显式历史窗口与原参数兼容

EIA新增可选weeks整数1～12；未传保持当周+上周。JODI新增可选years整数1～5，**仅显式传入**才返回最近N个日历年的可得月份序列，与现有month互斥。不传years时SHALL保留month历史单月查询和无month取最新月（null不回退）的旧行为，不能默认扩大成全年序列。

官方历史HTTPS资源必须核定；不暴露URL/代码参数。缓存键覆盖有效查询参数，成功响应命中保留原retrieved_at/hash/last_modified；错误不缓存、结构漂移驱逐、缺失不补0、不跨单位换算。新增窗口不意味着支持炼厂利润、开工率或价格指标。

#### Scenario: EIA历史窗口

- **WHEN** 显式weeks=8或未传weeks
- **THEN** 分别返回核定官方可得8周窗口或原当周+上周；缺期如实记录，不猜历史URL/列

#### Scenario: JODI原month兼容

- **WHEN** 只传month=YYYY-MM或month/years均未传
- **THEN** 分别保留原历史单月或最新月语义，不扩大原调用者的结果范围

#### Scenario: JODI显式多年

- **WHEN** 显式years=5且无month
- **THEN** 查询最近5日历年可得月份；month与years同传、越界/小数/非法类型均在发网前拒绝

#### Scenario: 回退只限缺省本年

- **WHEN** 多年请求本年404且上年已请求，或显式month对应年404
- **THEN** 前者只回退上年一次且去重，后者直接错误不回退；历史年缺失记gap，不级联向前或用旧期填null

### Requirement: 源能力与host如实声明

源实现实际请求的每个 HTTPS host SHALL 与 catalog HostAllowlist 一致（白名单以源实现为单一事实来源），并由使用真实 fetcher + catalog 白名单的回归测试钉住，测试 SHALL NOT 自带白名单绕过生产校验路径。发现阶段输入与研究阶段工具描述 SHALL 携带各源覆盖、频率、典型滞后、单位口径与各自能力边界——价格类源提供日度价格序列与价差计算原料、事件类源仅提供带时间戳的结构化热度/量（不含新闻正文或观点文本）、量类源（库存/产量/贸易流）不含价格；任何源均不含运价与政策文本原文。实测覆盖限制（如部分 reporter 无数据、symbol 无历史）SHALL 如实标注。需 key 的源在 key 未配置时 SHALL 如实标注不可用并返回显式错误，目录 status SHALL 与取数同源的 live resolver 计算并随 key 配置即时回写，SHALL NOT 用陈旧状态宣称可用性。

#### Scenario: EIA归档host核定

- **WHEN** 显式 weeks=8 请求 EIA 历史窗口且归档版次可得
- **THEN** 归档 host 在白名单内、窗口取回，不因「目标 host 不在核定白名单内」失败；白名单回归测试用真实 fetcher 覆盖归档 URL

#### Scenario: 能力声明进发现与研究

- **WHEN** 信号发现生成 research_question、研究阶段组装工具描述
- **THEN** 两者输入都含各源覆盖/频率/滞后与能力边界（价格源可算价差、事件源只给热度量、量源不含价格），不把无覆盖指标包装成可查，发现阶段自身不取数

#### Scenario: 未配置key源状态如实

- **WHEN** 需 key 的源在 UI 已配置 key 或 key 已移除
- **THEN** 目录 status 由同一 live resolver 即时计算为 enabled/disabled；研究工具描述对不可用源标注原因，调用返回显式错误帧