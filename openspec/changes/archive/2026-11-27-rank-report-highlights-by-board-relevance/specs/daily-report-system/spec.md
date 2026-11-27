## MODIFIED Requirements

### Requirement: 今日重点聚簇候选
系统 SHALL 在日报落库尾部为每版块生成「今日重点」候选：当天线索命中的 event 标签按 related-event 向量相似度聚簇（向量聚簇 → 语义向量兜底 → 主体近似三级降级）。LLM SHALL 只从候选白名单内选至多 3 条写 title/reason，SHALL NOT 编造事实或数字——簇文章数、簇身份等数值快照一律由服务端计算补齐（`cluster_id` / `article_count` / `cluster_mode` / `data_as_of` / `research_at`）。

**贴题档**：每个聚簇 SHALL 由服务端按「该簇完整成员标签所落的本期正文板块」定一个贴题档（0–5，越小越贴版块）；一个簇的成员标签分落多个板块时取最好的板块，同档取话题非候选者。档位口径：板块泳道档 `lane_tier = l1_direct` → 0；`lane_tier` 以 `watch_` 开头（用户手动关注命中）→ 1；`l2_llm` 且话题当期状态非候选 → 2；`l2_llm` 且话题当期为候选（`topic_status_at_report = candidate`）→ 3；`l3_new` → 4；本期正文无含该簇标签的板块、或板块无泳道档 → 5。取档 SHALL 以候选簇的完整成员标签对照板块标签集合，SHALL NOT 依赖模型回填的 `tag_ids` 子集。

**候选池（先筛，版块关联度优先）**：候选池 SHALL 在既有候选序（簇内文章数降序 → 簇内标签数降序 → 代表标签 id 升序）之上按贴题档组池——

- 贴题簇（贴题档 0 或 1）有 3 个及以上时，候选池 SHALL 只含贴题簇，按既有候选序取至多 8 个；其余簇 SHALL NOT 进入候选、SHALL NOT 被送去 LLM；
- 贴题簇只有 1–2 个时，候选池 SHALL 含全部贴题簇，并从其余聚簇中按贴题档升序、同档按既有候选序补位，直至池内共 3 个；补位的簇 SHALL NOT 挤掉贴题簇；
- 贴题簇一个都没有时，候选池 SHALL 维持既有口径（既有候选序前 8 个）。

版块 section 数与泳道热度 SHALL NOT 参与候选池与展示序的任何排序。

**展示序（挑完再排）**：LLM 选出的重点 SHALL 由服务端按贴题档稳定排序后再落库与返回，同档保持候选序。排序 SHALL NOT 依赖模型返回的顺序，SHALL NOT 改变重点的成员、文案与快照；模型选中的重点 SHALL NOT 因贴题档被移除或限制条数。读取本期正文板块信号失败时候选池与展示序 SHALL 一律退回既有口径（既有候选序前 8、候选序输出），记一条含 report id 与失败原因的 Warn 级日志，SHALL NOT 阻断重点落库。日报 `title` / `summary` 仍取排序后的首条重点。候选池规则与排序只适用于聚簇模式；零候选的标签回退模式不变。

**聚簇输入 SHALL NOT 因线索 `scope=composite` 被排除（本 change 起）**：composite 线索（同事件多表述标签聚合）的 event 标签 SHALL 与 single 线索同权参与聚簇。跨事件混编标签 SHALL 由聚簇相似度门槛隔离（自成小簇、不并入单事件簇）；composite「SHALL NOT 计入事件数、SHALL NOT 建立事件线」的既有语义 SHALL 不变。当天线索均无 event 标签或聚簇产出零候选时 SHALL 走标签回退模式（cluster_id 为空、快照字段缺省、不伪造篇数）。

#### Scenario: 单事件多标签大簇进候选
- **WHEN** 某线索聚合 17 个同事件不同表述的 event 标签、关联 20 篇文章（scope=composite），当天另有多个 1 篇文章的单标签线索，且各簇贴题档相同
- **THEN** 聚簇 SHALL 将这 17 个标签聚为一簇、簇内去重文章数为 20 篇级，该簇 SHALL 排候选池首位

