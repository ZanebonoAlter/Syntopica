# 冷启动观察复审（2026-10-28，只读）

## 用户反馈与设计纠偏

用户明确澄清：“继续”不是批准；pi-web吞掉的实际反馈是日期入口重复（顶部范围、中间独立日期、底部上一有内容日、拟增右侧日期），并遗漏卫生健康无持久化正式泳道时无法选日观察内容的冷启动流程。上一单入口picker原型仍依赖成熟板块数据，不能作为该流程的验收。

本轮只读GET/代码定位，无SQL、生成、激活、AI调用、服务起停、产品改动。后续仍只制作待审原型。

## 真实数据证据

以下为独立诊断会话01a1815a-c965-733f-b236-6e5accfef3bd的GET实测，均HTTP200；不以旧pin中的报告编号代替当前读数，也不据编号差异推断用户执行过重建。

| GET | 结果 |
| --- | --- |
| `/api/semantic-boards`、`/api/semantic-boards/overview` | 卫生健康246030；最新日报2026-10-24，overview anchor_date空、lanes空；全局资料日10/27，不等服务器今天10/28 |
| `/api/semantic-boards/246030/lane-dynamics?days=7/14/30/365`（逐档） | 均has_reports=true、lanes=[]、candidates=1；空泳道响应不带timeline |
| `/api/semantic-boards/246030/topics` | 仅候选1781可见，hit_count=10、consecutive_hits=7、section_count=8、can_activate=true；0 active。该端点隐藏未达门槛candidate，不代表DB仅有1个候选 |
| `/api/semantic-boards/246030/daily-reports?days=60`与365 | 同为12份completed日报，10/07—10/24；最新1238/10-24，最早1119/10-07 |
| `/api/semantic-boards/246030/section-timeline` | 16 sections / 12 dates / 18 relations（11 similarity、7 identity）；当前topic brief可见9个candidate ID，其中1781关联8个section/8日，其余8个各1分区/1日，不能据此推DB总数 |
| `/api/daily-reports/1238`、1228、1208 | 10/24、23、22各1分区/1线索，均报告快照candidate、anchor_hit，summary与相关文章ID存在 |
| `/api/daily-reports/1145`、1119、1120 | 分别10/16（1分区2线索）、10/07（2分区2线索）、10/08（1分区1线索）；抽样均candidate且topicId非空 |
| `/api/daily-reports/topics/1781/lifeline` | 8 section / 10 relation；是关联记录，不是历史hit_count/成长率时间序列 |

12个日期：10/07、08、09、10、11、12、13、14、16、22、23、24。10/07两分区、10/12三分区、10/14两分区，其余各一。

只抽取6/12份完整报告核历史快照，不能外推另6份均candidate或无NULL；section-timeline的persistent_topic是当前属性，section.status是分区生命周期，不是topic_status_at_report。新原型若展示全部历史快照，需只读取齐报告详情。

候选1781关联内容跨疫苗、传染病、热浪、婚育等，说明资格达标不等于主题适合作为长线；不能为填空界面推荐直接激活或自动修正归属。

## 产品断链

1. `useConsoleShellData.loadReportMeta`通过overview/report-index兜底获得最新日；`BoardConsoleShell`注入reportAnchor，pool/range正常（365池2025-10-25—2026-10-24，14日可视窗10/11—24）。
2. `useBoardConsole.ts`：has_reports=true且lanes.length=0判no-lanes，candidates不构成正式泳道。
3. `useConsoleSelection.selectedLaneRow`在0行时为null。
4. `laneConsoleView.reconcileSelectedDate`含`if (!lane) return null`，导致selectedDate必为null。
5. `ObservationMatrix.vue`的no-lanes分支不挂MatrixGrid，日期轴消失。
6. `BoardConsoleStage.vue`以`v-if="selectedLaneRow"`挂LaneChronicle；无泳道则读取composable不挂载，报告索引/详情不加载。
7. `useLaneDayReading.ensureDayReading`还要求WindowLaneDay；当前仅LaneChronicle调用，板块日不是独立阅读对象。

因此不是无报告/缺接口/范围错，而是“有泳道→有日期→有阅读”的前置链。现有0泳道测试只覆盖元信息/边界/header跳阅读台，没有覆盖控制台按日观察完整分区。

## 已有能力与缺口

- 已有：日报索引（锚服务器today、需动态lookback）、完整单日报告/threads/summary/历史快照、全板块section-timeline、lifeline、当前管理候选资格。
- 列表日期与365泳道池是不同数据能力：**不能因为所选报日无泳道或超出泳道池，就禁止读取真实已存在的日报**。
- 未有现成“候选成长率/历史命中曲线”，不得虚构。当前资格不能当作历史资格；相似关系不能冒充identity/同一事件发展。
- 冷启动观察可读当日报告的候选分区；不借新观察面曝光完整隐藏候选管理池，不给未达资格项激活入口。
- 当前控制台未有selectedDate独立URL契约，正式实施前须补深链/返回/刷新用例，不能照搬原型storage限制。

## 新原型方向（未批准实现）

- 默认246030卫生健康，成熟1974只作回归对照；不再先给成熟矩阵加补丁。
- 只保留一条板块级时间栏：日期来源是日报索引，所有日切换改同一selectedDate。中间独立档位/起止/刷选下沉“观察设置”，底部不再常驻另一套前后日条，右侧无独立日历。矩阵/演进点击仅作既有快捷。
- 有日报无正式泳道：主舞台是当日全部分区、完整标题、线索/原始摘要按需，不能显示大片无泳道矩阵或把主内容塞右栏。
- 有日报有泳道：保矩阵/演进与同一天阅读，当前泳道缺当天内容显示准确空态，同日其他内容仍可读。sourceDays/A名单、观察范围、selectedDate逻辑保持独立，UI入口收敛不等于混统计。
- 从观察到培育：可只读对照系统关联的历史分区标题/日期，说明同一归属未必同一事件；人工建立泳道/符合资格候选激活只承接既有入口，不执行、不推荐唯一杂糅候选为好泳道、不回填历史快照。
- 其他状态必须有设计答案：无日报无泳道、手动建了泳道但无日报；加载/错误不能误判冷启动。现场12个板块均有日报，后两空数据状态未有真实样本，原型需明确示意/未取样，不能冒充卫生健康现状。

旧两个原型与已投产8B基线保留，不视为本轮方向已批准；产品与数据不动。
