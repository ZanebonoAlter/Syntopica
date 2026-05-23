# PRD: 标签层级结构清理与去重

Status: ready-for-agent

## Problem Statement

标签层级模板系统（tag-hierarchy-templates）已于 2026-05-10 部署上线，但运行到 5/13 时暴露出严重的层级质量问题：

1. **旧 abstract 标签爆炸**：1725 个 abstract 标签中仅 160 个 active，891 个被 merge，674 个 inactive。仅"美伊"相关就有 231 个 abstract 标签，其中 227 个是孤儿（无子节点）。
2. **三个标签创建来源互相打架**：`findOrCreateTag`（实时）、`PlaceTagInHierarchy`（异步）、`ReviewHierarchyTrees`（定时调度）三个路径独立创建 abstract 标签和层级关系，缺乏协调，导致重复创建和语义碎片化。
3. **PlaceTagInHierarchy 覆盖率极低**：5/13 新建的 433 个 LLM 标签中，仅 26 个（6%）进入了层级结构。双重根因：(a) 新标签 embedding 异步生成，PlaceTag 执行时 `FindSimilarAbstractTags` 因 embedding 缺失报错（竞态条件）；(b) `placeTagAtL1ForParent` 的早退条件 `currentLevel >= 1` 对新创建的 abstract 恒为 true（depth=0 → level=1），L1 放置逻辑是**死代码**，从未执行过。6% 覆盖率全靠 L2 放置，L1 覆盖率为 0%。
4. **深度违规严重**：event 类 50.6% 路径超过模板限制（3 层），keyword 48.3%，person 76.2%。Person 甚至出现三节点环形引用（国际知名人物 ↔ 美国其他公众人物 ↔ 美国政治人物）。根因之一是 `maxHierarchyDepth=4` 为全局常量，不尊重 per-template 的 MaxLevel（person 应为 2）。
5. **hierarchy_pending_changes 从未使用**：`CleanupTemplateViolations` 只在 PUT config 时触发检测，从不主动修复，且该表为空。
6. **ai_call_logs 缺失层级放置记录**：`callLLMForL2Match`、`callLLMForL1Match` 等函数的 LLM 调用未被记录到 ai_call_logs，无法追踪层级放置的 LLM 开销。
7. **层级构建是一次性全路径设计**：`PlaceTagInHierarchy` 试图一次构建 L2+L1 全路径，但每层向上聚合都需要 embedding，新创建的 abstract 的 embedding 又是异步的，导致级联失败。
8. **放置逻辑硬编码 L1/L2，不支持 N 层模板**：`placeTagUpward` 用 switch/case 只处理 level 1 和 2，level ≥ 3 直接报错。prompt 函数（`buildL2MatchPrompt`、`buildL1CreationPrompt` 等）硬编码层级名，模板数据结构 `CategoryHierarchyTemplate.MaxLevel` 支持任意层数但代码不支持。
9. **Level 概念与 depth 语义混乱**：`GetTagLevel(tag) = depth + 1` 在代码中同时表示"树位置"和"模板 Level"，与模板 Levels[] 数组索引方向的关系容易混淆。`depth=0` 同时表示"根节点"和"未放置标签"，无法区分。
10. **根节点无限扩展**：abstract 根节点（depth 最高的 abstract）创建没有边界约束，每个新标签独立决定创建 abstract → 根节点自然膨胀。5/13 一天新增 14 个 event 根，现有 dedup 机制（dedupL1 两两比对、reviewOneTree 单树审查）效果差且慢，无法有效合并语义重叠的根节点。核心矛盾：根节点是树的"天花板"，天花板上没有更高层级约束它，唯一约束只能是同层横向去重，但现有去重是"创建后补救"模式，根节点已积累了子标签，merge 代价大。

## Solution

分两阶段修复，核心架构原则：**逐层聚合 + depth 驱动 + N 层泛化 + concept 围栏**。

**Design C：废弃 Level 概念，全用 depth**
- 代码内部只用 `depth`（`getTagDepthFromRoot` 返回的 BFS 向上跳数，0=无parent，越大越靠近根）
- 模板定义通过 `tmpl.Levels[depth]` 直接索引，无需方向转换
- `GetTagLevel` / `GetTagLevelByID` / `ResolveLevelFromDepth` 全部删除
- "Level" 仅出现在 prompt 文本和 UI 展示中

**Design D：board_concept 作为"围栏"约束 abstract 创建**
- abstract 根节点创建限定在 board_concept 边界内，防止根节点无限膨胀
- board_concept 三种创建路径：自动聚类生成（待确认）、用户手动创建（直接生效）、叙事生成发现新概念（待确认）
- 复用现有 `board_concepts` 表和 `narrative` 模块，概念不是 abstract 节点本身，而是抽象的分区标记
- 放置流程：新标签 → MatchTagToConcept → 在 concept 边界内找/创建 abstract
- 去重和聚合操作限定在同 concept 内，跨 concept 不创建 abstract
- 回收机制清理空心 abstract（≤1 子标签 + 0 文章引用）

