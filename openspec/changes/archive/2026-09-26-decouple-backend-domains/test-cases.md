# test-cases: decouple-backend-domains

> 复杂档（multi-module 协议类：跨 9 域包边界重构 + depguard 规则 + AutoMigrate 幂等 + 路由零漂移）。
> 主链路按 spec `backend-package-boundaries` 四 Requirement 串节拍：**深路径拒绝 → 门面放行 → 框架归位 → 白名单登记**。

## 基线（2026-09-26 复测，精确 grep 口径）

- 生产深路径域间边 **9 条 / 14 文件**：admin→tagmanagement 3、admin→reader 2、admin→topicgraph 1、admin→datasources 1、reader→tagmanagement 2、dataenrichment→{admin,reader,datasources} 各 1、datasources→dataenrichment 2。
- 域间深路径测试 **9 文件**（_test.go，depguard 豁免层）：admin→reader 2、admin→tagmanagement 2、admin→topicgraph 2、dataenrichment→topicgraph 1、reader→tagmanagement 1、reader→topicgraph 1。
- 豁免层（app/platform 前缀）深路径 import 不受 depguard 约束，但其中引用**被迁移符号**的须随批次改 import：app→admin 1（runtime_test.go）、platform→admin 1（analysispause 测试）、platform/articlerefs 1、platform/database 13（dataenrichment/topicgraph 模型）。
- tasks.md 1.2 行「15 个跨域 import 测试文件」为设计期旧口径；实现期实测域间 9 + 豁免层受影响若干，以本表为准登记。

## 故事 S1: 开发者在域 A 引用域 B，编译期边界把他拦在门面上（锚 Requirement: 业务域之间只许 import 对方 root 门面包）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 域 A 非测试文件 import `internal/tagmanagement/handler`（深路径插桩） | 深路径跨域 import 被拒 | `golangci-lint run` 报 depguard 错误并指出 root 门面替代 | lint 工具链 | 人工：临时插桩→lint 报错→撤销（task 5.2 验证输出留档） |
| 2 | 域 A 非测试文件改 import `internal/tagmanagement`（root 门面） | root 门面 import 通过 | depguard 放行，`golangci-lint run ./...` 退出码 0 | lint 工具链 | 人工：task 8.2 命令+退出码 0 |
| 3 | 域 A 的 `_test.go` import 域 B 子包 | 测试文件豁免深路径 | depguard 放行（测试不产生生产耦合） | lint 工具链 | 人工：9 个既有跨域 _test.go 保留深路径且 task 8.2 全绿 |
| 4 | 全仓收敛后反向 grep：生产深路径归零 | （隐含不变量） | task 8.6 grep 空输出 | grep 对账 | 人工：`grep -rnE '..." internal/{admin,reader,topicgraph,dataenrichment} --include='*.go' \| grep -v _test` 空 |
| 5 | dataenrichment 对 admin 的 import 归零 | 业务域不依赖其他域的框架类型（同 spec R2 场景，节拍衔接） | task 8.5 grep 空输出 | grep 对账 | 人工：task 8.5 命令空输出 |

## 故事 S2: 横切调度框架从 admin 搬到 platform，所有借用方编译通过且行为不变（锚 Requirement: 横切基础设施归 platform）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | `internal/platform/scheduler` 建包，base/registry/pause/persistence + SchedulerTask 随迁 | 域内 job 定义引用 platform 框架（前置） | `go build ./internal/platform/scheduler/...` 通过；框架配套测试随迁后 PASS | go build/test | task 2.1 + change-scope 判定包测试（task 2.4） |
| 2 | admin/scheduler 的 job_*.go 改 import 新框架路径（文件留原地） | 域内 job 定义引用 platform 框架 | job 文件 import `internal/platform/scheduler` 且 lint 通过 | go build | task 2.2 `go build ./internal/admin/...` |
| 3 | dataenrichment/scheduler_jobs.go、app/runtime.go、platform 测试改 import | 业务域不依赖其他域的框架类型 | `go build ./...` 通过；task 8.5 grep 空 | go build + grep | task 2.3 / 8.5 |
| 4 | 既有 scheduler 测试（pause_test 等）随包迁移断言不变 | （迁移不变量） | 迁移包测试全绿 | go test | task 2.4（change-scope 判定） |

