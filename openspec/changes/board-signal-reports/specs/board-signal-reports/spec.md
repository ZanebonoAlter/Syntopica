## Purpose

发现值得研究的新闻信号，由用户逐条决定是否深挖，生成可核查、允许保留判断的短报告。以人工触发控制研究成本，不自动查询、不设报告评审链。

## ADDED Requirements

### Requirement: 周期材料与固定候选快照

发现请求 SHALL 接收month/year及合法period，未来周期400。服务端业务时区计算半开边界；当前周期cutoff为发现started_at，历史为周期结束。历史 SHALL 标事后回顾，不声称历史可得版本回测。新闻主材料限目标周期，依据来自当时板块归属；不得用今天active泳道/最新摘要代替历史。研究 SHALL 消费候选已保存的快照与cutoff，不重新识别或悄悄改材料。

#### Scenario: 周期校验

- **WHEN** 传非法日历、week、空period或未来周期
- **THEN** 400且不创建job，不调用LLM

#### Scenario: 历史材料隔离

- **WHEN** 所选历史周期有今天非active泳道的新闻
- **THEN** 按周期材料归属纳入，排除周期后事实；无法恢复归属记gap，报告标事后回顾

#### Scenario: 迟些点击仍用原快照

- **WHEN** 候选生成后又出现新新闻，用户才点击深入分析
- **THEN** 使用候选已冻结材料/时间边界，显示发现时间；更新研究前提需用户重新发现

#### Scenario: 外部数据边界

- **WHEN** 工具返回cutoff之后的观测或无目标期覆盖
- **THEN** 在进入agent前过滤并记gap，完整原响应留日志，不以新数据替旧数据

### Requirement: 信号发现与人工研究边界

发现 SHALL 仅做周期补齐、材料装配和detect，成功保存批次/候选即结束job。score为1～10整数、阈值6且至少一个有效新闻依据。非法引用剔除后无依据的信号丢弃。空材料/合法无达标是no_signal；识别失败最多重试1次后failed，不隐藏故障。SHALL NOT 自动调用取数、计算、compose或judge。

#### Scenario: 达标只生成候选

- **WHEN** 至少一条有依据的信号score≥6
- **THEN** 保存候选并返回discovered，数据源/计算/成文调用均为0，无报告result；等待用户点击

#### Scenario: 零信号零打扰

- **WHEN** 无材料或合法无达标结果
- **THEN** 保存成功空批次，outcome=no_signal，无新报告/提示，不清空已有候选/报告

#### Scenario: 检测故障可见

- **WHEN** 两次识别尝试仍路由或解析失败
- **THEN** failed job显示阶段错误，不保存半批候选，不伪装空发现

#### Scenario: 依据白名单

- **WHEN** 高分信号仅引用悬空/跨板块/周期外证据
- **THEN** 该信号丢弃，不通过高分绕过依据限制

### Requirement: 候选持久化与展示来源

成功发现 SHALL 原子写入board_signal_discovery及board_signal_candidate，批次与候选owner/周期一致，内容不可变。同批完全重复信号可机械去重，跨批不做模糊合并。标题/原因/问题来自校验后的detect，新闻依据来自快照，发现时间来自批次；状态由live研究任务及成功报告派生，不由LLM自报或持久running位决定。多批候选和报告 SHALL 按周期可翻阅。

#### Scenario: 刷新与重启保留候选

- **WHEN** 用户发现候选但未点击研究，随后刷新或后端重启
- **THEN** 候选及其依据仍可查，无隐式研究启动，不留永久running状态

#### Scenario: 重复发现不覆盖

- **WHEN** 同周期再次发现信号
- **THEN** 新增批次，旧候选/报告仍在；本次空结果不抹去历史列表

#### Scenario: 列表字段可追溯

- **WHEN** 渲染候选或报告列表
- **THEN** 原因/问题/时间/状态/取数次数分别来自保存字段或实际job/工具记录；不让模型声称已研究或已评审

### Requirement: 单候选人工触发与幂等

用户显式POST候选research才启动研究；每次只处理一个candidate_id，服务端校验owner/增强开关，客户端不得覆盖材料。与所有现有board任务共享互斥：运行中409；无运行且已有成功报告时默认200返回已有result，显式regenerate=true才202重做。SHALL NOT 自动研究所有候选、失败自动续跑或自动挂日报调度。

#### Scenario: 点击一条只研究一条

- **WHEN** 列表有多条候选，用户点击其中一条
- **THEN** 仅该候选生成202研究job，其余不调用数据源或成文

#### Scenario: 重复点击与重新研究

- **WHEN** 并发点击同候选、成功后再次默认点击、或显式重新研究
- **THEN** 分别为单任务+409、200已有result且无新LLM、202新版本；检查与创建在同board锁下

#### Scenario: 跨板块拒绝

