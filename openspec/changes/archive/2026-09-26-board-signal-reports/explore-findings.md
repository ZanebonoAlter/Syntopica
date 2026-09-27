# Explore Findings：板块信号解读报告（board-signal-reports）

> 2026-09-19 三轮讨论共识沉淀（用户主导定调）。本文件是 proposal/specs 的设计输入。

## 一、目标产物：桥水式解读报告（最终共识，最高优先级）

**报告是观点，数据只是观点里引用的证据。** 查数是 agent 做研究时的内部动作——就像人做研究：翻了一堆资料，写出来只有两三句带数字的话。深度来自判断和机制，不来自数据量。

### 样张（用户已认可，作为 spec 故事锚点）

> **炼厂在利润转负后砍开工而非减采购——历史上这是价格见底前的固定动作**
>
> 本周 EIA 数据显示商业原油库存仅降了 **80 万桶**，而炼厂开工率下滑了 **2.1 个百分点**。开工降、库存几乎没降，意味着炼厂一边少买、一边还在消化库存——它们赌的是「先扛过去，等价格更低再补」。
>
> 这个行为模式在过去三轮下行周期里都出现过：利润转负 → 砍开工 → 库存被动去化 → 1~2 个月后价格触底。**如果下周三的开工率数据确认继续下滑，我们把 Q4 油价的基准判断从"震荡"上调为"偏多"。**
>
> 要推翻这个判断，只需要看到一个反例：开工率回升而库存不降（那说明炼厂在正常补货，需求没问题）。
>
> *数据：EIA 周报 2026-09-12（stocks/supply 两表）；全表记录见附录。*

### 报告结构契约

1. **观点开头**：标题即判断（不是「关于 X 的分析」）
2. **基础事实**：数据引用（2~3 个关键数嵌在论证句里）+ 1~3 张小倍图
3. **因果链**：刺激来源 → 传导 → 需求端状态（桥水路径：拆解需求与刺激来源）
4. **后果评估**：明确的方向判断（上调/下调/维持）+ 触发条件 + 自我质疑句
5. **数据附录**：每次取数的源/参数/期/值全记录（可核查，非报告主体）

### 验收锚（硬性，2026-09-20 用户修正）

**「报告里没有前瞻判断 = 不合格」**——这是「作文」和「解读」的分界线。
**「自我质疑」**以桥水语感融入后果段（一两句「我怎么知道自己是对的」），**不设独立证伪/风险块**（用户：纯属多余的程序化表达）。
**篇幅 <3000 字**（桥水 Daily Observations 纪律）；**图表 1~3 张**（小倍图，每图一个论点，数据=取数序列）。
**桥水分析路径**（用户引述）：基础事实出发（先拆解需求与刺激来源，再评估后果），因果链支撑判断，非堆数据。

## 二、管线：信号 → 钻取 → 解读

```
泳道新闻流（一段周期内）
   │
   ▼ ① 信号识别（分析主 prompt 的一部分）
   选择标准：「会不会改变对未来的判断」，不是「有没有数据可查」
   找的是异动：突变、反常、初现、背离（例：多篇报道都提「炼厂开工异常」）
   │
   ▼ ② 数据钻取（agent loop ≤4 轮，agent tool 非 MCP）
   问题驱动：每轮取数对应一个理解问题（「开工降了库存为什么没降？」）
   不是指标清单驱动
   │
   ▼ ③ 解读成文（按上面报告结构契约）
```

## 三、机制决策清单（用户已定）

| 决策 | 定论 |
| --- | --- |
| 调用方式 | agent loop 调 agent tool（四数据源工具已在注册表），不走 MCP |
| 取数上限 | 单次分析 ≤4 轮 |
| board_data_sources | **界面退役 + 代码标 deprecated**（不再做绑定管理，自主调用取代之） |
| 通用性 | 报告形态板块无关；无数据源板块自然降级为纯观点解读（数据附录空）；data_sources 目录可扩展，未来加源机制零改动 |
| 报告挂靠 | 数据增强域新 result_kind，review 流程复用现有 |

