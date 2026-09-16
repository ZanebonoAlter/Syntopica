# 测试用例（linux-native-dev-environment）

> 单元 = Requirement 的用户故事；spec Scenario 是断言片段，本文档串成完整故事。
> 复杂度声明：`complex`（proposal.md 头部）→ 白盒附加节为强义务。
> 层选择：门禁逻辑 = 扩展行为 smoke（mock `pi.exec`，不真跑工具链）；Docker 环境 = 人工留痕 + 集成测试；文档 = grep 一致性校验。

## 1. 主链路（故事节拍）

**故事 A：Linux 宿主上改代码，门禁真实生效**（来源：`gate-interop-health` MODIFIED 三个 Requirement）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| A1 | 会话启动 | — | 平台缓存清空；首个需要门禁的回合开始判定 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| A2 | 改一行 `backend-go/**/*.go`，触发 turn_end | 无 cmd.exe 时门禁走本机工具链执行 | 探测返回 ENOENT → 判定 native；**不**发环境故障 steer、**不**记 `policy.decision(interop-down)` | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| A3 | 同上回合 | 无 cmd.exe 时门禁走本机工具链执行 | 四条后端门禁（golangci-lint / go vet / go build / domain go test）经 `pi.exec(exe, args, {cwd})` 真实执行，各记 `gate.check` | 行为 smoke（断言命令构造）+ 人工（真实门禁一轮） | `quality-gate.behavior.smoke.cjs`；人工：tasks 6.6 |
| A4 | 改一行 `front/` 非 .md 文件 | 前端改动行为不变 | 仅执行 `pnpm exec eslint`，**不**执行 typecheck/build/test:unit | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| A5 | 令 A3 的门禁失败（植入编译错误） | native 模式失败提示标注本机链路 | steer 标注为本机原生执行，无 `wsl环境` 字样、无「重启 WSL」建议 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| A6 | 下一回合不碰代码 | 真实代码失败保持既有语义 | 该命令因粘性重跑（native 下失败一律按代码问题处理） | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| A7 | 构造 native 下输出含 `UtilAcceptVsock` 的失败 | native 模式失败一律按代码问题归因 | 仍按真实代码失败处理，**不**标环境故障 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |

**故事 B：Windows/WSL 宿主行为不回归**（来源：同一组 MODIFIED Requirement，反向保护）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| B1 | 探测返回 exit 0（模拟 cmd.exe 健康） | 探测成功不改变既有门禁行为 | 全部门禁经 `cmd.exe /C "cd /d <winPath> && <cmd>"` 执行，命令文本与改造前逐字一致 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| B2 | 探测超时 / exit≠0 | 探测失败短路整轮 cmd 链路门禁 | 门禁零执行，记一条 `policy.decision(interop-down)`，被跳过命令零 `gate.check` | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| B3 | 探测调用抛非 ENOENT 异常 | 探测调用异常也 fail-open | 同上短路，不抛错、不阻断回合 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| B4 | cmd 链路下失败 diag 含 interop 特征 | 环境故障失败不触发粘性重跑 | 不进粘性、下回合不重跑、归因环境故障 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |
| B5 | cmd 链路下失败（含 `wsl环境` 标注） | 后端门禁失败提示含链路标注 | steer 保留 `（wsl环境）` 标注 | 行为 smoke | `quality-gate.behavior.smoke.cjs` |

**故事 C：集成测试能跑且不留垃圾**（来源：`dev-image-reachability`）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| C1 | 按原始镜像名 `docker pull pgvector/pgvector:pg18-trixie` | 原始镜像名经加速来源拉取成功 | 成功，且 `git status` 零改动 | 人工 | tasks 1.2 |
| C2 | 同上拉 `testcontainers/ryuk:0.13.0` | 按清单可补齐缺失镜像 | 成功，`docker images` 出现无前缀 tag | 人工 | tasks 1.2 |
| C3 | 跑一个 DB 集成测试 | 补齐 Ryuk 后泄漏停止 | 测试 PASS，运行前后 `docker ps -aq \| wc -l` 数量一致 | testcontainer PG | `backend-go/internal/reader/service/content_form_pg_test.go` |
| C4 | 泄漏容器清理 | 泄漏容器清理不误伤生产库 | 随机名测试容器删净；`syntopica-postgres` 仍 healthy；`./data` 未动 | 人工 | tasks 1.3 |
| C5 | 仓库内检索加速站域名 | 仓库内不出现加速站硬编码 | 零命中（排除 docs/research 快照） | 人工（grep） | tasks 6.3 |

