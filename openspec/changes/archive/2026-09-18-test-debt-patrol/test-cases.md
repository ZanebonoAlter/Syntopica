# Test Cases — test-debt-patrol（复杂档白盒用例）

<!-- 对应 proposal.md 头部 complexity: complex；本文件是 case-first-testing（docs/reference/开发执行规范.md §2）复杂档要求的白盒用例清单，tasks.md 2.1 的产物。

用途：把「断言判据」（主线程定，见 §0 判据来源）机械展开为可判定的测试分支，覆盖本 change 两个 delta spec 的全部 13 个 Scenario。
本文件只做机械枚举，不写实现代码、不写测试代码；实现阶段按 tasks 7.1 逐条落成 `scripts/test-patrol.smoke.sh` 与静态断言，命名若与建议缝不一致须回改本文件。

关联 spec：
- openspec/changes/test-debt-patrol/specs/test-debt-patrol/spec.md（4 Requirement / 11 Scenario）
- openspec/changes/test-debt-patrol/specs/harness-fact-log/spec.md（1 Requirement / 2 Scenario） -->

## 0. 判据来源与落点

| 判据来源 | 取用的内容 |
| --- | --- |
| 主线程契约（本 change 任务书「待实现的契约」） | 8 个子命令语义、退出码 0/1/2、两张表字段、12 片轮转 + `be-all` 静态枚举、6 条关键实现约束 |
| design.md 决策 1–6 | 台账存 events.db 独立表（复用开库安全契约）、静态分片、单入口脚本、软约束记账、事件词汇 `patrol.check` 30 天 |
| survey.md §1–§4 | 耗时基线（后端整体 42s、前端 `--maxWorkers=2` 分片）、存量红清单、**§4.1 调用陷阱（`pnpm test:unit --` 吞 filter）** |
| `.pi/extensions/lib/harness-log.ts` | events 表结构、开库安全（app_id/user_version 校验）、`busy_timeout`/WAL、`RETENTION_DAYS` |
| `standard/frontend/testing.md` + `standard/shared/test-design.md` | load 红线（巡检限并发）、白盒附加件规范 |

**落点**：A–H、SUR 组 → `scripts/test-patrol.smoke.sh`（临时库 fixture + fake runner，毫秒级、不跑真实测试）；W 组 → 文档/扩展源码 grep + `bash .pi/extensions/tests/run-smoke.sh` 既有断言。真实 12 片全跑等长跑项见 §5。

| 组 | 主题 | 条数 |
| --- | --- | --- |
| A | 台账状态机 | 15 |
| B | 分片调度 | 12 |
| C | 执行引擎与失败解析 | 14 |
| V | 事件记账（patrol.check） | 10 |
| R | 报告聚合 | 9 |
| F | CLI 参数校验与退出码 | 12 |
| P | 资源预检 | 6 |
| X | 并发与输入安全 | 7 |
| SUR | 存量摸底复核 | 2 |
| W | 归档记账纪律与门禁不变 | 5 |

## 1. 前置实现契约（测试依赖的缝）

白盒用例必须能确定性复现「有失败的分片」「高负载」「31 天前的时间」而**不跑真实失败测试、不污染真实库**。以下缝是实现阶段需要提供的（建议名，语义固定）：

| 缝 | 形态 | 用途 | 用例 |
| --- | --- | --- | --- |
| `TEST_PATROL_DB` | 环境变量：台账 + 事件库路径（默认 `.pi/harness/events.db`） | smoke 全程用 tmp 库，真实台账零污染 | A–H 全部 |
| `TEST_PATROL_FAKE_OUTPUT` | 环境变量：runner 输出文本文件路径；设置后不执行真实测试命令，直接消费该文件 | 确定性复现全绿/失败/构建失败/解析边界 | C、V、R |
| `TEST_PATROL_FAKE_RC` | 环境变量：伪造 runner 退出码 | 退出码映射与异常退出 | C12、C13 |
| `TEST_PATROL_DRY_RUN` | 环境变量 =1：只打印「将要执行」的命令（cwd + 逐参数，每参数一行），不执行、不写库 | 命令形态断言（前端不得出现 `--`） | B11、C01–C03 |
| `TEST_PATROL_LOADAVG_FILE` | 环境变量：覆盖 loadavg 读取路径 | 高/低/边界负载注入 | P01–P03、P06 |
| `TEST_PATROL_NOW` | 环境变量：覆盖「当前时间」（ISO8601） | stale 30 天边界避免秒级竞态 | R05、R06 |
| `parse_backend_fails` / `parse_frontend_fails` | 脚本内函数：stdin = runner 输出，stdout = 去重后 test_id 列表（一行一条） | 解析分支可单独喂 fixture | C04–C11 |

降级约定：若实现不提供某个缝，对应用例降级为 tasks 3.2 允许的「临时注入失败测试 + 真实跑」或「源码静态断言」，但**断言内容不变**，且须在本文件留痕（不静默删用例）。

## 2. 场景/分支表

