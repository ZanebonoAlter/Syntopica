# review: 主体实体归一（normalize-event-subject-entities）

> **本文件已由作者处置（第二轮）**：R-1..R-11 逐条给出「按事实修复 / 已采纳 / 已驳回」与**具体承接位置**（制品文件与任务 id）；条目中的**文件行号指向修订前的版本**（修订后行号已变，请按条目里的描述与承接位置定位）。
> 处置同时以**主线程裁决**为优先：裁决与 review 建议不一致处，条目里写明采纳哪一方与理由（例：R-1 的「锁键=规范化提及名」被裁决否决，改为**全局解析闸门**；R-5 的「名字精确即身份硬复用」被驳回，只采纳其**事实缺口**部分）。

> 审查类型：**仅制品独立审查**（不实现、不跑测试、不调模型、不写真库）。范围：本 change 全部制品（proposal / design / tasks / test-cases / research / human-feedback / ui-design / specs 共 50+6+4=60 个 Scenario）+ `docs/research/subject-entity-resolution/evaluation/main-review-qwen-eval-20261109-181947.md` + 现行 flow 红线（topic-graph 13、semantic-board 1/2、ai-summary、daily-report）+ 既有代码事实（airouter 路由/缓存、extractor 指纹、topicgraph 归一）。
> 口径：只指出**本版无法真正测、或无法达到其自身承诺**的具体缺口；不因实现期测试文件尚未创建而要求伪造空测试；不把新功能建议当 bug；不自行改需求。
> 首轮审查结论：11 条待处理（首轮时全部待作者处置）。

## 一、问题清单

### R-1 2026-11-09【High】跨来源首次提及并发必然各建一行：锁键含来源标签，「跨进程有效」只覆盖同键重放
- **状态**：已修复
- **文件行号**：`design.md:67`、`:71`（D5）；`specs/entity-subject-resolution/spec.md:176`、`:227`；`tasks.md:36`（T5.4）、`:40`（T6.1）；`test-cases.md:3`
- **事实**：D5 用 PostgreSQL advisory lock「按提及键取」，并明说「锁键与『同篇同提及幂等键』一致」；而幂等键 =（来源标签, 提及键）。锁键因此天然含来源标签 → 两篇不同文章的同一提及键不同 → 不受同一把锁保护。spec:176 也只承诺「串行化同一提及键的创建」；spec:227（同一提及跨文章落到同一实体 ID）与 `test-cases.md:3`（「特朗普」「Trump」「老黄」三种写法最终指向同一个 ID）却是无条件 SHALL；`tasks.md:36` 反而要求「两个不同提及键即使规范名相同也各自建实体」。
- **影响**：实体池为空时两篇并发文章各提一次「特朗普」→ 两进程各判 none → 各建一行 → 身份漂移，正是本 change 宣称要消灭的失败模式；现有并发用例（T5.4）只测同键，不覆盖此路径。即 spec 并未误承诺跨来源全局防重，但 D5「跨进程有效」的措辞与 `test-cases.md:3` 的故事会被读成全局有效——属**承诺口径大于机制**。
- **最小处置**：初版允许粗粒度串行：锁键改用**规范化提及键（不含来源标签）**，锁内 SHALL 重查该键是否已有实体/提及行（命中即复用，见 R-2）；同时把 spec:176 与 `test-cases.md:3` 的措辞对齐为「同一提及键跨来源串行；不同写法靠召回+判定，不保证并发首提不重」。不用名称唯一、不用 Go mutex、不用无界重试。

- **处置与承接**：**处置（主线程裁决优先）**：不采用「锁键=规范化提及名」——Trump 与「特朗普」键不同，防不住跨写法；同名不同实体又不可强合。改用**跨进程全局实体解析闸门**：PostgreSQL **会话级咨询锁（独占独立连接、不跨 HTTP 持事务）**串行化「重读池 → 二次模型选择 → 原子登记/新建」全过程，**第一轮抽取在闸外**；取得闸门后**必须重读**已登记提及与候选池，闸外旧 `none` 作废。承诺范围收紧为「**同一提及键**跨来源串行只建一个」；跨写法/未召回/模型误选造成的重复**不作无条件保证**（归为效果指标）。
- **承接位置**：`design.md` D6（取舍段含闸门 vs 锁键 vs 长事务的对比）、D8（跨来源不得按名字跳过判别）；`specs/entity-subject-resolution/spec.md` 新增 Requirement「跨进程全局实体解析闸门」（6 个 Scenario）；`tasks.md` T7.1–T7.5；`test-cases.md` 故事 S5 + 白盒 B20–B25。

### R-2 2026-11-09【High】锁内未要求重查：同一提及键串行化之后仍可建出两个实体
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:192-197`（Scenario 并发同提及只建一个实体）、`:176`；`design.md:67`；`tasks.md:36`
- **事实**：Scenario 要求两进程同键只建一个实体；但 Requirement 只写「以数据库级并发控制串行化同一提及键的创建」，未要求取得控制后复查既有实体/提及行。设计明确「LLM 调用在锁外、锁只包写库段」→ A 建完释放锁后，B 拿到锁仍按自己那一轮的 none 继续建 → 两个实体。`tasks.md:35`（T2.2）虽列了「同提及键反查已存在实体」仓储方法，但无 SHALL 强制在锁内调用。
- **影响**：本 change 唯一的并发硬承诺按现文字不可满足；「不留半实体」只保证单次事务原子，不保证「只建一个」；测试可照现文字写而仍然测不出双建。
- **最小处置**：在「none 即新建且创建跨进程一致」Requirement 增一条 SHALL：取得并发控制后 SHALL 重新检查该提及键是否已有实体/提及行，命中则复用既有实体且不再新建；`tasks.md:36` 增一条对应验证。

- **处置与承接**：**处置**：在闸门 Requirement 内增「取得闸门后 SHALL 重新读取已登记提及与候选池；候选集变化时按最新候选重新判定」，并把「同名不同提及各自建实体」保留为**刻意不保证**的场景（锁不是合并器）。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「跨进程全局实体解析闸门」→ Scenario「取得闸门后重新读取候选」；`design.md` D6；`tasks.md` T7.2、T7.5；`test-cases.md` S5 步 3、白盒 B21。

### R-3 2026-11-09【High】误合率分母按「系统给出实体」取样 → 失败变多会刷好指标
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:353`、`:355`、`:363-368`；`tasks.md:61`（T9.3）、`:68`（T10.3）；`test-cases.md:89`
- **事实**：误合率分母 = 「有金标实体**且系统给出实体**的提及数」→ 技术失败/未归一的提及被排除在分母外；五个指标里**没有**「归一覆盖率/未归一率」。验收门槛只有「误合率不得高于基线、重复身份率须低于基线」。
- **影响**：把第二轮做成「多失败、少误合」即可降低误合率 → 「不得高于基线」可被失败刷过 → 「不达标就不启用生产」这条唯一效果门槛失去守门作用；且失败样本既不进误合率分母、也不在重复身份率/重复新建率的必然覆盖内（重复新建率分母要求「金标实体当时已在池中」），存在系统性盲区。
- **最小处置**：新增「归一覆盖率（分母 = 全部待归一提及数）」并要求覆盖率不低于基线（或设绝对下限）；误合率分母保留现口径但必须与未归一率同表披露，达标判定需同时看两者。

