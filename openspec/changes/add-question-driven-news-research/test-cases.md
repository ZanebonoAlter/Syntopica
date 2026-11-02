# 测试故事与继承账本

> 本轮为规划，所有测试/真实模型/浏览器验收待执行，不将规格校验当作产品测试。一个Requirement一个故事；以下每行对应可执行断言片段，跨故事主旅程为S2→S3→S4→S5→S6→S9。先复用已有文件，仅新增 `signal_sources_test.go` 与 `signal_page_fetch_test.go`（service目录）。测试fixture在临时PG或内存，不写真库、不引SQLite。

## 故事 S1：先得到可重放材料，再决定研究

锚：`board-signal-reports` / Requirement「周期材料与固定候选快照」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 新协议detect已选出候选且其有效切片有关联存文 | 新发现冻结有限原文 | 仅在已选依据范围内保存有界正文快照与article/section关联、来源时间和截取说明，不逐篇调用模型、不抓网页；正文不进入detect输入；候选后续研究使用此版本 | service + 临时PG |
| 2 | 发现时原文缺失、超预算未收录，或发现后原文被修改/删除 | 原文缺失与后改不偷换 | 缺失/未收录记gap；已冻结正文不随数据库变化，研究不隐式抓取最新正文补齐；获取新资料须遵守外部来源边界 | service + 临时PG |
| 3 | 旧候选缺少新协议标记或用户对旧候选重新研究 | 旧候选不自动升级 | 沿用旧快照及原工具授权，不重读今天文章填入历史；工作台提示重新发现才能使用新协议；未知协议版本拒绝研究而不默认放开工具 | service + 临时PG |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：周期校验、历史材料隔离、迟些点击仍用原快照、外部数据边界、只冻结合窗人工关注、后续撤销不改旧研究、失效依据不升级为有效信号、事后选择不冒充当时预测。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_material_test.go；backend-go/internal/dataenrichment/service/signal_sources_test.go；backend-go/internal/dataenrichment/repository/signal_candidate_test.go。

变体重点：时间/范围/快照与版本（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S2：无需专业方法也能发现值得理解的问题

锚：`board-signal-reports` / Requirement「信号发现与人工研究边界」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 至少一条有依据的信号score≥6 | 达标只生成候选 | 保存候选并返回discovered，数据源/计算/成文调用均为0，无报告result；等待用户点击 | 函数/service + handler |
| 2 | 聚合材料呈现政策改变或解释分歧而非市场异动 | 普通读者的问题推荐 | 可生成一个中心问题及非空未知/核查方向；不强加油价预测，不为了跨泳道而拼接无关线索 | 函数/service + handler |
| 3 | 材料只有已交代清楚的事实或无法说明研究增益 | 问题无价值时不凑数 | 允许零候选，不要求每泳道产出、不固定三条；浏览与日报到达均不自动调用发现或研究 | 函数/service + handler |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：零信号零打扰、检测故障可见、依据白名单。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_detect_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go。

变体重点：输入/空态/手动边界（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S3：刷新后仍能看到原问题与核查方向

锚：`board-signal-reports` / Requirement「候选持久化与展示来源」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 渲染候选或报告索引 | 列表字段可追溯 | 原因/问题/时间/任务状态/版本/取数次数来自保存字段、实际任务和账本；忽略来自用户处理标记，不让模型声称已研究或已评审 | 临时PG + handler + Vitest |
| 2 | 读取新旧候选或模型返回空白unknown、错误类型verification | 新旧简要说明与非法输出 | 新候选展示保存的未知/核查方向，旧候选不伪造字段；非法新输出按detect重试上限处理，失败不存半批；score仍不显示为可信度 | 临时PG + handler + Vitest |
| 3 | 候选元数据或原文快照的数据库读写发生系统错误 | 来源冻结保存失败 | 整批failed且不保存半批候选；正文业务性缺失可记gap，数据库故障不得伪装缺失 | 临时PG + handler + Vitest |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：刷新与重启保留候选、重复发现不覆盖。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/repository/signal_candidate_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go；front/app/features/tags/components/SignalCandidateList.test.ts。

变体重点：输入/原子保存/新旧字段（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S4：点一次研究，按原题核实并有界收束