- **WHEN** 用另一板块的candidate_id或result_id请求
- **THEN** 404，不消费客户端伪造证据/周期

### Requirement: 四十轮问题驱动研究

新研究loop SHALL 总决策≤40、总执行≤40，每轮至多一个call_tool/calculate/finish；数据源与本地计算合计计执行，失败也占预算。非法/重复动作不执行但计轮次。仅四数据源白名单可取数，不含web_search/代码执行。SHALL 关thinking、完整工具结果不截断、重复调用拦截，每次动作带非空question。允许提前finish，不强迫跑满或在30轮再审批。研究job超时 SHALL 与轮数预算对齐（150分钟；发现保持30分钟），超时/取消/失败时 SHALL 将已完成的轮次成果（轮次计数+账本：调用/观测/计算/缺口）持久化到研究进展表并保留可查，不得随job终止丢失；成功落库报告后进展行 SHALL 标记superseded保留。进展表不回写候选快照、不替代不可变报告。同一数据源连续无覆盖或失败时，SHALL 向研究历史注入如实反馈引导提前收束，不诱导换参刷轮次探明能力，不改变40轮上限与计轮规则；进程重启后残留的 running 进展行 SHALL 在启动时收敛为可查终态，不得永久悬挂。

#### Scenario: 空手源提示早停

- **WHEN** 同一数据源连续3次返回无观测或源错误
- **THEN** 后续研究历史收到「该源无覆盖、勿再换参重试、四源皆无则直接 finish」的反馈（每源最多一次，四源全触发后再提示一次终局）；不改40轮上限、不强制终止、反馈不伪装成工具结果

#### Scenario: 进程重启收敛孤儿进展

- **WHEN** 研究进行中进程被终止，随后后端重启
- **THEN** 启动收敛把残留 running 行标为 abandoned 且 stop_reason=orphaned_by_restart，候选可查到该次研究已终止与当时轮次，UI 不永久显示研究中

#### Scenario: 十几轮查询正常完成

- **WHEN** 第17轮完成研究且未超预算
- **THEN** 正常进入成文，不因旧4次/8轮限制提前截停

#### Scenario: 达到四十轮收束

- **WHEN** 第40轮或第40次执行完成且还想继续
- **THEN** 不再执行第41次，stop_reason=budget_exhausted，以已有证据成文并披露影响结论的缺口

#### Scenario: 重复与非法动作有限结束

- **WHEN** 模型持续发重复参数或未授权动作
- **THEN** 不执行，但每次消耗轮次，最多40轮结束，不无限循环

#### Scenario: 工具失败和任务超时不同

- **WHEN** 单源失败或全部无匹配数据，或者整个job超时/取消
- **THEN** 前者记gap可继续/无数据成文；后者failed可人工重试，不伪装正常预算收束

#### Scenario: 超时后进展不白跑

- **WHEN** 研究进行到第N轮时job超时/取消/失败
- **THEN** 进展表保留前N轮的轮次计数与全量账本（调用/观测/计算/缺口），候选可查上次进展与失败原因，可人工重试；重启后进展仍可查

#### Scenario: 成功后进展归档

- **WHEN** 研究成功落库signal_report
- **THEN** 对应进展行标记superseded保留供追溯，不删除不覆盖报告账本

### Requirement: 代码计算与公式证据

研究 SHALL 支持受限calculate动作：difference、percent_change、mean，不开放任意表达式/脚本。输入只能引用本次cutoff合格的原始观测，公式和value由代码生成。单位/系列兼容校验、null传递、base≤0拒绝百分比、最多4位half-up舍入 SHALL 留痕；禁止跨源换算、库存减流量、前向/循环引用。计算计入总执行预算。

#### Scenario: 同单位差值与百分比

- **WHEN** 同系列库存从420.0变419.0 MMbbl
- **THEN** 代码计算−1 MMbbl与−0.2381%，记录输入ID/公式/精度，不信模型提交的数值

#### Scenario: 受限兼容流量

- **WHEN** 同源同期间进口6400、出口3600 Mb/d请求difference
- **THEN** 经批准的兼容流量规则得到2800 Mb/d；MMbbl库存减Mb/d流量拒绝

#### Scenario: 缺失与非法计算

- **WHEN** 输入null、百分比基数0/负数、未知op、脚本或悬空引用
- **THEN** null产缺失结果+原因，其余拒绝；无补零、Infinity或任意代码执行

### Requirement: 报告论证与条件判断

报告 SHALL 按thesis/facts/causal/implication四段成文，代码附录为第五阅读区。论证要区分已证实事实、解释及未排除可能性；允许证据不足维持判断，不强迫方向结论。implication含verdict/direction(up/down/diverge/conditional)/horizon/trigger_condition/self_doubt。字段校验不等于证明因果真实。SHALL NOT 新增独立falsification/risk块或在成文后调用review/judge。