### A. 台账状态机

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-A01 | `test_debt` 空（0 行） | 巡检某片，fixture 全绿 | 台账仍 0 行（不产生任何记录）；进度表该片 `runs=1`、`last_ok=1`、`last_fails=0` | 空库冷启动 |
| TC-A02 | `test_debt` 空 | 巡检检出 1 个未知失败 `front/app/x.test.ts::suite > case` | 新增恰 1 条：`status=open`、`first_seen==last_seen==本次巡检时间`（秒级相同）、`context==分片名`、`domain==分片名`、`fixed_by/waived_reason/waived_at` 为 NULL | 新红自动登记（spec Scenario 1） |
| TC-A03 | 上条记录已存在且 `status=open` | 同片第二轮仍红，test_id 相同 | 行数不变；`last_seen` 变大、`first_seen` 不变、`status` 仍 open、`note` 不变、`context` 不被覆盖 | 已有记录不重复登记（spec Scenario 2） |
| TC-A04 | 同 TC-A03 | 同批次 fixture 内同一 test_id 出现 2 次（`×` 明细段 + 尾部 ` FAIL ` 段各一次） | 台账恰 1 条；`patrol.check.payload.fails` 长度 1；台账新增+复现行数 == `fails` 长度 | 批次内去重 |
| TC-A05 | 空台账 | `--register <新 id> --context test-debt-patrol --domain manual` | 新增 1 条 open；`context` 原文写入（含连字符）；`domain=manual`；`--report` 的 open 清单含该条 | 手工登记路径（归档语境，spec Scenario 8） |
| TC-A06 | 记录 `status=fixed`（`fixed_by=某 change`） | `--register 同 id --context test-debt-patrol` | `status` 迁回 open；`note` 追加一段含 `reopened from fixed@` + 日期；`fixed_by` 保留不清空；`first_seen` 不变 | 终态记录复现 → 迁回 open |
| TC-A07 | 记录 `status=waived`（`waived_reason` 非空） | 同 TC-A06 | 迁回 open；`note` 含 `reopened from waived@`；`waived_reason`、`waived_at` 保留 | 同上，waived 分支 |
| TC-A08 | 记录 `status=open` | `--resolve <id> --by fix-xyz` | `status=fixed`、`fixed_by=fix-xyz`；`first_seen`/`last_seen` 不变；`waived_*` 仍 NULL | 迁移留痕（fixed） |
| TC-A09 | 记录 `status=open`，`domain=be-reader` | 巡检 `be-reader` fixture 全绿 | 该记录迁 `fixed`、`fixed_by=patrol`；`first_seen` 保留；行不删除 | 转绿闭环（spec Scenario 7） |
| TC-A10 | 两条 open：`domain=be-reader` 与 `domain=manual` | 巡检 `be-reader` 全绿 | 前者迁 fixed(patrol)；`domain=manual` 的记录仍 open 且全字段零变化 | 转绿不误伤（负向） |
| TC-A11 | 记录 `status=open` | `--waive <id> --reason "已知环境问题，见 survey §3"` | `status=waived`；`waived_reason` 原文（含中文与 `§`）；`waived_at` 非空；`fixed_by` 仍 NULL | 迁移留痕（waived） |
| TC-A12 | 记录 `status=fixed` / `status=waived` | `--resolve 同 id` 或 `--waive 同 id --reason x` | 口径定稿：终态幂等 no-op（exit 0 + 提示已 fixed/waived），`status` 与 `waived_reason` 等字段零变化 | 终态不可被事后改理由（见待定 A-3） |
| TC-A13 | 库含 open/fixed/waived 混合 | 跑完整 A 组后统计 | `SELECT COUNT(*)` 只增不减（终态行仍在）；`grep -n 'DELETE FROM test_debt' scripts/test-patrol.sh` 零命中 | 终态不得物理删除 |
| TC-A14 | `--init` 已执行 | ①读 `.schema test_debt` ②直接 INSERT `status='closed'` | ①含 11 列与 `test_id` 唯一约束线索 ②非法 status 被拒（DDL CHECK 约束或脚本层拒绝，二选一，见待定 A-4），库中不出现第四种状态 | status 三态枚举约束 |
| TC-A15 | open 记录，`note` 为空 | `--register 同 id`（记录仍为 open 的重复登记） | `note` 仍为空——reopened 痕迹只写在「跨终态迁回」（TC-A06/A07），重复登记不写 | 痕迹写入时机（防噪声） |

### B. 分片调度

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-B01 | 进度表空（12 片 `last_run` 全 NULL） | 无参数执行（fake 全绿） | 选中 `be-admin`（静态表首片，stdout 摘要含分片名）；进度表仅 `be-admin` 有行（`runs=1`），其余 11 片无行或 `last_run` 仍 NULL | 冷启动：NULL 排最前 |
| TC-B02 | `be-admin` 已跑过 | 无参数执行 | 选 `be-dataenrichment`；两片 `last_run` 均非空且 be-admin 更早 | 第二片推进 |
| TC-B03 | 12 片各跑过一次 | 再跑一次 | 选 `last_run` 最早的那片；累计 13 次触发的选中序列 = 静态表顺序整轮循环 | 一轮完成后重新开始 |
| TC-B04 | 12 片 `last_run` 齐，其中一片人为置 NULL | 无参数执行 | 选该 NULL 片（NULL 优先于任何时间戳，即使其它片时间戳很旧） | NULL 排序优先级 |
| TC-B05 | 两片 `last_run` 人为写成完全相同（非 NULL） | 无参数执行两次（同构造） | 两次都选静态表中靠前者；结果稳定无随机 | 平局按静态表顺序 |
| TC-B06 | 进度表清空 | 无参数连跑 12 次 | 选中集合恰为 12 个轮转片（无重复、无遗漏）；`be-all` 从未被选中；进度表无 `be-all` 行 | `be-all` 不在轮转内 |
| TC-B07 | 进度表空 | `--shard be-all` | 执行整片（DRY_RUN 下命令含 `./internal/...` 与 `./cmd/...`）；进度表新增 `be-all` 行 | 整片仅显式指定 |
| TC-B08 | 进度表空 | `--shards 3` | 连续执行 3 片，顺序 = 最久未巡优先序（冷启动即静态表前 3 片）；stdout 3 行摘要；3 片 `runs=1` | n 片连续执行 |
| TC-B09 | 12 片已跑满（`last_run` 齐） | `--shards 99` | 12 片各跑一次（不重复、不报错）；全绿时 exit 0；stdout 12 行摘要 | 超过总数 → 跑完全部 |
| TC-B10 | 进度表空 | `--shards 12` | 恰跑 12 片；`be-all` 不在其中 | n == 总数边界 |
| TC-B11 | 任意 | `TEST_PATROL_DRY_RUN=1` 无参数执行 | 打印「将要执行」命令（cwd + 逐参数）；不执行、不写库（三表行数前后相等） | 命令形态断言入口 |
| TC-B12 | 仓库现状 | `--help` / 枚举输出 | 列出 12 个轮转片名 + `be-all`，与静态表逐字一致；每个前端片 filter 至少命中 1 个 `*.test.ts`（摸底基线：tags 43 / discovery 9 / features 13 / core 20 / composables 6 / components 9）；每个后端片至少命中 1 个包（admin 5 / dataenrichment 4 / reader 4 / tagmanagement 9 / topicgraph 4 / skeleton 24） | 枚举完整性 + 与目录现状一致 |

