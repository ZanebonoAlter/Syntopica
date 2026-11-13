## ADDED Requirements

### Requirement: 无头浏览器栈定时回收

系统 SHALL 提供常驻巡检入口 `scripts/harness/browser-leak-guard.sh`（配套用户级 systemd timer，周期 10 分钟），只针对 agent-browser 残留的无头浏览器栈执行回收；其他 dev 进程（后端 / 前端 / 数据库）MUST NOT 被该入口触碰（归 `dev-process-guard` 扩展管）。

栈的识别基准 MUST 为窄锚点：守护进程 = cmdline 含 `agent-browser/bin/agent-browser-` 且 `/proc/<pid>/environ` 含 `AGENT_BROWSER_DAEMON=1`；浏览器进程树 = 该守护进程的后代中 cmdline 含 `--user-data-dir=/tmp/agent-browser-chrome-` 者及其进程组；session 名取自守护进程 environ 的 `AGENT_BROWSER_SESSION`。用户自己的 chromium（不带该临时 profile 标志）MUST NOT 命中。

回收 MUST 为「安全五条件合取」，任一不满足即跳过并在报告中标明原因（`young` / `busy` / `first-seen` / `client-attached` / `self-protected` / `no-chromium`）：

1. 栈年龄 ≥ `--min-age`（默认 **1800 秒**，从守护进程启动时刻算；上游客 `agent-browser` daemon 自带 1 小时 idle 关停，本阈值取「上游一半」以减少钉内存时长，同时避免与「人在用、两次命令之间在想事」的会话抢会话）；
2. 两次采样（默认 1 秒间隔、`--idle-samples` 控制采样次数）的栈内累计 CPU 时间（utime+stime 求和）不变，即时判定为冻结；
3. **跨轮次冻结证据**：同一 `session + daemon starttime + cpu ticks` 在上一次巡检（≥ 1 个 timer 周期前）已被观测为冻结，本轮仍冻结；首次观测只落状态文件并标 `first-seen`，MUST NOT 动手——纯 1 秒采样窗无法区分「真死」与「LLM 两次 CLI 调用之间在思考」；状态存 `$XDG_RUNTIME_DIR/syntopica-browser-leak-guard/state`（daemon starttime 变化即视为新栈、重新取证）；
4. 不存在活着的客户端：除栈自身与其它 `AGENT_BROWSER_DAEMON=1` 守护进程外，无进程的 `/proc/<pid>/environ` 含该 session 名**或** cmdline 含 `--session <名>` / `--session=<名>`，且其 cmdline 含 `agent-browser`（真 CLI 调用形态；实测 chromium 的 `chrome_crashpad_handler` 会继承 `AGENT_BROWSER_SESSION`/`AGENT_BROWSER_DAEMON`，故 MUST 排除 daemon 与继承 env 的 helper）。判定 MUST 使用**动手前的即时快照**重算，不得复用巡检起始时的旧快照；
5. 待杀进程组 PGID 既不为 1 也不等于脚本自身进程组，且组首自证（`/proc/<pid>/stat` 的 pgrp 等于该 pid）——不自证的 PGID 只 kill 单 PID，不整组杀。

回收动作 MUST 优雅优先：先 `agent-browser close --session <名>`（超时 15 秒），宽限 5 秒后栈仍存活则 TERM 已识别的 chromium 进程组与守护进程组，再 2 秒后仍存活则 KILL；close 返回非 0 时 MUST 先复核栈是否其实已自退（已退则不降级发信号）。进程全部退出后删除对应 `/tmp/agent-browser-chrome-<uuid>` 目录（删除前 MUST 复核无进程 cmdline 仍引用该目录；路径 MUST 通过「前缀绝对路径、非 `/`、不含 `..`、规范化后仍以前缀开头」校验，`BLG_PROFILE_PREFIX` 等注入口为空串/`/`/相对路径时 MUST 以退出码 2 拒绝运行），并尽力清理 `$XDG_RUNTIME_DIR/agent-browser/<session>.*` 残留 socket/pid（同样复核无持有者）。任一目标处理异常 MUST fail-open（跳过该目标继续处理其余目标，不中断整个巡检）。巡检 MUST 用 `flock` 保证单实例（定时器与人工并发时后者直接退出 0）。

