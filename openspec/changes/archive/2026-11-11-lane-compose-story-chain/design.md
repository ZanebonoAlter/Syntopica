# Design — 泳道编排的事件线链（跨天同事件选材维度）

## Context

见 proposal.md「Why」——用户要的是「新建泳道时按事件线选材」。本设计只回答**怎么把事件线接进编排态**，不回答事件线本身怎么判定（那是上游 `add-event-story-heat` 的既有能力，本 change 不改其任何判定规则）。

实施现状（实读代码，非推测）：

| 事实 | 落点 |
|---|---|
| 候选链 = 纯函数：按 `persistent_topic_id`（status=candidate）分组 + 原 relations（identity/similarity）无向连通 + ≥2 分区且跨 ≥3 UTC 天 | `backend-go/internal/topicgraph/repository/compose_chain.go:BuildComposeCandidateChains` |
| similarity 边的 distance = `daily_report_sections.embedding` 的余弦距离；该 embedding 是分区**内容向量**（所聚标签 label/description/代表文章摘录 → 线索标题 → `cluster_label` 三级回退），不是事件身份 | `backend-go/internal/topicgraph/service/daily_report_embed_text.go:buildSectionEmbedText` |
| 编排态取数：`GetComposeCandidates` 返回窗口内全部带 embedding 的分区 + 窗口内原 relations + 服务端算好的 `candidate_chains` + `match_threshold` + `window` | `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:GetComposeCandidates` |
| section→threads→`story_id` 的批量反查**已有现成形态**（`section_id IN (...)` 批量 + 索引 `idx_daily_report_threads_section_id` + `daily_report_threads_story_idx`） | `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreadStats` |
| `story_id` → 存活事件线的一跳解析已有实现（`merged_into` 一跳；两层链报错） | `backend-go/internal/topicgraph/repository/story_repository.go:ResolveBoardStoryID` |
| 关系表每次 `SaveReport` 全量删建（`RebuildBoardRelations` 先删该板全部 relations，再写 identity + similarity） | `backend-go/internal/topicgraph/repository/daily_report_matching.go:RebuildBoardRelations` |
| section status（emerging/split/merge/continuing）是**按 relations 边集动态推导**的 | `openspec/specs/section-relations/spec.md` |
| 事件线身份与话题归属**正交**；同一话题一天可拆 1–6 桶 | `docs/reference/flow/topic-graph.md` 红线 14 |
| 事件的 `story_id` 是在 `SaveReport` **之后**由有界同步归属写入；未回填板全为 NULL | `docs/reference/flow/daily-report.md` §事件线归属与跨期去重 |

一句话现状：**数据链路已齐备，缺的是把 `story_id` 接到编排态的一条新维度上**。

## Goals / Non-Goals

**Goals:**

- 让用户在编排态左侧能按「同一事件」看到跨天材料，并一次勾齐。
- 事件线链与话题链**并列**呈现，互不替代、互不干扰（正交分组键）。
- 零 DDL、零迁移、零关系表写入；不污染既有 section status 推导与 lifecycle API。
- 事件线链**只是选材维度**：不得产生/合并/激活任何话题，不放宽候选生命周期与手动建话题红线。
- 全链路无 N+1；新增查询成本相对既有取数开销可忽略。

**Non-Goals:**

- 不改事件线的**归属判定**（subject_key 预筛 / 指纹 / 合并闸门一律不动）。
- 不改 embedding 的相似推荐口径、阈值与排序（见 D6）。
- 不做事件线详情页、不做时间轴视图、不做事件线与话题的合并/对齐功能。
- 不把事件线身份引入 `daily_report_section_relations`、不改 section status 推导。
- 不回填 `story_id`（回填是既有 `cmd/backfill-stories` 运维能力，本 change 不改它）。
- 不做 lane 动态/矩阵/雷达的任何口径改动。

## Decisions

### D1. 两条链的语义边界：话题链按话题分组，事件线链按存活事件线分组，两者正交

- **结论**：话题链 = 既有口径（candidate 话题分组 + 原 identity/similarity 边连通 + ≥2 分区跨 ≥3 天）。事件线链 = 新维度，按**存活事件线 id** 分组：窗口内任一分区若其线索归属于同一存活事件线，即归入该链，门槛同话题链（≥2 分区、跨 ≥3 UTC 天）。两条链**分组键正交**：一条事件线链可横跨多个 candidate 话题与未归属分区；一条话题链里可含多条事件线。事件线链是**辅助选材维度**，**不是候选话题资格判定**——它不产生、不合并、不激活任何话题，也不改写分区的话题归属（`persistent_topic_id`）。
- **落点**：`backend-go/internal/topicgraph/repository/compose_chain.go:BuildComposeCandidateChains`；Scenario「同一事件跨天直连成链」「事件线链不产生或激活话题」
- **取舍**：被否方案 = 「用事件线替换话题链作为唯一候选维度」——会直接摧毁 candidate→active 的渐进确认机制（红线 2/3），且把「同一事件」当成「同一话题」，违反话题与事件身份正交（红线 14）。被否方案 = 「话题链与事件线链互相合并/互为子集」——正交是事实，两者的交叉形态不可预设，强行合并必产生错误归并。

