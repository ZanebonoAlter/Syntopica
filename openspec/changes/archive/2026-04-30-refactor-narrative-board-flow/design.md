## Context

当前叙事系统存在三层问题：

1. **后端**：`GenerateAndSaveForCategory` 根据未分类事件数量分治为高量模式（>5 个事件标签走 LLM 分 Board + Board 内叙事）和低量模式（≤5 个事件标签走直出，无 Board）。抽象标签树在低量模式下被打包给 LLM 自由组合叙事，树的边界被打破——不会单独成为 Board。

2. **前端**：`NarrativePanel.vue`（848 行）同时维护两套独立的 mode/scope 状态（`boardMode`+`boardScopeMode`/`boardSelectedCategoryId` vs `scopeMode`+`selectedCategoryId`），维护了两套渲染组件（`NarrativeBoardCanvas` 和 `NarrativeCanvas`），切换时状态不同步。

3. **数据**：低量直出的叙事没有 `board_id`，前端只能通过 legacy 时间线视图展示，无法融入三层导航。

目标：用抽象标签树作为 Board 的确定性划分依据，消除高/低量分治，清理前端双模式。

## Goals / Non-Goals

**Goals:**
- 每棵活跃的抽象标签树 → 1 个 Board（确定性，无 LLM）
- 未归入任何抽象树的零散事件 → 杂项 Board（≤3 直接合并，>3 LLM 分组）
- 删除 `generateDirectForLowVolume`，所有叙事必须属于一个 Board
- 删除 `source="abstract"` 的叙事卡片，Board 本身就代表抽象树
- `prev_board_ids` 通过抽象标签 ID 确定性匹配，不依赖 LLM
- 前端只保留版块模式，删除时间线模式，统一一套作用域状态
- `TopicGraphPage.vue` 中叙事标签点击不跳转到图谱，保持在叙事上下文

**Non-Goals:**
- 不改变 `NarrativeBoard` 表的核心结构（仅加一个字段）
- 不修改 Board Timeline / Scopes 查询 API
- 不修改 `MergeGlobalBoards` 跨分类合并逻辑
- 不修改 `tag_feedback.go` 叙事驱动标签聚类
- 不修改 `TagHierarchyCleanupScheduler` 10阶段清理

## Decisions

### D1: Board 创建从 LLM 分组改为确定性映射

**选择**：通过 `CollectAbstractTreeInputsByCategory` 获取活跃抽象树，每棵树创建一个 Board，Board 名称和描述直接来自抽象标签的 label 和 description。

**原因**：
- 标签系统已经通过 `MatchAbstractTagHierarchy` 做了大量 LLM 聚合工作，叙事层不应该再次拆分
- 确定性映射消除了分治逻辑，前端可以统一展示
- Board 名称从前可能是 LLM 临时命名，现在和抽象标签一致，可追溯

**替代方案**：继续让 LLM 对抽象树 + 事件一起分组。
- **拒绝理由**：抽象树的边界是标签系统确定好的，LLM 再次拆分可能产生不一致。

### D2: 杂项事件的 Board 分组策略

**选择**：
- ≤3 个未分类事件 → 1 个"其他动态" Board（无 LLM）
- >3 个未分类事件 → LLM 分组（复用并简化 `board_generator.go` 的分组逻辑）

**原因**：
- ≤3 个事件太少，建多个 Board 没有意义
- >3 个事件可能形成多个自然分组，LLM 能较好识别

**替代方案**：所有杂项事件都归入 1 个 Board。
- **拒绝理由**：事件多时混在一起，叙事质量下降。

### D3: 删除 `source="abstract"` 卡片

**选择**：不再为抽象标签生成独立卡片叙事。Board 本身就是抽象树的视觉表示，Board 内的 AI 叙事会基于树结构生成。

**原因**：
- 当前 abstract 卡片是把抽象标签描述当叙事 summary，标题是其 label，实际不够叙事性
- Board 自身携带了抽象标签的描述，LLM 生成叙事时可以作为上下文
- 减少 DB 中不必要的中间记录

**受影响代码**：`generateAbstractTagCardsForBoards()`、`mapAbstractTagToNarrativeCard()`、`computeAbstractTagStatus()` 全部删除。

### D4: `prev_board_ids` 匹配策略

**选择**：通过抽象标签 ID 确定性匹配。今日 Board 的 `abstract_tag_id` = 昨日某个 Board 的 `abstract_tag_id` → 匹配成功。

**实现**：
```sql
SELECT id FROM narrative_boards
WHERE abstract_tag_id = ? AND period_date = ?
```

**原因**：精确匹配，消除 LLM 推测的不确定性。杂项 Board 没有 `abstract_tag_id`，通过名字相似度匹配（备选）。

### D5: 前端只保留版块模式

**选择**：删除 `boardMode` 切换，删除 `NarrativeCanvas.client.vue`，`NarrativePanel.vue` 只渲染 `NarrativeBoardCanvas`。

**原因**：
- 版块模式已经覆盖了全部场景（所有叙事都在 Board 内）
- 时间线模式是被标记为"legacy"但未清理的技术债
- 简化后约 400 行，维护难度大幅降低

### D6: 叙事标签点击行为

**选择**：叙事详情卡片中的标签点击不再无条件跳转到图谱视图。改为：
- 抽象标签 → 如果当前日期的 Board 中有该标签对应的 Board，展开它
- 其他标签 → 发射 `select-tag` 事件（保持现有行为，但不改 Tab）

**原因**：用户点击叙事内标签是希望了解更多上下文，而非中断叙事浏览流程。

## Data Model Change

```sql
ALTER TABLE narrative_boards ADD COLUMN abstract_tag_id INTEGER
    REFERENCES topic_tags(id) ON DELETE SET NULL;
```

杂项 Board 的 `abstract_tag_id` 为 NULL。

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| 杂项 Board 分组 LLM 质量不稳定 | prompt 只接收事件标签（不含抽象树），上下文更聚焦 |
| 抽象树有但当天无活跃事件 → 无 Board | 正确行为，不应该为活跃度为 0 的树生成空白 Board |
| 两个抽象树高度相关但被分到不同 Board | 标签系统负责合并抽象树（`MergeTags`），叙事层不处理 |
| 历史叙事（无 `board_id`）在重构后查询不到 | `RegenerateAndSave` 会先删除再重建，下次重生成时自然消失 |

## Open Questions

- 杂项 Board 的 `abstract_tag_id` 为 NULL，跨日延续时如何匹配？可用名字（如"其他·半导体供应链"）匹配，或直接不匹配依赖 LLM 推测 `parent_ids`
