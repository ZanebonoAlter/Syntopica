# Tasks — add-browser-leak-guard

## 1. 巡检脚本 `scripts/harness/browser-leak-guard.sh`

- [x] 1.1 头注释（用法/退出码/注入口/与 dev-process-guard 分工）+ `set -uo pipefail`；参数解析 `--apply` `--json` `--quiet` `--min-age <s>` `--idle-samples <n>` `--install` `--uninstall` `--help`；非法参数 exit 2
- [x] 1.2 栈枚举：`/proc/*/cmdline` 命中 `BLG_DAEMON_PATTERN`（默认 `agent-browser/bin/agent-browser-`）且 `/proc/<pid>/environ` 含 `AGENT_BROWSER_DAEMON=1` → 守护进程；读 `AGENT_BROWSER_SESSION`（缺省 `default`）；沿父进程映射收集后代，其中 cmdline 含 `--user-data-dir=<BLG_PROFILE_PREFIX>`（默认 `/tmp/agent-browser-chrome-`）者为 chromium 根并取其 PGID；单条目读取失败跳过（fail-open）
- [x] 1.3 安全五条件判定（design D3）：年龄（`/proc/<pid>/stat` starttime × btime）≥ `--min-age`（**默认 1800s**）；本轮两次采样（间隔 1s，`--idle-samples` 控制次数）栈内 utime+stime 求和不变；**跨轮次冻结证据**——状态文件 `$XDG_RUNTIME_DIR/syntopica-browser-leak-guard/state` 中同 `session + daemon starttime + cpu ticks` 上轮已冻结（首次观测只落盘标 `first-seen`）；无活客户端（environ 含同 session **或** cmdline 含 `--session <名>`/`--session=<名>`，且 cmdline 含 `agent-browser`，排除 `AGENT_BROWSER_DAEMON=1` 的 daemon 与 crashpad helper）；PGID ∉ {1, 自身 PGID} **且组首自证**（pgrp==pid，不自证则只 kill 单 PID）；动手前用**即时快照重算**客户端；未命中条件在报告里标 `young` / `busy` / `first-seen` / `client-attached` / `self-protected` / `no-chromium`
- [x] 1.3b 状态文件读写：原子写（临时文件 + mv）、条目清理（daemon 消失即删）、`XDG_RUNTIME_DIR` 不可用时降级到 `$TMPDIR`
- [x] 1.4 dry-run 与 `--apply` 分叉：默认只报告（`--json` 账本 outcome=`would-reap`，退出码 1）；`--apply` 执行回收
- [x] 1.5 回收动作（design D4）：`AGENT_BROWSER_SESSION=<名> <BLG_CLOSE_CMD> --session <名>`（默认 `agent-browser close`，超时 15s）→ 先复核栈是否已自退 → 宽限 5s 轮询（每 200ms）→ 仍存活 TERM chromium 根 PGID 与 daemon PGID（仅自证组）→ 2s → KILL；全部退出后 `rm -rf` profile（前缀/`..`/空串校验 + 复核无进程 cmdline 引用后才删），并尽力清 `$XDG_RUNTIME_DIR/agent-browser/<session>.*`（复核无持有者）；单目标异常 fail-open 继续
- [x] 1.6 输出与退出码：journal 友好单行前缀 `[browser-leak-guard]`（`reaped` / `would-reap` / 跳过原因 / 汇总一行）；`--quiet` 仅在发生回收/异常时输出，**但 MUST NOT 吞掉「发现泄漏」「回收失败」「no-chromium」三类内容**（`--json` 有泄漏账本时也不得被 quiet 抑制）；退出码 0 = 干净或全部回收成功（仅跳过项也计 0），1 = 报告模式发现可回收泄漏未处置或 `--apply` 下有目标失败，2 = 参数/环境错误；单实例 `flock`（非阻塞，拿不到锁即 exit 0）
- [x] 1.6b 注入口硬校验：`BLG_PROFILE_PREFIX` 必须非空绝对路径、不以 `/` 结尾、≥ 2 段；`BLG_DAEMON_PATTERN` 非空；非法即 exit 2（防 `rm -rf` 越界与全量命中）
- [x] 1.7 `--install` / `--uninstall`：`cp scripts/harness/systemd/syntopica-browser-leak-guard.{service,timer} ~/.config/systemd/user/` → `systemctl --user daemon-reload` → `enable --now` / `disable --now` + 删文件；幂等、打印 `systemctl --user list-timers` 关键行；非 Linux / 无 `systemctl` 时 exit 2 并说明

## 2. systemd 用户单元 `scripts/harness/systemd/`

- [x] 2.1 `syntopica-browser-leak-guard.service`：`Type=oneshot`、`WorkingDirectory=%h/software/Syntopica`、`ExecStart=/bin/bash %h/software/Syntopica/scripts/harness/browser-leak-guard.sh --apply --quiet`、`TimeoutStartSec=10min`、`Environment=PATH=…`（含 node 安装位兜底，`agent-browser` 在 `~/.nvm/.../bin`）
- [x] 2.2 `syntopica-browser-leak-guard.timer`：`OnBootSec=5min`、`OnUnitActiveSec=10min`、`AccuracySec=30s`、`Unit=syntopica-browser-leak-guard.service`、`WantedBy=timers.target`；头注释写清调周期/查日志/卸载三件事

## 3. 冒烟 `scripts/harness/browser-leak-guard.smoke.sh`

