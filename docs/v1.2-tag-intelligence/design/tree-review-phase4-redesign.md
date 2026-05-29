# Phase 4 整树审查替换设计

> 日期: 2026-04-25
> 状态: 已批准

## 背景

当前 Phase 4 (`ExecuteHierarchyCleanupPhase4`) 仅处理深度≥3 的标签树，逻辑为跨层去重 merge + 深度>4 的 AI 建议 reparent。存在两个问题：

1. **不审查归属正确性**：只看深度和重复，不检查子标签是否真的属于该父标签。导致"伊朗政治人物"被挂到"美国及拉美政治人物"下。
2. **深度阈值过于刚性**：深度<3 的树完全不审查，错配无法发现。

## 设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 审查粒度 | 整棵树送 LLM | 全局视角，能发现跨层错配 |
| 错配处理 | AI 建议新归属 + 自动迁移 | 减少人工干预 |
| 审查范围 | 仅含时间窗口内新关系的树 | 控制 LLM 消耗 |
| 新抽象归属 | 走 findSimilarExistingAbstract + MatchAbstractTagHierarchy | 不无脑放根目录 |
| 替换 vs 新增 | 替换 Phase 4 | 职责重叠，整树审查是超集 |

## 核心流程

```
Phase 4 (新): ReviewHierarchyTrees

对每个 category (event, keyword, person):
  1. BuildTagForest(category)           // 复用，minDepth=2
  2. 过滤：只保留含 windowDays 内新关系的树
  3. 大树拆分（>50 节点按子树拆）+ 根层审查（root + 直接子节点）
  4. 序列化为文本树 → LLM 审查
  5. LLM 返回 moves + new_abstracts
  6. 校验每条 move → 执行；迁移校验失败则跳过，不自动 detach
  7. 校验每条 new_abstract → 匹配已有抽象或创建新抽象，并分别统计 reused/created
```

## LLM 接口

### 输入

```
标签树（person 类别）:
[id:42] 政治人物
  ├── [id:101] 美国及拉美政治人物 (描述: ...)
  │     ├── [id:201] 美国总统 (描述: ...)
  │     └── [id:202] 拉美领导人 (描述: ...)
  ├── [id:102] 伊朗政治人物 (描述: ...)
  │     ├── [id:301] 伊朗总统 (描述: ...)
  └── [id:103] 东南亚政治人物 (描述: ...)

规则:
- 检查每个子标签是否真正属于其父标签
- 地理/区域不同且无直接关联的，不应在同一抽象父下
- 概念领域明显不同的，不应在同一父下
- to_parent=0 表示脱离成为独立根节点
- 非零 to_parent 表示迁移到指定标签下（必须是树中已有的 ID）
- new_abstracts 用于建议创建新的分组，不指定父（系统会自动匹配归属）
```

### 输出

```json
{
  "moves": [
    {"tag_id": 102, "to_parent": 0, "reason": "伊朗与美洲/东南亚属于不同地理区域"},
    {"tag_id": 305, "to_parent": 88, "reason": "更具体的归属"}
  ],
  "new_abstracts": [
    {"name": "中东政治人物", "description": "...", "children_ids": [102, 303], "reason": "为中东区域创建独立分组"}
  ]
}
```

## new_abstract 匹配流程

```
LLM 建议 new_abstract
  ↓
findSimilarExistingAbstract(name, desc, category, children)
  ↓
找到已有抽象 → 复用，children 挂到该抽象下
  ↓
没找到 → createAbstractTagDirectly() 创建
  → 异步: MatchAbstractTagHierarchy → 自动寻找层级归属
  → 异步: adoptNarrowerAbstractChildren
  → 异步: EnqueueAbstractTagUpdate
```

## Move 校验

| 检查 | 失败处理 |
|------|----------|
| tag_id 存在且 active | 跳过 |
| tag_id 在当前树中 | 跳过 |
| to_parent=0 或 to_parent 存在且 active | 跳过 |
| `wouldCreateCycle(tag_id, to_parent)` | 跳过 |
| 深度检查 ≤4 | 跳过 |

校验通过后执行：
1. 如果 to_parent=0：删除该 tag 的全部 abstract 父关系，让它成为独立根节点，并更新旧父摘要
2. 如果 to_parent≠0：先创建新父关系 `linkAbstractParentChild(tag_id, to_parent)`，成功后再 `resolveMultiParentConflict(tag_id)` 清理旧父关系
3. 迁移失败时不删除旧关系，避免产生孤儿标签
4. `EnqueueAbstractTagUpdate(旧父, "child_moved")`
5. 如果 to_parent≠0：`EnqueueAbstractTagUpdate(新父, "child_adopted")`

## 大树拆分策略

> 目标：控制单次 LLM token，同时不漏掉根节点与第一层子节点之间的错挂。

| 场景 | 审查方式 |
|------|----------|
| ≤50 节点 | 整树审查 |
| >50 节点 | 先审查 root + 直接子节点摘要，再按子树递归拆分审查 |

根层审查只允许移动直接子节点或建议新抽象，不审查深层节点。这样可以发现“伊朗政治人物”挂在“美国及拉美政治人物”同层/父层附近这类错配，同时避免把整棵大树塞进单次 prompt。

## 文件变更

| 文件 | 改动 |
|------|------|
| `topicanalysis/hierarchy_cleanup.go` | 新增 `ReviewHierarchyTrees` + `callTreeReviewLLM` + `serializeTreeForReview` + `validateTreeReviewMove` + `executeTreeReviewMove`；修改 `BuildTagForest` 增加 `minDepth` 参数（默认仍为 `MinTreeDepthForCleanup`，tree review 显式传 2）；删除旧 `cleanupDeepHierarchyTree`、`ProcessTree`、`processBatch`、`processRootCrossLayer` |
| `topicanalysis/hierarchy_cleanup.go` | 复用 `createAbstractTagDirectly` 现有的 `findSimilarExistingAbstract` 匹配逻辑 |
| `jobs/tag_hierarchy_cleanup.go` | Phase 4 调用替换 + summary 字段更新 + Reason 更新 |

## Summary 字段变更

```go
// 删除
Phase4Trees     int
Phase4Merges    int
Phase4Reparents int

// 替换为
TreesReviewed   int `json:"trees_reviewed"`
MovesApplied    int `json:"moves_applied"`
GroupsCreated   int `json:"tree_groups_created"`
GroupsReused    int `json:"tree_groups_reused"`
```

## 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| windowDays | 14 | 只审查含 N 天内新关系的树 |
| batchSize | 50 | 每批送 LLM 的最大节点数 |
| categories | event, keyword, person | 审查的标签类别 |
| minDepth | 2 | 最小树深度（<2 的树没有审查价值） |
