# 打标签流程全景说明

> **版本**：基于 `backend-go/internal/domain/topicanalysis/` 与 `topicextraction/tagger.go` 实际代码整理（2026-04 更新：清理调度器扩展为 10 阶段、标签提取限额调整、keyword 自动复用门槛降低）  
> **阅读建议**：先看主流程图，再按需深入各子章节

---

## 0. 标签提取配置

| 参数 | 值 | 说明 |
|------|-----|------|
| `maxArticleTags` | 5 | 每篇文章最多提取 5 个标签 |
| keyword 上限 | 3 | 其中 keyword 类型最多 3 个 |
| `cluster_max_tags` | 500 | 未分类标签聚类上限 |

keyword 提取 prompt 强调持久辨识度：只提取在多篇文章中反复出现的实体或术语，瞬态描述词不作为 keyword。原则是宁少勿多——如果文章只聚焦一个话题，2-3 个标签就够了。

---

## 1. 统一入口：`findOrCreateTag`

**触发场景**：文章/摘要生成标签、手动整理、叙事反馈等

```go
输入: tag topictypes.TopicTag, source string, articleContext string, articleID uint
输出: *models.TopicTag  (复用现有 / 新建 / 归入抽象)
```

### 主流程

```mermaid
flowchart TD
    START([新标签]) --> SVC{EmbeddingService<br/>可用?}
    SVC -->|否| FALLBACK[slug精确匹配<br/>或新建标签]
    SVC -->|是| TAGMATCH[TagMatch 三级匹配]
    
    TAGMATCH --> EXACT{MatchType}
    EXACT -->|exact| REUSE[复用现有标签<br/>更新元信息]
    EXACT -->|no_match| CREATE[新建标签<br/>生成embedding]
    EXACT -->|candidates| CAND{有候选<br/>≥0.78?}
    
    CAND -->|是 & event| COTAG[Co-tag扩展<br/>补充候选]
    CAND -->|否| CREATE
    COTAG --> LLM[送LLM批量判断]
    CAND -->|其他分类| LLM
    
    REUSE --> END1([返回标签])
    CREATE --> END1
    LLM --> RESULT{判断结果<br/>merges/abstracts/none}
    RESULT -->|merge| MERGE[合并到目标<br/>目标相似度须≥0.85]
    RESULT -->|abstract| ABSTRACT[创建抽象父标签]
    RESULT -->|none| CREATE
    MERGE --> END1
    ABSTRACT --> END1
    ```

> **注意**：对于 event/keyword 标签，description 在提取阶段（ExtractTagsFromArticle）已由 LLM 一并生成，创建时直接写入，无需额外调用 generateTagDescription。person 标签仍需要单独生成结构化属性。

---

## 2. Embedding 三级匹配：`TagMatch`

| 级别 | 匹配方式 | 阈值 | 行为 |
|------|----------|------|------|
| **L1** | slug 精确匹配 | — | 直接复用 |
| **L1** | 别名(alias)匹配 | — | 直接复用 |
| **L2** | embedding 相似搜索 | ≥0.97（keyword ≥0.90）| 自动复用(Exact) |
| **L2** | embedding 相似搜索 | 0.78~0.97（keyword 0.78~0.90）| 送LLM判断(Candidates) |
| **L2** | embedding 相似搜索 | <0.78 | 新建标签(No Match) |

---

## 3. LLM 批量判断：`callLLMForTagJudgment`

**输入**：候选列表(≤8个/批)、新标签名、分类、叙事上下文  
**输出**：`tagJudgment` (merges / abstracts / none 三个数组，每个候选必须且只能出现在一个数组中)

```mermaid
sequenceDiagram
    participant Caller as findOrCreateTag
    participant LLM as LLM Router
    
    Caller->>Caller: 检查候选最高相似度<br/>≥0.85? 正常merge提示 : 加CAUTION警告
    Caller->>LLM: 构建Prompt(merge始终可用+含分类专属规则)
    Note right of Caller: person/event/keyword<br/>规则各不相同
    LLM-->>Caller: JSON响应<br/>{merges: [...], abstracts: [...], none: [...]}
    Note right of Caller: 三个数组都必须返回<br/>每个候选只能出现在一个数组中<br/>例：候选A merge + BCD abstract + EF none
    Caller->>Caller: parseTagJudgmentResponse<br/>过滤无效候选、去重、交叉验证、截断
    Caller->>Caller: ensureNewLabelCandidateInAbstract<br/>确保newLabel在abstract.children中
```

