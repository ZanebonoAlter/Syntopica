## Purpose

研究 = 人工触发、异步产出、可核查报告，研究成本受用户控制（不自动查询、不设评审链）。本族定义两条研究线：①外部新闻信号——周期材料与固定候选快照、四十轮问题驱动研究、证据与数据时效声明、失败恢复、候选忽略与恢复；②版块语义调查——可扫描简报、用户选择问题后展开多假设/正反证的深度调查、父子不可变快照、旧报告兼容。两条线共享「候选 → 人工触发 → 异步产出 → 可核查」骨架，产出物都待人判断而非系统结论。

## ADDED Requirements

### Requirement: 周期材料与固定候选快照

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


发现请求 SHALL 接收month/year及合法period，未来周期400。服务端业务时区计算半开边界；当前周期cutoff为发现started_at，历史为周期结束。历史 SHALL 标事后回顾，不声称历史可得版本回测。新闻主材料限目标周期，依据来自当时板块归属；不得用今天active泳道/最新摘要代替历史。研究 SHALL 消费候选已保存的快照与cutoff，不重新识别或悄悄改材料。

手动发现 SHALL 纳入本次读取时已确认、同板块同泳道且来源事实属于目标周期并不晚于cutoff的关键节点作为人工关注线索。线索 SHALL 与新闻事实/AI 理由区分，并冻结在发现快照；历史期使用后来做出的人工选择 SHALL 明示为本次回顾关注，SHALL NOT 声称当时已知的预测信号。关注线索 SHALL 不直接等于信号候选，不绕过有效新闻切片引用白名单；无法在本次材料中验证依据时 SHALL 标记gap而不是用确认快照冒充有效原文。无节点的旧材料 SHALL 保持既有流程；后来确认/取消 SHALL 不改变已冻结批次、候选和报告。

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

#### Scenario: 只冻结合窗人工关注
- **WHEN** 本次有本泳道已确认节点、未确认建议、他板块节点及周期后事实
- **THEN** 只纳入同作用域已确认且事实合窗的线索，其余不进入人工关注字段；手动选择不预设重大影响

#### Scenario: 后续撤销不改旧研究
- **WHEN** 发现已冻结节点而用户随后取消标记或确认另一事件
- **THEN** 旧候选及其研究沿用原快照；只有重新手动发现才使用新选择

#### Scenario: 失效依据不升级为有效信号
- **WHEN** 已确认快照尚在但找不到本次材料白名单内的有效新闻切片
- **THEN** 如实记gap，不以人工标记绕过依据验证，不把快照当作新闻原文

#### Scenario: 事后选择不冒充当时预测
- **WHEN** 用户现在标记过去的事件并发现历史周期信号
- **THEN** 保留来源日期和确认时间，标为本次回顾关注，不声称当时已获得该信号

### Requirement: 信号发现与人工研究边界

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


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

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


成功发现 SHALL 原子保存批次与候选，owner/周期一致；候选新闻内容与依据不可变。同批完全重复可机械去重，跨批不做模糊合并。标题、原因和研究问题来自校验后 detect，新闻依据来自冻结快照，发现时间来自批次；运行态来自真实任务、可阅读性与版本来自成功报告，不由模型自报或永久 running 位决定。忽略 SHALL 是独立用户处理标记，不改候选内容、score、快照或报告。多批候选与报告按周期可查；内部 score SHALL NOT 展示为可信度、研究质量或必须完成的排序承诺。

#### Scenario: 刷新与重启保留候选

- **WHEN** 用户发现候选但未点击研究，随后刷新或后端重启
- **THEN** 候选及其依据仍可查，无隐式研究启动，不留永久running状态

#### Scenario: 重复发现不覆盖

- **WHEN** 同周期再次发现信号
- **THEN** 新增批次，旧候选/报告仍在；本次空结果不抹去历史列表

#### Scenario: 列表字段可追溯

- **WHEN** 渲染候选或报告索引
- **THEN** 原因/问题/时间/任务状态/版本/取数次数来自保存字段、实际任务和账本；忽略来自用户处理标记，不让模型声称已研究或已评审

### Requirement: 单候选人工触发与幂等

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


只有用户显式提交候选研究请求才启动研究，每次只处理一个候选；服务端校验 owner 与增强开关，客户端不得覆盖材料。与现有同 board 任务共享互斥：运行中409。无运行且已有成功报告时，默认请求仍200返回已有result且零新LLM；regenerate=true才请求新版本。对已忽略候选，新研究或显式重新研究 SHALL 返回409并要求先恢复，不阻止GET旧报告。忽略写入与新研究启动 SHALL 在同一允许性边界串行判定：不出现研究中成功忽略或忽略后仍新启动的竞态。SHALL NOT 自动研究其他候选、失败自动续跑或自动挂日报调度。

#### Scenario: 点击一条只研究一条

- **WHEN** 列表有多条候选，用户点击其中一条
- **THEN** 仅该候选生成202研究job，其余不调用数据源或成文

#### Scenario: 重复点击与重新研究

- **WHEN** 并发点击同候选、成功后再次默认点击、或显式重新研究
- **THEN** 分别为单任务+409、200已有result且无新LLM、202新版本；检查与创建在同board锁下

#### Scenario: 跨板块拒绝

- **WHEN** 用另一板块的candidate_id或result_id请求
- **THEN** 404，不消费客户端伪造证据/周期

#### Scenario: 忽略与研究启动竞争

- **WHEN** 同一候选的忽略与新研究并发到达
- **THEN** 至多一个操作使其进入新状态；研究先启动则忽略409，忽略先生效则新研究409；旧报告不删除

#### Scenario: 已忽略报告仍能只读

- **WHEN** 候选已忽略且已有成功报告
- **THEN** GET仍可读；无运行且增强开启时默认research保留已有result的200幂等读取，regenerate=true拒绝直到恢复

