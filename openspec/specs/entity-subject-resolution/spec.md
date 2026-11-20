# entity-subject-resolution Specification

## Purpose

本能力提供**全局稳定的实体身份层**与**主体提及归一服务**：把文章里的主体提及（"特朗普" / "Trump" / "Donald Trump"）归一到可登记、可核验的实体 ID 上，判定口径是「检索只负责召回、模型只负责判定是否**同一实体**」。同一实体被召回且被模型选中时归到同一 ID；未被召回或模型判 `none` 时允许新建另一个实体（重复由重复身份组率与覆盖率量化，不作无条件归一承诺）。「老黄」是**黄仁勋**的昵称、与特朗普**不是同一实体**，按上下文指向另一个实体。它与事件线身份（`subject_key` 三层判定）是两套并行身份：本期实体身份层以影子模式落地，**不参与既有事件线身份判定**（实体层自身仍照常做身份判别）。

## Requirements

### Requirement: 实体身份与证据存储

系统 SHALL 提供三张表承载全局实体身份：身份行（`entity_registry`）、名称与证据行（`entity_mentions`）、辅助标签溯源行（`entity_aux_links`）。

- 实体 ID SHALL 全局稳定（不复用）；辅助标签被 GC 置 disabled 或被删除时 SHALL 只移除溯源行，SHALL NOT 使实体身份失效或级联删除实体。
- 实体规范名 SHALL NOT 全局唯一：不同实体 SHALL 允许同名并存；SHALL NOT 以名称唯一约束或相似度自动合并两个实体。
- 实体类型 SHALL NOT 进入身份层：SHALL NOT 由模型猜测或由 `subject_key` 前缀推导并写入身份。
- 名称与证据层 SHALL 记录已登记叫法与逐次提及的原文证据；叫法 SHALL 允许歧义，SHALL NOT 因叫法字面相同自动合并两个实体。
- 辅助标签溯源 SHALL 只记录「本次标签的辅助标签列表中，规范化文本与本次提及文本相等的那一行」的 `aux_id`；SHALL NOT 以相似度或自动 alias 判定同一实体；未关联的辅助标签行 SHALL 只作资料，SHALL NOT 被当作已有实体选项。
- 稳定短身份说明 SHALL 来自第一轮同次真实提取；SHALL NOT 从 `subject_key` 回填，SHALL NOT 因说明为空而编造。

#### Scenario: 实体身份与名称证据按层持久化

- **WHEN** 某提及被归一到实体 E
- **THEN** E SHALL 持有稳定实体 ID、规范名与稳定短身份说明，规范名 SHALL NOT 施加唯一约束
- **AND** 提及原文与已登记叫法 SHALL 留在名称与证据层，SHALL NOT 覆盖规范字段
- **AND** 辅助标签溯源 SHALL 只记录与本次提及文本规范化相等的那一行
- **AND** 第一轮未产出短说明时，E 的短说明 SHALL 保持为空，SHALL NOT 由 `subject_key` 回填

#### Scenario: 辅助标签被清理后实体身份仍然有效

- **WHEN** 某实体溯源到的辅助标签被 GC 置为 disabled 或被删除
- **THEN** 实体身份行 SHALL 仍然存在且实体 ID 不变
- **AND** 溯源行 SHALL 被移除或失效，SHALL NOT 级联删除实体身份行

#### Scenario: 同名不同实体可并存

- **WHEN** 两个真实不同的实体规范名相同
- **THEN** 系统 SHALL 允许二者各持一个实体 ID
- **AND** SHALL NOT 因规范名相同把它们合并，也 SHALL NOT 因规范名相同而拒绝第二个实体

### Requirement: 主体提及的完整解析链

系统 SHALL 完成「第一轮抽取 → 候选检索 → 二次模型选择 → 原子登记 / 新建」全链：候选由**实体名向量召回**、**实体名+短身份说明向量召回**、**名称与证据层中已登记叫法命中本次提及**三路合并，按 `entity_id` 去重，同一实体只留一个选项；两套向量 SHALL 各自在**同 model / 版本 / 维度**空间内比较，SHALL NOT 跨空间比较或 fallback 混用；名路径 SHALL 始终存在（短说明为空只降级为仅名，SHALL NOT 因此隐形）；每路取前 K、并列按实体 ID 升序，K 与合并条数 SHALL 为可评测调整的配置而非身份裁决阈值。

