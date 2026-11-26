<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-11-13 晚树莓派 load 飙到 34（4 核）、swap 2.0G 全满，取证结论：常驻内存里躺着 **3 套无主的 agent-browser 无头浏览器栈**（39 个进程、2.6 GB RSS，年龄 7～30 分钟，CPU 时间 5 秒内纹丝不动）。根因是 `agent-browser open` 会拉起一个脱离终端的 session daemon（`agent-browser-linux-arm64`，env 带 `AGENT_BROWSER_DAEMON=1` 与 `AGENT_BROWSER_SESSION=<名>`）+ 整套 chromium（`--user-data-dir=/tmp/agent-browser-chrome-<uuid>`），活儿干完只要没 `agent-browser close`，daemon 就被 systemd 收养常驻，每套钉住约 780 MB，`/tmp` profile 也留在盘上。

现有 `dev-process-guard` 扩展只在**会话结束**清理「本会话窗口内」启动的进程，窗口外的历史遗留明确「不自动杀，需人工确认」——于是跨会话累积的浏览器栈没人管，只能靠人发现负载高再手动清。且它管不了「会话还活着但忘了 close」这种长久挂着的栈。

## What Changes

- **新增定时巡检脚本** `scripts/harness/browser-leak-guard.sh`：只盯 agent-browser 栈（不碰其他 dev 进程），按「安全四条件合取」判定无主：① 年龄 ≥ 阈值（默认 10 分钟）；② 两次采样的累计 CPU 时间冻结（在干活的不动）；③ 没有任何活着的 agent-browser CLI 客户端挂在该 session 上；④ 栈不是自身/init 进程组。默认只报告，`--apply` 才动手。
- **回收走优雅路径**：`agent-browser close --session <名>`（session 名从 `/proc/<daemon>/environ` 的 `AGENT_BROWSER_SESSION` 读），关不掉再 TERM/KILL 进程组兜底；进程全退后删对应 `/tmp/agent-browser-chrome-<uuid>`。
- **systemd 用户定时器**（`scripts/harness/systemd/*.{service,timer}`，10 分钟一次、开机 5 分钟后首跑），装法与 `sync-demo-seed` 同款：`cp` 到 `~/.config/systemd/user/` + `enable --now`；脚本自带 `--install` / `--uninstall` 幂等封装。
- **冒烟测试** `scripts/harness/browser-leak-guard.smoke.sh`：用假进程（copied `bash`/`sleep` 构造伪造栈）覆盖「陈旧栈被收 / 有客户端不动 / CPU 在动不动 / 太年轻不动 / 报告模式不动手 / close 失败降级组杀 / 非 agent-browser 的 chromium 不误伤」，全程不启动真浏览器。
- **文档**：`docs/reference/harness/pi-extensions.md` 增机制小节（与 dev-process-guard 的分工）+ 全景表一行；`AGENTS.md` harness 要点补一句安装/排查入口。

## Capabilities

### New Capabilities

（无——本 change 是既有 dev 进程治理能力的时序补位，不引入新业务能力。）

### Modified Capabilities

- `dev-process-guard`: 新增「无头浏览器栈定时回收」requirement——把 agent-browser 残留的处置从「仅会话窗口内、仅会话结束时」扩展为「常驻定时器可回收窗口外无主栈」，并规定安全四条件、优雅关闭优先、失败降级组杀、dry-run 默认与 journal 可追溯口径。既有五条件判定、窗口归因、pidfile 契约、turn_end 软提醒语义不变。

## Impact

- 新增：`scripts/harness/browser-leak-guard.sh`、`scripts/harness/browser-leak-guard.smoke.sh`、`scripts/harness/systemd/syntopica-browser-leak-guard.{service,timer}`
- 修改：`docs/reference/harness/pi-extensions.md`、`AGENTS.md`
- 运行环境：安装后自带 10 分钟周期任务（用户级 systemd，`Linger=yes` 已开，注销后在跑）；卸载为删两个 unit + `disable --now`，无状态残留。
- 不触碰 `front/`、`backend-go/`、数据库与任何业务 flow；无 Go / 前端测试影响面。
- 部署后行为变化：无主浏览器栈会在 10 分钟内被自动回收（含 `agent-browser close` 语义）。需要用户操作：装/卸定时器各一条命令（见 tasks 文档节）。
