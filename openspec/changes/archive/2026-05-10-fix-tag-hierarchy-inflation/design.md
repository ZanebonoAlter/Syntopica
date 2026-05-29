## Context

当前标签层级系统有三个结构性漏洞导致标签膨胀：

1. **Slug 空白差异**：`Slugify("DeepSeek首轮融资")` → `deepseek首轮融资`，`Slugify("DeepSeek 首轮融资")` → `deepseek 首轮融资`。两者 slug 不同，slug 级别的去重失败，embedding 匹配也未捕获。
2. **无信息增益检查**：LLM 自由创建抽象标签，最终 5 篇文章撑起 14 个标签的 4 层树，每层仅有 1-2 个子节点。
3. **多父节点**：同一个子标签可以被挂到多个不相关的父节点下，导致语义混乱。

现有 `CleanupSingleChildAbstractNodes` 只能被动清理单子节点，无法阻止问题产生。

## Goals / Non-Goals

**Goals:**
- Slug 规范化消除空白变体重复
- 创建抽象标签前检查信息增益（最小子节点数、文章集覆盖度）
- 修复已存在的空白变体重复、退化抽象层、多父冲突
- 多父节点分配时检查是否已存在合理的父节点

**Non-Goals:**
- 不修改 LLM prompt（prompt 改动风险大，先修护城河）
- 不改变 tag API 响应格式
- 不做标签质量重新评分
- 不删除任何文章关联

## Decisions

### Decision 1: Slug 在 `Slugify()` 内部规范化空白

**选择**：在 `topictypes.Slugify()` 中，`TrimSpace` 之后、`ToLower` 之前插入空白规范化：`\s+` → 单个空格。

**替代方案**：
- 在 `dedupeTagsWithCategory` 中做 → 拒绝：只修了一处，其他 slug 创建路径不受保护
- 在 `TagMatch` 中加 normalized-slug 回退 → 拒绝：治标不治本，新 tag 创建时 slug 仍然不一致

**理由**：从源头保证所有 slug 以相同方式生成，所有调用方自动受益。

```go
func Slugify(value string) string {
    clean := strings.ToLower(strings.TrimSpace(value))
    clean = spacePattern.ReplaceAllString(clean, " ")  // \s+ → single space
    clean = punctuationPattern.ReplaceAllString(clean, "-")
    clean = strings.Trim(clean, "-")
    return clean
}
```

### Decision 2: 信息增益检查在 `processAbstractJudgment()` 中执行

**选择**：在创建抽象标签前，对候选子标签做两步检查：

1. **最小子标签数 ≥ 2**：如果 LLM 返回 `children` 少于 2 个，直接拒绝创建抽象（等同于 merge into single）
2. **文章集重叠度 ≤ 70%**：对每对候选子标签，计算 Jaccard 相似度（交集/并集）。如果任一 pair 的 Jaccard > 0.7，拒绝创建抽象——这些标签应该被 merge 而非被一个抽象层包裹
3. **树叶比 ≥ 1.5**：如果当前子树的叶子标签总数 / 当前深度 < 1.5，拒绝创建抽象（退化的浅层树）

**替代方案**：
- 在 LLM prompt 中加规则 → 拒绝：LLM 不知道数据库中的文章数，无法计算
- 只检查子节点数 → 拒绝：不解决 2-child-per-level 的退化链

**实现位置**：`topicanalysis/abstract_tag_service.go` 的 `processAbstractJudgment()` 函数入口处。

### Decision 3: 清理任务作为新的 scheduler 函数

**选择**：新增三个清理函数，在现有 cleanup scheduler 中注册：

- `CleanupWhitespaceDuplicateTags()`：对 `topic_tags` 中 slug 经规范化后相同的 active tag 对，合并（含 article 关联迁移、tag merge、embedding 清理）
- `CleanupDegenerateAbstractTrees()`：遍历所有 abstract 标签，如果深度链 > 3 且叶子数 < 深度×2，将最深层的 abstract 提升为直接父节点（跳过中间的退化层）
- 复用 `CleanupMultiParentConflicts()`（已存在），增强其检查逻辑

**理由**：保持现有 cleanup scheduler 框架不变，新函数按需注册。

### Decision 4: 多父节点预防在 `linkAbstractParentChild()` 中

**选择**：调用 `linkAbstractParentChild()` 时（在 `abstract_tag_hierarchy.go`），如果子标签已有 active abstract 父节点且新父节点和现有父节点的文章集不彻底分离（Jaccard > 0.3），拒绝分配第二个父节点，记录日志。

**理由**：已有 `resolveMultiParentConflict` 和 `batchResolveMultiParentConflicts` 做被动清理，预防可以避免问题积累。

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Slug 规范化后现有 slug 与 DB 中的不一致 | 清理任务 `CleanupWhitespaceDuplicateTags` 会遍历修复；历史数据的 slug 保持不变（已创建 tag 不 retroactively 改 slug） |
| 信息增益阈值太激进，阻止合理抽象 | 阈值可配置（`embedding_config` 表），默认 0.7 偏保守 |
| Slug 规范化改变空格在 slug 中的角色（原先空格被 `\s+` 吃掉） | `\s+` → `-` 是原行为；现在 `\s+` → ` ` 再 `[^Lp{N}]` → `-`，空格保留为字分隔符，更合理 |
| 清理任务批量操作可能超时 | 分批处理（batch size 50），记录进度 |

## Migration Plan

1. 部署新代码（Slugify 改动 + 信息增益检查 + 新增清理函数）
2. scheduler 自动运行清理任务，首次执行会清理存量膨胀标签
3. 无需回滚方案——清理操作可逆（标签标记为 inactive/merged，不删除数据）
4. 监控日志中的清理计数，确认不在预期范围内的清理后调整阈值

## Open Questions

- 信息增益的 Jaccard 阈值 0.7 和树叶比 1.5 是否合理？需要观察清理任务首次运行的输出后调整
- `Slugify` 改动是否影响 embeddings（文本变了，需要重新生成对应 tag 的 embedding？→ 清理任务会处理）
