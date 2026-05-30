## Context

当前 my-robot 的标签→板块体系存在三个核心问题：

1. **OOV 匹配失败**：embedding 模型不认识的新词（如 happyhorse）无法准确归类到对应板块
2. **事件语义丢失**：事件标签（如"霍尔木兹海峡"）与板块（如"地缘政治"）的直接 embedding 比对语义关联弱
3. **分类僵化**：event/keyword/person 三类板块独立管理，丢失跨视角叙事能力

现有架构依赖层级模板（hierarchy_template）+ 抽象标签（abstract_tag）+ embedding 直接匹配，共约 20 个文件，逻辑复杂且难以扩展。

## Goals / Non-Goals

**Goals:**
- 引入辅助标签（Auxiliary Label）作为 tag 和 board 之间的统一语义中介层
- LLM 提取 tag 时同时生成辅助标签，解决 OOV 和语义丢失问题
- 辅助标签通过聚类 + LLM 判断自动升级为板块
- tag 可属于多个 board（多视角叙事）
- 统一板块管理，不再区分 event/keyword/person
- 删除现有层级体系和抽象标签体系

**Non-Goals:**
- 不做历史数据迁移（开发阶段全删重建）
- 不做联网搜索丰富辅助标签含义（后续迭代）
- 不做自动升级/自动回填（仅用户手动触发 + 用户确认 + 异步队列）
- 不保留旧 abstract tree → hotspot board 路径

## Decisions

### D1: 辅助标签与板块共用 semantic_labels 表

**决策**：辅助标签和板块共存于 `semantic_labels` 表，通过 `label_type` 字段区分。

**理由**：板块本质上是从辅助标签池中聚类升级出来的语义节点，共享 embedding、merge、alias 等基础设施。统一表避免了跨表 join 和数据同步。

**备选方案**：辅助标签和板块分表管理 — 被否决，因为增加了数据同步复杂度，且板块本质上就是辅助标签的聚合。

### D2: Board 是独立实体，不从辅助标签"变身"

**决策**：LLM 从辅助标签簇中生成全新的 board 实体（独立 name/description/embedding），辅助标签保持不变。通过 `board_composition` 表记录 board 由哪些辅助标签组成。

**理由**：辅助标签的 embedding 是短词向量，board 的 embedding 应基于 LLM 生成的 name+description，语义更丰富。保持辅助标签稳定，避免 embedding 不一致问题。

### D3: Tag → Board 匹配通过辅助标签交集

**决策**：不再做 tag embedding ↔ board embedding 直接比对。匹配逻辑为 tag 的辅助标签与 board 的构成标签取交集，计算命中率和 max_sim。

**三级匹配规则**：
- 直接命中：tag 的辅助标签 ∈ board 构成标签 → 直接挂载
- 命中率 > 50% → 直接挂载
- max_sim ≥ 0.8 → 直接挂载
- 加权综合：0.6 × max_sim + 0.4 × hit_rate ≥ 阈值 → 挂载

**理由**：辅助标签本身就是结构化的语义锚点，比直接 embedding 比对更精确。

### D4: 辅助标签 merge 阈值 0.95 + alias 自动积累

**决策**：新辅助标签入库时，embedding ≥0.95 合并到 ref_count 更大的一方，小方 label 自动加入大方 aliases。

**理由**：0.95 是非常高的阈值，误合并概率低。alias 积累后支持 L1 精确匹配复用，减少 embedding API 调用。

### D5: 板块升级采用两阶段：预聚类 + LLM 判断

**决策**：当 ref_count ≥ 5 的未升级辅助标签达到阈值时：(1) embedding 预聚类压缩为 8-10 个簇；(2) 每个簇补充 co-tag 事件上下文后送 LLM 判断升级/合并/跳过。

**理由**：直接将大量候选送 LLM 会导致 prompt 爆炸。预聚类压缩后再送 LLM，既保留了语义完整性，又控制了成本。co-tag 事件提供上下文语义，避免纯标签分组丢失事件含义。

