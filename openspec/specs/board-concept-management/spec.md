## Purpose

**DEPRECATED** — 已被 `semantic-label-model` + `board-management-api` + `board-upgrade` 替代。本 spec 描述的 board_concepts 表、embedding 直接匹配、LLM cold-start 建议等已全部废弃。保留仅供参考历史版本。

## Requirements

### Requirement: Board concept persistence (DEPRECATED)
系统 SHALL NOT 提供 board_concepts 表持久化；概念持久化已由 `semantic-label-model` 中的 `semantic_labels` 统一数据模型替代。board_concepts 表已删除。

#### Scenario: board_concepts 表不存在

- **WHEN** 查询数据库中的 board_concepts 表
- **THEN** 该表 SHALL 不存在（迁移已删除），概念数据由 semantic_labels 统一承载

### Requirement: Board concept LLM cold-start suggestion (DEPRECATED)
系统 SHALL NOT 提供 board concept 的 LLM 冷启动建议；该能力已由 `board-upgrade` 中的辅助标签聚类升级流程替代。

#### Scenario: 冷启动建议由聚类升级流程承担

- **WHEN** 用户请求为板块生成概念建议
- **THEN** 系统 SHALL 走 `board-upgrade` 的辅助标签聚类升级建议流程，不存在 board_concepts 专属冷启动接口

### Requirement: Board concept user CRUD (DEPRECATED)
系统 SHALL NOT 提供 board concept 的用户 CRUD；该能力已由 `board-management-api` 中的板块 CRUD API 替代。

#### Scenario: 板块增删改查走 semantic-boards API

- **WHEN** 用户创建/查看/编辑/删除板块
- **THEN** 系统 SHALL 通过 `/api/semantic-boards` 命名空间操作 semantic_labels（label_type=board），不存在 board_concepts CRUD 端点

### Requirement: Board concept embedding generation (DEPRECATED)
系统 SHALL NOT 为 board_concepts 生成 embedding；该能力已由 `semantic-label-model` 中的双 embedding 字段（merge_embedding + storage embedding）替代。

#### Scenario: embedding 由 semantic_labels 双字段承载

- **WHEN** 板块/辅助标签需要生成 embedding
- **THEN** 系统 SHALL 写入 semantic_labels 的 merge_embedding 与 storage embedding 字段，不存在 board_concepts embedding 生成流程
