<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

2026-09-17 晚树莓派资源紧张（load 9.17、内存 5.9/7.9G、swap 已换页），取证结论（`docs/research/orphan-dev-processes/explore-findings.md`，2026-09-17T22:05 快照）：

1. AI 会话起 dev 服务用手搓 `nohup setsid … &` 全脱管——bash 调用立即返回，但**无 PID 账本、无会话结束清理**，进程被 init 收养后无人认领；
2. 实测同时存在**两个孤儿 Nuxt dev 栈**（其一未占任何端口、纯耗内存的僵尸栈）+ 一个手搓后端；当前后端父进程是一条孤儿 `bash -c`，其命令里的 `kill 90802` 证明清理全靠「下一个回合碰巧想起来补刀」，形成「起一个→忘了→下个会话盲杀旧 PID→再起一个」循环；
3. `scripts/start-dev.sh` 自带幂等防护（已在跑跳过 / `--restart` 按端口杀），但防护可被手搓命令绕过，且「杀 go run 父进程不杀编译产物子进程」「端口已释放但进程组仍存活」两类残留脚本无能为力；
4. harness 层对「会话结束后遗留的 dev 进程」零感知——事实库无任何相关事件。

## What Changes

- **新增 pi 扩展 `dev-process-guard`**（`.pi/extensions/dev-process-guard.ts` + `lib/dev-process-scan.ts`）：
  - `session_start` 记录会话窗口起点（按 sessionId 隔离，兼容子线程共享模块实例）；
  - `session_shutdown` 扫描仓库目录下**泄漏类 dev 进程**（判定标准：cmdline 命中 dev 服务特征 + cwd 在仓库内 + 无控制终端 + 非 pidfile 接管 + 非自身进程组），其中**启动时间落在本会话窗口内**的 → TERM 进程组 → 2s 宽限 → KILL，记 `policy.decision(orphan-killed)`；
  - `turn_end` 软提醒（每会话至多一次）：存在泄漏类进程（含窗口外的历史遗留）时 steer 列出 pid/pgid/年龄/命令行并给处理建议；
  - 仅 Linux（/proc）实现，其他平台 no-op；全程 fail-open，记账进 events.db 可考古；
  - 泄漏特征覆盖 dev 服务（go run / pnpm dev / nuxt dev 系）与 **agent-browser 浏览器自动化残留**（CLI 二进制 + 其无头 chromium 的 `--user-data-dir=/tmp/agent-browser-chrome-` 窄锚点；2026-09-17 实测残留一棵 chromium 树 ~1.3GB）——用户自起的 chromium 不带该标志永不命中。
- **`scripts/start-dev.sh` pidfile 契约**：起服务时写 `.pi/run/{backend,front}.pgid`（setsid 会话首进程 = PGID）；`stop` 合并「端口 PID + pidfile 进程组」双路清理，覆盖僵尸栈（端口已释放但进程组存活）与 go run 父进程残留；`status` 显示 pidfile 接管状态。pidfile 是 dev-process-guard 的白名单——经脚本起的服务会话结束不杀。
- **文档同步**：`docs/reference/harness/pi-extensions.md` 全景表加行 + 机制节；AGENTS.md 扩展计数与「dev 服务起停必须走 start-dev.sh」红线同步。

## Capabilities

### New Capabilities

- `dev-process-guard`：会话生命周期挂钩的孤儿 dev 进程治理——泄漏类判定、会话窗口归因、shutdown 自动清理、turn_end 软提醒、pidfile 白名单、记账口径。

### Modified Capabilities

- 无（`same-origin-deployment` 等既有 capability 不受影响；start-dev.sh 行为增强归入本 capability 的 pidfile 契约 Requirement）。

## Stakeholder Concerns

- **误杀风险（最高优先）**：自动清理只针对「无终端 + 窗口内 + 非 pidfile」三重条件交集；用户在自己终端里手起的服务（有控制终端）永不自动杀；经 start-dev.sh 起的（pidfile）永不杀。
- **多会话并行**：按 sessionId 隔离窗口，子线程（pi-subagents 共享模块实例发 session_start{startup}/session_shutdown）只清自己窗口内的spawn。
- **遗留存量**：本 change 落地前已存在的孤儿（如当前手搓后端）不在任何新会话窗口内，不会被自动杀——由 turn_end 提醒人工处置，避免「升级即杀用户在用的服务」。