- 第二轮 SHALL 只输出候选集合内的合法实体 ID 或 `none`；判据 SHALL 是「是否指向同一实体」，仅相关 / 隶属 / 上下游 / 产品-厂商 / 机构-成员 SHALL NOT 判为同一实体。
- 输出不在候选集合内的 ID、不可解析响应、服务失败、超时 SHALL 归为**技术失败**：SHALL NOT 降级为 `none`，SHALL NOT 触发新建，SHALL NOT 留下半实体，SHALL NOT 进入任何业务等待队列；技术失败 SHALL 有限重试（同一提及最多 2 次尝试），耗尽后保持「未归一」并记录失败原因。
- `none` SHALL 直接新建实体（SHALL NOT 设待处理队列、人工种子表或人工确认闸门）；新建 SHALL 在**单个短事务**内完成（身份行 + 名称与证据行 + 辅助标签溯源行），任一步失败整体回滚；事务 SHALL NOT 跨模型调用。
- 调用 SHALL 经统一模型路由入口，携带非空 operation 与 capability=`topic_tagging`；SHALL NOT 新增 capability 枚举值，SHALL NOT 借用其它 capability 语义；每次尝试（成败）SHALL 写审计，可按 operation 重建一次提及的全部尝试。

#### Scenario: 新主体首次出现走完抽取到原子登记

- **WHEN** 影子态下某篇真实文章的一次主体提及在当前池内无候选
- **THEN** 系统 SHALL 用该次第一轮真实产出的主体原文与稳定短身份说明走完检索 → 二次选择 → 登记全过程
- **AND** 模型输出合法 `none` 时 SHALL 新建实体并把该提及归到它，SHALL NOT 生成待人工确认条目
- **AND** 身份行、名称与证据行、辅助标签溯源行 SHALL 在同一短事务内写成，SHALL NOT 留下半实体

#### Scenario: 稳定短身份说明为空回退仅名检索

- **WHEN** 第一轮未产出稳定短身份说明
- **THEN** 该提及 SHALL 走「仅实体名」检索路径，SHALL NOT 因缺说明而跳过名路径或报错
- **AND** 标签 SHALL 照常入库，SHALL NOT 因缺说明丢弃

#### Scenario: 合法 none 与技术失败是不同结论

- **WHEN** 模型返回合法 `none`
- **THEN** 系统 SHALL 视其为「当前池内无对应实体」并进入新建路径，SHALL NOT 记为技术失败
- **AND** WHEN 模型返回不在候选集合内的 ID、不可解析响应、服务失败或超时
- **THEN** 系统 SHALL 判为技术失败，SHALL NOT 降级为 `none`、SHALL NOT 新建、SHALL NOT 半写
- **AND** 重试耗尽后该提及 SHALL 保持「未归一」并记录失败原因，SHALL NOT 阻断标签入库、SHALL NOT 进入业务等待队列

#### Scenario: 后续跨叫法跨板块再次出现合为一个 ID 选项

- **WHEN** 某实体已登记，后一篇文章以另一种叫法提及同一主体
- **THEN** 三路候选 SHALL 合并为该实体**一个** ID 选项（同名但不同实体时 SHALL 保留各自选项，由模型按上下文选，SHALL NOT 硬复用）
- **AND** 模型选择该实体 ID 时，提及 SHALL 沿用同一统一 ID，SHALL NOT 因叫法不同而另建实体
- **AND** 提及原文与证据 SHALL 随该实体保留；该场景只约束「已被召回且被选中」时的归并，真实未召回导致的重复由重复身份组率与覆盖率如实评测，SHALL NOT 承诺 100% 归一

#### Scenario: 相关但不同的候选不被强合

