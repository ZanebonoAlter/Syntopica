> **已废弃**: 此文档描述的"深层树修剪"流程已被 [三级清理策略](tag-cleanup-redesign.md) 取代。保留供参考。

> **当前实际流程（简版）**: 现在调度器只做三件事：先停用长期没用的标签，再合并明显重复的抽象标签，最后清掉坏掉的层级关系。下面这份长流程图是旧方案，不再代表当前代码的主流程。

# Tag Hierarchy Cleanup — 完整流程图

## 端到端流程

```
┌─────────────────────────────────────────────────────────────────────┐
│  Trigger (cron @every 24h / manual POST)                           │
│  tag_hierarchy_cleanup.go:cleanupHierarchy()                       │
└──────────────────────┬──────────────────────────────────────────────┘
                       │ TryLock(executionMutex)
                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│  runCleanupCycle(triggerSource)                                     │
│  categories = ["event", "keyword"]                                  │
│                                                                     │
│  for each category:                                                 │
│    ┌───────────────────────────────────────────────────────────┐    │
│    │ BuildTagForest(category)          →  hierarchy_cleanup.go │    │
│    │   1. 加载所有 abstract 关系                               │    │
│    │   2. 构建 parent→children 映射                            │    │
│    │   3. 找根节点(无parent) / 环路入口                        │    │
│    │   4. 加载 active tags (按 category 过滤)                  │    │
│    │   5. 计算每棵树深度, 只保留 depth ≥ 5                    │    │
│    └─────────────────────┬─────────────────────────────────────┘    │
│                          │ forest []*TreeNode                       │
│                          ▼                                         │
│    for each tree in forest:                                         │
│    ┌───────────────────────────────────────────────────────────┐    │
│    │ ProcessTree(tree)                →  hierarchy_cleanup.go  │    │
│    │                                                           │    │
│    │  countNodes(tree) ≤ 50 ?                                  │    │
│    │     YES → processBatch(tree)                              │    │
│    │     NO  → for child in tree.Children:                     │    │
│    │              ProcessTree(child)   ← 递归                  │    │
│    │           processRootCrossLayer(tree)  ← ★新增★          │    │
│    └─────────────────────┬─────────────────────────────────────┘    │
│                          │                                         │
└──────────────────────────┼─────────────────────────────────────────┘
                           ▼
              updateSchedulerStatus("success")
```

---

## ProcessTree 详细分支

```
ProcessTree(node)
│
├─ 节点数 ≤ 50 ──► processBatch(node)
│                   │
│                   ├─ buildCleanupPrompt(root, tags)
│                   │   构建包含树结构信息的 JSON prompt
│                   │
│                   ├─ callCleanupLLM(prompt)
│                   │   ├─ airouter.NewRouter()
│                   │   ├─ CapabilityTopicTagging + JSONMode
│                   │   └─ 返回 treeCleanupJudgment
│                   │       { merges[], abstracts[], notes }
│                   │
│                   ├─ 对每个 merge:
│                   │   validateAndExecuteMerge(merge, tagMap)
│                   │     ├─ 检查 source/target 存在
│                   │     ├─ 检查非同一标签
│                   │     ├─ 检查均为 active
│                   │     ├─ 检查非直接父子
│                   │     ├─ 检查 depth 差 ≥ 2
│                   │     └─ MergeTags(sourceID, targetID)  ← 事务
│                   │         ├─ migrateTagRelations()
│                   │         └─ enqueueAbstractTagUpdateIfTargetIsAbstract()
│                   │
│                   └─ 对每个 abstract:
│                       validateAndExecuteAbstract(abstract, tagMap, category)
│                         ├─ 检查 ≥ 2 个 children
│                         ├─ 检查 children 存在且 active
│                         └─ createAbstractTagDirectly()  ← ★修复★ 不再调LLM
│                             ├─ Slugify(name)
│                             ├─ 检查 slug 冲突
│                             ├─ 查找相似已有 abstract
│                             ├─ DB Transaction:
│                             │   ├─ 创建 TopicTag (source=abstract)
│                             │   └─ 创建 TopicTagRelation (含环检测)
│                             └─ 后台: embedding + hierarchy + resolveMultiParent
│
└─ 节点数 > 50 ──► for child in node.Children:
                      ProcessTree(child)   ← 递归
                   processRootCrossLayer(node)  ← ★新增★
                      ├─ collectDeepNodes(depth ≥ 3)
                      ├─ batch = [root] + deepNodes (≤50)
                      ├─ buildCleanupPrompt → callCleanupLLM
                      └─ 只执行 merges (不创建 abstracts)
```

---

## 输入输出实例

### 输入: BuildTagForest("event")

假设数据库中有以下 abstract 关系:

```
parent_id  child_id  relation_type
─────────  ───────── ─────────────
10         20        abstract       ← depth 1→2
20         30        abstract       ← depth 2→3
30         40        abstract       ← depth 3→4
40         50        abstract       ← depth 4→5  ← 达到深度 5
40         51        abstract
10         21        abstract
```

所有 tags 的 category = "event", status = "active"。

