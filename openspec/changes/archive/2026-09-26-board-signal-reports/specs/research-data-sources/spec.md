## ADDED Requirements

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

源实现实际请求的每个 HTTPS host SHALL 与 catalog HostAllowlist 一致（白名单以源实现为单一事实来源），并由使用真实 fetcher + catalog 白名单的回归测试钉住，测试 SHALL NOT 自带白名单绕过生产校验路径。发现阶段输入与研究阶段工具描述 SHALL 携带四源覆盖、频率、典型滞后、单位口径与硬限制（无价格/裂解价差/运价/政策数据），实测覆盖限制（如部分 reporter 无数据）SHALL 如实标注。需 key 的源在 key 未配置时 SHALL 如实标注不可用并返回显式错误，目录 status SHALL 与取数同源的 live resolver 计算并随 key 配置即时回写，SHALL NOT 用陈旧状态宣称可用性。

#### Scenario: EIA归档host核定

- **WHEN** 显式 weeks=8 请求 EIA 历史窗口且归档版次可得
- **THEN** 归档 host 在白名单内、窗口取回，不因「目标 host 不在核定白名单内」失败；白名单回归测试用真实 fetcher 覆盖归档 URL

#### Scenario: 能力声明进发现与研究

- **WHEN** 信号发现生成 research_question、研究阶段组装工具描述
- **THEN** 两者输入都含四源覆盖/频率/滞后与硬限制，不把无覆盖指标包装成可查，发现阶段自身不取数

#### Scenario: 未配置key源状态如实

- **WHEN** 需 key 的源在 UI 已配置 key 或 key 已移除
- **THEN** 目录 status 由同一 live resolver 即时计算为 enabled/disabled；研究工具描述对不可用源标注原因，调用返回显式错误帧

## MODIFIED Requirements

### Requirement: 工具适配不改现有工具面

四源 SHALL 经Registry.Register动态注册。唯一新授权为board-signal-reports用户显式启动的研究loop，allowedTools仅eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade；其最多40轮/40次执行预算由报告域控制，本地受限calculate不是通用Registry工具。发现阶段 SHALL NOT 取数；既有循环A/B、QA、调查、关系发现等白名单不变。官方host/安全参数封闭不变。

#### Scenario: 研究显式授权

- **WHEN** 用户点击一条候选的深入分析
- **THEN** 新研究loop获得四源工具，不获得web_search/脚本/任意URL能力

#### Scenario: 注入后现有工具面不变

- **WHEN** 任意既有增强/QA/调查/问答发现/关系发现运行，或仅执行新信号发现
- **THEN** 既有工具面保持原样，新发现不调用数据源；仅注册工具不等于默认授权
