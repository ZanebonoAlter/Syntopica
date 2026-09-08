# Design: add-composite-labels

## Context

见 `proposal.md - Why`。现状要点：挂载体系基本单元是中性辅助标签（提取 prompt 要求"具体语义锚点"），指向信息在 LLM 抽取那一刻丢失；匹配四规则中 direct_hit（单标签重叠 ≥2）score=1.0 且豁免 direction check；direction check 度量主题相关性而非指向一致性（反义词 embedding 近邻）。升级建议聚类输入是 aux embedding，产出中性新板，问题代际复制。

调研详情存档：`docs/research/composite-label/explore-findings.md`（含现有代码定位：`semantic_board_matching.go` 四规则与 direction check、`semantic_board_upgrade.go` co-tag 统计配置、`extractor_enhanced.go` 提取 prompt）。

## Goals / Non-Goals

**Goals:**

- 挂载单元从"中性概念"升级为"指向性主题"：组合标签成为 tag→board 匹配的最强信号
- 堵住「单标签重叠免检直进」漏洞：direct_hit 降级 + 强制方向校验
- 组合标签产出以升级建议产线为主源（co-tag 共现数据已在，只当证据用 → 升格为聚类输入）
- 语义粒度阶梯 aux → composite → board 的第一级落地

**Non-Goals:**

- 走势方向槽位（B 类：上行/下行）——反义词需受控枚举，独立课题
- 提取侧 LLM 直接识别组合（见 Open Questions Q1）
- composite → board 第二级升级链路（见 Open Questions Q2）
- 存量共现对批量回填策略（见 Open Questions Q4）

## Decisions

### D1: 组合标签 = semantic_labels 第三种 label_type，不建独立表树

- **选择**：`label_type="composite"` + 新表 `composite_components`（composite_id, component_label_id, position），挂载关系复用 `topic_tag_semantic_labels` / `board_composition`。
- **理由**：挂载关系表、治理面板 CRUD、禁用即弃向量红线、ref_count 维护全部自动继承；改动集中在产出链路与匹配规则。
- **备选（否决）**：独立 composite_labels 表树——所有关联表要复制一套，治理面板另开一套 CRUD，收益为零。

### D2: 组合 embedding 由 LLM 对组合短语生成，禁止组件向量合成

- **选择**：创建时以「label + 组件序列化」调用 embedder（复用 board embedding 的生成模式：`label + ". " + description`），失败则创建事务回滚。
- **理由**：组件向量的加权/平均 ≈ 主题域泛化向量，恰好丢掉组合的指向性灵魂——这是整个方案成立与否的物理基础。
- **备选（否决）**：组件向量合成——免 LLM 调用但语义错误，等于白做。

### D3: 去重 canonical 化——组件 ID 归一 + 集合比较（L1），组合 embedding 兜底（L2）

- **选择**：L1 比组件 canonical ID 的无序集合（组件先归一到各自 aux 的 canonical ID，同义组件不产生不同组合）；L2 比 LLM 组合 embedding cosine ≥ `composite_dedupe_sim`（默认 0.95，对齐 aux L2 阈值），命中只 addAlias + ref_count++（照抄 aux 防黑洞纪律：不改 label、不重算 embedding）；均未命中才新建。
- **理由**：组合空间是 O(n²)，碎片化代价比单标签高一个量级；「美债收益率」「美国国债收益率」「国债收益率」三种说法的组件集合可能因 aux 同义簇不同而表面不同，纯 ID 集合比较不够，纯 embedding 比较又太松——两级串联刚好。
- **备选（否决）**：仅 embedding 相似（L2 单级）——0.95 阈值下「美债收益率」和「中债收益率」可能过近误合并；仅 ID 集合（L1 单级）——组件同义漂移产生重复组合。

### D4: compose 作为升级建议第三种决策，复用现有生命周期

- **选择**：co-tag 共现统计（现 `loadCoTagEventContext` 的窗口/去重配置，30 天窗口）升格为聚类输入：共现频次 ≥ `composite_cotag_min_cooccurrence`（默认 10）且组件 ref_count 达标 → 候选共现对 → LLM 裁决（过滤「日本+市场」类无意义组合）→ compose 建议落库。suggestion_hash 幂等、dismissed 冷却期、watch 观察池机制原样复用（ComputeSuggestionHash 扩展 compose 决策的签名输入：mode + decision + 组件 ID 有序序列）。
- **理由**：共现频次是"值得组合"的最强客观信号，数据已在采集；生命周期机制（幂等/冷却/确认事务联动）重复建设无意义。
- **备选（否决）**：独立组合建议表——生命周期状态机、冷却、幂等全部重写。

### D5: 匹配规则——composite_hit 最强 + 单标签 direct_hit 降级

- **选择**：
  - composite_hit：tag 组合（显式关联 ∪ **推导命中**：tag 挂齐某 active 组合全部组件 aux 即视为挂该组合——8.2 真实库验证发现显式关联零写入的链路缺口后补的闭环语义）∩ board composition 组合 ≠ ∅ → score=1.0，天然免 direction check（组合命中即指向一致）
  - 单标签 direct_hit：交集 ≥ direct_hit_min_overlap → score=`direct_hit_score_factor`（默认 0.7）× 原逻辑，且强制 direction check（低阈值 → direction_mismatch=true，前端默认隐藏，行为对齐间接匹配）
  - 间接三规则不变
  - 同一 tag-board 同时满足组合命中与单标签重叠时，只记录 composite_hit（最高优先级 match_reason）
