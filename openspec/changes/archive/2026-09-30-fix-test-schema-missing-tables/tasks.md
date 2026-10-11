# Tasks: fix-test-schema-missing-tables

## 1. dataenrichment CrossBoard 测试建库补注册

- [x] 1.1 `cross_board_relation_test.go` setup 补 tagmanagement 模型注册（blank import 触发 init，配注释引用 helpers_test.go:12 模式），复跑（不带 -short）7 条 CrossBoardRelation 族测试全绿
- [x] 1.2 同 setup 若其它迁移依赖表仍缺（复跑报新 relation does not exist），按同模式补对应域注册直至迁移链通过（实际：补 tagmanagement 后迁移链全程通过，无新增缺表）

## 2. platform/database 两个测试修复

- [x] 2.1 `constraints_test.go` setup 补 `_ "syntopica-backend/internal/platform/scheduler"` 注册（或等价显式 RegisterModels），`TestModelTagConstraints_MaterializedInDB` 不带 -short 全绿
- [x] 2.2 `board_analysis_scope_migration_test.go` 删除/改造 :71-74 reference_roles 断言块为退役语义（断言块 3 改为断言表不存在；断言块 5 改为验证 seed 迁移 tableExists 守卫安全跳过；头部注释与 cleanup 同步对齐），`TestBoardLevelAnalysisScopeMigration` 不带 -short 全绿

## 3. 迁移链退役表守卫

- [x] 3.1 核查 postgres_migrations.go 中全部 reference_roles 引用（:2207 / :3224 等），缺 tableExists 守卫的补齐（照 20260826_0001 写法）——实测全部已有守卫：20260826_0002 seed（:2207）、20260828_0002 copy（:3226，含 analysis_methods）、20260831_0001 retire（:3253）、20260926_0001 DROP（:2468 IF EXISTS），无需补代码
- [x] 3.2 干净测试库（testcontainer）跑完整 RunMigrations 链验证无 relation does not exist——dataenrichment/repository（补 tagmanagement 注册）与 platform/database（补 scheduler 注册）两个二进制各自全新容器全链跑通全绿

## 4. notification 偶发容差断言

- [x] 4.1 `notification_repository_test.go:104` 时间戳断言改容差比较（±2s，require.WithinDuration），`go test -count=3 -run '^TestNotificationEvictionAllUnread$' ./internal/platform/notification/` 3 连跑全绿

## 5. 台账清理

- [x] 5.1 复跑验收全绿后，14 条 open 记录逐条 `bash scripts/harness/test-patrol.sh --resolve <id> --by fix-test-schema-missing-tables`（含 6 条重复登记的多形式 id）
- [x] 5.2 `bash scripts/harness/test-patrol.sh --report` 显示 open=0 / fixed=162（既有 148 + 本次消 14；原写 154 为估算基数）

## 6. 测试

- 影响包（仅本 change 触及的测试文件，连库测试不带 -short，串行跑）：
  - `cd backend-go && go test -run '^(TestCrossBoardRelationInsertIdempotent|TestCrossBoardRelationInsertRejectsClosedStatus|TestCrossBoardRelationConfirmLifecycle|TestCrossBoardRelationConfirmInvalidStates|TestCrossBoardRelationConfirmTargetVanished|TestCrossBoardRelationDismissCooldown|TestCrossBoardRelationDismissUnresolvedAllowed)$' ./internal/dataenrichment/repository/`
  - `cd backend-go && go test -run '^(TestBoardLevelAnalysisScopeMigration|TestModelTagConstraints_MaterializedInDB)$' ./internal/platform/database/`
  - `cd backend-go && go test -count=3 -run '^TestNotificationEvictionAllUnread$' ./internal/platform/notification/`
- 既有测试不回归：`bash scripts/harness/change-scope.sh` 判定影响包并按其映射跑（本 change 改 _test.go 与迁移守卫，需覆盖 platform/database 全包连库测试）

## 7. 文档

<!-- doc-impact: standard -->
<!-- doc-impact-excuse: database=仅 platform/database 两个 _test.go 内 AutoMigrate 断言字样被启发式命中；本 change 零 schema 变更、生产迁移语义未动（proposal 明确），docs/reference/database/ 无需更新 -->
- 根因探索已存 `docs/research/test-schema-debt/explore-findings.md`
- 建库契约已沉淀：`docs/reference/standard/backend/testing.md`「黄金 schema 模型的三条约束」新增约束③（跨包测试建库不依赖被测包 import 链，含 blank import / 测试内 init() RegisterModels 两种修复模式与先例引用）

## 8. 验证

- [x] `cd backend-go && go test -run '^TestCrossBoardRelation' ./internal/dataenrichment/repository/ -v 2>&1 | grep -c '^--- PASS'` 期望 `12`（原 7 红全绿 + 既有 5 绿不回归；修复前实测正则匹配 12 个顶层测试，原期望 7 为红测试数）
- [x] `cd backend-go && go test -run '^(TestBoardLevelAnalysisScopeMigration|TestModelTagConstraints_MaterializedInDB)$' ./internal/platform/database/ | tail -1` 期望 `ok`
- [x] `cd backend-go && go test -count=3 -run '^TestNotificationEvictionAllUnread$' ./internal/platform/notification/ | tail -1` 期望 `ok`
- [x] `cd backend-go && golangci-lint run ./internal/platform/database/ ./internal/platform/notification/ ./internal/dataenrichment/...` 期望无输出（exit 0）
- [x] `bash scripts/harness/test-patrol.sh --report | head -2` 期望 `open 0 / fixed 162`（实测 14 条全消，既有 fixed 基数 148；原写 154 为估算值）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 跨域仓库层测试建库自洽 | backend-go/internal/dataenrichment/repository/cross_board_relation_test.go |
| 数据库层约束测试建库自洽 | backend-go/internal/platform/database/constraints_test.go |
| 干净库跑完整迁移链 | backend-go/internal/platform/database/board_analysis_scope_migration_test.go, backend-go/internal/platform/database/constraints_test.go |
| 迁移测试断言退役语义 | backend-go/internal/platform/database/board_analysis_scope_migration_test.go |
| notification 淘汰时间戳容差 | backend-go/internal/platform/notification/notification_repository_test.go |
