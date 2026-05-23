## Why

当前 `TagHierarchyCleanupScheduler` 的标签清理机制只做整树审查（Phase 6）和扁平去重（Phase 2 Flat Merge），缺少系统性的树间桥接能力。新产生的标签树在 Phase 6 的 14 天窗口过后就永远冻结不再审查，语义相同但分属不同树的标签无法自动合并，孤立的散落标签无法归入已有树结构。用户需要不断手动触发清理任务来缓解碎片化，这应该是自动化完成的。

## What Changes

- 在 `TagHierarchyCleanupScheduler` 中新增 **Phase 3.5: Tree Bridge**，在层级修剪后、收养更窄标签前执行
- 新建 `ExecuteTreeBridge(category string, budget LLMBudget)` 函数，扫描同类树根之间是否应该桥接（合并或建立父子关系）
- 基于 embedding 相似度 + 全局去重算法，避免同一标签对重复进入 LLM 判断
- 每对树根候选带着子节点上下文送 LLM 批量判断，输出 merge（合并）/ parent（父子）/ skip
- 结果通过已有的 `MergeTags` 和 `linkAbstractParentChild` 安全执行

## Capabilities

### New Capabilities
- `tree-bridge`: 跨树桥接——识别同类标签森林中应该合并或建立父子关系的树根对，通过 embedding 预筛 + 全局去重 + LLM 批量判断实现自动化树间整理

### Modified Capabilities
<!-- 不涉及现有 capability 的 spec 级变更 -->

## Impact

- `backend-go/internal/domain/topicanalysis/` — 新增 `tree_bridge.go`
- `backend-go/internal/jobs/tag_hierarchy_cleanup.go` — 插入 Phase 3.5 调用
- `backend-go/internal/jobs/cleanup_budget.go` — 可能新增 Phase 3.5 配额
- 不影响现有 API、前端、数据库 schema