**故事 D：文档口径不再误导后续会话**（来源：`change-scope-gate` / `scenario-trace-gate` / `dev-api-networking` MODIFIED）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| D1 | 在 Linux 跑 `bash scripts/change-scope.sh --json` | quality-gate 消费 / 平台标注随宿主变化 | JSON 可解析，`testTargets` 含 `tier=domain` 项，`notices` 的平台标注按宿主表述 | 人工（脚本输出核对） | tasks 6.2 |
| D2 | 检索 `docs/reference/` dev 网络口径 | 文档口径一致 | dev 口径为「后端 5100 / dev server 绑 0.0.0.0 / 绝对 base 直连」，无过时指引 | 人工（grep） | tasks 6.2 |
| D3 | 检索 localhost 探测卫生约定所在段落 | 探测卫生约定平台中立 | 以「存在系统代理时」为条件表述，未绑定特定跨系统环境 | 人工（grep） | tasks 6.2 |
| D4 | 归档前跑对账脚本 | 映射齐全通过 等 5 个 Scenario | `bash scripts/scenario-trace.sh <change>` 退出码 0 | 脚本烟测 | `scripts/scenario-trace.smoke.sh` |
| D5 | 前端 dev server 起在 `0.0.0.0` | WSL 可达前端 dev server | 从 `127.0.0.1:3000` 与局域网 IP 均可访问 | 人工 | tasks 6.6 |

## 2. 变体走查（五组固定清单，逐项给答案）

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空串 / 纯空白 | 不适用：门禁输入是 git 路径集合与命令输出，无自由文本入参 |
| 输入 | 纯分隔符 / 单 token / 特殊字符 | 不适用（同上）；命令构造参数固定，无用户拼接 |
| 输入 | 大小写 | 不适用：无大小写敏感的名称匹配（路径匹配用正则，已由既有 trigger-set 覆盖） |
| 输入 | 超长 | 已覆盖：命令输出走 `tail(output, 30)` 与 `truncateDiagGate`，既有逻辑不改 |
| 前置 | 空集（触发集为空） | 已有节拍：纯对话回合零门禁、零记账（`change-scope-gate` 既有 scenario「会话前残留改动不触发首轮门禁」） |
| 前置 | 单元素 | 覆盖：仅后端改动 → 只跑后端侧；仅前端改动 → 只跑 eslint |
| 前置 | 重复 | 覆盖：粘性失败跨回合重复执行同一命令（故事 A6） |
| 前置 | 越界引用 | 不适用：无外部资源 ID 引用 |
| 前置 | 部分满足 | 覆盖：一条命令失败、其余成功 → 仅失败项进粘性（既有 gateLog 语义，故事 A5+A6） |
| 时间窗口 | 边界两端 | 不适用：本 change 无时间窗口逻辑 |
| 时间窗口 | 空窗口 / 跨窗口 / 归一化 | 不适用（同上） |
| 幂等 | 重复执行 | 覆盖：多回合连续改动，每回合只按增量触发；平台判定缓存不重复探测（白盒 T3） |
| 幂等 | 部分失败重试 | 覆盖：粘性重跑只针对失败命令，成功命令不重复执行 |
| 幂等 | 并发 | 不适用：门禁为单会话串行 `turn_end`；`domain` 测试串行、vet/build 并行是既有设计 |
| 可用性 | 误输入反馈 | 不适用（非 UI change） |
| 可用性 | 空态 / 错误态 / 加载态 | 部分适用：错误态 = 门禁失败 steer（故事 A5/B4）；空态 = 触发集空（上面「空集」行） |
| 可用性 | 超长文本 / 重复提交 | 不适用（非 UI） |

## 3. 效果核对