锚：`board-signal-reports` / Requirement「四十轮问题驱动研究」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 第17轮完成研究且未超预算 | 十几轮查询正常完成 | 正常进入成文，不因旧4次/8轮限制提前截停 | service + handler |
| 2 | 第40轮或第40次执行完成且还想继续 | 达到四十轮收束 | 不再执行第41次，stop_reason=budget_exhausted，以已有证据成文并披露影响结论的缺口 | service + handler |
| 3 | 模型持续发重复参数或未授权动作 | 重复与非法动作有限结束 | 不执行，但每次消耗轮次，最多40轮结束，不无限循环 | service + handler |
| 4 | 研究进行到第N轮时job超时/取消/失败 | 超时后进展不白跑 | 进展表保留前N轮的轮次计数与全量账本（调用/观测/计算/缺口），候选可查上次进展与失败原因，可人工重试；重启后进展仍可查 | service + handler |
| 5 | 新协议已读取冻结原文，问题仅需澄清报道口径且已有证据足够 | 原文已足够则早停 | 可直接finish并成文，外搜/网页/无关数值调用为0，不升级成全球影响预测 | service + handler |
| 6 | 有冻结正文且模型先请求外搜、数值或calculate | 先核前提再补证 | 不执行，提示先读允许的source_id；完成至少一次有效正文读取后可按问题补证；不存在可读正文则如实记gap而非死锁 | service + handler |
| 7 | 新研究外搜未配置、请求失败或已用尽3次搜索/5次网页读取 | 网页预算与配置失败 | 失败有类型和原因且计实际调用预算；到上限拒绝额外请求，保留本地解释/未解决出口，不切换付费供应商、不重复相同请求 | service + handler |
| 8 | 必要的政策/部署/控制证据不可得但能取得价格 | 不把工具可用性当问题答案 | 输出主问题尚未回答，价格最多作有限背景，不据缺证据推断无影响；未请求且不相关的数据源不调用 | service + handler |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：空手源提示早停、进程重启收敛孤儿进展、工具失败和任务超时不同、成功后进展归档。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/dataenrichment/service/signal_sources_test.go；backend-go/internal/dataenrichment/handler/signal_research_test.go。

变体重点：预算/先读/失败/手动重试（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S5：读懂结论也读懂哪些没有回答

锚：`board-signal-reports` / Requirement「报告论证与条件判断」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 新稿缺论证角色/条件字段、含未知块或可见文案恰3000字符 | 结构与篇幅校验 | 校验错误回注；连续三次失败attempts=3/retries=2，不入库；不能退回任意HTML蒙混通过 | service + Vitest + 真实样本 |
| 2 | 只能回答原题一部分或必要证据不可得 | 缺证据也有可读结果 | 报告标部分回答/暂无法判断并列出未答部分，不把“没有取得证据”写成“没有影响”，任务仍可成功且不生成评审 | service + Vitest + 真实样本 |
| 3 | 新报告标explained或旧报告没有answer | 答题范围不是可信度 | 前者显示已解释与依据而非已证实认证；后者不猜测补标签；无证据的explained或空白reason由结构校验回注 | service + Vitest + 真实样本 |
| 4 | 原文要求忽略研究问题、访问内网或执行代码 | 来源内指令不是系统指令 | 仅作为不可信来源文字；不能改变工具授权、研究问题和预算，报告不执行或渲染指令代码 | service + Vitest + 真实样本 |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：不足以转多也能成文、图表降级、无数字仍可成文、块内引用必须核查。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_compose_test.go；front/app/features/tags/components/SignalReportView.test.ts。

变体重点：结构/长度/证据/诚实答题（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S6：从结论追到当次实际使用的来源