**阶段一（止血）**：
- 重写放置逻辑为通用 `placeTagAtLevel(child, tmpl, targetDepth)`，支持任意层数模板
- PlaceTagInHierarchy 精简为只放一层（紧邻上层），embedding 未就绪时返回 pending
- 新增 concept 约束层：放置前先 MatchTagToConcept，限定 abstract 创建范围
- 新增 concept bootstrap 流程：标签积累到阈值后自动聚类生成待确认 concept
- 新增空心 abstract 回收机制
- 全局 `maxHierarchyDepth=4` 替换为 per-template `MaxLevel - 1` 作为 depth 上界
- 关停来源 C 的 abstract 创建能力（保留复用已有 abstract 的路径）

**阶段二（治理）**：增强 Phase 3d/Phase 4 的模板合规检查，启用 `hierarchy_pending_changes` 的自动修复流程，补充 ai_call_logs 的层级操作记录，建立持续的数据质量监控。

## User Stories

1. 作为系统管理员，我希望新文章的标签能自动进入正确的层级位置（覆盖率达到 80%+），而不是只有 6%
2. 作为系统管理员，我希望同一个语义领域不会出现大量重复的 abstract 标签（如"美伊"不应有 231 个 abstract）
3. 作为系统管理员，我希望所有 event 标签树不超过 3 层、keyword 不超过 3 层、person 不超过 2 层，由系统自动维护
4. 作为系统管理员，我希望 Person 标签不出现环形引用
5. 作为系统管理员，我希望层级放置的 LLM 调用被记录到 ai_call_logs，以便追踪成本和调试问题
6. 作为系统管理员，我希望旧的垃圾 abstract 标签（无子节点、无文章引用）被自动清理
7. 作为系统管理员，我希望 `PlaceTagInHierarchy` 在新标签 embedding 就绪后执行 L2 放置，embedding 未就绪时由调度器重试
8. 作为系统管理员，我希望 Phase 4 adopt_narrower 收养标签时检查目标层级的模板合规性
9. 作为系统管理员，我希望 `CleanupTemplateViolations` 能自动修复深度违规（断开超深关系并重新放置），而不是只检测
10. 作为系统管理员，我希望 Phase 6 的树审查不再创建新的 abstract 标签，只做去重、移动和复用已有 abstract
11. 作为系统管理员，我希望前端层级配置页面的"待处理"列表有实际数据可显示
12. 作为系统管理员，我希望 rebuild 端点能修复深度违规和跨分类关系
13. 作为系统管理员，我希望看到每个 LLM 操作（l2_match、l1_match、l2_create、l1_create、l1_dedup、l1_aggregate）的调用次数、耗时、成功率
14. 作为系统管理员，我希望旧 Phase 6 的 `reviewOneTree` 不再通过 `createAbstractTagDirectly` 创建新 abstract，保留 merge/move 和复用已有 abstract 的能力
15. 作为系统管理员，我希望清理后标签层级树的平均深度回到模板限制内
16. 作为系统管理员，我希望新标签在 1 小时内完成 L2 和 L1 的层级放置（而不是 24 小时等调度器跑一轮）
17. 作为系统管理员，我希望 L1 聚合能利用 L2 子标签的信息做出更准确的分类判断
18. 作为系统管理员，我希望 abstract 根节点不会无限膨胀，有自然的数量约束
19. 作为系统管理员，我希望系统能根据标签内容自动生成"板块"概念建议，由我确认后生效
20. 作为系统管理员，我希望可以手动创建和管理板块概念，用来约束标签层级的结构
21. 作为系统管理员，我希望空心 abstract（≤1 子标签 + 0 文章引用）被自动回收

## Implementation Decisions

### 决策 0: 架构原则——逐层聚合 + depth 驱动 + N 层泛化

**逐层聚合**（保持不变）：
- 紧邻上层立即放置：新标签 embedding 就绪后，立即找/创建紧邻上层 abstract parent
- 更上层延后聚合：abstract 积累 N 个子标签后，系统根据子标签共性向上聚合
- 触发机制：每小时定时 + 子标签数 ≥ N 时立即触发

**depth 驱动**（Design C）：
- 废弃 `GetTagLevel` / `GetTagLevelByID` / `ResolveLevelFromDepth`，代码内部只用 `depth`
- `depth` = `getTagDepthFromRoot(tagID)` 返回的 BFS 向上跳数
  - depth=0：没有 abstract parent（可能是根，也可能是未放置标签）
  - depth 越大越靠近根
  - 合法范围：0 ≤ depth ≤ `tmpl.MaxLevel - 1`
- 模板映射：`tmpl.Levels[depth]` 直接索引该 depth 位置的模板定义
  - depth=0 → Levels[0] = Level 1（根/最抽象）
  - depth=MaxLevel-1 → Levels[MaxLevel-1] = Level MaxLevel（叶/最具体）
- 区分根 vs 未放置：`source == "abstract" && 有子标签` → 根，否则 → 未放置

**N 层泛化**：
- 所有放置/聚合/去重函数用 `depth` 参数化，不硬编码层级名
- 通用函数替代 L1/L2 专用函数：
  - `placeTagAtLevel(child, tmpl, targetDepth)` 替代 `placeTagAtL2` + `placeTagAtL1ForParent`
  - `resolveParent(child, candidates, existing, tmpl, levelDef)` 替代 `resolveL2Parent` + `resolveL1Parent`
  - `createAbstractAtLevel(child, tmpl, targetDepth, levelDef)` 替代 `createL2TagForChild` + `createL1ForL2Tag`
  - `dedupAtDepth(tag, depth)` 替代 `dedupL2` + `dedupL1`
