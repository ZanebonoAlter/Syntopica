# Design：板块信号解读报告

## 1. 两阶段流程与人工边界

本版取代「识别后自动钻取」：**发现信号只保存候选；用户逐条点击深入分析才进入研究。** 一次研究只服务一条候选信号、最多生成一篇报告。新报告完全不接review/judge/建议采纳链；旧功能的review不改。

```
手动「发现信号」→ 周期/开关预检 → discovery job
  → freshness → 周期材料 → detect（最多2次尝试）
    ├─ 失败：failed job，不伪装零信号
    └─ 成功：原子保存批次+候选 → job结束
         ├─ 候选为0：正常完成，不弹提示，不清旧记录
         └─ 候选≥1：界面等待用户选择（无后台研究、无LLM会话占用）

用户点击某条「深入分析」→ 归属/开关/互斥预检 → research job
  → 固定该候选的材料与cutoff
  → 问题驱动loop（≤40轮，≤40次执行，可提前结束）
  → compose（最多3次尝试）→ 结构/证据/计算校验
    ├─ 成功：保存不可变signal_report，直接阅读
    └─ 失败：failed job，无不合格result，候选保留可重试
```

发现与研究都手动、都共享现有board级202/409互斥；旧brief/investigation board job也互斥。等待人工选择不持锁。新API不改变旧brief生成端点语义，新工作台不再调用旧生成端点。

## 2. 周期与材料快照

发现请求必填 `granularity=month|year`、`period=YYYY-MM|YYYY`，服务端业务时区校验日历与不晚于当前周期，非法400且无job。半开区间写入快照；当前周期cutoff为发现任务started_at，结束周期cutoff为周期结束。历史标 `retrospective`，显示「事后回顾 · 本次取得的数据版本」，不声称point-in-time回测、不计历史预测兑现率。

- 新闻材料只取目标周期及当时归属板块的切片；候选泳道来自周期材料，不只用今天active泳道。无法还原归属记gap，不猜历史全貌。
- 不直接使用当前 `assembleSituationCards` 最新摘要。复用预算/排序思想，新增周期assembler；摘要混入周期后事实则回落有日期原切片。历史背景不晚于cutoff。
- freshness保持原活跃泳道month/year规则；历史非活跃泳道无摘要用原切片，不无限回填。空材料无需detect LLM，可成功保存空批次。
- 每批保存输入材料、证据白名单、时间边界及选择/排除原因。候选点击研究时读取已保存快照，**不重新识别、不偷偷换成最新材料**；界面显示发现时间，想研究新事实需重新发现。
- 外部工具结果由包装层按候选cutoff筛选后才进入agent。完整期间结束晚于cutoff的观测不得作为截止前已实现数据；无历史覆盖记gap。原响应完整进日志，筛选后观测完整进入附录。
- 官方数据可能修订，观测期过滤不能保证历史版本可得；历史研究始终标事后回顾，前瞻字段解释为当期视角的条件判断。

## 3. 发现、候选持久化与展示字段

detect输出 `signal, why_it_matters, research_question, evidence_refs, score, rationale`，score为1～10整数，prompt按异动强度/报道密度/判断价值打分，门槛6。字段非空，证据引用仅限本次白名单；悬空/跨板块/周期外引用剔除后，无有效依据的信号丢弃。非法schema/分数/路由错误最多重试1次后failed，不吞成空数组。

成功发现原子保存：

- `board_signal_discovery`：id、semantic_board_id、granularity、period、analysis_mode、cutoff、input_snapshot(jsonb)、session_id、candidate_count、created_at；仅成功批次（含0条）写入，不保存半批候选。
- `board_signal_candidate`：id、discovery_id、semantic_board_id、granularity、period、signal、why_it_matters、research_question、evidence_refs(jsonb)、score、rationale、created_at。候选内容不可变，owner/周期与批次一致（复合外键或等效DB约束）。同批完全相同标题+规范化证据集合机械去重；不做跨批模糊语义合并。
- 后续发现追加新批次，不覆盖旧候选/旧报告。列表按周期、批次倒序，空批次不清空已有候选、不额外弹出「无信号」提示；首次无候选显示安静空态。
- 展示字段来源：候选标题/值得查的原因/研究问题来自校验后的detect；新闻依据来自白名单快照；发现时间来自批次；取数次数来自真实执行记录，不从LLM自报；候选状态为服务端派生：该候选有running research→研究中，否则有成功报告→已有报告，否则→待研究。失败job可叠加错误/重试，不变造approved/rejected状态。
- 候选的真实ID由数据库分配，不信任模型id。点击请求仅传candidate_id，不接受浏览器自带材料或cutoff。