- **处置与承接**：**处置**：误合率分母不再按「系统给出实体」取样。验收改为**固定总样本 N** 为分母口径，并强制同表披露 **覆盖率、技术失败数、错误复用数** 与**按固定总 N** 的错误率；条件准确率只能附列。新增**覆盖率门槛**（实体 ID 侧覆盖率不得低于既有身份侧）与**技术失败数不为 0 即阻断**——堵住「多失败、少误合」的刷分路径。
- **承接位置**：`specs/entity-subject-resolution/spec.md` Requirement「效果验收口径与分母」（新增 Scenario「覆盖率低于既有身份即不达标」「技术失败数不为零即阻断」「未归一不以单元素组冒充正确」）；`design.md` D12；`tasks.md` T12.3、T12.4；`test-cases.md` §4 效果核对与故事 S7。

### R-4 2026-11-09【High】基线不可比 + 两段评测未分绑 → 验收门槛不可机械判定
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:351`、`:353`、`:355`；`tasks.md:67`（T10.2）、`:68`（T10.3）；`test-cases.md:187-190`
- **事实**：①基线是 `subject_key` 字符串身份，但**没有任何制品定义基线的误合率/重复身份率怎么算**（缺「subject_key 组 → 金标实体」的映射规则）；新系统的误合是提及级（归到 ≠ 金标实体），基线误合天然是组级（一个 subject_key 组跨了多个金标实体），两者分母与量纲不同却被要求直接比大小。②`tasks.md:67` 分「冷启动连续处理」「固定已有池重放」两段，指标定义未绑段：冷启动段池为空 → 金标实体多不在池 → 按 spec 既进误合率分母（系统给出实体）又被重复新建率分母排除 → 这批漏合要么被算成误合（门槛在冷启动段不可能达成），要么因两类错误定义重叠而被静默漏计。
- **影响**：「误合率不得高于基线、重复身份率须低于基线」是本 change 唯一的达标判据，当前不可机械复算 → 评审报告与后续 change 的启用结论都缺可核验依据。
- **最小处置**：在「效果验收口径与分母」补三件事：①两类错误互斥定义（误合 = 归到 ≠ 金标实体；重复新建 = 金标实体当时已在池中却新建了实体）；②每个指标注明按哪一段计算（冷启动段重复新建率记「不适用」）；③基线的计算定义（同一金标实体集合下，subject_key 组跨越多个金标实体记基线误合）。

- **处置与承接**：**处置**：三类错误互斥成对定义（误合=金标不同而预测同组；漏合=金标相同而预测不同组；重复身份组率=金标组被拆成 ≥2 预测组）；两套系统（既有 `subject_key` / 实体 ID）**在同一「两套都有预测」的可比子集**上计算，量纲可比；既有身份侧**空键单列「缺身份」**（不默认正确、不默认新实体）；冷启动段重复新建率只统计「金标实体当时已在池中」的提及且**首次自然新建不计重复**（该判定只由评分器依顺序历史金标事后标注、不回喂归一器）；重放段冻结**未经金标修正**的池；「当时池缺该实体」只描述当时池状态。门槛改为可机械复算的四条（误合不增加 / 重复身份减少 / 覆盖率不低于基线 / 技术失败为 0），数据不足、基线分母 0、某段未测 → 不判达标。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「效果验收口径与分母」（14 个 Scenario）；`design.md` D12；`tasks.md` T12.3；`test-cases.md` §4 与白盒 B35–B41。

### R-5 2026-11-09【High】「同一实体的多种叫法归入同一实体」是无条件 SHALL，但机制只有向量召回，缺证据层精确匹配通道
- **状态**：已采纳
- **文件行号**：`specs/entity-subject-resolution/spec.md:36-40`、`:85-87`、`:227-231`；`design.md:49-55`（D3）；`research.md:§3`；`docs/research/subject-entity-resolution/evaluation/main-review-qwen-eval-20261109-181947.md` 限制 1
- **事实**：候选构造只有向量召回（spec:85-87），aux 不作检索语料（`design.md:55`），名称与证据层 `entity_mentions` **没有任何检索/匹配入口**；召回不命中 → 模型只能在候选里选 `none` → 新建实体 → 场景不成立。研究侧的事实是：22k aux 池上同实体不同写法的非自命中 hit@1 仅 9%–18%（`research.md:§3`，样本对口径不可外推），说明召回不是可靠保障。
- **影响**：核心卖点（「三种写法指向同一个 ID」）没有可兑现机制；「漏召回导致的重复新建」在冷启动段是主失败模式，却没有任何单独指标归因（与 R-3 的盲区叠加）。
- **最小处置**：候选构造补一条**不依赖向量**的证据层通道（`entity_mentions` 规范化提及文本精确/别名命中即入候选）；或把两条无条件 SHALL 改写为「当该实体被召回时 SHALL 归到同一 ID」，并新增「别名召回失败率」指标。

- **处置与承接**：**处置（部分采纳 + 部分驳回）**：**采纳事实缺口**——候选构造此前只有向量召回一条通道，名称与证据层没有检索入口 → 新增**名称登记命中入口**（已登记叫法 → 实体 ID 入候选），与两路向量候选 **union 并按 `entity_id` 去重**。**驳回「名字/别名精确即身份硬复用」**：裸「苹果」指公司还是水果无法由名字判定；辅助标签 alias 是 L2 相似度自动写入的产物（`英伟达GPU` 挂在公司行，指向 GPU 产品 ≠ 公司），拿 alias 做同体证据是把相似度产物升级成事实。名称命中**只提供候选**，判定仍归第二轮模型结合上下文；同一叫法可对应多个实体 → 多个 ID 选项。幂等只对**同来源同提及**复用先前决策，**跨来源不得仅按名字跳过判别**。无条件 SHALL「同一实体所有叫法必合」已改写为**可测的结构契约**（候选映射到同一实体时只出现一个 ID 选项），真实归一率交给效果用例，未达标不算完成。
- **承接位置**：`specs/entity-subject-resolution/spec.md` 新增 Requirement「候选构造由两路向量与名称登记命中合并而成」（5 个 Scenario）+「同来源同提及幂等重放」（3 个 Scenario）；`design.md` D4（含「不做硬复用」的技术反驳）、D8；`tasks.md` T5.1–T5.4、T8.2–T8.3；`test-cases.md` 故事 S2/S3、白盒 B16–B18、B15；`research.md` §5.2（技术反驳留痕）。

### R-6 2026-11-09【Medium】两套检索表示并存，但「查询用哪个空间」未定义 → 召回口径不可复算
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:16`、`:61`、`:85`；`tasks.md:25`（T4.1）、`:27`（T4.2）
- **事实**：spec 要求同空间比较且禁跨空间；T4.1 规定「短说明为空时只写 name 空间」；但**没有任何制品规定查询向量用 name 空间还是 name+desc 空间**。短说明为空的实体只有 name 空间向量 → 用 name+desc 查询的提及永远召不到它们；用 name 查询则 name_desc 的收益（研究 Q05「老黄」rank10→rank1）全部丢失，且该收益恰是「稳定短说明」这条新链路的唯一卖点。
- **影响**：Recall@K 不可解释、两个空间的向量口径互相打架、重复新建率无法归因（分不清是空间不匹配还是真漏召回）；与 R-5 叠加后本期最关键的召回效果无从验收。
- **最小处置**：spec 写明查询空间选择规则（例如：有短说明→查 name_desc 空间，并以 name 空间的精确/近邻兜底；或统一只查 name 空间 + 名称精确匹配），并写明跨空间如何合并候选、同一实体如何去重。

