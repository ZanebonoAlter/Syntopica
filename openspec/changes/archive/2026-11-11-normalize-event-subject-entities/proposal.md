<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, topic-graph, ai-summary, daily-report -->
<!-- 域声明理由：复用辅助标签池候选与向量设施（semantic-board）、事件主体身份与 subject_key 判定边界（topic-graph）、新增 LLM/embedding 调用与审计（ai-summary）、事件线身份与 story_manual 人工归属兼容边界（daily-report） -->

## Why

事件主体的身份目前是**模型现编的一串字符串**，不是被登记过的实体——同一个主体在不同文章里会长出不同的 `subject_key`，而承载候选的辅助标签池（aux）本身只是一张「提及池」，不是实体登记表。四条实测把这件事钉死了（真库只读复核 + 研究小测，2026-11-09，见 `research.md`）：

1. **`subject_key` 在漂**。真库 910 条事件线 / 495 个 distinct key 里，`person:特朗普`（23 条）压过 `person:trump`；`company:deepseek` 与 `product:deepseek` 指同一主体却分属两型；`product:deepseekharness`（产品）被写成 `person:deepseek_harness_团队`。它由模型在同一次提取里现编，键的**类型前缀与规范名**都在漂。
2. **aux 池是「提及池」不是「实体登记表」**。active 22,268 行里，苹果散成 20+ 行（Apple / 苹果 / 苹果公司 / Apple Silicon / Apple Pay…），中国人民银行 3 行、中指研究院 12 行、国债期货 15 行；另有 190,396 行被 GC 置 disabled 且向量已清空——因为 aux 的身份判据是 0.95 余弦，而实测**同实体不同写法落在 0.79–0.94、跨实体共享 token 反而到 0.96**（DeepSeek↔DeepSeek Harness 0.9648），阈值方向是反的。
3. **相似度不能当身份判定器**。全池 22,268 行试算：真同实体对的非自命中名次 18/33、跨语言伙伴名次从未进过第 1（只有苹果↔Apple 例外），同语种兄弟标签总是压过它——必须有人（模型）判「是不是同一个」，而模型的判据不能是「相关」。
4. **模型选得对 ≠ 身份稳**。研究小测（Qwen，60 候选）里已有实体 15/15 语义正确，但换序题 Q09 从「唐纳德·特朗普」换成「特朗普」——两个 aux 行都算语义正确，**身份却变了**；none 题 Q20 把「影子公开市场委员会」误选成「Federal Reserve」（prompt 里那句「存在可能对应的候选就选最可能」是混杂因素）。

结论：**缺的不是又一个阈值，而是一个全局稳定、可登记、可核验的实体身份层**。本 change 建这一层，并给出**真实可评测**的服务与持久化设计；效果不达标就不启用生产。

## What Changes

- **新增全局实体身份层（独立表，不动 aux 业务语义）**
  - 身份层 `entity_registry`：稳定实体 ID（不随 aux GC 消失、不复用）、规范名（**不全局唯一**）、稳定短身份说明、两套检索表示（name 向量 / name+短说明向量）及其 model/版本/维度；**不含实体类型列**（模型猜类型会把漂移固化进身份）。
  - 名称与证据层 `entity_mentions`：同一实体的多种叫法（别名可歧义，只记证据与计数，不自动认同体）与逐次提及的原文证据。
  - aux 溯源层 `entity_aux_links`：只记录「本次标签的辅助标签列表中规范化文本与本次提及相等的那一行」的 `aux_id`（**溯源映射，不是同体证据**）；aux 被 GC/禁用时**只掉溯源行，不损毁实体身份**。