### Requirement: 四十轮问题驱动研究

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


新研究loop SHALL 总决策≤40、总执行≤40，每轮至多一个call_tool/calculate/finish；数据源与本地计算合计计执行，失败也占预算。非法/重复动作不执行但计轮次。仅核定数据源白名单（七源：eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade/yahoo_chart/fred_series/gdelt_doc）可取数，不含web_search/代码执行。SHALL 关thinking、完整工具结果不截断、重复调用拦截，每次动作带非空question。允许提前finish，不强迫跑满或在30轮再审批。研究job超时 SHALL 与轮数预算对齐（150分钟；发现保持30分钟），超时/取消/失败时 SHALL 将已完成的轮次成果（轮次计数+账本：调用/观测/计算/缺口）持久化到研究进展表并保留可查，不得随job终止丢失；成功落库报告后进展行 SHALL 标记superseded保留。进展表不回写候选快照、不替代不可变报告。同一数据源连续无覆盖或失败时，SHALL 向研究历史注入如实反馈引导提前收束，不诱导换参刷轮次探明能力，不改变40轮上限与计轮规则；进程重启后残留的 running 进展行 SHALL 在启动时收敛为可查终态，不得永久悬挂。

#### Scenario: 空手源提示早停

- **WHEN** 同一数据源连续3次返回无观测或源错误
- **THEN** 后续研究历史收到「该源无覆盖、勿再换参重试、白名单源皆无则直接 finish」的反馈（每源最多一次，全部源触发后再提示一次终局）；不改40轮上限、不强制终止、反馈不伪装成工具结果

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

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


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

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


新报告 SHALL 保留 thesis/facts/causal/implication 四种论证角色且按序唯一，但以受控段落、证据表、图表引用表达，而非只返回四个长字符串。模型只提交文案及有效证据/图表ID，不提交数值副本、HTML/CSS、可执行代码或自由图表配置；纯叙事报告不强制表格。每个证据块的值、单位、期间和来源由系统从账本生成。implication 继续包含 verdict/direction(up/down/diverge/conditional)/horizon/trigger_condition/self_doubt；允许证据不足维持判断，区分事实、解释和未排除可能性。SHALL NOT 新增独立falsification/risk模型块、成文后review/judge或结论真伪认证徽标。

模型可见标题、段落、证据表标题/行标签、图题和后果字段合计仍 SHALL 小于3000非空白Unicode字符，兼容text不重复计数、附录/UI不计。0～3图，相关证据不足允许0图并披露缺口，不为凑图查无关源。成文最多初次+2次重试；未知块、悬空引用、非法数值文案、重复图挂载或超限均回注，三次仍失败不保存不合格报告。旧schema=2不重编，按原四段兼容阅读。

#### Scenario: 不足以转多也能成文

- **WHEN** 数据只支持去库事实，不能证明需求或价格见底
- **THEN** 报告明确证据边界与需要的后续条件，可维持判断，不编造开工率/历史胜率凑结论

#### Scenario: 结构与篇幅校验

- **WHEN** 新稿缺论证角色/条件字段、含未知块或可见文案恰3000字符
- **THEN** 校验错误回注；连续三次失败attempts=3/retries=2，不入库；不能退回任意HTML蒙混通过

#### Scenario: 图表降级

- **WHEN** 只有单点/null/无相关序列，或有相关序列
- **THEN** 前者可0图并说明缺口，后者1～3图且每图有论点；不为凑图调用无关源

#### Scenario: 无数字仍可成文

- **WHEN** 问题只有可用新闻背景、没有相关可绘制观测
- **THEN** 用有依据的叙事与缺口成文，不强塞指标表或空图

#### Scenario: 块内引用必须核查

- **WHEN** 模型在证据表中复制数值、引用未知观测或把未定义chart挂入正文
- **THEN** 拒绝该稿并回注，不让复制值绕过账本或凭模型自报来源

### Requirement: 报告数据时效声明

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


系统 SHALL 由入库账本按数据源和引用证据组机械计算最新可得期，保持原频度（日YYYY-MM-DD、月YYYY-MM、年YYYY）。研究时点来自冻结cutoff，实际报告生成时间来自保存时间，两者不得混用。新schema=3通过证据/图表元信息并排呈现各源/各组时效；不再强制facts重复全局「数据截至」句，也不以一个最大日期暗示所有源均覆盖研究窗口。引用组比目标窗口早时 SHALL 就地标明覆盖缺口；缺失/无观测/未提供单位如实显示，模型不得编造更新期。

旧schema=2的正文保持不变，读取仍支持原机械as-of信息，但 SHALL 明示其为各源最大期并补可得的分源时效，不能改写旧文、升级结论或自动付费重生成。新旧载荷版本明确区分。

#### Scenario: 正文与阅读视图都标数据截至

- **WHEN** 读取旧schema=2，最新观测为2026-07而cutoff为2026-09-22
- **THEN** 旧facts原文不改；读取可见该最大期、研究边界及源时效，不把旧数据标为九月已验证

#### Scenario: 日度源参与时效声明

- **WHEN** 账本含日度价格最新2026-10-01和月度实物最新2026-07
- **THEN** 两组分别标期，实物组提示未覆盖十月；旧稿原max日期可保留但不统称所有资料已更新

#### Scenario: 时效缺失回注重试

- **WHEN** 新schema=3稿未写「数据截至」但账本具备观测日期
- **THEN** 不因缺固定短语拒绝；系统生成分组时效；若稿编造不受支持的新时点则校验/样例核对揭示错误

#### Scenario: 零观测不伪造时点

- **WHEN** 研究账本无可用观测或全为失败
- **THEN** 显示无可用数据期和缺口，不用cutoff或今天日期充当数据期

### Requirement: 观测与计算引用附录

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


