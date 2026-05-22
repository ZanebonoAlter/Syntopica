# Semantic Label Board System — 阶段 11 文档更新实现计划

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 更新 docs/reference 下的 5 个文档，反映 semantic label/board 新体系，标记/删除旧体系内容。

**Architecture:** 纯文档修改任务，无代码变更。每个文档文件对应一个 task，按照 design.md 和 delta specs 中的新模型替换旧内容。

**Tech Stack:** Markdown 文档编辑

---

## 上下文摘录

### 新模型核心概念
- `semantic_labels` 统一表存储辅助标签 (label_type=auxiliary) 和 SemanticBoard (label_type=board)
- `topic_tag_semantic_labels` 关联 tag → auxiliary label
- `topic_tag_board_labels` 持久化 tag → SemanticBoard 匹配结果 (含 score, match_reason)
- `board_composition` 记录 board 由哪些 auxiliary label 组成
- 新链路：LLM 提取 tag 时同时生成 3-5 个辅助标签 → 入库(L1 slug匹配/L2 embedding合并/L3 新建) → 辅助标签匹配到 SemanticBoard → SemanticBoard 派生 NarrativeBoard
- 旧体系完全删除：hierarchy_template, abstract_tag, board_concepts, topic_tag_relations, 层级清理7阶段

### 新 API 路由
- `/api/semantic-boards` — SemanticBoard CRUD
- `/api/semantic-boards/:id/composition` — Board 构成
- `/api/semantic-boards/upgrade-candidates` — 升级候选
- `/api/semantic-boards/upgrade-suggest` — LLM 升级建议
- `/api/semantic-boards/upgrade-execute` — 确认执行
- `/api/semantic-boards/backfill` — 回填
- `/api/semantic-boards/matching-config` — 匹配参数
- `/api/auxiliary-labels` — 辅助标签池治理
- `/api/tags/:id/auxiliary-labels` — Tag 辅助标签
- `/api/tags/:id/semantic-boards` — Tag 所属 Board

### 已删除的 API 路由
- `/api/hierarchy/*` — 层级管理
- `/api/narratives/board-concepts` — 板块概念 CRUD
- `/api/narratives/unclassified` — 未分类标签桶

### 叙事生成新流程
1. 读取 active SemanticBoard (label_type=board)
2. 按 date + scope + semantic_board_id 收集匹配 event tags (来自 topic_tag_board_labels)
3. 每个 SemanticBoard 创建 NarrativeBoard (写入 semantic_board_id)
4. prev_board_ids 按 semantic_board_id + scope + 前一日续接
5. 无 SemanticBoard 或无匹配 event tags 时不生成 NarrativeBoard，不报错

---

### Task 1: 更新 DATA_LIFECYCLE.md

**TDD scenario:** 纯文档修改，无需测试

**Files:**
- Modify: `docs/reference/database/DATA_LIFECYCLE.md`

**Step 1: 重写"主题标签生命周期"部分**

将旧的 "Tag → Node → Sector → 层级放置 → 清理 7 Phase" 完整链路替换为：

```
┌─ LLM 标签提取（含辅助标签）─────────────────────────────────────────────┐
│  来源: tag_jobs 处理 (article_lifecycle 触发)                            │
│                                                                          │
│  LLM → 候选标签列表 (label + category) + 3-5 个辅助标签               │
│                                                                          │
│  INSERT INTO ai_call_logs (capability='tag_extraction', ...)            │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Embedding 去重 + 入库 ──────────────────────────────────────────────────┐
│  (同旧，不变化)                                                           │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 辅助标签入库 ───────────────────────────────────────────────────────────┐
│  对每个 tag 的 3-5 个辅助标签：                                          │
│                                                                          │
│  L1: slug/alias 精确匹配 → 复用已有 auxiliary label，ref_count++         │
│  L2: embedding ≥ 0.95 合并 → 小方 label 加入大方 aliases，ref_count++  │
│  L3: 新建 → 创建 semantic_label(label_type=auxiliary)，生成 embedding  │
│                                                                          │
│  写入 topic_tag_semantic_labels 关联                                     │
│  禁用标签 (status=disabled) 不参与 L1/L2 匹配                           │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard 匹配 ──────────────────────────────────────────────────────┐
│  读取 tag 的辅助标签和 active SemanticBoard composition                  │
│                                                                          │
│  · 直接命中: tag 的辅助标签 ∈ board 构成标签 → 挂载                     │
│  · 命中率 > 50% → 挂载                                                   │
│  · max_sim ≥ 0.8 → 挂载                                                 │
│  · 加权综合: 0.6×max_sim + 0.4×hit_rate ≥ 阈值 → 挂载                  │
│                                                                          │
│  默认最多 3 个 board，按匹配分排序                                       │
│  写入 topic_tag_board_labels (topic_tag_id, semantic_board_id, score,    │
│    match_reason)                                                          │
│                                                                          │
│  冷启动无 SemanticBoard 时：不匹配，不报错                               │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard 升级建议（手动触发）───────────────────────────────────────┐
│  收集 ref_count ≥ semantic_board_upgrade_ref_count_threshold 的候选     │
│  辅助标签 → embedding 预聚类 → 每个簇补充 co-tag 事件上下文           │
│  → LLM 建议 (create_new / merge_into_existing / skip)                   │
│  → 用户确认后写入 SemanticBoard + board_composition                     │
│  → 可触发回填重算 topic_tag_board_labels                                │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 回填队列 ───────────────────────────────────────────────────────────────┐
│  支持 all / unassigned / board 三种模式                                  │
│  异步逐个执行 Board 匹配并重写 topic_tag_board_labels                   │
│  幂等：已有归属会被覆盖                                                 │
└─────────────────────────────────────────────────────────────────────────┘
```