锚：`board-signal-reports` / Requirement「观测与计算引用附录」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 正文或证据块引用c1:o2或k1 | 精确引用与定位 | 显示来自同一观测/计算的阅读值和原单位；点击到原值/公式；原字符串不改，悬空或计算失败的新引用拒绝 | 函数/service + 临时PG + Vitest |
| 2 | 一次研究返回超过50个合格观测或取数失败 | 完整账本 | 全部合格观测/失败记录保留，真实元信息缺失显示null，不伪造成功时间 | 函数/service + 临时PG + Vitest |
| 3 | 正文引用已读取source_id，或将它传给calculate/数值图表 | 文字来源可定位不可代算 | 前者定位到同份冻结文本及元数据，后者拒绝；data/calc原有渲染与核查不变 | 函数/service + 临时PG + Vitest |
| 4 | 成文引用搜索摘要、未读取source_id或别次研究的来源 | 摘要与悬空来源拒绝 | 拒绝并回注，不因URL看似权威就当成有效证据；无法读取记gap，不编造正文 | 函数/service + 临时PG + Vitest |
| 5 | 新研究已读文字来源后超时或成功 | 进展保留文字来源 | 失败进展及成功附录均保存已读来源的实际文本、取回/冻结时间、截取说明与失败记录；旧数字账本不裁减 | 函数/service + 临时PG + Vitest |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：图表口径不混淆、长小数与极小值、真实日期与图表断点。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_compose_test.go；backend-go/internal/dataenrichment/service/signal_sources_test.go；front/app/features/tags/components/reportPresentation.test.ts；backend-go/internal/dataenrichment/repository/signal_research_progress_test.go。

变体重点：引用/截取/进展/数字兼容（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S7：新结果直接可读，旧结果不变

锚：`board-signal-reports` / Requirement「成功快照与无评审输出」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 迁移旧库并对候选显式重新研究 | 旧数据与新版并存 | 旧schema2/旧kind和候选快照不变，新schema3只追加；按result_id可读独立版本，旧客户端有兼容text，不要求批量重生成 | 临时PG + Vitest |
| 2 | 部署后浏览旧报告或对新协议候选重新研究 | 新协议追加而不改历史 | 旧报告字节内容不变，新报告追加并记录本次证据版本；新候选重新研究沿用冻结本地来源与cutoff，不刷新新闻快照；新外部取回版本明确另记取回时间 | 临时PG + Vitest |

> 继承 Scenario（行为未改：照跑原断言，必要时补新协议 fixture，原契约不删；逐条处置见文末「继承与调整」）：PG直接写入约束、报告完成直接可读。
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/repository/signal_report_test.go；backend-go/internal/platform/database/board_signal_migration_test.go；front/app/features/tags/components/SignalReportView.test.ts。

变体重点：版本/不可变/无review（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S8：外部证据可用但不能越权或偷换时点

锚：`board-signal-reports` / Requirement「有界来源访问与时间语义」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 模型请求未登记URL、私网地址、带凭证地址，或公网地址重定向/重新解析到内网 | 公网与重定向安全 | 请求被拒绝，内网服务没有收到请求，缺口可查；合法URL仍受超时/体积/跳数上限 | service纯函数 + 假transport |
| 2 | 外部正文日期晚于cutoff、日期未知，或日期合格但取回晚于cutoff | 来源时间不能冒充研究时点 | 前两者不进入事实材料且不可引用；后者标来源日期与实际取回时间及事后核查，不称当时已知；无日期页面缺口不通过模型猜日期解决 | service纯函数 + 假transport |
| 3 | 本地/网页正文超出声明的文本预算、为空或只有搜索摘要 | 来源截取与空正文 | 超限只保存明确标注范围的有限片段且模型收到同一文本，空正文/摘要不能标已读正文；“该片段没提”不推断完整文档不存在某内容 | service纯函数 + 假transport |
| 4 | 新研究发起搜索或来源文字要求发送完整上下文到其它地址 | 检索出站最小化 | 仅允许限定长度的公开主题query交既有搜索后端；不自动上传原文/私有设置/凭证，不调用来源指定的外传工具 | service纯函数 + 假transport |
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_page_fetch_test.go；backend-go/internal/dataenrichment/service/signal_sources_test.go。

变体重点：安全/时间/来源预算（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S9：从候选到来源只用现有工作台