正文与受控证据块 SHALL 通过当前研究的原观测或成功计算ID引用，代码解析值、原生单位与公式。阅读格式与精确原值 SHALL 分开：常规价格/百分比可用短格式，原字符串、代码计算精度和公式在核查区逐项保留，不写回附录；极小非零值不能显示成0，非法数值不能变NaN/Infinity或假0，未知单位不能推断为货币。模型不决定阅读小数位，不代算。

图表只用这些引用，维持同源/同单位/系列/频率等校验，派生点标代码计算；日期/月份按实际时间位置，禁止按数组位置假装等时距。显式null/缺失保持断点，不补零、不补日期；连续性无法证明可降离散点并说明观测数。坐标/点值用可读短格式，原值可查，紧凑且标签不越界。新稿图不合法拒绝；旧图无法解析时局部降级并保留其他正文与原始依据。

appendix SHALL 由代码保存调用账本、筛选后全部观测、计算与gaps，完整原响应可追日志，模型不得覆写或任意裁50点。

#### Scenario: 精确引用与定位

- **WHEN** 正文或证据块引用c1:o2或k1
- **THEN** 显示来自同一观测/计算的阅读值和原单位；点击到原值/公式；原字符串不改，悬空或计算失败的新引用拒绝

#### Scenario: 图表口径不混淆

- **WHEN** 新图含重复点、混单位/频率或不足两个非空点
- **THEN** 拒绝不合格图；合法派生值标代码计算；旧图异常只降级该图而不伪装缺失为0

#### Scenario: 完整账本

- **WHEN** 一次研究返回超过50个合格观测或取数失败
- **THEN** 全部合格观测/失败记录保留，真实元信息缺失显示null，不伪造成功时间

#### Scenario: 长小数与极小值

- **WHEN** 原值为100.19999694824219、极小非零数、0、null或非法字符串
- **THEN** 常规值短格式可读，非零保留量级，真0仍为0，null/非法值显式缺失或无法格式化；原值核查可见

#### Scenario: 真实日期与图表断点

- **WHEN** 观测分别在十月7日、8日和14日，或中间显式null
- **THEN** 横轴按1天与6天实际距离；null不连线，不造中间数据，标签不输出十几位小数

### Requirement: 成功快照与无评审输出

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


signal_report SHALL 继续以board scope保存，semantic_board_id/周期/source_signal_id必填，topic/parent/question_key为空，owner/周期由DB约束保证。新受控块报告用schema_version=3并提供系统生成的兼容四段text；旧schema_version=2保留原样可读，旧kind新增列NULL不猜回填。成功只追加不可变快照，不存failed result；新链不调用judge、不读review digest、不写review或显示评审。忽略仅为候选展示处理，不是成功报告上的评审状态。

#### Scenario: PG直接写入约束

- **WHEN** 绕repository插入非法owner/周期/跨板块候选的报告
- **THEN** PostgreSQL拒绝，不仅依赖前端/SQLite校验

#### Scenario: 报告完成直接可读

- **WHEN** 第一份或后续版本校验入库
- **THEN** job成功并可读，无review行/调用/徽标/采纳步骤；当前正在阅读的旧版本不会被后台完成强切

#### Scenario: 旧数据与新版并存

- **WHEN** 迁移旧库并对候选显式重新研究
- **THEN** 旧schema2/旧kind和候选快照不变，新schema3只追加；按result_id可读独立版本，旧客户端有兼容text，不要求批量重生成

### Requirement: 失败恢复与周期查询

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


发现和研究仍为202异步，job区分kind/phase/outcome及候选/批次；失败保留已完成freshness与候选。job重启404 SHALL 停轮询，取持久候选/报告和终态进展，不自动续跑付费研究。列表按board/粒度/周期与id游标分页，默认20最大100，不能混旧kind。

候选查询新增显式visibility筛选（visible/ignored/all）；省略时沿用旧客户端all语义，新工作台显式用visible。新view筛选all/pending/readable的语义分别为全部、无报告且无live研究、存在成功报告（可同时正在重研）；省略新筛选时保持旧调用结果语义，不另造不存在的旧status兼容规则。计数只知道已加载页时必须标「已加载N条」。切周期、筛选、版块、候选或版本都是只读浏览，不改job身份或冻结材料；晚响应不能串台，当前board任务可独立返回。

#### Scenario: 研究失败可重试

- **WHEN** 研究超时或成文不合格耗尽
- **THEN** job错误，无新result，候选仍在且可用户手动重试；不自动恢复付费研究；错误信息携带进展摘要（已完成轮次/已取得观测数），可查完整进展账本

#### Scenario: 重启恢复

- **WHEN** job查询因后端重启404
- **THEN** 停止轮询说明状态不可恢复，重新获取持久候选/报告；候选不会永久研究中

#### Scenario: 周期分页

- **WHEN** 切换月/年/period或翻页
- **THEN** 只返回该board/周期候选与报告，保持旧页记录，不按created_at推断归属期

#### Scenario: 忽略筛选与分页兼容

- **WHEN** 新客户端使用visible/ignored筛选翻页，或旧客户端省略新参数
- **THEN** 新视图按处理标记返回该scope记录；旧调用仍含原候选；游标不漏跨页原报告，不用一页统计冒充总量

#### Scenario: 运行中仍能浏览

- **WHEN** 一候选研究中，用户翻到历史周期或其他报告再返回任务
- **THEN** 原任务不取消/重启，当前浏览不调用LLM，能返回原周期/候选看进展，晚响应不覆盖另一个scope

### Requirement: 研究工作台主从导航

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


系统 SHALL 在既有版块入口呈现研究工作台，候选索引与问题/进展/报告详情分工明确；同一候选只占一条索引，报告版本归其下，不另在长列表底部重复排列。窄屏为列表与详情单栏切换，返回保留选择和滚动。只读导航不触发发现、研究或成文；新研究开关关闭仍可读历史与背景。

