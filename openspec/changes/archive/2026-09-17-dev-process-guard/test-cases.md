# dev-process-guard 白盒测试用例

> change: dev-process-guard ｜ 用例先行制品（开发执行规范 §2，complex 档白盒用例）
> 依据：proposal.md + 设计决策（泄漏五条件 / cmdline 八模式 / 窗口归因 / shutdown 动作 / turn_end 动作 / fail-open / start-dev.sh pidfile 契约）。
> 断言从紧原则：每条断言可机械判真伪（脚本/注入器/字面值比对），禁「应该正常」类措辞。
> 用例总数：**83**（D 18 ＋ 模式表 28 ＋ B 14 ＋ SD 11 ＋ I 12；含 agent-browser 残留治理增量 P-08/09、N-15/16）。

---

## 0. 被测对象与共同前置

三层被测对象：

| 层 | 文件 | 内容 |
| --- | --- | --- |
| 纯函数层 | `.pi/extensions/lib/dev-process-scan.ts` | `isDevCmdline(cmdline)`、`joinCmdline(raw)`、`parseStatLine(line)`、`computeStarttimeSec(btime, ticks, clkTck)`、`isInsideRepo(cwd, repoRoot)`、`decide(procs, env)` |
| handler 层 | `.pi/extensions/dev-process-guard.ts` | `session_start` / `session_shutdown` / `turn_end` 三挂钩 |
| 脚本层 | `scripts/start-dev.sh` | pidfile 契约（写 / 双路 stop / 跳过不覆盖 / 删文件 / status） |

**可测性缝（实现必须提供，否则 I 节无法回放）**：

1. 纯函数层全部从模块 `export`，esbuild bundle 成 `.cjs` 后直接 `require` 断言（同 `run-harness-smoke.sh` 惯例：`npx -y esbuild ../lib/dev-process-scan.ts --bundle --platform=node --format=cjs --outfile=./.dps.cjs`）。
2. handler 层接受依赖注入对象 `{ readStat, readCmdline, readCwd, listProcDir, killGroup, now, platform, logEvent, consoleWarn }`（模块级默认接真实实现；测试经导出的注入点替换，或 handler 工厂函数传参）。扩展入口 bundle 成 `.dpg.cjs` 后用 fake `ExtensionAPI`（捕获 `pi.on` 注册的 handler）+ fake `ctx`（含 `cwd`、`sessionManager.getSessionId()`）驱动。
3. 「注入器」= 上述 DI 桩，记录调用序列供断言；`steer` 桩按实现实际使用的 steer API 打点。

**decide 签名与输出契约**：

```
decide(procs: ProcSnapshot[], env: {
  repoRoot: string;                 // 已规范化绝对路径
  window: { start: number } | undefined;  // 秒；session_start 记录，按 sessionId 隔离（Map 在 handler 层）
  pidfilePgids: Set<number>;        // 两份 pidfile 解析出的白名单
  ownPgid: number;
  now: number;                      // 秒
}) → { kills: ProcSnapshot[]; leaks: ProcSnapshot[] }
```

- `ProcSnapshot = { pid, pgid, cmdline /* NUL→空格 join 后 */, cwd, ttyNr, starttimeSec /* 秒 */ }`
- `kills ⊆ leaks`；`leaks` = 五条件全满足（与窗口无关，供 turn_end 提醒）；`kills` = `leaks` 中 `starttimeSec ∈ [window.start, now]`（闭区间；`window === undefined` → `kills = []`）。
- **组信号红线**：`kills` 中不得出现 `pgid ≤ 1` 的项（`kill(-1)` 会广播全系统，属灾难性误杀；此为杀名单额外保险，不改变 leaks 分类）。
- 路径规范化（尾斜杠、realpath）在 scan 层完成，`decide` 收到已规范化输入。

**D 节基线输入（未注明处每行沿用）**：

```
P0   = { pid:4242, pgid:4242, cmdline:"go run cmd/server/main.go",
         cwd:"/home/zanebono/software/Syntopica/backend-go", ttyNr:0, starttimeSec:1010 }
env  = { repoRoot:"/home/zanebono/software/Syntopica", window:{start:1000},
         pidfilePgids:new Set([5555,6666]), ownPgid:7777, now:2000 }
```

期望输出记法：`kills/leaks` 用 pid 集合表示，`∅` = 空集。

---