- **理由**：score=1.0 的免检通道必须只留给指向一致的信号；单标签重叠是"话题域相同"的弱证据，降级 + 方向校验止血。0.7 折扣让 direct_hit 仍强于典型 weighted 分但弱于组合命中与高质量 hit_rate 匹配，排序语义合理。
- **备选（否决）**：只加 composite_hit 不降级存量——组合标签冷启动期间（覆盖率低）中性漏洞依旧敞开。

### D6: 匹配输入与缓存扩展

- **选择**：MatchTopicTag 加载 tag 组合标签 + 各 board composition 组合标签（含 embedding），纳入 board match cache（与 board auxiliaries/embeddings 同等失效语义：composition 变更即失效）。
- **理由**：匹配是高频路径，组合标签查询必须走缓存。

### D7: 手动创建对话框的组件推荐交互（用户 review 补充）

- **问题**：可选组件（active aux）就那么一批，用户还得自己想关键词搜——纯搜索式选择器对不熟悉 aux 池的用户鸡肋。
- **组件候选按推荐度排序**（`GET /composite-labels/component-options`，服务端一次算好）：版块维度优先——被 active 版块挂载的 aux 是用户治理过的核心概念，排最前（挂载数多者在前），其后按 ref_count（使用频次），同分按 label。默认列表限 top 50，搜索时降级全量模糊搜索（保留原有能力）。chip 上展示「挂 N 版块」推荐信号，让用户理解为什么排前面。
- **选中组件后优先展示相关的现有组合**：前端用已有组合列表（Pool 页面已在手，props 传入对话框，零额外请求）本地过滤「含任一已选组件」的组合，展示组合名+组件链，引导复用防重复创建；选中集合与某组合组件集完全一致时直接提示「创建将复用该组合」（对应后端 L1 去重语义）。
- **升级（用户第二轮 review 后）**：入口与推荐都以**版块为中心**——`BoardCompositionPanel` 加「组合标签」区（本版块挂载组合列表 + 移除 + 「创建并挂载」入口）；`component-options` 支持 `board_id`（本版块挂载组件置顶标「本版块」）与 `related_to`（与已选组件同 tag 共现频次驱动候选实时重排，选完「美联储」→「加息」浮上来标「共现N」——真实库组合稀少，纯现有组合提示联动太弱）；创建成功自动挂载到版块上下文。
- **不做的**：不做后端个性化推荐（无用户画像）、不自动预填组件（推荐只是排序+提示，选择权在用户）。

### D8: 迁移 = 迁移表结构 + 建议先行 + 全量重算

- 上线步骤：(1) DB 迁移（composite_components 表 + 建议表 decision 枚举/展示兼容）；(2) 跑一轮 compose 建议生成（存量共现对）；(3) 用户确认一批组合标签；(4) 一次性 mode="all" 匹配重算（新规则重写存量归属）。
- **回滚**：代码回滚 + 重跑旧规则 backfill 即恢复原归类；composite_components 表与组合标签行留存无害（旧代码不读 label_type=composite）。

## Risks / Trade-offs

- [组合标签冷启动期覆盖率低] → 匹配行为 = 现状 + direct_hit 降级；上线序列强制「建议先行、确认后再全量重算」，避免用户看到归类突然变空。
- [compose 建议质量：共现 ≠ 值得组合（地名+通用词、偶然共现）] → LLM 裁决过滤 + 用户确认兜底 + dismiss 冷却期，不自动创建任何组合标签。
- [组合 embedding LLM 调用成本] → 每组合一次 embed（创建时），组合数量受共现阈值与用户确认双重闸门，量级可控。
- [co-tag 候选对数量 O(n²)] → 只统计 ref_count 达标的高频 aux，共现频次阈值先收敛，LLM 输入带 topN 上限（复用 CoTagHardLimit 模式）。
- [全量重算改变用户可见归类] → direct_hit 降级使部分存量归属的 score/direction_mismatch 变化；deployed-behavior 汇报明确告知（见 tasks 文档节），重算前建议先确认组合标签缓解空窗。
- [direction_mismatch 存量语义变化] → 原先 direct_hit 记录无 direction_mismatch 概念，重算后部分记录新增该标记且前端默认隐藏——用户可见行为变化，需在完工汇报中说明。

## Migration Plan

1. DB 迁移：`composite_components` 表（PK (composite_id, component_label_id)，FK 级联删除，component 校验 aux 类型在服务层）；建议表无 schema 变更（decision 为字符串列）。
2. 后端发布：匹配规则新代码 + compose 建议生成 + 组合标签 CRUD。
3. 运维序列：generate 一轮建议 → 用户确认 → backfill mode="all"。
4. 回滚：回滚代码 → 重跑旧规则 backfill；数据层面 composite 行无害残留。

## Open Questions

- **Q1 提取侧 LLM 直接识别组合**：文章明说"美债收益率"时提取端直接标记组合（组件 L1 匹配已有 aux 防碎片）。推迟理由：升级建议产线先验证组合标签的质量与覆盖模式，避免两个源头同时开闸导致碎片化失控；若建议产线产出偏慢再开提取侧。
- **Q2 composite → board 第二级升级**：组合标签成簇后升级为版块（复用 watch 观察池模式）。推迟理由：一期先让组合标签进入匹配体系，观察组合标签的 ref_count 增长形态再设计升级判据。
- **Q3 B 类走势方向槽位**：上行/下行作为组合第三槽位。推迟理由：反义词 embedding 近邻，必须走受控枚举（不能进 embedding 去重），涉及提取与匹配两端的词表管理，独立课题。
- **Q4 存量共现对初始回填**：一次性批量生成建议（可能一次几十条）vs 只从增量共现开始。推迟理由：取决于用户对首轮建议量的接受度，上线时用现有 generate 接口手动触发即可观察，无需预先写死策略。