- **处置与承接**：**处置**：写明两套表示各自在**同 model/版本/维度**空间内检索；**名路径始终存在**（短说明为空只降级为仅名，实体不隐形）；本次提及带说明时两路都检索；两路结果与名称登记命中按 `entity_id` 合并去重，每路取前 K、并列按实体 ID 升序（机械确定）；K 与合并条数是**可评测调整的配置，不是身份裁决阈值**；换模型只查同空间，未经证明同空间的辅助标签历史向量不混入。
- **承接位置**：`specs/entity-subject-resolution/spec.md` 新增 Requirement「两路检索空间与机械的候选选取规则」（4 个 Scenario）；`design.md` D3；`tasks.md` T4.1–T4.4；`test-cases.md` 白盒 B10–B13、B11。

### R-7 2026-11-09【Medium】预算公式不覆盖真实调用形态，「硬预算闸门」不是实际硬界，且与审计口径冲突
- **状态**：已修复
- **文件行号**：`design.md:99`（D9）；`tasks.md:60`（T9.2）；`specs/entity-subject-resolution/spec.md:312-329`
- **事实**：公式 = 模型尝试「提及数 × 2」；embedding 文本「提及数 + 2 × 新建实体数」；HTTP「ceil(文本数/32) × 重试系数」。缺口四处：①「现场重跑第一轮」（`design.md:99`、`tasks.md:67`）产生的第一轮 LLM 调用不在公式内；②一次 Chat 尝试内部按 provider 列表降级并 `errors.Join`（`backend-go/internal/platform/airouter/router.go:410`），一次「尝试」可能产生多条 `ai_call_logs`，与 spec:329「报告数字 SHALL 与审计一致」冲突；③`Router.Embed` 只在 operation 属于 `embeddingCacheOperations` 白名单（`router.go:446-451`，现仅 `tagmanagement.embedding` / `article_search_query`）时才批量，否则一条输入一次 HTTP（`router.go:502`），代码中**不存在「32 条一批」的切分**，HTTP 项公式与真实行为差一个量级；④每提及可能要 name 与 name+desc 两套查询向量，文本数项偏小；⑤「重试系数」无数值。
- **影响**：闸门既可能提前误杀真实 HTTP，也可能放行超预算调用；预算、审计、报告三处数字口径互不相同，「有界/可机械计算」不成立。
- **最小处置**：模型侧按 provider 尝试计数（含降级链）并把第一轮重跑单列一栏；embedding 文本按「每提及实际查询空间数 + 每新建实体 2」计；HTTP 项按 router 真实行为推导（新 operation 不在缓存白名单时＝每文本 1 次），或把 operation 纳入白名单后按批量口径算；「重试系数」写成具体数值。

- **处置与承接**：**处置**：撤回「提及数×2」与「ceil(/32)」两条与真实路由行为不符的公式（`router.go:446-451` 缓存白名单仅 2 个 operation，新 operation 不在白名单时**一条输入一次 HTTP**；代码中**不存在 32 条一批的切分**——那是研究 runner 自带的批处理）。预算清单改为按**真实阶段**分列：① 文章抽取次数 = 实际抽取文章数；② 二判次数 = 实际提及数 + **实际发生**的有限重试；③ embedding **实际提交文本数**（两路查询 + 实体创建 + 别名登记 + 材料读取，含重复发送与模型降级重复）；④ HTTP 次数按**发送前计数**，不支持批量按单条计，结构化输出降级链与客户端多次 HTTP 尝试一并计入。硬预算用**仅在评测入口注入**的 provider/transport 包装（不做生产路由整体重构，保持正常审计）；无法建立发送前计数能力则**预先阻断**。当前无新额度授权 → 本期只出预算清单生成与授权流程。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「有界评测入口与预算口径」（7 个 Scenario）；`design.md` D11；`tasks.md` T11.2、T11.3、T1.3、T12.4；`research.md` §5.1（撤回留痕）；`test-cases.md` 白盒 B32–B34、边界值「预算余量」。

### R-8 2026-11-09【Medium】event-story 的 MODIFIED 把「后续接线契约」写成现行规则，与同一段「不参与事件线身份判定」自相矛盾
- **状态**：已修复
- **文件行号**：`specs/event-story/spec.md:11`、`:29-37`；`specs/entity-subject-resolution/spec.md:246-251`（同一契约已在本能力内）
- **事实**：同一段里既写「本期…SHALL NOT 参与事件线身份判定」，又写「实体身份的比较规则 SHALL 为——两侧都有实体 ID 时以实体 ID 为准…回退到本 Requirement 的 `subject_key` 判定路径」。该 Requirement 是 MODIFIED，归档后进主 spec 成为现行规则；而本期实体 ID 不接线，三分支比较不可达。另外新增的两条 Scenario（`:29` 实体身份层不参与事件线身份判定、`:34` 存量事件线身份不因实体层改写）在实体 ID 未接线时**恒真**，属不可分辨测试。
- **影响**：主 spec 会同时声明「实体 ID 不参与判定」与「有 ID 按 ID 判」两套口径，后续 change 接线时易按后者读；两条新 Scenario 提供不了任何回归信号。
- **最小处置**：event-story 的 MODIFIED 只保留边界声明（本期不参与、既有三层逐字不变、人工归属不覆盖），三分支比较契约留在 entity-subject-resolution（已有同款）；若要保留，明确标注「后续接线契约，本期无实现，SHALL NOT 被读作现行行为」。

- **处置与承接**：**处置**：删除 event-story delta 里「双侧都有实体 ID 时以实体 ID 为准 / 单侧回退并标混合身份比较 / 两侧无 ID 走既有路径」这套本期不可达的三分支规则——它归档后会变成主 spec 的现行规则，与同段「本期不参与判定」自相矛盾。三分支接线规则移入 `design.md` 的 **Out of Scope（后续 change）**，不写为现行 SHALL。event-story 的 delta 只保留边界声明与 **legacy 零改动**保障场景；为避免「恒真无信号」，新场景额外断言**判定路径不读取实体 ID**（有人把实体 ID 接进 story 判定即失败）。`subjectPreFilter`、类型前缀、object/date、人工保护、composite、热度均未动，Change C 保持独立。
- **承接位置**：`specs/event-story/spec.md`（MODIFIED 段重写，3 条新场景）；`specs/entity-subject-resolution/spec.md`「本期身份判定零改动」（5 个 Scenario）；`design.md` D9 + Out of Scope 首条；`tasks.md` T9.1–T9.3；`test-cases.md` 故事 S6 步 5/6。

