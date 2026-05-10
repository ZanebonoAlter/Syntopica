## Context

叙事整理的前端分类版块交互和后端空 board 清理存在问题：

- `NarrativePanel.vue` 的 `switchScope('category')` 不调用 `loadScopes()`，导致分类列表永远为空
- `GetScopes` API 从 `narrative_summaries` 聚合分类，但 boards 可能先于 summaries 存在
- board 创建采用"先入库再尝试填充叙事"的两步设计，跳过/失败时 board 未被回删

涉及代码：
- 前端：`front/app/features/topic-graph/components/NarrativePanel.vue`、`front/app/api/topicGraph.ts`
- 后端：`backend-go/internal/domain/narrative/service.go`（GetScopes、GenerateAndSaveForCategory、GenerateAndSave）
- 类型：`front/app/api/topicGraph.ts`（NarrativeScopeCategory 接口）

## Goals / Non-Goals

**Goals:**
- 点击"分类版块"后正确展示有数据的分类列表
- 每个分类行显示 board 数量，点击钻取查看该分类下的 boards
- 生成结束后自动清理无叙事关联的空 board

**Non-Goals:**
- 不重构分类版块的整体交互模式
- 不改动 NarrativeBoardCanvas 的渲染逻辑
- 不处理存量空 board 的独立清理命令（只在新一轮生成时清理）

## Decisions

### D1: switchScope 加 loadScopes 调用

在 `switchScope` 函数中，当 mode 为 `'category'` 时调用 `loadScopes()`。最小修改，一行代码。

**替代方案**：从 boardTimelineDays 前端派生分类列表——需要额外获取 category 元数据，增加前端复杂度，不值得。

### D2: GetScopes 数据源改为 boards

将 `GetScopes` 的聚合查询从 `narrative_summaries` 改为 `narrative_boards`：
- `COUNT(*)` 统计 board 数而非 narrative 数
- 仍 JOIN `categories` 表获取 name/icon/color
- 字段名从 `narrative_count` 改为 `board_count`

前端 `NarrativeScopeCategory` 接口和模板同步调整。

**替代方案**：保留 summaries 查询额外加 boards 查询合并——多余复杂度，boards 才是用户关心的实体。

### D3: 生成结束后统一清理空 board

在 `GenerateAndSaveForCategory` 末尾和 `GenerateAndSave` 的全局清理阶段，执行：
```sql
DELETE FROM narrative_boards
WHERE id NOT IN (SELECT DISTINCT board_id FROM narrative_summaries WHERE board_id IS NOT NULL)
```
按当天日期范围限定删除范围。

**替代方案**：在每个跳过/失败点立即删除——分支多，易遗漏。统一兜底更可靠。

### D4: GetScopes 查询时间范围

当前 `GetScopes` 只查当天（单日）。改为查询 board timeline 相同的 7 天范围，使分类列表反映可见时间范围内的 boards。

前端调用时传入 `days` 参数，与 `loadBoardTimeline` 保持一致。

## Risks / Trade-offs

- **[字段重命名]** `narrative_count` → `board_count` 是 API 契约变更 → 此 API 仅前端消费，同步修改即可
- **[空 board 删除]** 删除是不可逆操作 → 只删除无 narrative 关联的 board，有叙事的 board 不受影响
- **[GetScopes 时间范围]** 从单日改为 7 天 → 返回的分类可能更多，但更符合用户看到的 timeline 范围
