## Purpose

定义文章批量操作（`PUT /api/articles/bulk-update`）的 scope 契约，消除「全部文章视图全站标已读」的隐式契约盲区（test-design ⑤）。

## ADDED Requirements

### Requirement: 批量操作 scope 契约
bulk-update SHALL 至少携带一个 scope（`all` / `ids` / `feed_id` / `category_id` / `uncategorized`），且至少携带一个更新字段（`read` / `favorite`）；两者缺一 SHALL 返回 400。全站标记（无限定范围）SHALL 仅由显式 `all=true` 触发——无 scope 且无 all 不得隐式执行全站更新。

#### Scenario: 无 scope 无 all 返回 400
- **WHEN** 请求仅带 `{read: true}`，无任何 scope
- **THEN** 返回 400，错误信息含 "Must specify a scope"

#### Scenario: 显式 all 全站标已读
- **WHEN** 请求 `{read: true, all: true}`
- **THEN** 全站未读文章置为已读，返回 200

#### Scenario: all 与其他 scope 互斥
- **WHEN** 请求同时带 `all: true` 与 `feed_id`
- **THEN** 返回 400（语义冲突，拒绝执行）

#### Scenario: 仅有 scope 无更新字段返回 400
- **WHEN** 请求 `{all: true}` 但 read/favorite 均未指定
- **THEN** 返回 400，错误信息含 "At least one field"