- **第一轮提取新增「稳定短身份说明」（仅影子模式）**：与现有 event 指纹**同一次调用**产出（零新增 LLM 调用），随 mention 原文一起落 `topic_tags.metadata.entity_subject`；**不进 `event_fingerprint`**，既有指纹结构与信任判定逐字不变；说明**不得包含当次事件/动作/对象**（否则说明随文章漂，检索就退化成事件检索）。**关闭态下第一轮提示与 schema 逐字不变**（不加字段输出、不发新增请求）。
- **候选 = 两路向量 ∪ 名称登记命中**：① 实体名向量；② 实体名+短说明向量；③ 名称与证据层中**已登记叫法命中本次提及**的实体；按 `entity_id` 合并去重，同一实体只留一个选项。同一叫法可对应多个实体 → 产生多个 ID 选项，由第二轮结合上下文选；**名字命中不等于身份判定**。
- **第二轮只做一道选择题**：模型只输出「某个合法实体 ID」或 `none`；prompt 明确「相关 ≠ 同一实体」。**非法 ID / 不可解析 / 服务失败 / 超时 = 技术失败**，不得降级为 `none`、不得新建、不得留半实体；`none` 即新建（无不确定队列、无人工种子表）。
- **检索只做召回**：embedding 只产候选，两套表示各自**同 model/版本/维度**空间内检索、跨空间不得混用；名入口**始终存在**（短说明为空只降级为仅名，不隐形）；既有 aux 存量向量**模型来源不可证**，不当作已验证同空间。
- **跨进程全局解析闸门**：用 PostgreSQL **会话级咨询锁（独占独立连接）**串行化「最新池检索 → 二次模型选择 → 原子登记/新建」全过程；**取得闸门后必须重读池与已登记提及**，不得提交闸外旧 `none`；事务仍只包写库段（短事务、原子）；等待/总时长/模型调用均有界，`finally` 释放并归还连接；**不引入无界乐观重试**。闸门**不消灭**「不同写法未召回/模型误选」的重复（那是效果指标）。
- **兼容边界（本期一律不动）**：既有 `subject_key` 三层判定、`subjectPreFilter` 的 subject_key 一票否决、`story_manual` 人工归属、组合标签不挂事件线、热度公式——**全部不变**；实体 ID 本期**不参与既有事件线身份判定**（影子模式；实体层自身仍照常做身份判别）；「实体 ID 接入事件线判定」的接线规则属后续 change，不写成本期现行规则。
- **影子模式 + 有界评测入口**：默认 `off`（2026-11-10 决策：影子实验结束，实体层不接入事件线判定——避免实体注册表只增不减成为黑洞；`shadow` 保留为实验态，需显式配置开启；`active` 本期显式拒绝）。`shadow` 写入范围限定三处（实体层 + 正常 AI 审计 + `topic_tags.metadata.entity_subject` 独立惰性键，仅影子态第一轮产出），不改 `event_fingerprint`、标签业务字段与归属、story/report/topic 与界面可见字段；`active` 本期**显式拒绝**（不静默忽略）。评测入口跑**真实第一轮产出 + 真实第二轮调用**，预算按真实阶段机械列示（文章抽取次数 / 二判次数与重试 / embedding 实际提交文本 / 发送前 HTTP 计数），有硬预算闸门。
- **评测协议冻结 + 金标隔离**：运行前冻结样本清单、文章快照、分组规则、模型版本、预算与评分规则；金标由与模型输出隔离的标注者**在运行前**按原文盲标，运行器不读金标，评测后由评分器 join；争议样本运行前预列为「不计身份分」并报数量。
- **不做**：不重算存量 tag/story、不批量归属、不预导入 22k aux、不建人工种子表、不做词表/后缀/长度阈值洗身份、不用相似度自动认同体、**不把模型猜测的类型写进身份层**、不把实体 ID 接入事件线判定。

## Capabilities

### New Capabilities

- `entity-subject-resolution`: 全局实体身份层与主体提及归一服务——按 5 个故事组织：**实体身份与证据存储**（稳定 ID、规范名不唯一、无类型列、aux 溯源口径）、**主体提及的完整解析链**（第一轮短说明仅影子态、三路候选按 entity_id 去重、二轮合法 ID/none 与技术失败分流、none 原子登记）、**并发重放与跨进程闸门**（同来源幂等、闸内重读最新池再二判、有界等待与资源释放）、**影子接入与既有判定零改动**（off 逐字不变、shadow 只写三处、事件线判定不读实体 ID、active 拒绝）、**有界评测入口与效果口径**（协议冻结与盲标隔离、发送前预算拦截、固定总 N 与覆盖率、误合/漏合/重复身份组率、零分母处理、未达标不启用生产）。

### Modified Capabilities

- `aggregate-tagging`: ADDED「事件主体稳定身份说明随同一次提取产出」——影子模式下第一轮同一次调用额外产出稳定短身份说明，零新增调用，指纹结构与信任判定不变，说明落在独立 metadata 键；**关闭态第一轮提示与 schema 逐字不变**。
- `event-story`: MODIFIED「主体归一化」——三层 subject_key 判定与事件线身份判定**本期逐字不变**；把原文「本期不建独立实体表」改为明确边界：全局实体身份层已存在，但**本期不参与事件线身份判定**（判定路径不读取实体 ID）、不改写任何既有事件线身份、不放宽 `subjectPreFilter` 的一票否决；「实体 ID 接入事件线判定」的接线规则属后续 change，本期不写为现行规则。

## Impact