## 1. 决策函数分支表（decide 纯函数，D-01～D-18）

| 编号 | 输入条件组合（仅列与基线的差异） | 期望 kills | 期望 leaks | 覆盖点 |
| --- | --- | --- | --- | --- |
| D-01 | 基线（五条件全满足＋窗口内） | {4242} | {4242} | 主路径 |
| D-02 | ①证伪：`cmdline:"pnpm build"` | ∅ | ∅ | 条件①独立排除 |
| D-03 | ②证伪：`cwd:"/tmp/other"` | ∅ | ∅ | 条件②独立排除 |
| D-04 | ③证伪：`ttyNr:34817`（有控制终端） | ∅ | ∅ | 条件③独立排除 |
| D-05 | ④证伪：`pgid:5555`（∈ pidfilePgids） | ∅ | ∅ | 条件④独立排除 |
| D-06 | ⑤证伪：`pgid:7777`（== ownPgid） | ∅ | ∅ | 条件⑤独立排除 |
| D-07 | 窗口外：`starttimeSec:999`（< start） | ∅ | {4242} | 遗留只提醒不杀 |
| D-08 | 窗口闭边界：`starttimeSec:1000`（== start） | {4242} | {4242} | 区间左端含 |
| D-09 | 窗口闭边界：`starttimeSec:2000`（== now） | {4242} | {4242} | 区间右端含 |
| D-10 | 时钟跳变：`starttimeSec:2001`（> now） | ∅ | {4242} | 越界保守不杀 |
| D-11 | `window: undefined`（无 session_start 记录） | ∅ | {4242} | 无法归因不杀 |
| D-12 | 混合名单：A(pgid:5555 白名单)＋B(P0)＋C({pid:4300,pgid:4300,cmdline:"go run cmd/server/main.go",cwd:同基线,ttyNr:0,starttimeSec:999})＋D({pid:5170,pgid:5170,cmdline:"pnpm test:unit",余同 P0}) | {4242} | {4242,4300} | 名单合成正确 |
| D-13 | 空输入 `[]` | ∅ | ∅ | 空集无害 |
| D-14 | `pgid:1`（其余全满足＋窗口内） | ∅ | {4242} | 组信号红线（见 B-02） |
| D-15 | `cwd:"/home/zanebono/software/Syntopica"`（== repoRoot） | {4242} | {4242} | 判定②含仓库根本身 |
| D-16 | `cwd:"/home/zanebono/software/Syntopica-mirror"`（裸前缀相同、非子路径） | ∅ | ∅ | 前缀判定须带路径分隔符边界，禁裸 `startsWith` |
| D-17 | 同组双进程：P0 ＋ {pid:4243, pgid:4242, cmdline:"/home/z/.cache/go-build/36/3609f1e2…-d/main", cwd/ttyNr/starttimeSec 同 P0} | {4242,4243} | {4242,4243} | go run 父子同组都进名单（信号层按组去重，见 I-01） |
| D-18 | 确定性：同输入连调两次，输出 deep-equal | — | — | 无隐藏随机性 |

---

## 2. cmdline 模式分类表

> 2026-09-17 用户补充 agent-browser 残留治理：六模式扩为八模式（增 m7/m8），正反例增 P-08/09、N-15/16，总数 78→82。

给定八模式（命中任一即为条件①满足）：

```
m1  go run .*cmd/server/main\.go
m2  \.cache/go-build/.*/main$
m3  \bsh -c nuxt dev\b
m4  nuxt\.mjs dev\b
m5  @nuxt\+cli.*dev/index\.mjs
m6  \bpnpm dev\b
m7  \bagent-browser\b
m8  --user-data-dir=/tmp/agent-browser-chrome-
```

输入为 `/proc/<pid>/cmdline` 按 NUL→空格 join 后的整串（见 B-07）。

### 2.1 正例（实测进程形状，必须命中）