## 故事 S3: 单域模型下放所属域，AutoMigrate 幂等、白名单登记不漏（锚 Requirement: 共享模型白名单）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 26 个单域模型迁至各域 models/ 子包（struct 字段与表名 tag 零改动） | 单域模型不落在 models（正向） | `go build ./...` 通过；diff 仅包路径 | go build + diff | task 3.2/4.1/4.2 验证命令 |
| 2 | `platform/database` AutoMigrate 补新模型包 import 后启动 | （迁移不变量） | migrate 空跑无 DDL 变更、无 error（日志 grep `ALTER\|CREATE INDEX` 无新增） | 真库启动 | 人工：task 6.3 启动后端+日志 grep |
| 3 | internal/models 剩 18 项白名单在 `.golangci.yml` depguard 配置旁注释登记 | 白名单模型新增需登记（正向登记义务） | 注释清单与 `ls internal/models/*.go` 导出 struct 一一对应 | grep 对账 | 人工：task 4.5 对账 grep 脚本 |
| 4 | 白名单外模型若回流 models（负向） | 单域模型不落在 models（负向） | 该模型必须已登记白名单+理由，否则 review 拒绝 | 人工 review | 人工：change review 检查项（无自动化，spec 原文） |

## 故事 S4: depguard 规则上线后 lint 即合规证明，豁免必须留痕（锚 Requirement: depguard 边界规则持续生效）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | `.golangci.yml` 新增 depguard 规则（域间仅 root 放行、app/platform 豁免、_test 豁免） | lint 通过即边界合规 | `golangci-lint run ./...` 退出码 0 | lint 工具链 | 人工：task 8.2 |
| 2 | 临时插桩深路径 import → lint 报 depguard | 深路径跨域 import 被拒 | 报错输出含 depguard 字样，验证后撤销 | lint 工具链 | 人工：task 5.2 输出留档 |
| 3 | 假设某 change 需临时豁免 → 配置注释登记 | 豁免留痕 | 豁免注释含 change 名+解除条件；无注释豁免视为违规 | 配置 review | 人工：`.golangci.yml` 豁免段 grep 检查（本 change 目标零豁免项） |

### 变体走查（五组固定清单）

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| 1 | **输入变体**：import root 包裸路径（`internal/tagmanagement`）vs root 下 Go 文件（wire.go re-export 符号） | 两者都放行——root 门面包本身合法 | lint | task 8.2 |
| 2 | **输入变体**：域 A→域 B models/ 子包（`internal/tagmanagement/models`） | 拒绝——models/ 同属深路径，模型引用必须经 root re-export | lint | task 5.2 插桩变体（tagmanagement/models 路径） |
| 3 | **输入变体**：app→任意域深路径、platform→任意域、任意→platform/models | 放行——装配层与基础设施豁免 | lint | task 8.2（现状即有 app/platform 深路径且绿） |
| 4 | **前置变体**：迁移中间态（某域已建但消费方未改） | 每批收口 `go build ./...` 全绿；批内顺序设计 D6（先建目标包→移模型→移 service→移 handler→改消费方→build） | go build | 各批任务验证命令 |
| 5 | **前置变体**：测试文件在豁免清单里但引用了被迁移符号 | 豁免 ≠ 免改：引用符号搬家后 import 路径仍须更新否则编译红 | go build | Batch 1/2/3 各批测试迁移 |
| 6 | **时间窗口变体**：~~不适用~~（无时间窗口语义；划除留痕：包结构无时间维度） | — | — | — |
| 7 | **幂等变体**：AutoMigrate 二次启动（模型已迁包） | 幂等空跑：无 DDL 变更无 error | 真库启动 | 人工：task 6.3 |
| 8 | **幂等变体**：`golangci-lint run ./...` 重跑 | 幂等：重跑退出码仍 0 | lint | task 8.2 重跑 |
| 9 | **可用性变体**：~~误输入反馈/空态/错误态不适用~~（无用户交互界面；划除留痕：纯后端结构重构）；错误态以**编译红/lint 红**为开发者可见反馈：深路径误写时 lint 错误信息须指出合法替代（root 门面包） | lint 错误信息可操作 | lint | task 5.2 输出检查 |
| 10 | **可用性变体**：迁移期间开发者 pull 到中间批次 | 每批独立可 revert 且全仓编译绿（Migration Plan 四批 commit） | git/go build | 各批收口验证 |