### C. 执行引擎与失败解析

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-C01 | DRY_RUN | 跑后端片 `be-reader` | 命令逐参数为 `go test -short -count=1 ./internal/reader/...`；含 `-short`、`-count=1`；不含 `./...` 全仓模式、不含 `-run` | 后端命令契约 |
| TC-C02 | DRY_RUN | 跑前端片 `fe-tags` | cwd = `front`；命令含 `pnpm test:unit`、≥1 个 filter（`app/features/tags`）、`--maxWorkers=2`；**参数序列中不存在单独的 `--` 元素** | 前端命令红线（survey §4.1） |
| TC-C03 | DRY_RUN | 逐个跑全部 12 片 + `be-all` | 每片命令的 filter 组合与静态枚举逐字一致；无任何片含 `--`；每片恰 1 次调用（不重试） | 全片命令体检 |
| TC-C04 | `FAKE_OUTPUT` = 后端全绿 fixture（只含 `ok` / `?` 行） | 巡检 | 解析 fail 列表为空；exit 0；台账零变；`patrol.check.payload.fails` == `[]` | 后端全绿 |
| TC-C05 | `FAKE_OUTPUT` 含 `--- FAIL: TestA (0.00s)` 与包级 `FAIL	syntopica-backend/internal/reader/service	0.123s` | 巡检 | test_id 恰为 `internal/reader/service::TestA`（剥 module 前缀 `syntopica-backend/`，不带 `./`）；无额外记录 | 后端失败解析（口径见待定 A-1） |
| TC-C06 | `FAKE_OUTPUT` 含父 `--- FAIL: TestA` 与子 `    --- FAIL: TestA/case_x` 两行 | 巡检 | 产出 `internal/<pkg>::TestA` 与 `internal/<pkg>::TestA/case_x` 两条，子测试 `/` 原样保留 | 子测试保留 `/` |
| TC-C07 | `FAKE_OUTPUT` 两包混排（各 1 失败） | 巡检 | 两条 test_id 的包归属正确（不串包）；台账 2 行 | 包切换归属 |
| TC-C08 | `FAKE_OUTPUT` 含 `# syntopica-backend/internal/foo` + 编译错误 + `FAIL	syntopica-backend/internal/foo [build failed]` | 巡检 | 产出 `internal/foo::<build-failed>` 恰 1 条（该包不再产出其它记录）；exit 1 | 构建失败标识 |
| TC-C09 | `FAKE_OUTPUT` 含 ` FAIL  app/plugins/chunk-error-fallback.test.ts [ app/plugins/chunk-error-fallback.test.ts ]` | 巡检 `fe-core` | 产出 `front/app/plugins/chunk-error-fallback.test.ts::<file-level>` 恰 1 条 | 前端整文件级失败（collect 失败） |
| TC-C10 | `FAKE_OUTPUT` 含 ` FAIL  app/composables/useOnboarding.test.ts > useOnboarding > first-run detection (isFirstRun) > is true when syntopica_onboarding_complete is absent` | 巡检 | 产出 `front/app/composables/useOnboarding.test.ts::useOnboarding > first-run detection (isFirstRun) > is true when ...`（` > ` 链逐字保留，含括号、双引号、`===` 等字符） | 前端用例级失败（嵌套 describe） |
| TC-C11 | `FAKE_OUTPUT` 同时含 `×` 明细段与尾部 ` FAIL ` 段（同 17 条） | 巡检 | 台账恰 17 条（不双计）；`payload.fails` 长度 17 | 双 reporter 段去重 |
| TC-C12 | ①`FAKE_RC=0` + 全绿 fixture；②`FAKE_RC=1` + 含 FAIL fixture；③`FAKE_RC=1` + 零 FAIL 行 | 三次巡检 | ①exit 0；②exit 1；③exit 2 且 stdout 提示「无法解析失败明细」，不新增台账行，`patrol.check` 仍写 `ok=false`/`fails=[]`，进度 `last_ok=0` | 退出码映射 + runner 异常退出（待定 A-5） |
| TC-C13 | `FAKE_RC=1` + 1 失败 fixture | 巡检 | exit 1 的同时四件事齐备：台账新增/更新、`patrol.check` 写入、`patrol_shard` 更新（`last_ok=0`、`last_fails=1`）、stdout 摘要——exit 1 不短路记账 | 失败片也完成全部记账 |
| TC-C14 | 某片已有 open 记录，fixture 全绿 | 巡检该片 | 台账行数不变（不新增）；`last_ok=1`、`last_fails=0`、`runs` +1、`total_fails` 不变；`last_ms` 为非负整数且 == `patrol.check.payload.ms` | 全绿片零台账变更 + 耗时口径 |