### D6: 删除层级体系全家族

**决策**：移除 hierarchy_template、hierarchy_placement、hierarchy_cleanup、hierarchy_dedup、hierarchy_aggregation、hierarchy_handler、hierarchy_orchestration、hierarchy_prompts、abstract_tag_*.go、adopt_narrower_queue、tree_bridge、multi_parent_resolve_queue、concept/bootstrap.go、concept/matcher.go。

**理由**：新架构通过辅助标签 → board 的扁平关系替代了多级层级树，这些代码不再有用途。

### D7: SemanticBoard 全局共享，NarrativeBoard 仍按 scope 每日生成

**决策**：`semantic_labels(label_type=board)` 表示全局共享的长期语义板块（SemanticBoard）。`narrative_boards` 表示每日叙事板实例，仍保留 `scope_type` / `scope_category_id`，从当日文章范围内属于该 SemanticBoard 的 event tags 派生。

**理由**：板块语义应跨订阅分类复用，避免同一个长期主题在不同 feed category 下重复维护；但每日叙事展示仍需要按 global/feed_category 分别生成。

### D8: Tag-Board 归属必须持久化

**决策**：新增 `topic_tag_board_labels` 表持久化 tag 到 SemanticBoard 的多对多归属。`topic_tag_semantic_labels` 只记录 tag 到辅助标签的关联，`topic_tag_board_labels` 记录匹配结果。

**理由**：动态计算无法稳定支持 tag_count、文章 board 筛选、回填幂等、前端多板块展示和叙事生成输入收集。持久化结果便于增量重算和排错。

### D9: 多板块归属默认最多 3 个，文章允许重复出现

**决策**：一个 tag 可归属多个 SemanticBoard，默认最多 3 个，按匹配分从高到低截断。同一 event tag 及其文章允许在多个 NarrativeBoard 中重复出现。

**理由**：多视角叙事需要允许重复，但无限归属会稀释 board 语义、增加 UI 噪音和叙事生成成本。

### D10: 板块升级必须用户手动触发并确认执行

**决策**：系统只在用户手动触发时运行 LLM 升级建议，返回 create_new / merge_into_existing / skip 建议；只有用户确认后才写入 SemanticBoard、board_composition，并允许用户手动触发回填。新辅助标签不会自动升级为板块；如果用户希望把辅助标签纳入已有板块，可通过 board composition 的辅助标签推荐和手动添加流程完成。

**理由**：SemanticBoard 是长期语义资产，自动生成过多低质量板块会污染后续匹配。手动确认把 LLM 作为建议器，而非最终写入者。

### D10a: 升级建议确认是逐项处理，不关闭整轮建议

**决策**：升级建议面板中的 create_new / merge_into_existing 建议 SHALL 支持逐项确认。确认单个建议成功后，面板保持打开，并将该建议标记为已处理或从待处理列表移除；用户可继续处理剩余建议。面板 SHALL 提供重新生成建议入口，允许用户在候选池或判断结果变化后重新调用 LLM 建议。

**理由**：一次升级建议通常包含多个候选簇。确认一个建议后立即关闭弹窗会打断批量治理流程，也会让用户误以为本轮建议已全部处理完成。重新生成入口让用户在已处理部分建议、手动调整 composition 或候选池变化后，可以获得新的建议集合。

### D11: 冷启动允许短期无 board

**决策**：冷启动阶段允许没有 SemanticBoard。系统先积累辅助标签池；当候选辅助标签达到阈值后，用户可手动触发 LLM 初始化建议，确认后创建第一批 SemanticBoard，再执行回填。

**理由**：没有历史迁移时，强行预置板块会引入外部先验；从真实标签池生成更贴合用户订阅语义。

### D12: 保留最小修正能力

**决策**：辅助标签和 board composition 必须支持最小治理能力：禁用辅助标签、手动合并 alias、从 board composition 移除辅助标签。修正动作不自动删除历史 tag-board 结果，用户可手动触发回填重算。

