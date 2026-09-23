# 研究数据源流程（Research Data Sources）

<!-- doc-impact-applies: backend-go/internal/datasources/ | section=业务约束与不变量 -->
> 第 10 个业务域（change integrate-research-data-sources 建立）：为研究对话助手供给结构化、可溯源的官方数据取数。互补：[configuration.md](../configuration.md)「研究数据源」节（配置与口径速查）、[api/datasources.md](../api/datasources.md)（端点契约）、[data-enrichment.md](data-enrichment.md)（工具注册表与 agent loop 宿主）。

## 需求说明

用户的原油/贸易/宏观研究此前依赖 dsh 外部工具（tools/energy-mcp，已停止演进）与网页正文抄数。本域把四个官方数据源内置进产品，作为后续「研究对话助手」（规划中）的数据底座：

- **EIA WPSR**：美国周度原油库存（stocks：商业不含 SPR/SPR/含 SPR 总量）与供需（supply：产量/进口/出口），当周+上周。
- **JODI Oil Primary**：96 经济体原油月度产量/进出口/期末库存（含中日韩美）。
- **世行 WDI**：宏观年度指标（出口/GDP、燃料进口占比等）。
- **UN Comtrade**：全球 HS 商品双边贸易（金额+数量，分伙伴国；原油 HS2709）。

配套：数据源目录（表 + 查询 API，为后期 pgvector「研究问题→源匹配」留底）、probe 连通验证、四源适配为 agent 工具（注册进 dataenrichment 工具注册表但**不进任何现有流程的默认工具集**）。

## 链路设计

```
调用方（未来研究会话 agent loop、板块信号研究 loop（board-signal-reports，唯一已授权方）── toolCall ──▶ 四工具 eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade）
                                              （wiring.BuildTools 适配：map 参数→强类型；typed error→错误 JSON；信号研究侧经 dataenrichment cutoff 过滤包装层消费）
                                                      │
                                              sources/{eia,jodi,wdi,comtrade}.go（取数器）
                                                      │
                        datasources.Fetcher（host 白名单/45→120s 时长/32MB 大小/有界重定向/HTML 检测）
                                                      │
                                              datasources.TTLCache（900s，成功才缓存，命中保留原 retrieved_at/sha256）
                                                      │
                                        官方源：ir.eia.gov + www.eia.gov(归档版次) / www.jodidata.org / api.worldbank.org / comtradeapi.un.org(key)
```

- **目录链**：`catalog.go` 常量（静态属性唯一真源，兼派生工具描述与发现/研究能力声明）→ 启动 `Init(db, resolve)` 幂等 upsert seed `data_sources` 表（status 由 **live resolver**〔wiring.ComtradeKeyResolver，UI 优先〕现算，运行时列不动）→ UI 保存 key 后 `UpdateStatus` 即时回写（免重启）→ `GET /api/datasources` 读表返回。
- **probe 链**：`POST /api/datasources/{code}/probe` → wiring 预注册的最小参数集取数 → 返回摘要/耗时/retrieved_at；不改目录状态。
- **错误统一三态**：INVALID_ARGUMENT（本地校验拒，零网络）/ SOURCE_UNAVAILABLE（网络/超时/预算/配置缺失，携 StatusCode）/ SCHEMA_CHANGED（上游结构漂移，驱逐缓存）。

## 业务约束与不变量

