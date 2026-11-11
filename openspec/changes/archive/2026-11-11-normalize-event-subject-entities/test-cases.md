# test-cases: 主体实体归一（全局稳定实体 ID）

**故事**：一篇文章说「特朗普宣布…」，系统要判断这个「特朗普」是哪个特朗普——落到已登记实体，还是登记一个新实体。登记下来的实体有稳定 ID，于是「特朗普」「Trump」「Donald Trump」在被召回且被选中时指向**同一个 ID**；未被召回或模型判 `none` 时允许新建另一个实体（重复由重复身份组率与覆盖率量化，**不保证 100% 归一**）。「老黄」是**黄仁勋**的昵称、与特朗普**不是同一实体**。

**本 change 的性质**：实体身份层以**影子模式**落地——真建实体、真归一提及，写入只限**实体层 + 正常 AI 审计 + `topic_tags.metadata.entity_subject` 独立惰性键**（仅影子态第一轮产出、无读路径），**不改** `event_fingerprint`、标签业务字段与归属、story/report/topic 与任何界面可见字段。本期的用户是**运维/评审者**（看评测报告的人），终端读者的故事等生产启用后的 change。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为**待实现 / 待执行**用例。

## 1. 五个主链路故事

### S1 新主体第一次出现（锚 Requirement：主体提及的完整解析链 / 实体身份与证据存储）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 真实文章跑影子态第一轮，产出 event 标签 | 影子态同一次调用产出主体原文与稳定短身份说明 | 标签带主体原文 + 短说明，**该片模型调用次数仍为 1**；`event_fingerprint` 结构不变 | 函数单测 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 2 | 池内无候选 → 检索 → 二轮判 `none` → 原子登记 | 新主体首次出现走完抽取到原子登记 | 建出 1 个实体、提及归到它；身份行 + 名称证据行 + 溯源行**同一短事务**写成，**无半实体** | 函数单测 + 集成（隔离 PG） | `backend-go/internal/tagmanagement/service/entity/entity_resolution_test.go`、`entity_repository_test.go` |
| 3 | 短说明为空时再走一次 | 稳定短身份说明为空回退仅名检索 | 走**仅名**路径，不报错、不丢标签、不半写 | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_retrieval_test.go` |

**失败变体（就近合入）**：二轮返回非法 ID / 不可解析 / 服务失败 / 超时 → 判**技术失败**，**不降级为 none、不新建、不半写**，最多重试 2 次，耗尽后保持「未归一」并记原因，不阻断标签入库、不进业务等待队列。（→ `合法 none 与技术失败是不同结论`）

### S2 后续跨叫法 / 跨板块再次出现（锚 Requirement：主体提及的完整解析链）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 同一主体换个叫法（"Trump"）再出现 | 后续跨叫法跨板块再次出现合为一个 ID 选项 | 三路候选（名向量 ∪ 名+短说明 ∪ 已登记叫法命中）合并后该实体**只一个 ID 选项**；模型选中后**沿用同一统一 ID**，原文与证据随实体留存 | 函数单测 + 集成（隔离 PG） | `backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go`、`entity_identity_test.go` |
| 2 | 跨板块提及同一主体 | 同上 | 实体**全局共享**，与板块无关 | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_identity_test.go` |
| 3 | 两个实体同名（都被叫"苹果"） | 同名不同实体可并存 | 两个 ID 各自保留、**不硬复用**；模型按上下文选 | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go` |

**失败变体（就近合入）**：真实未召回（名路径未命中且叫法未登记）→ 只能判 `none` → 新建另一实体；此路径**不承诺 100% 归一**，由重复身份组率与覆盖率如实评测。（→ `后续跨叫法跨板块再次出现合为一个 ID 选项`）

### S3 相关但不同（锚 Requirement：主体提及的完整解析链）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 候选里同时有产品与公司、团队与公司、人物与政府 | 相关但不同的候选不被强合 | **不因向量相近 / 名称相似 / 类型相同而合并**；判 `none` 时新建另一实体，**不因"看起来相关"而选任一候选** | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go` |
| 2 | aux 行的自动 alias 与本次提及相关 | 辅助标签别名不构成同体证据 | aux 的自动 alias **不**作同体证据、**不**单独成为候选；未关联 aux 只作资料 | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go` |

**失败变体（就近合入）**：技术失败与合法 `none` 的区别见 S1；本故事重点是**误合**——错合并的代价高于漏合并，由误合率（固定总 N 口径）量化。

### S4 并发与重放（锚 Requirement：并发、重放与跨进程闸门）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 同来源同提及处理两次 | 同来源同提及重放幂等 | 同一实体 ID、**不重复模型调用**（审计只有一次判定尝试） | 函数单测 | `backend-go/internal/tagmanagement/service/entity/entity_resolution_test.go` |
| 2 | 两进程跨来源并发同一提及键 | 跨来源并发在闸内重读最新候选再二判 | 后取得闸门者**闸内重读**并按最新候选二判、**不提交闸外旧 none**；新判定选中已登记实体 → 同一 ID；新判定合法 `none` → **允许新建**（不算违约）。**锁不保证实体全局不重** | 集成（隔离 PG） | `backend-go/internal/tagmanagement/service/entity/entity_gate_test.go` |
| 3 | 进程被取消 / 异常 / 审计失败 / 等待超时 | 取消超时异常释放闸门与连接 | 闸门释放、**连接归还**；以明确技术失败结束，不新建、不半写 | 集成（隔离 PG） | `backend-go/internal/tagmanagement/service/entity/entity_gate_test.go` |

**失败变体（就近合入）**：旧结论提交（闸外 none 直接落地）是本故事的核心失效模式 → 用「闸外结论作废」用例守住；禁止用姓名唯一性替代该判定。

### S5 看真实效果且不影响现网（锚 Requirement：影子接入与既有判定零改动 / 有界评测入口与效果口径）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 默认 `off` 跑打标 | 默认关闭态逐字不变 | 第一轮提示与 schema **逐字不变**、零新增调用、零写入（含不写 `entity_subject` 键） | 函数单测 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go`、`entity_mode_test.go` |
| 2 | 开影子 | 影子态写入限定三处 | 实体层 + 正常 AI 审计 + `entity_subject` 惰性键有新增；`event_fingerprint`、标签业务字段与归属、事件线/日报/话题/可见字段**不变** | 函数单测 + 人工（只读核对） | `backend-go/internal/tagmanagement/service/entity/entity_mode_test.go` |
| 3 | 事件线身份判定 | 事件线身份判定不读实体 ID | 仍只按 `subject_key` 三层 + 一票否决，判定路径**不读实体 ID**；人工归属不覆盖、存量不回填 | 既有测试（不改断言） | `backend-go/internal/topicgraph/service/story_subject_normalize_test.go` |
| 4 | 评测入口跑一轮 | 评测协议与金标在运行前冻结并隔离 | 样本/快照/分组/模型版本/预算/评分规则**运行前冻结**；**运行器不读金标**；争议项运行前预列并报数；**评测不写标签业务表**（第一轮原始结果落评测产物与实体层证据） | 集成（隔离 PG） | `backend-go/cmd/entity-resolve-eval/main_test.go` |
| 5 | 预算与评分 | 预算超限在发请求前被拦截 / 评分按固定总 N 与覆盖率并单列缺身份与未归一 | 超限**发请求前**拦截并计数；评分给固定总 N、覆盖率、技术失败数、错误复用数与按固定总 N 的错误率；空 `subject_key` 单列「缺身份」、无决策单列「未归一」，**不以单元素组冒充正确**；误合/漏合/重复身份组率在**同一可比子集**上算；分母为 0 报「不适用（N=0）」 | 函数单测 | `backend-go/cmd/entity-resolve-eval/main_test.go` |
| 6 | 出结论 | 未达标不启用生产 | 未达标 → 生产归一保持关闭、任务保持未完成并留痕 | 人工 | `evaluation-report.md`（未产出前留空，**不预填**） |