### Prompt 标签上下文

所有标签判断类 prompt 统一通过 `formatTagPromptContext` 补充标签上下文：普通字段包含 `description`，人物标签还会追加 `metadata` 中的 `country / organization / role / domains`。这些结构化人物属性会进入 merge、abstract、收养更窄标签、多父冲突、层级判断、跨层去重、树审查、flat merge 与抽象标签刷新等 prompt，避免只靠姓名和 embedding 相似度误判不同人物为同一人。

### 判断规则速查

| 分类 | Merge条件 | Abstract条件 |
|------|-----------|--------------|
| **person** | 同一人物不同称谓 | 共享身份/机构/领域 |
| **event** | 同一事件不同描述 | 因果关联或同一事件链 |
| **keyword** | 同义词/翻译 | 同一具体领域直接相关 |

---

## 4. 抽象标签创建：`processAbstractJudgment`

**关键保护机制**：

```mermaid
flowchart LR
    A[开始创建抽象标签] --> B{slug与候选冲突?}
    B -->|是| C[跳过创建]
    B -->|否| D[findSimilarExistingAbstract<br/>查重已有抽象]
    D --> E{找到相同概念?}
    E -->|是| F[复用已有抽象]
    E -->|否| G[事务创建新抽象]
    G --> H{子标签数≥最小值?}
    H -->|普通路径| I[min=1<br/>+ newLabel作为第2个]
    H -->|整理/反馈路径| J[min=2<br/>已有候选≥2]
    H -->|不足| K[事务回滚<br/>errInsufficientAbstractChildren]
    I --> L[建立父子关系]
    J --> L
    F --> L
```

**异步副作用**：
- 生成 `identity` + `semantic` embedding
- 触发 `MatchAbstractTagHierarchy`
- `EnqueueAdoptNarrower` 入队收养更窄标签（异步批量处理，见第10节）
- 入队 `abstract_tag_update_queues`

---

## 5. Event 标签 Co-tag 扩展

**目的**：用文章 keyword 反查共现 event，补充 embedding 召回遗漏

```mermaid
flowchart TD
    A[event标签有候选] --> B{来源}
    B -->|articleID| C1[取文章top5 keyword]
    B -->|abstractTagID| C2["聚合子树keyword覆盖<br/>topN = 5 + depth*2 - 2"]
    C1 --> D[反查共现文章]
    C2 --> D
    D --> E[提取关联event<br/>按hit_count排序]
    E --> F[与embedding候选并集<br/>相似度固定0.80]
```

---

## 6. 抽象层级匹配：`MatchAbstractTagHierarchy`

**触发**：新抽象标签创建后、刷新队列完成后

```mermaid
flowchart TD
    A[新抽象标签] --> B[Cross-layer Dedup]
    B --> C{高相似anchor<br/>≥0.97?}
    C -->|是| D[judgeCrossLayerDuplicate<br/>AI判断是否同一概念]
    D -->|是| E[MergeTags合并]
    D -->|否| F[继续层级匹配]
    C -->|否| F
    
    F --> G[FindSimilarAbstractTags]
    G --> SPLIT[按相似度分桶]
    SPLIT -->|≥0.97| I[mergeOrLinkSimilarAbstract<br/>逐个处理]
    I --> MERGED{已merge?}
    MERGED -->|是| END1([结束])
    MERGED -->|否| CHECK{中相似候选<br/>0.78~0.97?}
    SPLIT -->|0.78~0.97| COLLECT[收集到mediumSimilars]
    SPLIT -->|<0.78| SKIP[跳过]
    COLLECT --> CHECK

    CHECK -->|0个| END2([结束])
    CHECK -->|1个| SINGLE[judgeAbstractRelationship<br/>单候选逐个判断]
    CHECK -->|多个| BATCH[batchJudgeAbstractRelationships<br/>批量LLM判断]
    
    BATCH --> BATCHRESULT{批量结果}
    BATCHRESULT --> MERGE_FIRST[优先处理merge]
    MERGE_FIRST -->|有merge| END3([结束])
    MERGE_FIRST -->|无merge| PARENT[处理parent_A/parent_B]
    SINGLE --> SINGLEPROC[processAbstractRelationJudgment]
```