### R-9 2026-11-09【Medium】实体类型提示与 aux 关联判据都没有生产者/判据 → 两处「提示」实际无源
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:13`、`:15`、`:19-23`；`design.md:35`（D1）、`:55`（D3）；`tasks.md:18`（T2.1）、`:30`（T5.4）
- **事实**：①spec:13 定义实体类型枚举（person/company/org/product/other）并要求类型不符的候选仍入候选集，但第一轮只新增 `subject_desc`（`design.md` D2），第二轮只输出实体 ID → **实体类型无来源**；现实可用的只有模型现编的 `subject_key` 前缀（`research.md:§1` 实测 `company:deepseek` 与 `product:deepseek` 分型漂移），制品未说明是否采用。②spec:15 说 aux 是「检索资料与叫法证据来源」，但无规则说明**哪个 aux 行算某提及的证据**（一个 event 标签的 `auxiliary_labels` 里可能有多个与主体无关的辅助标签）；而 aux 不作检索语料、本期无读路径（`test-cases.md:§5`）→「可复用 aux」在本期没有可兑现出口。
- **影响**：实现时只能自造类型、自定关联判据；「类型提示不否决候选」「叫法歧义不自动合并两个实体」两条 Scenario 的夹具来源不明；`entity_aux_links` 可能成为无消费者的写侧负担。
- **最小处置**：spec 写明类型来源（建议取第一轮 `subject_key` 前缀，标注「模型提示、可为空、不得作硬否决」）；给出 `entity_aux_links` 的关联判据（例如只链接与提及文本规范化相等、或被第二轮判为同指的 aux 行；标签其余 aux 不关联），或本期明确「只建表不写关联行」。

- **处置与承接**：**处置**：**删除实体类型字段**（唯一现成来源是模型现编的 `subject_key` 前缀，而它已实测分型漂移；让模型猜类型只会把漂移固化进身份），类型若未来保留只作第一轮可选输出且绝不参与身份判定。**aux 关联给出判据并落实承接**：溯源行只记录「本次标签的辅助标签列表中规范化文本与本次提及相等的那一行」的 `aux_id`，**不得**用相似度或自动 alias 判同体；未关联 aux 只作资料、不作实体选项。短说明只来自真实同次抽取，**不得**从 `subject_key` 回填、不得因说明为空而编造。`entity_aux_links` 的**唯一写入方**是上述判据、**唯一消费方**是评测报告的「材料溯源」列与运维排查——不再是「无调用方的表」。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「全局实体身份层与名称证据层」（新增「辅助标签溯源只记录与提及文本相等的那一行」「辅助标签别名不构成同体证据」「短说明为空时不编造身份说明」）；`design.md` D1（含 aux 溯源的写入方/消费方）；`tasks.md` T2.1、T2.2；`test-cases.md` 故事 S1 步 7/8、白盒 B27、B28；`research.md` §5.2。

### R-10 2026-11-09【Medium】第一轮 schema 变更不受三模式开关约束，与「部署后用户可见变化：无」冲突且无回退开关
- **状态**：已修复
- **文件行号**：`tasks.md:20`（T3.1）、`:21`（T3.2）；`specs/entity-subject-resolution/spec.md:287-289`；`proposal.md:53`、`:27`
- **事实**：T3.1 在既有提取 schema 与提示中新增 `subject_desc`，**无条件生效**；模式开关（spec:289）只管第二轮调用与实体层写入（off = 零新增模型调用、零实体层写入），未覆盖第一轮字段。`proposal.md:53` 断言「部署后用户可见变化：无…事件线全部不变」，而事件指纹结构是用户可见的（`board_stories.subject_key/subject`），第一轮提示改动会改变模型对既有字段的产出（研究已证 prompt 改动会改判定，main-review 限制 2）。
- **影响**：默认部署即改线上第一轮 prompt，无开关可回退；「一字不变」只能由既有指纹单测夹具证明（T3.3 断言不改仍绿），证明不了线上产出不变——这正是「本版无法真正测」的一处。
- **最小处置**：把第一轮 `subject_desc` 产出也挂到模式开关（off 时提示不变），或在 `proposal.md:53` 改写为「第一轮 schema 变更始终生效，事件线主体键漂移列为上线后观测项」并给出观测口径。

- **处置与承接**：**处置**：`subject_desc` 字段**只在影子模式请求**。关闭态（默认）下第一轮**提示与 schema 逐字不变**、不加字段输出、不发新增请求、零实体层写入、**不写** `metadata.entity_subject` 键；影子态才启用该字段并只写实体层与审计；`active` 显式拒绝。评测入口走**显式影子路径**（进程内传参），**不修改应用全局配置**。因此本期**不宣称生产漂移已修好**（生产启用本期不实现）。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「影子模式与生产开关边界」（新增「默认关闭态零额外调用」）+「第一轮同次调用产出稳定短身份说明」（新增「关闭态第一轮提示与调用数逐字不变」）；`specs/aggregate-tagging/spec.md` ADDED 段（新增「关闭态第一轮提示与字段输出不变」）；`design.md` D2、D10；`tasks.md` T3.1、T10.2、T10.4；`test-cases.md` 故事 S6 步 1/2。

### R-11 2026-11-09【Medium】人工金标的标注时点与可见信息未固定 → 存在 gold 泄漏口径
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:351-352`、`:376-380`；`tasks.md:67-68`（T10.2/T10.3）；`test-cases.md:181-186`
- **事实**：spec 只写「金标 SHALL 由人工按真实上下文独立核验」「SHALL 只用于评分」；标注发生在两段评测跑完之后（`tasks.md:68`），此时人工可能已看到候选描述与系统输出；`tasks.md:67` 只约束「池不得按金标挑选」，不约束标注者可见信息。
- **影响**：标注者倾向按系统给出的实体 ID 划界 → 误合率被系统性低估，唯一效果门槛被污染；「金标不进生产」只管数据流向，管不了标注泄漏。
- **最小处置**：spec/tasks 补「金标 SHALL 在跑评测之前标注，且仅可见文章原文与提及上下文，不可见候选描述、系统输出与既有实体 ID」；工作表分「盲标版」与「复核版」两份。

- **处置与承接**：**处置**：运行前**冻结**协议要素（样本清单、文章快照、分组规则、模型与版本、预算清单、评分规则）；金标**在运行前**由与模型输出隔离的标注者按文章原文**盲标**形成；**运行器不读金标**，评测后由评分器 join；标注方式固定一种（盲标优先），冻结后不得因结果改样本、题目或规则；争议样本**运行前**预列为「不计身份分」并**报数量**；标注者不得改写查询描述或候选资料。工作表只含文章原文与提及上下文（**不含**候选描述、系统输出与既有实体 ID）。
- **承接位置**：`specs/entity-subject-resolution/spec.md` 新增 Requirement「评测协议冻结与金标隔离」（3 个 Scenario）；`design.md` D12；`tasks.md` T11.4、T12.1、T12.2；`test-cases.md` 故事 S7 步 1–3、白盒 B43–B45。

