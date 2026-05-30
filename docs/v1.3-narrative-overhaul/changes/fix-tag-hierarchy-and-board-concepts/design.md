## Context

当前系统存在 3 个独立但可合并的问题领域：

1. **Board Concepts 前后端断裂**：前端 `boardConcepts.ts` 指向 `/narratives/board-concepts`，后端仅有 `/hierarchy/concepts`。前端模型使用 `is_active: boolean`，后端返回 `status: string`。`bootstrap` 端点存在于后端但从未被 SAS 调用，`board_concepts` 表为空（0 行）。前端 `BoardConceptManager.vue` 的 `suggestConcepts()` 调用直接 404。

2. **Cleanup Phase 互食**：`tag_hierarchy_cleanup` scheduler 有 7+ 个 Phase，其中 Phase 2.5（event keyword clustering）创建 abstract 标签后，Phase 3 清理（15 empty + 6 single-child per cycle）立刻将其消除，Phase 6（template tree review）调用 LLM 审查树但 `tree_groups_created=0`。结果是 393/425 event topic 孤立。Phase 顺序和职责重叠导致系统持续自我抵消。

3. **LLM 提示词缺日期**：`ExtractionInput` 无 `PubDate` 字段，`articleContext` 仅包含 Title + Summary，`buildExtractionUserPrompt` 无日期行，`generateTagDescription` prompt 无事件时间，聚类 prompt 无时间范围。LLM 对事件时序完全无知，导致生成的 tag label/description 包含错误年份。

## Goals / Non-Goals

**Goals:**
- 前端 board concept API 路径和模型完全对齐后端，`/narratives/board-concepts` → `/hierarchy/concepts`，`is_active:boolean` → `status:string`
- 后端新增 `POST /hierarchy/concepts/suggest`：基于已有 tag label+description 让 LLM 建议新概念名称，只返回 JSON 列表不创建数据库记录
- 简化 `tag_hierarchy_cleanup` 流程为：清理 → flat merge → event clustering，移除 Phase 3d/4/5/6
- 在标签提取、描述生成、聚类判断三处 LLM prompt 中注入 `pub_date`
- 清理前端死代码（NarrativePanel.vue 中未使用的 `boardConceptsApi`）

**Non-Goals:**
- 不修改 `board_concepts` 表结构（status/is_active 字段已在 migration 中完成）
- 不恢复 `bootstrap` 前端调用或 scheduler 集成
- 不修改 `hierarchy_placement.go` 的 concept 短路逻辑
- 不向 `topic_tags` 或 `topic_tag_relations` 添加日期字段
- 不在 keyword/person 类别中注入日期上下文（仅 event 类别需要）

## Decisions

### D1: Suggest 端点设计 — 基于 tag+description，不读文章内容

**选择**: `SuggestConcepts(ctx, category)` 从 `topic_tags` 加载该类别下未匹配 concept 的 active tags，提取 label+description，发送给 LLM 建议 3-5 个概念。LLM 仅返回 `{name, description}` 建议列表，不创建 `board_concepts` 行。

**备选方案**:
- A) 复用 `BootstrapConcepts` 逻辑（聚类 + LLM 命名 + 创建 pending）→ 太重，用户未准备好自动创建
- B) 基于文章内容分析 → 数据量太大，探索阶段已否决
- C) 纯 API 返回建议，前端手动创建 ✓

**理由**: 用户明确倾向轻量、基于已有 tag 元数据。与现有 create 端点解耦，前端控制创建节奏。

### D2: Cleanup 简化 — 仅保留清理 + 聚类

**选择**: 移除 Phase 3d（模板合规检查）、Phase 4（adopt-narrower）、Phase 5（abstract-update）、Phase 6（模板树审查）。保留 Phase 1-1.7（数据清理）、Phase 3（关系清理，但移到 Phase 2 之前）、Phase 2（flat merge）、Phase 2.5（event keyword clustering）、Phase 3.5-3.6（格式清理）、Phase 7（描述回填）。

**备选方案**:
- A) 仅移除 Phase 6，保留 Phase 3d/4/5 → 这些 phase 依赖成熟的 abstract tag 生态（当前不存在），保留无意义
- B) 全部移除，只留 Phase 1 → 过于激进，event keyword clustering 是有效的新功能
- C) 当前方案 ✓

**理由**: Phase 2.5 是有价值的（19900 keyword edges → 24 clusters），但不能被后续 Phase 抵消。Phase 3 的 cleanup 应在 Phase 2 之前执行，确保聚类输入干净。

### D3: Phase 顺序调整 — Phase 3 关系清理提前

**选择**: 新顺序为：Phase 1-1.7（数据清理）→ Phase 3（关系清理）→ Phase 2（flat merge）→ Phase 2.5（event clustering）→ Phase 3.5-3.6（格式清理）→ Phase 7（描述回填）

**理由**: 先清理无效数据（zombie、空 abstract、孤儿关系），再在干净数据上执行 merge 和聚类，避免旧数据污染聚类结果。

### D4: 日期注入 — 三处 prompt 各加一行

**选择**:
1. `ExtractionInput` 添加 `PubDate string` 字段，`buildExtractionUserPrompt` 加 `发布日期: %s` 行
2. `articleContext` 构建时 prepend `[日期: 2025-05-10]` 
3. 聚类 prompt 中每个候选 tag 附带 `(最早文章: 2025-05-08, 最新: 2025-05-12)` 时间范围

**备选方案**:
- A) 仅在 extraction 加日期 → 描述生成和聚类仍缺时间上下文
- B) 使用 `time.Time` 类型而非 string → 增加序列化复杂度，LLM 接受字符串足够
- C) 三处都加 ✓

**理由**: 三处各有独立用途——extraction 影响 tag label 命名，description 影响事件释文准确性，clustering 影响事件链归属判断。`string` 类型避免时区转换问题。

### D5: 前端模型对齐 — 全面对齐后端

**选择**: `BoardConcept` 接口移除 `is_active: boolean`，新增 `status: string` 和 `category: string`。调整 API 路径为 `/hierarchy/concepts`。

**理由**: 遵循用户"向后端全面对齐"的决定。`is_active` 在模板中未直接使用，安全替换。

## Risks / Trade-offs

**[Phase 6 移除可能丢失大树群审查能力]** → Phase 6 当前 `tree_groups_created=0`，实际无产出。Phase 2.5 的 keyword-overlap 聚类覆盖了其主要功能（事件链抽象）。keyword/person 类别目前无层级需求。后续如需复杂树群审查可重新引入。

**[suggest 端点新增 LLM 调用]** → 每次 suggest 1 次 LLM 调用（~500 tokens input, ~200 tokens output），在现有 budget 内可忽略。

**[日期注入可能使 LLM 过于关注时间而非语义]** → 日期作为上下文注入而非判断依据，LLM prompt 中日期放在补充位置（"发布日期: 2025-05-10"），不影响主要判断逻辑。

**[清理 Phase 3 上移可能导致 flat merge 输入减少]** → Phase 3 清理 zombie/empty-abstract 等无效数据，上移反而增加 flat merge 的输入质量。merged_into 标签已不会出现在 active 查询中。

## Open Questions

- keyword/person 类别后续是否需要类似 event 的 keyword-aware clustering？当前 scope 仅 event
- `board_concepts` 表的 `category` 字段是否需要前端展示？当前 BoardConceptManager 不展示 category