锚：`board-signal-reports` / Requirement「普通读者研究字段呈现」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 用户打开新候选、显式研究并点击报告的文字来源引用 | 从问题到来源的主链路 | 能看到未知/核查方向、真实运行反馈、答题范围、被引用文本与来源时间；点击到达原核查区且返回后选择不丢，不新增评审步骤 | Vitest + agent-browser |
| 2 | 读取旧字段缺省、长URL、正文缺失、外搜失败或研究运行中状态 | 新旧和异常态可读 | 分别兼容原阅读、折行显示、安全缺口、明确失败原因和禁重复研究；请求失败不伪装空态，无模型生成的假来源链接 | Vitest + agent-browser |
> 落点（本故事全部步骤，含继承）：front/app/features/tags/components/SignalCandidateList.test.ts；front/app/features/tags/components/SignalReportView.test.ts；front/app/features/tags/composables/useSignalWorkbench.test.ts；人工：agent-browser 静态 :5100，证据见 ui-verification/report.md。

变体重点：UI全部状态（答案见「公共变体走查」，通用约束见其前全局注）。

## 故事 S10：仅新协议研究获得额外来源能力

锚：`research-data-sources` / Requirement「工具适配不改现有工具面」。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 |
| --- | --- | --- | --- | --- |
| 1 | 用户点击一条候选的深入分析 | 研究显式授权 | 旧协议获得原七源，新协议另获得已核定的来源检索/阅读；两者均无脚本或任意URL爬行能力 | service + wiring |
| 2 | 任意既有增强/QA/调查/问答发现/关系发现运行，或仅执行新信号发现 | 注入后现有工具面不变 | 既有工具面保持原样，新发现不调用数据源；仅注册工具不等于默认授权 | service + wiring |
| 3 | 新协议读取文字来源或搜索失败 | 来源能力不改变数值工具语义 | 七源参数、官方host、缓存及cutoff过滤不变，来源ID不得成为calculate输入，不新增数值数据源目录条目、不自动切供应商 | service + wiring |
> 落点（本故事全部步骤，含继承）：backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/datasources/wiring/tools_test.go。

变体重点：授权/旧调用不变（答案见「公共变体走查」，通用约束见其前全局注）。

> 全部故事通用：变体不生成全组合，按「适用重点」引用下文 V1～V5 逐项答案；S2/S4/S5/S8 涉及模型与来源效果还须完成 E1～E3，其余为确定性结构/存储/UI 验收，真实效果由该三样本共享，不重复付费运行。

## 公共变体走查（每故事按上文重点引用）

| 编号/组 | 固定条目 | 明确答案与落点 |
| --- | --- | --- |
| V1 输入 | 空串；纯空白（ASCII/全角/tab） | 新brief的unknown/verification、来源工具参数、answer.reason trim后为空拒绝；缺可选旧字段兼容。signal_detect/compose/sources测试 |
| V1 输入 | 纯分隔符（单个/连续/首尾）；单token | 工具query不做表达式拆分；纯标点不含可检索文字则参数拒绝，单个实体如“FEC”接受；URL/ID按类型解析，无“分隔符=全部”逻辑。新brief按非空/长度校验不冒充语义保证 |
| V1 输入 | 大小写 | source_id区分大小写，协议/枚举精确匹配；URL host小写归一，path/query保持原样；搜索query不强改大小写。sources/page测试 |
| V1 输入 | 正则元字符/引号/emoji；超长 | 作为文字不执行，UI转义；brief 600/601rune、query300/301、reason400/401与正文7999/8000/8001边界。detect/compose/sources/前端测试 |
| V2 前置 | 空集；单元素 | 空材料零LLM；一条切片允许一个候选；无正文记gap可外搜；所有来源不可得可unresolved；一份原文能解释则早停。detect/research测试 |
| V2 前置 | 重复；越界/跨域引用 | 同article去重；同URL去fragment、host归一但不删query；未注册URL/非本候选source/跨run拒绝，无源调用；跨board沿用404。sources/handler测试 |
| V2 前置 | 部分满足 | 只读成功正文能支持部分回答；摘要非正文；DB异常整批失败，Bocha未配置记gap、仍允许本地解释。repository/service测试 |
| V3 时间 | 边界两端/当天 | 本地pub_date和外部有效日期= cutoff接受、后一步拒绝；纯日期以业务日结束保守判断，当前未结束日不能冒充已合窗。page/material测试 |
| V3 时间 | 空窗口；跨窗口 | 空窗不发现；新新闻不进入旧候选；历史非active泳道按原归属保留；网站更新晚于cutoff拒绝。material/research测试 |
| V3 时间 | 归一化 | 带offset时间换同一instant，日期采用ShanghaiTZ结束边界；未知/冲突日期不猜测，冲突取较晚；捕获/取回时间独立展示。page测试 |
| V4 幂等 | 重复执行两次 | 同候选默认请求已有结果200零新增调用，regenerate202新增版本；同次tool+参数重复不执行、消耗轮次；来源快照不刷新。handler/research测试 |
| V4 幂等 | 部分失败重试 | 失败进展保留文字与数值账本；仅用户显式重试，不复活自动任务、不覆盖旧报告。本地仍冻结，外部取回新版本单独记时间。progress/research测试 |
| V4 幂等 | 并发 | 只继承现有board锁承诺：研究/忽略竞争仍409，本文不新增并行agent；来源抓取串行无并发安全新承诺。handler旧测试回归 |
| V5 可用性 | 误输入反馈 | 既有周期非法仍400无job、前端显示原因；不新增输入表单。新brief/answer非法是任务错误不静默空态。handler/Vitest |
| V5 可用性 | 空态；错误态 | 零候选保留旧索引；原文缺失/搜索失败明确缺口；整个job失败不伪装“暂无法判断”成功，详情请求失败不显示空列表。Vitest/browser |
| V5 可用性 | 加载态；超长文本；重复提交 | 运行态照常且禁重复、长URL与中文可折行、点击引用目标可见；默认200不启动LLM。Vitest+一次browser主故事 |

