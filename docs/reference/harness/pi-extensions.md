# pi harness 扩展机制（全景 + 注入 + 门禁）

<!--
doc-impact-applies: .pi/extensions, .pi/workflows, .pi/constraint-injection.json, .pi/harness.json, scripts/harness/harness-retro.sh, scripts/harness/constraint-digest.sh, scripts/harness/change-scope.sh, scripts/harness/change-abandon.sh
-->

> **权威源**：本文件是 pi harness 扩展**机制参考**的唯一权威——扩展全景表、约束注入/质量门禁的工作原理、事件账本入口。改 harness 相关代码或排查「为什么注入了/为什么被拦」先读这里。agent 日常行为红线（[回归]必修、测试范围、平台分流）在 AGENTS.md，流程编排在《开发执行规范》§0.6，两文互引不重复。事件考古查 skill `harness-facts`，改进复盘查 skill `harness-retro`。

## pi 扩展全景（`.pi/extensions/`）

**源码已入库**——`.gitignore` 的 `/.*` 排除后单独放行 `.pi/extensions/**` 与 `.pi/constraint-injection.json`。

| 扩展 | 挂点 | 触发 | 软硬 | fail 策略 | 事件库记账 |
| --- | --- | --- | --- | --- | --- |
| constraint-injection | `before_agent_start`（稳定层）+ `input`/`tool_execution_start`/`session_compact`（动态层） | 混合通道注入：稳定层 system prompt（档位生命周期内字节恒定）/ 动态层 steer 消息（指纹 diff，稳态零投递） | 软（不干预工具） | fail-open（注入失败不阻断） | constraint.inject / pin.* / mode.set（含 source） |
| quality-gate | `turn_end` | 执行链路平台判定（`cmd.exe` 可达性）→ windows 模式：interop 健康探测，vsock 故障整轮短路（harden-gate-interop-health）；native 模式：本机工具链直接执行 + 工具链可达性探测，缺失侧短路（harden-gate-native-toolchain）；另落 `edit.map` 归属地图（增量路径 × boundChange 聚合，coordinate-concurrent-changes）；失败报告边沿触发注入（首现/指纹变化全块、同指纹持续零注入、转绿收尾，aggregate-concurrent-gate-warns；指纹=归一化失败特征行（剥时间戳/端口/耗时/数字序列/仓库根前缀，add-foreign-gate-steer-attribution），展示与记账 diag 保留原文；包锚点目录∩已知归属展开 + 绝对路径剥根）+ 并发外部归因三态（[外部]/[并发]/本会话，attribute-concurrent-gate-noise + tune-quality-gate-concurrency；其他 change 的 edit.map 归属不依赖本会话绑定，add-foreign-gate-steer-attribution Fix C）；门禁互斥锁 + 限核（gate.lock 抢不到整轮跳过；GOMAXPROCS=2 / lint --concurrency=2 / vet-build 串行，tune-quality-gate-concurrency） | 软 steer 催修（windows 链路失败标（wsl环境），native 标本机；环境故障不计粘性；[外部] 失败不催修不进粘性；[并发] 混合归属进粘性不催修分级；同指纹持续零注入，红态可查 `scripts/harness/gate-status.sh`） | fail-open（门禁故障/锁被持放行） | gate.check（ok=false 附 ownership 四态）/ edit.map / policy.decision(interop-down / toolchain-down / foreign-breakage / gate-lock-held / concurrent-mixed / attribution-degraded) |
| quota-gate | Agent 派发前 | 子线程派发前查额度 | 硬 block（低额度阻断派发） | fail-open（查询失败放行） | policy.decision（quota-low/exhausted=block、quota-query-failed=fail-open、fuzzy-model-resolve=warn） |
| spec-gate | `tool_call` | bash 命中 `openspec archive` | 硬 block（归档门禁六检查：doc-impact/standards/尾三节 + complex 档 review 任务行 + major 档 UI 验收任务行（`check-tasks-tail.sh`，harden-archive-readiness 抽出为独立脚本，review-task-frontload 扩展 review 任务行判定：proposal 头部 complex 且未 waived 时 tasks 须含 review 任务行；add-ui-verification-trace-gate 扩展第三道防线：proposal 头部 major 且新 schema 时 tasks 须含指向 `ui-verification/report.md` 的任务行）/scenario-trace/review-trace 痕迹对账（add-review-trace-gate：review.md 建账或 proposal 头部 `<!-- review: waived -->` 豁免，皆无则 block）/UI 验收痕迹对账（add-ui-verification-trace-gate：spawn `scripts/harness/ui-verification-trace.sh` 取退出码，判定单一事实源在脚本——major report.md 建账+点名截图实物/minor 指名道姓映射/none N/A 一致/旧 schema fail-open；规划制品 approval+原型存在仍由 lib 校验；spawn 失败 warn 不阻断）；另检查⑤'归档并发 warn 不 block：树上存在归属其他 active change 的未 commit 文件 → steer 提醒 + concurrent-dirty-tree 记账，冷启动零输出；⑤/⑤' warn 边沿触发：同指纹会话内至多一条、转绿一行收尾、compact 重发，edge-trigger-archive-gate-warns） | `--force` / `SPEC_GATE_BYPASS=1` 逃生口留痕 | policy.decision（archive-check-failed=block、explicit-bypass=bypass、acceptance-wording=warn、concurrent-dirty-tree=warn；UI 缺证据另记 ui-design-gate block ui-verification-missing，此前有未解除同码 block 而本次证据齐备 → 归档放行时刻记 pass ui-design-satisfied，add-change-lifecycle-retro） |
| （非门禁）archive-readiness | 手动（agent 归档前） | `bash scripts/harness/archive-readiness.sh <change>` | 归档就绪聚合自查（harden-archive-readiness + add-review-trace-gate + add-ui-verification-trace-gate）：与 spec-gate 同口径六项脚本同一事实源（①doc-impact/②standards/③尾三节+review 任务行+UI 验收任务行/④scenario-trace/⑤review-trace/⑦ui-verification-trace UI 验收痕迹对账），逐项 ✓/✗，全绿再 archive；非门禁、不记账、不阻断 | 无（它本身就是入口不是拦截器） | 无 |
| ui-design-gate | `tool_call` | implementation 档绑定 syntopica-ui schema change：**具备写能力的** Agent 派发与 edit/write 项目代码，major 原型未批准（合同 block）时拦截；只读档（explore/plan/change-review）与原型编辑档（ui-design）派发放行并留痕（写侧边界不放宽——子线程经 `edit`/`write` 工具的每次写入仍逐个过门禁，shell 类通道不在本门禁覆盖范围——既有边界）；当前 change 的 ui-design.md/ui-prototype/** 修复不受限 | 硬 block（legacy schema 仅 front mutation 每会话/change warn 一次） | fail-open（检查异常放行+告警+记账）；`UI_DESIGN_GATE_BYPASS=1` 显式旁路留痕 | policy.decision（ui-impact-missing/ui-impact-mismatch/ui-design-missing/ui-prototype-missing/ui-approval-pending=block、explicit-bypass=bypass、ui-gate-check-failed=fail-open、**ui-dispatch-allowed=warn**（合同未就绪时放行只读档/原型编辑档派发，add-ui-prototype-editor-agent）；放行侧 pass 两码：ui-approval-approved（审批通过时刻）/ ui-design-satisfied（block 后补齐放行）——账本上有未解除 block 才记，健康放行仍零记录，add-change-lifecycle-retro；归档侧 ui-verification-missing 的解除由 spec-gate 记） |
| entry-gate | `turn_end` | 实现档切入后 complex 缺 test-cases 文档 | 软 steer 提醒 | fail-open | gate.check（cmd=entry-gate） |
| test-scope-guard | `tool_call` | bash + `ctx_execute`(仅 shell) + `ctx_batch_execute` 命中真·全量 `go test`（判定纯函数：裸 `./...` + 生效 cwd = 仓库根/backend-go；掩蔽引号/#注释/heredoc/`cat` 目标；影响包/子树/单包/写文档放行） | 软提醒（日常只跑影响包；文案列归档与依赖变更两条合法路径） | fail-open | policy.decision（full-go-test：soft=warn / hard=block；归档/依赖变更语境与 `# archive-gate`/`# allow-full-test` 逃生注释放行零记账） |
| sql-safety-guard | `tool_call` | bash + `ctx_execute`(仅 shell) + `ctx_batch_execute` 命中 psql 入口 ∧ TRUNCATE/DROP（判定基于命令原文、不掩蔽引号，`-f`/`--file` 形态读文件扫两词、不可读保守拦） | 硬 block（`SQL_SAFETY_GUARD` hard 默认 / soft 提醒 / off 关；逃生口：`# allow-truncate-drop` 注释、BEGIN+ROLLBACK 事务包裹、一次性容器放行；仅拦 TRUNCATE/DROP，DELETE/UPDATE/ALTER/INSERT 不拦） | fail-open（判定异常放行） | policy.decision（sql-safety：block/warn、逃生=bypass；off/未命中、事务与容器放行零记录） |
| tool-output-spill | `tool_result` | 工具输出 >32KB（`.pi/harness.json`） | 落盘替换+有界预览 | fail-open（spill 失败原样通过） | spill.write |
| dev-process-guard | `session_start`/`turn_end`/`session_shutdown` | 会话结束扫描仓库内泄漏类 dev 进程（go run/pnpm dev/nuxt dev 等且无终端、非 pidfile 白名单）：窗口内自动清，遗留软提醒 | 自动清（仅 session_shutdown，组杀 TERM→2s→KILL）+ 软 steer 提醒（每会话一次） | fail-open（扫描/杀/记账异常逐目标跳过）；非 Linux no-op | policy.decision（orphan-killed / orphan-warn；健康路径零记录） |
| harness-telemetry | `session_start`/`turn_end`/`tool_call`/`tool_result` | 通用事实采集（不干预）；turn_end 节流写 session.rollup 效能快照（轮次/token/成本/时长/**逐模型 models map**/**子会话 parentSessionId**，每 5 turn 或 token 增量>20%；同 session 取最新一条即终值）；session_start 回填 prev 终值（**自扫 sessions 目录**定位 prev——pi 事件从不携带 previousSessionFile，task-cost-metrics）；tool_result 配对 bash 归档命令成功 → `change.archive`（归档成功锚点，成功侧记账例外之一）；tool_result(Agent) → `subagent.dispatch` **携带 change 归属**（与 session.rollup 快照同源 detectActiveChange 判定，一次判定两用 agentChangeBindings + 事件 change 列，add-review-trace-gate；历史无归属行不回填——事件追加不可变约束） | — | fail-safe（断链不伪造，jsonl 缺失/坏行零写入；归档提取不到名零记录；change 解析不到记 null 不阻断派发） | session.start / session.rollup / subagent.* / change.archive |