- [x] 3.1 fixture：临时目录内复制 `bash`/`sleep` 造伪造 daemon 与 chromium（argv 注入 `--user-data-dir=<临时前缀>`、env 注入 `AGENT_BROWSER_DAEMON=1` / `AGENT_BROWSER_SESSION=<名>`），`BLG_*` 注入口指向 fixture；全程不启动真浏览器
- [x] 3.2 用例矩阵：①陈旧冻结栈 → `--apply` 回收（close 被调用 + 进程消失 + profile 目录删除）②有活 CLI 客户端（env 形式与 `--session` argv 形式各一）→ 跳过 `client-attached` ③CPU 在涨 → 跳过 `busy` ④太年轻 → 跳过 `young` ⑤首轮 → `first-seen` 不动手、次轮才回收 ⑥报告模式 → `would-reap` 且进程仍在、退出码 1 ⑦close 失败 → TERM/KILL 降级成功 ⑧close 返回非 0 但栈已自退 → 不降级发信号 ⑨无 profile 锚点的独立 chromium → 不命中 ⑩多目标 fail-open（一套 `no-chromium` + 一套正常 → 正常那套仍被回收）⑪`--json` 字段与 outcome 稳定（dry-run 与 apply 各一）⑫非法参数/非法注入口 → 退出码 2 ⑬无泄漏 → 零输出退出码 0 ⑭unit 静态断言（ExecStart 含 `--apply --quiet`、周期 10 分钟、`%h` 路径）
- [x] 3.3 结束清理 fixture（trap 杀残留假进程 + 删临时目录），任何断言失败 exit 1 并打印失败项

## 4. 文档

<!-- doc-impact: none(harness 工具链 change：仅改 docs/reference/harness/pi-extensions.md 与 AGENTS.md，均不在 8 个 flow 域路径内；无 flow 业务域变更) -->
- [x] 4.1 `docs/reference/harness/pi-extensions.md`：全景表旁补「browser-leak-guard（systemd timer）」机制小节——栈识别窄锚点、安全四条件、优雅 close 优先、dry-run/`--apply`、与 `dev-process-guard` 分工（窗口内 vs 窗口外常驻）
- [x] 4.2 `AGENTS.md` harness 要点补一句：无主浏览器栈由 10 分钟定时器自动回收，排查 `journalctl --user -u syntopica-browser-leak-guard`，手动看用脚本不带 `--apply`

## 5. 测试

- [x] 5.1 `bash scripts/harness/browser-leak-guard.smoke.sh` → 全绿（14 组用例，含跨轮次首轮不动手、argv 客户端、多目标 fail-open、降级组杀、注入口校验与 dry-run 不破坏断言）
- [x] 5.2 `bash -n scripts/harness/browser-leak-guard.sh` → 语法检查通过
- [x] 5.3 本机真跑两轮：`bash scripts/harness/browser-leak-guard.sh`（dry-run）→ 首轮被标 `first-seen` 的栈**不得**被报为 `would-reap`；间隔一轮后重跑，仍冻结且无客户端者才出现 `would-reap`；真实栈清理后 `ps` 无残留、`/tmp/agent-browser-chrome-*` 无残留（在用的栈 MUST 保持存活）

## 6. 文档一致性

- [x] 6.1 `grep -n 'browser-leak-guard' AGENTS.md docs/reference/harness/pi-extensions.md` → 两处均命中且安装/卸载/日志命令与 unit 文件实际内容一致

## 7. 验证

- [x] 7.1 `bash scripts/harness/browser-leak-guard.sh --help` → 退出码 0，列出全部参数与退出码约定
- [x] 7.2 `bash scripts/harness/browser-leak-guard.sh --apply --quiet; echo $?` → 无泄漏时无输出且退出码 0（或在用的栈被标 `client-attached`/`busy` 后仍退出码 1，脚本不得关它）
- [x] 7.3 `bash scripts/harness/browser-leak-guard.sh --install` 后 `systemctl --user list-timers syntopica-browser-leak-guard.timer` → 出现 NEXT 时间且为 10 分钟周期；`systemctl --user start syntopica-browser-leak-guard.service` 后 `journalctl --user -u syntopica-browser-leak-guard.service -n 5` → 出现 `[browser-leak-guard]` 行或无泄漏空输出
- [x] 7.4 真实栈端到端（真 agent-browser，非 fixture）：`AGENT_BROWSER_SESSION=blg-selftest agent-browser open about:blank` 后只结束 CLI → 首轮 `--apply --min-age 0 --min-gap 0` 标 `first-seen`（栈存活、profile 在），次轮同参 → `reaped ... escalation=close removed=yes runtime=cleaned`，`ps` 无残留、`/tmp/agent-browser-chrome-*` 已删

### Scenario → 测试映射

| Scenario | 测试文件 |
| --- | --- |
| 陈旧无主栈被回收 | scripts/harness/browser-leak-guard.smoke.sh |
| 正在干活的栈不动手 | scripts/harness/browser-leak-guard.smoke.sh |
| 有活跃客户端时不抢会话 | scripts/harness/browser-leak-guard.smoke.sh |
| 太年轻不回收 | scripts/harness/browser-leak-guard.smoke.sh |
| 首轮只取证不动手 | scripts/harness/browser-leak-guard.smoke.sh |
| 隔轮间歇干活不被判死 | scripts/harness/browser-leak-guard.smoke.sh |
| `--session` 形式的客户端也拦住 | scripts/harness/browser-leak-guard.smoke.sh |
| 默认只报告不动手 | scripts/harness/browser-leak-guard.smoke.sh |
| 优雅关闭失败降级组杀 | scripts/harness/browser-leak-guard.smoke.sh |
| 用户自起 chromium 不误伤 | scripts/harness/browser-leak-guard.smoke.sh |
| 定时器周期回收 | 人工：`systemctl --user list-timers syntopica-browser-leak-guard.timer` 显 NEXT = last + 10min；service ExecStart 带 `--apply --quiet`（smoke ⑭ 静态断言兜底） |
