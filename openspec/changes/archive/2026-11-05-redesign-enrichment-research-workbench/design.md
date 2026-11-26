## Context

动机见proposal.md，UI合同已获用户明确批准。本轮仅规划。用户补充要求「别过度设计和测试，精准即可」，因此本设计只保留实现已确认行为所必需的变化，不建立通用文档框架、全局任务中心或新研究引擎。

当前可复用基础：候选/发现快照不可变，研究按candidate_id读取；报告已有四段、charts refs及代码附录；已有候选/报告分页和旧结果/QA/辩论读取接口。现有问题主要是工作台堆叠、状态耦合与呈现粗糙。

## Goals / Non-Goals

**Goals:** 主从工作台；持久忽略；可靠数值/图表；最小成文结构；独立话题背景；全部旧产物只读可达。保持cutoff、研究预算、账本、旧JSON及既有API兼容。

**Non-Goals:** 不做通用AST/插件系统、新图表库、每组件一套状态框架、多board后台任务管理、自由提问、自动持续研究或假设评审；不顺带改全域ID/API风格，不把所有旧报告重写成新格式。

## Decisions

### D1. 忽略只加一张小覆盖表

`board_signal_disposition(candidate_id PK/FK, ignored_at timestamptz NOT NULL)`：有行即忽略，无行正常。归属从候选取得，不重复存user/board/period；重复忽略保留原时间，恢复只删除标记行。候选、报告和快照均不改。迁移用现有 `internal/platform/database/postgres_migrations.go`，编号按apply时序分配。

忽略写入与研究启动沿用同一board runner临界区：研究先启动→忽略409；忽略先生效→新研究/显式重研409，先恢复。已忽略但已有报告的默认research在无running、原门禁满足时仍可200返回原result，不产生新任务。研究中按钮禁用，不能把忽略当取消；restore幂等且不发模型调用。只承诺既有单进程runner边界，不另造分布式锁。

不用候选status存ignored/running，因为用户处理、任务状态和有无报告不是同一维度；不用localStorage保存业务标记。

### D2. 只补必要接口，历史详情复用旧GET

前缀为 `/api/semantic-boards/:boardId/enrichment/analysis`。沿用 `before_id` 排他游标、limit默认20最大100，不新增计数服务。

| 接口 | 最小变化 |
| --- | --- |
| GET /signals（既有） | 新增visibility=visible/ignored/all，省略all兼容旧客户端；新UI显式visible。view=all/pending/readable按无报告且无live任务/存在成功报告筛选；省略all。现有前端参数没有status，不设计虚构的旧status兼容分支 |
| GET /signals/:id（新增） | 调用已有候选读取和发现快照，返回详情、ignored_at、真实状态/最新报告、冻结新闻摘要及related_topics/gaps；只加载当前项，不把每页20条的完整快照都塞进列表 |
| PUT /signals/:id/disposition（新增） | required boolean ignored，拒绝正文/快照/score等字段；成功返回id及ignored_at/null。非法400、owner/不存在404、运行/忽略冲突409，写失败不部分改数据 |
| GET /history（新增） | 按board或所属topic JOIN汇总可验证归属的旧result，支持kind/topic与before_id分页；包括已归档topic，不混signal_report，不逐topic拉全量 |
| 旧历史详情/QA/debates/review GET（复用） | 根据历史行的scope/topic/result走既有接口与kind reader，补必要owner/父子校验；不再新增一套历史详情API，不调用旧生成service |

新view筛选在分页前生效，readable允许已有报告者正在重研；运行身份始终独立可达。界面仅显示「已加载N条」，不让一页数据冒充总量。API层作必要VM归一化，组件不直调HTTP；不全面迁移旧接口。

背景继续复用原 `/persistent-topics/:topicId/enrichment/contexts...` GET/PUT/regenerate。新增可选 `semantic_board_id` 供新多scope界面校验，旧客户端不传保持原行为。旧all API保留，首版UI周/月/年不扩展。

