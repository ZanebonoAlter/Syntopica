# test-cases（simple 档精简：specs Scenario 即用例，此处只记主链路落点与变体走查）

## 主链路表

| # | 场景 | 落点 | 期望 |
| --- | --- | --- | --- |
| 1 | 新分片枚举与参数向量 | TC-C01 同款 DRY_RUN 断言（smoke 现有模式，be-datasources 版） | `go test -short -count=1 -timeout 120s ./internal/datasources/...`，cwd=backend-go，无 `./...` 全仓模式 |
| 2 | 枚举完整性（spec 新 Scenario） | smoke TC-B12（13 片 + 9 目录计数） | 全绿；故意少列一片/少数一目录时红（由断言本质保证，不需额外用例） |
| 3 | 真跑落账 | `--shard be-datasources` | 全绿，`patrol_shard` 新增行 runs=1 last_ok=1；datasources 域测试（cache/catalog/fetch/handler/init/repository/wiring）全部执行 |

## 变体走查

- 冷启动 13 片轮转：TC-B06 选中集合断言动态对比 `SHARDS_STATIC`，无需额外用例 ✓
- `--shards 13` 多片：TC-B09/B10 计数断言 13 同步 ✓
- 前端 6 片与 be-all：不改动，既有断言覆盖 ✓
- patrol_shard 存量 12 行：主键追加语义，无需迁移 ✓

## 效果核对

- `--report` 中 open 从 18 → 17（TC-B12 欠账转 fixed）；巡检进度出现 `be-datasources` 行。
