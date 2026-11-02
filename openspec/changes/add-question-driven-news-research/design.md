## Context

动机见 proposal.md。当前已有显式发现→逐条研究→schema3报告的完整链路。本change不是新建智库平台，而是在同一条链内把问题、来源和结论对齐。

已核事实：
- `signal_detect.go` 的 detector 不持有 Registry；一次读聚合材料，最多2次模型尝试，score≥6。继续保留这个成本结构，不给每篇文章再跑模型。
- `signal_research.go:ResearchCandidate/buildSignalResearchRegistry` 只读发现快照，只授权7源，复用runToolLoop，40轮/40执行与150分钟超时，失败进展已持久化。
- `wire.go` 已接 BochaWebSearcher（DB→env→yaml动态配置），`summary:false`；返回标题/URL/snippet，没有可信发布时间字段。
- `fetch_page.go` 的ReaderPageFetcher会截4000rune，但底层readability.FromURL不走safefetch，不能直接作为本次任意公网URL的安全实现。现成safefetch可管每跳DNS/固定IP拨号、2MiB、10秒、3跳与禁私网。
- 前置change已在工作区实现schema3平面块、忽略与主从工作台，主spec尚未同步。apply必须先核对前置完成状态，不能把本change的完整MODIFIED条文覆盖回schema2。
- 已有board_investigation工具与研究机制不整套移植，其review/judge、动态跨板块探索不是本change目标。

## Goals / Non-Goals

Goals：沿用既有调度/互斥/进展/成文；给新候选增加轻量说明和有界来源；文字证据可追溯且与数字证据类型分离；解释性问题不强迫方向预测。

Non-Goals：不另起agent工作流框架，不新增通用文档库/嵌入索引/来源评分服务，不改变日报或泳道生命周期，不开放跨板块搜索、自由输入问题、自动监控、旧报告升级。不改变七源业务参数与计算数学。

## Decisions

### D1. 以研究协议版本隔离旧行为，不用schema版本判断工具授权

- 新发现的`input_snapshot.research_contract="question-v1"`；新增候选可空`research_brief` JSONB：`{unknown, verification, source_ids}`。unknown/verification由detect生成并验证，source_ids由代码填写，禁止信任模型自报。
- `signal`继续承载简短现象/问题标题，`why_it_matters`说明理解收益，`research_question`为一个中心问题；不新增三套平行问题字段或分类表。score仍1～10阈值6，但换成理解价值、未知具体性、核查路径的评分指导。其主观性不包装为可信度。
- unknown/verification各trim Unicode空白，1～600rune；旧行null。新协议缺brief或损坏直接失败，不静默降级；未知非空protocol拒绝。空协议=旧路径，不允许web工具。
- 报告仍schema3平面块，另保存`research_contract`与`report.answer={status,reason,unanswered[]}`；API未知可选字段按既有兼容策略读取，缺省字段不强转空认证；旧schema3无answer正常兼容。无需schema4或迁移重写历史payload。
- 未选方案：直接给所有旧候选开放新工具，会在原快照没有来源版本时读到今日正文；新建研究项目/新API则重复现有candidate/result关联。

### D2. 发现后只冻结候选所引的本地来源，不逐篇AI筛选

```text
现有周期装配 → detect(聚合材料) → 校验候选
  → 按候选有效section refs批量查询关联article
  → 冻结有界本地正文 → 原子保存discovery + candidates
```

- 在`signal_material.go`及repository增加批量取源方法，服务层传入当前材料白名单；查询必须验证section在本批、本board且article确经该section的thread关联。模型不直接传任意article_id读库。
- 每候选最多冻结8篇不同article（按候选evidence_refs顺序，再thread ID/article ID稳定顺序；跨切片同article去重）；这是可读预算，不宣称最重要的8篇。额外候选不因此丢弃，未纳入数和原因记gap。
- 每篇本地已存正文按`firecrawl_content > content`选非空内容，description只作摘要不可冒充正文。复用已有净化/HTML转文本能力，无网络调用。最多8000rune，保存实际进入模型的同一片段和`truncated/original_runes`，不假称完整文档。首次总量最多64k正文rune/候选；多候选共享来源在discovery.sources中按article_id+content_hash去重。
- `input_snapshot.sources[]`保存系统生成source_id（如n1）、article_id、section_ids、title、url、published_at、captured_at、content_hash、text、truncated、original_runes、temporal_note；brief.source_ids为该候选允许读取的子集。
- 本地文经板块周期切片关联结构性证明合窗（thread 属期内，不依赖 pub_date 反推）；pub_date 晚于 cutoff 的拒绝，未知 pub_date 冻结可引用但必须展示注记（时间行如实标未知，不冒充已核日期；网页侧无结构性证明故硬拒）。captured_at是实际复制时间，不是历史可得性证明。历史周期依旧标事后回顾；发现时正文缺失不启动爬虫，记gap。存文改变/删除不影响已有快照。
- 来源缺失是业务缺口；查询/事务失败是save/prepare错误，整批原子回滚，不以“没原文”掩盖DB故障。
- 未选方案：研究时按article_id直接读最新正文，无法解释再生成为何改变前提；全板块全文送模型则与成本目标相反。只存hash会导致后续正文变更后不可再读，因此存使用片段而不只存locator。