## 派发期红线取数（constraint-digest.sh）

`scripts/harness/constraint-digest.sh` 是**派发期**取数入口——**它不是扩展**（不在上方扩展全景表内，是主线程派发前手工跑的脚本）：扩展管会话内注入，脚本管派发前取数。

- `--paths <路径>[,<路径>...]`：按文档头部 `doc-impact-applies` 标签反查受影响文档 → 导出域清单；反查语义与 constraint-injection 的 `scanAppliesTags` 一致（只扫文档头部 15 行、同一正则、纯子串匹配、单文档首个匹配行即止）。
- `--domains <域>[,<域>...]`：直接指定业务域（域名 MUST 取域唯一真相源里的名字，未知域名非零退出并列出可用域名）。
- 输出形态：「域标题 + 源文档相对路径 + 逐行红线句 + 细节层 read 指引」，单域红线正文 ≤8 行——供主线程**原样**粘进派发任务文本的 `## 约束（红线，源 docs/reference/flow/<域>.md）` 段（该段落格式是 telemetry 解析契约，取数三步与模板见《开发执行规范》§0.6 步骤 3）。
- **红线提取复用 constraint-injection 的同一实现**（`.pi/extensions/lib/constraint-redlines.ts` 的 `extractRedlines()`），MUST NOT 在脚本内重写第二份正则；**域清单唯一真相源 = `docs/reference/flow/*.md` basename**——`.pi/constraint-injection.json` 的 `keywordDocs` 是别名/子集、`docs/reference/constraints-index.md` 域表是面向模型的人读索引、`scripts/harness/doc-impact.sh menu` 的 8 项是**文档目录轴**（flow/api/database/…）不是业务域轴，三处禁止混用。

