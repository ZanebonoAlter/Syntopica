# Design: fix-test-schema-missing-tables

simple 档精简设计。根因探索全文见 `docs/research/test-schema-debt/explore-findings.md`。

## D1 建库自洽：测试 setup 显式补注册（不建集中注册入口）

- **选型**：受影响测试 setup 内显式补注册——dataenrichment 的 CrossBoard setup 补 tagmanagement 模型注册（blank import `_ "syntopica-backend/internal/tagmanagement"` 触发 wire.go:40 的 init 注册，配注释说明）；platform/database 的 constraints 测试补 `_ "syntopica-backend/internal/platform/scheduler"`
- **先例**：`semantic_board_upgrade_test.go:1020`（测试内手动 RegisterModels）、`helpers_test.go:12`（注释明示机制）
- **否决备选**：testutil 提供集中 `RegisterAllDomainModels()`（blank import 全域包）——testutil 反向 import 各域引入编译耦合，改动面大；且 `platform/database` 测试对 `dataenrichment/repository` 存在循环依赖（后者 import 前者），集中注册也绕不开。跨包测试再踩坑时再升级
- **否决备选**：模型挪 `internal/models` 静态列表——与 decouple-backend-domains 的域自治方向相反

## D2 退役表：测试断言对齐 + 迁移守卫补齐

- `board_analysis_scope_migration_test.go:71-74` 的 reference_roles 插入断言块**删除或改为断言表不存在**（该测试主旨是 20260826_0001 的 scope 回填，reference_roles 断言是夹带的旧行为；该表已被 restructure-settings-navigation 迁移退役）
- 逐条核查迁移链中对 `reference_roles` 的残留引用（postgres_migrations.go:2207 已有 tableExists 守卫；:3224 的 copy 迁移需核查），缺守卫则按 20260826_0001 对 `topic_enrichment_result` 的守卫写法补
- destructive 迁移默认跳过行为（MIGRATIONS_ALLOW_DESTRUCTIVE 守卫）**不动**

## D3 notification 偶发：容差断言（不 waive）

- `notification_repository_test.go:104` 的时间戳严格相等断言改为容差比较（±2s 级），消除 DB now() 与 Go time 精度偏差假红
- 不选 waive：修复成本低于挂账噪声，且 3 连跑可机械验证

## D4 台账清理顺序

- 修复合入并复跑绿后，14 条 open 记录一次性 `--resolve`（`fixed_by=fix-test-schema-missing-tables`），重复登记的多条 id（裸名 / `platform/database:` 前缀 / `backend-go/...::` 前缀）逐条消，`--report` 验收 open=0

## 风险与回滚

全部为测试侧与迁移守卫增量分支，不改生产行为；迁移守卫只在"表缺失"分支新增跳过，既有库执行结果不变。回滚 = revert 测试文件改动。