脚本 MUST 默认 dry-run（只报告、发出信号为零），仅 `--apply` 真正回收；定时器 unit 的 ExecStart MUST 带 `--apply`。输出 MUST journal 友好（单行前缀 `[browser-leak-guard]`），`--json` 输出机器可读账本。退出码约定：0 = 无泄漏或已全部回收（仅 `young`/`busy`/`first-seen`/`client-attached` 等跳过项也算 0，避免定时器在「有人正在用浏览器」时每轮记失败），1 = 报告模式下发现可回收泄漏未处置，或 `--apply` 下有目标回收失败，2 = 参数/环境错误。`--quiet` MUST NOT 吞掉「发现泄漏」「回收失败」「无 chromium 根」这三类内容（`--json` 在有泄漏账本时也不得被 `--quiet` 抑制）。`--install` / `--uninstall` 幂等安装/卸载用户级 systemd 定时器。

#### Scenario: 陈旧无主栈被回收

- **WHEN** 某 agent-browser 栈守护进程启动已 20 分钟、两次采样 CPU 时间冻结、无 `agent-browser …` CLI 客户端挂在同 session 上、进程组非 init/自身
- **THEN** `--apply` 先对该 session 执行 `agent-browser close`，栈内进程全部退出后删除其 `/tmp/agent-browser-chrome-<uuid>`，输出一行 `reaped` 记录且退出码 0

#### Scenario: 正在干活的栈不动手

- **WHEN** 栈在两次采样间累计 CPU 时间增长（如正在跑截图/渲染）
- **THEN** 该栈不被 close、不被发信号，报告行标记 `busy` 且退出码按「无处置泄漏」口径返回

#### Scenario: 有活跃客户端时不抢会话

- **WHEN** 存在进程以 `agent-browser` CLI 形式挂在同 session 名下（例如另一会话正在 `eval`/`screenshot`）
- **THEN** 该栈跳过，不执行 `agent-browser close`，报告行标记 `client-attached`

#### Scenario: 太年轻不回收

- **WHEN** 栈年龄小于 `--min-age`（默认 1800 秒）
- **THEN** 跳过，报告行标记 `young`

#### Scenario: 首轮只取证不动手

- **WHEN** 某栈年龄已超 `--min-age` 且本轮 CPU 冻结，但上一轮巡检未记录过它（状态文件无同 `session + daemon starttime + cpu ticks` 条目）
- **THEN** 本轮只记录状态并标 `first-seen`，不发信号；下一轮仍冻结且无客户端时才回收

#### Scenario: 隔轮间歇干活不被判死

- **WHEN** 栈在上一轮与本轮之间累计 CPU 时间有增长（两次 CLI 调用之间仍在渲染/轮询）
- **THEN** 不回收，状态记录更新为本轮 ticks，`first-seen` 计时重新开始

#### Scenario: `--session` 形式的客户端也拦住

- **WHEN** 存在 cmdline 为 `agent-browser --session <名> snapshot` 的客户端进程（environ 未设 `AGENT_BROWSER_SESSION`）
- **THEN** 该栈判为 `client-attached` 跳过，不执行 close

#### Scenario: 默认只报告不动手

- **WHEN** 不带 `--apply` 运行且存在可回收栈
- **THEN** 只打印报告、`--json` 账本标 `would-reap`，进程与 `/tmp` profile 均保持原状，退出码 1

#### Scenario: 优雅关闭失败降级组杀

- **WHEN** `agent-browser close --session <名>` 超时或栈在宽限期内仍存活
- **THEN** 脚本 TERM 相应进程组、2 秒后仍存活则 KILL，最终栈清空且 `/tmp` profile 被删除

#### Scenario: 用户自起 chromium 不误伤

- **WHEN** 系统中存在 chromium 进程但其 cmdline 不含 `--user-data-dir=/tmp/agent-browser-chrome-`
- **THEN** 该进程不出现在任何候选/处置名单中

#### Scenario: 定时器周期回收

- **WHEN** 用户级 `syntopica-browser-leak-guard.timer` 已 `enable --now`（10 分钟周期）
- **THEN** 每个周期以 `--apply --quiet` 调起脚本；无泄漏时零输出、零信号、零 `journal` 噪音，有泄漏时 journal 出现 `[browser-leak-guard]` 前缀的处置行