### D3. 相关话题只解现有冻结数据

候选EvidenceRefs复用现有兼容解码，映射 `SignalMaterial.Lanes[].Articles[].SectionID → LaneID`；冻结label/status作为历史来源，当前topic表仅检查存在性/owner及提供当前名称。可验证归属的归档话题可打开；旧快照缺映射、topic删除/迁移则返回gap，不按今天active列表、标题相似度或最新摘要重建。

不增加关联实体，不改发现采样规则或input_snapshot。相邻change的ConfirmedKeyEvents只是attention，不提升为候选依据；BoardThreadBrowser只增加背景入口，保留关键事件、日报定位和话题生命周期。

### D4. 成文只增加三种固定块，不做通用文档框架

新draft保留title、四种section kind、原implication条件字段与charts。每section的blocks只接受：

```json
{"type":"paragraph","text":"文案与 [[data:c1:o1]] 引用"}
{"type":"evidence_table","title":"关键观测","rows":[{"label":"同系列观测","refs":["c1:o1","c1:o2"]}]}
{"type":"chart","chart_id":"ch1"}
```

这只是三种白名单的直接分支，不设计插件注册、任意嵌套、模型布局参数或新的DSL引擎。图表仍提交chart_id/kind/claim/refs；值、原生单位、时间、来源和公式由代码取得。表内同一行并列观测须兼容口径，不暗示跨源可算价差。

保留四角色顺序、条件字段、证据宇宙、3000文案预算、0～3图和最多3次成文尝试。表标题/标签计预算，兼容text不重复计数；未知块/字段、复制数值、悬空ID和重复chart挂载拒绝，聚合回注，不降成任意HTML。thesis/implication只用段落，表/图置于facts/causal。继续airouter、原session与完整日志，不改40轮研究或七源白名单。

持久化schema3，代码生成四段兼容text；schema2原文不动。新reader直接按schema2/3分支，不反向解析旧文生成表格。未知载荷局部错误并可只读原始记录，不自动AI修复。旧内置reader可用兼容text；外部严格schema2客户端需升级，不能承诺它自动接受新版本。

### D5. 精度和图形由少量呈现逻辑统一

优先在现有signalReport/SignalReportView上提取真正复用的逻辑，不为每个标签/章节建组件。必要边界是共享数值引用、证据表和ReportChart；普通章节继续模板分支。

- 阅读短值与raw字符串分开，绝不写回观测/计算。常规价格/百分比两位，其他按单位/量级；极小非零不能变0，空串/null不能被Number转换成0。大数/无法安全格式化时保留安全原值与说明，不猜货币或单位。公式和原precision在核查区保留。
- 每个证据组从appendix机械取源最新期，保留日/月/年频度；cutoff是研究材料边界，created_at是生成时间。比目标期早的资料就地标缺口，无观测不伪造时点。旧schema2保留原facts文字并补分源提示；schema3取消强制固定as-of句，不让模型独自负责时效。
- 复用SVG但提取几何和标签：真实日期距离，null断线，不补零/插值；连续性不明降离散点并标观测数。柱从0，线/点范围与刻度可读，避免小量级刻度全变0或标签越界；原值按需核查。主图紧凑，不按reader宽无限放大。混口径新图拒绝，旧坏图只降级该图。

### D6. 状态分离到够用，不建全局任务中心

在现有useSignalWorkbench中分开当前浏览键与当前board真实job身份。周期/候选/报告切换只换GET和滚动状态，任务入口可返回原周期。切board用epoch隔离晚响应；不持续轮询所有未选board，返回时用现有status API同步。卸载停客户端轮询不取消server任务；重启404/终态错误停轮询并读取持久progress，不自动付费重试。

分页外选中项需要时用新增候选详情读取；版本沿用source_signal_id查询和result_id详情。新版本完成只提示，不强切旧版。旧kind任务仍占原board锁，不因新UI退役其入口而忽略互斥。

