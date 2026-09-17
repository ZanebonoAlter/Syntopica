# test-debt-patrol Tasks

> 前置说明：任务 1（受控摸底）需**先停其它 pi 会话**（树莓派资源红线，`standard/frontend/testing.md` load-106 教训），开工前与用户确认时机。复杂档（proposal 声明 complex）：任务 2 前置 `test-cases.md`（台账状态机 ≥3 状态 + 巡检分片调度逻辑白盒用例，entry-gate 档位激活时提醒）。

## 1. 受控存量摸底（spec：存量摸底先行）

- [x] 1.1 用户确认窗口（停其它 pi 会话）后，分片跑后端 `go test -short ./internal/<domain>/...` × 5 + 骨架档 `go test -short ./internal/{platform,models,app}/... ./cmd/...`，逐片记录通过与失败用例、单片耗时 → 失败用例清单留 raw 输出
  - 实测（2026-09-17，`-short -count=1`，warm cache）：admin 5s / dataenrichment 10s / reader 12s / tagmanagement 4s / topicgraph 3s / skeleton 33s，**全绿、0 失败**；全后端整片 42s。明细见 `survey.md` §2
- [x] 1.2 分片跑前端 `pnpm test:unit -- --maxWorkers=2 <目录分组>`（components 子域/stores/composables 等静态分组），逐组记录通过与失败、单片耗时 → 失败用例清单留 raw 输出
  - 实测：**存量红 2 文件 / 18 用例**（`app/plugins/chunk-error-fallback.test.ts` 整文件 collect 失败；`app/composables/useOnboarding.test.ts` 17 例）；6 片耗时 4s~197s。明细见 `survey.md` §3/§4
  - ⚠️ 摸底同时发现调用陷阱：`pnpm test:unit -- <filter>` 会吞掉 filter 静默跑全量（3 次复现）——正确形式不带 `--`，已同步修 `standard/frontend/testing.md`
- [x] 1.3 汇总摸底结论：存量红总数与分布（登记台账作初始欠账，若台账未建先落 raw 清单在 tasks 验证节引用的文件）、全后端 `-short` 实测耗时、前端单片耗时 → 回填 design.md 决策 2 的分片粒度定夺
  - 结论：存量红 **2 文件 / 18 用例（全在前端）**、全后端 `-short` **42s**、前端 6 片 4~197s（全量一圈 ≈6.4 min）→ 后端保留 6 片 + `be-all` 整片；前端定 6 片。已回填 `design.md` 决策 2 与 `survey.md` §4.3；初始欠账清单在 `survey.md` §3（台账建成后登记）

## 2. 台账与脚本骨架（spec：测试欠账台账；test-cases 白盒用例先行）

- [x] 2.1 写 `test-cases.md`：台账状态机（open→fixed/waived、重复登记去重、stale 判定）+ 巡检分片调度（最久未巡优先、进度持久化、并发预检）白盒用例 → 覆盖 spec 全部 Scenario
  - 产出 `test-cases.md`（92 条用例：台账状态机 A15 / 分片调度 B12 / 执行引擎与解析 C14 / 事件记账 V10 / 报告聚合 R9 / CLI 校验 F12 / 资源预检 P6 / 并发与输入安全 X7 / 摸底复核 SUR2 / 归档纪律 W5；边界值 20 条；**13/13 Scenario 覆盖**）；§1 定义 7 个测试缝，§7 落主线程裁决定稿 A-1…A-9
- [x] 2.2 `scripts/test-patrol.sh --init`：events.db 建 `test_debt` 表（design 决策 1 结构，`test_id` UNIQUE）+ 分片进度表；重复执行幂等 → `sqlite3 .pi/harness/events.db '.schema test_debt'` 可见且二次执行不报错
  - 实测：`test_debt` 11 列（含 `CHECK(status IN ('open','fixed','waived'))` + `test_id` UNIQUE）、`patrol_shard` 7 列；`--init` 连跑 2 次均 exit 0；smoke TC-F01/F02/A14 覆盖
- [x] 2.3 脚本骨架：子命令路由（默认跑一片 / `--shard` / `--report` / `--register` / `--init`）、分片静态枚举（后端 6 片 + 前端按 1.2 实测分组）、load average >4 与 build/浏览器自动化并发预检提醒 → `bash scripts/test-patrol.sh --help` 各子命令可执行
  - 实测：`--help` 9 个入口（含额外 `--shards` / `--resolve` / `--waive`）；分片枚举 12 轮转 + `be-all`；预检实机报过两类告警（`load=4.16 > 4`、并发 esbuild 构建进程）且照常执行