同时删除旧的 "Tag 归属 Sector"、"质量评分"、"层级放置"、"清理机制 7 Phase" 四个 block。

**Step 2: 更新"叙事生成生命周期"**

将叙事生成流程替换为新版本（基于 spec semantic-board-narrative）：

```
┌─ 输入收集 ───────────────────────────────────────────────────────────────┐
│  NarrativeSummary 调度器 (86400s)                                        │
│                                                                          │
│  SELECT semantic_labels WHERE label_type='board' AND status='active'    │
│  → 全局共享 SemanticBoard 列表                                           │
│  → SELECT topic_tag_board_labels 获取每个 Board 的 active event tags     │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard → NarrativeBoard 生成 ────────────────────────────────────┐
│  分类维度: global / feed_category                                        │
│                                                                          │
│  CollectSemanticBoardNarrativeInputs                                     │
│    · 按 date + scope + semantic_board_id 收集 active event tags          │
│    · 数据源为 topic_tag_board_labels 持久化匹配结果                      │
│    · category scope 通过 articles → feeds.category_id 限定文章范围       │
│                                                                          │
│  对每个有事件的 SemanticBoard:                                           │
│    · INSERT INTO narrative_boards (semantic_board_id, event_tag_ids,     │
│             period_date, scope_type, scope_category_id, scope_label)     │
│    · prev_board_ids 按 semantic_board_id + scope + 前一日匹配            │
│    · 同一 event tag 可出现在多个 NarrativeBoard                          │
│                                                                          │
│  无 SemanticBoard 或无匹配 event tags 时生成 0 个 NarrativeBoard，不报错 │
│                                                                          │
│  后处理:                                                                  │
│  → DeriveBoardConnections: 派生 Board 间关系                            │
│  → runFeedbackFromTodayNarratives: 回写标签质量反馈                     │
│  → cleanEmptyBoards: 删除无 tag 的 Board                                │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Summary 生成 ───────────────────────────────────────────────────────────┐
│  (同旧，不变化)                                                           │
└─────────────────────────────────────────────────────────────────────────┘
```

**Step 3: 更新"配置要求"和"相关文档"**

将主题标签相关配置更新为：
- `ai_settings` 中的 `semantic_board_match_*` 控制 tag → SemanticBoard 匹配
- `ai_settings` 中的 `semantic_board_upgrade_*` 控制升级建议
- `topic_tag_semantic_labels` 记录 tag → auxiliary label
- `topic_tag_board_labels` 记录 tag → SemanticBoard

叙事摘要配置更新为需要 active SemanticBoard 且当日有匹配 event tags。

**Step 4: 更新更新日志**

添加 2026-05-22 更新日志条目。

**Step 5: 保存文件**

---

### Task 2: 更新 ER_DIAGRAM.md 和 DATABASE_FIELDS.md

**TDD scenario:** 纯文档修改，无需测试

**Files:**
- Modify: `docs/reference/database/ER_DIAGRAM.md`
- Modify: `docs/reference/database/DATABASE_FIELDS.md`

**Step 1: 更新 ER_DIAGRAM.md**

需要做的修改：

1. **全局概览 ASCII 图**：移除 Hierarchy 域，新增 Semantic Label 域。Topic Tags 域移除 `topic_tag_relations`、`hierarchy_*`、`adopt_narrower_*`、`multi_parent_*`、`abstract_tag_*`，新增 `topic_tag_semantic_labels`、`topic_tag_board_labels`。Narrative 域移除 `board_concepts`，新增语义指向。