**输出 forest:**
```
forest = [
  TreeNode {
    Tag: {ID:10, Label:"科技", Depth:1},
    Children: [
      {
        Tag: {ID:20, Label:"人工智能", Depth:2},
        Children: [
          {
            Tag: {ID:30, Label:"机器学习", Depth:3},
            Children: [
              {
                Tag: {ID:40, Label:"深度学习", Depth:4},
                Children: [
                  {Tag: {ID:50, Label:"神经网络", Depth:5}},
                  {Tag: {ID:51, Label:"DNN", Depth:5}},
                ]
              }
            ]
          }
        ]
      },
      {Tag: {ID:21, Label:"区块链", Depth:2}},
    ]
  }
]
// 深度=5 ≥ 5, 被保留
```

---

### 输入: processBatch 的 LLM Prompt (节选)

```json
{
  "tree_info": {
    "root_label": "科技",
    "max_depth": 5,
    "total_tags": 6,
    "category": "event"
  },
  "tags": [
    {"id": 10, "label": "科技",      "depth": 1, "article_count": 120, "children_ids": [20, 21]},
    {"id": 20, "label": "人工智能",  "depth": 2, "article_count": 80,  "children_ids": [30], "parent_id": 10},
    {"id": 21, "label": "区块链",    "depth": 2, "article_count": 45,  "parent_id": 10},
    {"id": 30, "label": "机器学习",  "depth": 3, "article_count": 60,  "children_ids": [40], "parent_id": 20},
    {"id": 40, "label": "深度学习",  "depth": 4, "article_count": 35,  "children_ids": [50, 51], "parent_id": 30},
    {"id": 50, "label": "神经网络",  "depth": 5, "article_count": 15,  "parent_id": 40},
    {"id": 51, "label": "DNN",       "depth": 5, "article_count": 8,   "parent_id": 40}
  ]
}
```

### LLM 返回示例

```json
{
  "merges": [
    {
      "source_id": 51,
      "target_id": 50,
      "reason": "DNN 是神经网络的一种实现形式，概念高度重叠，且 DNN 文章少，应并入神经网络"
    }
  ],
  "abstracts": [
    {
      "name": "AI基础理论",
      "description": "涵盖人工智能、机器学习、深度学习、神经网络等基础研究方向",
      "children_ids": [20, 30, 40],
      "reason": "这三级标签形成了一个完整的AI理论体系，可以用一个抽象标签统一"
    }
  ],
  "notes": "科技→人工智能 路径过深，建议后续考虑扁平化"
}
```

---

### 输出: TreeCleanupResult

```
TreeCleanupResult {
  TreeRootID:      10,
  TreeRootLabel:   "科技",
  TagsProcessed:   6,
  MergesApplied:   1,       ← DNN(51) → 神经网络(50)
  AbstractsCreated: 1,      ← "AI基础理论" 包含 [20, 30, 40]
  Errors:          [],
}
```

---

### 输出: Scheduler Status (旧示例，已过时)

```json
{
  "status": "success",
  "last_execution_result": {
    "trigger_source": "scheduled",
    "started_at": "2026-04-22T03:00:00Z",
    "finished_at": "2026-04-22T03:01:23Z",
    "zombie_deactivated": 120,
    "flat_merges_applied": 8,
    "orphaned_relations": 14,
    "multi_parent_fixed": 3,
    "empty_abstracts": 27,
    "errors": 0,
    "reason": "zombie=120, flat_merges=8, orphaned_rels=14, multi_parent=3, empty_abstracts=27"
  }
}
```

如果要看当前真实流程，请优先看 `docs/architecture/tag-cleanup-redesign.md`。

---

## 大树处理实例 (>50 节点)

```
输入: 一棵 80 节点的树, 根=科技(depth=1)

ProcessTree(科技)
│
├─ 80 > 50, 递归子树:
│   ├─ ProcessTree(人工智能)   → 30 节点 ≤ 50 → processBatch → 3 merges
│   ├─ ProcessTree(硬件)       → 25 节点 ≤ 50 → processBatch → 0 merges
│   └─ ProcessTree(互联网)     → 25 节点 ≤ 50 → processBatch → 1 merge
│
├─ processRootCrossLayer(科技)  ← ★新增★
│   collectDeepNodes(depth ≥ 3):
│     [机器学习(d3), 深度学习(d4), 神经网络(d5), DNN(d5), 
│      云计算(d3), 容器(d4), ...]  共 45 个
│   
│   batch = [科技(d1)] + 45个深层节点 = 46 ≤ 50
│   
│   LLM 判断: 科技 vs 深层节点的跨层比较
│   → 可能发现: "互联网(d2)" 子树下的 "云计算(d3)" 和
│      "人工智能(d2)" 子树下的 "深度学习(d4)" 有重叠
│
└─ 合并所有结果: 4 merges + 0 abstracts + crossResult
```

---

## 关键文件对照

| 文件 | 职责 |
|------|------|
| `hierarchy_cleanup.go` | 核心逻辑: 建树、分割、LLM 调用、验证、合并、创建 abstract |
| `hierarchy_cleanup_test.go` | 单元测试: 纯逻辑函数（无 DB/LLM 依赖） |
| `tag_hierarchy_cleanup.go` | 调度器: cron 定时、手动触发、状态管理、日志 |
| `runtime.go` | 注册调度器到 Runtime + graceful shutdown |
| `handler.go` | HTTP API: 状态查询、手动触发、修改间隔 |