**LLM 调用优化**：中等相似度候选（0.78~0.97）不再逐个调用 LLM，而是打包一次性 `batchJudgeAbstractRelationships`，仅在单候选时 fallback 到 `judgeAbstractRelationship`。

---

## 7. 父子链接与多父冲突

### `linkAbstractParentChild` 保护

| 检查项 | 失败行为 |
|--------|----------|
| 循环检测 | 返回错误 |
| 深度限制(≤4条边，常量 `maxHierarchyDepth`) | 触发AI建议替代位置 |
| 已存在关系 | 静默跳过 |

### `resolveMultiParentConflict`

```mermaid
flowchart LR
    A[子标签有多父?] --> B[removeRedundantAncestorParents<br/>检查祖先-后代]
    B -->|有祖先关系| C[删除祖先父<br/>保留更具体的]
    B -->|无祖先关系| D[aiJudgeBestParent<br/>AI选最佳归属]
    D --> E[删除其他父标签]
```

---

## 8. Embedding 保留策略

```mermaid
flowchart TD
    A[标签归入抽象父] --> B{父标签是抽象?}
    B -->|否| KEEP1[保留embedding<br/>独立标签]
    B -->|是| C{同级有抽象兄弟?}
    C -->|是| KEEP2[保留embedding<br/>需精确匹配锚点]
    C -->|否| DEL[删除embedding<br/>父抽象是唯一入口]
    
    KEEP1 --> END1([结束])
    KEEP2 --> END1
    DEL --> END1
    
    style KEEP1 fill:#e8f5e9,stroke:#2e7d32
    style KEEP2 fill:#e8f5e9,stroke:#2e7d32
    style DEL fill:#ffebee,stroke:#c62828
```

**动态补回**：当普通标签突然获得抽象兄弟时，`enqueueEmbeddingsForNormalChildren` 异步补生成 embedding。

---

## 9. 抽象标签刷新队列

**触发时机**：
- `new_child_added` — `ExtractAbstractTag` 完成
- `hierarchy_linked` — 建立抽象父子关系
- `tag_merged` — 合并到抽象标签
- `adopted_narrower_children` — 收养更窄子标签

**批量处理**：除了实时队列 worker（3s 轮询）外，`TagHierarchyCleanupScheduler` Phase 5 会批量处理所有 pending 的刷新任务，确保整树审查前抽象标签的 label/description 已更新。

```mermaid
flowchart TD
    TRIGGER([子标签变化触发入队]) --> |去重: 已有pending/processing则跳过| PENDING[pending]
    PENDING --> |worker 3s轮询 FOR UPDATE锁定| PROCESSING[processing]
    PENDING --> |Phase 5 批量处理| PROCESSING
    PROCESSING --> |LLM重生成label+desc → 检查slug冲突 → 生成identity+semantic embedding → 触发MatchAbstractTagHierarchy| COMPLETED[completed]
    PROCESSING --> |异常| FAILED[failed]
    FAILED --> |retry_count++ 可手动重试| DONE1([*])
    COMPLETED --> DONE2([*])

    style PENDING fill:#fff3e0,stroke:#e65100
    style PROCESSING fill:#e3f2fd,stroke:#1565c0
    style COMPLETED fill:#e8f5e9,stroke:#2e7d32
    style FAILED fill:#ffebee,stroke:#c62828
```

> 核心文件: `abstract_tag_update_queue.go`、`queue_batch_processor.go`

---

## 10. 收养更窄标签队列：`adopt_narrower_queues`

