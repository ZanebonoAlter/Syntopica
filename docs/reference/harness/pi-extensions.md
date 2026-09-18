# pi harness 扩展机制（全景 + 注入 + 门禁）

<!--
doc-impact-applies: .pi/extensions, .pi/workflows, .pi/constraint-injection.json, .pi/harness.json, scripts/harness/harness-retro.sh, scripts/harness/change-scope.sh
-->

> **权威源**：本文件是 pi harness 扩展**机制参考**的唯一权威——扩展全景表、约束注入/质量门禁的工作原理、事件账本入口。改 harness 相关代码或排查「为什么注入了/为什么被拦」先读这里。agent 日常行为红线（[回归]必修、测试范围、平台分流）在 AGENTS.md，流程编排在《开发执行规范》§0.6，两文互引不重复。事件考古查 skill `harness-facts`，改进复盘查 skill `harness-retro`。

## pi 扩展全景（`.pi/extensions/`）

**源码已入库**——`.gitignore` 的 `/.*` 排除后单独放行 `.pi/extensions/**` 与 `.pi/constraint-injection.json`。

| 扩展 | 挂点 | 触发 | 软硬 | fail 策略 | 事件库记账 |
| --- | --- | --- | --- | --- | --- |
| constraint-injection | `before_agent_start`（稳定层）+ `input`/`tool_execution_start`/`session_compact`（动态层） | 混合通道注入：稳定层 system prompt（档位生命周期内字节恒定）/ 动态层 steer 消息（指纹 diff，稳态零投递） | 软（不干预工具） | fail-open（注入失败不阻断） | constraint.inject / pin.* / mode.set（含 source） |
| quality-gate | `turn_end` | 执行链路平台判定（`cmd.exe` 可达性）→ windows 模式：interop 健康探测，vsock 故障整轮短路（harden-gate-interop-health）；native 模式：本机工具链直接执行 + 工具链可达性探测，缺失侧短路（harden-gate-native-toolchain）；另落 `edit.map` 归属地图（增量路径 × boundChange 聚合，coordinate-concurrent-changes）；失败报告按指纹去重（首现全块/持续单行/≥3 回合未修标记/转绿收尾）+ 并发外部归因（会话启动基线 ∪ 归属集合三向判定，attribute-concurrent-gate-noise） | 软 steer 催修（windows 链路失败标（wsl环境），native 标本机；环境故障不计粘性；[外部] 失败不催修不进粘性） | fail-open（门禁故障放行） | gate.check / edit.map / policy.decision(interop-down / toolchain-down / foreign-breakage) |
| quota-gate | Agent 派发前 | 子线程派发前查额度 | 硬 block（低额度阻断派发） | fail-open（查询失败放行） | policy.decision（quota-low/exhausted=block、quota-query-failed=fail-open、fuzzy-model-resolve=warn） |
| spec-gate | `tool_call` | bash 命中 `openspec archive` | 硬 block（归档门禁五检查：doc-impact/standards/尾三节/scenario-trace/UI 验收证据；另检查⑤'归档并发 warn 不 block：树上存在归属其他 active change 的未 commit 文件 → steer 提醒 + concurrent-dirty-tree 记账，冷启动零输出） | `--force` / `SPEC_GATE_BYPASS=1` 逃生口留痕 | policy.decision（archive-check-failed=block、explicit-bypass=bypass、acceptance-wording=warn、concurrent-dirty-tree=warn；UI 缺证据另记 ui-design-gate block ui-verification-missing） |
| ui-design-gate | `tool_call` | implementation 档绑定 syntopica-ui schema change：Agent 派发与 edit/write 项目代码，major 原型未批准（合同 block）时拦截；当前 change 的 ui-design.md/ui-prototype/** 修复不受限 | 硬 block（legacy schema 仅 front mutation 每会话/change warn 一次） | fail-open（检查异常放行+告警+记账）；`UI_DESIGN_GATE_BYPASS=1` 显式旁路留痕 | policy.decision（ui-impact-missing/ui-impact-mismatch/ui-design-missing/ui-prototype-missing/ui-approval-pending=block、explicit-bypass=bypass、ui-gate-check-failed=fail-open；健康放行零记录） |
| entry-gate | `turn_end` | 实现档切入后 complex 缺 test-cases 文档 | 软 steer 提醒 | fail-open | gate.check（cmd=entry-gate） |
| test-scope-guard | `tool_call` | bash + `ctx_execute`(仅 shell) + `ctx_batch_execute` 命中真·全量 `go test`（判定纯函数：裸 `./...` + 生效 cwd = 仓库根/backend-go；掩蔽引号/#注释/heredoc/`cat` 目标；影响包/子树/单包/写文档放行） | 软提醒（日常只跑影响包；文案列归档与依赖变更两条合法路径） | fail-open | policy.decision（full-go-test：soft=warn / hard=block；归档/依赖变更语境与 `# archive-gate`/`# allow-full-test` 逃生注释放行零记账） |
| tool-output-spill | `tool_result` | 工具输出 >32KB（`.pi/harness.json`） | 落盘替换+有界预览 | fail-open（spill 失败原样通过） | spill.write |
| dev-process-guard | `session_start`/`turn_end`/`session_shutdown` | 会话结束扫描仓库内泄漏类 dev 进程（go run/pnpm dev/nuxt dev 等且无终端、非 pidfile 白名单）：窗口内自动清，遗留软提醒 | 自动清（仅 session_shutdown，组杀 TERM→2s→KILL）+ 软 steer 提醒（每会话一次） | fail-open（扫描/杀/记账异常逐目标跳过）；非 Linux no-op | policy.decision（orphan-killed / orphan-warn；健康路径零记录） |
| harness-telemetry | `session_start`/`turn_end`/`tool_call`/`tool_result` | 通用事实采集（不干预）；turn_end 节流写 session.rollup 效能快照（轮次/token/成本/时长，每 5 turn 或 token 增量>20%；session_start 回填 prev 终值，同 session 取最新一条即终值） | — | fail-safe（断链不伪造，jsonl 缺失/坏行零写入） | session.start / session.rollup / subagent.* |

## 约束注入（constraint-injection，自动，管"知道"）

`.pi/extensions/constraint-injection.ts` 按**混合通道**注入约束上下文（harden-constraint-injection-channel）：

- **稳定层**（system prompt：索引 + mode-base + 声明域红线层；快照 key=mode|绑定 change，档位生命周期内**字节恒定** → system prompt 不变则其后 history 前缀缓存不失效）。
- **动态层**（追加消息：关键词命中全节 / JIT 命中全节 / change 级文件（explore-findings、词汇表）/ 稳定层差异；指纹 diff 驱动，稳态零投递；指纹按投递时的快照键记 regime（2026-09-17 fix-injection-transition-fingerprint-wipe：延迟快照重建仅在指纹 regime ≠ 新快照键时清空，mid-turn 切档后同 turn 已投递内容不重投）；turn 中途 JIT 命中经 `sendMessage(deliverAs:"steer", triggerTurn:false)` 即时送达；`session_compact` 后重发一次快照补偿压缩）。配置 `channel:"legacy"` 可回退旧的每 turn 全量进 system prompt。

**档位/绑定**：`input` 命令 / skill 路径 / 写 change 目录兜底均可激活；**绑定修正条件化**（read 永不抢绑、当前绑定健康时不抢绑、仅未绑定/绑定 change 消失时兜底）且**同 turn 锁定**；**所有绑定变化均记 `mode.set` 并带 `source`**（command/skill/edit-dir/recover/inherit/fallback），隐性绑定不存在——治多 change 并行注入污染（2026-09-16 事实库取证：隐性绑定 / 一毫秒 4 连绑）。

**注入源与命中规则**：flow「业务约束与不变量」节按 **proposal.md 业务域声明**（头部 `<!-- constraint-domains: 域, ... -->`，域名=flow 文档 basename 如 `daily-report`；纯工具链 change 可不写，widget 提示无域声明属预期；**声明域注入=红线层**——约束节内顶层列表项首个加粗红线句逐行 + 细节层取回指引，红线层提取 0 条或低于 512B 回退全节，格式见 `standard/shared/doc-authoring.md`「约束节红线句格式」）+ 对话输入关键词命中（**域限定**：仅声明域∪栈相关∪索引内的命中生效，跨域词不再误拉无关域全节）+ standard/flow 文档按头部 `doc-impact-applies` 标签对编辑路径 JIT 命中、`pin_finding` 工具持久化探索发现（档激活落 change 的 explore-findings.md，无档落 `docs/research/`）。配置 `.pi/constraint-injection.json`，常驻索引 `docs/reference/constraints-index.md`（旧 `doc-impact.sh context` 已退役）。

## 增量质量门禁（quality-gate，自动，管"做到"）

`.pi/extensions/quality-gate.ts` 已挂 `turn_end`，按**会话内增量路由**触发——会话开始时的 git 脏文件进基线不触发，仅本回合新增/变化路径命中后端（`backend-go/**.go`）才跑 `golangci-lint`+`go vet`+`go build`+影响包 `go test -short`（经 `scripts/harness/change-scope.sh` 判定，DB 集成测试 -short 下自动 skip），命中前端（`front/` 非 .md）才跑 eslint（带 `--cache` 增量；windows 模式经 cmd.exe 调 Windows 原生（实测 2~6s，WSL DrvFS 跑同命令 ~17 倍慢），native 模式本机直接跑；`front/package.json` 的 `pnpm lint` 仍全量供人工/归档用）；lint 先行作短路哨兵——lint 报编译失败（typechecking error）时 vet/build/test 必红同因，跳过执行不记账，无短路时 vet/build 并行。

**执行链路按宿主平台分流**（linux-native-dev-environment）：`cmd.exe` 不可达（Linux/macOS）走 native 模式（本机 PATH 的 go/golangci-lint/pnpm，工作目录经 `ExecOptions.cwd` 传入），可达则维持 windows 模式（cmd.exe interop，vsock 故障时整轮短路 + 记账）；平台身份会话内稳定。上回合失败未转绿的命令粘性重跑（催修，防"口头修复"漂移）。

**native 工具链探测短路**（harden-gate-native-toolchain，与 interop-down 同款套路）：native 模式在跑某侧门禁前探测该侧工具链在 PATH 可达（后端 go+golangci-lint 双检、前端 pnpm，`accessSync` 文件扫描非 exec，探测结果会话内缓存）；不可达即**整侧跳过**（fail-open，另一侧照常），零 gate.check 假失败。进入短路态首个回合记一条 `policy.decision(fail-open, toolchain-down, target=backend|frontend)` + 一条 steer（非代码问题，恢复方向指向 pi 启动环境 PATH，禁「重启 WSL」类跨系统建议），后续回合边沿触发不重复。兑底：命令输出含 `command not found` 特征的失败（探测后 PATH 内文件被删的窗口期）不进粘性、不按 [回归]/[中间态] 分级，归因环境。背景：2026-09-17 环境迁移当晨 pi 启动环境缺 go，两会话 945 条假失败（占 7 天窗口失败 56%）经粘性放大。windows 模式不参与（Windows 绝对路径执行，不经 bash PATH 查找）。

**steer 分级**：失败以 `steer` 消息分级喂回——**[回归]**（上回合尚绿，agent 必须修，不得忽略）与**[中间态]**（从未绿的新代码中间态，agent 若正在推进可继续、回合末复检）；归档前全绿硬要求不变（开发执行规范 §11）。

**记账口径**：成功事件采样记账（会话首条与转绿锚点必记，其后每 5 连续成功记 1 条），失败全量记账。**不跑**前端 typecheck/build 与完整集成测试（不带 -short 的 go test）——这是门禁分层设计（与平台无关），这些仍由 agent 手动跑 + §11 归档门禁兜底（分层全貌见开发执行规范 §4.1）。

**失败报告收敛与并发归因**（attribute-concurrent-gate-noise，spec `gate-failure-reporting`）：

- **指纹递变**：失败以 `(cmd, truncateDiagGate 特征行)` 为指纹去重——首现/指纹变化输出完整块（分级前缀 + tail 30），同指纹持续降为单行 `⟳ …第 N 回合未变化`，连续 ≥3 回合附加「未修」标记，转绿输出一行 `✓` 收尾并清条目；状态会话内内存维护（边界清零不跨会话），**粘性重跑语义不变**（门禁不沉默，只是不重复注入同样的字节）。背景：实测 91% 的失败行是同指纹重复注入，单事故 223 连击。
- **并发外部归因**：门禁命令全仓执行，并发共享工作树上别人的半成品会被算到本会话头上（实测 67% 的失败行在 ±30min 内另一 session 报同指纹）。判定只用可机证信号：`mine = 本会话累计触发集 ∪ 绑定 change 归属`，`foreign = 会话启动基线 ∪ 其他 change 归属`，失败输出提取路径/包锚点（`lib/failure-classify.ts` 双锚点白名单正则，`syntopica-backend` module 映射 `backend-go/`），`P≠∅ ∧ P∩mine=∅ ∧ P⊆foreign` 才降级 `[外部]`：不进粘性、不打 [回归]/[中间态]、一行提示（同指纹会话内至多一行，指纹变化重发），每命中回合记 `policy.decision(warn, foreign-breakage, target=<cmd>)`，gate.check 失败事实照记。任何混合/解析不出/信号缺失一律回现状（宁可多报不误判外部）。bash 编辑（`sed -i`/`gofmt -w`）不进 edit.map，mine 侧靠累计触发集兜底（非当回合 trigger）。
- **与归档门禁的分工**：本机制在 `turn_end` 实时降嗓；spec-gate 检查⑤'（`concurrent-dirty-tree` warn）在归档时点检查树上归属其他 active change 的未 commit 文件——一个管会话内报告噪声，一个管归档拆 commit 收口，互补不替代。
- suggest 侧配套（spec `doc-impact-gate`「suggest 预勾选输入口径」）：`doc-impact.sh suggest` 预勾选改归属优先（change 名三源解析：`--change` → `PI_SESSION_ID` 查最新 `mode.set.boundChange` → 空），脏文件三桶分列（本 change/其他 active change/无归属，各桶上限 20 行）不静默过滤，解析不到回退全树 diff 并显式标注，退出码恒 0。

## 子线程通道矩阵（harden-subagent-constraint-channel）

子线程（子代理）能否收到 harness 约束取决于派发通道与模式，实测矩阵（2026-09-18，根因证据链见 `docs/research/subagent-constraint-gap/explore-findings.md`）：

| 通道 × 模式 | 扩展加载 | 约束可达 | 门禁行为 |
| --- | --- | --- | --- |
| pi-web Agent × 内置档（general-purpose/explore/plan） | 否（profile 写死 `loadExtensions:false`） | 否（盲区，靠派发任务文本带红线兑底） | 无（扩展不在） |
| pi-web Agent × `implementer` 档（`.pi/agents/implementer.md`，`load_extensions: true`） | 是 | **是**（档位继承 `mode.set source=inherit` + 注入 + 事实库记账） | turn_end 降载：零重命令放行 + `policy.decision(bypass, child-session)` 记账 |
| pi-subagents × 前台 | 默认否（agent 定义 frontmatter `extensions:` 可显式挂载） | 挂载后可达 | 挂载后同降载语义 |
| pi-subagents × 后台（独立 runner） | 是（ambient 默认加载，`extensions: []` 可覆盖关） | 是 | 同降载语义 |

已知限制（设计裁决见 change design.md D6）：

1. **AGENTS.md 不自动进 pi-web 子线程**：`noContextFiles` 恒真（包硬编码）——约束注入补的是 harness 层，项目上下文仍靠派发任务文本（必读文件清单）。
2. **内置档与前台派发是盲区**：只读探索类任务用内置 explore/plan 保持轻量属预期；前台派发须任务文本带红线（编排纪律兑底）。
3. **pi-web 升级漂移依赖点**：profile 字段白名单（snake_case `load_extensions` 等）与扫描目录（`.pi/agents/`）；漂移表现为子线程退化无扩展（事件库三事件消失），可观测。

## 孤儿 dev 进程治理（dev-process-guard）

`.pi/extensions/dev-process-guard.ts` 治理「会话起 dev 服务不杀、进程孤儿化堆积」（2026-09-17 事故取证：`docs/research/orphan-dev-processes/explore-findings.md`）：

- **泄漏类判定（五条件合取）**：cmdline 命中特征（go run 后端 / go-build 编译产物 / pnpm dev / nuxt dev 系 / **agent-browser CLI 及其无头 chromium 的 user-data-dir 窄锚点**）∧ cwd 在仓库内 ∧ 无控制终端（tty_nr=0，豁免用户终端手起的服务）∧ PGID 不在 pidfile 白名单 ∧ 非自身进程组。
- **pidfile 白名单协议**：`scripts/dev/start-dev.sh` 起服务写 `.pi/run/{backend,front}.pgid`（setsid 会话首进程=PGID）；经脚本起的服务是合法长驻，guard 永不杀、stop 端口+pidfile 双路组清（覆盖僵尸栈：端口已释放但进程组存活）。**红线：dev 服务起停必须走 start-dev.sh，手提 nohup setsid 的会在会话结束时被自动清理**。
- **窗口归因**：session_start 记窗口起点（按 sessionId 隔离，子线程只扫自己的窗口）；session_shutdown 只自动清「窗口内」spawn；历史遗留孤儿（无法安全归因）由 turn_end 每会话一次的软提醒交人工处置，升级落地不误杀在用服务。
- **平台与 fail 策略**：仅 Linux /proc 实现，其他平台 no-op；扫描/组杀/记账任一异常逐目标跳过，不阻断会话关闭。

## 派发额度门禁（quota-gate）

`.pi/extensions/quota-gate.ts` 在每次 Agent 派发前自动查目标 provider 剩余额度：GLM/Kimi 查 5h/周窗口（GLM 老套餐仅 5h 窗口，MCP 的 TIME_LIMIT 不参与判定）；DeepSeek 查余额；opencode-go 无 API 直接放行。窗口剩余 <10% 或余额 <¥1 时派发被 **block**。阈值可用环境变量 `QUOTA_GATE_WINDOW_PCT` / `QUOTA_GATE_MIN_BALANCE` 调整；查询失败一律 fail-open 放行。收到 block 后的操作纪律（换 provider 全称重试）见开发执行规范 §0.6「子线程并行派发」。

## 测试欠账巡检（test-patrol.sh，脚本 + 事实库记账）

`bash scripts/harness/test-patrol.sh`（change: test-debt-patrol）把全量测试拆成静态分片滚动巡检，堵住「增量门禁只保局部绿、存量红无人记账」的缺口（不跑扩展，按纪律/归档流程调用）：

- **分片枚举**：12 片轮转（后端 6 片：`be-admin`/`be-dataenrichment`/`be-reader`/`be-tagmanagement`/`be-topicgraph`/`be-skeleton`；前端 6 片：`fe-tags`/`fe-discovery`/`fe-features`/`fe-core`/`fe-composables`/`fe-components`）+ `be-all`（整片，不入轮转，仅 `--shard` 显式指定）。默认跑「最久未巡优先」一片，`--shards <n>` 连跑 n 片。
- **资源红线**：前端分片固定 `--maxWorkers=2`（且 `pnpm test:unit` 的 filter **不带 `--`**——带 `--` 会吞掉 filter 静默跑全量，见 `standard/frontend/testing.md`）；跑前查 load average >4 或检出并发 build/浏览器自动化 → 只提醒不阻断。
- **`patrol.check` 事件**：每个分片执行完向 `.pi/harness/events.db` 的 `events` 表追加一条，payload 恰为 `{shard, ok, ms, fails[]}`，`change` 列为 NULL（巡检是仓库级活动，不属单个 change），**保留期 30 天**（与 `gate.check` 同级，登记于 `lib/harness-log.ts` 的 `RETENTION_DAYS`）。事件只记流水。
- **`test_debt` 台账（状态数据，不走 TTL）**：同一库内的独立表，登记「非本 change 改坏、不在本次修复范围内」的红测试——`test_id`（后端 `internal/<pkg>::<TestName>` / 前端 `front/<path>::<suite > case>`，整文件级用 `::<file-level>`）、`domain`（= 分片名）、`first_seen`/`last_seen`、`context`（巡检分片名 / 归档 change 名 / 存量摸底）、`status`（`open` → `fixed`｜`waived`，终态保留不物理删除）、`fixed_by`、`waived_reason`/`waived_at`、`note`。`patrol_shard` 表存分片进度（`last_run`/`last_ok`/`last_ms`/`last_fails`/`runs`/`total_fails`）。
- **双账不互写**：事件（TTL 30 天）与台账（持久）各司其职——事件被 TTL 清扫不影响台账记录与状态机。
- **消费**：`--report` 看四类聚合（status 计数 / domain 计数 / open 清单按 `first_seen` 升序 / `last_seen` 超 30 天 stale 提示）+ 分片进度表；`scripts/harness/harness-retro.sh` 报告显示窗口内巡检频次与欠账趋势。纪律（顺手跑一片、归档前看 report、域外红登记后放行）见 `AGENTS.md` 与《开发执行规范》§4.1/§11.4。

## 改进闭环（harness-retro）

`bash scripts/harness/harness-retro.sh`（只读消费事件账本，产出失败聚类六段报告——分母按采样口径还原、`fail-open` 单列为 harness 自身故障），配 `--save-baseline`/`--baseline` 把「改一条 harness 规则前后同类事件计数」变成可回检的准 A/B；读法与改进项判据（可回检指标 + 观察窗口 + 反 overfit）见 skill `harness-retro`，归档后可选回检流程见开发执行规范 §12.5。

## 定时任务脚本语义（schedule / workflowScript）

`.pi/subagents/schedules/` 的定时任务（`schedule.create`）与 `SubagentWorkflow` 工具**共用「workflow 脚本」这个词，但语义不同**——2026-09-18 凌晨两个 harness 定时任务触发即死（100~240ms 语法错误、零产出）正是踩了这个坑：

| 维度 | `SubagentWorkflow.script` | `subagent.workflowScript`（schedule 用这个） |
| --- | --- | --- |
| 脚本形态 | **模块**：必须 `export const meta = {...}` 开头 | **语句体**：被包进 async 函数执行，`export`/`import` 一律非法，顶层可用 `return` |
| 编排 API | `agent(prompt, opts)` / `phase()` / `parallel()` / `pipeline()` | `runs.run(key, {...})` / `runs.all([...])`；无 `agent()`、无 `phase()` |
| 指定模型 | `model: 'provider/id'` + `effort: 'max'` | `model: 'provider/id:max'`（thinking 走 **model 后缀**；`thinking` 参数仅 `watchdog.configure` 用） |
| 子线程类型 | `agentType: 'delegate'` | `agent: 'delegate'` |

**排错提示会误导**：上述冲突统一回一句 `'import' and 'export' may only appear at the top level`，外加「If task text contains Markdown fences or backticks, use an array joined with "\n"」——后半句是泛化文案，**实测只写 `export const meta` 就能复现同一报错串**。先查 export/import，再怀疑反引号模板字面量。（任务文本仍建议用 `JSON.stringify` 生成的普通字符串而非反引号模板字面量，少一个变量。）

**改脚本 = 删了重建**：schedule 动作只有 `create/list/show/history/pause/resume/run/run-due/delete`，**没有 update**（`pi-subagents/src/shared/types.ts` 的 `SUBAGENT_ACTIONS` 为准）；`show/history/pause/resume/run/delete` 的 `id` 参数要 **8 位 hex id，不是 name**（传 name 报 `Schedule '<name>' not found`）。重建会产生新 id，旧目录连同历史一起消失。

**纪律**：

1. `schedule.create` 前先跑 `{action:"validate", workflowScriptPath}` 过一遍脚本（离线、零成本、不派发）；
2. 定义/历史/事件存 `.pi/subagents/schedules/<id>/`（`schedule.json` / `history.json` / `events.jsonl`）——该目录是**机器本地运行时状态，已 gitignore**，加上 `schedule` 没有 update，脚本本体没有版本控制兜底：正文另存 `.pi/workflows/<name>.js`（命名 workflow 目录，**入库**）作权威副本，`schedule.create` 也可直接用 `workflowScriptPath` 引用它省掉内联。当前两个 harness 定时任务（`harness-retro-analysis` / `harness-lifecycle-doc`）是**内联 + `.pi/workflows/` 副本**形式，两边须同步（仅允许尾换行差异），别养出两份真相；
3. 一次性（`at`）任务跑完后 `nextRunAt` 归零（`manual_satisfied` 对非 interval 触发清空），要再跑得手动 `schedule.run` 或重建；确认已产出后应 `schedule.pause` 掉，防重复触发。

## 变更记录

| 日期 | 变更 | 摘要 | 归档位置 |
| 2026-09-18 | attribute-concurrent-gate-noise | quality-gate 失败报告按指纹去重（首现全块/持续单行/未修标记/转绿收尾）+ 并发外部归因（会话启动基线 ∪ 归属集合三向判定，[外部] 不催修不粘性，foreign-breakage 记账）；doc-impact suggest 改归属优先 + 三桶分列（change 名三源解析） | ../../../openspec/changes/attribute-concurrent-gate-noise |
| 2026-09-18 | harden-subagent-constraint-channel | 补「子线程通道矩阵」节：pi-web/pi-subagents × 前台/后台四象限（扩展加载/约束可达/门禁行为）+ 已知限制三条；implementer 档（load_extensions:true）上线，quality-gate 子线程 turn_end 降载（bypass + child-session 记账） | ../../../openspec/changes/archive/2026-09-18-harden-subagent-constraint-channel |
| 2026-09-18 | —（文档补记） | 补「定时任务脚本语义」节：schedule 的 `workflowScript` 是语句体语义（禁 export/import、用 `runs.run`、thinking 走 model 后缀），两套 script 语义对照 + validate 前置于 create + 改脚本需删重建 + 正文副本归 `.pi/workflows/` | —（纯文档，无 change） |
| 2026-09-17 | dev-process-guard | 新增孤儿 dev 进程治理扩展 dev-process-guard（session_shutdown 自动清窗口内泄漏进程组 / turn_end 软提醒 / agent-browser 残留八模式判定）+ start-dev.sh pidfile 契约（.pi/run/*.pgid 白名单 + 双路 stop + KILL 升级） | ../../../openspec/changes/archive/2026-09-17-dev-process-guard |
