# Design — add-browser-leak-guard

## D1 只盯 agent-browser，不扩到其他 dev 进程

`dev-process-guard` 扩展（八模式锚点）已在会话结束/`turn_end` 覆盖 go run / nuxt dev / pnpm dev / agent-browser。本入口不重复那套判定，只解决它管不到的一类：**跨会话累积、窗口外无主、或会话还在但忘了 close 的浏览器栈**。范围收窄到「daemon 带 `AGENT_BROWSER_DAEMON=1` + chromium 带 `/tmp/agent-browser-chrome-` 临时 profile」两个窄锚点，最大化降低误杀面（spec「用户自起 chromium 不误伤」）。

## D2 常驻 systemd timer，而不是扩展内定时

泄漏源头是「会话已死、daemon 被 systemd 收养」。pi 扩展只在 pi 进程活着时运行，杀掉泄漏源的场景恰恰是 pi 会话消失之后。故用用户级 systemd timer（`Linger=yes` 已验证开启，注销后仍周期跑），与 `sync-demo-seed` 同款落地方式（unit 入库 `scripts/harness/systemd/`，`cp` + `enable --now` 安装）。

## D3 判定用「危险度」四条件，不用「父进程是否 init」

daemon 正常工作时也必然 reparent 到 systemd（它主动 setsid 脱离终端），所以「ppid=1/1328」不能作为孤儿判据——2026-11-13 取证时两套残留与一套在用的栈 ppid 完全相同。

改用五条正交的危险度证据合取（2026-11-13 审查后从四条补到五条，见 D3b）：

| 条件 | 依据 | 反例（防止误杀） |
| --- | --- | --- |
| 年龄 ≥ 1800s | 上游 `agent-browser` daemon 自带 1h idle 关停（README: "After 1 hour with no commands … closes the browser and exits"），本入口取「上游一半」减少钉内存时长 | 刚起的栈、10 分钟内就忘关的会话 |
| 本轮 CPU 时间冻结（两次采样） | 2026-11-13 实测：残留栈 5 秒内 utime/stime 纹丝不动（47s→47s）；在用栈 5 秒涨 194s | 正在渲染/滚动的栈 |
| **跨轮次冻结（≥2 轮）** | 纯 1 秒采样窗分不清「真死」与「LLM 两次 CLI 调用之间在思考」；LLM 驱动会话天然长时间零 CPU | 间歇干活（上一轮 ticks 与本轮不同）的栈 |
| 无活 CLI 客户端 | CLI 调用两种写法都要认：`AGENT_BROWSER_SESSION` env **或** `--session <名>` argv；且 cmdline 必须含 `agent-browser`（排除继承了 env 的 crashpad helper） | 会话正在驱动浏览器 |
| 进程组安全 | PGID ∉ {1, 自身 PGID} **且组首自证**（pgrp==pid），不自证只 kill 单 PID | 杀 init 组、杀死自己、连带杀调用壳兄弟进程 |

`--min-age` 可调，但上调只能更保守；跨轮次证据不可关闭（`--no-cross-round` 不提供——那等于把 D3b 的风险重新引回来）。

## D3b 跨轮次冻结证据（审查后补，关键安全阀）

2026-11-13 实现第一版用「单轮 1 秒采样 CPU 冻结 + 年龄 ≥ 600s」判定，code review 指出两个真误杀面：

1. LLM 驱动的 `cityenv-*` 验收会话两次 CLI 调用之间天然零 CPU（思考期秒～分钟级），单轮冻结判不出「在思考」还是「已死」；
2. 上游 agent-browser 自己 1 小时 idle 才关，600s 阈值与它在同一量级，收益低而风险集中在「在用会话」上。

故引入状态文件 `$XDG_RUNTIME_DIR/syntopica-browser-leak-guard/state`（行：`session daemon_starttime cpu_ticks epoch`）：

- 首次观测（无同 `session+starttime+ticks` 条目）→ 只落盘，标 `first-seen`，不动手；
- 下一轮仍同 ticks → 判无主，进入回收；ticks 有变 → 状态刷新，重新取证；
- daemon starttime 变化（会话重启）→ 视为新栈，重新取证；
- 默认阈值抬到 1800s（上游 1h 的一半），配合 10 分钟周期 → 实际回收落在「年龄 30～40 分钟、连续两轮零 CPU」区间。

代价是多一轮延迟，换来「在用会话永不被抢」——对一个会 kill 别人验收会话的定时器，这个交换值得。

`agent-browser close --all` 会连正在用的会话一起关（2026-11-13 首次人工清理时只能在无活跃任务时用）。脚本改用 `agent-browser close --session <名>`：session 名从 `/proc/<daemon>/environ` 的 `AGENT_BROWSER_SESSION` 读出（实测存在，如 `cityenv-1218187`），只关自己认定的那一套。

## D4 回收优雅优先，session 名从 environ 读

`agent-browser close --all` 会连正在用的会话一起关（2026-11-13 首次人工清理时只能在无活跃任务时用）。脚本改用 `agent-browser close --session <名>`：session 名从 `/proc/<daemon>/environ` 的 `AGENT_BROWSER_SESSION` 读出（实测存在，如 `cityenv-1218187`），只关自己认定的那一套。

close 不生效（daemon 卡死/版本不支持）时先复核栈是否其实已自退（close rc≠0 但进程已没了 → 不降级发信号），再降级：TERM 已识别的 chromium 进程组 + daemon 组 → 2s → KILL；进程全退后才删 `/tmp/agent-browser-chrome-<uuid>`，删前复核无 cmdline 引用（防 1 秒内新进程复用同名目录），并尽力清 `$XDG_RUNTIME_DIR/agent-browser/<session>.*` 残留 socket/pid（优雅 close 会自清，KILL 路径不会）。路径删除前过「空前缀 / `/` / 相对路径 / `..` / 规范化越界」校验，注入口非法直接 exit 2；巡检加 `flock` 单实例，防定时器与人工并发判同一栈、双关同会话。

## D5 dry-run 默认，`--apply` 才动手

手动排查时默认不可破坏（报告 + 退出码 1），只有 unit 的 ExecStart 带 `--apply --quiet`。这样「人跑一下看看」与「定时器自动清」是两条明确路径，避免复制粘贴一个命令就把别人的验收会话关了。

## D6 测试用假进程，绝不启动真浏览器

冒烟用例在临时目录里用「复制 `bash`/`sleep` 成同名假二进制 + 注入 argv 里的 profile 参数 + `AGENT_BROWSER_DAEMON=1` / `AGENT_BROWSER_SESSION=…` 环境变量」构造伪造栈，全部外部命令经 `BLG_CLOSE_CMD`、`BLG_PROFILE_PREFIX`、`BLG_DAEMON_PATTERN`、`BLG_MIN_AGE` 注入口覆盖。这样 smoke 可在 CI/树莓派上秒级跑完，且不会与真实 agent-browser 会话抢进程（2026-11-13 现场就有一套在用栈，测试若碰它会直接毁掉别人验收）。

## D7 本轮不写 events.db

bash 直写 `events.db` 需耦合 harness-log 的 schema 与魔数（扩展侧负责），本轮先以 **journal + 退出码** 为账本；`harness-facts` 查询可用 `journalctl --user -u syntopica-browser-leak-guard` 对齐时间轴。若后续复盘需要机器可读记账，另开 change 走统一 `logEvent` 通道（不在本 change 夹带）。
