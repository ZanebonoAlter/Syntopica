## Context

当前 `TagHierarchyCleanupScheduler` 的 10 阶段流程中，跨树整理仅靠 Phase 2 Flat Merge（同类 abstract 标签拍平去重，上限 50 个/类）。Phase 6 整树审查是**单树视角**——LLM 只看到一棵树的结构，无法发现树与树之间的连接。同时，Phase 6 通过 14 天窗口过滤，老旧树一旦过了窗口就永远冻结。

现有基础设施已具备嵌入向量相似搜索（`FindSimilarAbstractTags`）、批量 LLM 判断（`batchJudgeAbstractRelationships` 模式）、安全合并/链接（`MergeTags` / `linkAbstractParentChild`），缺少的只是一个系统性的树间桥接编排层。

## Goals / Non-Goals

**Goals:**
- 在每个 cleanup 周期中，自动扫描同类标签森林中应连接但尚未连接的树根对
- 基于 embedding 相似度预筛 + 全局去重算法，确保每对树根只被 LLM 判断一次
- LLM 判断结果安全执行（merge 合并或 parent-child 链接）
- 散落标签（孤立单节点）也能通过同样的机制归入已有树

**Non-Goals:**
- 不解决树内部节点的归属问题（那是 Phase 6 的职责）
- 不修改现有 Phase 6 的 14 天窗口逻辑
- 不引入新的数据库 schema
- 不增加前端 API

## Decisions

### Decision 1: 全局去重算法

**问题**：逐根调用 `FindSimilarAbstractTags` 时，树根 A 的查询结果可能包含 B，树根 B 的查询结果也可能包含 A，导致同一标签对进入多次 LLM 判断。

**方案**：

```
Algorithm: collectTreeBridgePairs(category)

Input:  category (event|keyword|person)
Output: []uniqueTreePair (全局去重、按相似度降序)

1. forest = BuildTagForest(category, minDepth=1)
   → 收集所有树根标签（含仅 1 层的单节点散落标签）

2. pairSet = map[string]pairInfo{}  // key = "min(idA,idB)|max(idA,idB)"
   // string key 确保 id 有序，杜绝 (A,B) 和 (B,A) 重复

3. For each root in forest:
   a. candidates = FindSimilarAbstractTags(root.ID, category, limit=15)
   b. For each cand where cand.Similarity ≥ 0.78:
      key = format("%d|%d", min(root.ID, cand.ID), max(root.ID, cand.ID))
      if pairSet[key] exists → skip  // 已有其他 root 的查询产出
      pairSet[key] = {rootA: min 的一方, rootB: max 的一方, sim: cand.Similarity}

4. Flatten pairSet → list, sort by similarity DESC

5. Return list (global cap: 50 对，防止 LLM 预算溢出)
```

**选择理由**：
- `FindSimilarAbstractTags` 内部已有 `NOT EXISTS` 排除已有父子关系的标签，所以不会生成已连接对
- string key 基于 `min(idA,idB)|max(idA,idB)` 保证幂等
- 每 root 的 limit=15 兼顾召回率和查询开销
- **备选方案**：单条 bulk SQL 直接在 pgvector 层做 pairwise 比较。未选的原因：`FindSimilarAbstractTags` 已有成熟逻辑（semantic→identity 级联、active 过滤），复用代码比新建 SQL 更安全

### Decision 2: LLM 批量判断 prompt 设计

**问题**：需要让 LLM 在一次性调用中判断多对树根的关系。

**方案**：借鉴 `batchJudgeAbstractRelationships` 的批量模式，新建 `batchJudgeTreeBridgePairs`：