## 二、通过项（无需作者处理）

- **兼容边界与红线未松动**：event-story 的 4 条原 Scenario 逐字保留、0.92 阈值与 `subjectPreFilter` 一票否决不变；MODIFIED 只改「主体归一化」一条，与并发 change（事件指纹结构化产出/事件线数据模型/事件归属判定顺序）不重叠；`tasks.md:120-124`（13.1/13.2）指向的红线编号经核对正确（`docs/reference/flow/topic-graph.md:145` 为「本期不建独立实体表」条、`docs/reference/flow/semantic-board.md:384-385` 为 aux 三级去重两条）。
- **技术失败与 `none` 严格分开**：非法 ID / 不可解析 / 服务失败 / 超时四类各有独立 Scenario，均 SHALL NOT 降级为 none、SHALL NOT 新建；重试上限 2 次、耗尽保持「未归一」，与 FB-4 口径一致。
- **ai-summary 红线符合**：capability 取既有六类枚举内的 `topic_tagging`（`backend-go/internal/platform/airouter/store.go:21`），不新增枚举、不借 `digest_polish`；operation 非空（`router.go:483` 强制）；每次尝试成败都写审计，符合 `standard/backend/ai-logging.md`。
- **新增「稳定短身份说明」的失败语义正确**：必须由第一轮真实同次调用产出、空时不报错不丢弃标签、不写入 `event_fingerprint`、不改变既有指纹完整性判定（spec:52-63 + aggregate-tagging delta），与 FB-3 的「不得人工理想输入」口径一致。
- **不做的项守住**：无人工种子表、无词表/后缀/长度阈值洗身份、无相似度自动认同体、无待处理队列、无人工确认闸门（spec:170-173 明文 SHALL NOT）。
- **不越界宣称**：proposal 明确「本期不修跨天 story、不改 object/date/热度/事件线归属」，`active` 显式拒绝、生产默认 `off`，与 FB-5/FB-8 一致。
- **制品结构校验通过**：`openspec validate normalize-event-subject-entities --strict` → `Change 'normalize-event-subject-entities' is valid`。
- **Scenario 映射无遗漏**：`scenario-trace.sh` 报「60 个待对账 Scenario，45 项失败」，45 项**全部**是「映射文件不存在」（`entity_*_test.go`、`cmd/entity-resolve-eval/main_test.go` 等实现期文件）；**无一条「未映射」**。即 50+6+4 条 Scenario 在 `tasks.md` §14 映射表均有落点，制品阶段不要求现在补空测试或人工标签假通过。

## 三、核实记录（只读命令与代码/文档事实）

- `openspec validate normalize-event-subject-entities --strict` → valid（退出码 0）。
- `bash scripts/harness/scenario-trace.sh openspec/changes/normalize-event-subject-entities` → 60 个待对账 Scenario / 45 项失败，全部为映射文件不存在（无未映射项）。
- `bash scripts/harness/review-trace.sh openspec/changes/normalize-event-subject-entities` → 仅格式核验（本文件建账后复跑）。
- 代码事实（只读 grep，未编译、未跑测试、未连库）：`airouter/router.go:410`（provider 降级 `errors.Join`）、`:446-451`（embedding 缓存白名单仅 2 项）、`:502`（仅白名单 operation 才批量）、`airouter/store.go:20-25`（capability 六类枚举）；`tagmanagement/service/core/extractor_enhanced.go:310`、`:351`、`:669`（`subject_key` = 类型:规范名，模型同次产出，类型前缀即现成来源）；`topicgraph/service/story_subject_normalize.go`、`story_assign.go`（三层判定与一票否决，未改）。
- 文档事实：主 spec `openspec/specs/event-story/spec.md:31-53`（原「主体归一化」含「本期不建独立实体表」，本 change 的 MODIFIED 逐字承接其余段落）；`docs/reference/flow/topic-graph.md:145`、`docs/reference/flow/semantic-board.md:384-385`（红线编号核对）。
- 研究事实（引自 change 内 `research.md` 与 `docs/research/subject-entity-resolution/evaluation/main-review-qwen-eval-20261109-181947.md`）：Recall 旧四数已作废；重算口径 hit@1 9%–18%、hit@5 73%–100%、hit@10 91%（样本对、不可外推）；非自命中主组仅 6 题、候选仅 60 条、说明与候选描述经人工修订 → 不是生产准确率；「不在 Top10 → 合法 none → 重复新建」路径本轮未实测 → 本 change 不得以「重复新建 0」宣称风险已消除（与 R-3/R-5 的盲区一致）。

## 四、定点复核节（第二轮：作者修订后复核，2026-11-09）

> 复核口径：只核对本轮修订是否真正兑现 R-1..R-11 的**承诺侧**；**不改** R-1..R-11 的作者处置状态；下列为**文字级返修要求**（可一次修清），不新增功能、不要求在制品阶段补测试、不因实现期测试文件缺失而改动判定。

### 六项定点核对结果

| # | 核对点 | 结论 | 证据 |
|---|---|---|---|
| 1 | 全局闸门：会话级咨询锁 + 独立连接，覆盖「取得后重读 → 二判 → 原子登记」，第一轮在闸外，`finally` 释放并归还连接、等待有界 | **已兑现**；仅「只建一个」的结果承诺仍无条件 → R-12 | `specs/entity-subject-resolution/spec.md:223-262`、`design.md` D6、`tasks.md:46-51` |
| 2 | name/alias 只作候选入口：候选三路合并、按 `entity_id` 去重、同叫法可对应多实体产生多选项、跨来源不得按名字跳过判别 | **已兑现**；残留无条件表述见 R-13 | `spec:93-130`、`:295-318`、`design.md` D4/D8 |
| 3 | 效果口径：固定总 N + 覆盖率 + 技术失败数 + 空键「缺身份」/无决策「未归一」+ 配对可比子集 + 技术失败不为 0 即阻断 + 明禁「剔除失败样本后误合率更低」 | **已兑现** | `spec:458-547`、`design.md` D12 |
| 4 | 预算：按真实阶段四项分列、发送前计数、仅评测入口注入包装、无发送前计数能力则预先阻断；「提及数×2」「ceil(/32)」已删 | **已兑现** | `spec:380-430`、`design.md` D11、`tasks.md:75-77` |
| 5 | 关闭态：`off` 第一轮提示/schema/调用数逐字不变、零第二轮调用、零实体层写入；`shadow` 才启用新字段；本期事件线判定不读 entity ID（接线规则已移入 Out of Scope） | **已兑现**；一处措辞不一致见 R-14 | `spec:60-91`、`:319-379`、`specs/event-story/spec.md` |
| 6 | 金标隔离：协议运行前冻结、运行器不读金标、盲标工作表只含原文与提及上下文、争议样本运行前预列；旧小测仍标注「不是生产准确率/不可外推」 | **已兑现** | `spec:432-457`、`tasks.md:81-84`、`research.md:26-29` |