### V. 事件记账（patrol.check）

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-V01 | events 表存在 | 巡检 1 片（fake 1 失败） | 新增恰 1 条 `kind='patrol.check'`；payload JSON.parse 后键集合**恰为** `{shard, ok, ms, fails}`（无多余键）；`ok=false`、`ms` 为数字、`fails` 为数组且含该 test_id | payload 字段恰好 |
| TC-V02 | 同上 | 同上 | 该行 `change IS NULL`、`session_id` 非空、`ts` 为 ISO8601 UTC 字符串 | 归因列为空（仓库级活动） |
| TC-V03 | 同上 | 巡检全绿片 | `payload.ok=true`、`payload.fails=[]`（空数组，不是 null、不是缺键） | 成功空数组 |
| TC-V04 | 事件表预置 1 条 31 天前 + 1 条 29 天前的 `patrol.check` | 冷开库（扩展侧 TTL 清扫） | 静态断言：`RETENTION_DAYS` 含 `"patrol.check": 30`（grep `.pi/extensions/lib/harness-log.ts`）；行为断言：31 天前被清、29 天前保留（在 `.pi/extensions/tests/harness-log.smoke.cjs` 扩一条） | 保留期 30 天，与 gate.check 同级 |
| TC-V05 | 1 次失败巡检后 | 手工 `DELETE FROM events WHERE kind='patrol.check'`（模拟 TTL 到期） | `test_debt` 记录与状态零变化、进度表零变化、`--report` 输出不变 | 台账登记不双写事件（spec Scenario 13） |
| TC-V06 | `TEST_PATROL_DB` 指向不可创建/不可写路径 | 巡检 1 片（全绿） | 写入失败 → 出现含「写入失败」的告警行（带原因）；**分片摘要照常打印、exit 仍 0**；不产生半条脏行 | fail-open 不阻断（待定 A-5） |
| TC-V07 | tmp 库 `application_id=123` 且有 1 张用户表 | 巡检 1 片 | 拒绝写入：不建 `test_debt`/`patrol_shard`、不 INSERT events；`sqlite_master` 无新表；告警可见；退出码按巡检结果（fail-open） | 他人库拒绝（harness-log 开库契约） |
| TC-V08 | tmp 库 `application_id=SYNT` 且 `user_version=2` | 巡检 1 片 | 同 TC-V07 拒绝；告警含版本原因 | 未来版本库拒绝 |
| TC-V09 | events 表已有 N 行 | `--init` 后巡检 1 片 | 原 N 行不动（建表不清表）；新 patrol.check 追加 1 行 | init 不破坏既有事件 |
| TC-V10 | 巡检 1 片 | 源码/行为检查 | `grep -n 'busy_timeout' scripts/test-patrol.sh` ≥1 命中；写入后 `PRAGMA journal_mode` 仍为 `wal` | WAL + busy_timeout 前置（见 TC-X01） |

### R. 报告聚合

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-R01 | 台账空 + 进度空 | `--report` | exit 0；输出含三态计数行（open 0 / fixed 0 / waived 0）、open 清单为空提示、stale 段无条目、进度段「暂无巡检记录」提示；无 SQL 报错 | 空库报告 |
| TC-R02 | 造 open 2 / fixed 2 / waived 1 | `--report` | 计数行数字精确 2 / 2 / 1；终态不计入 open | status 聚合 |
| TC-R03 | 造 `domain=be-reader` 3 条、`manual` 1 条、`fe-core` 1 条 | `--report` | domain 聚合输出三行且数字正确；排序稳定（计数降序或字母序，实现定但不随机） | domain 聚合 |
| TC-R04 | 造 3 条 open，`first_seen` = T3 / T1 / T2（T1<T2<T3） | `--report` | open 清单顺序 = T1、T2、T3（按 `first_seen` 升序，最老欠账在前）；fixed/waived 不出现在该清单 | open 清单排序（口径见待定 A-2） |
| TC-R05 | 造 open+`last_seen=now-31d`、open+`now-1d`、fixed+`now-40d` | `--report` | stale 段只含第 1 条；第 2/3 条不出现（仅 open 计入 stale） | stale 过滤 |
| TC-R06 | 三条 open：`last_seen` = now-30d+1s / now-30d / now-30d-1s | `--report` | 「超 30 天」= 严格大于：仅 `now-30d-1s` 入选；恰好 30d 与 30d-1s 不入选 | stale 边界三档（±1s） |
| TC-R07 | 至少 1 条 open | `--report` | exit 0（有欠账也成功，报告不是门禁） | 有欠账不判失败 |
| TC-R08 | 两片各跑 1 次（分别 2 失败 / 1 失败） | `--report` | 进度段含两片行，字段齐全（`last_run`/`last_ok`/`last_ms`/`last_fails`/`runs`/`total_fails`）；`total_fails` 分别 == 2 与 1；`runs` 各 1 | 进度表输出（total_fails 语义见待定 A-9） |
| TC-R09 | 任意台账状态 | `--report` 前后对比三表 | 三表行数与 `test_debt` 全行内容逐字节不变（只读） | report 零写入 |