## 4. 深入研究预算与收束

新 `signal_research` 复用现有loop基建，隔离自身policy/action扩展。仅它获得四源工具白名单；既有A/B/QA/调查/关系发现不变。

- **总决策最多40轮，总执行最多40次**。每轮只执行一个动作：`call_tool`（官方取数）、`calculate`（本地受限计算）或`finish`。网络取数与本地计算合计占执行预算；调用失败也计执行。非法/重复动作不执行但计决策轮，避免无限空转。不强迫跑满，也不在30轮额外要求人工批准。
- guard继承关thinking、重复规范参数拦截、工具结果不截断。每次取数/计算必须有非空question且对应当前候选；四源不匹配时可提前finish，禁止乱调无关源凑数量。
- prompt要求逐步回答研究问题、寻找可能推翻主解释的事实，并记录未排除解释；不预选结论，不能因要给前瞻就硬做多/做空。
- 提前完成或预算耗尽均转成文；`stop_reason=finished|budget_exhausted`，记录decisions、source_calls、calculation_calls、gaps。耗尽不是失败，也不是强行宣称研究完整；正文须披露影响结论的缺口。
- 单源失败/参数错误记录后继续；取消/超时/上下文容量耗尽是failed，不伪装正常完成。40轮是上限，不保证有限墙钟预算内都能跑完。
- 发现沿用30分钟job超时；研究超时与轮数预算对齐：150分钟（实测单轮108~380s、均值~200s，覆盖40轮+compose 3稿的余量；2026-09-22真实验收两次30分钟超时均砍在第9/12轮，已付费轮次成果丢失促成此项修订）。研究不重复freshness；源码/源调用自身超时按剩余context收敛。成文也在同一总截止时间内，超时保留候选可重试，不自动续跑花费。
- **研究进展持久化（断了不能白跑）**：研究loop每轮结束后将进展滚动upsert到`board_signal_research_progress`（同job一行滚动更新：轮次/取数/计算计数+全量账本jsonb含calls/calcs/gaps）；超时/失败后进展行保留（status=abandoned+stop_reason+error），候选可查"上次研究进展到第N轮/已取得X观测"；成功落库result后进展行标记superseded保留供追溯。进展表不回写候选快照、不写topic_enrichment_result（成功报告仍不可变一行）。ai_call_logs按轮留痕不变，进展表补齐编排层视角，中断后可重建研究过程。前端消费：研究进行中每轮轮询顺带拉取进展端点，UI 显示真实计数（第N/40轮·取数·计算，ui-design「研究进度」契约）并可展开每步调用明细（tasks 4.8）；失败/任务失效后保留全量账本供错误区回看，成功后清空由报告 appendix 接管。
- 材料预算与源窗口在调用前约束，不截工具历史。完整结果无法装入上下文时明确失败并记录原因，不静默丢证据。40轮成本按阶段记录，不沿用旧版「只多两次LLM」估算。

## 5. 数据源窗口、可用能力与受限计算

### 源能力和窗口

当前EIA只提供原油库存三项与产量/进口/出口；JODI只提供CRUDEOIL的production/imports/exports/closing_stocks。**不声称已有炼厂利润、开工率、价格/历史命中率。** 首版不为旧样张顺手扩展指标；相关问题取不到必须记gap。新增协议需适配器、注册与显式授权，不是随意加目录行就能取。

EIA weeks可选，显式整数1～12；不传保持当周+上周。官方历史HTTPS资源模板必须先核定并留fixture，不能猜URL/列；核定失败是该能力实现阻塞。

JODI已有 `month=YYYY-MM` 精确历史单月能力，必须保留。新增 `years` **仅显式传入时启用**最近N个日历年所有可得月份（整数1～5），与month互斥；两者都不传保持旧行为：取当前年文件最新月，即使null不退旧值。不可把原缺省行为悄悄扩大成全年序列。多年窗口只有本年404可回退上一年一次，上一年已请求则复用不重复；历史年404记gap，显式month的404不回退。

缓存键覆盖全部有效参数。成功缓存保留真实retrieved_at/hash/last_modified；错误不缓存，漂移驱逐；缺失符号保留null，单位原生，EIA仅美国。数据源观测仍不落独立业务表，消费方报告留快照。

### 受限计算（不是任意代码工具）