- Prompt 参数化：`buildMatchPrompt(child, candidates, tmpl, levelDef)` 和 `buildCreationPrompt(child, tmpl, levelDef)` 替代 4 个硬编码 prompt

### 决策 1: 关停来源 C 的 abstract 创建能力（保留复用）

修改 `ReviewHierarchyTrees` → `reviewOneTree` 的流程：

1. `judgment.Merges` → 保持不变
2. `judgment.Moves` → 保持不变
3. `judgment.NewAbstracts` → `validateAndCreateReviewAbstract` 中：
   - **保留**"复用已有 abstract"路径（`findSimilarExistingAbstractFn` 命中 → `attachChildrenToReviewAbstract`）
   - **关停**"创建新 abstract"路径（不再调用 `createAbstractTagDirectly`）

`buildTreeReviewPrompt` 的 JSON schema 和 prompt 中保留 `new_abstracts` 字段（LLM 建议仍有参考价值），但处理时只执行复用，忽略需要新建的建议。

### 决策 2: PlaceTagInHierarchy 重写——通用 depth-based 放置

**删除的函数**（硬编码 L1/L2）：
- `placeTagUpward`（switch/case 只处理 1 和 2）
- `placeTagAtL2`、`placeTagAtL1`、`placeTagAtL1ForParent`
- `resolveL2Parent`、`resolveL1Parent`
- `createL2TagForChild`、`createL1ForL2Tag`
- `isL1Tag`、`loadExistingL1Tags`、`filterL2Candidates`、`filterL1Candidates`
- `GetTagLevel`、`GetTagLevelByID`、`ResolveLevelFromDepth`

**新增的通用函数**：

`PlaceTagInHierarchy(tag)` 入口重写：
```
1. tmpl = GetTemplate(tag.Category)
2. embedding 就绪检查（查询 topic_tag_embeddings）
   未就绪 → return {Action: "pending_embedding"}
3. depth = getTagDepthFromRoot(tag.ID)
   maxDepth = tmpl.MaxLevel - 1
4. if tag.Source == "abstract" && depth >= maxDepth:
     return {Action: "already_at_root"}
   if tag.Source != "abstract" && depth > 0:
     return {Action: "already_placed"}
5. targetDepth = depth + 1  // 向上放一层
6. result = placeTagAtLevel(tag, tmpl, targetDepth)
7. if result.newParent && targetDepth < maxDepth:
     if countChildren(result.parentID) >= N:
       go aggregateToUpperLevel(result.parentID, tmpl, targetDepth)
```

`placeTagAtLevel(child, tmpl, targetDepth, concept)` 核心逻辑（含 anchor 路径 + concept 约束）：
```
1. levelDef = tmpl.Levels[targetDepth]

// ── 第一层：anchor 投票（标签 vs 已放置标签）──
2. anchors = findAnchors(child)  // cotag + embedding 中已有 parent 的标签
3. validAnchors = anchors.filter(a =>
     a.parent.status == "active" &&
     a.parent belongsTo concept &&
     a.parentDepth == targetDepth)
4. if validAnchors 不为空:
     best = validAnchors.topBySimilarity()
     if best.sim >= AnchorHighThreshold:     // 默认 0.85，可配置
       return best.parent  // 高置信跟随 anchor
     if best.sim >= AnchorLowThreshold:       // 默认 0.70，可配置
       top3 = validAnchors.top(3)
       parent = llmAnchorVote(child, top3)    // LLM 看多个 anchor 的 parent 投票
       if parent != nil: return parent

// ── 第二层：abstract embedding 匹配（原有逻辑）──
5. candidates = FindSimilarAbstractTags(child.ID, child.Category)
6. filtered = filterByDepth(candidates, targetDepth)
7. filtered = filterByConcept(filtered, concept)
8. existing = loadTagsAtDepth(tmpl, targetDepth, concept)
9. parent = resolveParent(child, filtered, existing, tmpl, levelDef)  // 3 档阈值
10. if parent == nil:
      parent = createAbstractAtLevel(child, tmpl, targetDepth, levelDef)
      linkAbstractToConcept(parent.ID, concept.ID)
11. CreateRelation(parent, child)
12. go dedupAtDepth(parent, targetDepth, concept)
```

**anchor 语义**：已放置标签（有 parent）作为"路标"，告诉新标签该去哪。已放置标签永远不动，只读其 parent 信息。标签≈标签的相似度天然比标签≈abstract 高 0.05-0.10，因此 anchor 阈值需高于对应 abstract 阈值。

`aggregateToUpperLevel(tagID, tmpl, currentDepth)` 向上聚合：
```
if currentDepth >= tmpl.MaxLevel - 1: return  // 已到根
placeTagAtLevel(LoadTag(tagID), tmpl, currentDepth + 1)
```

`AggregateOrphanTags()` 调度器批量聚合（替代 AggregateL1Tags）：
```
for each tmpl in AllTemplates():
  maxDepth = tmpl.MaxLevel - 1
  for depth = 1 to maxDepth - 1:    // 从叶向根方向
    orphans = findOrphansAtDepth(tmpl, depth)
    // orphan: source=abstract, depth 匹配, 有子标签但自身无 parent
    for batch in orphans.chunk(5):
      aggregateBatch(batch, tmpl, depth)
```

### 决策 3: 全局 maxHierarchyDepth 替换为 per-template depth 上界

当前 `abstract_tag_hierarchy.go` 中 `const maxHierarchyDepth = 4`，所有深度检查都用这个全局常量。