### R-12 2026-11-09【High】跨来源并发场景仍是无条件「只建一个 / 同一实体 ID」承诺，与 none 即新建、跨来源不得按名字跳过判别互斥
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:233-237`（场景）、`:229-231`（闸门重读条）、`:271-273`（none 即新建）、`:313-318`（跨来源不得按名字跳过判别）；`design.md:83`（D6 取舍括注）；`tasks.md:51`（T7.5 首项）
- **事实**：闸门只保证「串行 + 取得后重读 + 不提交闸外旧 none」。第二个进程在闸内看到首个新建的实体后仍须走第二轮；若模型判 `none`（口径允许：上下文不同即不同实体、同名不同体不得强合），按「none 即新建」会建出第二个实体——此时场景的「实体层 SHALL 只出现一个对应的新实体 / 两条提及 SHALL 指向同一实体 ID」被违反，而这是**合法**行为。D6 取舍括注「后者看到前者刚登记的实体 → 复用或判 none 而不再双建」自相矛盾（判 none ⇒ 新建 ⇒ 即双建）；T7.5 首项仍把「跨来源同提及并发只建一个实体」列为用例。
- **影响**：保留了本轮 review R-1/R-2 想消除的**魔法 100% 归一**承诺；实现者只能靠硬编码假模型或改坏 none 语义才能让该场景为绿。
- **最小处置（纯文字）**：把场景改为断言机制保证 + 显式允许分支，例如 THEN 第二个处理者 SHALL 在闸内基于最新池判定且 SHALL NOT 提交闸外旧 none；AND 当其判定选中已登记实体时两条提及 SHALL 指向同一实体 ID；AND 其判定为 `none` 时 SHALL 按 none 路径新建（不视为违约，重复由重复身份组率/覆盖率量化）。同步修 D6 括注与 T7.5 首项措辞。

- **处置与承接**：**处置（纯文字）**：场景改名为「跨来源并发按最新候选重新判定」，WHEN 保留「两来源并发、闸外池内无候选」，THEN 改为「后取得闸门者 SHALL 闸内重读并按**最新候选**二判、SHALL NOT 提交闸外旧 `none`」，AND 显式分两支：① 新判定**选中已登记实体** → 两条提及指向同一实体 ID；② 新判定为**合法 `none`** → 按 none 路径新建，**不算违约**（重复由重复身份组率与覆盖率量化）；并加一句「闸门 SHALL NOT 被宣称为保证『只建一个』的唯一机制」——即该项作为**效果评测项**而非闸门承诺。design D6 取舍括注改为「复用或按最新 `none` 新建；**锁只防陈旧结论提交，不保证模型判别正确**」；tasks T7.5 首项同步并要求**分别 stub 两个分支**、不得用姓名唯一性替代；映射表与 test-cases 同步改名与断言。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「跨进程全局实体解析闸门」→ Scenario「跨来源并发按最新候选重新判定」；`design.md` D6 取舍段；`tasks.md` T7.5 与映射表；`test-cases.md` 故事 S2 步 8、变体 3.4 #4、白盒 B20/B21/B21b。

### R-13 2026-11-09【Medium】叫法归并场景与用例故事句仍是无条件 100% 归一，与 design 自述「不作无条件保证」互斥
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:30-34`（场景「同一实体的多种叫法归入同一实体」）、`:100`（候选为空即新建）；`test-cases.md:3`（故事句「三种写法最终指向同一个 ID」）；`design.md` D4/D6 取舍（明写「冷启动/未召回只能靠向量召回，这部分召回率必须由效果指标如实呈现，不作无条件保证」）
- **事实**：按 D4/R-5 处置，叫法登记命中只是候选入口，不是身份硬复用；两种叫法若第二次未被召回（名路径未命中、登记名不等），只能判 `none` → 新建实体 → 两条提及指向不同 ID。但场景与故事句仍无条件要求「指向同一实体 ID / 三种写法最终指向同一个 ID」。
- **影响**：实现者可能被推向已被驳回的「按名字硬复用」路径，或被迫让场景恒假；与 R-5 的处置结论（只接事实缺口、效果由指标量化）口径不一致。
- **最小处置（纯文字）**：给场景加前提（两处叫法均被召回且选中同一实体时 SHALL 指向同一 ID）或改写为效果口径表述；`test-cases.md:3` 故事句改为「同一实体被召回并选中时指向同一 ID；未召回或模型判 `none` 允许新建另一实体，重复由重复身份组率与覆盖率量化」。

- **处置与承接**：**处置（纯文字）**：spec 场景「同一实体的多种叫法归入同一实体」加前提——**后续提及的候选集合已包含该 entity 且模型在该次判定中选择了该实体 ID**，THEN 两条提及指向同一 ID 且名称与证据层记录两个叫法，并补一句「未被召回或模型判 `none` 时新建另一实体 SHALL NOT 视为违约（真实归一率由效果评测衡量）」。**笔误修正**：`test-cases.md` 开头故事句原写「特朗普 / Trump / 老黄三者同一 ID」——**老黄是黄仁勋，不是特朗普**，已改为「特朗普 / Trump / Donald Trump 在被召回且被选中时同 ID；老黄按上下文指向另一个实体（黄仁勋），同类人不得合并」。spec `## Purpose` 同步改写为「特朗普/Trump 可指同体；老黄按上下文指向另一个实体；不作无条件归一承诺」。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「全局实体身份层与名称证据层」→ Scenario「同一实体的多种叫法归入同一实体」+ `## Purpose` 段；`test-cases.md` 开头故事句与故事 S1 步 6 / S2 步 5；`design.md` D4 取舍段（已写明「不作无条件保证」）。

### R-14 2026-11-09【Low】影子模式写入口径与 metadata 落点措辞不一致，可能挡住评测所需的第一轮产出
- **状态**：已修复
- **文件行号**：`specs/entity-subject-resolution/spec.md:359`（`shadow` SHALL 只写实体层与审计；SHALL NOT 改写事件标签…）、`:65`（稳定短身份说明 SHALL 落在独立 metadata 键）；`proposal.md:55`
- **事实**：`shadow` 下第一轮产出的 `subject_desc` 落在 `topic_tags.metadata.entity_subject`（对标签行的写入），而 `shadow` 条写的是「只写实体层与审计、SHALL NOT 改写事件标签」。评测入口吃的正是这份影子路径持久化结果（`spec:381`）。
- **影响**：严格照字面实现会让影子态不落该 metadata → 评测拿不到第一轮产出 → §12 无法执行。
- **最小处置（纯文字）**：`shadow` 条补一句「SHALL 只写实体层、审计与 `topic_tags.metadata.entity_subject` 独立键（inert，无读路径），SHALL NOT 改写标签的展示与聚类语义」。

### 复核核实记录（只读）