**理由**：LLM 生成和 embedding merge 都可能出错。完全不提供修正会导致错误语义长期污染板块。

### D13: 彻底取消旧热点板路径

**决策**：删除 `abstract tree → hotspot NarrativeBoard` 路径。所有每日 NarrativeBoard 均从 SemanticBoard 派生；没有 SemanticBoard 或没有匹配 event tags 时，不生成对应 board。

**理由**：继续保留热点板会让新旧两套语义体系并行，增加概念混乱。突发热点后续可通过“高频辅助标签/事件簇升级为 SemanticBoard”表达。

### D14: 历史数据由用户手动清空，不做迁移

**决策**：本变更不提供旧 `board_concepts`、`concept_id`、`topic_tag_relations`、abstract tag 到新 semantic label 模型的数据迁移。开发阶段由用户手动删除旧数据后重建。

**理由**：旧体系和新体系语义模型差异较大，自动迁移容易制造低质量板块和错误归属。当前项目允许重建，优先降低实现复杂度。

### D15: 配置 key 使用 semantic_board 命名空间

**决策**：新增匹配和升级配置统一使用 `semantic_board_*` 前缀，例如 `semantic_board_match_sim_threshold`、`semantic_board_match_max_boards`、`semantic_board_upgrade_ref_count_threshold`。

**理由**：避免与现有 `narrative_board_embedding_threshold` 等旧配置混淆，也便于后续删除旧配置。

### D16: Keyword 标签直接进入辅助标签池

**决策**：category=keyword 的 tag 不再生成 3-5 个辅助标签，而是将 tag 自身（label + description）直接作为辅助标签入库。event/person 标签仍按原方式生成 3-5 个辅助标签。

**理由**：keyword 标签（如 "OpenAI"、"PostgreSQL"、"Transformer架构"）本身就是语义锚点，不需要再生成辅助标签来"解释"它们。当前 keyword 的辅助标签中第一个往往就是 tag 本身，造成冗余。keyword 已有 tagger.go 生成的 description（有文章上下文支撑，质量更高），直接复用即可。

**备选方案**：keyword 也生成辅助标签 — 被否决，因为浪费 LLM 输出 token，且辅助标签的 description 质量不如 tag 自身的 description。

### D17: 辅助标签 embedding 分离为 merge-embedding 和 storage-embedding

**决策**：辅助标签入库时使用两种 embedding：(1) `merge_embedding`：仅用 label 文本生成，用于 L2 ≥0.95 merge 判断；(2) `embedding`：用 label + description 生成，作为 storage embedding 存储到数据库并用于后续 board 推荐、匹配、升级聚类和回填。

**理由**：短文本 embedding 用于 merge 判断更稳定（不受 description 差异影响）；长文本（label+description）embedding 用于匹配区分度更高，能显著降低跨域误判（如 "Claude Code" ↔ "伊朗" 的余弦相似度）。L1 精确匹配不需要 embedding，L2 用 `merge_embedding` 判断，L3 新建时同时写入 `merge_embedding` 和 storage `embedding`，确保后续新标签仍能与既有标签做 label-only merge 比较。

### D18: 辅助标签 description 增强 embedding 语义

**决策**：辅助标签入库时写入 description 字段，storage-embedding 的输入为 label + ": " + description。

**理由**：当前 embedding 输入仅用 2-3 token 的短标签名，在 Qwen3-Embedding:4b 等小模型中区分度严重不足（跨域相似度 0.65+）。增加 description 后输入变为 20-30 token，embedding 被锚定到正确的语义子空间，预期跨域相似度降至 0.3-0.45，远低于当前 SimThreshold=0.72。

来源规则：
- keyword tag 直入：复用 tag 已有的 description（由 tagger.go 生成）
- event/person 的辅助标签：LLM 提取时为每个辅助标签附带简短 description

### D20: Tag 提取拆分为 event/person 和 keyword 双调用