### F. CLI 参数校验与退出码

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-F01 | 空库 | `--init` 连跑 2 次 | 两次均 exit 0；第二次无 ERROR 行；两次后两表均可查；预置行数据不变 | 幂等（exit 0、不报错） |
| TC-F02 | 空库 | `--init` | `test_debt` 含 11 列（id/test_id/domain/first_seen/last_seen/context/status/fixed_by/waived_reason/waived_at/note）；`patrol_shard` 含 7 列（shard/last_run/last_ok/last_ms/last_fails/runs/total_fails）；`test_id` UNIQUE（重复 INSERT 报错或 `PRAGMA index_list` 可证） | 表结构契约 |
| TC-F03 | 任意 | `--help` | exit 0；输出含 9 个入口字样：默认跑一片 / `--shard` / `--shards` / `--init` / `--report` / `--register` / `--resolve` / `--waive` / `--help` | 用法完备 |
| TC-F04 | 任意 | `--shard bogus` 与 `--shard ""` | 两种均 exit 2；输出列出可选分片名（至少 13 个：12 轮转 + `be-all`）；三表零变化 | 未知分片（含空串） |
| TC-F05 | 任意 | `--shards 0` / `--shards -1` / `--shards abc` / `--shards ""` | 四种均 exit 2；输出含用法提示；零分片被执行；三表零变化 | n ≥ 1 校验 |
| TC-F06 | 任意 | `--foo`（未知子命令） | exit 2；提示 `--help`；三表零变化 | 未知子命令 |
| TC-F07 | 任意 | `--register`（无参）/ `--register "" --context x` / `--register id`（缺 `--context`） | 三种均 exit 2；提示 `--context` 必填；三表零变化 | register 必填项 |
| TC-F08 | 台账无该 id | `--resolve nope::TestX` | exit 2；输出提示改用 `--register`；三表零变化 | resolve 未知 id |
| TC-F09 | 台账无该 id | `--waive nope::TestX --reason x` | exit 2；提示未知 id；三表零变化 | waive 未知 id |
| TC-F10 | 记录 open | `--waive id`（缺 `--reason`）/ `--waive id --reason ""` | 两种均 exit 2；记录仍 open、`waived_reason` 仍 NULL | waive 必填项 |
| TC-F11 | 任意 | `--register id --context x --bogus 1` | exit 2（严格解析，不静默忽略未知选项）；三表零变化 | 未知选项 |
| TC-F12 | 任意 | 逐一执行本组全部 exit 2 用例 | 每个用例三表行数前后相等；stdout 无 `✓` 成功标记；错误信息在 stderr 或带前缀的 stdout 行 | 退出码 2 零副作用 |

### P. 资源预检

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-P01 | 注入 loadavg 文件 `0.50 1.00 1.50 ...` | 无参数巡检 | stdout 不含负载告警行；巡检正常（摘要 + 记账齐备） | 低负载无噪声 |
| TC-P02 | 注入 `4.00 ...` 与 `4.01 ...` | 两次巡检 | 阈值 >4：4.00 无告警；4.01 有告警（含 1 分钟负载数值与「延后/错峰」建议字样） | 阈值边界 |
| TC-P03 | 注入 `8.30 ...` | 巡检 | 打印告警后**仍继续执行分片**（摘要存在、台账与事件照写、exit 按测试结果 0 或 1；绝不因预检 exit 2） | 只提醒不阻断 |
| TC-P04 | 造 pidfile 白名单外的 build/浏览器自动化进程特征（或注入进程清单文件） | 巡检 | 打印并发告警行（含命中的进程类别）；仍继续执行 | 并发检出（缝见待定 A-6） |
| TC-P05 | 注入高负载 | 巡检（stdout 全量捕获） | 告警行行号 < 分片摘要行行号（预检在跑之前）；预检本身不写三表 | 预检时机 |
| TC-P06 | loadavg 文件不存在 / 不可读 | 巡检 | 不报错退出、不阻断；摘要与记账照常；允许打印「无法读取负载」提示 | 预检 fail-open |

### X. 并发与输入安全

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-X01 | tmp 库已 init；fake runner 睡 3s、全绿 | 两个 shell 同时跑 `--shard be-admin` 与 `--shard be-reader` | 两进程各自 exit 0；输出无 `SQLITE_BUSY` / `database is locked`；`patrol_shard` 两行都在；`patrol.check` 恰 2 条；无数据丢失 | 并发两片写库 |
| TC-X02 | TC-X01 跑完 | 查 `PRAGMA journal_mode` | 返回 `wal`；写入期存在 `-wal` 文件 | WAL 生效 |
| TC-X03 | 已 init | `--shard "be-admin'; DROP TABLE test_debt;--"` | exit 2（未知分片）；`sqlite_master` 中 `test_debt` 仍存在；表内容行数不变 | 分片名 SQL 注入式输入 |
| TC-X04 | 已 init | `--register "internal/x::Test'; DROP TABLE test_debt;--" --context test-debt-patrol` | exit 0；读回 test_id 逐字节等于输入；表仍在；`--report` 正常输出（无 SQL 语法错） | test_id 注入式输入 |
| TC-X05 | 已 init | `--register 'front/app/x.test.ts::a > b "q" / [x]' --context c` 再 `--resolve` 同 id | 读回逐字节相等（含 `>` `/` `"` `[` `]`）；`--resolve` 命中同一行（读写同一转义路径），`status=fixed` | 特殊字符转义闭环 |
| TC-X06 | 已 init | `--register` 一个 4KB 长度 test_id | 口径二选一且前后一致：接受并原样存回（读回长度 == 输入长度，无截断）或 exit 2 明确拒绝；**不得出现「已截断入库」** | 超长输入（见待定 A-7） |
| TC-X07 | 已 init | `--register id --note $'第一行\n第二行 "引号"' --context c` | note 读回逐字节相等（含换行）；`--report` 输出不因换行破坏可读结构 | note 含换行/引号 |