不适用划除：没有用户表达式语言，故分隔符不做token组合；没有新日期输入控件、没有新弹层，无新增elementFromPoint弹层断言（若实现产生弹层则补）；没有新增并行研究算法，不做并发状态笛卡尔积。其余条目已给出明确答案。

## 白盒附加：只测新增边界，不重写旧测试网

| 分支 | 输入/边界 | 预期 | 计划用例名（TestQuestion前缀） |
| --- | --- | --- | --- |
| 研究协议 | absent / question-v1 / unknown / malformed brief | 原七源 /新工具 /拒绝 /拒绝；客户端不能覆写 | TestQuestionProtocol |
| 本地候选关联 | 无/1/8/9篇；同article跨两个section；跨board | 合窗去重，有界8篇，9篇记缺口，跨board拒绝；重复不重复拼正文 | TestQuestionSourceSnapshot |
| 快照持久化 | 存文更新/删除；事务第二步失败 | 原文快照不变；失败无半批 | TestQuestionSnapshotAtomicity |
| 原文门 | 存文存在未读 / 已读 / 无存文 | 初始prompt仅目录不含sources正文/其它候选原文；阻挡其它取证 / 放行 / 有gap放行；无证据explained拒绝 | TestQuestionReadFirst |
| 来源类型 | snippet / 本地读成功 / page日期合格 / page日期未知 | 不可引 /可引 /可引且事后标记 /gap不可引 | TestQuestionEvidenceEligibility |
| 时间 | cutoff−1ns /相等 /+1ns；dateOnly；dateModified更晚 | 接受/接受/拒绝；日末判断；取较晚 | TestQuestionSourceTime |
| 预算 | 第3/4次搜索、第5/6次抓页、40/41决策；失败/重复 | 上限内执行，额外拒绝；失败计执行，重复只计决策，最终可unresolved | TestQuestionSourceBudget |
| 网络 | 未登记/私网/重绑定/跳转私网/含userinfo/2MiB+1/超时 | 无危险目标请求，typed gap；保留旧七源host | TestQuestionSafePageFetch |
| 来源截取 | 7999/8000/8001rune；4000网页片段；空/HTML注入 | 同一使用片段可追溯、截取明示、空不可用、HTML不执行 | TestQuestionSourceText |
| 成文引用 | 当前已读/未读/他run/source进calc/搜索snippet | 仅当前已读可引用；其余拒绝，data/calc不回归 | TestQuestionSourceReferences |
| 答题范围 | 三枚举/unknown、无证据explained、空reason、partial空unanswered、总2999/3000 | 合法/拒绝、必要解释字段必填，3000拒绝；三次不合格无result | TestQuestionAnswerContract |