### D3. 单循环、先读后查，机械约束只管必要边界

在现有signal子Registry新增受协议控制的三个动作：
- 组装研究prompt时不得直接marshal新增后的整个发现快照：保留原聚合材料，只提供本candidate的来源目录（ID/标题/URL/日期/截取说明），不提前注入sources正文，也不泄漏其它候选专属原文。正文只能经read_source进入历史，compose只接收实际读过的合格来源。
- `read_source(source_id)`：只读本candidate冻结白名单，不查实时article表；返回保存片段。至少一份可读来源存在时，先成功读取一份，再开放外搜/七源/calculate；不强制读完全部8篇。finish可提前选择，但无任何有效证据不得标explained。
- `web_search(query)`：复用Bocha接口，query trim后1～300rune，须至少含一个Unicode字母或数字，纯分隔符/标点拒绝；只传公开主题/必要实体。prompt与工具参数均不包含key/用户设置，禁止拼入全文/内部ID；不提供模型任意HTTP POST能力。返回最多10条仅作导航，登记HTTP(S) URL候选；去掉userinfo、fragment，query string保持，不因归一化混合不同文档。
- `fetch_page(url)`：仅候选来源URL或本run搜索结果登记的URL可请求，不抓网页内链接，不开放域级无限授权。搜索登记不等于正文可引用；读取成功且时间合格才产生本run来源wN。

每个执行仍要求非空question并计入40总执行，失败也计；重复/非法请求不执行但计决策轮。搜索上限3、网页上限5；不加新的审批回合/阶段枚举。UI继续research→compose。无配置/401/429/超时/无覆盖成为typed gap；相同参数不重复，预算耗尽可以未解决成文。旧7源的3次空手提示保留，不把新来源工具误塞进数值源覆盖统计。

这里“先读”只保证完成一次来源阅读，不能机械证明模型思考充分；是否回答原题、是否考虑相反解释由明确prompt及小样本效果核对验证，不增加judge认证。

### D4. 单独的安全页面适配，不扩改全站crawler

新建signal专用page adapter，使用`platform/safefetch.Fetch`取得有界响应，再把bytes和final URL交已有readability解析依赖。禁止先用readability.FromURL抓完再做URL校验。默认10秒/2MiB/3跳；AllowedIPs不向模型开放、无环境proxy、仅http(s)、拒绝userinfo，各跳及全部DNS结果校验。HTTP非2xx、非HTML/text、空正文返回typed gap；网页正文最多4000rune，保存原字符数与截取标记，模型与附录使用相同片段；不下载PDF、不绕付费墙、不运行JS、不装浏览器、不新增供应商。

现有ReaderPageFetcher和旧调查/QA保持原授权与行为，不在本change顺手重构所有抓取。单测用可注入安全transport/DNS fixture，覆盖重定向内网、重绑定、超体积及超时；不访问真实内网或云metadata。复用safefetch现有测试，不重复它的完整地址分类表。

### D5. 严格区分新闻边界、网页发布时间与取回时间

- 发现新闻与本地正文冻结；新外部取证沿用原cutoff。七源原时间过滤不动。
- 安全page adapter从限定机器元数据（article:published_time、datePublished）解析发布日，dateModified若存在用更晚时点作边界，保留原值与字段出处。不从URL、搜索snippet或模型猜日期。不解析复杂任意JSON脚本，只处理已支持的JSON-LD Article/NewsArticle键；解析失败标unknown。
- 日期带时区按时间戳比较；只有日期时用该日在ShanghaiTZ结束时刻保守判断，结束不晚于cutoff才收录。恰等于cutoff合格；晚一步拒绝。字段冲突取更晚可解析值，不能选最早值绕过过滤。
- 后于cutoff、日期未知、正文空：不将正文交研究模型/compose，不生成可引用source；只返回URL和typed gap。可核验日期是网站声明而非真实性保证，取回时刻和“事后核查”标记必须展示。
- 搜索摘要可能提到后续事件，因此只作导航、不得进入compose证据包。不能声称检索模型完全没有看到未来线索；报告属于有边界的事后核查，绝非历史盲测/当时可得回测。日期缺失可能让有价值的官方页面不可用，宁可说明缺口，第一版不放开隐式例外。
- 新候选regenerate继续使用冻结本地正文/cutoff；外部来源可重新请求，但每run独立保存实际片段、hash与fetched_at，不覆盖前版，不称“完全相同的网页版本”。更新新闻前提必须重新发现。

