## Purpose

**DEPRECATED** — 已被 `semantic-label-model` + `board-management-api` + `board-upgrade` 替代。本 spec 描述的 board_concepts 表、embedding 直接匹配、LLM cold-start 建议等已全部废弃。保留仅供参考历史版本。

## Requirements

### Requirement: Board concept persistence (DEPRECATED)
已被 `semantic-label-model` 中的 `semantic_labels` 统一数据模型替代。board_concepts 表已删除。

### Requirement: Board concept LLM cold-start suggestion (DEPRECATED)
已被 `board-upgrade` 中的辅助标签聚类升级流程替代。

### Requirement: Board concept user CRUD (DEPRECATED)
已被 `board-management-api` 中的板块 CRUD API 替代。

### Requirement: Board concept embedding generation (DEPRECATED)
已被 `semantic-label-model` 中的双 embedding 字段（merge_embedding + storage embedding）替代。