**目的**：新抽象标签创建后，异步查找语义更窄的已有抽象标签并收养为子标签。通过队列去重 + 批量 LLM 判断，大幅减少 AI 调用次数。

**触发来源**：

| 来源 | source 标记 | 文件 |
|------|------------|------|
| `processAbstractJudgment` 新建抽象 | `processAbstractJudgment` | `abstract_tag_service.go` |
| `processAbstractJudgment` 复用已有抽象 | `processAbstractJudgment_reuse` | `abstract_tag_service.go` |
| `createAbstractTagDirectly` Phase 6 整树审查 | `createAbstractTagDirectly` | `hierarchy_cleanup.go` |

**批量处理**：除了实时队列 worker（5s 轮询）外，`TagHierarchyCleanupScheduler` Phase 4 会批量处理所有 pending 的收养任务，确保整树审查前窄概念标签已挂到对应抽象节点下。

### 队列流程

```mermaid
flowchart TD
    TRIGGER([抽象标签创建/复用]) --> EQ[EnqueueAdoptNarrower<br/>去重: 已有pending/processing则跳过]
    EQ --> PENDING[pending]
    PENDING --> |worker 5s轮询 FOR UPDATE锁定| PROCESSING[processing]
    PENDING --> |Phase 4 批量处理| PROCESSING
    PROCESSING --> EXEC[adoptNarrowerAbstractChildren]
    
    EXEC --> SEARCH[embedding相似搜索<br/>找同类抽象标签]
    SEARCH --> FILTER{过滤<br/>similarity ≥ LowSimilarity?}
    FILTER -->|无候选| COMPLETED[completed]
    FILTER -->|有候选eligible| BATCH
    
    BATCH[batchJudgeNarrowerConcepts<br/>批量LLM判断]
    Note right of BATCH | 旧: 逐候选调LLM → N次调用<br/>新: 打包一次LLM → 1次调用
    BATCH --> LINK[reparentOrLinkAbstractChild<br/>逐个建立父子关系]
    LINK --> |有收养| ENQUEUE2[入队抽象标签刷新<br/>adopted_narrower_children]
    LINK --> COMPLETED
    ENQUEUE2 --> COMPLETED
    
    PROCESSING --> |异常/panic| FAILED[failed]
    FAILED --> |retry_count++ 可手动重试| DONE1([*])
    COMPLETED --> DONE2([*])

    style PENDING fill:#fff3e0,stroke:#e65100
    style PROCESSING fill:#e3f2fd,stroke:#1565c0
    style COMPLETED fill:#e8f5e9,stroke:#2e7d32
    style FAILED fill:#ffebee,stroke:#c62828
    style BATCH fill:#e8eaf6,stroke:#283593
```

### 批量判断：`batchJudgeNarrowerConcepts`

将一个抽象标签的所有候选打包给 LLM，一次返回哪些是更窄概念：

```
输入: parentTag, candidates []TagCandidate (已过滤 LowSimilarity)
输出: narrowerIDs []uint (被判定为更窄的候选Tag ID列表)

特殊: 单候选时走 aiJudgeNarrowerConcept 逐个判断，保持向后兼容
      多候选时构建列表式 prompt，LLM 返回 {narrower_ids: [1,3,...]}
```

**LLM 调用优化**：假设 1 个抽象标签有 5 个候选 → 旧方案 5 次 `adopt_narrower_abstract` → 新方案 1 次 `adopt_narrower_abstract_batch`。

**去重保护**：PostgreSQL 部分唯一索引 `idx_adopt_narrower_active ON adopt_narrower_queues (abstract_tag_id) WHERE status IN ('pending', 'processing')`，同一标签不会有多个活跃任务。

> 核心文件: `adopt_narrower_queue.go`、`adopt_narrower_queue_handler.go`、`queue_batch_processor.go`  
> API: `GET /api/embedding/adopt-narrower/status`、`POST /api/embedding/adopt-narrower/retry`

---

## 11. 核心方法速查