**可观测（append-only）**：`subagent.dispatch` 事件追加两个字段——`constraintDigest`（`present`/`absent`：任务文本有无 `## 约束` 段）与 `constraintDomains`（`string[]`，从该段 `源 docs/reference/flow/<域>.md` 收域；段存在但收不到域名记 `present` + `[]`，不猜域、不阻断派发）。改造前的历史记录无这两个字段 → `harness-retro` 计「未知」并从带域比例分母剔除（**不回溯**记 `absent`，否则把历史数据算成遗漏），报告显式标注未知条数。

## 约束注入（constraint-injection，自动，管"知道"）

`.pi/extensions/constraint-injection.ts` 按**混合通道**注入约束上下文（harden-constraint-injection-channel）：

- **稳定层**（system prompt：索引 + mode-base + 声明域红线层；快照 key=mode|绑定 change，档位生命周期内**字节恒定** → system prompt 不变则其后 history 前缀缓存不失效）。
- **动态层**（追加消息：关键词命中全节 / JIT 命中全节 / change 级文件（explore-findings、词汇表）/ 稳定层差异；指纹 diff 驱动，稳态零投递；指纹按投递时的快照键记 regime（2026-09-17 fix-injection-transition-fingerprint-wipe：延迟快照重建仅在指纹 regime ≠ 新快照键时清空，mid-turn 切档后同 turn 已投递内容不重投）；turn 中途 JIT 命中经 `sendMessage(deliverAs:"steer", triggerTurn:false)` 即时送达；`session_compact` 后重发一次快照补偿压缩）。配置 `channel:"legacy"` 可回退旧的每 turn 全量进 system prompt。

**档位/绑定**：`input` 命令 / skill 路径 / 写 change 目录兜底均可激活；**绑定修正条件化**（read 永不抢绑、当前绑定健康时不抢绑、仅未绑定/绑定 change 消失时兜底）且**同 turn 锁定**；**所有绑定变化均记 `mode.set` 并带 `source`**（command/skill/edit-dir/recover/inherit/fallback），隐性绑定不存在——治多 change 并行注入污染（2026-09-16 事实库取证：隐性绑定 / 一毫秒 4 连绑）。

**注入源与命中规则**：flow「业务约束与不变量」节按 **proposal.md 业务域声明**（头部 `<!-- constraint-domains: 域, ... -->`，域名=flow 文档 basename 如 `daily-report`；纯工具链 change 可不写，widget 提示无域声明属预期；**声明域注入=红线层**——约束节内顶层列表项首个加粗红线句逐行 + 细节层取回指引，红线层提取 0 条或低于 512B 回退全节，格式见 `standard/shared/doc-authoring.md`「约束节红线句格式」）+ 对话输入关键词命中（**域限定**：仅声明域∪栈相关∪索引内的命中生效，跨域词不再误拉无关域全节）+ standard/flow 文档按头部 `doc-impact-applies` 标签对编辑路径 JIT 命中、`pin_finding` 工具持久化探索发现（档激活落 change 的 explore-findings.md，无档落 `docs/research/`）。配置 `.pi/constraint-injection.json`，常驻索引 `docs/reference/constraints-index.md`（旧 `doc-impact.sh context` 已退役）。

**无档编辑路径 JIT（enable-unbound-jit-injection）**：**未激活档（`mode = null`）会话写/编辑代码命中 `doc-impact-applies` 标签路径时，同样把命中文档追加进动态层并经 steer 送达完整约束节**——稳定层仍只注索引（快照键 `none|none` 不变、字节恒定），JIT 只走动态层。理由：路径标签是**文件系统事实**（零歧义），而关键词匹配是对话文本的推测；只开路径通道不开关键词通道，误命中面为零。关键词命中（`matchText`）、关键词域限定（`allowedKeywordDocs`）、change 绑定（`boundChange` / turn 绑锁 / `mode.set`）**仍以档位为开关**——未激活档 MUST NOT 因编辑路径命中而绑定 change。standard 文档与 flow 文档同走此通道（不限 flow）。落地盘点：`bash scripts/harness/constraint-coverage.sh`（业务路径 → 命中域矩阵 + 覆盖率）。

