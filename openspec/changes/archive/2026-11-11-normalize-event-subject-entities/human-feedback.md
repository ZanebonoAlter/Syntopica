# Human Feedback — normalize-event-subject-entities

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-11-09【产品|process】不要人工种子表、不要词表/后缀/长度阈值洗身份、不要用相似度自动认同体——身份必须是全局登记的实体 ID，不是补丁式修补

- **状态**：已消化
- **原文要点**：不要人工种子表、不要词表/后缀/长度阈值洗身份、不要用相似度自动认同体——身份必须是全局登记的实体 ID，不是补丁式修补
- **已核实事实**：aux 池的身份判据是 0.95 余弦，而实测同实体不同写法落在 0.79–0.94、跨实体共享 token 反而 0.9648 → 阈值方向是反的；`NormalizeSubjectKey` 只做小写+空白归一，注释自述「no entity table lookup」 → 现有「身份」其实是字符串归一，不是身份判定。
- **影响面**：`proposal.md`「What Changes / 不做」；`design.md` D1/D3（相似度只做召回、判定交给模型；规范名无唯一约束）；delta spec「全局实体身份层与名称证据层」「叫法歧义不自动合并两个实体」。
- **后续动作**：实现阶段按 D1 建独立身份层；相似度与阈值**不得**出现在身份判定路径（评审项，见 tasks T11.1）。

### FB-2 2026-11-09【产品|fact-check】要全局稳定实体 ID，而不是 LLM 现编的 subject_key 字符串

- **状态**：已消化
- **原文要点**：要全局稳定实体 ID，而不是 LLM 现编的 subject_key 字符串（subject_key 已被实测漂移：person:特朗普 vs person:trump、company/product 同体分型）
- **已核实事实**：真库 910 条事件线 / 495 个 distinct key 里 `person:特朗普`(23) 压过 `person:trump`；`company:deepseek` 与 `product:deepseek` 指同一主体；`product:deepseekharness`（产品）被写成 `person:deepseek_harness_团队`。
- **影响面**：`design.md` D1（独立 `entity_registry` + 稳定实体 ID）、D9（本期身份判定零改动；三分支接线规则移入 design 的 Out of Scope）；delta spec「本期身份判定零改动」；MODIFIED `event-story`「主体归一化」（实体 ID 本期不参与事件线判定、判定路径不读取实体 ID）。
- **后续动作**：实现阶段保留 `subject_key` 字段但**不作为身份来源**；实体 ID 与 `subject_key` 完全解耦（评审项，见 tasks T11.1）。

### FB-3 2026-11-09【产品|test-gap】验收要看真实效果：必须真实文章→真实第一轮抽取，不得用人工理想 query 说明或人工修订过的候选描述宣称生产效果；冷启动连续处理与固定已有池重放分开做

- **状态**：已消化
- **原文要点**：验收要看真实效果：必须真实文章→真实第一轮抽取，不得用人工理想 query 说明或人工修订过的候选描述宣称生产效果；冷启动连续处理与固定已有池重放分开做
- **已核实事实**：研究小测的查询侧短说明是研究者人工撰写的理想输入、候选池 60 条里有 11 条描述经人工修订 → 该结果是「带理想输入的受控测试」，不能当生产效果；Q05「老黄」在 name_only 下只到 rank10、name_desc 到 rank1，优势部分来自人工输入。
- **影响面**：`design.md` D2（短说明必须由第一轮真实产出、仅影子模式）、D12（两段评测协议 + 固定总 N + 覆盖率 + 配对基线对照）；`test-cases.md` §4 效果核对（量化结果标「待实测」，不预填）；delta spec「效果验收口径与分母」。
- **后续动作**：评测任务（tasks T10.1–T10.3）需**单独授权**真实模型调用；未授权则保持未勾选，不得以 fixture 冒充。

### FB-4 2026-11-09【产品|backend】技术失败不算 none；none 就新建，没有不确定队列

- **状态**：已消化
- **原文要点**：技术失败不算 none（非法 ID/不可解析/服务失败都必须重试或落失败，不得降级为 none 或直接新建）；none 就新建，没有不确定队列
- **已核实事实**：研究小测把「非法 ID / 解析失败 / 服务错误」与「合法 none」严格分开且跑通「不在 Top10 → 合法 none → 判重复新建」判定；Q20 误选说明「强行选一个」比「诚实答 none」更危险。
- **影响面**：`design.md` D5/D6；delta spec「第二轮只输出合法实体 ID 或 none」「none 即新建且登记原子」「跨进程全局实体解析闸门」；tasks T6.2/T6.3/T8.1。
- **后续动作**：实现阶段技术失败路径必须留审计且**不建实体**；评审项（tasks T11.1）。

### FB-5 2026-11-09【产品|backend】跨板块共用实体，但事件线仍按版块内、同 entity 不意味着同 story；人工归属不覆盖；组合标签不挂 story

