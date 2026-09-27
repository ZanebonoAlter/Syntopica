# Test Cases — integrate-research-data-sources

> 复杂档白盒用例文档（test-design.md 五问句口径）。主链路故事串联 spec Scenario；变体走查五组；白盒附加=分支表+边界值。断言判据以 spec `research-data-sources` 锚定。

## ⓪ 继承与调整（改契约了吗）

新 capability `research-data-sources`，无旧 spec 资产 → 反查 `scripts/harness/test-assets.sh research-data-sources` 无旧测试需处置。EIA/JODI 口径继承自 `tools/energy-mcp`（Python，非主 spec 资产），翻译时以 energy-mcp 测试清单为规格书逐条对齐（见白盒附加 E/J 表）。

## ① 主链路（节拍全吗：每步有落点，SHALL NOT 有负向节拍）

故事：研究者查目录 → 对源 probe → 各源取数语义成立 → 注入后现有工具面不变。

| # | 步/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| M1 | 启动 seed 四源后 GET /api/datasources | 目录查询返回全部已注册源 | 4 行，字段齐（code/名称/覆盖/主题/频度/典型滞后/单位纪律/requires_key/status） | handler | datasources handler 测试 |
| M2 | key 缺失时 GET /api/datasources | 未配置 key 的源显式禁用 | un_comtrade 行 disabled+原因，其余三源 enabled | handler | 同上 |
| M3 | POST /api/datasources/eia_wpsr/probe | probe 成功 | 200+摘要+耗时+retrieved_at | handler | handler 测试（mock fetcher） |
| M4 | probe key 缺失源 | probe 失败带原因 | 502+error_code=SOURCE_UNAVAILABLE+消息含配置项名 | handler | 同上 |
| M5 | EIA stocks 取数 | stocks 分区返回三类库存 | 商业(不含SPR)/SPR/含SPR 总量三行，单位 MMbbl，当周+上周+周结束日期 | 函数单测 | eia_test.go |
| M6 | EIA 上游列漂移 | 表结构漂移显式失败 | SCHEMA_CHANGED+缓存驱逐 | 函数单测 | eia_test.go |
| M7 | JODI 指定月取数 | 指定月份取数带元数据 | 观测值/null+TIME_PERIOD+retrieved_at | 函数单测 | jodi_test.go |
| M8 | JODI 缺省月最新期缺值 | 最新期缺值不回退 | 返回最新期 null+缺失标记，period=最新期 | 函数单测 | jodi_test.go |
| M9 | JODI 本年 404 | 本年 404 回退一次 | 注明 year_used+策略；二次 404→SOURCE_UNAVAILABLE | 函数单测 | jodi_test.go |
| M10 | WDI 多国取数 | 多国多指标查询 | 各国各年 value/null+lastupdated | 函数单测 | wdi_test.go |
| M11 | WDI 非法参数 | 非法参数拒绝 | INVALID_ARGUMENT 且零网络调用（mock 断言未达） | 函数单测 | wdi_test.go |
| M12 | Comtrade 双边取数 | 双边贸易查询 | 数量+金额+单位+期间+isAggregate | 函数单测 | comtrade_test.go |
| M13 | Comtrade key 缺失 | key 缺失时显式报配置缺失 | SOURCE_UNAVAILABLE+消息含 COMTRADE_API_KEY | 函数单测 | comtrade_test.go |
| M14 | 三类错误触发 | 三类错误可区分 | 三错误码程序化可区分；合法无数据≠错误 | 函数单测 | 各源+errors 测试 |
| M15 | 响应超预算 | 响应超预算 | 流式终止+SOURCE_UNAVAILABLE | 函数单测 | fetch_test.go |
| M16 | TTL 内二次取数 | 缓存命中保留原元数据 | retrieved_at 不变 | 函数单测 | cache_test.go |
| M17 | HTML 200 冒充 | HTML 冒充 CSV 被拒 | 拒绝+不进缓存+下次重抓 | 函数单测 | fetch_test.go |
| M18 | 装配注入后跑既有增强/QA 测试 | 注入后现有工具面不变 | allowedTools 清单不含数据源工具；dataenrichment 既有测试全绿 | 函数单测 | dataenrichment/service 新增断言用例 |
| M19 | 重复启动 seed | （design 决策5 幂等） | upsert 不重复插入，静态属性快照更新 | repository | datasources repo 测试 |