- **生命周期边界**：主会话 `session_compact` → 下一注入时机重发索引 + 当前 JIT 集合（`pendingSnapshot` 机制）；**无 `before_agent_start` 的子线程 compact 不重发**（该事件在子线程 SDK 不存在，不伪造通道），同文档重复编辑也不重投（去重语义），丢失内容靠显式 read 或父会话 brief；同进程 fork / 子线程继承时 `jitDocHits` 与指纹随 channel 拷贝，父已送达的文档不重投；`channel:"legacy"` 下无档 JIT **不即时 steer**，下一 turn `before_agent_start` 把含 jit-path 的完整计划放回 system prompt；reload / resume 会话状态重建 → `jitDocHits` 清零（后续编辑重新命中）。
- **记账口径**：`constraint.inject` payload 追加 **append-only** 字段 `tier`（`none` / `requirements` / `implementation`，由注入器自报当前会话档位）；既有 `mode` 字段语义不变（**文档形态** `section` / `digest` / `full`，MUST NOT 复用为档位）。历史记录无 `tier` → 统计计「未知」并**剔除分母**（同 `subagent.dispatch` 的 `constraintDigest` 三态口径，不回溯伪造）。观测入口：`bash scripts/harness/harness-retro.sh` 的 **⑦H 无档 JIT** 段（注入条数 / 会话数 / 平均域数 / **送达字节** / 降级占比；compact 重发计入条数不重计会话；域数只算 `path` 以 `docs/reference/flow/` 开头的条目，同会话同 path 只计一次）。
- **回滚**：把 `.pi/extensions/constraint-injection.ts` 的两处 gate 恢复 `state.mode &&`（JIT **写入** gate 与 JIT **投递** gate 各一处，一行级）即回到「未激活=仅索引」旧行为；无需新增配置项或环境变量，已注入的历史消息不受影响，重载会话后按旧行为走。

## 增量质量门禁（quality-gate，自动，管"做到"）

`.pi/extensions/quality-gate.ts` 已挂 `turn_end`，按**会话内增量路由**触发——会话开始时的 git 脏文件进基线不触发，仅本回合新增/变化路径命中后端（`backend-go/**.go`）才跑 `golangci-lint`+`go vet`+`go build`+影响包 `go test -short`（经 `scripts/harness/change-scope.sh` 判定，DB 集成测试 -short 下自动 skip），命中前端（`front/` 非 .md）才跑 eslint（带 `--cache` 增量；windows 模式经 cmd.exe 调 Windows 原生（实测 2~6s，WSL DrvFS 跑同命令 ~17 倍慢），native 模式本机直接跑；`front/package.json` 的 `pnpm lint` 仍全量供人工/归档用）；lint 先行作短路哨兵——lint 报编译失败（typechecking error）时 vet/build/test 必红同因，跳过执行不记账，无短路时 vet/build 并行。

**执行链路按宿主平台分流**（linux-native-dev-environment）：`cmd.exe` 不可达（Linux/macOS）走 native 模式（本机 PATH 的 go/golangci-lint/pnpm，工作目录经 `ExecOptions.cwd` 传入），可达则维持 windows 模式（cmd.exe interop，vsock 故障时整轮短路 + 记账）；平台身份会话内稳定。上回合失败未转绿的命令粘性重跑（催修，防"口头修复"漂移）。

**native 工具链探测短路**（harden-gate-native-toolchain，与 interop-down 同款套路）：native 模式在跑某侧门禁前探测该侧工具链在 PATH 可达（后端 go+golangci-lint 双检、前端 pnpm，`accessSync` 文件扫描非 exec，探测结果会话内缓存）；不可达即**整侧跳过**（fail-open，另一侧照常），零 gate.check 假失败。进入短路态首个回合记一条 `policy.decision(fail-open, toolchain-down, target=backend|frontend)` + 一条 steer（非代码问题，恢复方向指向 pi 启动环境 PATH，禁「重启 WSL」类跨系统建议），后续回合边沿触发不重复。兑底：命令输出含 `command not found` 特征的失败（探测后 PATH 内文件被删的窗口期）不进粘性、不按 [回归]/[中间态] 分级，归因环境。背景：2026-09-17 环境迁移当晨 pi 启动环境缺 go，两会话 945 条假失败（占 7 天窗口失败 56%）经粘性放大。windows 模式不参与（Windows 绝对路径执行，不经 bash PATH 查找）。

**steer 分级**：失败以 `steer` 消息分级喂回——**[回归]**（上回合尚绿，agent 必须修，不得忽略）与**[中间态]**（从未绿的新代码中间态，agent 若正在推进可继续、回合末复检）；归档前全绿硬要求不变（开发执行规范 §11）。

**记账口径**：成功事件采样记账（会话首条与转绿锚点必记，其后每 5 连续成功记 1 条），失败全量记账。**不跑**前端 typecheck/build 与完整集成测试（不带 -short 的 go test）——这是门禁分层设计（与平台无关），这些仍由 agent 手动跑 + §11 归档门禁兜底（分层全貌见开发执行规范 §4.1）。

**失败报告收敛与并发归因**（attribute-concurrent-gate-noise，spec `gate-failure-reporting`）：

