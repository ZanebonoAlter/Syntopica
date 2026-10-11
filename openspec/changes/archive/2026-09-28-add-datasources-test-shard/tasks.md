## 1. test-patrol.sh 新增分片

- [x] 1.1 `SHARD_ORDER` 在 `be-dataenrichment` 后插入 `be-datasources`（12→13 片）；验证：`--help` 列出 13 片轮转 + be-all
- [x] 1.2 新增 case 分支：`be-datasources) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 -timeout 120s ./internal/datasources/...) ;;`；验证：`TEST_PATROL_DRY_RUN=1 bash scripts/harness/test-patrol.sh --shard be-datasources` 输出参数向量与上述逐参数一致
- [x] 1.3 扩范围（用户 2026-09-28 批准）：`SHARD_ORDER` 在 `be-datasources` 后插入 `be-discovery`（13→14 片）；验证：`TEST_PATROL_DRY_RUN=1 bash scripts/harness/test-patrol.sh --shard be-discovery` 参数向量 = `go test -short -count=1 -timeout 120s ./internal/discovery/...`
- [x] 1.4 `fe-features` 分片 filter 并入 `app/features/feeds`；验证：`TEST_PATROL_DRY_RUN=1 bash scripts/harness/test-patrol.sh --shard fe-features` 输出含 `arg=app/features/feeds`

## 2. smoke 同步

- [x] 2.1 `test-patrol.smoke.sh` `SHARDS_STATIC` 同步 13 片（含 be-datasources）
- [x] 2.2 计数断言更新：TC-B06 文案「恰为 12 个轮转片」→13、TC-B09 两处 `"12"`→`"13"`、TC-B10 `"12"`→`"13"`、TC-B12 `--help` 文案「12 轮转片」→13、TC-B12 `for d` 列表补 `datasources`、internal 顶层目录断言 `"8"`→`"9"`
- [x] 2.3 全量验证：`bash scripts/harness/test-patrol.smoke.sh` 全绿
- [x] 2.4 扩范围断言同步：TC-B03 15 次回绕、TC-C03 `--shards 14`、TC-F04/TC-SUR02 枚举行 15、TC-B12 后端清单补 `discovery`（topdirs `9`→`10`）与前端清单补 `app/features/feeds`（覆盖 135/135）

## 3. 实跑与还账

- [x] 3.1 真跑一次 `bash scripts/harness/test-patrol.sh --shard be-datasources` 全绿落账（`patrol_shard` 新增 be-datasources 行）
- [x] 3.2 台账还账：`bash scripts/harness/test-patrol.sh --resolve "test-patrol.smoke.sh::TC-B12-internal-topdirs-9-vs-8"`，`--report` 确认该条转 fixed（fixed_by=add-datasources-test-shard）

## 4. 测试

- 本 change 为 harness 工具链改造，交付验证 = smoke 全量（第 5 节第 3 条）+ 两个新分片真跑落账（第 5 节第 4 条）；datasources/discovery 包单测本身为既有资产，不在本 change 改动范围。

## 5. 文档

- 同步 `docs/reference/harness/pi-extensions.md` §测试欠账滚动巡检 的分片枚举（12 片→14 片，含 be-datasources/be-discovery/fe-features 并入 feeds）。
<!-- doc-impact: none（纯 harness 工具链，不触及 flow 业务约束） -->

## 6. 验证

1. `bash scripts/harness/test-patrol.sh --help` → 列出 14 片轮转 + be-all，be-datasources/be-discovery 位于 be-dataenrichment 之后
2. `TEST_PATROL_DRY_RUN=1 bash scripts/harness/test-patrol.sh --shard be-discovery` → 参数向量逐参数等于 `go test -short -count=1 -timeout 120s ./internal/discovery/...`
3. `bash scripts/harness/test-patrol.smoke.sh` → 全部用例通过（0 FAIL）
4. `bash scripts/harness/test-patrol.sh --shard be-datasources && bash scripts/harness/test-patrol.sh --shard be-discovery` → 两片全绿，`--report` 可见 be-datasources/be-discovery 进度行
5. `bash scripts/harness/test-patrol.sh --resolve "test-patrol.smoke.sh::TC-B12-internal-topdirs-9-vs-8" --by add-datasources-test-shard && bash scripts/harness/test-patrol.sh --report` → 该欠账 status=fixed（fixed_by=add-datasources-test-shard）