2. **Mermaid ER 图**：
   - 删除 Hierarchy 域的 Mermaid 图
   - 更新 Narrative 域：移除 `narrative_boards.abstract_tag_id`、`narrative_boards.board_concept_id`、`board_concepts` 表。新增 `semantic_labels`、`topic_tag_semantic_labels`、`topic_tag_board_labels`、`board_composition` 的 Mermaid ER 图
   - Topic Tags 域：移除 `topic_tag_relations`，新增 `topic_tag_semantic_labels`、`topic_tag_board_labels`
   - 新增 Semantic Label 域 Mermaid ER 图

3. **FK 引用矩阵**：移除旧的层级/Narrative FK（`abstract_tag_id`、`board_concept_id`、`concept_id`、各种 hierarchy FK），新增 `topic_tag_semantic_labels`、`topic_tag_board_labels`、`board_composition` 的 FK

4. **关系模式说明**：删除层级相关自引用说明，新增 `board_composition` (board_id → semantic_labels, auxiliary_label_id → semantic_labels)、`topic_tag_semantic_labels` (多对多)、`topic_tag_board_labels` (多对多带 score/match_reason)

5. **表计数**：从 38 张更新为新数量（移除层级相关 5 张 + board_concepts 1 张，新增 4 张）

**Step 2: 更新 DATABASE_FIELDS.md**

需要做的修改：

1. **完整表清单**：移除 `board_concepts`、`hierarchy_config`/`hierarchy_config_versions`、`adopt_narrower_queues`、`multi_parent_resolve_queues`、`abstract_tag_update_queues`、`hierarchy_pending_changes`、`topic_tag_relations`。新增 `semantic_labels`、`topic_tag_semantic_labels`、`topic_tag_board_labels`、`board_composition`

2. **topic_tags** 字段更新：移除 `concept_id`、`merged_into_id`（合并已改为硬删除无软状态）、`status`（只保留 active，移除 merged 描述）。新增字段说明（如果后有新字段的话）

3. **narrative_boards** 字段更新：移除 `abstract_tag_id`、`abstract_tag_ids`、`board_concept_id`。新增 `semantic_board_id INTEGER` FK → `semantic_labels.id`

4. **新增四张表的定义**：
   - `semantic_labels`：id, label, slug, embedding vector(1536), label_type, aliases jsonb, ref_count, description, display_order, source, status, protected, created_at, updated_at
   - `topic_tag_semantic_labels`：id, topic_tag_id, semantic_label_id（唯一约束复合）
   - `topic_tag_board_labels`：id, topic_tag_id, semantic_board_id, score, match_reason, created_at, updated_at（唯一约束复合）
   - `board_composition`：id, board_id, auxiliary_label_id（唯一约束复合）

5. **删除 8 张旧表的定义章节**（board_concepts、hierarchy_config、hierarchy_config_versions、adopt_narrower_queues、multi_parent_resolve_queues、abstract_tag_update_queues、hierarchy_pending_changes、topic_tag_relations）

6. **索引清单更新**：新增 4 张表的索引，移除旧索引

7. **更新日志**：添加 2026-05-22 条目

**Step 3: 保存两个文件**

---

### Task 3: 更新 backend.md 和 data-flow.md

**TDD scenario:** 纯文档修改，无需测试

**Files:**
- Modify: `docs/reference/architecture/backend.md`
- Modify: `docs/reference/architecture/data-flow.md`

**Step 1: 更新 backend.md**

1. **目录结构**：移除 tagging/analysis 子包中的层级相关文件描述，新增 semantic-board 相关文件

2. **主题图谱子系统**：完全重写，替换为新体系：
   - `tagging/` 根包：共享类型和统一入口
   - `tagging/extraction`：标签提取（含辅助标签输出）
   - `tagging/analysis`：embedding、tag 合并（源 DELETE）、辅助标签入库（L1/L2/L3）
   - `tagging/watched`：关注标签管理
   
   新增后端模块说明：
   - `semantic/` 或在 `narrative/` 下：semantic_board_matching.go、semantic_board_upgrade.go、semantic_board_backfill.go、auxiliary_label_service.go

3. **叙事摘要**：重写，移除 "双轨制 Board 创建"，替换为：
   - SemanticBoard 全局共享，NarrativeBoard 按 scope 每日派生
   - 冷启动允许无 board
   - 多 board 归属允许事件重复展示
   - Board 叙事上下文来自 SemanticBoard label/description

4. **叙事域文件清单**：更新为实际新文件列表

5. **删除旧概念**：
   - 移除 PlaceTagInHierarchy、MatchTagToConcept、概念 Bootstrap 等描述
   - 移除 Sector（board_concepts）管理描述
   - 移除层级清理 7 Phase、rebuild_jobs 描述
   - 移除"抽象标签三层保护"、"Node 生命周期"

6. **新增概念**：
   - 辅助标签入库三级匹配（L1 slug/alias、L2 embedding≥0.95 merge、L3 新建）
   - SemanticBoard 匹配三规则（直接命中、命中率、加权综合）
   - 辅助标签治理（禁用、alias 合并、composition 移除）
   - 升级建议流程（预聚类 + LLM 判断 + 用户确认）
   - 回填队列（all/unassigned/board 三种模式）