改为：新增函数 `getMaxDepthForCategory(category string) int`，返回 `GetHierarchyManager().GetTemplate(category, "").MaxLevel - 1`。所有深度检查调用此函数，将 `maxHierarchyDepth` 替换为 `getMaxDepthForCategory`。影响约 18 处引用。

模板定义（depth 上界 = MaxLevel - 1）：
- event: maxDepth=2 (MaxLevel=3)
- person: maxDepth=1 (MaxLevel=2)
- keyword/technology: maxDepth=2 (MaxLevel=3)
- keyword/company_business: maxDepth=2 (MaxLevel=3)
- keyword/concept: maxDepth=2 (MaxLevel=3)

同时删除 `GetTagLevel` / `GetTagLevelByID` / `ResolveLevelFromDepth`，所有调用点改用 `getTagDepthFromRoot(tagID)` + `tmpl.Levels[depth]`。

受影响的 level 消费点：
- `abstract_tag_judgment.go:1140` adopt_narrower cross-level 检查 → 改为 `getTagDepthFromRoot` 比较
- `tag_cleanup.go:811` CleanupTemplateViolations 深度检查 → 改为 `depth >= tmpl.MaxLevel`
- `cmd/backfill-tag-levels/main.go` → 改为存 `hierarchy_depth` 而非 `hierarchy_level`

### 决策 4: Phase 4 adopt_narrower 增加 per-template 深度检查

`reparentOrLinkAbstractChild`（`abstract_tag_judgment.go:1101`）当前已有 cross-level 检查和 `maxHierarchyDepth` 深度检查。修改为：
1. 用 `getMaxDepthForCategory` 替换 `maxHierarchyDepth`
2. cross-level 检查从 `GetTagLevel` 改为 `getTagDepthFromRoot` 比较
3. 检查被收养标签和目标标签是否属于同一 category（已有 cross-level 检查近似，但应明确检查 category 字段）
4. 环形引用检查保留

### 决策 5: CleanupTemplateViolations 自动修复

将 Phase 3d 从"只检测"改为"检测 + 自动修复"：
1. 深度超限的关系：断开最深的父子关系，子标签标记为待重新放置
2. 跨分类的关系：直接断开，子标签标记为待重新放置
3. 结果写入 `hierarchy_pending_changes`，标记为 `auto_resolved`

### 决策 6: 补充 ai_call_logs 的层级操作记录 + prompt 泛化

**ai_call_logs**：在 `hierarchy_prompts.go` 的通用 LLM 调用函数（`callLLMForMatch`、`callLLMForCreation`）和 `hierarchy_dedup.go` 的 `callLLMForDedup` 中，确认 `Metadata` 字段包含 `operation` 和 `target_depth` 键。

问题根因可能是 `airouter.Router.Chat` 内部的日志记录逻辑没有从 `Metadata` 中提取 `operation` 写入 `ai_call_logs.request_meta`。需要在 airouter 的日志中间件或调用后补写逻辑中修复。

**Prompt 泛化**（替代 4 个硬编码 prompt 函数）：

删除：
- `buildL2MatchPrompt`、`buildL1MatchPrompt`、`buildL2CreationPrompt`、`buildL1CreationPrompt`
- `callLLMForL2Match`、`callLLMForL1Match`、`callLLMForL2Creation`、`callLLMForL1Creation`
- `l2MatchResponse`、`l1MatchResponse`

新增：
- `buildMatchPrompt(child, candidates, existingTags, tmpl, levelDef)` — 通用匹配 prompt
  - 从 `levelDef`（= `tmpl.Levels[targetDepth]`）取层级名称和描述
  - 通用 3 档阈值逻辑不变
- `buildCreationPrompt(child, tmpl, levelDef)` — 通用创建 prompt
  - 从 `levelDef` 取目标层级的名称、描述、约束
- `callLLMForMatch(...)` — 通用匹配 LLM 调用，operation = `"match_depth_{d}"`
- `callLLMForCreation(...)` — 通用创建 LLM 调用，operation = `"create_depth_{d}"`
- `matchResponse` — 统一响应结构（select / select_existing / create_new）

### 决策 7: anchor 阈值可配置

新增 anchor 相关阈值到现有 global settings 配置体系（`EmbeddingMatchThresholds` + `GET/PUT /api/hierarchy/config`）：

**新增配置项**：
```
AnchorHighThreshold  = 0.85  // anchor 高置信直跟随，无需 LLM
AnchorLowThreshold   = 0.70  // anchor 中间带，LLM 投票判断
```

**阈值对照表**（信号类型不同，阈值不可直接比较）：
```
阈值   用途                    信号类型           LLM
0.97   Source A 直接复用       标签≈标签          否
0.95   dedup L2 自动合并       abstract≈abstract  否
0.90   dedup L1 + LLM          abstract≈abstract  是
0.85   anchor 高置信跟随       标签≈标签          否  ← 新增
0.85   Placement L2 直挂       标签≈abstract      否
0.80   Placement L1 直挂       标签≈abstract      否
0.70   anchor 低置信截止       标签≈标签          —   ← 新增
0.60   Placement L2 低置信     标签≈abstract      —
0.55   Placement L1 低置信     标签≈abstract      —
```

**约束**：anchor 阈值需高于对应 abstract 匹配阈值，因为标签≈标签的相似度天然比标签≈abstract 高 0.05-0.10。