- **状态**：已消化
- **原文要点**：跨板块共用实体，但事件线仍按版块内、同 entity 不意味着同 story；人工归属不覆盖；组合标签不挂 story
- **已核实事实**：`board_stories` 身份本就按 `board_id` 落库（索引 `(board_id, subject_key)`），事件线与话题归属正交；`story_manual` 人工标记受既有合并闸门保护。
- **影响面**：`proposal.md`「What Changes / 不做」；`design.md` D9；delta spec「本期身份判定零改动」；tasks T9.1/T9.2。
- **后续动作**：本期实体层**不接线**到事件线；接线契约已写进 delta spec 供后续 change 使用。

### FB-6 2026-11-09【产品|backend】辅助标签池候选与向量设施可复用，但 aux_id ≠ 实体 ID；同实体不同叫法要合成同一选项；别名可歧义；类型提示不硬否决

- **状态**：已消化
- **原文要点**：辅助标签池候选与向量设施可复用，但 aux_id ≠ 实体 ID；同实体不同叫法要合成同一选项；别名可歧义；类型提示不硬否决
- **已核实事实**：研究里 Q09 换序从「唐纳德·特朗普」变成「特朗普」——两者 aux_id 不同但语义都对 → aux_id 不能直接当主体 ID；aux alias 可能指向产品而非公司（`英伟达GPU` 挂在公司行）→ alias 不能当同体证明；两个 embedding 绑定同文本 cos 0.57–0.76 → 向量设施可复用但**空间不可混**。
- **影响面**：`design.md` D1/D3/D4；delta spec「全局实体身份层与名称证据层」（含「辅助标签溯源只记录与提及文本相等的那一行」「辅助标签别名不构成同体证据」）+「候选构造由两路向量与名称登记命中合并而成」+「两路检索空间与机械的候选选取规则」。**类型提示本期已从身份层删除**（模型猜类型会把漂移固化进身份）。
- **后续动作**：辅助标签只作「资料 + 叫法证据 + 链接目标」；不做 22k 行预导入（tasks T2.1/T4.2）。

### FB-7 2026-11-09【产品|fact-check】研究里原始 Recall 百分比作废，不得再用；评测须自带分母与零分母处理

- **状态**：已消化
- **原文要点**：研究里原始 Recall 百分比作废（分母错误），不得再用；评测须自带分母与零分母处理
- **已核实事实**：旧口径把有向测量行当独立样本、并把 3 个非同一实体对计入正例；重算后有效无向配对为 11 对 / 18 个有向测量行（保守 hit@1=1/11、@5=8/11、@10=10/11、@50=11/11），且该数字是**样本对 Recall，不可外推**。
- **影响面**：`research.md` §3（作废声明 + 重算口径）；`design.md` D12；delta spec「效果验收口径与分母」（固定总 N + 覆盖率 + 技术失败数 + 配对可比；分母为 0 报「不适用（N=0）」）。
- **后续动作**：评测报告必须自带分母与基线对照，**不引用**已作废数字（评审项，tasks T11.1）。

### FB-8 2026-11-09【流程|process】本阶段 0 次模型调用；真实效果评测是 apply 之后单独的有界授权

- **状态**：已消化
- **原文要点**：本阶段 0 次模型调用；真实效果评测是 apply 之后单独的有界授权（上一次 25 次 Chat 授权已消耗完，不得复用或自动扩额）
- **已核实事实**：研究 run `qwen-eval-20261109-181947` 的 chat 尝试 25/25、embedding 提交文本 152/160 均已用尽；该 run 自身的协议写明「上限用尽即停，不补题、不扩额」。
- **影响面**：`proposal.md`「需要用户手动执行的操作」；`design.md` D11（预算按真实阶段分列）；tasks T1.3/T12.4（未授权则 §12 保持未勾选）；delta spec「有界评测入口与预算口径」。
- **后续动作**：评测入口的闸门在**发出请求前**拦截（tasks T9.2），超限即拒并计数。

### FB-9 2026-11-09【产品|process】制品太冗长：按主要用户故事覆盖全即可，不要求逐条穷举

- **状态**：已消化
- **原文要点**：任务太多太冗长了，包括测试用例也是，把主要的用户故事覆盖全就行；不是简单合并 checkbox 而保留原复杂度，要实质减量、故事覆盖全。
- **已核实事实**：上一版 delta 83 Scenario / 66 checkbox / 45 白盒分支中，存在同一契约被拆成多条 Requirement/Scenario 的重复表述（技术失败四类各占一条、影子写入范围两条重复断言、「第一轮短说明」占两个 Requirement）；同时确有不可删的功能与错误路径（技术失败非 none、并发旧结论、资源释放、关闭态回归、预算拦截、金标隔离、缺身份/未归一单列、零分母、未达标不启用生产）。
- **影响面**：`specs/entity-subject-resolution/spec.md`（18 Scenario，按 5 故事组织）；`specs/event-story/spec.md`（原 4 条逐字 + 影子边界合成 1）；`specs/aggregate-tagging/spec.md`（2）；`tasks.md`（7 条 checkbox + 尾三节）；`test-cases.md`（5 故事主链路 + 10 条高风险分支 + 压缩版 ⓪/变体/盘点）；`design.md`（D1–D12，落点改为新任务 id）。
- **后续动作**：减量只动**组织方式**，不动口径与事实；效果评测与实现验证仍未开始（无模型调用、无写库）。