| 编号 | cmdline（join 后字面值） | 期望 | 命中模式 |
| --- | --- | --- | --- |
| P-01 | `go run cmd/server/main.go` | 命中 | m1 |
| P-02 | `/home/z/.cache/go-build/36/3609f1e2…-d/main` | 命中 | m2 |
| P-03 | `sh -c nuxt dev --host` | 命中 | m3 |
| P-04 | `node /home/z/software/Syntopica/front/node_modules/nuxt/bin/nuxt.mjs dev --host` | 命中 | m4 |
| P-05 | `node /home/z/software/Syntopica/front/node_modules/.pnpm/@nuxt+cli@3.32.0/node_modules/nuxi/dist/pages/dev/index.mjs --host` | 命中 | m5 |
| P-06 | `/home/z/software/Syntopica/front/node_modules/.pnpm/@pnpm+exe@10.15.0/node_modules/pnpm/bin/pnpm dev --host` | 命中 | m6 |
| P-07 | `go run -tags=embed cmd/server/main.go` | 命中 | m1（`.*` 跨越 flag 段） |
| P-08 | `/home/z/.nvm/versions/node/v26.8.2/lib/node_modules/agent-browser/bin/agent-browser-linux-arm64` | 命中 | m7（AI 浏览器自动化 CLI，2026-09-17 实测残留） |
| P-09 | `/usr/lib/chromium/chromium --headless=new --remote-debugging-port=0 --user-data-dir=/tmp/agent-browser-chrome-03421e1a-5ce2-4303-9dd2-146decaa6db3 --window-size=1280,720` | 命中 | m8（无头 chromium 临时 profile 窄锚点） |

### 2.2 反例（必须不命中任何模式）

| 编号 | cmdline（join 后字面值） | 不命中理由 / 邻界说明 |
| --- | --- | --- |
| N-01 | `pnpm generate` | 静态生成非 dev（2026-09-17 实测同屏进程） |
| N-02 | `pnpm test:unit` | 测试非 dev |
| N-03 | `pnpm build` | 构建非 dev |
| N-04 | `go build ./...` | 无 `go run` |
| N-05 | `next-server (v16.3.1)` | pi-web 的 Node 进程 title，六模式全不中 |
| N-06 | `chromium --type=renderer --field-trial-handle=1,x` | agent-browser 渲染器 |
| N-07 | `go run ./cmd/other/main.go` | `go run` 但非 `cmd/server/main.go` |
| N-08 | `npm run dev` | npm 非 pnpm，亦无 m3/m4 形状（易误伤补） |
| N-09 | `node …/nuxt/bin/nuxt.mjs build` | `nuxt.mjs` 后是 build 非 dev（易误伤补） |
| N-10 | `node …/nuxt/bin/nuxt.mjs generate` | 同上（易误伤补） |
| N-11 | `bash -c nuxt dev --host` | `bash` 非 `sh`，`\bsh` 不匹配 bash 内嵌；按给定模式**不命中**（已知盲区，标注不收紧） |
| N-12 | `go vet ./cmd/server/main.go` | 含目标路径但无 `go run`（易误伤补） |
| N-13 | `sh -c nuxt build` | `sh -c nuxt` 后是 build 非 dev（易误伤补） |
| N-14 | `/home/z/.cache/go-build/36/3609f1e2…-d/main --port 5100` | m2 的 `$` 锚定串尾，带参不命中；该子进程由父 `go run`（m1）命中后整组清除兜底 |
| N-15 | `/usr/lib/chromium/chromium --headless=new --window-size=1280,720`（用户自起，无 agent-browser profile 标志） | m7/m8 均不中；有终端则条件③也拦（用户自起的 chromium 不带 `--user-data-dir=/tmp/agent-browser-chrome-`） |
| N-16 | `/usr/lib/chromium/chrome_crashpad_handler --monitor-self --database=/home/z/.config/chromium/Crash Reports …` | 八模式全不中（已知盲区：crashpad 不命中；其 client 死后自行退出，且同 session 的 chromium 组杀已兑底，标注不收紧） |

### 2.3 给定模式下的已知邻界命中（回归锚点，断言「命中」，不作为反例）

| 编号 | cmdline | 期望 | 说明 |
| --- | --- | --- | --- |
| K-01 | `pnpm dev:ssg` | 命中 | `:` 非词字符，`\b` 在 `dev` 后成立（m6） |
| K-02 | `pnpm dev-remote` | 命中 | `-` 同上（m6） |
| K-03 | `grep -rn nuxt.mjs dev front/app` | 命中 | 串含 `nuxt.mjs dev`（m4）；扫描型短命进程，靠条件②③⑤＋turn_end 时已退出口径兜底 |

> K 行是给定正则语义的忠实记录（防实现「顺手收紧/放宽」造成静漂移）；若产品要排除 K 类形状，须另开 change 改模式，本表随之更新。