**anchor 有效性检查**：anchor 的 parent 必须 status=active、属于同一 concept、parentDepth=targetDepth，任一不满足则该 anchor 无效。

**多 anchor 冲突**：取相似度最高的 top-1 anchor；若 < AnchorHighThreshold 且有多个 anchor，取 top-3 的 parent 列表给 LLM 投票。

### 决策 8: 清理旧数据——一次性脚本

创建一个新的迁移/脚本（类似现有的 `cmd/backfill-tag-levels`），执行：
1. 断开 Person 三节点环（国际知名人物 / 美国其他公众人物 / 美国政治人物之间的互相引用）
2. 清理所有深度超过模板限制的关系链
3. 将断开关系的子标签标记为待重新放置
4. 删除没有子节点且没有文章引用的 active abstract 标签（标记为 inactive）

### 决策 9: 新增 AggregateOrphanTags 函数（泛化版）

新增 `hierarchy_aggregation.go` 文件，包含通用批量聚合逻辑（替代原 AggregateL1Tags）：

**触发时机**：
- 调度器每小时运行一次（Phase 3.8）
- abstract 子标签数达到 N 时立即触发（`aggregateToUpperLevel`）

**流程**：
1. 遍历所有模板
2. 对每个模板，从 depth=1 到 maxDepth-1 逐层查找孤儿 abstract（有子标签但自身无 parent）
3. 按 depth 分组，每批处理 batchSize=5
4. 对每批孤儿 abstract：
   a. 用 `EmbeddingTypeSemantic` 搜索目标 depth 的候选（粗筛 top 10）
   b. 收集每个 abstract 的子标签列表作为上下文
   c. LLM 批量判断：每个 abstract 应挂在哪个已有 abstract 下，或是否需要创建新 abstract
   d. 调用 `placeTagAtLevel` 执行放置

**LLM prompt 策略**（通用化）：
- 输入：abstract 标签名 + description + 子标签列表 + 目标层 `levelDef` + 已有候选列表
- 输出：每个 abstract 的 `{action: "attach_existing" | "create_new", target_id | new_name/new_description, reason}`
- 关键规则：如果多个 abstract 适合同一个新 parent，合并为一个 `create_new`

**不新增 embedding 类型**：用已有的 IdentityEmbedding 做实体匹配，SemanticEmbedding 做粗筛 + LLM 做精判。

### 决策 10: 新增 RetryOrphanPlacements 函数

在 `hierarchy_placement.go` 中新增：

```
RetryOrphanPlacements():
  查找所有:
    - status = 'active'
    - source != 'abstract'
    - NOT EXISTS (abstract parent relation)
    - created_at < NOW() - 10min（给 embedding 足够时间）
  对每个孤儿标签:
    PlaceTagInHierarchy(tag)  // 此时 embedding 基本必然就绪
```

不需要新增队列表——标签创建 >10 分钟后，embedding worker 必然已完成。直接查"没有 parent 的叶标签"即可。

### 决策 11: 拆分调度器

将当前的 `TagHierarchyCleanupScheduler`（24h）拆为两个：

**调度器 1: TagHierarchyPlacementScheduler（新增）**
- 间隔：1 小时
- 职责：保证新标签进入层级结构
- 运行阶段：
  - Phase 3.7: `RetryOrphanPlacements`（重试失败的放置）
  - Phase 3.8: `AggregateOrphanTags`（通用批量向上聚合）
- 新增 scheduler struct + 配置 + 启动/停止逻辑
- 在 `runtime.go` 中注册

**调度器 2: TagHierarchyCleanupScheduler（已有）**
- 间隔：24 小时（不变）
- 运行阶段：
  - Phase 1-3: 数据清理（zombie, orphan relations, degenerate trees 等）
  - Phase 3d: `CleanupTemplateViolations`（自动修复）
  - Phase 4: `ProcessPendingAdoptNarrowerTasks`（加 per-template 检查）
  - Phase 5: `ProcessPendingAbstractTagUpdateTasks`
  - Phase 6: `ReviewHierarchyTrees`（砍掉 new_abstracts 创建，保留复用）
  - Phase 7: `BackfillMissingDescriptions`

### 决策 12: abstract 子标签阈值 N=3 立即触发上层聚合

在 `PlaceTagInHierarchy` 成功放置后（通用 `placeTagAtLevel`）：

```
result = placeTagAtLevel(tag, tmpl, targetDepth)
if result.newParent:
  parentDepth = targetDepth
  if parentDepth < tmpl.MaxLevel - 1:      // 还没到根
    if countChildren(result.parentID) >= 3:
      go aggregateToUpperLevel(result.parentID, tmpl, parentDepth)
```

N=3 的理由：
- N=1/2：信息量不够，回到一次性全路径的问题
- N=3：至少来自 2-3 篇不同文章，信号足够
- N=5：太保守，热门话题可能长时间达不到

### 决策 13: board_concept 作为"围栏"约束 abstract 创建

**问题根因**：abstract 根节点创建没有边界约束，每个新标签独立决定创建 abstract → 根节点自然膨胀。现有 dedup 机制（两两 embedding 比对 + LLM、单树 reviewOneTree）是"创建后补救"模式，效果差且慢。

**方案**：复用现有 `board_concepts` 表，赋予它"围栏"角色。abstract 创建限定在 concept 边界内，concept 天然限制根节点数量。