- **指纹递变（边沿触发注入）**：失败以 `(cmd, 归一化失败特征行)` 为指纹去重——**状态没变就闭嘴，状态一变就说话**：首现/指纹变化输出完整块（分级前缀 + tail 30），同指纹持续**零注入**（原单行 `⟳` 摘要与「≥3 回合未修」标记已退役——harness-retro ④段把逐回合提醒判为软提醒失效，41 次/7天并发 warn 重复触发），转绿输出一行 `✓` 收尾并清条目（每失败段至多一次）；状态会话内内存维护（边界清零不跨会话），**粘性重跑语义不变**（门禁不沉默、重跑次数不减，只是不重复注入同样的字节）。红态知情三路兑底（aggregate-concurrent-gate-warns D5）：会话内首次全文 → `bash scripts/harness/gate-status.sh` 随时主动查（只读，见下）→ spec-gate 归档前全绿硬门禁。背景：实测 91% 的失败行是同指纹重复注入，单事故 223 连击。**指纹归一化**（add-foreign-gate-steer-attribution，2026-09-28 实测 23 轮逐字重复轰炸的击穿点修复）：同指纹比较用 `normalizeDiagFingerprint`（剥日志时间戳/IP:port/耗时毫秒/裸耗时/独立数字序列/仓库根前缀；标识符邻接数字如 `e1`/`models2`/`go1.27` 保留——错误文本主体不折叠，宁可多报）；完整块展示与 `gate.check` 记账 diag 保留原文，仅比较路径归一化；归一化确定性（相同输出字节 → 相同指纹）。
- **状态查询入口（spec `gate-status-query`）**：`bash scripts/harness/gate-status.sh` 只读打开 facts 库（`file:…?mode=ro`），窗口函数取每 cmd 最新 `gate.check`，展示红/绿态 + 失败特征摘要（diag 首行截断）+ 时间 + 会话；退出码 0=查询成功（红态也算）、1=库缺失/损坏（stderr 说明）。「最新条目=当前态」的正确性依据是门禁记账协议（转红全量、转绿必记翻转锚点，skill harness-facts）。配套 smoke：`bash scripts/harness/gate-status.smoke.sh`。
- **并发外部归因（三态）**：门禁命令全仓执行，并发共享工作树上别人的半成品会被算到本会话头上（实测 67% 的失败行在 ±30min 内另一 session 报同指纹）。判定只用可机证信号：`mine = 本会话累计触发集 ∪ 绑定 change 归属`，`foreign = 会话启动基线 ∪ 其他 change 归属`（**不依赖本会话是否绑定**——无绑定探索会话同样吸入其他 change 归属，Fix C；累计触发集排除已归属其他 change 的路径——跨会话 diff 噪声不误进 mine，Fix A），失败输出提取路径/包锚点（`lib/failure-classify.ts` 双锚点白名单正则，`syntopica-backend` module 映射 `backend-go/`；**仓库根绝对路径前缀先剥除**——测试日志行的绝对路径与 repo 相对判定集永不命中，Fix B；包锚点 `# <pkg>` / `FAIL <pkg> [` / `vet: file:line` 三形态先映射 `backend-go/<pkg>/` 目录前缀，再与已知归属集（各回合 git 脏文件并集 ∪ 双侧 edit.map 归属）求交展开为**实际文件路径**并入 P——目录前缀精确匹配不上溯、空交不虚构路径保守回退，aggregate-concurrent-gate-warns D3），三态分流（`classifyFailureOwnership`）：`P≠∅ ∧ P∩mine=∅ ∧ P⊆foreign` 降级 `[外部]`——不进粘性、不打 [回归]/[中间态]、一行提示（同指纹会话内至多一行，指纹变化重发），每命中回合记 `policy.decision(warn, foreign-breakage, target=<cmd>)`，gate.check 失败事实照记；`P∩mine≠∅ ∧ P∩foreign≠∅` 降级 `[并发]`（见「混合归属降级」节）；其余（含解析不出/信号缺失）一律回现状按本会话失败（宁可多报不误判外部）。bash 编辑（`sed -i`/`gofmt -w`）不进 edit.map，mine 侧靠累计触发集兜底（非当回合 trigger）。（Fix A/B/C 均为 add-foreign-gate-steer-attribution 2026-09-28 复盘落地，断点定位与实账证据见 `docs/research/harness-gate-steer-attribution/explore-findings.md`）
- **归属可观测与归因降级记账**（add-foreign-gate-steer-attribution）：`gate.check` ok=false 附 `ownership` 四态（`mine`/`foreign`/`mixed` 透传判定结果，`attribution-degraded` = 归因信号缺席/判定异常的保守回退标记；ok=true 与环境故障失败无该键，存量无键事件消费侧判空回退）。归因保守回退**不静默**：判定过程异常 / 失败输出解析不出路径（P=∅）→ 记 `policy.decision(warn, attribution-degraded, target=cmd)`（同 (cmd, 归一化指纹) 会话内至多一条）；基线信号缺席（session_start git 快照失败）且失败无 mine 命中（本可判外部）→ 会话级单发一条；与 gate.check 失败事实不双写（gate.check 记失败本身与 ownership，policy.decision 只记降级原因）。已知残余（research 留痕）：他会话 bash 编辑（不进 edit.map）仍会污染 mine；错误浮出面失败（如 DB migration 破表后测试在无人编辑过的调用点报错——2026-09-28 实账 store_vendor.go 形态）路径归属模型无信号，保守 mine，伤害由指纹归一化抑制兜底（同指纹仅首现一次完整块）。
- **混合归属记账边沿化**：`policy.decision(warn, concurrent-mixed, target=<cmd>)` 与 [并发] 完整块**同一边沿**触发——新指纹（首现/指纹变化/失败段重建）各记一条，同指纹持续回合零注入零记账（每指纹会话内至多一条；指纹变化或转绿后再现作为新失败段可再记，aggregate-concurrent-gate-warns D2；回检指标：7 天计数 41 → ≤10）。`foreign-breakage` 维持每命中回合一条不变。
- **与归档门禁的分工**：本机制在 `turn_end` 实时降嗓；spec-gate 检查⑤'（`concurrent-dirty-tree` warn）在归档时点检查树上归属其他 active change 的未 commit 文件——一个管会话内报告噪声，一个管归档拆 commit 收口，互补不替代。
- suggest 侧配套（spec `doc-impact-gate`「suggest 预勾选输入口径」）：`doc-impact.sh suggest` 预勾选改归属优先（change 名三源解析：`--change` → `PI_SESSION_ID` 查最新 `mode.set.boundChange` → 空），脏文件三桶分列（本 change/其他 active change/无归属，各桶上限 20 行）不静默过滤，解析不到回退全树 diff 并显式标注，退出码恒 0。