loop新增 `calculate` 动作调用本地白名单解释器，不暴露脚本/SQL/URL/代码执行；不新增通用Registry工具、不改变旧loop语义。

请求：`{op, inputs:[observation_ref...], question}`。支持：

- `difference(a,b)=a-b`：同一系列不同时点，或同源同期间进/出口等批准的兼容流量对；同原生单位。
- `percent_change(current,base)=(current-base)/base*100`：同源同指标同单位，base>0；base=0/负数拒绝，不生成无穷大或误导百分比。
- `mean(refs)`：同源同指标同单位的明确期间集合；不得跨缺期偷偷称完整窗口。

观测必须来自本次已取得且通过cutoff的证据；禁止forward/循环引用，首版输入只允许原始观测，不支持任意表达式链。null任一输入导致缺失结果+原因，不填0；混单位/库存与流量相减/跨源换算拒绝。计算使用规范十进制值，固定half-up保留最多4位小数、去末尾0，公式和舍入规则记录。派生百分比标单位`%`，不是跨源换算。原始观测按源精度显示，不套派生舍入。

代码分配 `calc_id` 并记录 `{op, inputs, expression, value, unit, precision, status, computed_at}`；模型不能提交value。`[[calc:k1]]`由渲染器显示结果，附录可展开输入。代码能保证计算一致，不能自动证明选取指标的经济含义，真实样本验收仍需检查。

## 6. 成文与可核查附录

LLM输出 `{title, sections, charts}`，sections四段按序唯一：thesis/facts/causal/implication；代码追加appendix阅读区，唯一存储为payload顶层appendix，LLM不得生成/覆写。

正文回答：异常是什么→查到了什么→哪个解释更站得住、哪些仍不能排除→判断/触发条件如何改变。不同解释与缺口自然融入causal/implication，不新建独立风险/证伪块。允许「证据不足，维持原判断」，不强迫肯定方向。用户最终批准的表达以sample-report.md大白话版为准：先说结论、发生了什么、这意味着什么、接下来怎么看；避免用去库/转多等术语替代解释，公式/接口字段放附录，原单位数值与引用仍需可查。

- implication：verdict、direction（up/down/diverge/conditional）、horizon、trigger_condition、self_doubt均结构化；非空枚举检查只能保证表达要件，不能当语义真实性认证。
- 原观测引用 `[[data:c1:o2]]`，计算引用 `[[calc:k1]]`，全部由代码渲染数值和原生单位；禁止模型在文本另造与观测矛盾的统计值。新闻事实引用源于候选白名单，不能用无数据报告掩盖无新闻依据。
- charts：`{chart_id,kind:line|comparison,claim,refs[]}`；refs为原观测或已成功计算的派生点，必须同兼容系列/单位，显式标「原值/代码计算」。折线至少两非空点、按期间排序；null断线不补零。禁止跨源单位换算、混频误连。
- 有可绘制序列时1～3图，否则0图+缺口；无关序列不强迫成图。单点可引用。每图明确一个论点，不将几十轮查询堆成数据面板。
- 可见标题/正文/图题少于3000非空白Unicode字符（数字英文均计，附录/UI元信息不计）。不合格最多重试2次，attempts≤3，retries=attempts-1；无截断凑数。
- appendix由代码生成`calls[]/calculations[]/gaps[]`。调用含call_id/question/tool/args/status/error/真实来源时间与hash/完整筛选观测/filter_meta；观测含observation_id/series_id/period/value/unit/missing_marker及源维度。取消50点任意裁切，完整原响应留工具日志。
- 预算耗尽/关键指标缺失需在相关论证或后果段明示；程序可校验结构，结论是否超出证据须通过真实样本质量验收。没有review/judge补救链。

## 7. 结果存储、API与幂等

### 数据库

沿用topic_enrichment_result，新增kind=signal_report；payload放现有sectors，不新造payload列。结构：`{schema_version:2, signal_snapshot, report:{title,sections,charts}, appendix, generation_meta:{session_id,attempts,retries,decisions,source_calls,calculation_calls,stop_reason,analysis_mode,cutoff}}`。

新增可空granularity、period、source_signal_id列。signal_report必须board scope/semantic_board_id非空，persistent_topic_id/parent_result_id/question_key为空，周期与source_signal_id必填；通过复合外键确保候选与result同board/周期，repository与PostgreSQL CHECK双重约束。旧kind新增列全NULL，不猜测回填。新增候选/批次/报告的board-period-id索引及source_signal_id索引。迁移需隔离PostgreSQL测试（不得SQLite代替DB约束测试）。