| 方法 | 文件 | 输入 | 输出 | 一句话职责 |
|------|------|------|------|-----------|
| `findOrCreateTag` | `topicextraction/tagger.go` | tag, source, context, articleID | *TopicTag | **统一入口** |
| `TagMatch` | `topicanalysis/embedding.go` | label, category, aliases | TagMatchResult | **三级匹配** |
| `callLLMForTagJudgment` | `topicanalysis/abstract_tag_judgment.go` | candidates, newLabel, category, context | *tagJudgment | **LLM判断** |
| `processJudgment` | `topicanalysis/abstract_tag_service.go` | judgment, candidates, newLabel | TagExtractionResult | **结果处理** |
| `processAbstractJudgment` | `topicanalysis/abstract_tag_service.go` | candidates, judgment, newLabel, category | AbstractResult | **创建抽象标签** |
| `MatchAbstractTagHierarchy` | `topicanalysis/abstract_tag_hierarchy.go` | abstractTagID | - | **层级匹配** |
| `linkAbstractParentChild` | `topicanalysis/abstract_tag_hierarchy.go` | childID, parentID | error | **建立父子关系** |
| `resolveMultiParentConflict` | `topicanalysis/abstract_tag_hierarchy.go` | childID | bool | **解决多父冲突** |
| `ExpandEventCandidatesByArticleCoTags` | `topicanalysis/cotag_expansion.go` | articleID/abstractTagID | []TagCandidate | **co-tag扩展** |
| `refreshAbstractTag` | `topicanalysis/abstract_tag_update_queue.go` | abstractTagID | error | **刷新label/desc/emb** |
| `MergeTags` | `topicanalysis/embedding.go` | sourceID, targetID | error | **合并标签** |
| `adoptNarrowerAbstractChildren` | `topicanalysis/abstract_tag_hierarchy.go` | abstractTagID | error | **批量收养更窄标签** |
| `batchJudgeNarrowerConcepts` | `topicanalysis/abstract_tag_judgment.go` | parentTag, candidates | []uint | **批量LLM判断更窄** |
| `batchJudgeAbstractRelationships` | `topicanalysis/abstract_tag_judgment.go` | tagA, candidates | []batchAbstractRelationResult | **批量LLM判断抽象关系** |
| `EnqueueAdoptNarrower` | `topicanalysis/adopt_narrower_queue.go` | abstractTagID, source | - | **入队收养任务** |
| `ProcessPendingAdoptNarrowerTasks` | `topicanalysis/queue_batch_processor.go` | - | int | **批量处理pending收养任务** |
| `ProcessPendingAbstractTagUpdateTasks` | `topicanalysis/queue_batch_processor.go` | - | int | **批量处理pending抽象标签刷新** |
| `BackfillMissingDescriptions` | `topicextraction/description_backfill.go` | - | int | **批量补全缺失description** |
| `formatTagPromptContext` | `topicanalysis/tag_prompt_context.go` | TopicTag | string | **统一生成LLM标签上下文，人物包含结构化属性** |

---

## 12. 标签清理机制

### 实时清理：`cleanupOrphanedTags`

文章重新打标签后，旧标签若不再被任何 `article_topic_tags` 引用，直接删除（含 embedding）。

### 深度限制：`maxHierarchyDepth`

深度限制统一使用常量 `maxHierarchyDepth = 4`（4条边，即5个节点）。所有创建抽象父子关系的路径都必须调用 `checkDepthLimit(tx, parentID, childID)` 检查，该函数计算 `getAbstractSubtreeDepth(child) + getTagDepthFromRoot(parent) + 1` 是否超过限制。

涉及的代码路径：
- `linkAbstractParentChild` — `abstract_tag_hierarchy.go`
- `reparentOrLinkAbstractChild` — `abstract_tag_judgment.go`
- `processAbstractJudgment` — `abstract_tag_service.go`
- `validateTreeReviewMove` / `executeTreeReviewMove` — `hierarchy_cleanup.go`
- `validateAndCreateReviewAbstract` — `hierarchy_cleanup.go`
- `createAbstractTagDirectly` — `hierarchy_cleanup.go`

### 数据清理工具：`cmd/fix-hierarchy-depth`