### D6. 文字证据账本与报告回答范围：小扩展而非文档平台

复用signalResearchLedger与现有progress JSON，新增`sources`以及来源调用的类型区分；data observations、calculations路径不变。来源记录含id、kind(local_article/web_page)、title/url/final_url、published_at及出处、captured_at/fetched_at、text、hash、截取和时间说明。仅成功read_source或fetch_page的正文进入可引用集合；搜索结果只在调用记录，不伪造observations。

引用使用`[[source:n1]]`/`[[source:w1]]`。校验器只认当前report的来源集，拒绝未读/跨run/只有snippet；文字ID不能送calculate，不能画数值图。段落和evidence_table文本行可挂source refs，数值行仍由data/calc渲染；不让原文金额自动成为代码可算数值。所有来源作为不可信纯文本，不执行嵌入指令，不允许HTML/CSS/脚本块或javascript链接。

`answer`固定枚举：
- explained：在所声明的窄范围内已给出解释，至少一个有效来源/观测支持；不是“事实已经证实”标章。
- partial：主问题有未答部分，unanswered非空。
- unresolved：尚不能判断，unanswered非空；允许只有缺口而无成功证据。

reason非空≤400rune；unanswered最多5项、每项≤300rune，均计入3000可见字符总量。模型仍输出四个角色，answer描述范围，implication的观察条件只作文字。解释性问题direction=conditional，horizon允许明确“本轮仅解释现状，不预测期限”。模型生成答题内容，代码只验证类型/引用/长度，不能以结构绿灯宣称语义真实。成文三次上限沿用。

### D7. API/UI最小增量

沿用现有所有URL、POST请求体、202/409/200幂等与ignore允许性；无新设置页。GET候选列表/详情增加可空research_brief、详情增加research_contract；若列表不需要source_ids，可仅详情返回，但unknown/verification来源一致。报告payload增加可选research_contract/answer/sources，前端类型必须可缺省。

旧candidate默认研究与regenerate均走原协议；按钮旁显示升级说明，用户自行使用既有发现入口。工作台只扩展原文本和附录引用，不新增页面结构。工具计数按账本分别说明取数/搜索/阅读/计算，不能把文字读取算成“取得N条数值证据”；原总执行字段含义不变。

## Risks / Trade-offs

- [选题仍依赖模型判断] → 不发明硬评分保证；三种不同板块样本核对，记录逐题的收益、证据和答题范围，不报告未经验证的准确率。
- [固定预算漏掉重要原文] → 公开截取与未收录gap，不声称全覆盖；8篇/8000rune及3搜/5页是初始成本参数，不能以预算截断推断未存在证据。
- [网站缺时间/反爬造成未解决增多] → 明示配置/网络/时间缺口，不降安全限制；真实样本不达标则停在blocked，不能偷偷改契约放宽。
- [网页含错误/操控或同源转载] → 保存原始出处与片段、要求识别转述和对照解释；重复URL/article机械去重不等于独立信源判定。无“权威=真实”认证。
- [存文片段与元数据增加JSON大小] → 仅候选引用范围、有界截取、批次去重；不用文章正文全库复制。来源全文版本不是永久抓取库。
- [与前置change并行修改同文件] → apply入口确认前置完成并重新跑test-assets；不按旧主spec回滚schema3，不修改无关脏文件。
- [发现冻结时正文可能已被网站更新] → 保存captured_at与版本hash并标事后材料，不声称历史版本回测；来源真实性需要研究解释，不由日期字段认证。

## Migration Plan

1. 前置change实现完成、规格同步；核对差异和本change重叠Requirement，再apply。
2. 新增nullable research_brief列，旧行NULL，无UPDATE回填；JSON新增字段不要求迁移旧报告。迁移验证只在临时PostgreSQL，禁止用真库试DDL。
3. 部署后新发现写question-v1；已有候选/报告不变。用户需要新能力时显式重新发现；可选检查现有Bocha key，无强制新配置。首个新研究可能调用搜索后端，UI明确说明。
4. 来源失败可有限成文；全job失败保留进展，不自动恢复付费任务。无需清理或批量重生成。
5. 回滚优先关闭新发现/新研究入口并保留只读，不回滚掉数据列或覆盖JSON。旧二进制可能忽略question-v1而错误按旧协议研究，故不允许直接开放旧写入口；修复前只读查看旧兼容text，未处理新候选保留。恢复兼容版本后继续手动操作。

## 验证策略与规划状态

见test-cases.md与tasks.md：故事层账本、继承Scenario逐行处置、白盒预算/时间/授权边界、一个静态浏览器主链路、三个获准真实样本。只规划，不宣称测试已运行或效果已达标。不添加泛化评分框架或全量测试套件。规划中子线程因后台宿主不可用未启动，主线程直接完成；实现阶段仍安排独立review。
