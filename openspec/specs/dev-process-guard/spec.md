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

扩展 SHALL 在 `turn_end` 检查当前存活的泄漏类进程（含窗口外历史遗留），存在时以 steer 消息**每会话至多一次**提醒：列出 pid/pgid/年龄/命令行，区分「本会话窗口内（会话结束将被自动清理）」与「历史遗留（需人工处置）」，并给出处理建议（经 `scripts/start-dev.sh` 接管或手动清理）；零泄漏时 MUST 零输出零记账。提醒事件记 `policy.decision`（decision=`orphan-warn`）。

#### Scenario: 首次提醒后静默

- **WHEN** 同一会话存在泄漏类进程且已提醒过一次，后续 turn_end 进程仍在
- **THEN** 不再重复提醒（每会话至多一次）

#### Scenario: 零泄漏零输出

- **WHEN** 系统中不存在任何泄漏类进程
- **THEN** turn_end 无 steer 消息、无 events.db 写入

### Requirement: start-dev.sh pidfile 契约

`scripts/start-dev.sh` SHALL：起后端/前端后把 setsid 会话首进程 PID（= PGID）写入 `.pi/run/backend.pgid` / `.pi/run/front.pgid`；`is_up` 跳过路径 MUST NOT 覆盖既有 pidfile；`stop`/`--restart` 的清理范围为「端口监听 PID（现有 lsof 路径）∪ pidfile 记录的进程组」，**端口未监听但 pidfile 进程组仍存活（僵尸栈）时也 MUST 执行组清理**；组清理先 TERM 后 KILL，完成后删除 pidfile；pidfile 缺失/损坏时按无主处理（删除，不报错）；`status` SHALL 显示 pidfile 接管状态。

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