---

## 3. 边界值清单（B-01～B-14）

| 编号 | 前置 | 动作 | 断言 |
| --- | --- | --- | --- |
| B-01 | — | 窗口边界三连（登记行，详见 D-07/D-08/D-09/D-10） | `starttimeSec == start` 与 `== now` 均进 kills；`start−1` 与 `> now` 均只进 leaks |
| B-02 | D-14 场景接入 handler 注入器 | 触发 session_shutdown | kill 注入器全部调用记录中 `pgid ≤ 1` 出现次数 == 0（贯穿 I 节所有场景的公共断言）；kills 为空 |
| B-03 | P0 变体 `{pid:7777 /*==ownPid*/, pgid:4242, 余基线}` | 调 decide | kills == {7777}（条件⑤按 **PGID** 判，禁止误用 pid 比对）；反向（pgid==ownPgid）即 D-06 |
| B-04 | 快照 [P1,P2]（均为基线形状），`readStat(P1)` 返回 null（ENOENT） | 触发 session_shutdown | P1 被跳过（不在 kills/leaks/事件 procs），P2 照常进 kills 并被杀；handler 不抛异常 |
| B-05 | `/proc/stat` 注入为无 `btime` 行 | 触发 session_shutdown | kills == ∅（无法归因保守不杀）、leaks 照常、turn_end 提醒照常、无异常抛出 |
| B-06 | 纯函数直调 | `computeStarttimeSec(1000,1280,128)` 与 `computeStarttimeSec(1000,1280,100)` | 前者 == 1010，后者 == 1012.8，两值不等（证明用了注入 CLK_TCK 而非硬编码 100） |
| B-07 | 原始 cmdline buffer：`"sh\0-c\0nuxt dev --host\0"` | joinCmdline → isDevCmdline | join 结果 == `"sh -c nuxt dev --host"` 且命中 m3（若实现按 NUL 截断只取 argv[0]="sh" 则不命中——本用例锚定完整 join） |
| B-08 | 合成 stat 行①：``4242 (next-server (v1.2)) S 1 4242 4242 34817 -1 4146 100 0 2 0 5 3 0 0 80 0 12 0 7654321 123456 4567``；行②：``9000 (sh -c nuxt dev) S 8999 9001 9001 0 -1 4146 10 0 0 0 1 1 0 0 80 0 1 0 100 4096 100`` | parseStatLine 两行 | 行① → `{pid:4242, pgrp:4242, ttyNr:34817, starttimeTicks:7654321, state:"S"}`；行② → `{pid:9000, pgrp:9001, ttyNr:0, starttimeTicks:100}`（comm 含空格/内层右括号不串位；解析必须取**最后一个 `)`** 之后的字段段） |
| B-09 | pidfile 内容分别为 `"abc\n"`、空文件、`"123\n456\n"`（多行） | 解析白名单 | 三种均不抛异常、不贡献任何白名单成员；随后 pgid=123 的基线形状进程正常判为泄漏（损坏文件不得污染判定） |
| B-10 | pidfile 内容 `"999999"`（无此进程组） | decide（pidfilePgids={999999}）＋ P0 | P0 正常进 kills（死组白名单值无副作用、无异常） |
| B-11 | 注入 killGroup 记录器＋存活谓词「TERM 后即死」；快照 = 窗口内单泄漏 | 触发 session_shutdown | kill 注入器序列 == `[{pgid:4242, sig:"TERM"}]`（无 KILL 项）；事件照常落（decision=="orphan-killed" 且 procs 含该 pid） |
| B-12 | 注入 killGroup 记录器＋存活谓词「始终存活」；同上快照 | 触发 session_shutdown | 序列 == `[{pgid:4242,sig:"TERM"},{pgid:4242,sig:"KILL"}]`（同 pgid）；两次调用时间差 ∈ [1.8s, 2.6s]（宽限恰 ~2s，含轮询粒度容差）；真实进程版（`setsid bash -c 'trap "" TERM; while :; do sleep 0.5; done'`）组终态为死 |
| B-13 | 快照 [P1,P2]，`readCwd(P1)` 抛 EACCES（他人进程） | 触发 session_shutdown | P1 被保守排除（条件②无法证实→不进名单），P2 照常被杀；无异常抛出 |
| B-14 | P1 为 `state:"Z"` 僵尸，`readCmdline(P1)` 返回空（joinCmdline==""） | 扫描＋decide | 空串不命中任何模式（isDevCmdline("") == false），P1 排除；无异常 |

