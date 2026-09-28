
## TC-B12 分片修复落点

TC-B12 欠账（datasources 域无巡检分片，smoke 断言 8 实 9）的修复落点：

1. `scripts/harness/test-patrol.sh`：
   - L43-46 `SHARD_ORDER`（12 片静态枚举，backend 6 + frontend 6）：在 be-dataenrichment 后插入 `be-datasources`
   - L157-166 case 分支区：新增 `be-datasources) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 -timeout 120s ./internal/datasources/...) ;;`（与 be-admin/be-dataenrichment 等同款单行形态；be-skeleton 兜底 platform/models/app/cmd 不动）
   - usage_hint 的分片数用 `%d` 动态算（L135），无需改
2. `scripts/harness/test-patrol.smoke.sh` 修点清单：
   - L343 `SHARDS_STATIC` 同步 13 片
   - L409 TC-B06 断言文案「恰为 12 个轮转片」→13（断言值动态）
   - L437/438 TC-B09 两处硬编码 `"12"`→`"13"`
   - L443 TC-B10 硬编码 `"12"`→`"13"`
   - L455-479 TC-B12：`--help 逐字列出 12 轮转片` 文案→13；L473 `for d in admin dataenrichment reader tagmanagement topicgraph platform models app` 补 `datasources`（插 dataenrichment 后）；L478 `assert_eq ... "8" "$be_rows"`→`"9"`
3. datasources 包测试现状：cache_test.go / catalog_allowlist_test.go / fetch_test.go / handler_test.go / init_test.go / repository_test.go / wiring/resolver_test.go（+sources/ 子目录），均 -short 可跑（09-24 巡检 be-all 全绿基线含它）
4. 台账还账：全绿后 `bash scripts/harness/test-patrol.sh --resolve "test-patrol.smoke.sh::TC-B12-internal-topdirs-9-vs-8"`；--report 验证 open 18→17
5. 验证命令：`TEST_PATROL_DRY_RUN=1 bash scripts/harness/test-patrol.sh --shard be-datasources`（参数向量断言）→ `bash scripts/harness/test-patrol.smoke.sh`（全量 smoke）→ `bash scripts/harness/test-patrol.sh --shard be-datasources`（真跑落账）

<!-- pinned 2026-09-28T00:09:20Z -->

## 扩范围定案：14 片轮转（discovery + feeds）

用户 2026-09-28 批准扩范围（原 13 片方案 → 14 片轮转）：smoke 实施中暴露两笔范围外「加域忘加片」漂移——① backend-go/internal/discovery（4421c068 解耦重构挪出的顶层域，28 个测试文件零覆盖，-short 全绿 <1s）；② front/app/features/feeds/composables/useAutoRefresh.test.ts（TC-B12 前端覆盖断言 134/135，5 用例绿）。定案：SHARD_ORDER=14 片（be-datasources 后插 be-discovery，后端 8 片 + 前端 6 片），fe-features filter 并入 app/features/feeds。smoke 断言终态：TC-B06=14/B09=B10=14/C03(--shards 14)=14/B03=15 次/F04=SUR02=15（14 轮+be-all）/TC-B12 topdirs=10（for d 补 discovery）/前端覆盖 135/135（for d 补 feeds）/TC-B08 前 3 片不变（be-discovery 是第 4）。连带：docs/reference/harness/pi-extensions.md L104 的 12 片枚举、spec delta「当前 13 片」枚举、proposal What Changes/Impact 需同步 14 片。discovery/feeds 测试均绿，无需登记新欠账，只 resolve 旧债 TC-B12-internal-topdirs-9-vs-8。

<!-- pinned 2026-09-28T02:17:21Z -->