#### Scenario: 索引切换与返回

- **WHEN** 用户打开一条报告、换到另一条、再返回，或窄屏返回索引
- **THEN** 保留对应阅读位置与列表选择，不重新研究；标题全文在详情可读，不硬裁证据

#### Scenario: 关闭开关仍可阅读

- **WHEN** 增强开关关闭且已有报告或旧结果
- **THEN** 读取可用，新发现/研究按钮禁用并说明，不因查看要求配置或调用模型

#### Scenario: 加载失败不是空态

- **WHEN** 候选或详情请求失败、加载中或合法返回空集
- **THEN** 分别显示error/loading/empty和相应恢复入口；重试只读，未返回的数字不伪造0

### Requirement: 候选忽略与恢复

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


系统 SHALL 持久保存独立于候选内容和报告的忽略标记。非运行候选可忽略，正常索引不显示；已忽略视图可读原报告并恢复。正在研究或成文的候选不能忽略，前端禁用/说明，服务端409；重复忽略或恢复幂等。无效输入400、跨板块/不存在404，失败不部分改候选或结果。不永久屏蔽话题或后续批次。

#### Scenario: 忽略后重进与恢复

- **WHEN** 用户忽略未研究或已有报告候选，刷新/重启后到已忽略视图恢复
- **THEN** 标记保留并可恢复，原内容/报告/依据不变，所有动作零LLM，其他新批次不受影响

#### Scenario: 运行中的忽略不可达

- **WHEN** 候选处于research/compose，或并发直接请求忽略
- **THEN** 按钮禁用并有原因，API409且无标记变更、无取消任务；完成/失败后可再忽略

#### Scenario: 处理写入故障与越权

- **WHEN** 请求缺ignored布尔、用字符串布尔、无效ID、跨板块或保存失败
- **THEN** 给400/404或明确错误；UI回滚保留现场，不删原始数据、不复用另一scope的晚响应

### Requirement: 新闻依据话题关联

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


相关话题 SHALL 只由候选冻结新闻依据及可验证的归属元信息解析，可为一条、多条或空；不按标题相似度猜关联，不只限今天active话题。原始归属无法恢复、话题删除或不在允许board时显示缺口，不把当前最新摘要替换研究快照。查看关联不调用模型，已忽略候选同样可查看。

#### Scenario: 多话题和历史话题关联

- **WHEN** 候选的合法依据属于多个话题或含已归档话题
- **THEN** 返回可验证关联和对应依据；可只读打开其背景，不强制建研究项目或启动聚焦分析

#### Scenario: 无法验证的归属

- **WHEN** 旧快照缺归属字段、依据消失或出现跨板块topic
- **THEN** 显示未关联/不可恢复原因，不猜回填、不泄露另一board的背景、不触发AI修复

### Requirement: 工作台展示字段来源

<!-- from: board-signal-reports (2026-09-26 board-signal-reports) -->


候选标题/原因/问题/发现时间 SHALL 来自保存数据；pending/researching/reported来自任务和成功结果，忽略来自用户标记。报告标题/生成时间/目标期/版本来自对应不可变记录；研究时点来自cutoff，调用/计算/轮次仅来自实际记录，缺失为未记录而不是0。过程计数在次级区，不作为质量分；候选内部score不改称可信度。仅新增只读字段不能重写原快照。

#### Scenario: 展示不发明状态和质量

- **WHEN** 模型文案声称已验证或缺失generation_meta，任务/报告记录与其不符
- **THEN** UI以系统记录为准，未知计数显示未记录，不展示模型自报质量或评审徽标

#### Scenario: 版本选择按真实结果

- **WHEN** 同候选有多个成功result，用户读旧版时新报告完成
- **THEN** 可按真实result_id切换，各自时间/标题/附录一致，不覆盖或强切旧版，选择本身零LLM

### Requirement: 版块简报手动触发

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


系统 SHALL 以单个语义版块为对象手动生成版块简报，MUST 复用版块 `enrichment_enabled` 门槛。本期 SHALL NOT 定时生成。默认触发只读取内部态势卡与历史认知记录，MUST NOT 自动调用外部搜索，也 MUST NOT 自动继续生成深度调查。

#### Scenario: 版块未开启增强时拒绝

- **WHEN** 版块 `enrichment_enabled=false` 时用户触发版块简报
- **THEN** 请求被拒绝且不产生任何结果

#### Scenario: 默认触发只生成简报

- **WHEN** 用户点击“分析板块”
- **THEN** 系统生成一份 `board_brief` 快照，不调用 web 搜索、不产生 `board_investigation`，并向用户展示可继续调查的问题

#### Scenario: 多次触发保留独立简报

- **WHEN** 用户对同一版块再次生成简报
- **THEN** 新旧简报均作为不可变快照保留，新简报不覆盖旧简报

### Requirement: 简报与调查异步任务可区分

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


版块简报和调查 SHALL 脱离触发 HTTP 请求在后台运行，每次触发 SHALL 返回唯一 job id、job kind 与 board id，前端 SHALL 可按 job id 轮询 running/finished/error/result_id。同一版块的简报与调查 MUST 串行：已有任一任务运行时，新触发返回 409 并携带当前任务身份；不同版块可并行。客户端断连 MUST NOT 取消后台任务。

#### Scenario: 简报触发立即返回

- **WHEN** 用户触发简报后立即离开页面
- **THEN** 接口返回 `job_kind=board_brief` 的任务身份，后台继续完成，用户重进页面可恢复该 job 的轮询

#### Scenario: 调查状态不冒充简报状态

- **WHEN** 用户触发某问题调查
- **THEN** 状态返回 `job_kind=board_investigation` 与独立 job id，完成后 result_id 指向调查子结果，前端不把它当作新简报