## 四、演进轨迹（早期观点被否，防再犯）

- ✗ fact-check 徽章（每条 insight 挂证实/矛盾标）——太浅，被否
- ✗ 数据面板/分位/交叉指标面/历史类比模块——堆数据，被否（「解读不是堆数据」）
- ✓ 保留：查数由理解问题驱动；矛盾的作用是**发现信号**（新闻说法与官方数据对不上=值得钻取的信号），不是报告的产出形态

## 五、已知缺口

- **历史序列深度**：EIA 仅 2 期、JODI 12 期，agent 研究时（如「过去三轮周期都出现过」）需要更长序列。change ② 给工具加历史范围能力（EIA 多周、JODI 多年）。定位：**研究支持**（agent 内部消费），不进报告主体。
- **触发方式**：待定。倾向「周期扫 + 信号门槛」——周期分析时顺带跑信号识别，攒够信号才出报告，没料不出；另留手动触发入口。

## 六、UI 定位决策（2026-09-20 用户定案）

- 数据增强工作台整体换成**信号解读报告列表 + 详情阅读视图**；旧简报/调查/分析页面废弃（后端代码保留 deprecated，触发链摘除旧 kind 生成）。
- 调查未来形态=「针对报告的追问」，形式用户后续敲定，本次不做。
- 原型批准（ui-approval: approved）：纯编辑流备忘录排版（用户二次重制样式）+ 桥水三纪律（<3000 字 / 图表 1~3 张 / 自我质疑融入后果段，无独立证伪块）+ 报告列表形态沿用用户定稿 hairline 语言。

## 七、前置依赖

- integrate-research-data-sources **已归档**（research-data-sources 已进主 specs，2026-09-20 确认），本 change 对其「工具面不变量」requirement 做 MODIFIED（放开：允许分析流程显式加入四工具）无阻塞。

## 泳道周期分析触发链与真实数据画像

## 泳道周期分析现状与真实数据画像（触发机制设计输入，2026-09-19 实测）

### 触发链现状：手动触发，无周期调度
- `POST` 板块分析端点 → `analysis.Start(AnalysisScopeBoard, boardID, AnalysisJobKindBoardBrief, ...)` 异步 job → `orchestrator.EnrichBoard(ctx, boardID)`。
- **没有任何周期调度 job 自动跑分析**。周期补齐靠 freshness_gate（阈值 72h）：分析触发时对 month/year 粒度做 freshness 检查+补齐（granularity 集合={month,year}，week 已排除；LLM summarize 调用上限 40/次）。
- 含义：用户实际节奏 = 手动点「分析」，月级为主。

### DB 实测数据（syntopica-postgres，2026-09-19）
| 指标 | 值 |
| --- | --- |
| board_persistent_topics 总数 | 1240 |
| 有 lifeline 的活跃话题 | 80（distinct persistent_topic_id） |
| topic_lifeline_context 行数 | 267（month 175 / week 12 / year 80） |
| 近 30 天 week 行 | 仅 1（周度基本停摆） |
| 近 30 天 month 行 | 30（月度是主力节奏） |
| articles 总量 / 近 7d / 近 30d | 21414 / 4259 / 16769 |
| 每活跃话题每周文章量 | ~53 篇（4259/80，量大，信号识别不能逐篇读全文） |

### 触发机制设计结论
1. **周期扫挂手动分析触发**：EnrichBoard 在 freshness gate 补齐之后加「信号识别」步骤——对本次分析周期的泳道内容识别信号；达标则追加钻取+报告，不达标则零打扰出常规简报。零新调度、零新成本模型、周期节奏跟随用户真实使用（月级）。
2. **信号质量门槛而非数量门槛**：识别阶段对候选信号打分（异动强度/报道密度/判断价值），≥1 个达标信号才进钻取。0 信号是合法常见结果（大多数周期没有值得报告的异动）。
3. **信号识别输入**：复用泳道周期分析已有输入形态（lifeline 摘要/section 采样），不逐篇读全文（每话题每周 ~53 篇读不起）。
4. 候选联动钩子：board_topic_watches（status active/paused）+ topic_watch_hits 是现成「话题观察」表——观察中的话题可作为信号敏感度加成，一期可不动。

