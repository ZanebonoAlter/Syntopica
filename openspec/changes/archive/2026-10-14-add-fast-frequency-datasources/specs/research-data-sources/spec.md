# research-data-sources Delta

## MODIFIED Requirements

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

### Requirement: 工具适配不改现有工具面

七源 SHALL 经Registry.Register动态注册。唯一新授权为board-signal-reports用户显式启动的研究loop，allowedTools仅eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade/yahoo_chart/fred_series/gdelt_doc；其最多40轮/40次执行预算由报告域控制，本地受限calculate不是通用Registry工具。发现阶段 SHALL NOT 取数；既有循环A/B、QA、调查、关系发现等白名单不变。官方host/安全参数封闭不变。

#### Scenario: 研究显式授权

- **WHEN** 用户点击一条候选的深入分析
- **THEN** 新研究loop获得七源工具，不获得web_search/脚本/任意URL能力

#### Scenario: 注入后现有工具面不变

- **WHEN** 任意既有增强/QA/调查/问答发现/关系发现运行，或仅执行新信号发现
- **THEN** 既有工具面保持原样，新发现不调用数据源；仅注册工具不等于默认授权

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

## ADDED Requirements

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
