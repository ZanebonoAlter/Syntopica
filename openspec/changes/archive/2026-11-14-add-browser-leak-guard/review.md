# Review — add-browser-leak-guard

审查对象：`scripts/harness/browser-leak-guard.sh`、`browser-leak-guard.smoke.sh`、`systemd/*.{service,timer}`（change-review 只读档执行，规格源为本 change design.md / specs。

### R-1 2026-11-13【High】活客户端只认 environ，漏 `--session` argv 形式且只看巡检起始快照

- **状态**：已修复
- **现象**：`find_clients` 仅按 `AGENT_BROWSER_SESSION` environ 匹配，而上游主流写法是 `agent-browser --session <名> <cmd>`（argv 不带 environ）；且整轮只采一次快照，第二个目标判定时已过期数秒 —— 两条路径都能把「正在被人驱动」的栈判成无主。
- **处置**：`cmdline_has_session` 解析 `--session <名>` / `--session=<名>`；客户端判据收窄为「cmdline 含 `agent-browser` ∧ 非 `AGENT_BROWSER_DAEMON=1` ∧ 非 crashpad helper」；`reap_stack` 动手前用即时快照重算（`scan_clients_fresh` 在 apply 分支重新扫 `/proc`）。冒烟补 env 形式与 argv 形式两套栈。

### R-2 2026-11-13【High】`--min-age 600` 与上游 1h idle 语义错位，会抢「在用但短暂空闲」的会话

- **状态**：已修复
- **现象**：上游 `agent-browser` daemon 自带 1 小时无命令自关（README），600s 阈值只比它早一点，收益低；而 LLM 驱动会话两次 CLI 调用之间天然零 CPU，1 秒采样窗分不出「在思考」与「已死」。
- **处置**：默认阈值抬到 1800s（上游一半）；新增跨轮次冻结证据（状态文件 `session + daemon starttime + cpu ticks`，首轮只标 `first-seen` 不动手）；另加 `--min-gap`（默认 300s）防「手动连跑两轮秒级上膛」。spec/design 已同步，冒烟补 ⑤/⑤b 两轮用例。

### R-3 2026-11-13【Medium】组杀可能连带调用壳兄弟进程

- **状态**：已修复
- **处置**：`build_stack` 对候选 PGID 做组首自证（`pgrp == pid`），不自证者只进 `NOGROUP_PGIDS` 留痕、不进组杀名单，回落单 PID kill（starttime 校验）。

### R-4 2026-11-13【Medium】`rm -rf` 目标校验不足（空前缀 / `/` / 相对路径 / `..` / 规范化越界）

- **状态**：已修复
- **处置**：`BLG_PROFILE_PREFIX` / `BLG_DAEMON_PATTERN` 注入前硬校验（非空、绝对、非 `/`、无尾斜杠、≥2 段、无 `..`，否则 exit 2）；删除前 `normalize_path` + 「不含 `..` ∧ 以 prefix 开头」+ 无进程 cmdline/fd 引用复核。

### R-5 2026-11-13【Low】`close` 返回非 0 时无条件降级发信号

- **状态**：已修复
- **处置**：先 `stack_alive` 复核，栈其实已自退则记 `escalation=close-gone` 不再发信号（冒烟 ⑧）。

### R-6 2026-11-13【Low】`stack_alive` 未比 starttime，PID 复用会误报 `failed` 导致定时器每轮失败

- **状态**：已修复
- **处置**：栈内 PID 与组首均按 starttime 比对（与 `cpu_frozen` 同款护栏）。

### R-7 2026-11-13【Low】`--quiet` 会吞掉「发现泄漏 / 回收失败 / no-chromium」，`--json` 也被压掉

- **状态**：已修复
- **处置**：三类内容走 `say_always`；`--json` 在有 would-reap/reaped/failed/no-chromium 账本时不被 quiet 抑制；顺带修正 `--json` 时人读行走 stderr、stdout 保持纯 JSON（`| python3 json.load` 可解析）。

### R-8 2026-11-13【Low】KILL 降级路径残留 `$XDG_RUNTIME_DIR/agent-browser/<session>.*` socket/pid

- **状态**：已修复
- **处置**：`clean_runtime_state` 按字面前缀清理（复核无进程持有），清不掉只记 `runtime_kept`，不计回收失败。

### R-9 2026-11-13【Low】定时器与人工并发可判同一栈（双关同会话）

- **状态**：已修复
- **处置**：`flock -n` 单实例锁，拿不到锁静默 exit 0。

### R-10 2026-11-13【Low】service 写死 node 安装位版本号，node 升级后 close 步骤退化为纯信号回收

- **状态**：已修复
- **处置**：unit 的 `Environment=PATH` 只留系统目录，`agent-browser` 解析交由脚本（PATH → `~/.nvm/versions/node/*/bin` glob 兜底），注释写明行为。

### R-11 2026-11-13【Low】未采样的跳过项在 JSON 里输出 `cpu_ticks_*: 0`，看起来像「冻结证据」

- **状态**：已修复
- **处置**：young/busy/first-seen/client-attached/self-protected/no-chromium 一律 `null`，只有 would-reap/reaped/failed 带数字。

### R-12 2026-11-13【Low】冒烟覆盖缺口（self-protected、多目标 fail-open、exit 2、argv 客户端、apply 账本、quiet 出声、unit 静态断言）

- **状态**：已采纳
- **处置**：除 `self-protected` 与 `failed` 运行时分支外全部补齐（14 组 / 148 断言）；两项无法用非 root fixture 造出来的分支保留静态断言并在 tasks 备注中说明。