### lifeline 表结构（topic_lifeline_context）
persistent_topic_id / granularity(week|month|year) / content(text) / as_of_date / source(manual等) / period / created_at / updated_at。UNIQUE 未在此核实，更新语义走 freshness gate。

**引用**：backend-go/internal/dataenrichment/handler/board_enrichment_handler.go:52、backend-go/internal/dataenrichment/service/enrich_board.go:90、backend-go/internal/dataenrichment/service/freshness_gate.go:24

<!-- pinned 2026-09-20T02:10:34Z -->

## 信号报告视图的编辑版式契约（原型 token 对齐）

报告视图视觉契约（2026-09-20 原型重制定稿，index.html 已按此实现）：

**token 源**：`front/app/assets/css/main.css` `[data-theme="editorial"]`——bg 三阶 stone（base #faf7f2 / elevated #f5f0e6 / sunken #e8dfd1）、文字 #1a1a1a/#5a5a5a/#8a8a8a、accent 印刷红 #d94a4a（hover #c12f2f、subtle rgba(217,74,74,.08)）、warning #c4883c（subtle rgba(196,136,60,.14)）、shadow-print。实现一律 `var(--color-*)`，原型里的字面量仅为静态拷值。

**复用的编辑签名元素**（源：`front/app/components/article/ArticleContent.css`）：
- kicker：0.72rem/700/0.22em 字距红小标 + 28×1px 细线（`.kicker` 同形态）
- section-sep：34×3px 红短条转场（opacity .8，radius 2px）
- 大标题：`--font-serif-display`（Noto Serif SC 栈，main.css 自托管 300–700）clamp(1.7rem,2.8vw,2.1rem)/1.38，text-wrap:balance
- lede 导语：衬线 + 次要色 + 2px 左线（`.lede` 同形态，report 的 summary 段用它）
- 背景暖米渐变：`linear-gradient(180deg, bg-base, bg-elevated 55%, bg-sunken)`
- 后果评估段（v3 批准版，去容器化）：**纯 3px accent 左线段落块，无底色/圆角/阴影**——block-label「后果评估」+ 衬线判断句（.verdict 1.18rem/700 serif）+ muted 小标「触发条件」「自我质疑」两句；**无独立证伪块**（自我质疑融入后果段，2026-09-20 用户定案）。层级靠排版不靠色块（「报告是观点不是数据面板」）
- 小倍图（v3 批准版新增）：`figure.charts` flexwrap 布局，flex 1 1 240px；SVG 折线（accent 1.8px + 末点强调）/柱状（次要色，末柱 accent 强调）；轴 hairline + 10px muted 轴标；figcaption 0.72rem muted 题注一行；无边框无卡片
- 附录表（v2）：**去外框**——无外边框/圆角，只留行 hairline（td border-subtle / th border-medium），折叠 + 0.82rem 小字号 + muted 色本身已表达「非主体」
- block-label 小标：0.7rem/700/0.18em 字距（`.ai-head` 形态）

**数据引用**：印刷红粗体数字 + 1px dashed rgba(217,74,74,.45) 下划 + hover accent-subtle 底；tooltip 深蓝灰 #243b53 白字；点击锚点跳附录行（`tr:target td` 高亮 accent-subtle + src 列变红 + ↩ 回链）。