### D2. 事件线链的成员域与「一个分区多个事件线」的规则

- **结论**：成员 = 窗口内带存活事件线身份、且其归属话题**不是 active** 的分区（candidate 话题分区 / 未归属分区 / 已归档来源分区均可入链）。active 泳道分区**不入链**（在画布上仍是既有只读参照）。一个分区可承载多个事件，故**同一分区可同时属于多条事件线链**；成链时不做分区副本拆分，只做成员集合并集。链元信息按链聚合，不按分区去重后回写。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:GetComposeCandidates`；Scenario「一个分区可同属多条事件线链」
- **取舍**：被否方案 = 「一条分区只归一条链（按该分区占比最大的事件线）」——会静默丢掉同区多事件，且引入无依据的仲裁口径；被否方案 = 「active 泳道分区也入链」——与编排态「active 只读、跨正式泳道移入须另行审批」的既有边界冲突，整组勾齐会绕过审批。

### D3. 事件线边的构造：section→threads→`story_id` 批量反查 + `merged_into` 一跳存活解析

- **结论**：构造路径为三步：① 用**一次批量查询**取窗口内分区的线索身份（形态复用 `listLaneSectionThreadStats` 的 `section_id IN (...)` 批量 + 既有索引，但只投影 `section_id / story_id / scope` 三列，不拉 lane 动态才需要的 `related_article_ids` JSON 证据列）；② 用**一次批量查询**取该板全部 `board_stories` 的 `(id, title, merged_into)`，在内存做**一跳**存活解析（语义等同 `ResolveBoardStoryID`：未合并返回自身，已合并返回存活 id，**两层链/环/坏引用一律跳过该线索并记 warn**）；③ 按存活 id 分组。`scope=composite` 的线索与 `story_id IS NULL` 的线索**天然不入链**（composite 按红线永不建身份）。
- **落点**：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreadStats`；`backend-go/internal/topicgraph/repository/story_repository.go:ResolveBoardStoryID`；Scenario「链键按存活事件线归一」
- **取舍**：被否方案 = 「逐条调用 `ResolveBoardStoryID`」——每条线索 1–2 次查询，窗口内数千条线索即数千次往返（N+1），不可接受；改为**批量预载 + 内存一跳解析**，语义与错误处理与 `ResolveBoardStoryID` 一致（两层链同样视为异常），但查询数恒为 2。被否方案 = 「只按 `story_id` 原始 id 分组，不解析存活 id」——合并后 A→B 会裂成两条链，用户看到的「同一事件」被拆开。

### D4. 事件线链的成链门槛：与话题链同口径（≥2 分区、跨 ≥3 UTC 天）

- **结论**：门槛**写死**为与话题链完全一致：成员分区数 ≥2 且首末成员报告日期的 UTC 日差 ≥3（中间允许断档），跨度用**窗口内成员的极差**计算（复用 `utcDayDiff`）。**不**使用 `board_stories.first_seen_date/last_seen_date` 判定资格，也**不**把窗口外分区拉进链——那两列只用于展示事件线自身的活跃区间。
- **落点**：`backend-go/internal/topicgraph/repository/compose_chain.go`；Scenario「成员数或跨度不足不成链」
- **取舍**：被否方案 = 「事件线链另设门槛（如 ≥2 天或 ≥2 分区即可）」——两套门槛会让用户面对「同样是跨天材料，一条链能勾一条不能勾」的困惑，且事件线的跨天跨度由 `first_seen_date/last_seen_date` 给出的是**事件线存在期**（可能远长于本窗口），用它判资格会把窗口内根本凑不齐的材料算成链。理由：门槛服务于「一次勾齐跨天材料」，窗口内跨度才是用户当下能勾到的量。

### D5. 事件线边只在查询期内存构造，不落 `daily_report_section_relations`