| 项 | 触发原因 | 方法 | 量化结果 | 结论 |
|---|---|---|---|---|
| Linux 上门禁是否真的执行 | 改造前每轮 `cmd.exe` 探测必失败 → 短路 | 真实环境端到端脚本（真实 child_process + 真实文件系统，**非 mock**）跑一次 turn_end，断言判定/命令构造/cwd/记账/文案 | 已实测 9/9 全过：零 cmd.exe 调用、命令经 `bash -c` 且 cwd=backend-go、本机 golangci-lint 真实执行、`gate.check` 落库、零 `interop-down`、失败 steer 标 `[中间态][golangci-lint (本机)]` 且不含 wsl环境；lint 失败短路哨兵正确跳过 vet/build | 门禁恢复实际覆盖（脚本存档 `evidence/verify-real-gate.cjs`） |
| 平台判定在真实宿主的输出 | 判定靠 PATH 找 `cmd.exe`，非 `process.platform` | 真实环境跑判定函数 | 已实测：`process.platform=linux` → `cmdExeReachable()=false` → native 模式；本机 go/golangci-lint/pnpm 路径均可达 | 判定符合预期（脚本存档 `evidence/check-platform.cjs`） |
| 门禁命令在树莓派上的耗时 | 门禁恢复后每轮会真跑，需确认可接受 | 直接计时三条命令 | 已实测：golangci-lint 6.8s / go vet 9.4s / go build 4.2s（vet/build 并行） | 单轮增量门禁约 10~15s，可接受 |
| 会话级真实触发（非脚本） | 需确认 pi 加载新扩展后行为与脚本一致 | reload 后改一个后端 `.go`（testutil.go 注释）触发真实 turn_end，查账本 | 已实测：`gate.check` golangci-lint ok ms=2161 / go vet ok ms=7364 / go build ok ms=6958（均 flip 锚点）；本会话零 `interop-down`；另一 session（旧代码）仍有 interop-down——对比证明改造生效 | 会话级验证通过（tasks 6.6） |
| Docker 加速站是否让原始镜像名可用 | Docker Hub 直连不通 | 配 daemon.json 后按原始名拉取 | 已实测：`Registry Mirrors` 三条生效；`docker pull hello-world`（无前缀）成功 | 机器级配置足够，仓库零改动（tasks 1.1/1.2） |
| 后端全量测试能否作为本 change 的归档证据 | 并发 change 共享工作树（coordinate-concurrent-changes） | 跑 `go test ./...` 与 `go test -short ./...` | 已实测：全量 23 包失败，均指向 **`heal-dangling-article-refs`** 的未完成迁移 `20260917_0002`（引用已被 destructive migration `20260824_0001` drop 的 `daily_report_threads`，测试环境因 `MIGRATIONS_ALLOW_DESTRUCTIVE=1` 必然踩中）；`-short` 37 包 ok + 1 包 panic（`tagmanagement/service/merge` 后台 goroutine nil pointer）——本 change **未触及** tagmanagement/daily_report/migrations 任何文件 | 归档验证按「本改动影响面」判定：本 change 改动的模块（harness/scripts/注释/spec）全绿；孤立失败均归属他人中间态或既有 race |
| Ryuk 补齐是否止住泄漏 | Ryuk 镜像缺失 → sidecar 起不来 → 容器无人回收 | 单测一个集成测试，运行前后统计容器数 | 已实测（2026-09-16）：38 → 36（回到基线），测试 5.57s PASS | 回收链恢复 |
| 仓库脚本在 POSIX 宿主上是否真能跑 | git 记录 mode=100644，WSL/DrvFS 忽略权限掩盖了缺失的执行位 | `find . -name '*.sh' ! -perm -u+x` 计数 + 直调脚本的 smoke | 修复前：`scenario-trace.smoke.sh` 9 个用例全 rc=126（Permission denied）；`chmod +x` 后 9/9 全过 | 迁移遗留的权限缺失已消除 |
| 镜像可达性是否零仓库耦合 | Docker Hub 直连不通 | 配置 `registry-mirrors` 后按原始名拉取 | 待实测（tasks 1.2）：两个镜像原始名拉取成功且 `git status` 干净 | 机器级配置足够 |

## 4. 白盒附加（平台分流分支表 + 边界值）

**判定分支表**（探测结果 × 会话缓存态 → 模式与门禁行为）：