---

## 4. start-dev.sh 契约用例（SD-01～SD-10）

共同前置：工作目录 = 仓库根；`.pi/run/` 初始状态按各用例注明；端口 5100/3000 初始无监听（有则先 `stop` 清场）。

| 编号 | 前置 | 动作 | 断言（可机械执行） |
| --- | --- | --- | --- |
| SD-01 | `.pi/run/` 不存在；:5100 空闲 | `bash scripts/start-dev.sh back`（等待就绪输出） | ① `.pi/run/backend.pgid` 存在且内容为纯数字（`grep -qE '^[0-9]+$'`）；② `P=$(cat .pi/run/backend.pgid)` 后 `ps -o pgid= -p "$P" \| tr -d ' '` == `"$P"`（setsid 会话首：PID==PGID）；③ `ps -eo pgid=,args= \| awk -v g="$P" '$1==g'` 同时含 `go run cmd/server/main.go`（父）与 `.cache/go-build` 产物行（子，/health 就绪后必在组内）。慢用例（后端健康等待 ~60s 级） |
| SD-02 | `.pi/run/` 不存在；:3000 空闲 | `bash scripts/start-dev.sh front` | ① `.pi/run/front.pgid` 纯数字且 PID==PGID；② 组内进程列表含 `pnpm dev` 或 `nuxt` 形状行（`ps -eo pgid=,args= \| awk -v g="$P" '$1==g' \| grep -E 'pnpm dev\|nuxt'` 非空）。慢用例（Nuxt 冷启动 60~90s） |
| SD-03 | 预置 `.pi/run/backend.pgid` 内容 `654321\n`；:5100 被 `python3 -m http.server 5100 >/dev/null 2>&1 &` 占住 | `bash scripts/start-dev.sh back` | ① stdout 含「跳过」字样；② `printf '654321\n' \| cmp - .pi/run/backend.pgid` 退出码 0（跳过时**不覆盖**既有 pidfile）。收尾 kill 掉 python 进程 |
| SD-04 | `setsid sleep 300 & P=$!`；`echo "$P" > .pi/run/front.pgid`；:3000/:5100 无监听（纯僵尸栈：端口空转但组活） | `bash scripts/start-dev.sh stop` | ① `kill -0 "$P" 2>/dev/null` 失败（进程死）；② `kill -0 "-$P" 2>/dev/null` 失败（组不存在）；③ `[ ! -e .pi/run/front.pgid ]`（stop 成功删 pidfile）；④ 脚本退出码 0 |
| SD-05 | `setsid bash -c 'trap "" TERM; while :; do sleep 0.5; done' & P=$!`；`echo "$P" > .pi/run/front.pgid`；端口无监听 | `bash scripts/start-dev.sh stop` | ① `kill -0 "-$P" 2>/dev/null` 失败（TERM 被忽略后升级 KILL 清组）；② pidfile 已删；③ stop 总耗时 ≤ 20s（`time` 包裹，防等待无界挂死）；④ 退出码 0 |
| SD-06 | `python3 -m http.server 3000 --bind 127.0.0.1 >/dev/null 2>&1 & HTTP=$!`（占端口、不在 pidfile 组内）；`setsid sleep 300 & P=$!`，`echo "$P" > .pi/run/front.pgid`（占 pidfile、不监听端口）——两受害者互不覆盖 | `bash scripts/start-dev.sh stop` | ① `kill -0 "$HTTP"` 失败（端口 PID 路被清）；② `kill -0 "-$P"` 失败（pidfile PGID 路被清）；③ `[ ! -e .pi/run/front.pgid ]`。证明 stop = 端口 PID ∪ pidfile PGID 双路合并 |
| SD-07 | 无监听、`.pi/run/*.pgid` 均不存在 | `bash scripts/start-dev.sh stop` | ① 退出码 0；② stdout 含「本来就没在跑」（既有文案保持）；③ `.pi/run/` 下无新建 pgid 文件 |
| SD-08 | `echo not-a-number > .pi/run/front.pgid`；端口无监听 | `bash scripts/start-dev.sh stop` | ① 退出码 0；② stderr 无 bash 报错（`grep -cE 'integer expression\|syntax error' /tmp/stderr` == 0，损坏 pidfile fail-open 不炸脚本）；③ pidfile 文件去留不做断言（实现自定），但同状态二次 stop 仍退出码 0（幂等） |
| SD-09 | SD-01 已完成后 `P1=$(cat .pi/run/backend.pgid)` | `bash scripts/start-dev.sh --restart back`（等待就绪）后 `P2=$(cat .pi/run/backend.pgid)` | ① `P1 != P2`；② `kill -0 "-$P1"` 失败（旧组被清）；③ `kill -0 "-$P2"` 成功且新组健康（`curl -sf http://127.0.0.1:5100/health`）；④ pidfile 内容 == P2。很慢（两轮后端就绪），归档门禁前跑一次即可 |
| SD-10 | `setsid sleep 300 & P=$!`；`echo "$P" > .pi/run/front.pgid` | ① `bash scripts/start-dev.sh status`；② `rm .pi/run/front.pgid` 后再 `bash scripts/start-dev.sh status`；③ 收尾 `kill -- -"$P"` | ① 输出命中 `grep -E 'pidfile\|接管'` 且含 P 的数值（`grep -qE "(^\|[^0-9])$P([^0-9]\|$"`）；② 输出不再含该数值。status 必须反映接管状态 |
| SD-11 | `setsid bash -c 'trap "" TERM; exec python3 -m http.server <假端口>'`（TERM 免疫，模拟后端优雅关停偏慢：Registry.StopAll 上限 30s+，10s 观察窗内不让端口） | `bash scripts/start-dev.sh stop`（副本假端口） | ① 副本 TERM 后仍在监听时输出含「kill -9 兜底」并升级；② 端口释放（lsof 空）；③ 进程组死；④ 退出码 0 |