### 继承与调整（问句⓪：architecture-docs 为 MODIFIED，须逐行处置）

> test-assets.sh 反查：`backend-package-boundaries` 为 ADDED 新 capability，主 specs/archive 均不存在 → **N/A 无旧资产**。`architecture-docs` 有主 specs 9 Requirements/18 Scenarios，无历史 change delta；本 change MODIFIED 其中 3 个 Requirement（backend.md 目录结构 / backend.md 技术栈 / runtime.md SchedulerRegistry），被 MODIFIED 覆盖的旧 Scenario 处置如下；未列出的 6 个 Requirement（backend.md 调度器清单、runtimeinfo/AutoTagMerge 禁引、overview.md、tracing.md、data-flow.md）不受影响照旧。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| backend.md: cmd directory matches code | 改语义（目录树加 discovery/、各域 models/、platform/scheduler/） | 无自动化（文档检查） | 人工：task 7.1 按 delta spec 逐 Scenario 复核 |
| backend.md: internal directory matches code | 改语义（同上） | 无自动化 | 人工：task 7.1 |
| backend.md: platform subpackages match code | 改语义（platform 清单新增 scheduler/） | 无自动化 | 人工：task 7.1 + `ls backend-go/internal/platform/` 对照 |
| backend.md: Go version is correct | 继承（1.25 不变） | 无自动化 | 人工：task 7.1 复核 |
| backend.md: No reference to removed cron dependency | 改语义（新增断言：调度器框架描述改指 internal/platform/scheduler，job_*.go 留 admin/scheduler） | 无自动化 | 人工：task 7.1 |
| runtime.md: No references to removed runtimeinfo interfaces | 继承（禁引清单不变，框架位置描述更新） | 无自动化 | 人工：task 7.1 |
| runtime.md: No references to removed worker package functions | 继承（不变） | 无自动化 | 人工：task 7.1 |
| runtime.md: Scheduler list matches runtime.go | 继承（9 个调度器清单不变——纯搬家不注册新调度器） | 无自动化 | 人工：task 7.1 对照 runtime.go |
| 域间跨域测试 9 文件（生产耦合豁免层） | 继承（断言不变，import 随迁移符号改路径） | job_firecrawl_test.go / accept_transaction_test.go / poll_test.go / pause_test.go / job_daily_report_test.go / job_daily_report_window_test.go / signal_discovery_test.go / feed_service_test.go / article_refs_test.go | 照跑（回归网）；article_refs_test.go:18 的 topicgraph/repository blank import 随包路径更新 |
| 豁免层引用被迁移符号的测试 | 继承（import 改新路径） | runtime_test.go / runtime_shutdown_test.go / health_gate_compose_test.go / helpers_test.go / platform/database 13 个迁移测试 | 照跑（回归网） |

### 白盒附加（复杂档：depguard 规则分支表 + AutoMigrate/路由边界）

## 分支表：depguard 规则分支

