<!-- domain: harness -->

# dev-process-guard Specification

## Purpose
TBD - created by archiving change dev-process-guard. Update Purpose after archive.

## Requirements

### Requirement: 泄漏类 dev 进程判定

扩展 SHALL 以以下五条件的**合取**判定「泄漏类 dev 进程」，任一不满足即排除：

1. cmdline 命中 dev 服务或自动化工具残留特征之一：`go run …cmd/server/main.go`、`.cache/go-build/…/main`（go run 编译产物路径结尾）、`sh -c nuxt dev`、`nuxt.mjs dev`、`@nuxt+cli …/dev/index.mjs`、`pnpm dev`、`agent-browser`（AI 浏览器自动化 CLI，残留实测见 2026-09-17 取证：147654 会话首 + chromium 整树 ~1.3GB）、`--user-data-dir=/tmp/agent-browser-chrome-`（agent-browser 拉起的无头 chromium 窄锚点——用户自己的 chromium 不带此临时 profile 标志）；
2. 进程 cwd 位于仓库根目录之下；
3. `/proc/<pid>/stat` 的 tty_nr 为 0（无控制终端）；
4. 进程 PGID 不在 pidfile 白名单集合（`.pi/run/backend.pgid`、`.pi/run/front.pgid` 记录的 setsid 会话首进程 ID）；
5. 进程 PGID 不等于扩展自身所在进程组。

判定 MUST 为纯函数（输入进程快照与配置，输出 kill 候选/提醒名单），模式匹配 MUST 刻意窄化：`pnpm generate`、`pnpm test:unit`、`pnpm build`、`go build`、pi-web 的 next-server、chromium MUST NOT 命中。pidfile 白名单读取失败或内容损坏 MUST 按空集合处理（宁可漏杀不可误杀）。

#### Scenario: 五条件全中判为泄漏

- **WHEN** 进程 cmdline 为 `go run cmd/server/main.go`、cwd 在仓库内、tty_nr=0、PGID 不在白名单、不等于自身组
- **THEN** 判定为泄漏类，进入处置名单

#### Scenario: 任一条件不满足即排除

- **WHEN** 五条件中仅tty_nr≠0（用户终端手起）/ 仅 PGID 在 pidfile 白名单 / 仅 cwd 在仓库外 / 仅 cmdline 不命中 / 仅 PGID 等于自身组，其余条件满足
- **THEN** 五种单因子证伪场景均不进入 kill 名单（其中「仅窗口外」以外的提醒语义见软提醒 Requirement）

#### Scenario: 邻界进程不误伤

- **WHEN** 进程为 `pnpm generate`、`pnpm test:unit`、`pnpm build`、`go build ./...`、`next-server (v16.3.1)` 或 chromium
- **THEN** cmdline 模式不命中，不判为泄漏类

#### Scenario: 浏览器自动化残留命中

- **WHEN** 进程为 agent-browser CLI（cmdline 含 `agent-browser`）或其拉起的无头 chromium（cmdline 含 `--user-data-dir=/tmp/agent-browser-chrome-`），且满足其余泄漏条件
- **THEN** 判定为泄漏类，随会话结束被整组清理（chromium renderer/zygote 与主进程同 PGID，组杀覆盖）

#### Scenario: 用户自起 chromium 不误伤

- **WHEN** chromium 进程 cmdline 不含 `--user-data-dir=/tmp/agent-browser-chrome-`（用户正常 profile）
- **THEN** 即使 cwd 在仓库内也不命中模式，不判为泄漏类

#### Scenario: pidfile 白名单损坏时 fail-safe

- **WHEN** pidfile 内容非数字或指向已死亡进程
- **THEN** 白名单按空集合处理，不抛异常、不阻断扫描

### Requirement: 会话窗口归因与会话结束自动清理

扩展 SHALL 在 `session_start` 记录本会话窗口起点（按 sessionId 隔离存储，兼容子线程共享模块实例各记各的）；在 `session_shutdown` 对**启动时间落在本会话窗口内**的泄漏类进程执行：TERM 其进程组 → 最多 2s 宽限 → 仍存活则 KILL 进程组，并记 `policy.decision` 事件（decision=`orphan-killed`，payload 含进程 pid/pgid/年龄/命令行与窗口起点）。窗口外的历史遗留进程 MUST NOT 被自动清理。对 PGID 为 1 或扩展自身进程组的目标 MUST 跳过。清理链路任一步异常 MUST fail-open（跳过该目标继续，不阻断会话关闭）。

#### Scenario: 窗口内泄漏进程被整组清理