## 归档门禁 warn 边沿触发（spec-gate 检查⑤/⑤'，edge-trigger-archive-gate-warns）

归档是「block → 修一项 → 重试」循环，warn 内容在循环内通常不变；spec-gate 检查⑤（验收措辞）与⑤'（并发脏树）的 warn 投递改 per-sessionId 指纹态驱动（与 quality-gate 失败指纹状态机、constraint-injection 指纹 diff 同家族语义）：

- **状态模型**：`warnStates: Map<sessionId, Map<"<change>:<kind>", fp>>`（kind ∈ wording/concurrent），会话内存态、LRU 有界（上限 32，超限淘汰最久未写会话）、无 sessionId 落 `__no-session__` 兜底槽；不跨会话复用、不落盘（新会话首见警告必要）。
- **投递边沿**：首见/指纹变化 → 全量 warn + 记账（同边沿，同指纹会话内至多一条）；同指纹重试 → 零 sendMessage 零记账（console 一行留痕）；上次 warn 过本次转净（⑤零违例 / ⑤' exit 0）→ 一行「已转净/已清零」收尾并删条目；⑤' exit 3 冷启动维持零输出不触碰状态。指纹：⑤' 清单行排序去重后 sha256 前 16 字节（消顺序抖动假 diff）；⑤ 违例文案 join（保留新增关键词排前语义）。
- **重发通道**：`session_compact` 清该会话条目（早先 warning 被摘要掉，重发是正确性要求）；会话边界由键隔离天然清零。内容丢失兜底：`concurrency-status.sh <change>` / `gate-status.sh` 拉模式查询。
- **审计边界（D5）**：`--force` / `SPEC_GATE_BYPASS=1` 豁免、门禁自身异常 fail-open、提取不到 change 名的 warning 不进指纹表，保持每尝试一条（独立动作留痕）；检查①-⑥' block 判定与 `policy.decision(action=block)` 记账零改动。指纹态异常 fail-open：按无状态全量投递 + console.warn，绝不阻断归档。
- **回检指标（归档后 7 天窗口，harness-retro）**：同 `session_id+change` 的 `concurrent-dirty-tree` warn 多条对数 5 → 0；spec-gate warn 计数/7 天 31 → ≤15。

## 子线程通道矩阵（harden-subagent-constraint-channel）

子线程（子代理）能否收到 harness 约束取决于派发通道与模式，实测矩阵（2026-09-18，根因证据链见 `docs/research/subagent-constraint-gap/explore-findings.md`）：

| 通道 × 模式 | 扩展加载 | 约束可达 | 门禁行为 |
| --- | --- | --- | --- |
| pi-web Agent × 项目覆盖三档（`.pi/agents/{general-purpose,explore,plan}.md` 同名覆盖内置档，2026-10-13 上线；general-purpose 已兼实现档职能——implementer 档 2026-11-03 并入退役，见下） | 是（`load_extensions: true`；general-purpose 另开 `codemode: on`） | **是**（constraint-injection 注入 + 档位继承 + 事实库记账，探针实测 2026-10-13：codemode/ctx 工具/`<skills>` 节/约束注入块全在；档位继承 `mode.set source=inherit`） | turn_end 降载：零重命令放行 + `policy.decision(bypass, child-session)` 记账 |
| pi-web Agent × 原型编辑档（`.pi/agents/ui-design.md`，项目自定义档，2026-11-03 新增；只改当前 change 的 `ui-prototype/**` 与 `ui-design.md` 的 Prototype 节） | 是（`load_extensions: true`；codemode + mcp 开） | **是**（constraint-injection 注入 + 档位继承 + 事实库记账） | 写侧逐个过 ui-design-gate（经 `edit`/`write` 工具）：当前 change 目录内放行、越界（`front/` 等）block（shell 类通道不在本门禁覆盖范围——既有边界）；合同未就绪时的派发放行并留痕 `ui-dispatch-allowed`（只读档 + 原型档白名单，`lib/ui-design-gate.ts` 的 `UI_DISPATCH_ALLOWED_AGENTS`）；turn_end 降载：同项目覆盖档（child-session bypass） |
| pi-web Agent × 内置档默认值（无项目覆盖时的 pi-web 内置 general-purpose/explore/plan，留作漂移诊断基准） | 否（profile 写死 `codemode:"off"` / `loadSkills:false` / `loadExtensions:false`） | 否（盲区） | 无（扩展不在） |
| pi-subagents × 前台 | 默认否（agent 定义 frontmatter `extensions:` 可显式挂载） | 挂载后可达 | 挂载后同降载语义 |
| pi-subagents × 后台（独立 runner） | 是（ambient 默认加载，`extensions: []` 可覆盖关） | 是 | 同降载语义 |

已知限制（设计裁决见 change design.md D6）：

> **原型编辑档的模型选择**：`ui-design` 档**不绑模型**（原型审美质量对模型敏感，固定默认会锁死）——模型由派发时按 AGENTS.md「子线程模型偏好」指定，**原型审美任务建议用强模型**；档位定义不代替用户拍板模型。

