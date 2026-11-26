
## 时效诊断与新源实测（add-fast-frequency-datasources）

## 诊断实证（2026-10-02 库内取证）

board 1974「中东地缘政治与美伊关系」信号研究报告（topic_enrichment_result id=19，2026-09-23 生成，result_kind=signal_report）：
- 39 次动作 = jodi_oil_primary×20（主力）+ un_comtrade_trade×5（月度只敢查 202601）+ wb_wdi×3（只到 2024）+ eia_wpsr_table1×1（唯一周度源，但只覆盖美国石油，中东主题帮不上）+ calculate×10
- JODI 数据期只到 ~2026-07 → "信息差 2 个月"精确成立 = JODI 固有滞后 1.5-2 月
- 报告正文自白三缺口："柴油贸易流、裂解价差、禁令是否落地——四个数据源全部查不到直接数据"（价/快频贸易流/事件三类结构性缺位）
- 正文无任何"数据截至"披露（appendix 有 per-call last_modified，未聚合上浮）

## 消费链事实

- 四源唯一消费方 = signal_research（backend-go/internal/dataenrichment/service/signal_research.go:52 `signalResearchToolNames`，含硬拦截 isSignalSourceTool）；explorationToolNames 六件套（list_boards/list_lanes/get_lane_detail/web_search/fetch_page/search_internal_context，orchestrator.go:1588）不含数据源工具
- wire.go:101 注释"NOT part of any allowedTools"是 2026-09-19 integrate-research-data-sources 归档时点快照——9-26 board-signal-reports 才挂上消费链，探索时勿再被该注释误导

## 新源实测（2026-10-02 本机 curl，树莓派）

- ✅ Yahoo chart API 匿名活：query1.finance.yahoo.com/v8/finance/chart/CL=F（日度 OHLC，regularMarketTime 最新）
- ✅ GDELT DOC API 匿名活：api.gdeltproject.org/api/v2/doc/doc（query=iran oil&mode=timelinevol，小时级分辨率）
- ✅ FRED 端点活，标准报错"api_key is not set"→ 免费_key 模式同 Comtrade
- ❌ stooq CSV 已死（JS 反爬挑战页）——非官方接口无 SLA 的实证，Yahoo 需按同风险定位、FRED 作正规备份
- ❌ 中国海关 stats.customs.gov.cn 返回 412（反爬），月度 13 天时效价值极高但形态要专门攻 → 本 change Non-goal（第二批）
- ⚠ IMF PortWatch 主页 SPA 抓不动，API 端点待确认

## Spec 锚点（delta 必改点）

- openspec/specs/research-data-sources/spec.md:15 目录四源清单；:131 "allowedTools 仅 eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade"封闭枚举；:179 工具描述硬限制明文含"无价格/裂解价差/运价/政策数据"——加价格源直接推翻此句，三处都要 delta
- openspec/specs/board-signal-reports/spec.md 同含四源白名单锚点 + 报告结构（报告头"数据截至"聚合披露为新增 Requirement）

## 后续阶段硬约束（已由 JIT 注入提醒）

- complexity: complex → 必须有 test-cases.md（test-design ⑤a），主链路表串节拍 + 五问句 + 验收措辞三形态；test-cases.md 的 ⓪ 问要求跑 scripts/harness/test-assets.sh 反查旧测试资产（research-data-sources / board-signal-reports 两 capability 均改契约 M/R，旧断言"四源/仅四源"的测试会命中）
- FRED key 配置模式照 COMTRADE_API_KEY（config.go mapstructure + env 覆盖，缺 key → 源 disabled + probe 报配置缺失）

<!-- pinned 2026-10-02T07:31:25Z -->

## GDELT 源失败根因：匿名 API 限流

2026-10-14 研究任务（board_signal_research_progress id=74，candidate 9）9 次源调用：jodi×3/fred×2/yahoo×2 全成功，gdelt_doc×2 全失败（query "Bab el-Mandeb" 90d、"Houthi" 30d），错误均为「SOURCE_UNAVAILABLE 网络请求失败」，gaps 按设计如实记缺口不阻断。

根因：GDELT 匿名 DOC API 限流。①官方 429 文案 one every 5 seconds；②2026-10-14 复现：直连/走代理均 429，等 12s、6s 后仍 429——实际限流窗口远超 5s 口径，出口 IP 进了更长惩罚桶（GDELT 收紧或共享 IP 连坐）；③被限请求挂 12-14s 才回 429；④当时错误无状态码 → 走的是 fetch.go:124 client.Do 传输层 error（连 429 都没收到，疑似 Clash 代理 127.0.0.1:7890 掐断挂起连接或 ctx 取消；若收到 429 会走 fetch.go:170 UnavailableStatus 带状态码）。

代码事实：fetch.go Get() 非 200 → UnavailableStatus(带 StatusCode)；传输 error → Unavailable("网络请求失败") 丢弃底层 err.Error()。gdelt.go 用 url.QueryEscape(query) 无编码问题。signal_research.go 无 gdelt 节流；工具调用无独立超时（fetch 预算 120s）。live_smoke_test.go:102 注释已知 429 限流，smoke 遇 429 skip。

影响：七源中仅 gdelt 持续不可用，事件热度类缺口（本 change 三目标之一），gap 纪律兜底不阻断。潜在改进：fetch 层 429 识别+退避、错误带上底层 err.Error()、gdelt 进程级节流（≥5s/次）、工具描述标注限流特征。

<!-- pinned 2026-10-14T12:10:59Z -->
