<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: content-enrichment -->

# Proposal: fix-test-schema-missing-tables

## Why

2026-09-30 巡检复跑确认 9 个真红测试欠账（台账 14 条 open 记录，最早 09-19 挂账）：7 条 CrossBoardRelation 族 + 2 条 database 族全部死于同一根因——测试库建库不自洽：`RunAutoMigrate` 的表集依赖各域包 `init()` 里 `RegisterModels` 的副作用注册，跨包集成测试的 import 链拉不到依赖包就不建表，后续版本化迁移 ALTER 报 `relation does not exist`；另外 `reference_roles` 已被 restructure-settings-navigation 迁移退役，但测试与后续迁移仍引用它。红测试长期挂账污染巡检信号，也让归档全量跑持续撞红。

## What Changes

- **测试建库显式化**：受影响测试 setup 补齐模型注册（先例：`semantic_board_upgrade_test.go:1020` 手动 `RegisterModels`、`helpers_test.go:12` 注释），或经 testutil 提供集中注册入口——保证 `OpenTestDB → RunAutoMigrate → RunMigrations` 在任何测试二进制里建出迁移链所需的全部表
- **迁移链守卫补齐**：对操作 `reference_roles` 等退役/可选表的迁移补 `tableExists` 守卫（先例：20260826_0001 对 `topic_enrichment_result` 的守卫写法）
- **测试对齐退役现实**：`board_analysis_scope_migration_test.go:71-74` 断言 `reference_roles` 存在的部分改为退役语义（表不存在/不再断言）
- **notification 偶发处置**：`TestNotificationEvictionAllUnread` 时间戳断言 3 跑 1 红（高负载偶发），改为容差断言或登记 waive
- **台账清理**：合并 6 条重复登记（`TestBoardLevelAnalysisScopeMigration` ×3、`TestModelTagConstraints_MaterializedInDB` ×3），修复后逐条消账

不修改任何业务逻辑、API、生产迁移语义（destructive 守卫默认值不变）。

## Capabilities

- **New Capabilities**: `test-db-fixture` —— 测试库建库契约：集成测试经 `testutil.OpenTestDB` 建库后，`RunAutoMigrate`+`RunMigrations` 必须在不依赖被测包 import 链的前提下建出迁移链所需的全部表；退役表引用必须有 `tableExists` 守卫或对齐退役断言
- **Modified Capabilities**: （无——纯测试基建，不改任何产品 spec 行为）

## Impact

- 代码：`backend-go/internal/dataenrichment/repository/cross_board_relation_test.go`、`backend-go/internal/platform/database/{board_analysis_scope_migration_test.go, constraints_test.go}`、`backend-go/internal/platform/testutil/`（如集中注册入口）、`backend-go/internal/platform/database/postgres_migrations.go`（仅守卫补齐）、`backend-go/internal/platform/notification/notification_repository_test.go`
- 台账：`.pi/harness/events.db` test_debt 表（经 `test-patrol.sh --resolve/--waive`，fixed_by=本 change）
- 探索依据：`docs/research/test-schema-debt/explore-findings.md`（根因三线索）
- 风险：低——全部是测试侧与迁移守卫；迁移守卫只增 `tableExists` 分支，不改变既有库上的执行结果