**concept 不是 abstract 节点**：concept 是抽象的分区标记，不出现在标签树中。一个 concept 下可以有多个 abstract 根节点。concept 粒度比 abstract 根节点粗（4-8 个 concept vs 可能 10-30 个 abstract 根）。

**concept 生命周期**：
1. **创建路径**：
   - 路径 1：自动聚类生成 — 标签 embedding 积累到阈值（category ≥ 20 标签）后，对同 category 标签做聚类，LLM 命名每个 cluster → 生成 status=`pending` 的 concept
   - 路径 2：用户手动创建 — 用户在 UI 中填写 concept 名称+描述 → status=`active` 直接生效
   - 路径 3：叙事生成发现 — `GenerateAndSaveForCategory` 中匹配不到已有 concept 的 abstract tree → 建议新 concept → status=`pending`
2. **确认机制**：自动生成的 concept 需用户确认后变为 `active`；用户手写的直接 `active`
3. **状态流转**：`pending` → `active` → `inactive`（用户停用）；可被合并到其他 concept（`merged`）

**放置流程变更**（在决策 2 基础上增加 concept 约束）：

```
PlaceTagInHierarchy(tag) 新增 concept 约束：
1. tmpl = GetTemplate(tag.Category)
2. embedding 就绪检查
3. concept = MatchTagToConcept(tag)  // 新增：匹配 board_concept
   - 匹配到 concept C → 继续
   - 无匹配且 active concept 数量 < 上限 → 创建 pending concept，标签暂存
   - 无匹配且 active concept 数量 ≥ 上限 → 分配到最接近的 concept
4. depth = getTagDepthFromRoot(tag.ID)
5. targetDepth = depth + 1
6. result = placeTagAtLevel(tag, tmpl, targetDepth, concept)  // 传入 concept 约束
```

```
placeTagAtLevel(child, tmpl, targetDepth, concept) 变更：
1. levelDef = tmpl.Levels[targetDepth]
2. candidates = FindSimilarAbstractTags(child.ID, child.Category)
3. filtered = filterByDepth(candidates, targetDepth)
4. filtered = filterByConcept(filtered, concept)       // 新增：只保留同 concept 内的候选
5. existing = loadTagsAtDepth(tmpl, targetDepth, concept) // 新增：加载同 concept 内的已有标签
6. parent = resolveParent(child, filtered, existing, tmpl, levelDef)
7. if parent == nil:
     parent = createAbstractAtLevel(child, tmpl, targetDepth, levelDef)
     linkAbstractToConcept(parent.ID, concept.ID)        // 新增：关联 abstract 到 concept
8. CreateRelation(parent, child)
9. go dedupAtDepth(parent, targetDepth, concept)          // concept-aware 去重
```

**concept 与 abstract 的关联**：
- 新增关联表或字段：abstract tag ↔ board_concept（多对一：多个 abstract 根节点属于同一个 concept）
- 已有 abstract 不强制要求关联 concept（向后兼容），但新创建的必须关联
- concept 内的 abstract 数量无硬上限，但 concept 数量有自然上限（由聚类算法和用户管理决定）

**冷启动 bootstrap**：
- 系统初始状态：0 标签、0 abstract、0 concept
- T1：文章进来 → 标签提取 → embedding 生成
- T2：标签积累到阈值 → 触发聚类 → 生成 pending concept → 用户确认
- T3：concept active 后 → 后续新标签可正常放置
- 在 concept 确认前：新标签只做高置信度的即时 merge（来源 A），不创建 abstract

**concept-aware 去重和聚合**：
- `dedupAtDepth(tag, depth, concept)`：优先合并同 concept 内的重复 abstract
- `AggregateOrphanTags`：聚合时只在同 concept 内搜索候选
- `reviewOneTree`（Phase 6）：可以跨树但限定同 concept
- Phase 6 跨 concept 去重：LLM 批量审查同 concept 下所有根 abstract，发现重叠则合并

**空心 abstract 回收**：
```
RecycleEmptyAbstracts():
  查找所有 active abstract:
    - 子标签数 ≤ 1
    - 文章引用数 = 0
    - 存在时间 > 24h（给聚合时间）
  对每个:
    - 断开所有子标签关系
    - 标记为 inactive
    - 子标签重新进入放置队列
```

### 决策 14: concept 管理界面/API

复用现有 narrative 模块的 `board_concepts` 表和管理 API，扩展功能：
1. GET `/api/hierarchy/concepts` — 列出所有 concept（含 pending）
2. POST `/api/hierarchy/concepts` — 用户手动创建 concept
3. PUT `/api/hierarchy/concepts/:id` — 编辑 concept（名称、描述）
4. POST `/api/hierarchy/concepts/:id/confirm` — 确认 pending concept
5. DELETE `/api/hierarchy/concepts/:id` — 停用 concept
6. POST `/api/hierarchy/concepts/bootstrap` — 手动触发聚类生成 concept 建议

前端层级配置页面增加 concept 管理区域，显示：
- 当前 active concept 列表及每个 concept 下的 abstract 数量
- pending concept 列表（可一键确认或忽略）
- "生成板块建议"按钮（触发聚类）

### 模块划分