---

## 5. 集成场景（handler 级，esbuild bundle 回放，I-01～I-12）

共同前置（I-00，不计入用例数）：按 §0 可测性缝 bundle `.pi/extensions/dev-process-guard.ts` → `.dpg.cjs`；fake `ExtensionAPI` 捕获 `session_start` / `session_shutdown` / `turn_end` 三个 handler；fake `ctx` 提供 `cwd:"/home/zanebono/software/Syntopica"` 与 `sessionManager.getSessionId()`（返回注入值）；`now` 注入固定时钟；kill/logEvent/steer 注入器记录调用序列。若实现复用 `logPolicyDecision`，注意其 `action` 白名单（block\|warn\|bypass\|fail-open）与 `reasonCode` kebab-case 校验——`decision:"orphan-killed"` 字段的落点须兼容该收敛（实现注意项，非断言）。

| 编号 | 前置 | 动作 | 断言 |
| --- | --- | --- | --- |
| I-01 | sessionId="s1"；注入快照 = D-17 同组双进程（4242 `go run cmd/server/main.go` ＋ 4243 `.cache/go-build …/main`，pgid 均 4242，cwd=repo/backend-go，ttyNr=0，starttimeSec=1010 ∈ 窗口 [1000,2000]）；killGroup 记录器＋存活谓词「TERM 后死」 | 依次触发 `session_start`（now=1000）→ `session_shutdown`（now=2000） | ① kill 注入器恰调用 1 次（**按组去重**，同组双进程不重复发信号），参数 `{pgid:4242, sig:"TERM"}`；② logEvent 注入器恰 1 条：`type=="policy.decision"`、`payload.decision=="orphan-killed"`、`payload.procs` 含 pid 4242 与 4243 两项，每项含 `pgid==4242` 与非空 `cmdline` |
| I-02 | 同 I-01 快照，但两进程 `starttimeSec:999`（窗口外遗留） | 触发 `session_start` → `session_shutdown` | ① kill 注入器 0 次；② logEvent 注入器 0 条（无 orphan-killed 事件）；③ handler 正常返回不抛异常 |
| I-03 | sessionId="s1"；快照 = 窗口外遗留 1 个（pid 4242, pgid 4242, `go run cmd/server/main.go`, starttimeSec=999）；窗口内 0 个 | 触发 `turn_end`（now=2000） | ① steer 注入器恰 1 次；② 消息文本含 `"4242"`（pid）与 pgid 数值字面值、cmdline 可识别子串 `go run`、命中 `\d+[hms]` 的年龄信息、含字面 `start-dev.sh`（建议文案） |
| I-04 | I-03 之后同一 sessionId="s1"，快照不变（泄漏仍在） | 再次触发 `turn_end` | steer 注入器累计仍为 1 次（每会话至多一次，二次静默） |
| I-05 | sessionId="s1"；空快照（无任何 /proc 条目） | 触发 `turn_end` | steer 注入器 0 次、logEvent 注入器 0 条（零泄漏→零输出零记录） |
| I-06a | 快照 [P_a, P_b]（均为窗口内泄漏形状）；`readStat(P_a)` 抛 EACCES | 触发 `session_shutdown` | P_b 被 kill 且进事件 procs；P_a 不在 kills/leaks/事件中；无异常抛出 |
| I-06b | 同 I-01 快照；logEvent 注入器抛错（模拟事件库写失败） | 触发 `session_shutdown` | kill 注入器仍恰 1 次 TERM（记账失败不阻断清理）；handler 不抛异常（fail-open） |
| I-06c | 快照 [P1,P2]（均窗口内泄漏）；读取 P1 详情时 `readStat(P1)` 返回 null（扫描中途退出） | 触发 `session_shutdown` | P2 照常被杀并进事件；P1 被跳过；无异常 |
| I-06d | 快照两个不同组泄漏（pgid 4242 / 5151）；killGroup(4242,\*) 抛 EPERM | 触发 `session_shutdown` | pgid 4242 的调用被跳过（同 pgid 注入器调用 ≤ 2 次，无重试风暴）；pgid 5151 照常 TERM；handler 不抛异常 |
| I-07 | `platform` 注入 `"darwin"`；consoleWarn 记录器 | 依次触发 `session_start` → `turn_end` → `session_shutdown`（快照含窗口内泄漏形状） | ① consoleWarn 恰 1 次（全程仅一次，非每事件一次）；② kill 注入器 0 次；③ logEvent 0 条；④ steer 0 次（非 Linux 整体 no-op） |
| I-08 | 同一模块实例双 sessionId："s1" start=1000、"s2" start=1500；P1(starttime=1100, pgid=4242)、P2(starttime=1600, pgid=5151) 均五条件满足；now=2000 | 触发 s2 的 `session_shutdown` → 再触发 s1 的 `session_shutdown` | ① 第一次 kill 恰 1 次且 pgid==5151（只杀自己窗口内，不碰 P1）；② 第二次 kill 恰 1 次且 pgid==4242；③ 两次事件各自落库、procs 各只含本窗口进程（子线程共享模块实例按 sessionId 隔离） |
| I-09 | sessionId="s1" 但**不发** `session_start`（模拟扩展热加载后直接 shutdown）；快照含窗口内形状泄漏 | 直接触发 `session_shutdown` | kill 注入器 0 次（无窗口记录→无法归因→保守不杀）；无 orphan-killed 事件；不抛异常 |