## 3. 巡检与记账（spec：滚动分片巡检 + patrol.check 事件）

- [x] 3.1 分片执行引擎：后端片 `go test -short` / 前端片 `pnpm test:unit <filter> --maxWorkers=2`，解析失败用例为规范 `test_id` → 对一片已知全绿分片执行，退出码 0、零台账变更、进度表更新
  - 实测：`be-admin` 5162ms、`be-dataenrichment` 10024ms、`be-reader` 9459ms 均全绿 exit 0，台账零变更、进度表 `runs=1`；前端命令形态经 `TEST_PATROL_DRY_RUN=1` 逐参断言无 `--`（`pnpm test:unit app/features/tags --maxWorkers=2`）
- [x] 3.2 失败登记：新红插入 `open`（context=分片名）、已有 `open` 更新 `last_seen`，同批次去重 → 构造含已知失败的分片（临时注入失败测试或用 1.1 存量红所在片），验证台账新增/更新各一次不重复
  - 实测：真跑 `fe-discovery` 登记 5 条 open（第二轮 `last_seen` 前进、行数不变）；同批次去重补了「vitest 明细行尾耗时」导致的重复（真实输出 `× … 28ms` 与尾部 `FAIL` 段同 id），修后真跑恰 5 条 + smoke TC-C11/C11b（真实形态）双验
- [x] 3.3 `patrol.check` 事件落库（bash sqlite3 直写 events 表，change 列 NULL，payload `{shard,ok,ms,fails}`）→ 执行一片后 `SELECT payload FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1` 字段齐全
  - 实测：`{"shard":"fe-discovery","ok":true,"ms":15312,"fails":[]}`（`change` 为 NULL、`session_id` 取 PI_SESSION_ID）；失败轮 payload `ok=false` + 5 条 fails 逐字
- [x] 3.4 转绿闭环：分片全绿时将台账中该分片 `open` 记录迁 `fixed`（fixed_by=`patrol`）→ 用 3.2 构造的失败分片修复后重跑，验证状态迁移留痕
  - 实测：`fe-discovery` 第三轮全绿后，该片 5 条 `open` 全部迁 `fixed|patrol`，`first_seen` 保留（SQL 查证）

## 4. 查询与复盘接入（spec：台账可查询）

- [x] 4.1 `--report`：按 domain/状态/首见时间汇总 + `last_seen` 超 30 天 stale 记录提示 → 造临时台账数据跑 `--report`，输出含四类聚合与 stale 段
  - 实测：真实库输出含状态计数 / domain 聚合 / open 清单（`first_seen` 升序）/ stale 段（截止时间随 `--now` 计算）/ 进度表；smoke R 组 10 条覆盖空库、30 天 ±1s 边界、只读零写入
- [x] 4.2 `--register`：归档语境手工登记入口（测试标识 + context=change 名）→ 手工登记一条验证字段齐全
  - 实测：登记 2 条存量摸底欠账（`context=存量摸底`、`--domain fe-core/fe-composables`、中文 note），`--report` 可见；另补 `--resolve` / `--waive` 手工终态入口（终态再调用为幂等 no-op，见 test-cases §7 A-3）
- [x] 4.3 harness-retro 消费：`scripts/harness-retro.sh` 报告或效能看板补 `patrol.check` 频次与台账欠账趋势段（数据在即视为达成，展示形式从简）→ 跑一次 harness-retro.sh 可见巡检数据
  - 实测：`bash scripts/harness-retro.sh --days 7` ⑦效能看板新增「D 测试欠账巡检」子组：巡检频次 6 次（ok 4 / 失败 2，67% ok）+ 台账 open 0 / fixed 7；`--json` 有 `effectiveness.patrol` 对象 + `patrol.*` 扁平指标 8 键（可做 `--baseline` 准 A/B）；无表/零事件降级为「无巡检数据」（smoke H3/H4）

## 5. 归档记账纪律（spec：归档语境欠账记账；design 决策 4 软约束落地）

- [x] 5.1 `开发执行规范.md` §11.4 自检清单增补「域外红登记台账」条目；AGENTS.md 增补巡检顺手跑纪律一行 → 文档 diff 可见且与 spec 措辞一致
  - `开发执行规范.md`：§4.1 新增「全量兜底通道——滚动分片巡检」段 + §11.4 自检清单第二条「域外红登记台账」；`AGENTS.md`：AI Behavior Rules 新增「测试欠账滚动巡检」一行，并修正两处 `pnpm test:unit -- --maxWorkers=2` 坏形式（连带写明 `--` 吞 filter 陷阱）