- **WHEN** 候选中同时存在产品与公司、团队与公司、人物与政府等**相关但不是同一实体**的项
- **THEN** 系统 SHALL NOT 因向量相近、名称相似或类型相同而把它们合并
- **AND** 模型判 `none` 时 SHALL 新建另一实体，SHALL NOT 因「看起来相关」而选择任一候选

### Requirement: 并发、重放与跨进程闸门

- 同一文章同一标签的同一次主体提及被重复处理时，SHALL 复用先前决策（直接返回已存实体），SHALL NOT 重复建实体、SHALL NOT 重复消耗模型调用。
- 幂等键 SHALL 为（来源标签, 提及键）：**来源标签相同的跨文章复用视为同一来源**（同一 `topic_tags` 行被多篇文章复用同一叫法时，按幂等命中直接返回已存实体，不重新判定）；按**文章维度**重判属后续 change，届时需改 `uq_entity_mentions_source_key` 这条 partial 唯一索引（现为 `(source_tag_id, mention_key) WHERE source_tag_id IS NOT NULL`）。
- 跨来源的同一次提及 SHALL NOT 仅因叫法相同而跳过判别，SHALL 按当次上下文重新判定。
- 系统 SHALL 用一把跨进程生效的数据库级闸门（PostgreSQL **会话级咨询锁**，独占一条独立连接）串行化「**最新池检索 → 二次模型选择 → 原子登记 / 新建**」全过程；第一轮文章抽取 SHALL 在闸门之外；数据库**事务 SHALL 只包写库段**。
- 取得闸门后 SHALL **重新读取**已登记提及与候选池，SHALL NOT 提交闸外取得的旧 `none` 结论；候选集变化时 SHALL 按最新候选重新判定。
- 闸门等待、单次解析总时长、模型调用时长 SHALL 均有界；等待超时 SHALL 以明确技术失败结束，SHALL NOT 无限等待、SHALL NOT 无界乐观重试。
- 闸门 SHALL 在 `finally` 中释放并**归还占用连接**；进程取消、异常退出、审计写入失败 SHALL NOT 遗留占用。

#### Scenario: 同来源同提及重放幂等

- **WHEN** 同一文章同一标签的同一提及被处理两次
- **THEN** 两次 SHALL 返回同一实体 ID，实体层 SHALL NOT 多出一个实体
- **AND** 第二轮模型调用 SHALL NOT 再次发起，审计 SHALL 显示该提及只有一次判定尝试

#### Scenario: 跨来源并发在闸内重读最新候选再二判

- **WHEN** 两个进程同时处理同一提及键（来源标签不同）且两者在闸外检索时池内均无候选
- **THEN** 后取得闸门的一方 SHALL 在闸内重新读取候选池并按**最新候选**重新判定，SHALL NOT 提交闸外旧 `none`
- **AND** 其新判定选中已登记实体时两条提及 SHALL 指向同一实体 ID；其新判定为合法 `none` 时 SHALL 按 none 路径新建，SHALL NOT 被视为违约
- **AND** 闸门 SHALL NOT 被宣称为保证「实体全局不重」的唯一机制（重复由重复身份组率与覆盖率量化）

#### Scenario: 取消超时异常释放闸门与连接

- **WHEN** 持有闸门的进程被取消、发生异常、审计写入失败或等待超时
- **THEN** 闸门 SHALL 被释放且占用连接 SHALL 归还连接池，后续进程 SHALL 能正常取得
- **AND** 该次归一 SHALL 以明确技术失败结束，SHALL NOT 新建实体、SHALL NOT 留下半实体

### Requirement: 影子接入与既有判定零改动