| 模块 | 改动类型 | 说明 |
|------|---------|------|
| `hierarchy_template` | 修改 | 删除 `GetTagLevel` / `GetTagLevelByID` / `ResolveLevelFromDepth`；新增 `getLevelDef(tmpl, depth)` 辅助 |
| `hierarchy_placement` | **重写** | 通用 `placeTagAtLevel` 替代 L2/L1 专用函数；新增 embedding 就绪检查；新增 concept 约束层；新增 RetryOrphanPlacements；新增空心 abstract 回收；删除所有 L1/L2 硬编码 |
| `hierarchy_aggregation` | **新增** | `AggregateOrphanTags`（concept-aware 版）+ `aggregateToUpperLevel`；通用 depth-based 批量聚合，限定在同 concept 内 |
| `hierarchy_prompts` | **重写** | 通用 `buildMatchPrompt` + `buildCreationPrompt` 替代 4 个硬编码 prompt；通用 `callLLMForMatch` + `callLLMForCreation` |
| `hierarchy_dedup` | 修改 | `dedupAtDepth(tag, depth, concept)` 替代 `dedupL2` / `dedupL1`；concept-aware 去重优先合并同 concept 内的重复 |
| `concept_bootstrap` | **新增** | 标签 embedding 聚类 + LLM 命名 → 生成 pending board_concept；支持手动触发和自动阈值触发 |
| `concept_handler` | **新增** | concept CRUD API（管理界面）；确认 pending concept；查看 concept 下 abstract 统计 |
| `tagger` | 修改 | PlaceTagInHierarchy 调用不变（仍是异步 go func） |
| `queue_batch_processor` | 修改 | adopt_narrower 改用 per-template depth 检查 + `getTagDepthFromRoot` |
| `tag_cleanup` | 修改 | CleanupTemplateViolations 增加自动修复；depth 检查改为 `depth >= tmpl.MaxLevel`；新增 RecycleEmptyAbstracts |
| `hierarchy_cleanup` | 修改 | reviewOneTree 保留 merge/move/复用，关停新建 abstract；`Phase6_DedupL1/L2` → `Phase6_DedupAtDepth(depth, concept)` |
| `abstract_tag_hierarchy` | 修改 | `maxHierarchyDepth` → `getMaxDepthForCategory(category)` |
| `abstract_tag_judgment` | 修改 | `reparentOrLinkAbstractChild` 改用 per-template depth + `getTagDepthFromRoot` |
| `narrative/service` | 修改 | `GenerateAndSaveForCategory` 集成 concept 约束；`tag_feedback` 改为 concept-aware |
| `narrative/board_concept` | 修改 | 新增 `pending` status 支持；新增 concept ↔ abstract 关联 |
| `airouter` / 日志中间件 | 修改 | 确保 Metadata.operation 写入 ai_call_logs |
| `jobs/tag_hierarchy_placement` | **新增** | Placement scheduler struct（1h 间隔） |
| `cmd/hierarchy-cleanup` | **新增** | 一次性数据清理脚本 |
| `cmd/backfill-tag-levels` | 修改 | `hierarchy_level` → `hierarchy_depth`；`GetTagLevelByID` → `getTagDepthFromRoot` |

## Testing Decisions

### 测试原则

- 只测试外部行为（输入→输出），不测试内部实现细节
- 测试用例应覆盖：正常路径、边界情况、错误路径
- 优先使用 Go 标准 table-driven test 模式

### 需要测试的模块

1. **hierarchy_placement**（通用 placeTagAtLevel）：embedding 未就绪时返回 pending、embedding 就绪时正常放置到 targetDepth、放置后子标签 ≥ N 触发上层聚合、candidates 为空时创建新 abstract（而非跳过或报错）、4 层模板的完整放置链路
2. **hierarchy_aggregation**：批量聚合的 LLM prompt 解析、SemanticEmbedding 粗筛逻辑、多个 abstract 合并到同一新 parent、复用已有 abstract、无候选时创建新 abstract
3. **RetryOrphanPlacements**：>10min 的孤儿标签被重试、<10min 的不被重试、已放置的标签不被重试
4. **queue_batch_processor**（adopt_narrower 路径）：per-template depth 约束检查、跨 category 检查、环形检测
5. **tag_cleanup**（CleanupTemplateViolations）：自动修复 depth 超限（depth ≥ tmpl.MaxLevel）、自动修复跨分类、结果写入 pending_changes
6. **hierarchy_cleanup**（reviewOneTree）：new_abstracts 只执行复用不创建、merges/moves 仍正常工作
7. **getMaxDepthForCategory**：每个 category 返回正确的 MaxLevel-1、未知 category 的 fallback
8. **placement scheduler**：1h 间隔调度、Phase 3.7/3.8 执行顺序
9. **depth 语义**：getTagDepthFromRoot 对根/叶/中间节点的返回值正确、tmpl.Levels[depth] 正确映射各层定义
10. **N 层模板兼容性**：2 层（person）、3 层（event）、4+ 层自定义模板的放置和聚合均正常
11. **concept 约束**：放置时 concept 过滤正确、abstract 创建关联到 concept、无 concept 时的高置信度 merge 路径、concept 数量达到上限时的降级行为
12. **concept bootstrap 聚类**：从标签 embedding 生成 concept 建议、LLM 命名质量、阈值触发（category ≥ 20 标签）
13. **空心 abstract 回收**：≤1 子标签 + 0 文章引用 + >24h 存在 → 被回收；有文章引用的不被回收；刚创建 <24h 的不被回收
14. **concept-aware 去重**：同 concept 内的 abstract 优先合并、跨 concept 不合并