#### Scenario: 混编标签自成小簇不污染
- **WHEN** 某混编标签横跨 5 个事件共 12 篇文章，与任一单事件标签的向量距离均超聚簇阈值
- **THEN** 该标签 SHALL 自成一簇、SHALL NOT 并入任何单事件簇，其簇文章数按自身关联文章去重计算

#### Scenario: 全 composite 日报不再走标签回退
- **WHEN** 当天全部线索 scope 均为 composite 且均含 event 标签
- **THEN** 聚簇 SHALL 正常产出候选，SHALL NOT 仅因线索为 composite 而进入标签回退模式

#### Scenario: 贴题簇充足时不贴题的大簇不进候选
- **WHEN** 当天有 3 个以上贴题簇，另有一簇文章数最多、但其成员标签所落板块为中档且话题为候选
- **THEN** LLM SHALL 只收到贴题簇的事实与篇数，文章最多的那一簇 SHALL NOT 进入候选池、SHALL NOT 出现在重点中

#### Scenario: 贴题簇不足时按贴题档补位到三个
- **WHEN** 当天只有 2 个贴题簇，其余簇分属中档（话题在跟）、中档（候选话题）与远档
- **THEN** 候选池 SHALL 为这 2 个贴题簇加 1 个补位簇，补位簇 SHALL 取贴题档最低者（同档按既有候选序）；贴题簇 SHALL 全部保留在池内

#### Scenario: 贴题簇为空时维持原候选口径
- **WHEN** 当天没有任何贴题档为 0 或 1 的聚簇（例如版块尚无已成立话题与关注）
- **THEN** 候选池 SHALL 为既有候选序前 8 个，LLM SHALL 照常从中挑选

#### Scenario: 候选按簇内文章数截断
- **WHEN** 贴题簇有 10 个、既有候选序明确，第 9 个的簇内标签数最多
- **THEN** 候选池 SHALL 按既有候选序取前 8 个贴题簇，第 9 个 SHALL NOT 因标签数进入候选

#### Scenario: 模型只从白名单挑且不输出数字
- **WHEN** 模型返回候选池外的簇身份或自填 article_count 数值
- **THEN** SHALL 按既有非法语义同参重试，数值快照 SHALL 仅由服务端计算落库

#### Scenario: 贴题档低的大簇让位
- **WHEN** 模型选出的 3 条重点里，候选序首位的一簇贴题档为 3，另两簇贴题档为 0
- **THEN** 贴题档为 0 的两条 SHALL 排在前两位并保持候选序，贴题档为 3 的那条 SHALL 排第三位且仍在重点中；日报 title/summary SHALL 取首条

#### Scenario: 跨板块取最好一档
- **WHEN** 某簇的成员标签分落两个正文板块，一个为近档、一个为远档
- **THEN** 该簇 SHALL 按近档定贴题档并参与候选池与展示序

#### Scenario: 手动关注命中排在近档之后
- **WHEN** 三条重点的贴题档分别为 0、1（用户手动关注命中）、2
- **THEN** 展示序 SHALL 为 贴题档 0 → 1 → 2

#### Scenario: 候选话题同档靠后且缺失不惩罚
- **WHEN** 两条重点同属中档，一条所属话题当期为候选，另一条为在跟或话题状态缺失
- **THEN** 在跟或状态缺失的那条 SHALL 排前；两条均为在跟或均缺失时 SHALL 保持候选序

#### Scenario: 正文无对应板块的重点排最后
- **WHEN** 某重点的候选簇成员标签在本期任何正文板块的标签集合中都找不到，或所属板块无泳道档
- **THEN** 该重点的贴题档 SHALL 为 5，排在所有其他重点之后，且仍在重点中

#### Scenario: 读取板块信号失败退回原口径
- **WHEN** 聚簇后读取本期正文板块贴题信号的查询出错
- **THEN** 候选池 SHALL 为既有候选序前 8、重点 SHALL 按候选序落库、记一条含 report id 与失败原因的 Warn 日志，日报其余部分 SHALL NOT 受影响

#### Scenario: 零候选走标签回退
- **WHEN** 当天线索均无 event 标签或聚簇产出为空
- **THEN** SHALL 走标签回退模式，cluster_id 为空、快照字段缺省不伪造篇数，正文/归属/事件数不受影响