---

## 2. 旧资产继承（⓪）

已跑 `test-assets.sh event-story` / `aggregate-tagging`。本 change 触及 1 条 MODIFIED（event-story「主体归一化」）+ 1 条 ADDED（aggregate-tagging）。MODIFIED 的 4 条既有 Scenario **逐字保留、断言不改**；新增边界合成 1 条。

| 旧契约 | 处置 | 旧测试文件 | 动作 |
|---|---|---|---|
| 跨语言别名归并 / 同公司不同写法归并 / 不同主体不因向量相近而合并 / 三层均不命中时新建 | 继承 | `backend-go/internal/topicgraph/service/story_subject_normalize_test.go` | 照跑，**断言不改**（事件线侧行为未变）；同文件追加 1 条影子边界用例 |
| 主 spec「本期不建独立实体表」 | **改语义** | 同上 | 措辞改为「实体身份层存在但本期不参与事件线身份判定」；测试断言不变 |
| aggregate-tagging 指纹字段集（subject/subject_key/action/object/occurred_at） | 继承 + 扩充 | `core/extractor_section_test.go`、`extractor_enhanced_test.go`、`tagger_fingerprint_test.go` | 既有断言**全不改**；同文件追加影子态产出与关闭态原样两条 |
| topic-graph 红线 13、semantic-board 红线 1/2、ai-summary 红线 | 继承 | 既有测试照跑 | 本 change 不改 aux 三级去重、不新增 capability、不借 `digest_polish` |
| 并发 change（`enable-story-cross-day-continuation` / `unify-vector-column-dimension`） | 只做回归 | — | 本 change 不改 `board_stories` 结构、`story_assign.go`、`story_fingerprint.go`、向量列 typmod |

## 3. 变体走查（摘要）