1. **AGENTS.md 不自动进 pi-web 子线程**：`noContextFiles` 恒真（包硬编码）——约束注入补的是 harness 层，项目上下文仍靠派发任务文本（必读文件清单）。
2. **内置档默认是盲区，已由项目覆盖档修复（覆盖可被 pi-web 升级冲掉）**：pi-web 内置三档写死 codemode off / loadSkills false / loadExtensions false；项目以 `.pi/agents/` 同名覆盖档（2026-10-13）开启 codemode/skills/插件，AGENTS.md 仍不注入（限制 1 不可改），覆盖档 addendum 写「开工先自读 AGENTS.md」兑底。pi-web 升级若改 profile 字段名或覆盖机制，子线程会静默退回内置盲区（可观测：事件库三事件消失、探针报告无 codemode/插件）；前台派发须任务文本带红线（编排纪律兑底——取数三步与 brief 模板见《开发执行规范》§0.6 步骤 3；带没带可观测：`subagent.dispatch` 的 `constraintDigest`/`constraintDomains` 字段，见上「派发期红线取数」）。
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

- **分片枚举**：14 片轮转（后端 8 片：`be-admin`/`be-dataenrichment`/`be-datasources`/`be-discovery`/`be-reader`/`be-tagmanagement`/`be-topicgraph`/`be-skeleton`；前端 6 片：`fe-tags`/`fe-discovery`/`fe-features`（含 feeds）/`fe-core`/`fe-composables`/`fe-components`）+ `be-all`（整片，不入轮转，仅 `--shard` 显式指定）。默认跑「最久未巡优先」一片，`--shards <n>` 连跑 n 片。
- **资源红线**：前端分片固定 `--maxWorkers=2`（且 `pnpm test:unit` 的 filter **不带 `--`**——带 `--` 会吞掉 filter 静默跑全量，见 `standard/frontend/testing.md`）；跑前查 load average >4 或检出并发 build/浏览器自动化 → 只提醒不阻断。
- **`patrol.check` 事件**：每个分片执行完向 `.pi/harness/events.db` 的 `events` 表追加一条，payload 恰为 `{shard, ok, ms, fails[]}`，`change` 列为 NULL（巡检是仓库级活动，不属单个 change），**保留期 30 天**（与 `gate.check` 同级，登记于 `lib/harness-log.ts` 的 `RETENTION_DAYS`）。事件只记流水。
- **`test_debt` 台账（状态数据，不走 TTL）**：同一库内的独立表，登记「非本 change 改坏、不在本次修复范围内」的红测试——`test_id`（后端 `internal/<pkg>::<TestName>` / 前端 `front/<path>::<suite > case>`，整文件级用 `::<file-level>`）、`domain`（= 分片名）、`first_seen`/`last_seen`、`context`（巡检分片名 / 归档 change 名 / 存量摸底）、`status`（`open` → `fixed`｜`waived`，终态保留不物理删除）、`fixed_by`、`waived_reason`/`waived_at`、`note`。`patrol_shard` 表存分片进度（`last_run`/`last_ok`/`last_ms`/`last_fails`/`runs`/`total_fails`）。
- **双账不互写**：事件（TTL 30 天）与台账（持久）各司其职——事件被 TTL 清扫不影响台账记录与状态机。
- **消费**：`--report` 看四类聚合（status 计数 / domain 计数 / open 清单按 `first_seen` 升序 / `last_seen` 超 30 天 stale 提示）+ 分片进度表；`scripts/harness/harness-retro.sh` 报告显示窗口内巡检频次与欠账趋势。纪律（顺手跑一片、归档前看 report、域外红登记后放行）见 `AGENTS.md` 与《开发执行规范》§4.1/§11.4。

## 改进闭环（harness-retro）

`bash scripts/harness/harness-retro.sh`（只读消费事件账本，产出失败聚类六段报告——分母按采样口径还原、`fail-open` 单列为 harness 自身故障；⑦效能看板含 B' 每任务成本子组：窗口内已归档 change × 归因会话终值 Σcost 的分布与模型/complexity 分桶，双锚点=change.archive 事件 ∪ archive 目录日期前缀，指标键 `m7.task_cost_*`，task-cost-metrics；⑦段另含 E 生命周期面子组（add-retro-lifecycle-rollup）：human-feedback 覆盖（有账/无账/未处置）+ 归因五类跨 change 聚合 + 漏采信号（波次 ≥3 session 且零反馈的 change 名单，清单=活跃+窗口内归档），指标键 `m7.lifecycle_*`——把单 change 体检卡第五段的文件面数据拉进全局定期体检；⑦段 A 组门禁催修时距按 policy 分派解除信号：quality-gate=gate.check 转绿 / ui-design-gate=pass 放行事件 / quota-gate=同 session 后续成功派发 / spec-gate=change.archive，信号缺席样本剔除并披露剔除数——统一用转绿判解除对非 gate 裁决是错配（实测 89 分钟假慢），add-change-lifecycle-retro），配 `--save-baseline`/`--baseline` 把「改一条 harness 规则前后同类事件计数」变成可回检的准 A/B；另提供 `--lifecycle <change>` 单 change 生命周期体检卡（流程规范/UI 工作流/成本卡/健康度四段 + proposal 头注释声明元数据联合，不受 --days 窗口限制；成本卡终值按 (session_id, change) 探索配方归因，与 B' 按会话终归口径并存；UI 工作流放行态双口径：pass 事件直读 / 引入前历史按末 block 推断并标注）；读法与改进项判据（可回检指标 + 观察窗口 + 反 overfit）见 skill `harness-retro`，归档后可选回检流程见开发执行规范 §12.5。

## change 终态登记（change-abandon.sh，add-change-lifecycle-retro）