**决策**：`extractCandidates` 从单个 LLM 调用拆分为两个独立调用：
1. `extractEventPersonCandidates`：提取 event/person 标签，schema 强制 `auxiliary_labels` 为 required
2. `extractKeywordCandidates`：提取 keyword 标签，schema 只含 `label` + `description` + `aliases`，不含 `auxiliary_labels`

两个调用可并行发起，但不是 fail-fast：任一分支失败不得取消另一个分支。每个分支独立执行最多 3 次重试，最后合并成功分支的结果，并把失败分支错误写入 `ExtractionResult.Errors`。

合并规则：
- 合并后最多保留 5 个 tag，keyword 最多 3 个
- 同 slug 跨分类重复时，按 person > event > keyword 保留更具体分类
- event/person 分支全败时不生成 event/person tag，不用 heuristic 猜事件
- keyword 分支全败时可使用 heuristic keyword 作为展示兜底，但 heuristic keyword 缺少同次 LLM description，默认不进入辅助标签池
- 两个分支均失败时，沿用现有整体 heuristic fallback

metadata operation 拆分为 `tag_extraction_event_person` 和 `tag_extraction_keyword`，便于在 ai_call_logs / tracing 中单独观察失败率、耗时和成本。

**理由**：统一 schema 中 `auxiliary_labels` 对 event/person 必填、对 keyword 无意义，LLM 经常忽略或全不输出，导致 `parseExtractedTags` 对整个 batch fail-fast。拆分后每个 schema 语义单一，LLM 遵从率显著提高。keyword 的 description 是 required 字段，直接作为直入池的 embedding 输入，不再被 event 标签的 auxiliary_labels 缺失连带丢弃。

**影响**：每篇文章 LLM 调用从 1 次变为 2 次（可并行），token 成本增加约 30-50%，但重试失败率大幅降低，净成本可接受。`article_tagger.go` 下游入库逻辑保持不变，但 `ExtractTags` 需要表达 partial success，避免把“keyword 成功、event 失败”误报为整体成功或整体失败。

### D19: 辅助标签推荐只服务人工 composition 管理

**决策**：`suggest-auxiliaries` 仅用于用户手动创建或编辑 SemanticBoard 时推荐 board_composition 候选。推荐可使用 board label + description embedding 与辅助标签 storage embedding 排序，但推荐结果不直接写入、不参与自动 tag-board 匹配规则。

**理由**：人工推荐需要“相似候选排序”帮助用户快速选标签；自动匹配仍应保持 D3 的辅助标签交集/命中率规则，避免重新退回 tag embedding ↔ board embedding 的直接匹配模型。

## Risks / Trade-offs

- **[辅助标签质量]** LLM 生成的辅助标签质量直接决定下游匹配准确性 → 通过 prompt 工程、3-5 个数量限制、禁用/alias 合并/composition 移除修正能力控制质量
- **[升级粒度]** LLM 可能生成粒度过粗或过细的板块 → 用户可通过 manual 方式调整，protected 机制保留
- **[匹配性能]** tag × board × 辅助标签的匹配是 O(M×N) → tag 和 board 数量在可控范围内（预计 <100 board），匹配结果持久化后读路径可接受
- **[冷启动周期]** 新系统需要积累足够辅助标签才能触发首次升级 → 冷启动允许短期无 board，阈值设为 ref_count≥5，预计 50+ 篇文章后即可触发
- **[回填成本]** 手动回填可能需要处理大量历史 tag → 异步队列 + 批量处理
- **[重复事件展示]** 同一事件可能出现在多个 board → 默认最多 3 个归属，UI 需把重复解释为多视角而非数据重复
- **[description 跨域区分度]** description 增强后跨域相似度的实际降幅需要实测验证 → 如果降幅不足（仍 >0.5），可配合提高 SimThreshold 或升级 embedding 模型
- **[embedding 分离复杂度]** `merge_embedding` 和 storage `embedding` 分离增加入库逻辑和迁移复杂度 → L1 不需要 embedding、L2 用 `merge_embedding`、L3 生成两种 embedding，匹配/推荐/聚类统一使用 storage `embedding`