#### Scenario: 同版块任务防重入

- **WHEN** 同一版块已有简报或调查运行时再次触发任一种任务
- **THEN** 新任务不启动，接口返回 409 及当前 job id/job kind，前端恢复当前任务轮询

### Requirement: 简报反映观察而非强制立论

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


版块简报 SHALL 包含：`summary`、`observations`、`relationships`、`uncertainties`、`research_questions`、`lane_refs`。简报 MUST NOT 强制包含 thesis、固定反转句式、机制层、历史类比或系统重定位。每条 observation SHALL 指向相关泳道并标明依据或截止时间。

#### Scenario: 有素材但没有统一关系

- **WHEN** 多条泳道均有近期素材，但没有足够证据证明共同驱动或因果传导
- **THEN** 简报照常生成关键观察，并明确写“暂未发现统一关系”或等价结论，不硬造跨泳道命题，也不降级为 sparse

#### Scenario: 存在多个并行趋势

- **WHEN** 板块内同时存在方向不同且互不从属的变化
- **THEN** 简报分别展示这些趋势及其分化关系，不把它们压缩成单一底层原因

#### Scenario: 全部素材稀薄

- **WHEN** 所有活跃泳道均缺少可用近期观察
- **THEN** 简报诚实展示素材不足，不生成研究问题，不自动启动调查

### Requirement: 泳道关系有类型、有依据、可为空

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


系统 SHALL 仅在内部素材支持时声明跨泳道关系。关系类型 SHALL 为 `common_driver | possible_causal | divergent | context_only | unclear`；每条关系 SHALL 带相关 lane id、解释、置信度与观察依据。同期发生 MUST NOT 自动等同于因果关系。

#### Scenario: 仅语义相关

- **WHEN** 两条泳道只因属于同一概念版块而相关，缺少共同驱动或传导证据
- **THEN** 系统标为 `context_only` 或不建立关系，MUST NOT 标为 `possible_causal`

#### Scenario: 因果证据不足

- **WHEN** 素材显示两条泳道可能存在传导但中间环节缺失
- **THEN** 系统最多标为 `possible_causal` 且低/中置信，并把缺失环节写入 uncertainties

#### Scenario: 关系引用幽灵泳道

- **WHEN** LLM 返回不属于当前版块活跃集合的 lane id
- **THEN** 该关系与引用被拒绝或清理，其余合法内容继续保留

### Requirement: 简报提供可选择的研究问题

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


系统 SHALL 从观察、关系与未知项中生成 0-4 个具体问题式研究候选；每个候选 SHALL 包含问题、为什么值得调查及相关泳道。问题 MUST 可由证据支持或削弱，MUST NOT 只是抽象方法名。用户 MAY 自填问题替代候选。

#### Scenario: 用户选择候选问题

- **WHEN** 用户在简报中选择一个研究问题并点击“深入调查”
- **THEN** 调查以该问题及父简报为输入启动，不要求用户先接受任何预设命题

#### Scenario: 用户自填问题

- **WHEN** 用户输入一个自定义问题并触发调查
- **THEN** 系统记录问题来源为 custom，并按与生成问题相同的调查链处理

#### Scenario: 没有值得调查的问题

- **WHEN** 简报只发现普通、彼此独立的变化且无关键未知项
- **THEN** `research_questions` 可为空，系统不把空列表视为失败

### Requirement: 深度调查必须评估竞争假设

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


调查 SHALL 在检索前生成 2-4 个竞争假设，且 MUST 至少包含一个 `is_null=true` 的零假设，用于表达“没有统一机制、各变化可分别解释”或与问题等价的朴素解释。每个假设 SHALL 先声明支持它需要什么证据、什么证据会削弱它以及适用范围；此阶段 MUST NOT 预先选定赢家。

#### Scenario: 零假设不可缺席

- **WHEN** 调查问题涉及多个泳道是否存在共同机制
- **THEN** 假设集合中至少有一个零假设；缺少零假设的 LLM 输出不得直接进入研究阶段

#### Scenario: 候选假设都很宏大

- **WHEN** LLM 只返回多个宏大结构解释而没有普通解释
- **THEN** 系统要求重试或机械补入零假设，MUST NOT 在原集合上直接研究

### Requirement: 调研同时寻找支持与反证

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


研究阶段 SHALL 使用内部泳道工具与可用外部工具，围绕全部假设统一搜集基础事实、支持证据、反证、替代解释与关键缺口。对于每个非零假设，系统 SHALL 至少尝试一次削弱该假设的检索或核查；工具不可用或无命中时 SHALL 记录 gap，不得伪造证据。证据质量与问题相关性优先于证据类型数量。

#### Scenario: 搜索词不得只有既定结论

- **WHEN** 研究 agent 生成检索计划
- **THEN** 计划同时包含中性事实查询和反证/替代解释查询，MUST NOT 只把某一假设的结论词拼进全部搜索词

#### Scenario: 只有同向材料

- **WHEN** 检索只获得支持某假设的新闻转述，未获得一手材料或反证
- **THEN** 该假设不得标为高置信 supported，结果明确记录证据偏向与缺口

#### Scenario: 外部工具不可用

- **WHEN** web_search 或 fetch_page 未配置或失败
- **THEN** 调查继续使用内部材料并标记外部核查缺口，不影响父简报

### Requirement: 调查结论可改写、拆分或放弃假设

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


最终综合 SHALL 根据研究结果为每个假设标记 `supported | plausible | insufficient | weakened | refuted`，并产出带 confidence、scope 与 boundary 的有限结论。综合阶段 MAY 修改、合并或拆分初始假设，也 MAY 判定所有非零假设证据不足。系统 MUST NOT 强制输出反转句式、历史类比、固定机制层数或宏大系统结论。

