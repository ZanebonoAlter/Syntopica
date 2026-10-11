## Context

诊断实证与接口实测见 proposal + 本 change explore-findings（已 pin）。核心事实：signal_research 白名单四源里三源月度级以上滞后（JODI 1.5-2 月/Comtrade 4 月+/WDI 年度），唯一周度源 EIA 只覆盖美国石油，报告时间下限被主题决定（中东 → JODI 7 月）；报告自白三缺口（价/快频贸易流/事件）中价格与事件两类本 change 补，快频贸易流（中国海关）留第二批。

既有基建（integrate-research-data-sources + board-signal-reports）：`internal/datasources/`（sources 每源一文件 + catalog 常量唯一真源 + fetch.go HTTP 预算层 + cache.go TTL 900s + tools.go 适配 `service.Tool` + wiring 注入）；`signalResearchToolNames` 白名单 + `isSignalSourceTool` 硬拦截 + 空手源早停（每源一次）；compose 已有「数据可用性」行机械注入与 `validateSignalFactsAsOf` 校验（token 正则已支持 YYYY-MM-DD）。

## Goals / Non-Goals

**Goals**：三快频源（yahoo_finance 日度价格 / fred 序列 / gdelt_doc 事件热度）进 datasources 域与研究白名单；FRED key 配置链（同 Comtrade）；as-of 声明适配日度期值；设置页目录卡扩容。

**Non-Goals**：中国海关（412 反爬，第二批单独攻）；OPEC/IEA MOMR（PDF/xlsx 解析）；IMF PortWatch（端点待证）；web_search 进研究白名单（纯数据定位不动）；发现阶段取数；其他流程（QA/调查）消费数据源工具；calculate 算子扩容（价差用既有 difference/percent_change 即可算）。

## Decisions

1. **源 code 与工具名**：`yahoo_finance`/`fred`/`gdelt` → 工具 `yahoo_chart`/`fred_series`/`gdelt_doc`（命名风格与 eia_wpsr_table1/un_comtrade_trade 一致）。catalog 常量三源静态属性（覆盖/主题/频度=weekly 聚合呈现为 daily/典型滞后 T+0 或小时级/单位纪律/requires_key）+ HostAllowlist 扩 `query1.finance.yahoo.com`、`api.stlouisfed.org`、`api.gdeltproject.org`；启动 upsert seed 以 code 幂等加三行，`data_sources` 表零结构变更。
2. **Yahoo 参数面封闭**：symbol 封闭枚举（首批 17：能源 CL=F/BZ=F/HO=F/RB=F/NG=F、贵金属 GC=F/SI=F、股指 ^GSPC/^IXIC/^DJI/^VIX、汇率 JPY=X/EUR=X/USDCNY=X、加密 BTC-USD）+ range 枚举（1mo/3mo/6mo/1y，默认 1mo），interval 固定 daily；枚举外 INVALID_ARGUMENT 发网前拒绝。解析：`chart.result[0].timestamp[]` 与 `indicators.quote[0]` 对位取 OHLC，长度不齐/null close → SCHEMA_CHANGED/null 如实；货币取 meta.currency 原样透传。枚举清单维护在 catalog 常量（spec 锚封闭原则与首批清单，扩枚举走小 delta）。
3. **FRED 参数面**：series_id 格式校验 `^[A-Z0-9_]{1,64}$`（不枚举——序列空间 80 万，格式即边界）；observation_start/end 可选、窗口 ≤10 年、默认最近 1 年；file_type=json、api_key 走 resolver。key resolver 链 `ai_settings.fred_config`（DB>enabled）> `FRED_API_KEY` > `config.yaml fred.api_key`，风格逐条照 Comtrade（aisettings/config_store 加 `fred_config`，handler 加 `POST/GET /api/settings/fred` 脱敏回显/空串保留）。units/frequency 字段原样透传，`.` 缺失 → null。
4. **GDELT 参数面**：mode 枚举（timelinevol/timelinetone）、timespan 枚举（7d/30d/90d，默认 7d）、query ≤120 字符且拒绝控制字符；返回数据点序列带 ISO 时间戳，零结果空序列如实。工具 Description 明写「结构化热度走势，非事实核查、非新闻正文」，防研究 loop 误用为事件验证。
5. **白名单扩展**：`signalResearchToolNames` 追加三工具名；`CheckCall` 拦截反馈文案从硬编码「只能调用四源工具」改为按白名单动态生成（防下次扩源再漂移）；空手源早停 per-source 逻辑不变（源数 4→7 仅终局判断条件随集合长度）。
6. **as-of 日度适配（最小改动）**：「数据可用性」行各源最新期计算处（compose 机械注入），月度源保持 YYYY-MM，日度源输出 YYYY-MM-DD（取该源观测最大 date）；`validateSignalFactsAsOf` 比对逻辑：数据可用性行含日度期值时按日粒度精确匹配，纯月度时保持现状月粒度（token 正则无需动，已支持两种形态）。阅读视图 as-of 行读 appendix 最大期，零改动。
7. **HTTP 预算与缓存沿用**：三源走既有 fetch.go（host 白名单/45s/32MB）+ cache.go TTL 900s（价格日度数据日内不变，TTL 合理；GDELT 小时级数据 TTL 900s 足够新鲜）；错误不缓存、SCHEMA_CHANGED 驱逐，语义不变。
8. **probe 扩源**：`POST /api/datasources/{code}/probe` 对三新源自动可用（每源取一次最小参数摘要+耗时）；yahoo/gdelt 匿名 smoke 进 `DATASOURCES_LIVE=1`，fred smoke 仅 key 已配置时执行（照 Comtrade 模式）。
9. **测试资产反查**：本 change 改两 capability 契约（MODIFIED），实现前跑 `bash scripts/harness/test-assets.sh research-data-sources` 与 `board-signal-reports`，旧断言「四源/仅四源/无价格」的测试按 test-cases.md「继承与调整」表逐行处置。