**布局**：AppPageShell mode="reader"=760px 居中（`front/app/components/ui/AppPageShell.vue` SHELL_MAX_WIDTH），内层 reading-col 680px；原型页 = 单一展品（exhibit-cap 小节标 + hairline）：报告详情阅读视图样张全文。报告列表（SignalReportList）无独立原型，形态在 ui-design.md Interaction Contract 描述（hairline 分行列表，行首 inset 3px 红线为报告行签名）。已验证：1440/1920/375 三视口零横向溢出，后果评估块 bg=transparent/shadow=none/radius=0，附录折叠与锚点高亮可用。新组件样式作用域 `.signal-report`，不动 `.markdown-body` 基础排版红线。

**引用**：front/app/assets/css/main.css:[data-theme=editorial]、front/app/components/article/ArticleContent.css:.preview-mode、front/app/features/tags/components/BoardEnrichmentPanel.vue:.narrative、front/app/components/ui/AppPageShell.vue:SHELL_MAX_WIDTH、openspec/changes/board-signal-reports/ui-prototype/index.html

<!-- pinned 2026-09-20T03:39:28Z -->

## 审查：周期、零产出与存储评审接点

现有 EnrichBoard(ctx, boardID) 无 granularity/period 入参；ensureLaneFreshness 遍历 month/year 数据周期，assembleSituationCards 使用当前 active lanes/time.Now，不能直接当作历史周期输入。triggerBoardEnrichment 的 job 回调无条件 output.Result.ID，零信号需显式返回成功无 result，避免 nil panic。TopicEnrichmentResult 实际字段为 ResultKind/AnalysisScope/SemanticBoardID/PersistentTopicID/Sectors/InputSnapshot，无 payload、周期、生成状态列，设计需明确映射或迁移。AnalysisStatus 只有运行/完成/error/result_id，无 detect/drill/compose 阶段，且 runner 存内存 map；失败报告行和信号达标占位需补状态契约。judgeBoardBriefAgainstPrev 是取同 kind 前一报告对比，仅 ShouldReview 时新建独立 TopicEnrichmentReview（Applied bool），不是给每篇报告审批通过/驳回；不能只注册 kind 即实现新方案的报告审批状态。

**引用**：backend-go/internal/dataenrichment/service/enrich_board.go:EnrichBoard、backend-go/internal/dataenrichment/service/situation_cards.go:assembleSituationCards、backend-go/internal/dataenrichment/handler/board_enrichment_handler.go:triggerBoardEnrichment、backend-go/internal/dataenrichment/repository/models.go:TopicEnrichmentResult、backend-go/internal/dataenrichment/service/board_brief_review.go:judgeBoardBriefAgainstPrev

<!-- pinned 2026-09-21T11:41:46Z -->

## 样张内容核查：当前四源不支持炼厂利润/开工率论据

当前 EIA eiaStocksRows仅库存三项，eiaSupplyRows仅production/imports/exports；没有炼厂开工率、炼厂利润或价格序列。JODI jodiFlowMap仅production/imports/exports/closing_stocks，product固定CRUDEOIL，没有样张中的refinery flow，不能返回“历史四次行为三次领先价格触底”这类已计算结论。ui-prototype/index.html当前样张的开工率图、JODI历史行为总结只是虚构排版示意，不是可由现有工具实现的报告验收样本。即使扩展weeks/years窗口也不会自动补齐缺失指标。JODI现有工具已有显式month=YYYY-MM历史单月参数；新增years必须明确与month的组合/互斥规则，不能笼统声称现有只支持当年。

**引用**：backend-go/internal/datasources/sources/eia.go:eiaSupplyRows、backend-go/internal/datasources/sources/jodi.go:jodiFlowMap、backend-go/internal/datasources/wiring/tools.go:ResearchTools、openspec/changes/board-signal-reports/ui-prototype/index.html

<!-- pinned 2026-09-21T12:12:57Z -->

## 阶段0裁决：EIA归档CSV路线与阶段1实现要点

