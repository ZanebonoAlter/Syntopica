## Why

标签层级系统的 abstract 根节点无限膨胀（"美伊"相关 231 个 abstract，227 个孤儿），根因是 abstract 创建没有边界约束，三个来源（findOrCreateTag / PlaceTagInHierarchy / ReviewHierarchyTrees）独立创建 abstract 且缺乏协调。需要引入 `board_concept` 作为"围栏"，将 abstract 创建限定在 concept 边界内，同时重写放置逻辑为通用的 depth-based N 层泛化设计。

## What Changes

- **BREAKING**: 全量删除现有 abstract 标签、层级关系、narrative boards、board concepts，从零重建
- **BREAKING**: `board_concepts` 表的 `is_active` 字段替换为 `status` 字段（pending/active/inactive/merged）
- **BREAKING**: `board_concepts` 新增 `category` 字段（event/keyword/person），concept 按 category 隔离
- **BREAKING**: `topic_tags` 新增 `concept_id` 可空字段，仅 abstract 标签关联 concept
- **BREAKING**: Source A（findOrCreateTag）删除 abstract 创建路径，只保留 merge 和新建标签
- 新增 `domain/concept/` 独立包，从 narrative 包迁移 concept 相关代码（matcher、service、handler、embedding、bootstrap）
- 新增 concept bootstrap：pgvector embedding 聚类 + LLM 命名，仅手动触发
- 重写 PlaceTagInHierarchy 为通用 depth-based 放置，先 MatchTagToConcept 再在 concept 围栏内放置
- 新增 anchor 机制：cotag + embedding 信号，新标签跟随已放置标签的 parent
- 全局 `maxHierarchyDepth=4` 替换为 per-template depth 上界
- 废弃 Level 概念，代码内部全用 depth
- 新增 1 小时间隔的 placement scheduler（RetryOrphanPlacements + AggregateOrphanTags）
- 新增空心 abstract 回收机制
- 关停 ReviewHierarchyTrees 的 abstract 创建能力，保留 merge/move/复用
- Prompt 函数泛化为 `buildMatchPrompt` + `buildCreationPrompt`

## Capabilities

### New Capabilities
- `concept-fence`: concept 围栏机制——concept 按 category 隔离，abstract 创建限定在 concept 边界内，bootstrap 聚类生成 pending concept
- `depth-based-placement`: 通用 depth-based N 层放置——废弃 Level 概念，anchor + abstract embedding 两层信号，per-template depth 上界
- `anchor-signal`: anchor 信号机制——cotag 优先、embedding 兜底、多 anchor 分歧时 LLM 投票
- `concept-package`: 独立 concept 包——从 narrative 迁移 concept 代码到 domain/concept/

### Modified Capabilities
- `board-concept-management`: status 字段替代 is_active；新增 category 字段；concept embedding 生成复用已有标签 semantic embedding
- `tag-hierarchy-quality`: Source A 删除 abstract 创建；Source C 关停新建 abstract；通用 dedup；空心 abstract 回收
- `tag-to-board-matching`: MatchTagToConcept 迁移到 concept 包；复用已有 semantic embedding 而非重新生成

## Impact

- **数据层**: board_concepts 表 schema 变更（+category, +status, -is_active）；topic_tags 表新增 concept_id；全量数据清理
- **代码层**: 新增 domain/concept/ 包；tagging 包重写 hierarchy_placement/hierarchy_prompts/hierarchy_dedup；新增 hierarchy_aggregation；tagger.go 删除 abstract 创建路径；narrative 包移除 concept 相关文件
- **API 层**: concept API 路由从 /api/narratives/board-concepts 迁移；新增 POST .../bootstrap 端点
- **调度器**: 新增 1h 间隔 placement scheduler；24h scheduler 的 Phase 6 关停 abstract 创建
- **依赖**: 无新外部依赖，聚类使用 pgvector 原生能力
