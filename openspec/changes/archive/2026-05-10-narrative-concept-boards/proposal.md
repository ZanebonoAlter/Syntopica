## Why

当前叙事板块的 Board 层实质上是 Abstract Tag 的简单映射(一棵 abstract tree → 一个 board),Board 名称和描述直接复用 abstract tag 的 label/description,没有独立的"板块概念"。用户看到的是"叙事外面套了一层分组容器",和最初的 BBS 式板块设想("AI 前沿""编程工具"等持久概念板块)相差甚远。同时存在时区 bug 导致日期偏差一天。

## What Changes

- **BREAKING**: 新增 `board_concepts` 表,替代当前"每日重建"的 board 逻辑,板块变为跨日持久的概念实体
- 新增 LLM 扫描 + embedding 匹配的板块概念自动建议和标签分发机制
- 大 abstract tree(≥6 节点)独立成为"每日热点板",小树和未分类 event tags 通过 embedding 匹配到持久概念板
- 板块概念支持用户手动管理(增删改)和系统自动建议
- embedding 匹配阈值可通过 `ai_settings` 配置(默认 0.7)
- 修复时区 bug: `time.Parse` → `time.ParseInLocation` 统一使用本地时区
- 跨日延续: 每日热点板通过 `prev_board_ids` 串联,narratives 通过 `parent_ids` 形成分叉树

## Capabilities

### New Capabilities
- `board-concept-management`: 持久化板块概念的定义、存储、embedding 生成、用户 CRUD、LLM 自动建议
- `tag-to-board-matching`: 基于 embedding cosine similarity 将小 abstract tree 和未分类 event tags 匹配到板块概念
- `daily-hotspot-board`: 大 abstract tree(≥6 节点)自动创建每日热点板,支持跨日延续

### Modified Capabilities
- `narrative-board-generation`: 每日叙事生成流程从"按 abstract tree 分组建板"改为"大 tree→热点板 + 小 tree/event→embedding 匹配概念板"的双轨模式;global merge 逻辑替换为 embedding 匹配;新增未归类桶
- `narrative-board-frontend`: NarrativePanel 增加板概念分组视图;canvas 按 board_concept 着色;用户可管理板概念列表;显示"未归类"区域

## Impact

- **数据库**: 新增 `board_concepts` 表(含 pgvector embedding 列),`narrative_boards` 表增加 `board_concept_id` 列,`ai_settings` 表新增 `narrative_board_embedding_threshold` key
- **后端**: `internal/domain/narrative/` 下新增 board_concept service/collector,修改 `service.go` 的 GenerateAndSave 流程,废弃 `board_merge.go` 的 global merge 逻辑
- **前端**: `NarrativePanel.vue` 和 `NarrativeBoardCanvas.client.vue` 适配新的板概念分组;新增板概念管理组件