负向节拍（SHALL NOT）落点：M6 不模糊容错（断言错误码而非猜测值）；M8 不静默回退（断言 period）；M11 不发请求（mock 计数）；M17 不进缓存（二次请求 mock 计数=2）；M18 工具面不含（清单断言）；取数不落库（M5 后 DB 无观测值行——repo 测试断言仅目录表有行）。

## ② 变体走查（五组，不适用划除留痕）

- **输入**：空串/纯空白（全角·tab）/非法枚举（section=xyz、flow=production、ISO3=XX、HS=abc）/超长 period/大小写（Wdi 指标码大小写敏感否→按世行 API 文档大小写敏感，本地校验统一拒绝小写）→ 全 INVALID_ARGUMENT 分支，各源参数校验测试。
- **前置**：key 未配置（Comtrade disabled+报配置名）；上游 404/超时/HTML；重复维度行（JODI 同键多行值冲突→SCHEMA_CHANGED，值相同→合并）；空集（查询返回 count=0 合法无数据，非错误）。→ 各源+目录测试。
- **时间窗口**：TTL 恰好 900s（过期重取）/900s-1s（命中）；JODI 月份跨年边界（2026-01 本年文件 404→回退 2025 一次）；EIA 表头日期列取最右两个（周覆盖同一 URL，旧列累计）。→ cache/jodi/eia 测试。
- **幂等**：upsert seed 重复执行；probe 不改目录状态（probe 后 status/last_error 不变）；缓存命中重复读。→ repo/handler 测试。
- **并发**：~~不适用划除~~——design 未承诺并发安全（单用户单实例），缓存不做单飞。
- **可用性（UI 前三）**：~~不适用划除~~——ui-impact: none，无 UI。

## ③ 层选对了吗

纯解析/缓存/预算→函数单测（httptest mock 上游）；目录/probe→handler 测试；seed/表→repository 测试（PG，禁 SQLite，按 backend testing 红线）；工具注入不变→service 单测。~~opencli 端到端~~不适用（无前端交互；真联网 smoke 以 `DATASOURCES_LIVE=1` opt-in 单测形态落 task 10.2）。

## ④ 效果核对了吗

不依赖断言外因素：无 LLM 行为、无数据覆盖率依赖（上游数值仅断言结构与元数据，不断言具体数值——数值随期变化）。真联网 smoke 断言「结构+非空+元数据」，量化核对不适用。

## ⑤ 展示字段盘点（改数据结构时）

目录 API 新字段（code/名称/覆盖/主题/频度/典型滞后/单位纪律/requires_key/status）全部在 spec「数据源目录」Requirement 有锚；probe 响应字段（摘要/耗时/retrieved_at/error_code）在「probe 端点」Requirement 有锚。无隐式契约。

## 白盒附加（分支表+边界值）

### E. EIA 解析（继承 energy-mcp 21 用例口径）

| 分支/边界 | 断言判据 |
| --- | --- |
| CP1252 en-dash 对 "– –"(0x96 20 0x96) | →null+missing_marker，非 0 非乱码 |
| 千分位 "13,862" / 负号 / "0" | 数值剥离正确；0 保留为 0 非 null |
| 日期 M/D/YY（8/28/26） | →周结束日期 2026-08-28 |
| A 区/B 区双表头定位 | 按表头列索引，非行号硬编码 |
| B 区标签 "(1)     Domestic Production" 尾随空格/脚注号 | 剥离后匹配，raw_label 保留 |
| 列数变化/重复列/未知数值标记(NaN/inf) | SCHEMA_CHANGED |
| "Crude Oil" 顶层总量行(711) vs Commercial 行(424) | 三行严格区分，不混用 |
| supply 四行（产量/进口/出口/(9)原油进口子行） | 各值独立+单位 Mb/d |