网络安全继承safefetch原测试，只补signal适配确实经过它的集成证明，不抄整套IP枚举。使用假HTTP/DNS transport和临时容器，不探测真实内网。中文与Unicode按rune测，不用byte长度代替。

## 效果核对 E1～E3（尚未运行）

触发原因：选题价值、是否围绕原题解释、证据是否支持结论依赖真实LLM/来源，mock通过不能代替效果。

方法：执行前向用户确认具体3个样本、模型和调用预算；只读选择真实新闻并导入隔离验收库，保持原文/日期/关联，**不在生产库做测试写入**。建议上限3次发现+3次研究（含失败，不自动补跑），单研究最多40轮，外搜累计≤9、网页累计≤15；这是授权上限不是必须跑满，用户可缩减。先跑一例看证据再顺序跑另两例，不与build/浏览器并行。真实网站状态不预先假定，外搜失败如实记录。

| 样本 | 数据候选（需执行前复查，不当作已核实事实） | 要验证什么 |
| --- | --- | --- |
| E1 能增进理解 | 美国新闻article149928政治资金配置，配同周期对照报道 | 区分2.76亿与3.66亿期间，按资金来源/去向提问；外部证据不足可partial，不用股价答政治组织问题 |
| E2 原文即可解释 | 国内article149312加油机案件通报 | 若选题为查处口径/作弊机制，读原文即可解释；零无关数值调用，不能用399件推断全国发生率或全国趋势 |
| E3 必要证据不足 | 日本新闻挑一条来源稀疏的政治表态，执行前只读确认具体ID | 不从一次举动预测整个外交转向；缺前史/原始说明时unresolved/partial，明确所缺而不强凑解释 |

量化记录写 `research-validation.md`：每样本输入cutoff/冻结材料ID、候选数及中心问题、检索/读取/数值/计算调用数、成功/时间拒绝/网络失败数、实际证据数、answer、引用可定位数/总数、主问题是否回答（人工理由）、无依据断言数、模型token/耗时与费用可得值。当前所有数值=待执行，不预填PASS。

通过判据：样本逐一有原问题与实际回答对照；新来源引用100%可定位当次使用片段；不得出现“没取到=没发生”、从个案直接推广总体、隐瞒时间或用无关市场数据替代原题的实例。E2在可解释前提下不调用无关外源；E3允许未解决且必须写缺口。不要求每样本都得确定结论，不把3例当统计准确率。若E1只有缺口，则诚实记录“链路可运行但目标解释能力尚未证明”，先报告瓶颈再决定交付，不改fixture凑绿。

## 展示字段语义盘点

| 字段 | 来源 | spec锚 |
| --- | --- | --- |
| signal / why_it_matters / research_question | 校验后detect并冻结 | 候选持久化与展示来源；信号发现与人工研究边界 |
| research_brief.unknown / verification | 校验后detect，不由前端生成 | 候选持久化与展示来源 |
| research_contract与旧协议提示 | 服务端发现版本，未知版本拒绝 | 周期材料与固定候选快照 |
| answer.status / reason / unanswered | 校验后compose，非认证、非job状态 | 报告论证与条件判断 |
| sources标题/URL/片段/日期/取回时刻/截取 | 代码读取/冻结元数据，无值null | 观测与计算引用附录；有界来源访问与时间语义 |
| 次数/运行态/忽略/版本 | 原任务和账本/独立disposition/成功结果 | 候选持久化与展示来源；普通读者研究字段呈现 |

## 继承与调整（逐旧Scenario处置）

已运行 `bash scripts/harness/test-assets.sh board-signal-reports` 与 `bash scripts/harness/test-assets.sh research-data-sources`，历史映射用于找到旧测试；对重叠契约额外读取前置change的spec。以下只列本次MODIFIED Requirements的旧Scenario，无删除。前置仍未归档，apply前重跑并核对下表，不用历史文件名假装现存资产。