### SUR. 存量摸底复核（已完成制品）

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-SUR01 | survey.md §3 两条存量红（`chunk-error-fallback` 整文件、`useOnboarding` 17 用例） | `--init` 后按清单登记（context=存量摸底）或确认巡检自动登记 | `--report` 中两条均 open 且发现语境可辨识；test_id 与 survey §3 表逐字一致（`front/app/plugins/chunk-error-fallback.test.ts::<file-level>` + `front/app/composables/useOnboarding.test.ts::<17 条用例级>`） | spec「摸底产出初始欠账」 |
| TC-SUR02 | design.md 决策 2 回填 + 实测耗时 | 静态复核 | design.md / survey.md 记录三数据：存量红总数（2 文件 18 用例）、后端 `-short` 全量 42s、前端单片耗时表；分片枚举 12+1 与之匹配（「后端整体一片可行」结论保留 6 domain 片）；TC-B12 的目录计数为复核抓手 | spec「摸底数据决定分片粒度」 |

### W. 归档记账纪律与门禁不变

| ID | 前置状态 | 触发 | 期望结果（可核对断言） | 边界/变体说明 |
| --- | --- | --- | --- | --- |
| TC-W01 | tasks 5.1 声称已改 | `grep -n 'test-patrol.sh --register' docs/reference/开发执行规范.md` | §11.4 自检清单节内命中 ≥1 行，内容为「域外红 → 登记台账」指引 | 归档撞见域外红（纪律条款） |
| TC-W02 | 同上 | `grep -n 'test-patrol' AGENTS.md` | 命中「巡检顺手跑一片」一行且含脚本名 `scripts/test-patrol.sh` | 触发纪律落地 |
| TC-W03 | tasks 5.2 | `grep -n 'test-patrol.sh --register' .pi/extensions/test-scope-guard.ts` | 归档语境放行提示文案含记账指引 ≥1 命中 | 提示语（纯文案） |
| TC-W04 | tasks 5.2 | 同上对 `.pi/extensions/spec-gate.ts` | 归档指引文案含记账指引 ≥1 命中；且 `bash .pi/extensions/tests/run-smoke.sh` exit 0（含文案断言） | 同上 |
| TC-W05 | 门禁硬语义 | `git diff` 上述两扩展 + 跑 `.pi/extensions/tests/run-smoke.sh`、`quality-gate.behavior.smoke.cjs` | 差异仅文案行；test-scope-guard 的 hard/soft 判定分支与 `reasonCode=full-go-test` 零改动；§11「本 change 影响包内的红必须修复」既有条目未被放宽（grep 断言仍在）；smoke 全绿 | 本 change 红不走豁免通道的机器证据（执行行为归人工） |

## 3. 边界值清单

| 编号 | 变量 / 输入 | 取值 | 期望 | 用例 |
| --- | --- | --- | --- | --- |
| B-01 | `--shards n` | 0 / -1 / abc / 空串 | 全 exit 2，零执行零写入 | TC-F05 |
| B-02 | `--shards n` | 1（最小合法） / 12（等于总数） / 13 / 99（超总数） | ≤总数跑 n 片；超总数跑完全部且不重复 | TC-B08 / TC-B10 / TC-B09 |
| B-03 | `--shard` 名字 | 未知名 / 空串 / 注入式串（含 `'`、`;`、`--`） | 全 exit 2 + 列出可选片；库结构无损 | TC-F04 / TC-X03 |
| B-04 | 进度表 `last_run` | 全 NULL（冷启动） / 部分 NULL / 无 NULL | NULL 最优先；其次最早时间；平局按静态表顺序 | TC-B01 / TC-B04 / TC-B05 |
| B-05 | 轮转集合 | 12 片轮转 + `be-all` 显式片 | 无参数/`--shards` 永不选中 `be-all`；`--shard be-all` 可跑 | TC-B06 / TC-B07 |
| B-06 | 台账规模 | 空库（0 行）→ 单条 → 多条混合三态 | 各操作在空库合法（不报错、提示空态）；聚合数字精确 | TC-A01 / TC-R01 / TC-R02 |
| B-07 | 同一测试连续两轮红 | 第 1 轮新增、第 2 轮复现 | 行数不变，`last_seen` 前进，`first_seen` 冻结 | TC-A03 |
| B-08 | 同批次重复标识 | 同 test_id 出现 2 次 | 去重为 1 条记录 + `fails` 长度 1 | TC-A04 / TC-C11 |
| B-09 | 终态记录复现 | `fixed` / `waived` 各一轮后再次被检出 | 迁回 open + `reopened from <status>@<date>` 痕迹；终态字段保留 | TC-A06 / TC-A07 |
| B-10 | stale 判定 | 恰好 30 天 / 30 天 − 1s / 30 天 + 1s | 「超 30 天」= 严格大于：仅 +1s 入选 | TC-R06（口径见待定 A-2） |
| B-11 | stale 状态范围 | open 老记录 / open 新记录 / fixed 老记录 | 仅 open 且超期计入 | TC-R05 |
| B-12 | 前端 filter 传参 | 带 `--`（坏形式，仅作反例断言）vs 不带 `--` | 实际命令不得含 `--`；DRY_RUN 输出无 `--` 元素 | TC-C02 / TC-C03 |
| B-13 | test_id 字符集 | `>` `/` `[` `]` `(` `)` `"` `'` `;` `--` 中文、换行（note） | 读回逐字节相等；SQL 不注入；表格输出不崩 | TC-X04 / TC-X05 / TC-X07 |
| B-14 | test_id 长度 | 1 字符 / 常见长度 / 4KB 超长 | 超长按定稿口径「接受原样存回」或「exit 2 拒绝」，不得截断 | TC-X06 |
| B-15 | 并发写库 | 两片同时执行（写入期重叠） | 无 `SQLITE_BUSY` 报错、两片进度与事件都在 | TC-X01 / TC-X02 |
| B-16 | 库不可用 | 库缺失 / 不可写 / 他人库（app_id≠SYNT） / 未来版本（user_version>1） | 一律 fail-open：告警 + 判不建表不半写 + 巡检查出结果与退出码不受影响 | TC-V06 / TC-V07 / TC-V08 |
| B-17 | runner 退出码 | 0 + 全绿 / 1 + 有 FAIL / 1 + 零 FAIL 行 | 0 / 1 / 2（无法解析明细，见待定 A-5） | TC-C12 |
| B-18 | load average（1min） | 0.5 / 4.00 / 4.01 / 8.30 / 读不到 | >4 提示（4.00 不提示）；一律不阻断；读不到 fail-open | TC-P01 / TC-P02 / TC-P03 / TC-P06 |
| B-19 | 表结构约束 | 非法 status（如 `closed`） / 重复 `test_id` | 拒绝（CHECK 或脚本层） / 拒绝（UNIQUE） | TC-A14 / TC-F02 |
| B-20 | `--init` 重复 | 连续 2 次 / 已有数据后再执行 | exit 0、无报错、数据与事件表不动 | TC-F01 / TC-V09 |