```
Prompt 结构（每对一对）:

候选对列表（每对一个 entry）:
1. 树根 A: "机器学习" (25 子节点, 120 文章, desc: ..., 子标签: [监督学习, 无监督学习, ...])
   树根 B: "深度学习" (18 子节点, 80 文章, desc: ..., 子标签: [CNN, RNN, Transformer, ...])
   相似度: 0.86
   候选: 摘要对比

→ LLM 对每对判断:
{
  "pairs": [{
    "index": 1,
    "action": "merge" | "parent_A" | "parent_B" | "skip",
    "reason": "brief explanation"
  }]
}

merge: 两棵树描述同一概念，应合并（保留子节点多、文章多的一方）
parent_A: B 是 A 的窄概念，B 树整体应挂在 A 下
parent_B: A 是 B 的窄概念，A 树整体应挂在 B 下
skip: 不相关
```

每个 pair entry 包含根标签的完整 prompt 上下文（`formatTagPromptContext`，人物含结构化属性）和前 8 个子节点信息，确保 LLM 有足够上下文做判断。

**选择理由**：
- 复用 `batchJudgeAbstractRelationships` 的批量判断模式（一次 LLM 调用判断多对）
- 复用 `formatTagPromptContext` 提供结构化上下文
- 单 batch 放在 ≤20 对以内（每个 entry ~200 tokens，20 对 ≈ 4k tokens，在大多数模型窗口内）

### Decision 3: 执行顺序与安全

**问题**：先 merge 再 parent 可能产生级联效果——merge 后的 target 可能又是其他 pair 中的角色。需要正确的执行顺序。

**方案**：

```
执行顺序:
1. 先执行所有 "merge" 判断
   - MergeTags(source, target) → source 状态变成 "merged"
   - 维护 skipSet: 记录被合并（已消失）的标签 ID
   
2. 再执行 "parent" 判断
   - 检查 pair 中的两个标签是否都在 skipSet 中
   - 调用 linkAbstractParentChild(child, parent)
   - 已有循环检测 + 深度限制 + 重复关系跳过保护
```

**选择理由**：
- `MergeTags` 的 `migrateTagRelations` 自动将 source 的子节点迁移到 target，子树完整保留
- `linkAbstractParentChild` 自带循环检测、深度限制（maxHierarchyDepth=4）、已存在关系跳过
- **备选方案**：所有操作在同一个事务中执行。未选的原因：现有 `MergeTags` 和 `linkAbstractParentChild` 各自在独立事务中，合入会增加复杂度和失败域

### Decision 4: 调度集成位置

**方案**：作为 Phase 3.5，插入在 Phase 3（层级修剪）和 Phase 4（收养更窄标签）之间。

```
Phase 3: 层级修剪（无 LLM）
  → 此时所有孤儿关系、多父冲突已清除
  → 树结构干净，适合做桥接判断

Phase 3.5: Tree Bridge（有 LLM）
  → 扫描树根对，执行 merge/parent
  → 可能产生新的层级关系

Phase 4: 收养更窄标签（有 LLM）
  → 桥接后新形成的父标签可以收养更窄的抽象标签
```

**选择理由**：
- Phase 3 清完脏数据后再桥接，减少干扰
- 桥接后立刻进入 Phase 4 收养，新关系能无缝融入后续流程
- Phase 5 抽象刷新 + Phase 6 整树审查可以看到桥接后的最新结构

## Risks / Trade-offs

- **[风险] 嵌入过滤遗漏**：`FindSimilarAbstractTags` 使用 semantic embedding，不同语言的同概念标签可能相似度不够高。→ **缓解**：依赖 Phase 2 Flat Merge 和 Phase 4 收养更窄标签做补充覆盖
- **[风险] LLM 误判合并**：merge 是不可逆操作（source 标签状态变为 merged）。→ **缓解**：prompt 中强调 merge 仅用于"同一概念的同义词/翻译"，strict 条件；保留 `pairSet` 的 `similarity` 作为辅助信号
- **[风险] 桥接后深度溢出**：两棵树连接后总深度可能超过 maxHierarchyDepth=4。→ **缓解**：`linkAbstractParentChild` 内置深度检查，超限返回错误
- **[权衡] LLM 预算增加**：每次周期新增 3-9 次 LLM 调用（3 类 × 1-3 批）。当前总预算 60 次，足够容纳。但仍需设置 Phase 3.5 配额防止极端情况下挤占 Phase 6 预算
