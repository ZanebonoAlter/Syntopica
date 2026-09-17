# Tasks — dev-process-guard

## 1. lib：进程扫描与决策纯函数

- [x] 1.1 新增 `.pi/extensions/lib/dev-process-scan.ts`：`classifyCmdline`（对齐 test-cases `isDevCmdline`）按八模式分类（go run 后端 / go-build 编译产物 / sh -c nuxt dev / nuxt.mjs dev / @nuxt+cli dev worker / pnpm dev / agent-browser CLI / agent-browser 无头 chromium user-data-dir 窄锚点），正反例按 test-cases.md 分类表（含 P-08/09、N-15/16）
- [x] 1.2 `parseProcStat(stat, btimeSec, clkTck)`：comm 取最后一个 `)` 之后解析（兼容含空格/右括号 comm），返回 {ppid, pgid, ttyNr, startEpochSec}；字段损坏返回 null
- [x] 1.3 `decide(procs, {repoRoot, pidfilePgids, selfPgid, windowStartedAt, nowSec})` → `{kill: Proc[], warn: Proc[]}`：D3 五条件合取 + 窗口归因（窗口内→kill、窗口外→warn），纯函数可注入 fixture
- [x] 1.4 IO 包装 `scanLeakCandidates(repoRoot)`：readdir /proc 数字目录 → stat/cmdline/cwd 读取，单条目失败跳过；`readPidfilePgids()` 读 `.pi/run/*.pgid`（损坏/死进程→忽略）
- [x] 1.5 `killGroup(pgid)`：TERM 组 → 150ms 间隔轮询 2s → 仍存活 KILL 组；pgid===1 或===自身组拒绝执行

## 2. 扩展：dev-process-guard.ts

- [x] 2.1 `session_start`：Map<sessionId, startedAt> 记窗口起点（reason==="startup" 不清 Map，子线程共享实例防御对齐 quality-gate）；非 Linux 平台一次性 console.warn 后整体 no-op
- [x] 2.2 `session_shutdown`：scanLeakCandidates → decide → kill 名单逐组 killGroup（fail-open 逐目标 try/catch）→ `logEvent(policy.decision, {decision:"orphan-killed", procs, window})`；写入失败仅 console.error
- [x] 2.3 `turn_end`：存在泄漏类（含窗口外 warn 名单）且本会话未提醒过 → steer 列 pid/pgid/年龄/命令行 + 区分窗口内外 + 建议（start-dev.sh 接管或手动清）+ `logEvent(policy.decision, {decision:"orphan-warn"})`；零泄漏零输出零记账；`DEV_GUARD_ENABLE=0/off/false` 关闭
- [x] 2.4 冒烟 `.pi/extensions/tests/dev-process-guard.smoke.cjs`：esbuild bundle → classify/parse/decide 白盒（test-cases.md 分支表全量回放）+ handler 级行为（kill 动作注入 stub：shutdown 命中/窗口外不杀/自我防护/fail-open/turn_end 首次提醒→二次静默→零泄漏零记录）；挂入 `tests/run-harness-smoke.sh`

## 3. start-dev.sh pidfile 契约

- [x] 3.1 `start_back`/`start_front`：`nohup setsid … &` 后 `echo $! > "$REPO_ROOT/.pi/run/<name>.pgid"`（mkdir -p；is_up 跳过路径不动 pidfile）
- [x] 3.2 `stop_port` 增强：清理目标 = 端口 PID ∪ pidfile PGID 组；端口未监听但 pidfile 组存活（僵尸栈）也执行组清；TERM→等→KILL；完成后删 pidfile；pidfile 缺失/损坏按无主删除降级（旧端口路径行为不变）
- [x] 3.3 `show_status` 显示 pidfile 接管状态（存在/缺失 + 指向进程存活与否）
- [x] 3.4 冒烟 `scripts/start-dev.smoke.sh`：setsid sleep 假进程组验证——起服务写 pgid / stop 组清+删文件 / 僵尸栈（无端口占位）组清 / pidfile 损坏降级不报错
- [x] 3.5 stop_port 端口 PID 的 KILL 升级：TERM 10s 仍在监听 → `kill -9` 兜底（2026-09-17 实战：后端优雅关停 ≈50s 致 `--restart back` 空转）+ 冒烟用例 E（TERM 免疫端口进程）