7. **API 面更新**：移除旧路由（`/api/narratives/board-concepts`、`/api/narratives/unclassified`、层级相关路由），新增 `/api/semantic-boards` 等路由

8. **数据模型重点更新**：新增 `semantic_labels`、`topic_tag_semantic_labels`、`topic_tag_board_labels`、`board_composition`，移除旧模型

**Step 2: 更新 data-flow.md**

1. **叙事数据流**：替换为新的 SemanticBoard 派生流程

2. **删除旧流程**：
   - Board Concept 管理数据流（auto/LLM/manual 三模式）
   - 概念 Bootstrap 流程
   - 层级清理流程
   - 标签层级闭环状态机
   - 重建任务流程

3. **新增流程**：
   - 辅助标签入库流程（L1/L2/L3 三级匹配）
   - SemanticBoard 匹配流程（三规则 + 回写 topic_tag_board_labels）
   - 升级建议流程（手动触发 → 聚类 → LLM → 确认）
   - 回填数据流

4. **定时任务链路更新**：移除层级清理、Sector 生成、重建任务，新增辅助标签积累/升级提醒

**Step 3: 保存两个文件**

---

### Task 4: 更新 API 索引和 topic-graph 文档

**TDD scenario:** 纯文档修改，无需测试

**Files:**
- Modify: `docs/reference/api/_index.md`
- Modify: `docs/reference/api/topic-graph.md`

**Step 1: 读取 topic-graph.md 确认需修改的内容**

**Step 2: 更新 _index.md**

在索引表中：
- `topic-graph.md` 描述更新，移除旧的 "标签管理、Embedding、叙事摘要" 中的层级/概念路由引用
- 确认 `semantic-boards.md` 的条目正确覆盖所有新 API

**Step 3: 更新 topic-graph.md**

读取文件后，移除其中引用的已删除路由：
- `/api/narratives/board-concepts` 相关 API
- `/api/narratives/unclassified` 相关 API  
- `/api/hierarchy/*` 相关 API
- 旧的 `/api/narratives/boards` 中的 abstract_tag_id/board_concept_id 相关描述

更新叙事相关 API 描述：
- `GET /api/narratives/boards/timeline` 现在按 semantic_board_id 组织
- Board 创建流程说明更新

**Step 4: 保存文件**

---

### Task 5: 标记旧文档为废弃并验证一致性

**TDD scenario:** 纯文档修改，无需测试

**Files:**
- Read and confirm: `docs/reference/database/DATA_LIFECYCLE.md`
- Read and confirm: `docs/reference/database/ER_DIAGRAM.md`
- Read and confirm: `docs/reference/database/DATABASE_FIELDS.md`
- Read and confirm: `docs/reference/architecture/backend.md`
- Read and confirm: `docs/reference/architecture/data-flow.md`
- Read and confirm: `docs/reference/api/_index.md`
- Read and confirm: `docs/reference/api/semantic-boards.md`
- Read and confirm: `docs/reference/api/topic-graph.md`

**Step 1: 全局搜索确认无遗漏的旧概念引用**

在所有上述文档中搜索：
- `board_concepts` → 应该全部标记为废弃或删除
- `concept_id` → 应该替换为新模型
- `abstract_tag` → 应该标记为废弃
- `hierarchy_` 相关表名 → 应该标记为废弃
- `adopt_narrower`、`multi_parent_resolve`、`abstract_tag_update` → 应该标记为废弃
- `PlaceTagInHierarchy`、`MatchTagToConcept`、`SectorGenerationService` → 应该替换为新流程

**Step 2: 对残留引用做最终清理**

如果 Task 1-4 已清除大部分引用，此步骤确认无遗漏。如发现遗漏，逐一修复。

**Step 3: 更新各文档的"更新日志"**

在每个被修改的文档末尾添加：

```
### 2026-05-22

- 语义标签/板块体系重构：移除层级体系、抽象标签、板概念旧描述
- 新增 SemanticBoard/辅助标签模型和数据流
- 更新 ER 图、字段说明、数据生命周期、后端架构、数据流、API 索引
```

**Step 4: 最终一致性校验**

逐个文件阅读确认：
1. DATA_LIFECYCLE.md：新链路完整，无旧引用
2. ER_DIAGRAM.md：表数正确，FK 矩阵完整，无旧表
3. DATABASE_FIELDS.md：表清单正确，新增4表，无旧表定义
4. backend.md：子系统描述正确，无旧概念
5. data-flow.md：流程正确，无旧流程
6. _index.md / topic-graph.md：路由正确，无旧路由
7. semantic-boards.md：与实际代码一致

**Step 5: 完成并报告**