## 4. 覆盖对照表（13/13）

> spec 实为 11 个 Scenario（4 Requirement）+ 2 个（harness-fact-log）；标题逐字抄自 delta spec。

### 4.1 `test-debt-patrol/spec.md`

| # | Requirement | Scenario（逐字） | 本文件用例 |
| --- | --- | --- | --- |
| 1 | 测试欠账台账 | 巡检发现新红自动登记 | TC-A02（解析落点 TC-C05/C10/C09） |
| 2 | 测试欠账台账 | 已有记录不重复登记 | TC-A03 / TC-A04 |
| 3 | 测试欠账台账 | 状态迁移留痕 | TC-A06 / TC-A07 / TC-A08 / TC-A09 / TC-A11 / TC-A13 |
| 4 | 测试欠账台账 | 台账可查询 | TC-R01 – TC-R09 |
| 5 | 滚动分片巡检 | 单分片巡检落账 | TC-B01 / TC-B02 / TC-C13 / TC-C14 / TC-V01 |
| 6 | 滚动分片巡检 | 资源红线——低并发执行 | TC-C02 / TC-C03 / TC-P01 – TC-P06 |
| 7 | 滚动分片巡检 | 分片内测试全绿 | TC-A01 / TC-A09 / TC-A10 / TC-C04 / TC-C14 |
| 8 | 归档语境欠账记账 | 归档撞见域外红 | TC-A05（登记路径）+ TC-W01 / TC-W03 / TC-W04（文案）；执行行为归人工（§5 MAN-03） |
| 9 | 归档语境欠账记账 | 本 change 红不适用记账豁免 | TC-W05（机器可核对部分：未新增豁免通道 + 门禁语义不变）；实际修复闸门归归档门禁与人工 |
| 10 | 存量摸底先行 | 摸底产出初始欠账 | TC-SUR01 |
| 11 | 存量摸底先行 | 摸底数据决定分片粒度 | TC-SUR02（抓手 TC-B12） |

### 4.2 `harness-fact-log/spec.md`

| # | Requirement | Scenario（逐字） | 本文件用例 |
| --- | --- | --- | --- |
| 12 | 巡检事件记账（patrol.check） | 分片巡检完成落事件 | TC-V01 / TC-V02 / TC-V03 / TC-V04 / TC-V10 |
| 13 | 巡检事件记账（patrol.check） | 台账登记不双写事件 | TC-V05 |

## 5. 不在本文件范围（smoke 跑不了，留人工 / 长跑验收）

| ID | 场景 | 操作 | 期望 | 对应 |
| --- | --- | --- | --- | --- |
| MAN-01 | 真实 12 片全跑一轮的时长统计 | 分 3 天按纪律跑，记录每片 wall time（含外网超时噪声） | 每片耗时记录成表；超 5 分钟的片是否再拆的依据 | survey §4.2/§4.3 后续优化候选 |
| MAN-02 | 分片 filter 与实跑文件数一致 | 真实跑最便宜的前端片（`fe-discovery`，9 文件） | vitest 汇总 `Test Files` 数与静态枚举一致（防 filter 被吞） | TC-C03 的真实端到端补强 |
| MAN-03 | 归档语境实机走查 | 挑一个真实 change 归档，验证节实测撞见域外红 | `--register` 登记后继续归档；验证节记录「已记账」 | spec Scenario 8 |
| MAN-04 | 跨天/跨会话轮转推进 | 连续 3 个会话各跑 1 片（tasks 10.2） | 每次各推进一片、最久未巡优先、`patrol.check` 落库 | TC-B01/B03 的真实时间验证 |
| MAN-05 | harness-retro 消费 | 跑 `bash scripts/harness-retro.sh` | 报告/看板出现 `patrol.check` 频次与台账欠债趋势段（展示形式从简） | tasks 4.3 |
| MAN-06 | 跨平台执行前置 | Windows/WSL 宿主的巡检调用 | 本 v1 只在 Linux 本机直跑；跨平台策略随 `standard/frontend/testing.md` 既有约定，不在本 change 出圈 | tasks 9.2 |