Controller 裁决（阶段0验收后）：① EIA 历史窗口走官方归档版次 CSV（URL 模板 https://www.eia.gov/petroleum/supply/weekly/archive/{YYYY}/{YYYY_MM_DD}/csv/table1.csv，无 key，fixture 已存 testdata/eia-wpsr-archive-*.csv + README），禁走 api.eia.gov v2（需 key，违反匿名语义）；版次日映射有假日周四跳变（实证 2026_09_10），不得纯周三步进。② ir.eia.gov/wpsr/table9.csv 含炼厂开工率但首版禁接入（design §5 不扩指标，事实修正仅记录）。③ 阶段1 实现缺口：service/period.go ParsePeriodRange 现用 time.UTC 构造边界——须新写 Asia/Shanghai（models/utils.go ShanghaiTZ）感知 helper + 「未来周期400」校验，不得照抄；EIA 缓存键现为无参常量 eia:table1——weeks 必须入键；postgres_migrations.go:2190 两条具名 CHECK（kind/parent_shape）扩 signal_report 须 DROP+re-ADD（样板 20260828_0001）；analysis_runner job 回调 output.Result.ID 有 nil 风险，discovery 零信号须显式无 result。④ 阶段划分修正：tasks 4.2（result 迁移+DB 约束，signal_report_test.go）划阶段1；阶段2 拆 2a 发现闭环（2.3/2.4）与 2b 研究闭环（3.3/3.4/3.5/4.1/4.3/4.4/4.5）两批派发。

<!-- pinned 2026-09-22T02:11:19Z -->

## 遗留缺陷：研究job句柄丢失不自愈+失败静默回退

UI 真实验收（ui-acceptance/report.md ⑦-P2，2026-09-22 实证）发现两个非阻塞边缘缺陷，建议后续 change 修复：① job_id 句柄丢失场景不自愈——浏览器 tab 崩溃/离开页面丢失轮询句柄后，候选「研究中」徽标僵持 ~50 分钟（job 实际已 20:52 超时 failed，前端无从感知），需手动点「刷新」才回落「待研究」；② 失败静默回退——重拉列表后候选回 pending 但页面无错误条，研究失败原因（context deadline exceeded）只在 job 历史（analysis-status?scope=board&id=）里，前端不展示。修复方向：useSignalWorkbench 挂载/setBoard 时主动查一次 analysis-status?scope=board&id=<boardId>，发现 running 的 signal job 就接管轮询、发现刚终态的 failed job 就短暂展示错误提示。注意与 FE-12（job404 停轮询）区分：这是「job 存活但页面无句柄」的恢复路径，State Matrix 的 research failed 行语义要求显示阶段错误。另：本地 LLM（2-3min/轮）下 40 轮预算与 30 分钟 job 超时结构性不兼容（design§4 明文接受，非缺陷）——真实成功报告验证需云端快模型。

<!-- pinned 2026-09-22T13:28:12Z -->

## 2026-09-23 实战校准：首份真实报告诊断（四组修复的事实依据）

真实样本 `topic_enrichment_result.id=19`（board 1974 · month 2026-09 · 候选4，session board_signal_report_4_3c6c645a，11:56:45→12:47:19 共50分34秒）：generation_meta=`{decisions:40, source_calls:29, calculation_calls:10, stop_reason:finished, cutoff:"2026-09-22T11:38:37Z"}`；progress 行 rounds_done=39（39执行+第40轮 finish）。候选信号问「美国柴油出口禁令/裂解价差/印土替代采购」，报告正文全是沙特科威特 1~7 月原油出口库存——文不对题但未编造（正文自承四源查不到柴油/裂解/禁令数据）。