用于修复历史超限数据。从每个标签出发沿 parent 方向 DFS，如果任何路径超过4条边，断开该标签的所有父关系使其成为根节点。

```bash
cd backend-go
go run ./cmd/fix-hierarchy-depth --dry-run        # 预览
go run ./cmd/fix-hierarchy-depth --dry-run=false   # 执行
```

### 一次性清理工具：`cmd/cleanup-tags`

用于标签碎片化的一次性清理，独立于定时调度器运行：

```bash
cd backend-go
# 默认 dry-run 模式
go run ./cmd/cleanup-tags
# 实际执行
DRY_RUN=false go run ./cmd/cleanup-tags
```

Phase A：清理所有无文章关联的零文章标签（跨 event/keyword/person 三类）。
Phase B：清理 `quality_score < 0.15` 且仅关联 1 篇文章的 keyword 标签。

### 定时调度：`TagHierarchyCleanupScheduler`（10 阶段）

```mermaid
flowchart TD
    START([定时/手动触发]) --> P1[Phase 1: Zombie清理]
    P1 --> |无LLM 标记inactive| P1_5[Phase 1.5: 零文章标签清理]
    P1_5 --> P1_6[Phase 1.6: 低质量单文章keyword清理<br/>quality_score < 0.15]
    P1_6 --> P1_7[Phase 1.7: 过期零分标签清理<br/>quality_score < 0.05 & age > 7d]
    P1_7 --> P2[Phase 2: Flat Merge]
    P2 --> |LLM判断 同类抽象标签合并 event/keyword各≤50| P3[Phase 3: 层级修剪]
    P3 --> P3A[删除孤儿关系]
    P3A --> P3B[解决多父冲突]
    P3B --> P3C[清理空抽象节点]
    P3C --> P4[Phase 4: 收养更窄标签]
    P4 --> |批量处理pending队列| P5[Phase 5: 抽象标签刷新]
    P5 --> |批量处理pending队列<br/>LLM重生成label+desc| P6[Phase 6: 整树审查]
    P6 --> P6A{含14天内新关系?}
    P6A -->|否| P7[Phase 7: Description回填]
    P6A -->|是| P6B[构建 minDepth=2 标签树]
    P6B --> P6C{节点数>50?}
    P6C -->|是| P6D[root+直接子节点审查<br/>并递归拆分子树]
    P6C -->|否| P6E[整树序列化审查]
    P6D --> P6F[LLM返回 merges/moves/new_abstracts]
    P6E --> P6F
    P6F --> P6G[校验环/深度/active/目标存在]
    P6G --> P6H[安全迁移或复用/创建分组]
    P6H --> P7
    P7 --> |批量补全缺失description| END1([完成])

    style P1 fill:#fff3e0
    style P1_5 fill:#fff3e0
    style P1_6 fill:#fff3e0
    style P1_7 fill:#fff3e0
    style P2 fill:#e3f2fd
    style P3 fill:#f3e5f5
    style P4 fill:#e8eaf6
    style P5 fill:#e8eaf6
    style P6 fill:#e8f5e9
    style P7 fill:#fff8e1
```

| 阶段 | 文件 | LLM? | 条件 |
|------|------|-------|------|
| Phase 1 Zombie | `tag_cleanup.go` `CleanupZombieTags` | 否 | age>7d + 无关系 + 无文章/摘要引用 |
| Phase 1.5 零文章清理 | `tag_cleanup.go` `CleanupZeroArticleTags` | 否 | 无 article_topic_tags 引用 |
| Phase 1.6 低质量keyword清理 | `tag_cleanup.go` `CleanupLowQualitySingleArticleTags` | 否 | keyword 单文章 + quality_score < 0.15 |
| Phase 1.7 过期零分清理 | `tag_cleanup.go` `CleanupStaleZeroScoreTags` | 否 | quality_score < 0.05 + age > 7 天 |
| Phase 2 Flat Merge | `tag_cleanup.go` `ExecuteFlatMerge` | 是 | 同类 abstract 标签去重 |
| Phase 3 层级修剪 | `tag_cleanup.go` 三个函数 | 否 | 孤儿关系 / 多父 / 空抽象 |
| Phase 4 收养更窄标签 | `queue_batch_processor.go` `ProcessPendingAdoptNarrowerTasks` | 是 | 处理 pending 的 adopt_narrower 队列任务 |
| Phase 5 抽象标签刷新 | `queue_batch_processor.go` `ProcessPendingAbstractTagUpdateTasks` | 是 | 处理 pending 的 abstract_tag_update 队列任务 |
| Phase 6 整树审查 | `hierarchy_cleanup.go` `ReviewHierarchyTrees` | 是 | 含 windowDays 内新关系的 minDepth=2 标签树 |
| Phase 7 Description回填 | `description_backfill.go` `BackfillMissingDescriptions` | 是 | active 标签中 description 为空的，从关联文章获取上下文生成 |

