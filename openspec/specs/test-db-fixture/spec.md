# test-db-fixture Specification

## Purpose

测试库建库契约：集成测试经测试建库流程（testcontainer Postgres → AutoMigrate → 版本化迁移）后，迁移链所需的全部表必须存在且迁移可完整执行——不依赖被测包 import 链拉到的模型注册副作用，退役表引用必须有存在性守卫，时序敏感断言必须有容差。保证跨包集成测试建库自洽、干净库迁移链全绿、高负载下测试可重复。

## Requirements

### Requirement: 测试库建库不依赖被测包 import 链

测试基建 SHALL 保证：任何 Go 集成测试经测试建库流程（打开 testcontainer Postgres → AutoMigrate → 版本化迁移）后，版本化迁移链所需的全部表在测试库中存在，无论该测试二进制的 import 链是否覆盖各域模型注册包（`RegisterModels` 的 init 副作用不得成为建库前提）。

#### Scenario: 跨域仓库层测试建库自洽

- **WHEN** `internal/dataenrichment/repository` 的集成测试执行建库（AutoMigrate + RunMigrations）
- **THEN** 迁移 `20260403_0003`（ALTER `topic_tag_embeddings` 加列）执行成功，CrossBoardRelation 族 7 个测试不带 `-short` 全绿

#### Scenario: 数据库层约束测试建库自洽

- **WHEN** `internal/platform/database` 的集成测试执行建库后查询物化列约束
- **THEN** `scheduler_tasks.name` 等列的 information_schema 查询能取到行，`TestModelTagConstraints_MaterializedInDB` 不带 `-short` 全绿

### Requirement: 退役表引用有存在性守卫

版本化迁移对已退役表（如 `reference_roles`，已被 restructure-settings-navigation 的迁移 DROP）的引用 SHALL 有 `tableExists` 守卫或在表缺失时安全跳过；在不含退役表的干净库上执行完整迁移链 SHALL 不因退役表缺失而失败。集成测试 SHALL 与退役现实对齐：不再断言已退役表可写入。

#### Scenario: 干净库跑完整迁移链

- **WHEN** 在不含 `reference_roles` 的干净测试库上执行完整版本化迁移链
- **THEN** 迁移链整体成功，无 `relation "reference_roles" does not exist` 类错误

#### Scenario: 迁移测试断言退役语义

- **WHEN** board-level analysis scope 迁移测试在 `reference_roles` 已退役的语义下运行
- **THEN** 测试不再断言该表可插入，测试不带 `-short` 全绿

### Requirement: 偶发时间断言可重复

依赖数据库时间或 eviction 时序的测试断言 SHALL 使用容差比较（非严格相等），保证高负载下 3 连跑稳定全绿。

#### Scenario: notification 淘汰时间戳容差

- **WHEN** `TestNotificationEvictionAllUnread` 连续执行 3 次（不带 `-short`）
- **THEN** 3 次全绿，时间戳断言不再因精度偏差产生假红