忽略失败回滚/保留现场；视图缓存和sessionStorage只存导航，不存业务处理真相。保留BoardEnrichmentPanel、SignalCandidateList和SignalReportView薄宿主，避免同时新建Index/Detail/Section多层包装。

### D7. 背景和历史各一个入口，复用已有能力

TopicNewsBackground按topic+granularity+period独立读写；从研究只读预览，管理转话题上下文，不复用旧focus状态。保存失败留草稿，离开脏草稿需确认；生成需显式确认预算/覆盖。循环A事实记忆不写研究判断，不改已保存快照。

LegacyResearchHistory是一个列表/只读外壳，复用原board/topic reader和QA/辩论展示；不为每种旧kind造新框架。原载荷缺标题用「旧版分析 #ID」明确回退，计数无记录不补0；坏JSON单条降级，不影响其他列表，原始记录仍可只读访问。旧API保留既有行为，但不重启已退役的legacy_board_analysis生成。

### D8. 精准测试与字段锚

只为改变的行为补断言，优先扩展已有测试；不回归测试未修改的40轮循环、每个角色或全站页面。test-cases.md按Requirement列简短故事和旧Scenario处置，tasks映射到实际落点，不复制一套通用验收平台。

必须锁住的风险：①忽略/启动竞态与持久恢复；②schema2/3、非法块与引用；③长小数/非零小量/null/日期间距；④跨scope晚响应和任务可达；⑤背景/历史读写边界。用一条浏览器主链和背景/历史短走查检查整体，不做所有状态的笛卡尔积。

| 可见字段 | 来源 | spec锚 |
| --- | --- | --- |
| 候选标题/原因/问题/发现时间 | 已存candidate/discovery | 候选持久化与展示来源 |
| running/可读/忽略/版本 | live job、成功result、disposition | 工作台展示字段来源；候选忽略与恢复 |
| 数值/单位/公式 | 观测/代码calc，不是模型副本 | 观测与计算引用附录 |
| 研究边界/生成时间/证据时效 | cutoff/created_at/各组观测期 | 报告数据时效声明 |
| 已加载计数 | 当前实际加载条数 | 失败恢复与周期查询 |
| 历史标题/类型/子记录 | 原载荷+程序明确回退、原kind/QA/debate/review | 旧研究历史只读访问 |

效果核对只取必要样本：旧载荷只读各抽schema2和一种legacy；新协议在明确授权下复用成文入口跑一份混合证据样例，记录尝试数、块/引用合法性、原值保持及无HTML，不额外跑整套40轮研究。其他无数字/缺失/坏稿用fixture单测。没有授权或缺样本就写未完成，不假装mock即真实效果。

## Risks / Trade-offs

- [新块协议依赖模型遵循] → 三类平面块、原重试上限和一次代表样例；不加复杂自动修复。
- [旧正文仍冗长或有解释错误] → 只做格式适配，不承诺人工原型的重编文字自动出现，不升级结论。
- [七源仍非通用研究] → 明确缺口，工具能力另案，不用改名掩盖限制。
- [共享宿主/异步竞态] → apply先盘点add-topic-key-event-markers交点，最小添加与关键断言，不重做话题总览。
- [旧归属/载荷缺失] → 不猜回填，单条兼容错误+原记录读取，不造全库迁移项目。

## Migration Plan

1. apply读取本制品、运行doc-impact suggest及并发盘点；新迁移只在临时PG验证，不对真库跑试验SQL。
2. 新表无需回填；先支持新查询和schema3的后端，再部署前端。部署重启前检查真实任务，不能未告知就中断付费研究。
3. 用户无需清理、重配模型或重生成旧报告。回退保留新增表和schema3记录，不DROP、不批量降版，旧内置reader可用兼容text。
4. 按影响包测试、更新必要flow/API/schema/呈现约定并留静态:5100交互与四视口证据；本轮propose不执行。

## Open Questions

无改变规格/范围的待决问题。迁移序号按apply时仓库序列分配；实际文件划分保持上述小边界，不为规划的名称强行拆文件。
