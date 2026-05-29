## Why

事件维度冷启动阶段聚合效果差：244 个 event topic tag 中 240 个（98.4%）没有 abstract parent，全部孤立。根因是 `ClusterUnclassifiedTags` 从未被 scheduler 调用（死代码），且现有的 semantic-only 相似度对事件聚合区分度不足（同事件链 0.72~0.86，与不相关事件 0.70~0.85 重叠）。而已有但未被利用的 `event_keywords` 元数据对同事件链 tag 对表现出强信号（shared_kws>=2 时高质量聚合），可以与 semantic embedding 构成两阶段过滤，显著提升事件聚合召回率和精确度。

## What Changes

- 新增两阶段事件聚类函数：Stage 1 关键词文本交集召回（shared_kws >= 2），Stage 2 semantic embedding 过滤（sim >= 0.80）
- 将 `ClusterUnclassifiedTags` 集成到 `tag_hierarchy_cleanup` scheduler 的新 Phase 中（仅 event category）
- 新增 `embedding_config` 配置项控制聚类参数
- 前端同步展示聚类效果（事件聚合状态面板）

## Capabilities

### New Capabilities

- `event-keyword-clustering`: 两阶段事件 tag 聚类机制——关键词交集召回 + semantic 过滤，集成到 scheduler 定期执行

### Modified Capabilities

- `tagging-domain`: 新增 keyword-overlap 聚类路径，扩展 `ClusterUnclassifiedTags` 对 event category 的处理逻辑
- `tag-hierarchy-quality`: cleanup scheduler 新增 Phase，触发 event clustering

## Impact

- **后端**: `tag_clustering.go`（核心变更）、`tag_hierarchy_cleanup.go`（新增 Phase）、`embedding.go`（新增查询函数）、`embedding_config`（新增配置项）
- **前端**: 事件聚合状态展示（可选，低优先级）
- **LLM 调用**: 预计每轮 ~49 对进入 LLM judgment，在现有 budget 内可控
- **数据**: 无 schema 变更，纯逻辑层改动