核心文件: `jobs/tag_hierarchy_cleanup.go`（调度器）、`topicanalysis/tag_cleanup.go`（Phase 1-1.7-3）、`topicanalysis/queue_batch_processor.go`（Phase 4-5）、`topicanalysis/hierarchy_cleanup.go`（Phase 6）、`topicextraction/description_backfill.go`（Phase 7）

Phase 4-5 在整树审查前执行：先收养更窄标签挂到对应抽象节点下，再刷新抽象标签的 label/description，确保审查时层级结构已经是最新的。

Phase 6 不再只做"深度压缩"。它会把 `event`、`keyword`、`person` 三类抽象树序列化给 LLM 审查子标签归属是否合理。LLM 可同时返回三种操作：

- `merges`：合并树中语义重复的抽象标签（source 合并进 target），消除因多父关系导致的同一标签在树中重复出现。树的根节点不允许作为 merge source。
- `moves`：把标签迁移到树中已有父节点，或 `to_parent=0` 让其脱离为根节点。树的顶级根节点不允许被 move 为其他节点的子节点。
- `new_abstracts`：建议新的分组；系统先复用已有相似抽象或 slug 命中抽象，找不到才创建新抽象。

执行顺序：先 merges（消除重复节点）→ 再 moves（调整归属）→ 最后 new_abstracts（新建分组）。

执行 merge/move 前会校验标签存在、active、目标在当前审查树中、不会成环、深度不超过 4。非 detach 迁移在事务中先创建新父关系，再删除旧父关系；失败时保留旧关系，避免产生孤儿标签。大树会先审查 `root + 直接子节点`，再递归审查子树，防止第一层错挂漏审。

此外，根抽象标签的 label 受保护：`refreshAbstractTag` 不会修改根节点的 label 和 slug，仅允许更新 description。

Phase 7 批量查询 active 标签中 description 为空的记录，通过关联的 `article_topic_tags` 获取最近 3 篇文章标题作为上下文，调用 LLM 生成 description。每次最多处理 50 个标签，间隔 500ms。

---

## 总结

```
新标签 → Embedding三级匹配 → 有候选则LLM判断(merges/abstracts/none三个必填数组)
                                     ↓
                          event标签额外走co-tag扩展补充候选
                                     ↓
               LLM输出三个数组: merges(同概念merge) + abstracts(相关但不同) + none(无关/独立)
                                     ↓
               merge: processJudgment验证目标相似度≥0.85 → 合并
               abstract: 查重 → 子标签数保护 → 事务落库
               none: 未放入任何数组的候选自动补入none
                                     ↓
               异步: 生成embedding → 层级匹配 → EnqueueAdoptNarrower入队 → 入队刷新
                                     ↓
                定时清理(10阶段): zombie → 零文章清理 → 低质量keyword清理 → 过期零分清理 → flat merge → 层级修剪 → 收养窄标签 → 抽象刷新 → 整树审查 → description回填
                                     ↓
               收养队列: 5s轮询 + Phase4批量处理 → batchJudgeNarrowerConcepts批量LLM → 收养更窄标签
                                     ↓
               层级匹配: 跨层去重 + 深度限制 → AI判断父子
                                     ↓
               链接成功: 解决多父冲突 → embedding动态管理 → 父标签入队刷新
```
