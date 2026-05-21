# Tag Hierarchy Quality (Delta)

## ADDED Requirements

### Requirement: 模板深度合规检查
Phase 3 增加模板合规检查步骤。调度器 SHALL 扫描所有抽象关系，将路径深度超过模板最大层数的标签及其子关系标记为待处理，将子标签 category ≠ 父标签 category 的关系标记为跨分类违规。

#### Scenario: 检测深度超限
- **WHEN** Phase 3 扫描到 event 模板最大 3 层但某标签深度为 5
- **THEN** 该标签的所有子关系被标记为 `hierarchy_pending_changes` 记录，状态 pending

#### Scenario: 检测跨分类关系
- **WHEN** Phase 3 扫描到 keyword 标签作为 event 标签的子节点
- **THEN** 该关系被标记为跨分类违规，状态 pending

### Requirement: Phase 4 收养范围限定
收养更窄标签时，系统 SHALL 将搜索范围限定在模板同一层级内，不跨层收养。收养前检查目标标签的层级与候选的层级是否匹配。

#### Scenario: 拒绝跨层收养
- **WHEN** L1 标签尝试收养 L3 标签（跳级）
- **THEN** 收养被拒绝，记录日志

### Requirement: Phase 6 模板对齐审查
Phase 6 整树审查 SHALL 替换为模板对齐审查流程：
1. 层级对齐检查：每个叶子标签是否有清晰的向上路径到 L2 → L1
2. L1 去重：embedding 相似度 > 0.90 的 L1 标签对 → LLM 判断是否合并
3. L2 去重：embedding 相似度 > 0.95 的 L2 标签对 → 直接合并
4. 叶子归属抽查：随机抽取 10% L3 标签，LLM 复查与 L2 的语义合理性
5. 不自动修复，生成待处理清单供用户手动触发 rebuild

#### Scenario: L2 embedding 去重自动合并
- **WHEN** Phase 6 扫描到两个 L2 标签 embedding 相似度 0.97
- **THEN** 系统自动合并，不需 LLM 判断

#### Scenario: 叶子归属抽查发现异常
- **WHEN** Phase 6 随机抽查 "C++" 标签挂载在 "大模型后端与架构" 下
- **THEN** LLM 判定不合理，标记为待处理

## MODIFIED Requirements

### Requirement: Degenerate abstract trees are flattened
A scheduled cleanup task SHALL detect abstract tag chains where the leaf-to-depth ratio falls below 1.5 and flatten them by removing intermediate abstract nodes, relinking children to the nearest ancestor that provides meaningful grouping. The flattening SHALL respect hierarchy template depth limits: after flattening, no path SHALL exceed the template's maximum depth.

#### Scenario: Four-level chain with 5 leaves flattened within template limit
- **WHEN** cleanup encounters A→B→C→D with D's children being 5 leaf tags, and template allows max 3 levels
- **THEN** intermediate nodes B and C SHALL be deactivated and their children linked directly to A, resulting in max depth 2
