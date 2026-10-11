<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

`backend-go/internal/datasources` 是 board-signal-reports（2026-09-24 归档）引入的新 domain 目录，已有 7+ 个测试文件（`cache_test.go` / `catalog_allowlist_test.go` / `fetch_test.go` / `handler_test.go` / `init_test.go` / `repository_test.go` / `wiring/resolver_test.go` 等），但 `scripts/harness/test-patrol.sh` 的 `SHARD_ORDER` 是 12 片静态枚举（backend 6 片 + frontend 6 片），**没有 be-datasources 分片**——新域测试零巡检覆盖，与本 spec「后端按 internal/ domain 目录分片」的既有意图不符（实现漂移，非 spec 缺口）。

连带后果：smoke 的 TC-B12「后端 8 个 internal 顶层目录都被分片覆盖」计数断言 8 实 9 已经红了，该欠账 2026-09-24 已登记 test_debt 台账（`test-patrol.smoke.sh::TC-B12-internal-topdirs-9-vs-8`，context=board-signal-reports 遗留），挂账 3 天未还。

实施中（2026-09-28）smoke 又暴露两笔同类漂移，经用户批准扩范围一并修复：`backend-go/internal/discovery`（解耦重构 4421c068 挪出的顶层域，28 个测试文件零巡检覆盖，`-short` 全绿）与 `front/app/features/feeds`（TC-B12 前端覆盖断言 134/135，5 用例绿）。

## What Changes

- `scripts/harness/test-patrol.sh`：`SHARD_ORDER` 插入 `be-datasources` 与 `be-discovery`（12→14 片，be-* 保持字典序）；新增两个 case 分支映射 `go test -short -count=1 -timeout 120s ./internal/datasources/...` 与 `./internal/discovery/...`；`fe-features` 分片 filter 并入 `app/features/feeds`（与其余分片同款参数向量）。
- `scripts/harness/test-patrol.smoke.sh`：`SHARDS_STATIC` 同步 14 片；分片计数断言全部同步 14 片（TC-B06/B09/B10/B12/C03，TC-B03 回绕 15 次，TC-F04/TC-SUR02 枚举行 15=14 轮转+be-all）；TC-B12 的后端目录清单补 `datasources`/`discovery`、internal 顶层目录计数 `8`→`10`、前端清单补 `app/features/feeds`。
- 台账还账：实现验证全绿后 `test-patrol.sh --resolve test-patrol.smoke.sh::TC-B12-internal-topdirs-9-vs-8`（fixed_by=本 change）。

不改动：分片轮转策略、`--shards <n>` 多片语义、`be-all`（不入轮转）、前端 6 片划分、巡检进度表 schema。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `test-debt-patrol`：「滚动分片巡检」Requirement 新增 Scenario——internal 下新增含测试的 domain 目录 SHALL 纳入轮转分片，smoke 枚举完整性断言 SHALL 同步，分片枚举与目录现状不一致时 smoke 失败（把这次「加域忘加片」类漂移锁进防回归）。

## Impact

- **代码**：仅 `scripts/harness/test-patrol.sh` 与 `test-patrol.smoke.sh` 两个 harness 脚本；无业务代码、无前端、无数据变更。
- **部署后影响**：巡检轮转从 12 片变 14 片，datasources 与 discovery 域测试进入「最久未巡优先」滚动巡检（fe-features 片扩入 feeds 目录测试）；巡检进度表新增 be-datasources、be-discovery 两行（冷启动首轮补巡）。无用户可见行为变化。
- **需要的手动操作**：无（后续任意会话顺手巡检自动覆盖新片；如需立即补巡可 `bash scripts/harness/test-patrol.sh --shard be-datasources`）。
- **旧数据降级**：无——`patrol_shard` 表按 shard 主键追加，存量 12 行进度不受影响。