- [x] 5.2 test-scope-guard 归档语境放行提示语、spec-gate 归档指引文案各补一句记账指引（纯文案，不改门禁语义）→ `.pi/extensions/tests/` 对应 smoke 断言更新并通过
  - 实改：`test-scope-guard.ts` 新增归档语境 info 提示（`ARCHIVE_DEBT_NOTICE`，仍零 block/policy.decision）；`spec-gate.ts` `buildBlockReason` 增一条记账指引；smoke 新增两条断言（policy-decision.smoke.cjs / spec-gate.smoke.cjs），两个 smoke 实测 exit 0

## 6. 存量还债（design 决策 5：小修大拆）

- [x] 6.1 按摸底清单还债：存量红 ≤5 处本 change 直接修复（每处修复后对应分片转绿、台账迁 `fixed`）；>5 处或涉及行为判断的登记后拆独立 fix change（change 名记入台账 note）→ 修复处 `go test -short <pkg>` / `pnpm test:unit <file>` 实测绿；拆出则在台账可查
  - 存量 2 处（≤5）本 change 内直接修：`chunk-error-fallback`（vitest 缺 `#imports` 别名 → 新增测试专用 stub + alias）、`useOnboarding`（Node 26 实验性 `localStorage` 全局遮蔽 happy-dom → setup 层补浏览器语义实现），产品代码零改动；实测 `pnpm test:unit <两文件> --maxWorkers=2` → 2 files / 20 tests 全绿；台账两条已 `--resolve --by test-debt-patrol` 结清
  - 巡检期间撞见他 change 在飞的 5 条红（`fe-discovery`）：按纪律登记 open、未跨线修复；他 change 完成后该片转绿，5 条自动迁 `fixed|patrol`（专属于台账的生命周期留痕）
- [x] 6.2 台账闭环实机验证：确认至少一条记录完整走完 open→fixed 全周期（3.4 的构造流转或 6.1 实修均算）→ `--report` 中该记录状态 fixed 且首见/修复时间齐全
  - 实测：真实库 7 条记录全部 `fixed`（5 条 `fixed_by=patrol`、2 条 `fixed_by=test-debt-patrol`），`first_seen`/`last_seen` 均非空（SQL 查证）；`--report` 三态计数 open 0 / fixed 7 / waived 0

## 7. 测试制品（smoke 脚本与扩展断言）

- [x] 7.1 `scripts/test-patrol.smoke.sh`：--init 幂等 / 分片枚举完整（后端 6 片+前端 N 片与脚本枚举一致）/ 台账状态机流转 / --report 聚合，参照 `check-standards.smoke.sh` 惯例 → `bash scripts/test-patrol.smoke.sh` 退出码 0
  - 实测：928 行 / **252 断言 / 0 失败**（10.8s，连跑 2 次稳定）；覆盖 A/B/C/V/R/F/P/X/SUR 组，真实长跑项（MAN-*）留 §10 验证与人工