#### Scenario: 零假设最符合材料

- **WHEN** 研究只证明多条变化同期出现，未找到统一传导机制
- **THEN** 零假设可成为最可信解释，结论用直白语言说明各变化目前应分别理解

#### Scenario: 所有假设证据不足

- **WHEN** 支持与反证均不足以区分候选假设
- **THEN** 调查结论标为低置信/证据不足，列出下一步需要的材料，不强选赢家

#### Scenario: 初始假设被研究推翻

- **WHEN** 反证直接否定初始最显眼的解释
- **THEN** 最终结果将其标为 weakened/refuted，并允许结论与初始候选方向不同

### Requirement: 调查综合只修复可证明的单一根终止符

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


调查综合 SHALL 先按严格 JSON 解析。仅当输出已包含完整顶层末字段 `lane_refs`，词法扫描确认字符串与全部内部对象/数组均闭合且无错配、delimiter stack 恰好只剩根 `{`、最后有效字符为 `]` 时，系统 MAY 追加恰好一个 `}` 后继续同一套 schema 与证据校验，并 SHALL 在 generation meta 记录稳定修复原因。任何内部截断、字符串未闭合、括号错配、尾随正文或缺顶层末字段的输出 MUST NOT 被机械补全，继续既有纠错重试；两次仍失败不得落调查行。

#### Scenario: 完整综合只缺根对象右大括号

- **WHEN** `hypotheses`、`conclusion`、`evidence_chain` 与 `lane_refs` 均完整，字符串和内部括号均闭合，但响应只缺最外层最后一个 `}`
- **THEN** 系统只追加该单一终止符，记录修复原因并继续严格结构校验，无需浪费一次 LLM 重试

#### Scenario: 综合响应存在内部截断或括号错配

- **WHEN** 响应停在字符串/内部数组中、存在括号错配、带尾随正文，或尚未产出 `lane_refs`
- **THEN** 系统拒绝机械补全并进入纠错重试；第二次仍非法时保持 0 调查行

### Requirement: 跨 provider 证据字段归一不得产生悬空评估

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


lane 类型证据的规范持久字段 SHALL 为十进制字符串 `ref`。当且仅当 `ref` 为空时，parser MAY 将正整数且位于 JSON 安全整数域的 `lane_id` 归一为 `ref`，随后 MUST 执行相同的父简报 lane 白名单校验；显式 `ref` 与 `lane_id` 冲突时 SHALL 以 `ref` 为准且不得用别名掩盖非法值。证据清洗后，hypothesis 引用 SHALL 与存活 evidence 的 `supports/counters` 极性确定性合并；`supported` MUST 至少保留一条支持证据，`refuted` 或 `weakened` MUST 至少保留一条反证。违反该一致性 SHALL 进入结构纠错重试，第二次仍失败不得落库。

#### Scenario: fallback 使用 lane_id 表示合法泳道证据

- **WHEN** lane evidence 没有 `ref`、提供白名单内的安全正整数 `lane_id`，且 supports/counters 指向有效假设
- **THEN** 系统将其归一为字符串 `ref`，保留证据及双向假设引用，前端可展示证据并下钻该泳道

#### Scenario: 显式非法 ref 不被 lane_id 掩盖

- **WHEN** lane evidence 同时提供白名单外或非法 `ref` 与白名单内 `lane_id`
- **THEN** 系统以显式 `ref` 为准并剔除该证据，不用别名偷偷改写其身份

#### Scenario: 清洗后确定性评估失去全部对应证据

- **WHEN** `supported` 的支持引用或 `refuted/weakened` 的反证引用在白名单、同源或极性清洗后归零
- **THEN** 该综合视为结构非法并纠错重试，MUST NOT 持久化“没有证据但已证实/已推翻”的调查

#### Scenario: 零证据报告不猜测研究过程

- **WHEN** 合法调查最终没有通过核验、可展示的 evidence
- **THEN** 前端使用中性空态文案，不断言“没有采到材料”或虚构具体失败原因

### Requirement: 调查产物可核查且易读

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


`board_investigation` SHALL 包含父简报 id、问题、hypotheses、conclusion、evidence_chain、lane_refs 与 method_refs。报告首屏 SHALL 先展示问题、当前判断、置信度、适用范围和边界；支持证据、反证、缺口与来源详情 SHALL 可展开查看。抽象概念首次出现时 SHALL 说明其对应的具体行为、规则、数据或事件。

#### Scenario: 支持与反证分开显示

- **WHEN** 用户展开某个假设
- **THEN** 前端分别展示 support_evidence、counter_evidence 和 gaps，不把相反材料埋入同一段长文

#### Scenario: 避免重复机制长文

- **WHEN** 调查包含多个假设和结论
- **THEN** 前端不再连续重复渲染 argument.layers 与 depth.mechanism_layers；同一论点只在一个主位置表达

### Requirement: 简报与调查形成父子不可变快照

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


每份调查 SHALL 关联同版块的一份 `board_brief`，并持久化由规范化问题文本生成的 `question_key`；一份简报 MAY 派生多份调查。简报与调查均 append-only，不得因后续调查或追问被改写；追问继续以附属记录沉淀。

#### Scenario: 一份简报派生多份调查

- **WHEN** 用户分别调查同一简报中的两个问题
- **THEN** 系统产生两份独立 `board_investigation`，二者 parent 均指向该简报，互不覆盖

#### Scenario: 跨版块父结果被拒绝

- **WHEN** 调查请求引用另一个版块的简报 id
- **THEN** 请求被拒绝，不产生调查结果

### Requirement: 态势卡素材装配

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