`bash scripts/harness/change-abandon.sh <change-name>` 对「不再推进且不会归档」的 change 直写一条 `change.abandon` 事件（与 `change.archive` 家族对称：payload `{name}`、change 列同名、append-only 不去重、保留期 30 天）：校验 change 存在于 `openspec/changes/` 且未归档（archive 目录存在/误传归档目录名 → 拒绝，终态以归档事实为准），参数非法非零退出零记录（fail-open 不伪造终态）；开库安全对齐 harness-log（application_id 魔数校验，他人库拒绝）。消费侧：`harness-retro --lifecycle` 的终态判定与 ⑦段 B' 幸存者偏差统计把 abandon change 区分为「已放弃」（不占活跃口径，成本单列披露）；同名 archive 与 abandon 并存时以 archive 为准并输出冲突提示。解决「40+ 活跃 change 仅 25 个有下文」的僵尸 change 盲区——放弃也是终态，显式登记后 retro 能算完整分母。

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
| 2026-11-04 | add-dispatch-constraint-digest | 补「派发期红线取数（constraint-digest.sh）」节：派发期红线取数脚本定位（红线提取复用 `.pi/extensions/lib/constraint-redlines.ts` 的 `extractRedlines()`，与 constraint-injection 注入同一实现；`--paths` 反查语义与 `scanAppliesTags` 一致）+ 域清单唯一真相源=flow basename（`keywordDocs`/constraints-index 域表/`doc-impact.sh menu` 是别名或不同轴，禁止混用）+ `subagent.dispatch` 追加 `constraintDigest`/`constraintDomains` 两字段（append-only，历史计「未知」并剔除分母）；配套《开发执行规范》§0.6 步骤 3 三步取数 + brief 模板、AGENTS.md 取数指针 | ../../../openspec/changes/add-dispatch-constraint-digest（待归档） |
| 2026-10-12 | review-task-frontload | review 前移三层：check-tasks-tail.sh 扩展 complex 档 review 任务行判定（proposal 头部 marker 分档 + waived 豁免同检查⑤词表，单一事实源）；openspec-propose/new-change skill 注入 Review 节建档约定；openspec-apply-change skill 入口先跑 check-tasks-tail（缺行实现前暴露）；存量 in-flight complex change 不豁免 | ../../../openspec/changes/review-task-frontload（待归档） |
| 2026-09-28 | add-change-lifecycle-retro | harness-retro 新增 `--lifecycle <change>` 单 change 体检卡（四段 + proposal 声明元数据）；⑦段 A 组催修时距按 policy 分派解除信号（quota 89 分钟假慢修正）；ui-design-gate 放行侧 action=pass 两码（ui-approval-approved / ui-design-satisfied）显式记账；新增 change.abandon 终态事件与 `scripts/harness/change-abandon.sh` 登记脚本（B' 幸存者偏差区分已放弃） | ../../../openspec/changes/add-change-lifecycle-retro（待归档） |
| 2026-09-28 | split-retro-warn-classes | harness-retro ④ 段按情报型白名单（concurrent-mixed / concurrent-dirty-tree / foreign-breakage）拆分纠正型/情报型：失效名单只含纠正型、情报型单列计数不进失效判定；metrics 新增 soft.warn_total_corrective / soft.warn_total_informative / soft.informative_groups（stale_groups 键名不变、语义收窄；本周失效名单 3 组→0 全为口径修正） | ../../../openspec/changes/archive/2026-09-28-split-retro-warn-classes（已归档） |
| 2026-09-19 | tune-quality-gate-concurrency | quality-gate 并发互斥锁（gate.lock O_EXCL+TTL 180s，held 整轮跳过 gate-lock-held 记账）+ 限核（GOMAXPROCS=2 / lint --concurrency=2 / vet-build 串行）+ 失败归属三态化（混合归属 [并发] 降级进粘性不催修，concurrent-mixed 记账） | ../../../openspec/changes/archive/2026-09-19-tune-quality-gate-concurrency |
| 2026-09-19 | harden-archive-readiness | 归档就绪自查聚合入口：check-tasks-tail.sh 从 spec-gate 抽出（尾三节检查单一事实源）+ archive-readiness.sh 四项同口径只读自查 + spec-gate block 文案补自查指引（消除试探式归档重试）；效果回检挂账（7 天后 harness-retro 复测 m7.block_recur_groups ≤2） | ../../../openspec/changes/archive/2026-09-19-harden-archive-readiness |
| 2026-09-18 | attribute-concurrent-gate-noise | quality-gate 失败报告按指纹去重（首现全块/持续单行/未修标记/转绿收尾）+ 并发外部归因（会话启动基线 ∪ 归属集合三向判定，[外部] 不催修不粘性，foreign-breakage 记账）；doc-impact suggest 改归属优先 + 三桶分列（change 名三源解析） | ../../../openspec/changes/archive/2026-09-19-attribute-concurrent-gate-noise |
| 2026-09-18 | harden-subagent-constraint-channel | 补「子线程通道矩阵」节：pi-web/pi-subagents × 前台/后台四象限（扩展加载/约束可达/门禁行为）+ 已知限制三条；implementer 档（load_extensions:true）上线，quality-gate 子线程 turn_end 降载（bypass + child-session 记账） | ../../../openspec/changes/archive/2026-09-18-harden-subagent-constraint-channel |
| 2026-09-18 | —（文档补记） | 补「定时任务脚本语义」节：schedule 的 `workflowScript` 是语句体语义（禁 export/import、用 `runs.run`、thinking 走 model 后缀），两套 script 语义对照 + validate 前置于 create + 改脚本需删重建 + 正文副本归 `.pi/workflows/` | —（纯文档，无 change） |
| 2026-09-17 | dev-process-guard | 新增孤儿 dev 进程治理扩展 dev-process-guard（session_shutdown 自动清窗口内泄漏进程组 / turn_end 软提醒 / agent-browser 残留八模式判定）+ start-dev.sh pidfile 契约（.pi/run/*.pgid 白名单 + 双路 stop + KILL 升级） | ../../../openspec/changes/archive/2026-09-17-dev-process-guard |