|Requirement| Scenario（同组处置一致） |处置| 旧测试文件（当前落点） |动作|
|---| --- |---| --- |---|
|周期材料与固定候选快照|  周期校验、历史材料隔离、迟些点击仍用原快照、外部数据边界、只冻结合窗人工关注、后续撤销不改旧研究、失效依据不升级为有效信号、事后选择不冒充当时预测 |继承| backend-go/internal/dataenrichment/service/signal_material_test.go；backend-go/internal/dataenrichment/repository/signal_candidate_test.go |照跑原断言，必要时补新协议fixture；原契约不删|
|信号发现与人工研究边界|  达标只生成候选 |改语义| backend-go/internal/dataenrichment/service/signal_detect_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|信号发现与人工研究边界|  零信号零打扰、检测故障可见、依据白名单 |继承| backend-go/internal/dataenrichment/service/signal_detect_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go |照跑原断言，必要时补新协议fixture；原契约不删|
|候选持久化与展示来源|  刷新与重启保留候选、重复发现不覆盖 |继承| backend-go/internal/dataenrichment/repository/signal_candidate_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go；front/app/features/tags/components/SignalCandidateList.test.ts |照跑原断言，必要时补新协议fixture；原契约不删|
|候选持久化与展示来源|  列表字段可追溯 |改语义| backend-go/internal/dataenrichment/repository/signal_candidate_test.go；backend-go/internal/dataenrichment/handler/signal_discovery_test.go；front/app/features/tags/components/SignalCandidateList.test.ts |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|四十轮问题驱动研究|  空手源提示早停、进程重启收敛孤儿进展、工具失败和任务超时不同、成功后进展归档 |继承| backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/dataenrichment/handler/signal_research_test.go |照跑原断言，必要时补新协议fixture；原契约不删|
|四十轮问题驱动研究|  十几轮查询正常完成、达到四十轮收束、重复与非法动作有限结束、超时后进展不白跑 |改语义| backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/dataenrichment/handler/signal_research_test.go |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|报告论证与条件判断|  不足以转多也能成文、图表降级、无数字仍可成文、块内引用必须核查 |继承| backend-go/internal/dataenrichment/service/signal_compose_test.go；front/app/features/tags/components/SignalReportView.test.ts |照跑原断言，必要时补新协议fixture；原契约不删|
|报告论证与条件判断|  结构与篇幅校验 |改语义| backend-go/internal/dataenrichment/service/signal_compose_test.go；front/app/features/tags/components/SignalReportView.test.ts |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|观测与计算引用附录|  精确引用与定位、完整账本 |改语义| backend-go/internal/dataenrichment/service/signal_compose_test.go；front/app/features/tags/components/reportPresentation.test.ts；backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|观测与计算引用附录|  图表口径不混淆、长小数与极小值、真实日期与图表断点 |继承| backend-go/internal/dataenrichment/service/signal_compose_test.go；front/app/features/tags/components/reportPresentation.test.ts；backend-go/internal/dataenrichment/repository/signal_research_progress_test.go |照跑原断言，必要时补新协议fixture；原契约不删|
|成功快照与无评审输出|  PG直接写入约束、报告完成直接可读 |继承| backend-go/internal/dataenrichment/repository/signal_report_test.go；backend-go/internal/platform/database/board_signal_migration_test.go；front/app/features/tags/components/SignalReportView.test.ts |照跑原断言，必要时补新协议fixture；原契约不删|
|成功快照与无评审输出|  旧数据与新版并存 |改语义| backend-go/internal/dataenrichment/repository/signal_report_test.go；backend-go/internal/platform/database/board_signal_migration_test.go；front/app/features/tags/components/SignalReportView.test.ts |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|工具适配不改现有工具面|  研究显式授权 |改语义| backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/datasources/wiring/tools_test.go |保留旧协议断言，增加新协议分支；不删旧测试换绿|
|工具适配不改现有工具面|  注入后现有工具面不变 |继承| backend-go/internal/dataenrichment/service/signal_research_test.go；backend-go/internal/datasources/wiring/tools_test.go |照跑原断言，必要时补新协议fixture；原契约不删|

## 执行留痕

本轮仅进行了规格结构、映射和文件一致性检查。产品自动化、临时PG、真实模型和浏览器均未运行。实现期逐故事记录命令/结果及失败原因；任务未执行不可勾选完成。