- **WHEN** 会话期间 spawn 了 `nohup setsid pnpm dev --host &`（进程组含 pnpm/sh/nuxt/worker），会话关闭时进程组仍存活
- **THEN** TERM 后进程组内进程全部退出（或 2s 后 KILL），events.db 新增 orphan-killed 事件且 payload 列出该组进程

#### Scenario: 窗口外遗留不被自动杀

- **WHEN** 泄漏类进程的启动时间早于本会话窗口起点（如上一会话遗留的手搓后端）
- **THEN** session_shutdown 不对它执行任何 kill

#### Scenario: 自我防护

- **WHEN** 候选目标的 PGID 为 1 或等于扩展自身进程组
- **THEN** 跳过该目标，不发出任何信号

#### Scenario: 扫描中进程消失不炸

- **WHEN** 扫描 enumerating → 读取某 /proc 条目时进程已退出
- **THEN** 跳过该条目继续处理其余目标，会话关闭正常完成

#### Scenario: 非 Linux 平台整体 no-op

- **WHEN** 运行平台非 Linux 或 /proc 不可读
- **THEN** 扩展注册钩子但不执行任何扫描与信号动作，仅一次性 console.warn 留痕

### Requirement: turn_end 软提醒

扩展 SHALL 在 `turn_end` 检查当前存活的泄漏类进程（含窗口外历史遗留），存在时以 steer 消息**每会话至多一次**提醒：列出 pid/pgid/年龄/命令行，区分「本会话窗口内（会话结束将被自动清理）」与「历史遗留（需人工处置）」，并给出处理建议（经 `scripts/dev/start-dev.sh` 接管或手动清理）；零泄漏时 MUST 零输出零记账。提醒事件记 `policy.decision`（decision=`orphan-warn`）。

#### Scenario: 首次提醒后静默

- **WHEN** 同一会话存在泄漏类进程且已提醒过一次，后续 turn_end 进程仍在
- **THEN** 不再重复提醒（每会话至多一次）

#### Scenario: 零泄漏零输出

- **WHEN** 系统中不存在任何泄漏类进程
- **THEN** turn_end 无 steer 消息、无 events.db 写入

### Requirement: start-dev.sh pidfile 契约

`scripts/dev/start-dev.sh` SHALL：起后端/前端后把 setsid 会话首进程 PID（= PGID）写入 `.pi/run/backend.pgid` / `.pi/run/front.pgid`；`is_up` 跳过路径 MUST NOT 覆盖既有 pidfile；`stop`/`--restart` 的清理范围为「端口监听 PID（现有 lsof 路径）∪ pidfile 记录的进程组」，**端口未监听但 pidfile 进程组仍存活（僵尸栈）时也 MUST 执行组清理**；组清理先 TERM 后 KILL，完成后删除 pidfile；pidfile 缺失/损坏时按无主处理（删除，不报错）；`status` SHALL 显示 pidfile 接管状态。

#### Scenario: 起服务写 pidfile

- **WHEN** `start_back`/`start_front` 成功拉起 nohup setsid 后台进程
- **THEN** 对应 `.pi/run/<name>.pgid` 存在且内容为该会话首进程 PID

#### Scenario: 已在跑跳过不覆盖

- **WHEN** 端口已在监听时再次执行 start
- **THEN** 跳过启动且既有 pidfile 内容不变

#### Scenario: 僵尸栈清理（端口已释放、进程组存活）

- **WHEN** 前端 dev 栈未监听任何端口但 pidfile 进程组仍存活，执行 stop
- **THEN** 进程组内全部进程被终止，pidfile 被删除

#### Scenario: stop 常规清理

- **WHEN** 端口在监听且 pidfile 存在，执行 stop
- **THEN** 端口监听进程与 pidfile 进程组均被清理，pidfile 删除

#### Scenario: pidfile 缺失降级

- **WHEN** pidfile 不存在或内容损坏，执行 stop
- **THEN** 沿用端口清理路径，行为与旧版兼容，无报错退出异常

### Requirement: 记账口径

自动清理与软提醒 MUST 分别记 `policy.decision` 事件（decision=`orphan-killed` / `orphan-warn`，change 列为当前绑定 change 可空），随既有 policy.decision 保留期（30 天）清扫；健康路径（零泄漏、正常放行）与非 Linux no-op MUST NOT 记账。

#### Scenario: 事件可考古

- **WHEN** 一次会话结束清理杀掉进程组并触发过一次提醒
- **THEN** events.db 可按 decision=orphan-killed / orphan-warn 各查到对应行，payload 含进程清单

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