只保存成功不可变报告；研究失败不写result、不修改候选快照。重新研究成功追加新版本。报告不调用review/judge、不读review digest、不写TopicEnrichmentReview。

### 新API（相对现有board enrichment路由组）

- `POST /signal-discoveries` body `{granularity,period}`：202返回discovery job；成功job带discovery_id/candidate_count，0条outcome=no_signal，其余outcome=discovered。不会带report result_id或启动research。
- `GET /signals?granularity=…&period=…&before_id=…&limit=…`：按id倒序候选列表含批次时间、派生状态、最新成功result_id；默认20最大100，不被空新批次清空。
- `POST /signals/:candidate_id/research` body `{regenerate?:false}`：校验board归属/增强开关；先共享board互斥，running返回409；无running且已有成功报告、regenerate=false则200返回existing result_id（无新LLM）；否则202新research job。显式重新研究才regenerate=true，任何失败后重试仍需用户点击。
- `GET /signal-reports?granularity=…&period=…&before_id=…&limit=…`：仅成功报告，默认20最大100；可带source_signal_id过滤该候选版本（校验归属）。
- `GET /signal-reports/:result_id`：owner/kind不匹配404，返回不可变payload，**无review字段**。
- job查询沿用现有API，新增job_kind=`board_signal_discovery|board_signal_report`、phase=`prepare|detect|research|compose`、outcome=`discovered|no_signal|succeeded|failed`、candidate_id/discovery_id/周期及计数/error_stage（按kind可选）；不新增review phase。

### 任务恢复与安全

内存job不是持久队列：同进程刷新可恢复；后端重启job404停止轮询并提示可重试，候选/批次和成功报告仍可查。候选不保存running布尔值，防重启卡死；运行状态来自live job。失败仅job错误，不进报告历史。

research不接受客户端改标题/证据/周期；跨板块candidate/result均404。一个候选重复点击并发只有一个任务，完成后默认返回已有结果；创建job与已有报告检查需在同board互斥保护下，防检查/执行竞态。旧API语义、工具面不变。

## 8. UI与样例

沿用editorial reader≤760px/hairline/印刷红和新闻背景折叠区。工作台变为候选信号列表+已生成报告：顶部「发现信号」，每条列出异常/值得查的原因/研究问题/依据/发现时间，操作「深入分析」；已有报告展示「阅读报告」，详情提供显式「重新研究」。无评审按钮、徽标或采纳流程。

新人工确认步骤属于major交互变更；用户已在简化样例后明确确认进入apply，ui-approval=approved，批准记录见ui-design。不得再改交互结构而沿用此次批准。

样例文件`sample-report.md`与原型使用同一**合成数据fixture**，显著标非真实行情。示范仅用当前EIA适配器已有库存/产量/进出口字段及代码计算，删除开工率、炼厂利润、无据历史命中率。样例不能代替真实取数端到端验收，真实样本须另留日志/数据时间/hash。

## 9. 可观测、上线影响与操作

发现与研究各有独立session_id，通过discovery_id/candidate_id串联；不得把等待人点击伪装成长驻LLM session。airouter登记detect/research/compose，计算与工具事件记输入、结果、耗时、预算及stop_reason，无judge操作。成本按实际决策/取数/计算/重试分开计数。

实现部署后默认入口先出候选，不再自动产报告；用户点击深入分析才发生长研究。旧报告/绑定不删除、不迁移成新报告，仅旧API兼容；新增表和nullable列随后端迁移，前端验证后静态部署。用户无需清理旧数据，需手动发现并选择候选。本轮仅规划与原型修订，无部署/线上数据操作。

## 10. 实战校准：数据时效、能力前置与残留治理（2026-09-23 首份真实报告诊断）

首份真实 signal_report（result id=19，候选4，11:56→12:47 共50分34秒，decisions=40 全用满、source_calls=29、calculation_calls=10）暴露「数据滞后→文不对题 + 轮次烧满」。诊断事实（各源实测期/失败明细/轮次去向）固化在 explore-findings「2026-09-23 实战校准」节，四组修复如下，对应 specs delta。

### 10.1 源能力与host如实声明（治文不对题的根 + 白烧轮子）