版块级分析的素材注入 SHALL 以每泳道「态势卡」形态压缩（事实摘要 + 命中统计 + 质量信号），MUST NOT 将全部泳道的各粒度背景记忆全文拼接注入（防 token 爆炸）。态势卡事实摘要取材链 SHALL 为：**week 档 lifeline 摘要 → month 档 lifeline 摘要 → 近期 section 事实指纹 → 泳道描述 → 空**。month 档 SHALL 作为 week 缺失时的主要事实源（与循环 A 的实际覆盖形态对齐——month 由月度定时任务全量维护，week 依赖周任务逐周积累）；新鲜度门补齐的 month 档 SHALL 被本取材链消费（补齐成果不得白费）。section 指纹降级 MUST 携带 section 级实质事实内容（摘要/事实指纹），MUST NOT 输出仅含泳道名与篇数的同义反复。卡片事实来源（facts_source：lifeline_week | lifeline_month | section_fingerprint | description | none）SHALL 可追溯，供素材质量排查与效果核对。质量信号中的信息密度 SHALL 反映背景记忆丰满度（含 month 档可用性），MUST NOT 仅由文章篇数决定。

agent 需要某泳道论据细节时经泳道详情工具下钻获取（工具的素材可见范围见 `data-enrichment` 的「探索 agent 工具集」）。

#### Scenario: 低质量泳道不被静默删除

- **WHEN** 某泳道长期无命中或历史稀疏
- **THEN** 其卡片降权和缩短但仍可追溯，系统不得仅因质量低便从版块成员中删除

#### Scenario: week 缺失时 month 兜底

- **WHEN** 泳道无 week 档 lifeline 但存在 month 档（生产库常态形态）
- **THEN** 态势卡事实摘要取自 month 档（卡片标注来源为 month），不降级为 section 指纹

#### Scenario: 指纹降级有实质内容

- **WHEN** 泳道无任何 lifeline 档、仅有 daily_report section 可用
- **THEN** 指纹携带 section 级事实内容（摘要/事实指纹），不是「泳道名 (N篇)」形态；确无可读内容时如实标注空，不编造

#### Scenario: 泳道多时素材仍可控

- **WHEN** 板块活跃泳道数量较多（如 ≥10 条）
- **THEN** 分析照常执行，注入素材总量受态势卡形态约束，不因泳道数量线性膨胀为全文拼接

#### Scenario: 无任何素材时卡片仍产出

- **WHEN** 泳道既无 lifeline 记录也无可用 section
- **THEN** 卡片仅含命中统计 + 质量信号并标注无事实源，装配不报错、该泳道不静默消失

### Requirement: lane 引用与单泳道下钻

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


旧简报/调查中的lane引用 SHALL 保留并校验允许归属。新研究工作台和旧只读历史中，下钻打开对应话题的事实脉络/背景，不再预填或启动已退役的聚焦生成入口。原后端旧分析API与载荷字段保持兼容；不存在/无法验证的lane显示失效，不猜测替代话题，不把版块判断锁成一条泳道。

#### Scenario: 从简报观察下钻

- **WHEN** 用户点击旧简报observation中的合法lane引用
- **THEN** 进入相应话题只读资料/背景，不打开聚焦表单、不发起分析，原observation仍可读

#### Scenario: 从调查证据下钻

- **WHEN** 用户点击旧假设证据中的lane引用
- **THEN** 打开可验证话题上下文，无法验证时提示失效；不自动预填并执行旧lens分析

### Requirement: review 按结果种类隔离

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


新简报 SHALL 只与同版块上一份简报比较，记录观察、关系和不确定性的变化；调查只有在 `parent_result_id + question_key` 相同的重跑之间才自动比较。review SHALL NOT 在不同问题之间比较，也 SHALL NOT 回写新闻记忆。

#### Scenario: 第二份简报自动对比

- **WHEN** 同版块生成第二份 `board_brief`
- **THEN** review 对比两份简报的观察与关系变化，不读取 legacy thesis 作为上一份

#### Scenario: 不同问题调查不互比

- **WHEN** 同一简报下先后完成两个不同问题的调查
- **THEN** 系统不自动把二者作为前后认知版本比较

### Requirement: 旧论文式版块报告兼容

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


既有thesis/argument/depth形态版块结果 SHALL 标记legacy_board_analysis并保持只读可访问；旧board_brief/board_investigation、topic分析及其辩论/问答也有各自原载荷兼容阅读。新工作台不再提供旧形态生成入口，新signal研究不生成这些旧kind，任何新触发仍不得重新生成已退役的legacy_board_analysis形态。旧后端显式调用API保持其既有兼容行为；迁移不重写或删除旧JSON，不要求旧结果满足schema3。

#### Scenario: 查看旧报告

- **WHEN** 用户从只读历史列表打开旧版结果
- **THEN** 以其原kind兼容展示并标明旧版/只读；缺失字段如实标注，不能把旧JSON送模型改写成新报告

### Requirement: 版块分析工作台信息架构

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


版块研究工作台 SHALL 使用候选索引与详情的主从结构；退役聚焦分析、金融辩论及旧报告新增QA入口，不保留一个无用的泳道下拉来承接新闻背景。新闻背景在话题上下文独立可达，读取/编辑/显式生成与分析判断隔离。旧产物在只读历史入口，不把它们混入新signal_report列表或自动挂旧轮询/触发。新UI与话题总览共用既有导航和身份，不改变生命周期或关键事件视图。

#### Scenario: 单一下拉选择泳道

- **WHEN** 用户从新工作台选择研究或查看背景
- **THEN** 工作台不存在旧聚焦泳道选择器；关联入口带明确topic，话题背景自己持有上下文，不存在两处共享旧selectedTopicId

#### Scenario: 新闻背景入口保留

- **WHEN** 用户需要查看、翻阅或编辑某话题新闻背景
- **THEN** 在话题上下文完成既有操作，从研究可读预览；不要求选择或启动聚焦分析，研究判断不回写记忆

### Requirement: 分析触发异步化

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