| # | 条件/分支 | 输入 | 期望 | 测试用例名/落点 |
| 1 | import 目标=同域子包 | admin/handler import admin/service | 放行（域内自由） | task 8.2 全量 lint（现状即有） |
| 2 | import 目标=他域 root | reader import tagmanagement | 放行（门面合法） | task 8.2（现状 4 文件） |
| 3 | import 目标=他域 handler/service/repository 深路径 | 插桩 `internal/tagmanagement/handler` | 拒绝，报 root 替代 | task 5.2 插桩验证 |
| 4 | import 目标=他域 models/ 子包 | 插桩 `internal/tagmanagement/models` | 拒绝（models 同属深路径） | task 5.2 插桩变体 |
| 5 | import 来源=app/ 前缀 | app/runtime.go import admin/scheduler 旧路径→新 platform 路径 | 豁免该规则（装配层自由）；Batch 1 后改 platform/scheduler 照常编译 | task 2.3 `go build ./...` |
| 6 | import 来源=platform/ 前缀 | platform/database import tagmanagement/models（Batch 3 后） | 豁免该规则（AutoMigrate 需引模型） | task 4.1 `go build ./...` |
| 7 | import 来源=*_test.go | reader/service/feed_service_test.go import tagmanagement/handler | 豁免（issues.exclude-rules path `_test\.go`） | task 8.2（9 文件保留） |
| 8 | import 目标=platform/* 或 internal/models | 任意域 import platform/scheduler、internal/models | 不在本规则 deny 面（白名单层） | task 8.2 |

## 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名/落点 |
| import 路径深度 | root（0 层）恰放行 / 1 层（models）恰拒绝 | deny 列表含 `internal/<域>/...` 而列表外 root 不匹配 | task 5.1 配置 + 5.2 插桩 |
| 豁免路径匹配 | `internal/app/...`、`internal/platform/...` 前缀 | files 规则按目录前缀命中，cmd/ 不在豁免面 | task 5.1 配置 review |
| AutoMigrate 模型清单 | 26 个迁移模型逐个在新包 import 后注册 | migrate 幂等（无 ALTER/CREATE INDEX 新增） | task 6.3 |
| 路由注册表 | `/api/discovery/*` 全部路径字符串逐条 diff | admin/routes.go 挪 discovery/routes.go 前后字符串集相等 | task 3.4 diff 验证 |
| 白名单计数 | 18 项（12 共享 + 6 跨层） | `.golangci.yml` 注释清单 ↔ `internal/models` 导出 struct 一一对应 | task 4.5 对账 grep |

## 不适用划除（留痕）

- 变体走查「时间窗口变体」「UI 可用性变体（界面反馈/空态）」不适用：纯后端结构重构，无时间语义、无用户界面；错误反馈形态=编译红/lint 红，已在变体 #9 覆盖。
- 「效果核对（问句④）」不触发：期望结果不依赖数据覆盖率/LLM 行为/外部服务，全部由编译器、lint、grep、启动日志机械判定。
- 「展示字段盘点（问句⑤）」不触发：不改任何数据结构字段语义，模型只挪包不改定义。

## 分支表结论（2026-09-26 实施验证）

| 分支# | 结论 | 证据 |
| --- | --- | --- |
| 1 同域子包放行 | ✓ 绿 | 全量 lint 0 issues（admin/scheduler→admin/repository 等域内引用未被拦） |
| 2 他域 root 放行 | ✓ 绿 | runtime/discovery service→reader root 等现存且 lint 0 issues |
| 3 他域 handler/service/repository 拒绝 | ✓ 绿 | task 5.2 插桩验证：topicgraph→reader/service 报 depguard |
| 4 他域 models/ 拒绝 | ✓ 绿 | deny 枚举含 models 前缀（同 3 的机制，前缀匹配含子包） |
| 5 app/ 来源豁免 | ✓ 绿 | app/runtime.go→admin/scheduler 等保留且 lint 0 issues |
| 6 platform/ 来源豁免 | ✓ 绿 | platform/testutil→tagmodels、database→tagmodels 保留且 lint 0 issues |
| 7 _test.go 豁免 | ✓ 绿 | 9 个跨域 _test.go 深路径保留且 lint 0 issues（!$test + exclusions 双层） |
| 8 platform/models 白名单层 | ✓ 绿 | 任意域 import platform/scheduler、internal/models 未被拦 |

边界值结论：import 深度 root 恰放行/1 层恰拒绝 ✓（分支 3/4 与 2 对照）；AutoMigrate 26+1 迁移模型幂等 ✓（6.3 日志 0 DDL）；路由 62 条字符串集合迁移前后 diff 空 ✓（3.4）；白名单 19=19 对账 ✓（4.5）。