| 组 | 关键变体 | 期望 |
|---|---|---|
| 输入 | 空串 / 纯空白 / 单 token / 大小写 / 引号与 emoji（提示注入）/ 超长 | 空/纯空白不归一不报错；大小写不直接判异体；注入文本只作数据；超长走截断保护 |
| 前置 | 池空 / 池 1 项 / 候选同名多实体 / 候选含失效实体 / 跨板块提及 | 各自有确定答案（同名多实体→多选项；失效实体不出现在候选；跨板块允许） |
| 时间窗口 | 同日重复 / 隔天再出现 / 跨时区 | 幂等命中；实体不按时间过期；归一不依赖日期 |
| 幂等 | 同来源重放 ×2/×3 / 并发同提及 / 事务中途失败 / 闸内异常 | 幂等不重复调用；闸门串行 + 重读；失败无半实体；资源归还 |
| 可用性 | 空态（报告分母为 0）/ 错误态（技术失败原因可见）/ 重复提交 | 报告显式「不适用（N=0）」；失败原因落审计不静默；重复提交=幂等 |

不适用项：UI 误输入反馈 / 加载态 / 弹窗尺寸 / 双视口 / agent-browser 端到端 / 前端组件测试（`ui-impact: none`，无界面改动与读路径）；迁移回填验证（只建 3 张空表，无历史回填）。

## 4. 效果核对（问句④）

- **触发原因**：本能力的产出取决于**真实模型在真实上下文里判得对不对**，绿测只证明协议没写错。
- **核对方法**：两段有界真实评测（需**单独授权**）：① **冷启动连续处理**（池从全空起，按固定时间序处理真实样本：≥30 条非自匹配的已有实体提及 + ≥10 条真实 none，跨 ≥3 板块）；② **固定已有池重放**（冻结第一段产出的**未经金标修正**的池）。金标**运行前**由隔离标注者按原文盲标，**运行器不读**、评分器事后 join；「池中是否有金标实体」只由评分器依顺序历史金标**事后标注**。
- **量化结果**：**待实测**（本阶段 0 次模型调用；上一轮 25 次 Chat 授权已消耗完，本期只出预算清单与授权流程）。报告须含：固定总 N、覆盖率（两套各一份）、技术失败数、误合、漏合、重复身份组率、重复新建率、统一 ID 稳定性、**闸门等待与总解析时长**（本期串行化代价）、各阶段调用/用量/HTTP 次数、**全部错例清单**。
- **结论**：**待实测**。门槛（写死）：同一可比子集上误合不高于既有身份、重复身份组率低于既有身份、覆盖率不低于既有身份、技术失败为 0；数据不足 / 基线分母 0 / 某段未测 → **不判达标、任务保持未完成**；未达标 → **不启用生产**。

## 5. 展示字段盘点（问句⑤）

本 change **不新增任何界面可见字段**；所有新增字段当前都无读路径。

| 字段 | 界面可见 | 来源语义 | spec 锚 |
|---|---|---|---|
| `topic_tags.metadata.entity_subject` | 否（仅影子态写入、无读路径） | 影子态第一轮同次调用产出 | aggregate-tagging ADDED + entity capability「影子接入与既有判定零改动」 |
| `entity_registry` / `entity_mentions` / `entity_aux_links` | 否 | 归一服务写入 | entity capability「实体身份与证据存储」 |
| `board_stories.subject_key` / 事件线归属 / 热度 | 是（既有展示） | **本 change 不改** | event-story MODIFIED「主体归一化」 |

若后续 change 要把归一结果呈现在界面，须先补 Requirement 声明展示字段来源语义，再改代码。

## 6. 白盒高风险分支（节选，判据主线程定）

| # | 分支 | 期望 | 用例名 |
|---|---|---|---|
| B1 | 二轮返回非法 ID / 不可解析 / 服务失败 / 超时 | 技术失败；不降级 none、不新建、不半写；≤2 次尝试 | `TestEntityResolver_TechnicalFailureDistinctFromNone` |
| B2 | 同名多实体候选 | 保留多个 ID 选项，交模型按上下文选，不硬复用 | `TestEntityCandidate_SameNameDistinctOptions` |
| B3 | 闸外旧 `none` | 闸内重读后按最新候选重判，不提交旧结论 | `TestEntityGate_ReReadsAfterAcquire` |
| B4 | 并发两分支 | 选中已有 ID → 同 ID；判 none → 允许新建（不算违约） | `TestEntityGate_Concurrent_BothBranches` |
| B5 | 闸内取消 / 异常 / 超时 | 闸门释放 + 连接归还；技术失败结束 | `TestEntityGate_ReleaseOnCancelOrTimeout` |
| B6 | 关闭态回归 | 第一轮提示与调用数逐字不变、零写入 | `TestEntityMode_OffIsNoop` |
| B7 | 影子写入范围 | 只写三处；其余数据不变 | `TestEntityMode_ShadowWritesOnlyThreePlaces` |
| B8 | 评测预算 | 超限在发请求前拦截并计数 | `TestEntityEval_BudgetGuardBlocksRequest` |
| B9 | 金标泄漏 | 运行器不读金标；金标不进生产写入路径 | `TestEntityEval_RunnerNeverReadsGold` |
| B10 | 评分分母 | 固定总 N + 覆盖率；缺身份/未归一单列；分母 0 报不适用 | `TestEntityEval_FixedNAndCoverage` |