### 已有测试作为参考

- `hierarchy_placement_test.go` — 测试向上聚合算法
- `hierarchy_config_test.go` — 测试配置保存/加载
- `hierarchy_cleanup_test.go` — 测试 Phase 6 树审查
- `tag_cleanup_test.go` — 测试清理调度器各阶段
- `abstract_tag_service_test.go` — 测试 reparentOrLinkAbstractChild

## Out of Scope

- 前端大改（concept 管理界面按需添加，不在本 PRD 核心 scope）
- 新增模板或修改模板定义（5 个固定模板不变）
- Person 的 metadata 字段填充（仍然预留，不在本 PRD 范围）
- 配置管理 API 改动（GET/PUT /api/hierarchy/config 不变）
- embedding 生成机制本身的优化
- 修改 airouter 核心库（如果 operation 日志问题出在 airouter 内部，先在 tagging 包层面补写）
- 新增 embedding 类型（现有 Identity + Semantic 足够）

## Further Notes

### 数据诊断摘要（2026-05-13 快照）

| 指标 | 值 |
|------|-----|
| abstract 标签总量 | 1725 |
| active | 160 |
| merged | 891 |
| inactive | 674 |
| 活跃但无子节点 | 4 |
| PlaceTagInHierarchy 覆盖率 | 6%（433 新标签中 26 个进入层级） |
| event 深度违规率 | 50.6% |
| keyword 深度违规率 | 48.3% |
| person 深度违规率 | 76.2% |
| hierarchy_pending_changes | 0 条 |
| "美伊"相关 abstract | 231 个，227 个孤儿 |
| Person 环形引用 | 国际知名人物 ↔ 美国其他公众人物 ↔ 美国政治人物 |
| 5/13 topic_tagging LLM 调用 | 212 次（tag_extraction 71, batch_tag_judgment 23） |
| 5/13 embedding 调用 | 1209 次 |
| l2_match/l1_match/l2_create/l1_create 在 ai_call_logs 中 | 0 条记录 |
| maxHierarchyDepth | 全局常量 = 4（person 应为 2） |
| cleanup scheduler 间隔 | 86400 秒（24 小时） |

### 三个来源的当前职责

| 来源 | 入口 | 触发时机 | 创建 abstract | 创建关系 | 入队 adopt_narrower |
|------|------|---------|--------------|---------|-------------------|
| A: findOrCreateTag | tagger.go | 每个新标签 | 是 | 是 | 是 |
| B: PlaceTagInHierarchy | hierarchy_placement.go | 每个新标签（异步） | 是 | 是 | 否 |
| C: ReviewHierarchyTrees | hierarchy_cleanup.go | 定时调度器（24h） | **是**（需关停创建，保留复用） | 是 | 是 |

### 目标架构

| 来源 | 入口 | 触发时机 | 放置行为 | 复用已有 abstract |
|------|------|---------|---------|------------------|
| A: findOrCreateTag | tagger.go | 每个新标签 | merge/abstract 判断（即时） | 是 |
| B: PlaceTagInHierarchy | hierarchy_placement.go | 每个新标签（异步） | 通用 `placeTagAtLevel` + concept 围栏，放一层到紧邻上层 | 是 |
| D: AggregateOrphanTags | hierarchy_aggregation.go | 调度器 1h / 子标签≥N | 通用 `placeTagAtLevel`（concept-aware），批量向上聚合 | 是 |
| C: ReviewHierarchyTrees | hierarchy_cleanup.go | 调度器 24h | 只做 merge/move/复用，不创建新 abstract | 是 |
| E: ConceptBootstrap | concept_bootstrap.go | 标签阈值触发 / 手动 | 标签聚类 → 生成 pending concept 建议 | — |
| F: RecycleEmptyAbstracts | tag_cleanup.go | 调度器 24h | 回收空心 abstract，子标签重入放置队列 | — |

来源 A 处理即时 merge/abstract 判断，来源 B 负责紧邻上层放置（即时，concept 约束），来源 D 负责更高层聚合（延时，concept 约束），来源 C 只做 merge/move/复用，来源 E 生成 concept 建议，来源 F 回收空心 abstract。所有放置逻辑通过 `depth` 参数化，支持任意 N 层模板，abstract 创建限定在 board_concept 边界内。

### concept 围栏约束示意

```
board_concept "AI开发与效能" (active)
├── abstract "DeepSeek V4 生态事件" (depth=2, 13 children)
├── abstract "OpenAI GPT-5.5 发布与生态事件" (depth=2, 8 children)
├── abstract "英伟达产业链动态" (depth=2, 7 children)
└── abstract "AI 开源生态" (depth=2, 3 children)

board_concept "财经与宏观事件" (active)
├── abstract "港股市场交易时段动态" (depth=2, 4 children)
├── abstract "A 股市场及全球主要资本市场动态与波动" (depth=2, 3 children)
└── abstract "美联储货币政策决策" (depth=2, 2 children)

新标签 "GPT-6 训练动态" 进来
→ MatchTagToConcept → 匹配到 "AI开发与效能"
→ 在该 concept 的 abstract 中找 parent
→ 找到 "OpenAI GPT-5.5 发布与生态事件" → 挂上去 ✓
→ 不会意外挂到 "港股市场" 下
→ 不会创建新的 "GPT-6 训练动态" abstract 根节点
```
