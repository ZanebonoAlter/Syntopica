## ADDED Requirements

### Requirement: 跨树桥接扫描
系统在标签清理调度流程中 SHALL 对每个类别（event、keyword、person）的标签森林进行树间桥接扫描，识别应合并或建立父子关系的树根对。

#### Scenario: 同类别扫描
- **WHEN** `ExecuteTreeBridge` 被调用且传入 category
- **THEN** 系统仅扫描该 category 下 `source='abstract'` 且 active 的树根标签，不跨类别比较

#### Scenario: 嵌入预筛
- **WHEN** 扫描树根对
- **THEN** 系统使用 `FindSimilarAbstractTags` 按 semantic embedding 相似度查找候选，仅保留 similarity ≥ 0.78 的标签对

### Requirement: 全局去重
系统 SHALL 确保每对树根在整个桥接周期中最多被 LLM 判断一次，无论这对被多少个不同根标签的嵌入查询发现。

#### Scenario: 标签对去重
- **WHEN** 树根 A 的 `FindSimilarAbstractTags` 查询返回树根 B，且树根 B 的查询也返回树根 A
- **THEN** 系统使用全局去重映射 `min(idA,idB)|max(idA,idB)` 保证仅保留一份

#### Scenario: 已有关系排除
- **WHEN** 一对标签已存在 abstract 父子关系
- **THEN** `FindSimilarAbstractTags` 的 NOT EXISTS 子句自动排除，不进入候选列表

### Requirement: 批量 LLM 判断树对关系
系统 SHALL 将去重后的树根对批量交给 LLM 判断，每对返回 merge、parent_A（B 挂在 A 下）、parent_B（A 挂在 B 下）或 skip 四种动作之一。

#### Scenario: 批量判断格式
- **WHEN** 有多对树根待判断
- **THEN** 系统构建包含每对根标签上下文（label、description、前 8 个子节点、文章数、相似度）的 prompt，LLM 返回 `{pairs: [{index, action, reason}]}` JSON

#### Scenario: 批量大小限制
- **WHEN** 待判断对超过 20 对
- **THEN** 系统分批次调用 LLM，每批不超过 20 对

#### Scenario: 单对判断
- **WHEN** 去重后仅剩 1 对
- **THEN** 系统仍使用批量 prompt 格式（1 对列表），保持格式一致性

### Requirement: 安全执行桥接操作
系统 SHALL 按先 merge 后 parent 的顺序执行 LLM 返回的判断结果，并维护已消失标签集合以避免操作已合并的标签。

#### Scenario: Merge 执行
- **WHEN** LLM 判断一对标签为 "merge"
- **THEN** 系统调用 `MergeTags(source, target)`，其中 source 为子节点数更少或文章数更少的标签，source 被标记为 "merged" 后加入 skipSet

#### Scenario: Parent 执行
- **WHEN** LLM 判断一对标签为 parent_A 或 parent_B
- **THEN** 系统调用 `linkAbstractParentChild(childID, parentID)`，建立 abstract 父子关系

#### Scenario: 已合并标签跳过
- **WHEN** 一对标签中有任一方已在 skipSet 中（被前序 merge 操作合并）
- **THEN** 系统跳过该对，记录日志

### Requirement: 调度器集成
`TagHierarchyCleanupScheduler` SHALL 在 Phase 3（层级修剪）完成后、Phase 4（收养更窄标签）之前执行 Tree Bridge 阶段。

#### Scenario: Phase 3.5 位置
- **WHEN** cleanup 周期执行
- **THEN** Phase 3.5 在 Phase 3 之后运行，按 event → keyword → person 顺序处理三个类别

#### Scenario: 预算管理
- **WHEN** LLM 预算耗尽或超时
- **THEN** Phase 3.5 停止处理剩余类别，不阻塞后续 Phase 4-7
