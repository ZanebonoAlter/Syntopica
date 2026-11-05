# UI Design: speed-up-tag-embedding

## UI Impact

`<!-- ui-impact: none -->`（见 proposal.md 头声明）

本 change 为纯后端性能修复（索引/队列/批量/可观测），不改任何页面布局、组件与交互。

## Minimal N/A

无 UI 改动。唯一用户可感知变化：标签向量生成变快、`/api/embedding/queue/status` 返回体新增两个只读字段（`oldest_pending_age_seconds` / `throughput_per_min`），为纯增量数据字段，无界面消费方需同步（前端 status 面板读取 map 结构，新增 key 不破坏现有渲染）。
