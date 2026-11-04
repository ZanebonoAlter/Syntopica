# 研究依据与代码快照

- 快照日期：2026-10-15。
- 范围：仅本次工作台/呈现/忽略/历史与背景归属；非持续研究引擎设计。
- 用户已认可 `docs/research/data-enrichment-ui/` 的交互骨架、正文 v2 和结构化组件方向；原型示例经过人工重编，不能当真实研究或成文模型验证结果。

## 一、现有代码事实

### 1. 候选内容不可变，没有独立的用户处理状态

`backend-go/internal/dataenrichment/repository/signal_models.go:BoardSignalCandidate` 保存 discovery/board/period、signal、why_it_matters、research_question、evidence_refs、score、rationale。注释明确候选插入后不改内容；研究状态由任务/报告派生。忽略应独立存储，不能改写候选内容或持久化 running 位。

### 2. 研究绑定发现快照，不是一个自由问题接口

`service/signal_research.go:ResearchCandidate`：

```go
candidate, _ := s.store.GetSignalCandidateByID(ctx, candidateID)
discovery, _ := s.store.GetSignalDiscoveryByID(ctx, candidate.DiscoveryID)
json.Unmarshal(discovery.InputSnapshot, &material)
cutoff := discovery.Cutoff
// taskLine: "研究问题: " + candidate.ResearchQuestion
```

实际代码在这些操作前后检查错误、owner 和周期；此摘录仅展示流向。不得因布局、关联话题或翻页重新装配新闻、替换 cutoff、启动发现或研究。

### 3. 图表已经是模型意图 + 程序证据

`service/signal_compose.go:SignalReportChart` 只有 `chart_id/kind/claim/refs`；refs 指向当前账本观测或成功计算。附录由代码填充，模型不写数值和 HTML。当前白名单是七个固定数据源，无通用 web_search/fetch_page；本 change 不扩工具。

### 4. 长小数与图表问题来自呈现路径

`front/app/features/tags/components/signalReport.ts:formatObservationValue` 优先返回 raw_value，否则 String(value)；`resolveCalcRef` 返回 calc.value。`SignalReportView.vue:chartGeometry` 固定320×175视口，按数组索引等距排点，点标签直接使用 display；CSS放大SVG时，长小数和拥挤标签被同步放大。原值保留是正确基础，但不能和阅读短格式共用一个字符串。

### 5. 工作台确有两代对象耦合

`BoardEnrichmentPanel.vue:365-609` 将候选、报告、聚焦分析、新闻背景、金融折叠区纵向挂载；新闻背景与聚焦区共享 selectedTopicId，新闻背景空态要求先去聚焦区选泳道。`TagsPage.vue` 的话题总览宿主实际为 `BoardThreadBrowser.vue`，而非一个假定存在的 TopicDetail 页面。新背景入口应复用该宿主选中话题上下文，并由独立背景组件承载；从研究查看不依赖旧聚焦组件。

### 6. 历史类型与子产物分开保存

`front/app/api/boardEnrichment.ts:listBoardAnalysisResults/getBoardAnalysisResult` 读旧 board_brief/board_investigation/legacy_board_analysis；topic results API 读话题旧分析，debates/qa 归属于结果。新 signal-reports 列表故意不混旧 kind，不能通过放宽新报告过滤来拼历史。历史汇总需要只读 facade 与现有 kind reader，不批量拉每个话题的全部结果造成 N+1。

### 7. 主 specs 有必须显式替换的旧入口约定

- `data-enrichment` 的「板块 tab 『认知工作台』界面」仍写新闻背景折叠区；「报告追问交互」仍写工作台新增追问。
- `board-level-analysis` 的「版块分析工作台信息架构」「lane 引用与单泳道下钻」仍要求聚焦入口。
- `board-signal-reports` 规定 schema_version=2、固定四段和正文全局 as-of 句；新段落/证据块与分源时效必须走 MODIFIED，而非只在组件里暗改。

### 8. 相邻并发工作

`add-topic-key-event-markers` 活跃。`BoardThreadBrowser.vue` 是共享宿主；apply前盘点归属与交点。不得改它的关键事件筛选、lane identity、日报定位或话题生命周期。

## 二、外部资料（访问于 2026-10-15）

| 来源 | 可核查内容 | 本 change 的采纳边界 |
| --- | --- | --- |
| [CFA Financial Analysis Techniques](https://www.cfainstitute.org/insights/professional-learning/refresher-readings/2025/financial-analysis-techniques) | 明确目的、取证/处理/解释、沟通与定期跟进 | 保留研究问题、解释及后续观察；不把取数次数当质量 |
| [Capital Group The Capital System](https://www.capitalgroup.com/individual/about-us/capital-system.html) | 长期视角、多专业协作与多视角 | 不照搬金融指标、持仓或角色数量；本次只治理呈现 |
| [SWP How We Work](https://www.swp-berlin.org/en/swp/about-us/fundamentals/how-we-work) | 长期框架、动态议程、多视角、独立性及同行审查 | 背景与判断分开；不把新闻热度自动等于研究必要性。SWP明确不接受研究委托，不泛称智库都按委托启动 |
| [UK Futures Toolkit](https://www.gov.uk/government/publications/futures-toolkit-for-policy-makers-and-analysts/the-futures-toolkit-html) | 持续扫描弱信号、情景与政策压力测试 | 是政策方法参考，不是所有基金/智库统一SOP；持续研究机制后续另案 |
| [SIPRI Sources and Methods](https://www.sipri.org/databases/armstransfers/sources-and-methods) | 多种公开来源、谨慎估计和新信息更新 | 原值、来源、时效和缺口可核查；不声称当前七源足以研究任意议题 |

## 三、采纳决策表

| 议题 | 采纳 | 不采纳 / 延后 |
| --- | --- | --- |
| 研究定位 | 版块工作台、话题背景、具体问题研究 | 每条泳道强制建研究项目 |
| 生成与呈现 | 结构化文案/证据引用，由组件排版 | 模型写任意HTML/SVG/ECharts配置或复制数值 |
| 新旧内容 | 新报告受控块，旧报告适配并保持原文 | 用正则猜旧文语义或付费重写全部历史 |
| 忽略 | 单候选持久标记、可恢复；研究中拒绝忽略 | 忽略=取消、删除、报告评审或永久屏蔽话题 |
| 历史 | 所有可验证归属的旧报告/辩论/问答只读可达 | 重启旧生成入口、自动回填归属 |
| 周期/材料 | 原cutoff与快照不变 | 切换时偷偷改成最新材料 |
| 持续认知 | 为关联和后续观察保留表达空间 | 本次新增自动假设追踪/同题合并/自由研究 |

## 四、尚须验证而非假装已知

- 实库旧载荷/kind覆盖和可恢复归属比例，需要 apply 时只读抽样；无归属不猜回填，缺失资料明确提示。
- 新成文协议依赖模型实际遵循：mock 测试不等于可用。实施验收须有明确授权的受控样例验证及量化记录；当前没有执行任何产品 LLM 调用。
- 原型新增忽略、背景编辑、全部旧历史状态仍待整体 UI 确认；已认可局部不等于整个 ui-design 自动 approved。
