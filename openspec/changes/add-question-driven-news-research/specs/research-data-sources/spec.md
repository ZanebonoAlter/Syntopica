## MODIFIED Requirements

### Requirement: 工具适配不改现有工具面

七源 SHALL 经Registry.Register动态注册。唯一新授权为board-signal-reports用户显式启动的研究loop，七个数值/事件源仍为eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade/yahoo_chart/fred_series/gdelt_doc；旧候选仍只有这七源，新协议候选另可显式授权read_source/web_search/受限fetch_page，来源安全、时间边界与追加预算由board-signal-reports规定；其最多40轮/40次执行预算由报告域控制，本地受限calculate不是通用Registry工具。发现阶段 SHALL NOT 取数；既有循环A/B、QA、调查、关系发现等白名单不变。官方host/安全参数封闭不变。

#### Scenario: 研究显式授权

- **WHEN** 用户点击一条候选的深入分析
- **THEN** 旧协议获得原七源，新协议另获得已核定的来源检索/阅读；两者均无脚本或任意URL爬行能力

#### Scenario: 注入后现有工具面不变

- **WHEN** 任意既有增强/QA/调查/问答发现/关系发现运行，或仅执行新信号发现
- **THEN** 既有工具面保持原样，新发现不调用数据源；仅注册工具不等于默认授权

#### Scenario: 来源能力不改变数值工具语义
- **WHEN** 新协议读取文字来源或搜索失败
- **THEN** 七源参数、官方host、缓存及cutoff过滤不变，来源ID不得成为calculate输入，不新增数值数据源目录条目、不自动切供应商
