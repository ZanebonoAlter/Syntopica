## Context

当前标签层级通过 `topic_tag_relations` 表的 `parent_id → child_id` 关系链构建，最大深度 4（`maxHierarchyDepth`）。标签创建时走 `findOrCreateTag → callLLMForTagJudgment → ProcessJudgment` 流程，LLM 使用二元判断（merge/abstract/none），不带层级上下文。每次判断是局部的：只看当前标签和候选，不知道目标层级应该有什么语义。

结果：实际数据显示深度可达 7 层，同级标签被错误链成父子，跨分类关系（event 挂在 keyword 下）频繁出现。例如 `企业动态与商业融资 > OpenClaw平台动态 > ... > AutoGaze技术分享会`（7层 event），各层都是"事件"，没有粒度区分。

## Goals / Non-Goals

**Goals:**
- 引入固定层级模板，每个 category 有严格定义的层级序列和语义
- 新标签默认叶子节点，通过向上聚合逐层查找/创建父标签
- 层级通过路径深度反推（不改数据库 schema）
- 模板层级可配置（调整层级的名称、描述、约束参数）
- 配置变更安全：生成待处理清单，用户手动触发 rebuild
- L1/L2 去重：embedding 优先，LLM 兜底
- 现有清理调度器 Phase 3/4/6 对齐模板约束

**Non-Goals:**
- 不支持新增/删除模板（模板数量固定）
- 不改 `topic_tags` 表结构（无 `abstraction_level` 列）
- 不作权限管理（单用户应用）
- 不自动修复配置变更导致的层级违规

## Decisions

### 决策 1: 方案 B — 深度反推层级，不改表

通过 `getTagDepthFromRoot` 获取路径深度，对照模板层数映射到抽象层级。例如 event 模板 3 层 → depth=1 是 L1，depth=2 是 L2，depth≥3 是 L3。不新增数据库列，避免迁移复杂性和对现有清理逻辑的冲击。

替代方案 A（新增 `abstraction_level` 列）因需要数据迁移且需同步维护深度和列值而被否决。

### 决策 2: 固定模板，只调层级

5 个固定模板（event、person、keyword:technology、keyword:company_business、keyword:concept），模板名称和数量不可变更。层级定义（名称、描述、最大子标签数、是否叶子、禁止模式）可配置。配置存储为 `hierarchy_config` 表的单条 JSONB 记录，减少运维复杂度。

替代方案（完全开放的模板系统）因项目单用户、需求稳定而过度设计。

### 决策 3: 向上聚合流程 — 默认 L3，逐层上溯

```
新标签到达 → 默认 L3 叶子
  → Step 1: embedding 搜 L2 候选池 → 匹配/创建 L2 父标签
  → Step 2: 递归处理 L2 标签，向上找 L1 父标签
```

L2 匹配阈值：
- similarity ≥ 0.85 → 直接挂载
- similarity 0.60-0.85 → LLM 选择（选一个候选或创建新标签）
- similarity < 0.60 → 创建新 L2 标签

L1 匹配同上，但阈值放宽到 0.80/0.55。

Person 例外：只有 2 层，L2 就是叶子，跳过 Step 1。

### 决策 4: Embedding 优先去重，LLM 兜底

L1 去重：创建新 L1 后查 embedding 相似度 > 0.90 的已有 L1，存在时 LLM 判断是否合并。
L2 去重：创建新 L2 后查 embedding 相似度 > 0.95 的已有 L2，存在时直接合并（不调 LLM）。

原因：L2（事件主体/公司名）更容易通过 embedding 精确区分；L1（事件类型）语义较模糊需要 LLM。

### 决策 5: 配置变更安全机制

修改配置后：
1. 自动扫描所有现有标签，找出违反新规则的（深度超限、层级不匹配）
2. 生成 `hierarchy_pending_changes` 记录
3. 不自动修复
4. 用户通过 UI 查看待处理清单，手动触发 `POST /api/hierarchy/rebuild`
5. rebuild 对每个待处理标签重新走放置流程

### 决策 6: LLM Prompt 注入层级上下文

修改 `buildTagJudgmentPrompt` 和 `batchJudgeAbstractRelationships`，在 prompt 中注入当前层级定义和已有同级标签作为 few-shot 参考。不再问"谁是父谁是子"，而是"在目标层级中选最合适的父标签，或创建新的"。

### 决策 7: Person 简化为 2 层

L1: 人物群组（LLM 自由生成，如"AI研究者"、"中国科技界人物"）
L2: 具体人物（叶子）

国家/领域/角色等维度的元信息预留用 `metadata` JSONB 字段存储，但不在层级结构中体现。这样可以避免 4 层结构中中间层的模糊性。

## Risks / Trade-offs

- **风险**: 深度反推在层数变更后可能产生错误映射 → 缓解：rebuild 时重新计算所有标签的深度和层级
- **风险**: 大量标签需要重新挂载时 rebuild 耗时 → 缓解：支持按 category 过滤、WebSocket 进度广播
- **风险**: L1/L2 embedding 去重可能漏掉语义重复但向量距离远的标签 → 缓解：Phase 6 抽样 LLM 复查
- **权衡**: 深度反推不如显式列精确，但避免了 schema 迁移的复杂性
- **权衡**: 固定模板不如完全开放灵活，但匹配项目的稳定需求和单用户场景

## Open Questions

1. rebuild 后是否需要通知前端刷新？（默认：WebSocket 广播 progress）
2. Person 的 `metadata.country/role/domains` 字段何时由谁填充？（当前 discussion 范围外，后续跟进）