### FB-10 2026-11-10【产品|backend】实体层默认应该开起来（shadow），不是 off——这是后续主功能，而且当前没有独立配置入口

- **状态**：已消化
- **原文要点**：「实体层默认应该开起来，因为现在也没地方给他配置，而且这个作为后续的主要功能应该是开放的」。
- **决策与落地**：`viper.SetDefault("entity_subject.mode", "shadow")`（`internal/platform/config/config.go`）；`backend-go/configs/config.yaml` 新增 `entity_subject.mode: shadow`（补上可配置入口）；`ModeFromValue("")` 仍回退 `off`（配置未加载的测试/CLI 安全兜底，生效默认由 SetDefault 给出）。
- **口径同步**：delta spec「影子接入与既有判定零改动」模式条改为 `shadow`（默认）；`design.md` D10 决策句 + Migration Plan 第 2 步 + Risks「模型调用成本」；`proposal.md` What Changes / Impact / 部署后影响；`tasks.md` T3；`test-cases.md` S5 步 1（改为「显式配置 off 跑打标」）；`docs/reference/configuration.md`（默认值表 + 三模式表 + 延迟代价）；`docs/reference/flow/ai-summary.md` 红线 19、`topic-graph.md` 红线 15。
- **代价（已写进配置文档）**：`shadow` 同步跑在打标管线内，每个 event 标签多 1–2 次二判 + 查询 embedding，全局单键闸门串行；本地小模型单次二判可达数十秒，吞吐会下降。需要吞吐时把 `entity_subject.mode` 改回 `off`。

### FB-11 2026-11-10【流程|process】抽取 `finish_reason=length` 截断直接修，不单独开 change

- **状态**：已消化
- **原文要点**：「可以修截断，直接修不开 change」。
- **已核实事实**：`ai_call_logs` 近 90 分钟 `tagmanagement.extractor_enhanced` 400 成功 / 193 失败（32.5%），失败全部是 `finish_reason=length; response does not contain extractable valid JSON`；成功/失败 prompt 长度几乎相同（2894 vs 2857 runes）→ 不是输入过长，是输出被 `maxTokens := 2048` 截断。p50 19.3s / p95 38.4s，重试代价高。
- **落地**：`extractor_section.go` 新增共享常量 `extractionMaxOutputTokens = 4096`（含测量依据注释），section 与 merged/mono 两条路径共用；`extractor_section_test.go` 断言改为引用常量。
- **用户显式豁免**：本次按用户指令不新开 openspec change（用户当场指令优先于「所有改动默认走 openspec」）。

### FB-12 2026-11-10【产品|fact-check】实体层默认改回 off：不接入事件线判定（实体注册表只增不减 + 二判幻觉固化）

- **状态**：已消化
- **原文要点**：实体层默认改回 **off**——影子实验结束，实体层不接入事件线判定线；原因是实体注册表只增不减（黑洞）+ 二判幻觉会把错误身份固化。
- **已核实事实**：FB-10 把默认改成 `shadow` 的理由是「它是后续主功能、默认开启」，但**没有**给出「默认开启」的收益证据；相反，2026-11-10 C1 标定结论是实体层判定为「会变成只增不减的黑洞」，**先关停**。`entity_registry` 只在合法 `none` 时新建、无合并/退役路径 → 实体行数单调增长、错误主体一旦登记就长期污染候选池；二判是模型选择题，幻觉选中的 ID 会被当作身份事实落库，后续同名提及持续复用。
- **影响面**：`internal/platform/config/config.go`（`viper.SetDefault("entity_subject.mode", "off")`）；`configs/config.yaml`（`entity_subject.mode: off`）；`docs/reference/configuration.md`（默认值表 + 三模式表 + 延迟代价段）；`docs/reference/flow/topic-graph.md` 红线 15、`docs/reference/flow/ai-summary.md` 红线 19；delta spec「影子接入与既有判定零改动」模式条；`design.md` D10 + Migration Plan 第 2 步 + Risks「模型调用成本」；`proposal.md` What Changes / Impact / 部署后影响。
- **后续动作**：代码与数据**都保留**（不删实体表数据、不改实体层代码），`shadow` 仍是可用能力、需显式配置开启；「实体 ID 接入事件线判定」的接线规则按 `enable-story-cross-day-continuation` 的 C1 标定结论**撤销**（D14 标注已撤销），改走 embedding 判定路线（阈值 0.82）。