---

## 6. 覆盖对照与总数

| 设计决策条款 | 用例编号 |
| --- | --- |
| 五条件各自独立证伪 | D-02～D-06 |
| cmdline 八模式正/反例 | P-01～09、N-01～16、K-01～03 |
| 窗口归因（闭区间/时钟跳变/无记录） | D-07～D-11、B-01、I-09 |
| kills ⊆ leaks、混合名单、空集、确定性 | D-01、D-12、D-13、D-18 |
| PGID==1 组信号红线 | D-14、B-02 |
| cwd 判定（含根/前缀攻击/pid-pgid 维度） | D-03、D-15、D-16、B-03、B-13 |
| 同组双进程（go run 父子） | D-17、I-01 |
| TERM→2s→KILL 状态机 | B-11、B-12、SD-05 |
| session_shutdown 只杀窗口内＋事件落库 | I-01、I-02、I-08 |
| turn_end 每会话至多一次＋零泄漏零记录 | I-03、I-04、I-05 |
| fail-open（EACCES/ENOENT/写库失败/EPERM/btime 失败/僵尸/空 cmdline/NUL） | I-06a～d、B-04、B-05、B-07、B-13、B-14 |
| 非 Linux no-op＋warn 一次 | I-07 |
| pidfile 契约（写/会话首/双路 stop/僵尸栈/不覆盖/删除/status/幂等/restart） | SD-01～SD-10、B-09、B-10 |

**用例总数：83**（D 18 ＋ 模式表 28 ＋ B 14 ＋ SD 11 ＋ I 12）。
