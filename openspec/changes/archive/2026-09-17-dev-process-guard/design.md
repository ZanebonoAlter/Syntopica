# Design — dev-process-guard

> 事实依据：`docs/research/orphan-dev-processes/explore-findings.md`（2026-09-17T22:05 取证快照：两个孤儿 Nuxt dev 栈 + 手搓后端 + `nohup setsid` 手搓命令全文）。

## D1 挂点：session_shutdown（自动清）+ turn_end（软提醒）+ session_start（窗口锚点）

- 清理动作只能挂 `session_shutdown`（pi 生命周期契约：会话切换/退出必发）：会话都关了，steer 没人读，只有自动清有意义。reason 不区分（quit/new/resume/fork 都清窗口内 spawn）——「换会话」不该继承无主进程。
- 历史遗留孤儿（不属于任何活会话窗口）无人能安全归因 → 不自动杀，由 `turn_end` 每会话一次的软提醒交人工处置。理由：升级落地瞬间就自动杀「正在被别的活会话使用」的服务是最大误杀源。
- `session_start` 仅记 `startedAt`（Map<sessionId, epoch ms>）；`reason==="startup"` 不清 Map（子线程共享模块实例防御，quality-gate 同款）。

**备选否决**：before_agent_start 注入规则靠模型自觉（这次事故恰恰证明模型会忘）；turn_end 硬杀（会话还在干活，中途杀正在调试的服务）。

## D2 归因：进程启动时间 ∈ 本会话窗口

`/proc/<pid>/stat` 第 22 字段 starttime（clock ticks，comm 含空格/括号 → 取最后一个 `)` 之后解析）+ `/proc/stat` 的 btime + CLK_TCK（`getconf CLK_TCK`，失败回退 100）→ 换算 epoch 秒。`startedAt <= procStart` 即视为本会话 spawn。

多会话/子线程：按 sessionId 隔离窗口；子线程 session_shutdown 只扫自己（更晚起的）窗口，天然不误伤主会话更早起的服务。

## D3 泄漏类判定（五条件合取，缺一不杀）

1. cmdline 命中 dev 服务或自动化工具残留特征（go run 后端 / go-build 编译产物 / pnpm dev / sh -c nuxt dev / nuxt.mjs dev / @nuxt+cli dev worker / **agent-browser CLI / agent-browser 无头 chromium 的 `--user-data-dir=/tmp/agent-browser-chrome-` 窄锚点**）；
2. cwd（`/proc/<pid>/cwd` readlink）在仓库根之下；
3. `tty_nr == 0`（无控制终端）——用户在真实终端手起的服务**永不自动杀**，这是对「人」的豁免；
4. PGID 不在 pidfile 白名单（.pi/run/backend.pgid / front.pgid）——经 start-dev.sh 起的服务是**合法长驻**，永不杀；
5. PGID ≠ 扩展自身进程组。

模式刻意用窄匹配：`pnpm generate/test:unit/build`、`go build`、pi-web 的 next-server、chromium 均不命中（白盒分类表逐条验证）。

## D4 清理动作：进程组 TERM → 2s → KILL

对泄漏进程取其 PGID，`kill(-PGID, SIGTERM)` → 轮询 2s（150ms 间隔）→ 仍存活 `kill(-PGID, SIGKILL)`。整组杀的原因：`go run`（父）与编译产物（子）、pnpm→sh→nuxt→worker 链共享 PGID（setsid 语义），单杀 PID 必留孤儿——这正是本次事故的成因之一。防护：PGID===1 或===自身组直接跳过（`kill(-1)` 会广播全系统，属灾难性误杀）；同组多个泄漏进程（如 go run 父子）**去重后只发一次信号**。

## D5 pidfile 契约（start-dev.sh ↔ guard 的白名单协议）

- start_back/start_front 在 `nohup setsid … &` 后写 `$!`（setsid 会话首进程 = PGID）到 `.pi/run/<name>.pgid`（`.pi/` 已被 .gitignore 整体排除，运行产物不申请放行）。
- `stop`/`--restart` 清理 = 端口 PID（lsof，现有路径保留）∪ pidfile PGID 组杀；**端口已释放但 pidfile 组仍存活（僵尸栈）也要清**（本次 21:05 栈的同款形态）；stop 完成删 pidfile。**端口 PID 在 TERM 宽限（10s）后仍在监听时升级 `kill -9`**（后端优雅关停偏慢：`Registry.StopAll(30s)` 上限 + 端口要等 `os.Exit(0)` 才释放，2026-09-17 实测 ≈50s，10s 观察窗内必误报「仍在监听」致 restart 空转；dev 栈无状态可丢，确定性优先）。
- `is_up` 跳过路径**不得覆盖**既有 pidfile；pidfile 损坏（非数字/进程已死）视为无主，stop 侧直接删除。
- guard 侧：pidfile 读出 PGID 集合 → D3 条件 4 白名单；读不到/损坏 → 空集合（fail-open，绝不因此把 pidfile 服务误判为泄漏——宁可漏杀不可误杀）。

## D6 记账口径（复用 policy.decision 词汇，直写 logEvent，零 schema 迁移）

- kill：`logEvent(kind:"policy.decision")`，payload `{decision:"orphan-killed", cmd:"dev-process-guard", procs:[{pid,pgid,ageSec,cmd(≤120字符，≤20条)}], windowStartedAt}`；
- warn：同上，`{decision:"orphan-warn", …, offenders:[…]}`；
- **不走 logPolicyDecision helper**：其 action 白名单（block/warn/bypass/fail-open）会让 harness-retro 的既有分桶（③ harness 故障按 fail-open、④ 软提醒失效按 warn、催修时距按 block）误吸本扩展事件——直写时用 `decision` 键而非 `action` 键，retro 按 `$.action` 过滤自然跳过，零污染且可独立考古；
- fail-open/平台 no-op：console.warn 留痕，**不记账**（健康路径零写入，对齐 ui-design-gate 口径）。保留期随 policy.decision 30 天。

## D7 平台边界

仅 Linux 实现（/proc 可用性探测）：`process.platform !== "linux"` 或 `/proc/self/stat` 不可读 → 扩展注册钩子但全程 no-op + 一次性 console.warn。fail-open：扫描/单进程读取/组杀/记账任一异常，跳过该目标继续，绝不阻断会话关闭链。

## D8 测试策略

- lib 拆 `.pi/extensions/lib/dev-process-scan.ts`：纯函数（classifyCmdline / parseProcStat / decide）与 IO（/proc 扫描）分离，纯函数直接 fixture 白盒；
- 扩展冒烟 `.pi/extensions/tests/dev-process-guard.smoke.cjs`：esbuild bundle → handler 回放（杀进程动作经注入 stub 验证，不真杀）；
- 脚本冒烟 `scripts/start-dev.smoke.sh`：setsid sleep 假进程组验证 pidfile 写/停/僵尸栈清理（真进程、无端口依赖）；
- 用例来源：子线程枚举白盒分支表/边界值（test-cases.md），断言判据主线程核定。

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 误杀用户/其他会话在用服务 | D3 三重豁免（tty/pidfile/窗口）+ 遗留存量只提醒不杀 |
| AGENTS.md 树上归属冲突（redesign-reading-pane 等在改） | 只做最小增量（计数 + 一条红线），edit 精准锚定 |
| session_shutdown 阻塞会话关闭 | 扫描 O(procs) 毫秒级；TERM 宽限硬顶 2s；全链路 try/catch fail-open |
