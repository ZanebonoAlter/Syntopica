## Context

当前标签系统有三条互相独立创建 Node 的路径 (Source A: ExtractAbstractTag 实时路径, Source B: PlaceTagInHierarchy 异步路径, Source C: ReviewHierarchyTrees 定时路径)，以及一套不考虑 template 约束的清理机制。导致：

1. Phase 2.5 聚类创建的 Node 被 Phase 3 立刻清掉 (自毁循环)
2. Concept fence 的冷启动鸡生蛋：无 board_concepts → PlaceTagInHierarchy 返回 "no_matching_concept" → 标签不放置 → 无数据用于 concept 生成
3. topic_tags.status 的 merged/inactive 软状态增加了全链路复杂度，查询需要额外过滤
4. 前端层级管理分散在 TopicGraphPage (hierarchy tab) 和 GlobalSettingsDialog (hierarchy tab) 两个入口，无法闭环操作

约束：
- 单用户系统，无并发冲突
- PostgreSQL + pgvector 持久层
- LLM 调用需限流和成本控制
- 前端 Nuxt 4 + Vue 3 Composition API

## Goals / Non-Goals

**Goals:**

- 统一 Node 生命周期：唯一入口 PlaceTagInHierarchy，唯一终态 DELETE（无软状态）
- Sector (board_concept) 支持三种生成模式，解决冷启动问题
- 清理机制完全 template-aware，不再与层级生长矛盾
- Template 变更支持异步全量重建，带限流、断点续传、进度汇报
- 前端 /tags 页面统一所有层级管理操作
- JSON 序列化通过 LLM 参数传递，不靠 prompt 软约束

**Non-Goals:**

- 不改变 tag_jobs / embedding_queues 的提取和向量化流程
- 不改变叙事生成的 board 热点板匹配逻辑（只在输入侧受益于更干净的层级树）
- 不做跨 category 的 Tag 归属（Tag 只属于其 category 下的 Sector）
- 不做 Tag 级别的人工创建/编辑（Tag 只由 LLM 从文章提取）
- 不改变文章阅读反馈和偏好聚合链路

## Decisions

### D1: Node/Tag 合并后源直接 DELETE

**决策**: 合并时将 article_topic_tags 重指向 target，然后 DELETE 源 Tag/Node 及其所有关联行（topic_tag_relations, topic_tag_embeddings, article_topic_tags 中对源的引用）。

**理由**: 消除 merged/inactive 软状态，简化查询（不需要 `WHERE status='active'`），避免 "僵尸" 标签累积。叙事摘要允许引用断裂。

**替代方案**: 保留 merged 状态供历史追溯。被否决因为单用户系统不需要审计追踪，软状态增加的复杂度远大于收益。

### D2: Sector 作为顶层围栏，对应 template 的最外层

**决策**: Sector (board_concept) 只对应 Category 的顶层分组，不嵌套。一个 Category 下有 N 个 Sector，每个 Sector 内按 Template 的 Level 定义生长层级树。

**理由**: 避免 Sector 嵌套导致的递归复杂度。Sector 的语义是 "主题围栏" — 把相关标签圈在一起，内部层级由 Template 定义。

### D3: 三种 Sector 生成模式

**决策**:
- **Auto**: TagHierarchyPlacementScheduler 检测 unplaced Tag 数 > `auto_sector_threshold` 时自动触发 LLM 生成，review 阈值 0.85 过滤重复
- **LLM**: 用户点击 "重新生成板块"，LLM 输出增量建议 (保留/新增/合并/拆分)，展示 diff 后用户确认执行
- **Manual**: 用户输入 label + 可选 description，LLM 补全 description，创建 protected Sector

**理由**: Auto 解决冷启动；LLM 模式给用户控制力；Manual 满足确定性需求。protected 标记防止自动模式误删用户意图。

**替代方案**: 只有 Auto + Manual，去掉 LLM 模式。被否决因为 LLM 模式能根据内容趋势动态调整，是核心价值。

### D4: 聚类不直接创建 Node

**决策**: ClusterUnclassifiedTags 的输出改为聚类信号（哪组 Tag 应归在一起），作为 anchor 输入 PlaceTagInHierarchy。聚类本身不创建 Node，不修改 topic_tag_relations。

**理由**: 统一 Node 创建入口，避免聚类产出的 Node 被清理机制立刻删除。聚类负责 "发现关联"，PlaceTagInHierarchy 负责 "建立关系"。

### D5: Template 变更触发的全量重建