- `openspec validate normalize-event-subject-entities --strict` → `Change 'normalize-event-subject-entities' is valid`；`openspec status --change …` → `Progress: 5/5 artifacts complete`（planningComplete）。
- `bash scripts/harness/check-tasks-tail.sh …` → 通过（尾三节与 doc-impact 声明齐全）。
- `bash scripts/harness/review-trace.sh …` → 通过（本节新增 R-12..R-14 后条目计数上升，格式仍合法）。
- `bash scripts/harness/feedback.sh verify normalize-event-subject-entities` → 退出码 0（脚本提示「无 human-feedback.md（未启用反馈建账）」，目录内 `human-feedback.md` 存在但未被该脚本识别；不影响制品判定，不在本次返修范围）。
- `bash scripts/harness/scenario-trace.sh …` → 83 个待对账 Scenario / 64 项失败，**全部**为「映射文件不存在」（`entity_*_test.go`、`entity_gate_test.go`、`cmd/entity-resolve-eval/main_test.go` 等实现期文件），**0 条未映射**；按约定不因此要求补空测试，也不因此要求停下 propose。
- 未跑测试、未编译、未调模型、未连库、未改任何实现/生产文件/DB/配置；本次仅追加本节。

- **处置与承接**：**处置（纯文字）**：影子写入口径统一为**三处**——① 实体层；② 正常 AI 审计（`ai_call_logs`）；③ `topic_tags.metadata.entity_subject` **独立惰性键**（仅影子态第一轮产出、当前无读路径）。spec 明写该三处 SHALL NOT 改写 `event_fingerprint`、标签业务字段与归属、story / report / topic 任何字段与界面可见字段；关闭态 `off` 全不变（并补「不写 `entity_subject` 键」）。**两种入口区分写清**：应用常规打标在 shadow 下持久化惰性 metadata；**评测 CLI 不写标签业务表**，其第一轮原始结果存评测产物与实体层证据——评测入口不因此获得写真库的授权。
- **承接位置**：`specs/entity-subject-resolution/spec.md`「影子模式与生产开关边界」（Requirement 正文 + Scenario「影子态写入口径限定为三处」「默认关闭态零额外调用」）+「本期身份判定零改动」的影子条；`proposal.md`「影子模式 + 有界评测入口」与 Impact；`design.md` D10 与 Migration Plan 第 3 步；`tasks.md` T10.3 与映射表；`test-cases.md` 开头性质段、故事 S6 步 3、白盒 B30。

## 五、R-12..R-14 收口记录（第三轮：作者文字收口，2026-11-09）

> 本轮只做**制品文字收口**（不扩设计、不新增功能/表/场景、不实现）。R-1..R-11 的作者处置结论**保持不变**；它们仍**未被实现验证**（本 change 尚无实现，测试文件未落地），本记录**不把任何条目改成「已通过/已实现」**。

| 条目 | 状态 | 最小改动位置（文件:位置 → 改了什么） |
|---|---|---|
| R-12 | 已修复（文字） | `specs/entity-subject-resolution/spec.md`「跨进程全局实体解析闸门」→ 场景改名为「跨来源并发按最新候选重新判定」并加两分支 + 「不作闸门承诺」句；`design.md` D6 取舍括注 → 「复用或按最新 `none` 新建；锁只防陈旧结论提交，不保证模型判别正确」；`tasks.md` T7.5 首项措辞 + 要求分别 stub 两分支；`tasks.md` 映射表该行改名；`test-cases.md` S2 步 8、变体 3.4 #4、白盒 B20/B21/B21b |
| R-13 | 已修复（文字） | `specs/entity-subject-resolution/spec.md`「同一实体的多种叫法归入同一实体」→ 加「候选已含该实体且模型选中该 ID」前提 + 「未召回/判 none 不算违约」；同文件 `## Purpose` → 「特朗普/Trump 可指同体；老黄指向另一实体；不作无条件归一承诺」；`test-cases.md` 开头故事句 → **老黄笔误修正为黄仁勋**，同体示例改为特朗普/Trump/Donald Trump；故事 S1 步 6、S2 步 5 同步前提 |
| R-14 | 已修复（文字） | `specs/entity-subject-resolution/spec.md`「影子模式与生产开关边界」Requirement 正文 + Scenario「影子态写入口径限定为三处」「默认关闭态零额外调用」；同文件「本期身份判定零改动」影子条；`proposal.md` 影子模式条目；`design.md` D10 决策句 + Migration Plan 第 3 步；`tasks.md` T10.3 + 映射表该行改名；`test-cases.md` 开头性质段、故事 S6 步 3、白盒 B30 |

**本轮未做**：未新增 Requirement/Scenario/表/任务；未创建任何测试文件；未改实现、生产代码、主 spec、DB、配置；**0 次 Chat/Embed 调用、0 条业务数据写入**。

## 六、用户要求精简的收口记录（第四轮：故事化压缩，2026-11-09）

> 用户要求「任务太多太冗长，包括测试用例也是，把主要的用户故事覆盖全就行」。本轮按主线程确定的 **5 个主故事**（S1 新主体首次出现 / S2 跨叫法跨板块复现 / S3 相关但不同 / S4 并发与重放 / S5 真实效果且不影响现网）**实质减量**重排制品。

| 项 | 上一版（历史） | 本版 | 说明 |
|---|---|---|---|
| delta Scenario | 83 | **25** | entity-subject-resolution 18 + event-story MODIFIED 5（原 4 条逐字保留 + 影子边界合成 1）+ aggregate-tagging ADDED 2 |
| tasks checkbox | 66 | **7** | 4 个核心交付（T1 实体存储与证据层 / T2 完整解析链 / T3 并发闸门与影子接入 / T4 效果评测入口）+ T5 review + 文档节 2 条；尾三节按规范保留 |
| test-cases | 45 分支 / 12 边界 | **10 条高风险分支** + 5 故事主链路表 | 失败变体（技术失败非 none、歧义误合、并发旧结论、资源释放、off 回归、预算拦截、金标泄漏）就近合入故事；⓪ 继承、五组变体、展示字段盘点压成小表 |
| design 决策 | D1–D13 | **D1–D12**（D13 文件边界并入 Risks 与 D1 落点） | 每条保留「落点 + 取舍」，落点改为新任务 id 与新 Scenario 标题 |

**口径未被削弱**：功能、安全与错误路径一条未删；「不承诺无条件归一」「锁只防陈旧结论提交、不保证模型判别正确」「关闭态第一轮逐字不变」「影子只写三处」「评测需单独授权、金标隔离」「未达标不启用生产」全部保留。**R-1..R-14 的处置结论保持不变**，本记录**不把任何条目改成「已通过/已实现」**——本 change 仍无实现、测试文件仍未落地。

## 七、实现评审（T5，2026-11-09，只读子线程 + 主线程裁决）

> 范围：T1–T4 实现 + 文档 4.1/4.2（已建 `tasks.md` 勾选）。判据 = D1–D12 + 3 份 delta + 上文 R-1..R-14 的承诺。工作区含 9 个并发 change 的既有脏改动，本评审已区分「本 change 增量」与 `event_fingerprint` 等他人 hunk。
> 结论：**High 0 / Medium 3 / Low 5**；闸门纪律、技术失败与 `none` 分离、存储层契约、off 逐字不变、影子三处写入、金标隔离、评分口径经逐条核对**真实落地且测试可失败**。
> 主线程裁决：R-15/R-16/R-17（Medium）**全部修**；R-18 采纳为口径补充（不改幂等键、不动索引）；R-19/R-20/R-21/R-22 按最小改法修。