**四源实测（appendix 29 calls 逐条核过）**：
- Comtrade 5次（c1-c5：US/IN、2710/2709、2024/2025/202601）全部 `count:0,no_data:true`，status=ok。用库里 `ai_settings.key='comtrade_config'`（value列，key len32，2026-09-20 UI配置，enabled）直测：**CN(156) 2710 X 2024=189行、probe 中国2709M2024=46行，但 US(840)/JP(392) 任意期/任意商品/双流向全部 count=0** → 该订阅档对部分 reporter 无覆盖，不是代码错。
- EIA 1次（c13, `{"section":"supply","weeks":8}`）→ `SOURCE_UNAVAILABLE 目标host www.eia.gov 不在核定白名单内`。根因：`datasources/catalog.go:49` eia_wpsr `HostAllowlist:["ir.eia.gov"]`，而 `sources/eia.go:24` `eiaArchiveBase=https://www.eia.gov/petroleum/supply/weekly/archive`，`fetchWindow`→`fetchEditionForWeek` 固定走该 host；归档 URL 直测 200/404（可达），live `ir.eia.gov/wpsr/table1.csv` 302 到同 host `/secure/...`（现行白名单放行，probe rows=3 正常）→ **仅 weeks>2 的归档路径必坏**；eia_test.go 用 httptest 自建 archiveBase+自带 fetcher 绕过生产白名单，故测试全绿。fetch 层闸门 `datasources/fetch.go:161 checkURL→allowedHost`；白名单聚合 `wiring/register.go newStack` 直接吃 Catalog()。
- JODI 20次：全部 `periods 2026-01..2026-07`（cutoff 09-22 → 滞后约1.7月，与源声明 1.5-2月 一致）；**一调用一 geo+flow**，区域共性检验被迫逐国点名（SA/KW/IR/IQ/AE/US/IN/TR/DE/NL/JP/KR…），IR/IQ/AE 部分期带 missing_reason。
- WDI 3次（c6/c7/c25）：年份只到 2024（probe lastupdated=2026-07-13）→ 年度滞后约2年。

**40轮去向**：JODI 20 + Comtrade 5 + WDI 3 + EIA 1 + calculate 10 + finish 1。`CheckFinish` 不拦无配额（signal_research.go:534 直接放行），模型是真花光预算；每轮 LLM 往返 60-75s → 40轮≈50分钟。candidate 6 首次研究 11:07:45→11:51:58（34轮/22取数/12计算）被 11:52:13 SIGTERM 关机杀掉（`workers=0.0s` 不等任务），progress 行 **至今 status=running**（defer 的 WithoutCancel 终态写随进程退出丢失）。

**status 陈旧根因**：`datasources/init.go Init()` 自建 resolver 只读 `config.AppConfig.Comtrade.APIKey`（env/config），**忽略 UI 链** `wiring.ComtradeKeyResolver()`（aisettings `LoadComtradeConfig` 优先）→ 启动种子恒算出 disabled「未配置 COMTRADE_API_KEY」，而取数/probe 实际走 UI key 正常。`datasources/repository.go UpsertCatalog` 注释明言 status 由 resolver 重算（每次启动），修法=main.go:66 传 live resolver + UI保存 key 回写。`Registry.Execute` 对未注册工具返回 `{"error":"未知工具: X","available":[...]}` 错误帧（不 panic）。

**注入点定位**：detect prompt=`signal_detect.go:85 signalDetectSystemPrompt`（今日无任何数据源信息）；research prompt=`signal_research.go assembleSignalResearchPrompt`（收 buildToolsDesc 输出）；toolsDesc=`orchestrator.go:1585 buildToolsDesc`（name+description+schema JSON）；compose=`signal_compose.go assembleSignalComposePrompt`+`signalComposeSystemPrompt`（四段+引用纪律，无时效要求）；早停反馈通道=orchestrator.go runToolLoop 执行后 append 固定 historyLine（无 policy 注入钩子，需新增可选接口 type assertion，同 `toolLoopActionRunner` 先例）；catalog 元数据=`datasources/catalog.go` DataSourceDefinition{Coverage/Frequency/TypicalLag/UnitPolicy/RequiresKey/ConfigKeyName}。

<!-- pinned 2026-09-23 实战校准 -->
