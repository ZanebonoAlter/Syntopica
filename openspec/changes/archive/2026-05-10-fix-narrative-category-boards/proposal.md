## Why

叙事整理的「分类版块」交互完全不可用：点击切换到分类模式后，分类列表永远为空，即使 `narrative_boards` 表中已有 `scope_category_id` 数据。同时，生成过程中会产生大量无叙事关联的空 board，未被清理，污染数据。

## What Changes

- **前端最小修复**：`switchScope('category')` 时调用 `loadScopes()` 加载分类列表
- **后端 GetScopes 改为从 boards 查询**：将数据源从 `narrative_summaries` 改为 `narrative_boards`，使有 board 但无 summary 的分类也能展示
- **后端生成结束后清理空 board**：在 `GenerateAndSaveForCategory` 末尾删除当天该分类下无任何 narrative 关联的 board

## Capabilities

### New Capabilities

- `empty-board-cleanup`: 生成结束后统一删除无叙事关联的空 board

### Modified Capabilities

- `narrative-scope-query`: `GetScopes` 数据源从 summaries 改为 boards，返回 board_count 而非 narrative_count

## Impact

- **前端**：`NarrativePanel.vue` — `switchScope` 函数加一行 `loadScopes()` 调用；分类列表 badge 显示 board 数而非 narrative 数
- **后端 API**：`GET /api/narratives/scopes` 返回值 `narrative_count` 字段语义变为 `board_count`（字段名可考虑改为 `board_count`，前端同步调整）
- **后端生成流程**：`service.go` 的 `GenerateAndSaveForCategory` 和 `GenerateAndSave` 末尾各加一步清理
- **数据**：空 board 会在下次生成后被删除，已有存量空 board 也会被清理