### R-15 2026-11-09【Medium】评测预算报告 actual 侧半接线：三个「实际」字段恒 0，实际重试数与错例原因不可从产物复算
- **状态**：已修复
- **位置**：`cmd/entity-resolve-eval/budget.go:79-81`、`runner.go:Run`、`scoring.go:MentionRecord`
- **事实**：`BudgetReport.ActualExtraction/ActualSecondRound/ActualEmbedTexts` 全代码库无写入点；`Runner.Run` 只回填 `Rejected`/`ActualHTTP`；guard 的 `embedTexts` 无 getter；resolver 的 `Resolution.Attempts` 未进 `MentionRecord`；`Reason` 只在 hook 路径落 metadata，eval 路径丢弃。
- **违背**：D11 ②「二判次数 = 实际提及数 + **实际发生的有限重试**」、③「embedding 提交文本数含重复发送与模型降级重复」；spec「预算清单 SHALL 分别列出……二判次数与实际重试」。
- **裁决**：**修**。guard 暴露 `EmbedTexts()`；`Runner.Run` 聚合 `Attempts` + chat/embed 计数回填 actual 三项；`MentionRecord` 增 `Attempts`/`Reason`，错例清单随 records 落盘。

### R-16 2026-11-09【Medium】CLI `run` 在授权文件验过 BUDGET-APPROVED 后仍无条件拒绝，真实执行装配未实现
- **状态**：已修复
- **位置**：`cmd/entity-resolve-eval/main.go:cmdRun` 末尾
- **事实**：校验授权文件 + `VerifyFrozen` + 样本摘要通过后，仍 `return fmt.Errorf("run 未执行：当前无授权…")`；`NewRunner→Run→落 records` 无任何 CLI 调用方，错误消息与已验证事实矛盾。
- **违背**：T4「效果评测入口」可用性停在半程——授权下发后不改代码无法启动；运维会被错误消息误导。
- **裁决**：**修**。授权路径通过后走完整装配（连库 → `NewRunner` → `Run` → 落 records 产物）；无授权与「有授权但装配失败」分成两个明确分支；测试仍零网络、零真库（只在断言层面验证分支）。

### R-17 2026-11-09【Medium】评测入口运行主路径（NewRunner→Run）零测试覆盖
- **状态**：已修复
- **位置**：`cmd/entity-resolve-eval/main_test.go`
- **事实**：13 个子测试全为纯函数/反射/guard 单元级，`grep NewRunner|\.Run(` 在该文件 0 命中；`Run` 的争议项跳过、样本漂移前置拒绝、`TechFailure`/`EntityID`/`Status` 落 record、guard 计数回填从未被执行。B9 仅反射结构断言，B8 未验证 guard 经 `RegisterClient` 挂到 router 后真的拦截 resolver 调用。
- **违背**：T4 验证要求（`go test ./cmd/entity-resolve-eval` 绿）覆盖不到运行主路径；授权后首跑才暴露装配级 bug 的风险。
- **裁决**：**修**。补零网络集成用例：testcontainer PG + fakeProviderClient，走 `NewRunner→Run`（3–5 样本含 1 争议项），断言 records 形状、争议项不产 record、guard 计数 >0。

### R-18 2026-11-09【Low】幂等键无文章维度：两篇文章复用同一 event 标签行时第二篇不重新判定
- **状态**：已修复（仅口径补充；未改幂等键与 partial 唯一索引）
- **位置**：`service/entity/resolver.go:ResolveMention`、`hook.go:RunForTag`
- **事实**：`topic_tags` 跨文章共享，幂等键 =（`source_tag_id`, mention_key）；两文产出逐字同 label → 同一行 → 第二篇幂等短路跳过二轮。制品自身把「来源=来源标签」（test-cases S4 步 2 明写「来源标签不同」），实现与制品一致，属制品口径与数据模型之间的缝隙。
- **裁决**：**采纳为口径补充**（不改代码、不改索引）。在 spec「并发、重放与跨进程闸门」与 design D8 各补一句：来源标签相同的跨文章复用视为**同一来源**；按文章维度重判属后续 change（需改 partial 唯一索引）。

### R-19 2026-11-09【Low】MaxOptions 截断把「已登记叫法精确命中」置于 score=0 最低优先级
- **状态**：已修复（截断保留 registered 命中 + 专项用例）
- **位置**：`service/entity/candidate.go:MergeCandidates`
- **事实**：registered hits 以 `add(id, 0, "registered_name")` 进入、按 `BestScore` 降序截断；池增大后 12 席可被向量高分占满，导致唯一持有该叫法的实体被挤出（「老黄」→黄仁勋的 name 向量可能很低，registered-name 是其唯一召回路径）→ 候选空 → none → 重复新建。
- **裁决**：**修**（最小改法）：截断时先保留全部 registered 命中，再按分数补足向量路；`entity_candidate_test.go` 补一条「registered 命中不被向量高分挤出」。

### R-20 2026-11-09【Low】enhanced（mono 合并）路径的影子接线零测试
- **状态**：已修复（enhanced off/shadow 双层 golden）
- **位置**：`service/core/extractor_enhanced.go`（shadow 分支）
- **事实**：section 路径有双层 golden，enhanced 路径平行接线（`buildMergedExtractionPromptShadow`/`mergedTagExtractionSchemaShadow`）无 `subject_desc`/shadow 断言 → off 逐字性与 shadow 前缀性只靠同构代码「看起来对」。
- **裁决**：**修**。把 section 的两条 off/shadow 断言同款落到 `extractor_enhanced_test.go`（同款 recording router）。

### R-21 2026-11-09【Low】section off-golden 锚定了并发 change 的 prompt 行
- **状态**：已修复（纯注释，断言不变）
- **位置**：`service/core/extractor_section_test.go:TestSectionExtraction_OffPromptAndSchemaUnchanged`
- **事实**：golden 冻结的「事件指纹（仅 event 标签输出…）」等行由并发 change（add-event-story-heat）引入；该 change 归档前微调措辞会让本测试红，易被误读为本 change 回归。
- **裁决**：**修**（纯注释）：在 golden 处标明这些锚点属 `add-event-story-heat`，其措辞变更需同步本 golden；断言不变（跨 change 耦合是 design Risks 已声明的已知代价）。

### R-22 2026-11-09【Low】影子 hook 在打标管线内同步执行，配置文档未标注延迟代价
- **状态**：已修复（纯文档）
- **位置**：`docs/reference/configuration.md` shadow 行；`service/core/article_tagger.go:persistArticleTags`
- **事实**：event 分支同步调用 `EntitySubjectHook`（无 goroutine），`DefaultResolverConfig` TotalTimeout 5min / GateWaitTimeout 120s / CallTimeout 60s；单 event 标签最坏 5 分钟，3 标签文章最坏约 15 分钟；全局单键闸门串行 → 吞吐 ≈ 1/闸内时长（design Risks 已明示为代价），但配置文档未提示。
- **裁决**：**修**（纯文档）：`configuration.md` shadow 行补「打标管线单 event 标签同步增加归一时长（有界 ≤5min/提及，全局闸门串行）；建议低峰或小流量验证」。
