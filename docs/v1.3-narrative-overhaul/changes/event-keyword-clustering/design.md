## Context

当前事件 tag 聚合有 3 条路径，全部失效或覆盖不足：

1. **PlaceTagInHierarchy** — 依赖 `MatchTagToConcept`，但 `board_concepts` 表为空（手动触发），入口短路
2. **ClusterUnclassifiedTags** — 代码存在但从未被 scheduler 调用，且只用 semantic embedding（对事件区分度差）
3. **narrative/tag_feedback** — 窗口窄（仅 sim ∈ [0.72, 0.90)），每 narrative 最多 5 对

数据现状：244 个 event topic tag，240 个孤立。仅 1 个 abstract event tag。

已有的 `event_keywords` 元数据（每个 tag 5 个关键词）和对应的 `event_keyword` embedding 是未被利用的强信号。数据验证表明：
- shared_kws >= 2 的 75 对中，semantic >= 0.80 的 49 对几乎全部是同一事件链
- shared_kws=1 的 414 对噪音太大（"美国"、"微软"等泛化词）
- semantic-only 的 0.85 阈值漏掉大量同事件 tag 对（如 "特朗普车队抵达" vs "欢迎仪式" sim=0.80）

## Goals / Non-Goals

**Goals:**
- 将 240 个孤立 event tag 中的可聚合对识别出来，通过两阶段过滤后送入 LLM judgment
- 将 `ClusterUnclassifiedTags` 集成到现有 `tag_hierarchy_cleanup` scheduler，对 event category 生效
- 聚类参数可配置（关键词重叠阈值、semantic 阈值）

**Non-Goals:**
- 不修改 concept / board_concepts 机制（手动触发保持不变）
- 不修改 hierarchy_placement.go 的 concept 短路问题
- 不给 abstract tag 补充 event_keyword embedding（后续优化）
- 不做前端复杂交互，仅状态展示

## Decisions

### D1: 两阶段聚类（keyword overlap → semantic filter）

**选择**: Stage 1 关键词文本交集 shared_kws >= 2，Stage 2 semantic sim >= 0.80

**备选方案**:
- A) 纯 semantic 聚类（现有方案）— 区分度不够，0.70~0.85 区间真假阳性混合
- B) 纯 keyword embedding 余弦相似度 — 对共享泛化词（"美国"、"微软"）全部给出 1.000
- C) 加权融合 0.6\*semantic + 0.4\*keyword — 实现复杂，调参困难
- D) 两阶段 ✓ — 简单、可解释、数据验证充分

**理由**: 数据验证明确。shared_kws>=2 是高精度过滤器（75 对），再叠加 semantic>=0.80 进一步收紧到 49 对。这两个阈值的选取基于实际数据分布，不是拍脑袋。

### D2: 关键词比较用文本精确匹配，不用 embedding

**选择**: 直接 `jsonb_array_elements_text` 做交集计数

**理由**: event_keyword embedding 对相同文本给出 sim=1.000（hash 一致），本质上等价于文本精确匹配，但多了 embedding 计算开销。直接文本比较更简单高效。

### D3: 集成到 tag_hierarchy_cleanup scheduler

**选择**: 在 Phase 2（flat merge）之后新增 Phase 2.5，只对 event category 调用

**理由**: 复用现有 scheduler 基础设施，不引入新 scheduler。flat merge 先处理 obvious duplicates（semantic >= 0.85），然后 keyword-aware clustering 处理 semantic 0.80~0.85 的模糊地带。

### D4: 配置项扩展

新增 `embedding_config` 行：
- `event_cluster_kw_min_overlap` = 2（关键词最小重叠数）
- `event_cluster_sem_threshold` = 0.80（semantic 过滤阈值）

复用现有：`cluster_max_tags`、`cluster_max_size`

## Risks / Trade-offs

**[LLM 调用增加]** → 每轮 ~49 对进入 `ExtractAbstractTag`，在现有 budget（60 次/轮）内可控。且有 `cluster_max_size=8` 限制单 cluster 大小。

**[shared_kws=2 + sem=0.80 仍有少量假阳性]** → 这些对进入 LLM judgment 而非直接聚合，LLM 可拒绝。假阳性代价是浪费一次 LLM 调用，不是错误聚合。

**[关键词质量依赖 LLM 提取]** → 如果 event_keywords 提取质量下降（如全是泛化词），聚类效果会退化。但这属于上游问题，不在此变更范围内。

**[只对 event category 生效]** → keyword/person category 保持原有 semantic-only flat merge。后续可扩展但不急于一时。