- 系统 SHALL 提供 `off`（默认）/ `shadow` / `active` 三种模式；本期 SHALL 只实现 `off` 与 `shadow`；`active` SHALL 被显式拒绝并返回明确错误，SHALL NOT 静默按影子执行。（默认值变更：2026-11-10 决策，影子实验结束、实体层不接入事件线判定，默认回到 `off`——避免实体注册表只增不减成为黑洞；`shadow` 保留为实验态，需显式配置开启；`off` 仍保留为完全关闭开关。）
- `off` SHALL 保持第一轮提示与 schema 与调用数**逐字不变**，SHALL 零第二轮模型调用、SHALL 零实体层写入、SHALL NOT 写入 `topic_tags.metadata.entity_subject` 键。
- `shadow` SHALL 在第一轮提示中启用稳定短身份说明，并 SHALL 只写三处：① 实体层；② 正常 AI 审计（`ai_call_logs`）；③ `topic_tags.metadata.entity_subject` 这一**独立惰性键**（仅影子态第一轮产出、无读路径）。SHALL NOT 改写 `event_fingerprint`、标签的业务字段与归属，SHALL NOT 写 story / report / topic 任何字段与任何界面可见字段。
- **两个入口 SHALL 区分**：应用常规打标在 `shadow` 下 SHALL 持久化上述惰性 metadata 键；**评测 CLI SHALL NOT 写标签业务表**，其第一轮原始结果 SHALL 存放于评测产物与实体层证据。
- 事件线身份判定已随事件线退役整体下线（退役边界见 `event-story` 的退役 delta）：在线链路 SHALL NOT 再执行任何事件线身份判定、事件线归属或人工归属覆盖，实体层 SHALL NOT 参与任何在线身份判定。事件标签的指纹（`event_fingerprint` 与其中的 `subject_key`）仍由打标链产出，但 `subject_key` SHALL 只作指纹字段、SHALL NOT 被读作在线判定依据，SHALL NOT 参与任何热度或归属计算；组合标签 SHALL NOT 产生实体提及；存量实体身份 SHALL 保持为空（SHALL NOT 迁移或回填推断）。
- 「双侧都有实体 ID 时按 ID、单侧回退、两侧无 ID 走既有路径」等接线规则 SHALL NOT 作为现行规则写入本期规格（属后续 change）。

#### Scenario: 默认关闭态逐字不变

- **WHEN** 系统以默认配置运行打标
- **THEN** 第一轮提示与 schema 与调用数 SHALL 与上线前逐字一致，SHALL NOT 新增字段输出或模型请求
- **AND** SHALL NOT 发起第二轮模型调用、SHALL NOT 写入实体层、SHALL NOT 写入 `topic_tags.metadata.entity_subject` 键

#### Scenario: 影子态写入限定三处

- **WHEN** 系统以 `shadow` 模式处理一批事件标签
- **THEN** 实体层、正常 AI 审计、`topic_tags.metadata.entity_subject` 独立惰性键 SHALL 有新增
- **AND** `event_fingerprint`、标签的业务字段与归属、日报、话题与界面可见字段 SHALL 不变
- **AND** 评测 CLI 运行时 SHALL 不写标签业务表，第一轮原始结果 SHALL 只落评测产物与实体层证据

#### Scenario: 事件线身份判定不读实体 ID

- **WHEN** 某事件标签已被归一到某个全局实体 ID
- **THEN** 在线身份判定 SHALL 不存在（事件线已退役），实体层 SHALL NOT 产生任何在线判定输入或输出
- **AND** 存量 `subject_key` 指纹值 SHALL 与上线前一致，SHALL NOT 由 `subject_key` 或辅助标签行反推实体身份
- **AND** 人工归属 SHALL NOT 被归一结果覆盖，组合标签 SHALL NOT 产生实体提及

### Requirement: 有界评测入口与效果口径