- **结论**：不新增 `relation_type='story'` 边、不写关系表、不加表列。事件线链在 `GetComposeCandidates` 内以内存结构构造并单独返回（`story_chains` 字段），**原 `relations` 数组保持逐字不变**（仍只含 identity/similarity）。画布若要画事件线连线，由前端按链内相邻成员顺序连线并用独立线型呈现，不把第三种边塞进既有 `relations`。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_matching.go:RebuildBoardRelations`；Scenario「事件线链不写入关系表」
- **取舍**：被否方案 = 「落库 `relation_type='story'`」——三重代价：① **重建时序**：`RebuildBoardRelations` 每次 `SaveReport` 先删该板全部 relations 再重写，而 `thread.story_id` 是 `SaveReport` **之后**由有界同步归属写入，落库边要么缺失、要么必须在归属后再重建一次关系表（额外一次全板重建）；② **污染既有推导**：section status（emerging/split/merge/continuing）与 lifecycle API 都是按 relations 边集动态推导的，混入第三种边会改掉既有语义；③ **迁移与回填成本**：需 DDL + 存量边回填 + 重建时序治理。内存方案零迁移、存量板即时可用，代价只是每次查询重算——而重算成本远低于既有取数（见 D8）。被否方案 = 「把 story 边混进既有 `relations` 数组」——会连带改变前端 `relatedNodeIds` 的 1-hop 相关节点集合，属静默扩大改动面。

### D6. 不改 embedding 相似推荐的优先级：事件线只提供「整组勾选」，不参与召回

- **结论**：相似 section 推荐的口径**完全不变**——编排态侧 `laneComposeLogic.centroidRecommendations`（经 `useLaneCompose` 消费）与线程浏览器侧 `useInlineCompose.recommendations` 均仍按已选成员聚合向量质心做 cosine 距离、以既有 `matchThreshold` 筛选、按距离/ID 稳定排序、分组与文案一字不改。事件线身份**不进入**该召回集、不改阈值、不改排序、不参与质心计算、不被当作置信度或相近程度的替代表达。事件线链只提供**整组勾选**入口（用户要的是「一次勾齐」，不是「再猜几个像的」）。
- **落点**：Scenario「事件线不进入相似推荐召回」；`front/app/features/tags/composables/laneComposeLogic.ts:centroidRecommendations`（编排态实际使用的质心召回）；`front/app/features/tags/composables/useInlineCompose.ts`（线程浏览器侧的同口径实现）
- **取舍**：被否方案 = 「同事件线优先于 embedding 质心匹配」——会踩两条红线：其一，候选推荐**不是置信度**（既有 spec 明写），把「同一事件」升格成排序主键等于伪造置信度；其二，向量相似度只用于召回候选、MUST NOT 单独作为合并判定依据（红线 14），反方向同样成立——事件线身份也不该被升格成「更相似」的证明。用户的抱怨是「embedding 关联线索不靠谱」，正确的解法是**另给一条更可靠的线索**（事件线整组），而不是**篡改旧线索的排序**。若实施后用户仍反馈相似推荐噪声大，应单独立项重新标定召回口径，不在本 change 内悄悄改。

### D7. 回填依赖：未回填板 = 该板无事件线链，明确降级，不冒充无候选

- **结论**：响应体新增两个只读标记：`stories_backfilled`（板内窗口内是否存在任何非 NULL `story_id`；全 NULL → false）与 `stories_partial`（存在已回填线索但仍有未回填线索）。未回填时事件线链为空，界面**禁用**事件线维度并显示「事件身份未回填，本板暂无事件线链」；部分回填时在列表顶部显示「部分事件未回填，事件线链不完整」。**已回填但确无跨天事件**时，空态写「本板暂无可串联的事件线」，**不写**「事件身份未回填」——两者不可混为一谈。
- **落点**：Scenario「未回填板诚实降级」「部分回填标记不完整」
- **取舍**：被否方案 = 「未回填时静默显示空事件线维度」——用户无法区分「这板没事件线」与「这板没回填」，正是红线 30（事件身份未回填必须明示）禁止的「按 0/空态冒充无事件」。被否方案 = 「在编排态里顺手触发回填」——回填是重操作（LLM 判定 + 写库），编排态是只读取数面，绝不夹带写操作。

### D8. 性能：两次批量查询 + 内存分组，严格避免 N+1

- **结论**：新增查询**恒为 2 次**，与窗口内分区数、线索数无关：① 线索身份批量查询（`section_id IN (...)`，命中 `idx_daily_report_threads_section_id`）；② 该板 `board_stories` 批量查询（命中 `board_stories_subject_idx` 的 board_id 前缀）。`IN` 列表按 **500** 一批切分（远低于 PG 65535 参数上限）。分组与存活解析全在内存（map 聚合），复杂度 O(分区数 + 线索数 + 该板事件线数)。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:GetComposeCandidates`；`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreadStats`
- **取舍**：被否方案 = 「逐分区查线索 / 逐线索解析存活 id」——窗口上限 90 天、每日若干分区，量级在数百至数千，任何逐条往返都会把编排态取数从毫秒级拖到秒级。被否方案 = 「把 section→threads→story 结果做跨请求缓存」——事件线身份随每日归属与合并实时变化，缓存失效规则难以定准，且编排态取数本就在变化的数据上，收益不抵一致性风险。