## 6. 歧义与待定（实现阶段定稿，本文件不擅自放宽/收紧）

- **A-1 后端 test_id 的包口径**：本文件按「剥 module 前缀 `syntopica-backend/`、取 `internal/<domain>/...`」（TC-C05）。若实现保留完整 module 路径或写成 `./internal/...`，须回改 TC-C05/C06/C07/C08 与 smoke。
- **A-2 「按 first_seen 排序的 open 清单」方向**：本文件口径 = 升序（最老欠账在前，便于排期），TC-R04 按此写。若实现降序须同步。
- **A-3 终态记录再 `--resolve`/`--waive`**：本文件口径 = 幂等 no-op + exit 0 + 提示（TC-A12，理由：终态理由不可被事后改写）。若实现改为 exit 2，须同步 TC-A12 与 F 组。
- **A-4 status 三态约束落点**：DDL `CHECK` 约束 或 脚本层拒绝，二选一即可，但 TC-A14 与 smoke 断言必须与实现一致。
- **A-5 副作用失败的两条口径**：①runner 非 0 且零 FAIL 解析 → exit 2（TC-C12③，避免把跑不起来当"测试红"记）；②事件/台账写入失败 → fail-open，不改变巡检查出的退出码（TC-V06/V07/V08）。两条均为黑白选择，实现定稿后同步本文件。
- **A-6 资源预检的并发检出缝**：TC-P04 依赖可注入的进程清单（或读表 `.pi/run/` pidfile 白名单 + 进程特征）。若实现不提供缝，TC-P04 降级为「源码断言检出逻辑存在 + 人工造进程验证」。
- **A-7 超长 test_id**：默认口径 = 接受并原样存回（TC-X06 断言无截断）；若实现选择拒绝，须 exit 2 且不落半条记录。
- **A-8 spec 文本与实测冲突**：`test-debt-patrol/spec.md` 正文写 `pnpm test:unit -- <pattern>`，survey §4.1 实测证明带 `--` 会吞 filter 跑全量。本文件以主线程契约 + survey 为准（TC-C02/C03 断言命令中无 `--`）；spec 正文措辞若在实现期一并修订，Scenario 标题不变即可，不影响 scenario-trace 对账。
- **A-9 `patrol_shard.total_fails` 语义**：本文件口径 = 每次巡检 `fails[]` 长度累加（含重复复现，TC-R08/C14）。若定为「唯一失败标识去重累计」，须同步这两条。

## 7. 主线程裁决定稿（2026-09-17，覆盖 §6 的 A-1 – A-9）

> 断言判据归主线程（开发执行规范 §2）；以下为定稿口径，实现与 smoke 断言一律以此为准（不再二选一）。

| 编号 | 裁决定稿 | 理由 |
| --- | --- | --- |
| A-1 | 后端 test_id 用 `internal/<domain>/...::<TestName>`（剥 module 前缀 `syntopica-backend/`，不带 `./`） | 与 `go test ./internal/...` 调用形式对应，可读且与 change-scope.sh 的 domain 词汇一致 |
| A-2 | open 清单按 `first_seen` **升序**（最老欠账在前） | 排期先还老债；stale 告警同序，人工扫一眼即知 |
| A-3 | 终态（fixed/waived）再 `--resolve`/`--waive` = **幂等 no-op + exit 0 + 提示** | 终态理由不可被事后改写；exit 2 会误伤机械调用链 |
| A-4 | status 三态约束**双侧**：DDL `CHECK(status IN ('open','fixed','waived'))` + 脚本层先校验 | 库被脚本以外的手工 SQL 误写也不出现第四态 |
| A-5 | ① runner 非 0 且零 FAIL 解析 → **exit 2** + 提示「无法解析失败明细」，不新增台账行，但仍写 `patrol.check(ok=false, fails=[])` 与进度；② 台账/事件写入失败 → **fail-open**（告警可见，不改巡检查出退出码） | ① 不把「跑不起来」当测试红记账；② 记账坏不得阻断巡检信号（台账坏时至少 stdout 有结果） |
| A-6 | 资源预检提供注入缝 `TEST_PATROL_PROCESS_LIST`（每行一条 cmdline）；未提供时读 `ps` 实际进程 | 确定性可测；缝缺失才降级源码断言 |
| A-7 | 超长 test_id（4KB）**接受并原样存回**（不截断） | 测试标识天然可能很长（嵌套 describe 链）；截断会造成假去重 |
| A-8 | 本 change 内修 spec 正文为 `pnpm test:unit <filter...>`（不带 `--`），Scenario 标题不动 | 与 survey §4.1 实测一致；标题不变不影响 `scenario-trace.sh` 对账 |
| A-9 | `total_fails` = 每次巡检 `fails[]` 长度**累加**（含重复复现） | 该值是「该片失败压力」的流水指标，不用于台账统计（台账自身有 open/fixed 计数） |