- 评测 SHALL 在运行前**冻结**协议要素：样本清单、文章快照、分组规则、模型与版本、预算清单、评分规则；冻结后 SHALL NOT 因结果修改样本、题目或规则。
- 金标 SHALL 在**运行之前**由与模型输出隔离的标注者按文章原文盲标形成；运行器 SHALL NOT 读取金标；评测结束后由评分器把金标与运行结果连接。争议样本 SHALL 在运行前预列为「不计身份分」并 SHALL 报数量；标注者 SHALL NOT 改写查询描述或候选资料。
- 预算 SHALL 以**真实阶段**分列并机械可算：① 文章抽取调用次数 = 实际抽取文章数；② 二判调用次数 = 实际提及数 + 实际发生的有限重试数；③ embedding 提交文本数 = 两路查询文本 + 实体创建文本 + 别名登记文本 + 材料读取文本（若需 embedding），含重复发送与模型降级导致的重复提交；④ HTTP 次数 SHALL 按**发送前计数**口径，路径不支持批量时按单条实际计，结构化输出降级链与客户端多次 HTTP 尝试一并计入。
- 硬预算 SHALL 通过**仅在评测入口注入**的 provider / transport 包装实现（SHALL NOT 要求生产路由整体重构），在**发出请求前**拦截并保持正常审计；无法建立发送前计数能力时 SHALL 在发起任何请求前预先阻断。
- 评测 SHALL NOT 写入业务表（标签 / 事件线 / 日报）；SHALL 只写实体层与评测产物，并 SHALL 记录 run 标识以便清理。
- 评分 SHALL 以**固定总样本 N** 为分母口径，同时给出覆盖率、技术失败数、错误复用数与按固定总 N 的错误率；两套预测分组 SHALL 为：既有身份（`subject_key` 字符串，**空键单列「缺身份」**）与实体 ID（**无决策单列「未归一」**），二者 SHALL 在**同一「两套都有预测」的可比子集**上计算 pairwise 误合（金标不同而预测同组）、漏合（金标相同而预测不同组）与重复身份组率；缺决策 SHALL NOT 以单元素组冒充正确。
- 冷启动段 SHALL 从**全空**身份库按顺序处理，首次自然新建 SHALL NOT 计为重复；「金标实体当时已在池中」的判定 SHALL 只由评分器依顺序历史金标做事后标注，SHALL NOT 回喂归一器。重放段 SHALL 冻结**未经金标修正**的池；统一 ID 稳定性 SHALL 按实体 ID 相等计。
- 达标判定 SHALL 为可机械复算的对照：同一可比子集上误合率不高于既有身份、重复身份组率低于既有身份、覆盖率不低于既有身份、技术失败数为 0（否则 SHALL 阻断并要求修复后重跑）；未达标 SHALL NOT 启用生产，对应任务 SHALL 保持未完成。

#### Scenario: 评测协议与金标在运行前冻结并隔离

- **WHEN** 评测开始运行
- **THEN** 样本清单、文章快照、分组规则、模型版本、预算清单与评分规则 SHALL 已冻结，运行过程中 SHALL NOT 修改样本或金标
- **AND** 运行器 SHALL NOT 打开或读取金标文件
- **AND** 争议样本 SHALL 在运行前被列为「不计身份分」，报告 SHALL 给出其数量

#### Scenario: 预算超限在发请求前被拦截

- **WHEN** 评测已消耗到某一阶段的上限且仍需发起调用
- **THEN** 系统 SHALL 在发出请求前拒绝该次调用并记录拒绝计数，SHALL NOT 静默继续
- **AND** 预算清单 SHALL 分别列出文章抽取次数、二判次数与实际重试、embedding 实际提交文本数、发送前 HTTP 次数四项，各项 SHALL 由实际输入规模机械算出

#### Scenario: 评分按固定总 N 与覆盖率并单列缺身份与未归一

- **WHEN** 评测产出结果并进入评分
- **THEN** 报告 SHALL 给出固定总样本 N、覆盖率、技术失败数、错误复用数与按固定总 N 的错误率，条件准确率 SHALL NOT 替代覆盖率
- **AND** 既有身份侧的空 `subject_key` SHALL 单列为「缺身份」，实体 ID 侧的无决策 SHALL 单列为「未归一」，两者 SHALL NOT 以单元素组冒充正确
- **AND** 误合 / 漏合 / 重复身份组率 SHALL 在同一可比子集上计算；任一分母为 0 时该指标 SHALL 记「不适用（N=0）」，SHALL NOT 记 0% 或 100%

#### Scenario: 未达标不启用生产

- **WHEN** 任一验收条件未满足（误合升高、重复身份未减少、覆盖率低于基线、技术失败不为 0、数据不足或某段未测）
- **THEN** 生产归一 SHALL 保持关闭，评测 SHALL 判为未达标
- **AND** 对应任务 SHALL 保持未完成并留痕，SHALL NOT 以「先上线再说」方式跳过