- **源清单封闭**：仅目录常量中四源的核定官方 HTTPS host 白名单可访问；白名单以源实现为单一事实来源——源实现请求的每个 HTTPS host MUST 都在 catalog `HostAllowlist`，回归测试 MUST 从 `Catalog()` 派生走真 Fetcher，MUST NOT 手写 host 列表或测试自带白名单绕过生产闸门（EIA 归档 host 曾漏配致 weeks>2 生产必坏而单测全绿，2026-09-23 实战校准）；工具与 API 永不暴露 URL/路径/代码执行参数。
- **能力与覆盖必须如实声明**：四源 coverage/频率/典型滞后/单位口径与硬限制（无价格、裂解价差、运价、政策数据）MUST 注入发现阶段（detect prompt）与研究阶段（工具描述），实测覆盖限制（如 Comtrade 订阅档部分 reporter 无数据、count=0=该期未发布非调用失败）SHALL 如实标注；RequiresKey 源 key 未配置时 MUST 标「当前不可用：未配置 <CONFIG_KEY>」且不得被当成可用能力宣称。
- **口径红线**：EIA 仅美国不得称全球；单位不跨源跨类换算（MMbbl/Mb/d/KBD/KBBL/kg/USD 各守原生）；缺失标记（en-dash 对、`-`/`..`/`x`）一律 null+保留标记，绝不转 0；JODI 最新期缺值不静默回退旧值；本年 404 仅回退上一年一次且注明；Comtrade 月度滞后 4 月+（时效敏感流量用 JODI）、partner_code=0 为 World 合计行。历史窗口仅显式启用（board-signal-reports）：EIA `weeks` 显式整数 1~12 走官方归档版次 CSV（无 key，缓存键 `eia:table1:weeks=N`，不传保持当周+上周旧行为）；JODI `years` 仅显式传入启用最近 N 个日历年（整数 1~5）所有可得月份、与 `month` 互斥，两者都不传保持旧行为（历史年 404 记 gap 不级联，显式 month 404 不回退）。
- **结构漂移零容错**：列漂移/未知标记/冲突重复行显式 SCHEMA_CHANGED（并驱逐缓存），不做模糊容错匹配。
- **缓存纪律**：仅成功响应进缓存（错误不缓存、HTML 冒充不缓存）；命中保留原 retrieved_at/sha256/last_modified，绝不伪造更新时间。
- **取数不落库**：观测值只存在于内存 TTL 缓存；业务表只有目录表，快照留存归消费方。
- **工具面不变量**：四工具经 `Registry.Register` 动态注册，不加入任何现有流程（增强/QA/调查/关系发现）的 allowedTools——现有 agent 工具面零变化；**唯一例外（board-signal-reports）**：板块信号研究 loop（`dataenrichment/service/signal_research.go`）显式授权四源白名单 + `calculate` 动作（经 cutoff 过滤包装层消费，`filter_meta` 入账本），旧流程工具面仍零变化（`explorationToolNames` 逐字不变）；研究会话 agent loop 属后续 change。
- **key 语义（bocha 同款动态链）**：优先级 界面 DB（ai_settings.comtrade_config，动态免重启）> env COMTRADE_API_KEY > config.yaml；取数/probe 每次现读、目录 status 由同一 live resolver 计算（启动 seed 与保存回写共用 `StatusFor`，不再出现「界面已配 key 而 status 恒 disabled」的陈旧行），设置 API 脱敏回显、空 key 不清已存值、enabled=false 跳过界面值；全未配置 → 目录 disabled + 「配置缺失」型 SOURCE_UNAVAILABLE（消息含配置项名），不假装网络故障；其余三源匿名可用互不影响。

## 代码入口

- `backend-go/internal/datasources/`：`catalog.go`（目录常量+host 白名单单一事实来源，回归测试 `catalog_allowlist_test.go` 从 `Catalog()` 派生）、`errors.go`（三态 typed error）、`fetch.go`（预算 HTTP 层）、`cache.go`（TTL 缓存）、`model.go`+`repository.go`（表+seed+`StatusFor`/`UpdateStatus`）、`handler.go`（目录/probe 端点）、`init.go`（启动 seed，接收 live resolver）。
- `backend-go/internal/datasources/sources/`：`eia.go`/`jodi.go`/`wdi.go`/`comtrade.go`（四源取数器，口径翻译自 tools/energy-mcp 100 测试规格书；board-signal-reports 扩展：EIA `weeks` 归档版次窗口、JODI `years` 显式多年且与 month 互斥）。
- `backend-go/internal/datasources/wiring/`：`register.go`（栈装配+probe 注册+路由+不可用标注挂点）、`tools.go`（agent 工具适配 `ResearchTools`/`BuildTools` + `appendCatalogMetadata` 目录元数据注入）、`capability.go`（`SourceCapabilityText` 发现侧能力声明块，`Catalog()` 派生）。
- 设置面：`internal/admin/handler/comtrade_settings_handler.go`（GET/POST `/api/settings/comtrade` 脱敏/空串保留）+ `internal/platform/aisettings/config_store.go`（comtrade_config）+ `wiring.ComtradeKeyResolver`（动态链）。
- 装配点：`cmd/server/main.go`（`datasources.Init` seed）、`internal/app/router.go`（`wiring.RegisterRoutes`）、`internal/dataenrichment/wire.go`（`toolRegistry.Register(wiring.BuildTools(...)...)`）。
- 前端：设置页「研究数据源」section（`features/settings/components/SettingsSectionDatasources.vue`：四源状态卡+测试连通+Comtrade key 管理；composable `useDatasources.ts`、API `api/datasources.ts`）；研究对话页属后续 change。

## 变更溯源

| 日期 | 变更 | 摘要 | 归档位置 |
|------|------|------|----------|
| 2026-09-19 | integrate-research-data-sources | 建域：四源取数器（EIA/JODI 口径从 energy-mcp Go 化 + WDI/Comtrade 新接）、预算 HTTP 层+TTL 缓存、data_sources 目录表+seed、目录/probe API、agent 工具适配注入（现有工具面不变）、COMTRADE_API_KEY 配置 | [`openspec/changes/archive/2026-09-19-integrate-research-data-sources`](../../../openspec/changes/archive/2026-09-19-integrate-research-data-sources) |
| 2026-09-22 | board-signal-reports | 研究授权首例：板块信号研究 loop 显式授权四源（唯一例外，经 cutoff 过滤包装层消费，旧流程工具面零变化）+ EIA `weeks` 1~12 官方归档版次 CSV 历史窗口（无 key、缓存键参数化）+ JODI `years` 1~5 显式多年（与 month 互斥、本年 404 回退一次去重、历史年记 gap） | 实现完成待归档（归档后按 §12 补链接） |