**决策**: 用户确认 Template 变更后：
1. DELETE 该 Category 所有 `topic_tag_relations` (relation_type='abstract')
2. DELETE 该 Category 所有 `topic_tags` (source='abstract')
3. DELETE 相关 `topic_tag_embeddings`
4. 创建 `rebuild_jobs` 记录
5. 异步 job 按 batch 重新 PlaceTagInHierarchy 所有 leaf Tag

展示：影响 Tag 数 + 预估耗时（基于 ai_call_logs 历史平均 placement 时间）。重建期间层级视图显示进度条。

**理由**: Template 变更是低频操作，全量重建保证层级树与 template 完全一致。增量修补会导致新旧层级定义混杂。

**替代方案**: 增量迁移 — 只移动超出新 template 深度的标签。被否决因为 Level 语义变化时增量逻辑极复杂（比如把 3 层改成 2 层，中间层的语义完全不同）。

### D6: rebuild_jobs 表 + 断点续传

**决策**: 新建 `rebuild_jobs` 表记录异步重建任务。每个 batch 处理完后更新 `processed_tags` 和 `last_tag_id`（游标）。重启时从 `last_tag_id` 继续。通过 WebSocket 推送进度。

**限流**: 每个 batch 之间 sleep 可配置间隔（默认 1s），每个 batch 大小默认 20。避免打爆 LLM API。

### D7: /tags 页面统一入口

**决策**: 新建 `/tags` 页面（或 `/topic-tags`），包含：
- 左面板：Sector 列表 + 管理（添加/删除/LLM 重新生成）
- 右面板：层级树（选中 Sector 或全部）
- 底栏：重建进度 + PendingChange 计数
- 模板设置弹出框（从左栏按钮触发）

从 TopicGraphPage 移除 hierarchy tab，从 GlobalSettingsDialog 移除 hierarchy tab。

**理由**: 用户操作集中，减少认知负担。TopicGraphPage 回归 "探索" 定位（图谱可视化 + 叙事阅读），/tags 回归 "管理" 定位。

### D8: 清理机制重写

**决策**: 新的 Phase 顺序和逻辑：

| Phase | 功能 | 行为 |
|-------|------|------|
| 1 | 僵尸 Tag 清理 | 无文章、无关系、age > 7d → DELETE |
| 2 | 低质量 Tag 清理 | quality_score < 0.15 且 article_count = 1 → DELETE |
| 3 | 空 Node 清理 | 无子节点的 Node → DELETE |
| 4 | 同 Level 去重 | 同 Sector 同 Level 的 Node，embedding 相似 > 0.85 → 合并（源 DELETE） |
| 5 | Template 校验 | 检测违反 Template 约束的关系 → 生成 PendingChange |
| 6 | Sector 健康检查 | auto: 空→DELETE; llm: 衰退→标记; manual: 不动 |
| 7 | 聚类 (template 内) | unplaced Tags 聚类 → anchor 信号 |

**变更**: 移除旧的 CleanupSingleChildAbstractNodes（template-aware 后单子节点可能是正常的）、移除 CleanupStaleZeroScoreTags（被 Phase 2 覆盖）、移除 CleanupTemplateViolations 的旧实现（用 Phase 5 替代）。

### D9: JSON 序列化通过参数传递

**决策**: 所有 LLM 调用中涉及 JSON 格式要求的，通过 function calling / structured output / JSON mode 参数传递。不在 system prompt 或 user prompt 中描述 "请以 JSON 格式返回"。

**理由**: prompt 内的格式要求是软约束，LLM 可能不遵守。参数级别的约束由 API 层保证。

## Risks / Trade-offs

- **[风险] 全量重建耗时** → Tag 规模大时（5000+）重建可能超过 30 分钟。缓解：异步 + 进度条 + 断点续传，用户可离开。限流避免打爆 API。
- **[风险] Auto 模式生成的 Sector 质量不可控** → 缓解：review 阈值 0.85 过滤重复；生成的 Sector 如果持续无 Tag 归属，下次健康检查会被删除。
- **[风险] Template 变更后重建期间层级树为空** → 缓解：重建期间 UI 显示进度条，明确告知用户正在重建。
- **[风险] 删除旧清理代码可能遗漏边界情况** → 缓解：Phase 1-2 保留僵尸/低质量 Tag 清理（这些逻辑稳定），只重写 Node 相关的清理。
- **[Trade-off] 叙事引用断裂** → 合并/删除 Tag 后叙事中的 ID 引用失效。可接受因为叙事是时间快照，不影响新叙事生成。
- **[Trade-off] /tags 新页面开发量** → 需要新页面 + 多个子组件，但比散落在两个入口的维护成本低。