- **新增代码（不与并发 change 争同一文件）**
  - `backend-go/internal/tagmanagement/models/entity_subject.go`（新）：3 张表的 GORM 模型 + `RegisterModels` 注册（AutoMigrate 建表，无历史回填）；**不含实体类型列**；向量列无维度限定，model/版本/维度写在行上。
  - `backend-go/internal/tagmanagement/repository/entity_repository.go`（新）：实体/提及/aux 溯源读写、会话级咨询锁（独占连接、finally 释放归还）、同来源同提及幂等查询、已登记叫法反查实体。
  - `backend-go/internal/tagmanagement/service/entity/`（新包）：两路检索（embedding + 余弦 + 同空间守卫）、名称登记命中入口与三路候选合并、第二轮 prompt 与调用、全局解析闸门编排（重读池 → 判定 → 原子登记/新建）、幂等与失败语义。
  - `backend-go/internal/tagmanagement/service/core/extractor_section.go`、`extractor_enhanced.go`（改）：按模式分支的 event 提取 schema/提示（影子态新增 `subject_desc`）；`tagger.go`（改）：新增 metadata 键的读-改-写（**不得盲写整个 metadata**，既有 `event_fingerprint` 键不受影响）。
  - `backend-go/cmd/entity-resolve-eval/`（新）：有界评测 CLI + **发送前计数**的评测专用 transport 包装 + 盲标工作表导出 + 机械评分器。
  - `backend-go/internal/platform/database/postgres_migrations.go`（改，**仅一行注册**）：新表需要的索引/约束迁移放在独立文件 `entity_subject_migration.go`，避免与并发 change 争同一大文件。
- **数据影响**：新增 3 张空表（无回填、无存量改写）；`topic_tags.metadata` 增一个 `entity_subject` 键（**仅影子态写入**，当前无人消费）。既有 `topic_tags.metadata.event_fingerprint`、`board_stories`、`daily_report_*` 一字不动。
- **并发协调**：`enable-story-cross-day-continuation`（同改 `event-story` 主 spec）与 `unify-vector-column-dimension`（同改向量列约定）均为活跃 change。本 change **不触碰** `board_stories` 结构、`story_assign.go`、`story_fingerprint.go`、向量列 typmod；`event-story` 的 delta 只改「主体归一化」一条 Requirement（与前者的「事件指纹结构化产出 / 事件线数据模型 / 事件归属判定顺序」不重叠）；向量列按**无维度限定 `vector` + 行内记 model/版本/维度**声明，与后者的运行时维度契约一致（该能力尚未归档，本 change 自带维度口径，不依赖它）。
- **部署后用户可见变化**：**无界面可见变化**。默认 `off`：第一轮提示与 schema 与调用数逐字不变、零二轮调用、零写入，回到上线前行为；界面、报表、日报、雷达、事件线全部不变。要跑影子实验把 `entity_subject.mode` 改成 `shadow`：第一轮提示多了 `subject_desc` 字段、开始写实体层三表与 `metadata.entity_subject` 惰性键（无读路径）、每个 event 标签同步多 1–2 次模型调用（有界）。
- **需要用户手动执行的操作**：若要拿效果证据，需在部署后**另行授权**一次有界真实模型评测（`cmd/entity-resolve-eval`，需先出预算清单）；授权前评测不启动，本 change 不因此宣称效果已验证。**当前没有新的模型额度授权**，本阶段只产出预算清单生成与授权流程。
- **旧数据如何降级**：存量事件线与标签**没有实体 ID**（entity_id 恒为空），身份判定继续走既有 `subject_key` 三层路径；**不迁移、不猜测、不回填**。影子模式产出的实体层与旧数据互不影响，可整层删除而不影响任何既有功能。
- **不把本 change 的效果等同跨天 story 已修好**：跨天延续、object/date 归因属并发 change（Change C），本 change 不修 object/date、不改热度、不改事件线归属。**也不宣称生产漂移已修好**——生产启用本期不实现（`active` 显式拒绝）。

## 部署后影响与需要的操作

- (a) 部署/合并后用户可见行为：**界面不变**。日报、雷达、事件线、标签面板、话题编排全部照旧；默认 `off` 下实体层三表与 `metadata.entity_subject` **不再新增**、打标管线不再因归一变慢；想跑影子实验把 `entity_subject.mode` 改成 `shadow`（新表开始积累实体行，吞吐下降）。
- (b) 需要用户手动执行的操作：**无强制项**。可选：部署后跑一次有界评测（需先出预算清单并**单独授权**真实模型调用）；本 change 的生产开关**故意不实现**（`active` 被显式拒绝），启用与否由评测结论决定，另开 change。
- (c) 旧数据如何降级：存量身份**零改动**；实体层与旧数据完全解耦，删除新表即可回滚到今天的行为。
- (d) 已知代价：影子/评测路径的归一过程被**全局闸门串行化**，吞吐与等待时长是本期明确代价（评测会测出来），不承诺高并发。