## Risks / Trade-offs

- **Yahoo 非官方接口无 SLA**（stooq 2026-10 已被 JS 反爬干掉为实证）→ SCHEMA_CHANGED 显式失败 + 目录 status 如实 + FRED 日度现货（DCOILWTICO/DCOILBRENTEU 等）作价格类正规备份；两者重叠覆盖，单源死亡不致能力归零。
- **GDELT 被误用为事实核查** → spec SHALL NOT 锚 + 工具描述显式声明；风险剩余：模型仍可能拿热度推断事实——由 compose 校验（观测引用必须 `[[data:]]` 可解析）与 gap 披露纪律兜底。
- **symbol/枚举封闭的维护成本** → 封闭是「安全参数封闭」既有纪律的延续；研究场景新 symbol 需求出现时扩枚举是小 delta，可接受。
- **日度期值 × cutoff 过滤交互**：包装层「期结束 ≤ cutoff」对日度观测天然成立（收盘数据时点明确）；盘中调用取到的是最近收盘点，无未来数据泄漏。

## Migration Plan

无数据库结构迁移（`data_sources` 启动 upsert 自动加三行）；部署后：设置页出现三张新卡（yahoo/gdelt 即 enabled，fred 无 key disabled）；用户注册 FRED 免费 key 并配置后 fred 翻 enabled（stlouisfed.org 注册，1 分钟）；存量报告/候选零影响；新研究报告自动获得七源与日度 as-of。

## Open Questions

- `query2.finance.yahoo.com` 备用 host 是否进白名单——实现期 smoke 观察 query1 稳定性再定（不阻塞）。
- GDELT timespan=7d 返回小时点、30d/90d 返回日点（文档如此，实测校准工具描述如实写）——smoke 确认，不阻塞。
- FRED 默认窗口 1 年是否够研究用（JODI years=5 对照）——首版 1 年，不足时显式传 start/end，不阻塞。