最终标题/正文/图题 SHALL <3000非空白Unicode字符，附录/UI不计；有相关可绘制序列1～3图，否则0图+缺口。成文最多初次+2次重试，耗尽failed，无不合格result。

#### Scenario: 不足以转多也能成文

- **WHEN** 数据只支持去库事实，不能证明需求或价格见底
- **THEN** 报告明确证据边界与需要的后续条件，可维持判断，不编造开工率/历史胜率凑结论

#### Scenario: 结构与篇幅校验

- **WHEN** 缺段/条件字段、未知kind或正文恰3000字符
- **THEN** 校验错误回注重试；连续三次失败attempts=3/retries=2且不入库

#### Scenario: 图表降级

- **WHEN** 只有单点/null/无相关序列，或有相关序列
- **THEN** 前者可0图并说明缺口，后者1～3图且每图有论点；不为凑图调用无关源

### Requirement: 报告数据时效声明

成文 SHALL 基于账本机械计算各源最新可得期并注入研究时点，正文 facts 段 SHALL 出现「数据截至」与最新数据期，缺失则回注重试（沿用成文重试上限）；阅读视图 SHALL 由已入库 payload 机械渲染「数据截至 {最大期} · 研究时点 {cutoff}」，无观测期时如实显示无可用数据期。SHALL NOT 改变 payload schema 与 API 契约。

#### Scenario: 正文与阅读视图都标数据截至

- **WHEN** 账本最新观测期为 2026-07、研究时点为 2026-09-22
- **THEN** 正文 facts 段含「数据截至 2026-07」，阅读视图标题下渲染机械 as-of 行，读者能看出数据滞后

#### Scenario: 时效缺失回注重试

- **WHEN** 首稿正文未出现「数据截至」
- **THEN** 校验回注重试，成功或耗尽重试上限前不入库

### Requirement: 观测与计算引用附录

正文SHALL通过data:call_id:observation_id或calc:calc_id引用，代码渲染数值/单位。图表引用已有原观测或成功计算点，兼容系列/期间/单位，标注派生来源，null断线非0。appendix SHALL由代码保存调用账本、筛选后全部观测、计算与gaps；完整原响应在日志可追溯，LLM不得覆写附录、不任意裁50点。

#### Scenario: 精确引用与定位

- **WHEN** 引用c1:o2或k1
- **THEN** 显示该原观测/代码结果；点击展开附录并定位；悬空、计算失败引用拒绝

#### Scenario: 图表口径不混淆

- **WHEN** 图引用重复点、混单位/频率或不足两个非空点
- **THEN** 拒绝不合格图；合法派生图标代码计算，不伪装源返回原值

#### Scenario: 完整账本

- **WHEN** 一次研究返回超过50个合格观测或取数失败
- **THEN** 全部合格观测/失败记录保留，真实元信息缺失显示null，不伪造成功时间

### Requirement: 成功快照与无评审输出

signal_report SHALL以board scope存topic_enrichment_result，sectors为schema_version=2载荷；semantic_board_id/周期/source_signal_id必填，topic/parent/question_key为空，与候选同board/周期由DB约束保证。旧kind新增列NULL不猜回填。成功只追加不可变快照，无failed result；新链 SHALL NOT 调用judge、读review digest、写review或显示评审操作。

#### Scenario: PG直接写入约束

- **WHEN** 绕repository插入非法owner/周期/跨板块候选的报告
- **THEN** PostgreSQL拒绝，不仅依赖前端/SQLite校验

#### Scenario: 报告完成直接可读

- **WHEN** 第一份或后续报告校验入库
- **THEN** job=succeeded/result_id，直接阅读；无review行、调用、徽标或采纳步骤

#### Scenario: 旧数据与新版并存

- **WHEN** 迁移旧库并对候选重新研究
- **THEN** 旧kind内容不变且新增列NULL；新报告追加版本，不覆盖旧报告或候选

### Requirement: 失败恢复与周期查询

发现和研究均202异步，job区分kind/phase/outcome及候选/批次；失败保留已完成freshness和候选。job仍内存态，重启404 SHALL 停止轮询提示可重试，不丢持久候选/报告。候选/报告列表SHALL按board/粒度/周期和id游标分页，默认20最大100，不混旧kind。

#### Scenario: 研究失败可重试

- **WHEN** 研究超时或成文不合格耗尽
- **THEN** job错误，无新result，候选仍在且可用户手动重试；不自动恢复付费研究；错误信息携带进展摘要（已完成轮次/已取得观测数），可查完整进展账本

#### Scenario: 重启恢复

- **WHEN** job查询因后端重启404
- **THEN** 停止轮询说明状态不可恢复，重新获取持久候选/报告；候选不会永久研究中

#### Scenario: 周期分页

- **WHEN** 切换月/年/period或翻页
- **THEN** 只返回该board/周期候选与报告，保持旧页记录，不按created_at推断归属期
