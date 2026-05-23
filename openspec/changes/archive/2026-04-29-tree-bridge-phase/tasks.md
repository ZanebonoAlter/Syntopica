## 1. Core: Tree Bridge 执行器

- [x] 1.1 新建 `backend-go/internal/domain/topicanalysis/tree_bridge.go`
- [x] 1.2 实现 `collectTreeBridgePairs(category string) ([]treeBridgePair, error)` — 逐根调用 `FindSimilarAbstractTags` + 全局去重算法（`min(idA,idB)|max(idA,idB)` key）
- [x] 1.3 实现 `buildTreeBridgePrompt(pairs []treeBridgePair) string` — 构建批量 LLM prompt，每对含根标签上下文 + 前 8 子节点 + 相似度
- [x] 1.4 实现 `callTreeBridgeLLM(prompt string) (*treeBridgeJudgment, error)` — LLM 调用 + JSON 解析，输出 merge/parent_A/parent_B/skip
- [x] 1.5 实现 `executeTreeBridgePairs(pairs []treeBridgePair, judgment *treeBridgeJudgment, category string) (int, []string, error)` — 先执行 merge（维护 skipSet），再执行 parent，调用现有 `MergeTags` / `linkAbstractParentChild`
- [x] 1.6 实现 `ExecuteTreeBridge(category string, budget LLMBudget) (*TreeBridgeResult, error)` — 组合上述步骤，含预算检查和错误收集

## 2. Scheduler Integration

- [x] 2.1 在 `TagHierarchyCleanupRunSummary` 中新增 `TreeBridgeMerges int` 和 `TreeBridgeLinks int` 字段
- [x] 2.2 在 `runCleanupCycle` 中 Phase 3 之后插入 Phase 3.5，按 event → keyword → person 顺序调用 `ExecuteTreeBridge`
- [x] 2.3 将 Phase 3.5 结果计入 `summary`（merges 数 / parent links 数 / errors）

## 3. Budget Management

- [x] 3.1 在 `runCleanupCycle` 中为 Phase 3.5 设置配额 `budget.SetPhaseQuota("phase3_5", 5)`
- [x] 3.2 `ExecuteTreeBridge` 实现预算感知：每批次 LLM 调用前检查 `budget.ConsumeForPhase("phase3_5")` 和 `budget.IsTimedOut()`

## 4. Tests

- [x] 4.1 新增 `tree_bridge_test.go`：测试全局去重算法（注入 mock `FindSimilarAbstractTags` 复现 A→B、B→A 的重复场景）
- [x] 4.2 测试 `buildTreeBridgePrompt` 输出格式正确性
- [x] 4.3 测试 `executeTreeBridgePairs` 的 skipSet 和顺序正确性
- [x] 4.4 运行 `go test ./internal/domain/topicanalysis/...` 确保全部通过
- [x] 4.5 运行 `go test ./internal/jobs/...` 确保调度器测试通过

## 5. Documentation

- [x] 5.1 更新 `docs/user-guide/ai-features/tagging-flow.md` 第 12 节标签清理机制，新增 Phase 3.5 描述和流程图更新