循环 B 分析（板块档与单泳道档）SHALL 以异步方式触发：触发接口立即返回（含已在跑的冲突码），分析在脱离 HTTP 请求生命周期的后台上下文中执行并照常落库。分析执行 MUST NOT 因客户端断开（离开页面/关标签页/网络抖动）而中止——同步 HTTP 下 request-context 取消会作废整次已跑的分析（含补全门备料）。同目标重复触发 SHALL 被拒绝；分析完成/失败状态 SHALL 可被前端轮询，完成后可拿到落库结果 id。未开启增强的板块 SHALL 在触发时同步拒绝（4xx 语义保留）。

#### Scenario: 触发立即返回且断连不中止

- **WHEN** 用户触发板块/单泳道分析后在完成前离开页面（或网络断开）
- **THEN** 触发接口立即返回「已开始」，后台分析照常跑完并落库；用户回界面轮询状态可拿到结果 id

#### Scenario: 同目标防重入

- **WHEN** 同一板块/泳道已有分析在后台执行时再次触发
- **THEN** 触发接口返回冲突（409），不启动第二个并行分析；跑完后可再次触发

#### Scenario: 分析状态可轮询

- **WHEN** 前端在触发后或进入面板时查询分析状态
- **THEN** 状态接口返回 running/finished/error/result_id；从未触发或进程重启后视为空闲（running=false），不拖垮后续触发

### Requirement: 版块调查可引用经授权的跨版块泳道

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->

每份版块调查仍 SHALL 关联发起版块的一份不可变 `board_brief`，但研究循环 MAY 读取并引用本次会话动态授权的其他版块泳道。跨版块 lane reference SHALL 同时携带 lane ID 和所属 board ID；综合阶段 MUST 剔除未授权、已删除或无法确认归属的引用。

#### Scenario: 调查引用其他版块泳道
- **WHEN** 研究循环通过内部搜索发现并读取另一个版块的授权泳道，且该材料与问题相关
- **THEN** 调查报告可以引用该泳道，并显示其所属版块和证据用途

#### Scenario: 父简报归属保持不变
- **WHEN** 调查引用了多个其他版块泳道
- **THEN** 调查仍归属于原始 board 和 parent brief，MUST NOT 改写父简报的版块归属

#### Scenario: 未授权跨版块引用被剔除
- **WHEN** 综合输出包含未在本次会话授权记录中的跨版块 lane reference
- **THEN** 系统机械剔除该引用，且不得把它用于支持调查结论

### Requirement: 简报消费已确认的跨版块关系

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->

生成新版块简报前，系统 SHALL 查询与该版块相关、状态为 `confirmed` 且未过期的跨版块关系，在确定性数量和字符预算内作为“已确认外部关系背景”注入。版块简报本身 MUST NOT 因此主动调用联网工具；选中的关系及证据引用 SHALL 冻结到本次输入快照。原有 `relationships` 继续表示本版块态势卡之间的当次关系，跨版块关系 SHALL 以独立字段返回，MUST NOT 混入本版块 lane ID 校验集合。

#### Scenario: 新简报消费确认关系
- **WHEN** 版块存在仍有效的 `confirmed` 跨版块关系并生成新简报
- **THEN** 系统在预算内注入该关系，结果以独立跨版块关系字段返回，并可追溯关系建议 ID

#### Scenario: 未确认关系不进入简报
- **WHEN** 与版块相关的关系仅为 `unresolved`、`proposed`、`dismissed` 或 `expired`
- **THEN** 这些关系均不进入简报 prompt、输入快照或跨版块关系字段

#### Scenario: 简报生成期间不联网
- **WHEN** 简报消费已确认关系
- **THEN** 简报仍只执行原有单次生成调用，不调用 `web_search` 或 `fetch_page`

#### Scenario: 旧简报保持不可变
- **WHEN** 用户在一份简报生成后确认或撤销跨版块关系
- **THEN** 已有简报不被改写，关系变更仅影响之后生成的简报

#### Scenario: 关系数量超过预算
- **WHEN** 有效确认关系超过简报注入上限
- **THEN** 系统按确定性排序选择预算内关系并记录截断数量，MUST NOT 随数据库返回顺序漂移

### Requirement: 旧研究历史只读访问

<!-- from: board-level-analysis (2026-08-31 board-level-deep-analysis) -->


系统 SHALL 提供按board可验证归属、kind/topic及id游标分页的旧研究列表与详情，默认20最大100；包含可验证归属的已归档话题，不只查询今天active泳道。按原结果可读取旧版板块分析、简报/调查、话题聚焦分析及其已有辩论、QA、旧review记录；新signal_report仍走独立列表。所有历史读取零LLM，无新增触发/追问/沉淀/采纳控件，不通过批量逐topic请求全部数据构造首屏。

跨board/不存在拒绝，无法恢复归属不得猜回填；单条损坏或未知旧载荷给可定位的兼容错误，其他列表仍可浏览。旧API仍兼容，不删除数据或自动重建。新研究开关关闭不阻止只读访问。

#### Scenario: 旧结果与所有子产物可读

- **WHEN** 用户分页打开旧board或已归档topic结果及其辩论/问答/旧review
- **THEN** 原kind内容与来源可读，没有旧生成/追问/沉淀/采纳按钮；不触发模型，signal_report不混入历史集合

#### Scenario: 历史归属与损坏隔离

- **WHEN** 请求另一board结果或子产物，或单旧载荷损坏/缺归属
- **THEN** 跨域/不存在404，不猜归属或泄露子产物；单条错误不使全列表变空，不改旧JSON

#### Scenario: 历史分页与开关无关

- **WHEN** 旧结果超过一页且增强开关关闭
- **THEN** 按scope/游标继续加载，列表与详情独立错误恢复，读取仍可用且不制造新任务