### D9. 关键数据量级与成本对照（供实施期核对）

- **结论**：窗口上限 90 天。按每版块每日 3–10 个带 embedding 分区估，窗口内分区约 270–900、线索约 1k–4.5k。既有 `GetComposeCandidates` 已需为每个分区**解析 4096 维 pgvector 文本向量**（约 270–900 × 4096 ≈ 110 万–370 万个浮点数），其开销数量级**远高于**新增的两次批量查询与内存分组。因此事件线链的边际成本可忽略（经验估计 <5%），无需引入缓存或预计算表。
- **落点**：无代码落点：design.md 成本估算章节，因为该结论是 D8/D5 的取舍依据，无对应实现落点
- **取舍**：无备选：成本为既有实现的事实推算，不存在「选 A 还是选 B」的设计分叉；实施期若实测超出预期，再按 D8 的批量策略调优，不改本设计的语义决策。

## 红线兼容

| 红线 | 保持方式 |
|---|---|
| 红线 2：candidate 连续命中达阈值仅获人工激活资格，仍需手动 PATCH active | 事件线链只读事件身份，SHALL NOT 写 `board_persistent_topics` 任何字段；D1/D6/spec「事件线链不产生或激活话题」 |
| 红线 3：当天 section 无法归属已有话题时 auto_new 建 candidate 并种命中计数 1 | 本 change 不改归属路径；事件线链不写命中计数、不触发 auto_new |
| 红线 7：用户手动建话题直接落 active，跳过 candidate 门禁，`topic_match_confidence='manual'` | 保存路径一字不改；事件线链只是多一个选材入口，不改保存语义（spec MODIFIED「保存与只读边界」） |
| 红线 14：话题不是事件身份；事件线身份与话题归属正交；向量相似度只用于召回候选，MUST NOT 单独作为合并判定依据 | 两条链分组键正交（D1）；事件线链不入质心召回（D6）；事件线身份只读消费（D3） |
| 红线 30（semantic-board）：事件身份未回填必须明示，MUST NOT 按 0/空态冒充无事件 | D7 的未回填/部分回填/确无事件三态文案 |

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|---|---|---|
| 用户把「事件线链」误读成「这就是泳道」 | 误解导致建错泳道 | 事件线维度与话题维度**并列**且各自带明确文字标签；事件线组元信息写「同一事件 · 跨天 N 天」而非「泳道」；保存仍由用户命名 |
| 已回填板里事件线覆盖率低（多数线索 `story_id` 仍为 NULL），事件线维度看起来「空的」 | 感知为功能不可用 | 未回填与「确无跨天事件」用**不同文案**（D7）；效果核对阶段量化各板覆盖率并如实报告（test-cases §4） |
| 事件线链横跨多个话题，用户整组勾齐后得到一组「跨话题」材料 | 泳道语义发散 | 事件线链是**辅助**维度；话题链仍在同一位置且资格口径不变；用户可混合两链手动精修；保存时不因事件线产生任何话题变更 |
| 一个分区同时属于多条事件线链 → 「整组勾齐」可能一次带入大量跨话题材料 | 选择膨胀 | 链元信息显式给出成员分区数与跨天数；取消单链只撤销本链成员（spec「事件线链只提供整组勾选」） |
| 内存构造每次查询重算 | 高频刷新下有额外 CPU | D8/D9 的成本对照；必要时后续按 board+window 做短 TTL 缓存（不在本 change） |
| `board_stories` 一跳解析出现两层链/坏引用 | 少数线索被跳过 | 跳过 + warn，SHALL NOT 影响其它链（D3）；单测覆盖（test-cases §5.1） |
| 未来若把事件线边落库，会与 D5 的「不污染 status 推导」冲突 | 语义回归 | D5 已写死理由；若确需落库，必须先改 `section-relations` 的 status 推导契约并另立 change |

## Open Questions

无。是否存在需要用户再拍板的点：**无**（Q3 已由用户拍板为「事件线作为新增维度，两链并列」；成链门槛、落库 vs 内存、相似推荐优先级三项均在本设计内定案并给出理由与被否方案）。