### J. JODI 解析（继承 31 用例口径）

| 分支/边界 | 断言判据 |
| --- | --- |
| 缺失 '-'/'..'/'x' | null+保留标记，'x' 集中在 KBD×存量组合 |
| unit 规则：流量默认 KBD；库存 KBBL；库存+KBD 拒绝；跨单位不换算 | 默认值正确+拒绝组合 INVALID_ARGUMENT |
| CONVBBL 行 | 不当流量值使用（排除或仅元数据） |
| month 缺省：文件内 max(TIME_PERIOD) | 即使 null 不回退；显式 month 404 不回退 |
| 本年 404→上一年一次 | year_used+策略注明；双 404 SOURCE_UNAVAILABLE |
| 同维度重复合并：值同→合并；值冲突→SCHEMA_CHANGED | 两分支各有用例 |
| 非有限数（NaN/inf 字符串） | 拒绝 |

### W. WDI / C. Comtrade（新源）

| 分支/边界 | 断言判据 |
| --- | --- |
| WDI ISO3 多国去重/年份范围倒序 | 规范化后查询；非法本地拒 |
| WDI lastupdated 透传；value=null 保留 | 不删空期 |
| Comtrade partnerCode=0 合计行 isAggregate=true | 合计/伙伴行标记透传 |
| Comtrade count=0（月度未发布） | 合法空数据非错误 |
| Comtrade 429/403 | SOURCE_UNAVAILABLE+限速语义（消息可辨） |
| Comtrade key 空/空白 | disabled+报 COMTRADE_API_KEY |
| M49 码校验（中日韩 156/392/410、沙特 682） | 非法码 INVALID_ARGUMENT |

### F. HTTP 预算 / K. 缓存

| 分支/边界 | 断言判据 |
| --- | --- |
| host 白名单：精确匹配；大小写；带端口；子域不自动放行 | 拒绝分支全覆盖 |
| 重定向：同 host https OK；跨 host/协议降级/超跳数 | 拒绝 |
| 大小预算：恰好 32MB/超 1 字节 | 边界内通过/超限终止 |
| 时长预算超时 | SOURCE_UNAVAILABLE |
| HTML 检测：CT text/html；内容嗅探 `<html` | 拒绝+不进缓存 |
| TTL：命中/过期/驱逐(SCHEMA_CHANGED、HTML)后重取 | mock 计数断言 |
| 错误响应不缓存 | 同参数二次请求 mock 计数=2 |

## 迭代追加：Comtrade key 界面配置（2026-09-19）

| 用例 | 层 | 断言 |
| --- | --- | --- |
| T-SET-1 设置 roundtrip | handler 单测（SQLite） | 空默认（未配/enabled）→ 存 key → GET 只回 configured+末 4 位（完整 key 不出现在响应）→ 空串保存不清 key → enabled=false 指针关闭且 key 保留 |
| T-RES-1 解析链 | wiring 单测（SQLite） | 无 DB 无 static→""；DB enabled+key→返回 DB 值；DB disabled→跳过；空白 key→视为未配；非 COMTRADE_API_KEY 配置名→"" |
| T-LST-1 目录动态 status | handler 单测（SQLite） | seed 时未配（disabled 快照）→ SetKeyResolver 后 List 现算 enabled（免重启翻转）→ resolver 回 ""→ disabled 且 reason 含 COMTRADE_API_KEY |
| T-E2E-1 生产链路 | live 冒烟（重启后实测） | 设置 API 存 key→目录翻转 enabled→probe 真实取数 46 行→enabled=false→目录回 disabled→重新开启 |
