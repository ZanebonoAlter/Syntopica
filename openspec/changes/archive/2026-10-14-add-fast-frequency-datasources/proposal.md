<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: data-enrichment -->

## Why

深入分析（board-signal-reports 研究 loop）的时效天花板由其四源白名单里最快可用的源决定——实测（2026-09-23 board 1974「中东地缘政治与美伊关系」信号研究报告，39 次动作 = JODI×20 主力 + Comtrade×5 + WDI×3 + EIA×1 + calculate×10）：报告生成时 JODI 数据期只到 2026-07，**用户体感"信息差了大概 2 个月"精确成立**，且为结构性产物——中东主题下唯一周度快源 EIA（只覆盖美国石油）帮不上忙，时间下限被 JODI 滞后（1.5-2 月）钉死。报告正文自白三个"查不到"：裂解价差（四源全"量"无"价"）、柴油贸易流（Comtrade 月度滞后 4 月+）、禁令是否落地（白名单刻意排除 web_search，时事失明）。本 change 为研究 loop 补第一批评实可用的快频源（2026-10-02 本机实测匿名/免费接口存活状态），把报告时间下限从"月级"抬到"天级"，并让数据滞后在报告头透明可见。

## What Changes

- **新增三个快频数据源**（进 `backend-go/internal/datasources/` 既有可插拔架构，每源一个取数器，typed error 三态 + host 白名单 + 预算/缓存约束沿用现状）：
  - `yahoo_finance_chart`：日度价格（期货/商品/股指/汇率 OHLC，零 key）——裂解价差等价差类计算直接可算；**非官方接口无 SLA，SCHEMA_CHANGED 语义天然兜底**（2026-10-02 实测匿名可用；同类 stooq 已被 JS 反爬挡死，风险已知）。
  - `fred_series`：FRED 序列读取（免费 key，配置模式同 COMTRADE_API_KEY）——日度 WTI/Brent 现货（DCOILWTICO/DCOILBRENTEU）、SOFR、美债、DXY 等宏观全谱，作价格的正规军与 Yahoo 互为备份。
  - `gdelt_doc`：GDELT DOC API 新闻/事件时间线（小时级分辨率，零 key）——事件层结构化开口：只给带时间戳的量/热度/标题，不给正文观点，不破坏研究 loop"纯数据验证"定位（web_search 仍不进白名单）。
- **data_sources 目录 seed 扩三条**（频度/典型滞后/单位纪律按实测口径），`GET /api/datasources` 与设置页「研究数据源」状态卡数据驱动扩容（FRED key 未配置时显式 disabled，不假装可用）。
- **研究 loop 白名单扩展**：`signalResearchToolNames` 四源 → 七源（spec「工具面不变量」唯一放开口子经 delta 显式修订）；工具描述中的硬限制表述从"无价格/裂解价差/运价/政策数据"更新为如实标注新源能力边界与各自的覆盖限制。
- **既有「数据截至」时效声明机制适配日度源**：board-signal-reports 已有 facts 段「数据截至」强制声明与机械校验（各源最新期注入 prompt、模型原样引用、月粒度比对），但现四源最新期均为月度/年度形态；日度源接入后「数据可用性」行将出现 YYYY-MM-DD 期值，期值提取/比对需支持日粒度（如 yahoo 2026-10-01），否则新源最新期无法被如实声明。
- 不改发现阶段 cutoff 纪律与候选快照不可变约束（研究方法论不动）；不动既有循环 A/B、QA、调查、关系发现的工具面。

## Capabilities

### New Capabilities

（无——三源均落入既有数据源域行为契约）

### Modified Capabilities

- `research-data-sources`：目录 Requirement 从四源扩为七源（新源频度/滞后/单位纪律/key 要求入目录）；「研究 loop 白名单仅四源」Requirement 修订为七源并保持封闭枚举式表述；工具描述硬限制 Requirement（现含"无价格/裂解价差/运价/政策数据"）按新源能力边界改写，覆盖限制如实标注原则不变；host 白名单随源实现单一事实源扩容。
- `board-signal-reports`：研究 loop 工具面锚点同步（四源→七源）；既有「数据截至」时效声明 Requirement 适配日度粒度期值（提取/比对支持 YYYY-MM-DD，月度源行为不变）；报告域预算（40 轮/40 次）与 calculate 本地计算不变。

## Impact

- 后端：`backend-go/internal/datasources/`（三源取数器 + catalog 扩容 + seed）、`backend-go/internal/dataenrichment/service/signal_research.go`（白名单常量 + 工具描述组装 + 报告头聚合）、`backend-go/internal/platform/config/config.go`（`FRED_API_KEY`，风格照 `COMTRADE_API_KEY`）。
- 前端：设置页「研究数据源」section 数据驱动扩容（新源状态卡，含 FRED key 管理，模式照 Comtrade）——均在既有页面结构内加内容；信号报告视图 as-of 行零改动（既有机制读已入库 payload）。
- 外部依赖：FRED 免费 key（用户注册一次）；Yahoo/GDELT 匿名零 key。
- 风险：Yahoo 非官方接口无 SLA（stooq 之死为前车之鉴）——FRED 日度现货为正规备份，两者重叠覆盖价格类；GDELT 小时级数据为热度/量而非行情，工具描述须明确防误用。
- 部署后影响：目录/设置页多三张源状态卡；新研究报告（用户显式触发）可调用七源、报告头多一行数据截至；存量报告不动。用户需注册配置 FRED key 后该源才可用，未配置不影响其余六源。