- [x] 7.2 受影响扩展 smoke 更新（5.2 涉及的 test-scope-guard/spec-gate 断言）→ `bash .pi/extensions/tests/run-smoke.sh` 通过
  - 实测：`bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK（77 项断言全绿）；另加 `harness-log.smoke.cjs` 的 `patrol.check` 保留期断言、`harness-retro.smoke.sh` 的 H1–H4 巡检段断言（47/0）

## 8. 测试

- 本 change 影响的测试命令：`bash scripts/test-patrol.smoke.sh`、`bash .pi/extensions/tests/run-smoke.sh`、抽查分片 `go test -short ./internal/<domain>/...`（6.1 修复涉及包）、`pnpm test:unit <6.1 修复文件>`

## 9. 文档

<!-- doc-impact: standard -->
- [x] 9.1 `docs/reference/standard/backend/testing.md` — 巡检分片（后端）与台账查询章节（§运行 下新增「巡检分片」：`be-all` 42s 实测、6 片耗时、`-short -count=1` 口径、集成测试不在巡检范围的盲区声明）
- [x] 9.2 `docs/reference/standard/frontend/testing.md` — 巡检分片（前端、`--maxWorkers=2` 红线）与台账查询章节（新增 6 片表 + 实测耗时；并修正 `pnpm test:unit -- <参数>` 坏形式、写明 `--` 吞 filter 陷阱；连带修 `docs/reference/testing.md`、`docs/reference/development.md` 同源坏形式）
- [x] 9.3 `docs/reference/harness/pi-extensions.md` — patrol.check 事件词汇、test_debt 表、保留期（新增「测试欠账巡检（test-patrol.sh，脚本 + 事实库记账）」节）；另同步 skill `harness-facts`（事件表加 `patrol.check` 行 + 非事件状态表说明）与 skill `harness-retro`（⑦段 D 子组读法 + `patrol.*` 指标）
- [x] 9.4 `docs/reference/开发执行规范.md` — §4.1 门禁分层表巡检通道 + §11.4 记账条目（与 5.1 同一产物，已归位）

## 10. 验证

### 10.1 Scenario → 测试文件映射（scenario-trace.sh 对账用）

| Scenario | 测试文件 |
| --- | --- |
| 巡检发现新红自动登记 | scripts/test-patrol.smoke.sh |
| 已有记录不重复登记 | scripts/test-patrol.smoke.sh |
| 状态迁移留痕 | scripts/test-patrol.smoke.sh |
| 台账可查询 | scripts/test-patrol.smoke.sh |
| 单分片巡检落账 | scripts/test-patrol.smoke.sh |
| 资源红线——低并发执行 | scripts/test-patrol.smoke.sh |
| 分片内测试全绿 | scripts/test-patrol.smoke.sh |
| 归档撞见域外红 | 人工（规范条款 grep + 扩展文案断言；实机走查见 10.7 MAN-03） |
| 本 change 红不适用记账豁免 | 人工（§11.1 条件1 不变量人工对账） |
| 摸底产出初始欠账 | 人工（survey.md §3 清单一节 + 台账实机登记/结清留痕） |
| 摸底数据决定分片粒度 | 人工（survey.md §4.3 + design.md 决策 2 回填） |
| 分片巡检完成落事件 | scripts/test-patrol.smoke.sh |
| 台账登记不双写事件 | scripts/test-patrol.smoke.sh |

> 用例级对应关系（TC-* → 断言）见 `test-cases.md` §2 场景/分支表；上表按 scenario-trace 约定只写路径。

### 10.2 验证命令（每条均已实测）

- [x] 10.1 `bash scripts/test-patrol.sh --init && bash scripts/test-patrol.smoke.sh` → 两条 exit 0；smoke 输出 `test-patrol smoke：通过 252 / 失败 0`
- [x] 10.2 `bash scripts/test-patrol.sh` 连续 3 次 → 依次推进 `be-admin`(5162ms) → `be-dataenrichment`(10024ms) → `be-reader`(9459ms)，均 `✓ 全绿` exit 0、`patrol.check` 各落 1 条、全绿片零台账变更；冷启动 NULL 最优先 + 平局静态表序已由 smoke TC-B01/B03/B05 覆盖
- [x] 10.3 `bash scripts/test-patrol.sh --report` → exit 0，输出含三态计数 / domain 聚合 / open 清单（`first_seen` 升序）/ stale 段 / 分片进度表
- [x] 10.4 `bash .pi/extensions/tests/run-harness-smoke.sh` → exit 0（`SMOKE OK（77 项断言全绿）`，含 5.2 的 test-scope-guard/spec-gate 文案断言）
- [x] 10.5 存量摸底三项数据已回填 tasks 1.3 / design.md 决策 2 / survey.md：存量红 2 文件 18 用例（全在前端）、后端 `-short` 全量 42s、前端 6 片 4~197s（一圈 ≈6.4 min）
- [x] 10.6 `bash scripts/doc-impact.sh verify openspec/changes/test-debt-patrol` → exit 0（`声明: standard  文件:4 个`）；`bash scripts/check-standards.sh --change test-debt-patrol` → `通过 166 / 失败 0`
- [x] 10.7 `bash scripts/harness-retro.smoke.sh` → exit 0（`通过 47 / 失败 0`，含 H1–H4 巡检段）；`bash scripts/harness-retro.sh --days 7` → exit 0 且输出 D 子组（体检 6 次 / 台账 open 0 fixed 7）

### 10.3 人工/长跑项（smoke 覆盖不了，归档后按纪律继续观察）

| ID | 场景 | 期望 |
| --- | --- | --- |
| MAN-01 | 真实 12 片滚一轮的时长统计 | 跨若干天按纪律跑，记录每片 wall time（外网超时噪声）；超 5 分钟的片再拆 |
| MAN-02 | 分片 filter 与实跑文件数一致 | 已抽查：`fe-discovery` 实跑 `Test Files 9 passed (9)`，与静态枚举一致（防 filter 被吞） |
| MAN-03 | 归档语境实机走查 | 后续 change 归档撞见域外红时 → `--register` 登记后继续，验证节记「已记账」 |
| MAN-04 | 跨天/跨会话轮转推进 | 已实测连续 3 次推进（10.2）；跨天特性由持久化 + NULL/最早优先保证 |
| MAN-05 | harness-retro 消费 | 已完成（10.7）；观察窗口内 `patrol.checks` 是否持续 > 0 |