#### Scenario: 旧重点快照读取兼容
- **WHEN** 读取本 change 之前落库的重点（旧聚簇口径或标签回退产物）
- **THEN** 快照字段与顺序 SHALL 原样返回、不回填不重算不重排；该日报被重建时 SHALL 按新口径重算重点并按贴题档组池与排序

### Requirement: 当天同事件聚簇先筛重点候选

系统 SHALL 在本 report 当天同事件聚簇中去重筛选「今日重点」候选。候选 SHALL 为该版块当天日报命中的 event 标签聚成的簇（口径与降级链见 `related-events`「日报同事件聚簇」），每个候选 SHALL 附代表标签、label/description、簇内文章数（按内容身份去重）与聚簇口径标记。**既有候选序** SHALL 按簇内文章数降序 → 簇内 event 标签数降序 → 聚簇代表标签 id 升序排列，不按四舍五入后的篇数排序、不用版块 section 数或泳道热度替代。**候选池** SHALL 在既有候选序之上按版块关联度先筛：贴题簇优先、贴题簇不足 3 个时按贴题档补位到 3 个、贴题簇为空时维持既有候选序前 8 个（贴题档口径与细则见 Requirement「今日重点聚簇候选」）。

LLM SHALL 只从候选池内选至多 3 条不同聚簇写 title/reason，SHALL NOT 编造事实、数字、情绪或因果。模型返回的 `cluster_id` SHALL 属当次候选池白名单，`tag_ids` SHALL 属该聚簇的本期标签白名单；过滤后空 tag、空白标题理由、集外 id 或类型错误 SHALL 按既有语义非法响应同参重试一次；合法 `[]` 不重试，最终失败仍 `[]`。重复聚簇 SHALL 保留首条。

文章数、聚簇口径、数据截至日与研究时点 SHALL 由服务端补齐快照，SHALL NOT 由模型输出。候选不足允许 1 条，零候选 SHALL 在调用前选择标签回退模式（`cluster_id` 为空、缺省快照字段），SHALL NOT 伪造身份或编造聚簇。旧报告 SHALL NOT 回填补造聚簇身份或篇数快照。

#### Scenario: 候选按簇内文章数截断
- **WHEN** 本期有 10 个候选聚簇，其中 4 个为贴题簇（含文章数排第 9 的那一个），其余 6 个文章更多但属中档或远档
- **THEN** LLM SHALL 只收到这 4 个贴题簇的事实与篇数；文章更多的 6 个簇 SHALL NOT 进入候选池；section 数与泳道热度 SHALL NOT 参与任何排序

#### Scenario: 同分与精度不反转
- **WHEN** 两个候选簇贴题档相同、簇内文章数相同，或四舍五入后相同但原值不同
- **THEN** SHALL 依次按簇内 event 标签数降序、代表标签 id 升序破同分，未舍入篇数更高者仍排前

#### Scenario: 只可从候选事实写文案
- **WHEN** 模型返回集外 `cluster_id` 或仅含别的聚簇 tag_ids 的重点
- **THEN** SHALL 判非法并按既有上限重试，SHALL NOT 将该条作为合法重点落库

#### Scenario: 无聚簇走标签回退
- **WHEN** 当天全部为综合稿、无候选 event 标签，或全部候选无正文落点
- **THEN** SHALL 选择现有标签聚类重点路径，`cluster_id` 为空，正文与统计字段 SHALL NOT 为了凑候选被修改

#### Scenario: 少量候选与合法空返回
- **WHEN** 只有一条候选或候选事实不足导致模型返回 `[]`
- **THEN** SHALL 分别允许一条重点或合法空数组，SHALL NOT 为凑 2 条编造内容或重试合法 `[]`

#### Scenario: 重点快照与旧数据兼容
- **WHEN** 读取新聚簇重点、旧 tag_ids-only 重点或 JSON null
- **THEN** 新重点 SHALL 返回生成时来源、篇数、聚簇口径与时点快照，旧重点 SHALL 不伪造快照，JSON null SHALL 归一为 `[]`，首条重点仍为报告 title/summary 来源，无重点仍用既有兜底