| # | 探测结果 | 会话缓存 | 判定模式 | 门禁行为 | 记账 |
|---|---|---|---|---|---|
| T1 | spawn 失败（ENOENT，`cmd.exe` 不存在） | 空 | native | 本机 `go`/`golangci-lint`/`pnpm` + `cwd` 执行 | `gate.check` 正常（成功采样/失败全量） |
| T2 | spawn 失败（ENOENT） | 已 native | native | 同上（**不再探测**，直接执行） | 同上 |
| T3 | spawn 失败（ENOENT） | 已 windows | native | 会话内不应出现该组合（平台身份不逐回合变），若出现按 T1 处理 | 同上 |
| T4 | 成功，exit=0 | 空 | windows | `cmd.exe /C "cd /d <winPath> && <cmd>"` | `gate.check` 正常 |
| T5 | 成功，exit=0 | 已 windows | windows | 同上（可直接复用缓存判定） | 同上 |
| T6 | 成功，exit≠0 | 任意 | 短路 | 零门禁执行 | `policy.decision(fail-open / interop-down)`，零 `gate.check` |
| T7 | 超时（>2s） | 任意 | 短路 | 零门禁执行 | 同 T6 |
| T8 | 抛异常（非 ENOENT，如权限/内部错误） | 任意 | 短路 | 零门禁执行，不向 agent 抛错 | 同 T6 |
| T9 | 未探测（本回合无门禁需要） | 任意 | 未判定 | 零门禁执行（step 1/3 早退） | 零事件 |

**边界值清单**：

- 探测超时边界：`INTEROP_PROBE_TIMEOUT_MS = 2000`（正常 ~40ms 的 50 倍余量）；跨越该阈值即由 T4 转 T7。
- ENOENT 与「cmd.exe 存在但不可执行」的区分：仅 `spawn` 层 ENOENT 归 native；子进程已建立而返回非零归短路（T6）。
- 判定缓存生命周期：`session_start`（非 startup）与 `session_shutdown` 重置；子线程 session 事件不得清主会话缓存（沿用既有 ownerSessionId 防御）。
- native 模式下 `util AcceptVsock` 字样出现在输出：不得命中环境故障归因（故事 A7）。
- 命令工作目录边界：`cwd` 为 `<repoRoot>/backend-go` 与 `<repoRoot>/front`，不依赖 shell `cd`；路径含空格时仍成立（由进程 API 传参，无引号吞噬问题）。

## 5. 继承与调整（问句⓪：旧契约的既有测试如何处置）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
|---|---|---|---|
| 探测失败短路整轮 cmd 链路门禁 | 保留（措辞微调：加「cmd.exe 存在但」前提） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变，跑绿即可 |
| 探测成功不改变既有门禁行为 | 保留（语义扩展为「Windows 模式下」不变） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变；命令文本逐字比对继续生效 |
| 探测调用异常也 fail-open | 保留（异常范围收窄为非 ENOENT） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | **需新增**「ENOENT 不走短路」反向断言（故事 A2） |
| 环境故障失败不触发粘性重跑 | 保留（限定 cmd 链路模式） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变；**新增** native 下不误判（故事 A7） |
| 真实代码失败保持既有语义 | 保留 | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变 |
| 后端门禁失败提示含链路标注 | 保留（限定 cmd 链路模式） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变；**新增** native 无 `wsl环境` 断言（故事 A5） |
| 环境故障归因与链路标注同时呈现 | 保留 | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` | 断言不变 |
| 前端命令标注跨平台限制 | 修改（标注改为按平台） | 人工核对 `scripts/change-scope.sh --json` | **改脚本 notices 文案**，人工重核 |
| 前端改动行为不变 | 保留（理由从 cmd 限制改为分层设计） | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（既有行为断言） | **需新增**「native 下仍不跑 typecheck/build」断言（故事 A4） |
| 残留前端脏文件不触发前端门禁 | 保留 | `.pi/extensions/tests/quality-gate.smoke.cjs` | 断言不变 |
| 会话前残留改动不触发首轮门禁 | 保留 | `.pi/extensions/tests/quality-gate.smoke.cjs` | 断言不变 |
| 修复循环每轮按新变化触发对应侧 | 保留 | `.pi/extensions/tests/quality-gate.smoke.cjs` | 断言不变 |
| （全部 scenario-trace-gate 既有 Scenario） | 保留（仅 requirement 措辞去 WSL） | `scripts/scenario-trace.smoke.sh` | 断言不变，跑绿即可 |
| WSL 可达前端 dev server | 保留标题、内容平台中立化 | 人工（原为 evidence/front-3000.log） | 人工重核：本机 + 局域网双地址访问 |
| 文档口径一致 | 保留 | 人工（grep） | 人工重核 |
| 前端 dev server 绑定 / apiBase / 端口类 Scenario | 不变（本 change 未触碰） | 既有前端测试与人工证据 | 无需动作 |

**注意**：`gate-interop-health` 是 MODIFIED 契约，旧测试**跑绿不等于对**——上表前 7 行必须逐行确认断言仍在测新契约下的正确行为（尤其「异常 → 短路」这条，ENOENT 已从「异常」中分离出去）。
