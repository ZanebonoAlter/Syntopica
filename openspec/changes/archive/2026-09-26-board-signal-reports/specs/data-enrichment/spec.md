## MODIFIED Requirements

### Requirement: 板块 tab 「认知工作台」界面

数据增强默认工作台 SHALL 为候选信号列表与已生成报告，保留month/year周期选择、新闻背景折叠/历史翻阅/叙事内联编辑。顶部「发现信号」只生成候选，每条「深入分析」才研究，已有报告可阅读或显式重新研究。旧简报/调查/legacy产出及绑定管理入口不再挂载，旧数据仅API兼容。本次新报告 SHALL NOT 有评审/采纳按钮、徽标或judge阶段。

候选显示异常、值得查原因、研究问题、新闻依据、发现时间和派生状态；报告显示目标周期/实际生成时间/真实取数次数，历史标事后回顾。详情reader≤760px，精确数据/计算引用、可降级图表、机械附录。提供双主题和loading/empty/error，不将故障冒充无信号。

#### Scenario: 工作台呈现候选与报告

- **WHEN** 用户发现多条信号
- **THEN** 每条分别可深入分析，不自动研究；选一条不触发其余候选

#### Scenario: 周期筛选翻历史

- **WHEN** 在新闻背景选择月粒度并翻到历史period
- **THEN** 展示该period新闻与候选/报告，历史研究明确标事后回顾，不被当前周期覆盖

#### Scenario: 证据链 tooltip 不跳转

- **WHEN** 用户查看新报告新闻依据或数据/计算引用
- **THEN** 原地tooltip/展开显示来源，引用可定位附录；旧news/web/page/lane字段由API原样保留，不承诺已退役legacy UI入口

#### Scenario: 兑现度复盘可见

- **WHEN** 旧客户端读取带hit/part/miss的legacy报告
- **THEN** API仍返回历史字段；新工作台不挂legacy视图，也不为新报告生成兑现评审（本change明确收缩原UI范围）

#### Scenario: 契约为侦探墙铺路

- **WHEN** 客户端消费新候选/报告或旧brief/investigation/review
- **THEN** 新信号、依据ID、观测/计算和条件判断均为结构字段；旧观察/关系/假设结构不变，不依赖长文反解析

#### Scenario: 简报到调查由用户确认

- **WHEN** 新候选发现结束或旧兼容API完成简报
- **THEN** 不自动研究/调查；新候选须用户点击深入分析，旧调查须显式调用，报告后追问本次不做

#### Scenario: 报告阅读无需评审

- **WHEN** 信号报告成功生成
- **THEN** 直接可读，无通过/驳回/采纳/忽略按钮，新闻背景仍可编辑

### Requirement: 仅手动触发（不挂日报管线）

数据增强循环B SHALL 仅由用户触发。新工作台分两次手动操作：发现信号→持久候选→人工选择→深入研究/报告。SHALL NOT 从发现job自动启动研究，不自动调度或挂日报。仅enrichment_enabled=true允许发现/研究；二者与旧board任务共享互斥，等待人工不持锁。

新入口只生成候选或signal_report；旧brief/investigation/topic API保留deprecated原语义，新工作台不调用。报告/候选不得修改daily_report_sections、board_persistent_topics或新闻lifeline；前置freshness仍按原规则工作。研究失败保留候选和补齐结果，须人工重试。新报告不读写review链。

#### Scenario: 仅手动触发

- **WHEN** 用户只点发现而未点深入分析
- **THEN** 最多保存发现批次/候选，取数/计算/成文调用为0；增强关闭时400无任务

#### Scenario: 旧新入口语义隔离

- **WHEN** 调用新research入口与旧brief API
- **THEN** 各自保留新报告与旧brief语义，旧API不悄悄返回signal_report

#### Scenario: 只读不污染主数据

- **WHEN** 候选发现或研究成功/失败
- **THEN** daily_report_sections与board_persistent_topics字段不被这些流程修改，报告不回写lifeline；freshness已完成写入不回滚