## 4. 文档同步

- [x] 4.1 `docs/reference/harness/pi-extensions.md`：全景表加 dev-process-guard 行（挂点/触发/软硬/fail 策略/记账）+ 机制小节（判定五条件、窗口归因、pidfile 白名单协议、平台边界）
- [x] 4.2 `AGENTS.md`：扩展计数 9→10 + harness 配合要点补「dev 服务起停走 start-dev.sh，手搓 nohup 会被会话结束自动清」

## 5. 测试

- 影响测试命令（本 change 不触 Go/前端代码，无 go test / pnpm test:unit）：
  - `bash .pi/extensions/tests/run-harness-smoke.sh`（含新增 dev-process-guard.smoke.cjs）→ 全绿
  - `bash scripts/start-dev.smoke.sh` → 全绿

## 6. 文档

<!-- doc-impact: none(harness 工具链 change：仅更新 docs/reference/harness/pi-extensions.md 与 AGENTS.md，均不在 8 域路径内；无 flow 业务域变更) -->
- [x] docs/reference/harness/pi-extensions.md（全景表 + 机制节，见 4.1）
- [x] AGENTS.md（扩展计数与红线，见 4.2）
- [x] openspec 主 spec 同步：archive 时 `specs/dev-process-guard/spec.md` 落 `openspec/specs/dev-process-guard/`（归档自动）
- [x] §12.2 变更溯源：归档后在 flow 注册（harness 类 change 无业务 flow 域，溯源链接落 harness/pi-extensions.md 文末变更记录，如无该节则在 §12.3 校验范围外说明）

## 7. 验证

- [x] `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，新增用例全绿
- [x] `bash scripts/start-dev.smoke.sh` → 退出码 0
- [x] `bash scripts/start-dev.sh status` → 正常输出，pidfile 状态行出现
- [x] 实战验收（一次性，留痕到本节）：起一个 `nohup setsid pnpm dev --host &` 于窗口内 → 结束本 pi 会话 → 进程消失且 events.db 有 orphan-killed
  - 留痕（2026-09-17T23:21 实测）：无头 pi 会话 01a0aff4-9bca-7143-8871-5af9e5c411b0 窗口内起 `/tmp/fakebin/pnpm dev --host`（ageSec=6）→ 会话结束自动整组 TERM，进程消失；events.db 落 `policy.decision(decision=orphan-killed, procs=[{pid:316447,pgid:316447,cmdline:"/bin/sh /tmp/fakebin/pnpm dev --host"}], windowStartedAt:1789658457.321)`。提醒路径由用户会话自然验证（窗口外遗留只警告不杀）。
- [x] `bash scripts/doc-impact.sh verify dev-process-guard` → PASS
- [x] `bash scripts/check-standards.sh --change dev-process-guard` → A-D/F/G 零失败

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 五条件全中判为泄漏 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 任一条件不满足即排除 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 邻界进程不误伤 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 浏览器自动化残留命中 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 用户自起 chromium 不误伤 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| pidfile 白名单损坏时 fail-safe | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 窗口内泄漏进程被整组清理 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 窗口外遗留不被自动杀 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 自我防护 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 扫描中进程消失不炸 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 非 Linux 平台整体 no-op | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 首次提醒后静默 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 零泄漏零输出 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| 起服务写 pidfile | scripts/start-dev.smoke.sh |
| 已在跑跳过不覆盖 | scripts/start-dev.smoke.sh |
| 僵尸栈清理（端口已释放、进程组存活） | scripts/start-dev.smoke.sh |
| stop 常规清理 | scripts/start-dev.smoke.sh |
| pidfile 缺失降级 | scripts/start-dev.smoke.sh |
| 事件可考古 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