- **host 白名单单一事实来源**：源实现实际请求的每个 HTTPS host 必须在 catalog `HostAllowlist` 内。eia_wpsr 现漏 `www.eia.gov`（归档版次 `eiaArchiveBase`），导致 `weeks>2` 生产必坏（SOURCE_UNAVAILABLE「目标 host 不在核定白名单内」，result id=19 第15步实证；归档 URL 直测 200 可达）。修正 catalog；**回归测试必须用真实 Fetcher + catalog 白名单走归档路径**——现状 eia_test 用 httptest 自建 fetcher 绕过生产白名单，正是本 bug 漏网原因。
- **发现阶段能力前置**：detect system prompt 注入四源 catalog 元数据（coverage/frequency/typical_lag/unit_policy）+ 硬限制（无价格/裂解价差/运价/政策数据；EIA 仅美国、JODI 月滞后约1.5-2月、WDI 年度、Comtrade 视订阅档覆盖），使 research_question 落在可回答范围。发现阶段不因此开始取数（工具面不变量不动）。
- **研究阶段可用性如实**：toolsDesc 在 description/schema 之外附 catalog 覆盖与滞后；RequiresKey 源 key 缺失时该工具条目标注「当前不可用：未配置 <CONFIG_KEY>」且调用返回显式错误帧（Registry.Execute 未知工具/错误帧语义保留），不把不可用源当可用能力宣称。
- **实测覆盖限制如实**：Comtrade 订阅档实测仅部分 reporter 有数（CN 有数、US/JP 任意期 count=0），toolsDesc 注明 count=0=该期未发布不等于调用失败，避免 agent 换参空转。

### 10.2 无覆盖早停引导（治满40轮）

- 不改 maxLoops=40、不强制终止、计轮规则不变（spec「四十轮问题驱动研究」预算语义不动）。
- 触发：同一源**连续3次** `kept=0`（no_data）或源错误 → 该轮之后向 agent 历史注入反馈「该源在本研究问题上已连续3次无覆盖，换参数也不会有数据；若四源皆无覆盖请直接 finish」；**每源最多注入一次**防刷屏；四源全部触发过再注入一次终局提示。
- 注入通道：共享 `runToolLoop` 新增**可选** policy 扩展接口（type assertion，沿用 `toolLoopActionRunner` 先例），policy=nil 与既有 policy 行为字节不变；反馈是历史行，不伪装成工具结果。

### 10.3 报告数据时效声明（治「读者看不出滞后」）

- compose user prompt 注入机械计算的「各源最新可得期 + 研究时点 cutoff」（按源分组取账本观测 period 的 max），要求 facts 段必须出现「数据截至」+ 最新数据期；校验缺该字样**或所写期值不在各源最新期集合（∪「无可用数据期」）**回注重试（沿用 attempts≤3）——期值按月粒度归一比较，容忍 `YYYY年M月`/`YYYY-MM`/`YYYY-MM-DD` 写法差异，只拒编造期值（review M1：否则模型可写未来期值蒙过字样检查，正文与前端 as-of 行互相矛盾比不写更误导）。
- 前端阅读视图标题下渲染**机械行**「数据截至 {账本最大期} · 研究时点 {cutoff}」，数据取自已入库 payload 的 appendix+generation_meta——**不改 payload schema、不改 API 契约**；无观测期时如实显示「无可用数据期」。存量报告（id=19）同样受益。

### 10.4 残留与状态治理（治白跑与骗人状态）

- **孤儿进展收敛**：启动时（内存必然无活 job）把 `board_signal_research_progress` 中 status=running 的行收敛为 abandoned、`stop_reason=orphaned_by_restart`（status CHECK 不变，stop_reason 自由文本）。关机 defer 的 `context.WithoutCancel` 终态写继续保留，负责活进程内路径；进程死亡路径由启动收敛兜底——实证 2026-09-23 候选6 的34轮进展行至今 running、UI 永久悬挂。
- **data_sources status 状态源统一**：`datasources.Init` 现用仅读 `config.AppConfig` 的自建 resolver，忽略 UI 配置的 key（`wiring.ComtradeKeyResolver` 才是 UI 优先链），故 UI 已配 key（2026-09-20）而 status 恒为 disabled。改为 main.go 传入 wiring 的 live resolver；UI 保存 key 的路径同步回写 status（运行期即时生效，不必重启）。
- **不按陈旧 status 行过滤 registry**（会误杀实际可用源）：可用性判断一律走 live resolver（10.1 的 toolsDesc 标注 + Execute 显式错误），避免「状态陈旧 → 错误隐藏可用工具」的反向事故。
