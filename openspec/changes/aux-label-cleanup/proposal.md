## Why

辅助标签（auxiliary label）数量随时间单调增长，缺乏任何清理机制。每次 tag 提取和 board 匹配操作都需要加载全部 active 辅助标签进行 embedding 比较，随着标签数量增加，匹配性能持续退化。同时 ref_count 只增不减，即便引用的 topic_tag 已被删除，导致数据失真。

## What Changes

- **修正 ref_count 递减逻辑**：topic_tag 被删除（CleanupOrphanedTags / HardMergeTags）时，自动重算受影响 auxiliary label 的 ref_count
- **新增辅助标签 GC 机制**：定时扫描无活跃 topic_tag 引用的 auxiliary label，自动将其标记为 disabled
- **新增手动 GC API**：`POST /api/auxiliary-labels/gc`，支持 dry_run / disable / delete 三种模式
- **新增 AuxLabelCleanup 定时任务**：每小时执行一次 disable 模式的 GC，通过 GlobalSettings 定时任务面板展示和管理
- **一次性存量校准脚本**：对现有数据执行 ref_count 全量重算

## Capabilities

### New Capabilities
- `aux-label-gc`: 辅助标签垃圾回收——定时自动 + 手动按需清理无活跃引用的辅助标签

### Modified Capabilities
- `semantic-label-model`: ref_count 自动维护需求原本规定了「增减」，但实际只实现了「增」，本次补齐「减」的逻辑

## Impact

- **后端**：`CleanupOrphanedTags`、`HardMergeTags` 增加 ref_count 重算调用；`AuxiliaryLabelService` 新增 `RecountRefs`、`GC` 方法；新增 `AuxLabelCleanupScheduler` 定时任务
- **前端**：`schedulerMeta.ts` 新增 aux_label_cleanup 的展示配置；`auxiliaryLabels.ts` 新增 GC API 调用
- **API**：新增 `POST /api/auxiliary-labels/gc`
- **数据库**：无 schema 变更，仅数据修正
- **存量数据**：需手动执行一次性校准脚本
